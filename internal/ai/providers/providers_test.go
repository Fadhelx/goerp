package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderRegistryAndMock(t *testing.T) {
	mock := NewMockProvider()
	registry := NewRegistry(mock, NewOpenAICompatible(SecretRef{EnvName: "OPENAI_API_KEY"}), NewGeminiCompatible(SecretRef{StoreID: "gemini"}))

	provider, model, err := registry.ProviderForModel("mock-chat", ModelChat)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Kind() != KindMock || model.ID != "mock-chat" {
		t.Fatalf("provider=%s model=%+v", provider.Kind(), model)
	}
	response, err := provider.Chat(context.Background(), ChatRequest{Model: "mock-chat", SystemPrompts: []string{"system"}, UserPrompts: []string{"hello world"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "mock response" || response.Usage.TotalTokens == 0 {
		t.Fatalf("response = %+v", response)
	}

	embedder, embeddingModel, err := registry.ProviderForModel("mock-embedding", ModelEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	embedding, err := embedder.Embed(context.Background(), EmbeddingRequest{Model: embeddingModel.ID, Content: []string{"one", "two"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Vectors) != 2 || len(embedding.Vectors[0]) != 3 {
		t.Fatalf("embedding = %+v", embedding)
	}
}

func TestBuiltinModelInventoryAndSecretRedaction(t *testing.T) {
	models := BuiltinModels()
	required := map[string]bool{
		"gpt-4o":                 false,
		"gpt-4.1":                false,
		"gpt-4.1-mini":           false,
		"gpt-5":                  false,
		"gpt-5-mini":             false,
		"text-embedding-3-small": false,
		"gemini-2.5-pro":         false,
		"gemini-2.5-flash":       false,
		"gemini-embedding-001":   false,
		"gpt-3.5-turbo":          false,
		"gemini-1.5-flash":       false,
	}
	for _, model := range models {
		if _, ok := required[model.ID]; ok {
			required[model.ID] = true
		}
	}
	for model, seen := range required {
		if !seen {
			t.Fatalf("missing model %s", model)
		}
	}

	secret := SecretRef{Raw: "sk-live-secret"}
	if redacted := secret.Redacted(); redacted != "[redacted]" || strings.Contains(redacted, "sk-live-secret") {
		t.Fatalf("redacted = %q", redacted)
	}
	stub := NewOpenAICompatible(SecretRef{})
	_, err := stub.Chat(context.Background(), ChatRequest{Model: "gpt-4o"})
	if !errors.Is(err, ErrSecretMissing) {
		t.Fatalf("expected missing secret, got %v", err)
	}
}

func TestOpenAICompatibleChatAndEmbeddingHTTP(t *testing.T) {
	var sawResponses bool
	var sawEmbeddings bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatal("authorization header mismatch")
		}
		switch r.URL.Path {
		case "/responses":
			sawResponses = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "gpt-4o" || body["store"] != false {
				t.Fatalf("responses body = %#v", body)
			}
			tools, _ := body["tools"].([]any)
			if len(tools) != 1 {
				t.Fatalf("tools = %#v", body["tools"])
			}
			tool, _ := tools[0].(map[string]any)
			if tool["type"] != "function" || tool["name"] != "rename_partner" {
				t.Fatalf("tool = %#v", tool)
			}
			parameters, _ := tool["parameters"].(map[string]any)
			if parameters["type"] != "object" {
				t.Fatalf("parameters = %#v", parameters)
			}
			writeProviderJSON(t, w, map[string]any{
				"model": "gpt-4o",
				"output": []any{map[string]any{
					"type":    "message",
					"content": []any{map[string]any{"text": "OpenAI answer"}},
				}},
				"usage": map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
			})
		case "/embeddings":
			sawEmbeddings = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "text-embedding-3-small" {
				t.Fatalf("embedding body = %#v", body)
			}
			writeProviderJSON(t, w, map[string]any{
				"model": "text-embedding-3-small",
				"data":  []any{map[string]any{"embedding": []float64{0.1, 0.2}}},
				"usage": map[string]any{"prompt_tokens": 1, "total_tokens": 1},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewOpenAICompatible(SecretRef{Raw: "test-key"}, WithBaseURL(server.URL))
	chat, err := provider.Chat(context.Background(), ChatRequest{
		Model:         "gpt-4o",
		SystemPrompts: []string{"system"},
		Messages:      []Message{{Role: "assistant", Content: "history"}},
		UserPrompts:   []string{"question"},
		Tools: []ToolCall{{
			Name:        "rename_partner",
			Description: "Rename a partner.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chat.Text != "OpenAI answer" || chat.Model != "gpt-4o" || chat.Provider != KindOpenAI || chat.Usage.TotalTokens != 5 {
		t.Fatalf("chat = %+v", chat)
	}

	embedding, err := provider.Embed(context.Background(), EmbeddingRequest{Model: "text-embedding-3-small", Content: []string{"question"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Vectors) != 1 || len(embedding.Vectors[0]) != 2 || embedding.Provider != KindOpenAI || embedding.Usage.TotalTokens != 1 {
		t.Fatalf("embedding = %+v", embedding)
	}
	if !sawResponses || !sawEmbeddings {
		t.Fatalf("requests responses=%v embeddings=%v", sawResponses, sawEmbeddings)
	}
}

func TestGeminiCompatibleChatHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-2.5-flash:generateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Fatal("api key header mismatch")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["systemInstruction"] == nil || body["contents"] == nil {
			t.Fatalf("gemini body = %#v", body)
		}
		generation, _ := body["generationConfig"].(map[string]any)
		if generation["maxOutputTokens"] != float64(128) {
			t.Fatalf("gemini generation config = %#v", generation)
		}
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("gemini tools = %#v", body["tools"])
		}
		wrapper, _ := tools[0].(map[string]any)
		declarations, _ := wrapper["functionDeclarations"].([]any)
		if len(declarations) != 1 {
			t.Fatalf("gemini declarations = %#v", wrapper)
		}
		writeProviderJSON(t, w, map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{
					"parts": []any{map[string]any{"text": "Gemini answer"}},
				},
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 4, "candidatesTokenCount": 3, "totalTokenCount": 7},
		})
	}))
	defer server.Close()

	provider := NewGeminiCompatible(SecretRef{Raw: "gemini-key"}, WithGenerateBaseURL(server.URL))
	chat, err := provider.Chat(context.Background(), ChatRequest{
		Model:          "gemini-2.5-flash",
		SystemPrompts:  []string{"system"},
		Messages:       []Message{{Role: "assistant", Content: "history"}},
		UserPrompts:    []string{"question"},
		MaxOutputToken: 128,
		Tools: []ToolCall{{
			Name:        "lookup_policy",
			Description: "Look up a policy.",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chat.Text != "Gemini answer" || chat.Model != "gemini-2.5-flash" || chat.Provider != KindGemini || chat.Usage.TotalTokens != 7 {
		t.Fatalf("chat = %+v", chat)
	}
}

func TestGeminiCompatibleEmbeddingHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gemini-key" {
			t.Fatal("authorization header mismatch")
		}
		writeProviderJSON(t, w, map[string]any{
			"model": "gemini-embedding-001",
			"data":  []any{map[string]any{"embedding": []float64{0.3, 0.4, 0.5}}},
			"usage": map[string]any{"prompt_tokens": 2, "total_tokens": 2},
		})
	}))
	defer server.Close()

	provider := NewGeminiCompatible(SecretRef{Raw: "gemini-key"}, WithBaseURL(server.URL))
	embedding, err := provider.Embed(context.Background(), EmbeddingRequest{Model: "gemini-embedding-001", Content: []string{"question"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Vectors) != 1 || len(embedding.Vectors[0]) != 3 || embedding.Provider != KindGemini {
		t.Fatalf("embedding = %+v", embedding)
	}
}

func TestOpenAICompatibleRetriesTransientChatHTTP(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			writeProviderJSON(t, w, map[string]any{"error": map[string]any{"message": "temporary"}})
			return
		}
		writeProviderJSON(t, w, map[string]any{
			"model": "gpt-4o",
			"output": []any{map[string]any{
				"type":    "message",
				"content": []any{map[string]any{"text": "retry ok"}},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatible(SecretRef{Raw: "test-key"}, WithBaseURL(server.URL))
	chat, err := provider.Chat(context.Background(), ChatRequest{Model: "gpt-4o", UserPrompts: []string{"question"}, MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || chat.Text != "retry ok" {
		t.Fatalf("attempts=%d chat=%+v", attempts, chat)
	}
}

func TestCompatibleProviderDoesNotRetryBadRequest(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		writeProviderJSON(t, w, map[string]any{"error": map[string]any{"message": "bad request"}})
	}))
	defer server.Close()

	provider := NewOpenAICompatible(SecretRef{Raw: "test-key"}, WithBaseURL(server.URL))
	_, err := provider.Chat(context.Background(), ChatRequest{Model: "gpt-4o", UserPrompts: []string{"question"}, MaxRetries: 3})
	if !errors.Is(err, ErrProviderRequest) {
		t.Fatalf("expected provider request error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestCompatibleProviderRetriesEmbeddingRateLimitHTTP(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			writeProviderJSON(t, w, map[string]any{"error": map[string]any{"message": "rate limit"}})
			return
		}
		writeProviderJSON(t, w, map[string]any{
			"model": "text-embedding-3-small",
			"data":  []any{map[string]any{"embedding": []float64{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatible(SecretRef{Raw: "test-key"}, WithBaseURL(server.URL))
	embedding, err := provider.Embed(context.Background(), EmbeddingRequest{Model: "text-embedding-3-small", Content: []string{"question"}, MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(embedding.Vectors) != 1 || len(embedding.Vectors[0]) != 2 {
		t.Fatalf("attempts=%d embedding=%+v", attempts, embedding)
	}
}

func TestCompatibleProviderErrorDoesNotExposeSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeProviderJSON(t, w, map[string]any{"error": map[string]any{"message": "provider unavailable"}})
	}))
	defer server.Close()

	provider := NewOpenAICompatible(SecretRef{Raw: "secret-value"}, WithBaseURL(server.URL))
	_, err := provider.Chat(context.Background(), ChatRequest{Model: "gpt-4o", UserPrompts: []string{"question"}})
	if !errors.Is(err, ErrProviderRequest) {
		t.Fatalf("expected provider request error, got %v", err)
	}
	if !strings.Contains(err.Error(), "status 400") || strings.Contains(err.Error(), "provider unavailable") || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompatibleProviderUsesInjectedSecretResolver(t *testing.T) {
	var resolved SecretRef
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer resolved-key" {
			t.Fatal("authorization header mismatch")
		}
		writeProviderJSON(t, w, map[string]any{
			"model": "text-embedding-3-small",
			"data":  []any{map[string]any{"embedding": []float64{1, 2}}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatible(
		SecretRef{StoreID: "ai.openai_key"},
		WithBaseURL(server.URL),
		WithSecretResolver(func(_ context.Context, ref SecretRef) (string, error) {
			resolved = ref
			return "resolved-key", nil
		}),
	)
	_, err := provider.Embed(context.Background(), EmbeddingRequest{Model: "text-embedding-3-small", Content: []string{"hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.StoreID != "ai.openai_key" {
		t.Fatal("resolved reference mismatch")
	}
}

func writeProviderJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
