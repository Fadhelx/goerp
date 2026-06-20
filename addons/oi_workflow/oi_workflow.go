package oi_workflow

import (
	"gorp/addons/oi_base"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/workflow"
)

const ModuleName = "oi_workflow"

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "Workflow Engine Base",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Hidden/Extra Tools",
		Depends:       []string{"mail", oi_base.ModuleName, "oi_base_cache", "web", "base_automation", "oi_web_selection_field_dynamic", "oi_web_selection_tags", "automation"},
		Data: []string{
			"security/oi_workflow_groups.xml",
			"security/ir.model.access.csv",
			"data/ir_sequence.xml",
			"data/approval_record_templates.xml",
			"data/mail_activity_type.xml",
			"data/ir_cron.xml",
			"data/approval_settings.xml",
			"view/approval_settings.xml",
			"view/approval_buttons.xml",
			"view/approval_log.xml",
			"view/cancellation_record_view.xml",
			"view/approval_config.xml",
			"view/approval_automation.xml",
			"view/approval_escalation.xml",
			"view/approval_process_wizard.xml",
			"view/approval_state_update.xml",
			"view/ir_model.xml",
			"view/model_expression_editor.xml",
			"view/res_config_settings.xml",
			"view/res_groups.xml",
			"view/action.xml",
			"view/menu.xml",
			"view/templates.xml",
			"views/approval_record_templates.xml",
		},
		Assets: map[string][]string{
			"web.assets_backend": {
				"frontend/packages/oi-workflow/src/index.ts",
				"static/src/js/chatter.js",
				"static/src/js/debug_items.js",
				"static/src/js/form_controller.js",
				"static/src/js/list_controller.js",
				"static/src/js/statusbar_duration_state_field.js",
				"static/src/js/statusbar_field.js",
				"static/src/js/utils.js",
				"static/src/js/view_button.js",
				"static/src/xml/approval_user_info.xml",
				"static/src/xml/view_button.xml",
				"static/src/scss/statusbar_duration_field.scss",
			},
		},
		Installable:   true,
		AutoInstall:   true,
		Application:   false,
		SourceVersion: "18.0.1.8.7.cleanroom",
		SourceLicense: "clean-room feature parity; source manifests were inspected, code/assets not copied",
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{
			Name:          "Mail",
			TechnicalName: "mail",
			Version:       "19.0.1.0.0",
			Category:      "Productivity",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Automation Compatibility",
			TechnicalName: "automation",
			Version:       "19.0.1.0.0",
			Category:      "Automation",
			Depends:       []string{"base", "mail"},
			Installable:   true,
		},
		{
			Name:          "Base Automation",
			TechnicalName: "base_automation",
			Version:       "19.0.1.0.0",
			Category:      "Automation",
			Depends:       []string{"base", "mail"},
			Installable:   true,
		},
		{
			Name:          "Web",
			TechnicalName: "web",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "OI Base Cache",
			TechnicalName: "oi_base_cache",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base", oi_base.ModuleName},
			Installable:   true,
		},
		{
			Name:          "OI Web Selection Field Dynamic",
			TechnicalName: "oi_web_selection_field_dynamic",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base", "web"},
			Installable:   true,
		},
		{
			Name:          "OI Web Selection Tags",
			TechnicalName: "oi_web_selection_tags",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base", "web"},
			Installable:   true,
		},
		oi_base.Manifest(),
	}
}

func RegisterModels(reg *registry.Registry) error {
	return workflow.RegisterModels(reg)
}

func RegisterRecordModels(reg *record.Registry) error {
	for _, m := range workflow.Models() {
		if err := reg.Register(m); err != nil {
			return err
		}
	}
	return nil
}

func ModelNames() []string {
	return workflow.ModelNames()
}
