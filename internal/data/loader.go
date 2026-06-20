package data

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/record"
)

type ExternalID struct {
	Module   string
	Name     string
	Model    string
	ResID    int64
	Noupdate bool
}

type Loader struct {
	env         *record.Env
	module      string
	externalIDs map[string]ExternalID
	baseDir     string
}

type csvDeferredRaw struct {
	Raw string
}

type csvDeferredField struct {
	ModelName string
	RecordID  int64
	FieldName string
	Raw       string
}

func NewLoader(env *record.Env, module string) *Loader {
	return NewLoaderWithExternalIDs(env, module, nil)
}

func NewLoaderWithExternalIDs(env *record.Env, module string, externalIDs map[string]ExternalID) *Loader {
	if externalIDs == nil {
		externalIDs = map[string]ExternalID{}
	}
	return &Loader{
		env:         env,
		module:      module,
		externalIDs: externalIDs,
	}
}

func (l *Loader) ExternalIDs() map[string]ExternalID {
	out := make(map[string]ExternalID, len(l.externalIDs))
	for key, value := range l.externalIDs {
		out[key] = value
	}
	return out
}

func (l *Loader) SetBaseDir(baseDir string) {
	l.baseDir = baseDir
}

func (l *Loader) RegisterExternalID(external ExternalID) {
	if external.Module == "" {
		external.Module = l.module
	}
	l.externalIDs[external.Module+"."+external.Name] = external
}

func (l *Loader) LoadXML(r io.Reader) error {
	decoder := xml.NewDecoder(r)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return fmt.Errorf("expected odoo root")
		}
		if err != nil {
			return err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "odoo" {
			return fmt.Errorf("expected odoo root, got %s", start.Name.Local)
		}
		return l.loadXMLChildren(decoder, start.Name.Local, xmlBoolAttr(xmlAttr(start, "noupdate"), false))
	}
}

func (l *Loader) loadXMLChildren(decoder *xml.Decoder, parent string, noupdate bool) error {
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return fmt.Errorf("unexpected end of XML inside %s", parent)
		}
		if err != nil {
			return err
		}
		switch item := token.(type) {
		case xml.StartElement:
			if err := l.loadXMLElement(decoder, item, noupdate); err != nil {
				return err
			}
		case xml.EndElement:
			if item.Name.Local == parent {
				return nil
			}
		}
	}
}

func (l *Loader) loadXMLElement(decoder *xml.Decoder, start xml.StartElement, noupdate bool) error {
	switch start.Name.Local {
	case "data":
		return l.loadXMLChildren(decoder, start.Name.Local, xmlBoolAttr(xmlAttr(start, "noupdate"), noupdate))
	case "record":
		var item xmlRecord
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadRecord(item, noupdate)
	case "menuitem":
		var item xmlMenuItem
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadMenuItem(item, noupdate)
	case "template":
		var item xmlTemplate
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadTemplate(item, noupdate)
	case "asset":
		var item xmlAsset
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadAsset(item, noupdate)
	case "report":
		var item xmlReport
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadReport(item, noupdate)
	case "function":
		var item xmlFunction
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadFunction(item)
	case "delete":
		var item xmlDelete
		if err := decoder.DecodeElement(&item, &start); err != nil {
			return err
		}
		return l.loadDelete(item)
	default:
		return decoder.Skip()
	}
}

func xmlAttr(start xml.StartElement, name string) string {
	for _, attr := range start.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (l *Loader) loadXMLItems(records []xmlRecord, menuItems []xmlMenuItem, templates []xmlTemplate, assets []xmlAsset, reports []xmlReport, functions []xmlFunction, deletes []xmlDelete, noupdate bool) error {
	for _, item := range records {
		if err := l.loadRecord(item, noupdate); err != nil {
			return err
		}
	}
	for _, item := range menuItems {
		if err := l.loadMenuItem(item, noupdate); err != nil {
			return err
		}
	}
	for _, item := range templates {
		if err := l.loadTemplate(item, noupdate); err != nil {
			return err
		}
	}
	for _, item := range assets {
		if err := l.loadAsset(item, noupdate); err != nil {
			return err
		}
	}
	for _, item := range reports {
		if err := l.loadReport(item, noupdate); err != nil {
			return err
		}
	}
	for _, item := range functions {
		if err := l.loadFunction(item); err != nil {
			return err
		}
	}
	for _, item := range deletes {
		if err := l.loadDelete(item); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) LoadCSV(modelName string, r io.Reader) error {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return err
	}
	header = normalizeCSVHeader(header)
	if csvHeaderHasNestedFields(header) {
		return l.loadNestedCSV(modelName, reader, header)
	}
	var deferred []csvDeferredField
	for {
		row, err := reader.Read()
		if err == io.EOF {
			return l.applyDeferredCSVFields(deferred)
		}
		if err != nil {
			return err
		}
		row = normalizeCSVRow(row, len(header))
		if len(row) != len(header) {
			return fmt.Errorf("csv row has %d columns, header has %d", len(row), len(header))
		}
		values := map[string]any{}
		externalID := ""
		for i, key := range header {
			if key == "id" {
				externalID = row[i]
				continue
			}
			if l.skipCSVColumn(modelName, key) {
				continue
			}
			fieldName, value, err := l.csvValueForModelAllowDeferred(modelName, key, row[i])
			if err != nil {
				return err
			}
			values[fieldName] = value
		}
		deferredValues := extractDeferredCSVValues(values)
		normalized, err := l.normalizeEvalValues(modelName, values)
		if err != nil {
			return err
		}
		id, err := l.loadValues(modelName, externalID, normalized, false)
		if err != nil {
			return err
		}
		if err := l.syncOne2ManyInverses(modelName, id, normalized); err != nil {
			return err
		}
		deferred = appendDeferredCSVFields(deferred, modelName, id, deferredValues)
	}
}

func (l *Loader) loadNestedCSV(modelName string, reader *csv.Reader, header []string) error {
	idIndex := -1
	for i, key := range header {
		if key == "id" {
			idIndex = i
			break
		}
	}
	pendingExternalID := ""
	var pendingValues map[string]any
	var deferred []csvDeferredField
	flush := func() error {
		if pendingValues == nil {
			return nil
		}
		deferredValues := extractDeferredCSVValues(pendingValues)
		normalized, err := l.normalizeEvalValues(modelName, pendingValues)
		if err != nil {
			return err
		}
		id, err := l.loadValues(modelName, pendingExternalID, normalized, false)
		if err != nil {
			return err
		}
		err = l.syncOne2ManyInverses(modelName, id, normalized)
		deferred = appendDeferredCSVFields(deferred, modelName, id, deferredValues)
		pendingValues = nil
		pendingExternalID = ""
		return err
	}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			if err := flush(); err != nil {
				return err
			}
			return l.applyDeferredCSVFields(deferred)
		}
		if err != nil {
			return err
		}
		row = normalizeCSVRow(row, len(header))
		if len(row) != len(header) {
			return fmt.Errorf("csv row has %d columns, header has %d", len(row), len(header))
		}
		externalID := ""
		if idIndex >= 0 {
			externalID = row[idIndex]
		}
		if externalID != "" || pendingValues == nil {
			if err := flush(); err != nil {
				return err
			}
			pendingExternalID = externalID
			pendingValues = map[string]any{}
			for i, key := range header {
				if key == "id" || strings.Contains(key, "/") {
					continue
				}
				if l.skipCSVColumn(modelName, key) {
					continue
				}
				fieldName, value, err := l.csvValueForModelAllowDeferred(modelName, key, row[i])
				if err != nil {
					return err
				}
				pendingValues[fieldName] = value
			}
		}
		if pendingValues == nil {
			return fmt.Errorf("csv continuation row without previous record")
		}
		if err := l.appendNestedCSVValues(modelName, pendingValues, header, row); err != nil {
			return err
		}
	}
}

