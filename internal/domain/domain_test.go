package domain

import (
	"reflect"
	"testing"
)

func TestCompileCondition(t *testing.T) {
	sql, err := CompileSQL(Cond("name", ILike, "admin%"))
	if err != nil {
		t.Fatal(err)
	}
	if sql.Query != "name ILIKE $1" {
		t.Fatalf("query = %q", sql.Query)
	}
	if !reflect.DeepEqual(sql.Args, []any{"%admin%%"}) {
		t.Fatalf("args = %#v", sql.Args)
	}
}

func TestCompileLogicalDomain(t *testing.T) {
	sql, err := CompileSQL(And(
		Cond("active", Equal, true),
		Or(Cond("id", In, []int{1, 2}), Not(Cond("name", Equal, "Demo"))),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := "(active = $1) AND ((id IN ($2, $3)) OR (NOT (name = $4)))"
	if sql.Query != want {
		t.Fatalf("query = %q", sql.Query)
	}
}

func TestParseLegacyDomain(t *testing.T) {
	node, err := Parse([]any{
		"|",
		[]any{"name", "ilike", "admin"},
		[]any{"active", "=", true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Kind != Any || len(node.Children) != 2 {
		t.Fatalf("node = %#v", node)
	}
	if node.Children[0].Operator != ILike || node.Children[1].Field != "active" {
		t.Fatalf("children = %#v", node.Children)
	}
}

func TestParseImplicitAndAndOptionalEqual(t *testing.T) {
	node, err := Parse([]any{
		[]any{"name", "=", "Admin"},
		[]any{"company_id", "=?", false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Kind != All || len(node.Children) != 2 {
		t.Fatalf("node = %#v", node)
	}
	if node.Children[1].Kind != Literal || node.Children[1].Value != true {
		t.Fatalf("optional equality = %#v", node.Children[1])
	}
}

func TestParseBooleanLeaves(t *testing.T) {
	trueNode, err := Parse([]any{1, "=", 1})
	if err != nil {
		t.Fatal(err)
	}
	if trueNode.Kind != Literal || trueNode.Value != true {
		t.Fatalf("true node = %#v", trueNode)
	}
	falseNode, err := Parse([]any{0, "=", 1})
	if err != nil {
		t.Fatal(err)
	}
	if falseNode.Kind != Literal || falseNode.Value != false {
		t.Fatalf("false node = %#v", falseNode)
	}
}

func TestCompileOdooStringOperators(t *testing.T) {
	sql, err := CompileSQL(And(
		Cond("name", Like, "adm"),
		Cond("code", EqualILike, "A%"),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := "(name LIKE $1) AND (code ILIKE $2)"
	if sql.Query != want {
		t.Fatalf("query = %q", sql.Query)
	}
	if !reflect.DeepEqual(sql.Args, []any{"%adm%", "A%"}) {
		t.Fatalf("args = %#v", sql.Args)
	}
}

func TestCompileEmptyOrIsFalse(t *testing.T) {
	sql, err := CompileSQL(Or())
	if err != nil {
		t.Fatal(err)
	}
	if sql.Query != "FALSE" {
		t.Fatalf("query = %q", sql.Query)
	}
}

func TestCompileRejectsUnsafeField(t *testing.T) {
	if _, err := CompileSQL(Cond("name;drop", Equal, "x")); err == nil {
		t.Fatal("expected invalid field error")
	}
}

func TestParseLiteralPythonDomain(t *testing.T) {
	node, err := ParseLiteral("['|', ('email', '=', 'a@example.com'), ('active', '=', True)]")
	if err != nil {
		t.Fatal(err)
	}
	if node.Kind != Any || len(node.Children) != 2 {
		t.Fatalf("node = %#v", node)
	}
	if node.Children[0].Field != "email" || node.Children[0].Value != "a@example.com" {
		t.Fatalf("first child = %#v", node.Children[0])
	}
	if node.Children[1].Field != "active" || node.Children[1].Value != true {
		t.Fatalf("second child = %#v", node.Children[1])
	}
}

func TestParseLiteralTupleValues(t *testing.T) {
	node, err := ParseLiteral("[('id', 'in', (1, 2, 3)), ('parent_id', '=', None)]")
	if err != nil {
		t.Fatal(err)
	}
	if node.Kind != All || len(node.Children) != 2 {
		t.Fatalf("node = %#v", node)
	}
	if !reflect.DeepEqual(node.Children[0].Value, []any{int64(1), int64(2), int64(3)}) {
		t.Fatalf("tuple value = %#v", node.Children[0].Value)
	}
	if node.Children[1].Value != nil {
		t.Fatalf("none value = %#v", node.Children[1].Value)
	}
}

func TestParseLiteralValuePreservesPythonDomainShape(t *testing.T) {
	value, err := ParseLiteralValue("[('active', '=', True)]")
	if err != nil {
		t.Fatal(err)
	}
	items, ok := value.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("value = %#v", value)
	}
	condition, ok := items[0].([]any)
	if !ok || len(condition) != 3 {
		t.Fatalf("condition = %#v", items[0])
	}
	if condition[0] != "active" || condition[1] != "=" || condition[2] != true {
		t.Fatalf("condition = %#v", condition)
	}
}

func TestParseLiteralRejectsMalformedDomain(t *testing.T) {
	if _, err := ParseLiteral("[('name', '=', 'unterminated)]"); err == nil {
		t.Fatal("expected malformed literal error")
	}
}
