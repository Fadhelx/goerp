package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type dashboard struct {
	GeneratedAt      string
	BacklogPath      string
	ParityPath       string
	InventoryPath    string
	OIPath           string
	RefreshCommand   string
	DoneSlices       []doneSlice
	RecentDone       []doneSlice
	RemainingGaps    []gap
	LatestSlice      doneSlice
	Parity           coverageSummary
	Inventory        inventorySummary
	OI               oiSummary
	VerificationRows []statusRow
}

type doneSlice struct {
	Number       int
	Line         int
	Title        string
	Summary      string
	Verification string
	Source       string
}

type gap struct {
	Number int
	Text   string
}

type coverageSummary struct {
	Total       int
	Closed      int
	Implemented int
	Pending     int
	Omitted     int
	Blocked     int
	Rows        []statusRow
	Modules     []moduleCoverage
}

type statusRow struct {
	Status  string
	Label   string
	Count   int
	Percent int
	Class   string
}

type moduleCoverage struct {
	Module         string
	Total          int
	Closed         int
	Implemented    int
	Pending        int
	Omitted        int
	Blocked        int
	ClosedPct      int
	PendingPct     int
	ImplementedPct int
}

type inventorySummary struct {
	TotalFiles  int
	Modules     int
	Lines       int
	P1          int
	P2          int
	P3          int
	Kinds       []statusRow
	ModulesRows []inventoryModule
}

type inventoryModule struct {
	Module string
	Files  int
	Lines  int
	P1     int
	P2     int
	P3     int
}

type oiSummary struct {
	GeneratedAt string
	SourceBase  string
	Total       int
	Implemented int
	Omitted     int
	Blocked     int
	Rows        []statusRow
	Modules     []oiModule
}

type oiModule struct {
	Name        string
	Total       int
	Implemented int
	Omitted     int
	Blocked     int
	ClosedPct   int
}

type parityRecord struct {
	Module  string
	Path    string
	Status  string
	Feature string
}

type sourceInventoryRecord struct {
	Module   string `json:"module"`
	Kind     string `json:"kind"`
	Priority string `json:"priority"`
	Lines    int    `json:"lines"`
}

type oiReport struct {
	GeneratedAt string       `json:"generated_at"`
	SourceBase  string       `json:"source_base"`
	Modules     []oiModuleIn `json:"modules"`
	Summary     oiSummaryIn  `json:"summary"`
}

type oiModuleIn struct {
	Name    string      `json:"name"`
	Summary oiSummaryIn `json:"summary"`
}

type oiSummaryIn struct {
	Total                int `json:"total"`
	Implemented          int `json:"implemented"`
	IntentionallyOmitted int `json:"intentionally_omitted"`
	Blocked              int `json:"blocked"`
}

