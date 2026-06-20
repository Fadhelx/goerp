package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBacklogFindsDoneSlicesAndLatestGaps(t *testing.T) {
	text := strings.Join([]string{
		"- DONE this slice: closed WhatsApp tracked links. Focused tests passed. Full `make ci` passed. Remaining deferred gaps: sms composer.",
		"- DONE this slice: closed link tracker parity. Focused HTTP tests, affected package tests, and full CI passed. Remaining deferred gaps: exact digest mail layout, WhatsApp `/r/<code>/w/<message_id>` parity, and full Odoo route validation semantics.",
		"- DONE this slice: added base security. Focused tests passed.",
	}, "\n")

	got := parseBacklog(text)
	if len(got.DoneSlices) != 3 {
		t.Fatalf("done slices = %d, want 3", len(got.DoneSlices))
	}
	if got.DoneSlices[0].Number != 1 {
		t.Fatalf("first rendered slice number = %d, want newest slice 1", got.DoneSlices[0].Number)
	}
	if got.LatestSlice.Line != 1 {
		t.Fatalf("latest slice line = %d, want 1", got.LatestSlice.Line)
	}
	if got.LatestSlice.Verification != "full CI passed" {
		t.Fatalf("verification = %q, want full CI passed", got.LatestSlice.Verification)
	}
	if len(got.RemainingGaps) != 1 {
		t.Fatalf("remaining gaps = %d, want 1", len(got.RemainingGaps))
	}
	if got.RemainingGaps[0].Text != "sms composer" {
		t.Fatalf("first gap = %q", got.RemainingGaps[0].Text)
	}
}

func TestVerificationLabelHandlesFocusedPackageList(t *testing.T) {
	got := verificationLabel("Focused base/runtime/http/dashboard tests passed.")
	if got != "focused tests passed" {
		t.Fatalf("verification = %q, want focused tests passed", got)
	}
}

func TestSummarizeParityCountsStatusesByModule(t *testing.T) {
	records := parseParity(`records:
  - module: base
    path: "__manifest__.py"
    feature: "module manifest"
    status: implemented
  - module: base
    path: "models/res_users.py"
    feature: "model"
    status: pending
  - module: mail
    path: "__init__.py"
    feature: "module"
    status: intentionally_omitted
  - module: mail
    path: "models/mail_thread.py"
    feature: "model"
    status: blocked
`)

	got := summarizeParity(records)
	if got.Total != 4 || got.Implemented != 1 || got.Pending != 1 || got.Omitted != 1 || got.Blocked != 1 {
		t.Fatalf("summary = %+v", got)
	}
	if got.Closed != 2 {
		t.Fatalf("closed = %d, want 2", got.Closed)
	}
	if len(got.Modules) != 2 {
		t.Fatalf("modules = %d, want 2", len(got.Modules))
	}
}

func TestRunWritesDashboardHTML(t *testing.T) {
	dir := t.TempDir()
	backlogPath := filepath.Join(dir, "agent_audit_backlog.md")
	parityPath := filepath.Join(dir, "parity.yaml")
	inventoryPath := filepath.Join(dir, "source_inventory.json")
	oiPath := filepath.Join(dir, "oi_inventory.json")
	outPath := filepath.Join(dir, "progress_dashboard.html")

	writeFile(t, backlogPath, "- DONE this slice: closed bounded build dashboard. make ci passed. Remaining deferred gaps: next gap, and final gap.\n")
	writeFile(t, parityPath, `records:
  - module: base
    path: "__manifest__.py"
    feature: "module manifest"
    status: implemented
  - module: account
    path: "__manifest__.py"
    feature: "module manifest"
    status: pending
`)
	writeFile(t, inventoryPath, `[
  {"module":"base","kind":"model","priority":"P1","lines":10},
  {"module":"account","kind":"controller","priority":"P2","lines":20}
]`)
	writeFile(t, oiPath, `{
  "generated_at":"2026-06-19T00:00:00Z",
  "source_base":"/tmp/oi",
  "modules":[
    {"name":"oi_base","summary":{"total":2,"implemented":1,"intentionally_omitted":1,"blocked":0}}
  ],
  "summary":{"total":2,"implemented":1,"intentionally_omitted":1,"blocked":0}
}`)

	if err := run(backlogPath, parityPath, inventoryPath, oiPath, outPath); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"Gorp Build Dashboard", "Completed build slices", "Current Remaining Build Work", "closed bounded build dashboard", "next gap"} {
		if !strings.Contains(html, want) {
			t.Fatalf("dashboard html missing %q", want)
		}
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
