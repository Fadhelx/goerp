package record

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	coreaccounting "gorp/internal/accounting"
	internalbase "gorp/internal/base"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/sequencecore"
)

func TestRecordSetCRUD(t *testing.T) {
	env := testEnv(t)
	partners := env.Model("res.partner")
	id, err := partners.Create(map[string]any{"name": "Admin", "active": true})
	if err != nil {
		t.Fatal(err)
	}

	found, err := partners.Search(domain.Cond("name", domain.Equal, "Admin"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 || found.IDs()[0] != id {
		t.Fatalf("found ids = %+v", found.IDs())
	}

	if err := found.Write(map[string]any{"name": "Administrator"}); err != nil {
		t.Fatal(err)
	}
	values, err := found.Mapped("name")
	if err != nil {
		t.Fatal(err)
	}
	if values[0] != "Administrator" {
		t.Fatalf("values = %+v", values)
	}

	idRows, err := partners.Browse(id).Read("id", "name")
	if err != nil {
		t.Fatal(err)
	}
	if len(idRows) != 1 || idRows[0]["id"] != id || idRows[0]["name"] != "Administrator" {
		t.Fatalf("read id rows = %+v", idRows)
	}

	if err := found.Unlink(); err != nil {
		t.Fatal(err)
	}
	rows, err := partners.Browse(id).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected deleted row, got %+v", rows)
	}
}

func TestMailingSubscriptionDerivedListAndContactFields(t *testing.T) {
	env := baseRecordEnv(t)
	listID, err := env.Model("mailing.list").Create(map[string]any{"name": "Newsletter", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Reader", "email": "reader@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}

	contacts, err := env.Model("mailing.contact").Search(domain.Cond("list_ids", domain.In, []any{listID}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contacts.IDs(), []int64{contactID}) {
		t.Fatalf("contact ids = %+v", contacts.IDs())
	}
	contactRows, err := env.Model("mailing.contact").Browse(contactID).Read("list_ids", "subscription_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(contactRows) != 1 || !reflect.DeepEqual(contactRows[0]["list_ids"], []int64{listID}) || !reflect.DeepEqual(contactRows[0]["subscription_ids"], []int64{subscriptionID}) {
		t.Fatalf("contact row = %+v", contactRows)
	}
	listRows, err := env.Model("mailing.list").Browse(listID).Read("contact_ids", "subscription_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(listRows) != 1 || !reflect.DeepEqual(listRows[0]["contact_ids"], []int64{contactID}) || !reflect.DeepEqual(listRows[0]["subscription_ids"], []int64{subscriptionID}) {
		t.Fatalf("list row = %+v", listRows)
	}
}

func TestMarketingTraceWhatsAppClickDatetimeDerivedFromMessage(t *testing.T) {
	env := baseRecordEnv(t)
	clickedAt := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	staleTraceAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	whatsAppID, err := env.Model("whatsapp.message").Create(map[string]any{
		"state":                "sent",
		"links_click_datetime": clickedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("marketing.trace").Create(map[string]any{
		"whatsapp_message_id":  whatsAppID,
		"links_click_datetime": staleTraceAt,
		"state":                "scheduled",
	})
	if err != nil {
		t.Fatal(err)
	}
	plainTraceID, err := env.Model("marketing.trace").Create(map[string]any{
		"links_click_datetime": staleTraceAt,
		"state":                "scheduled",
	})
	if err != nil {
		t.Fatal(err)
	}

	traceRows, err := env.Model("marketing.trace").Browse(traceID, plainTraceID).Read("id", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]time.Time{}
	for _, row := range traceRows {
		byID[numericID(row["id"])] = auditTimeValue(row["links_click_datetime"])
	}
	if !byID[traceID].Equal(clickedAt) {
		t.Fatalf("whatsapp trace click datetime = %v, want %v rows=%+v", byID[traceID], clickedAt, traceRows)
	}
	if !byID[plainTraceID].Equal(staleTraceAt) {
		t.Fatalf("plain trace click datetime = %v, want %v rows=%+v", byID[plainTraceID], staleTraceAt, traceRows)
	}
	found, err := env.Model("marketing.trace").Search(domain.Cond("links_click_datetime", domain.Equal, clickedAt))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found.IDs(), []int64{traceID}) {
		t.Fatalf("derived datetime search ids = %+v", found.IDs())
	}

	nextClick := clickedAt.Add(2 * time.Hour)
	if err := env.Model("whatsapp.message").Browse(whatsAppID).Write(map[string]any{"links_click_datetime": nextClick}); err != nil {
		t.Fatal(err)
	}
	updatedRows, err := env.Model("marketing.trace").Browse(traceID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedRows) != 1 || !auditTimeValue(updatedRows[0]["links_click_datetime"]).Equal(nextClick) {
		t.Fatalf("updated trace rows = %+v", updatedRows)
	}
}

func TestIrConfigParameterCatchallAllowedDomainsSanitizes(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", ""},
		{"single", "hello.COM", "hello.com"},
		{"trailing_empty", "hello.com,,", "hello.com"},
		{"multi", "hello.COM, BONJOUR.com", "hello.com,bonjour.com"},
		{"preserve_duplicates", "EXAMPLE.COM,example.com", "example.com,example.com"},
		{"loose_shape", "not a domain,a@b.com,hello .com", "not a domain,a@b.com,hello .com"},
	}
	for _, tc := range cases {
		t.Run("create_"+tc.name, func(t *testing.T) {
			env := baseRecordEnv(t)
			id, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": tc.value})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := env.Model("ir.config_parameter").Browse(id).Read("value")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["value"] != tc.want {
				t.Fatalf("create value = %+v, want %q", rows, tc.want)
			}
		})
		t.Run("write_"+tc.name, func(t *testing.T) {
			env := baseRecordEnv(t)
			id, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": "valid.example"})
			if err != nil {
				t.Fatal(err)
			}
			if err := env.Model("ir.config_parameter").Browse(id).Write(map[string]any{"value": tc.value}); err != nil {
				t.Fatal(err)
			}
			rows, err := env.Model("ir.config_parameter").Browse(id).Read("value")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["value"] != tc.want {
				t.Fatalf("write value = %+v, want %q", rows, tc.want)
			}
		})
	}
}

func TestIrConfigParameterCatchallAllowedDomainsRejectsEmptySegments(t *testing.T) {
	for _, value := range []string{",", ",,", ", ,", "   "} {
		t.Run("create_"+value, func(t *testing.T) {
			env := baseRecordEnv(t)
			_, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": value})
			if err == nil || !strings.Contains(err.Error(), "cannot be validated") {
				t.Fatalf("create error = %v", err)
			}
			found, err := env.Model("ir.config_parameter").Search(domain.Cond("key", domain.Equal, "mail.catchall.domain.allowed"))
			if err != nil {
				t.Fatal(err)
			}
			if found.Len() != 0 {
				t.Fatalf("target row created for %q", value)
			}
		})
		t.Run("write_"+value, func(t *testing.T) {
			env := baseRecordEnv(t)
			id, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": "valid.example"})
			if err != nil {
				t.Fatal(err)
			}
			err = env.Model("ir.config_parameter").Browse(id).Write(map[string]any{"value": value})
			if err == nil || !strings.Contains(err.Error(), "cannot be validated") {
				t.Fatalf("write error = %v", err)
			}
			rows, err := env.Model("ir.config_parameter").Browse(id).Read("value")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["value"] != "valid.example" {
				t.Fatalf("write preserved value = %+v", rows)
			}
		})
	}
}

func TestIrConfigParameterCatchallAllowedDomainsUsesEffectiveWriteKey(t *testing.T) {
	env := baseRecordEnv(t)
	id, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "x.mail.catchall.domain.allowed", "value": "RAW"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("ir.config_parameter").Browse(id).Write(map[string]any{"key": "mail.catchall.domain.allowed", "value": "HELLO.COM,,"}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.config_parameter").Browse(id).Read("key", "value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["key"] != "mail.catchall.domain.allowed" || rows[0]["value"] != "hello.com" {
		t.Fatalf("effective key row = %+v", rows)
	}
}

func TestIrConfigParameterNonTargetKeyUntouched(t *testing.T) {
	env := baseRecordEnv(t)
	id, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "x.mail.catchall.domain.allowed", "value": ", ,"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("ir.config_parameter").Browse(id).Write(map[string]any{"value": "HELLO.COM, BONJOUR.com,,"}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.config_parameter").Browse(id).Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["value"] != "HELLO.COM, BONJOUR.com,," {
		t.Fatalf("non-target row = %+v", rows)
	}
}

func TestMailAliasDomainCompanyConstraint(t *testing.T) {
	env := baseRecordEnv(t)
	domainA, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "a.example"})
	if err != nil {
		t.Fatal(err)
	}
	domainB, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "b.example"})
	if err != nil {
		t.Fatal(err)
	}
	miscDomain, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "misc.example"})
	if err != nil {
		t.Fatal(err)
	}
	companyA, err := env.Model("res.company").Create(map[string]any{"name": "Company A", "alias_domain_id": domainA})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.company").Create(map[string]any{"name": "Company B", "alias_domain_id": domainB}); err != nil {
		t.Fatal(err)
	}
	companyNoDomain, err := env.Model("res.company").Create(map[string]any{"name": "No Domain Company"})
	if err != nil {
		t.Fatal(err)
	}
	partnerA, err := env.Model("res.partner").Create(map[string]any{"name": "Partner A", "company_id": companyA, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerNoDomain, err := env.Model("res.partner").Create(map[string]any{"name": "Partner No Domain", "company_id": companyNoDomain, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}

	ownerAliasID, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "owner-ok",
		"alias_domain_id":        domainA,
		"alias_model_id":         modelID,
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": partnerA,
		"active":                 true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.alias").Browse(ownerAliasID).Write(map[string]any{"alias_domain_id": domainB}); err == nil || !strings.Contains(err.Error(), "owner document belongs to company Company A") {
		t.Fatalf("owner mismatch error = %v", err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "owner-bad",
		"alias_domain_id":        domainB,
		"alias_model_id":         modelID,
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": partnerA,
		"active":                 true,
	}); err == nil || !strings.Contains(err.Error(), "owner document belongs to company Company A") {
		t.Fatalf("owner create mismatch error = %v", err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "owner-no-company-domain",
		"alias_domain_id":        domainA,
		"alias_model_id":         modelID,
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": partnerNoDomain,
		"active":                 true,
	}); err == nil || !strings.Contains(err.Error(), "owner document belongs to company No Domain Company") {
		t.Fatalf("owner no-domain mismatch error = %v", err)
	}
	if err := env.Model("mail.alias").Browse(ownerAliasID).Write(map[string]any{"alias_domain_id": miscDomain}); err != nil {
		t.Fatal(err)
	}

	targetAliasID, err := env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "target-ok",
		"alias_domain_id":       domainA,
		"model_name":            "res.partner",
		"alias_model_id":        modelID,
		"alias_force_thread_id": partnerA,
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.alias").Browse(targetAliasID).Write(map[string]any{"alias_domain_id": domainB}); err == nil || !strings.Contains(err.Error(), "target document belongs to company Company A") {
		t.Fatalf("target mismatch error = %v", err)
	}
}

func TestMailAliasNameDomainUniqueness(t *testing.T) {
	env := baseRecordEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	domainA, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "unique-a.example"})
	if err != nil {
		t.Fatal(err)
	}
	domainB, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "unique-b.example"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "Sales Team", "alias_domain_id": domainA, "alias_model_id": modelID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.alias").Browse(firstID).Read("alias_name", "alias_full_name", "alias_domain", "alias_defaults", "alias_contact", "alias_status", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_name"] != "sales-team" || rows[0]["alias_full_name"] != "sales-team@unique-a.example" || rows[0]["alias_domain"] != "unique-a.example" || rows[0]["alias_defaults"] != "{}" || rows[0]["alias_contact"] != "everyone" || rows[0]["alias_status"] != "not_tested" || rows[0]["active"] != true {
		t.Fatalf("normalized alias = %+v", rows)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "sales-team", "alias_domain_id": domainA, "alias_model_id": modelID}); err == nil || !strings.Contains(err.Error(), "cannot be used on several records") {
		t.Fatalf("duplicate create error = %v", err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "sales-team", "alias_domain_id": domainB, "alias_model_id": modelID}); err != nil {
		t.Fatalf("same alias on other domain: %v", err)
	}
	noDomainID, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "plain", "alias_model_id": modelID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "plain", "alias_model_id": modelID}); err == nil || !strings.Contains(err.Error(), "cannot be used on several records") {
		t.Fatalf("duplicate no-domain create error = %v", err)
	}
	if err := env.Model("mail.alias").Browse(noDomainID).Write(map[string]any{"alias_name": "sales-team", "alias_domain_id": domainA}); err == nil || !strings.Contains(err.Error(), "cannot be used on several records") {
		t.Fatalf("duplicate write error = %v", err)
	}
	rows, err = env.Model("mail.alias").Browse(noDomainID).Read("alias_name", "alias_domain_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_name"] != "plain" || numericID(rows[0]["alias_domain_id"]) != 0 {
		t.Fatalf("duplicate write rollback rows = %+v", rows)
	}
}

func TestMailAliasDomainNameAndLocalPartIntegrity(t *testing.T) {
	env := baseRecordEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"bad,example.com", "éxample.com", ""} {
		if _, err := env.Model("mail.alias.domain").Create(map[string]any{"name": invalid}); err == nil || !strings.Contains(err.Error(), "domain name") {
			t.Fatalf("invalid domain %q error = %v", invalid, err)
		}
	}
	domainID, err := env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "mail.example",
		"bounce_alias":   "Bounce Box",
		"catchall_alias": "Catch All",
		"default_from":   "Notify Team@Example.COM",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.alias.domain").Browse(domainID).Read("bounce_alias", "catchall_alias", "default_from", "bounce_email", "catchall_email", "default_from_email", "sequence")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["bounce_alias"] != "bounce-box" ||
		rows[0]["catchall_alias"] != "catch-all" ||
		rows[0]["default_from"] != "notify-team@example.com" ||
		rows[0]["bounce_email"] != "bounce-box@mail.example" ||
		rows[0]["catchall_email"] != "catch-all@mail.example" ||
		rows[0]["default_from_email"] != "notify-team@example.com" ||
		rows[0]["sequence"] != int64(10) {
		t.Fatalf("domain rows = %+v", rows)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "Bounce Box", "alias_domain_id": domainID, "alias_model_id": modelID}); err == nil || !strings.Contains(err.Error(), "bounce or catchall address") {
		t.Fatalf("bounce local clash error = %v", err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "Catch All", "alias_domain_id": domainID, "alias_model_id": modelID}); err == nil || !strings.Contains(err.Error(), "bounce or catchall address") {
		t.Fatalf("catchall local clash error = %v", err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "Notify Team", "alias_domain_id": domainID, "alias_model_id": modelID}); err != nil {
		t.Fatalf("default_from local alias should be allowed: %v", err)
	}
	otherDomainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "other.example", "bounce_alias": "Bounce Box"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "Bounce Box", "alias_domain_id": otherDomainID, "alias_model_id": modelID}); err == nil || !strings.Contains(err.Error(), "bounce or catchall address") {
		t.Fatalf("other same-domain bounce clash error = %v", err)
	}
	if _, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "mail.example", "bounce_alias": "Bounce Box"}); err == nil || !strings.Contains(err.Error(), "Bounce alias") {
		t.Fatalf("duplicate bounce domain error = %v", err)
	}
}

func TestMailAliasDefaultsValidationRollback(t *testing.T) {
	env := baseRecordEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	domainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "defaults.example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "bad-defaults", "alias_domain_id": domainID, "alias_model_id": modelID, "alias_defaults": "['bad']"}); err == nil || !strings.Contains(err.Error(), "literal python dictionary") {
		t.Fatalf("invalid defaults create error = %v", err)
	}
	found, err := env.Model("mail.alias").Search(domain.Cond("alias_name", domain.Equal, "bad-defaults"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("invalid defaults row created count = %d", found.Len())
	}
	aliasID, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "good-defaults", "alias_domain_id": domainID, "alias_model_id": modelID, "alias_defaults": "{'name': 'Ok'}"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.alias").Browse(aliasID).Write(map[string]any{"alias_defaults": "{bad}"}); err == nil || !strings.Contains(err.Error(), "literal python dictionary") {
		t.Fatalf("invalid defaults write error = %v", err)
	}
	rows, err := env.Model("mail.alias").Browse(aliasID).Read("alias_defaults", "alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_defaults"] != "{'name': 'Ok'}" || rows[0]["alias_status"] != "not_tested" {
		t.Fatalf("defaults rollback rows = %+v", rows)
	}
}

func TestMailAliasDomainFirstCreateAssignsArchivedRecords(t *testing.T) {
	env := baseRecordEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Archived Company", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := env.Model("mail.alias").Create(map[string]any{"alias_name": "archived", "alias_model_id": modelID, "active": false})
	if err != nil {
		t.Fatal(err)
	}
	domainID, err := env.Model("mail.alias.domain").Create(map[string]any{"name": "first.example"})
	if err != nil {
		t.Fatal(err)
	}
	companyRows, err := env.Model("res.company").Browse(companyID).Read("alias_domain_id")
	if err != nil {
		t.Fatal(err)
	}
	aliasRows, err := env.Model("mail.alias").Browse(aliasID).Read("alias_domain_id", "alias_domain", "alias_full_name")
	if err != nil {
		t.Fatal(err)
	}
	if len(companyRows) != 1 || companyRows[0]["alias_domain_id"] != domainID {
		t.Fatalf("company rows = %+v domain=%d", companyRows, domainID)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_domain_id"] != domainID || aliasRows[0]["alias_domain"] != "first.example" || aliasRows[0]["alias_full_name"] != "archived@first.example" {
		t.Fatalf("alias rows = %+v domain=%d", aliasRows, domainID)
	}
}

func TestLogAccessFieldsPopulateWhenDeclared(t *testing.T) {
	registry := NewRegistry()
	auditModel := model.New("x.audit", "x_audit")
	for _, f := range []field.Field{
		field.New("name", field.Char),
		field.New("create_uid", field.Many2One).WithRelation("res.users"),
		field.New("create_date", field.DateTime),
		field.New("write_uid", field.Many2One).WithRelation("res.users"),
		field.New("write_date", field.DateTime),
	} {
		auditModel.AddField(f)
	}
	partialModel := model.New("x.audit.partial", "x_audit_partial")
	partialModel.AddField(field.New("name", field.Char))
	partialModel.AddField(field.New("create_uid", field.Many2One).WithRelation("res.users"))
	plainModel := model.New("x.audit.plain", "x_audit_plain")
	plainModel.AddField(field.New("name", field.Char))
	for _, item := range []model.Model{auditModel, partialModel, plainModel} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := NewEnv(registry, Context{UserID: 11, CompanyID: 1})
	auditID, err := env.Model("x.audit").Create(map[string]any{"name": "Tracked"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("x.audit").Browse(auditID).Read("create_uid", "create_date", "write_uid", "write_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["create_uid"] != int64(11) || rows[0]["write_uid"] != int64(11) || auditTimeValue(rows[0]["create_date"]).IsZero() || auditTimeValue(rows[0]["write_date"]).IsZero() {
		t.Fatalf("audit row = %+v", rows)
	}
	createDate := auditTimeValue(rows[0]["create_date"])
	writeDate := auditTimeValue(rows[0]["write_date"])
	if err := env.WithContext(Context{UserID: 22, CompanyID: 1}).Model("x.audit").Browse(auditID).Write(map[string]any{"name": "Updated"}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("x.audit").Browse(auditID).Read("create_uid", "create_date", "write_uid", "write_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["create_uid"] != int64(11) || !auditTimeValue(rows[0]["create_date"]).Equal(createDate) || rows[0]["write_uid"] != int64(22) || auditTimeValue(rows[0]["write_date"]).Before(writeDate) {
		t.Fatalf("updated audit row = %+v", rows)
	}
	explicitDate := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	if err := env.WithContext(Context{UserID: 44, CompanyID: 1}).Model("x.audit").Browse(auditID).Write(map[string]any{"create_uid": int64(99), "create_date": explicitDate, "write_uid": int64(99), "write_date": explicitDate}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("x.audit").Browse(auditID).Read("create_uid", "create_date", "write_uid", "write_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["create_uid"] != int64(11) || !auditTimeValue(rows[0]["create_date"]).Equal(createDate) || rows[0]["write_uid"] != int64(44) || auditTimeValue(rows[0]["write_date"]).Equal(explicitDate) {
		t.Fatalf("spoofed write audit row = %+v", rows)
	}
	explicitID, err := env.Model("x.audit").Create(map[string]any{"name": "Explicit", "create_uid": int64(33), "create_date": explicitDate})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("x.audit").Browse(explicitID).Read("create_uid", "create_date", "write_uid", "write_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["create_uid"] != int64(11) || auditTimeValue(rows[0]["create_date"]).Equal(explicitDate) || rows[0]["write_uid"] != int64(11) || auditTimeValue(rows[0]["write_date"]).IsZero() {
		t.Fatalf("explicit audit row = %+v", rows)
	}
	partialID, err := env.Model("x.audit.partial").Create(map[string]any{"name": "Partial"})
	if err != nil {
		t.Fatal(err)
	}
	partialRows, err := env.Model("x.audit.partial").Browse(partialID).Read("create_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(partialRows) != 1 || partialRows[0]["create_uid"] != int64(11) {
		t.Fatalf("partial audit row = %+v", partialRows)
	}
	if _, err := env.Model("x.audit.plain").Create(map[string]any{"name": "Plain"}); err != nil {
		t.Fatal(err)
	}
}

func TestMailMailCreateAllowsPublicForcedServer(t *testing.T) {
	env := baseRecordEnv(t)
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Public",
		"active":          false,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(25),
		"smtp_encryption": "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID := createRecordMailMessage(t, env, "Public")
	if _, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Sender <sender@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Public",
		"body_html":       "<p>Public</p>",
		"state":           "outgoing",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMailMailCreateAllowsMessageCreatorPersonalServer(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(7))
	ownerEnv := env.WithContext(Context{UserID: 7, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID := createRecordMailMessage(t, ownerEnv, "Owned")
	if _, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Other <other@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Owned",
		"body_html":       "<p>Owned</p>",
		"state":           "outgoing",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMailMailCreateRejectsOtherUserPersonalServer(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(9))
	messageID := createRecordMailMessage(t, env, "Denied")
	_, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Denied",
		"body_html":       "<p>Denied</p>",
		"state":           "outgoing",
	})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("create error = %v", err)
	}
}

func TestMailMailWriteRejectsOtherUserPersonalServer(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(9))
	messageID := createRecordMailMessage(t, env, "Denied")
	mailID, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Denied",
		"body_html":       "<p>Denied</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = env.Model("mail.mail").Browse(mailID).Write(map[string]any{"mail_server_id": serverID})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("write error = %v", err)
	}
}

func TestMailMailWriteRevalidatesMessageCreator(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(7))
	ownerEnv := env.WithContext(Context{UserID: 7, CompanyID: 1, CompanyIDs: []int64{1}})
	otherEnv := env.WithContext(Context{UserID: 8, CompanyID: 1, CompanyIDs: []int64{1}})
	ownerMessageID := createRecordMailMessage(t, ownerEnv, "Owner")
	otherMessageID := createRecordMailMessage(t, otherEnv, "Other")
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": ownerMessageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Owner",
		"body_html":       "<p>Owner</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ownerEnv.Model("mail.mail").Browse(mailID).Write(map[string]any{"mail_message_id": otherMessageID})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("write error = %v", err)
	}
}

func TestMailMailCreateRejectsPersonalServerWhenDisabledByContext(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(7))
	disabledEnv := env.WithContext(Context{
		UserID:     7,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values:     map[string]any{"mail.disable_personal_mail_servers": true},
	})
	messageID := createRecordMailMessage(t, disabledEnv, "Disabled")
	_, err := disabledEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Disabled",
		"body_html":       "<p>Disabled</p>",
		"state":           "outgoing",
	})
	if err == nil || !strings.Contains(err.Error(), "personal mail_server_id unauthorized") {
		t.Fatalf("create error = %v", err)
	}
}

