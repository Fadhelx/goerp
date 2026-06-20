package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/actions"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/record"
	"gorp/internal/security"
)

var ErrUnhandledDispatch = errors.New("workflow dispatch unhandled")

const stateWriteHookSkipContextKey = "workflow_skip_state_write_hook"

type ApprovalDelegationProvider interface {
	DelegatedApprovalUserIDs(delegatorUserIDs []int64, approvalGroupIDs []int64, departmentIDs []int64, at time.Time) []int64
	ActiveApprovalDelegationID(delegateUserID int64, delegatorUserIDs []int64, approvalGroupIDs []int64, departmentIDs []int64, at time.Time) int64
}

type Dispatcher struct {
	Actions     *actions.Registry
	Mailer      Mailer
	Delegations ApprovalDelegationProvider
	Now         func() time.Time
}

type DispatchRequest struct {
	Model  string
	Method string
	Args   []any
	Kwargs map[string]any
	Values map[string]any
}

func (d Dispatcher) DispatchCall(ctx context.Context, env *record.Env, req DispatchRequest) (any, bool, error) {
	switch req.Method {
	case "approval_action_button":
		result, err := d.dispatchApprovalButton(ctx, env, req)
		return result, true, err
	case "approval_transition_button":
		result, err := d.dispatchTransitionButton(ctx, env, req)
		return result, true, err
	case "approval_transition_wizard":
		result, err := d.dispatchTransitionWizard(ctx, env, req)
		return result, true, err
	case "action_approve_all":
		result, err := d.dispatchApproveAll(ctx, env, req)
		return result, true, err
	case "process":
		switch req.Model {
		case ModelProcessWizard:
			result, err := d.dispatchProcessWizard(ctx, env, req)
			return result, true, err
		case ModelWorkflowWizard:
			result, err := d.dispatchWorkflowWizard(ctx, env, req)
			return result, true, err
		default:
			return nil, false, nil
		}
	case "action_update":
		if req.Model != ModelStateUpdateWizard {
			return nil, false, nil
		}
		result, err := d.dispatchStateUpdateWizard(ctx, env, req)
		return result, true, err
	default:
		return nil, false, nil
	}
}

func (d Dispatcher) dispatchApprovalButton(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	ids := idsFromAny(arg(req.Args, 0))
	if len(ids) == 0 {
		return nil, fmt.Errorf("approval_action_button requires record ids")
	}
	buttonID := int64FromAny(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "button_id"), req.Values["button_id"]))
	if buttonID == 0 {
		return nil, fmt.Errorf("approval_action_button requires button_id")
	}
	return d.runButtons(ctx, env, req.Model, ids, buttonID, inputFromRequest(req), false)
}

func (d Dispatcher) dispatchTransitionButton(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	ids := idsFromAny(arg(req.Args, 0))
	if len(ids) == 0 {
		return nil, fmt.Errorf("approval_transition_button requires record ids")
	}
	transitionID := int64FromAny(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "transition_id"), req.Values["transition_id"]))
	if transitionID == 0 {
		return nil, fmt.Errorf("approval_transition_button requires transition_id")
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return nil, err
	}
	store := NewProcessStore(env)
	userCtx := evalContextBase(env, req.Model)
	hooks := d.advancedHooks(store, env)
	ignoreComment := boolFromAny(firstNonNil(kwarg(req.Kwargs, "ignore_comment"), req.Values["ignore_comment"]))
	for _, id := range ids {
		process, ok, err := store.Find(req.Model, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("workflow process not found for %s:%d", req.Model, id)
		}
		workflow, ok := workflowByID(workflows, process.WorkflowID)
		if !ok {
			return nil, fmt.Errorf("workflow %d not found", process.WorkflowID)
		}
		transition, ok := workflowTransitionByID(workflow, transitionID)
		if !ok || transition.NodeID != process.NodeID {
			return softReloadAction(), nil
		}
		if transition.Comment != "" && !ignoreComment {
			node, ok := workflow.nodeByID(transition.NodeID)
			if !ok {
				return softReloadAction(), nil
			}
			return workflowNodeWizardAction(req.Model, id, []int64{id}, node, transition.ID), nil
		}
		ctx, err := evalContextForRecord(env, req.Model, id, userCtx)
		if err != nil {
			return nil, err
		}
		ctx.DelegationID = advancedApprovalDelegationID(env, workflow, process, d.Delegations, d.now())
		runAsSuperuser := transitionWillRunAsSuperuser(process, transition, ctx)
		writeEnv := workflowRunAsSuperuserEnv(env, runAsSuperuser)
		applyStore := workflowRunAsSuperuserStore(store, writeEnv, runAsSuperuser)
		updated, results, _, err := applyStore.ApplyTransitionForRecord(workflow, process, transitionID, ctx, hooks)
		if err != nil {
			return nil, err
		}
		if action, ok := firstActionResult(results, "mail.compose"); ok {
			if err := d.persistAdvancedRecordState(writeEnv, workflow, updated); err != nil {
				return nil, err
			}
			return action.Payload, nil
		}
		if err := d.persistAdvancedRecordState(writeEnv, workflow, updated); err != nil {
			return nil, err
		}
		if updated.NodeID != process.NodeID || !updated.Active {
			if err := deactivateActiveForwards(writeEnv, req.Model, id); err != nil {
				return nil, err
			}
		}
	}
	return softReloadAction(), nil
}

func (d Dispatcher) dispatchTransitionWizard(_ context.Context, env *record.Env, req DispatchRequest) (any, error) {
	ids := idsFromAny(arg(req.Args, 0))
	if len(ids) == 0 {
		return nil, fmt.Errorf("approval_transition_wizard requires record ids")
	}
	nodeID := int64FromAny(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "node_id"), req.Values["node_id"]))
	if nodeID == 0 {
		return nil, fmt.Errorf("approval_transition_wizard requires node_id")
	}
	transitionID := int64FromAny(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "transition_id"), req.Values["transition_id"]))
	recordID := ids[0]
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return nil, err
	}
	store := NewProcessStore(env)
	process, ok, err := store.Find(req.Model, recordID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return softReloadAction(), nil
	}
	workflow, ok := workflowByID(workflows, process.WorkflowID)
	if !ok {
		return softReloadAction(), nil
	}
	node, ok := workflow.nodeByID(nodeID)
	if !ok || process.NodeID != node.ID {
		return softReloadAction(), nil
	}
	ctx, err := evalContextForRecord(env, req.Model, recordID, evalContextBase(env, req.Model))
	if err != nil {
		return nil, err
	}
	available, err := workflow.AvailableTransitions(process, ctx)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return softReloadAction(), nil
	}
	return workflowNodeWizardAction(req.Model, recordID, ids, node, transitionID), nil
}

func (d Dispatcher) dispatchApproveAll(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	ids := idsFromAny(arg(req.Args, 0))
	if len(ids) == 0 {
		return softReloadAction(), nil
	}
	engine, err := d.engineFromEnv(env)
	if err != nil {
		return nil, err
	}
	user := userFromEnv(env)
	for _, id := range ids {
		wrec, err := workflowRecordFromEnv(env, req.Model, id, engine, d.Delegations, d.now())
		if err != nil {
			return nil, err
		}
		button, ok, err := engine.firstButton(user, wrec, ActionApprove)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		before := effectCounts(engine)
		result, err := engine.RunButton(ctx, user, &wrec, button.ID, Input{BulkApproval: true})
		if err != nil {
			return nil, err
		}
		if err := persistWorkflowResult(env, engine, before, wrec, d.Delegations, d.now()); err != nil {
			return nil, err
		}
		if wrec.State != result.OldState {
			if err := deactivateActiveForwards(env, req.Model, id); err != nil {
				return nil, err
			}
		}
	}
	return softReloadAction(), nil
}

func (d Dispatcher) dispatchProcessWizard(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	wizardIDs := idsFromAny(arg(req.Args, 0))
	if len(wizardIDs) == 0 {
		return nil, fmt.Errorf("approval.process.wizard process requires wizard ids")
	}
	rows, err := env.Model(ModelProcessWizard).Browse(wizardIDs...).Read("res_model", "model", "res_ids", "record_ids", "button_id", "comment", "return_state", "transfer_state", "forward_user_id", "target_user_id")
	if err != nil {
		return nil, err
	}
	var last any = closeWindowAction()
	for _, row := range rows {
		modelName := firstString(row["res_model"], row["model"])
		ids := idsFromAny(firstNonNil(row["res_ids"], row["record_ids"]))
		buttonID := int64FromAny(row["button_id"])
		input := Input{
			Comment:      stringFromAny(row["comment"]),
			TargetUserID: firstInt64(row["forward_user_id"], row["target_user_id"]),
			Values: map[string]any{
				"return_state":   stringFromAny(row["return_state"]),
				"transfer_state": stringFromAny(row["transfer_state"]),
			},
		}
		result, err := d.runButtons(ctx, env, modelName, ids, buttonID, input, false)
		if err != nil {
			return nil, err
		}
		last = result
	}
	return last, nil
}

func (d Dispatcher) dispatchWorkflowWizard(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	wizardIDs := idsFromAny(arg(req.Args, 0))
	if len(wizardIDs) == 0 {
		return nil, fmt.Errorf("workflow.process.wizard process requires wizard ids")
	}
	rows, err := env.Model(ModelWorkflowWizard).Browse(wizardIDs...).Read("model", "record_id", "workflow_transition_id", "comment")
	if err != nil {
		return nil, err
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return nil, err
	}
	store := NewProcessStore(env)
	userCtx := evalContextBase(env, "")
	var last any = softReloadAction()
	for _, row := range rows {
		modelName := stringFromAny(row["model"])
		recordID := int64FromAny(row["record_id"])
		transitionID := int64FromAny(row["workflow_transition_id"])
		comment := stringFromAny(row["comment"])
		if modelName == "" || recordID == 0 || transitionID == 0 {
			return nil, fmt.Errorf("workflow.process.wizard requires model, record_id, and workflow_transition_id")
		}
		process, ok, err := store.Find(modelName, recordID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("workflow process not found for %s:%d", modelName, recordID)
		}
		workflow, ok := workflowByID(workflows, process.WorkflowID)
		if !ok {
			return nil, fmt.Errorf("workflow %d not found", process.WorkflowID)
		}
		transition, ok := workflowTransitionByID(workflow, transitionID)
		if !ok {
			return nil, fmt.Errorf("workflow transition %d not found", transitionID)
		}
		base := userCtx
		base.Model = modelName
		ctx, err := evalContextForRecord(env, modelName, recordID, base)
		if err != nil {
			return nil, err
		}
		ctx.DelegationID = advancedApprovalDelegationID(env, workflow, process, d.Delegations, d.now())
		hooks := d.advancedHooks(store, env)
		if err := writeKnown(env, modelName, recordID, map[string]any{"_approval_comment": comment}); err != nil {
			return nil, err
		}
		if strings.TrimSpace(comment) != "" {
			if _, err := store.AppendApprovalLog(ApprovalLogEvent{
				At:           time.Now().UTC(),
				UserID:       env.Context().UserID,
				WorkflowID:   process.WorkflowID,
				Model:        modelName,
				RecordID:     recordID,
				OldNodeID:    process.NodeID,
				NewNodeID:    process.NodeID,
				DelegationID: ctx.DelegationID,
				Details:      map[string]string{"description": comment},
			}); err != nil {
				return nil, err
			}
		}
		ctx.MailComposed = boolFromAny(firstNonNil(kwarg(req.Kwargs, "mail_compose"), req.Values["mail_compose"]))
		runAsSuperuser := transitionWillRunAsSuperuser(process, transition, ctx)
		writeEnv := workflowRunAsSuperuserEnv(env, runAsSuperuser)
		applyStore := workflowRunAsSuperuserStore(store, writeEnv, runAsSuperuser)
		updated, results, _, err := applyStore.ApplyTransitionForRecord(workflow, process, transitionID, ctx, hooks)
		if err != nil {
			return nil, err
		}
		if action, ok := firstActionResult(results, "mail.compose"); ok {
			if err := d.persistAdvancedRecordState(writeEnv, workflow, updated); err != nil {
				return nil, err
			}
			return action.Payload, nil
		}
		if err := d.persistAdvancedRecordState(writeEnv, workflow, updated); err != nil {
			return nil, err
		}
		if updated.NodeID != process.NodeID || !updated.Active {
			if err := deactivateActiveForwards(writeEnv, modelName, recordID); err != nil {
				return nil, err
			}
		}
		last = softReloadAction()
	}
	return last, nil
}

