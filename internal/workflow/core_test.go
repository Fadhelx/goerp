package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorp/internal/actions"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/registry"
)

type fakeMailer struct {
	requests []EmailRequest
}

func (f *fakeMailer) SendWorkflowEmail(_ context.Context, request EmailRequest) error {
	f.requests = append(f.requests, request)
	return nil
}

type eventMailer struct {
	events *[]string
}

func (f eventMailer) SendWorkflowEmail(_ context.Context, request EmailRequest) error {
	*f.events = append(*f.events, fmt.Sprintf("mail:%d", request.TemplateID))
	return nil
}

func TestMatchDomainSupportsOdooBaseOperators(t *testing.T) {
	row := map[string]any{
		"name":    "Administrator",
		"code":    "ADM-001",
		"state":   "done",
		"score":   int64(10),
		"tag_ids": []int64{4, 7},
		"partner": map[string]any{
			"company_id": int64(2),
		},
	}
	tests := []struct {
		name string
		node domain.Node
	}{
		{"optional false", domain.Cond("missing", domain.OptionalEqual, false)},
		{"nested field", domain.Cond("partner.company_id", domain.Equal, 2)},
		{"slice in", domain.Cond("tag_ids", domain.In, []int{7})},
		{"not in", domain.Cond("state", domain.NotIn, []string{"draft", "cancel"})},
		{"not like", domain.Cond("name", domain.NotLike, "Demo")},
		{"not ilike", domain.Cond("name", domain.NotILike, "demo")},
		{"equal like", domain.Cond("code", domain.EqualLike, "ADM-%")},
		{"not equal like", domain.Cond("code", domain.NotEqualLike, "INV-%")},
		{"equal ilike", domain.Cond("code", domain.EqualILike, "adm-___")},
		{"not equal ilike", domain.Cond("code", domain.NotEqualILike, "inv-%")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := MatchDomain(row, tt.node)
			if err != nil || !ok {
				t.Fatalf("expected match, got %v %v", ok, err)
			}
		})
	}
}

