package hr

import (
	"reflect"
	"testing"

	"gorp/internal/base"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

func TestManifest(t *testing.T) {
	manifest := Manifest()
	if manifest.TechnicalName != ModuleName || !manifest.Installable || !manifest.Application {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !reflect.DeepEqual(manifest.Depends, []string{"base_setup", "digest", "phone_validation", "resource_mail", "web"}) {
		t.Fatalf("depends = %+v", manifest.Depends)
	}
}

func TestRegisterModels(t *testing.T) {
	reg := registry.New("test")
	manifests := append([]module.Manifest{base.Manifest(), {Name: "Mail", TechnicalName: "mail", Version: "19.0.1.0.0", Depends: []string{"base"}, Installable: true}}, DependencyManifests()...)
	manifests = append(manifests, Manifest())
	if err := reg.Install(manifests); err != nil {
		t.Fatal(err)
	}
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hr.department", "hr.employee", "hr.job", "hr.employee.category"} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
}

func TestModelFields(t *testing.T) {
	models := modelMap(Models())
	assertFields(t, models["hr.department"], "name", "complete_name", "active", "company_id", "parent_id", "child_ids", "manager_id", "member_ids", "has_read_access", "total_employee", "jobs_ids", "plan_ids", "plans_count", "note", "color", "parent_path", "master_department_id", "c_level_manager_id")
	assertFields(t, models["hr.employee"], "version_id", "current_version_id", "current_date_version", "version_ids", "versions_count", "name", "resource_id", "resource_calendar_id", "user_id", "user_partner_id", "share", "phone", "email", "active", "company_id", "work_phone", "mobile_phone", "work_email", "work_contact_id", "private_email", "department_id", "parent_id", "child_ids", "coach_id", "job_id", "job_title", "category_ids", "tz", "country_id", "identification_id", "barcode", "pin")
	assertFields(t, models["hr.job"], "active", "name", "sequence", "expected_employees", "no_of_employee", "no_of_recruitment", "employee_ids", "description", "requirements", "user_id", "allowed_user_ids", "department_id", "company_id", "contract_type_id")
	assertFields(t, models["hr.employee.category"], "name", "color", "employee_ids")
	if models["hr.department"].Fields["parent_id"].Relation != "hr.department" {
		t.Fatalf("department parent relation = %s", models["hr.department"].Fields["parent_id"].Relation)
	}
	if models["hr.employee"].Fields["department_id"].Relation != "hr.department" {
		t.Fatalf("employee department relation = %s", models["hr.employee"].Fields["department_id"].Relation)
	}
}

func TestExtensionModels(t *testing.T) {
	extensions := modelMap(ExtensionModels())
	users := extensions["res.users"]
	if !reflect.DeepEqual(users.Inherit, []string{"res.users"}) {
		t.Fatalf("res.users inherit = %+v", users.Inherit)
	}
	assertFields(t, users, "employee_ids", "employee_id", "job_title", "work_phone", "mobile_phone", "work_email", "category_ids", "work_contact_id", "work_location_id", "work_location_name", "work_location_type", "employee_count", "employee_resource_calendar_id", "create_employee", "create_employee_id")
	if users.Fields["employee_ids"].Relation != "hr.employee" || users.Fields["employee_ids"].RelationField != "user_id" {
		t.Fatalf("employee_ids field = %+v", users.Fields["employee_ids"])
	}
}

func TestRecordModelsCreateDepartmentHierarchy(t *testing.T) {
	models := map[string]model.Model{}
	var order []string
	add := func(m model.Model) {
		if existing, ok := models[m.Name]; ok {
			models[m.Name] = m.Compose(existing)
			return
		}
		models[m.Name] = m
		order = append(order, m.Name)
	}
	for _, group := range [][]model.Model{base.Models(), Models(), ExtensionModels()} {
		for _, m := range group {
			add(m)
		}
	}
	reg := record.NewRegistry()
	for _, name := range order {
		if err := reg.Register(models[name]); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 1})
	parentID, err := env.Model("hr.department").Create(map[string]any{"name": "Operations", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("hr.department").Create(map[string]any{"name": "Approvals", "active": true, "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Ada", "active": true, "department_id": childID})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("hr.employee").Browse(employeeID).Read("department_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["department_id"] != childID {
		t.Fatalf("employee rows = %+v", rows)
	}
	childRows, err := env.Model("hr.department").Browse(childID).Read("parent_id")
	if err != nil {
		t.Fatal(err)
	}
	if childRows[0]["parent_id"] != parentID {
		t.Fatalf("department rows = %+v", childRows)
	}
}

func modelMap(models []model.Model) map[string]model.Model {
	out := map[string]model.Model{}
	for _, m := range models {
		out[m.Name] = m
	}
	return out
}

func assertFields(t *testing.T, m model.Model, names ...string) {
	t.Helper()
	if m.Name == "" {
		t.Fatalf("missing model for fields %+v", names)
	}
	for _, name := range names {
		if _, ok := m.Fields[name]; !ok {
			t.Fatalf("missing %s on %s", name, m.Name)
		}
	}
}
