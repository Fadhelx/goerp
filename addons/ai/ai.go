package ai

import (
	internalai "gorp/internal/ai"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

const (
	ModuleName = "ai"

	ModelSettings     = "ai.settings"
	ModelAgent        = "ai.agent"
	ModelTopic        = "ai.topic"
	ModelAgentSource  = "ai.agent.source"
	ModelPromptButton = "ai.prompt.button"
	ModelEmbedding    = "ai.embedding"
	ModelComposer     = "ai.composer"
	ModelAuditLog     = "ai.audit.log"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "AI",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Hidden",
		Depends:       []string{"mail"},
		Installable:   true,
		Application:   false,
		Data: []string{
			"data/ir_actions_server_data.xml",
			"data/ai_topic_data.xml",
			"security/ir.model.access.csv",
			"views/res_config_settings_views.xml",
			"views/ai_audit_log_views.xml",
			"views/ir_actions_server_views.xml",
			"views/mail_scheduled_message_views.xml",
			"views/mail_template_views.xml",
			"views/templates.xml",
			"data/ir_cron.xml",
			"data/ai_agent_data.xml",
			"data/ai_composer_data.xml",
			"wizard/mail_compose_message_views.xml",
		},
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{
			Name:          "Mail",
			TechnicalName: "mail",
			Version:       "19.0.1.0.0",
			Category:      "Productivity",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Automation",
			TechnicalName: "automation",
			Version:       "19.0.1.0.0",
			Category:      "Automation",
			Depends:       []string{"base", "mail"},
			Installable:   true,
		},
	}
}

func RegisterModels(reg *registry.Registry) error {
	if err := reg.RegisterModel(aiSettings()); err != nil {
		return err
	}
	return internalai.RegisterModels(reg)
}

func RegisterRecordModels(reg *record.Registry) error {
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			return err
		}
	}
	return nil
}

func Models() []model.Model {
	models := []model.Model{aiSettings()}
	return append(models, internalai.Models()...)
}

func ModelNames() []string {
	models := Models()
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	return names
}

func aiSettings() model.Model {
	m := model.New(ModelSettings, "ai_settings")
	m.Transient = true
	for _, f := range []field.Field{
		field.New("name", field.Char),
		providerField("default_provider"),
		field.New("default_chat_model", field.Char),
		field.New("default_embedding_model", field.Char),
		field.New("token_budget", field.Int),
		field.New("rate_limit_per_minute", field.Int),
		field.New("prompt_system", field.Text),
		field.New("prompt_context", field.Text),
		field.New("prompt_safety", field.Text),
		secretSourceField("secret_source"),
		field.New("secret_ref", field.Char),
		field.New("openai_key_enabled", field.Bool),
		field.New("openai_key", field.Char),
		field.New("google_key_enabled", field.Bool),
		field.New("google_key", field.Char),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
	} {
		m.AddField(f)
	}
	return m
}

func providerField(name string) field.Field {
	f := field.New(name, field.Selection)
	f.Selection = []field.SelectionOption{
		{Value: string(ProviderOpenAI), Label: "OpenAI"},
		{Value: string(ProviderGemini), Label: "Google Gemini"},
	}
	return f
}

func secretSourceField(name string) field.Field {
	f := field.New(name, field.Selection)
	f.Selection = []field.SelectionOption{
		{Value: string(SecretSourceEnv), Label: "Environment"},
		{Value: string(SecretSourceStore), Label: "Secret Store"},
	}
	return f
}
