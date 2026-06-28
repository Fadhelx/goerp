package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openAIBaseURL          = "https://api.openai.com/v1"
	geminiOpenAIBaseURL    = "https://generativelanguage.googleapis.com/v1beta/openai"
	geminiGenerateBaseURL  = "https://generativelanguage.googleapis.com/v1beta"
	defaultProviderTimeout = 30 * time.Second
	maxProviderRetries     = 5
)

var ErrProviderRequest = errors.New("ai provider request failed")

type ProviderHTTPError struct {
	Provider  Kind
	Operation string
	Status    int
	Reason    string
}

func (e ProviderHTTPError) Error() string {
	operation := firstNonEmpty(e.Operation, "request")
	if e.Status != 0 {
		return fmt.Sprintf("%s provider %s failed with status %d", e.Provider, operation, e.Status)
	}
	if strings.TrimSpace(e.Reason) != "" {
		return fmt.Sprintf("%s provider %s failed: %s", e.Provider, operation, e.Reason)
	}
	return fmt.Sprintf("%s provider %s failed", e.Provider, operation)
}

func (e ProviderHTTPError) Unwrap() error {
	return ErrProviderRequest
}

type CompatibleProvider struct {
	ProviderKind Kind
	Secret       SecretRef
	Resolver     SecretResolver
	BaseURL      string
	GenerateURL  string
	HTTPClient   *http.Client
	models       []Model
}

type SecretResolver func(context.Context, SecretRef) (string, error)

type CompatibleOption func(*CompatibleProvider)

