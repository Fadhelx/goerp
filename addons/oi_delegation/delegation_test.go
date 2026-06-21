package oi_delegation

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorp/addons/hr"
	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/mail"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	"gorp/internal/workflow"
)

func TestManifestInstallAndModels(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || !manifest.Application {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"base", "hr", "mail", "oi_base", "oi_workflow"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if manifest.SourceVersion != "18.0.1.0.14" || manifest.SourceLicense == "" {
		t.Fatalf("source metadata = %+v", manifest)
	}

	reg := registry.New("test")
	manifests := []module.Manifest{base.Manifest()}
	manifests = append(manifests, DependencyManifests()...)
	manifests = append(manifests, manifest)
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	if reg.States[ModuleName] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ModelDelegation, ModelDelegationLine, ModelMailTemplateMetadata, ModelCacheEvent, ModelWorkflowHook} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	request := reg.Models[ModelDelegation]
	for _, fieldName := range []string{"date_from", "date_to", "employee_id", "user_id", "one_employee", "delegateTo_employee_id", "delegate_to_employee_id", "delegate_to_user_id", "state", "lines", "department_ids"} {
		if _, ok := request.Fields[fieldName]; !ok {
			t.Fatalf("missing delegation field %s", fieldName)
		}
	}
	if !request.Fields["date_from"].Required || !request.Fields["date_to"].Required || !request.Fields["employee_id"].Required {
		t.Fatalf("delegation required metadata = %+v %+v %+v", request.Fields["date_from"], request.Fields["date_to"], request.Fields["employee_id"])
	}
	if !request.Fields["user_id"].Readonly || request.Fields["user_id"].RelationField != "employee_id.user_id" {
		t.Fatalf("delegation user_id metadata = %+v", request.Fields["user_id"])
	}
	if request.Fields["lines"].Relation != ModelDelegationLine || request.Fields["lines"].RelationField != "delegation_id" {
		t.Fatalf("delegation lines metadata = %+v", request.Fields["lines"])
	}
	line := reg.Models[ModelDelegationLine]
	if !line.Fields["delegation_id"].Required || !line.Fields["group_id"].Required {
		t.Fatalf("delegation line required metadata = %+v %+v", line.Fields["delegation_id"], line.Fields["group_id"])
	}
	if !line.Fields["user_id"].Readonly || !line.Fields["user_id"].Index || line.Fields["user_id"].RelationField != "employee_id.user_id" {
		t.Fatalf("delegation line user_id metadata = %+v", line.Fields["user_id"])
	}
	if !line.Fields["delegator_user_id"].Readonly || !line.Fields["delegator_user_id"].Index || line.Fields["delegator_user_id"].RelationField != "delegation_id.user_id" {
		t.Fatalf("delegation line delegator_user_id metadata = %+v", line.Fields["delegator_user_id"])
	}
	extensions := extensionMap()
	for _, fieldName := range []string{"allow_delegation", "delegation_template_ids", "name_delegation", "allow_multiple_delegation", "restricted_access"} {
		if _, ok := extensions["res.groups"].Fields[fieldName]; !ok {
			t.Fatalf("missing res.groups extension field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"delegation_ids", "has_delegation", "active_groups_ids", "delegated_user_ids", "delegator_user_ids"} {
		if _, ok := extensions["res.users"].Fields[fieldName]; !ok {
			t.Fatalf("missing res.users extension field %s", fieldName)
		}
	}
}

func TestDelegationSourceFieldAliasesAndLineUniqueness(t *testing.T) {
	env := delegationFixtureEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorUserID, err := env.Model("res.users").Create(map[string]any{"login": "delegator", "name": "Delegator"})
	if err != nil {
		t.Fatal(err)
	}
	delegateUserID, err := env.Model("res.users").Create(map[string]any{"login": "delegate", "name": "Delegate"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegator", "user_id": delegatorUserID})
	if err != nil {
		t.Fatal(err)
	}
	delegateEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegate", "user_id": delegateUserID})
	if err != nil {
		t.Fatal(err)
	}
	requestID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_to":                "2026-12-31",
		"employee_id":            delegatorEmployeeID,
		"delegateTo_employee_id": delegateEmployeeID,
		"one_employee":           true,
		"state":                  "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model(ModelDelegation).Browse(requestID).Read("date_from", "user_id", "delegateTo_employee_id", "delegate_to_employee_id", "delegate_to_user_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["date_from"] == nil || rows[0]["user_id"] != delegatorUserID || rows[0]["delegateTo_employee_id"] != delegateEmployeeID || rows[0]["delegate_to_employee_id"] != delegateEmployeeID || rows[0]["delegate_to_user_id"] != delegateUserID {
		t.Fatalf("delegation alias row = %+v", rows)
	}
	lineID, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": requestID, "group_id": groupID})
	if err != nil {
		t.Fatal(err)
	}
	lineRows, err := env.Model(ModelDelegationLine).Browse(lineID).Read("active", "employee_id", "user_id", "delegator_id", "delegator_user_id", "one_employee", "state", "date_from", "date_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(lineRows) != 1 || lineRows[0]["active"] != true || lineRows[0]["employee_id"] != delegateEmployeeID || lineRows[0]["user_id"] != delegateUserID || lineRows[0]["delegator_id"] != delegatorEmployeeID || lineRows[0]["delegator_user_id"] != delegatorUserID || lineRows[0]["one_employee"] != true || lineRows[0]["state"] != "confirmed" || lineRows[0]["date_to"] != "2026-12-31" {
		t.Fatalf("delegation line related row = %+v", lineRows)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": requestID, "group_id": groupID, "employee_id": delegateEmployeeID}); err == nil || !strings.Contains(err.Error(), "role should be unique") {
		t.Fatalf("expected source unique role error, got %v", err)
	}
}

func TestDelegationDefaultsAndLineCommandMaterialization(t *testing.T) {
	env := delegationFixtureEnv(t)
	alphaGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Alpha", "full_name": "Role / Alpha", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	blockedGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Blocked", "full_name": "Role / Blocked", "allow_delegation": false})
	if err != nil {
		t.Fatal(err)
	}
	betaGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Beta", "full_name": "Role / Beta", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorUserID, err := env.Model("res.users").Create(map[string]any{"id": int64(1), "login": "delegator", "name": "Delegator", "groups_id": []int64{alphaGroupID, blockedGroupID, betaGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegator", "user_id": delegatorUserID})
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := env.Model(ModelDelegation).DefaultGet([]string{"date_from", "employee_id", "user_id", "lines"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults["date_from"] == nil || defaults["employee_id"] != delegatorEmployeeID || defaults["user_id"] != delegatorUserID {
		t.Fatalf("delegation defaults = %+v", defaults)
	}
	if _, ok := defaults["lines"]; ok {
		t.Fatalf("delegation defaults unexpectedly seeded lines = %+v", defaults)
	}

	delegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_to": "2099-12-31",
		"lines": []any{
			[]any{int64(0), false, map[string]any{"group_id": alphaGroupID}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	lineRows, err := env.Model(ModelDelegationLine).Search(domain.Cond("delegation_id", domain.Equal, delegationID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := lineRows.Read("group_id", "delegator_id", "delegator_user_id", "date_to", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["group_id"] != alphaGroupID || rows[0]["delegator_id"] != delegatorEmployeeID || rows[0]["delegator_user_id"] != delegatorUserID || rows[0]["date_to"] != "2099-12-31" || rows[0]["active"] != true {
		t.Fatalf("delegation line command rows = %+v", rows)
	}

	if err := env.Model(ModelDelegation).Browse(delegationID).Write(map[string]any{
		"lines": []any{
			[]any{int64(5), false, false},
			[]any{int64(0), false, map[string]any{"group_id": betaGroupID}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := env.Model(ModelDelegationLine).Search(domain.Cond("delegation_id", domain.Equal, delegationID))
	if err != nil {
		t.Fatal(err)
	}
	afterRows, err := after.Read("group_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRows) != 1 || afterRows[0]["group_id"] != betaGroupID {
		t.Fatalf("delegation line command replacement rows = %+v", afterRows)
	}
}

func TestDelegationPersistedDateSelfAndActiveParity(t *testing.T) {
	env := delegationFixtureEnv(t)
	delegatorUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Owner", "user_id": delegatorUserID})
	if err != nil {
		t.Fatal(err)
	}
	delegateEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegate"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2099-02-01", "date_to": "2099-01-01", "employee_id": delegatorEmployeeID}); err == nil || !strings.Contains(err.Error(), "From Date > To Date") {
		t.Fatalf("expected source date range error, got %v", err)
	}
	if _, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2099-01-01", "date_to": "2099-02-01", "employee_id": delegatorEmployeeID, "delegateTo_employee_id": delegatorEmployeeID}); err == nil || !strings.Contains(err.Error(), "delegator") {
		t.Fatalf("expected source self-delegation error, got %v", err)
	}
	activeID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "delegateTo_employee_id": delegateEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	inactiveID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "state": "revoked"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model(ModelDelegation).Browse(activeID, inactiveID).Read("isactive")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["isactive"] != true || rows[1]["isactive"] != false {
		t.Fatalf("isactive rows = %+v", rows)
	}
}

func TestDelegationConfirmGuardsRolesPastDateAndOverlap(t *testing.T) {
	env := delegationFixtureEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Expense", "full_name": "Role / Expense", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	multipleGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Multiple", "full_name": "Role / Multiple", "allow_delegation": true, "allow_multiple_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, delegateEmployeeID := createDelegationEmployees(t, env, "owner")
	_, secondDelegateID := createDelegationEmployees(t, env, "delegate2")

	existingID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2098-12-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": existingID, "group_id": groupID, "employee_id": delegateEmployeeID}); err != nil {
		t.Fatal(err)
	}
	candidateID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2099-01-01",
		"date_to":     "2099-01-31",
		"employee_id": delegatorEmployeeID,
		"lines":       []any{[]any{int64(0), false, map[string]any{"group_id": groupID, "employee_id": secondDelegateID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(candidateID).Write(map[string]any{"state": "confirmed"}); err == nil || !strings.Contains(err.Error(), "overlapped delegation") {
		t.Fatalf("expected overlap error, got %v", err)
	}

	noRoleID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2099-01-01", "date_to": "2099-01-31", "employee_id": delegatorEmployeeID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(noRoleID).Write(map[string]any{"state": "confirmed"}); err == nil || !strings.Contains(err.Error(), "No roles assigned") {
		t.Fatalf("expected roles assigned error, got %v", err)
	}

	pastID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2000-01-01",
		"date_to":     "2099-01-31",
		"employee_id": delegatorEmployeeID,
		"lines":       []any{[]any{int64(0), false, map[string]any{"group_id": multipleGroupID, "employee_id": delegateEmployeeID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(pastID).Write(map[string]any{"state": "confirmed"}); err == nil || !strings.Contains(err.Error(), "old date") {
		t.Fatalf("expected old-date error, got %v", err)
	}

	allowedExistingID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2098-12-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": allowedExistingID, "group_id": multipleGroupID, "employee_id": delegateEmployeeID}); err != nil {
		t.Fatal(err)
	}
	allowedID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2099-01-01",
		"date_to":     "2099-01-31",
		"employee_id": delegatorEmployeeID,
		"lines":       []any{[]any{int64(0), false, map[string]any{"group_id": multipleGroupID, "employee_id": secondDelegateID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(allowedID).Write(map[string]any{"state": "confirmed"}); err != nil {
		t.Fatalf("allow_multiple_delegation should skip overlap: %v", err)
	}
}

func TestDelegationRevokeAndExpireActionsSyncState(t *testing.T) {
	env := delegationFixtureEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, delegateEmployeeID := createDelegationEmployees(t, env, "revoke")
	currentID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	lineID, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": currentID, "group_id": groupID, "employee_id": delegateEmployeeID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(currentID).ActionRevokeDelegation(); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model(ModelDelegation).Browse(currentID).Read("state", "isactive")
	if err != nil {
		t.Fatal(err)
	}
	lineRows, err := env.Model(ModelDelegationLine).Browse(lineID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "revoked" || rows[0]["isactive"] != false || lineRows[0]["state"] != "revoked" {
		t.Fatalf("revoke rows = %+v line=%+v", rows, lineRows)
	}

	pastID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2000-01-01", "date_to": "2000-12-31", "employee_id": delegatorEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(pastID).ActionRevokeDelegation(); err == nil || !strings.Contains(err.Error(), "old date") {
		t.Fatalf("expected old revoke error, got %v", err)
	}

	expireID, err := env.Model(ModelDelegation).Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorEmployeeID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	expireLineID, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": expireID, "group_id": groupID, "employee_id": delegateEmployeeID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(expireID).ExpireDelegation(); err != nil {
		t.Fatal(err)
	}
	expiredRows, err := env.Model(ModelDelegationLine).Browse(expireLineID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if expiredRows[0]["state"] != "expired" {
		t.Fatalf("expired line rows = %+v", expiredRows)
	}
}

func TestDelegationLifecyclePersistsApprovalLogs(t *testing.T) {
	env := delegationFixtureEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, delegateEmployeeID := createDelegationEmployees(t, env, "logs")
	delegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2099-01-01",
		"date_to":     "2099-01-31",
		"employee_id": delegatorEmployeeID,
		"state":       "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": delegationID, "group_id": groupID, "employee_id": delegateEmployeeID}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(delegationID).ActionConfirmDelegation(); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(delegationID).ActionRevokeDelegation(); err != nil {
		t.Fatal(err)
	}
	assertDelegationApprovalLogStates(t, env, delegationID, [][2]string{
		{"draft", "confirmed"},
		{"confirmed", "revoked"},
	})
	rows, err := env.Model(ModelDelegation).Browse(delegationID).Read("state", "last_state_update")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "revoked" || rows[0]["last_state_update"] == nil {
		t.Fatalf("delegation lifecycle row = %+v", rows)
	}

	expireID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2020-01-01",
		"date_to":     "2099-12-31",
		"employee_id": delegatorEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": expireID, "group_id": groupID, "employee_id": delegateEmployeeID}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model(ModelDelegation).Browse(expireID).ExpireDelegation(); err != nil {
		t.Fatal(err)
	}
	assertDelegationApprovalLogStates(t, env, expireID, [][2]string{{"confirmed", "expired"}})
}

func TestApprovalLogDelegationEmployeeRelatedField(t *testing.T) {
	env := delegationFixtureEnv(t)
	firstEmployeeID, _ := createDelegationEmployees(t, env, "approval-log-first")
	secondEmployeeID, _ := createDelegationEmployees(t, env, "approval-log-second")
	firstDelegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2099-01-01",
		"date_to":     "2099-01-31",
		"employee_id": firstEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondDelegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2099-02-01",
		"date_to":     "2099-02-28",
		"employee_id": secondEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	logID, err := env.Model(workflow.ModelLog).Create(map[string]any{
		"model":         "purchase.order",
		"record_id":     int64(42),
		"delegation_id": firstDelegationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model(workflow.ModelLog).Browse(logID).Read("delegation_id", "delegation_employee_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["delegation_id"] != firstDelegationID || rows[0]["delegation_employee_id"] != firstEmployeeID {
		t.Fatalf("approval log create related field = %+v", rows[0])
	}
	if err := env.Model(workflow.ModelLog).Browse(logID).Write(map[string]any{"delegation_id": secondDelegationID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model(workflow.ModelLog).Browse(logID).Read("delegation_id", "delegation_employee_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["delegation_id"] != secondDelegationID || rows[0]["delegation_employee_id"] != secondEmployeeID {
		t.Fatalf("approval log write related field = %+v", rows[0])
	}
}

func assertDelegationApprovalLogStates(t *testing.T, env *record.Env, delegationID int64, want [][2]string) {
	t.Helper()
	logs, err := env.Model(workflow.ModelLog).Search(domain.And(
		domain.Cond("model", "=", ModelDelegation),
		domain.Cond("record_id", "=", delegationID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := logs.Read("model", "record_id", "user_id", "old_state", "new_state", "duration_seconds", "duration_hours")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) {
		t.Fatalf("approval log rows = %+v, want %d", rows, len(want))
	}
	for index, expected := range want {
		row := rows[index]
		if row["model"] != ModelDelegation || row["record_id"] != delegationID || row["user_id"] != int64(1) || row["old_state"] != expected[0] || row["new_state"] != expected[1] {
			t.Fatalf("approval log[%d] = %+v, want %v", index, row, expected)
		}
		if row["duration_seconds"] == nil || row["duration_hours"] == nil {
			t.Fatalf("approval log duration[%d] = %+v", index, row)
		}
	}
}

func createDelegationEmployees(t *testing.T, env *record.Env, prefix string) (int64, int64) {
	t.Helper()
	userID, err := env.Model("res.users").Create(map[string]any{"login": prefix + "-user", "name": prefix + " User"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("hr.employee").Create(map[string]any{"name": prefix + " Delegator", "user_id": userID})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("hr.employee").Create(map[string]any{"name": prefix + " Delegate"})
	if err != nil {
		t.Fatal(err)
	}
	return delegatorID, delegateID
}

func TestMailMailCreateTemplateIDExpandsDelegationCC(t *testing.T) {
	env := delegationFixtureEnv(t)
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Owner", "email": "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner", "partner_id": partnerID, "groups_id": []int64{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Owner", "user_id": delegatorUserID, "work_email": "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	delegateEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegate", "work_email": "delegate@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	delegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2020-01-01",
		"date_to":     "2099-12-31",
		"employee_id": delegatorEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": delegationID, "group_id": groupID, "employee_id": delegateEmployeeID}); err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{"name": "Delegation CC", "model": "res.partner", "delegation_group_ids": []int64{groupID}, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"template_id":   templateID,
		"email_to":      "owner@example.com, direct@example.com",
		"email_cc":      "copy@example.com; delegate@example.com",
		"recipient_ids": []any{[]any{int64(4), partnerID}},
		"state":         "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.mail").Browse(mailID).Read("email_to", "email_cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "owner@example.com, direct@example.com" || rows[0]["email_cc"] != "copy@example.com; delegate@example.com" {
		t.Fatalf("mail with delegation cc = %+v", rows)
	}
	plainTemplateID, err := env.Model("mail.template").Create(map[string]any{"name": "Plain", "model": "res.partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	plainID, err := env.Model("mail.mail").Create(map[string]any{"template_id": plainTemplateID, "email_to": "owner@example.com", "email_cc": "plain@example.com", "state": "outgoing"})
	if err != nil {
		t.Fatal(err)
	}
	plainRows, err := env.Model("mail.mail").Browse(plainID).Read("email_cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(plainRows) != 1 || plainRows[0]["email_cc"] != "plain@example.com" {
		t.Fatalf("plain template cc = %+v", plainRows)
	}
}

func TestResUsersWriteDeactivatesInvalidDelegationLines(t *testing.T) {
	env := delegationFixtureEnv(t)
	keepGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Keep", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	removeGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Remove", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorUserID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner", "active": true, "groups_id": []int64{keepGroupID, removeGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Owner", "user_id": delegatorUserID})
	if err != nil {
		t.Fatal(err)
	}
	keepEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Keep Delegate"})
	if err != nil {
		t.Fatal(err)
	}
	removeEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Remove Delegate"})
	if err != nil {
		t.Fatal(err)
	}
	delegationID, err := env.Model(ModelDelegation).Create(map[string]any{
		"date_from":   "2020-01-01",
		"date_to":     "2099-12-31",
		"employee_id": delegatorEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	keepLineID, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": delegationID, "group_id": keepGroupID, "employee_id": keepEmployeeID})
	if err != nil {
		t.Fatal(err)
	}
	removeLineID, err := env.Model(ModelDelegationLine).Create(map[string]any{"delegation_id": delegationID, "group_id": removeGroupID, "employee_id": removeEmployeeID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(delegatorUserID).Write(map[string]any{"groups_id": []int64{keepGroupID}}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model(ModelDelegationLine).Browse(keepLineID, removeLineID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["active"] != true || rows[1]["active"] != false {
		t.Fatalf("group removal active rows = %+v", rows)
	}
	if err := env.Model("res.users").Browse(delegatorUserID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model(ModelDelegationLine).Browse(keepLineID, removeLineID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["active"] != false || rows[1]["active"] != false {
		t.Fatalf("archive active rows = %+v", rows)
	}
}

func TestDelegationManifestFixtureFiles(t *testing.T) {
	manifest := Manifest()
	for _, path := range manifest.Data {
		assertManifestDataFile(t, path)
	}

	env := delegationFixtureEnv(t)
	loader := delegationFixtureLoader(t, env)
	loadXMLFixture(t, loader, "security/group.xml")
	loadCSVFixture(t, loader, "ir.model.access", "security/ir.model.access.csv")
	loadXMLFixture(t, loader, "security/rules.xml")
	for _, path := range []string{
		"data/sequences.xml",
		"data/ir_cron.xml",
		"data/mail_template.xml",
		"data/approval_config.xml",
		"data/approval_buttons.xml",
	} {
		loadXMLFixture(t, loader, path)
	}
	for _, path := range []string{
		"view/delegation.xml",
		"view/res_groups.xml",
		"view/mail_template.xml",
		"view/approval_log.xml",
		"view/action.xml",
		"view/menu.xml",
	} {
		loadXMLFixture(t, loader, path)
	}

	ids := loader.ExternalIDs()
	for _, name := range []string{
		"base.group_user",
		ModuleName + ".module_category_delegation",
		ModuleName + ".delegation_employee",
		ModuleName + ".delegation_manager",
		ModuleName + ".delegation_admin",
		ModuleName + ".rule_delegation_employee",
		ModuleName + ".rule_delegation_manager",
		ModuleName + ".rule_delegation_admin",
		ModuleName + ".group_delegation_admin",
		ModuleName + ".model_delegation",
		ModuleName + ".access_delegation_admin",
		ModuleName + ".access_delegation_employee",
		ModuleName + ".access_delegation_line_employee",
		ModuleName + ".access_delegation_line_user",
		ModuleName + ".sequence_delegation",
		ModuleName + ".mail_template_delegation_assigned",
		ModuleName + ".approval_settings_delegation",
		ModuleName + ".button_delegation_confirm",
		ModuleName + ".view_delegation_form_list",
		ModuleName + ".view_delegation_form_search",
		ModuleName + ".view_delegation_form",
		ModuleName + ".view_approval_log_delegation_list",
		ModuleName + ".act_delegation",
		ModuleName + ".action_delegation",
		ModuleName + ".action_delegation_lines",
		ModuleName + ".menu_delegation",
		ModuleName + ".menu_oi_delegation_root",
		ModuleName + ".menu_oi_delegation_requests",
		ModuleName + ".cron_clear_access_cache",
	} {
		if ids[name].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", name, ids)
		}
	}

	accessRows, err := env.Model("ir.model.access").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if want := len(ModelNames())*3 + 3; accessRows.Len() != want {
		t.Fatalf("ACL count = %d, want %d", accessRows.Len(), want)
	}
	assertPersistedACL(t, env, ids[ModuleName+".access_delegation_employee"].ResID, ids[ModuleName+".model_delegation"].ResID, ids[ModuleName+".delegation_employee"].ResID, true, true, true, true)
	assertPersistedACL(t, env, ids[ModuleName+".access_delegation_line_employee"].ResID, ids[ModuleName+".model_delegation_line"].ResID, ids[ModuleName+".delegation_employee"].ResID, true, true, true, true)
	assertPersistedACL(t, env, ids[ModuleName+".access_delegation_line_user"].ResID, ids[ModuleName+".model_delegation_line"].ResID, ids["base.group_user"].ResID, true, false, false, false)
	assertPersistedRuleRawPerms(t, env, ids[ModuleName+".rule_delegation_manager"].ResID, false, false, false)
	assertPersistedRuleRawPerms(t, env, ids[ModuleName+".rule_delegation_admin"].ResID, false, false, false)
	assertGroupCategory(t, env, ids[ModuleName+".delegation_employee"].ResID, ids[ModuleName+".module_category_delegation"].ResID)
	assertGroupCategory(t, env, ids[ModuleName+".delegation_manager"].ResID, ids[ModuleName+".module_category_delegation"].ResID)
	assertGroupCategory(t, env, ids[ModuleName+".delegation_admin"].ResID, ids[ModuleName+".module_category_delegation"].ResID)
	assertGroupImplies(t, env, ids[ModuleName+".delegation_manager"].ResID, []int64{ids[ModuleName+".delegation_employee"].ResID})
	assertGroupImplies(t, env, ids[ModuleName+".delegation_admin"].ResID, []int64{ids[ModuleName+".delegation_manager"].ResID})
	assertGroupUsers(t, env, ids[ModuleName+".delegation_manager"].ResID, []int64{ids["base.user_root"].ResID, ids["base.user_admin"].ResID})
	assertGroupUsers(t, env, ids[ModuleName+".delegation_admin"].ResID, []int64{ids["base.user_root"].ResID, ids["base.user_admin"].ResID})
	assertGroupImplies(t, env, ids[ModuleName+".group_delegation_manager"].ResID, []int64{ids[ModuleName+".group_delegation_user"].ResID})
	assertGroupImplies(t, env, ids[ModuleName+".group_delegation_admin"].ResID, []int64{ids[ModuleName+".group_delegation_manager"].ResID})
	sequenceRows, err := env.Model("ir.sequence").Browse(ids[ModuleName+".sequence_delegation"].ResID).Read("code", "padding")
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRows[0]["code"] != "delegation" || sequenceRows[0]["padding"] != int64(5) {
		t.Fatalf("sequence = %+v", sequenceRows[0])
	}
	templateRows, err := env.Model("mail.template").Browse(ids[ModuleName+".mail_template_delegation_assigned"].ResID).Read("model", "subject", "active")
	if err != nil {
		t.Fatal(err)
	}
	if templateRows[0]["model"] != ModelDelegation || templateRows[0]["subject"] != "Delegation assigned" || templateRows[0]["active"] != true {
		t.Fatalf("template = %+v", templateRows[0])
	}
	buttonRows, err := env.Model("approval.buttons").Browse(ids[ModuleName+".button_delegation_confirm"].ResID).Read("action_type", "state_value", "next_state", "email_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if buttonRows[0]["action_type"] != "approve" || buttonRows[0]["state_value"] != "draft" || buttonRows[0]["next_state"] != "confirmed" || buttonRows[0]["email_template_id"] == nil {
		t.Fatalf("button = %+v", buttonRows[0])
	}
	if ids[ModuleName+".button_delegation_expire"].ResID != 0 {
		t.Fatalf("unexpected source fixture expire button id = %+v", ids[ModuleName+".button_delegation_expire"])
	}
	if ids[ModuleName+".approval_state_delegation_expired"].ResID != 0 {
		t.Fatalf("unexpected source fixture expired state id = %+v", ids[ModuleName+".approval_state_delegation_expired"])
	}
	sourceActionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".act_delegation"].ResID).Read("name", "res_model", "view_mode", "context")
	if err != nil {
		t.Fatal(err)
	}
	if sourceActionRows[0]["name"] != "Delegation" || sourceActionRows[0]["res_model"] != ModelDelegation ||
		sourceActionRows[0]["view_mode"] != "list,form" || sourceActionRows[0]["context"] != "{'delegation' : True}" {
		t.Fatalf("source action = %+v", sourceActionRows[0])
	}
	actionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".action_delegation"].ResID).Read("res_model", "view_mode", "context")
	if err != nil {
		t.Fatal(err)
	}
	if actionRows[0]["res_model"] != ModelDelegation || actionRows[0]["view_mode"] != "list,form" || actionRows[0]["context"] != "{'delegation' : True}" {
		t.Fatalf("action = %+v", actionRows[0])
	}
	lineActionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".action_delegation_lines"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if lineActionRows[0]["res_model"] != ModelDelegationLine || lineActionRows[0]["view_mode"] != "list,form" {
		t.Fatalf("line action = %+v", lineActionRows[0])
	}
	viewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_delegation_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := viewRows[0]["arch"].(string)
	for _, want := range []string{
		`button name="action_revoked"`,
		`field name="state" widget="statusbar"`,
		`field name="employee_id" groups="base.group_system" widget="many2one_avatar_employee" domain="[('user_id.active','=', True)]" readonly="state!='draft'"`,
		`field name="employee_id" groups="!base.group_system" widget="many2one_avatar_employee" readonly="1"`,
		`field name="delegateTo_employee_id" widget="many2one_avatar_employee"`,
		`required="one_employee"`,
		`field name="department_ids" widget="many2many_tags"`,
		`field name="lines" nolabel="1" colspan="2" readonly="state != 'draft'"`,
		`list editable="bottom" delete="false" create="false"`,
		`field name="group_id" readonly="1" force_save="1"`,
		`<chatter/>`,
	} {
		if !strings.Contains(arch, want) {
			t.Fatalf("form arch missing %q: %s", want, arch)
		}
	}
	if viewRows[0]["model"] != ModelDelegation {
		t.Fatalf("view = %+v", viewRows[0])
	}
	listRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_delegation_form_list"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	listArch, _ := listRows[0]["arch"].(string)
	for _, want := range []string{
		`field name="employee_id" widget="many2one_avatar_employee"`,
		`field name="state" widget="badge" decoration-success="state =='approved'" decoration-info="state =='draft'" decoration-warning="waiting_approval"`,
		`field name="waiting_approval" column_invisible="1"`,
	} {
		if listRows[0]["model"] != ModelDelegation || !strings.Contains(listArch, want) {
			t.Fatalf("list arch missing %q: %+v", want, listRows[0])
		}
	}
	searchRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_delegation_form_search"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	searchArch, _ := searchRows[0]["arch"].(string)
	for _, want := range []string{
		`filter string="Active" name="active" domain="[('state','=','confirmed'), ('date_from','&gt;=', current_date), ('date_to','&lt;=', current_date)]"`,
		`filter string="Draft" name="draft" domain="[('state','=','draft')]"`,
		`filter string="Confirm" name="confirm" domain="[('state','=','confirmed')]"`,
		`filter string="Status" name="status" context="{'group_by':'state'}"`,
		`filter string="Employee" name="employee" context="{'group_by':'employee_id'}"`,
	} {
		if searchRows[0]["model"] != ModelDelegation || !strings.Contains(searchArch, want) {
			t.Fatalf("search arch missing %q: %+v", want, searchRows[0])
		}
	}
	menuRows, err := env.Model("ir.ui.menu").Browse(ids[ModuleName+".menu_oi_delegation_requests"].ResID).Read("parent_id", "action")
	if err != nil {
		t.Fatal(err)
	}
	if menuRows[0]["parent_id"] != ids[ModuleName+".menu_oi_delegation_root"].ResID ||
		menuRows[0]["action"] != "ir.actions.act_window,action_delegation" {
		t.Fatalf("menu = %+v", menuRows[0])
	}
	sourceMenuRows, err := env.Model("ir.ui.menu").Browse(ids[ModuleName+".menu_delegation"].ResID).Read("name", "action", "web_icon")
	if err != nil {
		t.Fatal(err)
	}
	sourceMenuAction := "ir.actions.act_window," + strconv.FormatInt(ids[ModuleName+".act_delegation"].ResID, 10)
	if sourceMenuRows[0]["name"] != "Delegation" || sourceMenuRows[0]["action"] != sourceMenuAction ||
		sourceMenuRows[0]["web_icon"] != "oi_delegation,static/description/icon.png" {
		t.Fatalf("source menu = %+v, want action %s", sourceMenuRows[0], sourceMenuAction)
	}
	cronRows, err := env.Model("ir.cron").Browse(ids[ModuleName+".cron_clear_access_cache"].ResID).Read("name", "user_id", "active", "interval_number", "interval_type", "model_id", "state", "code", "action_name", "ir_actions_server_id")
	if err != nil {
		t.Fatal(err)
	}
	cronActionID, _ := cronRows[0]["ir_actions_server_id"].(int64)
	if cronRows[0]["name"] != "Clear Access Cache" ||
		cronRows[0]["user_id"] != ids["base.user_root"].ResID ||
		cronRows[0]["active"] != true ||
		cronRows[0]["interval_number"] != int64(1) ||
		cronRows[0]["interval_type"] != "days" ||
		cronRows[0]["model_id"] != ids[ModuleName+".model_delegation"].ResID ||
		cronRows[0]["state"] != "code" ||
		cronRows[0]["code"] != "model._clear_access_cache()" ||
		cronRows[0]["action_name"] != "delegation_clear_expired_access" ||
		cronActionID == 0 {
		t.Fatalf("cron = %+v", cronRows[0])
	}

	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	loadPersistedGroups(t, env, engine)
	engine.Users[10] = security.User{ID: 10, Login: "fixture-user", Active: true, GroupIDs: []int64{ids[ModuleName+".group_delegation_user"].ResID}}
	engine.Users[20] = security.User{ID: 20, Login: "fixture-manager", Active: true, GroupIDs: []int64{ids[ModuleName+".group_delegation_manager"].ResID}}
	engine.Users[30] = security.User{ID: 30, Login: "fixture-admin", Active: true, GroupIDs: []int64{ids[ModuleName+".group_delegation_admin"].ResID}}
	engine.Users[40] = security.User{ID: 40, Login: "source-employee", Active: true, GroupIDs: []int64{ids[ModuleName+".delegation_employee"].ResID}}
	engine.Users[50] = security.User{ID: 50, Login: "source-manager", Active: true, GroupIDs: []int64{ids[ModuleName+".delegation_manager"].ResID}}
	engine.Users[60] = security.User{ID: 60, Login: "source-admin", Active: true, GroupIDs: []int64{ids[ModuleName+".delegation_admin"].ResID}}
	engine.Users[70] = security.User{ID: 70, Login: "base-user", Active: true, GroupIDs: []int64{ids["base.group_user"].ResID}}
	if err := engine.Check(record.Context{UserID: 10}, ModelDelegation, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, ModelDelegation, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected fixture user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, ModelDelegation, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, ModelDelegation, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, ModelDelegation, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 50}, ModelDelegationLine, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 60}, ModelDelegation, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 70}, ModelDelegationLine, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 70}, ModelDelegationLine, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	assertSecurityRulePerms(t, engine.Rules, "delegation_manager", true, false, false, false)
	assertSecurityRulePerms(t, engine.Rules, "delegation_admin", true, false, false, false)
	assertSourceDelegationRuleBehavior(t, engine, 40, 50, 60)
	ok, err := engine.AllowedByRecordRules(10, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(10), "delegator_user_id": int64(50)})
	if err != nil || !ok {
		t.Fatalf("fixture own line denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(60), "delegator_user_id": int64(70)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("fixture foreign line allowed")
	}
	ok, err = engine.AllowedByRecordRules(20, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(60), "delegator_user_id": int64(70)})
	if err != nil || !ok {
		t.Fatalf("fixture manager foreign line denied: %v %v", ok, err)
	}
}

func TestDelegationRecordModelInheritsApprovalRecordFields(t *testing.T) {
	env := delegationFixtureEnv(t)
	metadata, ok := env.ModelMetadata(ModelDelegation)
	if !ok {
		t.Fatal("missing delegation metadata")
	}
	for _, fieldName := range []string{"approval_user_ids", "approval_done_user_ids", "approval_partner_ids", "user_can_approve", "message_ids", "activity_ids"} {
		if _, ok := metadata.Fields[fieldName]; !ok {
			t.Fatalf("delegation missing inherited field %s", fieldName)
		}
	}
	if metadata.Fields["approval_state_id"].Relation != workflow.ModelConfig {
		t.Fatalf("approval_state_id relation = %s", metadata.Fields["approval_state_id"].Relation)
	}
}

func TestDelegationAddonSecurity(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Users[10] = security.User{ID: 10, Login: "requester", Active: true, GroupIDs: []int64{GroupDelegationUser}}
	engine.Users[20] = security.User{ID: 20, Login: "manager", Active: true, GroupIDs: []int64{GroupDelegationManager}}
	engine.Users[30] = security.User{ID: 30, Login: "admin", Active: true, GroupIDs: []int64{GroupDelegationAdmin}}
	engine.Users[40] = security.User{ID: 40, Login: "none", Active: true}
	engine.Users[50] = security.User{ID: 50, Login: "source-employee", Active: true, GroupIDs: []int64{GroupDelegationEmployee}}
	engine.Users[60] = security.User{ID: 60, Login: "source-manager", Active: true, GroupIDs: []int64{GroupDelegationEmployeeManager}}
	engine.Users[70] = security.User{ID: 70, Login: "source-admin", Active: true, GroupIDs: []int64{GroupDelegationSourceAdmin}}
	engine.Users[80] = security.User{ID: 80, Login: "base-user", Active: true, GroupIDs: []int64{GroupBaseUser}}

	if len(SourceCompatibleACLs()) != 3 {
		t.Fatalf("source-compatible ACLs = %+v", SourceCompatibleACLs())
	}
	if len(SourceCompatibleRuleDefinitions()) != 3 {
		t.Fatalf("source-compatible rules = %+v", SourceCompatibleRuleDefinitions())
	}
	if !engine.EffectiveGroupIDs(30)[GroupDelegationUser] {
		t.Fatal("delegation admin does not imply delegation user")
	}
	if !engine.EffectiveGroupIDs(70)[GroupDelegationEmployee] {
		t.Fatal("source delegation admin does not imply source employee")
	}
	if err := CheckDelegationAccess(engine, 10, ModelDelegation, record.OpRead); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 10, ModelDelegation, record.OpWrite); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected user write denied by ACL, got %v", err)
	}
	if err := CheckDelegationAccess(engine, 20, ModelDelegation, record.OpCreate); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 30, ModelDelegation, record.OpUnlink); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 40, ModelDelegation, record.OpRead); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected no-group denied, got %v", err)
	}
	if err := CheckDelegationAccess(engine, 50, ModelDelegation, record.OpUnlink); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 60, ModelDelegationLine, record.OpCreate); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 70, ModelDelegation, record.OpUnlink); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 80, ModelDelegationLine, record.OpRead); err != nil {
		t.Fatal(err)
	}
	if err := CheckDelegationAccess(engine, 80, ModelDelegationLine, record.OpWrite); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	assertSecurityRulePerms(t, engine.Rules, "delegation_manager", true, false, false, false)
	assertSecurityRulePerms(t, engine.Rules, "delegation_admin", true, false, false, false)
	assertSourceDelegationRuleBehavior(t, engine, 50, 60, 70)

	ok, err := engine.AllowedByRecordRules(10, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(10), "delegator_user_id": int64(50)})
	if err != nil || !ok {
		t.Fatalf("own line denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(60), "delegator_user_id": int64(70)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("foreign line allowed")
	}
	ok, err = engine.AllowedByRecordRules(20, ModelDelegationLine, record.OpRead, map[string]any{"user_id": int64(60), "delegator_user_id": int64(70)})
	if err != nil || !ok {
		t.Fatalf("manager foreign line denied: %v %v", ok, err)
	}
}

func TestDelegationRuntimeSecurityAndMailDispatch(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := security.NewEngine()
	engine.SetNow(func() time.Time { return now })
	ApplySecurity(engine)
	engine.ACLs = append(engine.ACLs, security.ACL{Model: "purchase.order", GroupID: GroupDelegableRole, PermWrite: true})
	engine.Users[10] = security.User{ID: 10, Login: "owner", Email: "owner@example.com", Name: "Owner", Active: true, GroupIDs: []int64{GroupDelegableRole}}
	engine.Users[20] = security.User{ID: 20, Login: "delegate", Email: "delegate@example.com", Name: "Delegate", Active: true}

	outbox := mail.NewOutbox()
	templates := RuntimeMailTemplates()
	templates[MailTemplateDelegationAssigned] = mail.Template{ID: MailTemplateDelegationAssigned, To: "{{ email }}", Subject: "Assigned {{ request_name }}", Body: "{{ group_ids }} {{ source_model }}"}
	templates[MailTemplateDelegationRevoked] = mail.Template{ID: MailTemplateDelegationRevoked, To: "{{ email }}", Subject: "Revoked {{ request_name }}", Body: "{{ delegate_user_id }}"}
	templates[MailTemplateDelegationExpired] = mail.Template{ID: MailTemplateDelegationExpired, To: "{{ email }}", Subject: "Expired {{ request_name }}", Body: "{{ date_to }}"}
	svc := NewRuntimeService(engine, outbox, templates, SecurityMailRecipientResolver(engine), func() time.Time { return now })

	if err := engine.Check(record.Context{UserID: 20}, "purchase.order", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected inactive runtime delegation denied, got %v", err)
	}
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines:           []delegation.LineInput{{GroupID: GroupDelegableRole, DelegateUserID: 20}},
		SourceModel:     "purchase.order",
		SourceRecordID:  99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 20}, "purchase.order", record.OpWrite, nil); err != nil {
		t.Fatal(err)
	}
	if got := mailSubjects(outbox.List()); !reflect.DeepEqual(got, []string{"Assigned DEL/00001"}) {
		t.Fatalf("subjects after confirm = %+v", got)
	}
	ccTemplate := mail.Template{
		ID:                 99,
		To:                 "{{ email }}",
		CC:                 "existing@example.com",
		Subject:            "Business mail",
		Body:               "body",
		DelegationGroupIDs: []int64{GroupDelegableRole},
	}
	ccID, err := EnqueueTemplateWithDelegationCC(outbox, svc, ccTemplate, map[string]any{"email": "owner@example.com"}, delegation.MailContext{UserID: 10, Model: "purchase.order", RecordID: 99}, now)
	if err != nil {
		t.Fatal(err)
	}
	ccMessage, ok := outbox.Get(ccID)
	if !ok {
		t.Fatal("cc message missing")
	}
	if ccMessage.CC != "delegate@example.com, existing@example.com" {
		t.Fatalf("cc message = %+v", ccMessage)
	}

	if _, err := svc.Revoke(req.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 20}, "purchase.order", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected revoked runtime delegation denied, got %v", err)
	}
	if got := mailSubjects(outbox.List()); !reflect.DeepEqual(got, []string{"Assigned DEL/00001", "Business mail", "Revoked DEL/00001"}) {
		t.Fatalf("subjects after revoke = %+v", got)
	}

	expiring, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now,
		DelegatorUserID: 10,
		Lines:           []delegation.LineInput{{GroupID: GroupDelegableRole, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(expiring.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExpireDue(now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	engine.SetNow(func() time.Time { return now.AddDate(0, 0, 1) })
	if err := engine.Check(record.Context{UserID: 20}, "purchase.order", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected expired runtime delegation denied, got %v", err)
	}
	if got := mailSubjects(outbox.List()); !reflect.DeepEqual(got, []string{"Assigned DEL/00001", "Assigned DEL/00002", "Business mail", "Expired DEL/00002", "Revoked DEL/00001"}) {
		t.Fatalf("subjects after expiry = %+v", got)
	}
}

func TestDelegationAddonServiceDefaults(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	ConfigureService(svc)
	if _, ok := svc.GroupConfig(GroupDelegableRole); !ok {
		t.Fatal("missing default delegable group")
	}
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines:           []delegation.LineInput{{GroupID: GroupDelegableRole, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if got := svc.DelegatedGroupIDs(20, now); !reflect.DeepEqual(got, []int64{GroupDelegableRole}) {
		t.Fatalf("delegated groups = %+v", got)
	}
	if len(MailTemplates()) != 3 {
		t.Fatalf("mail templates = %+v", MailTemplates())
	}
}

func TestConfigureServiceFromPersistedGroupMetadata(t *testing.T) {
	env := delegationFixtureEnv(t)
	loader := delegationFixtureLoader(t, env)
	loadXMLFixture(t, loader, "security/group.xml")
	ids := loader.ExternalIDs()

	svc := delegation.NewService()
	if err := ConfigureServiceFromEnv(svc, env); err != nil {
		t.Fatal(err)
	}
	if svc.RestrictedAccess() {
		t.Fatal("restricted access enabled without config")
	}
	role, ok := svc.GroupConfig(ids[ModuleName+".group_delegable_role"].ResID)
	if !ok || !role.AllowDelegation || role.AllowMultipleDelegation || role.DisplayName != "Delegable Role" || role.RestrictedAccess {
		t.Fatalf("role config = %+v ok=%v", role, ok)
	}
	multiple, ok := svc.GroupConfig(ids[ModuleName+".group_delegable_multiple_role"].ResID)
	if !ok || !multiple.AllowDelegation || !multiple.AllowMultipleDelegation || multiple.RestrictedAccess {
		t.Fatalf("multiple config = %+v ok=%v", multiple, ok)
	}

	customID, err := env.Model("res.groups").Create(map[string]any{
		"name":              "User Type Delegable",
		"category_id":       ids["base.module_category_user_type"].ResID,
		"allow_delegation":  true,
		"restricted_access": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureServiceFromEnv(svc, env); err != nil {
		t.Fatal(err)
	}
	custom, ok := svc.GroupConfig(customID)
	if !ok || !custom.RestrictedAccess {
		t.Fatalf("user type config = %+v ok=%v", custom, ok)
	}
	rows, err := env.Model("res.groups").Browse(customID).Read("restricted_access")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["restricted_access"] != true {
		t.Fatalf("stored restricted access = %+v", rows[0])
	}

	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "restricted_access", "value": "True"}); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureServiceFromEnv(svc, env); err != nil {
		t.Fatal(err)
	}
	if !svc.RestrictedAccess() {
		t.Fatal("restricted access config did not enable service mode")
	}
}

func extensionMap() map[string]model.Model {
	out := map[string]model.Model{}
	for _, m := range ExtensionModels() {
		out[m.Name] = m
	}
	return out
}

func mailSubjects(messages []mail.Message) []string {
	subjects := make([]string, 0, len(messages))
	for _, message := range messages {
		subjects = append(subjects, message.Subject)
	}
	sort.Strings(subjects)
	return subjects
}

func assertManifestDataFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("manifest data file %s: %v", path, err)
	}
	switch filepath.Ext(path) {
	case ".xml":
		var doc fixtureXMLDoc
		if err := xml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if doc.XMLName.Local != "odoo" {
			t.Fatalf("%s root = %s", path, doc.XMLName.Local)
		}
		records := doc.AllRecords()
		if len(records) == 0 {
			t.Fatalf("%s has no records", path)
		}
		for _, record := range records {
			if record.ID == "" || record.Model == "" {
				t.Fatalf("%s has incomplete record %+v", path, record)
			}
			for _, field := range record.Fields {
				if field.Name == "" {
					t.Fatalf("%s has field without name in %+v", path, record)
				}
			}
		}
	case ".csv":
		reader := csv.NewReader(strings.NewReader(string(raw)))
		rows, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(rows) < 2 || len(rows[0]) == 0 || rows[0][0] != "id" {
			t.Fatalf("%s invalid csv shape", path)
		}
	default:
		t.Fatalf("unsupported manifest file %s", path)
	}
}

func loadXMLFixture(t *testing.T, loader *data.Loader, path string) {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadXML(file)
	closeErr := file.Close()
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func loadCSVFixture(t *testing.T, loader *data.Loader, modelName string, path string) {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadCSV(modelName, file)
	closeErr := file.Close()
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func delegationFixtureLoader(t *testing.T, env *record.Env) *data.Loader {
	t.Helper()
	externalIDs := map[string]data.ExternalID{}
	baseLoader := data.NewLoaderWithExternalIDs(env, "base", externalIDs)
	err := baseLoader.LoadXML(strings.NewReader(`<odoo>
  <record id="module_category_user_type" model="ir.module.category">
    <field name="name">User Type</field>
  </record>
  <record id="group_user" model="res.groups">
    <field name="name">Internal User</field>
    <field name="category_id" ref="module_category_user_type"/>
  </record>
  <record id="user_root" model="res.users">
    <field name="login">root</field>
    <field name="name">OdooBot</field>
    <field name="active" eval="True"/>
  </record>
  <record id="user_admin" model="res.users">
    <field name="login">admin</field>
    <field name="name">Administrator</field>
    <field name="active" eval="True"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	return data.NewLoaderWithExternalIDs(env, ModuleName, externalIDs)
}

func assertPersistedACL(t *testing.T, env *record.Env, accessID int64, modelID int64, groupID int64, read bool, write bool, create bool, unlink bool) {
	t.Helper()
	rows, err := env.Model("ir.model.access").Browse(accessID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["model_id"] != modelID || row["group_id"] != groupID ||
		row["perm_read"] != read || row["perm_write"] != write ||
		row["perm_create"] != create || row["perm_unlink"] != unlink {
		t.Fatalf("ACL %d = %+v", accessID, row)
	}
}

func assertPersistedRuleRawPerms(t *testing.T, env *record.Env, ruleID int64, write bool, create bool, unlink bool) {
	t.Helper()
	rows, err := env.Model("ir.rule").Browse(ruleID).Read("perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["perm_write"] != write || row["perm_create"] != create || row["perm_unlink"] != unlink {
		t.Fatalf("rule %d = %+v", ruleID, row)
	}
}

func assertSecurityRulePerms(t *testing.T, rules []security.Rule, name string, read bool, write bool, create bool, unlink bool) {
	t.Helper()
	for _, rule := range rules {
		if rule.Name != name {
			continue
		}
		if rule.PermRead != read || rule.PermWrite != write || rule.PermCreate != create || rule.PermUnlink != unlink {
			t.Fatalf("rule %s = %+v", name, rule)
		}
		return
	}
	t.Fatalf("missing rule %s in %+v", name, rules)
}

func assertSourceDelegationRuleBehavior(t *testing.T, engine *security.Engine, employeeID int64, managerID int64, adminID int64) {
	t.Helper()
	own := map[string]any{"user_id": employeeID}
	managed := map[string]any{
		"user_id": int64(999),
		"employee_id": map[string]any{
			"parent_id": map[string]any{"user_id": managerID},
		},
	}
	foreign := map[string]any{
		"user_id": int64(999),
		"employee_id": map[string]any{
			"parent_id": map[string]any{"user_id": int64(404)},
		},
	}
	adminAny := map[string]any{"user_id": int64(999)}
	assertRecordRuleAllows(t, engine, employeeID, ModelDelegation, record.OpWrite, own)
	assertRecordRuleAllows(t, engine, managerID, ModelDelegation, record.OpRead, managed)
	assertRecordRuleDenies(t, engine, managerID, ModelDelegation, record.OpRead, foreign)
	assertRecordRuleDenies(t, engine, managerID, ModelDelegation, record.OpWrite, managed)
	assertRecordRuleAllows(t, engine, adminID, ModelDelegation, record.OpRead, adminAny)
	assertRecordRuleDenies(t, engine, adminID, ModelDelegation, record.OpWrite, adminAny)
}

func assertRecordRuleAllows(t *testing.T, engine *security.Engine, userID int64, modelName string, op record.Operation, row map[string]any) {
	t.Helper()
	ok, err := engine.AllowedByRecordRules(userID, modelName, op, row)
	if err != nil || !ok {
		t.Fatalf("record rule allow failed: user=%d model=%s op=%s ok=%v err=%v row=%+v", userID, modelName, op, ok, err, row)
	}
}

func assertRecordRuleDenies(t *testing.T, engine *security.Engine, userID int64, modelName string, op record.Operation, row map[string]any) {
	t.Helper()
	ok, err := engine.AllowedByRecordRules(userID, modelName, op, row)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("record rule allowed unexpectedly: user=%d model=%s op=%s row=%+v", userID, modelName, op, row)
	}
}

func assertGroupImplies(t *testing.T, env *record.Env, groupID int64, want []int64) {
	t.Helper()
	rows, err := env.Model("res.groups").Browse(groupID).Read("implied_ids")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(int64Slice(rows[0]["implied_ids"]), want) {
		t.Fatalf("group %d implied_ids = %+v, want %+v", groupID, rows[0]["implied_ids"], want)
	}
}

func assertGroupUsers(t *testing.T, env *record.Env, groupID int64, want []int64) {
	t.Helper()
	rows, err := env.Model("res.groups").Browse(groupID).Read("user_ids")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range want {
		if !containsInt64(rows[0]["user_ids"], id) {
			t.Fatalf("group %d user_ids missing %d: %+v", groupID, id, rows[0])
		}
	}
}

func assertGroupCategory(t *testing.T, env *record.Env, groupID int64, want int64) {
	t.Helper()
	rows, err := env.Model("res.groups").Browse(groupID).Read("category_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["category_id"] != want {
		t.Fatalf("group %d category_id = %+v, want %d", groupID, rows[0]["category_id"], want)
	}
}

func loadPersistedGroups(t *testing.T, env *record.Env, engine *security.Engine) {
	t.Helper()
	found, err := env.Model("res.groups").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("name", "implied_ids")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		id, _ := row["id"].(int64)
		engine.Groups[id] = security.Group{
			ID:         id,
			Name:       row["name"].(string),
			ImpliedIDs: int64Slice(row["implied_ids"]),
		}
	}
}

func int64Slice(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64{}, typed...)
	case nil:
		return nil
	default:
		return nil
	}
}

func containsInt64(value any, target int64) bool {
	for _, id := range int64Slice(value) {
		if id == target {
			return true
		}
	}
	return false
}

func delegationFixtureEnv(t *testing.T) *record.Env {
	t.Helper()
	models := map[string]model.Model{}
	order := []string{}
	add := func(m model.Model) {
		if existing, ok := models[m.Name]; ok {
			models[m.Name] = m.Compose(existing)
			return
		}
		models[m.Name] = m
		order = append(order, m.Name)
	}
	for _, m := range base.Models() {
		add(m)
	}
	for _, m := range workflow.Models() {
		add(m)
	}
	for _, m := range hr.Models() {
		add(m)
	}
	for _, m := range Models() {
		add(m)
	}
	for _, m := range ExtensionModels() {
		add(m)
	}

	reg := record.NewRegistry()
	for _, name := range order {
		if err := reg.Register(models[name]); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

type fixtureXMLDoc struct {
	XMLName xml.Name           `xml:"odoo"`
	Records []fixtureXMLRecord `xml:"record"`
	Data    []fixtureXMLData   `xml:"data"`
}

func (d fixtureXMLDoc) AllRecords() []fixtureXMLRecord {
	records := append([]fixtureXMLRecord{}, d.Records...)
	for _, data := range d.Data {
		records = append(records, data.Records...)
	}
	return records
}

type fixtureXMLData struct {
	Records []fixtureXMLRecord `xml:"record"`
}

type fixtureXMLRecord struct {
	ID     string            `xml:"id,attr"`
	Model  string            `xml:"model,attr"`
	Fields []fixtureXMLField `xml:"field"`
}

type fixtureXMLField struct {
	Name string `xml:"name,attr"`
}