func (l *Loader) appendNestedCSVValues(modelName string, values map[string]any, header []string, row []string) error {
	created := map[string]bool{}
	for i, key := range header {
		if !strings.Contains(key, "/") || row[i] == "" || csvTranslationColumn(key) {
			continue
		}
		parts := strings.Split(key, "/")
		currentModel := modelName
		currentValues := values
		for pathIndex, pathComponent := range parts[:len(parts)-1] {
			pathString := strings.Join(parts[:pathIndex+1], "/")
			info, err := l.fieldInfo(currentModel, pathComponent)
			if err != nil {
				if ignoreUnknownCSVFields(currentModel) {
					currentValues = nil
					break
				}
				return err
			}
			if info.Relation == "" {
				return fmt.Errorf("csv nested field %s.%s has no relation", currentModel, pathComponent)
			}
			if !created[pathString] {
				commandValues := map[string]any{}
				currentValues[pathComponent] = appendCSVCreateCommand(currentValues[pathComponent], commandValues)
				created[pathString] = true
				currentValues = commandValues
			} else {
				commandValues, err := lastCSVCreateCommandValues(currentValues[pathComponent])
				if err != nil {
					return err
				}
				currentValues = commandValues
			}
			currentModel = info.Relation
		}
		if currentValues == nil || l.skipCSVColumn(currentModel, parts[len(parts)-1]) {
			continue
		}
		fieldName, value, err := l.csvValueForModel(currentModel, parts[len(parts)-1], row[i])
		if err != nil {
			return err
		}
		currentValues[fieldName] = value
	}
	return nil
}

func normalizeCSVHeader(header []string) []string {
	out := make([]string, len(header))
	for i, key := range header {
		out[i] = strings.TrimSpace(key)
	}
	return out
}

func normalizeCSVRow(row []string, headerLen int) []string {
	for len(row) > headerLen && strings.TrimSpace(row[len(row)-1]) == "" {
		row = row[:len(row)-1]
	}
	return row
}

func (l *Loader) syncOne2ManyInverses(modelName string, parentID int64, values map[string]any) error {
	for fieldName, value := range values {
		ids, ok := value.([]int64)
		if !ok || len(ids) == 0 {
			continue
		}
		info, err := l.fieldInfo(modelName, fieldName)
		if err != nil {
			return err
		}
		if info.Kind != field.One2Many || info.Relation == "" || info.RelationField == "" {
			continue
		}
		if err := l.env.Model(info.Relation).Browse(ids...).Write(map[string]any{info.RelationField: parentID}); err != nil {
			return err
		}
	}
	return nil
}

func csvHeaderHasNestedFields(header []string) bool {
	for _, key := range header {
		if strings.Contains(key, "/") && !csvTranslationColumn(key) {
			return true
		}
	}
	return false
}

func (l *Loader) skipCSVColumn(modelName string, key string) bool {
	if strings.TrimSpace(key) == "" || csvTranslationColumn(key) {
		return true
	}
	fieldName := strings.TrimSuffix(key, ":id")
	if _, err := l.fieldInfo(modelName, fieldName); err != nil {
		return ignoreUnknownCSVFields(modelName)
	}
	return false
}

func csvTranslationColumn(key string) bool {
	return strings.Contains(key, "@")
}

func ignoreUnknownCSVFields(modelName string) bool {
	return strings.HasPrefix(modelName, "account.")
}

func appendCSVCreateCommand(existing any, values map[string]any) []any {
	command := evalTuple{int64(0), int64(0), values}
	switch typed := existing.(type) {
	case []any:
		return append(typed, command)
	case nil:
		return []any{command}
	default:
		return []any{typed, command}
	}
}

func lastCSVCreateCommandValues(existing any) (map[string]any, error) {
	commands, ok := existing.([]any)
	if !ok || len(commands) == 0 {
		return nil, fmt.Errorf("csv nested command list is empty")
	}
	command, ok := commands[len(commands)-1].(evalTuple)
	if !ok || len(command) < 3 {
		return nil, fmt.Errorf("csv nested command must be a create tuple")
	}
	values, ok := command[2].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("csv nested command values must be dict")
	}
	return values, nil
}

func (l *Loader) csvValueForModel(modelName string, key string, raw string) (string, any, error) {
	return l.csvValueForModelWithDeferred(modelName, key, raw, false)
}

func (l *Loader) csvValueForModelAllowDeferred(modelName string, key string, raw string) (string, any, error) {
	return l.csvValueForModelWithDeferred(modelName, key, raw, true)
}

func (l *Loader) csvValueForModelWithDeferred(modelName string, key string, raw string, allowDeferred bool) (string, any, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(key, ":id") {
		fieldName := strings.TrimSuffix(key, ":id")
		if csvNull(raw) {
			return fieldName, nil, nil
		}
		id, err := l.resolveRef(raw)
		if err != nil {
			if allowDeferred && l.deferUnknownCSVRefs(modelName, err) {
				return fieldName, csvDeferredRaw{Raw: raw}, nil
			}
			return "", nil, err
		}
		return fieldName, id, nil
	}
	info, err := l.fieldInfo(modelName, key)
	if err == nil {
		if csvNull(raw) {
			if isX2Many(info.Kind) {
				return key, []int64{}, nil
			}
			if info.Kind == field.Many2One {
				return key, nil, nil
			}
			if info.Kind == field.Int || info.Kind == field.Float || info.Kind == field.Decimal {
				return key, raw, nil
			}
		}
		switch info.Kind {
		case field.Bool:
			if value, ok := parseCSVBool(raw); ok {
				return key, value, nil
			}
		case field.Int:
			value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				return "", nil, err
			}
			return key, value, nil
		case field.Float, field.Decimal:
			value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return "", nil, err
			}
			return key, value, nil
		case field.Many2One:
			value, err := l.resolveRef(strings.TrimSpace(raw))
			if err != nil {
				if allowDeferred && l.deferUnknownCSVRefs(modelName, err) {
					return key, csvDeferredRaw{Raw: raw}, nil
				}
				return "", nil, err
			}
			return key, value, nil
		case field.Many2Many, field.One2Many:
			values, err := l.resolveCSVRefs(raw)
			if err != nil {
				if allowDeferred && l.deferUnknownCSVRefs(modelName, err) {
					return key, csvDeferredRaw{Raw: raw}, nil
				}
				return "", nil, err
			}
			return key, values, nil
		default:
			return key, raw, nil
		}
	}
	if value, ok := parseCSVBool(raw); ok {
		return key, value, nil
	}
	return key, raw, nil
}

func csvNull(raw string) bool {
	return raw == "" || raw == "False" || raw == "None"
}

func extractDeferredCSVValues(values map[string]any) map[string]string {
	out := map[string]string{}
	for fieldName, value := range values {
		deferred, ok := value.(csvDeferredRaw)
		if !ok {
			continue
		}
		out[fieldName] = deferred.Raw
		delete(values, fieldName)
	}
	return out
}

func appendDeferredCSVFields(out []csvDeferredField, modelName string, recordID int64, values map[string]string) []csvDeferredField {
	for fieldName, raw := range values {
		out = append(out, csvDeferredField{
			ModelName: modelName,
			RecordID:  recordID,
			FieldName: fieldName,
			Raw:       raw,
		})
	}
	return out
}

