package workflow

import "gorp/internal/record"

func WorkflowWizardDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model(ModelWorkflowWizard).DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	modelName := stringFromAny(firstNonNil(values["model"], workflowWizardContextValue(env, context, "default_model"), workflowWizardContextValue(env, context, "active_model")))
	recordID := int64FromAny(firstNonNil(values["record_id"], workflowWizardContextValue(env, context, "default_record_id"), workflowWizardContextValue(env, context, "active_id")))
	if recordID == 0 {
		activeIDs := idsFromAny(workflowWizardContextValue(env, context, "active_ids"))
		if len(activeIDs) == 1 {
			recordID = activeIDs[0]
		}
	}
	transitionID := int64FromAny(firstNonNil(values["workflow_transition_id"], workflowWizardContextValue(env, context, "default_workflow_transition_id")))
	if workflowWizardWantsField(fields, "model") && modelName != "" {
		values["model"] = modelName
	}
	if workflowWizardWantsField(fields, "record_id") && recordID != 0 {
		values["record_id"] = recordID
	}
	if workflowWizardWantsField(fields, "workflow_transition_id") && transitionID != 0 {
		values["workflow_transition_id"] = transitionID
	}
	if modelName == "" || recordID == 0 {
		return values, nil
	}
	if workflowWizardWantsField(fields, "record_name") {
		name, err := workflowWizardRecordName(env, modelName, recordID)
		if err != nil {
			return nil, err
		}
		values["record_name"] = name
	}

	store := NewProcessStore(env)
	found, err := store.searchProcess(modelName, recordID)
	if err != nil {
		return nil, err
	}
	if found.Len() == 0 {
		return values, nil
	}
	processID := found.IDs()[0]
	process, ok, err := store.Find(modelName, recordID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return values, nil
	}
	if workflowWizardWantsField(fields, "workflow_process_id") {
		values["workflow_process_id"] = processID
	}
	if workflowWizardWantsField(fields, "workflow_node_id") {
		values["workflow_node_id"] = process.NodeID
	}
	if workflowWizardWantsField(fields, "workflow_id") {
		values["workflow_id"] = process.WorkflowID
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return nil, err
	}
	workflow, ok := workflowByID(workflows, process.WorkflowID)
	if !ok {
		return values, nil
	}
	ctx, err := evalContextForRecord(env, modelName, recordID, evalContextBase(env, modelName))
	if err != nil {
		return nil, err
	}
	available, err := workflow.AvailableTransitions(process, ctx)
	if err != nil {
		return nil, err
	}
	availableIDs := make([]int64, 0, len(available))
	for _, transition := range available {
		availableIDs = append(availableIDs, transition.ID)
	}
	if workflowWizardWantsField(fields, "workflow_transition_ids") {
		values["workflow_transition_ids"] = availableIDs
	}
	if transitionID == 0 && len(available) == 1 {
		transitionID = available[0].ID
		if workflowWizardWantsField(fields, "workflow_transition_id") {
			values["workflow_transition_id"] = transitionID
		}
	}
	if workflowWizardWantsField(fields, "comment_required") {
		values["comment_required"] = workflowWizardTransitionCommentRequired(workflow, transitionID)
	}
	return values, nil
}

func workflowWizardContextValue(env *record.Env, context map[string]any, key string) any {
	if context != nil {
		if value, ok := context[key]; ok {
			return value
		}
	}
	if env != nil {
		if value, ok := env.Context().Values[key]; ok {
			return value
		}
	}
	return nil
}

func workflowWizardWantsField(fields []string, name string) bool {
	if len(fields) == 0 {
		return true
	}
	for _, field := range fields {
		if field == name {
			return true
		}
	}
	return false
}

func StateUpdateWizardDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model(ModelStateUpdateWizard).DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	modelName := firstString(values["res_model"], values["model"], workflowWizardContextValue(env, context, "default_res_model"), workflowWizardContextValue(env, context, "active_model"))
	recordIDs := idsFromAny(firstNonNil(values["res_ids"], workflowWizardContextValue(env, context, "default_res_ids"), workflowWizardContextValue(env, context, "active_ids"), workflowWizardContextValue(env, context, "active_id"), values["record_id"]))
	if workflowWizardWantsField(fields, "res_model") && modelName != "" {
		values["res_model"] = modelName
	}
	if workflowWizardWantsField(fields, "res_ids") && len(recordIDs) > 0 {
		values["res_ids"] = recordIDs
	}
	advanced, err := stateUpdateWizardModelIsAdvanced(env, modelName)
	if err != nil {
		return nil, err
	}
	if workflowWizardWantsField(fields, "workflow_model") {
		values["workflow_model"] = advanced
	}
	if !advanced || modelName == "" || len(recordIDs) == 0 {
		if workflowWizardWantsField(fields, "workflow_id") {
			values["workflow_id"] = nil
		}
		if workflowWizardWantsField(fields, "workflow_node_id") {
			values["workflow_node_id"] = nil
		}
		return values, nil
	}
	workflowID, nodeID, err := stateUpdateWizardSharedWorkflow(env, modelName, recordIDs)
	if err != nil {
		return nil, err
	}
	if workflowWizardWantsField(fields, "workflow_id") {
		if workflowID != 0 {
			values["workflow_id"] = workflowID
		} else {
			values["workflow_id"] = nil
		}
	}
	if workflowWizardWantsField(fields, "workflow_node_id") {
		if nodeID != 0 {
			values["workflow_node_id"] = nodeID
		} else {
			values["workflow_node_id"] = nil
		}
	}
	return values, nil
}

