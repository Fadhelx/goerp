package action

import "testing"

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	id, err := reg.Add(Action{Name: "Partners", XMLID: "base.action_partner", Kind: ActWindow, ResModel: "res.partner", ViewMode: "list,form", Path: "partners"})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := reg.Get(id)
	if !ok || action.ResModel != "res.partner" {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
	if found, ok := reg.FindByPath("partners"); !ok || found.ID != id {
		t.Fatalf("FindByPath = %+v ok=%v", found, ok)
	}
	if found, ok := reg.FindByXMLID("base.action_partner"); !ok || found.ID != id {
		t.Fatalf("FindByXMLID = %+v ok=%v", found, ok)
	}
	if len(reg.All()) != 1 {
		t.Fatalf("all = %+v", reg.All())
	}
}
