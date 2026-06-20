package accounting

import (
	"gorp/internal/domain"
	"gorp/internal/security"
)

const (
	GroupBaseUser   int64 = 1
	GroupBasePortal int64 = 2
	GroupBaseSystem int64 = 3

	GroupAccountReadonly     int64 = 6101
	GroupAccountInvoice      int64 = 6102
	GroupAccountBasic        int64 = 6103
	GroupBillingUser         int64 = 6104
	GroupAccountUser         int64 = 6105
	GroupAccountManager      int64 = 6106
	GroupReadOnlyAuditor     int64 = 6107
	GroupAccountSecured      int64 = 6108
	GroupCashRounding        int64 = 6109
	GroupValidateBankAccount int64 = 6110
	GroupReadOnlyAccounting        = GroupAccountReadonly
	GroupInvoiceUser               = GroupAccountInvoice
	GroupBasicAccounting           = GroupAccountBasic
	GroupAccountant                = GroupAccountUser
	GroupAdviserAdmin              = GroupAccountManager
)

type RuleDefinition struct {
	Name string
	Rule security.Rule
}

func ApplySecurity(engine *security.Engine) {
	for _, group := range SecurityGroups() {
		engine.Groups[group.ID] = group
	}
	engine.ACLs = append(engine.ACLs, SecurityACLs()...)
	for _, rule := range SecurityRuleDefinitions() {
		engine.Rules = append(engine.Rules, rule.Rule)
	}
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupBaseUser, Name: "base.group_user"},
		{ID: GroupBasePortal, Name: "base.group_portal"},
		{ID: GroupBaseSystem, Name: "base.group_system", ImpliedIDs: []int64{GroupBaseUser}},
		{ID: GroupAccountReadonly, Name: "account.group_account_readonly", ImpliedIDs: []int64{GroupBaseUser}},
		{ID: GroupAccountInvoice, Name: "account.group_account_invoice", ImpliedIDs: []int64{GroupBaseUser}},
		{ID: GroupAccountBasic, Name: "account.group_account_basic", ImpliedIDs: []int64{GroupAccountInvoice}},
		{ID: GroupAccountUser, Name: "account.group_account_user", ImpliedIDs: []int64{GroupAccountBasic, GroupAccountReadonly}},
		{ID: GroupAccountManager, Name: "account.group_account_manager", ImpliedIDs: []int64{GroupAccountInvoice}},
		{ID: GroupAccountSecured, Name: "account.group_account_secured"},
		{ID: GroupCashRounding, Name: "account.group_cash_rounding"},
		{ID: GroupValidateBankAccount, Name: "account.group_validate_bank_account"},
		{ID: GroupBillingUser, Name: "Accounting / Billing User Compatibility", ImpliedIDs: []int64{GroupAccountInvoice}},
		{ID: GroupReadOnlyAuditor, Name: "Accounting / Read-only Auditor Compatibility", ImpliedIDs: []int64{GroupAccountReadonly}},
	}
}

