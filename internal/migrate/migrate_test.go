package migrate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeExec struct {
	queries []string
}

func (f *fakeExec) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	f.queries = append(f.queries, query)
	return nil, nil
}

func TestRunnerOrdersMigrations(t *testing.T) {
	exec := &fakeExec{}
	runner := NewRunner([]Migration{
		{Version: 2, Name: "two", SQL: "two"},
		{Version: 1, Name: "one", SQL: "one"},
	})
	if err := runner.Apply(context.Background(), exec); err != nil {
		t.Fatal(err)
	}
	if len(exec.queries) != 2 || exec.queries[0] != "one" || exec.queries[1] != "two" {
		t.Fatalf("queries = %+v", exec.queries)
	}
}

func TestBaseMigrationsIncludeAutomationAndMail(t *testing.T) {
	names := map[string]bool{}
	for _, migration := range BaseMigrations {
		names[migration.Name] = true
	}
	for _, name := range []string{
		"ir_cron",
		"ir_cron_trigger",
		"ir_cron_progress",
		"ir_sequence",
		"ir_module_category",
		"ir_module_category_source_fields",
		"ir_default",
		"res_users",
		"res_users_active_partner",
		"res_users_source_seed_fields",
		"res_groups",
		"res_groups_delegation_metadata",
		"res_groups_source_seed_fields",
		"res_groups_hierarchy_source_fields",
		"ir_model_data_complete_name_noupdate",
		"ir_model_data_constraints",
		"base_security_source_anchor_models",
		"base_source_acl_anchor_models",
		"base_source_field_group_columns",
		"hr_base_tables",
		"res_company",
		"res_partner",
		"res_partner_signup_type",
		"res_partner_seed_fields",
		"res_currency",
		"res_country",
		"res_country_group",
		"res_country_state",
		"res_lang",
		"res_bank",
		"res_partner_bank",
		"res_groups_privilege",
		"res_users_settings",
		"res_users_apikeys",
		"ir_attachment",
		"ir_filters",
		"ir_ui_view",
		"ir_asset",
		"ir_ui_menu",
		"ir_actions",
		"ir_act_window",
		"ir_act_window_view",
		"ir_act_url",
		"ir_actions_todo",
		"ir_act_server",
		"ir_act_client",
		"ir_act_report_xml",
		"ir_actions_global_table_parity",
		"base_automation",
		"mail_message",
		"mail_inbound_message_lock",
		"mail_message_reaction",
		"mail_mail",
		"mail_notification",
		"mail_followers",
		"mail_activity",
		"mail_activity_chaining_archive",
		"mail_activity_attachment_ids",
		"activity_attachment_rel",
		"mail_activity_recommendation_fields",
		"mail_message_activity_type",
		"mail_compose_message",
		"mail_scheduled_message",
		"mail_template",
		"mail_alias",
		"mail_alias_route_fields",
		"mail_gateway_allowed",
		"mailing_trace_side_effect_models",
		"link_tracker_code_unique",
		"whatsapp_source_template_fields",
		"whatsapp_marketing_activity_source_fields",
		"whatsapp_template_variable_validation_fields",
		"whatsapp_template_webhook_account_fields",
		"whatsapp_message_status_webhook_fields",
		"whatsapp_account_template_sync_fields",
		"sms_template_tracked_link_generation",
		"sms_composer_wizard_parity",
		"digest_res_users_login_date",
		"mail_message_subtype",
		"fetchmail_server",
		"discuss_channel",
		"discuss_channel_member",
		"mail_guest",
		"mail_presence",
		"mail_mail_inherited_author",
		"account_account",
		"account_journal",
		"account_move",
		"account_move_reversal",
		"account_payment_register",
		"account_move_send_wizard",
		"account_move_send_batch_wizard",
		"account_move_line",
		"account_payment",
		"account_tax",
		"account_tax_repartition_line",
		"account_fiscal_position",
		"account_partial_reconcile",
		"account_full_reconcile",
		"account_reconcile_model",
		"account_report",
		"ai_agent",
		"ai_topic",
		"ai_agent_source",
		"ai_prompt_button",
		"ai_embedding",
		"ai_composer",
		"ai_settings",
		"approval_settings",
		"approval_settings_state",
		"approval_buttons",
		"approval_automation",
		"approval_log",
		"approval_log_voting",
		"approval_forward",
		"cancellation_record",
		"state_tags",
		"workflow_workflow",
		"workflow_node",
		"workflow_transition",
		"workflow_node_action",
		"workflow_process",
		"delegation",
		"delegation_line",
		"delegation_mail_template_metadata",
		"login_as_audit",
		"login_as_route",
		"account_dependency_anchor_tables",
		"account_dependency_anchor_fields",
		"cron_progress_loop_fields",
		"server_action_state_metadata_fields",
		"ir_model_fields_groups_relation_field",
		"ir_model_mail_metadata_flags",
		"mail_persistent_queue_fields",
		"mail_template_queue_fields",
		"mail_log_access_fields",
		"mail_compose_template_batch_parity",
		"base_action_metadata_parity",
		"account_structural_metadata_parity",
		"workflow_node_responsible_python_code",
		"workflow_node_schedule_activity_field",
		"workflow_node_responsible_value_filter",
		"workflow_node_advanced_metadata_fields",
		"resource_calendar_models",
		"approval_log_duration_fields",
		"delegation_source_field_metadata",
		"mail_activity_hide_in_chatter",
		"approval_buttons_email_compose_fields",
	} {
		if !names[name] {
			t.Fatalf("missing migration %s", name)
		}
	}
}

func TestDelegationMigrationsExposeSourceFieldAndUniqueLineRole(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, name := range []string{"delegation", "delegation_source_field_metadata"} {
		if !strings.Contains(sqlByName[name], "delegateTo_employee_id") {
			t.Fatalf("%s missing delegateTo_employee_id: %s", name, sqlByName[name])
		}
	}
	if !strings.Contains(sqlByName["delegation_source_field_metadata"], "delegation_line_delegation_group_unique") ||
		!strings.Contains(sqlByName["delegation_source_field_metadata"], "delegation_id, group_id") {
		t.Fatalf("delegation source metadata migration missing unique index: %s", sqlByName["delegation_source_field_metadata"])
	}
	baseSQL, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(baseSQL), "delegateTo_employee_id") || !strings.Contains(string(baseSQL), "delegation_line_delegation_group_unique") {
		t.Fatalf("base SQL missing delegation source parity columns/index")
	}
}

func TestMailMigrationsExposeLogAccessFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, spec := range []struct {
		name  string
		table string
	}{
		{"mail_message", "mail_message"},
		{"mail_mail", "mail_mail"},
		{"mail_notification", "mail_notification"},
		{"mail_log_access_fields", "mail_message"},
		{"mail_log_access_fields", "mail_mail"},
		{"mail_log_access_fields", "mail_notification"},
	} {
		sql := sqlByName[spec.name]
		if !strings.Contains(sql, spec.table) {
			t.Fatalf("%s missing table %s: %s", spec.name, spec.table, sql)
		}
		for _, column := range []string{"create_uid", "create_date", "write_uid", "write_date"} {
			if !strings.Contains(sql, column) {
				t.Fatalf("%s missing %s: %s", spec.name, column, sql)
			}
		}
	}
}

func TestHRMigrationsExposeDelegationFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	hrSQL := sqlByName["hr_base_tables"]
	for _, table := range []string{"hr_department", "hr_employee", "hr_job", "hr_employee_category"} {
		if !strings.Contains(hrSQL, table) {
			t.Fatalf("hr migration missing table %s: %s", table, hrSQL)
		}
	}
	for _, column := range []string{"parent_id", "child_ids", "manager_id", "member_ids", "c_level_manager_id"} {
		if !strings.Contains(hrSQL, column) {
			t.Fatalf("hr_department migration missing %s: %s", column, hrSQL)
		}
	}
	for _, column := range []string{"user_id", "department_id", "parent_id", "child_ids", "work_email"} {
		if !strings.Contains(hrSQL, column) {
			t.Fatalf("hr_employee migration missing %s: %s", column, hrSQL)
		}
	}
	for _, column := range []string{"employee_ids", "employee_id", "work_email", "create_employee_id"} {
		if !strings.Contains(hrSQL, "ADD COLUMN IF NOT EXISTS "+column) {
			t.Fatalf("res_users HR migration missing %s: %s", column, hrSQL)
		}
	}
}

func TestResGroupsMigrationsExposeCategoryAndDelegationMetadata(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"category_id", "allow_delegation", "delegation_template_ids", "name_delegation", "allow_multiple_delegation", "restricted_access"} {
		if !strings.Contains(sqlByName["res_groups"], column) {
			t.Fatalf("res_groups missing %s: %s", column, sqlByName["res_groups"])
		}
		if !strings.Contains(sqlByName["res_groups_delegation_metadata"], column) {
			t.Fatalf("res_groups_delegation_metadata missing %s: %s", column, sqlByName["res_groups_delegation_metadata"])
		}
	}
}

func TestResGroupsMigrationsExposeHierarchySourceFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"full_name", "share", "sequence", "all_implied_ids", "all_implied_by_ids", "disjoint_ids", "all_user_ids", "all_users_count", "model_access", "rule_groups", "menu_access", "view_access", "view_group_hierarchy"} {
		if !strings.Contains(sqlByName["res_groups"], column) || !strings.Contains(sqlByName["res_groups_hierarchy_source_fields"], column) {
			t.Fatalf("res_groups hierarchy column %s missing: create=%s alter=%s", column, sqlByName["res_groups"], sqlByName["res_groups_hierarchy_source_fields"])
		}
	}
	for _, column := range []string{"description", "placeholder", "group_ids"} {
		if !strings.Contains(sqlByName["res_groups_privilege"], column) || !strings.Contains(sqlByName["res_groups_hierarchy_source_fields"], column) {
			t.Fatalf("res_groups_privilege column %s missing: create=%s alter=%s", column, sqlByName["res_groups_privilege"], sqlByName["res_groups_hierarchy_source_fields"])
		}
	}
}

