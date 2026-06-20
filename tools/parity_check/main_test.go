package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDetectsMissingCoverage(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[{"module":"base","path":"models/res_users.py","kind":"model"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte("records:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, false, false); err == nil {
		t.Fatal("expected missing coverage error")
	}
}

func TestRunAcceptsCoveredFile(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[{"module":"base","path":"models/res_users.py","kind":"model"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte(`records:
  - module: base
    path: models/res_users.py
    feature: users
    target: internal/base/users.go
    status: pending
    reason: planned
    verification: go test ./internal/base
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsInvalidStatus(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[{"module":"base","path":"models/res_users.py","kind":"model"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte(`records:
  - module: base
    path: models/res_users.py
    feature: users
    target: internal/base
    status: done
    reason: invalid
    verification: go test ./internal/base
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, false, false); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestRunWriteMissingCoverage(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[{"module":"base","path":"models/res_users.py","kind":"model","priority":"P1"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte("records:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, true, false); err != nil {
		t.Fatal(err)
	}
	if err := run(inventory, coverage, false, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(coverage)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "records:\n" {
		t.Fatal("coverage was not updated")
	}
}

func TestRunSkipsStaticInventory(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[{"module":"web","path":"static/src/foo.js","kind":"static"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte("records:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestRunRewriteCoverageMarksOIImplemented(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	coverage := filepath.Join(dir, "parity.yaml")

	if err := os.WriteFile(inventory, []byte(`[
{"module":"oi_workflow","path":"models/approval_record.py","kind":"model","priority":"P2"},
{"module":"oi_workflow","path":"models/__init__.py","kind":"model","priority":"P2"}
]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverage, []byte("records:\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(inventory, coverage, false, true); err != nil {
		t.Fatal(err)
	}
	if err := run(inventory, coverage, false, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(coverage)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "status: implemented") || !strings.Contains(text, "status: intentionally_omitted") {
		t.Fatalf("coverage = %s", text)
	}
}
