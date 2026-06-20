package oi_workflow

import (
	"gorp/internal/domain"
	"gorp/internal/record"
	"gorp/internal/security"
	"gorp/internal/workflow"
)

const (
	GroupBaseUser        int64 = 1
	GroupBaseSystem      int64 = 3
	GroupWorkflowUser    int64 = 8201
	GroupWorkflowManager int64 = 8202
	GroupWorkflowAdmin   int64 = 8203
)

type RuleDefinition struct {
	Name string
	Rule security.Rule
}

func ApplySecurity(engine *security.Engine) {
	for _, group := range SourceCompatibleSecurityGroups() {
		engine.Groups[group.ID] = group
	}
	for _, group := range SecurityGroups() {
		engine.Groups[group.ID] = group
	}
	engine.ACLs = append(engine.ACLs, SecurityACLs()...)
	for _, definition := range SecurityRuleDefinitions() {
		engine.Rules = append(engine.Rules, definition.Rule)
	}
}

func SourceCompatibleSecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupBaseUser, Name: "base.group_user"},
		{ID: GroupBaseSystem, Name: "base.group_system", ImpliedIDs: []int64{GroupBaseUser}},
	}
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupWorkflowUser, Name: "Workflow / User"},
		{ID: GroupWorkflowManager, Name: "Workflow / Manager", ImpliedIDs: []int64{GroupWorkflowUser}},
		{ID: GroupWorkflowAdmin, Name: "Workflow / Administrator", ImpliedIDs: []int64{GroupWorkflowManager}},
	}
}

func SecurityACLs() []security.ACL {
	models := securedModelNames()
	acls := make([]security.ACL, 0, len(models)*3)
	for _, modelName := range models {
		acls = append(acls,
			readACL(modelName, GroupWorkflowUser),
			writeACL(modelName, GroupWorkflowManager),
			crudACL(modelName, GroupWorkflowAdmin),
		)
	}
	acls = append(acls, SourceCompatibleACLs()...)
	return acls
}

func SourceCompatibleACLs() []security.ACL {
	return []security.ACL{
		sourceReadACL("access_approval_log_user", workflow.ModelLog, GroupBaseUser),
		sourceCrudACL("access_approval_config_system", workflow.ModelConfig, GroupBaseSystem),
		sourceReadACL("access_approval_config_user", workflow.ModelConfig, GroupBaseUser),
		sourceCrudACL("access_approval_escalation_system", workflow.ModelEscalation, GroupBaseSystem),
		sourceCrudACL("access_approval_settings_system", workflow.ModelSettings, GroupBaseSystem),
		sourceCrudACL("access_approval_settings_state_system", workflow.ModelSettingsState, GroupBaseSystem),
		sourceCrudACL("access_approval_state_update_system", workflow.ModelStateUpdateWizard, GroupBaseSystem),
		sourceCrudACL("access_cancellation_record", workflow.ModelCancellation, GroupBaseUser),
		sourceCrudACL("access_state_tags", workflow.ModelStateTags, GroupBaseSystem),
		sourceCrudACL("access_approval_buttons", workflow.ModelButton, GroupBaseSystem),
		sourceReadACL("access_approval_buttons_user", workflow.ModelButton, GroupBaseUser),
		sourceCrudACL("access_model_approval_automation", workflow.ModelAutomation, GroupBaseSystem),
		sourceCrudACL("access_model_approval_forward", workflow.ModelForward, GroupBaseSystem),
		sourceCrudACL("access_approval_process_wizard_user", workflow.ModelProcessWizard, GroupBaseUser),
		sourceCrudACL("access_model_expression_editor_system", workflow.ModelExpressionEditor, GroupBaseSystem),
		sourceCrudACL("access_approval_log_voting", workflow.ModelLogVoting, GroupBaseSystem),
		sourceReadACL("access_approval_log_voting_user", workflow.ModelLogVoting, GroupBaseUser),
	}
}

func SecurityRuleDefinitions() []RuleDefinition {
	var definitions []RuleDefinition
	for _, modelName := range companyScopedModelNames() {
		definitions = append(definitions, RuleDefinition{
			Name: "oi_workflow_company_" + modelName,
			Rule: security.Rule{
				Model:      modelName,
				Domain:     CompanyDomain(),
				Global:     true,
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: true,
				Active:     true,
			},
		})
	}
	return definitions
}

func SecurityRules() []security.Rule {
	definitions := SecurityRuleDefinitions()
	rules := make([]security.Rule, 0, len(definitions))
	for _, definition := range definitions {
		rules = append(rules, definition.Rule)
	}
	return rules
}

func CompanyDomain() domain.Node {
	return domain.Or(
		domain.Cond("company_id", domain.Equal, nil),
		domain.Cond("company_id", domain.In, "user.company_ids"),
	)
}

func readACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Model: modelName, GroupID: groupID, PermRead: true}
}

func writeACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Model: modelName, GroupID: groupID, PermRead: true, PermWrite: true, PermCreate: true}
}

func crudACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Model: modelName, GroupID: groupID, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true}
}

func sourceReadACL(name string, modelName string, groupID int64) security.ACL {
	acl := readACL(modelName, groupID)
	acl.Name = name
	return acl
}

func sourceCrudACL(name string, modelName string, groupID int64) security.ACL {
	acl := crudACL(modelName, groupID)
	acl.Name = name
	return acl
}

func securedModelNames() []string {
	names := workflow.ModelNames()
	out := names[:0]
	for _, name := range names {
		if name != workflow.ModelApprovalRecord {
			out = append(out, name)
		}
	}
	return out
}

func companyScopedModelNames() []string {
	return []string{
		workflow.ModelSettings,
		workflow.ModelConfig,
		workflow.ModelAutomation,
		workflow.ModelEscalation,
		workflow.ModelLog,
		workflow.ModelForward,
		workflow.ModelCancellation,
	}
}

func CanApprove(engine *security.Engine, userID int64, modelName string, row map[string]any) bool {
	if err := engine.Check(record.Context{UserID: userID}, modelName, record.OpWrite, nil); err != nil {
		return false
	}
	ok, err := engine.AllowedByRecordRules(userID, modelName, record.OpWrite, row)
	return err == nil && ok
}
