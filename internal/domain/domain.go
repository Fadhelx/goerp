package domain

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

type Node struct {
	Kind     Kind
	Field    string
	Operator Operator
	Value    any
	Children []Node
}

type Kind string

const (
	Condition Kind = "condition"
	All       Kind = "and"
	Any       Kind = "or"
	None      Kind = "not"
	Literal   Kind = "literal"
)

type Operator string

const (
	Equal         Operator = "="
	NotEqual      Operator = "!="
	In            Operator = "in"
	NotIn         Operator = "not in"
	Less          Operator = "<"
	LessEqual     Operator = "<="
	Greater       Operator = ">"
	GreaterEqual  Operator = ">="
	Like          Operator = "like"
	NotLike       Operator = "not like"
	ILike         Operator = "ilike"
	NotILike      Operator = "not ilike"
	EqualLike     Operator = "=like"
	NotEqualLike  Operator = "not =like"
	EqualILike    Operator = "=ilike"
	NotEqualILike Operator = "not =ilike"
	OptionalEqual Operator = "=?"
	ChildOf       Operator = "child_of"
	ParentOf      Operator = "parent_of"
	AnyOf         Operator = "any"
	NotAnyOf      Operator = "not any"
)

var fieldNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)*$`)

func Cond(field string, op Operator, value any) Node {
	return Node{Kind: Condition, Field: field, Operator: op, Value: value}
}

func And(children ...Node) Node {
	return Node{Kind: All, Children: children}
}

func Or(children ...Node) Node {
	return Node{Kind: Any, Children: children}
}

func Not(child Node) Node {
	return Node{Kind: None, Children: []Node{child}}
}

func Bool(value bool) Node {
	return Node{Kind: Literal, Value: value}
}

func Parse(value any) (Node, error) {
	if value == nil {
		return And(), nil
	}
	switch typed := value.(type) {
	case Node:
		return typed, nil
	case []Node:
		return And(typed...), nil
	case []any:
		return parseList(typed)
	case []string:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = item
		}
		return parseList(items)
	default:
		return Node{}, fmt.Errorf("invalid domain value %T", value)
	}
}

type SQL struct {
	Query string
	Args  []any
}

func CompileSQL(node Node) (SQL, error) {
	var args []any
	query, err := compile(node, &args)
	if err != nil {
		return SQL{}, err
	}
	return SQL{Query: query, Args: args}, nil
}

func compile(node Node, args *[]any) (string, error) {
	switch node.Kind {
	case Literal:
		if truthy(node.Value) {
			return "TRUE", nil
		}
		return "FALSE", nil
	case Condition:
		return compileCondition(node, args)
	case All, Any:
		parts := make([]string, 0, len(node.Children))
		for _, child := range node.Children {
			part, err := compile(child, args)
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+part+")")
		}
		if len(parts) == 0 && node.Kind == All {
			return "TRUE", nil
		}
		if len(parts) == 0 {
			return "FALSE", nil
		}
		joiner := " AND "
		if node.Kind == Any {
			joiner = " OR "
		}
		return strings.Join(parts, joiner), nil
	case None:
		if len(node.Children) != 1 {
			return "", fmt.Errorf("not domain requires exactly one child")
		}
		part, err := compile(node.Children[0], args)
		if err != nil {
			return "", err
		}
		return "NOT (" + part + ")", nil
	default:
		return "", fmt.Errorf("unsupported domain kind %q", node.Kind)
	}
}

func compileCondition(node Node, args *[]any) (string, error) {
	if !fieldNamePattern.MatchString(node.Field) {
		return "", fmt.Errorf("invalid field name %q", node.Field)
	}
	column := strings.ReplaceAll(node.Field, ".", "__")
	switch node.Operator {
	case Equal, NotEqual, Less, LessEqual, Greater, GreaterEqual:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s %s $%d", column, node.Operator, len(*args)), nil
	case OptionalEqual:
		if !truthy(node.Value) {
			return "TRUE", nil
		}
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s = $%d", column, len(*args)), nil
	case Like:
		*args = append(*args, "%"+fmt.Sprint(node.Value)+"%")
		return fmt.Sprintf("%s LIKE $%d", column, len(*args)), nil
	case NotLike:
		*args = append(*args, "%"+fmt.Sprint(node.Value)+"%")
		return fmt.Sprintf("%s NOT LIKE $%d", column, len(*args)), nil
	case ILike:
		*args = append(*args, "%"+fmt.Sprint(node.Value)+"%")
		return fmt.Sprintf("%s ILIKE $%d", column, len(*args)), nil
	case NotILike:
		*args = append(*args, "%"+fmt.Sprint(node.Value)+"%")
		return fmt.Sprintf("%s NOT ILIKE $%d", column, len(*args)), nil
	case EqualLike:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s LIKE $%d", column, len(*args)), nil
	case NotEqualLike:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s NOT LIKE $%d", column, len(*args)), nil
	case EqualILike:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s ILIKE $%d", column, len(*args)), nil
	case NotEqualILike:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s NOT ILIKE $%d", column, len(*args)), nil
	case In, NotIn:
		values, err := sliceValues(node.Value)
		if err != nil {
			return "", err
		}
		if len(values) == 0 {
			if node.Operator == In {
				return "FALSE", nil
			}
			return "TRUE", nil
		}
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			*args = append(*args, value)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(*args)))
		}
		sqlOp := "IN"
		if node.Operator == NotIn {
			sqlOp = "NOT IN"
		}
		return fmt.Sprintf("%s %s (%s)", column, sqlOp, strings.Join(placeholders, ", ")), nil
	case ChildOf:
		values, err := sliceValues(node.Value)
		if err == nil {
			if len(values) == 0 {
				return "FALSE", nil
			}
			placeholders := make([]string, 0, len(values))
			for _, value := range values {
				*args = append(*args, value)
				placeholders = append(placeholders, fmt.Sprintf("$%d", len(*args)))
			}
			return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")), nil
		}
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s = $%d", column, len(*args)), nil
	case ParentOf:
		*args = append(*args, node.Value)
		return fmt.Sprintf("%s = $%d", column, len(*args)), nil
	case AnyOf, NotAnyOf:
		return "", fmt.Errorf("operator %q requires relational query support", node.Operator)
	default:
		return "", fmt.Errorf("unsupported operator %q", node.Operator)
	}
}

func parseList(items []any) (Node, error) {
	if len(items) == 0 {
		return And(), nil
	}
	if len(items) == 3 {
		if _, ok := items[1].(string); ok {
			if node, ok, err := parseCondition(items); ok || err != nil {
				return node, err
			}
		}
	}
	stack := make([]Node, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if op, ok := item.(string); ok {
			switch op {
			case "&":
				left, right, err := popBinary(&stack, op)
				if err != nil {
					return Node{}, err
				}
				stack = append(stack, And(left, right))
				continue
			case "|":
				left, right, err := popBinary(&stack, op)
				if err != nil {
					return Node{}, err
				}
				stack = append(stack, Or(left, right))
				continue
			case "!":
				child, err := popUnary(&stack, op)
				if err != nil {
					return Node{}, err
				}
				stack = append(stack, Not(child))
				continue
			default:
				return Node{}, fmt.Errorf("invalid domain operator %q", op)
			}
		}
		node, ok, err := parseToken(item)
		if err != nil {
			return Node{}, err
		}
		if !ok {
			return Node{}, fmt.Errorf("invalid domain item %v", item)
		}
		stack = append(stack, node)
	}
	if len(stack) == 1 {
		return stack[0], nil
	}
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return And(stack...), nil
}

func parseToken(item any) (Node, bool, error) {
	switch typed := item.(type) {
	case Node:
		return typed, true, nil
	case []any:
		return parseCondition(typed)
	case []string:
		items := make([]any, len(typed))
		for i, value := range typed {
			items[i] = value
		}
		return parseCondition(items)
	default:
		return Node{}, false, nil
	}
}

func parseCondition(parts []any) (Node, bool, error) {
	if len(parts) != 3 {
		return Node{}, false, nil
	}
	if boolNode, ok := parseBooleanLeaf(parts); ok {
		return boolNode, true, nil
	}
	field, ok := parts[0].(string)
	if !ok || field == "" {
		return Node{}, true, fmt.Errorf("invalid domain field")
	}
	opText, ok := parts[1].(string)
	if !ok {
		return Node{}, true, fmt.Errorf("invalid domain operator")
	}
	op, err := NormalizeOperator(opText)
	if err != nil {
		return Node{}, true, err
	}
	if op == OptionalEqual && !truthy(parts[2]) {
		return Bool(true), true, nil
	}
	return Cond(field, op, parts[2]), true, nil
}

func parseBooleanLeaf(parts []any) (Node, bool) {
	if NormalizeScalar(parts[1]) != "=" {
		return Node{}, false
	}
	left := NormalizeScalar(parts[0])
	right := NormalizeScalar(parts[2])
	if left == int64(1) && right == int64(1) {
		return Bool(true), true
	}
	if left == int64(0) && right == int64(1) {
		return Bool(false), true
	}
	return Node{}, false
}

func popBinary(stack *[]Node, op string) (Node, Node, error) {
	if len(*stack) < 2 {
		return Node{}, Node{}, fmt.Errorf("malformed domain near %q", op)
	}
	left := (*stack)[len(*stack)-1]
	right := (*stack)[len(*stack)-2]
	*stack = (*stack)[:len(*stack)-2]
	return left, right, nil
}

func popUnary(stack *[]Node, op string) (Node, error) {
	if len(*stack) < 1 {
		return Node{}, fmt.Errorf("malformed domain near %q", op)
	}
	child := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	return child, nil
}

func NormalizeOperator(op string) (Operator, error) {
	switch Operator(strings.TrimSpace(op)) {
	case "<>":
		return NotEqual, nil
	case Equal, NotEqual, In, NotIn, Less, LessEqual, Greater, GreaterEqual,
		Like, NotLike, ILike, NotILike, EqualLike, NotEqualLike,
		EqualILike, NotEqualILike, OptionalEqual, ChildOf, ParentOf, AnyOf, NotAnyOf:
		return Operator(strings.TrimSpace(op)), nil
	default:
		return "", fmt.Errorf("unsupported operator %q", op)
	}
}

func NormalizeScalar(value any) any {
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
	case float32:
		if typed == float32(int64(typed)) {
			return int64(typed)
		}
		return float64(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed)
		}
		return typed
	default:
		return value
	}
}

func truthy(value any) bool {
	switch typed := NormalizeScalar(value).(type) {
	case nil:
		return false
	case bool:
		return typed
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed != ""
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() > 0
		default:
			return true
		}
	}
}

func sliceValues(value any) ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("in operator requires slice value")
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("in operator requires slice value")
	}
	values := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		values = append(values, rv.Index(i).Interface())
	}
	return values, nil
}
