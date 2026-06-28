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
	ApplySecurityWithGroups(engine, GroupAIUser, GroupAIAdmin)
}

func ApplySecurityWithGroups(engine *security.Engine, userGroupID int64, adminGroupID int64) {
	if engine == nil {
		return
	}
	if userGroupID == 0 {
		userGroupID = GroupAIUser
	}
	if adminGroupID == 0 {
		adminGroupID = GroupAIAdmin
	}
	for _, group := range securityGroups(userGroupID, adminGroupID) {
		engine.Groups[group.ID] = mergeAIGroup(engine.Groups[group.ID], group)
	}
	engine.ACLs = append(engine.ACLs, securityACLs(userGroupID, adminGroupID)...)
	for _, rule := range SecurityRuleDefinitions() {
		engine.Rules = append(engine.Rules, rule.Rule)
	}
}

func mergeAIGroup(existing security.Group, fallback security.Group) security.Group {
	if existing.ID == 0 {
		return fallback
	}
	if existing.Name == "" {
		existing.Name = fallback.Name
	}
	for _, impliedID := range fallback.ImpliedIDs {
		if !hasAIGroupID(existing.ImpliedIDs, impliedID) {
			existing.ImpliedIDs = append(existing.ImpliedIDs, impliedID)
		}
	}
	return existing
}

func hasAIGroupID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func SecurityGroups() []security.Group {
	return securityGroups(GroupAIUser, GroupAIAdmin)
}

func securityGroups(userGroupID int64, adminGroupID int64) []security.Group {
	return []security.Group{
		{ID: userGroupID, Name: "base.group_user"},
		{ID: adminGroupID, Name: "base.group_system", ImpliedIDs: []int64{userGroupID}},
	}
}

func SecurityACLs() []security.ACL {
	return securityACLs(GroupAIUser, GroupAIAdmin)
}

func securityACLs(userGroupID int64, adminGroupID int64) []security.ACL {
	acls := make([]security.ACL, 0, len(AISecuredModelNames())*2+1)
	for _, modelName := range AISecuredModelNames() {
		acls = append(acls,
			readACL(modelName, userGroupID),
			crudACL(modelName, adminGroupID),
		)
	}
	acls = append(acls, crudACL(ModelSettings, adminGroupID))
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
		ModelAuditLog,
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