func TestMailMailWriteEmailFromDoesNotRevalidateForcedServer(t *testing.T) {
	env := baseRecordEnv(t)
	serverID := createRecordMailServer(t, env, int64(7))
	ownerEnv := env.WithContext(Context{UserID: 7, CompanyID: 1, CompanyIDs: []int64{1}})
	messageID := createRecordMailMessage(t, ownerEnv, "Owned")
	mailID, err := ownerEnv.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"mail_server_id":  serverID,
		"email_from":      "Owner <owner@example.com>",
		"email_to":        "recipient@example.com",
		"subject":         "Owned",
		"body_html":       "<p>Owned</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{"owner_user_id": int64(9)}); err != nil {
		t.Fatal(err)
	}
	if err := ownerEnv.Model("mail.mail").Browse(mailID).Write(map[string]any{"email_from": "Other <other@example.com>"}); err != nil {
		t.Fatal(err)
	}
}

func auditTimeValue(value any) time.Time {
	typed, _ := value.(time.Time)
	return typed
}

func TestIrModelDataNormalizesAndValidatesXMLIDs(t *testing.T) {
	registry := NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	externalID := model.New("ir.model.data", "ir_model_data")
	externalID.AddField(field.New("module", field.Char))
	externalID.AddField(field.New("name", field.Char))
	externalID.AddField(field.New("complete_name", field.Char))
	externalID.AddField(field.New("model", field.Char))
	externalID.AddField(field.New("res_id", field.Int))
	externalID.AddField(field.New("noupdate", field.Bool))
	for _, item := range []model.Model{partner, externalID} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	modelData := env.Model("ir.model.data")
	externalIDID, err := modelData.Create(map[string]any{
		"module":        "base",
		"name":          "partner_demo",
		"complete_name": "stale",
		"model":         "res.partner",
		"res_id":        partnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := modelData.Browse(externalIDID).Read("module", "name", "complete_name", "model", "res_id", "noupdate")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["complete_name"] != "base.partner_demo" || rows[0]["noupdate"] != false {
		t.Fatalf("normalized external id row = %+v", rows)
	}
	if _, err := modelData.Create(map[string]any{"module": "base", "name": "partner_demo", "model": "res.partner", "res_id": partnerID}); err == nil {
		t.Fatal("expected duplicate module/name rejection")
	}
	if _, err := modelData.Create(map[string]any{"module": "other", "name": "partner_demo", "model": "res.partner", "res_id": partnerID}); err != nil {
		t.Fatal(err)
	}
	if _, err := modelData.Create(map[string]any{"module": "base", "name": "bad id", "model": "res.partner", "res_id": partnerID}); err == nil {
		t.Fatal("expected space rejection")
	}
	if err := modelData.Browse(externalIDID).Write(map[string]any{"module": "", "name": "partner_demo_export"}); err != nil {
		t.Fatal(err)
	}
	rows, err = modelData.Browse(externalIDID).Read("module", "name", "complete_name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["module"] != "" || rows[0]["name"] != "partner_demo_export" || rows[0]["complete_name"] != "partner_demo_export" {
		t.Fatalf("renamed external id row = %+v", rows)
	}
	if _, err := modelData.Create(map[string]any{"module": "", "name": "partner_demo_export", "model": "res.partner", "res_id": partnerID}); err == nil {
		t.Fatal("expected empty-module duplicate rejection")
	}
}

func TestActionBaseLifecycleUsesGlobalIDs(t *testing.T) {
	registry := NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	windowID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":               "Partners",
		"type":               "ir.actions.act_window",
		"res_model":          "res.partner",
		"binding_type":       "action",
		"binding_view_types": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":        "Partner Label",
		"type":        "ir.actions.report",
		"model":       "res.partner",
		"report_name": "x.partner.label",
	})
	if err != nil {
		t.Fatal(err)
	}
	if windowID == reportID {
		t.Fatalf("action IDs collided: %d", windowID)
	}
	baseRows, err := env.Model("ir.actions.actions").Browse(windowID, reportID).Read("id", "name", "type", "binding_type")
	if err != nil {
		t.Fatal(err)
	}
	types := map[int64]string{}
	for _, row := range baseRows {
		types[numericID(row["id"])] = stringValue(row["type"])
	}
	if types[windowID] != "ir.actions.act_window" || types[reportID] != "ir.actions.report" {
		t.Fatalf("base action rows = %+v", baseRows)
	}
	if err := env.Model("ir.actions.act_window").Browse(windowID).Write(map[string]any{"name": "Customers", "binding_type": "report"}); err != nil {
		t.Fatal(err)
	}
	baseRows, err = env.Model("ir.actions.actions").Browse(windowID).Read("name", "binding_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseRows) != 1 || baseRows[0]["name"] != "Customers" || baseRows[0]["binding_type"] != "report" {
		t.Fatalf("base action not synced after write: %+v", baseRows)
	}
	if _, err := env.Model("ir.actions.client").Create(map[string]any{"id": windowID, "name": "Conflict", "type": "ir.actions.client"}); err == nil {
		t.Fatal("expected explicit ID conflict")
	}
	if err := env.Model("ir.actions.report").Browse(reportID).Unlink(); err != nil {
		t.Fatal(err)
	}
	baseRows, err = env.Model("ir.actions.actions").Browse(reportID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseRows) != 0 {
		t.Fatalf("base action not removed after unlink: %+v", baseRows)
	}
}

func TestActionCreateAppliesOdooDefaults(t *testing.T) {
	registry := NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	windowID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Partners",
		"res_model": "res.partner",
	})
	if err != nil {
		t.Fatal(err)
	}
	windowRows, err := env.Model("ir.actions.act_window").Browse(windowID).Read("type", "context", "target", "view_mode", "mobile_view_mode", "limit", "cache", "filter", "binding_type", "binding_view_types")
	if err != nil {
		t.Fatal(err)
	}
	if len(windowRows) != 1 ||
		windowRows[0]["type"] != "ir.actions.act_window" ||
		windowRows[0]["context"] != "{}" ||
		windowRows[0]["target"] != "current" ||
		windowRows[0]["view_mode"] != "list,form" ||
		windowRows[0]["mobile_view_mode"] != "kanban" ||
		numericID(windowRows[0]["limit"]) != 80 ||
		windowRows[0]["cache"] != true ||
		windowRows[0]["filter"] != false ||
		windowRows[0]["binding_type"] != "action" ||
		windowRows[0]["binding_view_types"] != "list,form" {
		t.Fatalf("window defaults = %+v", windowRows)
	}
	urlID, err := env.Model("ir.actions.act_url").Create(map[string]any{
		"name": "Docs",
		"url":  "https://example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := env.Model("ir.actions.client").Create(map[string]any{
		"name": "Client",
		"tag":  "x.client",
	})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":        "Report",
		"model":       "res.partner",
		"report_name": "x.report",
	})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":  "Server",
		"state": "code",
	})
	if err != nil {
		t.Fatal(err)
	}
	urlRows, err := env.Model("ir.actions.act_url").Browse(urlID).Read("type", "target", "close", "binding_type", "binding_view_types")
	if err != nil {
		t.Fatal(err)
	}
	if urlRows[0]["type"] != "ir.actions.act_url" || urlRows[0]["target"] != "new" || urlRows[0]["close"] != false || urlRows[0]["binding_type"] != "action" || urlRows[0]["binding_view_types"] != "list,form" {
		t.Fatalf("url defaults = %+v", urlRows)
	}
	clientRows, err := env.Model("ir.actions.client").Browse(clientID).Read("type", "target", "context")
	if err != nil {
		t.Fatal(err)
	}
	if clientRows[0]["type"] != "ir.actions.client" || clientRows[0]["target"] != "current" || clientRows[0]["context"] != "{}" {
		t.Fatalf("client defaults = %+v", clientRows)
	}
	reportRows, err := env.Model("ir.actions.report").Browse(reportID).Read("type", "report_type", "binding_type", "binding_view_types", "close_on_report_download")
	if err != nil {
		t.Fatal(err)
	}
	if reportRows[0]["type"] != "ir.actions.report" || reportRows[0]["report_type"] != "qweb-pdf" || reportRows[0]["binding_type"] != "report" || reportRows[0]["binding_view_types"] != "list,form" || reportRows[0]["close_on_report_download"] != false {
		t.Fatalf("report defaults = %+v", reportRows)
	}
	serverRows, err := env.Model("ir.actions.server").Browse(serverID).Read("type", "binding_type", "binding_view_types", "active", "usage", "sequence", "evaluation_type", "update_m2m_operation", "update_boolean_value")
	if err != nil {
		t.Fatal(err)
	}
	if serverRows[0]["type"] != "ir.actions.server" ||
		serverRows[0]["binding_type"] != "action" ||
		serverRows[0]["binding_view_types"] != "list,form" ||
		serverRows[0]["active"] != true ||
		serverRows[0]["usage"] != "ir_actions_server" ||
		numericID(serverRows[0]["sequence"]) != 5 ||
		serverRows[0]["evaluation_type"] != "value" ||
		serverRows[0]["update_m2m_operation"] != "add" ||
		serverRows[0]["update_boolean_value"] != "true" {
		t.Fatalf("server defaults = %+v", serverRows)
	}
	defaults, err := env.Model("ir.actions.act_window").DefaultGet([]string{"type", "context", "target", "view_mode", "mobile_view_mode", "limit", "cache", "binding_type", "binding_view_types"}, map[string]any{"default_target": "new"})
	if err != nil {
		t.Fatal(err)
	}
	if defaults["type"] != "ir.actions.act_window" || defaults["context"] != "{}" || defaults["target"] != "new" || defaults["view_mode"] != "list,form" || defaults["mobile_view_mode"] != "kanban" || numericID(defaults["limit"]) != 80 || defaults["cache"] != true || defaults["binding_type"] != "action" || defaults["binding_view_types"] != "list,form" {
		t.Fatalf("window default_get = %+v", defaults)
	}
	baseRows, err := env.Model("ir.actions.actions").Browse(windowID, reportID).Read("id", "type", "binding_type", "binding_view_types")
	if err != nil {
		t.Fatal(err)
	}
	baseByType := map[string]map[string]any{}
	for _, row := range baseRows {
		baseByType[stringValue(row["type"])] = row
	}
	if baseByType["ir.actions.act_window"]["binding_type"] != "action" || baseByType["ir.actions.report"]["binding_type"] != "report" {
		t.Fatalf("base defaults = %+v", baseRows)
	}
}

func TestRegisterComposesInheritedAbstractFields(t *testing.T) {
	registry := NewRegistry()
	parent := model.New("x.mixin", "")
	parent.Abstract = true
	parent.AddField(field.New("mixin_value", field.Char))
	parent.AddField(field.New("shared", field.Char))
	if err := registry.Register(parent); err != nil {
		t.Fatal(err)
	}
	child := model.New("x.child", "x_child")
	child.Inherit = []string{"x.mixin"}
	child.AddField(field.New("name", field.Char))
	child.AddField(field.New("shared", field.Text))
	if err := registry.Register(child); err != nil {
		t.Fatal(err)
	}
	registered, ok := registry.Model("x.child")
	if !ok {
		t.Fatal("missing child model")
	}
	if _, ok := registered.Fields["mixin_value"]; !ok {
		t.Fatalf("inherited fields missing: %+v", registered.Fields)
	}
	if registered.Fields["shared"].Kind != field.Text {
		t.Fatalf("child field override lost: %+v", registered.Fields["shared"])
	}
}

func TestRecordSetCallsPolicy(t *testing.T) {
	env := testEnv(t).WithPolicy(denyPolicy{})
	_, err := env.Model("res.partner").Create(map[string]any{"name": "Admin"})
	if !errors.Is(err, errDenied) {
		t.Fatalf("expected denied error, got %v", err)
	}
}

func TestAfterCreateHookRollsBackCreate(t *testing.T) {
	env := testEnv(t)
	hookErr := errors.New("hook failed")
	calls := 0
	env.RegisterAfterCreateHook(func(_ *Env, modelName string, id int64, row map[string]any) error {
		if modelName != "res.partner" {
			return nil
		}
		calls++
		if id != int64(1) || row["name"] != "Admin" {
			return errors.New("hook received wrong row")
		}
		return hookErr
	})
	id, err := env.Model("res.partner").Create(map[string]any{"name": "Admin"})
	if !errors.Is(err, hookErr) || id != 0 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	found, err := env.Model("res.partner").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || found.Len() != 0 {
		t.Fatalf("calls=%d rows=%d", calls, found.Len())
	}
}

func TestAfterWriteHookRollsBackWrite(t *testing.T) {
	env := testEnv(t)
	id, err := env.Model("res.partner").Create(map[string]any{"name": "Admin"})
	if err != nil {
		t.Fatal(err)
	}
	hookErr := errors.New("write hook failed")
	calls := 0
	env.RegisterAfterWriteHook(func(env *Env, modelName string, hookID int64, oldRow map[string]any, newRow map[string]any, values map[string]any) error {
		if modelName != "res.partner" {
			return nil
		}
		calls++
		if hookID != id || oldRow["name"] != "Admin" || newRow["name"] != "Administrator" || values["name"] != "Administrator" {
			return errors.New("hook received wrong write rows")
		}
		if _, err := env.Model("res.partner").Create(map[string]any{"name": "Hook Side Effect"}); err != nil {
			return err
		}
		return hookErr
	})
	err = env.Model("res.partner").Browse(id).Write(map[string]any{"name": "Administrator"})
	if !errors.Is(err, hookErr) {
		t.Fatalf("err = %v", err)
	}
	rows, err := env.Model("res.partner").Browse(id).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	found, err := env.Model("res.partner").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(rows) != 1 || rows[0]["name"] != "Admin" || found.Len() != 1 {
		t.Fatalf("calls=%d rows=%+v found=%d", calls, rows, found.Len())
	}
}

func TestBeforeUnlinkHookRollsBackUnlink(t *testing.T) {
	env := testEnv(t)
	id, err := env.Model("res.partner").Create(map[string]any{"name": "Admin"})
	if err != nil {
		t.Fatal(err)
	}
	hookErr := errors.New("unlink hook failed")
	calls := 0
	env.RegisterBeforeUnlinkHook(func(env *Env, modelName string, hookID int64, row map[string]any) error {
		if modelName != "res.partner" {
			return nil
		}
		calls++
		if hookID != id || row["name"] != "Admin" {
			return errors.New("hook received wrong unlink row")
		}
		if _, err := env.Model("res.partner").Create(map[string]any{"name": "Hook Side Effect"}); err != nil {
			return err
		}
		return hookErr
	})
	err = env.Model("res.partner").Browse(id).Unlink()
	if !errors.Is(err, hookErr) {
		t.Fatalf("err = %v", err)
	}
	rows, err := env.Model("res.partner").Browse(id).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	found, err := env.Model("res.partner").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(rows) != 1 || rows[0]["name"] != "Admin" || found.Len() != 1 {
		t.Fatalf("calls=%d rows=%+v found=%d", calls, rows, found.Len())
	}
}

func TestAccountAccountCreateWriteClassifiesRootAndGroup(t *testing.T) {
	env := accountingRecordEnv(t)
	if _, err := env.Model("account.group").Create(map[string]any{"name": "Assets", "code_prefix_start": "10", "code_prefix_end": "19", "company_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	specificGroupID, err := env.Model("account.group").Create(map[string]any{"name": "Cash", "code_prefix_start": "101", "code_prefix_end": "101", "company_id": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := env.Model("account.account").Create(map[string]any{"code": "101200", "name": "Bank", "account_type": "asset_cash", "company_id": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("account.account").Browse(accountID).Read("placeholder_code", "root_id", "group_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["placeholder_code"] != "101200" || rows[0]["root_id"] != "10" || rows[0]["group_id"] != specificGroupID {
		t.Fatalf("created classification = %+v", rows[0])
	}

	if err := env.Model("account.account").Browse(accountID).Write(map[string]any{"code": "209000"}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.account").Browse(accountID).Read("placeholder_code", "root_id", "group_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["placeholder_code"] != "209000" || rows[0]["root_id"] != "20" || rows[0]["group_id"] != int64(0) {
		t.Fatalf("updated classification = %+v", rows[0])
	}
	if err := env.Model("account.account").Browse(accountID).Write(map[string]any{"code": "AB123"}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.account").Browse(accountID).Read("placeholder_code", "root_id", "group_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["placeholder_code"] != "AB123" || rows[0]["root_id"] != "AB" || rows[0]["group_id"] != int64(0) {
		t.Fatalf("alphanumeric classification = %+v", rows[0])
	}
}

func TestAccountAccountNormalizesNameTypeAndCodeUniqueness(t *testing.T) {
	env := accountingRecordEnv(t)
	parentCompanyID, err := env.Model("res.company").Create(map[string]any{"name": "Parent"})
	if err != nil {
		t.Fatal(err)
	}
	childCompanyID, err := env.Model("res.company").Create(map[string]any{"name": "Child", "parent_id": parentCompanyID})
	if err != nil {
		t.Fatal(err)
	}
	otherCompanyID, err := env.Model("res.company").Create(map[string]any{"name": "Other"})
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := env.Model("account.account").Create(map[string]any{"name": "550003 Existing Account", "account_type": string(coreaccounting.AccountIncome), "company_id": parentCompanyID, "tax_ids": []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("account.account").Browse(accountID).Read("code", "name", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["code"] != "550003" || rows[0]["name"] != "Existing Account" || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{1, 2}) {
		t.Fatalf("normalized account = %+v", rows[0])
	}
	if _, err := env.Model("account.account").Create(map[string]any{"code": "550003", "name": "Child Duplicate", "account_type": string(coreaccounting.AccountIncome), "company_id": childCompanyID}); !errors.Is(err, coreaccounting.ErrAccountCodeDuplicate) {
		t.Fatalf("child duplicate error = %v", err)
	}
	if _, err := env.Model("account.account").Create(map[string]any{"code": "550003", "name": "Other Company", "account_type": string(coreaccounting.AccountIncome), "company_id": otherCompanyID}); err != nil {
		t.Fatalf("unrelated duplicate error = %v", err)
	}
	if err := env.Model("account.account").Browse(accountID).Write(map[string]any{"account_type": string(coreaccounting.AccountOffBalance)}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.account").Browse(accountID).Read("account_type", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_type"] != string(coreaccounting.AccountOffBalance) || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{}) {
		t.Fatalf("off-balance account = %+v", rows[0])
	}
}

func TestAccountMoveLineNormalizesFiscalPositionProductAccountAndTaxes(t *testing.T) {
	env := accountingRecordEnv(t)
	incomeID := createRecordAccount(t, env, "4000", "Income", coreaccounting.AccountIncome)
	mappedIncomeID := createRecordAccount(t, env, "4010", "Foreign Income", coreaccounting.AccountIncome)
	expenseID := createRecordAccount(t, env, "5000", "Expense", coreaccounting.AccountExpense)
	mappedExpenseID := createRecordAccount(t, env, "5010", "Foreign Expense", coreaccounting.AccountExpense)
	receivableID := createRecordAccount(t, env, "1100", "Receivable", coreaccounting.AccountReceivable)
	mappedReceivableID := createRecordAccount(t, env, "1110", "Foreign Receivable", coreaccounting.AccountReceivable)
	saleTaxID := createRecordTax(t, env, "Sale Tax", 1, nil, nil)
	mappedSaleTaxID := createRecordTax(t, env, "Mapped Sale Tax", 1, []int64{saleTaxID}, nil)
	secondMappedSaleTaxID := createRecordTax(t, env, "Second Mapped Sale Tax", 1, []int64{saleTaxID}, nil)
	purchaseTaxID := createRecordTax(t, env, "Purchase Tax", 1, nil, nil)
	mappedPurchaseTaxID := createRecordTax(t, env, "Mapped Purchase Tax", 1, []int64{purchaseTaxID}, nil)
	wrongCompanyTaxID := createRecordTax(t, env, "Wrong Company Tax", 2, nil, nil)
	fiscalPositionID, err := env.Model("account.fiscal.position").Create(map[string]any{"name": "Export", "company_id": int64(1), "tax_ids": []int64{mappedSaleTaxID, secondMappedSaleTaxID, mappedPurchaseTaxID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range []struct {
		source int64
		dest   int64
	}{
		{incomeID, mappedIncomeID},
		{expenseID, mappedExpenseID},
		{receivableID, mappedReceivableID},
	} {
		if _, err := env.Model("account.fiscal.position.account").Create(map[string]any{"position_id": fiscalPositionID, "company_id": int64(1), "account_src_id": mapping.source, "account_dest_id": mapping.dest}); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.Model("account.tax").Browse(mappedSaleTaxID).Write(map[string]any{"fiscal_position_ids": []int64{fiscalPositionID}}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.tax").Browse(secondMappedSaleTaxID).Write(map[string]any{"fiscal_position_ids": []int64{fiscalPositionID}}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.tax").Browse(mappedPurchaseTaxID).Write(map[string]any{"fiscal_position_ids": []int64{fiscalPositionID}}); err != nil {
		t.Fatal(err)
	}
	categoryID, err := env.Model("product.category").Create(map[string]any{"name": "All", "property_account_income_categ_id": incomeID, "property_account_expense_categ_id": expenseID})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("product.template").Create(map[string]any{
		"name":              "Service",
		"categ_id":          categoryID,
		"taxes_id":          []int64{saleTaxID, wrongCompanyTaxID},
		"supplier_taxes_id": []int64{purchaseTaxID},
	})
	if err != nil {
		t.Fatal(err)
	}
	productID, err := env.Model("product.product").Create(map[string]any{"name": "Service", "product_tmpl_id": templateID, "categ_id": categoryID})
	if err != nil {
		t.Fatal(err)
	}
	moveID, err := env.Model("account.move").Create(map[string]any{"name": "INV/1", "date": "2026-01-01", "state": "draft", "move_type": "out_invoice", "company_id": int64(1), "fiscal_position_id": fiscalPositionID})
	if err != nil {
		t.Fatal(err)
	}
	lineID, err := env.Model("account.move.line").Create(map[string]any{"move_id": moveID, "product_id": productID, "company_id": int64(1), "account_id": incomeID, "account_type": string(coreaccounting.AccountIncome)})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("account.move.line").Browse(lineID).Read("account_id", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_id"] != mappedIncomeID || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{mappedSaleTaxID, secondMappedSaleTaxID}) {
		t.Fatalf("sale product line = %+v", rows[0])
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"fiscal_position_id": int64(0)}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.move.line").Browse(lineID).Read("account_id", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_id"] != incomeID || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{saleTaxID}) {
		t.Fatalf("line after fiscal position removal = %+v", rows[0])
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"fiscal_position_id": fiscalPositionID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.move.line").Browse(lineID).Read("account_id", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_id"] != mappedIncomeID || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{mappedSaleTaxID, secondMappedSaleTaxID}) {
		t.Fatalf("line after fiscal position restore = %+v", rows[0])
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"move_type": "in_invoice"}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.move.line").Browse(lineID).Read("account_id", "tax_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_id"] != mappedExpenseID || !reflect.DeepEqual(rows[0]["tax_ids"], []int64{mappedPurchaseTaxID}) {
		t.Fatalf("purchase product line = %+v", rows[0])
	}
	termLineID, err := env.Model("account.move.line").Create(map[string]any{"move_id": moveID, "company_id": int64(1), "account_id": receivableID, "account_type": string(coreaccounting.AccountReceivable)})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.move.line").Browse(termLineID).Read("account_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["account_id"] != mappedReceivableID {
		t.Fatalf("payment term line = %+v", rows[0])
	}
}

func TestAccountLockExceptionNormalizesCreateAndRevoke(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "tax_lock_date": "2026-03-31"})
	if err != nil {
		t.Fatal(err)
	}
	lockID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(7), "tax_lock_date": "2026-01-31"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("account.lock_exception").Browse(lockID).Read("active", "state", "lock_date_field", "lock_date", "company_lock_date")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true || rows[0]["state"] != string(coreaccounting.LockExceptionActive) || rows[0]["lock_date_field"] != string(coreaccounting.LockTax) || !recordDateValue(rows[0]["lock_date"]).Equal(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)) || !recordDateValue(rows[0]["company_lock_date"]).Equal(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("normalized lock exception = %+v", rows[0])
	}
	if _, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "tax_lock_date": "2026-01-31", "sale_lock_date": "2026-01-31"}); !errors.Is(err, coreaccounting.ErrLockExceptionFields) {
		t.Fatalf("multiple lock fields error = %v", err)
	}
	directID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "lock_date_field": string(coreaccounting.LockTax), "lock_date": "2026-01-15"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.lock_exception").Browse(directID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.lock_exception").Browse(directID).Read("active", "state", "end_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != false || rows[0]["state"] != string(coreaccounting.LockExceptionRevoked) || recordDateValue(rows[0]["end_datetime"]).IsZero() {
		t.Fatalf("revoked lock exception = %+v", rows[0])
	}
	removalID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(7), "tax_lock_date": false})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("account.lock_exception").Browse(removalID).Read("active", "state", "lock_date_field", "lock_date", "company_lock_date")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true || rows[0]["state"] != string(coreaccounting.LockExceptionActive) || rows[0]["lock_date_field"] != string(coreaccounting.LockTax) || !recordDateValue(rows[0]["lock_date"]).IsZero() || !recordDateValue(rows[0]["company_lock_date"]).Equal(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("removal lock exception = %+v", rows[0])
	}
	if _, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "tax_lock_date": false, "sale_lock_date": "2026-01-31"}); !errors.Is(err, coreaccounting.ErrLockExceptionFields) {
		t.Fatalf("false plus second lock field error = %v", err)
	}
}