func (l *Loader) applyDeferredCSVFields(fields []csvDeferredField) error {
	for _, item := range fields {
		info, err := l.fieldInfo(item.ModelName, item.FieldName)
		if err != nil {
			return err
		}
		var value any
		switch {
		case info.Kind == field.Many2One:
			if csvNull(item.Raw) {
				value = nil
			} else {
				value, err = l.resolveRef(item.Raw)
			}
		case isX2Many(info.Kind):
			if csvNull(item.Raw) {
				value = []int64{}
			} else {
				value, err = l.resolveCSVRefs(item.Raw)
			}
		default:
			continue
		}
		if err != nil {
			return err
		}
		if err := l.env.Model(item.ModelName).Browse(item.RecordID).Write(map[string]any{item.FieldName: value}); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) deferUnknownCSVRefs(modelName string, err error) bool {
	return strings.HasPrefix(modelName, "account.") && strings.Contains(err.Error(), "unknown external id ref")
}

func (l *Loader) resolveCSVRefs(raw string) ([]int64, error) {
	if strings.Contains(raw, "||") {
		return l.resolveRefsWithDelimiter(raw, "||")
	}
	return l.resolveRefs(raw)
}

func parseCSVBool(raw string) (bool, bool) {
	switch strings.TrimSpace(raw) {
	case "0", "1", "true", "false", "True", "False", "TRUE", "FALSE":
		value, err := strconv.ParseBool(raw)
		return value, err == nil
	default:
		return false, false
	}
}

func (l *Loader) loadRecord(item xmlRecord, noupdate bool) error {
	if item.ID != "" && !xmlBoolAttr(item.ForceCreate, true) {
		if _, ok := l.externalIDs[qualifyRef(l.module, item.ID)]; !ok {
			return nil
		}
	}
	values := map[string]any{}
	var nested []xmlField
	for _, itemField := range item.Fields {
		if len(itemField.Records) > 0 {
			nested = append(nested, itemField)
			continue
		}
		value, err := l.xmlFieldValue(item.Model, itemField)
		if err != nil {
			return err
		}
		values[l.canonicalFieldName(item.Model, itemField.Name)] = value
	}
	if item.Model == "ir.cron" {
		if err := l.prepareCronDelegation(item.ID, values); err != nil {
			return err
		}
	}
	id, err := l.loadValues(item.Model, item.ID, values, noupdate)
	if err != nil {
		return err
	}
	return l.loadNestedFields(item.Model, id, nested, noupdate)
}

func (l *Loader) prepareCronDelegation(externalID string, values map[string]any) error {
	if actionID, _ := int64Value(values["ir_actions_server_id"]); actionID != 0 {
		return l.ensureCronServerActionUsage(actionID)
	}
	if existingID := l.existingCronServerActionID(externalID); existingID != 0 {
		values["ir_actions_server_id"] = existingID
		return l.ensureCronServerActionUsage(existingID)
	}
	actionValues := map[string]any{
		"name":   firstNonEmpty(stringFromAny(values["name"]), stringFromAny(values["cron_name"]), "Scheduled Action"),
		"active": boolFromAny(values["active"], true),
		"usage":  "ir_cron",
		"state":  firstNonEmpty(stringFromAny(values["state"]), "code"),
	}
	if modelID, _ := int64Value(values["model_id"]); modelID != 0 {
		actionValues["model_id"] = modelID
		if modelName := l.modelNameByID(modelID); modelName != "" {
			actionValues["model_name"] = modelName
		}
	}
	if code := stringFromAny(values["code"]); code != "" {
		actionValues["code"] = code
	}
	if actionName := stringFromAny(values["action_name"]); actionName != "" {
		actionValues["go_action_name"] = actionName
	}
	actionID, err := l.env.Model("ir.actions.server").Create(actionValues)
	if err != nil {
		return fmt.Errorf("create delegated cron server action: %w", err)
	}
	values["ir_actions_server_id"] = actionID
	if values["cron_name"] == nil {
		values["cron_name"] = actionValues["name"]
	}
	return nil
}

func (l *Loader) existingCronServerActionID(externalID string) int64 {
	if externalID == "" {
		return 0
	}
	external, ok := l.externalIDs[qualifyRef(l.module, externalID)]
	if !ok || external.Model != "ir.cron" || external.ResID == 0 {
		return 0
	}
	rows, err := l.env.Model("ir.cron").Browse(external.ResID).Read("ir_actions_server_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	id, _ := int64Value(rows[0]["ir_actions_server_id"])
	return id
}

func (l *Loader) ensureCronServerActionUsage(actionID int64) error {
	if actionID == 0 {
		return nil
	}
	return l.env.Model("ir.actions.server").Browse(actionID).Write(map[string]any{"usage": "ir_cron"})
}

func (l *Loader) modelNameByID(modelID int64) string {
	rows, err := l.env.Model("ir.model").Browse(modelID).Read("model")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringFromAny(rows[0]["model"])
}

func (l *Loader) loadNestedFields(modelName string, parentID int64, fields []xmlField, noupdate bool) error {
	for _, itemField := range fields {
		fieldName := l.canonicalFieldName(modelName, itemField.Name)
		info, err := l.fieldInfo(modelName, fieldName)
		if err != nil {
			return err
		}
		if !isX2Many(info.Kind) {
			return fmt.Errorf("nested records require x2many field %s.%s", modelName, fieldName)
		}
		if info.Relation == "" {
			return fmt.Errorf("nested records require relation model for %s.%s", modelName, fieldName)
		}
		if info.RelationField == "" {
			return fmt.Errorf("nested records require relation field for %s.%s", modelName, fieldName)
		}
		ids := make([]int64, 0, len(itemField.Records))
		for _, recordItem := range itemField.Records {
			childID, err := l.loadNestedRecord(info, parentID, recordItem, noupdate)
			if err != nil {
				return err
			}
			ids = append(ids, childID)
		}
		if err := l.env.Model(modelName).Browse(parentID).Write(map[string]any{fieldName: ids}); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) loadNestedRecord(info xmlFieldInfo, parentID int64, item xmlRecord, noupdate bool) (int64, error) {
	modelName := firstNonEmpty(item.Model, info.Relation)
	if modelName != info.Relation {
		return 0, fmt.Errorf("nested record model %s does not match relation %s", modelName, info.Relation)
	}
	values := map[string]any{info.RelationField: parentID}
	var nested []xmlField
	for _, itemField := range item.Fields {
		if len(itemField.Records) > 0 {
			nested = append(nested, itemField)
			continue
		}
		value, err := l.xmlFieldValue(modelName, itemField)
		if err != nil {
			return 0, err
		}
		values[l.canonicalFieldName(modelName, itemField.Name)] = value
	}
	id, err := l.loadValues(modelName, item.ID, values, noupdate)
	if err != nil {
		return 0, err
	}
	if err := l.loadNestedFields(modelName, id, nested, noupdate); err != nil {
		return 0, err
	}
	return id, nil
}

func (l *Loader) loadMenuItem(item xmlMenuItem, noupdate bool) error {
	values := map[string]any{}
	if item.Name != "" {
		values["name"] = item.Name
	}
	if item.Active != "" {
		values["active"] = xmlBoolAttr(item.Active, true)
	}
	if item.Parent != "" {
		parentID, err := l.resolveRef(item.Parent)
		if err != nil {
			return err
		}
		values["parent_id"] = parentID
	}
	if item.Action != "" {
		actionValue, err := l.menuActionValue(item.Action)
		if err != nil {
			return fmt.Errorf("menuitem %s action: %w", item.ID, err)
		}
		values["action"] = actionValue
		if values["name"] == nil {
			if name := l.menuActionName(actionValue); name != "" {
				values["name"] = name
			}
		}
	}
	if item.Sequence != "" {
		sequence, err := strconv.ParseInt(strings.TrimSpace(item.Sequence), 10, 64)
		if err != nil {
			return fmt.Errorf("menuitem %s sequence: %w", item.ID, err)
		}
		values["sequence"] = sequence
	}
	if item.Groups != "" {
		groups, err := l.resolveRefs(item.Groups)
		if err != nil {
			return err
		}
		values["groups_id"] = groups
	}
	if item.WebIcon != "" {
		values["web_icon"] = item.WebIcon
	}
	if item.WebIconData != "" {
		values["web_icon_data"] = item.WebIconData
	}
	if item.WebIconDataMimetype != "" {
		values["web_icon_data_mimetype"] = item.WebIconDataMimetype
	}
	_, err := l.loadValues("ir.ui.menu", item.ID, values, noupdate)
	return err
}

func (l *Loader) loadTemplate(item xmlTemplate, noupdate bool) error {
	values := map[string]any{
		"name":   firstNonEmpty(item.Name, item.ID),
		"type":   "qweb",
		"key":    qualifyRef(l.module, item.ID),
		"arch":   strings.TrimSpace(item.InnerXML),
		"active": true,
	}
	if item.Active != "" {
		values["active"] = xmlBoolAttr(item.Active, true)
	}
	if item.Primary != "" {
		primary := xmlBoolAttr(item.Primary, false)
		values["primary"] = primary
		if primary {
			values["mode"] = "primary"
		}
	}
	if item.CustomizeShow != "" {
		values["customize_show"] = xmlBoolAttr(item.CustomizeShow, false)
	}
	if item.Track != "" {
		values["track"] = xmlBoolAttr(item.Track, false)
	}
	if item.Page != "" {
		values["page"] = xmlBoolAttr(item.Page, false)
	}
	if item.Groups != "" {
		groups, err := l.resolveRefs(item.Groups)
		if err != nil {
			return err
		}
		values["groups_id"] = groups
	}
	if item.WebsiteID != "" {
		websiteID, err := l.resolveRef(item.WebsiteID)
		if err != nil {
			return err
		}
		values["website_id"] = websiteID
	}
	if item.Priority != "" {
		priority, err := strconv.ParseInt(strings.TrimSpace(item.Priority), 10, 64)
		if err != nil {
			return fmt.Errorf("template %s priority: %w", item.ID, err)
		}
		values["priority"] = priority
	}
	if item.InheritID != "" {
		values["inherit_id_ref"] = item.InheritID
		inheritID, err := l.resolveRef(item.InheritID)
		if err == nil {
			values["inherit_id"] = inheritID
		}
	}
	_, err := l.loadValues("ir.ui.view", item.ID, values, noupdate)
	return err
}

func (l *Loader) loadAsset(item xmlAsset, noupdate bool) error {
	values := map[string]any{
		"name":   firstNonEmpty(item.Name, item.ID),
		"active": xmlBoolAttr(item.Active, true),
		"bundle": strings.TrimSpace(item.Bundle.Value),
		"path":   strings.TrimSpace(item.Path),
	}
	if item.Bundle.Directive != "" {
		values["directive"] = item.Bundle.Directive
	}
	if item.Target != "" {
		values["target"] = strings.TrimSpace(item.Target)
	}
	for _, itemField := range item.Fields {
		value, err := l.xmlFieldValue("ir.asset", itemField)
		if err != nil {
			return err
		}
		values[itemField.Name] = value
	}
	_, err := l.loadValues("ir.asset", item.ID, values, noupdate)
	return err
}

func (l *Loader) loadReport(item xmlReport, noupdate bool) error {
	values := map[string]any{
		"name":        firstNonEmpty(item.String, item.Name, item.ID),
		"model":       item.Model,
		"report_name": item.Name,
	}
	if item.ReportType != "" {
		values["report_type"] = item.ReportType
	}
	if item.File != "" {
		values["report_file"] = item.File
	}
	if item.PrintReportName != "" {
		values["print_report_name"] = item.PrintReportName
	}
	if item.Attachment != "" {
		values["attachment"] = item.Attachment
	}
	if item.AttachmentUse != "" {
		values["attachment_use"] = xmlBoolAttr(item.AttachmentUse, false)
	}
	if item.PaperFormat != "" {
		paperFormatID, err := l.resolveRef(item.PaperFormat)
		if err != nil {
			return err
		}
		values["paperformat_id"] = paperFormatID
	}
	if item.Groups != "" {
		groups, err := l.resolveRefs(item.Groups)
		if err != nil {
			return err
		}
		values["groups_id"] = groups
	}
	if item.Model != "" {
		if modelID, ok := l.resolveModelExternalID(item.Model); ok {
			values["binding_model_id"] = modelID
			values["binding_type"] = "report"
			values["binding_view_types"] = "list,form"
		}
	}
	_, err := l.loadValues("ir.actions.report", item.ID, values, noupdate)
	return err
}

func (l *Loader) loadFunction(item xmlFunction) error {
	_, err := l.executeFunction(item)
	return err
}

func (l *Loader) executeFunction(item xmlFunction) (any, error) {
	if isKnownNoopFunction(item) {
		return nil, nil
	}
	args, err := l.functionArgs(item)
	if err != nil {
		return nil, err
	}
	switch {
	case item.Model == "ir.default" && item.Name == "set":
		return nil, l.applyDefaultSet(args)
	case item.Model == "res.lang" && item.Name == "install_lang":
		return true, l.applyInstallLang(args)
	case item.Model == "ir.config_parameter" && item.Name == "set_param":
		return nil, l.applyConfigSetParam(args)
	case item.Model == "ir.model.data" && item.Name == "_update_xmlids":
		return nil, l.applyUpdateXMLIDs(args)
	case item.Name == "search":
		return l.applyFunctionSearch(item.Model, args)
	case item.Name == "write":
		if len(args) < 2 {
			return nil, fmt.Errorf("function %s.write requires record ids and values", item.Model)
		}
		ids, err := idsFromFunctionArg(args[0])
		if err != nil {
			return nil, err
		}
		values, err := mapFromValue(args[1])
		if err != nil {
			return nil, err
		}
		normalized, err := l.normalizeEvalValues(item.Model, values)
		if err != nil {
			return nil, err
		}
		return nil, l.env.Model(item.Model).Browse(ids...).Write(normalized)
	case item.Name == "unlink":
		if len(args) < 1 {
			return nil, fmt.Errorf("function %s.unlink requires record ids", item.Model)
		}
		ids, err := idsFromFunctionArg(args[0])
		if err != nil {
			return nil, err
		}
		return nil, l.env.Model(item.Model).Browse(ids...).Unlink()
	default:
		return nil, fmt.Errorf("unsupported function %s.%s", item.Model, item.Name)
	}
}

func (l *Loader) functionArgs(item xmlFunction) ([]any, error) {
	var args []any
	if item.Eval != "" {
		value, err := parseEvalWithContext(item.Eval, l.evalContext(item.Model))
		if err != nil {
			return nil, fmt.Errorf("parse function eval for %s.%s: %w", item.Model, item.Name, err)
		}
		switch typed := value.(type) {
		case evalTuple:
			args = append(args, []any(typed)...)
		case []any:
			args = append(args, typed...)
		default:
			args = append(args, typed)
		}
	}
	for _, value := range item.Values {
		arg, err := l.xmlFunctionValueArg(item, value)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	for _, nested := range item.Functions {
		arg, err := l.executeFunction(nested)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return args, nil
}

func (l *Loader) xmlFunctionValueArg(item xmlFunction, value xmlValue) (any, error) {
	if item.Model == "ir.model.data" && item.Name == "_update_xmlids" && value.Eval != "" {
		ctx := l.evalContext(value.Model)
		ctx.preserveRS = true
		return parseEvalWithContext(value.Eval, ctx)
	}
	return l.xmlValueValue(value)
}

func (l *Loader) applyDefaultSet(args []any) error {
	if len(args) < 3 {
		return fmt.Errorf("ir.default.set requires model, field, and value")
	}
	modelName, ok := args[0].(string)
	if !ok {
		return fmt.Errorf("ir.default.set model must be string")
	}
	fieldName, ok := args[1].(string)
	if !ok {
		return fmt.Errorf("ir.default.set field must be string")
	}
	value := args[2]
	stored := fmt.Sprint(value)
	if isCollection(value) {
		encoded, err := json.Marshal(normalizeEvalTuples(value))
		if err != nil {
			return err
		}
		stored = string(encoded)
	}
	values := map[string]any{"model": modelName, "field": fieldName, "value": stored}
	modelSet := l.env.Model("ir.default")
	found, err := modelSet.Search(domain.And(domain.Cond("model", "=", modelName), domain.Cond("field", "=", fieldName)))
	if err != nil {
		return err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		_, err = modelSet.Create(values)
		return err
	}
	return modelSet.Browse(ids...).Write(values)
}

func (l *Loader) applyInstallLang(args []any) error {
	if len(args) != 0 {
		return fmt.Errorf("res.lang.install_lang does not accept loader arguments")
	}
	langCode := "en_US"
	ctx := l.env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	langSet := l.env.WithContext(ctx).Model("res.lang")
	found, err := langSet.Search(domain.Cond("code", "=", langCode))
	if err != nil {
		return err
	}
	if ids := found.IDs(); len(ids) > 0 {
		if err := langSet.Browse(ids...).Write(map[string]any{"active": true}); err != nil {
			return err
		}
	}
	defaultSet := l.env.Model("ir.default")
	defaultFound, err := defaultSet.Search(domain.And(domain.Cond("model", "=", "res.partner"), domain.Cond("field", "=", "lang")))
	if err != nil {
		return err
	}
	if defaultFound.Len() == 0 {
		return l.applyDefaultSet([]any{"res.partner", "lang", langCode})
	}
	return nil
}

func (l *Loader) applyConfigSetParam(args []any) error {
	if len(args) < 2 {
		return fmt.Errorf("ir.config_parameter.set_param requires key and value")
	}
	key, ok := args[0].(string)
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("ir.config_parameter.set_param key must be string")
	}
	value := fmt.Sprint(args[1])
	values := map[string]any{"key": key, "value": value}
	modelSet := l.env.Model("ir.config_parameter")
	found, err := modelSet.Search(domain.Cond("key", "=", key))
	if err != nil {
		return err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		_, err = modelSet.Create(values)
		return err
	}
	return modelSet.Browse(ids...).Write(values)
}

func (l *Loader) applyUpdateXMLIDs(args []any) error {
	if len(args) == 0 {
		return fmt.Errorf("ir.model.data._update_xmlids requires data list")
	}
	updateMode := false
	if len(args) > 1 {
		updateMode = boolFromAny(args[1], false)
	}
	rows, err := updateXMLIDRows(args[0])
	if err != nil {
		return err
	}
	for _, row := range rows {
		external, err := l.externalIDFromUpdateXMLIDRow(row)
		if err != nil {
			return err
		}
		if err := l.upsertExternalID(external, updateMode); err != nil {
			return err
		}
	}
	return nil
}

func updateXMLIDRows(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("ir.model.data._update_xmlids data must be a list")
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ir.model.data._update_xmlids entries must be dicts")
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (l *Loader) externalIDFromUpdateXMLIDRow(row map[string]any) (ExternalID, error) {
	xmlID := strings.TrimSpace(stringFromAny(row["xml_id"]))
	if xmlID == "" || !strings.Contains(xmlID, ".") {
		return ExternalID{}, fmt.Errorf("ir.model.data._update_xmlids requires qualified xml_id")
	}
	modelName, recordID, err := updateXMLIDRecord(row["record"])
	if err != nil {
		return ExternalID{}, err
	}
	moduleName, name := splitExternalID(l.module, xmlID)
	return ExternalID{
		Module:   moduleName,
		Name:     name,
		Model:    modelName,
		ResID:    recordID,
		Noupdate: boolFromAny(row["noupdate"], false),
	}, nil
}

func updateXMLIDRecord(value any) (string, int64, error) {
	switch typed := value.(type) {
	case evalRecordSet:
		if typed.model == "" || len(typed.ids) != 1 || typed.ids[0] == 0 {
			return "", 0, fmt.Errorf("ir.model.data._update_xmlids record must be a singleton recordset")
		}
		return typed.model, typed.ids[0], nil
	default:
		return "", 0, fmt.Errorf("ir.model.data._update_xmlids record must be a recordset")
	}
}

func (l *Loader) applyFunctionSearch(modelName string, args []any) ([]int64, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("function %s.search requires domain", modelName)
	}
	node, err := domain.Parse(normalizeEvalTuples(args[0]))
	if err != nil {
		return nil, err
	}
	found, err := l.env.Model(modelName).Search(node)
	if err != nil {
		return nil, err
	}
	return found.IDs(), nil
}

func isKnownNoopFunction(item xmlFunction) bool {
	if item.Model == "account.journal" && (item.Name == "_create_batch_payment_outbound_sequence" || item.Name == "_create_batch_payment_inbound_sequence") {
		return true
	}
	if item.Model == "account.chart.template" && (item.Name == "_install_demo" || item.Name == "_account_accountant_install_demo" || item.Name == "try_loading") {
		return true
	}
	if item.Name == "_add_members" || item.Name == "_ensure_uom_hours" || item.Name == "_create_checks" || item.Name == "create_version" || item.Name == "execute" {
		return true
	}
	if strings.HasPrefix(item.Name, "action_") {
		return true
	}
	return false
}

func (l *Loader) loadDelete(item xmlDelete) error {
	if strings.TrimSpace(item.Model) == "" {
		return fmt.Errorf("delete requires model")
	}
	ids := []int64{}
	if item.Search != "" {
		value, err := parseEvalWithContext(item.Search, l.evalContext(item.Model))
		if err != nil {
			return fmt.Errorf("parse delete search for %s: %w", item.Model, err)
		}
		node, err := domain.Parse(normalizeEvalTuples(value))
		if err != nil {
			return err
		}
		found, err := l.env.Model(item.Model).Search(node)
		if err != nil {
			return err
		}
		ids = append(ids, found.IDs()...)
	}
	if item.ID != "" {
		id, err := l.resolveRef(item.ID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return l.env.Model(item.Model).Browse(uniqueInt64(ids)...).Unlink()
}

func (l *Loader) loadValues(modelName string, externalID string, values map[string]any, noupdate bool) (int64, error) {
	if externalID == "" {
		return l.env.Model(modelName).Create(values)
	}
	key := qualifyRef(l.module, externalID)
	external, ok := l.externalIDs[key]
	if ok {
		if external.Model != "" && external.Model != modelName {
			return 0, fmt.Errorf("external id %s targets %s, got %s", key, external.Model, modelName)
		}
		exists, err := l.recordExists(modelName, external.ResID)
		if err != nil {
			return 0, err
		}
		if exists {
			external = normalizeExternalID(l.module, externalID, external, modelName)
			external.Noupdate = external.Noupdate || noupdate
			l.externalIDs[key] = external
			if err := l.persistExternalID(external); err != nil {
				return 0, err
			}
			if external.Noupdate || noupdate {
				return external.ResID, nil
			}
			if err := l.env.Model(modelName).Browse(external.ResID).Write(values); err != nil {
				return 0, err
			}
			if err := l.afterLoadValues(modelName, external.ResID, values); err != nil {
				return 0, err
			}
			return external.ResID, nil
		}
	}
	id, err := l.env.Model(modelName).Create(values)
	if err != nil {
		return 0, err
	}
	external = newExternalID(l.module, externalID, modelName, id, noupdate)
	l.externalIDs[key] = external
	if err := l.persistExternalID(external); err != nil {
		return 0, err
	}
	if err := l.afterLoadValues(modelName, id, values); err != nil {
		return 0, err
	}
	return id, nil
}

func (l *Loader) afterLoadValues(modelName string, id int64, values map[string]any) error {
	if modelName != "res.company" {
		return nil
	}
	currencyID, ok := int64Value(values["currency_id"])
	if !ok || currencyID == 0 {
		return nil
	}
	return l.env.Model("res.currency").Browse(currencyID).Write(map[string]any{"active": true})
}

func (l *Loader) persistExternalID(external ExternalID) error {
	return l.upsertExternalID(external, false)
}

func (l *Loader) upsertExternalID(external ExternalID, updateMode bool) error {
	modelData := l.env.Model("ir.model.data")
	if _, err := modelData.FieldsGet([]string{"module"}, nil); err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.data") {
			return nil
		}
		return err
	}
	external.Module = strings.TrimSpace(external.Module)
	external.Name = strings.TrimSpace(external.Name)
	values := map[string]any{
		"module":   external.Module,
		"name":     external.Name,
		"model":    external.Model,
		"res_id":   external.ResID,
		"noupdate": external.Noupdate,
	}
	found, err := modelData.Search(domain.And(
		domain.Cond("module", "=", external.Module),
		domain.Cond("name", "=", external.Name),
	))
	if err != nil {
		return err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		_, err = modelData.Create(values)
		if err == nil {
			l.externalIDs[external.Module+"."+external.Name] = external
		}
		return err
	}
	rows, err := modelData.Browse(ids[0]).Read("noupdate")
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	existingNoupdate := boolFromAny(rows[0]["noupdate"], false)
	if updateMode && existingNoupdate {
		return nil
	}
	values["noupdate"] = existingNoupdate
	if err := modelData.Browse(ids[0]).Write(values); err != nil {
		return err
	}
	external.Noupdate = existingNoupdate
	l.externalIDs[external.Module+"."+external.Name] = external
	return nil
}

func (l *Loader) recordExists(modelName string, id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	rows, err := l.env.Model(modelName).Browse(id).Read("id")
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (l *Loader) xmlFieldValue(modelName string, itemField xmlField) (any, error) {
	if itemField.Ref != "" {
		return l.resolveRef(itemField.Ref)
	}
	if itemField.Search != "" {
		return l.searchFieldValue(modelName, itemField.Name, itemField.Model, itemField.Search)
	}
	if itemField.Eval != "" {
		return l.evalFieldValue(modelName, itemField.Name, itemField.Model, itemField.Eval)
	}
	switch itemField.Type {
	case "xml", "html":
		return strings.TrimSpace(itemField.InnerXML), nil
	case "list":
		return l.xmlChildValues(itemField.Values)
	case "tuple":
		values, err := l.xmlChildValues(itemField.Values)
		if err != nil {
			return nil, err
		}
		return evalTuple(values), nil
	case "int":
		value := strings.TrimSpace(itemField.Value)
		if value == "None" {
			return nil, nil
		}
		return strconv.ParseInt(value, 10, 64)
	case "float":
		return strconv.ParseFloat(strings.TrimSpace(itemField.Value), 64)
	case "base64":
		if itemField.File == "" {
			return nil, fmt.Errorf("base64 field %s.%s requires file attribute", modelName, itemField.Name)
		}
	case "file":
		if itemField.File != "" {
			return l.fileReferenceValue(itemField.File)
		}
		return l.fileReferenceValue(strings.TrimSpace(itemField.Value))
	}
	if itemField.File != "" {
		return l.fileFieldValue(modelName, itemField)
	}
	return l.xmlScalarFieldValue(modelName, itemField.Name, strings.TrimSpace(itemField.Value))
}

func (l *Loader) xmlScalarFieldValue(modelName string, fieldName string, raw string) (any, error) {
	info, err := l.fieldInfo(modelName, fieldName)
	if err != nil {
		return raw, nil
	}
	switch info.Kind {
	case field.Bool:
		if value, ok := parseCSVBool(raw); ok {
			return value, nil
		}
	case field.Int:
		if raw == "" || raw == "None" {
			return nil, nil
		}
		return strconv.ParseInt(raw, 10, 64)
	case field.Float, field.Decimal:
		if raw == "" || raw == "None" {
			return nil, nil
		}
		return strconv.ParseFloat(raw, 64)
	}
	return raw, nil
}

func (l *Loader) xmlChildValues(values []xmlValue) ([]any, error) {
	out := make([]any, 0, len(values))
	for _, value := range values {
		item, err := l.xmlValueValue(value)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (l *Loader) xmlValueValue(value xmlValue) (any, error) {
	if value.Ref != "" {
		return l.resolveRef(value.Ref)
	}
	if value.Search != "" {
		return l.searchValue(value.Model, value.Use, value.Search)
	}
	if value.Eval != "" {
		return parseEvalWithContext(value.Eval, l.evalContext(value.Model))
	}
	switch value.Type {
	case "xml", "html":
		return strings.TrimSpace(value.InnerXML), nil
	case "list":
		return l.xmlChildValues(value.Values)
	case "tuple":
		values, err := l.xmlChildValues(value.Values)
		if err != nil {
			return nil, err
		}
		return evalTuple(values), nil
	case "int":
		text := strings.TrimSpace(value.Value)
		if text == "None" {
			return nil, nil
		}
		return strconv.ParseInt(text, 10, 64)
	case "float":
		return strconv.ParseFloat(strings.TrimSpace(value.Value), 64)
	case "base64":
		if value.File == "" {
			return nil, fmt.Errorf("base64 value requires file attribute")
		}
	case "file":
		if value.File != "" {
			return l.fileReferenceValue(value.File)
		}
		return l.fileReferenceValue(strings.TrimSpace(value.Value))
	}
	if value.File != "" {
		raw, err := l.readFieldFile(value.File)
		if err != nil {
			return nil, err
		}
		if value.Type == "base64" {
			return base64.StdEncoding.EncodeToString(raw), nil
		}
		return string(raw), nil
	}
	return strings.TrimSpace(value.Value), nil
}

func (l *Loader) fileFieldValue(modelName string, itemField xmlField) (any, error) {
	info, err := l.fieldInfo(modelName, itemField.Name)
	if err != nil {
		return nil, err
	}
	raw, err := l.readFieldFile(itemField.File)
	if err != nil {
		return nil, err
	}
	if itemField.Type == "base64" || info.Kind == field.Binary {
		return base64.StdEncoding.EncodeToString(raw), nil
	}
	return string(raw), nil
}

func (l *Loader) fileReferenceValue(rawPath string) (string, error) {
	clean, err := cleanRelativePath(rawPath)
	if err != nil {
		return "", err
	}
	if _, err := l.readFieldFile(clean); err != nil {
		return "", err
	}
	return l.module + "," + filepath.ToSlash(clean), nil
}

func (l *Loader) readFieldFile(rawPath string) ([]byte, error) {
	if l.baseDir == "" {
		return nil, fmt.Errorf("field file %s requires loader base directory", rawPath)
	}
	clean, err := cleanRelativePath(rawPath)
	if err != nil {
		return nil, err
	}
	candidates := l.fieldFileCandidates(clean)
	var readErr error
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err == nil {
			return raw, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read field file %s: %w", candidate, err)
		}
		readErr = err
	}
	if readErr != nil {
		return nil, fmt.Errorf("field file %s not found under %s", rawPath, l.baseDir)
	}
	return nil, fmt.Errorf("field file %s has no candidate path", rawPath)
}

func (l *Loader) fieldFileCandidates(clean string) []string {
	out := []string{}
	add := func(path string) {
		for _, existing := range out {
			if existing == path {
				return
			}
		}
		out = append(out, path)
	}
	modulePrefix := l.module + string(os.PathSeparator)
	if strings.HasPrefix(clean, modulePrefix) {
		add(filepath.Join(l.baseDir, strings.TrimPrefix(clean, modulePrefix)))
	}
	add(filepath.Join(l.baseDir, clean))
	add(filepath.Join(filepath.Dir(l.baseDir), clean))
	return out
}

func cleanRelativePath(rawPath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rawPath)))
	if clean == "" || clean == "." {
		return "", fmt.Errorf("field file path is empty")
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("field file path escapes module: %s", rawPath)
	}
	return clean, nil
}

func (l *Loader) evalFieldValue(modelName string, fieldName string, evalModel string, raw string) (any, error) {
	info, err := l.fieldInfo(modelName, fieldName)
	if err != nil {
		return nil, err
	}
	value, err := parseEvalWithContext(raw, l.evalContext(firstNonEmpty(evalModel, modelName)))
	if err != nil {
		return nil, fmt.Errorf("parse eval for %s.%s: %w", modelName, fieldName, err)
	}
	if isX2Many(info.Kind) {
		ids, ok, err := l.x2manyFieldIDs(info, value)
		if err != nil {
			return nil, fmt.Errorf("parse x2many commands for %s.%s: %w", modelName, fieldName, err)
		}
		if ok {
			return ids, nil
		}
	}
	value = normalizeEvalTuples(value)
	if isTextual(info.Kind) && isCollection(value) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return string(encoded), nil
	}
	return value, nil
}

func (l *Loader) x2manyFieldIDs(info xmlFieldInfo, value any) ([]int64, bool, error) {
	switch typed := value.(type) {
	case evalTuple:
		ids, err := l.applyX2ManyFieldCommand(info, nil, []any(typed))
		return ids, true, err
	case []any:
		if len(typed) == 0 {
			return []int64{}, true, nil
		}
		if containsTuple(typed) {
			ids := []int64{}
			for _, item := range typed {
				command, ok := item.(evalTuple)
				if !ok {
					return nil, true, fmt.Errorf("mixed x2many command list")
				}
				var err error
				ids, err = l.applyX2ManyFieldCommand(info, ids, []any(command))
				if err != nil {
					return nil, true, err
				}
			}
			return ids, true, nil
		}
		ids, ok := int64List(typed)
		return ids, ok, nil
	default:
		return nil, false, nil
	}
}

func (l *Loader) applyX2ManyFieldCommand(info xmlFieldInfo, ids []int64, command []any) ([]int64, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty x2many command")
	}
	code, ok := int64Value(command[0])
	if !ok {
		return nil, fmt.Errorf("x2many command code must be int")
	}
	switch code {
	case 0:
		if len(command) < 3 {
			return nil, fmt.Errorf("x2many create command requires values")
		}
		if info.Relation == "" {
			return nil, fmt.Errorf("x2many create command requires relation model")
		}
		values, err := mapFromValue(command[2])
		if err != nil {
			return nil, err
		}
		normalized, err := l.normalizeEvalValues(info.Relation, values)
		if err != nil {
			return nil, err
		}
		id, err := l.env.Model(info.Relation).Create(normalized)
		if err != nil {
			return nil, err
		}
		if !containsInt64(ids, id) {
			ids = append(ids, id)
		}
		return ids, nil
	case 1:
		if len(command) < 3 {
			return nil, fmt.Errorf("x2many update command requires id and values")
		}
		if info.Relation == "" {
			return nil, fmt.Errorf("x2many update command requires relation model")
		}
		id, ok := int64Value(command[1])
		if !ok {
			return nil, fmt.Errorf("x2many update id must be int")
		}
		values, err := mapFromValue(command[2])
		if err != nil {
			return nil, err
		}
		normalized, err := l.normalizeEvalValues(info.Relation, values)
		if err != nil {
			return nil, err
		}
		if err := l.env.Model(info.Relation).Browse(id).Write(normalized); err != nil {
			return nil, err
		}
		return ids, nil
	case 2, 3:
		if len(command) < 2 {
			return nil, fmt.Errorf("x2many unlink command requires id")
		}
		id, ok := int64Value(command[1])
		if !ok {
			return nil, fmt.Errorf("x2many unlink id must be int")
		}
		return removeInt64(ids, id), nil
	case 4:
		if len(command) < 2 {
			return nil, fmt.Errorf("x2many link command requires id")
		}
		id, ok := int64Value(command[1])
		if !ok {
			return nil, fmt.Errorf("x2many link id must be int")
		}
		if !containsInt64(ids, id) {
			ids = append(ids, id)
		}
		return ids, nil
	case 5:
		return []int64{}, nil
	case 6:
		if len(command) < 3 {
			return nil, fmt.Errorf("x2many set command requires id list")
		}
		next, err := idsFromValue(command[2])
		if err != nil {
			return nil, err
		}
		return next, nil
	default:
		return nil, fmt.Errorf("unsupported x2many command %d", code)
	}
}

func (l *Loader) normalizeEvalValues(modelName string, values map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for fieldName, value := range values {
		canonicalField := l.canonicalFieldName(modelName, fieldName)
		info, err := l.fieldInfo(modelName, canonicalField)
		if err != nil {
			return nil, err
		}
		if isX2Many(info.Kind) {
			ids, ok, err := l.x2manyFieldIDs(info, value)
			if err != nil {
				return nil, err
			}
			if ok {
				out[canonicalField] = ids
				continue
			}
		}
		normalized := normalizeEvalTuples(value)
		if isTextual(info.Kind) && isCollection(normalized) {
			encoded, err := json.Marshal(normalized)
			if err != nil {
				return nil, err
			}
			out[canonicalField] = string(encoded)
			continue
		}
		out[canonicalField] = normalized
	}
	return out, nil
}

func mapFromValue(value any) (map[string]any, error) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("x2many command values must be dict")
	}
	return typed, nil
}

