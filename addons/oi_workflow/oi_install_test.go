package oi_workflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gorp/addons/hr"
	"gorp/addons/oi_base"
	"gorp/addons/oi_delegation"
	"gorp/addons/oi_login_as"
	"gorp/addons/oi_workflow"
	"gorp/addons/oi_workflow_advance"
	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	internalworkflow "gorp/internal/workflow"
)

func TestInstallAllOI(t *testing.T) {
	reg := registry.New("test")
	manifests := uniqueManifests(
		[]module.Manifest{base.Manifest()},
		oi_workflow.DependencyManifests(),
		oi_workflow_advance.DependencyManifests(),
		oi_delegation.DependencyManifests(),
		oi_login_as.DependencyManifests(),
		hr.DependencyManifests(),
		[]module.Manifest{
			hr.Manifest(),
			oi_base.Manifest(),
			oi_workflow.Manifest(),
			oi_workflow_advance.Manifest(),
			oi_delegation.Manifest(),
			oi_login_as.Manifest(),
		},
	)
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		hr.ModuleName,
		oi_base.ModuleName,
		oi_workflow.ModuleName,
		oi_workflow_advance.ModuleName,
		oi_delegation.ModuleName,
		oi_login_as.ModuleName,
	} {
		if reg.States[name] != "installed" {
			t.Fatalf("module %s state = %q", name, reg.States[name])
		}
	}
	if err := oi_base.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	if err := hr.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	if err := oi_workflow.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	if err := oi_workflow_advance.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	if err := oi_delegation.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	if err := oi_login_as.RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"xml_id.mixin",
		"many2many.attachment.res_id.mixin",
		"hr.department",
		"hr.employee",
		"approval.settings",
		"approval.buttons",
		"workflow",
		"workflow.transition",
		"delegation",
		"delegation.line",
		"login.as",
		"login.as.audit",
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
}

func TestLoadAllOIFixturesInDependencyOrder(t *testing.T) {
	env := oiFixtureEnv(t)
	externalIDs := map[string]data.ExternalID{}
	root := repoRoot(t)

	loadFixtureMetadata(t, env, externalIDs)
	loadManifestData(t, env, externalIDs, base.Manifest().TechnicalName, filepath.Join(root, "internal/base"), base.Manifest().Data)
	loadManifestData(t, env, externalIDs, oi_base.ModuleName, filepath.Join(root, "addons/oi_base"), oi_base.Manifest().Data)
	loadManifestData(t, env, externalIDs, oi_workflow.ModuleName, filepath.Join(root, "addons/oi_workflow"), oi_workflow.Manifest().Data)
	loadManifestData(t, env, externalIDs, oi_delegation.ModuleName, filepath.Join(root, "addons/oi_delegation"), oi_delegation.Manifest().Data)
	loadManifestData(t, env, externalIDs, oi_workflow_advance.ModuleName, filepath.Join(root, "addons/oi_workflow_advance"), oi_workflow_advance.Manifest().Data)

	loginLoader := data.NewLoaderWithExternalIDs(env, oi_login_as.ModuleName, externalIDs)
	seedLoginAsExternalIDs(t, loginLoader)
	loadManifestData(t, env, externalIDs, oi_login_as.ModuleName, filepath.Join(root, "addons/oi_login_as"), oi_login_as.Manifest().Data)

	for _, name := range []string{
		"base.group_user",
		"oi_base.group_oi_base_user",
		"oi_workflow.model_approval_settings",
		"oi_workflow.group_workflow_admin",
		"oi_delegation.model_delegation",
		"oi_delegation.access_delegation_admin",
		"oi_workflow_advance.model_workflow",
		"oi_workflow_advance.access_workflow_user",
		"oi_login_as.model_login_as",
		"oi_login_as.access_login_as_user",
	} {
		if externalIDs[name].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", name, externalIDs)
		}
	}

	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	engine.Users[10] = security.User{ID: 10, Login: "workflow-admin", Active: true, GroupIDs: []int64{externalIDs["oi_workflow.group_workflow_admin"].ResID}}
	engine.Users[20] = security.User{ID: 20, Login: "delegation-admin", Active: true, GroupIDs: []int64{externalIDs["oi_delegation.group_delegation_admin"].ResID}}
	engine.Users[30] = security.User{ID: 30, Login: "login-as-user", Active: true, GroupIDs: []int64{externalIDs["oi_login_as.group_login_as_user"].ResID}}
	if err := engine.Check(record.Context{UserID: 10}, internalworkflow.ModelSettings, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 20}, oi_delegation.ModelDelegation, record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, oi_login_as.ModelLoginAsWizard, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
}

