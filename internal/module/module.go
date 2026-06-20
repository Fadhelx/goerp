package module

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Manifest struct {
	Name               string                      `json:"name"`
	TechnicalName      string                      `json:"technical_name"`
	Version            string                      `json:"version"`
	Category           string                      `json:"category"`
	Depends            []string                    `json:"depends"`
	Data               []string                    `json:"data"`
	Demo               []string                    `json:"demo"`
	Assets             map[string][]string         `json:"assets"`
	AssetOperations    map[string][]AssetOperation `json:"asset_operations,omitempty"`
	Installable        bool                        `json:"installable"`
	AutoInstall        bool                        `json:"auto_install"`
	AutoInstallDepends []string                    `json:"auto_install_depends,omitempty"`
	Application        bool                        `json:"application"`
	SourceVersion      string                      `json:"source_version"`
	SourceLicense      string                      `json:"source_license"`
}

type AssetOperation struct {
	Directive string `json:"directive"`
	Path      string `json:"path"`
	Target    string `json:"target,omitempty"`
}

func ParseManifest(data []byte) (Manifest, error) {
	return ParseManifestForModule(data, "")
}

func ParseManifestForModule(data []byte, moduleName string) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err == nil {
		finalizeManifest(&manifest, moduleName)
		return manifest, manifest.Validate()
	}
	text := string(data)
	if parsed, ok, err := parsePythonManifest(text); ok || err != nil {
		if err != nil {
			return Manifest{}, err
		}
		manifest = parsed
	} else {
		var err error
		manifest, err = parseYAMLManifest(text)
		if err != nil {
			return Manifest{}, err
		}
	}
	finalizeManifest(&manifest, moduleName)
	return manifest, manifest.Validate()
}

func finalizeManifest(manifest *Manifest, moduleName string) {
	if manifest.TechnicalName == "" && moduleName != "" {
		manifest.TechnicalName = moduleName
	}
	if manifest.TechnicalName == "" {
		manifest.TechnicalName = technicalName(manifest.Name)
	}
	if manifest.Assets == nil {
		manifest.Assets = map[string][]string{}
	}
	if manifest.AssetOperations == nil {
		manifest.AssetOperations = map[string][]AssetOperation{}
		for bundle, paths := range manifest.Assets {
			for _, path := range paths {
				manifest.AssetOperations[bundle] = append(manifest.AssetOperations[bundle], AssetOperation{Directive: "append", Path: path})
			}
		}
	}
}

func (m Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest requires name")
	}
	if m.TechnicalName == "" {
		return fmt.Errorf("manifest requires technical_name")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest requires version")
	}
	return nil
}

func SortByDependencies(manifests []Manifest) ([]Manifest, error) {
	byName := map[string]Manifest{}
	for _, manifest := range manifests {
		if _, exists := byName[manifest.TechnicalName]; exists {
			return nil, fmt.Errorf("duplicate module %s", manifest.TechnicalName)
		}
		byName[manifest.TechnicalName] = manifest
	}

	var ordered []Manifest
	temporary := map[string]bool{}
	permanent := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if permanent[name] {
			return nil
		}
		if temporary[name] {
			return fmt.Errorf("dependency cycle at %s", name)
		}
		manifest, exists := byName[name]
		if !exists {
			return fmt.Errorf("unknown dependency %s", name)
		}
		temporary[name] = true
		deps := append([]string(nil), manifest.Depends...)
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		temporary[name] = false
		permanent[name] = true
		ordered = append(ordered, manifest)
		return nil
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func parseYAMLManifest(text string) (Manifest, error) {
	var manifest Manifest
	var listKey string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") && listKey != "" {
			value := trimValue(strings.TrimPrefix(line, "- "))
			switch listKey {
			case "depends":
				manifest.Depends = append(manifest.Depends, value)
			case "data":
				manifest.Data = append(manifest.Data, value)
			case "demo":
				manifest.Demo = append(manifest.Demo, value)
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Manifest{}, fmt.Errorf("invalid manifest line %q", line)
		}
		key = strings.TrimSpace(key)
		value = trimValue(value)
		switch key {
		case "name":
			manifest.Name = value
		case "technical_name":
			manifest.TechnicalName = value
		case "version":
			manifest.Version = value
		case "category":
			manifest.Category = value
		case "installable":
			manifest.Installable = value == "true"
		case "auto_install":
			manifest.AutoInstall = value == "true"
		case "application":
			manifest.Application = value == "true"
		case "depends", "data", "demo":
			listKey = key
			continue
		default:
			return Manifest{}, fmt.Errorf("unsupported manifest key %q", key)
		}
		listKey = ""
	}
	return manifest, nil
}

