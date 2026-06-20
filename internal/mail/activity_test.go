package mail

import (
	"strings"
	"testing"

	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestScheduleActivityUsesXMLIDDefaults(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityIDs, err := ScheduleActivity(env, ActivityScheduleRequest{
		Model:             "res.partner",
		ResIDs:            []int64{partnerID},
		ActivityTypeXMLID: "mail.mail_activity_data_todo",
		DateDeadline:      "2026-07-01 08:00:00",
		Automated:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(activityIDs) != 1 {
		t.Fatalf("activity ids = %+v", activityIDs)
	}
	rows, err := env.Model("mail.activity").Browse(activityIDs...).Read("activity_type_id", "activity_category", "chaining_type", "res_model", "res_id", "user_id", "date_deadline", "summary", "note", "state", "automated")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["activity_type_id"] != ids["mail.mail_activity_data_todo"].ResID ||
		rows[0]["activity_category"] != "default" ||
		rows[0]["chaining_type"] != "suggest" ||
		rows[0]["res_model"] != "res.partner" ||
		rows[0]["res_id"] != partnerID ||
		rows[0]["user_id"] != ids["base.user_demo"].ResID ||
		rows[0]["date_deadline"] != "2026-07-01" ||
		rows[0]["summary"] != "Follow up" ||
		rows[0]["note"] != "Default todo note" ||
		rows[0]["state"] != "open" ||
		rows[0]["automated"] != true {
		t.Fatalf("activity row = %+v", rows)
	}
}

func TestApprovalActivityHideInChatterComputedFromXMLID(t *testing.T) {
	env, _ := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	approvalTypeID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":   "Approval",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "oi_workflow",
		"name":          "activity_type_approval",
		"complete_name": "oi_workflow.activity_type_approval",
		"model":         "mail.activity.type",
		"res_id":        approvalTypeID,
	}); err != nil {
		t.Fatal(err)
	}
	activityIDs, err := ScheduleActivity(env, ActivityScheduleRequest{
		Model:          "res.partner",
		ResIDs:         []int64{partnerID},
		ActivityTypeID: approvalTypeID,
		DateDeadline:   "2026-07-01",
		Automated:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.activity").Browse(activityIDs...).Read("hide_in_chatter")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["hide_in_chatter"] != true {
		t.Fatalf("approval activity hide_in_chatter = %+v", rows)
	}
	if err := env.Model("mail.activity").Browse(activityIDs...).Write(map[string]any{"automated": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mail.activity").Browse(activityIDs...).Read("hide_in_chatter")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["hide_in_chatter"] != false {
		t.Fatalf("manual approval activity hide_in_chatter = %+v", rows)
	}
}

func TestScheduleActivityUsesRecommendedTypeAndRelatedFields(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recommendedID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":            "Call",
		"summary":         "Call customer",
		"default_note":    "Use direct line",
		"default_user_id": ids["base.user_demo"].ResID,
		"category":        "phonecall",
		"chaining_type":   "suggest",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	previousID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":                    "Qualify",
		"suggested_next_type_ids": []int64{recommendedID},
		"active":                  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityIDs, err := ScheduleActivity(env, ActivityScheduleRequest{
		Model:             "res.partner",
		ResIDs:            []int64{partnerID},
		RecommendedTypeID: recommendedID,
		PreviousTypeID:    previousID,
		DateDeadline:      "2026-07-04",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.activity").Browse(activityIDs...).Read("activity_type_id", "recommended_activity_type_id", "previous_activity_type_id", "has_recommended_activities", "activity_category", "chaining_type", "summary", "note", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["activity_type_id"] != recommendedID ||
		rows[0]["recommended_activity_type_id"] != recommendedID ||
		rows[0]["previous_activity_type_id"] != previousID ||
		rows[0]["has_recommended_activities"] != true ||
		rows[0]["activity_category"] != "phonecall" ||
		rows[0]["chaining_type"] != "suggest" ||
		rows[0]["summary"] != "Call customer" ||
		rows[0]["note"] != "Use direct line" ||
		rows[0]["user_id"] != ids["base.user_demo"].ResID {
		t.Fatalf("recommended activity row = %+v", rows)
	}
}

func TestActivityTypeSuggestedNextMaintainsInverseDomain(t *testing.T) {
	env, _ := activityEnv(t)
	recommendedID, err := env.Model("mail.activity.type").Create(map[string]any{"name": "Recommended", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	alternateID, err := env.Model("mail.activity.type").Create(map[string]any{"name": "Alternate", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	previousID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":                    "Previous",
		"suggested_next_type_ids": []int64{recommendedID},
		"active":                  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.activity.type").Browse(recommendedID).Read("previous_type_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0]["previous_type_ids"].([]int64); len(got) != 1 || got[0] != previousID {
		t.Fatalf("previous_type_ids = %#v", rows[0]["previous_type_ids"])
	}
	found, err := env.Model("mail.activity.type").Search(domain.Cond("previous_type_ids", "=", previousID))
	if err != nil {
		t.Fatal(err)
	}
	if len(found.IDs()) != 1 || found.IDs()[0] != recommendedID {
		t.Fatalf("recommended domain ids = %+v", found.IDs())
	}
	if err := env.Model("mail.activity.type").Browse(previousID).Write(map[string]any{"triggered_next_type_id": alternateID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mail.activity.type").Browse(previousID).Read("chaining_type", "triggered_next_type_id", "suggested_next_type_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["chaining_type"] != "trigger" || rows[0]["triggered_next_type_id"] != alternateID || len(rows[0]["suggested_next_type_ids"].([]int64)) != 0 {
		t.Fatalf("trigger row = %+v", rows)
	}
	rows, err = env.Model("mail.activity.type").Browse(recommendedID).Read("previous_type_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows[0]["previous_type_ids"].([]int64)) != 0 {
		t.Fatalf("previous_type_ids after trigger = %#v", rows[0]["previous_type_ids"])
	}
	if err := env.Model("mail.activity.type").Browse(previousID).Write(map[string]any{"suggested_next_type_ids": []int64{recommendedID}}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("mail.activity.type").Browse(previousID).Read("chaining_type", "triggered_next_type_id", "suggested_next_type_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["chaining_type"] != "suggest" || rows[0]["triggered_next_type_id"] != int64(0) || len(rows[0]["suggested_next_type_ids"].([]int64)) != 1 {
		t.Fatalf("suggest row = %+v", rows)
	}
}

func TestActivityFeedbackPostsMessageWithAttachmentsAndMarksDone(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "done.txt",
		"res_model": "res.partner",
		"res_id":    partnerID,
		"type":      "binary",
		"datas":     "ZG9uZQ==",
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": ids["mail.mail_activity_data_todo"].ResID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          ids["base.user_demo"].ResID,
		"date_deadline":    "2026-07-01",
		"summary":          "Follow up",
		"note":             "<p>Remember</p>",
		"state":            "open",
		"automated":        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ActivityFeedback(env, ActivitySelectRequest{
		Model:              "res.partner",
		ResIDs:             []int64{partnerID},
		ActivityTypeXMLIDs: []string{"mail.mail_activity_data_todo"},
		UserID:             ids["base.user_demo"].ResID,
		OnlyAutomated:      true,
	}, "Finished\nLine 2", []int64{attachmentID})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected feedback to be handled")
	}
	activityRows, err := env.Model("mail.activity").Browse(activityID).Read("state", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if activityRows[0]["state"] != "done" {
		t.Fatalf("activity rows = %+v", activityRows)
	}
	if got := activityRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("activity attachment ids = %#v", activityRows[0]["attachment_ids"])
	}
	messages, err := env.Model("mail.message").Search(domain.And(
		domain.Cond("model", "=", "res.partner"),
		domain.Cond("res_id", "=", partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body", "attachment_ids", "subtype_id", "mail_activity_type_id", "body_is_html")
	if err != nil {
		t.Fatal(err)
	}
	expectedBody := `<div><p><span class="fa fa-check fa-fw"></span><span>To-Do</span> done (originally assigned to Demo): Follow up</p><div class="o_mail_note_title fw-bold">Original note:</div><div><p>Remember</p></div><div><div class="fw-bold">Feedback:</div>Finished<br/>Line 2</div></div>`
	if len(messageRows) != 1 || messageRows[0]["body"] != expectedBody || messageRows[0]["subtype_id"] != ids["mail.mt_activities"].ResID || messageRows[0]["mail_activity_type_id"] != ids["mail.mail_activity_data_todo"].ResID || messageRows[0]["body_is_html"] != true {
		t.Fatalf("messages = %+v", messageRows)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("attachment_ids = %#v", messageRows[0]["attachment_ids"])
	}
}

func TestActivityUnlinkFiltersAutomatedActivities(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	automatedID, err := env.Model("mail.activity").Create(activityValues(ids["mail.mail_activity_data_todo"].ResID, partnerID, true))
	if err != nil {
		t.Fatal(err)
	}
	manualID, err := env.Model("mail.activity").Create(activityValues(ids["mail.mail_activity_data_todo"].ResID, partnerID, false))
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ActivityUnlink(env, ActivitySelectRequest{
		Model:              "res.partner",
		ResIDs:             []int64{partnerID},
		ActivityTypeXMLIDs: []string{"mail.mail_activity_data_todo"},
		OnlyAutomated:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected unlink to be handled")
	}
	if rows, err := env.Model("mail.activity").Browse(automatedID).Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("automated rows = %+v err=%v", rows, err)
	}
	if rows, err := env.Model("mail.activity").Browse(manualID).Read("id"); err != nil || len(rows) != 1 {
		t.Fatalf("manual rows = %+v err=%v", rows, err)
	}
	ok, err = ActivityUnlink(env, ActivitySelectRequest{
		Model:              "res.partner",
		ResIDs:             []int64{partnerID},
		ActivityTypeXMLIDs: []string{"mail.mail_activity_data_todo"},
		OnlyAutomated:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected manual unlink to be handled")
	}
	if rows, err := env.Model("mail.activity").Browse(manualID).Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("manual rows after unlink = %+v err=%v", rows, err)
	}
}

func TestActivityActionFeedbackArchivesAndTriggersNext(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	nextTypeID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":            "Call",
		"summary":         "Call back",
		"default_user_id": ids["base.user_demo"].ResID,
		"category":        "phonecall",
		"delay_count":     int64(2),
		"delay_unit":      "days",
		"delay_from":      "previous_activity",
		"chaining_type":   "suggest",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	triggerTypeID, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":                   "Trigger",
		"summary":                "Trigger summary",
		"default_user_id":        ids["base.user_demo"].ResID,
		"delay_count":            int64(0),
		"delay_unit":             "days",
		"delay_from":             "previous_activity",
		"chaining_type":          "trigger",
		"triggered_next_type_id": nextTypeID,
		"active":                 true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": triggerTypeID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          ids["base.user_demo"].ResID,
		"date_deadline":    "2026-07-01",
		"summary":          "Trigger",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ActivityActionFeedback(env, []int64{activityID}, "Done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if int64FromAny(result) == 0 {
		t.Fatalf("action feedback result = %#v", result)
	}
	rows, err := env.Model("mail.activity").Browse(activityID).Read("state", "active", "feedback", "date_done")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "done" || rows[0]["active"] != false || rows[0]["feedback"] != "Done" || rows[0]["date_done"] == nil {
		t.Fatalf("archived activity = %+v", rows)
	}
	found, err := env.Model("mail.activity").Search(domain.And(
		domain.Cond("res_model", "=", "res.partner"),
		domain.Cond("res_id", "=", partnerID),
		domain.Cond("activity_type_id", "=", nextTypeID),
	))
	if err != nil {
		t.Fatal(err)
	}
	nextRows, err := found.Read("previous_activity_type_id", "has_recommended_activities", "activity_category", "chaining_type", "date_deadline", "summary", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(nextRows) != 1 || nextRows[0]["previous_activity_type_id"] != triggerTypeID || nextRows[0]["has_recommended_activities"] != false || nextRows[0]["activity_category"] != "phonecall" || nextRows[0]["chaining_type"] != "suggest" || nextRows[0]["date_deadline"] != "2026-07-03" || nextRows[0]["summary"] != "Call back" || nextRows[0]["active"] != true {
		t.Fatalf("next activity = %+v", nextRows)
	}
}

func TestActivityActionFeedbackMovesActivityAttachmentsToMessage(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": ids["mail.mail_activity_data_todo"].ResID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          ids["base.user_demo"].ResID,
		"date_deadline":    "2026-07-01",
		"summary":          "Manual",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityAttachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "activity.txt",
		"res_model": "mail.activity",
		"res_id":    activityID,
		"type":      "binary",
		"datas":     "YWN0aXZpdHk=",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ActivityActionFeedback(env, []int64{activityID}, "Done", nil)
	if err != nil {
		t.Fatal(err)
	}
	messageID := int64FromAny(result)
	if messageID == 0 {
		t.Fatalf("message id = %#v", result)
	}
	messageRows, err := env.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != activityAttachmentID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(activityAttachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "mail.message" || attachmentRows[0]["res_id"] != messageID {
		t.Fatalf("moved attachment = %+v", attachmentRows[0])
	}
}

func TestActivityActionFeedbackRemovesMissingTargetActivityAndAttachments(t *testing.T) {
	env, ids := activityEnv(t)
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": ids["mail.mail_activity_data_todo"].ResID,
		"res_model":        "res.partner",
		"res_id":           int64(999),
		"user_id":          ids["base.user_demo"].ResID,
		"date_deadline":    "2026-07-01",
		"summary":          "Missing",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityAttachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "lost.txt",
		"res_model": "mail.activity",
		"res_id":    activityID,
		"type":      "binary",
		"datas":     "bG9zdA==",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ActivityActionFeedback(env, []int64{activityID}, "Done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != false {
		t.Fatalf("missing target result = %#v", result)
	}
	if rows, err := env.Model("mail.activity").Browse(activityID).Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("activity rows after missing target completion = %+v err=%v", rows, err)
	}
	if rows, err := env.Model("ir.attachment").Browse(activityAttachmentID).Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("attachment rows after missing target completion = %+v err=%v", rows, err)
	}
	messages, err := env.Model("mail.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("messages after missing target completion = %d", messages.Len())
	}
}

func TestActivityActionFeedbackScheduleNextReturnsWizard(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": ids["mail.mail_activity_data_todo"].ResID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          ids["base.user_demo"].ResID,
		"date_deadline":    "2026-07-01",
		"summary":          "Manual next",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ActivityActionFeedbackScheduleNext(env, []int64{activityID}, "Done", nil)
	if err != nil {
		t.Fatal(err)
	}
	action := result.(map[string]any)
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "mail.activity" || action["target"] != "new" {
		t.Fatalf("schedule next action = %+v", action)
	}
	ctx := action["context"].(map[string]any)
	if ctx["default_previous_activity_type_id"] != ids["mail.mail_activity_data_todo"].ResID || ctx["default_res_id"] != partnerID || ctx["default_res_model"] != "res.partner" || ctx["activity_previous_deadline"] != "2026-07-01" {
		t.Fatalf("schedule next context = %+v", ctx)
	}
}

func TestActivityFormatReturnsStorePayload(t *testing.T) {
	env, ids := activityEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	assigneePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Assignee", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	assigneeID, err := env.Model("res.users").Create(map[string]any{"login": "assignee", "name": "Assignee", "partner_id": assigneePartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{"name": "Reminder", "model": "res.partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.activity.type").Browse(ids["mail.mail_activity_data_todo"].ResID).Write(map[string]any{
		"category":          "phonecall",
		"chaining_type":     "suggest",
		"mail_template_ids": []int64{templateID},
	}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "brief.txt", "res_model": "mail.activity", "res_id": int64(0), "type": "binary"})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("mail.activity").Create(map[string]any{
		"activity_type_id":  ids["mail.mail_activity_data_todo"].ResID,
		"activity_category": "phonecall",
		"res_model":         "res.partner",
		"res_id":            partnerID,
		"user_id":           assigneeID,
		"date_deadline":     "2026-07-02",
		"summary":           "Call customer",
		"note":              "<p>Prepare</p>",
		"state":             "open",
		"active":            true,
		"attachment_ids":    []int64{attachmentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := ActivityFormat(env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"mail_activity_today": "2026-07-02"}}), []int64{activityID})
	if err != nil {
		t.Fatal(err)
	}
	activityRows := payload["mail.activity"].([]map[string]any)
	if len(activityRows) != 1 {
		t.Fatalf("activity rows = %+v", payload)
	}
	row := activityRows[0]
	if row["display_name"] != "Call customer" || row["state"] != "today" || row["activity_type_id"] != ids["mail.mail_activity_data_todo"].ResID || row["mail_template_ids"].([]int64)[0] != templateID {
		t.Fatalf("activity format row = %+v", row)
	}
	if note := row["note"].([]any); len(note) != 2 || note[0] != "markup" || note[1] != "<p>Prepare</p>" {
		t.Fatalf("activity note = %+v", row["note"])
	}
	if len(payload["mail.activity.type"].([]map[string]any)) != 1 || len(payload["mail.template"].([]map[string]any)) != 1 || len(payload["ir.attachment"].([]map[string]any)) != 1 || len(payload["res.users"].([]map[string]any)) != 1 || len(payload["res.partner"].([]map[string]any)) != 1 {
		t.Fatalf("activity format related payload = %+v", payload)
	}
}

func TestGetActivityDataAggregatesOngoingAndDone(t *testing.T) {
	env, ids := activityEnv(t)
	ctxEnv := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"mail_activity_today": "2026-07-02"}})
	typeID := ids["mail.mail_activity_data_todo"].ResID
	templateID, err := env.Model("mail.template").Create(map[string]any{"name": "Reminder", "model": "res.partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.activity.type").Browse(typeID).Write(map[string]any{"mail_template_ids": []int64{templateID}}); err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "First", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Second", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	doneOnlyID, err := env.Model("res.partner").Create(map[string]any{"name": "Done Only", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userA, err := env.Model("res.users").Create(map[string]any{"login": "a", "name": "A", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userB, err := env.Model("res.users").Create(map[string]any{"login": "b", "name": "B", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	firstActivityIDs := []int64{}
	for _, spec := range []struct {
		date string
		user int64
		name string
	}{
		{date: "2026-07-01", user: userA, name: "Past"},
		{date: "2026-07-02", user: userB, name: "Today"},
		{date: "2026-07-03", user: userA, name: "Future"},
	} {
		id, err := env.Model("mail.activity").Create(map[string]any{"activity_type_id": typeID, "res_model": "res.partner", "res_id": firstID, "user_id": spec.user, "date_deadline": spec.date, "summary": spec.name, "state": "open", "active": true})
		if err != nil {
			t.Fatal(err)
		}
		firstActivityIDs = append(firstActivityIDs, id)
	}
	secondOngoingID, err := env.Model("mail.activity").Create(map[string]any{"activity_type_id": typeID, "res_model": "res.partner", "res_id": secondID, "user_id": userB, "date_deadline": "2026-07-04", "summary": "Second open", "state": "open", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "done.pdf", "res_model": "mail.activity", "res_id": int64(0), "type": "binary"})
	if err != nil {
		t.Fatal(err)
	}
	secondDoneID, err := env.Model("mail.activity").Create(map[string]any{"activity_type_id": typeID, "res_model": "res.partner", "res_id": secondID, "user_id": userA, "date_deadline": "2026-06-30", "date_done": "2026-07-05", "summary": "Second done", "state": "done", "active": false, "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}
	doneOnlyActivityID, err := env.Model("mail.activity").Create(map[string]any{"activity_type_id": typeID, "res_model": "res.partner", "res_id": doneOnlyID, "user_id": userA, "date_deadline": "2026-06-29", "date_done": "2026-07-06", "summary": "Done only", "state": "done", "active": false, "attachment_ids": []int64{attachmentID}})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := GetActivityData(ctxEnv, "res.partner", domain.And(), ActivityDataOptions{FetchDone: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := payload["activity_res_ids"].([]int64); len(got) != 3 || got[0] != firstID || got[1] != secondID || got[2] != doneOnlyID {
		t.Fatalf("activity res ids = %+v", payload["activity_res_ids"])
	}
	types := payload["activity_types"].([]map[string]any)
	if len(types) == 0 || types[0]["id"] != typeID || types[0]["template_ids"].([]map[string]any)[0]["name"] != "Reminder" {
		t.Fatalf("activity types = %+v", types)
	}
	grouped := payload["grouped_activities"].(map[int64]map[int64]map[string]any)
	first := grouped[firstID][typeID]
	if first["state"] != "overdue" || first["reporting_date"] != "2026-07-01" || !sameIDs(first["ids"], firstActivityIDs...) || !sameIDs(first["user_assigned_ids"], userA, userB) {
		t.Fatalf("first aggregate = %+v", first)
	}
	firstCounts := first["count_by_state"].(map[string]int)
	if firstCounts["overdue"] != 1 || firstCounts["today"] != 1 || firstCounts["planned"] != 1 {
		t.Fatalf("first counts = %+v", firstCounts)
	}
	second := grouped[secondID][typeID]
	if second["state"] != "planned" || second["reporting_date"] != "2026-07-04" || !sameIDs(second["ids"], secondOngoingID, secondDoneID) {
		t.Fatalf("second aggregate = %+v", second)
	}
	info := second["attachments_info"].(map[string]any)
	if info["count"] != 1 || info["most_recent_id"] != attachmentID || info["most_recent_name"] != "done.pdf" {
		t.Fatalf("attachment info = %+v", info)
	}
	doneOnly := grouped[doneOnlyID][typeID]
	if doneOnly["state"] != "done" || doneOnly["reporting_date"] != "2026-07-06" || !sameIDs(doneOnly["ids"], doneOnlyActivityID) {
		t.Fatalf("done aggregate = %+v", doneOnly)
	}

	ongoingOnly, err := GetActivityData(ctxEnv, "res.partner", domain.And(), ActivityDataOptions{FetchDone: false})
	if err != nil {
		t.Fatal(err)
	}
	if got := ongoingOnly["activity_res_ids"].([]int64); len(got) != 2 || got[0] != firstID || got[1] != secondID {
		t.Fatalf("ongoing activity res ids = %+v", got)
	}
	filtered, err := GetActivityData(ctxEnv, "res.partner", domain.Cond("id", "=", secondID), ActivityDataOptions{FetchDone: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := filtered["activity_res_ids"].([]int64); len(got) != 1 || got[0] != secondID {
		t.Fatalf("filtered activity res ids = %+v", got)
	}
}

func activityEnv(t *testing.T) (*record.Env, map[string]data.ExternalID) {
	t.Helper()
	env, ids := threadEnv(t)
	if _, err := env.Model("res.users").Create(map[string]any{"login": "current", "name": "Current", "active": true}); err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{"login": "demo", "name": "Demo", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ids["base.user_demo"] = data.ExternalID{Module: "base", Name: "user_demo", Model: "res.users", ResID: userID}
	loader := data.NewLoaderWithExternalIDs(env, "mail", ids)
	if err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="mail_activity_data_todo" model="mail.activity.type">
    <field name="name">To-Do</field>
    <field name="summary">Follow up</field>
    <field name="default_note">Default todo note</field>
    <field name="default_user_id" ref="base.user_demo"/>
    <field name="icon">fa-check</field>
    <field name="active" eval="True"/>
  </record>
</odoo>`)); err != nil {
		t.Fatal(err)
	}
	return env, loader.ExternalIDs()
}

func activityValues(activityTypeID int64, partnerID int64, automated bool) map[string]any {
	return map[string]any{
		"activity_type_id": activityTypeID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          int64(1),
		"date_deadline":    "2026-07-01",
		"summary":          "Follow up",
		"state":            "open",
		"automated":        automated,
		"active":           true,
	}
}

func sameIDs(value any, want ...int64) bool {
	got := int64SliceFromAny(value)
	got = uniqueIDs(got)
	want = uniqueIDs(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
