package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	serveractions "gorp/internal/actions"
	"gorp/internal/domain"
	"gorp/internal/notifications"
	"gorp/internal/record"
)

const (
	aiGoGetFields             = "ai.get_fields"
	aiGoSearch                = "ai.search"
	aiGoReadGroup             = "ai.read_group"
	aiGoGetMenuDetails        = "ai.get_menu_details"
	aiGoComputeReportMeasures = "ai.compute_report_measures"
	aiGoOpenMenuList          = "ai.open_menu_list"
	aiGoOpenMenuKanban        = "ai.open_menu_kanban"
	aiGoOpenMenuPivot         = "ai.open_menu_pivot"
	aiGoOpenMenuGraph         = "ai.open_menu_graph"
	aiGoAdjustSearch          = "ai.adjust_search"
)

func registerAIRuntimeToolActions(reg *serveractions.Registry, env *record.Env, bus *notifications.Bus) error {
	if reg == nil || env == nil {
		return nil
	}
	handlers := map[string]serveractions.GoAction{
		aiGoGetFields:             aiRuntimeGetFields(env),
		aiGoSearch:                aiRuntimeSearch(env),
		aiGoReadGroup:             aiRuntimeReadGroup(env),
		aiGoGetMenuDetails:        aiRuntimeGetMenuDetails(env),
		aiGoComputeReportMeasures: aiRuntimeComputeReportMeasures(env),
		aiGoOpenMenuList:          aiRuntimeOpenMenu(env, bus, "AI_OPEN_MENU_LIST", "list"),
		aiGoOpenMenuKanban:        aiRuntimeOpenMenu(env, bus, "AI_OPEN_MENU_KANBAN", "kanban"),
		aiGoOpenMenuPivot:         aiRuntimeOpenMenu(env, bus, "AI_OPEN_MENU_PIVOT", "pivot"),
		aiGoOpenMenuGraph:         aiRuntimeOpenMenu(env, bus, "AI_OPEN_MENU_GRAPH", "graph"),
		aiGoAdjustSearch:          aiRuntimeAdjustSearch(env, bus),
	}
	names := make([]string, 0, len(handlers))
	for name := range handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := reg.RegisterGo(name, handlers[name]); err != nil {
			return err
		}
	}
	return nil
}

func aiRuntimeGetFields(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		modelName := strings.TrimSpace(stringValue(exec.Values["model_name"]))
		if modelName == "" {
			return serveractions.Result{}, fmt.Errorf("model_name is required")
		}
		includeDescription := true
		if _, ok := exec.Values["include_description"]; ok {
			includeDescription = boolWithFallback(exec.Values["include_description"], false)
		}
		fields, err := env.Model(modelName).FieldsGet(nil, nil)
		if err != nil {
			return serveractions.Result{}, err
		}
		return aiToolResult(aiFieldsCSV(fields, includeDescription)), nil
	}
}

func aiRuntimeSearch(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		modelName := strings.TrimSpace(stringValue(exec.Values["model_name"]))
		if modelName == "" {
			return serveractions.Result{}, fmt.Errorf("model_name is required")
		}
		node, err := aiParseDomain(exec.Values["domain"])
		if err != nil {
			return serveractions.Result{}, err
		}
		searchEnv := aiRuntimeContextEnv(env, exec.Values)
		found, err := searchEnv.Model(modelName).SearchWithOptions(node, record.SearchOptions{
			Offset: intWithFallback(exec.Values["offset"], 0),
			Limit:  intWithFallback(exec.Values["limit"], 0),
			Order:  strings.TrimSpace(stringValue(exec.Values["order"])),
		})
		if err != nil {
			return serveractions.Result{}, err
		}
		rows, err := found.Read(stringSlice(exec.Values["fields"])...)
		if err != nil {
			return serveractions.Result{}, err
		}
		return aiToolResult(rows), nil
	}
}

