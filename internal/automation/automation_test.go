package automation

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gorp/internal/actions"
	"gorp/internal/domain"
)

func TestEngineRunsCreateWriteAndCreateOrWriteTriggers(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	mustAdd(t, engine, Automation{Name: "On Create", Model: "res.partner", Trigger: TriggerCreate, ActionID: 1})
	mustAdd(t, engine, Automation{Name: "On Write", Model: "res.partner", Trigger: TriggerWrite, ActionID: 2})
	mustAdd(t, engine, Automation{Name: "On Create Or Write", Model: "res.partner", Trigger: TriggerCreateOrWrite, ActionID: 3})

	results, err := engine.Run(context.Background(), Event{Trigger: TriggerCreate, Model: "res.partner", RecordID: 10, Values: map[string]any{"name": "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{1, 3}) {
		t.Fatalf("create action ids = %+v", got)
	}
	if runner.calls[0].exec.Trigger != "create" || runner.calls[0].exec.RecordID != 10 {
		t.Fatalf("create exec = %+v", runner.calls[0].exec)
	}

	runner.calls = nil
	results, err = engine.Run(context.Background(), Event{Trigger: TriggerWrite, Model: "res.partner", RecordIDs: []int64{10}, Values: map[string]any{"name": "Ada Lovelace"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{2, 3}) {
		t.Fatalf("write action ids = %+v", got)
	}
	if !reflect.DeepEqual(runner.calls[0].exec.RecordIDs, []int64{10}) {
		t.Fatalf("write record ids = %+v", runner.calls[0].exec.RecordIDs)
	}
}

func TestEngineRunsMultipleActions(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	mustAdd(t, engine, Automation{Name: "Multi", Model: "res.partner", Trigger: TriggerWrite, ActionIDs: []int64{8, 9}})

	results, err := engine.Run(context.Background(), Event{Trigger: TriggerWrite, Model: "res.partner", RecordID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{8, 9}) {
		t.Fatalf("multi action ids = %+v", got)
	}
}

func TestEngineFiltersTriggerFieldsAndDomain(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	mustAdd(t, engine, Automation{
		Name:          "Email Changed In Bahrain",
		Model:         "res.partner",
		Trigger:       TriggerWrite,
		TriggerFields: []string{"email"},
		Domain: domain.And(
			domain.Cond("active", domain.Equal, true),
			domain.Cond("country.code", domain.Equal, "BH"),
		),
		ActionID: 4,
	})

	results, err := engine.Run(context.Background(), Event{
		Trigger: TriggerWrite,
		Model:   "res.partner",
		Fields:  []string{"name"},
		Record:  map[string]any{"active": true, "country": map[string]any{"code": "BH"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected field filter miss, got %+v", results)
	}

	results, err = engine.Run(context.Background(), Event{
		Trigger: TriggerWrite,
		Model:   "res.partner",
		Fields:  []string{"email"},
		Record:  map[string]any{"active": false, "country": map[string]any{"code": "BH"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected domain miss, got %+v", results)
	}

	results, err = engine.Run(context.Background(), Event{
		Trigger: TriggerWrite,
		Model:   "res.partner",
		Fields:  []string{"email"},
		Record:  map[string]any{"active": true, "country": map[string]any{"code": "BH"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{4}) {
		t.Fatalf("action ids = %+v", got)
	}
}

func TestEngineFiltersPreAndPostDomains(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	mustAdd(t, engine, Automation{
		Name:            "Stage Enters Won",
		Model:           "crm.lead",
		Trigger:         TriggerWrite,
		FilterPreDomain: domain.Cond("stage", domain.NotEqual, "won"),
		Domain:          domain.Cond("stage", domain.Equal, "won"),
		ActionID:        11,
	})

	results, err := engine.Run(context.Background(), Event{
		Trigger:        TriggerWrite,
		Model:          "crm.lead",
		PreviousRecord: map[string]any{"stage": "new"},
		Record:         map[string]any{"stage": "won"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{11}) {
		t.Fatalf("pre/post match ids = %+v", got)
	}

	results, err = engine.Run(context.Background(), Event{
		Trigger:        TriggerWrite,
		Model:          "crm.lead",
		PreviousRecord: map[string]any{"stage": "won"},
		Record:         map[string]any{"stage": "won"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no transition match, got %+v", results)
	}
}

func TestEngineRunsManualAndTimeTriggersWithWindows(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	manualID := mustAdd(t, engine, Automation{Name: "Manual", Model: "res.partner", Trigger: TriggerManual, ActionID: 10})
	first := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	mustAdd(t, engine, Automation{Name: "Timed", Model: "res.partner", Trigger: TriggerTime, ActionID: 20, LastRunAt: first})

	result, err := engine.RunManual(context.Background(), manualID, Event{RecordID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionID != 10 {
		t.Fatalf("manual result = %+v", result)
	}
	if runner.calls[0].exec.Trigger != "manual" || runner.calls[0].exec.Model != "res.partner" {
		t.Fatalf("manual exec = %+v", runner.calls[0].exec)
	}

	runner.calls = nil
	second := first.Add(1 * time.Hour)
	results, err := engine.RunTime(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{20}) {
		t.Fatalf("time action ids = %+v", got)
	}
	if !runner.calls[0].exec.LastRunAt.Equal(first) || !runner.calls[0].exec.CurrentRunAt.Equal(second) {
		t.Fatalf("time exec window = %+v", runner.calls[0].exec)
	}

	runner.calls = nil
	third := second.Add(1 * time.Hour)
	_, err = engine.RunTime(context.Background(), third)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.calls[0].exec.LastRunAt.Equal(second) || !runner.calls[0].exec.CurrentRunAt.Equal(third) {
		t.Fatalf("advanced time window = %+v", runner.calls[0].exec)
	}
}

func TestRunTimeKeepsWindowWhenActionFails(t *testing.T) {
	errBoom := errors.New("boom")
	runner := &captureRunner{fail: map[int64]error{20: errBoom}}
	engine := NewEngine(runner)
	first := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	id := mustAdd(t, engine, Automation{Name: "Timed", Model: "res.partner", Trigger: TriggerTime, ActionID: 20, LastRunAt: first})
	second := first.Add(1 * time.Hour)

	_, err := engine.RunTime(context.Background(), second)
	if !errors.Is(err, errBoom) {
		t.Fatalf("error = %v", err)
	}
	automation, ok := engine.get(id)
	if !ok {
		t.Fatal("missing automation")
	}
	if !automation.LastRunAt.Equal(first) {
		t.Fatalf("last run advanced after failure: %+v", automation.LastRunAt)
	}

	runner.fail = nil
	runner.calls = nil
	results, err := engine.RunTime(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{20}) {
		t.Fatalf("retry action ids = %+v", got)
	}
	if !runner.calls[0].exec.LastRunAt.Equal(first) || !runner.calls[0].exec.CurrentRunAt.Equal(second) {
		t.Fatalf("retry window = %+v", runner.calls[0].exec)
	}
}

func TestWebhookRouteExecutionAndLogging(t *testing.T) {
	runner := &captureRunner{}
	engine := NewEngine(runner)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	mustAdd(t, engine, Automation{
		Name:            "Webhook",
		Model:           "res.partner",
		Trigger:         TriggerWebhook,
		WebhookUUID:     "abc",
		LogWebhookCalls: true,
		ActionID:        15,
	})

	routes := WebhookRoutes()
	if len(routes) != 1 || routes[0].Path != "/web/hook/<rule_uuid>" || routes[0].Auth != "public" || routes[0].CSRF {
		t.Fatalf("routes = %+v", routes)
	}
	if WebhookURL("https://example.com/", "abc") != "https://example.com/web/hook/abc" {
		t.Fatal("bad webhook URL")
	}
	results, status, err := engine.HandleWebhook(context.Background(), "abc", map[string]any{"_model": "res.partner", "_id": "42", "email": "ada@example.com"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(resultIDs(results), []int64{15}) {
		t.Fatalf("webhook result status=%d results=%+v", status, results)
	}
	if runner.calls[0].exec.RecordID != 42 || runner.calls[0].exec.Metadata["webhook_uuid"] != "abc" {
		t.Fatalf("webhook exec = %+v", runner.calls[0].exec)
	}
	logs := engine.WebhookLogs()
	if len(logs) != 1 || logs[0].Status != "ok" || logs[0].AutomationID == 0 {
		t.Fatalf("logs = %+v", logs)
	}
	_, status, err = engine.HandleWebhook(context.Background(), "missing", map[string]any{}, now)
	if err != ErrAutomationNotFound || status != 404 {
		t.Fatalf("missing webhook status=%d err=%v", status, err)
	}

	action := ActionOpenAutomation(99)
	if action.ResModel != "base.automation" || action.ResID != 99 || action.Type != "ir.actions.act_window" {
		t.Fatalf("action = %+v", action)
	}
}

func TestMatchDomainAgainstMaps(t *testing.T) {
	row := map[string]any{
		"name":   "Administrator",
		"age":    30,
		"active": true,
		"company": map[string]any{
			"country": "BH",
		},
	}
	node := domain.And(
		domain.Cond("name", domain.ILike, "admin%"),
		domain.Cond("age", domain.GreaterEqual, int64(30)),
		domain.Cond("company.country", domain.In, []string{"BH", "SA"}),
		domain.Not(domain.Cond("active", domain.Equal, false)),
	)
	ok, err := MatchDomain(row, node)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected domain match")
	}
}

func mustAdd(t *testing.T, engine *Engine, automation Automation) int64 {
	t.Helper()
	id, err := engine.Add(automation)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func resultIDs(results []actions.Result) []int64 {
	ids := make([]int64, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ActionID)
	}
	return ids
}

type runCall struct {
	id   int64
	exec actions.ExecutionContext
}

type captureRunner struct {
	calls []runCall
	fail  map[int64]error
}

func (r *captureRunner) Run(_ context.Context, id int64, exec actions.ExecutionContext) (actions.Result, error) {
	r.calls = append(r.calls, runCall{id: id, exec: exec})
	if err := r.fail[id]; err != nil {
		return actions.Result{ActionID: id}, err
	}
	return actions.Result{ActionID: id}, nil
}
