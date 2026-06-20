package accounting_enterprise_hooks

import (
	"context"
	"time"
)

type HookName string

const (
	HookAccountantDashboards        HookName = "accountant_dashboards"
	HookLockDateWizards             HookName = "lock_date_wizards"
	HookAssets                      HookName = "assets"
	HookBudgets                     HookName = "budgets"
	HookReports                     HookName = "reports"
	HookFollowup                    HookName = "followup"
	HookBankStatementImport         HookName = "bank_statement_import"
	HookBatchPayments               HookName = "batch_payments"
	HookISO20022SEPA                HookName = "iso20022_sepa"
	HookExternalTaxProviders        HookName = "external_tax_providers"
	HookIntrastatSAFT               HookName = "intrastat_saft"
	HookInvoiceBankStatementExtract HookName = "invoice_bank_statement_extraction"
	HookLoans                       HookName = "loans"
	HookTransfers                   HookName = "transfers"
)

var requiredHookNames = []HookName{
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

func RequiredHookNames() []HookName {
	return append([]HookName(nil), requiredHookNames...)
}

func IsRequiredHookName(name HookName) bool {
	for _, required := range requiredHookNames {
		if required == name {
			return true
		}
	}
	return false
}

type Provider interface {
	ProviderName() string
}

type Scope struct {
	CompanyID   int64
	UserID      int64
	CountryCode string
	Locale      string
}

type DateRange struct {
	From time.Time
	To   time.Time
}

type Request struct {
	Scope     Scope
	DateRange DateRange
	Payload   map[string]any
}

type Response struct {
	Entries []Entry
	Payload map[string]any
}

type Entry struct {
	Key   string
	Label string
	Value any
}

type AccountantDashboardHook interface {
	Provider
	BuildAccountantDashboard(context.Context, Request) (Response, error)
}

type LockDateWizardHook interface {
	Provider
	PrepareLockDateWizard(context.Context, Request) (Response, error)
}

type AssetHook interface {
	Provider
	PlanAssetLifecycle(context.Context, Request) (Response, error)
}

type BudgetHook interface {
	Provider
	EvaluateBudget(context.Context, Request) (Response, error)
}

type ReportHook interface {
	Provider
	RenderAccountReport(context.Context, Request) (Response, error)
}

type FollowupHook interface {
	Provider
	PrepareFollowup(context.Context, Request) (Response, error)
}

type BankStatementImportHook interface {
	Provider
	ImportBankStatement(context.Context, Request) (Response, error)
}

type BatchPaymentHook interface {
	Provider
	PrepareBatchPayment(context.Context, Request) (Response, error)
}

type ISO20022SEPAHook interface {
	Provider
	BuildISO20022SEPAFile(context.Context, Request) (Response, error)
}

type ExternalTaxProviderHook interface {
	Provider
	ComputeExternalTax(context.Context, Request) (Response, error)
}

type IntrastatSAFTHook interface {
	Provider
	BuildIntrastatSAFTExport(context.Context, Request) (Response, error)
}

type InvoiceBankStatementExtractionHook interface {
	Provider
	ExtractInvoiceOrBankStatement(context.Context, Request) (Response, error)
}

type LoanHook interface {
	Provider
	PlanLoanLifecycle(context.Context, Request) (Response, error)
}

type TransferHook interface {
	Provider
	PrepareTransfer(context.Context, Request) (Response, error)
}
