package record

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	coreaccounting "gorp/internal/accounting"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/phone"
	"gorp/internal/sequencecore"
)

const linkTrackerCodeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var linkTrackerRandomRead = rand.Read

type Context struct {
	UserID     int64
	CompanyID  int64
	CompanyIDs []int64
	Sudo       bool
	Values     map[string]any
}

type Operation string

const (
	OpRead   Operation = "read"
	OpWrite  Operation = "write"
	OpCreate Operation = "create"
	OpUnlink Operation = "unlink"
)

type Policy interface {
	Check(Context, string, Operation, map[string]any) error
	CheckRecord(Context, string, Operation, map[string]any) (bool, error)
	FilterFields(Context, string, []string) []string
}

type AfterCreateHook func(env *Env, modelName string, id int64, row map[string]any) error
type AfterWriteHook func(env *Env, modelName string, id int64, oldRow map[string]any, newRow map[string]any, values map[string]any) error
type BeforeUnlinkHook func(env *Env, modelName string, id int64, row map[string]any) error

type Registry struct {
	models map[string]model.Model
}

func NewRegistry() *Registry {
	return &Registry{models: map[string]model.Model{}}
}

func (r *Registry) Register(m model.Model) error {
	m = r.composeInheritedFields(m)
	if err := m.Validate(); err != nil {
		return err
	}
	if _, exists := r.models[m.Name]; exists {
		return fmt.Errorf("model %s already registered", m.Name)
	}
	r.models[m.Name] = m
	return nil
}

func (r *Registry) composeInheritedFields(m model.Model) model.Model {
	for _, inherited := range m.Inherit {
		parent, ok := r.models[inherited]
		if !ok {
			continue
		}
		m = m.Compose(parent)
	}
	return m
}

func (r *Registry) Model(name string) (model.Model, bool) {
	m, ok := r.models[name]
	return m, ok
}

type Env struct {
	registry          *Registry
	context           Context
	stores            map[string]*store
	policy            Policy
	sequenceNamespace string
	afterCreateHooks  []AfterCreateHook
	afterWriteHooks   []AfterWriteHook
	beforeUnlinkHooks []BeforeUnlinkHook
	accountMovePost   bool
}

type storeSnapshot struct {
	exists  bool
	nextID  int64
	records map[int64]map[string]any
}

type Snapshot struct {
	stores map[string]storeSnapshot
}

var envSequenceNamespaceCounter atomic.Int64

func NewEnv(registry *Registry, context Context) *Env {
	if context.Values == nil {
		context.Values = map[string]any{}
	}
	return &Env{
		registry:          registry,
		context:           context,
		stores:            map[string]*store{},
		sequenceNamespace: fmt.Sprintf("env:%d", envSequenceNamespaceCounter.Add(1)),
	}
}

func (e *Env) WithPolicy(policy Policy) *Env {
	e.policy = policy
	return e
}

func (e *Env) WithContext(context Context) *Env {
	if context.Values == nil {
		context.Values = map[string]any{}
	}
	return &Env{
		registry:          e.registry,
		context:           context,
		stores:            e.stores,
		policy:            e.policy,
		sequenceNamespace: e.sequenceNamespace,
		afterCreateHooks:  e.afterCreateHooks,
		afterWriteHooks:   e.afterWriteHooks,
		beforeUnlinkHooks: e.beforeUnlinkHooks,
		accountMovePost:   e.accountMovePost,
	}
}

func (e *Env) WithSequenceNamespace(namespace string) *Env {
	if strings.TrimSpace(namespace) == "" {
		namespace = fmt.Sprintf("env:%d", envSequenceNamespaceCounter.Add(1))
	}
	return &Env{
		registry:          e.registry,
		context:           e.context,
		stores:            e.stores,
		policy:            e.policy,
		sequenceNamespace: namespace,
		afterCreateHooks:  e.afterCreateHooks,
		afterWriteHooks:   e.afterWriteHooks,
		beforeUnlinkHooks: e.beforeUnlinkHooks,
		accountMovePost:   e.accountMovePost,
	}
}

func (e *Env) WithAccountMovePost() *Env {
	return &Env{
		registry:          e.registry,
		context:           e.context,
		stores:            e.stores,
		policy:            e.policy,
		sequenceNamespace: e.sequenceNamespace,
		afterCreateHooks:  e.afterCreateHooks,
		afterWriteHooks:   e.afterWriteHooks,
		beforeUnlinkHooks: e.beforeUnlinkHooks,
		accountMovePost:   true,
	}
}

func (e *Env) Context() Context {
	return e.context
}

func (e *Env) Policy() Policy {
	return e.policy
}

func (e *Env) RegisterAfterCreateHook(hook AfterCreateHook) {
	if hook != nil {
		e.afterCreateHooks = append(e.afterCreateHooks, hook)
	}
}

func (e *Env) RegisterAfterWriteHook(hook AfterWriteHook) {
	if hook != nil {
		e.afterWriteHooks = append(e.afterWriteHooks, hook)
	}
}

func (e *Env) RegisterBeforeUnlinkHook(hook BeforeUnlinkHook) {
	if hook != nil {
		e.beforeUnlinkHooks = append(e.beforeUnlinkHooks, hook)
	}
}

func (e *Env) SequenceNamespace(_ string) string {
	return e.sequenceNamespace
}

func (e *Env) ModelMetadata(name string) (model.Model, bool) {
	return e.registry.Model(name)
}

func (e *Env) Model(name string) ModelSet {
	meta, ok := e.registry.Model(name)
	if !ok {
		return ModelSet{err: fmt.Errorf("unknown model %s", name)}
	}
	s, ok := e.stores[name]
	if !ok {
		s = &store{nextID: 1, records: map[int64]map[string]any{}}
		e.stores[name] = s
	}
	return ModelSet{env: e, model: meta, store: s}
}

func (e *Env) Snapshot() Snapshot {
	if e == nil {
		return Snapshot{}
	}
	return Snapshot{stores: ModelSet{env: e}.snapshotEnv()}
}

func (e *Env) Restore(snapshot Snapshot) {
	if e == nil || snapshot.stores == nil {
		return
	}
	ModelSet{env: e}.restoreEnv(snapshot.stores)
}

type ModelSet struct {
	env   *Env
	model model.Model
	store *store
	err   error
}

func (m ModelSet) Create(values map[string]any) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	values = m.normalizeCreateValues(values)
	var err error
	values, err = m.sanitizeIrConfigParameterValues(nil, values)
	if err != nil {
		return 0, err
	}
	values = m.withCreateLogAccessValues(values, time.Now().UTC())
	switch m.model.Name {
	case "res.partner", "mailing.contact", "mailing.trace", "phone.blacklist":
		values = m.normalizeRecordWriteValues(nil, values)
	}
	explicitID := int64(0)
	if rawID, ok := values["id"]; ok {
		explicitID = numericID(rawID)
		if explicitID <= 0 {
			return 0, fmt.Errorf("invalid explicit id for %s", m.model.Name)
		}
		values = copyValues(values)
		delete(values, "id")
	}
	needsActionBase := m.needsActionBaseSync()
	envSnapshot := map[string]storeSnapshot{}
	irExportsPayload, hasIrExportsPayload := values["export_fields"]
	if len(m.env.afterCreateHooks) > 0 || needsActionBase || (m.model.Name == "ir.exports" && hasIrExportsPayload) || m.model.Name == "mail.alias.domain" || m.model.Name == "link.tracker" || m.model.Name == "mailing.mailing" || m.model.Name == "fetchmail.server" {
		envSnapshot = m.snapshotEnv()
	}
	createdID := int64(0)
	restoreCreate := func(err error) (int64, error) {
		if len(envSnapshot) > 0 {
			m.restoreEnv(envSnapshot)
		} else if createdID != 0 {
			delete(m.store.records, createdID)
		}
		return 0, err
	}
	modelSnapshot := storeSnapshot{}
	mailSnapshot := storeSnapshot{}
	trackingSnapshot := storeSnapshot{}
	mailMessageSnapshot := storeSnapshot{}
	mailMessageTrackingPayload, hasMailMessageTracking := values["tracking_value_ids"]
	if m.model.Name == "account.move.line" {
		modelSnapshot = m.snapshotStore("account.move.line")
		mailSnapshot = m.snapshotStore("mail.message")
		trackingSnapshot = m.snapshotStore("mail.tracking.value")
	} else if m.model.Name == "mail.message" && hasMailMessageTracking {
		mailMessageSnapshot = m.snapshotStore("mail.message")
		trackingSnapshot = m.snapshotStore("mail.tracking.value")
	}
	if m.model.Name == "mail.message" && hasMailMessageTracking {
		values = copyValues(values)
		values["tracking_value_ids"] = []int64{}
	}
	if m.model.Name == "ir.exports" && hasIrExportsPayload {
		values = copyValues(values)
		values["export_fields"] = []int64{}
	}
	if err := m.check(OpCreate, values); err != nil {
		return 0, err
	}
	for key, value := range values {
		if _, ok := m.model.Fields[key]; !ok {
			return 0, fmt.Errorf("unknown field %s.%s", m.model.Name, key)
		}
		_ = value
	}
	id := explicitID
	if needsActionBase && id == 0 {
		actionID, err := m.createActionBase(values)
		if err != nil {
			return restoreCreate(err)
		}
		id = actionID
	}
	if id == 0 {
		id = m.store.nextID
		m.store.nextID++
	} else {
		if _, exists := m.store.records[id]; exists {
			return restoreCreate(fmt.Errorf("%s:%d already exists", m.model.Name, id))
		}
		if id >= m.store.nextID {
			m.store.nextID = id + 1
		}
	}
	row := map[string]any{"id": id}
	for key, value := range values {
		row[key] = value
	}
	if err := m.validateCreateConstraints(row); err != nil {
		return restoreCreate(err)
	}
	if m.model.Name == "link.tracker" {
		if err := m.validateLinkTrackerCreate(row); err != nil {
			return restoreCreate(err)
		}
	}
	if m.model.Name == "link.tracker.code" {
		if err := m.validateLinkTrackerCodeUnique(row); err != nil {
			return restoreCreate(err)
		}
	}
	m.store.records[id] = row
	createdID = id
	if err := m.validateModelConstraints(row); err != nil {
		return restoreCreate(err)
	}
	if m.model.Name == "res.partner" {
		m.applyResPartnerDerivedFields(id, row)
	}
	if m.model.Name == "res.users" {
		m.applyResUsersDerivedFields(id, row)
	}
	if ok, err := m.allowedRecord(OpCreate, row); err != nil || !ok {
		if err != nil {
			return restoreCreate(err)
		}
		return restoreCreate(fmt.Errorf("record rule denied create on %s", m.model.Name))
	}
	if m.model.Name == "res.partner" {
		m.applyResPartnerDerivedFields(id, row)
	}
	if m.model.Name == "res.users" {
		m.syncResUserPartnerActive(row, false)
		m.applyResUsersDerivedFields(id, row)
		m.syncResUserPartnerShare(nil, row)
		m.syncAllResGroupsDerivedFields()
	}
	if m.model.Name == "mail.message" && hasMailMessageTracking {
		trackingIDs, err := m.applyMailMessageTrackingCommands(id, mailMessageTrackingPayload)
		if err != nil {
			if len(envSnapshot) > 0 {
				m.restoreEnv(envSnapshot)
			} else {
				m.restoreStore("mail.message", mailMessageSnapshot)
				m.restoreStore("mail.tracking.value", trackingSnapshot)
			}
			return 0, err
		}
		row["tracking_value_ids"] = trackingIDs
	}
	if m.model.Name == "ir.exports" && hasIrExportsPayload {
		exportLineIDs, err := m.applyIrExportsLineCommands(id, irExportsPayload)
		if err != nil {
			return restoreCreate(err)
		}
		row["export_fields"] = exportLineIDs
	}
	if m.model.Name == "account.move.line" {
		if err := m.trackAccountMoveLineCreate(row); err != nil {
			if len(envSnapshot) > 0 {
				m.restoreEnv(envSnapshot)
			} else {
				m.restoreStore("account.move.line", modelSnapshot)
				m.restoreStore("mail.message", mailSnapshot)
				m.restoreStore("mail.tracking.value", trackingSnapshot)
			}
			return 0, err
		}
	}
	if m.model.Name == "mail.activity.type" {
		m.syncMailActivityTypePreviousIDs()
	}
	if m.model.Name == "mailing.mailing" {
		if err := m.ensureMailingABTestingCampaign(row); err != nil {
			return restoreCreate(err)
		}
		m.syncAllMailingDerivedFields()
	}
	if m.model.Name == "marketing.trace" || m.model.Name == "whatsapp.message" {
		m.syncAllWhatsAppMarketingDerivedFields()
	}
	if m.model.Name == "res.groups" {
		m.syncResGroupsInverseFields(id, nil, row)
		m.syncAllResGroupsDerivedFields()
		m.syncAllResUsersDerivedFields()
	}
	if m.model.Name == "ir.model.data" && stringValue(row["model"]) == "res.groups" {
		m.syncAllResGroupsDerivedFields()
		m.syncAllResUsersDerivedFields()
	}
	if m.model.Name == "mail.alias.domain" {
		if err := m.assignFirstMailAliasDomain(id); err != nil {
			return restoreCreate(err)
		}
	}
	if m.modelChangesResGroupDerivedFields() {
		m.syncAllResGroupsDerivedFields()
	}
	if m.model.Name == "res.partner" {
		m.syncResPartnerDependents(id)
	}
	if err := m.syncActionBaseRow(id, row); err != nil {
		return restoreCreate(err)
	}
	if m.model.Name == "link.tracker" {
		if err := m.syncLinkTrackerDerivedFields(id, row, nil); err != nil {
			return restoreCreate(err)
		}
	}
	if m.model.Name == "phone.blacklist" {
		m.syncAllPhoneBlacklistDerivedFields()
	}
	if m.model.Name == "fetchmail.server" {
		if err := m.syncFetchmailGatewayCron(); err != nil {
			return restoreCreate(err)
		}
	}
	for _, hook := range m.env.afterCreateHooks {
		if err := hook(m.env, m.model.Name, id, copyValues(row)); err != nil {
			return restoreCreate(err)
		}
	}
	return id, nil
}

func (m ModelSet) needsActionBaseSync() bool {
	if !isConcreteActionModel(m.model.Name) {
		return false
	}
	_, ok := m.env.registry.Model("ir.actions.actions")
	return ok
}

func (m ModelSet) withCreateLogAccessValues(values map[string]any, now time.Time) map[string]any {
	if !m.hasAnyLogAccessField("create_uid", "create_date", "write_uid", "write_date") {
		return values
	}
	out := copyValues(values)
	userID := m.env.context.UserID
	m.setLogAccessValue(out, "create_uid", userID)
	m.setLogAccessValue(out, "write_uid", userID)
	m.setLogAccessValue(out, "create_date", now)
	m.setLogAccessValue(out, "write_date", now)
	return out
}

func (m ModelSet) withWriteLogAccessValues(values map[string]any, now time.Time) map[string]any {
	if !m.hasAnyLogAccessField("create_uid", "create_date", "write_uid", "write_date") {
		return values
	}
	out := copyValues(values)
	for _, name := range []string{"create_uid", "create_date", "write_uid", "write_date"} {
		if _, ok := m.model.Fields[name]; ok {
			delete(out, name)
		}
	}
	if _, ok := m.model.Fields["write_uid"]; ok {
		out["write_uid"] = m.env.context.UserID
	}
	if _, ok := m.model.Fields["write_date"]; ok {
		out["write_date"] = now
	}
	return out
}

func (m ModelSet) sanitizeIrConfigParameterValues(existing map[string]any, values map[string]any) (map[string]any, error) {
	if m.model.Name != "ir.config_parameter" || values == nil {
		return values, nil
	}
	rawValue, hasValue := values["value"]
	if !hasValue || !irConfigParameterValueTruthy(rawValue) {
		return values, nil
	}
	key := ""
	if rawKey, ok := values["key"]; ok {
		key = stringValue(rawKey)
	} else if existing != nil {
		key = stringValue(existing["key"])
	}
	if key != "mail.catchall.domain.allowed" {
		return values, nil
	}
	sanitized, err := sanitizeCatchallAllowedDomains(stringValue(rawValue))
	if err != nil {
		return nil, err
	}
	out := copyValues(values)
	out["value"] = sanitized
	return out, nil
}

func irConfigParameterValueTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return true
	}
}

func sanitizeCatchallAllowedDomains(value string) (string, error) {
	domains := []string{}
	for _, domain := range strings.Split(value, ",") {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("value %s for `mail.catchall.domain.allowed` cannot be validated.\nit should be a comma separated list of domains e.g. example.com,example.org", value)
	}
	return strings.Join(domains, ","), nil
}

func (m ModelSet) hasAnyLogAccessField(names ...string) bool {
	for _, name := range names {
		if _, ok := m.model.Fields[name]; ok {
			return true
		}
	}
	return false
}

func (m ModelSet) setLogAccessValue(values map[string]any, name string, value any) {
	if _, ok := m.model.Fields[name]; !ok {
		return
	}
	values[name] = value
}

func isConcreteActionModel(modelName string) bool {
	switch modelName {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return true
	default:
		return false
	}
}

func (m ModelSet) createActionBase(values map[string]any) (int64, error) {
	base := m.env.Model("ir.actions.actions")
	if base.err != nil {
		return 0, base.err
	}
	return base.Create(actionBaseValues(base.model, m.model.Name, values))
}

func (m ModelSet) syncActionBaseRow(id int64, row map[string]any) error {
	if id <= 0 || !m.needsActionBaseSync() {
		return nil
	}
	base := m.env.Model("ir.actions.actions")
	if base.err != nil {
		return base.err
	}
	values := actionBaseValues(base.model, m.model.Name, row)
	if existing, ok := base.store.records[id]; ok {
		existingType := stringValue(existing["type"])
		nextType := stringValue(values["type"])
		if existingType != "" && nextType != "" && existingType != nextType {
			return fmt.Errorf("global action id %d already belongs to %s", id, existingType)
		}
		for key, value := range values {
			existing[key] = value
		}
		return nil
	}
	values["id"] = id
	_, err := base.Create(values)
	return err
}

func actionBaseValues(base model.Model, actionModel string, row map[string]any) map[string]any {
	values := map[string]any{
		"name": firstNonEmptyRecordString(stringValue(row["name"]), "Action"),
		"type": firstNonEmptyRecordString(stringValue(row["type"]), actionModel),
	}
	for _, fieldName := range []string{"xml_id", "help", "path", "binding_model_id", "binding_type", "binding_view_types"} {
		if base.Fields[fieldName].Name == "" {
			continue
		}
		if value, ok := row[fieldName]; ok {
			values[fieldName] = value
		}
	}
	return values
}

func firstNonEmptyRecordString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (m ModelSet) syncLinkTrackerDerivedFields(id int64, row map[string]any, changed map[string]any) error {
	if id == 0 || row == nil {
		return nil
	}
	code := strings.TrimSpace(stringValue(row["code"]))
	explicitCode := ""
	if changed != nil {
		explicitCode = strings.TrimSpace(stringValue(changed["code"]))
	}
	codeRow := m.firstLinkTrackerCodeRow(id)
	if codeRow != nil {
		if explicitCode != "" {
			if !m.linkTrackerCodeAvailable(explicitCode, id, numericID(codeRow["id"])) {
				return fmt.Errorf("link.tracker.code %s already exists", explicitCode)
			}
			codeRow["code"] = explicitCode
		}
		code = strings.TrimSpace(stringValue(codeRow["code"]))
		if code == "" {
			code = strings.TrimSpace(stringValue(row["code"]))
			if code == "" {
				var err error
				code, err = m.nextLinkTrackerCode()
				if err != nil {
					return err
				}
			}
			if !m.linkTrackerCodeAvailable(code, id, numericID(codeRow["id"])) {
				return fmt.Errorf("link.tracker.code %s already exists", code)
			}
			codeRow["code"] = code
		}
	} else {
		if code == "" {
			var err error
			code, err = m.nextLinkTrackerCode()
			if err != nil {
				return err
			}
		}
		if _, ok := m.env.ModelMetadata("link.tracker.code"); ok {
			if _, err := m.env.Model("link.tracker.code").Create(map[string]any{"code": code, "link_id": id}); err != nil {
				return err
			}
		}
	}
	if code != "" {
		row["code"] = code
		baseURL := m.linkTrackerBaseURL()
		shortHost := baseURL + "/r/"
		row["short_url_host"] = shortHost
		row["short_url"] = shortHost + code
	}
	trackerURL := strings.TrimSpace(stringValue(row["url"]))
	if strings.TrimSpace(stringValue(row["title"])) == "" && trackerURL != "" {
		row["title"] = trackerURL
	}
	if strings.TrimSpace(stringValue(row["redirected_url"])) == "" || linkTrackerRedirectInputsChanged(changed) {
		if changed == nil || !hasAnyKey(changed, "redirected_url") {
			row["redirected_url"] = m.linkTrackerRedirectedURL(row)
		}
	}
	return nil
}

func (m ModelSet) normalizeLinkTrackerValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		for _, fieldName := range []string{"campaign_id", "source_id", "medium_id"} {
			if _, ok := out[fieldName]; !ok {
				out[fieldName] = int64(0)
			}
		}
	}
	if _, ok := out["url"]; ok || existing == nil {
		rawURL := stringValue(safeRowValue(existing, "url"))
		if value, ok := out["url"]; ok {
			rawURL = stringValue(value)
		}
		normalized := normalizeLinkTrackerURLRecord(rawURL)
		if normalized != "" {
			out["url"] = normalized
			if _, ok := out["absolute_url"]; !ok {
				out["absolute_url"] = normalized
			}
		}
	}
	if existing == nil {
		if _, ok := out["count"]; !ok {
			out["count"] = int64(0)
		}
	}
	return out
}

func normalizeLinkTrackerURLRecord(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return parsed.String()
}

func (m ModelSet) validateLinkTrackerCreate(row map[string]any) error {
	if strings.TrimSpace(stringValue(row["url"])) == "" {
		return fmt.Errorf("link.tracker requires url")
	}
	if err := validateLinkTrackerURLRecord(stringValue(row["url"])); err != nil {
		return err
	}
	return m.validateLinkTrackerUnique(row)
}

func (m ModelSet) validateLinkTrackerWrite(existing map[string]any, values map[string]any) error {
	merged := copyValues(existing)
	for key, value := range values {
		merged[key] = value
	}
	if strings.TrimSpace(stringValue(merged["url"])) == "" {
		return fmt.Errorf("link.tracker requires url")
	}
	if err := validateLinkTrackerURLRecord(stringValue(merged["url"])); err != nil {
		return err
	}
	return m.validateLinkTrackerUnique(merged)
}

func validateLinkTrackerURLRecord(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("link.tracker requires url")
	}
	if strings.HasPrefix(rawURL, "?") || strings.HasPrefix(rawURL, "#") {
		return fmt.Errorf("link.tracker url cannot start with ? or #")
	}
	if _, err := url.Parse(rawURL); err != nil {
		return fmt.Errorf("invalid link.tracker url: %w", err)
	}
	return nil
}

func (m ModelSet) validateLinkTrackerUnique(row map[string]any) error {
	for _, existing := range m.rows("link.tracker") {
		if numericID(existing["id"]) == numericID(row["id"]) {
			continue
		}
		if strings.TrimSpace(stringValue(existing["url"])) == strings.TrimSpace(stringValue(row["url"])) &&
			numericID(existing["campaign_id"]) == numericID(row["campaign_id"]) &&
			numericID(existing["medium_id"]) == numericID(row["medium_id"]) &&
			numericID(existing["source_id"]) == numericID(row["source_id"]) &&
			strings.TrimSpace(stringValue(existing["label"])) == strings.TrimSpace(stringValue(row["label"])) {
			return fmt.Errorf("link.tracker already exists for url/campaign/medium/source/label")
		}
	}
	return nil
}

func (m ModelSet) validateLinkTrackerCodeUnique(row map[string]any) error {
	code := strings.TrimSpace(stringValue(row["code"]))
	if code == "" {
		return nil
	}
	for _, existing := range m.rows("link.tracker.code") {
		if numericID(existing["id"]) == numericID(row["id"]) {
			continue
		}
		if strings.TrimSpace(stringValue(existing["code"])) == code {
			return fmt.Errorf("link.tracker.code %s already exists", code)
		}
	}
	return nil
}

func (m ModelSet) linkTrackerCodeAvailable(code string, _ int64, codeID int64) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return true
	}
	for _, row := range m.rows("link.tracker.code") {
		if numericID(row["id"]) == codeID {
			continue
		}
		if strings.TrimSpace(stringValue(row["code"])) == code {
			return false
		}
	}
	return true
}

func (m ModelSet) nextLinkTrackerCode() (string, error) {
	return NextLinkTrackerCode(m.env)
}

func (m ModelSet) firstLinkTrackerCodeRow(linkID int64) map[string]any {
	var out map[string]any
	for _, row := range m.rows("link.tracker.code") {
		if numericID(row["link_id"]) != linkID {
			continue
		}
		if out == nil || numericID(row["id"]) < numericID(out["id"]) {
			out = row
		}
	}
	return out
}

func linkTrackerRedirectInputsChanged(changed map[string]any) bool {
	if changed == nil {
		return false
	}
	return hasAnyKey(changed, "url", "campaign_id", "source_id", "medium_id")
}

func (m ModelSet) linkTrackerBaseURL() string {
	baseURL := strings.TrimRight(m.configParameterValue("web.base.url"), "/")
	if baseURL == "" {
		return "http://localhost"
	}
	return baseURL
}

func (m ModelSet) linkTrackerRedirectedURL(row map[string]any) string {
	rawURL := strings.TrimSpace(stringValue(row["url"]))
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if m.configParameterBool("link_tracker.no_external_tracking") {
		baseParsed, _ := url.Parse(m.linkTrackerBaseURL())
		if parsed.Host != "" && baseParsed != nil && parsed.Host != baseParsed.Host {
			return parsed.String()
		}
	}
	query := parsed.Query()
	if value := m.utmName("utm.campaign", numericID(row["campaign_id"])); value != "" {
		query.Set("utm_campaign", value)
	}
	if value := m.utmName("utm.source", numericID(row["source_id"])); value != "" {
		query.Set("utm_source", value)
	}
	if value := m.utmName("utm.medium", numericID(row["medium_id"])); value != "" {
		query.Set("utm_medium", value)
	}
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "...", "%2E%2E%2E")
	return parsed.String()
}

func (m ModelSet) utmName(modelName string, id int64) string {
	row := m.rowByID(modelName, id)
	if row == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(row["name"]))
}

func (m ModelSet) configParameterBool(key string) bool {
	value := strings.TrimSpace(m.configParameterValue(key))
	return value == "1" || strings.EqualFold(value, "true")
}

func (m ModelSet) configParameterValue(key string) string {
	if _, ok := m.env.ModelMetadata("ir.config_parameter"); !ok {
		return ""
	}
	for _, row := range m.rows("ir.config_parameter") {
		if strings.TrimSpace(stringValue(row["key"])) == key {
			return strings.TrimSpace(stringValue(row["value"]))
		}
	}
	return ""
}

func NextLinkTrackerCode(env *Env) (string, error) {
	codes, err := RandomLinkTrackerCodes(env, 1)
	if err != nil {
		return "", err
	}
	if len(codes) == 0 {
		return "", fmt.Errorf("link.tracker.code generation returned no codes")
	}
	return codes[0], nil
}

func RandomLinkTrackerCodes(env *Env, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	for size := 3; size <= 32; size++ {
		codes := make([]string, 0, n)
		seen := map[string]bool{}
		for len(codes) < n {
			code, err := randomLinkTrackerCode(size)
			if err != nil {
				return nil, err
			}
			if seen[code] {
				size++
				codes = nil
				seen = map[string]bool{}
				continue
			}
			seen[code] = true
			codes = append(codes, code)
		}
		if linkTrackerCodesAvailable(env, codes) {
			return codes, nil
		}
	}
	return nil, fmt.Errorf("could not generate unique link.tracker.code values")
}

func randomLinkTrackerCode(size int) (string, error) {
	if size <= 0 {
		return "", nil
	}
	buf := make([]byte, size)
	random := make([]byte, size)
	if _, err := linkTrackerRandomRead(random); err != nil {
		return "", err
	}
	for i, value := range random {
		buf[i] = linkTrackerCodeAlphabet[int(value)%len(linkTrackerCodeAlphabet)]
	}
	return string(buf), nil
}

func linkTrackerCodesAvailable(env *Env, codes []string) bool {
	if env == nil || len(codes) == 0 {
		return true
	}
	wanted := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			wanted[code] = true
		}
	}
	if len(wanted) == 0 {
		return true
	}
	if _, ok := env.ModelMetadata("link.tracker.code"); !ok {
		return true
	}
	for _, row := range env.Model("link.tracker.code").rows("link.tracker.code") {
		if wanted[strings.TrimSpace(stringValue(row["code"]))] {
			return false
		}
	}
	return true
}

func LinkTrackerSearchOrCreate(env *Env, valsList []map[string]any) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("link.tracker search_or_create requires env")
	}
	if len(valsList) == 0 {
		return []int64{}, nil
	}
	tracker := env.Model("link.tracker")
	normalized := make([]map[string]any, 0, len(valsList))
	for _, values := range valsList {
		next := tracker.normalizeLinkTrackerValues(nil, values)
		if strings.TrimSpace(stringValue(next["url"])) == "" {
			return nil, fmt.Errorf("link.tracker requires url")
		}
		if err := validateLinkTrackerURLRecord(stringValue(next["url"])); err != nil {
			return nil, err
		}
		for _, fieldName := range linkTrackerUniqueFields() {
			if _, ok := next[fieldName]; !ok || isZeroLinkTrackerValue(next[fieldName]) {
				next[fieldName] = linkTrackerDefaultValue(fieldName)
			}
		}
		normalized = append(normalized, next)
	}
	keyToID := map[string]int64{}
	for _, row := range tracker.rows("link.tracker") {
		key := linkTrackerSearchKey(row)
		if key != "" && keyToID[key] == 0 {
			keyToID[key] = numericID(row["id"])
		}
	}
	for _, values := range normalized {
		key := linkTrackerSearchKey(values)
		if key == "" || keyToID[key] != 0 {
			continue
		}
		id, err := tracker.Create(values)
		if err != nil {
			return nil, err
		}
		keyToID[key] = id
	}
	ids := make([]int64, 0, len(normalized))
	for _, values := range normalized {
		id := keyToID[linkTrackerSearchKey(values)]
		if id == 0 {
			return nil, fmt.Errorf("link.tracker search_or_create failed for %s", stringValue(values["url"]))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func linkTrackerUniqueFields() []string {
	return []string{"url", "campaign_id", "medium_id", "source_id", "label"}
}

func linkTrackerSearchKey(row map[string]any) string {
	if row == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(stringValue(row["url"])),
		strconv.FormatInt(numericID(row["campaign_id"]), 10),
		strconv.FormatInt(numericID(row["medium_id"]), 10),
		strconv.FormatInt(numericID(row["source_id"]), 10),
		strings.TrimSpace(stringValue(row["label"])),
	}, "\x00")
}

func isZeroLinkTrackerValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	default:
		return numericID(value) == 0
	}
}

func linkTrackerDefaultValue(fieldName string) any {
	if fieldName == "label" {
		return ""
	}
	return int64(0)
}

func (m ModelSet) normalizeCreateValues(values map[string]any) map[string]any {
	switch m.model.Name {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return m.normalizeActionCreateValues(values)
	case "account.account":
		return m.normalizeAccountAccountValues(nil, values)
	case "account.move.line":
		return m.normalizeAccountMoveLineValues(nil, values)
	case "account.lock_exception":
		return m.normalizeLockExceptionValues(nil, values)
	case "mail.activity":
		return m.normalizeMailActivityValues(nil, values)
	case "mail.activity.type":
		return m.normalizeMailActivityTypeValues(nil, values)
	case "mail.alias":
		return m.normalizeMailAliasValues(nil, values)
	case "mail.alias.domain":
		return m.normalizeMailAliasDomainValues(nil, values)
	case "mail.blacklist":
		return m.normalizeMailBlacklistValues(nil, values)
	case "mail.mail":
		return m.normalizeMailMailValues(nil, values)
	case "mailing.contact":
		return m.normalizeMailingContactValues(nil, values)
	case "mailing.subscription":
		return m.normalizeMailingSubscriptionValues(nil, values)
	case "mailing.mailing":
		return m.normalizeMailingMailingValues(nil, values)
	case "utm.campaign":
		return m.normalizeUTMCampaignValues(nil, values)
	case "mailing.trace":
		return m.normalizeMailingTraceValues(nil, values)
	case "link.tracker":
		return m.normalizeLinkTrackerValues(nil, values)
	case "sms.sms":
		return m.normalizeSMSSMSValues(nil, values)
	case "sms.tracker":
		return m.normalizeSMSTrackerValues(nil, values)
	case "whatsapp.template":
		return m.normalizeWhatsAppTemplateValues(nil, values)
	case "whatsapp.message", "whatsapp.template.button":
		return m.normalizeWhatsAppTemplateAliasValues(nil, values)
	case "ir.module.category":
		return m.normalizeModuleCategoryValues(values)
	case "ir.model.data":
		return m.normalizeIrModelDataValues(nil, values)
	case "res.users":
		return m.normalizeResUsersValues(nil, values)
	case "res.groups":
		return m.normalizeResGroupsValues(nil, values)
	case "res.groups.privilege":
		return m.normalizeResGroupsPrivilegeValues(nil, values)
	case "delegation":
		return m.normalizeDelegationValues(nil, m.normalizeSequencedNameCreateValues(values))
	case "delegation.line":
		return m.normalizeDelegationLineValues(nil, values)
	case "cancellation.record":
		return m.normalizeSequencedNameCreateValues(values)
	default:
		if m.inheritsModel("name.sequence.mixin") {
			return m.normalizeMixinSequencedNameCreateValues(values)
		}
		return values
	}
}

func (m ModelSet) normalizeActionCreateValues(values map[string]any) map[string]any {
	out := copyValues(values)
	setDefault := func(name string, value any) {
		if _, exists := out[name]; exists {
			return
		}
		if _, ok := m.model.Fields[name]; !ok {
			return
		}
		out[name] = value
	}
	setDefault("binding_view_types", "list,form")
	switch m.model.Name {
	case "ir.actions.actions":
		setDefault("binding_type", "action")
	case "ir.actions.act_window":
		setDefault("type", "ir.actions.act_window")
		setDefault("binding_type", "action")
		setDefault("context", "{}")
		setDefault("target", "current")
		setDefault("view_mode", "list,form")
		setDefault("mobile_view_mode", "kanban")
		setDefault("limit", int64(80))
		setDefault("filter", false)
		setDefault("cache", true)
		setDefault("close_on_report_download", false)
	case "ir.actions.act_window_close":
		setDefault("type", "ir.actions.act_window_close")
		setDefault("binding_type", "action")
	case "ir.actions.act_url":
		setDefault("type", "ir.actions.act_url")
		setDefault("binding_type", "action")
		setDefault("target", "new")
		setDefault("close", false)
	case "ir.actions.server":
		out = m.normalizeServerActionCreateValues(out)
		setDefault("type", "ir.actions.server")
		setDefault("binding_type", "action")
		setDefault("binding_view_types", "list,form")
		setDefault("active", true)
		setDefault("usage", "ir_actions_server")
		setDefault("sequence", int64(5))
		setDefault("evaluation_type", "value")
		setDefault("update_m2m_operation", "add")
		setDefault("update_boolean_value", "true")
		if stringValue(out["state"]) == "sms" {
			setDefault("sms_method", "sms")
		}
	case "ir.actions.report":
		setDefault("type", "ir.actions.report")
		setDefault("binding_type", "report")
		setDefault("binding_view_types", "list,form")
		setDefault("report_type", "qweb-pdf")
		setDefault("close_on_report_download", false)
	case "ir.actions.client":
		setDefault("type", "ir.actions.client")
		setDefault("binding_type", "action")
		setDefault("target", "current")
		setDefault("context", "{}")
	}
	return out
}

func (m ModelSet) normalizeModuleCategoryValues(values map[string]any) map[string]any {
	out := copyValues(values)
	if _, ok := out["visible"]; !ok && m.fieldExists("ir.module.category", "visible") {
		out["visible"] = true
	}
	if _, ok := out["exclusive"]; !ok && m.fieldExists("ir.module.category", "exclusive") {
		out["exclusive"] = false
	}
	return out
}

func (m ModelSet) inheritsModel(name string) bool {
	for _, inherited := range m.model.Inherit {
		if inherited == name {
			return true
		}
	}
	return false
}

func (m ModelSet) normalizeServerActionCreateValues(values map[string]any) map[string]any {
	parentID := numericID(values["parent_id"])
	if parentID == 0 {
		return values
	}
	parent, ok := m.store.records[parentID]
	if !ok {
		return values
	}
	out := copyValues(values)
	if value, ok := parent["model_id"]; ok {
		out["model_id"] = value
	}
	if value, ok := parent["model_name"]; ok {
		out["model_name"] = value
	}
	if value, ok := parent["group_ids"]; ok {
		out["group_ids"] = cloneValue(value)
	}
	return out
}

func (m ModelSet) normalizeAccountAccountValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	code := stringValue(out["code"])
	if code == "" {
		if splitCode, splitName, ok := coreaccounting.SplitAccountCodeName(stringValue(out["name"])); ok {
			out["code"] = splitCode
			out["name"] = splitName
			code = splitCode
		}
	}
	if code == "" && existing != nil {
		code = stringValue(existing["code"])
	}
	accountType := coreaccounting.AccountKind(stringValue(out["account_type"]))
	if accountType == "" && existing != nil {
		accountType = coreaccounting.AccountKind(stringValue(existing["account_type"]))
	}
	if accountType == coreaccounting.AccountOffBalance {
		out["tax_ids"] = []int64{}
	}
	companyID := numericID(out["company_id"])
	if companyID == 0 && existing != nil {
		companyID = numericID(existing["company_id"])
	}
	classification := coreaccounting.ClassifyAccount(code, m.accountGroups(), companyID)
	out["placeholder_code"] = classification.PlaceholderCode
	out["root_id"] = classification.RootID
	out["group_id"] = classification.GroupID
	return out
}

func (m ModelSet) accountGroups() []coreaccounting.AccountGroup {
	groupStore, ok := m.env.stores["account.group"]
	if !ok {
		return nil
	}
	groups := make([]coreaccounting.AccountGroup, 0, len(groupStore.records))
	for _, row := range groupStore.records {
		groups = append(groups, coreaccounting.AccountGroup{
			ID:              numericID(row["id"]),
			ParentID:        numericID(row["parent_id"]),
			ParentPath:      stringValue(row["parent_path"]),
			Name:            stringValue(row["name"]),
			CodePrefixStart: stringValue(row["code_prefix_start"]),
			CodePrefixEnd:   stringValue(row["code_prefix_end"]),
			CompanyID:       numericID(row["company_id"]),
		})
	}
	return groups
}

