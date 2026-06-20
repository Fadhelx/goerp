package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
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
	"gorp/internal/ai/embeddings"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/ai/rag"
	"gorp/internal/ai/sources"
	aitools "gorp/internal/ai/tools"
	"gorp/internal/assets"
	internalbase "gorp/internal/base"
	"gorp/internal/data"
	internaldelegation "gorp/internal/delegation"
	"gorp/internal/domain"
	internalmail "gorp/internal/mail"
	"gorp/internal/meta/view"
	"gorp/internal/module"
	"gorp/internal/notifications"
	"gorp/internal/record"
	"gorp/internal/scheduler"
	"gorp/internal/security"
	internalworkflow "gorp/internal/workflow"
)

func TestBootstrapOIInstallsRuntimeModules(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"base_setup",
		"onboarding",
		"uom",
		"product",
		"analytic",
		"portal",
		"digest",
		hr.ModuleName,
		aiaddon.ModuleName,
		oi_base.ModuleName,
		oi_workflow.ModuleName,
		oi_workflow_advance.ModuleName,
		oi_delegation.ModuleName,
		oi_login_as.ModuleName,
	} {
		if app.ModuleRegistry.States[name] != "installed" {
			t.Fatalf("module %s state = %q", name, app.ModuleRegistry.States[name])
		}
		if app.Modules[name].TechnicalName != name {
			t.Fatalf("server module %s missing: %+v", name, app.Modules[name])
		}
	}
	if app.ModuleRegistry.States[accounting.ModuleName] == "installed" {
		t.Fatal("accounting should be skipped in phase 1 by default")
	}
	if _, ok := app.Modules[accounting.ModuleName]; ok {
		t.Fatal("accounting module should not be exposed in default runtime")
	}
	found, err := app.Env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("name", "state")
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{}
	for _, row := range rows {
		if row["state"] == "installed" {
			installed[row["name"].(string)] = true
		}
	}
	for _, name := range []string{"base_setup", "onboarding", "uom", "product", "analytic", "portal", "digest", hr.ModuleName, aiaddon.ModuleName, oi_workflow.ModuleName, oi_delegation.ModuleName, oi_login_as.ModuleName} {
		if !installed[name] {
			t.Fatalf("missing ir.module.module row %s in %+v", name, rows)
		}
	}
	departmentMeta, ok := app.Env.ModelMetadata("hr.department")
	if !ok || departmentMeta.Fields["parent_id"].Relation != "hr.department" || departmentMeta.Fields["member_ids"].RelationField != "department_id" {
		t.Fatalf("hr.department metadata = %+v", departmentMeta)
	}
	employeeMeta, ok := app.Env.ModelMetadata("hr.employee")
	if !ok || employeeMeta.Fields["department_id"].Relation != "hr.department" || employeeMeta.Fields["user_id"].Relation != "res.users" {
		t.Fatalf("hr.employee metadata = %+v", employeeMeta)
	}
	userMeta, ok := app.Env.ModelMetadata("res.users")
	if !ok || userMeta.Fields["employee_ids"].RelationField != "user_id" || userMeta.Fields["employee_id"].Relation != "hr.employee" {
		t.Fatalf("res.users HR extension metadata = %+v", userMeta)
	}
	if _, ok := app.Modules[aiaddon.ModuleName]; !ok || app.Server().AIChatFactory == nil {
		t.Fatalf("ai runtime not wired: modules=%+v server=%+v", app.Modules, app.Server())
	}
	if app.ExternalIDs["account.account_payment_term_immediate"].ResID != 0 {
		t.Fatalf("accounting data loaded while disabled: %+v", app.ExternalIDs["account.account_payment_term_immediate"])
	}
	if app.ExternalIDs["oi_login_as.portal_login_as_banner"].ResID == 0 {
		t.Fatalf("missing persisted template external id: %+v", app.ExternalIDs)
	}
	if app.ExternalIDs["ai.ai_default_agent"].ResID == 0 || app.ExternalIDs["ai.ai_composer_mail"].ResID == 0 || app.ExternalIDs["ai.ai_mail_composer"].ResID == 0 || app.ExternalIDs["ai.ai_systray_action"].ResID == 0 {
		t.Fatalf("missing AI data external id: %+v", app.ExternalIDs)
	}
	if app.ExternalIDs["mail.ir_cron_mail_scheduler_action"].ResID == 0 {
		t.Fatalf("missing mail queue cron external id: %+v", app.ExternalIDs)
	}
	if app.ExternalIDs["mail.ir_cron_mail_gateway_action"].ResID == 0 {
		t.Fatalf("missing fetchmail cron external id: %+v", app.ExternalIDs)
	}
	if app.ExternalIDs["mass_mailing.ir_cron_mass_mailing_queue"].ResID == 0 {
		t.Fatalf("missing mass mailing cron external id: %+v", app.ExternalIDs)
	}
	if app.ExternalIDs["digest.ir_cron_digest_scheduler_action"].ResID == 0 {
		t.Fatalf("missing digest cron external id: %+v", app.ExternalIDs)
	}
	queueResult, err := app.ServerActions.RunNamed(context.Background(), "mail.process_email_queue", serveractions.ExecutionContext{Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if queueResult.Metadata["processed"] != 0 || queueResult.Metadata["sent"] != 0 || queueResult.Metadata["failed"] != 0 {
		t.Fatalf("mail queue action result = %+v", queueResult.Metadata)
	}
	fetchmailResult, err := app.ServerActions.RunNamed(context.Background(), "mail.fetchmail", serveractions.ExecutionContext{Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if fetchmailResult.Metadata["fetched"] != 0 || fetchmailResult.Metadata["processed"] != 0 || fetchmailResult.Metadata["failed"] != 0 {
		t.Fatalf("fetchmail action result = %+v", fetchmailResult.Metadata)
	}
	massMailingResult, err := app.ServerActions.RunNamed(context.Background(), "mailing.mailing.process_mass_mailing_queue", serveractions.ExecutionContext{Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if massMailingResult.Metadata["done"] != 0 || massMailingResult.Metadata["skipped"] != 0 || massMailingResult.Metadata["kpi_reports"] != 0 {
		t.Fatalf("mass mailing action result = %+v", massMailingResult.Metadata)
	}
	digestResult, err := app.ServerActions.RunNamed(context.Background(), "digest.digest._cron_send_digest_email", serveractions.ExecutionContext{Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if digestResult.Metadata["sent"] != 0 || digestResult.Metadata["skipped"] != 0 || digestResult.Metadata["slowed_down"] != 0 {
		t.Fatalf("digest action result = %+v", digestResult.Metadata)
	}
	aiActionID := app.ExternalIDs["ai.action_ai_record_assistant"].ResID
	aiAction, ok := app.ServerActions.Get(aiActionID)
	if aiActionID == 0 || !ok || aiAction.Kind != serveractions.KindAI {
		t.Fatalf("AI server action not loaded: id=%d action=%+v", aiActionID, aiAction)
	}
	if aiAction.Metadata["xml_id"] != "ai.action_ai_record_assistant" {
		t.Fatalf("AI server action XML ID metadata = %+v", aiAction.Metadata)
	}
	adminID := app.ExternalIDs["base.user_admin"].ResID
	if adminID == 0 || app.Security.Users[adminID].Login != "admin" {
		t.Fatalf("admin user not hydrated: id=%d users=%+v", adminID, app.Security.Users)
	}
	systemGroupID := app.ExternalIDs["base.group_system"].ResID
	if systemGroupID == 0 || !app.Security.EffectiveGroupIDs(adminID)[systemGroupID] {
		t.Fatalf("admin groups not hydrated: groups=%+v", app.Security.EffectiveGroupIDs(adminID))
	}
	if app.ServerActions == nil || app.Server().Workflow == nil {
		t.Fatal("workflow dispatcher/server-action registry not wired")
	}
	if app.Delegation == nil || app.Security.Delegations == nil {
		t.Fatal("delegation runtime provider not wired")
	}
}

func TestBootstrapOIAccountingPhaseGate(t *testing.T) {
	t.Setenv("GORP_ENABLE_ACCOUNTING", "1")
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	if app.ModuleRegistry.States[accounting.ModuleName] != "installed" {
		t.Fatalf("accounting state = %q", app.ModuleRegistry.States[accounting.ModuleName])
	}
	if app.Modules[accounting.ModuleName].TechnicalName != accounting.ModuleName {
		t.Fatalf("accounting module missing: %+v", app.Modules[accounting.ModuleName])
	}
	if app.ExternalIDs["account.account_payment_term_immediate"].ResID == 0 {
		t.Fatalf("missing accounting external id: %+v", app.ExternalIDs["account.account_payment_term_immediate"])
	}
}

func TestCSVModelNameFromTemplatePath(t *testing.T) {
	cases := map[string]string{
		"/addons/account/data/template/account.tax-generic_coa.csv": "account.tax",
		"/addons/account/data/template/account.account.csv":         "account.account",
		"/addons/base/security/ir.model.access.csv":                 "ir.model.access",
	}
	for path, want := range cases {
		if got := csvModelNameFromPath(path); got != want {
			t.Fatalf("csvModelNameFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRuntimeFetchmailCronDeactivatesWhenNoEligibleServers(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	cronID := app.ExternalIDs["mail.ir_cron_mail_gateway_action"].ResID
	if cronID == 0 {
		t.Fatalf("missing fetchmail cron external id: %+v", app.ExternalIDs)
	}
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	allCrons, err := app.Env.Model("ir.cron").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	allCronRows, err := allCrons.Read("id")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range allCronRows {
		if err := app.Env.Model("ir.cron").Browse(int64Value(row["id"])).Write(map[string]any{"active": false}); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Env.Model("ir.cron").Browse(cronID).Write(map[string]any{"active": true, "nextcall": now, "ir_actions_server_id": int64(0)}); err != nil {
		t.Fatal(err)
	}

	s := scheduler.New()
	if err := s.LoadFromEnv(app.Env); err != nil {
		t.Fatal(err)
	}
	result := s.RunDueActions(context.Background(), now, app.ServerActions)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if err := s.SyncToEnv(app.Env); err != nil {
		t.Fatal(err)
	}
	cronRows, err := app.Env.Model("ir.cron").Browse(cronID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(cronRows) != 1 || cronRows[0]["active"] != false {
		t.Fatalf("cron rows = %+v", cronRows)
	}
	progressRows, err := app.Env.Model("ir.cron.progress").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		t.Fatal(err)
	}
	progress, err := progressRows.Read("done", "remaining", "deactivate")
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0]["done"] != 0 || progress[0]["remaining"] != 0 || progress[0]["deactivate"] != true {
		t.Fatalf("progress = %+v", progress)
	}
	triggers, err := app.Env.Model("ir.cron.trigger").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		t.Fatal(err)
	}
	if triggers.Len() != 0 {
		t.Fatalf("unexpected fetchmail triggers = %d", triggers.Len())
	}
}

func TestFetchmailCronEndTimeUsesExplicitAndDefaultBudgets(t *testing.T) {
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	explicit := now.Add(30 * time.Second)
	if got := fetchmailCronEndTime(serveractions.ExecutionContext{Trigger: "cron", Metadata: map[string]any{"cron_end_time": explicit}}, now); !got.Equal(explicit) {
		t.Fatalf("explicit cron_end_time = %s", got)
	}
	if got := fetchmailCronEndTime(serveractions.ExecutionContext{Trigger: "cron", Metadata: map[string]any{"cron_time_budget_seconds": int64(7)}}, now); !got.Equal(now.Add(7 * time.Second)) {
		t.Fatalf("explicit seconds budget = %s", got)
	}
	if got := fetchmailCronEndTime(serveractions.ExecutionContext{Trigger: "cron"}, now); !got.Equal(now.Add(10 * time.Second)) {
		t.Fatalf("default cron budget = %s", got)
	}
	if got := fetchmailCronEndTime(serveractions.ExecutionContext{}, now); !got.IsZero() {
		t.Fatalf("manual budget = %s", got)
	}
}

type runtimeFetchmailConnector struct {
	conns map[int64]*runtimeFetchmailConnection
}

func (c *runtimeFetchmailConnector) Connect(_ context.Context, server internalmail.FetchmailServerConfig) (internalmail.FetchmailConnection, error) {
	if c != nil && c.conns != nil {
		if conn := c.conns[server.ID]; conn != nil {
			return conn, nil
		}
	}
	return &runtimeFetchmailConnection{}, nil
}

type runtimeFetchmailConnection struct {
	messages    []internalmail.FetchedMessage
	unreadCount int
	marked      []string
	closed      bool
}

func (c *runtimeFetchmailConnection) CheckUnreadMessages(context.Context) (int, error) {
	if c.unreadCount > 0 {
		return c.unreadCount, nil
	}
	return len(c.messages), nil
}

func (c *runtimeFetchmailConnection) RetrieveUnreadMessages(_ context.Context, limit int) ([]internalmail.FetchedMessage, error) {
	if limit > 0 && len(c.messages) > limit {
		return append([]internalmail.FetchedMessage(nil), c.messages[:limit]...), nil
	}
	return append([]internalmail.FetchedMessage(nil), c.messages...), nil
}

func (c *runtimeFetchmailConnection) MarkHandled(_ context.Context, message internalmail.FetchedMessage) error {
	c.marked = append(c.marked, message.Num)
	return nil
}

func (c *runtimeFetchmailConnection) Close() error {
	c.closed = true
	return nil
}

func TestRuntimeFetchmailCronProgressSchedulesImmediateTriggerForRemainingMessages(t *testing.T) {
	registry := record.NewRegistry()
	for _, model := range internalbase.Models() {
		if err := registry.Register(model); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	serverID, err := env.Model("fetchmail.server").Create(map[string]any{
		"name":        "Progress IMAP",
		"active":      true,
		"state":       "done",
		"server":      "mock",
		"server_type": "imap",
		"attach":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	conn := &runtimeFetchmailConnection{
		unreadCount: 2,
		messages: []internalmail.FetchedMessage{{
			Num: "1",
			Raw: []byte(strings.Join([]string{
				"Message-Id: <runtime-fetch-progress@remote>",
				"From: Runtime Fetch <runtime.fetch@example.com>",
				"To: catch@example.com",
				"Subject: Runtime Fetch Progress",
				"Content-Type: text/plain; charset=utf-8",
				"",
				"Runtime fetch body",
				"",
			}, "\r\n")),
		}},
	}
	app := &App{FetchmailConnector: &runtimeFetchmailConnector{conns: map[int64]*runtimeFetchmailConnection{serverID: conn}}}
	registryActions := serveractions.NewRegistry(serveractions.Hooks{})
	if err := registryActions.RegisterGo("mail.fetchmail", mailRuntimeFetchmail(env, app)); err != nil {
		t.Fatal(err)
	}

	s := scheduler.New()
	cron, err := s.AddCron(scheduler.Cron{
		Name:           "Mail: Fetchmail Service",
		Active:         true,
		ActionName:     "mail.fetchmail",
		IntervalNumber: 5,
		IntervalType:   scheduler.IntervalMinutes,
		NextCall:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := s.RunDueActions(context.Background(), now, registryActions)
	if result.Ran != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	progress, ok := s.Progress(cron.ID)
	if !ok || progress.Done != 2 || progress.Remaining != 1 || progress.Deactivate {
		t.Fatalf("progress = %+v", progress)
	}
	triggers := s.Triggers(cron.ID)
	if len(triggers) != 1 {
		t.Fatalf("fetchmail triggers = %+v", triggers)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" || !conn.closed {
		t.Fatalf("connection marked=%+v closed=%v", conn.marked, conn.closed)
	}
}

func TestFetchmailServerLifecycleTogglesGatewayCron(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	cronID := app.ExternalIDs["mail.ir_cron_mail_gateway_action"].ResID
	if cronID == 0 {
		t.Fatalf("missing fetchmail cron external id: %+v", app.ExternalIDs)
	}
	if err := app.Env.Model("ir.cron").Browse(cronID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	models, err := app.Env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, "res.partner"), record.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	modelRows, err := models.Read("id")
	if err != nil || len(modelRows) == 0 {
		t.Fatalf("res.partner model rows = %+v err=%v", modelRows, err)
	}
	modelID := int64Value(modelRows[0]["id"])

	assertActive := func(want bool) {
		t.Helper()
		rows, err := app.Env.Model("ir.cron").Browse(cronID).Read("active")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["active"] != want {
			t.Fatalf("cron active = %+v, want %v", rows, want)
		}
	}

	serverID, err := app.Env.Model("fetchmail.server").Create(map[string]any{
		"name":        "Lifecycle IMAP",
		"active":      true,
		"state":       "draft",
		"server_type": "imap",
		"object_id":   modelID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertActive(false)
	if err := app.Env.Model("fetchmail.server").Browse(serverID).Write(map[string]any{"state": "done"}); err != nil {
		t.Fatal(err)
	}
	assertActive(true)
	if err := app.Env.Model("fetchmail.server").Browse(serverID).Write(map[string]any{"active": false}); err != nil {
		t.Fatal(err)
	}
	assertActive(false)
	if err := app.Env.Model("fetchmail.server").Browse(serverID).Write(map[string]any{"active": true, "server_type": "local"}); err != nil {
		t.Fatal(err)
	}
	assertActive(false)
	if err := app.Env.Model("fetchmail.server").Browse(serverID).Write(map[string]any{"server_type": "imap"}); err != nil {
		t.Fatal(err)
	}
	assertActive(true)
	if err := app.Env.Model("fetchmail.server").Browse(serverID).Unlink(); err != nil {
		t.Fatal(err)
	}
	assertActive(false)
}

func TestBootstrapOIRegistersWorkflowEscalationAction(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.ServerActions.RunNamed(context.Background(), "workflow.process.escalation", serveractions.ExecutionContext{
		UserID: 1,
		Now:    time.Date(2026, 6, 16, 12, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != serveractions.KindGo || result.GoActionName != "workflow.process.escalation" {
		t.Fatalf("result = %+v", result)
	}
	if result.Metadata["due"] != 0 || result.Metadata["applied"] != 0 || result.Metadata["skipped"] != 0 {
		t.Fatalf("metadata = %+v", result.Metadata)
	}
}

func TestViewsFromEnvHydratesActivePriorityAndGroups(t *testing.T) {
	registry := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Restricted View Users"})
	if err != nil {
		t.Fatal(err)
	}
	publicID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":     "res.partner.public.form",
		"model":    "res.partner",
		"type":     "form",
		"arch":     `<form><field name="name"/></form>`,
		"priority": int64(20),
		"active":   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":      "res.partner.private.form",
		"model":     "res.partner",
		"type":      "form",
		"arch":      `<form><field name="name"/></form>`,
		"priority":  int64(5),
		"active":    true,
		"groups_id": []int64{groupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	inactiveID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":     "res.partner.inactive.form",
		"model":    "res.partner",
		"type":     "form",
		"arch":     `<form><field name="name"/></form>`,
		"priority": int64(1),
		"active":   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	views, err := viewsFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	publicViews := views.ForModelAndType("res.partner", view.Form, map[int64]bool{})
	if len(publicViews) != 1 || publicViews[0].ID != publicID {
		t.Fatalf("public views = %+v", publicViews)
	}
	groupViews := views.ForModelAndType("res.partner", view.Form, map[int64]bool{groupID: true})
	if len(groupViews) != 2 || groupViews[0].ID != privateID || groupViews[1].ID != publicID {
		t.Fatalf("group views = %+v", groupViews)
	}
	if _, ok := views.Get(inactiveID); ok {
		t.Fatalf("inactive view loaded: %d", inactiveID)
	}
}

func TestViewsFromEnvHydratesMode(t *testing.T) {
	registry := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	rootID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":   "res.partner.root.form",
		"model":  "res.partner",
		"type":   "form",
		"arch":   `<form><field name="name"/></form>`,
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	extensionID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":       "res.partner.extension.form",
		"model":      "res.partner",
		"type":       "form",
		"inherit_id": rootID,
		"mode":       "extension",
		"arch":       `<xpath expr="//field[@name='name']" position="after"><field name="email"/></xpath>`,
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	primaryID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":       "res.partner.primary.form",
		"model":      "res.partner",
		"type":       "form",
		"inherit_id": rootID,
		"mode":       "primary",
		"arch":       `<xpath expr="//field[@name='name']" position="after"><field name="phone"/></xpath>`,
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	views, err := viewsFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	extension, ok := views.Get(extensionID)
	if !ok {
		t.Fatalf("extension view %d missing", extensionID)
	}
	if extension.Mode != "extension" {
		t.Fatalf("extension mode = %q", extension.Mode)
	}
	primary, ok := views.Get(primaryID)
	if !ok {
		t.Fatalf("primary view %d missing", primaryID)
	}
	if primary.Mode != "primary" {
		t.Fatalf("primary mode = %q", primary.Mode)
	}
}

func TestMenusFromEnvSerializesNonWindowActionXMLIDs(t *testing.T) {
	registry := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	rootID, err := env.Model("ir.ui.menu").Create(map[string]any{"name": "Root"})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.actions.server").Create(map[string]any{"name": "Run Server", "model_name": "res.partner", "state": "code"})
	if err != nil {
		t.Fatal(err)
	}
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{"name": "Partner Report", "model": "res.partner", "report_name": "base.partner_report"})
	if err != nil {
		t.Fatal(err)
	}
	urlID, err := env.Model("ir.actions.act_url").Create(map[string]any{"name": "Docs", "url": "https://example.test/docs"})
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := env.Model("ir.actions.client").Create(map[string]any{"name": "Discuss", "tag": "mail.action_discuss"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name  string
		model string
		id    int64
	}{
		{name: "Server", model: "ir.actions.server", id: serverID},
		{name: "Report", model: "ir.actions.report", id: reportID},
		{name: "URL", model: "ir.actions.act_url", id: urlID},
		{name: "Client", model: "ir.actions.client", id: clientID},
	} {
		if _, err := env.Model("ir.ui.menu").Create(map[string]any{
			"name":      item.name,
			"parent_id": rootID,
			"action":    item.model + "," + strconv.FormatInt(item.id, 10),
		}); err != nil {
			t.Fatal(err)
		}
	}
	externalIDs := map[string]data.ExternalID{
		"base.menu_root":     {Module: "base", Name: "menu_root", Model: "ir.ui.menu", ResID: rootID},
		"base.action_server": {Module: "base", Name: "action_server", Model: "ir.actions.server", ResID: serverID},
		"base.action_report": {Module: "base", Name: "action_report", Model: "ir.actions.report", ResID: reportID},
		"base.action_url":    {Module: "base", Name: "action_url", Model: "ir.actions.act_url", ResID: urlID},
		"base.action_client": {Module: "base", Name: "action_client", Model: "ir.actions.client", ResID: clientID},
	}
	reg, err := menusFromEnv(env, externalIDs)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]struct {
		action string
		model  string
		id     int64
	}{}
	for _, root := range reg.Tree(map[int64]bool{}) {
		for _, child := range root.Children {
			byName[child.Menu.Name] = struct {
				action string
				model  string
				id     int64
			}{action: child.Menu.Action, model: child.Menu.ActionModel, id: child.Menu.ActionID}
		}
	}
	for _, item := range []struct {
		name  string
		model string
		id    int64
		xmlID string
	}{
		{name: "Server", model: "ir.actions.server", id: serverID, xmlID: "base.action_server"},
		{name: "Report", model: "ir.actions.report", id: reportID, xmlID: "base.action_report"},
		{name: "URL", model: "ir.actions.act_url", id: urlID, xmlID: "base.action_url"},
		{name: "Client", model: "ir.actions.client", id: clientID, xmlID: "base.action_client"},
	} {
		got := byName[item.name]
		if got.action != item.model+","+item.xmlID || got.model != item.model || got.id != item.id {
			t.Fatalf("%s menu = %+v", item.name, got)
		}
	}
}

func TestBootstrapOIDelegationProviderAffectsSecurityGroups(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	delegableGroupID := app.ExternalIDs["oi_delegation.group_delegable_role"].ResID
	if delegableGroupID == 0 {
		t.Fatalf("missing delegable group external id: %+v", app.ExternalIDs)
	}
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	app.Security.SetNow(func() time.Time { return now })
	app.Delegation.SetNow(func() time.Time { return now })
	app.Security.Users[70] = security.User{ID: 70, Login: "delegator", Active: true, GroupIDs: []int64{delegableGroupID}, CompanyID: 1, CompanyIDs: []int64{1}}
	app.Security.Users[80] = security.User{ID: 80, Login: "delegate", Active: true, CompanyID: 1, CompanyIDs: []int64{1}}
	if app.Security.EffectiveGroupIDs(80)[delegableGroupID] {
		t.Fatal("delegate has group before confirmed delegation")
	}
	req, err := app.Delegation.CreateRequest(internaldelegation.RequestInput{
		DateFrom:        now.Add(-time.Hour),
		DateTo:          now.Add(time.Hour),
		DelegatorUserID: 70,
		Lines:           []internaldelegation.LineInput{{GroupID: delegableGroupID, DelegateUserID: 80}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Delegation.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if !app.Security.EffectiveGroupIDs(80)[delegableGroupID] {
		t.Fatalf("delegate effective groups = %+v", app.Security.EffectiveGroupIDs(80))
	}
}

func TestOIEnvDelegationInheritsAdvancedApprovalRecordFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := env.ModelMetadata(oi_delegation.ModelDelegation)
	if !ok {
		t.Fatal("missing delegation model")
	}
	for _, fieldName := range []string{"approval_user_ids", "approval_done_user_ids", "approval_partner_ids", "user_can_approve", "workflow_node_id", "workflow_view_id", "message_ids", "activity_ids"} {
		if _, ok := metadata.Fields[fieldName]; !ok {
			t.Fatalf("delegation missing inherited field %s", fieldName)
		}
	}
	if metadata.Fields["approval_state_id"].Relation != internalworkflow.ModelConfig {
		t.Fatalf("approval_state_id relation = %s", metadata.Fields["approval_state_id"].Relation)
	}
}

func TestBootstrapOIServerActionHooksPreserveObjectEvaluatorAndSequence(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	equationID, err := app.ServerActions.Register(serveractions.ServerAction{
		Name:            "Equation Hook",
		Model:           "res.partner",
		Kind:            serveractions.KindWrite,
		UpdatePath:      "city",
		UpdateFieldType: "char",
		EvaluationType:  "equation",
		Value:           "record.name + '-ok'",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ServerActions.Run(context.Background(), equationID, serveractions.ExecutionContext{Model: "res.partner", RecordID: partnerID}); err != nil {
		t.Fatal(err)
	}
	sequenceID, err := app.Env.Model("ir.sequence").Create(map[string]any{
		"name":             "Hook Seq",
		"code":             "hook.seq",
		"prefix":           "H/",
		"padding":          int64(3),
		"number_next":      int64(4),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sequenceActionID, err := app.ServerActions.Register(serveractions.ServerAction{
		Name:            "Sequence Hook",
		Model:           "res.partner",
		Kind:            serveractions.KindWrite,
		UpdatePath:      "name",
		UpdateFieldType: "char",
		EvaluationType:  "sequence",
		SequenceID:      sequenceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ServerActions.Run(context.Background(), sequenceActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: partnerID}); err != nil {
		t.Fatal(err)
	}
	rows, err := app.Env.Model("res.partner").Browse(partnerID).Read("name", "city")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["city"] != "Ada-ok" || rows[0]["name"] != "H/004" {
		t.Fatalf("partner rows = %+v", rows)
	}
}

func TestActionsFromEnvIncludesEmbeddedActions(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Partners",
		"type":      "ir.actions.act_window",
		"res_model": "res.partner",
		"view_mode": "list,form",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.embedded.actions").Create(map[string]any{
		"name":              "Later",
		"sequence":          int64(20),
		"parent_action_id":  actionID,
		"parent_res_id":     int64(42),
		"parent_res_model":  "res.partner",
		"action_id":         int64(99),
		"python_method":     "action_later",
		"user_id":           int64(3),
		"is_deletable":      true,
		"default_view_mode": "form",
		"filter_ids":        []int64{6},
		"is_visible":        true,
		"domain":            `[["active","=",true]]`,
		"context":           `{"default_partner_id": 42}`,
		"groups_ids":        []int64{7},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.embedded.actions").Create(map[string]any{
		"name":             "First",
		"sequence":         int64(10),
		"parent_action_id": actionID,
		"parent_res_model": "res.partner",
		"python_method":    "action_first",
		"is_visible":       true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.embedded.actions").Create(map[string]any{
		"name":             "Hidden",
		"sequence":         int64(5),
		"parent_action_id": actionID,
		"parent_res_model": "res.partner",
		"is_visible":       false,
	}); err != nil {
		t.Fatal(err)
	}
	actions, err := actionsFromEnv(env, map[string]data.ExternalID{
		"base.action_partner_test": {Module: "base", Name: "action_partner_test", Model: "ir.actions.act_window", ResID: actionID},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := actions.Get(actionID)
	if !ok {
		t.Fatalf("missing action %d", actionID)
	}
	if loaded.XMLID != "base.action_partner_test" || len(loaded.EmbeddedActions) != 2 {
		t.Fatalf("loaded action = %+v", loaded)
	}
	if loaded.EmbeddedActions[0].Name != "First" || loaded.EmbeddedActions[1].Name != "Later" {
		t.Fatalf("embedded order = %+v", loaded.EmbeddedActions)
	}
	later := loaded.EmbeddedActions[1]
	if later.ParentResID != 42 || later.ParentResModel != "res.partner" || later.ActionID != 99 || later.UserID != 3 || !later.IsDeletable {
		t.Fatalf("embedded metadata = %+v", later)
	}
	if len(later.FilterIDs) != 1 || later.FilterIDs[0] != 6 || len(later.GroupIDs) != 1 || later.GroupIDs[0] != 7 {
		t.Fatalf("embedded relation ids = %+v", later)
	}
}

func TestActionsFromEnvIncludesWorkflowCreateContext(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	viewID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":  "purchase.order.form.workflow",
		"model": "purchase.order",
		"type":  "form",
		"arch":  `<form><field name="name"/></form>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":    "Auto Submit PO",
		"model":   "purchase.order",
		"active":  true,
		"view_id": viewID,
		"create_context": `{
			'approval_auto_submit': True,
			'default_note': 'hello, world',
			'default_count': 42,
			'default_list': [1, 'two', False],
			'default_tuple': (3, 'four')
		}`,
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Purchase Orders",
		"type":      "ir.actions.act_window",
		"res_model": "purchase.order",
		"view_mode": "list,form",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabledActionID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Plain Purchase Orders",
		"type":      "ir.actions.act_window",
		"res_model": "purchase.order",
		"view_mode": "list,form",
		"context":   "{'multi_workflow_view': False}",
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, err := actionsFromEnv(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := actions.Get(actionID)
	if !ok || loaded.MultiWorkflowView == "" {
		t.Fatalf("workflow action = %+v ok=%v", loaded, ok)
	}
	var workflowViews []map[string]any
	if err := json.Unmarshal([]byte(loaded.MultiWorkflowView), &workflowViews); err != nil {
		t.Fatal(err)
	}
	if len(workflowViews) != 1 ||
		workflowViews[0]["name"] != "Auto Submit PO" ||
		int64Value(workflowViews[0]["view_id"]) != viewID ||
		runtimeMapValue(workflowViews[0]["create_context"])["approval_auto_submit"] != true {
		t.Fatalf("workflow views = %+v", workflowViews)
	}
	createContext := runtimeMapValue(workflowViews[0]["create_context"])
	if createContext["default_note"] != "hello, world" ||
		int64Value(createContext["default_count"]) != int64(42) {
		t.Fatalf("literal create_context = %+v", createContext)
	}
	list := createContext["default_list"].([]any)
	if len(list) != 3 || int64Value(list[0]) != 1 || list[1] != "two" || list[2] != false {
		t.Fatalf("default_list = %+v", list)
	}
	tuple := createContext["default_tuple"].([]any)
	if len(tuple) != 2 || int64Value(tuple[0]) != 3 || tuple[1] != "four" {
		t.Fatalf("default_tuple = %+v", tuple)
	}
	disabled, ok := actions.Get(disabledActionID)
	if !ok || disabled.MultiWorkflowView != "" {
		t.Fatalf("disabled workflow action = %+v ok=%v", disabled, ok)
	}
}

func TestActionsFromEnvErrorsOnInvalidWorkflowCreateContext(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	viewID, err := env.Model("ir.ui.view").Create(map[string]any{
		"name":  "purchase.order.form.workflow.invalid",
		"model": "purchase.order",
		"type":  "form",
		"arch":  `<form><field name="name"/></form>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":           "Invalid PO Context",
		"model":          "purchase.order",
		"active":         true,
		"view_id":        viewID,
		"create_context": `{'default_company_id': env.company.id}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Purchase Orders",
		"type":      "ir.actions.act_window",
		"res_model": "purchase.order",
		"view_mode": "list,form",
	}); err != nil {
		t.Fatal(err)
	}

	_, err = actionsFromEnv(env, nil)
	if err == nil ||
		!strings.Contains(err.Error(), "create_context") ||
		!strings.Contains(err.Error(), "env.company.id") {
		t.Fatalf("actionsFromEnv err = %v", err)
	}
}

func TestServerActionsFromEnvRunsMailThreadStates(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner", "is_mail_thread": true, "is_mail_activity": true})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	followerID, err := env.Model("res.partner").Create(map[string]any{"name": "Grace"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":      "Ping Template",
		"model":     "res.partner",
		"subject":   "Ping",
		"body_html": "<p>Hello</p>",
		"email_to":  "ada@example.com",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityTypeID, err := env.Model("mail.activity.type").Create(map[string]any{"name": "Todo", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                 "Post Template",
		"model_id":             modelID,
		"model_name":           "res.partner",
		"state":                "mail_post",
		"active":               true,
		"template_id":          templateID,
		"mail_post_method":     "comment",
		"mail_post_autofollow": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	followerActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "Add Follower",
		"model_id":       modelID,
		"model_name":     "res.partner",
		"state":          "followers",
		"active":         true,
		"followers_type": "specific",
		"partner_ids":    []int64{followerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	activityActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                              "Create Activity",
		"model_id":                          modelID,
		"model_name":                        "res.partner",
		"state":                             "next_activity",
		"active":                            true,
		"activity_type_id":                  activityTypeID,
		"activity_summary":                  "Review",
		"activity_note":                     "<p>Check</p>",
		"activity_date_deadline_range":      int64(2),
		"activity_date_deadline_range_type": "days",
		"activity_user_type":                "specific",
		"activity_user_id":                  int64(7),
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	mailAction, ok := registry.Get(mailActionID)
	if !ok || mailAction.Kind != serveractions.KindMailPost || mailAction.TemplateID != templateID || mailAction.MailPostMethod != "comment" {
		t.Fatalf("mail action = %+v ok=%v", mailAction, ok)
	}
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	if _, err := registry.Run(context.Background(), mailActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID, Now: now}); err != nil {
		t.Fatal(err)
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("res_id", domain.Equal, recordID))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("model", "body", "subject", "message_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["model"] != "res.partner" || messageRows[0]["body"] != "<p>Hello</p>" || messageRows[0]["subject"] != "Ping" {
		t.Fatalf("messages = %+v", messageRows)
	}

	if _, err := registry.Run(context.Background(), followerActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID}); err != nil {
		t.Fatal(err)
	}
	followers, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", domain.Equal, "res.partner"),
		domain.Cond("res_id", domain.Equal, recordID),
		domain.Cond("partner_id", domain.Equal, followerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if followers.Len() != 1 {
		t.Fatalf("followers len = %d", followers.Len())
	}

	if _, err := registry.Run(context.Background(), activityActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID, Now: now}); err != nil {
		t.Fatal(err)
	}
	activities, err := env.Model("mail.activity").Search(domain.Cond("res_id", domain.Equal, recordID))
	if err != nil {
		t.Fatal(err)
	}
	activityRows, err := activities.Read("activity_type_id", "summary", "note", "date_deadline", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(activityRows) != 1 || activityRows[0]["activity_type_id"] != activityTypeID || activityRows[0]["summary"] != "Review" || activityRows[0]["date_deadline"] != "2026-06-19" || activityRows[0]["user_id"] != int64(7) {
		t.Fatalf("activities = %+v", activityRows)
	}
}

func TestServerActionsFromEnvRunsEnterpriseTransportAndDocumentStates(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	documentModelID, err := env.Model("ir.model").Create(map[string]any{"model": "documents.document", "name": "Document"})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	smsActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "SMS",
		"model_id":        partnerModelID,
		"model_name":      "res.partner",
		"state":           "sms",
		"active":          true,
		"sms_template_id": int64(11),
		"sms_method":      "sms",
	})
	if err != nil {
		t.Fatal(err)
	}
	whatsAppTemplateID, err := env.Model("whatsapp.template").Create(map[string]any{
		"name":   "WhatsApp",
		"body":   "WhatsApp template 12",
		"status": "approved",
		"model":  "res.partner",
	})
	if err != nil {
		t.Fatal(err)
	}
	whatsAppActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp",
		"model_id":       partnerModelID,
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": whatsAppTemplateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	documentActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                           "Create Vendor Bill",
		"model_id":                       documentModelID,
		"model_name":                     "documents.document",
		"state":                          "documents_account_record_create",
		"active":                         true,
		"documents_account_create_model": "account.move.in_invoice",
		"documents_account_journal_id":   int64(7),
		"documents_account_move_type":    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	smsAction, ok := registry.Get(smsActionID)
	if !ok || smsAction.Kind != serveractions.KindSMS || smsAction.SMSTemplateID != 11 || smsAction.SMSMethod != "sms" {
		t.Fatalf("sms action = %+v ok=%v", smsAction, ok)
	}
	whatsAppAction, ok := registry.Get(whatsAppActionID)
	if !ok || whatsAppAction.Kind != serveractions.KindWhatsApp || whatsAppAction.WhatsAppTemplateID != whatsAppTemplateID {
		t.Fatalf("whatsapp action = %+v ok=%v", whatsAppAction, ok)
	}
	documentAction, ok := registry.Get(documentActionID)
	if !ok || documentAction.Kind != serveractions.KindDocumentAccount || documentAction.DocumentsAccountMoveType != "in_invoice" {
		t.Fatalf("document action = %+v ok=%v", documentAction, ok)
	}

	smsResult, err := registry.Run(context.Background(), smsActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID})
	if err != nil {
		t.Fatal(err)
	}
	if !smsResult.SMSSent {
		t.Fatalf("sms result = %+v", smsResult)
	}
	whatsAppResult, err := registry.Run(context.Background(), whatsAppActionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID})
	if err != nil {
		t.Fatal(err)
	}
	if !whatsAppResult.WhatsAppSent {
		t.Fatalf("whatsapp result = %+v", whatsAppResult)
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("res_id", domain.Equal, recordID))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body", "message_type")
	if err != nil {
		t.Fatal(err)
	}
	seenMessages := map[string]bool{}
	for _, row := range messageRows {
		seenMessages[stringValue(row["message_type"])+":"+stringValue(row["body"])] = true
	}
	if !seenMessages["sms:SMS template 11"] || !seenMessages["whatsapp:WhatsApp template 12"] {
		t.Fatalf("messages = %+v", messageRows)
	}

	documentResult, err := registry.Run(context.Background(), documentActionID, serveractions.ExecutionContext{Model: "documents.document", RecordIDs: []int64{101, 102}})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := documentResult.Action.(map[string]any)
	if !ok || action["res_model"] != "account.move" {
		t.Fatalf("document result = %+v", documentResult)
	}
	moves, err := env.Model("account.move").Search(domain.Cond("move_type", domain.Equal, "in_invoice"))
	if err != nil {
		t.Fatal(err)
	}
	moveRows, err := moves.Read("name", "move_type", "journal_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(moveRows) != 2 {
		t.Fatalf("moves = %+v", moveRows)
	}
	for _, row := range moveRows {
		if row["move_type"] != "in_invoice" || row["journal_id"] != int64(7) {
			t.Fatalf("move row = %+v", row)
		}
	}
}

func TestRuntimeWhatsAppTemplateGeneratesTrackedLinks(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	utmCampaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "WhatsApp UTM"})
	if err != nil {
		t.Fatal(err)
	}
	marketingCampaignID, err := env.Model("marketing.campaign").Create(map[string]any{"name": "WhatsApp Campaign", "utm_campaign_id": utmCampaignID})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Activity", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Tracked WA", "body": "Open {{1}}", "status": "approved", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "https://example.com/body"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"wa_template_id": templateID, "sequence": int64(0), "button_type": "url", "url_type": "tracked", "website_url": "https://example.com/button", "text": "Tracked"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"wa_template_id": templateID, "sequence": int64(1), "button_type": "url", "url_type": "dynamic", "dynamic_url": "???", "text": "Dynamic"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "WA Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := env.Model("whatsapp.message").Create(map[string]any{"state": "outgoing", "wa_template_id": templateID, "model": "res.partner", "res_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp",
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": templateID,
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"whatsapp_message_id": whatsappID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WhatsAppSent {
		t.Fatalf("whatsapp result = %+v", result)
	}

	trackers, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err := trackers.Read("url", "code", "campaign_id", "short_url", "count")
	if err != nil {
		t.Fatal(err)
	}
	codeByURL := map[string]string{}
	for _, row := range trackerRows {
		if int64Value(row["campaign_id"]) != utmCampaignID {
			t.Fatalf("tracker row missing campaign: %+v", row)
		}
		codeByURL[stringValue(row["url"])] = stringValue(row["code"])
	}
	buttonCode := codeByURL["https://example.com/button"]
	bodyCode := codeByURL["https://example.com/body"]
	if buttonCode == "" || bodyCode == "" {
		t.Fatalf("tracker codes by url = %+v rows=%+v", codeByURL, trackerRows)
	}
	rows, err := env.Model("whatsapp.message").Browse(whatsappID).Read("body", "components", "mail_message_id", "wa_template_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || int64Value(rows[0]["mail_message_id"]) == 0 || int64Value(rows[0]["wa_template_id"]) != templateID {
		t.Fatalf("whatsapp rows = %+v", rows)
	}
	body := stringValue(rows[0]["body"])
	if body != "Open https://example.com/body" {
		t.Fatalf("stored body = %s", body)
	}
	var components []map[string]any
	if err := json.Unmarshal([]byte(stringValue(rows[0]["components"])), &components); err != nil {
		t.Fatal(err)
	}
	buttonTexts := map[int64]string{}
	bodyText := ""
	for _, component := range components {
		params, _ := component["parameters"].([]any)
		if len(params) == 0 {
			continue
		}
		param, _ := params[0].(map[string]any)
		switch component["type"] {
		case "body":
			bodyText = stringValue(param["text"])
		case "button":
			buttonTexts[int64Value(component["index"])] = stringValue(param["text"])
		}
	}
	if bodyText != "https://gorp.example/r/"+bodyCode+"/w/"+strconv.FormatInt(whatsappID, 10) {
		t.Fatalf("tracked body parameter = %q components=%+v", bodyText, components)
	}
	if buttonTexts[0] != "r/"+buttonCode+"/w/"+strconv.FormatInt(whatsappID, 10) {
		t.Fatalf("tracked button text = %+v", buttonTexts)
	}
	if buttonTexts[1] != "???" {
		t.Fatalf("dynamic button text = %+v", buttonTexts)
	}

	routePaths := map[string]string{
		"https://example.com/body":   strings.TrimPrefix(bodyText, "https://gorp.example"),
		"https://example.com/button": "/" + strings.TrimLeft(buttonTexts[0], "/"),
	}
	for targetURL, path := range routePaths {
		if strings.TrimSpace(path) == "" {
			t.Fatalf("missing route path for %s: body=%q buttons=%+v", targetURL, bodyText, buttonTexts)
		}
	}
	for _, row := range trackerRows {
		expectedShortURL := "https://gorp.example/r/" + stringValue(row["code"])
		if stringValue(row["short_url"]) != expectedShortURL || strings.Contains(stringValue(row["short_url"]), "/w/") {
			t.Fatalf("tracker short url row = %+v", row)
		}
		if int64Value(row["count"]) != 0 {
			t.Fatalf("tracker count before click = %+v", row)
		}
	}
	handler := app.Server().Handler()
	for targetURL, path := range routePaths {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != targetURL+"?utm_campaign=WhatsApp+UTM" {
			t.Fatalf("tracked route %s response %d location=%s body=%s", path, rec.Code, rec.Header().Get("Location"), rec.Body.String())
		}
	}
	clicks, err := env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 2 {
		t.Fatalf("click rows = %+v", clickRows)
	}
	for _, row := range clickRows {
		if int64Value(row["whatsapp_message_id"]) != whatsappID {
			t.Fatalf("click row missing whatsapp message: %+v", row)
		}
	}
	trackersAfter, err := trackers.Read("count")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range trackersAfter {
		if int64Value(row["count"]) != 1 {
			t.Fatalf("tracker count row = %+v", row)
		}
	}
	messageClickRows, err := env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageClickRows) != 1 || runtimeDateValue(messageClickRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("message click rows = %+v", messageClickRows)
	}
	traceRows, err := env.Model("marketing.trace").Browse(traceID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || runtimeDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func TestRuntimeWhatsAppTemplateShortensLeadingURLBodyParameterWithTrailingText(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	utmCampaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Trailing Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Trailing WA", "body": "Open {{1}}", "status": "approved", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "https://example.com/body trailing text"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Trailing Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp Trailing",
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": templateID,
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"campaign_id": utmCampaignID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WhatsAppSent {
		t.Fatalf("whatsapp result = %+v", result)
	}
	messages, err := env.Model("whatsapp.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("id", "components")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 {
		t.Fatalf("message rows = %+v", messageRows)
	}
	whatsappID := int64Value(messageRows[0]["id"])
	trackers, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err := trackers.Read("url", "code", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	codeByURL := map[string]string{}
	for _, row := range trackerRows {
		if int64Value(row["campaign_id"]) != utmCampaignID {
			t.Fatalf("tracker row missing campaign: %+v", row)
		}
		codeByURL[stringValue(row["url"])] = stringValue(row["code"])
	}
	bodyCode := codeByURL["https://example.com/body"]
	if bodyCode == "" {
		t.Fatalf("tracker codes by url = %+v rows=%+v", codeByURL, trackerRows)
	}
	var components []map[string]any
	if err := json.Unmarshal([]byte(stringValue(messageRows[0]["components"])), &components); err != nil {
		t.Fatal(err)
	}
	bodyText := ""
	for _, component := range components {
		if component["type"] != "body" {
			continue
		}
		params, _ := component["parameters"].([]any)
		if len(params) == 0 {
			continue
		}
		param, _ := params[0].(map[string]any)
		bodyText = stringValue(param["text"])
	}
	expectedPrefix := "https://gorp.example/r/" + bodyCode + "/w/" + strconv.FormatInt(whatsappID, 10)
	if bodyText != expectedPrefix+" trailing text" {
		t.Fatalf("body parameter = %q want %q", bodyText, expectedPrefix+" trailing text")
	}

	rec := httptest.NewRecorder()
	app.Server().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, strings.TrimPrefix(expectedPrefix, "https://gorp.example"), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://example.com/body?utm_campaign=Trailing+Campaign" {
		t.Fatalf("tracked route response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func TestRuntimeWhatsAppTemplateTracksLinksFromCampaignMetadata(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	utmCampaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Metadata Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Tracked Metadata WA", "body": "Open {{1}}", "status": "approved", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "https://example.com/metadata-body"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"wa_template_id": templateID, "sequence": int64(0), "button_type": "url", "url_type": "tracked", "website_url": "https://example.com/metadata-button", "text": "Tracked"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Metadata Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp Metadata",
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": templateID,
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"campaign_id": utmCampaignID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WhatsAppSent {
		t.Fatalf("whatsapp result = %+v", result)
	}
	messages, err := env.Model("whatsapp.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("id", "body", "components")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 {
		t.Fatalf("whatsapp message rows = %+v", messageRows)
	}
	whatsappID := int64Value(messageRows[0]["id"])
	trackers, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err := trackers.Read("url", "code", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	codeByURL := map[string]string{}
	for _, row := range trackerRows {
		if int64Value(row["campaign_id"]) != utmCampaignID {
			t.Fatalf("tracker row missing campaign: %+v", row)
		}
		codeByURL[stringValue(row["url"])] = stringValue(row["code"])
	}
	bodyCode := codeByURL["https://example.com/metadata-body"]
	buttonCode := codeByURL["https://example.com/metadata-button"]
	if bodyCode == "" || buttonCode == "" {
		t.Fatalf("tracker codes by url = %+v rows=%+v", codeByURL, trackerRows)
	}
	body := stringValue(messageRows[0]["body"])
	if body != "Open https://example.com/metadata-body" {
		t.Fatalf("stored body from metadata = %s", body)
	}
	if !strings.Contains(stringValue(messageRows[0]["components"]), "https://gorp.example/r/"+bodyCode+"/w/"+strconv.FormatInt(whatsappID, 10)) {
		t.Fatalf("body component not rewritten from metadata: %s", stringValue(messageRows[0]["components"]))
	}
	if !strings.Contains(stringValue(messageRows[0]["components"]), "r/"+buttonCode+"/w/"+strconv.FormatInt(whatsappID, 10)) {
		t.Fatalf("components not rewritten from metadata: %s", stringValue(messageRows[0]["components"]))
	}
	staticTemplateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Static Body WA", "body": "Static https://example.com/static-body", "status": "approved", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	staticActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp Static",
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": staticTemplateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err = serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err = registry.Run(context.Background(), staticActionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"campaign_id": utmCampaignID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WhatsAppSent {
		t.Fatalf("static whatsapp result = %+v", result)
	}
	trackers, err = env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err = trackers.Read("url")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range trackerRows {
		if stringValue(row["url"]) == "https://example.com/static-body" {
			t.Fatalf("static body URL should not be tracked: %+v", trackerRows)
		}
	}
}

func TestRuntimeWhatsAppTemplateRequiresApprovedStatus(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "WA Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		name   string
		values map[string]any
	}{
		{"pending", map[string]any{"name": "Pending", "body": "Hello", "status": "pending", "model": "res.partner"}},
		{"red", map[string]any{"name": "Red", "body": "Hello", "status": "approved", "quality": "red", "model": "res.partner"}},
		{"mismatch", map[string]any{"name": "Mismatch", "body": "Hello", "status": "approved", "model": "res.users"}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			templateID, err := env.Model("whatsapp.template").Create(spec.values)
			if err != nil {
				t.Fatal(err)
			}
			actionID, err := env.Model("ir.actions.server").Create(map[string]any{
				"name":           spec.name,
				"model_name":     "res.partner",
				"state":          "whatsapp",
				"active":         true,
				"wa_template_id": templateID,
			})
			if err != nil {
				t.Fatal(err)
			}
			registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: partnerID})
			if err == nil {
				t.Fatalf("expected send error for %+v", spec.values)
			}
		})
	}
	messages, err := env.Model("mail.message").Search(domain.Cond("message_type", domain.Equal, "whatsapp"))
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("whatsapp mail messages = %d", messages.Len())
	}
}

func TestRuntimeWhatsAppTemplateRendersBodyAndDynamicButtonVariables(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "WA Recipient", "email": "https://example.com/u/customer-1", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Variable WA", "body": "Hello {{1}} {{2}}", "status": "approved", "model": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "field", "field_name": "name", "demo_value": "Demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{2}}", "wa_template_id": templateID, "line_type": "body", "field_type": "free_text", "demo_value": "fallback"}); err != nil {
		t.Fatal(err)
	}
	buttonID, err := env.Model("whatsapp.template.button").Create(map[string]any{"wa_template_id": templateID, "sequence": int64(0), "button_type": "url", "url_type": "dynamic", "website_url": "https://example.com/u/", "text": "Visit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "Visit", "button_id": buttonID, "wa_template_id": templateID, "line_type": "button", "field_type": "field", "field_name": "email", "demo_value": "https://example.com/u/demo"}); err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "WhatsApp Variables",
		"model_name":     "res.partner",
		"state":          "whatsapp",
		"active":         true,
		"wa_template_id": templateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"free_text_json": map[string]any{"free_text_2": "today"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WhatsAppSent {
		t.Fatalf("whatsapp result = %+v", result)
	}
	messages, err := env.Model("whatsapp.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body", "components", "free_text_json")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || stringValue(messageRows[0]["body"]) != "Hello WA Recipient today" {
		t.Fatalf("whatsapp message rows = %+v", messageRows)
	}
	if !strings.Contains(stringValue(messageRows[0]["free_text_json"]), `"free_text_2":"today"`) {
		t.Fatalf("free text json = %s", stringValue(messageRows[0]["free_text_json"]))
	}
	var components []map[string]any
	if err := json.Unmarshal([]byte(stringValue(messageRows[0]["components"])), &components); err != nil {
		t.Fatal(err)
	}
	bodyParams := []string{}
	buttonParam := ""
	for _, component := range components {
		params, _ := component["parameters"].([]any)
		for _, rawParam := range params {
			param, _ := rawParam.(map[string]any)
			if component["type"] == "body" {
				bodyParams = append(bodyParams, stringValue(param["text"]))
			}
			if component["type"] == "button" {
				buttonParam = stringValue(param["text"])
			}
		}
	}
	if !reflect.DeepEqual(bodyParams, []string{"WA Recipient", "today"}) {
		t.Fatalf("body params = %+v components=%+v", bodyParams, components)
	}
	if buttonParam != "customer-1" {
		t.Fatalf("button param = %q components=%+v", buttonParam, components)
	}
}

func TestRuntimeSMSTemplateGeneratesTrackedLinks(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "SMS UTM"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "SMS Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "SMS"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Mailing", "campaign_id": campaignID, "source_id": sourceID, "medium_id": mediumID})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("sms.template").Create(map[string]any{
		"name":  "Tracked SMS",
		"model": "res.partner",
		"body":  "Open https://example.com/sms Existing https://gorp.example/r/Keep123 Unsub https://gorp.example/sms/99/abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "SMS Recipient", "active": true, "phone": "+15550101"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "SMS",
		"model_name":      "res.partner",
		"state":           "sms",
		"active":          true,
		"sms_template_id": templateID,
		"sms_method":      "sms",
	})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{
			"mass_mailing_id":       mailingID,
			"campaign_id":           campaignID,
			"source_id":             sourceID,
			"medium_id":             mediumID,
			"sms_allow_unsubscribe": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.SMSSent {
		t.Fatalf("sms result = %+v", result)
	}

	smsRecords, err := env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsRows, err := smsRecords.Read("id", "uuid", "number", "body", "partner_id", "mail_message_id", "mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsRows) != 1 {
		t.Fatalf("sms rows = %+v", smsRows)
	}
	if !smsWebhookUUIDForRuntimeTest(stringValue(smsRows[0]["uuid"])) {
		t.Fatalf("sms uuid = %+v", smsRows[0]["uuid"])
	}
	smsID := int64Value(smsRows[0]["id"])
	body := stringValue(smsRows[0]["body"])
	if int64Value(smsRows[0]["partner_id"]) != partnerID || int64Value(smsRows[0]["mailing_id"]) != mailingID || stringValue(smsRows[0]["number"]) != "+15550101" || int64Value(smsRows[0]["mail_message_id"]) == 0 {
		t.Fatalf("sms row = %+v", smsRows[0])
	}

	trackers, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	trackerRows, err := trackers.Read("url", "code", "campaign_id", "source_id", "medium_id", "mass_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(trackerRows) != 1 {
		t.Fatalf("tracker rows = %+v", trackerRows)
	}
	trackerCode := stringValue(trackerRows[0]["code"])
	if trackerRows[0]["url"] != "https://example.com/sms" ||
		int64Value(trackerRows[0]["campaign_id"]) != campaignID ||
		int64Value(trackerRows[0]["source_id"]) != sourceID ||
		int64Value(trackerRows[0]["medium_id"]) != mediumID ||
		int64Value(trackerRows[0]["mass_mailing_id"]) != mailingID ||
		trackerCode == "" {
		t.Fatalf("tracker row = %+v", trackerRows[0])
	}
	if !strings.Contains(body, "https://gorp.example/r/"+trackerCode+"/s/"+strconv.FormatInt(smsID, 10)) ||
		!strings.Contains(body, "https://gorp.example/r/Keep123/s/"+strconv.FormatInt(smsID, 10)) ||
		!strings.Contains(body, "https://gorp.example/sms/99/abc") {
		t.Fatalf("sms body not rewritten: %s", body)
	}

	traceRecords, err := env.Model("mailing.trace").Search(domain.Cond("sms_id_int", domain.Equal, smsID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traceRecords.Read("id", "trace_type", "sms_id", "sms_id_int", "sms_number", "sms_code", "mass_mailing_id", "campaign_id", "source_id", "medium_id", "model", "res_id", "trace_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		traceRows[0]["trace_type"] != "sms" ||
		int64Value(traceRows[0]["sms_id"]) != smsID ||
		stringValue(traceRows[0]["sms_number"]) != "+15550101" ||
		int64Value(traceRows[0]["mass_mailing_id"]) != mailingID ||
		int64Value(traceRows[0]["campaign_id"]) != campaignID ||
		int64Value(traceRows[0]["source_id"]) != sourceID ||
		int64Value(traceRows[0]["medium_id"]) != mediumID ||
		traceRows[0]["model"] != "res.partner" ||
		int64Value(traceRows[0]["res_id"]) != partnerID ||
		traceRows[0]["trace_status"] != "outgoing" {
		t.Fatalf("trace rows = %+v", traceRows)
	}
	smsCode := stringValue(traceRows[0]["sms_code"])
	if smsCode == "" || !strings.Contains(body, "STOP SMS: https://gorp.example/sms/"+strconv.FormatInt(mailingID, 10)+"/"+smsCode) {
		t.Fatalf("sms unsubscribe not generated: code=%q body=%s", smsCode, body)
	}
	notificationRecords, err := env.Model("mail.notification").Search(domain.Cond("sms_id_int", domain.Equal, smsID))
	if err != nil {
		t.Fatal(err)
	}
	notificationRows, err := notificationRecords.Read("id", "mail_message_id", "res_partner_id", "notification_type", "notification_status", "sms_id", "sms_id_int", "sms_number", "is_read")
	if err != nil {
		t.Fatal(err)
	}
	if len(notificationRows) != 1 ||
		int64Value(notificationRows[0]["mail_message_id"]) != int64Value(smsRows[0]["mail_message_id"]) ||
		int64Value(notificationRows[0]["res_partner_id"]) != partnerID ||
		notificationRows[0]["notification_type"] != "sms" ||
		notificationRows[0]["notification_status"] != "ready" ||
		int64Value(notificationRows[0]["sms_id"]) != smsID ||
		int64Value(notificationRows[0]["sms_id_int"]) != smsID ||
		notificationRows[0]["sms_number"] != "+15550101" ||
		notificationRows[0]["is_read"] != true {
		t.Fatalf("notification rows = %+v", notificationRows)
	}
	notificationID := int64Value(notificationRows[0]["id"])
	smsTrackers, err := env.Model("sms.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsTrackerRows, err := smsTrackers.Read("sms_uuid", "mail_notification_id", "mailing_trace_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsTrackerRows) != 1 ||
		smsTrackerRows[0]["sms_uuid"] != smsRows[0]["uuid"] ||
		int64Value(smsTrackerRows[0]["mail_notification_id"]) != notificationID ||
		int64Value(smsTrackerRows[0]["mailing_trace_id"]) != int64Value(traceRows[0]["id"]) {
		t.Fatalf("sms tracker rows = %+v", smsTrackerRows)
	}
	messageRows, err := env.Model("mail.message").Browse(int64Value(smsRows[0]["mail_message_id"])).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || stringValue(messageRows[0]["body"]) != body {
		t.Fatalf("message rows = %+v body=%s", messageRows, body)
	}
}

func TestRuntimeSMSTemplateUsesCountryAwareE164Recipient(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	beID, err := env.Model("res.country").Create(map[string]any{"name": "Belgium", "code": "BE", "phone_code": int64(32), "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("sms.template").Create(map[string]any{
		"name":  "BE SMS",
		"model": "res.partner",
		"body":  "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "BE SMS Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "BE Recipient", "country_id": beID, "active": true, "phone": "0456 04 05 06"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "BE SMS",
		"model_name":      "res.partner",
		"state":           "sms",
		"active":          true,
		"sms_template_id": templateID,
		"sms_method":      "sms",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{"mass_mailing_id": mailingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.SMSSent {
		t.Fatalf("sms result = %+v", result)
	}

	smsRecords, err := env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsRows, err := smsRecords.Read("id", "number", "partner_id", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsRows) != 1 || stringValue(smsRows[0]["number"]) != "+32456040506" || int64Value(smsRows[0]["partner_id"]) != partnerID || int64Value(smsRows[0]["mail_message_id"]) == 0 {
		t.Fatalf("sms rows = %+v", smsRows)
	}
	smsID := int64Value(smsRows[0]["id"])

	traceRecords, err := env.Model("mailing.trace").Search(domain.Cond("sms_id_int", domain.Equal, smsID))
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := traceRecords.Read("trace_type", "sms_number", "sms_id_int", "model", "res_id", "trace_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 ||
		traceRows[0]["trace_type"] != "sms" ||
		stringValue(traceRows[0]["sms_number"]) != "+32456040506" ||
		int64Value(traceRows[0]["sms_id_int"]) != smsID ||
		traceRows[0]["model"] != "res.partner" ||
		int64Value(traceRows[0]["res_id"]) != partnerID ||
		traceRows[0]["trace_status"] != "outgoing" {
		t.Fatalf("trace rows = %+v", traceRows)
	}

	notificationRecords, err := env.Model("mail.notification").Search(domain.Cond("sms_id_int", domain.Equal, smsID))
	if err != nil {
		t.Fatal(err)
	}
	notificationRows, err := notificationRecords.Read("res_partner_id", "notification_type", "notification_status", "sms_number")
	if err != nil {
		t.Fatal(err)
	}
	if len(notificationRows) != 1 ||
		int64Value(notificationRows[0]["res_partner_id"]) != partnerID ||
		notificationRows[0]["notification_type"] != "sms" ||
		notificationRows[0]["notification_status"] != "ready" ||
		stringValue(notificationRows[0]["sms_number"]) != "+32456040506" {
		t.Fatalf("notification rows = %+v", notificationRows)
	}
}

func TestRuntimeSMSTemplateMassMailingSMSRecipientExclusions(t *testing.T) {
	t.Run("phone_blacklist", func(t *testing.T) {
		app, err := BootstrapOI("")
		if err != nil {
			t.Fatal(err)
		}
		env := app.Env
		partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Blocked", "active": true, "phone": "+15550101"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.Model("phone.blacklist").Create(map[string]any{"number": "+15550101", "active": true}); err != nil {
			t.Fatal(err)
		}
		runRuntimeMassSMSAction(t, env, "res.partner", []int64{partnerID}, map[string]any{"use_exclusion_list": true})
		assertRuntimeSMSCanceledOnly(t, env, partnerID, "+15550101", "sms_blacklist")
	})

	t.Run("mailing_list_opt_out", func(t *testing.T) {
		app, err := BootstrapOI("")
		if err != nil {
			t.Fatal(err)
		}
		env := app.Env
		listID, err := env.Model("mailing.list").Create(map[string]any{"name": "SMS List", "active": true})
		if err != nil {
			t.Fatal(err)
		}
		contactID, err := env.Model("mailing.contact").Create(map[string]any{"name": "Opted", "active": true, "phone": "+32456040506"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID, "opt_out": true}); err != nil {
			t.Fatal(err)
		}
		runRuntimeMassSMSAction(t, env, "mailing.contact", []int64{contactID}, map[string]any{"contact_list_ids": []int64{listID}})
		assertRuntimeSMSCanceledOnly(t, env, contactID, "+32456040506", "sms_optout")
	})

	t.Run("missing_number", func(t *testing.T) {
		app, err := BootstrapOI("")
		if err != nil {
			t.Fatal(err)
		}
		env := app.Env
		partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Missing", "active": true})
		if err != nil {
			t.Fatal(err)
		}
		runRuntimeMassSMSAction(t, env, "res.partner", []int64{partnerID}, nil)
		assertRuntimeSMSCanceledOnly(t, env, partnerID, "", "sms_number_missing")
	})

	t.Run("invalid_local_number", func(t *testing.T) {
		app, err := BootstrapOI("")
		if err != nil {
			t.Fatal(err)
		}
		env := app.Env
		beID, err := env.Model("res.country").Create(map[string]any{"name": "Belgium", "code": "BE", "phone_code": int64(32), "active": true})
		if err != nil {
			t.Fatal(err)
		}
		partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Invalid", "country_id": beID, "active": true, "phone": "87645"})
		if err != nil {
			t.Fatal(err)
		}
		runRuntimeMassSMSAction(t, env, "res.partner", []int64{partnerID}, nil)
		assertRuntimeSMSCanceledOnly(t, env, partnerID, "87645", "sms_number_format")
	})
}

func TestRuntimeSMSTemplateMassMailingSMSDuplicateCreatesCanceledTraceOnly(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "First", "active": true, "phone": "+15550101"})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Second", "active": true, "phone": "+1 555 0101"})
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeMassSMSAction(t, env, "res.partner", []int64{firstID, secondID}, nil)

	smsRows := runtimeRows(t, env, "sms.sms", "id", "number", "partner_id", "state", "failure_type")
	if len(smsRows) != 1 || stringValue(smsRows[0]["number"]) != "+15550101" || int64Value(smsRows[0]["partner_id"]) != firstID || smsRows[0]["state"] != "outgoing" || stringValue(smsRows[0]["failure_type"]) != "" {
		t.Fatalf("sms rows = %+v", smsRows)
	}
	smsID := int64Value(smsRows[0]["id"])
	if rows := runtimeRows(t, env, "mail.notification", "sms_id_int"); len(rows) != 1 || int64Value(rows[0]["sms_id_int"]) != smsID {
		t.Fatalf("notification rows = %+v", rows)
	}
	if rows := runtimeRows(t, env, "sms.tracker", "mail_notification_id", "mailing_trace_id"); len(rows) != 1 || int64Value(rows[0]["mail_notification_id"]) == 0 || int64Value(rows[0]["mailing_trace_id"]) == 0 {
		t.Fatalf("tracker rows = %+v", rows)
	}
	if rows := runtimeRows(t, env, "mail.message", "message_type"); len(rows) != 1 || rows[0]["message_type"] != "sms" {
		t.Fatalf("message rows = %+v", rows)
	}

	traceRows := runtimeRows(t, env, "mailing.trace", "res_id", "sms_id_int", "sms_number", "trace_status", "failure_type")
	if len(traceRows) != 2 {
		t.Fatalf("trace rows = %+v", traceRows)
	}
	seenOutgoing := false
	seenCanceled := false
	for _, row := range traceRows {
		switch row["trace_status"] {
		case "outgoing":
			seenOutgoing = int64Value(row["res_id"]) == firstID && int64Value(row["sms_id_int"]) == smsID && stringValue(row["failure_type"]) == ""
		case "cancel":
			seenCanceled = int64Value(row["res_id"]) == secondID && int64Value(row["sms_id_int"]) == 0 && row["sms_number"] == "+15550101" && row["failure_type"] == "sms_duplicate"
		}
	}
	if !seenOutgoing || !seenCanceled {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func runRuntimeMassSMSAction(t *testing.T, env *record.Env, modelName string, recordIDs []int64, mailingValues map[string]any) {
	t.Helper()
	templateID, err := env.Model("sms.template").Create(map[string]any{"name": "Mass SMS", "model": modelName, "body": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	mailing := map[string]any{"name": "Runtime SMS", "mailing_model_real": modelName}
	for key, value := range mailingValues {
		mailing[key] = value
	}
	mailingID, err := env.Model("mailing.mailing").Create(mailing)
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Mass SMS",
		"model_name":      modelName,
		"state":           "sms",
		"active":          true,
		"sms_template_id": templateID,
		"sms_method":      "sms",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:     modelName,
		RecordIDs: recordIDs,
		Metadata:  map[string]any{"mass_mailing_id": mailingID},
	}); err != nil || !result.SMSSent {
		t.Fatalf("sms action result=%+v err=%v", result, err)
	}
}

func assertRuntimeSMSCanceledOnly(t *testing.T, env *record.Env, recordID int64, number string, failureType string) {
	t.Helper()
	if rows := runtimeRows(t, env, "sms.sms", "id"); len(rows) != 0 {
		t.Fatalf("sms rows = %+v", rows)
	}
	if rows := runtimeRows(t, env, "mail.notification", "id"); len(rows) != 0 {
		t.Fatalf("notification rows = %+v", rows)
	}
	if rows := runtimeRows(t, env, "sms.tracker", "id"); len(rows) != 0 {
		t.Fatalf("tracker rows = %+v", rows)
	}
	if rows := runtimeRows(t, env, "mail.message", "id"); len(rows) != 0 {
		t.Fatalf("message rows = %+v", rows)
	}
	traceRows := runtimeRows(t, env, "mailing.trace", "trace_type", "res_id", "sms_id_int", "sms_number", "trace_status", "failure_type")
	if len(traceRows) != 1 ||
		traceRows[0]["trace_type"] != "sms" ||
		int64Value(traceRows[0]["res_id"]) != recordID ||
		int64Value(traceRows[0]["sms_id_int"]) != 0 ||
		stringValue(traceRows[0]["sms_number"]) != number ||
		traceRows[0]["trace_status"] != "cancel" ||
		traceRows[0]["failure_type"] != failureType {
		t.Fatalf("trace rows = %+v", traceRows)
	}
}

func runtimeRows(t *testing.T, env *record.Env, modelName string, fields ...string) []map[string]any {
	t.Helper()
	found, err := env.Model(modelName).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read(fields...)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func smsWebhookUUIDForRuntimeTest(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' {
			continue
		}
		return false
	}
	return true
}

func TestRuntimeSMSTemplateSkipsUnsubscribeLinkWhenDisabled(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	env := app.Env
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	hooks := envActionHooks{env: env}
	body := "Ping https://example.com"

	for name, metadata := range map[string]map[string]any{
		"missing flag": {"mass_mailing_id": int64(99)},
		"false flag":   {"mass_mailing_id": int64(99), "sms_allow_unsubscribe": false},
		"no mailing":   {"sms_allow_unsubscribe": true},
	} {
		if got := hooks.appendSMSUnsubscribeInfo(body, "Stop1", metadata); got != body {
			t.Fatalf("%s appendSMSUnsubscribeInfo = %q", name, got)
		}
	}
	if got := hooks.appendSMSUnsubscribeInfo(body, "Stop1", map[string]any{"mass_mailing_id": int64(99), "sms_allow_unsubscribe": true}); !strings.Contains(got, "STOP SMS: https://gorp.example/sms/99/Stop1") {
		t.Fatalf("enabled appendSMSUnsubscribeInfo = %q", got)
	}

	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{"name": "Disabled SMS"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("sms.template").Create(map[string]any{"name": "Disabled SMS", "model": "res.partner", "body": "Plain https://example.com/disabled"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "SMS No Opt", "active": true, "phone": "+15550404"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "SMS No Opt",
		"model_name":      "res.partner",
		"state":           "sms",
		"active":          true,
		"sms_template_id": templateID,
		"sms_method":      "sms",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
		Metadata: map[string]any{
			"mass_mailing_id":       mailingID,
			"sms_allow_unsubscribe": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.SMSSent {
		t.Fatalf("sms result = %+v", result)
	}
	smsRecords, err := env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsRows, err := smsRecords.Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsRows) != 1 || strings.Contains(stringValue(smsRows[0]["body"]), "STOP SMS:") {
		t.Fatalf("disabled sms body rows = %+v", smsRows)
	}
}

func TestEnvActionHooksSendWorkflowEmailUsesTemplateBatch(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "email": "ada@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable Approver", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	delegatorEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Ada", "work_email": "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	delegateEmployeeID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegate", "work_email": "delegate@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	delegationID, err := env.Model(oi_delegation.ModelDelegation).Create(map[string]any{
		"name":        "DEL/CC",
		"date_from":   "2020-01-01",
		"date_to":     "2099-12-31",
		"employee_id": delegatorEmployeeID,
		"state":       "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(oi_delegation.ModelDelegationLine).Create(map[string]any{
		"delegation_id": delegationID,
		"group_id":      groupID,
		"employee_id":   delegateEmployeeID,
		"active":        true,
	}); err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":                 "Approval Template",
		"model":                "res.partner",
		"subject":              "Approval {{ object.name }}",
		"body_html":            "<p>{{ name }}</p>",
		"email_to":             "{{ email }}",
		"email_cc":             "audit@example.com",
		"delegation_group_ids": []int64{groupID},
		"scheduled_date":       "2026-07-02 09:30:00",
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := (envActionHooks{env: env}).SendWorkflowEmail(context.Background(), internalworkflow.EmailRequest{
		TemplateID: templateID,
		Model:      "res.partner",
		RecordID:   recordID,
	}); err != nil {
		t.Fatal(err)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	mailRows, err := mails.Read("id", "mail_message_id", "email_to", "email_cc", "subject", "body_html", "scheduled_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	scheduledDate, ok := mailRows[0]["scheduled_date"].(time.Time)
	if mailRows[0]["email_to"] != "ada@example.com" ||
		mailRows[0]["email_cc"] != "audit@example.com, delegate@example.com" ||
		mailRows[0]["subject"] != "Approval Ada" ||
		mailRows[0]["body_html"] != "<p>Ada</p>" ||
		!ok ||
		!scheduledDate.Equal(time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	messageID := mailRows[0]["mail_message_id"].(int64)
	messageRows, err := env.Model("mail.message").Browse(messageID).Read("message_type", "model", "res_id", "body_is_html")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["message_type"] != "email" || messageRows[0]["model"] != "res.partner" || messageRows[0]["res_id"] != recordID || messageRows[0]["body_is_html"] != true {
		t.Fatalf("message rows = %+v", messageRows)
	}
	scheduled, err := env.Model("mail.scheduled.message").Search(domain.Cond("mail_mail_id", "=", int64Value(mailRows[0]["id"])))
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.Len() != 1 {
		t.Fatalf("scheduled messages = %d", scheduled.Len())
	}
}

func TestEnvActionHooksSendMailPersistsQueueFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Queue Target", "email": "target@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := env.Model("ir.attachment").Create(map[string]any{"name": "brief.txt", "type": "binary", "mimetype": "text/plain", "datas": "YnJpZWY="})
	if err != nil {
		t.Fatal(err)
	}
	serverID, err := env.Model("ir.mail_server").Create(map[string]any{"name": "Outbound", "smtp_host": "127.0.0.1", "smtp_port": int64(2525), "smtp_encryption": "none"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":           "Queued Template",
		"model":          "res.partner",
		"subject":        "Queued subject",
		"body_html":      "<p>Queued</p>",
		"email_from":     "Sender <sender@example.com>",
		"email_to":       "target@example.com",
		"email_cc":       "copy@example.com",
		"reply_to":       "reply@example.com",
		"attachment_ids": []int64{attachmentID},
		"mail_server_id": serverID,
		"auto_delete":    true,
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := (envActionHooks{env: env}).SendMail(context.Background(), serveractions.MailRequest{
		TemplateID: templateID,
		Model:      "res.partner",
		RecordIDs:  []int64{recordID},
		Values:     map[string]any{"headers": `{"X-Action":"ok"}`},
		Metadata:   map[string]any{"mail_post_method": "email"},
	}); err != nil {
		t.Fatal(err)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := mails.Read("email_from", "email_to", "email_cc", "reply_to", "subject", "body_html", "attachment_ids", "mail_server_id", "auto_delete", "headers")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["email_from"] != "Sender <sender@example.com>" ||
		rows[0]["email_to"] != "target@example.com" ||
		rows[0]["email_cc"] != "copy@example.com" ||
		rows[0]["reply_to"] != "reply@example.com" ||
		rows[0]["subject"] != "Queued subject" ||
		rows[0]["body_html"] != "<p>Queued</p>" ||
		int64Value(rows[0]["mail_server_id"]) != serverID ||
		rows[0]["auto_delete"] != true ||
		rows[0]["headers"] != `{"X-Action":"ok"}` {
		t.Fatalf("mail row = %+v", rows)
	}
	if ids := int64Slice(rows[0]["attachment_ids"]); len(ids) != 1 || ids[0] != attachmentID {
		t.Fatalf("attachment ids = %+v", rows[0]["attachment_ids"])
	}
}

func TestServerActionsFromEnvRunsObjectWriteValueFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                 "Deactivate Partner",
		"model_id":             modelID,
		"model_name":           "res.partner",
		"state":                "object_write",
		"active":               true,
		"update_path":          "active",
		"update_field_type":    "boolean",
		"update_boolean_value": "false",
		"evaluation_type":      "value",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || action.Kind != serveractions.KindWrite || action.UpdatePath != "active" || action.UpdateBooleanValue != "false" {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(recordID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != false {
		t.Fatalf("partner = %+v", rows[0])
	}
}

func TestServerActionsFromEnvLoadsInactiveActionsAsDisabled(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":           "Inactive Action",
		"model_id":       modelID,
		"model_name":     "res.partner",
		"state":          "code",
		"active":         false,
		"go_action_name": "noop",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || action.Active || !action.Disabled {
		t.Fatalf("inactive action = %+v ok=%v", action, ok)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner"}); !errors.Is(err, serveractions.ErrActionDisabled) {
		t.Fatalf("inactive action run error = %v", err)
	}
}

func TestServerActionsFromEnvRunsObjectCreateValueFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Create Partner",
		"model_id":        modelID,
		"model_name":      "res.partner",
		"state":           "object_create",
		"active":          true,
		"crud_model_id":   modelID,
		"crud_model_name": "res.partner",
		"value":           "Created Partner",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(result.CreatedID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Created Partner" {
		t.Fatalf("created partner = %+v result=%+v", rows, result)
	}
}

func TestServerActionsFromEnvRunsObjectWriteSequenceNextByID(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Partner Seq",
		"code":             "res.partner.seq",
		"prefix":           "P/",
		"suffix":           "/X",
		"padding":          int64(4),
		"number_next":      int64(7),
		"number_increment": int64(3),
		"active":           true,
		"implementation":   "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Assign Sequence",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "name",
		"update_field_type": "char",
		"evaluation_type":   "sequence",
		"sequence_id":       sequenceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(recordID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "P/0010/X" {
		t.Fatalf("partner name = %+v", rows[0])
	}
	sequenceRows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRows[0]["number_next"] != int64(7) {
		t.Fatalf("sequence rows = %+v", sequenceRows)
	}
}

func TestEnvActionHooksNextSequenceUsesDateRanges(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Invoice Seq",
		"code":             "account.move",
		"prefix":           "INV/%(year)s/%(range_year)s/",
		"padding":          int64(3),
		"number_next":      int64(99),
		"number_increment": int64(5),
		"use_date_range":   true,
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.Context().Values["ir_sequence_date"] = "2026-04-15"
	hooks := envActionHooks{env: env}
	first, err := hooks.NextSequence(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hooks.NextSequence(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if first != "INV/2026/2026/001" || second != "INV/2026/2026/006" {
		t.Fatalf("sequence values = %q %q", first, second)
	}
	rangeRows, err := env.Model("ir.sequence.date_range").Search(domain.Cond("sequence_id", domain.Equal, sequenceID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := rangeRows.Read("date_from", "date_to", "number_next")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["date_from"] != "2026-01-01" || rows[0]["date_to"] != "2026-12-31" || rows[0]["number_next"] != int64(1) {
		t.Fatalf("date range rows = %+v", rows)
	}
	sequenceRows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRows[0]["number_next"] != int64(99) {
		t.Fatalf("parent sequence rows = %+v", sequenceRows)
	}
	env.Context().Values["ir_sequence_date"] = "2027-02-01"
	nextYear, err := hooks.NextSequence(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if nextYear != "INV/2027/2027/001" {
		t.Fatalf("next year sequence = %q", nextYear)
	}
}

func TestEnvActionHooksNextSequenceUsesExistingCustomDateRange(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Quarter Seq",
		"code":             "quarter.seq",
		"prefix":           "Q/%(year)s/%(range_month)s/",
		"padding":          int64(2),
		"number_increment": int64(2),
		"use_date_range":   true,
		"active":           true,
		"implementation":   "no_gap",
	})
	if err != nil {
		t.Fatal(err)
	}
	rangeID, err := env.Model("ir.sequence.date_range").Create(map[string]any{
		"date_from":   "2026-04-01",
		"date_to":     "2026-06-30",
		"sequence_id": sequenceID,
		"number_next": int64(42),
	})
	if err != nil {
		t.Fatal(err)
	}
	env.Context().Values["ir_sequence_date"] = "2026-05-10"
	value, err := (envActionHooks{env: env}).NextSequence(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Q/2026/04/42" {
		t.Fatalf("sequence value = %q", value)
	}
	rows, err := env.Model("ir.sequence.date_range").Browse(rangeID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["number_next"] != int64(44) {
		t.Fatalf("date range rows = %+v", rows)
	}
}

func TestEnvActionHooksNextSequenceInterpolatesOdooDateTokens(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Token Seq",
		"code":             "token.seq",
		"prefix":           "%(isoyear)s/%(isoy)s/%(isoweek)s/%(woy)s/%(weekday)s/%(range_year)s/%(range_month)s/",
		"padding":          int64(2),
		"number_increment": int64(1),
		"use_date_range":   true,
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.Context().Values["ir_sequence_date"] = "2021-01-03"
	value, err := (envActionHooks{env: env}).NextSequence(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if value != "2020/20/53/00/0/2021/01/01" {
		t.Fatalf("sequence value = %q", value)
	}
}

func TestEnvActionHooksNextSequenceRejectsInvalidInterpolationToken(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Bad Seq",
		"code":             "bad.seq",
		"prefix":           "%(missing)s/",
		"padding":          int64(2),
		"number_next":      int64(7),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (envActionHooks{env: env}).NextSequence(context.Background(), sequenceID); err == nil || !strings.Contains(err.Error(), "invalid prefix or suffix") {
		t.Fatalf("expected invalid interpolation error, got %v", err)
	}
	rows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["number_next"] != int64(7) {
		t.Fatalf("sequence rows = %+v", rows)
	}
}

func TestServerActionsFromEnvRunsObjectCopyAndLinksActiveRecord(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	companyModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.company", "name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	linkFieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.company", "name": "partner_id", "ttype": "many2one", "relation": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true, "email": "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "ACME", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Copy Partner",
		"model_id":        companyModelID,
		"model_name":      "res.company",
		"state":           "object_copy",
		"active":          true,
		"crud_model_id":   partnerModelID,
		"crud_model_name": "res.partner",
		"resource_ref":    "res.partner," + strconv.FormatInt(sourceID, 10),
		"link_field_id":   linkFieldID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.company", RecordID: companyID})
	if err != nil {
		t.Fatal(err)
	}
	companyRows, err := env.Model("res.company").Browse(companyID).Read("partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if companyRows[0]["partner_id"] != result.CreatedID {
		t.Fatalf("company rows = %+v result=%+v", companyRows, result)
	}
	partnerRows, err := env.Model("res.partner").Browse(result.CreatedID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["name"] != "Ada (copy)" || partnerRows[0]["email"] != "ada@example.com" {
		t.Fatalf("copied partner = %+v", partnerRows)
	}
}

func TestServerActionsFromEnvRunsObjectCreateAndLinksActiveRecord(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	companyModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.company", "name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	linkFieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.company", "name": "partner_id", "ttype": "many2one", "relation": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "ACME", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Create Partner",
		"model_id":        companyModelID,
		"model_name":      "res.company",
		"state":           "object_create",
		"active":          true,
		"crud_model_id":   partnerModelID,
		"crud_model_name": "res.partner",
		"value":           "Created Contact",
		"link_field_id":   linkFieldID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.company", RecordID: companyID})
	if err != nil {
		t.Fatal(err)
	}
	companyRows, err := env.Model("res.company").Browse(companyID).Read("partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if companyRows[0]["partner_id"] != result.CreatedID {
		t.Fatalf("company rows = %+v result=%+v", companyRows, result)
	}
}

func TestServerActionsFromEnvRunsObjectWriteDottedUpdatePath(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	oldCountryID, err := env.Model("res.country").Create(map[string]any{"name": "Old", "code": "OO", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	newCountryID, err := env.Model("res.country").Create(map[string]any{"name": "New", "code": "NN", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "ACME", "active": true, "country_id": oldCountryID})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true, "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Set Company Country",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "company_id.country_id",
		"update_field_type": "many2one",
		"evaluation_type":   "value",
		"value":             strconv.FormatInt(newCountryID, 10),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: partnerID}); err != nil {
		t.Fatal(err)
	}
	companyRows, err := env.Model("res.company").Browse(companyID).Read("country_id")
	if err != nil {
		t.Fatal(err)
	}
	if companyRows[0]["country_id"] != newCountryID {
		t.Fatalf("company rows = %+v", companyRows)
	}
}

func TestServerActionsFromEnvRunsX2ManyObjectWriteOperations(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.users", "name": "Users"})
	if err != nil {
		t.Fatal(err)
	}
	group1, err := env.Model("res.groups").Create(map[string]any{"name": "One"})
	if err != nil {
		t.Fatal(err)
	}
	group2, err := env.Model("res.groups").Create(map[string]any{"name": "Two"})
	if err != nil {
		t.Fatal(err)
	}
	group3, err := env.Model("res.groups").Create(map[string]any{"name": "Three"})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{"login": "ada", "name": "Ada", "active": true, "groups_id": []int64{group1}})
	if err != nil {
		t.Fatal(err)
	}
	registryAction := func(name string, operation string, value int64) int64 {
		t.Helper()
		actionID, err := env.Model("ir.actions.server").Create(map[string]any{
			"name":                 name,
			"model_id":             modelID,
			"model_name":           "res.users",
			"state":                "object_write",
			"active":               true,
			"update_path":          "groups_id",
			"update_field_type":    "many2many",
			"update_m2m_operation": operation,
			"evaluation_type":      "value",
			"value":                strconv.FormatInt(value, 10),
		})
		if err != nil {
			t.Fatal(err)
		}
		return actionID
	}
	addID := registryAction("Add Group", "add", group2)
	removeID := registryAction("Remove Group", "remove", group1)
	setID := registryAction("Set Group", "set", group3)
	clearID := registryAction("Clear Group", "clear", 0)
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	run := func(actionID int64, want []int64) {
		t.Helper()
		if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.users", RecordID: userID}); err != nil {
			t.Fatal(err)
		}
		rows, err := env.Model("res.users").Browse(userID).Read("groups_id")
		if err != nil {
			t.Fatal(err)
		}
		got := rows[0]["groups_id"]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("groups after action %d = %+v want %+v", actionID, got, want)
		}
	}
	run(addID, []int64{group1, group2})
	run(removeID, []int64{group2})
	run(setID, []int64{group3})
	run(clearID, []int64{})
}

func TestServerActionsFromEnvRunsObjectWriteEquationPerRecord(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("res.partner").Create(map[string]any{"name": "Grace", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Set City",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "city",
		"update_field_type": "char",
		"evaluation_type":   "equation",
		"value":             "'P' + record.id",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordIDs: []int64{firstID, secondID}}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(firstID, secondID).Read("city")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["city"] != "P"+strconv.FormatInt(firstID, 10) || rows[1]["city"] != "P"+strconv.FormatInt(secondID, 10) {
		t.Fatalf("partner rows = %+v", rows)
	}
}

func TestServerActionsFromEnvBlocksWarnings(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Partner Seq",
		"code":             "res.partner.seq",
		"prefix":           "P/",
		"padding":          int64(4),
		"number_next":      int64(7),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Bad Sequence",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "active",
		"update_field_type": "boolean",
		"evaluation_type":   "sequence",
		"sequence_id":       sequenceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || action.Warning != "A sequence must only be used with character fields." {
		t.Fatalf("action warning = %+v ok=%v", action, ok)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID})
	if !errors.Is(err, serveractions.ErrActionWarning) || result.DisabledReason != "A sequence must only be used with character fields." {
		t.Fatalf("error = %v result=%+v", err, result)
	}
	rows, err := env.Model("res.partner").Browse(recordID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true {
		t.Fatalf("partner rows = %+v", rows)
	}
	sequenceRows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if sequenceRows[0]["number_next"] != int64(7) {
		t.Fatalf("sequence rows = %+v", sequenceRows)
	}
}

func TestServerActionsFromEnvBlocksStoredWarning(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Warned",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "active",
		"update_field_type": "boolean",
		"evaluation_type":   "sequence",
		"warning":           "manual warning",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: recordID})
	if !errors.Is(err, serveractions.ErrActionWarning) || result.DisabledReason != "manual warning" {
		t.Fatalf("error = %v result=%+v", err, result)
	}
	rows, err := env.Model("res.partner").Browse(recordID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Ada" {
		t.Fatalf("partner rows = %+v", rows)
	}
}

func TestServerActionsFromEnvWarningsUseUpdateFieldMetadata(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	fieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.partner", "name": "active", "ttype": "boolean"})
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Partner Seq",
		"code":             "res.partner.seq",
		"padding":          int64(4),
		"number_next":      int64(7),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Bad Sequence",
		"model_id":        modelID,
		"model_name":      "res.partner",
		"state":           "object_write",
		"active":          true,
		"update_path":     "active",
		"update_field_id": fieldID,
		"evaluation_type": "sequence",
		"sequence_id":     sequenceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || action.UpdateFieldType != "boolean" || action.Warning != "A sequence must only be used with character fields." {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
}

func TestServerActionsFromEnvWarnsMultiChildModelMismatch(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	userModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.users", "name": "Users"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Users Child",
		"model_id":   userModelID,
		"model_name": "res.users",
		"state":      "object_write",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   partnerModelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"child_ids":  []int64{childID},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(parentID)
	if !ok || !strings.Contains(action.Warning, "same model") || !strings.Contains(action.Warning, "Users Child") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
	if _, err := registry.Run(context.Background(), parentID, serveractions.ExecutionContext{Model: "res.partner", RecordID: 1}); !errors.Is(err, serveractions.ErrActionWarning) {
		t.Fatalf("error = %v", err)
	}
}

func TestServerActionsFromEnvWarnsMultiChildGroupMismatch(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	firstGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "First"})
	if err != nil {
		t.Fatal(err)
	}
	secondGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Second"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Group Child",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "object_write",
		"active":     true,
		"group_ids":  []int64{secondGroupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"group_ids":  []int64{firstGroupID},
		"child_ids":  []int64{childID},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(parentID)
	if !ok || !strings.Contains(action.Warning, "same groups") || !strings.Contains(action.Warning, "Group Child") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
}

func TestServerActionsFromEnvPropagatesChildWarnings(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Bad Child",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "active",
		"update_field_type": "boolean",
		"evaluation_type":   "sequence",
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"child_ids":  []int64{childID},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(parentID)
	if !ok || !strings.Contains(action.Warning, "have warnings") || !strings.Contains(action.Warning, "Bad Child") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
}

func TestServerActionsFromEnvWarnsWebhookRestrictedFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	fieldID, err := env.Model("ir.model.fields").Create(map[string]any{
		"model":  "res.partner",
		"name":   "email",
		"ttype":  "char",
		"groups": "base.group_system",
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Webhook",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "webhook",
		"active":            true,
		"webhook_url":       "https://example.test",
		"webhook_field_ids": []int64{fieldID},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || !strings.Contains(action.Warning, "Group-restricted fields") || !strings.Contains(action.Warning, "email") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: 1}); !errors.Is(err, serveractions.ErrActionWarning) {
		t.Fatalf("error = %v", err)
	}
}

func TestServerActionsFromEnvWarnsMailStateModelParity(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	userModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.users", "name": "Users"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("mail.template").Create(map[string]any{
		"name":     "Users Template",
		"model":    "res.users",
		"model_id": userModelID,
		"subject":  "Users",
		"active":   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                         "Bad Mail",
		"model_id":                     partnerModelID,
		"model_name":                   "res.partner",
		"state":                        "mail_post",
		"active":                       true,
		"template_id":                  templateID,
		"mail_post_method":             "comment",
		"activity_date_deadline_range": int64(-1),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	expected := strings.Join([]string{
		"The 'Due Date In' value can't be negative.",
		"Mail template model of $(action_name)s does not match action model.",
		"This action can only be done on a mail thread models",
	}, "\n\n")
	if !ok || action.Warning != expected {
		t.Fatalf("action warning = %q ok=%v", action.Warning, ok)
	}
}

func TestServerActionsFromEnvWarnsMailStatesOnTransientAndNonActivityModels(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.config.settings", "name": "Settings", "transient": true, "is_mail_thread": true})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Bad Activity",
		"model_id":   modelID,
		"model_name": "res.config.settings",
		"state":      "next_activity",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	expected := strings.Join([]string{
		"This action cannot be done on transient models.",
		"A next activity can only be planned on models that use activities.",
	}, "\n\n")
	if !ok || action.Warning != expected {
		t.Fatalf("action warning = %q ok=%v", action.Warning, ok)
	}
}

func TestServerActionsFromEnvWarnsGenericMailFieldChains(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner", "is_mail_thread": true, "is_mail_activity": true})
	if err != nil {
		t.Fatal(err)
	}
	companyFieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.partner", "name": "company_id", "ttype": "many2one", "relation": "res.company"})
	if err != nil || companyFieldID == 0 {
		t.Fatalf("company field = %d err=%v", companyFieldID, err)
	}
	userFieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.partner", "name": "user_id", "ttype": "many2one", "relation": "res.users"})
	if err != nil || userFieldID == 0 {
		t.Fatalf("user field = %d err=%v", userFieldID, err)
	}
	partnerFieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.users", "name": "partner_id", "ttype": "many2one", "relation": "res.partner"})
	if err != nil || partnerFieldID == 0 {
		t.Fatalf("partner field = %d err=%v", partnerFieldID, err)
	}
	followerActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                         "Bad Dynamic Follower",
		"model_id":                     modelID,
		"model_name":                   "res.partner",
		"state":                        "followers",
		"active":                       true,
		"followers_type":               "generic",
		"followers_partner_field_name": "company_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	activityActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":                     "Bad Dynamic Activity",
		"model_id":                 modelID,
		"model_name":               "res.partner",
		"state":                    "next_activity",
		"active":                   true,
		"activity_user_type":       "generic",
		"activity_user_field_name": "user_id.partner_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(followerActionID)
	if !ok || action.Warning != "The field 'company_id' is not a partner field." {
		t.Fatalf("follower action warning = %q ok=%v", action.Warning, ok)
	}
	action, ok = registry.Get(activityActionID)
	if !ok || action.Warning != "The field 'user_id.partner_id' is not a user field." {
		t.Fatalf("activity action warning = %q ok=%v", action.Warning, ok)
	}
}

func TestServerActionsFromEnvWarnsJSONUpdatePath(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	fieldID, err := env.Model("ir.model.fields").Create(map[string]any{"model": "res.partner", "name": "payload", "ttype": "json"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":            "Update JSON",
		"model_id":        modelID,
		"model_name":      "res.partner",
		"state":           "object_write",
		"active":          true,
		"update_path":     "payload.name",
		"update_field_id": fieldID,
		"evaluation_type": "value",
		"value":           "Ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || !strings.Contains(action.Warning, "JSON fields") || !strings.Contains(action.Warning, "payload") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
}

func TestServerActionsFromEnvWarnsAutomationModelMismatch(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	userModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.users", "name": "Users"})
	if err != nil {
		t.Fatal(err)
	}
	automationID, err := env.Model("base.automation").Create(map[string]any{
		"name":       "Partner Automation",
		"model_id":   partnerModelID,
		"model_name": "res.partner",
		"trigger":    "on_create",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":               "Users Action",
		"model_id":           userModelID,
		"model_name":         "res.users",
		"state":              "object_write",
		"active":             true,
		"base_automation_id": automationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	action, ok := registry.Get(actionID)
	if !ok || !strings.Contains(action.Warning, "should match") || !strings.Contains(action.Warning, "Partner Automation") {
		t.Fatalf("action = %+v ok=%v", action, ok)
	}
}

func TestServerActionsFromEnvFiltersAutomationActionsFromMultiChildren(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	automationID, err := env.Model("base.automation").Create(map[string]any{
		"name":       "Partner Automation",
		"model_id":   modelID,
		"model_name": "res.partner",
		"trigger":    "on_create",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	automationActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":               "Automation Child",
		"model_id":           modelID,
		"model_name":         "res.partner",
		"state":              "object_write",
		"active":             true,
		"base_automation_id": automationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	normalActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Normal Child",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "object_write",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"child_ids":  []int64{automationActionID, normalActionID},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	parent, ok := registry.Get(parentID)
	if !ok || !reflect.DeepEqual(parent.ChildIDs, []int64{normalActionID}) {
		t.Fatalf("parent = %+v ok=%v", parent, ok)
	}
}

func TestServerActionsFromEnvHydratesChildIDsFromParentIDAndInheritsParentModelGroups(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	partnerModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	userModelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.users", "name": "Users"})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   partnerModelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"group_ids":  []int64{groupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Child",
		"model_id":   userModelID,
		"model_name": "res.users",
		"state":      "object_write",
		"active":     true,
		"parent_id":  parentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	childRows, err := env.Model("ir.actions.server").Browse(childID).Read("model_id", "model_name", "group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(childRows) != 1 || childRows[0]["model_id"] != partnerModelID || childRows[0]["model_name"] != "res.partner" || !reflect.DeepEqual(childRows[0]["group_ids"], []int64{groupID}) {
		t.Fatalf("stored child rows = %+v", childRows)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	parent, ok := registry.Get(parentID)
	if !ok || !reflect.DeepEqual(parent.ChildIDs, []int64{childID}) || parent.Warning != "" {
		t.Fatalf("parent = %+v ok=%v", parent, ok)
	}
	child, ok := registry.Get(childID)
	if !ok || child.Model != "res.partner" || !reflect.DeepEqual(child.GroupIDs, []int64{groupID}) {
		t.Fatalf("child = %+v ok=%v", child, ok)
	}
}

func TestServerActionStorageRejectsRecursiveChildren(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "First",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Second",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"parent_id":  firstID,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = env.Model("ir.actions.server").Browse(firstID).Write(map[string]any{"parent_id": secondID})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "recursion") {
		t.Fatalf("expected recursion error, got %v", err)
	}
	rows, err := env.Model("ir.actions.server").Browse(firstID).Read("parent_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["parent_id"] != nil {
		t.Fatalf("parent write was not restored: %+v", rows)
	}
}

func TestServerActionStorageRejectsWarnedChildren(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Warned Child",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "object_write",
		"active":     true,
		"warning":    "manual warning",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
		"child_ids":  []int64{childID},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "warnings") {
		t.Fatalf("expected warning child error, got %v", err)
	}
}

func TestServerActionStorageRejectsWarnedChildWriteAndRestores(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Warned Child",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "object_write",
		"active":     true,
		"warning":    "manual warning",
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":       "Parent",
		"model_id":   modelID,
		"model_name": "res.partner",
		"state":      "multi",
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = env.Model("ir.actions.server").Browse(parentID).Write(map[string]any{"child_ids": []int64{childID}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "warnings") {
		t.Fatalf("expected warning child write error, got %v", err)
	}
	rows, err := env.Model("ir.actions.server").Browse(parentID).Read("child_ids")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["child_ids"] != nil {
		t.Fatalf("child write was not restored: %+v", rows)
	}
}

func TestServerActionsFromEnvEvaluatesEquationRelationFields(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "ACME", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Ada", "active": true, "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Set City",
		"model_id":          modelID,
		"model_name":        "res.partner",
		"state":             "object_write",
		"active":            true,
		"update_path":       "city",
		"update_field_type": "char",
		"evaluation_type":   "equation",
		"value":             "record.company_id.name + ' / ' + record.name",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "res.partner", RecordID: partnerID}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("res.partner").Browse(partnerID).Read("city")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["city"] != "ACME / Ada" {
		t.Fatalf("partner rows = %+v", rows)
	}
}

func TestServerActionsFromEnvEvaluatesEquationTypedValues(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := env.Model("ir.model").Create(map[string]any{"model": "ir.sequence", "name": "Sequence"})
	if err != nil {
		t.Fatal(err)
	}
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Test Seq",
		"code":             "test.seq",
		"prefix":           "T/",
		"padding":          int64(4),
		"number_next":      int64(7),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":              "Increase Next",
		"model_id":          modelID,
		"model_name":        "ir.sequence",
		"state":             "object_write",
		"active":            true,
		"update_path":       "number_next",
		"update_field_type": "integer",
		"evaluation_type":   "equation",
		"value":             "record.number_next + 5",
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := serverActionsFromEnv(env, map[string]data.ExternalID{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), actionID, serveractions.ExecutionContext{Model: "ir.sequence", RecordID: sequenceID}); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["number_next"] != int64(12) {
		t.Fatalf("sequence rows = %+v", rows)
	}
}

func TestBootstrapOIAIToolActionsAndTopics(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	wantActions := map[string]bool{
		"ir_actions_server_get_fields":              false,
		"ir_actions_server_search":                  false,
		"ir_actions_server_read_group":              false,
		"ir_actions_server_get_menu_details":        false,
		"ir_actions_server_compute_report_measures": false,
		"ir_actions_server_open_menu_list":          true,
		"ir_actions_server_open_menu_kanban":        true,
		"ir_actions_server_open_menu_pivot":         true,
		"ir_actions_server_open_menu_graph":         true,
		"ir_actions_server_adjust_search":           true,
	}
	for xmlTail, allowEnd := range wantActions {
		xmlID := "ai." + xmlTail
		actionID := app.ExternalIDs[xmlID].ResID
		action, ok := app.ServerActions.Get(actionID)
		if actionID == 0 || !ok {
			t.Fatalf("missing action %s id=%d", xmlID, actionID)
		}
		if !action.UseInAI || action.AIToolAllowEndMessage != allowEnd || strings.TrimSpace(action.AIToolSchema) == "" {
			t.Fatalf("action %s = %+v", xmlID, action)
		}
		if action.Metadata["xml_id"] != xmlID {
			t.Fatalf("action metadata %s = %+v", xmlID, action.Metadata)
		}
		tool, err := aitools.ServerActionTool(action, app.ServerActions)
		if err != nil {
			t.Fatalf("tool %s: %v", xmlID, err)
		}
		if tool.Name != xmlTail {
			t.Fatalf("tool name for %s = %s", xmlID, tool.Name)
		}
	}
	assertRuntimeTopicTools(t, app, "ai.ai_topic_information_retrieval_query", []string{
		"ir_actions_server_get_fields",
		"ir_actions_server_search",
		"ir_actions_server_read_group",
	})
	assertRuntimeTopicTools(t, app, "ai.ai_topic_natural_language_query", []string{
		"ir_actions_server_get_fields",
		"ir_actions_server_get_menu_details",
		"ir_actions_server_compute_report_measures",
		"ir_actions_server_open_menu_list",
		"ir_actions_server_open_menu_kanban",
		"ir_actions_server_open_menu_pivot",
		"ir_actions_server_open_menu_graph",
		"ir_actions_server_adjust_search",
	})
}

func TestRuntimeAIToolActionsRunFromServerActions(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	fieldsResult, err := app.ServerActions.Run(context.Background(), app.ExternalIDs["ai.ir_actions_server_get_fields"].ResID, serveractions.ExecutionContext{
		Values: map[string]any{"model_name": "res.partner", "include_description": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	fieldsCSV := stringValue(fieldsResult.Metadata["result"])
	if !strings.Contains(fieldsCSV, "field_name|display_name|type|sortable|groupable") || !strings.Contains(fieldsCSV, "name|") {
		t.Fatalf("fields csv = %s", fieldsCSV)
	}
	partnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "Ada Lovelace", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	searchResult, err := app.ServerActions.Run(context.Background(), app.ExternalIDs["ai.ir_actions_server_search"].ResID, serveractions.ExecutionContext{
		Values: map[string]any{
			"model_name": "res.partner",
			"domain":     `[["name","ilike","Ada"]]`,
			"fields":     []string{"name"},
			"limit":      int64(1),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	searchRows, ok := searchResult.Metadata["result"].([]map[string]any)
	if !ok || len(searchRows) != 1 || int64Value(searchRows[0]["id"]) != partnerID || searchRows[0]["name"] != "Ada Lovelace" {
		t.Fatalf("search result = %+v", searchResult.Metadata["result"])
	}
	viewID, err := app.Env.Model("ir.ui.view").Create(map[string]any{
		"name":  "res.partner.search.ai",
		"model": "res.partner",
		"type":  "search",
		"arch":  `<search><field name="name"/></search>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := app.Env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Partners",
		"res_model": "res.partner",
		"view_id":   viewID,
		"view_mode": "list,kanban,pivot,graph",
		"domain":    "[]",
		"context":   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	menuID, err := app.Env.Model("ir.ui.menu").Create(map[string]any{
		"name":   "Partners",
		"action": strconv.FormatInt(actionID, 10),
	})
	if err != nil {
		t.Fatal(err)
	}
	menuResult, err := app.ServerActions.Run(context.Background(), app.ExternalIDs["ai.ir_actions_server_get_menu_details"].ResID, serveractions.ExecutionContext{
		Values: map[string]any{"menu_ids": []int64{menuID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	menuCSV := stringValue(menuResult.Metadata["result"])
	if !strings.Contains(menuCSV, "menu_id|model|context|domain|search_view") || !strings.Contains(menuCSV, "res.partner") || !strings.Contains(menuCSV, `<search><field name="name"/></search>`) {
		t.Fatalf("menu details = %s", menuCSV)
	}
	sub := app.Bus.Subscribe(aiUserBusChannel(1), 0)
	defer sub.Close()
	openResult, err := app.ServerActions.Run(context.Background(), app.ExternalIDs["ai.ir_actions_server_open_menu_list"].ResID, serveractions.ExecutionContext{
		Values: map[string]any{
			"menu_id":             menuID,
			"model_name":          "res.partner",
			"selected_filters":    []string{"active"},
			"selected_groupbys":   []string{"company_id"},
			"search":              []string{"Ada"},
			"custom_domain":       `[["active","=",true]]`,
			aitools.EndMessageKey: "Opened partners.",
		},
		Metadata: map[string]any{
			"user_id":               int64(1),
			"ai_session_identifier": "sid-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := openResult.Metadata["result"].(map[string]any)
	if !ok || payload["event"] != "AI_OPEN_MENU_LIST" || int64Value(payload["menuID"]) != menuID {
		t.Fatalf("open result = %+v", openResult.Metadata["result"])
	}
	if payload["aiSessionIdentifier"] != "sid-1" {
		t.Fatalf("open payload = %+v", payload)
	}
	event := readRuntimeBusEvent(t, sub.Events)
	if event.Name != "AI_OPEN_MENU_LIST" || event.Payload["event"] != nil || event.Payload["aiSessionIdentifier"] != "sid-1" || int64Value(event.Payload["menuID"]) != menuID {
		t.Fatalf("bus event = %+v", event)
	}
	if domainPayload, ok := event.Payload["customDomain"].([]any); !ok || len(domainPayload) != 1 {
		t.Fatalf("custom domain payload = %#v", event.Payload["customDomain"])
	}
	sessionCookie := runtimeSessionCookie(t, app)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bus/poll", bytes.NewBufferString(`{"channels":["user/1"],"last_id":0}`))
	req.AddCookie(sessionCookie)
	app.Server().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bus poll %d %s", rec.Code, rec.Body.String())
	}
	polled := decodeRuntimeJSONList(t, rec.Body.Bytes())
	if len(polled) != 1 || polled[0]["type"] != "AI_OPEN_MENU_LIST" {
		t.Fatalf("bus poll payload = %+v", polled)
	}
}

func TestBootstrapOIExposesHTTPModulesAssetsMenusAndViews(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	handler := app.Server().Handler()
	if app.ExternalIDs["mail.email_compose_message_wizard_form"].ResID == 0 {
		t.Fatalf("missing mail compose view external id: %+v", app.ExternalIDs)
	}

	sessionCookie := runtimeSessionCookie(t, app)
	body := getBodyWithCookie(t, handler, "/web/session/modules", sessionCookie)
	modulesPayload := decodeRuntimeJSON(t, []byte(body))
	installedModules := map[string]bool{}
	for _, name := range runtimeStringSlice(modulesPayload["installed_modules"]) {
		installedModules[name] = true
	}
	for _, want := range []string{"base_setup", "onboarding", "uom", "product", "analytic", "portal", "digest", "oi_base", "oi_workflow", "oi_workflow_advance", "oi_delegation", "oi_login_as"} {
		if !installedModules[want] {
			t.Fatalf("modules response missing %s: %s", want, body)
		}
	}
	if installedModules[accounting.ModuleName] {
		t.Fatalf("accounting should not be installed by default: %s", body)
	}
	modules := runtimeMapValue(modulesPayload["modules"])
	for _, name := range []string{"base_setup", "onboarding", "uom", "product", "analytic", "portal", "digest"} {
		modulePayload := runtimeMapValue(modules[name])
		if modulePayload["technical_name"] != name || modulePayload["state"] != "installed" || modulePayload["installable"] != true {
			t.Fatalf("anchor module %s payload = %+v", name, modulePayload)
		}
	}

	backend := app.Assets.Bundle(assets.Backend)
	for _, want := range []string{"static/src/js/form_controller.js", "static/src/login_as/login_as.js", "static/src/xml/templates.xml"} {
		if !containsString(backend, want) {
			t.Fatalf("backend assets missing %s in %+v", want, backend)
		}
	}
	body = getBody(t, handler, "/web/bundle/web.assets_backend")
	if !strings.Contains(body, "static/src/login_as/login_as.js") || !strings.Contains(body, "static/src/js/form_controller.js") {
		t.Fatalf("bundle response = %s", body)
	}

	body = getBodyWithCookie(t, handler, "/web/webclient/load_menus", sessionCookie)
	for _, want := range []string{"Approvals", "Delegation", "Approval Buttons", "Settings", "Technical", "Server Actions", "Scheduled Actions", "Automation Rules", "Access Rights", "Record Rules"} {
		if !strings.Contains(body, want) {
			t.Fatalf("menu response missing %s: %s", want, body)
		}
	}
	menuPayload := decodeRuntimeJSON(t, []byte(body))
	settingsMenu := runtimeMenuByXMLID(menuPayload, "base.menu_administration")
	if settingsMenu["name"] != "Settings" || settingsMenu["is_app"] != true || settingsMenu["actionModel"] != "ir.actions.act_window" || int64Value(settingsMenu["actionID"]) == 0 {
		t.Fatalf("settings menu = %+v", settingsMenu)
	}
	technicalMenu := runtimeMenuByXMLID(menuPayload, "base.menu_technical")
	if technicalMenu["name"] != "Technical" || int64Value(technicalMenu["parent_id"]) != int64Value(settingsMenu["id"]) {
		t.Fatalf("technical menu = %+v settings=%+v", technicalMenu, settingsMenu)
	}
	serverActionsMenu := runtimeMenuByXMLID(menuPayload, "base.menu_ir_actions_server")
	if serverActionsMenu["name"] != "Server Actions" || serverActionsMenu["actionModel"] != "ir.actions.act_window" || int64Value(serverActionsMenu["actionID"]) == 0 || int64Value(serverActionsMenu["parent_id"]) == 0 {
		t.Fatalf("server actions menu = %+v", serverActionsMenu)
	}
	recordRulesMenu := runtimeMenuByXMLID(menuPayload, "base.menu_ir_rule")
	if recordRulesMenu["name"] != "Record Rules" || recordRulesMenu["actionModel"] != "ir.actions.act_window" || int64Value(recordRulesMenu["actionID"]) == 0 {
		t.Fatalf("record rules menu = %+v", recordRulesMenu)
	}
	actionViews := postRuntimeJSONWithCookie(t, handler, "/web/dataset/call_kw", `{"model":"ir.actions.server","method":"get_views","kwargs":{"views":[[false,"list"],[false,"form"]],"options":{}}}`, sessionCookie)
	views := runtimeMapValue(actionViews["views"])
	actionListArch := stringValue(runtimeMapValue(views["list"])["arch"])
	actionFormArch := stringValue(runtimeMapValue(views["form"])["arch"])
	for _, want := range []string{`name="name"`, `name="state"`, `name="model_name"`} {
		if !strings.Contains(actionListArch, want) {
			t.Fatalf("server action list view missing %s: %s", want, actionListArch)
		}
	}
	for _, want := range []string{`name="name"`, `name="model_id"`, `name="state"`, `name="code"`} {
		if !strings.Contains(actionFormArch, want) {
			t.Fatalf("server action form view missing %s: %s", want, actionFormArch)
		}
	}

	body = getBodyWithCookie(t, handler, "/web/view/load?model="+internalworkflow.ModelWorkflow, sessionCookie)
	if !strings.Contains(body, "view_workflow_workflow_form") && !strings.Contains(body, "flowchart") {
		t.Fatalf("workflow views response = %s", body)
	}
	body = getBodyWithCookie(t, handler, "/web/view/load?model=mail.compose.message", sessionCookie)
	if !strings.Contains(body, "mail.compose.message.form") || !strings.Contains(body, "action_send_mail") {
		t.Fatalf("mail compose views response = %s", body)
	}
}

func TestBootstrapOIAIGenerateResponseUsesEnvStoreAndPersistedRAG(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	provider := &runtimeCaptureAIProvider{}
	app.AIProvider = provider
	app.AIEmbeddingModel = "mock-embedding"
	app.AIBaseURL = "https://gorp.test"

	agentPartnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "AI Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := app.Env.Model(aiaddon.ModelAgent).Create(map[string]any{
		"name":                "ERP Assistant",
		"system_prompt":       "Answer from authorized ERP sources.",
		"llm_model":           "mock-chat",
		"active":              true,
		"restrict_to_sources": true,
		"partner_id":          agentPartnerID,
		"company_id":          int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := app.Env.Model(aiaddon.ModelAgentSource).Create(map[string]any{
		"name":            "Policy",
		"agent_id":        agentID,
		"type":            "text",
		"source_type":     "text",
		"status":          "indexed",
		"is_active":       true,
		"state":           "ready",
		"attachment_id":   int64(100),
		"url":             "https://gorp.test/policy",
		"embedding_model": "mock-embedding",
		"company_id":      int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Env.Model(aiaddon.ModelAgent).Browse(agentID).Write(map[string]any{"source_ids": []int64{sourceID}, "sources_ids": []int64{sourceID}}); err != nil {
		t.Fatal(err)
	}
	_, err = rag.StoreChunks(app.Env, []embeddings.Chunk{{
		SourceID:       sourceID,
		Ref:            embeddings.RecordRef{Model: "res.partner", ID: agentPartnerID, CompanyID: 1},
		Content:        "The indexed policy says the needle is in storage bin A.",
		Vector:         []float64{1, 0, 0},
		EmbeddingModel: "mock-embedding",
		Metadata: map[string]any{
			sources.MetadataAgentID:        agentID,
			sources.MetadataSourceID:       sourceID,
			sources.MetadataSourceName:     "Policy",
			sources.MetadataAttachmentID:   int64(100),
			sources.MetadataSourceURL:      "https://gorp.test/policy",
			sources.MetadataCompanyID:      int64(1),
			sources.MetadataEmbeddingModel: "mock-embedding",
			"is_active":                    true,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	channelID, err := app.Env.Model("discuss.channel").Create(map[string]any{
		"name":           "AI Chat",
		"channel_type":   "ai_chat",
		"active":         true,
		"ai_agent_id":    agentID,
		"ai_env_context": "record context",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model("discuss.channel.member").Create(map[string]any{"channel_id": channelID, "user_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	messageID, err := app.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Where is the needle?</p>",
		"message_type": "comment",
		"model":        "discuss.channel",
		"res_id":       channelID,
		"date":         time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionCookie := runtimeSessionCookie(t, app)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":11,"params":{"mail_message_id":` + strconv.FormatInt(messageID, 10) + `,"channel_id":` + strconv.FormatInt(channelID, 10) + `}}`)
	req := httptest.NewRequest(http.MethodPost, "/ai/generate_response", body)
	req.AddCookie(sessionCookie)
	app.Server().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("generate response %d %s", rec.Code, rec.Body.String())
	}
	if len(provider.chatRequests) != 1 {
		t.Fatalf("chat requests = %+v", provider.chatRequests)
	}
	prompts := strings.Join(provider.chatRequests[0].SystemPrompts, "\n")
	if !strings.Contains(prompts, "##RAG context information") || !strings.Contains(prompts, "needle is in storage bin A") || !strings.Contains(prompts, "record context") {
		t.Fatalf("prompts = %s", prompts)
	}
	rows, err := app.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "discuss.channel"),
		domain.Cond("res_id", domain.Equal, channelID),
	))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := rows.Read("body", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	var posted string
	for _, message := range messages {
		if int64Value(message["author_id"]) == agentPartnerID {
			posted = stringValue(message["body"])
		}
	}
	if !strings.Contains(posted, `href="https://gorp.test/policy"`) || !strings.Contains(posted, "[1]") {
		t.Fatalf("posted assistant message = %q", posted)
	}
}

func TestBootstrapOIAIGenerateResponseRunsTopicToolLoop(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	provider := &runtimeCaptureAIProvider{
		providerKind: aiproviders.KindOpenAI,
		chatText:     "Tool answer",
		models: []aiproviders.Model{
			{ID: "gpt-5-mini", Label: "GPT-5 Mini", Kind: aiproviders.ModelChat, Provider: aiproviders.KindOpenAI},
		},
		toolResponses: [][]aiproviders.ToolCall{{
			{
				Name:      "ir_actions_server_get_fields",
				Arguments: map[string]any{"model_name": "res.partner", "include_description": false},
			},
		}},
	}
	app.AIProviders = aiproviders.NewRegistry(provider)
	app.AIProvider = nil
	defaultAgentID := app.ExternalIDs["ai.ai_default_agent"].ResID
	channelID, err := app.Env.Model("discuss.channel").Create(map[string]any{
		"name":         "AI Tool Chat",
		"channel_type": "ai_chat",
		"active":       true,
		"ai_agent_id":  defaultAgentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model("discuss.channel.member").Create(map[string]any{"channel_id": channelID, "user_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	messageID, err := app.Env.Model("mail.message").Create(map[string]any{
		"body":         "What fields exist on partners?",
		"message_type": "comment",
		"model":        "discuss.channel",
		"res_id":       channelID,
		"date":         time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionCookie := runtimeSessionCookie(t, app)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":13,"params":{"mail_message_id":` + strconv.FormatInt(messageID, 10) + `,"channel_id":` + strconv.FormatInt(channelID, 10) + `,"current_view_info":{"model":"res.partner","active_id":42}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ai/generate_response", body)
	req.AddCookie(sessionCookie)
	app.Server().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("generate response %d %s", rec.Code, rec.Body.String())
	}
	if len(provider.chatRequests) != 2 {
		t.Fatalf("chat requests = %+v", provider.chatRequests)
	}
	if !runtimeRequestHasTool(provider.chatRequests[0], "ir_actions_server_get_fields") || !runtimeRequestHasTool(provider.chatRequests[0], "ir_actions_server_search") {
		t.Fatalf("first request tools = %+v", provider.chatRequests[0].Tools)
	}
	if len(provider.chatRequests[1].Messages) == 0 || !strings.Contains(provider.chatRequests[1].Messages[len(provider.chatRequests[1].Messages)-1].Content, "field_name|display_name|type|sortable|groupable") {
		t.Fatalf("second request messages = %+v", provider.chatRequests[1].Messages)
	}
	messages, err := app.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "discuss.channel"),
		domain.Cond("res_id", domain.Equal, channelID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := messages.Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if !runtimeRowsContainBody(rows, "Tool answer") {
		t.Fatalf("messages = %+v", rows)
	}
}

func TestRuntimeAIDraftChannelCallKWCreatesContext(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "Ada Lovelace", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	handler := app.Server().Handler()
	sessionCookie := runtimeSessionCookie(t, app)
	body := `{"model":"discuss.channel","method":"create_ai_draft_channel","args":["mail_composer"],"kwargs":{"channel_title":"Partner reply","record_model":"res.partner","record_id":` + strconv.FormatInt(partnerID, 10) + `,"text_selection":"Please reply","front_end_info":{"view_type":"form"}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/discuss.channel/create_ai_draft_channel", bytes.NewBufferString(body))
	req.AddCookie(sessionCookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create_ai_draft_channel response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeRuntimeJSON(t, rec.Body.Bytes())
	channelID := int64Value(payload["ai_channel_id"])
	if channelID == 0 {
		t.Fatalf("missing ai_channel_id in %+v", payload)
	}
	if prompts := runtimeStringSlice(payload["prompts"]); len(prompts) == 0 || prompts[0] != "Write a followup answer" {
		t.Fatalf("prompts = %+v", payload["prompts"])
	}
	if payload["model_has_thread"] != false {
		t.Fatalf("model_has_thread = %+v", payload["model_has_thread"])
	}
	rows, err := app.Env.Model("discuss.channel").Browse(channelID).Read("name", "channel_type", "ai_agent_id", "ai_env_context")
	if err != nil || len(rows) != 1 {
		t.Fatalf("channel read rows=%+v err=%v", rows, err)
	}
	if rows[0]["name"] != "AI: Partner reply" || rows[0]["channel_type"] != "ai_chat" || int64Value(rows[0]["ai_agent_id"]) != app.ExternalIDs["ai.ai_default_agent"].ResID {
		t.Fatalf("channel row = %+v", rows[0])
	}
	context := stringValue(rows[0]["ai_env_context"])
	for _, want := range []string{"Draft message bodies only", "Current record: res.partner", "Ada Lovelace", "Selected text: Please reply", "view_type"} {
		if !strings.Contains(context, want) {
			t.Fatalf("context missing %q: %s", want, context)
		}
	}
	members, err := app.Env.Model("discuss.channel.member").Search(domain.And(
		domain.Cond("channel_id", domain.Equal, channelID),
		domain.Cond("user_id", domain.Equal, int64(1)),
	))
	if err != nil || members.Len() != 1 {
		t.Fatalf("user channel member len=%d err=%v", members.Len(), err)
	}
	positionalBody := `{"model":"discuss.channel","method":"create_ai_draft_channel","args":["mail_composer","Positional reply","res.partner",` + strconv.FormatInt(partnerID, 10) + `,{"view_type":"form"},"Selected via args"]}`
	positional := postRuntimeJSONWithCookie(t, handler, "/web/dataset/call_kw/discuss.channel/create_ai_draft_channel", positionalBody, sessionCookie)
	positionalChannelID := int64Value(positional["ai_channel_id"])
	if positionalChannelID == 0 || positionalChannelID == channelID {
		t.Fatalf("positional channel payload = %+v", positional)
	}
	positionalRows, err := app.Env.Model("discuss.channel").Browse(positionalChannelID).Read("name", "ai_env_context")
	if err != nil || len(positionalRows) != 1 {
		t.Fatalf("positional channel rows=%+v err=%v", positionalRows, err)
	}
	if positionalRows[0]["name"] != "AI: Positional reply" || !strings.Contains(stringValue(positionalRows[0]["ai_env_context"]), "Selected via args") {
		t.Fatalf("positional channel row = %+v", positionalRows[0])
	}
}

func TestRuntimeAIAskAIActionAlwaysCreatesNewChannel(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	handler := app.Server().Handler()
	sessionCookie := runtimeSessionCookie(t, app)
	actionOne := postRuntimeJSONWithCookie(t, handler, "/web/dataset/call_kw/ai.agent/action_ask_ai", `{"model":"ai.agent","method":"action_ask_ai","args":["Show partners"]}`, sessionCookie)
	actionTwo := postRuntimeJSONWithCookie(t, handler, "/web/dataset/call_kw/ai.agent/action_ask_ai", `{"model":"ai.agent","method":"action_ask_ai","args":["Show partners again"]}`, sessionCookie)
	paramsOne := runtimeMapValue(actionOne["params"])
	paramsTwo := runtimeMapValue(actionTwo["params"])
	channelOne := int64Value(paramsOne["channelId"])
	channelTwo := int64Value(paramsTwo["channelId"])
	if actionOne["tag"] != "agent_chat_action" || channelOne == 0 || channelTwo == 0 || channelOne == channelTwo {
		t.Fatalf("actions = %+v %+v", actionOne, actionTwo)
	}
	if paramsOne["user_prompt"] != "Show partners" {
		t.Fatalf("action params = %+v", actionOne["params"])
	}
	agent := postRuntimeJSONWithCookie(t, handler, "/web/dataset/call_kw/ai.agent/get_ask_ai_agent", `{"model":"ai.agent","method":"get_ask_ai_agent"}`, sessionCookie)
	if int64Value(agent["id"]) != app.ExternalIDs["ai.ai_default_agent"].ResID || strings.TrimSpace(stringValue(agent["name"])) == "" {
		t.Fatalf("ask ai agent = %+v", agent)
	}
}

func TestBootstrapOIAIGenerateResponseUsesSettingsDefaultProviderModel(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	provider := &runtimeCaptureAIProvider{
		providerKind: aiproviders.KindOpenAI,
		chatText:     "Settings answer",
		models: []aiproviders.Model{
			{ID: "gpt-5-mini", Label: "GPT-5 Mini", Kind: aiproviders.ModelChat, Provider: aiproviders.KindOpenAI},
			{ID: "text-embedding-3-small", Label: "Text Embedding 3 Small", Kind: aiproviders.ModelEmbedding, Provider: aiproviders.KindOpenAI},
		},
	}
	app.AIProviders = aiproviders.NewRegistry(provider)
	app.AIProvider = nil
	app.AIEmbeddingModel = ""
	if _, err := app.Env.Model(aiaddon.ModelSettings).Create(map[string]any{
		"name":                    "Default",
		"default_chat_model":      "gpt-5-mini",
		"default_embedding_model": "text-embedding-3-small",
		"default_provider":        "openai",
	}); err != nil {
		t.Fatal(err)
	}
	agentPartnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "Settings AI"})
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := app.Env.Model(aiaddon.ModelAgent).Create(map[string]any{
		"name":          "Settings Agent",
		"system_prompt": "Answer from settings.",
		"active":        true,
		"partner_id":    agentPartnerID,
		"company_id":    int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	channelID, err := app.Env.Model("discuss.channel").Create(map[string]any{
		"name":         "AI Chat",
		"channel_type": "ai_chat",
		"active":       true,
		"ai_agent_id":  agentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model("discuss.channel.member").Create(map[string]any{"channel_id": channelID, "user_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	messageID, err := app.Env.Model("mail.message").Create(map[string]any{
		"body":         "Use settings.",
		"message_type": "comment",
		"model":        "discuss.channel",
		"res_id":       channelID,
		"date":         time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionCookie := runtimeSessionCookie(t, app)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":12,"params":{"mail_message_id":` + strconv.FormatInt(messageID, 10) + `,"channel_id":` + strconv.FormatInt(channelID, 10) + `}}`)
	req := httptest.NewRequest(http.MethodPost, "/ai/generate_response", body)
	req.AddCookie(sessionCookie)
	app.Server().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("generate response %d %s", rec.Code, rec.Body.String())
	}
	if len(provider.chatRequests) != 1 || provider.chatRequests[0].Model != "gpt-5-mini" {
		t.Fatalf("chat requests = %+v", provider.chatRequests)
	}
	rows, err := app.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "discuss.channel"),
		domain.Cond("res_id", domain.Equal, channelID),
	))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := rows.Read("body", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	var posted string
	for _, message := range messages {
		if int64Value(message["author_id"]) == agentPartnerID {
			posted = stringValue(message["body"])
		}
	}
	if posted != "Settings answer" {
		t.Fatalf("posted assistant message = %q", posted)
	}
}

func TestAIProviderResolverUsesSettingsSecretReference(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model("ir.config_parameter").Create(map[string]any{"key": "ai.openai_key", "value": "stored-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model(aiaddon.ModelSettings).Create(map[string]any{
		"name":             "Settings",
		"default_provider": "openai",
		"secret_source":    "secret_store",
		"secret_ref":       "ai.openai_key",
	}); err != nil {
		t.Fatal(err)
	}
	settings := aiRuntimeSettingsFromEnv(app.Env)
	resolver := app.aiProviderResolver(app.Env, settings)
	provider, model, err := resolver.chatProvider("gpt-5-mini")
	if err != nil {
		t.Fatal(err)
	}
	if model != "gpt-5-mini" {
		t.Fatalf("model = %s", model)
	}
	secret, err := provider.(*aiproviders.CompatibleProvider).Resolver(context.Background(), provider.(*aiproviders.CompatibleProvider).Secret)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "stored-key" {
		t.Fatal("resolved secret mismatch")
	}
}

func TestAIProviderResolverUsesConfigParameterBeforeEnvWithoutSettings(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ODOO_AI_CHATGPT_TOKEN", "env-key")
	if _, err := app.Env.Model("ir.config_parameter").Create(map[string]any{"key": "ai.openai_key", "value": "stored-key"}); err != nil {
		t.Fatal(err)
	}
	settingsEnv := app.aiSettingsEnv(app.Env)
	resolver := app.aiProviderResolver(settingsEnv, aiRuntimeSettingsFromEnv(settingsEnv))
	provider, _, err := resolver.chatProvider("gpt-5-mini")
	if err != nil {
		t.Fatal(err)
	}
	compatible, ok := provider.(*aiproviders.CompatibleProvider)
	if !ok {
		t.Fatal("provider is not compatible provider")
	}
	secret, err := compatible.Resolver(context.Background(), compatible.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "stored-key" {
		t.Fatal("resolved secret mismatch")
	}
}

func TestAIProviderResolverMapsGoogleProviderAlias(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model("ir.config_parameter").Create(map[string]any{"key": "custom.google_key", "value": "stored-google-key"}); err != nil {
		t.Fatal(err)
	}
	settingsEnv := app.aiSettingsEnv(app.Env)
	resolver := app.aiProviderResolver(settingsEnv, aiRuntimeSettings{
		defaultProvider: aiProviderKindFromSettings("google"),
		secretSource:    "secret_store",
		secretRef:       "custom.google_key",
	})
	provider, _, err := resolver.chatProvider("gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	compatible, ok := provider.(*aiproviders.CompatibleProvider)
	if !ok {
		t.Fatal("provider is not compatible provider")
	}
	secret, err := compatible.Resolver(context.Background(), compatible.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "stored-google-key" {
		t.Fatal("resolved secret mismatch")
	}
}

func TestAISettingsEnvUsesSystemContext(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Env.Model(aiaddon.ModelSettings).Create(map[string]any{
		"name":               "Default",
		"default_chat_model": "gpt-5-mini",
	}); err != nil {
		t.Fatal(err)
	}
	ctx := app.Env.Context()
	ctx.UserID = 42
	requestEnv := app.Env.WithContext(ctx)
	settingsEnv := app.aiSettingsEnv(requestEnv)
	if settingsEnv.Context().UserID != 1 {
		t.Fatal("settings env is not system context")
	}
	settings := aiRuntimeSettingsFromEnv(settingsEnv)
	if settings.defaultChatModel != "gpt-5-mini" {
		t.Fatal("settings read failed")
	}
}

func TestRuntimeAIActionRunsSelectedToolAndLogsSummary(t *testing.T) {
	app, err := BootstrapOI("")
	if err != nil {
		t.Fatal(err)
	}
	provider := &runtimeCaptureAIProvider{
		providerKind: aiproviders.KindOpenAI,
		models: []aiproviders.Model{
			{ID: "gpt-4.1", Label: "GPT-4.1", Kind: aiproviders.ModelChat, Provider: aiproviders.KindOpenAI},
		},
		toolResponses: [][]aiproviders.ToolCall{{
			{
				Name: "rename_partner",
				Arguments: map[string]any{
					"name":          "Ada Lovelace",
					"__end_message": "Done.",
				},
			},
		}},
	}
	app.AIProviders = aiproviders.NewRegistry(provider)
	app.AIProvider = nil
	partnerID, err := app.Env.Model("res.partner").Create(map[string]any{"name": "Original", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	toolID, err := app.ServerActions.Register(serveractions.ServerAction{
		Name:                  "Rename Partner",
		Model:                 "res.partner",
		Kind:                  serveractions.KindWrite,
		UseInAI:               true,
		AIToolDescription:     "Rename the active partner.",
		AIToolAllowEndMessage: true,
		AIToolSchema: `{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"__end_message": {"type": "string"}
			},
			"required": ["name", "__end_message"]
		}`,
		Metadata: map[string]any{"xml_id": "ai.rename_partner"},
	})
	if err != nil {
		t.Fatal(err)
	}
	aiID, err := app.ServerActions.Register(serveractions.ServerAction{
		Name:           "AI Rename",
		Model:          "res.partner",
		Kind:           serveractions.KindAI,
		AIActionPrompt: "Rename the current partner using the provided name.",
		AIToolIDs:      []int64{toolID},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.ServerActions.Run(context.Background(), aiID, serveractions.ExecutionContext{
		Model:    "res.partner",
		RecordID: partnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != serveractions.KindAI || result.Metadata["response"] != "Done." || result.Metadata["tool_count"] != 1 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := app.Env.Model("res.partner").Browse(partnerID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Ada Lovelace" {
		t.Fatalf("partner = %+v", rows[0])
	}
	if len(provider.chatRequests) != 1 || len(provider.chatRequests[0].Tools) != 1 || provider.chatRequests[0].Tools[0].Name != "rename_partner" {
		t.Fatalf("chat request tools = %+v", provider.chatRequests)
	}
	messages, err := app.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "res.partner"),
		domain.Cond("res_id", domain.Equal, partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || !strings.Contains(stringValue(messageRows[0]["body"]), "Rename Partner") {
		t.Fatalf("messages = %+v", messageRows)
	}
}

func TestRuntimeSecretRefParsesCombinedAndSplitValues(t *testing.T) {
	if got := runtimeSecretRef("", "env:ODOO_AI_CHATGPT_TOKEN"); got.EnvName != "ODOO_AI_CHATGPT_TOKEN" {
		t.Fatalf("env ref = %+v", got)
	}
	if got := runtimeSecretRef("secret_store", "ai.openai_key"); got.StoreID != "ai.openai_key" {
		t.Fatalf("store ref = %+v", got)
	}
	if got := runtimeSecretRef("", "sk-raw-secret"); got.Redacted() != "" {
		t.Fatal("raw-looking value accepted")
	}
}

func TestAssetsFromSourcesAppliesAssetRowsAroundManifestAssets(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "pre asset", "active": true, "bundle": assets.Backend, "directive": "append", "path": "pre.js", "sequence": 1},
		{"name": "post asset", "active": true, "bundle": assets.Backend, "directive": "append", "path": "post.js", "sequence": 16},
		{"name": "remove asset", "active": true, "bundle": assets.Backend, "directive": "remove", "path": "remove-me.js", "sequence": 16},
	} {
		if _, err := env.Model("ir.asset").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := assetsFromSources(env, []module.Manifest{{
		Name:          "Web",
		TechnicalName: "web",
		Version:       "19.0.1.0.0",
		Assets: map[string][]string{
			assets.Backend: {"manifest.js", "remove-me.js"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(assets.Backend)
	want := []string{"pre.js", "manifest.js", "post.js"}
	if len(got) != len(want) {
		t.Fatalf("bundle = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bundle = %+v", got)
		}
	}
}

func TestAssetsFromSourcesAppliesManifestAssetOperations(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := assetsFromSources(env, []module.Manifest{{
		Name:          "Web",
		TechnicalName: "web",
		Version:       "19.0.1.0.0",
		AssetOperations: map[string][]module.AssetOperation{
			assets.Backend: {
				{Directive: "append", Path: "base.js"},
				{Directive: "after", Target: "base.js", Path: "after.js"},
				{Directive: "before", Target: "base.js", Path: "before.js"},
				{Directive: "remove", Path: "after.js"},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(assets.Backend)
	want := []string{"before.js", "base.js"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestAssetsFromSourcesUsesInstalledDependencyOrder(t *testing.T) {
	env, err := oiEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "base", "state": "installed"},
		{"name": "web", "state": "installed"},
		{"name": "theme", "state": "installed"},
		{"name": "ghost", "state": "uninstalled"},
	} {
		if _, err := env.Model("ir.module.module").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := assetsFromSources(env, []module.Manifest{
		{
			Name:          "Theme",
			TechnicalName: "theme",
			Version:       "19.0.1.0.0",
			Depends:       []string{"web"},
			Assets:        map[string][]string{assets.Backend: {"theme.js"}},
		},
		{
			Name:          "Ghost",
			TechnicalName: "ghost",
			Version:       "19.0.1.0.0",
			Depends:       []string{"base"},
			Assets:        map[string][]string{assets.Backend: {"ghost.js"}},
		},
		{
			Name:          "Web",
			TechnicalName: "web",
			Version:       "19.0.1.0.0",
			Depends:       []string{"base"},
			Assets:        map[string][]string{assets.Backend: {"web.js"}},
		},
		{
			Name:          "Base",
			TechnicalName: "base",
			Version:       "19.0.1.0.0",
			Assets:        map[string][]string{assets.Backend: {"base.js"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Bundle(assets.Backend)
	want := []string{"base.js", "web.js", "theme.js"}
	if len(got) != len(want) {
		t.Fatalf("bundle = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bundle = %+v", got)
		}
	}
}

func getBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func getBodyWithCookie(t *testing.T, handler http.Handler, path string, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func postRuntimeJSON(t *testing.T, handler http.Handler, path string, body string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return decodeRuntimeJSON(t, rec.Body.Bytes())
}

func postRuntimeJSONWithCookie(t *testing.T, handler http.Handler, path string, body string, cookie *http.Cookie) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return decodeRuntimeJSON(t, rec.Body.Bytes())
}

func runtimeSessionCookie(t *testing.T, app *App) *http.Cookie {
	t.Helper()
	if app.Security == nil {
		t.Fatal("missing security engine")
	}
	ctx := app.Env.Context()
	userID := ctx.UserID
	user := app.Security.Users[userID]
	user.ID = userID
	user.Active = true
	if user.Login == "" {
		user.Login = "runtime-test"
	}
	if user.CompanyID == 0 {
		user.CompanyID = ctx.CompanyID
	}
	if len(user.CompanyIDs) == 0 {
		user.CompanyIDs = append([]int64(nil), ctx.CompanyIDs...)
	}
	user.GroupIDs = user.GroupIDs[:0]
	for groupID := range app.Security.Groups {
		user.GroupIDs = append(user.GroupIDs, groupID)
	}
	app.Security.Users[userID] = user
	token := "test-session-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	app.Security.IssueSession(userID, token, time.Now().Add(time.Hour))
	return &http.Cookie{Name: "session_id", Value: token}
}

func decodeRuntimeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode JSON %s: %v", string(data), err)
	}
	return payload
}

func decodeRuntimeJSONList(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode JSON %s: %v", string(data), err)
	}
	return payload
}

func runtimeMapValue(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func runtimeMenuByXMLID(payload map[string]any, xmlid string) map[string]any {
	children := runtimeMapValue(payload["children"])
	for _, raw := range children {
		item := runtimeMapValue(raw)
		if item["xmlid"] == xmlid {
			return item
		}
	}
	return nil
}

func runtimeStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringValue(item))
		}
		return out
	default:
		return nil
	}
}

func runtimeInt64Slice(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64Value(item))
		}
		return out
	default:
		return nil
	}
}

func runtimeRequestHasTool(request aiproviders.ChatRequest, name string) bool {
	for _, tool := range request.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func runtimeRowsContainBody(rows []map[string]any, want string) bool {
	for _, row := range rows {
		if strings.Contains(stringValue(row["body"]), want) {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func readRuntimeBusEvent(t *testing.T, ch <-chan notifications.Event) notifications.Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bus event")
	}
	return notifications.Event{}
}

func assertRuntimeTopicTools(t *testing.T, app *App, xmlID string, want []string) {
	t.Helper()
	topicID := app.ExternalIDs[xmlID].ResID
	rows, err := app.Env.Model(aiaddon.ModelTopic).Browse(topicID).Read("tool_ids")
	if err != nil || topicID == 0 || len(rows) != 1 {
		t.Fatalf("topic %s rows=%+v err=%v", xmlID, rows, err)
	}
	got := map[string]bool{}
	for _, actionID := range int64Slice(rows[0]["tool_ids"]) {
		action, ok := app.ServerActions.Get(actionID)
		if !ok {
			t.Fatalf("missing topic action %d for %s", actionID, xmlID)
		}
		got[aitools.ServerActionToolName(action)] = true
	}
	if len(got) != len(want) {
		t.Fatalf("topic %s tools = %+v", xmlID, got)
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("topic %s missing %s in %+v", xmlID, name, got)
		}
	}
}

type runtimeCaptureAIProvider struct {
	providerKind  aiproviders.Kind
	chatText      string
	vector        []float64
	models        []aiproviders.Model
	toolResponses [][]aiproviders.ToolCall
	chatRequests  []aiproviders.ChatRequest
	embedRequests []aiproviders.EmbeddingRequest
}

func (p *runtimeCaptureAIProvider) Kind() aiproviders.Kind {
	if p.providerKind != "" {
		return p.providerKind
	}
	return aiproviders.KindMock
}

func (p *runtimeCaptureAIProvider) Models() []aiproviders.Model {
	if len(p.models) > 0 {
		return append([]aiproviders.Model(nil), p.models...)
	}
	return []aiproviders.Model{
		{ID: "mock-chat", Label: "Mock Chat", Kind: aiproviders.ModelChat, Provider: aiproviders.KindMock},
		{ID: "mock-embedding", Label: "Mock Embedding", Kind: aiproviders.ModelEmbedding, Provider: aiproviders.KindMock, Dimensions: 3},
	}
}

func (p *runtimeCaptureAIProvider) Chat(_ context.Context, request aiproviders.ChatRequest) (aiproviders.ChatResponse, error) {
	callIndex := len(p.chatRequests)
	p.chatRequests = append(p.chatRequests, request)
	if callIndex < len(p.toolResponses) && len(p.toolResponses[callIndex]) > 0 {
		return aiproviders.ChatResponse{ToolCalls: p.toolResponses[callIndex], Model: request.Model, Provider: p.Kind()}, nil
	}
	text := p.chatText
	if text == "" {
		text = "Answer [SOURCE:100]"
	}
	return aiproviders.ChatResponse{Text: text, Model: request.Model, Provider: p.Kind()}, nil
}

func (p *runtimeCaptureAIProvider) Embed(_ context.Context, request aiproviders.EmbeddingRequest) (aiproviders.EmbeddingResponse, error) {
	p.embedRequests = append(p.embedRequests, request)
	vector := p.vector
	if len(vector) == 0 {
		vector = []float64{1, 0, 0}
	}
	return aiproviders.EmbeddingResponse{Vectors: [][]float64{append([]float64(nil), vector...)}, Model: request.Model, Provider: p.Kind()}, nil
}