func SecurityACLs() []security.ACL {
	return []security.ACL{
		acl("access_account_cash_rounding_readonly", "account.cash.rounding", GroupAccountReadonly, true, false, false, false),
		acl("access_account_cash_rounding_uinvoice", "account.cash.rounding", GroupAccountInvoice, true, true, true, true),
		acl("access_account_fiscal_position_product_manager", "account.fiscal.position", GroupAccountManager, true, true, true, true),
		acl("access_account_fiscal_position", "account.fiscal.position", GroupBaseUser, true, false, false, false),

		acl("access_account_bank_statement_group_readonly", "account.bank.statement", GroupAccountReadonly, true, false, false, false),
		acl("access_account_bank_statement_group_invoice", "account.bank.statement", GroupAccountInvoice, true, false, false, false),
		acl("access_account_bank_statement_line_group_readonly", "account.bank.statement.line", GroupAccountReadonly, true, false, false, false),
		acl("access_account_bank_statement_line_group_invoice", "account.bank.statement.line", GroupAccountInvoice, true, false, false, false),
		acl("access_account_bank_statement", "account.bank.statement", GroupAccountBasic, true, true, true, true),
		acl("access_account_bank_statement_line", "account.bank.statement.line", GroupAccountBasic, true, true, true, true),

		acl("access_account_move_line_manager", "account.move.line", GroupAccountManager, true, false, false, false),
		acl("access_account_move_manager", "account.move", GroupAccountManager, true, false, false, false),
		acl("access_account_move_readonly", "account.move", GroupAccountReadonly, true, false, false, false),
		acl("access_account_move_uinvoice", "account.move", GroupAccountInvoice, true, true, true, true),
		acl("access_account_move_line_readonly", "account.move.line", GroupAccountReadonly, true, false, false, false),
		acl("access_account_move_line_uinvoice", "account.move.line", GroupAccountInvoice, true, true, true, true),
		acl("access_account_invoice_portal", "account.move", GroupBasePortal, true, false, false, false),
		acl("access_account_invoice_line_portal", "account.move.line", GroupBasePortal, true, false, false, false),
		acl("access_account_move_reversal", "account.move.reversal", GroupAccountInvoice, true, true, true, false),
		acl("access_account_payment_register", "account.payment.register", GroupAccountInvoice, true, true, true, false),
		acl("access_account_move_send_wizard", "account.move.send.wizard", GroupAccountInvoice, true, true, true, true),
		acl("access_account_move_send_batch_wizard", "account.move.send.batch.wizard", GroupAccountInvoice, true, true, true, true),

		acl("access_account_journal_readonly", "account.journal", GroupAccountReadonly, true, false, false, false),
		acl("access_account_journal_manager", "account.journal", GroupAccountManager, true, true, true, true),
		acl("access_account_journal_invoice", "account.journal", GroupAccountInvoice, true, false, false, false),

		acl("access_account_code_mapping", "account.code.mapping", GroupAccountReadonly, true, false, false, false),
		acl("access_account_code_mapping_manager", "account.code.mapping", GroupAccountManager, true, true, false, false),
		acl("access_account_account_manager", "account.account", GroupAccountManager, true, true, true, true),
		acl("access_account_account", "account.account", GroupAccountReadonly, true, false, false, false),
		acl("access_account_account_user", "account.account", GroupBaseUser, true, false, false, false),
		acl("access_account_account_invoice", "account.account", GroupAccountInvoice, true, false, false, false),
		acl("access_account_group_readonly", "account.group", GroupAccountReadonly, true, false, false, false),
		acl("access_account_group_manager", "account.group", GroupAccountManager, true, true, true, true),
		acl("access_account_root_readonly", "account.root", GroupAccountReadonly, true, false, false, false),
		acl("access_account_journal_group_readonly", "account.journal.group", GroupAccountReadonly, true, false, false, false),
		acl("access_account_journal_group_manager", "account.journal.group", GroupAccountManager, true, true, true, true),
		acl("access_account_incoterms_user", "account.incoterms", GroupBaseUser, true, false, false, false),
		acl("access_account_incoterms_manager", "account.incoterms", GroupAccountManager, true, true, true, true),
		acl("access_account_lock_exception_user", "account.lock_exception", GroupBaseUser, true, false, false, false),
		acl("access_account_lock_exception_manager", "account.lock_exception", GroupAccountManager, true, false, true, false),

		acl("access_account_tax_internal_user", "account.tax", GroupBaseUser, true, false, false, false),
		acl("access_account_tax_readonly", "account.tax", GroupAccountReadonly, true, false, false, false),
		acl("access_account_tax_invoice", "account.tax", GroupAccountInvoice, true, false, false, false),
		acl("access_account_tax_manager", "account.tax", GroupAccountManager, true, true, true, true),
		acl("access_account_tag_internal_user", "account.account.tag", GroupBaseUser, true, false, false, false),
		acl("access_account_account_tax", "account.account.tag", GroupAccountUser, true, true, true, true),
		acl("access_account_account_tax_readonly", "account.account.tag", GroupAccountReadonly, true, false, false, false),
		acl("access_account_account_tax_user", "account.account.tag", GroupAccountInvoice, true, false, false, false),
		acl("access_account_tax_repartition_line_user", "account.tax.repartition.line", GroupBaseUser, true, false, false, false),
		acl("access_account_tax_repartition_line_readonly", "account.tax.repartition.line", GroupAccountReadonly, true, false, false, false),
		acl("access_account_tax_repartition_line_invoice", "account.tax.repartition.line", GroupAccountInvoice, true, false, false, false),
		acl("access_account_tax_repartition_line_manager", "account.tax.repartition.line", GroupAccountManager, true, true, true, true),
		acl("access_account_tax_group_internal_user", "account.tax.group", GroupBaseUser, true, false, false, false),
		acl("access_account_tax_group_readonly", "account.tax.group", GroupAccountReadonly, true, false, false, false),
		acl("access_account_tax_group", "account.tax.group", GroupAccountInvoice, true, false, false, false),
		acl("access_account_tax_group_manager", "account.tax.group", GroupAccountManager, true, true, true, true),

		acl("access_account_reconcile_model_readonly", "account.reconcile.model", GroupAccountReadonly, true, false, false, false),
		acl("access_account_reconcile_model_billing", "account.reconcile.model", GroupAccountInvoice, true, false, true, false),
		acl("access_account_reconcile_model", "account.reconcile.model", GroupAccountBasic, true, true, true, true),
		acl("access_account_reconcile_model_line_readonly", "account.reconcile.model.line", GroupAccountReadonly, true, false, false, false),
		acl("access_account_reconcile_model_line_billing", "account.reconcile.model.line", GroupAccountInvoice, true, false, true, false),
		acl("access_account_reconcile_model_line", "account.reconcile.model.line", GroupAccountBasic, true, true, true, true),
		acl("access_account_partial_reconcile_readonly", "account.partial.reconcile", GroupAccountReadonly, true, false, false, false),
		acl("access_account_partial_reconcile_group_invoice", "account.partial.reconcile", GroupAccountInvoice, true, true, true, true),
		acl("access_account_partial_reconcile", "account.partial.reconcile", GroupAccountUser, true, true, true, true),
		acl("access_account_full_reconcile_group_readonly", "account.full.reconcile", GroupAccountReadonly, true, false, false, false),
		acl("access_account_full_reconcile_group_invoice", "account.full.reconcile", GroupAccountInvoice, true, true, true, true),
		acl("access_account_full_reconcile", "account.full.reconcile", GroupAccountUser, true, true, true, true),

		acl("access_account_payment_term_partner_manager", "account.payment.term", GroupBaseUser, true, false, false, false),
		acl("access_account_payment_term_portal", "account.payment.term", GroupBasePortal, true, false, false, false),
		acl("access_account_payment_term_manager", "account.payment.term", GroupAccountManager, true, true, true, true),
		acl("access_account_payment_term_line_partner_manager", "account.payment.term.line", GroupBaseUser, true, false, false, false),
		acl("access_account_payment_term_line_manager", "account.payment.term.line", GroupAccountManager, true, true, true, true),
		acl("access_account_payment_method_line_readonly", "account.payment.method.line", GroupBaseUser, true, false, false, false),
		acl("access_account_payment_method_line", "account.payment.method.line", GroupAccountInvoice, true, true, true, true),
		acl("access_account_payment_method_readonly", "account.payment.method", GroupBaseUser, true, false, false, false),
		acl("access_account_payment_method", "account.payment.method", GroupAccountInvoice, true, true, false, true),
		acl("access_account_payment_readonly", "account.payment", GroupAccountReadonly, true, false, false, false),
		acl("access_account_payment", "account.payment", GroupAccountInvoice, true, true, true, true),

		acl("access_account_report_basic", "account.report", GroupAccountBasic, true, false, false, false),
		acl("access_account_report_readonly", "account.report", GroupAccountReadonly, true, false, false, false),
		acl("access_account_report_ac_user", "account.report", GroupAccountManager, true, true, true, true),
		acl("access_account_report_line_basic", "account.report.line", GroupAccountBasic, true, false, false, false),
		acl("access_account_report_line_readonly", "account.report.line", GroupAccountReadonly, true, false, false, false),
		acl("access_account_report_line_ac_user", "account.report.line", GroupAccountManager, true, true, true, true),
		acl("access_account_report_expression_basic", "account.report.expression", GroupAccountBasic, true, false, false, false),
		acl("access_account_report_expression_readonly", "account.report.expression", GroupAccountReadonly, true, false, false, false),
		acl("access_account_report_expression_ac_user", "account.report.expression", GroupAccountManager, true, true, true, true),
		acl("access_account_report_column_basic", "account.report.column", GroupAccountBasic, true, false, false, false),
		acl("access_account_report_column_readonly", "account.report.column", GroupAccountReadonly, true, false, false, false),
		acl("access_account_report_column_ac_user", "account.report.column", GroupAccountManager, true, true, true, true),
		acl("access_account_report_external_value_readonly", "account.report.external.value", GroupAccountReadonly, true, false, false, false),
		acl("access_account_report_external_value_ac_user", "account.report.external.value", GroupAccountManager, true, true, true, true),
		acl("access_account_invoice_report_readonly", "account.invoice.report", GroupAccountReadonly, true, false, false, false),
		acl("access_account_invoice_report_invoice", "account.invoice.report", GroupAccountInvoice, true, false, false, false),

		acl("access_account_account_type_readonly_local", "account.account.type", GroupAccountReadonly, true, false, false, false),
		acl("access_account_account_type_manager_local", "account.account.type", GroupAccountManager, true, true, true, true),
		acl("access_account_chart_template_readonly_local", "account.chart.template", GroupAccountReadonly, true, false, false, false),
		acl("access_account_chart_template_manager_local", "account.chart.template", GroupAccountManager, true, true, true, true),
		acl("access_account_fiscal_position_account", "account.fiscal.position.account", GroupBaseUser, true, false, false, false),
		acl("access_account_fiscal_position_account_manager", "account.fiscal.position.account", GroupAccountManager, true, true, true, true),
		acl("access_account_automatic_entry_wizard", "account.automatic.entry.wizard", GroupAccountInvoice, true, true, true, false),
		acl("access_account_autopost_bills_wizard", "account.autopost.bills.wizard", GroupAccountInvoice, true, true, true, false),
		acl("access_account_resequence_wizard", "account.resequence.wizard", GroupAccountInvoice, true, true, true, false),
		acl("access_account_secure_entries_wizard", "account.secure.entries.wizard", GroupAccountManager, true, true, true, false),
		acl("access_account_merge_wizard", "account.merge.wizard", GroupAccountManager, true, true, true, false),
		acl("access_account_merge_wizard_line", "account.merge.wizard.line", GroupAccountManager, true, true, true, false),
		acl("access_account_accrued_orders_wizard", "account.accrued.orders.wizard", GroupAccountInvoice, true, true, true, false),
	}
}

