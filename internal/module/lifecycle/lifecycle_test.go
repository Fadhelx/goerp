package lifecycle

import (
	"strings"
	"testing"

	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/module"
	"gorp/internal/record"
)

func TestButtonInstallQueuesDependencies(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "base", "installed")
	appID := createModuleRow(t, env, "crm", "uninstalled")
	createModuleRow(t, env, "mail", "uninstalled")

	result, err := New(env, manifests).ButtonInstall([]int64{appID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "install" || strings.Join(result.Modules, ",") != "auto_crm,crm,mail" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "crm", "to install")
	assertModuleState(t, env, "mail", "to install")
	assertModuleState(t, env, "auto_crm", "to install")
}

func TestButtonImmediateInstallInstallsDependencies(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "base", "installed")
	appID := createModuleRow(t, env, "crm", "uninstalled")

	result, err := New(env, manifests).ButtonImmediateInstall([]int64{appID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "immediate_install" || strings.Join(result.Modules, ",") != "auto_crm,crm,mail" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "crm", "installed")
	assertModuleState(t, env, "mail", "installed")
	assertModuleState(t, env, "auto_crm", "installed")
}

func TestButtonInstallQueuesAutoInstallModules(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "base", "installed")
	createModuleRow(t, env, "mail", "installed")
	crmID := createModuleRow(t, env, "crm", "uninstalled")

	result, err := New(env, manifests).ButtonInstall([]int64{crmID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "install" || strings.Join(result.Modules, ",") != "auto_crm,crm" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "crm", "to install")
	assertModuleState(t, env, "auto_crm", "to install")
}

func TestButtonImmediateInstallAutoInstallCreatesDependencies(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "base", "installed")
	webID := createModuleRow(t, env, "web", "uninstalled")

	result, err := New(env, manifests).ButtonImmediateInstall([]int64{webID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "immediate_install" || strings.Join(result.Modules, ",") != "auto_web_extra,mail,web" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "web", "installed")
	assertModuleState(t, env, "mail", "installed")
	assertModuleState(t, env, "auto_web_extra", "installed")
}

func TestButtonInstallBlocksModuleExclusions(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "crm", "installed")
	mailID := createModuleRow(t, env, "mail", "uninstalled")
	createModuleExclusion(t, env, mailID, "crm")

	_, err := New(env, manifests).ButtonInstall([]int64{mailID})
	if err == nil || !strings.Contains(err.Error(), "module mail excludes module crm") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
	assertModuleState(t, env, "mail", "uninstalled")
}

func TestButtonUninstallRejectsInstalledDependents(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mailID := createModuleRow(t, env, "mail", "installed")
	createModuleRow(t, env, "crm", "installed")

	_, err := New(env, manifests).ButtonUninstall([]int64{mailID})
	if err == nil || !strings.Contains(err.Error(), "required by installed module crm") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "mail", "installed")
}

func TestButtonUninstallRejectsInactiveStates(t *testing.T) {
	for _, state := range []string{"uninstalled", "to install", "to remove"} {
		t.Run(state, func(t *testing.T) {
			env := lifecycleTestEnv(t)
			manifests := lifecycleTestManifests()
			crmID := createModuleRow(t, env, "crm", state)

			_, err := New(env, manifests).ButtonUninstall([]int64{crmID})
			if err == nil || !strings.Contains(err.Error(), "cannot run uninstall from state "+state) {
				t.Fatalf("error = %v", err)
			}
			assertModuleState(t, env, "crm", state)
		})
	}
}

func TestButtonUninstallRejectsQueuedDependents(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mailID := createModuleRow(t, env, "mail", "installed")
	createModuleRow(t, env, "crm", "to install")

	_, err := New(env, manifests).ButtonUninstall([]int64{mailID})
	if err == nil || !strings.Contains(err.Error(), "required by module crm in state to install") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "mail", "installed")
}

func TestButtonUpgradeAndImmediateUpgradeTransitions(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "installed")

	queued, err := New(env, manifests).ButtonUpgrade([]int64{crmID})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Operation != "upgrade" || strings.Join(queued.Modules, ",") != "crm" {
		t.Fatalf("queued = %+v", queued)
	}
	assertModuleState(t, env, "crm", "to upgrade")

	immediate, err := New(env, manifests).ButtonImmediateUpgrade([]int64{crmID})
	if err != nil {
		t.Fatal(err)
	}
	if immediate.Operation != "immediate_upgrade" || strings.Join(immediate.Modules, ",") != "crm" {
		t.Fatalf("immediate = %+v", immediate)
	}
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonUpgradeRejectsUninstalledModule(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "uninstalled")

	_, err := New(env, manifests).ButtonUpgrade([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "cannot run upgrade from state uninstalled") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "uninstalled")
}

