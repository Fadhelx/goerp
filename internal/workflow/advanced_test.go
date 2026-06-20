package workflow

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestAdvancedModelsRegister(t *testing.T) {
	reg := registry.New("test")
	if err := RegisterAdvancedModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ModelWorkflow, ModelNode, ModelTransition, ModelNodeAction, ModelProcess, ModelWorkflowWizard} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	workflowModel := reg.Models[ModelWorkflow]
	for _, fieldName := range []string{"name", "approval_settings_id", "model", "condition", "flowchart", "company_ids", "start_node_id"} {
		if _, ok := workflowModel.Fields[fieldName]; !ok {
			t.Fatalf("missing workflow field %s", fieldName)
		}
	}
	nodeModel := reg.Models[ModelNode]
	for _, fieldName := range []string{"type", "model_id", "responsible_group_ids", "responsible_value", "responsible_filter", "responsible_condition", "responsible_committee", "responsible_committee_limit", "schedule_activity", "schedule_activity_enabled", "button_type", "button_context", "button_icon", "button_validate_form", "wizard_view_id", "escalation_node_id", "trg_date_calendar_id"} {
		if _, ok := nodeModel.Fields[fieldName]; !ok {
			t.Fatalf("missing node field %s", fieldName)
		}
	}
	transitionModel := reg.Models[ModelTransition]
	for _, fieldName := range []string{"next_node_id", "groups_ids", "comment", "committee", "is_email", "email_template_id", "is_hidden"} {
		if _, ok := transitionModel.Fields[fieldName]; !ok {
			t.Fatalf("missing transition field %s", fieldName)
		}
	}
	if got := len(ApprovalLogFields()); got != 3 {
		t.Fatalf("approval log fields = %d", got)
	}
	extensions := map[string]map[string]bool{}
	for _, m := range AdvancedExtensionModels() {
		fields := map[string]bool{}
		for name := range m.Fields {
			fields[name] = true
		}
		extensions[m.Name] = fields
	}
	for _, fieldName := range []string{"workflow_process_ids", "workflow_id", "workflow_node_id", "workflow_transition_ids", "workflow_view_id", "_workflow_transition_id"} {
		if !extensions[ModelApprovalRecord][fieldName] {
			t.Fatalf("approval.record extension missing field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"workflow_ids", "workflow_count"} {
		if !extensions["approval.settings"][fieldName] {
			t.Fatalf("approval.settings extension missing field %s", fieldName)
		}
	}
	if !extensions[ModelForward]["workflow_node_id"] {
		t.Fatal("approval.forward extension missing workflow_node_id")
	}
	if !extensions["approval.config"]["workflow_advanced"] {
		t.Fatal("approval.config extension missing workflow_advanced")
	}
	if !extensions["ir.actions.act_window"]["multi_workflow_view"] {
		t.Fatal("ir.actions.act_window extension missing multi_workflow_view")
	}
	if !extensions["approval.state.update"]["workflow_model"] || !extensions["approval.state.update"]["workflow_node_id"] {
		t.Fatal("approval.state.update extension missing workflow fields")
	}
}

func TestConditionExpressionDSL(t *testing.T) {
	ctx := EvaluationContext{
		UserID:       7,
		UserGroupIDs: []int64{30, 40},
		CompanyID:    1,
		CompanyIDs:   []int64{1, 2},
		Model:        "purchase.order",
		RecordID:     99,
		Values: map[string]any{
			"amount":     1500,
			"state":      "draft",
			"company_id": int64(2),
		},
		Predicates: map[string]Predicate{
			"large_amount": func(ctx EvaluationContext) (bool, error) {
				return ctx.Values["amount"].(int) > 1000, nil
			},
		},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{`amount >= 1000 and state == "draft"`, true},
		{`company_id in user.company_ids`, true},
		{`user.id == 7 and 30 in user.group_ids`, true},
		{`state not in "posted"`, true},
		{`not (amount < 1000)`, true},
		{`predicate:large_amount`, true},
		{`amount < 1000 or state == "posted"`, false},
	}
	for _, tc := range cases {
		got, err := Expr(tc.expr).Evaluate(ctx)
		if err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("%s = %v, want %v", tc.expr, got, tc.want)
		}
	}
	if _, err := Expr(`amount >= 1000; unsafe()`).Evaluate(ctx); err == nil {
		t.Fatal("expected unsafe expression to fail")
	}
}