func parsePythonManifest(text string) (Manifest, bool, error) {
	text = stripPythonComments(text)
	start := strings.Index(text, "{")
	if start < 0 {
		return Manifest{}, false, nil
	}
	end := matchingPythonDelimiter(text, start, '{', '}')
	if end < 0 {
		return Manifest{}, true, fmt.Errorf("unterminated python manifest dict")
	}
	manifest := Manifest{Installable: true}
	for _, raw := range splitPythonTopLevel(text[start+1:end], ',') {
		keyExpr, valueExpr, ok := splitPythonTopLevelPair(raw, ':')
		if !ok {
			continue
		}
		key, ok := parsePythonString(strings.TrimSpace(keyExpr))
		if !ok {
			continue
		}
		value := strings.TrimSpace(valueExpr)
		switch key {
		case "name":
			manifest.Name, _ = parsePythonString(value)
		case "version":
			manifest.Version, _ = parsePythonString(value)
			manifest.SourceVersion = manifest.Version
		case "category":
			manifest.Category, _ = parsePythonString(value)
		case "depends":
			manifest.Depends = parsePythonStringList(value)
		case "data":
			manifest.Data = parsePythonStringList(value)
		case "demo":
			manifest.Demo = parsePythonStringList(value)
		case "assets":
			manifest.Assets, manifest.AssetOperations = parsePythonAssetOperations(value)
		case "installable":
			manifest.Installable = parsePythonBool(value)
		case "auto_install":
			manifest.AutoInstall, manifest.AutoInstallDepends = parsePythonAutoInstall(value)
		case "application":
			manifest.Application = parsePythonBool(value)
		case "license":
			manifest.SourceLicense, _ = parsePythonString(value)
		}
	}
	return manifest, true, nil
}

func parsePythonAssets(raw string) map[string][]string {
	paths, _ := parsePythonAssetOperations(raw)
	return paths
}

func parsePythonAssetOperations(raw string) (map[string][]string, map[string][]AssetOperation) {
	paths := map[string][]string{}
	operations := map[string][]AssetOperation{}
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return paths, operations
	}
	end := matchingPythonDelimiter(raw, 0, '{', '}')
	if end < 0 {
		return paths, operations
	}
	for _, item := range splitPythonTopLevel(raw[1:end], ',') {
		keyExpr, valueExpr, ok := splitPythonTopLevelPair(item, ':')
		if !ok {
			continue
		}
		key, ok := parsePythonString(strings.TrimSpace(keyExpr))
		if !ok {
			continue
		}
		bundlePaths, bundleOps := parsePythonAssetList(valueExpr)
		paths[key] = bundlePaths
		operations[key] = bundleOps
	}
	return paths, operations
}

func parsePythonAssetList(raw string) ([]string, []AssetOperation) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") {
		return nil, nil
	}
	end := matchingPythonDelimiter(raw, 0, '[', ']')
	if end < 0 {
		return nil, nil
	}
	var paths []string
	var operations []AssetOperation
	for _, item := range splitPythonTopLevel(raw[1:end], ',') {
		item = strings.TrimSpace(item)
		if value, ok := parsePythonString(item); ok {
			paths = append(paths, value)
			operations = append(operations, AssetOperation{Directive: "append", Path: value})
			continue
		}
		op, ok := parsePythonAssetTuple(item)
		if !ok {
			continue
		}
		operations = append(operations, op)
		if op.Directive == "append" && op.Path != "" {
			paths = append(paths, op.Path)
		}
	}
	return paths, operations
}