func StateUpdateWizardOnchange(env *record.Env, values map[string]any, changed []string) (map[string]any, error) {
	if values == nil {
		values = map[string]any{}
	}
	nodeID := int64FromAny(values["workflow_node_id"])
	if nodeID == 0 {
		return values, nil
	}
	nodeState, _, err := stateUpdateWizardNodeInfo(env, nodeID)
	if err != nil || nodeState == "" {
		return values, err
	}
	if len(changed) == 0 || containsString(changed, "workflow_node_id") {
		values["state"] = nodeState
		return values, nil
	}
	if containsString(changed, "state") {
		state := stringFromAny(values["state"])
		if state != "" && state != nodeState {
			values["workflow_node_id"] = nil
		}
	}
	return values, nil
}

func stateUpdateWizardModelIsAdvanced(env *record.Env, modelName string) (bool, error) {
	if env == nil || modelName == "" {
		return false, nil
	}
	rows, err := allRows(env, ModelSettings, "model", "active", "advance")
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if active, ok := row["active"].(bool); ok && !active {
			continue
		}
		if stringFromAny(row["model"]) == modelName && boolFromAny(row["advance"]) {
			return true, nil
		}
	}
	return false, nil
}

func stateUpdateWizardSharedWorkflow(env *record.Env, modelName string, recordIDs []int64) (int64, int64, error) {
	var sharedWorkflowID int64
	var sharedNodeID int64
	for index, recordID := range recordIDs {
		workflowID, nodeID, err := stateUpdateWizardRecordWorkflow(env, modelName, recordID)
		if err != nil {
			return 0, 0, err
		}
		if index == 0 {
			sharedWorkflowID = workflowID
			sharedNodeID = nodeID
			continue
		}
		if sharedWorkflowID != workflowID {
			sharedWorkflowID = 0
		}
		if sharedNodeID != nodeID {
			sharedNodeID = 0
		}
	}
	return sharedWorkflowID, sharedNodeID, nil
}

func stateUpdateWizardRecordWorkflow(env *record.Env, modelName string, recordID int64) (int64, int64, error) {
	process, ok, err := NewProcessStore(env).Find(modelName, recordID)
	if err != nil {
		return 0, 0, err
	}
	if ok {
		return process.WorkflowID, process.NodeID, nil
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return 0, 0, nil
	}
	fields := []string{}
	if _, ok := meta.Fields["workflow_id"]; ok {
		fields = append(fields, "workflow_id")
	}
	if _, ok := meta.Fields["workflow_node_id"]; ok {
		fields = append(fields, "workflow_node_id")
	}
	if len(fields) == 0 {
		return 0, 0, nil
	}
	rows, err := env.Model(modelName).Browse(recordID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return 0, 0, err
	}
	workflowID := int64FromAny(rows[0]["workflow_id"])
	nodeID := int64FromAny(rows[0]["workflow_node_id"])
	if workflowID == 0 && nodeID != 0 {
		_, workflowID, err = stateUpdateWizardNodeInfo(env, nodeID)
		if err != nil {
			return 0, 0, err
		}
	}
	return workflowID, nodeID, nil
}

func stateUpdateWizardNodeInfo(env *record.Env, nodeID int64) (string, int64, error) {
	rows, err := env.Model(ModelNode).Browse(nodeID).Read("state", "workflow_id")
	if err != nil || len(rows) == 0 {
		return "", 0, err
	}
	return stringFromAny(rows[0]["state"]), int64FromAny(rows[0]["workflow_id"]), nil
}

func workflowWizardRecordName(env *record.Env, modelName string, recordID int64) (string, error) {
	names, err := env.Model(modelName).Browse(recordID).NameGet()
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return stringFromAny(names[0][1]), nil
}

func workflowWizardTransitionCommentRequired(workflow Workflow, transitionID int64) bool {
	if transitionID == 0 {
		return false
	}
	transition, ok := workflowTransitionByID(workflow, transitionID)
	return ok && transition.Comment == CommentRequired
}
