package lifecycle

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/module"
	"gorp/internal/record"
)

type Service struct {
	Env                   *record.Env
	Manifests             map[string]module.Manifest
	CheckPythonDependency func(string) error
	CheckBinaryDependency func(string) error
}

type Result struct {
	Operation string   `json:"operation"`
	Modules   []string `json:"modules"`
}

type UpdateListResult struct {
	Updated int
	Added   int
	Modules []string
}

func New(env *record.Env, manifests map[string]module.Manifest) Service {
	return Service{Env: env, Manifests: manifests}
}

func (s Service) UpdateList() (UpdateListResult, error) {
	if s.Env == nil {
		return UpdateListResult{}, fmt.Errorf("module lifecycle requires env")
	}
	rows, err := s.moduleRowsByName()
	if err != nil {
		return UpdateListResult{}, err
	}
	result := UpdateListResult{}
	names := make([]string, 0, len(s.Manifests))
	for name := range s.Manifests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		manifest := s.Manifests[name]
		row, exists := rows[name]
		rowChanged := false
		added := false
		if !exists {
			state := "uninstalled"
			if !manifest.Installable {
				state = "uninstallable"
			}
			id, err := s.Env.Model("ir.module.module").Create(map[string]any{"name": name, "state": state})
			if err != nil {
				return UpdateListResult{}, err
			}
			row = moduleRow{ID: id, Name: name, State: state}
			rows[name] = row
			result.Added++
			added = true
		} else if manifest.Installable && row.State == "uninstallable" {
			if err := s.writeRows([]moduleRow{row}, "uninstalled"); err != nil {
				return UpdateListResult{}, err
			}
			row.State = "uninstalled"
			rows[name] = row
			rowChanged = true
		}
		depsChanged, err := s.syncModuleDependencies(row.ID, manifest)
		if err != nil {
			return UpdateListResult{}, err
		}
		exclusionsChanged, err := s.syncModuleExclusions(row.ID, manifest.Excludes)
		if err != nil {
			return UpdateListResult{}, err
		}
		if !added && (rowChanged || depsChanged || exclusionsChanged) {
			result.Updated++
		}
		if added || rowChanged || depsChanged || exclusionsChanged {
			result.Modules = append(result.Modules, name)
		}
	}
	sort.Strings(result.Modules)
	return result, nil
}

func (s Service) ButtonImmediateInstall(ids []int64) (Result, error) {
	return s.install(ids, true)
}

func (s Service) ButtonInstall(ids []int64) (Result, error) {
	return s.install(ids, false)
}

func (s Service) ButtonImmediateUpgrade(ids []int64) (Result, error) {
	return s.upgrade(ids, true)
}

func (s Service) ButtonUpgrade(ids []int64) (Result, error) {
	return s.upgrade(ids, false)
}

func (s Service) ButtonUninstall(ids []int64) (Result, error) {
	return s.uninstall(ids, false)
}

func (s Service) ButtonImmediateUninstall(ids []int64) (Result, error) {
	return s.uninstall(ids, true)
}

func (s Service) ButtonCancelInstall(ids []int64) (Result, error) {
	return s.markIfStates(ids, "uninstalled", "cancel_install", map[string]bool{"to install": true, "uninstalled": true})
}

func (s Service) ButtonCancelUninstall(ids []int64) (Result, error) {
	return s.markIfStates(ids, "installed", "cancel_uninstall", map[string]bool{"to remove": true, "installed": true})
}

func (s Service) ButtonCancelUpgrade(ids []int64) (Result, error) {
	return s.markIfStates(ids, "installed", "cancel_upgrade", map[string]bool{"to upgrade": true, "installed": true})
}

func (s Service) install(ids []int64, immediate bool) (Result, error) {
	rows, err := s.rowsByID(ids)
	if err != nil {
		return Result{}, err
	}
	operation := "install"
	targetState := "to install"
	if immediate {
		operation = "immediate_install"
		targetState = "installed"
	}
	plan := map[string]bool{}
	visiting := map[string]bool{}
	for _, row := range rows {
		if err := s.collectInstallName(row.Name, visiting, plan); err != nil {
			return Result{}, err
		}
	}
	if err := s.collectAutoInstallNames(targetState, plan); err != nil {
		return Result{}, err
	}
	if err := s.checkInstallExclusions(targetState, plan); err != nil {
		return Result{}, err
	}
	if err := s.checkExternalDependencies(plan, true); err != nil {
		return Result{}, err
	}
	changed, err := s.writeInstallPlan(targetState, plan)
	if err != nil {
		return Result{}, err
	}
	return Result{Operation: operation, Modules: sortedKeys(changed)}, nil
}

