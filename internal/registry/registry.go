package registry

import (
	"fmt"

	"gorp/internal/model"
	"gorp/internal/module"
)

type HookKind string

const (
	OnCreate    HookKind = "create"
	OnWrite     HookKind = "write"
	OnUnlink    HookKind = "unlink"
	OnArchive   HookKind = "archive"
	OnUnarchive HookKind = "unarchive"
	OnMessage   HookKind = "message"
	OnTime      HookKind = "time"
)

type Hook struct {
	Module string
	Model  string
	Kind   HookKind
	Name   string
}

type ExternalID struct {
	Module string
	Name   string
	Model  string
	ResID  int64
}

type Registry struct {
	Database   string
	Modules    map[string]module.Manifest
	States     map[string]string
	Models     map[string]model.Model
	ExternalID map[string]ExternalID
	Hooks      map[HookKind][]Hook
}

func New(database string) *Registry {
	return &Registry{
		Database:   database,
		Modules:    map[string]module.Manifest{},
		States:     map[string]string{},
		Models:     map[string]model.Model{},
		ExternalID: map[string]ExternalID{},
		Hooks:      map[HookKind][]Hook{},
	}
}

func (r *Registry) Install(manifests []module.Manifest) error {
	ordered, err := module.SortByDependencies(manifests)
	if err != nil {
		return err
	}
	for _, manifest := range ordered {
		r.Modules[manifest.TechnicalName] = manifest
		r.States[manifest.TechnicalName] = "installed"
	}
	return nil
}

func (r *Registry) RegisterModel(m model.Model) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if _, exists := r.Models[m.Name]; exists {
		return fmt.Errorf("model %s already registered", m.Name)
	}
	r.Models[m.Name] = m
	return nil
}

func (r *Registry) RegisterExternalID(id ExternalID) {
	r.ExternalID[id.Module+"."+id.Name] = id
}

func (r *Registry) RegisterHook(h Hook) {
	r.Hooks[h.Kind] = append(r.Hooks[h.Kind], h)
}

func (r *Registry) Invalidate() {
	r.Hooks = map[HookKind][]Hook{}
}