func (m ModelSet) normalizeAccountMoveLineValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	merged := copyValues(existing)
	for key, value := range out {
		merged[key] = value
	}
	moveID := numericID(merged["move_id"])
	if moveID == 0 {
		return out
	}
	move := m.rowByID("account.move", moveID)
	if move == nil {
		return out
	}
	moveType := stringValue(move["move_type"])
	if _, ok := m.model.Fields["parent_state"]; ok {
		if _, explicit := out["parent_state"]; !explicit {
			out["parent_state"] = move["state"]
		}
	}
	if _, ok := m.model.Fields["move_type"]; ok {
		if _, explicit := out["move_type"]; !explicit {
			out["move_type"] = moveType
		}
	}
	companyID := numericID(firstNonZero(merged["company_id"], move["company_id"]))
	fiscalPosition := m.fiscalPosition(numericID(move["fiscal_position_id"]))
	accountID := numericID(merged["account_id"])
	accountRow := m.rowByID("account.account", accountID)
	currentAccount := coreaccounting.Account{ID: accountID, Kind: coreaccounting.AccountKind(stringValue(firstNonEmpty(merged["account_type"], accountRow["account_type"])))}
	paymentTermAccount := coreaccounting.Account{}
	if currentAccount.Kind == coreaccounting.AccountReceivable || currentAccount.Kind == coreaccounting.AccountPayable {
		paymentTermAccount = currentAccount
	}
	product := m.productFiscalAccounts(numericID(merged["product_id"]))
	prepared := coreaccounting.PrepareInvoiceLine(coreaccounting.InvoiceLinePreparation{
		MoveType:           moveType,
		CompanyID:          companyID,
		CurrentAccount:     currentAccount,
		PaymentTermAccount: paymentTermAccount,
		Product:            product,
		AccountTaxIDs:      m.filteredTaxIDs(int64Values(accountRow["tax_ids"]), companyID),
		FiscalPosition:     fiscalPosition,
	})
	prepared.TaxIDs = m.filteredTaxIDs(prepared.TaxIDs, companyID)
	if prepared.Account.ID != 0 {
		out["account_id"] = prepared.Account.ID
	}
	out["tax_ids"] = prepared.TaxIDs
	return out
}

func (m ModelSet) fiscalPosition(id int64) coreaccounting.FiscalPosition {
	if id == 0 {
		return coreaccounting.FiscalPosition{}
	}
	fp := coreaccounting.FiscalPosition{ID: id}
	if row := m.rowByID("account.fiscal.position", id); row != nil {
		fp.CompanyID = numericID(row["company_id"])
	}
	for _, row := range m.rows("account.fiscal.position.account") {
		if numericID(row["position_id"]) != id {
			continue
		}
		fp.AccountLines = append(fp.AccountLines, coreaccounting.FiscalPositionAccountLine{
			ID:                   numericID(row["id"]),
			PositionID:           numericID(row["position_id"]),
			CompanyID:            numericID(row["company_id"]),
			SourceAccountID:      numericID(row["account_src_id"]),
			DestinationAccountID: numericID(row["account_dest_id"]),
		})
	}
	fp.TaxMappings = map[int64][]int64{}
	for _, row := range m.rows("account.tax") {
		if !idInSlice(id, int64Values(row["fiscal_position_ids"])) {
			continue
		}
		destID := numericID(row["id"])
		for _, sourceID := range int64Values(row["original_tax_ids"]) {
			fp.TaxMappings[sourceID] = append(fp.TaxMappings[sourceID], destID)
		}
	}
	return fp
}

func (m ModelSet) productFiscalAccounts(productID int64) coreaccounting.ProductFiscalAccounts {
	if productID == 0 {
		return coreaccounting.ProductFiscalAccounts{}
	}
	product := m.rowByID("product.product", productID)
	if product == nil {
		return coreaccounting.ProductFiscalAccounts{}
	}
	template := m.rowByID("product.template", numericID(product["product_tmpl_id"]))
	categoryID := numericID(firstNonZero(product["categ_id"], template["categ_id"]))
	category := m.rowByID("product.category", categoryID)
	incomeID := numericID(firstNonZero(template["property_account_income_id"], category["property_account_income_categ_id"]))
	expenseID := numericID(firstNonZero(template["property_account_expense_id"], category["property_account_expense_categ_id"]))
	customerTaxes := int64Values(product["taxes_id"])
	if len(customerTaxes) == 0 {
		customerTaxes = int64Values(template["taxes_id"])
	}
	supplierTaxes := int64Values(product["supplier_taxes_id"])
	if len(supplierTaxes) == 0 {
		supplierTaxes = int64Values(template["supplier_taxes_id"])
	}
	companyID := numericID(firstNonZero(product["company_id"], template["company_id"]))
	return coreaccounting.ProductFiscalAccounts{
		IncomeAccountID:  incomeID,
		ExpenseAccountID: expenseID,
		CustomerTaxIDs:   m.filteredTaxIDs(customerTaxes, companyID),
		SupplierTaxIDs:   m.filteredTaxIDs(supplierTaxes, companyID),
	}
}

func (m ModelSet) filteredTaxIDs(ids []int64, companyID int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		row := m.rowByID("account.tax", id)
		if row == nil {
			out = append(out, id)
			continue
		}
		taxCompanyID := numericID(row["company_id"])
		if companyID == 0 || taxCompanyID == 0 || taxCompanyID == companyID {
			out = append(out, id)
		}
	}
	return out
}

func (m ModelSet) rowByID(modelName string, id int64) map[string]any {
	if id == 0 {
		return nil
	}
	store, ok := m.env.stores[modelName]
	if !ok {
		return nil
	}
	return store.records[id]
}

func (m ModelSet) rows(modelName string) []map[string]any {
	store, ok := m.env.stores[modelName]
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(store.records))
	for _, row := range store.records {
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return numericID(out[i]["id"]) < numericID(out[j]["id"])
	})
	return out
}

func (m ModelSet) snapshotStore(modelName string) storeSnapshot {
	st, ok := m.env.stores[modelName]
	if !ok {
		return storeSnapshot{}
	}
	snapshot := storeSnapshot{exists: true, nextID: st.nextID, records: make(map[int64]map[string]any, len(st.records))}
	for id, row := range st.records {
		snapshot.records[id] = copyValues(row)
	}
	return snapshot
}

func (m ModelSet) restoreStore(modelName string, snapshot storeSnapshot) {
	if !snapshot.exists {
		delete(m.env.stores, modelName)
		return
	}
	st, ok := m.env.stores[modelName]
	if !ok {
		st = &store{}
		m.env.stores[modelName] = st
	}
	st.nextID = snapshot.nextID
	st.records = make(map[int64]map[string]any, len(snapshot.records))
	for id, row := range snapshot.records {
		st.records[id] = copyValues(row)
	}
}

func (m ModelSet) snapshotEnv() map[string]storeSnapshot {
	out := make(map[string]storeSnapshot, len(m.env.stores))
	for modelName := range m.env.stores {
		out[modelName] = m.snapshotStore(modelName)
	}
	return out
}

func (m ModelSet) restoreEnv(snapshot map[string]storeSnapshot) {
	for modelName := range m.env.stores {
		if _, ok := snapshot[modelName]; !ok {
			delete(m.env.stores, modelName)
		}
	}
	for modelName, storeSnapshot := range snapshot {
		m.restoreStore(modelName, storeSnapshot)
	}
}

func (m ModelSet) syncFetchmailGatewayCron() error {
	if m.env == nil || fetchmailCronRunning(m.env.context) {
		return nil
	}
	if m.configParameterBool("database.is_neutralized") {
		return nil
	}
	cronID, err := m.fetchmailGatewayCronID()
	if err != nil || cronID == 0 {
		return err
	}
	active, err := m.hasEligibleFetchmailServer()
	if err != nil {
		return err
	}
	return m.env.Model("ir.cron").Browse(cronID).Write(map[string]any{"active": active})
}

func (m ModelSet) fetchmailGatewayCronID() (int64, error) {
	if _, ok := m.env.ModelMetadata("ir.model.data"); ok {
		found, err := m.env.Model("ir.model.data").Search(domain.Cond("complete_name", "=", "mail.ir_cron_mail_gateway_action"))
		if err != nil {
			return 0, err
		}
		rows, err := found.Read("model", "res_id")
		if err != nil {
			return 0, err
		}
		for _, row := range rows {
			if stringValue(row["model"]) == "ir.cron" {
				if id := numericID(row["res_id"]); id != 0 {
					return id, nil
				}
			}
		}
	}
	if _, ok := m.env.ModelMetadata("ir.cron"); !ok {
		return 0, nil
	}
	found, err := m.env.Model("ir.cron").Search(domain.Cond("action_name", "=", "mail.fetchmail"))
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return numericID(rows[0]["id"]), nil
}

func (m ModelSet) hasEligibleFetchmailServer() (bool, error) {
	if _, ok := m.env.ModelMetadata("fetchmail.server"); !ok {
		return false, nil
	}
	found, err := m.env.Model("fetchmail.server").Search(domain.And(
		domain.Cond("active", "=", true),
		domain.Cond("state", "=", "done"),
		domain.Cond("server_type", "!=", "local"),
	))
	if err != nil {
		return false, err
	}
	return found.Len() > 0, nil
}

func fetchmailCronRunning(context Context) bool {
	if context.Values == nil {
		return false
	}
	return truthyRecordValue(context.Values["fetchmail_cron_running"])
}

func (m ModelSet) normalizeSequencedNameCreateValues(values map[string]any) map[string]any {
	value, ok := m.env.nextSimpleSequenceByCode(m.model.Name)
	if !ok {
		return values
	}
	out := copyValues(values)
	out["name"] = value
	return out
}

func (m ModelSet) normalizeMixinSequencedNameCreateValues(values map[string]any) map[string]any {
	name, ok := values["name"]
	if ok {
		text := strings.TrimSpace(fmt.Sprint(name))
		if text != "" && text != "New" && text != "<nil>" {
			return values
		}
	}
	value, sequenceOK := m.env.nextSimpleSequenceByCode(m.model.Name)
	if !sequenceOK {
		return values
	}
	out := copyValues(values)
	out["name"] = value
	return out
}

func (e *Env) nextSimpleSequenceByCode(code string) (string, bool) {
	sequenceSet := e.Model("ir.sequence")
	if sequenceSet.err != nil {
		return "", false
	}
	found, err := sequenceSet.Search(domain.Cond("code", domain.Equal, code))
	if err != nil {
		return "", false
	}
	rows, err := found.Read("name", "code", "prefix", "suffix", "padding", "number_next", "number_increment", "company_id", "active", "implementation")
	if err != nil || len(rows) == 0 {
		return "", false
	}
	companyID := e.context.CompanyID
	sort.SliceStable(rows, func(i, j int) bool {
		leftCompany := numericID(rows[i]["company_id"])
		rightCompany := numericID(rows[j]["company_id"])
		if leftCompany == companyID && rightCompany != companyID {
			return true
		}
		if rightCompany == companyID && leftCompany != companyID {
			return false
		}
		if leftCompany == 0 && rightCompany != 0 {
			return true
		}
		if rightCompany == 0 && leftCompany != 0 {
			return false
		}
		return numericID(rows[i]["id"]) < numericID(rows[j]["id"])
	})
	for _, row := range rows {
		if active, ok := row["active"]; ok && active != nil && !truthyRecordValue(active) {
			continue
		}
		rowCompanyID := numericID(row["company_id"])
		if rowCompanyID != 0 && rowCompanyID != companyID {
			continue
		}
		number, next, mutateRow, err := sequencecore.NextNumber(
			sequencecore.Key{Namespace: e.SequenceNamespace("ir.sequence"), Model: "ir.sequence", ID: numericID(row["id"])},
			stringValue(row["implementation"]),
			numericID(row["number_next"]),
			numericID(row["number_increment"]),
		)
		if err != nil {
			return "", false
		}
		value := fmt.Sprintf("%s%0*d%s", stringValue(row["prefix"]), int(numericID(row["padding"])), number, stringValue(row["suffix"]))
		if mutateRow {
			if err := sequenceSet.Browse(numericID(row["id"])).Write(map[string]any{"number_next": next, "number_next_actual": next}); err != nil {
				return "", false
			}
		}
		return value, true
	}
	return "", false
}

func truthyRecordValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func copyValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		return append([]any(nil), typed...)
	default:
		return value
	}
}

func numericID(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func floatRecordValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func int64Values(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id := numericID(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	case int64, int, float64:
		if id := numericID(typed); id != 0 {
			return []int64{id}
		}
	case string:
		parts := strings.Split(typed, ",")
		out := make([]int64, 0, len(parts))
		for _, part := range parts {
			var id int64
			_, _ = fmt.Sscanf(strings.TrimSpace(part), "%d", &id)
			if id != 0 {
				out = append(out, id)
			}
		}
		return out
	}
	return nil
}

func idInSlice(id int64, ids []int64) bool {
	for _, item := range ids {
		if item == id {
			return true
		}
	}
	return false
}

func firstNonZero(values ...any) any {
	for _, value := range values {
		if numericID(value) != 0 {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(stringValue(value)) != "" {
			return value
		}
	}
	return nil
}

func (m ModelSet) validateModelConstraints(row map[string]any) error {
	switch m.model.Name {
	case "ir.actions.server":
		for _, current := range m.serverActionConstraintRows(row) {
			if err := m.validateServerActionChildren(current); err != nil {
				return err
			}
		}
	case "account.account":
		if err := m.validateAccountAccount(row); err != nil {
			return err
		}
	case "account.lock_exception":
		if err := m.validateLockException(row); err != nil {
			return err
		}
	case "ir.sequence.date_range":
		if err := m.validateSequenceDateRange(row); err != nil {
			return err
		}
	case "mail.message.reaction":
		if err := m.validateMailMessageReaction(row); err != nil {
			return err
		}
	case "mail.inbound.message.lock":
		if err := m.validateMailInboundMessageLockCreate(row); err != nil {
			return err
		}
	case "res.groups":
		if err := m.validateResGroups(row); err != nil {
			return err
		}
	case "res.users":
		if err := m.validateResUsers(row); err != nil {
			return err
		}
	case "ir.model.data":
		if err := m.validateIrModelData(row); err != nil {
			return err
		}
	case "delegation.line":
		if err := m.validateDelegationLine(row); err != nil {
			return err
		}
	case "mail.alias":
		if err := m.validateMailAlias(row); err != nil {
			return err
		}
		if err := m.validateMailAliasDomainCompany(row); err != nil {
			return err
		}
	case "mail.alias.domain":
		if err := m.validateMailAliasDomain(row); err != nil {
			return err
		}
	case "mail.blacklist":
		if err := m.validateMailBlacklist(row); err != nil {
			return err
		}
	case "phone.blacklist":
		if err := m.validatePhoneBlacklist(row); err != nil {
			return err
		}
	case "mailing.trace":
		if err := m.validateMailingTrace(row); err != nil {
			return err
		}
	case "mailing.subscription":
		if err := m.validateMailingSubscription(row); err != nil {
			return err
		}
	case "sms.sms":
		if err := m.validateSMSSMS(row); err != nil {
			return err
		}
	case "sms.tracker":
		if err := m.validateSMSTracker(row); err != nil {
			return err
		}
	case "whatsapp.template":
		if err := m.validateWhatsAppTemplate(row); err != nil {
			return err
		}
	case "whatsapp.template.variable":
		if err := m.validateWhatsAppTemplateVariable(row); err != nil {
			return err
		}
	case "whatsapp.template.button":
		if err := m.validateWhatsAppTemplateButton(row); err != nil {
			return err
		}
	}
	return nil
}

func (m ModelSet) validateCreateConstraints(row map[string]any) error {
	switch m.model.Name {
	case "account.move":
		return m.validateAccountMoveCreate(row)
	case "mail.mail":
		return m.validateMailMailForcedServer(row)
	case "mail.inbound.message.lock":
		return m.validateMailInboundMessageLockCreate(row)
	default:
		return nil
	}
}

func (m ModelSet) validateMailInboundMessageLockCreate(row map[string]any) error {
	messageID := strings.TrimSpace(stringValue(row["message_id"]))
	if messageID == "" {
		return fmt.Errorf("mail.inbound.message.lock requires message_id")
	}
	for id, existing := range m.store.records {
		if id == numericID(row["id"]) {
			continue
		}
		if strings.TrimSpace(stringValue(existing["message_id"])) == messageID {
			return fmt.Errorf("mail.inbound.message.lock duplicate message_id")
		}
	}
	return nil
}

func (m ModelSet) normalizeMailMailValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	templateID := numericID(out["template_id"])
	delete(out, "template_id")
	if existing == nil && templateID != 0 {
		m.addDelegationEmailsToMailValues(out, templateID, time.Now().UTC())
	}
	return out
}

func (m ModelSet) addDelegationEmailsToMailValues(values map[string]any, templateID int64, at time.Time) {
	groupIDs := m.mailTemplateDelegationGroupIDs(templateID)
	if len(groupIDs) == 0 {
		return
	}
	employeeIDs := m.mailDelegationRecipientEmployeeIDs(values)
	if len(employeeIDs) == 0 {
		return
	}
	delegationIDs := m.activeDelegationIDsForEmployees(employeeIDs, at)
	if len(delegationIDs) == 0 {
		return
	}
	delegateEmployeeIDs := m.delegationLineEmployeeIDs(delegationIDs, groupIDs)
	if len(delegateEmployeeIDs) == 0 {
		return
	}
	cc := splitMailRecipients(stringValue(values["email_cc"]))
	cc = append(cc, m.employeeWorkEmails(delegateEmployeeIDs)...)
	if len(cc) == 0 {
		return
	}
	values["email_cc"] = strings.Join(uniqueRecordStrings(cc), "; ")
}

func (m ModelSet) mailTemplateDelegationGroupIDs(templateID int64) []int64 {
	template := m.rowByID("mail.template", templateID)
	if template == nil {
		return nil
	}
	return uniqueSortedRecordIDs(int64Values(template["delegation_group_ids"]))
}

func (m ModelSet) mailDelegationRecipientEmployeeIDs(values map[string]any) []int64 {
	seen := map[int64]bool{}
	add := func(id int64) {
		if id != 0 {
			seen[id] = true
		}
	}
	emails := splitMailRecipients(stringValue(values["email_to"]) + ";" + stringValue(values["email_cc"]))
	emailSet := map[string]bool{}
	for _, email := range emails {
		emailSet[email] = true
	}
	if len(emailSet) > 0 {
		for _, row := range m.rows("hr.employee") {
			if emailSet[strings.TrimSpace(stringValue(row["work_email"]))] {
				add(numericID(row["id"]))
			}
		}
	}
	for _, id := range m.mailDelegationEmployeeIDsByPartnerIDs(mailRecipientPartnerIDs(values["recipient_ids"])) {
		add(id)
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func mailRecipientPartnerIDs(value any) []int64 {
	switch typed := value.(type) {
	case []int64, int64, int, float64, string:
		return uniqueSortedRecordIDs(int64Values(typed))
	case []any:
		if len(typed) == 0 {
			return nil
		}
		if x2ManyCommandItems(typed) {
			ids := []int64{}
			for _, item := range typed {
				command := item.([]any)
				switch numericID(command[0]) {
				case 4:
					if len(command) > 1 {
						ids = appendUniqueID(ids, numericID(command[1]))
					}
				case 6:
					if len(command) > 2 {
						ids = uniqueRecordIDs(append(ids, int64Values(command[2])...))
					}
				}
			}
			return uniqueSortedRecordIDs(ids)
		}
		return uniqueSortedRecordIDs(int64Values(typed))
	default:
		return nil
	}
}

func (m ModelSet) mailDelegationEmployeeIDsByPartnerIDs(partnerIDs []int64) []int64 {
	if len(partnerIDs) == 0 {
		return nil
	}
	partnerSet := map[int64]bool{}
	for _, id := range partnerIDs {
		partnerSet[id] = true
	}
	userIDs := []int64{}
	employeeIDs := []int64{}
	for _, user := range m.rows("res.users") {
		if !partnerSet[numericID(user["partner_id"])] {
			continue
		}
		userID := numericID(user["id"])
		userIDs = appendUniqueID(userIDs, userID)
		employeeIDs = append(employeeIDs, int64Values(user["employee_ids"])...)
		if id := numericID(user["employee_id"]); id != 0 {
			employeeIDs = append(employeeIDs, id)
		}
	}
	if len(userIDs) > 0 {
		userSet := map[int64]bool{}
		for _, id := range userIDs {
			userSet[id] = true
		}
		for _, employee := range m.rows("hr.employee") {
			if userSet[numericID(employee["user_id"])] {
				employeeIDs = append(employeeIDs, numericID(employee["id"]))
			}
		}
	}
	return uniqueSortedRecordIDs(employeeIDs)
}

func (m ModelSet) activeDelegationIDsForEmployees(employeeIDs []int64, at time.Time) []int64 {
	if len(employeeIDs) == 0 {
		return nil
	}
	employeeSet := map[int64]bool{}
	for _, id := range employeeIDs {
		employeeSet[id] = true
	}
	today := dateOnlyTime(at)
	var ids []int64
	for _, row := range m.rows("delegation") {
		if !employeeSet[numericID(row["employee_id"])] || stringValue(row["state"]) != "confirmed" {
			continue
		}
		if !dateInRange(today, dateOnlyTime(recordDateValue(row["date_from"])), dateOnlyTime(recordDateValue(row["date_to"]))) {
			continue
		}
		ids = append(ids, numericID(row["id"]))
	}
	return uniqueSortedRecordIDs(ids)
}

func (m ModelSet) delegationLineEmployeeIDs(delegationIDs []int64, groupIDs []int64) []int64 {
	if len(delegationIDs) == 0 || len(groupIDs) == 0 {
		return nil
	}
	delegationSet := map[int64]bool{}
	for _, id := range delegationIDs {
		delegationSet[id] = true
	}
	groupSet := map[int64]bool{}
	for _, id := range groupIDs {
		groupSet[id] = true
	}
	var ids []int64
	for _, row := range m.rows("delegation.line") {
		if !delegationSet[numericID(row["delegation_id"])] || !groupSet[numericID(row["group_id"])] {
			continue
		}
		if row["active"] == false {
			continue
		}
		if id := numericID(row["employee_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return uniqueSortedRecordIDs(ids)
}

func (m ModelSet) employeeWorkEmails(employeeIDs []int64) []string {
	if len(employeeIDs) == 0 {
		return nil
	}
	employeeSet := map[int64]bool{}
	for _, id := range employeeIDs {
		employeeSet[id] = true
	}
	var emails []string
	for _, row := range m.rows("hr.employee") {
		if !employeeSet[numericID(row["id"])] {
			continue
		}
		if email := strings.TrimSpace(stringValue(row["work_email"])); email != "" {
			emails = append(emails, email)
		}
	}
	return uniqueRecordStrings(emails)
}

func splitMailRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return uniqueRecordStrings(out)
}

func uniqueRecordStrings(values []string) []string {
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

func dateOnlyTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func dateInRange(date time.Time, from time.Time, to time.Time) bool {
	if date.IsZero() || from.IsZero() || to.IsZero() {
		return false
	}
	return !date.Before(from) && !date.After(to)
}

func (m ModelSet) normalizeDelegationValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		if _, ok := out["date_from"]; !ok && m.hasField("date_from") {
			out["date_from"] = time.Now().UTC().Format("2006-01-02")
		}
	}
	if source, ok := out["delegateTo_employee_id"]; ok {
		out["delegate_to_employee_id"] = source
	} else if snake, ok := out["delegate_to_employee_id"]; ok && m.hasField("delegateTo_employee_id") {
		out["delegateTo_employee_id"] = snake
	}
	if employeeID := numericID(firstNonZero(out["employee_id"], safeRowValue(existing, "employee_id"))); employeeID != 0 {
		if userID := m.employeeUserID(employeeID); userID != 0 && m.hasField("user_id") {
			out["user_id"] = userID
		}
	}
	if delegateEmployeeID := numericID(firstNonZero(out["delegate_to_employee_id"], out["delegateTo_employee_id"], safeRowValue(existing, "delegate_to_employee_id"), safeRowValue(existing, "delegateTo_employee_id"))); delegateEmployeeID != 0 {
		if userID := m.employeeUserID(delegateEmployeeID); userID != 0 && m.hasField("delegate_to_user_id") {
			out["delegate_to_user_id"] = userID
		}
	}
	return out
}

func (m ModelSet) normalizeDelegationLineValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	delegationID := numericID(firstNonZero(out["delegation_id"], safeRowValue(existing, "delegation_id")))
	delegationRow := m.rowByID("delegation", delegationID)
	if existing == nil {
		if _, ok := out["active"]; !ok && m.hasField("active") {
			out["active"] = true
		}
	}
	if delegationRow != nil {
		for _, fieldName := range []string{"one_employee", "state", "date_from", "date_to"} {
			if _, ok := out[fieldName]; !ok {
				out[fieldName] = delegationRow[fieldName]
			}
		}
		if _, ok := out["delegator_id"]; !ok {
			out["delegator_id"] = delegationRow["employee_id"]
		}
		if _, ok := out["delegator_user_id"]; !ok {
			out["delegator_user_id"] = delegationRow["user_id"]
		}
		if truthyRecordValue(delegationRow["one_employee"]) && numericID(out["employee_id"]) == 0 {
			out["employee_id"] = firstNonZero(delegationRow["delegateTo_employee_id"], delegationRow["delegate_to_employee_id"])
		}
	}
	if employeeID := numericID(firstNonZero(out["employee_id"], safeRowValue(existing, "employee_id"))); employeeID != 0 {
		if userID := m.employeeUserID(employeeID); userID != 0 && m.hasField("user_id") {
			out["user_id"] = userID
		}
	}
	return out
}

func (m ModelSet) employeeUserID(employeeID int64) int64 {
	if employeeID == 0 {
		return 0
	}
	row := m.rowByID("hr.employee", employeeID)
	if row == nil {
		return 0
	}
	return numericID(row["user_id"])
}

func (m ModelSet) validateDelegationLine(row map[string]any) error {
	if m.store == nil {
		return nil
	}
	rowID := numericID(row["id"])
	delegationID := numericID(row["delegation_id"])
	groupID := numericID(row["group_id"])
	if delegationID == 0 || groupID == 0 {
		return nil
	}
	for id, existing := range m.store.records {
		if id == rowID {
			continue
		}
		if numericID(existing["delegation_id"]) == delegationID && numericID(existing["group_id"]) == groupID {
			return fmt.Errorf("delegation.line role should be unique per delegation")
		}
	}
	return nil
}

func (m ModelSet) normalizeMailBlacklistValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if raw, ok := out["email"]; ok {
		out["email"] = normalizeRecordEmailAddress(stringValue(raw))
	}
	if existing == nil {
		if _, ok := out["active"]; ok || !m.hasField("active") {
			return out
		}
		out["active"] = true
	}
	return out
}

func (m ModelSet) normalizePhoneBlacklistValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if raw, ok := out["number"]; ok {
		out["number"] = m.normalizePhoneNumber(stringValue(raw), m.companyPhoneCountry())
	}
	if existing == nil {
		if _, ok := out["active"]; !ok && m.hasField("active") {
			out["active"] = true
		}
	}
	return out
}

func (m ModelSet) normalizeResPartnerValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if _, phoneTouched := out["phone"]; (phoneTouched || valueExistsRecord(out, "country_id")) && m.hasField("phone_sanitized") {
		phoneValue := out["phone"]
		if !phoneTouched {
			phoneValue = safeRowValue(existing, "phone")
		}
		country := m.phoneCountryFromRecordValues(existing, out)
		phone := m.normalizePhoneNumber(stringValue(phoneValue), country)
		out["phone_sanitized"] = phone
		if m.hasField("phone_blacklisted") {
			out["phone_blacklisted"] = m.phoneNumberBlacklisted(phone)
		}
	}
	if existing == nil {
		if _, ok := out["active"]; !ok && m.hasField("active") {
			out["active"] = true
		}
	}
	return out
}

func (m ModelSet) validateMailBlacklist(row map[string]any) error {
	email := normalizeRecordEmailAddress(stringValue(row["email"]))
	if email == "" {
		return fmt.Errorf("invalid email address")
	}
	found, err := m.env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("email")
	if err != nil {
		return err
	}
	id := numericID(row["id"])
	for _, existing := range rows {
		if numericID(existing["id"]) != id && normalizeRecordEmailAddress(stringValue(existing["email"])) == email {
			return fmt.Errorf("email address already exists")
		}
	}
	return nil
}

func (m ModelSet) validatePhoneBlacklist(row map[string]any) error {
	number := m.normalizePhoneNumber(stringValue(row["number"]), m.companyPhoneCountry())
	if number == "" {
		return fmt.Errorf("invalid phone number")
	}
	found, err := m.env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("number")
	if err != nil {
		return err
	}
	id := numericID(row["id"])
	for _, existing := range rows {
		if numericID(existing["id"]) != id && m.normalizePhoneNumber(stringValue(existing["number"]), m.companyPhoneCountry()) == number {
			return fmt.Errorf("phone number already exists")
		}
	}
	return nil
}

func (m ModelSet) validateSMSSMS(row map[string]any) error {
	uuid := strings.TrimSpace(stringValue(row["uuid"]))
	if uuid == "" {
		return fmt.Errorf("sms.sms uuid is required")
	}
	for _, existing := range m.rows("sms.sms") {
		if numericID(existing["id"]) == numericID(row["id"]) {
			continue
		}
		if strings.TrimSpace(stringValue(existing["uuid"])) == uuid {
			return fmt.Errorf("sms.sms uuid already exists")
		}
	}
	return nil
}

func (m ModelSet) validateSMSTracker(row map[string]any) error {
	uuid := strings.TrimSpace(stringValue(row["sms_uuid"]))
	if uuid == "" {
		return fmt.Errorf("sms.tracker sms_uuid is required")
	}
	for _, existing := range m.rows("sms.tracker") {
		if numericID(existing["id"]) == numericID(row["id"]) {
			continue
		}
		if strings.TrimSpace(stringValue(existing["sms_uuid"])) == uuid {
			return fmt.Errorf("sms.tracker sms_uuid already exists")
		}
	}
	return nil
}

var whatsappTemplateVariableNamePattern = regexp.MustCompile(`^\{\{[1-9][0-9]*\}\}$`)
var whatsappTemplateTextVariablePattern = regexp.MustCompile(`\{\{[1-9][0-9]*\}\}`)

func (m ModelSet) validateWhatsAppTemplate(row map[string]any) error {
	if value := strings.TrimSpace(stringValue(row["status"])); value != "" && !validWhatsAppTemplateStatus(value) {
		return fmt.Errorf("invalid whatsapp template status %q", value)
	}
	if value := strings.TrimSpace(stringValue(row["quality"])); value != "" && !validWhatsAppTemplateQuality(value) {
		return fmt.Errorf("invalid whatsapp template quality %q", value)
	}
	if value := strings.TrimSpace(stringValue(row["template_type"])); value != "" && !validWhatsAppTemplateType(value) {
		return fmt.Errorf("invalid whatsapp template category %q", value)
	}
	headerType := strings.TrimSpace(stringValue(row["header_type"]))
	if headerType != "" && !validWhatsAppHeaderType(headerType) {
		return fmt.Errorf("invalid whatsapp template header type %q", headerType)
	}
	if headerType == "text" {
		matches := whatsappTemplateTextVariablePattern.FindAllString(strings.TrimSpace(stringValue(row["header_text"])), -1)
		if len(matches) > 1 || len(matches) == 1 && matches[0] != "{{1}}" {
			return fmt.Errorf("whatsapp template header text must contain no variable or only {{1}}")
		}
	}
	if err := m.validateWhatsAppTemplateVariableAggregate(row, headerType); err != nil {
		return err
	}
	if err := m.validateWhatsAppTemplateButtonAggregate(row); err != nil {
		return err
	}
	return nil
}

func validWhatsAppTemplateStatus(value string) bool {
	switch value {
	case "draft", "pending", "in_appeal", "approved", "paused", "disabled", "rejected", "pending_deletion", "deleted", "limit_exceeded":
		return true
	default:
		return false
	}
}

func validWhatsAppTemplateQuality(value string) bool {
	switch value {
	case "none", "red", "yellow", "green":
		return true
	default:
		return false
	}
}

func validWhatsAppTemplateType(value string) bool {
	switch value {
	case "authentication", "marketing", "utility":
		return true
	default:
		return false
	}
}

func validWhatsAppHeaderType(value string) bool {
	switch value {
	case "none", "text", "image", "video", "document", "location":
		return true
	default:
		return false
	}
}

func (m ModelSet) validateWhatsAppTemplateButton(row map[string]any) error {
	if value := strings.TrimSpace(stringValue(row["button_type"])); value != "" {
		switch value {
		case "url", "phone_number", "quick_reply":
		default:
			return fmt.Errorf("invalid whatsapp template button type %q", value)
		}
	}
	if value := strings.TrimSpace(stringValue(row["url_type"])); value != "" {
		switch value {
		case "static", "dynamic", "tracked":
		default:
			return fmt.Errorf("invalid whatsapp template URL type %q", value)
		}
	}
	if template := m.whatsAppTemplateForChild(row); template != nil {
		return m.validateWhatsAppTemplateButtonAggregate(template)
	}
	return nil
}

func (m ModelSet) validateWhatsAppTemplateVariableAggregate(row map[string]any, headerType string) error {
	templateID := numericID(row["id"])
	if templateID == 0 {
		return nil
	}
	var bodyIndexes []int
	bodyFreeTextCount := 0
	locationCount := 0
	headerIndexes := 0
	for _, variable := range m.rows("whatsapp.template.variable") {
		if numericID(variable["wa_template_id"]) != templateID || numericID(variable["button_id"]) != 0 {
			continue
		}
		switch strings.TrimSpace(stringValue(variable["line_type"])) {
		case "body":
			if index := whatsappTemplateVariableIndexRecord(variable); index != 0 {
				bodyIndexes = append(bodyIndexes, index)
			}
			if strings.TrimSpace(stringValue(variable["field_type"])) == "free_text" {
				bodyFreeTextCount++
			}
		case "header":
			if index := whatsappTemplateVariableIndexRecord(variable); index != 0 {
				headerIndexes++
				if index != 1 {
					return fmt.Errorf("whatsapp template header variables must use {{1}}")
				}
			}
		case "location":
			locationCount++
		}
	}
	if err := validateWhatsAppContiguousIndexes(bodyIndexes, "body"); err != nil {
		return err
	}
	if bodyFreeTextCount > 10 {
		return fmt.Errorf("whatsapp template body free text variables cannot exceed 10")
	}
	if headerType == "location" {
		if locationCount != 4 {
			return fmt.Errorf("location whatsapp template headers require exactly 4 variables")
		}
	} else if locationCount != 0 {
		return fmt.Errorf("location whatsapp template variables require a location header")
	}
	if headerType != "text" && headerIndexes != 0 {
		return fmt.Errorf("header whatsapp template variables require a text header")
	}
	return nil
}

func (m ModelSet) validateWhatsAppTemplateButtonAggregate(row map[string]any) error {
	templateID := numericID(row["id"])
	if templateID == 0 {
		return nil
	}
	total := 0
	urlCount := 0
	phoneCount := 0
	for _, button := range m.rows("whatsapp.template.button") {
		if numericID(button["wa_template_id"]) != templateID && numericID(button["template_id"]) != templateID {
			continue
		}
		total++
		switch strings.TrimSpace(stringValue(button["button_type"])) {
		case "url":
			urlCount++
		case "phone_number":
			phoneCount++
		}
	}
	if total > 10 {
		return fmt.Errorf("whatsapp templates cannot have more than 10 buttons")
	}
	if urlCount > 2 {
		return fmt.Errorf("whatsapp templates cannot have more than 2 URL buttons")
	}
	if phoneCount > 1 {
		return fmt.Errorf("whatsapp templates cannot have more than 1 phone button")
	}
	return nil
}

func validateWhatsAppContiguousIndexes(indexes []int, label string) error {
	if len(indexes) == 0 {
		return nil
	}
	sort.Ints(indexes)
	for i, index := range indexes {
		expected := i + 1
		if index != expected {
			return fmt.Errorf("whatsapp template %s variables must start at {{1}} and not skip indexes", label)
		}
	}
	return nil
}

func whatsappTemplateVariableIndexRecord(row map[string]any) int {
	name := strings.TrimSpace(stringValue(row["name"]))
	if match := whatsappTemplateVariableNamePattern.FindString(name); match != "" {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		parsed, _ := strconv.Atoi(trimmed)
		return parsed
	}
	return 0
}

func (m ModelSet) validateWhatsAppTemplateVariable(row map[string]any) error {
	lineType := strings.TrimSpace(stringValue(row["line_type"]))
	name := strings.TrimSpace(stringValue(row["name"]))
	if lineType == "" {
		return fmt.Errorf("whatsapp template variable line_type is required")
	}
	switch lineType {
	case "button", "header", "location", "body":
	default:
		return fmt.Errorf("invalid whatsapp template variable line_type %q", lineType)
	}
	fieldType := strings.TrimSpace(stringValue(row["field_type"]))
	if fieldType == "" {
		fieldType = "free_text"
	}
	switch fieldType {
	case "user_name", "user_phone", "free_text", "portal_url", "field":
	default:
		return fmt.Errorf("invalid whatsapp template variable field_type %q", fieldType)
	}
	if fieldType == "free_text" && lineType != "location" && strings.TrimSpace(stringValue(row["demo_value"])) == "" {
		return fmt.Errorf("free text whatsapp template variables require a demo value")
	}
	if fieldType == "field" {
		fieldName := strings.TrimSpace(stringValue(row["field_name"]))
		if fieldName == "" {
			return fmt.Errorf("field whatsapp template variables require a field_name")
		}
		if err := m.validateWhatsAppVariableFieldPath(row, fieldName); err != nil {
			return err
		}
	}
	if lineType == "location" {
		switch name {
		case "name", "address", "latitude", "longitude":
		default:
			return fmt.Errorf("location whatsapp template variable name must be name, address, latitude, or longitude")
		}
	} else if lineType == "button" {
		buttonID := numericID(row["button_id"])
		if buttonID == 0 {
			return fmt.Errorf("button whatsapp template variables must be linked to a button")
		}
		button := m.rowByID("whatsapp.template.button", buttonID)
		if button != nil && strings.TrimSpace(stringValue(button["name"])) != "" && name != strings.TrimSpace(stringValue(button["name"])) {
			return fmt.Errorf("button whatsapp template variable name must match its button name")
		}
	} else if !whatsappTemplateVariableNamePattern.MatchString(name) {
		return fmt.Errorf("whatsapp template variable name must use {{number}} format")
	}
	if err := m.validateWhatsAppTemplateVariableUnique(row); err != nil {
		return err
	}
	if lineType != "location" {
		if template := m.whatsAppTemplateForChild(row); template != nil {
			return m.validateWhatsAppTemplateVariableAggregate(template, strings.TrimSpace(stringValue(template["header_type"])))
		}
	}
	return nil
}

func (m ModelSet) whatsAppTemplateForChild(row map[string]any) map[string]any {
	templateID := numericID(firstNonZero(row["wa_template_id"], row["template_id"]))
	if templateID == 0 {
		return nil
	}
	return m.rowByID("whatsapp.template", templateID)
}

func (m ModelSet) validateWhatsAppVariableFieldPath(row map[string]any, fieldPath string) error {
	modelName := strings.TrimSpace(stringValue(row["model"]))
	if modelName == "" {
		if template := m.rowByID("whatsapp.template", numericID(row["wa_template_id"])); template != nil {
			modelName = strings.TrimSpace(stringValue(template["model"]))
		}
	}
	if modelName == "" || m.env == nil {
		return nil
	}
	parts := strings.Split(fieldPath, ".")
	currentModel := modelName
	for index, part := range parts {
		meta, ok := m.env.ModelMetadata(currentModel)
		if !ok {
			return fmt.Errorf("unknown model %s for whatsapp template variable", currentModel)
		}
		fieldMeta, ok := meta.Fields[strings.TrimSpace(part)]
		if !ok {
			return fmt.Errorf("invalid whatsapp template variable field path %s for model %s", fieldPath, modelName)
		}
		if index < len(parts)-1 {
			if fieldMeta.Relation == "" {
				return fmt.Errorf("invalid whatsapp template variable relation path %s for model %s", fieldPath, modelName)
			}
			currentModel = fieldMeta.Relation
		}
	}
	return nil
}