func TestWorkflowTransitionsAndAudit(t *testing.T) {
	engine := NewEngine()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	engine.SetNow(func() time.Time { return now })
	settingsID := engine.AddSettings(Settings{Name: "Invoice Approval", Model: "account.move", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Approve", Action: ActionApprove, GroupIDs: []int64{10}})
	returnID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "approved", Name: "Return", Action: ActionReturn, ReturnState: "draft", CommentRequired: true})
	forwardID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Forward", Action: ActionForward})
	transferID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Transfer", Action: ActionTransfer})
	cancelID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Cancel", Action: ActionCancel})

	user := User{ID: 3, GroupIDs: []int64{10}}
	record := Record{
		Model:           "account.move",
		ID:              42,
		State:           "draft",
		LastStateUpdate: now.Add(-2 * time.Hour),
		Values:          map[string]any{"amount_total": 1200},
		ApprovalUserIDs: []int64{3},
	}
	result, err := engine.RunButton(context.Background(), user, &record, approveID, Input{Comment: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" || result.OldState != "draft" || result.NewState != "approved" {
		t.Fatalf("approve state = %s result=%+v", record.State, result)
	}
	if len(engine.Logs) != 1 || engine.Logs[0].Duration != 2*time.Hour || engine.Logs[0].UserID != user.ID {
		t.Fatalf("logs = %+v", engine.Logs)
	}
	if len(record.DoneUserIDs) != 1 || record.DoneUserIDs[0] != user.ID {
		t.Fatalf("done users = %+v", record.DoneUserIDs)
	}

	if _, err := engine.RunButton(context.Background(), user, &record, returnID, Input{}); !errors.Is(err, ErrCommentRequired) {
		t.Fatalf("expected comment required, got %v", err)
	}
	if _, err := engine.RunButton(context.Background(), user, &record, returnID, Input{Comment: "revise"}); err != nil {
		t.Fatal(err)
	}
	if record.State != "draft" {
		t.Fatalf("return state = %s", record.State)
	}
	if _, err := engine.RunButton(context.Background(), user, &record, forwardID, Input{TargetUserID: 8}); err != nil {
		t.Fatal(err)
	}
	if len(engine.Forwards) != 1 || engine.Forwards[0].UserID != 8 || len(record.ForwardedToUserIDs) != 1 {
		t.Fatalf("forwards = %+v record=%+v", engine.Forwards, record)
	}
	if _, err := engine.RunButton(context.Background(), user, &record, transferID, Input{TargetUserID: 9}); err != nil {
		t.Fatal(err)
	}
	if len(record.ApprovalUserIDs) != 1 || record.ApprovalUserIDs[0] != 9 {
		t.Fatalf("approval users = %+v", record.ApprovalUserIDs)
	}
	if _, err := engine.RunButton(context.Background(), user, &record, cancelID, Input{Comment: "withdraw"}); err != nil {
		t.Fatal(err)
	}
	if record.State != "cancelled" || len(engine.Cancellations) != 1 {
		t.Fatalf("cancelled record=%+v cancellations=%+v", record, engine.Cancellations)
	}
}

func TestApprovalConfigProgression(t *testing.T) {
	engine := NewEngine()
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Submit", Action: ActionApprove, GroupIDs: []int64{10}})
	engine.AddButton(Button{SettingsID: settingsID, StateValue: "finance", Name: "Finance Approve", Action: ActionApprove, GroupIDs: []int64{10}})
	engine.AddConfig(ApprovalConfig{
		SettingsID: settingsID,
		State:      "finance",
		Name:       "Finance",
		Sequence:   10,
		GroupIDs:   []int64{10},
		Condition:  domain.Cond("amount_total", domain.GreaterEqual, 1000),
	})
	engine.AddConfig(ApprovalConfig{
		SettingsID: settingsID,
		State:      "director",
		Name:       "Director",
		Sequence:   20,
		GroupIDs:   []int64{20},
		Condition:  domain.Cond("amount_total", domain.GreaterEqual, 5000),
	})

	user := User{ID: 3, GroupIDs: []int64{10}}
	small := Record{Model: "purchase.order", ID: 41, State: "draft", Values: map[string]any{"amount_total": 750}}
	next, hasConfig, err := engine.NextApprovalState(user, small)
	if err != nil {
		t.Fatal(err)
	}
	if next != "approved" || hasConfig {
		t.Fatalf("small next = %s hasConfig=%v", next, hasConfig)
	}

	record := Record{Model: "purchase.order", ID: 42, State: "draft", Values: map[string]any{"amount_total": 1500}}
	nextConfig, ok, err := engine.NextApprovalConfig(record)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || nextConfig.State != "finance" {
		t.Fatalf("next config = %+v ok=%v", nextConfig, ok)
	}
	result, err := engine.RunButton(context.Background(), user, &record, approveID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "finance" || result.NewState != "finance" {
		t.Fatalf("record=%+v result=%+v", record, result)
	}

	financeButton, ok, err := engine.firstButton(user, record, ActionApprove)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("finance approve button not found")
	}
	result, err = engine.RunButton(context.Background(), user, &record, financeButton.ID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" || result.OldState != "finance" || result.NewState != "approved" {
		t.Fatalf("final record=%+v result=%+v", record, result)
	}
}

func TestApprovalConfigAutoApproveSkipsCurrentUserStep(t *testing.T) {
	engine := NewEngine()
	settingsID := engine.AddSettings(Settings{Name: "Large PO Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Submit", Action: ActionApprove, GroupIDs: []int64{10}})
	engine.AddConfig(ApprovalConfig{
		SettingsID:  settingsID,
		State:       "finance",
		Name:        "Finance",
		Sequence:    10,
		GroupIDs:    []int64{10},
		Condition:   domain.Cond("amount_total", domain.GreaterEqual, 1000),
		AutoApprove: true,
	})
	engine.AddConfig(ApprovalConfig{
		SettingsID: settingsID,
		State:      "director",
		Name:       "Director",
		Sequence:   20,
		GroupIDs:   []int64{20},
		Condition:  domain.Cond("amount_total", domain.GreaterEqual, 5000),
	})

	record := Record{Model: "purchase.order", ID: 43, State: "draft", Values: map[string]any{"amount_total": 6000}}
	result, err := engine.RunButton(context.Background(), User{ID: 3, GroupIDs: []int64{10}}, &record, approveID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "director" || result.NewState != "director" {
		t.Fatalf("auto-approved record=%+v result=%+v", record, result)
	}
}

func TestButtonVisibilityDomainAndMetadata(t *testing.T) {
	engine := NewEngine()
	settingsID := engine.AddSettings(Settings{Name: "Payment Approval", Model: "account.payment", Active: true})
	engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Approve", Action: ActionApprove, GroupIDs: []int64{20}, Sequence: 20})
	engine.AddButton(Button{
		SettingsID:    settingsID,
		StateValue:    "draft",
		Name:          "Manager Approve",
		Action:        ActionApprove,
		GroupIDs:      []int64{30},
		VisibleDomain: domain.Cond("amount", domain.Greater, 1000),
		Sequence:      10,
	})
	record := Record{Model: "account.payment", ID: 7, State: "draft", Values: map[string]any{"amount": 1500}}
	buttons, err := engine.VisibleButtons(User{ID: 1, GroupIDs: []int64{30}}, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(buttons) != 1 || buttons[0].Name != "Manager Approve" {
		t.Fatalf("buttons = %+v", buttons)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 2, GroupIDs: []int64{20}}, &record, buttons[0].ID, Input{}); !errors.Is(err, ErrButtonHidden) {
		t.Fatalf("expected hidden, got %v", err)
	}
	meta, err := engine.Metadata(User{ID: 1, GroupIDs: []int64{30}}, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.VisibleButtons) != 1 || meta.VisibleButtons[0].Sequence != 10 {
		t.Fatalf("metadata = %+v", meta)
	}
}

func TestWorkflowSideEffectsAndAutomations(t *testing.T) {
	engine := NewEngine()
	mailer := &fakeMailer{}
	engine.Mailer = mailer
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	if err := actionRegistry.RegisterGo("mark", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		if exec.Model != "purchase.order" || exec.RecordID != 11 {
			t.Fatalf("exec = %+v", exec)
		}
		return actions.Result{Metadata: map[string]any{"marked": true}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Mark", Kind: actions.KindGo, GoActionName: "mark"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	serverButtonID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Server", Action: ActionServerAction, ServerActionID: actionID, NextState: "checked"})
	emailButtonID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "checked", Name: "Email", Action: ActionEmail, EmailTemplateID: 99})
	methodButtonID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "checked", Name: "Method", Action: ActionMethod, MethodName: "stamp"})
	engine.AddAutomation(Automation{SettingsID: settingsID, Active: true, Trigger: TriggerStateChange, FromStates: []string{"draft"}, ToStates: []string{"checked"}, ServerActionIDs: []int64{actionID}, TemplateIDs: []int64{100}})
	if err := engine.RegisterMethod("stamp", func(_ context.Context, record *Record, _ Button, _ Input) error {
		record.Values = setValue(record.Values, "stamped", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	record := Record{Model: "purchase.order", ID: 11, State: "draft"}
	result, err := engine.RunButton(context.Background(), User{ID: 5}, &record, serverButtonID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "checked" || len(result.ActionResults) != 2 || len(mailer.requests) != 1 {
		t.Fatalf("server result=%+v record=%+v mail=%+v", result, record, mailer.requests)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, emailButtonID, Input{}); err != nil {
		t.Fatal(err)
	}
	if len(mailer.requests) != 2 || mailer.requests[1].TemplateID != 99 {
		t.Fatalf("mail = %+v", mailer.requests)
	}
	emailNextButtonID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "checked", Name: "Email Next", Action: ActionEmail, EmailTemplateID: 101, EmailNextAction: ActionReject, NextState: "rejected"})
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, emailNextButtonID, Input{MailComposed: true}); err != nil {
		t.Fatal(err)
	}
	if record.State != "rejected" || len(mailer.requests) != 2 {
		t.Fatalf("mail-composed email result record=%+v mail=%+v", record, mailer.requests)
	}
	record.State = "checked"
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, methodButtonID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.Values["stamped"] != true {
		t.Fatalf("record values = %+v", record.Values)
	}
}

