package oi_login_as

import (
	"gorp/internal/field"
	"gorp/internal/impersonation"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

const (
	ModuleName = "oi_login_as"

	ModelLoginAsWizard = "login.as"
	ModelLoginAsAudit  = "login.as.audit"
	ModelLoginAsRoute  = "login.as.route"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "OI Login As",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Administration",
		Depends:       []string{"web", "portal"},
		Data: []string{
			"security/ir.model.access.csv",
			"view/action.xml",
			"view/login_as.xml",
			"view/templates.xml",
		},
		Assets: map[string][]string{
			"web.assets_backend": {
				"static/src/login_as/login_as.js",
				"static/src/login_as/login_as.xml",
				"static/src/login_as/debug_menu_items.js",
			},
		},
		Installable:   true,
		AutoInstall:   true,
		Application:   false,
		SourceVersion: "18.0.1.2.7",
		SourceLicense: "OPL-1 reference; Go feature-parity implementation contains no copied source or assets",
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{Name: "Web", TechnicalName: "web", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base"}, Installable: true},
		{Name: "Portal", TechnicalName: "portal", Version: "19.0.1.0.0", Category: "Hidden", Depends: []string{"base", "web"}, Installable: true},
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
		loginAsWizardModel(),
		simple(ModelLoginAsAudit, "login_as_audit",
			field.New("action", field.Char),
			field.New("actor_id", field.Many2One).WithRelation("res.users"),
			field.New("effective_user_id", field.Many2One).WithRelation("res.users"),
			field.New("target_user_id", field.Many2One).WithRelation("res.users"),
			field.New("session_id", field.Char),
			field.New("model", field.Char),
			field.New("record_id", field.Int),
			field.New("ip_address", field.Char),
			field.New("user_agent", field.Char),
			field.New("details", field.Text),
			field.New("created_at", field.DateTime),
		),
		simple(ModelLoginAsRoute, "login_as_route",
			field.New("name", field.Char),
			field.New("path", field.Char),
			field.New("method", field.Char),
			field.New("auth", field.Char),
			field.New("enabled", field.Bool),
			field.New("requires_setting", field.Char),
		),
	}
}

func ExtensionModels() []model.Model {
	return []model.Model{
		extension("res.users", "res_users",
			field.New("allow_login_as", field.Bool),
			field.New("login_as_group_ids", field.Many2Many).WithRelation("res.groups"),
		),
		extension("ir.http", "ir_http",
			field.New("login_as_user_id", field.Many2One).WithRelation("res.users"),
			field.New("login_as_original_uid", field.Many2One).WithRelation("res.users"),
			field.New("login_as_banner", field.Char),
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

func DefaultConfig() impersonation.Config {
	config := impersonation.DefaultConfig()
	config.AdminGroupID = GroupLoginAsAdmin
	config.ImpersonatorGroupID = GroupLoginAsUser
	config.AllowInactiveGroupID = GroupLoginAsAllowInactive
	config.AllowSuperuserGroupID = GroupLoginAsAllowSuperuser
	config.DebugGroupID = GroupLoginAsDebug
	config.PortalSupport = true
	config.SystemUserID = 1
	return config
}

func NewService() *impersonation.Service {
	return impersonation.NewService(impersonation.WithConfig(DefaultConfig()))
}

func loginAsWizardModel() model.Model {
	m := simple(ModelLoginAsWizard, "login_as",
		field.New("group_id", field.Many2One).WithRelation("res.groups"),
		field.New("user_id", field.Many2One).WithRelation("res.users"),
		field.New("group_ids", field.Many2Many).WithRelation("res.groups"),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("company_ids", field.Many2Many).WithRelation("res.company"),
		field.New("return_to", field.Char),
		field.New("allow_inactive", field.Bool),
		field.New("allow_superuser", field.Bool),
	)
	m.Transient = true
	return m
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
