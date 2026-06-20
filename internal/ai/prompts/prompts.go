package prompts

import (
	"fmt"
	"strings"
)

type Button struct {
	ID        int64
	Name      string
	Prompt    string
	ModelName string
	UseInAI   bool
	Sequence  int
	Active    bool
}

func (b Button) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("prompt button requires name")
	}
	if strings.TrimSpace(b.Prompt) == "" {
		return fmt.Errorf("prompt button requires prompt")
	}
	if !b.Active {
		return fmt.Errorf("prompt button inactive")
	}
	return nil
}

func Build(system string, context []string, user string) []string {
	var parts []string
	if strings.TrimSpace(system) != "" {
		parts = append(parts, strings.TrimSpace(system))
	}
	for _, item := range context {
		if strings.TrimSpace(item) != "" {
			parts = append(parts, strings.TrimSpace(item))
		}
	}
	if strings.TrimSpace(user) != "" {
		parts = append(parts, strings.TrimSpace(user))
	}
	return parts
}
