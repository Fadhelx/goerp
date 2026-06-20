package oi_workflow_advance

import (
	"gorp/addons/oi_workflow"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	internalworkflow "gorp/internal/workflow"
)

const (
	ModuleName              = "oi_workflow_advance"
	WorkflowDependencyName  = "oi_workflow"
	FlowchartDependencyName = "oi_web_flowchart"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "Workflow Engine Advance",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Productivity",
		Depends:       []string{WorkflowDependencyName, FlowchartDependencyName},
		Data: []string{
			"security/ir_rule.xml",
			"security/ir.model.access.csv",
			"views/approval_settings.xml",
			"views/approval_config.xml",
			"views/workflow_node.xml",
			"views/workflow.xml",
			"views/workflow_transition.xml",
			"views/workflow_process_wizard.xml",
			"views/approval_state_update.xml",
			"views/approval_log.xml",
			"views/action.xml",
			"data/ir_cron.xml",
		},
		Assets: map[string][]string{
			"web.assets_backend": {
				"static/src/js/list_controller.js",
				"static/src/xml/templates.xml",
			},
		},
		Installable:   true,
		Application:   false,
		SourceVersion: "18.0.1.4.25.cleanroom",
		SourceLicense: "clean-room implementation; no proprietary source or assets copied",
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{
			Name:          "OI Workflow",
			TechnicalName: WorkflowDependencyName,
			Version:       "19.0.1.0.0",
			Category:      "Productivity",
			Depends:       []string{"base"},
			Installable:   true,
			Application:   false,
			SourceLicense: "dependency placeholder",
		},
		{
			Name:          "Web",
			TechnicalName: "web",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
			Application:   false,
			SourceLicense: "dependency placeholder",
		},
		{
			Name:          "OI Web Flowchart",
			TechnicalName: FlowchartDependencyName,
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base", "web"},
			Installable:   true,
			Application:   false,
			SourceLicense: "dependency placeholder",
		},
	}
}

func Models() []model.Model {
	return internalworkflow.AdvancedModels()
}

func ExtensionModels() []model.Model {
	return internalworkflow.AdvancedExtensionModels()
}

func ModelNames() []string {
	return internalworkflow.AdvancedModelNames()
}

func RegisterModels(reg *registry.Registry) error {
	return internalworkflow.RegisterAdvancedModels(reg)
}

func RegisterRecordModels(reg *record.Registry) error {
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			return err
		}
	}
	return nil
}

func ApprovalLogExtensionFields() []field.Field {
	return internalworkflow.ApprovalLogFields()
}

func SecurityRuleDefinitions() []internalworkflow.RuleDefinition {
	return internalworkflow.AdvancedSecurityRuleDefinitions()
}

func SecurityACLs() []security.ACL {
	models := ModelNames()
	acls := make([]security.ACL, 0, len(models)*3)
	userGroupID, managerGroupID, adminGroupID := RuntimeWorkflowGroupIDs()
	for _, modelName := range models {
		acls = append(acls,
			readACL(modelName, userGroupID),
			writeACL(modelName, managerGroupID),
			crudACL(modelName, adminGroupID),
		)
	}
	acls = append(acls, SourceCompatibleACLs()...)
	return acls
}

func RuntimeWorkflowGroupIDs() (int64, int64, int64) {
	return oi_workflow.GroupWorkflowUser, oi_workflow.GroupWorkflowManager, oi_workflow.GroupWorkflowAdmin
}

func SourceCompatibleACLs() []security.ACL {
	acls := make([]security.ACL, 0, 11)
	definitions := []struct {
		model      string
		userName   string
		systemName string
	}{
		{internalworkflow.ModelWorkflow, "access_workflow_group_user", "access_workflow_group_system"},
		{internalworkflow.ModelNode, "access_workflow_node_group_user", "access_workflow_node_group_system"},
		{internalworkflow.ModelNodeAction, "access_workflow_node_action_group_user", "access_workflow_node_action_group_system"},
		{internalworkflow.ModelTransition, "access_workflow_transition_group_user", "access_workflow_transition_group_system"},
		{internalworkflow.ModelProcess, "access_workflow_process_group_user", "access_workflow_process_group_system"},
	}
	for _, definition := range definitions {
		acls = append(acls,
			sourceReadACL(definition.userName, definition.model, oi_workflow.GroupBaseUser),
			sourceCrudACL(definition.systemName, definition.model, oi_workflow.GroupBaseSystem),
		)
	}
	acls = append(acls, sourceCrudACL("access_workflow_process_wizard", internalworkflow.ModelWorkflowWizard, oi_workflow.GroupBaseUser))
	return acls
}

func SecurityRules() []security.Rule {
	definitions := SecurityRuleDefinitions()
	rules := make([]security.Rule, 0, len(definitions))
	for _, definition := range definitions {
		rule := definition.Rule
		if rule.Name == "" {
			rule.Name = definition.Name
		}
		rules = append(rules, rule)
	}
	return rules
}

func ApplySecurity(engine *security.Engine) {
	for _, group := range oi_workflow.SourceCompatibleSecurityGroups() {
		engine.Groups[group.ID] = group
	}
	for _, group := range oi_workflow.SecurityGroups() {
		engine.Groups[group.ID] = group
	}
	engine.ACLs = append(engine.ACLs, SecurityACLs()...)
	for _, rule := range SecurityRules() {
		engine.Rules = append(engine.Rules, rule)
	}
}

func readACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Name: modelName + " user", Model: modelName, GroupID: groupID, Active: true, PermRead: true}
}

func writeACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Name: modelName + " manager", Model: modelName, GroupID: groupID, Active: true, PermRead: true, PermWrite: true, PermCreate: true}
}

func crudACL(modelName string, groupID int64) security.ACL {
	return security.ACL{Name: modelName + " administrator", Model: modelName, GroupID: groupID, Active: true, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true}
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