func TestWorkflowAutomationCodeFailsClosedBeforeSideEffects(t *testing.T) {
	engine := NewEngine()
	mailer := &fakeMailer{}
	engine.Mailer = mailer
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var actionRuns int
	if err := actionRegistry.RegisterGo("capture", func(_ context.Context, _ actions.ServerAction, _ actions.ExecutionContext) (actions.Result, error) {
		actionRuns++
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture", Kind: actions.KindGo, GoActionName: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	engine.AddAutomation(Automation{
		ID:              42,
		SettingsID:      settingsID,
		Active:          true,
		Trigger:         TriggerStateChange,
		Name:            "python code",
		FromStates:      []string{"draft"},
		ToStates:        []string{"checked"},
		Code:            "action = {'type': 'ir.actions.act_window'}",
		ServerActionIDs: []int64{actionID},
		TemplateIDs:     []int64{100},
	})

	record := Record{Model: "purchase.order", ID: 11, State: "checked"}
	_, err = engine.RunStateUpdatedAutomations(context.Background(), User{ID: 5}, record, "draft", Input{})
	if !errors.Is(err, actions.ErrCodeExecutionDisabled) {
		t.Fatalf("expected code disabled error, got %v", err)
	}
	if actionRuns != 0 {
		t.Fatalf("server action runs = %d", actionRuns)
	}
	if len(mailer.requests) != 0 {
		t.Fatalf("mail requests = %+v", mailer.requests)
	}
}

