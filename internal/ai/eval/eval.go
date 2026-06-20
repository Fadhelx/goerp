package eval

import (
	"context"
	"fmt"
	"strings"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/audit"
	"gorp/internal/ai/providers"
)

type Case struct {
	Name           string
	Prompt         string
	Context        []string
	WantContains   string
	WantMaxTokens  int
	WantToolDenied bool
}

type Result struct {
	Name   string
	Passed bool
	Text   string
	Usage  providers.TokenUsage
	Error  string
}

type Harness struct {
	Agent    agents.Agent
	Provider providers.Provider
	Audit    *audit.Log
}

func (h Harness) Run(ctx context.Context, cases []Case) []Result {
	results := make([]Result, 0, len(cases))
	for _, tc := range cases {
		response, err := agents.Generate(ctx, h.Agent, h.Provider, agents.Request{Prompt: tc.Prompt, Context: tc.Context, MaxOutput: tc.WantMaxTokens})
		result := Result{Name: tc.Name}
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Text = response.Text
			result.Usage = response.Usage
			result.Passed = matches(tc, response)
		}
		if h.Audit != nil {
			h.Audit.Append(audit.Event{
				AgentID:          h.Agent.ID,
				Model:            h.Agent.Model,
				InputTokens:      result.Usage.InputTokens,
				OutputTokens:     result.Usage.OutputTokens,
				PermissionResult: permissionResult(result.Passed),
				Error:            result.Error,
				Details:          map[string]string{"eval_case": tc.Name},
			})
		}
		results = append(results, result)
	}
	return results
}

func matches(tc Case, response agents.Response) bool {
	if tc.WantContains != "" && !strings.Contains(response.Text, tc.WantContains) {
		return false
	}
	if tc.WantMaxTokens > 0 && response.Usage.OutputTokens > tc.WantMaxTokens {
		return false
	}
	return true
}

func RequirePass(results []Result) error {
	for _, result := range results {
		if !result.Passed {
			if result.Error != "" {
				return fmt.Errorf("eval %s failed: %s", result.Name, result.Error)
			}
			return fmt.Errorf("eval %s failed", result.Name)
		}
	}
	return nil
}

func permissionResult(passed bool) string {
	if passed {
		return "allowed"
	}
	return "denied"
}
