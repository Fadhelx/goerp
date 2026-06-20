package registry

import (
	"testing"

	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
)

func TestRegistryInstallAndModel(t *testing.T) {
	reg := New("test")
	err := reg.Install([]module.Manifest{
		{Name: "Web", TechnicalName: "web", Version: "19.0", Depends: []string{"base"}},
		{Name: "Base", TechnicalName: "base", Version: "19.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.States["base"] != "installed" || reg.States["web"] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}

	users := model.New("res.users", "res_users")
	users.AddField(field.New("login", field.Char))
	if err := reg.RegisterModel(users); err != nil {
		t.Fatal(err)
	}
	if reg.Models["res.users"].Fields["login"].Kind != field.Char {
		t.Fatalf("model not registered: %+v", reg.Models)
	}
}

func TestRegistryHooks(t *testing.T) {
	reg := New("test")
	reg.RegisterHook(Hook{Module: "base_automation", Model: "res.partner", Kind: OnWrite, Name: "automation"})
	if len(reg.Hooks[OnWrite]) != 1 {
		t.Fatalf("hooks = %+v", reg.Hooks)
	}
	reg.Invalidate()
	if len(reg.Hooks) != 0 {
		t.Fatalf("hooks not invalidated: %+v", reg.Hooks)
	}
}
