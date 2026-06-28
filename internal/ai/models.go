package ai

import (
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/registry"
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
		simple("ai.agent", "ai_agent",
			field.New("name", field.Char),
			field.New("subtitle", field.Char),
			field.New("purpose", field.Text),
			field.New("system_prompt", field.Text),
			field.New("response_style", field.Selection),
			field.New("llm_model", field.Char),
			field.New("restrict_to_sources", field.Bool),
			field.New("active", field.Bool),
			field.New("image_128", field.Binary),
			field.New("avatar_128", field.Binary),
			field.New("topic_ids", field.Many2Many).WithRelation("ai.topic"),
			field.New("partner_id", field.Many2One).WithRelation("res.partner"),
			field.New("is_system_agent", field.Bool),
			field.New("source_ids", field.One2Many).WithRelation("ai.agent.source"),
			field.New("sources_ids", field.One2Many).WithRelation("ai.agent.source"),
			field.New("sources_fully_processed", field.Bool),
			field.New("is_ask_ai_agent", field.Bool),
			field.New("tool_allowlist", field.Text),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple("ai.topic", "ai_topic",
			field.New("name", field.Char),
			field.New("description", field.Text),
			field.New("instructions", field.Text),
			field.New("tool_ids", field.Many2Many).WithRelation("ir.actions.server"),
			field.New("active", field.Bool),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple("ai.agent.source", "ai_agent_source",
			field.New("name", field.Char),
			field.New("agent_id", field.Many2One).WithRelation("ai.agent"),
			field.New("type", field.Selection),
			field.New("source_type", field.Selection),
			field.New("res_model", field.Char),
			field.New("res_id", field.Int),
			field.New("status", field.Selection),
			field.New("is_active", field.Bool),
			field.New("error_details", field.Text),
			field.New("attachment_id", field.Many2One).WithRelation("ir.attachment"),
			field.New("mimetype", field.Char),
			field.New("file_size", field.Int),
			field.New("url", field.Char),
			field.New("content", field.Text),
			field.New("state", field.Selection),
			field.New("embedding_model", field.Char),
			field.New("user_has_access", field.Bool),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple("ai.prompt.button", "ai_prompt_button",
			field.New("name", field.Char),
			field.New("prompt", field.Text),
			field.New("model_name", field.Char),
			field.New("use_in_ai", field.Bool),
			field.New("sequence", field.Int),
			field.New("composer_id", field.Many2One).WithRelation("ai.composer"),
			field.New("active", field.Bool),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple("ai.embedding", "ai_embedding",
			field.New("agent_source_id", field.Many2One).WithRelation("ai.agent.source"),
			field.New("attachment_id", field.Many2One).WithRelation("ir.attachment"),
			field.New("checksum", field.Char),
			field.New("sequence", field.Int),
			field.New("res_model", field.Char),
			field.New("res_id", field.Int),
			field.New("content", field.Text),
			field.New("chunk_index", field.Int),
			field.New("embedding_model", field.Char),
			field.New("has_embedding_generation_failed", field.Bool),
			field.New("embedding_vector", field.Text),
			field.New("metadata", field.Text),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
		),
		simple("ai.audit.log", "ai_audit_log",
			field.New("name", field.Char),
			field.New("event_type", field.Selection),
			field.New("event_time", field.DateTime),
			field.New("user_id", field.Many2One).WithRelation("res.users"),
			field.New("company_id", field.Many2One).WithRelation("res.company"),
			field.New("agent_id", field.Many2One).WithRelation("ai.agent"),
			field.New("prompt_id", field.Many2One).WithRelation("ai.prompt.button"),
			field.New("action_id", field.Many2One).WithRelation("ir.actions.server"),
			field.New("provider", field.Char),
			field.New("ai_model", field.Char),
			field.New("res_model", field.Char),
			field.New("res_id", field.Int),
			field.New("input_tokens", field.Int),
			field.New("output_tokens", field.Int),
			field.New("latency_millis", field.Int),
			field.New("tool_names", field.Text),
			field.New("tool_count", field.Int),
			field.New("permission_result", field.Selection),
			field.New("status", field.Selection),
			field.New("error", field.Text),
			field.New("metadata", field.Text),
		),
		simple("ai.composer", "ai_composer",
			field.New("name", field.Char),
			field.New("interface_key", field.Selection),
			field.New("focused_models", field.Many2Many).WithRelation("ir.model"),
			field.New("ai_agent", field.Many2One).WithRelation("ai.agent"),
			field.New("default_prompt", field.Text),
			field.New("available_prompt_ids", field.Many2Many).WithRelation("ai.prompt.button"),
			field.New("available_prompts", field.One2Many).WithRelation("ai.prompt.button"),
			field.New("is_system_default", field.Bool),
			field.New("active", field.Bool),
		),
	}
}

func simple(name, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}
