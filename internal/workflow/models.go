package workflow

import (
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/registry"
)

const (
	ModelSettings          = "approval.settings"
	ModelSettingsState     = "approval.settings.state"
	ModelConfig            = "approval.config"
	ModelButton            = "approval.buttons"
	ModelAutomation        = "approval.automation"
	ModelEscalation        = "approval.escalation"
	ModelLog               = "approval.log"
	ModelLogVoting         = "approval.log.voting"
	ModelForward           = "approval.forward"
	ModelCancellation      = "cancellation.record"
	ModelStateTags         = "state.tags"
	ModelApprovalRecord    = "approval.record"
	ModelNameSequenceMixin = "name.sequence.mixin"
	ModelProcessWizard     = "approval.process.wizard"
	ModelStateUpdateWizard = "approval.state.update"
	ModelExpressionEditor  = "model.expression.editor"
)

func RegisterModels(reg *registry.Registry) error {
	for _, m := range Models() {
		if err := reg.RegisterModel(m); err != nil {
			return err
		}
	}
	return nil
}

func Models() []model.Model {
	return []model.Model{
		approvalRecord(),
		nameSequenceMixin(),
		simple(ModelSettings, "approval_settings",
			field.New("name", field.Char),
			field.New("model", field.Char),
			field.New("model_name", field.Char),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("sequence", field.Int),
			field.New("active", field.Bool),
			field.New("state_ids", field.One2Many).WithRelation(ModelSettingsState),
			field.New("approval_count", field.Int),
			field.New("automation_count", field.Int),
			field.New("button_count", field.Int),
			field.New("show_action_approve_all", field.Bool),
			field.New("show_status_duration_tracking", field.Bool),
			field.New("approval_all_groups", field.Many2Many).WithRelation("res.groups"),
			field.New("dynamic_statusbar_visible", field.Bool),
			field.New("static_states", field.Bool),
			field.New("advance", field.Bool),
			field.New("state_field", field.Char),
			field.New("draft_state", field.Char),
			field.New("approved_state", field.Char),
			field.New("rejected_state", field.Char),
			field.New("cancelled_state", field.Char),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple(ModelSettingsState, "approval_settings_state",
			field.New("settings_id", field.Many2One).WithRelation(ModelSettings),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("value", field.Char),
			field.New("state", field.Char),
			field.New("name", field.Char),
			field.New("sequence", field.Int),
			field.New("active", field.Bool),
			field.New("group_ids", field.Many2Many).WithRelation("res.groups"),
			field.New("tag_ids", field.Many2Many).WithRelation(ModelStateTags),
			field.New("type", field.Selection),
			field.New("kind", field.Selection),
			field.New("reject_state", field.Bool),
			field.New("condition_domain", field.Text),
			field.New("activity_delay_days", field.Int),
			field.New("activity_summary", field.Char),
			field.New("require_all_approvers", field.Bool),
		),
		simple(ModelConfig, "approval_config",
			field.New("name", field.Char),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("model", field.Char),
			field.New("model_name", field.Char),
			field.New("setting_id", field.Many2One).WithRelation(ModelSettings),
			field.New("settings_id", field.Many2One).WithRelation(ModelSettings),
			field.New("state", field.Char),
			field.New("sequence", field.Int),
			field.New("group_ids", field.Many2Many).WithRelation("res.groups"),
			field.New("user_python_code", field.Char),
			field.New("condition", field.Text),
			field.New("schedule_activity", field.Bool),
			field.New("schedule_activity_field_id", field.Many2One).WithRelation("ir.model.fields"),
			field.New("schedule_activity_days", field.Int),
			field.New("state_ids", field.One2Many).WithRelation(ModelSettingsState),
			field.New("button_ids", field.One2Many).WithRelation(ModelButton),
			field.New("automation_ids", field.One2Many).WithRelation(ModelAutomation),
			field.New("escalation_ids", field.One2Many).WithRelation(ModelEscalation),
			field.New("escalation_count", field.Int),
			field.New("tag_ids", field.Many2Many).WithRelation(ModelStateTags),
			field.New("auto_approve", field.Bool),
			field.New("default_mail_template_body", field.Char),
			field.New("default_reject_mail_template_body", field.Char),
			field.New("committee", field.Bool),
			field.New("committee_limit", field.Int),
			field.New("committee_vote_percentage", field.Float),
			field.New("user_ids", field.Many2Many).WithRelation("res.users"),
			field.New("is_voting", field.Bool),
			field.New("active", field.Bool),
		),
		simple(ModelButton, "approval_buttons",
			field.New("settings_id", field.Many2One).WithRelation(ModelSettings),
			field.New("config_id", field.Many2One).WithRelation(ModelConfig),
			field.New("model", field.Char),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("sequence", field.Int),
			field.New("active", field.Bool),
			field.New("name", field.Char),
			actionField("action_type"),
			field.New("state_value", field.Char),
			field.New("next_state", field.Char),
			field.New("return_state", field.Char),
			field.New("transfer_state", field.Char),
			field.New("visible_to", field.Selection),
			field.New("method", field.Char),
			field.New("visible_domain", field.Text),
			field.New("server_action_id", field.Many2One).WithRelation("ir.actions.server"),
			field.New("email_template_id", field.Many2One).WithRelation("mail.template"),
			field.New("email_wizard_form_id", field.Many2One).WithRelation("ir.ui.view"),
			field.New("email_next_action", field.Char),
			field.New("group_ids", field.Many2Many).WithRelation("res.groups"),
			field.New("comment", field.Selection),
			field.New("comment_required", field.Bool),
			field.New("context", field.Char),
			field.New("invisible", field.Char),
			field.New("icon", field.Char),
			field.New("states", field.Text),
			field.New("run_as_superuser", field.Bool),
			field.New("show_process_wizard", field.Bool),
			field.New("hotkey", field.Char),
			field.New("validate_form", field.Bool),
			field.New("confirm_message", field.Char),
			field.New("button_class", field.Char),
			field.New("voting_type", field.Selection),
			field.New("is_voting", field.Bool),
			field.New("vote_threshold", field.Int),
		),
		simple(ModelAutomation, "approval_automation",
			field.New("settings_id", field.Many2One).WithRelation(ModelSettings),
			field.New("model", field.Char),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("sequence", field.Int),
			field.New("active", field.Bool),
			field.New("name", field.Char),
			triggerField("trigger"),
			field.New("from_states", field.Text),
			field.New("to_states", field.Text),
			field.New("code", field.Text),
			field.New("filter_domain", field.Text),
			field.New("template_ids", field.Many2Many).WithRelation("mail.template"),
			field.New("server_action_ids", field.Many2Many).WithRelation("ir.actions.server"),
			field.New("action_dsl", field.Text),
		),
		simple(ModelEscalation, "approval_escalation",
			field.New("config_id", field.Many2One).WithRelation(ModelConfig),
			field.New("settings_id", field.Many2One).WithRelation(ModelSettings),
			field.New("automation_id", field.Many2One).WithRelation("base.automation"),
			field.New("name", field.Char),
			field.New("state_value", field.Char),
			field.New("delay_seconds", field.Int),
			field.New("user_id", field.Many2One).WithRelation("res.users"),
			field.New("server_action_id", field.Many2One).WithRelation("ir.actions.server"),
			field.New("active", field.Bool),
			field.New("sequence", field.Int),
		),
		simple(ModelLog, "approval_log",
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("user_id", field.Many2One).WithRelation("res.users"),
			field.New("date", field.DateTime),
			field.New("description", field.Text),
			field.New("old_state", field.Char),
			field.New("new_state", field.Char),
			field.New("old_status", field.Char),
			field.New("new_status", field.Char),
			field.New("duration_seconds", field.Float),
			field.New("duration_hours", field.Float),
			field.New("duration", field.Char),
			field.New("duration_hours_avg", field.Float),
			field.New("duration_seconds_avg", field.Float),
			field.New("approval_button_id", field.Many2One).WithRelation(ModelButton),
			field.New("bulk_approval", field.Bool),
			field.New("old_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("new_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("workflow_transition_id", field.Many2One).WithRelation(ModelTransition),
			field.New("delegation_id", field.Many2One).WithRelation("delegation"),
			field.New("delegation_employee_id", field.Many2One).WithRelation("hr.employee"),
		),
		simple(ModelLogVoting, "approval_log_voting",
			field.New("user_id", field.Many2One).WithRelation("res.users"),
			field.New("vote", field.Selection),
			field.New("button_id", field.Many2One).WithRelation(ModelButton),
			field.New("comment", field.Text),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("state", field.Char),
			field.New("button_class", field.Char),
		),
		simple(ModelForward, "approval_forward",
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("approval_state_id", field.Many2One).WithRelation(ModelConfig),
			field.New("state_value", field.Char),
			field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("active", field.Bool),
			field.New("user_id", field.Many2One).WithRelation("res.users"),
			field.New("forwarder_user_id", field.Many2One).WithRelation("res.users"),
		),
		simple(ModelCancellation, "cancellation_record",
			field.New("name", field.Char),
			field.New("requester_id", field.Many2One).WithRelation("res.users"),
			field.New("model_id", field.Many2One).WithRelation("ir.model"),
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("reason", field.Text),
			field.New("state", field.Selection),
		),
		simple(ModelStateTags, "state_tags",
			field.New("name", field.Char),
			field.New("color", field.Int),
			field.New("description", field.Text),
		),
		transient(ModelProcessWizard, "approval_process_wizard",
			field.New("model", field.Char),
			field.New("record_ids", field.Text),
			field.New("res_model", field.Char),
			field.New("res_ids", field.Text),
			field.New("confirm_message", field.Char),
			field.New("button_id", field.Many2One).WithRelation(ModelButton),
			field.New("action_type", field.Selection),
			field.New("fixed_return_state", field.Char),
			field.New("return_state", field.Char),
			field.New("transfer_state", field.Char),
			field.New("comment", field.Text),
			field.New("comment_required", field.Bool),
			field.New("visible_selections", field.Text),
			field.New("forward_user_id", field.Many2One).WithRelation("res.users"),
			field.New("target_user_id", field.Many2One).WithRelation("res.users"),
		),
		transient(ModelStateUpdateWizard, "approval_state_update",
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("res_model", field.Char),
			field.New("res_ids", field.Text),
			field.New("state", field.Char),
			field.New("comment", field.Text),
		),
		transient(ModelExpressionEditor, "model_expression_editor",
			field.New("model", field.Char),
			field.New("domain", field.Text),
			field.New("expression", field.Text),
			field.New("code", field.Char),
		),
	}
}

func ModelNames() []string {
	models := Models()
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	return names
}

func ExtensionModels() []model.Model {
	settings := workflowExtension("res.config.settings", "res_config_settings",
		field.New("module_oi_workflow_expense", field.Bool),
		field.New("module_oi_workflow_hr_contract", field.Bool),
		field.New("module_oi_workflow_hr_holidays", field.Bool),
		field.New("module_oi_workflow_hr_holidays_manager", field.Bool),
		field.New("module_oi_workflow_hr_payslip_run", field.Bool),
		field.New("module_oi_workflow_hr_payslip_run_e", field.Bool),
		field.New("module_oi_workflow_purchase_order", field.Bool),
		field.New("module_oi_workflow_purchase_requisition", field.Bool),
		field.New("module_oi_workflow_sale_order", field.Bool),
		field.New("module_oi_workflow_account_payment", field.Bool),
		field.New("module_oi_workflow_crm_lead", field.Bool),
		field.New("module_oi_workflow_invoice", field.Bool),
		field.New("module_oi_workflow_project", field.Bool),
		field.New("module_oi_workflow_project_task", field.Bool),
	)
	settings.Transient = true
	return []model.Model{
		settings,
	}
}

func approvalRecord() model.Model {
	m := model.New(ModelApprovalRecord, "")
	m.Abstract = true
	for _, f := range []field.Field{
		field.New("state", field.Selection),
		field.New("approval_state_id", field.Many2One).WithRelation(ModelConfig),
		field.New("approval_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("approval_done_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("approval_partner_ids", field.Many2Many).WithRelation("res.partner"),
		field.New("user_can_approve", field.Bool),
		field.New("document_user_id", field.Many2One).WithRelation("res.users"),
		field.New("waiting_approval", field.Bool),
		field.New("log_ids", field.One2Many).WithRelation(ModelLog),
		field.New("last_state_update", field.DateTime),
		field.New("workflow_states", field.Text),
		field.New("cancellation_count", field.Int),
		field.New("record_cancellation_count", field.Int),
		field.New("active_record_cancellation_count", field.Int),
		field.New("activity_deadline", field.Date),
		field.New("approval_activity_date_deadline", field.Date),
		field.New("_old_state", field.Char),
		field.New("_approval_button_id", field.Many2One).WithRelation(ModelButton),
		field.New("_approval_comment", field.Char),
		field.New("duration_state_tracking", field.Text),
		field.New("approved_button_clicked", field.Int),
		field.New("approval_voting_ids", field.One2Many).WithRelation(ModelLogVoting),
		field.New("voting_count", field.Int),
		field.New("approval_voting_count", field.Int),
		field.New("reject_voting_count", field.Int),
		field.New("vote_summary", field.Text),
		field.New("vote_summary_html", field.Text),
		field.New("visible_button_ids", field.Many2Many).WithRelation(ModelButton),
		field.New("approval_visible_button_ids", field.Many2Many).WithRelation(ModelButton),
	} {
		m.AddField(f)
	}
	return m
}

func nameSequenceMixin() model.Model {
	m := model.New(ModelNameSequenceMixin, "")
	m.Abstract = true
	m.AddField(field.New("name", field.Char))
	return m
}

func simple(name, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}

func transient(name, table string, fields ...field.Field) model.Model {
	m := simple(name, table, fields...)
	m.Transient = true
	return m
}

func workflowExtension(name string, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	m.Inherit = []string{name}
	if table == "" {
		m.Abstract = true
	}
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}

func triggerField(name string) field.Field {
	f := field.New(name, field.Selection)
	for _, option := range []AutomationTrigger{
		TriggerOnSubmit,
		TriggerOnEnterApproval,
		TriggerOnApprove,
		TriggerOnApproval,
		TriggerOnReject,
		TriggerOnReturn,
		TriggerOnCancel,
		TriggerOnDraft,
		TriggerOnForward,
		TriggerOnTransfer,
		TriggerOnStateUpdated,
		TriggerOnCreate,
		TriggerOnCommitteeApproval,
		TriggerStateChange,
	} {
		f.Selection = append(f.Selection, field.SelectionOption{Value: string(option), Label: string(option)})
	}
	return f
}

func actionField(name string) field.Field {
	f := field.New(name, field.Selection)
	for _, option := range []ButtonAction{
		ActionApprove,
		ActionDraft,
		ActionReject,
		ActionReturn,
		ActionCancel,
		ActionCancelWorkflow,
		ActionTransfer,
		ActionForward,
		ActionEmail,
		ActionServerAction,
		ActionLegacyServerAction,
		ActionMethod,
		ActionVote,
	} {
		f.Selection = append(f.Selection, field.SelectionOption{Value: string(option), Label: string(option)})
	}
	return f
}