func TestWorkflowAutomationsUseSourceOrderAndSkipFilteredRules(t *testing.T) {
	engine := NewEngine()
	var events []string
	engine.Mailer = eventMailer{events: &events}
	engine.CodeRunner = func(_ context.Context, req AutomationCodeRequest) (actions.Result, bool, error) {
		events = append(events, fmt.Sprintf("code:%d", req.Automation.ID))
		if req.Exec.Metadata["active_model"] != "purchase.order" || req.Exec.Metadata["active_id"] != int64(11) {
			t.Fatalf("exec metadata = %+v", req.Exec.Metadata)
		}
		if !reflect.DeepEqual(req.Exec.Metadata["active_ids"], []int64{11}) {
			t.Fatalf("active_ids = %+v", req.Exec.Metadata["active_ids"])
		}
		if req.Exec.UserID != 5 || !reflect.DeepEqual(req.Exec.UserGroupIDs, []int64{7}) {
			t.Fatalf("exec user = %+v", req.Exec)
		}
		return actions.Result{Kind: actions.KindCode, Metadata: map[string]any{"automation_id": req.Automation.ID}}, true, nil
	}
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	if err := actionRegistry.RegisterGo("capture", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		events = append(events, fmt.Sprintf("action:%d", exec.Metadata["automation_id"]))
		if exec.Metadata["active_model"] != "purchase.order" || exec.Metadata["active_id"] != int64(11) {
			t.Fatalf("exec metadata = %+v", exec.Metadata)
		}
		if !reflect.DeepEqual(exec.Metadata["active_ids"], []int64{11}) {
			t.Fatalf("active_ids = %+v", exec.Metadata["active_ids"])
		}
		return actions.Result{Metadata: map[string]any{"automation_id": exec.Metadata["automation_id"]}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture", Kind: actions.KindGo, GoActionName: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	engine.AddAutomation(Automation{
		ID:              5,
		SettingsID:      settingsID,
		Sequence:        1,
		Active:          true,
		Trigger:         TriggerStateChange,
		Filter:          domain.Cond("amount_total", domain.GreaterEqual, 9999),
		Code:            "action = {'name': 'skip'}",
		ServerActionIDs: []int64{actionID},
		TemplateIDs:     []int64{5},
	})
	engine.AddAutomation(Automation{
		ID:              20,
		SettingsID:      settingsID,
		Sequence:        10,
		Active:          true,
		Trigger:         TriggerStateChange,
		FromStates:      []string{"draft"},
		ToStates:        []string{"checked"},
		Code:            "action = {'name': 'second'}",
		ServerActionIDs: []int64{actionID},
		TemplateIDs:     []int64{20},
	})
	engine.AddAutomation(Automation{
		ID:              10,
		SettingsID:      settingsID,
		Sequence:        10,
		Active:          true,
		Trigger:         TriggerStateChange,
		FromStates:      []string{"draft"},
		ToStates:        []string{"checked"},
		Code:            "action = {'name': 'first'}",
		ServerActionIDs: []int64{actionID},
		TemplateIDs:     []int64{10},
	})

	record := Record{Model: "purchase.order", ID: 11, State: "checked", Values: map[string]any{"amount_total": 500}}
	results, err := engine.RunStateUpdatedAutomations(context.Background(), User{ID: 5, GroupIDs: []int64{7}}, record, "draft", Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("results = %+v", results)
	}
	want := []string{"code:10", "action:10", "mail:10", "code:20", "action:20", "mail:20"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}
}

