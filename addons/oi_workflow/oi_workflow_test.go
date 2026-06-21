package oi_workflow

import (
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorp/addons/oi_base"
	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	"gorp/internal/workflow"
)

func TestWorkflowCoreManifestAndModels(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || !manifest.AutoInstall || manifest.Application {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"mail", oi_base.ModuleName, "oi_base_cache", "web", "base_automation", "oi_web_selection_field_dynamic", "oi_web_selection_tags", "automation"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	for _, path := range []string{
		"data/approval_record_templates.xml",
		"view/approval_settings.xml",
		"view/approval_buttons.xml",
		"view/approval_log.xml",
		"view/cancellation_record_view.xml",
		"view/action.xml",
		"view/menu.xml",
		"view/templates.xml",
		"views/approval_record_templates.xml",
	} {
		assertManifestHasData(t, manifest.Data, path)
	}
	wantAssets := []string{
		"frontend/packages/oi-workflow/src/index.ts",
		"static/src/js/chatter.js",
		"static/src/js/debug_items.js",
		"static/src/js/form_controller.js",
		"static/src/js/list_controller.js",
		"static/src/js/statusbar_duration_state_field.js",
		"static/src/js/statusbar_field.js",
		"static/src/js/utils.js",
		"static/src/js/view_button.js",
		"static/src/xml/approval_user_info.xml",
		"static/src/xml/view_button.xml",
		"static/src/scss/statusbar_duration_field.scss",
	}
	if !reflect.DeepEqual(manifest.Assets["web.assets_backend"], wantAssets) {
		t.Fatalf("backend assets = %+v", manifest.Assets["web.assets_backend"])
	}
	for _, asset := range wantAssets[1:] {
		assertWorkflowAssetFile(t, asset)
	}
	reg := registry.New("test")
	manifests := []module.Manifest{base.Manifest()}
	manifests = append(manifests, DependencyManifests()...)
	manifests = append(manifests, manifest)
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	if reg.States[oi_base.ModuleName] != "installed" || reg.States[ModuleName] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		workflow.ModelSettings,
		workflow.ModelSettingsState,
		workflow.ModelButton,
		workflow.ModelAutomation,
		workflow.ModelEscalation,
		workflow.ModelLog,
		workflow.ModelLogVoting,
		workflow.ModelForward,
		workflow.ModelCancellation,
		workflow.ModelStateTags,
		workflow.ModelProcessWizard,
		workflow.ModelExpressionEditor,
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	extensions := modelMap(ExtensionModels())
	settingsExtension := extensions["res.config.settings"]
	if !settingsExtension.Transient || !reflect.DeepEqual(settingsExtension.Inherit, []string{"res.config.settings"}) {
		t.Fatalf("res.config.settings extension = %+v", settingsExtension)
	}
	for _, fieldName := range workflowSettingsToggleFields() {
		if _, ok := settingsExtension.Fields[fieldName]; !ok {
			t.Fatalf("res.config.settings missing workflow toggle %s", fieldName)
		}
	}
	button := reg.Models[workflow.ModelButton]
	for _, fieldName := range []string{"action_type", "visible_domain", "server_action_id", "email_template_id", "email_wizard_form_id", "email_next_action", "comment", "context", "icon", "hotkey", "validate_form", "voting_type", "vote_threshold"} {
		if _, ok := button.Fields[fieldName]; !ok {
			t.Fatalf("approval.buttons missing field %s", fieldName)
		}
	}
	approvalRecord := reg.Models[workflow.ModelApprovalRecord]
	for _, fieldName := range []string{"record_cancellation_count", "active_record_cancellation_count", "approval_activity_date_deadline", "duration_state_tracking", "approval_voting_ids", "approval_visible_button_ids"} {
		if _, ok := approvalRecord.Fields[fieldName]; !ok {
			t.Fatalf("approval.record missing field %s", fieldName)
		}
	}
}

func TestListControllerStaticActionKeysMatchOISource(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("static/src/js/list_controller.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "approve_log:") {
		t.Fatalf("list controller missing source approve_log key: %s", text)
	}
	if strings.Contains(text, "approval_log:") {
		t.Fatalf("list controller contains non-source approval_log key: %s", text)
	}
}

func TestWorkflowManifestDataFilesLoad(t *testing.T) {
	manifest := Manifest()
	for _, path := range manifest.Data {
		assertManifestDataFile(t, path)
	}

	env := workflowFixtureEnv(t)
	loader := workflowFixtureLoader(t, env)
	loadManifestDataFiles(t, loader, manifest.Data)

	ids := loader.ExternalIDs()
	for _, name := range []string{
		"group_workflow_admin",
		"seq_cancellation_record",
		"activity_type_approval",
		"cron_update_approval_activity",
		"approval_settings_cancellation_record",
		"mail_template_approval_request",
		"mail_template_approval_complete",
		"view_approval_config_form",
		"view_approval_automation_list",
		"view_approval_escalation_form",
		"view_approval_process_wizard_form",
		"view_approval_state_update_form",
		"view_model_expression_editor_form",
		"view_workflow_templates",
		"view_approval_record_template_form",
		"res_config_settings_view_form",
		"act_approval_settings",
		"act_approval_logs",
		"menu_approval_root",
		"menu_approval_settings",
	} {
		if ids[ModuleName+"."+name].ResID == 0 {
			t.Fatalf("missing fixture external id %s in %+v", name, ids)
		}
	}
	for _, name := range []string{"base.group_user", "base.group_system"} {
		if ids[name].ResID == 0 {
			t.Fatalf("missing fixture external id %s in %+v", name, ids)
		}
	}
	rows, err := env.Model("ir.model.access").Browse(ids[ModuleName+".access_approval_settings_admin"].ResID).Read("model_id", "group_id", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["perm_unlink"] != true || rows[0]["model_id"] == nil || rows[0]["group_id"] == nil {
		t.Fatalf("unexpected ACL row: %+v", rows[0])
	}
	accessRows, err := env.Model("ir.model.access").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if accessRows.Len() < 50 {
		t.Fatalf("ACL count = %d, want at least 50", accessRows.Len())
	}
	assertPersistedACL(t, env, ids[ModuleName+".access_approval_settings_system"].ResID, ids[ModuleName+".model_approval_settings"].ResID, ids["base.group_system"].ResID, true, true, true, true)
	assertPersistedACL(t, env, ids[ModuleName+".access_approval_config_user"].ResID, ids[ModuleName+".model_approval_config"].ResID, ids["base.group_user"].ResID, true, false, false, false)
	assertPersistedACL(t, env, ids[ModuleName+".access_approval_process_wizard_user"].ResID, ids[ModuleName+".model_approval_process_wizard"].ResID, ids["base.group_user"].ResID, true, true, true, true)
	sequenceRows, err := env.Model("ir.sequence").Browse(ids[ModuleName+".seq_cancellation_record"].ResID).Read("code", "padding")
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRows[0]["code"] != "cancellation.record" || sequenceRows[0]["padding"] != int64(4) {
		t.Fatalf("sequence = %+v", sequenceRows[0])
	}
	activityRows, err := env.Model("mail.activity.type").Browse(ids[ModuleName+".activity_type_approval"].ResID).Read("name", "icon", "active")
	if err != nil {
		t.Fatal(err)
	}
	if activityRows[0]["name"] != "Approval" || activityRows[0]["icon"] != "fa-check" || activityRows[0]["active"] != true {
		t.Fatalf("activity type = %+v", activityRows[0])
	}
	cronRows, err := env.Model("ir.cron").Browse(ids[ModuleName+".cron_update_approval_activity"].ResID).Read("model_id", "state", "code", "active", "interval_number", "interval_type")
	if err != nil {
		t.Fatal(err)
	}
	if cronRows[0]["model_id"] != ids[ModuleName+".model_approval_settings"].ResID ||
		cronRows[0]["state"] != "code" ||
		cronRows[0]["code"] != "model.update_pending_activities()" ||
		cronRows[0]["active"] != true ||
		cronRows[0]["interval_number"] != int64(1) ||
		cronRows[0]["interval_type"] != "days" {
		t.Fatalf("cron = %+v", cronRows[0])
	}
	settingsRows, err := env.Model(workflow.ModelSettings).Browse(ids[ModuleName+".approval_settings_cancellation_record"].ResID).Read("model", "model_id", "sequence", "state_field", "active")
	if err != nil {
		t.Fatal(err)
	}
	if settingsRows[0]["model"] != "cancellation.record" ||
		settingsRows[0]["model_id"] != ids[ModuleName+".model_cancellation_record"].ResID ||
		settingsRows[0]["sequence"] != int64(10) ||
		settingsRows[0]["state_field"] != "state" ||
		settingsRows[0]["active"] != true {
		t.Fatalf("approval settings = %+v", settingsRows[0])
	}
	templateRows, err := env.Model("mail.template").Browse(ids[ModuleName+".mail_template_approval_request"].ResID).Read("model", "subject", "active")
	if err != nil {
		t.Fatal(err)
	}
	if templateRows[0]["model"] != workflow.ModelApprovalRecord || templateRows[0]["subject"] != "Approval requested" || templateRows[0]["active"] != true {
		t.Fatalf("template = %+v", templateRows[0])
	}
	actionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".act_approval_settings"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if actionRows[0]["res_model"] != workflow.ModelSettings || actionRows[0]["view_mode"] != "list,form" {
		t.Fatalf("action = %+v", actionRows[0])
	}
	menuRows, err := env.Model("ir.ui.menu").Browse(ids[ModuleName+".menu_approval_settings"].ResID).Read("parent_id", "action")
	if err != nil {
		t.Fatal(err)
	}
	if menuRows[0]["parent_id"] != ids[ModuleName+".menu_approval_operations"].ResID ||
		menuRows[0]["action"] != "ir.actions.act_window,act_approval_settings" {
		t.Fatalf("menu = %+v", menuRows[0])
	}
	viewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_workflow_templates"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := viewRows[0]["arch"].(string)
	if viewRows[0]["model"] != workflow.ModelApprovalRecord || !strings.Contains(arch, `t-name="oi_workflow.ApprovalStateBadge"`) {
		t.Fatalf("template view = %+v", viewRows[0])
	}
	recordTemplateRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_approval_record_template_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	recordTemplateArch, _ := recordTemplateRows[0]["arch"].(string)
	if recordTemplateRows[0]["model"] != workflow.ModelApprovalRecord || !strings.Contains(recordTemplateArch, `field name="user_can_approve"`) {
		t.Fatalf("approval record template view = %+v", recordTemplateRows[0])
	}
	settingsViewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".res_config_settings_view_form"].ResID).Read("model", "priority", "inherit_id", "arch")
	if err != nil {
		t.Fatal(err)
	}
	settingsArch, _ := settingsViewRows[0]["arch"].(string)
	if settingsViewRows[0]["model"] != "res.config.settings" ||
		settingsViewRows[0]["priority"] != int64(100) ||
		settingsViewRows[0]["inherit_id"] != ids["base.res_config_settings_view_form"].ResID ||
		!strings.Contains(settingsArch, `<app name="oi_workflow"`) ||
		!strings.Contains(settingsArch, `name="oi_workflow_setting_container"`) {
		t.Fatalf("workflow settings view = %+v", settingsViewRows[0])
	}
	for _, fieldName := range workflowSettingsToggleFields() {
		if !strings.Contains(settingsArch, `field name="`+fieldName+`"`) {
			t.Fatalf("workflow settings view missing %s: %s", fieldName, settingsArch)
		}
	}
	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	engine.Users[10] = security.User{ID: 10, Login: "user", Active: true, GroupIDs: []int64{ids[ModuleName+".group_workflow_user"].ResID}}
	engine.Users[20] = security.User{ID: 20, Login: "manager", Active: true, GroupIDs: []int64{ids[ModuleName+".group_workflow_manager"].ResID}}
	engine.Users[30] = security.User{ID: 30, Login: "admin", Active: true, GroupIDs: []int64{ids[ModuleName+".group_workflow_admin"].ResID}}
	engine.Users[40] = security.User{ID: 40, Login: "base-user", Active: true, GroupIDs: []int64{ids["base.group_user"].ResID}}
	engine.Users[50] = security.User{ID: 50, Login: "base-system", Active: true, GroupIDs: []int64{ids["base.group_system"].ResID}}
	if err := engine.Check(record.Context{UserID: 10}, workflow.ModelSettings, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, workflow.ModelSettings, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected fixture user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, workflow.ModelSettings, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, workflow.ModelSettings, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelConfig, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelConfig, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelProcessWizard, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 50}, workflow.ModelSettings, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowTransitions(t *testing.T) {
	engine := workflow.NewEngine()
	settingsID := engine.AddSettings(workflow.Settings{Name: "Move Workflow", Model: "account.move", Active: true})
	approveID := engine.AddButton(workflow.Button{SettingsID: settingsID, StateValue: "draft", Name: "Approve", Action: workflow.ActionApprove, GroupIDs: []int64{GroupWorkflowManager}})
	rejectID := engine.AddButton(workflow.Button{SettingsID: settingsID, StateValue: "approved", Name: "Reject", Action: workflow.ActionReject, CommentRequired: true})
	record := workflow.Record{Model: "account.move", ID: 10, State: "draft", Values: map[string]any{"amount_total": 50}}
	if _, err := engine.RunButton(context.Background(), workflow.User{ID: 1, GroupIDs: []int64{GroupWorkflowUser}}, &record, approveID, workflow.Input{}); !errors.Is(err, workflow.ErrButtonHidden) {
		t.Fatalf("expected hidden approve, got %v", err)
	}
	if _, err := engine.RunButton(context.Background(), workflow.User{ID: 2, GroupIDs: []int64{GroupWorkflowManager}}, &record, approveID, workflow.Input{}); err != nil {
		t.Fatal(err)
	}
	if record.State != "approved" || len(engine.Logs) != 1 {
		t.Fatalf("record=%+v logs=%+v", record, engine.Logs)
	}
	if _, err := engine.RunButton(context.Background(), workflow.User{ID: 2, GroupIDs: []int64{GroupWorkflowManager}}, &record, rejectID, workflow.Input{}); !errors.Is(err, workflow.ErrCommentRequired) {
		t.Fatalf("expected comment required, got %v", err)
	}
	if _, err := engine.RunButton(context.Background(), workflow.User{ID: 2, GroupIDs: []int64{GroupWorkflowManager}}, &record, rejectID, workflow.Input{Comment: "no"}); err != nil {
		t.Fatal(err)
	}
	if record.State != "rejected" || engine.Logs[1].OldState != "approved" {
		t.Fatalf("record=%+v logs=%+v", record, engine.Logs)
	}
}

func loadManifestDataFiles(t *testing.T, loader *data.Loader, paths []string) {
	t.Helper()
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		switch filepath.Ext(path) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			modelName := strings.TrimSuffix(filepath.Base(path), ".csv")
			err = loader.LoadCSV(modelName, file)
		default:
			err = nil
		}
		closeErr := file.Close()
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", path, closeErr)
		}
	}
}

