package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	aiaddon "gorp/addons/ai"
	serveractions "gorp/internal/actions"
	"gorp/internal/ai/agents"
	aicontrollers "gorp/internal/ai/controllers"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/ai/rag"
	aitools "gorp/internal/ai/tools"
	"gorp/internal/domain"
	"gorp/internal/record"
)

const (
	defaultMockChatModel      = "mock-chat"
	defaultMockEmbeddingModel = "mock-embedding"
)

func defaultAIProviderRegistry() *aiproviders.Registry {
	return runtimeAIProviderRegistry(nil)
}

func runtimeAIProviderRegistry(secretResolver aiproviders.SecretResolver) *aiproviders.Registry {
	options := []aiproviders.CompatibleOption{}
	if secretResolver != nil {
		options = append(options, aiproviders.WithSecretResolver(secretResolver))
	}
	return aiproviders.NewRegistry(
		aiproviders.NewMockProvider(),
		aiproviders.NewOpenAICompatible(aiproviders.SecretRef{StoreID: "ai.openai_key", EnvName: "ODOO_AI_CHATGPT_TOKEN"}, options...),
		aiproviders.NewGeminiCompatible(aiproviders.SecretRef{StoreID: "ai.google_key", EnvName: "ODOO_AI_GEMINI_TOKEN"}, options...),
	)
}

func (a *App) aiChatFactory() func(*record.Env) *aicontrollers.ChatService {
	return func(env *record.Env) *aicontrollers.ChatService {
		if env == nil {
			env = a.Env
		}
		if env == nil {
			return nil
		}
		settingsEnv := a.aiSettingsEnv(env)
		settings := aiRuntimeSettingsFromEnv(settingsEnv)
		resolver := a.aiProviderResolver(settingsEnv, settings)
		if resolver.registry == nil {
			return nil
		}
		defaultChatModel := settings.defaultChatModel
		if strings.TrimSpace(defaultChatModel) == "" {
			defaultChatModel = resolver.defaultChatModel()
		}
		explicitEmbeddingModel := firstNonEmptyString(a.AIEmbeddingModel)
		return &aicontrollers.ChatService{
			Store:     envAIChannelStore{env: env, defaultChatModel: defaultChatModel},
			Responder: runtimeAIResponder{env: env, actions: a.ServerActions, resolver: resolver},
			Retriever: runtimeAIRetriever{env: env, resolver: resolver, explicitEmbeddingModel: explicitEmbeddingModel},
			BaseURL:   a.AIBaseURL,
		}
	}
}

func (a *App) aiProviderResolver(env *record.Env, settings aiRuntimeSettings) aiProviderResolver {
	registry := a.AIProviders
	if registry == nil {
		registry = runtimeAIProviderRegistry(recordEnvSecretResolver(env))
	}
	registerSettingsProvider(registry, settings, recordEnvSecretResolver(env))
	fallback := a.AIProvider
	if fallback == nil {
		fallback = aiproviders.NewMockProvider()
	}
	if fallback != nil {
		_ = registry.Register(fallback)
	}
	return aiProviderResolver{registry: registry, fallback: fallback}
}

type aiRuntimeSettings struct {
	defaultChatModel string
	defaultProvider  aiproviders.Kind
	secretSource     string
	secretRef        string
}

func aiRuntimeSettingsFromEnv(env *record.Env) aiRuntimeSettings {
	if env == nil {
		return aiRuntimeSettings{}
	}
	rows, err := allRows(env, aiaddon.ModelSettings, "default_chat_model", "default_provider", "secret_source", "secret_ref")
	if err != nil || len(rows) == 0 {
		return aiRuntimeSettings{}
	}
	row := rows[len(rows)-1]
	return aiRuntimeSettings{
		defaultChatModel: strings.TrimSpace(stringValue(row["default_chat_model"])),
		defaultProvider:  aiProviderKindFromSettings(stringValue(row["default_provider"])),
		secretSource:     strings.TrimSpace(stringValue(row["secret_source"])),
		secretRef:        strings.TrimSpace(stringValue(row["secret_ref"])),
	}
}