func TestWorkflowSourceAutomationTriggers(t *testing.T) {
	engine := NewEngine()
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var triggers []string
	if err := actionRegistry.RegisterGo("capture.trigger", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		triggers = append(triggers, exec.Trigger)
		return actions.Result{Metadata: map[string]any{"trigger": exec.Trigger}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Trigger", Kind: actions.KindGo, GoActionName: "capture.trigger"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	submitID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Submit", Action: ActionApprove})
	financeID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "finance", Name: "Finance", Action: ActionApprove})
	rejectID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "approved", Name: "Reject", Action: ActionReject})
	draftID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "rejected", Name: "Draft", Action: ActionDraft})
	cancelWorkflowID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "approved", Name: "Cancel Workflow", Action: ActionCancelWorkflow})
	forwardID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Forward", Action: ActionForward})
	engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "finance", Name: "Finance", Sequence: 10, Condition: domain.Cond("amount_total", domain.GreaterEqual, 1000)})
	for _, trigger := range []AutomationTrigger{
		TriggerOnSubmit,
		TriggerOnEnterApproval,
		TriggerOnApproval,
		TriggerOnApprove,
		TriggerOnReject,
		TriggerOnDraft,
		TriggerOnForward,
		TriggerOnStateUpdated,
		TriggerOnCreate,
	} {
		engine.AddAutomation(Automation{
			SettingsID:      settingsID,
			Active:          true,
			Trigger:         trigger,
			Name:            string(trigger),
			ServerActionIDs: []int64{actionID},
		})
	}
	engine.AddAutomation(Automation{
		SettingsID:      settingsID,
		Active:          true,
		Trigger:         TriggerOnReject,
		Name:            "reject filtered out",
		FromStates:      []string{"draft"},
		ToStates:        []string{"rejected"},
		ServerActionIDs: []int64{actionID},
	})

	record := Record{Model: "purchase.order", ID: 11, State: "draft", Values: map[string]any{"amount_total": 1500}}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, submitID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_submit", "on_enter_approval", "on_state_updated"})
	triggers = nil

	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, financeID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_approval", "on_approve", "on_state_updated"})
	triggers = nil

	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, rejectID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_reject", "on_state_updated"})
	triggers = nil

	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, draftID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_draft", "on_state_updated"})
	triggers = nil

	cancelled := Record{Model: "purchase.order", ID: 14, State: "approved"}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &cancelled, cancelWorkflowID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_state_updated"})
	triggers = nil

	forwardRecord := Record{Model: "purchase.order", ID: 12, State: "draft"}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &forwardRecord, forwardID, Input{TargetUserID: 8}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_forward"})
	triggers = nil

	created := Record{Model: "purchase.order", ID: 13, State: "draft"}
	if _, err := engine.RunCreateAutomations(context.Background(), User{ID: 5}, created, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_create"})
}

func TestWorkflowButtonRunAsSuperuserPropagatesToAutomations(t *testing.T) {
	engine := NewEngine()
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	if err := actionRegistry.RegisterGo("button.noop", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		if exec.UserID != 5 || !exec.Sudo {
			t.Fatalf("button exec = %+v", exec)
		}
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var captured actions.ExecutionContext
	if err := actionRegistry.RegisterGo("automation.sudo", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = exec
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	buttonActionID, err := actionRegistry.Register(actions.ServerAction{Name: "Button", Kind: actions.KindGo, GoActionName: "button.noop"})
	if err != nil {
		t.Fatal(err)
	}
	automationActionID, err := actionRegistry.Register(actions.ServerAction{
		Name:         "Automation Restricted",
		Kind:         actions.KindGo,
		GoActionName: "automation.sudo",
		GroupIDs:     []int64{99},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "PO Approval", Model: "purchase.order", Active: true})
	buttonID := engine.AddButton(Button{
		SettingsID:     settingsID,
		StateValue:     "draft",
		Name:           "Run Sudo",
		Action:         ActionServerAction,
		NextState:      "checked",
		ServerActionID: buttonActionID,
		RunAsSuperuser: true,
	})
	engine.AddAutomation(Automation{
		ID:              42,
		SettingsID:      settingsID,
		Active:          true,
		Trigger:         TriggerStateChange,
		FromStates:      []string{"draft"},
		ToStates:        []string{"checked"},
		ServerActionIDs: []int64{automationActionID},
	})

	record := Record{Model: "purchase.order", ID: 11, State: "draft"}
	if _, err := engine.RunButton(context.Background(), User{ID: 5, GroupIDs: []int64{7}}, &record, buttonID, Input{}); err != nil {
		t.Fatal(err)
	}
	if captured.UserID != 5 || !captured.Sudo || captured.Metadata["run_as_superuser"] != true || captured.Metadata["sudo"] != true {
		t.Fatalf("automation exec = %+v", captured)
	}
}

func TestWorkflowCommitteeApprovalTrigger(t *testing.T) {
	engine := NewEngine()
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var triggers []string
	if err := actionRegistry.RegisterGo("capture.committee", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		triggers = append(triggers, exec.Trigger)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Committee", Kind: actions.KindGo, GoActionName: "capture.committee"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "Committee Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Approve", Action: ActionApprove})
	engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "committee", Name: "Committee", Sequence: 10, Committee: true})
	for _, trigger := range []AutomationTrigger{TriggerOnCommitteeApproval, TriggerOnApproval, TriggerOnApprove, TriggerOnStateUpdated} {
		engine.AddAutomation(Automation{SettingsID: settingsID, Active: true, Trigger: trigger, Name: string(trigger), ServerActionIDs: []int64{actionID}})
	}

	record := Record{Model: "purchase.order", ID: 15, State: "committee"}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, approveID, Input{}); err != nil {
		t.Fatal(err)
	}
	assertTriggers(t, triggers, []string{"on_committee_approval", "on_approval", "on_approve", "on_state_updated"})
}

