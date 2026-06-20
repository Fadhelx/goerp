package automation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorp/internal/actions"
	"gorp/internal/domain"
)

type Trigger string

const (
	TriggerCreate        Trigger = "create"
	TriggerWrite         Trigger = "write"
	TriggerArchive       Trigger = "archive"
	TriggerUnarchive     Trigger = "unarchive"
	TriggerCreateOrWrite Trigger = "create_or_write"
	TriggerUnlink        Trigger = "unlink"
	TriggerOnchange      Trigger = "onchange"
	TriggerMessage       Trigger = "message"
	TriggerWebhook       Trigger = "webhook"
	TriggerTime          Trigger = "time"
	TriggerManual        Trigger = "manual"
)

var (
	ErrAutomationNotFound   = errors.New("automation not found")
	ErrAutomationNotMatched = errors.New("automation not matched")
	ErrRunnerMissing        = errors.New("automation action runner missing")
	ErrWebhookRecordMissing = errors.New("webhook record missing")
)

type Automation struct {
	ID              int64
	Name            string
	Description     string
	Model           string
	Active          bool
	Disabled        bool
	Trigger         Trigger
	TriggerFields   []string
	OnChangeFields  []string
	FilterPreDomain domain.Node
	Domain          domain.Node
	ActionID        int64
	ActionIDs       []int64
	WebhookUUID     string
	RecordGetter    string
	LogWebhookCalls bool
	TriggerFieldRef string
	DateField       string
	DateRange       int
	DateRangeMode   string
	DateRangeType   string
	DateCalendarID  int64
	LastRunAt       time.Time
	CurrentRunAt    time.Time
	Metadata        map[string]any
}

type Event struct {
	Trigger        Trigger
	Model          string
	RecordID       int64
	RecordIDs      []int64
	Record         map[string]any
	PreviousRecord map[string]any
	Values         map[string]any
	Fields         []string
	Now            time.Time
	LastRunAt      time.Time
	CurrentRunAt   time.Time
	Metadata       map[string]any
}

type WebhookLog struct {
	AutomationID int64
	UUID         string
	Payload      map[string]any
	Status       string
	Error        string
	At           time.Time
}

type Route struct {
	Name    string
	Path    string
	Methods []string
	Auth    string
	CSRF    bool
}

type WindowAction struct {
	Type     string
	Target   string
	ViewMode string
	ResModel string
	ResID    int64
	Context  map[string]any
}

type Runner interface {
	Run(context.Context, int64, actions.ExecutionContext) (actions.Result, error)
}

type Engine struct {
	mu          sync.Mutex
	nextID      int64
	runner      Runner
	automations []Automation
	webhookLogs []WebhookLog
}

func NewEngine(runner Runner) *Engine {
	return &Engine{nextID: 1, runner: runner}
}

func (e *Engine) Add(automation Automation) (int64, error) {
	if automation.Name == "" {
		return 0, fmt.Errorf("automation requires name")
	}
	if automation.Model == "" && automation.Trigger != TriggerTime {
		return 0, fmt.Errorf("automation requires model")
	}
	if !validTrigger(automation.Trigger) {
		return 0, fmt.Errorf("unsupported automation trigger %q", automation.Trigger)
	}
	if automation.ActionID == 0 && len(automation.ActionIDs) == 0 {
		return 0, fmt.Errorf("automation requires action id")
	}
	if automation.ActionID != 0 && !containsID(automation.ActionIDs, automation.ActionID) {
		automation.ActionIDs = append([]int64{automation.ActionID}, automation.ActionIDs...)
	}
	if automation.ActionID == 0 && len(automation.ActionIDs) > 0 {
		automation.ActionID = automation.ActionIDs[0]
	}
	if automation.Disabled {
		automation.Active = false
	} else {
		automation.Active = true
	}
	automation.TriggerFields = append([]string(nil), automation.TriggerFields...)
	automation.OnChangeFields = append([]string(nil), automation.OnChangeFields...)
	automation.ActionIDs = append([]int64(nil), automation.ActionIDs...)
	automation.RecordWindow()
	automation.Metadata = cloneMap(automation.Metadata)

	e.mu.Lock()
	defer e.mu.Unlock()
	if automation.ID == 0 {
		automation.ID = e.nextID
		e.nextID++
	} else {
		for _, existing := range e.automations {
			if existing.ID == automation.ID {
				return 0, fmt.Errorf("automation %d already registered", automation.ID)
			}
		}
		if automation.ID >= e.nextID {
			e.nextID = automation.ID + 1
		}
	}
	e.automations = append(e.automations, automation)
	return automation.ID, nil
}

