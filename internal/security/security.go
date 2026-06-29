package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/record"
)

var ErrAccessDenied = errors.New("access denied")

const noAccessGroupID int64 = -1

var bareDomainIdentifierRE = regexp.MustCompile(`\b(?:user(?:\.[A-Za-z_][A-Za-z0-9_]*)+|company_ids|company_id)\b`)

type User struct {
	ID                  int64
	Login               string
	Password            string
	Email               string
	Name                string
	Active              bool
	CompanyID           int64
	CompanyIDs          []int64
	PartnerID           int64
	CommercialPartnerID int64
	GroupIDs            []int64
}

type Group struct {
	ID                      int64
	Name                    string
	CategoryID              int64
	ImpliedIDs              []int64
	AllowDelegation         bool
	AllowMultipleDelegation bool
	RestrictedAccess        bool
}

type Company struct {
	ID        int64
	Name      string
	ParentID  int64
	CountryID int64
	Active    bool
}

type ACL struct {
	Name       string
	Model      string
	ModelID    int64
	GroupID    int64
	Active     bool
	PermRead   bool
	PermWrite  bool
	PermCreate bool
	PermUnlink bool
}

type Rule struct {
	Name       string
	Model      string
	ModelID    int64
	Domain     domain.Node
	DomainText string
	GroupIDs   []int64
	Global     bool
	PermRead   bool
	PermWrite  bool
	PermCreate bool
	PermUnlink bool
	Active     bool
}

type AuditEvent struct {
	At        time.Time
	ActorID   int64
	Action    string
	Model     string
	RecordID  int64
	IPAddress string
	UserAgent string
	Details   map[string]string
}

type DelegationProvider interface {
	EffectiveGroupIDs(userID int64, directGroupIDs []int64, at time.Time) []int64
}

type DelegationRuleDomainProvider interface {
	RuleDomains(userID int64, directGroupIDs []int64, modelName string, operation string, at time.Time) ([]delegation.RuleDomain, error)
}

type DelegationLineProvider interface {
	ActiveLines(userID int64, at time.Time) []delegation.Line
}

type RecordDepartmentResolver func(modelName string, row map[string]any) ([]int64, bool)

type DepartmentDescendantResolver func(departmentIDs []int64) []int64

type Engine struct {
	Users                 map[int64]User
	Groups                map[int64]Group
	Companies             map[int64]Company
	Hierarchies           map[string]map[int64]int64
	ACLs                  []ACL
	Rules                 []Rule
	FieldGroups           map[string]map[string][]int64
	Sessions              map[string]Token
	APITokens             map[string]Token
	Audit                 []AuditEvent
	sessionMu             sync.RWMutex
	Delegations           DelegationProvider
	RecordDepartments     RecordDepartmentResolver
	DepartmentDescendants DepartmentDescendantResolver
	now                   func() time.Time
}

type Token struct {
	UserID     int64
	Hash       string
	ExpiresAt  time.Time
	LastUsedAt time.Time
	RevokedAt  time.Time
	Scopes     []string
	Name       string
	CompanyID  int64
	CompanyIDs []int64
}

func NewEngine() *Engine {
	return &Engine{
		Users:       map[int64]User{},
		Groups:      map[int64]Group{},
		Companies:   map[int64]Company{},
		Hierarchies: map[string]map[int64]int64{},
		FieldGroups: map[string]map[string][]int64{},
		Sessions:    map[string]Token{},
		APITokens:   map[string]Token{},
		now:         time.Now,
	}
}

func (e *Engine) SetNow(now func() time.Time) {
	if now != nil {
		e.now = now
	}
}

func (e *Engine) SetDelegationProvider(provider DelegationProvider) {
	e.Delegations = provider
}

func (e *Engine) SetDepartmentResolvers(recordDepartments RecordDepartmentResolver, departmentDescendants DepartmentDescendantResolver) {
	e.RecordDepartments = recordDepartments
	e.DepartmentDescendants = departmentDescendants
}

func (e *Engine) InvalidateDelegationCache([]int64) {}

func (e *Engine) EffectiveGroupIDs(userID int64) map[int64]bool {
	user := e.Users[userID]
	groupIDs := append([]int64(nil), user.GroupIDs...)
	if e.Delegations != nil {
		groupIDs = e.Delegations.EffectiveGroupIDs(userID, groupIDs, e.now())
	}
	return e.groupClosure(groupIDs)
}

func (e *Engine) directEffectiveGroupIDs(userID int64) map[int64]bool {
	user := e.Users[userID]
	return e.groupClosure(user.GroupIDs)
}

func (e *Engine) groupClosure(groupIDs []int64) map[int64]bool {
	effective := map[int64]bool{}
	var visit func(int64)
	visit = func(id int64) {
		if effective[id] {
			return
		}
		effective[id] = true
		for _, implied := range e.Groups[id].ImpliedIDs {
			visit(implied)
		}
	}
	for _, id := range groupIDs {
		visit(id)
	}
	return effective
}

func (e *Engine) Check(ctx record.Context, modelName string, op record.Operation, values map[string]any) error {
	if ctx.UserID == 1 || ctx.Sudo {
		return nil
	}
	user, ok := e.Users[ctx.UserID]
	if !ok || !user.Active {
		e.Log(AuditEvent{ActorID: ctx.UserID, Action: "permission_denied", Model: modelName})
		return ErrAccessDenied
	}
	if !e.AllowedByACL(user.ID, modelName, op) {
		e.Log(AuditEvent{ActorID: user.ID, Action: "permission_denied", Model: modelName})
		return ErrAccessDenied
	}
	if err := e.checkFieldGroups(user.ID, modelName, values); err != nil {
		e.Log(AuditEvent{ActorID: user.ID, Action: "permission_denied", Model: modelName})
		return err
	}
	return nil
}

