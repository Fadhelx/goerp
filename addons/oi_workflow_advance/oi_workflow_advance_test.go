package oi_workflow_advance

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorp/addons/oi_workflow"
	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	internalworkflow "gorp/internal/workflow"
)

var manifestData = []string{
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
}

func TestManifestInstall(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || manifest.Application {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{WorkflowDependencyName, FlowchartDependencyName}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if !reflect.DeepEqual(manifest.Data, manifestData) {
		t.Fatalf("data = %+v", manifest.Data)
	}
	wantAssets := []string{
		"static/src/js/list_controller.js",
		"static/src/xml/templates.xml",
	}
	if !reflect.DeepEqual(manifest.Assets["web.assets_backend"], wantAssets) {
		t.Fatalf("backend assets = %+v", manifest.Assets["web.assets_backend"])
	}
	for _, asset := range wantAssets {
		assertWorkflowAdvanceAssetFile(t, asset)
	}

	reg := registry.New("test")
	manifests := []module.Manifest{base.Manifest()}
	manifests = append(manifests, DependencyManifests()...)
	manifests = append(manifests, manifest)
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	if reg.States[ModuleName] != "installed" || reg.States[WorkflowDependencyName] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		internalworkflow.ModelWorkflow,
		internalworkflow.ModelNode,
		internalworkflow.ModelTransition,
		internalworkflow.ModelNodeAction,
		internalworkflow.ModelProcess,
		internalworkflow.ModelWorkflowWizard,
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	if len(ApprovalLogExtensionFields()) != 3 {
		t.Fatalf("approval log extension fields = %+v", ApprovalLogExtensionFields())
	}
	extensions := extensionMap()
	for _, fieldName := range []string{"workflow_process_ids", "workflow_id", "workflow_node_id", "workflow_transition_ids", "workflow_view_id", "_workflow_transition_id"} {
		if _, ok := extensions[internalworkflow.ModelApprovalRecord].Fields[fieldName]; !ok {
			t.Fatalf("approval.record extension missing field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"workflow_ids", "workflow_count"} {
		if _, ok := extensions["approval.settings"].Fields[fieldName]; !ok {
			t.Fatalf("approval.settings extension missing field %s", fieldName)
		}
	}
	if _, ok := extensions[internalworkflow.ModelForward].Fields["workflow_node_id"]; !ok {
		t.Fatal("approval.forward extension missing workflow_node_id")
	}
	if _, ok := extensions["approval.config"].Fields["workflow_advanced"]; !ok {
		t.Fatal("approval.config extension missing workflow_advanced")
	}
}

func TestRegisterRecordModels(t *testing.T) {
	reg := record.NewRegistry()
	if err := RegisterRecordModels(reg); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Model(internalworkflow.ModelTransition); !ok {
		t.Fatalf("missing record model %s", internalworkflow.ModelTransition)
	}
}

func TestManifestFixtureFilesLoad(t *testing.T) {
	manifest := Manifest()
	for _, path := range manifest.Data {
		assertManifestDataFile(t, path)
	}

	env, ids := loadManifestFixtures(t)
	for _, name := range []string{
		"base.group_user",
		"base.group_system",
		"oi_workflow.group_workflow_user",
		"oi_workflow.group_workflow_manager",
		"oi_workflow.group_workflow_admin",
		ModuleName + ".group_workflow_advance_user",
		ModuleName + ".group_workflow_advance_manager",
		ModuleName + ".group_workflow_advance_admin",
		ModuleName + ".model_workflow",
		ModuleName + ".access_workflow_user",
		ModuleName + ".access_workflow_group_user",
		ModuleName + ".access_workflow_group_system",
		ModuleName + ".rule_workflow_company",
		ModuleName + ".view_workflow_workflow_form",
		ModuleName + ".view_workflow_process_wizard_form",
		ModuleName + ".act_workflow_advance_workflows",
		ModuleName + ".act_workflow_advance_flowchart",
		ModuleName + ".cron_process_escalation",
	} {
		if ids[name].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", name, ids)
		}
	}

	accessRows, err := env.Model("ir.model.access").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if want := len(ModelNames())*3 + 11; accessRows.Len() != want {
		t.Fatalf("ACL count = %d, want %d", accessRows.Len(), want)
	}
	assertPersistedACL(t, env, ids[ModuleName+".access_workflow_group_user"].ResID, ids[ModuleName+".model_workflow"].ResID, ids["base.group_user"].ResID, true, false, false, false)
	assertPersistedACL(t, env, ids[ModuleName+".access_workflow_group_system"].ResID, ids[ModuleName+".model_workflow"].ResID, ids["base.group_system"].ResID, true, true, true, true)
	assertPersistedACL(t, env, ids[ModuleName+".access_workflow_process_wizard"].ResID, ids[ModuleName+".model_workflow_process_wizard"].ResID, ids["base.group_user"].ResID, true, true, true, true)

	actionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".act_workflow_advance_workflows"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if actionRows[0]["res_model"] != internalworkflow.ModelWorkflow || actionRows[0]["view_mode"] != "list,form" {
		t.Fatalf("action = %+v", actionRows[0])
	}
	flowchartActionRows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".act_workflow_advance_flowchart"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if flowchartActionRows[0]["res_model"] != internalworkflow.ModelWorkflow || flowchartActionRows[0]["view_mode"] != "form" {
		t.Fatalf("flowchart action = %+v", flowchartActionRows[0])
	}
	viewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_workflow_workflow_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := viewRows[0]["arch"].(string)
	if viewRows[0]["model"] != internalworkflow.ModelWorkflow ||
		!strings.Contains(arch, `field name="start_node_id"`) ||
		!strings.Contains(arch, `field name="flowchart" widget="flowchart"`) {
		t.Fatalf("view = %+v", viewRows[0])
	}
	wizardViewRows, err := env.Model("ir.ui.view").Browse(ids[ModuleName+".view_workflow_process_wizard_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	wizardArch, _ := wizardViewRows[0]["arch"].(string)
	if wizardViewRows[0]["model"] != internalworkflow.ModelWorkflowWizard ||
		!strings.Contains(wizardArch, `field name="workflow_transition_ids"`) ||
		!strings.Contains(wizardArch, `domain="[('id','in',workflow_transition_ids)]"`) ||
		!strings.Contains(wizardArch, `button name="process" type="object"`) {
		t.Fatalf("wizard view = %+v", wizardViewRows[0])
	}
	cronRows, err := env.Model("ir.cron").Browse(ids[ModuleName+".cron_process_escalation"].ResID).Read("interval_type", "action_name", "active", "state", "code", "model_id")
	if err != nil {
		t.Fatal(err)
	}
	if cronRows[0]["interval_type"] != "hours" ||
		cronRows[0]["action_name"] != "workflow.process.escalation" ||
		cronRows[0]["state"] != "code" ||
		cronRows[0]["code"] != "model._process_escalation()" ||
		cronRows[0]["model_id"] == int64(0) ||
		cronRows[0]["active"] != true {
		t.Fatalf("cron = %+v", cronRows[0])
	}

	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	loadPersistedGroups(t, env, engine)
	userGroup := ids[ModuleName+".group_workflow_advance_user"].ResID
	managerGroup := ids[ModuleName+".group_workflow_advance_manager"].ResID
	adminGroup := ids[ModuleName+".group_workflow_advance_admin"].ResID
	engine.Users[10] = security.User{ID: 10, Login: "fixture-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{userGroup}}
	engine.Users[20] = security.User{ID: 20, Login: "fixture-manager", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{managerGroup}}
	engine.Users[30] = security.User{ID: 30, Login: "fixture-admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{adminGroup}}
	engine.Users[40] = security.User{ID: 40, Login: "fixture-none", Active: true, CompanyID: 1, CompanyIDs: []int64{1}}
	engine.Users[50] = security.User{ID: 50, Login: "base-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{ids["base.group_user"].ResID}}
	engine.Users[60] = security.User{ID: 60, Login: "base-system", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{ids["base.group_system"].ResID}}
	if !engine.EffectiveGroupIDs(10)[ids["oi_workflow.group_workflow_user"].ResID] {
		t.Fatal("workflow advance user does not imply workflow user")
	}
	if !engine.EffectiveGroupIDs(20)[ids["oi_workflow.group_workflow_manager"].ResID] {
		t.Fatal("workflow advance manager does not imply workflow manager")
	}
	if !engine.EffectiveGroupIDs(30)[ids["oi_workflow.group_workflow_admin"].ResID] {
		t.Fatal("workflow advance admin does not imply workflow admin")
	}
	if err := engine.Check(record.Context{UserID: 10}, internalworkflow.ModelWorkflow, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, internalworkflow.ModelWorkflow, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected fixture user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, internalworkflow.ModelNode, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, internalworkflow.ModelTransition, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, internalworkflow.ModelWorkflow, record.OpRead, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected no-group read denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 50}, internalworkflow.ModelWorkflow, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 50}, internalworkflow.ModelWorkflow, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 50}, internalworkflow.ModelWorkflowWizard, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 60}, internalworkflow.ModelWorkflow, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	ok, err := engine.AllowedByRecordRules(10, internalworkflow.ModelWorkflow, record.OpRead, map[string]any{"company_id": int64(1)})
	if err != nil || !ok {
		t.Fatalf("same-company workflow denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, internalworkflow.ModelWorkflow, record.OpRead, map[string]any{"company_id": int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("other-company workflow allowed")
	}
	ok, err = engine.AllowedByRecordRules(10, internalworkflow.ModelWorkflow, record.OpRead, map[string]any{"company_id": nil})
	if err != nil || !ok {
		t.Fatalf("company-free workflow denied: %v %v", ok, err)
	}
}

func TestSecurityRulesAndACLs(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	if len(engine.ACLs) != len(ModelNames())*3+11 {
		t.Fatalf("ACLs = %+v", engine.ACLs)
	}
	if len(SourceCompatibleACLs()) != 11 {
		t.Fatalf("source-compatible ACLs = %+v", SourceCompatibleACLs())
	}
	if len(engine.Rules) != 1 {
		t.Fatalf("rules = %+v", engine.Rules)
	}
	rule := engine.Rules[0]
	if rule.Name == "" || rule.Model != internalworkflow.ModelWorkflow || !rule.Global || !rule.PermRead || !rule.PermWrite || !rule.PermCreate || !rule.PermUnlink {
		t.Fatalf("rule = %+v", rule)
	}

	engine.Users[10] = security.User{ID: 10, Login: "user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{oi_workflow.GroupWorkflowUser}}
	engine.Users[20] = security.User{ID: 20, Login: "manager", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{oi_workflow.GroupWorkflowManager}}
	engine.Users[30] = security.User{ID: 30, Login: "admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{oi_workflow.GroupWorkflowAdmin}}
	engine.Users[40] = security.User{ID: 40, Login: "base-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{oi_workflow.GroupBaseUser}}
	engine.Users[50] = security.User{ID: 50, Login: "base-system", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{oi_workflow.GroupBaseSystem}}
	if err := engine.Check(record.Context{UserID: 10}, internalworkflow.ModelWorkflow, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, internalworkflow.ModelWorkflow, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, internalworkflow.ModelNode, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, internalworkflow.ModelTransition, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, internalworkflow.ModelWorkflow, record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, internalworkflow.ModelWorkflow, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected base user write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 40}, internalworkflow.ModelWorkflowWizard, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 50}, internalworkflow.ModelWorkflow, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	ok, err := engine.AllowedByRecordRules(10, internalworkflow.ModelWorkflow, record.OpRead, map[string]any{"company_id": int64(1)})
	if err != nil || !ok {
		t.Fatalf("same-company workflow denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, internalworkflow.ModelWorkflow, record.OpRead, map[string]any{"company_id": int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("other-company workflow allowed")
	}
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

func assertWorkflowAdvanceAssetFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("asset %s: %v", path, err)
	}
	text := string(raw)
	switch filepath.Ext(path) {
	case ".js":
		if !strings.Contains(text, "@odoo-module") || !strings.Contains(text, "multi_workflow_view") || !strings.Contains(text, "workflow_view_id") {
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
		if doc.XMLName.Local != "templates" || len(doc.Templates) == 0 || doc.Templates[0].Name != "oi_workflow_advance.confirm_view" {
			t.Fatalf("asset %s templates = %+v", path, doc.Templates)
		}
	default:
		t.Fatalf("unsupported asset %s", path)
	}
}

func loadManifestFixtures(t *testing.T) (*record.Env, map[string]data.ExternalID) {
	t.Helper()
	env := workflowAdvanceFixtureEnv(t)
	externalIDs := map[string]data.ExternalID{}
	seedBaseGroups(t, env, externalIDs)
	seedWorkflowGroups(t, env, externalIDs)
	loader := data.NewLoaderWithExternalIDs(env, ModuleName, externalIDs)
	for _, path := range Manifest().Data {
		switch filepath.Ext(path) {
		case ".xml":
			loadXMLFixture(t, loader, path)
		case ".csv":
			loadCSVFixture(t, loader, "ir.model.access", path)
		default:
			t.Fatalf("unsupported fixture file %s", path)
		}
	}
	return env, loader.ExternalIDs()
}

func seedWorkflowGroups(t *testing.T, env *record.Env, externalIDs map[string]data.ExternalID) {
	t.Helper()
	loader := data.NewLoaderWithExternalIDs(env, oi_workflow.ModuleName, externalIDs)
	loadXMLFixture(t, loader, "../oi_workflow/security/oi_workflow_groups.xml")
}

func seedBaseGroups(t *testing.T, env *record.Env, externalIDs map[string]data.ExternalID) {
	t.Helper()
	loader := data.NewLoaderWithExternalIDs(env, "base", externalIDs)
	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_user" model="res.groups">
    <field name="name">Internal User</field>
  </record>
  <record id="group_system" model="res.groups">
    <field name="name">Settings</field>
    <field name="implied_ids" eval="[(4, ref('group_user'))]"/>
  </record>
  <record id="user_root" model="res.users">
    <field name="login">__system__</field>
    <field name="name">System</field>
    <field name="active" eval="False"/>
    <field name="groups_id" eval="[(4, ref('group_system'))]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
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

func workflowAdvanceFixtureEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range internalworkflow.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

func extensionMap() map[string]model.Model {
	extensions := map[string]model.Model{}
	for _, m := range ExtensionModels() {
		extensions[m.Name] = m
	}
	return extensions
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
