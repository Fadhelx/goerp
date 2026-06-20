package accounting_enterprise_hooks

import "gorp/internal/module"

const (
	TechnicalName        = "accounting_enterprise_hooks"
	AccountingDependency = "account"
)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "Accounting Enterprise Hooks",
		TechnicalName: TechnicalName,
		Version:       "19.0.1.0.0",
		Category:      "Accounting",
		Depends:       []string{AccountingDependency},
		Installable:   true,
		AutoInstall:   false,
		Application:   false,
		SourceLicense: "clean-room extension points only",
	}
}
