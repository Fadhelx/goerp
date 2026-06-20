package hr

import (
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

const ModuleName = "hr"

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "Employees",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Human Resources/Employees",
		Depends:       []string{"base_setup", "digest", "phone_validation", "resource_mail", "web"},
		Installable:   true,
		Application:   true,
		SourceVersion: "19.0.cleanroom",
		SourceLicense: "Clean-room metadata parity; Odoo source inspected, no copied source or assets",
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{Name: "Base Setup", TechnicalName: "base_setup", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
		{Name: "Digest", TechnicalName: "digest", Version: "19.0.1.0.0", Category: "Productivity", Depends: []string{"base"}, Installable: true},
		{Name: "Phone Validation", TechnicalName: "phone_validation", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
		{Name: "Resource", TechnicalName: "resource", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
		{Name: "Resource Mail", TechnicalName: "resource_mail", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"mail", "resource"}, Installable: true},
		{Name: "Web", TechnicalName: "web", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
	}
}

func RegisterModels(reg *registry.Registry) error {
	for _, m := range Models() {
		if err := reg.RegisterModel(m); err != nil {
			return err
		}
	}
	return nil
}

func RegisterRecordModels(reg *record.Registry) error {
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			return err
		}
	}
	return nil
}

func Models() []model.Model {
	return []model.Model{
		departmentModel(),
		employeeModel(),
		jobModel(),
		employeeCategoryModel(),
	}
}

func ExtensionModels() []model.Model {
	return []model.Model{
		extension("res.users", "res_users",
			field.New("employee_ids", field.One2Many).WithRelation("hr.employee").WithRelationField("user_id"),
			field.New("employee_id", field.Many2One).WithRelation("hr.employee"),
			field.New("job_title", field.Char),
			field.New("work_phone", field.Char),
			field.New("mobile_phone", field.Char),
			field.New("work_email", field.Char),
			field.New("category_ids", field.Many2Many).WithRelation("hr.employee.category"),
			field.New("work_contact_id", field.Many2One).WithRelation("res.partner"),
			field.New("work_location_id", field.Many2One).WithRelation("hr.work.location"),
			field.New("work_location_name", field.Char),
			field.New("work_location_type", field.Selection),
			field.New("employee_count", field.Int),
			field.New("employee_resource_calendar_id", field.Many2One).WithRelation("resource.calendar"),
			field.New("create_employee", field.Bool),
			field.New("create_employee_id", field.Many2One).WithRelation("hr.employee"),
		),
	}
}

func ModelNames() []string {
	models := Models()
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	return names
}

func departmentModel() model.Model {
	m := simple("hr.department", "hr_department",
		required(field.New("name", field.Char)),
		field.New("complete_name", field.Char),
		field.New("active", field.Bool),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("parent_id", field.Many2One).WithRelation("hr.department"),
		field.New("child_ids", field.One2Many).WithRelation("hr.department").WithRelationField("parent_id"),
		field.New("manager_id", field.Many2One).WithRelation("hr.employee"),
		field.New("member_ids", field.One2Many).WithRelation("hr.employee").WithRelationField("department_id"),
		field.New("has_read_access", field.Bool),
		field.New("total_employee", field.Int),
		field.New("jobs_ids", field.One2Many).WithRelation("hr.job").WithRelationField("department_id"),
		field.New("plan_ids", field.One2Many).WithRelation("mail.activity.plan").WithRelationField("department_id"),
		field.New("plans_count", field.Int),
		field.New("note", field.Text),
		field.New("color", field.Int),
		field.New("parent_path", field.Char),
		field.New("master_department_id", field.Many2One).WithRelation("hr.department"),
		field.New("c_level_manager_id", field.Many2One).WithRelation("hr.employee"),
	)
	m.RecName = "complete_name"
	m.Order = "name"
	return m
}

func employeeModel() model.Model {
	m := simple("hr.employee", "hr_employee",
		field.New("version_id", field.Many2One).WithRelation("hr.version"),
		field.New("current_version_id", field.Many2One).WithRelation("hr.version"),
		field.New("current_date_version", field.Date),
		field.New("version_ids", field.One2Many).WithRelation("hr.version").WithRelationField("employee_id"),
		field.New("versions_count", field.Int),
		required(field.New("name", field.Char)),
		field.New("resource_id", field.Many2One).WithRelation("resource.resource"),
		field.New("resource_calendar_id", field.Many2One).WithRelation("resource.calendar"),
		field.New("user_id", field.Many2One).WithRelation("res.users"),
		field.New("user_partner_id", field.Many2One).WithRelation("res.partner"),
		field.New("share", field.Bool),
		field.New("phone", field.Char),
		field.New("email", field.Char),
		field.New("active", field.Bool),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("work_phone", field.Char),
		field.New("mobile_phone", field.Char),
		field.New("work_email", field.Char),
		field.New("work_contact_id", field.Many2One).WithRelation("res.partner"),
		field.New("private_email", field.Char),
		field.New("department_id", field.Many2One).WithRelation("hr.department"),
		field.New("parent_id", field.Many2One).WithRelation("hr.employee"),
		field.New("child_ids", field.One2Many).WithRelation("hr.employee").WithRelationField("parent_id"),
		field.New("coach_id", field.Many2One).WithRelation("hr.employee"),
		field.New("job_id", field.Many2One).WithRelation("hr.job"),
		field.New("job_title", field.Char),
		field.New("category_ids", field.Many2Many).WithRelation("hr.employee.category"),
		field.New("tz", field.Selection),
		field.New("country_id", field.Many2One).WithRelation("res.country"),
		field.New("identification_id", field.Char),
		field.New("barcode", field.Char),
		field.New("pin", field.Char),
	)
	m.Order = "name"
	m.Inherits = map[string]string{"hr.version": "version_id"}
	return m
}

func jobModel() model.Model {
	m := simple("hr.job", "hr_job",
		field.New("active", field.Bool),
		required(field.New("name", field.Char)),
		field.New("sequence", field.Int),
		field.New("expected_employees", field.Int),
		field.New("no_of_employee", field.Int),
		field.New("no_of_recruitment", field.Int),
		field.New("employee_ids", field.One2Many).WithRelation("hr.employee").WithRelationField("job_id"),
		field.New("description", field.Text),
		field.New("requirements", field.Text),
		field.New("user_id", field.Many2One).WithRelation("res.users"),
		field.New("allowed_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("department_id", field.Many2One).WithRelation("hr.department"),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("contract_type_id", field.Many2One).WithRelation("hr.contract.type"),
	)
	m.Order = "sequence"
	return m
}

func employeeCategoryModel() model.Model {
	return simple("hr.employee.category", "hr_employee_category",
		required(field.New("name", field.Char)),
		field.New("color", field.Int),
		field.New("employee_ids", field.Many2Many).WithRelation("hr.employee"),
	)
}

func simple(name string, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}

func extension(name string, table string, fields ...field.Field) model.Model {
	m := simple(name, table, fields...)
	m.Inherit = []string{name}
	return m
}

func required(f field.Field) field.Field {
	f.Required = true
	return f
}