func aiRuntimeReadGroup(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		modelName := strings.TrimSpace(stringValue(exec.Values["model_name"]))
		if modelName == "" {
			return serveractions.Result{}, fmt.Errorf("model_name is required")
		}
		node, err := aiParseDomain(exec.Values["domain"])
		if err != nil {
			return serveractions.Result{}, err
		}
		fields := append([]string{}, stringSlice(exec.Values["aggregates"])...)
		groupEnv := aiRuntimeContextEnv(env, exec.Values)
		rows, err := groupEnv.Model(modelName).ReadGroup(node, record.ReadGroupOptions{
			Fields:  fields,
			GroupBy: stringSlice(exec.Values["groupby"]),
			Order:   strings.TrimSpace(stringValue(exec.Values["order"])),
			Offset:  intWithFallback(exec.Values["offset"], 0),
			Limit:   intWithFallback(exec.Values["limit"], 0),
		})
		if err != nil {
			return serveractions.Result{}, err
		}
		return aiToolResult(rows), nil
	}
}

func aiRuntimeContextEnv(env *record.Env, values map[string]any) *record.Env {
	activeTest, ok := values["active_test"]
	if !ok {
		return env
	}
	ctx := env.Context()
	contextValues := map[string]any{}
	for key, value := range ctx.Values {
		contextValues[key] = value
	}
	contextValues["active_test"] = activeTest
	ctx.Values = contextValues
	return env.WithContext(ctx)
}

func aiRuntimeGetMenuDetails(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		menuIDs := int64Slice(exec.Values["menu_ids"])
		if len(menuIDs) == 0 {
			return serveractions.Result{}, fmt.Errorf("menu_ids is required")
		}
		var b strings.Builder
		b.WriteString("menu_id|model|context|domain|search_view\n")
		for _, menuID := range menuIDs {
			line, err := aiMenuDetailsLine(env, menuID)
			if err != nil {
				b.WriteString(fmt.Sprintf("%d|Error: %s|||\n", menuID, sanitizePipe(err.Error())))
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		return aiToolResult(strings.TrimSpace(b.String())), nil
	}
}

func aiRuntimeComputeReportMeasures(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		modelName := strings.TrimSpace(stringValue(exec.Values["model"]))
		if modelName == "" {
			modelName = strings.TrimSpace(stringValue(exec.Values["model_name"]))
		}
		if modelName == "" {
			return serveractions.Result{}, fmt.Errorf("model is required")
		}
		fields, err := env.Model(modelName).FieldsGet(nil, nil)
		if err != nil {
			return serveractions.Result{}, err
		}
		return aiToolResult(aiMeasuresCSV(fields)), nil
	}
}

func aiRuntimeOpenMenu(env *record.Env, bus *notifications.Bus, event string, viewType string) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		menuID := int64Value(exec.Values["menu_id"])
		modelName := strings.TrimSpace(stringValue(exec.Values["model_name"]))
		if menuID == 0 || modelName == "" {
			return serveractions.Result{}, fmt.Errorf("menu_id and model_name are required")
		}
		action, err := aiMenuAction(env, menuID)
		if err != nil {
			return serveractions.Result{}, err
		}
		if action.ResModel != modelName {
			return serveractions.Result{}, fmt.Errorf("menu model mismatch: %s != %s", action.ResModel, modelName)
		}
		if !aiContainsString(action.ViewModes, viewType) {
			return serveractions.Result{}, fmt.Errorf("%s view is not available for menu %d", viewType, menuID)
		}
		measures := stringSlice(exec.Values["measures"])
		if event == "AI_OPEN_MENU_PIVOT" {
			var sorted map[string]any
			measures, sorted, err = aiParseOrderedMeasures(measures)
			if err != nil {
				return serveractions.Result{}, err
			}
			if sorted != nil {
				exec.Values["sorted_column"] = sorted
			}
		}
		mode := strings.TrimSpace(stringValue(exec.Values["mode"]))
		order := strings.TrimSpace(stringValue(exec.Values["order"]))
		if event == "AI_OPEN_MENU_GRAPH" {
			if mode != "bar" && mode != "line" && mode != "pie" {
				return serveractions.Result{}, fmt.Errorf("invalid graph mode %q", mode)
			}
			if order != "ASC" && order != "DESC" {
				return serveractions.Result{}, fmt.Errorf("invalid graph order %q", order)
			}
		}
		payload := map[string]any{
			"event":              event,
			"menuID":             menuID,
			"model":              modelName,
			"selectedFilters":    stringSlice(exec.Values["selected_filters"]),
			"selectedGroupBys":   stringSlice(exec.Values["selected_groupbys"]),
			"rowGroupBys":        stringSlice(exec.Values["row_groupbys"]),
			"colGroupBys":        stringSlice(exec.Values["col_groupbys"]),
			"groupBys":           stringSlice(exec.Values["selected_groupbys"]),
			"measures":           measures,
			"measure":            strings.TrimSpace(stringValue(exec.Values["measure"])),
			"mode":               mode,
			"order":              order,
			"stacked":            boolWithFallback(exec.Values["stacked"], false),
			"cumulated":          boolWithFallback(exec.Values["cumulated"], false),
			"search":             stringSlice(exec.Values["search"]),
			"availableViewModes": action.ViewModes,
		}
		if sortedColumn, ok := exec.Values["sorted_column"].(map[string]any); ok {
			payload["sortedColumn"] = sortedColumn
		}
		if customDomain, ok, err := aiCustomDomainPayload(exec.Values["custom_domain"]); err != nil {
			return serveractions.Result{}, err
		} else if ok {
			payload["customDomain"] = customDomain
		}
		aiAttachSessionIdentifier(payload, exec.Metadata)
		aiPublishUserEvent(bus, env, exec, event, payload)
		return aiToolResult(payload), nil
	}
}