func registerSettingsProvider(registry *aiproviders.Registry, settings aiRuntimeSettings, secretResolver aiproviders.SecretResolver) {
	if registry == nil {
		return
	}
	ref := runtimeSecretRef(settings.secretSource, settings.secretRef)
	if ref.Redacted() == "" || settings.defaultProvider == "" {
		return
	}
	options := []aiproviders.CompatibleOption{aiproviders.WithSecretResolver(secretResolver)}
	switch settings.defaultProvider {
	case aiproviders.KindOpenAI:
		_ = registry.Register(aiproviders.NewOpenAICompatible(ref, options...))
	case aiproviders.KindGemini:
		_ = registry.Register(aiproviders.NewGeminiCompatible(ref, options...))
	}
}

func runtimeSecretRef(source string, value string) aiproviders.SecretRef {
	source = strings.TrimSpace(source)
	value = strings.TrimSpace(value)
	if value == "" {
		return aiproviders.SecretRef{}
	}
	if prefix, rest, ok := strings.Cut(value, ":"); ok {
		switch prefix {
		case string(aiaddon.SecretSourceEnv):
			source = prefix
			value = strings.TrimSpace(rest)
		case string(aiaddon.SecretSourceStore):
			source = prefix
			value = strings.TrimSpace(rest)
		}
	}
	ref := aiaddon.SecretReference{Source: aiaddon.SecretSource(source), Name: value}
	if err := ref.Validate(); err != nil {
		return aiproviders.SecretRef{}
	}
	return ref.ProviderSecretRef()
}

func recordEnvSecretResolver(env *record.Env) aiproviders.SecretResolver {
	return func(ctx context.Context, ref aiproviders.SecretRef) (string, error) {
		if strings.TrimSpace(ref.Raw) != "" {
			return aiproviders.DefaultSecretResolver(ctx, aiproviders.SecretRef{Raw: ref.Raw})
		}
		if env != nil && strings.TrimSpace(ref.StoreID) != "" {
			found, err := env.Model("ir.config_parameter").SearchWithOptions(domain.Cond("key", domain.Equal, strings.TrimSpace(ref.StoreID)), record.SearchOptions{Limit: 1})
			if err == nil {
				rows, err := found.Read("value")
				if err == nil && len(rows) > 0 {
					value := strings.TrimSpace(stringValue(rows[0]["value"]))
					if value != "" {
						return value, nil
					}
				}
			}
		}
		if strings.TrimSpace(ref.EnvName) != "" {
			return aiproviders.DefaultSecretResolver(ctx, aiproviders.SecretRef{EnvName: ref.EnvName})
		}
		return "", aiproviders.ErrSecretMissing
	}
}

func (a *App) aiSettingsEnv(requestEnv *record.Env) *record.Env {
	if a != nil && a.Env != nil {
		return systemEnv(a.Env)
	}
	return systemEnv(requestEnv)
}

func systemEnv(env *record.Env) *record.Env {
	if env == nil {
		return nil
	}
	ctx := env.Context()
	ctx.UserID = 1
	return env.WithContext(ctx)
}

func aiProviderKindFromSettings(value string) aiproviders.Kind {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "google":
		return aiproviders.KindGemini
	default:
		return aiproviders.Kind(strings.TrimSpace(value))
	}
}

type aiProviderResolver struct {
	registry *aiproviders.Registry
	fallback aiproviders.Provider
}

func (r aiProviderResolver) defaultChatModel() string {
	if r.fallback != nil && r.fallback.Kind() == aiproviders.KindMock {
		return defaultMockChatModel
	}
	return aiaddon.DefaultChatModel
}

