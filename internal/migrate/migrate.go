package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Migration struct {
	Version int
	Name    string
	SQL     string
}

type Runner struct {
	migrations []Migration
}

func NewRunner(migrations []Migration) Runner {
	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})
	return Runner{migrations: ordered}
}

func (r Runner) Apply(ctx context.Context, exec Executor) error {
	seen := map[int]bool{}
	for _, migration := range r.migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("migration %q has invalid version %d", migration.Name, migration.Version)
		}
		if seen[migration.Version] {
			return fmt.Errorf("duplicate migration version %d", migration.Version)
		}
		seen[migration.Version] = true
		if _, err := exec.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("migration %d %s: %w", migration.Version, migration.Name, err)
		}
	}
	return nil
}

var BaseMigrations = []Migration{
	{Version: 1, Name: "ir_module_module", SQL: `CREATE TABLE IF NOT EXISTS ir_module_module (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL UNIQUE, state TEXT NOT NULL)`},
	{Version: 2, Name: "ir_model", SQL: `CREATE TABLE IF NOT EXISTS ir_model (id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL UNIQUE, name TEXT NOT NULL, abstract BOOLEAN NOT NULL DEFAULT false, transient BOOLEAN NOT NULL DEFAULT false, is_mail_thread BOOLEAN NOT NULL DEFAULT false, is_mail_activity BOOLEAN NOT NULL DEFAULT false, is_mail_blacklist BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 3, Name: "ir_model_access", SQL: `CREATE TABLE IF NOT EXISTS ir_model_access (id BIGSERIAL PRIMARY KEY, name TEXT, active BOOLEAN NOT NULL DEFAULT true, model TEXT, model_id BIGINT, group_id BIGINT, perm_read BOOLEAN NOT NULL DEFAULT false, perm_write BOOLEAN NOT NULL DEFAULT false, perm_create BOOLEAN NOT NULL DEFAULT false, perm_unlink BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 4, Name: "ir_model_fields", SQL: `CREATE TABLE IF NOT EXISTS ir_model_fields (id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL, name TEXT NOT NULL, ttype TEXT NOT NULL, relation TEXT, relation_field TEXT, groups TEXT, ai_vector_size INTEGER)`},
	{Version: 5, Name: "ir_rule", SQL: `CREATE TABLE IF NOT EXISTS ir_rule (id BIGSERIAL PRIMARY KEY, name TEXT, model TEXT, model_id BIGINT, domain TEXT, domain_force TEXT, groups TEXT, group_ids TEXT, global BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true, perm_read BOOLEAN NOT NULL DEFAULT true, perm_write BOOLEAN NOT NULL DEFAULT true, perm_create BOOLEAN NOT NULL DEFAULT true, perm_unlink BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 6, Name: "ir_model_data", SQL: `CREATE TABLE IF NOT EXISTS ir_model_data (id BIGSERIAL PRIMARY KEY, module TEXT NOT NULL, name TEXT NOT NULL, complete_name TEXT, model TEXT NOT NULL, res_id BIGINT NOT NULL, noupdate BOOLEAN NOT NULL DEFAULT false, CONSTRAINT ir_model_data_name_nospaces CHECK (name NOT LIKE '% %'), CONSTRAINT ir_model_data_module_name_uniq UNIQUE (module, name)); CREATE INDEX IF NOT EXISTS ir_model_data_model_res_id_idx ON ir_model_data(model, res_id)`},
	{Version: 7, Name: "ir_config_parameter", SQL: `CREATE TABLE IF NOT EXISTS ir_config_parameter (id BIGSERIAL PRIMARY KEY, key TEXT NOT NULL UNIQUE, value TEXT)`},
	{Version: 8, Name: "ir_cron", SQL: `CREATE TABLE IF NOT EXISTS ir_cron (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, ir_actions_server_id BIGINT, cron_name TEXT, active BOOLEAN NOT NULL DEFAULT true, interval_number INTEGER NOT NULL DEFAULT 1, interval_type TEXT NOT NULL DEFAULT 'minutes', nextcall TIMESTAMPTZ, lastcall TIMESTAMPTZ, user_id BIGINT, model_id BIGINT, state TEXT, code TEXT, priority INTEGER NOT NULL DEFAULT 5, failure_count INTEGER NOT NULL DEFAULT 0, first_failure_date TIMESTAMPTZ, action_name TEXT)`},
	{Version: 9, Name: "ir_cron_trigger", SQL: `CREATE TABLE IF NOT EXISTS ir_cron_trigger (id BIGSERIAL PRIMARY KEY, cron_id BIGINT NOT NULL, call_at TIMESTAMPTZ NOT NULL)`},
	{Version: 10, Name: "ir_cron_progress", SQL: `CREATE TABLE IF NOT EXISTS ir_cron_progress (id BIGSERIAL PRIMARY KEY, cron_id BIGINT NOT NULL, done INTEGER NOT NULL DEFAULT 0, remaining INTEGER NOT NULL DEFAULT 0, deactivate BOOLEAN NOT NULL DEFAULT false, timed_out_counter INTEGER NOT NULL DEFAULT 0, started_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`},
	{Version: 11, Name: "base_automation", SQL: `CREATE TABLE IF NOT EXISTS base_automation (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, model_id BIGINT, model_name TEXT NOT NULL, model_is_mail_thread BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true, trigger TEXT NOT NULL, trigger_field_names TEXT, trigger_field_ids TEXT, on_change_field_ids TEXT, filter_pre_domain TEXT, previous_domain TEXT, filter_domain TEXT, action_server_id BIGINT, action_server_ids TEXT, url TEXT, webhook_uuid TEXT, record_getter TEXT, log_webhook_calls BOOLEAN NOT NULL DEFAULT false, last_run TIMESTAMPTZ, trg_selection_field_id BIGINT, trg_field_ref_model_name TEXT, trg_field_ref TEXT, trg_date_id BIGINT, trg_date_range INTEGER, trg_date_range_mode TEXT, trg_date_range_type TEXT, trg_date_calendar_id BIGINT)`},
	{Version: 12, Name: "mail_message", SQL: `CREATE TABLE IF NOT EXISTS mail_message (id BIGSERIAL PRIMARY KEY, subject TEXT, body TEXT, message_type TEXT NOT NULL DEFAULT 'notification', model TEXT, res_id BIGINT, author_id BIGINT, author_guest_id BIGINT, email_from TEXT, message_id TEXT, incoming_email_to TEXT, incoming_email_cc TEXT, outgoing_email_to TEXT, reply_to TEXT, reply_to_force_new BOOLEAN NOT NULL DEFAULT false, mail_server_id BIGINT, email_layout_xmlid TEXT, email_add_signature BOOLEAN NOT NULL DEFAULT true, date TIMESTAMPTZ, parent_id BIGINT, subtype_id BIGINT, mail_activity_type_id BIGINT, partner_ids TEXT, attachment_ids TEXT, body_is_html BOOLEAN NOT NULL DEFAULT false, is_internal BOOLEAN NOT NULL DEFAULT false, reaction_ids TEXT, starred BOOLEAN NOT NULL DEFAULT false, starred_partner_ids TEXT, tracking_value_ids TEXT, create_uid BIGINT, create_date TIMESTAMPTZ, write_uid BIGINT, write_date TIMESTAMPTZ)`},
	{Version: 13, Name: "mail_mail", SQL: `CREATE TABLE IF NOT EXISTS mail_mail (id BIGSERIAL PRIMARY KEY, mail_message_id BIGINT, recipient_ids TEXT, attachment_ids TEXT, mail_server_id BIGINT, author_id BIGINT, email_from TEXT, email_to TEXT NOT NULL, email_cc TEXT, reply_to TEXT, subject TEXT, body_html TEXT, state TEXT NOT NULL DEFAULT 'outgoing', failure_reason TEXT, failure_type TEXT, scheduled_date TIMESTAMPTZ, retry_count INTEGER NOT NULL DEFAULT 0, max_retries INTEGER NOT NULL DEFAULT 3, auto_delete BOOLEAN NOT NULL DEFAULT false, message_id TEXT, references TEXT, headers TEXT, is_notification BOOLEAN NOT NULL DEFAULT false, fetchmail_server_id BIGINT, mailing_id BIGINT, mailing_trace_ids TEXT, create_uid BIGINT, create_date TIMESTAMPTZ, write_uid BIGINT, write_date TIMESTAMPTZ)`},
	{Version: 14, Name: "mail_notification", SQL: `CREATE TABLE IF NOT EXISTS mail_notification (id BIGSERIAL PRIMARY KEY, mail_message_id BIGINT NOT NULL, mail_mail_id BIGINT, res_partner_id BIGINT, mail_email_address TEXT, notification_type TEXT, notification_status TEXT NOT NULL DEFAULT 'ready', failure_type TEXT, failure_reason TEXT, is_read BOOLEAN NOT NULL DEFAULT false, read_date TIMESTAMPTZ, author_id BIGINT, create_uid BIGINT, create_date TIMESTAMPTZ, write_uid BIGINT, write_date TIMESTAMPTZ)`},
	{Version: 15, Name: "mail_followers", SQL: `CREATE TABLE IF NOT EXISTS mail_followers (id BIGSERIAL PRIMARY KEY, res_model TEXT NOT NULL, res_id BIGINT NOT NULL, partner_id BIGINT NOT NULL, subtype_ids TEXT)`},
	{Version: 16, Name: "mail_activity", SQL: `CREATE TABLE IF NOT EXISTS mail_activity (id BIGSERIAL PRIMARY KEY, activity_type_id BIGINT, activity_category TEXT, recommended_activity_type_id BIGINT, previous_activity_type_id BIGINT, has_recommended_activities BOOLEAN NOT NULL DEFAULT false, chaining_type TEXT, res_model TEXT NOT NULL, res_id BIGINT NOT NULL, user_id BIGINT NOT NULL, date_deadline DATE, summary TEXT, note TEXT, state TEXT NOT NULL DEFAULT 'open', automated BOOLEAN NOT NULL DEFAULT false, hide_in_chatter BOOLEAN NOT NULL DEFAULT false, attachment_ids TEXT, active BOOLEAN NOT NULL DEFAULT true, date_done TIMESTAMPTZ, feedback TEXT)`},
	{Version: 17, Name: "mail_activity_type", SQL: `CREATE TABLE IF NOT EXISTS mail_activity_type (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, summary TEXT, res_model TEXT, category TEXT, default_note TEXT, default_user_id BIGINT, delay_count INTEGER NOT NULL DEFAULT 0, delay_unit TEXT NOT NULL DEFAULT 'days', delay_from TEXT NOT NULL DEFAULT 'previous_activity', chaining_type TEXT NOT NULL DEFAULT 'suggest', triggered_next_type_id BIGINT, suggested_next_type_ids TEXT, previous_type_ids TEXT, mail_template_ids TEXT, icon TEXT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 18, Name: "mail_template", SQL: `CREATE TABLE IF NOT EXISTS mail_template (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, model TEXT, model_id BIGINT, subject TEXT, body_html TEXT, email_from TEXT, email_to TEXT, email_cc TEXT, reply_to TEXT, partner_to TEXT, attachment_ids TEXT, report_template_ids TEXT, mail_server_id BIGINT, auto_delete BOOLEAN NOT NULL DEFAULT false, scheduled_date TIMESTAMPTZ, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 19, Name: "mail_alias", SQL: `CREATE TABLE IF NOT EXISTS mail_alias (id BIGSERIAL PRIMARY KEY, alias_name TEXT NOT NULL, model_name TEXT, alias_model_id BIGINT, alias_defaults TEXT NOT NULL DEFAULT '{}', alias_force_thread_id BIGINT, alias_parent_model_id BIGINT, alias_parent_thread_id BIGINT, alias_contact TEXT NOT NULL DEFAULT 'everyone', alias_incoming_local BOOLEAN NOT NULL DEFAULT false, alias_bounced_content TEXT, alias_status TEXT, alias_user_id BIGINT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 20, Name: "mail_message_subtype", SQL: `CREATE TABLE IF NOT EXISTS mail_message_subtype (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, res_model TEXT, default_subtype BOOLEAN NOT NULL DEFAULT false, internal BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 21, Name: "fetchmail_server", SQL: `CREATE TABLE IF NOT EXISTS fetchmail_server (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, state TEXT, server TEXT, port INTEGER, server_type TEXT, server_type_info TEXT, is_ssl BOOLEAN NOT NULL DEFAULT false, attach BOOLEAN NOT NULL DEFAULT true, original BOOLEAN NOT NULL DEFAULT false, date TIMESTAMPTZ, error_date TIMESTAMPTZ, error_message TEXT, "user" TEXT, password TEXT, object_id BIGINT, priority INTEGER NOT NULL DEFAULT 5, message_ids TEXT, configuration TEXT, script TEXT)`},
	{Version: 22, Name: "discuss_channel", SQL: `CREATE TABLE IF NOT EXISTS discuss_channel (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, channel_type TEXT, active BOOLEAN NOT NULL DEFAULT true, group_public_id BIGINT, ai_env_context TEXT, ai_agent_id BIGINT)`},
	{Version: 23, Name: "discuss_channel_member", SQL: `CREATE TABLE IF NOT EXISTS discuss_channel_member (id BIGSERIAL PRIMARY KEY, channel_id BIGINT NOT NULL, partner_id BIGINT, user_id BIGINT, guest_id BIGINT, last_seen_dt TIMESTAMPTZ)`},
	{Version: 24, Name: "mail_guest", SQL: `CREATE TABLE IF NOT EXISTS mail_guest (id BIGSERIAL PRIMARY KEY, name TEXT, email TEXT, access_token TEXT, country_id BIGINT, lang TEXT, timezone TEXT)`},
	{Version: 25, Name: "mail_presence", SQL: `CREATE TABLE IF NOT EXISTS mail_presence (id BIGSERIAL PRIMARY KEY, user_id BIGINT, guest_id BIGINT, status TEXT NOT NULL, last_poll TIMESTAMPTZ)`},
	{Version: 26, Name: "account_account", SQL: `CREATE TABLE IF NOT EXISTS account_account (id BIGSERIAL PRIMARY KEY, code TEXT NOT NULL, placeholder_code TEXT, name TEXT NOT NULL, account_type TEXT NOT NULL, include_initial_balance BOOLEAN NOT NULL DEFAULT false, internal_group TEXT, root_id TEXT, group_id BIGINT, company_id BIGINT, currency_id BIGINT, reconcile BOOLEAN NOT NULL DEFAULT false, deprecated BOOLEAN NOT NULL DEFAULT false, non_trade BOOLEAN NOT NULL DEFAULT false, tax_ids TEXT)`},
	{Version: 27, Name: "account_account_tag", SQL: `CREATE TABLE IF NOT EXISTS account_account_tag (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, applicability TEXT, country_id BIGINT)`},
	{Version: 28, Name: "account_account_type", SQL: `CREATE TABLE IF NOT EXISTS account_account_type (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL, include_initial_balance BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 29, Name: "account_bank_statement", SQL: `CREATE TABLE IF NOT EXISTS account_bank_statement (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, journal_id BIGINT, company_id BIGINT, balance_start NUMERIC, balance_end_real NUMERIC, state TEXT)`},
	{Version: 30, Name: "account_bank_statement_line", SQL: `CREATE TABLE IF NOT EXISTS account_bank_statement_line (id BIGSERIAL PRIMARY KEY, statement_id BIGINT, journal_id BIGINT, move_id BIGINT, company_id BIGINT, payment_ref TEXT, amount NUMERIC, date DATE)`},
	{Version: 31, Name: "account_cash_rounding", SQL: `CREATE TABLE IF NOT EXISTS account_cash_rounding (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, rounding NUMERIC NOT NULL, strategy TEXT, profit_account_id BIGINT, loss_account_id BIGINT)`},
	{Version: 32, Name: "account_chart_template", SQL: `CREATE TABLE IF NOT EXISTS account_chart_template (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code_digits INTEGER, currency_id BIGINT, visible BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 33, Name: "account_code_mapping", SQL: `CREATE TABLE IF NOT EXISTS account_code_mapping (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, company_id BIGINT, account_id BIGINT, code TEXT)`},
	{Version: 34, Name: "account_journal", SQL: `CREATE TABLE IF NOT EXISTS account_journal (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT NOT NULL, type TEXT NOT NULL, company_id BIGINT, currency_id BIGINT, default_account_id BIGINT, active BOOLEAN NOT NULL DEFAULT true, sequence INTEGER, restrict_mode_hash_table BOOLEAN NOT NULL DEFAULT false, sequence_number_next INTEGER NOT NULL DEFAULT 1, refund_sequence BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 35, Name: "account_move", SQL: `CREATE TABLE IF NOT EXISTS account_move (id BIGSERIAL PRIMARY KEY, name TEXT, ref TEXT, date DATE NOT NULL, invoice_date DATE, invoice_date_due DATE, state TEXT NOT NULL DEFAULT 'draft', move_type TEXT, journal_id BIGINT, company_id BIGINT, currency_id BIGINT, partner_id BIGINT, fiscal_position_id BIGINT, amount_total NUMERIC, amount_residual NUMERIC, amount_residual_signed NUMERIC, payment_state TEXT, status_in_payment TEXT, is_move_sent BOOLEAN NOT NULL DEFAULT false, sending_data TEXT, is_being_sent BOOLEAN NOT NULL DEFAULT false, invoice_pdf_report_id BIGINT, invoice_pdf_report_file TEXT, ubl_cii_xml_file TEXT, message_main_attachment_id BIGINT, access_url TEXT, access_token TEXT, access_warning TEXT, origin_payment_id BIGINT, statement_line_id BIGINT, matched_payment_ids TEXT, reconciled_payment_ids TEXT, payment_count INTEGER NOT NULL DEFAULT 0, posted_before BOOLEAN NOT NULL DEFAULT false, inalterable_hash TEXT, sequence_prefix TEXT, sequence_number INTEGER, made_sequence_gap BOOLEAN NOT NULL DEFAULT false, secure_sequence_number INTEGER, auto_post TEXT NOT NULL DEFAULT 'no', need_cancel_request BOOLEAN NOT NULL DEFAULT false, show_reset_to_draft_button BOOLEAN NOT NULL DEFAULT false, reversed_entry_id BIGINT)`},
	{Version: 36, Name: "account_move_line", SQL: `CREATE TABLE IF NOT EXISTS account_move_line (id BIGSERIAL PRIMARY KEY, move_id BIGINT, account_id BIGINT NOT NULL, account_type TEXT, account_internal_group TEXT, partner_id BIGINT, company_id BIGINT, currency_id BIGINT, name TEXT, parent_state TEXT, journal_id BIGINT, debit NUMERIC NOT NULL DEFAULT 0, credit NUMERIC NOT NULL DEFAULT 0, balance NUMERIC NOT NULL DEFAULT 0, quantity DOUBLE PRECISION, price_unit NUMERIC, price_subtotal NUMERIC, price_total NUMERIC, discount DOUBLE PRECISION, display_type TEXT, date_maturity DATE, product_id BIGINT, product_uom_id BIGINT, allowed_uom_ids TEXT, product_category_id BIGINT, analytic_line_ids TEXT, analytic_distribution TEXT, distribution_analytic_account_ids TEXT, analytic_precision INTEGER, has_invalid_analytics BOOLEAN NOT NULL DEFAULT false, amount_currency NUMERIC, amount_residual NUMERIC, amount_residual_currency NUMERIC, reconciled BOOLEAN NOT NULL DEFAULT false, payment_id BIGINT, full_reconcile_id BIGINT, matched_debit_ids TEXT, matched_credit_ids TEXT, tax_ids TEXT, tax_tag_ids TEXT, tax_line_id BIGINT, tax_repartition_line_id BIGINT)`},
	{Version: 37, Name: "account_payment", SQL: `CREATE TABLE IF NOT EXISTS account_payment (id BIGSERIAL PRIMARY KEY, name TEXT, payment_type TEXT, partner_type TEXT, partner_id BIGINT, amount NUMERIC, company_id BIGINT, currency_id BIGINT, journal_id BIGINT, move_id BIGINT, state TEXT, is_reconciled BOOLEAN NOT NULL DEFAULT false, is_matched BOOLEAN NOT NULL DEFAULT false, is_sent BOOLEAN NOT NULL DEFAULT false, invoice_ids TEXT, reconciled_invoice_ids TEXT, reconciled_bill_ids TEXT, need_cancel_request BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 38, Name: "account_payment_method", SQL: `CREATE TABLE IF NOT EXISTS account_payment_method (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT NOT NULL, payment_type TEXT)`},
	{Version: 39, Name: "account_payment_method_line", SQL: `CREATE TABLE IF NOT EXISTS account_payment_method_line (id BIGSERIAL PRIMARY KEY, name TEXT, journal_id BIGINT, payment_method_id BIGINT, payment_account_id BIGINT)`},
	{Version: 40, Name: "account_payment_term", SQL: `CREATE TABLE IF NOT EXISTS account_payment_term (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, company_id BIGINT, active BOOLEAN NOT NULL DEFAULT true, note TEXT)`},
	{Version: 41, Name: "account_payment_term_line", SQL: `CREATE TABLE IF NOT EXISTS account_payment_term_line (id BIGSERIAL PRIMARY KEY, payment_id BIGINT, value TEXT, value_amount NUMERIC, nb_days INTEGER)`},
	{Version: 42, Name: "account_tax", SQL: `CREATE TABLE IF NOT EXISTS account_tax (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, invoice_label TEXT, amount_type TEXT NOT NULL, amount NUMERIC NOT NULL DEFAULT 0, type_tax_use TEXT, company_id BIGINT, tax_group_id BIGINT, fiscal_position_ids TEXT, original_tax_ids TEXT, children_tax_ids TEXT, is_domestic BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 43, Name: "account_tax_group", SQL: `CREATE TABLE IF NOT EXISTS account_tax_group (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, country_id BIGINT, tax_payable_account_id BIGINT, tax_receivable_account_id BIGINT, company_id BIGINT, sequence INTEGER NOT NULL DEFAULT 10)`},
	{Version: 44, Name: "account_tax_repartition_line", SQL: `CREATE TABLE IF NOT EXISTS account_tax_repartition_line (id BIGSERIAL PRIMARY KEY, tax_id BIGINT NOT NULL, company_id BIGINT, document_type TEXT, repartition_type TEXT, factor_percent NUMERIC, account_id BIGINT, tag_ids TEXT, sequence INTEGER NOT NULL DEFAULT 1)`},
	{Version: 45, Name: "account_fiscal_position", SQL: `CREATE TABLE IF NOT EXISTS account_fiscal_position (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, company_id BIGINT, auto_apply BOOLEAN NOT NULL DEFAULT false, country_id BIGINT, sequence INTEGER, account_ids TEXT, tax_ids TEXT)`},
	{Version: 46, Name: "account_partial_reconcile", SQL: `CREATE TABLE IF NOT EXISTS account_partial_reconcile (id BIGSERIAL PRIMARY KEY, debit_move_id BIGINT NOT NULL, credit_move_id BIGINT NOT NULL, amount NUMERIC NOT NULL, debit_amount_currency NUMERIC, credit_amount_currency NUMERIC, full_reconcile_id BIGINT, company_id BIGINT)`},
	{Version: 47, Name: "account_full_reconcile", SQL: `CREATE TABLE IF NOT EXISTS account_full_reconcile (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, partial_reconcile_ids TEXT)`},
	{Version: 48, Name: "account_reconcile_model", SQL: `CREATE TABLE IF NOT EXISTS account_reconcile_model (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, sequence INTEGER NOT NULL DEFAULT 10, rule_type TEXT, company_id BIGINT)`},
	{Version: 49, Name: "account_reconcile_model_line", SQL: `CREATE TABLE IF NOT EXISTS account_reconcile_model_line (id BIGSERIAL PRIMARY KEY, model_id BIGINT, account_id BIGINT, company_id BIGINT, amount_type TEXT, amount NUMERIC)`},
	{Version: 50, Name: "account_report", SQL: `CREATE TABLE IF NOT EXISTS account_report (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, root_report_id BIGINT, country_id BIGINT, line_ids TEXT, column_ids TEXT)`},
	{Version: 51, Name: "account_report_line", SQL: `CREATE TABLE IF NOT EXISTS account_report_line (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, report_id BIGINT, parent_id BIGINT, children_ids TEXT, expression_ids TEXT)`},
	{Version: 52, Name: "account_report_expression", SQL: `CREATE TABLE IF NOT EXISTS account_report_expression (id BIGSERIAL PRIMARY KEY, label TEXT NOT NULL, engine TEXT, formula TEXT, report_line_id BIGINT)`},
	{Version: 53, Name: "account_report_column", SQL: `CREATE TABLE IF NOT EXISTS account_report_column (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, expression_label TEXT, figure_type TEXT, report_id BIGINT)`},
	{Version: 54, Name: "account_report_external_value", SQL: `CREATE TABLE IF NOT EXISTS account_report_external_value (id BIGSERIAL PRIMARY KEY, name TEXT, target_report_expression_id BIGINT, company_id BIGINT, date DATE, value NUMERIC)`},
	{Version: 55, Name: "ai_agent", SQL: `CREATE TABLE IF NOT EXISTS ai_agent (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, subtitle TEXT, purpose TEXT, system_prompt TEXT, response_style TEXT NOT NULL DEFAULT 'balanced', llm_model TEXT NOT NULL DEFAULT 'gpt-4o', restrict_to_sources BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true, image_128 BYTEA, avatar_128 BYTEA, topic_ids TEXT, partner_id BIGINT, is_system_agent BOOLEAN NOT NULL DEFAULT false, source_ids TEXT, sources_ids TEXT, sources_fully_processed BOOLEAN NOT NULL DEFAULT true, is_ask_ai_agent BOOLEAN NOT NULL DEFAULT false, tool_allowlist TEXT, company_id BIGINT)`},
	{Version: 56, Name: "ai_topic", SQL: `CREATE TABLE IF NOT EXISTS ai_topic (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, instructions TEXT, tool_ids TEXT, active BOOLEAN NOT NULL DEFAULT true, company_id BIGINT)`},
	{Version: 57, Name: "ai_agent_source", SQL: `CREATE TABLE IF NOT EXISTS ai_agent_source (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, agent_id BIGINT, type TEXT NOT NULL DEFAULT 'binary', source_type TEXT, res_model TEXT, res_id BIGINT, status TEXT NOT NULL DEFAULT 'processing', is_active BOOLEAN NOT NULL DEFAULT false, error_details TEXT, attachment_id BIGINT, mimetype TEXT, file_size INTEGER, url TEXT, content TEXT, state TEXT, embedding_model TEXT, user_has_access BOOLEAN NOT NULL DEFAULT false, company_id BIGINT)`},
	{Version: 58, Name: "ai_prompt_button", SQL: `CREATE TABLE IF NOT EXISTS ai_prompt_button (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, prompt TEXT, model_name TEXT, use_in_ai BOOLEAN NOT NULL DEFAULT true, sequence INTEGER NOT NULL DEFAULT 10, composer_id BIGINT, active BOOLEAN NOT NULL DEFAULT true, company_id BIGINT)`},
	{Version: 59, Name: "ai_embedding", SQL: `CREATE TABLE IF NOT EXISTS ai_embedding (id BIGSERIAL PRIMARY KEY, agent_source_id BIGINT, attachment_id BIGINT, checksum TEXT, sequence INTEGER NOT NULL DEFAULT 10, res_model TEXT, res_id BIGINT, content TEXT NOT NULL, chunk_index INTEGER, embedding_model TEXT NOT NULL, has_embedding_generation_failed BOOLEAN NOT NULL DEFAULT false, embedding_vector TEXT, metadata TEXT, company_id BIGINT)`},
	{Version: 60, Name: "ai_composer", SQL: `CREATE TABLE IF NOT EXISTS ai_composer (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, interface_key TEXT, focused_models TEXT, ai_agent BIGINT, default_prompt TEXT, available_prompt_ids TEXT, available_prompts TEXT, is_system_default BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 61, Name: "ai_settings", SQL: `CREATE TABLE IF NOT EXISTS ai_settings (id BIGSERIAL PRIMARY KEY, name TEXT, default_provider TEXT, default_chat_model TEXT, default_embedding_model TEXT, token_budget INTEGER, rate_limit_per_minute INTEGER, prompt_system TEXT, prompt_context TEXT, prompt_safety TEXT, secret_source TEXT, secret_ref TEXT, openai_key_enabled BOOLEAN NOT NULL DEFAULT false, openai_key TEXT, google_key_enabled BOOLEAN NOT NULL DEFAULT false, google_key TEXT, company_id BIGINT)`},
	{Version: 62, Name: "approval_settings", SQL: `CREATE TABLE IF NOT EXISTS approval_settings (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, model TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, state_field TEXT NOT NULL DEFAULT 'state', draft_state TEXT, approved_state TEXT, rejected_state TEXT, cancelled_state TEXT, company_id BIGINT)`},
	{Version: 63, Name: "approval_settings_state", SQL: `CREATE TABLE IF NOT EXISTS approval_settings_state (id BIGSERIAL PRIMARY KEY, settings_id BIGINT NOT NULL, value TEXT NOT NULL, name TEXT NOT NULL, sequence INTEGER NOT NULL DEFAULT 10, group_ids TEXT, tag_ids TEXT, kind TEXT, condition_domain TEXT, activity_delay_days INTEGER, activity_summary TEXT, require_all_approvers BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 64, Name: "approval_config", SQL: `CREATE TABLE IF NOT EXISTS approval_config (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, model_id BIGINT, model TEXT NOT NULL, model_name TEXT, setting_id BIGINT, settings_id BIGINT, state TEXT, sequence INTEGER NOT NULL DEFAULT 10, group_ids TEXT, user_python_code TEXT, condition TEXT, schedule_activity BOOLEAN NOT NULL DEFAULT true, schedule_activity_field_id BIGINT, schedule_activity_days INTEGER, button_ids TEXT, escalation_ids TEXT, escalation_count INTEGER, tag_ids TEXT, auto_approve BOOLEAN NOT NULL DEFAULT false, default_mail_template_body TEXT, default_reject_mail_template_body TEXT, committee BOOLEAN NOT NULL DEFAULT false, committee_limit INTEGER, committee_vote_percentage DOUBLE PRECISION, user_ids TEXT, is_voting BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 65, Name: "approval_buttons", SQL: `CREATE TABLE IF NOT EXISTS approval_buttons (id BIGSERIAL PRIMARY KEY, settings_id BIGINT NOT NULL, config_id BIGINT, model TEXT, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, name TEXT NOT NULL, action_type TEXT NOT NULL, state_value TEXT, next_state TEXT, return_state TEXT, transfer_state TEXT, visible_to TEXT, method TEXT, visible_domain TEXT, server_action_id BIGINT, email_template_id BIGINT, email_wizard_form_id BIGINT, email_next_action TEXT, group_ids TEXT, comment_required BOOLEAN NOT NULL DEFAULT false, confirm_message TEXT, button_class TEXT, vote_threshold INTEGER)`},
	{Version: 66, Name: "approval_automation", SQL: `CREATE TABLE IF NOT EXISTS approval_automation (id BIGSERIAL PRIMARY KEY, settings_id BIGINT NOT NULL, model TEXT, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, name TEXT NOT NULL, trigger TEXT, from_states TEXT, to_states TEXT, code TEXT, filter_domain TEXT, template_ids TEXT, server_action_ids TEXT, action_dsl TEXT)`},
	{Version: 67, Name: "approval_escalation", SQL: `CREATE TABLE IF NOT EXISTS approval_escalation (id BIGSERIAL PRIMARY KEY, settings_id BIGINT NOT NULL, name TEXT NOT NULL, state_value TEXT, delay_seconds INTEGER, user_id BIGINT, server_action_id BIGINT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 68, Name: "approval_log", SQL: `CREATE TABLE IF NOT EXISTS approval_log (id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL, record_id BIGINT NOT NULL, user_id BIGINT, date TIMESTAMPTZ, description TEXT, old_state TEXT, new_state TEXT, old_status TEXT, new_status TEXT, duration_seconds INTEGER, approval_button_id BIGINT, bulk_approval BOOLEAN NOT NULL DEFAULT false, old_node_id BIGINT, new_node_id BIGINT, workflow_transition_id BIGINT, delegation_id BIGINT, delegation_employee_id BIGINT)`},
	{Version: 69, Name: "approval_log_voting", SQL: `CREATE TABLE IF NOT EXISTS approval_log_voting (id BIGSERIAL PRIMARY KEY, user_id BIGINT, vote TEXT, button_id BIGINT, comment TEXT, model TEXT NOT NULL, record_id BIGINT NOT NULL, state TEXT, button_class TEXT)`},
	{Version: 70, Name: "approval_forward", SQL: `CREATE TABLE IF NOT EXISTS approval_forward (id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL, record_id BIGINT NOT NULL, approval_state_id BIGINT, state_value TEXT, active BOOLEAN NOT NULL DEFAULT true, user_id BIGINT NOT NULL, forwarder_user_id BIGINT)`},
	{Version: 71, Name: "cancellation_record", SQL: `CREATE TABLE IF NOT EXISTS cancellation_record (id BIGSERIAL PRIMARY KEY, name TEXT, requester_id BIGINT, model TEXT NOT NULL, record_id BIGINT NOT NULL, reason TEXT, state TEXT)`},
	{Version: 72, Name: "state_tags", SQL: `CREATE TABLE IF NOT EXISTS state_tags (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, color INTEGER, description TEXT)`},
	{Version: 73, Name: "workflow_workflow", SQL: `CREATE TABLE IF NOT EXISTS workflow_workflow (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, approval_settings_id BIGINT, model TEXT, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, condition TEXT, state TEXT, view_id BIGINT, create_context TEXT, on_create BOOLEAN NOT NULL DEFAULT false, company_id BIGINT, company_ids TEXT, start_node_id BIGINT)`},
	{Version: 74, Name: "workflow_node", SQL: `CREATE TABLE IF NOT EXISTS workflow_node (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, workflow_id BIGINT NOT NULL, model_id BIGINT, model TEXT, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, type TEXT, responsible_group_ids TEXT, responsible_user_ids TEXT, responsible_python_code TEXT, responsible_value TEXT, responsible_filter TEXT, responsible_condition TEXT, responsible_committee BOOLEAN NOT NULL DEFAULT false, responsible_committee_limit INTEGER, schedule_activity BOOLEAN NOT NULL DEFAULT false, schedule_activity_field_id BIGINT, schedule_activity_days INTEGER, schedule_activity_enabled BOOLEAN NOT NULL DEFAULT false, state TEXT, button_type TEXT, button_name TEXT, button_context TEXT, button_icon TEXT, button_validate_form BOOLEAN NOT NULL DEFAULT false, wizard_view_id BIGINT, allow_forward BOOLEAN NOT NULL DEFAULT false, escalation BOOLEAN NOT NULL DEFAULT false, escalation_delay_type TEXT, escalation_delay INTEGER, escalation_node_id BIGINT, trg_date_calendar_id BIGINT)`},
	{Version: 75, Name: "workflow_transition", SQL: `CREATE TABLE IF NOT EXISTS workflow_transition (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, node_id BIGINT NOT NULL, workflow_id BIGINT, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, run_as_superuser BOOLEAN NOT NULL DEFAULT false, condition TEXT, code TEXT, next_node_id BIGINT NOT NULL, groups_ids TEXT, comment TEXT, button_class TEXT, context TEXT, icon TEXT, committee BOOLEAN NOT NULL DEFAULT false, committee_limit INTEGER, validate_form BOOLEAN NOT NULL DEFAULT false, trigger TEXT, is_email BOOLEAN NOT NULL DEFAULT false, email_template_id BIGINT, is_hidden BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 76, Name: "workflow_node_action", SQL: `CREATE TABLE IF NOT EXISTS workflow_node_action (id BIGSERIAL PRIMARY KEY, node_id BIGINT NOT NULL, sequence INTEGER NOT NULL DEFAULT 10, active BOOLEAN NOT NULL DEFAULT true, condition TEXT, server_action_id BIGINT NOT NULL, action_key TEXT)`},
	{Version: 77, Name: "workflow_process", SQL: `CREATE TABLE IF NOT EXISTS workflow_process (id BIGSERIAL PRIMARY KEY, workflow_id BIGINT, model TEXT NOT NULL, record_id BIGINT NOT NULL, node_id BIGINT, active BOOLEAN NOT NULL DEFAULT true, state TEXT, last_transition_id BIGINT, started_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, approval_user_ids TEXT, approval_done_user_ids TEXT, approval_partner_ids TEXT, user_can_approve BOOLEAN NOT NULL DEFAULT false, escalation_date TIMESTAMPTZ)`},
	{Version: 78, Name: "delegation", SQL: `CREATE TABLE IF NOT EXISTS delegation (id BIGSERIAL PRIMARY KEY, name TEXT, date_from DATE NOT NULL, date_to DATE NOT NULL, employee_id BIGINT, user_id BIGINT NOT NULL, one_employee BOOLEAN NOT NULL DEFAULT false, delegate_to_employee_id BIGINT, delegate_to_user_id BIGINT, isactive BOOLEAN NOT NULL DEFAULT false, state TEXT NOT NULL DEFAULT 'draft', department_ids TEXT, source_model TEXT, source_record_id BIGINT, metadata TEXT)`},
	{Version: 79, Name: "delegation_line", SQL: `CREATE TABLE IF NOT EXISTS delegation_line (id BIGSERIAL PRIMARY KEY, delegation_id BIGINT NOT NULL, group_id BIGINT NOT NULL, employee_id BIGINT, user_id BIGINT NOT NULL, one_employee BOOLEAN NOT NULL DEFAULT false, delegator_id BIGINT, delegator_user_id BIGINT NOT NULL, state TEXT NOT NULL DEFAULT 'draft', date_from DATE NOT NULL, date_to DATE NOT NULL, active BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 80, Name: "delegation_mail_template_metadata", SQL: `CREATE TABLE IF NOT EXISTS delegation_mail_template_metadata (id BIGSERIAL PRIMARY KEY, xml_id TEXT NOT NULL, name TEXT NOT NULL, subject TEXT, purpose TEXT, delegation_group_ids TEXT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 81, Name: "login_as_audit", SQL: `CREATE TABLE IF NOT EXISTS login_as_audit (id BIGSERIAL PRIMARY KEY, action TEXT NOT NULL, actor_id BIGINT NOT NULL, effective_user_id BIGINT NOT NULL, target_user_id BIGINT, session_id TEXT NOT NULL, model TEXT, record_id BIGINT, ip_address TEXT, user_agent TEXT, details TEXT, created_at TIMESTAMPTZ NOT NULL)`},
	{Version: 82, Name: "login_as_route", SQL: `CREATE TABLE IF NOT EXISTS login_as_route (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL, method TEXT NOT NULL, auth TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true, requires_setting TEXT)`},
	{Version: 83, Name: "ir_sequence", SQL: `CREATE TABLE IF NOT EXISTS ir_sequence (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT NOT NULL, prefix TEXT, suffix TEXT, padding INTEGER NOT NULL DEFAULT 4, number_next INTEGER NOT NULL DEFAULT 1, number_next_actual INTEGER, number_increment INTEGER NOT NULL DEFAULT 1, company_id BIGINT, active BOOLEAN NOT NULL DEFAULT true, implementation TEXT, use_date_range BOOLEAN NOT NULL DEFAULT false, date_range_ids TEXT)`},
	{Version: 84, Name: "ir_module_category", SQL: `CREATE TABLE IF NOT EXISTS ir_module_category (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, parent_id BIGINT, child_ids TEXT, module_ids TEXT, privilege_ids TEXT, description TEXT, sequence INTEGER, visible BOOLEAN NOT NULL DEFAULT true, exclusive BOOLEAN NOT NULL DEFAULT false, xml_id TEXT)`},
	{Version: 85, Name: "ir_default", SQL: `CREATE TABLE IF NOT EXISTS ir_default (id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL, field TEXT NOT NULL, value TEXT)`},
	{Version: 86, Name: "res_users", SQL: `CREATE TABLE IF NOT EXISTS res_users (id BIGSERIAL PRIMARY KEY, login TEXT, password TEXT, email TEXT, name TEXT, active BOOLEAN NOT NULL DEFAULT true, active_partner BOOLEAN NOT NULL DEFAULT true, company_id BIGINT, company_ids TEXT, groups_id TEXT, group_ids TEXT, all_group_ids TEXT, accesses_count INTEGER, rules_count INTEGER, groups_count INTEGER, view_group_hierarchy JSONB, role TEXT, partner_id BIGINT, commercial_partner_id BIGINT, share BOOLEAN NOT NULL DEFAULT false, signature TEXT, image_1920 TEXT)`},
	{Version: 87, Name: "res_groups", SQL: `CREATE TABLE IF NOT EXISTS res_groups (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, full_name TEXT, share BOOLEAN NOT NULL DEFAULT false, sequence INTEGER, category_id BIGINT, privilege_id BIGINT, implied_ids TEXT, all_implied_ids TEXT, implied_by_ids TEXT, all_implied_by_ids TEXT, disjoint_ids TEXT, user_ids TEXT, all_user_ids TEXT, all_users_count INTEGER, model_access TEXT, rule_groups TEXT, menu_access TEXT, view_access TEXT, comment TEXT, api_key_duration DOUBLE PRECISION, view_group_hierarchy JSONB, allow_delegation BOOLEAN NOT NULL DEFAULT false, delegation_template_ids TEXT, name_delegation TEXT, allow_multiple_delegation BOOLEAN NOT NULL DEFAULT false, restricted_access BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 88, Name: "res_company", SQL: `CREATE TABLE IF NOT EXISTS res_company (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, parent_id BIGINT, currency_id BIGINT, country_id BIGINT, partner_id BIGINT, fiscalyear_lock_date DATE, tax_lock_date DATE, sale_lock_date DATE, purchase_lock_date DATE, hard_lock_date DATE, user_fiscalyear_lock_date DATE, user_tax_lock_date DATE, user_sale_lock_date DATE, user_purchase_lock_date DATE, user_hard_lock_date DATE, restrictive_audit_trail BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 89, Name: "res_partner", SQL: `CREATE TABLE IF NOT EXISTS res_partner (id BIGSERIAL PRIMARY KEY, name TEXT, active BOOLEAN NOT NULL DEFAULT true, email TEXT, email_normalized TEXT, message_bounce BIGINT NOT NULL DEFAULT 0, phone TEXT, phone_sanitized TEXT, phone_blacklisted BOOLEAN NOT NULL DEFAULT false, street TEXT, city TEXT, zip TEXT, country_id BIGINT, state_id BIGINT, company_id BIGINT, parent_id BIGINT, commercial_partner_id BIGINT, partner_share BOOLEAN NOT NULL DEFAULT false, is_company BOOLEAN NOT NULL DEFAULT false, image_1920 TEXT, signup_type TEXT, agent_ids TEXT)`},
	{Version: 90, Name: "res_currency", SQL: `CREATE TABLE IF NOT EXISTS res_currency (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, symbol TEXT, position TEXT, rounding DOUBLE PRECISION, decimal_places INTEGER, iso_numeric TEXT, full_name TEXT, currency_unit_label TEXT, currency_subunit_label TEXT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 91, Name: "res_country", SQL: `CREATE TABLE IF NOT EXISTS res_country (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, address_format TEXT, currency_id BIGINT, phone_code INTEGER, vat_label TEXT, state_required BOOLEAN NOT NULL DEFAULT false, zip_required BOOLEAN NOT NULL DEFAULT false, name_position TEXT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 92, Name: "res_country_state", SQL: `CREATE TABLE IF NOT EXISTS res_country_state (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, country_id BIGINT)`},
	{Version: 93, Name: "res_lang", SQL: `CREATE TABLE IF NOT EXISTS res_lang (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT NOT NULL, iso_code TEXT, url_code TEXT, direction TEXT, grouping TEXT, decimal_point TEXT, thousands_sep TEXT, date_format TEXT, time_format TEXT, week_start TEXT, flag_image TEXT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 94, Name: "res_bank", SQL: `CREATE TABLE IF NOT EXISTS res_bank (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, bic TEXT, active BOOLEAN NOT NULL DEFAULT true, country BIGINT)`},
	{Version: 95, Name: "res_partner_bank", SQL: `CREATE TABLE IF NOT EXISTS res_partner_bank (id BIGSERIAL PRIMARY KEY, acc_number TEXT NOT NULL, partner_id BIGINT, bank_id BIGINT, company_id BIGINT, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 96, Name: "res_groups_privilege", SQL: `CREATE TABLE IF NOT EXISTS res_groups_privilege (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, placeholder TEXT, sequence INTEGER, category_id BIGINT, group_ids TEXT)`},
	{Version: 97, Name: "res_users_settings", SQL: `CREATE TABLE IF NOT EXISTS res_users_settings (id BIGSERIAL PRIMARY KEY, user_id BIGINT, is_discuss_sidebar_category_channel_open BOOLEAN NOT NULL DEFAULT true, is_discuss_sidebar_category_chat_open BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 98, Name: "res_users_apikeys", SQL: `CREATE TABLE IF NOT EXISTS res_users_apikeys (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, user_id BIGINT, scope TEXT, key TEXT, create_date TIMESTAMPTZ)`},
	{Version: 99, Name: "ir_attachment", SQL: `CREATE TABLE IF NOT EXISTS ir_attachment (id BIGSERIAL PRIMARY KEY, name TEXT, res_model TEXT, res_field TEXT, res_id BIGINT, company_id BIGINT, type TEXT, url TEXT, mimetype TEXT, datas TEXT, file_size BIGINT, public BOOLEAN NOT NULL DEFAULT false, access_token TEXT, checksum TEXT, has_thumbnail BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 100, Name: "ir_filters", SQL: `CREATE TABLE IF NOT EXISTS ir_filters (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, model_id TEXT, domain TEXT, context TEXT, sort TEXT, user_id BIGINT, action_id BIGINT, embedded_action_id BIGINT, is_default BOOLEAN NOT NULL DEFAULT false, active BOOLEAN NOT NULL DEFAULT true)`},
	{Version: 101, Name: "ir_ui_view", SQL: `CREATE TABLE IF NOT EXISTS ir_ui_view (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, model TEXT, type TEXT, arch TEXT, key TEXT, inherit_id BIGINT, inherit_id_ref TEXT, mode TEXT, priority INTEGER, active BOOLEAN NOT NULL DEFAULT true, groups_id TEXT, primary BOOLEAN NOT NULL DEFAULT false, customize_show BOOLEAN NOT NULL DEFAULT false, track BOOLEAN NOT NULL DEFAULT false, page BOOLEAN NOT NULL DEFAULT false, website_id BIGINT)`},
	{Version: 102, Name: "ir_asset", SQL: `CREATE TABLE IF NOT EXISTS ir_asset (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, bundle TEXT, directive TEXT, path TEXT, target TEXT, sequence INTEGER)`},
	{Version: 103, Name: "ir_ui_menu", SQL: `CREATE TABLE IF NOT EXISTS ir_ui_menu (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, parent_id BIGINT, action TEXT, sequence INTEGER, groups_id TEXT, web_icon TEXT, web_icon_data TEXT, web_icon_data_mimetype TEXT)`},
	{Version: 104, Name: "ir_actions", SQL: `CREATE TABLE IF NOT EXISTS ir_actions (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT, xml_id TEXT, help TEXT, path TEXT, binding_model_id BIGINT, binding_type TEXT DEFAULT 'action', binding_view_types TEXT DEFAULT 'list,form', effect JSONB, infos JSONB)`},
	{Version: 105, Name: "ir_act_window", SQL: `CREATE TABLE IF NOT EXISTS ir_act_window (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT DEFAULT 'ir.actions.act_window', res_model TEXT, res_id BIGINT, view_mode TEXT DEFAULT 'list,form', views BYTEA, embedded_action_ids TEXT, close_on_report_download BOOLEAN NOT NULL DEFAULT false, mobile_view_mode TEXT DEFAULT 'kanban', view_id BIGINT, search_view_id BIGINT, domain TEXT, context TEXT DEFAULT '{}', target TEXT DEFAULT 'current', limit INTEGER DEFAULT 80, help TEXT, path TEXT, usage TEXT, filter BOOLEAN NOT NULL DEFAULT false, cache BOOLEAN NOT NULL DEFAULT true, group_ids TEXT, binding_model_id BIGINT, binding_type TEXT DEFAULT 'action', binding_view_types TEXT DEFAULT 'list,form')`},
	{Version: 106, Name: "ir_act_window_view", SQL: `CREATE TABLE IF NOT EXISTS ir_act_window_view (id BIGSERIAL PRIMARY KEY, sequence INTEGER, view_mode TEXT, view_id BIGINT, act_window_id BIGINT, multi BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 107, Name: "ir_act_url", SQL: `CREATE TABLE IF NOT EXISTS ir_act_url (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT DEFAULT 'ir.actions.act_url', url TEXT, target TEXT DEFAULT 'new', close BOOLEAN NOT NULL DEFAULT false, binding_model_id BIGINT, binding_type TEXT DEFAULT 'action', binding_view_types TEXT DEFAULT 'list,form')`},
	{Version: 108, Name: "ir_actions_todo", SQL: `CREATE TABLE IF NOT EXISTS ir_actions_todo (id BIGSERIAL PRIMARY KEY, name TEXT, action_id BIGINT, state TEXT, sequence INTEGER)`},
	{Version: 109, Name: "ir_act_server", SQL: `CREATE TABLE IF NOT EXISTS ir_act_server (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT DEFAULT 'ir.actions.server', model_id BIGINT, binding_model_id BIGINT, binding_type TEXT DEFAULT 'action', binding_view_types TEXT DEFAULT 'list,form', model_name TEXT, automated_name TEXT, allowed_states JSONB, available_model_ids TEXT, state TEXT, active BOOLEAN NOT NULL DEFAULT true, usage TEXT DEFAULT 'ir_actions_server', sequence INTEGER NOT NULL DEFAULT 5, base_automation_id BIGINT, go_action_name TEXT, ir_cron_ids TEXT, values TEXT, value TEXT, html_value TEXT, evaluation_type TEXT DEFAULT 'value', sequence_id BIGINT, resource_ref TEXT, selection_value BIGINT, parent_id BIGINT, child_ids TEXT, crud_model_id BIGINT, crud_model_name TEXT, link_field_id BIGINT, group_ids TEXT, update_field_id BIGINT, update_path TEXT, update_related_model_id BIGINT, update_field_type TEXT, update_m2m_operation TEXT DEFAULT 'add', update_boolean_value TEXT DEFAULT 'true', warning TEXT, show_code_history BOOLEAN NOT NULL DEFAULT false, value_field_to_show TEXT, mail_template_id BIGINT, template_id BIGINT, mail_post_autofollow BOOLEAN NOT NULL DEFAULT false, mail_post_method TEXT, followers_type TEXT, followers_partner_field_name TEXT, partner_ids TEXT, activity_type_id BIGINT, activity_summary TEXT, activity_note TEXT, activity_date_deadline_range INTEGER, activity_date_deadline_range_type TEXT, activity_user_type TEXT, activity_user_id BIGINT, activity_user_field_name TEXT, sms_template_id BIGINT, sms_method TEXT, wa_template_id BIGINT, documents_account_create_model TEXT, documents_account_journal_id BIGINT, documents_account_suitable_journal_ids TEXT, documents_account_move_type TEXT, queue_key TEXT, webhook_url TEXT, webhook_field_ids TEXT, webhook_sample_payload TEXT, code TEXT, ai_tool_ids TEXT, ai_action_prompt TEXT, ai_tool_show_warning BOOLEAN NOT NULL DEFAULT false, ai_tool_description TEXT, ai_tool_schema TEXT, use_in_ai BOOLEAN NOT NULL DEFAULT false, ai_tool_allow_end_message BOOLEAN NOT NULL DEFAULT false, ai_tool_is_candidate BOOLEAN NOT NULL DEFAULT false, ai_tool_has_schema BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 110, Name: "ir_act_client", SQL: `CREATE TABLE IF NOT EXISTS ir_act_client (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT DEFAULT 'ir.actions.client', tag TEXT, res_model TEXT, target TEXT DEFAULT 'current', context TEXT DEFAULT '{}', params JSONB, params_store BYTEA)`},
	{Version: 111, Name: "ir_act_report_xml", SQL: `CREATE TABLE IF NOT EXISTS ir_act_report_xml (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, type TEXT DEFAULT 'ir.actions.report', model TEXT, model_id BIGINT, report_name TEXT, report_type TEXT DEFAULT 'qweb-pdf', target TEXT, context TEXT, data JSONB, close_on_report_download BOOLEAN NOT NULL DEFAULT false, report_file TEXT, print_report_name TEXT, attachment TEXT, attachment_use BOOLEAN NOT NULL DEFAULT false, multi BOOLEAN NOT NULL DEFAULT false, binding_model_id BIGINT, binding_type TEXT DEFAULT 'report', binding_view_types TEXT DEFAULT 'list,form', paperformat_id BIGINT, groups_id TEXT, domain TEXT, is_invoice_report BOOLEAN NOT NULL DEFAULT false)`},
	{Version: 112, Name: "account_cancel_reversal_metadata", SQL: `
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS invoice_date DATE;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS invoice_date_due DATE;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS auto_post TEXT NOT NULL DEFAULT 'no';
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS need_cancel_request BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS show_reset_to_draft_button BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS reversed_entry_id BIGINT;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS move_id BIGINT;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS need_cancel_request BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 113, Name: "account_payment_state_metadata", SQL: `
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS amount_residual_signed NUMERIC;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS payment_state TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS status_in_payment TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS is_move_sent BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS origin_payment_id BIGINT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS statement_line_id BIGINT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS matched_payment_ids TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS reconciled_payment_ids TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS payment_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS amount_residual_currency NUMERIC;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS payment_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS full_reconcile_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS matched_debit_ids TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS matched_credit_ids TEXT;
ALTER TABLE account_partial_reconcile ADD COLUMN IF NOT EXISTS debit_amount_currency NUMERIC;
ALTER TABLE account_partial_reconcile ADD COLUMN IF NOT EXISTS credit_amount_currency NUMERIC;
ALTER TABLE account_partial_reconcile ADD COLUMN IF NOT EXISTS full_reconcile_id BIGINT;
`},
	{Version: 114, Name: "account_payment_reconciliation_metadata", SQL: `
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS is_reconciled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS is_matched BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS is_sent BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS invoice_ids TEXT;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS reconciled_invoice_ids TEXT;
ALTER TABLE account_payment ADD COLUMN IF NOT EXISTS reconciled_bill_ids TEXT;
`},
	{Version: 115, Name: "account_move_reversal", SQL: `CREATE TABLE IF NOT EXISTS account_move_reversal (id BIGSERIAL PRIMARY KEY, move_ids TEXT, new_move_ids TEXT, date DATE, reason TEXT, journal_id BIGINT, company_id BIGINT, available_journal_ids TEXT, country_code TEXT, residual NUMERIC, currency_id BIGINT, move_type TEXT)`},
	{Version: 116, Name: "ir_act_window_view_group_fields", SQL: `
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS view_id BIGINT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS group_ids TEXT;
`},
	{Version: 117, Name: "account_payment_register", SQL: `CREATE TABLE IF NOT EXISTS account_payment_register (
id BIGSERIAL PRIMARY KEY,
payment_date DATE,
amount NUMERIC,
hide_writeoff_section BOOLEAN NOT NULL DEFAULT false,
communication TEXT,
group_payment BOOLEAN NOT NULL DEFAULT false,
early_payment_discount_mode BOOLEAN NOT NULL DEFAULT false,
currency_id BIGINT,
journal_id BIGINT,
available_journal_ids TEXT,
available_partner_bank_ids TEXT,
partner_bank_id BIGINT,
company_currency_id BIGINT,
qr_code TEXT,
batches BYTEA,
installments_mode TEXT,
installments_switch_html TEXT,
installments_switch_amount NUMERIC,
custom_user_amount NUMERIC,
custom_user_currency_id BIGINT,
line_ids TEXT,
payment_type TEXT,
partner_type TEXT,
source_amount NUMERIC,
source_amount_currency NUMERIC,
source_currency_id BIGINT,
can_edit_wizard BOOLEAN NOT NULL DEFAULT false,
can_group_payments BOOLEAN NOT NULL DEFAULT false,
company_id BIGINT,
partner_id BIGINT,
payment_method_line_id BIGINT,
available_payment_method_line_ids TEXT,
payment_method_code TEXT,
payment_difference NUMERIC,
payment_difference_handling TEXT,
writeoff_account_id BIGINT,
writeoff_label TEXT,
writeoff_is_exchange_account BOOLEAN NOT NULL DEFAULT false,
show_payment_difference BOOLEAN NOT NULL DEFAULT false,
show_partner_bank_account BOOLEAN NOT NULL DEFAULT false,
require_partner_bank_account BOOLEAN NOT NULL DEFAULT false,
country_code TEXT,
duplicate_payment_ids TEXT,
is_register_payment_on_draft BOOLEAN NOT NULL DEFAULT false,
actionable_errors TEXT,
untrusted_bank_ids TEXT,
total_payments_amount INTEGER NOT NULL DEFAULT 0,
untrusted_payments_count INTEGER NOT NULL DEFAULT 0,
missing_account_partners TEXT
)`},
	{Version: 118, Name: "account_move_send_fields", SQL: `
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS sending_data TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS is_being_sent BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS invoice_pdf_report_id BIGINT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS invoice_pdf_report_file TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS message_main_attachment_id BIGINT;
`},
	{Version: 119, Name: "account_move_send_wizard", SQL: `CREATE TABLE IF NOT EXISTS account_move_send_wizard (
id BIGSERIAL PRIMARY KEY,
move_id BIGINT,
company_id BIGINT,
alerts TEXT,
sending_methods TEXT,
sending_method_checkboxes TEXT,
display_attachments_widget BOOLEAN NOT NULL DEFAULT false,
extra_edis TEXT,
extra_edi_checkboxes TEXT,
invoice_edi_format TEXT,
pdf_report_id BIGINT,
available_pdf_report_ids TEXT,
display_pdf_report_id BOOLEAN NOT NULL DEFAULT false,
template_id BIGINT,
lang TEXT,
mail_partner_ids TEXT,
mail_attachments_widget TEXT,
attachments_not_supported TEXT,
model TEXT,
res_ids TEXT,
template_name TEXT,
subject TEXT,
body TEXT,
render_model TEXT,
can_edit_body BOOLEAN NOT NULL DEFAULT false
)`},
	{Version: 120, Name: "account_move_send_batch_wizard", SQL: `CREATE TABLE IF NOT EXISTS account_move_send_batch_wizard (
id BIGSERIAL PRIMARY KEY,
move_ids TEXT,
summary_data TEXT,
alerts TEXT
)`},
	{Version: 121, Name: "account_report_send_download_fields", SQL: `
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS is_invoice_report BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS message_main_attachment_id BIGINT;
`},
	{Version: 122, Name: "action_menu_payload_fields", SQL: `
ALTER TABLE ir_ui_menu ADD COLUMN IF NOT EXISTS web_icon_data TEXT;
ALTER TABLE ir_ui_menu ADD COLUMN IF NOT EXISTS web_icon_data_mimetype TEXT;
ALTER TABLE ir_ui_menu ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS help TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS path TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS type TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS res_id BIGINT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS mobile_view_mode TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS search_view_id BIGINT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS limit INTEGER;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS help TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS path TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS usage TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS filter BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS cache BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 123, Name: "cron_mail_thread_payload_fields", SQL: `
ALTER TABLE ir_cron ADD COLUMN IF NOT EXISTS ir_actions_server_id BIGINT;
ALTER TABLE ir_cron ADD COLUMN IF NOT EXISTS cron_name TEXT;
ALTER TABLE ir_cron ADD COLUMN IF NOT EXISTS lastcall TIMESTAMPTZ;
ALTER TABLE ir_cron ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 5;
ALTER TABLE ir_cron ADD COLUMN IF NOT EXISTS first_failure_date TIMESTAMPTZ;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS parent_id BIGINT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS subtype_id BIGINT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS partner_ids TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS body_is_html BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 124, Name: "account_dependency_anchor_tables", SQL: `
CREATE TABLE IF NOT EXISTS res_config_settings (id BIGSERIAL PRIMARY KEY, company_id BIGINT, is_root_company BOOLEAN NOT NULL DEFAULT false, group_multi_currency BOOLEAN NOT NULL DEFAULT false, group_uom BOOLEAN NOT NULL DEFAULT false, group_analytic_accounting BOOLEAN NOT NULL DEFAULT false, digest_emails BOOLEAN NOT NULL DEFAULT false, digest_id BIGINT, portal_allow_api_keys BOOLEAN NOT NULL DEFAULT false, sale_tax_id BIGINT, purchase_tax_id BIGINT, chart_template TEXT, has_chart_of_accounts BOOLEAN NOT NULL DEFAULT false, has_accounting_entries BOOLEAN NOT NULL DEFAULT false, module_account_accountant BOOLEAN NOT NULL DEFAULT false, module_product_margin BOOLEAN NOT NULL DEFAULT false, use_invoice_terms BOOLEAN NOT NULL DEFAULT false, country_code TEXT, income_account_id BIGINT, expense_account_id BIGINT);
CREATE TABLE IF NOT EXISTS uom_category (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS uom_uom (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, category_id BIGINT, uom_type TEXT, factor DOUBLE PRECISION, factor_inv DOUBLE PRECISION, rounding DOUBLE PRECISION, active BOOLEAN NOT NULL DEFAULT true);
CREATE TABLE IF NOT EXISTS product_category (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, complete_name TEXT, parent_id BIGINT, child_id TEXT, parent_path TEXT, product_count INTEGER NOT NULL DEFAULT 0, property_account_income_categ_id BIGINT, property_account_expense_categ_id BIGINT);
CREATE TABLE IF NOT EXISTS product_tag (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, color INTEGER);
CREATE TABLE IF NOT EXISTS product_template (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, sequence INTEGER NOT NULL DEFAULT 1, description TEXT, description_purchase TEXT, description_sale TEXT, type TEXT, detailed_type TEXT, categ_id BIGINT, currency_id BIGINT, cost_currency_id BIGINT, list_price DOUBLE PRECISION, standard_price DOUBLE PRECISION, volume DOUBLE PRECISION, weight DOUBLE PRECISION, sale_ok BOOLEAN NOT NULL DEFAULT true, purchase_ok BOOLEAN NOT NULL DEFAULT true, uom_id BIGINT, uom_ids TEXT, uom_po_id BIGINT, uom_name TEXT, company_id BIGINT, seller_ids TEXT, active BOOLEAN NOT NULL DEFAULT true, color INTEGER, product_variant_ids TEXT, product_variant_id BIGINT, product_variant_count INTEGER NOT NULL DEFAULT 0, barcode TEXT, default_code TEXT, is_favorite BOOLEAN NOT NULL DEFAULT false, product_tag_ids TEXT, taxes_id TEXT, tax_string TEXT, supplier_taxes_id TEXT, property_account_income_id BIGINT, property_account_expense_id BIGINT, account_tag_ids TEXT, fiscal_country_codes TEXT);
CREATE TABLE IF NOT EXISTS product_product (id BIGSERIAL PRIMARY KEY, name TEXT, product_tmpl_id BIGINT, default_code TEXT, barcode TEXT, active BOOLEAN NOT NULL DEFAULT true, company_id BIGINT, categ_id BIGINT, uom_id BIGINT, uom_po_id BIGINT, lst_price DOUBLE PRECISION, standard_price DOUBLE PRECISION, taxes_id TEXT, supplier_taxes_id TEXT, tax_string TEXT, display_name TEXT);
CREATE TABLE IF NOT EXISTS product_supplierinfo (id BIGSERIAL PRIMARY KEY, name BIGINT, partner_id BIGINT, product_tmpl_id BIGINT, product_id BIGINT, company_id BIGINT, sequence INTEGER, min_qty DOUBLE PRECISION, price DOUBLE PRECISION, currency_id BIGINT, delay INTEGER);
CREATE TABLE IF NOT EXISTS product_pricelist (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, currency_id BIGINT, company_id BIGINT, item_ids TEXT);
CREATE TABLE IF NOT EXISTS product_pricelist_item (id BIGSERIAL PRIMARY KEY, name TEXT, pricelist_id BIGINT, product_tmpl_id BIGINT, product_id BIGINT, categ_id BIGINT, applied_on TEXT, compute_price TEXT, fixed_price DOUBLE PRECISION, percent_price DOUBLE PRECISION);
CREATE TABLE IF NOT EXISTS product_document (id BIGSERIAL PRIMARY KEY, name TEXT, res_model TEXT, res_id BIGINT, ir_attachment_id BIGINT, company_id BIGINT);
CREATE TABLE IF NOT EXISTS account_analytic_plan (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, active BOOLEAN NOT NULL DEFAULT true, sequence INTEGER, parent_id BIGINT, children_ids TEXT, root_id BIGINT, default_applicability TEXT, applicability_ids TEXT, company_id BIGINT);
CREATE TABLE IF NOT EXISTS account_analytic_applicability (id BIGSERIAL PRIMARY KEY, analytic_plan_id BIGINT, business_domain TEXT, applicability TEXT, account_prefix TEXT, product_categ_id BIGINT, display_account_prefix BOOLEAN NOT NULL DEFAULT false, account_prefix_placeholder TEXT, company_id BIGINT);
CREATE TABLE IF NOT EXISTS account_analytic_account (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, active BOOLEAN NOT NULL DEFAULT true, partner_id BIGINT, company_id BIGINT, currency_id BIGINT, plan_id BIGINT, root_plan_id BIGINT, line_ids TEXT, debit NUMERIC, credit NUMERIC, balance NUMERIC, invoice_count INTEGER NOT NULL DEFAULT 0, vendor_bill_count INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS account_analytic_line (id BIGSERIAL PRIMARY KEY, name TEXT, date DATE, account_id BIGINT, auto_account_id BIGINT, partner_id BIGINT, user_id BIGINT, company_id BIGINT, currency_id BIGINT, amount NUMERIC, unit_amount DOUBLE PRECISION, product_id BIGINT, product_uom_id BIGINT, general_account_id BIGINT, journal_id BIGINT, move_line_id BIGINT, code TEXT, ref TEXT, category TEXT);
CREATE TABLE IF NOT EXISTS account_analytic_distribution_model (id BIGSERIAL PRIMARY KEY, name TEXT, sequence INTEGER, company_id BIGINT, partner_id BIGINT, partner_category_id BIGINT, account_prefix TEXT, account_id BIGINT, product_id BIGINT, product_categ_id BIGINT, analytic_distribution TEXT, analytic_precision INTEGER, prefix_placeholder TEXT);
CREATE TABLE IF NOT EXISTS digest_digest (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT true, company_id BIGINT, currency_id BIGINT, periodicity TEXT, next_run_date DATE, user_ids TEXT, available_fields TEXT, is_subscribed BOOLEAN NOT NULL DEFAULT false, state TEXT, kpi_res_users_connected BOOLEAN NOT NULL DEFAULT false, kpi_res_users_connected_value INTEGER, kpi_mail_message_total BOOLEAN NOT NULL DEFAULT false, kpi_mail_message_total_value INTEGER, kpi_account_total_revenue BOOLEAN NOT NULL DEFAULT false, kpi_account_total_revenue_value NUMERIC);
CREATE TABLE IF NOT EXISTS digest_tip (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, sequence INTEGER, group_id BIGINT, user_ids TEXT, tip_description TEXT);
CREATE TABLE IF NOT EXISTS onboarding_onboarding (id BIGSERIAL PRIMARY KEY, name TEXT, route_name TEXT NOT NULL, step_ids TEXT, text_completed TEXT, is_per_company BOOLEAN NOT NULL DEFAULT false, panel_close_action_name TEXT, current_progress_id BIGINT, current_onboarding_state TEXT, is_onboarding_closed BOOLEAN NOT NULL DEFAULT false, progress_ids TEXT, sequence INTEGER);
CREATE TABLE IF NOT EXISTS onboarding_onboarding_step (id BIGSERIAL PRIMARY KEY, onboarding_ids TEXT, title TEXT, description TEXT, button_text TEXT, done_icon TEXT, done_text TEXT, step_image BYTEA, step_image_filename TEXT, step_image_alt TEXT, panel_step_open_action_name TEXT, current_progress_step_id BIGINT, current_step_state TEXT, progress_ids TEXT, is_per_company BOOLEAN NOT NULL DEFAULT true, sequence INTEGER);
CREATE TABLE IF NOT EXISTS onboarding_progress (id BIGSERIAL PRIMARY KEY, onboarding_id BIGINT, company_id BIGINT, progress_step_ids TEXT, onboarding_state TEXT, is_onboarding_closed BOOLEAN NOT NULL DEFAULT false);
CREATE TABLE IF NOT EXISTS onboarding_progress_step (id BIGSERIAL PRIMARY KEY, step_id BIGINT, progress_ids TEXT, company_id BIGINT, step_state TEXT);
CREATE TABLE IF NOT EXISTS portal_wizard (id BIGSERIAL PRIMARY KEY, partner_ids TEXT, user_ids TEXT, welcome_message TEXT);
CREATE TABLE IF NOT EXISTS portal_wizard_user (id BIGSERIAL PRIMARY KEY, wizard_id BIGINT, partner_id BIGINT, email TEXT, user_id BIGINT, login_date TIMESTAMPTZ, is_portal BOOLEAN NOT NULL DEFAULT false, is_internal BOOLEAN NOT NULL DEFAULT false, email_state TEXT);
CREATE TABLE IF NOT EXISTS portal_share (id BIGSERIAL PRIMARY KEY, res_model TEXT, res_id BIGINT, partner_ids TEXT, note TEXT);
`},
	{Version: 125, Name: "account_dependency_anchor_fields", SQL: `
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS access_url TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS access_token TEXT;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS access_warning TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS parent_state TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS journal_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS quantity DOUBLE PRECISION;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS price_unit NUMERIC;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS price_subtotal NUMERIC;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS price_total NUMERIC;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS discount DOUBLE PRECISION;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS display_type TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS date_maturity DATE;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS product_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS product_uom_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS allowed_uom_ids TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS product_category_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS analytic_line_ids TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS analytic_distribution TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS distribution_analytic_account_ids TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS analytic_precision INTEGER;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS has_invalid_analytics BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 126, Name: "cron_progress_loop_fields", SQL: `
ALTER TABLE ir_cron_progress ADD COLUMN IF NOT EXISTS deactivate BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_cron_progress ADD COLUMN IF NOT EXISTS timed_out_counter INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ir_cron_progress ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE ir_cron_progress ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
`},
	{Version: 127, Name: "server_action_state_metadata_fields", SQL: `
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sequence INTEGER NOT NULL DEFAULT 5;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS value TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS html_value TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS evaluation_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sequence_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS resource_ref TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS selection_value BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS parent_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS child_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS crud_model_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS crud_model_name TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS link_field_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS group_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_field_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_path TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_related_model_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_field_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_m2m_operation TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_boolean_value TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS warning TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS template_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS mail_post_autofollow BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS mail_post_method TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS followers_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS followers_partner_field_name TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS partner_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_type_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_summary TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_note TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_date_deadline_range INTEGER;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_date_deadline_range_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_user_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_user_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS activity_user_field_name TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sms_template_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sms_method TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_create_model TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_journal_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_suitable_journal_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_move_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS webhook_field_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS webhook_sample_payload TEXT;
`},
	{Version: 128, Name: "ir_model_fields_groups_relation_field", SQL: `
ALTER TABLE ir_model_fields ADD COLUMN IF NOT EXISTS relation_field TEXT;
ALTER TABLE ir_model_fields ADD COLUMN IF NOT EXISTS groups TEXT;
`},
	{Version: 129, Name: "ir_sequence_date_range", SQL: `
ALTER TABLE ir_sequence ADD COLUMN IF NOT EXISTS number_next_actual INTEGER;
ALTER TABLE ir_sequence ADD COLUMN IF NOT EXISTS use_date_range BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_sequence ADD COLUMN IF NOT EXISTS date_range_ids TEXT;
CREATE TABLE IF NOT EXISTS ir_sequence_date_range (
  id BIGSERIAL PRIMARY KEY,
  date_from DATE NOT NULL,
  date_to DATE NOT NULL,
  sequence_id BIGINT NOT NULL,
  number_next INTEGER NOT NULL DEFAULT 1,
  number_next_actual INTEGER,
  UNIQUE(sequence_id, date_from, date_to)
);
`},
	{Version: 130, Name: "mail_activity_message_parity", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS automated BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS summary TEXT;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS default_user_id BIGINT;
`},
	{Version: 131, Name: "approval_automation_code", SQL: `
ALTER TABLE approval_automation ADD COLUMN IF NOT EXISTS code TEXT;
`},
	{Version: 132, Name: "mail_compose_message", SQL: `CREATE TABLE IF NOT EXISTS mail_compose_message (
  id BIGSERIAL PRIMARY KEY,
  composition_mode TEXT,
  composition_comment_option TEXT,
  message_type TEXT,
  subtype_is_log BOOLEAN NOT NULL DEFAULT false,
  model TEXT,
  res_id BIGINT,
  res_ids TEXT,
  template_id BIGINT,
  subject TEXT,
  body TEXT,
  body_html TEXT,
  email_from TEXT,
  email_to TEXT,
  email_cc TEXT,
  reply_to TEXT,
  partner_ids TEXT,
  attachment_ids TEXT,
  parent_id BIGINT,
  subtype_id BIGINT,
  scheduled_date TIMESTAMPTZ,
  mail_server_id BIGINT,
  author_id BIGINT,
  auto_delete BOOLEAN NOT NULL DEFAULT false,
  mass_mailing_id BIGINT,
  use_exclusion_list BOOLEAN NOT NULL DEFAULT true,
  notify BOOLEAN NOT NULL DEFAULT true,
  body_is_html BOOLEAN NOT NULL DEFAULT true
)`},
	{Version: 133, Name: "mail_scheduled_message", SQL: `CREATE TABLE IF NOT EXISTS mail_scheduled_message (
  id BIGSERIAL PRIMARY KEY,
  mail_message_id BIGINT,
  mail_mail_id BIGINT,
  mail_template_id BIGINT,
  model TEXT,
  res_id BIGINT,
  scheduled_date TIMESTAMPTZ,
  author_id BIGINT,
  subject TEXT,
  body TEXT,
  partner_ids TEXT,
  attachment_ids TEXT,
  composition_comment_option TEXT,
  is_note BOOLEAN NOT NULL DEFAULT false,
  notification_parameters TEXT,
  send_context TEXT,
  state TEXT NOT NULL DEFAULT 'scheduled'
)`},
	{Version: 134, Name: "mail_compose_template_batch_parity", SQL: `
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS email_from TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS email_cc TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS reply_to TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS model_id BIGINT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS email_from TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS email_cc TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS reply_to TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS partner_to TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS scheduled_date TIMESTAMPTZ;
CREATE TABLE IF NOT EXISTS mail_compose_message (
  id BIGSERIAL PRIMARY KEY,
  composition_mode TEXT,
  composition_comment_option TEXT,
  message_type TEXT,
  subtype_is_log BOOLEAN NOT NULL DEFAULT false,
  model TEXT,
  res_id BIGINT,
  res_ids TEXT,
  template_id BIGINT,
  subject TEXT,
  body TEXT,
  body_html TEXT,
  email_from TEXT,
  email_to TEXT,
  email_cc TEXT,
  reply_to TEXT,
  partner_ids TEXT,
  attachment_ids TEXT,
  parent_id BIGINT,
  subtype_id BIGINT,
  scheduled_date TIMESTAMPTZ,
  mail_server_id BIGINT,
  author_id BIGINT,
  auto_delete BOOLEAN NOT NULL DEFAULT false,
  mass_mailing_id BIGINT,
  use_exclusion_list BOOLEAN NOT NULL DEFAULT true,
  notify BOOLEAN NOT NULL DEFAULT true,
  body_is_html BOOLEAN NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS mail_scheduled_message (
  id BIGSERIAL PRIMARY KEY,
  mail_message_id BIGINT,
  mail_mail_id BIGINT,
  mail_template_id BIGINT,
  model TEXT,
  res_id BIGINT,
  scheduled_date TIMESTAMPTZ,
  author_id BIGINT,
  subject TEXT,
  body TEXT,
  partner_ids TEXT,
  attachment_ids TEXT,
  composition_comment_option TEXT,
  is_note BOOLEAN NOT NULL DEFAULT false,
  notification_parameters TEXT,
  send_context TEXT,
  state TEXT NOT NULL DEFAULT 'scheduled'
);
`},
	{Version: 135, Name: "base_action_metadata_parity", SQL: `
ALTER TABLE ir_filters ADD COLUMN IF NOT EXISTS embedded_action_id BIGINT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS xml_id TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS help TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS path TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_model_id BIGINT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_type TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_view_types TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS effect JSONB;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS infos JSONB;
ALTER TABLE ir_actions ALTER COLUMN binding_type SET DEFAULT 'action';
ALTER TABLE ir_actions ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS views BYTEA;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS embedded_action_ids TEXT;
ALTER TABLE ir_act_window ADD COLUMN IF NOT EXISTS close_on_report_download BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_window ALTER COLUMN type SET DEFAULT 'ir.actions.act_window';
ALTER TABLE ir_act_window ALTER COLUMN context SET DEFAULT '{}';
ALTER TABLE ir_act_window ALTER COLUMN target SET DEFAULT 'current';
ALTER TABLE ir_act_window ALTER COLUMN view_mode SET DEFAULT 'list,form';
ALTER TABLE ir_act_window ALTER COLUMN mobile_view_mode SET DEFAULT 'kanban';
ALTER TABLE ir_act_window ALTER COLUMN limit SET DEFAULT 80;
ALTER TABLE ir_act_window ALTER COLUMN cache SET DEFAULT true;
ALTER TABLE ir_act_window ALTER COLUMN binding_type SET DEFAULT 'action';
ALTER TABLE ir_act_window ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
ALTER TABLE ir_act_window_view ADD COLUMN IF NOT EXISTS multi BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_url ADD COLUMN IF NOT EXISTS type TEXT DEFAULT 'ir.actions.act_url';
ALTER TABLE ir_act_url ADD COLUMN IF NOT EXISTS close BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_url ALTER COLUMN type SET DEFAULT 'ir.actions.act_url';
ALTER TABLE ir_act_url ALTER COLUMN target SET DEFAULT 'new';
ALTER TABLE ir_act_url ALTER COLUMN binding_type SET DEFAULT 'action';
ALTER TABLE ir_act_url ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
ALTER TABLE ir_actions_todo ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS type TEXT DEFAULT 'ir.actions.server';
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS binding_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS binding_view_types TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS usage TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS evaluation_type TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_m2m_operation TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS update_boolean_value TEXT;
ALTER TABLE ir_act_server ALTER COLUMN type SET DEFAULT 'ir.actions.server';
ALTER TABLE ir_act_server ALTER COLUMN binding_type SET DEFAULT 'action';
ALTER TABLE ir_act_server ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
ALTER TABLE ir_act_server ALTER COLUMN active SET DEFAULT true;
ALTER TABLE ir_act_server ALTER COLUMN usage SET DEFAULT 'ir_actions_server';
ALTER TABLE ir_act_server ALTER COLUMN sequence SET DEFAULT 5;
ALTER TABLE ir_act_server ALTER COLUMN evaluation_type SET DEFAULT 'value';
ALTER TABLE ir_act_server ALTER COLUMN update_m2m_operation SET DEFAULT 'add';
ALTER TABLE ir_act_server ALTER COLUMN update_boolean_value SET DEFAULT 'true';
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS automated_name TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS allowed_states JSONB;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS available_model_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS ir_cron_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS show_code_history BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS value_field_to_show TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sms_template_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS sms_method TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_create_model TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_journal_id BIGINT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_suitable_journal_ids TEXT;
ALTER TABLE ir_act_server ADD COLUMN IF NOT EXISTS documents_account_move_type TEXT;
ALTER TABLE ir_act_client ADD COLUMN IF NOT EXISTS type TEXT;
ALTER TABLE ir_act_client ADD COLUMN IF NOT EXISTS context TEXT;
ALTER TABLE ir_act_client ADD COLUMN IF NOT EXISTS params JSONB;
ALTER TABLE ir_act_client ADD COLUMN IF NOT EXISTS params_store BYTEA;
ALTER TABLE ir_act_client ALTER COLUMN type SET DEFAULT 'ir.actions.client';
ALTER TABLE ir_act_client ALTER COLUMN target SET DEFAULT 'current';
ALTER TABLE ir_act_client ALTER COLUMN context SET DEFAULT '{}';
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS type TEXT;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS model_id BIGINT;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS target TEXT;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS context TEXT;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS data JSONB;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS close_on_report_download BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS multi BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_act_report_xml ADD COLUMN IF NOT EXISTS binding_view_types TEXT;
ALTER TABLE ir_act_report_xml ALTER COLUMN type SET DEFAULT 'ir.actions.report';
ALTER TABLE ir_act_report_xml ALTER COLUMN report_type SET DEFAULT 'qweb-pdf';
ALTER TABLE ir_act_report_xml ALTER COLUMN binding_type SET DEFAULT 'report';
ALTER TABLE ir_act_report_xml ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
CREATE TABLE IF NOT EXISTS ir_actions_server_history (
  id BIGSERIAL PRIMARY KEY,
  action_id BIGINT,
  code TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS server_action_history_wizard (
  id BIGSERIAL PRIMARY KEY,
  action_id BIGINT,
  code_diff TEXT,
  current_code TEXT,
  revision BIGINT
);
CREATE TABLE IF NOT EXISTS ir_embedded_actions (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  sequence INTEGER,
  parent_action_id BIGINT NOT NULL,
  parent_res_id BIGINT,
  parent_res_model TEXT NOT NULL,
  action_id BIGINT,
  python_method TEXT,
  user_id BIGINT,
  is_deletable BOOLEAN NOT NULL DEFAULT false,
  default_view_mode TEXT,
  is_visible BOOLEAN NOT NULL DEFAULT true,
  domain TEXT,
  context TEXT,
  groups_ids TEXT
);
CREATE TABLE IF NOT EXISTS report_paperformat (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  "default" BOOLEAN NOT NULL DEFAULT false,
  format TEXT,
  page_height DOUBLE PRECISION,
  page_width DOUBLE PRECISION,
  orientation TEXT,
  margin_top DOUBLE PRECISION,
  margin_bottom DOUBLE PRECISION,
  margin_left DOUBLE PRECISION,
  margin_right DOUBLE PRECISION,
  header_line BOOLEAN NOT NULL DEFAULT false,
  header_spacing DOUBLE PRECISION,
  dpi INTEGER,
  css_margins BOOLEAN NOT NULL DEFAULT false,
  disable_shrinking BOOLEAN NOT NULL DEFAULT false
);
`},
	{Version: 136, Name: "account_structural_metadata_parity", SQL: `
CREATE TABLE IF NOT EXISTS account_group (
  id BIGSERIAL PRIMARY KEY,
  parent_id BIGINT,
  parent_path TEXT,
  name TEXT,
  code_prefix_start TEXT,
  code_prefix_end TEXT,
  company_id BIGINT
);
CREATE TABLE IF NOT EXISTS account_root (
  id TEXT PRIMARY KEY,
  name TEXT,
  parent_id TEXT
);
CREATE TABLE IF NOT EXISTS account_journal_group (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  company_id BIGINT,
  excluded_journal_ids TEXT,
  sequence INTEGER
);
CREATE TABLE IF NOT EXISTS account_incoterms (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  code TEXT,
  active BOOLEAN NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS account_lock_exception (
  id BIGSERIAL PRIMARY KEY,
  active BOOLEAN NOT NULL DEFAULT true,
  state TEXT,
  company_id BIGINT,
  user_id BIGINT,
  reason TEXT,
  end_datetime TIMESTAMPTZ,
  lock_date_field TEXT,
  lock_date DATE,
  company_lock_date DATE,
  fiscalyear_lock_date DATE,
  tax_lock_date DATE,
  sale_lock_date DATE,
  purchase_lock_date DATE
);
ALTER TABLE account_fiscal_position ADD COLUMN IF NOT EXISTS account_ids TEXT;
CREATE TABLE IF NOT EXISTS account_fiscal_position_account (
  id BIGSERIAL PRIMARY KEY,
  position_id BIGINT,
  company_id BIGINT,
  account_src_id BIGINT,
  account_dest_id BIGINT
);
CREATE TABLE IF NOT EXISTS account_invoice_report (
  id BIGSERIAL PRIMARY KEY,
  move_id BIGINT,
  journal_id BIGINT,
  company_id BIGINT,
  company_currency_id BIGINT,
  partner_id BIGINT,
  commercial_partner_id BIGINT,
  country_id BIGINT,
  invoice_user_id BIGINT,
  move_type TEXT,
  state TEXT,
  payment_state TEXT,
  fiscal_position_id BIGINT,
  invoice_date DATE,
  quantity DOUBLE PRECISION,
  product_id BIGINT,
  product_uom_id BIGINT,
  product_categ_id BIGINT,
  invoice_date_due DATE,
  account_id BIGINT,
  price_subtotal_currency NUMERIC,
  price_subtotal NUMERIC,
  price_total NUMERIC,
  price_total_currency NUMERIC,
  price_average NUMERIC,
  price_margin NUMERIC,
  inventory_value NUMERIC,
  currency_id BIGINT
);
CREATE TABLE IF NOT EXISTS account_automatic_entry_wizard (
  id BIGSERIAL PRIMARY KEY,
  action TEXT,
  move_data TEXT,
  preview_move_data TEXT,
  move_line_ids TEXT,
  date DATE,
  company_id BIGINT,
  company_currency_id BIGINT,
  percentage DOUBLE PRECISION,
  total_amount NUMERIC,
  journal_id BIGINT,
  account_type TEXT,
  expense_accrual_account BIGINT,
  revenue_accrual_account BIGINT,
  lock_date_message TEXT,
  destination_account_id BIGINT,
  display_currency_helper BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS account_autopost_bills_wizard (
  id BIGSERIAL PRIMARY KEY,
  partner_id BIGINT,
  partner_name TEXT,
  nb_unmodified_bills INTEGER
);
CREATE TABLE IF NOT EXISTS account_resequence_wizard (
  id BIGSERIAL PRIMARY KEY,
  sequence_number_reset TEXT,
  first_date DATE,
  end_date DATE,
  first_name TEXT,
  ordering TEXT,
  move_ids TEXT,
  new_values TEXT,
  preview_moves TEXT
);
CREATE TABLE IF NOT EXISTS account_secure_entries_wizard (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT,
  country_code TEXT,
  hash_date DATE,
  chains_to_hash_with_gaps TEXT,
  max_hash_date DATE,
  unreconciled_bank_statement_line_ids TEXT,
  not_hashable_unlocked_move_ids TEXT,
  move_to_hash_ids TEXT,
  warnings TEXT
);
CREATE TABLE IF NOT EXISTS account_merge_wizard (
  id BIGSERIAL PRIMARY KEY,
  account_ids TEXT,
  is_group_by_name BOOLEAN NOT NULL DEFAULT false,
  wizard_line_ids TEXT,
  disable_merge_button BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS account_merge_wizard_line (
  id BIGSERIAL PRIMARY KEY,
  wizard_id BIGINT,
  grouping_key TEXT,
  sequence INTEGER,
  display_type TEXT,
  is_selected BOOLEAN NOT NULL DEFAULT false,
  account_id BIGINT,
  company_ids TEXT,
  info TEXT,
  account_has_hashed_entries BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS account_accrued_orders_wizard (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT,
  journal_id BIGINT,
  date DATE,
  reversal_date DATE,
  amount NUMERIC,
  currency_id BIGINT,
  account_id BIGINT,
  preview_data TEXT,
  display_amount BOOLEAN NOT NULL DEFAULT false
);
`},
	{Version: 137, Name: "account_account_group_root_fields", SQL: `
ALTER TABLE account_account ADD COLUMN IF NOT EXISTS placeholder_code TEXT;
ALTER TABLE account_account ADD COLUMN IF NOT EXISTS root_id TEXT;
ALTER TABLE account_account ADD COLUMN IF NOT EXISTS group_id BIGINT;
`},
	{Version: 138, Name: "account_move_fiscal_tax_fields", SQL: `
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS fiscal_position_id BIGINT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS tax_ids TEXT;
`},
	{Version: 139, Name: "account_account_tax_ids", SQL: `
ALTER TABLE account_account ADD COLUMN IF NOT EXISTS tax_ids TEXT;
`},
	{Version: 140, Name: "account_fiscal_position_tax_mapping_fields", SQL: `
ALTER TABLE account_fiscal_position ADD COLUMN IF NOT EXISTS tax_ids TEXT;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS fiscal_position_ids TEXT;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS original_tax_ids TEXT;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS is_domestic BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 141, Name: "res_company_account_lock_fields", SQL: `
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS fiscalyear_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS tax_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS sale_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS purchase_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS hard_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS user_fiscalyear_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS user_tax_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS user_sale_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS user_purchase_lock_date DATE;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS user_hard_lock_date DATE;
`},
	{Version: 142, Name: "mail_tracking_value", SQL: `
CREATE TABLE IF NOT EXISTS mail_tracking_value (
  id BIGSERIAL PRIMARY KEY,
  field_id BIGINT,
  field_info TEXT,
  field_name TEXT,
  field_desc TEXT,
  field_type TEXT,
  old_value_integer BIGINT,
  old_value_float DOUBLE PRECISION,
  old_value_char TEXT,
  old_value_text TEXT,
  old_value_datetime TIMESTAMPTZ,
  new_value_integer BIGINT,
  new_value_float DOUBLE PRECISION,
  new_value_char TEXT,
  new_value_text TEXT,
  new_value_datetime TIMESTAMPTZ,
  currency_id BIGINT,
  mail_message_id BIGINT NOT NULL
);
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS tracking_value_ids TEXT;
ALTER TABLE account_move_line ADD COLUMN IF NOT EXISTS tax_tag_ids TEXT;
`},
	{Version: 143, Name: "res_users_partner_id", SQL: `
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS partner_id BIGINT;
`},
	{Version: 144, Name: "mail_guest_author_fields", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS author_guest_id BIGINT;
ALTER TABLE mail_guest ADD COLUMN IF NOT EXISTS access_token TEXT;
ALTER TABLE mail_guest ADD COLUMN IF NOT EXISTS country_id BIGINT;
ALTER TABLE mail_guest ADD COLUMN IF NOT EXISTS lang TEXT;
ALTER TABLE mail_guest ADD COLUMN IF NOT EXISTS timezone TEXT;
ALTER TABLE discuss_channel_member ADD COLUMN IF NOT EXISTS guest_id BIGINT;
ALTER TABLE mail_presence ADD COLUMN IF NOT EXISTS guest_id BIGINT;
ALTER TABLE mail_presence ALTER COLUMN user_id DROP NOT NULL;
`},
	{Version: 145, Name: "discuss_channel_group_public", SQL: `
ALTER TABLE discuss_channel ADD COLUMN IF NOT EXISTS group_public_id BIGINT;
`},
	{Version: 146, Name: "mail_message_is_internal", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS is_internal BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 147, Name: "ir_attachment_portal_fields", SQL: `
ALTER TABLE ir_attachment ADD COLUMN IF NOT EXISTS checksum TEXT;
ALTER TABLE ir_attachment ADD COLUMN IF NOT EXISTS has_thumbnail BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 148, Name: "mail_message_subtype_description", SQL: `
ALTER TABLE mail_message_subtype ADD COLUMN IF NOT EXISTS description TEXT;
`},
	{Version: 149, Name: "mail_message_reaction", SQL: `
CREATE TABLE IF NOT EXISTS mail_message_reaction (
  id BIGSERIAL PRIMARY KEY,
  message_id BIGINT NOT NULL,
  content TEXT NOT NULL,
  partner_id BIGINT,
  guest_id BIGINT
);
CREATE UNIQUE INDEX IF NOT EXISTS mail_message_reaction_partner_unique
  ON mail_message_reaction (message_id, content, partner_id)
  WHERE partner_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS mail_message_reaction_guest_unique
  ON mail_message_reaction (message_id, content, guest_id)
  WHERE guest_id IS NOT NULL;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'mail_message_reaction_partner_or_guest_exists'
  ) THEN
    ALTER TABLE mail_message_reaction
      ADD CONSTRAINT mail_message_reaction_partner_or_guest_exists
      CHECK ((partner_id IS NOT NULL AND guest_id IS NULL) OR (partner_id IS NULL AND guest_id IS NOT NULL));
  END IF;
END
$$;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS reaction_ids TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS starred BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS starred_partner_ids TEXT;
`},
	{Version: 150, Name: "res_partner_signup_type", SQL: `ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS signup_type TEXT`},
	{Version: 151, Name: "mail_activity_chaining_archive", SQL: `
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS previous_activity_type_id BIGINT;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS date_done TIMESTAMPTZ;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS feedback TEXT;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS delay_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS delay_unit TEXT NOT NULL DEFAULT 'days';
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS delay_from TEXT NOT NULL DEFAULT 'previous_activity';
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS chaining_type TEXT NOT NULL DEFAULT 'suggest';
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS triggered_next_type_id BIGINT;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS suggested_next_type_ids TEXT;
`},
	{Version: 152, Name: "mail_message_activity_type", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS mail_activity_type_id BIGINT;
`},
	{Version: 153, Name: "mail_activity_attachment_ids", SQL: `
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
`},
	{Version: 154, Name: "activity_attachment_rel", SQL: `
CREATE TABLE IF NOT EXISTS activity_attachment_rel (
  activity_id BIGINT NOT NULL,
  attachment_id BIGINT NOT NULL,
  PRIMARY KEY (activity_id, attachment_id)
);
`},
	{Version: 155, Name: "mail_activity_recommendation_fields", SQL: `
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS activity_category TEXT;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS recommended_activity_type_id BIGINT;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS has_recommended_activities BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS chaining_type TEXT;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS previous_type_ids TEXT;
ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS icon TEXT;
CREATE TABLE IF NOT EXISTS mail_activity_rel (
  activity_id BIGINT NOT NULL,
  recommended_id BIGINT NOT NULL,
  PRIMARY KEY (activity_id, recommended_id)
);
`},
	{Version: 156, Name: "res_groups_delegation_metadata", SQL: `
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS category_id BIGINT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS allow_delegation BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS delegation_template_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS name_delegation TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS allow_multiple_delegation BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS restricted_access BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 157, Name: "mail_compose_schedule_fields", SQL: `
ALTER TABLE mail_compose_message ADD COLUMN IF NOT EXISTS composition_comment_option TEXT;
ALTER TABLE mail_compose_message ADD COLUMN IF NOT EXISTS message_type TEXT;
ALTER TABLE mail_compose_message ADD COLUMN IF NOT EXISTS subtype_is_log BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_compose_message ADD COLUMN IF NOT EXISTS mass_mailing_id BIGINT;
ALTER TABLE mail_compose_message ADD COLUMN IF NOT EXISTS use_exclusion_list BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE mail_scheduled_message ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
ALTER TABLE mail_scheduled_message ADD COLUMN IF NOT EXISTS composition_comment_option TEXT;
ALTER TABLE mail_scheduled_message ADD COLUMN IF NOT EXISTS is_note BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_scheduled_message ADD COLUMN IF NOT EXISTS notification_parameters TEXT;
ALTER TABLE mail_scheduled_message ADD COLUMN IF NOT EXISTS send_context TEXT;
`},
	{Version: 158, Name: "hr_base_tables", SQL: `
CREATE TABLE IF NOT EXISTS hr_department (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  complete_name TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  company_id BIGINT,
  parent_id BIGINT,
  child_ids TEXT,
  manager_id BIGINT,
  member_ids TEXT,
  has_read_access BOOLEAN NOT NULL DEFAULT false,
  total_employee INTEGER,
  jobs_ids TEXT,
  plan_ids TEXT,
  plans_count INTEGER,
  note TEXT,
  color INTEGER,
  parent_path TEXT,
  master_department_id BIGINT,
  c_level_manager_id BIGINT
);
CREATE TABLE IF NOT EXISTS hr_employee (
  id BIGSERIAL PRIMARY KEY,
  version_id BIGINT,
  current_version_id BIGINT,
  current_date_version DATE,
  version_ids TEXT,
  versions_count INTEGER,
  name TEXT NOT NULL,
  resource_id BIGINT,
  resource_calendar_id BIGINT,
  user_id BIGINT,
  user_partner_id BIGINT,
  share BOOLEAN NOT NULL DEFAULT false,
  phone TEXT,
  email TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  company_id BIGINT,
  work_phone TEXT,
  mobile_phone TEXT,
  work_email TEXT,
  work_contact_id BIGINT,
  private_email TEXT,
  department_id BIGINT,
  parent_id BIGINT,
  child_ids TEXT,
  coach_id BIGINT,
  job_id BIGINT,
  job_title TEXT,
  category_ids TEXT,
  tz TEXT,
  country_id BIGINT,
  identification_id TEXT,
  barcode TEXT,
  pin TEXT
);
CREATE TABLE IF NOT EXISTS hr_job (
  id BIGSERIAL PRIMARY KEY,
  active BOOLEAN NOT NULL DEFAULT true,
  name TEXT NOT NULL,
  sequence INTEGER,
  expected_employees INTEGER,
  no_of_employee INTEGER,
  no_of_recruitment INTEGER,
  employee_ids TEXT,
  description TEXT,
  requirements TEXT,
  user_id BIGINT,
  allowed_user_ids TEXT,
  department_id BIGINT,
  company_id BIGINT,
  contract_type_id BIGINT
);
CREATE TABLE IF NOT EXISTS hr_employee_category (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  color INTEGER,
  employee_ids TEXT
);
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS employee_ids TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS employee_id BIGINT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS job_title TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_phone TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS mobile_phone TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_email TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS category_ids TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_contact_id BIGINT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_location_id BIGINT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_location_name TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS work_location_type TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS employee_count INTEGER;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS employee_resource_calendar_id BIGINT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS create_employee BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS create_employee_id BIGINT;
`},
	{Version: 159, Name: "approval_forward_workflow_node", SQL: `ALTER TABLE approval_forward ADD COLUMN IF NOT EXISTS workflow_node_id BIGINT`},
	{Version: 160, Name: "workflow_node_responsible_python_code", SQL: `ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS responsible_python_code TEXT`},
	{Version: 161, Name: "workflow_node_schedule_activity_field", SQL: `ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS schedule_activity_field_id BIGINT`},
	{Version: 162, Name: "mail_activity_hide_in_chatter", SQL: `ALTER TABLE mail_activity ADD COLUMN IF NOT EXISTS hide_in_chatter BOOLEAN NOT NULL DEFAULT false`},
	{Version: 163, Name: "approval_buttons_email_compose_fields", SQL: `ALTER TABLE approval_buttons ADD COLUMN IF NOT EXISTS email_wizard_form_id BIGINT;
ALTER TABLE approval_buttons ADD COLUMN IF NOT EXISTS email_next_action TEXT`},
	{Version: 164, Name: "res_country_group", SQL: `CREATE TABLE IF NOT EXISTS res_country_group (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, code TEXT, country_ids TEXT)`},
	{Version: 165, Name: "account_chart_template_fields", SQL: `
ALTER TABLE account_account ADD COLUMN IF NOT EXISTS non_trade BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS invoice_label TEXT;
ALTER TABLE account_tax ADD COLUMN IF NOT EXISTS children_tax_ids TEXT;
ALTER TABLE account_tax_group ADD COLUMN IF NOT EXISTS country_id BIGINT;
ALTER TABLE account_tax_group ADD COLUMN IF NOT EXISTS tax_payable_account_id BIGINT;
ALTER TABLE account_tax_group ADD COLUMN IF NOT EXISTS tax_receivable_account_id BIGINT;
ALTER TABLE account_tax_repartition_line ADD COLUMN IF NOT EXISTS tag_ids TEXT;
ALTER TABLE account_fiscal_position ADD COLUMN IF NOT EXISTS sequence INTEGER;
`},
	{Version: 166, Name: "res_partner_seed_fields", SQL: `
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS is_company BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS image_1920 TEXT;
CREATE TABLE IF NOT EXISTS res_partner_industry (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, full_name TEXT);
`},
	{Version: 167, Name: "res_users_source_seed_fields", SQL: `
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS group_ids TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS signature TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS image_1920 TEXT;
`},
	{Version: 168, Name: "res_groups_source_seed_fields", SQL: `
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS privilege_id BIGINT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS implied_by_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS user_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS comment TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS api_key_duration DOUBLE PRECISION;
`},
	{Version: 169, Name: "ir_model_data_complete_name_noupdate", SQL: `
ALTER TABLE ir_model_data ADD COLUMN IF NOT EXISTS complete_name TEXT;
ALTER TABLE ir_model_data ADD COLUMN IF NOT EXISTS noupdate BOOLEAN NOT NULL DEFAULT false;
	`},
	{Version: 170, Name: "base_security_source_anchor_models", SQL: `
		ALTER TABLE res_users ADD COLUMN IF NOT EXISTS commercial_partner_id BIGINT;
	ALTER TABLE res_users ADD COLUMN IF NOT EXISTS share BOOLEAN NOT NULL DEFAULT false;
	ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS parent_id BIGINT;
	ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS commercial_partner_id BIGINT;
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS partner_share BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS res_users_log (id BIGSERIAL PRIMARY KEY, create_uid BIGINT, create_date TIMESTAMPTZ);
CREATE TABLE IF NOT EXISTS res_users_identitycheck (id BIGSERIAL PRIMARY KEY, create_uid BIGINT, user_id BIGINT);
CREATE TABLE IF NOT EXISTS change_password_user (id BIGSERIAL PRIMARY KEY, create_uid BIGINT, user_id BIGINT, new_passwd TEXT);
CREATE TABLE IF NOT EXISTS change_password_own (id BIGSERIAL PRIMARY KEY, create_uid BIGINT, user_id BIGINT, new_passwd TEXT);
CREATE TABLE IF NOT EXISTS res_device (id BIGSERIAL PRIMARY KEY, name TEXT, user_id BIGINT);
CREATE TABLE IF NOT EXISTS res_device_log (id BIGSERIAL PRIMARY KEY, device_id BIGINT, user_id BIGINT);
CREATE TABLE IF NOT EXISTS res_currency_rate (id BIGSERIAL PRIMARY KEY, name DATE, currency_id BIGINT, company_id BIGINT, rate DOUBLE PRECISION);
CREATE TABLE IF NOT EXISTS ir_ui_view_custom (id BIGSERIAL PRIMARY KEY, user_id BIGINT, ref_id BIGINT, arch TEXT);
CREATE TABLE IF NOT EXISTS properties_base_definition (id BIGSERIAL PRIMARY KEY, name TEXT);
`},
	{Version: 171, Name: "base_source_acl_anchor_models", SQL: `
CREATE TABLE IF NOT EXISTS decimal_precision (id BIGSERIAL PRIMARY KEY, name TEXT, digits INTEGER);
CREATE TABLE IF NOT EXISTS ir_exports (id BIGSERIAL PRIMARY KEY, name TEXT, resource TEXT, export_fields TEXT);
CREATE TABLE IF NOT EXISTS ir_exports_line (id BIGSERIAL PRIMARY KEY, name TEXT, export_id BIGINT);
CREATE TABLE IF NOT EXISTS ir_model_constraint (id BIGSERIAL PRIMARY KEY, name TEXT, model BIGINT, type TEXT, definition TEXT);
CREATE TABLE IF NOT EXISTS ir_model_relation (id BIGSERIAL PRIMARY KEY, name TEXT, model BIGINT);
CREATE TABLE IF NOT EXISTS ir_model_inherit (id BIGSERIAL PRIMARY KEY, name TEXT, model_id BIGINT);
CREATE TABLE IF NOT EXISTS ir_model_fields_selection (id BIGSERIAL PRIMARY KEY, name TEXT, field_id BIGINT, value TEXT, sequence INTEGER);
CREATE TABLE IF NOT EXISTS ir_module_module_dependency (id BIGSERIAL PRIMARY KEY, name TEXT, module_id BIGINT);
CREATE TABLE IF NOT EXISTS ir_module_module_exclusion (id BIGSERIAL PRIMARY KEY, name TEXT, module_id BIGINT);
CREATE TABLE IF NOT EXISTS reset_view_arch_wizard (id BIGSERIAL PRIMARY KEY, view_id BIGINT);
CREATE TABLE IF NOT EXISTS res_partner_category (id BIGSERIAL PRIMARY KEY, name TEXT, parent_id BIGINT);
CREATE TABLE IF NOT EXISTS res_users_apikeys_description (id BIGSERIAL PRIMARY KEY, name TEXT, user_id BIGINT);
CREATE TABLE IF NOT EXISTS res_users_apikeys_show (id BIGSERIAL PRIMARY KEY, name TEXT, user_id BIGINT);
CREATE TABLE IF NOT EXISTS res_users_deletion (id BIGSERIAL PRIMARY KEY, user_id BIGINT);
CREATE TABLE IF NOT EXISTS report_layout (id BIGSERIAL PRIMARY KEY, name TEXT, view_id BIGINT);
CREATE TABLE IF NOT EXISTS ir_logging (id BIGSERIAL PRIMARY KEY, name TEXT, type TEXT, level TEXT, message TEXT);
CREATE TABLE IF NOT EXISTS ir_mail_server (id BIGSERIAL PRIMARY KEY, name TEXT, active BOOLEAN NOT NULL DEFAULT true);
CREATE TABLE IF NOT EXISTS ir_profile (id BIGSERIAL PRIMARY KEY, name TEXT, session TEXT, duration DOUBLE PRECISION);
CREATE TABLE IF NOT EXISTS base_enable_profiling_wizard (id BIGSERIAL PRIMARY KEY, duration INTEGER);
CREATE TABLE IF NOT EXISTS base_language_export (id BIGSERIAL PRIMARY KEY, name TEXT);
CREATE TABLE IF NOT EXISTS base_language_import (id BIGSERIAL PRIMARY KEY, name TEXT);
CREATE TABLE IF NOT EXISTS base_language_install (id BIGSERIAL PRIMARY KEY, lang TEXT);
CREATE TABLE IF NOT EXISTS base_module_update (id BIGSERIAL PRIMARY KEY, updated INTEGER);
CREATE TABLE IF NOT EXISTS base_module_upgrade (id BIGSERIAL PRIMARY KEY, module_info TEXT);
CREATE TABLE IF NOT EXISTS base_module_uninstall (id BIGSERIAL PRIMARY KEY, module_id BIGINT);
CREATE TABLE IF NOT EXISTS base_partner_merge_automatic_wizard (id BIGSERIAL PRIMARY KEY, partner_ids TEXT);
CREATE TABLE IF NOT EXISTS base_partner_merge_line (id BIGSERIAL PRIMARY KEY, wizard_id BIGINT, partner_id BIGINT);
CREATE TABLE IF NOT EXISTS wizard_ir_model_menu_create (id BIGSERIAL PRIMARY KEY, model_id BIGINT, menu_name TEXT);
CREATE TABLE IF NOT EXISTS ir_demo (id BIGSERIAL PRIMARY KEY, name TEXT);
CREATE TABLE IF NOT EXISTS ir_demo_failure (id BIGSERIAL PRIMARY KEY, name TEXT);
CREATE TABLE IF NOT EXISTS ir_demo_failure_wizard (id BIGSERIAL PRIMARY KEY, failure_id BIGINT);
	CREATE TABLE IF NOT EXISTS res_config (id BIGSERIAL PRIMARY KEY, name TEXT);
	CREATE TABLE IF NOT EXISTS change_password_wizard (id BIGSERIAL PRIMARY KEY, user_ids TEXT);
	`},
	{Version: 172, Name: "base_source_field_group_columns", SQL: `
	ALTER TABLE res_users_identitycheck ADD COLUMN IF NOT EXISTS request TEXT;
	ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_user TEXT;
	ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_pass TEXT;
	ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_ssl_certificate TEXT;
	ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_ssl_private_key TEXT;
	`},
	{Version: 173, Name: "res_users_active_partner", SQL: `ALTER TABLE res_users ADD COLUMN IF NOT EXISTS active_partner BOOLEAN NOT NULL DEFAULT true`},
	{Version: 174, Name: "ir_module_category_source_fields", SQL: `
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS child_ids TEXT;
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS module_ids TEXT;
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS privilege_ids TEXT;
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS visible BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS exclusive BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_module_category ADD COLUMN IF NOT EXISTS xml_id TEXT;
`},
	{Version: 175, Name: "res_groups_hierarchy_source_fields", SQL: `
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS full_name TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS share BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS sequence INTEGER;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS all_implied_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS all_implied_by_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS disjoint_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS all_user_ids TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS all_users_count INTEGER;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS model_access TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS rule_groups TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS menu_access TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS view_access TEXT;
ALTER TABLE res_groups ADD COLUMN IF NOT EXISTS view_group_hierarchy JSONB;
ALTER TABLE res_groups_privilege ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE res_groups_privilege ADD COLUMN IF NOT EXISTS placeholder TEXT;
ALTER TABLE res_groups_privilege ADD COLUMN IF NOT EXISTS group_ids TEXT;
`},
	{Version: 176, Name: "res_users_group_payload_fields", SQL: `
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS all_group_ids TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS accesses_count INTEGER;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS rules_count INTEGER;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS groups_count INTEGER;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS view_group_hierarchy JSONB;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS role TEXT;
`},
	{Version: 177, Name: "ir_actions_global_table_parity", SQL: `
CREATE TABLE IF NOT EXISTS ir_actions (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT,
  xml_id TEXT,
  help TEXT,
  path TEXT,
  binding_model_id BIGINT,
  binding_type TEXT DEFAULT 'action',
  binding_view_types TEXT DEFAULT 'list,form',
  effect JSONB,
  infos JSONB
);
CREATE TABLE IF NOT EXISTS ir_act_window (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT DEFAULT 'ir.actions.act_window',
  res_model TEXT,
  res_id BIGINT,
  view_mode TEXT DEFAULT 'list,form',
  views BYTEA,
  embedded_action_ids TEXT,
  close_on_report_download BOOLEAN NOT NULL DEFAULT false,
  mobile_view_mode TEXT DEFAULT 'kanban',
  view_id BIGINT,
  search_view_id BIGINT,
  domain TEXT,
  context TEXT DEFAULT '{}',
  target TEXT DEFAULT 'current',
  limit INTEGER DEFAULT 80,
  help TEXT,
  path TEXT,
  usage TEXT,
  filter BOOLEAN NOT NULL DEFAULT false,
  cache BOOLEAN NOT NULL DEFAULT true,
  group_ids TEXT,
  binding_model_id BIGINT,
  binding_type TEXT DEFAULT 'action',
  binding_view_types TEXT DEFAULT 'list,form'
);
CREATE TABLE IF NOT EXISTS ir_act_window_view (
  id BIGSERIAL PRIMARY KEY,
  sequence INTEGER,
  view_mode TEXT,
  view_id BIGINT,
  act_window_id BIGINT,
  multi BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS ir_act_url (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT DEFAULT 'ir.actions.act_url',
  url TEXT,
  target TEXT DEFAULT 'new',
  close BOOLEAN NOT NULL DEFAULT false,
  binding_model_id BIGINT,
  binding_type TEXT DEFAULT 'action',
  binding_view_types TEXT DEFAULT 'list,form'
);
CREATE TABLE IF NOT EXISTS ir_act_server (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT DEFAULT 'ir.actions.server',
  model_id BIGINT,
  binding_model_id BIGINT,
  binding_type TEXT DEFAULT 'action',
  binding_view_types TEXT DEFAULT 'list,form',
  model_name TEXT,
  automated_name TEXT,
  allowed_states JSONB,
  available_model_ids TEXT,
  state TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  usage TEXT DEFAULT 'ir_actions_server',
  sequence INTEGER NOT NULL DEFAULT 5,
  base_automation_id BIGINT,
  go_action_name TEXT,
  ir_cron_ids TEXT,
  values TEXT,
  value TEXT,
  html_value TEXT,
  evaluation_type TEXT DEFAULT 'value',
  sequence_id BIGINT,
  resource_ref TEXT,
  selection_value BIGINT,
  parent_id BIGINT,
  child_ids TEXT,
  crud_model_id BIGINT,
  crud_model_name TEXT,
  link_field_id BIGINT,
  group_ids TEXT,
  update_field_id BIGINT,
  update_path TEXT,
  update_related_model_id BIGINT,
  update_field_type TEXT,
  update_m2m_operation TEXT DEFAULT 'add',
  update_boolean_value TEXT DEFAULT 'true',
  warning TEXT,
  show_code_history BOOLEAN NOT NULL DEFAULT false,
  value_field_to_show TEXT,
  mail_template_id BIGINT,
  template_id BIGINT,
  mail_post_autofollow BOOLEAN NOT NULL DEFAULT false,
  mail_post_method TEXT,
  followers_type TEXT,
  followers_partner_field_name TEXT,
  partner_ids TEXT,
  activity_type_id BIGINT,
  activity_summary TEXT,
  activity_note TEXT,
  activity_date_deadline_range INTEGER,
  activity_date_deadline_range_type TEXT,
  activity_user_type TEXT,
  activity_user_id BIGINT,
  activity_user_field_name TEXT,
  sms_template_id BIGINT,
  sms_method TEXT,
  wa_template_id BIGINT,
  documents_account_create_model TEXT,
  documents_account_journal_id BIGINT,
  documents_account_suitable_journal_ids TEXT,
  documents_account_move_type TEXT,
  queue_key TEXT,
  webhook_url TEXT,
  webhook_field_ids TEXT,
  webhook_sample_payload TEXT,
  code TEXT,
  ai_tool_ids TEXT,
  ai_action_prompt TEXT,
  ai_tool_show_warning BOOLEAN NOT NULL DEFAULT false,
  ai_tool_description TEXT,
  ai_tool_schema TEXT,
  use_in_ai BOOLEAN NOT NULL DEFAULT false,
  ai_tool_allow_end_message BOOLEAN NOT NULL DEFAULT false,
  ai_tool_is_candidate BOOLEAN NOT NULL DEFAULT false,
  ai_tool_has_schema BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS ir_act_client (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT DEFAULT 'ir.actions.client',
  tag TEXT,
  res_model TEXT,
  target TEXT DEFAULT 'current',
  context TEXT DEFAULT '{}',
  params JSONB,
  params_store BYTEA
);
CREATE TABLE IF NOT EXISTS ir_act_report_xml (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT DEFAULT 'ir.actions.report',
  model TEXT,
  model_id BIGINT,
  report_name TEXT,
  report_type TEXT DEFAULT 'qweb-pdf',
  target TEXT,
  context TEXT,
  data JSONB,
  close_on_report_download BOOLEAN NOT NULL DEFAULT false,
  report_file TEXT,
  print_report_name TEXT,
  attachment TEXT,
  attachment_use BOOLEAN NOT NULL DEFAULT false,
  multi BOOLEAN NOT NULL DEFAULT false,
  binding_model_id BIGINT,
  binding_type TEXT DEFAULT 'report',
  binding_view_types TEXT DEFAULT 'list,form',
  paperformat_id BIGINT,
  groups_id TEXT,
  domain TEXT,
  is_invoice_report BOOLEAN NOT NULL DEFAULT false
);
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS xml_id TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS help TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS path TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_model_id BIGINT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_type TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS binding_view_types TEXT;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS effect JSONB;
ALTER TABLE ir_actions ADD COLUMN IF NOT EXISTS infos JSONB;
ALTER TABLE ir_actions ALTER COLUMN binding_type SET DEFAULT 'action';
ALTER TABLE ir_actions ALTER COLUMN binding_view_types SET DEFAULT 'list,form';
DO $$
BEGIN
  IF to_regclass('ir_actions_actions') IS NOT NULL THEN
    INSERT INTO ir_actions (id, name, type, xml_id, help, path, binding_model_id, binding_type, binding_view_types)
    SELECT id, name, type, xml_id, help, path, binding_model_id, binding_type, binding_view_types
    FROM ir_actions_actions
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      xml_id = EXCLUDED.xml_id,
      help = EXCLUDED.help,
      path = EXCLUDED.path,
      binding_model_id = EXCLUDED.binding_model_id,
      binding_type = EXCLUDED.binding_type,
      binding_view_types = EXCLUDED.binding_view_types;
  END IF;
  IF to_regclass('ir_actions_act_window_close') IS NOT NULL THEN
    INSERT INTO ir_actions (id, name, type, effect, infos)
    SELECT id, COALESCE(NULLIF(name, ''), 'Action'), type, effect, infos
    FROM ir_actions_act_window_close
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      effect = EXCLUDED.effect,
      infos = EXCLUDED.infos;
  END IF;
  IF to_regclass('ir_actions_act_window') IS NOT NULL THEN
    INSERT INTO ir_act_window (id, name, type, res_model, res_id, view_mode, views, embedded_action_ids, close_on_report_download, mobile_view_mode, view_id, search_view_id, domain, context, target, limit, help, path, usage, filter, cache, group_ids, binding_model_id, binding_type, binding_view_types)
    SELECT id, name, type, res_model, res_id, view_mode, views, embedded_action_ids, close_on_report_download, mobile_view_mode, view_id, search_view_id, domain, context, target, limit, help, path, usage, filter, cache, group_ids, binding_model_id, binding_type, binding_view_types
    FROM ir_actions_act_window
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      res_model = EXCLUDED.res_model,
      res_id = EXCLUDED.res_id,
      view_mode = EXCLUDED.view_mode,
      views = EXCLUDED.views,
      embedded_action_ids = EXCLUDED.embedded_action_ids,
      close_on_report_download = EXCLUDED.close_on_report_download,
      mobile_view_mode = EXCLUDED.mobile_view_mode,
      view_id = EXCLUDED.view_id,
      search_view_id = EXCLUDED.search_view_id,
      domain = EXCLUDED.domain,
      context = EXCLUDED.context,
      target = EXCLUDED.target,
      limit = EXCLUDED.limit,
      help = EXCLUDED.help,
      path = EXCLUDED.path,
      usage = EXCLUDED.usage,
      filter = EXCLUDED.filter,
      cache = EXCLUDED.cache,
      group_ids = EXCLUDED.group_ids,
      binding_model_id = EXCLUDED.binding_model_id,
      binding_type = EXCLUDED.binding_type,
      binding_view_types = EXCLUDED.binding_view_types;
  END IF;
  IF to_regclass('ir_actions_act_window_view') IS NOT NULL THEN
    INSERT INTO ir_act_window_view (id, sequence, view_mode, view_id, act_window_id, multi)
    SELECT id, sequence, view_mode, view_id, act_window_id, multi
    FROM ir_actions_act_window_view
    ON CONFLICT (id) DO UPDATE SET
      sequence = EXCLUDED.sequence,
      view_mode = EXCLUDED.view_mode,
      view_id = EXCLUDED.view_id,
      act_window_id = EXCLUDED.act_window_id,
      multi = EXCLUDED.multi;
  END IF;
  IF to_regclass('ir_actions_act_url') IS NOT NULL THEN
    INSERT INTO ir_act_url (id, name, type, url, target, close, binding_model_id, binding_type, binding_view_types)
    SELECT id, name, type, url, target, close, binding_model_id, binding_type, binding_view_types
    FROM ir_actions_act_url
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      url = EXCLUDED.url,
      target = EXCLUDED.target,
      close = EXCLUDED.close,
      binding_model_id = EXCLUDED.binding_model_id,
      binding_type = EXCLUDED.binding_type,
      binding_view_types = EXCLUDED.binding_view_types;
  END IF;
  IF to_regclass('ir_actions_server') IS NOT NULL THEN
    INSERT INTO ir_act_server (id, name, type, model_id, binding_model_id, binding_type, binding_view_types, model_name, automated_name, allowed_states, available_model_ids, state, active, usage, sequence, base_automation_id, go_action_name, ir_cron_ids, values, value, html_value, evaluation_type, sequence_id, resource_ref, selection_value, parent_id, child_ids, crud_model_id, crud_model_name, link_field_id, group_ids, update_field_id, update_path, update_related_model_id, update_field_type, update_m2m_operation, update_boolean_value, warning, show_code_history, value_field_to_show, mail_template_id, template_id, mail_post_autofollow, mail_post_method, followers_type, followers_partner_field_name, partner_ids, activity_type_id, activity_summary, activity_note, activity_date_deadline_range, activity_date_deadline_range_type, activity_user_type, activity_user_id, activity_user_field_name, sms_template_id, sms_method, wa_template_id, queue_key, webhook_url, webhook_field_ids, webhook_sample_payload, code, ai_tool_ids, ai_action_prompt, ai_tool_show_warning, ai_tool_description, ai_tool_schema, use_in_ai, ai_tool_allow_end_message, ai_tool_is_candidate, ai_tool_has_schema)
    SELECT id, name, type, model_id, binding_model_id, binding_type, binding_view_types, model_name, automated_name, allowed_states, available_model_ids, state, active, usage, sequence, base_automation_id, go_action_name, ir_cron_ids, values, value, html_value, evaluation_type, sequence_id, resource_ref, selection_value, parent_id, child_ids, crud_model_id, crud_model_name, link_field_id, group_ids, update_field_id, update_path, update_related_model_id, update_field_type, update_m2m_operation, update_boolean_value, warning, show_code_history, value_field_to_show, mail_template_id, template_id, mail_post_autofollow, mail_post_method, followers_type, followers_partner_field_name, partner_ids, activity_type_id, activity_summary, activity_note, activity_date_deadline_range, activity_date_deadline_range_type, activity_user_type, activity_user_id, activity_user_field_name, sms_template_id, sms_method, wa_template_id, queue_key, webhook_url, webhook_field_ids, webhook_sample_payload, code, ai_tool_ids, ai_action_prompt, ai_tool_show_warning, ai_tool_description, ai_tool_schema, use_in_ai, ai_tool_allow_end_message, ai_tool_is_candidate, ai_tool_has_schema
    FROM ir_actions_server
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      model_id = EXCLUDED.model_id,
      binding_model_id = EXCLUDED.binding_model_id,
      binding_type = EXCLUDED.binding_type,
      binding_view_types = EXCLUDED.binding_view_types,
      model_name = EXCLUDED.model_name,
      automated_name = EXCLUDED.automated_name,
      allowed_states = EXCLUDED.allowed_states,
      available_model_ids = EXCLUDED.available_model_ids,
      state = EXCLUDED.state,
      active = EXCLUDED.active,
      usage = EXCLUDED.usage,
      sequence = EXCLUDED.sequence,
      base_automation_id = EXCLUDED.base_automation_id,
      go_action_name = EXCLUDED.go_action_name,
      ir_cron_ids = EXCLUDED.ir_cron_ids,
      values = EXCLUDED.values,
      value = EXCLUDED.value,
      html_value = EXCLUDED.html_value,
      evaluation_type = EXCLUDED.evaluation_type,
      sequence_id = EXCLUDED.sequence_id,
      resource_ref = EXCLUDED.resource_ref,
      selection_value = EXCLUDED.selection_value,
      parent_id = EXCLUDED.parent_id,
      child_ids = EXCLUDED.child_ids,
      crud_model_id = EXCLUDED.crud_model_id,
      crud_model_name = EXCLUDED.crud_model_name,
      link_field_id = EXCLUDED.link_field_id,
      group_ids = EXCLUDED.group_ids,
      update_field_id = EXCLUDED.update_field_id,
      update_path = EXCLUDED.update_path,
      update_related_model_id = EXCLUDED.update_related_model_id,
      update_field_type = EXCLUDED.update_field_type,
      update_m2m_operation = EXCLUDED.update_m2m_operation,
      update_boolean_value = EXCLUDED.update_boolean_value,
      warning = EXCLUDED.warning,
      show_code_history = EXCLUDED.show_code_history,
      value_field_to_show = EXCLUDED.value_field_to_show,
      mail_template_id = EXCLUDED.mail_template_id,
      template_id = EXCLUDED.template_id,
      mail_post_autofollow = EXCLUDED.mail_post_autofollow,
      mail_post_method = EXCLUDED.mail_post_method,
      followers_type = EXCLUDED.followers_type,
      followers_partner_field_name = EXCLUDED.followers_partner_field_name,
      partner_ids = EXCLUDED.partner_ids,
      activity_type_id = EXCLUDED.activity_type_id,
      activity_summary = EXCLUDED.activity_summary,
      activity_note = EXCLUDED.activity_note,
      activity_date_deadline_range = EXCLUDED.activity_date_deadline_range,
      activity_date_deadline_range_type = EXCLUDED.activity_date_deadline_range_type,
      activity_user_type = EXCLUDED.activity_user_type,
      activity_user_id = EXCLUDED.activity_user_id,
      activity_user_field_name = EXCLUDED.activity_user_field_name,
      sms_template_id = EXCLUDED.sms_template_id,
      sms_method = EXCLUDED.sms_method,
      wa_template_id = EXCLUDED.wa_template_id,
      queue_key = EXCLUDED.queue_key,
      webhook_url = EXCLUDED.webhook_url,
      webhook_field_ids = EXCLUDED.webhook_field_ids,
      webhook_sample_payload = EXCLUDED.webhook_sample_payload,
      code = EXCLUDED.code,
      ai_tool_ids = EXCLUDED.ai_tool_ids,
      ai_action_prompt = EXCLUDED.ai_action_prompt,
      ai_tool_show_warning = EXCLUDED.ai_tool_show_warning,
      ai_tool_description = EXCLUDED.ai_tool_description,
      ai_tool_schema = EXCLUDED.ai_tool_schema,
      use_in_ai = EXCLUDED.use_in_ai,
      ai_tool_allow_end_message = EXCLUDED.ai_tool_allow_end_message,
      ai_tool_is_candidate = EXCLUDED.ai_tool_is_candidate,
      ai_tool_has_schema = EXCLUDED.ai_tool_has_schema;
  END IF;
  IF to_regclass('ir_actions_client') IS NOT NULL THEN
    INSERT INTO ir_act_client (id, name, type, tag, res_model, target, context, params, params_store)
    SELECT id, name, type, tag, res_model, target, context, params, params_store
    FROM ir_actions_client
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      tag = EXCLUDED.tag,
      res_model = EXCLUDED.res_model,
      target = EXCLUDED.target,
      context = EXCLUDED.context,
      params = EXCLUDED.params,
      params_store = EXCLUDED.params_store;
  END IF;
  IF to_regclass('ir_actions_report') IS NOT NULL THEN
    INSERT INTO ir_act_report_xml (id, name, type, model, model_id, report_name, report_type, target, context, data, close_on_report_download, report_file, print_report_name, attachment, attachment_use, multi, binding_model_id, binding_type, binding_view_types, paperformat_id, groups_id, domain, is_invoice_report)
    SELECT id, name, type, model, model_id, report_name, report_type, target, context, data, close_on_report_download, report_file, print_report_name, attachment, attachment_use, multi, binding_model_id, binding_type, binding_view_types, paperformat_id, groups_id, domain, is_invoice_report
    FROM ir_actions_report
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      type = EXCLUDED.type,
      model = EXCLUDED.model,
      model_id = EXCLUDED.model_id,
      report_name = EXCLUDED.report_name,
      report_type = EXCLUDED.report_type,
      target = EXCLUDED.target,
      context = EXCLUDED.context,
      data = EXCLUDED.data,
      close_on_report_download = EXCLUDED.close_on_report_download,
      report_file = EXCLUDED.report_file,
      print_report_name = EXCLUDED.print_report_name,
      attachment = EXCLUDED.attachment,
      attachment_use = EXCLUDED.attachment_use,
      multi = EXCLUDED.multi,
      binding_model_id = EXCLUDED.binding_model_id,
      binding_type = EXCLUDED.binding_type,
      binding_view_types = EXCLUDED.binding_view_types,
      paperformat_id = EXCLUDED.paperformat_id,
      groups_id = EXCLUDED.groups_id,
      domain = EXCLUDED.domain,
      is_invoice_report = EXCLUDED.is_invoice_report;
  END IF;
END $$;
`},
	{Version: 178, Name: "ir_model_data_constraints", SQL: `
UPDATE ir_model_data
SET complete_name = CASE WHEN module = '' THEN name ELSE module || '.' || name END
WHERE complete_name IS NULL OR complete_name = '' OR complete_name != CASE WHEN module = '' THEN name ELSE module || '.' || name END;
DELETE FROM ir_model_data a
USING ir_model_data b
WHERE a.id > b.id AND a.module = b.module AND a.name = b.name;
CREATE UNIQUE INDEX IF NOT EXISTS ir_model_data_module_name_uniq ON ir_model_data(module, name);
CREATE INDEX IF NOT EXISTS ir_model_data_model_res_id_idx ON ir_model_data(model, res_id);
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'ir_model_data_name_nospaces'
  ) THEN
    ALTER TABLE ir_model_data ADD CONSTRAINT ir_model_data_name_nospaces CHECK (name NOT LIKE '% %') NOT VALID;
  END IF;
END $$;
`},
	{Version: 179, Name: "mail_activity_type_templates", SQL: `ALTER TABLE mail_activity_type ADD COLUMN IF NOT EXISTS mail_template_ids TEXT`},
	{Version: 180, Name: "ir_model_mail_metadata_flags", SQL: `
ALTER TABLE ir_model ADD COLUMN IF NOT EXISTS abstract BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_model ADD COLUMN IF NOT EXISTS transient BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_model ADD COLUMN IF NOT EXISTS is_mail_thread BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_model ADD COLUMN IF NOT EXISTS is_mail_activity BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_model ADD COLUMN IF NOT EXISTS is_mail_blacklist BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 181, Name: "mail_persistent_queue_fields", SQL: `
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS recipient_ids TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS mail_server_id BIGINT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS failure_type TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS auto_delete BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS message_id TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS references TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS headers TEXT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS is_notification BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS fetchmail_server_id BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS mail_mail_id BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS mail_email_address TEXT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS failure_reason TEXT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS is_read BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS read_date TIMESTAMPTZ;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS author_id BIGINT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_host TEXT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_port INTEGER;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_encryption TEXT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_authentication TEXT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS from_filter TEXT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS sequence INTEGER;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS smtp_debug BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS max_email_size INTEGER;
`},
	{Version: 182, Name: "mail_template_queue_fields", SQL: `
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS mail_server_id BIGINT;
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS auto_delete BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 183, Name: "mail_server_owner_alias_domain", SQL: `
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS owner_user_id BIGINT;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS owner_limit_time TIMESTAMPTZ;
ALTER TABLE ir_mail_server ADD COLUMN IF NOT EXISTS owner_limit_count INTEGER;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS notification_type TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS outgoing_mail_server_id BIGINT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS outgoing_mail_server_type TEXT;
ALTER TABLE res_users ADD COLUMN IF NOT EXISTS has_external_mail_server BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS alias_domain_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_domain_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_domain TEXT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_full_name TEXT;
CREATE TABLE IF NOT EXISTS mail_alias_domain (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  company_ids TEXT,
  sequence INTEGER,
  bounce_alias TEXT,
  bounce_email TEXT,
  catchall_alias TEXT,
  catchall_email TEXT,
  default_from TEXT,
  default_from_email TEXT
);
`},
	{Version: 184, Name: "fetchmail_server_parity_fields", SQL: `
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS server TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS port INTEGER;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS server_type_info TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS is_ssl BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS attach BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS original BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS date TIMESTAMPTZ;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS error_date TIMESTAMPTZ;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS "user" TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS password TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS object_id BIGINT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 5;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS message_ids TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS configuration TEXT;
ALTER TABLE fetchmail_server ADD COLUMN IF NOT EXISTS script TEXT;
`},
	{Version: 185, Name: "mail_log_access_fields", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS create_uid BIGINT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS create_date TIMESTAMPTZ;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS write_uid BIGINT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS write_date TIMESTAMPTZ;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS create_uid BIGINT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS create_date TIMESTAMPTZ;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS write_uid BIGINT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS write_date TIMESTAMPTZ;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS create_uid BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS create_date TIMESTAMPTZ;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS write_uid BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS write_date TIMESTAMPTZ;
`},
	{Version: 186, Name: "mail_template_dynamic_reports", SQL: `
ALTER TABLE mail_template ADD COLUMN IF NOT EXISTS report_template_ids TEXT;
`},
	{Version: 187, Name: "ir_attachment_res_field", SQL: `
ALTER TABLE ir_attachment ADD COLUMN IF NOT EXISTS res_field TEXT;
`},
	{Version: 188, Name: "ir_attachment_file_size", SQL: `
ALTER TABLE ir_attachment ADD COLUMN IF NOT EXISTS file_size BIGINT;
`},
	{Version: 189, Name: "account_restrictive_audit_attachment_fields", SQL: `
ALTER TABLE ir_attachment ADD COLUMN IF NOT EXISTS company_id BIGINT;
ALTER TABLE res_company ADD COLUMN IF NOT EXISTS restrictive_audit_trail BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_move ADD COLUMN IF NOT EXISTS ubl_cii_xml_file TEXT;
`},
	{Version: 190, Name: "mail_bounce_processing_fields", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS message_id TEXT;
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS email_normalized TEXT;
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS message_bounce BIGINT NOT NULL DEFAULT 0;
`},
	{Version: 191, Name: "mail_message_gateway_fields", SQL: `
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS incoming_email_to TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS incoming_email_cc TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS outgoing_email_to TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS reply_to TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS reply_to_force_new BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS mail_server_id BIGINT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS email_layout_xmlid TEXT;
ALTER TABLE mail_message ADD COLUMN IF NOT EXISTS email_add_signature BOOLEAN NOT NULL DEFAULT true;
`},
	{Version: 192, Name: "mail_gateway_allowed", SQL: `
CREATE TABLE IF NOT EXISTS mail_gateway_allowed (
  id BIGSERIAL PRIMARY KEY,
  email TEXT,
  email_normalized TEXT
);
`},
	{Version: 193, Name: "mail_alias_route_fields", SQL: `
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_model_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_defaults TEXT NOT NULL DEFAULT '{}';
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_force_thread_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_parent_model_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_parent_thread_id BIGINT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_contact TEXT NOT NULL DEFAULT 'everyone';
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_incoming_local BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_bounced_content TEXT;
ALTER TABLE mail_alias ADD COLUMN IF NOT EXISTS alias_status TEXT;
`},
	{Version: 194, Name: "mailing_trace_side_effect_models", SQL: `
CREATE TABLE IF NOT EXISTS mail_blacklist (
  id BIGSERIAL PRIMARY KEY,
  email TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  message TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS phone_blacklist (
  id BIGSERIAL PRIMARY KEY,
  number TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  message TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS phone_blacklist_number_unique ON phone_blacklist(number);
CREATE TABLE IF NOT EXISTS utm_campaign (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  mailing_mail_ids TEXT,
  mailing_mail_count INTEGER NOT NULL DEFAULT 0,
  ab_testing_mailings_count INTEGER NOT NULL DEFAULT 0,
  ab_testing_completed BOOLEAN NOT NULL DEFAULT false,
  ab_testing_winner_mailing_id BIGINT,
  ab_testing_schedule_datetime TIMESTAMPTZ,
  ab_testing_winner_selection TEXT
);
CREATE TABLE IF NOT EXISTS utm_source (
  id BIGSERIAL PRIMARY KEY,
  name TEXT
);
CREATE TABLE IF NOT EXISTS utm_medium (
  id BIGSERIAL PRIMARY KEY,
  name TEXT
);
CREATE TABLE IF NOT EXISTS mailing_contact (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  first_name TEXT,
  last_name TEXT,
  company_name TEXT,
  email TEXT,
  email_normalized TEXT,
  phone TEXT,
  phone_sanitized TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  list_ids TEXT,
  subscription_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS mailing_list (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  is_public BOOLEAN NOT NULL DEFAULT false,
  contact_ids TEXT,
  subscription_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS mailing_subscription (
  id BIGSERIAL PRIMARY KEY,
  contact_id BIGINT,
  list_id BIGINT,
  opt_out BOOLEAN NOT NULL DEFAULT false,
  opt_out_reason_id BIGINT,
  opt_out_datetime TIMESTAMPTZ,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS mailing_subscription_optout (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  sequence INTEGER,
  active BOOLEAN NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS mailing_mailing (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  subject TEXT,
  preview TEXT,
  body_html TEXT,
  email_from TEXT,
  reply_to TEXT,
  reply_to_mode TEXT,
  mail_server_id BIGINT,
  attachment_ids TEXT,
  keep_archives BOOLEAN NOT NULL DEFAULT false,
  state TEXT NOT NULL DEFAULT 'draft',
  sent_date TIMESTAMPTZ,
  schedule_type TEXT NOT NULL DEFAULT 'now',
  schedule_date TIMESTAMPTZ,
  kpi_mail_required BOOLEAN NOT NULL DEFAULT false,
  user_id BIGINT,
  mailing_model_real TEXT,
  mailing_domain TEXT,
  mailing_on_mailing_list BOOLEAN NOT NULL DEFAULT false,
  use_exclusion_list BOOLEAN NOT NULL DEFAULT true,
  sms_allow_unsubscribe BOOLEAN NOT NULL DEFAULT false,
  contact_list_ids TEXT,
  campaign_id BIGINT,
  source_id BIGINT,
  medium_id BIGINT,
  ab_testing_enabled BOOLEAN NOT NULL DEFAULT false,
  ab_testing_pc INTEGER NOT NULL DEFAULT 10,
  ab_testing_is_winner_mailing BOOLEAN NOT NULL DEFAULT false,
  ab_testing_mailings_count INTEGER NOT NULL DEFAULT 0,
	  ab_testing_completed BOOLEAN NOT NULL DEFAULT false,
	  ab_testing_schedule_datetime TIMESTAMPTZ,
	  ab_testing_winner_selection TEXT,
	  is_ab_test_sent BOOLEAN NOT NULL DEFAULT false,
	  total INTEGER NOT NULL DEFAULT 0,
	  scheduled INTEGER NOT NULL DEFAULT 0,
	  expected INTEGER NOT NULL DEFAULT 0,
	  canceled INTEGER NOT NULL DEFAULT 0,
	  sent INTEGER NOT NULL DEFAULT 0,
	  process INTEGER NOT NULL DEFAULT 0,
	  pending INTEGER NOT NULL DEFAULT 0,
	  delivered INTEGER NOT NULL DEFAULT 0,
	  opened INTEGER NOT NULL DEFAULT 0,
	  clicked INTEGER NOT NULL DEFAULT 0,
	  replied INTEGER NOT NULL DEFAULT 0,
	  bounced INTEGER NOT NULL DEFAULT 0,
	  failed INTEGER NOT NULL DEFAULT 0,
	  received_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	  opened_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	  replied_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	  bounced_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	  clicks_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	  link_trackers_count INTEGER NOT NULL DEFAULT 0,
	  mailing_trace_ids TEXT,
	  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS mailing_mailing_test (
  id BIGSERIAL PRIMARY KEY,
  email_to TEXT,
  mass_mailing_id BIGINT
);
CREATE TABLE IF NOT EXISTS mailing_mailing_schedule_date (
  id BIGSERIAL PRIMARY KEY,
  schedule_date TIMESTAMPTZ,
  mass_mailing_id BIGINT
);
CREATE TABLE IF NOT EXISTS link_tracker (
  id BIGSERIAL PRIMARY KEY,
  url TEXT,
  absolute_url TEXT,
  short_url TEXT,
  redirected_url TEXT,
  short_url_host TEXT,
  title TEXT,
  label TEXT,
  link_code_ids TEXT,
  code TEXT,
  link_click_ids TEXT,
  count INTEGER NOT NULL DEFAULT 0,
  campaign_id BIGINT,
  medium_id BIGINT,
  source_id BIGINT,
  mass_mailing_id BIGINT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS link_tracker_code (
  id BIGSERIAL PRIMARY KEY,
  code TEXT,
  link_id BIGINT
);
CREATE UNIQUE INDEX IF NOT EXISTS link_tracker_code_code_unique ON link_tracker_code(code);
CREATE TABLE IF NOT EXISTS link_tracker_click (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT,
  link_id BIGINT,
  ip TEXT,
  country_id BIGINT,
  mailing_trace_id BIGINT,
  mass_mailing_id BIGINT,
  whatsapp_message_id BIGINT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS sms_sms (
  id BIGSERIAL PRIMARY KEY,
  uuid TEXT,
  number TEXT,
  body TEXT,
  partner_id BIGINT,
  mail_message_id BIGINT,
  state TEXT NOT NULL DEFAULT 'outgoing',
  failure_type TEXT,
  to_delete BOOLEAN NOT NULL DEFAULT false,
  mailing_id BIGINT,
  mailing_trace_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS sms_tracker (
  id BIGSERIAL PRIMARY KEY,
  sms_uuid TEXT,
  mail_notification_id BIGINT,
  mailing_trace_id BIGINT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS mailing_trace (
  id BIGSERIAL PRIMARY KEY,
  trace_type TEXT NOT NULL DEFAULT 'mail',
  is_test_trace BOOLEAN NOT NULL DEFAULT false,
  mail_mail_id BIGINT,
  mail_mail_id_int BIGINT,
  email TEXT,
  message_id TEXT,
  medium_id BIGINT,
  source_id BIGINT,
  model TEXT,
  res_id BIGINT,
  mass_mailing_id BIGINT,
  campaign_id BIGINT,
  sent_datetime TIMESTAMPTZ,
  open_datetime TIMESTAMPTZ,
  reply_datetime TIMESTAMPTZ,
  trace_status TEXT NOT NULL DEFAULT 'outgoing',
  failure_type TEXT,
  failure_reason TEXT,
  links_click_datetime TIMESTAMPTZ,
  sms_id BIGINT,
  sms_id_int BIGINT,
  sms_tracker_ids TEXT,
  sms_number TEXT,
  sms_code TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS mailing_id BIGINT;
ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS mailing_trace_ids TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS subject TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS preview TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS body_html TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS email_from TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS reply_to TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS reply_to_mode TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS mail_server_id BIGINT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS attachment_ids TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS keep_archives BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS sent_date TIMESTAMPTZ;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS schedule_type TEXT NOT NULL DEFAULT 'now';
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS schedule_date TIMESTAMPTZ;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS kpi_mail_required BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS user_id BIGINT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS mailing_model_real TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS mailing_domain TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS mailing_on_mailing_list BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS use_exclusion_list BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS contact_list_ids TEXT;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS mailing_mail_ids TEXT;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS mailing_mail_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_mailings_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_completed BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_winner_mailing_id BIGINT;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_schedule_datetime TIMESTAMPTZ;
ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_winner_selection TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_pc INTEGER NOT NULL DEFAULT 10;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_is_winner_mailing BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_mailings_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_completed BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_schedule_datetime TIMESTAMPTZ;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_winner_selection TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS is_ab_test_sent BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 195, Name: "link_tracker_code_unique", SQL: `CREATE UNIQUE INDEX IF NOT EXISTS link_tracker_code_code_unique ON link_tracker_code(code)`},
	{Version: 196, Name: "whatsapp_tracked_link_route_parity", SQL: `
ALTER TABLE link_tracker_click ADD COLUMN IF NOT EXISTS whatsapp_message_id BIGINT;
CREATE TABLE IF NOT EXISTS marketing_campaign (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  utm_campaign_id BIGINT,
  state TEXT NOT NULL DEFAULT 'draft',
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS marketing_activity (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  activity_type TEXT,
  campaign_id BIGINT,
  whatsapp_template_id BIGINT,
  server_action_id BIGINT,
  interval_number INTEGER NOT NULL DEFAULT 0,
  interval_type TEXT NOT NULL DEFAULT 'hours',
  trigger_type TEXT,
  trigger_category TEXT,
  whatsapp_error BOOLEAN NOT NULL DEFAULT false,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
ALTER TABLE marketing_campaign ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS server_action_id BIGINT;
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS interval_number INTEGER NOT NULL DEFAULT 0;
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS interval_type TEXT NOT NULL DEFAULT 'hours';
CREATE TABLE IF NOT EXISTS whatsapp_message (
  id BIGSERIAL PRIMARY KEY,
  state TEXT,
  links_click_datetime TIMESTAMPTZ,
  marketing_trace_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS marketing_trace (
  id BIGSERIAL PRIMARY KEY,
  activity_id BIGINT,
  whatsapp_message_id BIGINT,
  links_click_datetime TIMESTAMPTZ,
  state TEXT,
  schedule_date TIMESTAMPTZ,
  state_msg TEXT,
  parent_id BIGINT,
  child_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
`},
	{Version: 197, Name: "whatsapp_template_tracked_link_generation", SQL: `
CREATE TABLE IF NOT EXISTS whatsapp_template (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  template_name TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  wa_template_uid TEXT,
  model_id BIGINT,
  model TEXT,
  phone_field TEXT,
  lang_code TEXT,
  template_type TEXT,
  status TEXT NOT NULL DEFAULT 'draft',
  quality TEXT NOT NULL DEFAULT 'none',
  header_type TEXT NOT NULL DEFAULT 'none',
  header_text TEXT,
  header_attachment_ids TEXT,
  footer_text TEXT,
  body TEXT,
  variable_ids TEXT,
  button_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS whatsapp_template_button (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  text TEXT,
  template_id BIGINT,
  wa_template_id BIGINT,
  sequence INTEGER,
  button_type TEXT,
  url_type TEXT,
  website_url TEXT,
  dynamic_url TEXT,
  call_number TEXT,
  variable_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS whatsapp_template_variable (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  button_id BIGINT,
  wa_template_id BIGINT,
  model TEXT,
  line_type TEXT,
  field_type TEXT NOT NULL DEFAULT 'free_text',
  field_name TEXT,
  demo_value TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS template_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS msg_uid TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mail_message_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS model TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS res_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS body TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS components TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS free_text_json TEXT;
ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
`},
	{Version: 198, Name: "sms_tracked_link_route_parity", SQL: `
CREATE TABLE IF NOT EXISTS sms_sms (
  id BIGSERIAL PRIMARY KEY,
  uuid TEXT,
  number TEXT,
  body TEXT,
  partner_id BIGINT,
  mail_message_id BIGINT,
  state TEXT NOT NULL DEFAULT 'outgoing',
  failure_type TEXT,
  to_delete BOOLEAN NOT NULL DEFAULT false,
  mailing_id BIGINT,
  mailing_trace_ids TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS sms_tracker (
  id BIGSERIAL PRIMARY KEY,
  sms_uuid TEXT,
  mail_notification_id BIGINT,
  mailing_trace_id BIGINT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS sms_sms_uuid_unique ON sms_sms(uuid);
CREATE UNIQUE INDEX IF NOT EXISTS sms_tracker_sms_uuid_unique ON sms_tracker(sms_uuid);
ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_id BIGINT;
ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_id_int BIGINT;
ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_tracker_ids TEXT;
ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_number TEXT;
ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_code TEXT;
`},
	{Version: 199, Name: "sms_template_tracked_link_generation", SQL: `
CREATE TABLE IF NOT EXISTS sms_template (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  model_id BIGINT,
  model TEXT,
  body TEXT,
  sidebar_action_id BIGINT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
`},
	{Version: 200, Name: "sms_opt_out_parity", SQL: `
CREATE TABLE IF NOT EXISTS phone_blacklist (
  id BIGSERIAL PRIMARY KEY,
  number TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  message TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS phone_blacklist_number_unique ON phone_blacklist(number);
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS phone_sanitized TEXT;
ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS phone_blacklisted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mailing_contact ADD COLUMN IF NOT EXISTS phone TEXT;
ALTER TABLE mailing_contact ADD COLUMN IF NOT EXISTS phone_sanitized TEXT;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS sms_allow_unsubscribe BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 201, Name: "whatsapp_source_template_fields", SQL: `
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS msg_uid TEXT;
ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS wa_template_id BIGINT;
`},
	{Version: 202, Name: "sms_delivery_status_webhook_parity", SQL: `
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_id BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_id_int BIGINT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_tracker_ids TEXT;
ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_number TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS sms_sms_uuid_unique ON sms_sms(uuid);
CREATE UNIQUE INDEX IF NOT EXISTS sms_tracker_sms_uuid_unique ON sms_tracker(sms_uuid);
`},
	{Version: 203, Name: "whatsapp_marketing_activity_source_fields", SQL: `
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS activity_type TEXT;
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS whatsapp_template_id BIGINT;
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS trigger_category TEXT;
ALTER TABLE marketing_activity ADD COLUMN IF NOT EXISTS whatsapp_error BOOLEAN NOT NULL DEFAULT false;
`},
	{Version: 204, Name: "whatsapp_template_variable_validation_fields", SQL: `
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS template_name TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS wa_template_uid TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS model_id BIGINT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS model TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS phone_field TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS lang_code TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS template_type TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS quality TEXT NOT NULL DEFAULT 'none';
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_type TEXT NOT NULL DEFAULT 'none';
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_text TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_attachment_ids TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS footer_text TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS variable_ids TEXT;
ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS call_number TEXT;
ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS variable_ids TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS free_text_json TEXT;
CREATE TABLE IF NOT EXISTS whatsapp_template_variable (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  button_id BIGINT,
  wa_template_id BIGINT,
  model TEXT,
  line_type TEXT,
  field_type TEXT NOT NULL DEFAULT 'free_text',
  field_name TEXT,
  demo_value TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
`},
	{Version: 205, Name: "whatsapp_template_webhook_account_fields", SQL: `
CREATE TABLE IF NOT EXISTS whatsapp_account (
  id BIGSERIAL PRIMARY KEY,
  name TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  app_uid TEXT,
  account_uid TEXT,
  phone_uid TEXT,
  phone_number TEXT,
  token TEXT,
  app_secret TEXT,
  webhook_verify_token TEXT,
  callback_url TEXT,
  debug_logging BOOLEAN NOT NULL DEFAULT false,
  templates_count INTEGER NOT NULL DEFAULT 0,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS wa_account_id BIGINT;
`},
	{Version: 206, Name: "whatsapp_message_status_webhook_fields", SQL: `
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mobile_number TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mobile_number_formatted TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS message_type TEXT NOT NULL DEFAULT 'outbound';
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS failure_type TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS failure_reason TEXT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_account_id BIGINT;
ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS parent_id BIGINT;
`},
	{Version: 207, Name: "whatsapp_account_template_sync_fields", SQL: `
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS app_uid TEXT;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS phone_number TEXT;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS token TEXT;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS callback_url TEXT;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS debug_logging BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS templates_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_attachment_ids TEXT;
ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS footer_text TEXT;
`},
	{Version: 208, Name: "mass_mailing_statistics_fields", SQL: `
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS total INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS scheduled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS expected INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS canceled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS sent INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS process INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS pending INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS delivered INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS opened INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS clicked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS replied INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS bounced INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS failed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS received_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS opened_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS replied_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS bounced_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS clicks_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS link_trackers_count INTEGER NOT NULL DEFAULT 0;
`},
	{Version: 209, Name: "mail_inbound_message_lock", SQL: `
CREATE TABLE IF NOT EXISTS mail_inbound_message_lock (
  id BIGSERIAL PRIMARY KEY,
  message_id TEXT NOT NULL,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS mail_inbound_message_lock_message_id_unique
  ON mail_inbound_message_lock (message_id);
`},
	{Version: 210, Name: "digest_send_cron_fields", SQL: `
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS available_fields TEXT;
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS is_subscribed BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS kpi_res_users_connected BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS kpi_res_users_connected_value INTEGER;
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS kpi_mail_message_total BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE digest_digest ADD COLUMN IF NOT EXISTS kpi_mail_message_total_value INTEGER;
ALTER TABLE digest_tip ADD COLUMN IF NOT EXISTS user_ids TEXT;
ALTER TABLE res_users_log ADD COLUMN IF NOT EXISTS create_date TIMESTAMPTZ;
`},
	{Version: 211, Name: "mail_mail_inherited_author", SQL: `ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS author_id BIGINT`},
	{Version: 212, Name: "sms_composer_wizard_parity", SQL: `
CREATE TABLE IF NOT EXISTS sms_composer (
  id BIGSERIAL PRIMARY KEY,
  composition_mode TEXT,
  res_model TEXT,
  res_model_description TEXT,
  res_id BIGINT,
  res_ids TEXT,
  res_ids_count INTEGER NOT NULL DEFAULT 0,
  comment_single_recipient BOOLEAN NOT NULL DEFAULT false,
  mass_keep_log BOOLEAN NOT NULL DEFAULT true,
  mass_force_send BOOLEAN NOT NULL DEFAULT false,
  use_exclusion_list BOOLEAN NOT NULL DEFAULT true,
  recipient_valid_count INTEGER NOT NULL DEFAULT 0,
  recipient_invalid_count INTEGER NOT NULL DEFAULT 0,
  recipient_single_description TEXT,
  recipient_single_number TEXT,
  recipient_single_number_itf TEXT,
  recipient_single_valid BOOLEAN NOT NULL DEFAULT false,
  number_field_name TEXT,
  numbers TEXT,
  sanitized_numbers TEXT,
  template_id BIGINT,
  body TEXT,
  create_uid BIGINT,
  create_date TIMESTAMPTZ,
  write_uid BIGINT,
  write_date TIMESTAMPTZ
);
`},
	{Version: 213, Name: "digest_res_users_login_date", SQL: `ALTER TABLE res_users ADD COLUMN IF NOT EXISTS login_date TIMESTAMPTZ`},
	{Version: 214, Name: "workflow_node_responsible_value_filter", SQL: `
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS responsible_value TEXT;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS responsible_filter TEXT;
`},
	{Version: 215, Name: "workflow_node_advanced_metadata_fields", SQL: `
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS model_id BIGINT;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS responsible_committee BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS responsible_committee_limit INTEGER;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS schedule_activity_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS button_context TEXT;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS button_icon TEXT;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS button_validate_form BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS wizard_view_id BIGINT;
ALTER TABLE workflow_node ADD COLUMN IF NOT EXISTS trg_date_calendar_id BIGINT;
`},
}
