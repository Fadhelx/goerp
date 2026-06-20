package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	serveractions "gorp/internal/actions"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
)

func TestAIFieldsCSVFormatsRelationSelectionAndDescriptions(t *testing.T) {
	got := aiFieldsCSV(map[string]map[string]any{
		"country_id": {
			"string":     "Country",
			"type":       "many2one",
			"relation":   "res.country",
			"sortable":   true,
			"groupable":  true,
			"searchable": true,
		},
		"state": {
			"string":     "State",
			"type":       "selection",
			"selection":  [][2]string{{"draft", "Draft"}, {"posted", "Posted"}},
			"sortable":   true,
			"groupable":  true,
			"searchable": true,
			"help":       "Status | phase",
		},
		"hidden": {
			"string":     "Hidden",
			"type":       "char",
			"searchable": false,
		},
	}, true)
	for _, want := range []string{
		"field_name|display_name|type|sortable|groupable|description",
		"country_id|Country|many2one(res.country)|true|true|",
		"state|State|selection({'draft': 'Draft', 'posted': 'Posted'})|true|true|Status &#124; phase",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fields csv missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hidden|") {
		t.Fatalf("fields csv included unsearchable field:\n%s", got)
	}
}

func TestAIParseOrderedMeasures(t *testing.T) {
	measures, sorted, err := aiParseOrderedMeasures([]string{"amount_total desc", "__count"})
	if err != nil {
		t.Fatal(err)
	}
	if len(measures) != 2 || measures[0] != "amount_total" || measures[1] != "__count" {
		t.Fatalf("measures = %+v", measures)
	}
	if sorted["measure"] != "amount_total" || sorted["order"] != "desc" {
		t.Fatalf("sorted = %+v", sorted)
	}
	if _, _, err := aiParseOrderedMeasures([]string{"amount_total sideways"}); err == nil {
		t.Fatal("expected invalid order error")
	}
}

func TestAIRuntimeReadGroupRecordsetAggregateSerializesIDs(t *testing.T) {
	registry := record.NewRegistry()
	sample := model.New("x.ai.group.recordset", "x_ai_group_recordset")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("category", field.Char))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1})
	records := env.Model("x.ai.group.recordset")
	rowA, err := records.Create(map[string]any{"name": "a", "category": "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	rowB, err := records.Create(map[string]any{"name": "b", "category": "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	rowC, err := records.Create(map[string]any{"name": "c", "category": "beta"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := aiRuntimeReadGroup(env)(context.Background(), serveractions.ServerAction{}, serveractions.ExecutionContext{
		Values: map[string]any{
			"model_name": "x.ai.group.recordset",
			"domain":     []any{},
			"groupby":    []string{"category"},
			"aggregates": []string{"ids:recordset(id)"},
			"order":      "category desc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := result.Metadata["result"].([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("ai read_group result = %#v", result.Metadata["result"])
	}
	if rows[0]["category"] != "beta" {
		t.Fatalf("ordered ai read_group rows = %#v", rows)
	}
	recordset, ok := rows[1]["ids"].(record.RecordSet)
	if !ok || recordset.Len() != 2 {
		t.Fatalf("ids recordset = %#v", rows[1]["ids"])
	}
	payload, err := json.Marshal(result.Metadata["result"])
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded[0]["category"] != "beta" || !runtimeJSONIDsEqual(decoded[0]["ids"], rowC) || !runtimeJSONIDsEqual(decoded[1]["ids"], rowA, rowB) {
		t.Fatalf("serialized ai read_group = %s", payload)
	}
}

func TestAIRuntimeSearchOrdersAndLimits(t *testing.T) {
	registry := record.NewRegistry()
	sample := model.New("x.ai.search", "x_ai_search")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("score", field.Int))
	sample.AddField(field.New("active", field.Bool))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1})
	records := env.Model("x.ai.search")
	for _, values := range []map[string]any{
		{"name": "Alpha", "score": int64(2), "active": true},
		{"name": "Beta", "score": int64(5), "active": true},
		{"name": "Gamma", "score": int64(5), "active": true},
		{"name": "Archived", "score": int64(9), "active": false},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}

	result, err := aiRuntimeSearch(env)(context.Background(), serveractions.ServerAction{}, serveractions.ExecutionContext{
		Values: map[string]any{
			"model_name": "x.ai.search",
			"domain":     []any{},
			"fields":     []string{"name", "score"},
			"order":      "score desc, name asc",
			"limit":      int64(2),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := result.Metadata["result"].([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("ai search result = %#v", result.Metadata["result"])
	}
	if rows[0]["name"] != "Beta" || rows[1]["name"] != "Gamma" {
		t.Fatalf("ordered ai search rows = %#v", rows)
	}

	result, err = aiRuntimeSearch(env)(context.Background(), serveractions.ServerAction{}, serveractions.ExecutionContext{
		Values: map[string]any{
			"model_name":  "x.ai.search",
			"domain":      []any{},
			"fields":      []string{"name", "score"},
			"order":       "score desc, name asc",
			"limit":       int64(1),
			"active_test": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok = result.Metadata["result"].([]map[string]any)
	if !ok || len(rows) != 1 || rows[0]["name"] != "Archived" {
		t.Fatalf("active_test false ai search rows = %#v", result.Metadata["result"])
	}

	result, err = aiRuntimeReadGroup(env)(context.Background(), serveractions.ServerAction{}, serveractions.ExecutionContext{
		Values: map[string]any{
			"model_name":  "x.ai.search",
			"domain":      []any{},
			"groupby":     []string{"active"},
			"aggregates":  []string{"id:count"},
			"active_test": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := result.Metadata["result"].([]map[string]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("active_test false ai read_group rows = %#v", result.Metadata["result"])
	}
	foundInactive := false
	for _, group := range groups {
		if group["active"] == false {
			foundInactive = true
		}
	}
	if !foundInactive {
		t.Fatalf("active_test false ai read_group missing inactive group = %#v", groups)
	}
}

func runtimeJSONIDsEqual(value any, want ...int64) bool {
	values, ok := value.([]any)
	if !ok || len(values) != len(want) {
		return false
	}
	for index, item := range values {
		number, ok := item.(float64)
		if !ok || int64(number) != want[index] {
			return false
		}
	}
	return true
}
