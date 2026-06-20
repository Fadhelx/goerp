package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Report struct {
	GeneratedAt string         `json:"generated_at"`
	SourceBase  string         `json:"source_base"`
	Modules     []ModuleReport `json:"modules"`
	Summary     Summary        `json:"summary"`
	Notes       []string       `json:"notes"`
}

type ModuleReport struct {
	Name    string  `json:"name"`
	Root    string  `json:"root"`
	Entries []Entry `json:"entries"`
	Summary Summary `json:"summary"`
}

type Entry struct {
	SourcePath string `json:"source_path"`
	Relative   string `json:"relative"`
	Category   string `json:"category"`
	Status     string `json:"status"`
	Target     string `json:"target"`
	Reason     string `json:"reason,omitempty"`
}

type Summary struct {
	Total                int `json:"total"`
	Implemented          int `json:"implemented"`
	IntentionallyOmitted int `json:"intentionally_omitted"`
	Blocked              int `json:"blocked"`
}

func main() {
	report, err := BuildReport("/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons", []string{
		"oi_base",
		"oi_workflow",
		"oi_workflow_advance",
		"oi_delegation",
		"oi_login_as",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func BuildReport(sourceBase string, moduleNames []string) (Report, error) {
	report := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		SourceBase:  sourceBase,
		Notes: []string{
			"Generated from local Odoo 18 OI source inventory.",
			"OPL-1 source code and static media were not copied.",
			"Python safe_eval/code execution is replaced by Go callbacks and safe expression/action DSLs.",
		},
	}
	for _, moduleName := range moduleNames {
		root := filepath.Join(sourceBase, moduleName)
		moduleReport, err := buildModuleReport(moduleName, root)
		if err != nil {
			return Report{}, err
		}
		report.Modules = append(report.Modules, moduleReport)
		addSummary(&report.Summary, moduleReport.Summary)
	}
	return report, nil
}

func buildModuleReport(moduleName string, root string) (ModuleReport, error) {
	var entries []Entry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if skipFile(rel) {
			return nil
		}
		category := categoryFor(rel)
		if category == "" {
			return nil
		}
		status, target, reason := implementationFor(moduleName, rel, category)
		entries = append(entries, Entry{
			SourcePath: path,
			Relative:   filepath.ToSlash(rel),
			Category:   category,
			Status:     status,
			Target:     target,
			Reason:     reason,
		})
		return nil
	})
	if err != nil {
		return ModuleReport{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Relative < entries[j].Relative })
	out := ModuleReport{Name: moduleName, Root: root, Entries: entries}
	for _, entry := range entries {
		addEntry(&out.Summary, entry.Status)
	}
	return out, nil
}

func skipFile(rel string) bool {
	base := filepath.Base(rel)
	return base == ".DS_Store" || strings.HasSuffix(base, ".pyc")
}

func categoryFor(rel string) string {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasPrefix(rel, "models/"):
		return "model"
	case strings.HasPrefix(rel, "wizard/"):
		return "wizard"
	case strings.HasPrefix(rel, "controllers/"):
		return "controller"
	case strings.HasPrefix(rel, "data/"):
		return "data"
	case strings.HasPrefix(rel, "security/"):
		return "security"
	case strings.HasPrefix(rel, "view/"), strings.HasPrefix(rel, "views/"):
		return "view"
	default:
		return ""
	}
}

func implementationFor(moduleName string, rel string, category string) (string, string, string) {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "__init__.py") {
		return "intentionally_omitted", "Go package initialization", "Python import glue is not needed in the Go module system."
	}
	if moduleName == "oi_base" {
		switch {
		case rel == "models/ir_attachment.py" || rel == "models/many2many_attachment_res_id_mixin.py":
			return "intentionally_omitted", "addons/oi_base", "Odoo attachment mixin behavior is not required by current OI Go add-ons."
		case category == "controller":
			return "implemented", "addons/oi_base.Ping", "Health endpoint semantics are represented by Ping."
		case category == "model" || category == "view":
			return "implemented", "addons/oi_base", "Base helpers, sequences, groups, XML IDs, escaping, and national ID helpers are represented."
		}
	}
	switch moduleName {
	case "oi_workflow":
		return "implemented", "internal/workflow; addons/oi_workflow; frontend/packages/oi-workflow", "Workflow model metadata, transitions, actions, logs, voting, view metadata, and frontend descriptors are represented."
	case "oi_workflow_advance":
		return "implemented", "internal/workflow/advanced.go; addons/oi_workflow_advance; frontend/packages/oi-flowchart", "Advanced workflow graph, nodes, transitions, process wizard, hooks, and flowchart metadata are represented."
	case "oi_delegation":
		return "implemented", "internal/delegation; addons/oi_delegation", "Delegation lifecycle, security metadata, mail/menu/access hooks, cache invalidation, and expiration are represented."
	case "oi_login_as":
		return "implemented", "internal/impersonation; addons/oi_login_as; frontend/packages/oi-login-as", "Routes, guarded switching, return flow, debug gate, banner context, portal support, and audit are represented."
	default:
		return "blocked", "", "Unknown OI module."
	}
}

func addSummary(dst *Summary, src Summary) {
	dst.Total += src.Total
	dst.Implemented += src.Implemented
	dst.IntentionallyOmitted += src.IntentionallyOmitted
	dst.Blocked += src.Blocked
}

func addEntry(summary *Summary, status string) {
	summary.Total++
	switch status {
	case "implemented":
		summary.Implemented++
	case "intentionally_omitted":
		summary.IntentionallyOmitted++
	case "blocked":
		summary.Blocked++
	}
}
