package scheduler

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorp/internal/actions"
	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestCronIntervalAdvancesPastNow(t *testing.T) {
	s := New()
	nextCall := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 16, 2, 30, 0, 0, time.UTC)

	cron, err := s.AddCron(Cron{
		Name:           "hourly sync",
		Active:         true,
		UserID:         7,
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       nextCall,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := s.RunDue(now, func(run Cron) error {
		if run.ID != cron.ID || run.UserID != 7 {
			t.Fatalf("run cron = %#v", run)
		}
		return nil
	})
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}

	updated, ok := s.Cron(cron.ID)
	if !ok {
		t.Fatal("cron missing")
	}
	want := time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC)
	if !updated.NextCall.Equal(want) {
		t.Fatalf("nextcall = %s, want %s", updated.NextCall, want)
	}
}

func TestCronFailureCountAndRetry(t *testing.T) {
	s := New(WithRetryBackoff(func(Cron) time.Duration { return 2 * time.Minute }))
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	boom := errors.New("boom")

	cron, err := s.AddCron(Cron{
		Name:           "retrying cron",
		Active:         true,
		IntervalNumber: 5,
		IntervalType:   IntervalMinutes,
		NextCall:       now,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := s.RunDue(now, func(Cron) error { return boom })
	if result.Ran != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}

	updated, _ := s.Cron(cron.ID)
	if updated.FailureCount != 1 || updated.LastError != boom.Error() || !updated.NextCall.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("updated cron = %#v", updated)
	}
}