func (e *Engine) FilterFields(ctx record.Context, modelName string, fields []string) []string {
	if len(fields) == 0 {
		return fields
	}
	if ctx.UserID == 1 || ctx.Sudo {
		return fields
	}
	allowed := fields[:0]
	for _, field := range fields {
		groups := e.FieldGroups[modelName][field]
		if len(groups) == 0 || e.hasAnyGroup(ctx.UserID, groups) {
			allowed = append(allowed, field)
		}
	}
	return allowed
}

func (e *Engine) CheckRecord(ctx record.Context, modelName string, op record.Operation, row map[string]any) (bool, error) {
	if ctx.UserID == 1 || ctx.Sudo {
		return true, nil
	}
	return e.AllowedByRecordRules(ctx.UserID, modelName, op, row)
}

func (e *Engine) AllowedByACL(userID int64, modelName string, op record.Operation) bool {
	if userID == 1 {
		return true
	}
	user, ok := e.Users[userID]
	if !ok || !user.Active {
		return false
	}
	groups := e.EffectiveGroupIDs(userID)
	for _, acl := range e.ACLs {
		if acl.Model != modelName {
			continue
		}
		if acl.GroupID != 0 && !groups[acl.GroupID] {
			continue
		}
		if aclAllows(acl, op) {
			return true
		}
	}
	return false
}

func (e *Engine) AllowedByRecordRules(userID int64, modelName string, op record.Operation, row map[string]any) (bool, error) {
	user, ok := e.Users[userID]
	if !ok || !user.Active {
		return false, ErrAccessDenied
	}
	groups := e.directEffectiveGroupIDs(userID)
	for _, rule := range e.Rules {
		if !rule.Active || rule.Model != modelName || !ruleApplies(rule, op) || !ruleGlobal(rule) {
			continue
		}
		ok, err := e.evalDomain(modelName, row, user, rule.Domain)
		if err != nil || !ok {
			return ok, err
		}
	}

	groupMatched := false
	groupAllowed := false
	for _, rule := range e.Rules {
		if !rule.Active || rule.Model != modelName || !ruleApplies(rule, op) || ruleGlobal(rule) || !intersects(groups, rule.GroupIDs) {
			continue
		}
		groupMatched = true
		ok, err := e.evalDomain(modelName, row, user, rule.Domain)
		if err != nil {
			return false, err
		}
		groupAllowed = groupAllowed || ok
	}
	delegatedMatched, delegatedAllowed, err := e.allowedByDelegatedRecordRules(user, modelName, op, row)
	if err != nil {
		return false, err
	}
	if groupMatched || delegatedMatched {
		return groupAllowed || delegatedAllowed, nil
	}
	return true, nil
}

func (e *Engine) allowedByDelegatedRecordRules(user User, modelName string, op record.Operation, row map[string]any) (bool, bool, error) {
	if e.Delegations == nil {
		return false, false, nil
	}
	matched := false
	allowed := false
	if provider, ok := e.Delegations.(DelegationRuleDomainProvider); ok {
		domains, err := provider.RuleDomains(user.ID, user.GroupIDs, modelName, string(op), e.now())
		if err != nil {
			return false, false, err
		}
		for _, ruleDomain := range domains {
			if ruleDomain.Model != "" && ruleDomain.Model != modelName {
				continue
			}
			if ruleDomain.Operation != "" && ruleDomain.Operation != string(op) {
				continue
			}
			node, err := ParseDomainForce(ruleDomain.Expression)
			if err != nil {
				return false, false, err
			}
			ok, err := e.evalDelegatedDomain(user, modelName, ruleDomain.DelegatorUserID, ruleDomain.DepartmentIDs, row, node)
			if err != nil {
				return false, false, err
			}
			matched = true
			allowed = allowed || ok
		}
	}
	if provider, ok := e.Delegations.(DelegationLineProvider); ok {
		for _, line := range provider.ActiveLines(user.ID, e.now()) {
			lineGroups := e.groupClosure([]int64{line.GroupID})
			for _, rule := range e.Rules {
				if !rule.Active || rule.Model != modelName || !ruleApplies(rule, op) || ruleGlobal(rule) || !intersects(lineGroups, rule.GroupIDs) {
					continue
				}
				ok, err := e.evalDelegatedDomain(user, modelName, line.DelegatorUserID, line.DepartmentIDs, row, rule.Domain)
				if err != nil {
					return false, false, err
				}
				matched = true
				allowed = allowed || ok
			}
		}
	}
	return matched, allowed, nil
}

func (e *Engine) evalDelegatedDomain(delegate User, modelName string, delegatorID int64, departmentIDs []int64, row map[string]any, node domain.Node) (bool, error) {
	delegator, ok := e.Users[delegatorID]
	if !ok || !delegator.Active {
		return false, nil
	}
	domainUser, ok := delegatedDomainUser(delegate, delegator)
	if !ok {
		return false, nil
	}
	if !rowCompanyInBothUsers(row, delegate, domainUser) {
		return false, nil
	}
	if !e.rowDepartmentAllowed(modelName, row, departmentIDs) {
		return false, nil
	}
	return e.evalDomain(modelName, row, domainUser, node)
}

func EvalDomain(row map[string]any, user User, node domain.Node) (bool, error) {
	return evalDomainWithContext("", row, user, node, nil, nil)
}

func (e *Engine) evalDomain(modelName string, row map[string]any, user User, node domain.Node) (bool, error) {
	return evalDomainWithContext(modelName, row, user, node, e.Companies, e.Hierarchies)
}

