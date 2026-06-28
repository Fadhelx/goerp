package ai

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || manifest.Application {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.Category != "Hidden" {
		t.Fatalf("category = %s", manifest.Category)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"mail"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if len(manifest.Data) == 0 {
		t.Fatalf("expected ai data files")
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
	for _, name := range []string{ModelSettings, ModelAgent, ModelTopic, ModelAgentSource, ModelPromptButton, ModelEmbedding, "ai.composer"} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	settings := reg.Models[ModelSettings]
	for _, fieldName := range []string{"default_provider", "default_chat_model", "default_embedding_model", "token_budget", "rate_limit_per_minute", "prompt_system", "secret_ref", "openai_key_enabled", "openai_key", "google_key_enabled", "google_key"} {
		if _, ok := settings.Fields[fieldName]; !ok {
			t.Fatalf("missing ai.settings field %s", fieldName)
		}
	}
	agent := reg.Models[ModelAgent]
	for _, fieldName := range []string{"subtitle", "purpose", "system_prompt", "response_style", "llm_model", "restrict_to_sources", "topic_ids", "partner_id", "is_system_agent", "source_ids", "sources_ids", "sources_fully_processed", "is_ask_ai_agent", "tool_allowlist", "company_id"} {
		if _, ok := agent.Fields[fieldName]; !ok {
			t.Fatalf("missing ai.agent field %s", fieldName)
		}
	}
	if _, ok := agent.Fields["api_key"]; ok {
		t.Fatal("ai.agent exposes raw api_key field")
	}
	source := reg.Models[ModelAgentSource]
	for _, fieldName := range []string{"type", "status", "is_active", "error_details", "attachment_id", "mimetype", "file_size", "user_has_access"} {
		if _, ok := source.Fields[fieldName]; !ok {
			t.Fatalf("missing ai.agent.source field %s", fieldName)
		}
	}
	composer := reg.Models["ai.composer"]
	for _, fieldName := range []string{"interface_key", "focused_models", "ai_agent", "available_prompts", "is_system_default"} {
		if _, ok := composer.Fields[fieldName]; !ok {
			t.Fatalf("missing ai.composer field %s", fieldName)
		}
	}
}

func TestAISettings(t *testing.T) {
	settings := DefaultSettings()
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	if settings.DefaultProvider != ProviderOpenAI {
		t.Fatalf("provider = %s", settings.DefaultProvider)
	}
	if settings.DefaultChatModel != DefaultChatModel {
		t.Fatalf("chat model = %s", settings.DefaultChatModel)
	}
	if settings.DefaultEmbeddingModel != DefaultEmbeddingModel {
		t.Fatalf("embedding model = %s", settings.DefaultEmbeddingModel)
	}
	if settings.TokenBudget != DefaultTokenBudget || settings.RateLimitPerMinute != DefaultRateLimitPerMin {
		t.Fatalf("limits = %+v", settings)
	}
	if settings.PromptDefaults.System == "" || settings.PromptDefaults.Context == "" || settings.PromptDefaults.Safety == "" {
		t.Fatalf("prompt defaults = %+v", settings.PromptDefaults)
	}

	settings = settings.WithSecretRef(StoreSecret("providers/openai/default"))
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	exported := settings.Export()
	if exported["secret_ref"] != "secret_store:providers/openai/default" {
		t.Fatalf("secret ref export = %+v", exported["secret_ref"])
	}

	if chat := DefaultChatModelForProvider(ProviderGemini); chat != DefaultGeminiChatModel {
		t.Fatalf("gemini chat default = %s", chat)
	}
	if embedding := DefaultEmbeddingModelForProvider(ProviderGemini); embedding != DefaultGeminiEmbeddingModel {
		t.Fatalf("gemini embedding default = %s", embedding)
	}
}

func TestAISettingsRedactsSecrets(t *testing.T) {
	raw := "sk-test-raw-secret"
	settings := DefaultSettings().WithSecretRef(EnvSecret("OPENAI_API_KEY"))
	exported := fmt.Sprint(settings.Export())
	if strings.Contains(exported, raw) {
		t.Fatalf("raw key exported: %s", exported)
	}

	logFields := settings.LogFields()
	if strings.Contains(fmt.Sprint(logFields), raw) {
		t.Fatalf("raw key logged: %+v", logFields)
	}
	if logFields["secret_ref"] != "env:<set>" {
		t.Fatalf("secret ref log field = %+v", logFields["secret_ref"])
	}

	redacted := RedactSecretFields(map[string]string{
		"api_key":      raw,
		"access_token": raw,
		"token_budget": "4096",
		"secret_ref":   "env:OPENAI_API_KEY",
	})
	if redacted["api_key"] != RedactedValue || redacted["access_token"] != RedactedValue {
		t.Fatalf("secrets not redacted: %+v", redacted)
	}
	if redacted["token_budget"] != "4096" || redacted["secret_ref"] != "env:OPENAI_API_KEY" {
		t.Fatalf("non-secret fields redacted: %+v", redacted)
	}

	bad := DefaultSettings().WithSecretRef(EnvSecret(raw))
	if err := bad.Validate(); err == nil {
		t.Fatal("expected raw secret reference to be rejected")
	}
}

func TestAISecurity(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Companies[1] = security.Company{ID: 1, Name: "Main", Active: true}
	engine.Companies[2] = security.Company{ID: 2, Name: "Branch", ParentID: 1, Active: true}
	engine.Companies[3] = security.Company{ID: 3, Name: "Other", Active: true}
	engine.Users[10] = security.User{ID: 10, Login: "ai-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{GroupAIUser}}
	engine.Users[20] = security.User{ID: 20, Login: "ai-admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2, 3}, GroupIDs: []int64{GroupAIAdmin}}
	engine.Users[30] = security.User{ID: 30, Login: "no-ai", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: nil}

	for _, groupID := range []int64{GroupAIUser, GroupAIAdmin} {
		if engine.Groups[groupID].ID == 0 {
			t.Fatalf("missing group %d", groupID)
		}
	}
	if !engine.EffectiveGroupIDs(20)[GroupAIUser] {
		t.Fatalf("admin does not imply AI user")
	}

	for _, modelName := range AISecuredModelNames() {
		if err := engine.Check(record.Context{UserID: 10}, modelName, record.OpRead, nil); err != nil {
			t.Fatalf("AI user read %s: %v", modelName, err)
		}
		if err := engine.Check(record.Context{UserID: 10}, modelName, record.OpWrite, nil); !errors.Is(err, security.ErrAccessDenied) {
			t.Fatalf("expected AI user write denied on %s, got %v", modelName, err)
		}
		if err := engine.Check(record.Context{UserID: 20}, modelName, record.OpUnlink, nil); err != nil {
			t.Fatalf("AI admin unlink %s: %v", modelName, err)
		}
	}
	if err := engine.Check(record.Context{UserID: 30}, ModelAgent, record.OpRead, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected no-group user denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 10}, ModelSettings, record.OpRead, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected AI user settings read denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, ModelSettings, record.OpWrite, nil); err != nil {
		t.Fatalf("AI admin settings write: %v", err)
	}

	assertAIRule(t, engine, 10, ModelAgentSource, map[string]any{"company_id": nil}, true)
	assertAIRule(t, engine, 10, ModelAgentSource, map[string]any{"company_id": int64(2)}, true)
	assertAIRule(t, engine, 10, ModelAgentSource, map[string]any{"company_id": int64(3)}, false)

	foundCompanyRule := false
	for _, definition := range SecurityRuleDefinitions() {
		if definition.Rule.Model == ModelEmbedding && strings.Contains(definition.Name, "ai_company") {
			foundCompanyRule = definition.Rule.Domain.Kind == domain.Any
			break
		}
	}
	if !foundCompanyRule {
		t.Fatal("missing ai.embedding company rule shape")
	}
}

func assertAIRule(t *testing.T, engine *security.Engine, userID int64, modelName string, row map[string]any, want bool) {
	t.Helper()
	ok, err := engine.AllowedByRecordRules(userID, modelName, record.OpRead, row)
	if err != nil {
		t.Fatal(err)
	}
	if ok != want {
		t.Fatalf("rule(%s, %+v) = %v, want %v", modelName, row, ok, want)
	}
}