func idsFromFunctionArg(value any) ([]int64, error) {
	if id, ok := int64Value(value); ok {
		return []int64{id}, nil
	}
	switch typed := value.(type) {
	case []int64:
		return typed, nil
	case []any:
		ids, ok := int64List(typed)
		if !ok {
			return nil, fmt.Errorf("function record ids must contain ints")
		}
		return ids, nil
	case evalTuple:
		ids, ok := int64List([]any(typed))
		if !ok {
			return nil, fmt.Errorf("function record ids must contain ints")
		}
		return ids, nil
	default:
		return nil, fmt.Errorf("function record ids must be int or list")
	}
}

func (l *Loader) searchFieldValue(modelName string, fieldName string, searchModel string, raw string) (any, error) {
	info, err := l.fieldInfo(modelName, fieldName)
	if err != nil {
		return nil, err
	}
	relation := firstNonEmpty(searchModel, info.Relation)
	if relation == "" {
		return nil, fmt.Errorf("field %s.%s has no relation for search", modelName, fieldName)
	}
	value, err := parseEvalWithContext(raw, l.evalContext(relation))
	if err != nil {
		return nil, fmt.Errorf("parse search for %s.%s: %w", modelName, fieldName, err)
	}
	node, err := domain.Parse(normalizeEvalTuples(value))
	if err != nil {
		return nil, err
	}
	found, err := l.env.Model(relation).Search(node)
	if err != nil {
		return nil, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		if isX2Many(info.Kind) {
			return []int64{}, nil
		}
		return false, nil
	}
	if isX2Many(info.Kind) {
		return ids, nil
	}
	return ids[0], nil
}

