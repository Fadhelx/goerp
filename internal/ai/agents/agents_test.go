package agents

import (
	"context"
	"strings"
	"testing"

	"gorp/internal/ai/providers"
)

func TestGenerateUsesMockProviderWithoutDuplicateUserPrompt(t *testing.T) {
	provider := providers.NewMockProvider()
	agent := Agent{
		ID:           1,
		Name:         "Advisor",
		Purpose:      "Answer with internal policy.",
		SystemPrompt: "Be concise.",
		Model:        "mock-chat",
		Active:       true,
	}
	response, err := Generate(context.Background(), agent, provider, Request{
		Prompt:  "second question",
		Context: []string{"source chunk"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "mock response" || response.Usage.TotalTokens == 0 {
		t.Fatalf("response = %+v", response)
	}
}

func TestAgentToolAllowlist(t *testing.T) {
	agent := Agent{ToolAllowlist: []string{"mail.message_post"}}
	if !AllowsTool(agent, "mail.message_post") {
		t.Fatal("expected allowed tool")
	}
	if AllowsTool(agent, "raw.sql") {
		t.Fatal("raw sql must not be implicitly allowed")
	}
}

func TestGenerateAddsRestrictToSourcesInstruction(t *testing.T) {
	provider := &captureProvider{}
	_, err := Generate(context.Background(), Agent{Name: "Advisor", Model: "mock-chat", Active: true, RestrictToSources: true}, provider, Request{Prompt: "question"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(provider.request.SystemPrompts, "\n")
	if !strings.Contains(joined, "Use only the provided source context") {
		t.Fatalf("system prompts = %+v", provider.request.SystemPrompts)
	}
}

func TestGenerateRunsToolLoopAndFeedsResults(t *testing.T) {
	provider := &toolLoopProvider{responses: []providers.ChatResponse{
		{Model: "mock-chat", ToolCalls: []providers.ToolCall{{
			Name:      "lookup_partner",
			Arguments: map[string]any{"name": "Ada"},
		}}},
		{Model: "mock-chat", Text: "Done"},
	}}
	toolCalls := 0
	response, err := Generate(context.Background(), Agent{Name: "Advisor", Model: "mock-chat", Active: true}, provider, Request{
		Prompt: "Find Ada",
		Tools: []providers.ToolCall{{
			Name:        "lookup_partner",
			Description: "Lookup a partner.",
		}},
		ToolHandler: ToolHandlerFunc(func(_ context.Context, call providers.ToolCall, _ Request) (map[string]any, error) {
			toolCalls++
			if call.Name != "lookup_partner" || call.Arguments["name"] != "Ada" {
				t.Fatalf("tool call = %+v", call)
			}
			return map[string]any{"name": "Ada Lovelace"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "Done" || toolCalls != 1 || len(provider.requests) != 2 {
		t.Fatalf("response=%+v toolCalls=%d requests=%d", response, toolCalls, len(provider.requests))
	}
	if len(provider.requests[0].Tools) != 1 || provider.requests[0].Tools[0].Name != "lookup_partner" {
		t.Fatalf("tools = %+v", provider.requests[0].Tools)
	}
	if len(provider.requests[1].Messages) == 0 || !strings.Contains(provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content, "Tool result for lookup_partner") {
		t.Fatalf("second request messages = %+v", provider.requests[1].Messages)
	}
}

func TestGenerateStopsOnEndMessageFromTool(t *testing.T) {
	provider := &toolLoopProvider{responses: []providers.ChatResponse{{
		Model: "mock-chat",
		ToolCalls: []providers.ToolCall{{
			Name:      "open_menu",
			Arguments: map[string]any{"__end_message": "Opened."},
		}},
	}}}
	response, err := Generate(context.Background(), Agent{Name: "Advisor", Model: "mock-chat", Active: true}, provider, Request{
		Prompt: "Open partners",
		Tools:  []providers.ToolCall{{Name: "open_menu"}},
		ToolHandler: ToolHandlerFunc(func(_ context.Context, _ providers.ToolCall, _ Request) (map[string]any, error) {
			return map[string]any{"__end_message": "Opened."}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "Opened." || len(provider.requests) != 1 {
		t.Fatalf("response=%+v requests=%d", response, len(provider.requests))
	}
}

func TestGenerateEnforcesTotalToolCallLimit(t *testing.T) {
	provider := &toolLoopProvider{responses: []providers.ChatResponse{{
		Model: "mock-chat",
		ToolCalls: []providers.ToolCall{
			{Name: "one"},
			{Name: "two"},
		},
	}}}
	_, err := Generate(context.Background(), Agent{Name: "Advisor", Model: "mock-chat", Active: true}, provider, Request{
		Prompt:       "Run tools",
		Tools:        []providers.ToolCall{{Name: "one"}, {Name: "two"}},
		MaxToolCalls: 1,
		ToolHandler: ToolHandlerFunc(func(context.Context, providers.ToolCall, Request) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "tool call limit") {
		t.Fatalf("expected tool limit error, got %v", err)
	}
}

type captureProvider struct {
	request providers.ChatRequest
}

func (p *captureProvider) Kind() providers.Kind {
	return providers.KindMock
}

func (p *captureProvider) Models() []providers.Model {
	return []providers.Model{{ID: "mock-chat", Kind: providers.ModelChat, Provider: providers.KindMock}}
}

func (p *captureProvider) Chat(_ context.Context, request providers.ChatRequest) (providers.ChatResponse, error) {
	p.request = request
	return providers.ChatResponse{Text: "ok", Model: request.Model}, nil
}

func (p *captureProvider) Embed(context.Context, providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	return providers.EmbeddingResponse{}, nil
}

type toolLoopProvider struct {
	responses []providers.ChatResponse
	requests  []providers.ChatRequest
}

func (p *toolLoopProvider) Kind() providers.Kind {
	return providers.KindMock
}

func (p *toolLoopProvider) Models() []providers.Model {
	return []providers.Model{{ID: "mock-chat", Kind: providers.ModelChat, Provider: providers.KindMock}}
}

func (p *toolLoopProvider) Chat(_ context.Context, request providers.ChatRequest) (providers.ChatResponse, error) {
	index := len(p.requests)
	p.requests = append(p.requests, request)
	if index < len(p.responses) {
		return p.responses[index], nil
	}
	return providers.ChatResponse{Text: "ok", Model: request.Model}, nil
}

func (p *toolLoopProvider) Embed(context.Context, providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	return providers.EmbeddingResponse{}, nil
}