func TestAdvancedWorkflowProcess(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	w := testWorkflow()
	ctx := EvaluationContext{
		UserID:       7,
		UserGroupIDs: []int64{42},
		CompanyID:    1,
		CompanyIDs:   []int64{1, 2},
		Model:        "purchase.order",
		RecordID:     99,
		Values: map[string]any{
			"amount":     1500,
			"state":      "draft",
			"company_id": int64(1),
		},
		Now: now,
	}
	hooks := Hooks{}
	var actionKeys []string
	hooks.Action = func(action NodeAction, process Process, ctx EvaluationContext) (ActionResult, error) {
		actionKeys = append(actionKeys, action.ActionKey)
		return ActionResult{ActionID: action.ID, Key: action.ActionKey, Type: "server"}, nil
	}
	var logs []ApprovalLogEvent
	hooks.ApprovalLog = func(event ApprovalLogEvent) error {
		logs = append(logs, event)
		return nil
	}

	matches, err := w.Matches(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("workflow should match")
	}
	blockedCtx := ctx
	blockedCtx.CompanyID = 3
	matches, err = w.Matches(blockedCtx, false)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("workflow matched unauthorized company")
	}

	process, results, err := w.Start(ctx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if process.NodeID != 20 || !process.Active || process.State != "pending" {
		t.Fatalf("started process = %+v", process)
	}
	if !reflect.DeepEqual(actionKeys, []string{"stamp"}) || len(results) != 1 {
		t.Fatalf("actions = %+v, results = %+v", actionKeys, results)
	}
	if len(logs) != 1 || logs[0].OldNodeID != 10 || logs[0].NewNodeID != 20 || logs[0].TransitionID != 100 {
		t.Fatalf("logs after start = %+v", logs)
	}

	available, err := w.AvailableTransitions(process, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].ID != 200 {
		t.Fatalf("available transitions = %+v", available)
	}
	deniedCtx := ctx
	deniedCtx.UserGroupIDs = []int64{99}
	available, err = w.AvailableTransitions(process, deniedCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 0 {
		t.Fatalf("group restricted transitions = %+v", available)
	}
	responsibleWorkflow := w
	responsibleWorkflow.Nodes[1].ResponsibleGroupIDs = nil
	responsibleWorkflow.Nodes[1].Transitions[0].GroupIDs = []int64{42}
	otherApproverCtx := ctx
	otherApproverCtx.UserID = 9
	otherApproverCtx.UserGroupIDs = []int64{42}
	available, err = responsibleWorkflow.AvailableTransitions(process, otherApproverCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 0 {
		t.Fatalf("node responsibility restricted transitions = %+v", available)
	}
	if _, _, err := responsibleWorkflow.ApplyTransition(process, 200, otherApproverCtx, hooks); err == nil {
		t.Fatal("expected node responsibility to deny transition")
	}

	process, results, err = w.ApplyTransition(process, 200, ctx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if process.NodeID != 30 || process.Active || process.State != "approved" {
		t.Fatalf("approved process = %+v", process)
	}
	if len(results) != 0 {
		t.Fatalf("unexpected approval action results = %+v", results)
	}
	if len(logs) != 2 || logs[1].OldNodeID != 20 || logs[1].NewNodeID != 30 || logs[1].UserID != 7 {
		t.Fatalf("logs after approve = %+v", logs)
	}
}

func TestAdvancedCommitteePartialApprovalExcludesDoneUsers(t *testing.T) {
	w := testWorkflow()
	w.Nodes[1].ResponsibleUserIDs = []int64{7, 9}
	w.Nodes[1].ResponsibleGroupIDs = nil
	w.Nodes[1].ResponsibleCommittee = true
	w.Nodes[1].ResponsibleCommitteeLimit = 2
	w.Nodes[1].Transitions[0].GroupIDs = nil
	w.Nodes[1].Transitions[0].Committee = true
	w.Nodes[1].Transitions[0].CommitteeLimit = 2
	ctx := EvaluationContext{
		UserID:       7,
		UserGroupIDs: []int64{42},
		CompanyID:    1,
		CompanyIDs:   []int64{1},
		Model:        "purchase.order",
		RecordID:     99,
		Values: map[string]any{
			"amount":     1500,
			"state":      "draft",
			"company_id": int64(1),
		},
		Now: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
	}
	var logs []ApprovalLogEvent
	hooks := Hooks{ApprovalLog: func(event ApprovalLogEvent) error {
		logs = append(logs, event)
		return nil
	}}
	process, _, err := w.Start(ctx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	process, _, err = w.ApplyTransition(process, 200, ctx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if process.NodeID != 20 || !process.Active || !containsInt64(process.ApprovalDoneUserIDs, 7) || containsInt64(process.ApprovalUserIDs, 7) || !containsInt64(process.ApprovalUserIDs, 9) {
		t.Fatalf("partial committee process = %+v", process)
	}
	available, err := w.AvailableTransitions(process, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 0 {
		t.Fatalf("done user transitions = %+v", available)
	}
	nextCtx := ctx
	nextCtx.UserID = 9
	nextCtx.Now = ctx.Now.Add(time.Minute)
	available, err = w.AvailableTransitions(process, nextCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].ID != 200 {
		t.Fatalf("remaining user transitions = %+v", available)
	}
	process, _, err = w.ApplyTransition(process, 200, nextCtx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if process.NodeID != 30 || process.Active || len(process.ApprovalDoneUserIDs) != 0 || len(process.ApprovalUserIDs) != 0 {
		t.Fatalf("completed committee process = %+v", process)
	}
	if len(logs) != 3 || logs[1].OldNodeID != 20 || logs[1].NewNodeID != 20 || logs[2].OldNodeID != 20 || logs[2].NewNodeID != 30 {
		t.Fatalf("committee logs = %+v", logs)
	}
}

func TestProcessStorePersistsWorkflowLifecycleAndLogs(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	w := testWorkflow()
	ctx := EvaluationContext{
		UserID:       7,
		UserGroupIDs: []int64{42},
		CompanyID:    1,
		CompanyIDs:   []int64{1},
		Model:        "purchase.order",
		RecordID:     99,
		Values: map[string]any{
			"amount":     1500,
			"state":      "draft",
			"company_id": int64(1),
		},
		Now: now,
	}
	env := testAdvancedRecordEnv(t)
	store := NewProcessStore(env)
	var observedLogs []ApprovalLogEvent
	hooks := Hooks{ApprovalLog: func(event ApprovalLogEvent) error {
		observedLogs = append(observedLogs, event)
		return nil
	}}

	process, _, processID, err := store.StartForRecord(w, ctx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, ok, err := store.Find("purchase.order", 99)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("stored process not found")
	}
	if reloaded.WorkflowID != 1 || reloaded.NodeID != 20 || !reloaded.Active || reloaded.State != "pending" || reloaded.LastTransitionID != 100 {
		t.Fatalf("stored start process = %+v", reloaded)
	}
	if !reflect.DeepEqual(reloaded.ApprovalUserIDs, []int64{7}) {
		t.Fatalf("approval users = %+v", reloaded.ApprovalUserIDs)
	}
	if !reloaded.StartedAt.Equal(now) || !reloaded.UpdatedAt.Equal(now) {
		t.Fatalf("process timestamps = start %s update %s", reloaded.StartedAt, reloaded.UpdatedAt)
	}

	nextCtx := ctx
	nextCtx.Now = now.Add(time.Minute)
	process, _, updatedID, err := store.ApplyTransitionForRecord(w, process, 200, nextCtx, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if updatedID != processID {
		t.Fatalf("process id changed from %d to %d", processID, updatedID)
	}
	processes, err := env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"active_test": false}}).Model(ModelProcess).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if processes.Len() != 1 {
		t.Fatalf("workflow.process count = %d", processes.Len())
	}
	reloaded, ok, err = store.Find("purchase.order", 99)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || reloaded.NodeID != 30 || reloaded.Active || reloaded.State != "approved" || reloaded.LastTransitionID != 200 {
		t.Fatalf("stored approved process = %+v found=%v", reloaded, ok)
	}
	if !reloaded.UpdatedAt.Equal(nextCtx.Now) {
		t.Fatalf("updated_at = %s, want %s", reloaded.UpdatedAt, nextCtx.Now)
	}

	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 2 {
		t.Fatalf("approval.log count = %d", logs.Len())
	}
	rows, err := logs.Read("model", "record_id", "user_id", "old_node_id", "new_node_id", "workflow_transition_id", "description", "duration_seconds", "duration_hours")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["old_node_id"] != int64(10) || rows[0]["new_node_id"] != int64(20) || rows[0]["workflow_transition_id"] != int64(100) {
		t.Fatalf("start log = %+v", rows[0])
	}
	startSeconds, _ := toFloat(rows[0]["duration_seconds"])
	startHours, _ := toFloat(rows[0]["duration_hours"])
	if startSeconds != 0 || startHours != 0 {
		t.Fatalf("start log duration = %+v", rows[0])
	}
	if rows[1]["model"] != "purchase.order" || rows[1]["record_id"] != int64(99) || rows[1]["user_id"] != int64(7) ||
		rows[1]["old_node_id"] != int64(20) || rows[1]["new_node_id"] != int64(30) || rows[1]["workflow_transition_id"] != int64(200) ||
		rows[1]["description"] != "Workflow transition" {
		t.Fatalf("approval log = %+v", rows[1])
	}
	approveSeconds, _ := toFloat(rows[1]["duration_seconds"])
	approveHours, _ := toFloat(rows[1]["duration_hours"])
	if approveSeconds != 60 || approveHours != 1.0/60.0 {
		t.Fatalf("approval log duration = %+v", rows[1])
	}
	if len(observedLogs) != 2 || observedLogs[0].TransitionID != 100 || observedLogs[1].TransitionID != 200 {
		t.Fatalf("chained approval log hook = %+v", observedLogs)
	}

	if err := store.DeleteForRecord("purchase.order", 99); err != nil {
		t.Fatal(err)
	}
	reloaded, ok, err = store.Find("purchase.order", 99)
	if err != nil {
		t.Fatal(err)
	}
	if ok || reloaded.RecordID != 0 {
		t.Fatalf("process after delete = %+v found=%v", reloaded, ok)
	}
}

func TestProcessStoreRunsDueEscalations(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	w := testWorkflow()
	w.Nodes[1].Escalation = true
	w.Nodes[1].EscalationDelayType = DelayMinutes
	w.Nodes[1].EscalationDelay = 15
	w.Nodes[1].EscalationNodeID = 30
	ctx := EvaluationContext{
		UserID:       7,
		UserGroupIDs: []int64{42},
		CompanyID:    1,
		CompanyIDs:   []int64{1},
		Model:        "purchase.order",
		RecordID:     101,
		Values: map[string]any{
			"amount":     1500,
			"state":      "draft",
			"company_id": int64(1),
		},
		Now: now,
	}
	env := testAdvancedRecordEnv(t)
	store := NewProcessStore(env)
	_, _, _, err := store.StartForRecord(w, ctx, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, ok, err := store.Find("purchase.order", 101)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !reloaded.EscalationDate.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("stored escalation process = %+v found=%v", reloaded, ok)
	}
	due, err := store.DueEscalationProcesses(now.Add(14 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("early due processes = %+v", due)
	}
	due, err = store.DueEscalationProcesses(now.Add(15 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].RecordID != 101 || due[0].NodeID != 20 {
		t.Fatalf("due processes = %+v", due)
	}

	escalationCtx := EvaluationContext{UserID: 9, Now: now.Add(16 * time.Minute)}
	result, err := store.RunDueEscalations([]Workflow{w}, escalationCtx, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Due != 1 || result.Applied != 1 || result.Skipped != 0 {
		t.Fatalf("escalation result = %+v", result)
	}
	reloaded, ok, err = store.Find("purchase.order", 101)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || reloaded.NodeID != 30 || reloaded.Active || reloaded.State != "approved" || !reloaded.EscalationDate.IsZero() {
		t.Fatalf("escalated process = %+v found=%v", reloaded, ok)
	}
	logs, err := env.Model(ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := logs.Read("user_id", "old_node_id", "new_node_id", "workflow_transition_id", "description")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("approval log rows = %+v", rows)
	}
	if rows[1]["user_id"] != int64(9) || rows[1]["old_node_id"] != int64(20) || rows[1]["new_node_id"] != int64(30) ||
		rows[1]["workflow_transition_id"] != int64(0) || rows[1]["description"] != "Workflow escalation" {
		t.Fatalf("escalation log = %+v", rows[1])
	}
}

func TestGraphAndCompanyRule(t *testing.T) {
	w := testWorkflow()
	graph := w.Graph()
	if graph.StartNodeID != 10 || len(graph.Nodes) != 3 || len(graph.Edges) != 2 || len(graph.Actions) != 1 {
		t.Fatalf("graph = %+v", graph)
	}
	flowchart := w.Flowchart()
	for _, want := range []string{"st=>start: Start", "node10=>inputoutput: Start|approved", "trans100=>condition: submit", "trans100(yes)->node20->trans200", "action500=>operation: stamp|past"} {
		if !strings.Contains(flowchart, want) {
			t.Fatalf("flowchart missing %q:\n%s", want, flowchart)
		}
	}

	rule := AdvancedSecurityRuleDefinitions()[0].Rule
	user := security.User{ID: 7, Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}}
	for _, tc := range []struct {
		row  map[string]any
		want bool
	}{
		{map[string]any{"company_id": nil}, true},
		{map[string]any{"company_id": int64(2)}, true},
		{map[string]any{"company_id": int64(3)}, false},
	} {
		got, err := security.EvalDomain(tc.row, user, rule.Domain)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("company rule(%+v) = %v, want %v", tc.row, got, tc.want)
		}
	}
}

func testAdvancedRecordEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range AdvancedModels() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

func testWorkflow() Workflow {
	return Workflow{
		ID:         1,
		Name:       "Purchase Approval",
		Model:      "purchase.order",
		Active:     true,
		Condition:  Expr(`amount >= 1000 and state == "draft"`),
		CompanyIDs: []int64{1},
		Nodes: []Node{
			{
				ID:       10,
				Name:     "Start",
				Type:     NodeTypeAuto,
				Sequence: 1,
				Active:   true,
				Actions: []NodeAction{
					{ID: 500, NodeID: 10, Sequence: 1, Active: true, Condition: Expr(`amount >= 1000`), ActionKey: "stamp"},
				},
				Transitions: []Transition{
					{ID: 100, NodeID: 10, Name: "submit", Sequence: 1, Active: true, Condition: Expr(`amount >= 1000`), NextNodeID: 20},
				},
			},
			{
				ID:                  20,
				Name:                "Manager Review",
				Type:                NodeTypeUser,
				Sequence:            2,
				Active:              true,
				State:               "pending",
				ResponsibleUserIDs:  []int64{7},
				ResponsibleGroupIDs: []int64{42},
				ButtonType:          ButtonTypeMulti,
				Transitions: []Transition{
					{ID: 200, NodeID: 20, Name: "approve", Sequence: 1, Active: true, NextNodeID: 30, GroupIDs: []int64{42}, Comment: CommentOptional},
				},
			},
			{
				ID:       30,
				Name:     "Approved",
				Type:     NodeTypeEnd,
				Sequence: 3,
				Active:   true,
				State:    "approved",
			},
		},
	}
}
