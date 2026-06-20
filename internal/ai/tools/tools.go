package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
)

var (
	ErrToolNotFound     = errors.New("ai tool not found")
	ErrToolForbidden    = errors.New("ai tool forbidden")
	ErrSchemaValidation = errors.New("ai tool schema validation failed")
)

type SchemaType string

const (
	TypeString SchemaType = "string"
	TypeNumber SchemaType = "number"
	TypeBool   SchemaType = "boolean"
	TypeInt    SchemaType = "integer"
	TypeArray  SchemaType = "array"
	TypeObject SchemaType = "object"
	TypeNull   SchemaType = "null"
)

type Field struct {
	Type               SchemaType
	Required           bool
	Description        string
	Pattern            string
	Enum               []any
	MaxLength          int
	Items              *Field
	AnyOf              []Field
	Properties         Schema
	RequiredProperties []string
}

type Schema map[string]Field

type Request struct {
	UserID    int64
	CompanyID int64
	Model     string
	RecordID  int64
	RecordIDs []int64
	ToolName  string
	Input     map[string]any
	Metadata  map[string]any
}

type Result struct {
	Output map[string]any
}

type Handler func(context.Context, Request) (Result, error)

type Authorizer interface {
	CanRunTool(context.Context, Request) bool
}

type Audit interface {
	RecordToolCall(Request, string)
}

type Tool struct {
	Name        string
	Description string
	Schema      Schema
	Handler     Handler
}

type Registry struct {
	tools map[string]Tool
	auth  Authorizer
	audit Audit
}

func NewRegistry(auth Authorizer, audit Audit) *Registry {
	return &Registry{tools: map[string]Tool{}, auth: auth, audit: audit}
}

func (r *Registry) Register(tool Tool) error {
	if tool.Name == "" {
		return fmt.Errorf("tool requires name")
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool %s requires handler", tool.Name)
	}
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name)
	}
	tool.Schema = cloneSchema(tool.Schema)
	r.tools[tool.Name] = tool
	return nil
}