func WithBaseURL(baseURL string) CompatibleOption {
	return func(p *CompatibleProvider) {
		p.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithGenerateBaseURL(baseURL string) CompatibleOption {
	return func(p *CompatibleProvider) {
		p.GenerateURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithHTTPClient(client *http.Client) CompatibleOption {
	return func(p *CompatibleProvider) {
		if client != nil {
			p.HTTPClient = client
		}
	}
}

func WithSecretResolver(resolver SecretResolver) CompatibleOption {
	return func(p *CompatibleProvider) {
		p.Resolver = resolver
	}
}

func NewOpenAICompatible(secret SecretRef, opts ...CompatibleOption) *CompatibleProvider {
	provider := &CompatibleProvider{
		ProviderKind: KindOpenAI,
		Secret:       secret,
		BaseURL:      openAIBaseURL,
		HTTPClient:   &http.Client{Timeout: defaultProviderTimeout},
		models:       filterModels(KindOpenAI),
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

func NewGeminiCompatible(secret SecretRef, opts ...CompatibleOption) *CompatibleProvider {
	provider := &CompatibleProvider{
		ProviderKind: KindGemini,
		Secret:       secret,
		BaseURL:      geminiOpenAIBaseURL,
		GenerateURL:  geminiGenerateBaseURL,
		HTTPClient:   &http.Client{Timeout: defaultProviderTimeout},
		models:       filterModels(KindGemini),
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

func (p *CompatibleProvider) Kind() Kind {
	return p.ProviderKind
}

func (p *CompatibleProvider) Models() []Model {
	return append([]Model(nil), p.models...)
}

func (p *CompatibleProvider) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	switch p.ProviderKind {
	case KindOpenAI:
		return p.openAIChat(ctx, request)
	case KindGemini:
		return p.geminiChat(ctx, request)
	default:
		return ChatResponse{}, ErrUnknownProvider
	}
}

func (p *CompatibleProvider) Embed(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	secret, err := p.secret(ctx)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	body := map[string]any{
		"input": request.Content,
		"model": request.Model,
	}
	var response struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := p.postJSONWithRetry(ctx, "embedding", p.BaseURL+"/embeddings", p.authHeaders(secret), body, timeoutWithFallback(request.Timeout), request.MaxRetries, &response); err != nil {
		return EmbeddingResponse{}, err
	}
	vectors := make([][]float64, 0, len(response.Data))
	for _, item := range response.Data {
		vectors = append(vectors, append([]float64(nil), item.Embedding...))
	}
	model := firstNonEmpty(response.Model, request.Model)
	return EmbeddingResponse{
		Vectors:  vectors,
		Model:    model,
		Provider: p.ProviderKind,
		Usage: TokenUsage{
			InputTokens: response.Usage.PromptTokens,
			TotalTokens: response.Usage.TotalTokens,
		},
	}, nil
}

func (p *CompatibleProvider) openAIChat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	secret, err := p.secret(ctx)
	if err != nil {
		return ChatResponse{}, err
	}
	input := []map[string]any{}
	if len(request.SystemPrompts) > 0 {
		content := make([]map[string]string, 0, len(request.SystemPrompts))
		for _, prompt := range request.SystemPrompts {
			if strings.TrimSpace(prompt) != "" {
				content = append(content, map[string]string{"type": "input_text", "text": prompt})
			}
		}
		if len(content) > 0 {
			input = append(input, map[string]any{"role": "system", "content": content})
		}
	}
	for _, message := range request.Messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		role := message.Role
		if role == "" {
			role = "user"
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": []map[string]string{{"type": "input_text", "text": message.Content}},
		})
	}
	if len(request.UserPrompts) > 0 {
		content := make([]map[string]string, 0, len(request.UserPrompts))
		for _, prompt := range request.UserPrompts {
			if strings.TrimSpace(prompt) != "" {
				content = append(content, map[string]string{"type": "input_text", "text": prompt})
			}
		}
		if len(content) > 0 {
			input = append(input, map[string]any{"role": "user", "content": content})
		}
	}
	body := map[string]any{
		"model": request.Model,
		"input": input,
		"store": false,
	}
	if tools := openAITools(request.Tools); len(tools) > 0 {
		body["tools"] = tools
	}
	if request.MaxOutputToken > 0 {
		body["max_output_tokens"] = request.MaxOutputToken
	}
	if request.Model != "gpt-5" && request.Model != "gpt-5-mini" {
		body["temperature"] = 0.2
	}
	var response struct {
		Output []map[string]any `json:"output"`
		Model  string           `json:"model"`
		Usage  struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := p.postJSONWithRetry(ctx, "chat", p.BaseURL+"/responses", p.authHeaders(secret), body, timeoutWithFallback(request.Timeout), request.MaxRetries, &response); err != nil {
		return ChatResponse{}, err
	}
	text, tools := parseOpenAIOutput(response.Output)
	return ChatResponse{
		Text:      text,
		ToolCalls: tools,
		Model:     firstNonEmpty(response.Model, request.Model),
		Provider:  p.ProviderKind,
		Usage: TokenUsage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
	}, nil
}

func (p *CompatibleProvider) geminiChat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	secret, err := p.secret(ctx)
	if err != nil {
		return ChatResponse{}, err
	}
	body := map[string]any{
		"contents":         geminiContents(request.Messages, request.UserPrompts),
		"generationConfig": map[string]any{"temperature": 0.2},
	}
	if tools := geminiTools(request.Tools); len(tools) > 0 {
		body["tools"] = []map[string]any{{"functionDeclarations": tools}}
	}
	if len(request.SystemPrompts) > 0 {
		parts := []map[string]string{}
		for _, prompt := range request.SystemPrompts {
			if strings.TrimSpace(prompt) != "" {
				parts = append(parts, map[string]string{"text": prompt})
			}
		}
		if len(parts) > 0 {
			body["systemInstruction"] = map[string]any{"parts": parts}
		}
	}
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []map[string]any `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Usage struct {
			PromptTokens    int `json:"promptTokenCount"`
			CandidateTokens int `json:"candidatesTokenCount"`
			CachedTokens    int `json:"cachedContentTokenCount"`
			TotalTokenCount int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	url := strings.TrimRight(firstNonEmpty(p.GenerateURL, geminiGenerateBaseURL), "/") + "/models/" + request.Model + ":generateContent"
	if err := p.postJSONWithRetry(ctx, "chat", url, map[string]string{"x-goog-api-key": secret}, body, timeoutWithFallback(request.Timeout), request.MaxRetries, &response); err != nil {
		return ChatResponse{}, err
	}
	text, tools := parseGeminiCandidates(response.Candidates)
	total := response.Usage.TotalTokenCount
	if total == 0 {
		total = response.Usage.PromptTokens + response.Usage.CandidateTokens
	}
	return ChatResponse{
		Text:      text,
		ToolCalls: tools,
		Model:     request.Model,
		Provider:  p.ProviderKind,
		Usage: TokenUsage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CandidateTokens,
			TotalTokens:  total,
		},
	}, nil
}

func (p *CompatibleProvider) secret(ctx context.Context) (string, error) {
	if p.Resolver != nil {
		return p.Resolver(ctx, p.Secret)
	}
	return DefaultSecretResolver(ctx, p.Secret)
}

func DefaultSecretResolver(_ context.Context, ref SecretRef) (string, error) {
	switch {
	case strings.TrimSpace(ref.Raw) != "":
		return strings.TrimSpace(ref.Raw), nil
	case strings.TrimSpace(ref.EnvName) != "":
		value := strings.TrimSpace(os.Getenv(ref.EnvName))
		if value != "" {
			return value, nil
		}
	}
	return "", ErrSecretMissing
}

func (p *CompatibleProvider) authHeaders(secret string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + secret,
	}
}

func (p *CompatibleProvider) postJSON(ctx context.Context, operation string, url string, headers map[string]string, body map[string]any, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ProviderHTTPError{Provider: p.ProviderKind, Operation: operation, Reason: "invalid request"}
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultProviderTimeout}
	}
	response, err := client.Do(req)
	if err != nil {
		return ProviderHTTPError{Provider: p.ProviderKind, Operation: operation, Reason: "transport error"}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return ProviderHTTPError{Provider: p.ProviderKind, Operation: operation, Reason: "read error"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProviderHTTPError{Provider: p.ProviderKind, Operation: operation, Status: response.StatusCode}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return ProviderHTTPError{Provider: p.ProviderKind, Operation: operation, Reason: "invalid JSON response"}
	}
	return nil
}

func (p *CompatibleProvider) postJSONWithRetry(ctx context.Context, operation string, url string, headers map[string]string, body map[string]any, timeout time.Duration, maxRetries int, out any) error {
	retries := normalizedRetries(maxRetries)
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		err = p.postJSON(ctx, operation, url, headers, body, timeout, out)
		if err == nil {
			return nil
		}
		if attempt == retries || !retryableProviderError(err) {
			return err
		}
	}
	return err
}

func normalizedRetries(maxRetries int) int {
	if maxRetries <= 0 {
		return 0
	}
	if maxRetries > maxProviderRetries {
		return maxProviderRetries
	}
	return maxRetries
}

func retryableProviderError(err error) bool {
	var providerErr ProviderHTTPError
	if !errors.As(err, &providerErr) {
		return false
	}
	return providerErr.Status == http.StatusTooManyRequests ||
		providerErr.Status >= http.StatusInternalServerError ||
		(providerErr.Status == 0 && providerErr.Reason == "transport error")
}

func parseOpenAIOutput(output []map[string]any) (string, []ToolCall) {
	var texts []string
	var tools []ToolCall
	hasToolCalls := false
	for _, line := range output {
		if line["type"] == "function_call" {
			hasToolCalls = true
			id := firstNonEmpty(anyString(line["call_id"]), anyString(line["id"]))
			name, _ := line["name"].(string)
			args := map[string]any{}
			if raw, _ := line["arguments"].(string); raw != "" {
				_ = json.Unmarshal([]byte(raw), &args)
			}
			tools = append(tools, ToolCall{ID: id, Name: name, Arguments: args})
		}
	}
	if hasToolCalls {
		return "", tools
	}
	for _, line := range output {
		if text, _ := line["text"].(string); text != "" {
			texts = append(texts, text)
			continue
		}
		if line["type"] == "message" {
			for _, content := range anySlice(line["content"]) {
				item, _ := content.(map[string]any)
				if text, _ := item["text"].(string); text != "" {
					texts = append(texts, text)
				}
			}
		}
	}
	return strings.Join(texts, "\n"), tools
}

func parseGeminiCandidates(candidates []struct {
	Content struct {
		Parts []map[string]any `json:"parts"`
	} `json:"content"`
}) (string, []ToolCall) {
	var texts []string
	var tools []ToolCall
	hasToolCalls := false
	for _, candidate := range candidates {
		for _, part := range candidate.Content.Parts {
			call, _ := part["functionCall"].(map[string]any)
			if len(call) == 0 {
				continue
			}
			hasToolCalls = true
			name, _ := call["name"].(string)
			args, _ := call["args"].(map[string]any)
			tools = append(tools, ToolCall{Name: name, Arguments: cloneArgs(args)})
		}
	}
	if hasToolCalls {
		return "", tools
	}
	for _, candidate := range candidates {
		for _, part := range candidate.Content.Parts {
			if text, _ := part["text"].(string); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n"), tools
}

func geminiContents(messages []Message, prompts []string) []map[string]any {
	var out []map[string]any
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		role := "user"
		if message.Role == "assistant" || message.Role == "model" {
			role = "model"
		}
		out = append(out, map[string]any{"role": role, "parts": []map[string]string{{"text": message.Content}}})
	}
	for _, prompt := range prompts {
		if strings.TrimSpace(prompt) != "" {
			out = append(out, map[string]any{"role": "user", "parts": []map[string]string{{"text": prompt}}})
		}
	}
	return out
}

func openAITools(tools []ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		parameters := cloneMap(tool.Parameters)
		if len(parameters) == 0 {
			parameters = emptyParameterSchema()
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        name,
			"description": strings.TrimSpace(tool.Description),
			"parameters":  parameters,
		})
	}
	return out
}

func geminiTools(tools []ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		parameters := cloneMap(tool.Parameters)
		if len(parameters) == 0 {
			parameters = emptyParameterSchema()
		}
		out = append(out, map[string]any{
			"name":        name,
			"description": strings.TrimSpace(tool.Description),
			"parameters":  parameters,
		})
	}
	return out
}

func emptyParameterSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func timeoutWithFallback(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultProviderTimeout
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

func cloneArgs(args map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range args {
		out[key] = value
	}
	return out
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		if nested, ok := value.(map[string]any); ok {
			out[key] = cloneMap(nested)
			continue
		}
		if items, ok := value.([]any); ok {
			out[key] = append([]any(nil), items...)
			continue
		}
		out[key] = value
	}
	return out
}

func anyString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
