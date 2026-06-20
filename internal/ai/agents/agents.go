package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorp/internal/ai/providers"
)

type Agent struct {
	ID                int64
	Name              string
	Purpose           string
	SystemPrompt      string
	Model             string
	Active            bool
	RestrictToSources bool
	TopicIDs          []int64
	SourceIDs         []int64
	ToolAllowlist     []string
	CompanyID         int64
}

type Request struct {
	UserID       int64
	CompanyID    int64
	Prompt       string
	Context      []string
	MaxOutput    int
	CurrentTime  time.Time
	AllowedTools []string
	Conversation []providers.Message
	Tools        []providers.ToolCall
	ToolHandler  ToolHandler
	MaxToolCalls int
	ActiveModel  string
	ActiveID     int64
	SessionID    string
}

type Response struct {
	Text  string
	Usage providers.TokenUsage
	Model string
}

type ToolHandler interface {
	RunTool(context.Context, providers.ToolCall, Request) (map[string]any, error)
}

type ToolHandlerFunc func(context.Context, providers.ToolCall, Request) (map[string]any, error)

func (f ToolHandlerFunc) RunTool(ctx context.Context, call providers.ToolCall, request Request) (map[string]any, error) {
	return f(ctx, call, request)
}

func (a Agent) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("agent requires name")
	}
	if strings.TrimSpace(a.Model) == "" {
		return fmt.Errorf("agent requires model")
	}
	if !a.Active {
		return fmt.Errorf("agent inactive")
	}
	return nil
}

func Generate(ctx context.Context, agent Agent, provider providers.Provider, request Request) (Response, error) {
	if err := agent.Validate(); err != nil {
		return Response{}, err
	}
	if provider == nil {
		return Response{}, fmt.Errorf("provider missing")
	}
	system := []string{agent.SystemPrompt}
	if agent.Purpose != "" {
		system = append(system, "Purpose: "+agent.Purpose)
	}
	for _, item := range request.Context {
		if strings.TrimSpace(item) != "" {
			system = append(system, "Context: "+item)
		}
	}
	if agent.RestrictToSources {
		system = append(system, "Use only the provided source context. If no source information is provided, say that no source information has been provided for reference. Cite source chunks when used.")
	}
	messages := append([]providers.Message(nil), request.Conversation...)
	maxToolCalls := request.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = 20
	}
	totalToolCalls := 0
	for {
		chat, err := provider.Chat(ctx, providers.ChatRequest{
			Model:          agent.Model,
			SystemPrompts:  system,
			UserPrompts:    []string{request.Prompt},
			Messages:       messages,
			Tools:          append([]providers.ToolCall(nil), request.Tools...),
			MaxOutputToken: request.MaxOutput,
		})
		if err != nil {
			return Response{}, err
		}
		if len(chat.ToolCalls) == 0 || request.ToolHandler == nil || len(request.Tools) == 0 {
			return Response{Text: chat.Text, Usage: chat.Usage, Model: chat.Model}, nil
		}
		doneMessage := ""
		for _, call := range chat.ToolCalls {
			if totalToolCalls >= maxToolCalls {
				return Response{}, fmt.Errorf("ai tool call limit reached")
			}
			totalToolCalls++
			output, toolErr := request.ToolHandler.RunTool(ctx, call, request)
			if toolErr != nil {
				output = map[string]any{"error": toolErr.Error()}
			}
			if end, ok := output["__end_message"].(string); ok && strings.TrimSpace(end) != "" && toolErr == nil {
				doneMessage = strings.TrimSpace(end)
			}
			messages = append(messages, providers.Message{Role: "user", Content: toolResultContent(call.Name, output)})
		}
		if doneMessage != "" {
			return Response{Text: doneMessage, Usage: chat.Usage, Model: chat.Model}, nil
		}
	}
}

func toolResultContent(toolName string, output map[string]any) string {
	data, err := json.Marshal(output)
	if err != nil {
		data = []byte(`{"error":"invalid tool result"}`)
	}
	return "Tool result for " + toolName + ": " + string(data)
}

func GenerateOnce(ctx context.Context, agent Agent, provider providers.Provider, request Request) (Response, error) {
	if err := agent.Validate(); err != nil {
		return Response{}, err
	}
	if provider == nil {
		return Response{}, fmt.Errorf("provider missing")
	}
	system := []string{agent.SystemPrompt}
	if agent.Purpose != "" {
		system = append(system, "Purpose: "+agent.Purpose)
	}
	for _, item := range request.Context {
		if strings.TrimSpace(item) != "" {
			system = append(system, "Context: "+item)
		}
	}
	if agent.RestrictToSources {
		system = append(system, "Use only the provided source context. If no source information is provided, say that no source information has been provided for reference. Cite source chunks when used.")
	}
	chat, err := provider.Chat(ctx, providers.ChatRequest{
		Model:          agent.Model,
		SystemPrompts:  system,
		UserPrompts:    []string{request.Prompt},
		Messages:       append([]providers.Message(nil), request.Conversation...),
		MaxOutputToken: request.MaxOutput,
	})
	if err != nil {
		return Response{}, err
	}
	return Response{Text: chat.Text, Usage: chat.Usage, Model: chat.Model}, nil
}

func AllowsTool(agent Agent, toolName string) bool {
	if len(agent.ToolAllowlist) == 0 {
		return false
	}
	for _, allowed := range agent.ToolAllowlist {
		if allowed == toolName {
			return true
		}
	}
	return false
}
