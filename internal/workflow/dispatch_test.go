package workflow

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorp/internal/actions"
	"gorp/internal/base"
	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
	"gorp/internal/security"
)

func TestDispatcherApprovalActionButtonPersistsRecordAndLog(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"name":        "Finance",
		"settings_id": settingsID,
		"state":       "finance",
		"sequence":    int64(10),
		"active":      true,
		"condition":   `[["amount_total", ">=", 1000]]`,
	}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Submit",
		"action_type": string(ActionApprove),
		"state_value": "draft",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO001",
		"state":        "draft",
		"amount_total": float64(1500),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || result == nil {
		t.Fatalf("handled=%v result=%+v", handled, result)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "finance" {
		t.Fatalf("state = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_state", "new_state", "approval_button_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["model"] != "purchase.order" || logRows[0]["record_id"] != recordID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "finance" || logRows[0]["approval_button_id"] != buttonID {
		t.Fatalf("approval log = %+v", logRows)
	}
}

func TestDispatcherClassicEmailButtonOpensComposeAndCompletesNextAction(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id":          settingsID,
		"name":                 "Email Approve",
		"action_type":          string(ActionEmail),
		"state_value":          "draft",
		"next_state":           "approved",
		"email_template_id":    int64(99),
		"email_wizard_form_id": int64(123),
		"email_next_action":    string(ActionApprove),
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":  "PO Email",
		"state": "draft",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := result.(map[string]any)
	if !handled || !ok {
		t.Fatalf("compose result handled=%v result=%+v", handled, result)
	}
	ctx := action["context"].(map[string]any)
	if action["res_model"] != "mail.compose.message" ||
		action["view_id"] != int64(123) ||
		ctx["approval_button_id"] != buttonID ||
		ctx["default_template_id"] != int64(99) ||
		ctx["default_model"] != "purchase.order" {
		t.Fatalf("compose action = %+v", action)
	}
	if ids := idsFromAny(ctx["default_res_ids"]); len(ids) != 1 || ids[0] != recordID {
		t.Fatalf("default_res_ids = %+v", ctx["default_res_ids"])
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "draft" {
		t.Fatalf("record changed before compose send = %+v", rows[0])
	}
	composeID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":   "purchase.order",
		"res_ids": []int64{recordID},
	})
	if err != nil {
		t.Fatal(err)
	}
	completeAction, handledComplete, err := (Dispatcher{}).CompleteMailComposeTransition(env, []int64{composeID}, map[string]any{"approval_button_id": buttonID})
	if err != nil {
		t.Fatal(err)
	}
	if !handledComplete {
		t.Fatal("classic mail compose button was not handled")
	}
	if payload := completeAction.(map[string]any); payload["tag"] != "soft_reload" {
		t.Fatalf("complete action = %+v", payload)
	}
	rows, err = env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("record after compose send = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("approval_button_id", "old_state", "new_state")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["approval_button_id"] != buttonID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "approved" {
		t.Fatalf("approval log = %+v", logRows)
	}
}

func TestDispatcherClassicEmailButtonRequiredCommentOpensProcessWizardThenCompose(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id":          settingsID,
		"name":                 "Email Approve With Comment",
		"action_type":          string(ActionEmail),
		"state_value":          "draft",
		"next_state":           "approved",
		"email_wizard_form_id": int64(123),
		"email_next_action":    string(ActionApprove),
		"comment":              string(CommentRequired),
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":  "PO Email Comment",
		"state": "draft",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	})
	if err != nil {
		t.Fatal(err)
	}
	action := result.(map[string]any)
	ctx := action["context"].(map[string]any)
	if !handled || action["res_model"] != ModelProcessWizard || ctx["default_button_id"] != buttonID {
		t.Fatalf("process wizard action = %+v", action)
	}
	wizardID, err := env.Model(ModelProcessWizard).Create(map[string]any{
		"res_model": "purchase.order",
		"res_ids":   []int64{recordID},
		"button_id": buttonID,
		"comment":   "<p>Approved by email</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err = (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  ModelProcessWizard,
		Method: "process",
		Args:   []any{[]any{wizardID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	composeAction := result.(map[string]any)
	composeCtx := composeAction["context"].(map[string]any)
	if !handled || composeAction["res_model"] != "mail.compose.message" || composeCtx["approval_button_id"] != buttonID {
		t.Fatalf("compose action = %+v", composeAction)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "draft" {
		t.Fatalf("record changed before compose send = %+v", rows[0])
	}
}

func TestDispatcherClassicApprovalActivitiesFollowApprovalState(t *testing.T) {
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	activityTypeID := createDispatchApprovalActivityType(t, env)
	deadlineFieldID := createDispatchModelField(t, env, "expected_date", "date")
	settingsID := createDispatchSettings(t, env)
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"name":                       "Finance",
		"settings_id":                settingsID,
		"state":                      "finance",
		"sequence":                   int64(10),
		"user_ids":                   []int64{5},
		"schedule_activity":          true,
		"schedule_activity_field_id": deadlineFieldID,
		"schedule_activity_days":     int64(2),
		"active":                     true,
	}); err != nil {
		t.Fatal(err)
	}
	submitID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Submit",
		"action_type": string(ActionApprove),
		"state_value": "draft",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	approveID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(ActionApprove),
		"state_value": "finance",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Activity",
		"state":         "draft",
		"amount_total":  float64(1500),
		"expected_date": "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, handled, err := (Dispatcher{Now: func() time.Time { return now }}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, submitID},
	}); err != nil || !handled {
		t.Fatalf("submit handled=%v err=%v", handled, err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "approval_user_ids", "approval_activity_date_deadline")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "finance" || rows[0]["approval_activity_date_deadline"] != "2026-07-03" {
		t.Fatalf("record row = %+v", rows[0])
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 1 || got[0] != int64(5) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	activities := dispatchApprovalActivities(t, env, recordID)
	if len(activities) != 1 ||
		activities[0]["activity_type_id"] != activityTypeID ||
		activities[0]["res_model"] != "purchase.order" ||
		activities[0]["res_id"] != recordID ||
		activities[0]["user_id"] != int64(5) ||
		activities[0]["date_deadline"] != "2026-07-03" ||
		activities[0]["summary"] != approvalActivityDefaultSummary ||
		activities[0]["automated"] != true ||
		activities[0]["hide_in_chatter"] != true {
		t.Fatalf("activities = %+v", activities)
	}

	if _, handled, err := (Dispatcher{Now: func() time.Time { return now.Add(time.Hour) }}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, approveID},
	}); err != nil || !handled {
		t.Fatalf("approve handled=%v err=%v", handled, err)
	}
	if activities := dispatchApprovalActivities(t, env, recordID); len(activities) != 0 {
		t.Fatalf("activities after approve = %+v", activities)
	}
	rows, err = env.Model("purchase.order").Browse(recordID).Read("state", "approval_activity_date_deadline")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["approval_activity_date_deadline"] != "" {
		t.Fatalf("approved row = %+v", rows[0])
	}
}

func TestDispatcherClassicDelegatedApproverPersistsLogAttribution(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	departmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Requester", "department_id": departmentID})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("res.users").Create(map[string]any{"login": "classic.delegator", "name": "Classic Delegator", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Classic Delegate", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("res.users").Create(map[string]any{"login": "classic.delegate", "name": "Classic Delegate", "active": true, "partner_id": delegatePartnerID})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Classic Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"name":        "Draft Approval",
		"settings_id": settingsID,
		"state":       "draft",
		"group_ids":   []int64{groupID},
		"user_ids":    []int64{delegatorID},
		"sequence":    int64(10),
		"active":      true,
	}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(ActionApprove),
		"state_value": "draft",
		"visible_to":  "approval",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":        "PO Delegated Classic",
		"state":       "draft",
		"employee_id": employeeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	delegationSvc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	delegationSvc.SetGroupConfig(delegation.GroupConfig{GroupID: groupID, Name: "Classic Approvers", AllowDelegation: true})
	request, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{departmentID},
		Lines:           []delegation.LineInput{{GroupID: groupID, DelegateUserID: delegateID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(request.ID); err != nil {
		t.Fatal(err)
	}
	delegateEnv := env.WithContext(record.Context{
		UserID:     delegateID,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values:     map[string]any{"group_ids": []int64{}},
	})
	dispatcher := Dispatcher{
		Delegations: delegationSvc,
		Now:         func() time.Time { return now },
	}
	engine, err := dispatcher.engineFromEnv(delegateEnv)
	if err != nil {
		t.Fatal(err)
	}
	wrec, err := workflowRecordFromEnv(delegateEnv, "purchase.order", recordID, engine, delegationSvc, now)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt64(wrec.ApprovalUserIDs, delegatorID) || !containsInt64(wrec.ApprovalUserIDs, delegateID) || wrec.DelegationID != request.ID {
		t.Fatalf("delegated workflow record = %+v", wrec)
	}
	if _, handled, err := dispatcher.DispatchCall(context.Background(), delegateEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	}); err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "approval_user_ids", "approval_partner_ids", "user_can_approve")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["user_can_approve"] != true {
		t.Fatalf("record row = %+v", rows[0])
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); !containsInt64(got, delegateID) || !containsInt64(got, delegatorID) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != delegatePartnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("user_id", "delegation_id", "old_state", "new_state")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["user_id"] != delegateID || logRows[0]["delegation_id"] != request.ID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "approved" {
		t.Fatalf("approval log = %+v", logRows)
	}
}