func (r aiProviderResolver) chatProvider(model string) (aiproviders.Provider, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = r.defaultChatModel()
	}
	if r.registry != nil {
		provider, found, err := r.registry.ProviderForModel(model, aiproviders.ModelChat)
		if err == nil {
			return provider, found.ID, nil
		}
	}
	if r.fallback != nil {
		return r.fallback, model, nil
	}
	return nil, "", fmt.Errorf("ai provider missing for chat model %s", model)
}

func (r aiProviderResolver) embeddingProvider(chatModel string, explicitModel string) (aiproviders.Provider, string, error) {
	model := strings.TrimSpace(explicitModel)
	if model == "" {
		model = r.embeddingModelForChat(chatModel)
	}
	if r.registry != nil {
		provider, found, err := r.registry.ProviderForModel(model, aiproviders.ModelEmbedding)
		if err == nil {
			return provider, found.ID, nil
		}
	}
	if r.fallback != nil {
		return r.fallback, model, nil
	}
	return nil, "", fmt.Errorf("ai provider missing for embedding model %s", model)
}

func (r aiProviderResolver) embeddingModelForChat(chatModel string) string {
	provider, _, err := r.chatProvider(chatModel)
	if err != nil || provider == nil {
		return aiaddon.DefaultEmbeddingModel
	}
	switch provider.Kind() {
	case aiproviders.KindMock:
		return defaultMockEmbeddingModel
	case aiproviders.KindGemini:
		return "gemini-embedding-001"
	default:
		return aiaddon.DefaultEmbeddingModel
	}
}

type runtimeAIResponder struct {
	resolver aiProviderResolver
	env      *record.Env
	actions  *serveractions.Registry
}

func (r runtimeAIResponder) Generate(ctx context.Context, agent agents.Agent, request agents.Request) (agents.Response, error) {
	provider, model, err := r.resolver.chatProvider(agent.Model)
	if err != nil {
		return agents.Response{}, err
	}
	agent.Model = model
	if len(request.Tools) == 0 && request.ToolHandler == nil {
		toolRegistry, providerTools, err := r.agentTopicTools(agent)
		if err != nil {
			return agents.Response{}, err
		}
		if len(providerTools) > 0 {
			request.Tools = providerTools
			request.ToolHandler = agents.ToolHandlerFunc(func(ctx context.Context, call aiproviders.ToolCall, req agents.Request) (map[string]any, error) {
				output, err := toolRegistry.Run(ctx, aitools.Request{
					UserID:    req.UserID,
					CompanyID: req.CompanyID,
					Model:     req.ActiveModel,
					RecordID:  req.ActiveID,
					ToolName:  call.Name,
					Input:     cloneAIMap(call.Arguments),
					Metadata:  aiChatToolMetadata(req),
				})
				return cloneAIMap(output.Output), err
			})
		}
	}
	return agents.Generate(ctx, agent, provider, request)
}

func (r runtimeAIResponder) agentTopicTools(agent agents.Agent) (*aitools.Registry, []aiproviders.ToolCall, error) {
	if r.env == nil || r.actions == nil || len(agent.TopicIDs) == 0 {
		return nil, nil, nil
	}
	actionIDs, err := aiTopicToolIDs(r.env, agent.TopicIDs)
	if err != nil {
		return nil, nil, err
	}
	registry := aitools.NewRegistry(aiActionToolAuthorizer{}, nil)
	providerTools := make([]aiproviders.ToolCall, 0, len(actionIDs))
	for _, actionID := range actionIDs {
		action, ok := r.actions.Get(actionID)
		if !ok || !action.UseInAI {
			continue
		}
		tool, err := aitools.ServerActionTool(action, r.actions)
		if err != nil {
			return nil, nil, err
		}
		if len(agent.ToolAllowlist) > 0 && !agents.AllowsTool(agent, tool.Name) {
			continue
		}
		if err := registry.Register(tool); err != nil {
			return nil, nil, err
		}
		providerTools = append(providerTools, aiproviders.ToolCall{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  aiToolSchemaParameters(tool.Schema),
		})
	}
	return registry, providerTools, nil
}

