package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(`modules:
  - name: base
    root: /tmp/base
    version: "19.0"
    license: LGPL-3
    priority: P1
    include:
      - .py
      - .xml
    exclude:
      - __pycache__
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Modules) != 1 {
		t.Fatalf("modules = %d", len(cfg.Modules))
	}
	module := cfg.Modules[0]
	if module.Name != "base" || module.Root != "/tmp/base" || module.Version != "19.0" {
		t.Fatalf("unexpected module: %+v", module)
	}
	if len(module.Include) != 2 || module.Include[0] != ".py" {
		t.Fatalf("unexpected include: %+v", module.Include)
	}
}

func TestInventoryModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "__manifest__.py"), []byte(`{
    "name": "Base",
    "depends": ["mail", "web"],
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models", "res_users.py"), []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "__pycache__"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "__pycache__", "skip.py"), []byte("skip\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	records, err := inventoryModule(ModuleConfig{
		Name:    "base",
		Root:    dir,
		Include: []string{".py"},
		Exclude: []string{"__pycache__"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d", len(records))
	}
	var model Record
	for _, record := range records {
		if record.Path == "models/res_users.py" {
			model = record
			break
		}
	}
	if model.Path != "models/res_users.py" || model.Kind != "model" || model.Lines != 2 || len(model.ManifestDepends) != 2 {
		t.Fatalf("unexpected record: %+v", model)
	}
}

func TestParseManifestDepends(t *testing.T) {
	depends := parseManifestDepends(`{
    'name': 'Workflow',
    'depends': ['mail', 'base_automation', 'oi_base'],
}`)
	want := []string{"mail", "base_automation", "oi_base"}
	if len(depends) != len(want) {
		t.Fatalf("depends = %+v", depends)
	}
	for i := range want {
		if depends[i] != want[i] {
			t.Fatalf("depends = %+v", depends)
		}
	}
}