func TestAccountMoveLifecycleUsesEffectiveLockPolicy(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/LOCK/1")
	unlinkID := createRecordPostedMove(t, env, companyID, "INV/LOCK/2")
	hardID := createRecordPostedMove(t, env, companyID, "INV/LOCK/3")
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": "2026-03-31"}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MoveDraft)}); !errors.Is(err, coreaccounting.ErrFiscalLockDate) {
		t.Fatalf("reset without exception error = %v", err)
	}
	lockID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2025-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MoveDraft)}); err != nil {
		t.Fatalf("reset with exception error = %v", err)
	}
	if err := env.WithAccountMovePost().Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MovePosted)}); err != nil {
		t.Fatalf("restore posted with exception error = %v", err)
	}
	if err := env.Model("account.move").Browse(unlinkID).Unlink(); err != nil {
		t.Fatalf("unlink with exception error = %v", err)
	}
	if err := env.Model("account.lock_exception").Browse(lockID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MoveCancel)}); !errors.Is(err, coreaccounting.ErrFiscalLockDate) {
		t.Fatalf("cancel with revoked exception error = %v", err)
	}
	if _, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": false}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MoveCancel)}); err != nil {
		t.Fatalf("cancel with lock-removal exception error = %v", err)
	}
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"hard_lock_date": "2026-03-31"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "fiscalyear_lock_date": "2025-12-31"}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(hardID).Write(map[string]any{"state": string(coreaccounting.MoveDraft)}); !errors.Is(err, coreaccounting.ErrHardLockDate) {
		t.Fatalf("reset with hard lock error = %v", err)
	}
}

func TestAccountMoveRestrictiveAuditTrailFromCompany(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "restrictive_audit_trail": true})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AUDIT/MOVE")
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MoveDraft)}); !errors.Is(err, coreaccounting.ErrPostedMoveProtected) {
		t.Fatalf("restrictive reset error = %v", err)
	}
	if err := env.Model("account.move").Browse(moveID).Unlink(); !errors.Is(err, coreaccounting.ErrPostedMoveProtected) {
		t.Fatalf("restrictive unlink error = %v", err)
	}
}

func TestRestrictedAuditTrailAttachmentUnlinkAndWrite(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "restrictive_audit_trail": true})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AUDIT/PDF")
	pdfID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":       "INV-AUDIT.pdf",
		"res_model":  "account.move",
		"res_field":  "invoice_pdf_report_file",
		"res_id":     moveID,
		"company_id": companyID,
		"mimetype":   "application/pdf",
		"datas":      []byte("%PDF-1.4\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("ir.attachment").Browse(pdfID).Unlink(); err != nil {
		t.Fatalf("first official PDF unlink error = %v", err)
	}
	rows, err := env.Model("ir.attachment").Browse(pdfID).Read("name", "res_model", "res_field", "res_id", "datas")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["res_model"] != "account.move" || rows[0]["res_id"] != moveID || stringValue(rows[0]["res_field"]) != "" || !strings.Contains(stringValue(rows[0]["name"]), "detached by user 1 on") || string(rows[0]["datas"].([]byte)) != "%PDF-1.4\n" {
		t.Fatalf("detached attachment = %+v", rows)
	}
	if err := env.Model("ir.attachment").Browse(pdfID).Unlink(); err == nil || !strings.Contains(err.Error(), "restricted audit trail") {
		t.Fatalf("second unlink error = %v", err)
	}
	if err := env.Model("ir.attachment").Browse(pdfID).Write(map[string]any{"datas": []byte("%PDF-1.5\n")}); err == nil || !strings.Contains(err.Error(), "restricted audit trail") {
		t.Fatalf("protected content write error = %v", err)
	}
	if err := env.Model("ir.attachment").Browse(pdfID).Write(map[string]any{"name": "renamed.pdf"}); err != nil {
		t.Fatalf("metadata write error = %v", err)
	}
	if err := env.Model("ir.attachment").Browse(pdfID).Write(map[string]any{"res_model": "documents.document", "res_id": int64(99)}); err != nil {
		t.Fatalf("documents relink write error = %v", err)
	}
	rows, err = env.Model("ir.attachment").Browse(pdfID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["res_model"] != "account.move" || rows[0]["res_id"] != moveID {
		t.Fatalf("documents relink mutated protected owner = %+v", rows)
	}
}

func TestRestrictedAuditTrailXMLAttachmentUnlinkDetaches(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "restrictive_audit_trail": true})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AUDIT/XML")
	xmlID, err := env.Model("ir.attachment").Create(map[string]any{
		"name":       "invoice.xml",
		"res_model":  "account.move",
		"res_field":  "ubl_cii_xml_file",
		"res_id":     moveID,
		"company_id": companyID,
		"mimetype":   "application/xml",
		"datas":      []byte("<?xml version=\"1.0\"?><invoice/>"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("ir.attachment").Browse(xmlID).Unlink(); err != nil {
		t.Fatalf("first XML unlink error = %v", err)
	}
	rows, err := env.Model("ir.attachment").Browse(xmlID).Read("res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || stringValue(rows[0]["res_field"]) != "" {
		t.Fatalf("detached XML attachment = %+v", rows)
	}
}

func TestAccountMoveDirectPostRequiresAction(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("account.move").Create(map[string]any{
		"name":       "INV/DIRECT/CREATE",
		"date":       "2026-01-15",
		"state":      string(coreaccounting.MovePosted),
		"move_type":  "out_invoice",
		"company_id": companyID,
		"auto_post":  "no",
	}); !errors.Is(err, coreaccounting.ErrMovePostRequiresAction) {
		t.Fatalf("direct posted create error = %v", err)
	}
	moveID, err := env.Model("account.move").Create(map[string]any{
		"name":       "INV/DIRECT/WRITE",
		"date":       "2026-01-15",
		"state":      string(coreaccounting.MoveDraft),
		"move_type":  "out_invoice",
		"company_id": companyID,
		"auto_post":  "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move").Browse(moveID).Write(map[string]any{"state": string(coreaccounting.MovePosted)}); !errors.Is(err, coreaccounting.ErrMovePostRequiresAction) {
		t.Fatalf("direct posted write error = %v", err)
	}
}

func TestAccountMoveLineWriteChecksOnlyOdooProtectedLockFields(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AML/LOCK")
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": "2026-03-31"}); err != nil {
		t.Fatal(err)
	}
	lineID := recordMoveLineIDs(t, env, moveID)[1]
	if err := env.Model("account.move.line").Browse(lineID).Write(map[string]any{"name": "memo"}); err != nil {
		t.Fatalf("nonprotected line write error = %v", err)
	}
	if err := env.Model("account.move.line").Browse(lineID).Write(map[string]any{"partner_id": int64(9)}); !errors.Is(err, coreaccounting.ErrFiscalLockDate) {
		t.Fatalf("fiscal protected line write error = %v", err)
	}
}

func TestAccountMoveLineWriteChecksTaxLockPerAffectedLine(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AML/TAX")
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"tax_lock_date": "2026-03-31"}); err != nil {
		t.Fatal(err)
	}
	lineIDs := recordMoveLineIDs(t, env, moveID)
	receivableLineID := lineIDs[0]
	incomeLineID := lineIDs[1]
	taxID := createRecordTax(t, env, "VAT", companyID, nil, nil)
	env.stores["account.move.line"].records[incomeLineID]["tax_ids"] = []int64{taxID}
	if err := env.Model("account.move.line").Browse(receivableLineID).Write(map[string]any{"balance": int64(10000)}); err != nil {
		t.Fatalf("untaxed protected line write error = %v", err)
	}
	if err := env.Model("account.move.line").Browse(incomeLineID).Write(map[string]any{"balance": int64(-10000)}); !errors.Is(err, coreaccounting.ErrTaxLockDate) {
		t.Fatalf("taxed protected line write error = %v", err)
	}
	if err := env.Model("account.move.line").Browse(incomeLineID).Write(map[string]any{"tax_ids": []int64{}}); !errors.Is(err, coreaccounting.ErrPostedLineTaxImmutable) {
		t.Fatalf("posted tax field write error = %v", err)
	}
}

func TestAccountMoveLineWriteUnlinkPostsTrackingMessages(t *testing.T) {
	env := accountingRecordEnv(t)
	registerMailMessageModel(t, env)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	moveID := createRecordPostedMove(t, env, companyID, "INV/AML/TRACK")
	lineIDs := recordMoveLineIDs(t, env, moveID)
	incomeLineID := lineIDs[1]
	messages := accountMoveTrackingMessages(t, env, moveID)
	if len(messages) != 2 || !strings.Contains(stringValue(messages[0]["body"]), "created") || !strings.Contains(stringValue(messages[1]["body"]), "created") {
		t.Fatalf("create tracking messages = %+v", messages)
	}
	trackingValues := accountMoveTrackingValues(t, env, int64Values(messages[0]["tracking_value_ids"]))
	if len(trackingValues) == 0 || trackingValues[0]["mail_message_id"] != messages[0]["id"] || trackingValues[0]["field_name"] != "account_id" {
		t.Fatalf("create tracking values = %+v messages=%+v", trackingValues, messages)
	}
	if err := env.Model("account.move.line").Browse(incomeLineID).Write(map[string]any{"name": "Tracked memo"}); err != nil {
		t.Fatal(err)
	}
	messages = accountMoveTrackingMessages(t, env, moveID)
	if len(messages) != 3 || messages[2]["message_type"] != "notification" || messages[2]["body_is_html"] != true {
		t.Fatalf("write tracking messages = %+v", messages)
	}
	body := stringValue(messages[2]["body"])
	if !strings.Contains(body, "Journal Item #") || !strings.Contains(body, "updated") || !strings.Contains(body, "Label:") || !strings.Contains(body, "Tracked memo") {
		t.Fatalf("write tracking body = %s", body)
	}
	writeTrackingValues := accountMoveTrackingValues(t, env, int64Values(messages[2]["tracking_value_ids"]))
	if len(writeTrackingValues) != 1 || writeTrackingValues[0]["field_name"] != "name" || writeTrackingValues[0]["new_value_char"] != "Tracked memo" {
		t.Fatalf("write tracking values = %+v", writeTrackingValues)
	}
	quietEnv := env.WithContext(Context{UserID: 1, CompanyID: companyID, CompanyIDs: []int64{companyID}, Values: map[string]any{"tracking_disable": true}})
	if err := quietEnv.Model("account.move.line").Browse(incomeLineID).Write(map[string]any{"name": "Quiet memo"}); err != nil {
		t.Fatal(err)
	}
	if messages := accountMoveTrackingMessages(t, env, moveID); len(messages) != 3 {
		t.Fatalf("tracking_disable messages = %+v", messages)
	}
	if err := env.Model("account.move.line").Browse(lineIDs[0]).Unlink(); err != nil {
		t.Fatal(err)
	}
	messages = accountMoveTrackingMessages(t, env, moveID)
	if len(messages) != 4 {
		t.Fatalf("write+unlink tracking messages = %+v", messages)
	}
	deleteBody := stringValue(messages[3]["body"])
	if !strings.Contains(deleteBody, "deleted") || !strings.Contains(deleteBody, "Account:") {
		t.Fatalf("delete tracking body = %s", deleteBody)
	}
}

