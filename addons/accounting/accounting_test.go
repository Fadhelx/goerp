package accounting

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	coreaccounting "gorp/internal/accounting"
	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || !manifest.Application {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.Name != "Invoicing" || manifest.Category != "Accounting/Accounting" {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"base_setup", "onboarding", "product", "analytic", "portal", "digest"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if len(manifest.Data) == 0 || manifest.Data[len(manifest.Data)-1] != GenericChartFixture {
		t.Fatalf("data = %+v", manifest.Data)
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
	for _, name := range []string{
		"uom.uom",
		"product.template",
		"product.product",
		"product.category",
		"account.analytic.account",
		"account.analytic.line",
		"account.analytic.plan",
		"account.analytic.distribution.model",
		"digest.digest",
		"digest.tip",
		"onboarding.onboarding",
		"onboarding.onboarding.step",
		"portal.mixin",
		"portal.wizard",
		"account.account",
		"account.group",
		"account.root",
		"account.journal",
		"account.journal.group",
		"account.incoterms",
		"account.lock_exception",
		"account.move",
		"account.move.line",
		"account.move.reversal",
		"account.payment.register",
		"account.move.send.wizard",
		"account.move.send.batch.wizard",
		"account.tax",
		"account.fiscal.position.account",
		"account.invoice.report",
		"account.automatic.entry.wizard",
		"account.autopost.bills.wizard",
		"account.resequence.wizard",
		"account.secure.entries.wizard",
		"account.merge.wizard",
		"account.merge.wizard.line",
		"account.accrued.orders.wizard",
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	if !reg.Models["account.move.reversal"].Transient {
		t.Fatal("account.move.reversal must be transient")
	}
	if !reg.Models["account.payment.register"].Transient {
		t.Fatal("account.payment.register must be transient")
	}
	if !reg.Models["account.move.send.wizard"].Transient || !reg.Models["account.move.send.batch.wizard"].Transient {
		t.Fatal("account move send wizards must be transient")
	}
	for _, name := range []string{"account.automatic.entry.wizard", "account.autopost.bills.wizard", "account.resequence.wizard", "account.secure.entries.wizard", "account.merge.wizard", "account.merge.wizard.line", "account.accrued.orders.wizard"} {
		if !reg.Models[name].Transient {
			t.Fatalf("%s must be transient", name)
		}
	}
	if !reg.Models["portal.wizard"].Transient || !reg.Models["portal.wizard.user"].Transient {
		t.Fatal("portal wizards must be transient")
	}
}

func TestModelsUseCanonicalAccountingRegistry(t *testing.T) {
	if !reflect.DeepEqual(Models(), coreaccounting.Models()) {
		t.Fatal("addon accounting models diverge from canonical accounting registry")
	}

	byName := map[string]map[string]bool{}
	for _, m := range Models() {
		fields := map[string]bool{}
		for name := range m.Fields {
			fields[name] = true
		}
		byName[m.Name] = fields
	}
	for _, tt := range []struct {
		model string
		field string
	}{
		{"account.account", "internal_group"},
		{"account.account", "include_initial_balance"},
		{"account.account", "placeholder_code"},
		{"account.account", "root_id"},
		{"account.account", "group_id"},
		{"account.account", "tax_ids"},
		{"account.group", "code_prefix_start"},
		{"account.group", "code_prefix_end"},
		{"account.root", "parent_id"},
		{"account.journal.group", "excluded_journal_ids"},
		{"account.incoterms", "code"},
		{"account.lock_exception", "fiscalyear_lock_date"},
		{"account.lock_exception", "purchase_lock_date"},
		{"account.move.line", "account_type"},
		{"account.move.line", "account_internal_group"},
		{"account.move.line", "move_type"},
		{"account.move.line", "amount_residual_currency"},
		{"account.move.line", "payment_id"},
		{"account.move.line", "full_reconcile_id"},
		{"account.move.line", "product_id"},
		{"account.move.line", "product_uom_id"},
		{"account.move.line", "quantity"},
		{"account.move.line", "price_unit"},
		{"account.move.line", "tax_ids"},
		{"account.move", "fiscal_position_id"},
		{"account.tax", "fiscal_position_ids"},
		{"account.tax", "original_tax_ids"},
		{"account.tax", "is_domestic"},
		{"account.fiscal.position", "tax_ids"},
		{"account.move.line", "analytic_distribution"},
		{"account.move.line", "analytic_line_ids"},
		{"account.move.line", "has_invalid_analytics"},
		{"account.move", "invoice_date"},
		{"account.move", "amount_residual_signed"},
		{"account.move", "payment_state"},
		{"account.move", "status_in_payment"},
		{"account.move", "origin_payment_id"},
		{"account.move", "statement_line_id"},
		{"account.move", "matched_payment_ids"},
		{"account.move", "reconciled_payment_ids"},
		{"account.move", "payment_count"},
		{"account.move", "need_cancel_request"},
		{"account.move", "show_reset_to_draft_button"},
		{"account.move", "reversed_entry_id"},
		{"account.move", "sending_data"},
		{"account.move", "is_being_sent"},
		{"account.move", "invoice_pdf_report_id"},
		{"account.move", "invoice_pdf_report_file"},
		{"account.move", "message_main_attachment_id"},
		{"account.move", "access_url"},
		{"account.move", "access_token"},
		{"account.move", "access_warning"},
		{"account.move.reversal", "move_ids"},
		{"account.move.reversal", "new_move_ids"},
		{"account.move.reversal", "available_journal_ids"},
		{"account.payment.register", "line_ids"},
		{"account.payment.register", "payment_date"},
		{"account.payment.register", "payment_method_line_id"},
		{"account.payment.register", "payment_difference_handling"},
		{"account.payment.register", "total_payments_amount"},
		{"account.move.send.wizard", "move_id"},
		{"account.move.send.wizard", "sending_methods"},
		{"account.move.send.wizard", "mail_partner_ids"},
		{"account.move.send.wizard", "subject"},
		{"account.move.send.batch.wizard", "move_ids"},
		{"account.move.send.batch.wizard", "summary_data"},
		{"account.invoice.report", "price_total_currency"},
		{"account.invoice.report", "inventory_value"},
		{"account.fiscal.position", "account_ids"},
		{"account.fiscal.position.account", "account_dest_id"},
		{"account.automatic.entry.wizard", "destination_account_id"},
		{"account.autopost.bills.wizard", "nb_unmodified_bills"},
		{"account.resequence.wizard", "preview_moves"},
		{"account.secure.entries.wizard", "move_to_hash_ids"},
		{"account.merge.wizard", "wizard_line_ids"},
		{"account.merge.wizard.line", "account_has_hashed_entries"},
		{"account.accrued.orders.wizard", "preview_data"},
		{"account.partial.reconcile", "debit_amount_currency"},
		{"account.partial.reconcile", "credit_amount_currency"},
		{"account.move", "inalterable_hash"},
		{"account.payment", "move_id"},
		{"account.payment", "is_reconciled"},
		{"account.payment", "is_matched"},
		{"account.payment", "is_sent"},
		{"account.payment", "invoice_ids"},
		{"account.payment", "reconciled_invoice_ids"},
		{"account.payment", "reconciled_bill_ids"},
		{"account.payment", "need_cancel_request"},
		{"account.payment", "partner_type"},
		{"product.template", "taxes_id"},
		{"product.template", "supplier_taxes_id"},
		{"product.template", "property_account_income_id"},
		{"product.template", "property_account_expense_id"},
		{"product.product", "product_tmpl_id"},
		{"product.category", "property_account_income_categ_id"},
		{"product.category", "property_account_expense_categ_id"},
		{"account.analytic.plan", "applicability_ids"},
		{"account.analytic.applicability", "business_domain"},
		{"account.analytic.account", "plan_id"},
		{"account.analytic.line", "move_line_id"},
		{"account.analytic.distribution.model", "analytic_distribution"},
		{"digest.digest", "kpi_account_total_revenue"},
		{"digest.tip", "tip_description"},
		{"onboarding.onboarding", "step_ids"},
		{"onboarding.onboarding.step", "panel_step_open_action_name"},
		{"portal.mixin", "access_token"},
		{"portal.wizard", "user_ids"},
	} {
		if !byName[tt.model][tt.field] {
			t.Fatalf("missing canonical field %s.%s", tt.model, tt.field)
		}
	}
}

func TestInstallChart(t *testing.T) {
	env := accountingEnv(t)
	ids, err := LoadGenericChart(env)
	if err != nil {
		t.Fatal(err)
	}

	for _, externalID := range []string{
		"account.account_generic_receivable",
		"account.account_generic_payable",
		"account.account_generic_bank",
		"account.account_generic_cash",
		"account.account_generic_income",
		"account.account_generic_expenses",
		"account.account_generic_liabilities",
		"account.account_generic_equity",
		"account.account_generic_current_year_earnings",
		"account.journal_generic_sales",
		"account.journal_generic_purchases",
		"account.journal_generic_bank",
		"account.journal_generic_cash",
		"account.journal_generic_misc",
		"account.journal_generic_tax_return",
	} {
		if ids[externalID].ResID == 0 {
			t.Fatalf("missing external id %s", externalID)
		}
	}

	receivable := ids["account.account_generic_receivable"]
	rows, err := env.Model("account.account").Browse(receivable.ResID).Read("code", "name", "account_type", "internal_group", "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["code"] != "110000" || rows[0]["account_type"] != "asset_receivable" || rows[0]["reconcile"] != true {
		t.Fatalf("receivable = %+v", rows[0])
	}

	salesJournal := ids["account.journal_generic_sales"]
	income := ids["account.account_generic_income"]
	journalRows, err := env.Model("account.journal").Browse(salesJournal.ResID).Read("code", "type", "default_account_id")
	if err != nil {
		t.Fatal(err)
	}
	if journalRows[0]["code"] != "SLS" || journalRows[0]["type"] != "sale" || journalRows[0]["default_account_id"] != income.ResID {
		t.Fatalf("sales journal = %+v", journalRows[0])
	}
}

func TestAccountingManifestDataFilesLoad(t *testing.T) {
	manifest := Manifest()
	for _, path := range manifest.Data {
		assertManifestDataFile(t, path)
	}

	env := accountingFixtureEnv(t)
	externalIDs := map[string]data.ExternalID{}
	if err := data.LoadModelMetadata(env, base.Manifest().TechnicalName, base.Models(), externalIDs); err != nil {
		t.Fatal(err)
	}
	baseLoader := data.NewLoaderWithExternalIDs(env, base.Manifest().TechnicalName, externalIDs)
	loadManifestDataFilesFrom(t, baseLoader, filepath.Join("..", "..", "internal", "base"), base.Manifest().Data)
	if err := data.LoadModelMetadata(env, ModuleName, Models(), externalIDs); err != nil {
		t.Fatal(err)
	}
	loader := data.NewLoaderWithExternalIDs(env, ModuleName, externalIDs)
	loadManifestDataFiles(t, loader, manifest.Data)
	ids := loader.ExternalIDs()
	for _, externalID := range []string{
		"base.group_user",
		"base.group_portal",
		"account.group_account_invoice",
		"account.group_account_manager",
		"account.model_account_move",
		"account.model_account_group",
		"account.model_account_journal_group",
		"account.model_account_incoterms",
		"account.model_account_lock_exception",
		"account.model_account_invoice_report",
		"account.model_account_move_reversal",
		"account.model_account_payment_register",
		"account.model_account_move_send_wizard",
		"account.model_account_move_send_batch_wizard",
		"account.access_account_move_uinvoice",
		"account.access_account_move_reversal",
		"account.access_account_payment_register",
		"account.access_account_move_send_wizard",
		"account.access_account_move_send_batch_wizard",
		"account.access_account_group_manager",
		"account.access_account_journal_group_manager",
		"account.access_account_incoterms_user",
		"account.access_account_lock_exception_user",
		"account.access_account_lock_exception_manager",
		"account.access_account_invoice_report_invoice",
		"account.access_account_automatic_entry_wizard",
		"account.account_move_send_single_rule_group_invoice",
		"account.account_move_send_batch_rule_group_invoice",
		"account.account_payment_term_immediate",
		"account.email_template_edi_invoice",
		"account.seq_account_payment",
		"account.action_move_out_invoice",
		"account.action_view_account_move_reversal",
		"account.action_account_payment_register",
		"account.action_account_move_send_wizard",
		"account.action_account_move_send_batch_wizard",
		"account.action_account_group",
		"account.action_account_journal_group",
		"account.action_account_incoterms",
		"account.action_account_lock_exception",
		"account.action_account_invoice_report_all",
		"account.incoterm_EXW",
		"account.incoterm_FOB",
		"account.view_account_form",
		"account.view_account_group_tree",
		"account.view_account_journal_group_tree",
		"account.account_incoterms_view_tree",
		"account.account_lock_exception_view_tree",
		"account.view_account_invoice_report_tree",
		"account.account_automatic_entry_wizard_form",
		"account.view_account_move_reversal",
		"account.view_account_payment_register_form",
		"account.account_move_send_wizard_form",
		"account.account_move_send_batch_wizard_form",
		"uom.group_uom",
		"uom.product_uom_categ_unit",
		"uom.product_uom_unit",
		"product.product_category_all",
		"product.product_product_1_product_template",
		"product.product_product_1",
		"product.product_template_form_view",
		"product.product_category_action_form",
		"analytic.group_analytic_accounting",
		"analytic.analytic_plan_default",
		"analytic.analytic_account_default",
		"analytic.analytic_distribution_model_default",
		"analytic.view_account_analytic_account_form",
		"analytic.account_analytic_line_action_entries",
		"analytic.account_analytic_plan_action",
		"digest.digest_digest_default",
		"account.digest_tip_account_0",
		"account.digest_tip_account_1",
		"digest.digest_digest_view_form",
		"portal.portal_layout",
		"portal.portal_my_home",
		"account.onboarding_onboarding_step_company_data",
		"account.onboarding_onboarding_step_chart_of_accounts",
		"account.onboarding_onboarding_account_dashboard",
		"base_setup.res_config_settings_view_form",
		"account.res_config_settings_view_form",
		"account.action_account_config",
		"account.menu_account_config_settings",
		"account.menu_board_journal_1",
		"account.generic_tax_report",
		"account.account_generic_receivable",
	} {
		if ids[externalID].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", externalID, ids)
		}
	}

	rows, err := env.Model("ir.model.access").Browse(ids["account.access_account_move_uinvoice"].ResID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["model_id"] != ids["account.model_account_move"].ResID ||
		rows[0]["group_id"] != ids["account.group_account_invoice"].ResID ||
		rows[0]["perm_read"] != true ||
		rows[0]["perm_write"] != true ||
		rows[0]["perm_create"] != true ||
		rows[0]["perm_unlink"] != true {
		t.Fatalf("invoice ACL = %+v", rows[0])
	}
	reversalACLRows, err := env.Model("ir.model.access").Browse(ids["account.access_account_move_reversal"].ResID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if reversalACLRows[0]["model_id"] != ids["account.model_account_move_reversal"].ResID ||
		reversalACLRows[0]["group_id"] != ids["account.group_account_invoice"].ResID ||
		reversalACLRows[0]["perm_read"] != true ||
		reversalACLRows[0]["perm_write"] != true ||
		reversalACLRows[0]["perm_create"] != true ||
		reversalACLRows[0]["perm_unlink"] != false {
		t.Fatalf("reversal ACL = %+v", reversalACLRows[0])
	}
	paymentRegisterACLRows, err := env.Model("ir.model.access").Browse(ids["account.access_account_payment_register"].ResID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if paymentRegisterACLRows[0]["model_id"] != ids["account.model_account_payment_register"].ResID ||
		paymentRegisterACLRows[0]["group_id"] != ids["account.group_account_invoice"].ResID ||
		paymentRegisterACLRows[0]["perm_read"] != true ||
		paymentRegisterACLRows[0]["perm_write"] != true ||
		paymentRegisterACLRows[0]["perm_create"] != true ||
		paymentRegisterACLRows[0]["perm_unlink"] != false {
		t.Fatalf("payment register ACL = %+v", paymentRegisterACLRows[0])
	}
	sendWizardACLRows, err := env.Model("ir.model.access").Browse(ids["account.access_account_move_send_wizard"].ResID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if sendWizardACLRows[0]["model_id"] != ids["account.model_account_move_send_wizard"].ResID ||
		sendWizardACLRows[0]["group_id"] != ids["account.group_account_invoice"].ResID ||
		sendWizardACLRows[0]["perm_unlink"] != true {
		t.Fatalf("send wizard ACL = %+v", sendWizardACLRows[0])
	}
	viewRows, err := env.Model("ir.ui.view").Browse(ids["account.view_account_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := viewRows[0]["arch"].(string)
	if viewRows[0]["model"] != "account.account" || !strings.Contains(arch, `<field name="code"/>`) {
		t.Fatalf("account view = %+v", viewRows[0])
	}
	reversalViewRows, err := env.Model("ir.ui.view").Browse(ids["account.view_account_move_reversal"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	reversalArch, _ := reversalViewRows[0]["arch"].(string)
	if reversalViewRows[0]["model"] != "account.move.reversal" || !strings.Contains(reversalArch, `name="refund_moves"`) || !strings.Contains(reversalArch, `name="modify_moves"`) {
		t.Fatalf("reversal view = %+v", reversalViewRows[0])
	}
	paymentRegisterViewRows, err := env.Model("ir.ui.view").Browse(ids["account.view_account_payment_register_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	paymentRegisterArch, _ := paymentRegisterViewRows[0]["arch"].(string)
	if paymentRegisterViewRows[0]["model"] != "account.payment.register" || !strings.Contains(paymentRegisterArch, `name="action_create_payments"`) {
		t.Fatalf("payment register view = %+v", paymentRegisterViewRows[0])
	}
	sendViewRows, err := env.Model("ir.ui.view").Browse(ids["account.account_move_send_wizard_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	sendArch, _ := sendViewRows[0]["arch"].(string)
	if sendViewRows[0]["model"] != "account.move.send.wizard" || !strings.Contains(sendArch, `name="action_send_and_print"`) {
		t.Fatalf("send view = %+v", sendViewRows[0])
	}
	sendBatchViewRows, err := env.Model("ir.ui.view").Browse(ids["account.account_move_send_batch_wizard_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	if sendBatchViewRows[0]["model"] != "account.move.send.batch.wizard" {
		t.Fatalf("send batch view = %+v", sendBatchViewRows[0])
	}
	productRows, err := env.Model("product.product").Browse(ids["product.product_product_1"].ResID).Read("product_tmpl_id", "categ_id", "uom_id", "display_name")
	if err != nil {
		t.Fatal(err)
	}
	if productRows[0]["product_tmpl_id"] != ids["product.product_product_1_product_template"].ResID ||
		productRows[0]["categ_id"] != ids["product.product_category_all"].ResID ||
		productRows[0]["uom_id"] != ids["uom.product_uom_unit"].ResID {
		t.Fatalf("product anchor = %+v", productRows[0])
	}
	analyticRows, err := env.Model("account.analytic.account").Browse(ids["analytic.analytic_account_default"].ResID).Read("plan_id", "root_plan_id", "active")
	if err != nil {
		t.Fatal(err)
	}
	if analyticRows[0]["plan_id"] != ids["analytic.analytic_plan_default"].ResID || analyticRows[0]["root_plan_id"] != ids["analytic.analytic_plan_default"].ResID || analyticRows[0]["active"] != true {
		t.Fatalf("analytic anchor = %+v", analyticRows[0])
	}
	digestRows, err := env.Model("digest.digest").Browse(ids["digest.digest_digest_default"].ResID).Read("kpi_account_total_revenue", "periodicity")
	if err != nil {
		t.Fatal(err)
	}
	if digestRows[0]["kpi_account_total_revenue"] != true || digestRows[0]["periodicity"] != "weekly" {
		t.Fatalf("digest anchor = %+v", digestRows[0])
	}
	onboardingRows, err := env.Model("onboarding.onboarding").Browse(ids["account.onboarding_onboarding_account_dashboard"].ResID).Read("route_name", "step_ids")
	if err != nil {
		t.Fatal(err)
	}
	if onboardingRows[0]["route_name"] != "account_dashboard" || len(int64Slice(onboardingRows[0]["step_ids"])) != 3 {
		t.Fatalf("onboarding anchor = %+v", onboardingRows[0])
	}
	settingsRows, err := env.Model("ir.actions.act_window").Browse(ids["account.action_account_config"].ResID).Read("res_model", "view_id", "target")
	if err != nil {
		t.Fatal(err)
	}
	if settingsRows[0]["res_model"] != "res.config.settings" || settingsRows[0]["view_id"] != ids["account.res_config_settings_view_form"].ResID {
		t.Fatalf("settings action = %+v", settingsRows[0])
	}
	reversalActionRows, err := env.Model("ir.actions.act_window").Browse(ids["account.action_view_account_move_reversal"].ResID).Read("res_model", "view_id", "target", "binding_model_id", "binding_view_types", "group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if reversalActionRows[0]["res_model"] != "account.move.reversal" ||
		reversalActionRows[0]["view_id"] != ids["account.view_account_move_reversal"].ResID ||
		reversalActionRows[0]["target"] != "new" ||
		reversalActionRows[0]["binding_model_id"] != ids["account.model_account_move"].ResID ||
		reversalActionRows[0]["binding_view_types"] != "list,kanban" {
		t.Fatalf("reversal action = %+v", reversalActionRows[0])
	}
	paymentRegisterActionRows, err := env.Model("ir.actions.act_window").Browse(ids["account.action_account_payment_register"].ResID).Read("res_model", "view_id", "target", "binding_model_id", "binding_view_types", "group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if paymentRegisterActionRows[0]["res_model"] != "account.payment.register" ||
		paymentRegisterActionRows[0]["view_id"] != ids["account.view_account_payment_register_form"].ResID ||
		paymentRegisterActionRows[0]["target"] != "new" ||
		paymentRegisterActionRows[0]["binding_model_id"] != ids["account.model_account_move"].ResID ||
		paymentRegisterActionRows[0]["binding_view_types"] != "list,form" {
		t.Fatalf("payment register action = %+v", paymentRegisterActionRows[0])
	}
	sendActionRows, err := env.Model("ir.actions.act_window").Browse(ids["account.action_account_move_send_wizard"].ResID).Read("res_model", "view_id", "target", "binding_model_id", "binding_view_types", "group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if sendActionRows[0]["res_model"] != "account.move.send.wizard" ||
		sendActionRows[0]["view_id"] != ids["account.account_move_send_wizard_form"].ResID ||
		sendActionRows[0]["target"] != "new" ||
		sendActionRows[0]["binding_model_id"] != ids["account.model_account_move"].ResID ||
		sendActionRows[0]["binding_view_types"] != "form" {
		t.Fatalf("send action = %+v", sendActionRows[0])
	}
	menuRows, err := env.Model("ir.ui.menu").Browse(ids["account.menu_finance_receivables"].ResID).Read("parent_id", "action")
	if err != nil {
		t.Fatal(err)
	}
	if menuRows[0]["parent_id"] != ids["account.menu_board_journal_1"].ResID ||
		menuRows[0]["action"] != "ir.actions.act_window,action_move_out_invoice" {
		t.Fatalf("receivables menu = %+v", menuRows[0])
	}
	invoiceReportRows, err := env.Model("ir.actions.report").Browse(ids["account.account_invoices"].ResID).Read("report_type", "report_file", "print_report_name", "binding_type", "domain", "is_invoice_report")
	if err != nil {
		t.Fatal(err)
	}
	if invoiceReportRows[0]["report_type"] != "qweb-pdf" ||
		invoiceReportRows[0]["report_file"] != "account.report_invoice" ||
		invoiceReportRows[0]["binding_type"] != "report" ||
		!xmlTruth(invoiceReportRows[0]["is_invoice_report"]) ||
		!strings.Contains(invoiceReportRows[0]["domain"].(string), "out_invoice") {
		t.Fatalf("invoice report = %+v", invoiceReportRows[0])
	}
	reportRows, err := env.Model("account.report.column").Browse(ids["account.generic_tax_report_column_net"].ResID).Read("name", "report_id")
	if err != nil {
		t.Fatal(err)
	}
	if reportRows[0]["name"] != "Net" || reportRows[0]["report_id"] != ids["account.generic_tax_report"].ResID {
		t.Fatalf("report column = %+v", reportRows[0])
	}
	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	if len(engine.ACLs) == 0 || len(engine.Rules) == 0 {
		t.Fatalf("persisted security ACLs=%d rules=%d", len(engine.ACLs), len(engine.Rules))
	}
}

func TestAccountingReportNestedXMLLoads(t *testing.T) {
	env := accountingFixtureEnv(t)
	loader := data.NewLoader(env, ModuleName)
	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="nested_account_report" model="account.report">
    <field name="name">Nested Report</field>
    <field name="column_ids">
      <record id="nested_account_report_column" model="account.report.column">
        <field name="name">Balance</field>
        <field name="expression_label">balance</field>
      </record>
    </field>
    <field name="line_ids">
      <record id="nested_account_report_line" model="account.report.line">
        <field name="name">Root Line</field>
        <field name="children_ids">
          <record id="nested_account_report_child_line" model="account.report.line">
            <field name="name">Child Line</field>
            <field name="code">CHILD</field>
            <field name="expression_ids">
              <record id="nested_account_report_expression" model="account.report.expression">
                <field name="label">balance</field>
                <field name="engine">aggregation</field>
                <field name="formula">CHILD.balance</field>
              </record>
            </field>
          </record>
        </field>
      </record>
    </field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	reportID := ids[ModuleName+".nested_account_report"].ResID
	columnID := ids[ModuleName+".nested_account_report_column"].ResID
	lineID := ids[ModuleName+".nested_account_report_line"].ResID
	childLineID := ids[ModuleName+".nested_account_report_child_line"].ResID
	expressionID := ids[ModuleName+".nested_account_report_expression"].ResID
	assertAccountingField(t, env, "account.report", reportID, "column_ids", []int64{columnID})
	assertAccountingField(t, env, "account.report", reportID, "line_ids", []int64{lineID})
	assertAccountingField(t, env, "account.report.column", columnID, "report_id", reportID)
	assertAccountingField(t, env, "account.report.line", lineID, "report_id", reportID)
	assertAccountingField(t, env, "account.report.line", lineID, "children_ids", []int64{childLineID})
	assertAccountingField(t, env, "account.report.line", childLineID, "parent_id", lineID)
	assertAccountingField(t, env, "account.report.line", childLineID, "expression_ids", []int64{expressionID})
	assertAccountingField(t, env, "account.report.expression", expressionID, "report_line_id", childLineID)
}

func TestAccountingSecurity(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Companies[1] = security.Company{ID: 1, Name: "Main", Active: true}
	engine.Companies[2] = security.Company{ID: 2, Name: "Branch", ParentID: 1, Active: true}
	engine.Companies[3] = security.Company{ID: 3, Name: "Other", Active: true}
	engine.Users[10] = security.User{ID: 10, Login: "auditor", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupReadOnlyAuditor}}
	engine.Users[20] = security.User{ID: 20, Login: "accountant", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupAccountant}}
	engine.Users[30] = security.User{ID: 30, Login: "admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2, 3}, GroupIDs: []int64{GroupAdviserAdmin}}
	engine.Users[40] = security.User{ID: 40, Login: "invoice", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupInvoiceUser}}
	engine.Users[50] = security.User{ID: 50, Login: "basic", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupBasicAccounting}}
	engine.Users[60] = security.User{ID: 60, Login: "portal", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, PartnerID: 701, CommercialPartnerID: 700, GroupIDs: []int64{GroupBasePortal}}
	engine.Hierarchies["res.partner"] = map[int64]int64{
		700: 0,
		701: 700,
		702: 700,
		800: 0,
	}

	for _, groupID := range []int64{GroupBaseUser, GroupBasePortal, GroupBaseSystem, GroupInvoiceUser, GroupBasicAccounting, GroupReadOnlyAccounting, GroupBillingUser, GroupAccountant, GroupAdviserAdmin, GroupReadOnlyAuditor, GroupAccountSecured, GroupCashRounding, GroupValidateBankAccount} {
		if engine.Groups[groupID].ID == 0 {
			t.Fatalf("missing group %d", groupID)
		}
	}
	if !engine.EffectiveGroupIDs(20)[GroupAccountInvoice] || !engine.EffectiveGroupIDs(20)[GroupAccountReadonly] {
		t.Fatalf("account user closure = %+v", engine.EffectiveGroupIDs(20))
	}
	if !engine.EffectiveGroupIDs(30)[GroupAccountInvoice] || engine.EffectiveGroupIDs(30)[GroupAccountUser] {
		t.Fatalf("manager closure = %+v", engine.EffectiveGroupIDs(30))
	}

	if err := engine.Check(record.Context{UserID: 10}, "account.move", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, "account.move", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected read-only write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, "account.move", record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 30}, "account.move", record.OpUnlink, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, "account.journal", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected invoice journal write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 50}, "account.bank.statement", record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 60}, "account.move", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 60}, "account.move", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected portal move write denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 40}, "account.bank.statement", record.OpCreate, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected invoice bank statement create denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 30}, "account.code.mapping", record.OpCreate, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected manager code mapping create denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, "account.account.tag", record.OpWrite, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 40}, "account.account.tag", record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected invoice account tag write denied, got %v", err)
	}

	assertRule(t, engine, 20, "account.move", map[string]any{"company_id": int64(2)}, true)
	assertRule(t, engine, 20, "account.move", map[string]any{"company_id": int64(3)}, false)
	assertRule(t, engine, 20, "account.bank.statement", map[string]any{"company_id": nil}, true)
	assertRule(t, engine, 20, "account.bank.statement", map[string]any{"company_id": int64(2)}, true)
	assertRule(t, engine, 20, "account.bank.statement", map[string]any{"company_id": int64(3)}, false)
	assertRule(t, engine, 20, "account.payment.term", map[string]any{"company_id": nil}, true)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "out_invoice", "partner_id": int64(700)}, true)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "out_refund", "partner_id": int64(702)}, true)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "in_invoice", "partner_id": int64(701)}, true)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "out_receipt", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "cancel", "move_type": "out_invoice", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "draft", "move_type": "out_invoice", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "entry", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move", map[string]any{"company_id": int64(1), "state": "posted", "move_type": "out_invoice", "partner_id": int64(800)}, false)
	assertRule(t, engine, 60, "account.move.line", map[string]any{"company_id": int64(1), "parent_state": "posted", "move_type": "out_invoice", "partner_id": int64(702)}, true)
	assertRule(t, engine, 60, "account.move.line", map[string]any{"company_id": int64(1), "parent_state": "cancel", "move_type": "out_invoice", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move.line", map[string]any{"company_id": int64(1), "parent_state": "posted", "move_type": "out_receipt", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move.line", map[string]any{"company_id": int64(1), "parent_state": "posted", "move_type": "entry", "partner_id": int64(700)}, false)
	assertRule(t, engine, 60, "account.move.line", map[string]any{"company_id": int64(1), "parent_state": "posted", "move_type": "out_invoice", "partner_id": int64(800)}, false)

	definitionsByName := map[string]RuleDefinition{}
	for _, definition := range SecurityRuleDefinitions() {
		definitionsByName[definition.Name] = definition
	}
	for _, name := range []string{"account_move_comp_rule", "account_move_line_comp_rule", "account_payment_comp_rule", "account_bank_statement_comp_rule", "account_group_comp_rule", "account_journal_group_comp_rule", "account_invoice_report_comp_rule", "account_invoice_rule_portal", "account_invoice_line_rule_portal", "account_move_rule_group_readonly", "account_move_rule_group_invoice"} {
		if definitionsByName[name].Name == "" {
			t.Fatalf("missing source-named rule %s", name)
		}
	}
	if definitionsByName["account_invoice_rule_portal"].Rule.DomainText != "[('state', 'not in', ('cancel', 'draft')), ('move_type', 'in', ('out_invoice', 'out_refund', 'in_invoice', 'in_refund')), ('partner_id', 'child_of', [user.commercial_partner_id.id])]" {
		t.Fatalf("portal move rule = %+v", definitionsByName["account_invoice_rule_portal"])
	}
	if definitionsByName["account_invoice_line_rule_portal"].Rule.DomainText != "[('parent_state', 'not in', ('cancel', 'draft')), ('move_type', 'in', ('out_invoice', 'out_refund', 'in_invoice', 'in_refund')), ('partner_id', 'child_of', [user.commercial_partner_id.id])]" {
		t.Fatalf("portal line rule = %+v", definitionsByName["account_invoice_line_rule_portal"])
	}
	if definitionsByName["account_move_comp_rule"].Rule.DomainText != "[('company_id', 'in', company_ids)]" {
		t.Fatalf("move rule = %+v", definitionsByName["account_move_comp_rule"])
	}
	readonlyRule := definitionsByName["account_move_rule_group_readonly"].Rule
	if readonlyRule.PermWrite || readonlyRule.PermCreate || readonlyRule.PermUnlink || readonlyRule.Domain.Kind != domain.All {
		t.Fatalf("readonly rule = %+v", readonlyRule)
	}
}

func TestAccountingACLMatrixUsesSourceNames(t *testing.T) {
	byName := map[string]security.ACL{}
	for _, acl := range SecurityACLs() {
		if acl.Name == "" {
			t.Fatalf("unnamed ACL: %+v", acl)
		}
		if _, exists := byName[acl.Name]; exists {
			t.Fatalf("duplicate ACL name %s", acl.Name)
		}
		byName[acl.Name] = acl
	}
	assertACL(t, byName, "access_account_move_manager", "account.move", GroupAccountManager, true, false, false, false)
	assertACL(t, byName, "access_account_move_uinvoice", "account.move", GroupAccountInvoice, true, true, true, true)
	assertACL(t, byName, "access_account_move_reversal", "account.move.reversal", GroupAccountInvoice, true, true, true, false)
	assertACL(t, byName, "access_account_payment_register", "account.payment.register", GroupAccountInvoice, true, true, true, false)
	assertACL(t, byName, "access_account_move_send_wizard", "account.move.send.wizard", GroupAccountInvoice, true, true, true, true)
	assertACL(t, byName, "access_account_move_send_batch_wizard", "account.move.send.batch.wizard", GroupAccountInvoice, true, true, true, true)
	assertACL(t, byName, "access_account_reconcile_model_billing", "account.reconcile.model", GroupAccountInvoice, true, false, true, false)
	assertACL(t, byName, "access_account_journal_manager", "account.journal", GroupAccountManager, true, true, true, true)
	assertACL(t, byName, "access_account_group_manager", "account.group", GroupAccountManager, true, true, true, true)
	assertACL(t, byName, "access_account_journal_group_manager", "account.journal.group", GroupAccountManager, true, true, true, true)
	assertACL(t, byName, "access_account_incoterms_user", "account.incoterms", GroupBaseUser, true, false, false, false)
	assertACL(t, byName, "access_account_lock_exception_user", "account.lock_exception", GroupBaseUser, true, false, false, false)
	assertACL(t, byName, "access_account_lock_exception_manager", "account.lock_exception", GroupAccountManager, true, false, true, false)
	assertACL(t, byName, "access_account_invoice_report_invoice", "account.invoice.report", GroupAccountInvoice, true, false, false, false)
	assertACL(t, byName, "access_account_automatic_entry_wizard", "account.automatic.entry.wizard", GroupAccountInvoice, true, true, true, false)
	assertACL(t, byName, "access_account_merge_wizard_line", "account.merge.wizard.line", GroupAccountManager, true, true, true, false)
	assertACL(t, byName, "access_account_payment_method", "account.payment.method", GroupAccountInvoice, true, true, false, true)
	assertACL(t, byName, "access_account_report_column_readonly", "account.report.column", GroupAccountReadonly, true, false, false, false)
	assertACL(t, byName, "access_account_account_type_manager_local", "account.account.type", GroupAccountManager, true, true, true, true)
}

func assertAccountingField(t *testing.T, env *record.Env, modelName string, id int64, fieldName string, want any) {
	t.Helper()
	rows, err := env.Model(modelName).Browse(id).Read(fieldName)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows[0][fieldName], want) {
		t.Fatalf("%s.%s = %#v, want %#v", modelName, fieldName, rows[0][fieldName], want)
	}
}

func accountingEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	if err := RegisterRecordModels(reg); err != nil {
		t.Fatal(err)
	}
	return record.NewEnv(reg, record.Context{})
}

func accountingFixtureEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range Models() {
		if _, exists := reg.Model(m.Name); exists {
			continue
		}
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

func loadManifestDataFiles(t *testing.T, loader *data.Loader, paths []string) {
	t.Helper()
	loadManifestDataFilesFrom(t, loader, "", paths)
}

func loadManifestDataFilesFrom(t *testing.T, loader *data.Loader, baseDir string, paths []string) {
	t.Helper()
	if baseDir != "" {
		loader.SetBaseDir(baseDir)
	} else {
		loader.SetBaseDir(".")
	}
	for _, path := range paths {
		fullPath := path
		if baseDir != "" {
			fullPath = filepath.Join(baseDir, path)
		}
		file, err := os.Open(filepath.Clean(fullPath))
		if err != nil {
			t.Fatalf("open %s: %v", fullPath, err)
		}
		switch filepath.Ext(fullPath) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			modelName := strings.TrimSuffix(filepath.Base(fullPath), ".csv")
			err = loader.LoadCSV(modelName, file)
		default:
			err = nil
		}
		closeErr := file.Close()
		if err != nil {
			t.Fatalf("load %s: %v", fullPath, err)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", fullPath, closeErr)
		}
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
		var doc struct {
			XMLName xml.Name `xml:"odoo"`
		}
		if err := xml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if doc.XMLName.Local != "odoo" {
			t.Fatalf("%s root = %s", path, doc.XMLName.Local)
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

func xmlTruth(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	default:
		return false
	}
}

func int64Slice(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case int64:
				out = append(out, value)
			case int:
				out = append(out, int64(value))
			case float64:
				out = append(out, int64(value))
			}
		}
		return out
	default:
		return nil
	}
}

func assertRule(t *testing.T, engine *security.Engine, userID int64, modelName string, row map[string]any, want bool) {
	t.Helper()
	ok, err := engine.AllowedByRecordRules(userID, modelName, record.OpRead, row)
	if err != nil {
		t.Fatal(err)
	}
	if ok != want {
		t.Fatalf("rule(%s, %+v) = %v, want %v", modelName, row, ok, want)
	}
}

func assertACL(t *testing.T, byName map[string]security.ACL, name string, modelName string, groupID int64, read bool, write bool, create bool, unlink bool) {
	t.Helper()
	acl, ok := byName[name]
	if !ok {
		t.Fatalf("missing ACL %s", name)
	}
	if acl.Model != modelName || acl.GroupID != groupID || acl.PermRead != read || acl.PermWrite != write || acl.PermCreate != create || acl.PermUnlink != unlink {
		t.Fatalf("%s = %+v", name, acl)
	}
}
