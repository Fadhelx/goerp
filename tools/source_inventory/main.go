package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Modules []ModuleConfig
}

type ModuleConfig struct {
	Name     string
	Root     string
	Version  string
	License  string
	Priority string
	Include  []string
	Exclude  []string
}

type Record struct {
	Module          string   `json:"module"`
	Root            string   `json:"root"`
	Path            string   `json:"path"`
	Relative        string   `json:"relative"`
	Ext             string   `json:"ext"`
	Kind            string   `json:"kind"`
	Lines           int      `json:"lines"`
	SHA256          string   `json:"sha256"`
	Version         string   `json:"version"`
	License         string   `json:"license"`
	Priority        string   `json:"priority"`
	ManifestDepends []string `json:"manifest_depends,omitempty"`
}

func main() {
	configPath := flag.String("config", "", "inventory config")
	outPath := flag.String("out", "", "output JSON path")
	flag.Parse()

	if *configPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: source_inventory --config <file> --out <file>")
		os.Exit(2)
	}

	if err := run(*configPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configPath, outPath string) error {
	cfg, err := readConfig(configPath)
	if err != nil {
		return err
	}

	var records []Record
	for _, module := range cfg.Modules {
		moduleRecords, err := inventoryModule(module)
		if err != nil {
			return err
		}
		records = append(records, moduleRecords...)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Module == records[j].Module {
			return records[i].Path < records[j].Path
		}
		return records[i].Module < records[j].Module
	})

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("source inventory ok: %d files\n", len(records))
	return nil
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return parseConfig(string(data))
}

func parseConfig(text string) (Config, error) {
	var cfg Config
	var current *ModuleConfig
	var listKey string

	for _, rawLine := range strings.Split(text, "\n") {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " "))
		line := strings.TrimSpace(rawLine)
		if line == "modules:" {
			continue
		}
		if strings.HasPrefix(line, "- name:") {
			cfg.Modules = append(cfg.Modules, ModuleConfig{})
			current = &cfg.Modules[len(cfg.Modules)-1]
			current.Name = trimValue(strings.TrimPrefix(line, "- name:"))
			listKey = ""
			continue
		}
		if current == nil {
			return Config{}, fmt.Errorf("module field before module declaration: %s", line)
		}
		if strings.HasPrefix(line, "- ") && listKey != "" {
			value := trimValue(strings.TrimPrefix(line, "- "))
			if listKey == "include" {
				current.Include = append(current.Include, value)
			} else {
				current.Exclude = append(current.Exclude, value)
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("invalid config line: %s", line)
		}
		key = strings.TrimSpace(key)
		value = trimValue(value)
		if indent == 4 && (key == "include" || key == "exclude") {
			listKey = key
			continue
		}
		listKey = ""
		switch key {
		case "root":
			current.Root = value
		case "version":
			current.Version = value
		case "license":
			current.License = value
		case "priority":
			current.Priority = value
		case "include":
			listKey = "include"
		case "exclude":
			listKey = "exclude"
		default:
			return Config{}, fmt.Errorf("unknown config key %q", key)
		}
	}

	for _, module := range cfg.Modules {
		if module.Name == "" || module.Root == "" {
			return Config{}, fmt.Errorf("module requires name and root")
		}
	}
	return cfg, nil
}

func inventoryModule(module ModuleConfig) ([]Record, error) {
	if _, err := os.Stat(module.Root); err != nil {
		return nil, fmt.Errorf("%s: %w", module.Root, err)
	}
	depends, err := manifestDepends(module.Root)
	if err != nil {
		return nil, err
	}

	include := map[string]bool{}
	for _, ext := range module.Include {
		include[ext] = true
	}

	var records []Record
	err = filepath.WalkDir(module.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(module.Root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if excluded(rel, module.Exclude) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if !include[ext] {
			return nil
		}
		lines, sum, err := fileStats(path)
		if err != nil {
			return err
		}
		records = append(records, Record{
			Module:          module.Name,
			Root:            module.Root,
			Path:            filepath.ToSlash(rel),
			Relative:        filepath.ToSlash(rel),
			Ext:             ext,
			Kind:            classify(filepath.ToSlash(rel)),
			Lines:           lines,
			SHA256:          sum,
			Version:         module.Version,
			License:         module.License,
			Priority:        module.Priority,
			ManifestDepends: append([]string(nil), depends...),
		})
		return nil
	})
	return records, err
}

func manifestDepends(root string) ([]string, error) {
	for _, name := range []string{"__manifest__.py", "__openerp__.py"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if errorsIsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		depends := parseManifestDepends(string(data))
		sort.Strings(depends)
		return depends, nil
	}
	return nil, nil
}

func parseManifestDepends(text string) []string {
	var depends []string
	idx := strings.Index(text, "'depends'")
	if idx < 0 {
		idx = strings.Index(text, `"depends"`)
	}
	if idx < 0 {
		return nil
	}
	open := strings.Index(text[idx:], "[")
	if open < 0 {
		return nil
	}
	open += idx
	depth := 0
	close := -1
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				close = i
				i = len(text)
			}
		}
	}
	if close < 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, token := range strings.Split(text[open+1:close], ",") {
		value := trimValue(token)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		depends = append(depends, value)
	}
	return depends
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func excluded(rel string, excludes []string) bool {
	rel = filepath.ToSlash(rel)
	for _, item := range excludes {
		if item == "" {
			continue
		}
		item = strings.Trim(item, "/")
		if rel == item || strings.HasPrefix(rel, item+"/") || strings.Contains(rel, "/"+item+"/") {
			return true
		}
	}
	return false
}

func fileStats(path string) (int, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	hash := sha256.New()
	reader := bufio.NewReader(file)
	lines := 0
	for {
		chunk, err := reader.ReadBytes('\n')
		if len(chunk) > 0 {
			lines++
			if _, writeErr := hash.Write(chunk); writeErr != nil {
				return 0, "", writeErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, "", err
		}
	}
	return lines, hex.EncodeToString(hash.Sum(nil)), nil
}

func classify(rel string) string {
	switch {
	case strings.Contains(rel, "/models/") || strings.HasPrefix(rel, "models/"):
		return "model"
	case strings.Contains(rel, "/controllers/") || strings.HasPrefix(rel, "controllers/"):
		return "controller"
	case strings.Contains(rel, "/wizard/") || strings.Contains(rel, "/wizards/") || strings.HasPrefix(rel, "wizard/") || strings.HasPrefix(rel, "wizards/"):
		return "wizard"
	case strings.Contains(rel, "/security/") || strings.HasPrefix(rel, "security/"):
		return "security"
	case strings.Contains(rel, "/views/") || strings.Contains(rel, "/view/") || strings.HasPrefix(rel, "views/") || strings.HasPrefix(rel, "view/"):
		return "view"
	case strings.Contains(rel, "/data/") || strings.HasPrefix(rel, "data/"):
		return "data"
	case strings.Contains(rel, "/tests/") || strings.HasPrefix(rel, "tests/"):
		return "test"
	case strings.Contains(rel, "/static/") || strings.HasPrefix(rel, "static/"):
		return "static"
	default:
		return "module"
	}
}

func trimValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