func (d Dispatcher) dispatchStateUpdateWizard(ctx context.Context, env *record.Env, req DispatchRequest) (any, error) {
	wizardIDs := idsFromAny(arg(req.Args, 0))
	if len(wizardIDs) == 0 {
		return nil, fmt.Errorf("approval.state.update action_update requires wizard ids")
	}
	rows, err := env.Model(ModelStateUpdateWizard).Browse(wizardIDs...).Read("res_model", "model", "res_ids", "record_id", "state", "comment", "workflow_model", "workflow_node_id", "workflow_id")
	if err != nil {
		return nil, err
	}
	advancedUpdated := false
	for _, row := range rows {
		modelName := firstString(row["res_model"], row["model"])
		ids := idsFromAny(firstNonNil(row["res_ids"], row["record_id"]))
		newState := stringFromAny(row["state"])
		workflowModel := boolFromAny(row["workflow_model"])
		if !workflowModel {
			var err error
			workflowModel, err = stateUpdateWizardModelIsAdvanced(env, modelName)
			if err != nil {
				return nil, err
			}
		}
		if workflowModel {
			if err := d.applyAdvancedStateUpdateWizard(env, modelName, ids, int64FromAny(row["workflow_node_id"]), int64FromAny(row["workflow_id"]), newState); err != nil {
				return nil, err
			}
			advancedUpdated = true
			continue
		}
		for _, id := range ids {
			engine, err := d.engineFromEnv(env)
			if err != nil {
				return nil, err
			}
			settings, ok := engine.SettingsForModel(modelName)
			if !ok {
				if err := writeKnown(env, modelName, id, map[string]any{"state": newState}); err != nil {
					return nil, err
				}
				continue
			}
			wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
			if err != nil {
				return nil, err
			}
			oldState := wrec.State
			if err := writeKnown(env, modelName, id, map[string]any{settings.StateField: newState, "state": newState}); err != nil {
				return nil, err
			}
			if _, err := d.runStateUpdatedWrite(env, engine, settings, modelName, id, oldState, Input{Comment: stringFromAny(row["comment"])}); err != nil {
				return nil, err
			}
		}
	}
	if advancedUpdated {
		return softReloadAction(), nil
	}
	return closeWindowAction(), nil
}

func (d Dispatcher) applyAdvancedStateUpdateWizard(env *record.Env, modelName string, ids []int64, nodeID int64, workflowID int64, state string) error {
	if modelName == "" || len(ids) == 0 {
		return nil
	}
	nodeState := ""
	if nodeID != 0 {
		var err error
		nodeState, workflowID, err = stateUpdateWizardNodeInfo(env, nodeID)
		if err != nil {
			return err
		}
		if state == "" {
			state = nodeState
		}
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return err
	}
	store := NewProcessStore(env)
	for _, id := range ids {
		oldWorkflowID, oldNodeID, err := stateUpdateWizardRecordWorkflow(env, modelName, id)
		if err != nil {
			return err
		}
		if workflowID == 0 {
			workflowID = oldWorkflowID
		}
		if err := writeKnown(env, modelName, id, map[string]any{"_old_workflow_node_id": oldNodeID}); err != nil {
			return err
		}
		if nodeID == 0 {
			process, ok, err := store.Find(modelName, id)
			if err != nil {
				return err
			}
			oldState := ""
			if ok {
				oldState = process.State
				process.NodeID = 0
				process.State = state
				process.Active = true
				process.LastTransitionID = 0
				process.UpdatedAt = d.now()
				if _, err := store.Save(process); err != nil {
					return err
				}
			}
			if err := writeKnown(env, modelName, id, map[string]any{"state": state, "workflow_node_id": nil, "workflow_id": nil}); err != nil {
				return err
			}
			if oldState != "" && state != "" && oldState != state {
				engine, err := d.engineFromEnv(env)
				if err != nil {
					return err
				}
				if settings, ok := engine.SettingsForModel(modelName); ok {
					if _, err := d.runStateUpdatedWrite(env, engine, settings, modelName, id, oldState, Input{}); err != nil {
						return err
					}
				}
			}
			continue
		}
		workflow, ok := workflowByID(workflows, workflowID)
		if !ok {
			return fmt.Errorf("workflow %d not found for approval.state.update", workflowID)
		}
		process, ok, err := store.Find(modelName, id)
		if err != nil {
			return err
		}
		now := d.now()
		if !ok {
			process = Process{
				WorkflowID: workflowID,
				Model:      modelName,
				RecordID:   id,
				Active:     true,
				StartedAt:  now,
			}
		}
		oldState := process.State
		if oldState == "" {
			oldState = state
		}
		process.WorkflowID = workflowID
		process.NodeID = nodeID
		process.State = state
		process.Active = true
		process.LastTransitionID = 0
		process.UpdatedAt = now
		if err := d.persistAdvancedRecordState(env, workflow, process); err != nil {
			return err
		}
		if oldState != "" && state != "" && oldState != state {
			engine, err := d.engineFromEnv(env)
			if err != nil {
				return err
			}
			if settings, ok := engine.SettingsForModel(modelName); ok {
				if _, err := d.runStateUpdatedWrite(env, engine, settings, modelName, id, oldState, Input{}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d Dispatcher) runButtons(ctx context.Context, env *record.Env, modelName string, ids []int64, buttonID int64, input Input, forceClose bool) (any, error) {
	engine, err := d.engineFromEnv(env)
	if err != nil {
		return nil, err
	}
	user := userFromEnv(env)
	if !input.MailComposed {
		if settings, ok := engine.SettingsForModel(modelName); ok {
			if button, ok := engine.Buttons[buttonID]; ok && button.SettingsID == settings.ID && classicProcessWizardNeeded(button, input) {
				for _, id := range ids {
					wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
					if err != nil {
						return nil, err
					}
					visible, err := engine.buttonVisible(user, wrec, settings, button)
					if err != nil {
						return nil, err
					}
					if !visible {
						return nil, ErrButtonHidden
					}
				}
				return classicProcessWizardAction(modelName, ids, button), nil
			}
		}
	}
	if action, ok, err := d.classicEmailComposeAction(env, engine, user, modelName, ids, buttonID, input); err != nil || ok {
		return action, err
	}
	var last TransitionResult
	for _, id := range ids {
		button, hasButton := engine.Buttons[buttonID]
		wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
		if err != nil {
			return nil, err
		}
		if hasButton && button.Action == ActionForward {
			if !userCanReadRecord(env, modelName, id, input.TargetUserID) {
				return nil, fmt.Errorf("forward target user %d cannot read %s:%d", input.TargetUserID, modelName, id)
			}
		}
		before := effectCounts(engine)
		result, err := engine.RunButton(ctx, user, &wrec, buttonID, input)
		if err != nil {
			return nil, err
		}
		writeEnv := env
		if hasButton && button.RunAsSuperuser {
			writeEnv = workflowSudoEnv(env)
		}
		if err := persistWorkflowResult(writeEnv, engine, before, wrec, d.Delegations, d.now()); err != nil {
			return nil, err
		}
		if result.NewState != result.OldState {
			if err := deactivateActiveForwards(writeEnv, modelName, id); err != nil {
				return nil, err
			}
		}
		last = result
	}
	if forceClose {
		return closeWindowAction(), nil
	}
	return dispatchPayload(last), nil
}

func (d Dispatcher) classicEmailComposeAction(env *record.Env, engine *Engine, user User, modelName string, ids []int64, buttonID int64, input Input) (map[string]any, bool, error) {
	if input.MailComposed || engine == nil || len(ids) == 0 {
		return nil, false, nil
	}
	settings, ok := engine.SettingsForModel(modelName)
	if !ok {
		return nil, false, nil
	}
	button, ok := engine.Buttons[buttonID]
	if !ok || button.SettingsID != settings.ID || button.Action != ActionEmail {
		return nil, false, nil
	}
	for _, id := range ids {
		wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
		if err != nil {
			return nil, true, err
		}
		visible, err := engine.buttonVisible(user, wrec, settings, button)
		if err != nil {
			return nil, true, err
		}
		if !visible {
			return nil, true, ErrButtonHidden
		}
	}
	if button.EmailWizardFormID == 0 {
		button.EmailWizardFormID = defaultMailComposeFormID(env)
	}
	return classicMailComposeAction(modelName, ids, button), true, nil
}

func (d Dispatcher) engineFromEnv(env *record.Env) (*Engine, error) {
	engine := NewEngine()
	engine.SetNow(d.now)
	engine.Actions = d.Actions
	engine.Mailer = d.Mailer
	_ = engine.RegisterMethod("expire_delegation", func(_ context.Context, rec *Record, _ Button, _ Input) error {
		if rec == nil || rec.Model != "delegation" {
			return nil
		}
		return nil
	})
	if err := loadSettings(env, engine); err != nil {
		return nil, err
	}
	if err := loadConfigs(env, engine); err != nil {
		return nil, err
	}
	if err := loadButtons(env, engine); err != nil {
		return nil, err
	}
	if err := loadAutomations(env, engine); err != nil {
		return nil, err
	}
	return engine, nil
}

func classicProcessWizardNeeded(button Button, input Input) bool {
	if button.CommentRequired && strings.TrimSpace(input.Comment) == "" {
		return true
	}
	switch button.Action {
	case ActionForward, ActionTransfer:
		return input.TargetUserID == 0
	case ActionReturn:
		return button.ReturnState == "" && strings.TrimSpace(stringFromAny(input.Values["return_state"])) == ""
	default:
		return false
	}
}

func classicProcessWizardAction(modelName string, recordIDs []int64, button Button) map[string]any {
	return map[string]any{
		"type":      "ir.actions.act_window",
		"name":      button.Name,
		"res_model": ModelProcessWizard,
		"target":    "new",
		"view_mode": "form",
		"context": map[string]any{
			"default_button_id":       button.ID,
			"default_res_model":       modelName,
			"default_res_ids":         append([]int64(nil), recordIDs...),
			"default_confirm_message": button.ConfirmMessage,
			"active_ids":              append([]int64(nil), recordIDs...),
			"active_model":            modelName,
		},
	}
}

func loadSettings(env *record.Env, engine *Engine) error {
	rows, err := allRows(env, ModelSettings, "name", "model", "active", "state_field", "draft_state", "approved_state", "rejected_state", "cancelled_state", "company_id")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		engine.AddSettings(Settings{
			ID:             int64FromAny(row["id"]),
			Name:           stringFromAny(row["name"]),
			Model:          stringFromAny(row["model"]),
			Active:         true,
			StateField:     stringFromAny(row["state_field"]),
			DraftState:     stringFromAny(row["draft_state"]),
			ApprovedState:  stringFromAny(row["approved_state"]),
			RejectedState:  stringFromAny(row["rejected_state"]),
			CancelledState: stringFromAny(row["cancelled_state"]),
			CompanyID:      int64FromAny(row["company_id"]),
		})
	}
	return nil
}

func loadConfigs(env *record.Env, engine *Engine) error {
	rows, err := allRows(env, ModelConfig, "name", "setting_id", "settings_id", "state", "sequence", "group_ids", "user_ids", "user_python_code", "condition", "auto_approve", "committee", "committee_limit", "committee_vote_percentage", "is_voting", "schedule_activity", "schedule_activity_field_id", "schedule_activity_days", "active")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		committeeLimit := int(int64FromAny(row["committee_limit"]))
		isVoting := boolFromAny(row["is_voting"])
		if committeeLimit > 0 && isVoting {
			return fmt.Errorf("approval.config %d committee_limit cannot be set when voting is enabled", int64FromAny(row["id"]))
		}
		condition, err := parseDomainText(stringFromAny(row["condition"]))
		if err != nil {
			return fmt.Errorf("approval.config %d condition: %w", int64FromAny(row["id"]), err)
		}
		engine.AddConfig(ApprovalConfig{
			ID:                      int64FromAny(row["id"]),
			SettingsID:              firstInt64(row["settings_id"], row["setting_id"]),
			State:                   stringFromAny(row["state"]),
			Name:                    stringFromAny(row["name"]),
			Active:                  true,
			Sequence:                int(int64FromAny(row["sequence"])),
			GroupIDs:                idsFromAny(row["group_ids"]),
			UserIDs:                 idsFromAny(row["user_ids"]),
			UserPythonCode:          stringFromAny(row["user_python_code"]),
			Condition:               condition,
			AutoApprove:             boolFromAny(row["auto_approve"]),
			Committee:               boolFromAny(row["committee"]),
			CommitteeLimit:          committeeLimit,
			CommitteeVotePercentage: float64FromAny(row["committee_vote_percentage"]),
			IsVoting:                isVoting,
			ScheduleActivity:        boolFromAny(row["schedule_activity"]),
			ScheduleActivityField:   modelFieldName(env, int64FromAny(row["schedule_activity_field_id"])),
			ScheduleActivityDays:    int(int64FromAny(row["schedule_activity_days"])),
		})
	}
	return nil
}

func loadButtons(env *record.Env, engine *Engine) error {
	rows, err := allRows(env, ModelButton, "settings_id", "config_id", "model", "sequence", "active", "name", "action_type", "state_value", "next_state", "return_state", "transfer_state", "visible_to", "method", "visible_domain", "server_action_id", "email_template_id", "email_wizard_form_id", "email_next_action", "group_ids", "comment", "comment_required", "confirm_message", "button_class", "vote_threshold", "voting_type", "run_as_superuser")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		visibleDomain, err := parseDomainText(stringFromAny(row["visible_domain"]))
		if err != nil {
			return fmt.Errorf("approval.buttons %d visible_domain: %w", int64FromAny(row["id"]), err)
		}
		commentRequired := boolFromAny(row["comment_required"]) || stringFromAny(row["comment"]) == "required"
		action := buttonActionFromString(stringFromAny(row["action_type"]))
		votingType := stringFromAny(row["voting_type"])
		if strings.TrimSpace(votingType) != "" {
			action = ActionApprove
		}
		engine.AddButton(Button{
			ID:                int64FromAny(row["id"]),
			SettingsID:        int64FromAny(row["settings_id"]),
			StateValue:        firstString(row["state_value"], row["states"]),
			Name:              stringFromAny(row["name"]),
			Action:            action,
			NextState:         stringFromAny(row["next_state"]),
			ReturnState:       stringFromAny(row["return_state"]),
			TransferState:     stringFromAny(row["transfer_state"]),
			MethodName:        stringFromAny(row["method"]),
			ServerActionID:    int64FromAny(row["server_action_id"]),
			EmailTemplateID:   int64FromAny(row["email_template_id"]),
			EmailWizardFormID: int64FromAny(row["email_wizard_form_id"]),
			EmailNextAction:   buttonActionFromString(stringFromAny(row["email_next_action"])),
			GroupIDs:          idsFromAny(row["group_ids"]),
			VisibleTo:         stringFromAny(row["visible_to"]),
			VisibleDomain:     visibleDomain,
			CommentRequired:   commentRequired,
			ConfirmMessage:    stringFromAny(row["confirm_message"]),
			ButtonClass:       stringFromAny(row["button_class"]),
			Sequence:          int(int64FromAny(row["sequence"])),
			VoteThreshold:     int(int64FromAny(row["vote_threshold"])),
			VotingType:        votingType,
			RunAsSuperuser:    boolFromAny(row["run_as_superuser"]),
		})
	}
	return nil
}

func loadAutomations(env *record.Env, engine *Engine) error {
	rows, err := allRows(env, ModelAutomation, "settings_id", "model", "sequence", "active", "name", "trigger", "from_states", "to_states", "code", "filter_domain", "template_ids", "server_action_ids")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		filter, err := parseDomainText(stringFromAny(row["filter_domain"]))
		if err != nil {
			return fmt.Errorf("approval.automation %d filter_domain: %w", int64FromAny(row["id"]), err)
		}
		engine.AddAutomation(Automation{
			ID:              int64FromAny(row["id"]),
			SettingsID:      int64FromAny(row["settings_id"]),
			Name:            stringFromAny(row["name"]),
			Model:           stringFromAny(row["model"]),
			Sequence:        int(int64FromAny(row["sequence"])),
			Active:          true,
			Trigger:         AutomationTrigger(stringFromAny(row["trigger"])),
			FromStates:      stringsFromAny(row["from_states"]),
			ToStates:        stringsFromAny(row["to_states"]),
			Filter:          filter,
			Code:            stringFromAny(row["code"]),
			ServerActionIDs: idsFromAny(row["server_action_ids"]),
			TemplateIDs:     idsFromAny(row["template_ids"]),
		})
	}
	return nil
}