func TestWorkflowCommitteePartialApprovalHoldsUntilLimit(t *testing.T) {
	engine := NewEngine()
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var triggers []string
	if err := actionRegistry.RegisterGo("capture.committee.partial", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		triggers = append(triggers, exec.Trigger)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Committee Partial", Kind: actions.KindGo, GoActionName: "capture.committee.partial"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "Committee Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Approve", Action: ActionApprove, VisibleTo: "approval"})
	engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "committee", Name: "Committee", Sequence: 10, Committee: true, CommitteeLimit: 2})
	for _, trigger := range []AutomationTrigger{TriggerOnCommitteeApproval, TriggerOnApproval, TriggerOnApprove, TriggerOnStateUpdated} {
		engine.AddAutomation(Automation{SettingsID: settingsID, Active: true, Trigger: trigger, Name: string(trigger), ServerActionIDs: []int64{actionID}})
	}
	record := Record{Model: "purchase.order", ID: 16, State: "committee", ApprovalUserIDs: []int64{5, 6}}
	user5 := User{ID: 5}
	result, err := engine.RunButton(context.Background(), user5, &record, approveID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OldState != "committee" || result.NewState != "committee" || record.State != "committee" {
		t.Fatalf("partial result=%+v record=%+v", result, record)
	}
	if !containsInt64(record.DoneUserIDs, 5) || containsInt64(record.ApprovalUserIDs, 5) || !containsInt64(record.ApprovalUserIDs, 6) {
		t.Fatalf("partial users record=%+v", record)
	}
	buttons, err := engine.VisibleButtons(user5, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(buttons) != 0 {
		t.Fatalf("done user buttons = %+v", buttons)
	}
	if len(engine.Logs) != 1 || engine.Logs[0].OldState != "committee" || engine.Logs[0].NewState != "committee" {
		t.Fatalf("partial logs = %+v", engine.Logs)
	}
	assertTriggers(t, triggers, []string{"on_committee_approval"})
	_, err = engine.RunButton(context.Background(), User{ID: 6}, &record, approveID, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" || len(record.DoneUserIDs) != 0 || len(record.ApprovalUserIDs) != 0 {
		t.Fatalf("completed record = %+v", record)
	}
	if len(engine.Logs) != 2 || engine.Logs[1].OldState != "committee" || engine.Logs[1].NewState != "approved" {
		t.Fatalf("complete logs = %+v", engine.Logs)
	}
	assertTriggers(t, triggers, []string{"on_committee_approval", "on_committee_approval", "on_approval", "on_approve", "on_state_updated"})
}

func TestWorkflowCommitteeVotingRejectsBelowApprovalPercentage(t *testing.T) {
	engine := NewEngine()
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var triggers []string
	if err := actionRegistry.RegisterGo("capture.voting.trigger", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		triggers = append(triggers, exec.Trigger)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Voting Trigger", Kind: actions.KindGo, GoActionName: "capture.voting.trigger"})
	if err != nil {
		t.Fatal(err)
	}
	engine.Actions = actionRegistry
	settingsID := engine.AddSettings(Settings{Name: "Voting Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Approve", Action: ActionApprove, VotingType: "approve"})
	rejectID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Reject", Action: ActionApprove, VotingType: "reject"})
	engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "committee", Name: "Committee", Sequence: 10, Committee: true, IsVoting: true, CommitteeVotePercentage: 60})
	for _, trigger := range []AutomationTrigger{TriggerOnCommitteeApproval, TriggerOnReject, TriggerOnApproval, TriggerOnApprove, TriggerOnStateUpdated} {
		engine.AddAutomation(Automation{
			SettingsID:      settingsID,
			Active:          true,
			Trigger:         trigger,
			Name:            string(trigger),
			ServerActionIDs: []int64{actionID},
		})
	}
	record := Record{Model: "purchase.order", ID: 17, State: "committee", ApprovalUserIDs: []int64{5, 6}}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, approveID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "committee" || len(record.ApprovalUserIDs) != 1 || len(record.DoneUserIDs) != 1 {
		t.Fatalf("partial voting record = %+v", record)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 6}, &record, rejectID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "rejected" || len(record.ApprovalUserIDs) != 0 || len(record.DoneUserIDs) != 0 {
		t.Fatalf("rejected voting record = %+v", record)
	}
	if len(engine.Votes) != 2 || engine.Votes[0].VoteType != "approve" || engine.Votes[1].VoteType != "reject" {
		t.Fatalf("votes = %+v", engine.Votes)
	}
	assertTriggers(t, triggers, []string{"on_committee_approval", "on_committee_approval", "on_reject", "on_state_updated"})
}