func main() {
	backlogPath := flag.String("backlog", "reports/agent_audit_backlog.md", "agent audit backlog markdown")
	parityPath := flag.String("parity", "reports/parity.yaml", "parity coverage YAML")
	inventoryPath := flag.String("inventory", "reports/source_inventory.json", "source inventory JSON")
	oiPath := flag.String("oi", "reports/oi_inventory.json", "OI inventory JSON")
	outPath := flag.String("out", "reports/progress_dashboard.html", "dashboard HTML output path")
	flag.Parse()

	if err := run(*backlogPath, *parityPath, *inventoryPath, *oiPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(backlogPath, parityPath, inventoryPath, oiPath, outPath string) error {
	backlog, err := readBacklog(backlogPath)
	if err != nil {
		return err
	}
	parity, err := readParitySummary(parityPath)
	if err != nil {
		return err
	}
	inventory, err := readInventorySummary(inventoryPath)
	if err != nil {
		return err
	}
	oi, err := readOISummary(oiPath)
	if err != nil {
		return err
	}

	data := dashboard{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		BacklogPath:      backlogPath,
		ParityPath:       parityPath,
		InventoryPath:    inventoryPath,
		OIPath:           oiPath,
		RefreshCommand:   "go run ./tools/progress_dashboard --out reports/progress_dashboard.html",
		DoneSlices:       backlog.DoneSlices,
		RecentDone:       recentDone(backlog.DoneSlices, 12),
		RemainingGaps:    backlog.RemainingGaps,
		LatestSlice:      backlog.LatestSlice,
		Parity:           parity,
		Inventory:        inventory,
		OI:               oi,
		VerificationRows: verificationRows(backlog.DoneSlices),
	}

	var builder strings.Builder
	tmpl, err := template.New("dashboard").Funcs(template.FuncMap{
		"num":       formatNumber,
		"pct":       pct,
		"width":     widthCSS,
		"statusCls": statusClass,
		"add":       add,
	}).Parse(dashboardHTML)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(&builder, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("progress dashboard written: %s\n", outPath)
	return nil
}

type backlogSummary struct {
	DoneSlices    []doneSlice
	RemainingGaps []gap
	LatestSlice   doneSlice
}

func readBacklog(path string) (backlogSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return backlogSummary{}, err
	}
	return parseBacklog(string(data)), nil
}

func parseBacklog(text string) backlogSummary {
	var out backlogSummary
	lines := strings.Split(text, "\n")
	for lineIndex, raw := range lines {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "- DONE this slice:") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "- DONE this slice:"))
		description := body
		if idx := strings.Index(body, " Remaining deferred gaps"); idx >= 0 {
			description = strings.TrimSpace(body[:idx])
		}
		number := len(out.DoneSlices) + 1
		slice := doneSlice{
			Number:       number,
			Line:         lineIndex + 1,
			Title:        firstSentence(description),
			Summary:      description,
			Verification: verificationLabel(body),
			Source:       fmt.Sprintf("reports/agent_audit_backlog.md:%d", lineIndex+1),
		}
		out.DoneSlices = append(out.DoneSlices, slice)
		out.LatestSlice = slice
		if gaps := gapsFromLine(body); len(gaps) > 0 {
			out.RemainingGaps = gaps
		}
	}
	reverseDone(out.DoneSlices)
	return out
}

func reverseDone(items []doneSlice) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, ". "); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return strings.TrimSuffix(text, ".")
}

func verificationLabel(text string) string {
	lower := strings.ToLower(strings.ReplaceAll(text, "`", ""))
	switch {
	case strings.Contains(lower, "full ci passed"), strings.Contains(lower, "make ci passed"):
		return "full CI passed"
	case strings.Contains(lower, "affected package tests passed"), strings.Contains(lower, "package tests passed"):
		return "package tests passed"
	case strings.Contains(lower, "focused tests"):
		return "focused tests passed"
	case strings.Contains(lower, "regressions"):
		return "regression coverage added"
	default:
		return "not recorded"
	}
}

func gapsFromLine(text string) []gap {
	idx := strings.Index(text, "Remaining deferred gaps")
	if idx < 0 {
		return nil
	}
	tail := text[idx:]
	colon := strings.Index(tail, ":")
	if colon < 0 {
		return nil
	}
	tail = strings.TrimSpace(tail[colon+1:])
	tail = strings.TrimSuffix(tail, ".")
	tail = strings.ReplaceAll(tail, ", and ", ", ")
	parts := strings.Split(tail, ",")
	var gaps []gap
	for _, part := range parts {
		item := strings.TrimSpace(part)
		item = strings.TrimPrefix(item, "and ")
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		gaps = append(gaps, gap{Number: len(gaps) + 1, Text: item})
	}
	return gaps
}