func loadAdvancedWorkflows(env *record.Env) ([]Workflow, error) {
	workflowRows, err := allRows(env, ModelWorkflow, "name", "code", "approval_settings_id", "model", "sequence", "active", "condition", "state", "view_id", "create_context", "on_create", "company_ids", "start_node_id")
	if err != nil {
		return nil, err
	}
	nodeRows, err := allRows(env, ModelNode, "name", "code", "workflow_id", "model", "sequence", "active", "type", "responsible_group_ids", "responsible_user_ids", "responsible_python_code", "responsible_value", "responsible_filter", "responsible_condition", "responsible_committee", "responsible_committee_limit", "schedule_activity", "schedule_activity_enabled", "schedule_activity_field_id", "schedule_activity_days", "state", "button_type", "button_name", "button_context", "button_icon", "button_validate_form", "wizard_view_id", "allow_forward", "escalation", "escalation_delay_type", "escalation_delay", "escalation_node_id", "trg_date_calendar_id")
	if err != nil {
		return nil, err
	}
	calendarAttendances := loadResourceCalendarAttendances(env)
	transitionRows, err := allRows(env, ModelTransition, "name", "code", "node_id", "workflow_id", "sequence", "active", "run_as_superuser", "condition", "next_node_id", "groups_ids", "comment", "button_class", "wizard_view_id", "context", "icon", "committee", "committee_limit", "validate_form", "trigger", "is_email", "email_template_id", "email_wizard_form_id", "is_hidden")
	if err != nil {
		return nil, err
	}
	actionRows, err := allRows(env, ModelNodeAction, "node_id", "sequence", "active", "condition", "server_action_id", "action_key")
	if err != nil {
		return nil, err
	}

	nodesByWorkflow := map[int64][]Node{}
	nodeIndex := map[int64]*Node{}
	for _, row := range nodeRows {
		if row["active"] == false {
			continue
		}
		node := Node{
			ID:                        int64FromAny(row["id"]),
			Name:                      stringFromAny(row["name"]),
			Code:                      stringFromAny(row["code"]),
			WorkflowID:                int64FromAny(row["workflow_id"]),
			Sequence:                  int(int64FromAny(row["sequence"])),
			Active:                    true,
			Type:                      NodeType(stringFromAny(row["type"])),
			ResponsibleGroupIDs:       idsFromAny(row["responsible_group_ids"]),
			ResponsibleUserIDs:        idsFromAny(row["responsible_user_ids"]),
			ResponsiblePythonCode:     stringFromAny(row["responsible_python_code"]),
			ResponsibleValue:          stringFromAny(row["responsible_value"]),
			ResponsibleFilter:         stringFromAny(row["responsible_filter"]),
			ResponsibleCondition:      Expr(stringFromAny(row["responsible_condition"])),
			ResponsibleCommittee:      boolFromAny(row["responsible_committee"]),
			ResponsibleCommitteeLimit: int(int64FromAny(row["responsible_committee_limit"])),
			ScheduleActivity:          boolFromAny(row["schedule_activity"]) || boolFromAny(row["schedule_activity_enabled"]),
			ScheduleActivityField:     modelFieldName(env, int64FromAny(row["schedule_activity_field_id"])),
			ScheduleActivityDays:      int(int64FromAny(row["schedule_activity_days"])),
			State:                     stringFromAny(row["state"]),
			ButtonType:                ButtonType(stringFromAny(row["button_type"])),
			ButtonName:                stringFromAny(row["button_name"]),
			ButtonContext:             stringFromAny(row["button_context"]),
			ButtonIcon:                stringFromAny(row["button_icon"]),
			ButtonValidateForm:        boolFromAny(row["button_validate_form"]),
			WizardViewID:              int64FromAny(row["wizard_view_id"]),
			AllowForward:              boolFromAny(row["allow_forward"]),
			Escalation:                boolFromAny(row["escalation"]),
			EscalationDelayType:       DelayType(stringFromAny(row["escalation_delay_type"])),
			EscalationDelay:           int(int64FromAny(row["escalation_delay"])),
			EscalationNodeID:          int64FromAny(row["escalation_node_id"]),
			EscalationCalendarID:      int64FromAny(row["trg_date_calendar_id"]),
		}
		node.EscalationCalendar = append([]CalendarAttendance(nil), calendarAttendances[node.EscalationCalendarID]...)
		nodesByWorkflow[node.WorkflowID] = append(nodesByWorkflow[node.WorkflowID], node)
	}
	for workflowID, nodes := range nodesByWorkflow {
		for index := range nodes {
			nodeIndex[nodes[index].ID] = &nodesByWorkflow[workflowID][index]
		}
	}
	for _, row := range transitionRows {
		if row["active"] == false {
			continue
		}
		node := nodeIndex[int64FromAny(row["node_id"])]
		if node == nil {
			continue
		}
		node.Transitions = append(node.Transitions, Transition{
			ID:                int64FromAny(row["id"]),
			Name:              stringFromAny(row["name"]),
			Code:              stringFromAny(row["code"]),
			NodeID:            int64FromAny(row["node_id"]),
			Sequence:          int(int64FromAny(row["sequence"])),
			Active:            true,
			RunAsSuperuser:    boolFromAny(row["run_as_superuser"]),
			Condition:         Expr(stringFromAny(row["condition"])),
			NextNodeID:        int64FromAny(row["next_node_id"]),
			GroupIDs:          idsFromAny(row["groups_ids"]),
			Comment:           CommentMode(stringFromAny(row["comment"])),
			ButtonClass:       stringFromAny(row["button_class"]),
			WizardViewID:      int64FromAny(row["wizard_view_id"]),
			Context:           map[string]any{"raw": stringFromAny(row["context"])},
			Icon:              stringFromAny(row["icon"]),
			Committee:         boolFromAny(row["committee"]),
			CommitteeLimit:    int(int64FromAny(row["committee_limit"])),
			ValidateForm:      boolFromAny(row["validate_form"]),
			Trigger:           stringFromAny(row["trigger"]),
			IsEmail:           boolFromAny(row["is_email"]),
			EmailTemplateID:   int64FromAny(row["email_template_id"]),
			EmailWizardFormID: int64FromAny(row["email_wizard_form_id"]),
			IsHidden:          boolFromAny(row["is_hidden"]),
		})
	}
	for _, row := range actionRows {
		if row["active"] == false {
			continue
		}
		node := nodeIndex[int64FromAny(row["node_id"])]
		if node == nil {
			continue
		}
		node.Actions = append(node.Actions, NodeAction{
			ID:             int64FromAny(row["id"]),
			NodeID:         int64FromAny(row["node_id"]),
			Sequence:       int(int64FromAny(row["sequence"])),
			Active:         true,
			Condition:      Expr(stringFromAny(row["condition"])),
			ServerActionID: int64FromAny(row["server_action_id"]),
			ActionKey:      stringFromAny(row["action_key"]),
		})
	}

	workflows := make([]Workflow, 0, len(workflowRows))
	for _, row := range workflowRows {
		if row["active"] == false {
			continue
		}
		workflowID := int64FromAny(row["id"])
		workflows = append(workflows, Workflow{
			ID:                 workflowID,
			Name:               stringFromAny(row["name"]),
			Code:               stringFromAny(row["code"]),
			ApprovalSettingsID: int64FromAny(row["approval_settings_id"]),
			Model:              stringFromAny(row["model"]),
			Sequence:           int(int64FromAny(row["sequence"])),
			Active:             true,
			State:              stringFromAny(row["state"]),
			Condition:          Expr(stringFromAny(row["condition"])),
			ViewID:             int64FromAny(row["view_id"]),
			CreateContext:      ParseContextLiteral(stringFromAny(row["create_context"])),
			OnCreate:           boolFromAny(row["on_create"]),
			CompanyIDs:         idsFromAny(row["company_ids"]),
			StartNodeID:        int64FromAny(row["start_node_id"]),
			Nodes:              nodesByWorkflow[workflowID],
		})
	}
	return workflows, nil
}

