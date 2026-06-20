package action

import "fmt"

type Kind string

const (
	ActWindow Kind = "ir.actions.act_window"
	Server    Kind = "ir.actions.server"
	Report    Kind = "ir.actions.report"
	Client    Kind = "ir.actions.client"
)

type Action struct {
	ID                int64
	XMLID             string
	Name              string
	Kind              Kind
	ResModel          string
	ResID             int64
	ViewMode          string
	ViewID            int64
	Views             []ViewRef
	SearchViewID      int64
	Domain            string
	Context           map[string]any
	Target            string
	Limit             int
	Help              string
	Path              string
	Groups            []int64
	BindingModelID    int64
	BindingType       string
	BindingViewTypes  string
	MobileViewMode    string
	Filter            bool
	Cache             bool
	Tag               string
	EmbeddedActions   []EmbeddedAction
	MultiWorkflowView string
}

type ViewRef struct {
	ID   int64
	Mode string
}

type EmbeddedAction struct {
	ID              int64
	Name            string
	ParentActionID  int64
	ParentResID     int64
	ParentResModel  string
	ActionID        int64
	PythonMethod    string
	UserID          int64
	IsDeletable     bool
	DefaultViewMode string
	FilterIDs       []int64
	Domain          string
	Context         string
	GroupIDs        []int64
}

type Registry struct {
	nextID  int64
	actions map[int64]Action
}

func NewRegistry() *Registry {
	return &Registry{nextID: 1, actions: map[int64]Action{}}
}

func (r *Registry) Add(action Action) (int64, error) {
	if action.Name == "" {
		return 0, fmt.Errorf("action requires name")
	}
	if action.Kind == "" {
		return 0, fmt.Errorf("action requires kind")
	}
	id := r.nextID
	r.nextID++
	action.ID = id
	if action.Context == nil {
		action.Context = map[string]any{}
	}
	if action.Kind == ActWindow && !action.Cache {
		action.Cache = true
	}
	r.actions[id] = action
	return id, nil
}

func (r *Registry) AddWithID(action Action) error {
	if action.ID <= 0 {
		return fmt.Errorf("action requires id")
	}
	if action.Name == "" {
		return fmt.Errorf("action requires name")
	}
	if action.Kind == "" {
		return fmt.Errorf("action requires kind")
	}
	if action.Context == nil {
		action.Context = map[string]any{}
	}
	if action.Kind == ActWindow && !action.Cache {
		action.Cache = true
	}
	r.actions[action.ID] = action
	if action.ID >= r.nextID {
		r.nextID = action.ID + 1
	}
	return nil
}

func (r *Registry) Get(id int64) (Action, bool) {
	action, ok := r.actions[id]
	return action, ok
}

func (r *Registry) All() []Action {
	out := make([]Action, 0, len(r.actions))
	for _, action := range r.actions {
		out = append(out, action)
	}
	return out
}

func (r *Registry) FindByPath(path string) (Action, bool) {
	for _, action := range r.actions {
		if action.Path == path {
			return action, true
		}
	}
	return Action{}, false
}

func (r *Registry) FindByXMLID(xmlID string) (Action, bool) {
	for _, action := range r.actions {
		if action.XMLID == xmlID {
			return action, true
		}
	}
	return Action{}, false
}