func TestButtonImmediateUpgradeRejectsUninstalledModule(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "uninstalled")

	_, err := New(env, manifests).ButtonImmediateUpgrade([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "cannot run immediate_upgrade from state uninstalled") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "uninstalled")
}

func TestCancelTransitionsDoNotFlipInstalledOrUninstalledRows(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	installID := createModuleRow(t, env, "crm", "to install")
	removeID := createModuleRow(t, env, "mail", "to remove")

	cancelInstall, err := New(env, manifests).ButtonCancelInstall([]int64{installID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelInstall.Operation != "cancel_install" || strings.Join(cancelInstall.Modules, ",") != "crm" {
		t.Fatalf("cancel install = %+v", cancelInstall)
	}
	assertModuleState(t, env, "crm", "uninstalled")

	cancelUninstall, err := New(env, manifests).ButtonCancelUninstall([]int64{removeID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelUninstall.Operation != "cancel_uninstall" || strings.Join(cancelUninstall.Modules, ",") != "mail" {
		t.Fatalf("cancel uninstall = %+v", cancelUninstall)
	}
	assertModuleState(t, env, "mail", "installed")
}

func TestCancelUpgradeRestoresToInstalled(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "to upgrade")

	result, err := New(env, manifests).ButtonCancelUpgrade([]int64{crmID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "cancel_upgrade" || strings.Join(result.Modules, ",") != "crm" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "crm", "installed")
}

func TestInvalidMarkDoesNotPartiallyWrite(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "installed")
	mailID := createModuleRow(t, env, "mail", "uninstalled")

	_, err := New(env, manifests).ButtonUpgrade([]int64{crmID, mailID})
	if err == nil || !strings.Contains(err.Error(), "cannot run upgrade from state uninstalled") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
	assertModuleState(t, env, "mail", "uninstalled")
}

func TestCancelInstallRejectsInstalledModule(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crmID := createModuleRow(t, env, "crm", "installed")

	_, err := New(env, manifests).ButtonCancelInstall([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "cannot run cancel_install from state installed") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonImmediateUninstallBlocksBase(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	baseID := createModuleRow(t, env, "base", "installed")

	_, err := New(env, manifests).ButtonImmediateUninstall([]int64{baseID})
	if err == nil || !strings.Contains(err.Error(), "base cannot be uninstalled") {
		t.Fatalf("error = %v", err)
	}
}

func TestButtonInstallRejectsUnknownModule(t *testing.T) {
	env := lifecycleTestEnv(t)
	unknownID := createModuleRow(t, env, "unknown", "uninstalled")

	_, err := New(env, lifecycleTestManifests()).ButtonInstall([]int64{unknownID})
	if err == nil || !strings.Contains(err.Error(), "unknown module unknown") {
		t.Fatalf("error = %v", err)
	}
}

func lifecycleTestEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, model := range base.Models() {
		if err := reg.Register(model); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
}

func lifecycleTestManifests() map[string]module.Manifest {
	return map[string]module.Manifest{
		"base": {Name: "Base", TechnicalName: "base", Version: "19.0", Installable: true},
		"mail": {Name: "Mail", TechnicalName: "mail", Version: "19.0", Depends: []string{
			"base",
		}, Installable: true},
		"crm": {Name: "CRM", TechnicalName: "crm", Version: "19.0", Depends: []string{
			"mail",
		}, Installable: true},
		"auto_crm": {Name: "Auto CRM", TechnicalName: "auto_crm", Version: "19.0", Depends: []string{
			"mail", "crm",
		}, Installable: true, AutoInstall: true},
		"web": {Name: "Web", TechnicalName: "web", Version: "19.0", Depends: []string{
			"base",
		}, Installable: true},
		"auto_web_extra": {Name: "Auto Web Extra", TechnicalName: "auto_web_extra", Version: "19.0", Depends: []string{
			"web", "mail",
		}, AutoInstallDepends: []string{"web"}, Installable: true, AutoInstall: true},
	}
}

func createModuleRow(t *testing.T, env *record.Env, name string, state string) int64 {
	t.Helper()
	id, err := env.Model("ir.module.module").Create(map[string]any{"name": name, "state": state})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createModuleExclusion(t *testing.T, env *record.Env, moduleID int64, excluded string) int64 {
	t.Helper()
	id, err := env.Model("ir.module.module.exclusion").Create(map[string]any{"module_id": moduleID, "name": excluded})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertModuleState(t *testing.T, env *record.Env, name string, want string) {
	t.Helper()
	found, err := env.Model("ir.module.module").Search(domain.Cond("name", "=", name))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s rows = %d", name, len(rows))
	}
	if rows[0]["state"] != want {
		t.Fatalf("%s state = %v, want %s", name, rows[0]["state"], want)
	}
}
