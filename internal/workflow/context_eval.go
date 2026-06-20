package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/record"
)

func ParseContextLiteral(text string) map[string]any {
	return ParseContextForEnv(nil, text)
}

func ParseContextLiteralStrict(text string) (map[string]any, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "{}" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err == nil && out != nil {
		return out, nil
	}
	parsed, err := parsePythonContextDictStrict(text)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func ParseContextForEnv(env *record.Env, text string) map[string]any {
	text = strings.TrimSpace(text)
	if text == "" || text == "{}" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err == nil && out != nil {
		return out
	}
	if parsed, ok := parsePythonContextDict(env, text); ok {
		return parsed
	}
	normalized := strings.NewReplacer(
		"'", `"`,
		"True", "true",
		"False", "false",
		"None", "null",
	).Replace(text)
	if err := json.Unmarshal([]byte(normalized), &out); err == nil && out != nil {
		return out
	}
	return map[string]any{}
}

func parsePythonContextDict(env *record.Env, text string) (map[string]any, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, false
	}
	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return map[string]any{}, true
	}
	out := map[string]any{}
	for _, item := range splitTopLevel(body, ',') {
		keyExpr, valueExpr, ok := splitPair(item)
		if !ok {
			return nil, false
		}
		key, ok := parseContextKey(keyExpr)
		if !ok || key == "" {
			return nil, false
		}
		out[key] = parseContextValue(env, valueExpr)
	}
	return out, true
}

func parsePythonContextDictStrict(text string) (map[string]any, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, fmt.Errorf("context must be a dict literal")
	}
	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	for _, item := range splitTopLevel(body, ',') {
		if strings.TrimSpace(item) == "" {
			continue
		}
		keyExpr, valueExpr, ok := splitPair(item)
		if !ok {
			return nil, fmt.Errorf("invalid context item %q", item)
		}
		key, ok := parseQuoted(keyExpr)
		if !ok || key == "" {
			return nil, fmt.Errorf("context key must be a quoted string: %q", keyExpr)
		}
		value, err := parseContextValueStrict(valueExpr)
		if err != nil {
			return nil, fmt.Errorf("context value for %q: %w", key, err)
		}
		out[key] = value
	}
	return out, nil
}

func parseContextValueStrict(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if value, ok := parseQuoted(text); ok {
		return value, nil
	}
	switch text {
	case "True":
		return true, nil
	case "False":
		return false, nil
	case "None":
		return nil, nil
	}
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		return parsePythonContextDictStrict(text)
	}
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		return parseContextListStrict(text[1 : len(text)-1])
	}
	if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		return parseContextListStrict(text[1 : len(text)-1])
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return value, nil
	}
	if value, err := strconv.ParseFloat(text, 64); err == nil {
		return value, nil
	}
	return nil, fmt.Errorf("unsupported expression %q", text)
}

