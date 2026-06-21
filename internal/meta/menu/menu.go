package menu

import "sort"

type Menu struct {
	ID                  int64
	Name                string
	ParentID            int64
	Action              string
	ActionID            int64
	ActionModel         string
	ActionPath          string
	Groups              []int64
	Sequence            int
	WebIcon             string
	WebIconData         string
	WebIconDataMimetype string
	XMLID               string
}

type Node struct {
	Menu     Menu
	Children []Node
}

type Registry struct {
	nextID int64
	menus  map[int64]Menu
}

func NewRegistry() *Registry {
	return &Registry{nextID: 1, menus: map[int64]Menu{}}
}

func (r *Registry) Add(menu Menu) int64 {
	id := r.nextID
	r.nextID++
	menu.ID = id
	r.menus[id] = menu
	return id
}

func (r *Registry) AddWithID(menu Menu) {
	if menu.ID <= 0 {
		menu.ID = r.nextID
	}
	r.menus[menu.ID] = menu
	if menu.ID >= r.nextID {
		r.nextID = menu.ID + 1
	}
}

func (r *Registry) Get(id int64) (Menu, bool) {
	menu, ok := r.menus[id]
	return menu, ok
}

func (r *Registry) All() []Menu {
	out := make([]Menu, 0, len(r.menus))
	for _, menu := range r.menus {
		out = append(out, menu)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].Name < out[j].Name
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func (r *Registry) Tree(groupIDs map[int64]bool) []Node {
	return r.TreeFiltered(groupIDs, nil)
}

func (r *Registry) TreeFiltered(groupIDs map[int64]bool, include func(Menu) bool) []Node {
	var roots []Node
	for _, menu := range r.menus {
		if menu.ParentID != 0 || !allowed(menu.Groups, groupIDs) {
			continue
		}
		if node, ok := r.nodeFiltered(menu, groupIDs, include); ok {
			roots = append(roots, node)
		}
	}
	sortNodes(roots)
	return roots
}

func (r *Registry) node(menu Menu, groupIDs map[int64]bool) Node {
	node, _ := r.nodeFiltered(menu, groupIDs, nil)
	return node
}

func (r *Registry) nodeFiltered(menu Menu, groupIDs map[int64]bool, include func(Menu) bool) (Node, bool) {
	var children []Node
	hadChildren := false
	for _, child := range r.menus {
		if child.ParentID != menu.ID {
			continue
		}
		hadChildren = true
		if !allowed(child.Groups, groupIDs) {
			continue
		}
		if node, ok := r.nodeFiltered(child, groupIDs, include); ok {
			children = append(children, node)
		}
	}
	sortNodes(children)
	if include != nil {
		selfIncluded := included(menu, include)
		if !selfIncluded && len(children) == 0 {
			return Node{}, false
		}
		if selfIncluded && !hasAction(menu) && hadChildren && len(children) == 0 {
			return Node{}, false
		}
	}
	return Node{Menu: menu, Children: children}, true
}

func included(menu Menu, include func(Menu) bool) bool {
	return include == nil || include(menu)
}

func hasAction(menu Menu) bool {
	return menu.ActionID != 0 || menu.Action != ""
}

func allowed(required []int64, groups map[int64]bool) bool {
	if len(required) == 0 {
		return true
	}
	for _, group := range required {
		if groups[group] {
			return true
		}
	}
	return false
}

func sortNodes(nodes []Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Menu.Sequence == nodes[j].Menu.Sequence {
			return nodes[i].Menu.Name < nodes[j].Menu.Name
		}
		return nodes[i].Menu.Sequence < nodes[j].Menu.Sequence
	})
}