func (r *Registry) Run(ctx context.Context, request Request) (Result, error) {
	tool, ok := r.tools[request.ToolName]
	if !ok {
		return Result{}, ErrToolNotFound
	}
	if r.auth == nil || !r.auth.CanRunTool(ctx, request) {
		r.record(request, "denied")
		return Result{}, ErrToolForbidden
	}
	if err := Validate(tool.Schema, request.Input); err != nil {
		r.record(request, "schema_denied")
		return Result{}, err
	}
	result, err := tool.Handler(ctx, request)
	if err != nil {
		r.record(request, "error")
		return result, err
	}
	r.record(request, "allowed")
	return result, nil
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Validate(schema Schema, input map[string]any) error {
	if input == nil {
		input = map[string]any{}
	}
	for name := range input {
		if _, exists := schema[name]; !exists {
			return fmt.Errorf("%w: unexpected %s", ErrSchemaValidation, name)
		}
	}
	for name, field := range schema {
		value, exists := input[name]
		if field.Required && !exists {
			return fmt.Errorf("%w: missing %s", ErrSchemaValidation, name)
		}
		if !exists || value == nil {
			if exists && field.Required && field.Type != TypeNull {
				return fmt.Errorf("%w: %s", ErrSchemaValidation, name)
			}
			if exists && field.Type != TypeNull {
				continue
			}
			if exists && field.Type == TypeNull {
				continue
			}
			continue
		}
		normalized, err := validateValue(name, field, value)
		if err != nil {
			return err
		}
		input[name] = normalized
	}
	return nil
}

func ValidateSchema(schema Schema) error {
	if schema == nil {
		return fmt.Errorf("%w: missing properties", ErrSchemaValidation)
	}
	for name, field := range schema {
		if err := validateFieldDefinition(name, field, false); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldDefinition(name string, field Field, objectProperty bool) error {
	if field.Type == "" {
		return fmt.Errorf("%w: missing type for %s", ErrSchemaValidation, name)
	}
	if !supportedType(field.Type) {
		return fmt.Errorf("%w: unsupported type %s for %s", ErrSchemaValidation, field.Type, name)
	}
	if field.Pattern != "" && field.Type != TypeString && field.Type != TypeArray {
		return fmt.Errorf("%w: pattern only supported for string or string array %s", ErrSchemaValidation, name)
	}
	if field.Type == TypeArray {
		if field.Items == nil && len(field.AnyOf) == 0 {
			return fmt.Errorf("%w: array %s requires items", ErrSchemaValidation, name)
		}
		if field.Pattern != "" {
			if field.Items == nil || field.Items.Type != TypeString || len(field.AnyOf) > 0 {
				return fmt.Errorf("%w: array pattern requires string items for %s", ErrSchemaValidation, name)
			}
		}
		if field.Items != nil {
			if err := validateArrayItemDefinition(name, *field.Items); err != nil {
				return err
			}
		}
		for _, item := range field.AnyOf {
			if err := validateArrayItemDefinition(name, item); err != nil {
				return err
			}
		}
	}
	if field.Type == TypeObject {
		if objectProperty {
			return fmt.Errorf("%w: nested objects are not supported for %s", ErrSchemaValidation, name)
		}
		if field.Properties == nil {
			return fmt.Errorf("%w: object %s requires properties", ErrSchemaValidation, name)
		}
		required := map[string]bool{}
		for _, property := range field.RequiredProperties {
			if _, ok := field.Properties[property]; !ok {
				return fmt.Errorf("%w: missing required property definition %s", ErrSchemaValidation, property)
			}
			required[property] = true
		}
		for propertyName, property := range field.Properties {
			property.Required = required[propertyName]
			if err := validateFieldDefinition(propertyName, property, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateArrayItemDefinition(name string, field Field) error {
	if field.Type == TypeArray || field.Type == TypeObject {
		return fmt.Errorf("%w: array %s item type %s is not supported", ErrSchemaValidation, name, field.Type)
	}
	return validateFieldDefinition(name, field, false)
}

func validateValue(name string, field Field, value any) (any, error) {
	if !matchesType(value, field.Type) {
		return nil, fmt.Errorf("%w: %s", ErrSchemaValidation, name)
	}
	if len(field.Enum) > 0 && !inEnum(value, field.Enum) {
		return nil, fmt.Errorf("%w: enum %s", ErrSchemaValidation, name)
	}
	switch field.Type {
	case TypeString:
		text := value.(string)
		if field.Pattern != "" {
			ok, err := regexp.MatchString("^("+field.Pattern+")$", text)
			if err != nil {
				return nil, fmt.Errorf("%w: bad pattern %s", ErrSchemaValidation, name)
			}
			if !ok {
				return nil, fmt.Errorf("%w: pattern %s", ErrSchemaValidation, name)
			}
		}
		if field.MaxLength > 0 && len(text) > field.MaxLength {
			return text[:field.MaxLength] + "...", nil
		}
		return text, nil
	case TypeInt:
		if number, ok := normalizeInteger(value); ok {
			return number, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrSchemaValidation, name)
	case TypeArray:
		values := reflect.ValueOf(value)
		for i := 0; i < values.Len(); i++ {
			itemValue := values.Index(i).Interface()
			if err := validateArrayValue(name, field, itemValue); err != nil {
				return nil, err
			}
		}
		return value, nil
	case TypeObject:
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrSchemaValidation, name)
		}
		objectSchema := cloneSchema(field.Properties)
		for _, required := range field.RequiredProperties {
			property := objectSchema[required]
			property.Required = true
			objectSchema[required] = property
		}
		if err := Validate(objectSchema, object); err != nil {
			return nil, err
		}
		return object, nil
	default:
		return value, nil
	}
}

func validateArrayValue(name string, field Field, value any) error {
	candidates := field.AnyOf
	if field.Items != nil {
		candidates = append([]Field{*field.Items}, candidates...)
	}
	for _, candidate := range candidates {
		if _, err := validateValue(name, candidate, value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%w: array item %s", ErrSchemaValidation, name)
}

func (r *Registry) record(request Request, result string) {
	if r.audit != nil {
		r.audit.RecordToolCall(request, result)
	}
}

func matchesType(value any, schemaType SchemaType) bool {
	switch schemaType {
	case TypeString:
		_, ok := value.(string)
		return ok
	case TypeBool:
		_, ok := value.(bool)
		return ok
	case TypeInt:
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
			return true
		case uint64:
			return value.(uint64) <= math.MaxInt64
		case float32:
			float := float64(value.(float32))
			return math.Trunc(float) == float
		case float64:
			float := value.(float64)
			return math.Trunc(float) == float
		default:
			return false
		}
	case TypeNumber:
		kind := reflect.TypeOf(value).Kind()
		return kind >= reflect.Int && kind <= reflect.Float64 && kind != reflect.Bool
	case TypeArray:
		if value == nil {
			return false
		}
		kind := reflect.TypeOf(value).Kind()
		return kind == reflect.Slice || kind == reflect.Array
	case TypeObject:
		_, ok := value.(map[string]any)
		return ok
	case TypeNull:
		return value == nil
	default:
		if schemaType == "bool" {
			_, ok := value.(bool)
			return ok
		}
		if schemaType == "int" {
			switch value.(type) {
			case int, int8, int16, int32, int64:
				return true
			default:
				return false
			}
		}
		return false
	}
}

func normalizeInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int8:
		return int64(number), true
	case int16:
		return int64(number), true
	case int32:
		return int64(number), true
	case int64:
		return number, true
	case uint:
		if uint64(number) <= math.MaxInt64 {
			return int64(number), true
		}
	case uint8:
		return int64(number), true
	case uint16:
		return int64(number), true
	case uint32:
		return int64(number), true
	case uint64:
		if number <= math.MaxInt64 {
			return int64(number), true
		}
	case float32:
		float := float64(number)
		if math.Trunc(float) == float {
			return int64(float), true
		}
	case float64:
		if math.Trunc(number) == number {
			return int64(number), true
		}
	}
	return 0, false
}

func cloneSchema(schema Schema) Schema {
	out := make(Schema, len(schema))
	for key, value := range schema {
		if value.Items != nil {
			item := *value.Items
			value.Items = &item
		}
		value.AnyOf = append([]Field(nil), value.AnyOf...)
		value.RequiredProperties = append([]string(nil), value.RequiredProperties...)
		value.Enum = append([]any(nil), value.Enum...)
		value.Properties = cloneSchema(value.Properties)
		out[key] = value
	}
	return out
}

func supportedType(schemaType SchemaType) bool {
	switch schemaType {
	case TypeString, TypeNumber, TypeBool, TypeInt, TypeArray, TypeObject, TypeNull:
		return true
	default:
		return schemaType == "bool" || schemaType == "int"
	}
}

func inEnum(value any, enum []any) bool {
	for _, candidate := range enum {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}
