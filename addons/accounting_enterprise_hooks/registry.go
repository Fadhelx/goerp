package accounting_enterprise_hooks

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	platformregistry "gorp/internal/registry"
)

var (
	ErrNilRegistry             = errors.New("nil accounting enterprise hook registry")
	ErrHooksDisabled           = errors.New("accounting enterprise hooks disabled")
	ErrUnknownHook             = errors.New("unknown accounting enterprise hook")
	ErrDuplicateHook           = errors.New("duplicate accounting enterprise hook")
	ErrInvalidHook             = errors.New("invalid accounting enterprise hook")
	ErrHookNotRegistered       = errors.New("accounting enterprise hook not registered")
	ErrDependencyNotInstalled  = errors.New("accounting dependency not installed")
	ErrHookModuleNotInstalled  = errors.New("accounting enterprise hook module not installed")
	ErrPlatformRegistryMissing = errors.New("platform registry missing")
)

type Registry struct {
	enabled bool
	hooks   map[HookName]any
}

func NewRegistry() *Registry {
	return &Registry{
		hooks: map[HookName]any{},
	}
}

func GuardInstall(reg *platformregistry.Registry) error {
	if reg == nil {
		return ErrPlatformRegistryMissing
	}
	if reg.States[AccountingDependency] != "installed" {
		return fmt.Errorf("%w: %s", ErrDependencyNotInstalled, AccountingDependency)
	}
	if reg.States[TechnicalName] != "installed" {
		return fmt.Errorf("%w: %s", ErrHookModuleNotInstalled, TechnicalName)
	}
	return nil
}

func Install(reg *platformregistry.Registry, hooks *Registry) error {
	if err := GuardInstall(reg); err != nil {
		return err
	}
	if hooks == nil {
		return ErrNilRegistry
	}
	hooks.Enable()
	return nil
}

func (r *Registry) Enable() {
	r.enabled = true
}

func (r *Registry) Disable() {
	r.enabled = false
}

func (r *Registry) Enabled() bool {
	return r != nil && r.enabled
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.hooks)
}

func (r *Registry) Register(name HookName, hook any) error {
	if r == nil {
		return ErrNilRegistry
	}
	if !IsRequiredHookName(name) {
		return fmt.Errorf("%w: %s", ErrUnknownHook, name)
	}
	if isNil(hook) {
		return fmt.Errorf("%w: %s", ErrInvalidHook, name)
	}
	if _, exists := r.hooks[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateHook, name)
	}
	if err := validateHook(name, hook); err != nil {
		return err
	}
	r.hooks[name] = hook
	return nil
}

func (r *Registry) Require(name HookName) (any, error) {
	if r == nil {
		return nil, ErrNilRegistry
	}
	if !IsRequiredHookName(name) {
		return nil, fmt.Errorf("%w: %s", ErrUnknownHook, name)
	}
	if !r.enabled {
		return nil, fmt.Errorf("%w: %s", ErrHooksDisabled, name)
	}
	hook, exists := r.hooks[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrHookNotRegistered, name)
	}
	return hook, nil
}

func (r *Registry) AccountantDashboards() (AccountantDashboardHook, error) {
	hook, err := r.Require(HookAccountantDashboards)
	if err != nil {
		return nil, err
	}
	return hook.(AccountantDashboardHook), nil
}

func (r *Registry) LockDateWizards() (LockDateWizardHook, error) {
	hook, err := r.Require(HookLockDateWizards)
	if err != nil {
		return nil, err
	}
	return hook.(LockDateWizardHook), nil
}

func (r *Registry) Assets() (AssetHook, error) {
	hook, err := r.Require(HookAssets)
	if err != nil {
		return nil, err
	}
	return hook.(AssetHook), nil
}

func (r *Registry) Budgets() (BudgetHook, error) {
	hook, err := r.Require(HookBudgets)
	if err != nil {
		return nil, err
	}
	return hook.(BudgetHook), nil
}

func (r *Registry) Reports() (ReportHook, error) {
	hook, err := r.Require(HookReports)
	if err != nil {
		return nil, err
	}
	return hook.(ReportHook), nil
}

func (r *Registry) Followup() (FollowupHook, error) {
	hook, err := r.Require(HookFollowup)
	if err != nil {
		return nil, err
	}
	return hook.(FollowupHook), nil
}

func (r *Registry) BankStatementImport() (BankStatementImportHook, error) {
	hook, err := r.Require(HookBankStatementImport)
	if err != nil {
		return nil, err
	}
	return hook.(BankStatementImportHook), nil
}

func (r *Registry) BatchPayments() (BatchPaymentHook, error) {
	hook, err := r.Require(HookBatchPayments)
	if err != nil {
		return nil, err
	}
	return hook.(BatchPaymentHook), nil
}

func (r *Registry) ISO20022SEPA() (ISO20022SEPAHook, error) {
	hook, err := r.Require(HookISO20022SEPA)
	if err != nil {
		return nil, err
	}
	return hook.(ISO20022SEPAHook), nil
}

func (r *Registry) ExternalTaxProviders() (ExternalTaxProviderHook, error) {
	hook, err := r.Require(HookExternalTaxProviders)
	if err != nil {
		return nil, err
	}
	return hook.(ExternalTaxProviderHook), nil
}

func (r *Registry) IntrastatSAFT() (IntrastatSAFTHook, error) {
	hook, err := r.Require(HookIntrastatSAFT)
	if err != nil {
		return nil, err
	}
	return hook.(IntrastatSAFTHook), nil
}

func (r *Registry) InvoiceBankStatementExtraction() (InvoiceBankStatementExtractionHook, error) {
	hook, err := r.Require(HookInvoiceBankStatementExtract)
	if err != nil {
		return nil, err
	}
	return hook.(InvoiceBankStatementExtractionHook), nil
}

func (r *Registry) Loans() (LoanHook, error) {
	hook, err := r.Require(HookLoans)
	if err != nil {
		return nil, err
	}
	return hook.(LoanHook), nil
}

func (r *Registry) Transfers() (TransferHook, error) {
	hook, err := r.Require(HookTransfers)
	if err != nil {
		return nil, err
	}
	return hook.(TransferHook), nil
}

func validateHook(name HookName, hook any) error {
	var ok bool
	switch name {
	case HookAccountantDashboards:
		_, ok = hook.(AccountantDashboardHook)
	case HookLockDateWizards:
		_, ok = hook.(LockDateWizardHook)
	case HookAssets:
		_, ok = hook.(AssetHook)
	case HookBudgets:
		_, ok = hook.(BudgetHook)
	case HookReports:
		_, ok = hook.(ReportHook)
	case HookFollowup:
		_, ok = hook.(FollowupHook)
	case HookBankStatementImport:
		_, ok = hook.(BankStatementImportHook)
	case HookBatchPayments:
		_, ok = hook.(BatchPaymentHook)
	case HookISO20022SEPA:
		_, ok = hook.(ISO20022SEPAHook)
	case HookExternalTaxProviders:
		_, ok = hook.(ExternalTaxProviderHook)
	case HookIntrastatSAFT:
		_, ok = hook.(IntrastatSAFTHook)
	case HookInvoiceBankStatementExtract:
		_, ok = hook.(InvoiceBankStatementExtractionHook)
	case HookLoans:
		_, ok = hook.(LoanHook)
	case HookTransfers:
		_, ok = hook.(TransferHook)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownHook, name)
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrInvalidHook, name)
	}
	provider := hook.(Provider)
	if strings.TrimSpace(provider.ProviderName()) == "" {
		return fmt.Errorf("%w: %s provider name", ErrInvalidHook, name)
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
