package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Kind string

const (
	KindOpenAI Kind = "openai"
	KindGemini Kind = "gemini"
	KindMock   Kind = "mock"
)

var (
	ErrUnknownProvider = errors.New("unknown ai provider")
	ErrUnknownModel    = errors.New("unknown ai model")
	ErrSecretMissing   = errors.New("ai provider secret missing")
)

type ModelKind string

const (
	ModelChat      ModelKind = "chat"
	ModelEmbedding ModelKind = "embedding"
)

type Model struct {
	ID          string
	Label       string
	Kind        ModelKind
	Provider    Kind
	Deprecated  bool
	TokenLimit  int
	Dimensions  int
	Temperature bool
}

type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type Message struct {
	Role    string
	Content string
}

type ToolCall struct {
	ID          string
	Name        string
	Description string
	Parameters  map[string]any
	Arguments   map[string]any
}

type ChatRequest struct {
	Model          string
	SystemPrompts  []string
	UserPrompts    []string
	Messages       []Message
	Tools          []ToolCall
	Timeout        time.Duration
	MaxRetries     int
	MaxOutputToken int
}

type ChatResponse struct {
	Text      string
	ToolCalls []ToolCall
	Usage     TokenUsage
	Model     string
	Provider  Kind
}

type EmbeddingRequest struct {
	Model      string
	Content    []string
	Timeout    time.Duration
	MaxRetries int
}

type EmbeddingResponse struct {
	Vectors  [][]float64
	Usage    TokenUsage
	Model    string
	Provider Kind
}

type Provider interface {
	Kind() Kind
	Models() []Model
	Chat(context.Context, ChatRequest) (ChatResponse, error)
	Embed(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
}

type SecretRef struct {
	EnvName string
	StoreID string
	Raw     string
}

func (s SecretRef) Redacted() string {
	switch {
	case s.EnvName != "":
		return "env:" + s.EnvName
	case s.StoreID != "":
		return "secret:" + s.StoreID
	case s.Raw != "":
		return "[redacted]"
	default:
		return ""
	}
}

type Registry struct {
	mu        sync.RWMutex
	providers map[Kind]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: map[Kind]Provider{}}
	for _, provider := range providers {
		_ = r.Register(provider)
	}
	return r
}

func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Kind()] = provider
	return nil
}

func (r *Registry) Provider(kind Kind) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[kind]
	return provider, ok
}

func (r *Registry) ProviderForModel(model string, modelKind ModelKind) (Provider, Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, provider := range r.providers {
		for _, candidate := range provider.Models() {
			if candidate.ID == model && candidate.Kind == modelKind && !candidate.Deprecated {
				return provider, candidate, nil
			}
		}
	}
	return nil, Model{}, fmt.Errorf("%w: %s", ErrUnknownModel, model)
}

func BuiltinModels() []Model {
	return []Model{
		{ID: "gpt-4o", Label: "GPT-4o", Kind: ModelChat, Provider: KindOpenAI, TokenLimit: 128000, Temperature: true},
		{ID: "gpt-4.1", Label: "GPT-4.1", Kind: ModelChat, Provider: KindOpenAI, TokenLimit: 1047576, Temperature: true},
		{ID: "gpt-4.1-mini", Label: "GPT-4.1 Mini", Kind: ModelChat, Provider: KindOpenAI, TokenLimit: 1047576, Temperature: true},
		{ID: "gpt-5", Label: "GPT-5", Kind: ModelChat, Provider: KindOpenAI, TokenLimit: 400000},
		{ID: "gpt-5-mini", Label: "GPT-5 Mini", Kind: ModelChat, Provider: KindOpenAI, TokenLimit: 400000},
		{ID: "gpt-4", Label: "GPT-4", Kind: ModelChat, Provider: KindOpenAI, Deprecated: true, TokenLimit: 8192, Temperature: true},
		{ID: "gpt-3.5-turbo", Label: "GPT-3.5 Turbo", Kind: ModelChat, Provider: KindOpenAI, Deprecated: true, TokenLimit: 16385, Temperature: true},
		{ID: "text-embedding-3-small", Label: "Text Embedding 3 Small", Kind: ModelEmbedding, Provider: KindOpenAI, Dimensions: 1536},
		{ID: "gemini-2.5-pro", Label: "Gemini 2.5 Pro", Kind: ModelChat, Provider: KindGemini, TokenLimit: 1048576, Temperature: true},
		{ID: "gemini-2.5-flash", Label: "Gemini 2.5 Flash", Kind: ModelChat, Provider: KindGemini, TokenLimit: 1048576, Temperature: true},
		{ID: "gemini-1.5-pro", Label: "Gemini 1.5 Pro", Kind: ModelChat, Provider: KindGemini, Deprecated: true, TokenLimit: 1048576, Temperature: true},
		{ID: "gemini-1.5-flash", Label: "Gemini 1.5 Flash", Kind: ModelChat, Provider: KindGemini, Deprecated: true, TokenLimit: 1048576, Temperature: true},
		{ID: "gemini-embedding-001", Label: "Gemini Embedding", Kind: ModelEmbedding, Provider: KindGemini, Dimensions: 1536},
	}
}

type MockProvider struct {
	ChatText        string
	EmbeddingVector []float64
	models          []Model
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		ChatText:        "mock response",
		EmbeddingVector: []float64{1, 0, 0},
		models: []Model{
			{ID: "mock-chat", Label: "Mock Chat", Kind: ModelChat, Provider: KindMock, TokenLimit: 4096, Temperature: true},
			{ID: "mock-embedding", Label: "Mock Embedding", Kind: ModelEmbedding, Provider: KindMock, Dimensions: 3},
		},
	}
}

func (m *MockProvider) Kind() Kind {
	return KindMock
}

func (m *MockProvider) Models() []Model {
	return append([]Model(nil), m.models...)
}

func (m *MockProvider) Chat(_ context.Context, request ChatRequest) (ChatResponse, error) {
	content := strings.Join(append(append([]string{}, request.SystemPrompts...), request.UserPrompts...), " ")
	inputTokens := tokenEstimate(content)
	outputTokens := tokenEstimate(m.ChatText)
	return ChatResponse{
		Text:     m.ChatText,
		Usage:    TokenUsage{InputTokens: inputTokens, OutputTokens: outputTokens, TotalTokens: inputTokens + outputTokens},
		Model:    request.Model,
		Provider: KindMock,
	}, nil
}

func (m *MockProvider) Embed(_ context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	vectors := make([][]float64, 0, len(request.Content))
	for range request.Content {
		vectors = append(vectors, append([]float64(nil), m.EmbeddingVector...))
	}
	return EmbeddingResponse{
		Vectors:  vectors,
		Usage:    TokenUsage{InputTokens: tokenEstimate(strings.Join(request.Content, " ")), TotalTokens: tokenEstimate(strings.Join(request.Content, " "))},
		Model:    request.Model,
		Provider: KindMock,
	}, nil
}

func filterModels(kind Kind) []Model {
	var out []Model
	for _, model := range BuiltinModels() {
		if model.Provider == kind {
			out = append(out, model)
		}
	}
	return out
}

func tokenEstimate(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