func (s Service) uninstall(ids []int64, immediate bool) (Result, error) {
	rows, err := s.rowsByID(ids)
	if err != nil {
		return Result{}, err
	}
	targets := map[string]bool{}
	for _, row := range rows {
		targets[row.Name] = true
		if row.Name == "base" {
			return Result{}, fmt.Errorf("module base cannot be uninstalled")
		}
		if row.State != "installed" && row.State != "to upgrade" {
			return Result{}, fmt.Errorf("module %s cannot run uninstall from state %s", row.Name, row.State)
		}
	}
	active, err := s.activeDependentModules()
	if err != nil {
		return Result{}, err
	}
	for name := range targets {
		for dependent, manifest := range s.Manifests {
			state, activeDependent := active[dependent]
			if targets[dependent] || !activeDependent {
				continue
			}
			for _, dep := range manifest.Depends {
				if dep == name {
					if state == "installed" {
						return Result{}, fmt.Errorf("module %s is required by installed module %s", name, dependent)
					}
					return Result{}, fmt.Errorf("module %s is required by module %s in state %s", name, dependent, state)
				}
			}
		}
	}
	targetState := "to remove"
	operation := "uninstall"
	if immediate {
		targetState = "uninstalled"
		operation = "immediate_uninstall"
	}
	if err := s.writeRows(rows, targetState); err != nil {
		return Result{}, err
	}
	return Result{Operation: operation, Modules: rowNames(rows)}, nil
}

func (s Service) upgrade(ids []int64, immediate bool) (Result, error) {
	targetState := "to upgrade"
	operation := "upgrade"
	if immediate {
		targetState = "installed"
		operation = "immediate_upgrade"
	}
	rows, err := s.rowsByID(ids)
	if err != nil {
		return Result{}, err
	}
	for _, row := range rows {
		if row.State != "installed" && row.State != "to upgrade" {
			return Result{}, fmt.Errorf("module %s cannot run %s from state %s", row.Name, operation, row.State)
		}
	}
	plan, err := s.upgradePlan(rows)
	if err != nil {
		return Result{}, err
	}
	if err := s.checkExternalDependencies(plan, false); err != nil {
		return Result{}, err
	}
	changed := map[string]bool{}
	for _, name := range sortedKeys(plan) {
		row, err := s.ensureRow(name)
		if err != nil {
			return Result{}, err
		}
		if row.State == targetState {
			continue
		}
		if err := s.writeRows([]moduleRow{row}, targetState); err != nil {
			return Result{}, err
		}
		changed[row.Name] = true
	}
	if immediate {
		return Result{Operation: operation, Modules: sortedKeys(plan)}, nil
	}
	return Result{Operation: operation, Modules: sortedKeys(changed)}, nil
}

func (s Service) upgradePlan(rows []moduleRow) (map[string]bool, error) {
	states, err := s.moduleStates()
	if err != nil {
		return nil, err
	}
	plan := map[string]bool{}
	for _, row := range rows {
		plan[row.Name] = true
	}
	if plan["base"] {
		for name, state := range states {
			if state == "installed" && name != "studio_customization" {
				if _, ok := s.Manifests[name]; ok {
					plan[name] = true
				}
			}
		}
	}
	for {
		added := false
		for name, state := range states {
			if plan[name] || state != "installed" || name == "studio_customization" {
				continue
			}
			manifest, ok := s.Manifests[name]
			if !ok {
				continue
			}
			for _, dep := range manifest.Depends {
				if plan[dep] {
					plan[name] = true
					added = true
					break
				}
			}
		}
		if !added {
			return plan, nil
		}
	}
}

func (s Service) markIfStates(ids []int64, state string, operation string, allowed map[string]bool) (Result, error) {
	rows, err := s.rowsByID(ids)
	if err != nil {
		return Result{}, err
	}
	for _, row := range rows {
		if !allowed[row.State] {
			return Result{}, fmt.Errorf("module %s cannot run %s from state %s", row.Name, operation, row.State)
		}
	}
	changed := map[string]bool{}
	for _, row := range rows {
		if row.State == state {
			continue
		}
		if err := s.writeRows([]moduleRow{row}, state); err != nil {
			return Result{}, err
		}
		changed[row.Name] = true
	}
	return Result{Operation: operation, Modules: sortedKeys(changed)}, nil
}

