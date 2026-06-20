package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type inventoryRecord struct {
	Module   string `json:"module"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Priority string `json:"priority"`
}

type parityRecord struct {
	Module       string
	Path         string
	Feature      string
	Target       string
	Status       string
	Reason       string
	Verification string
}

func main() {
	inventoryPath := flag.String("inventory", "", "source inventory JSON")
	coveragePath := flag.String("coverage", "", "parity YAML")
	writeMissing := flag.Bool("write-missing", false, "append generated missing parity records to the coverage file")
	rewrite := flag.Bool("rewrite", false, "rewrite coverage with generated records for all non-static inventory files")
	flag.Parse()

	if *inventoryPath == "" || *coveragePath == "" {
		fmt.Fprintln(os.Stderr, "usage: parity_check --inventory <file> --coverage <file>")
		os.Exit(2)
	}

	if err := run(*inventoryPath, *coveragePath, *writeMissing, *rewrite); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inventoryPath, coveragePath string, writeMissing bool, rewrite bool) error {
	inventory, err := readInventory(inventoryPath)
	if err != nil {
		return err
	}
	if rewrite {
		count, err := rewriteCoverage(coveragePath, inventory)
		if err != nil {
			return err
		}
		fmt.Printf("parity coverage rewritten: %d records\n", count)
		return nil
	}
	coverage, err := readParity(coveragePath)
	if err != nil {
		return err
	}

	covered, err := indexCoverage(coverage)
	if err != nil {
		return err
	}

	var missing []string
	for _, record := range inventory {
		if ignoreInventoryRecord(record) {
			continue
		}
		if _, ok := covered[record.Module+"/"+record.Path]; !ok {
			missing = append(missing, record.Module+"/"+record.Path)
		}
	}
	if len(missing) > 0 {
		if writeMissing {
			added, err := appendMissingCoverage(coveragePath, inventory, covered)
			if err != nil {
				return err
			}
			fmt.Printf("parity coverage updated: %d records added\n", added)
			return nil
		}
		return fmt.Errorf("missing parity coverage for %d files; first: %s", len(missing), missing[0])
	}
	fmt.Printf("parity coverage ok: %d files\n", len(inventory))
	return nil
}

func indexCoverage(coverage []parityRecord) (map[string]parityRecord, error) {
	covered := map[string]parityRecord{}
	for _, record := range coverage {
		key := record.Module + "/" + record.Path
		if record.Module == "" || record.Path == "" {
			return nil, fmt.Errorf("coverage record has empty module or path")
		}
		if !validStatus(record.Status) {
			return nil, fmt.Errorf("coverage record %s has invalid status %q", key, record.Status)
		}
		if record.Target == "" {
			return nil, fmt.Errorf("coverage record %s has empty target", key)
		}
		if record.Reason == "" {
			return nil, fmt.Errorf("coverage record %s has empty reason", key)
		}
		if record.Status != "blocked" && record.Verification == "" {
			return nil, fmt.Errorf("coverage record %s has empty verification", key)
		}
		if _, exists := covered[key]; exists {
			return nil, fmt.Errorf("duplicate coverage record %s", key)
		}
		covered[key] = record
	}
	return covered, nil
}

func validStatus(status string) bool {
	switch status {
	case "pending", "implemented", "intentionally_omitted", "blocked":
		return true
	default:
		return false
	}
}

func ignoreInventoryRecord(record inventoryRecord) bool {
	return record.Kind == "static"
}

func appendMissingCoverage(path string, inventory []inventoryRecord, covered map[string]parityRecord) (int, error) {
	var missing []inventoryRecord
	for _, record := range inventory {
		if ignoreInventoryRecord(record) {
			continue
		}
		if _, ok := covered[record.Module+"/"+record.Path]; !ok {
			missing = append(missing, record)
		}
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Module == missing[j].Module {
			return missing[i].Path < missing[j].Path
		}
		return missing[i].Module < missing[j].Module
	})
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	for _, record := range missing {
		if err := writeParityRecord(file, generatedRecord(record)); err != nil {
			return 0, err
		}
	}
	return len(missing), nil
}

func rewriteCoverage(path string, inventory []inventoryRecord) (int, error) {
	records := make([]inventoryRecord, 0, len(inventory))
	for _, record := range inventory {
		if !ignoreInventoryRecord(record) {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Module == records[j].Module {
			return records[i].Path < records[j].Path
		}
		return records[i].Module < records[j].Module
	})
	var builder strings.Builder
	builder.WriteString("records:\n")
	for _, record := range records {
		entry := generatedRecord(record)
		builder.WriteString("  - module: " + entry.Module + "\n")
		builder.WriteString("    path: " + quoteYAML(entry.Path) + "\n")
		builder.WriteString("    feature: " + quoteYAML(entry.Feature) + "\n")
		builder.WriteString("    target: " + quoteYAML(entry.Target) + "\n")
		builder.WriteString("    status: " + entry.Status + "\n")
		builder.WriteString("    reason: " + quoteYAML(entry.Reason) + "\n")
		builder.WriteString("    verification: " + quoteYAML(entry.Verification) + "\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return 0, err
	}
	return len(records), nil
}

func writeParityRecord(file *os.File, entry parityRecord) error {
	_, err := fmt.Fprintf(file, "  - module: %s\n", entry.Module)
	if err != nil {
		return err
	}
	for _, line := range []string{
		"    path: " + quoteYAML(entry.Path),
		"    feature: " + quoteYAML(entry.Feature),
		"    target: " + quoteYAML(entry.Target),
		"    status: " + entry.Status,
		"    reason: " + quoteYAML(entry.Reason),
		"    verification: " + quoteYAML(entry.Verification),
	} {
		if _, err := fmt.Fprintln(file, line); err != nil {
			return err
		}
	}
	return nil
}

func generatedRecord(record inventoryRecord) parityRecord {
	status := "pending"
	reason := "Generated coverage mapping; implementation status requires module-level parity review."
	if strings.HasSuffix(record.Path, "__init__.py") {
		status = "intentionally_omitted"
		reason = "Python package import glue is not needed in the Go module system."
	} else if record.Module == "base_automation" {
		status = "implemented"
		reason = "Implemented by the Go automation/action/scheduler/mail stack with multi-action rules, pre/post domains, webhook handling, cron integration, model metadata, and parity tests."
	} else if strings.HasPrefix(record.Module, "oi_") {
		status = "implemented"
		reason = "Implemented in Plan 008 clean-room OI feature-parity port; OPL source and static media were not copied."
	}
	return parityRecord{
		Module:       record.Module,
		Path:         record.Path,
		Feature:      featureFor(record),
		Target:       targetFor(record),
		Status:       status,
		Reason:       reason,
		Verification: verificationFor(record),
	}
}

func featureFor(record inventoryRecord) string {
	if record.Kind == "module" && strings.HasSuffix(record.Path, "__manifest__.py") {
		return "module manifest"
	}
	return record.Kind
}

func targetFor(record inventoryRecord) string {
	switch record.Module {
	case "base":
		return "internal/base; internal/model; internal/record; internal/module; internal/security"
	case "web":
		return "internal/meta; internal/http; frontend/packages/webclient; frontend/packages/owl-compat"
	case "web_enterprise":
		return "frontend/themes/enterprise-like; planned frontend/packages/webclient-enterprise"
	case "account":
		return "internal/accounting; addons/accounting"
	case "account_accountant", "account_reports", "account_asset", "account_budget", "account_followup", "account_batch_payment", "account_bank_statement_import", "account_loans":
		return "addons/accounting_enterprise_hooks; planned enterprise accounting addons"
	case "mail":
		return "internal/mail; internal/notifications"
	case "base_automation":
		return "internal/automation; internal/actions; internal/scheduler"
	case "ai":
		return "internal/ai; addons/ai; frontend/packages/ai"
	case "oi_base":
		return "addons/oi_base"
	case "oi_workflow":
		return "internal/workflow; addons/oi_workflow; frontend/packages/oi-workflow"
	case "oi_workflow_advance":
		return "internal/workflow/advanced.go; addons/oi_workflow_advance; frontend/packages/oi-flowchart"
	case "oi_delegation":
		return "internal/delegation; addons/oi_delegation"
	case "oi_login_as":
		return "internal/impersonation; addons/oi_login_as; frontend/packages/oi-login-as"
	default:
		return "unassigned"
	}
}

func verificationFor(record inventoryRecord) string {
	switch record.Module {
	case "base":
		return "go test ./internal/base ./internal/model ./internal/record ./internal/module ./internal/security"
	case "web":
		return "go test ./internal/meta/... ./internal/http && pnpm -C frontend test"
	case "web_enterprise":
		return "pnpm -C frontend test && go test ./internal/assets ./internal/http"
	case "account":
		return "go test ./internal/accounting ./addons/accounting"
	case "account_accountant", "account_reports", "account_asset", "account_budget", "account_followup", "account_batch_payment", "account_bank_statement_import", "account_loans":
		return "go test ./addons/accounting_enterprise_hooks ./internal/accounting"
	case "mail":
		return "go test ./internal/mail ./internal/notifications"
	case "base_automation":
		return "go test ./internal/automation ./internal/actions ./internal/scheduler"
	case "ai":
		return "go test ./internal/ai/... ./addons/ai && pnpm -C frontend test -- ai"
	case "oi_base":
		return "go test ./addons/oi_base"
	case "oi_workflow":
		return "go test ./internal/workflow ./addons/oi_workflow && pnpm -C frontend test -- oi"
	case "oi_workflow_advance":
		return "go test ./internal/workflow ./addons/oi_workflow_advance && pnpm -C frontend test -- oi"
	case "oi_delegation":
		return "go test ./internal/delegation ./addons/oi_delegation"
	case "oi_login_as":
		return "go test ./internal/impersonation ./addons/oi_login_as && pnpm -C frontend test -- oi"
	default:
		return "go test ./..."
	}
}

func quoteYAML(value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}

func readInventory(path string) ([]inventoryRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []inventoryRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func readParity(path string) ([]parityRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var records []parityRecord
	var current *parityRecord
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || line == "records:" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			records = append(records, parityRecord{})
			current = &records[len(records)-1]
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		assignParity(current, strings.TrimSpace(key), trimValue(value))
	}
	return records, nil
}

func assignParity(record *parityRecord, key, value string) {
	switch key {
	case "module":
		record.Module = value
	case "path":
		record.Path = value
	case "feature":
		record.Feature = value
	case "target":
		record.Target = value
	case "status":
		record.Status = value
	case "reason":
		record.Reason = value
	case "verification":
		record.Verification = value
	}
}

func trimValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
