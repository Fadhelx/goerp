package data

import (
	"testing"

	"gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
)

func TestLoadModelMetadataCreatesModelsFieldsAndExternalIDs(t *testing.T) {
	env := testBaseEnv(t)
	externalIDs := map[string]ExternalID{}
	if err := LoadModelMetadata(env, "base", base.Models(), externalIDs); err != nil {
		t.Fatal(err)
	}

	modelID := externalIDs["base.model_res_partner"].ResID
	if modelID == 0 {
		t.Fatalf("missing model external id: %+v", externalIDs)
	}
	found, err := env.Model("ir.model").Search(domain.Cond("model", "=", "res.partner"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("ir.model res.partner count = %d", found.Len())
	}
	fieldExternal := externalIDs["base.field_res_partner__name"]
	if fieldExternal.ResID == 0 || fieldExternal.Model != "ir.model.fields" {
		t.Fatalf("missing field external id: %+v", fieldExternal)
	}
	fields, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "res.partner"),
		domain.Cond("name", "=", "name"),
	))
	if err != nil {
		t.Fatal(err)
	}
	modelRows, err := found.Read("is_mail_thread", "is_mail_activity")
	if err != nil {
		t.Fatal(err)
	}
	if len(modelRows) != 1 || modelRows[0]["is_mail_thread"] != true || modelRows[0]["is_mail_activity"] != true {
		t.Fatalf("res.partner model flags = %+v", modelRows)
	}
	rows, err := fields.Read("ttype", "relation")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["ttype"] != "char" || stringValue(rows[0]["relation"]) != "" {
		t.Fatalf("field rows = %+v", rows)
	}

	companyField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "res.partner"),
		domain.Cond("name", "=", "company_id"),
	))
	if err != nil {
		t.Fatal(err)
	}
	companyRows, err := companyField.Read("ttype", "relation")
	if err != nil {
		t.Fatal(err)
	}
	if len(companyRows) != 1 || companyRows[0]["ttype"] != "many2one" || stringValue(companyRows[0]["relation"]) != "res.company" {
		t.Fatalf("company field rows = %+v", companyRows)
	}

	actionChildField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "ir.actions.server"),
		domain.Cond("name", "=", "child_ids"),
	))
	if err != nil {
		t.Fatal(err)
	}
	actionChildRows, err := actionChildField.Read("ttype", "relation", "relation_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(actionChildRows) != 1 || actionChildRows[0]["ttype"] != "one2many" || stringValue(actionChildRows[0]["relation"]) != "ir.actions.server" || stringValue(actionChildRows[0]["relation_field"]) != "parent_id" {
		t.Fatalf("action child field rows = %+v", actionChildRows)
	}

	automationActionField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "base.automation"),
		domain.Cond("name", "=", "action_server_ids"),
	))
	if err != nil {
		t.Fatal(err)
	}
	automationActionRows, err := automationActionField.Read("ttype", "relation", "relation_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(automationActionRows) != 1 || automationActionRows[0]["ttype"] != "one2many" || stringValue(automationActionRows[0]["relation"]) != "ir.actions.server" || stringValue(automationActionRows[0]["relation_field"]) != "base_automation_id" {
		t.Fatalf("automation action field rows = %+v", automationActionRows)
	}

	sequenceRangeModel, err := env.Model("ir.model").Search(domain.Cond("model", "=", "ir.sequence.date_range"))
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRangeModel.Len() != 1 {
		t.Fatalf("ir.sequence.date_range count = %d", sequenceRangeModel.Len())
	}
	dateRangeField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "ir.sequence"),
		domain.Cond("name", "=", "date_range_ids"),
	))
	if err != nil {
		t.Fatal(err)
	}
	dateRangeRows, err := dateRangeField.Read("ttype", "relation", "relation_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(dateRangeRows) != 1 || dateRangeRows[0]["ttype"] != "one2many" || stringValue(dateRangeRows[0]["relation"]) != "ir.sequence.date_range" || stringValue(dateRangeRows[0]["relation_field"]) != "sequence_id" {
		t.Fatalf("sequence date range field rows = %+v", dateRangeRows)
	}
}