func (s Service) collectInstallName(name string, visiting map[string]bool, plan map[string]bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("module name is required")
	}
	if plan[name] {
		return nil
	}
	if visiting[name] {
		return fmt.Errorf("module dependency cycle at %s", name)
	}
	manifest, ok := s.Manifests[name]
	if !ok {
		return fmt.Errorf("unknown module %s", name)
	}
	if !manifest.Installable {
		return fmt.Errorf("module %s is not installable", name)
	}
	visiting[name] = true
	deps := append([]string(nil), manifest.Depends...)
	sort.Strings(deps)
	for _, dep := range deps {
		if err := s.collectInstallName(dep, visiting, plan); err != nil {
			return err
		}
	}
	delete(visiting, name)
	plan[name] = true
	return nil
}

func (s Service) collectAutoInstallNames(targetState string, plan map[string]bool) error {
	for {
		states, err := s.plannedStates(targetState, plan)
		if err != nil {
			return err
		}
		added := false
		names := make([]string, 0, len(s.Manifests))
		for name := range s.Manifests {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			manifest := s.Manifests[name]
			if !manifest.AutoInstall || plan[name] || isActiveState(states[name]) {
				continue
			}
			if !autoInstallEligible(manifest, states) {
				continue
			}
			if err := s.collectInstallName(name, map[string]bool{}, plan); err != nil {
				return err
			}
			added = true
		}
		if !added {
			return nil
		}
	}
}

func autoInstallEligible(manifest module.Manifest, states map[string]string) bool {
	deps := manifest.AutoInstallDepends
	if len(deps) == 0 {
		deps = manifest.Depends
	}
	if len(deps) == 0 {
		return false
	}
	for _, dep := range deps {
		if !isActiveState(states[dep]) {
			return false
		}
	}
	return true
}

func (s Service) checkInstallExclusions(targetState string, plan map[string]bool) error {
	states, err := s.plannedStates(targetState, plan)
	if err != nil {
		return err
	}
	exclusions, err := s.exclusions()
	if err != nil {
		return err
	}
	for _, exclusion := range exclusions {
		if isActiveState(states[exclusion.Module]) && isActiveState(states[exclusion.Excluded]) {
			return fmt.Errorf("module %s excludes module %s", exclusion.Module, exclusion.Excluded)
		}
	}
	return nil
}

func (s Service) checkExternalDependencies(plan map[string]bool, skipInstalled bool) error {
	states, err := s.moduleStates()
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(plan) {
		if skipInstalled && states[name] == "installed" {
			continue
		}
		manifest, ok := s.Manifests[name]
		if !ok {
			return fmt.Errorf("unknown module %s", name)
		}
		for _, pydep := range normalizedExternalDependencyList(manifest.ExternalDependencies["python"]) {
			if err := s.checkPythonDependency(pydep); err != nil {
				return fmt.Errorf("module %s missing external python dependency %s: %w", name, pydep, err)
			}
		}
		for _, binary := range normalizedExternalDependencyList(manifest.ExternalDependencies["bin"]) {
			if err := s.checkBinaryDependency(binary); err != nil {
				return fmt.Errorf("module %s missing external binary dependency %s: %w", name, binary, err)
			}
		}
	}
	return nil
}

func (s Service) checkPythonDependency(name string) error {
	if s.CheckPythonDependency != nil {
		return s.CheckPythonDependency(name)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		return err
	}
	distribution := pythonDistributionName(name)
	if distribution == "" {
		distribution = name
	}
	return exec.Command(python, "-c", "import importlib, importlib.metadata as metadata, sys\ntry:\n    metadata.distribution(sys.argv[1])\nexcept metadata.PackageNotFoundError:\n    importlib.import_module(sys.argv[2])", distribution, distribution).Run()
}

func pythonDistributionName(value string) string {
	value = strings.TrimSpace(value)
	if cut, _, ok := strings.Cut(value, ";"); ok {
		value = cut
	}
	for i, ch := range value {
		if strings.ContainsRune("<>!=~[ \t", ch) {
			return strings.TrimSpace(value[:i])
		}
	}
	return value
}

func (s Service) checkBinaryDependency(name string) error {
	if s.CheckBinaryDependency != nil {
		return s.CheckBinaryDependency(name)
	}
	_, err := exec.LookPath(name)
	return err
}

