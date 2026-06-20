package prompts

import "testing"

func TestPromptButtonAndBuild(t *testing.T) {
	button := Button{Name: "Summarize", Prompt: "Summarize this", Active: true}
	if err := button.Validate(); err != nil {
		t.Fatal(err)
	}
	parts := Build("system", []string{"", "context"}, "user")
	if len(parts) != 3 || parts[0] != "system" || parts[1] != "context" || parts[2] != "user" {
		t.Fatalf("parts = %+v", parts)
	}
}