func TestLoadModelMetadataPersistsGroupsAndRelationField(t *testing.T) {
	env := testBaseEnv(t)
	parent := model.New("x.parent", "x_parent")
	parent.AddField(field.New("name", field.Char).WithGroups("base.group_system", "base.group_user"))
	parent.AddField(field.New("child_ids", field.One2Many).WithRelation("x.child").WithRelationField("parent_id"))
	child := model.New("x.child", "x_child")
	child.AddField(field.New("parent_id", field.Many2One).WithRelation("x.parent"))

	if err := LoadModelMetadata(env, "x_module", []model.Model{parent, child}, map[string]ExternalID{}); err != nil {
		t.Fatal(err)
	}

	nameField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "x.parent"),
		domain.Cond("name", "=", "name"),
	))
	if err != nil {
		t.Fatal(err)
	}
	nameRows, err := nameField.Read("groups")
	if err != nil {
		t.Fatal(err)
	}
	if len(nameRows) != 1 || stringValue(nameRows[0]["groups"]) != "base.group_system,base.group_user" {
		t.Fatalf("name field rows = %+v", nameRows)
	}

	childField, err := env.Model("ir.model.fields").Search(domain.And(
		domain.Cond("model", "=", "x.parent"),
		domain.Cond("name", "=", "child_ids"),
	))
	if err != nil {
		t.Fatal(err)
	}
	childRows, err := childField.Read("relation", "relation_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(childRows) != 1 || stringValue(childRows[0]["relation"]) != "x.child" || stringValue(childRows[0]["relation_field"]) != "parent_id" {
		t.Fatalf("child field rows = %+v", childRows)
	}
}

func TestLoadModelMetadataPersistsModelFlags(t *testing.T) {
	env := testBaseEnv(t)
	mailModel := model.New("x.mail.record", "x_mail_record")
	mailModel.Inherit = []string{"mail.thread", "mail.activity.mixin"}
	wizard := model.New("x.mail.wizard", "x_mail_wizard")
	wizard.Transient = true
	mixin := model.New("x.mixin", "")
	mixin.Abstract = true

	if err := LoadModelMetadata(env, "x_module", []model.Model{mailModel, wizard, mixin}, map[string]ExternalID{}); err != nil {
		t.Fatal(err)
	}

	rows, err := env.Model("ir.model").Search(domain.Cond("model", "=", "x.mail.record"))
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := rows.Read("abstract", "transient", "is_mail_thread", "is_mail_activity")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["abstract"] != false || mailRows[0]["transient"] != false || mailRows[0]["is_mail_thread"] != true || mailRows[0]["is_mail_activity"] != true {
		t.Fatalf("mail model flags = %+v", mailRows)
	}
	rows, err = env.Model("ir.model").Search(domain.Cond("model", "=", "x.mail.wizard"))
	if err != nil {
		t.Fatal(err)
	}
	wizardRows, err := rows.Read("transient", "is_mail_thread", "is_mail_activity")
	if err != nil {
		t.Fatal(err)
	}
	if len(wizardRows) != 1 || wizardRows[0]["transient"] != true || wizardRows[0]["is_mail_thread"] != false || wizardRows[0]["is_mail_activity"] != false {
		t.Fatalf("wizard flags = %+v", wizardRows)
	}
	rows, err = env.Model("ir.model").Search(domain.Cond("model", "=", "x.mixin"))
	if err != nil {
		t.Fatal(err)
	}
	mixinRows, err := rows.Read("abstract", "transient")
	if err != nil {
		t.Fatal(err)
	}
	if len(mixinRows) != 1 || mixinRows[0]["abstract"] != true || mixinRows[0]["transient"] != false {
		t.Fatalf("mixin flags = %+v", mixinRows)
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
