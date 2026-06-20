package http

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/record"
)

const (
	aiModelAgent          = "ai.agent"
	aiModelComposer       = "ai.composer"
	aiModelPromptButton   = "ai.prompt.button"
	aiModelDiscussChannel = "discuss.channel"
)

func (s Server) dispatchAIModelMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	switch req.Model {
	case aiModelDiscussChannel:
		if req.Method == "create_ai_draft_channel" {
			result, err := s.createAIDraftChannel(env, req)
			return result, true, err
		}
	case aiModelAgent:
		switch req.Method {
		case "action_ask_ai":
			result, err := s.actionAskAI(env, req)
			return result, true, err
		case "get_ask_ai_agent":
			result, err := askAIAgent(env)
			return result, true, err
		}
	}
	return nil, false, nil
}

func (s Server) createAIDraftChannel(env *record.Env, req callKWRequest) (map[string]any, error) {
	caller := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "caller_component"))))
	if caller == "" {
		return nil, fmt.Errorf("create_ai_draft_channel requires caller_component")
	}
	title := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "channel_title"))))
	recordModel := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "record_model"))))
	recordID := int64Value(firstNonNil(arg(req.Args, 3), kwarg(req.Kwargs, "record_id")))
	frontEndInfo := firstNonNil(arg(req.Args, 4), kwarg(req.Kwargs, "front_end_info"))
	textSelection := firstNonNil(arg(req.Args, 5), kwarg(req.Kwargs, "text_selection"))
	composer, err := aiComposerFor(env, caller, recordModel)
	if err != nil {
		return nil, err
	}
	agentID := int64Value(composer["ai_agent"])
	if agentID == 0 {
		return nil, fmt.Errorf("AI not reachable, AI Agent not found")
	}
	agentName, agentPartnerID, err := aiAgentIdentity(env, agentID)
	if err != nil {
		return nil, err
	}
	channelName := agentName
	if title != "" {
		channelName = "AI: " + title
	}
	contextParts := aiDraftContext(env, composer, recordModel, recordID, frontEndInfo, textSelection)
	channelID, err := createAIChatChannel(env, channelName, agentID, agentPartnerID, contextParts)
	if err != nil {
		return nil, err
	}
	prompts, err := aiComposerPromptNames(env, composer)
	if err != nil {
		return nil, err
	}
	modelHasThread := aiModelHasThread(env, recordModel)
	if caller == "chatter_ai_button" && !modelHasThread {
		prompts = filterChatterPrompts(prompts)
	}
	if len(prompts) > 3 {
		prompts = prompts[:3]
	}
	return map[string]any{
		"ai_channel_id":    channelID,
		"data":             aiChannelStorePayload(channelID, channelName, agentID),
		"prompts":          prompts,
		"model_has_thread": modelHasThread,
	}, nil
}

func (s Server) actionAskAI(env *record.Env, req callKWRequest) (map[string]any, error) {
	prompt := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "user_prompt"))))
	agent, err := askAIAgent(env)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("No configured Ask AI agent")
	}
	agentID := int64Value(agent["id"])
	agentName := firstNonEmptyHTTPString(stringValue(agent["name"]), "AI")
	agentPartnerID, _ := aiAgentPartnerID(env, agentID)
	channelID, err := createAIChatChannel(env, agentName, agentID, agentPartnerID, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type": "ir.actions.client",
		"tag":  "agent_chat_action",
		"params": map[string]any{
			"channelId":   channelID,
			"user_prompt": prompt,
		},
	}, nil
}

func aiComposerFor(env *record.Env, caller string, recordModel string) (map[string]any, error) {
	rows, err := allAIComposerRows(env)
	if err != nil {
		return nil, err
	}
	modelID := aiModelID(env, recordModel)
	var fallback map[string]any
	for _, row := range rows {
		if !boolHTTPWithFallback(row["active"], true) || stringValue(row["interface_key"]) != caller {
			continue
		}
		focused := int64Slice(row["focused_models"])
		if modelID != 0 && containsHTTPID(focused, modelID) {
			return row, nil
		}
		if len(focused) == 0 && fallback == nil {
			fallback = row
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("AI composer %q not found", caller)
}

func allAIComposerRows(env *record.Env) ([]map[string]any, error) {
	found, err := env.Model(aiModelComposer).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "interface_key", "focused_models", "ai_agent", "default_prompt", "available_prompt_ids", "available_prompts", "active")
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return int64Value(rows[i]["id"]) > int64Value(rows[j]["id"]) })
	return rows, nil
}

func aiDraftContext(env *record.Env, composer map[string]any, recordModel string, recordID int64, frontEndInfo any, textSelection any) []string {
	var parts []string
	if prompt := strings.TrimSpace(stringValue(composer["default_prompt"])); prompt != "" {
		parts = append(parts, prompt)
	}
	if recordModel != "" {
		label := fmt.Sprintf("Current record: %s", recordModel)
		if recordID != 0 {
			label = fmt.Sprintf("%s,%d", label, recordID)
			if rows, err := env.Model(recordModel).Browse(recordID).Read("name"); err == nil && len(rows) == 1 {
				if name := strings.TrimSpace(stringValue(rows[0]["name"])); name != "" {
					label += " " + name
				}
			}
		}
		parts = append(parts, label)
	}
	if text := strings.TrimSpace(stringValue(textSelection)); text != "" {
		parts = append(parts, "Selected text: "+text)
	}
	if encoded := compactJSON(frontEndInfo); encoded != "" {
		parts = append(parts, "Frontend context: "+encoded)
	}
	return parts
}