func evalDomainWithContext(modelName string, row map[string]any, user User, node domain.Node, companies map[int64]Company, hierarchies map[string]map[int64]int64) (bool, error) {
	switch node.Kind {
	case domain.Literal:
		value, _ := domain.NormalizeScalar(node.Value).(bool)
		return value, nil
	case domain.Condition:
		left := valueForField(row, node.Field)
		right := resolveValue(user, node.Value, companies)
		switch node.Operator {
		case domain.Equal:
			return valuesEqual(left, right), nil
		case domain.NotEqual:
			return !valuesEqual(left, right), nil
		case domain.OptionalEqual:
			if !isTruthy(right) {
				return true, nil
			}
			return valuesEqual(left, right), nil
		case domain.In:
			return valueIn(left, right)
		case domain.NotIn:
			ok, err := valueIn(left, right)
			return !ok, err
		case domain.Less, domain.LessEqual, domain.Greater, domain.GreaterEqual:
			return compare(left, right, node.Operator)
		case domain.Like:
			return containsMatch(left, right, false, false), nil
		case domain.NotLike:
			return !containsMatch(left, right, false, false), nil
		case domain.ILike:
			return containsMatch(left, right, true, false), nil
		case domain.NotILike:
			return !containsMatch(left, right, true, false), nil
		case domain.EqualLike:
			return containsMatch(left, right, false, true), nil
		case domain.NotEqualLike:
			return !containsMatch(left, right, false, true), nil
		case domain.EqualILike:
			return containsMatch(left, right, true, true), nil
		case domain.NotEqualILike:
			return !containsMatch(left, right, true, true), nil
		case domain.ChildOf, domain.ParentOf:
			return hierarchyValueMatch(row, modelName, node.Field, left, right, node.Operator, companies, hierarchies)
		case domain.AnyOf:
			return valueAny(modelName, node.Field, left, right, user, companies, hierarchies)
		case domain.NotAnyOf:
			ok, err := valueAny(modelName, node.Field, left, right, user, companies, hierarchies)
			return !ok, err
		default:
			return false, fmt.Errorf("record rule operator %s not implemented", node.Operator)
		}
	case domain.All:
		for _, child := range node.Children {
			ok, err := evalDomainWithContext(modelName, row, user, child, companies, hierarchies)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	case domain.Any:
		for _, child := range node.Children {
			ok, err := evalDomainWithContext(modelName, row, user, child, companies, hierarchies)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case domain.None:
		if len(node.Children) != 1 {
			return false, fmt.Errorf("not requires one child")
		}
		ok, err := evalDomainWithContext(modelName, row, user, node.Children[0], companies, hierarchies)
		return !ok, err
	default:
		return false, fmt.Errorf("unsupported domain kind %s", node.Kind)
	}
}

func (e *Engine) LoadPersistedSecurity(env *record.Env) error {
	companies, err := loadPersistedCompanies(env)
	if err != nil {
		return err
	}
	if len(companies) > 0 {
		e.Companies = companies
	}
	groups, err := loadPersistedGroups(env)
	if err != nil {
		return err
	}
	if len(groups) > 0 {
		e.Groups = groups
	}
	users, err := loadPersistedUsers(env)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		e.Users = users
	}
	partnerHierarchy, err := loadPersistedHierarchy(env, "res.partner")
	if err != nil {
		return err
	}
	if len(partnerHierarchy) > 0 {
		if e.Hierarchies == nil {
			e.Hierarchies = map[string]map[int64]int64{}
		}
		e.Hierarchies["res.partner"] = partnerHierarchy
	}
	fieldGroups, err := loadPersistedFieldGroups(env)
	if err != nil {
		return err
	}
	if len(fieldGroups) > 0 {
		mergeFieldGroups(e.FieldGroups, fieldGroups)
	}
	acls, err := loadPersistedACLs(env)
	if err != nil {
		return err
	}
	rules, err := loadPersistedRules(env)
	if err != nil {
		return err
	}
	e.ACLs = acls
	e.Rules = rules
	return nil
}

func loadPersistedCompanies(env *record.Env) (map[int64]Company, error) {
	rows, err := readExistingFields(env, "res.company", "name", "parent_id", "country_id", "active")
	if err != nil {
		return nil, err
	}
	out := map[int64]Company{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		active, ok := optionalBool(row["active"])
		if !ok {
			active = true
		}
		out[id] = Company{
			ID:        id,
			Name:      stringValue(row["name"]),
			ParentID:  int64Value(row["parent_id"]),
			CountryID: int64Value(row["country_id"]),
			Active:    active,
		}
	}
	return out, nil
}

func loadPersistedGroups(env *record.Env) (map[int64]Group, error) {
	rows, err := readExistingFields(env, "res.groups", "name", "category_id", "implied_ids", "allow_delegation", "allow_multiple_delegation", "restricted_access")
	if err != nil {
		return nil, err
	}
	out := map[int64]Group{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		out[id] = Group{
			ID:                      id,
			Name:                    stringValue(row["name"]),
			CategoryID:              int64Value(row["category_id"]),
			ImpliedIDs:              int64Slice(row["implied_ids"]),
			AllowDelegation:         boolValue(row["allow_delegation"]),
			AllowMultipleDelegation: boolValue(row["allow_multiple_delegation"]),
			RestrictedAccess:        boolValue(row["restricted_access"]),
		}
	}
	return out, nil
}

func loadPersistedUsers(env *record.Env) (map[int64]User, error) {
	rows, err := readExistingFields(env, "res.users", "login", "password", "email", "name", "active", "company_id", "company_ids", "partner_id", "commercial_partner_id", "groups_id", "group_ids")
	if err != nil {
		return nil, err
	}
	out := map[int64]User{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		active, ok := optionalBool(row["active"])
		if !ok {
			active = true
		}
		companyID := int64Value(row["company_id"])
		companyIDs := int64Slice(row["company_ids"])
		if len(companyIDs) == 0 && companyID != 0 {
			companyIDs = []int64{companyID}
		}
		partnerID := int64Value(row["partner_id"])
		commercialPartnerID := int64Value(row["commercial_partner_id"])
		if commercialPartnerID == 0 {
			commercialPartnerID = partnerID
		}
		out[id] = User{
			ID:                  id,
			Login:               stringValue(row["login"]),
			Password:            stringValue(row["password"]),
			Email:               stringValue(row["email"]),
			Name:                stringValue(row["name"]),
			Active:              active,
			CompanyID:           companyID,
			CompanyIDs:          companyIDs,
			PartnerID:           partnerID,
			CommercialPartnerID: commercialPartnerID,
			GroupIDs:            int64Slice(firstNonNil(row["groups_id"], row["group_ids"])),
		}
	}
	return out, nil
}

func loadPersistedHierarchy(env *record.Env, modelName string) (map[int64]int64, error) {
	rows, err := readExistingFields(env, modelName, "parent_id")
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		out[id] = int64Value(row["parent_id"])
	}
	return out, nil
}