func TestWorkflowCommitteeVotingApprovesAtApprovalPercentage(t *testing.T) {
	engine := NewEngine()
	settingsID := engine.AddSettings(Settings{Name: "Voting Approval", Model: "purchase.order", Active: true})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Approve", Action: ActionApprove, VotingType: "approve"})
	rejectID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "committee", Name: "Reject", Action: ActionApprove, VotingType: "reject"})
	engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "committee", Name: "Committee", Sequence: 10, Committee: true, IsVoting: true, CommitteeVotePercentage: 50})
	record := Record{Model: "purchase.order", ID: 18, State: "committee", ApprovalUserIDs: []int64{5, 6}}
	if _, err := engine.RunButton(context.Background(), User{ID: 5}, &record, approveID, Input{}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 6}, &record, rejectID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" {
		t.Fatalf("state = %s", record.State)
	}
}

func TestWorkflowForwardOverridesApproversAndClearsOnApproval(t *testing.T) {
	engine := NewEngine()
	settingsID := engine.AddSettings(Settings{Name: "Forwarded Approval", Model: "purchase.order", Active: true})
	configID := engine.AddConfig(ApprovalConfig{SettingsID: settingsID, State: "finance", Name: "Finance", Sequence: 10, GroupIDs: []int64{10}})
	forwardID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "finance", Name: "Forward", Action: ActionForward})
	approveID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "finance", Name: "Approve", Action: ActionApprove, VisibleTo: "approval"})
	record := Record{Model: "purchase.order", ID: 17, State: "finance", ApprovalUserIDs: []int64{5}}

	if _, err := engine.RunButton(context.Background(), User{ID: 5, GroupIDs: []int64{10}}, &record, forwardID, Input{TargetUserID: 8}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.ApprovalUserIDs, []int64{8}) || !reflect.DeepEqual(record.ForwardedToUserIDs, []int64{8}) {
		t.Fatalf("forwarded users record=%+v", record)
	}
	if len(engine.Forwards) != 1 || engine.Forwards[0].UserID != 8 || engine.Forwards[0].ApprovalStateID != configID {
		t.Fatalf("forwards = %+v", engine.Forwards)
	}
	buttons, err := engine.VisibleButtons(User{ID: 5, GroupIDs: []int64{10}}, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(buttons) != 1 || buttons[0].ID != forwardID {
		t.Fatalf("original approver buttons = %+v", buttons)
	}
	buttons, err = engine.VisibleButtons(User{ID: 8}, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(buttons) != 2 || buttons[1].ID != approveID {
		t.Fatalf("forwarded approver buttons = %+v", buttons)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 8}, &record, approveID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" || len(record.ForwardedToUserIDs) != 0 {
		t.Fatalf("approved forwarded record = %+v", record)
	}
}