func TestResUsersMigrationsExposeGroupPayloadFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"all_group_ids", "accesses_count", "rules_count", "groups_count", "view_group_hierarchy", "role"} {
		if !strings.Contains(sqlByName["res_users"], column) || !strings.Contains(sqlByName["res_users_group_payload_fields"], column) {
			t.Fatalf("res_users group payload column %s missing: create=%s alter=%s", column, sqlByName["res_users"], sqlByName["res_users_group_payload_fields"])
		}
	}
}

func TestReferenceDataMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"iso_numeric", "full_name", "currency_unit_label", "currency_subunit_label"} {
		if !strings.Contains(sqlByName["res_currency"], column) {
			t.Fatalf("res_currency missing %s: %s", column, sqlByName["res_currency"])
		}
	}
	for _, column := range []string{"phone_code", "vat_label", "state_required", "zip_required", "name_position"} {
		if !strings.Contains(sqlByName["res_country"], column) {
			t.Fatalf("res_country missing %s: %s", column, sqlByName["res_country"])
		}
	}
	for _, column := range []string{"name", "code", "country_ids"} {
		if !strings.Contains(sqlByName["res_country_group"], column) {
			t.Fatalf("res_country_group missing %s: %s", column, sqlByName["res_country_group"])
		}
	}
	for _, column := range []string{"grouping", "decimal_point", "thousands_sep", "date_format", "time_format", "week_start", "flag_image"} {
		if !strings.Contains(sqlByName["res_lang"], column) {
			t.Fatalf("res_lang missing %s: %s", column, sqlByName["res_lang"])
		}
	}
}

func TestModuleCategoryMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"parent_id", "child_ids", "module_ids", "privilege_ids", "description", "sequence", "visible", "exclusive", "xml_id"} {
		if !strings.Contains(sqlByName["ir_module_category"], column) {
			t.Fatalf("ir_module_category missing %s: %s", column, sqlByName["ir_module_category"])
		}
		if column != "parent_id" && column != "description" && column != "sequence" && !strings.Contains(sqlByName["ir_module_category_source_fields"], column) {
			t.Fatalf("ir_module_category_source_fields missing %s: %s", column, sqlByName["ir_module_category_source_fields"])
		}
	}
}

func TestPartnerSeedMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"is_company", "image_1920"} {
		if !strings.Contains(sqlByName["res_partner"], column) || !strings.Contains(sqlByName["res_partner_seed_fields"], column) {
			t.Fatalf("res_partner seed column %s missing: create=%s alter=%s", column, sqlByName["res_partner"], sqlByName["res_partner_seed_fields"])
		}
	}
	for _, column := range []string{"name", "full_name"} {
		if !strings.Contains(sqlByName["res_partner_seed_fields"], column) {
			t.Fatalf("res_partner_industry missing %s: %s", column, sqlByName["res_partner_seed_fields"])
		}
	}
}

func TestUserSeedMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"active_partner", "login_date", "group_ids", "signature", "image_1920"} {
		if !strings.Contains(sqlByName["res_users"], column) || !strings.Contains(sqlByName["res_users_source_seed_fields"], column) {
			if column == "active_partner" && strings.Contains(sqlByName["res_users_active_partner"], column) {
				continue
			}
			if column == "login_date" && strings.Contains(sqlByName["digest_res_users_login_date"], column) {
				continue
			}
			t.Fatalf("res_users source seed column %s missing: create=%s alter=%s active_partner=%s", column, sqlByName["res_users"], sqlByName["res_users_source_seed_fields"], sqlByName["res_users_active_partner"])
		}
	}
}

func TestExternalIDMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"complete_name", "noupdate"} {
		if !strings.Contains(sqlByName["ir_model_data"], column) || !strings.Contains(sqlByName["ir_model_data_complete_name_noupdate"], column) {
			t.Fatalf("ir_model_data source column %s missing: create=%s alter=%s", column, sqlByName["ir_model_data"], sqlByName["ir_model_data_complete_name_noupdate"])
		}
	}
	for _, token := range []string{"ir_model_data_name_nospaces", "ir_model_data_module_name_uniq", "ir_model_data_model_res_id_idx"} {
		if !strings.Contains(sqlByName["ir_model_data"], token) || !strings.Contains(sqlByName["ir_model_data_constraints"], token) {
			t.Fatalf("ir_model_data constraint/index %s missing: create=%s harden=%s", token, sqlByName["ir_model_data"], sqlByName["ir_model_data_constraints"])
		}
	}
}

func TestBaseSecurityAnchorMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	securitySQL := sqlByName["base_security_source_anchor_models"]
	for _, column := range []string{"commercial_partner_id", "share"} {
		if !strings.Contains(sqlByName["res_users"], column) || !strings.Contains(securitySQL, column) {
			t.Fatalf("res_users security column %s missing: create=%s alter=%s", column, sqlByName["res_users"], securitySQL)
		}
	}
	for _, column := range []string{"parent_id", "commercial_partner_id", "partner_share"} {
		if !strings.Contains(sqlByName["res_partner"], column) || !strings.Contains(securitySQL, column) {
			t.Fatalf("res_partner security column %s missing: create=%s alter=%s", column, sqlByName["res_partner"], securitySQL)
		}
	}
	for _, table := range []string{"res_users_log", "res_users_identitycheck", "change_password_user", "change_password_own", "res_device", "res_device_log", "res_currency_rate", "ir_ui_view_custom", "properties_base_definition"} {
		if !strings.Contains(securitySQL, table) {
			t.Fatalf("base security anchor missing %s: %s", table, securitySQL)
		}
	}
	aclAnchorSQL := sqlByName["base_source_acl_anchor_models"]
	for _, table := range []string{
		"decimal_precision", "ir_exports", "ir_exports_line", "ir_model_constraint", "ir_model_relation", "ir_model_inherit", "ir_model_fields_selection", "ir_module_module_dependency", "ir_module_module_exclusion", "reset_view_arch_wizard", "res_partner_category", "res_users_apikeys_description", "res_users_apikeys_show", "res_users_deletion", "report_layout", "ir_logging", "ir_mail_server", "ir_profile", "base_enable_profiling_wizard", "base_language_export", "base_language_import", "base_language_install", "base_module_update", "base_module_upgrade", "base_module_uninstall", "base_partner_merge_automatic_wizard", "base_partner_merge_line", "wizard_ir_model_menu_create", "ir_demo", "ir_demo_failure", "ir_demo_failure_wizard", "res_config", "change_password_wizard",
	} {
		if !strings.Contains(aclAnchorSQL, table) {
			t.Fatalf("base source ACL anchor missing %s: %s", table, aclAnchorSQL)
		}
	}
}

func TestSecurityMigrationsExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"name", "active", "model", "model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink"} {
		if !strings.Contains(sqlByName["ir_model_access"], column) {
			t.Fatalf("ir_model_access missing %s: %s", column, sqlByName["ir_model_access"])
		}
	}
	for _, column := range []string{"name", "model", "model_id", "domain", "domain_force", "groups", "group_ids", "global", "active", "perm_read", "perm_write", "perm_create", "perm_unlink"} {
		if !strings.Contains(sqlByName["ir_rule"], column) {
			t.Fatalf("ir_rule missing %s: %s", column, sqlByName["ir_rule"])
		}
	}
	for _, column := range []string{"model", "name", "abstract", "transient", "is_mail_thread", "is_mail_activity", "is_mail_blacklist"} {
		if !strings.Contains(sqlByName["ir_model"], column) {
			t.Fatalf("ir_model missing %s: %s", column, sqlByName["ir_model"])
		}
		if column != "model" && column != "name" && !strings.Contains(sqlByName["ir_model_mail_metadata_flags"], column) {
			t.Fatalf("ir_model_mail_metadata_flags missing %s: %s", column, sqlByName["ir_model_mail_metadata_flags"])
		}
	}
	for _, column := range []string{"model", "name", "ttype", "relation", "relation_field", "groups", "ai_vector_size"} {
		if !strings.Contains(sqlByName["ir_model_fields"], column) {
			t.Fatalf("ir_model_fields missing %s: %s", column, sqlByName["ir_model_fields"])
		}
		if column == "relation_field" || column == "groups" {
			if !strings.Contains(sqlByName["ir_model_fields_groups_relation_field"], column) {
				t.Fatalf("ir_model_fields_groups_relation_field missing %s: %s", column, sqlByName["ir_model_fields_groups_relation_field"])
			}
		}
		fieldGroupSQL := sqlByName["base_source_field_group_columns"]
		for _, column := range []string{"request", "smtp_user", "smtp_pass", "smtp_ssl_certificate", "smtp_ssl_private_key"} {
			if !strings.Contains(fieldGroupSQL, column) {
				t.Fatalf("base source field group column missing %s: %s", column, fieldGroupSQL)
			}
		}
	}
}

func TestServerActionMigrationsExposeWarningAndEquationFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{
		"sequence", "value", "html_value", "evaluation_type", "sequence_id", "resource_ref", "selection_value",
		"parent_id", "child_ids", "crud_model_id", "crud_model_name", "link_field_id", "group_ids",
		"update_field_id", "update_path", "update_related_model_id", "update_field_type", "update_m2m_operation",
		"update_boolean_value", "warning", "webhook_field_ids", "webhook_sample_payload",
	} {
		if !strings.Contains(sqlByName["ir_act_server"], column) {
			t.Fatalf("ir_act_server missing %s: %s", column, sqlByName["ir_act_server"])
		}
		if !strings.Contains(sqlByName["server_action_state_metadata_fields"], column) {
			t.Fatalf("server_action_state_metadata_fields missing %s: %s", column, sqlByName["server_action_state_metadata_fields"])
		}
	}
}

func TestCronMigrationExposeOdooCodeFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"ir_actions_server_id", "cron_name", "model_id", "state", "code", "action_name", "lastcall", "priority", "failure_count", "first_failure_date"} {
		if !strings.Contains(sqlByName["ir_cron"], column) {
			t.Fatalf("ir_cron missing %s: %s", column, sqlByName["ir_cron"])
		}
	}
	for _, column := range []string{"deactivate", "timed_out_counter", "started_at", "updated_at"} {
		if !strings.Contains(sqlByName["ir_cron_progress"], column) {
			t.Fatalf("ir_cron_progress missing %s: %s", column, sqlByName["ir_cron_progress"])
		}
		if !strings.Contains(sqlByName["cron_progress_loop_fields"], column) {
			t.Fatalf("cron_progress_loop_fields missing %s: %s", column, sqlByName["cron_progress_loop_fields"])
		}
	}
	for _, column := range []string{"crud_model_id", "link_field_id", "update_path", "warning", "webhook_field_ids", "child_ids", "template_id", "mail_post_method", "followers_type", "partner_ids", "activity_type_id", "activity_user_id"} {
		if !strings.Contains(sqlByName["ir_act_server"], column) {
			t.Fatalf("ir_act_server missing %s: %s", column, sqlByName["ir_act_server"])
		}
		if !strings.Contains(sqlByName["server_action_state_metadata_fields"], column) {
			t.Fatalf("server_action_state_metadata_fields missing %s: %s", column, sqlByName["server_action_state_metadata_fields"])
		}
	}
}

func TestResPartnerMigrationExposeSignupFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	if !strings.Contains(sqlByName["res_partner"], "signup_type") {
		t.Fatalf("res_partner missing signup_type: %s", sqlByName["res_partner"])
	}
	if !strings.Contains(sqlByName["res_partner_signup_type"], "signup_type") {
		t.Fatalf("res_partner_signup_type missing signup_type: %s", sqlByName["res_partner_signup_type"])
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "signup_type TEXT") {
		t.Fatalf("0001_base.sql missing signup_type")
	}
}

func TestStaticBaseSQLIncludesMassMailingCoreAndStatisticFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS mailing_mailing",
		"subject TEXT",
		"body_html TEXT",
		"state TEXT NOT NULL DEFAULT 'draft'",
		"kpi_mail_required BOOLEAN NOT NULL DEFAULT false",
		"mailing_model_real TEXT",
		"use_exclusion_list BOOLEAN NOT NULL DEFAULT true",
		"received_ratio DOUBLE PRECISION NOT NULL DEFAULT 0",
		"clicks_ratio DOUBLE PRECISION NOT NULL DEFAULT 0",
		"link_trackers_count INTEGER NOT NULL DEFAULT 0",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("0001_base.sql missing %s", fragment)
		}
	}
}

func TestStaticBaseSQLIncludesSMSComposerWizardFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS sms_composer",
		"composition_mode TEXT",
		"res_model_description TEXT",
		"res_ids_count INTEGER NOT NULL DEFAULT 0",
		"recipient_single_number_itf TEXT",
		"mass_force_send BOOLEAN NOT NULL DEFAULT false",
		"template_id BIGINT",
		"write_date TIMESTAMPTZ",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("0001_base.sql missing %s", fragment)
		}
	}
}

func TestSMSComposerMigrationExposesWizardFields(t *testing.T) {
	var sql string
	for _, migration := range BaseMigrations {
		if migration.Name == "sms_composer_wizard_parity" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("sms_composer_wizard_parity migration missing")
	}
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS sms_composer",
		"composition_mode TEXT",
		"comment_single_recipient BOOLEAN NOT NULL DEFAULT false",
		"use_exclusion_list BOOLEAN NOT NULL DEFAULT true",
		"recipient_valid_count INTEGER NOT NULL DEFAULT 0",
		"sanitized_numbers TEXT",
		"template_id BIGINT",
		"create_uid BIGINT",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sms_composer_wizard_parity missing %s: %s", fragment, sql)
		}
	}
}

func TestMailMessageMigrationExposeThreadFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"parent_id", "subtype_id", "partner_ids", "attachment_ids", "body_is_html", "is_internal", "reaction_ids", "starred", "starred_partner_ids", "tracking_value_ids", "author_guest_id", "incoming_email_to", "incoming_email_cc", "outgoing_email_to", "reply_to", "reply_to_force_new", "mail_server_id", "email_layout_xmlid", "email_add_signature"} {
		if !strings.Contains(sqlByName["mail_message"], column) {
			t.Fatalf("mail_message missing %s: %s", column, sqlByName["mail_message"])
		}
	}
	for _, column := range []string{"message_id", "content", "partner_id", "guest_id"} {
		if !strings.Contains(sqlByName["mail_message_reaction"], column) {
			t.Fatalf("mail_message_reaction missing %s: %s", column, sqlByName["mail_message_reaction"])
		}
	}
	for _, fragment := range []string{
		"mail_message_reaction_partner_unique",
		"mail_message_reaction_guest_unique",
		"mail_message_reaction_partner_or_guest_exists",
		"CHECK ((partner_id IS NOT NULL AND guest_id IS NULL) OR (partner_id IS NULL AND guest_id IS NOT NULL))",
	} {
		if !strings.Contains(sqlByName["mail_message_reaction"], fragment) {
			t.Fatalf("mail_message_reaction missing %s: %s", fragment, sqlByName["mail_message_reaction"])
		}
	}
	for _, column := range []string{"access_token", "country_id", "lang", "timezone"} {
		if !strings.Contains(sqlByName["mail_guest"], column) {
			t.Fatalf("mail_guest missing %s: %s", column, sqlByName["mail_guest"])
		}
	}
	if !strings.Contains(sqlByName["discuss_channel"], "group_public_id") {
		t.Fatalf("discuss_channel missing group_public_id: %s", sqlByName["discuss_channel"])
	}
	if !strings.Contains(sqlByName["discuss_channel_member"], "guest_id") {
		t.Fatalf("discuss_channel_member missing guest_id: %s", sqlByName["discuss_channel_member"])
	}
	if !strings.Contains(sqlByName["mail_presence"], "guest_id") {
		t.Fatalf("mail_presence missing guest_id: %s", sqlByName["mail_presence"])
	}
	for _, column := range []string{"field_id", "field_info", "field_name", "field_desc", "field_type", "old_value_integer", "old_value_float", "old_value_char", "old_value_text", "old_value_datetime", "new_value_integer", "new_value_float", "new_value_char", "new_value_text", "new_value_datetime", "currency_id", "mail_message_id"} {
		if !strings.Contains(sqlByName["mail_tracking_value"], column) {
			t.Fatalf("mail_tracking_value missing %s: %s", column, sqlByName["mail_tracking_value"])
		}
	}
	if !strings.Contains(sqlByName["mail_message_subtype"], "description") {
		t.Fatalf("mail_message_subtype missing description: %s", sqlByName["mail_message_subtype"])
	}
	if !strings.Contains(sqlByName["mail_message"], "mail_activity_type_id") || !strings.Contains(sqlByName["mail_message_activity_type"], "mail_activity_type_id") {
		t.Fatalf("mail_message missing mail_activity_type_id")
	}
	for _, column := range []string{"automated", "hide_in_chatter"} {
		if !strings.Contains(sqlByName["mail_activity"], column) {
			t.Fatalf("mail_activity missing %s: %s", column, sqlByName["mail_activity"])
		}
	}
	for _, column := range []string{"activity_category", "recommended_activity_type_id", "previous_activity_type_id", "has_recommended_activities", "chaining_type", "active", "date_done", "feedback"} {
		if !strings.Contains(sqlByName["mail_activity"], column) || !strings.Contains(sqlByName["mail_activity_chaining_archive"], column) {
			if !strings.Contains(sqlByName["mail_activity_recommendation_fields"], column) {
				t.Fatalf("mail_activity missing %s", column)
			}
		}
	}
	if !strings.Contains(sqlByName["mail_activity"], "attachment_ids") || !strings.Contains(sqlByName["mail_activity_attachment_ids"], "attachment_ids") {
		t.Fatalf("mail_activity missing attachment_ids")
	}
	if !strings.Contains(sqlByName["activity_attachment_rel"], "activity_id") || !strings.Contains(sqlByName["activity_attachment_rel"], "attachment_id") {
		t.Fatalf("activity_attachment_rel missing columns: %s", sqlByName["activity_attachment_rel"])
	}
	if !strings.Contains(sqlByName["mail_activity_type"], "summary") ||
		!strings.Contains(sqlByName["mail_activity_type"], "default_user_id") {
		t.Fatalf("mail_activity_type missing Odoo fields: %s", sqlByName["mail_activity_type"])
	}
	for _, column := range []string{"delay_count", "delay_unit", "delay_from", "chaining_type", "triggered_next_type_id", "suggested_next_type_ids"} {
		if !strings.Contains(sqlByName["mail_activity_type"], column) || !strings.Contains(sqlByName["mail_activity_chaining_archive"], column) {
			t.Fatalf("mail_activity_type missing %s", column)
		}
	}
	for _, column := range []string{"previous_type_ids", "icon"} {
		if !strings.Contains(sqlByName["mail_activity_type"], column) || !strings.Contains(sqlByName["mail_activity_recommendation_fields"], column) {
			t.Fatalf("mail_activity_type missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["mail_activity_type"], "mail_template_ids") || !strings.Contains(sqlByName["mail_activity_type_templates"], "mail_template_ids") {
		t.Fatalf("mail_activity_type missing mail_template_ids")
	}
	if !strings.Contains(sqlByName["mail_activity_recommendation_fields"], "mail_activity_rel") ||
		!strings.Contains(sqlByName["mail_activity_recommendation_fields"], "recommended_id") {
		t.Fatalf("mail_activity_rel missing columns: %s", sqlByName["mail_activity_recommendation_fields"])
	}
	if !strings.Contains(sqlByName["cron_mail_thread_payload_fields"], "ir_actions_server_id") ||
		!strings.Contains(sqlByName["cron_mail_thread_payload_fields"], "body_is_html") {
		t.Fatalf("upgrade migration missing cron/mail fields: %s", sqlByName["cron_mail_thread_payload_fields"])
	}
	if !strings.Contains(sqlByName["mail_activity_message_parity"], "attachment_ids") ||
		!strings.Contains(sqlByName["mail_activity_message_parity"], "automated") ||
		!strings.Contains(sqlByName["mail_activity_message_parity"], "default_user_id") {
		t.Fatalf("mail parity migration missing fields: %s", sqlByName["mail_activity_message_parity"])
	}
	for _, column := range []string{"author_id", "email_from", "email_cc", "reply_to"} {
		if !strings.Contains(sqlByName["mail_mail"], column) || !strings.Contains(sqlByName["mail_compose_template_batch_parity"], column) {
			t.Fatalf("mail mail missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["mail_mail_inherited_author"], "ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS author_id BIGINT") {
		t.Fatalf("mail mail inherited author migration missing: %s", sqlByName["mail_mail_inherited_author"])
	}
	for _, column := range []string{"recipient_ids", "attachment_ids", "mail_server_id", "failure_type", "auto_delete", "message_id", "references", "headers", "is_notification", "fetchmail_server_id"} {
		if !strings.Contains(sqlByName["mail_mail"], column) || !strings.Contains(sqlByName["mail_persistent_queue_fields"], column) {
			t.Fatalf("mail persistent queue field missing %s", column)
		}
	}
	for _, column := range []string{"mailing_id", "mailing_trace_ids"} {
		if !strings.Contains(sqlByName["mail_mail"], column) {
			t.Fatalf("mail mass mailing field missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["mail_message"], "message_id") || !strings.Contains(sqlByName["mail_bounce_processing_fields"], "mail_message ADD COLUMN IF NOT EXISTS message_id") {
		t.Fatalf("mail message bounce field missing")
	}
	for _, column := range []string{"email_normalized", "message_bounce"} {
		if !strings.Contains(sqlByName["res_partner"], column) || !strings.Contains(sqlByName["mail_bounce_processing_fields"], column) {
			t.Fatalf("res_partner bounce field missing %s", column)
		}
	}
	for _, column := range []string{"mail_mail_id", "mail_email_address", "failure_reason", "is_read", "read_date", "author_id"} {
		if !strings.Contains(sqlByName["mail_notification"], column) || !strings.Contains(sqlByName["mail_persistent_queue_fields"], column) {
			t.Fatalf("mail notification queue field missing %s", column)
		}
	}
	for _, column := range []string{"smtp_host", "smtp_port", "smtp_encryption", "smtp_authentication", "from_filter", "sequence", "smtp_debug", "max_email_size"} {
		if !strings.Contains(sqlByName["mail_persistent_queue_fields"], column) {
			t.Fatalf("ir_mail_server queue field missing %s", column)
		}
	}
	for _, column := range []string{"owner_user_id", "owner_limit_time", "owner_limit_count"} {
		if !strings.Contains(sqlByName["mail_server_owner_alias_domain"], column) {
			t.Fatalf("ir_mail_server owner field missing %s", column)
		}
	}
	for _, column := range []string{"notification_type", "outgoing_mail_server_id", "outgoing_mail_server_type", "has_external_mail_server"} {
		if !strings.Contains(sqlByName["mail_server_owner_alias_domain"], column) {
			t.Fatalf("res_users mail server field missing %s", column)
		}
	}
	for _, column := range []string{"alias_domain_id", "alias_domain", "alias_full_name", "CREATE TABLE IF NOT EXISTS mail_alias_domain", "default_from_email"} {
		if !strings.Contains(sqlByName["mail_server_owner_alias_domain"], column) {
			t.Fatalf("mail alias domain migration missing %s", column)
		}
	}
	for _, column := range []string{"alias_model_id", "alias_defaults", "alias_force_thread_id", "alias_parent_model_id", "alias_parent_thread_id", "alias_contact", "alias_incoming_local", "alias_bounced_content", "alias_status"} {
		if !strings.Contains(sqlByName["mail_alias"], column) || !strings.Contains(sqlByName["mail_alias_route_fields"], column) {
			t.Fatalf("mail alias route field missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["mail_gateway_allowed"], "CREATE TABLE IF NOT EXISTS mail_gateway_allowed") ||
		!strings.Contains(sqlByName["mail_gateway_allowed"], "email_normalized") {
		t.Fatalf("mail gateway allowed migration missing fields: %s", sqlByName["mail_gateway_allowed"])
	}
	traceSQL := sqlByName["mailing_trace_side_effect_models"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS mail_blacklist",
		"CREATE TABLE IF NOT EXISTS utm_campaign",
		"ab_testing_winner_mailing_id BIGINT",
		"CREATE TABLE IF NOT EXISTS utm_source",
		"CREATE TABLE IF NOT EXISTS utm_medium",
		"CREATE TABLE IF NOT EXISTS mailing_contact",
		"CREATE TABLE IF NOT EXISTS mailing_list",
		"CREATE TABLE IF NOT EXISTS mailing_subscription",
		"CREATE TABLE IF NOT EXISTS mailing_subscription_optout",
		"CREATE TABLE IF NOT EXISTS mailing_mailing",
		"CREATE TABLE IF NOT EXISTS mailing_mailing_test",
		"CREATE TABLE IF NOT EXISTS mailing_mailing_schedule_date",
		"subject TEXT",
		"preview TEXT",
		"body_html TEXT",
		"email_from TEXT",
		"reply_to_mode TEXT",
		"mail_server_id BIGINT",
		"attachment_ids TEXT",
		"keep_archives BOOLEAN",
		"state TEXT",
		"sent_date TIMESTAMPTZ",
		"schedule_type TEXT",
		"schedule_date TIMESTAMPTZ",
		"kpi_mail_required BOOLEAN",
		"user_id BIGINT",
		"mailing_model_real TEXT",
		"mailing_domain TEXT",
		"mailing_on_mailing_list BOOLEAN",
		"use_exclusion_list BOOLEAN",
		"contact_list_ids TEXT",
		"ab_testing_enabled BOOLEAN",
		"ab_testing_pc INTEGER",
		"ab_testing_winner_selection TEXT",
		"received_ratio DOUBLE PRECISION",
		"link_trackers_count INTEGER",
		"CREATE TABLE IF NOT EXISTS link_tracker",
		"CREATE TABLE IF NOT EXISTS link_tracker_code",
		"link_tracker_code_code_unique",
		"CREATE TABLE IF NOT EXISTS link_tracker_click",
		"CREATE TABLE IF NOT EXISTS sms_sms",
		"CREATE TABLE IF NOT EXISTS sms_tracker",
		"CREATE TABLE IF NOT EXISTS mailing_trace",
		"mailing_trace_id BIGINT",
		"mass_mailing_id BIGINT",
		"whatsapp_message_id BIGINT",
		"sms_id_int BIGINT",
		"sms_tracker_ids TEXT",
		"sms_number TEXT",
		"sms_code TEXT",
		"message_id TEXT",
		"trace_status TEXT",
		"reply_datetime TIMESTAMPTZ",
		"failure_type TEXT",
		"ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS mailing_id BIGINT",
		"ALTER TABLE mail_mail ADD COLUMN IF NOT EXISTS mailing_trace_ids TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS subject TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS preview TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS body_html TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS email_from TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS reply_to_mode TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS state TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS schedule_type TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS user_id BIGINT",
		"ALTER TABLE utm_campaign ADD COLUMN IF NOT EXISTS ab_testing_winner_mailing_id BIGINT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS ab_testing_enabled BOOLEAN",
	} {
		if !strings.Contains(traceSQL, fragment) {
			t.Fatalf("mailing trace migration missing %s: %s", fragment, traceSQL)
		}
	}
	whatsappSQL := sqlByName["whatsapp_tracked_link_route_parity"]
	for _, fragment := range []string{
		"ALTER TABLE link_tracker_click ADD COLUMN IF NOT EXISTS whatsapp_message_id BIGINT",
		"CREATE TABLE IF NOT EXISTS marketing_campaign",
		"utm_campaign_id BIGINT",
		"state TEXT NOT NULL DEFAULT 'draft'",
		"CREATE TABLE IF NOT EXISTS marketing_activity",
		"activity_type TEXT",
		"campaign_id BIGINT",
		"whatsapp_template_id BIGINT",
		"server_action_id BIGINT",
		"interval_number INTEGER NOT NULL DEFAULT 0",
		"interval_type TEXT NOT NULL DEFAULT 'hours'",
		"CREATE TABLE IF NOT EXISTS whatsapp_message",
		"trigger_category TEXT",
		"whatsapp_error BOOLEAN",
		"links_click_datetime TIMESTAMPTZ",
		"marketing_trace_ids TEXT",
		"CREATE TABLE IF NOT EXISTS marketing_trace",
		"activity_id BIGINT",
		"whatsapp_message_id BIGINT",
		"parent_id BIGINT",
		"child_ids TEXT",
	} {
		if !strings.Contains(whatsappSQL, fragment) {
			t.Fatalf("whatsapp tracked-link migration missing %s: %s", fragment, whatsappSQL)
		}
	}
	whatsappTemplateSQL := sqlByName["whatsapp_template_tracked_link_generation"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS whatsapp_template",
		"active BOOLEAN NOT NULL DEFAULT true",
		"model TEXT",
		"phone_field TEXT",
		"status TEXT",
		"quality TEXT",
		"header_type TEXT",
		"header_text TEXT",
		"header_attachment_ids TEXT",
		"footer_text TEXT",
		"body TEXT",
		"variable_ids TEXT",
		"button_ids TEXT",
		"CREATE TABLE IF NOT EXISTS whatsapp_template_button",
		"template_id BIGINT",
		"wa_template_id BIGINT",
		"button_type TEXT",
		"url_type TEXT",
		"website_url TEXT",
		"dynamic_url TEXT",
		"CREATE TABLE IF NOT EXISTS whatsapp_template_variable",
		"field_type TEXT",
		"demo_value TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS template_id BIGINT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_template_id BIGINT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS msg_uid TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mail_message_id BIGINT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS components TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS free_text_json TEXT",
		"ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS wa_template_id BIGINT",
	} {
		if !strings.Contains(whatsappTemplateSQL, fragment) {
			t.Fatalf("whatsapp template migration missing %s: %s", fragment, whatsappTemplateSQL)
		}
	}
	whatsappVariableSQL := sqlByName["whatsapp_template_variable_validation_fields"]
	for _, fragment := range []string{
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS status TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS quality TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS active BOOLEAN",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS model TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS phone_field TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_text TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_attachment_ids TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS footer_text TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS free_text_json TEXT",
		"CREATE TABLE IF NOT EXISTS whatsapp_template_variable",
		"button_id BIGINT",
		"line_type TEXT",
		"field_type TEXT",
		"demo_value TEXT",
	} {
		if !strings.Contains(whatsappVariableSQL, fragment) {
			t.Fatalf("whatsapp variable migration missing %s: %s", fragment, whatsappVariableSQL)
		}
	}
	whatsappWebhookSQL := sqlByName["whatsapp_template_webhook_account_fields"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS whatsapp_account",
		"active BOOLEAN NOT NULL DEFAULT true",
		"app_uid TEXT",
		"account_uid TEXT",
		"phone_uid TEXT",
		"phone_number TEXT",
		"token TEXT",
		"app_secret TEXT",
		"webhook_verify_token TEXT",
		"callback_url TEXT",
		"debug_logging BOOLEAN NOT NULL DEFAULT false",
		"templates_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS wa_account_id BIGINT",
	} {
		if !strings.Contains(whatsappWebhookSQL, fragment) {
			t.Fatalf("whatsapp webhook migration missing %s: %s", fragment, whatsappWebhookSQL)
		}
	}
	whatsappSyncSQL := sqlByName["whatsapp_account_template_sync_fields"]
	for _, fragment := range []string{
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS app_uid TEXT",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS phone_number TEXT",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS token TEXT",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS callback_url TEXT",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS debug_logging BOOLEAN NOT NULL DEFAULT false",
		"ALTER TABLE whatsapp_account ADD COLUMN IF NOT EXISTS templates_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS header_attachment_ids TEXT",
		"ALTER TABLE whatsapp_template ADD COLUMN IF NOT EXISTS footer_text TEXT",
	} {
		if !strings.Contains(whatsappSyncSQL, fragment) {
			t.Fatalf("whatsapp sync migration missing %s: %s", fragment, whatsappSyncSQL)
		}
	}
	whatsappStatusSQL := sqlByName["whatsapp_message_status_webhook_fields"]
	for _, fragment := range []string{
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mobile_number TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS mobile_number_formatted TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS message_type TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS failure_type TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS failure_reason TEXT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_account_id BIGINT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS parent_id BIGINT",
	} {
		if !strings.Contains(whatsappStatusSQL, fragment) {
			t.Fatalf("whatsapp status migration missing %s: %s", fragment, whatsappStatusSQL)
		}
	}
	whatsappSourceFieldSQL := sqlByName["whatsapp_source_template_fields"]
	for _, fragment := range []string{
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS wa_template_id BIGINT",
		"ALTER TABLE whatsapp_message ADD COLUMN IF NOT EXISTS msg_uid TEXT",
		"ALTER TABLE whatsapp_template_button ADD COLUMN IF NOT EXISTS wa_template_id BIGINT",
	} {
		if !strings.Contains(whatsappSourceFieldSQL, fragment) {
			t.Fatalf("whatsapp source field migration missing %s: %s", fragment, whatsappSourceFieldSQL)
		}
	}
	smsSQL := sqlByName["sms_tracked_link_route_parity"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS sms_sms",
		"mailing_id BIGINT",
		"mailing_trace_ids TEXT",
		"CREATE TABLE IF NOT EXISTS sms_tracker",
		"mailing_trace_id BIGINT",
		"CREATE UNIQUE INDEX IF NOT EXISTS sms_sms_uuid_unique ON sms_sms(uuid)",
		"CREATE UNIQUE INDEX IF NOT EXISTS sms_tracker_sms_uuid_unique ON sms_tracker(sms_uuid)",
		"ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_id BIGINT",
		"ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_id_int BIGINT",
		"ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_tracker_ids TEXT",
		"ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_number TEXT",
		"ALTER TABLE mailing_trace ADD COLUMN IF NOT EXISTS sms_code TEXT",
	} {
		if !strings.Contains(smsSQL, fragment) {
			t.Fatalf("sms tracked-link migration missing %s: %s", fragment, smsSQL)
		}
	}
	smsTemplateSQL := sqlByName["sms_template_tracked_link_generation"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS sms_template",
		"model_id BIGINT",
		"model TEXT",
		"body TEXT",
		"sidebar_action_id BIGINT",
	} {
		if !strings.Contains(smsTemplateSQL, fragment) {
			t.Fatalf("sms template migration missing %s: %s", fragment, smsTemplateSQL)
		}
	}
	smsOptOutSQL := sqlByName["sms_opt_out_parity"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS phone_blacklist",
		"number TEXT",
		"active BOOLEAN NOT NULL DEFAULT true",
		"phone_blacklist_number_unique",
		"ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS phone_sanitized TEXT",
		"ALTER TABLE res_partner ADD COLUMN IF NOT EXISTS phone_blacklisted BOOLEAN NOT NULL DEFAULT false",
		"ALTER TABLE mailing_contact ADD COLUMN IF NOT EXISTS phone TEXT",
		"ALTER TABLE mailing_contact ADD COLUMN IF NOT EXISTS phone_sanitized TEXT",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS sms_allow_unsubscribe BOOLEAN NOT NULL DEFAULT false",
	} {
		if !strings.Contains(smsOptOutSQL, fragment) {
			t.Fatalf("sms opt-out migration missing %s: %s", fragment, smsOptOutSQL)
		}
	}
	mailingStatsSQL := sqlByName["mass_mailing_statistics_fields"]
	for _, fragment := range []string{
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS total INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS expected INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS received_ratio DOUBLE PRECISION NOT NULL DEFAULT 0",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS clicks_ratio DOUBLE PRECISION NOT NULL DEFAULT 0",
		"ALTER TABLE mailing_mailing ADD COLUMN IF NOT EXISTS link_trackers_count INTEGER NOT NULL DEFAULT 0",
	} {
		if !strings.Contains(mailingStatsSQL, fragment) {
			t.Fatalf("mailing stats migration missing %s: %s", fragment, mailingStatsSQL)
		}
	}
	smsStatusSQL := sqlByName["sms_delivery_status_webhook_parity"]
	for _, fragment := range []string{
		"ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_id BIGINT",
		"ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_id_int BIGINT",
		"ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_tracker_ids TEXT",
		"ALTER TABLE mail_notification ADD COLUMN IF NOT EXISTS sms_number TEXT",
		"CREATE UNIQUE INDEX IF NOT EXISTS sms_sms_uuid_unique ON sms_sms(uuid)",
		"CREATE UNIQUE INDEX IF NOT EXISTS sms_tracker_sms_uuid_unique ON sms_tracker(sms_uuid)",
	} {
		if !strings.Contains(smsStatusSQL, fragment) {
			t.Fatalf("sms status migration missing %s: %s", fragment, smsStatusSQL)
		}
	}
	for _, column := range []string{"server", "port", "server_type_info", "is_ssl", "attach", "original", "date", "error_date", "error_message", `"user"`, "password", "object_id", "priority", "message_ids", "configuration", "script"} {
		if !strings.Contains(sqlByName["fetchmail_server"], column) || !strings.Contains(sqlByName["fetchmail_server_parity_fields"], column) {
			t.Fatalf("fetchmail server migration missing %s", column)
		}
	}
	for _, column := range []string{"model_id", "email_from", "email_cc", "reply_to", "partner_to", "scheduled_date"} {
		if !strings.Contains(sqlByName["mail_template"], column) || !strings.Contains(sqlByName["mail_compose_template_batch_parity"], column) {
			t.Fatalf("mail template missing %s", column)
		}
	}
	for _, column := range []string{"attachment_ids", "mail_server_id", "auto_delete"} {
		if !strings.Contains(sqlByName["mail_template"], column) || !strings.Contains(sqlByName["mail_template_queue_fields"], column) {
			t.Fatalf("mail template queue field missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["mail_template"], "report_template_ids") || !strings.Contains(sqlByName["mail_template_dynamic_reports"], "report_template_ids") {
		t.Fatalf("mail template missing report_template_ids")
	}
	for _, name := range []string{"mail_compose_message", "mail_scheduled_message"} {
		if !strings.Contains(sqlByName[name], "scheduled_date") || !strings.Contains(sqlByName["mail_compose_template_batch_parity"], name) {
			t.Fatalf("%s missing compose parity fields", name)
		}
	}
	for _, column := range []string{"composition_comment_option", "message_type", "subtype_is_log", "mass_mailing_id", "use_exclusion_list"} {
		if !strings.Contains(sqlByName["mail_compose_message"], column) || !strings.Contains(sqlByName["mail_compose_schedule_fields"], column) {
			t.Fatalf("mail compose missing %s", column)
		}
	}
	for _, column := range []string{"attachment_ids", "composition_comment_option", "is_note", "notification_parameters", "send_context"} {
		if !strings.Contains(sqlByName["mail_scheduled_message"], column) || !strings.Contains(sqlByName["mail_compose_schedule_fields"], column) {
			t.Fatalf("mail scheduled message missing %s", column)
		}
	}
}

func TestStaticBaseSQLIncludesMailReactionConstraints(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"author_guest_id",
		"is_internal",
		"reaction_ids",
		"starred_partner_ids",
		"message_id TEXT",
		"CREATE TABLE IF NOT EXISTS mail_blacklist",
		"CREATE TABLE IF NOT EXISTS phone_blacklist",
		"phone_blacklist_number_unique",
		"CREATE TABLE IF NOT EXISTS link_tracker",
		"CREATE TABLE IF NOT EXISTS link_tracker_code",
		"link_tracker_code_code_unique",
		"CREATE TABLE IF NOT EXISTS link_tracker_click",
		"CREATE TABLE IF NOT EXISTS sms_sms",
		"CREATE TABLE IF NOT EXISTS sms_tracker",
		"sms_sms_uuid_unique",
		"sms_tracker_sms_uuid_unique",
		"CREATE TABLE IF NOT EXISTS mailing_trace",
		"phone_sanitized TEXT",
		"phone_blacklisted BOOLEAN NOT NULL DEFAULT false",
		"sms_allow_unsubscribe BOOLEAN NOT NULL DEFAULT false",
		"sms_id_int BIGINT",
		"sms_tracker_ids TEXT",
		"whatsapp_message_id BIGINT",
		"CREATE TABLE IF NOT EXISTS marketing_campaign",
		"CREATE TABLE IF NOT EXISTS marketing_activity",
		"activity_type TEXT",
		"whatsapp_template_id BIGINT",
		"trigger_category TEXT",
		"whatsapp_error BOOLEAN",
		"CREATE TABLE IF NOT EXISTS whatsapp_account",
		"account_uid TEXT",
		"webhook_verify_token TEXT",
		"CREATE TABLE IF NOT EXISTS whatsapp_template",
		"CREATE TABLE IF NOT EXISTS whatsapp_template_button",
		"wa_template_id BIGINT",
		"CREATE TABLE IF NOT EXISTS sms_template",
		"sidebar_action_id BIGINT",
		"CREATE TABLE IF NOT EXISTS whatsapp_message",
		"mobile_number TEXT",
		"message_type TEXT",
		"failure_type TEXT",
		"failure_reason TEXT",
		"msg_uid TEXT",
		"wa_account_id BIGINT",
		"parent_id BIGINT",
		"components TEXT",
		"CREATE TABLE IF NOT EXISTS marketing_trace",
		"incoming_email_to TEXT",
		"reply_to_force_new BOOLEAN",
		"CREATE TABLE IF NOT EXISTS mail_gateway_allowed",
		"CREATE TABLE IF NOT EXISTS mail_inbound_message_lock",
		"message_id TEXT NOT NULL",
		"mail_inbound_message_lock_message_id_unique",
		"CREATE TABLE IF NOT EXISTS mail_message_reaction",
		"mail_message_reaction_partner_unique",
		"mail_message_reaction_guest_unique",
		"mail_message_reaction_partner_or_guest_exists",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("0001_base.sql missing %s", fragment)
		}
	}
}

func TestLinkTrackerCodeSQLUniqueConstraint(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, name := range []string{"mailing_trace_side_effect_models", "link_tracker_code_unique"} {
		if !strings.Contains(sqlByName[name], "link_tracker_code_code_unique") || !strings.Contains(sqlByName[name], "link_tracker_code(code)") {
			t.Fatalf("%s missing link_tracker_code code uniqueness: %s", name, sqlByName[name])
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	if !strings.Contains(sql, "link_tracker_code_code_unique") || !strings.Contains(sql, "link_tracker_code(code)") {
		t.Fatalf("0001_base.sql missing link_tracker_code code uniqueness")
	}
}

func TestMailInboundDuplicateLockMigration(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	sql := sqlByName["mail_inbound_message_lock"]
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS mail_inbound_message_lock",
		"message_id TEXT NOT NULL",
		"mail_inbound_message_lock_message_id_unique",
		"ON mail_inbound_message_lock (message_id)",
		"create_date TIMESTAMPTZ",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("mail inbound duplicate lock migration missing %s: %s", fragment, sql)
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	staticSQL := string(data)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS mail_inbound_message_lock",
		"message_id TEXT NOT NULL",
		"mail_inbound_message_lock_message_id_unique",
		"ON mail_inbound_message_lock (message_id)",
	} {
		if !strings.Contains(staticSQL, fragment) {
			t.Fatalf("0001_base.sql missing %s", fragment)
		}
	}
}

func TestStaticBaseSQLIncludesAttachmentParityFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_base.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"email_normalized TEXT",
		"message_bounce BIGINT NOT NULL DEFAULT 0",
		"res_field TEXT",
		"company_id BIGINT",
		"file_size BIGINT",
		"checksum TEXT",
		"has_thumbnail BOOLEAN NOT NULL DEFAULT false",
		"restrictive_audit_trail BOOLEAN NOT NULL DEFAULT false",
		"ubl_cii_xml_file TEXT",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("0001_base.sql missing %s", fragment)
		}
	}
}

func TestActionWindowMigrationExposeOdooFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"type", "res_id", "views", "embedded_action_ids", "close_on_report_download", "mobile_view_mode", "view_id", "search_view_id", "limit", "help", "path", "usage", "filter", "cache", "group_ids", "binding_model_id", "binding_view_types"} {
		if !strings.Contains(sqlByName["ir_act_window"], column) {
			t.Fatalf("ir_act_window missing %s: %s", column, sqlByName["ir_act_window"])
		}
	}
	for _, token := range []string{"DEFAULT 'ir.actions.act_window'", "DEFAULT 'list,form'", "DEFAULT 'kanban'", "DEFAULT 'current'", "DEFAULT 80", "DEFAULT true"} {
		if !strings.Contains(sqlByName["ir_act_window"], token) && !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("ir_act_window default missing %s", token)
		}
	}
	for _, column := range []string{"type", "close"} {
		if !strings.Contains(sqlByName["ir_act_url"], column) || !strings.Contains(sqlByName["base_action_metadata_parity"], column) {
			t.Fatalf("ir_act_url metadata missing %s", column)
		}
	}
	for _, token := range []string{"DEFAULT 'ir.actions.act_url'", "DEFAULT 'new'"} {
		if !strings.Contains(sqlByName["ir_act_url"], token) && !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("ir_act_url default missing %s", token)
		}
	}
	for _, column := range []string{"help", "path", "xml_id", "binding_model_id", "binding_type", "binding_view_types", "effect", "infos"} {
		if !strings.Contains(sqlByName["ir_actions"], column) {
			t.Fatalf("ir_actions missing %s: %s", column, sqlByName["ir_actions"])
		}
	}
	for _, column := range []string{"multi"} {
		if !strings.Contains(sqlByName["ir_act_window_view"], column) {
			t.Fatalf("ir_act_window_view missing %s: %s", column, sqlByName["ir_act_window_view"])
		}
	}
	for _, column := range []string{"name"} {
		if !strings.Contains(sqlByName["ir_actions_todo"], column) {
			t.Fatalf("ir_actions_todo missing %s: %s", column, sqlByName["ir_actions_todo"])
		}
	}
	for _, column := range []string{"type", "binding_type", "automated_name", "allowed_states", "available_model_ids", "ir_cron_ids", "show_code_history", "value_field_to_show", "sms_template_id", "sms_method", "wa_template_id", "documents_account_create_model", "documents_account_journal_id", "documents_account_suitable_journal_ids", "documents_account_move_type"} {
		if !strings.Contains(sqlByName["ir_act_server"], column) || !strings.Contains(sqlByName["base_action_metadata_parity"], column) {
			t.Fatalf("ir_act_server metadata missing %s", column)
		}
	}
	for _, token := range []string{"DEFAULT 'ir.actions.server'", "DEFAULT 'ir_actions_server'", "DEFAULT 'value'", "DEFAULT 'add'", "DEFAULT 'true'"} {
		if !strings.Contains(sqlByName["ir_act_server"], token) && !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("ir_act_server default missing %s", token)
		}
	}
	for _, column := range []string{"type", "context", "params", "params_store"} {
		if !strings.Contains(sqlByName["ir_act_client"], column) || !strings.Contains(sqlByName["base_action_metadata_parity"], column) {
			t.Fatalf("ir_act_client metadata missing %s", column)
		}
	}
	for _, token := range []string{"DEFAULT 'ir.actions.client'", "DEFAULT 'current'"} {
		if !strings.Contains(sqlByName["ir_act_client"], token) && !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("ir_act_client default missing %s", token)
		}
	}
	for _, column := range []string{"type", "model_id", "target", "context", "data", "close_on_report_download", "multi", "binding_view_types"} {
		if !strings.Contains(sqlByName["ir_act_report_xml"], column) || !strings.Contains(sqlByName["base_action_metadata_parity"], column) {
			t.Fatalf("ir_act_report_xml metadata missing %s", column)
		}
	}
	for _, token := range []string{"DEFAULT 'ir.actions.report'", "DEFAULT 'qweb-pdf'", "DEFAULT 'report'"} {
		if !strings.Contains(sqlByName["ir_act_report_xml"], token) && !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("ir_act_report_xml default missing %s", token)
		}
	}
	if !strings.Contains(sqlByName["ir_act_report_xml"], "paperformat_id") {
		t.Fatalf("ir_act_report_xml missing paperformat_id: %s", sqlByName["ir_act_report_xml"])
	}
	for _, token := range []string{"embedded_action_id", "ir_actions", "ir_actions_server_history", "server_action_history_wizard", "ir_embedded_actions", "report_paperformat", `"default"`} {
		if !strings.Contains(sqlByName["base_action_metadata_parity"], token) {
			t.Fatalf("base_action_metadata_parity missing %s: %s", token, sqlByName["base_action_metadata_parity"])
		}
	}
	for _, token := range []string{
		"ir_actions_actions",
		"ir_actions_act_window",
		"ir_actions_act_window_view",
		"ir_actions_act_url",
		"ir_actions_server",
		"ir_actions_client",
		"ir_actions_report",
		"ir_actions_act_window_close",
		"ir_act_window",
		"ir_act_window_view",
		"ir_act_url",
		"ir_act_server",
		"ir_act_client",
		"ir_act_report_xml",
		"ON CONFLICT (id)",
	} {
		if !strings.Contains(sqlByName["ir_actions_global_table_parity"], token) {
			t.Fatalf("ir_actions_global_table_parity missing %s: %s", token, sqlByName["ir_actions_global_table_parity"])
		}
	}
	for _, column := range []string{"active", "web_icon_data", "web_icon_data_mimetype"} {
		if !strings.Contains(sqlByName["ir_ui_menu"], column) {
			t.Fatalf("ir_ui_menu missing %s: %s", column, sqlByName["ir_ui_menu"])
		}
	}
	for _, column := range []string{"res_field", "company_id", "file_size", "checksum", "has_thumbnail"} {
		if !strings.Contains(sqlByName["ir_attachment"], column) {
			t.Fatalf("ir_attachment missing %s: %s", column, sqlByName["ir_attachment"])
		}
	}
	if !strings.Contains(sqlByName["ir_attachment_res_field"], "res_field") {
		t.Fatalf("ir_attachment_res_field missing res_field")
	}
	if !strings.Contains(sqlByName["ir_attachment_file_size"], "file_size") {
		t.Fatalf("ir_attachment_file_size missing file_size")
	}
	for _, token := range []string{"company_id", "restrictive_audit_trail", "ubl_cii_xml_file"} {
		if !strings.Contains(sqlByName["account_restrictive_audit_attachment_fields"], token) {
			t.Fatalf("account_restrictive_audit_attachment_fields missing %s", token)
		}
	}
	if !strings.Contains(sqlByName["account_move"], "ubl_cii_xml_file") {
		t.Fatalf("account_move missing ubl_cii_xml_file")
	}
	if !strings.Contains(sqlByName["res_company"], "restrictive_audit_trail") {
		t.Fatalf("res_company missing restrictive_audit_trail")
	}
	for _, column := range []string{"fiscalyear_lock_date", "tax_lock_date", "sale_lock_date", "purchase_lock_date", "hard_lock_date", "user_fiscalyear_lock_date", "user_tax_lock_date", "user_sale_lock_date", "user_purchase_lock_date", "user_hard_lock_date"} {
		if !strings.Contains(sqlByName["res_company"], column) || !strings.Contains(sqlByName["res_company_account_lock_fields"], column) {
			t.Fatalf("res_company lock field missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["action_menu_payload_fields"], "search_view_id") || !strings.Contains(sqlByName["action_menu_payload_fields"], "web_icon_data") {
		t.Fatalf("action_menu_payload_fields migration incomplete: %s", sqlByName["action_menu_payload_fields"])
	}
}

func TestApprovalConfigMigrationExposesProgressionFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"model_id", "setting_id", "settings_id", "state", "sequence", "group_ids", "user_python_code", "condition", "auto_approve", "user_ids", "committee", "is_voting"} {
		if !strings.Contains(sqlByName["approval_config"], column) {
			t.Fatalf("approval_config missing %s: %s", column, sqlByName["approval_config"])
		}
	}
}