func aiRuntimeAdjustSearch(env *record.Env, bus *notifications.Bus) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		payload := map[string]any{
			"event":          "AI_ADJUST_SEARCH",
			"model":          firstNonEmptyString(strings.TrimSpace(stringValue(exec.Values["model_name"])), exec.Model),
			"removeFacets":   stringSlice(exec.Values["remove_facets"]),
			"toggleFilters":  stringSlice(exec.Values["toggle_filters"]),
			"toggleGroupBys": stringSlice(exec.Values["toggle_groupbys"]),
			"applySearches":  stringSlice(exec.Values["apply_searches"]),
			"measures":       stringSlice(exec.Values["measures"]),
			"mode":           strings.TrimSpace(stringValue(exec.Values["mode"])),
			"order":          firstNonEmptyString(strings.TrimSpace(stringValue(exec.Values["order"])), "ASC"),
			"stacked":        boolWithFallback(exec.Values["stacked"], false),
			"cumulated":      boolWithFallback(exec.Values["cumulated"], false),
			"switchViewType": strings.TrimSpace(stringValue(exec.Values["switch_view_type"])),
		}
		if customDomain, ok, err := aiCustomDomainPayload(exec.Values["custom_domain"]); err != nil {
			return serveractions.Result{}, err
		} else if ok {
			payload["customDomain"] = customDomain
		}
		aiAttachSessionIdentifier(payload, exec.Metadata)
		aiPublishUserEvent(bus, env, exec, "AI_ADJUST_SEARCH", payload)
		return aiToolResult(payload), nil
	}
}

func aiToolResult(value any) serveractions.Result {
	return serveractions.Result{Metadata: map[string]any{"result": value}}
}

