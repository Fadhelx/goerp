package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	aiaddon "gorp/addons/ai"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/record"
)

type aiAuditLogEvent struct {
	Name             string
	EventType        string
	EventTime        time.Time
	UserID           int64
	CompanyID        int64
	AgentID          int64
	PromptID         int64
	ActionID         int64
	Provider         string
	Model            string
	ResModel         string
	ResID            int64
	InputTokens      int
	OutputTokens     int
	LatencyMillis    int64
	ToolNames        []string
	ToolCount        int
	PermissionResult string
	Status           string
	Error            string
	Metadata         map[string]any
}

func persistAIAuditLog(env *record.Env, event aiAuditLogEvent) {
	if env == nil {
		return
	}
	if event.EventTime.IsZero() {
		event.EventTime = time.Now().UTC()
	}
	if event.UserID == 0 {
		event.UserID = env.Context().UserID
	}
	if event.CompanyID == 0 {
		event.CompanyID = env.Context().CompanyID
	}
	if event.Status == "" {
		event.Status = "success"
	}
	if event.PermissionResult == "" {
		event.PermissionResult = "allowed"
	}
	if event.Name == "" {
		event.Name = strings.TrimSpace(event.EventType + " " + event.Status)
	}
	var companyID any
	if event.CompanyID != 0 {
		companyID = event.CompanyID
	}
	values := map[string]any{
		"name":              event.Name,
		"event_type":        event.EventType,
		"event_time":        event.EventTime,
		"user_id":           event.UserID,
		"company_id":        companyID,
		"agent_id":          event.AgentID,
		"prompt_id":         event.PromptID,
		"action_id":         event.ActionID,
		"provider":          event.Provider,
		"ai_model":          event.Model,
		"res_model":         event.ResModel,
		"res_id":            event.ResID,
		"input_tokens":      event.InputTokens,
		"output_tokens":     event.OutputTokens,
		"latency_millis":    int(event.LatencyMillis),
		"tool_names":        strings.Join(event.ToolNames, ","),
		"tool_count":        firstNonZeroInt(event.ToolCount, len(event.ToolNames)),
		"permission_result": event.PermissionResult,
		"status":            event.Status,
		"error":             aiAuditSafeError(event.Error),
		"metadata":          aiAuditMetadataJSON(event.Metadata),
	}
	_, _ = systemEnv(env).Model(aiaddon.ModelAuditLog).Create(values)
}

func aiAuditStatus(err error) string {
	if err != nil {
		return "failure"
	}
	return "success"
}

func aiAuditErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func aiAuditProviderKind(provider aiproviders.Provider) string {
	if provider == nil {
		return ""
	}
	return string(provider.Kind())
}

func aiAuditSafeError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return fmt.Sprint(aiAuditRedactValue(value))
}

func aiAuditMetadataJSON(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "{}"
	}
	data, err := json.Marshal(aiAuditRedactValue(metadata))
	if err != nil {
		return "{}"
	}
	if len(data) > 16384 {
		truncated, _ := json.Marshal(map[string]any{"truncated": true})
		return string(truncated)
	}
	return string(data)
}

func aiAuditRedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if aiAuditSensitiveKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = aiAuditRedactValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if aiAuditSensitiveKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = aiAuditRedactValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = aiAuditRedactValue(item)
		}
		return out
	case []string:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = aiAuditRedactValue(item)
		}
		return out
	case string:
		return aiAuditRedactString(typed)
	default:
		return typed
	}
}

func aiAuditSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, marker := range []string{"authorization", "credential", "password", "secret", "token", "api_key", "apikey", "private_key"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func aiAuditRedactString(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(trimmed, "sk-") ||
		strings.HasPrefix(lower, "bearer ") ||
		strings.Contains(lower, "api_key=") ||
		strings.Contains(lower, "access_token=") ||
		strings.Contains(lower, "password=") {
		return "[redacted]"
	}
	if len(value) > 512 {
		return value[:512] + "...[truncated]"
	}
	return value
}

func aiAuditToolNames(history []map[string]any) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, entry := range history {
		name := firstNonEmptyString(stringValue(entry["tool"]), stringValue(entry["action"]))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