func assertManifestDataFile(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("manifest data file %s: %v", path, err)
	}
	switch filepath.Ext(path) {
	case ".xml":
		var doc struct {
			XMLName xml.Name `xml:"odoo"`
		}
		if err := xml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if doc.XMLName.Local != "odoo" {
			t.Fatalf("%s root = %s", path, doc.XMLName.Local)
		}
	case ".csv":
		reader := csv.NewReader(strings.NewReader(string(data)))
		rows, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(rows) < 2 {
			t.Fatalf("%s has no records", path)
		}
	}
}

func assertManifestHasData(t *testing.T, paths []string, want string) {
	t.Helper()
	for _, path := range paths {
		if path == want {
			return
		}
	}
	t.Fatalf("manifest data missing %s in %+v", want, paths)
}

func modelMap(models []model.Model) map[string]model.Model {
	out := map[string]model.Model{}
	for _, m := range models {
		out[m.Name] = m
	}
	return out
}

func workflowSettingsToggleFields() []string {
	return []string{
		"module_oi_workflow_expense",
		"module_oi_workflow_hr_contract",
		"module_oi_workflow_hr_holidays",
		"module_oi_workflow_hr_holidays_manager",
		"module_oi_workflow_hr_payslip_run",
		"module_oi_workflow_hr_payslip_run_e",
		"module_oi_workflow_purchase_order",
		"module_oi_workflow_purchase_requisition",
		"module_oi_workflow_sale_order",
		"module_oi_workflow_account_payment",
		"module_oi_workflow_crm_lead",
		"module_oi_workflow_invoice",
		"module_oi_workflow_project",
		"module_oi_workflow_project_task",
	}
}

func assertWorkflowAssetFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("asset %s: %v", path, err)
	}
	text := string(raw)
	switch filepath.Ext(path) {
	case ".js":
		if !strings.Contains(text, "@odoo-module") || (!strings.Contains(text, "Workflow") && !strings.Contains(text, "workflow") && !strings.Contains(text, "approval")) {
			t.Fatalf("asset %s has unexpected js content", path)
		}
	case ".xml":
		var doc struct {
			XMLName   xml.Name `xml:"templates"`
			Templates []struct {
				Name string `xml:"t-name,attr"`
			} `xml:"t"`
		}
		if err := xml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse asset %s: %v", path, err)
		}
		if doc.XMLName.Local != "templates" || len(doc.Templates) == 0 {
			t.Fatalf("asset %s has no templates", path)
		}
	case ".scss":
		if !strings.Contains(text, ".oi-workflow") {
			t.Fatalf("asset %s missing workflow selector", path)
		}
	default:
		t.Fatalf("unsupported asset %s", path)
	}
}

func workflowFixtureEnv(t *testing.T) *record.Env {
	t.Helper()
	models := map[string]model.Model{}
	var order []string
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
	for _, m := range workflow.ExtensionModels() {
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

func workflowFixtureLoader(t *testing.T, env *record.Env) *data.Loader {
	t.Helper()
	externalIDs := map[string]data.ExternalID{}
	baseLoader := data.NewLoaderWithExternalIDs(env, "base", externalIDs)
	err := baseLoader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_user" model="res.groups">
    <field name="name">Internal User</field>
  </record>
  <record id="group_system" model="res.groups">
    <field name="name">Settings</field>
    <field name="implied_ids" eval="[(4, ref('group_user'))]"/>
  </record>
  <record id="res_config_settings_view_form" model="ir.ui.view">
    <field name="name">res.config.settings.view.form</field>
    <field name="model">res.config.settings</field>
    <field name="type">form</field>
    <field name="arch" type="xml"><form/></field>
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

func TestWorkflowSecurity(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Users[10] = security.User{ID: 10, Login: "user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{GroupWorkflowUser}}
	engine.Users[20] = security.User{ID: 20, Login: "manager", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{GroupWorkflowManager}}
	engine.Users[30] = security.User{ID: 30, Login: "admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupWorkflowAdmin}}
	engine.Users[40] = security.User{ID: 40, Login: "base-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{GroupBaseUser}}
	engine.Users[50] = security.User{ID: 50, Login: "base-system", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupBaseSystem}}

	if !engine.EffectiveGroupIDs(30)[GroupWorkflowUser] {
		t.Fatal("workflow admin does not imply user")
	}
	if !engine.EffectiveGroupIDs(50)[GroupBaseUser] {
		t.Fatal("base system does not imply base user")
	}
	if len(SourceCompatibleACLs()) != 17 {
		t.Fatalf("source-compatible ACLs = %+v", SourceCompatibleACLs())
	}
	if err := engine.Check(record.Context{UserID: 10}, workflow.ModelSettings, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, workflow.ModelSettings, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, workflow.ModelSettings, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, workflow.ModelSettings, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelConfig, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelConfig, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 40}, workflow.ModelProcessWizard, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 50}, workflow.ModelSettings, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if !CanApprove(engine, 20, workflow.ModelLog, map[string]any{"company_id": int64(1)}) {
		t.Fatal("manager should approve same-company workflow log")
	}
	if CanApprove(engine, 20, workflow.ModelLog, map[string]any{"company_id": int64(2)}) {
		t.Fatal("manager should not approve other-company workflow log")
	}

	foundCompanyRule := false
	for _, definition := range SecurityRuleDefinitions() {
		if definition.Rule.Model == workflow.ModelLog && definition.Rule.Domain.Kind == domain.Any {
			foundCompanyRule = true
			break
		}
	}
	if !foundCompanyRule {
		t.Fatal("missing workflow company rule")
	}
}