func (l *Loader) searchValue(modelName string, use string, raw string) (any, error) {
	if modelName == "" {
		return nil, fmt.Errorf("search value requires model attribute")
	}
	value, err := parseEvalWithContext(raw, l.evalContext(modelName))
	if err != nil {
		return nil, err
	}
	node, err := domain.Parse(normalizeEvalTuples(value))
	if err != nil {
		return nil, err
	}
	found, err := l.env.Model(modelName).Search(node)
	if err != nil {
		return nil, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return false, nil
	}
	if use == "" || use == "id" {
		return ids[0], nil
	}
	rows, err := found.Read(use)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return rows[0][use], nil
}

func (l *Loader) resolveRef(raw string) (int64, error) {
	ref, ok, err := l.resolveExternalID(raw)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("unknown external id ref %s", raw)
	}
	return ref.ResID, nil
}

func (l *Loader) resolveExternalID(raw string) (ExternalID, bool, error) {
	key := qualifyRef(l.module, raw)
	ref, ok := l.externalIDs[key]
	if ok {
		return ref, true, nil
	}
	ref, ok, err := l.lookupPersistedExternalID(raw)
	if err != nil || !ok {
		return ExternalID{}, ok, err
	}
	l.externalIDs[ref.Module+"."+ref.Name] = ref
	return ref, true, nil
}