func createAIChatChannel(env *record.Env, channelName string, agentID int64, agentPartnerID int64, contextParts []string) (int64, error) {
	values := map[string]any{
		"name":         firstNonEmptyHTTPString(channelName, "AI"),
		"channel_type": "ai_chat",
		"active":       true,
		"ai_agent_id":  agentID,
	}
	if len(contextParts) > 0 {
		if encoded, err := json.Marshal(contextParts); err == nil {
			values["ai_env_context"] = string(encoded)
		}
	}
	channelID, err := env.Model(aiModelDiscussChannel).Create(values)
	if err != nil {
		return 0, err
	}
	if userID := env.Context().UserID; userID != 0 {
		if _, err := env.Model("discuss.channel.member").Create(map[string]any{"channel_id": channelID, "user_id": userID}); err != nil {
			return 0, err
		}
	}
	if agentPartnerID != 0 {
		if _, err := env.Model("discuss.channel.member").Create(map[string]any{"channel_id": channelID, "partner_id": agentPartnerID}); err != nil {
			return 0, err
		}
	}
	return channelID, nil
}

func askAIAgent(env *record.Env) (map[string]any, error) {
	found, err := env.Model(aiModelAgent).Search(domain.Cond("is_ask_ai_agent", domain.Equal, true))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "topic_ids", "partner_id", "active")
	if err != nil {
		return nil, err
	}
	var selected map[string]any
	for _, row := range rows {
		if !boolHTTPWithFallback(row["active"], true) {
			continue
		}
		if selected == nil || len(int64Slice(row["topic_ids"])) < len(int64Slice(selected["topic_ids"])) {
			selected = row
		}
	}
	if selected == nil {
		return nil, nil
	}
	return map[string]any{"id": int64Value(selected["id"]), "name": stringValue(selected["name"])}, nil
}

func aiAgentIdentity(env *record.Env, agentID int64) (string, int64, error) {
	rows, err := env.Model(aiModelAgent).Browse(agentID).Read("name", "partner_id")
	if err != nil {
		return "", 0, err
	}
	if len(rows) == 0 {
		return "", 0, fmt.Errorf("AI agent %d not found", agentID)
	}
	return firstNonEmptyHTTPString(stringValue(rows[0]["name"]), "AI"), int64Value(rows[0]["partner_id"]), nil
}

func aiAgentPartnerID(env *record.Env, agentID int64) (int64, error) {
	rows, err := env.Model(aiModelAgent).Browse(agentID).Read("partner_id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64Value(rows[0]["partner_id"]), nil
}

func aiComposerPromptNames(env *record.Env, composer map[string]any) ([]string, error) {
	ids := int64Slice(composer["available_prompts"])
	ids = append(ids, int64Slice(composer["available_prompt_ids"])...)
	if len(ids) == 0 {
		found, err := env.Model(aiModelPromptButton).Search(domain.Cond("composer_id", domain.Equal, int64Value(composer["id"])))
		if err != nil {
			return nil, err
		}
		rows, err := found.Read("name", "sequence", "active")
		if err != nil {
			return nil, err
		}
		return promptNamesFromRows(rows), nil
	}
	rows, err := env.Model(aiModelPromptButton).Browse(uniqueHTTPIDs(ids)...).Read("name", "sequence", "active")
	if err != nil {
		return nil, err
	}
	return promptNamesFromRows(rows), nil
}

func promptNamesFromRows(rows []map[string]any) []string {
	sort.SliceStable(rows, func(i, j int) bool {
		left := intHTTPWithFallback(rows[i]["sequence"], 100)
		right := intHTTPWithFallback(rows[j]["sequence"], 100)
		if left != right {
			return left < right
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if !boolHTTPWithFallback(row["active"], true) {
			continue
		}
		if name := strings.TrimSpace(stringValue(row["name"])); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func aiModelHasThread(env *record.Env, modelName string) bool {
	if strings.TrimSpace(modelName) == "" {
		return false
	}
	fields, err := env.Model(modelName).FieldsGet([]string{"message_ids"}, nil)
	if err != nil {
		return false
	}
	_, ok := fields["message_ids"]
	return ok
}

func aiModelID(env *record.Env, modelName string) int64 {
	if strings.TrimSpace(modelName) == "" {
		return 0
	}
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, modelName), record.SearchOptions{Limit: 1})
	if err != nil {
		return 0
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["id"])
}

func aiChannelStorePayload(channelID int64, channelName string, agentID int64) map[string]any {
	return map[string]any{
		"discuss.channel": []map[string]any{
			{
				"id":           channelID,
				"name":         channelName,
				"channel_type": "ai_chat",
				"ai_agent_id":  agentID,
				"is_member":    true,
			},
		},
	}
}

func filterChatterPrompts(prompts []string) []string {
	out := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		lower := strings.ToLower(prompt)
		if strings.Contains(lower, "chatter") || strings.Contains(lower, "followup") {
			continue
		}
		out = append(out, prompt)
	}
	return out
}

func compactJSON(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return text
		}
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) == "null" {
		return ""
	}
	return string(data)
}

func containsHTTPID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func uniqueHTTPIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func firstNonEmptyHTTPString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolHTTPWithFallback(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		if text == "" {
			return fallback
		}
		return text == "true" || text == "1" || text == "yes"
	default:
		return accountingBoolValue(value)
	}
}

func intHTTPWithFallback(value any, fallback int) int {
	if value == nil {
		return fallback
	}
	return intValue(value)
}
