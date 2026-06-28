package ai

import (
	"fmt"
	aiproviders "gorp/internal/ai/providers"
	"strconv"
	"strings"
)

type Provider = aiproviders.Kind

const (
	ProviderOpenAI Provider = aiproviders.KindOpenAI
	ProviderGemini Provider = aiproviders.KindGemini

	DefaultOpenAIChatModel      = "gpt-5-mini"
	DefaultOpenAIEmbeddingModel = "text-embedding-3-small"
	DefaultGeminiChatModel      = "gemini-2.5-flash"
	DefaultGeminiEmbeddingModel = "gemini-embedding-001"
	DefaultChatModel            = DefaultOpenAIChatModel
	DefaultEmbeddingModel       = DefaultOpenAIEmbeddingModel
	DefaultTokenBudget          = 4096
	DefaultRateLimitPerMin      = 60
	RedactedValue               = "[redacted]"
	defaultPromptSystem         = "Answer with business-safe ERP context."
	defaultPromptContext        = "Use only records, sources, and tools authorized for the current user."
	defaultPromptSafety         = "Do not expose secrets, raw credentials, or unauthorized record contents."
)

type SecretSource string

const (
	SecretSourceEnv   SecretSource = "env"
	SecretSourceStore SecretSource = "secret_store"
)

type PromptDefaults struct {
	System  string
	Context string
	Safety  string
}

type SecretReference struct {
	Source SecretSource
	Name   string
}

type Settings struct {
	DefaultProvider       Provider
	DefaultChatModel      string
	DefaultEmbeddingModel string
	TokenBudget           int
	RateLimitPerMinute    int
	PromptDefaults        PromptDefaults
	SecretRef             SecretReference
}

func DefaultSettings() Settings {
	return Settings{
		DefaultProvider:       ProviderOpenAI,
		DefaultChatModel:      DefaultChatModelForProvider(ProviderOpenAI),
		DefaultEmbeddingModel: DefaultEmbeddingModelForProvider(ProviderOpenAI),
		TokenBudget:           DefaultTokenBudget,
		RateLimitPerMinute:    DefaultRateLimitPerMin,
		PromptDefaults: PromptDefaults{
			System:  defaultPromptSystem,
			Context: defaultPromptContext,
			Safety:  defaultPromptSafety,
		},
		SecretRef: EnvSecret("OPENAI_API_KEY"),
	}
}

func DefaultChatModelForProvider(provider Provider) string {
	switch provider {
	case ProviderGemini:
		return DefaultGeminiChatModel
	default:
		return DefaultOpenAIChatModel
	}
}

func DefaultEmbeddingModelForProvider(provider Provider) string {
	switch provider {
	case ProviderGemini:
		return DefaultGeminiEmbeddingModel
	default:
		return DefaultOpenAIEmbeddingModel
	}
}

func EnvSecret(name string) SecretReference {
	return SecretReference{Source: SecretSourceEnv, Name: name}
}

func StoreSecret(name string) SecretReference {
	return SecretReference{Source: SecretSourceStore, Name: name}
}

func (s Settings) WithSecretRef(ref SecretReference) Settings {
	s.SecretRef = ref
	return s
}

func (s Settings) Validate() error {
	switch s.DefaultProvider {
	case ProviderOpenAI, ProviderGemini:
	default:
		return fmt.Errorf("unsupported AI provider %q", s.DefaultProvider)
	}
	if s.DefaultChatModel == "" {
		return fmt.Errorf("default chat model is required")
	}
	if s.DefaultEmbeddingModel == "" {
		return fmt.Errorf("default embedding model is required")
	}
	if s.TokenBudget <= 0 {
		return fmt.Errorf("token budget must be positive")
	}
	if s.RateLimitPerMinute <= 0 {
		return fmt.Errorf("rate limit must be positive")
	}
	if strings.TrimSpace(s.PromptDefaults.System) == "" {
		return fmt.Errorf("prompt system default is required")
	}
	return s.SecretRef.Validate()
}

func (s Settings) Export() map[string]any {
	return map[string]any{
		"default_provider":        string(s.DefaultProvider),
		"default_chat_model":      s.DefaultChatModel,
		"default_embedding_model": s.DefaultEmbeddingModel,
		"token_budget":            s.TokenBudget,
		"rate_limit_per_minute":   s.RateLimitPerMinute,
		"prompt_system":           s.PromptDefaults.System,
		"prompt_context":          s.PromptDefaults.Context,
		"prompt_safety":           s.PromptDefaults.Safety,
		"secret_ref":              s.SecretRef.String(),
	}
}

func (s Settings) LogFields() map[string]string {
	fields := map[string]string{
		"default_provider":        string(s.DefaultProvider),
		"default_chat_model":      s.DefaultChatModel,
		"default_embedding_model": s.DefaultEmbeddingModel,
		"token_budget":            strconv.Itoa(s.TokenBudget),
		"rate_limit_per_minute":   strconv.Itoa(s.RateLimitPerMinute),
		"secret_ref":              s.SecretRef.Redacted(),
	}
	return RedactSecretFields(fields)
}

func (r SecretReference) String() string {
	if r.IsZero() {
		return ""
	}
	return string(r.Source) + ":" + r.Name
}

func (r SecretReference) Redacted() string {
	if r.IsZero() {
		return ""
	}
	return string(r.Source) + ":<set>"
}

func (r SecretReference) ProviderSecretRef() aiproviders.SecretRef {
	switch r.Source {
	case SecretSourceEnv:
		return aiproviders.SecretRef{EnvName: r.Name}
	case SecretSourceStore:
		return aiproviders.SecretRef{StoreID: r.Name}
	default:
		return aiproviders.SecretRef{}
	}
}

func (r SecretReference) IsZero() bool {
	return r.Source == "" && r.Name == ""
}

func (r SecretReference) Validate() error {
	if r.IsZero() {
		return nil
	}
	if r.Source == "" || r.Name == "" {
		return fmt.Errorf("secret source and reference are required together")
	}
	switch r.Source {
	case SecretSourceEnv, SecretSourceStore:
	default:
		return fmt.Errorf("unsupported secret source %q", r.Source)
	}
	if strings.ContainsAny(r.Name, "\r\n\t") {
		return fmt.Errorf("secret reference contains invalid whitespace")
	}
	if looksLikeRawSecret(r.Name) {
		return fmt.Errorf("secret reference must not contain a raw secret")
	}
	return nil
}

func RedactSecretFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		if isSensitiveField(key) {
			out[key] = RedactedValue
			continue
		}
		out[key] = value
	}
	return out
}

func isSensitiveField(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasSuffix(lower, "_ref") || strings.HasSuffix(lower, "_source") {
		return false
	}
	if strings.Contains(lower, "token_budget") {
		return false
	}
	return strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "secret") ||
		lower == "token" ||
		strings.HasSuffix(lower, "_token") ||
		strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "refresh_token")
}

func looksLikeRawSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "sk-") ||
		strings.HasPrefix(lower, "bearer ") ||
		strings.Contains(lower, "-----begin ")
}