func workflowByID(workflows []Workflow, id int64) (Workflow, bool) {
	for _, workflow := range workflows {
		if workflow.ID == id {
			return workflow, true
		}
	}
	return Workflow{}, false
}

func workflowTransitionByID(workflow Workflow, id int64) (Transition, bool) {
	for _, transition := range workflow.allTransitions() {
		if transition.ID == id {
			return transition, true
		}
	}
	return Transition{}, false
}

func (d Dispatcher) ProcessEscalations(_ context.Context, env *record.Env) (EscalationRunResult, error) {
	if env == nil {
		return EscalationRunResult{}, fmt.Errorf("workflow escalation requires record env")
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return EscalationRunResult{}, nil
		}
		return EscalationRunResult{}, err
	}
	store := NewProcessStore(env)
	at := d.now()
	due, err := store.DueEscalationProcesses(at)
	if err != nil {
		return EscalationRunResult{}, err
	}
	result := EscalationRunResult{Due: len(due)}
	base := evalContextBase(env, "")
	hooks := d.advancedHooks(store, env)
	for _, process := range due {
		workflow, ok := workflowByID(workflows, process.WorkflowID)
		if !ok {
			result.Skipped++
			continue
		}
		processCtx := base
		processCtx.Model = process.Model
		processCtx.RecordID = process.RecordID
		processCtx.Now = at
		recordCtx, err := evalContextForRecord(env, process.Model, process.RecordID, processCtx)
		if err != nil {
			return result, err
		}
		updated, _, _, err := store.ApplyEscalationForRecord(workflow, process, recordCtx, hooks)
		if err != nil {
			return result, err
		}
		if err := d.persistAdvancedRecordState(env, workflow, updated); err != nil {
			return result, err
		}
		if updated.NodeID != process.NodeID || !updated.Active {
			if err := deactivateActiveForwards(env, process.Model, process.RecordID); err != nil {
				return result, err
			}
		}
		result.Applied++
	}
	return result, nil
}

func (d Dispatcher) advancedHooks(_ ProcessStore, envs ...*record.Env) Hooks {
	return Hooks{
		Action: func(action NodeAction, process Process, ctx EvaluationContext) (ActionResult, error) {
			if d.Actions == nil || action.ServerActionID == 0 {
				return ActionResult{ActionID: action.ServerActionID, Key: action.ActionKey, Type: "server"}, nil
			}
			result, err := d.Actions.Run(context.Background(), action.ServerActionID, actions.ExecutionContext{
				Model:        process.Model,
				RecordID:     process.RecordID,
				RecordIDs:    []int64{process.RecordID},
				Values:       cloneMap(ctx.Values),
				UserID:       ctx.UserID,
				UserGroupIDs: append([]int64(nil), ctx.UserGroupIDs...),
				Sudo:         ctx.RunAsSuperuser,
				Trigger:      "workflow_node_action",
				Metadata: map[string]any{
					"workflow_id":      process.WorkflowID,
					"node_id":          process.NodeID,
					"action_id":        action.ID,
					"user_id":          ctx.UserID,
					"run_as_superuser": ctx.RunAsSuperuser,
					"sudo":             ctx.RunAsSuperuser,
				},
			})
			return ActionResult{ActionID: result.ActionID, Key: action.ActionKey, Type: string(result.Kind), Payload: result.Metadata}, err
		},
		MailCompose: func(transition Transition, process Process, _ EvaluationContext) (ActionResult, error) {
			if transition.EmailWizardFormID == 0 && len(envs) > 0 {
				transition.EmailWizardFormID = defaultMailComposeFormID(envs[0])
			}
			return ActionResult{
				ActionID: transition.EmailTemplateID,
				Key:      transition.Name,
				Type:     "mail.compose",
				Payload:  mailComposeAction(process, transition),
			}, nil
		},
	}
}

func workflowRunAsSuperuserEnv(env *record.Env, runAsSuperuser bool) *record.Env {
	if runAsSuperuser {
		return workflowSudoEnv(env)
	}
	return env
}

func workflowRunAsSuperuserStore(store ProcessStore, env *record.Env, runAsSuperuser bool) ProcessStore {
	if runAsSuperuser {
		return NewProcessStore(env)
	}
	return store
}

func transitionWillRunAsSuperuser(process Process, transition Transition, ctx EvaluationContext) bool {
	if !transition.RunAsSuperuser {
		return false
	}
	if !transition.Committee || ctx.UserID == 1 {
		return true
	}
	done := advancedCommitteeDoneUserIDs(process, ctx)
	return len(advancedCommitteeRemainingUserIDs(process, transition, done)) == 0
}

func defaultMailComposeFormID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", domain.Equal, "mail"),
		domain.Cond("name", domain.Equal, "email_compose_message_wizard_form"),
	))
	if err != nil {
		return 0
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["res_id"])
}

func firstActionResult(results []ActionResult, resultType string) (ActionResult, bool) {
	for _, result := range results {
		if result.Type == resultType {
			return result, true
		}
	}
	return ActionResult{}, false
}

func workflowNodeWizardAction(modelName string, recordID int64, activeIDs []int64, node Node, transitionID int64) map[string]any {
	if len(activeIDs) == 0 {
		activeIDs = []int64{recordID}
	}
	return map[string]any{
		"type":      "ir.actions.act_window",
		"name":      "Process Workflow",
		"res_model": ModelWorkflowWizard,
		"target":    "new",
		"view_mode": "form",
		"view_id":   node.WizardViewID,
		"context": map[string]any{
			"default_model":                  modelName,
			"default_record_id":              recordID,
			"default_workflow_transition_id": transitionID,
			"active_ids":                     append([]int64(nil), activeIDs...),
			"active_id":                      recordID,
			"active_model":                   modelName,
		},
	}
}

func mailComposeAction(process Process, transition Transition) map[string]any {
	templateID := any(false)
	if transition.EmailTemplateID != 0 {
		templateID = transition.EmailTemplateID
	}
	return map[string]any{
		"name":      transition.Name,
		"type":      "ir.actions.act_window",
		"view_mode": "form",
		"res_model": "mail.compose.message",
		"views":     []any{[]any{transition.EmailWizardFormID, "form"}},
		"view_id":   transition.EmailWizardFormID,
		"target":    "new",
		"context": map[string]any{
			"default_model":            process.Model,
			"default_res_ids":          []int64{process.RecordID},
			"default_template_id":      templateID,
			"default_composition_mode": "comment",
			"workflow_transition_id":   transition.ID,
		},
	}
}

func classicMailComposeAction(modelName string, recordIDs []int64, button Button) map[string]any {
	templateID := any(false)
	if button.EmailTemplateID != 0 {
		templateID = button.EmailTemplateID
	}
	return map[string]any{
		"name":      button.Name,
		"type":      "ir.actions.act_window",
		"view_mode": "form",
		"res_model": "mail.compose.message",
		"views":     []any{[]any{button.EmailWizardFormID, "form"}},
		"view_id":   button.EmailWizardFormID,
		"target":    "new",
		"context": map[string]any{
			"default_model":            modelName,
			"default_res_ids":          append([]int64(nil), recordIDs...),
			"default_template_id":      templateID,
			"default_composition_mode": "comment",
			"approval_button_id":       button.ID,
		},
	}
}

func (d Dispatcher) CompleteMailComposeTransition(env *record.Env, composeIDs []int64, contextValues map[string]any) (any, bool, error) {
	if buttonID := int64FromAny(contextValues["approval_button_id"]); buttonID != 0 {
		return d.completeClassicMailComposeButton(env, composeIDs, buttonID)
	}
	transitionID := int64FromAny(contextValues["workflow_transition_id"])
	if env == nil || transitionID == 0 || len(composeIDs) == 0 {
		return nil, false, nil
	}
	rows, err := env.Model("mail.compose.message").Browse(composeIDs...).Read("model", "res_id", "res_ids")
	if err != nil {
		if strings.Contains(err.Error(), "unknown model mail.compose.message") {
			return nil, false, nil
		}
		return nil, false, err
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return nil, false, err
	}
	store := NewProcessStore(env)
	userCtx := evalContextBase(env, "")
	var last any = softReloadAction()
	for _, row := range rows {
		modelName := stringFromAny(row["model"])
		recordIDs := idsFromAny(row["res_ids"])
		if len(recordIDs) == 0 {
			if id := int64FromAny(row["res_id"]); id != 0 {
				recordIDs = []int64{id}
			}
		}
		if modelName == "" || len(recordIDs) == 0 {
			continue
		}
		for _, recordID := range recordIDs {
			process, ok, err := store.Find(modelName, recordID)
			if err != nil {
				return last, false, err
			}
			if !ok {
				return last, false, fmt.Errorf("workflow process not found for %s:%d", modelName, recordID)
			}
			workflow, ok := workflowByID(workflows, process.WorkflowID)
			if !ok {
				return last, false, fmt.Errorf("workflow %d not found", process.WorkflowID)
			}
			transition, ok := workflowTransitionByID(workflow, transitionID)
			if !ok {
				return last, false, fmt.Errorf("workflow transition %d not found", transitionID)
			}
			base := userCtx
			base.Model = modelName
			ctx, err := evalContextForRecord(env, modelName, recordID, base)
			if err != nil {
				return last, false, err
			}
			ctx.MailComposed = true
			ctx.DelegationID = advancedApprovalDelegationID(env, workflow, process, d.Delegations, d.now())
			runAsSuperuser := transitionWillRunAsSuperuser(process, transition, ctx)
			writeEnv := workflowRunAsSuperuserEnv(env, runAsSuperuser)
			applyStore := workflowRunAsSuperuserStore(store, writeEnv, runAsSuperuser)
			updated, results, _, err := applyStore.ApplyTransitionForRecord(workflow, process, transitionID, ctx, d.advancedHooks(store, env))
			if err != nil {
				return last, false, err
			}
			if err := d.persistAdvancedRecordState(writeEnv, workflow, updated); err != nil {
				return last, false, err
			}
			if updated.NodeID != process.NodeID || !updated.Active {
				if err := deactivateActiveForwards(writeEnv, modelName, recordID); err != nil {
					return last, false, err
				}
			}
			if action, ok := firstActionResult(results, "mail.compose"); ok {
				last = action.Payload
			} else {
				last = softReloadAction()
			}
		}
	}
	return last, true, nil
}