func TestResourceCalendarMigrationExposesOdooResourceModels(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	sql := sqlByName["resource_calendar_models"]
	for _, table := range []string{"resource_calendar", "resource_calendar_attendance", "resource_calendar_leaves", "resource_resource"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("resource calendar migration missing table %s: %s", table, sql)
		}
	}
	for _, column := range []string{"two_weeks_calendar", "tz", "dayofweek", "hour_from", "hour_to", "day_period", "week_type", "display_type", "date_from", "date_to", "resource_type", "time_efficiency"} {
		if !strings.Contains(sql, column) {
			t.Fatalf("resource calendar migration missing column %s: %s", column, sql)
		}
	}
}

func TestApprovalLogMigrationExposesDurationHours(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	if !strings.Contains(sqlByName["approval_log"], "duration_hours") {
		t.Fatalf("approval_log missing duration_hours: %s", sqlByName["approval_log"])
	}
	if !strings.Contains(sqlByName["approval_log"], "duration_seconds DOUBLE PRECISION") {
		t.Fatalf("approval_log duration_seconds should be float: %s", sqlByName["approval_log"])
	}
	if !strings.Contains(sqlByName["approval_log_duration_fields"], "duration_hours") || !strings.Contains(sqlByName["approval_log_duration_fields"], "duration_seconds TYPE DOUBLE PRECISION") {
		t.Fatalf("approval_log_duration_fields migration incomplete: %s", sqlByName["approval_log_duration_fields"])
	}
}