func aiFieldsCSV(fields map[string]map[string]any, includeDescription bool) string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	if includeDescription {
		b.WriteString("field_name|display_name|type|sortable|groupable|description\n")
	} else {
		b.WriteString("field_name|display_name|type|sortable|groupable\n")
	}
	for _, name := range names {
		field := fields[name]
		if searchable, ok := field["searchable"].(bool); ok && !searchable {
			continue
		}
		line := []string{
			name,
			sanitizePipe(firstNonEmptyString(stringValue(field["string"]), name)),
			sanitizePipe(aiFieldType(field)),
			fmt.Sprint(boolWithFallback(field["sortable"], false)),
			fmt.Sprint(boolWithFallback(firstNonNil(field["groupable"], field["store"], field["sortable"]), false)),
		}
		if includeDescription {
			line = append(line, sanitizePipe(stringValue(field["help"])))
		}
		b.WriteString(strings.Join(line, "|"))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func aiFieldType(field map[string]any) string {
	fieldType := stringValue(field["type"])
	if relation := strings.TrimSpace(stringValue(field["relation"])); relation != "" {
		fieldType += "(" + relation + ")"
	}
	if fieldType == "selection" {
		if suffix := aiSelectionSuffix(field["selection"]); suffix != "" {
			fieldType += "(" + suffix + ")"
		}
	}
	return fieldType
}

func aiSelectionSuffix(value any) string {
	pairs := aiSelectionPairs(value)
	if len(pairs) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, fmt.Sprintf("'%s': '%s'", strings.ReplaceAll(pair[0], "'", "\\'"), strings.ReplaceAll(pair[1], "'", "\\'")))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func aiSelectionPairs(value any) [][2]string {
	switch typed := value.(type) {
	case [][2]string:
		return append([][2]string(nil), typed...)
	case []any:
		out := make([][2]string, 0, len(typed))
		for _, item := range typed {
			switch pair := item.(type) {
			case [2]string:
				out = append(out, pair)
			case []string:
				if len(pair) >= 2 {
					out = append(out, [2]string{pair[0], pair[1]})
				}
			case []any:
				if len(pair) >= 2 {
					out = append(out, [2]string{stringValue(pair[0]), stringValue(pair[1])})
				}
			}
		}
		return out
	default:
		return nil
	}
}

func aiMeasuresCSV(fields map[string]map[string]any) string {
	names := make([]string, 0, len(fields))
	for name, field := range fields {
		switch stringValue(field["type"]) {
		case "integer", "float", "monetary":
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("field_name|field_display_name|field_type|aggregator|sortable\n")
	b.WriteString("__count|Count|integer|count|false\n")
	for _, name := range names {
		field := fields[name]
		b.WriteString(strings.Join([]string{
			name,
			sanitizePipe(firstNonEmptyString(stringValue(field["string"]), name)),
			sanitizePipe(stringValue(field["type"])),
			"sum",
			fmt.Sprint(boolWithFallback(field["sortable"], false)),
		}, "|"))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

type aiActionWindow struct {
	ID        int64
	ResModel  string
	Context   string
	Domain    string
	ViewModes []string
}

func aiMenuDetailsLine(env *record.Env, menuID int64) (string, error) {
	action, err := aiMenuAction(env, menuID)
	if err != nil {
		return "", err
	}
	searchView := aiSearchViewArch(env, action.ResModel)
	return strings.Join([]string{
		fmt.Sprint(menuID),
		sanitizePipe(action.ResModel),
		sanitizePipe(action.Context),
		sanitizePipe(action.Domain),
		sanitizePipe(searchView),
	}, "|"), nil
}

func aiMenuAction(env *record.Env, menuID int64) (aiActionWindow, error) {
	rows, err := env.Model("ir.ui.menu").Browse(menuID).Read("action")
	if err != nil {
		return aiActionWindow{}, err
	}
	if len(rows) == 0 {
		return aiActionWindow{}, fmt.Errorf("menu not found")
	}
	actionID := aiActionIDFromValue(env, stringValue(rows[0]["action"]))
	if actionID == 0 {
		return aiActionWindow{}, fmt.Errorf("menu action not found")
	}
	actionRows, err := env.Model("ir.actions.act_window").Browse(actionID).Read("res_model", "context", "domain", "view_mode")
	if err != nil {
		return aiActionWindow{}, err
	}
	if len(actionRows) == 0 {
		return aiActionWindow{}, fmt.Errorf("action not found")
	}
	return aiActionWindow{
		ID:        actionID,
		ResModel:  stringValue(actionRows[0]["res_model"]),
		Context:   stringValue(actionRows[0]["context"]),
		Domain:    stringValue(actionRows[0]["domain"]),
		ViewModes: splitViewModes(stringValue(actionRows[0]["view_mode"])),
	}, nil
}

func aiSearchViewArch(env *record.Env, modelName string) string {
	found, err := env.Model("ir.ui.view").SearchWithOptions(domain.And(
		domain.Cond("model", domain.Equal, modelName),
		domain.Cond("type", domain.Equal, "search"),
	), record.SearchOptions{Limit: 1})
	if err != nil {
		return ""
	}
	rows, err := found.Read("arch")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.Join(strings.Fields(stringValue(rows[0]["arch"])), " ")
}

func aiActionIDFromValue(env *record.Env, value string) int64 {
	_, raw, ok := strings.Cut(value, ",")
	if !ok {
		raw = value
	}
	raw = strings.TrimSpace(raw)
	if id := int64Value(raw); id != 0 {
		return id
	}
	if raw == "" {
		return 0
	}
	found, err := env.Model("ir.model.data").SearchWithOptions(domain.Cond("complete_name", domain.Equal, raw), record.SearchOptions{Limit: 1})
	if err != nil {
		return 0
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["res_id"])
}

func aiParseDomain(value any) (domain.Node, error) {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return domain.And(), nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return domain.Node{}, fmt.Errorf("invalid domain JSON: %w", err)
	}
	return domain.Parse(decoded)
}

func aiCustomDomainPayload(value any) (any, bool, error) {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return nil, false, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, false, fmt.Errorf("invalid custom domain JSON: %w", err)
	}
	if _, err := domain.Parse(decoded); err != nil {
		return nil, false, err
	}
	return decoded, true, nil
}

func aiParseOrderedMeasures(measures []string) ([]string, map[string]any, error) {
	parsed := make([]string, 0, len(measures))
	var sorted map[string]any
	for _, raw := range measures {
		parts := strings.Fields(strings.TrimSpace(raw))
		if len(parts) == 0 {
			continue
		}
		measure := parts[0]
		if len(parts) > 1 {
			order := strings.ToLower(parts[1])
			if order != "asc" && order != "desc" {
				return nil, nil, fmt.Errorf("invalid ordering specification %q for measure %q", parts[1], measure)
			}
			if sorted == nil {
				sorted = map[string]any{"measure": measure, "order": order}
			}
		}
		parsed = append(parsed, measure)
	}
	return parsed, sorted, nil
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var decoded []string
		if err := json.Unmarshal([]byte(text), &decoded); err == nil {
			return decoded
		}
		return []string{text}
	default:
		return nil
	}
}

func splitViewModes(value string) []string {
	out := []string{}
	for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func aiContainsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func sanitizePipe(value string) string {
	value = strings.ReplaceAll(value, "|", "&#124;")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}

func aiAttachSessionIdentifier(payload map[string]any, metadata map[string]any) {
	sessionID := strings.TrimSpace(stringValue(metadata["ai_session_identifier"]))
	if sessionID != "" {
		payload["aiSessionIdentifier"] = sessionID
	}
}

func aiPublishUserEvent(bus *notifications.Bus, env *record.Env, exec serveractions.ExecutionContext, event string, payload map[string]any) {
	if bus == nil {
		return
	}
	userID := int64Value(firstNonNil(exec.Metadata["user_id"], envUserID(env)))
	if userID == 0 {
		return
	}
	busPayload := aiBusPayload(event, payload)
	bus.Publish(aiUserBusChannel(userID), event, busPayload, time.Now().UTC())
}

func aiBusPayload(event string, payload map[string]any) map[string]any {
	keys := []string{}
	switch event {
	case "AI_OPEN_MENU_LIST", "AI_OPEN_MENU_KANBAN":
		keys = []string{"menuID", "selectedFilters", "selectedGroupBys", "search", "customDomain", "aiSessionIdentifier"}
	case "AI_OPEN_MENU_PIVOT":
		keys = []string{"menuID", "model", "selectedFilters", "rowGroupBys", "colGroupBys", "measures", "search", "sortedColumn", "customDomain", "aiSessionIdentifier"}
	case "AI_OPEN_MENU_GRAPH":
		keys = []string{"menuID", "selectedFilters", "groupBys", "measure", "mode", "order", "stacked", "cumulated", "search", "customDomain", "aiSessionIdentifier"}
	case "AI_ADJUST_SEARCH":
		keys = []string{"removeFacets", "toggleFilters", "toggleGroupBys", "applySearches", "measures", "mode", "order", "stacked", "cumulated", "switchViewType", "customDomain", "aiSessionIdentifier"}
	default:
		return cloneAIMap(payload)
	}
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			out[key] = value
		}
	}
	return out
}

func envUserID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	return env.Context().UserID
}

func aiUserBusChannel(userID int64) string {
	return fmt.Sprintf("user/%d", userID)
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