func (m ModelSet) validateWhatsAppTemplateVariableUnique(row map[string]any) error {
	if m.store == nil {
		return nil
	}
	rowID := numericID(row["id"])
	name := strings.TrimSpace(stringValue(row["name"]))
	lineType := strings.TrimSpace(stringValue(row["line_type"]))
	templateID := numericID(row["wa_template_id"])
	buttonID := numericID(row["button_id"])
	for id, existing := range m.store.records {
		if id == rowID {
			continue
		}
		if strings.TrimSpace(stringValue(existing["name"])) == name &&
			strings.TrimSpace(stringValue(existing["line_type"])) == lineType &&
			numericID(existing["wa_template_id"]) == templateID &&
			numericID(existing["button_id"]) == buttonID {
			return fmt.Errorf("whatsapp template variable names must be unique for a template")
		}
	}
	return nil
}

func (m ModelSet) normalizeMailingContactValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if raw, ok := out["email"]; ok {
		normalized := normalizeRecordEmailAddress(stringValue(raw))
		out["email_normalized"] = normalized
		if strings.TrimSpace(stringValue(out["email"])) == "" {
			out["email"] = normalized
		}
	}
	if raw, ok := out["phone"]; ok && m.hasField("phone_sanitized") {
		out["phone_sanitized"] = m.normalizePhoneNumber(stringValue(raw), m.companyPhoneCountry())
	}
	if existing == nil {
		if _, ok := out["active"]; !ok && m.hasField("active") {
			out["active"] = true
		}
	}
	return out
}

func (m ModelSet) normalizeMailingSubscriptionValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	optOut := truthyRecordValue(safeRowValue(existing, "opt_out"))
	optOutTouched := false
	if existing == nil {
		if _, ok := out["opt_out"]; !ok && m.hasField("opt_out") {
			out["opt_out"] = false
		}
	}
	if raw, ok := out["opt_out"]; ok {
		optOut = truthyRecordValue(raw)
		optOutTouched = true
	}
	if raw, ok := out["opt_out_datetime"]; ok && !recordDateValue(raw).IsZero() {
		out["opt_out"] = true
		optOut = true
		optOutTouched = true
	}
	if raw, ok := out["opt_out_reason_id"]; ok && numericID(raw) != 0 {
		out["opt_out"] = true
		optOut = true
		optOutTouched = true
	}
	if optOutTouched {
		if optOut {
			if recordDateValue(out["opt_out_datetime"]).IsZero() && m.hasField("opt_out_datetime") {
				out["opt_out_datetime"] = time.Now().UTC()
			}
		} else if m.hasField("opt_out_datetime") {
			out["opt_out_datetime"] = time.Time{}
		}
	}
	return out
}

func (m ModelSet) validateMailingSubscription(row map[string]any) error {
	contactID := numericID(row["contact_id"])
	listID := numericID(row["list_id"])
	if contactID == 0 || listID == 0 {
		return fmt.Errorf("mailing.subscription requires contact_id and list_id")
	}
	for _, existing := range m.rows("mailing.subscription") {
		if numericID(existing["id"]) == numericID(row["id"]) {
			continue
		}
		if numericID(existing["contact_id"]) == contactID && numericID(existing["list_id"]) == listID {
			return fmt.Errorf("mailing subscription already exists for contact/list")
		}
	}
	return nil
}

func (m ModelSet) normalizeUTMCampaignValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		if _, ok := out["ab_testing_winner_selection"]; !ok && m.hasField("ab_testing_winner_selection") {
			out["ab_testing_winner_selection"] = "opened_ratio"
		}
		if _, ok := out["ab_testing_completed"]; !ok && m.hasField("ab_testing_completed") {
			out["ab_testing_completed"] = false
		}
	}
	if raw, ok := out["ab_testing_winner_selection"]; ok && strings.TrimSpace(stringValue(raw)) == "" && m.hasField("ab_testing_winner_selection") {
		out["ab_testing_winner_selection"] = "opened_ratio"
	}
	return out
}

func (m ModelSet) normalizeMailingMailingValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		if _, ok := out["state"]; !ok && m.hasField("state") {
			out["state"] = "draft"
		}
		if _, ok := out["schedule_type"]; !ok && m.hasField("schedule_type") {
			out["schedule_type"] = "now"
		}
		if _, ok := out["kpi_mail_required"]; !ok && m.hasField("kpi_mail_required") {
			out["kpi_mail_required"] = false
		}
		if _, ok := out["user_id"]; !ok && m.hasField("user_id") && m.env.context.UserID != 0 {
			out["user_id"] = m.env.context.UserID
		}
		if _, ok := out["use_exclusion_list"]; !ok && m.hasField("use_exclusion_list") {
			out["use_exclusion_list"] = true
		}
		if _, ok := out["ab_testing_enabled"]; !ok && m.hasField("ab_testing_enabled") {
			out["ab_testing_enabled"] = false
		}
		if _, ok := out["ab_testing_pc"]; !ok && m.hasField("ab_testing_pc") {
			out["ab_testing_pc"] = int64(10)
		}
		if _, ok := out["ab_testing_winner_selection"]; !ok && m.hasField("ab_testing_winner_selection") {
			out["ab_testing_winner_selection"] = "opened_ratio"
		}
		if _, ok := out["ab_testing_schedule_datetime"]; !ok && m.hasField("ab_testing_schedule_datetime") {
			out["ab_testing_schedule_datetime"] = time.Now().UTC().Add(24 * time.Hour)
		}
	}
	if raw, ok := out["ab_testing_winner_selection"]; ok && strings.TrimSpace(stringValue(raw)) == "" && m.hasField("ab_testing_winner_selection") {
		out["ab_testing_winner_selection"] = "opened_ratio"
	}
	if raw, ok := out["ab_testing_pc"]; ok && numericID(raw) < 0 && m.hasField("ab_testing_pc") {
		out["ab_testing_pc"] = int64(0)
	}
	return out
}

func (m ModelSet) ensureMailingABTestingCampaign(row map[string]any) error {
	if row == nil || !truthyRecordValue(row["ab_testing_enabled"]) || numericID(row["campaign_id"]) != 0 {
		return nil
	}
	if _, ok := m.env.ModelMetadata("utm.campaign"); !ok {
		return nil
	}
	name := strings.TrimSpace(stringValue(row["subject"]))
	if name == "" {
		name = strings.TrimSpace(stringValue(row["name"]))
	}
	if name == "" {
		name = time.Now().UTC().Format(time.RFC3339)
	}
	values := map[string]any{"name": "A/B Test: " + name}
	if m.fieldExists("utm.campaign", "ab_testing_schedule_datetime") {
		values["ab_testing_schedule_datetime"] = row["ab_testing_schedule_datetime"]
	}
	if m.fieldExists("utm.campaign", "ab_testing_winner_selection") {
		values["ab_testing_winner_selection"] = row["ab_testing_winner_selection"]
	}
	campaignID, err := m.env.Model("utm.campaign").Create(values)
	if err != nil {
		return err
	}
	row["campaign_id"] = campaignID
	return nil
}

func (m ModelSet) normalizeMailingTraceValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		if _, ok := out["trace_type"]; !ok && m.hasField("trace_type") {
			out["trace_type"] = "mail"
		}
		if _, ok := out["trace_status"]; !ok && m.hasField("trace_status") {
			out["trace_status"] = "outgoing"
		}
	}
	if raw, ok := out["email"]; ok {
		out["email"] = normalizeRecordEmailAddress(stringValue(raw))
	}
	if raw, ok := out["mail_mail_id"]; ok && m.hasField("mail_mail_id_int") {
		out["mail_mail_id_int"] = numericID(raw)
	}
	mailingID := numericID(firstNonZero(out["mass_mailing_id"], safeRowValue(existing, "mass_mailing_id")))
	if mailingID != 0 {
		m.applyMailingTraceMailingDefaults(out, mailingID)
	}
	return out
}

func (m ModelSet) normalizeWhatsAppTemplateAliasValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	templateID := numericID(firstNonZero(out["wa_template_id"], out["template_id"], safeRowValue(existing, "wa_template_id"), safeRowValue(existing, "template_id")))
	if templateID == 0 {
		return out
	}
	if _, ok := out["wa_template_id"]; !ok && m.hasField("wa_template_id") {
		out["wa_template_id"] = templateID
	}
	if _, ok := out["template_id"]; !ok && m.hasField("template_id") {
		out["template_id"] = templateID
	}
	return out
}

func (m ModelSet) normalizeWhatsAppTemplateValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	normalizeProviderEnum := func(fieldName string) {
		if raw, ok := out[fieldName]; ok {
			out[fieldName] = normalizeWhatsAppProviderEnum(fieldName, stringValue(raw))
		}
	}
	normalizeProviderEnum("status")
	normalizeProviderEnum("quality")
	normalizeProviderEnum("template_type")
	normalizeProviderEnum("header_type")
	defaults := map[string]any{
		"status":        "draft",
		"quality":       "none",
		"lang_code":     "en",
		"template_type": "marketing",
		"header_type":   "none",
		"phone_field":   "phone",
		"active":        true,
	}
	for fieldName, defaultValue := range defaults {
		if !m.hasField(fieldName) {
			continue
		}
		if _, ok := out[fieldName]; ok {
			continue
		}
		if existing != nil {
			if current, ok := existing[fieldName]; ok && strings.TrimSpace(stringValue(current)) != "" {
				out[fieldName] = current
				continue
			}
		}
		out[fieldName] = defaultValue
	}
	if _, ok := out["model"]; !ok && m.hasField("model") {
		if modelName := m.whatsAppTemplateModelNameFromModelID(firstNonZero(out["model_id"], safeRowValue(existing, "model_id"))); modelName != "" {
			out["model"] = modelName
		} else if existing != nil && strings.TrimSpace(stringValue(existing["model"])) != "" {
			out["model"] = existing["model"]
		} else {
			out["model"] = "res.partner"
		}
	}
	return out
}

func normalizeWhatsAppProviderEnum(fieldName string, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch fieldName {
	case "quality":
		if normalized == "unknown" {
			return "none"
		}
	}
	return normalized
}

func (m ModelSet) whatsAppTemplateModelNameFromModelID(modelIDValue any) string {
	modelID := numericID(modelIDValue)
	if modelID == 0 || m.env == nil {
		return ""
	}
	if _, ok := m.env.ModelMetadata("ir.model"); !ok {
		return ""
	}
	rows, err := m.env.Model("ir.model").Browse(modelID).Read("model")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringValue(rows[0]["model"]))
}

func (m ModelSet) normalizeSMSSMSValues(_ map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if raw, ok := out["uuid"]; ok {
		out["uuid"] = strings.TrimSpace(stringValue(raw))
	}
	if strings.TrimSpace(stringValue(out["uuid"])) == "" && m.hasField("uuid") {
		out["uuid"] = randomSMSRecordUUID()
	}
	if _, ok := out["state"]; !ok && m.hasField("state") {
		out["state"] = "outgoing"
	}
	if _, ok := out["to_delete"]; !ok && m.hasField("to_delete") {
		out["to_delete"] = false
	}
	return out
}

func (m ModelSet) normalizeSMSTrackerValues(_ map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if raw, ok := out["sms_uuid"]; ok {
		out["sms_uuid"] = strings.TrimSpace(stringValue(raw))
	}
	return out
}

func randomSMSRecordUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x", b[:])
}

func (m ModelSet) applyMailingTraceMailingDefaults(out map[string]any, mailingID int64) {
	if _, ok := m.env.ModelMetadata("mailing.mailing"); !ok {
		return
	}
	rows, err := m.env.Model("mailing.mailing").Browse(mailingID).Read("campaign_id", "source_id", "medium_id")
	if err != nil || len(rows) == 0 {
		return
	}
	for _, fieldName := range []string{"campaign_id", "source_id", "medium_id"} {
		if _, exists := out[fieldName]; exists {
			continue
		}
		if !m.hasField(fieldName) {
			continue
		}
		if id := numericID(rows[0][fieldName]); id != 0 {
			out[fieldName] = id
		}
	}
}

func (m ModelSet) validateMailingTrace(row map[string]any) error {
	if numericID(row["res_id"]) == 0 {
		return fmt.Errorf("traces have to be linked to records with a not null res_id")
	}
	return nil
}

func normalizeRecordEmailAddress(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.LastIndex(value, "<"); start >= 0 {
		if end := strings.Index(value[start:], ">"); end > 0 {
			value = value[start+1 : start+end]
		}
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRecordPhoneNumber(value string) string {
	return phone.NormalizeE164(value, phone.Country{})
}

func (m ModelSet) normalizePhoneNumber(value string, country phone.Country) string {
	return phone.NormalizeE164(value, country)
}

func (m ModelSet) phoneNumberBlacklisted(number string) bool {
	number = m.normalizePhoneNumber(number, m.companyPhoneCountry())
	if number == "" {
		return false
	}
	for _, row := range m.rows("phone.blacklist") {
		if truthyRecordValue(row["active"]) && m.normalizePhoneNumber(stringValue(row["number"]), m.companyPhoneCountry()) == number {
			return true
		}
	}
	return false
}

func (m ModelSet) syncAllPhoneBlacklistDerivedFields() {
	for _, modelName := range []string{"res.partner"} {
		if !m.fieldExists(modelName, "phone_sanitized") || !m.fieldExists(modelName, "phone_blacklisted") {
			continue
		}
		for _, row := range m.rows(modelName) {
			row["phone_blacklisted"] = m.phoneNumberBlacklisted(stringValue(row["phone_sanitized"]))
		}
	}
}

func (m ModelSet) phoneCountryFromRecordValues(existing map[string]any, values map[string]any) phone.Country {
	countryID := numericID(firstNonZero(values["country_id"], safeRowValue(existing, "country_id")))
	if countryID == 0 {
		return m.companyPhoneCountry()
	}
	return m.phoneCountryByID(countryID)
}

func (m ModelSet) companyPhoneCountry() phone.Country {
	if m.env == nil || m.env.context.CompanyID == 0 {
		return phone.Country{}
	}
	row := m.rowByID("res.company", m.env.context.CompanyID)
	if row == nil {
		return phone.Country{}
	}
	return m.phoneCountryByID(numericID(row["country_id"]))
}

func (m ModelSet) phoneCountryByID(countryID int64) phone.Country {
	if countryID == 0 {
		return phone.Country{}
	}
	row := m.rowByID("res.country", countryID)
	if row == nil {
		return phone.Country{}
	}
	return phone.Country{
		Code:      strings.ToUpper(strings.TrimSpace(stringValue(row["code"]))),
		PhoneCode: numericID(row["phone_code"]),
	}
}

func valueExistsRecord(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	_, ok := values[key]
	return ok
}

var (
	mailAliasATextPattern       = `[a-zA-Z0-9!#$%&'*+\-/=?^_` + "`" + `{|}~]`
	mailAliasDotAtomTextPattern = regexp.MustCompile(`^` + mailAliasATextPattern + `+(\.` + mailAliasATextPattern + `+)*$`)
	mailAliasUnsafeCharsPattern = regexp.MustCompile(`[^\w!#$%&'*+\-/=?^_` + "`" + `{|}~.]+`)
	mailAliasLeadingDotsPattern = regexp.MustCompile(`^\.+|\.+$`)
	mailAliasRepeatedDotPattern = regexp.MustCompile(`\.+`)
)

func (m ModelSet) normalizeMailAliasValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	aliasName := stringValue(safeRowValue(existing, "alias_name"))
	if raw, ok := out["alias_name"]; ok {
		aliasName = sanitizeMailAliasName(stringValue(raw), false)
		out["alias_name"] = aliasName
	} else if existing == nil {
		aliasName = sanitizeMailAliasName(aliasName, false)
		out["alias_name"] = aliasName
	}
	if existing == nil {
		if _, ok := out["alias_defaults"]; !ok && m.hasField("alias_defaults") {
			out["alias_defaults"] = "{}"
		}
		if _, ok := out["alias_contact"]; !ok && m.hasField("alias_contact") {
			out["alias_contact"] = "everyone"
		}
		if _, ok := out["alias_incoming_local"]; !ok && m.hasField("alias_incoming_local") {
			out["alias_incoming_local"] = false
		}
		if _, ok := out["active"]; !ok && m.hasField("active") {
			out["active"] = true
		}
		if _, ok := out["alias_domain_id"]; !ok && m.hasField("alias_domain_id") {
			if domainID := m.contextCompanyAliasDomainID(); domainID != 0 {
				out["alias_domain_id"] = domainID
			}
		}
	}
	if _, ok := out["alias_contact"]; ok || existing == nil {
		if strings.TrimSpace(stringValue(out["alias_contact"])) == "" && m.hasField("alias_contact") {
			out["alias_contact"] = "everyone"
		}
	}
	resetStatus := existing == nil
	for _, fieldName := range []string{"alias_contact", "alias_defaults", "alias_model_id"} {
		if _, ok := out[fieldName]; ok {
			resetStatus = true
			break
		}
	}
	if resetStatus && m.hasField("alias_status") {
		out["alias_status"] = "not_tested"
	}
	aliasDomainID := numericID(safeRowValue(existing, "alias_domain_id"))
	if raw, ok := out["alias_domain_id"]; ok {
		aliasDomainID = numericID(raw)
	}
	if _, aliasNameChanged := out["alias_name"]; aliasNameChanged || existing == nil || hasAnyKey(out, "alias_domain_id") {
		aliasDomainName := m.mailAliasDomainName(aliasDomainID)
		if m.hasField("alias_domain") {
			out["alias_domain"] = aliasDomainName
		}
		if m.hasField("alias_full_name") {
			switch {
			case aliasName != "" && aliasDomainName != "":
				out["alias_full_name"] = aliasName + "@" + aliasDomainName
			case aliasName != "":
				out["alias_full_name"] = aliasName
			default:
				out["alias_full_name"] = ""
			}
		}
	}
	return out
}

func (m ModelSet) normalizeMailAliasDomainValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	name := strings.TrimSpace(stringValue(safeRowValue(existing, "name")))
	if raw, ok := out["name"]; ok {
		name = strings.TrimSpace(stringValue(raw))
		out["name"] = name
	}
	bounceAlias := stringValue(safeRowValue(existing, "bounce_alias"))
	if raw, ok := out["bounce_alias"]; ok {
		bounceAlias = sanitizeMailAliasName(stringValue(raw), false)
		out["bounce_alias"] = bounceAlias
	} else if existing == nil {
		if bounceAlias == "" {
			bounceAlias = "bounce"
		}
		out["bounce_alias"] = sanitizeMailAliasName(bounceAlias, false)
		bounceAlias = stringValue(out["bounce_alias"])
	}
	catchallAlias := stringValue(safeRowValue(existing, "catchall_alias"))
	if raw, ok := out["catchall_alias"]; ok {
		catchallAlias = sanitizeMailAliasName(stringValue(raw), false)
		out["catchall_alias"] = catchallAlias
	} else if existing == nil {
		if catchallAlias == "" {
			catchallAlias = "catchall"
		}
		out["catchall_alias"] = sanitizeMailAliasName(catchallAlias, false)
		catchallAlias = stringValue(out["catchall_alias"])
	}
	defaultFrom := stringValue(safeRowValue(existing, "default_from"))
	if raw, ok := out["default_from"]; ok {
		defaultFrom = sanitizeMailAliasName(stringValue(raw), true)
		out["default_from"] = defaultFrom
	} else if existing == nil {
		if defaultFrom == "" {
			defaultFrom = "notifications"
		}
		out["default_from"] = sanitizeMailAliasName(defaultFrom, true)
		defaultFrom = stringValue(out["default_from"])
	}
	if _, ok := out["sequence"]; !ok && existing == nil && m.hasField("sequence") {
		out["sequence"] = int64(10)
	}
	if m.hasField("bounce_email") && (existing == nil || hasAnyKey(out, "name", "bounce_alias")) {
		if name != "" && bounceAlias != "" {
			out["bounce_email"] = bounceAlias + "@" + name
		} else {
			out["bounce_email"] = ""
		}
	}
	if m.hasField("catchall_email") && (existing == nil || hasAnyKey(out, "name", "catchall_alias")) {
		if name != "" && catchallAlias != "" {
			out["catchall_email"] = catchallAlias + "@" + name
		} else {
			out["catchall_email"] = ""
		}
	}
	if m.hasField("default_from_email") && (existing == nil || hasAnyKey(out, "name", "default_from")) {
		if defaultFrom == "" {
			out["default_from_email"] = ""
		} else if strings.Contains(defaultFrom, "@") {
			out["default_from_email"] = defaultFrom
		} else if name != "" {
			out["default_from_email"] = defaultFrom + "@" + name
		} else {
			out["default_from_email"] = defaultFrom
		}
	}
	return out
}

func (m ModelSet) validateMailAlias(row map[string]any) error {
	aliasName := strings.TrimSpace(stringValue(row["alias_name"]))
	if aliasName != "" && !mailAliasDotAtomTextPattern.MatchString(aliasName) {
		return fmt.Errorf("you cannot use anything else than unaccented latin characters in the alias address %s", aliasName)
	}
	if err := validateMailAliasDefaults(row["alias_defaults"]); err != nil {
		return err
	}
	if aliasName != "" {
		if err := m.validateMailAliasUnique(row); err != nil {
			return err
		}
		if err := m.validateMailAliasDomainClash(row); err != nil {
			return err
		}
	}
	return nil
}

func (m ModelSet) validateMailAliasDomain(row map[string]any) error {
	name := strings.TrimSpace(stringValue(row["name"]))
	if name == "" || !mailAliasDotAtomTextPattern.MatchString(name) {
		return fmt.Errorf("you cannot use anything else than unaccented latin characters in the domain name %s", name)
	}
	for _, fieldName := range []string{"bounce_alias", "catchall_alias"} {
		value := strings.TrimSpace(stringValue(row[fieldName]))
		if value == "" || !mailAliasDotAtomTextPattern.MatchString(value) {
			return fmt.Errorf("you cannot use anything else than unaccented latin characters in the alias address %s", value)
		}
	}
	if err := m.validateMailAliasDomainUnique(row, "bounce_alias", "bounce_email", "bounce"); err != nil {
		return err
	}
	if err := m.validateMailAliasDomainUnique(row, "catchall_alias", "catchall_email", "catchall"); err != nil {
		return err
	}
	return m.validateMailAliasDomainReservedClash(row)
}

func (m ModelSet) validateMailAliasUnique(row map[string]any) error {
	aliasName := strings.TrimSpace(stringValue(row["alias_name"]))
	aliasDomainID := numericID(row["alias_domain_id"])
	id := numericID(row["id"])
	if aliasName == "" {
		return nil
	}
	if store, ok := m.env.stores["mail.alias"]; ok {
		for otherID, other := range store.records {
			if otherID == id {
				continue
			}
			if strings.TrimSpace(stringValue(other["alias_name"])) == aliasName && numericID(other["alias_domain_id"]) == aliasDomainID {
				return fmt.Errorf("email aliases %s cannot be used on several records at the same time", aliasName)
			}
		}
	}
	return nil
}

func (m ModelSet) validateMailAliasDomainClash(row map[string]any) error {
	aliasName := strings.TrimSpace(stringValue(row["alias_name"]))
	aliasDomainID := numericID(row["alias_domain_id"])
	domainRow := m.rowByID("mail.alias.domain", aliasDomainID)
	if aliasName == "" || domainRow == nil {
		return nil
	}
	if aliasName == strings.TrimSpace(stringValue(domainRow["bounce_alias"])) || aliasName == strings.TrimSpace(stringValue(domainRow["catchall_alias"])) {
		display := aliasName
		if domainName := strings.TrimSpace(stringValue(domainRow["name"])); domainName != "" {
			display += "@" + domainName
		}
		return fmt.Errorf("aliases %s is already used as bounce or catchall address", display)
	}
	return nil
}

func (m ModelSet) validateMailAliasDomainUnique(row map[string]any, aliasField string, emailField string, label string) error {
	name := strings.TrimSpace(stringValue(row["name"]))
	value := strings.TrimSpace(stringValue(row[aliasField]))
	id := numericID(row["id"])
	if name == "" || value == "" {
		return nil
	}
	if store, ok := m.env.stores["mail.alias.domain"]; ok {
		for otherID, other := range store.records {
			if otherID == id {
				continue
			}
			if strings.TrimSpace(stringValue(other["name"])) == name && strings.TrimSpace(stringValue(other[aliasField])) == value {
				email := strings.TrimSpace(stringValue(row[emailField]))
				if email == "" {
					email = value + "@" + name
				}
				return fmt.Errorf("%s alias %s is already used for another domain with same name", titleRecordLabel(label), email)
			}
		}
	}
	return nil
}

func (m ModelSet) validateMailAliasDomainReservedClash(row map[string]any) error {
	name := strings.TrimSpace(stringValue(row["name"]))
	reserved := map[string]string{}
	for _, fieldName := range []string{"bounce_alias", "catchall_alias"} {
		local := strings.TrimSpace(stringValue(row[fieldName]))
		if local != "" && name != "" {
			reserved[local+"@"+name] = local
		}
	}
	if len(reserved) == 0 {
		return nil
	}
	if store, ok := m.env.stores["mail.alias"]; ok {
		for _, alias := range store.records {
			fullName := strings.TrimSpace(stringValue(alias["alias_full_name"]))
			if fullName == "" {
				local := strings.TrimSpace(stringValue(alias["alias_name"]))
				if local != "" {
					if domainName := m.mailAliasDomainName(numericID(alias["alias_domain_id"])); domainName != "" {
						fullName = local + "@" + domainName
					}
				}
			}
			if _, ok := reserved[fullName]; ok {
				return fmt.Errorf("bounce/catchall %q is already used", fullName)
			}
		}
	}
	return nil
}

func validateMailAliasDefaults(value any) error {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		text = "{}"
	}
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
	}
	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return nil
	}
	segments := splitRecordTopLevel(body, ',')
	if len(segments) == 0 {
		return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
	}
	for _, segment := range segments {
		left, right, ok := splitRecordTopLevelPair(segment, ':')
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
			return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
		}
	}
	return nil
}

func splitRecordTopLevel(text string, sep rune) []string {
	var parts []string
	start := 0
	depth := 0
	var quote rune
	escaped := false
	for i, r := range text {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		default:
			if r == sep && depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + len(string(r))
			}
		}
		if depth < 0 {
			return nil
		}
	}
	if quote != 0 || depth != 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(text[start:]))
	return parts
}

func splitRecordTopLevelPair(text string, sep rune) (string, string, bool) {
	depth := 0
	var quote rune
	escaped := false
	for i, r := range text {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		default:
			if r == sep && depth == 0 {
				return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+len(string(r)):]), true
			}
		}
		if depth < 0 {
			return "", "", false
		}
	}
	return "", "", false
}

func validateRecordPythonDictLiteral(text string) error {
	segments := splitRecordTopLevel(text, ',')
	if len(segments) == 0 {
		return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
	}
	for _, segment := range segments {
		left, right, ok := splitRecordTopLevelPair(segment, ':')
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
			return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
		}
	}
	return nil
}

func validateRecordPythonDictLiteralLegacy(text string) error {
	depth := 0
	var quote rune
	escaped := false
	for _, r := range text {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
			}
		case ':':
			if depth == 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("invalid expression, it must be a literal python dictionary definition")
}

func sanitizeMailAliasName(name string, isEmail bool) string {
	sanitizedName := strings.TrimSpace(name)
	rightPart := ""
	if isEmail {
		parts := strings.SplitN(sanitizedName, "@", 2)
		sanitizedName = parts[0]
		if len(parts) == 2 {
			rightPart = strings.ToLower(strings.TrimSpace(parts[1]))
		}
	} else if at := strings.Index(sanitizedName, "@"); at >= 0 {
		sanitizedName = sanitizedName[:at]
	}
	sanitizedName = strings.ToLower(sanitizedName)
	sanitizedName = mailAliasUnsafeCharsPattern.ReplaceAllString(sanitizedName, "-")
	sanitizedName = mailAliasRepeatedDotPattern.ReplaceAllString(sanitizedName, ".")
	sanitizedName = mailAliasLeadingDotsPattern.ReplaceAllString(sanitizedName, "")
	if strings.TrimSpace(sanitizedName) == "" {
		return ""
	}
	if isEmail && rightPart != "" {
		return sanitizedName + "@" + rightPart
	}
	return sanitizedName
}

func (m ModelSet) contextCompanyAliasDomainID() int64 {
	if m.env == nil || m.env.context.CompanyID == 0 {
		return 0
	}
	row := m.rowByID("res.company", m.env.context.CompanyID)
	if row == nil {
		return 0
	}
	return numericID(row["alias_domain_id"])
}

func (m ModelSet) mailAliasDomainName(aliasDomainID int64) string {
	row := m.rowByID("mail.alias.domain", aliasDomainID)
	if row == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(row["name"]))
}