func parseContextListStrict(body string) ([]any, error) {
	if strings.TrimSpace(body) == "" {
		return []any{}, nil
	}
	items := splitTopLevel(body, ',')
	out := make([]any, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		value, err := parseContextValueStrict(item)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func splitPair(item string) (string, string, bool) {
	parts := splitTopLevel(item, ':')
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseContextKey(text string) (string, bool) {
	if value, ok := parseQuoted(text); ok {
		return value, true
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	for _, r := range text {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return "", false
		}
	}
	return text, true
}

func parseContextValue(env *record.Env, text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if value, ok := parseQuoted(text); ok {
		return value
	}
	switch text {
	case "True", "true":
		return true
	case "False", "false":
		return false
	case "None", "none", "null":
		return nil
	case "uid":
		if env != nil {
			return env.Context().UserID
		}
		return nil
	}
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		return parseContextList(env, text[1:len(text)-1])
	}
	if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		return parseContextList(env, text[1:len(text)-1])
	}
	if strings.HasPrefix(text, "context.get(") && strings.HasSuffix(text, ")") {
		return parseContextGet(env, strings.TrimSuffix(strings.TrimPrefix(text, "context.get("), ")"))
	}
	if value, ok := parseEnvModelCall(env, text); ok {
		return value
	}
	if value, ok := resolveContextPath(env, text); ok {
		return value
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return value
	}
	if value, err := strconv.ParseFloat(text, 64); err == nil {
		return value
	}
	return nil
}

func parseContextList(env *record.Env, body string) []any {
	if strings.TrimSpace(body) == "" {
		return []any{}
	}
	items := splitTopLevel(body, ',')
	out := make([]any, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		out = append(out, parseContextValue(env, item))
	}
	return out
}

func parseContextGet(env *record.Env, body string) any {
	args := splitTopLevel(body, ',')
	if len(args) == 0 {
		return nil
	}
	key, ok := parseQuoted(args[0])
	if !ok {
		if len(args) > 1 {
			return parseContextValue(env, args[1])
		}
		return nil
	}
	if env == nil {
		return nil
	}
	if value, ok := env.Context().Values[key]; ok {
		return value
	}
	if len(args) > 1 {
		return parseContextValue(env, args[1])
	}
	return nil
}

func parseEnvModelCall(env *record.Env, text string) (any, bool) {
	if env == nil || !strings.HasPrefix(text, "env[") {
		return nil, false
	}
	closeBracket := strings.Index(text, "]")
	if closeBracket < 0 {
		return nil, false
	}
	modelName, ok := parseQuoted(strings.TrimSpace(text[4:closeBracket]))
	if !ok || modelName == "" {
		return nil, false
	}
	tail := text[closeBracket+1:]
	tail = strings.ReplaceAll(tail, ".sudo()", "")
	if strings.HasPrefix(tail, ".get_param(") && strings.HasSuffix(tail, ")") {
		key, ok := parseQuoted(strings.TrimSuffix(strings.TrimPrefix(tail, ".get_param("), ")"))
		if !ok {
			return nil, true
		}
		return configParam(env, key), true
	}
	return nil, true
}

func resolveContextPath(env *record.Env, text string) (any, bool) {
	if env == nil {
		return nil, false
	}
	switch {
	case text == "env.uid":
		return env.Context().UserID, true
	case text == "env.company":
		return env.Context().CompanyID, true
	case strings.HasPrefix(text, "env.company."):
		return resolveRecordPath(env, "res.company", []int64{env.Context().CompanyID}, strings.Split(strings.TrimPrefix(text, "env.company."), "."))
	case text == "env.user":
		return env.Context().UserID, true
	case strings.HasPrefix(text, "env.user."):
		return resolveRecordPath(env, "res.users", []int64{env.Context().UserID}, strings.Split(strings.TrimPrefix(text, "env.user."), "."))
	case text == "user":
		return env.Context().UserID, true
	case strings.HasPrefix(text, "user."):
		return resolveRecordPath(env, "res.users", []int64{env.Context().UserID}, strings.Split(strings.TrimPrefix(text, "user."), "."))
	}
	return nil, false
}

func resolveRecordPath(env *record.Env, modelName string, ids []int64, parts []string) (any, bool) {
	ids = compactIDs(ids)
	if len(parts) == 0 {
		return scalarOrList(ids), true
	}
	part := strings.TrimSpace(parts[0])
	switch part {
	case "":
		return nil, true
	case "id":
		if len(ids) == 0 {
			return nil, true
		}
		return ids[0], true
	case "ids":
		return idsAny(ids), true
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, true
	}
	fieldMeta, ok := meta.Fields[part]
	if !ok || len(ids) == 0 {
		return nil, true
	}
	rows, err := env.Model(modelName).Browse(ids...).Read(part)
	if err != nil {
		return nil, true
	}
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[part])
	}
	switch fieldMeta.Kind {
	case field.Many2One:
		nextIDs := compactIDs(idsFromValues(values))
		if len(parts) == 1 {
			return scalarOrList(nextIDs), true
		}
		return resolveRecordPath(env, fieldMeta.Relation, nextIDs, parts[1:])
	case field.Many2Many, field.One2Many:
		nextIDs := compactIDs(idsFromValues(values))
		if len(parts) == 1 {
			return idsAny(nextIDs), true
		}
		return resolveRecordPath(env, fieldMeta.Relation, nextIDs, parts[1:])
	default:
		if len(values) == 0 {
			return nil, true
		}
		if len(values) == 1 {
			return values[0], true
		}
		return values, true
	}
}

func configParam(env *record.Env, key string) any {
	found, err := env.Model("ir.config_parameter").Search(domain.Cond("key", "=", key))
	if err != nil || found.Len() == 0 {
		return nil
	}
	rows, err := found.Read("value")
	if err != nil || len(rows) == 0 {
		return nil
	}
	return rows[0]["value"]
}

func splitTopLevel(text string, sep rune) []string {
	var out []string
	var b strings.Builder
	var quote rune
	depth := 0
	escaped := false
	for _, r := range text {
		if quote != 0 {
			b.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '[', '{', '(':
			depth++
			b.WriteRune(r)
		case ']', '}', ')':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		default:
			if r == sep && depth == 0 {
				out = append(out, strings.TrimSpace(b.String()))
				b.Reset()
				continue
			}
			b.WriteRune(r)
		}
	}
	out = append(out, strings.TrimSpace(b.String()))
	return out
}

func parseQuoted(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return "", false
	}
	quote := text[0]
	if (quote != '\'' && quote != '"') || text[len(text)-1] != quote {
		return "", false
	}
	if quote == '"' {
		value, err := strconv.Unquote(text)
		if err == nil {
			return value, true
		}
	}
	inner := text[1 : len(text)-1]
	inner = strings.ReplaceAll(inner, `\'`, `'`)
	inner = strings.ReplaceAll(inner, `\\`, `\`)
	return inner, true
}

func compactIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func idsFromValues(values []any) []int64 {
	var out []int64
	for _, value := range values {
		out = append(out, idsFromAny(value)...)
		if id := int64FromAny(value); id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func scalarOrList(ids []int64) any {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		return ids[0]
	}
	return idsAny(ids)
}

func idsAny(ids []int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}
