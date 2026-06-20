package oi_delegation

import (
	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

const (
	ModuleName = "oi_delegation"

	ModelDelegation           = "delegation"
	ModelDelegationLine       = "delegation.line"
	ModelMailTemplateMetadata = "delegation.mail.template.metadata"
	ModelCacheEvent           = "delegation.cache.event"
	ModelWorkflowHook         = "delegation.workflow.hook"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "OI Delegation",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Administration",
		Depends:       []string{"base", "hr", "mail", "oi_base", "oi_workflow"},
		Data: []string{
			"security/group.xml",
			"security/ir.model.access.csv",
			"security/rules.xml",
			"data/sequences.xml",
			"data/ir_cron.xml",
			"data/mail_template.xml",
			"data/approval_config.xml",
			"data/approval_buttons.xml",
			"view/delegation.xml",
			"view/res_groups.xml",
			"view/mail_template.xml",
			"view/approval_log.xml",
			"view/action.xml",
			"view/menu.xml",
		},
		Installable:   true,
		Application:   true,
		SourceVersion: "18.0.1.0.14",
		SourceLicense: "OPL-1 reference; Go feature-parity implementation contains no copied source or assets",
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{Name: "HR", TechnicalName: "hr", Version: "19.0.1.0.0", Category: "Human Resources", Depends: []string{"base"}, Installable: true},
		{Name: "Mail", TechnicalName: "mail", Version: "19.0.1.0.0", Category: "Productivity", Depends: []string{"base"}, Installable: true},
		{Name: "OI Base", TechnicalName: "oi_base", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
		{Name: "OI Workflow", TechnicalName: "oi_workflow", Version: "19.0.1.0.0", Category: "Workflow", Depends: []string{"base", "mail", "oi_base"}, Installable: true},
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
		delegationModel(),
		delegationLineModel(),
		simple(ModelMailTemplateMetadata, "delegation_mail_template_metadata",
			field.New("xml_id", field.Char),
			field.New("name", field.Char),
			field.New("subject", field.Char),
			field.New("purpose", field.Text),
			field.New("delegation_group_ids", field.Many2Many).WithRelation("res.groups"),
			field.New("active", field.Bool),
		),
		simple(ModelCacheEvent, "delegation_cache_event",
			field.New("user_ids", field.Text),
			field.New("reason", field.Char),
			field.New("created_at", field.DateTime),
		),
		simple(ModelWorkflowHook, "delegation_workflow_hook",
			field.New("delegation_id", field.Many2One).WithRelation(ModelDelegation),
			field.New("event", field.Selection),
			field.New("state", field.Selection),
			field.New("payload", field.Text),
		),
	}
}

func ExtensionModels() []model.Model {
	return []model.Model{
		extension("res.groups", "res_groups",
			field.New("allow_delegation", field.Bool),
			field.New("delegation_template_ids", field.Many2Many).WithRelation("mail.template"),
			field.New("name_delegation", field.Char),
			field.New("allow_multiple_delegation", field.Bool),
			field.New("restricted_access", field.Bool),
		),
		extension("res.users", "res_users",
			field.New("delegation_ids", field.Many2Many).WithRelation(ModelDelegationLine),
			field.New("has_delegation", field.Bool),
			field.New("active_groups_ids", field.Many2Many).WithRelation("res.groups"),
			field.New("delegated_user_ids", field.Many2Many).WithRelation("res.users"),
			field.New("delegator_user_ids", field.Many2Many).WithRelation("res.users"),
			field.New("active_groups_count", field.Int),
			field.New("groups_count", field.Int),
		),
		extension("mail.template", "mail_template",
			field.New("delegation_group_ids", field.Many2Many).WithRelation("res.groups"),
		),
		extension("hr.employee", "hr_employee",
			field.New("delegated_employee_ids", field.Many2Many).WithRelation("hr.employee"),
			field.New("delegator_employee_ids", field.Many2Many).WithRelation("hr.employee"),
		),
		extension("approval.log", "approval_log",
			field.New("delegation_id", field.Many2One).WithRelation(ModelDelegation),
			field.New("delegation_employee_id", field.Many2One).WithRelation("hr.employee"),
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

func NewService() *delegation.Service {
	svc := delegation.NewService()
	ConfigureService(svc)
	return svc
}

func ConfigureService(svc *delegation.Service) {
	for _, config := range DefaultDelegableGroups() {
		svc.SetGroupConfig(config)
	}
}

func ConfigureServiceFromEnv(svc *delegation.Service, env *record.Env) error {
	if svc == nil || env == nil {
		return nil
	}
	found, err := env.Model("res.groups").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("name", "category_id", "allow_delegation", "delegation_template_ids", "name_delegation", "allow_multiple_delegation", "restricted_access")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !boolFromAny(row["allow_delegation"]) {
			continue
		}
		name := stringFromAny(row["name"])
		displayName := stringFromAny(row["name_delegation"])
		if displayName == "" {
			displayName = name
		}
		svc.SetGroupConfig(delegation.GroupConfig{
			GroupID:                 int64FromAny(row["id"]),
			Name:                    name,
			DisplayName:             displayName,
			AllowDelegation:         true,
			AllowMultipleDelegation: boolFromAny(row["allow_multiple_delegation"]),
			RestrictedAccess:        boolFromAny(row["restricted_access"]),
			TemplateIDs:             int64SliceFromAny(row["delegation_template_ids"]),
		})
	}
	return nil
}

func DefaultDelegableGroups() []delegation.GroupConfig {
	return []delegation.GroupConfig{
		{GroupID: GroupDelegableRole, Name: "Delegable Role", DisplayName: "Delegable Role", AllowDelegation: true, TemplateIDs: []int64{1, 2, 3}},
		{GroupID: GroupDelegableMultipleRole, Name: "Delegable Multiple Role", DisplayName: "Delegable Multiple Role", AllowDelegation: true, AllowMultipleDelegation: true, TemplateIDs: []int64{1, 2, 3}},
	}
}

func MailTemplates() []delegation.MailTemplateMetadata {
	return delegation.DefaultMailTemplates()
}

func delegationModel() model.Model {
	m := simple(ModelDelegation, "delegation",
		field.New("name", field.Char),
		required(field.New("date_from", field.Date)),
		required(field.New("date_to", field.Date)),
		required(field.New("employee_id", field.Many2One).WithRelation("hr.employee")),
		readonly(related(field.New("user_id", field.Many2One).WithRelation("res.users"), "employee_id.user_id")),
		field.New("one_employee", field.Bool),
		field.New("delegateTo_employee_id", field.Many2One).WithRelation("hr.employee"),
		field.New("delegate_to_employee_id", field.Many2One).WithRelation("hr.employee"),
		readonly(related(field.New("delegate_to_user_id", field.Many2One).WithRelation("res.users"), "delegate_to_employee_id.user_id")),
		field.New("isactive", field.Bool),
		field.New("state", field.Selection),
		field.New("lines", field.One2Many).WithRelation(ModelDelegationLine).WithRelationField("delegation_id"),
		field.New("department_ids", field.Many2Many).WithRelation("hr.department"),
		field.New("source_model", field.Char),
		field.New("source_record_id", field.Int),
		field.New("metadata", field.Text),
	)
	m.Inherit = []string{"approval.record", "name.sequence.mixin", "mail.thread", "mail.activity.mixin"}
	return m
}

func delegationLineModel() model.Model {
	return simple(ModelDelegationLine, "delegation_line",
		required(field.New("delegation_id", field.Many2One).WithRelation(ModelDelegation)),
		required(field.New("group_id", field.Many2One).WithRelation("res.groups")),
		field.New("employee_id", field.Many2One).WithRelation("hr.employee"),
		readonly(indexed(related(field.New("user_id", field.Many2One).WithRelation("res.users"), "employee_id.user_id"))),
		readonly(related(field.New("one_employee", field.Bool), "delegation_id.one_employee")),
		readonly(related(field.New("delegator_id", field.Many2One).WithRelation("hr.employee"), "delegation_id.employee_id")),
		readonly(indexed(related(field.New("delegator_user_id", field.Many2One).WithRelation("res.users"), "delegation_id.user_id"))),
		readonly(related(field.New("state", field.Selection), "delegation_id.state")),
		readonly(related(field.New("date_from", field.Date), "delegation_id.date_from")),
		readonly(related(field.New("date_to", field.Date), "delegation_id.date_to")),
		field.New("active", field.Bool),
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

func readonly(f field.Field) field.Field {
	f.Readonly = true
	return f
}

func indexed(f field.Field) field.Field {
	f.Index = true
	return f
}

func related(f field.Field, path string) field.Field {
	f.RelationField = path
	f.Store = true
	return f
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed == "true" || typed == "True" || typed == "1"
	default:
		return false
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func int64SliceFromAny(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64{}, typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id := int64FromAny(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	default:
		return nil
	}
}

func stringFromAny(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}