func aiChatToolMetadata(req agents.Request) map[string]any {
	metadata := map[string]any{}
	if strings.TrimSpace(req.SessionID) != "" {
		metadata["ai_session_identifier"] = strings.TrimSpace(req.SessionID)
	}
	return metadata
}

func aiTopicToolIDs(env *record.Env, topicIDs []int64) ([]int64, error) {
	rows, err := env.Model(aiaddon.ModelTopic).Browse(topicIDs...).Read("tool_ids")
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	out := []int64{}
	for _, row := range rows {
		for _, id := range int64Slice(row["tool_ids"]) {
			if id != 0 && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

type runtimeAIRetriever struct {
	env                    *record.Env
	resolver               aiProviderResolver
	explicitEmbeddingModel string
}

func (r runtimeAIRetriever) Retrieve(ctx context.Context, req rag.Request) ([]rag.RetrievedChunk, error) {
	if len(req.Agent.SourceIDs) == 0 {
		return nil, nil
	}
	provider, model, err := r.resolver.embeddingProvider(req.Agent.Model, r.explicitEmbeddingModel)
	if err != nil {
		return nil, err
	}
	return rag.PersistedRetriever{Env: r.env, Provider: provider, EmbeddingModel: model}.Retrieve(ctx, req)
}

type envAIChannelStore struct {
	env              *record.Env
	defaultChatModel string
}

func (s envAIChannelStore) Channel(_ context.Context, id int64) (aicontrollers.Channel, bool) {
	rows, err := s.env.Model("discuss.channel").Browse(id).Read("name", "channel_type", "active", "ai_env_context", "ai_agent_id")
	if err != nil || len(rows) == 0 || !boolWithFallback(rows[0]["active"], true) {
		return aicontrollers.Channel{}, false
	}
	agent, _ := s.agent(int64Value(rows[0]["ai_agent_id"]))
	return aicontrollers.Channel{
		ID:           id,
		Name:         stringValue(rows[0]["name"]),
		Type:         stringValue(rows[0]["channel_type"]),
		IsMember:     s.isMember(id),
		Agent:        agent,
		AIEnvContext: contextParts(rows[0]["ai_env_context"]),
	}, true
}

func (s envAIChannelStore) Message(_ context.Context, id int64) (aicontrollers.Message, bool) {
	rows, err := s.env.Model("mail.message").Browse(id).Read("body", "author_id", "date")
	if err != nil || len(rows) == 0 {
		return aicontrollers.Message{}, false
	}
	return aicontrollers.Message{
		ID:        id,
		Body:      stringValue(rows[0]["body"]),
		CreatedAt: timeValue(rows[0]["date"]),
	}, true
}

func (s envAIChannelStore) History(_ context.Context, channelID int64, limit int) []aicontrollers.Message {
	found, err := s.env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "discuss.channel"),
		domain.Cond("res_id", domain.Equal, channelID),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("body", "author_id", "date")
	if err != nil {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool { return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"]) })
	if limit > 0 && len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	agentPartnerID := s.channelAgentPartnerID(channelID)
	out := make([]aicontrollers.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, aicontrollers.Message{
			ID:            int64Value(row["id"]),
			Body:          stringValue(row["body"]),
			AuthorIsAgent: agentPartnerID != 0 && int64Value(row["author_id"]) == agentPartnerID,
			CreatedAt:     timeValue(row["date"]),
		})
	}
	return out
}

func (s envAIChannelStore) DeleteChannel(_ context.Context, channelID int64) error {
	return s.env.Model("discuss.channel").Browse(channelID).Unlink()
}

func (s envAIChannelStore) PostAssistantMessage(_ context.Context, channelID int64, text string) error {
	values := map[string]any{
		"body":         text,
		"message_type": "comment",
		"model":        "discuss.channel",
		"res_id":       channelID,
		"date":         time.Now().UTC(),
	}
	if partnerID := s.channelAgentPartnerID(channelID); partnerID != 0 {
		values["author_id"] = partnerID
	}
	_, err := s.env.Model("mail.message").Create(values)
	return err
}

func (s envAIChannelStore) agent(id int64) (agents.Agent, bool) {
	if id == 0 {
		return agents.Agent{}, false
	}
	rows, err := s.env.Model(aiaddon.ModelAgent).Browse(id).Read("name", "purpose", "system_prompt", "llm_model", "restrict_to_sources", "active", "topic_ids", "source_ids", "sources_ids", "tool_allowlist", "company_id")
	if err != nil || len(rows) == 0 {
		return agents.Agent{}, false
	}
	modelName := strings.TrimSpace(stringValue(rows[0]["llm_model"]))
	if modelName == "" {
		modelName = s.defaultChatModel
	}
	sourceIDs := int64Slice(rows[0]["source_ids"])
	if len(sourceIDs) == 0 {
		sourceIDs = int64Slice(rows[0]["sources_ids"])
	}
	return agents.Agent{
		ID:                id,
		Name:              firstNonEmptyString(stringValue(rows[0]["name"]), "AI"),
		Purpose:           stringValue(rows[0]["purpose"]),
		SystemPrompt:      stringValue(rows[0]["system_prompt"]),
		Model:             modelName,
		Active:            boolWithFallback(rows[0]["active"], true),
		RestrictToSources: boolWithFallback(rows[0]["restrict_to_sources"], false),
		TopicIDs:          int64Slice(rows[0]["topic_ids"]),
		SourceIDs:         sourceIDs,
		ToolAllowlist:     textList(rows[0]["tool_allowlist"]),
		CompanyID:         int64Value(rows[0]["company_id"]),
	}, true
}

func (s envAIChannelStore) isMember(channelID int64) bool {
	userID := s.env.Context().UserID
	if userID == 1 {
		return true
	}
	if userID == 0 {
		return false
	}
	partnerID := s.userPartnerID(userID)
	node := domain.And(
		domain.Cond("channel_id", domain.Equal, channelID),
		domain.Or(
			domain.Cond("user_id", domain.Equal, userID),
			domain.Cond("partner_id", domain.Equal, partnerID),
		),
	)
	found, err := s.env.Model("discuss.channel.member").SearchWithOptions(node, record.SearchOptions{Limit: 1})
	return err == nil && found.Len() > 0
}

func (s envAIChannelStore) userPartnerID(userID int64) int64 {
	rows, err := s.env.Model("res.users").Browse(userID).Read("partner_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["partner_id"])
}

func (s envAIChannelStore) channelAgentPartnerID(channelID int64) int64 {
	rows, err := s.env.Model("discuss.channel").Browse(channelID).Read("ai_agent_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	agentRows, err := s.env.Model(aiaddon.ModelAgent).Browse(int64Value(rows[0]["ai_agent_id"])).Read("partner_id")
	if err != nil || len(agentRows) == 0 {
		return 0
	}
	return int64Value(agentRows[0]["partner_id"])
}

func contextParts(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return cleanStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringValue(item))
		}
		return cleanStrings(out)
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var jsonStrings []string
		if err := json.Unmarshal([]byte(text), &jsonStrings); err == nil {
			return cleanStrings(jsonStrings)
		}
		var jsonValues []any
		if err := json.Unmarshal([]byte(text), &jsonValues); err == nil {
			out := make([]string, 0, len(jsonValues))
			for _, item := range jsonValues {
				out = append(out, stringValue(item))
			}
			return cleanStrings(out)
		}
		return []string{text}
	default:
		text := strings.TrimSpace(stringValue(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func textList(value any) []string {
	items := contextParts(value)
	if len(items) != 1 {
		return items
	}
	text := items[0]
	if !strings.ContainsAny(text, ",\n") {
		return items
	}
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	return cleanStrings(fields)
}

func cleanStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func timeValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			parsed, err := time.Parse(layout, strings.TrimSpace(typed))
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