func (d Dispatcher) completeClassicMailComposeButton(env *record.Env, composeIDs []int64, buttonID int64) (any, bool, error) {
	if env == nil || buttonID == 0 || len(composeIDs) == 0 {
		return nil, false, nil
	}
	buttonRows, err := env.Model(ModelButton).Browse(buttonID).Read("email_next_action")
	if err != nil {
		return nil, false, err
	}
	if len(buttonRows) == 0 || strings.TrimSpace(stringFromAny(buttonRows[0]["email_next_action"])) == "" {
		return nil, true, nil
	}
	rows, err := env.Model("mail.compose.message").Browse(composeIDs...).Read("model", "res_id", "res_ids")
	if err != nil {
		if strings.Contains(err.Error(), "unknown model mail.compose.message") {
			return nil, false, nil
		}
		return nil, false, err
	}
	var last any = closeWindowAction()
	for _, row := range rows {
		modelName := stringFromAny(row["model"])
		recordIDs := idsFromAny(row["res_ids"])
		if len(recordIDs) == 0 {
			if id := int64FromAny(row["res_id"]); id != 0 {
				recordIDs = []int64{id}
			}
		}
		if modelName == "" || len(recordIDs) == 0 {
			continue
		}
		result, err := d.runButtons(context.Background(), env, modelName, recordIDs, buttonID, Input{MailComposed: true}, false)
		if err != nil {
			return last, false, err
		}
		last = result
	}
	return last, true, nil
}

func (d Dispatcher) AutoStartCreateHook() record.AfterCreateHook {
	return func(env *record.Env, modelName string, id int64, _ map[string]any) error {
		if _, err := d.RunCreateAutomationsForRecord(env, modelName, id); err != nil {
			return err
		}
		if boolFromAny(env.Context().Values["approval_auto_submit"]) {
			if _, err := d.AutoSubmitWorkflowForRecord(context.Background(), env, modelName, id); err != nil {
				return err
			}
		}
		_, err := d.AutoStartWorkflowForRecord(env, modelName, id)
		return err
	}
}

func (d Dispatcher) StateUpdatedWriteHook() record.AfterWriteHook {
	return func(env *record.Env, modelName string, id int64, oldRow map[string]any, newRow map[string]any, _ map[string]any) error {
		if env == nil || boolFromAny(env.Context().Values[stateWriteHookSkipContextKey]) {
			return nil
		}
		engine, err := d.engineFromEnv(env)
		if err != nil {
			if strings.Contains(err.Error(), "unknown model ") {
				return nil
			}
			return err
		}
		settings, ok := engine.SettingsForModel(modelName)
		if !ok {
			return nil
		}
		oldState := stateFromRow(settings, oldRow)
		newState := stateFromRow(settings, newRow)
		if oldState == newState {
			return nil
		}
		_, err = d.runStateUpdatedWrite(env, engine, settings, modelName, id, oldState, Input{Values: cloneMap(newRow)})
		return err
	}
}

func (d Dispatcher) UnlinkHook() record.BeforeUnlinkHook {
	return func(env *record.Env, modelName string, id int64, row map[string]any) error {
		if env == nil {
			return nil
		}
		engine, err := d.engineFromEnv(env)
		if err != nil {
			if strings.Contains(err.Error(), "unknown model ") {
				return nil
			}
			return err
		}
		settings, ok := engine.SettingsForModel(modelName)
		if !ok {
			return nil
		}
		state := stateFromRow(settings, row)
		if !workflowUnlinkStateAllowed(settings, state) {
			return fmt.Errorf("You can delete in %s status only", settings.DraftState)
		}
		return deleteApprovalLogsForRecord(env, modelName, id)
	}
}

func workflowUnlinkStateAllowed(settings Settings, state string) bool {
	allowed := map[string]bool{
		settings.DraftState:     true,
		settings.CancelledState: true,
		"cancel":                true,
		"cancelled":             true,
		"canceled":              true,
	}
	return allowed[state]
}

func deleteApprovalLogsForRecord(env *record.Env, modelName string, id int64) error {
	logs, err := env.Model(ModelLog).Search(domain.And(
		domain.Cond("model", "=", modelName),
		domain.Cond("record_id", "=", id),
	))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model "+ModelLog) {
			return nil
		}
		return err
	}
	if logs.Len() == 0 {
		return nil
	}
	return logs.Unlink()
}

func (d Dispatcher) runStateUpdatedWrite(env *record.Env, engine *Engine, settings Settings, modelName string, id int64, oldState string, input Input) (bool, error) {
	if env == nil || modelName == "" || id == 0 {
		return false, nil
	}
	hookEnv := stateWriteHookQuietEnv(env)
	if engine == nil {
		var err error
		engine, err = d.engineFromEnv(hookEnv)
		if err != nil {
			if strings.Contains(err.Error(), "unknown model ") {
				return false, nil
			}
			return false, err
		}
		var ok bool
		settings, ok = engine.SettingsForModel(modelName)
		if !ok {
			return false, nil
		}
	}
	wrec, err := workflowRecordFromEnv(hookEnv, modelName, id, engine, d.Delegations, d.now())
	if err != nil {
		return false, err
	}
	newState := wrec.State
	if oldState == newState {
		return false, nil
	}
	before := effectCounts(engine)
	user := userFromEnv(hookEnv)
	log := engine.appendLog(user.ID, wrec, Button{}, oldState, newState, input)
	wrec.LastStateUpdate = log.At
	wrec.Values = setValue(wrec.Values, settings.StateField, newState)
	if input.Values == nil {
		input.Values = cloneMap(wrec.Values)
	}
	if _, err := engine.RunStateUpdatedAutomations(context.Background(), user, wrec, oldState, input); err != nil {
		return false, err
	}
	if err := persistWorkflowResult(hookEnv, engine, before, wrec, d.Delegations, d.now()); err != nil {
		return false, err
	}
	if err := deactivateActiveForwards(hookEnv, modelName, id); err != nil {
		return false, err
	}
	return true, nil
}

func (d Dispatcher) RunCreateAutomationsForRecord(env *record.Env, modelName string, id int64) (bool, error) {
	if env == nil || modelName == "" || id == 0 {
		return false, nil
	}
	engine, err := d.engineFromEnv(env)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return false, nil
		}
		return false, err
	}
	settings, ok := engine.SettingsForModel(modelName)
	if !ok {
		return false, nil
	}
	wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
	if err != nil {
		return false, err
	}
	if len(engine.matchingAutomations(TriggerOnCreate, settings.ID, "", wrec.State)) == 0 {
		return false, nil
	}
	if _, err := engine.RunCreateAutomations(context.Background(), userFromEnv(env), wrec, Input{Values: cloneMap(wrec.Values)}); err != nil {
		return false, err
	}
	return true, nil
}

func (d Dispatcher) AutoSubmitWorkflowForRecord(ctx context.Context, env *record.Env, modelName string, id int64) (bool, error) {
	if env == nil || modelName == "" || id == 0 {
		return false, nil
	}
	engine, err := d.engineFromEnv(env)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return false, nil
		}
		return false, err
	}
	if _, ok := engine.SettingsForModel(modelName); !ok {
		return false, nil
	}
	wrec, err := workflowRecordFromEnv(env, modelName, id, engine, d.Delegations, d.now())
	if err != nil {
		return false, err
	}
	user := userFromEnv(env)
	before := effectCounts(engine)
	if _, err := engine.ApproveRecord(ctx, user, &wrec, Input{Values: cloneMap(wrec.Values)}); err != nil {
		return false, err
	}
	if err := persistWorkflowResult(env, engine, before, wrec, d.Delegations, d.now()); err != nil {
		return false, err
	}
	return true, nil
}