func TestWorkflowNodeMigrationExposesResponsiblePythonCode(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	if !strings.Contains(sqlByName["workflow_node"], "responsible_python_code") {
		t.Fatalf("workflow_node missing responsible_python_code: %s", sqlByName["workflow_node"])
	}
	if !strings.Contains(sqlByName["workflow_node"], "responsible_value") || !strings.Contains(sqlByName["workflow_node"], "responsible_filter") {
		t.Fatalf("workflow_node missing responsible value/filter: %s", sqlByName["workflow_node"])
	}
	if !strings.Contains(sqlByName["workflow_node"], "schedule_activity_field_id") {
		t.Fatalf("workflow_node missing schedule_activity_field_id: %s", sqlByName["workflow_node"])
	}
	if !strings.Contains(sqlByName["workflow_node_responsible_python_code"], "responsible_python_code") {
		t.Fatalf("workflow_node_responsible_python_code migration incomplete: %s", sqlByName["workflow_node_responsible_python_code"])
	}
	if !strings.Contains(sqlByName["workflow_node_schedule_activity_field"], "schedule_activity_field_id") {
		t.Fatalf("workflow_node_schedule_activity_field migration incomplete: %s", sqlByName["workflow_node_schedule_activity_field"])
	}
	if !strings.Contains(sqlByName["workflow_node_responsible_value_filter"], "responsible_value") || !strings.Contains(sqlByName["workflow_node_responsible_value_filter"], "responsible_filter") {
		t.Fatalf("workflow_node_responsible_value_filter migration incomplete: %s", sqlByName["workflow_node_responsible_value_filter"])
	}
	for _, column := range []string{"model_id", "responsible_committee", "responsible_committee_limit", "schedule_activity_enabled", "button_context", "button_icon", "button_validate_form", "wizard_view_id", "trg_date_calendar_id"} {
		if !strings.Contains(sqlByName["workflow_node"], column) {
			t.Fatalf("workflow_node missing advanced metadata column %s: %s", column, sqlByName["workflow_node"])
		}
		if !strings.Contains(sqlByName["workflow_node_advanced_metadata_fields"], column) {
			t.Fatalf("workflow_node_advanced_metadata_fields missing %s: %s", column, sqlByName["workflow_node_advanced_metadata_fields"])
		}
	}
	if !strings.Contains(sqlByName["mail_activity_hide_in_chatter"], "hide_in_chatter") {
		t.Fatalf("mail_activity_hide_in_chatter migration incomplete: %s", sqlByName["mail_activity_hide_in_chatter"])
	}
	for _, column := range []string{"email_wizard_form_id", "email_next_action"} {
		if !strings.Contains(sqlByName["approval_buttons"], column) || !strings.Contains(sqlByName["approval_buttons_email_compose_fields"], column) {
			t.Fatalf("approval button compose column %s missing: create=%s alter=%s", column, sqlByName["approval_buttons"], sqlByName["approval_buttons_email_compose_fields"])
		}
	}
}