func loadFixtureMetadata(t *testing.T, env *record.Env, externalIDs map[string]data.ExternalID) {
	t.Helper()
	loads := []struct {
		moduleName string
		models     []model.Model
	}{
		{base.Manifest().TechnicalName, base.Models()},
		{hr.ModuleName, hr.Models()},
		{oi_base.ModuleName, oi_base.Models()},
		{oi_workflow.ModuleName, internalworkflow.Models()},
		{oi_workflow_advance.ModuleName, internalworkflow.AdvancedModels()},
		{oi_delegation.ModuleName, oi_delegation.Models()},
		{oi_login_as.ModuleName, oi_login_as.Models()},
	}
	for _, load := range loads {
		if err := data.LoadModelMetadata(env, load.moduleName, load.models, externalIDs); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceManifestFileCoverage(t *testing.T) {
	root := repoRoot(t)
	checkManifestCoverage(t, "oi_workflow", filepath.Join(root, "addons/oi_workflow"), oi_workflow.Manifest(), []string{
		"security/ir.model.access.csv",
		"data/ir_sequence.xml",
		"data/approval_record_templates.xml",
		"view/approval_config.xml",
		"view/approval_escalation.xml",
		"view/approval_state_update.xml",
		"view/approval_settings.xml",
		"view/cancellation_record_view.xml",
		"view/action.xml",
		"view/menu.xml",
		"view/templates.xml",
		"data/mail_activity_type.xml",
		"view/res_config_settings.xml",
		"data/ir_cron.xml",
		"data/approval_settings.xml",
		"view/approval_automation.xml",
		"view/approval_buttons.xml",
		"view/approval_process_wizard.xml",
		"view/approval_log.xml",
		"view/ir_model.xml",
		"view/model_expression_editor.xml",
		"view/res_groups.xml",
	})
	checkManifestCoverage(t, "oi_workflow_advance", filepath.Join(root, "addons/oi_workflow_advance"), oi_workflow_advance.Manifest(), []string{
		"views/approval_settings.xml",
		"views/approval_config.xml",
		"views/workflow_node.xml",
		"views/workflow.xml",
		"views/workflow_process_wizard.xml",
		"views/approval_state_update.xml",
		"views/action.xml",
		"security/ir.model.access.csv",
		"data/ir_cron.xml",
		"views/approval_log.xml",
		"security/ir_rule.xml",
		"views/workflow_transition.xml",
	})
	checkManifestCoverage(t, "oi_delegation", filepath.Join(root, "addons/oi_delegation"), oi_delegation.Manifest(), []string{
		"data/ir_cron.xml",
		"data/mail_template.xml",
		"data/sequences.xml",
		"security/group.xml",
		"security/ir.model.access.csv",
		"security/rules.xml",
		"view/delegation.xml",
		"view/mail_template.xml",
		"view/res_groups.xml",
		"view/action.xml",
		"view/menu.xml",
		"data/approval_config.xml",
		"data/approval_buttons.xml",
		"view/approval_log.xml",
	})
	checkManifestCoverage(t, "oi_login_as", filepath.Join(root, "addons/oi_login_as"), oi_login_as.Manifest(), []string{
		"view/login_as.xml",
		"view/templates.xml",
		"security/ir.model.access.csv",
		"view/action.xml",
	})
}

func checkManifestCoverage(t *testing.T, moduleName string, baseDir string, manifest module.Manifest, sourcePaths []string) {
	t.Helper()
	if manifest.TechnicalName != moduleName {
		t.Fatalf("%s technical name = %s", moduleName, manifest.TechnicalName)
	}
	for _, sourcePath := range sourcePaths {
		found := false
		for _, path := range manifest.Data {
			if path == sourcePath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s manifest missing source data path %s in %+v", moduleName, sourcePath, manifest.Data)
		}
		if _, err := os.Stat(filepath.Join(baseDir, sourcePath)); err != nil {
			t.Fatalf("%s source-compatible fixture %s: %v", moduleName, sourcePath, err)
		}
	}
}

func uniqueManifests(groups ...[]module.Manifest) []module.Manifest {
	byName := map[string]module.Manifest{}
	for _, group := range groups {
		for _, manifest := range group {
			byName[manifest.TechnicalName] = manifest
		}
	}
	order := []string{"base", "mail", "automation", "base_automation", "web", "portal", "base_setup", "digest", "phone_validation", "resource", "resource_mail", "hr", oi_base.ModuleName, "oi_base_cache", "oi_web_selection_field_dynamic", "oi_web_selection_tags", "oi_web_flowchart", oi_workflow.ModuleName, oi_workflow_advance.ModuleName, oi_delegation.ModuleName, oi_login_as.ModuleName}
	var out []module.Manifest
	for _, name := range order {
		if manifest, ok := byName[name]; ok {
			out = append(out, manifest)
			delete(byName, name)
		}
	}
	for _, manifest := range byName {
		out = append(out, manifest)
	}
	return out
}

func oiFixtureEnv(t *testing.T) *record.Env {
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
	for _, m := range hr.Models() {
		add(m)
	}
	for _, m := range hr.ExtensionModels() {
		add(m)
	}
	for _, m := range oi_base.Models() {
		add(m)
	}
	for _, m := range oi_base.ExtensionModels() {
		add(m)
	}
	for _, m := range internalworkflow.Models() {
		add(m)
	}
	for _, m := range internalworkflow.AdvancedModels() {
		add(m)
	}
	for _, m := range oi_delegation.Models() {
		add(m)
	}
	for _, m := range oi_delegation.ExtensionModels() {
		add(m)
	}
	for _, m := range oi_login_as.Models() {
		add(m)
	}
	for _, m := range oi_login_as.ExtensionModels() {
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

func loadManifestData(t *testing.T, env *record.Env, externalIDs map[string]data.ExternalID, moduleName string, baseDir string, paths []string) {
	t.Helper()
	loader := data.NewLoaderWithExternalIDs(env, moduleName, externalIDs)
	loader.SetBaseDir(baseDir)
	for _, rel := range paths {
		path := filepath.Join(baseDir, rel)
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		switch filepath.Ext(path) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			err = loader.LoadCSV(strings.TrimSuffix(filepath.Base(path), ".csv"), file)
		default:
			err = fmt.Errorf("unsupported fixture file %s", path)
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

func seedLoginAsExternalIDs(t *testing.T, loader *data.Loader) {
	t.Helper()
	var seed strings.Builder
	seed.WriteString("<odoo>")
	for _, group := range oi_login_as.SecurityGroups() {
		seed.WriteString(`<record id="` + loginAsGroupExternalID(group.ID) + `" model="res.groups">`)
		seed.WriteString(`<field name="name">` + group.Name + `</field>`)
		if len(group.ImpliedIDs) > 0 {
			seed.WriteString(`<field name="implied_ids" eval="[`)
			for i, implied := range group.ImpliedIDs {
				if i > 0 {
					seed.WriteByte(',')
				}
				seed.WriteString(`(4, ref('` + loginAsGroupExternalID(implied) + `'))`)
			}
			seed.WriteString(`]"/>`)
		}
		seed.WriteString(`</record>`)
	}
	for _, m := range oi_login_as.Models() {
		seed.WriteString(`<record id="` + modelExternalID(m.Name) + `" model="ir.model">`)
		seed.WriteString(`<field name="model">` + m.Name + `</field>`)
		seed.WriteString(`<field name="name">` + m.Name + `</field>`)
		seed.WriteString(`</record>`)
	}
	seed.WriteString("</odoo>")
	if err := loader.LoadXML(strings.NewReader(seed.String())); err != nil {
		t.Fatal(err)
	}
}

func loginAsGroupExternalID(groupID int64) string {
	switch groupID {
	case oi_login_as.GroupLoginAsUser:
		return "group_login_as_user"
	case oi_login_as.GroupLoginAsAdmin:
		return "group_login_as_admin"
	case oi_login_as.GroupLoginAsAllowInactive:
		return "group_login_as_allow_inactive"
	case oi_login_as.GroupLoginAsAllowSuperuser:
		return "group_login_as_allow_superuser"
	case oi_login_as.GroupLoginAsDebug:
		return "group_login_as_debug"
	default:
		return fmt.Sprintf("group_login_as_%d", groupID)
	}
}

func modelExternalID(modelName string) string {
	return "model_" + strings.NewReplacer(".", "_").Replace(modelName)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
