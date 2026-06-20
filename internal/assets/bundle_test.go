package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleOrder(t *testing.T) {
	reg := NewRegistry()
	err := reg.Apply(Backend,
		Operation{Kind: Append, Path: "base.js"},
		Operation{Kind: Before, Path: "pre.js", Target: "base.js"},
		Operation{Kind: After, Path: "post.js", Target: "base.js"},
		Operation{Kind: Replace, Path: "base.v2.js", Target: "base.js"},
		Operation{Kind: Remove, Target: "post.js"},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(Backend)
	want := []string{"pre.js", "base.v2.js"}
	if len(got) != len(want) {
		t.Fatalf("bundle = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bundle = %+v", got)
		}
	}
}

func TestBundleIncludeExpandsExistingBundle(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Apply("web._assets_helpers",
		Operation{Kind: Append, Path: "helpers/a.scss"},
		Operation{Kind: Append, Path: "helpers/b.scss"},
	); err != nil {
		t.Fatal(err)
	}
	if err := reg.Apply(Backend,
		Operation{Kind: Append, Path: "webclient.js"},
		Operation{Kind: Include, Path: "web._assets_helpers"},
		Operation{Kind: After, Target: "helpers/a.scss", Path: "helpers/a.patch.scss"},
		Operation{Kind: Remove, Path: "helpers/b.scss"},
	); err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(Backend)
	want := []string{"webclient.js", "helpers/a.scss", "helpers/a.patch.scss"}
	if len(got) != len(want) {
		t.Fatalf("bundle = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bundle = %+v", got)
		}
	}
}

func TestBundleIncludeRequiresExistingBundle(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Apply(Backend, Operation{Kind: Include, Path: "web.missing"}); err == nil {
		t.Fatal("expected missing include error")
	}
}

func TestFilesystemResolverExpandsStaticGlobInSortedOrder(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "web/static/src/js/b.js")
	writeAsset(t, root, "web/static/src/js/a.js")
	writeAsset(t, root, "web/static/src/js/nested/c.js")
	writeAsset(t, root, "web/static/src/js/ignored.txt")
	reg := NewRegistryWithResolver(NewFilesystemResolver(root))
	if err := reg.Apply(Backend, Operation{Kind: Append, Path: "web/static/src/**/*.js"}); err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(Backend)
	want := []string{
		"web/static/src/js/a.js",
		"web/static/src/js/b.js",
		"web/static/src/js/nested/c.js",
	}
	if len(got) != len(want) {
		t.Fatalf("bundle = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bundle = %+v", got)
		}
	}
}

func TestFilesystemResolverFiltersUninstalledAddonAssets(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "web/static/src/js/a.js")
	writeAsset(t, root, "ghost/static/src/js/b.js")
	reg := NewRegistryWithResolver(NewFilesystemResolver(root).WithInstalledAddons(map[string]bool{"web": true}))
	if err := reg.Apply(Backend,
		Operation{Kind: Append, Path: "web/static/src/js/a.js"},
		Operation{Kind: Append, Path: "ghost/static/src/js/b.js"},
		Operation{Kind: Append, Path: "ghost/static/src/**/*.js"},
	); err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(Backend)
	if len(got) != 1 || got[0] != "web/static/src/js/a.js" {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestRegistryDebugFileServesBundleAssetFromAddonFilesystem(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "addons/web/static/src/js/a.js")
	writeAsset(t, root, "addons/ghost/static/src/js/b.js")
	reg := NewRegistryWithResolver(NewFilesystemResolver(root).WithInstalledAddons(map[string]bool{"web": true}))
	if err := reg.Apply(Backend, Operation{Kind: Append, Path: "web/static/src/js/a.js"}); err != nil {
		t.Fatal(err)
	}
	filename, ok, err := reg.DebugFile(Backend, "web/static/src/js/a.js")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || filepath.ToSlash(filename) != filepath.ToSlash(filepath.Join(root, "addons/web/static/src/js/a.js")) {
		t.Fatalf("file = %q ok=%v", filename, ok)
	}
	if _, ok, err := reg.DebugFile(Backend, "ghost/static/src/js/b.js"); err != nil || ok {
		t.Fatalf("ghost file ok=%v err=%v", ok, err)
	}
	if _, ok, err := reg.DebugFile(Backend, "../secret.js"); err != nil || ok {
		t.Fatalf("traversal ok=%v err=%v", ok, err)
	}
	if _, ok, err := reg.DebugFile("web.assets_missing", "web/static/src/js/a.js"); err != nil || ok {
		t.Fatalf("missing bundle ok=%v err=%v", ok, err)
	}
}

func TestFilesystemResolverResolveFileSearchesInstalledAddonRelativeAssets(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "addons/web/static/src/js/a.js")
	writeAsset(t, root, "addons/ghost/static/src/js/a.js")
	resolver := NewFilesystemResolver(root).WithInstalledAddons(map[string]bool{"web": true})
	filename, ok, err := resolver.ResolveFile("static/src/js/a.js")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || filepath.ToSlash(filename) != filepath.ToSlash(filepath.Join(root, "addons/web/static/src/js/a.js")) {
		t.Fatalf("file = %q ok=%v", filename, ok)
	}
	if _, ok, err := resolver.ResolveFile("ghost/static/src/js/a.js"); err != nil || ok {
		t.Fatalf("uninstalled addon ok=%v err=%v", ok, err)
	}
}

func TestBundleGlobOperationsKeepExistingDuplicatePosition(t *testing.T) {
	root := t.TempDir()
	writeAsset(t, root, "web/static/src/js/a.js")
	writeAsset(t, root, "web/static/src/js/b.js")
	writeAsset(t, root, "web/static/src/js/c.js")
	reg := NewRegistryWithResolver(NewFilesystemResolver(root))
	if err := reg.Apply(Backend,
		Operation{Kind: Append, Path: "web/static/src/js/b.js"},
		Operation{Kind: Prepend, Path: "web/static/src/js/*.js"},
		Operation{Kind: Replace, Target: "web/static/src/js/b.js", Path: "web/static/src/js/c.js"},
		Operation{Kind: Remove, Path: "web/static/src/js/a.js"},
	); err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(Backend)
	want := []string{"web/static/src/js/c.js"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestOperationFromDirective(t *testing.T) {
	op, err := OperationFromDirective("before", "patch.scss", "base.scss")
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != Before || op.Path != "patch.scss" || op.Target != "base.scss" {
		t.Fatalf("operation = %+v", op)
	}
	op, err = OperationFromDirective("", "base.js", "")
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != Append || op.Path != "base.js" {
		t.Fatalf("operation = %+v", op)
	}
}

func TestManifest(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Apply(Common, Operation{Kind: Append, Path: "a.js"}); err != nil {
		t.Fatal(err)
	}
	data, err := reg.Manifest(Common)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["bundle"] != Common || payload["hash"] == "" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestManifestDebugAssets(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Apply(Common, Operation{Kind: Append, Path: "web/static/src/js/a.js"}); err != nil {
		t.Fatal(err)
	}
	data, err := reg.ManifestWithOptions(Common, ManifestOptions{Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	assetsPayload, ok := payload["assets"].([]any)
	if !ok || len(assetsPayload) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	entry := assetsPayload[0].(map[string]any)
	if entry["path"] != "web/static/src/js/a.js" || entry["type"] != "script" || entry["url"] == "" || payload["checksum"] == "" {
		t.Fatalf("payload = %+v", payload)
	}
}

func writeAsset(t *testing.T, root string, rel string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
		t.Fatal(err)
	}
}