func TestAccountingMigrationsExposeCanonicalFields(t *testing.T) {
	sqlByName := map[string]string{}
	for _, migration := range BaseMigrations {
		sqlByName[migration.Name] = migration.SQL
	}
	for _, column := range []string{"account_type", "include_initial_balance", "internal_group", "company_id", "currency_id", "reconcile", "deprecated", "non_trade"} {
		if !strings.Contains(sqlByName["account_account"], column) {
			t.Fatalf("account_account missing %s: %s", column, sqlByName["account_account"])
		}
	}
	for _, column := range []string{"description", "invoice_label", "children_tax_ids"} {
		if !strings.Contains(sqlByName["account_tax"], column) || !strings.Contains(sqlByName["account_chart_template_fields"], column) {
			t.Fatalf("account_tax chart field %s missing: create=%s alter=%s", column, sqlByName["account_tax"], sqlByName["account_chart_template_fields"])
		}
	}
	for _, column := range []string{"country_id", "tax_payable_account_id", "tax_receivable_account_id"} {
		if !strings.Contains(sqlByName["account_tax_group"], column) || !strings.Contains(sqlByName["account_chart_template_fields"], column) {
			t.Fatalf("account_tax_group chart field %s missing: create=%s alter=%s", column, sqlByName["account_tax_group"], sqlByName["account_chart_template_fields"])
		}
	}
	if !strings.Contains(sqlByName["account_tax_repartition_line"], "tag_ids") || !strings.Contains(sqlByName["account_chart_template_fields"], "tag_ids") {
		t.Fatalf("account_tax_repartition_line missing tag_ids")
	}
	if !strings.Contains(sqlByName["account_fiscal_position"], "sequence") || !strings.Contains(sqlByName["account_chart_template_fields"], "sequence") {
		t.Fatalf("account_fiscal_position missing sequence")
	}
	for _, column := range []string{"line_ids", "column_ids"} {
		if !strings.Contains(sqlByName["account_report"], column) {
			t.Fatalf("account_report missing %s: %s", column, sqlByName["account_report"])
		}
	}
	for _, column := range []string{"children_ids", "expression_ids"} {
		if !strings.Contains(sqlByName["account_report_line"], column) {
			t.Fatalf("account_report_line missing %s: %s", column, sqlByName["account_report_line"])
		}
	}
	if !strings.Contains(sqlByName["approval_automation"], "code TEXT") {
		t.Fatalf("approval_automation migration missing code column")
	}
	if !strings.Contains(sqlByName["approval_automation_code"], "approval_automation ADD COLUMN IF NOT EXISTS code TEXT") {
		t.Fatalf("approval automation code migration missing alter column")
	}
	for _, column := range []string{"default_account_id", "active", "sequence", "restrict_mode_hash_table", "sequence_number_next"} {
		if !strings.Contains(sqlByName["account_journal"], column) {
			t.Fatalf("account_journal missing %s: %s", column, sqlByName["account_journal"])
		}
	}
	for _, column := range []string{"placeholder_code", "root_id", "group_id", "tax_ids"} {
		if !strings.Contains(sqlByName["account_account"], column) {
			t.Fatalf("account_account missing %s: %s", column, sqlByName["account_account"])
		}
		if column == "tax_ids" {
			continue
		}
		if !strings.Contains(sqlByName["account_account_group_root_fields"], column) {
			t.Fatalf("account_account_group_root_fields missing %s: %s", column, sqlByName["account_account_group_root_fields"])
		}
	}
	if !strings.Contains(sqlByName["account_account_tax_ids"], "tax_ids") {
		t.Fatalf("account_account_tax_ids missing tax_ids: %s", sqlByName["account_account_tax_ids"])
	}
	for _, column := range []string{"tax_ids"} {
		if !strings.Contains(sqlByName["account_fiscal_position"], column) || !strings.Contains(sqlByName["account_fiscal_position_tax_mapping_fields"], column) {
			t.Fatalf("account fiscal position tax mapping missing %s", column)
		}
	}
	for _, column := range []string{"fiscal_position_ids", "original_tax_ids", "is_domestic"} {
		if !strings.Contains(sqlByName["account_tax"], column) || !strings.Contains(sqlByName["account_fiscal_position_tax_mapping_fields"], column) {
			t.Fatalf("account tax mapping missing %s", column)
		}
	}
	if !strings.Contains(sqlByName["account_account"], "root_id TEXT") || !strings.Contains(sqlByName["account_structural_metadata_parity"], "id TEXT PRIMARY KEY") {
		t.Fatalf("account root schema must use text ids: account=%s root=%s", sqlByName["account_account"], sqlByName["account_structural_metadata_parity"])
	}
	for _, column := range []string{"invoice_date", "invoice_date_due", "amount_residual_signed", "payment_state", "status_in_payment", "is_move_sent", "sending_data", "is_being_sent", "invoice_pdf_report_id", "invoice_pdf_report_file", "message_main_attachment_id", "access_url", "access_token", "access_warning", "origin_payment_id", "statement_line_id", "matched_payment_ids", "reconciled_payment_ids", "payment_count", "auto_post", "need_cancel_request", "show_reset_to_draft_button", "reversed_entry_id"} {
		if !strings.Contains(sqlByName["account_move"], column) {
			t.Fatalf("account_move missing %s: %s", column, sqlByName["account_move"])
		}
	}
	for _, column := range []string{"fiscal_position_id"} {
		if !strings.Contains(sqlByName["account_move"], column) || !strings.Contains(sqlByName["account_move_fiscal_tax_fields"], column) {
			t.Fatalf("account_move fiscal migration missing %s", column)
		}
	}
	for _, column := range []string{"move_id", "account_id", "account_type", "account_internal_group", "parent_state", "journal_id", "quantity", "price_unit", "price_subtotal", "price_total", "discount", "display_type", "date_maturity", "product_id", "product_uom_id", "allowed_uom_ids", "product_category_id", "analytic_line_ids", "analytic_distribution", "distribution_analytic_account_ids", "analytic_precision", "has_invalid_analytics", "amount_residual", "amount_residual_currency", "reconciled", "payment_id", "full_reconcile_id", "matched_debit_ids", "matched_credit_ids", "tax_ids", "tax_repartition_line_id"} {
		if !strings.Contains(sqlByName["account_move_line"], column) {
			t.Fatalf("account_move_line missing %s: %s", column, sqlByName["account_move_line"])
		}
	}
	if !strings.Contains(sqlByName["account_move_fiscal_tax_fields"], "tax_ids") {
		t.Fatalf("account_move_fiscal_tax_fields missing tax_ids: %s", sqlByName["account_move_fiscal_tax_fields"])
	}
	for _, column := range []string{"move_ids", "new_move_ids", "date", "reason", "journal_id", "company_id", "available_journal_ids", "country_code", "residual", "currency_id", "move_type"} {
		if !strings.Contains(sqlByName["account_move_reversal"], column) {
			t.Fatalf("account_move_reversal missing %s: %s", column, sqlByName["account_move_reversal"])
		}
	}
	for _, column := range []string{"payment_date", "amount", "communication", "group_payment", "currency_id", "journal_id", "available_journal_ids", "line_ids", "payment_type", "partner_type", "source_amount", "source_amount_currency", "company_id", "partner_id", "payment_method_line_id", "payment_difference_handling", "writeoff_label", "total_payments_amount"} {
		if !strings.Contains(sqlByName["account_payment_register"], column) {
			t.Fatalf("account_payment_register missing %s: %s", column, sqlByName["account_payment_register"])
		}
	}
	for _, column := range []string{"move_id", "sending_methods", "sending_method_checkboxes", "mail_partner_ids", "subject", "body", "render_model"} {
		if !strings.Contains(sqlByName["account_move_send_wizard"], column) {
			t.Fatalf("account_move_send_wizard missing %s: %s", column, sqlByName["account_move_send_wizard"])
		}
	}
	for _, column := range []string{"move_ids", "summary_data", "alerts"} {
		if !strings.Contains(sqlByName["account_move_send_batch_wizard"], column) {
			t.Fatalf("account_move_send_batch_wizard missing %s: %s", column, sqlByName["account_move_send_batch_wizard"])
		}
	}
	for _, column := range []string{"debit_amount_currency", "credit_amount_currency", "full_reconcile_id"} {
		if !strings.Contains(sqlByName["account_partial_reconcile"], column) {
			t.Fatalf("account_partial_reconcile missing %s: %s", column, sqlByName["account_partial_reconcile"])
		}
	}
	for _, tt := range []struct {
		table  string
		column string
	}{
		{"account_bank_statement_line", "company_id"},
		{"account_payment", "company_id"},
		{"account_payment", "move_id"},
		{"account_payment", "is_reconciled"},
		{"account_payment", "is_matched"},
		{"account_payment", "is_sent"},
		{"account_payment", "invoice_ids"},
		{"account_payment", "reconciled_invoice_ids"},
		{"account_payment", "reconciled_bill_ids"},
		{"account_payment", "need_cancel_request"},
		{"account_payment_term", "company_id"},
		{"account_tax_repartition_line", "company_id"},
		{"account_reconcile_model_line", "company_id"},
		{"account_report_external_value", "company_id"},
	} {
		if !strings.Contains(sqlByName[tt.table], tt.column) {
			t.Fatalf("%s missing %s: %s", tt.table, tt.column, sqlByName[tt.table])
		}
	}
	for _, table := range []string{
		"account_group",
		"account_root",
		"account_journal_group",
		"account_incoterms",
		"account_lock_exception",
		"account_fiscal_position_account",
		"account_invoice_report",
		"account_automatic_entry_wizard",
		"account_autopost_bills_wizard",
		"account_resequence_wizard",
		"account_secure_entries_wizard",
		"account_merge_wizard",
		"account_merge_wizard_line",
		"account_accrued_orders_wizard",
	} {
		if !strings.Contains(sqlByName["account_structural_metadata_parity"], table) {
			t.Fatalf("account_structural_metadata_parity missing %s: %s", table, sqlByName["account_structural_metadata_parity"])
		}
	}
	for _, tt := range []struct {
		table  string
		column string
	}{
		{"account_group", "code_prefix_start"},
		{"account_group", "code_prefix_end"},
		{"account_lock_exception", "purchase_lock_date"},
		{"account_fiscal_position", "account_ids"},
		{"account_invoice_report", "price_total_currency"},
		{"account_invoice_report", "inventory_value"},
		{"account_automatic_entry_wizard", "destination_account_id"},
		{"account_secure_entries_wizard", "move_to_hash_ids"},
		{"account_merge_wizard_line", "account_has_hashed_entries"},
		{"account_accrued_orders_wizard", "preview_data"},
	} {
		if !strings.Contains(sqlByName[tt.table], tt.column) && !strings.Contains(sqlByName["account_structural_metadata_parity"], tt.column) {
			t.Fatalf("account structural migration missing %s.%s", tt.table, tt.column)
		}
	}
	for _, table := range []string{
		"res_config_settings",
		"uom_uom",
		"product_template",
		"product_product",
		"product_category",
		"account_analytic_plan",
		"account_analytic_account",
		"account_analytic_line",
		"account_analytic_distribution_model",
		"digest_digest",
		"digest_tip",
		"onboarding_onboarding",
		"onboarding_onboarding_step",
		"onboarding_progress",
		"portal_wizard",
		"portal_wizard_user",
	} {
		if !strings.Contains(sqlByName["account_dependency_anchor_tables"], table) {
			t.Fatalf("anchor migration missing %s: %s", table, sqlByName["account_dependency_anchor_tables"])
		}
	}
	for _, column := range []string{"product_id", "product_uom_id", "analytic_distribution", "has_invalid_analytics", "access_token"} {
		if !strings.Contains(sqlByName["account_dependency_anchor_fields"], column) {
			t.Fatalf("anchor field migration missing %s: %s", column, sqlByName["account_dependency_anchor_fields"])
		}
	}
}
