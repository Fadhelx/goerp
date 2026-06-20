package workflow

import (
	"fmt"
	"strconv"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type ProcessStore struct {
	Env *record.Env
}

type EscalationRunResult struct {
	Due     int
	Applied int
	Skipped int
}

func NewProcessStore(env *record.Env) ProcessStore {
	return ProcessStore{Env: env}
}

func (s ProcessStore) Save(process Process) (int64, error) {
	if s.Env == nil {
		return 0, fmt.Errorf("workflow process store requires record env")
	}
	if process.Model == "" {
		return 0, fmt.Errorf("workflow process requires model")
	}
	if process.RecordID == 0 {
		return 0, fmt.Errorf("workflow process requires record id")
	}
	saveEnv := s.Env
	if process.RunAsSuperuser {
		saveEnv = workflowSudoEnv(saveEnv)
	}
	saveStore := ProcessStore{Env: saveEnv}
	values := processValues(process)
	found, err := saveStore.searchProcess(process.Model, process.RecordID)
	if err != nil {
		return 0, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return saveEnv.Model(ModelProcess).Create(values)
	}
	if err := saveEnv.Model(ModelProcess).Browse(ids[0]).Write(values); err != nil {
		return 0, err
	}
	if len(ids) > 1 {
		if err := saveEnv.Model(ModelProcess).Browse(ids[1:]...).Unlink(); err != nil {
			return 0, err
		}
	}
	return ids[0], nil
}

func (s ProcessStore) StartForRecord(workflow Workflow, ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, int64, error) {
	hooks.ApprovalLog = chainedApprovalLogHook(hooks.ApprovalLog, s.ApprovalLogHook())
	process, results, err := workflow.Start(ctx, hooks)
	if err != nil {
		return Process{}, nil, 0, err
	}
	id, err := s.Save(process)
	if err != nil {
		return Process{}, nil, 0, err
	}
	return process, results, id, nil
}

func (s ProcessStore) ApplyTransitionForRecord(workflow Workflow, process Process, transitionID int64, ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, int64, error) {
	hooks.ApprovalLog = chainedApprovalLogHook(hooks.ApprovalLog, s.ApprovalLogHook())
	next, results, err := workflow.ApplyTransition(process, transitionID, ctx, hooks)
	if err != nil {
		return process, nil, 0, err
	}
	id, err := s.Save(next)
	if err != nil {
		return process, nil, 0, err
	}
	return next, results, id, nil
}

func (s ProcessStore) ApplyEscalationForRecord(workflow Workflow, process Process, ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, int64, error) {
	hooks.ApprovalLog = chainedApprovalLogHook(hooks.ApprovalLog, s.ApprovalLogHook())
	next, results, err := workflow.ApplyEscalation(process, ctx, hooks)
	if err != nil {
		return process, nil, 0, err
	}
	id, err := s.Save(next)
	if err != nil {
		return process, nil, 0, err
	}
	return next, results, id, nil
}

func (s ProcessStore) Find(modelName string, recordID int64) (Process, bool, error) {
	if s.Env == nil {
		return Process{}, false, fmt.Errorf("workflow process store requires record env")
	}
	found, err := s.searchProcess(modelName, recordID)
	if err != nil {
		return Process{}, false, err
	}
	if found.Len() == 0 {
		return Process{}, false, nil
	}
	rows, err := s.Env.Model(ModelProcess).Browse(found.IDs()[0]).Read(
		"workflow_id",
		"model",
		"record_id",
		"node_id",
		"active",
		"state",
		"last_transition_id",
		"started_at",
		"updated_at",
		"approval_user_ids",
		"approval_done_user_ids",
		"approval_partner_ids",
		"user_can_approve",
		"escalation_date",
	)
	if err != nil {
		return Process{}, false, err
	}
	if len(rows) == 0 {
		return Process{}, false, nil
	}
	process := processFromRow(rows[0])
	return process, true, nil
}

func (s ProcessStore) Delete(modelName string, recordIDs ...int64) error {
	if s.Env == nil {
		return fmt.Errorf("workflow process store requires record env")
	}
	if len(recordIDs) == 0 {
		return nil
	}
	processEnv := s.processEnv()
	found, err := processEnv.Model(ModelProcess).Search(domain.And(
		domain.Cond("model", domain.Equal, modelName),
		domain.Cond("record_id", domain.In, recordIDs),
	))
	if err != nil {
		return err
	}
	return found.Unlink()
}

func (s ProcessStore) DeleteForRecord(modelName string, recordID int64) error {
	return s.Delete(modelName, recordID)
}

func (s ProcessStore) DueEscalationProcesses(at time.Time) ([]Process, error) {
	if s.Env == nil {
		return nil, fmt.Errorf("workflow process store requires record env")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	found, err := s.Env.Model(ModelProcess).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read(
		"workflow_id",
		"model",
		"record_id",
		"node_id",
		"active",
		"state",
		"last_transition_id",
		"started_at",
		"updated_at",
		"approval_user_ids",
		"approval_done_user_ids",
		"escalation_date",
	)
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, len(rows))
	for _, row := range rows {
		process := processFromRow(row)
		if !process.Active || process.EscalationDate.IsZero() || process.EscalationDate.After(at) {
			continue
		}
		out = append(out, process)
	}
	return out, nil
}

func (s ProcessStore) RunDueEscalations(workflows []Workflow, ctx EvaluationContext, hooks Hooks) (EscalationRunResult, error) {
	at := ctx.now()
	due, err := s.DueEscalationProcesses(at)
	if err != nil {
		return EscalationRunResult{}, err
	}
	result := EscalationRunResult{Due: len(due)}
	byID := map[int64]Workflow{}
	for _, workflow := range workflows {
		byID[workflow.ID] = workflow
	}
	for _, process := range due {
		workflow, ok := byID[process.WorkflowID]
		if !ok {
			result.Skipped++
			continue
		}
		processCtx := ctx
		processCtx.Model = process.Model
		processCtx.RecordID = process.RecordID
		processCtx.Now = at
		if _, _, _, err := s.ApplyEscalationForRecord(workflow, process, processCtx, hooks); err != nil {
			return result, err
		}
		result.Applied++
	}
	return result, nil
}

func (s ProcessStore) AppendApprovalLog(event ApprovalLogEvent) (int64, error) {
	if s.Env == nil {
		return 0, fmt.Errorf("workflow process store requires record env")
	}
	at := event.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	description := event.Details["description"]
	if description == "" {
		description = "Workflow transition"
	}
	if event.Committee {
		description = "Workflow committee approval"
	}
	values := map[string]any{
		"model":                  event.Model,
		"record_id":              event.RecordID,
		"user_id":                event.UserID,
		"date":                   at,
		"description":            description,
		"old_node_id":            event.OldNodeID,
		"new_node_id":            event.NewNodeID,
		"workflow_transition_id": event.TransitionID,
		"delegation_id":          event.DelegationID,
		"delegation_employee_id": event.DelegationEmployeeID,
	}
	for _, key := range []string{"old_state", "new_state", "old_status", "new_status", "duration"} {
		if value := event.Details[key]; value != "" {
			values[key] = value
		}
	}
	if duration := event.Details["duration_seconds"]; duration != "" {
		seconds, err := strconv.ParseFloat(duration, 64)
		if err != nil {
			return 0, fmt.Errorf("approval log duration_seconds: %w", err)
		}
		values["duration_seconds"] = seconds
	}
	if duration := event.Details["duration_hours"]; duration != "" {
		hours, err := strconv.ParseFloat(duration, 64)
		if err != nil {
			return 0, fmt.Errorf("approval log duration_hours: %w", err)
		}
		values["duration_hours"] = hours
	}
	return s.Env.Model(ModelLog).Create(values)
}

func (s ProcessStore) ApprovalLogHook() ApprovalLogHook {
	return func(event ApprovalLogEvent) error {
		_, err := s.AppendApprovalLog(event)
		return err
	}
}

func chainedApprovalLogHook(first ApprovalLogHook, second ApprovalLogHook) ApprovalLogHook {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(event ApprovalLogEvent) error {
		if err := first(event); err != nil {
			return err
		}
		return second(event)
	}
}

func (s ProcessStore) searchProcess(modelName string, recordID int64) (record.RecordSet, error) {
	return s.processEnv().Model(ModelProcess).Search(domain.And(
		domain.Cond("model", domain.Equal, modelName),
		domain.Cond("record_id", domain.Equal, recordID),
	))
}

func (s ProcessStore) processEnv() *record.Env {
	ctx := s.Env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	return s.Env.WithContext(ctx)
}

func processValues(process Process) map[string]any {
	values := map[string]any{
		"workflow_id":            process.WorkflowID,
		"model":                  process.Model,
		"record_id":              process.RecordID,
		"node_id":                process.NodeID,
		"active":                 process.Active,
		"state":                  process.State,
		"last_transition_id":     process.LastTransitionID,
		"started_at":             nil,
		"updated_at":             nil,
		"approval_user_ids":      append([]int64(nil), process.ApprovalUserIDs...),
		"approval_done_user_ids": append([]int64(nil), process.ApprovalDoneUserIDs...),
		"approval_partner_ids":   append([]int64(nil), process.ApprovalPartnerIDs...),
		"user_can_approve":       process.UserCanApprove,
		"escalation_date":        nil,
	}
	if !process.StartedAt.IsZero() {
		values["started_at"] = process.StartedAt
	}
	if !process.UpdatedAt.IsZero() {
		values["updated_at"] = process.UpdatedAt
	}
	if !process.EscalationDate.IsZero() {
		values["escalation_date"] = process.EscalationDate
	}
	return values
}

func processFromRow(row map[string]any) Process {
	return Process{
		WorkflowID:          int64Value(row["workflow_id"]),
		Model:               stringValue(row["model"]),
		RecordID:            int64Value(row["record_id"]),
		NodeID:              int64Value(row["node_id"]),
		State:               stringValue(row["state"]),
		Active:              boolValue(row["active"]),
		ApprovalUserIDs:     int64SliceValue(row["approval_user_ids"]),
		ApprovalDoneUserIDs: int64SliceValue(row["approval_done_user_ids"]),
		ApprovalPartnerIDs:  int64SliceValue(row["approval_partner_ids"]),
		UserCanApprove:      boolValue(row["user_can_approve"]),
		EscalationDate:      timeValue(row["escalation_date"]),
		LastTransitionID:    int64Value(row["last_transition_id"]),
		StartedAt:           timeValue(row["started_at"]),
		UpdatedAt:           timeValue(row["updated_at"]),
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	out, _ := value.(string)
	return out
}

func boolValue(value any) bool {
	out, _ := value.(bool)
	return out
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func int64SliceValue(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []int:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64(item))
		}
		return out
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64Value(item))
		}
		return out
	default:
		return nil
	}
}

func timeValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	default:
		return time.Time{}
	}
}
