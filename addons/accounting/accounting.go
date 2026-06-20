package accounting

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	coreaccounting "gorp/internal/accounting"
	"gorp/internal/data"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
)

const (
	ModuleName          = "account"
	LegacyModuleName    = "accounting"
	GenericChartFixture = "data/generic_chart.xml"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "Invoicing",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Accounting/Accounting",
		Depends:       []string{"base_setup", "onboarding", "product", "analytic", "portal", "digest"},
		Data:          accountDataFiles(),
		Installable:   true,
		Application:   true,
	}
}

func DependencyManifests() []module.Manifest {
	return []module.Manifest{
		{
			Name:          "Base Setup",
			TechnicalName: "base_setup",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Units of Measure",
			TechnicalName: "uom",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Onboarding",
			TechnicalName: "onboarding",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Product",
			TechnicalName: "product",
			Version:       "19.0.1.0.0",
			Category:      "Sales",
			Depends:       []string{"base", "uom"},
			Installable:   true,
		},
		{
			Name:          "Analytic Accounting",
			TechnicalName: "analytic",
			Version:       "19.0.1.0.0",
			Category:      "Accounting",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Portal",
			TechnicalName: "portal",
			Version:       "19.0.1.0.0",
			Category:      "Hidden",
			Depends:       []string{"base"},
			Installable:   true,
		},
		{
			Name:          "Digest",
			TechnicalName: "digest",
			Version:       "19.0.1.0.0",
			Category:      "Productivity",
			Depends:       []string{"base"},
			Installable:   true,
		},
	}
}

func accountDataFiles() []string {
	return []string{
		"security/account_security.xml",
		"security/ir.model.access.csv",
		"data/account_data.xml",
		"data/account_incoterms_data.xml",
		"data/product_data.xml",
		"data/analytic_data.xml",
		"data/digest_data.xml",
		"views/account_report.xml",
		"views/digest_views.xml",
		"views/portal_templates.xml",
		"data/mail_template_data.xml",
		"data/onboarding_data.xml",
		"data/account_tour.xml",
		"data/ir_sequence.xml",
		"views/product_views.xml",
		"views/analytic_views.xml",
		"views/res_config_settings_views.xml",
		"views/account_move_views.xml",
		"views/account_move_reversal_views.xml",
		"views/account_payment_register_views.xml",
		"views/account_move_send_views.xml",
		"views/account_account_views.xml",
		"views/account_group_views.xml",
		"views/account_incoterms_view.xml",
		"views/account_lock_exception_views.xml",
		"views/account_fiscal_position_views.xml",
		"views/account_invoice_report_view.xml",
		"views/account_wizard_views.xml",
		"views/account_journal_views.xml",
		"views/account_tax_views.xml",
		"views/account_payment_view.xml",
		"views/account_menuitem.xml",
		"data/account_reports_data.xml",
		GenericChartFixture,
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

func LoadGenericChart(env *record.Env) (map[string]data.ExternalID, error) {
	path, err := GenericChartPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	loader := data.NewLoader(env, ModuleName)
	if err := loader.LoadXML(file); err != nil {
		return nil, err
	}
	return loader.ExternalIDs(), nil
}

func GenericChartPath() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot resolve accounting source path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(source), GenericChartFixture))
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func Models() []model.Model {
	return coreaccounting.Models()
}
