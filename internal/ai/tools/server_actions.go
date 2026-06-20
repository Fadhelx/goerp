package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorp/internal/actions"
)

const EndMessageKey = "__end_message"

type ActionRunner interface {
	Run(context.Context, int64, actions.ExecutionContext) (actions.Result, error)
}

func RegisterServerActionTools(registry *Registry, runner ActionRunner, serverActions ...actions.ServerAction) error {
	for _, serverAction := range serverActions {
		if !serverAction.UseInAI {
			continue
		}
		tool, err := ServerActionTool(serverAction, runner)
		if err != nil {
			return err
		}
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

func ServerActionTool(serverAction actions.ServerAction, runner ActionRunner) (Tool, error) {
	if !serverAction.UseInAI {
		return Tool{}, fmt.Errorf("%w: action %d is not enabled for AI", ErrToolForbidden, serverAction.ID)
	}
	if runner == nil {
		return Tool{}, fmt.Errorf("server action tool requires action runner")
	}
	schema, err := SchemaFromJSON(serverAction.AIToolSchema)
	if err != nil {
		return Tool{}, err
	}
	if serverAction.AIToolAllowEndMessage {
		schema = withEndMessage(schema)
	}
	if err := ValidateSchema(schema); err != nil {
		return Tool{}, err
	}
	name := ServerActionToolName(serverAction)
	return Tool{
		Name:        name,
		Description: firstNonEmpty(serverAction.AIToolDescription, serverAction.Name),
		Schema:      schema,
		Handler: func(ctx context.Context, request Request) (Result, error) {
			return runServerActionTool(ctx, runner, serverAction, request)
		},
	}, nil
}

func ServerActionToolName(serverAction actions.ServerAction) string {
	if xmlID, ok := serverAction.Metadata["xml_id"].(string); ok {
		parts := strings.Split(xmlID, ".")
		candidate := parts[len(parts)-1]
		if isToolName(candidate) {
			return candidate
		}
	}
	if serverAction.ID != 0 {
		return "action_" + strconv.FormatInt(serverAction.ID, 10)
	}
	return sanitizeToolName(serverAction.Name)
}

func SchemaFromJSON(raw string) (Schema, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Schema{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrSchemaValidation)
	}
	properties, err := objectMap(root["properties"])
	if err != nil {
		return nil, fmt.Errorf("%w: properties", ErrSchemaValidation)
	}
	required := stringSet(root["required"])
	schema := make(Schema, len(properties))
	keys := make([]string, 0, len(properties))
	for name := range properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		field, err := fieldFromJSON(name, properties[name])
		if err != nil {
			return nil, err
		}
		field.Required = required[name]
		schema[name] = field
	}
	return schema, ValidateSchema(schema)
}

func runServerActionTool(ctx context.Context, runner ActionRunner, serverAction actions.ServerAction, request Request) (Result, error) {
	values := cloneInput(request.Input)
	var endMessage string
	if value, ok := values[EndMessageKey]; ok {
		if text, ok := value.(string); ok {
			endMessage = text
		}
		delete(values, EndMessageKey)
	}
	exec := actions.ExecutionContext{
		Model:     firstNonEmpty(request.Model, serverAction.Model),
		RecordID:  request.RecordID,
		RecordIDs: append([]int64(nil), request.RecordIDs...),
		Values:    values,
		Trigger:   "ai",
		Metadata: map[string]any{
			"ai_tool":    true,
			"tool_name":  request.ToolName,
			"action_id":  serverAction.ID,
			"user_id":    request.UserID,
			"company_id": request.CompanyID,
		},
	}
	for key, value := range request.Metadata {
		if _, exists := exec.Metadata[key]; !exists {
			exec.Metadata[key] = value
		}
	}
	if endMessage != "" {
		exec.Metadata[EndMessageKey] = endMessage
	}
	for key, value := range serverAction.Metadata {
		if _, exists := exec.Metadata[key]; !exists {
			exec.Metadata[key] = value
		}
	}
	result, err := runner.Run(ctx, serverAction.ID, exec)
	output := actionResultOutput(result)
	if endMessage != "" {
		output[EndMessageKey] = endMessage
	}
	return Result{Output: output}, err
}