func (d Dispatcher) AutoStartWorkflowForRecord(env *record.Env, modelName string, id int64) (bool, error) {
	if env == nil || modelName == "" || id == 0 {
		return false, nil
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return false, nil
		}
		return false, err
	}
	if len(workflows) == 0 {
		return false, nil
	}
	sort.SliceStable(workflows, func(i, j int) bool {
		if workflows[i].Sequence == workflows[j].Sequence {
			return workflows[i].ID < workflows[j].ID
		}
		return workflows[i].Sequence < workflows[j].Sequence
	})
	base := evalContextBase(env, modelName)
	ctx, err := evalContextForRecord(env, modelName, id, base)
	if err != nil {
		return false, err
	}
	ctx.Now = d.now()
	store := NewProcessStore(env)
	if _, ok, err := store.Find(modelName, id); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}
	for _, workflow := range workflows {
		if _, ok := workflow.startNode(); !ok {
			continue
		}
		ok, err := workflow.Matches(ctx, true)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		process, _, _, err := store.StartForRecord(workflow, ctx, d.advancedHooks(store, env))
		if err != nil {
			return false, err
		}
		if err := d.persistAdvancedRecordState(env, workflow, process); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func evalContextBase(env *record.Env, modelName string) EvaluationContext {
	ctx := env.Context()
	return EvaluationContext{
		UserID:       ctx.UserID,
		UserGroupIDs: idsFromAny(ctx.Values["group_ids"]),
		CompanyID:    ctx.CompanyID,
		CompanyIDs:   append([]int64(nil), ctx.CompanyIDs...),
		Model:        modelName,
	}
}

func evalContextForRecord(env *record.Env, modelName string, id int64, base EvaluationContext) (EvaluationContext, error) {
	fields, err := evaluationFieldNames(env, modelName)
	if err != nil {
		return EvaluationContext{}, err
	}
	rows, err := env.Model(modelName).Browse(id).Read(fields...)
	if err != nil {
		return EvaluationContext{}, err
	}
	if len(rows) == 0 {
		return EvaluationContext{}, fmt.Errorf("%s:%d not found", modelName, id)
	}
	ctx := base
	ctx.Model = modelName
	ctx.RecordID = id
	ctx.Values = rows[0]
	if companyID := int64FromAny(rows[0]["company_id"]); companyID != 0 {
		ctx.CompanyID = companyID
	}
	return ctx, nil
}

func evaluationFieldNames(env *record.Env, modelName string) ([]string, error) {
	fields, err := env.Model(modelName).FieldsGet(nil, nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		if name == "id" || name == "display_name" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (d Dispatcher) persistAdvancedRecordState(env *record.Env, workflow Workflow, process Process) error {
	return persistAdvancedRecordState(env, workflow, process, d.Delegations, d.now())
}

func (d Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func persistAdvancedRecordState(env *record.Env, workflow Workflow, process Process, delegations ApprovalDelegationProvider, at time.Time) error {
	var err error
	process, err = enrichAdvancedApprovalState(env, workflow, process, delegations, at)
	if err != nil {
		return err
	}
	if _, err := NewProcessStore(env).Save(process); err != nil {
		return err
	}
	activityOpts := advancedApprovalActivityOptions(workflow, process, at)
	transitionIDs, err := advancedAvailableTransitionIDsForRecord(env, workflow, process)
	if err != nil {
		return err
	}
	values := map[string]any{
		"state":                           process.State,
		"workflow_id":                     process.WorkflowID,
		"workflow_node_id":                process.NodeID,
		"_workflow_transition_id":         process.LastTransitionID,
		"workflow_transition_ids":         transitionIDs,
		"approval_user_ids":               process.ApprovalUserIDs,
		"approval_done_user_ids":          process.ApprovalDoneUserIDs,
		"approval_partner_ids":            process.ApprovalPartnerIDs,
		"user_can_approve":                process.UserCanApprove,
		"workflow_view_id":                workflow.ViewID,
		"approval_activity_date_deadline": approvalActivityDateValue(env, activityOpts),
	}
	if err := writeKnown(env, process.Model, process.RecordID, values); err != nil {
		return err
	}
	return syncApprovalActivities(env, activityOpts)
}

func advancedAvailableTransitionIDsForRecord(env *record.Env, workflow Workflow, process Process) ([]int64, error) {
	if env == nil || !process.Active || !process.UserCanApprove {
		return nil, nil
	}
	node, ok := workflow.nodeByID(process.NodeID)
	if !ok || node.Type != NodeTypeUser {
		return nil, nil
	}
	ctx, err := evalContextForRecord(env, process.Model, process.RecordID, evalContextBase(env, process.Model))
	if err != nil {
		return nil, err
	}
	transitions := orderedTransitions(node.Transitions)
	ids := make([]int64, 0, len(transitions))
	for _, transition := range transitions {
		ok, err := transition.Available(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ids = append(ids, transition.ID)
	}
	return ids, nil
}

func advancedApprovalActivityOptions(workflow Workflow, process Process, at time.Time) approvalActivityOptions {
	node, ok := workflow.nodeByID(process.NodeID)
	enabled := process.Active && ok && node.ScheduleActivity
	days := 0
	dateField := ""
	if ok {
		days = node.ScheduleActivityDays
		dateField = node.ScheduleActivityField
	}
	return approvalActivityOptions{
		Enabled:   enabled,
		Model:     process.Model,
		RecordID:  process.RecordID,
		UserIDs:   process.ApprovalUserIDs,
		DateField: dateField,
		Days:      days,
		At:        at,
	}
}

func enrichAdvancedApprovalState(env *record.Env, workflow Workflow, process Process, delegations ApprovalDelegationProvider, at time.Time) (Process, error) {
	node, ok := workflow.nodeByID(process.NodeID)
	if !ok {
		return process, nil
	}
	approvalUserIDs := activeForwardUserIDs(env, process.Model, process.RecordID, 0, process.NodeID, process.State)
	hasForward := len(approvalUserIDs) > 0
	if !hasForward {
		responsibleUserIDs, err := nodeResponsibleUserIDsForRecord(env, process.Model, process.RecordID, node)
		if err != nil {
			return process, err
		}
		approvalUserIDs = uniqueSortedIDs(append(append([]int64{}, process.ApprovalUserIDs...), responsibleUserIDs...))
	}
	if !hasForward && delegations != nil {
		departmentIDs := approvalRecordDepartmentIDs(env, process.Model, process.RecordID)
		approvalUserIDs = uniqueSortedIDs(append(approvalUserIDs, delegations.DelegatedApprovalUserIDs(approvalUserIDs, node.ResponsibleGroupIDs, departmentIDs, at)...))
	}
	approvalUserIDs = approvalUsersWithReadAccess(env, process.Model, process.RecordID, approvalUserIDs)
	if node.ResponsibleCommittee {
		approvalUserIDs = withoutIDs(approvalUserIDs, process.ApprovalDoneUserIDs)
	}
	process.ApprovalUserIDs = approvalUserIDs
	process.ApprovalPartnerIDs = uniqueSortedIDs(partnerIDsForUsers(env, process.ApprovalUserIDs))
	if hasForward {
		process.UserCanApprove = env.Context().UserID != 0 && containsInt64(process.ApprovalUserIDs, env.Context().UserID)
	} else {
		process.UserCanApprove = approvalAllowedForContext(env.Context(), node, process.ApprovalUserIDs)
	}
	return process, nil
}

func advancedApprovalDelegationID(env *record.Env, workflow Workflow, process Process, delegations ApprovalDelegationProvider, at time.Time) int64 {
	if env == nil || delegations == nil || env.Context().UserID == 0 {
		return 0
	}
	node, ok := workflow.nodeByID(process.NodeID)
	if !ok {
		return 0
	}
	approvalUserIDs, err := nodeResponsibleUserIDsForRecord(env, process.Model, process.RecordID, node)
	if err != nil {
		return 0
	}
	if len(approvalUserIDs) == 0 {
		return 0
	}
	departmentIDs := approvalRecordDepartmentIDs(env, process.Model, process.RecordID)
	return delegations.ActiveApprovalDelegationID(env.Context().UserID, approvalUserIDs, node.ResponsibleGroupIDs, departmentIDs, at)
}

func userCanReadRecord(env *record.Env, modelName string, recordID int64, userID int64) bool {
	if userID == 0 {
		return false
	}
	rows, err := env.WithContext(approvalCandidateContext(env, userID)).Model(modelName).Browse(recordID).Read("id")
	return err == nil && len(rows) > 0
}

func approvalUsersWithReadAccess(env *record.Env, modelName string, recordID int64, userIDs []int64) []int64 {
	userIDs = uniqueSortedIDs(append([]int64{}, userIDs...))
	if env == nil || env.Policy() == nil || modelName == "" || recordID == 0 || len(userIDs) == 0 {
		return userIDs
	}
	out := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		candidateEnv := env.WithContext(approvalCandidateContext(env, userID))
		rows, err := candidateEnv.Model(modelName).Browse(recordID).Read("id")
		if err == nil && len(rows) > 0 {
			out = append(out, userID)
		}
	}
	return uniqueSortedIDs(out)
}

func approvalCandidateContext(env *record.Env, userID int64) record.Context {
	base := env.Context()
	ctx := record.Context{
		UserID:     userID,
		CompanyID:  base.CompanyID,
		CompanyIDs: append([]int64(nil), base.CompanyIDs...),
		Values:     cloneMap(base.Values),
	}
	if userID == 0 {
		return ctx
	}
	users, ok := env.ModelMetadata("res.users")
	if !ok {
		return ctx
	}
	fields := []string{}
	for _, fieldName := range []string{"company_id", "company_ids", userGroupsField(env)} {
		if fieldName == "" {
			continue
		}
		if _, ok := users.Fields[fieldName]; ok {
			fields = append(fields, fieldName)
		}
	}
	if len(fields) == 0 {
		return ctx
	}
	systemCtx := record.Context{
		UserID:     1,
		CompanyID:  base.CompanyID,
		CompanyIDs: append([]int64(nil), base.CompanyIDs...),
		Values:     cloneMap(base.Values),
	}
	rows, err := env.WithContext(systemCtx).Model("res.users").Browse(userID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return ctx
	}
	if companyID := int64FromAny(rows[0]["company_id"]); companyID != 0 {
		ctx.CompanyID = companyID
	}
	if companyIDs := idsFromAny(rows[0]["company_ids"]); len(companyIDs) > 0 {
		ctx.CompanyIDs = companyIDs
	}
	if groupField := userGroupsField(env); groupField != "" {
		if groupIDs := idsFromAny(rows[0][groupField]); len(groupIDs) > 0 {
			ctx.Values["group_ids"] = groupIDs
		}
	}
	return ctx
}

func approvalRecordDepartmentIDs(env *record.Env, modelName string, recordID int64) []int64 {
	if env == nil || modelName == "" || recordID == 0 {
		return nil
	}
	env = workflowSystemEnv(env)
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil
	}
	for _, fieldName := range []string{"employee_id", "x_employee_id", "department_id", "x_department_id"} {
		fieldMeta, ok := meta.Fields[fieldName]
		if !ok || !fieldMeta.Store || fieldMeta.Kind != field.Many2One {
			continue
		}
		switch fieldMeta.Relation {
		case "hr.employee":
			value, _ := resolveRecordPath(env, modelName, []int64{recordID}, []string{fieldName, "department_id"})
			return departmentIDsWithDescendants(env, idsFromAny(value))
		case "hr.department":
			value, _ := resolveRecordPath(env, modelName, []int64{recordID}, []string{fieldName})
			return departmentIDsWithDescendants(env, idsFromAny(value))
		}
	}
	return nil
}

func departmentIDsWithDescendants(env *record.Env, departmentIDs []int64) []int64 {
	env = workflowSystemEnv(env)
	departmentIDs = uniqueSortedIDs(departmentIDs)
	if len(departmentIDs) == 0 {
		return nil
	}
	meta, ok := env.ModelMetadata("hr.department")
	if !ok {
		return departmentIDs
	}
	parentField, ok := meta.Fields["parent_id"]
	if !ok || parentField.Relation != "hr.department" {
		return departmentIDs
	}
	rows, err := allRows(env, "hr.department", "parent_id")
	if err != nil {
		return departmentIDs
	}
	allowed := map[int64]bool{}
	for _, id := range departmentIDs {
		allowed[id] = true
	}
	changed := true
	for changed {
		changed = false
		for _, row := range rows {
			id := int64FromAny(row["id"])
			parentID := int64FromAny(row["parent_id"])
			if id == 0 || parentID == 0 || !allowed[parentID] || allowed[id] {
				continue
			}
			allowed[id] = true
			changed = true
		}
	}
	out := make([]int64, 0, len(allowed))
	for id := range allowed {
		out = append(out, id)
	}
	return uniqueSortedIDs(out)
}

func workflowSystemEnv(env *record.Env) *record.Env {
	if env == nil || env.Context().UserID == 1 {
		return env
	}
	base := env.Context()
	return env.WithContext(record.Context{
		UserID:     1,
		CompanyID:  base.CompanyID,
		CompanyIDs: append([]int64(nil), base.CompanyIDs...),
		Values:     cloneMap(base.Values),
	})
}

func workflowSudoEnv(env *record.Env) *record.Env {
	if env == nil || env.Context().Sudo {
		return env
	}
	ctx := env.Context()
	ctx.Sudo = true
	ctx.CompanyIDs = append([]int64(nil), ctx.CompanyIDs...)
	ctx.Values = cloneMap(ctx.Values)
	return env.WithContext(ctx)
}

func nodeResponsibleUserIDs(env *record.Env, node Node) []int64 {
	userIDs, _ := nodeResponsibleUserIDsForRecord(env, "", 0, node)
	return userIDs
}

func nodeResponsibleUserIDsForRecord(env *record.Env, modelName string, recordID int64, node Node) ([]int64, error) {
	userIDs := append([]int64{}, node.ResponsibleUserIDs...)
	if strings.TrimSpace(node.ResponsiblePythonCode) != "" {
		dynamicUserIDs, err := evalApprovalUserPythonCode(env, modelName, recordID, node.ResponsiblePythonCode)
		if err != nil {
			return nil, fmt.Errorf("workflow.node %d responsible_python_code: %w", node.ID, err)
		}
		userIDs = append(userIDs, dynamicUserIDs...)
		return uniqueSortedIDs(userIDs), nil
	}
	groupUserIDs := userIDsForGroups(env, node.ResponsibleGroupIDs)
	if strings.TrimSpace(node.ResponsibleFilter) != "" {
		filtered, err := filterResponsibleGroupUsers(env, modelName, recordID, groupUserIDs, node.ResponsibleFilter)
		if err != nil {
			return nil, fmt.Errorf("workflow.node %d responsible_filter: %w", node.ID, err)
		}
		groupUserIDs = filtered
	}
	userIDs = append(userIDs, groupUserIDs...)
	if strings.TrimSpace(node.ResponsibleValue) != "" {
		dynamicUserIDs, err := evalApprovalUserExpression(env, modelName, recordID, node.ResponsibleValue)
		if err != nil {
			return nil, fmt.Errorf("workflow.node %d responsible_value: %w", node.ID, err)
		}
		userIDs = append(userIDs, dynamicUserIDs...)
	}
	return uniqueSortedIDs(userIDs), nil
}

func filterResponsibleGroupUsers(env *record.Env, modelName string, recordID int64, userIDs []int64, expression string) ([]int64, error) {
	userIDs = uniqueSortedIDs(userIDs)
	if len(userIDs) == 0 || strings.TrimSpace(expression) == "" {
		return userIDs, nil
	}
	condition := Expr(expression)
	out := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 1 {
			out = append(out, userID)
			continue
		}
		candidateEnv := env.WithContext(approvalCandidateContext(env, userID))
		ctx, err := evalContextForRecord(candidateEnv, modelName, recordID, evalContextBase(candidateEnv, modelName))
		if err != nil {
			return nil, err
		}
		ok, err := condition.Evaluate(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, userID)
		}
	}
	return uniqueSortedIDs(out), nil
}

func userIDsForGroups(env *record.Env, groupIDs []int64) []int64 {
	groupIDs = uniqueSortedIDs(append([]int64{}, groupIDs...))
	if len(groupIDs) == 0 {
		return nil
	}
	groupField := userGroupsField(env)
	if groupField == "" {
		return nil
	}
	rows, err := allRows(env, "res.users", groupField)
	if err != nil {
		return nil
	}
	var userIDs []int64
	for _, row := range rows {
		if intersectsInt64(idsFromAny(row[groupField]), groupIDs) {
			userIDs = append(userIDs, int64FromAny(row["id"]))
		}
	}
	return uniqueSortedIDs(userIDs)
}

func partnerIDsForUsers(env *record.Env, userIDs []int64) []int64 {
	userIDs = uniqueSortedIDs(append([]int64{}, userIDs...))
	if len(userIDs) == 0 || userPartnerField(env) == "" {
		return nil
	}
	rows, err := env.Model("res.users").Browse(userIDs...).Read("partner_id")
	if err != nil {
		return nil
	}
	var partnerIDs []int64
	for _, row := range rows {
		if partnerID := int64FromAny(row["partner_id"]); partnerID != 0 {
			partnerIDs = append(partnerIDs, partnerID)
		}
	}
	return uniqueSortedIDs(partnerIDs)
}

func approvalAllowedForContext(ctx record.Context, node Node, approvalUserIDs []int64) bool {
	if ctx.UserID != 0 && containsInt64(approvalUserIDs, ctx.UserID) {
		return true
	}
	groupIDs := idsFromAny(ctx.Values["group_ids"])
	return len(node.ResponsibleGroupIDs) > 0 && intersectsInt64(node.ResponsibleGroupIDs, groupIDs)
}

func userGroupsField(env *record.Env) string {
	users, ok := env.ModelMetadata("res.users")
	if !ok {
		return ""
	}
	if _, ok := users.Fields["groups_id"]; ok {
		return "groups_id"
	}
	if _, ok := users.Fields["group_ids"]; ok {
		return "group_ids"
	}
	return ""
}

func userPartnerField(env *record.Env) string {
	users, ok := env.ModelMetadata("res.users")
	if !ok {
		return ""
	}
	if _, ok := users.Fields["partner_id"]; ok {
		return "partner_id"
	}
	return ""
}

func workflowRecordFromEnv(env *record.Env, modelName string, id int64, engine *Engine, delegations ApprovalDelegationProvider, at time.Time) (Record, error) {
	settings, ok := engine.SettingsForModel(modelName)
	if !ok {
		return Record{}, ErrSettingsNotFound
	}
	fields := []string{settings.StateField, "state", "user_id", "create_uid", "owner_user_id", "company_id", "approval_state_id", "workflow_node_id", "last_state_update", "approval_user_ids", "approval_done_user_ids", "approval_forward_user_ids", "amount_total", "amount", "name"}
	rows, err := env.Model(modelName).Browse(id).Read(fields...)
	if err != nil {
		return Record{}, err
	}
	if len(rows) == 0 {
		return Record{}, fmt.Errorf("%s:%d not found", modelName, id)
	}
	row := rows[0]
	state := stringFromAny(firstNonNil(row[settings.StateField], row["state"]))
	approvalStateID := int64FromAny(row["approval_state_id"])
	if approvalStateID == 0 {
		if config, ok := engine.configForState(settings.ID, state); ok {
			approvalStateID = config.ID
		}
	}
	workflowNodeID := int64FromAny(row["workflow_node_id"])
	approvalUserIDs := idsFromAny(row["approval_user_ids"])
	forwardedUserIDs := idsFromAny(row["approval_forward_user_ids"])
	doneUserIDs := idsFromAny(row["approval_done_user_ids"])
	var delegationID int64
	if forwardUserIDs := activeForwardUserIDs(env, modelName, id, approvalStateID, workflowNodeID, state); len(forwardUserIDs) > 0 {
		approvalUserIDs = forwardUserIDs
		forwardedUserIDs = forwardUserIDs
	} else if config, ok := engine.configForState(settings.ID, state); ok {
		baseApprovalUserIDs, err := classicApprovalConfigUserIDs(env, modelName, id, approvalUserIDs, config)
		if err != nil {
			return Record{}, err
		}
		if delegations != nil {
			departmentIDs := approvalRecordDepartmentIDs(env, modelName, id)
			delegatedUserIDs := delegations.DelegatedApprovalUserIDs(baseApprovalUserIDs, config.GroupIDs, departmentIDs, at)
			approvalUserIDs = uniqueSortedIDs(append(baseApprovalUserIDs, delegatedUserIDs...))
			if env.Context().UserID != 0 {
				delegationID = delegations.ActiveApprovalDelegationID(env.Context().UserID, baseApprovalUserIDs, config.GroupIDs, departmentIDs, at)
			}
		} else {
			approvalUserIDs = baseApprovalUserIDs
		}
		approvalUserIDs = approvalUsersWithReadAccess(env, modelName, id, approvalUserIDs)
		if config.Committee {
			approvalUserIDs = withoutIDs(approvalUserIDs, doneUserIDs)
		}
	}
	return Record{
		Model:              modelName,
		ID:                 id,
		State:              state,
		OwnerUserID:        firstInt64(row["owner_user_id"], row["user_id"], row["create_uid"]),
		CompanyID:          int64FromAny(row["company_id"]),
		ApprovalStateID:    approvalStateID,
		WorkflowNodeID:     workflowNodeID,
		LastStateUpdate:    timeFromAny(row["last_state_update"]),
		ApprovalUserIDs:    approvalUserIDs,
		DoneUserIDs:        doneUserIDs,
		ForwardedToUserIDs: forwardedUserIDs,
		DelegationID:       delegationID,
		Values:             row,
	}, nil
}

func classicApprovalConfigUserIDs(env *record.Env, modelName string, recordID int64, existing []int64, config ApprovalConfig) ([]int64, error) {
	userIDs := append([]int64{}, existing...)
	userIDs = append(userIDs, config.UserIDs...)
	if strings.TrimSpace(config.UserPythonCode) != "" {
		dynamicUserIDs, err := evalApprovalUserPythonCode(env, modelName, recordID, config.UserPythonCode)
		if err != nil {
			return nil, fmt.Errorf("approval.config %d user_python_code: %w", config.ID, err)
		}
		userIDs = append(userIDs, dynamicUserIDs...)
		return uniqueSortedIDs(userIDs), nil
	}
	userIDs = append(userIDs, userIDsForGroups(env, config.GroupIDs)...)
	return uniqueSortedIDs(userIDs), nil
}

func evalApprovalUserPythonCode(env *record.Env, modelName string, recordID int64, code string) ([]int64, error) {
	expr, ok := approvalUserResultExpression(code)
	if !ok {
		return nil, fmt.Errorf("missing result assignment")
	}
	return evalApprovalUserExpression(env, modelName, recordID, expr)
}

func evalApprovalUserExpression(env *record.Env, modelName string, recordID int64, expr string) ([]int64, error) {
	expr = normalizeApprovalUserExpression(expr)
	ids, err := data.SafeEvalIDs(expr, data.SafeEvalOptions{
		Env:      env,
		Model:    modelName,
		RecordID: recordID,
	})
	if err != nil {
		return nil, err
	}
	return uniqueSortedIDs(ids), nil
}

func approvalUserResultExpression(code string) (string, bool) {
	var statements []string
	for _, line := range strings.Split(strings.ReplaceAll(code, "\r\n", "\n"), "\n") {
		for _, statement := range splitTopLevel(line, ';') {
			statement = strings.TrimSpace(stripPythonLineComment(statement))
			if statement != "" {
				statements = append(statements, statement)
			}
		}
	}
	for i := len(statements) - 1; i >= 0; i-- {
		statement := statements[i]
		if !strings.HasPrefix(statement, "result") {
			continue
		}
		parts := strings.SplitN(statement, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != "result" {
			continue
		}
		expr := strings.TrimSpace(parts[1])
		if expr != "" {
			return expr, true
		}
	}
	return "", false
}

func normalizeApprovalUserExpression(expr string) string {
	replacer := strings.NewReplacer(
		"self.", "record.",
		"object.", "record.",
		"env[", "obj().env[",
		"env.ref(", "obj().env.ref(",
	)
	return replacer.Replace(strings.TrimSpace(expr))
}

func stripPythonLineComment(text string) string {
	var quote rune
	escaped := false
	for idx, ch := range text {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '#' {
			return text[:idx]
		}
	}
	return text
}

func activeForwardUserIDs(env *record.Env, modelName string, recordID int64, approvalStateID int64, workflowNodeID int64, state string) []int64 {
	if env == nil || modelName == "" || recordID == 0 {
		return nil
	}
	rows, err := allRows(env, ModelForward, "model", "record_id", "approval_state_id", "state_value", "workflow_node_id", "active", "user_id")
	if err != nil {
		return nil
	}
	var userIDs []int64
	for _, row := range rows {
		if row["active"] == false || int64FromAny(row["record_id"]) != recordID {
			continue
		}
		if rowModel := stringFromAny(row["model"]); rowModel != "" && rowModel != modelName {
			continue
		}
		rowWorkflowNodeID := int64FromAny(row["workflow_node_id"])
		rowApprovalStateID := int64FromAny(row["approval_state_id"])
		rowState := stringFromAny(row["state_value"])
		if workflowNodeID != 0 {
			if rowWorkflowNodeID != workflowNodeID {
				continue
			}
		} else if approvalStateID != 0 {
			if rowApprovalStateID != approvalStateID {
				continue
			}
		} else if state != "" && rowState != "" && rowState != state {
			continue
		}
		if userID := int64FromAny(row["user_id"]); userID != 0 {
			userIDs = append(userIDs, userID)
		}
	}
	return uniqueSortedIDs(userIDs)
}

func deactivateActiveForwards(env *record.Env, modelName string, recordID int64) error {
	rows, err := allRows(env, ModelForward, "model", "record_id", "active")
	if err != nil {
		return nil
	}
	var ids []int64
	for _, row := range rows {
		if row["active"] == false || int64FromAny(row["record_id"]) != recordID {
			continue
		}
		if rowModel := stringFromAny(row["model"]); rowModel != "" && rowModel != modelName {
			continue
		}
		ids = append(ids, int64FromAny(row["id"]))
	}
	if len(ids) == 0 {
		return nil
	}
	return env.Model(ModelForward).Browse(ids...).Write(map[string]any{"active": false})
}

type effectSnapshot struct {
	logs          int
	forwards      int
	votes         int
	cancellations int
}

func effectCounts(engine *Engine) effectSnapshot {
	return effectSnapshot{
		logs:          len(engine.Logs),
		forwards:      len(engine.Forwards),
		votes:         len(engine.Votes),
		cancellations: len(engine.Cancellations),
	}
}

func persistWorkflowResult(env *record.Env, engine *Engine, before effectSnapshot, wrec Record, delegations ApprovalDelegationProvider, at time.Time) error {
	settings, ok := engine.SettingsForModel(wrec.Model)
	if !ok {
		return ErrSettingsNotFound
	}
	var err error
	wrec, err = prepareClassicApprovalState(env, engine, settings, wrec, delegations, at)
	if err != nil {
		return err
	}
	activityOpts := classicApprovalActivityOptions(engine, settings, wrec, at)
	values := map[string]any{
		settings.StateField:               wrec.State,
		"state":                           wrec.State,
		"last_state_update":               wrec.LastStateUpdate,
		"approval_user_ids":               wrec.ApprovalUserIDs,
		"approval_done_user_ids":          wrec.DoneUserIDs,
		"approval_partner_ids":            partnerIDsForUsers(env, wrec.ApprovalUserIDs),
		"user_can_approve":                env.Context().UserID != 0 && containsInt64(wrec.ApprovalUserIDs, env.Context().UserID),
		"approval_forward_user_ids":       wrec.ForwardedToUserIDs,
		"approval_activity_date_deadline": approvalActivityDateValue(env, activityOpts),
	}
	if err := writeKnown(env, wrec.Model, wrec.ID, values); err != nil {
		return err
	}
	if err := syncApprovalActivities(env, activityOpts); err != nil {
		return err
	}
	for _, log := range engine.Logs[before.logs:] {
		if err := createKnown(env, ModelLog, map[string]any{
			"model":              log.Model,
			"record_id":          log.RecordID,
			"user_id":            log.UserID,
			"date":               log.At,
			"description":        log.Comment,
			"old_state":          log.OldState,
			"new_state":          log.NewState,
			"duration_seconds":   log.Duration.Seconds(),
			"duration_hours":     log.Duration.Hours(),
			"approval_button_id": log.ButtonID,
			"bulk_approval":      log.BulkApproval,
			"delegation_id":      log.DelegationID,
		}); err != nil {
			return err
		}
	}
	for _, forward := range engine.Forwards[before.forwards:] {
		if err := createKnown(env, ModelForward, map[string]any{
			"model":             forward.Model,
			"record_id":         forward.RecordID,
			"state_value":       forward.StateValue,
			"approval_state_id": forward.ApprovalStateID,
			"workflow_node_id":  forward.WorkflowNodeID,
			"active":            forward.Active,
			"user_id":           forward.UserID,
			"forwarder_user_id": forward.ForwarderUserID,
		}); err != nil {
			return err
		}
	}
	for _, vote := range engine.Votes[before.votes:] {
		voteValue := normalizedVoteType(vote.VoteType)
		if vote.VoteType == "" && !vote.Approved {
			voteValue = "reject"
		}
		if err := createKnown(env, ModelLogVoting, map[string]any{
			"model":        vote.Model,
			"record_id":    vote.RecordID,
			"state":        vote.StateValue,
			"user_id":      vote.UserID,
			"button_id":    vote.ButtonID,
			"vote":         voteValue,
			"comment":      vote.Comment,
			"button_class": vote.ButtonClass,
		}); err != nil {
			return err
		}
	}
	for _, cancellation := range engine.Cancellations[before.cancellations:] {
		if err := createKnown(env, ModelCancellation, map[string]any{
			"model":        cancellation.Model,
			"record_id":    cancellation.RecordID,
			"requester_id": cancellation.RequesterID,
			"reason":       cancellation.Reason,
			"state":        "draft",
		}); err != nil {
			return err
		}
	}
	return nil
}

func prepareClassicApprovalState(env *record.Env, engine *Engine, settings Settings, wrec Record, delegations ApprovalDelegationProvider, at time.Time) (Record, error) {
	config, ok := engine.configForState(settings.ID, wrec.State)
	if !ok || len(wrec.ApprovalUserIDs) > 0 {
		return wrec, nil
	}
	baseApprovalUserIDs, err := classicApprovalConfigUserIDs(env, wrec.Model, wrec.ID, nil, config)
	if err != nil {
		return wrec, err
	}
	approvalUserIDs := baseApprovalUserIDs
	if delegations != nil {
		departmentIDs := approvalRecordDepartmentIDs(env, wrec.Model, wrec.ID)
		approvalUserIDs = uniqueSortedIDs(append(approvalUserIDs, delegations.DelegatedApprovalUserIDs(baseApprovalUserIDs, config.GroupIDs, departmentIDs, at)...))
	}
	approvalUserIDs = approvalUsersWithReadAccess(env, wrec.Model, wrec.ID, approvalUserIDs)
	if config.Committee {
		approvalUserIDs = withoutIDs(approvalUserIDs, wrec.DoneUserIDs)
	}
	wrec.ApprovalUserIDs = approvalUserIDs
	return wrec, nil
}

func classicApprovalActivityOptions(engine *Engine, settings Settings, wrec Record, at time.Time) approvalActivityOptions {
	config, ok := engine.configForState(settings.ID, wrec.State)
	activityAt := wrec.LastStateUpdate
	if activityAt.IsZero() {
		activityAt = at
	}
	return approvalActivityOptions{
		Enabled:   ok && config.ScheduleActivity,
		Model:     wrec.Model,
		RecordID:  wrec.ID,
		UserIDs:   wrec.ApprovalUserIDs,
		DateField: config.ScheduleActivityField,
		Days:      config.ScheduleActivityDays,
		At:        activityAt,
	}
}

func allRows(env *record.Env, modelName string, fields ...string) ([]map[string]any, error) {
	found, err := env.Model(modelName).Search(domain.And())
	if err != nil {
		return nil, err
	}
	return found.Read(fields...)
}

func loadResourceCalendarAttendances(env *record.Env) map[int64][]CalendarAttendance {
	if env == nil {
		return nil
	}
	if _, ok := env.ModelMetadata("resource.calendar.attendance"); !ok {
		return nil
	}
	calendarTwoWeeks := map[int64]bool{}
	if _, ok := env.ModelMetadata("resource.calendar"); ok {
		rows, err := allRows(env, "resource.calendar", "two_weeks_calendar")
		if err == nil {
			for _, row := range rows {
				calendarTwoWeeks[int64FromAny(row["id"])] = boolFromAny(row["two_weeks_calendar"])
			}
		}
	}
	rows, err := allRows(env, "resource.calendar.attendance", "calendar_id", "dayofweek", "hour_from", "hour_to", "day_period", "week_type", "display_type", "sequence")
	if err != nil {
		return nil
	}
	out := map[int64][]CalendarAttendance{}
	for _, row := range rows {
		calendarID := int64FromAny(row["calendar_id"])
		if calendarID == 0 {
			continue
		}
		out[calendarID] = append(out[calendarID], CalendarAttendance{
			ID:         int64FromAny(row["id"]),
			CalendarID: calendarID,
			DayOfWeek:  int(int64FromAny(row["dayofweek"])),
			HourFrom:   floatFromAny(row["hour_from"]),
			HourTo:     floatFromAny(row["hour_to"]),
			DayPeriod:  stringFromAny(row["day_period"]),
			WeekType:   stringFromAny(row["week_type"]),
			Display:    stringFromAny(row["display_type"]),
			Sequence:   int(int64FromAny(row["sequence"])),
			TwoWeeks:   calendarTwoWeeks[calendarID],
		})
	}
	return out
}

func modelFieldName(env *record.Env, id int64) string {
	if id == 0 {
		return ""
	}
	if _, ok := env.ModelMetadata("ir.model.fields"); !ok {
		return ""
	}
	rows, err := env.Model("ir.model.fields").Browse(id).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringFromAny(rows[0]["name"])
}

func stateFromRow(settings Settings, row map[string]any) string {
	if row == nil {
		return ""
	}
	stateField := settings.StateField
	if stateField == "" {
		stateField = "state"
	}
	return stringFromAny(firstNonNil(row[stateField], row["state"]))
}

func stateWriteHookQuietEnv(env *record.Env) *record.Env {
	if env == nil {
		return nil
	}
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values[stateWriteHookSkipContextKey] = true
	ctx.Values = values
	return env.WithContext(ctx)
}

func writeKnown(env *record.Env, modelName string, id int64, values map[string]any) error {
	filtered, err := filterKnownFields(env, modelName, values)
	if err != nil {
		return err
	}
	if len(filtered) == 0 {
		return nil
	}
	return stateWriteHookQuietEnv(env).Model(modelName).Browse(id).Write(filtered)
}

func createKnown(env *record.Env, modelName string, values map[string]any) error {
	filtered, err := filterKnownFields(env, modelName, values)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model "+modelName) {
			return nil
		}
		return err
	}
	_, err = env.Model(modelName).Create(filtered)
	return err
}

