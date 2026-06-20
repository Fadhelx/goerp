package accounting_enterprise_hooks

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorp/internal/module"
	platformregistry "gorp/internal/registry"
)

func TestManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != TechnicalName {
		t.Fatalf("technical name = %q", manifest.TechnicalName)
	}
	if !manifest.Installable {
		t.Fatal("manifest must be installable")
	}
	if manifest.AutoInstall {
		t.Fatal("manifest must not auto install")
	}
	if manifest.Application {
		t.Fatal("manifest must not be an application")
	}
	if !reflect.DeepEqual(manifest.Depends, []string{AccountingDependency}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if len(manifest.Data) != 0 || len(manifest.Demo) != 0 || len(manifest.Assets) != 0 {
		t.Fatalf("manifest must not ship data, demo, or assets: %+v", manifest)
	}

	accounting := module.Manifest{
		Name:          "Accounting",
		TechnicalName: AccountingDependency,
		Version:       "19.0.1.0.0",
		Installable:   true,
	}
	ordered, err := module.SortByDependencies([]module.Manifest{manifest, accounting})
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].TechnicalName != AccountingDependency || ordered[1].TechnicalName != TechnicalName {
		t.Fatalf("unexpected install order: %+v", ordered)
	}
}

func TestRequiredHookNames(t *testing.T) {
	expected := []HookName{
		HookAccountantDashboards,
		HookLockDateWizards,
		HookAssets,
		HookBudgets,
		HookReports,
		HookFollowup,
		HookBankStatementImport,
		HookBatchPayments,
		HookISO20022SEPA,
		HookExternalTaxProviders,
		HookIntrastatSAFT,
		HookInvoiceBankStatementExtract,
		HookLoans,
		HookTransfers,
	}
	if !reflect.DeepEqual(RequiredHookNames(), expected) {
		t.Fatalf("required names = %+v", RequiredHookNames())
	}

	names := RequiredHookNames()
	names[0] = "mutated"
	if RequiredHookNames()[0] != HookAccountantDashboards {
		t.Fatal("required hook names must be immutable to callers")
	}
}

func TestDuplicateRegistrationRejected(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(HookAccountantDashboards, dashboardHook{name: "dashboard"}); err != nil {
		t.Fatal(err)
	}
	err := reg.Register(HookAccountantDashboards, dashboardHook{name: "other_dashboard"})
	if !errors.Is(err, ErrDuplicateHook) {
		t.Fatalf("expected duplicate hook error, got %v", err)
	}
}

func TestMissingHookGuard(t *testing.T) {
	reg := NewRegistry()
	reg.Enable()
	_, err := reg.Reports()
	if !errors.Is(err, ErrHookNotRegistered) {
		t.Fatalf("expected missing hook guard error, got %v", err)
	}
}

func TestDisabledByDefault(t *testing.T) {
	reg := NewRegistry()
	if reg.Enabled() {
		t.Fatal("registry must be disabled by default")
	}
	if reg.Len() != 0 {
		t.Fatalf("default hooks = %d", reg.Len())
	}
	_, err := reg.AccountantDashboards()
	if !errors.Is(err, ErrHooksDisabled) {
		t.Fatalf("expected disabled guard error, got %v", err)
	}
	if Manifest().AutoInstall {
		t.Fatal("module must not auto install")
	}
}

func TestInstallGuards(t *testing.T) {
	platform := platformregistry.New("test")
	hooks := NewRegistry()
	err := Install(platform, hooks)
	if !errors.Is(err, ErrDependencyNotInstalled) {
		t.Fatalf("expected missing accounting dependency, got %v", err)
	}

	accounting := module.Manifest{
		Name:          "Accounting",
		TechnicalName: AccountingDependency,
		Version:       "19.0.1.0.0",
		Installable:   true,
	}
	if err := platform.Install([]module.Manifest{accounting, Manifest()}); err != nil {
		t.Fatal(err)
	}
	if err := Install(platform, hooks); err != nil {
		t.Fatal(err)
	}
	if !hooks.Enabled() {
		t.Fatal("install must enable registry")
	}
}

type dashboardHook struct {
	name string
}

func (h dashboardHook) ProviderName() string {
	return h.name
}

func (h dashboardHook) BuildAccountantDashboard(context.Context, Request) (Response, error) {
	return Response{}, nil
}