func parsePythonAssetTuple(raw string) (AssetOperation, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "(") {
		return AssetOperation{}, false
	}
	end := matchingPythonDelimiter(raw, 0, '(', ')')
	if end < 0 {
		return AssetOperation{}, false
	}
	var values []string
	for _, item := range splitPythonTopLevel(raw[1:end], ',') {
		value, ok := parsePythonString(strings.TrimSpace(item))
		if !ok {
			return AssetOperation{}, false
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return AssetOperation{}, false
	}
	switch values[0] {
	case "include":
		if len(values) != 2 {
			return AssetOperation{}, false
		}
		return AssetOperation{Directive: "include", Path: values[1]}, true
	case "remove":
		if len(values) != 2 {
			return AssetOperation{}, false
		}
		return AssetOperation{Directive: "remove", Path: values[1]}, true
	case "before", "after", "replace":
		if len(values) != 3 {
			return AssetOperation{}, false
		}
		return AssetOperation{Directive: values[0], Target: values[1], Path: values[2]}, true
	default:
		return AssetOperation{}, false
	}
}

func parsePythonStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") {
		return nil
	}
	end := matchingPythonDelimiter(raw, 0, '[', ']')
	if end < 0 {
		return nil
	}
	var out []string
	for _, item := range splitPythonTopLevel(raw[1:end], ',') {
		value, ok := parsePythonString(strings.TrimSpace(item))
		if !ok || value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parsePythonBool(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "True", "true":
		return true
	default:
		return false
	}
}

func parsePythonAutoInstall(raw string) (bool, []string) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		deps := parsePythonStringList(raw)
		return len(deps) > 0, deps
	}
	return parsePythonBool(raw), nil
}

func parsePythonString(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return "", false
	}
	quote := raw[0]
	if quote != '\'' && quote != '"' {
		return "", false
	}
	if len(raw) >= 6 && raw[1] == quote && raw[2] == quote {
		end := strings.LastIndex(raw[3:], string([]byte{quote, quote, quote}))
		if end < 0 {
			return "", false
		}
		return raw[3 : 3+end], true
	}
	var out strings.Builder
	for i := 1; i < len(raw); i++ {
		ch := raw[i]
		if ch == quote {
			return out.String(), true
		}
		if ch == '\\' && i+1 < len(raw) {
			i++
			switch raw[i] {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			default:
				out.WriteByte(raw[i])
			}
			continue
		}
		out.WriteByte(ch)
	}
	return "", false
}

func stripPythonComments(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '#' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			if i < len(text) {
				out.WriteByte(text[i])
			}
			continue
		}
		if text[i] == '\'' || text[i] == '"' {
			i = copyPythonString(&out, text, i) - 1
			continue
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

func copyPythonString(out *strings.Builder, text string, start int) int {
	quote := text[start]
	triple := start+2 < len(text) && text[start+1] == quote && text[start+2] == quote
	if triple {
		out.WriteByte(quote)
		out.WriteByte(quote)
		out.WriteByte(quote)
		for i := start + 3; i < len(text); i++ {
			out.WriteByte(text[i])
			if i+2 < len(text) && text[i] == quote && text[i+1] == quote && text[i+2] == quote {
				out.WriteByte(text[i+1])
				out.WriteByte(text[i+2])
				return i + 3
			}
		}
		return len(text)
	}
	out.WriteByte(quote)
	for i := start + 1; i < len(text); i++ {
		out.WriteByte(text[i])
		if text[i] == '\\' && i+1 < len(text) {
			i++
			out.WriteByte(text[i])
			continue
		}
		if text[i] == quote {
			return i + 1
		}
	}
	return len(text)
}

func matchingPythonDelimiter(text string, open int, openCh byte, closeCh byte) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '\'', '"':
			i = copyPythonString(&strings.Builder{}, text, i) - 1
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitPythonTopLevel(text string, sep byte) []string {
	var out []string
	start := 0
	depth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'', '"':
			i = copyPythonString(&strings.Builder{}, text, i) - 1
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		default:
			if text[i] == sep && depth == 0 {
				out = append(out, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(text[start:]))
	return out
}

func splitPythonTopLevelPair(text string, sep byte) (string, string, bool) {
	depth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'', '"':
			i = copyPythonString(&strings.Builder{}, text, i) - 1
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		default:
			if text[i] == sep && depth == 0 {
				return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:]), true
			}
		}
	}
	return "", "", false
}

func technicalName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "_")
}

func trimValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
