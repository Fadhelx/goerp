package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestProcessInboundEmailMarksBounceNotificationFromDeliveryStatus(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":         "example.com",
		"bounce_alias": "bounce",
	}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "email": "ada@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Bounce", "body": "<p>Bounce</p>", "message_type": "email", "model": "res.partner", "res_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "sender@example.com",
		"email_to":        "ada@example.com",
		"subject":         "Bounce",
		"body_html":       "<p>Bounce</p>",
		"state":           "outgoing",
		"recipient_ids":   []int64{partnerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	notificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"mail_mail_id":        mailID,
		"res_partner_id":      partnerID,
		"mail_email_address":  "ada@example.com",
		"notification_type":   "email",
		"notification_status": "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, []int64{mailID}, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || len(sender.sent) == 0 || sender.sent[0].Headers["Message-Id"] == "" {
		t.Fatalf("send result = %+v sent = %+v", result, sender.sent)
	}
	messageRows, err := env.Model("mail.message").Browse(messageID).Read("message_id")
	if err != nil {
		t.Fatal(err)
	}
	outboundMessageID := stringAny(messageRows[0]["message_id"])
	if outboundMessageID == "" || outboundMessageID != sender.sent[0].Headers["Message-Id"] {
		t.Fatalf("stored message id = %+v sent = %+v", messageRows, sender.sent[0].Headers)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"message_id": outboundMessageID,
		"email":      "ada@example.com",
		"model":      "res.partner",
		"res_id":     partnerID,
	})
	if err != nil {
		t.Fatal(err)
	}

	inbound := deliveryStatusMessage("bounce@example.com", "ada@example.com", outboundMessageID, true)
	processed, err := ProcessInboundEmail(env, []byte(inbound))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.IsBounce || processed.MessageID != messageID || processed.BouncedEmail != "ada@example.com" || len(processed.NotificationIDs) != 1 || processed.NotificationIDs[0] != notificationID {
		t.Fatalf("processed = %+v", processed)
	}
	notificationRows, err := env.Model("mail.notification").Browse(notificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != "bounce" || notificationRows[0]["failure_type"] != "mail_bounce" || !strings.Contains(notificationRows[0]["failure_reason"].(string), "550 No such user") {
		t.Fatalf("notification = %+v", notificationRows)
	}
	partnerRows, err := env.Model("res.partner").Browse(partnerID).Read("email_normalized", "message_bounce")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["email_normalized"] != "ada@example.com" || partnerRows[0]["message_bounce"] != int64(1) {
		t.Fatalf("partner = %+v", partnerRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "bounce" || traceRows[0]["failure_type"] != "mail_bounce" || !strings.Contains(stringAny(traceRows[0]["failure_reason"]), "550 No such user") {
		t.Fatalf("trace = %+v", traceRows)
	}
}

func TestProcessInboundEmailMarksMailingTraceOpenReplyFromReferences(t *testing.T) {
	env, _ := threadEnv(t)
	threadID, err := env.Model("gateway.thread").Create(map[string]any{"name": "Trace Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Mailing Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "gateway.thread",
		"res_id":       threadID,
		"message_id":   "<mailing-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"message_id":   "<mailing-parent@local>",
		"email":        "reply@example.com",
		"model":        "gateway.thread",
		"res_id":       threadID,
		"trace_status": "sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"Message-Id: <mailing-reply@remote>",
		"From: Reply <reply@example.com>",
		"To: catch@example.com",
		"Subject: Mailing Reply",
		"In-Reply-To: <mailing-parent@local>",
		"References: <mailing-parent@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Reply body",
		"",
	}, "\r\n")

	processed, err := ProcessInboundEmail(env, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.ResID != threadID || processed.ParentID != parentID {
		t.Fatalf("processed = %+v parent=%d", processed, parentID)
	}
	rows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "reply_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["trace_status"] != "reply" || timeValue(rows[0]["open_datetime"]).IsZero() || timeValue(rows[0]["reply_datetime"]).IsZero() {
		t.Fatalf("trace = %+v", rows)
	}
}

func TestProcessInboundEmailPropagatesMailingTraceUTMToNewRecord(t *testing.T) {
	env, _ := threadEnv(t)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Email"})
	if err != nil {
		t.Fatal(err)
	}
	overrideSourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Override"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":        "Mailing",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.trace").Create(map[string]any{
		"message_id":      "<utm-parent@local>",
		"email":           "recipient@example.com",
		"model":           "res.partner",
		"res_id":          partnerID,
		"mass_mailing_id": mailingID,
	}); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"Message-Id: <utm-reply@remote>",
		"From: Prospect <prospect@example.com>",
		"To: catch@example.com",
		"Subject: UTM Reply",
		"References: <utm-parent@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"UTM body",
		"",
	}, "\r\n")

	processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{
		FallbackModel: "gateway.thread",
		CustomValues:  map[string]any{"source_id": overrideSourceID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "gateway.thread" || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("gateway.thread").Browse(processed.ResID).Read("campaign_id", "source_id", "medium_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["campaign_id"] != campaignID || rows[0]["source_id"] != overrideSourceID || rows[0]["medium_id"] != mediumID {
		t.Fatalf("gateway UTM rows = %+v", rows)
	}
}

func TestProcessInboundEmailFallsBackToSingleNotificationPartner(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Grace", "email": "grace@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Fallback", "body": "<p>Fallback</p>", "message_type": "email", "message_id": "<fallback@local>"})
	if err != nil {
		t.Fatal(err)
	}
	notificationID, err := env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"res_partner_id":      partnerID,
		"notification_type":   "email",
		"notification_status": "sent",
	})
	if err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(deliveryStatusMessage("bounce@example.com", "", "<fallback@local>", false)))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.IsBounce || processed.MessageID != messageID || processed.BouncedEmail != "grace@example.com" || len(processed.BouncedPartners) != 1 || processed.BouncedPartners[0] != partnerID {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("mail.notification").Browse(notificationID).Read("notification_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["notification_status"] != "bounce" || rows[0]["failure_type"] != "mail_bounce" {
		t.Fatalf("notification = %+v", rows)
	}
}