func actionResultOutput(result actions.Result) map[string]any {
	output := map[string]any{
		"action_id": result.ActionID,
		"kind":      string(result.Kind),
	}
	if result.GoActionName != "" {
		output["go_action_name"] = result.GoActionName
	}
	if result.CreatedID != 0 {
		output["created_id"] = result.CreatedID
	}
	if len(result.WrittenIDs) > 0 {
		output["written_ids"] = append([]int64(nil), result.WrittenIDs...)
	}
	if result.MailSent {
		output["mail_sent"] = true
	}
	if result.Enqueued {
		output["enqueued"] = true
	}
	if result.DisabledReason != "" {
		output["disabled_reason"] = result.DisabledReason
	}
	if len(result.Metadata) > 0 {
		output["metadata"] = cloneInput(result.Metadata)
	}
	return output
}

func withEndMessage(schema Schema) Schema {
	schema = cloneSchema(schema)
	field := schema[EndMessageKey]
	if field.Type == "" {
		field = Field{
			Type:        TypeString,
			Description: "Final assistant message when the tool call completes the request.",
			MaxLength:   2048,
		}
	}
	field.Required = true
	schema[EndMessageKey] = field
	return schema
}

func fieldFromJSON(name string, value any) (Field, error) {
	data, ok := value.(map[string]any)
	if !ok {
		return Field{}, fmt.Errorf("%w: property %s", ErrSchemaValidation, name)
	}
	field := Field{
		Type:        schemaType(data["type"]),
		Description: stringValue(data["description"]),
		Pattern:     stringValue(data["pattern"]),
		MaxLength:   intValue(data["maxLength"]),
		Enum:        anySlice(data["enum"]),
	}
	if field.Type == "" && data["anyOf"] != nil {
		field.Type = TypeArray
	}
	if field.Type == TypeArray {
		if items, ok := data["items"].(map[string]any); ok {
			item, err := fieldFromJSON(name, items)
			if err != nil {
				return Field{}, err
			}
			field.Items = &item
		}
		for _, item := range anySlice(data["anyOf"]) {
			option, err := fieldFromJSON(name, item)
			if err != nil {
				return Field{}, err
			}
			field.AnyOf = append(field.AnyOf, option)
		}
	}
	if field.Type == TypeObject {
		properties, err := objectMap(data["properties"])
		if err != nil {
			return Field{}, fmt.Errorf("%w: object %s properties", ErrSchemaValidation, name)
		}
		required := stringSet(data["required"])
		field.Properties = make(Schema, len(properties))
		field.RequiredProperties = sortedKeys(required)
		for propertyName, propertyValue := range properties {
			property, err := fieldFromJSON(propertyName, propertyValue)
			if err != nil {
				return Field{}, err
			}
			property.Required = required[propertyName]
			field.Properties[propertyName] = property
		}
	}
	return field, nil
}

func objectMap(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	out, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not an object")
	}
	return out, nil
}

func schemaType(value any) SchemaType {
	switch typed := value.(type) {
	case string:
		return SchemaType(typed)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && text != string(TypeNull) {
				return SchemaType(text)
			}
		}
	}
	return ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func anySlice(value any) []any {
	if value == nil {
		return nil
	}
	out, ok := value.([]any)
	if !ok {
		return nil
	}
	return out
}

func stringSet(value any) map[string]bool {
	out := map[string]bool{}
	for _, item := range anySlice(value) {
		if text, ok := item.(string); ok {
			out[text] = true
		}
	}
	return out
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneInput(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeToolName(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out.WriteRune(r)
		default:
			out.WriteByte('_')
		}
	}
	name := strings.Trim(out.String(), "_")
	if name == "" {
		return "action"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "action_" + name
	}
	return name
}

func isToolName(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			continue
		}
		if index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
