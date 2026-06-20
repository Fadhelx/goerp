package mail

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestProcessEmailQueueSendsDueOutgoingMails(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "email": "ada@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Due", "body": "<p>Due</p>", "message_type": "email", "author_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":     "brief.txt",
		"type":     "binary",
		"mimetype": "text/plain",
		"datas":    "YnJpZWY=",
	})
	if err != nil {
		t.Fatal(err)
	}
	dueID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@example.com>",
		"email_to":        "ada@example.com",
		"reply_to":        "reply@example.com",
		"subject":         "Due",
		"body_html":       "<p>Due</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(-time.Hour),
		"recipient_ids":   []int64{partnerID},
		"attachment_ids":  []int64{attachmentID},
		"headers":         `{"X-Test":"ok"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":   dueID,
		"email":          "ada@example.com",
		"model":          "res.partner",
		"res_id":         partnerID,
		"failure_reason": "kept by source set_sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	futureID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "future@example.com",
		"subject":         "Future",
		"body_html":       "<p>Future</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "sent@example.com",
		"subject":         "Sent",
		"body_html":       "<p>Sent</p>",
		"state":           "sent",
	}); err != nil {
		t.Fatal(err)
	}
	notificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        dueID,
		"res_partner_id":      partnerID,
		"mail_email_address":  "ada@example.com",
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := ProcessEmailQueue(context.Background(), env, sender, QueueOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	if len(sender.sent) != 2 || sender.sent[0].ID != dueID || sender.sent[0].From != "Sender <sender@example.com>" || sender.sent[0].To != "ada@example.com" || sender.sent[0].ReplyTo != "reply@example.com" || sender.sent[0].Headers["X-Test"] != "ok" || len(sender.sent[0].Attachments) != 1 || string(sender.sent[0].Attachments[0].Data) != "brief" || !sender.sent[0].CreatedAt.Equal(now) {
		t.Fatalf("sent messages = %+v", sender.sent)
	}
	if sender.sent[1].To != "ada@example.com" || sender.sent[1].CC != "" {
		t.Fatalf("partner split message = %+v", sender.sent[1])
	}
	dueRows, err := env.Model("mail.mail").Browse(dueID).Read("state", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(dueRows) != 1 || dueRows[0]["state"] != "sent" || dueRows[0]["failure_type"] != "" || dueRows[0]["failure_reason"] != "" || dueRows[0]["message_id"] == "" {
		t.Fatalf("due mail row = %+v", dueRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "sent_datetime", "failure_type", "failure_reason", "message_id", "mail_mail_id_int")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		traceRows[0]["trace_status"] != "sent" ||
		timeValue(traceRows[0]["sent_datetime"]).IsZero() ||
		traceRows[0]["failure_type"] != "" ||
		traceRows[0]["failure_reason"] != "kept by source set_sent" ||
		traceRows[0]["message_id"] != dueRows[0]["message_id"] ||
		traceRows[0]["mail_mail_id_int"] != dueID {
		t.Fatalf("trace row = %+v mail=%+v", traceRows, dueRows)
	}
	futureRows, err := env.Model("mail.mail").Browse(futureID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(futureRows) != 1 || futureRows[0]["state"] != "outgoing" {
		t.Fatalf("future row = %+v", futureRows)
	}
	notificationRows, err := env.Model("mail.notification").Browse(notificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != "sent" || notificationRows[0]["failure_type"] != "" || notificationRows[0]["failure_reason"] != "" {
		t.Fatalf("notification row = %+v", notificationRows)
	}
}

func TestMailQueueRewritesMassMailingTrackedLinksAndAppendsOpenPixel(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "queue-track-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Tracked", "email": "tracked@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Tracked Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	body := `<html><p><a href="https://gorp.example/r/AbC123">Track</a><a href="https://gorp.example/r/Already/m/9">Already</a><a href="mailto:test@example.com">Mail</a><a href="https://example.com/page">Plain</a><a href="/unsubscribe_from_list">Unsub</a><a href="/view">View</a></p></html>`
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "tracked@example.com",
		"subject":        "Tracked",
		"body_html":      body,
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "tracked@example.com",
		"model":           "res.partner",
		"res_id":          partnerID,
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || len(sender.sent) != 1 {
		t.Fatalf("queue result = %+v sent=%+v", result, sender.sent)
	}
	sentBody := sender.sent[0].Body
	if !strings.Contains(sentBody, "https://gorp.example/r/AbC123/m/"+strconv.FormatInt(traceID, 10)) {
		t.Fatalf("tracked link not rewritten: %s", sentBody)
	}
	if !strings.Contains(sentBody, "https://gorp.example/r/Already/m/9/m/"+strconv.FormatInt(traceID, 10)) {
		t.Fatalf("existing traced link did not follow source double append: %s", sentBody)
	}
	if !strings.Contains(sentBody, `href="mailto:test@example.com"`) {
		t.Fatalf("mailto link changed: %s", sentBody)
	}
	if !strings.Contains(sentBody, `href="https://example.com/page"`) {
		t.Fatalf("plain link changed: %s", sentBody)
	}
	if !strings.Contains(sentBody, "https://gorp.example/mailing/"+strconv.FormatInt(mailingID, 10)+"/confirm_unsubscribe?") || strings.Contains(sentBody, `href="/unsubscribe_from_list"`) {
		t.Fatalf("unsubscribe link not rewritten: %s", sentBody)
	}
	if !strings.Contains(sentBody, "https://gorp.example/mailing/"+strconv.FormatInt(mailingID, 10)+"/view?") || strings.Contains(sentBody, `href="/view"`) {
		t.Fatalf("view link not rewritten: %s", sentBody)
	}
	if !strings.Contains(sentBody, "https://gorp.example/mail/track/"+strconv.FormatInt(mailID, 10)+"/") || !strings.Contains(sentBody, "/blank.gif") {
		t.Fatalf("open pixel missing: %s", sentBody)
	}
	if strings.Index(sentBody, `<img src="https://gorp.example/mail/track/`) > strings.Index(sentBody, "</html>") {
		t.Fatalf("open pixel not inserted before html close: %s", sentBody)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["body_html"] != body {
		t.Fatalf("stored body mutated = %+v", mailRows)
	}
	headers := sender.sent[0].Headers
	if !strings.Contains(headers["List-Unsubscribe"], "/unsubscribe_oneclick?") || !strings.Contains(headers["List-Unsubscribe"], "/confirm_unsubscribe?") || headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || headers["Precedence"] != "list" || headers["X-Auto-Response-Suppress"] != "OOF" {
		t.Fatalf("mass mailing headers = %+v", headers)
	}
}

