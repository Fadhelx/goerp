package runtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	accounting "gorp/addons/accounting"
	aiaddon "gorp/addons/ai"
	"gorp/addons/hr"
	"gorp/addons/oi_base"
	"gorp/addons/oi_delegation"
	"gorp/addons/oi_login_as"
	"gorp/addons/oi_workflow"
	"gorp/addons/oi_workflow_advance"
	serveractions "gorp/internal/actions"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/assets"
	"gorp/internal/base"
	"gorp/internal/data"
	internaldelegation "gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/field"
	web "gorp/internal/http"
	"gorp/internal/impersonation"
	internalmail "gorp/internal/mail"
	"gorp/internal/meta/action"
	"gorp/internal/meta/menu"
	"gorp/internal/meta/view"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/notifications"
	"gorp/internal/phone"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
	"gorp/internal/sequences"
	internalworkflow "gorp/internal/workflow"
)

type App struct {
	Env                 *record.Env
	Assets              *assets.Registry
	Actions             *action.Registry
	ServerActions       *serveractions.Registry
	Menus               *menu.Registry
	Views               *view.Registry
	Modules             map[string]module.Manifest
	ModuleRegistry      *registry.Registry
	Security            *security.Engine
	Delegation          *internaldelegation.Service
	Impersonation       *impersonation.Service
	Bus                 *notifications.Bus
	ExternalIDs         map[string]data.ExternalID
	AIProvider          aiproviders.Provider
	AIProviders         *aiproviders.Registry
	AIEmbeddingModel    string
	AIBaseURL           string
	MailSender          internalmail.Sender
	FetchmailConnector  internalmail.FetchmailConnector
	InboundMessageLock  internalmail.InboundMessageIDLocker
	FetchmailServerLock internalmail.FetchmailServerLocker
}

func BootstrapOI(root string) (*App, error) {
	if strings.TrimSpace(root) == "" {
		root = repoRoot()
	}
	moduleReg := registry.New("gorp")
	manifests := oiManifests()
	if err := moduleReg.Install(manifests); err != nil {
		return nil, err
	}
	if err := registerModuleModels(moduleReg); err != nil {
		return nil, err
	}
	env, err := oiEnv()
	if err != nil {
		return nil, err
	}
	externalIDs := map[string]data.ExternalID{}
	if err := loadBootstrapMetadata(env, externalIDs); err != nil {
		return nil, err
	}
	if err := loadBootstrapData(root, env, externalIDs); err != nil {
		return nil, err
	}
	if err := createInstalledModuleRows(env, manifests); err != nil {
		return nil, err
	}
	assetReg, err := assetsFromSources(env, manifests)
	if err != nil {
		return nil, err
	}
	actionReg, err := actionsFromEnv(env, externalIDs)
	if err != nil {
		return nil, err
	}
	serverActionReg, err := serverActionsFromEnv(env, externalIDs)
	if err != nil {
		return nil, err
	}
	viewReg, err := viewsFromEnv(env)
	if err != nil {
		return nil, err
	}
	menuReg, err := menusFromEnv(env, externalIDs)
	if err != nil {
		return nil, err
	}
	securityEngine := security.NewEngine()
	if err := securityEngine.LoadPersistedSecurity(env); err != nil {
		return nil, err
	}
	securityEngine.SetDepartmentResolvers(recordDepartmentResolver(env), departmentDescendantResolver(env))
	aiaddon.ApplySecurity(securityEngine)
	delegationService := oi_delegation.NewRuntimeService(securityEngine, nil, nil, oi_delegation.SecurityMailRecipientResolver(securityEngine), nil)
	if err := oi_delegation.ConfigureServiceFromEnv(delegationService, env); err != nil {
		return nil, err
	}
	oi_delegation.BindSecurity(securityEngine, delegationService)
	env.WithPolicy(securityEngine)
	impersonationService := impersonationFromExternalIDs(externalIDs)
	bus := notifications.NewBus(100)
	modules := map[string]module.Manifest{}
	for _, manifest := range manifests {
		modules[manifest.TechnicalName] = manifest
	}
	app := &App{
		Env:            env,
		Assets:         assetReg,
		Actions:        actionReg,
		ServerActions:  serverActionReg,
		Menus:          menuReg,
		Views:          viewReg,
		Modules:        modules,
		ModuleRegistry: moduleReg,
		Security:       securityEngine,
		Delegation:     delegationService,
		Impersonation:  impersonationService,
		Bus:            bus,
		ExternalIDs:    externalIDs,
		AIProvider:     aiproviders.NewMockProvider(),
	}
	actionHooks := envActionHooks{env: env, app: app}
	serverActionReg.SetHooks(serveractions.Hooks{
		Creator:           actionHooks,
		Writer:            actionHooks,
		Sequencer:         actionHooks,
		ObjectOperator:    actionHooks,
		Evaluator:         actionHooks,
		Mailer:            actionHooks,
		FollowerUpdater:   actionHooks,
		ActivityScheduler: actionHooks,
		SMSSender:         actionHooks,
		WhatsAppSender:    actionHooks,
		DocumentCreator:   actionHooks,
		AIRunner:          actionHooks,
	})
	if err := registerAIRuntimeToolActions(serverActionReg, env, bus); err != nil {
		return nil, err
	}
	if err := registerWorkflowRuntimeActions(serverActionReg, env, delegationService, actionHooks); err != nil {
		return nil, err
	}
	if err := registerMailRuntimeActions(serverActionReg, env, app); err != nil {
		return nil, err
	}
	workflowDispatcher := &internalworkflow.Dispatcher{Actions: serverActionReg, Mailer: actionHooks, Delegations: delegationService}
	env.RegisterAfterCreateHook(workflowDispatcher.AutoStartCreateHook())
	env.RegisterAfterWriteHook(workflowDispatcher.StateUpdatedWriteHook())
	env.RegisterBeforeUnlinkHook(workflowDispatcher.UnlinkHook())
	return app, nil
}

func (a *App) Server() web.Server {
	return web.Server{
		Env:                 a.Env,
		Assets:              a.Assets,
		Actions:             a.Actions,
		ServerActions:       a.ServerActions,
		Menus:               a.Menus,
		Views:               a.Views,
		Modules:             a.Modules,
		ExternalIDs:         a.ExternalIDs,
		Security:            a.Security,
		Impersonation:       a.Impersonation,
		Bus:                 a.Bus,
		Workflow:            &internalworkflow.Dispatcher{Actions: a.ServerActions, Mailer: envActionHooks{env: a.Env, app: a}, Delegations: a.Delegation},
		AIChatFactory:       a.aiChatFactory(),
		MailSender:          a.MailSender,
		FetchmailConnector:  a.FetchmailConnector,
		InboundMessageLock:  a.InboundMessageLock,
		FetchmailServerLock: a.FetchmailServerLock,
	}
}

func recordDepartmentResolver(env *record.Env) security.RecordDepartmentResolver {
	return func(modelName string, row map[string]any) ([]int64, bool) {
		if env == nil || row == nil {
			return nil, false
		}
		meta, ok := env.ModelMetadata(modelName)
		if !ok {
			return nil, false
		}
		for _, fieldName := range []string{"employee_id", "x_employee_id", "department_id", "x_department_id"} {
			fieldMeta, ok := meta.Fields[fieldName]
			if !ok || !fieldMeta.Store || fieldMeta.Kind != field.Many2One {
				continue
			}
			switch fieldMeta.Relation {
			case "hr.department":
				if departmentID := int64Value(row[fieldName]); departmentID != 0 {
					return []int64{departmentID}, true
				}
			case "hr.employee":
				if departmentID := employeeDepartmentID(env, int64Value(row[fieldName])); departmentID != 0 {
					return []int64{departmentID}, true
				}
			}
			return nil, true
		}
		return nil, false
	}
}

func departmentDescendantResolver(env *record.Env) security.DepartmentDescendantResolver {
	return func(departmentIDs []int64) []int64 {
		return departmentIDsWithDescendants(env, departmentIDs)
	}
}

func employeeDepartmentID(env *record.Env, employeeID int64) int64 {
	if env == nil || employeeID == 0 {
		return 0
	}
	rows, err := env.Model("hr.employee").Browse(employeeID).Read("department_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["department_id"])
}

func departmentIDsWithDescendants(env *record.Env, departmentIDs []int64) []int64 {
	departmentIDs = uniqueInt64s(departmentIDs)
	if env == nil || len(departmentIDs) == 0 {
		return departmentIDs
	}
	meta, ok := env.ModelMetadata("hr.department")
	if !ok {
		return departmentIDs
	}
	parentField, ok := meta.Fields["parent_id"]
	if !ok || parentField.Relation != "hr.department" {
		return departmentIDs
	}
	found, err := env.Model("hr.department").Search(domain.And())
	if err != nil {
		return departmentIDs
	}
	rows, err := found.Read("parent_id")
	if err != nil {
		return departmentIDs
	}
	allowed := map[int64]bool{}
	for _, id := range departmentIDs {
		if id != 0 {
			allowed[id] = true
		}
	}
	changed := true
	for changed {
		changed = false
		for _, row := range rows {
			id := int64Value(row["id"])
			parentID := int64Value(row["parent_id"])
			if id == 0 || parentID == 0 || !allowed[parentID] || allowed[id] {
				continue
			}
			allowed[id] = true
			changed = true
		}
	}
	out := make([]int64, 0, len(allowed))
	for id := range allowed {
		out = append(out, id)
	}
	return uniqueInt64s(out)
}

func uniqueInt64s(values []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func oiManifests() []module.Manifest {
	dependencies := accounting.DependencyManifests()
	accountingManifests := []module.Manifest{}
	if runtimeAccountingEnabled() {
		accountingManifests = append(accountingManifests, accounting.Manifest())
	}
	return uniqueManifests(
		[]module.Manifest{base.Manifest()},
		oi_workflow.DependencyManifests(),
		oi_workflow_advance.DependencyManifests(),
		oi_delegation.DependencyManifests(),
		oi_login_as.DependencyManifests(),
		aiaddon.DependencyManifests(),
		dependencies,
		hr.DependencyManifests(),
		accountingManifests,
		[]module.Manifest{
			hr.Manifest(),
			aiaddon.Manifest(),
			oi_base.Manifest(),
			oi_workflow.Manifest(),
			oi_workflow_advance.Manifest(),
			oi_delegation.Manifest(),
			oi_login_as.Manifest(),
		},
	)
}

func runtimeAccountingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GORP_ENABLE_ACCOUNTING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func uniqueManifests(groups ...[]module.Manifest) []module.Manifest {
	byName := map[string]module.Manifest{}
	for _, group := range groups {
		for _, manifest := range group {
			byName[manifest.TechnicalName] = manifest
		}
	}
	order := []string{"base", "mail", aiaddon.ModuleName, "automation", "base_automation", "web", "portal", "base_setup", "onboarding", "uom", "product", "analytic", "digest", "phone_validation", "resource", "resource_mail", "hr", accounting.ModuleName, oi_base.ModuleName, "oi_base_cache", "oi_web_selection_field_dynamic", "oi_web_selection_tags", "oi_web_flowchart", oi_workflow.ModuleName, oi_workflow_advance.ModuleName, oi_delegation.ModuleName, oi_login_as.ModuleName}
	out := make([]module.Manifest, 0, len(byName))
	for _, name := range order {
		if manifest, ok := byName[name]; ok {
			out = append(out, manifest)
			delete(byName, name)
		}
	}
	var rest []string
	for name := range byName {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	for _, name := range rest {
		out = append(out, byName[name])
	}
	return out
}

func registerModuleModels(reg *registry.Registry) error {
	for _, fn := range []func(*registry.Registry) error{
		base.RegisterModels,
		aiaddon.RegisterModels,
		hr.RegisterModels,
		oi_base.RegisterModels,
		oi_workflow.RegisterModels,
		oi_workflow_advance.RegisterModels,
		oi_delegation.RegisterModels,
		oi_login_as.RegisterModels,
	} {
		if err := fn(reg); err != nil {
			return err
		}
	}
	return registerModuleModelsIfAbsent(reg, accounting.Models())
}

func registerModuleModelsIfAbsent(reg *registry.Registry, models []model.Model) error {
	for _, m := range models {
		if _, exists := reg.Models[m.Name]; exists {
			continue
		}
		if err := reg.RegisterModel(m); err != nil {
			return err
		}
	}
	return nil
}

func loadBootstrapMetadata(env *record.Env, externalIDs map[string]data.ExternalID) error {
	loads := []struct {
		moduleName string
		models     []model.Model
	}{
		{base.Manifest().TechnicalName, base.Models()},
		{aiaddon.ModuleName, aiaddon.Models()},
		{accounting.ModuleName, accounting.Models()},
		{hr.ModuleName, hr.Models()},
		{oi_base.ModuleName, oi_base.Models()},
		{oi_workflow.ModuleName, internalworkflow.Models()},
		{oi_workflow_advance.ModuleName, internalworkflow.AdvancedModels()},
		{oi_delegation.ModuleName, oi_delegation.Models()},
		{oi_login_as.ModuleName, oi_login_as.Models()},
	}
	for _, load := range loads {
		if err := data.LoadModelMetadata(env, load.moduleName, load.models, externalIDs); err != nil {
			return err
		}
	}
	return nil
}

func oiEnv() (*record.Env, error) {
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
	for _, group := range [][]model.Model{
		base.Models(),
		aiaddon.Models(),
		accounting.Models(),
		hr.Models(),
		hr.ExtensionModels(),
		oi_base.Models(),
		oi_base.ExtensionModels(),
		internalworkflow.Models(),
		internalworkflow.AdvancedModels(),
		internalworkflow.AdvancedExtensionModels(),
		oi_delegation.Models(),
		oi_delegation.ExtensionModels(),
		oi_login_as.Models(),
		oi_login_as.ExtensionModels(),
	} {
		for _, m := range group {
			add(m)
		}
	}
	reg := record.NewRegistry()
	for _, name := range order {
		if err := reg.Register(models[name]); err != nil {
			return nil, err
		}
	}
	return record.NewEnv(reg, record.Context{
		UserID:     1,
		CompanyID:  1,
		CompanyIDs: []int64{1},
		Values: map[string]any{
			"db":        "gorp",
			"lang":      "en_US",
			"tz":        "UTC",
			"edition":   "e",
			"group_ids": []int64{1},
		},
	}), nil
}

func loadBootstrapData(root string, env *record.Env, externalIDs map[string]data.ExternalID) error {
	loads := []struct {
		moduleName string
		baseDir    string
		paths      []string
	}{
		{base.Manifest().TechnicalName, filepath.Join(root, "internal/base"), base.Manifest().Data},
		{aiaddon.ModuleName, filepath.Join(root, "addons/ai"), aiaddon.Manifest().Data},
	}
	if runtimeAccountingEnabled() {
		loads = append(loads, struct {
			moduleName string
			baseDir    string
			paths      []string
		}{accounting.ModuleName, filepath.Join(root, "addons/accounting"), accounting.Manifest().Data})
	}
	loads = append(loads,
		struct {
			moduleName string
			baseDir    string
			paths      []string
		}{oi_base.ModuleName, filepath.Join(root, "addons/oi_base"), oi_base.Manifest().Data},
		struct {
			moduleName string
			baseDir    string
			paths      []string
		}{oi_workflow.ModuleName, filepath.Join(root, "addons/oi_workflow"), oi_workflow.Manifest().Data},
		struct {
			moduleName string
			baseDir    string
			paths      []string
		}{oi_delegation.ModuleName, filepath.Join(root, "addons/oi_delegation"), oi_delegation.Manifest().Data},
		struct {
			moduleName string
			baseDir    string
			paths      []string
		}{oi_workflow_advance.ModuleName, filepath.Join(root, "addons/oi_workflow_advance"), oi_workflow_advance.Manifest().Data},
	)
	for _, load := range loads {
		if err := loadManifestData(env, externalIDs, load.moduleName, load.baseDir, load.paths); err != nil {
			return err
		}
	}
	loginLoader := data.NewLoaderWithExternalIDs(env, oi_login_as.ModuleName, externalIDs)
	if err := seedLoginAsExternalIDs(loginLoader); err != nil {
		return err
	}
	return loadManifestData(env, externalIDs, oi_login_as.ModuleName, filepath.Join(root, "addons/oi_login_as"), oi_login_as.Manifest().Data)
}

func loadManifestData(env *record.Env, externalIDs map[string]data.ExternalID, moduleName string, baseDir string, paths []string) error {
	loader := data.NewLoaderWithExternalIDs(env, moduleName, externalIDs)
	loader.SetBaseDir(baseDir)
	for _, rel := range paths {
		path := filepath.Join(baseDir, rel)
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		switch filepath.Ext(path) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			err = loader.LoadCSV(csvModelNameFromPath(path), file)
		default:
			err = fmt.Errorf("unsupported fixture file %s", path)
		}
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", path, closeErr)
		}
	}
	return nil
}

func csvModelNameFromPath(path string) string {
	modelName := strings.TrimSuffix(filepath.Base(path), ".csv")
	if strings.Contains(filepath.ToSlash(path), "/data/template/") {
		if prefix, _, ok := strings.Cut(modelName, "-"); ok {
			return prefix
		}
	}
	return modelName
}

func seedLoginAsExternalIDs(loader *data.Loader) error {
	var seed strings.Builder
	seed.WriteString("<odoo>")
	for _, group := range oi_login_as.SecurityGroups() {
		seed.WriteString(`<record id="` + loginAsGroupExternalID(group.ID) + `" model="res.groups">`)
		seed.WriteString(`<field name="name">` + group.Name + `</field>`)
		if len(group.ImpliedIDs) > 0 {
			seed.WriteString(`<field name="implied_ids" eval="[`)
			for i, implied := range group.ImpliedIDs {
				if i > 0 {
					seed.WriteByte(',')
				}
				seed.WriteString(`(4, ref('` + loginAsGroupExternalID(implied) + `'))`)
			}
			seed.WriteString(`]"/>`)
		}
		seed.WriteString(`</record>`)
	}
	for _, m := range oi_login_as.Models() {
		seed.WriteString(`<record id="` + modelExternalID(m.Name) + `" model="ir.model">`)
		seed.WriteString(`<field name="model">` + m.Name + `</field>`)
		seed.WriteString(`<field name="name">` + m.Name + `</field>`)
		seed.WriteString(`</record>`)
	}
	seed.WriteString("</odoo>")
	return loader.LoadXML(strings.NewReader(seed.String()))
}

func createInstalledModuleRows(env *record.Env, manifests []module.Manifest) error {
	existing, err := env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := existing.Read("name")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[stringValue(row["name"])] = true
	}
	for _, manifest := range manifests {
		if seen[manifest.TechnicalName] {
			continue
		}
		if _, err := env.Model("ir.module.module").Create(map[string]any{"name": manifest.TechnicalName, "state": "installed"}); err != nil {
			return err
		}
	}
	return nil
}

