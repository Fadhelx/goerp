package model

import (
	"fmt"
	"regexp"

	"gorp/internal/field"
)

var modelNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

type Model struct {
	Name                 string
	Table                string
	Inherit              []string
	Inherits             map[string]string
	RecName              string
	Order                string
	Abstract             bool
	Transient            bool
	DefaultFieldReadonly string
	Fields               map[string]field.Field
}

func New(name, table string) Model {
	return Model{
		Name:     name,
		Table:    table,
		RecName:  "name",
		Order:    "id",
		Inherits: map[string]string{},
		Fields:   map[string]field.Field{},
	}
}

func (m *Model) AddField(f field.Field) {
	m.Fields[f.Name] = f
}

func (m Model) Validate() error {
	if !modelNamePattern.MatchString(m.Name) {
		return fmt.Errorf("invalid model name %q", m.Name)
	}
	if m.Table == "" && !m.Abstract {
		return fmt.Errorf("model %s requires table", m.Name)
	}
	for name := range m.Fields {
		if name == "" {
			return fmt.Errorf("model %s has empty field name", m.Name)
		}
	}
	return nil
}

func (m Model) Compose(parent Model) Model {
	out := m
	if out.Fields == nil {
		out.Fields = map[string]field.Field{}
	}
	for name, f := range parent.Fields {
		if _, exists := out.Fields[name]; !exists {
			out.Fields[name] = f
		}
	}
	return out
}