func readExistingFields(env *record.Env, modelName string, requested ...string) ([]map[string]any, error) {
	modelSet := env.Model(modelName)
	descriptions, err := modelSet.FieldsGet(requested, nil)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return nil, nil
		}
		return nil, err
	}
	fields := []string{}
	for _, name := range requested {
		if _, ok := descriptions[name]; ok {
			fields = append(fields, name)
		}
	}
	found, err := modelSet.Search(domain.And())
	if err != nil {
		return nil, err
	}
	return found.Read(fields...)
}

func loadPersistedFieldGroups(env *record.Env) (map[string]map[string][]int64, error) {
	rows, err := readExistingFields(env, "ir.model.fields", "model", "name", "groups")
	if err != nil {
		return nil, err
	}
	groupXMLIDs, err := groupIDsByXMLID(env)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][]int64{}
	for _, row := range rows {
		modelName := stringValue(row["model"])
		fieldName := stringValue(row["name"])
		groupText := strings.TrimSpace(stringValue(row["groups"]))
		if modelName == "" || fieldName == "" || groupText == "" {
			continue
		}
		groupIDs := parsePersistedFieldGroupIDs(groupText, groupXMLIDs)
		if len(groupIDs) == 0 {
			continue
		}
		if out[modelName] == nil {
			out[modelName] = map[string][]int64{}
		}
		out[modelName][fieldName] = groupIDs
	}
	return out, nil
}

func mergeFieldGroups(dst map[string]map[string][]int64, src map[string]map[string][]int64) {
	for modelName, fields := range src {
		if dst[modelName] == nil {
			dst[modelName] = map[string][]int64{}
		}
		for fieldName, groupIDs := range fields {
			dst[modelName][fieldName] = append([]int64(nil), groupIDs...)
		}
	}
}

func groupIDsByXMLID(env *record.Env) (map[string]int64, error) {
	rows, err := readExistingFields(env, "ir.model.data", "module", "name", "model", "res_id")
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		if stringValue(row["model"]) != "res.groups" {
			continue
		}
		module := stringValue(row["module"])
		name := stringValue(row["name"])
		id := int64Value(row["res_id"])
		if module == "" || name == "" || id == 0 {
			continue
		}
		out[module+"."+name] = id
		if module == "base" {
			out[name] = id
		}
	}
	return out, nil
}

func parsePersistedFieldGroupIDs(text string, groupXMLIDs map[string]int64) []int64 {
	if strings.TrimSpace(text) == "." {
		return []int64{noAccessGroupID}
	}
	seen := map[int64]bool{}
	var out []int64
	for _, token := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		token = strings.TrimSpace(token)
		if token == "" || strings.HasPrefix(token, "!") {
			continue
		}
		if token == "." {
			if !seen[noAccessGroupID] {
				seen[noAccessGroupID] = true
				out = append(out, noAccessGroupID)
			}
			continue
		}
		id := groupXMLIDs[token]
		if id == 0 && !strings.Contains(token, ".") {
			id = groupXMLIDs["base."+token]
		}
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func loadPersistedACLs(env *record.Env) ([]ACL, error) {
	models, err := modelNamesByID(env)
	if err != nil {
		return nil, err
	}
	found, err := env.Model("ir.model.access").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "model", "model_id", "group_id", "active", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		return nil, err
	}
	acls := make([]ACL, 0, len(rows))
	for _, row := range rows {
		active, ok := optionalBool(row["active"])
		if ok && !active {
			continue
		}
		modelID := int64Value(row["model_id"])
		modelName := stringValue(row["model"])
		if modelName == "" {
			modelName = models[modelID]
		}
		acls = append(acls, ACL{
			Name:       stringValue(row["name"]),
			Model:      modelName,
			ModelID:    modelID,
			GroupID:    int64Value(row["group_id"]),
			Active:     true,
			PermRead:   boolValue(row["perm_read"]),
			PermWrite:  boolValue(row["perm_write"]),
			PermCreate: boolValue(row["perm_create"]),
			PermUnlink: boolValue(row["perm_unlink"]),
		})
	}
	return acls, nil
}

func loadPersistedRules(env *record.Env) ([]Rule, error) {
	models, err := modelNamesByID(env)
	if err != nil {
		return nil, err
	}
	found, err := env.Model("ir.rule").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "model", "model_id", "domain", "domain_force", "groups", "group_ids", "global", "active", "perm_read", "perm_write", "perm_create", "perm_unlink")
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(rows))
	for _, row := range rows {
		active, ok := optionalBool(row["active"])
		if ok && !active {
			continue
		}
		domainText := stringValue(firstNonNil(row["domain_force"], row["domain"]))
		node, err := ParseDomainForce(domainText)
		if err != nil {
			return nil, err
		}
		groupIDs := int64Slice(firstNonNil(row["groups"], row["group_ids"]))
		global, ok := optionalBool(row["global"])
		if !ok {
			global = len(groupIDs) == 0
		}
		modelID := int64Value(row["model_id"])
		modelName := stringValue(row["model"])
		if modelName == "" {
			modelName = models[modelID]
		}
		rules = append(rules, Rule{
			Name:       stringValue(row["name"]),
			Model:      modelName,
			ModelID:    modelID,
			Domain:     node,
			DomainText: domainText,
			GroupIDs:   groupIDs,
			Global:     global,
			Active:     true,
			PermRead:   boolDefault(row["perm_read"], true),
			PermWrite:  boolDefault(row["perm_write"], true),
			PermCreate: boolDefault(row["perm_create"], true),
			PermUnlink: boolDefault(row["perm_unlink"], true),
		})
	}
	return rules, nil
}