func filterKnownFields(env *record.Env, modelName string, values map[string]any) (map[string]any, error) {
	fields, err := env.Model(modelName).FieldsGet(nil, []string{"type"})
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for key, value := range values {
		if _, ok := fields[key]; ok && value != nil {
			out[key] = value
		}
	}
	return out, nil
}

func inputFromRequest(req DispatchRequest) Input {
	values := cloneMap(req.Values)
	for key, value := range req.Kwargs {
		if values == nil {
			values = map[string]any{}
		}
		values[key] = value
	}
	return Input{
		Comment:      stringFromAny(firstNonNil(kwarg(req.Kwargs, "comment"), req.Values["comment"])),
		TargetUserID: firstInt64(kwarg(req.Kwargs, "forward_user_id"), kwarg(req.Kwargs, "target_user_id"), kwarg(req.Kwargs, "user_id"), req.Values["forward_user_id"], req.Values["target_user_id"]),
		Values:       values,
	}
}

func dispatchPayload(result TransitionResult) map[string]any {
	return map[string]any{
		"type":      "ir.actions.client",
		"tag":       "soft_reload",
		"old_state": result.OldState,
		"new_state": result.NewState,
		"button_id": result.Button.ID,
	}
}

func softReloadAction() map[string]any {
	return map[string]any{"type": "ir.actions.client", "tag": "soft_reload"}
}

