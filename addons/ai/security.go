package ai

import (
	"gorp/internal/domain"
	"gorp/internal/security"
)

const (
	GroupBaseUser   int64 = 1
	GroupBaseSystem int64 = 3
	GroupAIUser           = GroupBaseUser
	GroupAIAdmin          = GroupBaseSystem
)

type RuleDefinition struct {
	Name string
	Rule security.Rule
}

func ApplySecurity(engine *security.Engine) {
	for _, group := range SecurityGroups() {
		engine.Groups[group.ID] = group
	}
	engine.ACLs = append(engine.ACLs, SecurityACLs()...)
	for _, rule := range SecurityRuleDefinitions() {
		engine.Rules = append(engine.Rules, rule.Rule)
	}
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupAIUser, Name: "base.group_user"},
		{ID: GroupAIAdmin, Name: "base.group_system", ImpliedIDs: []int64{GroupAIUser}},
	}
}

func SecurityACLs() []security.ACL {
	acls := make([]security.ACL, 0, len(AISecuredModelNames())*2+1)
	for _, modelName := range AISecuredModelNames() {
		acls = append(acls,
			readACL(modelName, GroupAIUser),
			crudACL(modelName, GroupAIAdmin),
		)
	}
	acls = append(acls, crudACL(ModelSettings, GroupAIAdmin))
	return acls
}

func SecurityRuleDefinitions() []RuleDefinition {
	models := append(AISecuredModelNames(), ModelSettings)
	definitions := make([]RuleDefinition, 0, len(models))
	for _, modelName := range models {
		definitions = append(definitions, RuleDefinition{
			Name: "ai_company_" + modelName,
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

func AISecuredModelNames() []string {
	return []string{
		ModelAgent,
		ModelTopic,
		ModelAgentSource,
		ModelPromptButton,
		ModelEmbedding,
		ModelComposer,
	}
}

func CompanyDomain() domain.Node {
	return domain.Or(
		domain.Cond("company_id", domain.Equal, nil),
		domain.Cond("company_id", domain.In, "user.company_ids"),
	)
}

func readACL(modelName string, groupID int64) security.ACL {
	return security.ACL{
		Model:    modelName,
		GroupID:  groupID,
		PermRead: true,
	}
}

func crudACL(modelName string, groupID int64) security.ACL {
	return security.ACL{
		Model:      modelName,
		GroupID:    groupID,
		PermRead:   true,
		PermWrite:  true,
		PermCreate: true,
		PermUnlink: true,
	}
}
