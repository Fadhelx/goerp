package mail

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type trackingFieldMeta struct {
	Model string
	Name  string
	Type  string
	Label string
}

func attachTrackingValues(env *record.Env, rows []map[string]any) ([]map[string]any, error) {
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["id"]); id != 0 {
			messageIDs = append(messageIDs, id)
		}
	}
	if len(messageIDs) == 0 {
		return nil, nil
	}
	found, err := env.Model("mail.tracking.value").Search(domain.Cond("mail_message_id", "in", uniqueIDs(messageIDs)))
	if err != nil {
		return nil, err
	}
	trackingRows, err := found.Read(
		"field_id",
		"field_info",
		"field_name",
		"field_desc",
		"field_type",
		"old_value_integer",
		"old_value_float",
		"old_value_char",
		"old_value_text",
		"old_value_datetime",
		"new_value_integer",
		"new_value_float",
		"new_value_char",
		"new_value_text",
		"new_value_datetime",
		"currency_id",
		"mail_message_id",
	)
	if err != nil {
		return nil, err
	}
	fieldMeta := trackingFieldMetadata(env, trackingRows)
	byMessage := map[int64][]map[string]any{}
	for _, row := range trackingRows {
		messageID := int64FromAny(row["mail_message_id"])
		byMessage[messageID] = append(byMessage[messageID], formatTrackingValue(row, fieldMeta[int64FromAny(row["field_id"])]))
	}
	for _, items := range byMessage {
		sort.SliceStable(items, func(i, j int) bool {
			return trackingSortKey(items[i]) < trackingSortKey(items[j])
		})
		for _, item := range items {
			delete(item, "_sort")
		}
	}
	for _, row := range rows {
		messageID := int64FromAny(row["id"])
		trackingValues := byMessage[messageID]
		if trackingValues == nil {
			trackingValues = []map[string]any{}
		}
		row["trackingValues"] = trackingValues
	}
	return trackingRows, nil
}

func trackingFieldMetadata(env *record.Env, rows []map[string]any) map[int64]trackingFieldMeta {
	fieldIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["field_id"]); id != 0 {
			fieldIDs = append(fieldIDs, id)
		}
	}
	if len(fieldIDs) == 0 {
		return nil
	}
	found, err := env.Model("ir.model.fields").Search(domain.Cond("id", "in", uniqueIDs(fieldIDs)))
	if err != nil {
		return nil
	}
	fieldRows, err := found.Read("model", "name", "ttype")
	if err != nil {
		return nil
	}
	out := map[int64]trackingFieldMeta{}
	for _, row := range fieldRows {
		id := int64FromAny(row["id"])
		meta := trackingFieldMeta{
			Model: stringAny(row["model"]),
			Name:  stringAny(row["name"]),
			Type:  stringAny(row["ttype"]),
		}
		meta.Label = trackingFieldLabel(env, meta)
		out[id] = meta
	}
	return out
}

func trackingFieldLabel(env *record.Env, meta trackingFieldMeta) string {
	if strings.TrimSpace(meta.Model) == "" || strings.TrimSpace(meta.Name) == "" {
		return ""
	}
	fields, err := env.Model(meta.Model).FieldsGet([]string{meta.Name}, []string{"string"})
	if err != nil {
		return ""
	}
	info := fields[meta.Name]
	return stringAny(info["string"])
}

func formatTrackingValue(row map[string]any, meta trackingFieldMeta) map[string]any {
	info := parseTrackingFieldInfo(row["field_info"])
	fieldName := firstText(meta.Name, info["name"], row["field_name"], "unknown")
	fieldType := firstText(meta.Type, info["type"], row["field_type"], "char")
	changedField := firstText(meta.Label, info["desc"], row["field_desc"], fieldName)
	isProperty := fieldType == "properties"
	sequence := intFromAny(firstNonNil(info["sequence"], int64(100)))
	formatted := map[string]any{
		"id": int64FromAny(row["id"]),
		"fieldInfo": map[string]any{
			"changedField":    changedField,
			"currencyId":      int64FromAny(row["currency_id"]),
			"fieldType":       fieldType,
			"isPropertyField": isProperty,
		},
		"newValue": formatTrackingDisplayValue(row, fieldType, true),
		"oldValue": formatTrackingDisplayValue(row, fieldType, false),
		"_sort":    trackingSortToken(sequence, isProperty, fieldName),
	}
	return formatted
}

func parseTrackingFieldInfo(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err == nil {
			return out
		}
	}
	return map[string]any{}
}

func formatTrackingDisplayValue(row map[string]any, fieldType string, newValue bool) any {
	prefix := "old"
	if newValue {
		prefix = "new"
	}
	switch fieldType {
	case "boolean":
		return int64FromAny(row[prefix+"_value_integer"]) != 0
	case "date":
		value := trackingTimeValue(row[prefix+"_value_datetime"])
		if value.IsZero() {
			return nil
		}
		return value.UTC().Format("2006-01-02")
	case "datetime":
		value := trackingTimeValue(row[prefix+"_value_datetime"])
		if value.IsZero() {
			return nil
		}
		return value.UTC().Format("2006-01-02 15:04:05Z")
	case "integer":
		return int64FromAny(row[prefix+"_value_integer"])
	case "float", "monetary":
		return floatFromAny(row[prefix+"_value_float"])
	case "text":
		return stringAny(row[prefix+"_value_text"])
	default:
		return stringAny(row[prefix+"_value_char"])
	}
}

func trackingTimeValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func trackingSortKey(row map[string]any) string {
	return stringAny(row["_sort"])
}

func trackingSortToken(sequence int, property bool, fieldName string) string {
	propertyToken := "0"
	if property {
		propertyToken = "1"
	}
	return leftPadInt(sequence, 6) + "|" + propertyToken + "|" + strings.TrimSpace(fieldName)
}

func leftPadInt(value int, width int) string {
	text := strconv.Itoa(value)
	for len(text) < width {
		text = "0" + text
	}
	return text
}

func intFromAny(value any) int {
	return int(int64FromAny(value))
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