func TestVotingMassApproveEscalationAndModels(t *testing.T) {
	engine := NewEngine()
	now := time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC)
	engine.SetNow(func() time.Time { return now })
	settingsID := engine.AddSettings(Settings{Name: "Expense Approval", Model: "hr.expense", Active: true})
	voteID := engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Vote", Action: ActionVote, VoteThreshold: 2})
	engine.AddButton(Button{SettingsID: settingsID, StateValue: "draft", Name: "Approve", Action: ActionApprove})
	engine.AddEscalation(Escalation{SettingsID: settingsID, StateValue: "draft", After: time.Hour, Active: true, ToUserID: 9})
	record := Record{Model: "hr.expense", ID: 1, State: "draft", LastStateUpdate: now.Add(-2 * time.Hour)}
	if _, err := engine.RunButton(context.Background(), User{ID: 1}, &record, voteID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "draft" {
		t.Fatalf("state after one vote = %s", record.State)
	}
	if _, err := engine.RunButton(context.Background(), User{ID: 2}, &record, voteID, Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" {
		t.Fatalf("state after threshold = %s", record.State)
	}

	records := []*Record{
		{Model: "hr.expense", ID: 2, State: "draft"},
		{Model: "hr.expense", ID: 3, State: "draft"},
	}
	results, err := engine.MassApprove(context.Background(), User{ID: 3}, records, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || records[0].State != "approved" || records[1].State != "approved" || !results[0].Log.BulkApproval {
		t.Fatalf("mass approve results=%+v records=%+v", results, records)
	}
	if due := engine.DueEscalations(Record{Model: "hr.expense", ID: 4, State: "draft", LastStateUpdate: now.Add(-2 * time.Hour)}, now); len(due) != 1 {
		t.Fatalf("due escalations = %+v", due)
	}

	reg := registry.New("test")
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ModelSettings, ModelButton, ModelAutomation, ModelLog, ModelLogVoting, ModelForward, ModelCancellation, ModelApprovalRecord, ModelNameSequenceMixin} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	if !reg.Models[ModelNameSequenceMixin].Abstract {
		t.Fatal("name.sequence.mixin must be abstract")
	}
	if _, ok := reg.Models[ModelNameSequenceMixin].Fields["name"]; !ok {
		t.Fatal("name.sequence.mixin missing name field")
	}
	if _, ok := reg.Models[ModelSettings].Fields["sequence"]; !ok {
		t.Fatal("approval.settings missing sequence")
	}
	for _, fieldName := range []string{"model_name", "state_ids", "approval_count", "button_count", "approval_all_groups", "static_states", "advance"} {
		if _, ok := reg.Models[ModelSettings].Fields[fieldName]; !ok {
			t.Fatalf("approval.settings missing source-compatible field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"state", "type", "reject_state", "model_id"} {
		if _, ok := reg.Models[ModelSettingsState].Fields[fieldName]; !ok {
			t.Fatalf("approval.settings.state missing source-compatible field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"model_id", "setting_id", "state", "sequence", "group_ids", "condition", "schedule_activity", "committee", "is_voting"} {
		if _, ok := reg.Models[ModelConfig].Fields[fieldName]; !ok {
			t.Fatalf("approval.config missing source-compatible field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"action_type", "comment", "context", "invisible", "icon", "states", "run_as_superuser", "hotkey", "validate_form", "voting_type", "is_voting"} {
		if _, ok := reg.Models[ModelButton].Fields[fieldName]; !ok {
			t.Fatalf("approval.buttons missing source-compatible field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"res_model", "res_ids", "confirm_message", "action_type", "return_state", "transfer_state", "forward_user_id"} {
		if _, ok := reg.Models[ModelProcessWizard].Fields[fieldName]; !ok {
			t.Fatalf("approval.process.wizard missing source-compatible field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"record_cancellation_count", "approval_activity_date_deadline", "_approval_button_id", "duration_state_tracking", "approval_voting_ids", "approval_visible_button_ids"} {
		if _, ok := reg.Models[ModelApprovalRecord].Fields[fieldName]; !ok {
			t.Fatalf("approval.record missing source-compatible field %s", fieldName)
		}
	}
	if reg.Models[ModelApprovalRecord].Fields["approval_state_id"].Relation != ModelConfig ||
		reg.Models[ModelForward].Fields["approval_state_id"].Relation != ModelConfig {
		t.Fatalf("approval_state_id relation mismatch: approval.record=%s approval.forward=%s", reg.Models[ModelApprovalRecord].Fields["approval_state_id"].Relation, reg.Models[ModelForward].Fields["approval_state_id"].Relation)
	}
	if !reg.Models[ModelApprovalRecord].Abstract {
		t.Fatal("approval.record must be abstract")
	}
	automationTrigger := reg.Models[ModelAutomation].Fields["trigger"]
	for _, value := range []string{"on_submit", "on_enter_approval", "on_approve", "on_approval", "on_reject", "on_return", "on_cancel", "on_draft", "on_forward", "on_transfer", "on_state_updated", "on_create", "on_committee_approval"} {
		if !selectionHasValue(automationTrigger.Selection, value) {
			t.Fatalf("approval.automation trigger missing %s", value)
		}
	}
	buttonAction := reg.Models[ModelButton].Fields["action_type"]
	if !selectionHasValue(buttonAction.Selection, "draft") {
		t.Fatal("approval.buttons action_type missing draft")
	}
	if !selectionHasValue(buttonAction.Selection, "action") || !selectionHasValue(buttonAction.Selection, "server_action") {
		t.Fatal("approval.buttons action_type missing source or legacy server action value")
	}
}

func assertTriggers(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("triggers = %+v, want %+v", got, want)
	}
}

func selectionHasValue(options []field.SelectionOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}