func normalizedExternalDependencyList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s Service) writeInstallPlan(targetState string, plan map[string]bool) (map[string]bool, error) {
	changed := map[string]bool{}
	names := sortedKeys(plan)
	for _, name := range names {
		row, err := s.ensureRow(name)
		if err != nil {
			return nil, err
		}
		if targetState == "to install" && row.State == "installed" {
			continue
		}
		if row.State == targetState {
			continue
		}
		if err := s.writeRows([]moduleRow{row}, targetState); err != nil {
			return nil, err
		}
		changed[name] = true
	}
	return changed, nil
}

type moduleRow struct {
	ID    int64
	Name  string
	State string
}

func (s Service) rowsByID(ids []int64) ([]moduleRow, error) {
	if s.Env == nil {
		return nil, fmt.Errorf("module lifecycle requires env")
	}
	clean := positiveIDs(ids)
	if len(clean) == 0 {
		return nil, fmt.Errorf("module lifecycle requires module ids")
	}
	rows, err := s.Env.Model("ir.module.module").Browse(clean...).Read("id", "name", "state")
	if err != nil {
		return nil, err
	}
	out := make([]moduleRow, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		if name == "" {
			return nil, fmt.Errorf("module row %d has no name", int64Value(row["id"]))
		}
		out = append(out, moduleRow{ID: int64Value(row["id"]), Name: name, State: strings.TrimSpace(fmt.Sprint(row["state"]))})
	}
	if len(out) != len(clean) {
		return nil, fmt.Errorf("module ids not found")
	}
	return out, nil
}

func (s Service) ensureRow(name string) (moduleRow, error) {
	row, ok, err := s.rowByName(name)
	if err != nil {
		return moduleRow{}, err
	}
	if ok {
		return row, nil
	}
	id, err := s.Env.Model("ir.module.module").Create(map[string]any{"name": name, "state": "uninstalled"})
	if err != nil {
		return moduleRow{}, err
	}
	return moduleRow{ID: id, Name: name, State: "uninstalled"}, nil
}

func (s Service) rowByName(name string) (moduleRow, bool, error) {
	found, err := s.Env.Model("ir.module.module").Search(domain.Cond("name", "=", name))
	if err != nil {
		return moduleRow{}, false, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return moduleRow{}, false, nil
	}
	rows, err := s.Env.Model("ir.module.module").Browse(ids[0]).Read("id", "name", "state")
	if err != nil {
		return moduleRow{}, false, err
	}
	if len(rows) == 0 {
		return moduleRow{}, false, nil
	}
	row := rows[0]
	return moduleRow{ID: int64Value(row["id"]), Name: strings.TrimSpace(fmt.Sprint(row["name"])), State: strings.TrimSpace(fmt.Sprint(row["state"]))}, true, nil
}

func (s Service) moduleRowsByName() (map[string]moduleRow, error) {
	found, err := s.Env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "name", "state")
	if err != nil {
		return nil, err
	}
	out := map[string]moduleRow{}
	for _, row := range rows {
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		if name == "" {
			continue
		}
		out[name] = moduleRow{ID: int64Value(row["id"]), Name: name, State: strings.TrimSpace(fmt.Sprint(row["state"]))}
	}
	return out, nil
}

func (s Service) syncModuleDependencies(moduleID int64, manifest module.Manifest) (bool, error) {
	required := autoInstallRequiredDependencies(manifest)
	names := normalizedExternalDependencyList(manifest.Depends)
	extras := map[string]map[string]any{}
	for _, name := range names {
		extras[name] = map[string]any{"auto_install_required": required[name]}
	}
	return s.syncModuleNamedRows("ir.module.module.dependency", moduleID, names, extras)
}

func autoInstallRequiredDependencies(manifest module.Manifest) map[string]bool {
	required := map[string]bool{}
	if !manifest.AutoInstall {
		return required
	}
	deps := manifest.AutoInstallDepends
	if len(deps) == 0 {
		deps = manifest.Depends
	}
	for _, dep := range deps {
		dep = strings.TrimSpace(dep)
		if dep != "" {
			required[dep] = true
		}
	}
	return required
}

func (s Service) syncModuleExclusions(moduleID int64, excludes []string) (bool, error) {
	return s.syncModuleNamedRows("ir.module.module.exclusion", moduleID, normalizedExternalDependencyList(excludes), nil)
}

