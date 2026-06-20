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