func TestDispatcherClassicDelegationUsesSystemDepartmentResolutionWithRestrictedHR(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	allowedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	childDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed Child", "parent_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	deniedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Denied"})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Requester", "department_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	approvalGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Approval Delegation"})
	if err != nil {
		t.Fatal(err)
	}
	securityGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Runtime Users"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("res.users").Create(map[string]any{"login": "restricted.delegator", "name": "Delegator", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("res.users").Create(map[string]any{"login": "restricted.delegate", "name": "Delegate", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	otherDelegateID, err := env.Model("res.users").Create(map[string]any{"login": "restricted.other", "name": "Other Delegate", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"name":        "Draft Approval",
		"settings_id": settingsID,
		"state":       "draft",
		"group_ids":   []int64{approvalGroupID},
		"user_ids":    []int64{delegatorID},
		"sequence":    int64(10),
		"active":      true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":        "PO Restricted Classic",
		"state":       "draft",
		"employee_id": employeeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	delegationSvc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	delegationSvc.SetGroupConfig(delegation.GroupConfig{GroupID: approvalGroupID, Name: "Approver", AllowDelegation: true, AllowMultipleDelegation: true})
	request, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{childDepartmentID},
		Lines:           []delegation.LineInput{{GroupID: approvalGroupID, DelegateUserID: delegateID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(request.ID); err != nil {
		t.Fatal(err)
	}
	deniedRequest, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{deniedDepartmentID},
		Lines:           []delegation.LineInput{{GroupID: approvalGroupID, DelegateUserID: otherDelegateID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(deniedRequest.ID); err != nil {
		t.Fatal(err)
	}
	dispatcher := Dispatcher{
		Delegations: delegationSvc,
		Now:         func() time.Time { return now },
	}
	wfEngine, err := dispatcher.engineFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(restrictedWorkflowRuntimePolicy(securityGroupID, delegatorID, delegateID, otherDelegateID))
	restrictedEnv := env.WithContext(record.Context{
		UserID:     delegateID,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values:     map[string]any{"group_ids": []int64{securityGroupID}},
	})
	if rows, err := restrictedEnv.Model("hr.employee").Browse(employeeID).Read("department_id"); err == nil && len(rows) > 0 {
		t.Fatalf("restricted HR read succeeded = %+v", rows)
	}
	wrec, err := workflowRecordFromEnv(restrictedEnv, "purchase.order", recordID, wfEngine, delegationSvc, now)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt64(wrec.ApprovalUserIDs, delegatorID) || !containsInt64(wrec.ApprovalUserIDs, delegateID) {
		t.Fatalf("approval_user_ids missing expected delegates = %+v", wrec.ApprovalUserIDs)
	}
	if containsInt64(wrec.ApprovalUserIDs, otherDelegateID) {
		t.Fatalf("denied department delegate included = %+v", wrec.ApprovalUserIDs)
	}
	if wrec.DelegationID != request.ID {
		t.Fatalf("delegation_id = %d, want %d", wrec.DelegationID, request.ID)
	}
}

func TestDispatcherClassicUserPythonCodeSelectsDynamicApprover(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner", "active": true, "partner_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Static Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	groupUserID, err := env.Model("res.users").Create(map[string]any{"login": "static", "name": "Static", "active": true, "groups_id": []int64{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"name":             "Dynamic Draft",
		"settings_id":      settingsID,
		"state":            "draft",
		"group_ids":        []int64{groupID},
		"user_python_code": "result = record.owner_user_id",
		"sequence":         int64(10),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(ActionApprove),
		"state_value": "draft",
		"visible_to":  "approval",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Dynamic Classic",
		"state":         "draft",
		"owner_user_id": ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := (Dispatcher{}).engineFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	wrec, err := workflowRecordFromEnv(env, "purchase.order", recordID, engine, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := wrec.ApprovalUserIDs; len(got) != 1 || got[0] != ownerID || containsInt64(got, groupUserID) {
		t.Fatalf("approval users = %+v", got)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerID, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{}}})
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), ownerEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	}); err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "approval_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("state = %+v", rows[0])
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 1 || got[0] != ownerID {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != ownerPartnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
}

func TestDispatcherSourceActionRunsServerActionAndForwardPersists(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.workflow", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = exec
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture", Kind: actions.KindGo, GoActionName: "capture.workflow"})
	if err != nil {
		t.Fatal(err)
	}
	serverButtonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id":      settingsID,
		"name":             "Run",
		"action_type":      string(ActionServerAction),
		"state_value":      "draft",
		"next_state":       "checked",
		"server_action_id": actionID,
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	forwardButtonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Forward",
		"action_type": string(ActionForward),
		"state_value": "draft",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runRecordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO002", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	forwardRecordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO003", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := Dispatcher{Actions: actionRegistry}
	if _, handled, err := dispatcher.DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{runRecordID}, serverButtonID},
	}); err != nil || !handled {
		t.Fatalf("server action handled=%v err=%v", handled, err)
	}
	if captured.Model != "purchase.order" || captured.RecordID != runRecordID || captured.Trigger != "workflow_button" {
		t.Fatalf("captured exec = %+v", captured)
	}
	rows, err := env.Model("purchase.order").Browse(runRecordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "checked" {
		t.Fatalf("state = %+v", rows[0])
	}

	if _, handled, err := dispatcher.DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{forwardRecordID}, forwardButtonID},
		Kwargs: map[string]any{"target_user_id": int64(9)},
	}); err != nil || !handled {
		t.Fatalf("forward handled=%v err=%v", handled, err)
	}
	forwards, err := env.Model(ModelForward).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	forwardRows, err := forwards.Read("model", "record_id", "user_id", "forwarder_user_id", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(forwardRows) != 1 || forwardRows[0]["record_id"] != forwardRecordID || forwardRows[0]["user_id"] != int64(9) || forwardRows[0]["forwarder_user_id"] != int64(5) || forwardRows[0]["active"] != true {
		t.Fatalf("forwards = %+v", forwardRows)
	}
}

func TestDispatcherClassicButtonRunAsSuperuserBypassesServerActionGroups(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.sudo.workflow", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = exec
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{
		Name:         "Capture Sudo",
		Kind:         actions.KindGo,
		GoActionName: "capture.sudo.workflow",
		GroupIDs:     []int64{99},
	})
	if err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id":      settingsID,
		"name":             "Run Sudo",
		"action_type":      string(ActionServerAction),
		"state_value":      "draft",
		"next_state":       "checked",
		"server_action_id": actionID,
		"run_as_superuser": true,
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO Sudo", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}

	dispatcher := Dispatcher{Actions: actionRegistry}
	if _, handled, err := dispatcher.DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	}); err != nil || !handled {
		t.Fatalf("run_as_superuser handled=%v err=%v", handled, err)
	}
	if captured.UserID != env.Context().UserID || !captured.Sudo || int64FromAny(captured.Metadata["user_id"]) != env.Context().UserID || captured.Metadata["run_as_superuser"] != true || captured.Metadata["sudo"] != true {
		t.Fatalf("captured sudo exec = %+v", captured)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "checked" {
		t.Fatalf("state = %+v", rows[0])
	}
}

