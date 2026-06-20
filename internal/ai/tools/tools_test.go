package tools

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorp/internal/actions"
)

func TestToolAuthorization(t *testing.T) {
	audit := &captureAudit{}
	registry := NewRegistry(toolAuth{allowed: map[string]bool{"safe.read": true}}, audit)
	if err := registry.Register(Tool{
		Name: "safe.read",
		Schema: Schema{
			"model": {Type: TypeString, Required: true},
			"id":    {Type: TypeInt, Required: true},
		},
		Handler: func(_ context.Context, request Request) (Result, error) {
			return Result{Output: map[string]any{"ok": true, "model": request.Input["model"]}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Tool{Name: "unsafe.sql", Handler: func(context.Context, Request) (Result, error) {
		return Result{}, nil
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), Request{ToolName: "safe.read", Input: map[string]any{"model": "res.partner", "id": int64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output["ok"] != true {
		t.Fatalf("result = %+v", result)
	}
	if audit.results[len(audit.results)-1] != "allowed" {
		t.Fatalf("audit = %+v", audit.results)
	}

	_, err = registry.Run(context.Background(), Request{ToolName: "unsafe.sql"})
	if !errors.Is(err, ErrToolForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if audit.results[len(audit.results)-1] != "denied" {
		t.Fatalf("audit = %+v", audit.results)
	}
}

func TestSchemaValidation(t *testing.T) {
	err := Validate(Schema{"name": {Type: TypeString, Required: true}}, map[string]any{"name": 10})
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected schema error, got %v", err)
	}
	err = Validate(Schema{"name": {Type: TypeString, Required: true}}, nil)
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected missing field error, got %v", err)
	}
}

func TestOdooStyleSchemaValidation(t *testing.T) {
	schema := Schema{
		"name": {
			Type:      TypeString,
			Required:  true,
			Pattern:   `[a-z]+`,
			MaxLength: 8,
			Enum:      []any{"invoice", "payment"},
		},
		"tags": {
			Type:  TypeArray,
			Items: &Field{Type: TypeString, MaxLength: 12},
		},
		"payload": {
			Type: TypeObject,
			Properties: Schema{
				"amount": {Type: TypeNumber},
				"posted": {Type: TypeBool},
			},
			RequiredProperties: []string{"amount"},
		},
	}
	if err := ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"name": "invoice",
		"tags": []any{"draft", "customer"},
		"payload": map[string]any{
			"amount": 12.5,
			"posted": true,
		},
	}
	if err := Validate(schema, input); err != nil {
		t.Fatal(err)
	}
	if err := Validate(schema, map[string]any{"name": "bad-value"}); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected pattern or enum error, got %v", err)
	}
	if err := Validate(schema, map[string]any{"name": "invoice", "payload": map[string]any{"posted": true}}); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected missing object property error, got %v", err)
	}
	if err := Validate(schema, map[string]any{"name": "invoice", "extra": true}); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected unexpected field error, got %v", err)
	}
}

func TestStringMaxLengthTruncatesLikeOdoo(t *testing.T) {
	input := map[string]any{"summary": "abcdefghij"}
	err := Validate(Schema{"summary": {Type: TypeString, MaxLength: 4}}, input)
	if err != nil {
		t.Fatal(err)
	}
	if input["summary"] != "abcd..." {
		t.Fatalf("summary = %q", input["summary"])
	}
}

func TestServerActionToolRunsActionWithAIContext(t *testing.T) {
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	var captured actions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.ai", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		captured = exec
		if got := exec.Values[EndMessageKey]; got != nil {
			t.Fatalf("end message leaked into action values: %#v", got)
		}
		return actions.Result{Metadata: map[string]any{
			"status":     "ok",
			"value":      exec.Values["value"],
			"user_id":    exec.Metadata["user_id"],
			"company_id": exec.Metadata["company_id"],
			"end":        exec.Metadata[EndMessageKey],
		}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{
		ID:                    44,
		Name:                  "Write Name",
		Model:                 "res.partner",
		Kind:                  actions.KindGo,
		GoActionName:          "capture.ai",
		UseInAI:               true,
		AIToolDescription:     "Update a partner name.",
		AIToolAllowEndMessage: true,
		AIToolSchema: `{
			"type": "object",
			"properties": {
				"value": {"type": "string", "description": "New value"},
				"priority": {"type": "integer"},
				"payload": {
					"type": "object",
					"properties": {"posted": {"type": "boolean"}},
					"required": ["posted"]
				}
			},
			"required": ["value", "priority"]
		}`,
		Metadata: map[string]any{"xml_id": "base.action_write_name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	serverAction, ok := actionRegistry.Get(actionID)
	if !ok {
		t.Fatal("missing registered action")
	}

	tool, err := ServerActionTool(serverAction, actionRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "action_write_name" || tool.Description != "Update a partner name." {
		t.Fatalf("tool = %+v", tool)
	}
	if _, ok := tool.Schema[EndMessageKey]; !ok || !tool.Schema[EndMessageKey].Required {
		t.Fatalf("end message schema = %+v", tool.Schema[EndMessageKey])
	}
	registry := NewRegistry(toolAuth{allowed: map[string]bool{tool.Name: true}}, nil)
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), Request{
		UserID:    7,
		CompanyID: 3,
		Model:     "res.partner",
		RecordID:  42,
		RecordIDs: []int64{42, 43},
		ToolName:  tool.Name,
		Input: map[string]any{
			"value":       "Ada",
			"priority":    float64(9),
			"payload":     map[string]any{"posted": true},
			EndMessageKey: "Renamed.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Trigger != "ai" || captured.Model != "res.partner" || captured.RecordID != 42 || !reflect.DeepEqual(captured.RecordIDs, []int64{42, 43}) {
		t.Fatalf("captured exec = %+v", captured)
	}
	if captured.Values["priority"] != int64(9) {
		t.Fatalf("priority = %#v", captured.Values["priority"])
	}
	metadata := result.Output["metadata"].(map[string]any)
	if metadata["status"] != "ok" || metadata["value"] != "Ada" || metadata["end"] != "Renamed." {
		t.Fatalf("metadata = %+v", metadata)
	}
	if result.Output[EndMessageKey] != "Renamed." {
		t.Fatalf("result = %+v", result)
	}
}

func TestServerActionToolRequiresEndMessageWhenAllowed(t *testing.T) {
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	if err := actionRegistry.RegisterGo("noop", func(context.Context, actions.ServerAction, actions.ExecutionContext) (actions.Result, error) {
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{
		ID:                    45,
		Name:                  "Noop",
		Kind:                  actions.KindGo,
		GoActionName:          "noop",
		UseInAI:               true,
		AIToolAllowEndMessage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	serverAction, _ := actionRegistry.Get(actionID)
	tool, err := ServerActionTool(serverAction, actionRegistry)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(toolAuth{allowed: map[string]bool{tool.Name: true}}, nil)
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	_, err = registry.Run(context.Background(), Request{ToolName: tool.Name, Input: map[string]any{}})
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestServerActionToolSkipsNonAIToolsAndReportsBadSchema(t *testing.T) {
	actionRegistry := actions.NewRegistry(actions.Hooks{})
	registry := NewRegistry(toolAuth{allowed: map[string]bool{}}, nil)
	if err := RegisterServerActionTools(registry, actionRegistry, actions.ServerAction{ID: 1, Name: "Hidden", Kind: actions.KindGo}); err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 0 {
		t.Fatalf("names = %+v", registry.Names())
	}

	_, err := ServerActionTool(actions.ServerAction{
		ID:           2,
		Name:         "Bad Schema",
		Kind:         actions.KindGo,
		UseInAI:      true,
		AIToolSchema: "{bad",
	}, actionRegistry)
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected schema error, got %v", err)
	}

	_, err = ServerActionTool(actions.ServerAction{ID: 3, Name: "Not AI", Kind: actions.KindGo}, actionRegistry)
	if !errors.Is(err, ErrToolForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestSchemaFromJSONAllowsOdooStringWithoutMaxLength(t *testing.T) {
	schema, err := SchemaFromJSON(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"ids": {"type": "array", "items": {"type": "integer"}}
		},
		"required": ["name"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"name": "invoice", "ids": []any{float64(1), float64(2)}}
	if err := Validate(schema, input); err != nil {
		t.Fatal(err)
	}
	if got := input["ids"].([]any)[0]; got != float64(1) {
		t.Fatalf("array item should not be rewritten in-place, got %#v", got)
	}
}

type toolAuth struct {
	allowed map[string]bool
}

func (a toolAuth) CanRunTool(_ context.Context, request Request) bool {
	return a.allowed[request.ToolName]
}

type captureAudit struct {
	results []string
}

func (a *captureAudit) RecordToolCall(_ Request, result string) {
	a.results = append(a.results, result)
}