func modelNamesByID(env *record.Env) (map[int64]string, error) {
	found, err := env.Model("ir.model").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("model")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		modelName := stringValue(row["model"])
		if id != 0 && modelName != "" {
			out[id] = modelName
		}
	}
	return out, nil
}

func ParseDomainForce(text string) (domain.Node, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.And(), nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		return domain.Parse(value)
	}
	text = normalizeCompanyCountryIDs(text)
	normalized := strings.NewReplacer(
		"(", "[",
		")", "]",
		"'", "\"",
		"False", "false",
		"True", "true",
		"None", "null",
	).Replace(text)
	normalized = normalizeCompanyIDsPlusFalse(normalized)
	normalized = quoteBareDomainIdentifiers(normalized)
	if err := json.Unmarshal([]byte(normalized), &value); err == nil {
		return domain.Parse(value)
	}
	return domain.Node{}, fmt.Errorf("unsupported domain_force %q", text)
}

func normalizeCompanyIDsPlusFalse(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`company_ids\s*\+\s*\[\s*false\s*\]`),
		regexp.MustCompile(`\[\s*false\s*\]\s*\+\s*company_ids`),
	}
	for _, pattern := range patterns {
		text = pattern.ReplaceAllString(text, `"company_ids_plus_false"`)
	}
	return text
}

func normalizeCompanyCountryIDs(text string) string {
	mappedCountryIDs := `user\.env\.companies\.mapped\(\s*["']country_id["']\s*\)\.ids`
	patterns := []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		{regexp.MustCompile(mappedCountryIDs + `\s*\+\s*\[\s*(?:False|false)\s*\]`), `"company_country_ids_plus_false"`},
		{regexp.MustCompile(`\[\s*(?:False|false)\s*\]\s*\+\s*` + mappedCountryIDs), `"company_country_ids_plus_false"`},
		{regexp.MustCompile(mappedCountryIDs), `"company_country_ids"`},
	}
	for _, pattern := range patterns {
		text = pattern.pattern.ReplaceAllString(text, pattern.replacement)
	}
	return text
}

func quoteBareDomainIdentifiers(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '"' {
			j := i + 1
			for j < len(text) {
				if text[j] == '\\' {
					j += 2
					continue
				}
				if text[j] == '"' {
					j++
					break
				}
				j++
			}
			out.WriteString(text[i:j])
			i = j
			continue
		}
		j := i
		for j < len(text) && text[j] != '"' {
			j++
		}
		out.WriteString(bareDomainIdentifierRE.ReplaceAllString(text[i:j], `"$0"`))
		i = j
	}
	return out.String()
}

func (e *Engine) IssueSession(userID int64, rawToken string, expiresAt time.Time) {
	hash := tokenHash(rawToken)
	e.sessionMu.Lock()
	e.Sessions[hash] = Token{UserID: userID, Hash: hash, ExpiresAt: expiresAt}
	e.sessionMu.Unlock()
	e.Log(AuditEvent{ActorID: userID, Action: "session_create"})
}

func (e *Engine) AuthenticateSession(rawToken string) (int64, bool) {
	hash := tokenHash(rawToken)
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	token, ok := e.Sessions[hash]
	if !ok || token.RevokedAt.After(time.Time{}) || !token.ExpiresAt.After(e.now()) {
		return 0, false
	}
	token.LastUsedAt = e.now()
	e.Sessions[hash] = token
	return token.UserID, true
}

func (e *Engine) RevokeSession(rawToken string) {
	hash := tokenHash(rawToken)
	e.sessionMu.Lock()
	token := e.Sessions[hash]
	token.RevokedAt = e.now()
	e.Sessions[hash] = token
	e.sessionMu.Unlock()
	e.Log(AuditEvent{ActorID: token.UserID, Action: "session_revoke"})
}

func (e *Engine) SetSessionCompanies(rawToken string, companyID int64, companyIDs []int64) bool {
	hash := tokenHash(rawToken)
	e.sessionMu.Lock()
	token, ok := e.Sessions[hash]
	if !ok || token.RevokedAt.After(time.Time{}) || !token.ExpiresAt.After(e.now()) {
		e.sessionMu.Unlock()
		return false
	}
	token.CompanyID = companyID
	token.CompanyIDs = uniqueSessionCompanyIDs(companyIDs)
	e.Sessions[hash] = token
	e.sessionMu.Unlock()
	e.Log(AuditEvent{ActorID: token.UserID, Action: "session_switch_company", Details: map[string]string{"company_id": strconv.FormatInt(companyID, 10)}})
	return true
}

func (e *Engine) SessionCompanies(rawToken string) (int64, []int64, bool) {
	hash := tokenHash(rawToken)
	e.sessionMu.RLock()
	defer e.sessionMu.RUnlock()
	token, ok := e.Sessions[hash]
	if !ok || token.RevokedAt.After(time.Time{}) || !token.ExpiresAt.After(e.now()) {
		return 0, nil, false
	}
	if token.CompanyID == 0 && len(token.CompanyIDs) == 0 {
		return 0, nil, false
	}
	return token.CompanyID, append([]int64(nil), token.CompanyIDs...), true
}

