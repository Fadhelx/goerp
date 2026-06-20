package oi_delegation

import (
	"gorp/internal/domain"
	"gorp/internal/record"
	"gorp/internal/security"
)

const (
	GroupBaseUser int64 = 1

	GroupDelegationUser    int64 = 7201
	GroupDelegationManager int64 = 7202
	GroupDelegationAdmin   int64 = 7203

	GroupDelegableRole         int64 = 7210
	GroupDelegableMultipleRole int64 = 7211

	GroupDelegationEmployee        int64 = 7220
	GroupDelegationEmployeeManager int64 = 7221
	GroupDelegationSourceAdmin     int64 = 7222
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
	for _, rule := range SecurityRules() {
		engine.Rules = append(engine.Rules, rule)
	}
}

func SourceCompatibleSecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupBaseUser, Name: "base.group_user"},
		{ID: GroupDelegationEmployee, Name: "Employee"},
		{ID: GroupDelegationEmployeeManager, Name: "Employee Manager", ImpliedIDs: []int64{GroupDelegationEmployee}},
		{ID: GroupDelegationSourceAdmin, Name: "Administrator", ImpliedIDs: []int64{GroupDelegationEmployeeManager}},
	}
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupDelegationUser, Name: "Delegation / User"},
		{ID: GroupDelegationManager, Name: "Delegation / Manager", ImpliedIDs: []int64{GroupDelegationUser}},
		{ID: GroupDelegationAdmin, Name: "Delegation / Administrator", ImpliedIDs: []int64{GroupDelegationManager}},
		{ID: GroupDelegableRole, Name: "Delegable Role"},
		{ID: GroupDelegableMultipleRole, Name: "Delegable Multiple Role"},
	}
}

func SecurityACLs() []security.ACL {
	var acls []security.ACL
	for _, modelName := range ModelNames() {
		acls = append(acls,
			security.ACL{Model: modelName, GroupID: GroupDelegationUser, PermRead: true},
			security.ACL{Model: modelName, GroupID: GroupDelegationManager, PermRead: true, PermWrite: true, PermCreate: true},
			security.ACL{Model: modelName, GroupID: GroupDelegationAdmin, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
		)
	}
	acls = append(acls, SourceCompatibleACLs()...)
	return acls
}

func SourceCompatibleACLs() []security.ACL {
	return []security.ACL{
		{Name: "access_delegation_employee", Model: ModelDelegation, GroupID: GroupDelegationEmployee, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
		{Name: "access_delegation_line_employee", Model: ModelDelegationLine, GroupID: GroupDelegationEmployee, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
		{Name: "access_delegation_line_user", Model: ModelDelegationLine, GroupID: GroupBaseUser, PermRead: true},
	}
}

func SecurityRuleDefinitions() []RuleDefinition {
	definitions := SourceCompatibleRuleDefinitions()
	definitions = append(definitions, []RuleDefinition{
		{
			Name: "delegation_user_own_request",
			Rule: security.Rule{
				Model:      ModelDelegation,
				GroupIDs:   []int64{GroupDelegationUser},
				Domain:     domain.Cond("user_id", domain.Equal, "user.id"),
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: false,
				Active:     true,
			},
		},
		{
			Name: "delegation_line_delegate_or_delegator",
			Rule: security.Rule{
				Model:    ModelDelegationLine,
				GroupIDs: []int64{GroupDelegationUser},
				Domain: domain.Or(
					domain.Cond("user_id", domain.Equal, "user.id"),
					domain.Cond("delegator_user_id", domain.Equal, "user.id"),
				),
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: false,
				Active:     true,
			},
		},
		{
			Name: "delegation_manager_all_requests",
			Rule: security.Rule{
				Model:      ModelDelegation,
				GroupIDs:   []int64{GroupDelegationManager},
				Domain:     domain.And(),
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: true,
				Active:     true,
			},
		},
		{
			Name: "delegation_line_manager_all",
			Rule: security.Rule{
				Model:      ModelDelegationLine,
				GroupIDs:   []int64{GroupDelegationManager},
				Domain:     domain.And(),
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: true,
				Active:     true,
			},
		},
	}...)
	return definitions
}

func SourceCompatibleRuleDefinitions() []RuleDefinition {
	return []RuleDefinition{
		{
			Name: "delegation_employee",
			Rule: security.Rule{
				Model:      ModelDelegation,
				GroupIDs:   []int64{GroupDelegationEmployee},
				Domain:     domain.Cond("user_id", domain.Equal, "user.id"),
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: true,
				Active:     true,
			},
		},
		{
			Name: "delegation_manager",
			Rule: security.Rule{
				Model:      ModelDelegation,
				GroupIDs:   []int64{GroupDelegationEmployeeManager},
				Domain:     domain.Cond("employee_id.parent_id.user_id", domain.Equal, "user.id"),
				PermRead:   true,
				PermWrite:  false,
				PermCreate: false,
				PermUnlink: false,
				Active:     true,
			},
		},
		{
			Name: "delegation_admin",
			Rule: security.Rule{
				Model:      ModelDelegation,
				GroupIDs:   []int64{GroupDelegationSourceAdmin},
				Domain:     domain.And(),
				PermRead:   true,
				PermWrite:  false,
				PermCreate: false,
				PermUnlink: false,
				Active:     true,
			},
		},
	}
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

func CheckDelegationAccess(engine *security.Engine, userID int64, modelName string, op record.Operation) error {
	return engine.Check(record.Context{UserID: userID}, modelName, op, nil)
}
