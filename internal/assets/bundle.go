package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	Common    = "web.assets_common"
	Backend   = "web.assets_backend"
	Frontend  = "web.assets_frontend"
	UnitTests = "web.assets_unit_tests"
)

type OperationKind string

const (
	Append  OperationKind = "append"
	Prepend OperationKind = "prepend"
	Include OperationKind = "include"
	Before  OperationKind = "before"
	After   OperationKind = "after"
	Remove  OperationKind = "remove"
	Replace OperationKind = "replace"
)

type Operation struct {
	Kind   OperationKind
	Path   string
	Target string
}

type PathResolver interface {
	Resolve(path string) ([]string, error)
}

type FileResolver interface {
	ResolveFile(path string) (string, bool, error)
}

type ManifestOptions struct {
	Debug bool
}

type Registry struct {
	bundles  map[string][]string
	resolver PathResolver
}

func NewRegistry() *Registry {
	return &Registry{bundles: map[string][]string{}}
}

func NewRegistryWithResolver(resolver PathResolver) *Registry {
	reg := NewRegistry()
	reg.resolver = resolver
	return reg
}

func (r *Registry) SetResolver(resolver PathResolver) {
	r.resolver = resolver
}

func (r *Registry) DebugFile(bundle string, assetPath string) (string, bool, error) {
	if r == nil {
		return "", false, nil
	}
	assetPath, ok := safeAssetPath(assetPath)
	if !ok || !bundleContains(r.bundles[bundle], assetPath) {
		return "", false, nil
	}
	resolver, ok := r.resolver.(FileResolver)
	if !ok {
		return "", false, nil
	}
	return resolver.ResolveFile(assetPath)
}

func (r *Registry) Apply(bundle string, ops ...Operation) error {
	items := append([]string(nil), r.bundles[bundle]...)
	for _, op := range ops {
		next, err := r.apply(items, op, map[string]bool{bundle: true})
		if err != nil {
			return fmt.Errorf("%s: %w", bundle, err)
		}
		items = next
	}
	r.bundles[bundle] = dedupe(items)
	return nil
}

func (r *Registry) Bundle(name string) []string {
	return append([]string(nil), r.bundles[name]...)
}

func (r *Registry) Manifest(name string) ([]byte, error) {
	return r.ManifestWithOptions(name, ManifestOptions{})
}

func (r *Registry) ManifestWithOptions(name string, options ManifestOptions) ([]byte, error) {
	items := r.Bundle(name)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%q", items)))
	hash := hex.EncodeToString(sum[:])
	files := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"path": item,
			"type": assetType(item),
		}
		if options.Debug {
			entry["url"] = fmt.Sprintf("/web/assets/debug/%s/%s?v=%s", name, item, hash[:12])
		}
		files = append(files, entry)
	}
	payload := map[string]any{
		"bundle":   name,
		"checksum": hash,
		"debug":    options.Debug,
		"files":    items,
		"hash":     hash,
	}
	if options.Debug {
		payload["assets"] = files
	}
	return json.Marshal(payload)
}

func OperationFromDirective(directive, path, target string) (Operation, error) {
	kind := OperationKind(directive)
	if kind == "" {
		kind = Append
	}
	switch kind {
	case Append, Prepend, Include, Before, After, Remove, Replace:
		return Operation{Kind: kind, Path: path, Target: target}, nil
	default:
		return Operation{}, fmt.Errorf("unsupported directive %s", directive)
	}
}

