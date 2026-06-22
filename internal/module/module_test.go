package module

import (
	"reflect"
	"testing"
)

func TestParseJSONManifest(t *testing.T) {
	manifest, err := ParseManifest([]byte(`{
	  "name": "Base",
	  "technical_name": "base",
	  "version": "19.0.1.0.0",
	  "depends": [],
	  "external_dependencies": {"python": ["html2text"], "bin": ["wkhtmltopdf"]},
	  "external_dependency_hints": {"apt": {"html2text": "python3-html2text"}},
	  "data": ["data/base.xml"],
	  "installable": true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TechnicalName != "base" || !manifest.Installable || len(manifest.Data) != 1 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.ExternalDependencies["python"], []string{"html2text"}) || !reflect.DeepEqual(manifest.ExternalDependencies["bin"], []string{"wkhtmltopdf"}) {
		t.Fatalf("external dependencies = %+v", manifest.ExternalDependencies)
	}
	if !reflect.DeepEqual(manifest.ExternalDependencyHints["apt"], map[string]string{"html2text": "python3-html2text"}) {
		t.Fatalf("external dependency hints = %+v", manifest.ExternalDependencyHints)
	}
}

func TestParseOdooPythonManifestAccount(t *testing.T) {
	manifest, err := ParseManifestForModule([]byte(`{
    'name': 'Invoicing',
    'version': '1.4',
    'category': 'Accounting/Accounting',
    'depends': ['base_setup', 'onboarding', 'product', 'analytic', 'portal', 'digest'],
    'external_dependencies': {
        'python': ['stdnum'],
        'bin': ['wkhtmltopdf'],
        'apt': {'stdnum': 'python3-stdnum'},
    },
    'data': [
        'security/account_security.xml',
        'security/ir.model.access.csv',
        'data/account_data.xml',
    ],
    'demo': [
        'demo/account_demo.xml',
    ],
    'assets': {
        'web.assets_backend': [
            'account/static/src/components/**/*',
            'account/static/src/services/*.js',
        ],
        'web.assets_unit_tests': [
            'account/static/tests/**/*',
            ('remove', 'account/static/tests/tours/**/*'),
        ],
    },
    'installable': True,
    'application': True,
    'license': 'LGPL-3',
}`), "account")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TechnicalName != "account" || manifest.Name != "Invoicing" || manifest.Version != "1.4" || manifest.Category != "Accounting/Accounting" {
		t.Fatalf("manifest header = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"base_setup", "onboarding", "product", "analytic", "portal", "digest"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if !reflect.DeepEqual(manifest.ExternalDependencies["python"], []string{"stdnum"}) || !reflect.DeepEqual(manifest.ExternalDependencies["bin"], []string{"wkhtmltopdf"}) {
		t.Fatalf("external dependencies = %+v", manifest.ExternalDependencies)
	}
	if !reflect.DeepEqual(manifest.ExternalDependencyHints["apt"], map[string]string{"stdnum": "python3-stdnum"}) {
		t.Fatalf("external dependency hints = %+v", manifest.ExternalDependencyHints)
	}
	if !reflect.DeepEqual(manifest.Data, []string{"security/account_security.xml", "security/ir.model.access.csv", "data/account_data.xml"}) {
		t.Fatalf("data = %+v", manifest.Data)
	}
	if !reflect.DeepEqual(manifest.Demo, []string{"demo/account_demo.xml"}) {
		t.Fatalf("demo = %+v", manifest.Demo)
	}
	if !manifest.Installable || !manifest.Application || manifest.SourceLicense != "LGPL-3" {
		t.Fatalf("flags = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Assets["web.assets_backend"], []string{"account/static/src/components/**/*", "account/static/src/services/*.js"}) {
		t.Fatalf("backend assets = %+v", manifest.Assets["web.assets_backend"])
	}
	unitOps := manifest.AssetOperations["web.assets_unit_tests"]
	if len(unitOps) != 2 || unitOps[1].Directive != "remove" || unitOps[1].Path != "account/static/tests/tours/**/*" {
		t.Fatalf("unit asset operations = %+v", unitOps)
	}
}

func TestParseOdooPythonManifestAutoInstallList(t *testing.T) {
	manifest, err := ParseManifestForModule([]byte(`{
    'name': 'Web Enterprise',
    'version': '1.0',
    'depends': ['web'],
    'auto_install': ['web'],
    'data': ['views/webclient_templates.xml'],
}`), "web_enterprise")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.AutoInstall || !reflect.DeepEqual(manifest.AutoInstallDepends, []string{"web"}) {
		t.Fatalf("auto install = %+v", manifest)
	}
}

func TestParseOdooManifestAssetTupleOps(t *testing.T) {
	manifest, err := ParseManifestForModule([]byte(`{
    'name': 'Web Enterprise',
    'version': '1.0',
    'assets': {
        'web._assets_primary_variables': [
            ('after', 'web/static/src/scss/primary_variables.scss', 'web_enterprise/static/src/**/*.variables.scss'),
            ('before', 'web/static/src/scss/primary_variables.scss', 'web_enterprise/static/src/scss/primary_variables.scss'),
            ('include', 'web.dark_mode_variables'),
            ('replace', 'web/static/src/main.js', 'web_enterprise/static/src/main.js'),
            ('remove', 'web_enterprise/static/tests/**/*.test.js'),
        ],
    },
}`), "web_enterprise")
	if err != nil {
		t.Fatal(err)
	}
	ops := manifest.AssetOperations["web._assets_primary_variables"]
	if len(ops) != 5 {
		t.Fatalf("ops = %+v", ops)
	}
	want := []AssetOperation{
		{Directive: "after", Target: "web/static/src/scss/primary_variables.scss", Path: "web_enterprise/static/src/**/*.variables.scss"},
		{Directive: "before", Target: "web/static/src/scss/primary_variables.scss", Path: "web_enterprise/static/src/scss/primary_variables.scss"},
		{Directive: "include", Path: "web.dark_mode_variables"},
		{Directive: "replace", Target: "web/static/src/main.js", Path: "web_enterprise/static/src/main.js"},
		{Directive: "remove", Path: "web_enterprise/static/tests/**/*.test.js"},
	}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("ops = %+v", ops)
	}
	if len(manifest.Assets["web._assets_primary_variables"]) != 0 {
		t.Fatalf("tuple-only assets should not be appended as plain paths: %+v", manifest.Assets)
	}
}

func TestSortByDependencies(t *testing.T) {
	ordered, err := SortByDependencies([]Manifest{
		{Name: "Web", TechnicalName: "web", Version: "19.0", Depends: []string{"base"}},
		{Name: "Base", TechnicalName: "base", Version: "19.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].TechnicalName != "base" || ordered[1].TechnicalName != "web" {
		t.Fatalf("unexpected order: %+v", ordered)
	}
}

func TestSortDetectsCycle(t *testing.T) {
	_, err := SortByDependencies([]Manifest{
		{Name: "A", TechnicalName: "a", Version: "1", Depends: []string{"b"}},
		{Name: "B", TechnicalName: "b", Version: "1", Depends: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}