func (e *Engine) List() []Automation {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Automation, 0, len(e.automations))
	for _, automation := range e.automations {
		out = append(out, cloneAutomation(automation))
	}
	return out
}

func (e *Engine) Run(ctx context.Context, event Event) ([]actions.Result, error) {
	event = normalizeEvent(event)
	automations := e.List()
	results := make([]actions.Result, 0, len(automations))
	for _, automation := range automations {
		ok, err := matches(automation, event)
		if err != nil {
			return results, err
		}
		if !ok {
			continue
		}
		more, err := e.runActions(ctx, automation, event)
		results = append(results, more...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func (e *Engine) RunManual(ctx context.Context, automationID int64, event Event) (actions.Result, error) {
	event = normalizeEvent(event)
	event.Trigger = TriggerManual

	automation, ok := e.get(automationID)
	if !ok {
		return actions.Result{}, ErrAutomationNotFound
	}
	if event.Model == "" {
		event.Model = automation.Model
	}
	ok, err := matches(automation, event)
	if err != nil {
		return actions.Result{}, err
	}
	if !ok {
		return actions.Result{}, ErrAutomationNotMatched
	}
	results, err := e.runActions(ctx, automation, event)
	if len(results) > 0 {
		return results[0], err
	}
	return actions.Result{}, err
}

func (e *Engine) HandleWebhook(ctx context.Context, uuid string, payload map[string]any, now time.Time) ([]actions.Result, int, error) {
	automation, ok := e.getByWebhookUUID(uuid)
	if !ok {
		return nil, 404, ErrAutomationNotFound
	}
	event, err := webhookEvent(automation, payload, now)
	if err != nil {
		e.appendWebhookLog(automation, payload, "error", err.Error(), now)
		return nil, 500, err
	}
	results, err := e.Run(ctx, event)
	if err != nil {
		e.appendWebhookLog(automation, payload, "error", err.Error(), now)
		return results, 500, err
	}
	e.appendWebhookLog(automation, payload, "ok", "", now)
	return results, 200, nil
}

func (e *Engine) WebhookLogs() []WebhookLog {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]WebhookLog, 0, len(e.webhookLogs))
	for _, log := range e.webhookLogs {
		log.Payload = cloneMap(log.Payload)
		out = append(out, log)
	}
	return out
}

func WebhookRoutes() []Route {
	return []Route{{Name: "base_automation_webhook", Path: "/web/hook/<rule_uuid>", Methods: []string{"GET", "POST"}, Auth: "public", CSRF: false}}
}

func WebhookURL(baseURL string, uuid string) string {
	return strings.TrimRight(baseURL, "/") + "/web/hook/" + uuid
}

func ActionOpenAutomation(automationID int64) WindowAction {
	return WindowAction{
		Type:     "ir.actions.act_window",
		Target:   "current",
		ViewMode: "form",
		ResModel: "base.automation",
		ResID:    automationID,
		Context:  map[string]any{"active_id": automationID},
	}
}

func (e *Engine) RunTime(ctx context.Context, currentRunAt time.Time) ([]actions.Result, error) {
	if currentRunAt.IsZero() {
		currentRunAt = time.Now().UTC()
	}
	automations := e.timeWindows(currentRunAt)
	results := make([]actions.Result, 0, len(automations))
	for _, automation := range automations {
		event := Event{
			Trigger:      TriggerTime,
			Model:        automation.Model,
			Now:          currentRunAt,
			LastRunAt:    automation.LastRunAt,
			CurrentRunAt: automation.CurrentRunAt,
		}
		ok, err := matches(automation, event)
		if err != nil {
			return results, err
		}
		if !ok {
			e.commitTimeWindow(automation.ID, currentRunAt)
			continue
		}
		more, err := e.runActions(ctx, automation, event)
		results = append(results, more...)
		if err != nil {
			return results, err
		}
		e.commitTimeWindow(automation.ID, currentRunAt)
	}
	return results, nil
}

func (e *Engine) get(id int64) (Automation, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, automation := range e.automations {
		if automation.ID == id {
			return cloneAutomation(automation), true
		}
	}
	return Automation{}, false
}

func (e *Engine) getByWebhookUUID(uuid string) (Automation, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, automation := range e.automations {
		if automation.WebhookUUID == uuid && automation.Trigger == TriggerWebhook {
			return cloneAutomation(automation), true
		}
	}
	return Automation{}, false
}

func (e *Engine) timeWindows(currentRunAt time.Time) []Automation {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Automation
	for idx := range e.automations {
		automation := e.automations[idx]
		if automation.Trigger != TriggerTime || automation.Disabled || !automation.Active {
			continue
		}
		automation.CurrentRunAt = currentRunAt
		out = append(out, cloneAutomation(automation))
	}
	return out
}

func (e *Engine) commitTimeWindow(id int64, currentRunAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for idx := range e.automations {
		if e.automations[idx].ID == id {
			e.automations[idx].LastRunAt = currentRunAt
			e.automations[idx].CurrentRunAt = currentRunAt
			return
		}
	}
}

func (e *Engine) runActions(ctx context.Context, automation Automation, event Event) ([]actions.Result, error) {
	if e.runner == nil {
		return nil, ErrRunnerMissing
	}
	actionIDs := automation.ActionIDs
	if len(actionIDs) == 0 && automation.ActionID != 0 {
		actionIDs = []int64{automation.ActionID}
	}
	results := make([]actions.Result, 0, len(actionIDs))
	for _, actionID := range actionIDs {
		result, err := e.runner.Run(ctx, actionID, executionContext(automation, event))
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func executionContext(automation Automation, event Event) actions.ExecutionContext {
	model := event.Model
	if model == "" {
		model = automation.Model
	}
	return actions.ExecutionContext{
		Model:        model,
		RecordID:     event.RecordID,
		RecordIDs:    cloneIDs(event.RecordIDs),
		Record:       eventRow(event),
		Values:       cloneMap(event.Values),
		Fields:       eventFields(event),
		Trigger:      string(event.Trigger),
		Now:          event.Now,
		LastRunAt:    event.LastRunAt,
		CurrentRunAt: event.CurrentRunAt,
		Metadata:     executionMetadata(automation, event),
	}
}

func executionMetadata(automation Automation, event Event) map[string]any {
	metadata := cloneMap(event.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	for key, value := range automation.Metadata {
		metadata[key] = value
	}
	metadata["automation_id"] = automation.ID
	metadata["automation_name"] = automation.Name
	return metadata
}

func matches(automation Automation, event Event) (bool, error) {
	if automation.Disabled || !automation.Active {
		return false, nil
	}
	if !triggerMatches(automation.Trigger, event.Trigger) {
		return false, nil
	}
	if automation.Model != "" && event.Model != "" && automation.Model != event.Model {
		return false, nil
	}
	if !fieldsMatch(automation.TriggerFields, eventFields(event)) {
		return false, nil
	}
	if automation.FilterPreDomain.Kind != "" {
		ok, err := MatchDomain(previousEventRow(event), automation.FilterPreDomain)
		if err != nil || !ok {
			return ok, err
		}
	}
	return MatchDomain(eventRow(event), automation.Domain)
}

func triggerMatches(automationTrigger Trigger, eventTrigger Trigger) bool {
	if automationTrigger == TriggerCreateOrWrite {
		return eventTrigger == TriggerCreate || eventTrigger == TriggerWrite
	}
	return automationTrigger == eventTrigger
}

func fieldsMatch(required []string, changed []string) bool {
	if len(required) == 0 {
		return true
	}
	changedSet := map[string]struct{}{}
	for _, field := range changed {
		changedSet[field] = struct{}{}
	}
	if len(changedSet) == 0 {
		return false
	}
	for _, field := range required {
		if _, ok := changedSet[field]; ok {
			return true
		}
	}
	return false
}

func eventFields(event Event) []string {
	if len(event.Fields) > 0 {
		return append([]string(nil), event.Fields...)
	}
	fields := make([]string, 0, len(event.Values))
	for field := range event.Values {
		fields = append(fields, field)
	}
	return fields
}

func eventRow(event Event) map[string]any {
	row := cloneMap(event.Record)
	if row == nil {
		row = map[string]any{}
	}
	for key, value := range event.Values {
		row[key] = value
	}
	return row
}

func previousEventRow(event Event) map[string]any {
	row := cloneMap(event.PreviousRecord)
	if row == nil {
		row = map[string]any{}
	}
	if event.RecordID != 0 {
		row["id"] = event.RecordID
	}
	return row
}

func normalizeEvent(event Event) Event {
	if event.Now.IsZero() {
		event.Now = time.Now().UTC()
	}
	if event.Trigger == TriggerTime {
		if event.CurrentRunAt.IsZero() {
			event.CurrentRunAt = event.Now
		}
	}
	return event
}

func (a *Automation) RecordWindow() {
	if a.Trigger != TriggerTime || a.CurrentRunAt.IsZero() {
		return
	}
	if a.LastRunAt.IsZero() {
		a.LastRunAt = a.CurrentRunAt
	}
}

func MatchDomain(row map[string]any, node domain.Node) (bool, error) {
	if emptyDomain(node) {
		return true, nil
	}
	switch node.Kind {
	case domain.Condition:
		return matchCondition(row, node)
	case domain.All:
		for _, child := range node.Children {
			ok, err := MatchDomain(row, child)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	case domain.Any:
		if len(node.Children) == 0 {
			return true, nil
		}
		for _, child := range node.Children {
			ok, err := MatchDomain(row, child)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case domain.None:
		if len(node.Children) != 1 {
			return false, fmt.Errorf("not domain requires exactly one child")
		}
		ok, err := MatchDomain(row, node.Children[0])
		return !ok, err
	default:
		return false, fmt.Errorf("unsupported domain kind %q", node.Kind)
	}
}

func matchCondition(row map[string]any, node domain.Node) (bool, error) {
	left, _ := lookupField(row, node.Field)
	switch node.Operator {
	case domain.Equal:
		return valuesEqual(left, node.Value), nil
	case domain.NotEqual:
		return !valuesEqual(left, node.Value), nil
	case domain.In:
		return valueIn(left, node.Value)
	case domain.NotIn:
		ok, err := valueIn(left, node.Value)
		return !ok, err
	case domain.Less, domain.LessEqual, domain.Greater, domain.GreaterEqual:
		cmp, ok := compareValues(left, node.Value)
		if !ok {
			return false, nil
		}
		switch node.Operator {
		case domain.Less:
			return cmp < 0, nil
		case domain.LessEqual:
			return cmp <= 0, nil
		case domain.Greater:
			return cmp > 0, nil
		case domain.GreaterEqual:
			return cmp >= 0, nil
		}
	case domain.Like:
		return likeMatch(left, node.Value, false), nil
	case domain.ILike:
		return likeMatch(left, node.Value, true), nil
	case domain.ChildOf:
		if valuesEqual(left, node.Value) {
			return true, nil
		}
		return valueIn(left, node.Value)
	default:
		return false, fmt.Errorf("unsupported domain operator %q", node.Operator)
	}
	return false, nil
}

func lookupField(row map[string]any, field string) (any, bool) {
	if row == nil {
		return nil, false
	}
	if value, ok := row[field]; ok {
		return value, true
	}
	parts := strings.Split(field, ".")
	var current any = row
	for _, part := range parts {
		value, ok := lookupMapValue(current, part)
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func lookupMapValue(value any, key string) (any, bool) {
	if typed, ok := value.(map[string]any); ok {
		out, exists := typed[key]
		return out, exists
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	out := rv.MapIndex(reflect.ValueOf(key))
	if !out.IsValid() {
		return nil, false
	}
	return out.Interface(), true
}

func valuesEqual(left any, right any) bool {
	if leftNumber, leftOK := numericValue(left); leftOK {
		if rightNumber, rightOK := numericValue(right); rightOK {
			return leftNumber == rightNumber
		}
	}
	return reflect.DeepEqual(left, right)
}

func valueIn(left any, values any) (bool, error) {
	if values == nil {
		return false, fmt.Errorf("domain in operator requires slice value")
	}
	rv := reflect.ValueOf(values)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false, fmt.Errorf("domain in operator requires slice value")
	}
	for idx := 0; idx < rv.Len(); idx++ {
		if valuesEqual(left, rv.Index(idx).Interface()) {
			return true, nil
		}
	}
	return false, nil
}

func compareValues(left any, right any) (int, bool) {
	if leftNumber, leftOK := numericValue(left); leftOK {
		if rightNumber, rightOK := numericValue(right); rightOK {
			switch {
			case leftNumber < rightNumber:
				return -1, true
			case leftNumber > rightNumber:
				return 1, true
			default:
				return 0, true
			}
		}
	}
	leftTime, leftOK := left.(time.Time)
	rightTime, rightOK := right.(time.Time)
	if leftOK && rightOK {
		switch {
		case leftTime.Before(rightTime):
			return -1, true
		case leftTime.After(rightTime):
			return 1, true
		default:
			return 0, true
		}
	}
	leftString, leftOK := left.(string)
	rightString, rightOK := right.(string)
	if leftOK && rightOK {
		return strings.Compare(leftString, rightString), true
	}
	return 0, false
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func likeMatch(value any, pattern any, insensitive bool) bool {
	if value == nil || pattern == nil {
		return false
	}
	left := fmt.Sprint(value)
	right := fmt.Sprint(pattern)
	if insensitive {
		left = strings.ToLower(left)
		right = strings.ToLower(right)
	}
	regex, err := regexp.Compile("^" + likePatternToRegexp(right) + "$")
	if err != nil {
		return false
	}
	return regex.MatchString(left)
}

func likePatternToRegexp(pattern string) string {
	var builder strings.Builder
	for _, r := range pattern {
		switch r {
		case '%':
			builder.WriteString(".*")
		case '_':
			builder.WriteByte('.')
		default:
			builder.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return builder.String()
}

func emptyDomain(node domain.Node) bool {
	return node.Kind == "" && node.Field == "" && node.Operator == "" && node.Value == nil && len(node.Children) == 0
}

func validTrigger(trigger Trigger) bool {
	switch trigger {
	case TriggerCreate, TriggerWrite, TriggerArchive, TriggerUnarchive, TriggerCreateOrWrite, TriggerUnlink, TriggerOnchange, TriggerMessage, TriggerWebhook, TriggerTime, TriggerManual:
		return true
	default:
		return false
	}
}

func cloneAutomation(automation Automation) Automation {
	automation.TriggerFields = append([]string(nil), automation.TriggerFields...)
	automation.OnChangeFields = append([]string(nil), automation.OnChangeFields...)
	automation.ActionIDs = append([]int64(nil), automation.ActionIDs...)
	automation.Metadata = cloneMap(automation.Metadata)
	return automation
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneIDs(in []int64) []int64 {
	return append([]int64(nil), in...)
}

func webhookEvent(automation Automation, payload map[string]any, now time.Time) (Event, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	modelName := automation.Model
	if payloadModel, ok := payload["_model"].(string); ok && payloadModel != "" {
		modelName = payloadModel
	}
	recordID := int64Value(payload["_id"])
	if recordID == 0 {
		recordID = int64Value(payload["id"])
	}
	if modelName == "" || recordID == 0 {
		return Event{}, ErrWebhookRecordMissing
	}
	record := cloneMap(payload)
	record["id"] = recordID
	return Event{
		Trigger:  TriggerWebhook,
		Model:    modelName,
		RecordID: recordID,
		Record:   record,
		Values:   cloneMap(payload),
		Now:      now,
		Metadata: map[string]any{
			"payload":      cloneMap(payload),
			"webhook_uuid": automation.WebhookUUID,
		},
	}, nil
}

func (e *Engine) appendWebhookLog(automation Automation, payload map[string]any, status string, message string, at time.Time) {
	if !automation.LogWebhookCalls {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.webhookLogs = append(e.webhookLogs, WebhookLog{
		AutomationID: automation.ID,
		UUID:         automation.WebhookUUID,
		Payload:      cloneMap(payload),
		Status:       status,
		Error:        message,
		At:           at,
	})
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float64:
		return int64(typed)
	case string:
		var out int64
		if _, err := fmt.Sscan(typed, &out); err == nil {
			return out
		}
	}
	return 0
}

func containsID(ids []int64, id int64) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}
