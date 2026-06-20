package oi_base

import (
	"encoding/csv"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorp/internal/base"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestOIBaseManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"base"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	reg := registry.New("test")
	if err := reg.Install([]module.Manifest{base.Manifest(), manifest}); err != nil {
		t.Fatal(err)
	}
	if reg.States[ModuleName] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ModelXMLIDMixin, ModelMany2manyAttachmentResID} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
}

func TestOIBaseManifestDataFixtures(t *testing.T) {
	manifest := Manifest()
	want := []string{
		"security/oi_base_groups.xml",
		"security/ir.model.access.csv",
		"data/oi_sequence_data.xml",
		"view/res_groups.xml",
	}
	if !reflect.DeepEqual(manifest.Data, want) {
		t.Fatalf("data = %+v", manifest.Data)
	}
	for _, rel := range manifest.Data {
		raw, err := os.ReadFile(filepath.Clean(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		switch filepath.Ext(rel) {
		case ".xml":
			assertOdooXMLFixture(t, rel, raw)
		case ".csv":
			assertAccessCSVFixture(t, rel, raw)
		default:
			t.Fatalf("unsupported fixture extension: %s", rel)
		}
	}
}

func TestOIBaseSourceCompatibleModels(t *testing.T) {
	reg := registry.New("test")
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	xmlID := reg.Models[ModelXMLIDMixin]
	if !xmlID.Abstract {
		t.Fatal("xml_id.mixin must be abstract")
	}
	xmlIDField := xmlID.Fields[FieldXMLID]
	if xmlIDField.Kind != field.Char || xmlIDField.Store || !xmlIDField.Readonly {
		t.Fatalf("xml_id field = %+v", xmlIDField)
	}
	attachment := reg.Models[ModelMany2manyAttachmentResID]
	if !attachment.Abstract {
		t.Fatal("many2many.attachment.res_id.mixin must be abstract")
	}

	extensions := extensionMap()
	groups := extensions[ModelResGroups]
	if !reflect.DeepEqual(groups.Inherit, []string{ModelResGroups}) {
		t.Fatalf("res.groups inherit = %+v", groups.Inherit)
	}
	inheritedBy := groups.Fields[FieldInheritedByIDs]
	if inheritedBy.Kind != field.Many2Many || inheritedBy.Relation != ModelResGroups {
		t.Fatalf("inherited_by_ids field = %+v", inheritedBy)
	}

	settings := extensions[ModelResConfigSettings]
	if !settings.Transient || !reflect.DeepEqual(settings.Inherit, []string{ModelResConfigSettings}) {
		t.Fatalf("res.config.settings extension = %+v", settings)
	}
	enterprise := settings.Fields[FieldEnterprise]
	if enterprise.Kind != field.Bool || enterprise.Store || !enterprise.Readonly {
		t.Fatalf("is_enterprise field = %+v", enterprise)
	}
}

func TestOIBaseHelpers(t *testing.T) {
	seq := NewSequence("approval", "APR/", 3, 7)
	if seq.Next() != "APR/007" || seq.Next() != "APR/008" {
		t.Fatal("sequence did not increment with prefix and padding")
	}
	id, err := ParseXMLID("oi_base.record")
	if err != nil {
		t.Fatal(err)
	}
	if id.Module != "oi_base" || id.Name != "record" {
		t.Fatalf("xml id = %+v", id)
	}
	if _, err := ParseXMLID("bad id"); err == nil {
		t.Fatal("expected invalid xml id")
	}
	if EscapeText(`<tag a="b">x&y</tag>`) != "&lt;tag a=&#34;b&#34;&gt;x&amp;y&lt;/tag&gt;" {
		t.Fatal("escape text failed")
	}
	if !ValidNationalID("79927398713") {
		t.Fatal("expected valid Luhn id")
	}
	if Ping()["status"] != "ok" {
		t.Fatal("ping failed")
	}
}

func TestOIBaseSourceCompatibleHelpers(t *testing.T) {
	xmlIDs := map[int64][]string{7: []string{"module.alpha", "module.beta"}}
	if got := XMLIDMixinValue(xmlIDs, 7); got != "module.alpha,module.beta" {
		t.Fatalf("xml id value = %q", got)
	}
	if got := XMLIDMixinValue(xmlIDs, 9); got != "" {
		t.Fatalf("missing xml id value = %q", got)
	}

	implied := map[int64][]int64{
		30: []int64{GroupAdmin, GroupUser},
		10: []int64{GroupUser},
		20: []int64{GroupAdmin},
	}
	if got := InheritedByGroupIDs(implied, GroupUser); !reflect.DeepEqual(got, []int64{10, 30}) {
		t.Fatalf("inherited by = %+v", got)
	}

	attachments := []AttachmentReference{{ID: 1}, {ID: 2, ResID: 99, ResField: "old"}}
	got := SetMany2manyAttachmentResID(42, "document_ids", attachments)
	want := []AttachmentReference{{ID: 1, ResID: 42, ResField: "document_ids"}, {ID: 2, ResID: 42, ResField: "document_ids"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attachment refs = %+v", got)
	}
	if attachments[1].ResID != 99 || attachments[1].ResField != "old" {
		t.Fatalf("input mutated: %+v", attachments)
	}

	if !IsEnterpriseVersionInfo([]any{18, 0, 1, "final", "e"}) {
		t.Fatal("expected enterprise version")
	}
	if IsEnterpriseVersionInfo([]any{18, 0, 1, "final"}) || IsEnterpriseVersionInfo(nil) {
		t.Fatal("expected community/empty version")
	}
}

func TestOIBaseGroupClosure(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Groups[9000] = security.Group{ID: 9000, Name: "Custom", ImpliedIDs: []int64{GroupAdmin}}
	closure := EffectiveGroupClosure(engine.Groups, []int64{9000})
	if !reflect.DeepEqual(closure, []int64{GroupUser, GroupAdmin, 9000}) {
		t.Fatalf("closure = %+v", closure)
	}
}

func extensionMap() map[string]model.Model {
	out := map[string]model.Model{}
	for _, m := range ExtensionModels() {
		out[m.Name] = m
	}
	return out
}

type odooFixtureDoc struct {
	XMLName xml.Name          `xml:"odoo"`
	Records []odooRecord      `xml:"record"`
	Data    []odooFixtureData `xml:"data"`
}

type odooFixtureData struct {
	Records []odooRecord `xml:"record"`
}

type odooRecord struct {
	ID     string      `xml:"id,attr"`
	Model  string      `xml:"model,attr"`
	Fields []odooField `xml:"field"`
}

type odooField struct {
	Name string `xml:"name,attr"`
}

func assertOdooXMLFixture(t *testing.T, rel string, raw []byte) {
	t.Helper()
	var doc odooFixtureDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	if doc.XMLName.Local != "odoo" {
		t.Fatalf("%s root = %s", rel, doc.XMLName.Local)
	}
	records := append([]odooRecord{}, doc.Records...)
	for _, data := range doc.Data {
		records = append(records, data.Records...)
	}
	if len(records) == 0 {
		t.Fatalf("%s has no records", rel)
	}
	for _, record := range records {
		if record.ID == "" || record.Model == "" {
			t.Fatalf("%s has invalid record: %+v", rel, record)
		}
		for _, field := range record.Fields {
			if field.Name == "" {
				t.Fatalf("%s has unnamed field in record %s", rel, record.ID)
			}
		}
	}
}

func assertAccessCSVFixture(t *testing.T, rel string, raw []byte) {
	t.Helper()
	reader := csv.NewReader(strings.NewReader(string(raw)))
	header, err := reader.Read()
	if err != nil {
		t.Fatalf("parse %s header: %v", rel, err)
	}
	want := []string{"id", "name", "model_id:id", "group_id:id", "perm_read", "perm_write", "perm_create", "perm_unlink"}
	if !reflect.DeepEqual(header, want) {
		t.Fatalf("%s header = %+v", rel, header)
	}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("parse %s row: %v", rel, err)
		}
		if len(row) != len(header) {
			t.Fatalf("%s row has %d columns, header has %d", rel, len(row), len(header))
		}
	}
}