func closeWindowAction() map[string]any {
	return map[string]any{"type": "ir.actions.act_window_close"}
}

func parseDomainText(text string) (domain.Node, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "[]" || text == "{}" || text == "false" || text == "False" {
		return domain.And(), nil
	}
	return security.ParseDomainForce(text)
}

func buttonActionFromString(value string) ButtonAction {
	switch ButtonAction(value) {
	case ActionLegacyServerAction:
		return ActionServerAction
	default:
		return ButtonAction(value)
	}
}

func userFromEnv(env *record.Env) User {
	ctx := env.Context()
	return User{ID: ctx.UserID, GroupIDs: idsFromAny(ctx.Values["group_ids"])}
}

func arg(args []any, index int) any {
	if index < 0 || index >= len(args) {
		return nil
	}
	return args[index]
}

func kwarg(kwargs map[string]any, key string) any {
	if kwargs == nil {
		return nil
	}
	return kwargs[key]
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := stringFromAny(value); text != "" {
			return text
		}
	}
	return ""
}

func firstInt64(values ...any) int64 {
	for _, value := range values {
		if id := int64FromAny(value); id != 0 {
			return id
		}
	}
	return 0
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "True" || typed == "1"
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		id, _ := typed.Int64()
		return id
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return id
	default:
		return 0
	}
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		value, _ := typed.Float64()
		return value
	case string:
		value, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return value
	default:
		return 0
	}
}

func float64FromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	case int:
		return float64(typed)
	case string:
		out, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return out
	default:
		return 0
	}
}

func idsFromAny(value any) []int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int64, int, int32, float64, json.Number:
		id := int64FromAny(typed)
		if id == 0 {
			return nil
		}
		return []int64{id}
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
			if nested, ok := item.([]any); ok && len(nested) > 0 {
				out = append(out, idsFromAny(nested)...)
				continue
			}
			if id := int64FromAny(item); id != 0 {
				out = append(out, id)
			}
		}
		return uniqueSortedIDs(out)
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var values []any
		if strings.HasPrefix(text, "[") && json.Unmarshal([]byte(text), &values) == nil {
			return idsFromAny(values)
		}
		parts := strings.Split(text, ",")
		out := make([]int64, 0, len(parts))
		for _, part := range parts {
			if id := int64FromAny(strings.TrimSpace(part)); id != 0 {
				out = append(out, id)
			}
		}
		return uniqueSortedIDs(out)
	default:
		return nil
	}
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringFromAny(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var values []any
		if strings.HasPrefix(text, "[") && json.Unmarshal([]byte(text), &values) == nil {
			return stringsFromAny(values)
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.Trim(strings.TrimSpace(part), `"'`)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func timeFromAny(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if out, err := time.Parse(layout, typed); err == nil {
				return out
			}
		}
	}
	return time.Time{}
}

func uniqueSortedIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ids[:0]
	var last int64
	for index, id := range ids {
		if id == 0 || (index > 0 && id == last) {
			continue
		}
		out = append(out, id)
		last = id
	}
	return append([]int64(nil), out...)
}

func withoutIDs(ids []int64, remove []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	if len(remove) == 0 {
		return uniqueSortedIDs(append([]int64(nil), ids...))
	}
	blocked := make(map[int64]struct{}, len(remove))
	for _, id := range remove {
		if id != 0 {
			blocked[id] = struct{}{}
		}
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := blocked[id]; !ok {
			out = append(out, id)
		}
	}
	return uniqueSortedIDs(out)
}