func (r *Registry) apply(items []string, op Operation, stack map[string]bool) ([]string, error) {
	switch op.Kind {
	case Append:
		paths, err := r.resolve(op.Path)
		if err != nil {
			return nil, err
		}
		return insertAt(items, paths, len(items)), nil
	case Prepend:
		paths, err := r.resolve(op.Path)
		if err != nil {
			return nil, err
		}
		return insertAt(items, paths, 0), nil
	case Include:
		included, err := r.expandBundle(op.Path, stack)
		if err != nil {
			return nil, err
		}
		return insertAt(items, included, len(items)), nil
	case Before:
		targets, err := r.resolve(target(op))
		if err != nil {
			return nil, err
		}
		targetPath := firstPath(targets, target(op))
		idx := indexOf(items, targetPath)
		if idx < 0 {
			return nil, fmt.Errorf("target %s not found", targetPath)
		}
		paths, err := r.resolve(op.Path)
		if err != nil {
			return nil, err
		}
		return insertAt(items, paths, idx), nil
	case After:
		targets, err := r.resolve(target(op))
		if err != nil {
			return nil, err
		}
		targetPath := firstPath(targets, target(op))
		idx := indexOf(items, targetPath)
		if idx < 0 {
			return nil, fmt.Errorf("target %s not found", targetPath)
		}
		paths, err := r.resolve(op.Path)
		if err != nil {
			return nil, err
		}
		return insertAt(items, paths, idx+1), nil
	case Remove:
		paths, err := r.resolve(target(op))
		if err != nil {
			return nil, err
		}
		return removeMany(items, paths), nil
	case Replace:
		targets, err := r.resolve(target(op))
		if err != nil {
			return nil, err
		}
		targetPath := firstPath(targets, target(op))
		idx := indexOf(items, targetPath)
		if idx < 0 {
			return nil, fmt.Errorf("target %s not found", targetPath)
		}
		paths, err := r.resolve(op.Path)
		if err != nil {
			return nil, err
		}
		out := insertAt(items, paths, idx)
		return removeMany(out, targets), nil
	default:
		return nil, fmt.Errorf("unsupported operation %s", op.Kind)
	}
}

func (r *Registry) resolve(path string) ([]string, error) {
	if r.resolver == nil {
		if path == "" {
			return nil, nil
		}
		return []string{path}, nil
	}
	return r.resolver.Resolve(path)
}

func (r *Registry) expandBundle(name string, stack map[string]bool) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("include requires bundle name")
	}
	if stack[name] {
		return nil, fmt.Errorf("asset include cycle at %s", name)
	}
	items, exists := r.bundles[name]
	if !exists {
		return nil, fmt.Errorf("included bundle %s not found", name)
	}
	stack[name] = true
	defer delete(stack, name)
	return append([]string(nil), items...), nil
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func indexOf(items []string, target string) int {
	for i, item := range items {
		if item == target {
			return i
		}
	}
	return -1
}

func remove(items []string, target string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}

func bundleContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func removeMany(items []string, targets []string) []string {
	removeSet := map[string]bool{}
	for _, item := range targets {
		removeSet[item] = true
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if !removeSet[item] {
			out = append(out, item)
		}
	}
	return out
}

func insertAt(items []string, paths []string, index int) []string {
	if index < 0 {
		index = 0
	}
	if index > len(items) {
		index = len(items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item] = true
	}
	insert := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		insert = append(insert, path)
	}
	out := append([]string{}, items[:index]...)
	out = append(out, insert...)
	out = append(out, items[index:]...)
	return out
}

func firstPath(paths []string, fallback string) string {
	if len(paths) > 0 {
		return paths[0]
	}
	return fallback
}

func target(op Operation) string {
	if op.Target != "" {
		return op.Target
	}
	return op.Path
}

type FilesystemResolver struct {
	Root            string
	Extensions      map[string]bool
	InstalledAddons map[string]bool
}

func NewFilesystemResolver(root string) FilesystemResolver {
	return FilesystemResolver{Root: root, Extensions: defaultExtensions()}
}

func (r FilesystemResolver) WithInstalledAddons(addons map[string]bool) FilesystemResolver {
	r.InstalledAddons = map[string]bool{}
	for addon, installed := range addons {
		if installed {
			r.InstalledAddons[addon] = true
		}
	}
	return r
}

func (r FilesystemResolver) Resolve(path string) ([]string, error) {
	path = filepath.ToSlash(path)
	if !r.installed(path) {
		return nil, nil
	}
	if !isGlob(path) {
		return []string{path}, nil
	}
	if !isStaticAssetPath(path) {
		return nil, nil
	}
	root := r.root()
	matches, err := resolveGlobAcrossRoots(root, path)
	if err != nil {
		return nil, err
	}
	extensions := r.Extensions
	if extensions == nil {
		extensions = defaultExtensions()
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		relPath := match.rel
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(relPath)), ".")
		if !extensions[ext] || !r.installed(relPath) {
			continue
		}
		out = append(out, relPath)
	}
	return out, nil
}