func (m ModelSet) assignFirstMailAliasDomain(aliasDomainID int64) error {
	if aliasDomainID == 0 || m.countRows("mail.alias.domain") != 1 {
		return nil
	}
	if store, ok := m.env.stores["res.company"]; ok {
		for _, row := range store.records {
			if numericID(row["alias_domain_id"]) == 0 {
				row["alias_domain_id"] = aliasDomainID
			}
		}
	}
	if store, ok := m.env.stores["mail.alias"]; ok {
		for _, row := range store.records {
			if numericID(row["alias_domain_id"]) == 0 {
				row["alias_domain_id"] = aliasDomainID
				m.applyMailAliasComputed(row)
				if err := m.validateMailAlias(row); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m ModelSet) recomputeMailAliasesForDomain(aliasDomainID int64) error {
	if aliasDomainID == 0 {
		return nil
	}
	if store, ok := m.env.stores["mail.alias"]; ok {
		for _, row := range store.records {
			if numericID(row["alias_domain_id"]) != aliasDomainID {
				continue
			}
			m.applyMailAliasComputed(row)
			if err := m.validateMailAlias(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m ModelSet) applyMailAliasComputed(row map[string]any) {
	if row == nil {
		return
	}
	aliasName := strings.TrimSpace(stringValue(row["alias_name"]))
	aliasDomainName := m.mailAliasDomainName(numericID(row["alias_domain_id"]))
	if meta, ok := m.env.ModelMetadata("mail.alias"); ok {
		if _, ok := meta.Fields["alias_domain"]; ok {
			row["alias_domain"] = aliasDomainName
		}
		if _, ok := meta.Fields["alias_full_name"]; ok {
			switch {
			case aliasName != "" && aliasDomainName != "":
				row["alias_full_name"] = aliasName + "@" + aliasDomainName
			case aliasName != "":
				row["alias_full_name"] = aliasName
			default:
				row["alias_full_name"] = ""
			}
		}
	}
}

func (m ModelSet) countRows(modelName string) int {
	if store, ok := m.env.stores[modelName]; ok {
		return len(store.records)
	}
	return 0
}

func hasAnyKey(values map[string]any, names ...string) bool {
	for _, name := range names {
		if _, ok := values[name]; ok {
			return true
		}
	}
	return false
}

func titleRecordLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (m ModelSet) validateMailAliasDomainCompany(row map[string]any) error {
	aliasDomainID := numericID(row["alias_domain_id"])
	if aliasDomainID == 0 || len(m.mailAliasDomainCompanyIDs(aliasDomainID)) == 0 {
		return nil
	}
	if ownerCompanyID := m.mailAliasRelatedRecordCompanyID(m.mailAliasParentModel(row), numericID(row["alias_parent_thread_id"])); ownerCompanyID != 0 {
		if companyDomainID := m.companyAliasDomainID(ownerCompanyID); companyDomainID != aliasDomainID {
			return fmt.Errorf("alias domain belongs to another company while the owner document belongs to company %s", m.companyDisplayName(ownerCompanyID))
		}
	}
	if targetCompanyID := m.mailAliasRelatedRecordCompanyID(m.mailAliasTargetModel(row), numericID(row["alias_force_thread_id"])); targetCompanyID != 0 {
		if companyDomainID := m.companyAliasDomainID(targetCompanyID); companyDomainID != aliasDomainID {
			return fmt.Errorf("alias domain belongs to another company while the target document belongs to company %s", m.companyDisplayName(targetCompanyID))
		}
	}
	return nil
}

func (m ModelSet) mailAliasTargetModel(row map[string]any) string {
	if modelName := strings.TrimSpace(stringValue(row["model_name"])); modelName != "" {
		return modelName
	}
	return m.irModelName(numericID(row["alias_model_id"]))
}

func (m ModelSet) mailAliasParentModel(row map[string]any) string {
	return m.irModelName(numericID(row["alias_parent_model_id"]))
}

func (m ModelSet) irModelName(modelID int64) string {
	row := m.rowByID("ir.model", modelID)
	if row == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(row["model"]))
}

func (m ModelSet) mailAliasRelatedRecordCompanyID(modelName string, resID int64) int64 {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || resID == 0 {
		return 0
	}
	meta, ok := m.env.ModelMetadata(modelName)
	if !ok {
		return 0
	}
	if _, ok := meta.Fields["company_id"]; !ok {
		return 0
	}
	row := m.rowByID(modelName, resID)
	if row == nil {
		return 0
	}
	return numericID(row["company_id"])
}

func (m ModelSet) mailAliasDomainCompanyIDs(aliasDomainID int64) []int64 {
	if aliasDomainID == 0 {
		return nil
	}
	seen := map[int64]bool{}
	out := []int64{}
	if row := m.rowByID("mail.alias.domain", aliasDomainID); row != nil {
		for _, companyID := range int64Values(row["company_ids"]) {
			if companyID != 0 && !seen[companyID] {
				seen[companyID] = true
				out = append(out, companyID)
			}
		}
	}
	if store, ok := m.env.stores["res.company"]; ok {
		for companyID, row := range store.records {
			if numericID(row["alias_domain_id"]) == aliasDomainID && !seen[companyID] {
				seen[companyID] = true
				out = append(out, companyID)
			}
		}
	}
	return out
}

func (m ModelSet) companyAliasDomainID(companyID int64) int64 {
	row := m.rowByID("res.company", companyID)
	if row == nil {
		return 0
	}
	return numericID(row["alias_domain_id"])
}

func (m ModelSet) companyDisplayName(companyID int64) string {
	row := m.rowByID("res.company", companyID)
	if row == nil {
		return fmt.Sprint(companyID)
	}
	if name := strings.TrimSpace(stringValue(row["name"])); name != "" {
		return name
	}
	return fmt.Sprint(companyID)
}

func (m ModelSet) validateMailMailWrite(existing map[string]any, values map[string]any) error {
	if m.model.Name != "mail.mail" {
		return nil
	}
	if _, ok := values["mail_server_id"]; !ok {
		if _, ok := values["mail_message_id"]; !ok {
			return nil
		}
	}
	merged := copyValues(existing)
	for key, value := range values {
		merged[key] = value
	}
	return m.validateMailMailForcedServer(merged)
}

func (m ModelSet) validateMailMailForcedServer(row map[string]any) error {
	if m.model.Name != "mail.mail" {
		return nil
	}
	serverID := numericID(row["mail_server_id"])
	if serverID == 0 {
		return nil
	}
	server := m.rowByID("ir.mail_server", serverID)
	if server == nil {
		return fmt.Errorf("mail.mail mail_server_id unavailable")
	}
	ownerUserID := numericID(server["owner_user_id"])
	if ownerUserID == 0 {
		return nil
	}
	if m.env != nil && truthyRecordValue(m.env.Context().Values["mail.disable_personal_mail_servers"]) {
		return fmt.Errorf("mail.mail personal mail_server_id unauthorized")
	}
	if m.recordMailMessageCreateUserID(numericID(row["mail_message_id"])) != ownerUserID {
		return fmt.Errorf("mail.mail personal mail_server_id unauthorized")
	}
	return nil
}

func (m ModelSet) recordMailMessageCreateUserID(messageID int64) int64 {
	message := m.rowByID("mail.message", messageID)
	if message == nil {
		return 0
	}
	return numericID(message["create_uid"])
}

func (m ModelSet) validateIrAttachmentWrite(existing map[string]any, values map[string]any) error {
	if m.model.Name != "ir.attachment" || !irAttachmentWriteTouchesAuditTrail(values) {
		return nil
	}
	if m.irAttachmentAuditTrailProtected(existing) {
		return fmt.Errorf("cannot remove parts of a restricted audit trail")
	}
	return nil
}

func irAttachmentWriteTouchesAuditTrail(values map[string]any) bool {
	for _, fieldName := range []string{"res_id", "res_model", "datas", "company_id"} {
		if _, ok := values[fieldName]; ok {
			return true
		}
	}
	return false
}

func irAttachmentWriteTouchesContent(values map[string]any) bool {
	_, ok := values["datas"]
	return ok
}

func (m ModelSet) irAttachmentUnlinkDetaches(row map[string]any) (bool, error) {
	if m.irAttachmentOfficialRestricted(row) {
		return true, nil
	}
	if m.irAttachmentAuditTrailProtected(row) {
		return false, fmt.Errorf("cannot remove parts of a restricted audit trail")
	}
	return false, nil
}

func (m ModelSet) irAttachmentOfficialRestricted(row map[string]any) bool {
	if stringValue(row["res_model"]) != "account.move" || numericID(row["res_id"]) == 0 {
		return false
	}
	switch stringValue(row["res_field"]) {
	case "invoice_pdf_report_file", "ubl_cii_xml_file":
	default:
		return false
	}
	return m.irAttachmentCompanyRestrictive(row, false)
}

func (m ModelSet) irAttachmentAuditTrailProtected(row map[string]any) bool {
	if stringValue(row["res_model"]) != "account.move" || numericID(row["res_id"]) == 0 {
		return false
	}
	move := m.rowByID("account.move", numericID(row["res_id"]))
	if move == nil || !truthyRecordValue(move["posted_before"]) {
		return false
	}
	return m.irAttachmentCompanyRestrictive(row, true) && irAttachmentPDFOrXML(row)
}

func (m ModelSet) irAttachmentCompanyRestrictive(row map[string]any, requireAttachmentCompany bool) bool {
	companyID := numericID(row["company_id"])
	if companyID == 0 {
		if requireAttachmentCompany {
			return false
		}
		if move := m.rowByID("account.move", numericID(row["res_id"])); move != nil {
			companyID = numericID(move["company_id"])
		}
	}
	if companyID == 0 {
		return false
	}
	company := m.rowByID("res.company", companyID)
	return company != nil && truthyRecordValue(company["restrictive_audit_trail"])
}

func irAttachmentPDFOrXML(row map[string]any) bool {
	mimetype := strings.ToLower(strings.TrimSpace(stringValue(row["mimetype"])))
	if strings.Contains(mimetype, "pdf") || strings.Contains(mimetype, "xml") {
		return true
	}
	data := bytesRecordValue(row["datas"])
	trimmed := strings.TrimSpace(string(data))
	return strings.HasPrefix(string(data), "%PDF-") || strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<")
}

func bytesRecordValue(value any) []byte {
	switch typed := value.(type) {
	case []byte:
		return typed
	case string:
		return []byte(typed)
	default:
		return nil
	}
}

func (m ModelSet) detachIrAttachmentAuditTrail(id int64) {
	store := m.env.stores["ir.attachment"]
	if store == nil {
		return
	}
	row := store.records[id]
	if row == nil {
		return
	}
	row["res_field"] = nil
	row["name"] = detachedAttachmentName(stringValue(row["name"]), id, m.currentUserDisplayName(), time.Now().UTC().Format("2006-01-02"))
}

func (m ModelSet) validateResGroups(row map[string]any) error {
	if strings.HasPrefix(strings.TrimSpace(stringValue(row["name"])), "-") {
		return fmt.Errorf("the name of the group can not start with \"-\"")
	}
	if _, ok := m.model.Fields["api_key_duration"]; ok {
		if value, ok := numeric(row["api_key_duration"]); ok && value < 0 {
			return fmt.Errorf("the api key duration cannot be a negative value")
		}
	}
	if _, ok := m.model.Fields["restricted_access"]; ok && m.isUserTypeGroupCategory(numericID(row["category_id"])) && !isTruthy(row["restricted_access"]) {
		return fmt.Errorf("user type cannot be deactivated in restricted access")
	}
	return nil
}

func (m ModelSet) validateResUsers(row map[string]any) error {
	userTypeIDs := m.resGroupUserTypeIDs()
	if len(userTypeIDs) < 2 {
		return nil
	}
	effective := m.resUserAllGroupIDs(row)
	disjoint := []int64{}
	for _, groupID := range userTypeIDs {
		if idInSlice(groupID, effective) {
			disjoint = append(disjoint, groupID)
		}
	}
	if len(disjoint) <= 1 {
		return nil
	}
	names := make([]string, 0, len(disjoint))
	for _, groupID := range disjoint {
		group := m.rowByID("res.groups", groupID)
		if group == nil {
			names = append(names, fmt.Sprint(groupID))
			continue
		}
		names = append(names, m.resGroupFullName(group))
	}
	sort.Strings(names)
	return fmt.Errorf("user %q cannot be at the same time in exclusive groups %s", stringValue(row["name"]), strings.Join(names, ", "))
}

func (m ModelSet) isUserTypeGroupCategory(categoryID int64) bool {
	if categoryID == 0 {
		return false
	}
	category := m.rowByID("ir.module.category", categoryID)
	name := strings.ToLower(strings.TrimSpace(stringValue(category["name"])))
	return name == "user type" || name == "user types"
}

func (m ModelSet) validateMailMessageReaction(row map[string]any) error {
	messageID := numericID(row["message_id"])
	content := stringValue(row["content"])
	if m.fieldExists("res.users", "active_partner") {
		row["active_partner"] = false
	}
	partnerID := numericID(row["partner_id"])
	guestID := numericID(row["guest_id"])
	if messageID == 0 {
		return fmt.Errorf("mail.message.reaction requires message_id")
	}
	if content == "" {
		return fmt.Errorf("mail.message.reaction requires content")
	}
	if (partnerID == 0 && guestID == 0) || (partnerID != 0 && guestID != 0) {
		return fmt.Errorf("mail.message.reaction requires exactly one partner_id or guest_id")
	}
	rowID := numericID(row["id"])
	for id, existing := range m.store.records {
		if id == rowID {
			continue
		}
		if numericID(existing["message_id"]) != messageID || stringValue(existing["content"]) != content {
			continue
		}
		if partnerID != 0 && numericID(existing["partner_id"]) == partnerID {
			return fmt.Errorf("mail.message.reaction duplicate partner reaction")
		}
		if guestID != 0 && numericID(existing["guest_id"]) == guestID {
			return fmt.Errorf("mail.message.reaction duplicate guest reaction")
		}
	}
	return nil
}

func (m ModelSet) validateAccountAccount(row map[string]any) error {
	accounts := make([]coreaccounting.AccountCodeRecord, 0, len(m.store.records))
	for _, current := range m.store.records {
		accounts = append(accounts, coreaccounting.AccountCodeRecord{
			ID:        numericID(current["id"]),
			Code:      stringValue(current["code"]),
			CompanyID: numericID(current["company_id"]),
		})
	}
	companies := m.companyRelations()
	return coreaccounting.ValidateAccountCodeUnique(accounts, coreaccounting.AccountCodeRecord{
		ID:        numericID(row["id"]),
		Code:      stringValue(row["code"]),
		CompanyID: numericID(row["company_id"]),
	}, companies)
}

func (m ModelSet) companyRelations() []coreaccounting.CompanyRelation {
	companyStore, ok := m.env.stores["res.company"]
	if !ok {
		return nil
	}
	companies := make([]coreaccounting.CompanyRelation, 0, len(companyStore.records))
	for _, row := range companyStore.records {
		companies = append(companies, coreaccounting.CompanyRelation{
			ID:       numericID(row["id"]),
			ParentID: numericID(row["parent_id"]),
		})
	}
	return companies
}

func (m ModelSet) validateLockException(row map[string]any) error {
	if explicitLockExceptionDateCount(row) > 1 {
		return coreaccounting.ErrLockExceptionFields
	}
	exception := m.lockExceptionFromValues(row)
	if !exception.Active {
		return nil
	}
	_, err := coreaccounting.NewLockException(exception, m.companyLockPolicy(exception.CompanyID))
	return err
}

func recordDateValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
			parsed, err := time.Parse(layout, text)
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func (m ModelSet) validateSequenceDateRange(row map[string]any) error {
	id := numericID(row["id"])
	sequenceID := numericID(row["sequence_id"])
	dateFrom := strings.TrimSpace(fmt.Sprint(row["date_from"]))
	dateTo := strings.TrimSpace(fmt.Sprint(row["date_to"]))
	if sequenceID == 0 || dateFrom == "" || dateTo == "" {
		return nil
	}
	for otherID, other := range m.store.records {
		if otherID == id {
			continue
		}
		if numericID(other["sequence_id"]) != sequenceID {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(other["date_from"])) == dateFrom && strings.TrimSpace(fmt.Sprint(other["date_to"])) == dateTo {
			return fmt.Errorf("you cannot create two date ranges for the same sequence with the same date range")
		}
	}
	return nil
}

func (m ModelSet) validateServerActionChildren(row map[string]any) error {
	actionID := numericID(row["id"])
	if actionID == 0 {
		return nil
	}
	if m.hasServerActionCycle(actionID, actionID, map[int64]bool{}) {
		return fmt.Errorf("recursion found in child server actions")
	}
	warned := m.warnedServerActionChildren(actionID)
	if len(warned) > 0 {
		return fmt.Errorf("following child actions have warnings: %s", strings.Join(warned, ", "))
	}
	return nil
}

func (m ModelSet) serverActionConstraintRows(row map[string]any) []map[string]any {
	out := []map[string]any{row}
	seen := map[int64]bool{}
	for parentID := numericID(row["parent_id"]); parentID != 0; {
		if seen[parentID] {
			break
		}
		seen[parentID] = true
		parent, ok := m.store.records[parentID]
		if !ok {
			break
		}
		out = append(out, parent)
		parentID = numericID(parent["parent_id"])
	}
	return out
}

func (m ModelSet) hasServerActionCycle(rootID int64, currentID int64, visiting map[int64]bool) bool {
	if visiting[currentID] {
		return true
	}
	visiting[currentID] = true
	defer delete(visiting, currentID)
	for _, childID := range m.serverActionChildIDs(currentID) {
		if childID == rootID {
			return true
		}
		if _, ok := m.store.records[childID]; !ok {
			continue
		}
		if m.hasServerActionCycle(rootID, childID, visiting) {
			return true
		}
	}
	return false
}

func (m ModelSet) warnedServerActionChildren(actionID int64) []string {
	out := []string{}
	for _, childID := range m.serverActionChildIDs(actionID) {
		child, ok := m.store.records[childID]
		if !ok {
			continue
		}
		if warning, ok := child["warning"]; ok && warning != nil && strings.TrimSpace(fmt.Sprint(warning)) != "" {
			out = append(out, m.displayName(child))
		}
	}
	return out
}

func (m ModelSet) serverActionChildIDs(parentID int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	if parent, ok := m.store.records[parentID]; ok {
		for _, childID := range int64IDs(parent["child_ids"]) {
			if childID != 0 && !seen[childID] {
				seen[childID] = true
				out = append(out, childID)
			}
		}
	}
	for childID, row := range m.store.records {
		if numericID(row["parent_id"]) == parentID && !seen[childID] {
			seen[childID] = true
			out = append(out, childID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func int64IDs(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []int:
		out := make([]int64, 0, len(typed))
		for _, id := range typed {
			out = append(out, int64(id))
		}
		return out
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id := numericID(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	default:
		if id := numericID(value); id != 0 {
			return []int64{id}
		}
		return nil
	}
}

func (m ModelSet) Browse(ids ...int64) RecordSet {
	return RecordSet{model: m, ids: append([]int64(nil), ids...)}
}

func (m ModelSet) Search(node domain.Node) (RecordSet, error) {
	return m.SearchWithOptions(node, SearchOptions{})
}

type SearchOptions struct {
	Offset int
	Limit  int
	Order  string
}

type ReadGroupOptions struct {
	Fields  []string
	GroupBy []string
	Order   string
	Offset  int
	Limit   int
	Lazy    *bool
}

type readGroupAggregateSpec struct {
	Key           string
	Field         string
	Func          string
	Kind          field.Kind
	Relation      string
	CurrencyField string
}

var readGroupAggregatePattern = regexp.MustCompile(`^(\w+)(?::(\w+)(?:\((\w+)\))?)?$`)
var readGroupOrderPartPattern = regexp.MustCompile(`(?i)^\s*([a-z0-9_]+(?:\.[\w.]+)?(?::[a-z_]+)?)(?:\s+(asc|desc))?(?:\s+(nulls\s+(?:first|last)))?\s*$`)
var searchOrderPartPattern = regexp.MustCompile(`(?i)^\s*([a-z0-9_]+(?:\.[a-z0-9_]+)?)(?::[a-z_]+)?(?:\s+(asc|desc))?(?:\s+(nulls\s+(?:first|last)))?\s*$`)

type searchOrderTerm struct {
	Term       string
	Path       []string
	Kind       field.Kind
	Relation   string
	Descending bool
	NullsFirst bool
}

type searchOrderComposite struct {
	Model ModelSet
	ID    int64
	Row   map[string]any
	Terms []searchOrderTerm
}

func (m ModelSet) SearchWithOptions(node domain.Node, opts SearchOptions) (RecordSet, error) {
	if m.err != nil {
		return RecordSet{}, m.err
	}
	if err := m.check(OpRead, nil); err != nil {
		return RecordSet{}, err
	}
	if m.model.Name == "mailing.contact" || m.model.Name == "mailing.list" || m.model.Name == "mailing.mailing" || m.model.Name == "utm.campaign" {
		m.syncAllMailingDerivedFields()
	}
	if m.model.Name == "marketing.trace" || m.model.Name == "whatsapp.message" {
		m.syncAllWhatsAppMarketingDerivedFields()
	}
	activeName := m.activeTestField(node)
	var ids []int64
	for id, row := range m.store.records {
		ok, err := m.match(row, node)
		if err != nil {
			return RecordSet{}, err
		}
		if ok && activeName != "" && !activeTestRowVisible(row, activeName) {
			ok = false
		}
		if ok {
			allowed, err := m.allowedRecord(OpRead, row)
			if err != nil {
				return RecordSet{}, err
			}
			if !allowed {
				continue
			}
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if err := m.sortSearchIDs(ids, opts.Order); err != nil {
		return RecordSet{}, err
	}
	if opts.Offset > 0 {
		if opts.Offset >= len(ids) {
			ids = nil
		} else {
			ids = ids[opts.Offset:]
		}
	}
	if opts.Limit > 0 && opts.Limit < len(ids) {
		ids = ids[:opts.Limit]
	}
	return RecordSet{model: m, ids: ids}, nil
}

func (m ModelSet) activeTestField(node domain.Node) string {
	name := m.activeFieldName()
	if name == "" || !contextActiveTestEnabled(m.env.context) || domainContainsField(node, name) {
		return ""
	}
	return name
}

func (m ModelSet) activeFieldName() string {
	if f, ok := m.model.Fields["active"]; ok && f.Kind == field.Bool {
		return "active"
	}
	if f, ok := m.model.Fields["x_active"]; ok && f.Kind == field.Bool {
		return "x_active"
	}
	return ""
}

func contextActiveTestEnabled(ctx Context) bool {
	value, ok := ctx.Values["active_test"]
	if !ok {
		return true
	}
	return truthyRecordValue(value)
}

func domainContainsField(node domain.Node, name string) bool {
	switch node.Kind {
	case domain.Condition:
		return node.Field == name
	case domain.All, domain.Any, domain.None:
		for _, child := range node.Children {
			if domainContainsField(child, name) {
				return true
			}
		}
	}
	return false
}

func activeTestRowVisible(row map[string]any, name string) bool {
	value, ok := row[name]
	if !ok || value == nil {
		return true
	}
	return truthyRecordValue(value)
}

func (m ModelSet) sortSearchIDs(ids []int64, order string) error {
	terms, err := m.searchOrderTerms(order)
	if err != nil {
		return err
	}
	if len(ids) < 2 || len(terms) == 0 {
		return nil
	}
	sort.SliceStable(ids, func(i, j int) bool {
		leftRow := m.store.records[ids[i]]
		rightRow := m.store.records[ids[j]]
		return m.compareSearchRows(ids[i], leftRow, ids[j], rightRow, terms) < 0
	})
	return nil
}

func (m ModelSet) searchOrderTerms(order string) ([]searchOrderTerm, error) {
	order = strings.TrimSpace(order)
	if order == "" {
		order = strings.TrimSpace(m.model.Order)
	}
	if order == "" {
		order = "id"
	}
	parts := strings.Split(order, ",")
	terms := make([]searchOrderTerm, 0, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("invalid search order %q", order)
		}
		match := searchOrderPartPattern.FindStringSubmatch(raw)
		if match == nil {
			return nil, fmt.Errorf("invalid search order %q", order)
		}
		term := searchOrderTerm{Term: match[1], Path: strings.Split(match[1], ".")}
		direction := strings.ToLower(strings.TrimSpace(match[2]))
		term.Descending = direction == "desc"
		nulls := strings.ToLower(strings.Join(strings.Fields(match[3]), " "))
		switch nulls {
		case "nulls first":
			term.NullsFirst = true
		case "nulls last":
			term.NullsFirst = false
		default:
			term.NullsFirst = term.Descending
		}
		resolved, err := m.resolveSearchOrderTerm(term)
		if err != nil {
			return nil, err
		}
		terms = append(terms, resolved)
	}
	return terms, nil
}

func (m ModelSet) resolveSearchOrderTerm(term searchOrderTerm) (searchOrderTerm, error) {
	if len(term.Path) == 0 || len(term.Path) > 2 {
		return term, fmt.Errorf("invalid search order term %q on model %s", term.Term, m.model.Name)
	}
	if term.Path[0] == "id" {
		if len(term.Path) > 1 {
			return term, fmt.Errorf("invalid search order term %q on model %s", term.Term, m.model.Name)
		}
		term.Kind = field.Int
		return term, nil
	}
	if term.Path[0] == "display_name" {
		if len(term.Path) > 1 {
			return term, fmt.Errorf("invalid search order term %q on model %s", term.Term, m.model.Name)
		}
		term.Kind = field.Char
		return term, nil
	}
	meta, ok := m.model.Fields[term.Path[0]]
	if !ok {
		return term, fmt.Errorf("invalid search order field %q on model %s", term.Path[0], m.model.Name)
	}
	if len(term.Path) > 1 {
		if meta.Kind == field.Many2One {
			if term.Path[1] != "id" {
				return term, fmt.Errorf("invalid search order term %q on model %s", term.Term, m.model.Name)
			}
		} else if meta.Kind != field.Properties {
			return term, fmt.Errorf("invalid search order term %q on model %s", term.Term, m.model.Name)
		}
	}
	if meta.Kind == field.One2Many || meta.Kind == field.Many2Many {
		return term, fmt.Errorf("invalid search order on relational field %q on model %s", term.Term, m.model.Name)
	}
	if !meta.Store && meta.Kind != field.Computed {
		return term, fmt.Errorf("search order requires stored field %s.%s", m.model.Name, meta.Name)
	}
	term.Kind = meta.Kind
	term.Relation = meta.Relation
	return term, nil
}

func (m ModelSet) compareSearchRows(leftID int64, leftRow map[string]any, rightID int64, rightRow map[string]any, terms []searchOrderTerm) int {
	for _, term := range terms {
		leftValue, leftNull := m.searchOrderValue(leftID, leftRow, term)
		rightValue, rightNull := m.searchOrderValue(rightID, rightRow, term)
		cmp := searchOrderCompare(leftValue, leftNull, rightValue, rightNull, term)
		if cmp == 0 {
			continue
		}
		if term.Descending {
			return -cmp
		}
		return cmp
	}
	return 0
}

func (m ModelSet) searchOrderValue(id int64, row map[string]any, term searchOrderTerm) (any, bool) {
	if row == nil {
		return nil, true
	}
	if len(term.Path) == 1 {
		switch term.Path[0] {
		case "id":
			return id, false
		case "display_name":
			value := m.displayName(row)
			return value, strings.TrimSpace(value) == ""
		}
	}
	value := row[term.Path[0]]
	if term.Kind == field.Bool {
		if value == nil {
			return false, false
		}
		return value, false
	}
	if term.Kind == field.Many2One {
		relatedID := numericID(value)
		if relatedID == 0 {
			return nil, true
		}
		if len(term.Path) == 2 && term.Path[1] == "id" {
			return relatedID, false
		}
		relatedModel, ok := m.env.registry.Model(term.Relation)
		if !ok {
			return relatedID, false
		}
		relatedRow := m.rowByID(term.Relation, relatedID)
		if relatedRow == nil {
			return nil, true
		}
		relatedSet := ModelSet{env: m.env, model: relatedModel}
		terms, err := relatedSet.searchOrderTerms(relatedModel.Order)
		if err != nil || len(terms) == 0 {
			return relatedID, false
		}
		return searchOrderComposite{Model: relatedSet, ID: relatedID, Row: relatedRow, Terms: terms}, false
	}
	if term.Kind == field.Properties && len(term.Path) == 2 {
		value = readGroupPropertyValueMap(value)[term.Path[1]]
	}
	null := value == nil
	if value == false {
		null = true
	}
	return value, null
}

func (m ModelSet) searchRelatedOrderValue(id int64, row map[string]any, name string) (any, bool) {
	switch name {
	case "id":
		return id, false
	case "display_name":
		value := m.displayName(row)
		return value, strings.TrimSpace(value) == ""
	}
	meta, ok := m.model.Fields[name]
	if !ok {
		return nil, true
	}
	value := row[name]
	if meta.Kind == field.Bool {
		if value == nil {
			return false, false
		}
		return value, false
	}
	if meta.Kind == field.Many2One {
		relatedID := numericID(value)
		if relatedID == 0 {
			return nil, true
		}
		relatedModel, ok := m.env.registry.Model(meta.Relation)
		if !ok {
			return relatedID, false
		}
		relatedRow := m.rowByID(meta.Relation, relatedID)
		if relatedRow == nil {
			return nil, true
		}
		relatedSet := ModelSet{env: m.env, model: relatedModel}
		terms, err := relatedSet.searchOrderTerms(relatedModel.Order)
		if err != nil || len(terms) == 0 {
			return relatedID, false
		}
		return searchOrderComposite{Model: relatedSet, ID: relatedID, Row: relatedRow, Terms: terms}, false
	}
	null := value == nil
	if value == false {
		null = true
	}
	return value, null
}

func searchOrderCompare(left any, leftNull bool, right any, rightNull bool, term searchOrderTerm) int {
	if leftNull || rightNull {
		return readGroupOrderCompare(left, leftNull, right, rightNull, readGroupOrderTerm{NullsFirst: term.NullsFirst})
	}
	leftComposite, leftIsComposite := left.(searchOrderComposite)
	rightComposite, rightIsComposite := right.(searchOrderComposite)
	if leftIsComposite && rightIsComposite {
		return leftComposite.Model.compareSearchRows(leftComposite.ID, leftComposite.Row, rightComposite.ID, rightComposite.Row, leftComposite.Terms)
	}
	return readGroupCompareNonNull(left, right)
}

func (m ModelSet) SearchCount(node domain.Node, limit int) (int, error) {
	found, err := m.SearchWithOptions(node, SearchOptions{Limit: limit})
	if err != nil {
		return 0, err
	}
	return found.Len(), nil
}

func (m ModelSet) match(row map[string]any, node domain.Node) (bool, error) {
	return matchWithModelContext(m.env.context, m.model, row, node)
}

func (m ModelSet) ReadGroup(node domain.Node, opts ReadGroupOptions) ([]map[string]any, error) {
	allGroupBy, err := m.readGroupSpecs(opts.GroupBy)
	if err != nil {
		return nil, err
	}
	allGroupBy = readGroupLegacySpecs(allGroupBy)
	groupBy := allGroupBy
	if readGroupLazy(opts) && len(groupBy) > 1 {
		groupBy = groupBy[:1]
	}
	aggregates, err := m.readGroupLegacyAggregates(opts.Fields, groupBy, readGroupLazy(opts))
	if err != nil {
		return nil, err
	}
	rows, err := m.readGroupWithSpecs(node, opts, groupBy, aggregates)
	if err != nil {
		return nil, err
	}
	return m.readGroupLegacyRows(rows, groupBy, opts.GroupBy[len(groupBy):], readGroupLazy(opts)), nil
}

func (m ModelSet) FormattedReadGroup(node domain.Node, opts ReadGroupOptions) ([]map[string]any, error) {
	groupBy, err := m.readGroupSpecs(opts.GroupBy)
	if err != nil {
		return nil, err
	}
	aggregates, err := m.readGroupFormattedAggregates(opts.Fields)
	if err != nil {
		return nil, err
	}
	rows, err := m.readGroupWithSpecs(node, opts, groupBy, aggregates)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		formatted := map[string]any{}
		groupNames := map[string]bool{}
		for _, groupSpec := range groupBy {
			groupNames[groupSpec.Name] = true
		}
		for key, value := range row {
			if key == "__domain" || key == "__range" || key == readGroupInternalSpecsKey {
				continue
			}
			if groupNames[key] {
				continue
			}
			formatted[key] = value
		}
		for _, groupSpec := range readGroupOutputSpecs(row, groupBy) {
			value := row[groupSpec.Name]
			if formattedValue, ok := m.readGroupPropertyDisplayValue(groupSpec, value); ok {
				formatted[groupSpec.Key] = formattedValue
				continue
			}
			if formattedValue, ok := readGroupFormattedValue(groupSpec, value); ok {
				formatted[groupSpec.Key] = formattedValue
				continue
			}
			if formattedValue, ok := m.readGroupRelationalValue(groupSpec, value); ok {
				formatted[groupSpec.Key] = formattedValue
				continue
			}
			formatted[groupSpec.Key] = value
		}
		if extraDomain := readGroupFormattedExtraDomain(row); len(extraDomain) > 0 {
			formatted["__extra_domain"] = extraDomain
		} else {
			formatted["__extra_domain"] = []any{}
		}
		out = append(out, formatted)
	}
	return out, nil
}

const readGroupInternalSpecsKey = "__read_group_specs"

func readGroupOutputSpecs(row map[string]any, fallback []readGroupSpec) []readGroupSpec {
	specs, ok := row[readGroupInternalSpecsKey].([]readGroupSpec)
	if !ok || len(specs) != len(fallback) {
		return fallback
	}
	return specs
}

func readGroupFormattedExtraDomain(row map[string]any) []any {
	components, ok := row["__domain"].([]any)
	if !ok || len(components) == 0 {
		return nil
	}
	return readGroupAndDomainComponents(components)
}

func readGroupFormattedIntervalDomain(spec readGroupSpec, value any) ([]any, bool) {
	if strings.TrimSpace(spec.Interval) == "" || (spec.Kind != field.Date && spec.Kind != field.DateTime) {
		return nil, false
	}
	if readGroupNumberInterval(spec.Interval) {
		return readGroupNumberIntervalDomain(spec, value), true
	}
	start, end, ok := readGroupIntervalRange(spec, value)
	if !ok {
		if value == nil || value == false || value == "" {
			return []any{spec.Name, "=", false}, true
		}
		return nil, false
	}
	return []any{
		"&",
		[]any{spec.Name, ">=", readGroupTemporalDomainValue(start, spec.Kind)},
		[]any{spec.Name, "<", readGroupTemporalDomainValue(end, spec.Kind)},
	}, true
}

func readGroupAndDomainComponents(components []any) []any {
	switch len(components) {
	case 0:
		return nil
	case 1:
		if component, ok := components[0].([]any); ok {
			if len(component) > 0 {
				if op, ok := component[0].(string); ok && (op == "&" || op == "|" || op == "!") {
					return component
				}
			}
			return []any{component}
		}
		return []any{components[0]}
	default:
		out := make([]any, 0, len(components)*2-1)
		for index := 1; index < len(components); index++ {
			out = append(out, "&")
		}
		out = append(out, components...)
		return out
	}
}

func (m ModelSet) readGroupLegacyAggregates(fields []string, groupBy []readGroupSpec, lazy bool) ([]readGroupAggregateSpec, error) {
	aggregates := []readGroupAggregateSpec{{Key: "__count", Func: "__count"}}
	groupKeys := map[string]bool{}
	for _, spec := range groupBy {
		groupKeys[spec.Key] = true
	}
	for _, raw := range fields {
		fieldSpec := strings.TrimSpace(raw)
		if fieldSpec == "" || fieldSpec == "__count" {
			continue
		}
		key, fieldName, fn, explicit, err := m.parseReadGroupAggregateSpec(fieldSpec)
		if err != nil {
			return nil, err
		}
		if !explicit {
			if groupKeys[fieldSpec] {
				continue
			}
			meta, ok := m.model.Fields[fieldName]
			if !ok {
				return nil, fmt.Errorf("read_group invalid aggregate field %s.%s", m.model.Name, fieldName)
			}
			if !meta.Store || strings.TrimSpace(meta.Aggregator) == "" {
				continue
			}
			fn = meta.Aggregator
		}
		aggregate, err := m.readGroupAggregateSpec(key, fieldName, fn)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, aggregate)
	}
	return aggregates, nil
}

func (m ModelSet) readGroupFormattedAggregates(fields []string) ([]readGroupAggregateSpec, error) {
	aggregates := []readGroupAggregateSpec{{Key: "__count", Func: "__count"}}
	for _, raw := range fields {
		fieldSpec := strings.TrimSpace(strings.ReplaceAll(raw, ":recordset", ":array_agg"))
		if fieldSpec == "" || fieldSpec == "__count" {
			continue
		}
		key, fieldName, fn, explicit, err := m.parseReadGroupAggregateSpec(fieldSpec)
		if err != nil {
			return nil, err
		}
		if explicit && strings.Contains(fieldSpec, "(") {
			return nil, fmt.Errorf("formatted_read_group does not support aggregate alias syntax %q", raw)
		}
		if !explicit {
			meta, ok := m.model.Fields[fieldName]
			if !ok {
				return nil, fmt.Errorf("read_group invalid aggregate field %s.%s", m.model.Name, fieldName)
			}
			if !meta.Store || strings.TrimSpace(meta.Aggregator) == "" {
				continue
			}
			fn = meta.Aggregator
			key = fieldName
		} else if !strings.Contains(fieldSpec, "(") {
			key = fieldSpec
		}
		aggregate, err := m.readGroupAggregateSpec(key, fieldName, fn)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, aggregate)
	}
	return aggregates, nil
}

func (m ModelSet) parseReadGroupAggregateSpec(fieldSpec string) (string, string, string, bool, error) {
	match := readGroupAggregatePattern.FindStringSubmatch(fieldSpec)
	if match == nil {
		return "", "", "", false, fmt.Errorf("read_group invalid field specification %q", fieldSpec)
	}
	name, fn, fieldName := match[1], match[2], match[3]
	if fieldName != "" {
		return name, fieldName, fn, true, nil
	}
	if fn != "" {
		return name, name, fn, true, nil
	}
	return name, name, "", false, nil
}

func (m ModelSet) readGroupAggregateSpec(key string, fieldName string, fn string) (readGroupAggregateSpec, error) {
	fn = strings.ToLower(strings.TrimSpace(fn))
	if fn == "" {
		return readGroupAggregateSpec{}, fmt.Errorf("read_group missing aggregate function for %s.%s", m.model.Name, fieldName)
	}
	if fn == "__count" {
		return readGroupAggregateSpec{Key: key, Func: fn}, nil
	}
	meta, ok := m.model.Fields[fieldName]
	if !ok && fieldName != "id" {
		return readGroupAggregateSpec{}, fmt.Errorf("read_group invalid aggregate field %s.%s", m.model.Name, fieldName)
	}
	if ok && !meta.Store {
		return readGroupAggregateSpec{}, fmt.Errorf("read_group requires stored aggregate field %s.%s", m.model.Name, fieldName)
	}
	switch fn {
	case "sum", "avg", "min", "max", "count", "count_distinct", "array_agg", "array_agg_distinct", "recordset", "bool_or", "bool_and", "sum_currency":
	default:
		return readGroupAggregateSpec{}, fmt.Errorf("read_group aggregate %q is not supported for %s.%s", fn, m.model.Name, fieldName)
	}
	kind := field.Int
	currencyField := ""
	if ok {
		kind = meta.Kind
		currencyField = meta.CurrencyField
		if currencyField == "" && (kind == field.Monetary || fn == "sum_currency") {
			if _, ok := m.model.Fields["currency_id"]; ok {
				currencyField = "currency_id"
			} else if _, ok := m.model.Fields["x_currency_id"]; ok {
				currencyField = "x_currency_id"
			}
		}
	}
	if fn == "sum_currency" {
		if !ok || kind != field.Monetary {
			return readGroupAggregateSpec{}, fmt.Errorf(`read_group aggregate "sum_currency" only works on monetary field %s.%s`, m.model.Name, fieldName)
		}
		if currencyField == "" {
			return readGroupAggregateSpec{}, fmt.Errorf("read_group aggregate sum_currency requires currency field for %s.%s", m.model.Name, fieldName)
		}
		if currencyMeta, ok := m.model.Fields[currencyField]; !ok || !currencyMeta.Store {
			return readGroupAggregateSpec{}, fmt.Errorf("read_group aggregate sum_currency requires stored currency field %s.%s", m.model.Name, currencyField)
		}
	}
	relation := ""
	if fn == "recordset" {
		if fieldName == "id" {
			relation = m.model.Name
		} else if !ok || !readGroupRelationalKind(kind) {
			return readGroupAggregateSpec{}, fmt.Errorf(`read_group aggregate "recordset" can only be used on relational field or id for %s.%s`, m.model.Name, fieldName)
		} else if strings.TrimSpace(meta.Relation) == "" {
			return readGroupAggregateSpec{}, fmt.Errorf("read_group aggregate recordset requires relation for %s.%s", m.model.Name, fieldName)
		} else {
			relation = meta.Relation
		}
	}
	return readGroupAggregateSpec{Key: key, Field: fieldName, Func: fn, Kind: kind, Relation: relation, CurrencyField: currencyField}, nil
}

func readGroupRelationalKind(kind field.Kind) bool {
	return kind == field.Many2One || kind == field.One2Many || kind == field.Many2Many
}

func readGroupLazy(opts ReadGroupOptions) bool {
	if opts.Lazy == nil {
		return true
	}
	return *opts.Lazy
}

func readGroupLegacySpecs(groupBy []readGroupSpec) []readGroupSpec {
	out := make([]readGroupSpec, len(groupBy))
	for index, spec := range groupBy {
		if spec.Interval == "" && (spec.Kind == field.Date || spec.Kind == field.DateTime) {
			spec.Interval = "month"
		}
		out[index] = spec
	}
	return out
}

func (m ModelSet) readGroupLegacyRows(rows []map[string]any, groupBy []readGroupSpec, remainingGroupBy []string, lazy bool) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	countKey := "__count"
	if lazy && len(groupBy) == 1 {
		countKey = readGroupLegacyCountKey(groupBy[0])
	}
	for _, row := range rows {
		legacy := map[string]any{}
		for key, value := range row {
			switch key {
			case "__count", "__range", readGroupInternalSpecsKey:
				continue
			default:
				if readGroupSpecByName(groupBy, key) == nil {
					legacy[key] = value
				}
			}
		}
		outputSpecs := readGroupOutputSpecs(row, groupBy)
		for _, spec := range outputSpecs {
			value := row[spec.Name]
			if formatted, ok := m.readGroupPropertyDisplayValue(spec, value); ok {
				legacy[spec.Key] = formatted
			} else if formatted, ok := readGroupLegacyValue(spec, value); ok {
				legacy[spec.Key] = formatted
			} else if formatted, ok := m.readGroupRelationalValue(spec, value); ok {
				legacy[spec.Key] = formatted
			} else {
				legacy[spec.Key] = value
			}
		}
		legacy[countKey] = row["__count"]
		if ranges := readGroupLegacyRanges(row["__range"], outputSpecs); len(ranges) > 0 {
			legacy["__range"] = ranges
		}
		if lazy && len(remainingGroupBy) > 0 {
			legacy["__context"] = map[string]any{"group_by": append([]string(nil), remainingGroupBy...)}
		}
		out = append(out, legacy)
	}
	return out
}

func readGroupSpecByName(groupBy []readGroupSpec, name string) *readGroupSpec {
	for index := range groupBy {
		if groupBy[index].Name == name {
			return &groupBy[index]
		}
	}
	return nil
}

func readGroupLegacyCountKey(spec readGroupSpec) string {
	base := spec.Key
	if cut := strings.Index(base, ":"); cut >= 0 {
		base = base[:cut]
	}
	return base + "_count"
}

func readGroupLegacyValue(spec readGroupSpec, value any) (any, bool) {
	if strings.TrimSpace(spec.Interval) == "" || readGroupNumberInterval(spec.Interval) || (spec.Kind != field.Date && spec.Kind != field.DateTime) {
		return nil, false
	}
	if value == nil || value == false || value == "" {
		return false, true
	}
	label, ok := readGroupTemporalLabel(value, spec)
	if !ok {
		return nil, false
	}
	return label, true
}

func readGroupLegacyRanges(value any, groupBy []readGroupSpec) map[string]any {
	source, ok := value.(map[string]any)
	if !ok || len(source) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, spec := range groupBy {
		if rangeValue, ok := source[spec.Name]; ok {
			out[spec.Key] = rangeValue
		}
	}
	return out
}

func (m ModelSet) readGroupRelationalValue(spec readGroupSpec, value any) (any, bool) {
	if spec.Property {
		return nil, false
	}
	if spec.Name == "id" {
		id := numericID(value)
		if id == 0 {
			return false, true
		}
		return m.readGroupNamePair(m.model.Name, id), true
	}
	if spec.Kind != field.Many2One && spec.Kind != field.Many2Many {
		return nil, false
	}
	id := numericID(value)
	if id == 0 {
		return false, true
	}
	return m.readGroupNamePair(spec.Relation, id), true
}

func (m ModelSet) readGroupPropertyDisplayValue(spec readGroupSpec, value any) (any, bool) {
	if !spec.Property {
		return nil, false
	}
	if readGroupFalsyValue(value) {
		return false, true
	}
	switch spec.PropertyType {
	case "many2one", "many2many":
		id := numericID(value)
		if id == 0 {
			return false, true
		}
		return m.readGroupNamePair(spec.Relation, id), true
	case "tags":
		if tag := readGroupPropertyTagTuple(spec, value); len(tag) > 0 {
			return tag, true
		}
		return false, true
	default:
		return nil, false
	}
}

func readGroupPropertyTagTuple(spec readGroupSpec, value any) []any {
	key := strings.TrimSpace(fmt.Sprint(domain.NormalizeScalar(value)))
	if key == "" {
		return nil
	}
	items, err := collectionValues(spec.PropertyDefinition["tags"])
	if err != nil {
		return nil
	}
	for _, item := range items {
		values, err := collectionValues(item)
		if err != nil || len(values) == 0 {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(domain.NormalizeScalar(values[0]))) != key {
			continue
		}
		out := make([]any, 0, len(values))
		out = append(out, values...)
		return out
	}
	return nil
}

func (m ModelSet) readGroupNamePair(modelName string, id int64) []any {
	relatedRow := m.rowByID(modelName, id)
	if relatedRow == nil {
		return []any{id, ""}
	}
	relatedModel, ok := m.env.registry.Model(modelName)
	if !ok {
		return []any{id, ""}
	}
	return []any{id, ModelSet{env: m.env, model: relatedModel}.displayName(relatedRow)}
}

func (m ModelSet) readGroupAggregateValue(spec readGroupAggregateSpec, rows []map[string]any, count int) any {
	switch spec.Func {
	case "__count":
		return count
	case "sum":
		total, _, integral, ok := readGroupNumericAggregate(spec, rows)
		if !ok {
			return nil
		}
		if integral {
			return int64(total)
		}
		return total
	case "avg":
		total, valueCount, _, ok := readGroupNumericAggregate(spec, rows)
		if !ok || valueCount == 0 {
			return nil
		}
		return total / float64(valueCount)
	case "min", "max":
		value, ok := readGroupMinMaxAggregate(spec, rows, spec.Func == "max")
		if !ok {
			return nil
		}
		return value
	case "count":
		return readGroupCountAggregate(spec, rows, false)
	case "count_distinct":
		return readGroupCountAggregate(spec, rows, true)
	case "array_agg":
		return readGroupArrayAggregate(spec, rows, false)
	case "recordset":
		return m.readGroupRecordSetAggregate(spec, rows)
	case "array_agg_distinct":
		return readGroupArrayAggregate(spec, rows, true)
	case "bool_or", "bool_and":
		return readGroupBoolAggregate(spec, rows, spec.Func == "bool_and")
	case "sum_currency":
		total, ok := m.readGroupCurrencyAggregate(spec, rows)
		if !ok {
			return nil
		}
		return total
	default:
		return nil
	}
}

func (m ModelSet) readGroupCurrencyAggregate(spec readGroupAggregateSpec, rows []map[string]any) (float64, bool) {
	total := 0.0
	seen := false
	for _, row := range rows {
		amount, ok, _ := readGroupFloatValue(row[spec.Field])
		if !ok {
			continue
		}
		rate := m.readGroupCurrencyRate(numericID(row[spec.CurrencyField]))
		if rate == 0 {
			rate = 1
		}
		total += amount / rate
		seen = true
	}
	return total, seen
}

func (m ModelSet) readGroupCurrencyRate(currencyID int64) float64 {
	if currencyID == 0 {
		return 1
	}
	if _, ok := m.env.registry.Model("res.currency.rate"); !ok {
		return 1
	}
	rows := m.rows("res.currency.rate")
	companyID := m.env.context.CompanyID
	today := time.Now().UTC()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	findRate := func(companyOnly bool) (float64, bool) {
		bestPastIndex := -1
		bestFutureIndex := -1
		for index, row := range rows {
			if numericID(row["currency_id"]) != currencyID {
				continue
			}
			rowCompanyID := numericID(row["company_id"])
			if companyOnly {
				if rowCompanyID != companyID {
					continue
				}
			} else if rowCompanyID != 0 {
				continue
			}
			rateDate := recordDateValue(row["name"])
			if rateDate.IsZero() {
				continue
			}
			rateDate = time.Date(rateDate.Year(), rateDate.Month(), rateDate.Day(), 0, 0, 0, 0, time.UTC)
			if rateDate.After(todayDate) {
				if bestFutureIndex < 0 || rateDate.Before(recordDateValue(rows[bestFutureIndex]["name"])) {
					bestFutureIndex = index
				}
				continue
			}
			if bestPastIndex < 0 || rateDate.After(recordDateValue(rows[bestPastIndex]["name"])) {
				bestPastIndex = index
			}
		}
		for _, index := range []int{bestFutureIndex, bestPastIndex} {
			if index >= 0 {
				if rate, ok, _ := readGroupFloatValue(rows[index]["rate"]); ok && rate != 0 {
					return rate, true
				}
			}
		}
		return 0, false
	}
	if companyID != 0 {
		if rate, ok := findRate(true); ok {
			return rate
		}
	}
	if rate, ok := findRate(false); ok {
		return rate
	}
	return 1
}

func readGroupNumericAggregate(spec readGroupAggregateSpec, rows []map[string]any) (float64, int, bool, bool) {
	total := 0.0
	count := 0
	integral := true
	seen := false
	for _, row := range rows {
		number, ok, valueIntegral := readGroupFloatValue(row[spec.Field])
		if !ok {
			continue
		}
		if !valueIntegral {
			integral = false
		}
		total += number
		count++
		seen = true
	}
	return total, count, integral, seen
}

func readGroupFloatValue(value any) (float64, bool, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true, true
	case int64:
		return float64(typed), true, true
	case float32:
		return float64(typed), true, false
	case float64:
		return typed, true, false
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil, false
	default:
		return 0, false, true
	}
}

func readGroupMinMaxAggregate(spec readGroupAggregateSpec, rows []map[string]any, maximum bool) (any, bool) {
	var selected any
	var selectedKey any
	seen := false
	for _, row := range rows {
		value := row[spec.Field]
		key, ok := readGroupComparableValue(value)
		if !ok {
			continue
		}
		if !seen || readGroupComparableLess(selectedKey, key) == maximum {
			selected = value
			selectedKey = key
			seen = true
		}
	}
	return selected, seen
}

func readGroupComparableValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false
		}
		return typed, true
	case bool:
		return typed, true
	default:
		return fmt.Sprint(value), true
	}
}

func readGroupComparableLess(left any, right any) bool {
	switch leftValue := left.(type) {
	case float64:
		rightValue, _ := right.(float64)
		return leftValue < rightValue
	case bool:
		rightValue, _ := right.(bool)
		return !leftValue && rightValue
	default:
		return fmt.Sprint(left) < fmt.Sprint(right)
	}
}

func readGroupCountAggregate(spec readGroupAggregateSpec, rows []map[string]any, distinct bool) int {
	count := 0
	seen := map[string]bool{}
	for _, row := range rows {
		value := row[spec.Field]
		if value == nil {
			continue
		}
		if distinct {
			key := readGroupKey(value)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		count++
	}
	return count
}

func readGroupArrayAggregate(spec readGroupAggregateSpec, rows []map[string]any, distinct bool) []any {
	out := make([]any, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		value := row[spec.Field]
		if distinct {
			key := readGroupKey(value)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, value)
	}
	return out
}

func (m ModelSet) readGroupRecordSetAggregate(spec readGroupAggregateSpec, rows []map[string]any) RecordSet {
	relation := spec.Relation
	if relation == "" {
		relation = m.model.Name
	}
	modelSet := m.env.Model(relation)
	ids := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, row := range rows {
		if spec.Field == "id" {
			add(numericID(row["id"]))
			continue
		}
		switch spec.Kind {
		case field.Many2Many, field.One2Many:
			for _, id := range int64Values(row[spec.Field]) {
				add(id)
			}
		default:
			add(numericID(row[spec.Field]))
		}
	}
	return modelSet.Browse(ids...)
}

func readGroupBoolAggregate(spec readGroupAggregateSpec, rows []map[string]any, and bool) any {
	if len(rows) == 0 {
		return nil
	}
	value := and
	seen := false
	for _, row := range rows {
		current, ok := row[spec.Field].(bool)
		if !ok {
			continue
		}
		if and {
			value = value && current
		} else {
			value = value || current
		}
		seen = true
	}
	if !seen {
		return nil
	}
	return value
}

func (m ModelSet) readGroupWithSpecs(node domain.Node, opts ReadGroupOptions, groupBy []readGroupSpec, aggregates []readGroupAggregateSpec) ([]map[string]any, error) {
	found, err := m.Search(node)
	if err != nil {
		return nil, err
	}
	groupFields := readGroupReadFields(groupBy, aggregates)
	rows, err := found.Read(groupFields...)
	if err != nil {
		return nil, err
	}
	if len(groupBy) == 0 {
		row := map[string]any{"__domain": []any{}}
		for _, aggregate := range aggregates {
			row[aggregate.Key] = m.readGroupAggregateValue(aggregate, rows, len(rows))
		}
		return paginateReadGroupRows([]map[string]any{row}, opts), nil
	}
	type groupCandidate struct {
		values map[string]any
		specs  map[string]readGroupSpec
	}
	index := map[string]*readGroupBucket{}
	ordered := []*readGroupBucket{}
	for _, row := range rows {
		candidates := []groupCandidate{{values: map[string]any{}, specs: map[string]readGroupSpec{}}}
		for _, groupSpec := range groupBy {
			rawValue, rowSpec := m.readGroupRowValue(row, groupSpec)
			values, err := m.readGroupValues(rawValue, rowSpec)
			if err != nil {
				return nil, fmt.Errorf("read_group %s.%s: %w", m.model.Name, groupSpec.Name, err)
			}
			next := make([]groupCandidate, 0, len(candidates)*len(values))
			for _, candidate := range candidates {
				for _, value := range values {
					nextValues := cloneAnyMap(candidate.values)
					nextSpecs := cloneReadGroupSpecMap(candidate.specs)
					nextValues[groupSpec.Name] = value
					nextSpecs[groupSpec.Name] = rowSpec
					next = append(next, groupCandidate{values: nextValues, specs: nextSpecs})
				}
			}
			candidates = next
		}
		for _, candidate := range candidates {
			keyParts := make([]string, 0, len(groupBy))
			for _, groupSpec := range groupBy {
				keyParts = append(keyParts, readGroupKey(candidate.values[groupSpec.Name]))
			}
			key := strings.Join(keyParts, "\x00")
			group, ok := index[key]
			if !ok {
				group = &readGroupBucket{values: candidate.values, specs: candidate.specs}
				index[key] = group
				ordered = append(ordered, group)
			}
			group.rows = append(group.rows, row)
			group.count++
		}
	}
	allGroupValues := readGroupAllValues(groupBy, ordered)
	out := make([]map[string]any, 0, len(ordered))
	for _, group := range ordered {
		row := map[string]any{}
		for _, groupSpec := range groupBy {
			row[groupSpec.Name] = group.values[groupSpec.Name]
		}
		for _, aggregate := range aggregates {
			row[aggregate.Key] = m.readGroupAggregateValue(aggregate, group.rows, group.count)
		}
		bucketSpecs := readGroupBucketSpecs(groupBy, group.specs)
		row[readGroupInternalSpecsKey] = bucketSpecs
		row["__domain"] = readGroupDomain(bucketSpecs, group.values, allGroupValues)
		if ranges := readGroupRanges(bucketSpecs, group.values); len(ranges) > 0 {
			row["__range"] = ranges
		}
		out = append(out, row)
	}
	if err := m.sortReadGroupRows(out, groupBy, aggregates, opts.Order); err != nil {
		return nil, err
	}
	return paginateReadGroupRows(out, opts), nil
}

func (m ModelSet) FieldsGet(names []string, attributes []string) (map[string]map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	if err := m.check(OpRead, nil); err != nil {
		return nil, err
	}
	include := map[string]bool{}
	for _, name := range names {
		include[name] = true
	}
	filter := map[string]bool{}
	for _, attr := range attributes {
		filter[attr] = true
	}
	fieldNames := make([]string, 0, len(m.model.Fields))
	for _, name := range []string{"id", "display_name"} {
		if len(include) > 0 && !include[name] {
			continue
		}
		fieldNames = append(fieldNames, name)
	}
	for name := range m.model.Fields {
		if len(include) > 0 && !include[name] {
			continue
		}
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	fieldNames = m.filterFields(fieldNames)
	out := map[string]map[string]any{}
	for _, name := range fieldNames {
		if name == "id" {
			out[name] = filterDescription(fieldDescription(field.New("id", field.Int)), filter)
			continue
		}
		if name == "display_name" {
			out[name] = filterDescription(fieldDescription(field.New("display_name", field.Char)), filter)
			continue
		}
		out[name] = filterDescription(fieldDescription(m.model.Fields[name]), filter)
	}
	return out, nil
}

func (m ModelSet) DefaultGet(names []string, context map[string]any) (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	if err := m.check(OpCreate, nil); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, name := range names {
		if _, ok := m.model.Fields[name]; !ok {
			continue
		}
		if value, ok := context["default_"+name]; ok {
			out[name] = value
			continue
		}
		if value, ok := m.env.context.Values["default_"+name]; ok {
			out[name] = value
		}
	}
	if m.model.Name == "ir.actions.actions" || isConcreteActionModel(m.model.Name) {
		defaults := m.normalizeActionCreateValues(map[string]any{})
		for _, name := range names {
			if _, exists := out[name]; exists {
				continue
			}
			if value, ok := defaults[name]; ok {
				out[name] = value
			}
		}
	}
	return out, nil
}

func (m ModelSet) NameSearch(name string, node domain.Node, op domain.Operator, limit int) ([][2]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	nameNode := domain.And()
	if name != "" {
		nameNode = domain.Cond(m.recName(), op, name)
	}
	found, err := m.SearchWithOptions(domain.And(nameNode, node), SearchOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	return found.NameGet()
}

type RecordSet struct {
	model ModelSet
	ids   []int64
}

func (r RecordSet) IDs() []int64 {
	return append([]int64(nil), r.ids...)
}

func (r RecordSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.IDs())
}

func (r RecordSet) Len() int {
	return len(r.ids)
}

func (r RecordSet) Read(fields ...string) ([]map[string]any, error) {
	if r.model.err != nil {
		return nil, r.model.err
	}
	if err := r.model.check(OpRead, nil); err != nil {
		return nil, err
	}
	if r.model.model.Name == "res.groups" {
		r.model.syncAllResGroupsDerivedFields()
	}
	if r.model.model.Name == "res.users" {
		r.model.syncAllResUsersDerivedFields()
	}
	if r.model.model.Name == "mailing.contact" || r.model.model.Name == "mailing.list" || r.model.model.Name == "mailing.mailing" || r.model.model.Name == "utm.campaign" {
		r.model.syncAllMailingDerivedFields()
	}
	if r.model.model.Name == "marketing.trace" || r.model.model.Name == "whatsapp.message" {
		r.model.syncAllWhatsAppMarketingDerivedFields()
	}
	fields = r.model.filterFields(fields)
	var out []map[string]any
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		allowed, err := r.model.allowedRecord(OpRead, row)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		copy := map[string]any{"id": id}
		for _, fieldName := range fields {
			if fieldName == "id" {
				copy[fieldName] = id
				continue
			}
			value := row[fieldName]
			if meta, ok := r.model.model.Fields[fieldName]; ok && meta.Kind == field.Binary && r.model.binarySizeRequested(fieldName) {
				value = r.model.binarySizeValue(fieldName, value)
			}
			copy[fieldName] = value
		}
		out = append(out, copy)
	}
	return out, nil
}

func (m ModelSet) binarySizeRequested(fieldName string) bool {
	values := m.env.context.Values
	return truthyRecordValue(values["bin_size"]) || truthyRecordValue(values["bin_size_"+fieldName])
}

func (m ModelSet) binarySizeValue(fieldName string, value any) any {
	size, ok := m.binaryByteSize(fieldName, value)
	if !ok || size == 0 {
		return false
	}
	if m.model.Name == "ir.attachment" && fieldName == "datas" {
		return humanSize(size)
	}
	return prettyByteSize(size)
}

func (m ModelSet) binaryByteSize(fieldName string, value any) (int64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case []byte:
		return int64(len(typed)), true
	case string:
		if m.model.Name == "ir.attachment" && fieldName == "datas" {
			if decoded, err := base64.StdEncoding.DecodeString(typed); err == nil {
				return int64(len(decoded)), true
			}
		}
		return int64(len(typed)), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	default:
		text := fmt.Sprint(value)
		if text == "" || text == "<nil>" {
			return 0, false
		}
		return int64(len(text)), true
	}
}

func humanSize(size int64) string {
	units := []string{"bytes", "Kb", "Mb", "Gb", "Tb"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func prettyByteSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d bytes", size)
	}
	units := []string{"bytes", "kB", "MB", "GB", "TB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.0f %s", value, units[unit])
}

func (r RecordSet) WebRead(specification map[string]any) ([]map[string]any, error) {
	fields := make([]string, 0, len(specification))
	for name := range specification {
		if name == "display_name" {
			continue
		}
		fields = append(fields, name)
	}
	sort.Strings(fields)
	if len(fields) == 0 && len(specification) > 0 {
		fields = []string{"id"}
	}
	rows, err := r.Read(fields...)
	if err != nil {
		return nil, err
	}
	if _, needsDisplayName := specification["display_name"]; needsDisplayName {
		for _, row := range rows {
			id, _ := row["id"].(int64)
			if stored, ok := r.model.store.records[id]; ok {
				row["display_name"] = r.model.displayName(stored)
			}
		}
	}
	for _, fieldName := range fields {
		meta, ok := r.model.model.Fields[fieldName]
		if !ok || meta.Kind != field.Many2One {
			continue
		}
		for _, row := range rows {
			row[fieldName] = r.model.webMany2OneValue(meta, row[fieldName])
		}
	}
	for _, fieldName := range fields {
		meta, ok := r.model.model.Fields[fieldName]
		if !ok || (meta.Kind != field.One2Many && meta.Kind != field.Many2Many) {
			continue
		}
		fieldSpec := webReadSpecMap(specification[fieldName])
		if err := r.webReadX2ManyField(rows, meta, fieldSpec); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r RecordSet) webReadX2ManyField(rows []map[string]any, meta field.Field, fieldSpec map[string]any) error {
	if meta.Relation == "" {
		return nil
	}
	idsByParent := make(map[int64][]int64, len(rows))
	allIDs := []int64{}
	for _, row := range rows {
		parentID := numericID(row["id"])
		ids := r.model.webReadX2ManyIDs(parentID, row, meta)
		idsByParent[parentID] = ids
		allIDs = append(allIDs, ids...)
		row[meta.Name] = append([]int64(nil), ids...)
	}
	allIDs = uniqueRecordIDs(allIDs)
	if len(fieldSpec) == 0 || len(allIDs) == 0 {
		return nil
	}
	related := r.model.x2manyRelationModel(meta)
	if related.err != nil {
		return related.err
	}
	if order := strings.TrimSpace(stringValue(fieldSpec["order"])); order != "" {
		orderRelated := related.withActiveTest(false)
		found, err := orderRelated.SearchWithOptions(domain.Cond("id", domain.In, allIDs), SearchOptions{Order: order})
		if err != nil {
			return err
		}
		orderByID := map[int64]int{}
		for index, id := range found.IDs() {
			orderByID[id] = index
		}
		for _, row := range rows {
			parentID := numericID(row["id"])
			ids := idsByParent[parentID]
			ids = filterAndOrderWebReadX2ManyIDs(ids, orderByID)
			idsByParent[parentID] = ids
			row[meta.Name] = append([]int64(nil), ids...)
		}
	}
	childSpec, hasChildSpec := webReadChildSpec(fieldSpec)
	if !hasChildSpec {
		return nil
	}
	idsToRead := []int64{}
	limit, hasLimit := webReadLimit(fieldSpec)
	for _, row := range rows {
		parentID := numericID(row["id"])
		ids := idsByParent[parentID]
		if hasLimit {
			if limit < len(ids) {
				ids = ids[:limit]
			}
		}
		idsToRead = append(idsToRead, ids...)
	}
	idsToRead = uniqueRecordIDs(idsToRead)
	relatedEnv := related.env
	if context, ok := webReadContextValues(fieldSpec); ok {
		ctx := relatedEnv.Context()
		ctx.Values = mergeContextValues(ctx.Values, context)
		relatedEnv = relatedEnv.WithContext(ctx)
		related = relatedEnv.Model(meta.Relation)
	}
	childRows, err := related.Browse(idsToRead...).WebRead(childSpec)
	if err != nil {
		return err
	}
	childByID := map[int64]map[string]any{}
	for _, childRow := range childRows {
		childByID[numericID(childRow["id"])] = childRow
	}
	for _, row := range rows {
		parentID := numericID(row["id"])
		items := make([]map[string]any, 0, len(idsByParent[parentID]))
		for _, id := range idsByParent[parentID] {
			if childRow, ok := childByID[id]; ok {
				items = append(items, childRow)
			} else {
				items = append(items, map[string]any{"id": id})
			}
		}
		row[meta.Name] = items
	}
	return nil
}

func (m ModelSet) webReadX2ManyIDs(parentID int64, row map[string]any, meta field.Field) []int64 {
	if meta.Kind == field.One2Many && meta.Relation != "" && meta.RelationField != "" && parentID != 0 {
		found, err := m.x2manyRelationModel(meta).Search(domain.Cond(meta.RelationField, domain.Equal, parentID))
		if err == nil {
			return found.IDs()
		}
	}
	ids := uniqueRecordIDs(int64Values(row[meta.Name]))
	if meta.Kind == field.Many2Many && len(ids) > 0 && meta.Relation != "" {
		found, err := m.x2manyRelationModel(meta).Search(domain.Cond("id", domain.In, ids))
		if err == nil {
			return found.IDs()
		}
	}
	return ids
}

func (m ModelSet) x2manyRelationModel(meta field.Field) ModelSet {
	related := m.env.Model(meta.Relation)
	if len(meta.Context) == 0 || related.err != nil {
		return related
	}
	ctx := related.env.Context()
	ctx.Values = mergeContextValues(ctx.Values, meta.Context)
	return related.env.WithContext(ctx).Model(meta.Relation)
}

func (m ModelSet) withActiveTest(active bool) ModelSet {
	if m.err != nil {
		return m
	}
	ctx := m.env.Context()
	ctx.Values = mergeContextValues(ctx.Values, map[string]any{"active_test": active})
	return m.env.WithContext(ctx).Model(m.model.Name)
}

func filterAndOrderWebReadX2ManyIDs(ids []int64, orderByID map[int64]int) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := orderByID[id]; ok {
			out = append(out, id)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return orderByID[out[i]] < orderByID[out[j]]
	})
	return out
}

func webReadSpecMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed
	}
	return map[string]any{}
}

