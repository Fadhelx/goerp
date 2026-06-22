package data

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
)

func TestLoadXMLRecordsExternalID(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_admin" model="res.partner">
    <field name="name">Administrator</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	external := ids["base.res_partner_admin"]
	if external.Model != "res.partner" || external.ResID == 0 {
		t.Fatalf("unexpected external id: %+v", external)
	}
}

func TestLoadCSV(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadCSV("res.partner", strings.NewReader("id,name\nres_partner_demo,Demo\n"))
	if err != nil {
		t.Fatal(err)
	}

	if loader.ExternalIDs()["base.res_partner_demo"].ResID == 0 {
		t.Fatal("missing external id")
	}
}

func TestLoadCSVNestedContinuationRows(t *testing.T) {
	env := testCommandEnv(t)
	loader := NewLoader(env, "x")

	err := loader.LoadCSV("x.parent", strings.NewReader(`id,name,child_ids/name
parent_csv,Parent,Child A
,,Child B
`))
	if err != nil {
		t.Fatal(err)
	}

	parentID := loader.ExternalIDs()["x.parent_csv"].ResID
	childIDs := fieldValue(t, env, "x.parent", parentID, "child_ids").([]int64)
	if len(childIDs) != 2 {
		t.Fatalf("child ids = %#v", childIDs)
	}
	assertField(t, env, "x.child", childIDs[0], "name", "Child A")
	assertField(t, env, "x.child", childIDs[0], "parent_id", parentID)
	assertField(t, env, "x.child", childIDs[1], "name", "Child B")
	assertField(t, env, "x.child", childIDs[1], "parent_id", parentID)
}

func TestLoadCSVPlainRelationRefs(t *testing.T) {
	env := testCommandEnv(t)
	loader := NewLoader(env, "x")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="parent_ref" model="x.parent">
    <field name="name">Parent</field>
  </record>
  <record id="tag_a" model="x.tag">
    <field name="name">A</field>
  </record>
  <record id="tag_b" model="x.tag">
    <field name="name">B</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadCSV("x.child", strings.NewReader("id,name,parent_id,tag_ids\nchild_csv,Child,parent_ref,\"tag_a,tag_b\"\n"))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	childID := ids["x.child_csv"].ResID
	assertField(t, env, "x.child", childID, "parent_id", ids["x.parent_ref"].ResID)
	assertField(t, env, "x.child", childID, "tag_ids", []int64{ids["x.tag_a"].ResID, ids["x.tag_b"].ResID})
}

func TestLoadCSVChartTemplateHeaderTranslationAndUnknownFields(t *testing.T) {
	env := testAccountCSVEnv(t)
	loader := NewLoader(env, "account")

	err := loader.LoadCSV("account.account", strings.NewReader("id,name, \"name@cs\",non_trade,\naccount_cash,Cash,Hotovost,False,\n"))
	if err != nil {
		t.Fatal(err)
	}

	accountID := loader.ExternalIDs()["account.account_cash"].ResID
	assertField(t, env, "account.account", accountID, "name", "Cash")
}

func TestLoadCSVChartTemplateFalseRelationAndCharString(t *testing.T) {
	env := testAccountCSVEnv(t)
	loader := NewLoader(env, "account")

	err := loader.LoadCSV("account.tax", strings.NewReader(`id,invoice_label,repartition_line_ids/account_id
tax_sale,False,False
`))
	if err != nil {
		t.Fatal(err)
	}

	taxID := loader.ExternalIDs()["account.tax_sale"].ResID
	assertField(t, env, "account.tax", taxID, "invoice_label", "False")
	lineIDs := fieldValue(t, env, "account.tax", taxID, "repartition_line_ids").([]int64)
	if len(lineIDs) != 1 {
		t.Fatalf("repartition lines = %#v", lineIDs)
	}
	assertField(t, env, "account.tax.repartition.line", lineIDs[0], "tax_id", taxID)
	assertField(t, env, "account.tax.repartition.line", lineIDs[0], "account_id", nil)
}

func TestLoadCSVChartTemplatePipeDelimitedRefs(t *testing.T) {
	env := testAccountCSVEnv(t)
	loader := NewLoader(env, "account")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="tag_a" model="account.account.tag">
    <field name="name">A</field>
  </record>
  <record id="tag_b" model="account.account.tag">
    <field name="name">B</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadCSV("account.account", strings.NewReader("id,name,tag_ids\naccount_cash,Cash,tag_a||tag_b\n"))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "account.account", ids["account.account_cash"].ResID, "tag_ids", []int64{ids["account.tag_a"].ResID, ids["account.tag_b"].ResID})
}