func recentDone(items []doneSlice, limit int) []doneSlice {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func verificationRows(items []doneSlice) []statusRow {
	counts := map[string]int{}
	for _, item := range items {
		counts[item.Verification]++
	}
	var rows []statusRow
	for label, count := range counts {
		rows = append(rows, statusRow{
			Status:  label,
			Label:   label,
			Count:   count,
			Percent: pct(count, len(items)),
			Class:   statusClass(label),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return rows[i].Label < rows[j].Label
		}
		return rows[i].Count > rows[j].Count
	})
	return rows
}

func readParitySummary(path string) (coverageSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return coverageSummary{}, err
	}
	records := parseParity(string(data))
	return summarizeParity(records), nil
}

func parseParity(text string) []parityRecord {
	var records []parityRecord
	var current *parityRecord
	for _, rawLine := range strings.Split(text, "\n") {
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
	return records
}

func assignParity(record *parityRecord, key, value string) {
	switch key {
	case "module":
		record.Module = value
	case "path":
		record.Path = value
	case "feature":
		record.Feature = value
	case "status":
		record.Status = value
	}
}

func summarizeParity(records []parityRecord) coverageSummary {
	var out coverageSummary
	modules := map[string]*moduleCoverage{}
	for _, record := range records {
		if record.Module == "" {
			continue
		}
		out.Total++
		module := modules[record.Module]
		if module == nil {
			module = &moduleCoverage{Module: record.Module}
			modules[record.Module] = module
		}
		module.Total++
		switch record.Status {
		case "implemented":
			out.Implemented++
			module.Implemented++
		case "intentionally_omitted":
			out.Omitted++
			module.Omitted++
		case "blocked":
			out.Blocked++
			module.Blocked++
		default:
			out.Pending++
			module.Pending++
		}
	}
	out.Closed = out.Implemented + out.Omitted
	out.Rows = fixedStatusRows(out.Total, map[string]int{
		"implemented":           out.Implemented,
		"pending":               out.Pending,
		"intentionally_omitted": out.Omitted,
		"blocked":               out.Blocked,
	})
	for _, module := range modules {
		module.Closed = module.Implemented + module.Omitted
		module.ClosedPct = pct(module.Closed, module.Total)
		module.PendingPct = pct(module.Pending, module.Total)
		module.ImplementedPct = pct(module.Implemented, module.Total)
		out.Modules = append(out.Modules, *module)
	}
	sort.Slice(out.Modules, func(i, j int) bool {
		if out.Modules[i].Pending == out.Modules[j].Pending {
			return out.Modules[i].Module < out.Modules[j].Module
		}
		return out.Modules[i].Pending > out.Modules[j].Pending
	})
	return out
}

func fixedStatusRows(total int, counts map[string]int) []statusRow {
	order := []string{"implemented", "pending", "intentionally_omitted", "blocked"}
	var rows []statusRow
	for _, status := range order {
		rows = append(rows, statusRow{
			Status:  status,
			Label:   statusLabel(status),
			Count:   counts[status],
			Percent: pct(counts[status], total),
			Class:   statusClass(status),
		})
	}
	return rows
}

func readInventorySummary(path string) (inventorySummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inventorySummary{}, err
	}
	var records []sourceInventoryRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return inventorySummary{}, err
	}
	return summarizeInventory(records), nil
}