func (l *Loader) evalContext(modelName string) evalContext {
	return evalContext{
		env:           l.env,
		currentModel:  modelName,
		resolveID:     l.resolveRef,
		resolveRecord: l.resolveEvalRef,
	}
}

func (l *Loader) resolveEvalRef(raw string) (evalRef, error) {
	ref, ok, err := l.resolveExternalID(raw)
	if err != nil {
		return evalRef{}, err
	}
	if !ok {
		return evalRef{}, fmt.Errorf("unknown external id ref %s", raw)
	}
	return evalRef{Model: ref.Model, ID: ref.ResID}, nil
}

func (l *Loader) lookupPersistedExternalID(raw string) (ExternalID, bool, error) {
	modelData := l.env.Model("ir.model.data")
	if _, err := modelData.FieldsGet([]string{"module"}, nil); err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.data") {
			return ExternalID{}, false, nil
		}
		return ExternalID{}, false, err
	}
	moduleName, name := splitExternalID(l.module, raw)
	found, err := modelData.Search(domain.And(domain.Cond("module", "=", moduleName), domain.Cond("name", "=", name)))
	if err != nil {
		return ExternalID{}, false, err
	}
	rows, err := found.Read("module", "name", "model", "res_id", "noupdate")
	if err != nil {
		return ExternalID{}, false, err
	}
	if len(rows) == 0 {
		return ExternalID{}, false, nil
	}
	resID, _ := int64Value(rows[0]["res_id"])
	noupdate, _ := rows[0]["noupdate"].(bool)
	return ExternalID{
		Module:   fmt.Sprint(rows[0]["module"]),
		Name:     fmt.Sprint(rows[0]["name"]),
		Model:    fmt.Sprint(rows[0]["model"]),
		ResID:    resID,
		Noupdate: noupdate,
	}, true, nil
}