func TestLoadCSVChartTemplateDeferredForwardRefs(t *testing.T) {
	env := testAccountCSVEnv(t)
	loader := NewLoader(env, "account")

	err := loader.LoadCSV("account.tax", strings.NewReader(`id,name,children_tax_ids,original_tax_ids
tax_grouped,Grouped,"tax_child_a,tax_child_b",tax_later
tax_child_a,Child A,,
tax_child_b,Child B,,
tax_later,Later,,
`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "account.tax", ids["account.tax_grouped"].ResID, "children_tax_ids", []int64{ids["account.tax_child_a"].ResID, ids["account.tax_child_b"].ResID})
	assertField(t, env, "account.tax", ids["account.tax_grouped"].ResID, "original_tax_ids", []int64{ids["account.tax_later"].ResID})
}

func TestLoadXMLFunctionInstallLangActivatesExistingLanguage(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadCSV("res.lang", strings.NewReader(`id,name,code,iso_code,direction,active
lang_en,English (US),en_US,en,Left-to-Right,False
`))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadXML(strings.NewReader(`<odoo>
  <function name="install_lang" model="res.lang"/>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	langID := loader.ExternalIDs()["base.lang_en"].ResID
	assertField(t, env, "res.lang", langID, "active", true)
	defaultRows, err := env.Model("ir.default").Search(domain.And(domain.Cond("model", "=", "res.partner"), domain.Cond("field", "=", "lang")))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := defaultRows.Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "en_US" {
		t.Fatalf("default lang rows = %+v", rows)
	}
}

func TestLoadXMLFunctionInstallLangKeepsExistingDefault(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	if err := loader.LoadCSV("res.lang", strings.NewReader("id,name,code,iso_code,direction,active\nlang_en,English (US),en_US,en,Left-to-Right,False\n")); err != nil {
		t.Fatal(err)
	}
	if err := loader.LoadXML(strings.NewReader(`<odoo>
  <function model="ir.default" name="set" eval="('res.partner', 'lang', 'fr_FR')"/>
  <function name="install_lang" model="res.lang"/>
</odoo>`)); err != nil {
		t.Fatal(err)
	}

	defaultRows, err := env.Model("ir.default").Search(domain.And(domain.Cond("model", "=", "res.partner"), domain.Cond("field", "=", "lang")))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := defaultRows.Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "fr_FR" {
		t.Fatalf("default lang rows = %+v", rows)
	}
}

func TestLoadXMLBaseResLangDataOrdering(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadCSV("res.lang", strings.NewReader(`id,name,code,iso_code,direction,active
lang_en,English (US),en_US,en,Left-to-Right,False
lang_es,Spanish / Español,es_ES,es,Left-to-Right,True
`))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadXML(strings.NewReader(`<odoo>
  <data noupdate="1">
    <record id="base.lang_es" model="res.lang">
      <field name="url_code">es_ES</field>
    </record>
    <function name="install_lang" model="res.lang"/>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	if ids["base.lang_es"].ResID == 0 {
		t.Fatalf("missing lang_es external id: %+v", ids)
	}
	assertField(t, env, "res.lang", ids["base.lang_en"].ResID, "active", true)
}

func TestLoadXMLReloadExternalIDUpdatesExistingRecord(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_demo" model="res.partner">
    <field name="name">Demo</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	firstID := loader.ExternalIDs()["base.res_partner_demo"].ResID

	err = loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_demo" model="res.partner">
    <field name="name">Updated Demo</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	secondID := loader.ExternalIDs()["base.res_partner_demo"].ResID
	if secondID != firstID {
		t.Fatalf("expected same res id, got %d then %d", firstID, secondID)
	}
	assertField(t, env, "res.partner", firstID, "name", "Updated Demo")
	assertModelCount(t, env, "res.partner", 1)
}

func TestLoadCSVReloadExternalIDUpdatesExistingRecord(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadCSV("res.partner", strings.NewReader("id,name\nres_partner_demo,Demo\n"))
	if err != nil {
		t.Fatal(err)
	}
	firstID := loader.ExternalIDs()["base.res_partner_demo"].ResID

	err = loader.LoadCSV("res.partner", strings.NewReader("id,name\nres_partner_demo,Updated Demo\n"))
	if err != nil {
		t.Fatal(err)
	}

	secondID := loader.ExternalIDs()["base.res_partner_demo"].ResID
	if secondID != firstID {
		t.Fatalf("expected same res id, got %d then %d", firstID, secondID)
	}
	assertField(t, env, "res.partner", firstID, "name", "Updated Demo")
	assertModelCount(t, env, "res.partner", 1)
}

func TestLoadXMLRefAndNoupdate(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
  <data noupdate="1">
    <record id="user_admin" model="res.users">
      <field name="name">Admin</field>
      <field name="company_id" ref="company_main"/>
    </record>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	if !ids["base.user_admin"].Noupdate {
		t.Fatal("expected noupdate external id")
	}
	rows, err := env.Model("res.users").Browse(ids["base.user_admin"].ResID).Read("company_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["company_id"] != ids["base.company_main"].ResID {
		t.Fatalf("unexpected ref value: %+v", rows)
	}
}

func TestLoadXMLFieldTypeXMLPreservesNestedArch(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="view_partner_form" model="ir.ui.view">
    <field name="name">partner.form</field>
    <field name="model">res.partner</field>
    <field name="arch" type="xml">
      <form string="Partner">
        <sheet>
          <field name="name" domain="[('active','=',True)]"/>
          <button name="action_demo" type="object" string="Demo"/>
        </sheet>
      </form>
    </field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	viewID := loader.ExternalIDs()["base.view_partner_form"].ResID
	rows, err := env.Model("ir.ui.view").Browse(viewID).Read("arch")
	if err != nil {
		t.Fatal(err)
	}
	arch, _ := rows[0]["arch"].(string)
	for _, want := range []string{`<form string="Partner">`, `domain="[('active','=',True)]"`, `<button name="action_demo"`} {
		if !strings.Contains(arch, want) {
			t.Fatalf("arch missing %q: %s", want, arch)
		}
	}
}

func TestLoadXMLMenuitemAndTemplate(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_demo" model="res.groups">
    <field name="name">Demo Group</field>
  </record>
  <record id="action_partner" model="ir.actions.act_window">
    <field name="name">Partners</field>
    <field name="res_model">res.partner</field>
    <field name="view_mode">list,form</field>
  </record>
  <record id="action_server" model="ir.actions.server">
    <field name="name">Run Server</field>
    <field name="model_name">res.partner</field>
    <field name="state">code</field>
    <field name="code">action = False</field>
  </record>
  <record id="action_report" model="ir.actions.report">
    <field name="name">Partner Report</field>
    <field name="model">res.partner</field>
    <field name="report_name">base.partner_report</field>
  </record>
  <record id="action_url" model="ir.actions.act_url">
    <field name="name">Docs</field>
    <field name="url">https://example.test/docs</field>
  </record>
  <record id="action_client" model="ir.actions.client">
    <field name="name">Discuss</field>
    <field name="tag">mail.action_discuss</field>
  </record>
  <menuitem id="menu_root" name="Root" sequence="5"/>
  <menuitem id="menu_child" name="Child" active="True" parent="menu_root" action="action_partner" sequence="10" groups="group_demo" web_icon="fa-users,#fff,#000" web_icon_data="abc123" web_icon_data_mimetype="image/png"/>
  <menuitem id="menu_server" name="Server" parent="menu_root" action="action_server"/>
  <menuitem id="menu_report" name="Report" parent="menu_root" action="action_report"/>
  <menuitem id="menu_url" name="URL" parent="menu_root" action="action_url"/>
  <menuitem id="menu_client" name="Client" parent="menu_root" action="action_client"/>
  <template id="base_template" name="Base Template">
    <t t-name="base.Template"><span>Base</span></t>
  </template>
  <template id="child_template" inherit_id="base_template" priority="20" groups="group_demo" active="False" primary="True" customize_show="True" track="True" page="True">
    <xpath expr="." position="inside"><span>Child</span></xpath>
  </template>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	groupID := ids["base.group_demo"].ResID
	rootID := ids["base.menu_root"].ResID
	childID := ids["base.menu_child"].ResID
	actionPartnerID := ids["base.action_partner"].ResID
	if rootID == 0 || childID == 0 {
		t.Fatalf("menu external ids = %+v", ids)
	}
	rows, err := env.Model("ir.ui.menu").Browse(childID).Read("name", "active", "parent_id", "action", "sequence", "groups_id", "web_icon", "web_icon_data", "web_icon_data_mimetype")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["name"] != "Child" || row["active"] != true || row["parent_id"] != rootID || row["action"] != "ir.actions.act_window,"+strconv.FormatInt(actionPartnerID, 10) || row["sequence"] != int64(10) || row["web_icon"] != "fa-users,#fff,#000" || row["web_icon_data"] != "abc123" || row["web_icon_data_mimetype"] != "image/png" {
		t.Fatalf("menu row = %+v", row)
	}
	if !reflect.DeepEqual(row["groups_id"], []int64{groupID}) {
		t.Fatalf("menu groups = %#v", row["groups_id"])
	}
	for _, item := range []struct {
		menuKey string
		model   string
		action  string
	}{
		{"base.menu_server", "ir.actions.server", "base.action_server"},
		{"base.menu_report", "ir.actions.report", "base.action_report"},
		{"base.menu_url", "ir.actions.act_url", "base.action_url"},
		{"base.menu_client", "ir.actions.client", "base.action_client"},
	} {
		menuID := ids[item.menuKey].ResID
		actionID := ids[item.action].ResID
		rows, err := env.Model("ir.ui.menu").Browse(menuID).Read("action")
		if err != nil {
			t.Fatal(err)
		}
		want := item.model + "," + strconv.FormatInt(actionID, 10)
		if rows[0]["action"] != want {
			t.Fatalf("%s action = %#v, want %s", item.menuKey, rows[0]["action"], want)
		}
	}

	baseTemplateID := ids["base.base_template"].ResID
	childTemplateID := ids["base.child_template"].ResID
	rows, err = env.Model("ir.ui.view").Browse(childTemplateID).Read("type", "key", "inherit_id", "inherit_id_ref", "priority", "arch", "active", "groups_id", "primary", "customize_show", "track", "page", "mode")
	if err != nil {
		t.Fatal(err)
	}
	row = rows[0]
	if row["type"] != "qweb" || row["key"] != "base.child_template" || row["inherit_id"] != baseTemplateID || row["inherit_id_ref"] != "base_template" || row["priority"] != int64(20) {
		t.Fatalf("template row = %+v", row)
	}
	if row["active"] != false || row["primary"] != true || row["customize_show"] != true || row["track"] != true || row["page"] != true || row["mode"] != "primary" {
		t.Fatalf("template attrs = %+v", row)
	}
	if !reflect.DeepEqual(row["groups_id"], []int64{groupID}) {
		t.Fatalf("template groups = %#v", row["groups_id"])
	}
	if !strings.Contains(row["arch"].(string), "<xpath") || !strings.Contains(row["arch"].(string), "Child") {
		t.Fatalf("template arch = %s", row["arch"])
	}
}

func TestLoadXMLAssetTag(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <asset id="test_asset_tag_extra" name="Test asset tag with extra field" active="False">
    <bundle directive="prepend">test_asset_bundle</bundle>
    <path>base/tests/something.scss</path>
    <field name="sequence" eval="17"/>
  </asset>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	assetID := loader.ExternalIDs()["base.test_asset_tag_extra"].ResID
	rows, err := env.Model("ir.asset").Browse(assetID).Read("name", "active", "bundle", "directive", "path", "sequence")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["name"] != "Test asset tag with extra field" || row["active"] != false || row["bundle"] != "test_asset_bundle" || row["directive"] != "prepend" || row["path"] != "base/tests/something.scss" || row["sequence"] != int64(17) {
		t.Fatalf("asset row = %+v", row)
	}
}

func TestLoadXMLLegacyReportTag(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Partner</field>
  </record>
  <record id="group_demo" model="res.groups">
    <field name="name">Demo Group</field>
  </record>
  <report id="action_report_partner"
    string="Partner Report"
    model="res.partner"
    report_type="qweb-pdf"
    name="base.report_partner"
    file="base.report_partner"
    print_report_name="'Partner - %s' % (object.name)"
    attachment="'partner.pdf'"
    attachment_use="True"
    groups="group_demo"/>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	reportID := ids["base.action_report_partner"].ResID
	rows, err := env.Model("ir.actions.report").Browse(reportID).Read("name", "model", "report_name", "report_type", "report_file", "print_report_name", "attachment", "attachment_use", "binding_model_id", "binding_type", "binding_view_types", "groups_id")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["name"] != "Partner Report" || row["model"] != "res.partner" || row["report_name"] != "base.report_partner" || row["report_type"] != "qweb-pdf" || row["report_file"] != "base.report_partner" {
		t.Fatalf("report row = %+v", row)
	}
	if row["print_report_name"] != "'Partner - %s' % (object.name)" || row["attachment"] != "'partner.pdf'" || row["attachment_use"] != true {
		t.Fatalf("report print/attachment row = %+v", row)
	}
	if row["binding_model_id"] != ids["base.model_res_partner"].ResID || row["binding_type"] != "report" || row["binding_view_types"] != "list,form" {
		t.Fatalf("report binding row = %+v", row)
	}
	if !reflect.DeepEqual(row["groups_id"], []int64{ids["base.group_demo"].ResID}) {
		t.Fatalf("report groups = %#v", row["groups_id"])
	}
}

func TestLoadXMLPersistsIrModelData(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <data noupdate="1">
    <record id="res_partner_model_data" model="res.partner">
      <field name="name">Model Data</field>
    </record>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	external := loader.ExternalIDs()["base.res_partner_model_data"]
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", "=", "base"),
		domain.Cond("name", "=", "res_partner_model_data"),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("module", "name", "complete_name", "model", "res_id", "noupdate")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("ir.model.data rows = %d", len(rows))
	}
	row := rows[0]
	if row["module"] != "base" || row["name"] != "res_partner_model_data" || row["complete_name"] != "base.res_partner_model_data" || row["model"] != "res.partner" || row["res_id"] != external.ResID || row["noupdate"] != true {
		t.Fatalf("ir.model.data row = %+v, external = %+v", row, external)
	}
}

func TestLoadXMLUpdateXMLIDsUpsertsModelData(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_a" model="res.partner">
    <field name="name">Partner A</field>
  </record>
  <record id="partner_b" model="res.partner">
    <field name="name">Partner B</field>
  </record>
  <function model="ir.model.data" name="_update_xmlids">
    <value model="res.partner" eval="[{'xml_id': 'base.partner_alias', 'record': obj().env.ref('partner_a'), 'noupdate': True}]"/>
  </function>
  <function model="ir.model.data" name="_update_xmlids">
    <value model="res.partner" eval="[{'xml_id': 'base.partner_alias', 'record': obj().env.ref('partner_b'), 'noupdate': False}]"/>
  </function>
  <function model="ir.model.data" name="_update_xmlids">
    <value model="res.partner" eval="[{'xml_id': 'base.partner_alias', 'record': obj().env.ref('partner_a'), 'noupdate': False}]"/>
    <value eval="True"/>
  </function>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	partnerBID := loader.ExternalIDs()["base.partner_b"].ResID
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", "=", "base"),
		domain.Cond("name", "=", "partner_alias"),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("module", "name", "complete_name", "model", "res_id", "noupdate")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("alias rows = %+v", rows)
	}
	row := rows[0]
	if row["complete_name"] != "base.partner_alias" || row["model"] != "res.partner" || row["res_id"] != partnerBID || row["noupdate"] != true {
		t.Fatalf("alias row = %+v", row)
	}
	external := loader.ExternalIDs()["base.partner_alias"]
	if external.Model != "res.partner" || external.ResID != partnerBID || external.Noupdate != true {
		t.Fatalf("alias external id = %+v", external)
	}
}

func TestLoadXMLRefFallsBackToPersistedIrModelData(t *testing.T) {
	env := testBaseEnv(t)
	baseLoader := NewLoader(env, "base")
	err := baseLoader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	otherLoader := NewLoader(env, "other")
	err = otherLoader.LoadXML(strings.NewReader(`<odoo>
  <record id="user_other" model="res.users">
    <field name="name">Other User</field>
    <field name="company_id" ref="base.company_main"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	userID := otherLoader.ExternalIDs()["other.user_other"].ResID
	assertField(t, env, "res.users", userID, "company_id", baseLoader.ExternalIDs()["base.company_main"].ResID)
}

func TestLoadXMLEvalRefFalseFallback(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="user_missing_positional" model="res.users">
    <field name="name">Missing Positional</field>
    <field name="company_id" eval="ref('missing_company', False)"/>
  </record>
  <record id="user_missing_keyword" model="res.users">
    <field name="name">Missing Keyword</field>
    <field name="company_id" eval="ref('missing_company', raise_if_not_found=False)"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.users", ids["base.user_missing_positional"].ResID, "company_id", false)
	assertField(t, env, "res.users", ids["base.user_missing_keyword"].ResID, "company_id", false)
}

func TestLoadXMLEvalDateTimeAndArithmetic(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_eval_time" model="res.partner">
    <field name="name" eval="datetime(1994, 4, 7).date()"/>
    <field name="age" eval="datetime(2024, 1, 3).weekday()"/>
    <field name="score" eval="5/3600"/>
    <field name="html_body" eval="(datetime(2024, 1, 8) - timedelta(days=datetime(2024, 1, 3).weekday() + 1)).date()"/>
    <field name="file_ref" eval="(datetime(2024, 1, 8) - timedelta(days=datetime(2024, 1, 3).weekday() - 1)).date()"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	partnerID := loader.ExternalIDs()["base.res_partner_eval_time"].ResID
	assertField(t, env, "res.partner", partnerID, "name", "1994-04-07")
	assertField(t, env, "res.partner", partnerID, "age", int64(2))
	assertField(t, env, "res.partner", partnerID, "score", float64(5)/float64(3600))
	assertField(t, env, "res.partner", partnerID, "html_body", "2024-01-05")
	assertField(t, env, "res.partner", partnerID, "file_ref", "2024-01-07")
}

func TestLoadXMLEvalRelativeDeltaWeeksAndWeekday(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_relative_date" model="res.partner">
    <field name="name" eval="(datetime(2024, 1, 2) + relativedelta(weeks=1, weekday=4)).date()"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	id := loader.ExternalIDs()["base.partner_relative_date"].ResID
	assertField(t, env, "res.partner", id, "name", "2024-01-12")

	err = loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_bad_delta" model="res.partner">
    <field name="name" eval="datetime(2024, 1, 2) + relativedelta(unknown=1)"/>
  </record>
</odoo>`))
	if err == nil || !strings.Contains(err.Error(), "unsupported delta argument unknown") {
		t.Fatalf("expected unsupported delta argument error, got %v", err)
	}
}

func TestLoadXMLEvalDateTimeStrftime(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="cron_eval_time" model="ir.cron">
    <field name="name">Cron Eval Time</field>
    <field name="nextcall" eval="(DateTime.now() + timedelta(days=1)).strftime('%Y-%m-%d 22:00:00')"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	cronID := loader.ExternalIDs()["base.cron_eval_time"].ResID
	rows, err := env.Model("ir.cron").Browse(cronID).Read("nextcall")
	if err != nil {
		t.Fatal(err)
	}
	nextcall := rows[0]["nextcall"].(string)
	if len(nextcall) != len("2006-01-02 22:00:00") || !strings.HasSuffix(nextcall, "22:00:00") {
		t.Fatalf("nextcall = %q", nextcall)
	}
}

func TestLoadXMLOdooCronCreatesDelegatedServerAction(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Partner</field>
  </record>
  <record id="cron_partner_sync" model="ir.cron">
    <field name="name">Partner Sync</field>
    <field name="model_id" ref="model_res_partner"/>
    <field name="state">code</field>
    <field name="code">model.sync_partners()</field>
    <field name="active" eval="True"/>
    <field name="interval_number" eval="1"/>
    <field name="interval_type">days</field>
    <field name="priority" eval="3"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	cronID := loader.ExternalIDs()["base.cron_partner_sync"].ResID
	rows, err := env.Model("ir.cron").Browse(cronID).Read("ir_actions_server_id", "cron_name", "priority")
	if err != nil {
		t.Fatal(err)
	}
	actionID, _ := int64Value(rows[0]["ir_actions_server_id"])
	if actionID == 0 || rows[0]["cron_name"] != "Partner Sync" || rows[0]["priority"] != int64(3) {
		t.Fatalf("cron row = %+v", rows[0])
	}
	actionRows, err := env.Model("ir.actions.server").Browse(actionID).Read("name", "model_id", "model_name", "state", "code", "usage")
	if err != nil {
		t.Fatal(err)
	}
	if len(actionRows) != 1 || actionRows[0]["name"] != "Partner Sync" || actionRows[0]["model_name"] != "res.partner" || actionRows[0]["state"] != "code" || actionRows[0]["code"] != "model.sync_partners()" || actionRows[0]["usage"] != "ir_cron" {
		t.Fatalf("action row = %+v", actionRows)
	}
}

func TestLoadXMLCronProgressOdooFields(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="cron_progress" model="ir.cron.progress">
    <field name="cron_id" eval="42"/>
    <field name="done" eval="2"/>
    <field name="remaining" eval="3"/>
    <field name="deactivate" eval="True"/>
    <field name="timed_out_counter" eval="1"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	progressID := loader.ExternalIDs()["base.cron_progress"].ResID
	rows, err := env.Model("ir.cron.progress").Browse(progressID).Read("cron_id", "done", "remaining", "deactivate", "timed_out_counter")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["cron_id"] != int64(42) || rows[0]["done"] != int64(2) || rows[0]["remaining"] != int64(3) || rows[0]["deactivate"] != true || rows[0]["timed_out_counter"] != int64(1) {
		t.Fatalf("progress row = %+v", rows[0])
	}
}

func TestLoadCSVOdooRefsAndBooleans(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Partner</field>
  </record>
  <record id="group_user" model="res.groups">
    <field name="name">User</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadCSV("ir.model.access", strings.NewReader(`id,name,model_id:id,group_id:id,perm_read,perm_write,perm_create,perm_unlink
access_partner_user,Partner user,model_res_partner,group_user,1,0,True,False
`))
	if err != nil {
		t.Fatal(err)
	}

	accessID := loader.ExternalIDs()["base.access_partner_user"].ResID
	modelID := loader.ExternalIDs()["base.model_res_partner"].ResID
	groupID := loader.ExternalIDs()["base.group_user"].ResID
	rows, err := env.Model("ir.model.access").Browse(accessID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["model_id"] != modelID || row["group_id"] != groupID {
		t.Fatalf("unexpected refs: %+v", row)
	}
	if row["perm_read"] != true || row["perm_write"] != false || row["perm_create"] != true || row["perm_unlink"] != false {
		t.Fatalf("unexpected permissions: %+v", row)
	}
}

func TestLoadXMLDuplicateFieldUsesLastValueForAccess(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_demo" model="ir.model">
    <field name="model">x.demo</field>
    <field name="name">Demo</field>
  </record>
  <record id="group_manager" model="res.groups">
    <field name="name">Manager</field>
  </record>
  <record id="group_admin" model="res.groups">
    <field name="name">Administrator</field>
  </record>
  <record id="access_demo_manager" model="ir.model.access">
    <field name="name">Demo manager</field>
    <field name="model_id" ref="model_demo"/>
    <field name="group_id" ref="group_manager"/>
    <field name="perm_read" eval="True"/>
    <field name="perm_write" eval="True"/>
    <field name="perm_write" eval="False"/>
    <field name="perm_create" eval="True"/>
    <field name="perm_unlink" eval="False"/>
  </record>
  <record id="access_demo_admin" model="ir.model.access">
    <field name="name">Demo admin</field>
    <field name="model_id" ref="model_demo"/>
    <field name="group_id" ref="group_admin"/>
    <field name="perm_read" eval="True"/>
    <field name="perm_write" eval="False"/>
    <field name="perm_write" eval="True"/>
    <field name="perm_create" eval="True"/>
    <field name="perm_unlink" eval="True"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	modelID := ids["base.model_demo"].ResID
	managerGroupID := ids["base.group_manager"].ResID
	adminGroupID := ids["base.group_admin"].ResID
	managerAccessID := ids["base.access_demo_manager"].ResID
	adminAccessID := ids["base.access_demo_admin"].ResID
	if modelID == 0 || managerGroupID == 0 || adminGroupID == 0 || managerAccessID == 0 || adminAccessID == 0 {
		t.Fatalf("external ids = %+v", ids)
	}
	assertModelCount(t, env, "ir.model.access", 2)
	assertLoadedAccessPerms(t, env, managerAccessID, modelID, managerGroupID, true, false, true, false)
	assertLoadedAccessPerms(t, env, adminAccessID, modelID, adminGroupID, true, true, true, true)
}

func TestLoadCrossModuleRefsWithSharedExternalIDs(t *testing.T) {
	env := testBaseEnv(t)
	externalIDs := map[string]ExternalID{}
	baseLoader := NewLoaderWithExternalIDs(env, "base", externalIDs)
	workflowLoader := NewLoaderWithExternalIDs(env, "oi_workflow", externalIDs)

	err := baseLoader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Partner</field>
  </record>
  <record id="group_user" model="res.groups">
    <field name="name">Internal User</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	err = workflowLoader.LoadCSV("ir.model.access", strings.NewReader(`id,name,model_id:id,group_id:id,perm_read,perm_write,perm_create,perm_unlink
access_partner_user,Partner user,base.model_res_partner,base.group_user,1,0,0,0
`))
	if err != nil {
		t.Fatal(err)
	}
	accessID := workflowLoader.ExternalIDs()["oi_workflow.access_partner_user"].ResID
	rows, err := env.Model("ir.model.access").Browse(accessID).Read("model_id", "group_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["model_id"] != externalIDs["base.model_res_partner"].ResID || rows[0]["group_id"] != externalIDs["base.group_user"].ResID {
		t.Fatalf("unexpected cross-module refs: %+v", rows[0])
	}
}

func TestLoadXMLRootNoupdate(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo noupdate="1">
  <record id="res_partner_noupdate" model="res.partner">
    <field name="name">Noupdate</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	if !loader.ExternalIDs()["base.res_partner_noupdate"].Noupdate {
		t.Fatal("expected root noupdate external id")
	}
}

func TestLoadXMLNoupdateExternalIDIsNotOverwritten(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <data noupdate="1">
    <record id="res_partner_noupdate" model="res.partner">
      <field name="name">Original</field>
    </record>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	firstID := loader.ExternalIDs()["base.res_partner_noupdate"].ResID

	err = loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_noupdate" model="res.partner">
    <field name="name">Changed</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	secondID := loader.ExternalIDs()["base.res_partner_noupdate"].ResID
	if secondID != firstID {
		t.Fatalf("expected same res id, got %d then %d", firstID, secondID)
	}
	assertField(t, env, "res.partner", firstID, "name", "Original")
	assertModelCount(t, env, "res.partner", 1)
}

func TestLoadXMLLaterNoupdateMarksExistingExternalID(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_later_noupdate" model="res.partner">
    <field name="name">Original</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	firstID := loader.ExternalIDs()["base.res_partner_later_noupdate"].ResID

	err = loader.LoadXML(strings.NewReader(`<odoo noupdate="1">
  <record id="res_partner_later_noupdate" model="res.partner">
    <field name="name">Skipped</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	external := loader.ExternalIDs()["base.res_partner_later_noupdate"]
	if external.ResID != firstID || !external.Noupdate {
		t.Fatalf("external id = %+v", external)
	}
	assertField(t, env, "res.partner", firstID, "name", "Original")
}

func TestLoadXMLForceCreateFalseSkipsMissingExternalID(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_skip" model="res.partner" forcecreate="False">
    <field name="name">Skipped</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loader.ExternalIDs()["base.res_partner_skip"]; ok {
		t.Fatal("unexpected external id for forcecreate=False missing record")
	}
	assertModelCount(t, env, "res.partner", 0)
}

func TestLoadXMLDeleteByIDAndSearch(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="delete_by_id" model="res.partner">
    <field name="name">Delete By ID</field>
    <field name="active" eval="True"/>
  </record>
  <record id="delete_by_search" model="res.partner">
    <field name="name">Delete By Search</field>
    <field name="active" eval="False"/>
  </record>
  <record id="keep_partner" model="res.partner">
    <field name="name">Keep</field>
    <field name="active" eval="True"/>
  </record>
  <delete model="res.partner" id="delete_by_id"/>
  <delete model="res.partner" search="[('active', '=', False)]"/>
  <delete model="res.partner" id="missing_external_id"/>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	assertModelCount(t, env, "res.partner", 1)
	assertField(t, env, "res.partner", loader.ExternalIDs()["base.keep_partner"].ResID, "name", "Keep")
}

func TestLoadXMLFunctionSetDefaultWriteAndUnlink(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_write" model="res.partner">
    <field name="name">Before</field>
    <field name="active" eval="True"/>
  </record>
  <record id="partner_unlink" model="res.partner">
    <field name="name">Remove</field>
    <field name="active" eval="False"/>
  </record>
  <function model="ir.default" name="set" eval="('res.partner', 'trust', 'normal')"/>
  <function model="ir.config_parameter" name="set_param" eval="('auth_signup.reset_password', True)"/>
  <function model="ir.config_parameter" name="set_param" eval="('mail.catchall.domain.allowed', 'HELLO.com,, BONJOUR.com')"/>
  <function model="res.partner" name="write">
    <value model="res.partner" search="[('name', '=', 'Before')]"/>
    <value eval="{'name': 'After'}"/>
  </function>
  <function model="res.partner" name="search" eval="[('name', '=', 'After')]"/>
  <function model="res.partner" name="unlink">
    <value eval="[ref('partner_unlink')]"/>
  </function>
  <function model="discuss.channel" name="_add_members">
    <value eval="[ref('partner_write')]"/>
    <value name="partners" eval="obj().env['res.partner'].browse(ref('partner_write'))"/>
  </function>
  <function model="mrp.workorder" name="_create_checks">
    <function eval="[[('state', 'not in', ('cancel', 'done'))]]" model="mrp.workorder" name="search"/>
  </function>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.partner", ids["base.partner_write"].ResID, "name", "After")
	assertModelCount(t, env, "res.partner", 1)
	found, err := env.Model("ir.default").Search(domain.And(domain.Cond("model", "=", "res.partner"), domain.Cond("field", "=", "trust")))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "normal" {
		t.Fatalf("ir.default rows = %+v", rows)
	}
	found, err = env.Model("ir.config_parameter").Search(domain.Cond("key", "=", "auth_signup.reset_password"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "true" {
		t.Fatalf("ir.config_parameter rows = %+v", rows)
	}
	found, err = env.Model("ir.config_parameter").Search(domain.Cond("key", "=", "mail.catchall.domain.allowed"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "hello.com,bonjour.com" {
		t.Fatalf("mail catchall ir.config_parameter rows = %+v", rows)
	}
}

func TestLoadXMLSafeEvalObjEnvRecordsets(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
  <record id="user_admin" model="res.users">
    <field name="name">Admin</field>
    <field name="company_id" eval="obj().env.ref('company_main').id"/>
  </record>
  <record id="partner_active" model="res.partner">
    <field name="name">Before Active</field>
    <field name="active" eval="True"/>
  </record>
  <record id="partner_browse" model="res.partner">
    <field name="name">Before Browse</field>
    <field name="active" eval="False"/>
  </record>
  <record id="partner_concat" model="res.partner">
    <field name="name" eval="'Hello ' + 'World'"/>
  </record>
  <record id="partner_time" model="res.partner">
    <field name="name" eval="'Batch for ' + time.strftime('%Y')"/>
  </record>
  <record id="partner_replace" model="res.partner">
    <field name="name" eval="'Salary Slip - ' + DateTime.today().replace(day=1).strftime('%B %Y')"/>
  </record>
  <function model="res.partner" name="write">
    <value eval="obj().env['res.partner'].search([('active', '=', True)]).ids"/>
    <value eval="{'name': 'Search Updated'}"/>
  </function>
  <function model="res.partner" name="write">
    <value model="res.partner" eval="obj().browse(ref('partner_browse'))"/>
    <value eval="{'name': 'Browse Updated'}"/>
  </function>
  <function model="res.partner" name="write" eval="[[p.id for p in [obj().env.ref('partner_browse')]], {'active': True}]"/>
  <record id="partner_company" model="res.partner">
    <field name="name">Company Holder</field>
    <field name="model_search_id" eval="obj().env.ref('user_admin').company_id.id"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.users", ids["base.user_admin"].ResID, "company_id", ids["base.company_main"].ResID)
	assertField(t, env, "res.partner", ids["base.partner_active"].ResID, "name", "Search Updated")
	assertField(t, env, "res.partner", ids["base.partner_browse"].ResID, "name", "Browse Updated")
	assertField(t, env, "res.partner", ids["base.partner_browse"].ResID, "active", true)
	assertField(t, env, "res.partner", ids["base.partner_concat"].ResID, "name", "Hello World")
	assertField(t, env, "res.partner", ids["base.partner_time"].ResID, "name", "Batch for "+time.Now().UTC().Format("2006"))
	assertField(t, env, "res.partner", ids["base.partner_replace"].ResID, "name", "Salary Slip - "+time.Now().UTC().Format("January 2006"))
	assertField(t, env, "res.partner", ids["base.partner_company"].ResID, "model_search_id", ids["base.company_main"].ResID)
}

func TestLoadXMLSafeEvalObjRecordStringMethodsAndIndexing(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_source" model="res.partner">
    <field name="name">Ionel Popescu</field>
  </record>
  <record id="partner_login" model="res.partner">
    <field name="name" model="res.partner" eval="obj(ref('partner_source')).name.replace(' ', '.').lower() + '@example.com'"/>
  </record>
  <record id="partner_signature" model="res.partner">
    <field name="name" model="res.partner" eval="obj(ref('partner_source')).name.split(' ')[0][0] + '. ' + obj(ref('partner_source')).name.split(' ')[-1]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.partner", ids["base.partner_login"].ResID, "name", "ionel.popescu@example.com")
	assertField(t, env, "res.partner", ids["base.partner_signature"].ResID, "name", "I. Popescu")
}

func TestLoadXMLSafeEvalBooleanFallback(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Contact</field>
  </record>
  <function model="ir.default" name="set">
    <value eval="'res.partner'"/>
    <value eval="'present'"/>
    <value eval="obj()._get_id('res.partner') and 'has-model' or 'missing'"/>
  </function>
  <function model="ir.default" name="set">
    <value eval="'res.partner'"/>
    <value eval="'absent'"/>
    <value eval="obj()._get_id('missing.model') and 'has-model' or 'missing'"/>
  </function>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	found, err := env.Model("ir.default").Search(domain.Cond("model", "=", "res.partner"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("field", "value")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	for _, row := range rows {
		values[row["field"].(string)] = row["value"]
	}
	if values["present"] != "has-model" || values["absent"] != "missing" {
		t.Fatalf("unexpected defaults: %+v", values)
	}
}

func TestLoadXMLSafeEvalListComprehensionX2ManyCommands(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_a" model="res.groups">
    <field name="name">A</field>
  </record>
  <record id="group_b" model="res.groups">
    <field name="name">B</field>
  </record>
  <record id="group_holder" model="res.groups">
    <field name="name">Holder</field>
    <field name="implied_ids" eval="[(4, ref('group_a')), (4, ref('group_b'))]"/>
  </record>
  <record id="group_sink" model="res.groups">
    <field name="name">Sink</field>
    <field name="implied_ids" eval="[(5, 0, 0)] + [(4, p) for p in obj().env.ref('group_holder').implied_ids.ids]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.groups", ids["base.group_sink"].ResID, "implied_ids", []int64{
		ids["base.group_a"].ResID,
		ids["base.group_b"].ResID,
	})
}

func TestLoadXMLSafeEvalTernaryUserHasGroup(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_project_milestone" model="res.groups">
    <field name="name">Milestone Users</field>
  </record>
  <record id="user_demo" model="res.users">
    <field name="login">demo</field>
    <field name="name">Demo</field>
    <field name="groups_id" eval="[Command.link(ref('group_project_milestone'))]"/>
  </record>
  <record id="partner_milestone" model="res.partner">
    <field name="name" eval="'milestones' if obj().env.user.has_group('group_project_milestone') else 'manual'"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	ids := loader.ExternalIDs()
	env = env.WithContext(record.Context{UserID: ids["base.user_demo"].ResID})
	loader = NewLoaderWithExternalIDs(env, "base", ids)
	err = loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_milestone" model="res.partner">
    <field name="name" eval="'milestones' if obj().env.user.has_group('group_project_milestone') else 'manual'"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	assertField(t, env, "res.partner", ids["base.partner_milestone"].ResID, "name", "milestones")
}

func TestLoadXMLSafeEvalBuiltinsDatePartsFloorDivisionAndFormatting(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
  <record id="user_admin" model="res.users">
    <field name="name">Admin</field>
  </record>
  <record id="partner_budget" model="res.partner">
    <field name="name" eval="'Budget Optimistic '+str(datetime.now().year)"/>
    <field name="age" eval="(DateTime.today() + relativedelta(years=-1, month=5, day=1)).month // 4 + 1"/>
    <field name="html_body" eval="str([('all_group_ids', 'in', [ref('company_main')])])"/>
    <field name="file_ref" eval="'/web/image/res.users/%s/image_128' % ref('user_admin')"/>
    <field name="literal_dict" eval="dict({'a': 1}, **{'b': str(datetime.now().year)})"/>
  </record>
  <record id="partner_contract" model="res.partner">
    <field name="file_ref" eval="DateTime(DateTime.today().year-3,month=2,day=25).date()"/>
  </record>
  <record id="partner_fstring" model="res.partner">
    <field name="name" eval="f'Budget {datetime.now().year}: Home Construction'"/>
    <field name="file_ref" eval="f'{datetime.now().year + 1}-01-01'"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	year := time.Now().UTC().Format("2006")
	assertField(t, env, "res.partner", ids["base.partner_budget"].ResID, "name", "Budget Optimistic "+year)
	assertField(t, env, "res.partner", ids["base.partner_budget"].ResID, "age", int64(2))
	assertField(t, env, "res.partner", ids["base.partner_budget"].ResID, "html_body", "[('all_group_ids', 'in', ["+strconv.FormatInt(ids["base.company_main"].ResID, 10)+"])]")
	assertField(t, env, "res.partner", ids["base.partner_budget"].ResID, "file_ref", "/web/image/res.users/"+strconv.FormatInt(ids["base.user_admin"].ResID, 10)+"/image_128")
	assertField(t, env, "res.partner", ids["base.partner_budget"].ResID, "literal_dict", map[string]any{"a": int64(1), "b": year})
	assertField(t, env, "res.partner", ids["base.partner_contract"].ResID, "file_ref", strconv.Itoa(time.Now().UTC().Year()-3)+"-02-25")
	assertField(t, env, "res.partner", ids["base.partner_fstring"].ResID, "name", "Budget "+year+": Home Construction")
	assertField(t, env, "res.partner", ids["base.partner_fstring"].ResID, "file_ref", strconv.Itoa(time.Now().UTC().Year()+1)+"-01-01")
}

func TestLoadXMLSafeEvalEnvCompanyAndUserFields(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
    <field name="extra_hour" eval="3"/>
    <field name="partnership_label">Partners</field>
  </record>
  <record id="user_admin" model="res.users">
    <field name="name">Admin</field>
    <field name="company_id" ref="company_main"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	ids := loader.ExternalIDs()
	env = env.WithContext(record.Context{
		UserID:     ids["base.user_admin"].ResID,
		CompanyID:  ids["base.company_main"].ResID,
		CompanyIDs: []int64{ids["base.company_main"].ResID},
	})
	loader = NewLoaderWithExternalIDs(env, "base", ids)

	err = loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_env" model="res.partner">
    <field name="name" model="res.users" eval="obj().env.user.company_id.partnership_label"/>
    <field name="age" model="res.company" eval="obj().env.company.extra_hour"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	assertField(t, env, "res.partner", ids["base.partner_env"].ResID, "name", "Partners")
	assertField(t, env, "res.partner", ids["base.partner_env"].ResID, "age", int64(3))
}

func TestLoadXMLSafeEvalRecordsetContextMappedFilteredAndCreate(t *testing.T) {
	env := testSafeEvalRecordsetEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="line_empty" model="x.move.line">
    <field name="name">Line A</field>
  </record>
  <record id="line_lot" model="x.move.line">
    <field name="name">Line B</field>
    <field name="lot_name">LOT-1</field>
  </record>
  <record id="move_one" model="x.move">
    <field name="name">Move</field>
    <field name="move_line_ids" eval="[(6, 0, [ref('line_empty'), ref('line_lot')])]"/>
  </record>
  <record id="partner_recordset" model="res.partner">
    <field name="name" model="x.move" eval="'Return of %s' % obj().env.ref('move_one').move_line_ids.mapped('name')[:1]"/>
    <field name="age" model="x.move" eval="obj().env.ref('move_one').with_context(active_test=False).move_line_ids.filtered(lambda line: not line.lot_name).id"/>
  </record>
  <record id="partner_created" model="res.partner">
    <field name="model_search_id" model="x.created" eval="obj().env['x.created'].sudo().create(dict(obj().default_get(list(obj().fields_get())), **{'name': 'Generated'})).id"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.partner", ids["base.partner_recordset"].ResID, "name", "Return of ['Line A']")
	assertField(t, env, "res.partner", ids["base.partner_recordset"].ResID, "age", ids["base.line_empty"].ResID)
	createdID := fieldValue(t, env, "res.partner", ids["base.partner_created"].ResID, "model_search_id")
	assertField(t, env, "x.created", createdID.(int64), "name", "Generated")
}

func TestLoadXMLSafeEvalMultiGeneratorComprehensionRangeMapJoinAndFormat(t *testing.T) {
	env := testCommandEnv(t)
	loader := NewLoader(env, "x")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="tag_a" model="x.tag">
    <field name="name">A</field>
  </record>
  <record id="tag_b" model="x.tag">
    <field name="name">B</field>
  </record>
  <record id="parent_source_eval" model="x.parent">
    <field name="name" eval="','.join(['false', str(ref('tag_a')), str(ref('tag_b'))])"/>
    <field name="child_ids" eval="[(5, 0, 0)] + [
      (0, 0, {'name': 'weekday ' + weekday + ' hour ' + str(start_hour)})
      for weekday in list(map(str, range(1, 3)))
      for start_hour in (18, 18.50)
    ]"/>
  </record>
  <record id="parent_format" model="x.parent">
    <field name="name">Formatted</field>
    <field name="child_ids" eval="[Command.create({'name': 'x.{}_demo'.format(ref('tag_a'))})]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "x.parent", ids["x.parent_source_eval"].ResID, "name", "false,"+strconv.FormatInt(ids["x.tag_a"].ResID, 10)+","+strconv.FormatInt(ids["x.tag_b"].ResID, 10))
	childIDs := fieldValue(t, env, "x.parent", ids["x.parent_source_eval"].ResID, "child_ids").([]int64)
	if len(childIDs) != 4 {
		t.Fatalf("child count = %d, ids = %#v", len(childIDs), childIDs)
	}
	assertField(t, env, "x.child", childIDs[0], "name", "weekday 1 hour 18")
	assertField(t, env, "x.child", childIDs[1], "name", "weekday 1 hour 18.5")
	assertField(t, env, "x.child", childIDs[2], "name", "weekday 2 hour 18")
	assertField(t, env, "x.child", childIDs[3], "name", "weekday 2 hour 18.5")
	formattedChildIDs := fieldValue(t, env, "x.parent", ids["x.parent_format"].ResID, "child_ids").([]int64)
	if len(formattedChildIDs) != 1 {
		t.Fatalf("formatted child ids = %#v", formattedChildIDs)
	}
	assertField(t, env, "x.child", formattedChildIDs[0], "name", "x."+strconv.FormatInt(ids["x.tag_a"].ResID, 10)+"_demo")
}

func TestLoadXMLSafeEvalConditionalListComprehension(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_a" model="res.groups">
    <field name="name">A</field>
  </record>
  <record id="group_b" model="res.groups">
    <field name="name">B</field>
  </record>
  <record id="group_c" model="res.groups">
    <field name="name">C</field>
    <field name="implied_ids" eval="[(4, p) for p in [ref('group_a'), ref('group_b')] if p not in [ref('group_a')]]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	ids := loader.ExternalIDs()
	assertField(t, env, "res.groups", ids["base.group_c"].ResID, "implied_ids", []int64{ids["base.group_b"].ResID})
}

func TestLoadXMLSafeEvalEmbeddedListExpressions(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_a" model="res.groups">
    <field name="name">A</field>
  </record>
  <record id="group_b" model="res.groups">
    <field name="name">B</field>
  </record>
  <record id="group_c" model="res.groups">
    <field name="name">C</field>
    <field name="implied_ids" eval="[
      (4, ref('group_a') if True else ref('group_b')),
      (4, ref('group_b') and ref('group_b') or ref('group_a'))
    ]"/>
  </record>
  <record id="group_d" model="res.groups">
    <field name="name">D</field>
    <field name="implied_ids" eval="[Command.set([ref('group_a') if False else ref('group_b')])]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	ids := loader.ExternalIDs()
	assertField(t, env, "res.groups", ids["base.group_c"].ResID, "implied_ids", []int64{ids["base.group_a"].ResID, ids["base.group_b"].ResID})
	assertField(t, env, "res.groups", ids["base.group_d"].ResID, "implied_ids", []int64{ids["base.group_b"].ResID})
}

func TestLoadXMLProcessesItemsInDocumentOrder(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="partner_ordered" model="res.partner">
    <field name="name">Original</field>
  </record>
  <function model="res.partner" name="write">
    <value model="res.partner" search="[('name', '=', 'Original')]"/>
    <value eval="{'name': 'Function Update'}"/>
  </function>
  <record id="partner_ordered" model="res.partner">
    <field name="name">Record Update</field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	assertField(t, env, "res.partner", loader.ExternalIDs()["base.partner_ordered"].ResID, "name", "Record Update")
}

func TestLoadXMLRootAndGroupNoupdateSkipRepeatedLoads(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo noupdate="1">
  <record id="root_partner" model="res.partner">
    <field name="name">Root Original</field>
  </record>
  <data>
    <record id="root_group_partner" model="res.partner">
      <field name="name">Root Group Original</field>
    </record>
  </data>
  <data noupdate="1">
    <record id="group_partner" model="res.partner">
      <field name="name">Group Original</field>
    </record>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}
	ids := loader.ExternalIDs()

	err = loader.LoadXML(strings.NewReader(`<odoo noupdate="1">
  <record id="root_partner" model="res.partner">
    <field name="name">Root Changed</field>
  </record>
  <data>
    <record id="root_group_partner" model="res.partner">
      <field name="name">Root Group Changed</field>
    </record>
  </data>
  <data noupdate="1">
    <record id="group_partner" model="res.partner">
      <field name="name">Group Changed</field>
    </record>
  </data>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	assertField(t, env, "res.partner", ids["base.root_partner"].ResID, "name", "Root Original")
	assertField(t, env, "res.partner", ids["base.root_group_partner"].ResID, "name", "Root Group Original")
	assertField(t, env, "res.partner", ids["base.group_partner"].ResID, "name", "Group Original")
	assertModelCount(t, env, "res.partner", 3)
	for _, key := range []string{"base.root_partner", "base.root_group_partner", "base.group_partner"} {
		if !loader.ExternalIDs()[key].Noupdate {
			t.Fatalf("expected %s to remain noupdate", key)
		}
	}
}

func TestLoadXMLEvalScalarsAndCollections(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_eval" model="res.partner">
    <field name="name" eval="'Administrator'"/>
    <field name="active" eval="True"/>
    <field name="age" eval="42"/>
    <field name="score" eval="3.5"/>
    <field name="literal_list" eval="[1, 'two', False]"/>
    <field name="literal_dict" eval="{'key': 'value', 'count': 2}"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	partnerID := loader.ExternalIDs()["base.res_partner_eval"].ResID
	rows, err := env.Model("res.partner").Browse(partnerID).Read("name", "active", "age", "score", "literal_list", "literal_dict")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["name"] != "Administrator" || row["active"] != true || row["age"] != int64(42) || row["score"] != 3.5 {
		t.Fatalf("unexpected scalar values: %+v", row)
	}
	if !reflect.DeepEqual(row["literal_list"], []any{int64(1), "two", false}) {
		t.Fatalf("unexpected list: %#v", row["literal_list"])
	}
	wantDict := map[string]any{"key": "value", "count": int64(2)}
	if !reflect.DeepEqual(row["literal_dict"], wantDict) {
		t.Fatalf("unexpected dict: %#v", row["literal_dict"])
	}
}

func TestLoadXMLStructuredFieldTypes(t *testing.T) {
	env := testEnv(t)
	moduleDir := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "doc.txt"), []byte("document"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(env, "base")
	loader.SetBaseDir(moduleDir)

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
  <record id="res_partner_structured" model="res.partner">
    <field name="name">Structured</field>
    <field name="html_body" type="html">
      <p class="o_view_nocontent_smiling_face">Hello <b>World</b></p>
    </field>
    <field name="age" type="int">7</field>
    <field name="score" type="float">9.5</field>
    <field name="file_ref" type="file">doc.txt</field>
    <field name="literal_list" type="list">
      <value eval="ref('company_main')"/>
      <value type="int">42</value>
      <value type="tuple">
        <value>nested</value>
        <value eval="True"/>
      </value>
    </field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	partnerID := ids["base.res_partner_structured"].ResID
	rows, err := env.Model("res.partner").Browse(partnerID).Read("html_body", "age", "score", "file_ref", "literal_list")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if !strings.Contains(row["html_body"].(string), `<p class="o_view_nocontent_smiling_face">`) || !strings.Contains(row["html_body"].(string), "<b>World</b>") {
		t.Fatalf("html_body = %s", row["html_body"])
	}
	if row["age"] != int64(7) || row["score"] != 9.5 || row["file_ref"] != "base,doc.txt" {
		t.Fatalf("typed fields = %+v", row)
	}
	wantList := []any{ids["base.company_main"].ResID, int64(42), evalTuple{"nested", true}}
	if !reflect.DeepEqual(row["literal_list"], wantList) {
		t.Fatalf("literal_list = %#v, want %#v", row["literal_list"], wantList)
	}
}

func TestLoadXMLSearchAndX2ManyCommands(t *testing.T) {
	env := testBaseEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="model_res_partner" model="ir.model">
    <field name="model">res.partner</field>
    <field name="name">Partner</field>
  </record>
  <record id="group_a" model="res.groups">
    <field name="name">Group A</field>
  </record>
  <record id="group_b" model="res.groups">
    <field name="name">Group B</field>
  </record>
  <record id="group_link" model="res.groups">
    <field name="name">Group Link</field>
    <field name="implied_ids" eval="[(4, ref('group_a'))]"/>
  </record>
  <record id="group_set" model="res.groups">
    <field name="name">Group Set</field>
    <field name="implied_ids" eval="[(6, 0, [ref('group_a'), ref('base.group_b')])]"/>
  </record>
  <record id="group_empty" model="res.groups">
    <field name="name">Group Empty</field>
    <field name="implied_ids" eval="[]"/>
  </record>
  <record id="group_command_link" model="res.groups">
    <field name="name">Group Command Link</field>
    <field name="implied_ids" eval="[Command.link(ref('group_a'))]"/>
  </record>
  <record id="group_command_set" model="res.groups">
    <field name="name">Group Command Set</field>
    <field name="implied_ids" eval="[Command.set([ref('group_a'), ref('base.group_b')])]"/>
  </record>
  <record id="group_command_clear" model="res.groups">
    <field name="name">Group Command Clear</field>
    <field name="implied_ids" eval="[Command.link(ref('group_a')), Command.clear()]"/>
  </record>
  <record id="group_command_unlink" model="res.groups">
    <field name="name">Group Command Unlink</field>
    <field name="implied_ids" eval="[Command.link(ref('group_a')), Command.link(ref('group_b')), Command.unlink(ref('group_a'))]"/>
  </record>
  <record id="group_command_delete" model="res.groups">
    <field name="name">Group Command Delete</field>
    <field name="implied_ids" eval="[Command.link(ref('group_a')), Command.delete(ref('group_a'))]"/>
  </record>
  <record id="rule_partner" model="ir.rule">
    <field name="name">Partner rule</field>
    <field name="model_id" search="[('model', '=', 'res.partner')]"/>
    <field name="domain_force" eval="[('active', '=', True)]"/>
    <field name="groups" eval="[(4, ref('group_a'))]"/>
    <field name="global" eval="False"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	groupA := ids["base.group_a"].ResID
	groupB := ids["base.group_b"].ResID
	assertField(t, env, "res.groups", ids["base.group_link"].ResID, "implied_ids", []int64{groupA})
	assertField(t, env, "res.groups", ids["base.group_set"].ResID, "implied_ids", []int64{groupA, groupB})
	assertField(t, env, "res.groups", ids["base.group_empty"].ResID, "implied_ids", []int64{})
	assertField(t, env, "res.groups", ids["base.group_command_link"].ResID, "implied_ids", []int64{groupA})
	assertField(t, env, "res.groups", ids["base.group_command_set"].ResID, "implied_ids", []int64{groupA, groupB})
	assertField(t, env, "res.groups", ids["base.group_command_clear"].ResID, "implied_ids", []int64{})
	assertField(t, env, "res.groups", ids["base.group_command_unlink"].ResID, "implied_ids", []int64{groupB})
	assertField(t, env, "res.groups", ids["base.group_command_delete"].ResID, "implied_ids", []int64{})

	rows, err := env.Model("ir.rule").Browse(ids["base.rule_partner"].ResID).Read("model_id", "domain_force", "groups", "global")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if row["model_id"] != ids["base.model_res_partner"].ResID {
		t.Fatalf("unexpected search result: %+v", row)
	}
	if row["domain_force"] != `[["active","=",true]]` {
		t.Fatalf("unexpected domain serialization: %#v", row["domain_force"])
	}
	if !reflect.DeepEqual(row["groups"], []int64{groupA}) {
		t.Fatalf("unexpected rule groups: %#v", row["groups"])
	}
	if row["global"] != false {
		t.Fatalf("unexpected global value: %#v", row["global"])
	}
}

func TestLoadXMLSearchUsesModelAttribute(t *testing.T) {
	env := testEnv(t)
	loader := NewLoader(env, "base")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="company_main" model="res.company">
    <field name="name">Main</field>
  </record>
  <record id="res_partner_search_model" model="res.partner">
    <field name="name">Partner</field>
    <field name="model_search_id" model="res.company" search="[('name', '=', 'Main')]"/>
  </record>
  <record id="res_partner_search_empty" model="res.partner">
    <field name="name">Empty Search</field>
    <field name="model_search_id" model="res.company" search="[('name', '=', 'Missing')]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	assertField(t, env, "res.partner", ids["base.res_partner_search_model"].ResID, "model_search_id", ids["base.company_main"].ResID)
	assertField(t, env, "res.partner", ids["base.res_partner_search_empty"].ResID, "model_search_id", false)
}

func TestLoadXMLX2ManyCommandCreateAndUpdate(t *testing.T) {
	env := testCommandEnv(t)
	loader := NewLoader(env, "x")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="tag_a" model="x.tag">
    <field name="name">A</field>
  </record>
  <record id="tag_b" model="x.tag">
    <field name="name">B</field>
  </record>
  <record id="existing_child" model="x.child">
    <field name="name">Old</field>
  </record>
  <record id="parent" model="x.parent">
    <field name="name">Parent</field>
    <field name="child_ids" eval="[
      Command.clear(),
      Command.create({'name': 'Created', 'tag_ids': [Command.set([ref('tag_a'), ref('tag_b')])]}),
      Command.link(ref('existing_child')),
      Command.update(ref('existing_child'), {'name': 'Updated'})
    ]"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	rows, err := env.Model("x.parent").Browse(ids["x.parent"].ResID).Read("child_ids")
	if err != nil {
		t.Fatal(err)
	}
	childIDs := rows[0]["child_ids"].([]int64)
	if len(childIDs) != 2 || childIDs[1] != ids["x.existing_child"].ResID {
		t.Fatalf("parent child ids = %#v", childIDs)
	}
	assertField(t, env, "x.child", ids["x.existing_child"].ResID, "name", "Updated")
	rows, err = env.Model("x.child").Browse(childIDs[0]).Read("name", "tag_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Created" || !reflect.DeepEqual(rows[0]["tag_ids"], []int64{ids["x.tag_a"].ResID, ids["x.tag_b"].ResID}) {
		t.Fatalf("created child row = %+v", rows[0])
	}
}

func TestLoadXMLNestedX2ManyRecords(t *testing.T) {
	env := testCommandEnv(t)
	loader := NewLoader(env, "x")

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="parent_nested" model="x.parent">
    <field name="name">Nested Parent</field>
    <field name="child_ids">
      <record id="child_nested_a" model="x.child">
        <field name="name">Child A</field>
        <field name="grandchild_ids">
          <record id="grandchild_nested_a" model="x.grandchild">
            <field name="name">Grandchild A</field>
          </record>
        </field>
      </record>
      <record id="child_nested_b" model="x.child">
        <field name="name">Child B</field>
      </record>
    </field>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	ids := loader.ExternalIDs()
	parentID := ids["x.parent_nested"].ResID
	childA := ids["x.child_nested_a"].ResID
	childB := ids["x.child_nested_b"].ResID
	grandchildA := ids["x.grandchild_nested_a"].ResID
	assertField(t, env, "x.parent", parentID, "child_ids", []int64{childA, childB})
	assertField(t, env, "x.child", childA, "parent_id", parentID)
	assertField(t, env, "x.child", childA, "grandchild_ids", []int64{grandchildA})
	assertField(t, env, "x.child", childB, "parent_id", parentID)
	assertField(t, env, "x.grandchild", grandchildA, "child_id", childA)
}

func TestLoadXMLFileAttributeBase64(t *testing.T) {
	env := testEnv(t)
	moduleDir := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("binary\x00payload")
	if err := os.WriteFile(filepath.Join(moduleDir, "payload.bin"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(env, "base")
	loader.SetBaseDir(moduleDir)

	err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="res_partner_file" model="res.partner">
    <field name="name">File Payload</field>
    <field name="payload" type="base64" file="base/payload.bin"/>
  </record>
</odoo>`))
	if err != nil {
		t.Fatal(err)
	}

	partnerID := loader.ExternalIDs()["base.res_partner_file"].ResID
	assertField(t, env, "res.partner", partnerID, "payload", base64.StdEncoding.EncodeToString(payload))
}

func assertField(t *testing.T, env *record.Env, modelName string, id int64, fieldName string, want any) {
	t.Helper()
	got := fieldValue(t, env, modelName, id, fieldName)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s.%s = %#v, want %#v", modelName, fieldName, got, want)
	}
}

func fieldValue(t *testing.T, env *record.Env, modelName string, id int64, fieldName string) any {
	t.Helper()
	rows, err := env.Model(modelName).Browse(id).Read(fieldName)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s(%d) rows = %+v", modelName, id, rows)
	}
	return rows[0][fieldName]
}

func assertLoadedAccessPerms(t *testing.T, env *record.Env, accessID int64, modelID int64, groupID int64, read bool, write bool, create bool, unlink bool) {
	t.Helper()
	rows, err := env.Model("ir.model.access").Browse(accessID).Read("model_id", "group_id", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("access rows = %+v", rows)
	}
	row := rows[0]
	if row["model_id"] != modelID || row["group_id"] != groupID ||
		row["perm_read"] != read || row["perm_write"] != write ||
		row["perm_create"] != create || row["perm_unlink"] != unlink {
		t.Fatalf("access %d = %+v", accessID, row)
	}
}

func assertModelCount(t *testing.T, env *record.Env, modelName string, want int) {
	t.Helper()
	found, err := env.Model(modelName).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != want {
		t.Fatalf("%s count = %d, want %d", modelName, found.Len(), want)
	}
}

func testEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	partner.AddField(field.New("age", field.Int))
	partner.AddField(field.New("score", field.Float))
	partner.AddField(field.New("payload", field.Binary))
	partner.AddField(field.New("model_search_id", field.Many2One))
	partner.AddField(field.New("html_body", field.Text))
	partner.AddField(field.New("file_ref", field.Char))
	partner.AddField(field.New("literal_list", field.Computed))
	partner.AddField(field.New("literal_dict", field.Computed))
	company := model.New("res.company", "res_company")
	company.AddField(field.New("name", field.Char))
	company.AddField(field.New("extra_hour", field.Int))
	company.AddField(field.New("partnership_label", field.Char))
	user := model.New("res.users", "res_users")
	user.AddField(field.New("name", field.Char))
	user.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(company); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(user); err != nil {
		t.Fatal(err)
	}
	return record.NewEnv(reg, record.Context{})
}

func testSafeEvalRecordsetEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("age", field.Int))
	partner.AddField(field.New("model_search_id", field.Many2One).WithRelation("x.created"))
	move := model.New("x.move", "x_move")
	move.AddField(field.New("name", field.Char))
	move.AddField(field.New("move_line_ids", field.Many2Many).WithRelation("x.move.line"))
	line := model.New("x.move.line", "x_move_line")
	line.AddField(field.New("name", field.Char))
	line.AddField(field.New("lot_name", field.Char))
	created := model.New("x.created", "x_created")
	created.AddField(field.New("name", field.Char))
	for _, m := range []model.Model{partner, move, line, created} {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{})
}

func testBaseEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{})
}

func testCommandEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	parent := model.New("x.parent", "x_parent")
	parent.AddField(field.New("name", field.Char))
	parent.AddField(field.New("child_ids", field.One2Many).WithRelation("x.child").WithRelationField("parent_id"))
	child := model.New("x.child", "x_child")
	child.AddField(field.New("name", field.Char))
	child.AddField(field.New("parent_id", field.Many2One).WithRelation("x.parent"))
	child.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.tag"))
	child.AddField(field.New("grandchild_ids", field.One2Many).WithRelation("x.grandchild").WithRelationField("child_id"))
	tag := model.New("x.tag", "x_tag")
	tag.AddField(field.New("name", field.Char))
	grandchild := model.New("x.grandchild", "x_grandchild")
	grandchild.AddField(field.New("name", field.Char))
	grandchild.AddField(field.New("child_id", field.Many2One).WithRelation("x.child"))
	for _, m := range []model.Model{parent, child, tag, grandchild} {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{})
}

func testAccountCSVEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	account := model.New("account.account", "account_account")
	account.AddField(field.New("code", field.Char))
	account.AddField(field.New("placeholder_code", field.Char))
	account.AddField(field.New("name", field.Char))
	account.AddField(field.New("account_type", field.Selection))
	account.AddField(field.New("root_id", field.Many2One).WithRelation("account.root"))
	account.AddField(field.New("group_id", field.Many2One).WithRelation("account.group"))
	account.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	account.AddField(field.New("tag_ids", field.Many2Many).WithRelation("account.account.tag"))
	root := model.New("account.root", "account_root")
	root.AddField(field.New("name", field.Char))
	group := model.New("account.group", "account_group")
	group.AddField(field.New("name", field.Char))
	tag := model.New("account.account.tag", "account_account_tag")
	tag.AddField(field.New("name", field.Char))
	tax := model.New("account.tax", "account_tax")
	tax.AddField(field.New("name", field.Char))
	tax.AddField(field.New("invoice_label", field.Char))
	tax.AddField(field.New("repartition_line_ids", field.One2Many).WithRelation("account.tax.repartition.line").WithRelationField("tax_id"))
	tax.AddField(field.New("children_tax_ids", field.Many2Many).WithRelation("account.tax"))
	tax.AddField(field.New("original_tax_ids", field.Many2Many).WithRelation("account.tax"))
	line := model.New("account.tax.repartition.line", "account_tax_repartition_line")
	line.AddField(field.New("tax_id", field.Many2One).WithRelation("account.tax"))
	line.AddField(field.New("account_id", field.Many2One).WithRelation("account.account"))
	for _, m := range []model.Model{account, root, group, tag, tax, line} {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{})
}
