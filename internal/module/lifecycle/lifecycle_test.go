package lifecycle

import (
	"fmt"
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

func TestButtonInstallChecksPythonExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crm := manifests["crm"]
	crm.ExternalDependencies = map[string][]string{"python": {"missing_py"}}
	manifests["crm"] = crm
	createModuleRow(t, env, "base", "installed")
	createModuleRow(t, env, "mail", "uninstalled")
	crmID := createModuleRow(t, env, "crm", "uninstalled")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "missing_py" {
			return fmt.Errorf("not importable")
		}
		return nil
	}
	service.CheckBinaryDependency = func(string) error {
		t.Fatal("binary dependency check should not run after python failure")
		return nil
	}

	_, err := service.ButtonInstall([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "module crm missing external python dependency missing_py") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "mail", "uninstalled")
	assertModuleState(t, env, "crm", "uninstalled")
}

func TestButtonInstallChecksTransitiveExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mail := manifests["mail"]
	mail.ExternalDependencies = map[string][]string{"python": {"missing_mail_dep"}}
	manifests["mail"] = mail
	createModuleRow(t, env, "base", "installed")
	createModuleRow(t, env, "mail", "uninstalled")
	crmID := createModuleRow(t, env, "crm", "uninstalled")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "missing_mail_dep" {
			return fmt.Errorf("not importable")
		}
		return nil
	}

	_, err := service.ButtonInstall([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "module mail missing external python dependency missing_mail_dep") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "mail", "uninstalled")
	assertModuleState(t, env, "crm", "uninstalled")
	assertModuleMissing(t, env, "auto_crm")
}

func TestButtonInstallSkipsExternalDependenciesForInstalledDependency(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	base := manifests["base"]
	base.ExternalDependencies = map[string][]string{"python": {"already_installed_missing"}}
	manifests["base"] = base
	createModuleRow(t, env, "base", "installed")
	mailID := createModuleRow(t, env, "mail", "uninstalled")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "already_installed_missing" {
			return fmt.Errorf("installed module dependency should not be checked")
		}
		return nil
	}

	result, err := service.ButtonInstall([]int64{mailID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "install" || strings.Join(result.Modules, ",") != "mail" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "base", "installed")
	assertModuleState(t, env, "mail", "to install")
}

func TestButtonInstallChecksAutoInstallExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	autoWebExtra := manifests["auto_web_extra"]
	autoWebExtra.ExternalDependencies = map[string][]string{"bin": {"missing-auto-bin"}}
	manifests["auto_web_extra"] = autoWebExtra
	createModuleRow(t, env, "base", "installed")
	webID := createModuleRow(t, env, "web", "uninstalled")

	service := New(env, manifests)
	service.CheckBinaryDependency = func(name string) error {
		if name == "missing-auto-bin" {
			return fmt.Errorf("not in path")
		}
		return nil
	}

	_, err := service.ButtonInstall([]int64{webID})
	if err == nil || !strings.Contains(err.Error(), "module auto_web_extra missing external binary dependency missing-auto-bin") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "web", "uninstalled")
	assertModuleMissing(t, env, "mail")
	assertModuleMissing(t, env, "auto_web_extra")
}

func TestButtonInstallChecksBinaryExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	web := manifests["web"]
	web.ExternalDependencies = map[string][]string{"bin": {"missing-bin"}}
	manifests["web"] = web
	createModuleRow(t, env, "base", "installed")
	webID := createModuleRow(t, env, "web", "uninstalled")

	service := New(env, manifests)
	service.CheckBinaryDependency = func(name string) error {
		if name == "missing-bin" {
			return fmt.Errorf("not in path")
		}
		return nil
	}

	_, err := service.ButtonImmediateInstall([]int64{webID})
	if err == nil || !strings.Contains(err.Error(), "module web missing external binary dependency missing-bin") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "web", "uninstalled")
}