func assetsFromSources(env *record.Env, manifests []module.Manifest) (*assets.Registry, error) {
	reg := assets.NewRegistry()
	if err := applyAssetsFromEnv(env, reg, func(sequence int) bool { return sequence < 16 }); err != nil {
		return nil, err
	}
	assetManifests, err := installedAssetManifests(env, manifests)
	if err != nil {
		return nil, err
	}
	if err := applyManifestAssets(reg, assetManifests); err != nil {
		return nil, err
	}
	if err := applyAssetsFromEnv(env, reg, func(sequence int) bool { return sequence >= 16 }); err != nil {
		return nil, err
	}
	return reg, nil
}

func assetsFromManifests(manifests []module.Manifest) (*assets.Registry, error) {
	reg := assets.NewRegistry()
	if err := applyManifestAssets(reg, manifests); err != nil {
		return nil, err
	}
	return reg, nil
}

func applyManifestAssets(reg *assets.Registry, manifests []module.Manifest) error {
	ordered, err := module.SortByDependencies(manifests)
	if err != nil {
		return err
	}
	for _, manifest := range ordered {
		operations := manifest.AssetOperations
		if len(operations) == 0 {
			operations = map[string][]module.AssetOperation{}
			for bundle, paths := range manifest.Assets {
				for _, path := range paths {
					operations[bundle] = append(operations[bundle], module.AssetOperation{Directive: "append", Path: path})
				}
			}
		}
		for bundle, ops := range operations {
			for _, op := range ops {
				assetOp, err := assets.OperationFromDirective(op.Directive, op.Path, op.Target)
				if err != nil {
					return err
				}
				if err := reg.Apply(bundle, assetOp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func installedAssetManifests(env *record.Env, manifests []module.Manifest) ([]module.Manifest, error) {
	rows, err := allRows(env, "ir.module.module", "name", "state")
	if err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	for _, row := range rows {
		if stringValue(row["state"]) == "installed" {
			installed[stringValue(row["name"])] = true
		}
	}
	if len(installed) == 0 {
		return manifests, nil
	}
	out := make([]module.Manifest, 0, len(manifests))
	for _, manifest := range manifests {
		if installed[manifest.TechnicalName] {
			out = append(out, manifest)
		}
	}
	return out, nil
}

func applyAssetsFromEnv(env *record.Env, reg *assets.Registry, includeSequence func(int) bool) error {
	rows, err := allRows(env, "ir.asset", "name", "active", "bundle", "directive", "path", "target", "sequence")
	if err != nil {
		return err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := intWithFallback(rows[i]["sequence"], 16)
		right := intWithFallback(rows[j]["sequence"], 16)
		if left != right {
			return left < right
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
	for _, row := range rows {
		if !boolWithFallback(row["active"], true) {
			continue
		}
		sequence := intWithFallback(row["sequence"], 16)
		if includeSequence != nil && !includeSequence(sequence) {
			continue
		}
		bundle := stringValue(row["bundle"])
		if bundle == "" {
			continue
		}
		op, err := assets.OperationFromDirective(stringValue(row["directive"]), stringValue(row["path"]), stringValue(row["target"]))
		if err != nil {
			return fmt.Errorf("ir.asset %d: %w", int64Value(row["id"]), err)
		}
		if err := reg.Apply(bundle, op); err != nil {
			return fmt.Errorf("ir.asset %d: %w", int64Value(row["id"]), err)
		}
	}
	return nil
}

func actionsFromEnv(env *record.Env, externalIDs map[string]data.ExternalID) (*action.Registry, error) {
	reg := action.NewRegistry()
	viewTypes, err := viewTypesByID(env)
	if err != nil {
		return nil, err
	}
	actionViews, err := actionViewsByAction(env)
	if err != nil {
		return nil, err
	}
	embeddedActions, err := embeddedActionsByParent(env)
	if err != nil {
		return nil, err
	}
	workflowViews, err := workflowCreateViewsByModel(env)
	if err != nil {
		return nil, err
	}
	rows, err := allRows(env, "ir.actions.act_window", "name", "type", "res_model", "res_id", "view_mode", "mobile_view_mode", "view_id", "search_view_id", "domain", "context", "target", "limit", "help", "path", "group_ids", "binding_model_id", "binding_type", "binding_view_types", "filter", "cache")
	if err != nil {
		return nil, err
	}
	actionXMLIDs := externalIDByRecord(externalIDs, "ir.actions.act_window")
	for _, row := range rows {
		id := int64Value(row["id"])
		viewID := int64Value(row["view_id"])
		viewMode := stringValue(row["view_mode"])
		actionContext := actionContextMap(row["context"])
		if viewMode == "" {
			viewMode = "list,form"
		}
		if err := reg.AddWithID(action.Action{
			ID:                id,
			XMLID:             actionXMLIDs[id],
			Name:              stringValue(row["name"]),
			Kind:              action.ActWindow,
			ResModel:          stringValue(row["res_model"]),
			ResID:             int64Value(row["res_id"]),
			ViewMode:          viewMode,
			MobileViewMode:    stringValue(row["mobile_view_mode"]),
			ViewID:            viewID,
			Views:             computeActionViews(viewMode, viewID, viewTypes[viewID], actionViews[id]),
			SearchViewID:      int64Value(row["search_view_id"]),
			Domain:            stringValue(row["domain"]),
			Context:           actionContext,
			Target:            firstNonEmptyString(stringValue(row["target"]), "current"),
			Limit:             intWithFallback(row["limit"], 80),
			Help:              stringValue(row["help"]),
			Path:              stringValue(row["path"]),
			Groups:            int64Slice(row["group_ids"]),
			BindingModelID:    int64Value(row["binding_model_id"]),
			BindingType:       stringValue(row["binding_type"]),
			BindingViewTypes:  stringValue(row["binding_view_types"]),
			Filter:            boolValue(row["filter"]),
			Cache:             boolWithFallback(row["cache"], true),
			EmbeddedActions:   embeddedActions[id],
			MultiWorkflowView: multiWorkflowViewForAction(stringValue(row["res_model"]), actionContext, workflowViews),
		}); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func workflowCreateViewsByModel(env *record.Env) (map[string]string, error) {
	rows, err := allRows(env, internalworkflow.ModelWorkflow, "name", "model", "active", "view_id", "create_context")
	if err != nil {
		if strings.Contains(err.Error(), "unknown model "+internalworkflow.ModelWorkflow) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	itemsByModel := map[string][]map[string]any{}
	for _, row := range rows {
		modelName := stringValue(row["model"])
		viewID := int64Value(row["view_id"])
		if row["active"] == false || modelName == "" || viewID == 0 {
			continue
		}
		createContext, err := internalworkflow.ParseContextLiteralStrict(stringValue(row["create_context"]))
		if err != nil {
			return nil, fmt.Errorf("workflow %d create_context: %w", int64Value(row["id"]), err)
		}
		itemsByModel[modelName] = append(itemsByModel[modelName], map[string]any{
			"id":             int64Value(row["id"]),
			"name":           stringValue(row["name"]),
			"view_id":        viewID,
			"create_context": createContext,
		})
	}
	out := map[string]string{}
	for modelName, items := range itemsByModel {
		data, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		out[modelName] = string(data)
	}
	return out, nil
}

func actionContextMap(value any) map[string]any {
	if parsed := jsonMap(value); parsed != nil {
		return parsed
	}
	return internalworkflow.ParseContextLiteral(stringValue(value))
}

func multiWorkflowViewForAction(modelName string, actionContext map[string]any, workflowViews map[string]string) string {
	if actionContext != nil {
		if value, ok := actionContext["multi_workflow_view"]; ok && !boolValue(value) {
			return ""
		}
	}
	return workflowViews[modelName]
}

func embeddedActionsByParent(env *record.Env) (map[int64][]action.EmbeddedAction, error) {
	rows, err := allRows(env, "ir.embedded.actions", "name", "sequence", "parent_action_id", "parent_res_id", "parent_res_model", "action_id", "python_method", "user_id", "is_deletable", "default_view_mode", "filter_ids", "is_visible", "domain", "context", "groups_ids")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		leftParent := int64Value(rows[i]["parent_action_id"])
		rightParent := int64Value(rows[j]["parent_action_id"])
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		leftSequence := intWithFallback(rows[i]["sequence"], 0)
		rightSequence := intWithFallback(rows[j]["sequence"], 0)
		if leftSequence != rightSequence {
			return leftSequence < rightSequence
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
	out := map[int64][]action.EmbeddedAction{}
	for _, row := range rows {
		parentID := int64Value(row["parent_action_id"])
		if parentID == 0 || !boolWithFallback(row["is_visible"], true) {
			continue
		}
		out[parentID] = append(out[parentID], action.EmbeddedAction{
			ID:              int64Value(row["id"]),
			Name:            stringValue(row["name"]),
			ParentActionID:  parentID,
			ParentResID:     int64Value(row["parent_res_id"]),
			ParentResModel:  stringValue(row["parent_res_model"]),
			ActionID:        int64Value(row["action_id"]),
			PythonMethod:    stringValue(row["python_method"]),
			UserID:          int64Value(row["user_id"]),
			IsDeletable:     boolValue(row["is_deletable"]),
			DefaultViewMode: stringValue(row["default_view_mode"]),
			FilterIDs:       int64Slice(row["filter_ids"]),
			Domain:          firstNonEmptyString(stringValue(row["domain"]), "[]"),
			Context:         firstNonEmptyString(stringValue(row["context"]), "{}"),
			GroupIDs:        int64Slice(row["groups_ids"]),
		})
	}
	return out, nil
}

func viewTypesByID(env *record.Env) (map[int64]string, error) {
	rows, err := allRows(env, "ir.ui.view", "type")
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for _, row := range rows {
		out[int64Value(row["id"])] = stringValue(row["type"])
	}
	return out, nil
}

func actionViewsByAction(env *record.Env) (map[int64][]action.ViewRef, error) {
	rows, err := allRows(env, "ir.actions.act_window.view", "sequence", "view_mode", "view_id", "act_window_id")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		leftAction := int64Value(rows[i]["act_window_id"])
		rightAction := int64Value(rows[j]["act_window_id"])
		if leftAction != rightAction {
			return leftAction < rightAction
		}
		leftSequence := intWithFallback(rows[i]["sequence"], 0)
		rightSequence := intWithFallback(rows[j]["sequence"], 0)
		if leftSequence != rightSequence {
			return leftSequence < rightSequence
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
	out := map[int64][]action.ViewRef{}
	for _, row := range rows {
		actionID := int64Value(row["act_window_id"])
		if actionID == 0 {
			continue
		}
		out[actionID] = append(out[actionID], action.ViewRef{ID: int64Value(row["view_id"]), Mode: stringValue(row["view_mode"])})
	}
	return out, nil
}

func computeActionViews(viewMode string, viewID int64, viewType string, explicit []action.ViewRef) []action.ViewRef {
	views := append([]action.ViewRef(nil), explicit...)
	seen := map[string]bool{}
	for _, item := range views {
		if item.Mode != "" {
			seen[item.Mode] = true
		}
	}
	missing := make([]string, 0)
	for _, mode := range strings.Split(viewMode, ",") {
		mode = strings.TrimSpace(mode)
		if mode != "" && !seen[mode] {
			missing = append(missing, mode)
		}
	}
	if viewID != 0 && viewType != "" {
		for index, mode := range missing {
			if mode == viewType {
				views = append(views, action.ViewRef{ID: viewID, Mode: viewType})
				missing = append(missing[:index], missing[index+1:]...)
				break
			}
		}
	}
	for _, mode := range missing {
		views = append(views, action.ViewRef{Mode: mode})
	}
	return views
}

func serverActionsFromEnv(env *record.Env, externalIDs map[string]data.ExternalID) (*serveractions.Registry, error) {
	actionHooks := envActionHooks{env: env}
	reg := serveractions.NewRegistry(serveractions.Hooks{
		Creator:           actionHooks,
		Writer:            actionHooks,
		Sequencer:         actionHooks,
		ObjectOperator:    actionHooks,
		Evaluator:         actionHooks,
		Mailer:            actionHooks,
		FollowerUpdater:   actionHooks,
		ActivityScheduler: actionHooks,
		SMSSender:         actionHooks,
		WhatsAppSender:    actionHooks,
		DocumentCreator:   actionHooks,
	})
	modelNames, err := modelNamesByID(env)
	if err != nil {
		return nil, err
	}
	fieldMeta, err := fieldMetadataByID(env)
	if err != nil {
		return nil, err
	}
	modelMeta, err := modelMetadataByName(env)
	if err != nil {
		return nil, err
	}
	templateMeta, err := mailTemplateMetadataByID(env, modelNames)
	if err != nil {
		return nil, err
	}
	automationMeta, err := automationMetadataByID(env, modelNames)
	if err != nil {
		return nil, err
	}
	rows, err := allRows(env, "ir.actions.server", "name", "model_id", "model_name", "state", "active", "sequence", "base_automation_id", "go_action_name", "mail_template_id", "template_id", "mail_post_autofollow", "mail_post_method", "followers_type", "followers_partner_field_name", "partner_ids", "activity_type_id", "activity_summary", "activity_note", "activity_date_deadline_range", "activity_date_deadline_range_type", "activity_user_type", "activity_user_id", "activity_user_field_name", "sms_template_id", "sms_method", "wa_template_id", "documents_account_create_model", "documents_account_journal_id", "documents_account_move_type", "queue_key", "webhook_url", "webhook_field_ids", "webhook_sample_payload", "code", "values", "value", "html_value", "evaluation_type", "sequence_id", "resource_ref", "selection_value", "parent_id", "child_ids", "crud_model_id", "crud_model_name", "link_field_id", "group_ids", "update_field_id", "update_path", "update_related_model_id", "update_field_type", "update_m2m_operation", "update_boolean_value", "warning", "ai_tool_ids", "ai_action_prompt", "ai_tool_description", "ai_tool_schema", "use_in_ai", "ai_tool_allow_end_message")
	if err != nil {
		return nil, err
	}
	actionXMLIDs := externalIDByRecord(externalIDs, "ir.actions.server")
	actions := make([]serveractions.ServerAction, 0, len(rows))
	actionsByID := make(map[int64]serveractions.ServerAction, len(rows))
	actionIndexByID := make(map[int64]int, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		action := serveractions.ServerAction{
			ID:                            id,
			Name:                          stringValue(row["name"]),
			Model:                         firstNonEmptyString(stringValue(row["model_name"]), modelNames[int64Value(row["model_id"])]),
			Kind:                          serverActionKind(row),
			Sequence:                      intWithFallback(row["sequence"], 5),
			BaseAutomationID:              int64Value(row["base_automation_id"]),
			GoActionName:                  stringValue(row["go_action_name"]),
			MailTemplateID:                int64Value(row["mail_template_id"]),
			TemplateID:                    int64Value(row["template_id"]),
			MailPostAutoFollow:            boolValue(row["mail_post_autofollow"]),
			MailPostMethod:                stringValue(row["mail_post_method"]),
			FollowersType:                 stringValue(row["followers_type"]),
			FollowersPartnerFieldName:     stringValue(row["followers_partner_field_name"]),
			PartnerIDs:                    int64Slice(row["partner_ids"]),
			ActivityTypeID:                int64Value(row["activity_type_id"]),
			ActivitySummary:               stringValue(row["activity_summary"]),
			ActivityNote:                  stringValue(row["activity_note"]),
			ActivityDateDeadlineRange:     int(int64Value(row["activity_date_deadline_range"])),
			ActivityDateDeadlineRangeType: stringValue(row["activity_date_deadline_range_type"]),
			ActivityUserType:              stringValue(row["activity_user_type"]),
			ActivityUserID:                int64Value(row["activity_user_id"]),
			ActivityUserFieldName:         stringValue(row["activity_user_field_name"]),
			SMSTemplateID:                 int64Value(row["sms_template_id"]),
			SMSMethod:                     stringValue(row["sms_method"]),
			WhatsAppTemplateID:            int64Value(row["wa_template_id"]),
			DocumentsAccountCreateModel:   stringValue(row["documents_account_create_model"]),
			DocumentsAccountJournalID:     int64Value(row["documents_account_journal_id"]),
			DocumentsAccountMoveType:      documentsAccountMoveType(stringValue(row["documents_account_move_type"]), stringValue(row["documents_account_create_model"])),
			QueueKey:                      stringValue(row["queue_key"]),
			WebhookURL:                    stringValue(row["webhook_url"]),
			WebhookFieldIDs:               int64Slice(row["webhook_field_ids"]),
			WebhookSamplePayload:          stringValue(row["webhook_sample_payload"]),
			Code:                          stringValue(row["code"]),
			Values:                        jsonMap(row["values"]),
			Value:                         stringValue(row["value"]),
			HTMLValue:                     stringValue(row["html_value"]),
			EvaluationType:                stringValue(row["evaluation_type"]),
			SequenceID:                    int64Value(row["sequence_id"]),
			ResourceRef:                   stringValue(row["resource_ref"]),
			SelectionValue:                int64Value(row["selection_value"]),
			ParentID:                      int64Value(row["parent_id"]),
			ChildIDs:                      int64Slice(row["child_ids"]),
			CrudModelID:                   int64Value(row["crud_model_id"]),
			CrudModelName:                 firstNonEmptyString(stringValue(row["crud_model_name"]), modelNames[int64Value(row["crud_model_id"])]),
			LinkFieldID:                   int64Value(row["link_field_id"]),
			LinkFieldName:                 fieldMeta[int64Value(row["link_field_id"])].Name,
			LinkFieldType:                 fieldMeta[int64Value(row["link_field_id"])].Type,
			GroupIDs:                      int64Slice(row["group_ids"]),
			UpdateFieldID:                 int64Value(row["update_field_id"]),
			UpdatePath:                    stringValue(row["update_path"]),
			UpdateRelatedModelID:          int64Value(row["update_related_model_id"]),
			UpdateFieldType:               stringValue(row["update_field_type"]),
			UpdateM2MOperation:            stringValue(row["update_m2m_operation"]),
			UpdateBooleanValue:            stringValue(row["update_boolean_value"]),
			Warning:                       stringValue(row["warning"]),
			AIActionPrompt:                stringValue(row["ai_action_prompt"]),
			AIToolIDs:                     int64Slice(row["ai_tool_ids"]),
			UseInAI:                       boolValue(row["use_in_ai"]),
			AIToolDescription:             stringValue(row["ai_tool_description"]),
			AIToolSchema:                  stringValue(row["ai_tool_schema"]),
			AIToolAllowEndMessage:         boolValue(row["ai_tool_allow_end_message"]),
			Disabled:                      row["active"] == false,
		}
		if updateFieldType := fieldMeta[action.UpdateFieldID].Type; updateFieldType != "" {
			action.UpdateFieldType = updateFieldType
		}
		if xmlID := actionXMLIDs[id]; xmlID != "" {
			action.Metadata = map[string]any{"xml_id": xmlID}
		}
		actionIndexByID[action.ID] = len(actions)
		actions = append(actions, action)
		actionsByID[action.ID] = action
	}
	for _, action := range actions {
		if action.ParentID == 0 || action.ID == 0 {
			continue
		}
		parent, ok := actionsByID[action.ParentID]
		if !ok || containsInt64(parent.ChildIDs, action.ID) {
			continue
		}
		parent.ChildIDs = append(parent.ChildIDs, action.ID)
		actionsByID[parent.ID] = parent
		if index, ok := actionIndexByID[parent.ID]; ok {
			actions[index] = parent
		}
	}
	for index, action := range actions {
		if len(action.ChildIDs) == 0 {
			continue
		}
		action.ChildIDs = filterNonAutomationChildIDs(action.ChildIDs, actionsByID)
		actions[index] = action
		actionsByID[action.ID] = action
	}
	for index, action := range actions {
		if parent := actionsByID[action.ParentID]; action.ParentID != 0 && parent.ID != 0 {
			action.Model = parent.Model
			action.GroupIDs = append([]int64(nil), parent.GroupIDs...)
		}
		actions[index] = action
		actionsByID[action.ID] = action
	}
	for index, action := range actions {
		action.Warning = firstNonEmptyString(action.Warning, computedServerActionWarning(action, actionsByID, fieldMeta, modelMeta, templateMeta, automationMeta))
		if action.Name == "" {
			action.Name = fmt.Sprintf("Server Action %d", action.ID)
		}
		if action.Kind == serveractions.KindCreate && action.Model == "" && action.CrudModelName == "" {
			action.Kind = serveractions.KindCode
		}
		actions[index] = action
		actionsByID[action.ID] = action
	}
	for _, action := range actions {
		if _, err := reg.Register(action); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func serverActionKind(row map[string]any) serveractions.Kind {
	if stringValue(row["go_action_name"]) != "" {
		return serveractions.KindGo
	}
	switch stringValue(row["state"]) {
	case "ai":
		return serveractions.KindAI
	case "object_create", "create":
		return serveractions.KindCreate
	case "object_write", "write":
		return serveractions.KindWrite
	case "object_copy":
		return serveractions.KindCopy
	case "multi":
		return serveractions.KindMulti
	case "mail_post":
		return serveractions.KindMailPost
	case "followers":
		return serveractions.KindFollowers
	case "remove_followers":
		return serveractions.KindRemoveFollowers
	case "next_activity":
		return serveractions.KindNextActivity
	case "sms":
		return serveractions.KindSMS
	case "whatsapp":
		return serveractions.KindWhatsApp
	case "documents_account_record_create":
		return serveractions.KindDocumentAccount
	case "email", "mail":
		return serveractions.KindSendMail
	case "webhook":
		return serveractions.KindWebhook
	case "python":
		return serveractions.KindPython
	default:
		return serveractions.KindCode
	}
}

func documentsAccountMoveType(raw string, createModel string) string {
	if raw != "" {
		return raw
	}
	for _, prefix := range []string{"account.move.", "account.bank."} {
		if strings.HasPrefix(createModel, prefix) {
			return strings.TrimPrefix(createModel, prefix)
		}
	}
	return ""
}

func computedServerActionWarning(action serveractions.ServerAction, actionsByID map[int64]serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata, modelMeta map[string]modelMetadata, templateMeta map[int64]mailTemplateMetadata, automationMeta map[int64]automationMetadata) string {
	return computedServerActionWarningWithVisited(action, actionsByID, fieldMeta, modelMeta, templateMeta, automationMeta, map[int64]bool{})
}

func computedServerActionWarningWithVisited(action serveractions.ServerAction, actionsByID map[int64]serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata, modelMeta map[string]modelMetadata, templateMeta map[int64]mailTemplateMetadata, automationMeta map[int64]automationMetadata, visited map[int64]bool) string {
	if action.ID != 0 {
		if visited[action.ID] {
			return ""
		}
		visited[action.ID] = true
		defer delete(visited, action.ID)
	}
	warnings := []string{}
	if action.Kind == serveractions.KindMulti {
		if children := childActionsWithDifferentModel(action, actionsByID); len(children) > 0 {
			warnings = append(warnings, fmt.Sprintf("Following child actions should have the same model (%s): %s", action.Model, strings.Join(children, ", ")))
		}
		if len(action.GroupIDs) > 0 {
			if children := childActionsWithDifferentGroups(action, actionsByID); len(children) > 0 {
				warnings = append(warnings, fmt.Sprintf("Following child actions should have the same groups: %s", strings.Join(children, ", ")))
			}
		}
		if children := childActionsWithWarnings(action, actionsByID, fieldMeta, modelMeta, templateMeta, automationMeta, visited); len(children) > 0 {
			warnings = append(warnings, fmt.Sprintf("Following child actions have warnings: %s", strings.Join(children, ", ")))
		}
	}
	updateFieldType := actionUpdateFieldType(action, fieldMeta)
	if action.Kind == serveractions.KindWrite && action.EvaluationType == "sequence" {
		switch updateFieldType {
		case "", "char", "text":
		default:
			warnings = append(warnings, "A sequence must only be used with character fields.")
		}
	}
	if action.Kind == serveractions.KindWrite && updateFieldType == "json" {
		warnings = append(warnings, fmt.Sprintf("I'm sorry to say that JSON fields (such as '%s') are currently not supported.", firstNonEmptyString(firstUpdatePathSegment(action.UpdatePath), action.UpdatePath)))
	}
	if action.Kind == serveractions.KindWebhook {
		restricted := restrictedWebhookFieldNames(action, fieldMeta)
		if len(restricted) > 0 {
			warnings = append(warnings, fmt.Sprintf("Group-restricted fields cannot be included in webhook payloads: %s", strings.Join(restricted, ", ")))
		}
	}
	if action.BaseAutomationID != 0 {
		if automation := automationMeta[action.BaseAutomationID]; automation.ID != 0 && automation.Model != "" && action.Model != "" && automation.Model != action.Model {
			warnings = append(warnings, fmt.Sprintf("Model of action %s should match the one from automated rule %s.", serverActionDisplayName(action), firstNonEmptyString(automation.Name, fmt.Sprintf("Automation %d", automation.ID))))
		}
	}
	warnings = append(warnings, mailServerActionWarnings(action, fieldMeta, modelMeta, templateMeta)...)
	return strings.Join(warnings, "\n\n")
}

func mailServerActionWarnings(action serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata, modelMeta map[string]modelMetadata, templateMeta map[int64]mailTemplateMetadata) []string {
	warnings := []string{}
	if action.ActivityDateDeadlineRange < 0 {
		warnings = append(warnings, "The 'Due Date In' value can't be negative.")
	}
	if action.Kind == serveractions.KindMailPost && action.TemplateID != 0 {
		if template := templateMeta[action.TemplateID]; template.Model != "" && action.Model != "" && template.Model != action.Model {
			warnings = append(warnings, "Mail template model of $(action_name)s does not match action model.")
		}
	}
	meta := modelMeta[action.Model]
	if mailStateRequiresNonTransientModel(action.Kind) && meta.Transient {
		warnings = append(warnings, "This action cannot be done on transient models.")
	}
	if actionRequiresMailThread(action) && !meta.IsMailThread {
		warnings = append(warnings, "This action can only be done on a mail thread models")
	}
	if action.Kind == serveractions.KindNextActivity && !meta.IsMailActivity {
		warnings = append(warnings, "A next activity can only be planned on models that use activities.")
	}
	if (action.Kind == serveractions.KindFollowers || action.Kind == serveractions.KindRemoveFollowers) && action.FollowersType == "generic" && action.FollowersPartnerFieldName != "" {
		fields, fieldChain := relationFieldChain(action.Model, action.FollowersPartnerFieldName, fieldMeta)
		if len(fields) > 0 && fields[len(fields)-1].Relation != "res.partner" {
			warnings = append(warnings, fmt.Sprintf("The field '%s' is not a partner field.", fieldChain))
		}
	}
	if action.Kind == serveractions.KindNextActivity && action.ActivityUserType == "generic" && action.ActivityUserFieldName != "" {
		fields, fieldChain := relationFieldChain(action.Model, action.ActivityUserFieldName, fieldMeta)
		if len(fields) > 0 && fields[len(fields)-1].Relation != "res.users" {
			warnings = append(warnings, fmt.Sprintf("The field '%s' is not a user field.", fieldChain))
		}
	}
	return warnings
}

func mailStateRequiresNonTransientModel(kind serveractions.Kind) bool {
	return kind == serveractions.KindMailPost || kind == serveractions.KindFollowers || kind == serveractions.KindRemoveFollowers || kind == serveractions.KindNextActivity
}

func actionRequiresMailThread(action serveractions.ServerAction) bool {
	return action.Kind == serveractions.KindFollowers ||
		action.Kind == serveractions.KindRemoveFollowers ||
		(action.Kind == serveractions.KindMailPost && action.MailPostMethod != "email")
}

func relationFieldChain(modelName string, fieldPath string, fieldMeta map[int64]modelFieldMetadata) ([]modelFieldMetadata, string) {
	parts := splitFieldPath(fieldPath)
	if modelName == "" || len(parts) == 0 {
		return nil, strings.TrimSpace(fieldPath)
	}
	currentModel := modelName
	out := make([]modelFieldMetadata, 0, len(parts))
	for _, part := range parts {
		meta := modelFieldMetadataByName(fieldMeta, currentModel, part)
		if meta.Name == "" {
			return nil, strings.Join(parts, ".")
		}
		out = append(out, meta)
		currentModel = meta.Relation
		if currentModel == "" && len(out) < len(parts) {
			return out, strings.Join(parts, ".")
		}
	}
	return out, strings.Join(parts, ".")
}

func childActionsWithDifferentModel(action serveractions.ServerAction, actionsByID map[int64]serveractions.ServerAction) []string {
	out := []string{}
	if action.Model == "" {
		return out
	}
	for _, childID := range action.ChildIDs {
		child, ok := actionsByID[childID]
		if !ok {
			continue
		}
		if child.Model != "" && child.Model != action.Model {
			out = append(out, serverActionDisplayName(child))
		}
	}
	return out
}

func filterNonAutomationChildIDs(childIDs []int64, actionsByID map[int64]serveractions.ServerAction) []int64 {
	out := make([]int64, 0, len(childIDs))
	for _, childID := range childIDs {
		child, ok := actionsByID[childID]
		if ok && child.BaseAutomationID != 0 {
			continue
		}
		out = append(out, childID)
	}
	return out
}

func childActionsWithDifferentGroups(action serveractions.ServerAction, actionsByID map[int64]serveractions.ServerAction) []string {
	out := []string{}
	for _, childID := range action.ChildIDs {
		child, ok := actionsByID[childID]
		if !ok {
			continue
		}
		if !sameInt64Set(child.GroupIDs, action.GroupIDs) {
			out = append(out, serverActionDisplayName(child))
		}
	}
	return out
}

func childActionsWithWarnings(action serveractions.ServerAction, actionsByID map[int64]serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata, modelMeta map[string]modelMetadata, templateMeta map[int64]mailTemplateMetadata, automationMeta map[int64]automationMetadata, visited map[int64]bool) []string {
	out := []string{}
	for _, childID := range action.ChildIDs {
		child, ok := actionsByID[childID]
		if !ok {
			continue
		}
		warning := firstNonEmptyString(child.Warning, computedServerActionWarningWithVisited(child, actionsByID, fieldMeta, modelMeta, templateMeta, automationMeta, visited))
		if strings.TrimSpace(warning) != "" {
			out = append(out, serverActionDisplayName(child))
		}
	}
	return out
}

func restrictedWebhookFieldNames(action serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata) []string {
	out := []string{}
	for _, fieldID := range action.WebhookFieldIDs {
		meta := fieldMeta[fieldID]
		if strings.TrimSpace(meta.Groups) != "" {
			out = append(out, firstNonEmptyString(meta.Name, fmt.Sprintf("field %d", fieldID)))
		}
	}
	return out
}

func sameInt64Set(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	l := append([]int64(nil), left...)
	r := append([]int64(nil), right...)
	sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
	sort.Slice(r, func(i, j int) bool { return r[i] < r[j] })
	for i := range l {
		if l[i] != r[i] {
			return false
		}
	}
	return true
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func serverActionDisplayName(action serveractions.ServerAction) string {
	return firstNonEmptyString(action.Name, fmt.Sprintf("Server Action %d", action.ID))
}

type envActionHooks struct {
	env *record.Env
	app *App
}

func (h envActionHooks) Create(_ context.Context, modelName string, values map[string]any) (int64, error) {
	return h.env.Model(modelName).Create(values)
}

func (h envActionHooks) Write(_ context.Context, modelName string, ids []int64, values map[string]any) error {
	return h.env.Model(modelName).Browse(ids...).Write(values)
}

func (h envActionHooks) NextSequence(ctx context.Context, sequenceID int64) (string, error) {
	return sequences.Service{Env: h.env}.NextByID(ctx, sequenceID)
}

func (h envActionHooks) CreateObject(ctx context.Context, action serveractions.ServerAction, exec serveractions.ExecutionContext, values map[string]any) (int64, error) {
	modelName := serverActionTargetModel(action, exec)
	activeIDs := serverActionTargetIDs(action, exec)
	createCount := 1
	if len(activeIDs) > 1 {
		createCount = len(activeIDs)
	}
	var createdID int64
	for index := 0; index < createCount; index++ {
		id, err := h.Create(ctx, modelName, values)
		if err != nil {
			return createdID, err
		}
		createdID = id
		if len(activeIDs) > 0 {
			activeID := activeIDs[0]
			if index < len(activeIDs) {
				activeID = activeIDs[index]
			}
			if err := h.linkCreatedRecord(action, exec, modelName, activeID, id); err != nil {
				return createdID, err
			}
		}
	}
	return createdID, nil
}

func (h envActionHooks) WriteObject(_ context.Context, action serveractions.ServerAction, exec serveractions.ExecutionContext, values map[string]any) ([]int64, error) {
	ids := serverActionTargetIDs(action, exec)
	if len(ids) == 0 {
		return nil, serveractions.ErrRecordSelectionMissing
	}
	modelName := serverActionBaseModel(action, exec)
	written := make([]int64, 0, len(ids))
	for _, id := range ids {
		recordExec := exec
		recordExec.RecordID = id
		recordExec.RecordIDs = []int64{id}
		recordValues := values
		if len(recordValues) == 0 {
			var err error
			recordValues, err = h.objectWriteValues(action, recordExec)
			if err != nil {
				return written, err
			}
		}
		targetModel, targetIDs, err := h.traverseActionPath(modelName, []int64{id}, action.UpdatePath)
		if err != nil {
			return written, err
		}
		if len(targetIDs) == 0 {
			continue
		}
		if err := h.writeObjectValues(targetModel, targetIDs, recordValues); err != nil {
			return written, err
		}
		written = append(written, targetIDs...)
	}
	return uniqueIDs(written), nil
}

func (h envActionHooks) CopyObject(ctx context.Context, action serveractions.ServerAction, exec serveractions.ExecutionContext) (int64, error) {
	modelName, sourceID := parseResourceRef(action.ResourceRef)
	if modelName == "" {
		modelName = action.CrudModelName
	}
	if modelName == "" {
		modelName = serverActionBaseModel(action, exec)
	}
	if sourceID == 0 {
		return 0, fmt.Errorf("object copy requires resource_ref")
	}
	values, err := h.copyValues(modelName, sourceID)
	if err != nil {
		return 0, err
	}
	activeIDs := serverActionTargetIDs(action, exec)
	createCount := 1
	if len(activeIDs) > 1 {
		createCount = len(activeIDs)
	}
	var createdID int64
	for index := 0; index < createCount; index++ {
		id, err := h.Create(ctx, modelName, values)
		if err != nil {
			return createdID, err
		}
		createdID = id
		if len(activeIDs) > 0 {
			activeID := activeIDs[0]
			if index < len(activeIDs) {
				activeID = activeIDs[index]
			}
			if err := h.linkCreatedRecord(action, exec, modelName, activeID, id); err != nil {
				return createdID, err
			}
		}
	}
	return createdID, nil
}

func (h envActionHooks) EvaluateActionValue(_ context.Context, action serveractions.ServerAction, exec serveractions.ExecutionContext) (any, error) {
	modelName := serverActionBaseModel(action, exec)
	ids := serverActionTargetIDs(action, exec)
	var recordID int64
	if len(ids) > 0 {
		recordID = ids[0]
	}
	value, err := data.SafeEvalExpression(action.Value, data.SafeEvalOptions{
		Env:       h.env,
		Model:     modelName,
		RecordID:  recordID,
		RecordIDs: ids,
		Locals:    cloneStringAnyMap(exec.Values),
	})
	if err != nil {
		return nil, err
	}
	switch action.UpdateFieldType {
	case "char", "text", "html", "selection":
		return fmt.Sprint(value), nil
	default:
		return value, nil
	}
}

func (h envActionHooks) objectWriteValues(action serveractions.ServerAction, exec serveractions.ExecutionContext) (map[string]any, error) {
	fieldName := updateFieldName(action.UpdatePath)
	if fieldName == "" {
		return cloneStringAnyMap(exec.Values), nil
	}
	value, err := h.objectActionValue(action, exec)
	if err != nil {
		return nil, err
	}
	return map[string]any{fieldName: value}, nil
}

func (h envActionHooks) objectActionValue(action serveractions.ServerAction, exec serveractions.ExecutionContext) (any, error) {
	switch action.EvaluationType {
	case "sequence":
		return h.NextSequence(context.Background(), action.SequenceID)
	case "equation":
		return h.EvaluateActionValue(context.Background(), action, exec)
	}
	switch action.UpdateFieldType {
	case "boolean", "bool":
		return action.UpdateBooleanValue == "true" || strings.EqualFold(action.Value, "true"), nil
	case "many2one", "integer", "int":
		if modelName, id := parseResourceRef(action.ResourceRef); id != 0 || modelName != "" {
			if action.UpdateFieldType == "many2one" {
				if id == 0 {
					return false, nil
				}
				return id, nil
			}
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(action.Value), 10, 64); err == nil {
			if id == 0 && action.UpdateFieldType == "many2one" {
				return false, nil
			}
			return id, nil
		}
	case "float":
		if value, err := strconv.ParseFloat(strings.TrimSpace(action.Value), 64); err == nil {
			return value, nil
		}
	case "html":
		return action.HTMLValue, nil
	case "one2many", "many2many":
		ids := idsFromAny(action.Value)
		if len(ids) == 0 {
			if _, id := parseResourceRef(action.ResourceRef); id != 0 {
				ids = []int64{id}
			}
		}
		switch action.UpdateM2MOperation {
		case "clear":
			return serveractions.RelationCommand{Operation: serveractions.RelationClear}, nil
		case "set":
			return serveractions.RelationCommand{Operation: serveractions.RelationSet, IDs: ids}, nil
		case "add":
			return serveractions.RelationCommand{Operation: serveractions.RelationAdd, IDs: ids}, nil
		case "remove":
			return serveractions.RelationCommand{Operation: serveractions.RelationRemove, IDs: ids}, nil
		}
	}
	if action.ResourceRef != "" {
		_, id := parseResourceRef(action.ResourceRef)
		return id, nil
	}
	return action.Value, nil
}

func (h envActionHooks) linkCreatedRecord(action serveractions.ServerAction, exec serveractions.ExecutionContext, createdModel string, activeID int64, createdID int64) error {
	if action.LinkFieldID == 0 && action.LinkFieldName == "" {
		return nil
	}
	fieldName := action.LinkFieldName
	fieldType := action.LinkFieldType
	relation := ""
	baseModel := serverActionBaseModel(action, exec)
	if meta, ok := h.env.ModelMetadata(baseModel); ok {
		f, exists := meta.Fields[fieldName]
		if !exists {
			return fmt.Errorf("link field %s.%s not found", baseModel, fieldName)
		}
		fieldType = string(f.Kind)
		relation = f.Relation
	}
	if fieldName == "" {
		return fmt.Errorf("link field metadata missing for action %d", action.ID)
	}
	if relation != "" && createdModel != "" && relation != createdModel {
		return fmt.Errorf("link field %s.%s relation %s does not match %s", baseModel, fieldName, relation, createdModel)
	}
	switch fieldType {
	case string(field.One2Many), string(field.Many2Many):
		return h.writeObjectValues(baseModel, []int64{activeID}, map[string]any{
			fieldName: serveractions.RelationCommand{Operation: serveractions.RelationAdd, IDs: []int64{createdID}},
		})
	default:
		return h.env.Model(baseModel).Browse(activeID).Write(map[string]any{fieldName: createdID})
	}
}

func (h envActionHooks) traverseActionPath(modelName string, ids []int64, path string) (string, []int64, error) {
	parts := splitFieldPath(path)
	if len(parts) <= 1 {
		return modelName, ids, nil
	}
	currentModel := modelName
	currentIDs := append([]int64(nil), ids...)
	for _, fieldName := range parts[:len(parts)-1] {
		meta, ok := h.env.ModelMetadata(currentModel)
		if !ok {
			return "", nil, fmt.Errorf("unknown model %s", currentModel)
		}
		f, ok := meta.Fields[fieldName]
		if !ok {
			return "", nil, fmt.Errorf("unknown field %s.%s", currentModel, fieldName)
		}
		if f.Relation == "" || (f.Kind != field.Many2One && f.Kind != field.One2Many && f.Kind != field.Many2Many) {
			return "", nil, fmt.Errorf("field %s.%s is not relational", currentModel, fieldName)
		}
		rows, err := h.env.Model(currentModel).Browse(currentIDs...).Read(fieldName)
		if err != nil {
			return "", nil, err
		}
		nextIDs := make([]int64, 0, len(rows))
		seen := map[int64]bool{}
		for _, row := range rows {
			for _, id := range idsFromAny(row[fieldName]) {
				if id == 0 || seen[id] {
					continue
				}
				seen[id] = true
				nextIDs = append(nextIDs, id)
			}
		}
		currentModel = f.Relation
		currentIDs = nextIDs
	}
	return currentModel, currentIDs, nil
}

func (h envActionHooks) writeObjectValues(modelName string, ids []int64, values map[string]any) error {
	plain := map[string]any{}
	for key, value := range values {
		command, ok := value.(serveractions.RelationCommand)
		if !ok {
			plain[key] = value
			continue
		}
		if err := h.applyRelationCommand(modelName, ids, key, command); err != nil {
			return err
		}
	}
	if len(plain) == 0 {
		return nil
	}
	return h.env.Model(modelName).Browse(ids...).Write(plain)
}

func (h envActionHooks) applyRelationCommand(modelName string, ids []int64, fieldName string, command serveractions.RelationCommand) error {
	rows, err := h.env.Model(modelName).Browse(ids...).Read(fieldName)
	if err != nil {
		return err
	}
	for _, row := range rows {
		id := int64Value(row["id"])
		current := idsFromAny(row[fieldName])
		next := applyRelationIDs(current, command)
		if err := h.env.Model(modelName).Browse(id).Write(map[string]any{fieldName: next}); err != nil {
			return err
		}
	}
	return nil
}

func (h envActionHooks) copyValues(modelName string, id int64) (map[string]any, error) {
	meta, ok := h.env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("unknown model %s", modelName)
	}
	fieldNames := make([]string, 0, len(meta.Fields))
	for name := range meta.Fields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	rows, err := h.env.Model(modelName).Browse(id).Read(fieldNames...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s,%d not found", modelName, id)
	}
	values := map[string]any{}
	for _, name := range fieldNames {
		values[name] = rows[0][name]
	}
	if name, ok := values["name"].(string); ok && name != "" {
		values["name"] = name + " (copy)"
	}
	return values, nil
}

func applyRelationIDs(current []int64, command serveractions.RelationCommand) []int64 {
	switch command.Operation {
	case serveractions.RelationClear:
		return []int64{}
	case serveractions.RelationSet:
		return uniqueIDs(command.IDs)
	case serveractions.RelationRemove:
		remove := map[int64]bool{}
		for _, id := range command.IDs {
			remove[id] = true
		}
		out := make([]int64, 0, len(current))
		for _, id := range current {
			if !remove[id] {
				out = append(out, id)
			}
		}
		return out
	case serveractions.RelationAdd:
		out := uniqueIDs(current)
		seen := map[int64]bool{}
		for _, id := range out {
			seen[id] = true
		}
		for _, id := range command.IDs {
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		return out
	default:
		return uniqueIDs(current)
	}
}

func uniqueIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func runtimeDate(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func runtimeDateValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return runtimeDate(typed)
	case string:
		text := strings.TrimSpace(typed)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return runtimeDate(parsed)
			}
		}
	}
	return time.Time{}
}

func dateInRange(date time.Time, from time.Time, to time.Time) bool {
	if date.IsZero() {
		return false
	}
	if !from.IsZero() && date.Before(from) {
		return false
	}
	if !to.IsZero() && date.After(to) {
		return false
	}
	return true
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func idsFromAny(value any) []int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int64:
		if typed == 0 {
			return nil
		}
		return []int64{typed}
	case int:
		if typed == 0 {
			return nil
		}
		return []int64{int64(typed)}
	case float64:
		if typed == 0 {
			return nil
		}
		return []int64{int64(typed)}
	case []int64:
		return append([]int64(nil), typed...)
	case []int:
		out := make([]int64, 0, len(typed))
		for _, id := range typed {
			if id != 0 {
				out = append(out, int64(id))
			}
		}
		return out
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, idsFromAny(item)...)
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var raw []any
		if err := json.Unmarshal([]byte(text), &raw); err == nil {
			return idsFromAny(raw)
		}
		if id, err := strconv.ParseInt(text, 10, 64); err == nil && id != 0 {
			return []int64{id}
		}
	}
	return nil
}

func splitFieldPath(path string) []string {
	raw := strings.Split(strings.TrimSpace(path), ".")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func updateFieldName(path string) string {
	parts := splitFieldPath(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func serverActionTargetModel(action serveractions.ServerAction, exec serveractions.ExecutionContext) string {
	if action.Kind == serveractions.KindCreate && action.CrudModelName != "" {
		return action.CrudModelName
	}
	if action.Kind == serveractions.KindCopy && action.CrudModelName != "" {
		return action.CrudModelName
	}
	return serverActionBaseModel(action, exec)
}

func serverActionBaseModel(action serveractions.ServerAction, exec serveractions.ExecutionContext) string {
	if action.Model != "" {
		return action.Model
	}
	return exec.Model
}

func serverActionTargetIDs(action serveractions.ServerAction, exec serveractions.ExecutionContext) []int64 {
	if len(action.RecordIDs) > 0 {
		return append([]int64(nil), action.RecordIDs...)
	}
	if len(exec.RecordIDs) > 0 {
		return append([]int64(nil), exec.RecordIDs...)
	}
	if exec.RecordID != 0 {
		return []int64{exec.RecordID}
	}
	return nil
}

func parseResourceRef(value string) (string, int64) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", 0
	}
	if modelName, rawID, ok := strings.Cut(text, ","); ok {
		id, _ := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		return strings.TrimSpace(modelName), id
	}
	id, _ := strconv.ParseInt(text, 10, 64)
	return "", id
}

func (h envActionHooks) SendMail(_ context.Context, request serveractions.MailRequest) error {
	template := h.mailTemplate(request.TemplateID)
	method := firstNonEmptyString(stringValue(request.Metadata["mail_post_method"]), "email")
	if method == "comment" || method == "note" {
		subtype := "mail.mt_note"
		if method == "comment" {
			subtype = "mail.mt_comment"
		}
		for _, recordID := range request.RecordIDs {
			if _, err := internalmail.PostMessage(h.env, internalmail.PostRequest{
				Model:        request.Model,
				ResID:        recordID,
				Body:         firstNonEmptyString(stringValue(template["body_html"]), stringValue(request.Values["body"])),
				Subject:      firstNonEmptyString(stringValue(template["subject"]), stringValue(request.Values["subject"])),
				MessageType:  "auto_comment",
				SubtypeXMLID: subtype,
				BodyIsHTML:   true,
				AutoFollow:   boolWithFallback(request.Metadata["mail_post_autofollow"], false),
			}); err != nil {
				return err
			}
		}
		return nil
	}
	for range request.RecordIDs {
		if _, err := h.env.Model("mail.mail").Create(map[string]any{
			"mail_message_id": int64(0),
			"attachment_ids":  int64Slice(firstNonNil(request.Values["attachment_ids"], template["attachment_ids"])),
			"mail_server_id":  int64Value(firstNonNil(request.Values["mail_server_id"], template["mail_server_id"])),
			"email_from":      firstNonEmptyString(stringValue(request.Values["email_from"]), stringValue(template["email_from"])),
			"email_to":        firstNonEmptyString(stringValue(template["email_to"]), stringValue(request.Values["email_to"])),
			"email_cc":        firstNonEmptyString(stringValue(request.Values["email_cc"]), stringValue(template["email_cc"])),
			"reply_to":        firstNonEmptyString(stringValue(request.Values["reply_to"]), stringValue(template["reply_to"])),
			"subject":         firstNonEmptyString(stringValue(template["subject"]), stringValue(request.Values["subject"])),
			"body_html":       firstNonEmptyString(stringValue(template["body_html"]), stringValue(request.Values["body_html"]), stringValue(request.Values["body"])),
			"state":           "outgoing",
			"scheduled_date":  time.Now().UTC(),
			"retry_count":     int64(0),
			"max_retries":     int64(3),
			"auto_delete":     boolWithFallback(firstNonNil(request.Values["auto_delete"], template["auto_delete"]), false),
			"headers":         stringValue(request.Values["headers"]),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h envActionHooks) SendSMS(_ context.Context, request serveractions.SMSRequest) error {
	if request.TemplateID == 0 || len(request.RecordIDs) == 0 {
		return nil
	}
	subject := firstNonEmptyString(request.Method, "sms")
	seenNumbers := map[string]bool{}
	for _, recordID := range request.RecordIDs {
		body, err := h.renderSMSTemplateBody(request.TemplateID)
		if err != nil {
			return err
		}
		recipient := h.smsRecipientInfo(request, recordID)
		number := recipient.Number
		smsCode := h.smsTraceCode(request.Metadata)
		failureType, err := h.smsMassRecipientFailureType(request, recordID, recipient, seenNumbers)
		if err != nil {
			return err
		}
		if number != "" {
			seenNumbers[number] = true
		}
		if failureType != "" && smsRequestMailingID(request) != 0 {
			if err := h.createSMSCanceledTrace(request, recordID, recipient, smsCode, failureType); err != nil {
				return err
			}
			continue
		}
		body = h.appendSMSUnsubscribeInfo(body, smsCode, request.Metadata)
		smsID, smsUUID, err := h.createSMSRecord(request, recordID, number, body)
		if err != nil {
			return err
		}
		if smsID != 0 {
			body, err = h.rewriteSMSBodyLinks(body, smsID, request.Metadata)
			if err != nil {
				return err
			}
			if err := h.env.Model("sms.sms").Browse(smsID).Write(map[string]any{"body": body}); err != nil {
				return err
			}
		}
		messageID, err := internalmail.PostMessage(h.env, internalmail.PostRequest{
			Model:        request.Model,
			ResID:        recordID,
			Body:         body,
			Subject:      subject,
			MessageType:  "sms",
			SubtypeXMLID: "mail.mt_comment",
			BodyIsHTML:   false,
		})
		if err != nil {
			return err
		}
		if smsID != 0 {
			if err := h.env.Model("sms.sms").Browse(smsID).Write(map[string]any{"mail_message_id": messageID}); err != nil {
				return err
			}
			notificationID, err := h.createSMSNotification(request, recordID, smsID, number, messageID)
			if err != nil {
				return err
			}
			if err := h.createSMSTraceAndTracker(request, recordID, smsID, smsUUID, number, smsCode, notificationID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h envActionHooks) renderSMSTemplateBody(templateID int64) (string, error) {
	body := fmt.Sprintf("SMS template %d", templateID)
	if h.env == nil || !runtimeModelHasField(h.env, "sms.template", "body") {
		return body, nil
	}
	templateRows, err := h.env.Model("sms.template").Browse(templateID).Read("body")
	if err != nil || len(templateRows) == 0 {
		return body, err
	}
	if templateBody := strings.TrimSpace(stringValue(templateRows[0]["body"])); templateBody != "" {
		body = templateBody
	}
	return body, nil
}

type smsRecipientInfo struct {
	Number string
	Raw    string
}

func (h envActionHooks) createSMSRecord(request serveractions.SMSRequest, recordID int64, number string, body string) (int64, string, error) {
	if h.env == nil || !runtimeModelHasField(h.env, "sms.sms", "body") {
		return 0, "", nil
	}
	smsUUID := runtimeSMSUUID()
	values := map[string]any{
		"uuid":      smsUUID,
		"number":    number,
		"body":      body,
		"state":     "outgoing",
		"to_delete": false,
	}
	if request.Model == "res.partner" && recordID != 0 && runtimeModelHasField(h.env, "sms.sms", "partner_id") {
		values["partner_id"] = recordID
	}
	if mailingID := int64Value(firstNonNil(request.Metadata["mailing_id"], request.Metadata["mass_mailing_id"])); mailingID != 0 && runtimeModelHasField(h.env, "sms.sms", "mailing_id") {
		values["mailing_id"] = mailingID
	}
	smsID, err := h.env.Model("sms.sms").Create(values)
	if err != nil {
		return 0, "", err
	}
	return smsID, smsUUID, nil
}

func (h envActionHooks) rewriteSMSBodyLinks(body string, smsID int64, metadata map[string]any) (string, error) {
	if h.env == nil || smsID == 0 || int64Value(firstNonNil(metadata["mailing_id"], metadata["mass_mailing_id"])) == 0 {
		return body, nil
	}
	baseURL := runtimeBaseURL(h.env)
	trackerValues := h.smsTrackerValues(metadata)
	var firstErr error
	out := whatsappURLTextPattern.ReplaceAllStringFunc(body, func(rawURL string) string {
		if firstErr != nil {
			return rawURL
		}
		if smsTrackingURLSkipped(rawURL, baseURL) {
			return rawURL
		}
		if rewritten, ok := appendSMSTrackingSuffix(rawURL, smsID, baseURL); ok {
			return rewritten
		}
		shortURL, err := internalmail.SearchOrCreateLinkTrackerShortURL(h.env, rawURL, trackerValues, baseURL)
		if err != nil {
			firstErr = err
			return rawURL
		}
		if shortURL == "" {
			return rawURL
		}
		return shortURL + fmt.Sprintf("/s/%d", smsID)
	})
	if firstErr != nil {
		return body, firstErr
	}
	return out, nil
}

func (h envActionHooks) smsTrackerValues(metadata map[string]any) map[string]any {
	values := map[string]any{}
	if campaignID := int64Value(firstNonNil(metadata["campaign_id"], metadata["utm_campaign_id"])); campaignID != 0 {
		values["campaign_id"] = campaignID
	}
	if sourceID := int64Value(firstNonNil(metadata["source_id"], metadata["utm_source_id"])); sourceID != 0 {
		values["source_id"] = sourceID
	}
	if mediumID := int64Value(firstNonNil(metadata["medium_id"], metadata["utm_medium_id"])); mediumID != 0 {
		values["medium_id"] = mediumID
	}
	if mailingID := int64Value(firstNonNil(metadata["mailing_id"], metadata["mass_mailing_id"])); mailingID != 0 {
		values["mass_mailing_id"] = mailingID
	}
	return values
}

func (h envActionHooks) appendSMSUnsubscribeInfo(body string, smsCode string, metadata map[string]any) string {
	mailingID := int64Value(firstNonNil(metadata["mailing_id"], metadata["mass_mailing_id"]))
	if h.env == nil || mailingID == 0 || smsCode == "" || !boolWithFallback(metadata["sms_allow_unsubscribe"], false) {
		return body
	}
	unsubscribe := fmt.Sprintf("STOP SMS: %s/sms/%d/%s", runtimeBaseURL(h.env), mailingID, smsCode)
	if strings.Contains(body, unsubscribe) {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return unsubscribe
	}
	return body + "\n" + unsubscribe
}

func (h envActionHooks) smsTraceCode(metadata map[string]any) string {
	if value := strings.TrimSpace(stringValue(metadata["sms_code"])); value != "" {
		return value
	}
	return randomSMSCode()
}

func (h envActionHooks) createSMSNotification(request serveractions.SMSRequest, recordID int64, smsID int64, number string, messageID int64) (int64, error) {
	if h.env == nil || smsID == 0 || messageID == 0 || !runtimeModelHasField(h.env, "mail.notification", "mail_message_id") {
		return 0, nil
	}
	values := map[string]any{"mail_message_id": messageID}
	if runtimeModelHasField(h.env, "mail.notification", "notification_type") {
		values["notification_type"] = "sms"
	}
	if runtimeModelHasField(h.env, "mail.notification", "notification_status") {
		values["notification_status"] = "ready"
	}
	if runtimeModelHasField(h.env, "mail.notification", "sms_id") {
		values["sms_id"] = smsID
	}
	if runtimeModelHasField(h.env, "mail.notification", "sms_id_int") {
		values["sms_id_int"] = smsID
	}
	if runtimeModelHasField(h.env, "mail.notification", "sms_number") {
		values["sms_number"] = number
	}
	if runtimeModelHasField(h.env, "mail.notification", "is_read") {
		values["is_read"] = true
	}
	if request.Model == "res.partner" && recordID != 0 && runtimeModelHasField(h.env, "mail.notification", "res_partner_id") {
		values["res_partner_id"] = recordID
	}
	if runtimeModelHasField(h.env, "mail.notification", "author_id") && runtimeModelHasField(h.env, "mail.message", "author_id") {
		rows, err := h.env.Model("mail.message").Browse(messageID).Read("author_id")
		if err != nil {
			return 0, err
		}
		if len(rows) > 0 {
			if authorID := int64Value(rows[0]["author_id"]); authorID != 0 {
				values["author_id"] = authorID
			}
		}
	}
	return h.env.Model("mail.notification").Create(values)
}

func (h envActionHooks) createSMSTraceAndTracker(request serveractions.SMSRequest, recordID int64, smsID int64, smsUUID string, number string, smsCode string, notificationID int64) error {
	if h.env == nil || smsID == 0 {
		return nil
	}
	if int64Value(firstNonNil(request.Metadata["mailing_id"], request.Metadata["mass_mailing_id"])) == 0 || !runtimeModelHasField(h.env, "mailing.trace", "sms_id_int") {
		return h.ensureSMSTracker(smsUUID, notificationID, 0)
	}
	values := map[string]any{
		"trace_type":   "sms",
		"model":        request.Model,
		"res_id":       recordID,
		"sms_id":       smsID,
		"sms_id_int":   smsID,
		"sms_number":   number,
		"sms_code":     smsCode,
		"trace_status": "outgoing",
	}
	for key, value := range h.smsTrackerValues(request.Metadata) {
		values[key] = value
	}
	traceID, err := h.env.Model("mailing.trace").Create(values)
	if err != nil {
		return err
	}
	return h.ensureSMSTracker(smsUUID, notificationID, traceID)
}

func (h envActionHooks) createSMSCanceledTrace(request serveractions.SMSRequest, recordID int64, recipient smsRecipientInfo, smsCode string, failureType string) error {
	if h.env == nil || smsRequestMailingID(request) == 0 || !runtimeModelHasField(h.env, "mailing.trace", "trace_status") {
		return nil
	}
	values := map[string]any{
		"trace_type":   "sms",
		"model":        request.Model,
		"res_id":       recordID,
		"sms_number":   firstNonEmptyString(recipient.Number, recipient.Raw),
		"sms_code":     smsCode,
		"trace_status": "cancel",
		"failure_type": failureType,
	}
	for key, value := range h.smsTrackerValues(request.Metadata) {
		values[key] = value
	}
	_, err := h.env.Model("mailing.trace").Create(values)
	return err
}

func (h envActionHooks) smsMassRecipientFailureType(request serveractions.SMSRequest, recordID int64, recipient smsRecipientInfo, seenNumbers map[string]bool) (string, error) {
	if smsRequestMailingID(request) == 0 {
		return "", nil
	}
	number := strings.TrimSpace(recipient.Number)
	if number == "" {
		if strings.TrimSpace(recipient.Raw) != "" {
			return "sms_number_format", nil
		}
		return "sms_number_missing", nil
	}
	blacklisted, err := h.smsNumberBlacklisted(request, number)
	if err != nil || blacklisted {
		if blacklisted {
			return "sms_blacklist", nil
		}
		return "", err
	}
	optedOut, err := h.smsRecipientOptedOut(request, recordID)
	if err != nil || optedOut {
		if optedOut {
			return "sms_optout", nil
		}
		return "", err
	}
	if seenNumbers[number] {
		return "sms_duplicate", nil
	}
	alreadyTraced, err := h.smsRecipientAlreadyTraced(request, recordID)
	if err != nil || alreadyTraced {
		if alreadyTraced {
			return "sms_duplicate", nil
		}
		return "", err
	}
	return "", nil
}

func (h envActionHooks) smsNumberBlacklisted(request serveractions.SMSRequest, number string) (bool, error) {
	if h.env == nil || strings.TrimSpace(number) == "" || !h.smsUseExclusionList(request) {
		return false, nil
	}
	if _, ok := h.env.ModelMetadata("phone.blacklist"); !ok {
		return false, nil
	}
	found, err := h.env.Model("phone.blacklist").Search(domain.And(
		domain.Cond("number", domain.Equal, strings.TrimSpace(number)),
		domain.Cond("active", domain.Equal, true),
	))
	if err != nil {
		return false, err
	}
	return found.Len() > 0, nil
}

func (h envActionHooks) smsUseExclusionList(request serveractions.SMSRequest) bool {
	if _, ok := request.Metadata["use_exclusion_list"]; ok {
		return boolWithFallback(request.Metadata["use_exclusion_list"], true)
	}
	mailingID := smsRequestMailingID(request)
	if h.env == nil || mailingID == 0 || !runtimeModelHasField(h.env, "mailing.mailing", "use_exclusion_list") {
		return true
	}
	rows, err := h.env.Model("mailing.mailing").Browse(mailingID).Read("use_exclusion_list")
	if err != nil || len(rows) == 0 {
		return true
	}
	return boolWithFallback(rows[0]["use_exclusion_list"], true)
}

func (h envActionHooks) smsRecipientOptedOut(request serveractions.SMSRequest, recordID int64) (bool, error) {
	mailingID := smsRequestMailingID(request)
	if h.env == nil || mailingID == 0 || recordID == 0 || request.Model != "mailing.contact" {
		return false, nil
	}
	if !runtimeModelHasField(h.env, "mailing.mailing", "contact_list_ids") || !runtimeModelHasField(h.env, "mailing.subscription", "opt_out") {
		return false, nil
	}
	mailingRows, err := h.env.Model("mailing.mailing").Browse(mailingID).Read("contact_list_ids")
	if err != nil || len(mailingRows) == 0 {
		return false, err
	}
	listIDs := int64Slice(mailingRows[0]["contact_list_ids"])
	if len(listIDs) == 0 {
		return false, nil
	}
	found, err := h.env.Model("mailing.subscription").Search(domain.Cond("contact_id", domain.Equal, recordID))
	if err != nil || found.Len() == 0 {
		return false, err
	}
	rows, err := found.Read("list_id", "opt_out")
	if err != nil {
		return false, err
	}
	optOut := false
	optIn := false
	for _, row := range rows {
		if !containsRuntimeInt64(listIDs, int64Value(row["list_id"])) {
			continue
		}
		if boolWithFallback(row["opt_out"], false) {
			optOut = true
		} else {
			optIn = true
		}
	}
	return optOut && !optIn, nil
}

func (h envActionHooks) smsRecipientAlreadyTraced(request serveractions.SMSRequest, recordID int64) (bool, error) {
	mailingID := smsRequestMailingID(request)
	if h.env == nil || mailingID == 0 || recordID == 0 || !runtimeModelHasField(h.env, "mailing.trace", "mass_mailing_id") {
		return false, nil
	}
	found, err := h.env.Model("mailing.trace").Search(domain.And(
		domain.Cond("mass_mailing_id", domain.Equal, mailingID),
		domain.Cond("trace_type", domain.Equal, "sms"),
		domain.Cond("model", domain.Equal, request.Model),
		domain.Cond("res_id", domain.Equal, recordID),
	))
	if err != nil {
		return false, err
	}
	return found.Len() > 0, nil
}

func smsRequestMailingID(request serveractions.SMSRequest) int64 {
	return int64Value(firstNonNil(request.Metadata["mailing_id"], request.Metadata["mass_mailing_id"]))
}

func containsRuntimeInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (h envActionHooks) ensureSMSTracker(smsUUID string, notificationID int64, traceID int64) error {
	if h.env == nil || strings.TrimSpace(smsUUID) == "" || !runtimeModelHasField(h.env, "sms.tracker", "sms_uuid") {
		return nil
	}
	values := map[string]any{"sms_uuid": strings.TrimSpace(smsUUID)}
	if notificationID != 0 && runtimeModelHasField(h.env, "sms.tracker", "mail_notification_id") {
		values["mail_notification_id"] = notificationID
	}
	if traceID != 0 && runtimeModelHasField(h.env, "sms.tracker", "mailing_trace_id") {
		values["mailing_trace_id"] = traceID
	}
	if len(values) == 1 {
		return nil
	}
	found, err := h.env.Model("sms.tracker").Search(domain.Cond("sms_uuid", domain.Equal, values["sms_uuid"]))
	if err != nil {
		return err
	}
	if found.Len() == 0 {
		_, err = h.env.Model("sms.tracker").Create(values)
		return err
	}
	fields := []string{"id"}
	if runtimeModelHasField(h.env, "sms.tracker", "mail_notification_id") {
		fields = append(fields, "mail_notification_id")
	}
	if runtimeModelHasField(h.env, "sms.tracker", "mailing_trace_id") {
		fields = append(fields, "mailing_trace_id")
	}
	rows, err := found.Read(fields...)
	if err != nil || len(rows) == 0 {
		return err
	}
	writeValues := map[string]any{}
	if notificationID != 0 && runtimeModelHasField(h.env, "sms.tracker", "mail_notification_id") && int64Value(rows[0]["mail_notification_id"]) != notificationID {
		writeValues["mail_notification_id"] = notificationID
	}
	if traceID != 0 && runtimeModelHasField(h.env, "sms.tracker", "mailing_trace_id") && int64Value(rows[0]["mailing_trace_id"]) != traceID {
		writeValues["mailing_trace_id"] = traceID
	}
	if len(writeValues) == 0 {
		return nil
	}
	return h.env.Model("sms.tracker").Browse(int64Value(rows[0]["id"])).Write(writeValues)
}

func (h envActionHooks) smsRecipientNumber(request serveractions.SMSRequest, recordID int64) string {
	return h.smsRecipientInfo(request, recordID).Number
}

func (h envActionHooks) smsRecipientInfo(request serveractions.SMSRequest, recordID int64) smsRecipientInfo {
	metadataCountry := h.smsPhoneCountryFromMetadata(request.Metadata)
	for _, key := range []string{"sms_number", "number", "phone", "mobile"} {
		if value := strings.TrimSpace(stringValue(request.Metadata[key])); value != "" {
			return normalizeSMSRecipientRaw(value, metadataCountry)
		}
	}
	if h.env == nil || request.Model == "" || recordID == 0 {
		return smsRecipientInfo{}
	}
	fields := make([]string, 0, 3)
	for _, fieldName := range []string{"mobile", "phone"} {
		if runtimeModelHasField(h.env, request.Model, fieldName) {
			fields = append(fields, fieldName)
		}
	}
	if runtimeModelHasField(h.env, request.Model, "country_id") {
		fields = append(fields, "country_id")
	}
	if len(fields) == 0 {
		return smsRecipientInfo{}
	}
	rows, err := h.env.Model(request.Model).Browse(recordID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return smsRecipientInfo{}
	}
	country := metadataCountry
	if strings.TrimSpace(country.Code) == "" && country.PhoneCode == 0 {
		if countryID := int64Value(rows[0]["country_id"]); countryID != 0 {
			country = h.phoneCountryByID(countryID)
		}
	}
	if strings.TrimSpace(country.Code) == "" && country.PhoneCode == 0 {
		country = h.companyPhoneCountry()
	}
	for _, fieldName := range fields {
		if fieldName == "country_id" {
			continue
		}
		if value := strings.TrimSpace(stringValue(rows[0][fieldName])); value != "" {
			return normalizeSMSRecipientRaw(value, country)
		}
	}
	return smsRecipientInfo{}
}

func normalizeSMSRecipientRaw(value string, country phone.Country) smsRecipientInfo {
	value = strings.TrimSpace(value)
	if value == "" {
		return smsRecipientInfo{}
	}
	if formatted, ok := phone.FormatE164(value, country); ok {
		return smsRecipientInfo{Number: formatted, Raw: value}
	}
	if smsCountryKnown(country) && smsLooksLocalNumber(value) {
		return smsRecipientInfo{Raw: value}
	}
	return smsRecipientInfo{Number: phone.NormalizeE164(value, country), Raw: value}
}

func smsCountryKnown(country phone.Country) bool {
	return strings.TrimSpace(country.Code) != "" || country.PhoneCode != 0
}

func smsLooksLocalNumber(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "+") {
		return false
	}
	var digits strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return !strings.HasPrefix(digits.String(), "00")
}

func (h envActionHooks) smsPhoneCountryFromMetadata(metadata map[string]any) phone.Country {
	if h.env == nil || metadata == nil {
		return phone.Country{}
	}
	if countryID := int64Value(metadata["country_id"]); countryID != 0 {
		return h.phoneCountryByID(countryID)
	}
	country := phone.Country{
		Code:      strings.ToUpper(strings.TrimSpace(stringValue(metadata["country_code"]))),
		PhoneCode: int64Value(firstNonNil(metadata["phone_code"], metadata["country_phone_code"])),
	}
	if country.Code != "" && country.PhoneCode == 0 && runtimeModelHasField(h.env, "res.country", "code") {
		found, err := h.env.Model("res.country").SearchWithOptions(domain.Cond("code", domain.Equal, country.Code), record.SearchOptions{Limit: 1})
		if err == nil && found.Len() > 0 {
			rows, err := found.Read("id")
			if err == nil && len(rows) > 0 {
				return h.phoneCountryByID(int64Value(rows[0]["id"]))
			}
		}
	}
	return country
}

func (h envActionHooks) companyPhoneCountry() phone.Country {
	if h.env == nil || h.env.Context().CompanyID == 0 || !runtimeModelHasField(h.env, "res.company", "country_id") {
		return phone.Country{}
	}
	rows, err := h.env.Model("res.company").Browse(h.env.Context().CompanyID).Read("country_id")
	if err != nil || len(rows) == 0 {
		return phone.Country{}
	}
	return h.phoneCountryByID(int64Value(rows[0]["country_id"]))
}

func (h envActionHooks) phoneCountryByID(countryID int64) phone.Country {
	if h.env == nil || countryID == 0 || !runtimeModelHasField(h.env, "res.country", "code") {
		return phone.Country{}
	}
	fields := []string{"code"}
	if runtimeModelHasField(h.env, "res.country", "phone_code") {
		fields = append(fields, "phone_code")
	}
	rows, err := h.env.Model("res.country").Browse(countryID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return phone.Country{}
	}
	return phone.Country{
		Code:      strings.ToUpper(strings.TrimSpace(stringValue(rows[0]["code"]))),
		PhoneCode: int64Value(rows[0]["phone_code"]),
	}
}

func (h envActionHooks) SendWhatsApp(_ context.Context, request serveractions.WhatsAppRequest) error {
	if request.TemplateID == 0 || len(request.RecordIDs) == 0 {
		return nil
	}
	template, err := h.whatsAppTemplateForSend(request.TemplateID, request.Model)
	if err != nil {
		return err
	}
	for _, recordID := range request.RecordIDs {
		whatsappID, err := h.ensureWhatsAppMessage(request, recordID)
		if err != nil {
			return err
		}
		body, components, err := h.renderWhatsAppTemplatePayload(template, request.Model, recordID, whatsappID, request.Metadata)
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			body = fmt.Sprintf("WhatsApp template %d", request.TemplateID)
		}
		messageID, err := internalmail.PostMessage(h.env, internalmail.PostRequest{
			Model:        request.Model,
			ResID:        recordID,
			Body:         body,
			Subject:      "whatsapp",
			MessageType:  "whatsapp",
			SubtypeXMLID: "mail.mt_comment",
			BodyIsHTML:   false,
		})
		if err != nil {
			return err
		}
		if err := h.updateWhatsAppMessagePayload(whatsappID, request, recordID, messageID, body, components); err != nil {
			return err
		}
	}
	return nil
}

var whatsappURLTextPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
var whatsappTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*(\d+)\s*\}\}`)

type whatsappTemplateRuntime struct {
	ID         int64
	Body       string
	HeaderType string
	HeaderText string
	Model      string
	Status     string
	Quality    string
}

func (h envActionHooks) whatsAppTemplateForSend(templateID int64, requestModel string) (whatsappTemplateRuntime, error) {
	template := whatsappTemplateRuntime{ID: templateID, Body: fmt.Sprintf("WhatsApp template %d", templateID)}
	if h.env == nil {
		return template, nil
	}
	if _, ok := h.env.ModelMetadata("whatsapp.template"); !ok {
		return template, nil
	}
	fields := runtimeWhatsAppTemplateReadFields(h.env)
	rows, err := h.env.Model("whatsapp.template").Browse(templateID).Read(fields...)
	if err != nil {
		return template, err
	}
	if len(rows) == 0 {
		return template, fmt.Errorf("whatsapp template %d not found", templateID)
	}
	row := rows[0]
	if value := strings.TrimSpace(stringValue(row["body"])); value != "" {
		template.Body = value
	}
	template.HeaderType = strings.TrimSpace(stringValue(row["header_type"]))
	template.HeaderText = strings.TrimSpace(stringValue(row["header_text"]))
	template.Model = strings.TrimSpace(stringValue(row["model"]))
	template.Status = strings.ToLower(strings.TrimSpace(stringValue(row["status"])))
	template.Quality = strings.ToLower(strings.TrimSpace(stringValue(row["quality"])))
	if runtimeModelHasField(h.env, "whatsapp.template", "status") && template.Status != "approved" {
		if template.Status == "" {
			template.Status = "draft"
		}
		return template, fmt.Errorf("whatsapp template %d is not approved", templateID)
	}
	if runtimeModelHasField(h.env, "whatsapp.template", "quality") && template.Quality == "red" {
		return template, fmt.Errorf("whatsapp template %d has red quality", templateID)
	}
	if template.Model != "" && strings.TrimSpace(requestModel) != "" && template.Model != strings.TrimSpace(requestModel) {
		return template, fmt.Errorf("whatsapp template %d targets %s, not %s", templateID, template.Model, requestModel)
	}
	return template, nil
}

func runtimeWhatsAppTemplateReadFields(env *record.Env) []string {
	fields := []string{"body"}
	for _, fieldName := range []string{"header_type", "header_text", "model", "status", "quality"} {
		if runtimeModelHasField(env, "whatsapp.template", fieldName) {
			fields = append(fields, fieldName)
		}
	}
	return fields
}

func (h envActionHooks) ensureWhatsAppMessage(request serveractions.WhatsAppRequest, recordID int64) (int64, error) {
	if h.env == nil {
		return 0, nil
	}
	if _, ok := h.env.ModelMetadata("whatsapp.message"); !ok {
		return 0, nil
	}
	existingID := int64(0)
	if len(request.RecordIDs) == 1 {
		existingID = int64Value(request.Metadata["whatsapp_message_id"])
	}
	values := map[string]any{
		"state":  "sent",
		"model":  request.Model,
		"res_id": recordID,
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "wa_template_id") {
		values["wa_template_id"] = request.TemplateID
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "template_id") {
		values["template_id"] = request.TemplateID
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "free_text_json") {
		if freeTextJSON := whatsappFreeTextJSONString(request.Metadata); freeTextJSON != "" {
			values["free_text_json"] = freeTextJSON
		}
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "msg_uid") {
		if msgUID := strings.TrimSpace(stringValue(request.Metadata["msg_uid"])); msgUID != "" {
			values["msg_uid"] = msgUID
		}
	}
	if existingID != 0 && runtimeRecordExists(h.env, "whatsapp.message", existingID) {
		return existingID, h.env.Model("whatsapp.message").Browse(existingID).Write(values)
	}
	return h.env.Model("whatsapp.message").Create(values)
}

func (h envActionHooks) renderWhatsAppTemplatePayload(template whatsappTemplateRuntime, recordModel string, recordID int64, whatsappID int64, metadata map[string]any) (string, []map[string]any, error) {
	body := template.Body
	baseURL := runtimeBaseURL(h.env)
	trackerValues := h.whatsappTemplateTrackerValues(whatsappID, metadata)
	freeText := whatsappFreeTextValues(metadata)
	variables, err := h.whatsappTemplateVariableRows(template.ID)
	if err != nil {
		return body, nil, err
	}
	bodyVariables := whatsappTemplateVariablesByLine(variables, "body")
	headerVariables := whatsappTemplateVariablesByLine(variables, "header")
	bodyValues, err := h.whatsappTemplateVariableTexts(bodyVariables, recordModel, recordID, freeText, baseURL)
	if err != nil {
		return body, nil, err
	}
	body = renderWhatsAppTemplateText(body, bodyVariables, bodyValues)
	components := make([]map[string]any, 0)
	if strings.EqualFold(template.HeaderType, "text") && len(headerVariables) != 0 {
		headerValues, err := h.whatsappTemplateVariableTexts(headerVariables, recordModel, recordID, freeText, baseURL)
		if err != nil {
			return body, nil, err
		}
		components = append(components, map[string]any{
			"type":       "header",
			"parameters": whatsappTemplateTextParameters(headerValues),
		})
	}
	if strings.TrimSpace(body) != "" {
		parameters := []map[string]any{{"type": "text", "text": body}}
		if len(bodyVariables) != 0 {
			if len(trackerValues) != 0 {
				for index, value := range bodyValues {
					bodyValues[index] = h.shortenWhatsAppParameterURL(value, whatsappID, trackerValues, baseURL)
				}
			}
			parameters = whatsappTemplateTextParameters(bodyValues)
		}
		components = append(components, map[string]any{
			"type":       "body",
			"parameters": parameters,
		})
	}
	buttonComponents, err := h.whatsappButtonComponents(template.ID, whatsappID, trackerValues, baseURL, recordModel, recordID, freeText)
	if err != nil {
		return body, nil, err
	}
	components = append(components, buttonComponents...)
	return body, components, nil
}

func (h envActionHooks) whatsappTemplateTrackerValues(whatsappID int64, metadata map[string]any) map[string]any {
	campaignID := int64Value(metadata["campaign_id"])
	if campaignID == 0 {
		campaignID = int64Value(metadata["utm_campaign_id"])
	}
	if campaignID == 0 {
		campaignID = h.whatsappMessageUTMCampaignID(whatsappID)
	}
	if campaignID == 0 {
		return nil
	}
	return map[string]any{"campaign_id": campaignID}
}

func (h envActionHooks) whatsappMessageUTMCampaignID(whatsappID int64) int64 {
	if h.env == nil || whatsappID == 0 || !runtimeModelHasField(h.env, "marketing.trace", "whatsapp_message_id") {
		return 0
	}
	traces, err := h.env.Model("marketing.trace").SearchWithOptions(domain.Cond("whatsapp_message_id", domain.Equal, whatsappID), record.SearchOptions{Limit: 1})
	if err != nil || traces.Len() == 0 {
		return 0
	}
	traceRows, err := traces.Read("activity_id")
	if err != nil || len(traceRows) == 0 {
		return 0
	}
	activityID := int64Value(traceRows[0]["activity_id"])
	if activityID == 0 || !runtimeModelHasField(h.env, "marketing.activity", "campaign_id") {
		return 0
	}
	activityRows, err := h.env.Model("marketing.activity").Browse(activityID).Read("campaign_id")
	if err != nil || len(activityRows) == 0 {
		return 0
	}
	campaignID := int64Value(activityRows[0]["campaign_id"])
	if campaignID == 0 || !runtimeModelHasField(h.env, "marketing.campaign", "utm_campaign_id") {
		return 0
	}
	campaignRows, err := h.env.Model("marketing.campaign").Browse(campaignID).Read("utm_campaign_id")
	if err != nil || len(campaignRows) == 0 {
		return 0
	}
	return int64Value(campaignRows[0]["utm_campaign_id"])
}

func (h envActionHooks) shortenWhatsAppBodyLinks(body string, whatsappID int64, trackerValues map[string]any, baseURL string) (string, error) {
	var firstErr error
	out := whatsappURLTextPattern.ReplaceAllStringFunc(body, func(rawURL string) string {
		if firstErr != nil {
			return rawURL
		}
		shortURL, err := internalmail.SearchOrCreateLinkTrackerShortURL(h.env, rawURL, trackerValues, baseURL)
		if err != nil {
			firstErr = err
			return rawURL
		}
		if shortURL == "" {
			return rawURL
		}
		return shortURL + fmt.Sprintf("/w/%d", whatsappID)
	})
	if firstErr != nil {
		return body, firstErr
	}
	return out, nil
}

func (h envActionHooks) whatsappButtonComponents(templateID int64, whatsappID int64, trackerValues map[string]any, baseURL string, recordModel string, recordID int64, freeText map[string]string) ([]map[string]any, error) {
	if h.env == nil {
		return nil, nil
	}
	if _, ok := h.env.ModelMetadata("whatsapp.template.button"); !ok {
		return nil, nil
	}
	buttons, err := h.env.Model("whatsapp.template.button").Search(runtimeWhatsAppTemplateButtonDomain(h.env, templateID))
	if err != nil || buttons.Len() == 0 {
		return nil, err
	}
	rows, err := buttons.Read("id", "sequence", "button_type", "url_type", "website_url", "dynamic_url", "text", "name")
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		return int64Value(rows[i]["sequence"]) < int64Value(rows[j]["sequence"])
	})
	components := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(stringValue(row["button_type"])) != "url" {
			continue
		}
		text := firstNonEmptyString(stringValue(row["website_url"]), stringValue(row["dynamic_url"]))
		if stringValue(row["url_type"]) == "tracked" && len(trackerValues) != 0 && strings.TrimSpace(stringValue(row["website_url"])) != "" {
			shortURL, err := internalmail.SearchOrCreateLinkTrackerShortURL(h.env, strings.TrimSpace(stringValue(row["website_url"])), trackerValues, baseURL)
			if err != nil {
				return nil, err
			}
			text = strings.TrimLeft(strings.Replace(shortURL+fmt.Sprintf("/w/%d", whatsappID), strings.TrimRight(baseURL, "/"), "", 1), "/")
		} else if stringValue(row["url_type"]) == "dynamic" {
			variables, err := h.whatsappButtonVariableRows(int64Value(row["id"]))
			if err != nil {
				return nil, err
			}
			values, err := h.whatsappTemplateVariableTexts(variables, recordModel, recordID, freeText, baseURL)
			if err != nil {
				return nil, err
			}
			if len(values) != 0 {
				text = whatsappDynamicURLSuffix(values[0], stringValue(row["website_url"]))
			}
		}
		components = append(components, map[string]any{
			"type":     "button",
			"sub_type": "url",
			"index":    int64Value(row["sequence"]),
			"text":     firstNonEmptyString(stringValue(row["text"]), stringValue(row["name"])),
			"parameters": []map[string]any{{
				"type": "text",
				"text": text,
			}},
		})
	}
	return components, nil
}

func (h envActionHooks) whatsappTemplateVariableRows(templateID int64) ([]map[string]any, error) {
	if h.env == nil || templateID == 0 {
		return nil, nil
	}
	if _, ok := h.env.ModelMetadata("whatsapp.template.variable"); !ok {
		return nil, nil
	}
	variables, err := h.env.Model("whatsapp.template.variable").Search(domain.Cond("wa_template_id", domain.Equal, templateID))
	if err != nil || variables.Len() == 0 {
		return nil, err
	}
	rows, err := variables.Read(runtimeWhatsAppTemplateVariableReadFields(h.env)...)
	if err != nil {
		return nil, err
	}
	sortWhatsAppTemplateVariables(rows)
	return rows, nil
}

func (h envActionHooks) whatsappButtonVariableRows(buttonID int64) ([]map[string]any, error) {
	if h.env == nil || buttonID == 0 {
		return nil, nil
	}
	if _, ok := h.env.ModelMetadata("whatsapp.template.variable"); !ok {
		return nil, nil
	}
	variables, err := h.env.Model("whatsapp.template.variable").Search(domain.Cond("button_id", domain.Equal, buttonID))
	if err != nil || variables.Len() == 0 {
		return nil, err
	}
	rows, err := variables.Read(runtimeWhatsAppTemplateVariableReadFields(h.env)...)
	if err != nil {
		return nil, err
	}
	sortWhatsAppTemplateVariables(rows)
	return rows, nil
}

func runtimeWhatsAppTemplateVariableReadFields(env *record.Env) []string {
	fields := []string{"id"}
	for _, fieldName := range []string{"name", "button_id", "wa_template_id", "model", "line_type", "field_type", "field_name", "demo_value"} {
		if runtimeModelHasField(env, "whatsapp.template.variable", fieldName) {
			fields = append(fields, fieldName)
		}
	}
	return fields
}

func whatsappTemplateVariablesByLine(rows []map[string]any, lineType string) []map[string]any {
	out := make([]map[string]any, 0)
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(stringValue(row["line_type"])), lineType) && int64Value(row["button_id"]) == 0 {
			out = append(out, row)
		}
	}
	sortWhatsAppTemplateVariables(out)
	return out
}

func sortWhatsAppTemplateVariables(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := whatsappTemplateVariableIndex(rows[i])
		right := whatsappTemplateVariableIndex(rows[j])
		if left != right {
			return left < right
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
}

func (h envActionHooks) whatsappTemplateVariableTexts(rows []map[string]any, recordModel string, recordID int64, freeText map[string]string, baseURL string) ([]string, error) {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value, err := h.whatsappTemplateVariableText(row, recordModel, recordID, freeText, baseURL)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) == "" {
			value = " "
		}
		values = append(values, value)
	}
	return values, nil
}

func (h envActionHooks) whatsappTemplateVariableText(row map[string]any, recordModel string, recordID int64, freeText map[string]string, baseURL string) (string, error) {
	fieldType := strings.TrimSpace(stringValue(row["field_type"]))
	switch fieldType {
	case "field":
		value, err := h.whatsappRecordFieldValue(recordModel, recordID, strings.TrimSpace(stringValue(row["field_name"])))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return value, nil
		}
	case "portal_url":
		modelName := firstNonEmptyString(strings.TrimSpace(stringValue(row["model"])), recordModel)
		if modelName != "" && recordID != 0 {
			return strings.TrimRight(baseURL, "/") + "/odoo/" + strings.ReplaceAll(modelName, ".", "/") + "/" + strconv.FormatInt(recordID, 10), nil
		}
	case "user_name":
		if value := h.whatsappCurrentUserField("name"); strings.TrimSpace(value) != "" {
			return value, nil
		}
	case "user_phone":
		if value := h.whatsappCurrentUserPhone(); strings.TrimSpace(value) != "" {
			return value, nil
		}
	case "free_text":
		if value := whatsappFreeTextVariableValue(row, freeText); strings.TrimSpace(value) != "" {
			return value, nil
		}
	}
	if value := strings.TrimSpace(stringValue(row["demo_value"])); value != "" {
		return value, nil
	}
	if value := whatsappFreeTextVariableValue(row, freeText); strings.TrimSpace(value) != "" {
		return value, nil
	}
	return " ", nil
}

func (h envActionHooks) whatsappRecordFieldValue(modelName string, recordID int64, fieldPath string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	fieldPath = strings.TrimSpace(fieldPath)
	if h.env == nil || modelName == "" || recordID == 0 || fieldPath == "" {
		return "", nil
	}
	parts := strings.Split(fieldPath, ".")
	currentModel := modelName
	currentID := recordID
	for index, part := range parts {
		part = strings.TrimSpace(part)
		meta, ok := h.env.ModelMetadata(currentModel)
		if !ok {
			return "", nil
		}
		fieldMeta, ok := meta.Fields[part]
		if !ok {
			return "", nil
		}
		rows, err := h.env.Model(currentModel).Browse(currentID).Read(part)
		if err != nil || len(rows) == 0 {
			return "", err
		}
		value := rows[0][part]
		if index == len(parts)-1 {
			return strings.TrimSpace(stringValue(value)), nil
		}
		if fieldMeta.Relation == "" {
			return "", nil
		}
		currentID = int64Value(value)
		if currentID == 0 {
			return "", nil
		}
		currentModel = fieldMeta.Relation
	}
	return "", nil
}

func (h envActionHooks) whatsappCurrentUserField(fieldName string) string {
	if h.env == nil || h.env.Context().UserID == 0 || !runtimeModelHasField(h.env, "res.users", fieldName) {
		return ""
	}
	rows, err := h.env.Model("res.users").Browse(h.env.Context().UserID).Read(fieldName)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringValue(rows[0][fieldName]))
}

func (h envActionHooks) whatsappCurrentUserPhone() string {
	if h.env == nil || h.env.Context().UserID == 0 {
		return ""
	}
	if runtimeModelHasField(h.env, "res.users", "phone") {
		return h.whatsappCurrentUserField("phone")
	}
	if !runtimeModelHasField(h.env, "res.users", "partner_id") || !runtimeModelHasField(h.env, "res.partner", "phone") {
		return ""
	}
	rows, err := h.env.Model("res.users").Browse(h.env.Context().UserID).Read("partner_id")
	if err != nil || len(rows) == 0 {
		return ""
	}
	partnerID := int64Value(rows[0]["partner_id"])
	if partnerID == 0 {
		return ""
	}
	partnerRows, err := h.env.Model("res.partner").Browse(partnerID).Read("phone")
	if err != nil || len(partnerRows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringValue(partnerRows[0]["phone"]))
}

func renderWhatsAppTemplateText(text string, variables []map[string]any, values []string) string {
	out := text
	for index, row := range variables {
		if index >= len(values) {
			break
		}
		placeholderIndex := whatsappTemplateVariableIndex(row)
		if placeholderIndex == 0 {
			continue
		}
		pattern := regexp.MustCompile(fmt.Sprintf(`\{\{\s*%d\s*\}\}`, placeholderIndex))
		out = pattern.ReplaceAllString(out, values[index])
	}
	return out
}

func whatsappTemplateTextParameters(values []string) []map[string]any {
	parameters := make([]map[string]any, 0, len(values))
	for _, value := range values {
		parameters = append(parameters, map[string]any{"type": "text", "text": value})
	}
	return parameters
}

func (h envActionHooks) shortenWhatsAppParameterURL(value string, whatsappID int64, trackerValues map[string]any, baseURL string) string {
	trimmed := strings.TrimSpace(value)
	match := whatsappURLTextPattern.FindStringIndex(trimmed)
	if trimmed == "" || len(match) != 2 || match[0] != 0 {
		return value
	}
	return whatsappURLTextPattern.ReplaceAllStringFunc(value, func(rawURL string) string {
		shortURL, err := internalmail.SearchOrCreateLinkTrackerShortURL(h.env, rawURL, trackerValues, baseURL)
		if err != nil || shortURL == "" {
			return rawURL
		}
		return shortURL + fmt.Sprintf("/w/%d", whatsappID)
	})
}

func whatsappDynamicURLSuffix(value string, baseURL string) string {
	value = strings.TrimSpace(value)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" && strings.HasPrefix(value, baseURL) {
		value = strings.TrimPrefix(value, baseURL)
	}
	return strings.TrimLeft(value, "/")
}

func whatsappTemplateVariableIndex(row map[string]any) int {
	name := strings.TrimSpace(stringValue(row["name"]))
	if match := whatsappTemplatePlaceholderPattern.FindStringSubmatch(name); len(match) == 2 {
		parsed, _ := strconv.Atoi(match[1])
		return parsed
	}
	digits := ""
	for _, char := range name {
		if char >= '0' && char <= '9' {
			digits += string(char)
		}
	}
	if digits == "" {
		return int(int64Value(row["id"]))
	}
	parsed, _ := strconv.Atoi(digits)
	return parsed
}

func whatsappFreeTextVariableValue(row map[string]any, freeText map[string]string) string {
	if len(freeText) == 0 {
		return ""
	}
	name := strings.TrimSpace(stringValue(row["name"]))
	lineType := strings.TrimSpace(stringValue(row["line_type"]))
	index := whatsappTemplateVariableIndex(row)
	candidates := []string{
		lineType + "-" + name,
		name,
	}
	if index != 0 {
		candidates = append([]string{
			fmt.Sprintf("%s-{{%d}}", lineType, index),
			fmt.Sprintf("free_text_%d", index),
		}, candidates...)
	}
	if buttonID := int64Value(row["button_id"]); buttonID != 0 {
		candidates = append([]string{
			fmt.Sprintf("button-%d", buttonID),
			fmt.Sprintf("button_dynamic_url_%d", index),
		}, candidates...)
	}
	for _, key := range candidates {
		if value := strings.TrimSpace(freeText[key]); value != "" {
			return value
		}
	}
	return ""
}

func whatsappFreeTextValues(metadata map[string]any) map[string]string {
	out := map[string]string{}
	if len(metadata) == 0 {
		return out
	}
	appendMap := func(values map[string]any) {
		for key, value := range values {
			if text := strings.TrimSpace(stringValue(value)); text != "" {
				out[key] = text
			}
		}
	}
	switch typed := metadata["free_text_json"].(type) {
	case map[string]any:
		appendMap(typed)
	case map[string]string:
		for key, value := range typed {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	case string:
		appendMap(jsonMap(typed))
	}
	for key, value := range metadata {
		if key == "free_text_json" {
			continue
		}
		if text := strings.TrimSpace(stringValue(value)); text != "" {
			out[key] = text
		}
	}
	return out
}

func whatsappFreeTextJSONString(metadata map[string]any) string {
	values := whatsappFreeTextValues(metadata)
	if len(values) == 0 {
		return ""
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(payload)
}

func runtimeWhatsAppTemplateButtonDomain(env *record.Env, templateID int64) domain.Node {
	var conditions []domain.Node
	if runtimeModelHasField(env, "whatsapp.template.button", "wa_template_id") {
		conditions = append(conditions, domain.Cond("wa_template_id", domain.Equal, templateID))
	}
	if runtimeModelHasField(env, "whatsapp.template.button", "template_id") {
		conditions = append(conditions, domain.Cond("template_id", domain.Equal, templateID))
	}
	switch len(conditions) {
	case 0:
		return domain.Bool(false)
	case 1:
		return conditions[0]
	default:
		return domain.Or(conditions...)
	}
}

func (h envActionHooks) updateWhatsAppMessagePayload(whatsappID int64, request serveractions.WhatsAppRequest, recordID int64, mailMessageID int64, body string, components []map[string]any) error {
	if h.env == nil || whatsappID == 0 || !runtimeModelHasField(h.env, "whatsapp.message", "body") {
		return nil
	}
	componentsJSON := ""
	if len(components) != 0 {
		payload, err := json.Marshal(components)
		if err != nil {
			return err
		}
		componentsJSON = string(payload)
	}
	values := map[string]any{
		"mail_message_id": mailMessageID,
		"model":           request.Model,
		"res_id":          recordID,
		"body":            body,
		"components":      componentsJSON,
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "wa_template_id") {
		values["wa_template_id"] = request.TemplateID
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "template_id") {
		values["template_id"] = request.TemplateID
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "free_text_json") {
		if freeTextJSON := whatsappFreeTextJSONString(request.Metadata); freeTextJSON != "" {
			values["free_text_json"] = freeTextJSON
		}
	}
	if runtimeModelHasField(h.env, "whatsapp.message", "msg_uid") {
		if msgUID := strings.TrimSpace(stringValue(request.Metadata["msg_uid"])); msgUID != "" {
			values["msg_uid"] = msgUID
		}
	}
	return h.env.Model("whatsapp.message").Browse(whatsappID).Write(values)
}

func (h envActionHooks) CreateDocumentAccountRecord(_ context.Context, request serveractions.DocumentAccountRecordRequest) (any, error) {
	if request.Model != "documents.document" || len(request.RecordIDs) == 0 || request.CreateModel == "" {
		return nil, nil
	}
	switch {
	case request.CreateModel == "account.move" || strings.HasPrefix(request.CreateModel, "account.move."):
		return h.createDocumentAccountMoves(request)
	case request.CreateModel == "account.bank.statement":
		return h.createDocumentBankStatements(request)
	default:
		return nil, fmt.Errorf("unsupported documents account create model %s", request.CreateModel)
	}
}

func (h envActionHooks) createDocumentAccountMoves(request serveractions.DocumentAccountRecordRequest) (any, error) {
	if _, ok := h.env.ModelMetadata("account.move"); !ok {
		return nil, fmt.Errorf("unknown model account.move")
	}
	moveType := firstNonEmptyString(request.MoveType, documentsAccountMoveType("", request.CreateModel), "entry")
	createdIDs := make([]int64, 0, len(request.RecordIDs))
	today := time.Now().UTC().Format("2006-01-02")
	for _, documentID := range request.RecordIDs {
		values := map[string]any{
			"name":      fmt.Sprintf("Document %d", documentID),
			"date":      today,
			"state":     "draft",
			"move_type": moveType,
		}
		if request.JournalID != 0 {
			values["journal_id"] = request.JournalID
		}
		createdID, err := h.env.Model("account.move").Create(values)
		if err != nil {
			return nil, err
		}
		createdIDs = append(createdIDs, createdID)
	}
	return documentAccountWindowAction("Generated Entries", "account.move", createdIDs), nil
}

func (h envActionHooks) createDocumentBankStatements(request serveractions.DocumentAccountRecordRequest) (any, error) {
	if _, ok := h.env.ModelMetadata("account.bank.statement"); !ok {
		return nil, fmt.Errorf("unknown model account.bank.statement")
	}
	createdIDs := make([]int64, 0, len(request.RecordIDs))
	for _, documentID := range request.RecordIDs {
		values := map[string]any{
			"name":  fmt.Sprintf("Document %d", documentID),
			"state": "open",
		}
		if request.JournalID != 0 {
			values["journal_id"] = request.JournalID
		}
		createdID, err := h.env.Model("account.bank.statement").Create(values)
		if err != nil {
			return nil, err
		}
		createdIDs = append(createdIDs, createdID)
	}
	return documentAccountWindowAction("Generated Statements", "account.bank.statement", createdIDs), nil
}

func documentAccountWindowAction(name string, modelName string, ids []int64) map[string]any {
	action := map[string]any{
		"type":      "ir.actions.act_window",
		"name":      name,
		"res_model": modelName,
		"view_mode": "form",
		"domain":    []any{[]any{"id", "in", ids}},
	}
	if len(ids) == 1 {
		action["res_id"] = ids[0]
	}
	return action
}

func (h envActionHooks) SendWorkflowEmail(_ context.Context, request internalworkflow.EmailRequest) error {
	if request.TemplateID == 0 || request.RecordID == 0 {
		return nil
	}
	_, err := internalmail.SendTemplateBatch(h.env, internalmail.TemplateSendRequest{
		TemplateID:  request.TemplateID,
		Model:       request.Model,
		ResIDs:      []int64{request.RecordID},
		EmailValues: request.Values,
		UserID:      request.UserID,
		CCExpander:  h.delegationMailCCExpander(),
	})
	return err
}

func (h envActionHooks) delegationMailCCExpander() internalmail.CCExpander {
	if h.env == nil {
		return nil
	}
	return func(request internalmail.CCExpansionRequest) ([]string, error) {
		groupIDs := uniqueIDs(request.TemplateGroupIDs)
		if len(groupIDs) == 0 {
			return request.InitialCC, nil
		}
		employeeIDs := h.delegationRecipientEmployeeIDs(request)
		if len(employeeIDs) == 0 {
			return request.InitialCC, nil
		}
		delegationIDs := h.activeDelegationIDs(employeeIDs, request.At)
		if len(delegationIDs) == 0 {
			return request.InitialCC, nil
		}
		delegateEmployeeIDs := h.delegationLineEmployeeIDs(delegationIDs, groupIDs)
		if len(delegateEmployeeIDs) == 0 {
			return request.InitialCC, nil
		}
		return uniqueStrings(append(request.InitialCC, h.employeeWorkEmails(delegateEmployeeIDs)...)), nil
	}
}

func (h envActionHooks) delegationRecipientEmployeeIDs(request internalmail.CCExpansionRequest) []int64 {
	env := h.systemEnv()
	if env == nil {
		return nil
	}
	var employeeIDs []int64
	if _, ok := env.ModelMetadata("hr.employee"); ok && len(request.RecipientEmails) > 0 {
		found, err := env.Model("hr.employee").Search(domain.Cond("work_email", "in", uniqueStrings(request.RecipientEmails)))
		if err == nil {
			if rows, err := found.Read("id"); err == nil {
				for _, row := range rows {
					employeeIDs = append(employeeIDs, int64Value(row["id"]))
				}
			}
		}
	}
	if _, ok := env.ModelMetadata("res.users"); ok && len(request.PartnerIDs) > 0 {
		found, err := env.Model("res.users").Search(domain.Cond("partner_id", "in", uniqueIDs(request.PartnerIDs)))
		if err == nil {
			rows, err := found.Read("employee_ids", "employee_id")
			if err == nil {
				for _, row := range rows {
					employeeIDs = append(employeeIDs, int64Slice(row["employee_ids"])...)
					if id := int64Value(row["employee_id"]); id != 0 {
						employeeIDs = append(employeeIDs, id)
					}
				}
			}
		}
	}
	return uniqueIDs(employeeIDs)
}

func (h envActionHooks) activeDelegationIDs(employeeIDs []int64, at time.Time) []int64 {
	env := h.systemEnv()
	if env == nil {
		return nil
	}
	if _, ok := env.ModelMetadata(oi_delegation.ModelDelegation); !ok {
		return nil
	}
	found, err := env.Model(oi_delegation.ModelDelegation).Search(domain.And(
		domain.Cond("employee_id", "in", uniqueIDs(employeeIDs)),
		domain.Cond("state", "=", string(internaldelegation.StateConfirmed)),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("id", "date_from", "date_to")
	if err != nil {
		return nil
	}
	today := runtimeDate(at)
	var ids []int64
	for _, row := range rows {
		if dateInRange(today, runtimeDateValue(row["date_from"]), runtimeDateValue(row["date_to"])) {
			ids = append(ids, int64Value(row["id"]))
		}
	}
	return uniqueIDs(ids)
}

func (h envActionHooks) delegationLineEmployeeIDs(delegationIDs []int64, groupIDs []int64) []int64 {
	env := h.systemEnv()
	if env == nil {
		return nil
	}
	if _, ok := env.ModelMetadata(oi_delegation.ModelDelegationLine); !ok {
		return nil
	}
	found, err := env.Model(oi_delegation.ModelDelegationLine).Search(domain.And(
		domain.Cond("delegation_id", "in", uniqueIDs(delegationIDs)),
		domain.Cond("group_id", "in", uniqueIDs(groupIDs)),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("employee_id", "active")
	if err != nil {
		return nil
	}
	var ids []int64
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		if id := int64Value(row["employee_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return uniqueIDs(ids)
}

func (h envActionHooks) employeeWorkEmails(employeeIDs []int64) []string {
	env := h.systemEnv()
	if env == nil || len(employeeIDs) == 0 {
		return nil
	}
	rows, err := env.Model("hr.employee").Browse(uniqueIDs(employeeIDs)...).Read("work_email")
	if err != nil {
		return nil
	}
	var emails []string
	for _, row := range rows {
		if email := strings.TrimSpace(stringValue(row["work_email"])); email != "" {
			emails = append(emails, email)
		}
	}
	return uniqueStrings(emails)
}

func (h envActionHooks) systemEnv() *record.Env {
	if h.env == nil {
		return nil
	}
	ctx := h.env.Context()
	ctx.UserID = 1
	ctx.Values = cloneStringAnyMap(ctx.Values)
	return h.env.WithContext(ctx)
}

func (h envActionHooks) UpdateFollowers(_ context.Context, request serveractions.FollowersRequest) error {
	for _, recordID := range request.RecordIDs {
		partnerIDs := append([]int64(nil), request.PartnerIDs...)
		if len(partnerIDs) == 0 && request.PartnerFieldName != "" {
			partnerIDs = h.relatedRecordIDs(request.Model, recordID, request.PartnerFieldName)
		}
		if request.Remove {
			if err := internalmail.Unsubscribe(h.env, request.Model, recordID, partnerIDs); err != nil {
				return err
			}
			continue
		}
		if err := internalmail.Subscribe(h.env, request.Model, recordID, partnerIDs, nil); err != nil {
			return err
		}
	}
	return nil
}

func (h envActionHooks) ScheduleActivity(_ context.Context, request serveractions.ActivityRequest) error {
	now := request.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	deadline := activityDeadline(now, request.DeadlineRange, request.DeadlineRangeType)
	for _, recordID := range request.RecordIDs {
		userID := request.UserID
		if request.UserType == "generic" && request.UserFieldName != "" {
			ids := h.relatedRecordIDs(request.Model, recordID, request.UserFieldName)
			if len(ids) > 0 {
				userID = ids[0]
			}
		}
		if userID == 0 {
			continue
		}
		if _, err := h.env.Model("mail.activity").Create(map[string]any{
			"activity_type_id": request.ActivityTypeID,
			"res_model":        request.Model,
			"res_id":           recordID,
			"user_id":          userID,
			"date_deadline":    deadline,
			"summary":          request.Summary,
			"note":             request.Note,
			"state":            "open",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h envActionHooks) mailTemplate(id int64) map[string]any {
	if id == 0 {
		return map[string]any{}
	}
	rows, err := h.env.Model("mail.template").Browse(id).Read("subject", "body_html", "email_to", "email_cc", "email_from", "reply_to", "attachment_ids", "mail_server_id", "auto_delete")
	if err != nil || len(rows) == 0 {
		return map[string]any{}
	}
	return rows[0]
}

func (h envActionHooks) relatedRecordIDs(modelName string, recordID int64, fieldPath string) []int64 {
	rows, err := h.env.Model(modelName).Browse(recordID).Read(strings.Split(strings.TrimSpace(fieldPath), ".")[0])
	if err != nil || len(rows) == 0 {
		return nil
	}
	value := rows[0][strings.Split(strings.TrimSpace(fieldPath), ".")[0]]
	return int64Slice(value)
}

func activityDeadline(now time.Time, amount int, unit string) string {
	if amount <= 0 {
		return now.UTC().Format("2006-01-02")
	}
	switch unit {
	case "weeks":
		return now.UTC().AddDate(0, 0, amount*7).Format("2006-01-02")
	case "months":
		return now.UTC().AddDate(0, amount, 0).Format("2006-01-02")
	default:
		return now.UTC().AddDate(0, 0, amount).Format("2006-01-02")
	}
}

func viewsFromEnv(env *record.Env) (*view.Registry, error) {
	reg := view.NewRegistry()
	rows, err := allRows(env, "ir.ui.view", "name", "model", "type", "arch", "inherit_id", "mode", "priority", "active", "groups_id", "primary")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !boolWithFallback(row["active"], true) {
			continue
		}
		arch := stringValue(row["arch"])
		viewType := view.Type(stringValue(row["type"]))
		if viewType == "" {
			viewType = inferViewType(arch)
		}
		if stringValue(row["model"]) == "" && viewType == "qweb" {
			continue
		}
		if err := reg.AddWithID(view.View{
			ID:        int64Value(row["id"]),
			Name:      stringValue(row["name"]),
			Model:     stringValue(row["model"]),
			Type:      viewType,
			Arch:      arch,
			InheritID: int64Value(row["inherit_id"]),
			Mode:      stringValue(row["mode"]),
			Primary:   boolWithFallback(row["primary"], false),
			Priority:  int(int64Value(row["priority"])),
			Groups:    int64Slice(row["groups_id"]),
		}); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func menusFromEnv(env *record.Env, externalIDs map[string]data.ExternalID) (*menu.Registry, error) {
	reg := menu.NewRegistry()
	rows, err := allRows(env, "ir.ui.menu", "name", "active", "parent_id", "action", "sequence", "groups_id", "web_icon", "web_icon_data", "web_icon_data_mimetype")
	if err != nil {
		return nil, err
	}
	recordModules := externalIDModules(externalIDs, "ir.ui.menu")
	menuXMLIDs := externalIDByRecord(externalIDs, "ir.ui.menu")
	actionXMLIDs := externalIDByActionRecord(externalIDs)
	actionsByID, err := actionsByIDFromEnv(env, externalIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !boolWithFallback(row["active"], true) {
			continue
		}
		id := int64Value(row["id"])
		actionValue := stringValue(row["action"])
		actionID := actionIDFromMenuValue(actionValue, recordModules[id], externalIDs)
		actionModel := actionModelFromMenuValue(actionValue, actionID)
		reg.AddWithID(menu.Menu{
			ID:                  id,
			Name:                stringValue(row["name"]),
			ParentID:            int64Value(row["parent_id"]),
			Action:              resolvedMenuActionValue(actionValue, actionID, actionModel, actionXMLIDs),
			ActionID:            actionID,
			ActionModel:         actionModel,
			ActionPath:          actionsByID[actionID].Path,
			Groups:              int64Slice(row["groups_id"]),
			Sequence:            intWithFallback(row["sequence"], int(id)),
			WebIcon:             stringValue(row["web_icon"]),
			WebIconData:         stringValue(row["web_icon_data"]),
			WebIconDataMimetype: stringValue(row["web_icon_data_mimetype"]),
			XMLID:               menuXMLIDs[id],
		})
	}
	return reg, nil
}

func actionsByIDFromEnv(env *record.Env, externalIDs map[string]data.ExternalID) (map[int64]action.Action, error) {
	reg, err := actionsFromEnv(env, externalIDs)
	if err != nil {
		return nil, err
	}
	out := map[int64]action.Action{}
	for _, row := range reg.All() {
		out[row.ID] = row
	}
	return out, nil
}

func allRows(env *record.Env, modelName string, fields ...string) ([]map[string]any, error) {
	env = activeTestDisabledEnv(env)
	found, err := env.Model(modelName).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read(fields...)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"]) })
	return rows, nil
}

func activeTestDisabledEnv(env *record.Env) *record.Env {
	ctx := env.Context()
	values := cloneStringAnyMap(ctx.Values)
	if values == nil {
		values = map[string]any{}
	}
	values["active_test"] = false
	ctx.Values = values
	return env.WithContext(ctx)
}

func actionIDFromMenuValue(value string, moduleName string, externalIDs map[string]data.ExternalID) int64 {
	_, raw, ok := strings.Cut(value, ",")
	if !ok {
		return 0
	}
	raw = strings.TrimSpace(raw)
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return id
	}
	if strings.Contains(raw, ".") {
		return externalIDs[raw].ResID
	}
	if moduleName != "" {
		return externalIDs[moduleName+"."+raw].ResID
	}
	return 0
}

func actionModelFromMenuValue(value string, actionID int64) string {
	modelName, _, ok := strings.Cut(strings.TrimSpace(value), ",")
	if ok && strings.TrimSpace(modelName) != "" {
		return strings.TrimSpace(modelName)
	}
	if actionID != 0 {
		return string(action.ActWindow)
	}
	return ""
}

func resolvedMenuActionValue(value string, actionID int64, modelName string, actionXMLIDs map[string]map[int64]string) string {
	if modelName == "" || actionID == 0 {
		return strings.TrimSpace(value)
	}
	if xmlID := actionXMLIDs[modelName][actionID]; xmlID != "" {
		return modelName + "," + xmlID
	}
	return modelName + "," + strconv.FormatInt(actionID, 10)
}

func externalIDByActionRecord(externalIDs map[string]data.ExternalID) map[string]map[int64]string {
	out := map[string]map[int64]string{}
	names := make([]string, 0, len(externalIDs))
	for name := range externalIDs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		external := externalIDs[name]
		if !isConcreteActionModelName(external.Model) || external.ResID == 0 {
			continue
		}
		if out[external.Model] == nil {
			out[external.Model] = map[int64]string{}
		}
		if out[external.Model][external.ResID] == "" {
			out[external.Model][external.ResID] = name
		}
	}
	return out
}

func externalIDModules(externalIDs map[string]data.ExternalID, modelName string) map[int64]string {
	out := map[int64]string{}
	for _, external := range externalIDs {
		if external.Model == modelName && external.ResID != 0 {
			out[external.ResID] = external.Module
		}
	}
	return out
}

func externalIDByRecord(externalIDs map[string]data.ExternalID, modelName string) map[int64]string {
	out := map[int64]string{}
	names := make([]string, 0, len(externalIDs))
	for name := range externalIDs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		external := externalIDs[name]
		if external.Model == modelName && external.ResID != 0 && out[external.ResID] == "" {
			out[external.ResID] = name
		}
	}
	return out
}

func isConcreteActionModelName(modelName string) bool {
	switch modelName {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return true
	default:
		return false
	}
}

func inferViewType(arch string) view.Type {
	trimmed := strings.TrimSpace(arch)
	switch {
	case strings.HasPrefix(trimmed, "<list"), strings.HasPrefix(trimmed, "<tree"):
		return view.List
	case strings.HasPrefix(trimmed, "<search"):
		return view.Search
	case strings.HasPrefix(trimmed, "<kanban"):
		return view.Kanban
	default:
		return view.Form
	}
}

func impersonationFromExternalIDs(externalIDs map[string]data.ExternalID) *impersonation.Service {
	svc := oi_login_as.NewService()
	rootID := externalIDs["base.user_root"].ResID
	if rootID == 0 {
		rootID = 1
	}
	adminID := externalIDs["base.user_admin"].ResID
	if adminID == 0 {
		adminID = rootID
	}
	publicID := externalIDs["base.public_user"].ResID
	svc.SetUser(impersonation.User{ID: rootID, Login: "__system__", Name: "System", Active: true, Superuser: true, GroupIDs: []int64{oi_login_as.GroupLoginAsAdmin, oi_login_as.GroupLoginAsAllowSuperuser, oi_login_as.GroupLoginAsDebug}})
	svc.SetUser(impersonation.User{ID: adminID, Login: "admin", Name: "Administrator", Active: true, GroupIDs: []int64{oi_login_as.GroupLoginAsAdmin}})
	if publicID != 0 {
		svc.SetUser(impersonation.User{ID: publicID, Login: "public", Name: "Public User", Active: true, Portal: true, GroupIDs: []int64{oi_login_as.GroupLoginAsUser}})
	}
	return svc
}

func loginAsGroupExternalID(groupID int64) string {
	switch groupID {
	case oi_login_as.GroupLoginAsUser:
		return "group_login_as_user"
	case oi_login_as.GroupLoginAsAdmin:
		return "group_login_as_admin"
	case oi_login_as.GroupLoginAsAllowInactive:
		return "group_login_as_allow_inactive"
	case oi_login_as.GroupLoginAsAllowSuperuser:
		return "group_login_as_allow_superuser"
	case oi_login_as.GroupLoginAsDebug:
		return "group_login_as_debug"
	default:
		return fmt.Sprintf("group_login_as_%d", groupID)
	}
}

func modelExternalID(modelName string) string {
	return "model_" + strings.NewReplacer(".", "_").Replace(modelName)
}

func repoRoot() string {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func modelNamesByID(env *record.Env) (map[int64]string, error) {
	rows, err := allRows(env, "ir.model", "model")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		modelName := strings.TrimSpace(stringValue(row["model"]))
		if id != 0 && modelName != "" {
			out[id] = modelName
		}
	}
	return out, nil
}

type automationMetadata struct {
	ID    int64
	Name  string
	Model string
}

func automationMetadataByID(env *record.Env, modelNames map[int64]string) (map[int64]automationMetadata, error) {
	rows, err := allRows(env, "base.automation", "name", "model_id", "model_name")
	if err != nil {
		if strings.Contains(err.Error(), "unknown model base.automation") {
			return map[int64]automationMetadata{}, nil
		}
		return nil, err
	}
	out := make(map[int64]automationMetadata, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		out[id] = automationMetadata{
			ID:    id,
			Name:  stringValue(row["name"]),
			Model: firstNonEmptyString(stringValue(row["model_name"]), modelNames[int64Value(row["model_id"])]),
		}
	}
	return out, nil
}

type modelMetadata struct {
	ID              int64
	Model           string
	Abstract        bool
	Transient       bool
	IsMailThread    bool
	IsMailActivity  bool
	IsMailBlacklist bool
}

func modelMetadataByName(env *record.Env) (map[string]modelMetadata, error) {
	rows, err := allRows(env, "ir.model", "model", "abstract", "transient", "is_mail_thread", "is_mail_activity", "is_mail_blacklist")
	if err != nil {
		return nil, err
	}
	out := make(map[string]modelMetadata, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		modelName := strings.TrimSpace(stringValue(row["model"]))
		if id == 0 || modelName == "" {
			continue
		}
		out[modelName] = modelMetadata{
			ID:              id,
			Model:           modelName,
			Abstract:        boolValue(row["abstract"]),
			Transient:       boolValue(row["transient"]),
			IsMailThread:    boolValue(row["is_mail_thread"]),
			IsMailActivity:  boolValue(row["is_mail_activity"]),
			IsMailBlacklist: boolValue(row["is_mail_blacklist"]),
		}
	}
	return out, nil
}

type mailTemplateMetadata struct {
	ID    int64
	Model string
}

func mailTemplateMetadataByID(env *record.Env, modelNames map[int64]string) (map[int64]mailTemplateMetadata, error) {
	rows, err := allRows(env, "mail.template", "model", "model_id")
	if err != nil {
		if strings.Contains(err.Error(), "unknown model mail.template") {
			return map[int64]mailTemplateMetadata{}, nil
		}
		return nil, err
	}
	out := make(map[int64]mailTemplateMetadata, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		out[id] = mailTemplateMetadata{
			ID:    id,
			Model: firstNonEmptyString(stringValue(row["model"]), modelNames[int64Value(row["model_id"])]),
		}
	}
	return out, nil
}

type modelFieldMetadata struct {
	Model         string
	Name          string
	Type          string
	Relation      string
	RelationField string
	Groups        string
}

func fieldMetadataByID(env *record.Env) (map[int64]modelFieldMetadata, error) {
	rows, err := allRows(env, "ir.model.fields", "model", "name", "ttype", "relation", "relation_field", "groups")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]modelFieldMetadata, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		out[id] = modelFieldMetadata{
			Model:         stringValue(row["model"]),
			Name:          stringValue(row["name"]),
			Type:          stringValue(row["ttype"]),
			Relation:      stringValue(row["relation"]),
			RelationField: stringValue(row["relation_field"]),
			Groups:        stringValue(row["groups"]),
		}
	}
	return out, nil
}

func actionUpdateFieldType(action serveractions.ServerAction, fieldMeta map[int64]modelFieldMetadata) string {
	if meta := modelFieldMetadataByName(fieldMeta, action.Model, firstUpdatePathSegment(action.UpdatePath)); meta.Type != "" {
		return meta.Type
	}
	if meta := fieldMeta[action.UpdateFieldID]; meta.Type != "" {
		return meta.Type
	}
	return strings.TrimSpace(action.UpdateFieldType)
}

func modelFieldMetadataByName(fieldMeta map[int64]modelFieldMetadata, modelName string, fieldName string) modelFieldMetadata {
	if modelName == "" || fieldName == "" {
		return modelFieldMetadata{}
	}
	for _, meta := range fieldMeta {
		if meta.Model == modelName && meta.Name == fieldName {
			return meta
		}
	}
	return modelFieldMetadata{}
}

func firstUpdatePathSegment(path string) string {
	parts := splitFieldPath(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func int64Slice(value any) []int64 {
	switch v := value.(type) {
	case []int64:
		return append([]int64(nil), v...)
	case []any:
		out := make([]int64, 0, len(v))
		for _, item := range v {
			if id := int64Value(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	default:
		if id := int64Value(value); id != 0 {
			return []int64{id}
		}
		return nil
	}
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y":
			return true
		}
	}
	return false
}

func jsonMap(value any) map[string]any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return nil
		}
		return out
	default:
		return nil
	}
}

func intWithFallback(value any, fallback int) int {
	if id := int64Value(value); id != 0 {
		return int(id)
	}
	return fallback
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolWithFallback(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case nil:
		return fallback
	}
	return fallback
}

func runtimeModelHasField(env *record.Env, modelName string, fieldName string) bool {
	if env == nil {
		return false
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return false
	}
	_, ok = meta.Fields[fieldName]
	return ok
}

func runtimeRecordExists(env *record.Env, modelName string, id int64) bool {
	if env == nil || id == 0 {
		return false
	}
	rows, err := env.Model(modelName).Browse(id).Read("id")
	return err == nil && len(rows) != 0
}

func runtimeConfigParameter(env *record.Env, key string) string {
	if env == nil || !runtimeModelHasField(env, "ir.config_parameter", "value") {
		return ""
	}
	found, err := env.Model("ir.config_parameter").SearchWithOptions(domain.Cond("key", domain.Equal, key), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return ""
	}
	rows, err := found.Read("value")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringValue(rows[0]["value"]))
}

func runtimeBaseURL(env *record.Env) string {
	baseURL := strings.TrimRight(runtimeConfigParameter(env, "web.base.url"), "/")
	if baseURL == "" {
		return "http://localhost"
	}
	return baseURL
}

func runtimeSMSUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x", b[:])
}

func randomSMSCode() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%03d", time.Now().UnixNano()%1000)
	}
	out := make([]byte, len(b))
	for i, value := range b {
		out[i] = alphabet[int(value)%len(alphabet)]
	}
	return string(out)
}

func smsTrackingURLSkipped(rawURL string, baseURL string) bool {
	return strings.HasPrefix(rawURL, strings.TrimRight(baseURL, "/")+"/sms/")
}

func appendSMSTrackingSuffix(rawURL string, smsID int64, baseURL string) (string, bool) {
	prefix := strings.TrimRight(baseURL, "/") + "/r/"
	if !strings.HasPrefix(rawURL, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(rawURL, prefix)
	if rest == "" {
		return rawURL, true
	}
	pathEnd := len(rest)
	for _, sep := range []string{"?", "#"} {
		if index := strings.Index(rest, sep); index >= 0 && index < pathEnd {
			pathEnd = index
		}
	}
	codePath := rest[:pathEnd]
	tail := rest[pathEnd:]
	if codePath == "" || strings.Contains(codePath, "/") {
		return rawURL, true
	}
	return prefix + codePath + fmt.Sprintf("/s/%d", smsID) + tail, true
}
