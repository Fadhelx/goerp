package menu

import "testing"

func TestTreeFiltersGroups(t *testing.T) {
	reg := NewRegistry()
	root := reg.Add(Menu{Name: "Settings", Sequence: 10})
	reg.Add(Menu{Name: "Users", ParentID: root, Groups: []int64{1}})
	reg.Add(Menu{Name: "Hidden", ParentID: root, Groups: []int64{2}})
	tree := reg.Tree(map[int64]bool{1: true})
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].Menu.Name != "Users" {
		t.Fatalf("tree = %+v", tree)
	}
}

func TestTreeFilteredAppliesPredicate(t *testing.T) {
	reg := NewRegistry()
	root := reg.Add(Menu{Name: "Settings", Sequence: 10})
	reg.Add(Menu{Name: "Visible", ParentID: root, Groups: []int64{1}, ActionID: 10})
	reg.Add(Menu{Name: "Hidden", ParentID: root, Groups: []int64{1}, ActionID: 20})
	tree := reg.TreeFiltered(map[int64]bool{1: true}, func(item Menu) bool {
		return item.ActionID != 20
	})
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].Menu.Name != "Visible" {
		t.Fatalf("tree = %+v", tree)
	}
}