func TestMailQueueCancelsListOptOutMassMailingRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Optout List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Opted", "email": "Opted@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID, "opt_out": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Optout Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "opted@example.com",
		"subject":        "Optout",
		"body_html":      "<p>Optout</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "opted@example.com",
		"model":           "mailing.contact",
		"res_id":          contactID,
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 0 || result.Failed != 0 || result.Skipped != 1 || len(sender.sent) != 0 {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "cancel" || mailRows[0]["failure_type"] != "mail_optout" || stringAny(mailRows[0]["failure_reason"]) != "" || stringAny(mailRows[0]["message_id"]) != "" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "message_id", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "cancel" || traceRows[0]["failure_type"] != "mail_optout" || stringAny(traceRows[0]["message_id"]) != "" || !timeValue(traceRows[0]["sent_datetime"]).IsZero() {
		t.Fatalf("trace row = %+v", traceRows)
	}
}

func TestMailQueueCancelsBlacklistedMassMailingRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("mail.blacklist").Create(map[string]any{"email": "queued.blocked@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Queued Blacklist"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "Queued Blocked <queued.blocked@example.com>",
		"subject":        "Queued Blacklist",
		"body_html":      "<p>Queued Blacklist</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "queued.blocked@example.com",
		"model":           "mailing.contact",
		"res_id":          int64(99),
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 0 || result.Failed != 0 || result.Skipped != 1 || len(sender.sent) != 0 {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "cancel" || mailRows[0]["failure_type"] != "mail_bl" || stringAny(mailRows[0]["failure_reason"]) != "" || stringAny(mailRows[0]["message_id"]) != "" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "message_id", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "cancel" || traceRows[0]["failure_type"] != "mail_bl" || stringAny(traceRows[0]["message_id"]) != "" || !timeValue(traceRows[0]["sent_datetime"]).IsZero() {
		t.Fatalf("trace row = %+v", traceRows)
	}
}

func TestMailQueueIgnoresBlacklistWhenExclusionDisabled(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("mail.blacklist").Create(map[string]any{"email": "queued.allowed@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Queued Allowed", "use_exclusion_list": false})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "queued.allowed@example.com",
		"subject":        "Queued Allowed",
		"body_html":      "<p>Queued Allowed</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "queued.allowed@example.com",
		"model":           "mailing.contact",
		"res_id":          int64(100),
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 || len(sender.sent) != 1 || sender.sent[0].To != "queued.allowed@example.com" {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || traceRows[0]["failure_type"] != "" || timeValue(traceRows[0]["sent_datetime"]).IsZero() {
		t.Fatalf("trace row = %+v", traceRows)
	}
}

func TestMailQueueCancelsLaterDuplicateMassMailingRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Queued Duplicate"})
	if err != nil {
		t.Fatal(err)
	}
	firstMailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "dupe.queue@example.com",
		"subject":        "Queued Duplicate",
		"body_html":      "<p>First</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondMailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "Dupe Queue <DUPE.QUEUE@example.com>",
		"subject":        "Queued Duplicate",
		"body_html":      "<p>Second</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstTraceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    firstMailID,
		"email":           "dupe.queue@example.com",
		"model":           "mailing.contact",
		"res_id":          int64(101),
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondTraceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    secondMailID,
		"email":           "dupe.queue@example.com",
		"model":           "mailing.contact",
		"res_id":          int64(102),
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{firstMailID, secondMailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 2 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 1 || len(sender.sent) != 1 || sender.sent[0].To != "dupe.queue@example.com" {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	mailRows, err := env.Model("mail.mail").Browse(firstMailID, secondMailID).Read("state", "failure_type", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 2 ||
		mailRows[0]["state"] != "sent" ||
		stringAny(mailRows[0]["failure_type"]) != "" ||
		stringAny(mailRows[0]["message_id"]) == "" ||
		mailRows[1]["state"] != "cancel" ||
		mailRows[1]["failure_type"] != "mail_dup" ||
		stringAny(mailRows[1]["message_id"]) != "" {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(firstTraceID, secondTraceID).Read("trace_status", "failure_type", "message_id", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 2 ||
		traceRows[0]["trace_status"] != "sent" ||
		stringAny(traceRows[0]["failure_type"]) != "" ||
		stringAny(traceRows[0]["message_id"]) == "" ||
		timeValue(traceRows[0]["sent_datetime"]).IsZero() ||
		traceRows[1]["trace_status"] != "cancel" ||
		traceRows[1]["failure_type"] != "mail_dup" ||
		stringAny(traceRows[1]["message_id"]) != "" ||
		!timeValue(traceRows[1]["sent_datetime"]).IsZero() {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestMailQueueListOptInOverridesOptOutForSelectedLists(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	optoutListID, err := env.Model("mailing.list").Create(map[string]any{"name": "Optout List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	optinListID, err := env.Model("mailing.list").Create(map[string]any{"name": "Optin List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Override", "email": "override@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": optoutListID, "opt_out": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": optinListID, "opt_out": false}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Override Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{optoutListID, optinListID},
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "override@example.com",
		"subject":        "Override",
		"body_html":      "<p>Override</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "override@example.com",
		"model":           "mailing.contact",
		"res_id":          contactID,
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 || len(sender.sent) != 1 || sender.sent[0].To != "override@example.com" {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || traceRows[0]["failure_type"] != "" || timeValue(traceRows[0]["sent_datetime"]).IsZero() {
		t.Fatalf("trace row = %+v", traceRows)
	}
}

func TestMailQueueFiltersOnlyListOptOutRawRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Mixed List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	optedID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Opted", "email": "Opted <opted@example.com>", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activeID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Active", "email": "active@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": optedID, "list_id": listID, "opt_out": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": activeID, "list_id": listID, "opt_out": false}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Mixed Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "Opted <opted@example.com>, active@example.com",
		"subject":        "Mixed",
		"body_html":      "<p>Mixed</p>",
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	optedTraceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "opted@example.com",
		"model":           "mailing.contact",
		"res_id":          optedID,
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	activeTraceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "active@example.com",
		"model":           "mailing.contact",
		"res_id":          activeID,
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 || len(sender.sent) != 1 || sender.sent[0].To != "active@example.com" {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	traceRows, err := env.Model("mailing.trace").Browse(optedTraceID, activeTraceID).Read("trace_status", "failure_type", "message_id", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 2 ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_optout" ||
		stringAny(traceRows[0]["message_id"]) != "" ||
		!timeValue(traceRows[0]["sent_datetime"]).IsZero() ||
		traceRows[1]["trace_status"] != "sent" ||
		traceRows[1]["failure_type"] != "" ||
		stringAny(traceRows[1]["message_id"]) == "" ||
		timeValue(traceRows[1]["sent_datetime"]).IsZero() {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestMailQueueDoesNotRewriteMassMailingBodyWithoutTrace(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "queue-no-trace-secret"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "No Trace Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	body := `<p><a href="https://gorp.example/r/AbC123">Track</a></p>`
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"email_to":       "no.trace@example.com",
		"subject":        "No Trace",
		"body_html":      body,
		"state":          "outgoing",
		"scheduled_date": now.Add(-time.Minute),
		"mailing_id":     mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || len(sender.sent) != 1 {
		t.Fatalf("queue result = %+v sent=%+v", result, sender.sent)
	}
	if sender.sent[0].Body != body {
		t.Fatalf("body rewritten without trace: %s", sender.sent[0].Body)
	}
}

func TestMailQueueSkipsMailingTraceUpdateWhenTraceModelAbsent(t *testing.T) {
	env := queueEnvWithoutMailingTrace(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "No trace", "body": "<p>No trace</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "no.trace@example.com",
		"subject":         "No trace",
		"body_html":       "<p>No trace</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, &recordingSender{}, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("state", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "sent" || rows[0]["message_id"] == "" {
		t.Fatalf("mail row = %+v", rows)
	}
}

func TestDueMailIDsOrdersByCreateDateBeforeID(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Order", "body": "<p>Order</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	olderHighID, err := env.Model("mail.mail").Create(map[string]any{
		"id":              int64(200),
		"mail_message_id": messageID,
		"email_to":        "older@example.com",
		"subject":         "Older",
		"body_html":       "<p>Older</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newerLowID, err := env.Model("mail.mail").Create(map[string]any{
		"id":              int64(100),
		"mail_message_id": messageID,
		"email_to":        "newer@example.com",
		"subject":         "Newer",
		"body_html":       "<p>Newer</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	ids, err := DueMailIDs(env, QueueOptions{Now: now, EmailIDs: []int64{newerLowID, olderHighID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 2 || ids[0] != olderHighID || ids[1] != newerLowID {
		t.Fatalf("due ids = %+v", ids)
	}
}

func queueEnvWithoutMailingTrace(t *testing.T) *record.Env {
	t.Helper()
	registry := record.NewRegistry()
	for _, m := range base.Models() {
		if m.Name == "mailing.trace" || m.Name == "mailing.mailing" {
			continue
		}
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(registry, record.Context{UserID: 1})
}

func TestMailQueueSplitsRecipientsAndKeepsPartialNotificationFailures(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	validPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Valid", "email": "valid@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	missingPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Missing", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Partial", "body": "<p>Partial</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "raw@example.com",
		"email_cc":        "copy@example.com",
		"subject":         "Partial",
		"body_html":       "<p>Partial</p>",
		"state":           "outgoing",
		"recipient_ids":   []int64{validPartnerID, missingPartnerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	validNotificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"res_partner_id":      validPartnerID,
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingNotificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"res_partner_id":      missingPartnerID,
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	rawNotificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"mail_email_address":  "raw@example.com",
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	traceIDs := map[string]int64{}
	for _, spec := range []struct {
		label string
		email string
		model string
		resID int64
	}{
		{"valid", "valid@example.com", "res.partner", validPartnerID},
		{"missing", "missing@example.com", "res.partner", missingPartnerID},
		{"raw", "raw@example.com", "mail.message", messageID},
	} {
		traceID, err := env.Model("mailing.trace").Create(map[string]any{
			"mail_mail_id": mailID,
			"email":        spec.email,
			"model":        spec.model,
			"res_id":       spec.resID,
		})
		if err != nil {
			t.Fatal(err)
		}
		traceIDs[spec.label] = traceID
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || len(sender.sent) != 2 {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	if sender.sent[0].To != "raw@example.com" || sender.sent[0].CC != "copy@example.com" || sender.sent[1].To != "valid@example.com" {
		t.Fatalf("sent messages = %+v", sender.sent)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "sent" || mailRows[0]["failure_type"] != "" || mailRows[0]["failure_reason"] != "" || mailRows[0]["message_id"] == "" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	validRows, err := env.Model("mail.notification").Browse(validNotificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(validRows) != 1 || validRows[0]["notification_status"] != "sent" || validRows[0]["failure_type"] != "" || validRows[0]["failure_reason"] != "" {
		t.Fatalf("valid notification = %+v", validRows)
	}
	rawRows, err := env.Model("mail.notification").Browse(rawNotificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rawRows) != 1 || rawRows[0]["notification_status"] != "sent" || rawRows[0]["failure_type"] != "" || rawRows[0]["failure_reason"] != "" {
		t.Fatalf("raw notification = %+v", rawRows)
	}
	missingRows, err := env.Model("mail.notification").Browse(missingNotificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(missingRows) != 1 || missingRows[0]["notification_status"] != "exception" || missingRows[0]["failure_type"] != "mail_email_missing" || missingRows[0]["failure_reason"] != "send failed" {
		t.Fatalf("missing notification = %+v", missingRows)
	}
	for _, spec := range []struct {
		label string
		id    int64
	}{
		{"valid", traceIDs["valid"]},
		{"missing", traceIDs["missing"]},
		{"raw", traceIDs["raw"]},
	} {
		rows, err := env.Model("mailing.trace").Browse(spec.id).Read("trace_status", "failure_type", "failure_reason", "message_id", "sent_datetime")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["trace_status"] != "error" || rows[0]["failure_type"] != "mail_email_missing" || stringAny(rows[0]["failure_reason"]) != "" || rows[0]["message_id"] != mailRows[0]["message_id"] || !timeValue(rows[0]["sent_datetime"]).IsZero() {
			t.Fatalf("%s trace = %+v mail=%+v", spec.label, rows, mailRows)
		}
	}
}

func TestProcessEmailQueueResolvesMailServerAndAttachments(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "No Filter",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Domain Filter",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "other.example, example.com",
		"sequence":        int64(20),
	}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":     "report.txt",
		"type":     "binary",
		"mimetype": "text/plain",
		"datas":    "c2FtcGxl",
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Server", "body": "<p>Server</p>", "message_type": "email", "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@example.com>",
		"email_to":        "recipient@example.com",
		"email_cc":        "copy@example.com",
		"reply_to":        "reply@example.com",
		"subject":         "Server",
		"body_html":       "<p>Server</p>",
		"state":           "outgoing",
		"headers":         `{"X-Queue":"ok"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := ProcessEmailQueue(context.Background(), env, nil, QueueOptions{Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "sender@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	if len(server.rcptTo) != 2 || server.rcptTo[0] != "recipient@example.com" || server.rcptTo[1] != "copy@example.com" {
		t.Fatalf("rcpt to = %+v", server.rcptTo)
	}
	for _, needle := range []string{
		"From: Sender <sender@example.com>\r\n",
		"Reply-To: reply@example.com\r\n",
		"X-Queue: ok\r\n",
		"Content-Type: multipart/mixed;",
		"Content-Disposition: attachment; filename=report.txt",
		"c2FtcGxl",
	} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "sent" || rows[0]["failure_type"] != "" || rows[0]["failure_reason"] != "" {
		t.Fatalf("mail row = %+v", rows)
	}
}

func TestMailQueueTreatsPartialSMTPRCPTRefusalAsSent(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") && trimSMTPPath(strings.TrimPrefix(line, "RCPT TO:")) == "refused@example.com" {
			return "550 bad recipient\r\n", true
		}
		return "", false
	})
	defer server.Close()
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Partial Refusal",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Partial RCPT", "body": "<p>Partial</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "accepted@example.com, refused@example.com",
		"subject":         "Partial RCPT",
		"body_html":       "<p>Partial</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	notificationIDs := map[string]int64{}
	traceIDs := map[string]int64{}
	for _, email := range []string{"accepted@example.com", "refused@example.com"} {
		notificationID, err := env.Model("mail.notification").Create(map[string]any{
			"mail_message_id":     messageID,
			"mail_mail_id":        mailID,
			"mail_email_address":  email,
			"notification_type":   "email",
			"notification_status": "ready",
		})
		if err != nil {
			t.Fatal(err)
		}
		traceID, err := env.Model("mailing.trace").Create(map[string]any{
			"mail_mail_id": mailID,
			"email":        email,
			"model":        "mail.message",
			"res_id":       messageID,
		})
		if err != nil {
			t.Fatal(err)
		}
		notificationIDs[email] = notificationID
		traceIDs[email] = traceID
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if len(server.rcptTo) != 2 || server.rcptTo[0] != "accepted@example.com" || server.rcptTo[1] != "refused@example.com" || len(server.acceptedRcptTo) != 1 || server.acceptedRcptTo[0] != "accepted@example.com" || len(server.refusedRcptTo) != 1 || server.refusedRcptTo[0] != "refused@example.com" || server.data == "" {
		t.Fatalf("server state rcpt=%+v accepted=%+v refused=%+v data=%q", server.rcptTo, server.acceptedRcptTo, server.refusedRcptTo, server.data)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "sent" || mailRows[0]["failure_type"] != "" || mailRows[0]["failure_reason"] != "" || mailRows[0]["message_id"] == "" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	for _, email := range []string{"accepted@example.com", "refused@example.com"} {
		notificationRows, err := env.Model("mail.notification").Browse(notificationIDs[email]).Read("notification_status", "failure_type", "failure_reason")
		if err != nil {
			t.Fatal(err)
		}
		if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != "sent" || notificationRows[0]["failure_type"] != "" || notificationRows[0]["failure_reason"] != "" {
			t.Fatalf("%s notification = %+v", email, notificationRows)
		}
		traceRows, err := env.Model("mailing.trace").Browse(traceIDs[email]).Read("trace_status", "sent_datetime", "failure_type", "message_id")
		if err != nil {
			t.Fatal(err)
		}
		if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || timeValue(traceRows[0]["sent_datetime"]).IsZero() || traceRows[0]["failure_type"] != "" || traceRows[0]["message_id"] != mailRows[0]["message_id"] {
			t.Fatalf("%s trace = %+v mail=%+v", email, traceRows, mailRows)
		}
	}
}

func TestMailQueueFailsWhenAllSMTPRCPTRefused(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") {
			return "550 bad recipient\r\n", true
		}
		return "", false
	})
	defer server.Close()
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "All Refused",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "All Refused", "body": "<p>All Refused</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "first@example.com, second@example.com",
		"subject":         "All Refused",
		"body_html":       "<p>All Refused</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	notificationIDs := map[string]int64{}
	traceIDs := map[string]int64{}
	for _, email := range []string{"first@example.com", "second@example.com"} {
		notificationID, err := env.Model("mail.notification").Create(map[string]any{
			"mail_message_id":     messageID,
			"mail_mail_id":        mailID,
			"mail_email_address":  email,
			"notification_type":   "email",
			"notification_status": "ready",
		})
		if err != nil {
			t.Fatal(err)
		}
		traceID, err := env.Model("mailing.trace").Create(map[string]any{
			"mail_mail_id": mailID,
			"email":        email,
			"model":        "mail.message",
			"res_id":       messageID,
		})
		if err != nil {
			t.Fatal(err)
		}
		notificationIDs[email] = notificationID
		traceIDs[email] = traceID
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 0 || result.Failed != 1 || result.Skipped != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if len(server.rcptTo) != 2 || len(server.acceptedRcptTo) != 0 || len(server.refusedRcptTo) != 2 || server.data != "" {
		t.Fatalf("server state rcpt=%+v accepted=%+v refused=%+v data=%q", server.rcptTo, server.acceptedRcptTo, server.refusedRcptTo, server.data)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "exception" || mailRows[0]["failure_type"] != "mail_smtp" || mailRows[0]["failure_reason"] != "send failed" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	for _, email := range []string{"first@example.com", "second@example.com"} {
		notificationRows, err := env.Model("mail.notification").Browse(notificationIDs[email]).Read("notification_status", "failure_type", "failure_reason")
		if err != nil {
			t.Fatal(err)
		}
		if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != "exception" || notificationRows[0]["failure_type"] != "mail_smtp" || notificationRows[0]["failure_reason"] != "send failed" {
			t.Fatalf("%s notification = %+v", email, notificationRows)
		}
		traceRows, err := env.Model("mailing.trace").Browse(traceIDs[email]).Read("trace_status", "sent_datetime", "failure_type", "failure_reason", "message_id")
		if err != nil {
			t.Fatal(err)
		}
		if len(traceRows) != 1 || traceRows[0]["trace_status"] != "error" || !timeValue(traceRows[0]["sent_datetime"]).IsZero() || traceRows[0]["failure_type"] != "mail_smtp" || stringAny(traceRows[0]["failure_reason"]) != "" || traceRows[0]["message_id"] == "" {
			t.Fatalf("%s trace = %+v", email, traceRows)
		}
	}
}

func TestMailQueueSkipsBodyLinkedAttachments(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Default",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":     "linked.txt",
		"type":     "binary",
		"mimetype": "text/plain",
		"datas":    "bGlua2Vk",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `<p>Linked <a href="/web/content/` + strconv.FormatInt(attachmentID, 10) + `?download=1&amp;access_token=stored">file</a></p>`
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Linked", "body": body, "message_type": "email", "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "Linked",
		"body_html":       body,
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if strings.Contains(server.data, "filename=linked.txt") || strings.Contains(server.data, "bGlua2Vk") || strings.Contains(server.data, "multipart/mixed") {
		t.Fatalf("body-linked attachment was sent as binary: %q", server.data)
	}
	if !strings.Contains(server.data, "/web/content/"+strconv.FormatInt(attachmentID, 10)) {
		t.Fatalf("body link missing: %q", server.data)
	}
}

func TestMailQueueConvertsURLOnlyAttachmentsToBodyLinks(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Default",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":     "cloud.pdf",
		"type":     "url",
		"url":      "https://files.example/cloud.pdf",
		"mimetype": "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "URL", "body": "<p>URL</p>", "message_type": "email", "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "URL",
		"body_html":       "<p>URL</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if strings.Contains(server.data, "filename=cloud.pdf") || strings.Contains(server.data, "multipart/mixed") {
		t.Fatalf("url attachment was sent as binary: %q", server.data)
	}
	for _, needle := range []string{"Download cloud.pdf", "/web/content/" + strconv.FormatInt(attachmentID, 10) + "?download=1", "access_token="} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
	rows, err := env.Model("ir.attachment").Browse(attachmentID).Read("access_token")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || strings.TrimSpace(stringAny(rows[0]["access_token"])) == "" {
		t.Fatalf("attachment token = %+v", rows)
	}
}

func TestMailQueueConvertsOversizedRecordOwnedAttachmentsToBodyLinks(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "base.default_max_email_size",
		"value": "0.001",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Default",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Record", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "owned.bin",
		"type":      "binary",
		"mimetype":  "application/octet-stream",
		"datas":     "b3duZWQ=",
		"res_model": "res.partner",
		"res_id":    recordID,
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Owned", "body": "<p>Owned</p>", "message_type": "email", "model": "res.partner", "res_id": recordID, "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "Owned",
		"body_html":       "<p>Owned</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if strings.Contains(server.data, "filename=owned.bin") || strings.Contains(server.data, "b3duZWQ=") || strings.Contains(server.data, "multipart/mixed") {
		t.Fatalf("oversized record attachment was sent as binary: %q", server.data)
	}
	for _, needle := range []string{"Download owned.bin", "/web/content/" + strconv.FormatInt(attachmentID, 10) + "?download=1", "access_token="} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
}

func TestMailQueueDefaultSelectionIgnoresPersonalServersAndUsesNotificationFallback(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "mail.default.from",
		"value": "notifications@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "sender@example.com",
		"owner_user_id":   int64(99),
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Notifications",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "notifications@example.com",
		"sequence":        int64(2),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Fallback", "body": "<p>Fallback</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Fallback",
		"body_html":       "<p>Fallback</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "notifications@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	if !strings.Contains(server.data, "From: notifications@example.com\r\n") {
		t.Fatalf("missing notification from: %q", server.data)
	}
}

func TestMailQueueUsesAliasDomainBounceAsEnvelopeSender(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_alias":   "bounce",
		"default_from":   "notifications",
		"catchall_alias": "catchall",
		"sequence":       int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{
		"name":            "MailCo",
		"alias_domain_id": aliasDomainID,
	})
	if err != nil {
		t.Fatal(err)
	}
	env = env.WithContext(record.Context{UserID: 1, CompanyID: companyID, CompanyIDs: []int64{companyID}})
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Alias Domain",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "example.com",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Bounce", "body": "<p>Bounce</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@other.example>",
		"email_to":        "recipient@example.com",
		"subject":         "Bounce",
		"body_html":       "<p>Bounce</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "bounce@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	for _, needle := range []string{
		"From: notifications@example.com\r\n",
		"Return-Path: bounce@example.com\r\n",
	} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
}

func TestMailQueueUsesMessageRecordAliasDomainForDefaultFromAndBounce(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	contextAliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":         "context.example",
		"bounce_alias": "bounce",
		"default_from": "notifications",
		"sequence":     int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	recordAliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":         "record.example",
		"bounce_alias": "bounce",
		"default_from": "notifications",
		"sequence":     int64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextCompanyID, err := env.Model("res.company").Create(map[string]any{
		"name":            "ContextCo",
		"alias_domain_id": contextAliasDomainID,
	})
	if err != nil {
		t.Fatal(err)
	}
	env = env.WithContext(record.Context{UserID: 1, CompanyID: contextCompanyID, CompanyIDs: []int64{contextCompanyID}})
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Record Domain",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "record.example",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{
		"subject":                "Record Domain",
		"body":                   "<p>Record Domain</p>",
		"message_type":           "email",
		"record_alias_domain_id": recordAliasDomainID,
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@other.example>",
		"email_to":        "recipient@example.com",
		"subject":         "Record Domain",
		"body_html":       "<p>Record Domain</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "bounce@record.example" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	for _, needle := range []string{
		"From: notifications@record.example\r\n",
		"Return-Path: bounce@record.example\r\n",
	} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
}

func TestMailQueueUsesLinkedRecordCompanyAliasDomainForDefaultFromAndBounce(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	contextAliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":         "context.example",
		"bounce_alias": "bounce",
		"default_from": "notifications",
		"sequence":     int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	recordAliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":         "record.example",
		"bounce_alias": "bounce",
		"default_from": "notifications",
		"sequence":     int64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextCompanyID, err := env.Model("res.company").Create(map[string]any{
		"name":            "ContextCo",
		"alias_domain_id": contextAliasDomainID,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordCompanyID, err := env.Model("res.company").Create(map[string]any{
		"name":            "RecordCo",
		"alias_domain_id": recordAliasDomainID,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{
		"name":       "Record Partner",
		"active":     true,
		"company_id": recordCompanyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	env = env.WithContext(record.Context{UserID: 1, CompanyID: contextCompanyID, CompanyIDs: []int64{contextCompanyID, recordCompanyID}})
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Record Domain",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "record.example",
		"sequence":        int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Linked Record Domain",
		"body":         "<p>Linked Record Domain</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Sender <sender@other.example>",
		"email_to":        "recipient@example.com",
		"subject":         "Linked Record Domain",
		"body_html":       "<p>Linked Record Domain</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "bounce@record.example" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	for _, needle := range []string{
		"From: notifications@record.example\r\n",
		"Return-Path: bounce@record.example\r\n",
	} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
}

func TestMailMailCreateRejectsUnauthorizedForcedPersonalServer(t *testing.T) {
	env, _ := threadEnv(t)
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   int64(99),
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Denied", "body": "<p>Denied</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "sender@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "Denied",
		"body_html":       "<p>Denied</p>",
		"state":           "outgoing",
	})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("create error = %v", err)
	}
}

func TestMailMailWriteRejectsUnauthorizedForcedPersonalServer(t *testing.T) {
	env, _ := threadEnv(t)
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   int64(99),
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Denied", "body": "<p>Denied</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "owner@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "Denied",
		"body_html":       "<p>Denied</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = env.Model("mail.mail").Browse(mailID).Write(map[string]any{"mail_server_id": serverID})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("write error = %v", err)
	}
}

func TestMailQueueRejectsAuthorSpoofWithoutOwnerOutgoingServer(t *testing.T) {
	env, _ := threadEnv(t)
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-spoof", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Spoof", "body": "<p>Spoof</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Spoof",
		"body_html":       "<p>Spoof</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 {
		t.Fatalf("queue result = %+v", result)
	}
}

func TestMailMailCreateRejectsForcedPersonalServerFromDifferentMessageCreator(t *testing.T) {
	env, _ := threadEnv(t)
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Other", "email": "other@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-create-uid", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := env.Model("res.users").Create(map[string]any{"login": "other-create-uid", "name": "Other", "partner_id": otherPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	otherEnv := env.WithContext(record.Context{UserID: otherUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := otherEnv.Model("mail.message").Create(map[string]any{"subject": "Spoof", "body": "<p>Spoof</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Spoof",
		"body_html":       "<p>Spoof</p>",
		"state":           "outgoing",
	})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("create error = %v", err)
	}
}

func TestMailQueueCustomSenderStillRejectsUnauthorizedForcedPersonalServer(t *testing.T) {
	env, _ := threadEnv(t)
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-custom", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Denied", "body": "<p>Denied</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "owner@example.com",
		"email_to":        "recipient@example.com",
		"subject":         "Denied",
		"body_html":       "<p>Denied</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 || len(sender.sent) != 0 {
		t.Fatalf("queue result = %+v sent=%+v", result, sender.sent)
	}
}

func TestMailQueueRejectsForcedPersonalServerDomainFilter(t *testing.T) {
	env, _ := threadEnv(t)
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-domain", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Domain Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Domain", "body": "<p>Domain</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Domain",
		"body_html":       "<p>Domain</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 {
		t.Fatalf("queue result = %+v", result)
	}
}

func TestMailQueueAllowsAuthorOwnedForcedPersonalServer(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Personal",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Allowed", "body": "<p>Allowed</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Allowed",
		"body_html":       "<p>Allowed</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	if server.mailFrom != "owner@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
}

func TestMailQueueThrottlesPersonalMailServer(t *testing.T) {
	env, _ := threadEnv(t)
	server := newFakeSMTPServer(t, nil)
	defer server.Close()
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "mail.server.personal.limit.minutes",
		"value": "1",
	}); err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-throttle", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Current User Personal",
		"active":          true,
		"smtp_host":       server.host,
		"smtp_port":       int64(server.port),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Throttle", "body": "<p>Throttle</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "first@example.com",
		"subject":         "First",
		"body_html":       "<p>First</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "second@example.com",
		"subject":         "Second",
		"body_html":       "<p>Second</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 19, 10, 15, 30, 0, time.UTC)

	result, err := SendMails(context.Background(), env, nil, []int64{firstID, secondID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 2 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 1 {
		t.Fatalf("queue result = %+v", result)
	}
	server.wait(t)
	serverRows, err := env.Model("ir.mail_server").Browse(serverID).Read("owner_limit_time", "owner_limit_count")
	if err != nil {
		t.Fatal(err)
	}
	if len(serverRows) != 1 || serverRows[0]["owner_limit_count"] != int64(1) || !timeValue(serverRows[0]["owner_limit_time"]).Equal(now.Truncate(time.Minute)) {
		t.Fatalf("server rows = %+v", serverRows)
	}
	mailRows, err := env.Model("mail.mail").Browse(secondID).Read("state", "scheduled_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "outgoing" || !timeValue(mailRows[0]["scheduled_date"]).Equal(now.Truncate(time.Minute).Add(time.Minute)) {
		t.Fatalf("delayed mail row = %+v", mailRows)
	}
}

func TestMailQueueDelaysPersonalServerOverflowAcrossMinuteBucketsAndTriggersCron(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "mail.server.personal.limit.minutes",
		"value": "2",
	}); err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-buckets", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Bucket Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	createMailQueueCron(t, env)
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Buckets", "body": "<p>Buckets</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailIDs := []int64{}
	for _, to := range []string{"first@example.com", "second@example.com", "third@example.com", "fourth@example.com", "fifth@example.com"} {
		mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
			"mail_message_id": messageID,
			"mail_server_id":  serverID,
			"email_from":      "Owner <owner@example.com>",
			"email_to":        to,
			"subject":         "Buckets",
			"body_html":       "<p>Buckets</p>",
			"state":           "outgoing",
		})
		if err != nil {
			t.Fatal(err)
		}
		mailIDs = append(mailIDs, mailID)
	}
	now := time.Date(2026, 6, 19, 10, 45, 30, 0, time.UTC)

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, mailIDs, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 5 || result.Sent != 2 || result.Failed != 0 || result.Skipped != 3 || len(sender.sent) != 2 {
		t.Fatalf("queue result = %+v sent=%+v", result, sender.sent)
	}
	rows, err := env.Model("mail.mail").Browse(mailIDs...).Read("state", "scheduled_date")
	if err != nil {
		t.Fatal(err)
	}
	scheduledByID := map[int64]time.Time{}
	stateByID := map[int64]string{}
	for _, row := range rows {
		id := int64FromAny(row["id"])
		scheduledByID[id] = timeValue(row["scheduled_date"])
		stateByID[id] = stringAny(row["state"])
	}
	if stateByID[mailIDs[0]] != "sent" || stateByID[mailIDs[1]] != "sent" {
		t.Fatalf("sent states = %+v", stateByID)
	}
	firstBucket := now.Truncate(time.Minute).Add(time.Minute)
	secondBucket := firstBucket.Add(time.Minute)
	if !scheduledByID[mailIDs[2]].Equal(firstBucket) || !scheduledByID[mailIDs[3]].Equal(firstBucket) || !scheduledByID[mailIDs[4]].Equal(secondBucket) {
		t.Fatalf("scheduled buckets = %+v", scheduledByID)
	}
	assertMailQueueCronTrigger(t, env, firstBucket.Add(59*time.Second))
}

func TestMailQueueSplitOverflowUsesNextFutureBucketForFollowingDelay(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "mail.server.personal.limit.minutes",
		"value": "2",
	}); err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-split-buckets", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Split Bucket Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	createMailQueueCron(t, env)
	partners := []int64{}
	for _, spec := range []struct {
		name  string
		email string
	}{
		{"First", "first@example.com"},
		{"Second", "second@example.com"},
		{"Third", "third@example.com"},
		{"Fourth", "fourth@example.com"},
		{"Fifth", "fifth@example.com"},
	} {
		id, err := env.Model("res.partner").Create(map[string]any{"name": spec.name, "email": spec.email, "active": true})
		if err != nil {
			t.Fatal(err)
		}
		partners = append(partners, id)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Split Buckets", "body": "<p>Split Buckets</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	splitID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"subject":         "Split Buckets",
		"body_html":       "<p>Split Buckets</p>",
		"state":           "outgoing",
		"recipient_ids":   partners,
	})
	if err != nil {
		t.Fatal(err)
	}
	rawID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "raw@example.com",
		"subject":         "Raw",
		"body_html":       "<p>Raw</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 19, 11, 0, 15, 0, time.UTC)

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{splitID, rawID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 2 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 1 || len(sender.sent) != 2 {
		t.Fatalf("queue result = %+v sent=%+v", result, sender.sent)
	}
	rows, err := env.Model("mail.mail").Browse(splitID, rawID).Read("recipient_ids", "state", "scheduled_date")
	if err != nil {
		t.Fatal(err)
	}
	firstBucket := now.Truncate(time.Minute).Add(time.Minute)
	secondBucket := firstBucket.Add(time.Minute)
	for _, row := range rows {
		switch int64FromAny(row["id"]) {
		case splitID:
			if row["state"] != "outgoing" || !timeValue(row["scheduled_date"]).Equal(firstBucket) {
				t.Fatalf("split row = %+v", row)
			}
			if got := int64s(row["recipient_ids"]); len(got) != 3 || got[0] != partners[2] || got[1] != partners[3] || got[2] != partners[4] {
				t.Fatalf("split recipients = %+v", got)
			}
		case rawID:
			if row["state"] != "outgoing" || !timeValue(row["scheduled_date"]).Equal(secondBucket) {
				t.Fatalf("raw row = %+v", row)
			}
		}
	}
	assertMailQueueCronTrigger(t, env, firstBucket.Add(59*time.Second))
}

func TestMailQueueSplitsOverLimitPersonalMailRecipients(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "mail.server.personal.limit.minutes",
		"value": "2",
	}); err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner-split", "name": "Owner", "partner_id": ownerPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Owner Split Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(ownerUserID).Write(map[string]any{"outgoing_mail_server_id": serverID}); err != nil {
		t.Fatal(err)
	}
	partners := []int64{}
	for _, spec := range []struct {
		name  string
		email string
	}{
		{"First", "first@example.com"},
		{"Second", "second@example.com"},
		{"Third", "third@example.com"},
	} {
		id, err := env.Model("res.partner").Create(map[string]any{"name": spec.name, "email": spec.email, "active": true})
		if err != nil {
			t.Fatal(err)
		}
		partners = append(partners, id)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID, err := ownerEnv.Model("mail.message").Create(map[string]any{"subject": "Split", "body": "<p>Split</p>", "message_type": "email", "author_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "raw@example.com",
		"email_cc":        "copy@example.com",
		"subject":         "Split",
		"body_html":       "<p>Split</p>",
		"state":           "outgoing",
		"recipient_ids":   partners,
		"headers":         `{"X-Split":"yes"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	notificationIDs := map[string]int64{}
	for index, partnerID := range partners {
		notificationID, err := env.Model("mail.notification").Create(map[string]any{
			"mail_message_id":     messageID,
			"mail_mail_id":        mailID,
			"res_partner_id":      partnerID,
			"notification_type":   "email",
			"notification_status": "ready",
		})
		if err != nil {
			t.Fatal(err)
		}
		notificationIDs[[]string{"first", "second", "third"}[index]] = notificationID
	}
	rawNotificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"mail_email_address":  "raw@example.com",
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	traceIDs := map[string]int64{}
	for index, spec := range []struct {
		label string
		email string
	}{
		{"first", "first@example.com"},
		{"second", "second@example.com"},
		{"third", "third@example.com"},
	} {
		traceID, err := env.Model("mailing.trace").Create(map[string]any{
			"mail_mail_id": mailID,
			"email":        spec.email,
			"model":        "res.partner",
			"res_id":       partners[index],
		})
		if err != nil {
			t.Fatal(err)
		}
		traceIDs[spec.label] = traceID
	}
	rawTraceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id": mailID,
		"email":        "raw@example.com",
		"model":        "mail.message",
		"res_id":       messageID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceIDs["raw"] = rawTraceID
	now := time.Date(2026, 6, 19, 10, 30, 15, 0, time.UTC)

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	if len(sender.sent) != 3 || sender.sent[0].To != "raw@example.com" || sender.sent[0].CC != "copy@example.com" || sender.sent[1].To != "first@example.com" || sender.sent[2].To != "second@example.com" || sender.sent[0].Headers["X-Split"] != "yes" {
		t.Fatalf("sent messages = %+v", sender.sent)
	}
	serverRows, err := env.Model("ir.mail_server").Browse(serverID).Read("owner_limit_time", "owner_limit_count")
	if err != nil {
		t.Fatal(err)
	}
	if len(serverRows) != 1 || serverRows[0]["owner_limit_count"] != int64(2) || !timeValue(serverRows[0]["owner_limit_time"]).Equal(now.Truncate(time.Minute)) {
		t.Fatalf("server rows = %+v", serverRows)
	}
	originalRows, err := env.Model("mail.mail").Browse(mailID).Read("state", "recipient_ids", "email_to", "email_cc", "scheduled_date", "failure_type", "failure_reason", "message_id", "create_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(originalRows) != 1 || originalRows[0]["state"] != "outgoing" || originalRows[0]["email_to"] != "" || originalRows[0]["email_cc"] != "" || !timeValue(originalRows[0]["scheduled_date"]).Equal(now.Truncate(time.Minute).Add(time.Minute)) || originalRows[0]["failure_type"] != "" || originalRows[0]["failure_reason"] != "" || originalRows[0]["message_id"] != "" || originalRows[0]["create_uid"] != ownerUserID {
		t.Fatalf("original row = %+v", originalRows)
	}
	if got := int64s(originalRows[0]["recipient_ids"]); len(got) != 1 || got[0] != partners[2] {
		t.Fatalf("original recipients = %+v", got)
	}
	found, err := env.Model("mail.mail").Search(domain.Cond("state", "=", "sent"))
	if err != nil {
		t.Fatal(err)
	}
	sentRows, err := found.Read("recipient_ids", "email_to", "email_cc", "headers", "message_id", "create_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(sentRows) != 1 {
		t.Fatalf("sent rows = %+v", sentRows)
	}
	copyID := sentRows[0]["id"].(int64)
	if copyID == mailID || sentRows[0]["email_to"] != "raw@example.com" || sentRows[0]["email_cc"] != "copy@example.com" || sentRows[0]["headers"] != `{"X-Split":"yes"}` || sentRows[0]["message_id"] == "" || sentRows[0]["create_uid"] != ownerUserID {
		t.Fatalf("copy row = %+v", sentRows[0])
	}
	if got := int64s(sentRows[0]["recipient_ids"]); len(got) != 2 || got[0] != partners[0] || got[1] != partners[1] {
		t.Fatalf("copy recipients = %+v", got)
	}
	for _, spec := range []struct {
		id     int64
		mailID int64
		status string
		label  string
	}{
		{notificationIDs["first"], copyID, "sent", "first"},
		{notificationIDs["second"], copyID, "sent", "second"},
		{rawNotificationID, copyID, "sent", "raw"},
		{notificationIDs["third"], mailID, "ready", "third"},
	} {
		rows, err := env.Model("mail.notification").Browse(spec.id).Read("mail_mail_id", "notification_status", "failure_type", "failure_reason")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["mail_mail_id"] != spec.mailID || rows[0]["notification_status"] != spec.status {
			t.Fatalf("%s notification = %+v", spec.label, rows)
		}
		if spec.status == "sent" && (rows[0]["failure_type"] != "" || rows[0]["failure_reason"] != "") {
			t.Fatalf("%s notification failure = %+v", spec.label, rows)
		}
	}
	copyTraceSearch, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id", "=", copyID))
	if err != nil {
		t.Fatal(err)
	}
	copyTraceRows, err := copyTraceSearch.Read("mail_mail_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(copyTraceRows) != 0 {
		t.Fatalf("copy traces = %+v", copyTraceRows)
	}
	for _, spec := range []struct {
		label string
		id    int64
	}{
		{"first", traceIDs["first"]},
		{"second", traceIDs["second"]},
		{"third", traceIDs["third"]},
		{"raw", traceIDs["raw"]},
	} {
		rows, err := env.Model("mailing.trace").Browse(spec.id).Read("mail_mail_id", "mail_mail_id_int", "trace_status", "message_id", "sent_datetime", "failure_type")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || int64FromAny(rows[0]["mail_mail_id"]) != mailID || int64FromAny(rows[0]["mail_mail_id_int"]) != mailID || rows[0]["trace_status"] != "outgoing" || stringAny(rows[0]["message_id"]) != "" || !timeValue(rows[0]["sent_datetime"]).IsZero() || stringAny(rows[0]["failure_type"]) != "" {
			t.Fatalf("%s trace = %+v", spec.label, rows)
		}
	}
}

func TestMailQueueFailureRetryAndCancel(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Failure", "body": "<p>Failure</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "fail@example.com",
		"subject":         "Failure",
		"body_html":       "<p>Failure</p>",
		"state":           "outgoing",
		"scheduled_date":  now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id": mailID,
		"email":        "fail@example.com",
		"model":        "mail.message",
		"res_id":       messageID,
	})
	if err != nil {
		t.Fatal(err)
	}
	notificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, failingSender{err: errors.New("smtp password secret leaked")}, []int64{mailID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "exception" || rows[0]["failure_type"] != "mail_smtp" || rows[0]["failure_reason"] != "send failed" {
		t.Fatalf("failed mail row = %+v", rows)
	}
	if strings.Contains(rows[0]["failure_reason"].(string), "password") || strings.Contains(rows[0]["failure_reason"].(string), "secret") {
		t.Fatalf("unsafe failure reason = %+v", rows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "error" || traceRows[0]["failure_type"] != "mail_smtp" || stringAny(traceRows[0]["failure_reason"]) != "" || traceRows[0]["message_id"] == "" {
		t.Fatalf("trace row = %+v", traceRows)
	}
	notificationRows, err := env.Model("mail.notification").Browse(notificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != "exception" || notificationRows[0]["failure_type"] != "mail_smtp" || notificationRows[0]["failure_reason"] != "send failed" {
		t.Fatalf("notification row = %+v", notificationRows)
	}

	if err := RetryMails(env, []int64{mailID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "outgoing" || rows[0]["failure_type"] != "mail_smtp" || rows[0]["failure_reason"] != "send failed" {
		t.Fatalf("retried mail row = %+v", rows)
	}
	if err := CancelMails(env, []int64{mailID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mail.mail").Browse(mailID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "cancel" {
		t.Fatalf("cancelled mail row = %+v", rows)
	}
}

func TestMailQueueClassifiesMissingRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Default",
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Invalid", "body": "<p>Invalid</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "not an address",
		"subject":         "Invalid",
		"body_html":       "<p>Invalid</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "exception" || rows[0]["failure_type"] != "mail_email_missing" || rows[0]["failure_reason"] != "send failed" {
		t.Fatalf("missing mail row = %+v", rows)
	}
}

func TestMailQueueClassifiesInvalidRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Default",
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(1),
		"smtp_encryption": "none",
	}); err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Invalid", "body": "<p>Invalid</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "bad@",
		"subject":         "Invalid",
		"body_html":       "<p>Invalid</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMails(context.Background(), env, nil, []int64{mailID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Failed != 1 || result.Sent != 0 {
		t.Fatalf("queue result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "exception" || rows[0]["failure_type"] != "mail_email_invalid" || rows[0]["failure_reason"] != "send failed" {
		t.Fatalf("invalid mail row = %+v", rows)
	}
}

func TestSendMailsSkipsNonOutgoingRows(t *testing.T) {
	env, _ := threadEnv(t)
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Skip", "body": "<p>Skip</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "skip@example.com",
		"subject":         "Skip",
		"body_html":       "<p>Skip</p>",
		"state":           "sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 0 || result.Skipped != 1 || len(sender.sent) != 0 {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	found, err := env.Model("mail.mail").Search(domain.Cond("state", "=", "sent"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found.IDs()) == 0 {
		t.Fatal("sent row missing")
	}
}

func assertMailQueueCronTrigger(t *testing.T, env *record.Env, want time.Time) {
	t.Helper()
	found, err := env.Model("ir.model.data").Search(domain.Cond("complete_name", "=", "mail.ir_cron_mail_scheduler_action"))
	if err != nil {
		t.Fatal(err)
	}
	xmlRows, err := found.Read("model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(xmlRows) != 1 || xmlRows[0]["model"] != "ir.cron" {
		t.Fatalf("mail queue cron xml id = %+v", xmlRows)
	}
	cronID := int64FromAny(xmlRows[0]["res_id"])
	triggerRows, err := env.Model("ir.cron.trigger").Search(domain.Cond("cron_id", "=", cronID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := triggerRows.Read("call_at")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if timeValue(row["call_at"]).Equal(want) {
			return
		}
	}
	t.Fatalf("mail queue trigger for %s missing in %+v", want, rows)
}

func createMailQueueCron(t *testing.T, env *record.Env) int64 {
	t.Helper()
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Mail: Email Queue Manager",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		"failure_count":   int64(0),
		"priority":        int64(6),
		"state":           "code",
		"code":            "model.process_email_queue(batch_size=1000)",
		"action_name":     mailProcessQueueActionName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module": "mail",
		"name":   "ir_cron_mail_scheduler_action",
		"model":  "ir.cron",
		"res_id": cronID,
	}); err != nil {
		t.Fatal(err)
	}
	return cronID
}