func webReadChildSpec(spec map[string]any) (map[string]any, bool) {
	value, ok := spec["fields"]
	if !ok {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed, true
	}
	return map[string]any{}, true
}

func webReadLimit(spec map[string]any) (int, bool) {
	value, ok := spec["limit"]
	if !ok {
		return 0, false
	}
	limit := int(numericID(value))
	if limit < 0 {
		limit = 0
	}
	return limit, true
}

func webReadContextValues(spec map[string]any) (map[string]any, bool) {
	value, ok := spec["context"]
	if !ok {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed, true
	}
	return map[string]any{}, true
}

func mergeContextValues(base map[string]any, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func (m ModelSet) webMany2OneValue(meta field.Field, value any) any {
	id := numericID(value)
	if id == 0 {
		return false
	}
	relatedRow := m.rowByID(meta.Relation, id)
	if relatedRow == nil {
		return []any{id, ""}
	}
	relatedModel, ok := m.env.registry.Model(meta.Relation)
	if !ok {
		return []any{id, ""}
	}
	return []any{id, ModelSet{env: m.env, model: relatedModel}.displayName(relatedRow)}
}

func (r RecordSet) NameGet() ([][2]any, error) {
	if r.model.err != nil {
		return nil, r.model.err
	}
	rows, err := r.Read(r.model.recName())
	if err != nil {
		return nil, err
	}
	out := make([][2]any, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(int64)
		if stored, ok := r.model.store.records[id]; ok {
			out = append(out, [2]any{id, r.model.displayName(stored)})
			continue
		}
		out = append(out, [2]any{id, row[r.model.recName()]})
	}
	return out, nil
}

func (r RecordSet) Write(values map[string]any) error {
	if r.model.err != nil {
		return r.model.err
	}
	if err := r.model.check(OpWrite, values); err != nil {
		return err
	}
	needsActionBase := r.model.needsActionBaseSync()
	envSnapshot := map[string]storeSnapshot{}
	if len(r.model.env.afterWriteHooks) > 0 || needsActionBase || r.model.model.Name == "account.move" || r.model.model.Name == "mail.alias.domain" || r.model.model.Name == "link.tracker" || r.model.model.Name == "mailing.mailing" || r.model.model.Name == "utm.campaign" || r.model.model.Name == "fetchmail.server" {
		envSnapshot = r.model.snapshotEnv()
	}
	oldRows := map[int64]map[string]any{}
	writeValues := map[int64]map[string]any{}
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		allowed, err := r.model.allowedRecord(OpWrite, row)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("record rule denied write on %s:%d", r.model.model.Name, id)
		}
		oldRows[id] = copyValues(row)
		rowValues := r.model.normalizeRecordWriteValues(row, values)
		rowValues, err = r.model.sanitizeIrConfigParameterValues(row, rowValues)
		if err != nil {
			return err
		}
		if r.model.model.Name == "res.partner" && partnerWriteArchives(rowValues) && r.model.hasActiveLinkedUser(id) {
			return fmt.Errorf("cannot archive contact linked to an active user")
		}
		rowValues = r.model.withWriteLogAccessValues(rowValues, time.Now().UTC())
		for key := range rowValues {
			if _, ok := r.model.model.Fields[key]; !ok {
				return fmt.Errorf("unknown field %s.%s", r.model.model.Name, key)
			}
		}
		if err := r.model.validateMailMailWrite(row, rowValues); err != nil {
			return err
		}
		if err := r.model.validateIrAttachmentWrite(row, rowValues); err != nil {
			return err
		}
		if err := r.model.validateAccountingWrite(row, rowValues); err != nil {
			return err
		}
		if r.model.model.Name == "link.tracker" {
			if err := r.model.validateLinkTrackerWrite(row, rowValues); err != nil {
				return err
			}
		}
		if r.model.model.Name == "link.tracker.code" {
			merged := copyValues(row)
			for key, value := range rowValues {
				merged[key] = value
			}
			if err := r.model.validateLinkTrackerCodeUnique(merged); err != nil {
				return err
			}
		}
		writeValues[id] = rowValues
	}
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		for key, value := range writeValues[id] {
			row[key] = value
		}
	}
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		if err := r.model.validateModelConstraints(row); err != nil {
			for oldID, oldRow := range oldRows {
				r.model.store.records[oldID] = oldRow
			}
			return err
		}
	}
	if r.model.model.Name == "account.move" {
		moveIDs := []int64{}
		for _, id := range r.ids {
			oldRow, ok := oldRows[id]
			if !ok {
				continue
			}
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if accountMoveStateDetachesInvoicePDF(oldRow, row) {
				moveIDs = append(moveIDs, id)
			}
		}
		r.model.detachAccountMoveInvoicePDFs(moveIDs)
	}
	if r.model.model.Name == "account.move" && accountMoveWriteRecomputesLines(values) {
		for _, id := range r.ids {
			if err := r.model.recomputeMoveLines(id); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	if r.model.model.Name == "res.company" && companyWriteChangesSoftLocks(values) {
		lockExceptionSnapshot := r.model.snapshotStore("account.lock_exception")
		for _, id := range r.ids {
			if err := r.model.recreateCompanyLockExceptions(id, values); err != nil {
				for oldID, oldRow := range oldRows {
					r.model.store.records[oldID] = oldRow
				}
				r.model.restoreStore("account.lock_exception", lockExceptionSnapshot)
				return err
			}
		}
	}
	if r.model.model.Name == "account.move.line" {
		mailSnapshot := r.model.snapshotStore("mail.message")
		trackingSnapshot := r.model.snapshotStore("mail.tracking.value")
		if err := r.model.trackAccountMoveLineWrite(oldRows); err != nil {
			for oldID, oldRow := range oldRows {
				r.model.store.records[oldID] = oldRow
			}
			r.model.restoreStore("mail.message", mailSnapshot)
			r.model.restoreStore("mail.tracking.value", trackingSnapshot)
			return err
		}
	}
	if r.model.model.Name == "mail.activity.type" {
		r.model.syncMailActivityTypePreviousIDs()
	}
	if r.model.model.Name == "mailing.mailing" {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if err := r.model.ensureMailingABTestingCampaign(row); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
		r.model.syncAllMailingDerivedFields()
	}
	if r.model.model.Name == "utm.campaign" {
		r.model.syncAllMailingDerivedFields()
	}
	if r.model.model.Name == "mail.alias.domain" && hasAnyKey(values, "name") {
		for _, id := range r.ids {
			if err := r.model.recomputeMailAliasesForDomain(id); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	if r.model.model.Name == "res.groups" {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			r.model.syncResGroupsInverseFields(id, oldRows[id], row)
		}
		r.model.syncAllResGroupsDerivedFields()
		r.model.syncAllResUsersDerivedFields()
	}
	if r.model.model.Name == "ir.model.data" {
		changedGroups := false
		for _, id := range r.ids {
			row := r.model.store.records[id]
			if stringValue(row["model"]) == "res.groups" || stringValue(safeRowValue(oldRows[id], "model")) == "res.groups" {
				changedGroups = true
				break
			}
		}
		if changedGroups {
			r.model.syncAllResGroupsDerivedFields()
			r.model.syncAllResUsersDerivedFields()
		}
	}
	if r.model.modelChangesResGroupDerivedFields() {
		r.model.syncAllResGroupsDerivedFields()
	}
	if r.model.model.Name == "res.users" {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if active, ok := values["active"]; ok && truthyRecordValue(active) {
				r.model.syncResUserPartnerActive(row, true)
			}
			r.model.applyResUsersDerivedFields(id, row)
			r.model.syncResUserPartnerShare(oldRows[id], row)
		}
		r.model.syncAllResGroupsDerivedFields()
		if hasAnyKey(values, "active", "groups_id", "group_ids") {
			for _, id := range r.ids {
				row, ok := r.model.store.records[id]
				if !ok {
					continue
				}
				r.model.deactivateInvalidDelegationLinesForUser(row, time.Now().UTC())
			}
		}
	}
	if r.model.model.Name == "res.partner" {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			r.model.applyResPartnerDerivedFields(id, row)
			r.model.syncResPartnerDependents(id)
		}
	}
	if needsActionBase {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if err := r.model.syncActionBaseRow(id, row); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	if r.model.model.Name == "link.tracker" {
		for _, id := range r.ids {
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if err := r.model.syncLinkTrackerDerivedFields(id, row, writeValues[id]); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	if r.model.model.Name == "phone.blacklist" {
		r.model.syncAllPhoneBlacklistDerivedFields()
	}
	if r.model.model.Name == "marketing.trace" || r.model.model.Name == "whatsapp.message" {
		r.model.syncAllWhatsAppMarketingDerivedFields()
	}
	if r.model.model.Name == "fetchmail.server" {
		if err := r.model.syncFetchmailGatewayCron(); err != nil {
			r.model.restoreEnv(envSnapshot)
			return err
		}
	}
	for _, hook := range r.model.env.afterWriteHooks {
		for _, id := range r.ids {
			oldRow, ok := oldRows[id]
			if !ok {
				continue
			}
			row, ok := r.model.store.records[id]
			if !ok {
				continue
			}
			if err := hook(r.model.env, r.model.model.Name, id, copyValues(oldRow), copyValues(row), copyValues(writeValues[id])); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	return nil
}

func accountMoveWriteRecomputesLines(values map[string]any) bool {
	for _, fieldName := range []string{"fiscal_position_id", "move_type", "company_id"} {
		if _, ok := values[fieldName]; ok {
			return true
		}
	}
	return false
}

func (m ModelSet) recomputeMoveLines(moveID int64) error {
	lineStore, ok := m.env.stores["account.move.line"]
	if !ok {
		return nil
	}
	for _, row := range lineStore.records {
		if numericID(row["move_id"]) != moveID {
			continue
		}
		values := m.normalizeAccountMoveLineValues(row, map[string]any{})
		for key, value := range values {
			row[key] = value
		}
	}
	return nil
}

func (m ModelSet) normalizeLockExceptionValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	merged := copyValues(existing)
	for key, value := range out {
		merged[key] = value
	}
	if existing == nil {
		if _, ok := out["active"]; !ok {
			out["active"] = true
			merged["active"] = true
		}
	}
	if activeValue, ok := merged["active"]; ok && !truthyRecordValue(activeValue) {
		out["active"] = false
		out["state"] = string(coreaccounting.LockExceptionRevoked)
		if _, ok := merged["end_datetime"]; !ok || recordDateValue(merged["end_datetime"]).IsZero() {
			out["end_datetime"] = time.Now().UTC()
		}
		return out
	}
	lockException := m.lockExceptionFromValues(merged)
	if lockException.LockDateField == "" {
		if field, lockDate, ok := explicitLockExceptionDate(merged); ok {
			lockException.LockDateField = field
			lockException.LockDate = lockDate
		}
	}
	if lockException.LockDateField == "" || lockException.LockDate.IsZero() {
		normalized, err := coreaccounting.NewLockException(lockException, m.companyLockPolicy(lockException.CompanyID))
		if err == nil {
			lockException = normalized
		}
	} else if lockException.CompanyLockDate.IsZero() {
		lockException.CompanyLockDate = m.companyLockDate(lockException.CompanyID, lockException.LockDateField)
	}
	out["active"] = true
	out["lock_date_field"] = string(lockException.LockDateField)
	out["lock_date"] = lockException.LockDate
	out["company_lock_date"] = lockException.CompanyLockDate
	out["state"] = string(lockException.StateAt(time.Now().UTC()))
	return out
}

func explicitLockExceptionDate(values map[string]any) (coreaccounting.LockDateKind, time.Time, bool) {
	var field coreaccounting.LockDateKind
	var lockDate time.Time
	for _, candidate := range softLockDateFields() {
		value, ok := values[string(candidate)]
		if !ok {
			continue
		}
		if field != "" {
			return "", time.Time{}, false
		}
		field = candidate
		lockDate = recordDateValue(value)
	}
	return field, lockDate, field != ""
}

func explicitLockExceptionDateCount(values map[string]any) int {
	var count int
	for _, candidate := range softLockDateFields() {
		if _, ok := values[string(candidate)]; ok {
			count++
		}
	}
	return count
}

func (m ModelSet) lockExceptionFromValues(values map[string]any) coreaccounting.LockException {
	return coreaccounting.LockException{
		ID:                 numericID(values["id"]),
		Active:             truthyRecordValue(values["active"]),
		CompanyID:          numericID(values["company_id"]),
		UserID:             numericID(values["user_id"]),
		Reason:             stringValue(values["reason"]),
		EndDatetime:        recordDateValue(values["end_datetime"]),
		LockDateField:      coreaccounting.LockDateKind(stringValue(values["lock_date_field"])),
		LockDate:           recordDateValue(values["lock_date"]),
		CompanyLockDate:    recordDateValue(values["company_lock_date"]),
		FiscalYearLockDate: recordDateValue(values["fiscalyear_lock_date"]),
		TaxLockDate:        recordDateValue(values["tax_lock_date"]),
		SaleLockDate:       recordDateValue(values["sale_lock_date"]),
		PurchaseLockDate:   recordDateValue(values["purchase_lock_date"]),
	}
}

func (m ModelSet) companyLockPolicy(companyID int64) coreaccounting.LockPolicy {
	row := m.rowByID("res.company", companyID)
	return coreaccounting.LockPolicy{
		FiscalLockDate:        recordDateValue(row["fiscalyear_lock_date"]),
		TaxLockDate:           recordDateValue(row["tax_lock_date"]),
		SaleLockDate:          recordDateValue(row["sale_lock_date"]),
		PurchaseLockDate:      recordDateValue(row["purchase_lock_date"]),
		HardLockDate:          recordDateValue(row["hard_lock_date"]),
		RestrictiveAuditTrail: truthyRecordValue(row["restrictive_audit_trail"]),
	}
}

func (m ModelSet) EffectiveAccountLockPolicy(companyID int64) coreaccounting.LockPolicy {
	if companyID == 0 {
		companyID = m.env.context.CompanyID
	}
	return coreaccounting.EffectiveLockPolicy(m.companyLockPolicyChain(companyID), m.env.context.UserID, m.lockExceptions(), time.Now().UTC())
}

func (m ModelSet) companyLockPolicyChain(companyID int64) []coreaccounting.CompanyLockPolicy {
	var chain []coreaccounting.CompanyLockPolicy
	seen := map[int64]bool{}
	for companyID != 0 && !seen[companyID] {
		seen[companyID] = true
		row := m.rowByID("res.company", companyID)
		if row == nil {
			break
		}
		chain = append(chain, coreaccounting.CompanyLockPolicy{CompanyID: companyID, Locks: m.companyLockPolicy(companyID)})
		companyID = numericID(row["parent_id"])
	}
	return chain
}

func (m ModelSet) lockExceptions() []coreaccounting.LockException {
	rows := m.rows("account.lock_exception")
	out := make([]coreaccounting.LockException, 0, len(rows))
	for _, row := range rows {
		out = append(out, m.lockExceptionFromValues(row))
	}
	return out
}

func (m ModelSet) recreateCompanyLockExceptions(companyID int64, values map[string]any) error {
	company := m.rowByID("res.company", companyID)
	if company == nil {
		return nil
	}
	now := time.Now().UTC()
	for _, field := range softLockDateFields() {
		if _, changed := values[string(field)]; !changed {
			continue
		}
		companyLockDate := recordDateValue(company[string(field)])
		if companyLockDate.IsZero() {
			continue
		}
		for _, row := range m.rows("account.lock_exception") {
			exception := m.lockExceptionFromValues(row)
			if exception.CompanyID != companyID || exception.LockDateField != field || exception.StateAt(now) != coreaccounting.LockExceptionActive {
				continue
			}
			if !exception.LockDate.IsZero() && !exception.LockDate.Before(companyLockDate) {
				continue
			}
			values := map[string]any{
				"company_id":        exception.CompanyID,
				"user_id":           exception.UserID,
				"reason":            exception.Reason,
				"end_datetime":      exception.EndDatetime,
				"lock_date_field":   string(exception.LockDateField),
				"lock_date":         exception.LockDate,
				"company_lock_date": companyLockDate,
			}
			if _, err := m.env.Model("account.lock_exception").Create(values); err != nil {
				return err
			}
			if err := m.env.Model("account.lock_exception").Browse(numericID(row["id"])).RevokeAccountLockExceptions(true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m ModelSet) companyLockDate(companyID int64, field coreaccounting.LockDateKind) time.Time {
	locks := m.companyLockPolicy(companyID)
	switch field {
	case coreaccounting.LockFiscalYear:
		return locks.FiscalLockDate
	case coreaccounting.LockTax:
		return locks.TaxLockDate
	case coreaccounting.LockSale:
		return locks.SaleLockDate
	case coreaccounting.LockPurchase:
		return locks.PurchaseLockDate
	default:
		return time.Time{}
	}
}

func softLockDateFields() []coreaccounting.LockDateKind {
	return []coreaccounting.LockDateKind{
		coreaccounting.LockFiscalYear,
		coreaccounting.LockTax,
		coreaccounting.LockSale,
		coreaccounting.LockPurchase,
	}
}

func companyWriteChangesSoftLocks(values map[string]any) bool {
	for _, field := range softLockDateFields() {
		if _, ok := values[string(field)]; ok {
			return true
		}
	}
	return false
}

func (m ModelSet) normalizeRecordWriteValues(existing map[string]any, values map[string]any) map[string]any {
	switch m.model.Name {
	case "account.account":
		if _, hasCode := values["code"]; hasCode {
			return m.normalizeAccountAccountValues(existing, values)
		}
		if _, hasCompany := values["company_id"]; hasCompany {
			return m.normalizeAccountAccountValues(existing, values)
		}
		if _, hasType := values["account_type"]; hasType {
			return m.normalizeAccountAccountValues(existing, values)
		}
		if _, hasName := values["name"]; hasName && stringValue(existing["code"]) == "" {
			return m.normalizeAccountAccountValues(existing, values)
		}
	case "account.move.line":
		for _, fieldName := range []string{"move_id", "product_id", "account_id", "company_id", "account_type"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeAccountMoveLineValues(existing, values)
			}
		}
	case "account.lock_exception":
		for _, fieldName := range []string{"active", "end_datetime", "lock_date_field", "lock_date", "company_lock_date", "fiscalyear_lock_date", "tax_lock_date", "sale_lock_date", "purchase_lock_date"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeLockExceptionValues(existing, values)
			}
		}
	case "ir.attachment":
		if stringValue(values["res_model"]) == "documents.document" && !irAttachmentWriteTouchesContent(values) && m.irAttachmentAuditTrailProtected(existing) {
			out := copyValues(values)
			delete(out, "res_model")
			delete(out, "res_id")
			return out
		}
	case "mail.activity":
		for _, fieldName := range []string{"activity_type_id", "recommended_activity_type_id", "previous_activity_type_id", "automated"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailActivityValues(existing, values)
			}
		}
	case "mail.activity.type":
		for _, fieldName := range []string{"category", "chaining_type", "triggered_next_type_id", "suggested_next_type_ids"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailActivityTypeValues(existing, values)
			}
		}
	case "mail.alias":
		for _, fieldName := range []string{"alias_name", "alias_domain_id", "alias_contact", "alias_defaults", "alias_model_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailAliasValues(existing, values)
			}
		}
	case "mail.alias.domain":
		for _, fieldName := range []string{"name", "bounce_alias", "catchall_alias", "default_from"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailAliasDomainValues(existing, values)
			}
		}
	case "mail.blacklist":
		if _, ok := values["email"]; ok {
			return m.normalizeMailBlacklistValues(existing, values)
		}
	case "phone.blacklist":
		for _, fieldName := range []string{"number", "active"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizePhoneBlacklistValues(existing, values)
			}
		}
	case "res.partner":
		for _, fieldName := range []string{"phone", "country_id", "active"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeResPartnerValues(existing, values)
			}
		}
	case "mailing.contact":
		for _, fieldName := range []string{"email", "phone", "active"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailingContactValues(existing, values)
			}
		}
	case "mailing.subscription":
		for _, fieldName := range []string{"opt_out", "opt_out_datetime", "opt_out_reason_id", "contact_id", "list_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailingSubscriptionValues(existing, values)
			}
		}
	case "mailing.mailing":
		for _, fieldName := range []string{"ab_testing_enabled", "ab_testing_pc", "ab_testing_schedule_datetime", "ab_testing_winner_selection"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailingMailingValues(existing, values)
			}
		}
	case "utm.campaign":
		for _, fieldName := range []string{"ab_testing_winner_selection", "ab_testing_winner_mailing_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeUTMCampaignValues(existing, values)
			}
		}
	case "mailing.trace":
		for _, fieldName := range []string{"mail_mail_id", "email", "mass_mailing_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeMailingTraceValues(existing, values)
			}
		}
	case "link.tracker":
		for _, fieldName := range []string{"url", "campaign_id", "source_id", "medium_id", "redirected_url"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeLinkTrackerValues(existing, values)
			}
		}
	case "sms.sms":
		for _, fieldName := range []string{"uuid", "state", "to_delete"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeSMSSMSValues(existing, values)
			}
		}
	case "sms.tracker":
		if _, ok := values["sms_uuid"]; ok {
			return m.normalizeSMSTrackerValues(existing, values)
		}
	case "whatsapp.template":
		for _, fieldName := range []string{"status", "quality", "lang_code", "template_type", "header_type", "phone_field", "model", "model_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeWhatsAppTemplateValues(existing, values)
			}
		}
	case "whatsapp.message", "whatsapp.template.button":
		for _, fieldName := range []string{"wa_template_id", "template_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeWhatsAppTemplateAliasValues(existing, values)
			}
		}
	case "res.groups":
		for _, fieldName := range []string{"category_id", "restricted_access"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeResGroupsValues(existing, values)
			}
		}
	case "res.groups.privilege":
		for _, fieldName := range []string{"sequence", "placeholder"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeResGroupsPrivilegeValues(existing, values)
			}
		}
	case "res.users":
		for _, fieldName := range []string{"group_ids", "groups_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeResUsersValues(existing, values)
			}
		}
	case "delegation":
		for _, fieldName := range []string{"employee_id", "delegateTo_employee_id", "delegate_to_employee_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeDelegationValues(existing, values)
			}
		}
	case "delegation.line":
		for _, fieldName := range []string{"delegation_id", "employee_id", "group_id"} {
			if _, ok := values[fieldName]; ok {
				return m.normalizeDelegationLineValues(existing, values)
			}
		}
	case "ir.model.data":
		return m.normalizeIrModelDataValues(existing, values)
	}
	return values
}

