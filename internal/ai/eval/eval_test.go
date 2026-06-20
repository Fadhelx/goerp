package eval

import (
	"context"
	"testing"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/audit"
	"gorp/internal/ai/providers"
)

func TestGoldenEvalHarnessWithMockProvider(t *testing.T) {
	log := audit.NewLog()
	harness := Harness{
		Agent: agents.Agent{
			ID:           1,
			Name:         "Mock Agent",
			SystemPrompt: "Answer from approved sources.",
			Model:        "mock-chat",
			Active:       true,
		},
		Provider: providers.NewMockProvider(),
		Audit:    log,
	}
	results := harness.Run(context.Background(), []Case{
		{Name: "basic", Prompt: "Summarize source", Context: []string{"approved source"}, WantContains: "mock response", WantMaxTokens: 5},
	})
	if err := RequirePass(results); err != nil {
		t.Fatal(err)
	}
	events := log.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Model != "mock-chat" || events[0].PermissionResult != "allowed" {
		t.Fatalf("event = %+v", events[0])
	}
}
