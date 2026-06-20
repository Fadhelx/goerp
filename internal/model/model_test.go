package model

import (
	"testing"

	"gorp/internal/field"
)

func TestModelValidate(t *testing.T) {
	m := New("res.users", "res_users")
	m.AddField(field.New("name", field.Char))
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestModelValidateAllowsSingleSegmentOdooNames(t *testing.T) {
	m := New("workflow", "workflow")
	m.AddField(field.New("name", field.Char))
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestModelCompose(t *testing.T) {
	parent := New("mail.thread", "mail_thread")
	parent.AddField(field.New("message_ids", field.One2Many).WithRelation("mail.message"))
	child := New("res.partner", "res_partner")
	combined := child.Compose(parent)
	if combined.Fields["message_ids"].Relation != "mail.message" {
		t.Fatalf("missing inherited field: %+v", combined.Fields)
	}
}