func (m ModelSet) normalizeIrModelDataValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	moduleName := stringValue(safeRowValue(existing, "module"))
	if value, ok := out["module"]; ok {
		moduleName = stringValue(value)
	} else if existing == nil {
		out["module"] = ""
	}
	name := stringValue(safeRowValue(existing, "name"))
	if value, ok := out["name"]; ok {
		name = stringValue(value)
	}
	if existing == nil {
		if _, ok := out["noupdate"]; !ok && m.hasField("noupdate") {
			out["noupdate"] = false
		}
	}
	if m.hasField("complete_name") {
		out["complete_name"] = irModelDataCompleteName(moduleName, name)
	}
	return out
}

func (m ModelSet) hasField(name string) bool {
	_, ok := m.model.Fields[name]
	return ok
}

func irModelDataCompleteName(moduleName string, name string) string {
	moduleName = strings.TrimSpace(moduleName)
	name = strings.TrimSpace(name)
	switch {
	case moduleName == "":
		return name
	case name == "":
		return moduleName
	default:
		return moduleName + "." + name
	}
}

func (m ModelSet) validateIrModelData(row map[string]any) error {
	name := stringValue(row["name"])
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("ir.model.data external id name is required")
	}
	if strings.Contains(name, " ") {
		return fmt.Errorf("external ids cannot contain spaces")
	}
	moduleName := stringValue(row["module"])
	rowID := numericID(row["id"])
	for id, current := range m.store.records {
		if id == rowID {
			continue
		}
		if stringValue(current["module"]) == moduleName && stringValue(current["name"]) == name {
			return fmt.Errorf("ir.model.data external id %s already exists", irModelDataCompleteName(moduleName, name))
		}
	}
	return nil
}

func (m ModelSet) normalizeResUsersValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	value, ok := out["group_ids"]
	if !ok {
		value, ok = out["groups_id"]
	}
	if !ok {
		return out
	}
	current := uniqueRecordIDs(append(int64Values(safeRowValue(existing, "group_ids")), int64Values(safeRowValue(existing, "groups_id"))...))
	ids := normalizeX2ManyRecordIDs(current, value)
	if m.fieldExists("res.users", "group_ids") {
		out["group_ids"] = ids
	}
	if m.fieldExists("res.users", "groups_id") {
		out["groups_id"] = ids
	}
	return out
}

func (m ModelSet) deactivateInvalidDelegationLinesForUser(user map[string]any, at time.Time) {
	userID := numericID(user["id"])
	if userID == 0 {
		return
	}
	today := dateOnlyTime(at)
	active := m.rowIsActive("res.users", user)
	groupSet := map[int64]bool{}
	for _, groupID := range uniqueRecordIDs(append(int64Values(user["groups_id"]), int64Values(user["group_ids"])...)) {
		groupSet[groupID] = true
	}
	store, ok := m.env.stores["delegation.line"]
	if !ok {
		return
	}
	for _, line := range store.records {
		if numericID(line["delegator_user_id"]) != userID || line["active"] == false || stringValue(line["state"]) != "confirmed" {
			continue
		}
		dateTo := dateOnlyTime(recordDateValue(line["date_to"]))
		if dateTo.IsZero() || dateTo.Before(today) {
			continue
		}
		if active && groupSet[numericID(line["group_id"])] {
			continue
		}
		line["active"] = false
	}
}

func normalizeX2ManyRecordIDs(current []int64, value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return uniqueSortedRecordIDs(typed)
	case []any:
		if len(typed) == 0 {
			return []int64{}
		}
		if x2ManyCommandItems(typed) {
			ids := append([]int64(nil), current...)
			for _, item := range typed {
				ids = applyX2ManyRecordCommand(ids, item.([]any))
			}
			return uniqueSortedRecordIDs(ids)
		}
		return uniqueSortedRecordIDs(int64Values(typed))
	default:
		return uniqueSortedRecordIDs(int64Values(typed))
	}
}

func x2ManyCommandItems(items []any) bool {
	for _, item := range items {
		if command, ok := item.([]any); !ok || len(command) == 0 {
			return false
		}
	}
	return true
}

func applyX2ManyRecordCommand(ids []int64, command []any) []int64 {
	switch numericID(command[0]) {
	case 2, 3:
		if len(command) > 1 {
			return removeRecordID(ids, numericID(command[1]))
		}
	case 4:
		if len(command) > 1 {
			return appendUniqueID(ids, numericID(command[1]))
		}
	case 5:
		return []int64{}
	case 6:
		if len(command) > 2 {
			return uniqueRecordIDs(int64Values(command[2]))
		}
	}
	return ids
}

func (m ModelSet) applyIrExportsLineCommands(exportID int64, payload any) ([]int64, error) {
	commands, ok := payload.([]any)
	if !ok {
		return normalizeX2ManyRecordIDs(nil, payload), nil
	}
	ids := []int64{}
	lineModel := m.env.Model("ir.exports.line")
	for _, item := range commands {
		command, ok := item.([]any)
		if !ok || len(command) == 0 {
			continue
		}
		switch numericID(command[0]) {
		case 0:
			values := copyValues(commandMapValue(commandValue(command, 2)))
			values["export_id"] = exportID
			id, err := lineModel.Create(values)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		case 2, 3:
			if len(command) > 1 {
				id := numericID(command[1])
				if err := lineModel.Browse(id).Unlink(); err != nil {
					return nil, err
				}
				ids = removeRecordID(ids, id)
			}
		case 4:
			if len(command) > 1 {
				id := numericID(command[1])
				if err := lineModel.Browse(id).Write(map[string]any{"export_id": exportID}); err != nil {
					return nil, err
				}
				ids = appendUniqueID(ids, id)
			}
		case 5:
			for _, id := range ids {
				if err := lineModel.Browse(id).Unlink(); err != nil {
					return nil, err
				}
			}
			ids = []int64{}
		case 6:
			nextIDs := uniqueRecordIDs(int64Values(commandValue(command, 2)))
			for _, id := range nextIDs {
				if err := lineModel.Browse(id).Write(map[string]any{"export_id": exportID}); err != nil {
					return nil, err
				}
			}
			ids = nextIDs
		}
	}
	return ids, nil
}

func commandValue(command []any, index int) any {
	if index < 0 || index >= len(command) {
		return nil
	}
	return command[index]
}

func commandMapValue(value any) map[string]any {
	typed, _ := value.(map[string]any)
	if typed == nil {
		return map[string]any{}
	}
	return typed
}

func uniqueSortedRecordIDs(ids []int64) []int64 {
	out := uniqueRecordIDs(ids)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (m ModelSet) normalizeResGroupsPrivilegeValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if _, ok := out["sequence"]; !ok && existing == nil && m.fieldExists("res.groups.privilege", "sequence") {
		out["sequence"] = int64(100)
	}
	if _, ok := out["placeholder"]; !ok && existing == nil && m.fieldExists("res.groups.privilege", "placeholder") {
		out["placeholder"] = "No"
	}
	return out
}

func (m ModelSet) normalizeResGroupsValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	merged := copyValues(existing)
	for key, value := range out {
		merged[key] = value
	}
	if _, ok := m.model.Fields["restricted_access"]; ok && m.isUserTypeGroupCategory(numericID(merged["category_id"])) {
		out["restricted_access"] = true
	}
	if _, ok := out["share"]; !ok && existing == nil && m.fieldExists("res.groups", "share") {
		out["share"] = false
	}
	if _, ok := out["sequence"]; !ok && existing == nil && m.fieldExists("res.groups", "sequence") {
		out["sequence"] = int64(0)
	}
	for _, fieldName := range []string{"implied_ids", "implied_by_ids", "user_ids"} {
		if value, ok := out[fieldName]; ok {
			ids := int64Values(value)
			if len(ids) == 0 {
				out[fieldName] = []int64{}
			} else {
				out[fieldName] = ids
			}
		}
	}
	return out
}

func (m ModelSet) syncResGroupsInverseFields(groupID int64, oldRow map[string]any, newRow map[string]any) {
	m.syncResGroupsImpliedBy(groupID, int64Values(safeRowValue(oldRow, "implied_by_ids")), int64Values(safeRowValue(newRow, "implied_by_ids")))
	m.syncResGroupsUserIDs(groupID, int64Values(safeRowValue(oldRow, "user_ids")), int64Values(safeRowValue(newRow, "user_ids")))
}

func (m ModelSet) syncResGroupsImpliedBy(groupID int64, oldParentIDs []int64, newParentIDs []int64) {
	groupStore, ok := m.env.stores["res.groups"]
	if !ok {
		return
	}
	for _, parentID := range oldParentIDs {
		if idInSlice(parentID, newParentIDs) {
			continue
		}
		if row, ok := groupStore.records[parentID]; ok {
			row["implied_ids"] = removeRecordID(int64Values(row["implied_ids"]), groupID)
		}
	}
	for _, parentID := range newParentIDs {
		if row, ok := groupStore.records[parentID]; ok {
			row["implied_ids"] = appendUniqueID(int64Values(row["implied_ids"]), groupID)
		}
	}
}

func (m ModelSet) syncResGroupsUserIDs(groupID int64, oldUserIDs []int64, newUserIDs []int64) {
	userStore, ok := m.env.stores["res.users"]
	if !ok {
		return
	}
	for _, userID := range oldUserIDs {
		if idInSlice(userID, newUserIDs) {
			continue
		}
		if row, ok := userStore.records[userID]; ok {
			row["groups_id"] = removeRecordID(int64Values(row["groups_id"]), groupID)
			row["group_ids"] = removeRecordID(int64Values(row["group_ids"]), groupID)
			m.applyResUsersDerivedFields(userID, row)
			m.syncResUserPartnerShare(row, row)
		}
	}
	for _, userID := range newUserIDs {
		if row, ok := userStore.records[userID]; ok {
			row["groups_id"] = appendUniqueID(int64Values(row["groups_id"]), groupID)
			row["group_ids"] = appendUniqueID(int64Values(row["group_ids"]), groupID)
			m.applyResUsersDerivedFields(userID, row)
			m.syncResUserPartnerShare(row, row)
		}
	}
}

func (m ModelSet) syncAllResGroupsDerivedFields() {
	groupStore, ok := m.env.stores["res.groups"]
	if !ok {
		return
	}
	for groupID, row := range groupStore.records {
		m.applyResGroupsDerivedFields(groupID, row)
	}
	if m.fieldExists("res.groups", "view_group_hierarchy") {
		hierarchy := m.resGroupsViewGroupHierarchy()
		for _, row := range groupStore.records {
			row["view_group_hierarchy"] = hierarchy
		}
	}
	m.syncAllResGroupsPrivilegeDerivedFields()
}

func (m ModelSet) modelChangesResGroupDerivedFields() bool {
	switch m.model.Name {
	case "res.groups.privilege", "ir.module.category":
		return true
	default:
		return false
	}
}

func (m ModelSet) syncAllResGroupsPrivilegeDerivedFields() {
	if !m.fieldExists("res.groups.privilege", "group_ids") {
		return
	}
	privilegeStore, ok := m.env.stores["res.groups.privilege"]
	if !ok {
		return
	}
	for privilegeID, row := range privilegeStore.records {
		row["group_ids"] = m.relatedRowsByField("res.groups", "privilege_id", privilegeID)
	}
}

func (m ModelSet) applyResGroupsDerivedFields(groupID int64, row map[string]any) {
	if groupID == 0 || row == nil {
		return
	}
	if m.fieldExists("res.groups", "full_name") {
		row["full_name"] = m.resGroupFullName(row)
	}
	if m.fieldExists("res.groups", "all_implied_ids") {
		row["all_implied_ids"] = m.resGroupClosure(groupID, false)
	}
	if m.fieldExists("res.groups", "all_implied_by_ids") {
		row["all_implied_by_ids"] = m.resGroupClosure(groupID, true)
	}
	if m.fieldExists("res.groups", "disjoint_ids") {
		row["disjoint_ids"] = m.resGroupDisjointIDs(groupID)
	}
	if m.fieldExists("res.groups", "all_user_ids") {
		row["all_user_ids"] = m.resGroupAllUserIDs(groupID)
	}
	if m.fieldExists("res.groups", "all_users_count") {
		row["all_users_count"] = int64(len(int64Values(row["all_user_ids"])))
	}
	if m.fieldExists("res.groups", "model_access") {
		row["model_access"] = m.relatedRowsByField("ir.model.access", "group_id", groupID)
	}
	if m.fieldExists("res.groups", "rule_groups") {
		row["rule_groups"] = m.relatedRowsByMany2Many("ir.rule", groupID, "groups", "group_ids")
	}
	if m.fieldExists("res.groups", "menu_access") {
		row["menu_access"] = m.relatedRowsByMany2Many("ir.ui.menu", groupID, "groups_id")
	}
	if m.fieldExists("res.groups", "view_access") {
		row["view_access"] = m.relatedRowsByMany2Many("ir.ui.view", groupID, "groups_id")
	}
}

func (m ModelSet) resGroupFullName(row map[string]any) string {
	name := strings.TrimSpace(stringValue(row["name"]))
	privilege := m.rowByID("res.groups.privilege", numericID(row["privilege_id"]))
	privilegeName := strings.TrimSpace(stringValue(safeRowValue(privilege, "name")))
	if privilegeName == "" {
		return name
	}
	if name == "" {
		return privilegeName
	}
	return privilegeName + " / " + name
}