func (l *Loader) resolveRefs(raw string) ([]int64, error) {
	return l.resolveRefsWithDelimiter(raw, ",")
}

func (l *Loader) resolveRefsWithDelimiter(raw string, delimiter string) ([]int64, error) {
	parts := strings.Split(raw, delimiter)
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		ref := strings.TrimSpace(part)
		if ref == "" {
			continue
		}
		id, err := l.resolveRef(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (l *Loader) resolveModelExternalID(modelName string) (int64, bool) {
	modelRef := "model_" + strings.ReplaceAll(modelName, ".", "_")
	for _, ref := range []string{modelRef, l.module + "." + modelRef, "base." + modelRef} {
		if external, ok := l.externalIDs[qualifyRef(l.module, ref)]; ok {
			return external.ResID, true
		}
	}
	return 0, false
}

type xmlFieldInfo struct {
	Kind          field.Kind
	Relation      string
	RelationField string
}

func (l *Loader) fieldInfo(modelName string, fieldName string) (xmlFieldInfo, error) {
	fieldName = l.canonicalFieldName(modelName, fieldName)
	fields, err := l.env.Model(modelName).FieldsGet([]string{fieldName}, []string{"type", "relation", "relation_field"})
	if err != nil {
		return xmlFieldInfo{}, err
	}
	description, ok := fields[fieldName]
	if !ok {
		return xmlFieldInfo{}, fmt.Errorf("unknown field %s.%s", modelName, fieldName)
	}
	kind, _ := description["type"].(string)
	relation, _ := description["relation"].(string)
	relationField, _ := description["relation_field"].(string)
	return xmlFieldInfo{Kind: field.Kind(kind), Relation: relation, RelationField: relationField}, nil
}

func (l *Loader) canonicalFieldName(modelName string, fieldName string) string {
	if modelName == "res.groups" && fieldName == "users" {
		return "user_ids"
	}
	return fieldName
}

func isX2Many(kind field.Kind) bool {
	return kind == field.Many2Many || kind == field.One2Many
}

func isTextual(kind field.Kind) bool {
	switch kind {
	case field.Char, field.Text, field.Selection, field.Date, field.DateTime, field.Binary:
		return true
	default:
		return false
	}
}

func isCollection(value any) bool {
	switch value.(type) {
	case []any, map[string]any:
		return true
	default:
		return false
	}
}

type xmlDoc struct {
	XMLName   xml.Name      `xml:"odoo"`
	Noupdate  string        `xml:"noupdate,attr"`
	Records   []xmlRecord   `xml:"record"`
	MenuItems []xmlMenuItem `xml:"menuitem"`
	Templates []xmlTemplate `xml:"template"`
	Assets    []xmlAsset    `xml:"asset"`
	Reports   []xmlReport   `xml:"report"`
	Functions []xmlFunction `xml:"function"`
	Deletes   []xmlDelete   `xml:"delete"`
	Data      []xmlData     `xml:"data"`
}

type xmlData struct {
	Noupdate  string        `xml:"noupdate,attr"`
	Records   []xmlRecord   `xml:"record"`
	MenuItems []xmlMenuItem `xml:"menuitem"`
	Templates []xmlTemplate `xml:"template"`
	Assets    []xmlAsset    `xml:"asset"`
	Reports   []xmlReport   `xml:"report"`
	Functions []xmlFunction `xml:"function"`
	Deletes   []xmlDelete   `xml:"delete"`
}

type xmlRecord struct {
	ID          string     `xml:"id,attr"`
	Model       string     `xml:"model,attr"`
	ForceCreate string     `xml:"forcecreate,attr"`
	Fields      []xmlField `xml:"field"`
}

type xmlField struct {
	Name     string      `xml:"name,attr"`
	Model    string      `xml:"model,attr"`
	Use      string      `xml:"use,attr"`
	Type     string      `xml:"type,attr"`
	Ref      string      `xml:"ref,attr"`
	Eval     string      `xml:"eval,attr"`
	Search   string      `xml:"search,attr"`
	File     string      `xml:"file,attr"`
	Value    string      `xml:",chardata"`
	InnerXML string      `xml:",innerxml"`
	Values   []xmlValue  `xml:"value"`
	Records  []xmlRecord `xml:"record"`
}

type xmlValue struct {
	Name     string     `xml:"name,attr"`
	Model    string     `xml:"model,attr"`
	Use      string     `xml:"use,attr"`
	Type     string     `xml:"type,attr"`
	Ref      string     `xml:"ref,attr"`
	Eval     string     `xml:"eval,attr"`
	Search   string     `xml:"search,attr"`
	File     string     `xml:"file,attr"`
	Value    string     `xml:",chardata"`
	InnerXML string     `xml:",innerxml"`
	Values   []xmlValue `xml:"value"`
}

type xmlMenuItem struct {
	ID                  string `xml:"id,attr"`
	Name                string `xml:"name,attr"`
	Active              string `xml:"active,attr"`
	Parent              string `xml:"parent,attr"`
	Action              string `xml:"action,attr"`
	Sequence            string `xml:"sequence,attr"`
	Groups              string `xml:"groups,attr"`
	WebIcon             string `xml:"web_icon,attr"`
	WebIconData         string `xml:"web_icon_data,attr"`
	WebIconDataMimetype string `xml:"web_icon_data_mimetype,attr"`
}

type xmlTemplate struct {
	ID            string `xml:"id,attr"`
	Name          string `xml:"name,attr"`
	InheritID     string `xml:"inherit_id,attr"`
	Priority      string `xml:"priority,attr"`
	Active        string `xml:"active,attr"`
	Primary       string `xml:"primary,attr"`
	Groups        string `xml:"groups,attr"`
	CustomizeShow string `xml:"customize_show,attr"`
	Track         string `xml:"track,attr"`
	Page          string `xml:"page,attr"`
	WebsiteID     string `xml:"website_id,attr"`
	InnerXML      string `xml:",innerxml"`
}

type xmlAsset struct {
	ID     string         `xml:"id,attr"`
	Name   string         `xml:"name,attr"`
	Active string         `xml:"active,attr"`
	Bundle xmlAssetBundle `xml:"bundle"`
	Path   string         `xml:"path"`
	Target string         `xml:"target"`
	Fields []xmlField     `xml:"field"`
}

type xmlAssetBundle struct {
	Directive string `xml:"directive,attr"`
	Value     string `xml:",chardata"`
}

type xmlReport struct {
	ID              string `xml:"id,attr"`
	String          string `xml:"string,attr"`
	Model           string `xml:"model,attr"`
	ReportType      string `xml:"report_type,attr"`
	Name            string `xml:"name,attr"`
	File            string `xml:"file,attr"`
	PrintReportName string `xml:"print_report_name,attr"`
	Attachment      string `xml:"attachment,attr"`
	AttachmentUse   string `xml:"attachment_use,attr"`
	PaperFormat     string `xml:"paperformat,attr"`
	Groups          string `xml:"groups,attr"`
}

type xmlFunction struct {
	Model     string        `xml:"model,attr"`
	Name      string        `xml:"name,attr"`
	Eval      string        `xml:"eval,attr"`
	Context   string        `xml:"context,attr"`
	Values    []xmlValue    `xml:"value"`
	Functions []xmlFunction `xml:"function"`
}

type xmlDelete struct {
	ID     string `xml:"id,attr"`
	Model  string `xml:"model,attr"`
	Search string `xml:"search,attr"`
}

func qualifyRef(module, ref string) string {
	if strings.Contains(ref, ".") {
		return ref
	}
	return module + "." + ref
}

func (l *Loader) menuActionValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, ",") {
		return "", fmt.Errorf("expected external id, got %s", raw)
	}
	ref, ok, err := l.resolveExternalID(raw)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown external id ref %s", raw)
	}
	modelName, err := l.concreteActionModelForExternalID(ref)
	if err != nil {
		return "", err
	}
	return modelName + "," + strconv.FormatInt(ref.ResID, 10), nil
}

