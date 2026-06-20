package oi_login_as

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/impersonation"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

func TestManifestInstallAndModels(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || !manifest.AutoInstall || manifest.Application {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"web", "portal"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
	if manifest.SourceVersion != "18.0.1.2.7" || manifest.SourceLicense == "" {
		t.Fatalf("source metadata = %+v", manifest)
	}

	reg := registry.New("test")
	manifests := []module.Manifest{base.Manifest()}
	manifests = append(manifests, DependencyManifests()...)
	manifests = append(manifests, manifest)
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	if reg.States[ModuleName] != "installed" {
		t.Fatalf("states = %+v", reg.States)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ModelLoginAsWizard, ModelLoginAsAudit, ModelLoginAsRoute} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	wizard := reg.Models[ModelLoginAsWizard]
	if !wizard.Transient {
		t.Fatal("login.as must be transient")
	}
	for _, fieldName := range []string{"group_id", "user_id", "group_ids", "company_id", "company_ids", "return_to", "allow_inactive", "allow_superuser"} {
		if _, ok := wizard.Fields[fieldName]; !ok {
			t.Fatalf("missing wizard field %s", fieldName)
		}
	}
	extensions := extensionMap()
	for _, fieldName := range []string{"allow_login_as", "login_as_group_ids"} {
		if _, ok := extensions["res.users"].Fields[fieldName]; !ok {
			t.Fatalf("missing res.users extension field %s", fieldName)
		}
	}
	for _, fieldName := range []string{"login_as_user_id", "login_as_original_uid", "login_as_banner"} {
		if _, ok := extensions["ir.http"].Fields[fieldName]; !ok {
			t.Fatalf("missing ir.http extension field %s", fieldName)
		}
	}
}

func TestLoginAsManifestFixtures(t *testing.T) {
	manifest := Manifest()
	wantData := []string{
		"security/ir.model.access.csv",
		"view/action.xml",
		"view/login_as.xml",
		"view/templates.xml",
	}
	if !reflect.DeepEqual(manifest.Data, wantData) {
		t.Fatalf("data = %+v", manifest.Data)
	}
	wantAssets := []string{
		"static/src/login_as/login_as.js",
		"static/src/login_as/login_as.xml",
		"static/src/login_as/debug_menu_items.js",
	}
	if !reflect.DeepEqual(manifest.Assets["web.assets_backend"], wantAssets) {
		t.Fatalf("backend assets = %+v", manifest.Assets["web.assets_backend"])
	}

	baseDir := packageDir(t)
	for _, rel := range manifest.Data {
		raw := readFixture(t, baseDir, rel)
		switch filepath.Ext(rel) {
		case ".xml":
			assertOdooXMLFixture(t, rel, raw)
		case ".csv":
			assertAccessCSVFixture(t, rel, raw)
		default:
			t.Fatalf("unsupported manifest data extension: %s", rel)
		}
	}
	for _, rel := range wantAssets {
		raw := readFixture(t, baseDir, rel)
		switch filepath.Ext(rel) {
		case ".xml":
			assertOwlTemplateFixture(t, rel, raw)
		case ".js":
			assertOdooJSFixture(t, rel, raw)
		default:
			t.Fatalf("unsupported asset extension: %s", rel)
		}
	}
}

func TestLoginAsAccessCSVLoadsWithLocalModels(t *testing.T) {
	baseDir := packageDir(t)
	env := loginAsFixtureEnv(t)
	loader := data.NewLoader(env, ModuleName)
	seedLoginAsExternalIDs(t, loader)

	for _, rel := range []string{"view/action.xml", "view/login_as.xml"} {
		file := openFixture(t, baseDir, rel)
		err := loader.LoadXML(file)
		closeErr := file.Close()
		if err != nil {
			t.Fatalf("load %s: %v", rel, err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}

	file := openFixture(t, baseDir, "security/ir.model.access.csv")
	err := loader.LoadCSV("ir.model.access", file)
	closeErr := file.Close()
	if err != nil {
		t.Fatalf("load access csv: %v", err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	ids := loader.ExternalIDs()
	for _, name := range []string{
		ModuleName + ".access_login_as_user",
		ModuleName + ".access_login_as_admin",
		ModuleName + ".access_login_as_audit_admin",
		ModuleName + ".access_login_as_route_admin",
		ModuleName + ".act_login_as",
		ModuleName + ".view_login_as_form",
	} {
		if ids[name].ResID == 0 {
			t.Fatalf("missing external id %s in %+v", name, ids)
		}
	}

	rows, err := env.Model("ir.actions.act_window").Browse(ids[ModuleName+".act_login_as"].ResID).Read("res_model", "view_mode")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["res_model"] != ModelLoginAsWizard || rows[0]["view_mode"] != "form" {
		t.Fatalf("action row = %+v", rows[0])
	}
	rows, err = env.Model("ir.ui.view").Browse(ids[ModuleName+".view_login_as_form"].ResID).Read("model", "arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := rows[0]["arch"].(string)
	if rows[0]["model"] != ModelLoginAsWizard || !strings.Contains(arch, `field name="user_id"`) {
		t.Fatalf("view row = %+v", rows[0])
	}

	accessRows, err := env.Model("ir.model.access").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if accessRows.Len() != 4 {
		t.Fatalf("ACL count = %d, want 4", accessRows.Len())
	}

	engine := security.NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	userGroup := ids[ModuleName+".group_login_as_user"].ResID
	adminGroup := ids[ModuleName+".group_login_as_admin"].ResID
	engine.Users[10] = security.User{ID: 10, Login: "user", Active: true, GroupIDs: []int64{userGroup}}
	engine.Users[20] = security.User{ID: 20, Login: "admin", Active: true, GroupIDs: []int64{adminGroup}}
	if err := engine.Check(record.Context{UserID: 10}, ModelLoginAsWizard, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, ModelLoginAsAudit, record.OpRead, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected audit denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20}, ModelLoginAsRoute, record.OpCreate, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 20}, ModelLoginAsRoute, record.OpUnlink, nil); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected route unlink denied, got %v", err)
	}
}

func TestLoginAsAddonSecurity(t *testing.T) {
	engine := security.NewEngine()
	ApplySecurity(engine)
	engine.Users[10] = security.User{ID: 10, Login: "user", Active: true, GroupIDs: []int64{GroupLoginAsUser}}
	engine.Users[20] = security.User{ID: 20, Login: "admin", Active: true, GroupIDs: []int64{GroupLoginAsAdmin}}
	engine.Users[30] = security.User{ID: 30, Login: "none", Active: true}

	if err := CheckLoginAsAccess(engine, 10, ModelLoginAsWizard, record.OpCreate); err != nil {
		t.Fatal(err)
	}
	if err := CheckLoginAsAccess(engine, 10, ModelLoginAsAudit, record.OpRead); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected audit denied, got %v", err)
	}
	if err := CheckLoginAsAccess(engine, 20, ModelLoginAsAudit, record.OpRead); err != nil {
		t.Fatal(err)
	}
	if err := CheckLoginAsAccess(engine, 30, ModelLoginAsWizard, record.OpRead); !errors.Is(err, security.ErrAccessDenied) {
		t.Fatalf("expected no-group denied, got %v", err)
	}
}

func TestLoginAsAddonServiceDefaults(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := impersonation.NewService(impersonation.WithConfig(DefaultConfig()), impersonation.WithNow(func() time.Time { return now }))
	svc.SetUser(impersonation.User{ID: 1, Login: "root", Name: "System", Active: true, Superuser: true, GroupIDs: []int64{GroupLoginAsAdmin, GroupLoginAsAllowSuperuser, GroupLoginAsDebug}})
	svc.SetUser(impersonation.User{ID: 10, Login: "admin", Name: "Admin", Active: true, GroupIDs: []int64{GroupLoginAsAdmin}})
	svc.SetUser(impersonation.User{ID: 20, Login: "portal", Name: "Portal", Active: true, Portal: true, GroupIDs: []int64{40}})

	action, err := svc.WizardAction(10, 20, impersonation.SwitchOptions{GroupID: 40, ReturnTo: "/web"})
	if err != nil {
		t.Fatal(err)
	}
	if action.Route != "/web/login_as/20" {
		t.Fatalf("action = %+v", action)
	}
	session, err := svc.Start("sid", 10, 20, impersonation.SwitchOptions{GroupID: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !session.Impersonating || session.OriginalUserID != 10 || session.UserID != 20 {
		t.Fatalf("session = %+v", session)
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
	XMLName   xml.Name              `xml:"odoo"`
	Records   []odooFixtureRecord   `xml:"record"`
	Data      []odooFixtureData     `xml:"data"`
	Templates []odooFixtureTemplate `xml:"template"`
}

type odooFixtureData struct {
	Records []odooFixtureRecord `xml:"record"`
}

type odooFixtureRecord struct {
	ID     string             `xml:"id,attr"`
	Model  string             `xml:"model,attr"`
	Fields []odooFixtureField `xml:"field"`
}

type odooFixtureField struct {
	Name string `xml:"name,attr"`
}

type odooFixtureTemplate struct {
	ID string `xml:"id,attr"`
}

type owlTemplateDoc struct {
	XMLName   xml.Name      `xml:"templates"`
	Templates []owlTemplate `xml:"t"`
}

type owlTemplate struct {
	Name string `xml:"t-name,attr"`
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
	records := append([]odooFixtureRecord{}, doc.Records...)
	for _, group := range doc.Data {
		records = append(records, group.Records...)
	}
	if len(records) == 0 && len(doc.Templates) == 0 {
		t.Fatalf("%s has no records or templates", rel)
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
	for _, template := range doc.Templates {
		if template.ID == "" {
			t.Fatalf("%s has unnamed template", rel)
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
	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse %s row: %v", rel, err)
		}
		if len(row) != len(header) {
			t.Fatalf("%s row has %d columns, header has %d", rel, len(row), len(header))
		}
		if row[0] == "" || row[2] == "" || row[3] == "" {
			t.Fatalf("%s has incomplete access row: %+v", rel, row)
		}
		count++
	}
	if count == 0 {
		t.Fatalf("%s has no access rows", rel)
	}
}

func assertOwlTemplateFixture(t *testing.T, rel string, raw []byte) {
	t.Helper()
	var doc owlTemplateDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	if doc.XMLName.Local != "templates" {
		t.Fatalf("%s root = %s", rel, doc.XMLName.Local)
	}
	for _, template := range doc.Templates {
		if template.Name == "oi_login_as.LoginAs" {
			return
		}
	}
	t.Fatalf("%s missing oi_login_as.LoginAs template", rel)
}

func assertOdooJSFixture(t *testing.T, rel string, raw []byte) {
	t.Helper()
	text := string(raw)
	if !strings.Contains(text, "@odoo-module") {
		t.Fatalf("%s missing odoo module marker", rel)
	}
	if !strings.Contains(text, "login.as") && !strings.Contains(text, "Login As") {
		t.Fatalf("%s missing login-as intent", rel)
	}
	if rel == "static/src/login_as/login_as.js" && !strings.Contains(text, "/web/login_back") {
		t.Fatalf("%s missing login_back route", rel)
	}
}

func seedLoginAsExternalIDs(t *testing.T, loader *data.Loader) {
	t.Helper()
	var seed strings.Builder
	seed.WriteString("<odoo>")
	for _, group := range SecurityGroups() {
		id := groupExternalID(group.ID)
		seed.WriteString(`<record id="` + id + `" model="res.groups">`)
		seed.WriteString(`<field name="name">` + group.Name + `</field>`)
		if len(group.ImpliedIDs) > 0 {
			seed.WriteString(`<field name="implied_ids" eval="[`)
			for i, implied := range group.ImpliedIDs {
				if i > 0 {
					seed.WriteByte(',')
				}
				seed.WriteString(`(4, ref('` + groupExternalID(implied) + `'))`)
			}
			seed.WriteString(`]"/>`)
		}
		seed.WriteString(`</record>`)
	}
	for _, m := range Models() {
		seed.WriteString(`<record id="` + modelExternalID(m.Name) + `" model="ir.model">`)
		seed.WriteString(`<field name="model">` + m.Name + `</field>`)
		seed.WriteString(`<field name="name">` + m.Name + `</field>`)
		seed.WriteString(`</record>`)
	}
	seed.WriteString("</odoo>")
	if err := loader.LoadXML(strings.NewReader(seed.String())); err != nil {
		t.Fatal(err)
	}
}

func loginAsFixtureEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

func modelExternalID(modelName string) string {
	return "model_" + strings.NewReplacer(".", "_").Replace(modelName)
}

func groupExternalID(groupID int64) string {
	switch groupID {
	case GroupLoginAsUser:
		return "group_login_as_user"
	case GroupLoginAsAdmin:
		return "group_login_as_admin"
	case GroupLoginAsAllowInactive:
		return "group_login_as_allow_inactive"
	case GroupLoginAsAllowSuperuser:
		return "group_login_as_allow_superuser"
	case GroupLoginAsDebug:
		return "group_login_as_debug"
	default:
		return fmt.Sprintf("group_login_as_%d", groupID)
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Dir(file)
}

func readFixture(t *testing.T, baseDir string, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(baseDir, filepath.Clean(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		t.Fatalf("%s is empty", rel)
	}
	return raw
}

func openFixture(t *testing.T, baseDir string, rel string) *os.File {
	t.Helper()
	file, err := os.Open(filepath.Join(baseDir, filepath.Clean(rel)))
	if err != nil {
		t.Fatalf("open %s: %v", rel, err)
	}
	return file
}