func TestProcessInboundEmailResetsPartnerBounceOnNonBounce(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{
		"name":             "Linus",
		"email":            "linus@example.com",
		"email_normalized": "linus@example.com",
		"message_bounce":   int64(3),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmail(env, []byte("From: Linus <linus@example.com>\r\nTo: inbox@example.com\r\nSubject: Hello\r\n\r\nBody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if processed.IsBounce {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("res.partner").Browse(partnerID).Read("message_bounce")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["message_bounce"] != int64(0) {
		t.Fatalf("partner = %+v", rows)
	}
}

func TestProcessInboundEmailRoutesReplyAndSkipsDuplicateMessageID(t *testing.T) {
	env, ids := threadEnv(t)
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Sender", "email": "sender@example.com", "email_normalized": "sender@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Original",
		"body":         "<p>Original</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<parent@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw := inboundReplyMessage("<reply@remote>", "<parent@local>", "Sender <sender@example.com>", "Reply", "Plain reply")
	processed, err := ProcessInboundEmail(env, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Duplicate || processed.MessageID == 0 || processed.ResID != recordID || processed.ParentID != parentID || processed.AuthorID != authorID || processed.RFCMessageID != "<reply@remote>" {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "author_id", "email_from", "message_id", "body", "subtype_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != recordID || rows[0]["parent_id"] != parentID || rows[0]["author_id"] != authorID || rows[0]["email_from"] != "Sender <sender@example.com>" || rows[0]["message_id"] != "<reply@remote>" || rows[0]["subtype_id"] != ids["mail.mt_comment"].ResID {
		t.Fatalf("message row = %+v", rows)
	}
	if !strings.Contains(rows[0]["body"].(string), "<pre>Plain reply</pre>") {
		t.Fatalf("message body = %q", rows[0]["body"])
	}

	duplicate, err := ProcessInboundEmail(env, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.MessageID != processed.MessageID || duplicate.Routed {
		t.Fatalf("duplicate = %+v", duplicate)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<reply@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("duplicate message count = %d", found.Len())
	}
}

func TestProcessInboundEmailTreatsConcurrentMessageIDAsDuplicate(t *testing.T) {
	env, _ := threadEnv(t)
	locked := make(chan struct{})
	release := make(chan struct{})
	restore := setInboundAfterMessageIDLockHook(func(messageID string) {
		if messageID != "<concurrent@remote>" {
			return
		}
		select {
		case <-locked:
		default:
			close(locked)
			<-release
		}
	})
	defer restore()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	raw := inboundNewMessage("<concurrent@remote>", "Concurrent <concurrent@example.com>", "Concurrent", "<p>Concurrent</p>")

	type result struct {
		processed InboundProcessResult
		err       error
	}
	firstDone := make(chan result, 1)
	go func() {
		processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
		firstDone <- result{processed: processed, err: err}
	}()

	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("first inbound message did not acquire duplicate lock")
	}
	duplicate, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.Routed || duplicate.MessageID != 0 || duplicate.RFCMessageID != "<concurrent@remote>" {
		t.Fatalf("duplicate = %+v", duplicate)
	}
	close(release)
	var first result
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first inbound message did not finish")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if !first.processed.Routed || first.processed.Duplicate || first.processed.MessageID == 0 {
		t.Fatalf("first processed = %+v", first.processed)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<concurrent@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("concurrent message count = %d", found.Len())
	}
}

func TestProcessInboundEmailExternalMessageIDLockerBusyReturnsDuplicate(t *testing.T) {
	env, _ := threadEnv(t)
	called := 0
	raw := inboundNewMessage("<external-lock@remote>", "Locked <locked@example.com>", "Locked", "<p>Locked</p>")
	processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{
		FallbackModel: "res.partner",
		MessageIDLocker: InboundMessageIDLockFunc(func(messageID string) (func(), bool, error) {
			called++
			if messageID != "<external-lock@remote>" {
				t.Fatalf("messageID = %s", messageID)
			}
			return func() {}, false, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 || !processed.Duplicate || processed.Routed || processed.MessageID != 0 || processed.RFCMessageID != "<external-lock@remote>" {
		t.Fatalf("called=%d processed=%+v", called, processed)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<external-lock@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("locked duplicate should not create mail.message, count = %d", found.Len())
	}
}

func TestProcessInboundEmailUsesSharedInboundMessageIDLockAcrossWorkers(t *testing.T) {
	env, _ := threadEnv(t)
	otherWorker := env.WithSequenceNamespace("mail-worker-2")
	locked := make(chan struct{})
	release := make(chan struct{})
	restore := setInboundAfterMessageIDLockHook(func(messageID string) {
		if messageID != "<cross-worker@remote>" {
			return
		}
		select {
		case <-locked:
		default:
			close(locked)
			<-release
		}
	})
	defer restore()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	raw := inboundNewMessage("<cross-worker@remote>", "Concurrent <concurrent@example.com>", "Concurrent", "<p>Concurrent</p>")

	type result struct {
		processed InboundProcessResult
		err       error
	}
	firstDone := make(chan result, 1)
	go func() {
		processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
		firstDone <- result{processed: processed, err: err}
	}()

	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("first inbound message did not acquire duplicate lock")
	}
	lockRows, err := env.Model("mail.inbound.message.lock").Search(domain.Cond("message_id", "=", "<cross-worker@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if lockRows.Len() != 1 {
		t.Fatalf("expected held DB-backed message lock, got %d", lockRows.Len())
	}
	duplicate, err := ProcessInboundEmailWithOptions(otherWorker, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.Routed || duplicate.MessageID != 0 || duplicate.RFCMessageID != "<cross-worker@remote>" {
		t.Fatalf("duplicate = %+v", duplicate)
	}
	close(release)
	var first result
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first inbound message did not finish")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if !first.processed.Routed || first.processed.Duplicate || first.processed.MessageID == 0 {
		t.Fatalf("first processed = %+v", first.processed)
	}
	lockRows, err = env.Model("mail.inbound.message.lock").Search(domain.Cond("message_id", "=", "<cross-worker@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if lockRows.Len() != 0 {
		t.Fatalf("released DB-backed message lock count = %d", lockRows.Len())
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<cross-worker@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("cross-worker message count = %d", found.Len())
	}
}

func TestProcessInboundEmailTreatsConcurrentReplyMessageIDAsDuplicate(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Original",
		"body":         "<p>Original</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<concurrent-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	restore := setInboundAfterMessageIDLockHook(func(messageID string) {
		if messageID != "<concurrent-reply@remote>" {
			return
		}
		select {
		case <-locked:
		default:
			close(locked)
			<-release
		}
	})
	defer restore()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	raw := inboundReplyMessage("<concurrent-reply@remote>", "<concurrent-parent@local>", "Reply <reply@example.com>", "Concurrent reply", "Reply body")
	type result struct {
		processed InboundProcessResult
		err       error
	}
	firstDone := make(chan result, 1)
	go func() {
		processed, err := ProcessInboundEmail(env, []byte(raw))
		firstDone <- result{processed: processed, err: err}
	}()
	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("first reply did not acquire duplicate lock")
	}
	duplicate, err := ProcessInboundEmail(env, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.Routed || duplicate.MessageID != 0 {
		t.Fatalf("duplicate = %+v", duplicate)
	}
	close(release)
	var first result
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first reply did not finish")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if !first.processed.Routed || first.processed.ParentID != parentID || first.processed.ResID != recordID || first.processed.MessageID == 0 {
		t.Fatalf("first processed = %+v", first.processed)
	}
	later, err := ProcessInboundEmail(env, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !later.Duplicate || later.MessageID != first.processed.MessageID || later.Routed {
		t.Fatalf("later duplicate = %+v", later)
	}
}

func TestProcessInboundEmailStoresRawMessageIDForDuplicateKey(t *testing.T) {
	env, _ := threadEnv(t)
	bare, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("raw@example.com", "Bare <bare@example.com>", "Bare", "<p>Bare</p>")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	angle, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("<raw@example.com>", "Angle <angle@example.com>", "Angle", "<p>Angle</p>")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !bare.Routed || !angle.Routed || bare.MessageID == angle.MessageID {
		t.Fatalf("bare=%+v angle=%+v", bare, angle)
	}
	rows, err := env.Model("mail.message").Browse(bare.MessageID, angle.MessageID).Read("message_id")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[stringAny(row["message_id"])] = true
	}
	if !seen["raw@example.com"] || !seen["<raw@example.com>"] {
		t.Fatalf("message ids = %+v", rows)
	}
}

func TestProcessInboundEmailGeneratesUniqueFallbackMessageIDs(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	firstRaw := strings.Replace(inboundNewMessage("<missing-one@remote>", "Missing One <missing.one@example.com>", "Missing one", "<p>One</p>"), "Message-Id: <missing-one@remote>\r\n", "", 1)
	secondRaw := strings.Replace(inboundNewMessage("<missing-two@remote>", "Missing Two <missing.two@example.com>", "Missing two", "<p>Two</p>"), "Message-Id: <missing-two@remote>\r\n", "", 1)
	first, err := ProcessInboundEmailWithOptions(env, []byte(firstRaw), InboundProcessOptions{FallbackModel: "res.partner", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProcessInboundEmailWithOptions(env, []byte(secondRaw), InboundProcessOptions{FallbackModel: "res.partner", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Routed || !second.Routed || first.RFCMessageID == second.RFCMessageID {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	rows, err := env.Model("mail.message").Browse(first.MessageID, second.MessageID).Read("message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || stringAny(rows[0]["message_id"]) == "" || stringAny(rows[0]["message_id"]) == stringAny(rows[1]["message_id"]) {
		t.Fatalf("message rows = %+v", rows)
	}
}

func TestProcessInboundEmailIgnoresLoopDetectionBounceReply(t *testing.T) {
	env, _ := threadEnv(t)
	for _, tc := range []struct {
		name      string
		messageID string
		header    string
	}{
		{
			name:      "references",
			messageID: "<loop-references@remote>",
			header:    "References: <original@remote> <20260619-loop-detection-bounce-email@example.com>",
		},
		{
			name:      "in_reply_to",
			messageID: "<loop-reply@remote>",
			header:    "In-Reply-To: <20260619-loop-detection-bounce-email@example.com>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Join([]string{
				"Message-Id: " + tc.messageID,
				"From: Auto Reply <auto@example.com>",
				"To: catch@example.com",
				tc.header,
				"Subject: Loop reply",
				"Content-Type: text/plain; charset=utf-8",
				"",
				"Loop reply body",
				"",
			}, "\r\n")
			processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
			if err != nil {
				t.Fatal(err)
			}
			if !processed.LoopDetected || processed.Routed || processed.Duplicate || processed.RFCMessageID != tc.messageID {
				t.Fatalf("processed = %+v", processed)
			}
			found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", tc.messageID))
			if err != nil {
				t.Fatal(err)
			}
			if found.Len() != 0 {
				t.Fatalf("loop reply should not create mail.message, count = %d", found.Len())
			}
		})
	}
}

func TestProcessInboundEmailDetectsSenderLoopForNewRecord(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.gateway.loop.threshold", "value": "2"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Existing A", "Existing B"} {
		if _, err := env.Model("res.partner").Create(map[string]any{
			"name":             name,
			"email":            "auto@example.com",
			"email_normalized": "auto@example.com",
			"active":           true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	processed, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("<loop-new@remote>", "Auto <auto@example.com>", "Loop new", "<p>Loop</p>")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.LoopDetected || processed.Routed || processed.Model != "res.partner" || processed.ResID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<loop-new@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("looping inbound message count = %d", found.Len())
	}
	if count := loopBounceMailCount(t, env); count != 1 {
		t.Fatalf("loop bounce mail count = %d", count)
	}
}

func TestProcessInboundEmailGatewayAllowedBypassesSenderLoop(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.gateway.loop.threshold", "value": "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.partner").Create(map[string]any{
		"name":             "Existing",
		"email":            "allowed@example.com",
		"email_normalized": "allowed@example.com",
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.gateway.allowed").Create(map[string]any{"email": `"Allowed" <allowed@example.com>`}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("<allowed-loop@remote>", "Allowed <allowed@example.com>", "Allowed loop", "<p>Allowed</p>")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.LoopDetected || processed.MessageID == 0 || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	if count := loopBounceMailCount(t, env); count != 0 {
		t.Fatalf("loop bounce mail count = %d", count)
	}
}

func TestProcessInboundEmailDetectsSenderLoopForThreadReply(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.gateway.loop.threshold", "value": "2"}); err != nil {
		t.Fatal(err)
	}
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Auto", "email": "auto.reply@example.com", "email_normalized": "auto.reply@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<loop-parent@local>",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := env.Model("mail.message").Create(map[string]any{
			"subject":      "Previous",
			"body":         "<p>Previous</p>",
			"message_type": "email",
			"model":        "res.partner",
			"res_id":       recordID,
			"author_id":    authorID,
			"email_from":   "Auto <auto.reply@example.com>",
		}); err != nil {
			t.Fatal(err)
		}
	}

	processed, err := ProcessInboundEmail(env, []byte(inboundReplyMessage("<loop-thread@remote>", "<loop-parent@local>", "Auto <auto.reply@example.com>", "Loop reply", "Loop body")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.LoopDetected || processed.Routed || processed.ResID != recordID {
		t.Fatalf("processed = %+v", processed)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<loop-thread@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("looping inbound message count = %d", found.Len())
	}
}

func TestProcessInboundEmailDoesNotTreatAutoResponseHeadersAsLoop(t *testing.T) {
	env, _ := threadEnv(t)
	raw := strings.Join([]string{
		"Message-Id: <auto-response-headers@remote>",
		"From: Auto Headers <auto.headers@example.com>",
		"To: catch@example.com",
		"Auto-Submitted: auto-replied",
		"Precedence: bulk",
		"X-Auto-Response-Suppress: All",
		"X-Odoo-Objects: res.partner-99",
		"Subject: Auto headers",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Auto response body",
		"",
	}, "\r\n")
	processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.LoopDetected || processed.MessageID == 0 || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
}

func TestProcessInboundEmailCreatesFallbackThreadRecord(t *testing.T) {
	env, _ := threadEnv(t)
	processed, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("<new@remote>", "Customer <customer@example.com>", "New partner", "<p>Hello</p>")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID == 0 || processed.MessageID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	partnerRows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email", "email_normalized", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["name"] != "New partner" || partnerRows[0]["email"] != "customer@example.com" || partnerRows[0]["email_normalized"] != "customer@example.com" || partnerRows[0]["active"] != true {
		t.Fatalf("partner = %+v", partnerRows)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "message_id", "body")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["model"] != "res.partner" || messageRows[0]["res_id"] != processed.ResID || messageRows[0]["message_id"] != "<new@remote>" || !strings.Contains(messageRows[0]["body"].(string), "<p>Hello</p>") {
		t.Fatalf("message = %+v", messageRows)
	}
}

func TestProcessInboundEmailUsesGatewayUserForTargetAndRootForMessage(t *testing.T) {
	env, _ := threadEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "gateway.thread", "name": "Gateway Thread"})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Gateway User Partner", "email": "gateway.user@example.com", "email_normalized": "gateway.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "gateway-user", "name": "Gateway User", "partner_id": authorPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "gateway",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<gateway-user@remote>", "Gateway User <gateway.user@example.com>", "gateway@example.com", "Gateway User Creates")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "gateway.thread" || processed.ResID == 0 || processed.AuthorID != authorPartnerID {
		t.Fatalf("processed = %+v author=%d", processed, authorPartnerID)
	}
	targetRows, err := env.Model("gateway.thread").Browse(processed.ResID).Read("create_uid", "write_uid", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 || targetRows[0]["create_uid"] != authorUserID || targetRows[0]["write_uid"] != authorUserID || targetRows[0]["email"] != "gateway.user@example.com" {
		t.Fatalf("target rows = %+v user=%d", targetRows, authorUserID)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["create_uid"] != int64(1) ||
		messageRows[0]["write_uid"] != int64(1) ||
		messageRows[0]["author_id"] != authorPartnerID ||
		messageRows[0]["model"] != "gateway.thread" ||
		messageRows[0]["res_id"] != processed.ResID {
		t.Fatalf("message rows = %+v", messageRows)
	}
}

func TestProcessInboundEmailFallbackUsesCallerUserWhenSenderHasNoUser(t *testing.T) {
	env, _ := threadEnv(t)
	callerEnv := env.WithContext(record.Context{UserID: 77, CompanyID: 1, CompanyIDs: []int64{1}})
	processed, err := ProcessInboundEmailWithOptions(callerEnv, []byte(inboundNewMessage("<gateway-fallback@remote>", "No User <no.user@example.com>", "Fallback Gateway", "<p>Fallback</p>")), InboundProcessOptions{FallbackModel: "gateway.thread"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "gateway.thread" || processed.ResID == 0 || processed.AuthorID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	targetRows, err := env.Model("gateway.thread").Browse(processed.ResID).Read("create_uid", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 || targetRows[0]["create_uid"] != int64(77) || targetRows[0]["write_uid"] != int64(77) {
		t.Fatalf("target rows = %+v", targetRows)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("create_uid", "write_uid", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["create_uid"] != int64(1) || messageRows[0]["write_uid"] != int64(1) || messageRows[0]["author_id"] != int64(0) {
		t.Fatalf("message rows = %+v", messageRows)
	}
}

func TestProcessInboundEmailRunsMessageNewHandlerUnderGatewayUser(t *testing.T) {
	env, _ := threadEnv(t)
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Hook Sender", "email": "hook.sender@example.com", "email_normalized": "hook.sender@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "hook-sender", "name": "Hook Sender", "partner_id": authorPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	unregister := RegisterInboundMessageHandler("gateway.thread", InboundMessageHandler{
		MessageNew: func(hookEnv *record.Env, req InboundMessageNewRequest) (int64, error) {
			if hookEnv.Context().UserID != authorUserID || req.Message.AuthorID != authorPartnerID || req.Message.MessageID != "<gateway-message-new@remote>" || req.Message.IncomingEmailTo != "catch@example.com" {
				t.Fatalf("hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			return hookEnv.Model(req.Model).Create(map[string]any{
				"name":            req.Message.Subject + " handled",
				"email":           firstEmailAddress(req.Message.EmailFrom),
				"description":     stringAny(req.CustomValues["description"]) + "|" + req.Message.BodyHTML,
				"message_count":   int64(len(req.Message.Attachments)),
				"gateway_user_id": hookEnv.Context().UserID,
				"active":          true,
			})
		},
	})
	t.Cleanup(unregister)

	processed, err := ProcessInboundEmailWithOptions(env, []byte(inboundNewMessage("<gateway-message-new@remote>", "Hook Sender <hook.sender@example.com>", "Gateway Hook New", "<p>Hook body</p>")), InboundProcessOptions{
		FallbackModel: "gateway.thread",
		CustomValues:  map[string]any{"description": "custom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "gateway.thread" || processed.ResID == 0 || processed.AuthorID != authorPartnerID {
		t.Fatalf("processed = %+v", processed)
	}
	targetRows, err := env.Model("gateway.thread").Browse(processed.ResID).Read("name", "email", "description", "message_count", "gateway_user_id", "create_uid", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 ||
		targetRows[0]["name"] != "Gateway Hook New handled" ||
		targetRows[0]["email"] != "hook.sender@example.com" ||
		!strings.Contains(stringAny(targetRows[0]["description"]), "custom|<p>Hook body</p>") ||
		targetRows[0]["message_count"] != int64(0) ||
		targetRows[0]["gateway_user_id"] != authorUserID ||
		targetRows[0]["create_uid"] != authorUserID ||
		targetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("target rows = %+v user=%d", targetRows, authorUserID)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["create_uid"] != int64(1) ||
		messageRows[0]["write_uid"] != int64(1) ||
		messageRows[0]["author_id"] != authorPartnerID ||
		messageRows[0]["model"] != "gateway.thread" ||
		messageRows[0]["res_id"] != processed.ResID {
		t.Fatalf("message rows = %+v", messageRows)
	}
}

func TestProcessInboundEmailRunsMessageUpdateHandlerUnderGatewayUser(t *testing.T) {
	env, ids := threadEnv(t)
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Update Sender", "email": "update.sender@example.com", "email_normalized": "update.sender@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "update-sender", "name": "Update Sender", "partner_id": authorPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := env.Model("gateway.thread").Create(map[string]any{"name": "Existing Hook", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Gateway parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "gateway.thread",
		"res_id":       threadID,
		"message_id":   "<gateway-update-parent@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}
	unregister := RegisterInboundMessageHandler("gateway.thread", InboundMessageHandler{
		MessageUpdate: func(hookEnv *record.Env, req InboundMessageUpdateRequest) error {
			if hookEnv.Context().UserID != authorUserID || req.ResID != threadID || req.Message.ParentID != parentID || req.Message.MessageID != "<gateway-message-update@remote>" {
				t.Fatalf("hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			return hookEnv.Model(req.Model).Browse(req.ResID).Write(map[string]any{
				"description":     req.Message.Subject + "|" + req.Message.BodyHTML,
				"message_count":   int64(1),
				"gateway_user_id": hookEnv.Context().UserID,
			})
		},
	})
	t.Cleanup(unregister)

	processed, err := ProcessInboundEmail(env, []byte(inboundReplyMessage("<gateway-message-update@remote>", "<gateway-update-parent@local>", "Update Sender <update.sender@example.com>", "Gateway Hook Update", "Update body")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "gateway.thread" || processed.ResID != threadID || processed.ParentID != parentID || processed.AuthorID != authorPartnerID {
		t.Fatalf("processed = %+v", processed)
	}
	targetRows, err := env.Model("gateway.thread").Browse(threadID).Read("description", "message_count", "gateway_user_id", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 ||
		!strings.Contains(stringAny(targetRows[0]["description"]), "Gateway Hook Update|<pre>Update body</pre>") ||
		targetRows[0]["message_count"] != int64(1) ||
		targetRows[0]["gateway_user_id"] != authorUserID ||
		targetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("target rows = %+v user=%d", targetRows, authorUserID)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("create_uid", "write_uid", "author_id", "model", "res_id", "parent_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["create_uid"] != int64(1) ||
		messageRows[0]["write_uid"] != int64(1) ||
		messageRows[0]["author_id"] != authorPartnerID ||
		messageRows[0]["model"] != "gateway.thread" ||
		messageRows[0]["res_id"] != threadID ||
		messageRows[0]["parent_id"] != parentID {
		t.Fatalf("message rows = %+v", messageRows)
	}
}

func TestProcessInboundEmailRoutesAliasToNewThreadRecord(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "partners",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <alias@remote>",
		"From: Alias Sender <alias.sender@example.com>",
		"To: partners@example.com",
		"Subject: Alias partner",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Alias body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Alias partner" || rows[0]["email"] != "alias.sender@example.com" {
		t.Fatalf("alias-created partner = %+v", rows)
	}
}

func TestProcessInboundEmailRoutesAliasToForcedThread(t *testing.T) {
	env, _ := threadEnv(t)
	targetID, err := env.Model("res.partner").Create(map[string]any{"name": "Forced Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "support",
		"alias_domain_id":       aliasDomainID,
		"model_name":            "res.partner",
		"alias_force_thread_id": targetID,
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <alias-forced@remote>",
		"From: Forced Sender <forced.sender@example.com>",
		"To: support@example.com",
		"Subject: Should not create partner",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Forced body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID != targetID {
		t.Fatalf("processed = %+v target=%d", processed, targetID)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["model"] != "res.partner" || messageRows[0]["res_id"] != targetID || messageRows[0]["parent_id"] != int64(0) || messageRows[0]["message_id"] != "<alias-forced@remote>" {
		t.Fatalf("forced message = %+v", messageRows)
	}
	created, err := env.Model("res.partner").Search(domain.Cond("name", "=", "Should not create partner"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Len() != 0 {
		t.Fatalf("forced alias created new partner count = %d", created.Len())
	}
}

func TestProcessInboundEmailAliasDanglingForceThreadFallsBackToCreate(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "dangling",
		"alias_domain_id":       aliasDomainID,
		"model_name":            "res.partner",
		"alias_force_thread_id": int64(999999),
		"alias_defaults":        "{'name': 'Dangling Fallback'}",
		"alias_contact":         "everyone",
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-dangling-forced@remote>", "Dangling Sender <dangling.sender@example.com>", "dangling@example.com", "Dangling Forced")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID == 0 || processed.ResID == 999999 || processed.ParentID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email", "email_normalized")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Dangling Fallback" || rows[0]["email"] != "dangling.sender@example.com" || rows[0]["email_normalized"] != "dangling.sender@example.com" {
		t.Fatalf("fallback partner = %+v", rows)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "message_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["model"] != "res.partner" ||
		messageRows[0]["res_id"] != processed.ResID ||
		messageRows[0]["parent_id"] != int64(0) ||
		messageRows[0]["message_id"] != "<alias-dangling-forced@remote>" ||
		messageRows[0]["incoming_email_to"] != "" {
		t.Fatalf("fallback message = %+v", messageRows)
	}
	if bounceRows := aliasBounceMailRows(t, env, "<alias-dangling-forced@remote>"); len(bounceRows) != 0 {
		t.Fatalf("dangling forced alias bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailAppliesAliasDefaultsAndModelID(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	modelID := mailTestModelID(t, env, "res.partner")
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "defaults",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"alias_defaults":  "{'name': 'Alias Default', 'phone': '555-0100', 'active': False}",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmailWithOptions(env, []byte(strings.Join([]string{
		"Message-Id: <alias-defaults@remote>",
		"From: Defaults Sender <defaults.sender@example.com>",
		"To: defaults@example.com",
		"Subject: Header Name",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Defaults body",
		"",
	}, "\r\n")), InboundProcessOptions{FallbackModel: "mail.thread", CustomValues: map[string]any{"name": "Caller Default"}})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "phone", "email", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Alias Default" || rows[0]["phone"] != "555-0100" || rows[0]["email"] != "defaults.sender@example.com" || rows[0]["active"] != false {
		t.Fatalf("alias-default partner = %+v", rows)
	}
}

func TestProcessInboundEmailRoutesAliasByIncomingLocalPart(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":           "local",
		"alias_domain_id":      aliasDomainID,
		"model_name":           "res.partner",
		"alias_incoming_local": true,
		"active":               true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <alias-local@remote>",
		"From: Local Sender <local.sender@example.com>",
		"To: local@other.example",
		"Subject: Local alias",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Local body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
}

func TestProcessInboundEmailAliasIncomingLocalHonorsAllowedDomains(t *testing.T) {
	for _, tc := range []struct {
		name             string
		allowedDomains   string
		extraAliasDomain string
		to               string
		wantRouted       bool
	}{
		{name: "unrestricted", to: "local@outside.example", wantRouted: true},
		{name: "restricted denied", allowedDomains: "allowed.example", to: "local@outside.example", wantRouted: false},
		{name: "restricted allowed", allowedDomains: "allowed.example", to: "local@allowed.example", wantRouted: true},
		{name: "alias domain always allowed", allowedDomains: "allowed.example", extraAliasDomain: "secondary.example", to: "local@secondary.example", wantRouted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := threadEnv(t)
			aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.extraAliasDomain != "" {
				if _, err := env.Model("mail.alias.domain").Create(map[string]any{"name": tc.extraAliasDomain}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.allowedDomains != "" {
				if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": tc.allowedDomains}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := env.Model("mail.alias").Create(map[string]any{
				"alias_name":           "local",
				"alias_domain_id":      aliasDomainID,
				"model_name":           "res.partner",
				"alias_incoming_local": true,
				"active":               true,
			}); err != nil {
				t.Fatal(err)
			}

			messageID := "<alias-local-allowed-" + strings.ReplaceAll(tc.name, " ", "-") + "@remote>"
			processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
				"Message-Id: " + messageID,
				"From: Local Sender <local.sender@example.com>",
				"To: " + tc.to,
				"Subject: Local alias allowed",
				"Content-Type: text/plain; charset=utf-8",
				"",
				"Local allowed body",
				"",
			}, "\r\n")))
			if err != nil {
				t.Fatal(err)
			}
			if processed.Routed != tc.wantRouted {
				t.Fatalf("processed = %+v want routed %v", processed, tc.wantRouted)
			}
			foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", messageID))
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantRouted {
				if foundMessages.Len() != 0 {
					t.Fatalf("denied local alias created message count = %d", foundMessages.Len())
				}
				return
			}
			if processed.MessageID == 0 || foundMessages.Len() != 1 {
				t.Fatalf("routed local alias message id=%d count=%d", processed.MessageID, foundMessages.Len())
			}
			rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("incoming_email_to")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || strings.TrimSpace(stringAny(rows[0]["incoming_email_to"])) != "" {
				t.Fatalf("local alias metadata should be filtered, rows = %+v", rows)
			}
		})
	}
}

func TestProcessInboundEmailReplyToOtherModelAliasBypassesParent(t *testing.T) {
	env, ids := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "forward",
		"alias_domain_id": aliasDomainID,
		"model_name":      "portal.thread",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	parentRecordID, err := env.Model("res.partner").Create(map[string]any{"name": "Reply Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       parentRecordID,
		"message_id":   "<parent-forward@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmailWithOptions(env, []byte(strings.Join([]string{
		"Message-Id: <reply-forward@remote>",
		"From: Forward Sender <sender@example.net>",
		"To: forward@example.com",
		"Subject: Forward Alias",
		"In-Reply-To: <parent-forward@local>",
		"References: <parent-forward@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Forward body",
		"",
	}, "\r\n")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "portal.thread" || processed.ResID == 0 || processed.ParentID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != processed.ResID || rows[0]["parent_id"] != int64(0) || strings.TrimSpace(stringAny(rows[0]["incoming_email_to"])) != "" {
		t.Fatalf("forward message rows = %+v", rows)
	}
	parentRows, err := env.Model("mail.message").Browse(parentID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(parentRows) != 1 {
		t.Fatalf("parent message missing = %+v", parentRows)
	}
}

func TestProcessInboundEmailReplyToOtherModelAliasInCCKeepsParent(t *testing.T) {
	env, ids := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "forward",
		"alias_domain_id": aliasDomainID,
		"model_name":      "portal.thread",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	parentRecordID, err := env.Model("res.partner").Create(map[string]any{"name": "CC Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       parentRecordID,
		"message_id":   "<parent-forward-cc@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmailWithOptions(env, []byte(strings.Join([]string{
		"Message-Id: <reply-forward-cc@remote>",
		"From: Forward Sender <sender@example.net>",
		"To: parent@example.com",
		"Cc: forward@example.com",
		"Subject: Forward Alias CC",
		"In-Reply-To: <parent-forward-cc@local>",
		"References: <parent-forward-cc@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Forward body",
		"",
	}, "\r\n")), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "res.partner" || processed.ResID != parentRecordID || processed.ParentID != parentID {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "incoming_email_to", "incoming_email_cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != parentRecordID || rows[0]["parent_id"] != parentID || rows[0]["incoming_email_to"] != "parent@example.com" || strings.TrimSpace(stringAny(rows[0]["incoming_email_cc"])) != "" {
		t.Fatalf("cc reply rows = %+v", rows)
	}
}

func TestProcessInboundEmailReplySameModelAliasContactApplies(t *testing.T) {
	env, ids := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "restricted",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "partners",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	parentRecordID, err := env.Model("res.partner").Create(map[string]any{"name": "Restricted Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       parentRecordID,
		"message_id":   "<parent-restricted@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	}); err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <reply-restricted@remote>",
		"From: Unknown <unknown.restricted@example.net>",
		"To: restricted@example.com",
		"Subject: Restricted Reply",
		"In-Reply-To: <parent-restricted@local>",
		"References: <parent-restricted@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Restricted body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if processed.Routed || processed.MessageID != 0 || processed.Model != "res.partner" || processed.ResID != parentRecordID {
		t.Fatalf("processed = %+v", processed)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<reply-restricted@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("restricted reply message count = %d", found.Len())
	}
	bounceRows := aliasBounceMailRows(t, env, "<reply-restricted@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["email_to"] != "Unknown <unknown.restricted@example.net>" ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "registered partners") {
		t.Fatalf("restricted reply bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailMultipleAliasesCreatesAllRoutes(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "alpha",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_defaults":  "{'name': 'Alpha Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "portal",
		"alias_domain_id": aliasDomainID,
		"model_name":      "portal.thread",
		"alias_defaults":  "{'name': 'Portal Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <multi-alias@remote>",
		"From: Multi Sender <multi.sender@example.net>",
		"To: alpha@example.com, portal@example.com",
		"Subject: Multi Alias",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Multi body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.Model != "portal.thread" || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	alpha, err := env.Model("res.partner").Search(domain.Cond("name", "=", "Alpha Alias"))
	if err != nil {
		t.Fatal(err)
	}
	portal, err := env.Model("portal.thread").Search(domain.Cond("name", "=", "Portal Alias"))
	if err != nil {
		t.Fatal(err)
	}
	if alpha.Len() != 1 || portal.Len() != 1 {
		t.Fatalf("created alpha=%d portal=%d", alpha.Len(), portal.Len())
	}
	foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<multi-alias@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := foundMessages.Read("model", "res_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]int{}
	for _, row := range rows {
		models[stringAny(row["model"])]++
		if strings.TrimSpace(stringAny(row["incoming_email_to"])) != "" {
			t.Fatalf("alias metadata should be filtered, rows = %+v", rows)
		}
	}
	if len(rows) != 2 || models["res.partner"] != 1 || models["portal.thread"] != 1 {
		t.Fatalf("message rows = %+v", rows)
	}
}

func TestProcessInboundEmailAliasPartnersRequiresKnownAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "known",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "partners",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	rejected, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-partners-reject@remote>", "Unknown <unknown@example.com>", "known@example.com", "Rejected partners")))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Routed || rejected.MessageID != 0 || rejected.AuthorID != 0 {
		t.Fatalf("rejected = %+v", rejected)
	}
	rejectedMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<alias-partners-reject@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if rejectedMessages.Len() != 0 {
		t.Fatalf("rejected message count = %d", rejectedMessages.Len())
	}
	bounceRows := aliasBounceMailRows(t, env, "<alias-partners-reject@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["email_to"] != "Unknown <unknown@example.com>" ||
		bounceRows[0]["subject"] != "Re: Rejected partners" ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "registered partners") {
		t.Fatalf("partners bounce rows = %+v", bounceRows)
	}

	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Known Author", "email": "known.author@example.com", "email_normalized": "known.author@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-partners-accept@remote>", "Known <known.author@example.com>", "known@example.com", "Accepted partners")))
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Routed || accepted.MessageID == 0 || accepted.ResID == 0 || accepted.AuthorID != authorID {
		t.Fatalf("accepted = %+v author=%d", accepted, authorID)
	}
}

func TestProcessInboundEmailAliasFollowersRequiresTargetFollower(t *testing.T) {
	env, _ := threadEnv(t)
	targetID, err := env.Model("res.partner").Create(map[string]any{"name": "Follower Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Follower Author", "email": "follower.author@example.com", "email_normalized": "follower.author@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	strangerID, err := env.Model("res.partner").Create(map[string]any{"name": "Known Stranger", "email": "known.stranger@example.com", "email_normalized": "known.stranger@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := Subscribe(env, "res.partner", targetID, []int64{authorID}, nil); err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "followers",
		"alias_domain_id":       aliasDomainID,
		"model_name":            "res.partner",
		"alias_force_thread_id": targetID,
		"alias_contact":         "followers",
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}

	rejected, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-followers-reject@remote>", "Stranger <known.stranger@example.com>", "followers@example.com", "Rejected follower")))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Routed || rejected.MessageID != 0 || rejected.AuthorID != strangerID {
		t.Fatalf("rejected = %+v stranger=%d", rejected, strangerID)
	}
	accepted, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-followers-accept@remote>", "Follower <follower.author@example.com>", "followers@example.com", "Accepted follower")))
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Routed || accepted.MessageID == 0 || accepted.ResID != targetID || accepted.AuthorID != authorID {
		t.Fatalf("accepted = %+v author=%d target=%d", accepted, authorID, targetID)
	}
}

func TestProcessInboundEmailAliasFollowersUsesParentForNewThread(t *testing.T) {
	env, _ := threadEnv(t)
	parentID, err := env.Model("res.partner").Create(map[string]any{"name": "Alias Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Parent Follower", "email": "parent.follower@example.com", "email_normalized": "parent.follower@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := Subscribe(env, "res.partner", parentID, []int64{authorID}, nil); err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	modelID := mailTestModelID(t, env, "res.partner")
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "parent-followers",
		"alias_domain_id":        aliasDomainID,
		"alias_model_id":         modelID,
		"alias_contact":          "followers",
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": parentID,
		"active":                 true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-parent-followers@remote>", "Follower <parent.follower@example.com>", "parent-followers@example.com", "Parent follower creates")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.MessageID == 0 || processed.ResID == 0 || processed.ResID == parentID || processed.AuthorID != authorID {
		t.Fatalf("processed = %+v author=%d parent=%d", processed, authorID, parentID)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Parent follower creates" || rows[0]["email"] != "parent.follower@example.com" {
		t.Fatalf("created rows = %+v", rows)
	}
	aliasRows, err := env.Model("mail.alias").Search(domain.Cond("alias_name", "=", "parent-followers"))
	if err != nil {
		t.Fatal(err)
	}
	statusRows, err := aliasRows.Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(statusRows) != 1 || statusRows[0]["alias_status"] != "valid" {
		t.Fatalf("alias status rows = %+v", statusRows)
	}
}

func TestProcessInboundEmailAliasDanglingForceThreadUsesParentForFollowers(t *testing.T) {
	env, _ := threadEnv(t)
	parentID, err := env.Model("res.partner").Create(map[string]any{"name": "Dangling Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Dangling Parent Follower", "email": "dangling.parent@example.com", "email_normalized": "dangling.parent@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := Subscribe(env, "res.partner", parentID, []int64{authorID}, nil); err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	modelID := mailTestModelID(t, env, "res.partner")
	aliasID, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "dangling-parent-followers",
		"alias_domain_id":        aliasDomainID,
		"alias_model_id":         modelID,
		"alias_force_thread_id":  int64(999999),
		"alias_contact":          "followers",
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": parentID,
		"alias_status":           "not_tested",
		"active":                 true,
	})
	if err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-dangling-parent-followers@remote>", "Follower <dangling.parent@example.com>", "dangling-parent-followers@example.com", "Dangling parent follower creates")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.MessageID == 0 || processed.ResID == 0 || processed.ResID == 999999 || processed.ResID == parentID || processed.ParentID != 0 || processed.AuthorID != authorID {
		t.Fatalf("processed = %+v author=%d parent=%d", processed, authorID, parentID)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Dangling parent follower creates" || rows[0]["email"] != "dangling.parent@example.com" {
		t.Fatalf("created rows = %+v", rows)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["model"] != "res.partner" ||
		messageRows[0]["res_id"] != processed.ResID ||
		messageRows[0]["parent_id"] != int64(0) ||
		messageRows[0]["message_id"] != "<alias-dangling-parent-followers@remote>" {
		t.Fatalf("message rows = %+v", messageRows)
	}
	aliasRows, err := env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "valid" {
		t.Fatalf("alias rows = %+v", aliasRows)
	}
	if bounceRows := aliasBounceMailRows(t, env, "<alias-dangling-parent-followers@remote>"); len(bounceRows) != 0 {
		t.Fatalf("dangling parent followers bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailAliasFollowersConfigErrorMarksInvalidAndBounces(t *testing.T) {
	env, _ := threadEnv(t)
	authorID, err := env.Model("res.partner").Create(map[string]any{"name": "Known Follower", "email": "known.follower@example.com", "email_normalized": "known.follower@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "broken-followers",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "followers",
		"alias_status":    "not_tested",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}

	rejected, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-followers-config@remote>", "Known <known.follower@example.com>", "broken-followers@example.com", "Broken followers")))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Routed || rejected.MessageID != 0 || rejected.ResID != 0 || rejected.AuthorID != authorID {
		t.Fatalf("rejected = %+v author=%d", rejected, authorID)
	}
	aliasRows, err := env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "invalid" {
		t.Fatalf("alias rows = %+v", aliasRows)
	}
	bounceRows := aliasBounceMailRows(t, env, "<alias-followers-config@remote>")
	if len(bounceRows) != 1 || !strings.Contains(stringAny(bounceRows[0]["body_html"]), "Please try again later") {
		t.Fatalf("config bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailAliasContactSkipsDeniedAliasForAllowedRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	modelID := mailTestModelID(t, env, "res.partner")
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "denied",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"alias_defaults":  "{'name': 'Denied Alias'}",
		"alias_contact":   "partners",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "allowed",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"alias_defaults":  "{'name': 'Allowed Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-contact-skip@remote>", "Unknown <unknown.alias@example.com>", "denied@example.com, allowed@example.com", "Alias contact skip")))
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.MessageID == 0 || processed.ResID == 0 || processed.AuthorID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	rows, err := env.Model("res.partner").Browse(processed.ResID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Allowed Alias" || rows[0]["email"] != "unknown.alias@example.com" {
		t.Fatalf("created rows = %+v", rows)
	}
	deniedRows, err := env.Model("res.partner").Search(domain.Cond("name", "=", "Denied Alias"))
	if err != nil {
		t.Fatal(err)
	}
	if deniedRows.Len() != 0 {
		t.Fatalf("denied alias created partner count = %d", deniedRows.Len())
	}
	if bounceRows := aliasBounceMailRows(t, env, "<alias-contact-skip@remote>"); len(bounceRows) != 0 {
		t.Fatalf("skipped denied alias created bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailAliasContactRejectsUnknownPolicy(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "invalid-policy",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "invalid-policy",
		"alias_status":    "not_tested",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}

	rejected, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-contact-invalid@remote>", "Known <known.invalid@example.com>", "invalid-policy@example.com", "Invalid alias policy")))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Routed || rejected.MessageID != 0 || rejected.ResID != 0 {
		t.Fatalf("rejected = %+v", rejected)
	}
	foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<alias-contact-invalid@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if foundMessages.Len() != 0 {
		t.Fatalf("invalid policy message count = %d", foundMessages.Len())
	}
	foundPartners, err := env.Model("res.partner").Search(domain.Cond("name", "=", "Invalid alias policy"))
	if err != nil {
		t.Fatal(err)
	}
	if foundPartners.Len() != 0 {
		t.Fatalf("invalid policy partner count = %d", foundPartners.Len())
	}
	aliasRows, err := env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "invalid" {
		t.Fatalf("invalid policy alias rows = %+v", aliasRows)
	}
	bounceRows := aliasBounceMailRows(t, env, "<alias-contact-invalid@remote>")
	if len(bounceRows) != 1 || bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` || !strings.Contains(stringAny(bounceRows[0]["body_html"]), "Please try again later") {
		t.Fatalf("invalid policy bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailAliasBounceUsesCustomContent(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "custom-bounce",
		"alias_domain_id":        aliasDomainID,
		"model_name":             "res.partner",
		"alias_contact":          "partners",
		"alias_bounced_content":  "<p>Custom denied message</p>",
		"alias_incoming_local":   true,
		"alias_force_thread_id":  int64(0),
		"alias_parent_thread_id": int64(0),
		"active":                 true,
	}); err != nil {
		t.Fatal(err)
	}

	rejected, err := ProcessInboundEmail(env, []byte(aliasInboundMessage("<alias-custom-bounce@remote>", "Unknown <unknown.custom@example.com>", "custom-bounce@other.example", "Custom bounce")))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Routed || rejected.MessageID != 0 || rejected.ResID != 0 {
		t.Fatalf("rejected = %+v", rejected)
	}
	bounceRows := aliasBounceMailRows(t, env, "<alias-custom-bounce@remote>")
	if len(bounceRows) != 1 ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "Custom denied message") ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "Custom bounce body") {
		t.Fatalf("custom bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailBouncesDirectCatchall(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_email":   "bounce@example.com",
		"catchall_alias": "catchall",
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmailWithOptions(env, []byte(aliasInboundMessage("<catchall-direct@remote>", "Customer <customer@example.com>", `"My Super Catchall" <catchall@example.com>`, "Should Bounce")), InboundProcessOptions{FallbackModel: "res.partner", Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if processed.Routed || processed.MessageID != 0 || processed.ResID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<catchall-direct@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if foundMessages.Len() != 0 {
		t.Fatalf("direct catchall message count = %d", foundMessages.Len())
	}
	bounceRows := catchallBounceMailRows(t, env, "<catchall-direct@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["email_to"] != "Customer <customer@example.com>" ||
		bounceRows[0]["subject"] != "Re: Should Bounce" ||
		!strings.Contains(stringAny(bounceRows[0]["references"]), "loop-detection-bounce-email") ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "cannot be processed") ||
		!strings.Contains(stringAny(bounceRows[0]["body_html"]), "Should Bounce body") {
		t.Fatalf("direct catchall bounce rows = %+v", bounceRows)
	}
}

func TestProcessInboundEmailRoutesAliasWhenCatchallAlsoRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_email":   "bounce@example.com",
		"catchall_alias": "catchall",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "partners",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		messageID string
		to        string
	}{
		{name: "catchall first", messageID: "<catchall-alias-first@remote>", to: "catchall@example.com, partners@example.com"},
		{name: "catchall second", messageID: "<catchall-alias-second@remote>", to: "partners@example.com, catchall@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processed, err := ProcessInboundEmail(env, []byte(aliasInboundMessage(tc.messageID, "Alias Sender <alias.sender@example.com>", tc.to, "Catchall Not Blocking")))
			if err != nil {
				t.Fatal(err)
			}
			if !processed.Routed || processed.Model != "res.partner" || processed.MessageID == 0 || processed.ResID == 0 {
				t.Fatalf("processed = %+v", processed)
			}
			if bounceRows := catchallBounceMailRows(t, env, tc.messageID); len(bounceRows) != 0 {
				t.Fatalf("alias route created catchall bounce rows = %+v", bounceRows)
			}
		})
	}
}

func TestProcessInboundEmailBouncesCatchallWithUnroutableRecipient(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_email":   "bounce@example.com",
		"catchall_alias": "catchall",
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessInboundEmailWithOptions(env, []byte(aliasInboundMessage("<catchall-unroutable@remote>", "Customer <customer@example.com>", `"My Super Catchall" <catchall@example.com>, Unroutable <unroutable@example.com>`, "Should Bounce")), InboundProcessOptions{Now: time.Date(2026, 6, 19, 10, 5, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if processed.Routed || processed.MessageID != 0 || processed.ResID != 0 {
		t.Fatalf("processed = %+v", processed)
	}
	foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<catchall-unroutable@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if foundMessages.Len() != 0 {
		t.Fatalf("catchall unroutable message count = %d", foundMessages.Len())
	}
	bounceRows := catchallBounceMailRows(t, env, "<catchall-unroutable@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["subject"] != "Re: Should Bounce" ||
		!strings.Contains(stringAny(bounceRows[0]["references"]), "loop-detection-bounce-email") {
		t.Fatalf("catchall unroutable bounce rows = %+v", bounceRows)
	}
}

func mailTestModelID(t *testing.T, env *record.Env, modelName string) int64 {
	t.Helper()
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, modelName), record.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("model %s not found", modelName)
	}
	return int64FromAny(rows[0]["id"])
}

func aliasInboundMessage(messageID string, from string, to string, subject string) string {
	return strings.Join([]string{
		"Message-Id: " + messageID,
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Content-Type: text/plain; charset=utf-8",
		"",
		subject + " body",
		"",
	}, "\r\n")
}

func aliasBounceMailRows(t *testing.T, env *record.Env, rfcMessageID string) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.Cond("references", "=", rfcMessageID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("email_from", "email_to", "subject", "body_html", "state", "auto_delete", "references", "is_notification")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func catchallBounceMailRows(t *testing.T, env *record.Env, rfcMessageID string) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.Cond("references", domain.Like, rfcMessageID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("email_from", "email_to", "subject", "body_html", "state", "auto_delete", "references", "is_notification", "message_id", "record_alias_domain_id", "reply_to")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestProcessInboundEmailCreatesAttachmentsAndRewritesCID(t *testing.T) {
	env, _ := threadEnv(t)
	raw := inboundMultipartMessage("<attach@remote>", "Attached partner", true)
	processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if !processed.Routed || processed.MessageID == 0 || processed.ResID == 0 {
		t.Fatalf("processed = %+v", processed)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("body", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	attachmentIDs := messageRows[0]["attachment_ids"].([]int64)
	if len(attachmentIDs) != 2 {
		t.Fatalf("attachment ids = %#v", attachmentIDs)
	}
	body := messageRows[0]["body"].(string)
	if strings.Contains(body, "cid:logo123") || !strings.Contains(body, "/web/image/") || !strings.Contains(body, "access_token=") {
		t.Fatalf("body = %s", body)
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(attachmentIDs...).Read("name", "res_model", "res_id", "mimetype", "datas", "file_size", "access_token")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]any{}
	for _, row := range attachmentRows {
		seen[row["name"].(string)] = row
		if row["res_model"] != "res.partner" || row["res_id"] != processed.ResID {
			t.Fatalf("attachment owner = %+v", row)
		}
	}
	if string(bytesAny(seen["brief.pdf"]["datas"])) != "%PDF-1.4\n" || seen["brief.pdf"]["mimetype"] != "application/pdf" || int64FromAny(seen["brief.pdf"]["file_size"]) != int64(len("%PDF-1.4\n")) {
		t.Fatalf("brief attachment = %+v", seen["brief.pdf"])
	}
	if string(bytesAny(seen["logo.png"]["datas"])) != "PNG" || seen["logo.png"]["mimetype"] != "image/png" || strings.TrimSpace(stringAny(seen["logo.png"]["access_token"])) == "" {
		t.Fatalf("logo attachment = %+v", seen["logo.png"])
	}
}

func TestProcessInboundEmailSaveOriginalAndStripAttachments(t *testing.T) {
	env, _ := threadEnv(t)
	processed, err := ProcessInboundEmailWithOptions(env, []byte(inboundMultipartMessage("<original@remote>", "Keep original", false)), InboundProcessOptions{
		FallbackModel: "res.partner",
		SaveOriginal:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := env.Model("mail.message").Browse(processed.MessageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	attachmentIDs := messageRows[0]["attachment_ids"].([]int64)
	if len(attachmentIDs) != 2 {
		t.Fatalf("save original attachment ids = %#v", attachmentIDs)
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(attachmentIDs...).Read("name", "mimetype", "datas")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]map[string]any{}
	for _, row := range attachmentRows {
		names[row["name"].(string)] = row
	}
	if names["original_email.eml"] == nil || names["original_email.eml"]["mimetype"] != "message/rfc822" || !strings.Contains(string(bytesAny(names["original_email.eml"]["datas"])), "Message-Id: <original@remote>") {
		t.Fatalf("original attachment = %+v", names["original_email.eml"])
	}
	if names["brief.pdf"] == nil {
		t.Fatalf("attachments = %+v", attachmentRows)
	}

	stripped, err := ProcessInboundEmailWithOptions(env, []byte(inboundMultipartMessage("<strip@remote>", "Strip", false)), InboundProcessOptions{
		FallbackModel:    "res.partner",
		SaveOriginal:     true,
		StripAttachments: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	strippedRows, err := env.Model("mail.message").Browse(stripped.MessageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := strippedRows[0]["attachment_ids"].([]int64); len(got) != 0 {
		t.Fatalf("stripped attachment ids = %#v", got)
	}
}

func TestProcessInboundEmailStoresIncomingHeaderMetadata(t *testing.T) {
	env, _ := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "catch",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"Message-Id: <metadata@remote>",
		"From: Sender <sender@example.com>",
		"To: Catch <catch@example.com>, Other <other@example.com>",
		"Cc: Copy <copy@example.com>",
		"Reply-To: Help <help@example.com>",
		"Subject: Metadata",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Metadata body",
		"",
	}, "\r\n")
	processed, err := ProcessInboundEmailWithOptions(env, []byte(raw), InboundProcessOptions{FallbackModel: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("incoming_email_to", "incoming_email_cc", "reply_to", "email_from", "email_add_signature")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["incoming_email_to"] != `"Other" <other@example.com>` ||
		rows[0]["incoming_email_cc"] != `"Copy" <copy@example.com>` ||
		rows[0]["reply_to"] != "Sender <sender@example.com>" ||
		rows[0]["email_from"] != "Sender <sender@example.com>" ||
		rows[0]["email_add_signature"] != true {
		t.Fatalf("message rows = %+v", rows)
	}
}

func TestProcessInboundEmailReplyFiltersAliasDomainMetadata(t *testing.T) {
	env, ids := threadEnv(t)
	if _, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_alias":   "bounce",
		"catchall_alias": "catchall",
		"default_from":   "notifications",
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<parent-filter@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <reply-filter@remote>",
		"From: Sender <sender@example.net>",
		`To: "Catchall" <catchall@example.com>, "Other" <other@example.com>, notifications@example.com`,
		`Cc: "Bounce" <bounce@example.com>, "Copy" <copy@example.com>`,
		"Subject: Re: Parent",
		"In-Reply-To: <parent-filter@local>",
		"References: <parent-filter@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Reply body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "incoming_email_to", "incoming_email_cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["model"] != "res.partner" ||
		rows[0]["res_id"] != recordID ||
		rows[0]["parent_id"] != parentID ||
		rows[0]["incoming_email_to"] != `"Other" <other@example.com>` ||
		rows[0]["incoming_email_cc"] != `"Copy" <copy@example.com>` {
		t.Fatalf("reply metadata rows = %+v", rows)
	}
}

func TestProcessInboundEmailReplyFiltersAllowedLocalAliasMetadata(t *testing.T) {
	env, ids := threadEnv(t)
	aliasDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": "allowed.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":           "sales",
		"alias_domain_id":      aliasDomainID,
		"model_name":           "res.partner",
		"alias_incoming_local": true,
		"active":               true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Local Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Local Parent",
		"body":         "<p>Local Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<parent-local-filter@local>",
		"subtype_id":   ids["mail.mt_comment"].ResID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ProcessInboundEmail(env, []byte(strings.Join([]string{
		"Message-Id: <reply-local-filter@remote>",
		"From: Sender <sender@example.net>",
		`To: "Allowed" <sales@allowed.example>, "Blocked" <sales@blocked.example>, "Other" <other@example.com>`,
		"Subject: Re: Local Parent",
		"In-Reply-To: <parent-local-filter@local>",
		"References: <parent-local-filter@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Reply body",
		"",
	}, "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(processed.MessageID).Read("model", "res_id", "parent_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["model"] != "res.partner" ||
		rows[0]["res_id"] != recordID ||
		rows[0]["parent_id"] != parentID ||
		rows[0]["incoming_email_to"] != `"Blocked" <sales@blocked.example>,"Other" <other@example.com>` {
		t.Fatalf("local reply metadata rows = %+v", rows)
	}
}

func deliveryStatusMessage(to string, finalRecipient string, originalMessageID string, includeFinalRecipient bool) string {
	dsn := "Reporting-MTA: dns; mx.example.net\r\n"
	if includeFinalRecipient {
		dsn += "Final-Recipient: rfc822; " + finalRecipient + "\r\n"
	}
	dsn += "Action: failed\r\nStatus: 5.1.1\r\nDiagnostic-Code: smtp; 550 No such user\r\n"
	return strings.Join([]string{
		"From: Mailer-Daemon <mailer-daemon@example.net>",
		"To: " + to,
		"Subject: Delivery Status Notification",
		"MIME-Version: 1.0",
		`Content-Type: multipart/report; report-type=delivery-status; boundary="bounce-boundary"`,
		"",
		"--bounce-boundary",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Delivery failed",
		"",
		"--bounce-boundary",
		"Content-Type: message/delivery-status",
		"",
		dsn,
		"--bounce-boundary",
		"Content-Type: message/rfc822",
		"",
		"Message-Id: " + originalMessageID,
		"From: sender@example.com",
		"To: " + firstNonEmpty(finalRecipient, "recipient@example.com"),
		"Subject: Original",
		"",
		"Original body",
		"--bounce-boundary--",
		"",
	}, "\r\n")
}

func inboundReplyMessage(messageID string, inReplyTo string, from string, subject string, body string) string {
	return strings.Join([]string{
		"Message-Id: " + messageID,
		"From: " + from,
		"To: thread@example.com",
		"Subject: " + subject,
		"In-Reply-To: " + inReplyTo,
		"References: " + inReplyTo,
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
		"",
	}, "\r\n")
}

func inboundNewMessage(messageID string, from string, subject string, htmlBody string) string {
	return strings.Join([]string{
		"Message-Id: " + messageID,
		"From: " + from,
		"To: catch@example.com",
		"Subject: " + subject,
		"Content-Type: text/html; charset=utf-8",
		"",
		htmlBody,
		"",
	}, "\r\n")
}

func inboundMultipartMessage(messageID string, subject string, includeCID bool) string {
	lines := []string{
		"Message-Id: " + messageID,
		"From: Attach Sender <attach.sender@example.com>",
		"To: catch@example.com",
		"Subject: " + subject,
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="inbound-mixed"`,
		"",
		"--inbound-mixed",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Hello",
	}
	if includeCID {
		lines[len(lines)-1] += `<img src="cid:logo123">`
	}
	lines[len(lines)-1] += "</p>"
	if includeCID {
		lines = append(lines,
			"--inbound-mixed",
			`Content-Type: image/png; name="logo.png"`,
			`Content-Disposition: inline; filename="logo.png"`,
			"Content-ID: <logo123>",
			"Content-Transfer-Encoding: base64",
			"",
			"UE5H",
		)
	}
	lines = append(lines,
		"--inbound-mixed",
		`Content-Type: application/pdf; name="brief.pdf"`,
		`Content-Disposition: attachment; filename="brief.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		"JVBERi0xLjQK",
		"--inbound-mixed--",
		"",
	)
	return strings.Join(lines, "\r\n")
}

func loopBounceMailCount(t *testing.T, env *record.Env) int {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("references")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range rows {
		if strings.Contains(stringAny(row["references"]), "loop-detection-bounce-email") {
			count++
		}
	}
	return count
}

func setInboundAfterMessageIDLockHook(hook func(string)) func() {
	previous := inboundAfterMessageIDLockHook
	inboundAfterMessageIDLockHook = hook
	return func() {
		inboundAfterMessageIDLockHook = previous
	}
}
