package mail

import (
	"strings"
	"testing"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestSendMassMailingsSelectsListContactsAndMarksDone(t *testing.T) {
	env, _ := threadEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Customers", "active": true, "is_public": true})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("mailing.contact").Create(map[string]any{"name": "First", "email": "first@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Second", "email": "second@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, contactID := range []int64{firstID, secondID} {
		if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Customer Mailing",
		"subject":                 "Hello {{ name }}",
		"body_html":               "<p>{{ email }}</p>",
		"email_from":              "Sender <sender@example.com>",
		"mailing_model_real":      "mailing.contact",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
		"state":                   "draft",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || result.Skipped != 0 || len(result.MailingIDs) != 1 || result.MailingIDs[0] != mailingID || len(result.MailIDs) != 2 {
		t.Fatalf("result = %+v", result)
	}
	mailRows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_from", "email_to", "subject", "body_html", "mailing_id", "state")
	if err != nil {
		t.Fatal(err)
	}
	seenMail := map[string]bool{}
	for _, row := range mailRows {
		if row["email_from"] != "Sender <sender@example.com>" || row["mailing_id"] != mailingID || row["state"] != "outgoing" {
			t.Fatalf("mail row = %+v", row)
		}
		seenMail[stringAny(row["email_to"])] = true
	}
	if !seenMail["first@example.com"] || !seenMail["second@example.com"] {
		t.Fatalf("mail recipients = %+v rows=%+v", seenMail, mailRows)
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 2 {
		t.Fatalf("trace rows = %+v", traceRows)
	}
	for _, row := range traceRows {
		if row["model"] != "mailing.contact" || row["trace_status"] != "outgoing" || stringAny(row["failure_type"]) != "" {
			t.Fatalf("trace row = %+v", row)
		}
	}
	mailingRows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date", "kpi_mail_required")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailingRows) != 1 || mailingRows[0]["state"] != "done" || !timeValue(mailingRows[0]["sent_date"]).Equal(now) || mailingRows[0]["kpi_mail_required"] != true {
		t.Fatalf("mailing row = %+v", mailingRows)
	}
}

func TestSendMassMailingsPrependsPreviewText(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Preview Partner", "email": "preview@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Preview Mailing",
		"subject":            "Preview {{ name }}",
		"preview":            "Open {{ name }} & win",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "[('id', '=', " + stringAny(partnerID) + ")]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	body := stringAny(rows[0]["body_html"])
	if !strings.Contains(body, `display:none;font-size:1px;height:0px;width:0px;opacity:0;`) ||
		!strings.Contains(body, "Open Preview Partner &amp; win") ||
		!strings.Contains(body, "<p>preview@example.com</p>") {
		t.Fatalf("body_html = %s", body)
	}
}

func TestSendMassMailingTestsCreatesOutgoingSample(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("res.partner").Create(map[string]any{"name": "Sample Partner", "email": "sample.partner@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Sample Mailing",
		"subject":            "Sample {{ name }}",
		"preview":            "Preview {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"email_from":         "Sender <sender@example.com>",
		"mailing_model_real": "res.partner",
		"state":              "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mailing.mailing.test").Create(map[string]any{
		"email_to":        "valid@example.com\nnot-an-email",
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailingTests(env, []int64{wizardID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MailIDs) != 1 || len(result.Invalid) != 1 || result.Invalid[0] != "not-an-email" {
		t.Fatalf("test result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_to", "subject", "body_html", "mailing_id", "state", "auto_delete")
	if err != nil {
		t.Fatal(err)
	}
	body := stringAny(rows[0]["body_html"])
	if len(rows) != 1 || rows[0]["email_to"] != "valid@example.com" || rows[0]["subject"] != "[TEST] Sample Sample Partner" || rows[0]["mailing_id"] != mailingID || rows[0]["state"] != "outgoing" || rows[0]["auto_delete"] != false || !strings.Contains(body, `class="o_mail_wrapper"`) || !strings.Contains(body, "Preview Sample Partner") {
		t.Fatalf("test mail rows = %+v", rows)
	}
	mailingRows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date", "kpi_mail_required")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailingRows) != 1 || mailingRows[0]["state"] != "draft" || !timeValue(mailingRows[0]["sent_date"]).IsZero() || mailingRows[0]["kpi_mail_required"] != false {
		t.Fatalf("mailing mutated = %+v", mailingRows)
	}
}

func TestScheduleMassMailingAtQueuesAndTriggersCron(t *testing.T) {
	env, _ := threadEnv(t)
	cronID := createMassMailingQueueCron(t, env)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Scheduled", "subject": "Scheduled", "body_html": "<p>Scheduled</p>", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 20, 9, 30, 0, 0, time.UTC)

	if err := ScheduleMassMailingAt(env, mailingID, at); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "schedule_type", "schedule_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" || rows[0]["schedule_type"] != "scheduled" || !timeValue(rows[0]["schedule_date"]).Equal(at) {
		t.Fatalf("mailing rows = %+v", rows)
	}
	assertMassMailingQueueCronTrigger(t, env, cronID, at)
}

func TestSendMassMailingsAppliesDomainToListContacts(t *testing.T) {
	env, _ := threadEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Filtered", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activeID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Active", "email": "active-list@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	inactiveID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Inactive", "email": "inactive-list@example.com", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	for _, contactID := range []int64{activeID, inactiveID} {
		if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID}); err != nil {
			t.Fatal(err)
		}
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Filtered List",
		"subject":                 "Filtered {{ name }}",
		"body_html":               "<p>{{ email }}</p>",
		"mailing_model_real":      "mailing.contact",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
		"mailing_domain":          "[('active', '=', True)]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || len(result.MailIDs) != 1 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "active-list@example.com" {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestSendMassMailingsSupportsStoredListIDsDomain(t *testing.T) {
	env, _ := threadEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Stored Domain List", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Stored", "email": "stored-list@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	outsideID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Outside", "email": "outside-list@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Stored List Domain",
		"subject":                 "Stored {{ name }}",
		"body_html":               "<p>{{ email }}</p>",
		"mailing_model_real":      "mailing.contact",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
		"mailing_domain":          "[('list_ids', 'in', [" + stringAny(listID) + "]), ('id', 'in', [" + stringAny(contactID) + ", " + stringAny(outsideID) + "])]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || len(result.MailIDs) != 1 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "stored-list@example.com" {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestSendMassMailingsSkipsAlreadyTracedRecipients(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Already", "email": "already@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Fresh", "email": "fresh@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Domain Mailing",
		"subject":            "Domain {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "[['id', 'in', [" + stringAny(firstID) + "," + stringAny(secondID) + "]]]",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.trace").Create(map[string]any{
		"email":           "already@example.com",
		"model":           "res.partner",
		"res_id":          firstID,
		"mass_mailing_id": mailingID,
		"trace_status":    "sent",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || len(result.MailIDs) != 1 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "fresh@example.com" {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestSendMassMailingsBlankAndFalseDomainsSelectNoRecipients(t *testing.T) {
	for _, tc := range []struct {
		name   string
		domain any
	}{
		{name: "blank", domain: ""},
		{name: "false", domain: "False"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := threadEnv(t)
			if _, err := env.Model("res.partner").Create(map[string]any{"name": "Unsafe", "email": "unsafe@example.com", "active": true}); err != nil {
				t.Fatal(err)
			}
			mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
				"name":               "No Recipients",
				"subject":            "No Recipients",
				"body_html":          "<p>No Recipients</p>",
				"mailing_model_real": "res.partner",
				"mailing_domain":     tc.domain,
			})
			if err != nil {
				t.Fatal(err)
			}

			result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), "there are no recipients selected") {
				t.Fatalf("err = %v result=%+v", err, result)
			}
			mails, err := env.Model("mail.mail").Search(domain.And())
			if err != nil {
				t.Fatal(err)
			}
			if mails.Len() != 0 {
				t.Fatalf("mail rows = %d", mails.Len())
			}
		})
	}
}

func TestSendMassMailingsParsesPythonLiteralDomain(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Alpha", "email": "alpha@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Beta", "email": "beta@example.net", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	thirdID, err := env.Model("res.partner").Create(map[string]any{"name": "Gamma", "email": "gamma@example.org", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Literal Domain",
		"subject":            "Literal {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "[('id', 'in', (" + stringAny(firstID) + ", " + stringAny(secondID) + ", " + stringAny(thirdID) + ")), ('active', '=', True)]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || len(result.MailIDs) != 2 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("email_to")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[stringAny(row["email_to"])] = true
	}
	if !seen["alpha@example.com"] || !seen["beta@example.net"] || seen["gamma@example.org"] {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestSendMassMailingsParsesPrefixLiteralDomain(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Prefix One", "email": "prefix-one@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Prefix Two", "email": "prefix-two@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.partner").Create(map[string]any{"name": "Other", "email": "other@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Prefix Domain",
		"subject":            "Prefix {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "['|', ('id', '=', " + stringAny(firstID) + "), ('id', '=', " + stringAny(secondID) + ")]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || len(result.MailIDs) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestSendMassMailingsABTestingSamplesAndExcludesSiblings(t *testing.T) {
	env, _ := threadEnv(t)
	partnerIDs := make([]int64, 0, 10)
	for i := 0; i < 10; i++ {
		id, err := env.Model("res.partner").Create(map[string]any{"name": "AB Partner " + stringAny(i), "email": "ab" + stringAny(i) + "@example.com", "active": true})
		if err != nil {
			t.Fatal(err)
		}
		partnerIDs = append(partnerIDs, id)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "AB Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	domainText := "[('id', 'in', [" + strings.Join(int64Strings(partnerIDs), ",") + "])]"
	firstID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                         "AB First",
		"subject":                      "AB First {{ name }}",
		"body_html":                    "<p>{{ email }}</p>",
		"mailing_model_real":           "res.partner",
		"mailing_domain":               domainText,
		"campaign_id":                  campaignID,
		"ab_testing_enabled":           true,
		"ab_testing_pc":                int64(30),
		"ab_testing_schedule_datetime": time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                         "AB Second",
		"subject":                      "AB Second {{ name }}",
		"body_html":                    "<p>{{ email }}</p>",
		"mailing_model_real":           "res.partner",
		"mailing_domain":               domainText,
		"campaign_id":                  campaignID,
		"ab_testing_enabled":           true,
		"ab_testing_pc":                int64(30),
		"ab_testing_schedule_datetime": time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := SendMassMailings(env, []int64{firstID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	second, err := SendMassMailings(env, []int64{secondID}, nil, time.Date(2026, 6, 19, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.MailIDs) != 3 || len(second.MailIDs) != 3 {
		t.Fatalf("sample sizes first=%+v second=%+v", first, second)
	}
	firstRecipients := mailingTraceRecipientSet(t, env, firstID)
	secondRecipients := mailingTraceRecipientSet(t, env, secondID)
	for id := range firstRecipients {
		if secondRecipients[id] {
			t.Fatalf("duplicate A/B recipient %d first=%+v second=%+v", id, firstRecipients, secondRecipients)
		}
	}
}

func TestSelectABWinnerMassMailingQueuesFinalForRemainingRecipients(t *testing.T) {
	env, _ := threadEnv(t)
	partnerIDs := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		id, err := env.Model("res.partner").Create(map[string]any{"name": "Winner Partner " + stringAny(i), "email": "winner" + stringAny(i) + "@example.com", "active": true})
		if err != nil {
			t.Fatal(err)
		}
		partnerIDs = append(partnerIDs, id)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Winner Campaign", "ab_testing_winner_selection": "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Winner Source",
		"subject":            "Winner {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "[('id', 'in', [" + strings.Join(int64Strings(partnerIDs), ",") + "])]",
		"campaign_id":        campaignID,
		"ab_testing_enabled": true,
		"ab_testing_pc":      int64(40),
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := SendMassMailings(env, []int64{sourceID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(source.MailIDs) != 2 {
		t.Fatalf("source result = %+v", source)
	}
	winner, err := SelectABWinnerMassMailing(env, []int64{sourceID}, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mailing.mailing").Browse(winner.WinnerMailingID).Read("state", "ab_testing_pc", "ab_testing_is_winner_mailing")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" || int64FromAny(rows[0]["ab_testing_pc"]) != 100 || rows[0]["ab_testing_is_winner_mailing"] != true {
		t.Fatalf("winner row = %+v", rows)
	}
	campaignRows, err := env.Model("utm.campaign").Browse(campaignID).Read("ab_testing_completed", "ab_testing_winner_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(campaignRows) != 1 || campaignRows[0]["ab_testing_completed"] != true || int64FromAny(campaignRows[0]["ab_testing_winner_mailing_id"]) != winner.WinnerMailingID {
		t.Fatalf("campaign row = %+v", campaignRows)
	}
	final, err := ProcessMassMailingQueue(env, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(final.MailIDs) != 3 {
		t.Fatalf("final winner result = %+v", final)
	}
}

func TestSendABWinnerMassMailingSelectsAutomaticMetricWinner(t *testing.T) {
	env, _ := threadEnv(t)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Metric Campaign", "ab_testing_winner_selection": "opened_ratio"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Metric First", "subject": "Metric First", "body_html": "<p>First</p>", "campaign_id": campaignID, "ab_testing_enabled": true, "state": "done"})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Metric Second", "subject": "Metric Second", "body_html": "<p>Second</p>", "campaign_id": campaignID, "ab_testing_enabled": true, "state": "done"})
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range []map[string]any{
		{"mass_mailing_id": firstID, "model": "res.partner", "res_id": int64(1), "email": "first1@example.com", "trace_status": "open", "open_datetime": time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)},
		{"mass_mailing_id": firstID, "model": "res.partner", "res_id": int64(2), "email": "first2@example.com", "trace_status": "open", "open_datetime": time.Date(2026, 6, 19, 10, 1, 0, 0, time.UTC)},
		{"mass_mailing_id": secondID, "model": "res.partner", "res_id": int64(3), "email": "second1@example.com", "trace_status": "open", "open_datetime": time.Date(2026, 6, 19, 10, 2, 0, 0, time.UTC)},
		{"mass_mailing_id": secondID, "model": "res.partner", "res_id": int64(4), "email": "second2@example.com", "trace_status": "sent"},
	} {
		if _, err := env.Model("mailing.trace").Create(trace); err != nil {
			t.Fatal(err)
		}
	}

	result, err := SendABWinnerMassMailing(env, []int64{secondID}, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mailing.mailing").Browse(result.WinnerMailingID).Read("subject", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["subject"] != "Metric First" || rows[0]["state"] != "in_queue" {
		t.Fatalf("automatic winner row = %+v result=%+v", rows, result)
	}
}

func TestSendMassMailingsInvalidLiteralDomainSelectsNoRecipients(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("res.partner").Create(map[string]any{"name": "Invalid Domain Recipient", "email": "invalid-domain@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":               "Invalid Domain",
		"subject":            "Invalid",
		"body_html":          "<p>Invalid</p>",
		"mailing_model_real": "res.partner",
		"mailing_domain":     "[('email', '=', 'unterminated)]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendMassMailings(env, []int64{mailingID}, nil, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "there are no recipients selected") {
		t.Fatalf("err = %v result=%+v", err, result)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("mail rows = %d", mails.Len())
	}
}

func int64Strings(values []int64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, stringAny(value))
	}
	return out
}

func mailingTraceRecipientSet(t *testing.T, env *record.Env, mailingID int64) map[int64]bool {
	t.Helper()
	found, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("res_id")
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]bool{}
	for _, row := range rows {
		out[int64FromAny(row["res_id"])] = true
	}
	return out
}

func createMassMailingQueueCron(t *testing.T, env *record.Env) int64 {
	t.Helper()
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Mass Mailing: Process queue",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		"failure_count":   int64(0),
		"priority":        int64(6),
		"state":           "code",
		"code":            "model._process_mass_mailing_queue()",
		"action_name":     "mailing.mailing.process_mass_mailing_queue",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "mass_mailing",
		"name":          "ir_cron_mass_mailing_queue",
		"model":         "ir.cron",
		"res_id":        cronID,
		"complete_name": "mass_mailing.ir_cron_mass_mailing_queue",
	}); err != nil {
		t.Fatal(err)
	}
	return cronID
}

func assertMassMailingQueueCronTrigger(t *testing.T, env *record.Env, cronID int64, want time.Time) {
	t.Helper()
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
	t.Fatalf("mass mailing queue trigger for %s missing in %+v", want, rows)
}

func createMassMailingReportUser(t *testing.T, env *record.Env) int64 {
	t.Helper()
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Email Marketing / User"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "mass_mailing",
		"name":          "group_mass_mailing_user",
		"model":         "res.groups",
		"res_id":        groupID,
		"complete_name": "mass_mailing.group_mass_mailing_user",
	}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Marketing User", "email": "marketing@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{
		"login":      "marketing",
		"name":       "Marketing User",
		"email":      "marketing@example.com",
		"active":     true,
		"partner_id": partnerID,
		"groups_id":  []int64{groupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return userID
}

func createConfigParameter(t *testing.T, env *record.Env, key string, value string) {
	t.Helper()
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": key, "value": value}); err != nil {
		t.Fatal(err)
	}
}

func reportMailRows(t *testing.T, env *record.Env) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.Cond("subject", domain.Like, "24H Stats"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("subject", "body_html", "email_to", "state", "auto_delete")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestMassMailingKPIDigestTipFiltersByEmailMarketingPrivilege(t *testing.T) {
	env, _ := threadEnv(t)
	privilegeID, err := env.Model("res.groups.privilege").Create(map[string]any{"name": "Email Marketing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "mass_mailing",
		"name":          "res_groups_privilege_email_marketing",
		"model":         "res.groups.privilege",
		"res_id":        privilegeID,
		"complete_name": "mass_mailing.res_groups_privilege_email_marketing",
	}); err != nil {
		t.Fatal(err)
	}
	marketingGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Email Marketing / User", "privilege_id": privilegeID})
	if err != nil {
		t.Fatal(err)
	}
	otherGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("digest.tip").Create(map[string]any{"name": "Other Tip", "group_id": otherGroupID, "tip_description": `<p>Other Tip</p>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("digest.tip").Create(map[string]any{"name": "Marketing Tip", "group_id": marketingGroupID, "tip_description": `<p>Marketing Tip</p>`}); err != nil {
		t.Fatal(err)
	}
	tip := massMailingKPIDigestTipHTML(env)
	if !strings.Contains(tip, "Marketing Tip") || strings.Contains(tip, "Other Tip") {
		t.Fatalf("tip = %s", tip)
	}
}

func TestMassMailingKPIReportUserFallbackIgnoresWriteUID(t *testing.T) {
	env, _ := threadEnv(t)
	adminPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Administrator", "email": "admin@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "admin", "name": "Administrator", "email": "admin@example.com", "partner_id": adminPartnerID, "active": true}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Writer", "email": "writer@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	writerID, err := env.Model("res.users").Create(map[string]any{"login": "writer", "name": "Writer", "email": "writer@example.com", "partner_id": partnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	values, err := massMailingKPIReportMailValues(env, map[string]any{"subject": "Fallback", "write_uid": writerID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stringAny(values["email_to"]), "writer@example.com") || !strings.Contains(stringAny(values["email_to"]), "admin@example.com") {
		t.Fatalf("email_to = %v", values["email_to"])
	}
}

func TestProcessMassMailingQueueSendsKPIReportOnlyAfterOneDayAndOnce(t *testing.T) {
	env, _ := threadEnv(t)
	createConfigParameter(t, env, "mass_mailing.mass_mailing_reports", "True")
	createConfigParameter(t, env, "database.secret", "report-secret")
	userID := createMassMailingReportUser(t, env)
	sentAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("digest.tip").Create(map[string]any{"name": "Marketing KPI Tip", "tip_description": `<p>Tip: KPI discipline</p>`}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":              "KPI Mailing",
		"subject":           "KPI Subject",
		"state":             "done",
		"sent_date":         sentAt,
		"kpi_mail_required": true,
		"user_id":           userID,
	})
	if err != nil {
		t.Fatal(err)
	}
	recipientID, err := env.Model("res.partner").Create(map[string]any{"name": "KPI Recipient", "email": "kpi@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range []map[string]any{
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "sent", "sent_datetime": sentAt},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "open", "sent_datetime": sentAt, "open_datetime": sentAt.Add(time.Hour), "links_click_datetime": sentAt.Add(2 * time.Hour)},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "reply", "sent_datetime": sentAt, "open_datetime": sentAt.Add(time.Hour), "reply_datetime": sentAt.Add(3 * time.Hour)},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "bounce", "failure_type": "mail_bounce"},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "error", "failure_type": "mail_failed"},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "cancel", "failure_type": "mail_optout"},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "pending"},
		{"mass_mailing_id": mailingID, "model": "res.partner", "res_id": recipientID, "trace_status": "sent"},
	} {
		if _, err := env.Model("mailing.trace").Create(trace); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Model("link.tracker").Create(map[string]any{"mass_mailing_id": mailingID, "url": "https://example.com", "label": "Landing", "count": int64(2)}); err != nil {
		t.Fatal(err)
	}

	result, err := ProcessMassMailingQueue(env, sentAt.Add(23*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.KPIReports != 0 || len(reportMailRows(t, env)) != 0 {
		t.Fatalf("early KPI result=%+v rows=%+v", result, reportMailRows(t, env))
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("kpi_mail_required")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["kpi_mail_required"] != true {
		t.Fatalf("early flag = %+v", rows)
	}

	result, err = ProcessMassMailingQueue(env, sentAt.Add(25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	mails := reportMailRows(t, env)
	if result.KPIReports != 1 || len(result.KPIReportIDs) != 1 || len(mails) != 1 {
		t.Fatalf("KPI result=%+v rows=%+v", result, mails)
	}
	body := stringAny(mails[0]["body_html"])
	if mails[0]["state"] != "outgoing" || mails[0]["auto_delete"] != true ||
		!strings.Contains(stringAny(mails[0]["subject"]), `24H Stats of Emails "KPI Subject"`) ||
		!strings.Contains(stringAny(mails[0]["email_to"]), "marketing@example.com") ||
		!strings.Contains(body, "More Info") || !strings.Contains(body, "/odoo/mailing.mailing/") ||
		!strings.Contains(body, "Engagement on 8 Emails Sent") ||
		!strings.Contains(body, "57.14%") || !strings.Contains(body, "RECEIVED (4)") ||
		!strings.Contains(body, "40.0%") || !strings.Contains(body, "OPENED (2)") ||
		!strings.Contains(body, "20.0%") || !strings.Contains(body, "REPLIED (1)") ||
		!strings.Contains(body, "Click Rate Report on 8 Emails Sent") ||
		!strings.Contains(body, "Button Label") || !strings.Contains(body, "%Click (Total)") ||
		!strings.Contains(body, "Landing") || !strings.Contains(body, "66% (2)") ||
		!strings.Contains(body, "Tip: KPI discipline") ||
		!strings.Contains(body, "Turn off Mailing Reports") {
		t.Fatalf("KPI mail = %+v", mails[0])
	}
	rows, err = env.Model("mailing.mailing").Browse(mailingID).Read("kpi_mail_required")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["kpi_mail_required"] != false {
		t.Fatalf("KPI flag = %+v", rows)
	}

	result, err = ProcessMassMailingQueue(env, sentAt.Add(26*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.KPIReports != 0 || len(reportMailRows(t, env)) != 1 {
		t.Fatalf("duplicate KPI result=%+v rows=%+v", result, reportMailRows(t, env))
	}
}

func TestProcessMassMailingQueueSkipsKPIReportsWhenDisabledOrOutsideWindow(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		env, _ := threadEnv(t)
		createConfigParameter(t, env, "mass_mailing.mass_mailing_reports", "False")
		sentAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
		mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Disabled", "subject": "Disabled", "state": "done", "sent_date": sentAt, "kpi_mail_required": true})
		if err != nil {
			t.Fatal(err)
		}
		result, err := ProcessMassMailingQueue(env, sentAt.Add(25*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("kpi_mail_required")
		if err != nil {
			t.Fatal(err)
		}
		if result.KPIReports != 0 || len(reportMailRows(t, env)) != 0 || rows[0]["kpi_mail_required"] != true {
			t.Fatalf("disabled result=%+v rows=%+v mailing=%+v", result, reportMailRows(t, env), rows)
		}
	})
	t.Run("outside window", func(t *testing.T) {
		env, _ := threadEnv(t)
		createConfigParameter(t, env, "mass_mailing.mass_mailing_reports", "True")
		now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
		recentID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Recent", "subject": "Recent", "state": "done", "sent_date": now.Add(-23 * time.Hour), "kpi_mail_required": true})
		if err != nil {
			t.Fatal(err)
		}
		staleID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Stale", "subject": "Stale", "state": "done", "sent_date": now.Add(-6 * 24 * time.Hour), "kpi_mail_required": true})
		if err != nil {
			t.Fatal(err)
		}
		result, err := ProcessMassMailingQueue(env, now)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := env.Model("mailing.mailing").Browse(recentID, staleID).Read("kpi_mail_required")
		if err != nil {
			t.Fatal(err)
		}
		if result.KPIReports != 0 || len(reportMailRows(t, env)) != 0 || rows[0]["kpi_mail_required"] != true || rows[1]["kpi_mail_required"] != true {
			t.Fatalf("outside result=%+v rows=%+v mailing=%+v", result, reportMailRows(t, env), rows)
		}
	})
}

func TestProcessMassMailingQueueMarksNoRecipientMailingDone(t *testing.T) {
	env, _ := threadEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Empty List", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Empty Mailing",
		"subject":                 "Empty",
		"body_html":               "<p>Empty</p>",
		"mailing_model_real":      "mailing.contact",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
		"state":                   "in_queue",
		"schedule_date":           now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := ProcessMassMailingQueue(env, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Done != 1 || result.Skipped != 0 || len(result.MailIDs) != 0 || len(result.MailingIDs) != 1 || result.MailingIDs[0] != mailingID {
		t.Fatalf("result = %+v", result)
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "done" || !timeValue(rows[0]["sent_date"]).Equal(now) {
		t.Fatalf("mailing row = %+v", rows)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("mail rows = %d", mails.Len())
	}
}

func TestMassMailingScheduleCancelAndRetryFailedButtons(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":          "Button Mailing",
		"subject":       "Button",
		"body_html":     "<p>Button</p>",
		"state":         "draft",
		"schedule_type": "scheduled",
		"schedule_date": now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := ScheduleMassMailings(env, []int64{mailingID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("future schedule did not queue mailing")
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "schedule_date", "schedule_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" || rows[0]["schedule_type"] != "scheduled" || !timeValue(rows[0]["schedule_date"]).Equal(now.Add(time.Hour)) {
		t.Fatalf("queued row = %+v", rows)
	}

	if err := CancelMassMailings(env, []int64{mailingID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mailing.mailing").Browse(mailingID).Read("state", "schedule_date", "schedule_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "draft" || !timeValue(rows[0]["schedule_date"]).IsZero() || rows[0]["schedule_type"] != "now" {
		t.Fatalf("canceled row = %+v", rows)
	}

	messageID, err := env.Model("mail.message").Create(map[string]any{"subject": "Failed", "body": "<p>Failed</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "failed@example.com",
		"subject":         "Failed",
		"body_html":       "<p>Failed</p>",
		"state":           "exception",
		"mailing_id":      mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "failed@example.com",
		"model":           "res.partner",
		"res_id":          int64(1),
		"mass_mailing_id": mailingID,
		"trace_status":    "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RetryFailedMassMailings(env, []int64{mailingID}); err != nil {
		t.Fatal(err)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 0 {
		t.Fatalf("failed mail not removed = %+v", mailRows)
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 0 {
		t.Fatalf("failed trace not removed = %+v", traceRows)
	}
	rows, err = env.Model("mailing.mailing").Browse(mailingID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" {
		t.Fatalf("retry row = %+v", rows)
	}
}