func (l *Loader) menuActionName(actionValue string) string {
	parts := strings.Split(actionValue, ",")
	if len(parts) != 2 {
		return ""
	}
	modelName := strings.TrimSpace(parts[0])
	id, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || id == 0 {
		return ""
	}
	rows, err := l.env.Model(modelName).Browse(id).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(rows[0]["name"]))
}

func (l *Loader) concreteActionModelForExternalID(ref ExternalID) (string, error) {
	if isMenuActionModelName(ref.Model) {
		return ref.Model, nil
	}
	if ref.Model == "ir.actions.actions" {
		return l.concreteActionModel(ref.ResID)
	}
	return "", fmt.Errorf("external id %s.%s targets %s, not an action", ref.Module, ref.Name, ref.Model)
}

func (l *Loader) concreteActionModel(id int64) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid action id %d", id)
	}
	if _, ok := l.env.ModelMetadata("ir.actions.actions"); !ok {
		return "", fmt.Errorf("unknown model ir.actions.actions")
	}
	rows, err := l.env.Model("ir.actions.actions").Browse(id).Read("type")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("unknown action id %d", id)
	}
	modelName := stringFromAny(rows[0]["type"])
	if !isMenuActionModelName(modelName) {
		return "", fmt.Errorf("unsupported action type %s", modelName)
	}
	return modelName, nil
}

func isMenuActionModelName(modelName string) bool {
	switch modelName {
	case "ir.actions.act_window", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func boolFromAny(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func uniqueInt64(items []int64) []int64 {
	out := make([]int64, 0, len(items))
	seen := map[int64]bool{}
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func xmlBoolAttr(raw string, fallback bool) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	return raw == "1" || strings.EqualFold(raw, "true")
}

func newExternalID(module string, name string, modelName string, resID int64, noupdate bool) ExternalID {
	externalModule, externalName := splitExternalID(module, name)
	return ExternalID{
		Module:   externalModule,
		Name:     externalName,
		Model:    modelName,
		ResID:    resID,
		Noupdate: noupdate,
	}
}

func normalizeExternalID(module string, name string, external ExternalID, modelName string) ExternalID {
	externalModule, externalName := splitExternalID(module, name)
	if external.Module == "" {
		external.Module = externalModule
	}
	if external.Name == "" {
		external.Name = externalName
	}
	if external.Model == "" {
		external.Model = modelName
	}
	return external
}

func splitExternalID(module string, name string) (string, string) {
	qualified := qualifyRef(module, name)
	parts := strings.SplitN(qualified, ".", 2)
	if len(parts) == 1 {
		return module, parts[0]
	}
	return parts[0], parts[1]
}