func SecurityRuleDefinitions() []RuleDefinition {
	return []RuleDefinition{
		globalRule("account_move_comp_rule", "account.move", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("account_move_line_comp_rule", "account.move.line", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("journal_comp_rule", "account.journal", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_journal_group_comp_rule", "account.journal.group", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_comp_rule", "account.account", companyIn("company_id"), "[('company_ids', 'parent_of', company_ids)]"),
		globalRule("account_group_comp_rule", "account.group", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("tax_group_comp_rule", "account.tax.group", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("tax_comp_rule", "account.tax", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("tax_rep_comp_rule", "account.tax.repartition.line", optionalCompanyIn("company_id"), "['|', ('company_id', '=', False), ('company_id', 'parent_of', company_ids)]"),
		globalRule("account_fiscal_position_comp_rule", "account.fiscal.position", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_fiscal_position_account_comp_rule", "account.fiscal.position.account", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_lock_exception_comp_rule", "account.lock_exception", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("account_invoice_report_comp_rule", "account.invoice.report", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("account_bank_statement_comp_rule", "account.bank.statement", optionalCompanyIn("company_id"), "[('company_id', 'in', company_ids + [False])]"),
		globalRule("account_bank_statement_line_comp_rule", "account.bank.statement.line", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("account_reconcile_model_template_comp_rule", "account.reconcile.model", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_reconcile_model_line_template_comp_rule", "account.reconcile.model.line", companyIn("company_id"), "[('company_id', 'parent_of', company_ids)]"),
		globalRule("account_payment_comp_rule", "account.payment", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		globalRule("account_payment_term_comp_rule", "account.payment.term", optionalCompanyIn("company_id"), "['|', ('company_id', '=', False), ('company_id', 'parent_of', company_ids)]"),
		globalRule("report_external_value_comp_rule", "account.report.external.value", companyIn("company_id"), "[('company_id', 'in', company_ids)]"),
		groupRuleDomain("account_invoice_rule_portal", "account.move", GroupBasePortal, portalMoveDomain(), "[('state', 'not in', ('cancel', 'draft')), ('move_type', 'in', ('out_invoice', 'out_refund', 'in_invoice', 'in_refund')), ('partner_id', 'child_of', [user.commercial_partner_id.id])]", true, false, false, false),
		groupRuleDomain("account_invoice_line_rule_portal", "account.move.line", GroupBasePortal, portalMoveLineDomain(), "[('parent_state', 'not in', ('cancel', 'draft')), ('move_type', 'in', ('out_invoice', 'out_refund', 'in_invoice', 'in_refund')), ('partner_id', 'child_of', [user.commercial_partner_id.id])]", true, false, false, false),
		groupRule("account_move_see_all", "account.move", GroupAccountInvoice, true, true, true, true),
		groupRule("account_move_line_see_all", "account.move.line", GroupAccountInvoice, true, true, true, true),
		groupRule("account_move_rule_group_readonly", "account.move", GroupAccountReadonly, true, false, false, false),
		groupRule("account_move_line_rule_group_readonly", "account.move.line", GroupAccountReadonly, true, false, false, false),
		groupRule("account_move_rule_group_invoice", "account.move", GroupAccountInvoice, true, true, true, true),
		groupRule("account_move_line_rule_group_invoice", "account.move.line", GroupAccountInvoice, true, true, true, true),
	}
}

func SecurityRules() []security.Rule {
	definitions := SecurityRuleDefinitions()
	rules := make([]security.Rule, 0, len(definitions))
	for _, definition := range definitions {
		rules = append(rules, definition.Rule)
	}
	return rules
}

func CompanyTreeDomain() domain.Node {
	return companyIn("company_id")
}

func acl(name string, modelName string, groupID int64, canRead bool, canWrite bool, canCreate bool, canUnlink bool) security.ACL {
	return security.ACL{
		Name:       name,
		Model:      modelName,
		GroupID:    groupID,
		Active:     true,
		PermRead:   canRead,
		PermWrite:  canWrite,
		PermCreate: canCreate,
		PermUnlink: canUnlink,
	}
}

func globalRule(name string, modelName string, node domain.Node, domainText string) RuleDefinition {
	return RuleDefinition{
		Name: name,
		Rule: security.Rule{
			Name:       name,
			Model:      modelName,
			Domain:     node,
			DomainText: domainText,
			Global:     true,
			PermRead:   true,
			PermWrite:  true,
			PermCreate: true,
			PermUnlink: true,
			Active:     true,
		},
	}
}

func groupRule(name string, modelName string, groupID int64, canRead bool, canWrite bool, canCreate bool, canUnlink bool) RuleDefinition {
	return groupRuleDomain(name, modelName, groupID, domain.And(), "[(1, '=', 1)]", canRead, canWrite, canCreate, canUnlink)
}

func groupRuleDomain(name string, modelName string, groupID int64, node domain.Node, domainText string, canRead bool, canWrite bool, canCreate bool, canUnlink bool) RuleDefinition {
	return RuleDefinition{
		Name: name,
		Rule: security.Rule{
			Name:       name,
			Model:      modelName,
			Domain:     node,
			DomainText: domainText,
			GroupIDs:   []int64{groupID},
			PermRead:   canRead,
			PermWrite:  canWrite,
			PermCreate: canCreate,
			PermUnlink: canUnlink,
			Active:     true,
		},
	}
}

func portalMoveDomain() domain.Node {
	return domain.And(
		domain.Cond("state", domain.NotIn, []string{"cancel", "draft"}),
		domain.Cond("move_type", domain.In, portalMoveTypes()),
		domain.Cond("partner_id", domain.ChildOf, []any{"user.commercial_partner_id.id"}),
	)
}

func portalMoveLineDomain() domain.Node {
	return domain.And(
		domain.Cond("parent_state", domain.NotIn, []string{"cancel", "draft"}),
		domain.Cond("move_type", domain.In, portalMoveTypes()),
		domain.Cond("partner_id", domain.ChildOf, []any{"user.commercial_partner_id.id"}),
	)
}

func portalMoveTypes() []string {
	return []string{"out_invoice", "out_refund", "in_invoice", "in_refund"}
}

func companyIn(fieldName string) domain.Node {
	return domain.Cond(fieldName, domain.In, "user.company_ids")
}

func optionalCompanyIn(fieldName string) domain.Node {
	return domain.Or(
		domain.Cond(fieldName, domain.Equal, nil),
		companyIn(fieldName),
	)
}