func (m ModelSet) resGroupClosure(groupID int64, reverse bool) []int64 {
	seen := map[int64]bool{}
	var visit func(int64)
	visit = func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		for _, next := range m.resGroupAdjacentIDs(id, reverse) {
			visit(next)
		}
	}
	visit(groupID)
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == groupID {
			return true
		}
		if out[j] == groupID {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

func (m ModelSet) resGroupAdjacentIDs(groupID int64, reverse bool) []int64 {
	if !reverse {
		return int64Values(safeRowValue(m.rowByID("res.groups", groupID), "implied_ids"))
	}
	out := []int64{}
	for _, row := range m.rows("res.groups") {
		if idInSlice(groupID, int64Values(row["implied_ids"])) {
			out = append(out, numericID(row["id"]))
		}
	}
	return out
}

func (m ModelSet) resGroupDisjointIDs(groupID int64) []int64 {
	userTypeIDs := m.resGroupUserTypeIDs()
	if !idInSlice(groupID, userTypeIDs) {
		return nil
	}
	out := make([]int64, 0, len(userTypeIDs)-1)
	for _, id := range userTypeIDs {
		if id != groupID {
			out = append(out, id)
		}
	}
	return out
}

func (m ModelSet) resGroupUserTypeIDs() []int64 {
	ids := []int64{}
	for _, xmlID := range []string{"base.group_user", "base.group_portal", "base.group_public"} {
		if id := m.externalIDResID(xmlID, "res.groups"); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m ModelSet) externalIDResID(xmlID string, modelName string) int64 {
	moduleName, name, ok := strings.Cut(xmlID, ".")
	if !ok {
		return 0
	}
	store, ok := m.env.stores["ir.model.data"]
	if !ok {
		return 0
	}
	for _, row := range store.records {
		if stringValue(row["module"]) == moduleName && stringValue(row["name"]) == name && (modelName == "" || stringValue(row["model"]) == modelName) {
			return numericID(row["res_id"])
		}
	}
	return 0
}

func (m ModelSet) resGroupAllUserIDs(groupID int64) []int64 {
	allImplying := m.resGroupClosure(groupID, true)
	seen := map[int64]bool{}
	for _, implyingID := range allImplying {
		row := m.rowByID("res.groups", implyingID)
		for _, userID := range int64Values(safeRowValue(row, "user_ids")) {
			seen[userID] = true
		}
		for _, user := range m.rows("res.users") {
			if idInSlice(implyingID, append(int64Values(user["groups_id"]), int64Values(user["group_ids"])...)) {
				seen[numericID(user["id"])] = true
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		if id != 0 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (m ModelSet) relatedRowsByField(modelName string, fieldName string, value int64) []int64 {
	out := []int64{}
	for _, row := range m.rows(modelName) {
		if numericID(row[fieldName]) == value {
			out = append(out, numericID(row["id"]))
		}
	}
	return out
}

func (m ModelSet) relatedRowsByMany2Many(modelName string, value int64, fieldNames ...string) []int64 {
	out := []int64{}
	for _, row := range m.rows(modelName) {
		for _, fieldName := range fieldNames {
			if idInSlice(value, int64Values(row[fieldName])) {
				out = append(out, numericID(row["id"]))
				break
			}
		}
	}
	return out
}

func (m ModelSet) syncAllMailingDerivedFields() {
	if m.fieldExists("utm.campaign", "mailing_mail_ids") ||
		m.fieldExists("utm.campaign", "mailing_mail_count") ||
		m.fieldExists("utm.campaign", "ab_testing_mailings_count") ||
		m.fieldExists("utm.campaign", "ab_testing_completed") {
		for _, campaign := range m.rows("utm.campaign") {
			campaignID := numericID(campaign["id"])
			mailingIDs := []int64{}
			abTestingCount := int64(0)
			for _, mailing := range m.rows("mailing.mailing") {
				if numericID(mailing["campaign_id"]) != campaignID {
					continue
				}
				mailingID := numericID(mailing["id"])
				mailingIDs = append(mailingIDs, mailingID)
				if truthyRecordValue(mailing["ab_testing_enabled"]) {
					abTestingCount++
				}
			}
			if m.fieldExists("utm.campaign", "mailing_mail_ids") {
				campaign["mailing_mail_ids"] = mailingIDs
			}
			if m.fieldExists("utm.campaign", "mailing_mail_count") {
				campaign["mailing_mail_count"] = int64(len(mailingIDs))
			}
			if m.fieldExists("utm.campaign", "ab_testing_mailings_count") {
				campaign["ab_testing_mailings_count"] = abTestingCount
			}
			if m.fieldExists("utm.campaign", "ab_testing_completed") {
				campaign["ab_testing_completed"] = numericID(campaign["ab_testing_winner_mailing_id"]) != 0
			}
		}
	}
	if m.fieldExists("mailing.mailing", "ab_testing_is_winner_mailing") ||
		m.fieldExists("mailing.mailing", "ab_testing_mailings_count") ||
		m.fieldExists("mailing.mailing", "ab_testing_completed") ||
		m.fieldExists("mailing.mailing", "is_ab_test_sent") {
		for _, mailing := range m.rows("mailing.mailing") {
			mailingID := numericID(mailing["id"])
			campaignID := numericID(mailing["campaign_id"])
			campaign := m.rowByID("utm.campaign", campaignID)
			winnerID := numericID(safeRowValue(campaign, "ab_testing_winner_mailing_id"))
			abTestingCount := int64(0)
			for _, sibling := range m.rows("mailing.mailing") {
				if numericID(sibling["campaign_id"]) == campaignID && truthyRecordValue(sibling["ab_testing_enabled"]) {
					abTestingCount++
				}
			}
			if m.fieldExists("mailing.mailing", "ab_testing_is_winner_mailing") {
				mailing["ab_testing_is_winner_mailing"] = winnerID != 0 && winnerID == mailingID
			}
			if m.fieldExists("mailing.mailing", "ab_testing_completed") {
				mailing["ab_testing_completed"] = winnerID != 0
			}
			if m.fieldExists("mailing.mailing", "ab_testing_mailings_count") {
				mailing["ab_testing_mailings_count"] = abTestingCount
			}
			if m.fieldExists("mailing.mailing", "is_ab_test_sent") {
				mailing["is_ab_test_sent"] = len(m.relatedRowsByField("mailing.trace", "mass_mailing_id", mailingID)) > 0
			}
		}
	}
	if m.fieldExists("mailing.contact", "subscription_ids") || m.fieldExists("mailing.contact", "list_ids") {
		for _, row := range m.rows("mailing.contact") {
			contactID := numericID(row["id"])
			subscriptionIDs := []int64{}
			listIDs := []int64{}
			for _, subscription := range m.rows("mailing.subscription") {
				if numericID(subscription["contact_id"]) != contactID {
					continue
				}
				subscriptionIDs = append(subscriptionIDs, numericID(subscription["id"]))
				listID := numericID(subscription["list_id"])
				if listID != 0 && !idInSlice(listID, listIDs) {
					listIDs = append(listIDs, listID)
				}
			}
			if m.fieldExists("mailing.contact", "subscription_ids") {
				row["subscription_ids"] = subscriptionIDs
			}
			if m.fieldExists("mailing.contact", "list_ids") {
				row["list_ids"] = listIDs
			}
		}
	}
	if m.fieldExists("mailing.list", "subscription_ids") || m.fieldExists("mailing.list", "contact_ids") {
		for _, row := range m.rows("mailing.list") {
			listID := numericID(row["id"])
			subscriptionIDs := []int64{}
			contactIDs := []int64{}
			for _, subscription := range m.rows("mailing.subscription") {
				if numericID(subscription["list_id"]) != listID {
					continue
				}
				subscriptionIDs = append(subscriptionIDs, numericID(subscription["id"]))
				contactID := numericID(subscription["contact_id"])
				if contactID != 0 && !idInSlice(contactID, contactIDs) {
					contactIDs = append(contactIDs, contactID)
				}
			}
			if m.fieldExists("mailing.list", "subscription_ids") {
				row["subscription_ids"] = subscriptionIDs
			}
			if m.fieldExists("mailing.list", "contact_ids") {
				row["contact_ids"] = contactIDs
			}
		}
	}
}

func (m ModelSet) syncAllWhatsAppMarketingDerivedFields() {
	if !m.fieldExists("marketing.trace", "whatsapp_message_id") ||
		!m.fieldExists("marketing.trace", "links_click_datetime") ||
		!m.fieldExists("whatsapp.message", "links_click_datetime") {
		return
	}
	for _, trace := range m.rows("marketing.trace") {
		whatsAppID := numericID(trace["whatsapp_message_id"])
		if whatsAppID == 0 {
			continue
		}
		message := m.rowByID("whatsapp.message", whatsAppID)
		if message == nil {
			trace["links_click_datetime"] = nil
			continue
		}
		trace["links_click_datetime"] = message["links_click_datetime"]
	}
}

func (m ModelSet) resGroupsViewGroupHierarchy() map[string]any {
	groups := map[int64]any{}
	for _, row := range m.rows("res.groups") {
		id := numericID(row["id"])
		groups[id] = map[string]any{
			"id":                 id,
			"name":               row["name"],
			"comment":            row["comment"],
			"privilege_id":       numericID(row["privilege_id"]),
			"disjoint_ids":       int64Values(row["disjoint_ids"]),
			"implied_ids":        int64Values(row["implied_ids"]),
			"all_implied_ids":    int64Values(row["all_implied_ids"]),
			"all_implied_by_ids": int64Values(row["all_implied_by_ids"]),
		}
	}
	privileges := map[int64]any{}
	for _, row := range m.rows("res.groups.privilege") {
		id := numericID(row["id"])
		groupIDs := m.relatedRowsByField("res.groups", "privilege_id", id)
		sort.Slice(groupIDs, func(i, j int) bool {
			left := m.rowByID("res.groups", groupIDs[i])
			right := m.rowByID("res.groups", groupIDs[j])
			leftDepth := m.resGroupPrivilegeImpliedDepth(left, groupIDs)
			rightDepth := m.resGroupPrivilegeImpliedDepth(right, groupIDs)
			if leftDepth != rightDepth {
				return leftDepth < rightDepth
			}
			if numericID(left["sequence"]) != numericID(right["sequence"]) {
				return numericID(left["sequence"]) < numericID(right["sequence"])
			}
			return groupIDs[i] < groupIDs[j]
		})
		privileges[id] = map[string]any{
			"id":          id,
			"name":        row["name"],
			"category_id": numericID(row["category_id"]),
			"description": row["description"],
			"placeholder": row["placeholder"],
			"group_ids":   groupIDs,
		}
	}
	categories := []any{}
	for _, row := range m.rows("ir.module.category") {
		privilegeIDs := m.relatedRowsByField("res.groups.privilege", "category_id", numericID(row["id"]))
		sort.Slice(privilegeIDs, func(i, j int) bool {
			left := m.rowByID("res.groups.privilege", privilegeIDs[i])
			right := m.rowByID("res.groups.privilege", privilegeIDs[j])
			if numericID(left["sequence"]) != numericID(right["sequence"]) {
				return numericID(left["sequence"]) < numericID(right["sequence"])
			}
			return privilegeIDs[i] < privilegeIDs[j]
		})
		filtered := []int64{}
		for _, privilegeID := range privilegeIDs {
			if len(m.relatedRowsByField("res.groups", "privilege_id", privilegeID)) > 0 {
				filtered = append(filtered, privilegeID)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		categories = append(categories, map[string]any{
			"id":            numericID(row["id"]),
			"name":          row["name"],
			"privilege_ids": filtered,
		})
	}
	return map[string]any{"groups": groups, "privileges": privileges, "categories": categories}
}

func (m ModelSet) resGroupPrivilegeImpliedDepth(row map[string]any, privilegeGroupIDs []int64) int {
	if row == nil || numericID(row["privilege_id"]) == 0 {
		return 0
	}
	depth := 0
	for _, impliedID := range int64Values(row["all_implied_ids"]) {
		if idInSlice(impliedID, privilegeGroupIDs) {
			depth++
		}
	}
	return depth
}

func (m ModelSet) applyResPartnerDerivedFields(partnerID int64, row map[string]any) {
	if partnerID == 0 || row == nil {
		return
	}
	m.applyResPartnerCommercialField(partnerID, row)
	if m.fieldExists("res.partner", "partner_share") {
		row["partner_share"] = m.computeResPartnerShare(partnerID)
	}
}

func (m ModelSet) applyResPartnerCommercialField(partnerID int64, row map[string]any) {
	if partnerID == 0 || row == nil || !m.fieldExists("res.partner", "commercial_partner_id") {
		return
	}
	commercialID := partnerID
	if !truthyRecordValue(row["is_company"]) {
		parentID := numericID(row["parent_id"])
		if parentID != 0 {
			parent := m.rowByID("res.partner", parentID)
			parentCommercialID := numericID(safeRowValue(parent, "commercial_partner_id"))
			if parentCommercialID == 0 {
				parentCommercialID = parentID
			}
			commercialID = parentCommercialID
		}
	}
	row["commercial_partner_id"] = commercialID
}

func (m ModelSet) computeResPartnerShare(partnerID int64) bool {
	userStore, ok := m.env.stores["res.users"]
	if !ok {
		return true
	}
	for userID, user := range userStore.records {
		if numericID(user["partner_id"]) != partnerID {
			continue
		}
		if userID == 1 {
			return false
		}
		if !m.rowIsActive("res.users", user) {
			continue
		}
		if !m.computeResUsersShare(userID, user) {
			return false
		}
	}
	return true
}

func (m ModelSet) applyResUsersDerivedFields(userID int64, row map[string]any) {
	if userID == 0 || row == nil {
		return
	}
	allGroupIDs := m.resUserAllGroupIDs(row)
	if m.fieldExists("res.users", "all_group_ids") {
		row["all_group_ids"] = allGroupIDs
	}
	if m.fieldExists("res.users", "share") {
		row["share"] = m.computeResUsersShare(userID, row)
	}
	if m.fieldExists("res.users", "accesses_count") || m.fieldExists("res.users", "rules_count") {
		accessesCount, rulesCount := m.resUserAccessRuleCounts(allGroupIDs)
		if m.fieldExists("res.users", "accesses_count") {
			row["accesses_count"] = accessesCount
		}
		if m.fieldExists("res.users", "rules_count") {
			row["rules_count"] = rulesCount
		}
	}
	if m.fieldExists("res.users", "groups_count") {
		row["groups_count"] = int64(len(allGroupIDs))
	}
	if m.fieldExists("res.users", "view_group_hierarchy") {
		row["view_group_hierarchy"] = m.resGroupsViewGroupHierarchy()
	}
	if m.fieldExists("res.users", "role") {
		row["role"] = m.resUserRole(allGroupIDs)
	}
	if !m.fieldExists("res.users", "commercial_partner_id") {
		return
	}
	partnerID := numericID(row["partner_id"])
	commercialID := partnerID
	if partnerID != 0 {
		partner := m.rowByID("res.partner", partnerID)
		if partner != nil {
			if m.fieldExists("res.users", "active_partner") {
				row["active_partner"] = m.rowIsActive("res.partner", partner)
			}
			m.applyResPartnerCommercialField(partnerID, partner)
			if id := numericID(partner["commercial_partner_id"]); id != 0 {
				commercialID = id
			}
		}
	}
	row["commercial_partner_id"] = commercialID
}

func (m ModelSet) computeResUsersShare(_ int64, row map[string]any) bool {
	groupUserID := m.baseGroupUserID()
	if groupUserID == 0 {
		return true
	}
	return !idInSlice(groupUserID, m.resUserAllGroupIDs(row))
}

func (m ModelSet) resUserAllGroupIDs(row map[string]any) []int64 {
	groupIDs := append(int64Values(row["group_ids"]), int64Values(row["groups_id"])...)
	effective := m.effectiveResGroupIDs(groupIDs)
	out := make([]int64, 0, len(effective))
	for id := range effective {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (m ModelSet) resUserAccessRuleCounts(groupIDs []int64) (int64, int64) {
	groupSet := map[int64]bool{}
	for _, id := range groupIDs {
		groupSet[id] = true
	}
	accessIDs := map[int64]bool{}
	for _, row := range m.rows("ir.model.access") {
		if groupSet[numericID(row["group_id"])] {
			accessIDs[numericID(row["id"])] = true
		}
	}
	ruleIDs := map[int64]bool{}
	for _, row := range m.rows("ir.rule") {
		for _, groupID := range append(int64Values(row["groups"]), int64Values(row["group_ids"])...) {
			if groupSet[groupID] {
				ruleIDs[numericID(row["id"])] = true
				break
			}
		}
	}
	return int64(len(accessIDs)), int64(len(ruleIDs))
}

func (m ModelSet) resUserRole(allGroupIDs []int64) any {
	if systemID := m.baseGroupSystemID(); systemID != 0 && idInSlice(systemID, allGroupIDs) {
		return "group_system"
	}
	if userID := m.baseGroupUserID(); userID != 0 && idInSlice(userID, allGroupIDs) {
		return "group_user"
	}
	return false
}

func (m ModelSet) baseGroupUserID() int64 {
	if dataStore, ok := m.env.stores["ir.model.data"]; ok {
		for _, row := range dataStore.records {
			if stringValue(row["module"]) == "base" && stringValue(row["name"]) == "group_user" && stringValue(row["model"]) == "res.groups" {
				if id := numericID(row["res_id"]); id != 0 {
					return id
				}
			}
		}
	}
	groupStore, ok := m.env.stores["res.groups"]
	if !ok {
		return 0
	}
	for id, row := range groupStore.records {
		switch strings.ToLower(strings.TrimSpace(stringValue(row["name"]))) {
		case "role / user", "internal user", "internal":
			return id
		}
	}
	return 0
}

func (m ModelSet) baseGroupSystemID() int64 {
	if id := m.externalIDResID("base.group_system", "res.groups"); id != 0 {
		return id
	}
	groupStore, ok := m.env.stores["res.groups"]
	if !ok {
		return 0
	}
	for id, row := range groupStore.records {
		switch strings.ToLower(strings.TrimSpace(stringValue(row["name"]))) {
		case "role / administrator", "administrator", "settings":
			return id
		}
	}
	return 0
}

func (m ModelSet) effectiveResGroupIDs(groupIDs []int64) map[int64]bool {
	effective := map[int64]bool{}
	groupStore, ok := m.env.stores["res.groups"]
	if !ok {
		return effective
	}
	var visit func(int64)
	visit = func(groupID int64) {
		if groupID == 0 || effective[groupID] {
			return
		}
		effective[groupID] = true
		row := groupStore.records[groupID]
		for _, impliedID := range int64Values(safeRowValue(row, "implied_ids")) {
			visit(impliedID)
		}
	}
	for _, groupID := range groupIDs {
		visit(groupID)
	}
	return effective
}

func (m ModelSet) syncResUserPartnerShare(oldRow map[string]any, newRow map[string]any) {
	partnerIDs := []int64{numericID(safeRowValue(oldRow, "partner_id")), numericID(safeRowValue(newRow, "partner_id"))}
	for _, partnerID := range uniqueRecordIDs(partnerIDs) {
		partner := m.rowByID("res.partner", partnerID)
		if partner != nil {
			m.applyResPartnerDerivedFields(partnerID, partner)
		}
	}
}

func (m ModelSet) syncResUserPartnerActive(user map[string]any, activateOnly bool) {
	partnerID := numericID(safeRowValue(user, "partner_id"))
	if partnerID == 0 || !m.fieldExists("res.partner", "active") || !m.fieldExists("res.users", "active") {
		return
	}
	active, ok := explicitRowBool(user, "active")
	if !ok {
		return
	}
	if activateOnly && !active {
		return
	}
	partner := m.rowByID("res.partner", partnerID)
	if partner != nil {
		partner["active"] = active
	}
}

func (m ModelSet) syncResPartnerDependents(partnerID int64) {
	if partnerID == 0 {
		return
	}
	seen := map[int64]bool{}
	queue := []int64{partnerID}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if currentID == 0 || seen[currentID] {
			continue
		}
		seen[currentID] = true
		partner := m.rowByID("res.partner", currentID)
		if partner == nil {
			continue
		}
		m.applyResPartnerDerivedFields(currentID, partner)
		if userStore, ok := m.env.stores["res.users"]; ok {
			for userID, user := range userStore.records {
				if numericID(user["partner_id"]) == currentID {
					m.applyResUsersDerivedFields(userID, user)
				}
			}
		}
		if partnerStore, ok := m.env.stores["res.partner"]; ok {
			for childID, child := range partnerStore.records {
				if numericID(child["parent_id"]) == currentID {
					queue = append(queue, childID)
				}
			}
		}
	}
}

func (m ModelSet) syncAllResUsersDerivedFields() {
	userStore, ok := m.env.stores["res.users"]
	if !ok {
		return
	}
	for userID, user := range userStore.records {
		oldRow := copyValues(user)
		m.applyResUsersDerivedFields(userID, user)
		m.syncResUserPartnerShare(oldRow, user)
	}
}

func (m ModelSet) fieldExists(modelName string, fieldName string) bool {
	meta, ok := m.env.registry.Model(modelName)
	if !ok {
		return false
	}
	_, ok = meta.Fields[fieldName]
	return ok
}

func safeRowValue(row map[string]any, key string) any {
	if row == nil {
		return nil
	}
	return row[key]
}

func explicitRowBool(row map[string]any, key string) (bool, bool) {
	if row == nil {
		return false, false
	}
	value, ok := row[key]
	if !ok || value == nil {
		return false, false
	}
	return truthyRecordValue(value), true
}

func (m ModelSet) rowIsActive(modelName string, row map[string]any) bool {
	if !m.fieldExists(modelName, "active") {
		return true
	}
	active, ok := explicitRowBool(row, "active")
	return !ok || active
}

func partnerWriteArchives(values map[string]any) bool {
	active, ok := explicitRowBool(values, "active")
	return ok && !active
}

func (m ModelSet) hasActiveLinkedUser(partnerID int64) bool {
	userStore, ok := m.env.stores["res.users"]
	if !ok {
		return false
	}
	for _, user := range userStore.records {
		if numericID(user["partner_id"]) == partnerID && m.rowIsActive("res.users", user) {
			return true
		}
	}
	return false
}

func removeRecordID(ids []int64, id int64) []int64 {
	if len(ids) == 0 || id == 0 {
		return ids
	}
	out := ids[:0]
	for _, item := range ids {
		if item != id {
			out = append(out, item)
		}
	}
	return out
}

func (m ModelSet) normalizeMailActivityValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	merged := copyValues(existing)
	for key, value := range out {
		merged[key] = value
	}
	activityTypeID := numericID(merged["activity_type_id"])
	if activityTypeID == 0 {
		if recommendedID := numericID(merged["recommended_activity_type_id"]); recommendedID != 0 {
			activityTypeID = recommendedID
			out["activity_type_id"] = recommendedID
			merged["activity_type_id"] = recommendedID
		}
	}
	if activityTypeID == 0 {
		previousType := m.rowByID("mail.activity.type", numericID(merged["previous_activity_type_id"]))
		if triggeredID := numericID(previousType["triggered_next_type_id"]); triggeredID != 0 {
			activityTypeID = triggeredID
			out["activity_type_id"] = triggeredID
			merged["activity_type_id"] = triggeredID
		}
	}
	if previousTypeID := numericID(merged["previous_activity_type_id"]); previousTypeID != 0 {
		previousType := m.rowByID("mail.activity.type", previousTypeID)
		out["has_recommended_activities"] = len(int64Values(previousType["suggested_next_type_ids"])) > 0
	}
	if activityTypeID == 0 {
		return out
	}
	activityType := m.rowByID("mail.activity.type", activityTypeID)
	if activityType == nil {
		return out
	}
	out["activity_category"] = stringValue(firstNonEmpty(activityType["category"], "default"))
	out["chaining_type"] = stringValue(firstNonEmpty(activityType["chaining_type"], "suggest"))
	if _, ok := m.model.Fields["hide_in_chatter"]; ok {
		out["hide_in_chatter"] = m.isApprovalActivityHiddenInChatter(activityTypeID, merged["automated"])
	}
	return out
}

func (m ModelSet) isApprovalActivityHiddenInChatter(activityTypeID int64, automated any) bool {
	if activityTypeID == 0 || !truthyRecordValue(automated) {
		return false
	}
	for _, row := range m.rows("ir.model.data") {
		if stringValue(row["module"]) == "oi_workflow" &&
			stringValue(row["name"]) == "activity_type_approval" &&
			stringValue(row["model"]) == "mail.activity.type" &&
			numericID(row["res_id"]) == activityTypeID {
			return true
		}
	}
	return false
}

func (m ModelSet) normalizeMailActivityTypeValues(existing map[string]any, values map[string]any) map[string]any {
	out := copyValues(values)
	if existing == nil {
		if _, ok := out["category"]; !ok {
			out["category"] = "default"
		}
		if _, ok := out["chaining_type"]; !ok {
			out["chaining_type"] = "suggest"
		}
	}
	if triggeredID := numericID(out["triggered_next_type_id"]); triggeredID != 0 {
		out["chaining_type"] = "trigger"
		out["suggested_next_type_ids"] = []int64{}
		return out
	}
	if suggestedIDs, ok := out["suggested_next_type_ids"]; ok && len(int64Values(suggestedIDs)) > 0 {
		out["chaining_type"] = "suggest"
		out["triggered_next_type_id"] = int64(0)
		return out
	}
	if chainingType, ok := out["chaining_type"]; ok {
		switch strings.TrimSpace(stringValue(chainingType)) {
		case "trigger":
			out["suggested_next_type_ids"] = []int64{}
		case "suggest":
			out["triggered_next_type_id"] = int64(0)
		}
		return out
	}
	if value, ok := out["triggered_next_type_id"]; ok && isFalsey(value) {
		out["chaining_type"] = "suggest"
	}
	return out
}

func (m ModelSet) syncMailActivityTypePreviousIDs() {
	store, ok := m.env.stores["mail.activity.type"]
	if !ok {
		return
	}
	previousByRecommended := map[int64][]int64{}
	for id, row := range store.records {
		for _, recommendedID := range int64Values(row["suggested_next_type_ids"]) {
			previousByRecommended[recommendedID] = appendUniqueID(previousByRecommended[recommendedID], id)
		}
	}
	for id, row := range store.records {
		row["previous_type_ids"] = previousByRecommended[id]
		if row["previous_type_ids"] == nil {
			row["previous_type_ids"] = []int64{}
		}
	}
}

func (m ModelSet) validateAccountingWrite(existing map[string]any, values map[string]any) error {
	switch m.model.Name {
	case "account.move":
		return m.validateAccountMoveWrite(existing, values)
	case "account.move.line":
		return m.validateAccountMoveLineWrite(existing, values)
	default:
		return nil
	}
}

func (m ModelSet) validateAccountMoveWrite(existing map[string]any, values map[string]any) error {
	before := m.accountMoveFromRow(existing, nil)
	if stringValue(values["state"]) == string(coreaccounting.MovePosted) && before.State != coreaccounting.MovePosted && !m.env.accountMovePost {
		return coreaccounting.ErrMovePostRequiresAction
	}
	if before.State == coreaccounting.MovePosted {
		immutableValues := values
		if m.accountMoveInitialLineLink(existing, values) {
			immutableValues = copyValues(values)
			delete(immutableValues, "line_ids")
		}
		if err := coreaccounting.UpdatePostedFields(before, immutableValues, nil); err != nil {
			return err
		}
	}
	if !accountMoveWriteChecksLockDates(before, values) {
		return nil
	}
	merged := copyValues(existing)
	for key, value := range values {
		merged[key] = value
	}
	after := m.accountMoveFromRow(merged, nil)
	locks := m.EffectiveAccountLockPolicy(after.CompanyID)
	if before.State == coreaccounting.MovePosted && after.State == coreaccounting.MoveDraft {
		move := before
		return coreaccounting.ButtonDraft(&move, locks)
	}
	if before.State != coreaccounting.MoveCancel && after.State == coreaccounting.MoveCancel {
		move := before
		return coreaccounting.ButtonCancel(&move, locks)
	}
	return coreaccounting.ValidateLockDates(after, locks)
}

func (m ModelSet) validateAccountMoveCreate(row map[string]any) error {
	if stringValue(row["state"]) == string(coreaccounting.MovePosted) && !m.env.accountMovePost {
		return coreaccounting.ErrMovePostRequiresAction
	}
	return nil
}

func (m ModelSet) accountMoveInitialLineLink(existing map[string]any, values map[string]any) bool {
	rawLineIDs, ok := values["line_ids"]
	if !ok || len(int64Values(existing["line_ids"])) > 0 {
		return false
	}
	moveID := numericID(existing["id"])
	for _, lineID := range int64Values(rawLineIDs) {
		row := m.rowByID("account.move.line", lineID)
		if row == nil || numericID(row["move_id"]) != moveID {
			return false
		}
	}
	return true
}

func accountMoveWriteChecksLockDates(before coreaccounting.Move, values map[string]any) bool {
	if before.State != coreaccounting.MovePosted && before.State != coreaccounting.MoveCancel {
		if state := coreaccounting.MoveState(stringValue(values["state"])); state != coreaccounting.MovePosted && state != coreaccounting.MoveCancel {
			return false
		}
	}
	for _, fieldName := range []string{"date", "invoice_date", "journal_id", "company_id", "move_type", "line_ids", "state"} {
		if _, ok := values[fieldName]; ok {
			return true
		}
	}
	return false
}

func accountMoveStateDetachesInvoicePDF(oldRow map[string]any, newRow map[string]any) bool {
	before := coreaccounting.MoveState(stringValue(oldRow["state"]))
	after := coreaccounting.MoveState(stringValue(newRow["state"]))
	return (after == coreaccounting.MoveDraft && (before == coreaccounting.MovePosted || before == coreaccounting.MoveCancel)) ||
		(before == coreaccounting.MovePosted && after == coreaccounting.MoveCancel)
}

func (m ModelSet) detachAccountMoveInvoicePDFs(moveIDs []int64) {
	if len(moveIDs) == 0 {
		return
	}
	moveSet := map[int64]bool{}
	for _, id := range moveIDs {
		if id != 0 {
			moveSet[id] = true
		}
	}
	if len(moveSet) == 0 {
		return
	}
	attachmentStore := m.env.stores["ir.attachment"]
	if attachmentStore != nil {
		userName := m.currentUserDisplayName()
		dateText := time.Now().UTC().Format("2006-01-02")
		for attachmentID, row := range attachmentStore.records {
			if !accountMoveInvoicePDFAttachment(row, moveSet) {
				continue
			}
			row["res_field"] = nil
			row["name"] = detachedAttachmentName(stringValue(row["name"]), attachmentID, userName, dateText)
		}
	}
	moveStore := m.env.stores["account.move"]
	if moveStore == nil {
		return
	}
	for id := range moveSet {
		if row := moveStore.records[id]; row != nil {
			row["invoice_pdf_report_id"] = nil
			row["invoice_pdf_report_file"] = nil
		}
	}
}

func (m ModelSet) unlinkAccountMoveInvoicePDFs(moveIDs []int64) {
	if len(moveIDs) == 0 {
		return
	}
	attachmentStore := m.env.stores["ir.attachment"]
	if attachmentStore == nil {
		return
	}
	moveSet := map[int64]bool{}
	for _, id := range moveIDs {
		if id != 0 {
			moveSet[id] = true
		}
	}
	for attachmentID, row := range attachmentStore.records {
		if accountMoveInvoicePDFAttachment(row, moveSet) {
			delete(attachmentStore.records, attachmentID)
		}
	}
}

func accountMoveInvoicePDFAttachment(row map[string]any, moveSet map[int64]bool) bool {
	return stringValue(row["res_model"]) == "account.move" &&
		moveSet[numericID(row["res_id"])] &&
		stringValue(row["res_field"]) == "invoice_pdf_report_file"
}

func (m ModelSet) currentUserDisplayName() string {
	userID := m.env.context.UserID
	if userID == 0 {
		return "user"
	}
	if userStore := m.env.stores["res.users"]; userStore != nil {
		if row := userStore.records[userID]; row != nil {
			if name := strings.TrimSpace(stringValue(row["name"])); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("user %d", userID)
}

func detachedAttachmentName(name string, attachmentID int64, userName string, dateText string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("attachment-%d", attachmentID)
	}
	stem := name
	extension := ""
	if dot := strings.LastIndex(name, "."); dot > 0 {
		stem = name[:dot]
		extension = name[dot:]
	}
	return fmt.Sprintf("%s (detached by %s on %s)%s", stem, userName, dateText, extension)
}

func (m ModelSet) validateAccountMoveLineWrite(existing map[string]any, values map[string]any) error {
	moveID := numericID(firstNonZero(values["move_id"], existing["move_id"]))
	if moveID == 0 {
		return nil
	}
	moveRow := m.rowByID("account.move", moveID)
	if moveRow == nil {
		return nil
	}
	current := m.accountMoveFromRow(moveRow, nil)
	if current.State != coreaccounting.MovePosted {
		return nil
	}
	if !accountMoveLineAnyFieldWillChange(existing, values) {
		return nil
	}
	for fieldName := range accountMoveLineTaxFields() {
		if accountMoveLineFieldWillChange(existing, values, fieldName) {
			return fmt.Errorf("%w: %s", coreaccounting.ErrPostedLineTaxImmutable, fieldName)
		}
	}
	lineID := numericID(existing["id"])
	mergedLine := copyValues(existing)
	for key, value := range values {
		mergedLine[key] = value
	}
	move := m.accountMoveFromRow(moveRow, map[int64]map[string]any{lineID: mergedLine})
	locks := m.EffectiveAccountLockPolicy(move.CompanyID)
	if accountMoveLineAnyProtectedFieldWillChange(existing, values, accountMoveLineFiscalLockFields()) {
		fiscalLocks := locks
		fiscalLocks.TaxLockDate = time.Time{}
		if err := coreaccounting.ValidateLockDates(move, fiscalLocks); err != nil {
			return err
		}
	}
	if accountMoveLineAnyProtectedFieldWillChange(existing, values, accountMoveLineTaxLockFields()) && accountMoveLineAffectsTaxReport(mergedLine) {
		taxLocks := locks
		taxLocks.FiscalLockDate = time.Time{}
		taxLocks.SaleLockDate = time.Time{}
		taxLocks.PurchaseLockDate = time.Time{}
		taxMove := move
		taxMove.Lines = accountMoveLineByID(move.Lines, lineID)
		if err := coreaccounting.ValidateLockDates(taxMove, taxLocks); err != nil {
			return err
		}
	}
	return nil
}

func accountMoveLineTaxFields() map[string]bool {
	return map[string]bool{"tax_ids": true, "tax_line_id": true}
}

func accountMoveLineTaxLockFields() map[string]bool {
	return map[string]bool{"balance": true, "tax_ids": true, "tax_line_id": true, "tax_tag_ids": true}
}

func accountMoveLineFiscalLockFields() map[string]bool {
	fields := accountMoveLineTaxLockFields()
	for _, fieldName := range []string{"account_id", "journal_id", "amount_currency", "currency_id", "partner_id"} {
		fields[fieldName] = true
	}
	return fields
}

func accountMoveLineAnyFieldWillChange(existing map[string]any, values map[string]any) bool {
	for fieldName := range values {
		if accountMoveLineFieldWillChange(existing, values, fieldName) {
			return true
		}
	}
	return false
}

func accountMoveLineAnyProtectedFieldWillChange(existing map[string]any, values map[string]any, protected map[string]bool) bool {
	for fieldName := range protected {
		if accountMoveLineFieldWillChange(existing, values, fieldName) {
			return true
		}
	}
	return false
}

func accountMoveLineFieldWillChange(existing map[string]any, values map[string]any, fieldName string) bool {
	value, ok := values[fieldName]
	if !ok {
		return false
	}
	existingValue := existing[fieldName]
	if isInt64ListValue(value) || isInt64ListValue(existingValue) {
		return !equalInt64Lists(int64Values(existingValue), int64Values(value))
	}
	if id := numericID(value); id != 0 || numericID(existingValue) != 0 {
		return numericID(existingValue) != id
	}
	return !reflect.DeepEqual(existingValue, value)
}

func isInt64ListValue(value any) bool {
	switch value.(type) {
	case []int64, []any, []int, []float64:
		return true
	default:
		return false
	}
}

func equalInt64Lists(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func accountMoveLineAffectsTaxReport(row map[string]any) bool {
	return len(int64Values(row["tax_ids"])) > 0 || len(int64Values(row["tax_tag_ids"])) > 0 || numericID(row["tax_line_id"]) != 0 || numericID(row["tax_repartition_line_id"]) != 0
}

func accountMoveLineByID(lines []coreaccounting.MoveLine, id int64) []coreaccounting.MoveLine {
	for _, line := range lines {
		if line.ID == id {
			return []coreaccounting.MoveLine{line}
		}
	}
	return nil
}

func (m ModelSet) trackAccountMoveLineWrite(oldRows map[int64]map[string]any) error {
	if !m.accountMoveLineTrackingEnabled() {
		return nil
	}
	for id, before := range oldRows {
		after := m.rowByID("account.move.line", id)
		if after == nil || !m.accountMoveLineMoveWasPosted(after) {
			continue
		}
		changes := m.accountMoveLineTrackedChanges(before, after)
		if len(changes) == 0 {
			continue
		}
		if err := m.createAccountMoveLineTrackingMessage(after, "updated", changes); err != nil {
			return err
		}
	}
	return nil
}

func (m ModelSet) trackAccountMoveLineCreate(row map[string]any) error {
	if !m.accountMoveLineTrackingEnabled() || !m.accountMoveLineMoveWasPosted(row) {
		return nil
	}
	changes := m.accountMoveLineTrackedChanges(map[string]any{}, row)
	if len(changes) == 0 {
		return nil
	}
	return m.createAccountMoveLineTrackingMessage(row, "created", changes)
}

func (m ModelSet) trackAccountMoveLineUnlink(oldRows map[int64]map[string]any) error {
	if !m.accountMoveLineTrackingEnabled() {
		return nil
	}
	empty := map[string]any{}
	ids := make([]int64, 0, len(oldRows))
	for id := range oldRows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		before := oldRows[id]
		if !m.accountMoveLineMoveWasPosted(before) {
			continue
		}
		changes := m.accountMoveLineTrackedChanges(empty, before)
		if len(changes) == 0 {
			continue
		}
		if err := m.createAccountMoveLineTrackingMessage(before, "deleted", changes); err != nil {
			return err
		}
	}
	return nil
}

func (m ModelSet) accountMoveLineTrackingEnabled() bool {
	if truthyRecordValue(m.env.context.Values["tracking_disable"]) {
		return false
	}
	_, ok := m.env.registry.Model("mail.message")
	return ok
}

func (m ModelSet) applyMailMessageTrackingCommands(messageID int64, payload any) ([]int64, error) {
	ids := []int64{}
	for _, command := range mailMessageTrackingCommands(payload) {
		switch command.kind {
		case 0:
			values := copyValues(command.values)
			values["mail_message_id"] = messageID
			id, err := m.env.Model("mail.tracking.value").Create(values)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		case 1:
			if command.id == 0 {
				continue
			}
			values := copyValues(command.values)
			values["mail_message_id"] = messageID
			if err := m.env.Model("mail.tracking.value").Browse(command.id).Write(values); err != nil {
				return nil, err
			}
			ids = appendUniqueID(ids, command.id)
		case 2:
			if command.id != 0 {
				if err := m.env.Model("mail.tracking.value").Browse(command.id).Unlink(); err != nil {
					return nil, err
				}
			}
		case 3:
			if command.id != 0 {
				if err := m.env.Model("mail.tracking.value").Browse(command.id).Write(map[string]any{"mail_message_id": int64(0)}); err != nil {
					return nil, err
				}
			}
		case 4:
			if command.id == 0 {
				continue
			}
			if err := m.env.Model("mail.tracking.value").Browse(command.id).Write(map[string]any{"mail_message_id": messageID}); err != nil {
				return nil, err
			}
			ids = appendUniqueID(ids, command.id)
		case 5:
			ids = []int64{}
		case 6:
			setIDs := int64Values(command.values["ids"])
			if err := m.env.Model("mail.tracking.value").Browse(setIDs...).Write(map[string]any{"mail_message_id": messageID}); err != nil {
				return nil, err
			}
			ids = uniqueRecordIDs(setIDs)
		}
	}
	return ids, nil
}

type mailMessageTrackingCommand struct {
	kind   int64
	id     int64
	values map[string]any
}

func mailMessageTrackingCommands(payload any) []mailMessageTrackingCommand {
	switch typed := payload.(type) {
	case []map[string]any:
		out := make([]mailMessageTrackingCommand, 0, len(typed))
		for _, values := range typed {
			out = append(out, mailMessageTrackingCommand{kind: 0, values: values})
		}
		return out
	case map[string]any:
		return []mailMessageTrackingCommand{{kind: 0, values: typed}}
	case []int64:
		return []mailMessageTrackingCommand{{kind: 6, values: map[string]any{"ids": typed}}}
	case []any:
		out := make([]mailMessageTrackingCommand, 0, len(typed))
		for _, item := range typed {
			if values, ok := item.(map[string]any); ok {
				out = append(out, mailMessageTrackingCommand{kind: 0, values: values})
				continue
			}
			command, ok := item.([]any)
			if !ok || len(command) == 0 {
				continue
			}
			kind := numericID(command[0])
			parsed := mailMessageTrackingCommand{kind: kind}
			if len(command) > 1 {
				parsed.id = numericID(command[1])
			}
			if len(command) > 2 {
				if values, ok := command[2].(map[string]any); ok {
					parsed.values = values
				} else {
					parsed.values = map[string]any{"ids": command[2]}
				}
			}
			if parsed.values == nil {
				parsed.values = map[string]any{}
			}
			out = append(out, parsed)
		}
		return out
	default:
		if id := numericID(typed); id != 0 {
			return []mailMessageTrackingCommand{{kind: 4, id: id, values: map[string]any{}}}
		}
		return nil
	}
}

func appendUniqueID(ids []int64, id int64) []int64 {
	if id == 0 || idInSlice(id, ids) {
		return ids
	}
	return append(ids, id)
}

func uniqueRecordIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		out = appendUniqueID(out, id)
	}
	return out
}

func (m ModelSet) accountMoveLineMoveWasPosted(row map[string]any) bool {
	moveID := numericID(row["move_id"])
	if moveID == 0 {
		return false
	}
	move := m.rowByID("account.move", moveID)
	return move != nil && truthyRecordValue(move["posted_before"])
}

type accountMoveLineTrackingChange struct {
	fieldName string
	label     string
	fieldType string
	oldValue  any
	newValue  any
	line      string
}

func (m ModelSet) createAccountMoveLineTrackingMessage(row map[string]any, action string, changes []accountMoveLineTrackingChange) error {
	moveID := numericID(row["move_id"])
	if moveID == 0 {
		return nil
	}
	lineID := numericID(row["id"])
	body := fmt.Sprintf("Journal Item #%d %s", lineID, action)
	if len(changes) > 0 {
		lines := make([]string, 0, len(changes))
		for _, change := range changes {
			lines = append(lines, change.line)
		}
		body += "<br/>" + strings.Join(lines, "<br/>")
	}
	messageID, err := m.env.Model("mail.message").Create(map[string]any{
		"body":         body,
		"message_type": "notification",
		"model":        "account.move",
		"res_id":       moveID,
		"date":         time.Now().UTC(),
		"body_is_html": true,
	})
	if err != nil {
		return err
	}
	trackingIDs, err := m.createAccountMoveLineTrackingValues(messageID, changes)
	if err != nil {
		return err
	}
	if len(trackingIDs) > 0 {
		if message := m.rowByID("mail.message", messageID); message != nil {
			message["tracking_value_ids"] = trackingIDs
		}
	}
	return nil
}

func (m ModelSet) createAccountMoveLineTrackingValues(messageID int64, changes []accountMoveLineTrackingChange) ([]int64, error) {
	if _, ok := m.env.registry.Model("mail.tracking.value"); !ok {
		return nil, nil
	}
	ids := make([]int64, 0, len(changes))
	for _, change := range changes {
		values := map[string]any{
			"field_name":      change.fieldName,
			"field_desc":      change.label,
			"field_type":      change.fieldType,
			"mail_message_id": messageID,
		}
		switch change.fieldType {
		case "many2one":
			values["old_value_integer"] = numericID(change.oldValue)
			values["new_value_integer"] = numericID(change.newValue)
			values["old_value_char"] = m.accountMoveLineTrackingDisplayValue(change.fieldName, change.oldValue)
			values["new_value_char"] = m.accountMoveLineTrackingDisplayValue(change.fieldName, change.newValue)
		case "many2many":
			values["old_value_char"] = m.accountMoveLineTrackingDisplayValue(change.fieldName, change.oldValue)
			values["new_value_char"] = m.accountMoveLineTrackingDisplayValue(change.fieldName, change.newValue)
		case "monetary", "float":
			values["old_value_float"] = floatRecordValue(change.oldValue)
			values["new_value_float"] = floatRecordValue(change.newValue)
		case "date", "datetime":
			values["old_value_datetime"] = recordDateValue(change.oldValue)
			values["new_value_datetime"] = recordDateValue(change.newValue)
		case "text":
			values["old_value_text"] = stringValue(change.oldValue)
			values["new_value_text"] = stringValue(change.newValue)
		default:
			values["old_value_char"] = stringValue(change.oldValue)
			values["new_value_char"] = stringValue(change.newValue)
		}
		id, err := m.env.Model("mail.tracking.value").Create(values)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (m ModelSet) accountMoveLineTrackedChanges(before map[string]any, after map[string]any) []accountMoveLineTrackingChange {
	tracked := []struct {
		name      string
		label     string
		fieldType string
	}{
		{name: "account_id", label: "Account", fieldType: "many2one"},
		{name: "name", label: "Label", fieldType: "char"},
		{name: "balance", label: "Balance", fieldType: "monetary"},
		{name: "tax_ids", label: "Taxes", fieldType: "many2many"},
		{name: "tax_tag_ids", label: "Tags", fieldType: "many2many"},
		{name: "date_maturity", label: "Due Date", fieldType: "date"},
	}
	changes := []accountMoveLineTrackingChange{}
	for _, field := range tracked {
		if !accountMoveLineFieldWillChange(before, after, field.name) {
			continue
		}
		oldValue := before[field.name]
		newValue := after[field.name]
		changes = append(changes, accountMoveLineTrackingChange{
			fieldName: field.name,
			label:     field.label,
			fieldType: field.fieldType,
			oldValue:  oldValue,
			newValue:  newValue,
			line:      html.EscapeString(fmt.Sprintf("%s: %s -> %s", field.label, m.accountMoveLineTrackingDisplayValue(field.name, oldValue), m.accountMoveLineTrackingDisplayValue(field.name, newValue))),
		})
	}
	return changes
}

func (m ModelSet) accountMoveLineTrackingDisplayValue(fieldName string, value any) string {
	if ids := int64Values(value); len(ids) > 0 {
		return m.displayNamesForTracking(fieldName, ids)
	}
	if id := numericID(value); id != 0 {
		return m.displayNameForTracking(fieldName, id)
	}
	switch typed := value.(type) {
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format("2006-01-02")
	default:
		return stringValue(value)
	}
}

func (m ModelSet) displayNamesForTracking(fieldName string, ids []int64) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, m.displayNameForTracking(fieldName, id))
	}
	return strings.Join(names, ", ")
}

func (m ModelSet) displayNameForTracking(fieldName string, id int64) string {
	modelName := map[string]string{
		"account_id":  "account.account",
		"tax_ids":     "account.tax",
		"tax_tag_ids": "account.account.tag",
	}[fieldName]
	if modelName == "" {
		return fmt.Sprint(id)
	}
	row := m.rowByID(modelName, id)
	if row == nil {
		return fmt.Sprint(id)
	}
	name := strings.TrimSpace(stringValue(row["display_name"]))
	if name == "" {
		name = strings.TrimSpace(stringValue(row["name"]))
	}
	if code := strings.TrimSpace(stringValue(row["code"])); code != "" && name != "" && !strings.Contains(name, code) {
		name = code + " " + name
	}
	if name == "" {
		return fmt.Sprint(id)
	}
	return name
}

func (m ModelSet) validateAccountMoveUnlink(row map[string]any) error {
	move := m.accountMoveFromRow(row, nil)
	return coreaccounting.CanUnlink(move, m.EffectiveAccountLockPolicy(move.CompanyID))
}

func (m ModelSet) validateAccountMoveLineUnlink(row map[string]any) error {
	moveID := numericID(row["move_id"])
	if moveID == 0 {
		return nil
	}
	moveRow := m.rowByID("account.move", moveID)
	if moveRow == nil {
		return nil
	}
	move := m.accountMoveFromRow(moveRow, nil)
	if move.State != coreaccounting.MovePosted && move.State != coreaccounting.MoveCancel {
		return nil
	}
	return coreaccounting.CanUnlink(move, m.EffectiveAccountLockPolicy(move.CompanyID))
}

func (m ModelSet) accountMoveFromRow(row map[string]any, lineOverrides map[int64]map[string]any) coreaccounting.Move {
	if row == nil {
		return coreaccounting.Move{}
	}
	journalID := numericID(row["journal_id"])
	journal := coreaccounting.Journal{ID: journalID}
	if journalRow := m.rowByID("account.journal", journalID); journalRow != nil {
		journal.Type = coreaccounting.JournalType(stringValue(journalRow["type"]))
		journal.CompanyID = numericID(journalRow["company_id"])
		journal.RestrictModeHashTable = truthyRecordValue(journalRow["restrict_mode_hash_table"])
	}
	moveID := numericID(row["id"])
	return coreaccounting.Move{
		ID:                   moveID,
		Name:                 stringValue(row["name"]),
		Ref:                  stringValue(row["ref"]),
		Date:                 recordDateValue(row["date"]),
		InvoiceDate:          recordDateValue(row["invoice_date"]),
		InvoiceDateDue:       recordDateValue(row["invoice_date_due"]),
		State:                coreaccounting.MoveState(stringValue(row["state"])),
		MoveType:             stringValue(row["move_type"]),
		Journal:              journal,
		CompanyID:            numericID(row["company_id"]),
		CurrencyID:           numericID(row["currency_id"]),
		PartnerID:            numericID(row["partner_id"]),
		FiscalPositionID:     numericID(row["fiscal_position_id"]),
		Lines:                m.accountMoveLines(moveID, row["line_ids"], lineOverrides),
		PostedBefore:         truthyRecordValue(row["posted_before"]),
		InalterableHash:      stringValue(row["inalterable_hash"]),
		SequencePrefix:       stringValue(row["sequence_prefix"]),
		SequenceNumber:       numericID(row["sequence_number"]),
		MadeSequenceGap:      truthyRecordValue(row["made_sequence_gap"]),
		SecureSequenceNumber: numericID(row["secure_sequence_number"]),
		AmountTotal:          numericID(row["amount_total"]),
		AmountResidual:       numericID(row["amount_residual"]),
		AmountResidualSigned: numericID(row["amount_residual_signed"]),
		AutoPost:             stringValue(row["auto_post"]),
		PaymentState:         coreaccounting.PaymentState(stringValue(row["payment_state"])),
		StatusInPayment:      stringValue(row["status_in_payment"]),
		IsMoveSent:           truthyRecordValue(row["is_move_sent"]),
		OriginPaymentID:      numericID(row["origin_payment_id"]),
		StatementLineID:      numericID(row["statement_line_id"]),
		MatchedPaymentIDs:    int64Values(row["matched_payment_ids"]),
		ReconciledPaymentIDs: int64Values(row["reconciled_payment_ids"]),
		PaymentCount:         int(numericID(row["payment_count"])),
		NeedCancelRequest:    truthyRecordValue(row["need_cancel_request"]),
		ReversedEntryID:      numericID(row["reversed_entry_id"]),
	}
}

func (m ModelSet) accountMoveLines(moveID int64, rawLineIDs any, overrides map[int64]map[string]any) []coreaccounting.MoveLine {
	lineRows := m.rows("account.move.line")
	lineIDs := int64Values(rawLineIDs)
	allowed := map[int64]bool{}
	for _, id := range lineIDs {
		allowed[id] = true
	}
	out := make([]coreaccounting.MoveLine, 0, len(lineRows))
	for _, row := range lineRows {
		lineID := numericID(row["id"])
		if len(allowed) > 0 {
			if !allowed[lineID] {
				continue
			}
		} else if numericID(row["move_id"]) != moveID {
			continue
		}
		if override := overrides[lineID]; override != nil {
			row = override
		}
		accountID := numericID(row["account_id"])
		accountKind := coreaccounting.AccountKind(stringValue(firstNonEmpty(row["account_type"], row["account_internal_group"])))
		if accountRow := m.rowByID("account.account", accountID); accountRow != nil && accountKind == "" {
			accountKind = coreaccounting.AccountKind(stringValue(accountRow["account_type"]))
		}
		taxID := numericID(row["tax_line_id"])
		if taxID == 0 {
			taxIDs := int64Values(row["tax_ids"])
			if len(taxIDs) > 0 {
				taxID = taxIDs[0]
			}
		}
		out = append(out, coreaccounting.MoveLine{
			ID:                   lineID,
			Account:              coreaccounting.Account{ID: accountID, Kind: accountKind},
			PartnerID:            numericID(row["partner_id"]),
			CompanyID:            numericID(row["company_id"]),
			CurrencyID:           numericID(row["currency_id"]),
			Name:                 stringValue(row["name"]),
			Debit:                numericID(row["debit"]),
			Credit:               numericID(row["credit"]),
			Quantity:             floatRecordValue(row["quantity"]),
			PriceUnit:            numericID(row["price_unit"]),
			PriceSubtotal:        numericID(row["price_subtotal"]),
			PriceTotal:           numericID(row["price_total"]),
			Discount:             floatRecordValue(row["discount"]),
			DisplayType:          stringValue(row["display_type"]),
			DateMaturity:         recordDateValue(row["date_maturity"]),
			ProductID:            numericID(row["product_id"]),
			ProductUOMID:         numericID(row["product_uom_id"]),
			ProductCategoryID:    numericID(row["product_category_id"]),
			AmountCurrency:       numericID(row["amount_currency"]),
			Residual:             numericID(row["amount_residual"]),
			ResidualCurrency:     numericID(row["amount_residual_currency"]),
			Reconciled:           truthyRecordValue(row["reconciled"]),
			PaymentID:            numericID(row["payment_id"]),
			FullReconcileID:      numericID(row["full_reconcile_id"]),
			MatchedDebitIDs:      int64Values(row["matched_debit_ids"]),
			MatchedCreditIDs:     int64Values(row["matched_credit_ids"]),
			TaxID:                taxID,
			TaxRepartitionLineID: numericID(row["tax_repartition_line_id"]),
		})
	}
	return out
}

func (r RecordSet) Unlink() error {
	if r.model.err != nil {
		return r.model.err
	}
	if err := r.model.check(OpUnlink, nil); err != nil {
		return err
	}
	oldRows := map[int64]map[string]any{}
	detachedRows := map[int64]map[string]any{}
	mailSnapshot := storeSnapshot{}
	trackingSnapshot := storeSnapshot{}
	envSnapshot := map[string]storeSnapshot{}
	needsActionBase := r.model.needsActionBaseSync()
	if r.model.model.Name == "account.move.line" {
		mailSnapshot = r.model.snapshotStore("mail.message")
		trackingSnapshot = r.model.snapshotStore("mail.tracking.value")
	}
	if len(r.model.env.beforeUnlinkHooks) > 0 || needsActionBase || r.model.model.Name == "account.move" || r.model.model.Name == "ir.attachment" || r.model.model.Name == "fetchmail.server" {
		envSnapshot = r.model.snapshotEnv()
	}
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		allowed, err := r.model.allowedRecord(OpUnlink, row)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("record rule denied unlink on %s:%d", r.model.model.Name, id)
		}
		switch r.model.model.Name {
		case "account.move":
			if err := r.model.validateAccountMoveUnlink(row); err != nil {
				return err
			}
		case "account.move.line":
			if err := r.model.validateAccountMoveLineUnlink(row); err != nil {
				return err
			}
		case "ir.attachment":
			detach, err := r.model.irAttachmentUnlinkDetaches(row)
			if err != nil {
				return err
			}
			if detach {
				detachedRows[id] = copyValues(row)
				continue
			}
		}
		oldRows[id] = copyValues(row)
	}
	for _, hook := range r.model.env.beforeUnlinkHooks {
		for _, id := range r.ids {
			row, ok := oldRows[id]
			if !ok {
				continue
			}
			if err := hook(r.model.env, r.model.model.Name, id, copyValues(row)); err != nil {
				r.model.restoreEnv(envSnapshot)
				return err
			}
		}
	}
	for _, id := range r.ids {
		if _, ok := oldRows[id]; !ok {
			continue
		}
		delete(r.model.store.records, id)
	}
	if r.model.model.Name == "ir.attachment" {
		for id := range detachedRows {
			r.model.detachIrAttachmentAuditTrail(id)
		}
	}
	if r.model.model.Name == "account.move" {
		moveIDs := make([]int64, 0, len(oldRows))
		for id := range oldRows {
			moveIDs = append(moveIDs, id)
		}
		r.model.unlinkAccountMoveInvoicePDFs(moveIDs)
	}
	if needsActionBase {
		base := r.model.env.Model("ir.actions.actions")
		if base.err != nil {
			r.model.restoreEnv(envSnapshot)
			return base.err
		}
		for id := range oldRows {
			delete(base.store.records, id)
		}
	}
	if r.model.model.Name == "account.move.line" {
		if err := r.model.trackAccountMoveLineUnlink(oldRows); err != nil {
			if len(r.model.env.beforeUnlinkHooks) > 0 {
				r.model.restoreEnv(envSnapshot)
			} else {
				for oldID, oldRow := range oldRows {
					r.model.store.records[oldID] = oldRow
				}
				r.model.restoreStore("mail.message", mailSnapshot)
				r.model.restoreStore("mail.tracking.value", trackingSnapshot)
			}
			return err
		}
	}
	if r.model.model.Name == "fetchmail.server" {
		if err := r.model.syncFetchmailGatewayCron(); err != nil {
			r.model.restoreEnv(envSnapshot)
			return err
		}
	}
	return nil
}

func (r RecordSet) RevokeAccountLockExceptions(canManage bool) error {
	if r.model.err != nil {
		return r.model.err
	}
	if r.model.model.Name != "account.lock_exception" {
		return fmt.Errorf("revoke is only supported on account.lock_exception")
	}
	now := time.Now().UTC()
	for _, id := range r.ids {
		row, ok := r.model.store.records[id]
		if !ok {
			continue
		}
		exception := r.model.lockExceptionFromValues(row)
		if exception.StateAt(now) != coreaccounting.LockExceptionActive {
			continue
		}
		revoked, err := coreaccounting.RevokeLockException(exception, now, canManage)
		if err != nil {
			return err
		}
		row["active"] = revoked.Active
		row["state"] = string(revoked.State)
		row["end_datetime"] = revoked.EndDatetime
	}
	return nil
}

func (r RecordSet) Mapped(fieldName string) ([]any, error) {
	rows, err := r.Read(fieldName)
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[fieldName])
	}
	return values, nil
}

func (r RecordSet) Filtered(fn func(map[string]any) bool) (RecordSet, error) {
	rows, err := r.Read()
	if err != nil {
		return RecordSet{}, err
	}
	var ids []int64
	for _, row := range rows {
		if fn(row) {
			ids = append(ids, row["id"].(int64))
		}
	}
	return RecordSet{model: r.model, ids: ids}, nil
}

type store struct {
	nextID  int64
	records map[int64]map[string]any
}

func (m ModelSet) check(op Operation, values map[string]any) error {
	if m.env.policy == nil {
		return nil
	}
	return m.env.policy.Check(m.env.context, m.model.Name, op, values)
}

func (m ModelSet) filterFields(fields []string) []string {
	if m.env.policy == nil {
		return fields
	}
	return m.env.policy.FilterFields(m.env.context, m.model.Name, fields)
}

func (m ModelSet) allowedRecord(op Operation, row map[string]any) (bool, error) {
	if m.env.policy == nil {
		return true, nil
	}
	return m.env.policy.CheckRecord(m.env.context, m.model.Name, op, row)
}

func (m ModelSet) recName() string {
	if m.model.RecName != "" {
		if _, ok := m.model.Fields[m.model.RecName]; ok {
			return m.model.RecName
		}
	}
	return "id"
}

func (m ModelSet) displayName(row map[string]any) string {
	if value, ok := row[m.recName()]; ok && value != nil {
		return fmt.Sprint(value)
	}
	return fmt.Sprintf("%s,%v", m.model.Name, row["id"])
}

type readGroupSpec struct {
	Key                string
	Name               string
	Interval           string
	Kind               field.Kind
	Relation           string
	WeekStart          int
	Timezone           *time.Location
	Property           bool
	PropertyField      string
	PropertyName       string
	PropertyType       string
	PropertyDefinition map[string]any
	DefinitionRecord   string
	DefinitionField    string
}

func (m ModelSet) readGroupSpecs(groupBy []string) ([]readGroupSpec, error) {
	out := make([]readGroupSpec, 0, len(groupBy))
	seen := map[string]bool{}
	weekStart := m.readGroupWeekStart()
	timezone := m.readGroupTimezone()
	for _, raw := range groupBy {
		fieldName, interval := readGroupFieldSpec(raw)
		if fieldName == "" || seen[fieldName] {
			continue
		}
		spec := readGroupSpec{Key: readGroupSpecKey(raw, fieldName, interval), Name: fieldName, Interval: interval, WeekStart: weekStart, Timezone: timezone}
		if fieldName != "id" {
			if propertySpec, ok, err := m.readGroupPropertySpec(fieldName, interval, weekStart, timezone); err != nil {
				return nil, err
			} else if ok {
				spec = propertySpec
				seen[fieldName] = true
				out = append(out, spec)
				continue
			}
			f, ok := m.model.Fields[fieldName]
			if !ok {
				return nil, fmt.Errorf("unknown field %s.%s", m.model.Name, fieldName)
			}
			if !f.Store {
				return nil, fmt.Errorf("read_group requires stored field %s.%s", m.model.Name, fieldName)
			}
			if f.Kind == field.One2Many {
				return nil, fmt.Errorf("read_group does not support grouping %s.%s", m.model.Name, fieldName)
			}
			spec.Kind = f.Kind
			spec.Relation = f.Relation
			if interval != "" {
				if f.Kind != field.Date && f.Kind != field.DateTime {
					return nil, fmt.Errorf("read_group interval %q requires date or datetime field %s.%s", interval, m.model.Name, fieldName)
				}
				if !readGroupValidTimeInterval(interval) {
					return nil, fmt.Errorf("read_group invalid interval %q for %s.%s", interval, m.model.Name, fieldName)
				}
			}
		}
		seen[fieldName] = true
		out = append(out, spec)
	}
	return out, nil
}

func (m ModelSet) readGroupPropertySpec(fieldName string, interval string, weekStart int, timezone *time.Location) (readGroupSpec, bool, error) {
	propertyField, propertyName, ok := strings.Cut(fieldName, ".")
	if !ok || propertyField == "" || propertyName == "" || strings.Contains(propertyName, ".") {
		return readGroupSpec{}, false, nil
	}
	f, ok := m.model.Fields[propertyField]
	if !ok || f.Kind != field.Properties {
		return readGroupSpec{}, false, nil
	}
	if !f.Store {
		return readGroupSpec{}, true, fmt.Errorf("read_group requires stored field %s.%s", m.model.Name, propertyField)
	}
	spec := readGroupSpec{
		Key:              readGroupSpecKey("", fieldName, interval),
		Name:             fieldName,
		Interval:         interval,
		Kind:             m.readGroupPropertyKind(f, propertyName),
		WeekStart:        weekStart,
		Timezone:         timezone,
		Property:         true,
		PropertyField:    propertyField,
		PropertyName:     propertyName,
		DefinitionRecord: f.DefinitionRecord,
		DefinitionField:  f.DefinitionField,
	}
	if spec.Kind == field.One2Many {
		return readGroupSpec{}, true, fmt.Errorf("read_group does not support grouping %s.%s", m.model.Name, fieldName)
	}
	if interval != "" {
		if readGroupNumberInterval(interval) {
			return readGroupSpec{}, true, fmt.Errorf("read_group interval %q is not supported for property field %s.%s", interval, m.model.Name, fieldName)
		}
		if spec.Kind != field.Date && spec.Kind != field.DateTime {
			return readGroupSpec{}, true, fmt.Errorf("read_group interval %q requires date or datetime field %s.%s", interval, m.model.Name, fieldName)
		}
		if !readGroupValidTimeInterval(interval) {
			return readGroupSpec{}, true, fmt.Errorf("read_group invalid interval %q for %s.%s", interval, m.model.Name, fieldName)
		}
	}
	return spec, true, nil
}

func (m ModelSet) readGroupPropertyKind(propertyField field.Field, propertyName string) field.Kind {
	if propertyField.DefinitionRecord == "" || propertyField.DefinitionField == "" {
		return field.Char
	}
	definitionRecordMeta, ok := m.model.Fields[propertyField.DefinitionRecord]
	if !ok || definitionRecordMeta.Relation == "" {
		return field.Char
	}
	for _, row := range m.rows(definitionRecordMeta.Relation) {
		for _, definition := range readGroupPropertyDefinitionMaps(row[propertyField.DefinitionField]) {
			if strings.TrimSpace(stringValue(definition["name"])) == propertyName {
				return readGroupPropertyKindFromType(stringValue(definition["type"]))
			}
		}
	}
	return field.Char
}

func readGroupPropertyKindFromType(value string) field.Kind {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "bool", "boolean":
		return field.Bool
	case "int", "integer":
		return field.Int
	case "float":
		return field.Float
	case "date":
		return field.Date
	case "datetime":
		return field.DateTime
	case "selection":
		return field.Selection
	case "many2one":
		return field.Many2One
	case "many2many", "tags":
		return field.Many2Many
	default:
		return field.Char
	}
}

func (m ModelSet) readGroupWeekStart() int {
	langCode := strings.TrimSpace(fmt.Sprint(m.env.context.Values["lang"]))
	if langCode == "" {
		langCode = "en_US"
	}
	if _, ok := m.env.registry.Model("res.lang"); !ok {
		return 7
	}
	langSet := m.env.Model("res.lang")
	found, err := langSet.SearchWithOptions(domain.Cond("code", domain.Equal, langCode), SearchOptions{Limit: 1})
	if err != nil || len(found.ids) == 0 {
		return 7
	}
	rows, err := found.Read("week_start")
	if err != nil || len(rows) == 0 {
		return 7
	}
	weekStart, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(rows[0]["week_start"])))
	if err != nil || weekStart < 1 || weekStart > 7 {
		return 7
	}
	return weekStart
}

