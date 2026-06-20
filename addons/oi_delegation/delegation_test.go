package oi_delegation

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
		ModuleName + ".view_oi_delegation_form",
		ModuleName + ".view_approval_log_delegation_list",
		ModuleName + ".action_delegation",
		ModuleName + ".action_delegation_lines",
		ModuleName + ".menu_oi_delegation_root",
		ModuleName + ".menu_oi_delegation_requests",
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
	actionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".action_delegation"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if actionRows[0]["res_model"] != ModelDelegation || actionRows[0]["view_mode"] != "tree,form" {
		t.Fatalf("action = %+v", actionRows[0])
	}
	lineActionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".action_delegation_lines"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if lineActionRows[0]["res_model"] != ModelDelegationLine || lineActionRows[0]["view_mode"] != "tree,form" {
		t.Fatalf("line action = %+v", lineActionRows[0])
	}
	viewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_oi_delegation_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := viewRows[0]["arch"].(string)
	if viewRows[0]["model"] != ModelDelegation || !strings.Contains(arch, `field name="delegateTo_employee_id"`) || !strings.Contains(arch, `field name="department_ids"`) {
		t.Fatalf("view = %+v", viewRows[0])
	}
	menuRows, err := env.Model("ir.ui.menu").Browse(ids[ModuleName+".menu_oi_delegation_requests"].ResID).Read("parent_id", "action")
	if err != nil {
		t.Fatal(err)
	}
	if menuRows[0]["parent_id"] != ids[ModuleName+".menu_oi_delegation_root"].ResID ||
		menuRows[0]["action"] != "ir.actions.act_window,action_delegation" {
		t.Fatalf("menu = %+v", menuRows[0])
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
