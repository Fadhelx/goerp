package base

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != "base" || !manifest.Installable {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestPartnerNameDefaultExportCompatible(t *testing.T) {
	for _, m := range Models() {
		if m.Name != "res.partner" {
			continue
		}
		if !m.Fields["name"].DefaultExport {
			t.Fatalf("res.partner.name default export flag = false")
		}
		return
	}
	t.Fatalf("res.partner model not found")
}

func TestManifestDataFilesLoad(t *testing.T) {
	manifest := Manifest()
	env := testDataEnv(t)
	externalIDs := map[string]data.ExternalID{}
	loader := data.NewLoaderWithExternalIDs(env, manifest.TechnicalName, externalIDs)
	baseDir := packageDir(t)
	loader.SetBaseDir(baseDir)
	if err := data.LoadModelMetadata(env, manifest.TechnicalName, Models(), externalIDs); err != nil {
		t.Fatal(err)
	}

	for _, name := range manifest.Data {
		file, err := os.Open(filepath.Join(baseDir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		switch filepath.Ext(name) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			err = loader.LoadCSV(strings.TrimSuffix(filepath.Base(name), ".csv"), file)
		default:
			err = nil
		}
		if err != nil {
			_ = file.Close()
			t.Fatalf("load %s: %v", name, err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
	}

	ids := loader.ExternalIDs()
	for _, name := range []string{
		"base.main_company",
		"base.USD",
		"base.EUR",
		"base.BHD",
		"base.lang_en",
		"base.lang_ar",
		"base.us",
		"base.bh",
		"base.state_us_1",
		"base.state_ca_ab",
		"base.europe",
		"base.gulf_cooperation_council",
		"base.module_category_hidden",
		"base.module_category_user_type",
		"base.module_category_master_data",
		"base.res_bank_1",
		"base.partner_root",
		"base.partner_admin",
		"base.public_partner",
		"base.res_partner_industry_A",
		"base.res_partner_industry_U",
		"base.user_admin_settings",
		"base.group_user",
		"base.group_system",
		"mass_mailing.res_groups_privilege_email_marketing",
		"mass_mailing.group_mass_mailing_user",
		"base.group_erp_manager",
		"base.group_sanitize_override",
		"base.group_allow_export",
		"base.group_partner_manager",
		"base.res_groups_privilege_export",
		"base.res_groups_privilege_contact",
		"base.template_portal_user_id",
		"base.default_template_user_config",
		"base.config_mass_mailing_reports",
		"base.res_partner_rule",
		"base.access_res_partner_public",
		"base.access_res_partner_portal",
		"base.access_res_users_public",
		"base.access_res_users_portal",
		"base.access_res_users_apikeys_description_portal",
		"base.api_key_user",
		"base.user_device",
		"base.properties_base_definition_rule_admin",
		"base.res_config_settings_view_form",
		"base.action_currency_form",
		"base.view_ir_actions_server_list",
		"base.view_ir_actions_server_form",
		"base.view_ir_actions_server_search",
		"base.view_ir_cron_list",
		"base.view_ir_cron_form",
		"base.view_base_automation_list",
		"base.view_base_automation_form",
		"base.view_ir_ui_view_list",
		"base.view_ir_ui_view_form",
		"base.view_ir_ui_menu_list",
		"base.view_ir_ui_menu_form",
		"base.view_ir_model_list",
		"base.view_ir_model_form",
		"base.view_ir_model_fields_list",
		"base.view_ir_model_fields_form",
		"base.view_ir_model_access_list",
		"base.view_ir_model_access_form",
		"base.view_ir_rule_list",
		"base.view_ir_rule_form",
		"base.view_mail_template_list",
		"base.view_mail_template_form",
		"base.view_ir_mail_server_list",
		"base.view_ir_mail_server_form",
		"base.view_fetchmail_server_list",
		"base.view_fetchmail_server_form",
		"base.view_mail_mail_list",
		"base.view_mail_mail_form",
		"base.view_mail_message_list",
		"base.view_mail_message_form",
		"base.action_res_config_settings",
		"base.action_res_users",
		"base.action_res_groups",
		"base.action_res_company",
		"base.action_ir_actions_server",
		"base.action_ir_cron",
		"base.action_base_automation",
		"base.action_ir_ui_view",
		"base.action_ir_ui_menu",
		"base.action_ir_model",
		"base.action_ir_model_fields",
		"base.action_ir_model_access",
		"base.action_ir_rule",
		"base.action_ir_module_module",
		"base.action_mail_template",
		"base.action_ir_mail_server",
		"base.action_fetchmail_server",
		"base.action_mail_mail",
		"base.action_mail_message",
		"base.access_base_automation_group_system",
		"base.access_mail_template_group_system",
		"base.access_fetchmail_server_group_system",
		"base.access_mail_mail_group_system",
		"base.access_mail_message_group_system",
		"base.menu_administration",
		"base.menu_users",
		"base.menu_users_users",
		"base.menu_users_groups",
		"base.menu_users_companies",
		"base.menu_technical",
		"base.menu_technical_actions",
		"base.menu_ir_actions_server",
		"base.menu_ir_cron",
		"base.menu_base_automation",
		"base.menu_technical_user_interface",
		"base.menu_ir_ui_view",
		"base.menu_ir_ui_menu",
		"base.menu_technical_database_structure",
		"base.menu_ir_model",
		"base.menu_ir_model_fields",
		"base.menu_technical_security",
		"base.menu_ir_model_access",
		"base.menu_ir_rule",
		"base.menu_technical_email",
		"base.menu_mail_template",
		"base.menu_ir_mail_server",
		"base.menu_fetchmail_server",
		"base.menu_mail_mail",
		"base.menu_mail_message",
		"base.menu_ir_module_module",
		"web.action_base_document_layout_configurator",
		"mail.email_compose_message_wizard_form",
		"mail.action_email_compose_message_wizard",
		"mail.mt_note",
		"mail.mt_comment",
		"mail.mt_activities",
		"base.autovacuum_job",
		"mail.ir_cron_mail_scheduler_action",
		"mass_mailing.ir_cron_mass_mailing_queue",
		"base.paperformat_euro",
		"base.paperformat_us",
		"base.paperformat_batch_deposit",
		"base.user_root",
		"base.user_admin",
		"base.public_user",
	} {
		if ids[name].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", name, ids)
		}
	}
	assertRecordCount(t, env, "res.lang", 93)
	assertRecordCount(t, env, "res.currency", 170)
	assertRecordCount(t, env, "res.country", 251)
	assertRecordCount(t, env, "res.country.state", 2003)
	assertRecordCount(t, env, "res.country.group", 8)
	assertRecordCount(t, env, "res.bank", 1)
	assertRecordCount(t, env, "res.partner", 4)
	assertRecordCount(t, env, "res.partner.industry", 21)
	assertRecordCount(t, env, "res.groups", 13)
	assertRecordCount(t, env, "res.groups.privilege", 3)
	assertRecordCount(t, env, "ir.model.access", 151)
	assertRecordCount(t, env, "ir.rule", 32)
	assertField(t, env, "res.company", ids["base.main_company"].ResID, "currency_id", ids["base.USD"].ResID)
	assertField(t, env, "res.company", ids["base.main_company"].ResID, "partner_id", ids["base.main_partner"].ResID)
	assertField(t, env, "res.currency", ids["base.USD"].ResID, "active", true)
	assertField(t, env, "res.country.state", ids["base.state_us_1"].ResID, "country_id", ids["base.us"].ResID)
	assertField(t, env, "res.country.state", ids["base.state_ca_ab"].ResID, "country_id", ids["base.ca"].ResID)
	assertField(t, env, "ir.module.category", ids["base.module_category_master_data"].ResID, "visible", true)
	assertField(t, env, "ir.module.category", ids["base.module_category_master_data"].ResID, "exclusive", false)
	assertField(t, env, "res.partner", ids["base.main_partner"].ResID, "is_company", true)
	assertField(t, env, "res.partner", ids["base.main_partner"].ResID, "commercial_partner_id", ids["base.main_partner"].ResID)
	assertField(t, env, "res.partner", ids["base.main_partner"].ResID, "partner_share", false)
	assertField(t, env, "res.partner", ids["base.public_partner"].ResID, "commercial_partner_id", ids["base.public_partner"].ResID)
	assertField(t, env, "res.partner", ids["base.public_partner"].ResID, "partner_share", true)
	assertField(t, env, "res.users", ids["base.user_root"].ResID, "email", "odoobot@example.com")
	assertField(t, env, "res.users", ids["base.user_admin"].ResID, "partner_id", ids["base.main_partner"].ResID)
	assertField(t, env, "res.users", ids["base.user_admin"].ResID, "commercial_partner_id", ids["base.main_partner"].ResID)
	assertField(t, env, "res.users", ids["base.user_admin"].ResID, "share", false)
	assertField(t, env, "res.users", ids["base.user_admin"].ResID, "active_partner", true)
	assertField(t, env, "res.users", ids["base.public_user"].ResID, "commercial_partner_id", ids["base.public_partner"].ResID)
	assertField(t, env, "res.users", ids["base.public_user"].ResID, "share", true)
	assertField(t, env, "res.users", ids["base.public_user"].ResID, "active_partner", false)
	assertField(t, env, "res.groups", ids["base.group_user"].ResID, "name", "Role / User")
	assertField(t, env, "res.groups", ids["base.group_user"].ResID, "api_key_duration", 90.0)
	assertField(t, env, "res.groups", ids["base.group_allow_export"].ResID, "privilege_id", ids["base.res_groups_privilege_export"].ResID)
	assertField(t, env, "res.groups", ids["base.group_allow_export"].ResID, "full_name", "Export / Allowed")
	assertField(t, env, "res.groups", ids["mass_mailing.group_mass_mailing_user"].ResID, "privilege_id", ids["mass_mailing.res_groups_privilege_email_marketing"].ResID)
	assertField(t, env, "res.groups.privilege", ids["base.res_groups_privilege_export"].ResID, "placeholder", "No")
	systemRows, err := env.Model("res.groups").Browse(ids["base.group_system"].ResID).Read("implied_ids", "user_ids", "comment")
	if err != nil {
		t.Fatal(err)
	}
	systemGroup := systemRows[0]
	for _, id := range []int64{ids["base.group_erp_manager"].ResID, ids["base.group_sanitize_override"].ResID, ids["base.group_no_one"].ResID, ids["base.group_allow_export"].ResID, ids["base.group_partner_manager"].ResID} {
		if !containsInt64(systemGroup["implied_ids"], id) {
			t.Fatalf("group_system implied_ids missing %d: %+v", id, systemGroup)
		}
	}
	for _, id := range []int64{ids["base.user_root"].ResID, ids["base.user_admin"].ResID} {
		if !containsInt64(systemGroup["user_ids"], id) {
			t.Fatalf("group_system user_ids missing %d: %+v", id, systemGroup)
		}
	}
	if !strings.Contains(systemGroup["comment"].(string), "settings") {
		t.Fatalf("group_system comment = %+v", systemGroup)
	}
	composeViewRows, err := env.Model("ir.ui.view").Browse(ids["mail.email_compose_message_wizard_form"].ResID).Read("name", "model", "type", "mode", "arch")
	if err != nil {
		t.Fatal(err)
	}
	if composeViewRows[0]["name"] != "mail.compose.message.form" ||
		composeViewRows[0]["model"] != "mail.compose.message" ||
		composeViewRows[0]["type"] != "form" ||
		composeViewRows[0]["mode"] != "primary" ||
		!strings.Contains(composeViewRows[0]["arch"].(string), `name="action_send_mail"`) ||
		!strings.Contains(composeViewRows[0]["arch"].(string), `name="action_schedule_message"`) ||
		!strings.Contains(composeViewRows[0]["arch"].(string), `name="scheduled_date"`) {
		t.Fatalf("compose view = %+v", composeViewRows[0])
	}
	composeActionRows, err := env.Model("ir.actions.act_window").Browse(ids["mail.action_email_compose_message_wizard"].ResID).Read("res_model", "view_id", "target")
	if err != nil {
		t.Fatal(err)
	}
	if composeActionRows[0]["res_model"] != "mail.compose.message" ||
		composeActionRows[0]["view_id"] != ids["mail.email_compose_message_wizard_form"].ResID ||
		composeActionRows[0]["target"] != "new" {
		t.Fatalf("compose action = %+v", composeActionRows[0])
	}
	serverActionSearchRows, err := env.Model("ir.ui.view").Browse(ids["base.view_ir_actions_server_search"].ResID).Read("name", "model", "type", "arch")
	if err != nil {
		t.Fatal(err)
	}
	serverActionSearchArch := serverActionSearchRows[0]["arch"].(string)
	for _, want := range []string{
		`filter string="Active" name="active" domain="[('active','=',True)]"`,
		`filter string="Code" name="code" domain="[('state','=','code')]"`,
		`filter string="Model" name="group_model" context="{'group_by':'model_id'}"`,
		`filter string="Binding Model" name="group_binding_model" context="{'group_by':'binding_model_id'}"`,
	} {
		if !strings.Contains(serverActionSearchArch, want) {
			t.Fatalf("server action search arch missing %s: %s", want, serverActionSearchArch)
		}
	}
	if serverActionSearchRows[0]["name"] != "ir.actions.server.search" ||
		serverActionSearchRows[0]["model"] != "ir.actions.server" ||
		serverActionSearchRows[0]["type"] != "search" {
		t.Fatalf("server action search view = %+v", serverActionSearchRows[0])
	}
	serverActionRows, err := env.Model("ir.actions.act_window").Browse(ids["base.action_ir_actions_server"].ResID).Read("res_model", "view_mode", "search_view_id")
	if err != nil {
		t.Fatal(err)
	}
	if serverActionRows[0]["res_model"] != "ir.actions.server" ||
		serverActionRows[0]["view_mode"] != "list,form" ||
		serverActionRows[0]["search_view_id"] != ids["base.view_ir_actions_server_search"].ResID {
		t.Fatalf("server action window action = %+v", serverActionRows[0])
	}

	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	if len(engine.Rules) != 32 {
		t.Fatalf("persisted rules = %d, want 32", len(engine.Rules))
	}
}

func TestRegisterModels(t *testing.T) {
	reg := registry.New("test")
	if err := reg.Install([]module.Manifest{Manifest()}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for modelName, tableName := range map[string]string{
		"ir.actions.actions":          "ir_actions",
		"ir.actions.act_window":       "ir_act_window",
		"ir.actions.act_window.view":  "ir_act_window_view",
		"ir.actions.act_window_close": "ir_actions",
		"ir.actions.act_url":          "ir_act_url",
		"ir.actions.todo":             "ir_actions_todo",
		"ir.actions.server":           "ir_act_server",
		"ir.actions.client":           "ir_act_client",
		"ir.actions.report":           "ir_act_report_xml",
	} {
		if got := reg.Models[modelName].Table; got != tableName {
			t.Fatalf("%s table = %q, want %s", modelName, got, tableName)
		}
	}
	for _, name := range []string{
		"res.users",
		"res.groups",
		"ir.model",
		"ir.model.access",
		"ir.rule",
		"ir.model.constraint",
		"ir.model.relation",
		"ir.model.inherit",
		"ir.model.fields.selection",
		"ir.module.category",
		"ir.module.module.dependency",
		"ir.module.module.exclusion",
		"res.config.settings",
		"res.config",
		"ir.attachment",
		"ir.filters",
		"ir.actions.server",
		"ir.actions.actions",
		"ir.actions.act_url",
		"ir.actions.todo",
		"ir.actions.act_window.view",
		"ir.actions.act_window_close",
		"ir.actions.server.history",
		"server.action.history.wizard",
		"ir.embedded.actions",
		"report.paperformat",
		"ir.cron.trigger",
		"base.automation",
		"res.country",
		"res.country.group",
		"res.country.state",
		"res.currency",
		"res.lang",
		"res.bank",
		"res.partner.bank",
		"res.partner.category",
		"res.partner.industry",
		"res.groups.privilege",
		"res.users.settings",
		"res.users.apikeys",
		"res.users.apikeys.description",
		"res.users.apikeys.show",
		"res.users.deletion",
		"res.users.log",
		"res.users.identitycheck",
		"change.password.wizard",
		"change.password.user",
		"change.password.own",
		"res.device",
		"res.device.log",
		"res.currency.rate",
		"ir.ui.view.custom",
		"properties.base.definition",
		"decimal.precision",
		"ir.exports",
		"ir.exports.line",
		"ir.logging",
		"ir.mail_server",
		"ir.profile",
		"base.enable.profiling.wizard",
		"base.language.export",
		"base.language.import",
		"base.language.install",
		"base.module.update",
		"base.module.upgrade",
		"base.module.uninstall",
		"base.partner.merge.automatic.wizard",
		"base.partner.merge.line",
		"wizard.ir.model.menu.create",
		"reset.view.arch.wizard",
		"report.layout",
		"ir.demo",
		"ir.demo.failure",
		"ir.demo.failure.wizard",
		"mail.thread",
		"mail.message",
		"mail.message.reaction",
		"mail.mail",
		"mail.notification",
		"mail.followers",
		"mail.activity",
		"mail.composer.mixin",
		"mail.compose.message",
		"mail.scheduled.message",
		"mail.template",
		"mail.alias",
		"mail.alias.domain",
		"mail.gateway.allowed",
		"mail.blacklist",
		"phone.blacklist",
		"mailing.contact",
		"mailing.list",
		"mailing.subscription",
		"mailing.subscription.optout",
		"utm.campaign",
		"utm.source",
		"utm.medium",
		"mailing.mailing",
		"link.tracker",
		"link.tracker.code",
		"link.tracker.click",
		"sms.template",
		"sms.composer",
		"sms.sms",
		"sms.tracker",
		"mailing.trace",
		"mail.message.subtype",
		"fetchmail.server",
		"discuss.channel",
		"discuss.channel.member",
		"mail.guest",
		"mail.presence",
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
}

func TestSecurityModelsExposeOdooFields(t *testing.T) {
	models := map[string]map[string]bool{}
	for _, m := range Models() {
		fields := map[string]bool{}
		for name := range m.Fields {
			fields[name] = true
		}
		models[m.Name] = fields
	}

	assertFields(t, models["ir.model.access"], "name", "active", "model", "model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	assertFields(t, models["ir.model"], "model", "name", "abstract", "transient", "is_mail_thread", "is_mail_activity", "is_mail_blacklist")
	assertFields(t, models["ir.model.fields"], "model", "name", "ttype", "relation", "relation_field", "groups", "ai_vector_size")
	assertFields(t, models["ir.model.fields.selection"], "name", "field_id", "value", "sequence")
	assertFields(t, models["ir.model.constraint"], "name", "model", "type", "definition")
	assertFields(t, models["ir.model.relation"], "name", "model")
	assertFields(t, models["ir.model.inherit"], "name", "model_id")
	assertFields(t, models["ir.module.category"], "name", "parent_id", "child_ids", "module_ids", "privilege_ids", "description", "sequence", "visible", "exclusive", "xml_id")
	assertFields(t, models["ir.module.module.dependency"], "name", "module_id")
	assertFields(t, models["ir.module.module.exclusion"], "name", "module_id")
	assertFields(t, models["ir.rule"], "name", "model", "model_id", "domain", "domain_force", "groups", "group_ids", "global", "active", "perm_read", "perm_write", "perm_create", "perm_unlink")
	assertFields(t, models["ir.model.data"], "module", "name", "complete_name", "model", "res_id", "noupdate")
	assertFields(t, models["ir.default"], "model", "field", "value")
	assertFields(t, models["res.config.settings"], "company_id", "is_root_company", "group_multi_currency", "group_uom", "group_analytic_accounting", "digest_emails", "digest_id", "portal_allow_api_keys", "sale_tax_id", "purchase_tax_id", "chart_template", "has_chart_of_accounts", "has_accounting_entries", "module_account_accountant", "module_product_margin", "use_invoice_terms")
	assertFields(t, models["ir.cron"], "name", "ir_actions_server_id", "cron_name", "active", "interval_number", "interval_type", "nextcall", "lastcall", "model_id", "state", "code", "priority", "failure_count", "first_failure_date")
	assertFields(t, models["ir.cron.progress"], "cron_id", "done", "remaining", "deactivate", "timed_out_counter", "started_at", "updated_at")
	assertFields(t, models["res.company"], "name", "active", "parent_id", "currency_id", "country_id", "partner_id", "fiscalyear_lock_date", "tax_lock_date", "sale_lock_date", "purchase_lock_date", "hard_lock_date", "user_fiscalyear_lock_date", "user_tax_lock_date", "user_sale_lock_date", "user_purchase_lock_date", "user_hard_lock_date", "restrictive_audit_trail", "alias_domain_id")
	assertFields(t, models["res.partner"], "name", "active", "email", "email_normalized", "message_bounce", "phone", "phone_sanitized", "phone_blacklisted", "street", "city", "zip", "country_id", "state_id", "company_id", "parent_id", "commercial_partner_id", "partner_share", "is_company", "image_1920", "signup_type")
	assertFields(t, models["res.partner.industry"], "name", "full_name")
	assertFields(t, models["res.users"], "login", "password", "email", "name", "active", "active_partner", "login_date", "company_id", "company_ids", "groups_id", "group_ids", "all_group_ids", "accesses_count", "rules_count", "groups_count", "view_group_hierarchy", "role", "partner_id", "commercial_partner_id", "share", "signature", "image_1920", "notification_type", "outgoing_mail_server_id", "outgoing_mail_server_type", "has_external_mail_server")
	assertFields(t, models["res.groups"], "name", "full_name", "share", "sequence", "category_id", "privilege_id", "implied_ids", "all_implied_ids", "implied_by_ids", "all_implied_by_ids", "disjoint_ids", "user_ids", "all_user_ids", "all_users_count", "model_access", "rule_groups", "menu_access", "view_access", "comment", "api_key_duration", "view_group_hierarchy")
	assertFields(t, models["resource.calendar"], "name", "active", "company_id", "attendance_ids", "leave_ids", "global_leave_ids", "schedule_type", "duration_based", "flexible_hours", "full_time_required_hours", "hours_per_day", "hours_per_week", "two_weeks_calendar", "tz")
	assertFields(t, models["resource.calendar.attendance"], "name", "dayofweek", "hour_from", "hour_to", "duration_hours", "duration_days", "calendar_id", "duration_based", "day_period", "week_type", "two_weeks_calendar", "display_type", "sequence")
	assertFields(t, models["resource.calendar.leaves"], "name", "company_id", "calendar_id", "date_from", "date_to", "resource_id", "time_type")
	assertFields(t, models["resource.resource"], "name", "active", "company_id", "resource_type", "user_id", "time_efficiency", "calendar_id", "tz")
	assertFields(t, models["res.country"], "name", "code", "address_format", "currency_id", "phone_code", "vat_label", "state_required", "zip_required", "name_position", "state_ids", "active")
	assertFields(t, models["res.country.group"], "name", "code", "country_ids")
	assertFields(t, models["res.country.state"], "name", "code", "country_id")
	assertFields(t, models["res.currency"], "name", "symbol", "position", "rounding", "decimal_places", "iso_numeric", "full_name", "currency_unit_label", "currency_subunit_label", "active")
	assertFields(t, models["res.lang"], "name", "code", "iso_code", "url_code", "direction", "grouping", "decimal_point", "thousands_sep", "date_format", "time_format", "week_start", "flag_image", "active")
	assertFields(t, models["res.bank"], "name", "bic", "active", "country")
	assertFields(t, models["res.partner.bank"], "acc_number", "partner_id", "bank_id", "company_id", "active")
	assertFields(t, models["res.partner.category"], "name", "parent_id")
	assertFields(t, models["res.groups.privilege"], "name", "description", "placeholder", "sequence", "category_id", "group_ids")
	assertFields(t, models["res.users.settings"], "user_id", "is_discuss_sidebar_category_channel_open", "is_discuss_sidebar_category_chat_open")
	assertFields(t, models["res.users.apikeys"], "name", "user_id", "scope", "key", "create_date")
	assertFields(t, models["digest.digest"], "name", "active", "company_id", "currency_id", "periodicity", "next_run_date", "user_ids", "available_fields", "is_subscribed", "state", "kpi_res_users_connected", "kpi_res_users_connected_value", "kpi_mail_message_total", "kpi_mail_message_total_value", "kpi_account_total_revenue", "kpi_account_total_revenue_value")
	assertFields(t, models["digest.tip"], "name", "sequence", "group_id", "user_ids", "tip_description")
	assertFields(t, models["res.users.apikeys.description"], "name", "user_id")
	assertFields(t, models["res.users.apikeys.show"], "name", "user_id")
	assertFields(t, models["res.users.deletion"], "user_id")
	assertFields(t, models["res.users.log"], "create_uid")
	assertFields(t, models["res.users.identitycheck"], "create_uid", "user_id", "request")
	assertFields(t, models["change.password.wizard"], "user_ids")
	assertFields(t, models["change.password.user"], "create_uid", "user_id", "new_passwd")
	assertFields(t, models["change.password.own"], "create_uid", "user_id", "new_passwd")
	assertFields(t, models["res.device"], "name", "user_id")
	assertFields(t, models["res.device.log"], "device_id", "user_id")
	assertFields(t, models["res.currency.rate"], "name", "currency_id", "company_id", "rate")
	assertFields(t, models["ir.ui.view.custom"], "user_id", "ref_id", "arch")
	assertFields(t, models["properties.base.definition"], "name")
	assertFields(t, models["decimal.precision"], "name", "digits")
	assertFields(t, models["ir.exports"], "name", "resource", "export_fields")
	assertFields(t, models["ir.exports.line"], "name", "export_id")
	assertFields(t, models["ir.logging"], "name", "type", "level", "message")
	assertFields(t, models["ir.mail_server"], "name", "active", "smtp_host", "smtp_port", "smtp_encryption", "smtp_authentication", "from_filter", "sequence", "smtp_debug", "max_email_size", "smtp_user", "smtp_pass", "smtp_ssl_certificate", "smtp_ssl_private_key", "owner_user_id", "owner_limit_time", "owner_limit_count")
	assertFields(t, models["ir.profile"], "name", "session", "duration")
	assertFields(t, models["base.enable.profiling.wizard"], "duration")
	assertFields(t, models["base.language.export"], "name")
	assertFields(t, models["base.language.import"], "name")
	assertFields(t, models["base.language.install"], "lang")
	assertFields(t, models["base.module.update"], "updated")
	assertFields(t, models["base.module.upgrade"], "module_info")
	assertFields(t, models["base.module.uninstall"], "module_id")
	assertFields(t, models["base.partner.merge.automatic.wizard"], "partner_ids")
	assertFields(t, models["base.partner.merge.line"], "wizard_id", "partner_id")
	assertFields(t, models["wizard.ir.model.menu.create"], "model_id", "menu_name")
	assertFields(t, models["reset.view.arch.wizard"], "view_id")
	assertFields(t, models["report.layout"], "name", "view_id")
	assertFields(t, models["ir.demo"], "name")
	assertFields(t, models["ir.demo.failure"], "name")
	assertFields(t, models["ir.demo.failure.wizard"], "failure_id")
	assertFields(t, models["ir.attachment"], "name", "res_model", "res_field", "res_id", "company_id", "type", "url", "mimetype", "datas", "file_size", "public", "access_token", "checksum", "has_thumbnail")
	assertFields(t, models["ir.filters"], "name", "model_id", "domain", "context", "sort", "user_id", "action_id", "embedded_action_id", "is_default", "active")
	assertFields(t, models["ir.ui.view"], "name", "model", "type", "arch", "key", "inherit_id", "inherit_id_ref", "mode", "priority", "active", "groups_id", "primary", "customize_show", "track", "page", "website_id")
	assertFields(t, models["ir.asset"], "name", "active", "bundle", "directive", "path", "target", "sequence")
	assertFields(t, models["ir.ui.menu"], "name", "active", "parent_id", "action", "sequence", "groups_id", "web_icon", "web_icon_data", "web_icon_data_mimetype")
	assertFields(t, models["ir.actions.actions"], "name", "type", "xml_id", "help", "path", "binding_model_id", "binding_type", "binding_view_types")
	assertFields(t, models["ir.actions.act_window"], "name", "type", "res_model", "res_id", "view_mode", "views", "embedded_action_ids", "close_on_report_download", "mobile_view_mode", "view_id", "search_view_id", "domain", "context", "target", "limit", "help", "path", "usage", "filter", "cache", "group_ids", "binding_model_id", "binding_type", "binding_view_types")
	assertFields(t, models["ir.actions.act_window.view"], "sequence", "view_mode", "view_id", "act_window_id", "multi")
	assertFields(t, models["ir.actions.act_window_close"], "name", "type", "effect", "infos")
	assertFields(t, models["ir.actions.act_url"], "name", "type", "url", "target", "close", "binding_model_id", "binding_type", "binding_view_types")
	assertFields(t, models["ir.actions.todo"], "name", "action_id", "state", "sequence")
	assertFields(t, models["ir.actions.server"], "name", "type", "model_id", "binding_model_id", "binding_type", "binding_view_types", "model_name", "automated_name", "allowed_states", "available_model_ids", "state", "active", "sequence", "code", "parent_id", "child_ids", "crud_model_id", "crud_model_name", "link_field_id", "group_ids", "update_field_id", "update_path", "update_related_model_id", "update_field_type", "update_m2m_operation", "update_boolean_value", "value", "html_value", "evaluation_type", "sequence_id", "resource_ref", "selection_value", "warning", "ir_cron_ids", "show_code_history", "value_field_to_show", "template_id", "mail_post_autofollow", "mail_post_method", "followers_type", "followers_partner_field_name", "partner_ids", "activity_type_id", "activity_summary", "activity_note", "activity_date_deadline_range", "activity_date_deadline_range_type", "activity_user_type", "activity_user_id", "activity_user_field_name", "sms_template_id", "sms_method", "wa_template_id", "documents_account_create_model", "documents_account_journal_id", "documents_account_suitable_journal_ids", "documents_account_move_type", "webhook_url", "webhook_field_ids", "webhook_sample_payload", "ai_tool_ids", "ai_action_prompt", "ai_tool_description", "ai_tool_schema", "use_in_ai", "ai_tool_allow_end_message")
	assertFields(t, models["ir.actions.server.history"], "action_id", "code", "create_uid", "create_date")
	assertFields(t, models["server.action.history.wizard"], "action_id", "code_diff", "current_code", "revision")
	assertFields(t, models["ir.actions.client"], "name", "type", "tag", "res_model", "target", "context", "params", "params_store")
	assertFields(t, models["ir.actions.report"], "name", "type", "model", "model_id", "report_name", "report_type", "target", "context", "data", "close_on_report_download", "report_file", "print_report_name", "attachment", "attachment_use", "multi", "binding_model_id", "binding_type", "binding_view_types", "paperformat_id", "groups_id", "domain", "is_invoice_report")
	assertFields(t, models["ir.embedded.actions"], "name", "sequence", "parent_action_id", "parent_res_id", "parent_res_model", "action_id", "python_method", "user_id", "is_deletable", "default_view_mode", "filter_ids", "is_visible", "domain", "context", "groups_ids")
	assertFields(t, models["report.paperformat"], "name", "default", "format", "page_height", "page_width", "orientation", "margin_top", "margin_bottom", "margin_left", "margin_right", "header_line", "header_spacing", "dpi", "css_margins", "disable_shrinking")
	assertFields(t, models["mail.message"], "subject", "body", "message_type", "model", "res_id", "record_alias_domain_id", "record_company_id", "author_id", "author_guest_id", "email_from", "message_id", "incoming_email_to", "incoming_email_cc", "outgoing_email_to", "reply_to", "reply_to_force_new", "mail_server_id", "email_layout_xmlid", "email_add_signature", "date", "parent_id", "subtype_id", "mail_activity_type_id", "partner_ids", "attachment_ids", "body_is_html", "is_internal", "reaction_ids", "starred", "starred_partner_ids", "tracking_value_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mail.inbound.message.lock"], "message_id", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mail.tracking.value"], "field_id", "field_info", "field_name", "field_desc", "field_type", "old_value_integer", "old_value_float", "old_value_char", "old_value_text", "old_value_datetime", "new_value_integer", "new_value_float", "new_value_char", "new_value_text", "new_value_datetime", "currency_id", "mail_message_id")
	assertFields(t, models["mail.message.reaction"], "message_id", "content", "partner_id", "guest_id")
	assertFields(t, models["mail.mail"], "mail_message_id", "recipient_ids", "attachment_ids", "mail_server_id", "record_alias_domain_id", "record_company_id", "author_id", "email_from", "email_to", "email_cc", "reply_to", "subject", "body_html", "state", "failure_reason", "failure_type", "scheduled_date", "retry_count", "max_retries", "auto_delete", "message_id", "references", "headers", "is_notification", "fetchmail_server_id", "mailing_id", "mailing_trace_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mail.notification"], "mail_message_id", "mail_mail_id", "res_partner_id", "mail_email_address", "notification_type", "notification_status", "failure_type", "failure_reason", "is_read", "read_date", "author_id", "sms_id", "sms_id_int", "sms_tracker_ids", "sms_number", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mail.template"], "name", "model", "model_id", "subject", "body_html", "email_from", "email_to", "email_cc", "reply_to", "partner_to", "attachment_ids", "report_template_ids", "mail_server_id", "auto_delete", "scheduled_date", "active")
	assertFields(t, models["mail.alias"], "alias_name", "model_name", "alias_model_id", "alias_defaults", "alias_force_thread_id", "alias_parent_model_id", "alias_parent_thread_id", "alias_contact", "alias_incoming_local", "alias_bounced_content", "alias_status", "alias_user_id", "alias_domain_id", "alias_domain", "alias_full_name", "active")
	assertFields(t, models["mail.alias.domain"], "name", "company_ids", "sequence", "bounce_alias", "bounce_email", "catchall_alias", "catchall_email", "default_from", "default_from_email")
	assertFields(t, models["mail.gateway.allowed"], "email", "email_normalized")
	assertFields(t, models["mail.blacklist"], "email", "active", "message", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["phone.blacklist"], "number", "active", "message", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["utm.campaign"], "name", "mailing_mail_ids", "mailing_mail_count", "ab_testing_mailings_count", "ab_testing_completed", "ab_testing_winner_mailing_id", "ab_testing_schedule_datetime", "ab_testing_winner_selection")
	assertFields(t, models["utm.source"], "name")
	assertFields(t, models["utm.medium"], "name")
	assertFields(t, models["mailing.contact"], "name", "first_name", "last_name", "company_name", "email", "email_normalized", "phone", "phone_sanitized", "active", "list_ids", "subscription_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mailing.list"], "name", "active", "is_public", "contact_ids", "subscription_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mailing.subscription"], "contact_id", "list_id", "opt_out", "opt_out_reason_id", "opt_out_datetime", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mailing.subscription.optout"], "name", "sequence", "active")
	assertFields(t, models["mailing.mailing"], "name", "subject", "preview", "body_html", "email_from", "reply_to", "reply_to_mode", "mail_server_id", "attachment_ids", "keep_archives", "state", "sent_date", "schedule_type", "schedule_date", "kpi_mail_required", "user_id", "mailing_model_real", "mailing_domain", "mailing_on_mailing_list", "use_exclusion_list", "sms_allow_unsubscribe", "contact_list_ids", "campaign_id", "source_id", "medium_id", "ab_testing_enabled", "ab_testing_pc", "ab_testing_is_winner_mailing", "ab_testing_mailings_count", "ab_testing_completed", "ab_testing_schedule_datetime", "ab_testing_winner_selection", "is_ab_test_sent", "total", "scheduled", "expected", "canceled", "sent", "process", "pending", "delivered", "opened", "clicked", "replied", "bounced", "failed", "received_ratio", "opened_ratio", "replied_ratio", "bounced_ratio", "clicks_ratio", "link_trackers_count", "mailing_trace_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mailing.mailing.test"], "email_to", "mass_mailing_id")
	assertFields(t, models["mailing.mailing.schedule.date"], "schedule_date", "mass_mailing_id")
	assertFields(t, models["link.tracker"], "url", "absolute_url", "short_url", "redirected_url", "short_url_host", "title", "label", "link_code_ids", "code", "link_click_ids", "count", "campaign_id", "medium_id", "source_id", "mass_mailing_id", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["link.tracker.code"], "code", "link_id")
	assertFields(t, models["link.tracker.click"], "campaign_id", "link_id", "ip", "country_id", "mailing_trace_id", "mass_mailing_id", "whatsapp_message_id", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["sms.template"], "name", "model_id", "model", "body", "sidebar_action_id", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["sms.composer"], "composition_mode", "res_model", "res_model_description", "res_id", "res_ids", "res_ids_count", "comment_single_recipient", "mass_keep_log", "mass_force_send", "use_exclusion_list", "recipient_valid_count", "recipient_invalid_count", "recipient_single_description", "recipient_single_number", "recipient_single_number_itf", "recipient_single_valid", "number_field_name", "numbers", "sanitized_numbers", "template_id", "body", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["sms.sms"], "uuid", "number", "body", "partner_id", "mail_message_id", "state", "failure_type", "to_delete", "mailing_id", "mailing_trace_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["sms.tracker"], "sms_uuid", "mail_notification_id", "mailing_trace_id", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mailing.trace"], "trace_type", "is_test_trace", "mail_mail_id", "mail_mail_id_int", "email", "message_id", "medium_id", "source_id", "model", "res_id", "mass_mailing_id", "campaign_id", "sent_datetime", "open_datetime", "reply_datetime", "trace_status", "failure_type", "failure_reason", "links_click_datetime", "sms_id", "sms_id_int", "sms_tracker_ids", "sms_number", "sms_code", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["marketing.campaign"], "name", "utm_campaign_id", "state", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["marketing.activity"], "name", "activity_type", "campaign_id", "whatsapp_template_id", "server_action_id", "interval_number", "interval_type", "trigger_type", "trigger_category", "whatsapp_error", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["whatsapp.account"], "name", "active", "app_uid", "account_uid", "phone_uid", "phone_number", "token", "app_secret", "webhook_verify_token", "callback_url", "debug_logging", "templates_count", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["whatsapp.template"], "name", "template_name", "active", "wa_template_uid", "wa_account_id", "model_id", "model", "phone_field", "lang_code", "template_type", "status", "quality", "header_type", "header_text", "header_attachment_ids", "footer_text", "body", "variable_ids", "button_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["whatsapp.template.button"], "name", "text", "template_id", "wa_template_id", "sequence", "button_type", "url_type", "website_url", "dynamic_url", "call_number", "variable_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["whatsapp.template.variable"], "name", "button_id", "wa_template_id", "model", "line_type", "field_type", "field_name", "demo_value", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["whatsapp.message"], "mobile_number", "mobile_number_formatted", "message_type", "state", "failure_type", "failure_reason", "template_id", "wa_template_id", "msg_uid", "wa_account_id", "parent_id", "mail_message_id", "model", "res_id", "body", "components", "free_text_json", "links_click_datetime", "marketing_trace_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["marketing.trace"], "activity_id", "whatsapp_message_id", "links_click_datetime", "state", "schedule_date", "state_msg", "parent_id", "child_ids", "create_uid", "create_date", "write_uid", "write_date")
	assertFields(t, models["mail.composer.mixin"], "subject", "body", "body_html", "attachment_ids", "partner_ids")
	assertFields(t, models["mail.compose.message"], "composition_mode", "composition_comment_option", "message_type", "subtype_is_log", "model", "res_id", "res_ids", "template_id", "subject", "body", "body_html", "email_from", "email_to", "email_cc", "reply_to", "partner_ids", "attachment_ids", "parent_id", "subtype_id", "scheduled_date", "mail_server_id", "author_id", "auto_delete", "mass_mailing_id", "use_exclusion_list", "notify", "body_is_html")
	assertFields(t, models["mail.scheduled.message"], "mail_message_id", "mail_mail_id", "mail_template_id", "model", "res_id", "scheduled_date", "author_id", "subject", "body", "partner_ids", "attachment_ids", "composition_comment_option", "is_note", "notification_parameters", "send_context", "state")
	assertFields(t, models["mail.activity"], "activity_type_id", "activity_category", "recommended_activity_type_id", "previous_activity_type_id", "has_recommended_activities", "chaining_type", "res_model", "res_id", "user_id", "date_deadline", "summary", "note", "state", "automated", "hide_in_chatter", "attachment_ids", "active", "date_done", "feedback")
	assertFields(t, models["mail.activity.type"], "name", "summary", "res_model", "category", "default_note", "default_user_id", "delay_count", "delay_unit", "delay_from", "chaining_type", "triggered_next_type_id", "suggested_next_type_ids", "previous_type_ids", "mail_template_ids", "icon", "active")
	assertFields(t, models["mail.message.subtype"], "name", "description", "res_model", "default", "internal")
	assertFields(t, models["fetchmail.server"], "name", "active", "state", "server", "port", "server_type", "server_type_info", "is_ssl", "attach", "original", "date", "error_date", "error_message", "user", "password", "object_id", "priority", "message_ids", "configuration", "script")
	assertFields(t, models["discuss.channel"], "name", "channel_type", "active", "group_public_id", "ai_env_context", "ai_agent_id")
	assertFields(t, models["discuss.channel.member"], "channel_id", "partner_id", "user_id", "guest_id", "last_seen_dt")
	assertFields(t, models["mail.guest"], "name", "email", "access_token", "country_id", "lang", "timezone")
	assertFields(t, models["mail.presence"], "user_id", "guest_id", "status", "last_poll")
}

func TestWhatsAppTrackedLinkSourceFieldTypes(t *testing.T) {
	models := map[string]map[string]field.Field{}
	for _, m := range Models() {
		models[m.Name] = m.Fields
	}
	specs := []struct {
		model         string
		name          string
		kind          field.Kind
		relation      string
		relationField string
	}{
		{model: "link.tracker.click", name: "whatsapp_message_id", kind: field.Many2One, relation: "whatsapp.message"},
		{model: "whatsapp.message", name: "links_click_datetime", kind: field.DateTime},
		{model: "whatsapp.message", name: "marketing_trace_ids", kind: field.One2Many, relation: "marketing.trace", relationField: "whatsapp_message_id"},
		{model: "marketing.trace", name: "whatsapp_message_id", kind: field.Many2One, relation: "whatsapp.message"},
		{model: "marketing.trace", name: "links_click_datetime", kind: field.DateTime},
		{model: "marketing.activity", name: "interval_number", kind: field.Int},
		{model: "marketing.activity", name: "interval_type", kind: field.Selection},
		{model: "marketing.activity", name: "trigger_type", kind: field.Selection},
		{model: "marketing.activity", name: "trigger_category", kind: field.Selection},
		{model: "marketing.campaign", name: "utm_campaign_id", kind: field.Many2One, relation: "utm.campaign"},
	}
	for _, spec := range specs {
		fields, ok := models[spec.model]
		if !ok {
			t.Fatalf("missing model %s", spec.model)
		}
		got, ok := fields[spec.name]
		if !ok {
			t.Fatalf("%s missing %s", spec.model, spec.name)
		}
		if got.Kind != spec.kind || got.Relation != spec.relation || got.RelationField != spec.relationField {
			t.Fatalf("%s.%s = kind %s relation %s relation_field %s", spec.model, spec.name, got.Kind, got.Relation, got.RelationField)
		}
	}
}

func TestMailAliasModelExposesOdoo19RoutingFields(t *testing.T) {
	var aliasFields map[string]field.Field
	for _, m := range Models() {
		if m.Name == "mail.alias" {
			aliasFields = m.Fields
			break
		}
	}
	if aliasFields == nil {
		t.Fatal("mail.alias model not found")
	}
	specs := map[string]struct {
		kind     field.Kind
		relation string
	}{
		"alias_model_id":        {kind: field.Many2One, relation: "ir.model"},
		"alias_force_thread_id": {kind: field.Int},
		"alias_defaults":        {kind: field.Text},
		"alias_contact":         {kind: field.Selection},
		"alias_user_id":         {kind: field.Many2One, relation: "res.users"},
	}
	for name, spec := range specs {
		got, ok := aliasFields[name]
		if !ok {
			t.Fatalf("mail.alias missing %s", name)
		}
		if got.Kind != spec.kind || got.Relation != spec.relation {
			t.Fatalf("mail.alias.%s = kind %s relation %s", name, got.Kind, got.Relation)
		}
	}
}

func assertFields(t *testing.T, fields map[string]bool, names ...string) {
	t.Helper()
	for _, name := range names {
		if !fields[name] {
			t.Fatalf("missing field %s in %+v", name, fields)
		}
	}
}

func assertRecordCount(t *testing.T, env *record.Env, modelName string, want int) {
	t.Helper()
	countEnv := env.WithContext(record.Context{UserID: 1, CompanyID: env.Context().CompanyID, CompanyIDs: env.Context().CompanyIDs, Values: map[string]any{"active_test": false}})
	records, err := countEnv.Model(modelName).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if records.Len() != want {
		t.Fatalf("%s count = %d, want %d", modelName, records.Len(), want)
	}
}

func assertField(t *testing.T, env *record.Env, modelName string, id int64, fieldName string, want any) {
	t.Helper()
	rows, err := env.Model(modelName).Browse(id).Read(fieldName)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s(%d) rows = %+v", modelName, id, rows)
	}
	if !reflect.DeepEqual(rows[0][fieldName], want) {
		t.Fatalf("%s.%s = %#v, want %#v", modelName, fieldName, rows[0][fieldName], want)
	}
}

func containsInt64(value any, target int64) bool {
	ids, ok := value.([]int64)
	if !ok {
		return false
	}
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Dir(file)
}

func testDataEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, model := range Models() {
		if err := reg.Register(model); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{})
}