func (m ModelSet) readGroupTimezone() *time.Location {
	timezone := strings.TrimSpace(fmt.Sprint(m.env.context.Values["tz"]))
	if timezone == "" {
		return nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil
	}
	return location
}

func readGroupValidTimeInterval(interval string) bool {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "day", "week", "month", "quarter", "year":
		return true
	case "year_number", "quarter_number", "month_number", "iso_week_number", "day_of_year", "day_of_month", "day_of_week", "hour_number", "minute_number", "second_number":
		return true
	default:
		return false
	}
}

func readGroupFieldName(raw string) string {
	name, _ := readGroupFieldSpec(raw)
	return name
}

func readGroupFieldSpec(raw string) (string, string) {
	fieldName := strings.TrimSpace(raw)
	interval := ""
	if cut := strings.Index(fieldName, ":"); cut >= 0 {
		interval = strings.TrimSpace(fieldName[cut+1:])
		fieldName = fieldName[:cut]
	}
	return strings.TrimSpace(fieldName), interval
}

func readGroupSpecKey(raw string, fieldName string, interval string) string {
	key := strings.TrimSpace(raw)
	if key != "" {
		return key
	}
	if strings.TrimSpace(interval) != "" {
		return fmt.Sprintf("%s:%s", fieldName, interval)
	}
	return fieldName
}

func readGroupSpecNames(groupBy []readGroupSpec) []string {
	out := make([]string, 0, len(groupBy))
	for _, spec := range groupBy {
		out = append(out, spec.Name)
	}
	return out
}

func readGroupReadFields(groupBy []readGroupSpec, aggregates []readGroupAggregateSpec) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, spec := range groupBy {
		if spec.Property {
			add(spec.PropertyField)
			add(spec.DefinitionRecord)
			continue
		}
		add(spec.Name)
	}
	for _, aggregate := range aggregates {
		add(aggregate.Field)
		if aggregate.Func == "sum_currency" {
			add(aggregate.CurrencyField)
		}
	}
	return out
}

func readGroupBucketSpecs(groupBy []readGroupSpec, overrides map[string]readGroupSpec) []readGroupSpec {
	out := make([]readGroupSpec, len(groupBy))
	for index, spec := range groupBy {
		if override, ok := overrides[spec.Name]; ok {
			out[index] = override
			continue
		}
		out[index] = spec
	}
	return out
}

func cloneAnyMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneReadGroupSpecMap(source map[string]readGroupSpec) map[string]readGroupSpec {
	out := make(map[string]readGroupSpec, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func (m ModelSet) readGroupRowValue(row map[string]any, spec readGroupSpec) (any, readGroupSpec) {
	if !spec.Property {
		return row[spec.Name], spec
	}
	rowSpec := m.readGroupPropertyRowSpec(row, spec)
	values := readGroupPropertyValueMap(row[spec.PropertyField])
	value, ok := values[spec.PropertyName]
	if !ok {
		value = m.readGroupPropertyDefault(row, rowSpec)
	}
	return value, rowSpec
}

func (m ModelSet) readGroupValues(rawValue any, spec readGroupSpec) ([]any, error) {
	if !spec.Property {
		if spec.Kind == field.Many2Many {
			values := m.readGroupMany2ManyValues(rawValue, spec)
			if len(values) == 0 {
				return []any{false}, nil
			}
			out := make([]any, 0, len(values))
			for _, value := range values {
				out = append(out, value)
			}
			return out, nil
		}
		value, err := readGroupValue(rawValue, spec)
		if err != nil {
			return nil, err
		}
		return []any{value}, nil
	}
	switch spec.PropertyType {
	case "selection":
		value, err := readGroupValue(rawValue, spec)
		if err != nil {
			return nil, err
		}
		if value == false || readGroupPropertySelectionHasValue(spec, value) {
			return []any{value}, nil
		}
		return []any{false}, nil
	case "many2one":
		id := numericID(rawValue)
		if id == 0 || !m.readGroupPropertyRelationExists(spec, id) {
			return []any{false}, nil
		}
		return []any{id}, nil
	case "many2many":
		ids := m.readGroupPropertyMany2ManyValues(rawValue, spec)
		if len(ids) == 0 {
			return []any{false}, nil
		}
		out := make([]any, 0, len(ids))
		for _, id := range ids {
			out = append(out, id)
		}
		return out, nil
	case "tags":
		tags := readGroupPropertyTagValues(rawValue, spec)
		if len(tags) == 0 {
			return []any{false}, nil
		}
		out := make([]any, 0, len(tags))
		for _, tag := range tags {
			out = append(out, tag)
		}
		return out, nil
	default:
		value, err := readGroupValue(rawValue, spec)
		if err != nil {
			return nil, err
		}
		return []any{value}, nil
	}
}

func (m ModelSet) readGroupMany2ManyValues(rawValue any, spec readGroupSpec) []int64 {
	values, err := collectionValues(rawValue)
	if err != nil {
		return nil
	}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		id := numericID(value)
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func readGroupPropertySelectionHasValue(spec readGroupSpec, value any) bool {
	options := readGroupPropertySelectionOptions(spec)
	if len(options) == 0 {
		return false
	}
	text := fmt.Sprint(domain.NormalizeScalar(value))
	for _, option := range options {
		if option == text {
			return true
		}
	}
	return false
}

func readGroupPropertySelectionOptions(spec readGroupSpec) []string {
	return readGroupPropertyPairFirstValues(spec.PropertyDefinition["selection"])
}

func readGroupPropertyTagOptions(spec readGroupSpec) []string {
	return readGroupPropertyPairFirstValues(spec.PropertyDefinition["tags"])
}

func readGroupPropertyPairFirstValues(value any) []string {
	items, err := collectionValues(value)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		values, err := collectionValues(item)
		if err != nil || len(values) == 0 {
			continue
		}
		key := strings.TrimSpace(fmt.Sprint(domain.NormalizeScalar(values[0])))
		if key != "" {
			out = append(out, key)
		}
	}
	return out
}

func (m ModelSet) readGroupPropertyRelationExists(spec readGroupSpec, id int64) bool {
	if strings.TrimSpace(spec.Relation) == "" {
		return false
	}
	return m.rowByID(spec.Relation, id) != nil
}

func (m ModelSet) readGroupPropertyMany2ManyValues(rawValue any, spec readGroupSpec) []int64 {
	values, err := collectionValues(rawValue)
	if err != nil {
		return nil
	}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		id := numericID(value)
		if id == 0 || !m.readGroupPropertyRelationExists(spec, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func readGroupPropertyTagValues(rawValue any, spec readGroupSpec) []string {
	values, err := collectionValues(rawValue)
	if err != nil {
		return nil
	}
	allowed := map[string]bool{}
	for _, tag := range readGroupPropertyTagOptions(spec) {
		allowed[tag] = true
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		tag := strings.TrimSpace(fmt.Sprint(domain.NormalizeScalar(value)))
		if tag != "" && allowed[tag] {
			out = append(out, tag)
		}
	}
	return out
}

func (m ModelSet) readGroupPropertyRowSpec(row map[string]any, spec readGroupSpec) readGroupSpec {
	if !spec.Property || spec.DefinitionRecord == "" || spec.DefinitionField == "" {
		return spec
	}
	definition := m.readGroupPropertyDefinition(row, spec)
	if definition == nil {
		return spec
	}
	spec.PropertyType = strings.ToLower(strings.TrimSpace(stringValue(definition["type"])))
	if kind := readGroupPropertyKindFromType(stringValue(definition["type"])); kind != "" {
		spec.Kind = kind
	}
	if relation := strings.TrimSpace(stringValue(definition["comodel"])); relation != "" {
		spec.Relation = relation
	}
	spec.PropertyDefinition = definition
	return spec
}

func (m ModelSet) readGroupPropertyDefault(row map[string]any, spec readGroupSpec) any {
	definition := m.readGroupPropertyDefinition(row, spec)
	if definition == nil {
		return nil
	}
	return definition["default"]
}

func (m ModelSet) readGroupPropertyDefinition(row map[string]any, spec readGroupSpec) map[string]any {
	definitionID := readGroupFirstPropertyID(row[spec.DefinitionRecord])
	if definitionID == 0 {
		return nil
	}
	definitionRecordMeta, ok := m.model.Fields[spec.DefinitionRecord]
	if !ok || definitionRecordMeta.Relation == "" {
		return nil
	}
	definitionRow := m.rowByID(definitionRecordMeta.Relation, definitionID)
	if definitionRow == nil {
		return nil
	}
	for _, definition := range readGroupPropertyDefinitionMaps(definitionRow[spec.DefinitionField]) {
		if strings.TrimSpace(stringValue(definition["name"])) == spec.PropertyName {
			return definition
		}
	}
	return nil
}

func readGroupFirstPropertyID(value any) int64 {
	switch typed := value.(type) {
	case []any:
		if len(typed) > 0 {
			return numericID(typed[0])
		}
	case []int64:
		if len(typed) > 0 {
			return typed[0]
		}
	case [2]any:
		return numericID(typed[0])
	}
	return numericID(value)
}

func readGroupPropertyValueMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err == nil && out != nil {
			return out
		}
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

func readGroupPropertyDefinitionMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if definition, ok := item.(map[string]any); ok {
				out = append(out, definition)
			}
		}
		return out
	default:
		return nil
	}
}

func readGroupValue(value any, spec readGroupSpec) (any, error) {
	if value == nil {
		return false, nil
	}
	if strings.TrimSpace(spec.Interval) != "" && (spec.Kind == field.Date || spec.Kind == field.DateTime) {
		if bucket, ok := readGroupTemporalBucket(value, spec); ok {
			return bucket, nil
		}
	}
	switch typed := domain.NormalizeScalar(value).(type) {
	case nil:
		return false, nil
	case bool, int64, float64, string:
		return typed, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func readGroupTemporalBucket(value any, spec readGroupSpec) (any, bool) {
	dateValue := recordDateValue(value)
	if dateValue.IsZero() {
		return nil, false
	}
	localTimezone := readGroupDateTimeTimezone(spec)
	if localTimezone != nil {
		dateValue = dateValue.UTC().In(localTimezone)
	} else {
		dateValue = dateValue.UTC()
	}
	if readGroupNumberInterval(spec.Interval) {
		return readGroupDatePartNumber(dateValue, spec), true
	}
	switch strings.ToLower(strings.TrimSpace(spec.Interval)) {
	case "day":
		dateValue = time.Date(dateValue.Year(), dateValue.Month(), dateValue.Day(), 0, 0, 0, 0, readGroupTemporalLocation(dateValue))
	case "week":
		start := readGroupWeekBucketStart(dateValue, spec.WeekStart)
		dateValue = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, readGroupTemporalLocation(start))
	case "month":
		dateValue = time.Date(dateValue.Year(), dateValue.Month(), 1, 0, 0, 0, 0, readGroupTemporalLocation(dateValue))
	case "quarter":
		month := time.Month(((int(dateValue.Month()) - 1) / 3 * 3) + 1)
		dateValue = time.Date(dateValue.Year(), month, 1, 0, 0, 0, 0, readGroupTemporalLocation(dateValue))
	case "year":
		dateValue = time.Date(dateValue.Year(), 1, 1, 0, 0, 0, 0, readGroupTemporalLocation(dateValue))
	default:
		return nil, false
	}
	if spec.Kind == field.DateTime {
		if localTimezone != nil {
			return dateValue.UTC().Format("2006-01-02 15:04:05"), true
		}
		return dateValue.Format("2006-01-02 15:04:05"), true
	}
	return dateValue.Format("2006-01-02"), true
}

func readGroupDateTimeTimezone(spec readGroupSpec) *time.Location {
	if spec.Kind != field.DateTime || spec.Timezone == nil {
		return nil
	}
	return spec.Timezone
}

func readGroupTemporalLocation(value time.Time) *time.Location {
	if location := value.Location(); location != nil {
		return location
	}
	return time.UTC
}

func readGroupWeekBucketStart(value time.Time, weekStart int) time.Time {
	if weekStart < 1 || weekStart > 7 {
		weekStart = 7
	}
	target := time.Weekday(weekStart % 7)
	diff := (int(value.Weekday()) - int(target) + 7) % 7
	return value.AddDate(0, 0, -diff)
}

func readGroupKey(value any) string {
	return fmt.Sprintf("%T:%v", value, value)
}

type readGroupBucket struct {
	values map[string]any
	specs  map[string]readGroupSpec
	rows   []map[string]any
	count  int
}

type readGroupValueBag map[string][]any

func readGroupAllValues(groupBy []readGroupSpec, groups []*readGroupBucket) readGroupValueBag {
	out := readGroupValueBag{}
	for _, groupSpec := range groupBy {
		seen := map[string]bool{}
		for _, group := range groups {
			value := group.values[groupSpec.Name]
			if readGroupFalsyValue(value) {
				continue
			}
			key := readGroupKey(value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out[groupSpec.Name] = append(out[groupSpec.Name], value)
		}
	}
	return out
}

func readGroupDomain(groupBy []readGroupSpec, values map[string]any, allValues readGroupValueBag) []any {
	out := make([]any, 0, len(groupBy))
	for _, groupSpec := range groupBy {
		if domainItems, ok := readGroupIntervalDomain(groupSpec, values[groupSpec.Name]); ok {
			out = append(out, domainItems...)
			continue
		}
		if domainItems, ok := readGroupPropertyDomain(groupSpec, values[groupSpec.Name], allValues[groupSpec.Name]); ok {
			out = append(out, domainItems...)
			continue
		}
		if domainItems, ok := readGroupMany2ManyDomain(groupSpec, values[groupSpec.Name]); ok {
			out = append(out, domainItems)
			continue
		}
		out = append(out, []any{groupSpec.Name, "=", values[groupSpec.Name]})
	}
	return out
}

func readGroupMany2ManyDomain(spec readGroupSpec, value any) ([]any, bool) {
	if spec.Property || spec.Kind != field.Many2Many {
		return nil, false
	}
	if readGroupFalsyValue(value) {
		return []any{spec.Name, "not any", []any{}}, true
	}
	return []any{spec.Name, "=", value}, true
}

func readGroupPropertyDomain(spec readGroupSpec, value any, allValues []any) ([]any, bool) {
	if !spec.Property {
		return nil, false
	}
	if readGroupFalsyValue(value) {
		switch spec.PropertyType {
		case "selection":
			return []any{readGroupPropertyFalsyOrNotInDomain(spec.Name, readGroupStringAnys(readGroupPropertySelectionOptions(spec)))}, true
		case "many2one":
			return []any{readGroupPropertyFalsyOrNotInDomain(spec.Name, allValues)}, true
		case "many2many":
			if len(allValues) == 0 {
				return nil, true
			}
			return []any{readGroupPropertyFalsyOrAllNotInDomain(spec.Name, allValues)}, true
		case "tags":
			tags := readGroupStringAnys(readGroupPropertyTagOptions(spec))
			if len(tags) == 0 {
				return nil, true
			}
			return []any{readGroupPropertyFalsyOrAllNotInDomain(spec.Name, tags)}, true
		default:
			return []any{[]any{spec.Name, "=", false}}, true
		}
	}
	switch spec.PropertyType {
	case "many2many", "tags":
		return []any{[]any{spec.Name, "in", value}}, true
	default:
		return []any{[]any{spec.Name, "=", value}}, true
	}
}

func readGroupPropertyFalsyOrNotInDomain(name string, values []any) []any {
	return readGroupOrDomainComponents([]any{
		[]any{name, "=", false},
		[]any{name, "not in", values},
	})
}

func readGroupPropertyFalsyOrAllNotInDomain(name string, values []any) []any {
	out := []any{"|", []any{name, "=", false}}
	for index := 1; index < len(values); index++ {
		out = append(out, "&")
	}
	for _, value := range values {
		out = append(out, []any{name, "not in", value})
	}
	return out
}

func readGroupStringAnys(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func readGroupFalsyValue(value any) bool {
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
		values, err := collectionValues(value)
		return err == nil && len(values) == 0
	}
}

func readGroupOrDomainComponents(components []any) []any {
	switch len(components) {
	case 0:
		return nil
	case 1:
		if component, ok := components[0].([]any); ok {
			return component
		}
		return []any{components[0]}
	default:
		out := make([]any, 0, len(components)*2-1)
		for index := 1; index < len(components); index++ {
			out = append(out, "|")
		}
		out = append(out, components...)
		return out
	}
}

func readGroupRanges(groupBy []readGroupSpec, values map[string]any) map[string]any {
	ranges := map[string]any{}
	for _, groupSpec := range groupBy {
		if rangeValue, ok := readGroupIntervalRangeValue(groupSpec, values[groupSpec.Name]); ok {
			ranges[groupSpec.Name] = rangeValue
		}
	}
	return ranges
}

func readGroupIntervalRangeValue(spec readGroupSpec, value any) (any, bool) {
	if strings.TrimSpace(spec.Interval) == "" || readGroupNumberInterval(spec.Interval) || (spec.Kind != field.Date && spec.Kind != field.DateTime) {
		return nil, false
	}
	start, end, ok := readGroupIntervalRange(spec, value)
	if ok {
		return map[string]any{
			"from": readGroupTemporalDomainValue(start, spec.Kind),
			"to":   readGroupTemporalDomainValue(end, spec.Kind),
		}, true
	}
	if value == nil || value == false || value == "" {
		return false, true
	}
	return nil, false
}

func readGroupFormattedValue(spec readGroupSpec, value any) (any, bool) {
	if strings.TrimSpace(spec.Interval) == "" || readGroupNumberInterval(spec.Interval) || (spec.Kind != field.Date && spec.Kind != field.DateTime) {
		return nil, false
	}
	if value == nil || value == false || value == "" {
		return false, true
	}
	label, ok := readGroupTemporalLabel(value, spec)
	if !ok {
		return nil, false
	}
	return []any{value, label}, true
}

func readGroupIntervalDomain(spec readGroupSpec, value any) ([]any, bool) {
	if strings.TrimSpace(spec.Interval) == "" || (spec.Kind != field.Date && spec.Kind != field.DateTime) {
		return nil, false
	}
	if readGroupNumberInterval(spec.Interval) {
		return []any{readGroupNumberIntervalDomain(spec, value)}, true
	}
	start, end, ok := readGroupIntervalRange(spec, value)
	if !ok {
		return nil, false
	}
	return []any{
		[]any{spec.Name, ">=", readGroupTemporalDomainValue(start, spec.Kind)},
		[]any{spec.Name, "<", readGroupTemporalDomainValue(end, spec.Kind)},
	}, true
}

func readGroupIntervalRange(spec readGroupSpec, value any) (time.Time, time.Time, bool) {
	start, ok := readGroupIntervalStart(spec, value)
	if !ok || start.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	end, ok := readGroupIntervalEnd(start, spec.Interval)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func readGroupTemporalLabel(value any, spec readGroupSpec) (string, bool) {
	start, ok := readGroupIntervalStart(spec, value)
	if !ok || start.IsZero() {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(spec.Interval)) {
	case "day":
		return start.Format("02 Jan 2006"), true
	case "week":
		year, week := readGroupWeekNumber(start, spec.WeekStart)
		return fmt.Sprintf("W%d %04d", week, year), true
	case "month":
		return start.Format("January 2006"), true
	case "quarter":
		return fmt.Sprintf("Q%d %d", ((int(start.Month())-1)/3)+1, start.Year()), true
	case "year":
		return strconv.Itoa(start.Year()), true
	default:
		return "", false
	}
}

func readGroupNumberInterval(interval string) bool {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "year_number", "quarter_number", "month_number", "iso_week_number", "day_of_year", "day_of_month", "day_of_week", "hour_number", "minute_number", "second_number":
		return true
	default:
		return false
	}
}

func readGroupNumberIntervalDomain(spec readGroupSpec, value any) []any {
	fieldName := fmt.Sprintf("%s.%s", spec.Name, strings.ToLower(strings.TrimSpace(spec.Interval)))
	if value == nil || value == false || value == "" {
		return []any{fieldName, "=", false}
	}
	return []any{fieldName, "=", value}
}

func readGroupDatePartNumber(value time.Time, spec readGroupSpec) int {
	switch strings.ToLower(strings.TrimSpace(spec.Interval)) {
	case "year_number":
		return value.Year()
	case "quarter_number":
		return ((int(value.Month()) - 1) / 3) + 1
	case "month_number":
		return int(value.Month())
	case "iso_week_number":
		_, week := value.ISOWeek()
		return week
	case "day_of_year":
		return value.YearDay()
	case "day_of_month":
		return value.Day()
	case "day_of_week":
		return int(value.Weekday())
	case "hour_number":
		if spec.Kind == field.Date {
			return 0
		}
		return value.Hour()
	case "minute_number":
		if spec.Kind == field.Date {
			return 0
		}
		return value.Minute()
	case "second_number":
		if spec.Kind == field.Date {
			return 0
		}
		return value.Second()
	default:
		return 0
	}
}

func readGroupWeekNumber(value time.Time, weekStart int) (int, int) {
	if weekStart < 1 || weekStart > 7 {
		weekStart = 7
	}
	if weekStart == 1 {
		year, week := value.ISOWeek()
		return year, week
	}
	location := value.Location()
	if location == nil {
		location = time.UTC
	}
	nextYearStart := time.Date(value.Year()+1, 1, 1, 0, 0, 0, 0, location)
	firstNextYearWeekStart := readGroupWeekBucketStart(nextYearStart, weekStart)
	if !value.Before(firstNextYearWeekStart) {
		return value.Year() + 1, 1
	}
	currentYearStart := time.Date(value.Year(), 1, 1, 0, 0, 0, 0, location)
	firstCurrentYearWeekStart := readGroupWeekBucketStart(currentYearStart, weekStart)
	days := readGroupCalendarDayDiff(firstCurrentYearWeekStart, value)
	return value.Year(), days/7 + 1
}

func readGroupCalendarDayDiff(start time.Time, end time.Time) int {
	startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(endDate.Sub(startDate).Hours() / 24)
}

func readGroupIntervalStart(spec readGroupSpec, value any) (time.Time, bool) {
	if timezone := readGroupDateTimeTimezone(spec); timezone != nil {
		start := recordDateValue(value)
		if start.IsZero() {
			return time.Time{}, false
		}
		return start.UTC().In(timezone), true
	}
	start := recordDateValue(value)
	return start, !start.IsZero()
}

func readGroupIntervalEnd(start time.Time, interval string) (time.Time, bool) {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "day":
		return start.AddDate(0, 0, 1), true
	case "week":
		return start.AddDate(0, 0, 7), true
	case "month":
		return start.AddDate(0, 1, 0), true
	case "quarter":
		return start.AddDate(0, 3, 0), true
	case "year":
		return start.AddDate(1, 0, 0), true
	default:
		return time.Time{}, false
	}
}

func readGroupTemporalDomainValue(value time.Time, kind field.Kind) string {
	value = value.UTC()
	if kind == field.DateTime {
		return value.Format("2006-01-02 15:04:05")
	}
	return value.Format("2006-01-02")
}

type readGroupOrderTerm struct {
	Term       string
	Source     string
	Kind       field.Kind
	Relation   string
	Descending bool
	NullsFirst bool
	Explicit   bool
}

func (m ModelSet) sortReadGroupRows(rows []map[string]any, groupBy []readGroupSpec, aggregates []readGroupAggregateSpec, order string) error {
	if len(rows) < 2 {
		return nil
	}
	terms, err := m.readGroupOrderTerms(order, groupBy, aggregates)
	if err != nil {
		return err
	}
	if len(terms) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for _, term := range terms {
			left, leftNull := m.readGroupOrderValue(rows[i], term)
			right, rightNull := m.readGroupOrderValue(rows[j], term)
			cmp := readGroupOrderCompare(left, leftNull, right, rightNull, term)
			if cmp == 0 {
				continue
			}
			if term.Descending {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	return nil
}

func (m ModelSet) readGroupOrderTerms(order string, groupBy []readGroupSpec, aggregates []readGroupAggregateSpec) ([]readGroupOrderTerm, error) {
	explicit := strings.TrimSpace(order) != ""
	parts := []string{}
	if explicit {
		parts = strings.Split(order, ",")
	} else {
		for _, spec := range groupBy {
			parts = append(parts, spec.Key)
		}
	}
	terms := make([]readGroupOrderTerm, 0, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		match := readGroupOrderPartPattern.FindStringSubmatch(raw)
		if match == nil {
			return nil, fmt.Errorf("read_group invalid order %q", order)
		}
		term := readGroupOrderTerm{Term: match[1], Explicit: explicit}
		direction := strings.ToLower(strings.TrimSpace(match[2]))
		term.Descending = direction == "desc"
		nulls := strings.ToLower(strings.Join(strings.Fields(match[3]), " "))
		switch nulls {
		case "nulls first":
			term.NullsFirst = true
		case "nulls last":
			term.NullsFirst = false
		default:
			term.NullsFirst = term.Descending
		}
		resolved, ok := readGroupResolveOrderTerm(term, groupBy, aggregates)
		if !ok {
			return nil, fmt.Errorf("read_group order term %q is not a valid aggregate or groupby", raw)
		}
		terms = append(terms, resolved)
	}
	return terms, nil
}

func readGroupResolveOrderTerm(term readGroupOrderTerm, groupBy []readGroupSpec, aggregates []readGroupAggregateSpec) (readGroupOrderTerm, bool) {
	for _, spec := range groupBy {
		if term.Term == spec.Key || term.Term == spec.Name {
			term.Source = spec.Name
			term.Kind = spec.Kind
			term.Relation = spec.Relation
			return term, true
		}
	}
	for _, aggregate := range aggregates {
		if term.Term == aggregate.Key {
			term.Source = aggregate.Key
			term.Kind = aggregate.Kind
			term.Relation = aggregate.Relation
			return term, true
		}
		if aggregate.Func != "__count" && term.Term == aggregate.Field+":"+aggregate.Func {
			term.Source = aggregate.Key
			term.Kind = aggregate.Kind
			term.Relation = aggregate.Relation
			return term, true
		}
	}
	if term.Term == "__count" {
		term.Source = "__count"
		term.Kind = field.Int
		return term, true
	}
	if len(groupBy) == 1 && term.Term == readGroupLegacyCountKey(groupBy[0]) {
		term.Source = "__count"
		term.Kind = field.Int
		return term, true
	}
	return readGroupOrderTerm{}, false
}

func (m ModelSet) readGroupOrderValue(row map[string]any, term readGroupOrderTerm) (any, bool) {
	value := row[term.Source]
	null := value == nil
	if value == false && term.Kind != field.Bool {
		null = true
	}
	if term.Explicit && term.Kind == field.Many2One && term.Relation != "" && !null {
		if relatedModel, ok := m.env.registry.Model(term.Relation); ok && relatedModel.Order != "" && strings.TrimSpace(relatedModel.Order) != "id" {
			if relatedRow := m.rowByID(term.Relation, numericID(value)); relatedRow != nil {
				if ordered := readGroupRelatedOrderValue(relatedModel, relatedRow); ordered != nil {
					return ordered, false
				}
			}
		}
	}
	return value, null
}

func readGroupRelatedOrderValue(relatedModel model.Model, row map[string]any) any {
	for _, part := range strings.Split(relatedModel.Order, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		name = strings.Fields(name)[0]
		if name == "id" {
			return row["id"]
		}
		if value, ok := row[name]; ok {
			return value
		}
	}
	return nil
}

func readGroupOrderCompare(left any, leftNull bool, right any, rightNull bool, term readGroupOrderTerm) int {
	if leftNull || rightNull {
		if leftNull && rightNull {
			return 0
		}
		if leftNull {
			if term.NullsFirst {
				return -1
			}
			return 1
		}
		if term.NullsFirst {
			return 1
		}
		return -1
	}
	return readGroupCompareNonNull(left, right)
}

func readGroupCompareNonNull(left any, right any) int {
	switch leftValue := left.(type) {
	case int:
		return readGroupCompareFloat(float64(leftValue), readGroupOrderFloat(right))
	case int64:
		return readGroupCompareFloat(float64(leftValue), readGroupOrderFloat(right))
	case float32:
		return readGroupCompareFloat(float64(leftValue), readGroupOrderFloat(right))
	case float64:
		return readGroupCompareFloat(leftValue, readGroupOrderFloat(right))
	case bool:
		rightValue, _ := right.(bool)
		if leftValue == rightValue {
			return 0
		}
		if !leftValue && rightValue {
			return -1
		}
		return 1
	case time.Time:
		rightValue, ok := right.(time.Time)
		if !ok {
			return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
		}
		if leftValue.Equal(rightValue) {
			return 0
		}
		if leftValue.Before(rightValue) {
			return -1
		}
		return 1
	default:
		return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
	}
}

func readGroupCompareFloat(left float64, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func readGroupOrderFloat(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float32:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func paginateReadGroupRows(rows []map[string]any, opts ReadGroupOptions) []map[string]any {
	if opts.Offset > 0 {
		if opts.Offset >= len(rows) {
			return nil
		}
		rows = rows[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(rows) {
		rows = rows[:opts.Limit]
	}
	return rows
}

func fieldDescription(f field.Field) map[string]any {
	out := map[string]any{
		"string":                    f.Label,
		"type":                      string(f.Kind),
		"required":                  f.Required,
		"readonly":                  f.Readonly,
		"store":                     f.Store,
		"sortable":                  f.Store,
		"manual":                    false,
		"company_dependent":         f.CompanyDependent,
		"searchable":                f.Store,
		"exportable":                true,
		"name":                      f.Name,
		"depends":                   []string{},
		"change_default":            false,
		"deprecated":                false,
		"aggregator":                false,
		"trim":                      true,
		"translate":                 f.Translate,
		"groups":                    strings.Join(f.Groups, ","),
		"relation":                  f.Relation,
		"relation_field":            f.RelationField,
		"context":                   cloneAnyMap(f.Context),
		"currency_field":            f.CurrencyField,
		"definition_record":         f.DefinitionRecord,
		"definition_record_field":   f.DefinitionField,
		"group_expand":              false,
		"help":                      "",
		"default_export_compatible": f.DefaultExport,
	}
	if selection := selectionDescription(f.Selection); len(selection) > 0 {
		out["selection"] = selection
	}
	if f.Aggregator != "" {
		out["aggregator"] = f.Aggregator
	}
	for key, value := range out {
		if value == "" || value == nil {
			delete(out, key)
		}
	}
	return out
}

func filterDescription(description map[string]any, attributes map[string]bool) map[string]any {
	if len(attributes) == 0 {
		return description
	}
	out := map[string]any{}
	for attr := range attributes {
		if value, ok := description[attr]; ok {
			out[attr] = value
		}
	}
	return out
}

func selectionDescription(selection []field.SelectionOption) [][2]string {
	if len(selection) == 0 {
		return nil
	}
	out := make([][2]string, 0, len(selection))
	for _, item := range selection {
		out = append(out, [2]string{item.Value, item.Label})
	}
	return out
}

func match(row map[string]any, node domain.Node) (bool, error) {
	return matchWithContext(Context{}, row, node)
}

func matchWithContext(ctx Context, row map[string]any, node domain.Node) (bool, error) {
	return matchWithModelContext(ctx, model.Model{}, row, node)
}

func matchWithModelContext(ctx Context, meta model.Model, row map[string]any, node domain.Node) (bool, error) {
	switch node.Kind {
	case domain.Literal:
		value, _ := domain.NormalizeScalar(node.Value).(bool)
		return value, nil
	case domain.Condition:
		left := valueForFieldWithModelContext(ctx, meta, row, node.Field)
		if invalid, ok := left.(invalidFieldValue); ok {
			return false, invalid.err
		}
		switch node.Operator {
		case domain.Equal:
			return valuesEqual(left, node.Value), nil
		case domain.NotEqual:
			return !valuesEqual(left, node.Value), nil
		case domain.OptionalEqual:
			if !isTruthy(node.Value) {
				return true, nil
			}
			return valuesEqual(left, node.Value), nil
		case domain.In:
			return valueIn(left, node.Value)
		case domain.NotIn:
			ok, err := valueIn(left, node.Value)
			return !ok, err
		case domain.Less, domain.LessEqual, domain.Greater, domain.GreaterEqual:
			return compare(left, node.Value, node.Operator)
		case domain.Like:
			return containsMatch(left, node.Value, false, false), nil
		case domain.NotLike:
			return !containsMatch(left, node.Value, false, false), nil
		case domain.ILike:
			return containsMatch(left, node.Value, true, false), nil
		case domain.NotILike:
			return !containsMatch(left, node.Value, true, false), nil
		case domain.EqualLike:
			return containsMatch(left, node.Value, false, true), nil
		case domain.NotEqualLike:
			return !containsMatch(left, node.Value, false, true), nil
		case domain.EqualILike:
			return containsMatch(left, node.Value, true, true), nil
		case domain.NotEqualILike:
			return !containsMatch(left, node.Value, true, true), nil
		case domain.ChildOf, domain.ParentOf:
			return valueIn(left, node.Value)
		case domain.AnyOf:
			return valueAny(left, node.Value)
		case domain.NotAnyOf:
			ok, err := valueAny(left, node.Value)
			return !ok, err
		default:
			return false, fmt.Errorf("record search operator %s not implemented in memory matcher", node.Operator)
		}
	case domain.All:
		for _, child := range node.Children {
			ok, err := matchWithModelContext(ctx, meta, row, child)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	case domain.Any:
		for _, child := range node.Children {
			ok, err := matchWithModelContext(ctx, meta, row, child)
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
		ok, err := matchWithModelContext(ctx, meta, row, node.Children[0])
		return !ok, err
	default:
		return false, fmt.Errorf("unsupported domain kind %s", node.Kind)
	}
}

type invalidFieldValue struct {
	err error
}

func valueForField(row map[string]any, fieldName string) any {
	return valueForFieldWithContext(Context{}, row, fieldName)
}

func valueForFieldWithContext(ctx Context, row map[string]any, fieldName string) any {
	return valueForFieldWithModelContext(ctx, model.Model{}, row, fieldName)
}

func valueForFieldWithModelContext(ctx Context, meta model.Model, row map[string]any, fieldName string) any {
	current := any(row)
	currentKind := field.Kind("")
	parts := strings.Split(fieldName, ".")
	for index := 0; index < len(parts); index++ {
		part := parts[index]
		if values, ok := current.(map[string]any); ok {
			current = values[part]
			if currentKind == "" {
				if f, ok := meta.Fields[part]; ok {
					currentKind = f.Kind
					if f.Kind == field.Properties {
						if index+1 >= len(parts) {
							return current
						}
						index++
						propertyName := parts[index]
						current = readGroupPropertyValueMap(current)[propertyName]
						currentKind = ""
						if index+1 < len(parts) {
							return invalidFieldValue{err: fmt.Errorf("unsupported property path %s", fieldName)}
						}
					}
				}
			}
			continue
		}
		if readGroupNumberInterval(part) {
			return valueDatePartNumberWithKind(ctx, current, part, currentKind)
		}
		return nil
	}
	return current
}

func valueDatePartNumber(ctx Context, value any, part string) any {
	return valueDatePartNumberWithKind(ctx, value, part, "")
}

func valueDatePartNumberWithKind(ctx Context, value any, part string, kind field.Kind) any {
	dateValue := recordDateValue(value)
	if dateValue.IsZero() {
		return false
	}
	if kind == field.DateTime || (kind != field.Date && shouldApplyDatePartTimezone(value)) {
		if location := contextTimezone(ctx); location != nil {
			dateValue = dateValue.UTC().In(location)
		} else {
			dateValue = dateValue.UTC()
		}
	} else {
		dateValue = dateValue.UTC()
	}
	partKind := field.DateTime
	if kind == field.Date {
		partKind = field.Date
	}
	return readGroupDatePartNumber(dateValue, readGroupSpec{Interval: part, Kind: partKind})
}

func shouldApplyDatePartTimezone(value any) bool {
	switch typed := value.(type) {
	case string:
		return len(strings.TrimSpace(typed)) > len("2006-01-02")
	case time.Time:
		return typed.Hour() != 0 || typed.Minute() != 0 || typed.Second() != 0 || typed.Nanosecond() != 0
	default:
		return false
	}
}

func contextTimezone(ctx Context) *time.Location {
	timezone := strings.TrimSpace(fmt.Sprint(ctx.Values["tz"]))
	if timezone == "" {
		return nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil
	}
	return location
}

func valuesEqual(left any, right any) bool {
	left = domain.NormalizeScalar(left)
	right = domain.NormalizeScalar(right)
	if left == nil || right == nil {
		return isFalsey(left) && isFalsey(right)
	}
	if leftValues, leftErr := collectionValues(left); leftErr == nil {
		if _, rightErr := collectionValues(right); rightErr != nil {
			for _, leftValue := range leftValues {
				if valuesEqual(leftValue, right) {
					return true
				}
			}
			return false
		}
	}
	if rightValues, rightErr := collectionValues(right); rightErr == nil {
		if _, leftErr := collectionValues(left); leftErr != nil {
			for _, rightValue := range rightValues {
				if valuesEqual(left, rightValue) {
					return true
				}
			}
			return false
		}
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

func valueAny(left any, right any) (bool, error) {
	leftValues, err := collectionValues(left)
	if err != nil {
		return false, nil
	}
	child, err := domain.Parse(right)
	if err == nil {
		if child.Kind == domain.All && len(child.Children) == 0 {
			return len(leftValues) > 0, nil
		}
		return false, fmt.Errorf("record search operator any with nested domain is not implemented in memory matcher")
	}
	rightValues, err := collectionValues(right)
	if err != nil {
		return false, nil
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