func TestDispatcherVotingTypeForcesApproveAction(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"settings_id":               settingsID,
		"name":                      "Committee",
		"state":                     "committee",
		"sequence":                  int64(10),
		"committee":                 true,
		"is_voting":                 true,
		"committee_vote_percentage": float64(60),
		"active":                    true,
	}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve Vote",
		"action_type": string(ActionReject),
		"state_value": "committee",
		"voting_type": "approve",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":              "PO Voting",
		"state":             "committee",
		"approval_user_ids": []int64{env.Context().UserID, 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	}); err != nil || !handled {
		t.Fatalf("voting handled=%v err=%v", handled, err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "committee" {
		t.Fatalf("state = %+v", rows[0])
	}
	votes, err := env.Model(ModelLogVoting).Search(domain.And(domain.Cond("record_id", domain.Equal, recordID)))
	if err != nil {
		t.Fatal(err)
	}
	voteRows, err := votes.Read("vote", "button_id", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(voteRows) != 1 || voteRows[0]["vote"] != "approve" || voteRows[0]["button_id"] != buttonID || voteRows[0]["user_id"] != env.Context().UserID {
		t.Fatalf("vote rows = %+v", voteRows)
	}
}

func TestDispatcherRejectsVotingConfigWithCommitteeLimit(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	if _, err := env.Model(ModelConfig).Create(map[string]any{
		"settings_id":     settingsID,
		"name":            "Invalid Voting",
		"state":           "committee",
		"sequence":        int64(10),
		"committee":       true,
		"committee_limit": int64(1),
		"is_voting":       true,
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(ActionApprove),
		"state_value": "committee",
		"voting_type": "approve",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO Invalid Voting", "state": "committee"})
	if err != nil {
		t.Fatal(err)
	}
	_, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, buttonID},
	})
	if !handled || err == nil || !strings.Contains(err.Error(), "committee_limit cannot be set when voting is enabled") {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}

func TestDispatcherForwardOverridesClassicApproversAndDeactivatesOnApproval(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	originalPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Original", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	forwardPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Forwarded", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	originalUserID, err := env.Model("res.users").Create(map[string]any{"login": "original", "name": "Original", "active": true, "groups_id": []int64{groupID}, "partner_id": originalPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	forwardUserID, err := env.Model("res.users").Create(map[string]any{"login": "forwarded", "name": "Forwarded", "active": true, "partner_id": forwardPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	configID, err := env.Model(ModelConfig).Create(map[string]any{
		"name":        "Draft Approval",
		"settings_id": settingsID,
		"state":       "draft",
		"sequence":    int64(10),
		"group_ids":   []int64{groupID},
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	forwardButtonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Forward",
		"action_type": string(ActionForward),
		"state_value": "draft",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	approveButtonID, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(ActionApprove),
		"state_value": "draft",
		"visible_to":  "approval",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":                 "PO Forward",
		"state":                "draft",
		"approval_user_ids":    []int64{originalUserID},
		"approval_partner_ids": []int64{originalPartnerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, forwardButtonID},
		Kwargs: map[string]any{"target_user_id": forwardUserID},
	}); err != nil || !handled {
		t.Fatalf("forward handled=%v err=%v", handled, err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_forward_user_ids", "approval_partner_ids", "user_can_approve")
	if err != nil {
		t.Fatal(err)
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 1 || got[0] != forwardUserID {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_forward_user_ids"]); len(got) != 1 || got[0] != forwardUserID {
		t.Fatalf("approval_forward_user_ids = %+v", rows[0]["approval_forward_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != forwardPartnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	if rows[0]["user_can_approve"] != false {
		t.Fatalf("user_can_approve = %+v", rows[0]["user_can_approve"])
	}
	forwards, err := env.Model(ModelForward).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	forwardRows, err := forwards.Read("approval_state_id", "user_id", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(forwardRows) != 1 || forwardRows[0]["approval_state_id"] != configID || forwardRows[0]["user_id"] != forwardUserID || forwardRows[0]["active"] != true {
		t.Fatalf("forward rows = %+v", forwardRows)
	}
	forwardEnv := env.WithContext(record.Context{UserID: forwardUserID, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{}}})
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), forwardEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_action_button",
		Args:   []any{[]any{recordID}, approveButtonID},
	}); err != nil || !handled {
		t.Fatalf("approve handled=%v err=%v", handled, err)
	}
	rows, err = env.Model("purchase.order").Browse(recordID).Read("state", "approval_forward_user_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || len(idsFromAny(rows[0]["approval_forward_user_ids"])) != 0 {
		t.Fatalf("approved row = %+v", rows[0])
	}
	forwardRows, err = forwards.Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(forwardRows) != 1 || forwardRows[0]["active"] != false {
		t.Fatalf("forward active rows = %+v", forwardRows)
	}
}

func TestParseContextLiteralEvaluatesPythonDictLiterals(t *testing.T) {
	values := ParseContextLiteral(`{
		'approval_auto_submit': True,
		'default_note': 'hello, world',
		'default_count': 42,
		'default_ratio': 1.5,
		'default_empty': None,
		'default_list': [1, 'two', False],
		'default_tuple': (3, 'four')
	}`)
	if values["approval_auto_submit"] != true ||
		values["default_note"] != "hello, world" ||
		values["default_count"] != int64(42) ||
		values["default_ratio"] != 1.5 ||
		values["default_empty"] != nil {
		t.Fatalf("context values = %+v", values)
	}
	list := values["default_list"].([]any)
	if len(list) != 3 || list[0] != int64(1) || list[1] != "two" || list[2] != false {
		t.Fatalf("default_list = %+v", list)
	}
	tuple := values["default_tuple"].([]any)
	if len(tuple) != 2 || tuple[0] != int64(3) || tuple[1] != "four" {
		t.Fatalf("default_tuple = %+v", tuple)
	}
}

func TestParseContextLiteralStrictRejectsNonLiteralExpressions(t *testing.T) {
	values, err := ParseContextLiteralStrict(`{
		'approval_auto_submit': True,
		'default_note': 'hello, world',
		'default_count': 42,
		'default_ratio': 1.5,
		'default_empty': None,
		'default_list': [1, 'two', False],
		'default_tuple': (3, 'four'),
		'default_nested': {'flag': True}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if values["approval_auto_submit"] != true ||
		values["default_note"] != "hello, world" ||
		values["default_count"] != int64(42) ||
		values["default_ratio"] != 1.5 ||
		values["default_empty"] != nil {
		t.Fatalf("strict context values = %+v", values)
	}
	if nested := values["default_nested"].(map[string]any); nested["flag"] != true {
		t.Fatalf("default_nested = %+v", nested)
	}

	for _, expr := range []string{
		`{'default_company_id': env.company.id}`,
		`{'default_user_id': uid}`,
		`{'default_note': context.get('note')}`,
		`{default_note: 'missing quotes'}`,
	} {
		_, err := ParseContextLiteralStrict(expr)
		if err == nil || !strings.Contains(err.Error(), "context") {
			t.Fatalf("ParseContextLiteralStrict(%s) err = %v", expr, err)
		}
	}
}

func TestDispatcherRunsCreateAutomationsAfterCreate(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured []actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.create", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = append(captured, exec)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Create", Kind: actions.KindGo, GoActionName: "capture.create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelAutomation).Create(map[string]any{
		"settings_id":       settingsID,
		"name":              "On Create",
		"sequence":          int64(10),
		"active":            true,
		"trigger":           string(TriggerOnCreate),
		"filter_domain":     `[["amount_total", ">=", 1000]]`,
		"server_action_ids": []int64{actionID},
	}); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{Actions: actionRegistry}).AutoStartCreateHook())

	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Create Automation",
		"state":        "draft",
		"amount_total": float64(1500),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 ||
		captured[0].Model != "purchase.order" ||
		captured[0].RecordID != recordID ||
		captured[0].Trigger != "on_create" ||
		captured[0].Metadata["active_model"] != "purchase.order" ||
		captured[0].Metadata["active_id"] != recordID ||
		captured[0].Values["amount_total"] != float64(1500) {
		t.Fatalf("captured = %+v", captured)
	}
	if _, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Filtered",
		"state":        "draft",
		"amount_total": float64(10),
	}); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 {
		t.Fatalf("filtered automation captured = %+v", captured)
	}
}

func TestDispatcherRunsStateUpdatedAutomationsAfterStateWrite(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured []actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.state.write", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = append(captured, exec)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture State Write", Kind: actions.KindGo, GoActionName: "capture.state.write"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelAutomation).Create(map[string]any{
		"settings_id":       settingsID,
		"name":              "On State Updated",
		"sequence":          int64(10),
		"active":            true,
		"trigger":           string(TriggerOnStateUpdated),
		"from_states":       []string{"draft"},
		"to_states":         []string{"approved"},
		"server_action_ids": []int64{actionID},
	}); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterWriteHook((Dispatcher{Actions: actionRegistry}).StateUpdatedWriteHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO State Write",
		"state":        "draft",
		"amount_total": float64(100),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.Model("purchase.order").Browse(recordID).Write(map[string]any{"state": "approved"}); err != nil {
		t.Fatal(err)
	}

	if len(captured) != 1 ||
		captured[0].Model != "purchase.order" ||
		captured[0].RecordID != recordID ||
		captured[0].Trigger != string(TriggerOnStateUpdated) ||
		captured[0].Metadata["old_state"] != "draft" ||
		captured[0].Metadata["new_state"] != "approved" ||
		captured[0].Values["state"] != "approved" {
		t.Fatalf("captured = %+v", captured)
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_state", "new_state")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["model"] != "purchase.order" || logRows[0]["record_id"] != recordID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "approved" {
		t.Fatalf("logs = %+v", logRows)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "last_state_update")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["last_state_update"] == nil {
		t.Fatalf("record = %+v", rows[0])
	}
}

func TestDispatcherDoesNotRunStateUpdatedAutomationsAfterNonStateWrite(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured []actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.non.state.write", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = append(captured, exec)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Non State Write", Kind: actions.KindGo, GoActionName: "capture.non.state.write"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelAutomation).Create(map[string]any{
		"settings_id":       settingsID,
		"name":              "On State Updated",
		"sequence":          int64(10),
		"active":            true,
		"trigger":           string(TriggerOnStateUpdated),
		"server_action_ids": []int64{actionID},
	}); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterWriteHook((Dispatcher{Actions: actionRegistry}).StateUpdatedWriteHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Non State Write",
		"state":        "draft",
		"amount_total": float64(100),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.Model("purchase.order").Browse(recordID).Write(map[string]any{"amount_total": float64(200)}); err != nil {
		t.Fatal(err)
	}

	if len(captured) != 0 {
		t.Fatalf("captured = %+v", captured)
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("logs = %+v", logs.IDs())
	}
}

func TestDispatcherUnlinkHookGuardsStateAndCleansApprovalLogs(t *testing.T) {
	env := dispatchEnv(t)
	createDispatchSettings(t, env)
	env.RegisterBeforeUnlinkHook((Dispatcher{}).UnlinkHook())
	approvedID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Approved",
		"state":        "approved",
		"amount_total": float64(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	approvedLogID, err := env.Model(ModelLog).Create(map[string]any{
		"model":     "purchase.order",
		"record_id": approvedID,
		"old_state": "draft",
		"new_state": "approved",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = env.Model("purchase.order").Browse(approvedID).Unlink()
	if err == nil || !strings.Contains(err.Error(), "draft status only") {
		t.Fatalf("approved unlink err = %v", err)
	}
	if rows, err := env.Model("purchase.order").Browse(approvedID).Read("state"); err != nil || len(rows) != 1 || rows[0]["state"] != "approved" {
		t.Fatalf("approved record rows=%+v err=%v", rows, err)
	}
	if logs, err := env.Model(ModelLog).Browse(approvedLogID).Read("record_id"); err != nil || len(logs) != 1 {
		t.Fatalf("approved log rows=%+v err=%v", logs, err)
	}

	draftID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Draft",
		"state":        "draft",
		"amount_total": float64(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelLog).Create(map[string]any{
		"model":     "purchase.order",
		"record_id": draftID,
		"old_state": "draft",
		"new_state": "finance",
	}); err != nil {
		t.Fatal(err)
	}

	if err := env.Model("purchase.order").Browse(draftID).Unlink(); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(draftID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("draft record rows = %+v", rows)
	}
	logs, err := env.Model(ModelLog).Search(domain.And(
		domain.Cond("model", "=", "purchase.order"),
		domain.Cond("record_id", "=", draftID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("draft logs = %+v", logs.IDs())
	}
}

func TestDispatcherAutoSubmitsClassicWorkflowOnCreateContext(t *testing.T) {
	env := dispatchEnv(t)
	settingsID := createDispatchSettings(t, env)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured []actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.auto.submit", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = append(captured, exec)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Auto Submit", Kind: actions.KindGo, GoActionName: "capture.auto.submit"})
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range []AutomationTrigger{TriggerOnSubmit, TriggerOnApprove} {
		if _, err := env.Model(ModelAutomation).Create(map[string]any{
			"settings_id":       settingsID,
			"name":              string(trigger),
			"sequence":          int64(10),
			"active":            true,
			"trigger":           string(trigger),
			"server_action_ids": []int64{actionID},
		}); err != nil {
			t.Fatal(err)
		}
	}
	env.RegisterAfterCreateHook((Dispatcher{Actions: actionRegistry}).AutoStartCreateHook())
	ctx := env.Context()
	ctx.Values = map[string]any{"approval_auto_submit": true}
	submitEnv := env.WithContext(ctx)

	recordID, err := submitEnv.Model("purchase.order").Create(map[string]any{
		"name":  "PO Auto Submit",
		"state": "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("auto-submit state = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_state", "new_state", "approval_button_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 ||
		logRows[0]["model"] != "purchase.order" ||
		logRows[0]["record_id"] != recordID ||
		logRows[0]["old_state"] != "draft" ||
		logRows[0]["new_state"] != "approved" ||
		logRows[0]["approval_button_id"] != int64(0) {
		t.Fatalf("auto-submit logs = %+v", logRows)
	}
	if len(captured) != 2 ||
		captured[0].Trigger != string(TriggerOnSubmit) ||
		captured[1].Trigger != string(TriggerOnApprove) ||
		captured[0].RecordID != recordID ||
		captured[1].RecordID != recordID {
		t.Fatalf("captured triggers = %+v", captured)
	}

	plainID, err := env.Model("purchase.order").Create(map[string]any{
		"name":  "PO Manual Submit",
		"state": "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	plainRows, err := env.Model("purchase.order").Browse(plainID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if plainRows[0]["state"] != "draft" {
		t.Fatalf("plain create state = %+v", plainRows[0])
	}
	logs, err = env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}
}

func TestDispatcherAdvancedTransitionButtonPersistsProcessRecordAndLog(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":           "Pending",
		"workflow_id":    workflowID,
		"type":           string(NodeTypeUser),
		"state":          "pending",
		"wizard_view_id": int64(77),
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO004",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{
		WorkflowID: workflowID,
		Model:      "purchase.order",
		RecordID:   recordID,
		NodeID:     pendingNodeID,
		State:      "pending",
		Active:     true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, transitionID},
	}); err != nil || !handled {
		t.Fatalf("transition handled=%v err=%v", handled, err)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.Active || process.State != "approved" || process.LastTransitionID != transitionID {
		t.Fatalf("process = %+v ok=%v", process, ok)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["workflow_node_id"] != doneNodeID || rows[0]["_workflow_transition_id"] != transitionID {
		t.Fatalf("record row = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_node_id", "new_node_id", "workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["record_id"] != recordID || logRows[0]["old_node_id"] != pendingNodeID || logRows[0]["new_node_id"] != doneNodeID || logRows[0]["workflow_transition_id"] != transitionID {
		t.Fatalf("advanced logs = %+v", logRows)
	}
}

func TestDispatcherAdvancedTransitionRunAsSuperuserBypassesServerActionGroups(t *testing.T) {
	env := dispatchEnv(t)
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.advanced.sudo", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = exec
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{
		Name:         "Advanced Sudo",
		Kind:         actions.KindGo,
		GoActionName: "capture.advanced.sudo",
		GroupIDs:     []int64{99},
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced Sudo",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelNodeAction).Create(map[string]any{
		"node_id":          doneNodeID,
		"server_action_id": actionID,
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":             "Approve as Sudo",
		"node_id":          pendingNodeID,
		"workflow_id":      workflowID,
		"next_node_id":     doneNodeID,
		"run_as_superuser": true,
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO Advanced Sudo",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}

	dispatcher := Dispatcher{Actions: actionRegistry}
	if _, handled, err := dispatcher.DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, transitionID},
	}); err != nil || !handled {
		t.Fatalf("advanced sudo transition handled=%v err=%v", handled, err)
	}
	if captured.UserID != env.Context().UserID || !captured.Sudo || int64FromAny(captured.Metadata["user_id"]) != env.Context().UserID || captured.Metadata["run_as_superuser"] != true || captured.Metadata["sudo"] != true {
		t.Fatalf("captured advanced sudo exec = %+v", captured)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.State != "approved" || process.Active {
		t.Fatalf("advanced sudo process = %+v ok=%v", process, ok)
	}
}

func TestDispatcherProcessEscalationsPersistsProcessRecordAndLog(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 30, 0, 0, time.UTC)
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Escalation",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"escalation":            true,
		"escalation_delay_type": string(DelayMinutes),
		"escalation_delay":      int64(15),
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Escalated",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelNode).Browse(pendingNodeID).Write(map[string]any{"escalation_node_id": doneNodeID}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO Escalate",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{
		WorkflowID:      workflowID,
		Model:           "purchase.order",
		RecordID:        recordID,
		NodeID:          pendingNodeID,
		State:           "pending",
		Active:          true,
		UpdatedAt:       now.Add(-time.Hour),
		EscalationDate:  now.Add(-time.Minute),
		ApprovalUserIDs: []int64{7},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := (Dispatcher{Now: func() time.Time { return now }}).ProcessEscalations(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if result.Due != 1 || result.Applied != 1 || result.Skipped != 0 {
		t.Fatalf("escalation result = %+v", result)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.Active || process.State != "approved" || !process.EscalationDate.IsZero() {
		t.Fatalf("process = %+v ok=%v", process, ok)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["workflow_node_id"] != doneNodeID || rows[0]["_workflow_transition_id"] != int64(0) {
		t.Fatalf("record row = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_node_id", "new_node_id", "workflow_transition_id", "description", "duration_seconds", "duration_hours")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 ||
		logRows[0]["record_id"] != recordID ||
		logRows[0]["old_node_id"] != pendingNodeID ||
		logRows[0]["new_node_id"] != doneNodeID ||
		logRows[0]["workflow_transition_id"] != int64(0) ||
		logRows[0]["description"] != "Workflow escalation" {
		t.Fatalf("escalation logs = %+v", logRows)
	}
	durationSeconds, _ := toFloat(logRows[0]["duration_seconds"])
	durationHours, _ := toFloat(logRows[0]["duration_hours"])
	if durationSeconds != 3600 || durationHours != 1 {
		t.Fatalf("escalation log duration = %+v", logRows[0])
	}
}

func TestDispatcherAutoStartEscalationUsesResourceCalendarWorkingDays(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	calendarID, err := env.Model("resource.calendar").Create(map[string]any{
		"name":               "Weekdays",
		"active":             true,
		"two_weeks_calendar": false,
		"tz":                 "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, weekday := range []string{"0", "1", "2", "3", "4"} {
		if _, err := env.Model("resource.calendar.attendance").Create(map[string]any{
			"name":        "Workday",
			"calendar_id": calendarID,
			"dayofweek":   weekday,
			"hour_from":   float64(9),
			"hour_to":     float64(17),
			"day_period":  "full_day",
			"sequence":    int64(10),
		}); err != nil {
			t.Fatal(err)
		}
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "PO Working Day Escalation",
		"model":     "purchase.order",
		"active":    true,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"escalation":            true,
		"escalation_delay_type": string(DelayDays),
		"escalation_delay":      int64(1),
		"trg_date_calendar_id":  calendarID,
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelNode).Browse(pendingNodeID).Write(map[string]any{"escalation_node_id": doneNodeID}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO Calendar", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := (Dispatcher{Now: func() time.Time { return now }}).AutoStartWorkflowForRecord(env, "purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("workflow not started")
	}
	process, ok, err := NewProcessStore(env).Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 22, 17, 0, 0, 0, time.UTC)
	if !ok || !process.EscalationDate.Equal(want) {
		t.Fatalf("calendar escalation date = %s found=%v want %s", process.EscalationDate, ok, want)
	}
}

func TestDispatcherAdvancedTransitionOpensCommentWizard(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":           "Pending",
		"workflow_id":    workflowID,
		"type":           string(NodeTypeUser),
		"state":          "pending",
		"wizard_view_id": int64(77),
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve With Comment",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"comment":      string(CommentRequired),
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO-COMMENT",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}
	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, transitionID},
	})
	if err != nil || !handled {
		t.Fatalf("transition handled=%v err=%v", handled, err)
	}
	action := result.(map[string]any)
	ctx := action["context"].(map[string]any)
	if action["type"] != "ir.actions.act_window" || action["res_model"] != ModelWorkflowWizard || action["view_id"] != int64(77) ||
		ctx["default_model"] != "purchase.order" ||
		ctx["default_record_id"] != recordID ||
		ctx["default_workflow_transition_id"] != transitionID {
		t.Fatalf("wizard action = %+v", action)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != pendingNodeID || process.State != "pending" || !process.Active {
		t.Fatalf("process changed = %+v ok=%v", process, ok)
	}
}

func TestDispatcherAdvancedTransitionWizardButtonOpensWorkflowWizard(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced Wizard Button",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":           "Pending",
		"workflow_id":    workflowID,
		"type":           string(NodeTypeUser),
		"state":          "pending",
		"button_type":    string(ButtonTypeOne),
		"wizard_view_id": int64(88),
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO-WIZARD-BUTTON",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_wizard",
		Args:   []any{[]any{recordID}, pendingNodeID},
	})
	if err != nil || !handled {
		t.Fatalf("transition wizard handled=%v err=%v", handled, err)
	}
	action := result.(map[string]any)
	ctx := action["context"].(map[string]any)
	if action["type"] != "ir.actions.act_window" ||
		action["res_model"] != ModelWorkflowWizard ||
		action["target"] != "new" ||
		action["view_mode"] != "form" ||
		action["view_id"] != int64(88) ||
		ctx["default_model"] != "purchase.order" ||
		ctx["default_record_id"] != recordID ||
		int64FromAny(ctx["default_workflow_transition_id"]) != 0 ||
		ctx["active_id"] != recordID ||
		ctx["active_model"] != "purchase.order" {
		t.Fatalf("wizard action = %+v", action)
	}
	if ids := idsFromAny(ctx["active_ids"]); len(ids) != 1 || ids[0] != recordID {
		t.Fatalf("active_ids = %+v", ctx["active_ids"])
	}
	defaults, err := WorkflowWizardDefaultGet(env, []string{"workflow_transition_id"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaults["workflow_transition_id"] != transitionID {
		t.Fatalf("wizard defaults = %+v", defaults)
	}
}

func TestDispatcherAdvancedEmailTransitionOpensComposeAndCompletesAfterSend(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":                 "Email Approve",
		"node_id":              pendingNodeID,
		"workflow_id":          workflowID,
		"next_node_id":         doneNodeID,
		"is_email":             true,
		"email_template_id":    int64(99),
		"email_wizard_form_id": int64(123),
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO-EMAIL",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}
	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, transitionID},
	})
	if err != nil || !handled {
		t.Fatalf("transition handled=%v err=%v", handled, err)
	}
	action := result.(map[string]any)
	ctx := action["context"].(map[string]any)
	if action["type"] != "ir.actions.act_window" ||
		action["res_model"] != "mail.compose.message" ||
		action["view_id"] != int64(123) ||
		ctx["default_model"] != "purchase.order" ||
		ctx["default_template_id"] != int64(99) ||
		ctx["workflow_transition_id"] != transitionID {
		t.Fatalf("compose action = %+v", action)
	}
	if got := idsFromAny(ctx["default_res_ids"]); len(got) != 1 || got[0] != recordID {
		t.Fatalf("default_res_ids = %+v", ctx["default_res_ids"])
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != pendingNodeID || process.State != "pending" || !process.Active {
		t.Fatalf("process changed before mail send = %+v ok=%v", process, ok)
	}
	composeID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":   "purchase.order",
		"res_ids": []int64{recordID},
	})
	if err != nil {
		t.Fatal(err)
	}
	completeAction, handledComplete, err := (Dispatcher{}).CompleteMailComposeTransition(env, []int64{composeID}, map[string]any{"workflow_transition_id": transitionID})
	if err != nil {
		t.Fatal(err)
	}
	if !handledComplete {
		t.Fatal("mail compose transition was not handled")
	}
	if payload := completeAction.(map[string]any); payload["tag"] != "soft_reload" {
		t.Fatalf("complete action = %+v", payload)
	}
	process, ok, err = store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.State != "approved" || process.Active || process.LastTransitionID != transitionID {
		t.Fatalf("process after mail send = %+v ok=%v", process, ok)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["workflow_node_id"] != doneNodeID || rows[0]["_workflow_transition_id"] != transitionID {
		t.Fatalf("record row = %+v", rows[0])
	}
}

func TestDispatcherAutoEmailTransitionOpensComposeBeforeAdvancing(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":    "PO Advanced",
		"model":   "purchase.order",
		"active":  true,
		"view_id": int64(555),
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	autoNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Auto Email",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "email_pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	submitTransitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": autoNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	emailTransitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":                 "Email Approve",
		"node_id":              autoNodeID,
		"workflow_id":          workflowID,
		"next_node_id":         doneNodeID,
		"is_email":             true,
		"email_template_id":    int64(99),
		"email_wizard_form_id": int64(123),
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO-AUTO-EMAIL",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}
	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, submitTransitionID},
	})
	if err != nil || !handled {
		t.Fatalf("transition handled=%v err=%v", handled, err)
	}
	action := result.(map[string]any)
	ctx := action["context"].(map[string]any)
	if action["type"] != "ir.actions.act_window" ||
		action["res_model"] != "mail.compose.message" ||
		action["view_id"] != int64(123) ||
		ctx["workflow_transition_id"] != emailTransitionID ||
		ctx["default_template_id"] != int64(99) {
		t.Fatalf("compose action = %+v", action)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != autoNodeID || process.State != "email_pending" || !process.Active || process.LastTransitionID != emailTransitionID {
		t.Fatalf("process before mail send = %+v ok=%v", process, ok)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id", "workflow_view_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "email_pending" || rows[0]["workflow_node_id"] != autoNodeID || rows[0]["_workflow_transition_id"] != emailTransitionID || rows[0]["workflow_view_id"] != int64(555) {
		t.Fatalf("record before send = %+v", rows[0])
	}
	composeID, err := env.Model("mail.compose.message").Create(map[string]any{
		"model":   "purchase.order",
		"res_ids": []int64{recordID},
	})
	if err != nil {
		t.Fatal(err)
	}
	completeAction, handledComplete, err := (Dispatcher{}).CompleteMailComposeTransition(env, []int64{composeID}, map[string]any{"workflow_transition_id": emailTransitionID})
	if err != nil {
		t.Fatal(err)
	}
	if !handledComplete {
		t.Fatal("mail compose transition was not handled")
	}
	if payload := completeAction.(map[string]any); payload["tag"] != "soft_reload" {
		t.Fatalf("complete action = %+v", payload)
	}
	process, ok, err = store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.State != "approved" || process.Active || process.LastTransitionID != emailTransitionID {
		t.Fatalf("process after mail send = %+v ok=%v", process, ok)
	}
}

func TestDispatcherAutoStartsWorkflowOnCreate(t *testing.T) {
	env := dispatchEnv(t)
	skippedWorkflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Manual Only",
		"model":     "purchase.order",
		"sequence":  int64(1),
		"active":    true,
		"on_create": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	skippedNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Skipped",
		"workflow_id": skippedWorkflowID,
		"type":        string(NodeTypeUser),
		"state":       "wrong",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(skippedWorkflowID).Write(map[string]any{"start_node_id": skippedNodeID}); err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Auto Start",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"state":     "",
		"condition": `amount_total >= 1000 and state == "draft"`,
		"view_id":   int64(555),
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Approver", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	groupUserID, err := env.Model("res.users").Create(map[string]any{
		"login":      "approver",
		"name":       "Approver",
		"active":     true,
		"groups_id":  []int64{groupID},
		"partner_id": partnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"responsible_user_ids":  []int64{7},
		"responsible_group_ids": []int64{groupID},
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	approveID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"workflow_id":  workflowID,
		"node_id":      pendingNodeID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())

	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Auto",
		"state":        "draft",
		"amount_total": float64(1500),
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id", "workflow_view_id", "workflow_transition_ids", "approval_user_ids", "approval_partner_ids", "user_can_approve")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "pending" || rows[0]["workflow_node_id"] != pendingNodeID || rows[0]["_workflow_transition_id"] != transitionID || rows[0]["workflow_view_id"] != int64(555) {
		t.Fatalf("record row = %+v", rows[0])
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 2 || !containsInt64(got, groupUserID) || !containsInt64(got, 7) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != partnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	if rows[0]["user_can_approve"] != true {
		t.Fatalf("user_can_approve = %+v", rows[0]["user_can_approve"])
	}
	if got := idsFromAny(rows[0]["workflow_transition_ids"]); len(got) != 1 || got[0] != approveID {
		t.Fatalf("workflow_transition_ids = %+v", rows[0]["workflow_transition_ids"])
	}
	processes, err := env.Model(ModelProcess).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	processRows, err := processes.Read("workflow_id", "model", "record_id", "node_id", "state", "active", "last_transition_id", "approval_user_ids", "approval_partner_ids", "user_can_approve")
	if err != nil {
		t.Fatal(err)
	}
	if len(processRows) != 1 ||
		processRows[0]["workflow_id"] != workflowID ||
		processRows[0]["model"] != "purchase.order" ||
		processRows[0]["record_id"] != recordID ||
		processRows[0]["node_id"] != pendingNodeID ||
		processRows[0]["state"] != "pending" ||
		processRows[0]["active"] != true ||
		processRows[0]["last_transition_id"] != transitionID {
		t.Fatalf("process rows = %+v", processRows)
	}
	if got := idsFromAny(processRows[0]["approval_user_ids"]); len(got) != 2 || !containsInt64(got, groupUserID) || !containsInt64(got, 7) {
		t.Fatalf("process approval_user_ids = %+v", processRows[0]["approval_user_ids"])
	}
	if got := idsFromAny(processRows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != partnerID {
		t.Fatalf("process approval_partner_ids = %+v", processRows[0]["approval_partner_ids"])
	}
	if processRows[0]["user_can_approve"] != true {
		t.Fatalf("process user_can_approve = %+v", processRows[0]["user_can_approve"])
	}
	unmatchedID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Small",
		"state":        "draft",
		"amount_total": float64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	unmatchedRows, err := env.Model("purchase.order").Browse(unmatchedID).Read("state", "workflow_node_id")
	if err != nil {
		t.Fatal(err)
	}
	if unmatchedRows[0]["state"] != "draft" || unmatchedRows[0]["workflow_node_id"] != nil {
		t.Fatalf("unmatched row = %+v", unmatchedRows[0])
	}
	processes, err = env.Model(ModelProcess).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if processes.Len() != 1 {
		t.Fatalf("process count = %d", processes.Len())
	}
	started, err := (Dispatcher{}).AutoStartWorkflowForRecord(env, "purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if started {
		t.Fatal("duplicate auto-start returned true")
	}
	processes, err = env.Model(ModelProcess).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if processes.Len() != 1 {
		t.Fatalf("duplicate process count = %d", processes.Len())
	}
}

func TestDispatcherAdvancedApprovalActivitiesFollowActiveNode(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	activityTypeID := createDispatchApprovalActivityType(t, env)
	deadlineFieldID := createDispatchModelField(t, env, "expected_date", "date")
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Activity Auto Start",
		"model":     "purchase.order",
		"active":    true,
		"condition": `state == "draft"`,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                       "Pending",
		"workflow_id":                workflowID,
		"type":                       string(NodeTypeUser),
		"state":                      "pending",
		"responsible_user_ids":       []int64{5},
		"schedule_activity":          true,
		"schedule_activity_field_id": deadlineFieldID,
		"schedule_activity_days":     int64(4),
		"active":                     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	approveID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"workflow_id":  workflowID,
		"node_id":      pendingNodeID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := Dispatcher{Now: func() time.Time { return now }}
	env.RegisterAfterCreateHook(dispatcher.AutoStartCreateHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Advanced Activity",
		"state":         "draft",
		"expected_date": "2026-08-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	activities := dispatchApprovalActivities(t, env, recordID)
	if len(activities) != 1 ||
		activities[0]["activity_type_id"] != activityTypeID ||
		activities[0]["user_id"] != int64(5) ||
		activities[0]["date_deadline"] != "2026-08-14" ||
		activities[0]["summary"] != approvalActivityDefaultSummary ||
		activities[0]["automated"] != true ||
		activities[0]["hide_in_chatter"] != true {
		t.Fatalf("activities = %+v", activities)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "approval_activity_date_deadline")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "pending" || rows[0]["workflow_node_id"] != pendingNodeID || rows[0]["approval_activity_date_deadline"] != "2026-08-14" {
		t.Fatalf("record row = %+v", rows[0])
	}

	if _, handled, err := dispatcher.DispatchCall(context.Background(), env, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, approveID},
	}); err != nil || !handled {
		t.Fatalf("transition handled=%v err=%v", handled, err)
	}
	if activities := dispatchApprovalActivities(t, env, recordID); len(activities) != 0 {
		t.Fatalf("activities after transition = %+v", activities)
	}
	rows, err = env.Model("purchase.order").Browse(recordID).Read("state", "approval_activity_date_deadline")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["approval_activity_date_deadline"] != "" {
		t.Fatalf("approved row = %+v", rows[0])
	}
}

func TestDispatcherAutoStartAddsDelegatedApproversForRecordDepartment(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	allowedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	childDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed Child", "parent_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	deniedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Denied"})
	if err != nil {
		t.Fatal(err)
	}
	allowedEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Allowed", "department_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	deniedEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Denied", "department_id": deniedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("res.users").Create(map[string]any{"login": "delegator", "name": "Delegator", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("res.users").Create(map[string]any{"login": "delegate", "name": "Delegate", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherDelegateID, err := env.Model("res.users").Create(map[string]any{"login": "other.delegate", "name": "Other Delegate", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	otherGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Other Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(delegatorID).Write(map[string]any{"groups_id": []int64{groupID}}); err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Delegated Auto Start",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"responsible_user_ids":  []int64{delegatorID},
		"responsible_group_ids": []int64{groupID},
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "done",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	approveTransitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"workflow_id":  workflowID,
		"node_id":      pendingNodeID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	delegationSvc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	delegationSvc.SetGroupConfig(delegation.GroupConfig{GroupID: groupID, Name: "Approver", AllowDelegation: true})
	delegationSvc.SetGroupConfig(delegation.GroupConfig{GroupID: otherGroupID, Name: "Other Approver", AllowDelegation: true})
	req, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{childDepartmentID},
		Lines: []delegation.LineInput{
			{GroupID: groupID, DelegateUserID: delegateID},
			{GroupID: otherGroupID, DelegateUserID: otherDelegateID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{
		Delegations: delegationSvc,
		Now:         func() time.Time { return now },
	}).AutoStartCreateHook())

	allowedRecordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Allowed",
		"state":         "draft",
		"employee_id":   allowedEmployeeID,
		"department_id": deniedDepartmentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	deniedRecordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Denied",
		"state":         "draft",
		"employee_id":   deniedEmployeeID,
		"department_id": allowedDepartmentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(allowedRecordID, deniedRecordID).Read("approval_user_ids")
	if err != nil {
		t.Fatal(err)
	}
	allowedUsers := idsFromAny(rows[0]["approval_user_ids"])
	if !containsInt64(allowedUsers, delegatorID) || !containsInt64(allowedUsers, delegateID) {
		t.Fatalf("allowed approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if containsInt64(allowedUsers, otherDelegateID) {
		t.Fatalf("other group delegate included = %+v", rows[0]["approval_user_ids"])
	}
	deniedUsers := idsFromAny(rows[1]["approval_user_ids"])
	if !containsInt64(deniedUsers, delegatorID) || containsInt64(deniedUsers, delegateID) {
		t.Fatalf("denied approval_user_ids = %+v", rows[1]["approval_user_ids"])
	}
	delegateEnv := env.WithContext(record.Context{
		UserID:     delegateID,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values:     map[string]any{"group_ids": []int64{}},
	})
	dispatcher := Dispatcher{
		Delegations: delegationSvc,
		Now:         func() time.Time { return now },
	}
	if _, handled, err := dispatcher.DispatchCall(context.Background(), delegateEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{allowedRecordID}, approveTransitionID},
	}); err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	transitionLogs, err := env.Model(ModelLog).Search(domain.And(domain.Cond("workflow_transition_id", domain.Equal, approveTransitionID)))
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := transitionLogs.Read("user_id", "delegation_id", "workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["user_id"] != delegateID || logRows[0]["delegation_id"] != req.ID || logRows[0]["workflow_transition_id"] != approveTransitionID {
		t.Fatalf("approval log = %+v", logRows)
	}
}

func TestDispatcherAutoStartDelegationUsesSystemDepartmentResolutionWithRestrictedHR(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	env := dispatchEnv(t)
	allowedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	childDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Allowed Child", "parent_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	deniedDepartmentID, err := env.Model("hr.department").Create(map[string]any{"name": "Denied"})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Requester", "department_id": allowedDepartmentID})
	if err != nil {
		t.Fatal(err)
	}
	approvalGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Approval Delegation"})
	if err != nil {
		t.Fatal(err)
	}
	securityGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Runtime Users"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("res.users").Create(map[string]any{"login": "advanced.delegator", "name": "Delegator", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("res.users").Create(map[string]any{"login": "advanced.delegate", "name": "Delegate", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	otherDelegateID, err := env.Model("res.users").Create(map[string]any{"login": "advanced.other", "name": "Other Delegate", "active": true, "groups_id": []int64{securityGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Restricted Delegated Auto Start",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"responsible_user_ids":  []int64{delegatorID},
		"responsible_group_ids": []int64{approvalGroupID},
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":        "PO Restricted Advanced",
		"state":       "draft",
		"employee_id": employeeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	delegationSvc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	delegationSvc.SetGroupConfig(delegation.GroupConfig{GroupID: approvalGroupID, Name: "Approver", AllowDelegation: true, AllowMultipleDelegation: true})
	request, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{childDepartmentID},
		Lines:           []delegation.LineInput{{GroupID: approvalGroupID, DelegateUserID: delegateID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(request.ID); err != nil {
		t.Fatal(err)
	}
	deniedRequest, err := delegationSvc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: delegatorID,
		DepartmentIDs:   []int64{deniedDepartmentID},
		Lines:           []delegation.LineInput{{GroupID: approvalGroupID, DelegateUserID: otherDelegateID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationSvc.Confirm(deniedRequest.ID); err != nil {
		t.Fatal(err)
	}
	env.WithPolicy(restrictedWorkflowRuntimePolicy(securityGroupID, delegatorID, delegateID, otherDelegateID))
	restrictedEnv := env.WithContext(record.Context{
		UserID:     delegateID,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values:     map[string]any{"group_ids": []int64{securityGroupID}},
	})
	if rows, err := restrictedEnv.Model("hr.employee").Browse(employeeID).Read("department_id"); err == nil && len(rows) > 0 {
		t.Fatalf("restricted HR read succeeded = %+v", rows)
	}
	started, err := (Dispatcher{
		Delegations: delegationSvc,
		Now:         func() time.Time { return now },
	}).AutoStartWorkflowForRecord(restrictedEnv, "purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("auto-start returned false")
	}
	systemEnv := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{1}}})
	rows, err := systemEnv.Model("purchase.order").Browse(recordID).Read("approval_user_ids")
	if err != nil {
		t.Fatal(err)
	}
	approvalUserIDs := idsFromAny(rows[0]["approval_user_ids"])
	if !containsInt64(approvalUserIDs, delegatorID) || !containsInt64(approvalUserIDs, delegateID) {
		t.Fatalf("approval_user_ids missing expected delegates = %+v", rows[0]["approval_user_ids"])
	}
	if containsInt64(approvalUserIDs, otherDelegateID) {
		t.Fatalf("denied department delegate included = %+v", rows[0]["approval_user_ids"])
	}
}

func TestDispatcherAutoStartResponsiblePythonCodeSelectsDynamicApprover(t *testing.T) {
	env := dispatchEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Node Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Node Owner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := env.Model("res.users").Create(map[string]any{"login": "node.owner", "name": "Node Owner", "active": true, "partner_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	groupUserID, err := env.Model("res.users").Create(map[string]any{"login": "node.group", "name": "Node Group", "active": true, "groups_id": []int64{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Dynamic Node",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                    "Pending",
		"workflow_id":             workflowID,
		"type":                    string(NodeTypeUser),
		"state":                   "pending",
		"responsible_group_ids":   []int64{groupID},
		"responsible_python_code": "result = record.owner_user_id",
		"active":                  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "done",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	approveTransitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"workflow_id":  workflowID,
		"node_id":      pendingNodeID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Dynamic Node",
		"state":         "draft",
		"owner_user_id": ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 1 || got[0] != ownerID || containsInt64(got, groupUserID) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != ownerPartnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	groupEnv := env.WithContext(record.Context{UserID: groupUserID, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{groupID}}})
	if _, _, err := (Dispatcher{}).DispatchCall(context.Background(), groupEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, approveTransitionID},
	}); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("group transition err = %v", err)
	}
	ownerEnv := env.WithContext(record.Context{UserID: ownerID, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{}}})
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), ownerEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, approveTransitionID},
	}); err != nil || !handled {
		t.Fatalf("owner handled=%v err=%v", handled, err)
	}
}

func TestDispatcherAutoStartResponsibleValueAndFilterSelectApprovers(t *testing.T) {
	env := dispatchEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Filtered Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	ownerPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner Approver", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	valuePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Value Approver", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := env.Model("res.users").Create(map[string]any{"login": "filter.owner", "name": "Filter Owner", "active": true, "groups_id": []int64{groupID}, "partner_id": ownerPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	valueID, err := env.Model("res.users").Create(map[string]any{"login": "value.approver", "name": "Value Approver", "active": true, "partner_id": valuePartnerID})
	if err != nil {
		t.Fatal(err)
	}
	deniedID, err := env.Model("res.users").Create(map[string]any{"login": "filter.denied", "name": "Filter Denied", "active": true, "groups_id": []int64{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Responsible Value Filter",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"responsible_group_ids": []int64{groupID},
		"responsible_filter":    "user.id == record.owner_user_id",
		"responsible_value":     "[" + strconv.FormatInt(valueID, 10) + "]",
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":          "PO Responsible Value Filter",
		"state":         "draft",
		"owner_user_id": ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	approvalUsers := idsFromAny(rows[0]["approval_user_ids"])
	if len(approvalUsers) != 2 || !containsInt64(approvalUsers, ownerID) || !containsInt64(approvalUsers, valueID) || containsInt64(approvalUsers, deniedID) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	approvalPartners := idsFromAny(rows[0]["approval_partner_ids"])
	if len(approvalPartners) != 2 || !containsInt64(approvalPartners, ownerPartnerID) || !containsInt64(approvalPartners, valuePartnerID) {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
}

func TestDispatcherAutoStartFiltersApproversWithoutRecordReadAccess(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Read Filtered",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"state":     "",
		"condition": `state == "draft"`,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Readable Approvers"})
	if err != nil {
		t.Fatal(err)
	}
	allowedPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Allowed", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	deniedPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Denied", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	allowedUserID, err := env.Model("res.users").Create(map[string]any{"login": "allowed", "name": "Allowed", "active": true, "groups_id": []int64{groupID}, "partner_id": allowedPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	deniedUserID, err := env.Model("res.users").Create(map[string]any{"login": "denied", "name": "Denied", "active": true, "groups_id": []int64{groupID}, "partner_id": deniedPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                  "Pending",
		"workflow_id":           workflowID,
		"type":                  string(NodeTypeUser),
		"state":                 "pending",
		"responsible_group_ids": []int64{groupID},
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Users[allowedUserID] = security.User{ID: allowedUserID, Login: "allowed", Active: true, GroupIDs: []int64{groupID}, CompanyID: 1, CompanyIDs: []int64{1}}
	engine.Users[deniedUserID] = security.User{ID: deniedUserID, Login: "denied", Active: true, GroupIDs: []int64{groupID}, CompanyID: 1, CompanyIDs: []int64{1}}
	engine.ACLs = []security.ACL{{Model: "purchase.order", GroupID: groupID, Active: true, PermRead: true}}
	engine.Rules = []security.Rule{
		{
			Name:     "approver own purchase order",
			Model:    "purchase.order",
			Domain:   domain.Cond("owner_user_id", domain.Equal, "user.id"),
			GroupIDs: []int64{groupID},
			PermRead: true,
			Active:   true,
		},
	}
	env.WithPolicy(engine)
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())
	systemEnv := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{1}}})

	recordID, err := systemEnv.Model("purchase.order").Create(map[string]any{
		"name":          "PO Read Filter",
		"state":         "draft",
		"owner_user_id": allowedUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := systemEnv.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	approvalUsers := idsFromAny(rows[0]["approval_user_ids"])
	if !containsInt64(approvalUsers, allowedUserID) || containsInt64(approvalUsers, deniedUserID) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	approvalPartners := idsFromAny(rows[0]["approval_partner_ids"])
	if !containsInt64(approvalPartners, allowedPartnerID) || containsInt64(approvalPartners, deniedPartnerID) {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
}

func TestDispatcherResponsibleCommitteeExcludesDoneApprovers(t *testing.T) {
	env := dispatchEnv(t)
	donePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Done", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	pendingPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Pending", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	doneUserID, err := env.Model("res.users").Create(map[string]any{"login": "done", "name": "Done", "active": true, "partner_id": donePartnerID})
	if err != nil {
		t.Fatal(err)
	}
	pendingUserID, err := env.Model("res.users").Create(map[string]any{"login": "pending", "name": "Pending", "active": true, "partner_id": pendingPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Committee Auto Start",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"condition": `state == "draft"`,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                        "Committee",
		"workflow_id":                 workflowID,
		"type":                        string(NodeTypeUser),
		"state":                       "pending",
		"responsible_user_ids":        []int64{doneUserID, pendingUserID},
		"responsible_committee":       true,
		"responsible_committee_limit": int64(2),
		"active":                      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Submit",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO Committee", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("process not found")
	}
	if got := idsFromAny(process.ApprovalUserIDs); !containsInt64(got, doneUserID) || !containsInt64(got, pendingUserID) {
		t.Fatalf("initial approval users = %+v", process.ApprovalUserIDs)
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		t.Fatal(err)
	}
	var workflow Workflow
	for _, candidate := range workflows {
		if candidate.ID == workflowID {
			workflow = candidate
			break
		}
	}
	if workflow.ID == 0 {
		t.Fatal("workflow not loaded")
	}
	process.ApprovalDoneUserIDs = []int64{doneUserID}
	if err := persistAdvancedRecordState(env, workflow, process, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_done_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	approvalUsers := idsFromAny(rows[0]["approval_user_ids"])
	if containsInt64(approvalUsers, doneUserID) || !containsInt64(approvalUsers, pendingUserID) {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	doneUsers := idsFromAny(rows[0]["approval_done_user_ids"])
	if !containsInt64(doneUsers, doneUserID) {
		t.Fatalf("approval_done_user_ids = %+v", rows[0]["approval_done_user_ids"])
	}
	approvalPartners := idsFromAny(rows[0]["approval_partner_ids"])
	if containsInt64(approvalPartners, donePartnerID) || !containsInt64(approvalPartners, pendingPartnerID) {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	process, ok, err = store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("process not found after committee persist")
	}
	if containsInt64(process.ApprovalUserIDs, doneUserID) || !containsInt64(process.ApprovalUserIDs, pendingUserID) {
		t.Fatalf("process approval_user_ids = %+v", process.ApprovalUserIDs)
	}
	if !containsInt64(process.ApprovalDoneUserIDs, doneUserID) {
		t.Fatalf("process approval_done_user_ids = %+v", process.ApprovalDoneUserIDs)
	}
}

func TestDispatcherAdvancedForwardOverridesNodeApprovers(t *testing.T) {
	env := dispatchEnv(t)
	originalPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Original", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	forwardPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Forwarded", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	originalUserID, err := env.Model("res.users").Create(map[string]any{"login": "advanced.original", "name": "Original", "active": true, "partner_id": originalPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	forwardUserID, err := env.Model("res.users").Create(map[string]any{"login": "advanced.forwarded", "name": "Forwarded", "active": true, "partner_id": forwardPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Advanced Forward",
		"model":     "purchase.order",
		"sequence":  int64(10),
		"active":    true,
		"condition": `state == "draft"`,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	startNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Start",
		"workflow_id": workflowID,
		"type":        string(NodeTypeAuto),
		"state":       "started",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":                 "Pending",
		"workflow_id":          workflowID,
		"type":                 string(NodeTypeUser),
		"state":                "pending",
		"responsible_user_ids": []int64{originalUserID},
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": startNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"workflow_id":  workflowID,
		"node_id":      startNodeID,
		"next_node_id": pendingNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	approveTransitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Done",
		"workflow_id":  workflowID,
		"node_id":      pendingNodeID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterAfterCreateHook((Dispatcher{}).AutoStartCreateHook())
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "PO Advanced Forward", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelForward).Create(map[string]any{
		"model":             "purchase.order",
		"record_id":         recordID,
		"workflow_node_id":  pendingNodeID,
		"state_value":       "pending",
		"active":            true,
		"user_id":           forwardUserID,
		"forwarder_user_id": originalUserID,
	}); err != nil {
		t.Fatal(err)
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		t.Fatal(err)
	}
	workflow, ok := workflowByID(workflows, workflowID)
	if !ok {
		t.Fatal("workflow not loaded")
	}
	store := NewProcessStore(env)
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.LastTransitionID != transitionID {
		t.Fatalf("process = %+v ok=%v", process, ok)
	}
	if err := persistAdvancedRecordState(env, workflow, process, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("approval_user_ids", "approval_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := idsFromAny(rows[0]["approval_user_ids"]); len(got) != 1 || got[0] != forwardUserID {
		t.Fatalf("approval_user_ids = %+v", rows[0]["approval_user_ids"])
	}
	if got := idsFromAny(rows[0]["approval_partner_ids"]); len(got) != 1 || got[0] != forwardPartnerID {
		t.Fatalf("approval_partner_ids = %+v", rows[0]["approval_partner_ids"])
	}
	process, ok, err = store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(process.ApprovalUserIDs) != 1 || process.ApprovalUserIDs[0] != forwardUserID {
		t.Fatalf("process after forward = %+v ok=%v", process, ok)
	}
	forwardEnv := env.WithContext(record.Context{UserID: forwardUserID, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{}}})
	if _, handled, err := (Dispatcher{}).DispatchCall(context.Background(), forwardEnv, DispatchRequest{
		Model:  "purchase.order",
		Method: "approval_transition_button",
		Args:   []any{[]any{recordID}, approveTransitionID},
	}); err != nil || !handled {
		t.Fatalf("advanced approve handled=%v err=%v", handled, err)
	}
	rows, err = env.Model("purchase.order").Browse(recordID).Read("state", "approval_forward_user_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("advanced approved row = %+v", rows[0])
	}
	forwards, err := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"active_test": false}}).Model(ModelForward).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	forwardRows, err := forwards.Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(forwardRows) != 1 || forwardRows[0]["active"] != false {
		t.Fatalf("advanced forward active rows = %+v", forwardRows)
	}
}

func TestWorkflowViewIDForRecordMatchesPreProcessWorkflow(t *testing.T) {
	env := dispatchEnv(t)
	skippedID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Skipped",
		"model":     "purchase.order",
		"sequence":  int64(1),
		"active":    true,
		"condition": `amount_total > 5000`,
		"view_id":   int64(111),
	})
	if err != nil {
		t.Fatal(err)
	}
	skippedNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Skipped Node",
		"workflow_id": skippedID,
		"type":        string(NodeTypeUser),
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(skippedID).Write(map[string]any{"start_node_id": skippedNodeID}); err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":      "Matched",
		"model":     "purchase.order",
		"sequence":  int64(2),
		"active":    true,
		"condition": `amount_total >= 1000 and state == "draft"`,
		"view_id":   int64(222),
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Matched Node",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": nodeID}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Pre",
		"state":        "draft",
		"amount_total": float64(1500),
	})
	if err != nil {
		t.Fatal(err)
	}
	viewID, err := WorkflowViewIDForRecord(env, "purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if viewID != 222 {
		t.Fatalf("viewID = %d", viewID)
	}
	unmatchedID, err := env.Model("purchase.order").Create(map[string]any{
		"name":         "PO Small",
		"state":        "draft",
		"amount_total": float64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	viewID, err = WorkflowViewIDForRecord(env, "purchase.order", unmatchedID)
	if err != nil {
		t.Fatal(err)
	}
	if viewID != 0 {
		t.Fatalf("unmatched viewID = %d", viewID)
	}
	nodeRecordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO Node",
		"state":            "draft",
		"amount_total":     float64(10),
		"workflow_node_id": nodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	viewID, err = WorkflowViewIDForRecord(env, "purchase.order", nodeRecordID)
	if err != nil {
		t.Fatal(err)
	}
	if viewID != 222 {
		t.Fatalf("node viewID = %d", viewID)
	}
}

func TestDispatcherWorkflowProcessWizardAppliesTransitionAndComment(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO-WIZ",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewProcessStore(env)
	if _, err := store.Save(Process{
		WorkflowID: workflowID,
		Model:      "purchase.order",
		RecordID:   recordID,
		NodeID:     pendingNodeID,
		State:      "pending",
		Active:     true,
	}); err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model(ModelWorkflowWizard).Create(map[string]any{
		"model":                  "purchase.order",
		"record_id":              recordID,
		"workflow_transition_id": transitionID,
		"comment":                "<p>Approved by wizard</p>",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  ModelWorkflowWizard,
		Method: "process",
		Args:   []any{[]any{wizardID}},
	})
	if err != nil || !handled {
		t.Fatalf("wizard handled=%v result=%+v err=%v", handled, result, err)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.Active || process.State != "approved" || process.LastTransitionID != transitionID {
		t.Fatalf("process = %+v ok=%v", process, ok)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id", "_approval_comment")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["workflow_node_id"] != doneNodeID || rows[0]["_workflow_transition_id"] != transitionID || rows[0]["_approval_comment"] != "<p>Approved by wizard</p>" {
		t.Fatalf("record row = %+v", rows[0])
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("description", "record_id", "old_node_id", "new_node_id", "workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 2 ||
		logRows[0]["description"] != "<p>Approved by wizard</p>" ||
		logRows[0]["record_id"] != recordID ||
		logRows[0]["old_node_id"] != pendingNodeID ||
		logRows[0]["new_node_id"] != pendingNodeID ||
		logRows[1]["description"] != "Workflow transition" ||
		logRows[1]["record_id"] != recordID ||
		logRows[1]["workflow_transition_id"] != transitionID {
		t.Fatalf("wizard logs = %+v", logRows)
	}
}

func TestWorkflowWizardDefaultGetHydratesProcessAndAvailableTransitions(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":   "PO Advanced Defaults",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	approveID, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"groups_ids":   []int64{10},
		"comment":      string(CommentRequired),
		"sequence":     int64(20),
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Hidden",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"is_hidden":    true,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Denied",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"groups_ids":   []int64{99},
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelTransition).Create(map[string]any{
		"name":         "Condition False",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"condition":    `amount_total > 1000`,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO Wizard Defaults",
		"state":            "pending",
		"amount_total":     float64(25),
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processID, err := NewProcessStore(env).Save(Process{
		WorkflowID: workflowID,
		Model:      "purchase.order",
		RecordID:   recordID,
		NodeID:     pendingNodeID,
		State:      "pending",
		Active:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	values, err := WorkflowWizardDefaultGet(env, []string{
		"model",
		"record_id",
		"record_name",
		"workflow_process_id",
		"workflow_node_id",
		"workflow_id",
		"workflow_transition_ids",
		"workflow_transition_id",
		"comment_required",
	}, map[string]any{
		"default_model":                  "purchase.order",
		"default_record_id":              recordID,
		"default_workflow_transition_id": approveID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if values["model"] != "purchase.order" ||
		values["record_id"] != recordID ||
		values["record_name"] != "PO Wizard Defaults" ||
		values["workflow_process_id"] != processID ||
		values["workflow_node_id"] != pendingNodeID ||
		values["workflow_id"] != workflowID ||
		values["workflow_transition_id"] != approveID ||
		values["comment_required"] != true {
		t.Fatalf("wizard defaults = %+v", values)
	}
	if ids := idsFromAny(values["workflow_transition_ids"]); len(ids) != 1 || ids[0] != approveID {
		t.Fatalf("workflow_transition_ids = %+v", values["workflow_transition_ids"])
	}
}

func TestStateUpdateWizardDefaultGetHydratesAdvancedFields(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, pendingNodeID, _, recordID := setupStateUpdateAdvancedRecord(t, env)

	values, err := StateUpdateWizardDefaultGet(env, []string{
		"res_model",
		"res_ids",
		"workflow_model",
		"workflow_id",
		"workflow_node_id",
	}, map[string]any{
		"default_res_model": "purchase.order",
		"default_res_ids":   []int64{recordID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if values["res_model"] != "purchase.order" ||
		!boolFromAny(values["workflow_model"]) ||
		values["workflow_id"] != workflowID ||
		values["workflow_node_id"] != pendingNodeID {
		t.Fatalf("state update defaults = %+v", values)
	}
	if ids := idsFromAny(values["res_ids"]); len(ids) != 1 || ids[0] != recordID {
		t.Fatalf("res_ids = %+v", values["res_ids"])
	}

	otherWorkflowID, err := env.Model(ModelWorkflow).Create(map[string]any{"name": "Other", "model": "purchase.order", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherNodeID, err := env.Model(ModelNode).Create(map[string]any{"name": "Other Node", "workflow_id": otherWorkflowID, "type": string(NodeTypeUser), "state": "other", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherRecordID, err := env.Model("purchase.order").Create(map[string]any{"name": "Other PO", "state": "other", "workflow_node_id": otherNodeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProcessStore(env).Save(Process{WorkflowID: otherWorkflowID, Model: "purchase.order", RecordID: otherRecordID, NodeID: otherNodeID, State: "other", Active: true}); err != nil {
		t.Fatal(err)
	}
	values, err = StateUpdateWizardDefaultGet(env, []string{"workflow_model", "workflow_id", "workflow_node_id"}, map[string]any{
		"default_res_model": "purchase.order",
		"default_res_ids":   []int64{recordID, otherRecordID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !boolFromAny(values["workflow_model"]) || values["workflow_id"] != nil || values["workflow_node_id"] != nil {
		t.Fatalf("mixed defaults = %+v", values)
	}
}

func TestStateUpdateWizardOnchangeMirrorsSourceBehavior(t *testing.T) {
	env := dispatchEnv(t)
	_, _, doneNodeID, _ := setupStateUpdateAdvancedRecord(t, env)

	values, err := StateUpdateWizardOnchange(env, map[string]any{"workflow_node_id": doneNodeID}, []string{"workflow_node_id"})
	if err != nil {
		t.Fatal(err)
	}
	if values["state"] != "approved" {
		t.Fatalf("node onchange values = %+v", values)
	}
	values, err = StateUpdateWizardOnchange(env, map[string]any{"workflow_node_id": doneNodeID, "state": "rejected"}, []string{"state"})
	if err != nil {
		t.Fatal(err)
	}
	if values["workflow_node_id"] != nil {
		t.Fatalf("state onchange values = %+v", values)
	}
}

func TestDispatcherStateUpdateWizardAdvancedUpdatesWorkflowNode(t *testing.T) {
	env := dispatchEnv(t)
	workflowID, pendingNodeID, doneNodeID, recordID := setupStateUpdateAdvancedRecord(t, env)
	wizardID, err := env.Model(ModelStateUpdateWizard).Create(map[string]any{
		"res_model":        "purchase.order",
		"res_ids":          []int64{recordID},
		"state":            "approved",
		"workflow_model":   true,
		"workflow_id":      workflowID,
		"workflow_node_id": doneNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  ModelStateUpdateWizard,
		Method: "action_update",
		Args:   []any{[]any{wizardID}},
	})
	if err != nil || !handled {
		t.Fatalf("state update handled=%v result=%+v err=%v", handled, result, err)
	}
	action := result.(map[string]any)
	if action["type"] != "ir.actions.client" || action["tag"] != "soft_reload" {
		t.Fatalf("advanced action result = %+v", result)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state", "workflow_id", "workflow_node_id", "_old_workflow_node_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" ||
		rows[0]["workflow_id"] != workflowID ||
		rows[0]["workflow_node_id"] != doneNodeID ||
		rows[0]["_old_workflow_node_id"] != pendingNodeID {
		t.Fatalf("advanced updated row = %+v", rows[0])
	}
	process, ok, err := NewProcessStore(env).Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.WorkflowID != workflowID || process.NodeID != doneNodeID || process.State != "approved" || !process.Active {
		t.Fatalf("advanced process = %+v ok=%v", process, ok)
	}
}

func TestDispatcherStateUpdateWizardClassicFallbackClosesWindow(t *testing.T) {
	env := dispatchEnv(t)
	recordID, err := env.Model("purchase.order").Create(map[string]any{"name": "Classic PO", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := env.Model(ModelStateUpdateWizard).Create(map[string]any{
		"res_model": "purchase.order",
		"res_ids":   []int64{recordID},
		"state":     "cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err := (Dispatcher{}).DispatchCall(context.Background(), env, DispatchRequest{
		Model:  ModelStateUpdateWizard,
		Method: "action_update",
		Args:   []any{[]any{wizardID}},
	})
	if err != nil || !handled {
		t.Fatalf("classic state update handled=%v result=%+v err=%v", handled, result, err)
	}
	action := result.(map[string]any)
	if action["type"] != "ir.actions.act_window_close" {
		t.Fatalf("classic action result = %+v", result)
	}
	rows, err := env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "cancel" {
		t.Fatalf("classic updated row = %+v", rows[0])
	}
}

func setupStateUpdateAdvancedRecord(t *testing.T, env *record.Env) (int64, int64, int64, int64) {
	t.Helper()
	settingsID, err := env.Model(ModelSettings).Create(map[string]any{
		"name":        "Purchase Advanced",
		"model":       "purchase.order",
		"active":      true,
		"advance":     true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := env.Model(ModelWorkflow).Create(map[string]any{
		"name":                 "PO Advanced State Update",
		"model":                "purchase.order",
		"approval_settings_id": settingsID,
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := env.Model(ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("purchase.order").Create(map[string]any{
		"name":             "PO State Update",
		"state":            "pending",
		"workflow_id":      workflowID,
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProcessStore(env).Save(Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}
	return workflowID, pendingNodeID, doneNodeID, recordID
}

func dispatchEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		switch m.Name {
		case "ir.model.data", "ir.model.fields", "ir.actions.server", "mail.activity", "mail.activity.type", "resource.calendar", "resource.calendar.attendance":
			if err := reg.Register(m); err != nil {
				t.Fatal(err)
			}
		}
	}
	workflowModels := map[string]model.Model{}
	var workflowOrder []string
	addWorkflowModel := func(m model.Model) {
		if existing, ok := workflowModels[m.Name]; ok {
			workflowModels[m.Name] = m.Compose(existing)
			return
		}
		workflowModels[m.Name] = m
		workflowOrder = append(workflowOrder, m.Name)
	}
	for _, m := range Models() {
		addWorkflowModel(m)
	}
	for _, m := range AdvancedExtensionModels() {
		addWorkflowModel(m)
	}
	for _, name := range workflowOrder {
		if err := reg.Register(workflowModels[name]); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range AdvancedModels() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	po := model.New("purchase.order", "purchase_order")
	po.AddField(field.New("name", field.Char))
	po.AddField(field.New("state", field.Selection))
	po.AddField(field.New("amount_total", field.Float))
	po.AddField(field.New("expected_date", field.Date))
	po.AddField(field.New("owner_user_id", field.Many2One).WithRelation("res.users"))
	po.AddField(field.New("employee_id", field.Many2One).WithRelation("hr.employee"))
	po.AddField(field.New("department_id", field.Many2One).WithRelation("hr.department"))
	po.AddField(field.New("last_state_update", field.DateTime))
	po.AddField(field.New("approval_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("approval_done_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("approval_partner_ids", field.Many2Many).WithRelation("res.partner"))
	po.AddField(field.New("user_can_approve", field.Bool))
	po.AddField(field.New("approval_forward_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("approval_activity_date_deadline", field.Date))
	po.AddField(field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow))
	po.AddField(field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode))
	po.AddField(field.New("workflow_transition_ids", field.Many2Many).WithRelation(ModelTransition))
	po.AddField(field.New("workflow_view_id", field.Many2One).WithRelation("ir.ui.view"))
	po.AddField(field.New("_old_workflow_node_id", field.Many2One).WithRelation(ModelNode))
	po.AddField(field.New("_workflow_transition_id", field.Many2One).WithRelation(ModelTransition))
	po.AddField(field.New("_approval_comment", field.Char))
	if err := reg.Register(po); err != nil {
		t.Fatal(err)
	}
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	groups := model.New("res.groups", "res_groups")
	groups.AddField(field.New("name", field.Char))
	if err := reg.Register(groups); err != nil {
		t.Fatal(err)
	}
	users := model.New("res.users", "res_users")
	users.AddField(field.New("login", field.Char))
	users.AddField(field.New("name", field.Char))
	users.AddField(field.New("active", field.Bool))
	users.AddField(field.New("groups_id", field.Many2Many).WithRelation("res.groups"))
	users.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	if err := reg.Register(users); err != nil {
		t.Fatal(err)
	}
	departments := model.New("hr.department", "hr_department")
	departments.AddField(field.New("name", field.Char))
	departments.AddField(field.New("parent_id", field.Many2One).WithRelation("hr.department"))
	if err := reg.Register(departments); err != nil {
		t.Fatal(err)
	}
	employees := model.New("hr.employee", "hr_employee")
	employees.AddField(field.New("name", field.Char))
	employees.AddField(field.New("department_id", field.Many2One).WithRelation("hr.department"))
	if err := reg.Register(employees); err != nil {
		t.Fatal(err)
	}
	compose := model.New("mail.compose.message", "mail_compose_message")
	compose.Transient = true
	compose.AddField(field.New("model", field.Char))
	compose.AddField(field.New("res_id", field.Int))
	compose.AddField(field.New("res_ids", field.Many2Many))
	if err := reg.Register(compose); err != nil {
		t.Fatal(err)
	}
	return record.NewEnv(reg, record.Context{UserID: 5, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{1, 10}}})
}

func createDispatchApprovalActivityType(t *testing.T, env *record.Env) int64 {
	t.Helper()
	id, err := env.Model("mail.activity.type").Create(map[string]any{
		"name":   approvalActivityTypeName,
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
		"res_id":        id,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func createDispatchModelField(t *testing.T, env *record.Env, name string, ttype string) int64 {
	t.Helper()
	id, err := env.Model("ir.model.fields").Create(map[string]any{
		"model": "purchase.order",
		"name":  name,
		"ttype": ttype,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func dispatchApprovalActivities(t *testing.T, env *record.Env, recordID int64) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.activity").Search(domain.And(
		domain.Cond("res_model", "=", "purchase.order"),
		domain.Cond("res_id", "=", recordID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("activity_type_id", "res_model", "res_id", "user_id", "date_deadline", "summary", "state", "automated", "hide_in_chatter")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func restrictedWorkflowRuntimePolicy(securityGroupID int64, userIDs ...int64) *security.Engine {
	engine := security.NewEngine()
	for _, userID := range userIDs {
		engine.Users[userID] = security.User{
			ID:         userID,
			Active:     true,
			GroupIDs:   []int64{securityGroupID},
			CompanyID:  1,
			CompanyIDs: []int64{1},
		}
	}
	for _, modelName := range []string{
		"purchase.order",
		"res.users",
		ModelSettings,
		ModelConfig,
		ModelButton,
		ModelLog,
		ModelForward,
		ModelWorkflow,
		ModelNode,
		ModelTransition,
		ModelNodeAction,
		ModelProcess,
		ModelWorkflowWizard,
	} {
		engine.ACLs = append(engine.ACLs, security.ACL{
			Model:      modelName,
			GroupID:    securityGroupID,
			Active:     true,
			PermRead:   true,
			PermWrite:  true,
			PermCreate: true,
			PermUnlink: true,
		})
	}
	return engine
}

func createDispatchSettings(t *testing.T, env *record.Env) int64 {
	t.Helper()
	id, err := env.Model(ModelSettings).Create(map[string]any{
		"name":            "PO Approval",
		"model":           "purchase.order",
		"active":          true,
		"state_field":     "state",
		"draft_state":     "draft",
		"approved_state":  "approved",
		"rejected_state":  "rejected",
		"cancelled_state": "cancelled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
