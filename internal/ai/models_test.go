package ai

import (
	"testing"

	"gorp/internal/registry"
)

func TestModels(t *testing.T) {
	reg := registry.New("test")
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ai.agent", "ai.topic", "ai.agent.source", "ai.prompt.button", "ai.embedding", "ai.composer"} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
}
