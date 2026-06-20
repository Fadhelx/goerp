package oi_login_as

import (
	"gorp/internal/record"
	"gorp/internal/security"
)

const (
	GroupLoginAsUser           int64 = 7301
	GroupLoginAsAdmin          int64 = 7302
	GroupLoginAsAllowInactive  int64 = 7303
	GroupLoginAsAllowSuperuser int64 = 7304
	GroupLoginAsDebug          int64 = 7305
)

func ApplySecurity(engine *security.Engine) {
	for _, group := range SecurityGroups() {
		engine.Groups[group.ID] = group
	}
	engine.ACLs = append(engine.ACLs, SecurityACLs()...)
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupLoginAsUser, Name: "Login As / User"},
		{ID: GroupLoginAsAdmin, Name: "Login As / Administrator", ImpliedIDs: []int64{GroupLoginAsUser}},
		{ID: GroupLoginAsAllowInactive, Name: "Login As / Allow Inactive"},
		{ID: GroupLoginAsAllowSuperuser, Name: "Login As / Allow Superuser"},
		{ID: GroupLoginAsDebug, Name: "Login As / Debug Route"},
	}
}

func SecurityACLs() []security.ACL {
	return []security.ACL{
		{Model: ModelLoginAsWizard, GroupID: GroupLoginAsUser, PermRead: true, PermCreate: true},
		{Model: ModelLoginAsWizard, GroupID: GroupLoginAsAdmin, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
		{Model: ModelLoginAsAudit, GroupID: GroupLoginAsAdmin, PermRead: true},
		{Model: ModelLoginAsRoute, GroupID: GroupLoginAsAdmin, PermRead: true, PermWrite: true, PermCreate: true},
	}
}

func CheckLoginAsAccess(engine *security.Engine, userID int64, modelName string, op record.Operation) error {
	return engine.Check(record.Context{UserID: userID}, modelName, op, nil)
}