func TestButtonInstallExternalDependencyChecksAreNormalized(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	web := manifests["web"]
	web.ExternalDependencies = map[string][]string{
		"python": {" zlib ", "zlib", ""},
		"bin":    {" sh ", "sh", ""},
	}
	manifests["web"] = web
	createModuleRow(t, env, "base", "installed")
	webID := createModuleRow(t, env, "web", "uninstalled")

	var pythonCalls []string
	var binaryCalls []string
	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		pythonCalls = append(pythonCalls, name)
		return nil
	}
	service.CheckBinaryDependency = func(name string) error {
		binaryCalls = append(binaryCalls, name)
		return nil
	}

	result, err := service.ButtonInstall([]int64{webID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "install" || strings.Join(result.Modules, ",") != "auto_web_extra,mail,web" {
		t.Fatalf("result = %+v", result)
	}
	if strings.Join(pythonCalls, ",") != "zlib" || strings.Join(binaryCalls, ",") != "sh" {
		t.Fatalf("dependency checks python=%v binary=%v", pythonCalls, binaryCalls)
	}
	assertModuleState(t, env, "web", "to install")
	assertModuleState(t, env, "mail", "to install")
	assertModuleState(t, env, "auto_web_extra", "to install")
}

func TestPythonDistributionNameNormalizesRequirements(t *testing.T) {
	cases := map[string]string{
		"python-ldap":                          "python-ldap",
		" geoip2 ":                             "geoip2",
		"phonenumbers>=8.13":                   "phonenumbers",
		"google-auth[reauth]>=2":               "google-auth",
		"asn1crypto; python_version >= '3.10'": "asn1crypto",
	}
	for input, want := range cases {
		if got := pythonDistributionName(input); got != want {
			t.Fatalf("pythonDistributionName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUpdateListCreatesMissingRowsAndPreservesExistingStates(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "base", "installed")
	createModuleRow(t, env, "mail", "to install")
	createModuleRow(t, env, "crm", "to upgrade")
	createModuleRow(t, env, "web", "to remove")

	result, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 2 || result.Updated != 3 || strings.Join(result.Modules, ",") != "auto_crm,auto_web_extra,crm,mail,web" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "base", "installed")
	assertModuleState(t, env, "mail", "to install")
	assertModuleState(t, env, "crm", "to upgrade")
	assertModuleState(t, env, "web", "to remove")
	assertModuleState(t, env, "auto_crm", "uninstalled")
	assertModuleState(t, env, "auto_web_extra", "uninstalled")
}

func TestUpdateListReactivatesInstallableUninstallableRows(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	createModuleRow(t, env, "crm", "uninstallable")

	result, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Added != 5 {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "crm", "uninstalled")
}

func TestUpdateListKeepsNonInstallableMissingRowsUninstallable(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := map[string]module.Manifest{
		"legacy": {Name: "Legacy", TechnicalName: "legacy", Version: "19.0", Installable: false},
	}

	result, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || result.Updated != 0 || strings.Join(result.Modules, ",") != "legacy" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "legacy", "uninstallable")
}

func TestUpdateListSyncsDependenciesAndExclusions(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := map[string]module.Manifest{
		"feature": {
			Name:               "Feature",
			TechnicalName:      "feature",
			Version:            "19.0",
			Depends:            []string{"base", "mail"},
			Excludes:           []string{"legacy_feature"},
			Installable:        true,
			AutoInstall:        true,
			AutoInstallDepends: []string{"mail"},
		},
	}
	featureID := createModuleRow(t, env, "feature", "installed")
	createModuleDependency(t, env, featureID, "old_dep", false)
	createModuleDependency(t, env, featureID, "base", true)
	createModuleExclusion(t, env, featureID, "old_feature")

	result, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Added != 0 || strings.Join(result.Modules, ",") != "feature" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleDependency(t, env, featureID, "base", false)
	assertModuleDependency(t, env, featureID, "mail", true)
	assertModuleDependencyMissing(t, env, featureID, "old_dep")
	assertModuleExclusion(t, env, featureID, "legacy_feature")
	assertModuleExclusionMissing(t, env, featureID, "old_feature")
}

func TestUpdateListIsIdempotentAfterSync(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()

	first, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(env, manifests).UpdateList()
	if err != nil {
		t.Fatal(err)
	}
	if first.Added != 6 || second.Added != 0 || second.Updated != 0 || len(second.Modules) != 0 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
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

func TestButtonUpgradeChecksExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crm := manifests["crm"]
	crm.ExternalDependencies = map[string][]string{"python": {"missing_upgrade_dep"}}
	manifests["crm"] = crm
	crmID := createModuleRow(t, env, "crm", "installed")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "missing_upgrade_dep" {
			return fmt.Errorf("not importable")
		}
		return nil
	}

	_, err := service.ButtonUpgrade([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "module crm missing external python dependency missing_upgrade_dep") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonUpgradeDoesNotCheckInstalledDependencyExternalDependencies(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mail := manifests["mail"]
	mail.ExternalDependencies = map[string][]string{"python": {"installed_dependency_missing"}}
	manifests["mail"] = mail
	createModuleRow(t, env, "mail", "installed")
	crmID := createModuleRow(t, env, "crm", "installed")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "installed_dependency_missing" {
			return fmt.Errorf("installed dependency should not be checked")
		}
		return nil
	}

	result, err := service.ButtonUpgrade([]int64{crmID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "upgrade" || strings.Join(result.Modules, ",") != "crm" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "mail", "installed")
	assertModuleState(t, env, "crm", "to upgrade")
}

func TestButtonUpgradeQueuesInstalledReverseDependents(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mailID := createModuleRow(t, env, "mail", "installed")
	createModuleRow(t, env, "crm", "installed")

	result, err := New(env, manifests).ButtonUpgrade([]int64{mailID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "upgrade" || strings.Join(result.Modules, ",") != "crm,mail" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "mail", "to upgrade")
	assertModuleState(t, env, "crm", "to upgrade")
}

func TestButtonImmediateUpgradeReturnsInstalledPlanForReload(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mailID := createModuleRow(t, env, "mail", "installed")
	createModuleRow(t, env, "crm", "installed")

	result, err := New(env, manifests).ButtonImmediateUpgrade([]int64{mailID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation != "immediate_upgrade" || strings.Join(result.Modules, ",") != "crm,mail" {
		t.Fatalf("result = %+v", result)
	}
	assertModuleState(t, env, "mail", "installed")
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonUpgradeChecksReverseDependentExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crm := manifests["crm"]
	crm.ExternalDependencies = map[string][]string{"python": {"missing_reverse_dep"}}
	manifests["crm"] = crm
	mailID := createModuleRow(t, env, "mail", "installed")
	createModuleRow(t, env, "crm", "installed")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "missing_reverse_dep" {
			return fmt.Errorf("not importable")
		}
		return nil
	}

	_, err := service.ButtonUpgrade([]int64{mailID})
	if err == nil || !strings.Contains(err.Error(), "module crm missing external python dependency missing_reverse_dep") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "mail", "installed")
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonImmediateUpgradeChecksExternalDependenciesBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crm := manifests["crm"]
	crm.ExternalDependencies = map[string][]string{"bin": {"missing-upgrade-bin"}}
	manifests["crm"] = crm
	crmID := createModuleRow(t, env, "crm", "to upgrade")

	service := New(env, manifests)
	service.CheckBinaryDependency = func(name string) error {
		if name == "missing-upgrade-bin" {
			return fmt.Errorf("not in path")
		}
		return nil
	}

	_, err := service.ButtonImmediateUpgrade([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "module crm missing external binary dependency missing-upgrade-bin") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "to upgrade")
}

func TestButtonImmediateUpgradeChecksExternalDependenciesFromInstalledBeforeWriting(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	crm := manifests["crm"]
	crm.ExternalDependencies = map[string][]string{"python": {"missing_immediate_installed_dep"}}
	manifests["crm"] = crm
	crmID := createModuleRow(t, env, "crm", "installed")

	service := New(env, manifests)
	service.CheckPythonDependency = func(name string) error {
		if name == "missing_immediate_installed_dep" {
			return fmt.Errorf("not importable")
		}
		return nil
	}

	_, err := service.ButtonImmediateUpgrade([]int64{crmID})
	if err == nil || !strings.Contains(err.Error(), "module crm missing external python dependency missing_immediate_installed_dep") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
}

func TestButtonUpgradeExternalDependencyFailureDoesNotPartiallyWriteMultipleRows(t *testing.T) {
	env := lifecycleTestEnv(t)
	manifests := lifecycleTestManifests()
	mail := manifests["mail"]
	mail.ExternalDependencies = map[string][]string{"bin": {"missing-multi-bin"}}
	manifests["mail"] = mail
	crmID := createModuleRow(t, env, "crm", "installed")
	mailID := createModuleRow(t, env, "mail", "installed")

	service := New(env, manifests)
	service.CheckBinaryDependency = func(name string) error {
		if name == "missing-multi-bin" {
			return fmt.Errorf("not in path")
		}
		return nil
	}

	_, err := service.ButtonUpgrade([]int64{crmID, mailID})
	if err == nil || !strings.Contains(err.Error(), "module mail missing external binary dependency missing-multi-bin") {
		t.Fatalf("error = %v", err)
	}
	assertModuleState(t, env, "crm", "installed")
	assertModuleState(t, env, "mail", "installed")
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

func createModuleDependency(t *testing.T, env *record.Env, moduleID int64, name string, autoInstallRequired bool) int64 {
	t.Helper()
	id, err := env.Model("ir.module.module.dependency").Create(map[string]any{"module_id": moduleID, "name": name, "auto_install_required": autoInstallRequired})
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

func assertModuleMissing(t *testing.T, env *record.Env, name string) {
	t.Helper()
	found, err := env.Model("ir.module.module").Search(domain.Cond("name", "=", name))
	if err != nil {
		t.Fatal(err)
	}
	if ids := found.IDs(); len(ids) != 0 {
		t.Fatalf("%s should be missing, found ids %v", name, ids)
	}
}

func assertModuleDependency(t *testing.T, env *record.Env, moduleID int64, name string, autoInstallRequired bool) {
	t.Helper()
	found, err := env.Model("ir.module.module.dependency").Search(domain.And(
		domain.Cond("module_id", "=", moduleID),
		domain.Cond("name", "=", name),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("auto_install_required")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("dependency %d/%s rows = %+v", moduleID, name, rows)
	}
	if rows[0]["auto_install_required"] != autoInstallRequired {
		t.Fatalf("dependency %d/%s auto_install_required = %+v, want %v", moduleID, name, rows[0]["auto_install_required"], autoInstallRequired)
	}
}

func assertModuleDependencyMissing(t *testing.T, env *record.Env, moduleID int64, name string) {
	t.Helper()
	assertModuleNamedRowMissing(t, env, "ir.module.module.dependency", moduleID, name)
}

func assertModuleExclusion(t *testing.T, env *record.Env, moduleID int64, name string) {
	t.Helper()
	found, err := env.Model("ir.module.module.exclusion").Search(domain.And(
		domain.Cond("module_id", "=", moduleID),
		domain.Cond("name", "=", name),
	))
	if err != nil {
		t.Fatal(err)
	}
	if rows := found.IDs(); len(rows) != 1 {
		t.Fatalf("exclusion %d/%s rows = %+v", moduleID, name, rows)
	}
}

func assertModuleExclusionMissing(t *testing.T, env *record.Env, moduleID int64, name string) {
	t.Helper()
	assertModuleNamedRowMissing(t, env, "ir.module.module.exclusion", moduleID, name)
}

func assertModuleNamedRowMissing(t *testing.T, env *record.Env, modelName string, moduleID int64, name string) {
	t.Helper()
	found, err := env.Model(modelName).Search(domain.And(
		domain.Cond("module_id", "=", moduleID),
		domain.Cond("name", "=", name),
	))
	if err != nil {
		t.Fatal(err)
	}
	if ids := found.IDs(); len(ids) != 0 {
		t.Fatalf("%s %d/%s should be missing, found ids %v", modelName, moduleID, name, ids)
	}
}