func TestRunDueActionsRunsRegisteredServerAction(t *testing.T) {
	s := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	registry := actions.NewRegistry(actions.Hooks{})
	var execs []actions.ExecutionContext
	if err := registry.RegisterGo("cron.capture", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		execs = append(execs, exec)
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := registry.Register(actions.ServerAction{Name: "Capture Cron", Kind: actions.KindGo, GoActionName: "cron.capture"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddCron(Cron{
		Name:           "registered action cron",
		Active:         true,
		UserID:         7,
		Model:          "res.partner",
		ActionID:       actionID,
		Context:        map[string]any{"lang": "en_US"},
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       now,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(execs) != 1 {
		t.Fatalf("execs = %+v", execs)
	}
	exec := execs[0]
	if exec.Trigger != "cron" || exec.Model != "res.partner" || exec.Metadata["cron_name"] != "registered action cron" || exec.Values["lang"] != "en_US" {
		t.Fatalf("exec = %+v", exec)
	}
}

func TestRunDueActionsRunsNamedGoAction(t *testing.T) {
	s := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	registry := actions.NewRegistry(actions.Hooks{})
	var called bool
	if err := registry.RegisterGo("base.autovacuum", func(_ context.Context, action actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		called = true
		if action.GoActionName != "base.autovacuum" || exec.Metadata["cron_id"] == nil {
			t.Fatalf("action=%+v exec=%+v", action, exec)
		}
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.AddCron(Cron{
		Name:           "named action cron",
		Active:         true,
		ActionName:     "base.autovacuum",
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       now,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 || !called {
		t.Fatalf("result=%+v called=%v", result, called)
	}
}

func TestRunDueActionsRunsWorkflowEscalationCron(t *testing.T) {
	s := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	registry := actions.NewRegistry(actions.Hooks{})
	var exec actions.ExecutionContext
	if err := registry.RegisterGo("workflow.process.escalation", func(_ context.Context, action actions.ServerAction, got actions.ExecutionContext) (actions.Result, error) {
		if action.GoActionName != "workflow.process.escalation" {
			t.Fatalf("action=%+v", action)
		}
		exec = got
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.AddCron(Cron{
		Name:           "Process Workflow Escalation",
		Active:         true,
		UserID:         1,
		Model:          "workflow.process",
		ActionName:     "workflow.process.escalation",
		Code:           "model._process_escalation()",
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       now,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 {
		t.Fatalf("result=%+v", result)
	}
	if exec.Trigger != "cron" || exec.Model != "workflow.process" || exec.UserID != int64(1) || exec.Metadata["cron_name"] != "Process Workflow Escalation" {
		t.Fatalf("exec=%+v", exec)
	}
}

func TestSnapshotRestorePreservesCronTriggerAndProgress(t *testing.T) {
	s := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	cron, err := s.AddCron(Cron{
		Name:           "persistent cron",
		Active:         true,
		ActionName:     "base.autovacuum",
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       now,
		Context:        map[string]any{"lang": "en_US"},
	})
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := s.AddTrigger(Trigger{CronID: cron.ID, At: now})
	if err != nil {
		t.Fatal(err)
	}
	progress := s.SetProgress(cron.ID, 2, 3)
	snapshot := s.Snapshot()

	restored := New()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	gotCron, ok := restored.Cron(cron.ID)
	if !ok || gotCron.Name != cron.Name || gotCron.Context["lang"] != "en_US" {
		t.Fatalf("cron = %+v ok=%v", gotCron, ok)
	}
	gotTriggers := restored.Triggers(cron.ID)
	if len(gotTriggers) != 1 || gotTriggers[0].ID != trigger.ID || gotTriggers[0].Status != TriggerPending {
		t.Fatalf("triggers = %+v", gotTriggers)
	}
	gotProgress, ok := restored.Progress(cron.ID)
	if !ok || !reflect.DeepEqual(gotProgress, progress) {
		t.Fatalf("progress = %+v ok=%v want %+v", gotProgress, ok, progress)
	}
	created, err := restored.AddCron(Cron{Name: "next", Active: true, IntervalNumber: 1, IntervalType: IntervalDays})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= cron.ID {
		t.Fatalf("created id = %d, want > %d", created.ID, cron.ID)
	}
}

func TestCommitProgressUsesOdooProgressSemantics(t *testing.T) {
	s := New()
	cron, err := s.AddCron(Cron{Name: "progress cron", Active: true, IntervalNumber: 1, IntervalType: IntervalHours})
	if err != nil {
		t.Fatal(err)
	}
	remaining := 5
	progress, err := s.CommitProgress(cron.ID, 2, &remaining, true)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Done != 2 || progress.Remaining != 5 || !progress.Deactivate {
		t.Fatalf("progress = %+v", progress)
	}
	progress, err = s.CommitProgress(cron.ID, 3, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Done != 5 || progress.Remaining != 2 || !progress.Deactivate {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestLoadFromEnvSyncsCronProgressFields(t *testing.T) {
	env := schedulerEnv(t)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Progress cron",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        "2026-06-16 08:00:00",
		"action_name":     "base.progress",
	})
	if err != nil {
		t.Fatal(err)
	}
	progressID, err := env.Model("ir.cron.progress").Create(map[string]any{
		"cron_id":           cronID,
		"done":              int64(2),
		"remaining":         int64(4),
		"deactivate":        true,
		"timed_out_counter": int64(2),
		"started_at":        "2026-06-16 08:00:00",
		"updated_at":        "2026-06-16 08:01:00",
	})
	if err != nil {
		t.Fatal(err)
	}

	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	progress, ok := s.Progress(cronID)
	if !ok || progress.ID != progressID || progress.Done != 2 || progress.Remaining != 4 || !progress.Deactivate || progress.TimedOutCounter != 2 {
		t.Fatalf("progress = %+v ok=%v", progress, ok)
	}
	if _, err := s.CommitProgress(cronID, 3, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.cron.progress").Browse(progressID).Read("done", "remaining", "deactivate", "timed_out_counter", "started_at", "updated_at")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["done"] != 5 || rows[0]["remaining"] != 1 || rows[0]["deactivate"] != true || rows[0]["timed_out_counter"] != 2 {
		t.Fatalf("synced progress = %+v", rows[0])
	}
	if rows[0]["started_at"] == nil || rows[0]["updated_at"] == nil {
		t.Fatalf("missing progress timestamps = %+v", rows[0])
	}
}

func TestLoadFromEnvIncludesInactiveCrons(t *testing.T) {
	env := schedulerEnv(t)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Inactive cron",
		"active":          false,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        "2026-06-16 08:00:00",
		"action_name":     "base.inactive",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	cron, ok := s.Cron(cronID)
	if !ok || cron.Active {
		t.Fatalf("inactive cron = %+v ok=%v", cron, ok)
	}
}

func TestSnapshotRestoreRecoversMissedRun(t *testing.T) {
	s := New()
	last := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	now := last.Add(4 * time.Hour)
	_, err := s.AddCron(Cron{
		Name:           "missed cron",
		Active:         true,
		ActionName:     "base.autovacuum",
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       last,
	})
	if err != nil {
		t.Fatal(err)
	}
	restored := New()
	if err := restored.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	var exec actions.ExecutionContext
	if err := registry.RegisterGo("base.autovacuum", func(_ context.Context, _ actions.ServerAction, got actions.ExecutionContext) (actions.Result, error) {
		exec = got
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	result := restored.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !exec.LastRunAt.Equal(last) || !exec.CurrentRunAt.Equal(now) {
		t.Fatalf("exec window = %+v", exec)
	}
	cron, _ := restored.Cron(1)
	if !cron.NextCall.Equal(now.Add(time.Hour)) {
		t.Fatalf("nextcall = %s", cron.NextCall)
	}
}

func TestLoadFromEnvRunsCronAndSyncsState(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Base: Auto-vacuum internal data",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        "2026-06-16 08:00:00",
		"model_id":        modelID,
		"action_name":     "base.autovacuum",
		"failure_count":   int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}

	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	loaded, ok := s.Cron(cronID)
	if !ok || loaded.Model != "res.partner" || loaded.ActionName != "base.autovacuum" {
		t.Fatalf("loaded cron = %+v ok=%v", loaded, ok)
	}

	registry := actions.NewRegistry(actions.Hooks{})
	var exec actions.ExecutionContext
	if err := registry.RegisterGo("base.autovacuum", func(_ context.Context, _ actions.ServerAction, got actions.ExecutionContext) (actions.Result, error) {
		exec = got
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 {
		t.Fatalf("result = %+v", result)
	}
	if exec.Trigger != "cron" || exec.Model != "res.partner" || !exec.LastRunAt.Equal(time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("exec = %+v", exec)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.cron").Browse(cronID).Read("nextcall", "failure_count")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["failure_count"] != 0 {
		t.Fatalf("row = %+v", rows[0])
	}
	nextCall, ok := rows[0]["nextcall"].(time.Time)
	if !ok || !nextCall.Equal(time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("nextcall = %#v", rows[0]["nextcall"])
	}
}

func TestLoadFromEnvRunsDelegatedServerActionCron(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "Delegated Cron Action",
		"model_id":       modelID,
		"model_name":     "res.partner",
		"state":          "code",
		"active":         true,
		"usage":          "ir_cron",
		"go_action_name": "cron.delegated",
	})
	if err != nil {
		t.Fatal(err)
	}
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":                 "Delegated cron",
		"cron_name":            "Delegated cron",
		"ir_actions_server_id": actionID,
		"active":               true,
		"interval_number":      int64(1),
		"interval_type":        "hours",
		"nextcall":             "2026-06-16 08:00:00",
		"lastcall":             "2026-06-16 07:00:00",
		"model_id":             modelID,
		"state":                "code",
		"code":                 "model.run()",
		"priority":             int64(2),
		"failure_count":        int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	loaded, ok := s.Cron(cronID)
	if !ok || loaded.ActionID != actionID || loaded.Priority != 2 {
		t.Fatalf("loaded cron = %+v ok=%v", loaded, ok)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	var exec actions.ExecutionContext
	if err := registry.RegisterGo("cron.delegated", func(_ context.Context, action actions.ServerAction, got actions.ExecutionContext) (actions.Result, error) {
		if action.ID != actionID {
			t.Fatalf("action = %+v", action)
		}
		exec = got
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Register(actions.ServerAction{ID: actionID, Name: "Delegated Cron Action", Kind: actions.KindGo, Model: "res.partner", GoActionName: "cron.delegated"}); err != nil {
		t.Fatal(err)
	}
	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if exec.Trigger != "cron" || exec.Model != "res.partner" || !exec.LastRunAt.Equal(time.Date(2026, 6, 16, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("exec = %+v", exec)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.cron").Browse(cronID).Read("lastcall", "nextcall", "failure_count", "first_failure_date")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["failure_count"] != 0 || !rows[0]["lastcall"].(time.Time).Equal(now) || !rows[0]["nextcall"].(time.Time).Equal(time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("synced cron = %+v", rows[0])
	}
}

func TestRunDuePartialProgressSchedulesImmediateTrigger(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Partial cron",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        now,
		"action_name":     "base.partial",
		"failure_count":   int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}

	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	if err := registry.RegisterGo("base.partial", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		remaining := 2
		if _, err := s.CommitProgress(int64Value(exec.Metadata["cron_id"]), 1, &remaining, false); err != nil {
			t.Fatal(err)
		}
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	progressRows, err := env.Model("ir.cron.progress").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		t.Fatal(err)
	}
	progress, err := progressRows.Read("done", "remaining", "timed_out_counter")
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0]["done"] != 1 || progress[0]["remaining"] != 2 || progress[0]["timed_out_counter"] != 0 {
		t.Fatalf("progress = %+v", progress)
	}
	triggerRows, err := env.Model("ir.cron.trigger").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := triggerRows.Read("call_at")
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 1 || !triggers[0]["call_at"].(time.Time).Equal(now) {
		t.Fatalf("triggers = %+v", triggers)
	}
}

func TestRunDueActionsCommitsActionResultProgressMetadata(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Metadata progress cron",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        now,
		"action_name":     "base.metadata_progress",
		"failure_count":   int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}

	s := New()
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	if err := registry.RegisterGo("base.metadata_progress", func(context.Context, actions.ServerAction, actions.ExecutionContext) (actions.Result, error) {
		return actions.Result{Metadata: map[string]any{
			"cron_progress_done":       int64(3),
			"cron_progress_remaining":  int64(0),
			"cron_progress_deactivate": true,
		}}, nil
	}); err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	progressRows, err := env.Model("ir.cron.progress").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		t.Fatal(err)
	}
	progress, err := progressRows.Read("done", "remaining", "deactivate")
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0]["done"] != 3 || progress[0]["remaining"] != 0 || progress[0]["deactivate"] != true {
		t.Fatalf("progress = %+v", progress)
	}
	rows, err := env.Model("ir.cron").Browse(cronID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["active"] != false {
		t.Fatalf("cron = %+v", rows)
	}
}

func TestLoadFromEnvFailureSyncsRetryState(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Failing cron",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        now,
		"action_name":     "base.failing",
		"failure_count":   int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(WithRetryBackoff(func(Cron) time.Duration { return 2 * time.Minute }))
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	boom := errors.New("boom")
	if err := registry.RegisterGo("base.failing", func(context.Context, actions.ServerAction, actions.ExecutionContext) (actions.Result, error) {
		return actions.Result{}, boom
	}); err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.cron").Browse(cronID).Read("nextcall", "failure_count")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["failure_count"] != 1 {
		t.Fatalf("row = %+v", rows[0])
	}
	nextCall, ok := rows[0]["nextcall"].(time.Time)
	if !ok || !nextCall.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("nextcall = %#v", rows[0]["nextcall"])
	}
}

func TestRunDueSkipsCallbackAfterRepeatedTimeoutProgress(t *testing.T) {
	env := schedulerEnv(t)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cronID, err := env.Model("ir.cron").Create(map[string]any{
		"name":            "Timed out cron",
		"active":          true,
		"interval_number": int64(1),
		"interval_type":   "hours",
		"nextcall":        now,
		"action_name":     "base.timeout",
		"failure_count":   int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	progressID, err := env.Model("ir.cron.progress").Create(map[string]any{
		"cron_id":           cronID,
		"done":              int64(0),
		"remaining":         int64(4),
		"timed_out_counter": int64(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(WithRetryBackoff(func(Cron) time.Duration { return time.Minute }))
	if err := s.LoadFromEnv(env); err != nil {
		t.Fatal(err)
	}
	registry := actions.NewRegistry(actions.Hooks{})
	var called bool
	if err := registry.RegisterGo("base.timeout", func(context.Context, actions.ServerAction, actions.ExecutionContext) (actions.Result, error) {
		called = true
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	result := s.RunDueActions(context.Background(), now, registry)
	if result.Ran != 1 || result.Failed != 1 || called {
		t.Fatalf("result=%+v called=%v", result, called)
	}
	if !errors.Is(result.Errors[0], ErrCronTimedOut) {
		t.Fatalf("errors = %+v", result.Errors)
	}
	if err := s.SyncToEnv(env); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.cron.progress").Browse(progressID).Read("timed_out_counter")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["timed_out_counter"] != 0 {
		t.Fatalf("progress = %+v", rows[0])
	}
}

func TestConcurrentRunDueExecutesCronOnce(t *testing.T) {
	s := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)

	if _, err := s.AddCron(Cron{
		Name:           "locked cron",
		Active:         true,
		IntervalNumber: 1,
		IntervalType:   IntervalHours,
		NextCall:       now,
	}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan Result)
	var calls int32

	go func() {
		done <- s.RunDue(now, func(Cron) error {
			atomic.AddInt32(&calls, 1)
			close(started)
			<-release
			return nil
		})
	}()

	<-started
	second := s.RunDue(now, func(Cron) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if second.Ran != 0 || second.SkippedLocked != 1 {
		t.Fatalf("second result = %#v", second)
	}

	close(release)
	first := <-done
	if first.Ran != 1 || first.Succeeded != 1 {
		t.Fatalf("first result = %#v", first)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func schedulerEnv(t *testing.T) *record.Env {
	t.Helper()
	registry := record.NewRegistry()
	for _, m := range base.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(registry, record.Context{UserID: 1})
}