func uniqueSessionCompanyIDs(ids []int64) []int64 {
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

func (e *Engine) IssueAPIToken(userID int64, name, rawToken string, scopes []string, expiresAt time.Time) {
	hash := tokenHash(rawToken)
	e.APITokens[hash] = Token{UserID: userID, Name: name, Hash: hash, Scopes: append([]string(nil), scopes...), ExpiresAt: expiresAt}
	e.Log(AuditEvent{ActorID: userID, Action: "api_token_create"})
}

func (e *Engine) Log(event AuditEvent) {
	if event.At.IsZero() {
		event.At = e.now()
	}
	e.Audit = append(e.Audit, redact(event))
}

func (e *Engine) checkFieldGroups(userID int64, modelName string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	for fieldName := range values {
		groups := e.FieldGroups[modelName][fieldName]
		if len(groups) > 0 && !e.hasAnyGroup(userID, groups) {
			return ErrAccessDenied
		}
	}
	return nil
}

func (e *Engine) hasAnyGroup(userID int64, groupIDs []int64) bool {
	groups := e.EffectiveGroupIDs(userID)
	return intersects(groups, groupIDs)
}

func aclAllows(acl ACL, op record.Operation) bool {
	switch op {
	case record.OpRead:
		return acl.PermRead
	case record.OpWrite:
		return acl.PermWrite
	case record.OpCreate:
		return acl.PermCreate
	case record.OpUnlink:
		return acl.PermUnlink
	default:
		return false
	}
}

func ruleApplies(rule Rule, op record.Operation) bool {
	switch op {
	case record.OpRead:
		return rule.PermRead
	case record.OpWrite:
		return rule.PermWrite
	case record.OpCreate:
		return rule.PermCreate
	case record.OpUnlink:
		return rule.PermUnlink
	default:
		return false
	}
}

func ruleGlobal(rule Rule) bool {
	return rule.Global || len(rule.GroupIDs) == 0
}

func intersects(groups map[int64]bool, ids []int64) bool {
	for _, id := range ids {
		if groups[id] {
			return true
		}
	}
	return false
}

func containsID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func rowCompanyInBothUsers(row map[string]any, delegate User, delegator User) bool {
	companyID := int64Value(valueForField(row, "company_id"))
	if companyID == 0 {
		return true
	}
	delegateCompanies := userCompanyIDs(delegate)
	delegatorCompanies := userCompanyIDs(delegator)
	if len(delegateCompanies) > 0 && !containsID(delegateCompanies, companyID) {
		return false
	}
	if len(delegatorCompanies) > 0 && !containsID(delegatorCompanies, companyID) {
		return false
	}
	return true
}

func delegatedDomainUser(delegate User, delegator User) (User, bool) {
	delegateCompanies := userCompanyIDs(delegate)
	delegatorCompanies := userCompanyIDs(delegator)
	if len(delegateCompanies) == 0 && len(delegatorCompanies) == 0 {
		return delegator, true
	}
	allowedCompanies := intersectIDs(delegateCompanies, delegatorCompanies)
	if len(allowedCompanies) == 0 {
		return User{}, false
	}
	scoped := delegator
	scoped.CompanyIDs = allowedCompanies
	switch {
	case containsID(allowedCompanies, delegate.CompanyID):
		scoped.CompanyID = delegate.CompanyID
	case containsID(allowedCompanies, delegator.CompanyID):
		scoped.CompanyID = delegator.CompanyID
	default:
		scoped.CompanyID = allowedCompanies[0]
	}
	return scoped, true
}

func intersectIDs(left []int64, right []int64) []int64 {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := map[int64]bool{}
	for _, id := range right {
		if id != 0 {
			rightSet[id] = true
		}
	}
	seen := map[int64]bool{}
	var out []int64
	for _, id := range left {
		if id == 0 || !rightSet[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (e *Engine) rowDepartmentAllowed(modelName string, row map[string]any, departmentIDs []int64) bool {
	if len(departmentIDs) == 0 {
		return true
	}
	allowedDepartmentIDs := departmentIDs
	if e.DepartmentDescendants != nil {
		allowedDepartmentIDs = e.DepartmentDescendants(departmentIDs)
	}
	recordDepartmentIDs := e.recordDepartmentIDs(modelName, row)
	if len(recordDepartmentIDs) == 0 {
		return false
	}
	for _, departmentID := range recordDepartmentIDs {
		if departmentID != 0 && containsID(allowedDepartmentIDs, departmentID) {
			return true
		}
	}
	return false
}

func (e *Engine) recordDepartmentIDs(modelName string, row map[string]any) []int64 {
	if e.RecordDepartments != nil {
		if ids, ok := e.RecordDepartments(modelName, row); ok {
			return ids
		}
	}
	for _, fieldName := range []string{"employee_id.department_id", "x_employee_id.department_id", "department_id", "x_department_id"} {
		if departmentID := int64Value(valueForField(row, fieldName)); departmentID != 0 {
			return []int64{departmentID}
		}
	}
	return nil
}

func userCompanyIDs(user User) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range append([]int64{user.CompanyID}, user.CompanyIDs...) {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func singletonIDSlice(id int64) []int64 {
	if id == 0 {
		return []int64{}
	}
	return []int64{id}
}

func resolveValue(user User, value any, companies map[int64]Company) any {
	switch typed := value.(type) {
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, resolveValue(user, item, companies))
		}
		return out
	}
	switch value {
	case "user.id":
		return user.ID
	case "user.company_id.id", "user.company_id", "user.env.company.id", "user.env.company":
		return user.CompanyID
	case "user.company_id.ids":
		return singletonIDSlice(user.CompanyID)
	case "user.company_ids", "user.company_ids.ids", "user.env.companies", "user.env.companies.ids":
		return userCompanyIDs(user)
	case "user.ids":
		return []int64{user.ID}
	case "user.partner_id", "user.partner_id.id":
		return user.PartnerID
	case "user.partner_id.ids":
		return singletonIDSlice(user.PartnerID)
	case "user.commercial_partner_id", "user.commercial_partner_id.id":
		if user.CommercialPartnerID != 0 {
			return user.CommercialPartnerID
		}
		return user.PartnerID
	case "user.commercial_partner_id.ids", "user.partner_id.commercial_partner_id.ids":
		return singletonIDSlice(firstNonZeroInt64(user.CommercialPartnerID, user.PartnerID))
	case "user.partner_id.commercial_partner_id", "user.partner_id.commercial_partner_id.id":
		return firstNonZeroInt64(user.CommercialPartnerID, user.PartnerID)
	case "user.group_ids", "user.group_ids.ids", "user.all_group_ids", "user.all_group_ids.ids":
		return user.GroupIDs
	case "user.employee_id", "user.employee_id.id", "user.employee_id.parent_id", "user.employee_id.parent_id.id":
		return int64(0)
	case "user.employee_id.ids", "user.employee_ids", "user.employee_ids.ids":
		return []int64{}
	case "company_id":
		return user.CompanyID
	case "company_ids":
		return userCompanyIDs(user)
	case "company_ids_plus_false":
		companyIDs := userCompanyIDs(user)
		values := make([]any, 0, len(companyIDs)+1)
		for _, id := range companyIDs {
			values = append(values, id)
		}
		return append(values, false)
	case "company_country_ids":
		return companyCountryIDs(user, companies)
	case "company_country_ids_plus_false":
		countryIDs := companyCountryIDs(user, companies)
		values := make([]any, 0, len(countryIDs)+1)
		for _, id := range countryIDs {
			values = append(values, id)
		}
		return append(values, false)
	default:
		return value
	}
}

func companyCountryIDs(user User, companies map[int64]Company) []int64 {
	if len(companies) == 0 {
		return nil
	}
	seen := map[int64]bool{}
	out := []int64{}
	for _, companyID := range userCompanyIDs(user) {
		countryID := companies[companyID].CountryID
		if countryID == 0 || seen[countryID] {
			continue
		}
		seen[countryID] = true
		out = append(out, countryID)
	}
	return out
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func optionalBool(value any) (bool, bool) {
	if value == nil {
		return false, false
	}
	return boolValue(value), true
}

func boolDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return boolValue(value)
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(typed)
		return parsed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func int64Value(value any) int64 {
	switch typed := domain.NormalizeScalar(value).(type) {
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func int64Slice(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []int:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64(item))
		}
		return out
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64Value(item))
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		var values []int64
		if err := json.Unmarshal([]byte(typed), &values); err == nil {
			return values
		}
		parts := strings.Split(typed, ",")
		out := make([]int64, 0, len(parts))
		for _, part := range parts {
			value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil {
				out = append(out, value)
			}
		}
		return out
	default:
		id := int64Value(value)
		if id == 0 {
			return nil
		}
		return []int64{id}
	}
}

func valueForField(row map[string]any, fieldName string) any {
	current := any(row)
	for _, part := range strings.Split(fieldName, ".") {
		values, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = values[part]
	}
	return current
}

func valuesEqual(left any, right any) bool {
	left = domain.NormalizeScalar(left)
	right = domain.NormalizeScalar(right)
	if left == nil || right == nil {
		return isFalsey(left) && isFalsey(right)
	}
	if reflect.TypeOf(left).Comparable() && reflect.TypeOf(right).Comparable() {
		return left == right
	}
	return reflect.DeepEqual(left, right)
}

func valueIn(left any, right any) (bool, error) {
	values, err := collectionValues(right)
	if err != nil {
		if isFalsey(right) {
			return isFalsey(left), nil
		}
		values = []any{right}
	}
	if leftValues, err := collectionValues(left); err == nil {
		if len(leftValues) == 0 {
			for _, value := range values {
				if isFalsey(value) {
					return true, nil
				}
			}
			return false, nil
		}
		for _, leftValue := range leftValues {
			for _, value := range values {
				if valuesEqual(leftValue, value) {
					return true, nil
				}
			}
		}
		return false, nil
	}
	for _, value := range values {
		if valuesEqual(left, value) {
			return true, nil
		}
	}
	return false, nil
}

func valueAny(modelName string, fieldName string, left any, right any, user User, companies map[int64]Company, hierarchies map[string]map[int64]int64) (bool, error) {
	leftValues, err := collectionValues(left)
	if err != nil {
		return false, fmt.Errorf("record rule operator any requires row-resident collection for %s.%s", modelName, fieldName)
	}
	childDomain, err := domain.Parse(right)
	if err != nil {
		rightValues, rightErr := collectionValues(right)
		if rightErr != nil {
			return false, fmt.Errorf("record rule operator any requires nested domain or collection value for %s.%s", modelName, fieldName)
		}
		if len(rightValues) == 0 {
			return len(leftValues) > 0, nil
		}
		for _, leftValue := range leftValues {
			for _, rightValue := range rightValues {
				if valuesEqual(leftValue, rightValue) {
					return true, nil
				}
			}
		}
		return false, nil
	}
	if childDomain.Kind == domain.All && len(childDomain.Children) == 0 {
		return len(leftValues) > 0, nil
	}
	for _, leftValue := range leftValues {
		childRow, ok := leftValue.(map[string]any)
		if !ok {
			return false, fmt.Errorf("record rule operator any with nested domain requires row-resident child maps for %s.%s", modelName, fieldName)
		}
		ok, err := evalDomainWithContext("", childRow, user, childDomain, companies, hierarchies)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func hierarchyValueMatch(row map[string]any, modelName string, fieldName string, left any, right any, op domain.Operator, companies map[int64]Company, hierarchies map[string]map[int64]int64) (bool, error) {
	leftIDs := int64Slice(left)
	rightIDs := int64Slice(right)
	if len(leftIDs) == 0 || len(rightIDs) == 0 {
		return valueIn(left, right)
	}
	if fieldName == "id" {
		if hierarchy := hierarchies[modelName]; len(hierarchy) > 0 {
			return hierarchyIDsMatch(leftIDs, rightIDs, op, hierarchy), nil
		}
		if op == domain.ChildOf {
			commercialID := int64Value(row["commercial_partner_id"])
			for _, rightID := range rightIDs {
				if commercialID != 0 && commercialID == rightID {
					return true, nil
				}
			}
		}
	}
	if partnerHierarchyField(fieldName) {
		if hierarchy := hierarchies["res.partner"]; len(hierarchy) > 0 {
			return hierarchyIDsMatch(leftIDs, rightIDs, op, hierarchy), nil
		}
	}
	if !(fieldName == "company_id" || (modelName == "res.company" && fieldName == "id")) || len(companies) == 0 {
		return valueIn(left, right)
	}
	for _, leftID := range leftIDs {
		for _, rightID := range rightIDs {
			switch op {
			case domain.ChildOf:
				if companyIsSelfOrDescendant(leftID, rightID, companies) {
					return true, nil
				}
			case domain.ParentOf:
				if companyIsSelfOrDescendant(rightID, leftID, companies) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func hierarchyIDsMatch(leftIDs []int64, rightIDs []int64, op domain.Operator, hierarchy map[int64]int64) bool {
	for _, leftID := range leftIDs {
		for _, rightID := range rightIDs {
			switch op {
			case domain.ChildOf:
				if recordIsSelfOrDescendant(leftID, rightID, hierarchy) {
					return true
				}
			case domain.ParentOf:
				if recordIsSelfOrDescendant(rightID, leftID, hierarchy) {
					return true
				}
			}
		}
	}
	return false
}

func partnerHierarchyField(fieldName string) bool {
	return fieldName == "partner_id" || strings.HasSuffix(fieldName, ".partner_id")
}

func recordIsSelfOrDescendant(candidate int64, ancestor int64, hierarchy map[int64]int64) bool {
	if candidate == 0 || ancestor == 0 {
		return false
	}
	if candidate == ancestor {
		return true
	}
	seen := map[int64]bool{}
	for current := candidate; current != 0 && !seen[current]; {
		seen[current] = true
		current = hierarchy[current]
		if current == ancestor {
			return true
		}
	}
	return false
}

func companyIsSelfOrDescendant(candidate int64, ancestor int64, companies map[int64]Company) bool {
	if candidate == 0 || ancestor == 0 {
		return false
	}
	if candidate == ancestor {
		return true
	}
	seen := map[int64]bool{}
	for current := candidate; current != 0 && !seen[current]; {
		seen[current] = true
		company, ok := companies[current]
		if !ok {
			return false
		}
		current = company.ParentID
		if current == ancestor {
			return true
		}
	}
	return false
}

func collectionValues(value any) ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("not a collection")
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("not a collection")
	}
	values := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		values = append(values, rv.Index(i).Interface())
	}
	return values, nil
}

func compare(left any, right any, op domain.Operator) (bool, error) {
	left = domain.NormalizeScalar(left)
	right = domain.NormalizeScalar(right)
	if leftNumber, ok := numeric(left); ok {
		rightNumber, ok := numeric(right)
		if !ok {
			return false, fmt.Errorf("cannot compare numeric value with %T", right)
		}
		return compareFloat(leftNumber, rightNumber, op), nil
	}
	leftText, leftOK := left.(string)
	rightText, rightOK := right.(string)
	if leftOK && rightOK {
		return compareString(leftText, rightText, op), nil
	}
	return false, fmt.Errorf("cannot compare %T and %T", left, right)
}

func numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func compareFloat(left float64, right float64, op domain.Operator) bool {
	switch op {
	case domain.Less:
		return left < right
	case domain.LessEqual:
		return left <= right
	case domain.Greater:
		return left > right
	case domain.GreaterEqual:
		return left >= right
	default:
		return false
	}
}

func compareString(left string, right string, op domain.Operator) bool {
	switch op {
	case domain.Less:
		return left < right
	case domain.LessEqual:
		return left <= right
	case domain.Greater:
		return left > right
	case domain.GreaterEqual:
		return left >= right
	default:
		return false
	}
}

func containsMatch(left any, right any, caseFold bool, pattern bool) bool {
	leftText := fmt.Sprint(left)
	rightText := fmt.Sprint(right)
	if caseFold {
		leftText = strings.ToLower(leftText)
		rightText = strings.ToLower(rightText)
	}
	if pattern {
		return wildcardMatch(leftText, rightText)
	}
	return strings.Contains(leftText, rightText)
}

func wildcardMatch(text string, pattern string) bool {
	var builder strings.Builder
	builder.WriteString("^")
	for _, char := range pattern {
		switch char {
		case '%':
			builder.WriteString(".*")
		case '_':
			builder.WriteString(".")
		default:
			builder.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	builder.WriteString("$")
	ok, _ := regexp.MatchString(builder.String(), text)
	return ok
}

func isTruthy(value any) bool {
	return !isFalsey(value)
}

func isFalsey(value any) bool {
	switch typed := domain.NormalizeScalar(value).(type) {
	case nil:
		return true
	case bool:
		return !typed
	case int64:
		return typed == 0
	case float64:
		return typed == 0
	case string:
		return typed == ""
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() == 0
		default:
			return false
		}
	}
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func TokensEqual(raw, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(tokenHash(raw)), []byte(hash)) == 1
}

func redact(event AuditEvent) AuditEvent {
	if len(event.Details) == 0 {
		return event
	}
	details := map[string]string{}
	for key, value := range event.Details {
		switch key {
		case "password", "token", "api_key", "secret":
			details[key] = "[redacted]"
		default:
			details[key] = value
		}
	}
	event.Details = details
	return event
}