func (s Service) syncModuleNamedRows(modelName string, moduleID int64, names []string, extras map[string]map[string]any) (bool, error) {
	found, err := s.Env.Model(modelName).Search(domain.Cond("module_id", domain.Equal, moduleID))
	if err != nil {
		return false, err
	}
	fields := []string{"id", "module_id", "name"}
	extraFields := map[string]bool{}
	for _, values := range extras {
		for key := range values {
			extraFields[key] = true
		}
	}
	for key := range extraFields {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	rows, err := found.Read(fields...)
	if err != nil {
		return false, err
	}
	want := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			want[name] = true
		}
	}
	seen := map[string]int64{}
	changed := false
	for _, row := range rows {
		id := int64Value(row["id"])
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		if id == 0 || name == "" {
			continue
		}
		if !want[name] {
			if err := s.Env.Model(modelName).Browse(id).Unlink(); err != nil {
				return false, err
			}
			changed = true
			continue
		}
		seen[name] = id
		values := map[string]any{"name": name, "module_id": moduleID}
		for key, value := range extras[name] {
			values[key] = value
		}
		if len(values) > 2 && rowValuesDiffer(row, values) {
			if err := s.Env.Model(modelName).Browse(id).Write(values); err != nil {
				return false, err
			}
			changed = true
		}
	}
	for _, name := range names {
		if seen[name] > 0 {
			continue
		}
		values := map[string]any{"name": name, "module_id": moduleID}
		for key, value := range extras[name] {
			values[key] = value
		}
		if _, err := s.Env.Model(modelName).Create(values); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func rowValuesDiffer(row map[string]any, values map[string]any) bool {
	for key, want := range values {
		switch wantValue := want.(type) {
		case bool:
			if boolValue(row[key]) != wantValue {
				return true
			}
		default:
			if fmt.Sprint(row[key]) != fmt.Sprint(want) {
				return true
			}
		}
	}
	return false
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func (s Service) activeDependentModules() (map[string]string, error) {
	states, err := s.moduleStates()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, state := range states {
		if isActiveState(state) {
			out[name] = state
		}
	}
	return out, nil
}

func (s Service) plannedStates(targetState string, plan map[string]bool) (map[string]string, error) {
	states, err := s.moduleStates()
	if err != nil {
		return nil, err
	}
	for name := range plan {
		if targetState == "to install" && states[name] == "installed" {
			continue
		}
		states[name] = targetState
	}
	return states, nil
}

func (s Service) moduleStates() (map[string]string, error) {
	found, err := s.Env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "state")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, row := range rows {
		state := strings.TrimSpace(fmt.Sprint(row["state"]))
		if state == "" {
			state = "uninstalled"
		}
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		if name != "" {
			out[name] = state
		}
	}
	return out, nil
}

type moduleExclusion struct {
	Module   string
	Excluded string
}

func (s Service) exclusions() ([]moduleExclusion, error) {
	found, err := s.Env.Model("ir.module.module.exclusion").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("module_id", "name")
	if err != nil {
		return nil, err
	}
	idToName, err := s.moduleNamesByID()
	if err != nil {
		return nil, err
	}
	out := make([]moduleExclusion, 0, len(rows))
	for _, row := range rows {
		moduleName := idToName[int64Value(row["module_id"])]
		excluded := strings.TrimSpace(fmt.Sprint(row["name"]))
		if moduleName == "" || excluded == "" {
			continue
		}
		out = append(out, moduleExclusion{Module: moduleName, Excluded: excluded})
	}
	return out, nil
}

func (s Service) moduleNamesByID() (map[int64]string, error) {
	found, err := s.Env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "name")
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for _, row := range rows {
		id := int64Value(row["id"])
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		if id > 0 && name != "" {
			out[id] = name
		}
	}
	return out, nil
}

func isActiveState(state string) bool {
	return state == "installed" || state == "to install" || state == "to upgrade"
}

func (s Service) writeRows(rows []moduleRow, state string) error {
	for _, row := range rows {
		if err := s.Env.Model("ir.module.module").Browse(row.ID).Write(map[string]any{"state": state}); err != nil {
			return err
		}
	}
	return nil
}

func positiveIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func rowNames(rows []moduleRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case jsonNumber:
		n, _ := v.Int64()
		return n
	default:
		n, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(v)), 10, 64)
		return n
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}