func TestMailMessageCreateProcessesTrackingCommands(t *testing.T) {
	env := accountingRecordEnv(t)
	registerMailMessageModel(t, env)

	messageID, err := env.Model("mail.message").Create(map[string]any{
		"body":         "Tracked",
		"message_type": "notification",
		"model":        "res.partner",
		"res_id":       int64(42),
		"tracking_value_ids": []any{[]any{int64(0), int64(0), map[string]any{
			"field_name":     "name",
			"field_desc":     "Name",
			"field_type":     "char",
			"old_value_char": "Old",
			"new_value_char": "New",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := env.Model("mail.message").Browse(messageID).Read("tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	trackingIDs := int64Values(messages[0]["tracking_value_ids"])
	if len(trackingIDs) != 1 {
		t.Fatalf("tracking ids = %+v", messages)
	}
	rows, err := env.Model("mail.tracking.value").Browse(trackingIDs...).Read("field_name", "field_desc", "field_type", "old_value_char", "new_value_char", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["field_name"] != "name" || rows[0]["field_desc"] != "Name" || rows[0]["field_type"] != "char" || rows[0]["old_value_char"] != "Old" || rows[0]["new_value_char"] != "New" || rows[0]["mail_message_id"] != messageID {
		t.Fatalf("tracking rows = %+v", rows)
	}
}

func TestMailMessageCreateRollsBackTrackingCommandFailure(t *testing.T) {
	env := accountingRecordEnv(t)
	registerMailMessageModel(t, env)

	if _, err := env.Model("mail.message").Create(map[string]any{
		"body":         "Tracked",
		"message_type": "notification",
		"model":        "res.partner",
		"res_id":       int64(42),
		"tracking_value_ids": []any{[]any{int64(0), int64(0), map[string]any{
			"missing_field": "name",
		}}},
	}); err == nil || !strings.Contains(err.Error(), "unknown field mail.tracking.value.missing_field") {
		t.Fatalf("create error = %v", err)
	}
	if rows, err := env.Model("mail.message").Search(domain.And()); err != nil || rows.Len() != 0 {
		t.Fatalf("mail message rows after rollback = len %d err %v", rows.Len(), err)
	}
	if rows, err := env.Model("mail.tracking.value").Search(domain.And()); err != nil || rows.Len() != 0 {
		t.Fatalf("tracking rows after rollback = len %d err %v", rows.Len(), err)
	}
	messageID, err := env.Model("mail.message").Create(map[string]any{
		"body":         "Tracked after rollback",
		"message_type": "notification",
		"model":        "res.partner",
		"res_id":       int64(42),
		"tracking_value_ids": []any{[]any{int64(0), int64(0), map[string]any{
			"field_name": "name",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if messageID != 1 {
		t.Fatalf("message id after rollback = %d", messageID)
	}
	messages, err := env.Model("mail.message").Browse(messageID).Read("tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	trackingIDs := int64Values(messages[0]["tracking_value_ids"])
	if len(trackingIDs) != 1 || trackingIDs[0] != 1 {
		t.Fatalf("tracking ids after rollback = %+v", trackingIDs)
	}
}

func TestMailMessageDirectWriteAllowsTrackedContent(t *testing.T) {
	env := accountingRecordEnv(t)
	registerMailMessageModel(t, env)

	trackedID, err := env.Model("mail.message").Create(map[string]any{
		"body":         "Tracked",
		"message_type": "comment",
		"model":        "res.partner",
		"res_id":       int64(42),
		"tracking_value_ids": []any{[]any{int64(0), int64(0), map[string]any{
			"field_name": "name",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("mail.message").Browse(trackedID).Write(map[string]any{"body": "Changed"}); err != nil {
		t.Fatalf("tracked direct write error = %v", err)
	}
	rows, err := env.Model("mail.message").Browse(trackedID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["body"] != "Changed" {
		t.Fatalf("body after direct write = %+v", rows)
	}
}

func TestCompanySoftLockWriteRecreatesActiveExceptions(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "fiscalyear_lock_date": "2020-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	revokedID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2010-01-01", "reason": "revoked"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.lock_exception").Browse(revokedID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	activeID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2010-01-01", "reason": "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": "2021-01-01"}); err != nil {
		t.Fatal(err)
	}
	inactiveEnv := env.WithContext(Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"active_test": false}})
	found, err := inactiveEnv.Model("account.lock_exception").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "active", "state", "reason", "lock_date_field", "lock_date", "company_lock_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("exception count = %d rows=%+v", len(rows), rows)
	}
	byID := map[int64]map[string]any{}
	var recreated map[string]any
	for _, row := range rows {
		id := row["id"].(int64)
		byID[id] = row
		if id != revokedID && id != activeID {
			recreated = row
		}
	}
	if byID[revokedID]["active"] != false || !recordDateValue(byID[revokedID]["company_lock_date"]).Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("revoked exception changed = %+v", byID[revokedID])
	}
	if byID[activeID]["active"] != false || byID[activeID]["state"] != string(coreaccounting.LockExceptionRevoked) {
		t.Fatalf("active exception not revoked = %+v", byID[activeID])
	}
	if recreated == nil || recreated["active"] != true || recreated["reason"] != "active" || recreated["lock_date_field"] != string(coreaccounting.LockFiscalYear) || !recordDateValue(recreated["lock_date"]).Equal(time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)) || !recordDateValue(recreated["company_lock_date"]).Equal(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("recreated exception = %+v", recreated)
	}
}

func TestCompanySoftLockWriteRecreatesRemovalExceptions(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "fiscalyear_lock_date": "2020-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	activeID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": false, "reason": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": "2021-01-01"}); err != nil {
		t.Fatal(err)
	}
	inactiveEnv := env.WithContext(Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"active_test": false}})
	found, err := inactiveEnv.Model("account.lock_exception").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "active", "state", "reason", "lock_date_field", "lock_date", "company_lock_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("exception count = %d rows=%+v", len(rows), rows)
	}
	for _, row := range rows {
		if row["id"].(int64) == activeID {
			if row["active"] != false || row["state"] != string(coreaccounting.LockExceptionRevoked) {
				t.Fatalf("original removal exception = %+v", row)
			}
			continue
		}
		if row["active"] != true || row["reason"] != "remove" || row["lock_date_field"] != string(coreaccounting.LockFiscalYear) || !recordDateValue(row["lock_date"]).IsZero() || !recordDateValue(row["company_lock_date"]).Equal(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("recreated removal exception = %+v", row)
		}
	}
}

func TestCompanySoftLockWriteRollbackRestoresLockExceptionsOnRecreateFailure(t *testing.T) {
	env := accountingRecordEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Company", "fiscalyear_lock_date": "2020-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2010-01-01", "reason": "first"})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2010-01-01", "reason": "second"})
	if err != nil {
		t.Fatal(err)
	}
	nextBefore := env.stores["account.lock_exception"].nextID
	sentinel := errors.New("deny second replacement")
	env.WithPolicy(&denyNthCreatePolicy{modelName: "account.lock_exception", allowedCreates: 1, err: sentinel})
	err = env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": "2021-01-01"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("company write error = %v", err)
	}
	env.WithPolicy(nil)
	companyRows, err := env.Model("res.company").Browse(companyID).Read("fiscalyear_lock_date")
	if err != nil {
		t.Fatal(err)
	}
	if !recordDateValue(companyRows[0]["fiscalyear_lock_date"]).Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("company lock date = %+v", companyRows[0])
	}
	found, err := env.Model("account.lock_exception").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "active", "state", "reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("exception count = %d rows=%+v", len(rows), rows)
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[row["id"].(int64)] = row
	}
	for _, id := range []int64{firstID, secondID} {
		row := byID[id]
		if row == nil || row["active"] != true || row["state"] != string(coreaccounting.LockExceptionActive) {
			t.Fatalf("restored exception %d = %+v", id, row)
		}
	}
	if got := env.stores["account.lock_exception"].nextID; got != nextBefore {
		t.Fatalf("nextID = %d, want %d", got, nextBefore)
	}
	nextID, err := env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(1), "fiscalyear_lock_date": "2010-01-01", "reason": "after rollback"})
	if err != nil {
		t.Fatal(err)
	}
	if nextID != nextBefore {
		t.Fatalf("next create id = %d, want %d", nextID, nextBefore)
	}
}

func TestRecordSearchOdooDomainOperators(t *testing.T) {
	env := testEnv(t)
	partners := env.Model("res.partner")
	first, err := partners.Create(map[string]any{"name": "Administrator", "active": true, "age": 40, "tag_ids": []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partners.Create(map[string]any{"name": "Guest", "active": false, "age": 20, "tag_ids": []int64{3}}); err != nil {
		t.Fatal(err)
	}

	node, err := domain.Parse([]any{
		"&",
		[]any{"name", "ilike", "admin"},
		[]any{"tag_ids", "in", []any{float64(2)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	found, err := partners.Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 || found.IDs()[0] != first {
		t.Fatalf("found = %+v", found.IDs())
	}

	found, err = partners.Search(domain.Cond("tag_ids", "=", int64(2)))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 || found.IDs()[0] != first {
		t.Fatalf("membership found = %+v", found.IDs())
	}

	node, err = domain.Parse([]any{
		[]any{"age", ">=", float64(21)},
		[]any{"active", "=?", false},
	})
	if err != nil {
		t.Fatal(err)
	}
	found, err = partners.Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 || found.IDs()[0] != first {
		t.Fatalf("comparison found = %+v", found.IDs())
	}
}

func TestRecordSearchOrder(t *testing.T) {
	registry := NewRegistry()
	company := model.New("res.company", "res_company")
	company.Order = "name"
	company.AddField(field.New("name", field.Char))
	if err := registry.Register(company); err != nil {
		t.Fatal(err)
	}
	partner := model.New("res.partner", "res_partner")
	partner.Order = "name desc"
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	partner.AddField(field.New("age", field.Int))
	partner.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	partner.AddField(field.New("tag_ids", field.Many2Many).WithRelation("res.partner.category"))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	zuluID, err := env.Model("res.company").Create(map[string]any{"name": "Zulu"})
	if err != nil {
		t.Fatal(err)
	}
	alphaID, err := env.Model("res.company").Create(map[string]any{"name": "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	partners := env.Model("res.partner")
	for _, values := range []map[string]any{
		{"name": "Beta", "active": true, "age": int64(30), "company_id": zuluID},
		{"name": "Delta", "active": false, "age": int64(20), "company_id": alphaID},
		{"name": "Alpha", "active": true, "age": int64(30)},
	} {
		if _, err := partners.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	partners = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}}).Model("res.partner")

	found, err := partners.Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if names := []any{rows[0]["name"], rows[1]["name"], rows[2]["name"]}; !reflect.DeepEqual(names, []any{"Delta", "Beta", "Alpha"}) {
		t.Fatalf("default ordered rows = %#v", rows)
	}
	found, err = partners.SearchWithOptions(domain.And(), SearchOptions{Order: "age asc, name:lower desc"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if names := []any{rows[0]["name"], rows[1]["name"], rows[2]["name"]}; !reflect.DeepEqual(names, []any{"Delta", "Beta", "Alpha"}) {
		t.Fatalf("explicit ordered rows = %#v", rows)
	}
	found, err = partners.SearchWithOptions(domain.And(), SearchOptions{Order: "company_id"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Delta" || rows[1]["name"] != "Beta" || rows[2]["name"] != "Alpha" {
		t.Fatalf("many2one ordered rows = %#v", rows)
	}
	found, err = partners.SearchWithOptions(domain.And(), SearchOptions{Order: "company_id.id"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Beta" || rows[1]["name"] != "Delta" || rows[2]["name"] != "Alpha" {
		t.Fatalf("many2one id ordered rows = %#v", rows)
	}
	found, err = partners.SearchWithOptions(domain.And(), SearchOptions{Order: "company_id nulls first"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Alpha" {
		t.Fatalf("nulls-first ordered rows = %#v", rows)
	}
	found, err = partners.SearchWithOptions(domain.And(), SearchOptions{Order: "name asc", Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = found.Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Beta" {
		t.Fatalf("paged ordered rows = %#v", rows)
	}
	if _, err := partners.SearchWithOptions(domain.And(), SearchOptions{Order: "missing desc"}); err == nil || !strings.Contains(err.Error(), "invalid search order field") {
		t.Fatalf("missing order field error = %v", err)
	}
	if _, err := partners.SearchWithOptions(domain.And(), SearchOptions{Order: "company_id.name"}); err == nil || !strings.Contains(err.Error(), "invalid search order term") {
		t.Fatalf("many2one dotted order field error = %v", err)
	}
	if _, err := partners.SearchWithOptions(domain.And(), SearchOptions{Order: "tag_ids"}); err == nil || !strings.Contains(err.Error(), "relational field") {
		t.Fatalf("relational order field error = %v", err)
	}
}

func TestRecordSearchActiveTest(t *testing.T) {
	registry := NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	country := model.New("x.country", "x_country")
	country.AddField(field.New("name", field.Char))
	country.AddField(field.New("x_active", field.Bool))
	if err := registry.Register(country); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	partners := env.Model("res.partner")
	activeID, err := partners.Create(map[string]any{"name": "Visible", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	archivedID, err := partners.Create(map[string]any{"name": "Archived", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	missingActiveID, err := partners.Create(map[string]any{"name": "Implicit"})
	if err != nil {
		t.Fatal(err)
	}

	found, err := partners.SearchWithOptions(domain.And(), SearchOptions{Order: "id"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found.IDs(), []int64{activeID, missingActiveID}) {
		t.Fatalf("default active ids = %+v", found.IDs())
	}
	found, err = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}}).Model("res.partner").SearchWithOptions(domain.And(), SearchOptions{Order: "id"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found.IDs(), []int64{activeID, archivedID, missingActiveID}) {
		t.Fatalf("active_test false ids = %+v", found.IDs())
	}
	found, err = partners.SearchWithOptions(domain.Cond("active", domain.Equal, false), SearchOptions{Order: "id"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found.IDs(), []int64{archivedID, missingActiveID}) {
		t.Fatalf("explicit active domain ids = %+v", found.IDs())
	}
	pairs, err := partners.NameSearch("Archived", domain.And(), domain.ILike, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Fatalf("default active name_search = %+v", pairs)
	}
	pairs, err = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}}).Model("res.partner").NameSearch("Archived", domain.And(), domain.ILike, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0][0] != archivedID {
		t.Fatalf("active_test false name_search = %+v", pairs)
	}
	groups, err := partners.ReadGroup(domain.Cond("name", domain.NotEqual, "Implicit"), ReadGroupOptions{GroupBy: []string{"active"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0]["active"] != true || groups[0]["active_count"] != 1 {
		t.Fatalf("default active read_group = %+v", groups)
	}

	countries := env.Model("x.country")
	ussrID, err := countries.Create(map[string]any{"name": "USSR", "x_active": false})
	if err != nil {
		t.Fatal(err)
	}
	if found, err = countries.Search(domain.Cond("name", domain.Equal, "USSR")); err != nil || found.Len() != 0 {
		t.Fatalf("default x_active search ids = %+v err=%v", found.IDs(), err)
	}
	if found, err = countries.Search(domain.And(domain.Cond("name", domain.Equal, "USSR"), domain.Cond("x_active", domain.Equal, false))); err != nil || !reflect.DeepEqual(found.IDs(), []int64{ussrID}) {
		t.Fatalf("explicit x_active search ids = %+v err=%v", found.IDs(), err)
	}
}

func TestResGroupsUserTypeForcesRestrictedAccess(t *testing.T) {
	registry := NewRegistry()
	category := model.New("ir.module.category", "ir_module_category")
	category.AddField(field.New("name", field.Char))
	if err := registry.Register(category); err != nil {
		t.Fatal(err)
	}
	groups := model.New("res.groups", "res_groups")
	groups.AddField(field.New("name", field.Char))
	groups.AddField(field.New("category_id", field.Many2One).WithRelation("ir.module.category"))
	groups.AddField(field.New("restricted_access", field.Bool))
	if err := registry.Register(groups); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1})
	userTypeID, err := env.Model("ir.module.category").Create(map[string]any{"name": "User Type"})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{
		"name":              "Role / User",
		"category_id":       userTypeID,
		"restricted_access": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.groups").Browse(groupID).Read("restricted_access")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["restricted_access"] != true {
		t.Fatalf("restricted access = %+v", rows[0])
	}
	if err := env.Model("res.groups").Browse(groupID).Write(map[string]any{"restricted_access": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.groups").Browse(groupID).Read("restricted_access")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["restricted_access"] != true {
		t.Fatalf("restricted access after write = %+v", rows[0])
	}
}

func TestResGroupsDerivedHierarchyFields(t *testing.T) {
	env := securityDerivedFieldsEnv(t)
	categoryID, err := env.Model("ir.module.category").Create(map[string]any{"name": "Master Data"})
	if err != nil {
		t.Fatal(err)
	}
	privilegeID, err := env.Model("res.groups.privilege").Create(map[string]any{
		"name":        "Export",
		"description": "Export data",
		"sequence":    int64(10),
		"category_id": categoryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	lowerPrivilegeID, err := env.Model("res.groups.privilege").Create(map[string]any{
		"name":        "Lower",
		"description": "Lower sequence privilege",
		"sequence":    int64(5),
		"category_id": categoryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	userGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / User"})
	if err != nil {
		t.Fatal(err)
	}
	portalGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Portal"})
	if err != nil {
		t.Fatal(err)
	}
	publicGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Public"})
	if err != nil {
		t.Fatal(err)
	}
	systemGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Administrator", "implied_ids": []int64{userGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	baseExportGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Basic", "privilege_id": privilegeID, "sequence": int64(99)})
	if err != nil {
		t.Fatal(err)
	}
	exportGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Allowed", "privilege_id": privilegeID, "sequence": int64(1), "implied_ids": []int64{baseExportGroupID, userGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	lowerGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Lower Allowed", "privilege_id": lowerPrivilegeID})
	if err != nil {
		t.Fatal(err)
	}
	for key, id := range map[string]int64{
		"group_user":   userGroupID,
		"group_portal": portalGroupID,
		"group_public": publicGroupID,
	} {
		if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": key, "model": "res.groups", "res_id": id}); err != nil {
			t.Fatal(err)
		}
	}
	userID, err := env.Model("res.users").Create(map[string]any{"login": "manager", "name": "Manager", "active": true, "groups_id": []int64{systemGroupID}})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := env.Model("res.groups").Browse(userGroupID, exportGroupID).Read("full_name", "all_implied_ids", "all_implied_by_ids", "disjoint_ids", "all_user_ids", "all_users_count", "view_group_hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	userGroup := rows[0]
	if !containsInt64(userGroup["all_implied_by_ids"], systemGroupID) || !containsInt64(userGroup["all_implied_by_ids"], exportGroupID) {
		t.Fatalf("user group all_implied_by_ids = %+v", userGroup)
	}
	if !containsInt64(userGroup["disjoint_ids"], portalGroupID) || !containsInt64(userGroup["disjoint_ids"], publicGroupID) {
		t.Fatalf("user group disjoint_ids = %+v", userGroup)
	}
	if !containsInt64(userGroup["all_user_ids"], userID) || userGroup["all_users_count"] != int64(1) {
		t.Fatalf("user group all users = %+v", userGroup)
	}
	exportGroup := rows[1]
	if exportGroup["full_name"] != "Export / Allowed" || !containsInt64(exportGroup["all_implied_ids"], userGroupID) {
		t.Fatalf("export group derived fields = %+v", exportGroup)
	}
	hierarchy := exportGroup["view_group_hierarchy"].(map[string]any)
	privileges := hierarchy["privileges"].(map[int64]any)
	privilege := privileges[privilegeID].(map[string]any)
	privilegeGroupIDs := int64Values(privilege["group_ids"])
	if privilege["placeholder"] != "No" || len(privilegeGroupIDs) < 2 || privilegeGroupIDs[0] != baseExportGroupID || privilegeGroupIDs[1] != exportGroupID {
		t.Fatalf("hierarchy privilege = %+v", privilege)
	}
	categories := hierarchy["categories"].([]any)
	category := categories[0].(map[string]any)
	privilegeIDs := int64Values(category["privilege_ids"])
	if len(privilegeIDs) < 2 || privilegeIDs[0] != lowerPrivilegeID || privilegeIDs[1] != privilegeID {
		t.Fatalf("hierarchy category privilege order = %+v lower group=%d", category, lowerGroupID)
	}
	privilegeRows, err := env.Model("res.groups.privilege").Browse(privilegeID).Read("sequence", "placeholder", "group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if privilegeRows[0]["placeholder"] != "No" || !containsInt64(privilegeRows[0]["group_ids"], exportGroupID) {
		t.Fatalf("privilege row = %+v", privilegeRows[0])
	}

	if _, err := env.Model("res.groups").Create(map[string]any{"name": "-invalid"}); err == nil {
		t.Fatal("expected invalid group name error")
	}
	if _, err := env.Model("res.groups").Create(map[string]any{"name": "Bad API", "api_key_duration": -1.0}); err == nil {
		t.Fatal("expected negative api key duration error")
	}
}

func TestResUsersGroupPayloadX2ManyCommands(t *testing.T) {
	env := securityDerivedFieldsEnv(t)
	groupUserID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / User"})
	if err != nil {
		t.Fatal(err)
	}
	groupSystemID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Administrator", "implied_ids": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	for key, id := range map[string]int64{
		"group_user":   groupUserID,
		"group_system": groupSystemID,
	} {
		if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": key, "model": "res.groups", "res_id": id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Model("ir.model.access").Create(map[string]any{"name": "user access", "group_id": groupUserID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.rule").Create(map[string]any{"name": "system rule", "group_ids": []int64{groupSystemID}}); err != nil {
		t.Fatal(err)
	}

	userID, err := env.Model("res.users").Create(map[string]any{
		"login":     "command-user",
		"name":      "Command User",
		"active":    true,
		"group_ids": []any{[]any{int64(6), false, []any{groupSystemID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.users").Browse(userID).Read("group_ids", "groups_id", "all_group_ids", "accesses_count", "rules_count", "groups_count", "role", "view_group_hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	row := rows[0]
	if !containsInt64(row["group_ids"], groupSystemID) || !containsInt64(row["groups_id"], groupSystemID) {
		t.Fatalf("explicit groups after set command = %+v", row)
	}
	if !containsInt64(row["all_group_ids"], groupUserID) || !containsInt64(row["all_group_ids"], groupSystemID) {
		t.Fatalf("all groups after set command = %+v", row)
	}
	if row["role"] != "group_system" || row["groups_count"] != int64(2) || row["accesses_count"] != int64(1) || row["rules_count"] != int64(1) {
		t.Fatalf("computed group payload after set command = %+v", row)
	}
	groups := row["view_group_hierarchy"].(map[string]any)["groups"].(map[int64]any)
	if _, ok := groups[groupSystemID]; !ok {
		t.Fatalf("view group hierarchy missing system group = %+v", row["view_group_hierarchy"])
	}

	if err := env.Model("res.users").Browse(userID).Write(map[string]any{"group_ids": []any{[]any{int64(3), groupSystemID, false}, []any{int64(4), groupUserID, false}}}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.users").Browse(userID).Read("group_ids", "groups_id", "all_group_ids", "rules_count", "groups_count", "role")
	if err != nil {
		t.Fatal(err)
	}
	row = rows[0]
	if containsInt64(row["group_ids"], groupSystemID) || !containsInt64(row["group_ids"], groupUserID) || !containsInt64(row["groups_id"], groupUserID) {
		t.Fatalf("explicit groups after unlink/link commands = %+v", row)
	}
	if row["role"] != "group_user" || row["groups_count"] != int64(1) || row["rules_count"] != int64(0) {
		t.Fatalf("computed group payload after unlink/link commands = %+v", row)
	}

	if err := env.Model("res.users").Browse(userID).Write(map[string]any{"group_ids": []any{[]any{int64(5), false, false}}}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.users").Browse(userID).Read("group_ids", "groups_id", "all_group_ids", "groups_count", "role")
	if err != nil {
		t.Fatal(err)
	}
	row = rows[0]
	if len(int64Values(row["group_ids"])) != 0 || len(int64Values(row["groups_id"])) != 0 || len(int64Values(row["all_group_ids"])) != 0 || row["groups_count"] != int64(0) || row["role"] != false {
		t.Fatalf("group payload after clear command = %+v", row)
	}
}

func TestResUsersReadDoesNotMutateDerivedFields(t *testing.T) {
	env := securityDerivedFieldsEnv(t)
	groupUserID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / User"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_user", "model": "res.groups", "res_id": groupUserID}); err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{
		"login":     "read-user",
		"name":      "Read User",
		"active":    true,
		"group_ids": []int64{groupUserID},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := env.stores["res.users"].records[userID]
	delete(stored, "all_group_ids")
	delete(stored, "share")
	delete(stored, "groups_count")
	delete(stored, "role")

	rows, err := env.Model("res.users").Browse(userID).Read("id", "name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Read User" {
		t.Fatalf("read rows = %+v", rows)
	}
	for _, fieldName := range []string{"all_group_ids", "share", "groups_count", "role"} {
		if _, ok := stored[fieldName]; ok {
			t.Fatalf("read mutated stored %s: %+v", fieldName, stored)
		}
	}
}

func TestResUsersRejectDisjointUserTypeGroups(t *testing.T) {
	env := securityDerivedFieldsEnv(t)
	groupUserID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / User"})
	if err != nil {
		t.Fatal(err)
	}
	groupPortalID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Portal"})
	if err != nil {
		t.Fatal(err)
	}
	groupSystemID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Administrator", "implied_ids": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	for key, id := range map[string]int64{
		"group_user":   groupUserID,
		"group_portal": groupPortalID,
		"group_system": groupSystemID,
	} {
		if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": key, "model": "res.groups", "res_id": id}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := env.Model("res.users").Create(map[string]any{"login": "bad-create", "name": "Bad Create", "group_ids": []int64{groupUserID, groupPortalID}}); err == nil || !strings.Contains(err.Error(), "exclusive groups") {
		t.Fatalf("expected create disjoint group error, got %v", err)
	}

	userID, err := env.Model("res.users").Create(map[string]any{"login": "valid", "name": "Valid", "group_ids": []int64{groupSystemID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(userID).Write(map[string]any{"group_ids": []any{[]any{int64(6), false, []any{groupSystemID, groupPortalID}}}}); err == nil || !strings.Contains(err.Error(), "exclusive groups") {
		t.Fatalf("expected write disjoint group error, got %v", err)
	}
	rows, err := env.Model("res.users").Browse(userID).Read("group_ids", "all_group_ids", "role")
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt64(rows[0]["group_ids"], groupSystemID) || containsInt64(rows[0]["group_ids"], groupPortalID) || rows[0]["role"] != "group_system" {
		t.Fatalf("user groups after failed write = %+v", rows[0])
	}
}

func TestResPartnerAndUsersDerivedSecurityFields(t *testing.T) {
	env := securityDerivedFieldsEnv(t)
	groupUserID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / User"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_user", "model": "res.groups", "res_id": groupUserID}); err != nil {
		t.Fatal(err)
	}
	groupSystemID, err := env.Model("res.groups").Create(map[string]any{"name": "Role / Administrator", "implied_ids": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_system", "model": "res.groups", "res_id": groupSystemID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.access").Create(map[string]any{"name": "user access", "group_id": groupUserID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.rule").Create(map[string]any{"name": "system rule", "group_ids": []int64{groupSystemID}}); err != nil {
		t.Fatal(err)
	}
	rootPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Root", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false, "partner_id": rootPartnerID}); err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("res.partner").Create(map[string]any{"name": "Parent", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("res.partner").Create(map[string]any{"name": "Child", "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(parentID, childID).Read("commercial_partner_id", "partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["commercial_partner_id"] != parentID || rows[0]["partner_share"] != true {
		t.Fatalf("parent derived fields = %+v", rows[0])
	}
	if rows[1]["commercial_partner_id"] != parentID || rows[1]["partner_share"] != true {
		t.Fatalf("child derived fields = %+v", rows[1])
	}

	inactivePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Inactive Linked", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	inactiveUserID, err := env.Model("res.users").Create(map[string]any{"login": "inactive", "name": "Inactive Internal", "active": false, "partner_id": inactivePartnerID, "groups_id": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(inactivePartnerID).Read("active", "partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != false || rows[0]["partner_share"] != true {
		t.Fatalf("inactive user partner fields = %+v", rows[0])
	}
	userRows, err := env.Model("res.users").Browse(inactiveUserID).Read("share", "active_partner")
	if err != nil {
		t.Fatal(err)
	}
	if userRows[0]["share"] != false || userRows[0]["active_partner"] != false {
		t.Fatalf("inactive internal user fields = %+v", userRows[0])
	}

	activeLinkedPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Active Linked", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activeLinkedUserID, err := env.Model("res.users").Create(map[string]any{"login": "active-linked", "name": "Active Linked", "active": true, "partner_id": activeLinkedPartnerID, "groups_id": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(activeLinkedPartnerID).Read("active", "partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true || rows[0]["partner_share"] != false {
		t.Fatalf("active user partner fields = %+v", rows[0])
	}
	if err := env.Model("res.partner").Browse(activeLinkedPartnerID).Write(map[string]any{"active": false}); err == nil || !strings.Contains(err.Error(), "active user") {
		t.Fatalf("expected archive linked active user denial, got %v", err)
	}
	if err := env.Model("res.users").Browse(activeLinkedUserID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(activeLinkedPartnerID).Read("active", "partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true || rows[0]["partner_share"] != true {
		t.Fatalf("deactivated user partner fields = %+v", rows[0])
	}
	if err := env.Model("res.users").Browse(inactiveUserID).Write(map[string]any{"active": true}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(inactivePartnerID).Read("active", "partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true || rows[0]["partner_share"] != false {
		t.Fatalf("reactivated user partner fields = %+v", rows[0])
	}

	internalUserID, err := env.Model("res.users").Create(map[string]any{"login": "internal", "name": "Internal", "partner_id": parentID, "groups_id": []int64{groupUserID}})
	if err != nil {
		t.Fatal(err)
	}
	portalUserID, err := env.Model("res.users").Create(map[string]any{"login": "portal", "name": "Portal", "partner_id": childID})
	if err != nil {
		t.Fatal(err)
	}
	userRows, err = env.Model("res.users").Browse(internalUserID, portalUserID).Read("share", "commercial_partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if userRows[0]["share"] != false || userRows[0]["commercial_partner_id"] != parentID {
		t.Fatalf("internal user derived fields = %+v", userRows[0])
	}
	if userRows[1]["share"] != true || userRows[1]["commercial_partner_id"] != parentID {
		t.Fatalf("portal user derived fields = %+v", userRows[1])
	}
	systemUserID, err := env.Model("res.users").Create(map[string]any{"login": "system", "name": "System", "partner_id": parentID, "groups_id": []int64{groupSystemID}})
	if err != nil {
		t.Fatal(err)
	}
	userRows, err = env.Model("res.users").Browse(internalUserID, systemUserID).Read("role", "all_group_ids", "accesses_count", "rules_count", "groups_count", "view_group_hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	if userRows[0]["role"] != "group_user" || !containsInt64(userRows[0]["all_group_ids"], groupUserID) || userRows[0]["groups_count"] != int64(1) {
		t.Fatalf("internal user group payload = %+v", userRows[0])
	}
	if userRows[1]["role"] != "group_system" || !containsInt64(userRows[1]["all_group_ids"], groupUserID) || !containsInt64(userRows[1]["all_group_ids"], groupSystemID) {
		t.Fatalf("system user groups = %+v", userRows[1])
	}
	if userRows[1]["accesses_count"] != int64(1) || userRows[1]["rules_count"] != int64(1) || userRows[1]["groups_count"] != int64(2) {
		t.Fatalf("system user counts = %+v", userRows[1])
	}
	hierarchy := userRows[1]["view_group_hierarchy"].(map[string]any)
	groups := hierarchy["groups"].(map[int64]any)
	if _, ok := groups[groupSystemID]; !ok {
		t.Fatalf("system user hierarchy = %+v", hierarchy)
	}
	rows, err = env.Model("res.partner").Browse(parentID, childID).Read("partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["partner_share"] != false || rows[1]["partner_share"] != true {
		t.Fatalf("partner share after users = %+v", rows)
	}

	if err := env.Model("res.groups").Browse(groupUserID).Write(map[string]any{"user_ids": []int64{portalUserID}}); err != nil {
		t.Fatal(err)
	}
	userRows, err = env.Model("res.users").Browse(portalUserID).Read("share")
	if err != nil {
		t.Fatal(err)
	}
	if userRows[0]["share"] != false {
		t.Fatalf("portal user share after group inverse = %+v", userRows[0])
	}
	rows, err = env.Model("res.partner").Browse(childID).Read("partner_share")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["partner_share"] != false {
		t.Fatalf("child partner_share after group inverse = %+v", rows[0])
	}

	newParentID, err := env.Model("res.partner").Create(map[string]any{"name": "New Parent", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.partner").Browse(childID).Write(map[string]any{"parent_id": newParentID}); err != nil {
		t.Fatal(err)
	}
	userRows, err = env.Model("res.users").Browse(portalUserID).Read("commercial_partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if userRows[0]["commercial_partner_id"] != newParentID {
		t.Fatalf("portal user commercial partner after parent write = %+v", userRows[0])
	}
}

func TestModelSetMetadataAndWebHelpers(t *testing.T) {
	env := testEnv(t)
	partners := env.Model("res.partner")
	first, err := partners.Create(map[string]any{"name": "Administrator", "active": true, "age": 40})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partners.Create(map[string]any{"name": "Guest", "active": false, "age": 20}); err != nil {
		t.Fatal(err)
	}

	fields, err := partners.FieldsGet([]string{"name", "display_name"}, []string{"string", "type"})
	if err != nil {
		t.Fatal(err)
	}
	if fields["name"]["type"] != "char" || fields["display_name"]["type"] != "char" {
		t.Fatalf("fields = %#v", fields)
	}

	filteredFields, err := partners.env.WithPolicy(fieldFilterPolicy{blocked: map[string]bool{"age": true}}).Model("res.partner").FieldsGet([]string{"name", "age", "display_name"}, []string{"type"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := filteredFields["age"]; ok {
		t.Fatalf("age should be hidden in fields_get: %#v", filteredFields)
	}
	if filteredFields["name"]["type"] != "char" || filteredFields["display_name"]["type"] != "char" {
		t.Fatalf("filtered fields = %#v", filteredFields)
	}

	defaults, err := partners.DefaultGet([]string{"name"}, map[string]any{"default_name": "Context Name"})
	if err != nil {
		t.Fatal(err)
	}
	if defaults["name"] != "Context Name" {
		t.Fatalf("defaults = %#v", defaults)
	}

	count, err := partners.SearchCount(domain.Cond("name", domain.ILike, "a"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}

	pairs, err := partners.NameSearch("admin", domain.And(), domain.ILike, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0][0] != first || pairs[0][1] != "Administrator" {
		t.Fatalf("pairs = %#v", pairs)
	}

	rows, err := partners.Browse(first).WebRead(map[string]any{"name": map[string]any{}, "display_name": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Administrator" || rows[0]["display_name"] != "Administrator" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestWebReadFormatsMany2One(t *testing.T) {
	registry := NewRegistry()
	company := model.New("res.company", "res_company")
	company.AddField(field.New("name", field.Char))
	if err := registry.Register(company); err != nil {
		t.Fatal(err)
	}
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "ACME"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(partnerID).WebRead(map[string]any{"company_id": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	companyValue, ok := rows[0]["company_id"].([]any)
	if !ok || len(companyValue) != 2 || companyValue[0] != companyID || companyValue[1] != "ACME" {
		t.Fatalf("company_id = %#v", rows[0]["company_id"])
	}
}

func TestWebReadFormatsX2ManySpecs(t *testing.T) {
	registry := NewRegistry()
	user := model.New("x.web.user", "x_web_user")
	user.AddField(field.New("name", field.Char))
	if err := registry.Register(user); err != nil {
		t.Fatal(err)
	}
	tag := model.New("x.web.tag", "x_web_tag")
	tag.Order = "name"
	tag.AddField(field.New("name", field.Char))
	tag.AddField(field.New("active", field.Bool))
	if err := registry.Register(tag); err != nil {
		t.Fatal(err)
	}
	parent := model.New("x.web.parent", "x_web_parent")
	parent.AddField(field.New("name", field.Char))
	parent.AddField(field.New("line_ids", field.One2Many).WithRelation("x.web.line").WithRelationField("parent_id"))
	parent.AddField(field.New("all_line_ids", field.One2Many).WithRelation("x.web.line").WithRelationField("parent_id").WithContext(map[string]any{"active_test": false}))
	parent.AddField(field.New("active_line_ids", field.One2Many).WithRelation("x.web.line").WithRelationField("parent_id").WithContext(map[string]any{"active_test": true}))
	parent.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.web.tag"))
	parent.AddField(field.New("all_tag_ids", field.Many2Many).WithRelation("x.web.tag").WithContext(map[string]any{"active_test": false}))
	parent.AddField(field.New("active_tag_ids", field.Many2Many).WithRelation("x.web.tag").WithContext(map[string]any{"active_test": true}))
	if err := registry.Register(parent); err != nil {
		t.Fatal(err)
	}
	line := model.New("x.web.line", "x_web_line")
	line.Order = "sequence"
	line.AddField(field.New("name", field.Char))
	line.AddField(field.New("sequence", field.Int))
	line.AddField(field.New("active", field.Bool))
	line.AddField(field.New("parent_id", field.Many2One).WithRelation("x.web.parent"))
	line.AddField(field.New("user_id", field.Many2One).WithRelation("x.web.user"))
	line.AddField(field.New("payload", field.Binary))
	if err := registry.Register(line); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	userID, err := env.Model("x.web.user").Create(map[string]any{"name": "Reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	tagA, err := env.Model("x.web.tag").Create(map[string]any{"name": "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	tagBInactive, err := env.Model("x.web.tag").Create(map[string]any{"name": "Bravo", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	tagZ, err := env.Model("x.web.tag").Create(map[string]any{"name": "Zulu"})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("x.web.parent").Create(map[string]any{
		"name":           "Parent",
		"tag_ids":        []int64{tagZ, tagBInactive, tagA},
		"all_tag_ids":    []int64{tagZ, tagBInactive, tagA},
		"active_tag_ids": []int64{tagZ, tagBInactive, tagA},
	})
	if err != nil {
		t.Fatal(err)
	}
	lineLow, err := env.Model("x.web.line").Create(map[string]any{"name": "Low", "sequence": int64(10), "parent_id": parentID, "user_id": userID, "payload": "low"})
	if err != nil {
		t.Fatal(err)
	}
	lineMid, err := env.Model("x.web.line").Create(map[string]any{"name": "Mid", "sequence": int64(20), "parent_id": parentID, "user_id": userID, "payload": "mid"})
	if err != nil {
		t.Fatal(err)
	}
	lineHigh, err := env.Model("x.web.line").Create(map[string]any{"name": "High", "sequence": int64(30), "parent_id": parentID, "user_id": userID, "payload": "high"})
	if err != nil {
		t.Fatal(err)
	}
	lineInactive, err := env.Model("x.web.line").Create(map[string]any{"name": "Archived", "sequence": int64(40), "active": false, "parent_id": parentID, "user_id": userID, "payload": "archived"})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{"line_ids": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0]["line_ids"]; !reflect.DeepEqual(got, []int64{lineLow, lineMid, lineHigh}) {
		t.Fatalf("empty one2many spec ids = %#v", got)
	}
	rows, err = env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{"tag_ids": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0]["tag_ids"]; !reflect.DeepEqual(got, []int64{tagA, tagZ}) {
		t.Fatalf("empty many2many spec ids = %#v", got)
	}
	rows, err = env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"all_line_ids": map[string]any{
			"fields": map[string]any{"name": map[string]any{}},
			"order":  "sequence desc",
		},
		"all_tag_ids": map[string]any{
			"fields": map[string]any{"display_name": map[string]any{}},
			"order":  "name asc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	allLines, ok := rows[0]["all_line_ids"].([]map[string]any)
	if !ok || len(allLines) != 4 || allLines[0]["id"] != lineInactive || allLines[0]["name"] != "Archived" {
		t.Fatalf("field context one2many active_test false = %#v", rows[0]["all_line_ids"])
	}
	allTags, ok := rows[0]["all_tag_ids"].([]map[string]any)
	if !ok || len(allTags) != 3 || allTags[1]["id"] != tagBInactive || allTags[1]["display_name"] != "Bravo" {
		t.Fatalf("field context many2many active_test false = %#v", rows[0]["all_tag_ids"])
	}
	inactiveEnv := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}})
	rows, err = inactiveEnv.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"active_line_ids": map[string]any{},
		"active_tag_ids":  map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0]["active_line_ids"]; !reflect.DeepEqual(got, []int64{lineLow, lineMid, lineHigh}) {
		t.Fatalf("field context one2many active_test true = %#v", got)
	}
	if got := rows[0]["active_tag_ids"]; !reflect.DeepEqual(got, []int64{tagA, tagZ}) {
		t.Fatalf("field context many2many active_test true = %#v", got)
	}
	rows, err = env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"tag_ids": map[string]any{
			"fields":  map[string]any{"display_name": map[string]any{}},
			"context": map[string]any{"active_test": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contextTags, ok := rows[0]["tag_ids"].([]map[string]any)
	if !ok || len(contextTags) != 2 || contextTags[0]["id"] != tagA || contextTags[1]["id"] != tagZ {
		t.Fatalf("web_read spec context changed current membership = %#v", rows[0]["tag_ids"])
	}

	rows, err = env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"line_ids": map[string]any{
			"fields":  map[string]any{"name": map[string]any{}, "user_id": map[string]any{}, "payload": map[string]any{}},
			"context": map[string]any{"bin_size": true},
			"domain":  []any{[]any{"name", "=", "Low"}},
			"offset":  int64(1),
			"order":   "sequence desc",
			"limit":   int64(2),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines, ok := rows[0]["line_ids"].([]map[string]any)
	if !ok || len(lines) != 3 {
		t.Fatalf("line_ids = %#v", rows[0]["line_ids"])
	}
	if lines[0]["id"] != lineHigh || lines[0]["name"] != "High" || lines[1]["id"] != lineMid || lines[1]["name"] != "Mid" {
		t.Fatalf("ordered limited line_ids = %#v", lines)
	}
	if lines[0]["payload"] != "4 bytes" || lines[1]["payload"] != "3 bytes" {
		t.Fatalf("nested context payloads = %#v", lines)
	}
	if !reflect.DeepEqual(lines[2], map[string]any{"id": lineLow}) {
		t.Fatalf("limited placeholder line = %#v", lines[2])
	}
	userValue, ok := lines[0]["user_id"].([]any)
	if !ok || len(userValue) != 2 || userValue[0] != userID || userValue[1] != "Reviewer" {
		t.Fatalf("nested many2one = %#v", lines[0]["user_id"])
	}

	rows, err = env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"tag_ids": map[string]any{
			"fields": map[string]any{"display_name": map[string]any{}},
			"order":  "name desc",
			"limit":  int64(1),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tags, ok := rows[0]["tag_ids"].([]map[string]any)
	if !ok || len(tags) != 2 || tags[0]["id"] != tagZ || tags[0]["display_name"] != "Zulu" || !reflect.DeepEqual(tags[1], map[string]any{"id": tagA}) {
		t.Fatalf("ordered limited tag_ids = %#v", rows[0]["tag_ids"])
	}

	if _, err := env.Model("x.web.parent").Browse(parentID).WebRead(map[string]any{
		"line_ids": map[string]any{"fields": map[string]any{"name": map[string]any{}}, "order": "missing desc"},
	}); err == nil || !strings.Contains(err.Error(), "invalid search order field") {
		t.Fatalf("invalid nested order error = %v", err)
	}
}

func TestReadBinaryBinSizeContext(t *testing.T) {
	registry := NewRegistry()
	doc := model.New("x.document", "x_document")
	doc.AddField(field.New("name", field.Char))
	doc.AddField(field.New("payload", field.Binary))
	doc.AddField(field.New("thumbnail", field.Binary))
	if err := registry.Register(doc); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	id, err := env.Model("x.document").Create(map[string]any{
		"name":      "Manual",
		"payload":   "content",
		"thumbnail": "thumbnail-data",
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"bin_size": true}}).Model("x.document").Browse(id).Read("payload", "thumbnail")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["payload"] != "7 bytes" || rows[0]["thumbnail"] != "14 bytes" {
		t.Fatalf("bin_size rows = %#v", rows)
	}

	rows, err = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"bin_size_payload": true}}).Model("x.document").Browse(id).Read("payload", "thumbnail")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["payload"] != "7 bytes" || rows[0]["thumbnail"] != "thumbnail-data" {
		t.Fatalf("field bin_size rows = %#v", rows)
	}
}

func TestWebReadBinaryBinSizeContext(t *testing.T) {
	registry := NewRegistry()
	doc := model.New("x.document", "x_document")
	doc.AddField(field.New("name", field.Char))
	doc.AddField(field.New("payload", field.Binary))
	if err := registry.Register(doc); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	id, err := env.Model("x.document").Create(map[string]any{"name": "Manual", "payload": "content"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"bin_size": true}}).Model("x.document").Browse(id).WebRead(map[string]any{"payload": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["payload"] != "7 bytes" {
		t.Fatalf("web_read rows = %#v", rows)
	}
}

func TestAttachmentDatasBinSizeContextUsesDecodedSize(t *testing.T) {
	registry := NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	id, err := env.Model("ir.attachment").Create(map[string]any{
		"name":  "ping.txt",
		"type":  "binary",
		"datas": "cGluZw==",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"bin_size": true}}).Model("ir.attachment").Browse(id).Read("datas")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["datas"] != "4.00 bytes" {
		t.Fatalf("attachment datas = %#v", rows[0]["datas"])
	}
}

func TestModelSetReadGroup(t *testing.T) {
	env := testEnv(t)
	partners := env.Model("res.partner")
	if _, err := partners.Create(map[string]any{"name": "Alpha", "active": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := partners.Create(map[string]any{"name": "Beta", "active": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := partners.Create(map[string]any{"name": "Gamma", "active": false}); err != nil {
		t.Fatal(err)
	}
	partners = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}}).Model("res.partner")

	groups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[0]["active"] != false || groups[0]["active_count"] != 1 {
		t.Fatalf("active group = %#v", groups[0])
	}
	if _, hasRawCount := groups[0]["__count"]; hasRawCount {
		t.Fatalf("legacy read_group leaked __count alias = %#v", groups[0])
	}
	groupDomain, ok := groups[0]["__domain"].([]any)
	if !ok || len(groupDomain) != 1 {
		t.Fatalf("group domain = %#v", groups[0]["__domain"])
	}
	groupCondition, ok := groupDomain[0].([]any)
	if !ok || len(groupCondition) != 3 || groupCondition[0] != "active" || groupCondition[1] != "=" || groupCondition[2] != false {
		t.Fatalf("group condition = %#v", groupDomain[0])
	}
	descGroups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active"}, Order: "active desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(descGroups) != 2 || descGroups[0]["active"] != true || descGroups[0]["active_count"] != 2 {
		t.Fatalf("ordered active groups = %#v", descGroups)
	}
	countGroups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active"}, Order: "__count desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(countGroups) != 2 || countGroups[0]["active"] != true || countGroups[0]["active_count"] != 2 {
		t.Fatalf("count ordered groups = %#v", countGroups)
	}
	pagedGroups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active"}, Order: "active desc", Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(pagedGroups) != 1 || pagedGroups[0]["active"] != false || pagedGroups[0]["active_count"] != 1 {
		t.Fatalf("paged ordered groups = %#v", pagedGroups)
	}

	countOnly, err := partners.ReadGroup(domain.Cond("name", domain.ILike, "a"), ReadGroupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(countOnly) != 1 || countOnly[0]["__count"] != 3 {
		t.Fatalf("count group = %#v", countOnly)
	}

	lazyGroups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active", "name"}})
	if err != nil {
		t.Fatal(err)
	}
	contextValue, ok := lazyGroups[0]["__context"].(map[string]any)
	if !ok {
		t.Fatalf("lazy context = %#v", lazyGroups[0]["__context"])
	}
	remaining, ok := contextValue["group_by"].([]string)
	if !ok || !reflect.DeepEqual(remaining, []string{"name"}) {
		t.Fatalf("lazy remaining groupby = %#v", contextValue["group_by"])
	}
	if _, hasName := lazyGroups[0]["name"]; hasName {
		t.Fatalf("lazy read_group grouped by deferred field = %#v", lazyGroups[0])
	}
	eager := false
	eagerGroups, err := partners.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"active", "name"}, Lazy: &eager})
	if err != nil {
		t.Fatal(err)
	}
	if len(eagerGroups) != 3 || eagerGroups[0]["__count"] != 1 || eagerGroups[0]["active"] != false || eagerGroups[0]["name"] != "Gamma" {
		t.Fatalf("eager groups = %#v", eagerGroups)
	}
	if _, hasContext := eagerGroups[0]["__context"]; hasContext {
		t.Fatalf("eager read_group emitted context = %#v", eagerGroups[0])
	}
}

func TestModelSetReadGroupAggregatesAndMany2OneLabels(t *testing.T) {
	registry := NewRegistry()
	company := model.New("res.company", "res_company")
	company.AddField(field.New("name", field.Char))
	if err := registry.Register(company); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.aggregate", "x_read_group_aggregate")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	sample.AddField(field.New("score", field.Int).WithAggregator("avg"))
	sample.AddField(field.New("active", field.Bool).WithAggregator("bool_or"))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	alphaID, err := env.Model("res.company").Create(map[string]any{"name": "Alpha Corp"})
	if err != nil {
		t.Fatal(err)
	}
	betaID, err := env.Model("res.company").Create(map[string]any{"name": "Beta Corp"})
	if err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.aggregate")
	for _, values := range []map[string]any{
		{"name": "a", "company_id": alphaID, "amount": 10.5, "score": int64(2), "active": false},
		{"name": "b", "company_id": alphaID, "amount": 4.5, "score": int64(4), "active": true},
		{"name": "c", "company_id": betaID, "amount": 7.0, "score": int64(8), "active": false},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	records = env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"active_test": false}}).Model("x.read.group.aggregate")
	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"amount", "score:avg", "score_total:sum(score)", "active"},
		GroupBy: []string{"company_id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("aggregate groups = %#v", groups)
	}
	companyValue, ok := groups[0]["company_id"].([]any)
	if !ok || len(companyValue) != 2 || companyValue[0] != alphaID || companyValue[1] != "Alpha Corp" {
		t.Fatalf("company group value = %#v", groups[0]["company_id"])
	}
	if groups[0]["company_id_count"] != 2 || groups[0]["amount"] != 15.0 || groups[0]["score"] != 3.0 || groups[0]["score_total"] != int64(6) || groups[0]["active"] != true {
		t.Fatalf("first aggregate group = %#v", groups[0])
	}
	if groups[1]["company_id_count"] != 1 || groups[1]["amount"] != 7.0 || groups[1]["score"] != 8.0 || groups[1]["score_total"] != int64(8) || groups[1]["active"] != false {
		t.Fatalf("second aggregate group = %#v", groups[1])
	}
	orderedGroups, err := records.ReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"amount", "score:avg", "score_total:sum(score)"},
		GroupBy: []string{"company_id"},
		Order:   "score desc",
	})
	if err != nil {
		t.Fatal(err)
	}
	orderedCompany, ok := orderedGroups[0]["company_id"].([]any)
	if !ok || orderedCompany[0] != betaID || orderedGroups[0]["score"] != 8.0 {
		t.Fatalf("ordered aggregate groups = %#v", orderedGroups)
	}
	orderedAliasGroups, err := records.ReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"score_total:sum(score)"},
		GroupBy: []string{"company_id"},
		Order:   "score_total desc",
	})
	if err != nil {
		t.Fatal(err)
	}
	orderedAliasCompany, ok := orderedAliasGroups[0]["company_id"].([]any)
	if !ok || orderedAliasCompany[0] != betaID || orderedAliasGroups[0]["score_total"] != int64(8) {
		t.Fatalf("ordered alias groups = %#v", orderedAliasGroups)
	}
	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"amount:sum", "score:avg"},
		GroupBy: []string{"company_id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	formattedCompany, ok := formatted[0]["company_id"].([]any)
	if !ok || formattedCompany[0] != alphaID || formattedCompany[1] != "Alpha Corp" {
		t.Fatalf("formatted company = %#v", formatted[0]["company_id"])
	}
	if formatted[0]["amount:sum"] != 15.0 || formatted[0]["score:avg"] != 3.0 || formatted[0]["__count"] != 2 {
		t.Fatalf("formatted aggregates = %#v", formatted[0])
	}
	formatted, err = records.FormattedReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"amount:sum", "score:avg"},
		GroupBy: []string{"company_id"},
		Order:   "amount:sum asc",
	})
	if err != nil {
		t.Fatal(err)
	}
	formattedCompany, ok = formatted[0]["company_id"].([]any)
	if !ok || formattedCompany[0] != betaID || formatted[0]["amount:sum"] != 7.0 {
		t.Fatalf("formatted ordered aggregates = %#v", formatted)
	}
	if _, err := records.ReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount:median"}, GroupBy: []string{"company_id"}}); err == nil || !strings.Contains(err.Error(), `aggregate "median" is not supported`) {
		t.Fatalf("invalid aggregate error = %v", err)
	}
	if _, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount:sum"}, GroupBy: []string{"company_id"}, Order: "missing desc"}); err == nil || !strings.Contains(err.Error(), `order term`) {
		t.Fatalf("invalid order error = %v", err)
	}
}

func TestModelSetReadGroupRecordsetAggregate(t *testing.T) {
	registry := NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	tag := model.New("x.read.group.recordset.tag", "x_read_group_recordset_tag")
	tag.AddField(field.New("name", field.Char))
	if err := registry.Register(tag); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.recordset", "x_read_group_recordset")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("category", field.Char))
	sample.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	sample.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.read.group.recordset.tag"))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	partnerA, err := env.Model("res.partner").Create(map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := env.Model("res.partner").Create(map[string]any{"name": "Babbage"})
	if err != nil {
		t.Fatal(err)
	}
	tagA, err := env.Model("x.read.group.recordset.tag").Create(map[string]any{"name": "Tag A"})
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := env.Model("x.read.group.recordset.tag").Create(map[string]any{"name": "Tag B"})
	if err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.recordset")
	rowA, err := records.Create(map[string]any{"name": "a", "category": "alpha", "partner_id": partnerA, "tag_ids": []int64{tagA, tagB}})
	if err != nil {
		t.Fatal(err)
	}
	rowB, err := records.Create(map[string]any{"name": "b", "category": "alpha", "partner_id": partnerA, "tag_ids": []int64{tagB}})
	if err != nil {
		t.Fatal(err)
	}
	rowC, err := records.Create(map[string]any{"name": "c", "category": "alpha", "partner_id": partnerB, "tag_ids": []int64{tagA}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(map[string]any{"name": "d", "category": "beta"}); err != nil {
		t.Fatal(err)
	}

	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"ids:recordset(id)", "partner_records:recordset(partner_id)", "partner_values:array_agg(partner_id)", "tag_records:recordset(tag_ids)"},
		GroupBy: []string{"category"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var alpha map[string]any
	for _, row := range groups {
		if row["category"] == "alpha" {
			alpha = row
			break
		}
	}
	if alpha == nil {
		t.Fatalf("missing alpha row: %#v", groups)
	}
	idRecords, ok := alpha["ids"].(RecordSet)
	if !ok || !reflect.DeepEqual(idRecords.IDs(), []int64{rowA, rowB, rowC}) {
		t.Fatalf("ids = %#v", alpha["ids"])
	}
	partnerRecords, ok := alpha["partner_records"].(RecordSet)
	if !ok || !reflect.DeepEqual(partnerRecords.IDs(), []int64{partnerA, partnerB}) {
		t.Fatalf("partner_records = %#v", alpha["partner_records"])
	}
	tagRecords, ok := alpha["tag_records"].(RecordSet)
	if !ok || !reflect.DeepEqual(tagRecords.IDs(), []int64{tagA, tagB}) {
		t.Fatalf("tag_records = %#v", alpha["tag_records"])
	}
	if !reflect.DeepEqual(alpha["partner_values"], []any{partnerA, partnerA, partnerB}) {
		t.Fatalf("partner_values = %#v", alpha["partner_values"])
	}

	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"id:recordset"},
		GroupBy: []string{"category"},
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha = nil
	for _, row := range formatted {
		if row["category"] == "alpha" {
			alpha = row
			break
		}
	}
	if alpha == nil {
		t.Fatalf("missing formatted alpha row: %#v", formatted)
	}
	if _, ok := alpha["id:recordset"]; ok {
		t.Fatalf("formatted_read_group kept recordset aggregate key = %#v", alpha)
	}
	if !reflect.DeepEqual(alpha["id:array_agg"], []any{rowA, rowB, rowC}) {
		t.Fatalf("formatted id:array_agg = %#v", alpha["id:array_agg"])
	}
	if _, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"ids:recordset(id)"}, GroupBy: []string{"category"}}); err == nil || !strings.Contains(err.Error(), `alias syntax`) {
		t.Fatalf("formatted alias aggregate error = %v", err)
	}

	legacy, err := records.ReadGroup(domain.And(), ReadGroupOptions{
		Fields:  []string{"partner_records:recordset(partner_id)"},
		GroupBy: []string{"category"},
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha = nil
	for _, row := range legacy {
		if row["category"] == "alpha" {
			alpha = row
			break
		}
	}
	if alpha == nil {
		t.Fatalf("missing legacy alpha row: %#v", legacy)
	}
	legacyRecords, ok := alpha["partner_records"].(RecordSet)
	if !ok || !reflect.DeepEqual(legacyRecords.IDs(), []int64{partnerA, partnerB}) {
		t.Fatalf("legacy partner_records = %#v", alpha["partner_records"])
	}
	if _, err := records.ReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"name:recordset"}, GroupBy: []string{"category"}}); err == nil || !strings.Contains(err.Error(), `recordset`) {
		t.Fatalf("invalid recordset aggregate error = %v", err)
	}
}

func TestModelSetReadGroupStoredMany2Many(t *testing.T) {
	registry := NewRegistry()
	tag := model.New("x.read.group.tag", "x_read_group_tag")
	tag.AddField(field.New("name", field.Char))
	if err := registry.Register(tag); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.m2m", "x_read_group_m2m")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.read.group.tag"))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	tagA, err := env.Model("x.read.group.tag").Create(map[string]any{"name": "Tag A"})
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := env.Model("x.read.group.tag").Create(map[string]any{"name": "Tag B"})
	if err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.m2m")
	for _, values := range []map[string]any{
		{"name": "ab", "tag_ids": []int64{tagA, tagB}, "amount": 10.0},
		{"name": "b", "tag_ids": []int64{tagB, tagB}, "amount": 5.0},
		{"name": "empty", "tag_ids": []int64{}, "amount": 1.0},
		{"name": "missing", "amount": 2.0},
		{"name": "stale", "tag_ids": []int64{999}, "amount": 3.0},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount"}, GroupBy: []string{"tag_ids"}})
	if err != nil {
		t.Fatal(err)
	}
	tagAGroup := readGroupTestFindPair(groups, "tag_ids", tagA)
	tagBGroup := readGroupTestFindPair(groups, "tag_ids", tagB)
	staleGroup := readGroupTestFindPair(groups, "tag_ids", int64(999))
	falseGroup := readGroupTestFindScalar(groups, "tag_ids", false)
	if numericID(tagAGroup["tag_ids_count"]) != 1 || tagAGroup["amount"] != 10.0 {
		t.Fatalf("tag A group = %#v", tagAGroup)
	}
	if numericID(tagBGroup["tag_ids_count"]) != 2 || tagBGroup["amount"] != 15.0 || readGroupTestPairLabel(tagBGroup["tag_ids"]) != "Tag B" {
		t.Fatalf("tag B group = %#v", tagBGroup)
	}
	if numericID(staleGroup["tag_ids_count"]) != 1 || staleGroup["amount"] != 3.0 || readGroupTestPairLabel(staleGroup["tag_ids"]) != "" {
		t.Fatalf("stale group = %#v", staleGroup)
	}
	if numericID(falseGroup["tag_ids_count"]) != 2 || falseGroup["amount"] != 3.0 {
		t.Fatalf("false group = %#v", falseGroup)
	}
	readGroupTestAssertDomainCount(t, records, tagBGroup["__domain"], 2)
	readGroupTestAssertDomainCount(t, records, staleGroup["__domain"], 1)
	readGroupTestAssertDomainCount(t, records, falseGroup["__domain"], 2)

	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount:sum"}, GroupBy: []string{"tag_ids"}})
	if err != nil {
		t.Fatal(err)
	}
	formattedTagB := readGroupTestFindPair(formatted, "tag_ids", tagB)
	formattedStale := readGroupTestFindPair(formatted, "tag_ids", int64(999))
	formattedFalse := readGroupTestFindScalar(formatted, "tag_ids", false)
	if numericID(formattedTagB["__count"]) != 2 || formattedTagB["amount:sum"] != 15.0 || readGroupTestPairLabel(formattedTagB["tag_ids"]) != "Tag B" {
		t.Fatalf("formatted tag B group = %#v", formattedTagB)
	}
	if numericID(formattedStale["__count"]) != 1 || formattedStale["amount:sum"] != 3.0 || readGroupTestPairLabel(formattedStale["tag_ids"]) != "" {
		t.Fatalf("formatted stale group = %#v", formattedStale)
	}
	if numericID(formattedFalse["__count"]) != 2 || formattedFalse["amount:sum"] != 3.0 {
		t.Fatalf("formatted false group = %#v", formattedFalse)
	}
	if _, leaksDomain := formattedTagB["__domain"]; leaksDomain {
		t.Fatalf("formatted group leaked __domain = %#v", formattedTagB)
	}
	readGroupTestAssertDomainCount(t, records, formattedTagB["__extra_domain"], 2)
	readGroupTestAssertDomainCount(t, records, formattedStale["__extra_domain"], 1)
	readGroupTestAssertDomainCount(t, records, formattedFalse["__extra_domain"], 2)
}

func TestModelSetReadGroupSumCurrencyAggregate(t *testing.T) {
	registry := NewRegistry()
	currency := model.New("res.currency", "res_currency")
	currency.AddField(field.New("name", field.Char))
	if err := registry.Register(currency); err != nil {
		t.Fatal(err)
	}
	rate := model.New("res.currency.rate", "res_currency_rate")
	rate.AddField(field.New("name", field.Date))
	rate.AddField(field.New("currency_id", field.Many2One).WithRelation("res.currency"))
	rate.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	rate.AddField(field.New("rate", field.Float))
	if err := registry.Register(rate); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.currency", "x_read_group_currency")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("category", field.Char))
	sample.AddField(field.New("currency_id", field.Many2One).WithRelation("res.currency"))
	sample.AddField(field.New("amount", field.Monetary).WithCurrencyField("currency_id").WithAggregator("sum"))
	sample.AddField(field.New("plain_amount", field.Float))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	bhdID, err := env.Model("res.currency").Create(map[string]any{"name": "BHD"})
	if err != nil {
		t.Fatal(err)
	}
	usdID, err := env.Model("res.currency").Create(map[string]any{"name": "USD"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "2020-01-01", "currency_id": bhdID, "rate": 1.0},
		{"name": "2020-01-01", "currency_id": usdID, "rate": 0.5},
		{"name": "2099-01-01", "currency_id": usdID, "rate": 0.25},
		{"name": "2099-01-01", "currency_id": usdID, "company_id": int64(1), "rate": 0.2},
	} {
		if _, err := env.Model("res.currency.rate").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	records := env.Model("x.read.group.currency")
	for _, values := range []map[string]any{
		{"name": "a", "category": "sales", "currency_id": bhdID, "amount": 100.0, "plain_amount": 100.0},
		{"name": "b", "category": "sales", "currency_id": usdID, "amount": 50.0, "plain_amount": 50.0},
		{"name": "c", "category": "cost", "currency_id": usdID, "amount": 20.0, "plain_amount": 20.0},
		{"name": "d", "category": "cost", "amount": 10.0, "plain_amount": 10.0},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount_company:sum_currency(amount)"}, GroupBy: []string{"category"}})
	if err != nil {
		t.Fatal(err)
	}
	sales := readGroupTestFindScalar(groups, "category", "sales")
	cost := readGroupTestFindScalar(groups, "category", "cost")
	if sales["amount_company"] != 350.0 || cost["amount_company"] != 110.0 {
		t.Fatalf("sum_currency groups = %#v", groups)
	}

	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"amount:sum_currency"}, GroupBy: []string{"category"}})
	if err != nil {
		t.Fatal(err)
	}
	formattedSales := readGroupTestFindScalar(formatted, "category", "sales")
	if formattedSales["amount:sum_currency"] != 350.0 || numericID(formattedSales["__count"]) != 2 {
		t.Fatalf("formatted sum_currency = %#v", formatted)
	}
	if _, err := records.ReadGroup(domain.And(), ReadGroupOptions{Fields: []string{"plain_amount:sum_currency"}, GroupBy: []string{"category"}}); err == nil || !strings.Contains(err.Error(), `sum_currency`) {
		t.Fatalf("invalid sum_currency error = %v", err)
	}

	fields, err := records.FieldsGet([]string{"amount"}, []string{"type", "currency_field", "aggregator"})
	if err != nil {
		t.Fatal(err)
	}
	amountField := fields["amount"]
	if amountField["type"] != "monetary" || amountField["currency_field"] != "currency_id" || amountField["aggregator"] != "sum" {
		t.Fatalf("monetary field description = %#v", amountField)
	}
}

func TestModelSetReadGroupDateIntervals(t *testing.T) {
	registry := NewRegistry()
	lang := model.New("res.lang", "res_lang")
	lang.AddField(field.New("code", field.Char))
	lang.AddField(field.New("week_start", field.Char))
	if err := registry.Register(lang); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.date", "x_read_group_date")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1, Values: map[string]any{"lang": "en_US"}})
	if _, err := env.Model("res.lang").Create(map[string]any{"code": "en_US", "week_start": "7"}); err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.date")
	for _, values := range []map[string]any{
		{"name": "a", "date_value": "2026-01-10", "moment_value": "2026-01-05 08:00:00"},
		{"name": "b", "date_value": "2026-01-20", "moment_value": "2026-01-07 09:00:00"},
		{"name": "c", "date_value": "2026-02-05", "moment_value": "2026-01-12 10:00:00"},
		{"name": "d", "date_value": "2022-01-29", "moment_value": "2022-01-29 08:00:00"},
		{"name": "e", "date_value": "2022-01-30", "moment_value": "2022-01-30 08:00:00"},
		{"name": "f", "date_value": "2022-01-31", "moment_value": "2022-01-31 08:00:00"},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	months, err := records.ReadGroup(domain.Cond("name", domain.In, []any{"a", "b", "c"}), ReadGroupOptions{GroupBy: []string{"date_value:month"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 || months[0]["date_value:month"] != "January 2026" || months[0]["date_value_count"] != 2 || months[1]["date_value:month"] != "February 2026" || months[1]["date_value_count"] != 1 {
		t.Fatalf("month groups = %#v", months)
	}
	descMonths, err := records.ReadGroup(domain.Cond("name", domain.In, []any{"a", "b", "c"}), ReadGroupOptions{GroupBy: []string{"date_value:month"}, Order: "date_value:month desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(descMonths) != 2 || descMonths[0]["date_value:month"] != "February 2026" || descMonths[0]["date_value_count"] != 1 {
		t.Fatalf("ordered month groups = %#v", descMonths)
	}
	weeks, err := records.ReadGroup(domain.Cond("name", domain.In, []any{"d", "e", "f"}), ReadGroupOptions{GroupBy: []string{"date_value:week"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 2 || weeks[0]["date_value:week"] != "W5 2022" || weeks[0]["date_value_count"] != 1 || weeks[1]["date_value:week"] != "W6 2022" || weeks[1]["date_value_count"] != 2 {
		t.Fatalf("week groups = %#v", weeks)
	}
	weekDomain := weeks[1]["__domain"].([]any)
	weekStart := weekDomain[0].([]any)
	weekEnd := weekDomain[1].([]any)
	if weekStart[1] != ">=" || weekStart[2] != "2022-01-30" || weekEnd[1] != "<" || weekEnd[2] != "2022-02-06" {
		t.Fatalf("week domain = %#v", weekDomain)
	}
	groupDomain, ok := months[0]["__domain"].([]any)
	if !ok || len(groupDomain) != 2 {
		t.Fatalf("month domain = %#v", months[0]["__domain"])
	}
	start, ok := groupDomain[0].([]any)
	if !ok || len(start) != 3 || start[0] != "date_value" || start[1] != ">=" || start[2] != "2026-01-01" {
		t.Fatalf("month start condition = %#v", groupDomain[0])
	}
	end, ok := groupDomain[1].([]any)
	if !ok || len(end) != 3 || end[0] != "date_value" || end[1] != "<" || end[2] != "2026-02-01" {
		t.Fatalf("month end condition = %#v", groupDomain[1])
	}
	groupRange, ok := months[0]["__range"].(map[string]any)
	if !ok {
		t.Fatalf("month range = %#v", months[0]["__range"])
	}
	dateRange, ok := groupRange["date_value:month"].(map[string]any)
	if !ok || dateRange["from"] != "2026-01-01" || dateRange["to"] != "2026-02-01" {
		t.Fatalf("month date range = %#v", groupRange["date_value:month"])
	}
	defaultMonths, err := records.ReadGroup(domain.Cond("name", domain.In, []any{"a", "b", "c"}), ReadGroupOptions{GroupBy: []string{"date_value"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultMonths) != 2 || defaultMonths[0]["date_value"] != "January 2026" || defaultMonths[0]["date_value_count"] != 2 {
		t.Fatalf("default month groups = %#v", defaultMonths)
	}
	formatted, err := records.FormattedReadGroup(domain.Cond("name", domain.In, []any{"a", "b", "c"}), ReadGroupOptions{GroupBy: []string{"date_value:month"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 2 {
		t.Fatalf("formatted month groups = %#v", formatted)
	}
	formattedMonth, ok := formatted[0]["date_value:month"].([]any)
	if !ok || len(formattedMonth) != 2 || formattedMonth[0] != "2026-01-01" || formattedMonth[1] != "January 2026" {
		t.Fatalf("formatted month value = %#v", formatted[0]["date_value:month"])
	}
	if _, ok := formatted[0]["__domain"]; ok {
		t.Fatalf("formatted group leaked __domain = %#v", formatted[0])
	}
	formattedDomain, ok := formatted[0]["__extra_domain"].([]any)
	if !ok || len(formattedDomain) != 3 || formattedDomain[0] != "&" {
		t.Fatalf("formatted extra domain = %#v", formatted[0]["__extra_domain"])
	}
	if _, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"date_value:bogus"}}); err == nil || !strings.Contains(err.Error(), `invalid interval "bogus"`) {
		t.Fatalf("invalid interval error = %v", err)
	}
}

func TestModelSetReadGroupNumericDateParts(t *testing.T) {
	registry := NewRegistry()
	sample := model.New("x.read.group.date.part", "x_read_group_date_part")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	records := env.Model("x.read.group.date.part")
	for _, values := range []map[string]any{
		{"name": "sat", "date_value": "2022-01-29", "moment_value": "2022-01-29 13:55:12"},
		{"name": "sun", "date_value": "2022-01-30", "moment_value": "2022-01-30 13:54:14"},
		{"name": "mon", "date_value": "2022-01-31", "moment_value": "2022-01-31 15:55:14"},
		{"name": "tue", "date_value": "2022-02-01", "moment_value": "2022-02-01 14:54:13"},
		{"name": "may", "date_value": "2022-05-29", "moment_value": "2022-05-29 14:55:13"},
		{"name": "next", "date_value": "2023-01-29", "moment_value": "2023-01-29 15:55:13"},
		{"name": "q4", "date_value": "2022-10-15", "moment_value": "2022-10-15 01:02:03"},
		{"name": "empty"},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	groupCounts := func(groupBy string, key string) map[any]int {
		t.Helper()
		groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{groupBy}})
		if err != nil {
			t.Fatal(err)
		}
		countKey := strings.Split(groupBy, ":")[0] + "_count"
		out := map[any]int{}
		for _, group := range groups {
			count, ok := group[countKey].(int)
			if !ok {
				t.Fatalf("%s count = %#v", groupBy, group[countKey])
			}
			out[group[key]] = count
			if _, hasRange := group["__range"]; hasRange {
				t.Fatalf("%s numeric group emitted range: %#v", groupBy, group)
			}
		}
		return out
	}
	if got, want := groupCounts("date_value:month_number", "date_value:month_number"), (map[any]int{1: 4, 2: 1, 5: 1, 10: 1, false: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("month_number groups = %#v", got)
	}
	if got, want := groupCounts("date_value:quarter_number", "date_value:quarter_number"), (map[any]int{1: 5, 2: 1, 4: 1, false: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("quarter_number groups = %#v", got)
	}
	if got, want := groupCounts("date_value:day_of_week", "date_value:day_of_week"), (map[any]int{6: 2, 0: 3, 1: 1, 2: 1, false: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("day_of_week groups = %#v", got)
	}
	if got, want := groupCounts("moment_value:second_number", "moment_value:second_number"), (map[any]int{12: 1, 14: 2, 13: 3, 3: 1, false: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("second_number groups = %#v", got)
	}

	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"date_value:month_number"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 5 || formatted[0]["date_value:month_number"] != 1 {
		t.Fatalf("formatted month_number groups = %#v", formatted)
	}
	if _, hasRange := formatted[0]["__range"]; hasRange {
		t.Fatalf("formatted numeric group emitted range: %#v", formatted[0])
	}
	if _, hasDomain := formatted[0]["__domain"]; hasDomain {
		t.Fatalf("formatted numeric group leaked __domain: %#v", formatted[0])
	}
	extraDomain, ok := formatted[0]["__extra_domain"].([]any)
	if !ok || len(extraDomain) != 1 {
		t.Fatalf("formatted numeric extra domain = %#v", formatted[0]["__extra_domain"])
	}
	extraCondition, ok := extraDomain[0].([]any)
	if !ok || len(extraCondition) != 3 || extraCondition[0] != "date_value.month_number" || extraCondition[1] != "=" || extraCondition[2] != 1 {
		t.Fatalf("formatted numeric condition = %#v", extraDomain[0])
	}
	node, err := domain.Parse(formatted[0]["__extra_domain"])
	if err != nil {
		t.Fatal(err)
	}
	found, err := records.Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 4 {
		t.Fatalf("month_number search count = %d", found.Len())
	}

	last := formatted[len(formatted)-1]
	if last["date_value:month_number"] != false {
		t.Fatalf("formatted false numeric group = %#v", last)
	}
	falseExtraDomain := last["__extra_domain"].([]any)
	falseCondition := falseExtraDomain[0].([]any)
	if falseCondition[0] != "date_value.month_number" || falseCondition[1] != "=" || falseCondition[2] != false {
		t.Fatalf("formatted false numeric domain = %#v", falseExtraDomain)
	}
	falseNode, err := domain.Parse(falseExtraDomain)
	if err != nil {
		t.Fatal(err)
	}
	falseFound, err := records.Search(falseNode)
	if err != nil {
		t.Fatal(err)
	}
	if falseFound.Len() != 1 {
		t.Fatalf("false month_number search count = %d", falseFound.Len())
	}
}

func TestModelSetReadGroupNumericDatePartsUseContextTimezone(t *testing.T) {
	registry := NewRegistry()
	sample := model.New("x.read.group.date.part.tz", "x_read_group_date_part_tz")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1, Values: map[string]any{"tz": "Pacific/Auckland"}})
	records := env.Model("x.read.group.date.part.tz")
	if _, err := records.Create(map[string]any{"name": "a", "moment_value": "2023-02-05 23:55:00"}); err != nil {
		t.Fatal(err)
	}
	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"moment_value:iso_week_number"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0]["moment_value:iso_week_number"] != 6 || groups[0]["moment_value_count"] != 1 {
		t.Fatalf("timezone iso_week_number groups = %#v", groups)
	}
	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"moment_value:iso_week_number"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 1 || formatted[0]["moment_value:iso_week_number"] != 6 {
		t.Fatalf("formatted timezone iso_week_number groups = %#v", formatted)
	}
	extraDomain := formatted[0]["__extra_domain"].([]any)
	extraCondition := extraDomain[0].([]any)
	if extraCondition[0] != "moment_value.iso_week_number" || extraCondition[1] != "=" || extraCondition[2] != 6 {
		t.Fatalf("formatted timezone numeric domain = %#v", extraDomain)
	}
	node, err := domain.Parse(extraDomain)
	if err != nil {
		t.Fatal(err)
	}
	found, err := records.Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("timezone iso_week_number search count = %d", found.Len())
	}
}

func TestModelSetReadGroupDateTimeUsesContextTimezone(t *testing.T) {
	registry := NewRegistry()
	sample := model.New("x.read.group.tz", "x_read_group_tz")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1, Values: map[string]any{"tz": "Asia/Bahrain"}})
	records := env.Model("x.read.group.tz")
	for _, values := range []map[string]any{
		{"name": "a", "moment_value": "2026-01-01 20:30:00"},
		{"name": "b", "moment_value": "2026-01-01 22:30:00"},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	groups, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"moment_value:day"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0]["moment_value:day"] != "01 Jan 2026" || groups[0]["moment_value_count"] != 1 || groups[1]["moment_value:day"] != "02 Jan 2026" || groups[1]["moment_value_count"] != 1 {
		t.Fatalf("timezone day groups = %#v", groups)
	}
	secondDomain := groups[1]["__domain"].([]any)
	secondStart := secondDomain[0].([]any)
	secondEnd := secondDomain[1].([]any)
	if secondStart[0] != "moment_value" || secondStart[1] != ">=" || secondStart[2] != "2026-01-01 21:00:00" || secondEnd[0] != "moment_value" || secondEnd[1] != "<" || secondEnd[2] != "2026-01-02 21:00:00" {
		t.Fatalf("timezone second domain = %#v", secondDomain)
	}
	formatted, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"moment_value:day"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 2 {
		t.Fatalf("formatted timezone groups = %#v", formatted)
	}
	formattedSecond, ok := formatted[1]["moment_value:day"].([]any)
	if !ok || len(formattedSecond) != 2 || formattedSecond[0] != "2026-01-01 21:00:00" || formattedSecond[1] != "02 Jan 2026" {
		t.Fatalf("formatted timezone value = %#v", formatted[1]["moment_value:day"])
	}
	formattedSecondDomain := formatted[1]["__extra_domain"].([]any)
	formattedSecondStart := formattedSecondDomain[1].([]any)
	if formattedSecondStart[2] != "2026-01-01 21:00:00" {
		t.Fatalf("formatted timezone domain = %#v", formattedSecondDomain)
	}
	utcRecords := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"tz": "Mars/Base"}}).Model("x.read.group.tz")
	utcGroups, err := utcRecords.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"moment_value:day"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(utcGroups) != 1 || utcGroups[0]["moment_value:day"] != "01 Jan 2026" || utcGroups[0]["moment_value_count"] != 2 {
		t.Fatalf("invalid timezone groups = %#v", utcGroups)
	}
}

func TestModelSetReadGroupWeekStartVariants(t *testing.T) {
	registry := NewRegistry()
	lang := model.New("res.lang", "res_lang")
	lang.AddField(field.New("code", field.Char))
	lang.AddField(field.New("week_start", field.Char))
	if err := registry.Register(lang); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.week.variant", "x_read_group_week_variant")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("date_value", field.Date))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	for _, values := range []map[string]any{
		{"code": "fr_BE", "week_start": "1"},
		{"code": "ar_SY", "week_start": "6"},
		{"code": "en_US", "week_start": "7"},
		{"code": "broken", "week_start": "0"},
	} {
		if _, err := env.Model("res.lang").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	records := env.Model("x.read.group.week.variant")
	for _, values := range []map[string]any{
		{"name": "a", "date_value": "2022-01-28"},
		{"name": "b", "date_value": "2022-01-29"},
		{"name": "c", "date_value": "2022-01-30"},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		lang  string
		want  []string
		count []int
	}{
		{lang: "fr_BE", want: []string{"2022-01-24"}, count: []int{3}},
		{lang: "ar_SY", want: []string{"2022-01-22", "2022-01-29"}, count: []int{1, 2}},
		{lang: "en_US", want: []string{"2022-01-23", "2022-01-30"}, count: []int{2, 1}},
		{lang: "missing", want: []string{"2022-01-23", "2022-01-30"}, count: []int{2, 1}},
		{lang: "broken", want: []string{"2022-01-23", "2022-01-30"}, count: []int{2, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			got, err := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"lang": tc.lang}}).Model("x.read.group.week.variant").ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"date_value:week"}})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("groups = %#v", got)
			}
			for index, want := range tc.want {
				groupRange, _ := got[index]["__range"].(map[string]any)
				weekRange, _ := groupRange["date_value:week"].(map[string]any)
				if weekRange["from"] != want || got[index]["date_value_count"] != tc.count[index] {
					t.Fatalf("groups = %#v", got)
				}
			}
		})
	}
}

func TestModelSetReadGroupPropertyDateIntervals(t *testing.T) {
	registry := NewRegistry()
	definition := model.New("x.read.group.property.definition", "x_read_group_property_definition")
	definition.AddField(field.New("name", field.Char))
	definition.AddField(field.New("properties_definition", field.PropertiesDefinition))
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.property", "x_read_group_property")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("definition_id", field.Many2One).WithRelation("x.read.group.property.definition"))
	sample.AddField(field.New("properties", field.Properties).WithPropertyDefinition("definition_id", "properties_definition"))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	definitionID, err := env.Model("x.read.group.property.definition").Create(map[string]any{
		"name": "Definition",
		"properties_definition": []map[string]any{
			{"name": "date_prop", "type": "date", "string": "Date"},
			{"name": "datetime_prop", "type": "datetime", "string": "DateTime"},
			{"name": "selection_prop", "type": "selection", "string": "Selection", "selection": []any{[]any{"one", "One"}, []any{"two", "Two"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.property")
	for _, values := range []map[string]any{
		{"name": "a", "definition_id": definitionID, "properties": map[string]any{"date_prop": "2026-01-10", "selection_prop": "one"}},
		{"name": "b", "definition_id": definitionID, "properties": map[string]any{"date_prop": "2026-01-20"}},
		{"name": "c", "definition_id": definitionID, "properties": map[string]any{"date_prop": "2026-02-05", "selection_prop": "two"}},
		{"name": "d", "definition_id": definitionID, "properties": `{"date_prop":"2026-01-25","datetime_prop":"2023-02-05 23:55:00"}`},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}
	months, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.date_prop:month"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 || months[0]["properties.date_prop:month"] != "January 2026" || months[0]["properties.date_prop_count"] != 3 || months[1]["properties.date_prop:month"] != "February 2026" || months[1]["properties.date_prop_count"] != 1 {
		t.Fatalf("property month groups = %#v", months)
	}
	groupDomain, ok := months[0]["__domain"].([]any)
	if !ok || len(groupDomain) != 2 {
		t.Fatalf("property month domain = %#v", months[0]["__domain"])
	}
	start, ok := groupDomain[0].([]any)
	if !ok || len(start) != 3 || start[0] != "properties.date_prop" || start[1] != ">=" || start[2] != "2026-01-01" {
		t.Fatalf("property month start condition = %#v", groupDomain[0])
	}
	end, ok := groupDomain[1].([]any)
	if !ok || len(end) != 3 || end[0] != "properties.date_prop" || end[1] != "<" || end[2] != "2026-02-01" {
		t.Fatalf("property month end condition = %#v", groupDomain[1])
	}
	groupRange, ok := months[0]["__range"].(map[string]any)
	if !ok {
		t.Fatalf("property month range = %#v", months[0]["__range"])
	}
	propertyRange, ok := groupRange["properties.date_prop:month"].(map[string]any)
	if !ok || propertyRange["from"] != "2026-01-01" || propertyRange["to"] != "2026-02-01" {
		t.Fatalf("property month date range = %#v", groupRange["properties.date_prop:month"])
	}
	formattedMonths, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.date_prop:month"}})
	if err != nil {
		t.Fatal(err)
	}
	formattedMonth, ok := formattedMonths[0]["properties.date_prop:month"].([]any)
	if !ok || len(formattedMonth) != 2 || formattedMonth[0] != "2026-01-01" || formattedMonth[1] != "January 2026" {
		t.Fatalf("formatted property month = %#v", formattedMonths[0]["properties.date_prop:month"])
	}
	extraDomain, ok := formattedMonths[0]["__extra_domain"].([]any)
	if !ok || len(extraDomain) != 3 || extraDomain[0] != "&" {
		t.Fatalf("formatted property extra domain = %#v", formattedMonths[0]["__extra_domain"])
	}
	formattedMonthNode, err := domain.Parse(formattedMonths[0]["__extra_domain"])
	if err != nil {
		t.Fatal(err)
	}
	formattedMonthFound, err := records.Search(formattedMonthNode)
	if err != nil {
		t.Fatal(err)
	}
	if formattedMonthFound.Len() != 3 {
		t.Fatalf("formatted property month search count = %d", formattedMonthFound.Len())
	}
	if _, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.date_prop:month_number"}}); err == nil || !strings.Contains(err.Error(), `not supported for property field`) {
		t.Fatalf("property numeric interval error = %v", err)
	}
	invalidPropertyPart := domain.Cond("properties.date_prop.month_number", domain.Equal, int64(1))
	if _, err := records.Search(invalidPropertyPart); err == nil || !strings.Contains(err.Error(), `unsupported property path`) {
		t.Fatalf("property date-part domain error = %v", err)
	}
	tzRecords := env.WithContext(Context{UserID: 1, CompanyID: 1, Values: map[string]any{"tz": "Pacific/Auckland"}}).Model("x.read.group.property")
	if _, err := tzRecords.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.datetime_prop:iso_week_number"}}); err == nil || !strings.Contains(err.Error(), `not supported for property field`) {
		t.Fatalf("property datetime numeric interval error = %v", err)
	}
	selections, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.selection_prop"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 3 || selections[0]["properties.selection_prop"] != "one" || selections[1]["properties.selection_prop"] != "two" || selections[2]["properties.selection_prop"] != false {
		t.Fatalf("property selection groups = %#v", selections)
	}
	if _, err := records.ReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.selection_prop:month"}}); err == nil || !strings.Contains(err.Error(), `requires date or datetime field`) {
		t.Fatalf("property invalid interval error = %v", err)
	}
}

func TestModelSetReadGroupPropertyRelationalTagsAndInvalidValues(t *testing.T) {
	registry := NewRegistry()
	partner := model.New("x.read.group.property.partner", "x_read_group_property_partner")
	partner.AddField(field.New("name", field.Char))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	definition := model.New("x.read.group.property.relation.definition", "x_read_group_property_relation_definition")
	definition.AddField(field.New("name", field.Char))
	definition.AddField(field.New("properties_definition", field.PropertiesDefinition))
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.read.group.property.relation", "x_read_group_property_relation")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("definition_id", field.Many2One).WithRelation("x.read.group.property.relation.definition"))
	sample.AddField(field.New("properties", field.Properties).WithPropertyDefinition("definition_id", "properties_definition"))
	if err := registry.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := NewEnv(registry, Context{UserID: 1, CompanyID: 1})
	partnerA, err := env.Model("x.read.group.property.partner").Create(map[string]any{"name": "Partner A"})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := env.Model("x.read.group.property.partner").Create(map[string]any{"name": "Partner B"})
	if err != nil {
		t.Fatal(err)
	}
	definitionID, err := env.Model("x.read.group.property.relation.definition").Create(map[string]any{
		"name": "Definition",
		"properties_definition": []map[string]any{
			{"name": "selection_prop", "type": "selection", "string": "Selection", "selection": []any{[]any{"open", "Open"}, []any{"done", "Done"}}},
			{"name": "owner_prop", "type": "many2one", "string": "Owner", "comodel": "x.read.group.property.partner"},
			{"name": "watcher_prop", "type": "many2many", "string": "Watchers", "comodel": "x.read.group.property.partner"},
			{"name": "tags_prop", "type": "tags", "string": "Tags", "tags": []any{[]any{"red", "Red", 3}, []any{"blue", "Blue", 4}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	records := env.Model("x.read.group.property.relation")
	for _, values := range []map[string]any{
		{"name": "valid a", "definition_id": definitionID, "properties": map[string]any{"selection_prop": "open", "owner_prop": partnerA, "watcher_prop": []int64{partnerA, partnerB}, "tags_prop": []any{"red", "blue"}}},
		{"name": "invalid only", "definition_id": definitionID, "properties": map[string]any{"selection_prop": "removed", "owner_prop": int64(999), "watcher_prop": []int64{999}, "tags_prop": []any{"removed"}}},
		{"name": "missing", "definition_id": definitionID, "properties": map[string]any{}},
		{"name": "valid b", "definition_id": definitionID, "properties": map[string]any{"owner_prop": partnerB, "watcher_prop": []int64{partnerB}, "tags_prop": []any{"blue"}}},
	} {
		if _, err := records.Create(values); err != nil {
			t.Fatal(err)
		}
	}

	selectionRows, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.selection_prop"}})
	if err != nil {
		t.Fatal(err)
	}
	selectionOpen := readGroupTestFindScalar(selectionRows, "properties.selection_prop", "open")
	selectionFalse := readGroupTestFindScalar(selectionRows, "properties.selection_prop", false)
	if numericID(selectionOpen["__count"]) != 1 || numericID(selectionFalse["__count"]) != 3 {
		t.Fatalf("selection groups = %#v", selectionRows)
	}
	readGroupTestAssertDomainCount(t, records, selectionFalse["__extra_domain"], 3)

	ownerRows, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.owner_prop"}})
	if err != nil {
		t.Fatal(err)
	}
	ownerA := readGroupTestFindPair(ownerRows, "properties.owner_prop", partnerA)
	ownerB := readGroupTestFindPair(ownerRows, "properties.owner_prop", partnerB)
	ownerFalse := readGroupTestFindScalar(ownerRows, "properties.owner_prop", false)
	if readGroupTestPairLabel(ownerA["properties.owner_prop"]) != "Partner A" || readGroupTestPairLabel(ownerB["properties.owner_prop"]) != "Partner B" || numericID(ownerFalse["__count"]) != 2 {
		t.Fatalf("owner groups = %#v", ownerRows)
	}
	readGroupTestAssertDomainCount(t, records, ownerFalse["__extra_domain"], 2)

	watcherRows, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.watcher_prop"}})
	if err != nil {
		t.Fatal(err)
	}
	watcherA := readGroupTestFindPair(watcherRows, "properties.watcher_prop", partnerA)
	watcherB := readGroupTestFindPair(watcherRows, "properties.watcher_prop", partnerB)
	watcherFalse := readGroupTestFindScalar(watcherRows, "properties.watcher_prop", false)
	if numericID(watcherA["__count"]) != 1 || numericID(watcherB["__count"]) != 2 || numericID(watcherFalse["__count"]) != 2 {
		t.Fatalf("watcher groups = %#v", watcherRows)
	}
	readGroupTestAssertDomainCount(t, records, watcherB["__extra_domain"], 2)
	readGroupTestAssertDomainCount(t, records, watcherFalse["__extra_domain"], 2)

	tagRows, err := records.FormattedReadGroup(domain.And(), ReadGroupOptions{GroupBy: []string{"properties.tags_prop"}})
	if err != nil {
		t.Fatal(err)
	}
	tagRed := readGroupTestFindPair(tagRows, "properties.tags_prop", "red")
	tagBlue := readGroupTestFindPair(tagRows, "properties.tags_prop", "blue")
	tagFalse := readGroupTestFindScalar(tagRows, "properties.tags_prop", false)
	if numericID(tagRed["__count"]) != 1 || numericID(tagBlue["__count"]) != 2 || numericID(tagFalse["__count"]) != 2 {
		t.Fatalf("tag groups = %#v", tagRows)
	}
	tagBlueValue, ok := tagBlue["properties.tags_prop"].([]any)
	if !ok || len(tagBlueValue) != 3 || tagBlueValue[1] != "Blue" || numericID(tagBlueValue[2]) != 4 {
		t.Fatalf("tag blue value = %#v", tagBlue["properties.tags_prop"])
	}
	readGroupTestAssertDomainCount(t, records, tagBlue["__extra_domain"], 2)
	readGroupTestAssertDomainCount(t, records, tagFalse["__extra_domain"], 2)
}

func readGroupTestFindScalar(rows []map[string]any, key string, value any) map[string]any {
	for _, row := range rows {
		if valuesEqual(row[key], value) {
			return row
		}
	}
	return map[string]any{}
}

func readGroupTestFindPair(rows []map[string]any, key string, id any) map[string]any {
	for _, row := range rows {
		pair, ok := row[key].([]any)
		if ok && len(pair) > 0 && valuesEqual(pair[0], id) {
			return row
		}
	}
	return map[string]any{}
}

func readGroupTestPairLabel(value any) string {
	pair, ok := value.([]any)
	if !ok || len(pair) < 2 {
		return ""
	}
	return stringValue(pair[1])
}

func readGroupTestAssertDomainCount(t *testing.T, records ModelSet, rawDomain any, want int64) {
	t.Helper()
	node, err := domain.Parse(rawDomain)
	if err != nil {
		t.Fatal(err)
	}
	found, err := records.Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if int64(found.Len()) != want {
		t.Fatalf("domain %v count = %d, want %d", rawDomain, found.Len(), want)
	}
}

func TestCreateUsesPersistedSequenceForDelegationAndCancellation(t *testing.T) {
	sequencecore.ResetForTesting()
	env := sequenceNameEnv(t)
	if _, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Delegation",
		"code":             "delegation",
		"prefix":           "DEL/",
		"padding":          int64(5),
		"number_next":      int64(1),
		"number_increment": int64(1),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Cancellation",
		"code":             "cancellation.record",
		"prefix":           "CAN",
		"padding":          int64(4),
		"number_next":      int64(9),
		"number_increment": int64(1),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	delegationID, err := env.Model("delegation").Create(map[string]any{"name": "Manual"})
	if err != nil {
		t.Fatal(err)
	}
	cancellationID, err := env.Model("cancellation.record").Create(map[string]any{"name": "Manual", "model": "res.partner", "record_id": int64(4)})
	if err != nil {
		t.Fatal(err)
	}
	delegationRows, err := env.Model("delegation").Browse(delegationID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	cancellationRows, err := env.Model("cancellation.record").Browse(cancellationID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if delegationRows[0]["name"] != "DEL/00001" || cancellationRows[0]["name"] != "CAN0009" {
		t.Fatalf("sequence names delegation=%+v cancellation=%+v", delegationRows, cancellationRows)
	}
	sequenceRows, err := env.Model("ir.sequence").Search(domain.Cond("code", domain.In, []string{"delegation", "cancellation.record"}))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := sequenceRows.Read("code", "number_next")
	if err != nil {
		t.Fatal(err)
	}
	nextByCode := map[string]int64{}
	for _, row := range rows {
		nextByCode[row["code"].(string)] = row["number_next"].(int64)
	}
	if nextByCode["delegation"] != 1 || nextByCode["cancellation.record"] != 9 {
		t.Fatalf("sequence counters = %+v", rows)
	}
}

func TestCreateUsesNameSequenceMixinOnlyForDefaultNames(t *testing.T) {
	sequencecore.ResetForTesting()
	env := sequenceNameEnv(t)
	if _, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Mixed",
		"code":             "x.mixed",
		"prefix":           "M/",
		"padding":          int64(3),
		"number_next":      int64(4),
		"number_increment": int64(1),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("x.mixed").Create(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("x.mixed").Create(map[string]any{"name": "New"})
	if err != nil {
		t.Fatal(err)
	}
	manualID, err := env.Model("x.mixed").Create(map[string]any{"name": "Manual"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("x.mixed").Browse(firstID, secondID, manualID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "M/004" || rows[1]["name"] != "M/005" || rows[2]["name"] != "Manual" {
		t.Fatalf("mixin rows = %+v", rows)
	}
}

var errDenied = errors.New("denied")

type denyPolicy struct{}

func (denyPolicy) Check(Context, string, Operation, map[string]any) error {
	return errDenied
}

func (denyPolicy) CheckRecord(Context, string, Operation, map[string]any) (bool, error) {
	return true, nil
}

func (denyPolicy) FilterFields(_ Context, _ string, fields []string) []string {
	return fields
}

type fieldFilterPolicy struct {
	blocked map[string]bool
}

func (fieldFilterPolicy) Check(Context, string, Operation, map[string]any) error {
	return nil
}

func (fieldFilterPolicy) CheckRecord(Context, string, Operation, map[string]any) (bool, error) {
	return true, nil
}

func (p fieldFilterPolicy) FilterFields(_ Context, _ string, fields []string) []string {
	out := fields[:0]
	for _, name := range fields {
		if !p.blocked[name] {
			out = append(out, name)
		}
	}
	return out
}

type denyNthCreatePolicy struct {
	modelName      string
	allowedCreates int
	err            error
}

func (p *denyNthCreatePolicy) Check(_ Context, modelName string, op Operation, _ map[string]any) error {
	if modelName == p.modelName && op == OpCreate {
		if p.allowedCreates > 0 {
			p.allowedCreates--
			return nil
		}
		return p.err
	}
	return nil
}

func (*denyNthCreatePolicy) CheckRecord(Context, string, Operation, map[string]any) (bool, error) {
	return true, nil
}

func (*denyNthCreatePolicy) FilterFields(_ Context, _ string, fields []string) []string {
	return fields
}

func testEnv(t *testing.T) *Env {
	t.Helper()
	registry := NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	partner.AddField(field.New("age", field.Int))
	partner.AddField(field.New("tag_ids", field.Many2Many).WithRelation("res.partner.category"))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	return NewEnv(registry, Context{UserID: 1, CompanyID: 1})
}

func baseRecordEnv(t *testing.T) *Env {
	t.Helper()
	registry := NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	return NewEnv(registry, Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
}

func TestPhoneBlacklistNormalizesAndSyncsPartnerState(t *testing.T) {
	env := baseRecordEnv(t)
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "SMS Partner", "active": true, "phone": "+1 (555) 000-3030"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(partnerID).Read("phone_sanitized", "phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["phone_sanitized"] != "+15550003030" || rows[0]["phone_blacklisted"] != false {
		t.Fatalf("initial partner rows = %+v", rows)
	}

	blacklistID, err := env.Model("phone.blacklist").Create(map[string]any{"number": "+1 555 000 3030"})
	if err != nil {
		t.Fatal(err)
	}
	blacklistRows, err := env.Model("phone.blacklist").Browse(blacklistID).Read("number", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(blacklistRows) != 1 || blacklistRows[0]["number"] != "+15550003030" || blacklistRows[0]["active"] != true {
		t.Fatalf("blacklist rows = %+v", blacklistRows)
	}
	rows, err = env.Model("res.partner").Browse(partnerID).Read("phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["phone_blacklisted"] != true {
		t.Fatalf("blacklisted partner rows = %+v", rows)
	}

	if err := env.Model("phone.blacklist").Browse(blacklistID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(partnerID).Read("phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["phone_blacklisted"] != false {
		t.Fatalf("deactivated partner rows = %+v", rows)
	}
}

func TestPhoneNormalizationUsesCountryAwareE164(t *testing.T) {
	env := baseRecordEnv(t)
	beID, err := env.Model("res.country").Create(map[string]any{"name": "Belgium", "code": "BE", "phone_code": int64(32), "active": true})
	if err != nil {
		t.Fatal(err)
	}
	usID, err := env.Model("res.country").Create(map[string]any{"name": "United States", "code": "US", "phone_code": int64(1), "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.company").Create(map[string]any{"name": "BE Company", "country_id": beID, "active": true}); err != nil {
		t.Fatal(err)
	}

	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "BE SMS", "country_id": beID, "active": true, "phone": "0456 04 05 06"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(partnerID).Read("phone_sanitized", "phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["phone_sanitized"] != "+32456040506" || rows[0]["phone_blacklisted"] != false {
		t.Fatalf("country normalized partner rows = %+v", rows)
	}

	if err := env.Model("res.partner").Browse(partnerID).Write(map[string]any{"country_id": usID}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("res.partner").Browse(partnerID).Read("phone_sanitized")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["phone_sanitized"] != "+1456040506" {
		t.Fatalf("country recomputed partner rows = %+v", rows)
	}

	contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "BE Contact", "phone": "0032 456 07 08 09", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactRows, err := env.Model("mailing.contact").Browse(contactID).Read("phone_sanitized")
	if err != nil {
		t.Fatal(err)
	}
	if len(contactRows) != 1 || contactRows[0]["phone_sanitized"] != "+32456070809" {
		t.Fatalf("company fallback contact rows = %+v", contactRows)
	}
}

func TestLinkTrackerCreateGeneratesCodeAndRedirectedURL(t *testing.T) {
	env := baseRecordEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Launch Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Newsletter"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Email"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := env.Model("link.tracker").Create(map[string]any{
		"url":         "https://example.com/a?x=1",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("link.tracker").Browse(linkID).Read("code", "short_url", "short_url_host", "redirected_url", "title", "count")
	if err != nil {
		t.Fatal(err)
	}
	code := stringValue(rows[0]["code"])
	if len(code) < 3 || rows[0]["short_url"] != "https://gorp.example/r/"+code || rows[0]["short_url_host"] != "https://gorp.example/r/" || rows[0]["title"] != "https://example.com/a?x=1" || numericID(rows[0]["count"]) != 0 {
		t.Fatalf("link tracker row = %+v", rows[0])
	}
	redirected, err := url.Parse(stringValue(rows[0]["redirected_url"]))
	if err != nil {
		t.Fatal(err)
	}
	query := redirected.Query()
	if query.Get("x") != "1" || query.Get("utm_campaign") != "Launch Campaign" || query.Get("utm_source") != "Newsletter" || query.Get("utm_medium") != "Email" {
		t.Fatalf("redirected query = %s row=%+v", redirected.RawQuery, rows[0])
	}
	codes, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	codeRows, err := codes.Read("code", "link_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(codeRows) != 1 || codeRows[0]["code"] != code || numericID(codeRows[0]["link_id"]) != linkID {
		t.Fatalf("code rows = %+v", codeRows)
	}
}

func TestLinkTrackerSearchOrCreatePreservesOrderAndDuplicates(t *testing.T) {
	env := baseRecordEnv(t)
	existingID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/a", "label": ""})
	if err != nil {
		t.Fatal(err)
	}
	bVals := map[string]any{"url": "https://example.com/b", "label": "B"}
	cVals := map[string]any{"url": "https://example.com/c", "label": "C"}
	first, err := LinkTrackerSearchOrCreate(env, []map[string]any{bVals, cVals, map[string]any{"url": "https://example.com/a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || first[2] != existingID || first[0] == 0 || first[1] == 0 || first[0] == first[1] || first[0] == existingID || first[1] == existingID {
		t.Fatalf("first ids = %+v existing=%d", first, existingID)
	}
	second, err := LinkTrackerSearchOrCreate(env, []map[string]any{bVals, cVals, map[string]any{"url": "https://example.com/a", "label": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeat ids = %+v want %+v", second, first)
	}
	duplicates, err := LinkTrackerSearchOrCreate(env, []map[string]any{cVals, map[string]any{"url": "https://example.com/a"}, cVals, map[string]any{"url": "https://example.com/a", "label": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicates) != 4 || duplicates[0] != first[1] || duplicates[2] != first[1] || duplicates[1] != existingID || duplicates[3] != existingID {
		t.Fatalf("duplicate ids = %+v first=%+v existing=%d", duplicates, first, existingID)
	}
	newDuplicates, err := LinkTrackerSearchOrCreate(env, []map[string]any{map[string]any{"url": "https://example.com/d"}, map[string]any{"url": "https://example.com/d", "label": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if len(newDuplicates) != 2 || newDuplicates[0] == 0 || newDuplicates[0] != newDuplicates[1] {
		t.Fatalf("new duplicate ids = %+v", newDuplicates)
	}
	all, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if all.Len() != 4 {
		t.Fatalf("tracker count = %d", all.Len())
	}
}

func TestLinkTrackerRandomCodeGeneratorGrowsOnCollision(t *testing.T) {
	env := baseRecordEnv(t)
	oldRandom := linkTrackerRandomRead
	defer func() { linkTrackerRandomRead = oldRandom }()
	calls := [][]byte{
		{0, 0, 0},
		{0, 0, 0},
		{1, 1, 1, 1},
		{2, 2, 2, 2},
	}
	linkTrackerRandomRead = func(b []byte) (int, error) {
		if len(calls) == 0 {
			t.Fatalf("unexpected random read length %d", len(b))
		}
		copy(b, calls[0])
		calls = calls[1:]
		return len(b), nil
	}
	codes, err := RandomLinkTrackerCodes(env, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(codes, []string{"bbbb", "cccc"}) {
		t.Fatalf("codes = %+v", codes)
	}
}

func TestLinkTrackerWriteRecomputesUTMAndReusesCode(t *testing.T) {
	env := baseRecordEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Updated Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Updated Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Updated Medium"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/original", "code": "Keep1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("link.tracker").Browse(linkID).Write(map[string]any{
		"url":         "https://example.com/updated?x=1",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("link.tracker").Browse(linkID).Read("code", "short_url", "redirected_url")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["code"] != "Keep1" || rows[0]["short_url"] != "https://gorp.example/r/Keep1" {
		t.Fatalf("link row = %+v", rows[0])
	}
	redirected, err := url.Parse(stringValue(rows[0]["redirected_url"]))
	if err != nil {
		t.Fatal(err)
	}
	query := redirected.Query()
	if query.Get("x") != "1" || query.Get("utm_campaign") != "Updated Campaign" || query.Get("utm_source") != "Updated Source" || query.Get("utm_medium") != "Updated Medium" {
		t.Fatalf("redirected query = %s row=%+v", redirected.RawQuery, rows[0])
	}
	codes, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if codes.Len() != 1 {
		t.Fatalf("code count = %d", codes.Len())
	}
}

func TestLinkTrackerCodeUniquenessOnCreateAndWrite(t *testing.T) {
	env := baseRecordEnv(t)
	firstID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/one", "code": "Same1"})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/two", "code": "Other1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("link.tracker.code").Create(map[string]any{"code": "Same1", "link_id": secondID}); err == nil {
		t.Fatalf("expected duplicate code create error")
	}
	secondCodes, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", domain.Equal, secondID))
	if err != nil {
		t.Fatal(err)
	}
	if secondCodes.Len() != 1 {
		t.Fatalf("second code count = %d first=%d", secondCodes.Len(), firstID)
	}
	secondRows, err := secondCodes.Read("id", "code")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("link.tracker.code").Browse(numericID(secondRows[0]["id"])).Write(map[string]any{"code": "Same1"}); err == nil {
		t.Fatalf("expected duplicate code write error")
	}
	afterRows, err := secondCodes.Read("code")
	if err != nil {
		t.Fatal(err)
	}
	if afterRows[0]["code"] != "Other1" {
		t.Fatalf("second code after failed write = %+v", afterRows[0])
	}
}

func TestLinkTrackerUniquenessAndURLValidation(t *testing.T) {
	env := baseRecordEnv(t)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Unique"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/a", "campaign_id": campaignID, "label": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/a", "campaign_id": campaignID}); err == nil {
		t.Fatalf("expected duplicate tracker error")
	}
	if _, err := env.Model("link.tracker").Create(map[string]any{"url": "?x=1"}); err == nil {
		t.Fatalf("expected query-only tracker URL error")
	}
	if _, err := env.Model("link.tracker").Create(map[string]any{"url": "#anchor"}); err == nil {
		t.Fatalf("expected anchor-only tracker URL error")
	}
}

func TestLinkTrackerNoExternalTrackingSuppressesUTM(t *testing.T) {
	env := baseRecordEnv(t)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "link_tracker.no_external_tracking", "value": "true"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "No Track"})
	if err != nil {
		t.Fatal(err)
	}
	externalID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://external.example/a?x=1", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	localID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://gorp.example/a?x=1", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("link.tracker").Browse(externalID, localID).Read("redirected_url")
	if err != nil {
		t.Fatal(err)
	}
	if stringValue(rows[0]["redirected_url"]) != "https://external.example/a?x=1" {
		t.Fatalf("external redirected = %+v", rows)
	}
	local, err := url.Parse(stringValue(rows[1]["redirected_url"]))
	if err != nil {
		t.Fatal(err)
	}
	if local.Query().Get("utm_campaign") != "No Track" {
		t.Fatalf("local redirected = %+v", rows[1])
	}
}

func TestSMSSMSUUIDDefaultAndUniqueness(t *testing.T) {
	env := baseRecordEnv(t)
	smsID, err := env.Model("sms.sms").Create(map[string]any{"number": "+15550101", "body": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("sms.sms").Browse(smsID).Read("uuid", "state", "to_delete")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !smsWebhookUUIDForRecordTest(stringValue(rows[0]["uuid"])) || rows[0]["state"] != "outgoing" || rows[0]["to_delete"] != false {
		t.Fatalf("sms defaults = %+v", rows)
	}
	if _, err := env.Model("sms.sms").Create(map[string]any{"uuid": rows[0]["uuid"], "number": "+15550102"}); err == nil {
		t.Fatalf("expected duplicate sms uuid create error")
	}
	secondID, err := env.Model("sms.sms").Create(map[string]any{"uuid": "11111111111111111111111111111111", "number": "+15550103"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("sms.sms").Browse(secondID).Write(map[string]any{"uuid": rows[0]["uuid"]}); err == nil {
		t.Fatalf("expected duplicate sms uuid write error")
	}
	secondRows, err := env.Model("sms.sms").Browse(secondID).Read("uuid")
	if err != nil {
		t.Fatal(err)
	}
	if secondRows[0]["uuid"] != "11111111111111111111111111111111" {
		t.Fatalf("second sms uuid after failed write = %+v", secondRows[0])
	}
}

func TestSMSTrackerUUIDRequiredAndUnique(t *testing.T) {
	env := baseRecordEnv(t)
	if _, err := env.Model("sms.tracker").Create(map[string]any{}); err == nil {
		t.Fatalf("expected missing tracker uuid create error")
	}
	firstID, err := env.Model("sms.tracker").Create(map[string]any{"sms_uuid": "22222222222222222222222222222222"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("sms.tracker").Create(map[string]any{"sms_uuid": "22222222222222222222222222222222"}); err == nil {
		t.Fatalf("expected duplicate tracker uuid create error")
	}
	secondID, err := env.Model("sms.tracker").Create(map[string]any{"sms_uuid": "33333333333333333333333333333333"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("sms.tracker").Browse(secondID).Write(map[string]any{"sms_uuid": "22222222222222222222222222222222"}); err == nil {
		t.Fatalf("expected duplicate tracker uuid write error")
	}
	rows, err := env.Model("sms.tracker").Browse(firstID, secondID).Read("sms_uuid")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["sms_uuid"] != "22222222222222222222222222222222" || rows[1]["sms_uuid"] != "33333333333333333333333333333333" {
		t.Fatalf("tracker uuid rows = %+v", rows)
	}
}

func smsWebhookUUIDForRecordTest(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' {
			continue
		}
		return false
	}
	return true
}

func TestWhatsAppTemplateAliasFieldsMirrorOnCreateAndWrite(t *testing.T) {
	env := baseRecordEnv(t)
	templateA, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Template A", "body": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	templateB, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Template B", "body": "Hi"})
	if err != nil {
		t.Fatal(err)
	}

	messageID, err := env.Model("whatsapp.message").Create(map[string]any{"state": "sent", "template_id": templateA})
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := env.Model("whatsapp.message").Browse(messageID).Read("template_id", "wa_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || numericID(messageRows[0]["template_id"]) != templateA || numericID(messageRows[0]["wa_template_id"]) != templateA {
		t.Fatalf("message create aliases = %+v", messageRows)
	}
	if err := env.Model("whatsapp.message").Browse(messageID).Write(map[string]any{"wa_template_id": templateB}); err != nil {
		t.Fatal(err)
	}
	messageRows, err = env.Model("whatsapp.message").Browse(messageID).Read("template_id", "wa_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || numericID(messageRows[0]["template_id"]) != templateB || numericID(messageRows[0]["wa_template_id"]) != templateB {
		t.Fatalf("message write aliases = %+v", messageRows)
	}

	buttonID, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Tracked", "wa_template_id": templateA, "button_type": "url", "url_type": "tracked"})
	if err != nil {
		t.Fatal(err)
	}
	buttonRows, err := env.Model("whatsapp.template.button").Browse(buttonID).Read("template_id", "wa_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(buttonRows) != 1 || numericID(buttonRows[0]["template_id"]) != templateA || numericID(buttonRows[0]["wa_template_id"]) != templateA {
		t.Fatalf("button create aliases = %+v", buttonRows)
	}
	if err := env.Model("whatsapp.template.button").Browse(buttonID).Write(map[string]any{"template_id": templateB}); err != nil {
		t.Fatal(err)
	}
	buttonRows, err = env.Model("whatsapp.template.button").Browse(buttonID).Read("template_id", "wa_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(buttonRows) != 1 || numericID(buttonRows[0]["template_id"]) != templateB || numericID(buttonRows[0]["wa_template_id"]) != templateB {
		t.Fatalf("button write aliases = %+v", buttonRows)
	}
}

func TestWhatsAppTemplateDefaultsAndValidation(t *testing.T) {
	env := baseRecordEnv(t)
	modelID, err := env.Model("ir.model").Create(map[string]any{"name": "Partner", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Defaulted", "body": "Hello", "model_id": modelID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("whatsapp.template").Browse(templateID).Read("status", "quality", "lang_code", "template_type", "header_type", "phone_field", "model")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["status"] != "draft" || rows[0]["quality"] != "none" || rows[0]["lang_code"] != "en" || rows[0]["template_type"] != "marketing" || rows[0]["header_type"] != "none" || rows[0]["phone_field"] != "phone" || rows[0]["model"] != "res.partner" {
		t.Fatalf("template defaults = %+v", rows)
	}
	if err := env.Model("whatsapp.template").Browse(templateID).Write(map[string]any{"quality": "UNKNOWN", "status": "APPROVED", "template_type": "UTILITY"}); err != nil {
		t.Fatal(err)
	}
	rows, err = env.Model("whatsapp.template").Browse(templateID).Read("quality", "status", "template_type", "model")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["quality"] != "none" || rows[0]["status"] != "approved" || rows[0]["template_type"] != "utility" || rows[0]["model"] != "res.partner" {
		t.Fatalf("template preserved fields = %+v", rows)
	}

	for _, spec := range []struct {
		name   string
		values map[string]any
	}{
		{"bad-status", map[string]any{"name": "Bad status", "status": "READY"}},
		{"bad-quality", map[string]any{"name": "Bad quality", "quality": "blue"}},
		{"bad-category", map[string]any{"name": "Bad category", "template_type": "sales"}},
		{"bad-header-index", map[string]any{"name": "Bad header", "header_type": "text", "header_text": "Hello {{2}}"}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			if _, err := env.Model("whatsapp.template").Create(spec.values); err == nil {
				t.Fatalf("expected validation error for %+v", spec.values)
			}
		})
	}
}

func TestWhatsAppTemplateVariableAndButtonValidation(t *testing.T) {
	env := baseRecordEnv(t)
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Variable Template", "body": "Hello {{1}} {{2}}", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "field", "field_name": "name", "demo_value": "Ada"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{3}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "Gap"}); err == nil {
		t.Fatalf("expected skipped body variable index error")
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "bad", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "Bad"}); err == nil {
		t.Fatalf("expected variable name format error")
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{2}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text"}); err == nil {
		t.Fatalf("expected missing demo value error")
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{2}}", "wa_template_id": templateID, "line_type": "body", "field_type": "field", "field_name": "missing_field", "demo_value": "Bad"}); err == nil {
		t.Fatalf("expected invalid field path error")
	}
	for i := 2; i <= 11; i++ {
		if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": fmt.Sprintf("{{%d}}", i), "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "Text"}); err != nil {
			t.Fatalf("body variable %d: %v", i, err)
		}
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{12}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "Too many"}); err == nil {
		t.Fatalf("expected body free text limit error")
	}

	buttonID, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Open", "wa_template_id": templateID, "button_type": "url", "url_type": "dynamic", "website_url": "https://example.com/"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "Other", "button_id": buttonID, "wa_template_id": templateID, "line_type": "button", "field_type": "free_text", "demo_value": "https://example.com/a"}); err == nil {
		t.Fatalf("expected button variable name mismatch error")
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Second URL", "wa_template_id": templateID, "button_type": "url", "url_type": "static", "website_url": "https://example.com/2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Third URL", "wa_template_id": templateID, "button_type": "url", "url_type": "static", "website_url": "https://example.com/3"}); err == nil {
		t.Fatalf("expected URL button limit error")
	}
}

func createRecordMailServer(t *testing.T, env *Env, ownerUserID int64) int64 {
	t.Helper()
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{
		"name":            "Personal",
		"active":          true,
		"smtp_host":       "127.0.0.1",
		"smtp_port":       int64(25),
		"smtp_encryption": "none",
		"from_filter":     "owner@example.com",
		"owner_user_id":   ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return serverID
}

func createRecordMailMessage(t *testing.T, env *Env, subject string) int64 {
	t.Helper()
	messageID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      subject,
		"body":         "<p>" + subject + "</p>",
		"message_type": "email",
	})
	if err != nil {
		t.Fatal(err)
	}
	return messageID
}

func securityDerivedFieldsEnv(t *testing.T) *Env {
	t.Helper()
	registry := NewRegistry()
	category := model.New("ir.module.category", "ir_module_category")
	category.AddField(field.New("name", field.Char))
	if err := registry.Register(category); err != nil {
		t.Fatal(err)
	}
	externalID := model.New("ir.model.data", "ir_model_data")
	externalID.AddField(field.New("module", field.Char))
	externalID.AddField(field.New("name", field.Char))
	externalID.AddField(field.New("model", field.Char))
	externalID.AddField(field.New("res_id", field.Int))
	if err := registry.Register(externalID); err != nil {
		t.Fatal(err)
	}
	privilege := model.New("res.groups.privilege", "res_groups_privilege")
	privilege.AddField(field.New("name", field.Char))
	privilege.AddField(field.New("description", field.Text))
	privilege.AddField(field.New("placeholder", field.Char))
	privilege.AddField(field.New("sequence", field.Int))
	privilege.AddField(field.New("category_id", field.Many2One).WithRelation("ir.module.category"))
	privilege.AddField(field.New("group_ids", field.One2Many).WithRelation("res.groups").WithRelationField("privilege_id"))
	if err := registry.Register(privilege); err != nil {
		t.Fatal(err)
	}
	groups := model.New("res.groups", "res_groups")
	groups.AddField(field.New("name", field.Char))
	groups.AddField(field.New("full_name", field.Char))
	groups.AddField(field.New("share", field.Bool))
	groups.AddField(field.New("sequence", field.Int))
	groups.AddField(field.New("privilege_id", field.Many2One).WithRelation("res.groups.privilege"))
	groups.AddField(field.New("implied_ids", field.Many2Many).WithRelation("res.groups"))
	groups.AddField(field.New("all_implied_ids", field.Many2Many).WithRelation("res.groups"))
	groups.AddField(field.New("implied_by_ids", field.Many2Many).WithRelation("res.groups"))
	groups.AddField(field.New("all_implied_by_ids", field.Many2Many).WithRelation("res.groups"))
	groups.AddField(field.New("disjoint_ids", field.Many2Many).WithRelation("res.groups"))
	groups.AddField(field.New("user_ids", field.Many2Many).WithRelation("res.users"))
	groups.AddField(field.New("all_user_ids", field.Many2Many).WithRelation("res.users"))
	groups.AddField(field.New("all_users_count", field.Int))
	groups.AddField(field.New("api_key_duration", field.Float))
	groups.AddField(field.New("view_group_hierarchy", field.Json))
	if err := registry.Register(groups); err != nil {
		t.Fatal(err)
	}
	access := model.New("ir.model.access", "ir_model_access")
	access.AddField(field.New("name", field.Char))
	access.AddField(field.New("group_id", field.Many2One).WithRelation("res.groups"))
	if err := registry.Register(access); err != nil {
		t.Fatal(err)
	}
	rule := model.New("ir.rule", "ir_rule")
	rule.AddField(field.New("name", field.Char))
	rule.AddField(field.New("groups", field.Many2Many).WithRelation("res.groups"))
	rule.AddField(field.New("group_ids", field.Many2Many).WithRelation("res.groups"))
	if err := registry.Register(rule); err != nil {
		t.Fatal(err)
	}
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	partner.AddField(field.New("is_company", field.Bool))
	partner.AddField(field.New("parent_id", field.Many2One).WithRelation("res.partner"))
	partner.AddField(field.New("commercial_partner_id", field.Many2One).WithRelation("res.partner"))
	partner.AddField(field.New("partner_share", field.Bool))
	if err := registry.Register(partner); err != nil {
		t.Fatal(err)
	}
	users := model.New("res.users", "res_users")
	users.AddField(field.New("login", field.Char))
	users.AddField(field.New("name", field.Char))
	users.AddField(field.New("active", field.Bool))
	users.AddField(field.New("active_partner", field.Bool))
	users.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	users.AddField(field.New("commercial_partner_id", field.Many2One).WithRelation("res.partner"))
	users.AddField(field.New("groups_id", field.Many2Many).WithRelation("res.groups"))
	users.AddField(field.New("group_ids", field.Many2Many).WithRelation("res.groups"))
	users.AddField(field.New("all_group_ids", field.Many2Many).WithRelation("res.groups"))
	users.AddField(field.New("accesses_count", field.Int))
	users.AddField(field.New("rules_count", field.Int))
	users.AddField(field.New("groups_count", field.Int))
	users.AddField(field.New("view_group_hierarchy", field.Json))
	users.AddField(field.New("role", field.Selection))
	users.AddField(field.New("share", field.Bool))
	if err := registry.Register(users); err != nil {
		t.Fatal(err)
	}
	return NewEnv(registry, Context{UserID: 1, CompanyID: 1})
}

func accountingRecordEnv(t *testing.T) *Env {
	t.Helper()
	registry := NewRegistry()
	company := model.New("res.company", "res_company")
	company.AddField(field.New("name", field.Char))
	company.AddField(field.New("parent_id", field.Many2One).WithRelation("res.company"))
	company.AddField(field.New("fiscalyear_lock_date", field.Date))
	company.AddField(field.New("tax_lock_date", field.Date))
	company.AddField(field.New("sale_lock_date", field.Date))
	company.AddField(field.New("purchase_lock_date", field.Date))
	company.AddField(field.New("hard_lock_date", field.Date))
	company.AddField(field.New("restrictive_audit_trail", field.Bool))
	if err := registry.Register(company); err != nil {
		t.Fatal(err)
	}
	for _, item := range coreaccounting.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	attachment := model.New("ir.attachment", "ir_attachment")
	attachment.AddField(field.New("name", field.Char))
	attachment.AddField(field.New("res_model", field.Char))
	attachment.AddField(field.New("res_field", field.Char))
	attachment.AddField(field.New("res_id", field.Int))
	attachment.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	attachment.AddField(field.New("mimetype", field.Char))
	attachment.AddField(field.New("datas", field.Binary))
	if err := registry.Register(attachment); err != nil {
		t.Fatal(err)
	}
	return NewEnv(registry, Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
}

func containsInt64(value any, target int64) bool {
	for _, id := range int64Values(value) {
		if id == target {
			return true
		}
	}
	return false
}

func createRecordAccount(t *testing.T, env *Env, code string, name string, kind coreaccounting.AccountKind) int64 {
	t.Helper()
	id, err := env.Model("account.account").Create(map[string]any{"code": code, "name": name, "account_type": string(kind), "company_id": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createRecordTax(t *testing.T, env *Env, name string, companyID int64, originalTaxIDs []int64, fiscalPositionIDs []int64) int64 {
	t.Helper()
	id, err := env.Model("account.tax").Create(map[string]any{
		"name":                name,
		"amount_type":         "percent",
		"amount":              int64(15),
		"type_tax_use":        "sale",
		"company_id":          companyID,
		"original_tax_ids":    originalTaxIDs,
		"fiscal_position_ids": fiscalPositionIDs,
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createRecordPostedMove(t *testing.T, env *Env, companyID int64, name string) int64 {
	t.Helper()
	receivableID := createRecordAccount(t, env, name+"-1100", "Receivable", coreaccounting.AccountReceivable)
	incomeID := createRecordAccount(t, env, name+"-4000", "Income", coreaccounting.AccountIncome)
	journalID, err := env.Model("account.journal").Create(map[string]any{"name": name + " Journal", "code": "SAJ", "type": string(coreaccounting.JournalSale), "company_id": companyID, "sequence_number_next": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	moveEnv := env.WithAccountMovePost()
	moveID, err := moveEnv.Model("account.move").Create(map[string]any{
		"name":          name,
		"date":          "2026-01-15",
		"invoice_date":  "2026-01-15",
		"state":         string(coreaccounting.MovePosted),
		"move_type":     "out_invoice",
		"journal_id":    journalID,
		"company_id":    companyID,
		"currency_id":   int64(1),
		"partner_id":    int64(7),
		"posted_before": true,
		"auto_post":     "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	line1, err := moveEnv.Model("account.move.line").Create(map[string]any{"move_id": moveID, "account_id": receivableID, "account_type": string(coreaccounting.AccountReceivable), "company_id": companyID, "partner_id": int64(7), "debit": int64(10000), "credit": int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	line2, err := moveEnv.Model("account.move.line").Create(map[string]any{"move_id": moveID, "account_id": incomeID, "account_type": string(coreaccounting.AccountIncome), "company_id": companyID, "debit": int64(0), "credit": int64(10000)})
	if err != nil {
		t.Fatal(err)
	}
	if err := moveEnv.Model("account.move").Browse(moveID).Write(map[string]any{"line_ids": []int64{line1, line2}}); err != nil {
		t.Fatal(err)
	}
	return moveID
}

func recordMoveLineIDs(t *testing.T, env *Env, moveID int64) []int64 {
	t.Helper()
	rows, err := env.Model("account.move").Browse(moveID).Read("line_ids")
	if err != nil {
		t.Fatal(err)
	}
	ids := int64Values(rows[0]["line_ids"])
	if len(ids) == 0 {
		t.Fatalf("move %d has no lines", moveID)
	}
	return ids
}

func accountMoveTrackingMessages(t *testing.T, env *Env, moveID int64) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "account.move"),
		domain.Cond("res_id", domain.Equal, moveID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "body", "message_type", "body_is_html", "tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func accountMoveTrackingValues(t *testing.T, env *Env, ids []int64) []map[string]any {
	t.Helper()
	rows, err := env.Model("mail.tracking.value").Browse(ids...).Read("field_name", "field_desc", "field_type", "old_value_integer", "new_value_integer", "old_value_char", "new_value_char", "old_value_float", "new_value_float", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func registerMailMessageModel(t *testing.T, env *Env) {
	t.Helper()
	message := model.New("mail.message", "mail_message")
	for _, item := range []field.Field{
		field.New("body", field.Text),
		field.New("message_type", field.Selection),
		field.New("model", field.Char),
		field.New("res_id", field.Int),
		field.New("date", field.DateTime),
		field.New("body_is_html", field.Bool),
		field.New("tracking_value_ids", field.One2Many).WithRelation("mail.tracking.value").WithRelationField("mail_message_id"),
	} {
		message.AddField(item)
	}
	if err := env.registry.Register(message); err != nil {
		t.Fatal(err)
	}
	tracking := model.New("mail.tracking.value", "mail_tracking_value")
	for _, item := range []field.Field{
		field.New("field_name", field.Char),
		field.New("field_desc", field.Char),
		field.New("field_type", field.Selection),
		field.New("old_value_integer", field.Int),
		field.New("new_value_integer", field.Int),
		field.New("old_value_char", field.Char),
		field.New("new_value_char", field.Char),
		field.New("old_value_float", field.Float),
		field.New("new_value_float", field.Float),
		field.New("mail_message_id", field.Many2One).WithRelation("mail.message"),
	} {
		tracking.AddField(item)
	}
	if err := env.registry.Register(tracking); err != nil {
		t.Fatal(err)
	}
}

func sequenceNameEnv(t *testing.T) *Env {
	t.Helper()
	registry := NewRegistry()
	sequence := model.New("ir.sequence", "ir_sequence")
	for _, item := range []field.Field{
		field.New("name", field.Char),
		field.New("code", field.Char),
		field.New("prefix", field.Char),
		field.New("suffix", field.Char),
		field.New("padding", field.Int),
		field.New("number_next", field.Int),
		field.New("number_next_actual", field.Int),
		field.New("number_increment", field.Int),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("active", field.Bool),
		field.New("implementation", field.Selection),
	} {
		sequence.AddField(item)
	}
	delegation := model.New("delegation", "delegation")
	delegation.AddField(field.New("name", field.Char))
	cancellation := model.New("cancellation.record", "cancellation_record")
	cancellation.AddField(field.New("name", field.Char))
	cancellation.AddField(field.New("model", field.Char))
	cancellation.AddField(field.New("record_id", field.Int))
	mixed := model.New("x.mixed", "x_mixed")
	mixed.Inherit = []string{"name.sequence.mixin"}
	mixed.AddField(field.New("name", field.Char))
	for _, item := range []model.Model{sequence, delegation, cancellation, mixed} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	return NewEnv(registry, Context{UserID: 1, CompanyID: 1})
}