func summarizeInventory(records []sourceInventoryRecord) inventorySummary {
	var out inventorySummary
	kinds := map[string]int{}
	modules := map[string]*inventoryModule{}
	for _, record := range records {
		out.TotalFiles++
		out.Lines += record.Lines
		kinds[record.Kind]++
		module := modules[record.Module]
		if module == nil {
			module = &inventoryModule{Module: record.Module}
			modules[record.Module] = module
		}
		module.Files++
		module.Lines += record.Lines
		switch strings.ToUpper(record.Priority) {
		case "P1":
			out.P1++
			module.P1++
		case "P2":
			out.P2++
			module.P2++
		case "P3":
			out.P3++
			module.P3++
		}
	}
	out.Modules = len(modules)
	for kind, count := range kinds {
		out.Kinds = append(out.Kinds, statusRow{
			Status:  kind,
			Label:   kind,
			Count:   count,
			Percent: pct(count, out.TotalFiles),
			Class:   statusClass(kind),
		})
	}
	sort.Slice(out.Kinds, func(i, j int) bool {
		if out.Kinds[i].Count == out.Kinds[j].Count {
			return out.Kinds[i].Label < out.Kinds[j].Label
		}
		return out.Kinds[i].Count > out.Kinds[j].Count
	})
	for _, module := range modules {
		out.ModulesRows = append(out.ModulesRows, *module)
	}
	sort.Slice(out.ModulesRows, func(i, j int) bool {
		if out.ModulesRows[i].Files == out.ModulesRows[j].Files {
			return out.ModulesRows[i].Module < out.ModulesRows[j].Module
		}
		return out.ModulesRows[i].Files > out.ModulesRows[j].Files
	})
	return out
}

func readOISummary(path string) (oiSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return oiSummary{}, err
	}
	var report oiReport
	if err := json.Unmarshal(data, &report); err != nil {
		return oiSummary{}, err
	}
	out := oiSummary{
		GeneratedAt: report.GeneratedAt,
		SourceBase:  report.SourceBase,
		Total:       report.Summary.Total,
		Implemented: report.Summary.Implemented,
		Omitted:     report.Summary.IntentionallyOmitted,
		Blocked:     report.Summary.Blocked,
	}
	if out.Total == 0 {
		for _, module := range report.Modules {
			out.Total += module.Summary.Total
			out.Implemented += module.Summary.Implemented
			out.Omitted += module.Summary.IntentionallyOmitted
			out.Blocked += module.Summary.Blocked
		}
	}
	out.Rows = fixedStatusRows(out.Total, map[string]int{
		"implemented":           out.Implemented,
		"pending":               0,
		"intentionally_omitted": out.Omitted,
		"blocked":               out.Blocked,
	})
	for _, module := range report.Modules {
		out.Modules = append(out.Modules, oiModule{
			Name:        module.Name,
			Total:       module.Summary.Total,
			Implemented: module.Summary.Implemented,
			Omitted:     module.Summary.IntentionallyOmitted,
			Blocked:     module.Summary.Blocked,
			ClosedPct:   pct(module.Summary.Implemented+module.Summary.IntentionallyOmitted, module.Summary.Total),
		})
	}
	sort.Slice(out.Modules, func(i, j int) bool { return out.Modules[i].Name < out.Modules[j].Name })
	return out, nil
}

func statusLabel(status string) string {
	switch status {
	case "implemented":
		return "Implemented"
	case "pending":
		return "Pending"
	case "intentionally_omitted":
		return "Omitted"
	case "blocked":
		return "Blocked"
	default:
		return strings.ReplaceAll(status, "_", " ")
	}
}

func statusClass(status string) string {
	status = strings.ToLower(status)
	switch {
	case strings.Contains(status, "implemented"), strings.Contains(status, "full ci"), strings.Contains(status, "passed"):
		return "good"
	case strings.Contains(status, "pending"), strings.Contains(status, "todo"):
		return "warn"
	case strings.Contains(status, "blocked"):
		return "bad"
	case strings.Contains(status, "omitted"):
		return "muted"
	default:
		return "plain"
	}
}

func pct(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part)/float64(total)*100 + 0.5)
}

func add(left, right int) int {
	return left + right
}

func widthCSS(part, total int) template.CSS {
	if total <= 0 || part <= 0 {
		return "width: 0%"
	}
	return template.CSS(fmt.Sprintf("width: %.2f%%", float64(part)/float64(total)*100))
}

func formatNumber(value int) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	s := fmt.Sprintf("%d", value)
	var out []byte
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(r))
	}
	return string(out)
}

func trimValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Gorp Build Dashboard</title>
  <style>
    :root {
      --bg: #f7f8f7;
      --panel: #ffffff;
      --ink: #161916;
      --muted: #647067;
      --line: #d9dfda;
      --strong-line: #aeb8b0;
      --good: #007a5a;
      --warn: #a85f00;
      --bad: #b42318;
      --soft-good: #e5f4ee;
      --soft-warn: #fff2d9;
      --soft-bad: #fde8e6;
      --soft-muted: #eef1ee;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
      font: 14px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    header {
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    .wrap {
      width: min(1440px, calc(100vw - 32px));
      margin: 0 auto;
    }
    .top {
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 24px;
      padding: 24px 0 18px;
      align-items: end;
    }
    h1, h2, h3, p { margin: 0; }
    h1 {
      font-size: 32px;
      line-height: 1.05;
      letter-spacing: 0;
    }
    h2 {
      font-size: 18px;
      line-height: 1.2;
      margin-bottom: 12px;
    }
    h3 {
      font-size: 14px;
      line-height: 1.2;
      margin-bottom: 8px;
    }
    .sub, .meta, .small {
      color: var(--muted);
    }
    .meta {
      display: grid;
      gap: 4px;
      text-align: right;
      font-size: 12px;
    }
    code {
      padding: 2px 5px;
      border: 1px solid var(--line);
      background: #fbfbfa;
      border-radius: 4px;
      font-size: 12px;
    }
    main {
      padding: 20px 0 36px;
    }
    section {
      margin-top: 20px;
      padding-top: 4px;
    }
    .metrics {
      display: grid;
      grid-template-columns: repeat(5, minmax(150px, 1fr));
      gap: 10px;
    }
    .metric, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
    }
    .metric {
      min-height: 108px;
      padding: 14px;
      display: grid;
      align-content: space-between;
      gap: 8px;
    }
    .metric .label {
      color: var(--muted);
      text-transform: uppercase;
      font-size: 11px;
      letter-spacing: .08em;
    }
    .metric .value {
      font-size: 30px;
      line-height: 1;
      font-weight: 700;
    }
    .metric .note {
      color: var(--muted);
      font-size: 12px;
      min-height: 18px;
    }
    .grid-2 {
      display: grid;
      grid-template-columns: minmax(0, 1.05fr) minmax(360px, .95fr);
      gap: 16px;
      align-items: start;
    }
    .panel {
      padding: 16px;
      min-width: 0;
    }
    .bar {
      height: 16px;
      display: flex;
      overflow: hidden;
      border-radius: 4px;
      border: 1px solid var(--line);
      background: var(--soft-muted);
    }
    .seg.good { background: var(--good); }
    .seg.warn { background: var(--warn); }
    .seg.bad { background: var(--bad); }
    .seg.muted { background: var(--strong-line); }
    .status-grid {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 8px;
      margin-top: 12px;
    }
    .status {
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 10px;
      min-width: 0;
    }
    .status .count {
      font-weight: 700;
      font-size: 18px;
    }
    .status.good { background: var(--soft-good); }
    .status.warn { background: var(--soft-warn); }
    .status.bad { background: var(--soft-bad); }
    .status.muted { background: var(--soft-muted); }
    .gap-list {
      display: grid;
      gap: 8px;
      margin: 0;
      padding: 0;
      list-style: none;
    }
    .gap-list li {
      display: grid;
      grid-template-columns: 36px 1fr;
      gap: 10px;
      align-items: start;
      padding: 9px 0;
      border-bottom: 1px solid var(--line);
    }
    .gap-list li:last-child { border-bottom: 0; }
    .num {
      width: 28px;
      height: 24px;
      display: inline-grid;
      place-items: center;
      border: 1px solid var(--line);
      border-radius: 4px;
      color: var(--muted);
      font-size: 12px;
    }
    .toolbar {
      display: grid;
      grid-template-columns: minmax(240px, 420px) auto;
      gap: 12px;
      align-items: center;
      margin-bottom: 10px;
    }
    label {
      display: grid;
      gap: 5px;
      color: var(--muted);
      font-size: 12px;
    }
    input {
      width: 100%;
      min-height: 36px;
      border: 1px solid var(--strong-line);
      border-radius: 6px;
      padding: 8px 10px;
      font: inherit;
      color: var(--ink);
      background: var(--panel);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      table-layout: fixed;
      background: var(--panel);
      border: 1px solid var(--line);
    }
    .table-scroll {
      max-width: 100%;
    }
    th, td {
      padding: 10px;
      border-bottom: 1px solid var(--line);
      text-align: left;
      vertical-align: top;
    }
    th {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: .06em;
      color: var(--muted);
      background: #fbfbfa;
    }
    tbody tr:last-child td { border-bottom: 0; }
    .slice-no { width: 74px; }
    .slice-verification { width: 150px; }
    .slice-line { width: 92px; }
    .module-name { width: 210px; }
    .numeric { text-align: right; font-variant-numeric: tabular-nums; }
    .badge {
      display: inline-block;
      max-width: 100%;
      padding: 3px 7px;
      border-radius: 4px;
      border: 1px solid var(--line);
      font-size: 12px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .badge.good { color: var(--good); background: var(--soft-good); }
    .badge.warn { color: var(--warn); background: var(--soft-warn); }
    .badge.bad { color: var(--bad); background: var(--soft-bad); }
    .badge.muted { color: var(--muted); background: var(--soft-muted); }
    .path-list {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
      margin-top: 10px;
    }
    .path-list code {
      display: block;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .empty {
      color: var(--muted);
      border: 1px dashed var(--line);
      border-radius: 8px;
      padding: 14px;
      background: var(--panel);
    }
    [hidden] { display: none !important; }
    @media (max-width: 1080px) {
      .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .grid-2 { grid-template-columns: 1fr; }
      .status-grid { grid-template-columns: repeat(2, 1fr); }
      .top { grid-template-columns: 1fr; }
      .meta { text-align: left; }
    }
    @media (max-width: 720px) {
      .wrap { width: min(100vw - 20px, 1440px); }
      .metrics, .status-grid, .path-list, .toolbar { grid-template-columns: 1fr; }
      table { table-layout: auto; }
      th, td { min-width: 110px; }
      .table-scroll { overflow-x: auto; }
    }
  </style>
</head>
<body>
  <header>
    <div class="wrap top">
      <div>
        <h1>Gorp Build Dashboard</h1>
        <p class="sub">Tracks build progress from audit backlog, parity coverage, source inventory, and OI inventory.</p>
      </div>
      <div class="meta">
        <span>Generated: {{.GeneratedAt}}</span>
        <span>Refresh: <code>{{.RefreshCommand}}</code></span>
      </div>
    </div>
  </header>

  <main class="wrap">
    <section class="metrics" aria-label="Build metrics">
      <div class="metric">
        <div class="label">Completed build slices</div>
        <div class="value">{{num (len .DoneSlices)}}</div>
        <div class="note">Latest line {{.LatestSlice.Line}}</div>
      </div>
      <div class="metric">
        <div class="label">Remaining gaps</div>
        <div class="value">{{num (len .RemainingGaps)}}</div>
        <div class="note">From latest backlog entry</div>
      </div>
      <div class="metric">
        <div class="label">Parity closed</div>
        <div class="value">{{pct .Parity.Closed .Parity.Total}}%</div>
        <div class="note">{{num .Parity.Closed}} / {{num .Parity.Total}} records</div>
      </div>
      <div class="metric">
        <div class="label">Source inventory</div>
        <div class="value">{{num .Inventory.TotalFiles}}</div>
        <div class="note">{{num .Inventory.Modules}} modules, {{num .Inventory.Lines}} lines</div>
      </div>
      <div class="metric">
        <div class="label">OI coverage</div>
        <div class="value">{{pct (add .OI.Implemented .OI.Omitted) .OI.Total}}%</div>
        <div class="note">{{num .OI.Implemented}} implemented, {{num .OI.Blocked}} blocked</div>
      </div>
    </section>

    <section class="grid-2">
      <div class="panel">
        <h2>Current Remaining Build Work</h2>
        {{if .RemainingGaps}}
        <ol class="gap-list">
          {{range .RemainingGaps}}
          <li><span class="num">{{.Number}}</span><span>{{.Text}}</span></li>
          {{end}}
        </ol>
        {{else}}
        <div class="empty">No remaining gaps found in the latest backlog entry.</div>
        {{end}}
      </div>
      <div class="panel">
        <h2>Parity Coverage</h2>
        <div class="bar" aria-label="Parity coverage status">
          <span class="seg good" style="{{width .Parity.Implemented .Parity.Total}}"></span>
          <span class="seg muted" style="{{width .Parity.Omitted .Parity.Total}}"></span>
          <span class="seg warn" style="{{width .Parity.Pending .Parity.Total}}"></span>
          <span class="seg bad" style="{{width .Parity.Blocked .Parity.Total}}"></span>
        </div>
        <div class="status-grid">
          {{range .Parity.Rows}}
          <div class="status {{.Class}}">
            <div class="count">{{num .Count}}</div>
            <div>{{.Label}}</div>
            <div class="small">{{.Percent}}%</div>
          </div>
          {{end}}
        </div>
      </div>
    </section>

    <section class="panel">
      <h2>Latest Completed Build Slices</h2>
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th class="slice-no">Slice</th>
              <th>Done</th>
              <th class="slice-verification">Verification</th>
              <th class="slice-line">Source</th>
            </tr>
          </thead>
          <tbody>
            {{range .RecentDone}}
            <tr>
              <td class="numeric">#{{.Number}}</td>
              <td>{{.Title}}</td>
              <td><span class="badge {{statusCls .Verification}}">{{.Verification}}</span></td>
              <td><code>line {{.Line}}</code></td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </section>

    <section class="panel">
      <div class="toolbar">
        <label>Filter completed slices and modules
          <input id="filter" type="search" placeholder="Search title, module, status, line">
        </label>
        <p class="small" id="filter-count">{{num (len .DoneSlices)}} slices, {{num (len .Parity.Modules)}} modules</p>
      </div>
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th class="slice-no">Slice</th>
              <th>Completed build work</th>
              <th class="slice-verification">Verification</th>
              <th class="slice-line">Source</th>
            </tr>
          </thead>
          <tbody>
            {{range .DoneSlices}}
            <tr class="slice-row" data-search="{{.Number}} {{.Title}} {{.Summary}} {{.Verification}} line {{.Line}}">
              <td class="numeric">#{{.Number}}</td>
              <td>{{.Summary}}</td>
              <td><span class="badge {{statusCls .Verification}}">{{.Verification}}</span></td>
              <td><code>line {{.Line}}</code></td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </section>

    <section class="panel">
      <h2>Module Parity Workload</h2>
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th class="module-name">Module</th>
              <th class="numeric">Total</th>
              <th class="numeric">Implemented</th>
              <th class="numeric">Omitted</th>
              <th class="numeric">Pending</th>
              <th class="numeric">Blocked</th>
              <th class="numeric">Closed</th>
            </tr>
          </thead>
          <tbody>
            {{range .Parity.Modules}}
            <tr class="module-row" data-search="{{.Module}} implemented {{.Implemented}} omitted {{.Omitted}} pending {{.Pending}} blocked {{.Blocked}}">
              <td>{{.Module}}</td>
              <td class="numeric">{{num .Total}}</td>
              <td class="numeric">{{num .Implemented}}</td>
              <td class="numeric">{{num .Omitted}}</td>
              <td class="numeric">{{num .Pending}}</td>
              <td class="numeric">{{num .Blocked}}</td>
              <td class="numeric">{{.ClosedPct}}%</td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </section>

    <section class="grid-2">
      <div class="panel">
        <h2>Source Inventory</h2>
        <div class="status-grid">
          <div class="status plain"><div class="count">{{num .Inventory.P1}}</div><div>P1 files</div></div>
          <div class="status plain"><div class="count">{{num .Inventory.P2}}</div><div>P2 files</div></div>
          <div class="status plain"><div class="count">{{num .Inventory.P3}}</div><div>P3 files</div></div>
          <div class="status plain"><div class="count">{{num .Inventory.Modules}}</div><div>Modules</div></div>
        </div>
        <h3 style="margin-top:14px">Largest source modules</h3>
        <div class="table-scroll">
          <table>
            <thead><tr><th>Module</th><th class="numeric">Files</th><th class="numeric">Lines</th><th class="numeric">P1</th></tr></thead>
            <tbody>
              {{range .Inventory.ModulesRows}}
              <tr><td>{{.Module}}</td><td class="numeric">{{num .Files}}</td><td class="numeric">{{num .Lines}}</td><td class="numeric">{{num .P1}}</td></tr>
              {{end}}
            </tbody>
          </table>
        </div>
      </div>
      <div class="panel">
        <h2>OI Build Coverage</h2>
        <div class="bar" aria-label="OI build coverage">
          <span class="seg good" style="{{width .OI.Implemented .OI.Total}}"></span>
          <span class="seg muted" style="{{width .OI.Omitted .OI.Total}}"></span>
          <span class="seg bad" style="{{width .OI.Blocked .OI.Total}}"></span>
        </div>
        <div class="status-grid">
          {{range .OI.Rows}}
          <div class="status {{.Class}}">
            <div class="count">{{num .Count}}</div>
            <div>{{.Label}}</div>
            <div class="small">{{.Percent}}%</div>
          </div>
          {{end}}
        </div>
        <h3 style="margin-top:14px">OI modules</h3>
        <div class="table-scroll">
          <table>
            <thead><tr><th>Module</th><th class="numeric">Implemented</th><th class="numeric">Omitted</th><th class="numeric">Blocked</th><th class="numeric">Closed</th></tr></thead>
            <tbody>
              {{range .OI.Modules}}
              <tr><td>{{.Name}}</td><td class="numeric">{{num .Implemented}}</td><td class="numeric">{{num .Omitted}}</td><td class="numeric">{{num .Blocked}}</td><td class="numeric">{{.ClosedPct}}%</td></tr>
              {{end}}
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section class="panel">
      <h2>Dashboard Sources</h2>
      <div class="path-list">
        <code>{{.BacklogPath}}</code>
        <code>{{.ParityPath}}</code>
        <code>{{.InventoryPath}}</code>
        <code>{{.OIPath}}</code>
      </div>
    </section>
  </main>

  <script>
    const filterInput = document.getElementById('filter');
    const filterCount = document.getElementById('filter-count');
    const sliceRows = Array.from(document.querySelectorAll('.slice-row'));
    const moduleRows = Array.from(document.querySelectorAll('.module-row'));
    function applyFilter() {
      const q = filterInput.value.trim().toLowerCase();
      let shownSlices = 0;
      let shownModules = 0;
      for (const row of sliceRows) {
        const show = !q || row.dataset.search.toLowerCase().includes(q);
        row.hidden = !show;
        if (show) shownSlices++;
      }
      for (const row of moduleRows) {
        const show = !q || row.dataset.search.toLowerCase().includes(q);
        row.hidden = !show;
        if (show) shownModules++;
      }
      filterCount.textContent = shownSlices.toLocaleString() + ' slices, ' + shownModules.toLocaleString() + ' modules';
    }
    filterInput.addEventListener('input', applyFilter);
  </script>
</body>
</html>
`
