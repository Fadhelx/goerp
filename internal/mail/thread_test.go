package mail

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
	"gorp/internal/security"
)

func TestPostMessageCreatesThreadMessageWithDefaults(t *testing.T) {
	env, ids := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Mentioned", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)

	messageID, err := PostMessage(env, PostRequest{
		Model:      "res.partner",
		ResID:      recordID,
		Body:       `<b>Hello</b>`,
		PartnerIDs: []int64{partnerID},
		AutoFollow: true,
		Now:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("body", "message_type", "model", "res_id", "subtype_id", "partner_ids", "date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["body"] != `&lt;b&gt;Hello&lt;/b&gt;` || rows[0]["message_type"] != "notification" || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != recordID || rows[0]["subtype_id"] != ids["mail.mt_note"].ResID {
		t.Fatalf("message = %+v", rows)
	}
	if got := rows[0]["partner_ids"].([]int64); len(got) != 1 || got[0] != partnerID {
		t.Fatalf("partner_ids = %#v", rows[0]["partner_ids"])
	}
	followers, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", "res.partner"),
		domain.Cond("res_id", "=", recordID),
		domain.Cond("partner_id", "=", partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if followers.Len() != 1 {
		t.Fatalf("followers = %d", followers.Len())
	}
	notifications, err := env.Model("mail.notification").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		t.Fatal(err)
	}
	if notifications.Len() != 1 {
		t.Fatalf("notifications = %d", notifications.Len())
	}
}

func TestSubscribeUpdatesExistingFollowerSubtypes(t *testing.T) {
	env, ids := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Follower", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	commentID := ids["mail.mt_comment"].ResID
	noteID := ids["mail.mt_note"].ResID
	if err := Subscribe(env, "res.partner", recordID, []int64{partnerID}, []int64{commentID}); err != nil {
		t.Fatal(err)
	}
	if err := Subscribe(env, "res.partner", recordID, []int64{partnerID}, []int64{noteID}); err != nil {
		t.Fatal(err)
	}
	followers, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", "res.partner"),
		domain.Cond("res_id", "=", recordID),
		domain.Cond("partner_id", "=", partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if followers.Len() != 1 {
		t.Fatalf("followers = %d", followers.Len())
	}
	rows, err := followers.Read("subtype_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := int64SliceFromAny(rows[0]["subtype_ids"]); len(got) != 1 || got[0] != noteID {
		t.Fatalf("subtype_ids = %+v", rows[0]["subtype_ids"])
	}
}

func TestPostMessageStoresRecordCompanyAndAliasDomain(t *testing.T) {
	env, _ := threadEnv(t)
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
		"name":       "Thread",
		"active":     true,
		"company_id": recordCompanyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	env = env.WithContext(record.Context{UserID: 1, CompanyID: contextCompanyID, CompanyIDs: []int64{contextCompanyID, recordCompanyID}})

	messageID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: "Record domain"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("record_company_id", "record_alias_domain_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["record_company_id"] != recordCompanyID || rows[0]["record_alias_domain_id"] != recordAliasDomainID {
		t.Fatalf("message record routing fields = %+v", rows)
	}
}

func TestPostMessageCreatesTrackingValues(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}

	messageID, err := PostMessage(env, PostRequest{
		Model: "res.partner",
		ResID: recordID,
		Body:  "Name changed",
		TrackingValues: []TrackingValue{
			{
				FieldName:    "name",
				FieldDesc:    "Name",
				FieldType:    "char",
				OldValueChar: "Before",
				NewValueChar: "After",
			},
			{
				FieldName:       "active",
				FieldDesc:       "Active",
				FieldType:       "boolean",
				OldValueInteger: 0,
				NewValueInteger: 1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := env.Model("mail.message").Browse(messageID).Read("tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	trackingIDs := messages[0]["tracking_value_ids"].([]int64)
	if len(trackingIDs) != 2 {
		t.Fatalf("tracking ids = %#v", trackingIDs)
	}
	tracking, err := env.Model("mail.tracking.value").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tracking.Read("field_name", "field_desc", "field_type", "old_value_char", "new_value_char", "old_value_integer", "new_value_integer", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("tracking rows = %+v", rows)
	}
	byField := map[string]map[string]any{}
	for _, row := range rows {
		byField[row["field_name"].(string)] = row
	}
	nameRow := byField["name"]
	if nameRow["field_desc"] != "Name" || nameRow["field_type"] != "char" || nameRow["old_value_char"] != "Before" || nameRow["new_value_char"] != "After" || nameRow["mail_message_id"] != messageID {
		t.Fatalf("name tracking = %+v", nameRow)
	}
	activeRow := byField["active"]
	if activeRow["field_desc"] != "Active" || activeRow["field_type"] != "boolean" || activeRow["old_value_integer"] != int64(0) || activeRow["new_value_integer"] != int64(1) || activeRow["mail_message_id"] != messageID {
		t.Fatalf("active tracking = %+v", activeRow)
	}
	fetched, err := FetchThreadMessages(env, ThreadMessagesRequest{Model: "res.partner", ResID: recordID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	data := fetched["data"].(map[string]any)
	fetchedMessages := data["mail.message"].([]map[string]any)
	if len(fetchedMessages) != 1 {
		t.Fatalf("fetched messages = %+v", fetchedMessages)
	}
	rawTracking := data["mail.tracking.value"].([]map[string]any)
	if len(rawTracking) != 2 {
		t.Fatalf("raw fetched tracking = %+v", rawTracking)
	}
	formatted := fetchedMessages[0]["trackingValues"].([]map[string]any)
	if len(formatted) != 2 {
		t.Fatalf("formatted tracking = %+v", formatted)
	}
	byChangedField := map[string]map[string]any{}
	for _, item := range formatted {
		info := item["fieldInfo"].(map[string]any)
		byChangedField[info["changedField"].(string)] = item
	}
	if item := byChangedField["Name"]; item["oldValue"] != "Before" || item["newValue"] != "After" || item["fieldInfo"].(map[string]any)["fieldType"] != "char" {
		t.Fatalf("formatted name tracking = %+v", item)
	}
	if item := byChangedField["Active"]; item["oldValue"] != false || item["newValue"] != true || item["fieldInfo"].(map[string]any)["fieldType"] != "boolean" {
		t.Fatalf("formatted active tracking = %+v", item)
	}
}

func TestFetchThreadMessagesSearchMatchesOdooFields(t *testing.T) {
	env, ids := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	commentSubtypeID := ids["mail.mt_comment"].ResID
	customSubtypeID, err := env.Model("mail.message.subtype").Create(map[string]any{"name": "Custom", "description": "SubtypeNeedle", "internal": false})
	if err != nil {
		t.Fatal(err)
	}
	subjectID, err := env.Model("mail.message").Create(map[string]any{"body": "<p>Body only</p>", "subject": "Subject bridge Token", "message_type": "comment", "model": "res.partner", "res_id": recordID, "subtype_id": commentSubtypeID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "attachment-needle.pdf", "res_model": "res.partner", "res_id": recordID, "type": "binary", "datas": "ZGF0YQ=="})
	if err != nil {
		t.Fatal(err)
	}
	attachmentMessageID, err := env.Model("mail.message").Create(map[string]any{"body": "<p>Attachment holder</p>", "message_type": "comment", "model": "res.partner", "res_id": recordID, "subtype_id": commentSubtypeID, "attachment_ids": []int64{attachmentID}, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	subtypeMessageID, err := env.Model("mail.message").Create(map[string]any{"body": "<p>Subtype holder</p>", "message_type": "comment", "model": "res.partner", "res_id": recordID, "subtype_id": customSubtypeID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	trackingMessageID, err := env.Model("mail.message").Create(map[string]any{"body": "<p>Tracking holder</p>", "message_type": "comment", "model": "res.partner", "res_id": recordID, "subtype_id": commentSubtypeID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.tracking.value").Create(map[string]any{"field_name": "email", "field_desc": "Email", "field_type": "char", "old_value_char": "before@example.com", "new_value_char": "tracking-needle@example.com", "mail_message_id": trackingMessageID}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		term string
		want int64
	}{
		{term: "Subject Token", want: subjectID},
		{term: "attachment-needle", want: attachmentMessageID},
		{term: "SubtypeNeedle", want: subtypeMessageID},
		{term: "tracking-needle", want: trackingMessageID},
	}
	for _, tc := range cases {
		fetched, err := FetchThreadMessages(env, ThreadMessagesRequest{Model: "res.partner", ResID: recordID, SearchTerm: tc.term, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if int64FromAny(fetched["count"]) != 1 {
			t.Fatalf("search %q count payload = %+v", tc.term, fetched)
		}
		messages := fetched["messages"].([]int64)
		if len(messages) != 1 || messages[0] != tc.want {
			t.Fatalf("search %q messages = %+v want %d", tc.term, messages, tc.want)
		}
	}
}

func TestUpdateMessageContentRejectsTrackedAndNonCommentMessages(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: "<p>Old</p>", MessageType: "comment", BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>New</p>", BodySet: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>New</p>") || !strings.Contains(body, "o-mail-Message-edited") {
		t.Fatalf("updated body = %s", body)
	}
	trackedID, err := PostMessage(env, PostRequest{
		Model:       "res.partner",
		ResID:       recordID,
		Body:        "Tracked",
		MessageType: "comment",
		TrackingValues: []TrackingValue{{
			FieldName:    "name",
			FieldDesc:    "Name",
			FieldType:    "char",
			OldValueChar: "Old",
			NewValueChar: "New",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{MessageID: trackedID, Body: "Blocked", BodySet: true}); err == nil || !strings.Contains(err.Error(), "tracking values cannot be modified") {
		t.Fatalf("tracked update error = %v", err)
	}
	notificationID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: "System"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{MessageID: notificationID, Body: "Blocked", BodySet: true}); err == nil || !strings.Contains(err.Error(), "only messages type comment") {
		t.Fatalf("notification update error = %v", err)
	}
}

func TestUpdateMessageContentDiscussChannelParity(t *testing.T) {
	env, _ := threadEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Discuss Readers"})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Author", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	allowedPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Allowed", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	blockedPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Blocked", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "discuss-author", "name": "Author", "active": true, "partner_id": authorPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "allowed", "name": "Allowed", "active": true, "partner_id": allowedPartnerID, "groups_id": []int64{groupID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "blocked", "name": "Blocked", "active": true, "partner_id": blockedPartnerID}); err != nil {
		t.Fatal(err)
	}
	channelID, err := env.Model("discuss.channel").Create(map[string]any{"name": "Internal", "channel_type": "channel", "group_public_id": groupID})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{
		Model:       "discuss.channel",
		ResID:       channelID,
		Body:        "<p>Old</p>",
		MessageType: "comment",
		AuthorID:    authorPartnerID,
		BodyIsHTML:  true,
		TrackingValues: []TrackingValue{{
			FieldName:    "name",
			FieldDesc:    "Name",
			FieldType:    "char",
			OldValueChar: "Old",
			NewValueChar: "New",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{
		MessageID:     messageID,
		Body:          "<p>Discuss edit</p>",
		BodySet:       true,
		PartnerIDs:    []int64{allowedPartnerID, blockedPartnerID},
		PartnerIDsSet: true,
	}); err != nil {
		t.Fatalf("discuss tracked update error = %v", err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("body", "partner_ids", "tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>Discuss edit</p>") || !strings.Contains(body, "o-mail-Message-edited") {
		t.Fatalf("discuss body = %s", body)
	}
	if got := rows[0]["partner_ids"].([]int64); len(got) != 1 || got[0] != allowedPartnerID {
		t.Fatalf("channel filtered partner_ids = %#v", got)
	}
	if got := rows[0]["tracking_value_ids"].([]int64); len(got) != 1 {
		t.Fatalf("discuss tracking ids = %#v", got)
	}

	memberChannelID, err := env.Model("discuss.channel").Create(map[string]any{"name": "Group", "channel_type": "group"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("discuss.channel.member").Create(map[string]any{"channel_id": memberChannelID, "partner_id": allowedPartnerID}); err != nil {
		t.Fatal(err)
	}
	memberMessageID, err := PostMessage(env, PostRequest{Model: "discuss.channel", ResID: memberChannelID, Body: "<p>Group</p>", MessageType: "comment", AuthorID: authorPartnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{MessageID: memberMessageID, PartnerIDs: []int64{allowedPartnerID, blockedPartnerID}, PartnerIDsSet: true}); err != nil {
		t.Fatalf("member-filtered update error = %v", err)
	}
	memberRows, err := env.Model("mail.message").Browse(memberMessageID).Read("partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := memberRows[0]["partner_ids"].([]int64); len(got) != 1 || got[0] != allowedPartnerID {
		t.Fatalf("group filtered partner_ids = %#v", got)
	}

	notificationID, err := PostMessage(env, PostRequest{Model: "discuss.channel", ResID: channelID, Body: "System"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMessageContent(env, MessageContentUpdateRequest{MessageID: notificationID, Body: "Blocked", BodySet: true}); err == nil || !strings.Contains(err.Error(), "model 'discuss.channel'") {
		t.Fatalf("discuss notification update error = %v", err)
	}

	if _, err := env.Model("discuss.channel.member").Create(map[string]any{"channel_id": memberChannelID, "user_id": authorUserID, "partner_id": authorPartnerID}); err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Users[authorUserID] = security.User{ID: authorUserID, Login: "discuss-author", Active: true}
	env.WithPolicy(engine)
	authorEnv := env.WithContext(record.Context{UserID: authorUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(authorEnv, MessageContentUpdateRequest{MessageID: memberMessageID, Body: "<p>Author member</p>", BodySet: true}); err != nil {
		t.Fatalf("member read access update error = %v", err)
	}
}

func TestUpdateMessageContentRequiresAuthorOrAdmin(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Author", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Other", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false, "partner_id": authorPartnerID}); err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "author", "name": "Author", "active": true, "partner_id": authorPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := env.Model("res.users").Create(map[string]any{"login": "other", "name": "Other", "active": true, "partner_id": otherPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	erpManagerGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Access Rights"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_erp_manager", "complete_name": "base.group_erp_manager", "model": "res.groups", "res_id": erpManagerGroupID}); err != nil {
		t.Fatal(err)
	}
	adminUserID, err := env.Model("res.users").Create(map[string]any{"login": "admin2", "name": "Admin", "active": true, "partner_id": otherPartnerID, "groups_id": []int64{erpManagerGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: "<p>Old</p>", MessageType: "comment", AuthorID: authorPartnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}

	otherEnv := env.WithContext(record.Context{UserID: otherUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(otherEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Blocked</p>", BodySet: true}); err == nil || !strings.Contains(err.Error(), "not allowed to edit message") {
		t.Fatalf("other user update error = %v", err)
	}

	authorEnv := env.WithContext(record.Context{UserID: authorUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(authorEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Author</p>", BodySet: true}); err != nil {
		t.Fatalf("author update error = %v", err)
	}

	adminEnv := env.WithContext(record.Context{UserID: adminUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(adminEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Admin</p>", BodySet: true}); err != nil {
		t.Fatalf("admin update error = %v", err)
	}

	superEnv := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(superEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Root</p>", BodySet: true}); err != nil {
		t.Fatalf("superuser update error = %v", err)
	}
}

func TestUpdateMessageContentAuthorAndImpliedSystemGroupWithPolicy(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Author", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	adminPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Admin", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false, "partner_id": authorPartnerID}); err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "author2", "name": "Author", "active": true, "partner_id": authorPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	adminUserID, err := env.Model("res.users").Create(map[string]any{"login": "admin3", "name": "Admin", "active": true, "partner_id": adminPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	userGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	erpManagerGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Access Rights", "implied_ids": []int64{userGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	systemGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Settings", "implied_ids": []int64{erpManagerGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_erp_manager", "complete_name": "base.group_erp_manager", "model": "res.groups", "res_id": erpManagerGroupID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_system", "complete_name": "base.group_system", "model": "res.groups", "res_id": systemGroupID}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(authorUserID).Write(map[string]any{"groups_id": []int64{userGroupID}}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(adminUserID).Write(map[string]any{"groups_id": []int64{systemGroupID}}); err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: "<p>Old</p>", MessageType: "comment", AuthorID: authorPartnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}

	engine := security.NewEngine()
	engine.Groups[userGroupID] = security.Group{ID: userGroupID, Name: "Internal"}
	engine.Groups[erpManagerGroupID] = security.Group{ID: erpManagerGroupID, Name: "Access Rights", ImpliedIDs: []int64{userGroupID}}
	engine.Groups[systemGroupID] = security.Group{ID: systemGroupID, Name: "Settings", ImpliedIDs: []int64{erpManagerGroupID}}
	engine.Users[authorUserID] = security.User{ID: authorUserID, Login: "author2", Active: true, GroupIDs: []int64{userGroupID}}
	engine.Users[adminUserID] = security.User{ID: adminUserID, Login: "admin3", Active: true, GroupIDs: []int64{systemGroupID}}
	engine.ACLs = []security.ACL{
		{Model: "mail.message", GroupID: userGroupID, Active: true, PermRead: true, PermCreate: true},
		{Model: "mail.message", GroupID: erpManagerGroupID, Active: true, PermRead: true, PermCreate: true},
		{Model: "res.users", GroupID: userGroupID, Active: true, PermRead: true},
		{Model: "res.users", GroupID: erpManagerGroupID, Active: true, PermRead: true},
		{Model: "ir.model.data", GroupID: userGroupID, Active: true, PermRead: true},
		{Model: "ir.model.data", GroupID: erpManagerGroupID, Active: true, PermRead: true},
	}
	env.WithPolicy(engine)

	authorEnv := env.WithContext(record.Context{UserID: authorUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(authorEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Author policy</p>", BodySet: true}); err != nil {
		t.Fatalf("author policy update error = %v", err)
	}

	adminEnv := env.WithContext(record.Context{UserID: adminUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := UpdateMessageContent(adminEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Implied system</p>", BodySet: true}); err != nil {
		t.Fatalf("implied system update error = %v", err)
	}
}

func TestUpdateMessageContentAttachmentOwnershipAndDeletion(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Author", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false, "partner_id": authorPartnerID}); err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{"login": "author3", "name": "Author", "active": true, "partner_id": authorPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	userGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(authorUserID).Write(map[string]any{"groups_id": []int64{userGroupID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mail-test-secret"}); err != nil {
		t.Fatal(err)
	}
	existingAttachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "old.txt",
		"res_model": "res.partner",
		"res_id":    recordID,
		"type":      "binary",
		"datas":     "b2xk",
	})
	if err != nil {
		t.Fatal(err)
	}
	newAttachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":         "new.txt",
		"res_model":    "mail.compose.message",
		"res_id":       int64(99),
		"type":         "binary",
		"datas":        "bmV3",
		"access_token": "stored-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{
		Model:         "res.partner",
		ResID:         recordID,
		Body:          "<p>Old</p>",
		MessageType:   "comment",
		AuthorID:      authorPartnerID,
		AttachmentIDs: []int64{existingAttachmentID},
		BodyIsHTML:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := security.NewEngine()
	engine.Groups[userGroupID] = security.Group{ID: userGroupID, Name: "Internal"}
	engine.Users[authorUserID] = security.User{ID: authorUserID, Login: "author3", Active: true, GroupIDs: []int64{userGroupID}}
	engine.ACLs = []security.ACL{
		{Model: "mail.message", GroupID: userGroupID, Active: true, PermRead: true, PermCreate: true},
	}
	env.WithPolicy(engine)
	authorEnv := env.WithContext(record.Context{UserID: authorUserID, CompanyID: 1, CompanyIDs: []int64{1}})

	_, err = UpdateMessageContent(authorEnv, MessageContentUpdateRequest{
		MessageID:           messageID,
		AttachmentIDs:       []int64{newAttachmentID},
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{},
		AttachmentTokensSet: true,
	})
	if err == nil || !strings.Contains(err.Error(), "attachments do not exist") {
		t.Fatalf("empty token ownership error = %v", err)
	}
	_, err = UpdateMessageContent(authorEnv, MessageContentUpdateRequest{
		MessageID:           messageID,
		AttachmentIDs:       []int64{newAttachmentID},
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{"stored-token"},
		AttachmentTokensSet: true,
	})
	if err == nil || !strings.Contains(err.Error(), "attachments do not exist") {
		t.Fatalf("stored access token error = %v", err)
	}
	_, err = UpdateMessageContent(authorEnv, MessageContentUpdateRequest{
		MessageID:           messageID,
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{"extra"},
		AttachmentTokensSet: true,
	})
	if err == nil || !strings.Contains(err.Error(), "access token must be provided") {
		t.Fatalf("extra token count error = %v", err)
	}
	expires := fmt.Sprintf("0x%x", time.Now().Add(time.Hour).Unix())
	ownershipToken := limitedFieldAccessToken(env, "ir.attachment", newAttachmentID, "id", expires, "attachment_ownership")
	if _, err := UpdateMessageContent(authorEnv, MessageContentUpdateRequest{
		MessageID:           messageID,
		AttachmentIDs:       []int64{newAttachmentID},
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{ownershipToken},
		AttachmentTokensSet: true,
	}); err != nil {
		t.Fatalf("owned attachment update error = %v", err)
	}
	messageRows, err := env.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 2 || got[0] != existingAttachmentID || got[1] != newAttachmentID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(newAttachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "res.partner" || attachmentRows[0]["res_id"] != recordID {
		t.Fatalf("linked attachment = %+v", attachmentRows[0])
	}

	if _, err := UpdateMessageContent(authorEnv, MessageContentUpdateRequest{MessageID: messageID, AttachmentIDsSet: true}); err != nil {
		t.Fatalf("attachment delete update error = %v", err)
	}
	messageRows, err = env.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 0 {
		t.Fatalf("cleared attachment ids = %#v", got)
	}
	deletedRows, err := env.Model("ir.attachment").Browse(existingAttachmentID, newAttachmentID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedRows) != 0 {
		t.Fatalf("deleted attachments still exist = %+v", deletedRows)
	}
}

func TestPostMessageAttachmentOwnershipAndRelink(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false}); err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{"login": "poster", "name": "Poster", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(userID).Write(map[string]any{"groups_id": []int64{userGroupID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mail-post-secret"}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":         "post.txt",
		"res_model":    "mail.compose.message",
		"res_id":       int64(12),
		"type":         "binary",
		"datas":        "cG9zdA==",
		"access_token": "stored-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Groups[userGroupID] = security.Group{ID: userGroupID, Name: "Internal"}
	engine.Users[userID] = security.User{ID: userID, Login: "poster", Active: true, GroupIDs: []int64{userGroupID}}
	engine.ACLs = []security.ACL{
		{Model: "res.partner", GroupID: userGroupID, Active: true, PermRead: true},
		{Model: "mail.message", GroupID: userGroupID, Active: true, PermCreate: true, PermRead: true},
	}
	env.WithPolicy(engine)
	posterEnv := env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})

	_, err = PostMessage(posterEnv, PostRequest{
		Model:               "res.partner",
		ResID:               recordID,
		Body:                "<p>Bad</p>",
		MessageType:         "comment",
		AttachmentIDs:       []int64{attachmentID},
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{"stored-token"},
		AttachmentTokensSet: true,
		BodyIsHTML:          true,
	})
	if err == nil || !strings.Contains(err.Error(), "attachments do not exist") {
		t.Fatalf("stored post token error = %v", err)
	}
	_, err = PostMessage(posterEnv, PostRequest{
		Model:               "res.partner",
		ResID:               recordID,
		Body:                "<p>Bad</p>",
		MessageType:         "comment",
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{"extra"},
		AttachmentTokensSet: true,
		BodyIsHTML:          true,
	})
	if err == nil || !strings.Contains(err.Error(), "access token must be provided") {
		t.Fatalf("post token count error = %v", err)
	}
	expires := fmt.Sprintf("0x%x", time.Now().Add(time.Hour).Unix())
	ownershipToken := limitedFieldAccessToken(env, "ir.attachment", attachmentID, "id", expires, "attachment_ownership")
	messageID, err := PostMessage(posterEnv, PostRequest{
		Model:               "res.partner",
		ResID:               recordID,
		Body:                "<p>Good</p>",
		MessageType:         "comment",
		AttachmentIDs:       []int64{attachmentID},
		AttachmentIDsSet:    true,
		AttachmentTokens:    []string{ownershipToken},
		AttachmentTokensSet: true,
		BodyIsHTML:          true,
	})
	if err != nil {
		t.Fatalf("owned post attachment error = %v", err)
	}
	messageRows, err := env.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("post message attachment ids = %#v", got)
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "res.partner" || attachmentRows[0]["res_id"] != recordID {
		t.Fatalf("post linked attachment = %+v", attachmentRows[0])
	}
}

func TestPostMessagePortalAccessTokenSetsPublicAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})

	if _, err := PostMessage(publicEnv, PostRequest{Model: "portal.thread", ResID: threadID, Body: "<p>Bad</p>", MessageType: "comment", AccessToken: "bad", BodyIsHTML: true}); err == nil {
		t.Fatal("invalid portal token allowed post")
	}
	messageID, err := PostMessage(publicEnv, PostRequest{Model: "portal.thread", ResID: threadID, Body: "<p>Portal</p>", MessageType: "comment", AccessToken: "thread-token", BodyIsHTML: true})
	if err != nil {
		t.Fatalf("portal token post error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != threadID {
		t.Fatalf("portal message = %+v", rows)
	}
}

func TestPostMessagePortalParentHashSetsPublicAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("portal.thread").Create(map[string]any{"name": "Parent Thread", "partner_id": partnerID, "access_token": "parent-token"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("portal.thread").Create(map[string]any{"name": "Child Thread", "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "portal-parent-secret"}); err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})
	hash := portalAccessHash(publicEnv, "parent-token", partnerID)
	if hash == "" {
		t.Fatal("empty parent portal hash")
	}

	messageID, err := PostMessage(publicEnv, PostRequest{Model: "portal.thread", ResID: childID, Body: "<p>Via Parent</p>", MessageType: "comment", AccessHash: hash, AccessPID: partnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatalf("parent hash post error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != childID {
		t.Fatalf("parent hash portal message = %+v", rows)
	}
}

func TestPostMessageProjectTaskParentHashUsesProjectToken(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := env.Model("project.project").Create(map[string]any{"name": "Project", "partner_id": partnerID, "access_token": "project-token"})
	if err != nil {
		t.Fatal(err)
	}
	parentTaskID, err := env.Model("project.task").Create(map[string]any{"name": "Parent Task", "project_id": projectID, "partner_id": partnerID, "access_token": "parent-task-token"})
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := env.Model("project.task").Create(map[string]any{"name": "Task", "project_id": projectID, "parent_id": parentTaskID, "partner_id": partnerID, "access_token": "task-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "project-parent-secret"}); err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})
	parentTaskHash := portalAccessHash(publicEnv, "parent-task-token", partnerID)
	if parentTaskHash == "" {
		t.Fatal("empty parent task hash")
	}
	if _, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Wrong Parent</p>", MessageType: "comment", AccessHash: parentTaskHash, AccessPID: partnerID, BodyIsHTML: true}); err == nil {
		t.Fatal("project task accepted same-model parent hash")
	}
	if _, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Project Token</p>", MessageType: "comment", AccessToken: "project-token", BodyIsHTML: true}); err == nil {
		t.Fatal("project task accepted project token without project_sharing_id")
	}
	if _, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Wrong Project</p>", MessageType: "comment", AccessToken: "project-token", ProjectSharingID: projectID + 999, BodyIsHTML: true}); err == nil {
		t.Fatal("project task accepted wrong project_sharing_id")
	}
	if _, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Wrong Direct Token</p>", MessageType: "comment", AccessToken: "task-token", ProjectSharingID: projectID + 999, BodyIsHTML: true}); err == nil {
		t.Fatal("project task accepted task token with wrong project_sharing_id")
	}
	sharingMessageID, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Via Sharing</p>", MessageType: "comment", AccessToken: "project-token", ProjectSharingID: projectID, BodyIsHTML: true})
	if err != nil {
		t.Fatalf("project sharing token post error = %v", err)
	}
	projectHash := portalAccessHash(publicEnv, "project-token", partnerID)
	if projectHash == "" {
		t.Fatal("empty project hash")
	}
	messageID, err := PostMessage(publicEnv, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Via Project</p>", MessageType: "comment", AccessHash: projectHash, AccessPID: partnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatalf("project parent hash post error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	sharingRows, err := messageSystemEnv(env).Model("mail.message").Browse(sharingMessageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(sharingRows) != 1 || sharingRows[0]["author_id"] != partnerID || sharingRows[0]["model"] != "project.task" || sharingRows[0]["res_id"] != taskID {
		t.Fatalf("project sharing token message = %+v", sharingRows)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "project.task" || rows[0]["res_id"] != taskID {
		t.Fatalf("project parent hash message = %+v", rows)
	}
}

func TestUpdateMessageContentPortalHashAllowsPublicAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Other", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "portal-secret"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "portal.thread", ResID: threadID, Body: "<p>Old</p>", MessageType: "comment", AuthorID: partnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	otherMessageID, err := PostMessage(env, PostRequest{Model: "portal.thread", ResID: threadID, Body: "<p>Other</p>", MessageType: "comment", AuthorID: otherPartnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})

	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Bad</p>", BodySet: true, AccessToken: "bad"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("invalid portal update error = %v", err)
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: otherMessageID, Body: "<p>Wrong</p>", BodySet: true, AccessToken: "thread-token"}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("portal author mismatch error = %v", err)
	}
	hash := portalAccessHash(publicEnv, "thread-token", partnerID)
	if hash == "" {
		t.Fatal("empty portal hash")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Portal</p>", BodySet: true, AccessHash: hash, AccessPID: partnerID}); err != nil {
		t.Fatalf("portal hash update error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("body", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["author_id"] != partnerID || !strings.Contains(rows[0]["body"].(string), "<p>Portal</p>") {
		t.Fatalf("portal updated message = %+v", rows[0])
	}
}

func TestUpdateMessageContentPortalParentHashAllowsPublicAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("portal.thread").Create(map[string]any{"name": "Parent Thread", "partner_id": partnerID, "access_token": "parent-token"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("portal.thread").Create(map[string]any{"name": "Child Thread", "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "portal-parent-update-secret"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "portal.thread", ResID: childID, Body: "<p>Old</p>", MessageType: "comment", AuthorID: partnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})
	hash := portalAccessHash(publicEnv, "parent-token", partnerID)
	if hash == "" {
		t.Fatal("empty parent portal hash")
	}

	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Parent Hash</p>", BodySet: true, AccessHash: hash, AccessPID: partnerID}); err != nil {
		t.Fatalf("parent hash portal update error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("body", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["author_id"] != partnerID || !strings.Contains(rows[0]["body"].(string), "<p>Parent Hash</p>") {
		t.Fatalf("parent hash portal updated message = %+v", rows[0])
	}
}

func TestUpdateMessageContentProjectTaskParentHashUsesProjectToken(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := env.Model("project.project").Create(map[string]any{"name": "Project", "partner_id": partnerID, "access_token": "project-token"})
	if err != nil {
		t.Fatal(err)
	}
	parentTaskID, err := env.Model("project.task").Create(map[string]any{"name": "Parent Task", "project_id": projectID, "partner_id": partnerID, "access_token": "parent-task-token"})
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := env.Model("project.task").Create(map[string]any{"name": "Task", "project_id": projectID, "parent_id": parentTaskID, "partner_id": partnerID, "access_token": "task-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "project-parent-update-secret"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{Model: "project.task", ResID: taskID, Body: "<p>Old</p>", MessageType: "comment", AuthorID: partnerID, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(security.NewEngine())
	publicEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "gorp"}})
	parentTaskHash := portalAccessHash(publicEnv, "parent-task-token", partnerID)
	if parentTaskHash == "" {
		t.Fatal("empty parent task hash")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Wrong</p>", BodySet: true, AccessHash: parentTaskHash, AccessPID: partnerID}); err == nil {
		t.Fatal("project task accepted same-model parent hash update")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Project Token</p>", BodySet: true, AccessToken: "project-token"}); err == nil {
		t.Fatal("project task accepted project token update without project_sharing_id")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Wrong Project</p>", BodySet: true, AccessToken: "project-token", ProjectSharingID: projectID + 999}); err == nil {
		t.Fatal("project task accepted wrong project_sharing_id update")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Wrong Direct Token</p>", BodySet: true, AccessToken: "task-token", ProjectSharingID: projectID + 999}); err == nil {
		t.Fatal("project task accepted task token update with wrong project_sharing_id")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Via Sharing</p>", BodySet: true, AccessToken: "project-token", ProjectSharingID: projectID}); err != nil {
		t.Fatalf("project sharing token update error = %v", err)
	}
	projectHash := portalAccessHash(publicEnv, "project-token", partnerID)
	if projectHash == "" {
		t.Fatal("empty project hash")
	}
	if _, err := UpdateMessageContent(publicEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Via Project</p>", BodySet: true, AccessHash: projectHash, AccessPID: partnerID}); err != nil {
		t.Fatalf("project parent hash update error = %v", err)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("body", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["author_id"] != partnerID || !strings.Contains(rows[0]["body"].(string), "<p>Via Project</p>") {
		t.Fatalf("project parent hash update rows = %+v", rows[0])
	}
}

func TestUpdateMessageContentAllowsCurrentGuestAuthor(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	guestID, err := env.Model("mail.guest").Create(map[string]any{"name": "Guest", "email": "guest@example.test", "access_token": "guest-token"})
	if err != nil {
		t.Fatal(err)
	}
	otherGuestID, err := env.Model("mail.guest").Create(map[string]any{"name": "Other", "access_token": "other-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := PostMessage(env, PostRequest{
		Model:         "res.partner",
		ResID:         recordID,
		Body:          "<p>Guest</p>",
		MessageType:   "comment",
		AuthorGuestID: guestID,
		BodyIsHTML:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherGuestEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"guest_id": otherGuestID}})
	if _, err := UpdateMessageContent(otherGuestEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Blocked</p>", BodySet: true}); err == nil || !strings.Contains(err.Error(), "not allowed to edit message") {
		t.Fatalf("other guest update error = %v", err)
	}
	guestEnv := env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"guest_id": guestID}})
	if _, err := UpdateMessageContent(guestEnv, MessageContentUpdateRequest{MessageID: messageID, Body: "<p>Guest edit</p>", BodySet: true}); err != nil {
		t.Fatalf("guest update error = %v", err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("body", "author_guest_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["author_guest_id"] != guestID || !strings.Contains(rows[0]["body"].(string), "<p>Guest edit</p>") {
		t.Fatalf("guest message = %+v", rows[0])
	}
}

func TestPostMessageRejectsInvalidTargetsAndPreservesHTML(t *testing.T) {
	env, _ := threadEnv(t)
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PostMessage(env, PostRequest{Model: "mail.thread", ResID: recordID}); err == nil {
		t.Fatal("expected abstract mail.thread rejection")
	}
	if _, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, MessageType: "user_notification"}); err == nil {
		t.Fatal("expected user_notification rejection")
	}
	messageID, err := PostMessage(env, PostRequest{Model: "res.partner", ResID: recordID, Body: `<p>Safe</p>`, BodyIsHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.message").Browse(messageID).Read("body", "body_is_html")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["body"] != `<p>Safe</p>` || rows[0]["body_is_html"] != true {
		t.Fatalf("html message = %+v", rows[0])
	}
}

func TestSendTemplateBatchCreatesMailAndScheduledRows(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Thread", "email": "thread@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "template.txt",
		"res_model": "mail.template",
		"type":      "binary",
		"mimetype":  "text/plain",
		"datas":     []byte("template attachment"),
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":           "Partner Template",
		"model":          "res.partner",
		"subject":        "Hello {{ name }}",
		"body_html":      "<p>{{ object.name }}</p>",
		"email_to":       "{{ email }}",
		"email_cc":       "audit@example.com",
		"attachment_ids": []int64{attachmentID},
		"scheduled_date": "2026-07-01 10:00:00",
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("mail_message_id", "email_to", "email_cc", "subject", "body_html", "attachment_ids", "scheduled_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || mails[0]["email_to"] != "thread@example.com" || mails[0]["email_cc"] != "audit@example.com" || mails[0]["subject"] != "Hello Thread" || mails[0]["body_html"] != "<p>Thread</p>" {
		t.Fatalf("mail rows = %+v", mails)
	}
	if got := mails[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("mail attachment ids = %#v", mails[0]["attachment_ids"])
	}
	scheduled, err := env.Model("mail.scheduled.message").Search(domain.Cond("mail_mail_id", "=", mailIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.Len() != 1 {
		t.Fatalf("scheduled messages = %d", scheduled.Len())
	}
	scheduledRows, err := scheduled.Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := scheduledRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("scheduled attachment ids = %#v", scheduledRows[0]["attachment_ids"])
	}
	messageID := mails[0]["mail_message_id"].(int64)
	messages, err := env.Model("mail.message").Browse(messageID).Read("message_type", "model", "res_id", "attachment_ids", "body_is_html")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0]["message_type"] != "email" || messages[0]["model"] != "res.partner" || messages[0]["res_id"] != partnerID || messages[0]["body_is_html"] != true {
		t.Fatalf("message rows = %+v", messages)
	}
	if got := messages[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("message attachment ids = %#v", messages[0]["attachment_ids"])
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "mail.template" || int64FromAny(attachmentRows[0]["res_id"]) != 0 {
		t.Fatalf("template attachment owner = %+v", attachmentRows[0])
	}
}

func TestSendTemplateBatchCreatesMassMailingTrace(t *testing.T) {
	env, _ := threadEnv(t)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Launch"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Newsletter"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Email"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":        "June Campaign",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Trace", "email": "Trace@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Mass Template",
		"model":     "res.partner",
		"subject":   "Hello {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
		Now:         time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs[0]).Read("state", "mail_message_id", "email_to", "subject", "body_html", "is_notification", "mailing_id", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 ||
		mailRows[0]["state"] != "outgoing" ||
		int64FromAny(mailRows[0]["mail_message_id"]) == 0 ||
		mailRows[0]["email_to"] != "Trace@Example.COM" ||
		mailRows[0]["subject"] != "Hello Trace" ||
		mailRows[0]["body_html"] != "<p>Trace@Example.COM</p>" ||
		mailRows[0]["is_notification"] != true ||
		mailRows[0]["mailing_id"] != mailingID ||
		stringAny(mailRows[0]["message_id"]) != "" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	found, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id", "=", mailIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := found.Read("trace_type", "mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "mass_mailing_id", "campaign_id", "source_id", "medium_id", "trace_status", "failure_type", "failure_reason", "message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		traceRows[0]["mail_mail_id"] != mailIDs[0] ||
		traceRows[0]["mail_mail_id_int"] != mailIDs[0] ||
		traceRows[0]["email"] != "trace@example.com" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["mass_mailing_id"] != mailingID ||
		traceRows[0]["campaign_id"] != campaignID ||
		traceRows[0]["source_id"] != sourceID ||
		traceRows[0]["medium_id"] != mediumID ||
		traceRows[0]["trace_type"] != "mail" ||
		traceRows[0]["trace_status"] != "outgoing" ||
		stringAny(traceRows[0]["failure_type"]) != "" ||
		stringAny(traceRows[0]["failure_reason"]) != "" ||
		stringAny(traceRows[0]["message_id"]) != "" {
		t.Fatalf("trace rows = %+v", traceRows)
	}

	sender := &recordingSender{}
	result, err := SendMails(context.Background(), env, sender, mailIDs, time.Date(2026, 6, 19, 10, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 1 || len(sender.sent) != 1 {
		t.Fatalf("result=%+v sent=%+v", result, sender.sent)
	}
	mailRows, err = env.Model("mail.mail").Browse(mailIDs[0]).Read("message_id")
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err = env.Model("mailing.trace").Browse(int64FromAny(traceRows[0]["id"])).Read("trace_status", "sent_datetime", "message_id", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || timeValue(traceRows[0]["sent_datetime"]).IsZero() || traceRows[0]["message_id"] != mailRows[0]["message_id"] || stringAny(traceRows[0]["failure_type"]) != "" {
		t.Fatalf("sent trace = %+v mail=%+v", traceRows, mailRows)
	}
}

func TestSendTemplateBatchMassMailingShortensLinksAndCreatesTrackers(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Short Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Short Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Short Email"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":        "Short Mailing",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Shorty", "email": "short@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Short Links",
		"model":     "res.partner",
		"subject":   "Short",
		"body_html": `<p><a href="https://example.com/product">Product</a><a href="/local/page">Local</a><a href="mailto:a@example.com">Mail</a></p>`,
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs...).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	body := stringAny(mailRows[0]["body_html"])
	if strings.Contains(body, "https://example.com/product") || strings.Contains(body, `href="/local/page"`) || strings.Count(body, "https://gorp.example/r/") != 2 || !strings.Contains(body, `href="mailto:a@example.com"`) {
		t.Fatalf("shortened body = %s", body)
	}
	found, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err := found.Read("url", "label", "redirected_url", "campaign_id", "source_id", "medium_id", "mass_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(trackerRows) != 2 {
		t.Fatalf("tracker rows = %+v", trackerRows)
	}
	seenURLs := map[string]bool{}
	seenLabels := map[string]bool{}
	for _, row := range trackerRows {
		if row["campaign_id"] != campaignID || row["source_id"] != sourceID || row["medium_id"] != mediumID || row["mass_mailing_id"] != mailingID {
			t.Fatalf("tracker UTM row = %+v", row)
		}
		seenURLs[stringAny(row["url"])] = true
		seenLabels[stringAny(row["label"])] = true
		redirected, err := url.Parse(stringAny(row["redirected_url"]))
		if err != nil {
			t.Fatal(err)
		}
		query := redirected.Query()
		if query.Get("utm_campaign") != "Short Campaign" || query.Get("utm_source") != "Short Source" || query.Get("utm_medium") != "Short Email" {
			t.Fatalf("redirected UTM row = %+v query=%s", row, redirected.RawQuery)
		}
		codeSearch, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", "=", int64FromAny(row["id"])))
		if err != nil {
			t.Fatal(err)
		}
		if codeSearch.Len() != 1 {
			t.Fatalf("tracker code count for %+v = %d", row, codeSearch.Len())
		}
	}
	if !seenURLs["https://example.com/product"] || !seenURLs["https://gorp.example/local/page"] {
		t.Fatalf("tracker URLs = %+v", seenURLs)
	}
	if !seenLabels["Product"] || !seenLabels["Local"] {
		t.Fatalf("tracker labels = %+v", seenLabels)
	}
}

func TestMassMailingShortenerExtractsOdooHTMLLabels(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Label Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	body := `<p>` +
		`<a href="https://example.com/text"> Plain &lt; Text </a>` +
		`<a href="https://example.com/image-alt"><img src="/assets/hero.png" alt="Hero"></a>` +
		`<a href="https://example.com/image-src"><img src="https://cdn.example.com/path/logo.png"></a>` +
		`<a href="https://example.com/image-title"><img src="https://cdn.example.com/title.png" title="Title Only"></a>` +
		`<a href="https://example.com/direct-wins"> blurp <img src="/ignored.png" alt="Ignored"></a>` +
		`<a href="https://example.com/outlook"><p class="o_outlook_hack"><img src="/outlook.png" alt="Outlook Hero"></p></a>` +
		`<a href="https://example.com/nested"><em>here</em></a>` +
		`<a href="https://example.com/empty-img"><img/></a>` +
		`<a href="https://example.com/long">1234567890123456789012345678901234567890ABCDE</a>` +
		`<a href="https://example.com/self"/>` +
		`<a href="https://example.com/mso"><!--[if mso]><img src="https://cdn.example.com/mso.png" alt="Wrong"><![endif]--><!--[if !mso]><!--><img src="https://cdn.example.com/visible.png" alt="Visible"/><!--<![endif]--></a>` +
		`<a href="https://example.com/same"><img src="https://cdn.example.com/same.png"></a>` +
		`<a href="https://example.com/same"><img src="https://cdn.example.com/same.png"></a>` +
		`<a href="https://example.com/same">Different</a>` +
		`</p>`

	rewritten, err := shortenMassMailingRenderedLinks(env, body, mailingID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rewritten, "https://example.com/text") || strings.Count(rewritten, "https://gorp.example/r/") != 14 {
		t.Fatalf("rewritten body = %s", rewritten)
	}
	found, err := env.Model("link.tracker").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("url", "label", "short_url")
	if err != nil {
		t.Fatal(err)
	}
	trackers := map[string]map[string]any{}
	for _, row := range rows {
		trackers[stringAny(row["url"])+"|"+stringAny(row["label"])] = row
	}
	for _, want := range []struct {
		url   string
		label string
	}{
		{"https://example.com/text", "Plain < Text"},
		{"https://example.com/image-alt", "[media] Hero"},
		{"https://example.com/image-src", "[media] logo.png"},
		{"https://example.com/image-title", "[media] title.png"},
		{"https://example.com/direct-wins", "blurp"},
		{"https://example.com/outlook", "[media] Outlook Hero"},
		{"https://example.com/nested", ""},
		{"https://example.com/empty-img", ""},
		{"https://example.com/long", "1234567890123456789012345678901234567890"},
		{"https://example.com/self", ""},
		{"https://example.com/mso", "[media] Visible"},
		{"https://example.com/same", "[media] same.png"},
		{"https://example.com/same", "Different"},
	} {
		if trackers[want.url+"|"+want.label] == nil {
			t.Fatalf("missing tracker url=%s label=%q rows=%+v", want.url, want.label, rows)
		}
	}
	sameImageShort := stringAny(trackers["https://example.com/same|[media] same.png"]["short_url"])
	sameDifferentShort := stringAny(trackers["https://example.com/same|Different"]["short_url"])
	if sameImageShort == "" || sameDifferentShort == "" || sameImageShort == sameDifferentShort {
		t.Fatalf("same url short urls image=%q different=%q", sameImageShort, sameDifferentShort)
	}
	if strings.Count(rewritten, sameImageShort) != 2 || strings.Count(rewritten, sameDifferentShort) != 1 {
		t.Fatalf("same-url replacement body=%s image=%s different=%s", rewritten, sameImageShort, sameDifferentShort)
	}
}

func TestSendTemplateBatchMassMailingShortenerReusesAndSkipsLinks(t *testing.T) {
	env, _ := threadEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Reuse Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	existingID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/reused", "label": "One", "mass_mailing_id": mailingID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("link.tracker").Browse(existingID).Write(map[string]any{"code": "ReUse1"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Reuse", "email": "reuse@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Reuse Links",
		"model":     "res.partner",
		"subject":   "Reuse",
		"body_html": `<p><a href="https://example.com/reused">One</a><a href="https://example.com/reused">Two</a><a href="https://example.com/keep">Keep</a><a href="https://gorp.example/r/Old123">Old</a><a href="/unsubscribe_from_list">Unsub</a><a href="/view">View</a><a href="/viewform">View Form</a><a href="/cards/1">Cards</a><a href="tel:+123">Tel</a><a href="sms:+123">SMS</a></p>`,
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs...).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	body := stringAny(mailRows[0]["body_html"])
	if strings.Count(body, "https://gorp.example/r/ReUse1") != 1 || strings.Contains(body, "https://example.com/reused") {
		t.Fatalf("reused body = %s", body)
	}
	for _, expected := range []string{"https://gorp.example/r/Old123", "/unsubscribe_from_list", "/view", "/cards/1", "tel:+123", "sms:+123"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("skipped link %s missing from %s", expected, body)
		}
	}
	if strings.Contains(body, `href="/viewform"`) {
		t.Fatalf("viewform boundary link was not shortened: %s", body)
	}
	found, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 4 {
		rows, _ := found.Read("url", "label")
		t.Fatalf("tracker count = %d rows=%+v", found.Len(), rows)
	}
	codeSearch, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", "=", existingID))
	if err != nil {
		t.Fatal(err)
	}
	if codeSearch.Len() != 1 {
		t.Fatalf("existing tracker code count = %d", codeSearch.Len())
	}
}

func TestSendTemplateBatchCreatesMassMailingTraceInitialStates(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "State Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "State Template",
		"model":     "res.partner",
		"subject":   "State {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		name          string
		email         string
		state         string
		failureType   string
		traceStatus   string
		messageID     string
		expectedMsgID string
	}{
		{"Canceled", "cancel@example.com", "cancel", "mail_bl", "cancel", "cancel@example.test", "<cancel@example.test>"},
		{"Exception", "exception@example.com", "exception", "mail_smtp", "error", "<exception@example.test>", "<exception@example.test>"},
	} {
		partnerID, err := env.Model("res.partner").Create(map[string]any{"name": spec.name, "email": spec.email, "active": true})
		if err != nil {
			t.Fatal(err)
		}
		mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
			TemplateID: templateID,
			ResIDs:     []int64{partnerID},
			EmailValues: map[string]any{
				"mass_mailing_id": mailingID,
				"state":           spec.state,
				"failure_type":    spec.failureType,
				"message_id":      spec.messageID,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(mailIDs) != 1 {
			t.Fatalf("%s mail ids = %+v", spec.name, mailIDs)
		}
		traceSearch, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id", "=", mailIDs[0]))
		if err != nil {
			t.Fatal(err)
		}
		traceRows, err := traceSearch.Read("trace_status", "failure_type", "message_id", "email", "mail_mail_id_int")
		if err != nil {
			t.Fatal(err)
		}
		if len(traceRows) != 1 ||
			traceRows[0]["trace_status"] != spec.traceStatus ||
			traceRows[0]["failure_type"] != spec.failureType ||
			traceRows[0]["message_id"] != spec.expectedMsgID ||
			traceRows[0]["email"] != spec.email ||
			traceRows[0]["mail_mail_id_int"] != mailIDs[0] {
			t.Fatalf("%s trace = %+v", spec.name, traceRows)
		}
	}
}

func TestSendTemplateBatchMassMailingBlacklistCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Blacklist Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Blocked", "email": "Blocked@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.blacklist").Create(map[string]any{"email": "blocked@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Blacklist Template",
		"model":     "res.partner",
		"subject":   "Blocked {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
		Now:         time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 0 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Search(domain.Cond("mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("blacklist generated mail rows: %d", mails.Len())
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("subject", "=", "Blocked Blocked"))
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("blacklist generated messages: %d", messages.Len())
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		traceRows[0]["email"] != "blocked@example.com" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_bl" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendTemplateBatchMassMailingDuplicateCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Duplicate Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	firstPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "First", "email": "dup@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Second", "email": "Dup@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Duplicate Template",
		"model":     "res.partner",
		"subject":   "Duplicate {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{firstPartnerID, secondPartnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
		Now:         time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Search(domain.Cond("mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 1 {
		t.Fatalf("mail count = %d", mails.Len())
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("subject", "=", "Duplicate Second"))
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("duplicate generated messages: %d", messages.Len())
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 2 {
		t.Fatalf("trace rows = %+v", traceRows)
	}
	var outgoingRow, canceledRow map[string]any
	for _, row := range traceRows {
		switch stringAny(row["failure_type"]) {
		case "":
			outgoingRow = row
		case "mail_dup":
			canceledRow = row
		}
	}
	if outgoingRow == nil ||
		int64FromAny(outgoingRow["mail_mail_id"]) != mailIDs[0] ||
		int64FromAny(outgoingRow["mail_mail_id_int"]) != mailIDs[0] ||
		outgoingRow["email"] != "dup@example.com" ||
		outgoingRow["res_id"] != firstPartnerID ||
		outgoingRow["trace_status"] != "outgoing" {
		t.Fatalf("outgoing trace = %+v", outgoingRow)
	}
	if canceledRow == nil ||
		int64FromAny(canceledRow["mail_mail_id"]) != 0 ||
		int64FromAny(canceledRow["mail_mail_id_int"]) != 0 ||
		canceledRow["email"] != "dup@example.com" ||
		canceledRow["model"] != "res.partner" ||
		canceledRow["res_id"] != secondPartnerID ||
		canceledRow["trace_status"] != "cancel" {
		t.Fatalf("canceled trace = %+v", canceledRow)
	}
}

func TestSendTemplateBatchMassMailingMissingRecipientCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Missing Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "No Email", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Missing Template",
		"model":     "res.partner",
		"subject":   "Missing {{ name }}",
		"body_html": "<p>{{ name }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 0 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Search(domain.Cond("mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("missing generated mail rows: %d", mails.Len())
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("subject", "=", "Missing No Email"))
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("missing generated messages: %d", messages.Len())
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		stringAny(traceRows[0]["email"]) != "" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_email_missing" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendTemplateBatchMassMailingInvalidRecipientCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Invalid Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Invalid", "email": "not-an-email", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Invalid Template",
		"model":     "res.partner",
		"subject":   "Invalid {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 0 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		traceRows[0]["email"] != "not-an-email" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_email_invalid" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendTemplateBatchMassMailingListOptOutCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Generated Optout", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Generated Opted", "email": "Generated.Optout@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID, "opt_out": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Generated Optout Campaign",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Generated", "email": "generated.optout@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Optout Template",
		"model":     "res.partner",
		"subject":   "Optout {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 0 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		traceRows[0]["email"] != "generated.optout@example.com" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_optout" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendTemplateBatchMassMailingExclusionDisabledIgnoresBlacklist(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "No Exclusion Campaign", "use_exclusion_list": false})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Allowed", "email": "allowed.blacklist@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.blacklist").Create(map[string]any{"email": "allowed.blacklist@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "No Exclusion Template",
		"model":     "res.partner",
		"subject":   "Allowed {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	rows, err := env.Model("mail.mail").Browse(mailIDs[0]).Read("state", "failure_type", "email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "outgoing" || stringAny(rows[0]["failure_type"]) != "" || rows[0]["email_to"] != "allowed.blacklist@example.com" {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestSendTemplateBatchGeneratedMassMailingTraceFailsWithQueue(t *testing.T) {
	env, _ := threadEnv(t)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Failure Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Failure Mailing", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Failure Trace", "email": "failure.trace@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Failure Template",
		"model":     "res.partner",
		"subject":   "Failure {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := SendMails(context.Background(), env, failingSender{err: fmt.Errorf("smtp failure")}, mailIDs, time.Date(2026, 6, 19, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.Sent != 0 || result.Failed != 1 {
		t.Fatalf("queue result = %+v", result)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs[0]).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["state"] != "exception" || mailRows[0]["failure_type"] != "mail_smtp" || mailRows[0]["failure_reason"] != "send failed" {
		t.Fatalf("mail row = %+v", mailRows)
	}
	traceSearch, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id", "=", mailIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traceSearch.Read("trace_status", "failure_type", "failure_reason", "message_id", "sent_datetime", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		traceRows[0]["trace_status"] != "error" ||
		traceRows[0]["failure_type"] != "mail_smtp" ||
		stringAny(traceRows[0]["failure_reason"]) != "" ||
		stringAny(traceRows[0]["message_id"]) == "" ||
		!timeValue(traceRows[0]["sent_datetime"]).IsZero() ||
		traceRows[0]["campaign_id"] != campaignID {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendTemplateBatchGeneratesDynamicReportAttachment(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Report Partner", "email": "report@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	staticAttachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "static.txt",
		"res_model": "mail.template",
		"type":      "binary",
		"mimetype":  "text/plain",
		"datas":     []byte("static"),
	})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Partner Label",
		"type":              "ir.actions.report",
		"model":             "res.partner",
		"report_name":       "x.partner_label",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Partner - %s' % (object.name)",
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":                "Partner Report Template",
		"model":               "res.partner",
		"subject":             "Report {{ name }}",
		"body_html":           "<p>Report</p>",
		"email_to":            "{{ email }}",
		"attachment_ids":      []int64{staticAttachmentID},
		"report_template_ids": []int64{reportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("mail_message_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 {
		t.Fatalf("mails = %+v", mails)
	}
	messageID := mails[0]["mail_message_id"].(int64)
	attachmentIDs := mails[0]["attachment_ids"].([]int64)
	if len(attachmentIDs) != 2 || !containsID(attachmentIDs, staticAttachmentID) {
		t.Fatalf("mail attachment ids = %+v static=%d", attachmentIDs, staticAttachmentID)
	}
	reportAttachmentID := int64(0)
	for _, attachmentID := range attachmentIDs {
		if attachmentID != staticAttachmentID {
			reportAttachmentID = attachmentID
		}
	}
	rows, err := env.Model("ir.attachment").Browse(reportAttachmentID).Read("name", "res_model", "res_id", "mimetype", "datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Partner - Report Partner.pdf" || rows[0]["res_model"] != "mail.message" || rows[0]["res_id"] != messageID || rows[0]["mimetype"] != "application/pdf" {
		t.Fatalf("report attachment = %+v", rows)
	}
	if data, ok := rows[0]["datas"].([]byte); !ok || !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("report attachment data = %#v", rows[0]["datas"])
	}
}

func TestSendTemplateBatchCachesDynamicReportAttachment(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Cache Partner", "email": "cache@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Cached Label",
		"type":              "ir.actions.report",
		"model":             "res.partner",
		"report_name":       "x.cached_label",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Outgoing - %s' % (object.name)",
		"attachment":        "'Cache - %s.pdf' % (object.name)",
		"attachment_use":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":                "Cached Report Template",
		"model":               "res.partner",
		"subject":             "Cached {{ name }}",
		"body_html":           "<p>Cached</p>",
		"email_to":            "{{ email }}",
		"report_template_ids": []int64{reportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstMailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	firstAttachmentID := onlyMailAttachmentID(t, env, firstMailIDs[0])
	firstRows, err := env.Model("ir.attachment").Browse(firstAttachmentID).Read("name", "res_model", "res_id", "datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstRows) != 1 || firstRows[0]["name"] != "Outgoing - Cache Partner.pdf" || firstRows[0]["res_model"] != "mail.message" || int64FromAny(firstRows[0]["res_id"]) == 0 {
		t.Fatalf("first outgoing attachment = %+v", firstRows)
	}
	cacheFound, err := env.Model("ir.attachment").Search(domain.And(
		domain.Cond("name", domain.Equal, "Cache - Cache Partner.pdf"),
		domain.Cond("res_model", domain.Equal, "res.partner"),
		domain.Cond("res_id", domain.Equal, partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	cacheIDs := cacheFound.IDs()
	if len(cacheIDs) != 1 {
		t.Fatalf("cache ids = %+v", cacheIDs)
	}
	if err := env.Model("ir.attachment").Browse(cacheIDs[0]).Write(map[string]any{"datas": []byte("%PDF-cached-version"), "file_size": len("%PDF-cached-version")}); err != nil {
		t.Fatal(err)
	}

	secondMailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	secondAttachmentID := onlyMailAttachmentID(t, env, secondMailIDs[0])
	secondRows, err := env.Model("ir.attachment").Browse(secondAttachmentID).Read("name", "res_model", "datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(secondRows) != 1 || secondRows[0]["name"] != "Outgoing - Cache Partner.pdf" || secondRows[0]["res_model"] != "mail.message" || string(secondRows[0]["datas"].([]byte)) != "%PDF-cached-version" {
		t.Fatalf("second outgoing attachment = %+v", secondRows)
	}
	cacheFound, err = env.Model("ir.attachment").Search(domain.And(
		domain.Cond("name", domain.Equal, "Cache - Cache Partner.pdf"),
		domain.Cond("res_model", domain.Equal, "res.partner"),
		domain.Cond("res_id", domain.Equal, partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if cacheIDs = cacheFound.IDs(); len(cacheIDs) != 1 {
		t.Fatalf("cache ids after reuse = %+v", cacheIDs)
	}
}

func TestSendTemplateBatchDoesNotReuseReportCacheWhenAttachmentUseFalse(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Fresh Partner", "email": "fresh@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	cacheID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "Fresh Cache.pdf",
		"res_model": "res.partner",
		"res_id":    partnerID,
		"type":      "binary",
		"mimetype":  "application/pdf",
		"datas":     []byte("%PDF-stale"),
		"file_size": len("%PDF-stale"),
	})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Fresh Label",
		"type":              "ir.actions.report",
		"model":             "res.partner",
		"report_name":       "x.fresh_label",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Fresh - %s' % (object.name)",
		"attachment":        "'Fresh Cache.pdf'",
		"attachment_use":    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":                "Fresh Report Template",
		"model":               "res.partner",
		"subject":             "Fresh {{ name }}",
		"body_html":           "<p>Fresh</p>",
		"email_to":            "{{ email }}",
		"report_template_ids": []int64{reportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID := onlyMailAttachmentID(t, env, mailIDs[0])
	rows, err := env.Model("ir.attachment").Browse(attachmentID).Read("datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || string(rows[0]["datas"].([]byte)) == "%PDF-stale" {
		t.Fatalf("fresh outgoing attachment reused stale cache = %+v", rows)
	}
	cacheRows, err := env.Model("ir.attachment").Browse(cacheID).Read("datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheRows) != 1 || string(cacheRows[0]["datas"].([]byte)) != "%PDF-stale" {
		t.Fatalf("cache was overwritten = %+v", cacheRows)
	}
}

func TestSendTemplateBatchAddsAccountMoveUBLPostprocessAttachment(t *testing.T) {
	env, _ := threadEnv(t)
	moveID, err := env.Model("account.move").Create(map[string]any{"name": "INV/UBL"})
	if err != nil {
		t.Fatal(err)
	}
	xmlID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "INV-UBL.xml",
		"res_model": "account.move",
		"res_field": "ubl_cii_xml_file",
		"res_id":    moveID,
		"type":      "binary",
		"mimetype":  "application/xml",
		"datas":     []byte("<Invoice/>"),
		"file_size": len("<Invoice/>"),
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Invoice UBL Template",
		"model":     "account.move",
		"subject":   "Invoice {{ name }}",
		"body_html": "<p>{{ object.name }}</p>",
		"email_to":  "customer@example.com",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{moveID}})
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs...).Read("mail_message_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	if got := int64s(mailRows[0]["attachment_ids"]); len(got) != 1 || got[0] != xmlID {
		t.Fatalf("mail attachment ids = %#v", mailRows[0]["attachment_ids"])
	}
	messageRows, err := env.Model("mail.message").Browse(int64FromAny(mailRows[0]["mail_message_id"])).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := int64s(messageRows[0]["attachment_ids"]); len(got) != 1 || got[0] != xmlID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}
}

func TestSendTemplateBatchAddsAccountPaymentCFDIPostprocessAttachment(t *testing.T) {
	env, _ := threadEnv(t)
	cfdiID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "PAY-CFDI.xml",
		"res_model": "account.payment",
		"res_id":    int64(0),
		"type":      "binary",
		"mimetype":  "application/xml",
		"datas":     []byte("<cfdi/>"),
		"file_size": len("<cfdi/>"),
	})
	if err != nil {
		t.Fatal(err)
	}
	paymentID, err := env.Model("account.payment").Create(map[string]any{"name": "PAY/CFDI", "l10n_mx_edi_cfdi_attachment_id": cfdiID})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Payment CFDI Template",
		"model":     "account.payment",
		"subject":   "Payment {{ name }}",
		"body_html": "<p>{{ object.name }}</p>",
		"email_to":  "customer@example.com",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{TemplateID: templateID, ResIDs: []int64{paymentID}})
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs...).Read("mail_message_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	if got := int64s(mailRows[0]["attachment_ids"]); len(got) != 1 || got[0] != cfdiID {
		t.Fatalf("mail attachment ids = %#v", mailRows[0]["attachment_ids"])
	}
	messageRows, err := env.Model("mail.message").Browse(int64FromAny(mailRows[0]["mail_message_id"])).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := int64s(messageRows[0]["attachment_ids"]); len(got) != 1 || got[0] != cfdiID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}
}

func onlyMailAttachmentID(t *testing.T, env *record.Env, mailID int64) int64 {
	t.Helper()
	rows, err := env.Model("mail.mail").Browse(mailID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("mail rows = %+v", rows)
	}
	attachmentIDs := int64s(rows[0]["attachment_ids"])
	if len(attachmentIDs) != 1 {
		t.Fatalf("mail attachment ids = %+v", attachmentIDs)
	}
	return attachmentIDs[0]
}

func TestSendTemplateBatchKeepsPartnerRecipientsOutOfRawTo(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Recipient", "email": "recipient@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Partner Only",
		"model":     "res.partner",
		"subject":   "Hello",
		"body_html": "<p>Hello</p>",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendTemplateBatch(env, TemplateSendRequest{
		TemplateID:  templateID,
		ResIDs:      []int64{partnerID},
		EmailValues: map[string]any{"partner_ids": []int64{partnerID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "recipient_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || mails[0]["email_to"] != "" {
		t.Fatalf("mail rows = %+v", mails)
	}
	recipients := mails[0]["recipient_ids"].([]int64)
	if len(recipients) != 1 || recipients[0] != partnerID {
		t.Fatalf("recipient_ids = %+v", recipients)
	}
}

func TestScheduleComposeMessagesCreatesScheduledRowOnly(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Scheduled Target", "email": "scheduled@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":                      "res.partner",
		"res_id":                     partnerID,
		"res_ids":                    []int64{partnerID},
		"subject":                    "Delayed",
		"body":                       "<p>Later</p>",
		"email_to":                   "scheduled@example.com",
		"scheduled_date":             "2026-07-01 10:00:00",
		"composition_comment_option": "log",
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduledIDs, err := ScheduleComposeMessages(env, []int64{wizardID}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduledIDs) != 1 {
		t.Fatalf("scheduled ids = %+v", scheduledIDs)
	}
	rows, err := env.Model("mail.scheduled.message").Browse(scheduledIDs...).Read("mail_message_id", "mail_mail_id", "model", "res_id", "subject", "body", "scheduled_date", "composition_comment_option", "is_note", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["mail_message_id"] != int64(0) ||
		rows[0]["mail_mail_id"] != int64(0) ||
		rows[0]["model"] != "res.partner" ||
		rows[0]["res_id"] != partnerID ||
		rows[0]["subject"] != "Delayed" ||
		rows[0]["body"] != "<p>Later</p>" ||
		rows[0]["composition_comment_option"] != "log" ||
		rows[0]["is_note"] != true ||
		rows[0]["state"] != "scheduled" {
		t.Fatalf("scheduled row = %+v", rows)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("mail rows = %d", mails.Len())
	}
	messages, err := env.Model("mail.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("message rows = %d", messages.Len())
	}
}

func TestScheduleComposeMessagesCopiesAttachmentsPerTarget(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Schedule One", "email": "one@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Schedule Two", "email": "two@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "schedule.txt", "res_model": "mail.compose.message", "type": "binary", "datas": []byte("schedule")})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":                      "res.partner",
		"res_ids":                    []int64{firstID, secondID},
		"subject":                    "Delayed",
		"body":                       "<p>Later</p>",
		"email_to":                   "target@example.com",
		"attachment_ids":             []int64{attachmentID},
		"scheduled_date":             "2026-07-01 10:00:00",
		"composition_comment_option": "log",
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduledIDs, err := ScheduleComposeMessages(env, []int64{wizardID}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduledIDs) != 2 {
		t.Fatalf("scheduled ids = %+v", scheduledIDs)
	}
	rows, err := env.Model("mail.scheduled.message").Browse(scheduledIDs...).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, row := range rows {
		ids := row["attachment_ids"].([]int64)
		if len(ids) != 1 || ids[0] == attachmentID || seen[ids[0]] {
			t.Fatalf("scheduled attachment ids = %+v original=%d seen=%+v", rows, attachmentID, seen)
		}
		seen[ids[0]] = true
		attachmentRows, err := env.Model("ir.attachment").Browse(ids[0]).Read("res_model", "res_id")
		if err != nil {
			t.Fatal(err)
		}
		if attachmentRows[0]["res_model"] != "mail.scheduled.message" || int64FromAny(attachmentRows[0]["res_id"]) == 0 {
			t.Fatalf("scheduled attachment owner = %+v", attachmentRows[0])
		}
	}
}

func TestSendDirectComposeMassMailingInvalidRecipientCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Direct Invalid"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Direct Invalid", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":           "res.partner",
		"res_id":          partnerID,
		"res_ids":         []int64{partnerID},
		"subject":         "Direct Invalid",
		"body":            "<p>Direct Invalid</p>",
		"email_to":        "bad@",
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 0 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Search(domain.Cond("mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 0 {
		t.Fatalf("direct invalid generated mail rows: %d", mails.Len())
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("subject", "=", "Direct Invalid"))
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("direct invalid generated messages: %d", messages.Len())
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		traceRows[0]["email"] != "bad@" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_email_invalid" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestScheduleComposeMessagesMassMailingInvalidRecipientCreatesCanceledTraceOnly(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Scheduled Invalid"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Scheduled Invalid", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":           "res.partner",
		"res_id":          partnerID,
		"res_ids":         []int64{partnerID},
		"subject":         "Scheduled Invalid",
		"body":            "<p>Scheduled Invalid</p>",
		"email_to":        "bad@",
		"scheduled_date":  "2026-07-01 10:00:00",
		"mass_mailing_id": mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduledIDs, err := ScheduleComposeMessages(env, []int64{wizardID}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduledIDs) != 0 {
		t.Fatalf("scheduled ids = %+v", scheduledIDs)
	}
	scheduled, err := env.Model("mail.scheduled.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.Len() != 0 {
		t.Fatalf("scheduled rows = %d", scheduled.Len())
	}
	traces, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traces.Read("mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "trace_status", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		int64FromAny(traceRows[0]["mail_mail_id"]) != 0 ||
		int64FromAny(traceRows[0]["mail_mail_id_int"]) != 0 ||
		traceRows[0]["email"] != "bad@" ||
		traceRows[0]["model"] != "res.partner" ||
		traceRows[0]["res_id"] != partnerID ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != "mail_email_invalid" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestSendDirectComposeKeepsPartnerRecipientsOutOfRawTo(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Direct", "email": "direct@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":       "res.partner",
		"res_id":      partnerID,
		"res_ids":     []int64{partnerID},
		"subject":     "Direct",
		"body":        "<p>Direct</p>",
		"partner_ids": []int64{partnerID},
		"notify":      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "recipient_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || mails[0]["email_to"] != "" {
		t.Fatalf("mails = %+v", mails)
	}
	recipients := mails[0]["recipient_ids"].([]int64)
	if len(recipients) != 1 || recipients[0] != partnerID {
		t.Fatalf("recipient_ids = %+v", recipients)
	}
}

func TestSendDirectComposeCreatesMassMailingTrace(t *testing.T) {
	env, _ := threadEnv(t)
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Direct Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Direct Trace", "email": "direct.trace@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":           "res.partner",
		"res_ids":         []int64{partnerID},
		"subject":         "Direct Trace",
		"body":            "<p>Direct Trace</p>",
		"email_to":        "Direct.Trace@Example.com",
		"mass_mailing_id": mailingID,
		"notify":          true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mailRows, err := env.Model("mail.mail").Browse(mailIDs[0]).Read("mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["mailing_id"] != mailingID {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	traceSearch, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id", "=", mailIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traceSearch.Read("email", "model", "res_id", "mass_mailing_id", "trace_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["email"] != "direct.trace@example.com" || traceRows[0]["model"] != "res.partner" || traceRows[0]["res_id"] != partnerID || traceRows[0]["mass_mailing_id"] != mailingID || traceRows[0]["trace_status"] != "outgoing" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestScheduleComposeMessagesGeneratesTemplateReportPerTarget(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Scheduled One", "email": "one@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Scheduled Two", "email": "two@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Scheduled Label",
		"type":              "ir.actions.report",
		"model":             "res.partner",
		"report_name":       "x.scheduled_label",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Scheduled - %s' % (object.name)",
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":                "Scheduled Report Template",
		"model":               "res.partner",
		"subject":             "Scheduled {{ name }}",
		"body_html":           "<p>{{ email }}</p>",
		"email_to":            "{{ email }}",
		"report_template_ids": []int64{reportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":          "res.partner",
		"res_ids":        []int64{firstID, secondID},
		"template_id":    templateID,
		"scheduled_date": "2026-07-01 10:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduledIDs, err := ScheduleComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduledIDs) != 2 {
		t.Fatalf("scheduled ids = %+v", scheduledIDs)
	}
	rows, err := env.Model("mail.scheduled.message").Browse(scheduledIDs...).Read("id", "res_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	seenAttachments := map[int64]bool{}
	for _, row := range rows {
		ids := row["attachment_ids"].([]int64)
		if len(ids) != 1 || seenAttachments[ids[0]] {
			t.Fatalf("scheduled attachments = %+v", rows)
		}
		seenAttachments[ids[0]] = true
		attachmentRows, err := env.Model("ir.attachment").Browse(ids[0]).Read("name", "res_model", "res_id")
		if err != nil {
			t.Fatal(err)
		}
		if len(attachmentRows) != 1 || attachmentRows[0]["res_model"] != "mail.scheduled.message" || attachmentRows[0]["res_id"] != row["id"] {
			t.Fatalf("scheduled report attachment = %+v scheduled=%+v", attachmentRows, row)
		}
	}
}

func TestSendDirectComposeCopiesAttachmentsPerTarget(t *testing.T) {
	env, _ := threadEnv(t)
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Direct One", "email": "one@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Direct Two", "email": "two@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "direct.txt", "res_model": "mail.compose.message", "type": "binary", "datas": []byte("direct")})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":          "res.partner",
		"res_ids":        []int64{firstID, secondID},
		"subject":        "Direct",
		"body":           "<p>Direct</p>",
		"email_to":       "target@example.com",
		"attachment_ids": []int64{attachmentID},
		"notify":         true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mailIDs, err := SendComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 2 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("mail_message_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, mail := range mails {
		ids := mail["attachment_ids"].([]int64)
		if len(ids) != 1 || ids[0] == attachmentID || seen[ids[0]] {
			t.Fatalf("mail attachment ids = %+v original=%d seen=%+v", mails, attachmentID, seen)
		}
		seen[ids[0]] = true
		messageID := mail["mail_message_id"].(int64)
		attachmentRows, err := env.Model("ir.attachment").Browse(ids[0]).Read("res_model", "res_id")
		if err != nil {
			t.Fatal(err)
		}
		if attachmentRows[0]["res_model"] != "mail.message" || attachmentRows[0]["res_id"] != messageID {
			t.Fatalf("mail attachment owner = %+v message=%d", attachmentRows[0], messageID)
		}
	}
}

func TestComposeDefaultGetAndSend(t *testing.T) {
	env, _ := threadEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Compose", "email": "compose@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":      "compose-template.txt",
		"res_model": "mail.template",
		"type":      "binary",
		"mimetype":  "text/plain",
		"datas":     []byte("compose template attachment"),
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":           "Compose Template",
		"model":          "res.partner",
		"subject":        "Compose {{ name }}",
		"body_html":      "<p>{{ email }}</p>",
		"email_to":       "{{ email }}",
		"attachment_ids": []int64{attachmentID},
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := ComposeDefaultGet(env, []string{"model", "res_id", "res_ids", "template_id", "subject", "body", "email_to", "attachment_ids", "body_is_html"}, map[string]any{
		"active_model":        "res.partner",
		"active_ids":          []int64{partnerID},
		"default_template_id": templateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaults["subject"] != "Compose Compose" || defaults["email_to"] != "compose@example.com" || defaults["body_is_html"] != true {
		t.Fatalf("defaults = %+v", defaults)
	}
	if got := defaults["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("default attachment ids = %#v", defaults["attachment_ids"])
	}
	wizardID, err := env.Model("mail.compose.message").Create(defaults)
	if err != nil {
		t.Fatal(err)
	}
	mailIDs, err := SendComposeMessages(env, []int64{wizardID}, time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v", mailIDs)
	}
	mails, err := env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "subject", "body_html", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || mails[0]["email_to"] != "compose@example.com" || mails[0]["subject"] != "Compose Compose" || mails[0]["body_html"] != "<p>compose@example.com</p>" {
		t.Fatalf("mails = %+v", mails)
	}
	if got := mails[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] == attachmentID {
		t.Fatalf("mail attachment ids = %#v", mails[0]["attachment_ids"])
	}
}

func threadEnv(t *testing.T) (*record.Env, map[string]data.ExternalID) {
	t.Helper()
	registry := record.NewRegistry()
	for _, m := range base.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.Register(accountMoveTemplatePostprocessTestModel()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(accountPaymentTemplatePostprocessTestModel()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(portalThreadTestModel()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(gatewayThreadTestModel()); err != nil {
		t.Fatal(err)
	}
	for _, m := range projectPortalTestModels() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1})
	ids := map[string]data.ExternalID{}
	if err := data.LoadModelMetadata(env, "base", base.Models(), ids); err != nil {
		t.Fatal(err)
	}
	loader := data.NewLoaderWithExternalIDs(env, "base", ids)
	if err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="mail.mt_note" model="mail.message.subtype">
    <field name="name">Note</field>
    <field name="default" eval="True"/>
    <field name="internal" eval="True"/>
  </record>
  <record id="mail.mt_comment" model="mail.message.subtype">
    <field name="name">Comment</field>
    <field name="default" eval="True"/>
    <field name="internal" eval="False"/>
  </record>
  <record id="mail.mt_activities" model="mail.message.subtype">
    <field name="name">Activities</field>
    <field name="default" eval="True"/>
    <field name="internal" eval="True"/>
  </record>
</odoo>`)); err != nil {
		t.Fatal(err)
	}
	return env, loader.ExternalIDs()
}

func accountMoveTemplatePostprocessTestModel() model.Model {
	m := model.New("account.move", "account_move")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("ubl_cii_xml_file", field.Binary))
	return m
}

func accountPaymentTemplatePostprocessTestModel() model.Model {
	m := model.New("account.payment", "account_payment")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("l10n_mx_edi_cfdi_attachment_id", field.Many2One).WithRelation("ir.attachment"))
	return m
}

func portalThreadTestModel() model.Model {
	m := model.New("portal.thread", "portal_thread")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("parent_id", field.Many2One).WithRelation("portal.thread"))
	m.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	m.AddField(field.New("access_token", field.Char))
	return m
}

func gatewayThreadTestModel() model.Model {
	m := model.New("gateway.thread", "gateway_thread")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("active", field.Bool))
	m.AddField(field.New("email", field.Char))
	m.AddField(field.New("email_normalized", field.Char))
	m.AddField(field.New("description", field.Text))
	m.AddField(field.New("message_count", field.Int))
	m.AddField(field.New("gateway_user_id", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("campaign_id", field.Many2One).WithRelation("utm.campaign"))
	m.AddField(field.New("source_id", field.Many2One).WithRelation("utm.source"))
	m.AddField(field.New("medium_id", field.Many2One).WithRelation("utm.medium"))
	m.AddField(field.New("create_uid", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("create_date", field.DateTime))
	m.AddField(field.New("write_uid", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("write_date", field.DateTime))
	return m
}

func projectPortalTestModels() []model.Model {
	project := model.New("project.project", "project_project")
	project.AddField(field.New("name", field.Char))
	project.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	project.AddField(field.New("access_token", field.Char))
	task := model.New("project.task", "project_task")
	task.AddField(field.New("name", field.Char))
	task.AddField(field.New("project_id", field.Many2One).WithRelation("project.project"))
	task.AddField(field.New("parent_id", field.Many2One).WithRelation("project.task"))
	task.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	task.AddField(field.New("access_token", field.Char))
	return []model.Model{project, task}
}