func (r FilesystemResolver) ResolveFile(path string) (string, bool, error) {
	path, ok := safeAssetPath(path)
	if !ok || isGlob(path) {
		return "", false, nil
	}
	if !r.installed(path) && !strings.HasPrefix(path, "static/") {
		return "", false, nil
	}
	root := r.root()
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	for _, candidate := range r.fileCandidates(root, path) {
		abs, ok := safeJoin(rootAbs, candidate)
		if !ok {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false, err
		}
		if info.IsDir() {
			continue
		}
		return abs, true, nil
	}
	return "", false, nil
}

func (r FilesystemResolver) root() string {
	if r.Root == "" {
		return "."
	}
	return r.Root
}

func (r FilesystemResolver) fileCandidates(root string, path string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(candidate string) {
		candidate = filepath.ToSlash(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	if strings.HasPrefix(path, "static/") {
		for _, addon := range r.addonNames(root) {
			add(filepath.Join(addon, path))
			add(filepath.Join("addons", addon, path))
		}
		return out
	}
	add(path)
	if isStaticAssetPath(path) {
		add(filepath.Join("addons", path))
		add(filepath.Join("internal", path))
	}
	return out
}

func (r FilesystemResolver) addonNames(root string) []string {
	if len(r.InstalledAddons) > 0 {
		addons := make([]string, 0, len(r.InstalledAddons))
		for addon := range r.InstalledAddons {
			addons = append(addons, addon)
		}
		sort.Strings(addons)
		return addons
	}
	seen := map[string]bool{}
	var addons []string
	for _, base := range []string{root, filepath.Join(root, "addons")} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "" || strings.HasPrefix(entry.Name(), ".") || seen[entry.Name()] {
				continue
			}
			if _, err := os.Stat(filepath.Join(base, entry.Name(), "static")); err == nil {
				seen[entry.Name()] = true
				addons = append(addons, entry.Name())
			}
		}
	}
	sort.Strings(addons)
	return addons
}

func (r FilesystemResolver) installed(path string) bool {
	if len(r.InstalledAddons) == 0 {
		return true
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[1] == "static" {
		return r.InstalledAddons[parts[0]]
	}
	return true
}

type globMatch struct {
	file string
	rel  string
}

func resolveGlobAcrossRoots(root string, pattern string) ([]globMatch, error) {
	roots := []string{root, filepath.Join(root, "addons"), filepath.Join(root, "internal")}
	seen := map[string]bool{}
	var out []globMatch
	for _, base := range roots {
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			continue
		}
		matches, err := resolveGlob(base, pattern)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			rel, err := filepath.Rel(base, match)
			if err != nil {
				return nil, err
			}
			relPath := filepath.ToSlash(rel)
			if seen[relPath] {
				continue
			}
			seen[relPath] = true
			out = append(out, globMatch{file: match, rel: relPath})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].rel < out[j].rel
	})
	return out, nil
}

func resolveGlob(root string, pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
	}
	matcher, err := globStarRegexp(pattern)
	if err != nil {
		return nil, err
	}
	var matches []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if matcher.MatchString(filepath.ToSlash(rel)) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func globStarRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end >= 0 {
				b.WriteString(pattern[i : i+end+2])
				i += end + 1
			} else {
				b.WriteString(`\[`)
			}
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\':
			b.WriteByte('\\')
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func isGlob(path string) bool {
	return strings.ContainsAny(path, "*[]?")
}

func safeAssetPath(path string) (string, bool) {
	path = filepath.ToSlash(path)
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "\x00") {
		return "", false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return path, true
}

func safeJoin(rootAbs string, candidate string) (string, bool) {
	candidate, ok := safeAssetPath(candidate)
	if !ok {
		return "", false
	}
	abs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(candidate)))
	if err != nil {
		return "", false
	}
	if abs == rootAbs || strings.HasPrefix(abs, rootAbs+string(os.PathSeparator)) {
		return abs, true
	}
	return "", false
}

func isStaticAssetPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) >= 3 && parts[0] != "" && parts[1] == "static"
}

func defaultExtensions() map[string]bool {
	return map[string]bool{
		"js":   true,
		"css":  true,
		"xml":  true,
		"scss": true,
		"sass": true,
		"less": true,
	}
}

func assetType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css", ".scss", ".sass", ".less":
		return "style"
	case ".xml":
		return "template"
	case ".js", ".mjs", ".ts":
		return "script"
	default:
		return "asset"
	}
}
