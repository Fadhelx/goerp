package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	hraddon "gorp/addons/hr"
	oidelegation "gorp/addons/oi_delegation"
	"gorp/addons/oi_login_as"
	coreaccounting "gorp/internal/accounting"
	internalactions "gorp/internal/actions"
	"gorp/internal/ai/agents"
	aicontrollers "gorp/internal/ai/controllers"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/assets"
	internalbase "gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/impersonation"
	internalmail "gorp/internal/mail"
	"gorp/internal/meta/action"
	"gorp/internal/meta/menu"
	"gorp/internal/meta/view"
	"gorp/internal/model"
	"gorp/internal/notifications"
	"gorp/internal/record"
	"gorp/internal/security"
	internalworkflow "gorp/internal/workflow"
)

func TestWebRoutes(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/info", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"uid":1`) {
		t.Fatalf("session response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body := bytes.NewBufferString(`{"model":"res.partner","method":"create","values":{"name":"Demo"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":1`) {
		t.Fatalf("call_kw response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/action/load?id=1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"res_model":"res.partner"`) || !strings.Contains(rec.Body.String(), `"views"`) {
		t.Fatalf("action response %d %s", rec.Code, rec.Body.String())
	}
}

func TestReportsStaticDashboardRoute(t *testing.T) {
	assertReportsStaticDashboardRoute(t, "reports")
}

func TestReportsStaticDashboardRouteUsesCurrentReleaseLayout(t *testing.T) {
	assertReportsStaticDashboardRoute(t, filepath.Join("current", "reports"))
}

func assertReportsStaticDashboardRoute(t *testing.T, reportsDir string) {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	reportRoot := filepath.Join(dir, reportsDir)
	if err := os.MkdirAll(reportRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reportRoot, "progress_dashboard.html"), []byte("<h1>Gorp Build Dashboard</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := (Server{}).Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reports/progress_dashboard.html", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Gorp Build Dashboard") {
		t.Fatalf("dashboard response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/reports/progress_dashboard.html", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("dashboard HEAD response %d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/reports/progress_dashboard.html", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("dashboard POST response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reports/../secret.txt", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("dashboard traversal response %d %s", rec.Code, rec.Body.String())
	}
}

func TestBusPollRequiresSecuritySessionCookie(t *testing.T) {
	server := testServer(t)
	engine := security.NewEngine()
	engine.Users[9] = security.User{ID: 9, Login: "bus-user", Active: true, CompanyID: 2, CompanyIDs: []int64{2}}
	engine.IssueSession(9, "bus-sid", time.Now().Add(time.Hour))
	server.Security = engine
	server.Bus = notifications.NewBus(100)
	server.Bus.Publish(userBusChannel(9), "ping", map[string]any{"value": "ok"}, time.Now())
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/bus/poll", bytes.NewBufferString(`{"last_id":0}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated bus poll %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/bus/poll", bytes.NewBufferString(`{"last_id":0}`))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "bus-sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated bus poll %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONList(t, rec.Body.Bytes())
	if len(payload) != 1 || payload[0]["type"] != "ping" || mapValue(payload[0]["payload"])["value"] != "ok" {
		t.Fatalf("bus payload = %+v", payload)
	}
}

func TestActionLoadIncludesMultiWorkflowView(t *testing.T) {
	actions := action.NewRegistry()
	if err := actions.AddWithID(action.Action{
		ID:                42,
		Name:              "Purchase Orders",
		Kind:              action.ActWindow,
		ResModel:          "purchase.order",
		ViewMode:          "list,form",
		MultiWorkflowView: `[{"id":7,"name":"Auto Submit","view_id":9,"create_context":{"approval_auto_submit":true}}]`,
	}); err != nil {
		t.Fatal(err)
	}
	server := Server{Actions: actions}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/action/load?id=42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("action response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["multi_workflow_view"] != `[{"id":7,"name":"Auto Submit","view_id":9,"create_context":{"approval_auto_submit":true}}]` {
		t.Fatalf("action payload = %+v", payload)
	}
}

func TestCallKWSequenceNextByCodeAndID(t *testing.T) {
	server := testSequenceServer(t, 2)
	globalID, err := server.Env.Model("ir.sequence").Create(map[string]any{
		"name":             "Global",
		"code":             "stock.picking",
		"prefix":           "G/",
		"padding":          int64(3),
		"number_next":      int64(2),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := server.Env.Model("ir.sequence").Create(map[string]any{
		"name":             "Company",
		"code":             "stock.picking",
		"prefix":           "C/%(year)s/",
		"padding":          int64(2),
		"number_next":      int64(9),
		"number_increment": int64(1),
		"company_id":       int64(2),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.sequence/next_by_code", bytes.NewBufferString(`{"args":["stock.picking","2026-05-01"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("next_by_code response %d %s", rec.Code, rec.Body.String())
	}
	var value string
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value != "C/2026/09" {
		t.Fatalf("next_by_code value = %q", value)
	}
	companyRows, err := server.Env.Model("ir.sequence").Browse(companyID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	globalRows, err := server.Env.Model("ir.sequence").Browse(globalID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if companyRows[0]["number_next"] != int64(9) || globalRows[0]["number_next"] != int64(2) {
		t.Fatalf("sequence rows company=%+v global=%+v", companyRows, globalRows)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"ir.sequence","method":"next_by_id","args":[%d]}`, globalID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("next_by_id response %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value != "G/002" {
		t.Fatalf("next_by_id value = %q", value)
	}
}

func TestCallKWLinkTrackerSearchOrCreate(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	existingID, err := env.Model("link.tracker").Create(map[string]any{"url": "https://example.com/a", "label": ""})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	payload := `{"model":"link.tracker","method":"search_or_create","args":[[{"url":"https://example.com/b","label":"B"},{"url":"https://example.com/a"},{"url":"https://example.com/b","label":"B"},{"url":"https://example.com/c","label":""},{"url":"https://example.com/c"}]]}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/link.tracker/search_or_create", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("search_or_create response %d %s", rec.Code, rec.Body.String())
	}
	var ids []int64
	if err := json.Unmarshal(rec.Body.Bytes(), &ids); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 5 || ids[1] != existingID || ids[0] == 0 || ids[0] != ids[2] || ids[3] == 0 || ids[3] != ids[4] || ids[0] == ids[3] || ids[0] == existingID || ids[3] == existingID {
		t.Fatalf("ids = %+v existing=%d", ids, existingID)
	}
	found, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 3 {
		t.Fatalf("tracker count = %d", found.Len())
	}
}

func TestCallKWWhatsAppTemplateButtonComponent(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{
		"key":   "web.base.url",
		"value": "https://gorp.example/",
	}); err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Marketing", "body": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{
		"name":           "url_tracked",
		"text":           "ignored text",
		"wa_template_id": templateID,
		"sequence":       int64(0),
		"button_type":    "url",
		"url_type":       "tracked",
		"website_url":    "https://landing.example/tracked",
	}); err != nil {
		t.Fatal(err)
	}
	dynamicButtonID, err := env.Model("whatsapp.template.button").Create(map[string]any{
		"name":        "url_dynamic",
		"template_id": templateID,
		"sequence":    int64(1),
		"button_type": "url",
		"url_type":    "dynamic",
		"website_url": "https://landing.example/dynamic/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{
		"name":           "url_dynamic",
		"button_id":      dynamicButtonID,
		"wa_template_id": templateID,
		"line_type":      "button",
		"field_type":     "field",
		"field_name":     "name",
		"demo_value":     "https://landing.example/dynamic/demo-slug",
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	payload := fmt.Sprintf(`{"args":[[%d]]}`, templateID)
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/whatsapp.template/_get_template_button_component", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("button component response %d %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON(t, rec.Body.Bytes())
	if got["type"] != "BUTTONS" {
		t.Fatalf("component type = %+v", got)
	}
	buttons, ok := got["buttons"].([]any)
	if !ok || len(buttons) != 2 {
		t.Fatalf("buttons = %+v", got["buttons"])
	}
	tracked := mapValue(buttons[0])
	dynamic := mapValue(buttons[1])
	if tracked["type"] != "URL" || tracked["text"] != "url_tracked" || tracked["url"] != "https://gorp.example/{{1}}" || tracked["example"] != "https://gorp.example/???" {
		t.Fatalf("tracked button = %+v", tracked)
	}
	if dynamic["type"] != "URL" || dynamic["text"] != "url_dynamic" || dynamic["url"] != "https://landing.example/dynamic/{{1}}" || dynamic["example"] != "https://landing.example/dynamic/demo-slug" {
		t.Fatalf("dynamic button = %+v", dynamic)
	}
	trackers, err := env.Model("link.tracker").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if trackers.Len() != 0 {
		t.Fatalf("tracker count = %d", trackers.Len())
	}

	emptyTemplateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Empty", "body": "No buttons"})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	payload = fmt.Sprintf(`{"args":[[%d]]}`, emptyTemplateID)
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/whatsapp.template/_get_template_button_component", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("empty button component response %d %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("empty button component = %s", rec.Body.String())
	}
}

func TestCallButtonWhatsAppAccountSyncTemplatesImportsProviderPayload(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	accountID, err := env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "token": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"args":[[%d]],"kwargs":{"context":{"display_phone_number":"+973 1700 0000","whatsapp_template_response":{"data":[{"id":"101","name":"order_update","language":"en_US","status":"APPROVED","category":"UTILITY","quality_score":{"score":"GREEN"},"components":[{"type":"HEADER","format":"TEXT","text":"Order {{1}}","example":{"header_text":["SO001"]}},{"type":"BODY","text":"Hello {{1}}, open {{2}}","example":{"body_text":[["Ada","https://example.com/order"]]}},{"type":"FOOTER","text":"Thanks"},{"type":"BUTTONS","buttons":[{"type":"URL","text":"Open","url":"https://example.com/order/{{1}}","example":["ABC"]},{"type":"PHONE_NUMBER","text":"Call","phone_number":"+97317000000"},{"type":"QUICK_REPLY","text":"Confirm"}]}]},{"id":"102","name":"promo","language":"en","status":"PENDING","category":"MARKETING","quality_score":{"score":"UNKNOWN"},"components":[{"type":"BODY","text":"Sale"}]}]}}}}`, accountID)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/whatsapp.account/button_sync_whatsapp_account_templates", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("sync response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["tag"] != "display_notification" {
		t.Fatalf("sync action = %+v", action)
	}
	templates := readWhatsAppTemplatesByUIDForTest(t, env, accountID)
	order := templates["101"]
	promo := templates["102"]
	if order == nil || promo == nil {
		t.Fatalf("templates = %+v", templates)
	}
	if order["name"] != "Order Update" || order["template_name"] != "order_update" || order["status"] != "approved" || order["quality"] != "green" || order["template_type"] != "utility" || order["lang_code"] != "en_US" {
		t.Fatalf("order template = %+v", order)
	}
	if order["header_type"] != "text" || order["header_text"] != "Order {{1}}" || order["body"] != "Hello {{1}}, open {{2}}" || order["footer_text"] != "Thanks" {
		t.Fatalf("order content = %+v", order)
	}
	if promo["quality"] != "none" || promo["status"] != "pending" || promo["body"] != "Sale" {
		t.Fatalf("promo template = %+v", promo)
	}
	orderID := int64Value(order["id"])
	buttonRows := readWhatsAppTemplateButtonsForTest(t, env, orderID)
	if len(buttonRows) != 3 {
		t.Fatalf("buttons = %+v", buttonRows)
	}
	if buttonRows[0]["name"] != "Open" || buttonRows[0]["button_type"] != "url" || buttonRows[0]["url_type"] != "dynamic" || buttonRows[0]["website_url"] != "https://example.com/order/" {
		t.Fatalf("url button = %+v", buttonRows[0])
	}
	if buttonRows[1]["button_type"] != "phone_number" || buttonRows[1]["call_number"] != "+97317000000" {
		t.Fatalf("phone button = %+v", buttonRows[1])
	}
	variableRows := readWhatsAppTemplateVariablesForTest(t, env, orderID)
	if len(variableRows) != 4 {
		t.Fatalf("variables = %+v", variableRows)
	}
	if !whatsAppVariableExistsForTest(variableRows, "header", "{{1}}", "SO001", 0) ||
		!whatsAppVariableExistsForTest(variableRows, "body", "{{1}}", "Ada", 0) ||
		!whatsAppVariableExistsForTest(variableRows, "body", "{{2}}", "https://example.com/order", 0) {
		t.Fatalf("template variables = %+v", variableRows)
	}
	if !whatsAppVariableExistsForTest(variableRows, "button", "Open", "ABC", int64Value(buttonRows[0]["id"])) {
		t.Fatalf("button variable = %+v buttons=%+v", variableRows, buttonRows)
	}
	accountRows, err := env.Model("whatsapp.account").Browse(accountID).Read("phone_number", "templates_count")
	if err != nil {
		t.Fatal(err)
	}
	if accountRows[0]["phone_number"] != "+973 1700 0000" || int64Value(accountRows[0]["templates_count"]) != 2 {
		t.Fatalf("account sync fields = %+v", accountRows[0])
	}
}

func TestCallButtonWhatsAppAccountSyncTemplatesScopesInactiveAndIdempotentUpdates(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	accountID, err := env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1"})
	if err != nil {
		t.Fatal(err)
	}
	otherAccountID, err := env.Model("whatsapp.account").Create(map[string]any{"name": "Other", "account_uid": "acct-2", "phone_uid": "phone-2"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Old", "template_name": "old", "wa_template_uid": "777", "wa_account_id": accountID, "active": false, "body": "Old {{1}}", "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Other", "template_name": "other", "wa_template_uid": "777", "wa_account_id": otherAccountID, "body": "Other", "status": "draft", "quality": "none", "template_type": "marketing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.variable").Create(map[string]any{"name": "{{1}}", "wa_template_id": templateID, "line_type": "body", "field_type": "field", "field_name": "name", "demo_value": "Old Demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Stale", "wa_template_id": templateID, "sequence": int64(0), "button_type": "quick_reply"}); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"args":[[%d]],"kwargs":{"whatsapp_template_response":{"data":[{"id":"777","name":"order_update","language":"en_US","status":"APPROVED","category":"UTILITY","quality_score":{"score":"YELLOW"},"components":[{"type":"BODY","text":"New {{1}}","example":{"body_text":[["New Demo"]]}},{"type":"BUTTONS","buttons":[{"type":"URL","text":"Open","url":"https://example.com/{{1}}","example":["slug"]}]}]}]}}}`, accountID)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/whatsapp.account/button_sync_whatsapp_account_templates", bytes.NewBufferString(payload)))
		if rec.Code != http.StatusOK {
			t.Fatalf("sync %d response %d %s", i, rec.Code, rec.Body.String())
		}
	}
	templates := readWhatsAppTemplatesByUIDForTest(t, env, accountID)
	updated := templates["777"]
	if updated == nil || int64Value(updated["id"]) != templateID {
		t.Fatalf("updated template = %+v id=%d", updated, templateID)
	}
	if updated["active"] != false || updated["body"] != "New {{1}}" || updated["status"] != "approved" || updated["quality"] != "yellow" || updated["template_type"] != "utility" {
		t.Fatalf("updated fields = %+v", updated)
	}
	otherTemplates := readWhatsAppTemplatesByUIDForTest(t, env, otherAccountID)
	if otherTemplates["777"]["body"] != "Other" {
		t.Fatalf("other account changed = %+v", otherTemplates["777"])
	}
	buttonRows := readWhatsAppTemplateButtonsForTest(t, env, templateID)
	if len(buttonRows) != 1 || buttonRows[0]["name"] != "Open" || buttonRows[0]["button_type"] != "url" {
		t.Fatalf("buttons after idempotent sync = %+v", buttonRows)
	}
	variableRows := readWhatsAppTemplateVariablesForTest(t, env, templateID)
	if len(variableRows) != 2 {
		t.Fatalf("variables after idempotent sync = %+v", variableRows)
	}
	if !whatsAppVariableExistsForTest(variableRows, "body", "{{1}}", "Old Demo", 0) {
		t.Fatalf("body variable metadata not preserved = %+v", variableRows)
	}
	if !whatsAppVariableExistsForTest(variableRows, "button", "Open", "slug", int64Value(buttonRows[0]["id"])) {
		t.Fatalf("button variable not recreated = %+v", variableRows)
	}
}

func TestCallButtonWhatsAppAccountSyncTemplatesPreservesTrackedURLButtons(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	accountID, err := env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Tracked", "template_name": "tracked", "wa_template_uid": "778", "wa_account_id": accountID, "body": "Old", "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("whatsapp.template.button").Create(map[string]any{"name": "Tracked", "wa_template_id": templateID, "sequence": int64(0), "button_type": "url", "url_type": "tracked", "website_url": "https://landing.example/tracked"}); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"args":[[%d]],"kwargs":{"whatsapp_template_response":{"data":[{"id":"778","name":"tracked","language":"en","status":"APPROVED","category":"MARKETING","components":[{"type":"BODY","text":"New"},{"type":"BUTTONS","buttons":[{"type":"URL","text":"Open","url":"https://gorp.example/{{1}}","example":["https://gorp.example/???"]}]}]}]}}}`, accountID)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/whatsapp.account/button_sync_whatsapp_account_templates", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("tracked sync response %d %s", rec.Code, rec.Body.String())
	}
	buttonRows := readWhatsAppTemplateButtonsForTest(t, env, templateID)
	if len(buttonRows) != 1 || buttonRows[0]["name"] != "Open" || buttonRows[0]["button_type"] != "url" || buttonRows[0]["url_type"] != "tracked" || buttonRows[0]["website_url"] != "https://landing.example/tracked" {
		t.Fatalf("tracked button after sync = %+v", buttonRows)
	}
}

func TestCallKWWhatsAppTemplateSyncSingleTemplate(t *testing.T) {
	registry := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	server := Server{Env: env}
	accountID, err := env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := env.Model("whatsapp.template").Create(map[string]any{"name": "Draft", "template_name": "draft", "wa_template_uid": "333", "wa_account_id": accountID, "body": "Old", "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"args":[[%d],{"id":"333","name":"utility_notice","language":"en","status":"APPROVED","category":"UTILITY","quality_score":{"score":"GREEN"},"components":[{"type":"BODY","text":"Synced"}]}]}`, templateID)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/whatsapp.template/button_sync_template", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("single sync response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["tag"] != "reload" {
		t.Fatalf("single sync action = %+v", action)
	}
	rows, err := env.Model("whatsapp.template").Browse(templateID).Read("body", "status", "quality", "template_type")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["body"] != "Synced" || rows[0]["status"] != "approved" || rows[0]["quality"] != "green" || rows[0]["template_type"] != "utility" {
		t.Fatalf("single sync row = %+v", rows[0])
	}
}

func readWhatsAppTemplatesByUIDForTest(t *testing.T, env *record.Env, accountID int64) map[string]map[string]any {
	t.Helper()
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	found, err := env.WithContext(ctx).Model("whatsapp.template").Search(domain.Cond("wa_account_id", domain.Equal, accountID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "name", "template_name", "active", "wa_template_uid", "wa_account_id", "lang_code", "template_type", "status", "quality", "header_type", "header_text", "footer_text", "body")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, row := range rows {
		out[stringValue(row["wa_template_uid"])] = row
	}
	return out
}

func readWhatsAppTemplateButtonsForTest(t *testing.T, env *record.Env, templateID int64) []map[string]any {
	t.Helper()
	found, err := env.Model("whatsapp.template.button").Search(whatsappTemplateButtonDomainHTTP(env, templateID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "name", "sequence", "button_type", "url_type", "website_url", "call_number")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(rows, func(i, j int) bool {
		return int64Value(rows[i]["sequence"]) < int64Value(rows[j]["sequence"])
	})
	return rows
}

func readWhatsAppTemplateVariablesForTest(t *testing.T, env *record.Env, templateID int64) []map[string]any {
	t.Helper()
	found, err := env.Model("whatsapp.template.variable").Search(domain.Cond("wa_template_id", domain.Equal, templateID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id", "name", "line_type", "button_id", "field_type", "field_name", "demo_value")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(rows, func(i, j int) bool {
		left := stringValue(rows[i]["line_type"]) + ":" + stringValue(rows[i]["name"])
		right := stringValue(rows[j]["line_type"]) + ":" + stringValue(rows[j]["name"])
		return left < right
	})
	return rows
}

func whatsAppVariableExistsForTest(rows []map[string]any, lineType string, name string, demoValue string, buttonID int64) bool {
	for _, row := range rows {
		if row["line_type"] == lineType && row["name"] == name && stringValue(row["demo_value"]) == demoValue && int64Value(row["button_id"]) == buttonID {
			return true
		}
	}
	return false
}

func TestCallKWSequenceNextByCodeReturnsFalseWhenMissing(t *testing.T) {
	server := testSequenceServer(t, 1)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.sequence/next_by_code", bytes.NewBufferString(`{"args":["missing.code"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("next_by_code missing response %d %s", rec.Code, rec.Body.String())
	}
	var value bool
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value {
		t.Fatalf("missing next_by_code = %v", value)
	}
}

func TestCallKWSequenceNextByCodeUsesAllowedCompanyContext(t *testing.T) {
	server := testSequenceServer(t, 1)
	if _, err := server.Env.Model("ir.sequence").Create(map[string]any{
		"name":             "Global",
		"code":             "purchase.order",
		"prefix":           "G/",
		"padding":          int64(2),
		"number_next":      int64(3),
		"number_increment": int64(1),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.sequence").Create(map[string]any{
		"name":             "Company 2",
		"code":             "purchase.order",
		"prefix":           "C2/",
		"padding":          int64(2),
		"number_next":      int64(7),
		"number_increment": int64(1),
		"company_id":       int64(2),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"model":"ir.sequence","method":"next_by_code","args":["purchase.order"],"kwargs":{"context":{"allowed_company_ids":[2,1]}}}`)
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("next_by_code context response %d %s", rec.Code, rec.Body.String())
	}
	var value string
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value != "C2/07" {
		t.Fatalf("context next_by_code value = %q", value)
	}
}

func TestCallKWMessagePostCreatesThreadMessage(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	recipientID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "ping.txt",
		"res_model": "res.partner",
		"res_id":    partnerID,
		"type":      "binary",
		"datas":     "cGluZw==",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"res.partner","method":"message_post","args":[[%d]],"kwargs":{"body":"<b>Ping</b>","message_type":"comment","partner_ids":[%d],"attachment_ids":[%d],"tracking_value_ids":[[0,0,{"field_name":"name","field_desc":"Name","field_type":"char","old_value_char":"Old","new_value_char":"New"}]],"context":{"mail_post_autofollow":true}}}`, partnerID, recipientID, attachmentID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/message_post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("message_post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["id"])
	rows, err := server.Env.Model("mail.message").Browse(messageID).Read("body", "message_type", "model", "res_id", "partner_ids", "attachment_ids", "tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["body"] != "&lt;b&gt;Ping&lt;/b&gt;" || rows[0]["message_type"] != "comment" || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != partnerID {
		t.Fatalf("message rows = %+v", rows)
	}
	if got := rows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("message attachments = %#v", rows[0]["attachment_ids"])
	}
	trackingIDs := rows[0]["tracking_value_ids"].([]int64)
	if len(trackingIDs) != 1 {
		t.Fatalf("message tracking ids = %#v", trackingIDs)
	}
	trackingRows, err := server.Env.Model("mail.tracking.value").Browse(trackingIDs...).Read("field_name", "field_desc", "field_type", "old_value_char", "new_value_char", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(trackingRows) != 1 || trackingRows[0]["field_name"] != "name" || trackingRows[0]["field_desc"] != "Name" || trackingRows[0]["field_type"] != "char" || trackingRows[0]["old_value_char"] != "Old" || trackingRows[0]["new_value_char"] != "New" || trackingRows[0]["mail_message_id"] != messageID {
		t.Fatalf("tracking rows = %+v", trackingRows)
	}
	followers, err := server.Env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", "res.partner"),
		domain.Cond("res_id", "=", partnerID),
		domain.Cond("partner_id", "=", recipientID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if followers.Len() != 1 {
		t.Fatalf("followers = %d", followers.Len())
	}
}

func TestCallKWMailMessageCreateCreatesTrackingValues(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"mail.message","method":"create","values":{"body":"Tracked","message_type":"notification","model":"res.partner","res_id":%d,"tracking_value_ids":[[0,0,{"field_name":"name","field_desc":"Name","field_type":"char","old_value_char":"Old","new_value_char":"New"}]]}}`, partnerID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/create", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail.message create response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["id"])
	if messageID == 0 {
		t.Fatalf("message payload = %+v", payload)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(messageID).Read("tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	trackingIDs := messageRows[0]["tracking_value_ids"].([]int64)
	if len(trackingIDs) != 1 {
		t.Fatalf("message tracking ids = %#v", trackingIDs)
	}
	trackingRows, err := server.Env.Model("mail.tracking.value").Browse(trackingIDs...).Read("field_name", "field_desc", "field_type", "old_value_char", "new_value_char", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(trackingRows) != 1 || trackingRows[0]["field_name"] != "name" || trackingRows[0]["field_desc"] != "Name" || trackingRows[0]["field_type"] != "char" || trackingRows[0]["old_value_char"] != "Old" || trackingRows[0]["new_value_char"] != "New" || trackingRows[0]["mail_message_id"] != messageID {
		t.Fatalf("tracking rows = %+v", trackingRows)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"mail.message","method":"write","args":[[%d],{"body":"Direct write remains allowed"}]}`, messageID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/write", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail.message direct write response %d %s", rec.Code, rec.Body.String())
	}
}

func TestMailMessageUpdateContentRoute(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Author", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Other", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false, "partner_id": authorPartnerID}); err != nil {
		t.Fatal(err)
	}
	otherUserID, err := server.Env.Model("res.users").Create(map[string]any{"login": "other", "name": "Other", "active": true, "partner_id": otherPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := server.Env.Model("res.users").Create(map[string]any{"login": "author", "name": "Author", "active": true, "partner_id": authorPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Old</p>",
		"message_type": "comment",
		"model":        "res.partner",
		"res_id":       partnerID,
		"author_id":    authorPartnerID,
		"body_is_html": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedServer := server
	unauthorizedServer.Env = server.Env.WithContext(record.Context{UserID: otherUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Blocked</p>"}}`, messageID))
	unauthorizedServer.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized update content response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["body"] != "<p>Old</p>" {
		t.Fatalf("unauthorized update changed body = %+v", rows[0])
	}
	authorServer := server
	authorServer.Env = server.Env.WithContext(record.Context{UserID: authorUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Author</p>"}}`, messageID))
	authorServer.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("author update content response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":null}}`, messageID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("null body update content response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>Author</p>") {
		t.Fatalf("null body update changed body = %s", body)
	}
	routeAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":         "route.txt",
		"res_model":    "mail.compose.message",
		"res_id":       int64(1),
		"type":         "binary",
		"datas":        "cm91dGU=",
		"access_token": "route-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"attachment_ids":[],"attachment_tokens":["extra"]}}`, messageID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("attachment token count response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"attachment_ids":[%d]}}`, messageID, routeAttachmentID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("attachment token update response %d %s", rec.Code, rec.Body.String())
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Browse(routeAttachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "res.partner" || attachmentRows[0]["res_id"] != partnerID {
		t.Fatalf("route attachment = %+v", attachmentRows[0])
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>New</p>","partner_ids":[%d]}}`, messageID, partnerID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("message update content response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("mail.message").Browse(messageID).Read("body", "partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>New</p>") || !strings.Contains(body, "o-mail-Message-edited") {
		t.Fatalf("updated body = %s", body)
	}
	if got := rows[0]["partner_ids"].([]int64); len(got) != 1 || got[0] != partnerID {
		t.Fatalf("updated partners = %#v", rows[0]["partner_ids"])
	}
	trackedID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "Tracked",
		"message_type": "comment",
		"model":        "res.partner",
		"res_id":       partnerID,
		"tracking_value_ids": []any{[]any{float64(0), float64(0), map[string]any{
			"field_name": "name",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"Blocked"}}`, trackedID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tracked update content response %d %s", rec.Code, rec.Body.String())
	}
	notificationID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "System",
		"message_type": "notification",
		"model":        "res.partner",
		"res_id":       partnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"Blocked"}}`, notificationID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("notification update content response %d %s", rec.Code, rec.Body.String())
	}
}

func TestMailMessageUpdateContentRouteAllowsGuestCookieAuthor(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	guestID, err := server.Env.Model("mail.guest").Create(map[string]any{"name": "Guest", "email": "guest@example.test", "access_token": "guest-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":            "<p>Guest</p>",
		"message_type":    "comment",
		"model":           "res.partner",
		"res_id":          partnerID,
		"author_guest_id": guestID,
		"body_is_html":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Blocked</p>"}}`, messageID))
	badReq := httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body)
	badReq.AddCookie(&http.Cookie{Name: "dgid", Value: fmt.Sprintf("%d|wrong", guestID)})
	handler.ServeHTTP(rec, badReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad guest update response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Guest edit</p>"}}`, messageID))
	req := httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body)
	req.AddCookie(&http.Cookie{Name: "dgid", Value: fmt.Sprintf("%d|guest-token", guestID)})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("guest update response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>Guest edit</p>") || !strings.Contains(body, "o-mail-Message-edited") {
		t.Fatalf("guest updated body = %s", body)
	}
}

func TestMailMessageUpdateContentRouteAllowsDiscussTrackedComment(t *testing.T) {
	server := testMailThreadServer(t)
	channelID, err := server.Env.Model("discuss.channel").Create(map[string]any{"name": "Discuss", "channel_type": "channel"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Old</p>",
		"message_type": "comment",
		"model":        "discuss.channel",
		"res_id":       channelID,
		"body_is_html": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	trackingID, err := server.Env.Model("mail.tracking.value").Create(map[string]any{
		"field_name":      "name",
		"field_desc":      "Name",
		"field_type":      "char",
		"old_value_char":  "Old",
		"new_value_char":  "New",
		"mail_message_id": messageID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("mail.message").Browse(messageID).Write(map[string]any{"tracking_value_ids": []int64{trackingID}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Discuss route</p>"}}`, messageID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("discuss tracked update response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mail.message").Browse(messageID).Read("body", "tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	if body := rows[0]["body"].(string); !strings.Contains(body, "<p>Discuss route</p>") || !strings.Contains(body, "o-mail-Message-edited") {
		t.Fatalf("discuss route body = %s", body)
	}
	if got := rows[0]["tracking_value_ids"].([]int64); len(got) != 1 || got[0] != trackingID {
		t.Fatalf("discuss route tracking ids = %#v", got)
	}
}

func TestMailMessagePostRouteAttachmentTokens(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false}); err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "poster", "name": "Poster", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("res.users").Browse(userID).Write(map[string]any{"groups_id": []int64{groupID}}); err != nil {
		t.Fatal(err)
	}
	secret := "mail-post-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":         "post-route.txt",
		"res_model":    "mail.compose.message",
		"res_id":       int64(7),
		"type":         "binary",
		"datas":        "cm91dGU=",
		"access_token": "stored-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Groups[groupID] = security.Group{ID: groupID, Name: "Internal"}
	engine.Users[userID] = security.User{ID: userID, Login: "poster", Active: true, GroupIDs: []int64{groupID}}
	engine.ACLs = []security.ACL{
		{Model: "res.partner", GroupID: groupID, Active: true, PermRead: true},
		{Model: "mail.message", GroupID: groupID, Active: true, PermCreate: true, PermRead: true},
	}
	server.Env.WithPolicy(engine)
	server.Env = server.Env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Bad</p>","attachment_ids":[%d],"attachment_tokens":["stored-token"]}}`, partnerID, attachmentID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stored token post response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Bad</p>","attachment_ids":[],"attachment_tokens":["extra"]}}`, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("post token count response %d %s", rec.Code, rec.Body.String())
	}

	token := testAttachmentOwnershipToken(secret, attachmentID, time.Now().Add(time.Hour))
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Good</p>","attachment_ids":[%d],"attachment_tokens":["%s"]}}`, partnerID, attachmentID, token))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	if messageID == 0 {
		t.Fatalf("message payload = %+v", payload)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("post route message attachments = %#v", got)
	}
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	attachmentRows, err := systemEnv.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "res.partner" || attachmentRows[0]["res_id"] != partnerID {
		t.Fatalf("post route attachment = %+v", attachmentRows[0])
	}
}

func TestMailAttachmentUploadPortalPendingOwnership(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "upload-secret"}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartUploadRequest(t, "/mail/attachment/upload", map[string]string{
		"thread_model": "portal.thread",
		"thread_id":    fmt.Sprint(threadID),
		"token":        "bad",
		"is_pending":   "true",
	}, "ufile", "portal.txt", "hello"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad upload response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartUploadRequest(t, "/mail/attachment/upload", map[string]string{
		"thread_model": "portal.thread",
		"thread_id":    fmt.Sprint(threadID),
		"token":        "thread-token",
		"is_pending":   "true",
	}, "ufile", "portal.txt", "hello"))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	data := payload["data"].(map[string]any)
	attachmentID := int64Value(data["attachment_id"])
	store := data["store_data"].(map[string]any)
	attachments := store["ir.attachment"].([]any)
	if attachmentID == 0 || len(attachments) != 1 {
		t.Fatalf("upload payload = %+v", payload)
	}
	attachment := attachments[0].(map[string]any)
	ownershipToken := stringValue(attachment["ownership_token"])
	if attachment["name"] != "portal.txt" || attachment["res_model"] != "mail.compose.message" || int64Value(attachment["res_id"]) != 0 || ownershipToken == "" || stringValue(attachment["raw_access_token"]) == "" || stringValue(attachment["thumbnail_access_token"]) == "" {
		t.Fatalf("upload attachment payload = %+v", attachment)
	}
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	attachmentRows, err := systemEnv.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "mail.compose.message" || attachmentRows[0]["res_id"] != int64(0) {
		t.Fatalf("pending attachment row = %+v", attachmentRows[0])
	}

	rec = httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>Portal</p>","attachment_ids":[%d],"attachment_tokens":["%s"]},"token":"thread-token"}`, threadID, attachmentID, ownershipToken))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("post uploaded attachment response %d %s", rec.Code, rec.Body.String())
	}
	messagePayload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(messagePayload["message_id"])
	messageRows, err := systemEnv.Model("mail.message").Browse(messageID).Read("author_id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["author_id"] != partnerID {
		t.Fatalf("posted upload message = %+v", messageRows)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("posted upload attachments = %#v", messageRows[0]["attachment_ids"])
	}
	attachmentRows, err = systemEnv.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "portal.thread" || attachmentRows[0]["res_id"] != threadID {
		t.Fatalf("linked uploaded attachment row = %+v", attachmentRows[0])
	}
}

func TestMailAttachmentDeleteRouteOwnershipAndBus(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Owner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Attachment</p>", "message_type": "comment", "model": "res.partner", "res_id": partnerID, "author_id": partnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{"name": "owned.txt", "res_model": "mail.compose.message", "res_id": int64(0), "type": "binary", "datas": "owned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("mail.message").Browse(messageID).Write(map[string]any{"attachment_ids": []int64{attachmentID}}); err != nil {
		t.Fatal(err)
	}
	secret := "delete-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.users").Create(map[string]any{"login": "root", "name": "Root", "active": false}); err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Users[userID] = security.User{ID: userID, Login: "owner", Active: true}
	server.Env = server.Env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})
	server.Env.WithPolicy(engine)
	server.Bus = notifications.NewBus(100)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"params":{"attachment_id":%d,"access_token":"bad"}}`, attachmentID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/attachment/delete", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad delete response %d %s", rec.Code, rec.Body.String())
	}
	if rows, err := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}}).Model("ir.attachment").Browse(attachmentID).Read("id"); err != nil || len(rows) != 1 {
		t.Fatalf("attachment after bad delete rows=%+v err=%v", rows, err)
	}

	server.Bus = notifications.NewBus(100)
	handler = server.Handler()
	sub := server.Bus.Subscribe(userBusChannel(userID), 0)
	defer sub.Close()
	token := testAttachmentOwnershipToken(secret, attachmentID, time.Now().Add(time.Hour))
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"params":{"attachment_id":%d,"access_token":"%s"}}`, attachmentID, token))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/attachment/delete", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["result"] != nil {
		t.Fatalf("delete payload = %+v", payload)
	}
	event := nextHTTPBusEvent(t, sub)
	if event.Name != "ir.attachment/delete" || int64Value(event.Payload["id"]) != attachmentID {
		t.Fatalf("delete bus event = %+v", event)
	}
	message := event.Payload["message"].(map[string]any)
	if int64Value(message["id"]) != messageID {
		t.Fatalf("delete bus message = %+v", event.Payload)
	}
	rows, err := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}}).Model("ir.attachment").Browse(attachmentID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("attachment still exists = %+v", rows)
	}
}

func TestMailMessagePostRoutePortalHashSetsPublicAuthor(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>Bad</p>"},"token":"bad"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad portal post response %d %s", rec.Code, rec.Body.String())
	}

	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>Portal</p>"},"hash":"%s","pid":%d}`, threadID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != threadID {
		t.Fatalf("portal post message = %+v", rows)
	}
}

func TestSecurityPublicMailRoutesUseAnonymousContext(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Secured Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "security-public-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	engine := security.NewEngine()
	server.Security = engine
	server.Env.WithPolicy(engine)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("session info response %d %s", rec.Code, rec.Body.String())
	}
	sessionPayload := decodeJSON(t, rec.Body.Bytes())
	if sessionPayload["uid"] != float64(0) {
		t.Fatalf("anonymous session payload = %+v", sessionPayload)
	}

	rec = httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>No token</p>"}}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous portal post response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/view?model=portal.thread&res_id=%d", threadID), nil))
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/web/login?") {
		t.Fatalf("anonymous mail view redirect %d %s", rec.Code, rec.Header().Get("Location"))
	}

	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>Portal</p>"},"hash":"%s","pid":%d}`, threadID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal token post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	rows, err := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}}).Model("mail.message").Browse(messageID).Read("author_id", "body")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || !strings.Contains(stringValue(rows[0]["body"]), "Portal") {
		t.Fatalf("portal message row = %+v", rows)
	}
}

func TestMailMessagePostRoutePortalParentHashSetsPublicAuthor(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Parent Thread", "partner_id": partnerID, "access_token": "parent-token"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Child Thread", "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-parent-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()
	hash := testPortalThreadHash(secret, "demo", "parent-token", partnerID)

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"post_data":{"body":"<p>Parent</p>"},"hash":"%s","pid":%d}`, childID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal parent post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != childID {
		t.Fatalf("portal parent post message = %+v", rows)
	}
}

func TestPortalChatterRoutesProjectTaskParentHash(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := server.Env.Model("project.project").Create(map[string]any{"name": "Project", "partner_id": partnerID, "access_token": "project-token"})
	if err != nil {
		t.Fatal(err)
	}
	parentTaskID, err := server.Env.Model("project.task").Create(map[string]any{"name": "Parent Task", "project_id": projectID, "partner_id": partnerID, "access_token": "parent-task-token"})
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := server.Env.Model("project.task").Create(map[string]any{"name": "Task", "project_id": projectID, "parent_id": parentTaskID, "partner_id": partnerID, "access_token": "task-token"})
	if err != nil {
		t.Fatal(err)
	}
	commentSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Comment", "default": true, "internal": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "mail", "name": "mt_comment", "model": "mail.message.subtype", "res_id": commentSubtypeID}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Task Visible</p>", "message_type": "comment", "model": "project.task", "res_id": taskID, "author_id": partnerID, "subtype_id": commentSubtypeID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	secret := "project-parent-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()
	parentTaskHash := testPortalThreadHash(secret, "demo", "parent-task-token", partnerID)

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"hash":"%s","pid":%d}`, taskID, parentTaskHash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("bad project parent init response %d %s", rec.Code, rec.Body.String())
	}
	if payload := decodeJSON(t, rec.Body.Bytes()); len(payload) != 0 {
		t.Fatalf("same-model task parent hash init payload = %+v", payload)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"fetch_params":{"limit":10},"hash":"%s","pid":%d}`, taskID, parentTaskHash, partnerID))))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("same-model task parent hash fetch response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"token":"project-token"}`, taskID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("project token init response %d %s", rec.Code, rec.Body.String())
	}
	if payload := decodeJSON(t, rec.Body.Bytes()); len(payload) != 0 {
		t.Fatalf("project token without sharing init payload = %+v", payload)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"token":"project-token","project_sharing_id":%d}`, taskID, projectID+999))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("wrong project sharing init response %d %s", rec.Code, rec.Body.String())
	}
	if payload := decodeJSON(t, rec.Body.Bytes()); len(payload) != 0 {
		t.Fatalf("wrong project sharing init payload = %+v", payload)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"token":"project-token","project_sharing_id":%d}`, taskID, projectID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("project sharing init response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	threads := payload["mail.thread"].([]any)
	if len(threads) != 1 || threads[0].(map[string]any)["model"] != "project.task" || int64Value(threads[0].(map[string]any)["id"]) != taskID || int64Value(threads[0].(map[string]any)["portal_partner"]) != partnerID {
		t.Fatalf("project sharing init payload = %+v", payload)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"post_data":{"body":"<p>Sharing Post</p>"},"token":"project-token","project_sharing_id":%d}`, taskID, projectID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("project sharing post response %d %s", rec.Code, rec.Body.String())
	}
	postPayload := decodeJSON(t, rec.Body.Bytes())
	postedID := int64Value(postPayload["message_id"])
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	rows, err := systemEnv.Model("mail.message").Browse(postedID).Read("author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["author_id"] != partnerID || rows[0]["model"] != "project.task" || rows[0]["res_id"] != taskID {
		t.Fatalf("project sharing post rows = %+v", rows)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Bad Sharing Update</p>"},"token":"project-token"}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("project token update response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Sharing Update</p>"},"token":"project-token","project_sharing_id":%d}`, messageID, projectID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("project sharing update response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = systemEnv.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0]["body"].(string), "<p>Sharing Update</p>") {
		t.Fatalf("project sharing update rows = %+v", rows)
	}

	projectHash := testPortalThreadHash(secret, "demo", "project-token", partnerID)
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"hash":"%s","pid":%d}`, taskID, projectHash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("project parent init response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	threads = payload["mail.thread"].([]any)
	if len(threads) != 1 || threads[0].(map[string]any)["model"] != "project.task" || int64Value(threads[0].(map[string]any)["id"]) != taskID || int64Value(threads[0].(map[string]any)["portal_partner"]) != partnerID {
		t.Fatalf("project parent init payload = %+v", payload)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"project.task","thread_id":%d,"fetch_params":{"limit":10},"hash":"%s","pid":%d}`, taskID, projectHash, partnerID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("project parent fetch response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	messages := payload["messages"].([]any)
	if len(messages) != 1 || int64Value(messages[0]) != messageID {
		t.Fatalf("project parent fetch messages = %+v", messages)
	}
}

func TestMailMessageUpdateContentRoutePortalTokenAndHash(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Other", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-update-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Old</p>",
		"message_type": "comment",
		"model":        "portal.thread",
		"res_id":       threadID,
		"author_id":    partnerID,
		"body_is_html": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherMessageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Other</p>",
		"message_type": "comment",
		"model":        "portal.thread",
		"res_id":       threadID,
		"author_id":    otherPartnerID,
		"body_is_html": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Bad</p>"},"token":"bad"}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad portal update response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Wrong</p>"},"token":"thread-token"}`, otherMessageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatched portal update response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Token</p>"},"token":"thread-token"}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("token portal update response %d %s", rec.Code, rec.Body.String())
	}

	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Hash</p>"},"hash":"%s","pid":%d}`, messageID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("hash portal update response %d %s", rec.Code, rec.Body.String())
	}
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	rows, err := systemEnv.Model("mail.message").Browse(messageID, otherMessageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rows[0]["body"].(string), "<p>Hash</p>") || strings.Contains(rows[1]["body"].(string), "<p>Wrong</p>") {
		t.Fatalf("portal update rows = %+v", rows)
	}
}

func TestMailMessageUpdateContentRoutePortalParentHash(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Parent Thread", "partner_id": partnerID, "access_token": "parent-token"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Child Thread", "parent_id": parentID})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-parent-update-route-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":         "<p>Old</p>",
		"message_type": "comment",
		"model":        "portal.thread",
		"res_id":       childID,
		"author_id":    partnerID,
		"body_is_html": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()
	hash := testPortalThreadHash(secret, "demo", "parent-token", partnerID)

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"update_data":{"body":"<p>Parent Hash</p>"},"hash":"%s","pid":%d}`, messageID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/update_content", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("parent hash portal update response %d %s", rec.Code, rec.Body.String())
	}
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rows[0]["body"].(string), "<p>Parent Hash</p>") {
		t.Fatalf("parent hash portal update rows = %+v", rows)
	}
}

func TestPortalChatterInitRoute(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "email": "portal@example.test", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-init-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"token":"bad"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("bad portal init response %d %s", rec.Code, rec.Body.String())
	}
	if payload := decodeJSON(t, rec.Body.Bytes()); len(payload) != 0 {
		t.Fatalf("bad portal init payload = %+v", payload)
	}

	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"hash":"%s","pid":%d}`, threadID, hash, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/portal/chatter_init", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal init response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	threads := payload["mail.thread"].([]any)
	if len(threads) != 1 {
		t.Fatalf("portal init threads = %+v", payload)
	}
	thread := threads[0].(map[string]any)
	if thread["model"] != "portal.thread" || int64Value(thread["id"]) != threadID || thread["can_react"] != true || thread["hasReadAccess"] != true || int64Value(thread["portal_partner"]) != partnerID {
		t.Fatalf("portal init thread = %+v", thread)
	}
	partners := payload["res.partner"].([]any)
	if len(partners) != 1 || int64Value(partners[0].(map[string]any)["id"]) != partnerID {
		t.Fatalf("portal init partners = %+v", partners)
	}
}

func TestMailChatterFetchRoutePortalFiltersAndAccess(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	commentSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Comment", "default": true, "internal": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "mail", "name": "mt_comment", "model": "mail.message.subtype", "res_id": commentSubtypeID}); err != nil {
		t.Fatal(err)
	}
	noteSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Note", "default": true, "internal": true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	visibleID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Visible</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "date": now, "subtype_id": commentSubtypeID, "body_is_html": true, "is_internal": false, "starred_partner_ids": []int64{partnerID}})
	if err != nil {
		t.Fatal(err)
	}
	reactorID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Reactor", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	guestID, err := server.Env.Model("mail.guest").Create(map[string]any{"name": "Visitor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message.reaction").Create(map[string]any{"message_id": visibleID, "content": "ok", "partner_id": reactorID}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message.reaction").Create(map[string]any{"message_id": visibleID, "content": "ok", "guest_id": guestID}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{"name": "portal.txt", "res_model": "portal.thread", "res_id": threadID, "type": "binary", "datas": "cG9ydGFs", "mimetype": "text/plain", "access_token": "att-token", "checksum": "abc123", "has_thumbnail": true})
	if err != nil {
		t.Fatal(err)
	}
	attachmentMessageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "date": now.Add(time.Minute), "subtype_id": commentSubtypeID, "attachment_ids": []int64{attachmentID}, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Internal</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "subtype_id": noteSubtypeID, "body_is_html": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{"body": `<span class="o-mail-Message-edited"></span>`, "message_type": "comment", "model": "portal.thread", "res_id": threadID, "subtype_id": commentSubtypeID, "body_is_html": true}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":10},"token":"bad"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad chatter fetch response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":1},"token":"thread-token"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter fetch response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if _, ok := payload["count"]; ok {
		t.Fatalf("portal chatter count should be absent without search = %+v", payload)
	}
	messages := payload["messages"].([]any)
	if len(messages) != 1 || int64Value(messages[0]) != attachmentMessageID {
		t.Fatalf("portal chatter message ids = %+v", messages)
	}
	data := payload["data"].(map[string]any)
	messageRows := data["mail.message"].([]any)
	if len(messageRows) != 1 || int64Value(messageRows[0].(map[string]any)["id"]) != attachmentMessageID {
		t.Fatalf("portal chatter data = %+v", data)
	}
	attachmentMessage := messageRows[0].(map[string]any)
	if body := attachmentMessage["body"].([]any); len(body) != 2 || body[0] != "markup" || body[1] != "" {
		t.Fatalf("portal chatter attachment body = %+v", attachmentMessage["body"])
	}
	attachments := attachmentMessage["attachment_ids"].([]any)
	if len(attachments) != 1 {
		t.Fatalf("portal chatter attachments = %+v", attachmentMessage["attachment_ids"])
	}
	attachment := attachments[0].(map[string]any)
	if int64Value(attachment["id"]) != attachmentID || attachment["filename"] != "portal.txt" || attachment["mimetype"] != "text/plain" || stringValue(attachment["raw_access_token"]) == "" || stringValue(attachment["thumbnail_access_token"]) == "" || stringValue(attachment["ownership_token"]) == "" || attachment["checksum"] != "abc123" || attachment["has_thumbnail"] != true {
		t.Fatalf("portal chatter attachment payload = %+v", attachment)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":10,"before":%d},"token":"thread-token"}`, threadID, attachmentMessageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter before response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	messages = payload["messages"].([]any)
	if len(messages) != 1 || int64Value(messages[0]) != visibleID {
		t.Fatalf("portal chatter before ids = %+v", messages)
	}
	data = payload["data"].(map[string]any)
	messageRows = data["mail.message"].([]any)
	message := messageRows[0].(map[string]any)
	if body := message["body"].([]any); len(body) != 2 || body[0] != "markup" || body[1] != "<p>Visible</p>" {
		t.Fatalf("portal chatter body = %+v", message["body"])
	}
	if message["is_internal"] != false || message["is_message_subtype_note"] != false || message["published_date_str"] != "2026-01-02 03:04:00" || message["starred"] != true {
		t.Fatalf("portal chatter flags = %+v", message)
	}
	if message["author_guest_id"] != false {
		t.Fatalf("portal chatter guest author = %+v", message["author_guest_id"])
	}
	reactions := message["reactions"].([]any)
	if len(reactions) != 1 {
		t.Fatalf("portal chatter reactions = %+v", reactions)
	}
	reaction := reactions[0].(map[string]any)
	if reaction["content"] != "ok" || int64Value(reaction["count"]) != 2 || int64Value(reaction["message"]) != visibleID {
		t.Fatalf("portal chatter reaction = %+v", reaction)
	}
	reactionPartners := reaction["partners"].([]any)
	if len(reactionPartners) != 1 || int64Value(reactionPartners[0].(map[string]any)["id"]) != reactorID || reactionPartners[0].(map[string]any)["name"] != "Reactor" {
		t.Fatalf("portal chatter reaction partners = %+v", reactionPartners)
	}
	reactionGuests := reaction["guests"].([]any)
	if len(reactionGuests) != 1 || int64Value(reactionGuests[0].(map[string]any)["id"]) != guestID || reactionGuests[0].(map[string]any)["name"] != "Visitor" {
		t.Fatalf("portal chatter reaction guests = %+v", reactionGuests)
	}
	author := message["author_id"].(map[string]any)
	if int64Value(author["id"]) != partnerID || author["name"] != "Portal" {
		t.Fatalf("portal chatter author = %+v", author)
	}
	thread := message["thread"].(map[string]any)
	if thread["model"] != "portal.thread" || int64Value(thread["id"]) != threadID || thread["has_mail_thread"] != true {
		t.Fatalf("portal chatter thread = %+v", thread)
	}
	if avatar := message["author_avatar_url"].(string); !strings.Contains(avatar, fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50", visibleID)) || !strings.Contains(avatar, "access_token=thread-token") {
		t.Fatalf("portal chatter avatar = %s", avatar)
	}
}

func TestMailMessageAvatarRoutePortalAccess(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Hello</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-avatar-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50?access_token=bad", messageID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad avatar response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50?access_token=thread-token", messageID), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("token avatar response %d type=%s body=%x", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes())
	}

	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50?_hash=%s&pid=%d", messageID, hash, partnerID), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("hash avatar response %d type=%s body=%x", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes())
	}
}

func TestResPartnerSignupAuthParams(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "signup-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "auth_signup.invitation_scope", "value": "b2c"}); err != nil {
		t.Fatal(err)
	}
	signupPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Invitee", "email": "invitee@example.test", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Existing", "email": "existing@example.test", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.users").Create(map[string]any{"name": "Existing", "login": "existing@example.test", "partner_id": userPartnerID, "active": true}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"res.partner","method":"signup_get_auth_param","args":[[%d,%d]]}`, signupPartnerID, userPartnerID))
	payload := decodeJSON(t, []byte(body))
	signupParams := payload[fmt.Sprint(signupPartnerID)].(map[string]any)
	if token := stringValue(signupParams["auth_signup_token"]); token == "" {
		t.Fatalf("signup params = %+v", payload)
	}
	userParams := payload[fmt.Sprint(userPartnerID)].(map[string]any)
	if userParams["auth_login"] != "existing@example.test" {
		t.Fatalf("user params = %+v", payload)
	}
	rows, err := server.Env.Model("res.partner").Browse(signupPartnerID).Read("signup_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["signup_type"] != "signup" {
		t.Fatalf("signup type after auth params = %+v", rows)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"res.partner","method":"signup_prepare","args":[[%d],"reset"]}`, signupPartnerID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("signup_prepare response %s", body)
	}
	rows, err = server.Env.Model("res.partner").Browse(signupPartnerID).Read("signup_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["signup_type"] != "reset" {
		t.Fatalf("signup type after prepare = %+v", rows)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"res.partner","method":"signup_cancel","args":[[%d]]}`, signupPartnerID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("signup_cancel response %s", body)
	}
	rows, err = server.Env.Model("res.partner").Browse(signupPartnerID).Read("signup_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["signup_type"] != "" {
		t.Fatalf("signup type after cancel = %+v", rows)
	}
}

func TestMailViewRoutePortalRedirectAndAuthCookies(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "email": "portal@example.test", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Hello</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	secret := "portal-view-secret"
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": secret}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()
	hash := testPortalThreadHash(secret, "demo", "thread-token", partnerID)

	rec := httptest.NewRecorder()
	target := fmt.Sprintf("/mail/view?model=portal.thread&res_id=%d&access_token=thread-token&pid=%d&hash=%s&auth_signup_token=signup-token&auth_login=portal%%40example.test&ignored=drop&highlight_message_id=%d", threadID, partnerID, hash, messageID)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("mail view response %d %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != fmt.Sprintf("/odoo/portal.thread/%d", threadID) {
		t.Fatalf("mail view location = %s", location)
	}
	query := parsed.Query()
	if query.Get("pid") != fmt.Sprint(partnerID) || query.Get("hash") != hash || query.Get("highlight_message_id") != fmt.Sprint(messageID) {
		t.Fatalf("mail view redirect query = %s", location)
	}
	if strings.Contains(location, "ignored") || strings.Contains(location, "auth_signup_token") || strings.Contains(location, "auth_login") {
		t.Fatalf("mail view leaked params = %s", location)
	}
	cookies := map[string]string{}
	for _, cookie := range rec.Result().Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	if cookies["auth_signup_token"] != "signup-token" || cookies["auth_login"] != "portal@example.test" {
		t.Fatalf("auth cookies = %+v", cookies)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/view?model=portal.thread&res_id=%d&auth_login=portal%%40example.test&ignored=drop", threadID), nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("mail view login response %d %s", rec.Code, rec.Body.String())
	}
	loginLocation, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loginLocation.Path != "/web/login" {
		t.Fatalf("login location = %s", rec.Header().Get("Location"))
	}
	redirect := loginLocation.Query().Get("redirect")
	redirectURL, err := url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	if redirectURL.Path != "/mail/view" || redirectURL.Query().Get("auth_login") != "portal@example.test" || redirectURL.Query().Get("ignored") != "" {
		t.Fatalf("login redirect = %s", redirect)
	}
}

func TestMailMessageReactionRouteUserAndPortal(t *testing.T) {
	server := testMailThreadServer(t)
	userPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "User Partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"name": "User", "login": "user", "partner_id": userPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})
	server.Bus = notifications.NewBus(100)
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": userPartnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Hello</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": userPartnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	sub := server.Bus.Subscribe(userBusChannel(userID), 0)
	defer sub.Close()

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"content":"ok","action":"add"}`, messageID))
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/reaction", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("reaction add response %d %s", rec.Code, rec.Body.String())
		}
		payload := decodeJSON(t, rec.Body.Bytes())
		messages := payload["mail.message"].([]any)
		if len(messages) != 1 || int64Value(messages[0].(map[string]any)["id"]) != messageID {
			t.Fatalf("reaction message payload = %+v", payload)
		}
		reactionCommand := messages[0].(map[string]any)["reactions"].([]any)[0].([]any)
		if reactionCommand[0] != "ADD" {
			t.Fatalf("reaction add command = %+v", reactionCommand)
		}
		groups := payload["MessageReactions"].([]any)
		if len(groups) != 1 || groups[0].(map[string]any)["content"] != "ok" || int64Value(groups[0].(map[string]any)["count"]) != 1 {
			t.Fatalf("reaction groups = %+v", groups)
		}
		event := nextHTTPBusEvent(t, sub)
		if event.Name != "mail.record/insert" {
			t.Fatalf("reaction bus event name = %s", event.Name)
		}
		busGroups := event.Payload["MessageReactions"].([]map[string]any)
		if len(busGroups) != 1 || int64Value(busGroups[0]["message"]) != messageID || int64Value(busGroups[0]["count"]) != 1 {
			t.Fatalf("reaction bus payload = %+v", event.Payload)
		}
	}
	found, err := server.Env.Model("mail.message.reaction").Search(domain.And(
		domain.Cond("message_id", "=", messageID),
		domain.Cond("content", "=", "ok"),
	))
	if err != nil {
		t.Fatal(err)
	}
	reactionRows, err := found.Read("partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(reactionRows) != 1 || int64Value(reactionRows[0]["partner_id"]) != userPartnerID {
		t.Fatalf("reaction rows after duplicate add = %+v", reactionRows)
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"message_id":%d,"content":"ok","action":"remove"}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/reaction", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("reaction remove response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	command := payload["mail.message"].([]any)[0].(map[string]any)["reactions"].([]any)[0].([]any)
	if command[0] != "DELETE" {
		t.Fatalf("reaction delete command = %+v", command)
	}
	event := nextHTTPBusEvent(t, sub)
	if event.Name != "mail.record/insert" {
		t.Fatalf("reaction remove bus event name = %s", event.Name)
	}
	busMessageRows := event.Payload["mail.message"].([]map[string]any)
	if len(busMessageRows) != 1 || busMessageRows[0]["reactions"].([]any)[0].([]any)[0] != "DELETE" {
		t.Fatalf("reaction remove bus payload = %+v", event.Payload)
	}
	found, err = server.Env.Model("mail.message.reaction").Search(domain.And(
		domain.Cond("message_id", "=", messageID),
		domain.Cond("content", "=", "ok"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := found.Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("reaction rows after remove rows=%+v err=%v", rows, err)
	}

	portalPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	portalThreadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread 2", "partner_id": portalPartnerID, "access_token": "portal-token"})
	if err != nil {
		t.Fatal(err)
	}
	portalMessageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Portal</p>", "message_type": "comment", "model": "portal.thread", "res_id": portalThreadID, "author_id": portalPartnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}})
	handler = server.Handler()
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"params":{"message_id":%d,"content":"portal","action":"add","token":"portal-token"}}`, portalMessageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/reaction", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal reaction response %d %s", rec.Code, rec.Body.String())
	}
	envelope := decodeJSON(t, rec.Body.Bytes())
	result := envelope["result"].(map[string]any)
	groups := result["MessageReactions"].([]any)
	if len(groups) != 1 || int64Value(groups[0].(map[string]any)["count"]) != 1 {
		t.Fatalf("portal reaction groups = %+v", groups)
	}
	systemEnv := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	found, err = systemEnv.Model("mail.message.reaction").Search(domain.And(
		domain.Cond("message_id", "=", portalMessageID),
		domain.Cond("content", "=", "portal"),
	))
	if err != nil {
		t.Fatal(err)
	}
	reactionRows, err = found.Read("partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(reactionRows) != 1 || int64Value(reactionRows[0]["partner_id"]) != portalPartnerID {
		t.Fatalf("portal reaction rows = %+v", reactionRows)
	}
}

func TestMailMessageReactionConstraints(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	guestID, err := server.Env.Model("mail.guest").Create(map[string]any{"name": "Visitor"})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Hello</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	reactions := server.Env.Model("mail.message.reaction")
	partnerReactionID, err := reactions.Create(map[string]any{"message_id": messageID, "content": "ok", "partner_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	guestReactionID, err := reactions.Create(map[string]any{"message_id": messageID, "content": "ok", "guest_id": guestID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "ok", "partner_id": partnerID}); err == nil || !strings.Contains(err.Error(), "duplicate partner") {
		t.Fatalf("duplicate partner err = %v", err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "ok", "guest_id": guestID}); err == nil || !strings.Contains(err.Error(), "duplicate guest") {
		t.Fatalf("duplicate guest err = %v", err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "both", "partner_id": partnerID, "guest_id": guestID}); err == nil || !strings.Contains(err.Error(), "exactly one partner_id or guest_id") {
		t.Fatalf("both actors err = %v", err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "none"}); err == nil || !strings.Contains(err.Error(), "exactly one partner_id or guest_id") {
		t.Fatalf("missing actor err = %v", err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "other", "partner_id": partnerID}); err != nil {
		t.Fatalf("same partner different content err = %v", err)
	}
	if _, err := reactions.Create(map[string]any{"message_id": messageID, "content": "ok ", "partner_id": partnerID}); err != nil {
		t.Fatalf("same partner raw-distinct content err = %v", err)
	}
	if err := reactions.Browse(guestReactionID).Write(map[string]any{"guest_id": false, "partner_id": partnerID}); err == nil || !strings.Contains(err.Error(), "duplicate partner") {
		t.Fatalf("write duplicate err = %v", err)
	}
	rows, err := reactions.Browse(guestReactionID).Read("guest_id", "partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || int64Value(rows[0]["guest_id"]) != guestID || int64Value(rows[0]["partner_id"]) != 0 {
		t.Fatalf("guest reaction after failed write = %+v", rows)
	}
	if err := reactions.Browse(partnerReactionID).Write(map[string]any{"guest_id": guestID}); err == nil || !strings.Contains(err.Error(), "exactly one partner_id or guest_id") {
		t.Fatalf("write both actors err = %v", err)
	}
}

func TestLivechatCORSMessageReactionUsesGuestToken(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Livechat Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	guestID, err := server.Env.Model("mail.guest").Create(map[string]any{"name": "Visitor", "access_token": "guest-token"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":            "<p>Guest</p>",
		"message_type":    "comment",
		"model":           "res.partner",
		"res_id":          partnerID,
		"author_guest_id": guestID,
		"body_is_html":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Bus = notifications.NewBus(100)
	server.Env = server.Env.WithContext(record.Context{UserID: 9, CompanyID: 1, CompanyIDs: []int64{1}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"guest_token":"%d|bad","message_id":%d,"content":"ok","action":"add"}`, guestID, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/im_livechat/cors/message/reaction", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad livechat reaction response %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS header on bad response: %+v", rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "POST" || !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("bad CORS headers on bad response: %+v", rec.Header())
	}

	sub := server.Bus.Subscribe(guestBusChannel(guestID), 0)
	defer sub.Close()
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":20,"params":{"guest_token":"%d|guest-token","message_id":%d,"content":"ok","action":"add"}}`, guestID, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/im_livechat/cors/message/reaction", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("livechat reaction response %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS header: %+v", rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "POST" || !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "X-Requested-With") {
		t.Fatalf("bad CORS headers: %+v", rec.Header())
	}
	envelope := decodeJSON(t, rec.Body.Bytes())
	result := envelope["result"].(map[string]any)
	groups := result["MessageReactions"].([]any)
	if len(groups) != 1 || int64Value(groups[0].(map[string]any)["count"]) != 1 {
		t.Fatalf("livechat reaction groups = %+v", groups)
	}
	event := nextHTTPBusEvent(t, sub)
	if event.Name != "mail.record/insert" {
		t.Fatalf("livechat reaction bus event name = %s", event.Name)
	}
	found, err := server.Env.Model("mail.message.reaction").Search(domain.And(
		domain.Cond("message_id", "=", messageID),
		domain.Cond("content", "=", "ok"),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("guest_id", "partner_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || int64Value(rows[0]["guest_id"]) != guestID || int64Value(rows[0]["partner_id"]) != 0 {
		t.Fatalf("livechat reaction rows = %+v", rows)
	}
}

func TestMailStarredMessagesAndCallKWMethods(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "User Partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"name": "User", "login": "user", "partner_id": partnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Other User Partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := server.Env.Model("res.users").Create(map[string]any{"name": "Other User", "login": "other", "partner_id": otherPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})
	server.Bus = notifications.NewBus(100)
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Thread", "partner_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Star Alpha</p>", "subject": "Tracked", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "body_is_html": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Alpha unstarred</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "body_is_html": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Other starred</p>", "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": otherPartnerID, "body_is_html": true, "starred_partner_ids": []int64{otherPartnerID}}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	sub := server.Bus.Subscribe(userBusChannel(userID), 0)
	defer sub.Close()
	assertStarredStore := func(wantCounter int64, wantBusID int64) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/get_session_info", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("session info response %d %s", rec.Code, rec.Body.String())
		}
		payload := decodeJSON(t, rec.Body.Bytes())
		starredStore := mapValue(mapValue(payload["Store"])["starred"])
		if int64Value(starredStore["counter"]) != wantCounter || int64Value(starredStore["counter_bus_id"]) != wantBusID || stringValue(starredStore["id"]) != "starred" || stringValue(starredStore["model"]) != "mail.box" {
			t.Fatalf("session starred store = %+v", starredStore)
		}
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"params":{"args":[[%d]]}}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/toggle_message_starred", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle star response %d %s", rec.Code, rec.Body.String())
	}
	envelope := decodeJSON(t, rec.Body.Bytes())
	result := envelope["result"].(map[string]any)
	messages := result["mail.message"].([]any)
	if len(messages) != 1 || int64Value(messages[0].(map[string]any)["id"]) != messageID || messages[0].(map[string]any)["starred"] != true {
		t.Fatalf("toggle star payload = %+v", result)
	}
	event := nextHTTPBusEvent(t, sub)
	if event.Name != "mail.message/toggle_star" || event.Payload["starred"] != true {
		t.Fatalf("toggle star bus event = %+v", event)
	}
	if ids := event.Payload["message_ids"].([]int64); len(ids) != 1 || ids[0] != messageID {
		t.Fatalf("toggle star bus ids = %+v", event.Payload["message_ids"])
	}
	assertStarredStore(1, event.ID)

	server.Env = server.Env.WithContext(record.Context{UserID: otherUserID, CompanyID: 1, CompanyIDs: []int64{1}})
	handler = server.Handler()
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":10}}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/messages", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("other user thread messages response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	otherRows := payload["data"].(map[string]any)["mail.message"].([]any)
	var otherMessage map[string]any
	for _, item := range otherRows {
		row := item.(map[string]any)
		if int64Value(row["id"]) == messageID {
			otherMessage = row
			break
		}
	}
	if otherMessage == nil || otherMessage["starred"] != false {
		t.Fatalf("other user computed starred row = %+v", otherMessage)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"params":{"model":"mail.message","method":"read","args":[[%d],["starred"]]}}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/read", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("other user read response %d %s", rec.Code, rec.Body.String())
	}
	envelope = decodeJSON(t, rec.Body.Bytes())
	readRows := envelope["result"].([]any)
	if len(readRows) != 1 || readRows[0].(map[string]any)["starred"] != false {
		t.Fatalf("other user read rows = %+v", readRows)
	}
	if _, ok := readRows[0].(map[string]any)["starred_partner_ids"]; ok {
		t.Fatalf("internal starred partner field leaked in read result = %+v", readRows[0])
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"mail.message","method":"write","args":[[%d],{"starred":true}]}`, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/write", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("computed starred write response %d %s", rec.Code, rec.Body.String())
	}
	rawRows, err := server.Env.Model("mail.message").Browse(messageID).Read("starred")
	if err != nil {
		t.Fatal(err)
	}
	if rawRows[0]["starred"] == true {
		t.Fatalf("computed starred was persisted by write = %+v", rawRows[0])
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":3,"params":{"model":"mail.message","method":"search_count","args":[[["starred","=",true]]]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/search_count", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("other user search_count response %d %s", rec.Code, rec.Body.String())
	}
	envelope = decodeJSON(t, rec.Body.Bytes())
	if int64Value(envelope["result"]) != 1 {
		t.Fatalf("other user starred search_count = %+v", envelope)
	}

	server.Env = server.Env.WithContext(record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}})
	handler = server.Handler()
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":4,"params":{"model":"mail.message","method":"search_read","args":[[["starred","=",true]],["starred"]]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/search_read", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("current user search_read response %d %s", rec.Code, rec.Body.String())
	}
	envelope = decodeJSON(t, rec.Body.Bytes())
	searchRows := envelope["result"].([]any)
	if len(searchRows) != 1 || int64Value(searchRows[0].(map[string]any)["id"]) != messageID || searchRows[0].(map[string]any)["starred"] != true {
		t.Fatalf("current user starred search_read = %+v", searchRows)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"fetch_params":{"limit":10,"search_term":"Alpha"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/starred/messages", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("starred messages response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if int64Value(payload["count"]) != 1 {
		t.Fatalf("starred search count = %+v", payload)
	}
	starredIDs := payload["messages"].([]any)
	if len(starredIDs) != 1 || int64Value(starredIDs[0]) != messageID {
		t.Fatalf("starred ids = %+v", starredIDs)
	}
	data := payload["data"].(map[string]any)
	messageRows := data["mail.message"].([]any)
	if len(messageRows) != 1 {
		t.Fatalf("starred data rows = %+v", messageRows)
	}
	row := messageRows[0].(map[string]any)
	if row["starred"] != true {
		t.Fatalf("starred row = %+v", row)
	}
	if bodyValue := row["body"].([]any); len(bodyValue) != 2 || bodyValue[0] != "markup" || bodyValue[1] != "<p>Star Alpha</p>" {
		t.Fatalf("starred body = %+v", row["body"])
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/mail.message/unstar_all", bytes.NewBufferString(`{"args":[]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unstar all response %d %s", rec.Code, rec.Body.String())
	}
	event = nextHTTPBusEvent(t, sub)
	if event.Name != "mail.message/toggle_star" || event.Payload["starred"] != false {
		t.Fatalf("unstar all bus event = %+v", event)
	}
	if ids := event.Payload["message_ids"].([]int64); len(ids) != 1 || ids[0] != messageID {
		t.Fatalf("unstar all bus ids = %+v", event.Payload["message_ids"])
	}
	assertStarredStore(0, event.ID)
	rows, err := server.Env.Model("mail.message").Browse(messageID).Read("starred_partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(int64Slice(rows[0]["starred_partner_ids"])) != 0 {
		t.Fatalf("message after unstar all = %+v", rows[0])
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/starred/messages", bytes.NewBufferString(`{"fetch_params":{"limit":10}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("starred messages empty response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if ids := payload["messages"].([]any); len(ids) != 0 {
		t.Fatalf("starred ids after unstar = %+v", ids)
	}
}

func TestMailChatterFetchRouteAroundAfterSearchAndNotification(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Portal", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("portal.thread").Create(map[string]any{"name": "Portal Thread", "partner_id": partnerID, "access_token": "thread-token"})
	if err != nil {
		t.Fatal(err)
	}
	commentSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Comment", "default": true, "internal": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "mail", "name": "mt_comment", "model": "mail.message.subtype", "res_id": commentSubtypeID}); err != nil {
		t.Fatal(err)
	}
	messageIDs := make([]int64, 0, 5)
	for _, body := range []string{"<p>One Alpha</p>", "<p>Two</p>", "<p>Three Alpha</p>", "<p>Four</p>", "<p>Five</p>"} {
		id, err := server.Env.Model("mail.message").Create(map[string]any{"body": body, "message_type": "comment", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "subtype_id": commentSubtypeID, "body_is_html": true})
		if err != nil {
			t.Fatal(err)
		}
		messageIDs = append(messageIDs, id)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{"body": "<p>Notice</p>", "message_type": "notification", "model": "portal.thread", "res_id": threadID, "author_id": partnerID, "subtype_id": commentSubtypeID, "body_is_html": true}); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(security.NewEngine())
	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"db": "demo"}})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":2,"after":%d},"token":"thread-token"}`, threadID, messageIDs[1]))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter after response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messages := payload["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("portal chatter after ids = %+v", messages)
	}
	if got := []int64{int64Value(messages[0]), int64Value(messages[1])}; got[0] != messageIDs[3] || got[1] != messageIDs[2] {
		t.Fatalf("portal chatter after ids = %+v", messages)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":4,"around":%d},"token":"thread-token"}`, threadID, messageIDs[2]))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter around response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	messages = payload["messages"].([]any)
	wantAround := []int64{messageIDs[4], messageIDs[3], messageIDs[2], messageIDs[1]}
	if len(messages) != len(wantAround) {
		t.Fatalf("portal chatter around ids = %+v", messages)
	}
	for idx, want := range wantAround {
		if int64Value(messages[idx]) != want {
			t.Fatalf("portal chatter around ids = %+v want %+v", messages, wantAround)
		}
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":10,"search_term":"Alpha"},"token":"thread-token"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter search response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if int64Value(payload["count"]) != 2 {
		t.Fatalf("portal chatter search count = %+v", payload)
	}
	messages = payload["messages"].([]any)
	if len(messages) != 2 || int64Value(messages[0]) != messageIDs[2] || int64Value(messages[1]) != messageIDs[0] {
		t.Fatalf("portal chatter search ids = %+v", messages)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"portal.thread","thread_id":%d,"fetch_params":{"limit":10,"is_notification":true},"token":"thread-token"}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/chatter_fetch", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal chatter notification response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	messages = payload["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("portal chatter notification ids = %+v", messages)
	}
}

func TestMailUserRoutesRequireSecuritySessionCookie(t *testing.T) {
	server := testMailThreadServer(t)
	engine := security.NewEngine()
	engine.Users[9] = security.User{ID: 9, Login: "mail-user", Active: true, CompanyID: 1, CompanyIDs: []int64{1}}
	engine.ACLs = []security.ACL{{Model: "res.partner", Active: true, PermRead: true}}
	engine.IssueSession(9, "mail-sid", time.Now().Add(time.Hour))
	server.Security = engine
	handler := server.Handler()

	routes := []struct {
		target string
		body   string
	}{
		{"/mail/thread/messages", `{"thread_model":"portal.thread","thread_id":1,"fetch_params":{"limit":10}}`},
		{"/mail/starred/messages", `{"fetch_params":{"limit":10}}`},
		{"/mail/partner/from_email", `{"emails":["user@example.com"],"no_create":true}`},
	}
	for _, route := range routes {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, route.target, bytes.NewBufferString(route.body)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated response %d %s", route.target, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/mail/partner/from_email", bytes.NewBufferString(`{"emails":["user@example.com"],"no_create":true}`))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "mail-sid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated partner_from_email response %d %s", rec.Code, rec.Body.String())
	}
}

func TestMailRoutesPostFetchAndPartnerFromEmail(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "email": "thread@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Hello</p>","subject":"Route","partner_ids":[%d],"tracking_value_ids":[{"field_name":"email","field_desc":"Email","field_type":"char","old_value_char":"","new_value_char":"thread@example.com"}]},"context":{"mail_post_autofollow":true}}`, partnerID, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail message post response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	if messageID == 0 {
		t.Fatalf("message payload = %+v", payload)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(messageID).Read("tracking_value_ids")
	if err != nil {
		t.Fatal(err)
	}
	trackingIDs := messageRows[0]["tracking_value_ids"].([]int64)
	if len(trackingIDs) != 1 {
		t.Fatalf("route tracking ids = %#v", trackingIDs)
	}
	trackingRows, err := server.Env.Model("mail.tracking.value").Browse(trackingIDs...).Read("field_name", "new_value_char", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(trackingRows) != 1 || trackingRows[0]["field_name"] != "email" || trackingRows[0]["new_value_char"] != "thread@example.com" || trackingRows[0]["mail_message_id"] != messageID {
		t.Fatalf("route tracking rows = %+v", trackingRows)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"fetch_params":{"limit":10}}`, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/messages", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail thread messages response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if _, ok := payload["count"]; ok {
		t.Fatalf("thread messages count should be absent without search = %+v", payload)
	}
	data := payload["data"].(map[string]any)
	messageItems := data["mail.message"].([]any)
	if len(messageItems) != 1 {
		t.Fatalf("thread message items = %+v", messageItems)
	}
	messageItem := messageItems[0].(map[string]any)
	formattedTracking := messageItem["trackingValues"].([]any)
	if len(formattedTracking) != 1 {
		t.Fatalf("formatted tracking = %+v", formattedTracking)
	}
	trackingItem := formattedTracking[0].(map[string]any)
	fieldInfo := trackingItem["fieldInfo"].(map[string]any)
	if fieldInfo["changedField"] != "Email" || fieldInfo["fieldType"] != "char" || trackingItem["newValue"] != "thread@example.com" {
		t.Fatalf("formatted tracking item = %+v", trackingItem)
	}
	rawTracking := data["mail.tracking.value"].([]any)
	if len(rawTracking) != 1 || int64Value(rawTracking[0].(map[string]any)["mail_message_id"]) != messageID {
		t.Fatalf("raw tracking = %+v", rawTracking)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"emails":["New Person <new-person@example.com>"]}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/partner/from_email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("partner from email response %d %s", rec.Code, rec.Body.String())
	}
	var partners []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &partners); err != nil {
		t.Fatal(err)
	}
	if len(partners) != 1 || partners[0]["email"] != "new-person@example.com" {
		t.Fatalf("partners = %+v", partners)
	}
}

func TestMailPartnerFromEmailRequiresPartnerManagerToCreate(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Existing", "email": "existing-route@example.com", "email_normalized": "existing-route@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}

	nonManager := server
	nonManager.Env = server.Env.WithContext(record.Context{UserID: 20, CompanyID: 1, CompanyIDs: []int64{1}})
	body := bytes.NewBufferString(`{"emails":["Existing <existing-route@example.com>","Blocked <blocked-from-email@example.com>"]}`)
	rec := httptest.NewRecorder()
	nonManager.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/partner/from_email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("non-manager partner_from_email response %d %s", rec.Code, rec.Body.String())
	}
	var partners []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &partners); err != nil {
		t.Fatal(err)
	}
	if len(partners) != 1 || partners[0]["email"] != "existing-route@example.com" {
		t.Fatalf("non-manager partners = %+v", partners)
	}
	found, err := server.Env.Model("res.partner").Search(domain.Cond("email", "=", "blocked-from-email@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("non-manager created partner count = %d", found.Len())
	}

	body = bytes.NewBufferString(`{"emails":["Allowed <allowed-from-email@example.com>"]}`)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/partner/from_email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manager partner_from_email response %d %s", rec.Code, rec.Body.String())
	}
	found, err = server.Env.Model("res.partner").Search(domain.Cond("email", "=", "allowed-from-email@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("manager created partner count = %d", found.Len())
	}
}

func TestMailThreadRecipientsRoutesCreateReplyAllPartners(t *testing.T) {
	server := testMailThreadServer(t)
	threadID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Author", "email": "sender@example.com", "email_normalized": "sender@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	existingID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Existing", "email": "existing@example.com", "email_normalized": "existing@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	followerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Follower", "email": "follower@example.com", "email_normalized": "follower@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.followers").Create(map[string]any{"res_model": "res.partner", "res_id": threadID, "partner_id": followerID}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "catchall_email": "catchall@example.com"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":              "<p>Inbound</p>",
		"body_is_html":      true,
		"message_type":      "email",
		"model":             "res.partner",
		"res_id":            threadID,
		"author_id":         authorID,
		"partner_ids":       []int64{existingID, followerID},
		"incoming_email_to": `"To" <to@example.com>, "Catch" <catchall@example.com>, "Follower" <follower@example.com>`,
		"incoming_email_cc": `"Copy" <copy@example.com>`,
		"email_from":        `"From" <from@example.com>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"message_id":%d}`, threadID, messageID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail thread recipients response %d %s", rec.Code, rec.Body.String())
	}
	recipients := decodeJSONList(t, rec.Body.Bytes())
	byEmail := map[string]int64{}
	for _, recipient := range recipients {
		byEmail[stringValue(recipient["email"])] = int64Value(recipient["id"])
	}
	for _, email := range []string{"sender@example.com", "existing@example.com", "to@example.com", "copy@example.com", "from@example.com"} {
		if byEmail[email] == 0 {
			t.Fatalf("missing %s in recipients %+v", email, recipients)
		}
	}
	for _, email := range []string{"follower@example.com", "catchall@example.com"} {
		if byEmail[email] != 0 {
			t.Fatalf("unexpected %s in recipients %+v", email, recipients)
		}
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/fields", bytes.NewBufferString(`{"thread_model":"res.partner"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail thread recipient fields response %d %s", rec.Code, rec.Body.String())
	}
	fields := decodeJSON(t, rec.Body.Bytes())
	primary := fields["primary_email_field"].([]any)
	if len(primary) != 1 || primary[0] != "email" {
		t.Fatalf("primary email field = %+v", fields)
	}
	if partnerFields := fields["partner_fields"].([]any); len(partnerFields) != 0 {
		t.Fatalf("res.partner partner fields = %+v", fields)
	}
}

func TestMailThreadRecipientsGetSuggestedRecipientsUsesFrontendUpdates(t *testing.T) {
	server := testMailThreadServer(t)
	additionalID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Additional", "email": "additional@example.com", "email_normalized": "additional@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := server.Env.Model("gateway.thread").Create(map[string]any{"name": "Gateway", "active": true, "email": "old@example.com", "email_normalized": "old@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"gateway.thread","thread_id":%d,"partner_ids":[%d],"main_email":"main@example.com"}`, threadID, additionalID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/get_suggested_recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail thread get suggested recipients response %d %s", rec.Code, rec.Body.String())
	}
	recipients := decodeJSONList(t, rec.Body.Bytes())
	seenAdditional := false
	seenMainEmail := false
	for _, recipient := range recipients {
		if int64Value(recipient["partner_id"]) == additionalID && recipient["email"] == "additional@example.com" {
			seenAdditional = true
		}
		if int64Value(recipient["partner_id"]) == 0 && recipient["email"] == "main@example.com" {
			seenMainEmail = true
		}
		if recipient["email"] == "old@example.com" {
			t.Fatalf("old primary email should be replaced by main_email: %+v", recipients)
		}
	}
	if !seenAdditional || !seenMainEmail {
		t.Fatalf("suggested recipients = %+v", recipients)
	}
}

func TestMailThreadRecipientsUseModelSpecificHooks(t *testing.T) {
	server := testMailThreadServer(t)
	workContactID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Work Contact", "email": "work.contact@example.com", "email_normalized": "work.contact@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "User Partner", "email": "user.partner@example.com", "email_normalized": "user.partner@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := server.Env.Model("hr.employee").Create(map[string]any{
		"name":            "Employee",
		"active":          true,
		"email":           "wrong@example.com",
		"work_email":      "right.work@example.com",
		"work_contact_id": workContactID,
		"user_partner_id": userPartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/fields", bytes.NewBufferString(`{"thread_model":"hr.employee"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("hr recipient fields response %d %s", rec.Code, rec.Body.String())
	}
	fields := decodeJSON(t, rec.Body.Bytes())
	primary := fields["primary_email_field"].([]any)
	if len(primary) != 1 || primary[0] != "work_email" {
		t.Fatalf("primary email field = %+v", fields)
	}
	partnerFields := fields["partner_fields"].([]any)
	if len(partnerFields) != 2 || partnerFields[0] != "work_contact_id" || partnerFields[1] != "user_partner_id" {
		t.Fatalf("partner fields = %+v", fields)
	}

	rec = httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"hr.employee","thread_id":%d}`, employeeID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/get_suggested_recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("hr suggested recipients response %d %s", rec.Code, rec.Body.String())
	}
	recipients := decodeJSONList(t, rec.Body.Bytes())
	byEmail := map[string]int64{}
	for _, recipient := range recipients {
		byEmail[stringValue(recipient["email"])] = int64Value(recipient["partner_id"])
	}
	if byEmail["work.contact@example.com"] != workContactID || byEmail["user.partner@example.com"] != userPartnerID {
		t.Fatalf("partner hook recipients = %+v", recipients)
	}
	if byEmail["right.work@example.com"] != 0 {
		t.Fatalf("primary work email should be raw recipient = %+v", recipients)
	}
	if _, ok := byEmail["wrong@example.com"]; ok {
		t.Fatalf("non-primary email should not be used for hr.employee = %+v", recipients)
	}
}

func TestMailThreadRecipientsOnlyRegisteredCCModelsIncludeEmailCC(t *testing.T) {
	server := testMailThreadServer(t)
	unregister := internalmail.RegisterSuggestedRecipientHook("cc.mixin.thread", internalmail.SuggestedRecipientHook{IncludeEmailCC: true})
	defer unregister()
	genericID, err := server.Env.Model("cc.generic.thread").Create(map[string]any{"name": "Generic", "active": true, "email": "generic@example.com", "email_cc": "generic.cc@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	mixinID, err := server.Env.Model("cc.mixin.thread").Create(map[string]any{"name": "Mixin", "active": true, "email": "mixin@example.com", "email_cc": "mixin.cc@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"cc.generic.thread","thread_id":%d}`, genericID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/get_suggested_recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("generic cc recipients response %d %s", rec.Code, rec.Body.String())
	}
	recipients := decodeJSONList(t, rec.Body.Bytes())
	if !testRecipientEmailSeen(recipients, "generic@example.com") || testRecipientEmailSeen(recipients, "generic.cc@example.com") {
		t.Fatalf("generic cc recipients = %+v", recipients)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"cc.mixin.thread","thread_id":%d}`, mixinID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/get_suggested_recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mixin cc recipients response %d %s", rec.Code, rec.Body.String())
	}
	recipients = decodeJSONList(t, rec.Body.Bytes())
	if !testRecipientEmailSeen(recipients, "mixin@example.com") || !testRecipientEmailSeen(recipients, "mixin.cc@example.com") {
		t.Fatalf("mixin cc recipients = %+v", recipients)
	}
}

func TestMailThreadRecipientsReplyAllIgnoresNonCommentSubtype(t *testing.T) {
	server := testMailThreadServer(t)
	threadID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	commentSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Comment", "default": true, "internal": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "mail", "name": "mt_comment", "complete_name": "mail.mt_comment", "model": "mail.message.subtype", "res_id": commentSubtypeID}); err != nil {
		t.Fatal(err)
	}
	noteSubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{"name": "Note", "default": true, "internal": true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	if _, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":              "<p>Comment</p>",
		"message_type":      "comment",
		"model":             "res.partner",
		"res_id":            threadID,
		"subtype_id":        commentSubtypeID,
		"incoming_email_to": "Comment <comment@example.com>",
		"date":              now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{
		"body":              "<p>Note</p>",
		"message_type":      "comment",
		"model":             "res.partner",
		"res_id":            threadID,
		"subtype_id":        noteSubtypeID,
		"incoming_email_to": "Note <note@example.com>",
		"date":              now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/thread/recipients/get_suggested_recipients", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("reply-all subtype response %d %s", rec.Code, rec.Body.String())
	}
	recipients := decodeJSONList(t, rec.Body.Bytes())
	if !testRecipientEmailSeen(recipients, "comment@example.com") || testRecipientEmailSeen(recipients, "note@example.com") {
		t.Fatalf("reply-all subtype recipients = %+v", recipients)
	}
}

func TestMailMessagePostRouteCreatesPartnerEmails(t *testing.T) {
	server := testMailThreadServer(t)
	threadID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Reply</p>","message_type":"comment","partner_emails":["Reply <reply-all@example.com>"]}}`, threadID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("mail message post partner_emails response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	messageID := int64Value(payload["message_id"])
	if messageID == 0 {
		t.Fatalf("message payload = %+v", payload)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(messageID).Read("partner_ids")
	if err != nil {
		t.Fatal(err)
	}
	partnerIDs := int64Slice(messageRows[0]["partner_ids"])
	if len(partnerIDs) != 1 {
		t.Fatalf("message partner ids = %+v", messageRows)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(partnerIDs...).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["name"] != "Reply" || partnerRows[0]["email"] != "reply-all@example.com" {
		t.Fatalf("created partner rows = %+v", partnerRows)
	}
	found, err := server.Env.Model("mail.notification").Search(domain.And(
		domain.Cond("mail_message_id", "=", messageID),
		domain.Cond("res_partner_id", "=", partnerIDs[0]),
	))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("partner email notification count = %d", found.Len())
	}
}

func TestMailMessagePostPartnerEmailsRequirePartnerManager(t *testing.T) {
	server := testMailThreadServer(t)
	threadID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	nonManager := server
	nonManager.Env = server.Env.WithContext(record.Context{UserID: 20, CompanyID: 1, CompanyIDs: []int64{1}})
	handler := nonManager.Handler()
	body := bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Reply</p>","message_type":"comment","partner_emails":["Blocked <blocked-partner@example.com>"]}}`, threadID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("non-manager partner_emails response %d %s", rec.Code, rec.Body.String())
	}
	found, err := server.Env.Model("res.partner").Search(domain.Cond("email", "=", "blocked-partner@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("non-manager created partner count = %d", found.Len())
	}

	handler = server.Handler()
	body = bytes.NewBufferString(fmt.Sprintf(`{"thread_model":"res.partner","thread_id":%d,"post_data":{"body":"<p>Reply</p>","message_type":"comment","partner_emails":["Allowed <allowed-partner@example.com>"]}}`, threadID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/message/post", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manager partner_emails response %d %s", rec.Code, rec.Body.String())
	}
	found, err = server.Env.Model("res.partner").Search(domain.Cond("email", "=", "allowed-partner@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("manager created partner count = %d", found.Len())
	}
}

func TestMailTemplateAndComposeCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Template Target", "email": "target@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{
		"name":      "Route Template",
		"model":     "res.partner",
		"subject":   "Subject {{ name }}",
		"body_html": "<p>{{ email }}</p>",
		"email_to":  "{{ email }}",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.template","method":"send_mail","args":[[%d],%d]}`, templateID, partnerID))
	var mailID float64
	if err := json.Unmarshal([]byte(body), &mailID); err != nil {
		t.Fatal(err)
	}
	if mailID == 0 {
		t.Fatalf("mail id response %s", body)
	}
	defaultBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"default_get","args":[["model","res_id","res_ids","template_id","subject","body","email_to","body_is_html"]],"kwargs":{"context":{"active_model":"res.partner","active_ids":[%d],"default_template_id":%d}}}`, partnerID, templateID))
	defaults := decodeJSON(t, []byte(defaultBody))
	if defaults["subject"] != "Subject Template Target" || defaults["email_to"] != "target@example.com" {
		t.Fatalf("compose defaults = %+v", defaults)
	}
	wizardID, err := server.Env.Model("mail.compose.message").Create(defaults)
	if err != nil {
		t.Fatal(err)
	}
	actionBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"action_send_mail","args":[[%d]]}`, wizardID))
	action := decodeJSON(t, []byte(actionBody))
	if action["type"] != "ir.actions.act_window_close" {
		t.Fatalf("compose action = %+v", action)
	}

	scheduledWizardID, err := server.Env.Model("mail.compose.message").Create(map[string]any{
		"model":          "res.partner",
		"res_id":         partnerID,
		"res_ids":        []int64{partnerID},
		"subject":        "Scheduled Subject",
		"body":           "<p>Scheduled</p>",
		"email_to":       "target@example.com",
		"scheduled_date": "2026-07-01 10:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduleBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"action_schedule_message","args":[[%d]]}`, scheduledWizardID))
	scheduleAction := decodeJSON(t, []byte(scheduleBody))
	if scheduleAction["type"] != "ir.actions.act_window_close" {
		t.Fatalf("schedule action = %+v", scheduleAction)
	}
	scheduledIDs := int64Slice(scheduleAction["scheduled_message_ids"])
	if len(scheduledIDs) != 1 {
		t.Fatalf("scheduled ids = %+v", scheduleAction["scheduled_message_ids"])
	}
	scheduledRows, err := server.Env.Model("mail.scheduled.message").Browse(scheduledIDs...).Read("mail_message_id", "mail_mail_id", "subject", "body", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduledRows) != 1 ||
		scheduledRows[0]["mail_message_id"] != int64(0) ||
		scheduledRows[0]["mail_mail_id"] != int64(0) ||
		scheduledRows[0]["subject"] != "Scheduled Subject" ||
		scheduledRows[0]["body"] != "<p>Scheduled</p>" ||
		scheduledRows[0]["state"] != "scheduled" {
		t.Fatalf("scheduled rows = %+v", scheduledRows)
	}
}

func TestSMSComposerDefaultGetCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Target", "phone": "+1555010101", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("sms.template").Create(map[string]any{
		"name":  "SMS Template",
		"model": "res.partner",
		"body":  "Hi {{ name }}",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"sms.composer","method":"default_get","args":[["composition_mode","res_model","res_id","template_id","body","recipient_single_description","recipient_single_number","recipient_single_number_itf","recipient_single_valid","recipient_valid_count","recipient_invalid_count","number_field_name","mass_keep_log","use_exclusion_list","comment_single_recipient"]],"kwargs":{"context":{"active_model":"res.partner","active_id":%d,"default_template_id":%d,"sms_composition_mode":"guess"}}}`, partnerID, templateID))
	defaults := decodeJSON(t, []byte(body))
	if defaults["composition_mode"] != "comment" ||
		defaults["res_model"] != "res.partner" ||
		int64Value(defaults["res_id"]) != partnerID ||
		int64Value(defaults["template_id"]) != templateID ||
		defaults["body"] != "Hi SMS Target" ||
		defaults["recipient_single_description"] != "SMS Target" ||
		defaults["recipient_single_number_itf"] != "+1555010101" ||
		defaults["recipient_single_valid"] != true ||
		int64Value(defaults["recipient_valid_count"]) != 1 ||
		int64Value(defaults["recipient_invalid_count"]) != 0 ||
		defaults["number_field_name"] != "phone" ||
		defaults["mass_keep_log"] != true ||
		defaults["use_exclusion_list"] != true ||
		defaults["comment_single_recipient"] != true {
		t.Fatalf("sms composer defaults = %+v", defaults)
	}
}

func TestSMSComposerDefaultGetMassCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerA, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS A", "phone": "+1555010201", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS B", "phone": "+1555010202", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"sms.composer","method":"default_get","args":[["composition_mode","res_model","res_ids","res_ids_count","mass_keep_log","use_exclusion_list"]],"kwargs":{"context":{"active_model":"res.partner","active_ids":[%d,%d],"sms_composition_mode":"guess"}}}`, partnerA, partnerB))
	defaults := decodeJSON(t, []byte(body))
	if defaults["composition_mode"] != "mass" ||
		defaults["res_model"] != "res.partner" ||
		defaults["res_ids"] != fmt.Sprintf("[%d,%d]", partnerA, partnerB) ||
		int64Value(defaults["res_ids_count"]) != 2 ||
		defaults["mass_keep_log"] != true ||
		defaults["use_exclusion_list"] != true {
		t.Fatalf("sms composer mass defaults = %+v", defaults)
	}
}

func TestSMSComposerActionSendSMSNumbersCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	composerID, err := server.Env.Model("sms.composer").Create(map[string]any{
		"composition_mode": "numbers",
		"numbers":          "+1555010301,+1555010302",
		"body":             "Numbers body",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"sms.composer","method":"action_send_sms","args":[[%d]]}`, composerID))
	var result bool
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result {
		t.Fatalf("sms composer action result = %s", body)
	}
	found, err := server.Env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("number", "body", "state", "mail_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 ||
		rows[0]["number"] != "+1555010301" ||
		rows[1]["number"] != "+1555010302" ||
		rows[0]["body"] != "Numbers body" ||
		rows[1]["body"] != "Numbers body" ||
		rows[0]["state"] != "outgoing" ||
		rows[1]["state"] != "outgoing" ||
		int64Value(rows[0]["mail_message_id"]) != 0 ||
		int64Value(rows[1]["mail_message_id"]) != 0 {
		t.Fatalf("sms number rows = %+v", rows)
	}
	messages, err := server.Env.Model("mail.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("mail messages created for numbers mode = %d", messages.Len())
	}
}

func TestSMSComposerActionSendSMSCommentCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Comment", "phone": "+1555010401", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	composerID, err := server.Env.Model("sms.composer").Create(map[string]any{
		"composition_mode":            "comment",
		"res_model":                   "res.partner",
		"res_id":                      partnerID,
		"number_field_name":           "phone",
		"recipient_single_number_itf": "+1555010499",
		"body":                        "Comment body",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"sms.composer","method":"action_send_sms","args":[[%d]]}`, composerID))
	var result bool
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result {
		t.Fatalf("sms comment action result = %s", body)
	}
	smsRows, err := server.Env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsData, err := smsRows.Read("number", "body", "partner_id", "mail_message_id", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsData) != 1 ||
		smsData[0]["number"] != "+1555010499" ||
		smsData[0]["body"] != "Comment body" ||
		int64Value(smsData[0]["partner_id"]) != partnerID ||
		int64Value(smsData[0]["mail_message_id"]) == 0 ||
		smsData[0]["state"] != "outgoing" {
		t.Fatalf("sms comment row = %+v", smsData)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(int64Value(smsData[0]["mail_message_id"])).Read("model", "res_id", "body", "message_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["model"] != "res.partner" ||
		int64Value(messageRows[0]["res_id"]) != partnerID ||
		messageRows[0]["body"] != "Comment body" ||
		messageRows[0]["message_type"] != "sms" {
		t.Fatalf("sms comment message row = %+v", messageRows)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(partnerID).Read("phone")
	if err != nil {
		t.Fatal(err)
	}
	if partnerRows[0]["phone"] != "+1555010499" {
		t.Fatalf("partner phone not updated = %+v", partnerRows)
	}
}

func TestSMSComposerActionSendSMSMassNowCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerA, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Mass A", "phone": "+1555010501", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Mass B", "phone": "+1555010502", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	composerID, err := server.Env.Model("sms.composer").Create(map[string]any{
		"composition_mode": "mass",
		"res_model":        "res.partner",
		"res_ids":          fmt.Sprintf("[%d,%d]", partnerA, partnerB),
		"body":             "Mass body {{ name }}",
		"mass_keep_log":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"sms.composer","method":"action_send_sms_mass_now","args":[[%d]]}`, composerID))
	var result bool
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result {
		t.Fatalf("sms mass action result = %s", body)
	}
	composerRows, err := server.Env.Model("sms.composer").Browse(composerID).Read("mass_force_send")
	if err != nil {
		t.Fatal(err)
	}
	if composerRows[0]["mass_force_send"] != true {
		t.Fatalf("mass_force_send not set = %+v", composerRows)
	}
	smsRows, err := server.Env.Model("sms.sms").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	smsData, err := smsRows.Read("number", "body", "partner_id", "mail_message_id", "state", "failure_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(smsData) != 2 ||
		smsData[0]["number"] != "+1555010501" ||
		smsData[0]["body"] != "Mass body Mass A" ||
		int64Value(smsData[0]["partner_id"]) != partnerA ||
		int64Value(smsData[0]["mail_message_id"]) == 0 ||
		smsData[0]["state"] != "outgoing" ||
		smsData[1]["number"] != "+1555010502" ||
		smsData[1]["body"] != "Mass body Mass B" ||
		int64Value(smsData[1]["partner_id"]) != partnerB ||
		int64Value(smsData[1]["mail_message_id"]) == 0 ||
		smsData[1]["state"] != "outgoing" {
		t.Fatalf("sms mass rows = %+v", smsData)
	}
	messageRows, err := server.Env.Model("mail.message").Search(domain.Cond("message_type", domain.Equal, "sms"))
	if err != nil {
		t.Fatal(err)
	}
	if messageRows.Len() != 2 {
		t.Fatalf("sms mass message count = %d rows=%+v", messageRows.Len(), smsData)
	}
}

func TestMailMailQueueCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	sender := &httpRecordingMailSender{}
	server.MailSender = sender
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Queue",
		"body":         "<p>Queue</p>",
		"message_type": "email",
	})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := server.Env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "queue@example.com",
		"subject":         "Queue",
		"body_html":       "<p>Queue</p>",
		"state":           "outgoing",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), `{"model":"mail.mail","method":"process_email_queue","kwargs":{"batch_size":1000}}`)
	result := decodeJSON(t, []byte(body))
	if result["processed"] != float64(1) || result["sent"] != float64(1) || result["failed"] != float64(0) {
		t.Fatalf("queue response = %+v", result)
	}
	if len(sender.sent) != 1 || sender.sent[0].ID != mailID || sender.sent[0].To != "queue@example.com" {
		t.Fatalf("sent = %+v", sender.sent)
	}
	rows, err := server.Env.Model("mail.mail").Browse(mailID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "sent" || rows[0]["failure_type"] != "" || rows[0]["failure_reason"] != "" {
		t.Fatalf("mail rows = %+v", rows)
	}
}

func TestMassMailingActionSendMailCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "HTTP List", "active": true, "is_public": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "HTTP Contact", "email": "http.contact@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "HTTP Mailing",
		"subject":                 "HTTP {{ name }}",
		"body_html":               "<p>{{ email }}</p>",
		"mailing_model_real":      "mailing.contact",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
		"state":                   "draft",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_send_mail","args":[[%d]]}`, mailingID))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window_close" {
		t.Fatalf("action = %+v", action)
	}
	mailIDs := int64Slice(action["mail_ids"])
	if len(mailIDs) != 1 {
		t.Fatalf("mail ids = %+v action=%+v", mailIDs, action)
	}
	rows, err := server.Env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "subject", "mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "http.contact@example.com" || rows[0]["subject"] != "HTTP HTTP Contact" || rows[0]["mailing_id"] != mailingID {
		t.Fatalf("mail rows = %+v", rows)
	}
	mailingRows, err := server.Env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailingRows) != 1 || mailingRows[0]["state"] != "done" {
		t.Fatalf("mailing rows = %+v", mailingRows)
	}
	if _, ok := mailingRows[0]["sent_date"].(time.Time); !ok {
		t.Fatalf("mailing sent date = %+v", mailingRows[0]["sent_date"])
	}
}

func TestDigestActionSendManualCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "http-digest-secret"}); err != nil {
		t.Fatal(err)
	}
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, map[string]any{"state": "activated", "active": true, "next_run_date": "2026-06-20"})

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"digest.digest","method":"action_send_manual","args":[[%d]]}`, digestID))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window_close" || intValue(action["sent"]) != 1 || intValue(action["slowed_down"]) != 0 {
		t.Fatalf("digest action = %+v", action)
	}
	mailIDs := int64Slice(action["mail_ids"])
	if len(mailIDs) != 1 {
		t.Fatalf("digest mail ids = %+v action=%+v", mailIDs, action)
	}
	mailRows, err := server.Env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "subject", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 || mailRows[0]["email_to"] != "HTTP Digest User <http.digest.user@example.com>" || mailRows[0]["subject"] != "HTTP Digest Co: HTTP Digest" || mailRows[0]["state"] != "outgoing" {
		t.Fatalf("digest mail rows = %+v", mailRows)
	}
	digestRows, err := server.Env.Model("digest.digest").Browse(digestID).Read("periodicity", "next_run_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(digestRows) != 1 || digestRows[0]["periodicity"] != "daily" || accountingDateValue(digestRows[0]["next_run_date"]).IsZero() {
		t.Fatalf("digest rows = %+v", digestRows)
	}
}

func TestDigestCronCallKWProcessesDueActivatedOnly(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	dueID := createHTTPDigest(t, server, "Due Digest", companyID, userID, map[string]any{"state": "activated", "active": true, "next_run_date": "2000-01-01"})
	futureID := createHTTPDigest(t, server, "Future Digest", companyID, userID, map[string]any{"state": "activated", "active": true, "next_run_date": "2999-01-01"})
	deactivatedID := createHTTPDigest(t, server, "Deactivated Digest", companyID, userID, map[string]any{"state": "deactivated", "active": true, "next_run_date": "2000-01-01"})

	body := postCallKW(t, server.Handler(), `{"model":"digest.digest","method":"_cron_send_digest_email","args":[]}`)
	result := decodeJSON(t, []byte(body))
	if intValue(result["sent"]) != 1 || intValue(result["slowed_down"]) != 1 {
		t.Fatalf("digest cron result = %+v", result)
	}
	rows, err := server.Env.Model("digest.digest").Browse(dueID, futureID, deactivatedID).Read("id", "periodicity")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]string{}
	for _, row := range rows {
		byID[int64Value(row["id"])] = stringValue(row["periodicity"])
	}
	if byID[dueID] != "weekly" || byID[futureID] != "daily" || byID[deactivatedID] != "daily" {
		t.Fatalf("digest periodicity rows = %+v", byID)
	}
}

func TestDigestActionSetPeriodicityCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, nil)

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"digest.digest","method":"action_set_periodicity","args":[[%d],"monthly"]}`, digestID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("set periodicity result = %s", body)
	}
	assertHTTPDigestPeriodicity(t, server, digestID, "monthly")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"digest.digest","method":"action_set_periodicity","args":[[%d],"yearly"]}`, digestID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "invalid periodicity") {
		t.Fatalf("invalid call_kw response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestPeriodicity(t, server, digestID, "monthly")
}

func TestDigestUnsubscribeValidTokenRemovesOnlyTargetUser(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	otherUserID := createHTTPDigestUser(t, server, companyID, "HTTP Other Digest User", "http.other.digest.user@example.com")
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "http-digest-secret"}); err != nil {
		t.Fatal(err)
	}
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, map[string]any{"user_ids": []int64{userID, otherUserID}})
	token := internalmail.DigestUnsubscribeToken(server.systemRequestEnv(), digestID, userID)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/unsubscribe?token=%s&user_id=%d", digestID, url.QueryEscape(token), userID), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Unsubscribed") {
		t.Fatalf("unsubscribe response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestUsers(t, server, digestID, []int64{otherUserID})
}

func TestDigestUnsubscribeRejectsInvalidTokenWithoutSideEffects(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	otherUserID := createHTTPDigestUser(t, server, companyID, "HTTP Other Digest User", "http.other.digest.user@example.com")
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "http-digest-secret"}); err != nil {
		t.Fatal(err)
	}
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, map[string]any{"user_ids": []int64{userID, otherUserID}})
	token := internalmail.DigestUnsubscribeToken(server.systemRequestEnv(), digestID, userID)
	handler := server.Handler()

	for _, target := range []string{
		fmt.Sprintf("/digest/%d/unsubscribe?token=bad&user_id=%d", digestID, userID),
		fmt.Sprintf("/digest/%d/unsubscribe?token=%s&user_id=%d", digestID, url.QueryEscape(token), otherUserID),
		fmt.Sprintf("/digest/%d/unsubscribe?token=%s&user_id=%d", digestID+9999, url.QueryEscape(token), userID),
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s response %d %s", target, rec.Code, rec.Body.String())
		}
		assertHTTPDigestUsers(t, server, digestID, []int64{userID, otherUserID})
	}

	anonymousServer := server
	anonymousServer.Security = security.NewEngine()
	rec := httptest.NewRecorder()
	anonymousServer.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/unsubscribe", digestID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("anonymous missing-token response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestUsers(t, server, digestID, []int64{userID, otherUserID})
}

func TestDigestUnsubscribeOneClikRequiresPost(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "http-digest-secret"}); err != nil {
		t.Fatal(err)
	}
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, nil)
	token := internalmail.DigestUnsubscribeToken(server.systemRequestEnv(), digestID, userID)
	target := fmt.Sprintf("/digest/%d/unsubscribe_oneclik?token=%s&user_id=%d", digestID, url.QueryEscape(token), userID)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("one-click GET response %d allow=%q body=%s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
	assertHTTPDigestUsers(t, server, digestID, []int64{userID})

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, strings.NewReader("")))
	if rec.Code != http.StatusOK {
		t.Fatalf("one-click POST response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestUsers(t, server, digestID, nil)
}

func TestDigestUnsubscribeLegacyAuthUsesCurrentInternalUser(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	groupUserID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Internal User"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_user", "complete_name": "base.group_user", "model": "res.groups", "res_id": groupUserID}); err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("res.users").Browse(userID).Write(map[string]any{"groups_id": []int64{groupUserID}}); err != nil {
		t.Fatal(err)
	}
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, nil)
	engine := security.NewEngine()
	engine.Users[1] = security.User{ID: 1, Login: "admin", Active: true, CompanyID: companyID, CompanyIDs: []int64{companyID}}
	engine.Users[userID] = security.User{ID: userID, Login: "http.digest.user@example.com", Active: true, CompanyID: companyID, CompanyIDs: []int64{companyID}, GroupIDs: []int64{groupUserID}}
	engine.IssueSession(userID, "digest-sid", time.Now().Add(time.Hour))
	server.Security = engine

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/unsubscribe", digestID), nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "digest-sid"})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy unsubscribe response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestUsers(t, server, digestID, nil)
}

func TestDigestSetPeriodicityRequiresManagerAndValidValue(t *testing.T) {
	server := testMailThreadServer(t)
	companyID, userID := createHTTPDigestCompanyUser(t, server)
	digestID := createHTTPDigest(t, server, "HTTP Digest", companyID, userID, nil)
	managerGroupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "ERP Manager"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "base", "name": "group_erp_manager", "complete_name": "base.group_erp_manager", "model": "res.groups", "res_id": managerGroupID}); err != nil {
		t.Fatal(err)
	}
	engine := security.NewEngine()
	engine.Users[1] = security.User{ID: 1, Login: "admin", Active: true, CompanyID: companyID, CompanyIDs: []int64{companyID}, GroupIDs: []int64{managerGroupID}}
	engine.Users[userID] = security.User{ID: userID, Login: "http.digest.user@example.com", Active: true, CompanyID: companyID, CompanyIDs: []int64{companyID}}
	engine.IssueSession(userID, "digest-user-sid", time.Now().Add(time.Hour))
	managerUserID := createHTTPDigestUser(t, server, companyID, "HTTP Digest Manager", "http.digest.manager@example.com")
	engine.Users[managerUserID] = security.User{ID: managerUserID, Login: "http.digest.manager@example.com", Active: true, CompanyID: companyID, CompanyIDs: []int64{companyID}, GroupIDs: []int64{managerGroupID}}
	engine.IssueSession(managerUserID, "digest-manager-sid", time.Now().Add(time.Hour))
	server.Security = engine
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/set_periodicity?periodicity=weekly", digestID), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated periodicity response %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/set_periodicity?periodicity=weekly", digestID), nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "digest-user-sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-manager periodicity response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestPeriodicity(t, server, digestID, "daily")

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/set_periodicity?periodicity=weekly", digestID), nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "digest-manager-sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != fmt.Sprintf("/odoo/digest.digest/%d", digestID) {
		t.Fatalf("manager periodicity response %d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	assertHTTPDigestPeriodicity(t, server, digestID, "weekly")

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/digest/%d/set_periodicity?periodicity=yearly", digestID), nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "digest-manager-sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid periodicity response %d %s", rec.Code, rec.Body.String())
	}
	assertHTTPDigestPeriodicity(t, server, digestID, "weekly")
}

func createHTTPDigestCompanyUser(t *testing.T, server Server) (int64, int64) {
	t.Helper()
	companyPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Digest Co", "email": "http.digest.co@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := server.Env.Model("res.company").Create(map[string]any{"name": "HTTP Digest Co", "partner_id": companyPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	userPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Digest User", "email": "http.digest.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{
		"name":       "HTTP Digest User",
		"login":      "http.digest.user@example.com",
		"email":      "http.digest.user@example.com",
		"partner_id": userPartnerID,
		"company_id": companyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return companyID, userID
}

func createHTTPDigestUser(t *testing.T, server Server, companyID int64, name string, email string) int64 {
	t.Helper()
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": name, "email": email, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{
		"name":       name,
		"login":      email,
		"email":      email,
		"partner_id": partnerID,
		"company_id": companyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return userID
}

func createHTTPDigest(t *testing.T, server Server, name string, companyID int64, userID int64, overrides map[string]any) int64 {
	t.Helper()
	values := map[string]any{
		"name":        name,
		"active":      true,
		"state":       "activated",
		"periodicity": "daily",
		"user_ids":    []int64{userID},
		"company_id":  companyID,
	}
	for key, value := range overrides {
		values[key] = value
	}
	id, err := server.Env.Model("digest.digest").Create(values)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertHTTPDigestUsers(t *testing.T, server Server, digestID int64, expected []int64) {
	t.Helper()
	rows, err := server.Env.Model("digest.digest").Browse(digestID).Read("user_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("digest rows = %+v", rows)
	}
	got := int64Slice(rows[0]["user_ids"])
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	expected = append([]int64(nil), expected...)
	sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("digest users got=%v expected=%v", got, expected)
	}
}

func assertHTTPDigestPeriodicity(t *testing.T, server Server, digestID int64, expected string) {
	t.Helper()
	rows, err := server.Env.Model("digest.digest").Browse(digestID).Read("periodicity")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["periodicity"] != expected {
		t.Fatalf("digest periodicity rows = %+v expected=%s", rows, expected)
	}
}

func TestMassMailingTestWizardCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Sample", "email": "http.sample@example.com", "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":               "HTTP Test Mailing",
		"subject":            "HTTP Test {{ name }}",
		"preview":            "Preview {{ name }}",
		"body_html":          "<p>{{ email }}</p>",
		"mailing_model_real": "res.partner",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_test","args":[[%d]]}`, mailingID))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "mailing.mailing.test" || action["target"] != "new" {
		t.Fatalf("test action = %+v", action)
	}
	wizardID, err := server.Env.Model("mailing.mailing.test").Create(map[string]any{"email_to": "http.test@example.com", "mass_mailing_id": mailingID})
	if err != nil {
		t.Fatal(err)
	}
	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing.test","method":"send_mail_test","args":[[%d]]}`, wizardID))
	result := decodeJSON(t, []byte(body))
	mailIDs := int64Slice(result["mail_ids"])
	if len(mailIDs) != 1 {
		t.Fatalf("test result = %+v", result)
	}
	rows, err := server.Env.Model("mail.mail").Browse(mailIDs...).Read("email_to", "subject", "body_html", "mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email_to"] != "http.test@example.com" || rows[0]["subject"] != "[TEST] HTTP Test HTTP Sample" || rows[0]["mailing_id"] != mailingID || !strings.Contains(stringValue(rows[0]["body_html"]), `class="o_mail_wrapper"`) {
		t.Fatalf("test mail rows = %+v", rows)
	}
}

func TestMassMailingButtonCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	future := time.Now().UTC().Add(time.Hour)
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":          "Buttons",
		"subject":       "Buttons",
		"body_html":     "<p>Buttons</p>",
		"state":         "draft",
		"schedule_type": "scheduled",
		"schedule_date": future,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_schedule","args":[[%d]]}`, mailingID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("future schedule response = %s", body)
	}
	rows, err := server.Env.Model("mailing.mailing").Browse(mailingID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" {
		t.Fatalf("scheduled row = %+v", rows)
	}

	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_cancel","args":[[%d]]}`, mailingID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("cancel response = %s", body)
	}
	rows, err = server.Env.Model("mailing.mailing").Browse(mailingID).Read("state", "schedule_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "draft" || rows[0]["schedule_type"] != "now" {
		t.Fatalf("canceled row = %+v", rows)
	}

	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_schedule","args":[[%d]]}`, mailingID))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "mailing.mailing.schedule.date" {
		t.Fatalf("schedule wizard action = %+v", action)
	}
	wizardDate := future.Add(time.Hour).UTC().Truncate(time.Second)
	wizardID, err := server.Env.Model("mailing.mailing.schedule.date").Create(map[string]any{"mass_mailing_id": mailingID, "schedule_date": wizardDate})
	if err != nil {
		t.Fatal(err)
	}
	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing.schedule.date","method":"action_schedule_date","args":[[%d]]}`, wizardID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("schedule date response = %s", body)
	}
	rows, err = server.Env.Model("mailing.mailing").Browse(mailingID).Read("state", "schedule_type", "schedule_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" || rows[0]["schedule_type"] != "scheduled" || !accountingDateValue(rows[0]["schedule_date"]).Equal(wizardDate) {
		t.Fatalf("schedule date row = %+v", rows)
	}

	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"subject": "Failed", "body": "<p>Failed</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := server.Env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        "failed@example.com",
		"subject":         "Failed",
		"body_html":       "<p>Failed</p>",
		"state":           "exception",
		"mailing_id":      mailingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{
		"mail_mail_id":    mailID,
		"email":           "failed@example.com",
		"model":           "res.partner",
		"res_id":          int64(1),
		"mass_mailing_id": mailingID,
		"trace_status":    "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_retry_failed","args":[[%d]]}`, mailingID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("retry response = %s", body)
	}
	mailRows, err := server.Env.Model("mail.mail").Browse(mailID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 0 || len(traceRows) != 0 {
		t.Fatalf("retry cleanup mail=%+v trace=%+v", mailRows, traceRows)
	}
}

func TestMassMailingABWinnerButtonCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "AB HTTP", "email": "ab-http@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "HTTP AB Campaign", "ab_testing_winner_selection": "manual"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                        "HTTP AB",
		"subject":                     "HTTP AB {{ name }}",
		"body_html":                   "<p>{{ email }}</p>",
		"mailing_model_real":          "res.partner",
		"mailing_domain":              fmt.Sprintf("[('id', '=', %d)]", partnerID),
		"campaign_id":                 campaignID,
		"ab_testing_enabled":          true,
		"ab_testing_pc":               int64(50),
		"ab_testing_winner_selection": "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"mailing.mailing","method":"action_select_as_winner","args":[[%d]]}`, mailingID))
	action := decodeJSON(t, []byte(body))
	winnerID := int64Value(action["res_id"])
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "mailing.mailing" || winnerID == 0 {
		t.Fatalf("winner action = %+v", action)
	}
	rows, err := server.Env.Model("mailing.mailing").Browse(winnerID).Read("state", "ab_testing_pc", "ab_testing_is_winner_mailing")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "in_queue" || int64Value(rows[0]["ab_testing_pc"]) != 100 || rows[0]["ab_testing_is_winner_mailing"] != true {
		t.Fatalf("winner rows = %+v", rows)
	}
	campaignRows, err := server.Env.Model("utm.campaign").Browse(campaignID).Read("ab_testing_completed", "ab_testing_winner_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(campaignRows) != 1 || campaignRows[0]["ab_testing_completed"] != true || int64Value(campaignRows[0]["ab_testing_winner_mailing_id"]) != winnerID {
		t.Fatalf("campaign rows = %+v", campaignRows)
	}
}

func TestFetchmailServerButtonCallKW(t *testing.T) {
	server := testMailThreadServer(t)
	fetchmailID, err := server.Env.Model("fetchmail.server").Create(map[string]any{
		"name":          "Inbound",
		"server_type":   "imap",
		"state":         "draft",
		"active":        true,
		"error_message": "previous error",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"fetchmail.server","method":"button_confirm_login","args":[[%d]]}`, fetchmailID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("confirm response = %s", body)
	}
	rows, err := server.Env.Model("fetchmail.server").Browse(fetchmailID).Read("state", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "done" || rows[0]["error_message"] != "previous error" {
		t.Fatalf("confirmed row = %+v", rows)
	}

	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"fetchmail.server","method":"fetch_mail","args":[[%d]]}`, fetchmailID))
	result := decodeJSON(t, []byte(body))
	if result["fetched"] != float64(0) {
		t.Fatalf("fetch response = %+v", result)
	}
	rows, err = server.Env.Model("fetchmail.server").Browse(fetchmailID).Read("date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["date"] == nil {
		t.Fatalf("fetch row = %+v", rows)
	}

	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"fetchmail.server","method":"set_draft","args":[[%d]]}`, fetchmailID))
	if strings.TrimSpace(body) != "true" {
		t.Fatalf("draft response = %s", body)
	}
	rows, err = server.Env.Model("fetchmail.server").Browse(fetchmailID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "draft" {
		t.Fatalf("draft row = %+v", rows)
	}
}

func TestMailThreadMessageProcessMarksBounceNotification(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Bounce", "email": "bounce-target@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"subject": "Original", "body": "<p>Original</p>", "message_type": "email", "message_id": "<http-bounce@local>"})
	if err != nil {
		t.Fatal(err)
	}
	notificationID, err := server.Env.Model("mail.notification").Create(map[string]any{
		"mail_message_id":     messageID,
		"res_partner_id":      partnerID,
		"mail_email_address":  "bounce-target@example.com",
		"notification_type":   "email",
		"notification_status": "sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args":   []any{"res.partner", mailThreadBounceMessage("bounce-target@example.com", "<http-bounce@local>")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	rows, err := server.Env.Model("mail.notification").Browse(notificationID).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["notification_status"] != "bounce" || rows[0]["failure_type"] != "mail_bounce" || !strings.Contains(rows[0]["failure_reason"].(string), "550 No such user") {
		t.Fatalf("notification = %+v", rows)
	}
}

func TestMailThreadMessageProcessRoutesReply(t *testing.T) {
	server := testMailThreadServer(t)
	recordID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Original",
		"body":         "<p>Original</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<http-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-reply@remote>",
			"From: Reply <reply@example.com>",
			"To: thread@example.com",
			"Cc: copy@example.com",
			"Reply-To: Help <help@example.com>",
			"Subject: Re: Original",
			"In-Reply-To: <http-parent@local>",
			"References: <http-parent@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Reply body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != fmt.Sprint(recordID) {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-reply@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "res_id", "parent_id", "body", "incoming_email_to", "incoming_email_cc", "reply_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["model"] != "res.partner" ||
		rows[0]["res_id"] != recordID ||
		rows[0]["parent_id"] != parentID ||
		!strings.Contains(rows[0]["body"].(string), "Reply body") ||
		rows[0]["incoming_email_to"] != "thread@example.com" ||
		rows[0]["incoming_email_cc"] != "copy@example.com" ||
		rows[0]["reply_to"] != "Reply <reply@example.com>" {
		t.Fatalf("reply rows = %+v", rows)
	}
}

func TestMailThreadMessageProcessReplyFiltersAliasMetadata(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_alias":   "bounce",
		"catchall_alias": "catchall",
		"default_from":   "notifications",
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Filtered Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Original",
		"body":         "<p>Original</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       recordID,
		"message_id":   "<http-filter-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-filter-reply@remote>",
			"From: Reply <reply@example.com>",
			`To: "Catchall" <catchall@example.com>, "Other" <other@example.com>, notifications@example.com`,
			`Cc: "Bounce" <bounce@example.com>, "Copy" <copy@example.com>`,
			"Subject: Re: Original",
			"In-Reply-To: <http-filter-parent@local>",
			"References: <http-filter-parent@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Reply body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != fmt.Sprint(recordID) {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-filter-reply@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "res_id", "parent_id", "incoming_email_to", "incoming_email_cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["model"] != "res.partner" ||
		rows[0]["res_id"] != recordID ||
		rows[0]["parent_id"] != parentID ||
		rows[0]["incoming_email_to"] != `"Other" <other@example.com>` ||
		rows[0]["incoming_email_cc"] != `"Copy" <copy@example.com>` {
		t.Fatalf("filtered reply rows = %+v", rows)
	}
}

func TestMailThreadMessageProcessReplyOtherModelAliasBypassesParent(t *testing.T) {
	server := testMailThreadServer(t)
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "forward",
		"alias_domain_id": aliasDomainID,
		"model_name":      "portal.thread",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	parentRecordID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       parentRecordID,
		"message_id":   "<http-parent-forward@local>",
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-reply-forward@remote>",
			"From: Reply <reply@example.com>",
			"To: forward@example.com",
			"Subject: HTTP Forward Alias",
			"In-Reply-To: <http-parent-forward@local>",
			"References: <http-parent-forward@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Reply body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-reply-forward@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "res_id", "parent_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "portal.thread" || rows[0]["res_id"] != resID || rows[0]["parent_id"] != int64(0) || strings.TrimSpace(stringValue(rows[0]["incoming_email_to"])) != "" {
		t.Fatalf("forward reply rows = %+v response=%s", rows, body)
	}
}

func TestMailThreadMessageProcessReplySameModelAliasContactApplies(t *testing.T) {
	server := testMailThreadServer(t)
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "restricted",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "partners",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	parentRecordID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Restricted Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "res.partner",
		"res_id":       parentRecordID,
		"message_id":   "<http-parent-restricted@local>",
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-reply-restricted@remote>",
			"From: Unknown <unknown.restricted@example.net>",
			"To: restricted@example.com",
			"Subject: HTTP Restricted Reply",
			"In-Reply-To: <http-parent-restricted@local>",
			"References: <http-parent-restricted@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Reply body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	if body != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-reply-restricted@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("restricted reply message count = %d", found.Len())
	}
	bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-reply-restricted@remote>")
	if len(bounceRows) != 1 || bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` || !strings.Contains(stringValue(bounceRows[0]["body_html"]), "registered partners") {
		t.Fatalf("restricted bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessMultipleAliasesCreatesAllRoutes(t *testing.T) {
	server := testMailThreadServer(t)
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "alpha",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_defaults":  "{'name': 'HTTP Alpha Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "portal",
		"alias_domain_id": aliasDomainID,
		"model_name":      "portal.thread",
		"alias_defaults":  "{'name': 'HTTP Portal Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-multi-alias@remote>",
			"From: Multi Sender <multi.sender@example.net>",
			"To: alpha@example.com, portal@example.com",
			"Subject: HTTP Multi Alias",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Multi body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	portalRows, err := server.Env.Model("portal.thread").Browse(resID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if len(portalRows) != 1 || portalRows[0]["name"] != "HTTP Portal Alias" {
		t.Fatalf("portal response row = %+v body=%s", portalRows, body)
	}
	alpha, err := server.Env.Model("res.partner").Search(domain.Cond("name", "=", "HTTP Alpha Alias"))
	if err != nil {
		t.Fatal(err)
	}
	if alpha.Len() != 1 {
		t.Fatalf("alpha count = %d", alpha.Len())
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-multi-alias@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]int{}
	for _, row := range rows {
		models[stringValue(row["model"])]++
		if strings.TrimSpace(stringValue(row["incoming_email_to"])) != "" {
			t.Fatalf("multi alias rows = %+v", rows)
		}
	}
	if len(rows) != 2 || models["res.partner"] != 1 || models["portal.thread"] != 1 {
		t.Fatalf("multi alias rows = %+v", rows)
	}
}

func TestMailThreadMessageProcessSkipsDeniedAliasForAllowedRecipient(t *testing.T) {
	server := testMailThreadServer(t)
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "denied",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"alias_defaults":  "{'name': 'HTTP Denied Alias'}",
		"alias_contact":   "partners",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "allowed",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"alias_defaults":  "{'name': 'HTTP Allowed Alias'}",
		"alias_contact":   "everyone",
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-contact-skip@remote>",
			"From: Unknown <unknown.alias@example.com>",
			"To: denied@example.com, allowed@example.com",
			"Subject: HTTP Alias Contact Skip",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Alias body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	rows, err := server.Env.Model("res.partner").Browse(resID).Read("name")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "HTTP Allowed Alias" {
		t.Fatalf("allowed row = %+v response=%s", rows, body)
	}
	denied, err := server.Env.Model("res.partner").Search(domain.Cond("name", "=", "HTTP Denied Alias"))
	if err != nil {
		t.Fatal(err)
	}
	if denied.Len() != 0 {
		t.Fatalf("denied alias created partner count = %d", denied.Len())
	}
	if bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-alias-contact-skip@remote>"); len(bounceRows) != 0 {
		t.Fatalf("skipped denied alias created bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessSkipsDuplicateMessageID(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "Existing",
		"body":         "<p>Existing</p>",
		"message_type": "email",
		"message_id":   "<http-duplicate@remote>",
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-duplicate@remote>",
			"From: Duplicate <duplicate@example.com>",
			"To: catch@example.com",
			"Subject: Duplicate",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Duplicate body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-duplicate@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("duplicate message count = %d", found.Len())
	}
}

func TestMailThreadMessageProcessRoutesAliasForceThreadID(t *testing.T) {
	server := testMailThreadServer(t)
	targetID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Forced", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "http-forced",
		"alias_domain_id":       aliasDomainID,
		"alias_model_id":        modelID,
		"alias_force_thread_id": targetID,
		"alias_defaults":        "{'name': 'Should Not Create'}",
		"alias_contact":         "everyone",
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-forced@remote>",
			"From: HTTP Forced <http.forced@example.com>",
			"To: http-forced@example.com",
			"Subject: HTTP Alias Forced",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP forced body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != fmt.Sprint(targetID) {
		t.Fatalf("message_process response = %s target=%d", body, targetID)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-forced@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "res_id", "body")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != targetID || !strings.Contains(rows[0]["body"].(string), "HTTP forced body") {
		t.Fatalf("forced alias message rows = %+v", rows)
	}
	created, err := server.Env.Model("res.partner").Search(domain.Cond("name", "=", "Should Not Create"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Len() != 0 {
		t.Fatalf("forced HTTP alias created partner count = %d", created.Len())
	}
}

func TestMailThreadMessageProcessUsesGatewayUserForTargetAndRootForMessage(t *testing.T) {
	server := testMailThreadServer(t)
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "gateway.thread", "name": "Gateway Thread"})
	if err != nil {
		t.Fatal(err)
	}
	authorPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Gateway User Partner", "email": "http.gateway.user@example.com", "email_normalized": "http.gateway.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := server.Env.Model("res.users").Create(map[string]any{"login": "http-gateway-user", "name": "HTTP Gateway User", "partner_id": authorPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "http-gateway",
		"alias_domain_id": aliasDomainID,
		"alias_model_id":  modelID,
		"active":          true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"gateway.thread", strings.Join([]string{
			"Message-Id: <http-gateway-user@remote>",
			"From: HTTP Gateway User <http.gateway.user@example.com>",
			"To: http-gateway@example.com",
			"Subject: HTTP Gateway User Creates",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP gateway body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	targetRows, err := server.Env.Model("gateway.thread").Browse(resID).Read("create_uid", "write_uid", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 || targetRows[0]["create_uid"] != authorUserID || targetRows[0]["write_uid"] != authorUserID || targetRows[0]["email"] != "http.gateway.user@example.com" {
		t.Fatalf("target rows = %+v user=%d", targetRows, authorUserID)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-gateway-user@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := found.Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["create_uid"] != int64(1) ||
		messageRows[0]["write_uid"] != int64(1) ||
		messageRows[0]["author_id"] != authorPartnerID ||
		messageRows[0]["model"] != "gateway.thread" ||
		messageRows[0]["res_id"] != resID {
		t.Fatalf("message rows = %+v", messageRows)
	}
}

func TestMailThreadMessageProcessRunsInboundMessageNewAndUpdateHooks(t *testing.T) {
	server := testMailThreadServer(t)
	authorPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Hook Sender", "email": "http.hook@example.com", "email_normalized": "http.hook@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := server.Env.Model("res.users").Create(map[string]any{"login": "http-hook", "name": "HTTP Hook", "partner_id": authorPartnerID, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	hookCalls := []string{}
	unregister := internalmail.RegisterInboundMessageHandler("gateway.thread", internalmail.InboundMessageHandler{
		MessageNew: func(hookEnv *record.Env, req internalmail.InboundMessageNewRequest) (int64, error) {
			if hookEnv.Context().UserID != authorUserID || req.Message.AuthorID != authorPartnerID || req.Message.MessageID != "<http-hook-new@remote>" {
				t.Fatalf("new hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			hookCalls = append(hookCalls, "new")
			return hookEnv.Model(req.Model).Create(map[string]any{
				"name":            req.Message.Subject + " handled",
				"description":     req.Message.BodyHTML,
				"gateway_user_id": hookEnv.Context().UserID,
				"active":          true,
			})
		},
		MessageUpdate: func(hookEnv *record.Env, req internalmail.InboundMessageUpdateRequest) error {
			if hookEnv.Context().UserID != authorUserID || req.Message.AuthorID != authorPartnerID || req.Message.MessageID != "<http-hook-update@remote>" || req.ResID == 0 || req.Message.ParentID == 0 {
				t.Fatalf("update hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			hookCalls = append(hookCalls, "update")
			return hookEnv.Model(req.Model).Browse(req.ResID).Write(map[string]any{
				"description":     req.Message.Subject + "|" + req.Message.BodyHTML,
				"message_count":   int64(1),
				"gateway_user_id": hookEnv.Context().UserID,
			})
		},
	})
	t.Cleanup(unregister)

	handler := server.Handler()
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"gateway.thread", strings.Join([]string{
			"Message-Id: <http-hook-new@remote>",
			"From: HTTP Hook <http.hook@example.com>",
			"To: catch@example.com",
			"Subject: HTTP Hook New",
			"Content-Type: text/html; charset=utf-8",
			"",
			"<p>HTTP hook body</p>",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, handler, string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process new response = %s", body)
	}
	targetRows, err := server.Env.Model("gateway.thread").Browse(resID).Read("name", "description", "gateway_user_id", "create_uid", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 ||
		targetRows[0]["name"] != "HTTP Hook New handled" ||
		targetRows[0]["description"] != "<p>HTTP hook body</p>" ||
		targetRows[0]["gateway_user_id"] != authorUserID ||
		targetRows[0]["create_uid"] != authorUserID ||
		targetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("new target rows = %+v user=%d", targetRows, authorUserID)
	}
	newMessages, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-hook-new@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	newRows, err := newMessages.Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(newRows) != 1 || newRows[0]["create_uid"] != int64(1) || newRows[0]["write_uid"] != int64(1) || newRows[0]["author_id"] != authorPartnerID || newRows[0]["model"] != "gateway.thread" || newRows[0]["res_id"] != resID {
		t.Fatalf("new message rows = %+v", newRows)
	}
	parentID, err := server.Env.Model("mail.message").Create(map[string]any{
		"subject":      "HTTP parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "gateway.thread",
		"res_id":       resID,
		"message_id":   "<http-hook-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err = json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"gateway.thread", strings.Join([]string{
			"Message-Id: <http-hook-update@remote>",
			"From: HTTP Hook <http.hook@example.com>",
			"To: catch@example.com",
			"Subject: HTTP Hook Update",
			"In-Reply-To: <http-hook-parent@local>",
			"References: <http-hook-parent@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP update body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = strings.TrimSpace(postCallKW(t, handler, string(payload)))
	updateResID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || updateResID != resID {
		t.Fatalf("message_process update response = %s id=%d", body, resID)
	}
	updateTargetRows, err := server.Env.Model("gateway.thread").Browse(resID).Read("description", "message_count", "gateway_user_id", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(updateTargetRows) != 1 ||
		!strings.Contains(fmt.Sprint(updateTargetRows[0]["description"]), "HTTP Hook Update|<pre>HTTP update body</pre>") ||
		updateTargetRows[0]["message_count"] != int64(1) ||
		updateTargetRows[0]["gateway_user_id"] != authorUserID ||
		updateTargetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("update target rows = %+v user=%d", updateTargetRows, authorUserID)
	}
	updateMessages, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-hook-update@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	updateRows, err := updateMessages.Read("create_uid", "write_uid", "author_id", "model", "res_id", "parent_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(updateRows) != 1 ||
		updateRows[0]["create_uid"] != int64(1) ||
		updateRows[0]["write_uid"] != int64(1) ||
		updateRows[0]["author_id"] != authorPartnerID ||
		updateRows[0]["model"] != "gateway.thread" ||
		updateRows[0]["res_id"] != resID ||
		updateRows[0]["parent_id"] != parentID {
		t.Fatalf("update message rows = %+v parent=%d", updateRows, parentID)
	}
	if strings.Join(hookCalls, ",") != "new,update" {
		t.Fatalf("hook calls = %+v", hookCalls)
	}
}

func TestMailThreadMessageProcessPropagatesMailingTraceUTMAndCustomValues(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "HTTP Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := server.Env.Model("utm.source").Create(map[string]any{"name": "HTTP Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := server.Env.Model("utm.medium").Create(map[string]any{"name": "HTTP Email"})
	if err != nil {
		t.Fatal(err)
	}
	overrideSourceID, err := server.Env.Model("utm.source").Create(map[string]any{"name": "HTTP Override"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":        "HTTP Mailing",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{
		"message_id":      "<http-utm-parent@local>",
		"email":           "http.recipient@example.com",
		"model":           "res.partner",
		"res_id":          partnerID,
		"mass_mailing_id": mailingID,
	}); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"Message-Id: <http-utm-reply@remote>",
		"From: HTTP Prospect <http.prospect@example.com>",
		"To: catch@example.com",
		"Subject: HTTP UTM Reply",
		"References: <http-utm-parent@local>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"HTTP UTM body",
		"",
	}, "\r\n")
	handler := server.Handler()
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args":   []any{"gateway.thread", raw, map[string]any{"source_id": overrideSourceID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, handler, string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	rows, err := server.Env.Model("gateway.thread").Browse(resID).Read("campaign_id", "source_id", "medium_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || int64Value(rows[0]["campaign_id"]) != campaignID || int64Value(rows[0]["source_id"]) != overrideSourceID || int64Value(rows[0]["medium_id"]) != mediumID {
		t.Fatalf("gateway UTM rows = %+v", rows)
	}
}

func TestMassMailingOpenPixelMarksTraceOpened(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-open-secret"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"subject": "Track", "body": "<p>Track</p>", "message_type": "email"})
	if err != nil {
		t.Fatal(err)
	}
	mailID, err := server.Env.Model("mail.mail").Create(map[string]any{"mail_message_id": messageID, "email_to": "trace@example.com", "state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Trace Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"mail_mail_id": mailID, "email": "trace@example.com", "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}

	token := massMailingOpenToken(server.systemRequestEnv(), mailID)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/track/%d/%s/blank.gif", mailID, token), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/gif" || rec.Body.Len() == 0 {
		t.Fatalf("open pixel response %d type=%s body=%x", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes())
	}
	rows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["trace_status"] != "open" || accountingDateValue(rows[0]["open_datetime"]).IsZero() || !accountingDateValue(rows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("open trace rows = %+v", rows)
	}
}

func TestMassMailingTrackedLinkClickMarksTraceAndRedirects(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Click Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "Click Mailing", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Click Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"mass_mailing_id": mailingID, "email": "click@example.com", "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/foo/bar?baz=qux", "campaign_id": campaignID, "code": "AbC123"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/AbC123/m/%d", traceID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/foo/bar?baz=qux&utm_campaign=Click+Campaign" {
		t.Fatalf("tracked redirect response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "mailing_trace_id", "mass_mailing_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["mailing_trace_id"]) != traceID || int64Value(clickRows[0]["mass_mailing_id"]) != mailingID || int64Value(clickRows[0]["campaign_id"]) != campaignID {
		t.Fatalf("click rows = %+v", clickRows)
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "open" || accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("clicked trace rows = %+v", traceRows)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("link rows = %+v", linkRows)
	}
}

func TestSMSTrackedLinkClickMarksTraceAndRedirects(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "SMS Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Mailing", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"number": "+15550101", "body": "https://gorp.example/r/Sms123/s/1", "state": "sent", "mailing_id": mailingID})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "sms_id_int": smsID, "sms_number": "+15550101", "mass_mailing_id": mailingID, "campaign_id": campaignID, "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/sms/path?x=1", "code": "Sms123"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/Sms123/s/%d", smsID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/sms/path?x=1" {
		t.Fatalf("sms redirect response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "mailing_trace_id", "mass_mailing_id", "campaign_id", "whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["mailing_trace_id"]) != traceID || int64Value(clickRows[0]["mass_mailing_id"]) != mailingID || int64Value(clickRows[0]["campaign_id"]) != campaignID || int64Value(clickRows[0]["whatsapp_message_id"]) != 0 {
		t.Fatalf("sms click rows = %+v", clickRows)
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "open" || accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("sms trace rows = %+v", traceRows)
	}
}

func TestSMSTrackedLinkMissingTraceCreatesPlainClick(t *testing.T) {
	server := testMailThreadServer(t)
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/sms-missing", "code": "SmsMiss1"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r/SmsMiss1/s/999999", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/sms-missing" {
		t.Fatalf("missing sms trace response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "mailing_trace_id", "mass_mailing_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["mailing_trace_id"]) != 0 || int64Value(clickRows[0]["mass_mailing_id"]) != 0 || int64Value(clickRows[0]["campaign_id"]) != 0 {
		t.Fatalf("missing sms click rows = %+v", clickRows)
	}
}

func TestSMSTrackedLinkBotDoesNotCreateClick(t *testing.T) {
	server := testMailThreadServer(t)
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Bot Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Bot Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"number": "+15550202", "body": "https://gorp.example/r/SmsBot1/s/1", "state": "sent", "mailing_id": mailingID})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "sms_id_int": smsID, "sms_number": "+15550202", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/sms-bot", "code": "SmsBot1"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/SmsBot1/s/%d", smsID), nil)
	req.Header.Set("User-Agent", "Googlebot/2.1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/sms-bot" {
		t.Fatalf("sms bot redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("sms bot click count = %d", clicks.Len())
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || !accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || !accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("sms bot trace rows = %+v", traceRows)
	}
}

func TestSMSDeliveryStatusWebhookRouteValidation(t *testing.T) {
	server := testMailThreadServer(t)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sms/status", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("sms status GET response %d allow=%s body=%s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
}

func TestSMSDeliveryStatusWebhookUpdatesNotifications(t *testing.T) {
	server := testMailThreadServer(t)
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"model": "res.partner", "res_id": int64(1), "body": "SMS"})
	if err != nil {
		t.Fatal(err)
	}
	processingUUID := "11111111111111111111111111111111"
	deliveredUUID := "22222222222222222222222222222222"
	failedUUID := "33333333333333333333333333333333"
	bounceUUID := "44444444444444444444444444444444"
	unknownUUID := "55555555555555555555555555555555"
	notificationIDs := map[string]int64{}
	for uuid, status := range map[string]string{
		processingUUID: "process",
		deliveredUUID:  "pending",
		failedUUID:     "pending",
		bounceUUID:     "pending",
		unknownUUID:    "pending",
	} {
		smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"uuid": uuid, "number": "+15550101", "body": "Hello", "state": "outgoing"})
		if err != nil {
			t.Fatal(err)
		}
		notificationID, err := server.Env.Model("mail.notification").Create(map[string]any{"mail_message_id": messageID, "notification_type": "sms", "notification_status": status, "sms_id_int": smsID, "sms_number": "+15550101"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.Env.Model("sms.tracker").Create(map[string]any{"sms_uuid": uuid, "mail_notification_id": notificationID}); err != nil {
			t.Fatal(err)
		}
		notificationIDs[uuid] = notificationID
	}

	result := postSMSStatusJSONRPC(t, server, []smsStatusReport{
		{SMSStatus: "sent", UUIDs: []string{processingUUID}},
		{SMSStatus: "delivered", UUIDs: []string{deliveredUUID}},
		{SMSStatus: "not_delivered", UUIDs: []string{failedUUID}},
		{SMSStatus: "invalid_destination", UUIDs: []string{bounceUUID}},
		{SMSStatus: "something_new", UUIDs: []string{unknownUUID}},
	})
	if result != "OK" {
		t.Fatalf("sms status result = %v", result)
	}
	rows, err := server.Env.Model("mail.notification").Browse(
		notificationIDs[processingUUID],
		notificationIDs[deliveredUUID],
		notificationIDs[failedUUID],
		notificationIDs[bounceUUID],
		notificationIDs[unknownUUID],
	).Read("notification_status", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		status        string
		failureType   string
		failureReason string
	}{
		{"pending", "", ""},
		{"sent", "", ""},
		{"exception", "sms_not_delivered", ""},
		{"bounce", "sms_invalid_destination", ""},
		{"exception", "unknown", "something_new"},
	}
	for i, row := range rows {
		if row["notification_status"] != want[i].status || stringValue(row["failure_type"]) != want[i].failureType || stringValue(row["failure_reason"]) != want[i].failureReason {
			t.Fatalf("notification row %d = %+v want %+v", i, row, want[i])
		}
	}
	smsRows, err := server.Env.Model("sms.sms").Search(domain.Cond("uuid", domain.In, []string{processingUUID, deliveredUUID, failedUUID, bounceUUID, unknownUUID}))
	if err != nil {
		t.Fatal(err)
	}
	smsRead, err := smsRows.Read("to_delete")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range smsRead {
		if row["to_delete"] != true {
			t.Fatalf("sms row not marked to_delete: %+v", row)
		}
	}
}

func TestSMSDeliveryStatusWebhookSuccessStatusMatrix(t *testing.T) {
	server := testMailThreadServer(t)
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"model": "res.partner", "res_id": int64(1), "body": "SMS"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Status Matrix", "state": "sending"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Matrix Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		providerStatus     string
		uuid               string
		notificationStatus string
		traceStatus        string
	}{
		{"processing", "66666666666666666666666666666666", "process", "process"},
		{"success", "77777777777777777777777777777777", "pending", "pending"},
		{"sent", "88888888888888888888888888888888", "pending", "pending"},
		{"delivered", "99999999999999999999999999999999", "sent", "sent"},
	}
	notificationIDs := map[string]int64{}
	traceIDs := map[string]int64{}
	for _, tc := range cases {
		smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"uuid": tc.uuid, "number": "+15550101", "body": "Hello", "state": "outgoing", "mailing_id": mailingID})
		if err != nil {
			t.Fatal(err)
		}
		notificationID, err := server.Env.Model("mail.notification").Create(map[string]any{"mail_message_id": messageID, "notification_type": "sms", "notification_status": "ready", "sms_id_int": smsID, "sms_number": "+15550101"})
		if err != nil {
			t.Fatal(err)
		}
		traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": partnerID, "sms_id_int": smsID, "sms_number": "+15550101", "trace_status": "outgoing"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.Env.Model("sms.tracker").Create(map[string]any{"sms_uuid": tc.uuid, "mail_notification_id": notificationID, "mailing_trace_id": traceID}); err != nil {
			t.Fatal(err)
		}
		notificationIDs[tc.uuid] = notificationID
		traceIDs[tc.uuid] = traceID
	}
	reports := make([]smsStatusReport, 0, len(cases))
	for _, tc := range cases {
		reports = append(reports, smsStatusReport{SMSStatus: tc.providerStatus, UUIDs: []string{tc.uuid}})
	}
	if result := postSMSStatusJSONRPC(t, server, reports); result != "OK" {
		t.Fatalf("sms status matrix result = %v", result)
	}
	for _, tc := range cases {
		notificationRows, err := server.Env.Model("mail.notification").Browse(notificationIDs[tc.uuid]).Read("notification_status", "failure_type", "failure_reason")
		if err != nil {
			t.Fatal(err)
		}
		traceRows, err := server.Env.Model("mailing.trace").Browse(traceIDs[tc.uuid]).Read("trace_status", "failure_type", "failure_reason")
		if err != nil {
			t.Fatal(err)
		}
		if len(notificationRows) != 1 || notificationRows[0]["notification_status"] != tc.notificationStatus || stringValue(notificationRows[0]["failure_type"]) != "" || stringValue(notificationRows[0]["failure_reason"]) != "" {
			t.Fatalf("%s notification rows = %+v", tc.providerStatus, notificationRows)
		}
		if len(traceRows) != 1 || traceRows[0]["trace_status"] != tc.traceStatus || stringValue(traceRows[0]["failure_type"]) != "" || stringValue(traceRows[0]["failure_reason"]) != "" {
			t.Fatalf("%s trace rows = %+v", tc.providerStatus, traceRows)
		}
	}
}

func TestSMSDeliveryStatusWebhookUpdatesMailingTracesAndMailingState(t *testing.T) {
	server := testMailThreadServer(t)
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Status Mailing", "state": "sending"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Status Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	processUUID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pendingUUID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	bounceUUID := "cccccccccccccccccccccccccccccccc"
	errorUUID := "dddddddddddddddddddddddddddddddd"
	unknownUUID := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	traceIDs := map[string]int64{}
	for uuid, status := range map[string]string{
		processUUID: "process",
		pendingUUID: "pending",
		bounceUUID:  "pending",
		errorUUID:   "pending",
		unknownUUID: "pending",
	} {
		smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"uuid": uuid, "number": "+15550101", "body": "Hello", "state": "outgoing", "mailing_id": mailingID})
		if err != nil {
			t.Fatal(err)
		}
		traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": partnerID, "sms_id_int": smsID, "sms_number": "+15550101", "trace_status": status})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.Env.Model("sms.tracker").Create(map[string]any{"sms_uuid": uuid, "mailing_trace_id": traceID}); err != nil {
			t.Fatal(err)
		}
		traceIDs[uuid] = traceID
	}

	if result := postSMSStatusJSONRPC(t, server, []smsStatusReport{{SMSStatus: "sent", UUIDs: []string{processUUID}}}); result != "OK" {
		t.Fatalf("process result = %v", result)
	}
	rows, err := server.Env.Model("mailing.trace").Browse(traceIDs[processUUID]).Read("trace_status", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["trace_status"] != "pending" || accountingDateValue(rows[0]["sent_datetime"]).IsZero() {
		t.Fatalf("process trace rows = %+v", rows)
	}
	mailingRows, err := server.Env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date", "kpi_mail_required")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailingRows) != 1 || mailingRows[0]["state"] != "done" || accountingDateValue(mailingRows[0]["sent_date"]).IsZero() || mailingRows[0]["kpi_mail_required"] != true {
		t.Fatalf("mailing rows after process = %+v", mailingRows)
	}

	if err := server.Env.Model("mailing.mailing").Browse(mailingID).Write(map[string]any{"state": "sending", "sent_date": time.Time{}, "kpi_mail_required": false}); err != nil {
		t.Fatal(err)
	}
	if result := postSMSStatusJSONRPC(t, server, []smsStatusReport{
		{SMSStatus: "delivered", UUIDs: []string{pendingUUID}},
		{SMSStatus: "invalid_destination", UUIDs: []string{bounceUUID}},
		{SMSStatus: "not_delivered", UUIDs: []string{errorUUID}},
		{SMSStatus: "provider_new", UUIDs: []string{unknownUUID}},
	}); result != "OK" {
		t.Fatalf("mixed result = %v", result)
	}
	rows, err = server.Env.Model("mailing.trace").Browse(traceIDs[pendingUUID], traceIDs[bounceUUID], traceIDs[errorUUID], traceIDs[unknownUUID]).Read("trace_status", "failure_type", "failure_reason", "sent_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 ||
		rows[0]["trace_status"] != "sent" || accountingDateValue(rows[0]["sent_datetime"]).IsZero() ||
		rows[1]["trace_status"] != "bounce" || rows[1]["failure_type"] != "sms_invalid_destination" ||
		rows[2]["trace_status"] != "error" || rows[2]["failure_type"] != "sms_not_delivered" ||
		rows[3]["trace_status"] != "error" || rows[3]["failure_type"] != "unknown" || rows[3]["failure_reason"] != "provider_new" {
		t.Fatalf("mixed trace rows = %+v", rows)
	}
}

func TestSMSDeliveryStatusWebhookPublishesNotificationUpdateBus(t *testing.T) {
	server := testMailThreadServer(t)
	server.Bus = notifications.NewBus(100)
	messageID, err := server.Env.Model("mail.message").Create(map[string]any{"model": "res.partner", "res_id": int64(1), "body": "SMS"})
	if err != nil {
		t.Fatal(err)
	}
	uuid := "abababababababababababababababab"
	smsID, err := server.Env.Model("sms.sms").Create(map[string]any{"uuid": uuid, "number": "+15550101", "body": "Hello", "state": "outgoing"})
	if err != nil {
		t.Fatal(err)
	}
	notificationID, err := server.Env.Model("mail.notification").Create(map[string]any{"mail_message_id": messageID, "notification_type": "sms", "notification_status": "pending", "sms_id_int": smsID, "sms_number": "+15550101"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("sms.tracker").Create(map[string]any{"sms_uuid": uuid, "mail_notification_id": notificationID}); err != nil {
		t.Fatal(err)
	}
	sub := server.Bus.Subscribe(userBusChannel(1), 0)
	defer sub.Close()
	if result := postSMSStatusJSONRPC(t, server, []smsStatusReport{{SMSStatus: "delivered", UUIDs: []string{uuid}}}); result != "OK" {
		t.Fatalf("sms status bus result = %v", result)
	}
	event := nextHTTPBusEvent(t, sub)
	if event.Name != "mail.record/insert" {
		t.Fatalf("sms status bus event name = %s", event.Name)
	}
	messageRows := event.Payload["mail.message"].([]map[string]any)
	if len(messageRows) != 1 || int64Value(messageRows[0]["id"]) != messageID {
		t.Fatalf("sms status bus payload = %+v", event.Payload)
	}
}

func TestSMSDeliveryStatusWebhookRejectsBadPayloadAndAllowsMissingUUID(t *testing.T) {
	server := testMailThreadServer(t)
	for _, report := range []smsStatusReport{
		{SMSStatus: "delivered", UUIDs: []string{"not-a-uuid"}},
		{SMSStatus: "delivered", UUIDs: []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}},
		{SMSStatus: "delivered", UUIDs: nil},
		{SMSStatus: "", UUIDs: []string{"11111111111111111111111111111111"}},
		{SMSStatus: "not delivered", UUIDs: []string{"11111111111111111111111111111111"}},
	} {
		rec := postSMSStatusRaw(t, server, []smsStatusReport{report})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Bad parameters") {
			t.Fatalf("bad payload response %d body=%s report=%+v", rec.Code, rec.Body.String(), report)
		}
	}
	if result := postSMSStatusJSONRPC(t, server, []smsStatusReport{{SMSStatus: "delivered", UUIDs: []string{"00000000000000000000000000000000"}}}); result != "OK" {
		t.Fatalf("missing uuid result = %v", result)
	}
}

func TestSMSDeliveryStatusWebhookRejectsMissingMessageStatusesAndAllowsEmptyList(t *testing.T) {
	server := testMailThreadServer(t)
	rec := postSMSStatusParamsRaw(t, server, map[string]any{})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Bad parameters") {
		t.Fatalf("missing message_statuses response %d body=%s", rec.Code, rec.Body.String())
	}
	if result := postSMSStatusJSONRPC(t, server, []smsStatusReport{}); result != "OK" {
		t.Fatalf("empty message_statuses result = %v", result)
	}
}

func TestWhatsAppWebhookRouteValidation(t *testing.T) {
	server := testMailThreadServer(t)
	accountID, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret", "webhook_verify_token": "verify-me"})
	if err != nil {
		t.Fatal(err)
	}
	if accountID == 0 {
		t.Fatal("missing account id")
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whatsapp/webhook/?hub.mode=subscribe&hub.verify_token=verify-me&hub.challenge=challenge-123", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "challenge-123" {
		t.Fatalf("verify response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whatsapp/webhook/?hub.mode=subscribe&hub.verify_token=bad&hub.challenge=challenge-123", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad verify response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/whatsapp/webhook/", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("method response %d allow=%s body=%s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/whatsapp/webhook/", bytes.NewBufferString("{bad")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json response %d %s", rec.Code, rec.Body.String())
	}
}

func TestWhatsAppWebhookUpdatesTemplateStatusQualityCategory(t *testing.T) {
	server := testMailThreadServer(t)
	accountID, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret", "webhook_verify_token": "verify-me"})
	if err != nil {
		t.Fatal(err)
	}
	otherAccountID, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Other Meta", "account_uid": "acct-2", "phone_uid": "phone-2", "app_secret": "other-secret"})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("whatsapp.template").Create(map[string]any{"name": "Template", "wa_template_uid": "987", "wa_account_id": accountID, "active": false, "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := server.Env.Model("whatsapp.template").Create(map[string]any{"name": "Other", "wa_template_uid": "987", "wa_account_id": otherAccountID, "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "message_template_status_update",
			"value": map[string]any{"message_template_id": "987", "event": "REJECTED", "other_info": map[string]any{"description": "Policy issue"}},
		}},
	}}})
	postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "message_template_quality_update",
			"value": map[string]any{"message_template_id": "987", "new_quality_score": "UNKNOWN"},
		}, {
			"field": "template_category_update",
			"value": map[string]any{"message_template_id": "987", "new_category": "UTILITY"},
		}, {
			"field": "message_template_status_update",
			"value": map[string]any{"message_template_id": "missing", "event": "APPROVED"},
		}},
	}}})
	rows, err := server.Env.Model("whatsapp.template").Browse(templateID, otherID).Read("status", "quality", "template_type")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["status"] != "rejected" || rows[0]["quality"] != "none" || rows[0]["template_type"] != "utility" {
		t.Fatalf("template rows = %+v", rows)
	}
	if rows[1]["status"] != "draft" || rows[1]["quality"] != "none" || rows[1]["template_type"] != "marketing" {
		t.Fatalf("other account row changed = %+v", rows[1])
	}
	messages, err := server.Env.Model("mail.message").Search(domain.Cond("model", domain.Equal, "whatsapp.template"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || int64Value(messageRows[0]["res_id"]) != templateID || !strings.Contains(stringValue(messageRows[0]["body"]), "Policy issue") {
		t.Fatalf("rejection messages = %+v", messageRows)
	}
}

func TestWhatsAppWebhookRejectsBadSignatureAndIgnoresNonTemplateEvents(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret"}); err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("whatsapp.template").Create(map[string]any{"name": "Template", "wa_template_uid": "987", "status": "draft", "quality": "none", "template_type": "marketing"})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"entry": []map[string]any{{"id": "acct-1", "changes": []map[string]any{{"field": "messages", "value": map[string]any{"metadata": map[string]any{"phone_number_id": "phone-1"}}}}}}}
	rec := postWhatsAppWebhookPayload(t, server, "bad-secret", payload)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad signature response %d %s", rec.Code, rec.Body.String())
	}
	rec = postWhatsAppWebhookPayload(t, server, "secret", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("non-template response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("whatsapp.template").Browse(templateID).Read("status", "quality", "template_type")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["status"] != "draft" || rows[0]["quality"] != "none" || rows[0]["template_type"] != "marketing" {
		t.Fatalf("template changed = %+v", rows)
	}
	messages, err := server.Env.Model("mail.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Len() != 0 {
		t.Fatalf("mail message side effects = %d", messages.Len())
	}
}

func TestWhatsAppWebhookMessageStatusStateMatrix(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret"}); err != nil {
		t.Fatal(err)
	}
	messageIDs := map[string]int64{}
	for _, uid := range []string{"wamid.sent", "wamid.delivered", "wamid.read", "wamid.failed", "wamid.bounced"} {
		id, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "outgoing", "msg_uid": uid})
		if err != nil {
			t.Fatal(err)
		}
		messageIDs[uid] = id
	}
	rec := postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "messages",
			"value": map[string]any{
				"metadata": map[string]any{"phone_number_id": "phone-1"},
				"statuses": []map[string]any{
					{"id": "wamid.sent", "status": "sent"},
					{"id": "wamid.delivered", "status": "delivered"},
					{"id": "wamid.read", "status": "read"},
					{"id": "wamid.failed", "status": "failed", "errors": []map[string]any{{"code": int64(131000), "title": "Temporary failure"}}},
					{"id": "wamid.bounced", "status": "failed", "errors": []map[string]any{{"code": int64(131045), "title": "Registration issue"}}},
				},
			},
		}},
	}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status webhook response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("whatsapp.message").Browse(
		messageIDs["wamid.sent"],
		messageIDs["wamid.delivered"],
		messageIDs["wamid.read"],
		messageIDs["wamid.failed"],
		messageIDs["wamid.bounced"],
	).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		state         string
		failureType   string
		failureReason string
	}{
		{"sent", "", ""},
		{"delivered", "", ""},
		{"read", "", ""},
		{"error", "whatsapp_recoverable", "131000 : Temporary failure"},
		{"bounced", "whatsapp_recoverable", "131045 : Registration issue"},
	}
	for i, row := range rows {
		if row["state"] != want[i].state || stringValue(row["failure_type"]) != want[i].failureType || stringValue(row["failure_reason"]) != want[i].failureReason {
			t.Fatalf("message row %d = %+v want %+v", i, row, want[i])
		}
	}
}

func TestWhatsAppWebhookMessageStatusScopingSignatureAndUnknownIDs(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Other Meta", "account_uid": "acct-2", "phone_uid": "phone-2", "app_secret": "other-secret"}); err != nil {
		t.Fatal(err)
	}
	messageID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent", "msg_uid": "wamid.scope"})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "messages",
			"value": map[string]any{"metadata": map[string]any{"phone_number_id": "phone-1"}, "statuses": []map[string]any{{"id": "wamid.scope", "status": "read"}}},
		}},
	}}}
	rec := postWhatsAppWebhookPayload(t, server, "bad-secret", payload)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad signature response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("whatsapp.message").Browse(messageID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "sent" {
		t.Fatalf("bad signature mutated message = %+v", rows)
	}
	rec = postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "messages",
			"value": map[string]any{"metadata": map[string]any{"phone_number_id": "phone-2"}, "statuses": []map[string]any{{"id": "wamid.scope", "status": "read"}}},
		}},
	}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("phone mismatch response %d body=%s", rec.Code, rec.Body.String())
	}
	rec = postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "messages",
			"value": map[string]any{"metadata": map[string]any{"phone_number_id": "phone-1"}, "statuses": []map[string]any{{"id": "wamid.missing", "status": "read"}}},
		}},
	}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("unknown id response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("whatsapp.message").Browse(messageID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "sent" {
		t.Fatalf("scoping/unknown mutated message = %+v", rows)
	}
}

func TestWhatsAppWebhookMessageStatusProcessesMarketingTraceEvents(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("whatsapp.account").Create(map[string]any{"name": "Meta", "account_uid": "acct-1", "phone_uid": "phone-1", "app_secret": "secret"}); err != nil {
		t.Fatal(err)
	}
	readActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Read", "trigger_type": "whatsapp_read"})
	if err != nil {
		t.Fatal(err)
	}
	notReadActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Not Read", "trigger_type": "whatsapp_not_read"})
	if err != nil {
		t.Fatal(err)
	}
	bouncedActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Bounced", "trigger_type": "whatsapp_bounced"})
	if err != nil {
		t.Fatal(err)
	}
	notClickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Not Click", "trigger_type": "whatsapp_not_click"})
	if err != nil {
		t.Fatal(err)
	}
	readMessageID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "delivered", "msg_uid": "wamid.read.event"})
	if err != nil {
		t.Fatal(err)
	}
	readTraceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": readActivityID, "whatsapp_message_id": readMessageID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	readChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": readActivityID, "parent_id": readTraceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	notReadChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notReadActivityID, "parent_id": readTraceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	bouncedMessageID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent", "msg_uid": "wamid.bounced.event"})
	if err != nil {
		t.Fatal(err)
	}
	bouncedTraceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": bouncedActivityID, "whatsapp_message_id": bouncedMessageID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	bouncedChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": bouncedActivityID, "parent_id": bouncedTraceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	cancelledChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notClickActivityID, "parent_id": bouncedTraceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}

	rec := postWhatsAppWebhookPayload(t, server, "secret", map[string]any{"entry": []map[string]any{{
		"id": "acct-1",
		"changes": []map[string]any{{
			"field": "messages",
			"value": map[string]any{
				"metadata": map[string]any{"phone_number_id": "phone-1"},
				"statuses": []map[string]any{
					{"id": "wamid.read.event", "status": "read"},
					{"id": "wamid.bounced.event", "status": "failed", "errors": []map[string]any{{"code": int64(131026), "title": "Unable to deliver"}}},
				},
			},
		}},
	}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status marketing response %d body=%s", rec.Code, rec.Body.String())
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(readMessageID, bouncedMessageID).Read("state", "failure_type", "failure_reason")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 2 || messageRows[0]["state"] != "read" || messageRows[1]["state"] != "bounced" || messageRows[1]["failure_type"] != "whatsapp_unrecoverable" || messageRows[1]["failure_reason"] != "131026 : Unable to deliver" {
		t.Fatalf("message status rows = %+v", messageRows)
	}
	parentRows, err := server.Env.Model("marketing.trace").Browse(readTraceID, bouncedTraceID).Read("state", "schedule_date", "state_msg")
	if err != nil {
		t.Fatal(err)
	}
	if parentRows[0]["state"] != "scheduled" || !accountingDateValue(parentRows[0]["schedule_date"]).IsZero() || stringValue(parentRows[0]["state_msg"]) != "" {
		t.Fatalf("read parent row = %+v", parentRows[0])
	}
	if parentRows[1]["state"] != "canceled" || accountingDateValue(parentRows[1]["schedule_date"]).IsZero() || parentRows[1]["state_msg"] != "WhatsApp canceled" {
		t.Fatalf("bounced parent row = %+v", parentRows[1])
	}
	childRows, err := server.Env.Model("marketing.trace").Browse(readChildID, notReadChildID, bouncedChildID, cancelledChildID).Read("id", "state", "schedule_date", "state_msg")
	if err != nil {
		t.Fatal(err)
	}
	children := map[int64]map[string]any{}
	for _, row := range childRows {
		children[int64Value(row["id"])] = row
	}
	if row := children[readChildID]; row["state"] != "processed" || accountingDateValue(row["schedule_date"]).IsZero() || stringValue(row["state_msg"]) != "" {
		t.Fatalf("read child row = %+v", row)
	}
	if row := children[notReadChildID]; row["state"] != "canceled" || accountingDateValue(row["schedule_date"]).IsZero() || row["state_msg"] != "Parent Whatsapp message got opened" {
		t.Fatalf("not-read child row = %+v", row)
	}
	if row := children[bouncedChildID]; row["state"] != "processed" || accountingDateValue(row["schedule_date"]).IsZero() || stringValue(row["state_msg"]) != "" {
		t.Fatalf("bounced child row = %+v", row)
	}
	if row := children[cancelledChildID]; row["state"] != "canceled" || accountingDateValue(row["schedule_date"]).IsZero() || row["state_msg"] != "Parent whatsapp was bounced" {
		t.Fatalf("cancelled bounced child row = %+v", row)
	}
}

func postWhatsAppWebhookPayload(t *testing.T, server Server, secret string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/whatsapp/webhook/", bytes.NewReader(body))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func postSMSStatusJSONRPC(t *testing.T, server Server, reports []smsStatusReport) any {
	t.Helper()
	rec := postSMSStatusRaw(t, server, reports)
	if rec.Code != http.StatusOK {
		t.Fatalf("sms status response %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload["result"]
}

func postSMSStatusRaw(t *testing.T, server Server, reports []smsStatusReport) *httptest.ResponseRecorder {
	t.Helper()
	return postSMSStatusParamsRaw(t, server, map[string]any{"message_statuses": reports})
}

func postSMSStatusParamsRaw(t *testing.T, server Server, params map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"params":  params,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sms/status", bytes.NewReader(payload)))
	return rec
}

func TestWhatsAppTrackedLinkClickMarksMessageAndRedirects(t *testing.T) {
	server := testMailThreadServer(t)
	utmCampaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "WhatsApp Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	linkCampaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Tracker Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	countryID, err := server.Env.Model("res.country").Create(map[string]any{"name": "Zedland", "code": "ZZ"})
	if err != nil {
		t.Fatal(err)
	}
	marketingCampaignID, err := server.Env.Model("marketing.campaign").Create(map[string]any{"name": "Automation Campaign", "utm_campaign_id": utmCampaignID})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Activity", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	notClickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Not Click", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_not_click"})
	if err != nil {
		t.Fatal(err)
	}
	readActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Read", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_read"})
	if err != nil {
		t.Fatal(err)
	}
	delayedClickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Click Later", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click", "interval_number": int64(2), "interval_type": "days"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	secondTraceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	notClickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notClickActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	clickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	secondNotClickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notClickActivityID, "parent_id": secondTraceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	readChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": readActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	delayedClickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": delayedClickActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa/path?x=1", "code": "WaClick1", "campaign_id": linkCampaignID})
	if err != nil {
		t.Fatal(err)
	}

	beforeClick := time.Now().UTC()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaClick1/w/%d", whatsappID), nil)
	req.RemoteAddr = "203.0.113.9:61234"
	req.Header.Set("X-GeoIP-Country-Code", "ZZ")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa/path?utm_campaign=Tracker+Campaign&x=1" {
		t.Fatalf("whatsapp redirect response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id", "campaign_id", "mailing_trace_id", "mass_mailing_id", "ip", "country_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID || int64Value(clickRows[0]["campaign_id"]) != utmCampaignID || int64Value(clickRows[0]["campaign_id"]) == linkCampaignID || int64Value(clickRows[0]["mailing_trace_id"]) != 0 || int64Value(clickRows[0]["mass_mailing_id"]) != 0 || clickRows[0]["ip"] != "203.0.113.9" || int64Value(clickRows[0]["country_id"]) != countryID {
		t.Fatalf("whatsapp click rows = %+v", clickRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("state", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["state"] != "sent" || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("whatsapp message rows = %+v", messageRows)
	}
	traceRows, err := server.Env.Model("marketing.trace").Browse(traceID, secondTraceID).Read("id", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 2 || accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() || accountingDateValue(traceRows[1]["links_click_datetime"]).IsZero() {
		t.Fatalf("marketing trace rows = %+v", traceRows)
	}
	childRows, err := server.Env.Model("marketing.trace").Browse(notClickChildID, clickChildID, secondNotClickChildID, readChildID, delayedClickChildID).Read("id", "state", "schedule_date", "state_msg")
	if err != nil {
		t.Fatal(err)
	}
	children := map[int64]map[string]any{}
	for _, row := range childRows {
		children[int64Value(row["id"])] = row
	}
	for _, childID := range []int64{notClickChildID, secondNotClickChildID} {
		row := children[childID]
		if row["state"] != "canceled" || accountingDateValue(row["schedule_date"]).IsZero() || row["state_msg"] != "Parent Whatsapp message was clicked" {
			t.Fatalf("not-click child row = %+v", row)
		}
	}
	if row := children[readChildID]; row["state"] != "scheduled" || !accountingDateValue(row["schedule_date"]).IsZero() || strings.TrimSpace(stringValue(row["state_msg"])) != "" {
		t.Fatalf("read child row = %+v", row)
	}
	if row := children[clickChildID]; row["state"] != "processed" || accountingDateValue(row["schedule_date"]).IsZero() || strings.TrimSpace(stringValue(row["state_msg"])) != "" {
		t.Fatalf("click child row = %+v", row)
	}
	if row := children[delayedClickChildID]; row["state"] != "scheduled" || accountingDateValue(row["schedule_date"]).Before(beforeClick.Add(47*time.Hour)) || strings.TrimSpace(stringValue(row["state_msg"])) != "" {
		t.Fatalf("delayed click child row = %+v", row)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("whatsapp link rows = %+v", linkRows)
	}
}

func TestWhatsAppTrackedLinkUsesRemoteAddrLikeSourceRoute(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-forwarded-ip", "code": "WaIP1"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaIP1/w/%d", whatsappID), nil)
	req.RemoteAddr = "10.0.0.9:43210"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-forwarded-ip" {
		t.Fatalf("forwarded IP redirect response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("ip", "whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || clickRows[0]["ip"] != "10.0.0.9" || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID {
		t.Fatalf("remote address click rows = %+v", clickRows)
	}
}

func TestWhatsAppTrackedLinkStoppedCampaignDoesNotProcessChildEvents(t *testing.T) {
	server := testMailThreadServer(t)
	utmCampaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Stopped WhatsApp Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	marketingCampaignID, err := server.Env.Model("marketing.campaign").Create(map[string]any{"name": "Stopped Automation Campaign", "utm_campaign_id": utmCampaignID, "state": "stopped"})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Stopped Parent", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	clickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Stopped Click Child", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	notClickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Stopped Not Click Child", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_not_click"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	clickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": clickActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	notClickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notClickActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-stopped", "code": "WaStopped1"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaStopped1/w/%d", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-stopped" {
		t.Fatalf("stopped campaign response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 1 {
		t.Fatalf("stopped campaign click count = %d", clicks.Len())
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("stopped campaign message rows = %+v", messageRows)
	}
	childRows, err := server.Env.Model("marketing.trace").Browse(clickChildID, notClickChildID).Read("id", "state", "schedule_date", "state_msg")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range childRows {
		if row["state"] != "scheduled" || !accountingDateValue(row["schedule_date"]).IsZero() || strings.TrimSpace(stringValue(row["state_msg"])) != "" {
			t.Fatalf("stopped campaign child row = %+v", row)
		}
	}
}

func TestWhatsAppTrackedLinkFallsBackToTrackerCampaign(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Tracker Only"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-fallback?x=1", "code": "WaFallback1", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaFallback1/w/%d", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-fallback?utm_campaign=Tracker+Only&x=1" {
		t.Fatalf("fallback whatsapp redirect response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID || int64Value(clickRows[0]["campaign_id"]) != campaignID {
		t.Fatalf("fallback whatsapp click rows = %+v", clickRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("fallback whatsapp message rows = %+v", messageRows)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("fallback whatsapp link rows = %+v", linkRows)
	}
}

func TestWhatsAppTrackedLinkTraceWithoutUTMCampaignFallsBackToTrackerCampaign(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Tracker Fallback"})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Activity", "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-trace-fallback?x=1", "code": "WaTraceFallback1", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaTraceFallback1/w/%d", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-trace-fallback?utm_campaign=Tracker+Fallback&x=1" {
		t.Fatalf("trace fallback whatsapp response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID || int64Value(clickRows[0]["campaign_id"]) != campaignID {
		t.Fatalf("trace fallback whatsapp click rows = %+v", clickRows)
	}
	traceRows, err := server.Env.Model("marketing.trace").Browse(traceID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("trace fallback whatsapp trace rows = %+v", traceRows)
	}
}

func TestWhatsAppTrackedLinkRedirectedURLWinsOverTrackedURL(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Ignored UTM"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{
		"url":            "https://www.example.com/source?x=1",
		"redirected_url": "https://redirect.example/final?keep=1",
		"code":           "WaRedirect1",
		"campaign_id":    campaignID,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaRedirect1/w/%d", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://redirect.example/final?keep=1" {
		t.Fatalf("redirected whatsapp response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 1 {
		t.Fatalf("redirected whatsapp click count = %d", clicks.Len())
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("redirected whatsapp link rows = %+v", linkRows)
	}
}

func TestWhatsAppTrackedLinkIgnoresIncomingRouteQuery(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Tracker Query"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{
		"url":         "https://www.example.com/wa-query?keep=1",
		"code":        "WaQuery1",
		"campaign_id": campaignID,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaQuery1/w/%d?utm_campaign=Injected&redirect=https://evil.example", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-query?keep=1&utm_campaign=Tracker+Query" {
		t.Fatalf("query whatsapp response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID || int64Value(clickRows[0]["campaign_id"]) != campaignID {
		t.Fatalf("query whatsapp click rows = %+v", clickRows)
	}
}

func TestWhatsAppTrackedLinkMissingMessageCreatesPlainClick(t *testing.T) {
	server := testMailThreadServer(t)
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/missing-wa", "code": "MissingWa1"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r/MissingWa1/w/999999", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/missing-wa" {
		t.Fatalf("missing whatsapp redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != 0 || int64Value(clickRows[0]["campaign_id"]) != 0 {
		t.Fatalf("missing whatsapp click rows = %+v", clickRows)
	}
}

func TestWhatsAppTrackedLinkNoExternalTrackingSuppressesRedirectUTMButKeepsClickCampaign(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "web.base.url", "value": "https://gorp.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "link_tracker.no_external_tracking", "value": "true"}); err != nil {
		t.Fatal(err)
	}
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Suppressed Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://external.example/wa?x=1", "campaign_id": campaignID, "code": "WaNoExt1"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaNoExt1/w/%d", whatsappID), nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://external.example/wa?x=1" {
		t.Fatalf("no external tracking response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID || int64Value(clickRows[0]["campaign_id"]) != campaignID {
		t.Fatalf("no external tracking click rows = %+v", clickRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("no external tracking message rows = %+v", messageRows)
	}
}

func TestWhatsAppTrackedLinkMissingMessageIgnoresOrphanTraceMetadata(t *testing.T) {
	server := testMailThreadServer(t)
	traceCampaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Orphan Trace Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	trackerCampaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Tracker Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	marketingCampaignID, err := server.Env.Model("marketing.campaign").Create(map[string]any{"name": "Orphan Marketing", "utm_campaign_id": traceCampaignID})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "Orphan WhatsApp Click", "campaign_id": marketingCampaignID, "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": int64(999999), "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/orphan-wa", "code": "WaOrphan1", "campaign_id": trackerCampaignID})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r/WaOrphan1/w/999999", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/orphan-wa?utm_campaign=Tracker+Campaign" {
		t.Fatalf("orphan whatsapp response %d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id", "campaign_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["whatsapp_message_id"]) != 0 || int64Value(clickRows[0]["campaign_id"]) != trackerCampaignID {
		t.Fatalf("orphan whatsapp click rows = %+v", clickRows)
	}
	traceRows, err := server.Env.Model("marketing.trace").Browse(traceID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || !accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("orphan whatsapp trace rows = %+v", traceRows)
	}
}

func TestWhatsAppTrackedLinkZeroMessageIDCreatesPlainClick(t *testing.T) {
	server := testMailThreadServer(t)
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/zero-wa", "code": "ZeroWa1"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/r/ZeroWa1/w/0", nil)
	req.Header.Set("User-Agent", "Googlebot/2.1")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/zero-wa" {
		t.Fatalf("zero whatsapp redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != 0 {
		t.Fatalf("zero whatsapp click rows = %+v", clickRows)
	}
}

func TestParseLinkTrackerPathWhatsAppParityEdges(t *testing.T) {
	code, traceID, whatsappID, smsID, ok := parseLinkTrackerPath("/r/WaZero/w/0")
	if !ok || code != "WaZero" || traceID != 0 || whatsappID != 0 || smsID != 0 {
		t.Fatalf("zero whatsapp parse code=%q trace=%d whatsapp=%d sms=%d ok=%v", code, traceID, whatsappID, smsID, ok)
	}
	for _, path := range []string{"/r/WaSlash/w/1/", "/r//WaSlash/w/1", "/r/WaSlash//w/1", "/r/WaSlash/w//1"} {
		if code, traceID, whatsappID, smsID, ok := parseLinkTrackerPath(path); ok {
			t.Fatalf("extra slash path %q parsed as code=%q trace=%d whatsapp=%d sms=%d", path, code, traceID, whatsappID, smsID)
		}
	}
	for _, path := range []string{"/r/WaZero/m/0", "/r/WaZero/s/0"} {
		if code, traceID, whatsappID, smsID, ok := parseLinkTrackerPath(path); ok {
			t.Fatalf("zero non-whatsapp path %q parsed as code=%q trace=%d whatsapp=%d sms=%d", path, code, traceID, whatsappID, smsID)
		}
	}
}

func TestWhatsAppTrackedLinkBotStillTracksLikeSourceRoute(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-bot", "code": "WaBot1"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaBot1/w/%d", whatsappID), nil)
	req.Header.Set("User-Agent", "WhatsApp/2.24 Preview")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-bot" {
		t.Fatalf("whatsapp bot redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID {
		t.Fatalf("whatsapp bot click rows = %+v", clickRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("whatsapp bot message rows = %+v", messageRows)
	}
}

func TestWhatsAppTrackedLinkUnsafeMethodsRequireCSRFLikeSourceRoute(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-post-bot", "code": "WaPostBot1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, fmt.Sprintf("/r/WaPostBot1/w/%d", whatsappID), nil)
		req.Header.Set("User-Agent", "WhatsApp/2.24 Preview")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Session expired") {
			t.Fatalf("%s response %d location=%s body=%s", method, rec.Code, rec.Header().Get("Location"), rec.Body.String())
		}
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("unsafe method click count = %d", clicks.Len())
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || !accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("unsafe method message rows = %+v", messageRows)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 0 {
		t.Fatalf("unsafe method link rows = %+v", linkRows)
	}
}

func TestWhatsAppTrackedLinkOtherMethodsStillTrackLikeSourceRoute(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-methods", "code": "WaMethods1"})
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodHead, http.MethodOptions, http.MethodTrace} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(method, fmt.Sprintf("/r/WaMethods1/w/%d", whatsappID), nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-methods" {
			t.Fatalf("%s response %d location=%s body=%s", method, rec.Code, rec.Header().Get("Location"), rec.Body.String())
		}
	}

	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 3 {
		t.Fatalf("method click rows = %+v", clickRows)
	}
	for _, row := range clickRows {
		if int64Value(row["whatsapp_message_id"]) != whatsappID {
			t.Fatalf("method click row missing whatsapp message: %+v", row)
		}
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 3 {
		t.Fatalf("method link rows = %+v", linkRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("method message rows = %+v", messageRows)
	}
}

func TestWhatsAppTrackedLinkRepeatedClicksCreateSeparateClicks(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Click", "trigger_type": "whatsapp_click"})
	if err != nil {
		t.Fatal(err)
	}
	notClickActivityID, err := server.Env.Model("marketing.activity").Create(map[string]any{"name": "WhatsApp Not Click", "trigger_type": "whatsapp_not_click"})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "whatsapp_message_id": whatsappID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	clickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": activityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	notClickChildID, err := server.Env.Model("marketing.trace").Create(map[string]any{"activity_id": notClickActivityID, "parent_id": traceID, "state": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-repeat", "code": "WaRepeat1"})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaRepeat1/w/%d", whatsappID), nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/wa-repeat" {
			t.Fatalf("repeat click %d response %d location=%s body=%s", i+1, rec.Code, rec.Header().Get("Location"), rec.Body.String())
		}
	}

	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 2 {
		t.Fatalf("repeat click rows = %+v", clickRows)
	}
	for _, row := range clickRows {
		if int64Value(row["whatsapp_message_id"]) != whatsappID {
			t.Fatalf("repeat click row missing whatsapp message: %+v", row)
		}
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 2 {
		t.Fatalf("repeat link rows = %+v", linkRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("repeat message rows = %+v", messageRows)
	}
	childRows, err := server.Env.Model("marketing.trace").Browse(clickChildID, notClickChildID).Read("id", "state", "schedule_date", "state_msg")
	if err != nil {
		t.Fatal(err)
	}
	children := map[int64]map[string]any{}
	for _, row := range childRows {
		children[int64Value(row["id"])] = row
	}
	if row := children[clickChildID]; row["state"] != "processed" || accountingDateValue(row["schedule_date"]).IsZero() || strings.TrimSpace(stringValue(row["state_msg"])) != "" {
		t.Fatalf("repeat click child row = %+v", row)
	}
	if row := children[notClickChildID]; row["state"] != "canceled" || accountingDateValue(row["schedule_date"]).IsZero() || row["state_msg"] != "Parent Whatsapp message was clicked" {
		t.Fatalf("repeat not-click child row = %+v", row)
	}
}

func TestWhatsAppTrackedLinkMissingCodeCreatesNoClick(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/MissingWaCode/w/%d", whatsappID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing whatsapp code response %d body=%s", rec.Code, rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("missing whatsapp code created clicks: %d", clicks.Len())
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || !accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("missing whatsapp code message rows = %+v", messageRows)
	}
}

func TestWhatsAppTrackedLinkUnsafeAndMalformedCreateNoExtraClick(t *testing.T) {
	server := testMailThreadServer(t)
	whatsappID, err := server.Env.Model("whatsapp.message").Create(map[string]any{"state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/wa-safe", "code": "WaSafe1"})
	if err != nil {
		t.Fatal(err)
	}

	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/WaSafe1/w/%d", whatsappID), nil))
	if getRec.Code != http.StatusMovedPermanently || getRec.Header().Get("Location") != "https://www.example.com/wa-safe" {
		t.Fatalf("get response %d location=%s body=%s", getRec.Code, getRec.Header().Get("Location"), getRec.Body.String())
	}
	postRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(postRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/r/WaSafe1/w/%d", whatsappID), nil))
	if postRec.Code != http.StatusBadRequest || !strings.Contains(postRec.Body.String(), "Session expired") {
		t.Fatalf("post response %d location=%s body=%s", postRec.Code, postRec.Header().Get("Location"), postRec.Body.String())
	}
	for _, path := range []string{
		"/r/WaSafe1/w/not-a-number",
		"/r/WaSafe1/w/-1",
		fmt.Sprintf("/r/WaSafe1/w/%d/extra", whatsappID),
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("malformed path %s %s response %d body=%s", method, path, rec.Code, rec.Body.String())
			}
		}
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 1 {
		t.Fatalf("unsafe/malformed click count = %d", clicks.Len())
	}
	clickRows, err := clicks.Read("link_id", "whatsapp_message_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["link_id"]) != linkID || int64Value(clickRows[0]["whatsapp_message_id"]) != whatsappID {
		t.Fatalf("unsafe/malformed click rows = %+v", clickRows)
	}
	messageRows, err := server.Env.Model("whatsapp.message").Browse(whatsappID).Read("links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || accountingDateValue(messageRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("unsafe/malformed message rows = %+v", messageRows)
	}
}

func TestLinkTrackerRedirectPostRejectedForNonWhatsAppRoutes(t *testing.T) {
	server := testMailThreadServer(t)
	baseLinkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/base-post", "code": "BasePost1"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "POST Rejected", "email": "post.rejected@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_status": "outgoing", "model": "res.partner", "res_id": partnerID})
	if err != nil {
		t.Fatal(err)
	}
	mailLinkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/mail-post", "code": "MailPost1"})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/r/BasePost1", fmt.Sprintf("/r/MailPost1/m/%d", traceID)} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("non-whatsapp post %s response %d allow=%s body=%s", path, rec.Code, rec.Header().Get("Allow"), rec.Body.String())
		}
	}

	clicks, err := server.Env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("non-whatsapp post click count = %d", clicks.Len())
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("links_click_datetime", "trace_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || !accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() || traceRows[0]["trace_status"] != "outgoing" {
		t.Fatalf("non-whatsapp post trace rows = %+v", traceRows)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(baseLinkID, mailLinkID).Read("id", "count")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range linkRows {
		if int64Value(row["count"]) != 0 {
			t.Fatalf("non-whatsapp post link rows = %+v", linkRows)
		}
	}
}

func TestLinkTrackerRedirectSkipsClickForBots(t *testing.T) {
	server := testMailThreadServer(t)
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/bot", "code": "Bot123"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/r/Bot123", nil)
	req.Header.Set("User-Agent", "curl/8.4.0")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/bot" {
		t.Fatalf("bot redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("bot click count = %d", clicks.Len())
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 0 {
		t.Fatalf("bot link count rows = %+v", linkRows)
	}
}

func TestLinkTrackerGenericRedirectSkipsEmailPreviewUserAgents(t *testing.T) {
	server := testMailThreadServer(t)
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/preview", "code": "Prv123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, userAgent := range []string{
		"Mozilla/5.0 MicrosoftPreview/2.0 +https://aka.ms/MicrosoftPreview",
		"Mozilla/5.0 Google-PageRenderer Google (+https://developers.google.com/+/web/snippet/)",
	} {
		req := httptest.NewRequest(http.MethodGet, "/r/Prv123", nil)
		req.Header.Set("User-Agent", userAgent)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/preview" {
			t.Fatalf("preview redirect response %d location=%s ua=%s", rec.Code, rec.Header().Get("Location"), userAgent)
		}
		clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
		if err != nil {
			t.Fatal(err)
		}
		if clicks.Len() != 0 {
			t.Fatalf("preview click count = %d ua=%s", clicks.Len(), userAgent)
		}
		linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
		if err != nil {
			t.Fatal(err)
		}
		if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 0 {
			t.Fatalf("preview link count rows = %+v ua=%s", linkRows, userAgent)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/r/Prv123", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/preview" {
		t.Fatalf("browser redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 1 {
		t.Fatalf("browser click count = %d", clicks.Len())
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("browser link count rows = %+v", linkRows)
	}
}

func TestIsHTTPRequestBotMatchesOdooIrHTTPTokens(t *testing.T) {
	cases := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{"bot", "FriendlyBot/1.0", true},
		{"crawl", "Crawler", true},
		{"slurp", "Yahoo Slurp", true},
		{"spider", "Spider", true},
		{"curl", "curl/8.4.0", true},
		{"wget", "Wget/1.21", true},
		{"facebookexternalhit", "facebookexternalhit/1.1", true},
		{"whatsapp", "WhatsApp/2.24", true},
		{"trendsmapresolver", "TrendsmapResolver", true},
		{"pinterest", "Pinterest/0.2", true},
		{"instagram", "Instagram 310", true},
		{"google-pagerenderer", "Google-PageRenderer", true},
		{"preview", "MicrosoftPreview/2.0", true},
		{"browser", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/r/Test123", nil)
			req.Header.Set("User-Agent", tc.userAgent)
			if got := isHTTPRequestBot(req); got != tc.want {
				t.Fatalf("isHTTPRequestBot(%q) = %v want %v", tc.userAgent, got, tc.want)
			}
		})
	}
}

func TestMassMailingTrackedLinkBotStillTracksLikeSourceRoute(t *testing.T) {
	server := testMailThreadServer(t)
	campaignID, err := server.Env.Model("utm.campaign").Create(map[string]any{"name": "Bot Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "Bot Mailing", "campaign_id": campaignID})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Bot Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"mass_mailing_id": mailingID, "email": "bot@example.com", "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/bot-click", "campaign_id": campaignID, "code": "Wa123"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/Wa123/m/%d", traceID), nil)
	req.Header.Set("User-Agent", "WhatsApp/2.24 Preview")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/bot-click?utm_campaign=Bot+Campaign" {
		t.Fatalf("bot tracked redirect response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 1 {
		t.Fatalf("bot tracked click count = %d", clicks.Len())
	}
	clickRows, err := clicks.Read("mailing_trace_id", "campaign_id", "mass_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["mailing_trace_id"]) != traceID || int64Value(clickRows[0]["campaign_id"]) != campaignID || int64Value(clickRows[0]["mass_mailing_id"]) != mailingID {
		t.Fatalf("bot tracked click rows = %+v", clickRows)
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "open" || accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("bot trace rows = %+v", traceRows)
	}
	linkRows, err := server.Env.Model("link.tracker").Browse(linkID).Read("count")
	if err != nil {
		t.Fatal(err)
	}
	if len(linkRows) != 1 || int64Value(linkRows[0]["count"]) != 1 {
		t.Fatalf("bot tracked link count rows = %+v", linkRows)
	}
}

func TestMassMailingTrackingRejectsInvalidTokenOrIDs(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-reject-secret"}); err != nil {
		t.Fatal(err)
	}
	mailID, err := server.Env.Model("mail.mail").Create(map[string]any{"email_to": "reject@example.com", "state": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Reject Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := server.Env.Model("mailing.trace").Create(map[string]any{"mail_mail_id": mailID, "email": "reject@example.com", "model": "res.partner", "res_id": partnerID, "trace_status": "sent"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Env.Model("link.tracker").Create(map[string]any{"url": "https://www.example.com/ok", "code": "Valid9"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mail/track/%d/bad/blank.gif", mailID), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token response %d %s", rec.Code, rec.Body.String())
	}
	traceRows, err := server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || !accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || !accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("bad token trace rows = %+v", traceRows)
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r/Missing9/m/%d", traceID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad code response %d %s", rec.Code, rec.Body.String())
	}
	clicks, err := server.Env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if clicks.Len() != 0 {
		t.Fatalf("bad code created clicks: %d", clicks.Len())
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r/Valid9/m/999999", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.example.com/ok" {
		t.Fatalf("missing trace response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	clicks, err = server.Env.Model("link.tracker.click").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	clickRows, err := clicks.Read("mailing_trace_id", "mass_mailing_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clickRows) != 1 || int64Value(clickRows[0]["mailing_trace_id"]) != 0 || int64Value(clickRows[0]["mass_mailing_id"]) != 0 {
		t.Fatalf("missing trace click rows = %+v", clickRows)
	}
	traceRows, err = server.Env.Model("mailing.trace").Browse(traceID).Read("trace_status", "open_datetime", "links_click_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(traceRows) != 1 || traceRows[0]["trace_status"] != "sent" || !accountingDateValue(traceRows[0]["open_datetime"]).IsZero() || !accountingDateValue(traceRows[0]["links_click_datetime"]).IsZero() {
		t.Fatalf("missing trace mutated rows = %+v", traceRows)
	}
}

func TestMassMailingReportUnsubscribeDisablesReportsWithValidToken(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-report-secret"}); err != nil {
		t.Fatal(err)
	}
	configID, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "mass_mailing.mass_mailing_reports", "value": "True"})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Email Marketing / User"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "mass_mailing",
		"name":          "group_mass_mailing_user",
		"model":         "res.groups",
		"res_id":        groupID,
		"complete_name": "mass_mailing.group_mass_mailing_user",
	}); err != nil {
		t.Fatal(err)
	}
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Report User", "email": "report@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{
		"login":      "report",
		"name":       "Report User",
		"email":      "report@example.com",
		"active":     true,
		"partner_id": partnerID,
		"groups_id":  []int64{groupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := internalmail.MassMailingReportToken(server.systemRequestEnv(), userID)
	noGroupPartnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "No Group Report User", "email": "nogroup-report@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	noGroupUserID, err := server.Env.Model("res.users").Create(map[string]any{
		"login":      "nogroup-report",
		"name":       "No Group Report User",
		"email":      "nogroup-report@example.com",
		"active":     true,
		"partner_id": noGroupPartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	noGroupToken := internalmail.MassMailingReportToken(server.systemRequestEnv(), noGroupUserID)

	for _, path := range []string{
		fmt.Sprintf("/mailing/report/unsubscribe?user_id=%d", userID),
		fmt.Sprintf("/mailing/report/unsubscribe?user_id=%d&token=bad", userID),
		fmt.Sprintf("/mailing/report/unsubscribe?user_id=%d&token=%s", noGroupUserID, noGroupToken),
	} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
			t.Fatalf("invalid report unsubscribe response %d body=%s", rec.Code, rec.Body.String())
		}
		rows, err := server.Env.Model("ir.config_parameter").Browse(configID).Read("value")
		if err != nil {
			t.Fatal(err)
		}
		if rows[0]["value"] != "True" {
			t.Fatalf("invalid call mutated config: %+v", rows)
		}
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mailing/report/unsubscribe?user_id=%d&token=%s", userID, token), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Mailing Reports Turned Off") {
		t.Fatalf("valid report unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("ir.config_parameter").Browse(configID).Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["value"] != "False" {
		t.Fatalf("report config = %+v", rows)
	}
	if err := server.Env.Model("ir.config_parameter").Browse(configID).Write(map[string]any{"value": "True"}); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"user_id": {strconv.FormatInt(userID, 10)}, "token": {token}}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mailing/report/unsubscribe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Mailing Reports Turned Off") {
		t.Fatalf("post report unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("ir.config_parameter").Browse(configID).Read("value")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["value"] != "False" {
		t.Fatalf("post report config = %+v", rows)
	}
}

func TestMassMailingViewInBrowserValidatesTokenAndRendersBody(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-view-secret"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":      "View Mailing",
		"body_html": `<section><h1>Hello</h1><a href="/unsubscribe_from_list">Unsubscribe</a></section>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, 42, "viewer@example.com")
	reqURL := fmt.Sprintf("/mailing/%d/view?document_id=42&email=%s&hash_token=%s", mailingID, url.QueryEscape("viewer@example.com"), token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, reqURL, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h1>Hello</h1>") || !strings.Contains(rec.Body.String(), fmt.Sprintf("/mailing/%d/confirm_unsubscribe", mailingID)) || strings.Contains(rec.Body.String(), "/unsubscribe_from_list") {
		t.Fatalf("view response %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mailing/%d/view?document_id=42&email=viewer@example.com&hash_token=bad", mailingID), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token response %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMassMailingUnsubscribeAddsBlacklist(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-unsub-secret"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "Unsub Mailing"})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, 51, "leave@example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mailing/%d/unsubscribe?document_id=51&email=leave@example.com&hash_token=%s", mailingID, token), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no longer") {
		t.Fatalf("unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	found, err := server.Env.Model("mail.blacklist").Search(domain.Cond("email", domain.Equal, "leave@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("email", "active", "message")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["email"] != "leave@example.com" || rows[0]["active"] != true || !strings.Contains(stringValue(rows[0]["message"]), "mailing") {
		t.Fatalf("blacklist rows = %+v", rows)
	}
}

func TestMassMailingListUnsubscribeOptsOutSubscriptionsWithoutBlacklist(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-list-unsub-secret"}); err != nil {
		t.Fatal(err)
	}
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Public List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherListID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Other List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "List Contact", "email": "List.User@Example.COM", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	otherSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": otherListID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "List Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, contactID, "list.user@example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=list.user@example.com&hash_token=%s", mailingID, contactID, token), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(subID, otherSubID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("target subscription not opted out: %+v", rows[0])
	}
	if rows[1]["opt_out"] == true || !accountingDateValue(rows[1]["opt_out_datetime"]).IsZero() {
		t.Fatalf("other subscription mutated: %+v", rows[1])
	}
	blacklist, err := server.Env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("list unsubscribe created blacklist rows: %d", blacklist.Len())
	}
}

func TestMassMailingListUnsubscribeHandlesFormattedEmailAndIsIdempotent(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-list-formatted-secret"}); err != nil {
		t.Fatal(err)
	}
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Formatted List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "Formatted Contact", "email": "Formatted User <Formatted.User@Example.COM>", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Formatted Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	email := "Formatted User <Formatted.User@Example.COM>"
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, contactID, email)
	reqURL := fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=%s&hash_token=%s", mailingID, contactID, url.QueryEscape(email), token)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, reqURL, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("formatted unsubscribe response %d body=%s", rec.Code, rec.Body.String())
		}
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(subID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("formatted subscription rows = %+v", rows)
	}
	found, err := server.Env.Model("mailing.subscription").Search(domain.Cond("contact_id", domain.Equal, contactID))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("subscription count = %d", found.Len())
	}
	blacklist, err := server.Env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("formatted list unsubscribe created blacklist rows: %d", blacklist.Len())
	}
}

func TestSMSOptOutRouteAddsPhoneBlacklist(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "SMS Partner", "phone": "+91 123 465 7890", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Mailing", "mailing_model_real": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": partnerID, "sms_code": "SmsA1", "sms_number": "+911234657890", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/unsubscribe/SmsA1?sms_number=%s", mailingID, url.QueryEscape("+911234657890")), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "+911234657890") {
		t.Fatalf("sms unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.Cond("number", domain.Equal, "+911234657890"))
	if err != nil {
		t.Fatal(err)
	}
	blacklistRows, err := blacklist.Read("number", "active", "message")
	if err != nil {
		t.Fatal(err)
	}
	if len(blacklistRows) != 1 || blacklistRows[0]["number"] != "+911234657890" || blacklistRows[0]["active"] != true || !strings.Contains(stringValue(blacklistRows[0]["message"]), "SMS Marketing") {
		t.Fatalf("phone blacklist rows = %+v", blacklistRows)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(partnerID).Read("phone_sanitized", "phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["phone_sanitized"] != "+911234657890" || partnerRows[0]["phone_blacklisted"] != true {
		t.Fatalf("partner rows = %+v", partnerRows)
	}
}

func TestSMSOptOutRouteReactivatesExistingPhoneBlacklist(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Existing SMS Partner", "phone": "+1 555 000 2020", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	blacklistID, err := server.Env.Model("phone.blacklist").Create(map[string]any{"number": "+15550002020", "active": false, "message": "old"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Existing", "mailing_model_real": "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": partnerID, "sms_code": "React1", "sms_number": "+15550002020", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/sms/%d/unsubscribe/React1", mailingID), strings.NewReader("sms_number=%2B15550002020"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "+15550002020") {
		t.Fatalf("sms unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	blacklistRows, err := blacklist.Read("id", "number", "active", "message")
	if err != nil {
		t.Fatal(err)
	}
	if len(blacklistRows) != 1 || int64Value(blacklistRows[0]["id"]) != blacklistID || blacklistRows[0]["number"] != "+15550002020" || blacklistRows[0]["active"] != true || !strings.Contains(stringValue(blacklistRows[0]["message"]), "mailing ID") {
		t.Fatalf("phone blacklist rows = %+v", blacklistRows)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(partnerID).Read("phone_blacklisted")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["phone_blacklisted"] != true {
		t.Fatalf("partner rows = %+v", partnerRows)
	}
}

func TestSMSOptOutRouteUpdatesListSubscriptions(t *testing.T) {
	server := testMailThreadServer(t)
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	otherListID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Other SMS List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "SMS Contact", "phone": "+32 456 000 000", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	otherSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": otherListID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS List Mailing", "contact_list_ids": []int64{listID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "mailing.contact", "res_id": contactID, "sms_code": "Lst1", "sms_number": "+32456000000", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/unsubscribe/Lst1?sms_number=%s", mailingID, url.QueryEscape("+32456000000")), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sms list unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(subID, otherSubID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("target subscription not opted out: %+v", rows[0])
	}
	if rows[1]["opt_out"] == true || !accountingDateValue(rows[1]["opt_out_datetime"]).IsZero() {
		t.Fatalf("other subscription mutated: %+v", rows[1])
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("list unsubscribe created phone blacklist rows: %d", blacklist.Len())
	}
}

func TestSMSOptOutRouteDisplaysActualPublicAndPrivateOptOutLists(t *testing.T) {
	server := testMailThreadServer(t)
	publicID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS Public Actual", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	privateID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS Private Secret", "is_public": false, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	unmatchedPublicID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS Unmatched Public", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	outsideID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS Outside Public", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "SMS Display Contact", "phone": "+32 456 111 111", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	publicSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": publicID})
	if err != nil {
		t.Fatal(err)
	}
	privateSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": privateID})
	if err != nil {
		t.Fatal(err)
	}
	outsideSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": outsideID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Display Mailing", "contact_list_ids": []int64{publicID, privateID, unmatchedPublicID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "mailing.contact", "res_id": contactID, "sms_code": "LstDisplay1", "sms_number": "+32456111111", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/unsubscribe/LstDisplay1?sms_number=%s", mailingID, url.QueryEscape("+32456111111")), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sms display unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"successfully removed from", "SMS Public Actual", "Mailing List"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %q: %s", expected, body)
		}
	}
	for _, unexpected := range []string{"SMS Private Secret", "SMS Unmatched Public", "SMS Outside Public", "successfully blacklisted"} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("response leaked %q: %s", unexpected, body)
		}
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(publicSubID, privateSubID, outsideSubID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("public subscription not opted out: %+v", rows[0])
	}
	if rows[1]["opt_out"] != true || accountingDateValue(rows[1]["opt_out_datetime"]).IsZero() {
		t.Fatalf("private subscription not opted out: %+v", rows[1])
	}
	if rows[2]["opt_out"] == true || !accountingDateValue(rows[2]["opt_out_datetime"]).IsZero() {
		t.Fatalf("outside subscription mutated: %+v", rows[2])
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("list unsubscribe created phone blacklist rows: %d", blacklist.Len())
	}
}

func TestSMSOptOutRouteListModeWithoutMatchingSubscriptionShowsFallback(t *testing.T) {
	server := testMailThreadServer(t)
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "SMS Empty List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "SMS Other Contact", "phone": "+32 456 222 222", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Empty Mailing", "contact_list_ids": []int64{listID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "mailing.contact", "res_id": contactID, "sms_code": "LstEmpty1", "sms_number": "+32456333333", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/unsubscribe/LstEmpty1?sms_number=%s", mailingID, url.QueryEscape("+32456333333")), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "successfully blacklisted") {
		t.Fatalf("sms empty list unsubscribe response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(subID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] == true || !accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("unmatched subscription mutated: %+v", rows[0])
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("list unsubscribe fallback created phone blacklist rows: %d", blacklist.Len())
	}
}

func TestSMSOptOutEntryRedirectsWithSanitizedNumber(t *testing.T) {
	server := testMailThreadServer(t)
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Entry"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": int64(44), "sms_code": "Ent1", "sms_number": "+911234657890", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/Ent1?sms_number=%s", mailingID, url.QueryEscape("+91 123 465 7890")), nil))
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), fmt.Sprintf("/sms/%d/unsubscribe/Ent1?", mailingID)) || !strings.Contains(rec.Header().Get("Location"), "%2B911234657890") {
		t.Fatalf("entry redirect code=%d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func TestSMSOptOutEntryUsesGeoCountryE164(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("res.country").Create(map[string]any{"name": "Belgium", "code": "BE", "phone_code": int64(32), "active": true}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Geo Entry"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": int64(44), "sms_code": "Geo1", "sms_number": "+32456040506", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/Geo1?sms_number=%s", mailingID, url.QueryEscape("0456 04 05 06")), nil)
	req.Header.Set("X-GeoIP-Country-Code", "BE")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != fmt.Sprintf("/sms/%d/unsubscribe/Geo1?sms_number=%%2B32456040506", mailingID) {
		t.Fatalf("entry redirect code=%d location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("entry route created phone blacklist rows: %d", blacklist.Len())
	}
}

func TestSMSOptOutPathAndMethodEdges(t *testing.T) {
	for _, tc := range []struct {
		path        string
		mailingID   int64
		traceCode   string
		unsubscribe bool
		ok          bool
	}{
		{path: "/sms/12/ABC", mailingID: 12, traceCode: "ABC", ok: true},
		{path: "/sms/12/unsubscribe/ABC", mailingID: 12, traceCode: "ABC", unsubscribe: true, ok: true},
		{path: "/sms/0/ABC"},
		{path: "/sms/12/unsubscribe/ABC/extra"},
		{path: "/sms/12/"},
		{path: "/mailing/12/ABC"},
	} {
		mailingID, traceCode, unsubscribe, ok := parseSMSOptOutPath(tc.path)
		if mailingID != tc.mailingID || traceCode != tc.traceCode || unsubscribe != tc.unsubscribe || ok != tc.ok {
			t.Fatalf("parseSMSOptOutPath(%q) = id=%d code=%q unsubscribe=%v ok=%v", tc.path, mailingID, traceCode, unsubscribe, ok)
		}
	}

	server := testMailThreadServer(t)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/sms/1/ABC", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("delete sms opt-out response code=%d allow=%q body=%s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
}

func TestSMSOptOutInvalidTraceRedirectsAndMismatchDoesNotMutate(t *testing.T) {
	server := testMailThreadServer(t)
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "SMS Invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mailing.trace").Create(map[string]any{"trace_type": "sms", "mass_mailing_id": mailingID, "model": "res.partner", "res_id": int64(55), "sms_code": "Ok1", "sms_number": "+15550101", "trace_status": "sent"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/bad?sms_number=%s", mailingID, url.QueryEscape("+15550101")), nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/odoo" {
		t.Fatalf("invalid trace response code=%d location=%s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sms/%d/unsubscribe/Ok1?sms_number=%s", mailingID, url.QueryEscape("+15550102")), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("mismatch response code=%d body=%s", rec.Code, rec.Body.String())
	}
	blacklist, err := server.Env.Model("phone.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("mismatch created phone blacklist rows: %d", blacklist.Len())
	}
}

func TestMassMailingListConfirmAndExactPostUsePublicLists(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-list-confirm-secret"}); err != nil {
		t.Fatal(err)
	}
	publicID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Public Alpha", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	privateID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Private Ops", "is_public": false, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "Confirm Contact", "email": "confirm@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	publicSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": publicID})
	if err != nil {
		t.Fatal(err)
	}
	privateSubID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": privateID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Confirm List Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{publicID, privateID},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, contactID, "confirm@example.com")
	query := fmt.Sprintf("document_id=%d&email=confirm@example.com&hash_token=%s", contactID, token)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/mailing/%d/confirm_unsubscribe?%s", mailingID, query), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Public Alpha") || strings.Contains(rec.Body.String(), "Private Ops") || !strings.Contains(rec.Body.String(), `name="mailing_id"`) {
		t.Fatalf("confirm response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(publicSubID, privateSubID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] == true || rows[1]["opt_out"] == true {
		t.Fatalf("confirm get mutated subscriptions: %+v", rows)
	}

	form := url.Values{}
	form.Set("mailing_id", strconv.FormatInt(mailingID, 10))
	form.Set("document_id", strconv.FormatInt(contactID, 10))
	form.Set("email", "confirm@example.com")
	form.Set("hash_token", token)
	req := httptest.NewRequest(http.MethodPost, "/mailing/confirm_unsubscribe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm post response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("mailing.subscription").Browse(publicSubID, privateSubID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() || rows[1]["opt_out"] != true || accountingDateValue(rows[1]["opt_out_datetime"]).IsZero() {
		t.Fatalf("confirm post subscription rows = %+v", rows)
	}
	blacklist, err := server.Env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("confirm list unsubscribe created blacklist rows: %d", blacklist.Len())
	}
}

func TestMassMailingListUnsubscribeRejectsInvalidTokenWithoutSideEffects(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-list-reject-secret"}); err != nil {
		t.Fatal(err)
	}
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "Reject List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "Reject Contact", "email": "reject-list@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "Reject List Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, contactID, "reject-list@example.com")
	cases := []struct {
		name string
		path string
		code int
	}{
		{"missing token", fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=reject-list@example.com", mailingID, contactID), http.StatusBadRequest},
		{"bad token", fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=reject-list@example.com&hash_token=bad", mailingID, contactID), http.StatusUnauthorized},
		{"wrong email", fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=other@example.com&hash_token=%s", mailingID, contactID, token), http.StatusUnauthorized},
		{"wrong document", fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=reject-list@example.com&hash_token=%s", mailingID, contactID+100, token), http.StatusUnauthorized},
		{"wrong mailing", fmt.Sprintf("/mailing/%d/unsubscribe?document_id=%d&email=reject-list@example.com&hash_token=%s", mailingID+100, contactID, token), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.code {
				t.Fatalf("response %d body=%s", rec.Code, rec.Body.String())
			}
			rows, err := server.Env.Model("mailing.subscription").Browse(subID).Read("opt_out", "opt_out_datetime")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["opt_out"] == true || !accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
				t.Fatalf("subscription mutated: %+v", rows)
			}
			blacklist, err := server.Env.Model("mail.blacklist").Search(domain.And())
			if err != nil {
				t.Fatal(err)
			}
			if blacklist.Len() != 0 {
				t.Fatalf("invalid unsubscribe created blacklist rows: %d", blacklist.Len())
			}
		})
	}
}

func TestMassMailingListOneClickUsesListModeNoBlacklist(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-list-oneclick-secret"}); err != nil {
		t.Fatal(err)
	}
	listID, err := server.Env.Model("mailing.list").Create(map[string]any{"name": "One Click List", "is_public": true, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	contactID, err := server.Env.Model("mailing.contact").Create(map[string]any{"name": "One Click Contact", "email": "one-list@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	subID, err := server.Env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{
		"name":                    "One Click List Mailing",
		"mailing_on_mailing_list": true,
		"contact_list_ids":        []int64{listID},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, contactID, "one-list@example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/mailing/%d/unsubscribe_oneclick?document_id=%d&email=one-list@example.com&hash_token=%s", mailingID, contactID, token), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list oneclick response %d body=%s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mailing.subscription").Browse(subID).Read("opt_out", "opt_out_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["opt_out"] != true || accountingDateValue(rows[0]["opt_out_datetime"]).IsZero() {
		t.Fatalf("list oneclick subscription rows = %+v", rows)
	}
	blacklist, err := server.Env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if blacklist.Len() != 0 {
		t.Fatalf("list oneclick created blacklist rows: %d", blacklist.Len())
	}
}

func TestMassMailingUnsubscribeOneClickAndPlaceholderRoutes(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "mass-oneclick-secret"}); err != nil {
		t.Fatal(err)
	}
	mailingID, err := server.Env.Model("mailing.mailing").Create(map[string]any{"name": "One Click"})
	if err != nil {
		t.Fatal(err)
	}
	token := mailingRecipientToken(server.systemRequestEnv(), mailingID, 52, "one@example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/mailing/%d/unsubscribe_oneclick?document_id=52&email=one@example.com&hash_token=%s", mailingID, token), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("oneclick response %d body=%s", rec.Code, rec.Body.String())
	}
	found, err := server.Env.Model("mail.blacklist").Search(domain.Cond("email", domain.Equal, "one@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("oneclick blacklist count = %d", found.Len())
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unsubscribe_from_list", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/mailing/my" {
		t.Fatalf("placeholder response %d location=%s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/view", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "browser") {
		t.Fatalf("view placeholder response %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMailThreadMessageProcessAliasDanglingForceThreadFallsBackToCreate(t *testing.T) {
	server := testMailThreadServer(t)
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "http-dangling",
		"alias_domain_id":       aliasDomainID,
		"alias_model_id":        modelID,
		"alias_force_thread_id": int64(999999),
		"alias_defaults":        "{'name': 'HTTP Dangling Fallback'}",
		"alias_contact":         "everyone",
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-dangling-forced@remote>",
			"From: HTTP Dangling <http.dangling@example.com>",
			"To: http-dangling@example.com",
			"Subject: HTTP Dangling Forced",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP dangling body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 || resID == 999999 {
		t.Fatalf("message_process response = %s", body)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(resID).Read("name", "email", "email_normalized")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["name"] != "HTTP Dangling Fallback" || partnerRows[0]["email"] != "http.dangling@example.com" || partnerRows[0]["email_normalized"] != "http.dangling@example.com" {
		t.Fatalf("fallback partner rows = %+v", partnerRows)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-dangling-forced@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := found.Read("model", "res_id", "parent_id", "incoming_email_to")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["model"] != "res.partner" ||
		messageRows[0]["res_id"] != resID ||
		messageRows[0]["parent_id"] != int64(0) ||
		messageRows[0]["incoming_email_to"] != "" {
		t.Fatalf("fallback message rows = %+v", messageRows)
	}
	if bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-alias-dangling-forced@remote>"); len(bounceRows) != 0 {
		t.Fatalf("dangling forced alias bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessAliasIncomingLocalHonorsAllowedDomains(t *testing.T) {
	for _, tc := range []struct {
		name             string
		allowedDomains   string
		extraAliasDomain string
		to               string
		wantResponse     string
	}{
		{name: "restricted denied", allowedDomains: "allowed.example", to: "http-local@outside.example", wantResponse: "false"},
		{name: "restricted allowed", allowedDomains: "allowed.example", to: "http-local@allowed.example"},
		{name: "alias domain always allowed", allowedDomains: "allowed.example", extraAliasDomain: "secondary.example", to: "http-local@secondary.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testMailThreadServer(t)
			aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.extraAliasDomain != "" {
				if _, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": tc.extraAliasDomain}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.allowedDomains != "" {
				if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": tc.allowedDomains}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := server.Env.Model("mail.alias").Create(map[string]any{
				"alias_name":           "http-local",
				"alias_domain_id":      aliasDomainID,
				"model_name":           "res.partner",
				"alias_incoming_local": true,
				"active":               true,
			}); err != nil {
				t.Fatal(err)
			}
			messageID := "<http-alias-local-allowed-" + strings.ReplaceAll(tc.name, " ", "-") + "@remote>"
			raw := strings.Join([]string{
				"Message-Id: " + messageID,
				"From: HTTP Local <http.local@example.com>",
				"To: " + tc.to,
				"Subject: HTTP Local Allowed",
				"Content-Type: text/plain; charset=utf-8",
				"",
				"HTTP local allowed body",
				"",
			}, "\r\n")
			payloadValues := map[string]any{
				"model":  "mail.thread",
				"method": "message_process",
				"args":   []any{"res.partner", raw},
			}
			if tc.wantResponse == "false" {
				payloadValues = map[string]any{
					"model":  "mail.thread",
					"method": "message_process",
					"kwargs": map[string]any{"message": raw},
				}
			}
			payload, err := json.Marshal(payloadValues)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
			if tc.wantResponse == "false" {
				if body != "false" {
					t.Fatalf("message_process response = %s", body)
				}
				found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", messageID))
				if err != nil {
					t.Fatal(err)
				}
				if found.Len() != 0 {
					t.Fatalf("denied local alias HTTP message count = %d", found.Len())
				}
				return
			}
			resID, err := strconv.ParseInt(body, 10, 64)
			if err != nil || resID == 0 {
				t.Fatalf("message_process response = %s", body)
			}
			found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", messageID))
			if err != nil {
				t.Fatal(err)
			}
			rows, err := found.Read("model", "res_id", "incoming_email_to")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != resID || strings.TrimSpace(stringValue(rows[0]["incoming_email_to"])) != "" {
				t.Fatalf("HTTP local alias rows = %+v response=%s", rows, body)
			}
		})
	}
}

func TestMailThreadMessageProcessRejectsAliasPartnersUnknownAuthor(t *testing.T) {
	server := testMailThreadServer(t)
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "http-known",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "partners",
		"alias_status":    "not_tested",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-partners-reject@remote>",
			"From: Unknown <unknown.http@example.com>",
			"To: http-known@example.com",
			"Subject: HTTP Alias Partners Reject",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP reject body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-partners-reject@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("rejected HTTP alias message count = %d", found.Len())
	}
	aliasRows, err := server.Env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "not_tested" {
		t.Fatalf("HTTP partners alias rows = %+v", aliasRows)
	}
	bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-alias-partners-reject@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["email_to"] != "Unknown <unknown.http@example.com>" ||
		bounceRows[0]["subject"] != "Re: HTTP Alias Partners Reject" ||
		!strings.Contains(stringValue(bounceRows[0]["body_html"]), "registered partners") {
		t.Fatalf("HTTP partners bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessAliasFollowersForcedThread(t *testing.T) {
	server := testMailThreadServer(t)
	targetID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Follower Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Follower", "email": "http.follower@example.com", "email_normalized": "http.follower@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	strangerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Stranger", "email": "http.stranger@example.com", "email_normalized": "http.stranger@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := internalmail.Subscribe(server.Env, "res.partner", targetID, []int64{authorID}, nil); err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "http-followers",
		"alias_domain_id":       aliasDomainID,
		"model_name":            "res.partner",
		"alias_force_thread_id": targetID,
		"alias_contact":         "followers",
		"active":                true,
	}); err != nil {
		t.Fatal(err)
	}

	rejectedPayload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-followers-reject@remote>",
			"From: Stranger <http.stranger@example.com>",
			"To: http-followers@example.com",
			"Subject: HTTP Alias Followers Reject",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP followers reject body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(rejectedPayload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process reject response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-followers-reject@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("rejected follower HTTP alias message count = %d", found.Len())
	}

	acceptedPayload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-followers-accept@remote>",
			"From: Follower <http.follower@example.com>",
			"To: http-followers@example.com",
			"Subject: HTTP Alias Followers Accept",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP followers accept body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = postCallKW(t, server.Handler(), string(acceptedPayload))
	if strings.TrimSpace(body) != fmt.Sprint(targetID) {
		t.Fatalf("message_process accept response = %s", body)
	}
	found, err = server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-followers-accept@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("model", "res_id", "author_id", "body")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["model"] != "res.partner" || rows[0]["res_id"] != targetID || rows[0]["author_id"] != authorID || !strings.Contains(rows[0]["body"].(string), "HTTP followers accept body") {
		t.Fatalf("accepted follower HTTP alias rows = %+v author=%d stranger=%d", rows, authorID, strangerID)
	}
}

func TestMailThreadMessageProcessAliasDanglingForceThreadUsesParentForFollowers(t *testing.T) {
	server := testMailThreadServer(t)
	parentID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Dangling Parent", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Dangling Parent Follower", "email": "http.dangling.parent@example.com", "email_normalized": "http.dangling.parent@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := internalmail.Subscribe(server.Env, "res.partner", parentID, []int64{authorID}, nil); err != nil {
		t.Fatal(err)
	}
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":             "http-dangling-parent-followers",
		"alias_domain_id":        aliasDomainID,
		"alias_model_id":         modelID,
		"alias_force_thread_id":  int64(999999),
		"alias_contact":          "followers",
		"alias_parent_model_id":  modelID,
		"alias_parent_thread_id": parentID,
		"alias_status":           "not_tested",
		"active":                 true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-dangling-parent-followers@remote>",
			"From: Follower <http.dangling.parent@example.com>",
			"To: http-dangling-parent-followers@example.com",
			"Subject: HTTP Dangling Parent Followers",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP dangling parent followers body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(postCallKW(t, server.Handler(), string(payload)))
	resID, err := strconv.ParseInt(body, 10, 64)
	if err != nil || resID == 0 || resID == 999999 || resID == parentID {
		t.Fatalf("message_process response = %s parent=%d", body, parentID)
	}
	partnerRows, err := server.Env.Model("res.partner").Browse(resID).Read("name", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(partnerRows) != 1 || partnerRows[0]["name"] != "HTTP Dangling Parent Followers" || partnerRows[0]["email"] != "http.dangling.parent@example.com" {
		t.Fatalf("created partner rows = %+v", partnerRows)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-dangling-parent-followers@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := found.Read("model", "res_id", "parent_id", "author_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["model"] != "res.partner" ||
		messageRows[0]["res_id"] != resID ||
		messageRows[0]["parent_id"] != int64(0) ||
		messageRows[0]["author_id"] != authorID {
		t.Fatalf("message rows = %+v author=%d", messageRows, authorID)
	}
	aliasRows, err := server.Env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "valid" {
		t.Fatalf("alias rows = %+v", aliasRows)
	}
	if bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-alias-dangling-parent-followers@remote>"); len(bounceRows) != 0 {
		t.Fatalf("dangling parent followers bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessAliasConfigErrorCreatesBounce(t *testing.T) {
	server := testMailThreadServer(t)
	authorID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "HTTP Known", "email": "http.known@example.com", "email_normalized": "http.known@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	aliasDomainID, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "example.com", "bounce_email": "bounce@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":      "http-broken-followers",
		"alias_domain_id": aliasDomainID,
		"model_name":      "res.partner",
		"alias_contact":   "followers",
		"alias_status":    "not_tested",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-alias-config-reject@remote>",
			"From: Known <http.known@example.com>",
			"To: http-broken-followers@example.com",
			"Subject: HTTP Alias Config Reject",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP config reject body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process config response = %s", body)
	}
	aliasRows, err := server.Env.Model("mail.alias").Browse(aliasID).Read("alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliasRows) != 1 || aliasRows[0]["alias_status"] != "invalid" {
		t.Fatalf("HTTP config alias rows = %+v", aliasRows)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-alias-config-reject@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("config rejection message count = %d author=%d", found.Len(), authorID)
	}
	bounceRows := httpAliasBounceMailRows(t, server.Env, "<http-alias-config-reject@remote>")
	if len(bounceRows) != 1 || !strings.Contains(stringValue(bounceRows[0]["body_html"]), "Please try again later") {
		t.Fatalf("HTTP config bounce rows = %+v", bounceRows)
	}
}

func TestMailThreadMessageProcessBouncesDirectCatchall(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("mail.alias.domain").Create(map[string]any{
		"name":           "example.com",
		"bounce_email":   "bounce@example.com",
		"catchall_alias": "catchall",
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-catchall-direct@remote>",
			"From: Customer <customer@example.com>",
			"Return-Path: <return@example.com>",
			`To: "My Super Catchall" <catchall@example.com>`,
			"Subject: HTTP Should Bounce",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"HTTP catchall body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process catchall response = %s", body)
	}
	foundMessages, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-catchall-direct@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if foundMessages.Len() != 0 {
		t.Fatalf("HTTP catchall message count = %d", foundMessages.Len())
	}
	bounceRows := httpCatchallBounceMailRows(t, server.Env, "<http-catchall-direct@remote>")
	if len(bounceRows) != 1 ||
		bounceRows[0]["email_from"] != `"MAILER-DAEMON" <bounce@example.com>` ||
		bounceRows[0]["email_to"] != "return@example.com" ||
		bounceRows[0]["subject"] != "Re: HTTP Should Bounce" ||
		!strings.Contains(stringValue(bounceRows[0]["references"]), "loop-detection-bounce-email") ||
		!strings.Contains(stringValue(bounceRows[0]["body_html"]), "cannot be processed") {
		t.Fatalf("HTTP catchall bounce rows = %+v", bounceRows)
	}
}

func httpAliasBounceMailRows(t *testing.T, env *record.Env, rfcMessageID string) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.Cond("references", "=", rfcMessageID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("email_from", "email_to", "subject", "body_html", "state", "auto_delete", "references", "is_notification")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func httpCatchallBounceMailRows(t *testing.T, env *record.Env, rfcMessageID string) []map[string]any {
	t.Helper()
	found, err := env.Model("mail.mail").Search(domain.Cond("references", domain.Like, rfcMessageID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("email_from", "email_to", "subject", "body_html", "state", "auto_delete", "references", "is_notification", "message_id", "record_alias_domain_id", "reply_to")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestMailThreadMessageProcessIgnoresLoopDetectionBounceReply(t *testing.T) {
	server := testMailThreadServer(t)
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-loop-reply@remote>",
			"From: Auto Reply <auto@example.com>",
			"To: catch@example.com",
			"References: <original@remote> <20260619-loop-detection-bounce-email@example.com>",
			"Subject: Loop reply",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Loop reply body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-loop-reply@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("loop reply should not create mail.message, count = %d", found.Len())
	}
}

func TestMailThreadMessageProcessDetectsSenderLoop(t *testing.T) {
	server := testMailThreadServer(t)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.gateway.loop.threshold", "value": "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.partner").Create(map[string]any{
		"name":             "Existing Loop",
		"email":            "http.loop@example.com",
		"email_normalized": "http.loop@example.com",
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{"res.partner", strings.Join([]string{
			"Message-Id: <http-sender-loop@remote>",
			"From: HTTP Loop <http.loop@example.com>",
			"To: catch@example.com",
			"Subject: Sender loop",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Loop body",
			"",
		}, "\r\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	if strings.TrimSpace(body) != "false" {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-sender-loop@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("looping inbound message count = %d", found.Len())
	}
}

func TestMailThreadMessageProcessPreservesOriginalAndStripsAttachments(t *testing.T) {
	server := testMailThreadServer(t)
	payload, err := json.Marshal(map[string]any{
		"model":  "mail.thread",
		"method": "message_process",
		"args": []any{
			"res.partner",
			strings.Join([]string{
				"Message-Id: <http-original@remote>",
				"From: HTTP Sender <http.sender@example.com>",
				"To: catch@example.com",
				"Subject: HTTP original",
				"MIME-Version: 1.0",
				`Content-Type: multipart/mixed; boundary="http-original-boundary"`,
				"",
				"--http-original-boundary",
				"Content-Type: text/plain; charset=utf-8",
				"",
				"Body",
				"--http-original-boundary",
				`Content-Type: text/plain; name="note.txt"`,
				`Content-Disposition: attachment; filename="note.txt"`,
				"",
				"Attached",
				"--http-original-boundary--",
				"",
			}, "\r\n"),
		},
		"kwargs": map[string]any{
			"save_original":     true,
			"strip_attachments": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, server.Handler(), string(payload))
	recordID, err := strconv.ParseInt(strings.TrimSpace(body), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if recordID == 0 {
		t.Fatalf("message_process response = %s", body)
	}
	found, err := server.Env.Model("mail.message").Search(domain.Cond("message_id", "=", "<http-original@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]["attachment_ids"].([]int64)) != 0 {
		t.Fatalf("stripped message rows = %+v", rows)
	}
}

func mailThreadBounceMessage(finalRecipient string, originalMessageID string) string {
	return strings.Join([]string{
		"From: Mailer-Daemon <mailer-daemon@example.net>",
		"To: bounce@example.com",
		"Subject: Delivery Status Notification",
		"MIME-Version: 1.0",
		`Content-Type: multipart/report; report-type=delivery-status; boundary="http-bounce-boundary"`,
		"",
		"--http-bounce-boundary",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Delivery failed",
		"",
		"--http-bounce-boundary",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; mx.example.net",
		"Final-Recipient: rfc822; " + finalRecipient,
		"Action: failed",
		"Status: 5.1.1",
		"Diagnostic-Code: smtp; 550 No such user",
		"",
		"--http-bounce-boundary",
		"Content-Type: message/rfc822",
		"",
		"Message-Id: " + originalMessageID,
		"From: sender@example.com",
		"To: " + finalRecipient,
		"Subject: Original",
		"",
		"Original body",
		"--http-bounce-boundary--",
		"",
	}, "\r\n")
}

func TestCallKWActivityScheduleFeedbackAndUnlink(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Thread", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "activity", "name": "Activity User", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityTypeID, err := server.Env.Model("mail.activity.type").Create(map[string]any{
		"name":            "To-Do",
		"summary":         "Follow up",
		"default_note":    "Default note",
		"default_user_id": userID,
		"category":        "phonecall",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "mail",
		"name":          "mail_activity_data_todo",
		"complete_name": "mail.mail_activity_data_todo",
		"model":         "mail.activity.type",
		"res_id":        activityTypeID,
	}); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "done.txt",
		"res_model": "res.partner",
		"res_id":    partnerID,
		"type":      "binary",
		"datas":     "ZG9uZQ==",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"res.partner","method":"activity_schedule","args":[[%d]],"kwargs":{"act_type_xmlid":"mail.mail_activity_data_todo","date_deadline":"2026-07-02"}}`, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/activity_schedule", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("activity_schedule response %d %s", rec.Code, rec.Body.String())
	}
	var activityIDs []int64
	if err := json.Unmarshal(rec.Body.Bytes(), &activityIDs); err != nil {
		t.Fatal(err)
	}
	if len(activityIDs) != 1 {
		t.Fatalf("activity ids = %+v", activityIDs)
	}
	activityRows, err := server.Env.Model("mail.activity").Browse(activityIDs...).Read("activity_type_id", "res_model", "res_id", "user_id", "date_deadline", "summary", "note", "automated", "activity_category", "chaining_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(activityRows) != 1 || activityRows[0]["activity_type_id"] != activityTypeID || activityRows[0]["res_id"] != partnerID || activityRows[0]["user_id"] != userID || activityRows[0]["date_deadline"] != "2026-07-02" || activityRows[0]["summary"] != "Follow up" || activityRows[0]["note"] != "Default note" || activityRows[0]["automated"] != true || activityRows[0]["activity_category"] != "phonecall" || activityRows[0]["chaining_type"] != "suggest" {
		t.Fatalf("activity rows = %+v", activityRows)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"res.partner","method":"activity_feedback","args":[[%d],["mail.mail_activity_data_todo"],null,"Done"],"kwargs":{"attachment_ids":[%d]}}`, partnerID, attachmentID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/activity_feedback", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("activity_feedback response %d %s", rec.Code, rec.Body.String())
	}
	var ok bool
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("activity_feedback returned false")
	}
	activityRows, err = server.Env.Model("mail.activity").Browse(activityIDs...).Read("state", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if activityRows[0]["state"] != "done" {
		t.Fatalf("activity rows after feedback = %+v", activityRows)
	}
	if got := activityRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("activity attachment ids = %#v", activityRows[0]["attachment_ids"])
	}
	messages, err := server.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", "=", "res.partner"),
		domain.Cond("res_id", "=", partnerID),
	))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("body", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	bodyHTML := stringValue(messageRows[0]["body"])
	for _, want := range []string{`<span class="fa fa-fw"></span><span>To-Do</span> done: Follow up`, `<div class="o_mail_note_title fw-bold">Original note:</div><div>Default note</div>`, `<div class="fw-bold">Feedback:</div>Done`} {
		if !strings.Contains(bodyHTML, want) {
			t.Fatalf("message body missing %q: %s", want, bodyHTML)
		}
	}
	if len(messageRows) != 1 {
		t.Fatalf("message rows = %+v", messageRows)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != attachmentID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}

	manualID, err := server.Env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": activityTypeID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          userID,
		"date_deadline":    "2026-07-03",
		"summary":          "Manual",
		"state":            "open",
		"automated":        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"res.partner","method":"activity_unlink","args":[[%d],["mail.mail_activity_data_todo"]]}`, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/activity_unlink", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("activity_unlink response %d %s", rec.Code, rec.Body.String())
	}
	if rows, err := server.Env.Model("mail.activity").Browse(manualID).Read("id"); err != nil || len(rows) != 1 {
		t.Fatalf("manual rows after automated unlink = %+v err=%v", rows, err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"res.partner","method":"activity_unlink","args":[[%d],["mail.mail_activity_data_todo"]],"kwargs":{"only_automated":false}}`, partnerID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/activity_unlink", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("activity_unlink manual response %d %s", rec.Code, rec.Body.String())
	}
	if rows, err := server.Env.Model("mail.activity").Browse(manualID).Read("id"); err != nil || len(rows) != 0 {
		t.Fatalf("manual rows after manual unlink = %+v err=%v", rows, err)
	}
}

func TestCallKWMailActivityActionFeedbackScheduleNext(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "activity", "name": "Activity User", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityTypeID, err := server.Env.Model("mail.activity.type").Create(map[string]any{
		"name":            "To-Do",
		"summary":         "Follow up",
		"default_user_id": userID,
		"chaining_type":   "suggest",
		"active":          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recommendedTypeID, err := server.Env.Model("mail.activity.type").Create(map[string]any{
		"name":          "Call",
		"summary":       "Call now",
		"category":      "phonecall",
		"chaining_type": "suggest",
		"active":        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("mail.activity.type").Browse(activityTypeID).Write(map[string]any{"suggested_next_type_ids": []int64{recommendedTypeID}}); err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": activityTypeID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          userID,
		"date_deadline":    "2026-07-03",
		"summary":          "Manual next",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.activity","method":"action_feedback_schedule_next","args":[[%d],"Done"]}`, activityID))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "mail.activity" || action["target"] != "new" {
		t.Fatalf("feedback schedule action = %+v", action)
	}
	context := action["context"].(map[string]any)
	if int64Value(context["default_previous_activity_type_id"]) != activityTypeID || int64Value(context["default_res_id"]) != partnerID || context["default_res_model"] != "res.partner" || context["activity_previous_deadline"] != "2026-07-03" {
		t.Fatalf("feedback schedule context = %+v", context)
	}
	rows, err := server.Env.Model("mail.activity").Browse(activityID).Read("state", "active", "feedback")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "done" || rows[0]["active"] != false || rows[0]["feedback"] != "Done" {
		t.Fatalf("feedback archived row = %+v", rows)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"res.partner","method":"activity_schedule","args":[[%d]],"kwargs":{"recommended_activity_type_id":%d,"summary":"Next manual","date_deadline":"2026-07-08","context":%s}}`, partnerID, recommendedTypeID, mustJSON(t, context)))
	var scheduledIDs []int64
	if err := json.Unmarshal([]byte(body), &scheduledIDs); err != nil {
		t.Fatal(err)
	}
	if len(scheduledIDs) != 1 {
		t.Fatalf("scheduled ids = %+v", scheduledIDs)
	}
	rows, err = server.Env.Model("mail.activity").Browse(scheduledIDs...).Read("activity_type_id", "recommended_activity_type_id", "previous_activity_type_id", "has_recommended_activities", "activity_category", "chaining_type", "summary", "active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["activity_type_id"] != recommendedTypeID || rows[0]["recommended_activity_type_id"] != recommendedTypeID || rows[0]["previous_activity_type_id"] != activityTypeID || rows[0]["has_recommended_activities"] != true || rows[0]["activity_category"] != "phonecall" || rows[0]["chaining_type"] != "suggest" || rows[0]["summary"] != "Next manual" || rows[0]["active"] != true {
		t.Fatalf("scheduled next row = %+v", rows)
	}
}

func TestCallKWMailActivityActionFeedback(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "activity-direct", "name": "Activity User", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activitySubtypeID, err := server.Env.Model("mail.message.subtype").Create(map[string]any{
		"name":     "Activities",
		"default":  true,
		"internal": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "mail",
		"name":          "mt_activities",
		"complete_name": "mail.mt_activities",
		"model":         "mail.message.subtype",
		"res_id":        activitySubtypeID,
	}); err != nil {
		t.Fatal(err)
	}
	activityTypeID, err := server.Env.Model("mail.activity.type").Create(map[string]any{
		"name":          "To-Do",
		"summary":       "Follow up",
		"icon":          "fa-check",
		"chaining_type": "suggest",
		"active":        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("mail.activity").Create(map[string]any{
		"activity_type_id": activityTypeID,
		"res_model":        "res.partner",
		"res_id":           partnerID,
		"user_id":          userID,
		"date_deadline":    "2026-07-03",
		"summary":          "Manual",
		"note":             "<p>Call first</p>",
		"state":            "open",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	activityAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "activity-direct.txt",
		"res_model": "mail.activity",
		"res_id":    activityID,
		"type":      "binary",
		"datas":     "ZGlyZWN0",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.activity","method":"action_feedback","args":[[%d]],"kwargs":{"feedback":"Direct done"}}`, activityID))
	var messageID int64
	if err := json.Unmarshal([]byte(body), &messageID); err != nil {
		t.Fatal(err)
	}
	if messageID == 0 {
		t.Fatalf("message id = %d", messageID)
	}
	messageRows, err := server.Env.Model("mail.message").Browse(messageID).Read("body", "model", "res_id", "subtype_id", "mail_activity_type_id", "attachment_ids", "body_is_html")
	if err != nil {
		t.Fatal(err)
	}
	bodyHTML := stringValue(messageRows[0]["body"])
	for _, want := range []string{`<span class="fa fa-check fa-fw"></span><span>To-Do</span> done: Manual`, `<div class="o_mail_note_title fw-bold">Original note:</div><div><p>Call first</p></div>`, `<div class="fw-bold">Feedback:</div>Direct done`} {
		if !strings.Contains(bodyHTML, want) {
			t.Fatalf("message body missing %q: %s", want, bodyHTML)
		}
	}
	if len(messageRows) != 1 || messageRows[0]["model"] != "res.partner" || messageRows[0]["res_id"] != partnerID || messageRows[0]["subtype_id"] != activitySubtypeID || messageRows[0]["mail_activity_type_id"] != activityTypeID || messageRows[0]["body_is_html"] != true {
		t.Fatalf("message row = %+v", messageRows)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 1 || got[0] != activityAttachmentID {
		t.Fatalf("message attachment ids = %#v", messageRows[0]["attachment_ids"])
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Browse(activityAttachmentID).Read("res_model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if attachmentRows[0]["res_model"] != "mail.message" || attachmentRows[0]["res_id"] != messageID {
		t.Fatalf("activity attachment row = %+v", attachmentRows[0])
	}
	activityRows, err := server.Env.Model("mail.activity").Browse(activityID).Read("state", "active", "feedback", "date_done")
	if err != nil {
		t.Fatal(err)
	}
	if activityRows[0]["state"] != "done" || activityRows[0]["active"] != false || activityRows[0]["feedback"] != "Direct done" || activityRows[0]["date_done"] == nil {
		t.Fatalf("activity row = %+v", activityRows)
	}
}

func TestCallKWMailActivityFormatDataAndReschedule(t *testing.T) {
	server := testMailThreadServer(t)
	partnerID, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Activity Target", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "activity-format", "name": "Activity User", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	newUserID, err := server.Env.Model("res.users").Create(map[string]any{"login": "activity-new", "name": "New User", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{"name": "Reminder", "model": "res.partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	activityTypeID, err := server.Env.Model("mail.activity.type").Create(map[string]any{
		"name":              "To-Do",
		"summary":           "Follow up",
		"category":          "phonecall",
		"chaining_type":     "suggest",
		"mail_template_ids": []int64{templateID},
		"active":            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{"module": "mail", "name": "mail_activity_data_todo", "model": "mail.activity.type", "res_id": activityTypeID}); err != nil {
		t.Fatal(err)
	}
	activityID, err := server.Env.Model("mail.activity").Create(map[string]any{
		"activity_type_id":  activityTypeID,
		"activity_category": "phonecall",
		"res_model":         "res.partner",
		"res_id":            partnerID,
		"user_id":           userID,
		"date_deadline":     "2026-07-02",
		"summary":           "Call customer",
		"note":              "<p>Prepare</p>",
		"state":             "open",
		"automated":         true,
		"active":            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.activity","method":"activity_format","args":[[%d]],"kwargs":{"context":{"mail_activity_today":"2026-07-02"}}}`, activityID))
	payload := decodeJSON(t, []byte(body))
	activityRows := payload["mail.activity"].([]any)
	if len(activityRows) != 1 {
		t.Fatalf("activity format payload = %+v", payload)
	}
	activity := activityRows[0].(map[string]any)
	if activity["display_name"] != "Call customer" || activity["state"] != "today" || int64Value(activity["activity_type_id"]) != activityTypeID {
		t.Fatalf("activity format row = %+v", activity)
	}
	note := activity["note"].([]any)
	if len(note) != 2 || note[0] != "markup" || note[1] != "<p>Prepare</p>" {
		t.Fatalf("activity note = %+v", activity["note"])
	}
	types := payload["mail.activity.type"].([]any)
	if len(types) != 1 || int64Value(types[0].(map[string]any)["id"]) != activityTypeID {
		t.Fatalf("activity format types = %+v", types)
	}
	templates := payload["mail.template"].([]any)
	if len(templates) != 1 || templates[0].(map[string]any)["name"] != "Reminder" {
		t.Fatalf("activity format templates = %+v", templates)
	}

	body = postCallKW(t, handler, `{"model":"mail.activity","method":"get_activity_data","args":["res.partner",[],0,0,true],"kwargs":{"context":{"mail_activity_today":"2026-07-02"}}}`)
	payload = decodeJSON(t, []byte(body))
	resIDs := payload["activity_res_ids"].([]any)
	if len(resIDs) != 1 || int64Value(resIDs[0]) != partnerID {
		t.Fatalf("activity data res ids = %+v", payload)
	}
	grouped := payload["grouped_activities"].(map[string]any)
	byType := grouped[strconv.FormatInt(partnerID, 10)].(map[string]any)
	group := byType[strconv.FormatInt(activityTypeID, 10)].(map[string]any)
	if group["state"] != "today" || group["reporting_date"] != "2026-07-02" {
		t.Fatalf("activity data group = %+v", group)
	}
	dataTypes := payload["activity_types"].([]any)
	templateRows := dataTypes[0].(map[string]any)["template_ids"].([]any)
	if len(templateRows) != 1 || templateRows[0].(map[string]any)["name"] != "Reminder" {
		t.Fatalf("activity data types = %+v", dataTypes)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"res.partner","method":"activity_reschedule","args":[[%d],["mail.mail_activity_data_todo"],null,"2026-08-01",%d]}`, partnerID, newUserID))
	var rescheduled []int64
	if err := json.Unmarshal([]byte(body), &rescheduled); err != nil {
		t.Fatal(err)
	}
	if len(rescheduled) != 1 || rescheduled[0] != activityID {
		t.Fatalf("rescheduled ids = %+v", rescheduled)
	}
	rows, err := server.Env.Model("mail.activity").Browse(activityID).Read("date_deadline", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["date_deadline"] != "2026-08-01" || rows[0]["user_id"] != newUserID {
		t.Fatalf("rescheduled row = %+v", rows[0])
	}
}

func TestAIRouteGenerateResponseReturnsNullAndPosts(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	channel := httpAIChannel(10, true)
	channel.AIEnvContext = []string{"record context"}
	store.channels[10] = channel
	store.messages[30] = aicontrollers.Message{ID: 30, Body: "<p>Hello&nbsp;AI</p>"}
	store.history[10] = []aicontrollers.Message{{ID: 29, Body: "Earlier"}}
	responder := &httpAIResponder{response: agents.Response{Text: "Answer", Model: "mock-chat"}}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: responder}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":7,"params":{"mail_message_id":30,"channel_id":10,"current_view_info":{"model":"res.partner","view_type":"form"},"ai_session_identifier":"sid"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/generate_response", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("generate response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["jsonrpc"] != "2.0" || payload["id"] != float64(7) {
		t.Fatalf("rpc payload = %#v", payload)
	}
	if payload["result"] != nil {
		t.Fatalf("ai result = %#v", payload["result"])
	}
	if store.posted[10][0] != "Answer" {
		t.Fatalf("posted = %+v", store.posted)
	}
	if len(responder.requests) != 1 || responder.requests[0].UserID != 1 || responder.requests[0].CompanyID != 1 || responder.requests[0].Prompt != "Hello AI" {
		t.Fatalf("request = %+v", responder.requests)
	}
}

func TestAIRouteGenerateResponseMissingMessageNoop(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[10] = httpAIChannel(10, true)
	responder := &httpAIResponder{response: agents.Response{Text: "Answer"}}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: responder}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":8,"params":{"mail_message_id":999,"channel_id":10}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/generate_response", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("generate response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["result"] != nil || len(responder.requests) != 0 || len(store.posted) != 0 {
		t.Fatalf("payload=%#v requests=%+v posted=%+v", payload, responder.requests, store.posted)
	}
}

func TestAIRouteGenerateResponseRejectsNonAIChannel(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[10] = aicontrollers.Channel{ID: 10}
	store.messages[30] = aicontrollers.Message{ID: 30, Body: "Hello"}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: &httpAIResponder{}}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":9,"params":{"mail_message_id":30,"channel_id":10}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/generate_response", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("not ai channel = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAIRouteGenerateResponseRejectsNonMemberAIChannel(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[10] = httpAIChannel(10, false)
	store.messages[30] = aicontrollers.Message{ID: 30, Body: "Hello"}
	responder := &httpAIResponder{}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: responder}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":9,"params":{"mail_message_id":30,"channel_id":10}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/generate_response", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-member channel = %d %s", rec.Code, rec.Body.String())
	}
	if len(responder.requests) != 0 || len(store.posted) != 0 {
		t.Fatalf("requests=%+v posted=%+v", responder.requests, store.posted)
	}
}

func TestAIRouteGenerateResponseProviderFailureReturnsBadGateway(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[10] = httpAIChannel(10, true)
	store.messages[30] = aicontrollers.Message{ID: 30, Body: "Hello"}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: &httpAIResponder{err: aiproviders.ProviderHTTPError{Provider: aiproviders.KindOpenAI, Operation: "chat", Status: http.StatusTooManyRequests}}}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":9,"params":{"mail_message_id":30,"channel_id":10}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/generate_response", body))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("provider failure = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("provider error leaked sensitive text: %s", rec.Body.String())
	}
}

func TestAIRouteCloseAIChatReturnsNullAndDeletesOnlyMemberAIChannel(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[10] = httpAIChannel(10, true)
	server.AIChat = &aicontrollers.ChatService{Store: store}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":10,"params":{"channel_id":10}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/close_ai_chat", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("close response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["result"] != nil || !store.deleted[10] {
		t.Fatalf("payload=%#v deleted=%+v", payload, store.deleted)
	}
}

func TestAIRouteCloseAIChatNoopsForNonMemberAndNonAIChannel(t *testing.T) {
	server := testServer(t)
	store := newHTTPAIStore()
	store.channels[20] = httpAIChannel(20, false)
	store.channels[30] = aicontrollers.Channel{ID: 30, IsMember: true}
	server.AIChat = &aicontrollers.ChatService{Store: store}
	handler := server.Handler()

	for _, channelID := range []int64{20, 30} {
		rec := httptest.NewRecorder()
		body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"params":{"channel_id":%d}}`, channelID, channelID))
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/close_ai_chat", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("close response %d %s", rec.Code, rec.Body.String())
		}
		payload := decodeJSON(t, rec.Body.Bytes())
		if payload["result"] != nil || store.deleted[channelID] {
			t.Fatalf("channel=%d payload=%#v deleted=%+v", channelID, payload, store.deleted)
		}
	}
}

func TestAIRoutesRequireSecuritySessionCookie(t *testing.T) {
	server := testServer(t)
	engine := security.NewEngine()
	engine.Users[9] = security.User{ID: 9, Login: "ai-user", Active: true, CompanyID: 2, CompanyIDs: []int64{2}}
	engine.IssueSession(9, "ai-sid", time.Now().Add(time.Hour))
	server.Security = engine
	store := newHTTPAIStore()
	store.channels[10] = httpAIChannel(10, true)
	store.messages[30] = aicontrollers.Message{ID: 30, Body: "Hello"}
	responder := &httpAIResponder{response: agents.Response{Text: "Answer", Model: "mock-chat"}}
	server.AIChat = &aicontrollers.ChatService{Store: store, Responder: responder}
	provider := &httpTranscriptionProvider{response: httpTranscriptionSession()}
	server.AITranscript = &aicontrollers.TranscriptionService{Provider: provider}
	handler := server.Handler()

	for _, route := range []struct {
		target string
		body   string
	}{
		{aicontrollers.RouteGenerateResponse, `{"jsonrpc":"2.0","id":"ai","params":{"mail_message_id":30,"channel_id":10}}`},
		{aicontrollers.RouteCloseAIChat, `{"jsonrpc":"2.0","id":"close","params":{"channel_id":10}}`},
		{aicontrollers.RouteTranscriptionSession, `{"jsonrpc":"2.0","id":"voice","params":{"language":"en","prompt":"Summarize"}}`},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, route.target, bytes.NewBufferString(route.body)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated response %d %s", route.target, rec.Code, rec.Body.String())
		}
	}
	if len(responder.requests) != 0 || store.deleted[10] || provider.params.Session.Type != "" {
		t.Fatalf("unauthenticated side effects: requests=%+v deleted=%+v params=%+v", responder.requests, store.deleted, provider.params)
	}

	req := httptest.NewRequest(http.MethodPost, aicontrollers.RouteGenerateResponse, bytes.NewBufferString(`{"jsonrpc":"2.0","id":"ai","params":{"mail_message_id":30,"channel_id":10}}`))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "ai-sid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated generate response %d %s", rec.Code, rec.Body.String())
	}
	if len(responder.requests) != 1 || responder.requests[0].UserID != 9 || responder.requests[0].CompanyID != 2 {
		t.Fatalf("authenticated request = %+v", responder.requests)
	}
}

func TestAIRouteTranscriptionSessionRequiresUser(t *testing.T) {
	server := testServer(t)
	ctx := server.Env.Context()
	ctx.UserID = 0
	server.Env = server.Env.WithContext(ctx)
	provider := &httpTranscriptionProvider{response: httpTranscriptionSession()}
	server.AITranscript = &aicontrollers.TranscriptionService{Provider: provider}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":"voice","params":{"language":"en","prompt":"Summarize"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/transcription/session", body))
	if rec.Code != http.StatusForbidden || provider.params.Session.Type != "" {
		t.Fatalf("transcription auth = %d %s params=%+v", rec.Code, rec.Body.String(), provider.params)
	}
}

func TestAIRouteTranscriptionSessionReturnsProviderShape(t *testing.T) {
	server := testServer(t)
	provider := &httpTranscriptionProvider{response: httpTranscriptionSession()}
	server.AITranscript = &aicontrollers.TranscriptionService{Provider: provider}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":"voice","params":{"language":"en","prompt":"Summarize"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/transcription/session", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("transcription response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	if result["value"] != "secret" || result["expires_at"] != float64(7200) || result["token"] != nil {
		t.Fatalf("session result = %#v", result)
	}
	session := result["session"].(map[string]any)
	if session["type"] != "transcription" {
		t.Fatalf("session = %#v", session)
	}
}

func TestAIRouteTranscriptionSessionBuildsOdooRealtimeParams(t *testing.T) {
	server := testServer(t)
	provider := &httpTranscriptionProvider{response: httpTranscriptionSession()}
	server.AITranscript = &aicontrollers.TranscriptionService{Provider: provider}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":"voice","params":{"language":"en","prompt":"Summarize"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ai/transcription/session", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("transcription response %d %s", rec.Code, rec.Body.String())
	}
	if provider.params.ExpiresAfter.Anchor != "created_at" || provider.params.ExpiresAfter.Seconds != aicontrollers.DefaultTokenLifespanSeconds {
		t.Fatalf("expires_after = %+v", provider.params.ExpiresAfter)
	}
	input := provider.params.Session.Audio.Input
	if provider.params.Session.Type != "transcription" ||
		input.Transcription.Language != "en" ||
		input.Transcription.Model != "whisper-1" ||
		input.Transcription.Prompt != "Summarize" ||
		input.TurnDetection.Type != "server_vad" ||
		input.NoiseReduction.Type != "far_field" {
		t.Fatalf("transcription params = %+v", provider.params)
	}
}

func TestAIRouteMethodGuard(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ai/generate_response", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method guard = %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebJSONRPCRoutes(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"params":{}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/session/get_session_info", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("session_info status %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["id"] != float64(1) {
		t.Fatalf("session_info id %#v", payload["id"])
	}
	result := payload["result"].(map[string]any)
	if result["uid"] != float64(1) {
		t.Fatalf("session_info result %#v", result)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"params":{"args":[{"name":"Path Demo"}]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/create", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("path create status %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	result = payload["result"].(map[string]any)
	if result["id"] != float64(1) {
		t.Fatalf("path create result %#v", result)
	}
}

func TestWebDatasetCallKWCRUD(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"model":"res.partner","method":"create","values":{"name":"Demo"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("create status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"create","values":{"name":"Alpha"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("second create status %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"read","args":[[1]],"kwargs":{"fields":["name"]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Demo"`) {
		t.Fatalf("read response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"search_read","args":[[["name","=","Demo"]]],"kwargs":{"fields":["name"]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Demo"`) {
		t.Fatalf("search_read response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"search_read","args":[["|",["name","ilike","alp"],["name","=","Demo"]]],"kwargs":{"fields":["name"]}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Demo"`) || !strings.Contains(rec.Body.String(), `"name":"Alpha"`) {
		t.Fatalf("prefix search_read response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"search_read","kwargs":{"domain":[],"fields":["name"],"order":"name asc","limit":1}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("ordered search_read response %d %s", rec.Code, rec.Body.String())
	}
	var searchRows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &searchRows); err != nil {
		t.Fatal(err)
	}
	if len(searchRows) != 1 || searchRows[0]["name"] != "Alpha" {
		t.Fatalf("ordered search_read rows = %+v", searchRows)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"search_read","kwargs":{"domain":[],"fields":["name"],"order":"missing desc"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "invalid search order field") {
		t.Fatalf("invalid search_read order response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"write","args":[[1],{"name":"Renamed"}]}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `true`) {
		t.Fatalf("write response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"res.partner","method":"unlink","args":[[1]]}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `true`) {
		t.Fatalf("unlink response %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebDatasetCallKWSanitizesCatchallAllowedDomains(t *testing.T) {
	t.Run("create_write", func(t *testing.T) {
		server := testMailThreadServer(t)
		handler := server.Handler()
		body := postCallKW(t, handler, `{"model":"ir.config_parameter","method":"create","values":{"key":"mail.catchall.domain.allowed","value":"HELLO.com,, BONJOUR.com"}}`)
		payload := decodeJSON(t, []byte(body))
		id := int64Value(payload["id"])
		rows, err := server.Env.Model("ir.config_parameter").Browse(id).Read("value")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["value"] != "hello.com,bonjour.com" {
			t.Fatalf("created row = %+v", rows)
		}

		body = postCallKW(t, handler, fmt.Sprintf(`{"model":"ir.config_parameter","method":"write","args":[[%d],{"value":"SECOND.com,, THIRD.com"}]}`, id))
		if strings.TrimSpace(body) != "true" {
			t.Fatalf("write response %s", body)
		}
		rows, err = server.Env.Model("ir.config_parameter").Browse(id).Read("value")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["value"] != "second.com,third.com" {
			t.Fatalf("written row = %+v", rows)
		}
	})

	t.Run("rejects_invalid_create", func(t *testing.T) {
		for _, value := range []string{",", ",,", ", ,"} {
			server := testMailThreadServer(t)
			rec := httptest.NewRecorder()
			payload := fmt.Sprintf(`{"model":"ir.config_parameter","method":"create","values":{"key":"mail.catchall.domain.allowed","value":%q}}`, value)
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(payload)))
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cannot be validated") {
				t.Fatalf("create %q response %d %s", value, rec.Code, rec.Body.String())
			}
			found, err := server.Env.Model("ir.config_parameter").Search(domain.Cond("key", domain.Equal, "mail.catchall.domain.allowed"))
			if err != nil {
				t.Fatal(err)
			}
			if found.Len() != 0 {
				t.Fatalf("target row created for %q", value)
			}
		}
	})

	t.Run("rejects_invalid_write", func(t *testing.T) {
		server := testMailThreadServer(t)
		id, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.catchall.domain.allowed", "value": "valid.example"})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		payload := fmt.Sprintf(`{"model":"ir.config_parameter","method":"write","args":[[%d],{"value":", ,"}]}`, id)
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(payload)))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cannot be validated") {
			t.Fatalf("write response %d %s", rec.Code, rec.Body.String())
		}
		rows, err := server.Env.Model("ir.config_parameter").Browse(id).Read("value")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["value"] != "valid.example" {
			t.Fatalf("preserved row = %+v", rows)
		}
	})

	t.Run("non_target_untouched", func(t *testing.T) {
		server := testMailThreadServer(t)
		handler := server.Handler()
		body := postCallKW(t, handler, `{"model":"ir.config_parameter","method":"create","values":{"key":"x.mail.catchall.domain.allowed","value":", ,"}}`)
		payload := decodeJSON(t, []byte(body))
		id := int64Value(payload["id"])
		body = postCallKW(t, handler, fmt.Sprintf(`{"model":"ir.config_parameter","method":"write","args":[[%d],{"value":"HELLO.COM, BONJOUR.com,,"}]}`, id))
		if strings.TrimSpace(body) != "true" {
			t.Fatalf("write response %s", body)
		}
		rows, err := server.Env.Model("ir.config_parameter").Browse(id).Read("value")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["value"] != "HELLO.COM, BONJOUR.com,," {
			t.Fatalf("non-target row = %+v", rows)
		}
	})
}

func TestWebDatasetCallKWRejectsMailAliasCompanyDomainMismatch(t *testing.T) {
	server := testMailThreadServer(t)
	handler := server.Handler()
	domainA, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "a.example"})
	if err != nil {
		t.Fatal(err)
	}
	domainB, err := server.Env.Model("mail.alias.domain").Create(map[string]any{"name": "b.example"})
	if err != nil {
		t.Fatal(err)
	}
	companyA, err := server.Env.Model("res.company").Create(map[string]any{"name": "Company A", "alias_domain_id": domainA})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("res.company").Create(map[string]any{"name": "Company B", "alias_domain_id": domainB}); err != nil {
		t.Fatal(err)
	}
	companyNoDomain, err := server.Env.Model("res.company").Create(map[string]any{"name": "No Domain Company"})
	if err != nil {
		t.Fatal(err)
	}
	partnerA, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Partner A", "company_id": companyA, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	partnerNoDomain, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Partner No Domain", "company_id": companyNoDomain, "active": true})
	if err != nil {
		t.Fatal(err)
	}
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	createPayload := fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"http-owner-bad","alias_domain_id":%d,"alias_model_id":%d,"alias_parent_model_id":%d,"alias_parent_thread_id":%d,"active":true}}`, domainB, modelID, modelID, partnerA)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(createPayload)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "owner document belongs to company Company A") {
		t.Fatalf("owner create response %d %s", rec.Code, rec.Body.String())
	}
	found, err := server.Env.Model("mail.alias").Search(domain.Cond("alias_name", domain.Equal, "http-owner-bad"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("invalid alias created count = %d", found.Len())
	}

	rec = httptest.NewRecorder()
	noDomainPayload := fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"http-owner-no-company-domain","alias_domain_id":%d,"alias_model_id":%d,"alias_parent_model_id":%d,"alias_parent_thread_id":%d,"active":true}}`, domainA, modelID, modelID, partnerNoDomain)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(noDomainPayload)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "owner document belongs to company No Domain Company") {
		t.Fatalf("owner no-domain response %d %s", rec.Code, rec.Body.String())
	}

	targetAliasID, err := server.Env.Model("mail.alias").Create(map[string]any{
		"alias_name":            "http-target-ok",
		"alias_domain_id":       domainA,
		"model_name":            "res.partner",
		"alias_model_id":        modelID,
		"alias_force_thread_id": partnerA,
		"active":                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	writePayload := fmt.Sprintf(`{"model":"mail.alias","method":"write","args":[[%d],{"alias_domain_id":%d}]}`, targetAliasID, domainB)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(writePayload)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "target document belongs to company Company A") {
		t.Fatalf("target write response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("mail.alias").Browse(targetAliasID).Read("alias_domain_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_domain_id"] != domainA {
		t.Fatalf("preserved alias rows = %+v", rows)
	}
}

func TestWebDatasetCallKWMailAliasIntegrity(t *testing.T) {
	server := testMailThreadServer(t)
	handler := server.Handler()
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, handler, `{"model":"mail.alias.domain","method":"create","values":{"name":"http-unique-a.example"}}`)
	domainA := int64Value(decodeJSON(t, []byte(body))["id"])
	body = postCallKW(t, handler, `{"model":"mail.alias.domain","method":"create","values":{"name":"http-unique-b.example"}}`)
	domainB := int64Value(decodeJSON(t, []byte(body))["id"])
	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"HTTP Sales","alias_domain_id":%d,"alias_model_id":%d}}`, domainA, modelID))
	aliasID := int64Value(decodeJSON(t, []byte(body))["id"])
	rows, err := server.Env.Model("mail.alias").Browse(aliasID).Read("alias_name", "alias_full_name", "alias_defaults", "alias_status")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_name"] != "http-sales" || rows[0]["alias_full_name"] != "http-sales@http-unique-a.example" || rows[0]["alias_defaults"] != "{}" || rows[0]["alias_status"] != "not_tested" {
		t.Fatalf("created alias rows = %+v", rows)
	}
	rec := httptest.NewRecorder()
	createDuplicate := fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"http-sales","alias_domain_id":%d,"alias_model_id":%d}}`, domainA, modelID)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(createDuplicate)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cannot be used on several records") {
		t.Fatalf("duplicate create response %d %s", rec.Code, rec.Body.String())
	}
	if _, err := server.Env.Model("mail.alias").Create(map[string]any{"alias_name": "http-sales", "alias_domain_id": domainB, "alias_model_id": modelID}); err != nil {
		t.Fatal(err)
	}
	otherID, err := server.Env.Model("mail.alias").Create(map[string]any{"alias_name": "http-other", "alias_domain_id": domainB, "alias_model_id": modelID})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	writeDuplicate := fmt.Sprintf(`{"model":"mail.alias","method":"write","args":[[%d],{"alias_name":"http-sales","alias_domain_id":%d}]}`, otherID, domainA)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(writeDuplicate)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cannot be used on several records") {
		t.Fatalf("duplicate write response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("mail.alias").Browse(otherID).Read("alias_name", "alias_domain_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["alias_name"] != "http-other" || rows[0]["alias_domain_id"] != domainB {
		t.Fatalf("duplicate write rollback rows = %+v", rows)
	}
	rec = httptest.NewRecorder()
	invalidDefaults := fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"http-invalid-defaults","alias_domain_id":%d,"alias_model_id":%d,"alias_defaults":"[bad]"}}`, domainA, modelID)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(invalidDefaults)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "literal python dictionary") {
		t.Fatalf("invalid defaults response %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebDatasetCallKWMailAliasDomainIntegrity(t *testing.T) {
	server := testMailThreadServer(t)
	handler := server.Handler()
	modelID, err := server.Env.Model("ir.model").Create(map[string]any{"model": "res.partner", "name": "Contact"})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, handler, `{"model":"mail.alias.domain","method":"create","values":{"name":"http-domain.example","bounce_alias":"Bounce Box","catchall_alias":"Catch All","default_from":"Notify Team@Example.COM"}}`)
	domainID := int64Value(decodeJSON(t, []byte(body))["id"])
	rows, err := server.Env.Model("mail.alias.domain").Browse(domainID).Read("bounce_alias", "catchall_alias", "default_from", "bounce_email", "catchall_email", "default_from_email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 ||
		rows[0]["bounce_alias"] != "bounce-box" ||
		rows[0]["catchall_alias"] != "catch-all" ||
		rows[0]["default_from"] != "notify-team@example.com" ||
		rows[0]["bounce_email"] != "bounce-box@http-domain.example" ||
		rows[0]["catchall_email"] != "catch-all@http-domain.example" ||
		rows[0]["default_from_email"] != "notify-team@example.com" {
		t.Fatalf("domain rows = %+v", rows)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(`{"model":"mail.alias.domain","method":"create","values":{"name":"bad,domain"}}`)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "domain name") {
		t.Fatalf("invalid domain response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	createAliasClash := fmt.Sprintf(`{"model":"mail.alias","method":"create","values":{"alias_name":"Bounce Box","alias_domain_id":%d,"alias_model_id":%d}}`, domainID, modelID)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(createAliasClash)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "bounce or catchall address") {
		t.Fatalf("alias clash response %d %s", rec.Code, rec.Body.String())
	}
	aliasID, err := server.Env.Model("mail.alias").Create(map[string]any{"alias_name": "reserved", "alias_domain_id": domainID, "alias_model_id": modelID})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	writeClash := fmt.Sprintf(`{"model":"mail.alias.domain","method":"write","args":[[%d],{"bounce_alias":"Reserved"}]}`, domainID)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(writeClash)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "already used") {
		t.Fatalf("domain write clash response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("mail.alias.domain").Browse(domainID).Read("bounce_alias", "bounce_email")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["bounce_alias"] != "bounce-box" || rows[0]["bounce_email"] != "bounce-box@http-domain.example" {
		t.Fatalf("domain write rollback rows = %+v alias=%d", rows, aliasID)
	}
}

func TestWebDatasetModelMethods(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Alpha"}}`)
	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Beta"}}`)

	body := postCallKW(t, handler, `{"model":"res.partner","method":"fields_get","args":[["name","display_name"],["string","type"]]}`)
	if !strings.Contains(body, `"name":{"string":"name","type":"char"}`) || !strings.Contains(body, `"display_name"`) {
		t.Fatalf("fields_get response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"default_get","args":[["name"]],"kwargs":{"context":{"default_name":"Context Name"}}}`)
	if !strings.Contains(body, `"name":"Context Name"`) {
		t.Fatalf("default_get response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"search_count","args":[[["name","ilike","a"]]],"kwargs":{"limit":1}}`)
	if strings.TrimSpace(body) != "1" {
		t.Fatalf("search_count response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"search","kwargs":{"domain":[],"order":"name desc","limit":1}}`)
	if strings.TrimSpace(body) != "[2]" {
		t.Fatalf("ordered search response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"name_search","kwargs":{"name":"alp","domain":[],"operator":"ilike","limit":10}}`)
	if !strings.Contains(body, `"Alpha"`) {
		t.Fatalf("name_search response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"web_read","args":[[1]],"kwargs":{"specification":{"name":{},"display_name":{}}}}`)
	if !strings.Contains(body, `"display_name":"Alpha"`) || !strings.Contains(body, `"name":"Alpha"`) {
		t.Fatalf("web_read response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"web_search_read","kwargs":{"domain":[["name","ilike","a"]],"specification":{"display_name":{}},"limit":1,"order":"name desc"}}`)
	if !strings.Contains(body, `"length":2`) || !strings.Contains(body, `"records"`) || !strings.Contains(body, `"display_name":"Beta"`) {
		t.Fatalf("web_search_read response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"web_save","args":[[],{"name":"Gamma"}],"kwargs":{"specification":{"display_name":{}}}}`)
	if !strings.Contains(body, `"display_name":"Gamma"`) {
		t.Fatalf("web_save create response %s", body)
	}

	body = postCallKW(t, handler, `{"model":"res.partner","method":"web_save_multi","args":[[1,2],[{"name":"A1"},{"name":"B2"}]],"kwargs":{"specification":{"name":{}}}}`)
	if !strings.Contains(body, `"name":"A1"`) || !strings.Contains(body, `"name":"B2"`) {
		t.Fatalf("web_save_multi response %s", body)
	}
}

func TestWebDatasetActiveTestContext(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()
	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Visible","active":true}}`)
	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Archived","active":false}}`)

	body := postCallKW(t, handler, `{"model":"res.partner","method":"search","kwargs":{"domain":[],"order":"name asc"}}`)
	if strings.Contains(body, "2") || !strings.Contains(body, "1") {
		t.Fatalf("default active search response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"search","kwargs":{"domain":[],"order":"name asc","context":{"active_test":false}}}`)
	if !strings.Contains(body, "1") || !strings.Contains(body, "2") {
		t.Fatalf("active_test false search response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"search_count","kwargs":{"domain":[]}}`)
	if strings.TrimSpace(body) != "1" {
		t.Fatalf("default active search_count response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"search_count","kwargs":{"domain":[],"context":{"active_test":false}}}`)
	if strings.TrimSpace(body) != "2" {
		t.Fatalf("active_test false search_count response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"name_search","kwargs":{"name":"Archived","domain":[],"operator":"ilike","limit":10,"context":{"active_test":false}}}`)
	if !strings.Contains(body, `"Archived"`) {
		t.Fatalf("active_test false name_search response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"search_read","kwargs":{"domain":[["name","=","Archived"]],"fields":["name"]}}`)
	if strings.Contains(body, "Archived") {
		t.Fatalf("default active search_read response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"search_read","kwargs":{"domain":[["name","=","Archived"]],"fields":["name"],"context":{"active_test":false}}}`)
	if !strings.Contains(body, `"name":"Archived"`) {
		t.Fatalf("active_test false search_read response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"web_search_read","kwargs":{"domain":[],"specification":{"display_name":{}},"context":{"active_test":false},"order":"name asc"}}`)
	if !strings.Contains(body, `"length":2`) || !strings.Contains(body, `"display_name":"Archived"`) || !strings.Contains(body, `"display_name":"Visible"`) {
		t.Fatalf("active_test false web_search_read response %s", body)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"read_group","kwargs":{"domain":[],"fields":["id:count"],"groupby":["active"],"context":{"active_test":false}}}`)
	if !strings.Contains(body, `"active":false`) || !strings.Contains(body, `"active":true`) {
		t.Fatalf("active_test false read_group response %s", body)
	}
}

func TestCallKWWebReadX2ManyOrderLimit(t *testing.T) {
	reg := record.NewRegistry()
	parent := model.New("x.http.parent", "x_http_parent")
	parent.AddField(field.New("name", field.Char))
	parent.AddField(field.New("line_ids", field.One2Many).WithRelation("x.http.line").WithRelationField("parent_id"))
	parent.AddField(field.New("all_line_ids", field.One2Many).WithRelation("x.http.line").WithRelationField("parent_id").WithContext(map[string]any{"active_test": false}))
	parent.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.http.tag"))
	parent.AddField(field.New("all_tag_ids", field.Many2Many).WithRelation("x.http.tag").WithContext(map[string]any{"active_test": false}))
	if err := reg.Register(parent); err != nil {
		t.Fatal(err)
	}
	tag := model.New("x.http.tag", "x_http_tag")
	tag.Order = "name"
	tag.AddField(field.New("name", field.Char))
	tag.AddField(field.New("active", field.Bool))
	if err := reg.Register(tag); err != nil {
		t.Fatal(err)
	}
	line := model.New("x.http.line", "x_http_line")
	line.Order = "sequence"
	line.AddField(field.New("name", field.Char))
	line.AddField(field.New("sequence", field.Int))
	line.AddField(field.New("active", field.Bool))
	line.AddField(field.New("parent_id", field.Many2One).WithRelation("x.http.parent"))
	if err := reg.Register(line); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	tagAlpha, err := env.Model("x.http.tag").Create(map[string]any{"name": "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	tagBravo, err := env.Model("x.http.tag").Create(map[string]any{"name": "Bravo", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	tagZulu, err := env.Model("x.http.tag").Create(map[string]any{"name": "Zulu"})
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := env.Model("x.http.parent").Create(map[string]any{"name": "Parent", "tag_ids": []int64{tagAlpha, tagBravo, tagZulu}, "all_tag_ids": []int64{tagAlpha, tagBravo, tagZulu}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("x.http.line").Create(map[string]any{"name": "Low", "sequence": int64(10), "parent_id": parentID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("x.http.line").Create(map[string]any{"name": "High", "sequence": int64(20), "parent_id": parentID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("x.http.line").Create(map[string]any{"name": "Archived", "sequence": int64(30), "active": false, "parent_id": parentID}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Env: env}.Handler()

	body := postCallKW(t, handler, fmt.Sprintf(`{
		"model":"x.http.parent",
		"method":"web_read",
		"args":[[%d]],
		"kwargs":{"specification":{
			"line_ids":{"fields":{"name":{}},"order":"sequence desc","limit":1},
			"all_line_ids":{"fields":{"name":{}},"order":"sequence desc"},
			"tag_ids":{"fields":{"display_name":{}},"order":"name asc"},
			"all_tag_ids":{"fields":{"display_name":{}},"order":"name asc"}
		}}
	}`, parentID))
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	lineRows := rows[0]["line_ids"].([]any)
	if len(lineRows) != 2 || lineRows[0].(map[string]any)["name"] != "High" {
		t.Fatalf("web_read line rows = %#v", rows)
	}
	if _, ok := lineRows[1].(map[string]any)["name"]; ok {
		t.Fatalf("web_read line rows = %#v", rows)
	}
	allLineRows := rows[0]["all_line_ids"].([]any)
	if len(allLineRows) != 3 || allLineRows[0].(map[string]any)["name"] != "Archived" {
		t.Fatalf("web_read all line rows = %#v", rows)
	}
	tagRows := rows[0]["tag_ids"].([]any)
	if len(tagRows) != 2 || tagRows[0].(map[string]any)["display_name"] != "Alpha" || tagRows[1].(map[string]any)["display_name"] != "Zulu" {
		t.Fatalf("web_read tag rows = %#v", rows)
	}
	allTagRows := rows[0]["all_tag_ids"].([]any)
	if len(allTagRows) != 3 || allTagRows[1].(map[string]any)["display_name"] != "Bravo" {
		t.Fatalf("web_read all tag rows = %#v", rows)
	}
}

func TestWebSaveResUsersGroupPayloadCommands(t *testing.T) {
	server, ids := testViewGroupXMLIDServer(t)
	handler := server.Handler()
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "group-http", "name": "Group HTTP", "active": true})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, handler, fmt.Sprintf(`{
		"model":"res.users",
		"method":"web_save",
		"args":[[%d],{"group_ids":[[6,false,[%d]]]}],
		"kwargs":{"specification":{"group_ids":{},"groups_id":{},"all_group_ids":{},"groups_count":{},"role":{},"view_group_hierarchy":{}}}
	}`, userID, ids["base.group_system"]))
	if !strings.Contains(body, `"role":"group_system"`) || !strings.Contains(body, `"groups_count":2`) || !strings.Contains(body, `"view_group_hierarchy"`) {
		t.Fatalf("res.users web_save response %s", body)
	}

	rows, err := server.Env.Model("res.users").Browse(userID).Read("group_ids", "groups_id", "all_group_ids")
	if err != nil {
		t.Fatal(err)
	}
	if !testContainsRecordID(rows[0]["group_ids"], ids["base.group_system"]) || !testContainsRecordID(rows[0]["groups_id"], ids["base.group_system"]) || !testContainsRecordID(rows[0]["all_group_ids"], ids["base.group_user"]) {
		t.Fatalf("stored groups after web_save = %+v", rows[0])
	}
}

func TestOnchangeResUsersRoleUpdatesGroups(t *testing.T) {
	server, ids := testViewGroupXMLIDServer(t)
	handler := server.Handler()
	body := postCallKW(t, handler, fmt.Sprintf(`{
		"model":"res.users",
		"method":"onchange",
		"args":[{"role":"group_system","group_ids":[[6,false,[%d]]]},["role"],{}]
	}`, ids["base.group_user"]))
	payload := decodeJSON(t, []byte(body))
	values := payload["value"].(map[string]any)
	commands := values["group_ids"].([]any)
	command := commands[0].([]any)
	if command[0] != float64(6) || testContainsRecordID(command[2], ids["base.group_user"]) || !testContainsRecordID(command[2], ids["base.group_system"]) {
		t.Fatalf("role onchange group command = %#v", command)
	}
}

func TestOnchangeDelegationSeedsDelegableGroupLines(t *testing.T) {
	server := testDelegationHTTPServer(t)
	handler := server.Handler()
	zuluGroupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Zulu", "full_name": "Role / Zulu", "name_delegation": "Zulu Delegation", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	blockedGroupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Blocked", "full_name": "Role / Blocked", "name_delegation": "Blocked Delegation", "allow_delegation": false})
	if err != nil {
		t.Fatal(err)
	}
	alphaGroupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Alpha", "full_name": "Role / Alpha", "name_delegation": "Alpha Delegation", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"login": "delegator", "name": "Delegator", "groups_id": []int64{zuluGroupID, blockedGroupID, alphaGroupID}})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := server.Env.Model("hr.employee").Create(map[string]any{"name": "Delegator", "user_id": userID})
	if err != nil {
		t.Fatal(err)
	}
	delegateEmployeeID, err := server.Env.Model("hr.employee").Create(map[string]any{"name": "Delegate"})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, handler, fmt.Sprintf(`{
		"model":"delegation",
		"method":"onchange",
		"args":[{"employee_id":%d},["employee_id"],{}],
		"kwargs":{"context":{"delegation":true}}
	}`, employeeID))
	payload := decodeJSON(t, []byte(body))
	values := payload["value"].(map[string]any)
	commands := values["lines"].([]any)
	if len(commands) != 3 {
		t.Fatalf("delegation onchange commands = %#v", commands)
	}
	clearCommand := commands[0].([]any)
	if clearCommand[0] != float64(5) {
		t.Fatalf("delegation onchange clear command = %#v", clearCommand)
	}
	firstCreate := commands[1].([]any)
	secondCreate := commands[2].([]any)
	firstValues := firstCreate[2].(map[string]any)
	secondValues := secondCreate[2].(map[string]any)
	if firstCreate[0] != float64(0) || secondCreate[0] != float64(0) || int64Value(firstValues["group_id"]) != alphaGroupID || int64Value(secondValues["group_id"]) != zuluGroupID {
		t.Fatalf("delegation onchange seeded groups = %#v", commands)
	}
	if testContainsRecordID([]any{firstValues["group_id"], secondValues["group_id"]}, blockedGroupID) {
		t.Fatalf("delegation onchange included non-delegable group = %#v", commands)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{
		"model":"delegation",
		"method":"onchange",
		"args":[{"delegateTo_employee_id":%d,"lines":[[0,false,{"group_id":%d}]]},["delegateTo_employee_id"],{}]
	}`, delegateEmployeeID, alphaGroupID))
	payload = decodeJSON(t, []byte(body))
	values = payload["value"].(map[string]any)
	commands = values["lines"].([]any)
	lineValues := commands[0].([]any)[2].(map[string]any)
	if int64Value(lineValues["employee_id"]) != delegateEmployeeID {
		t.Fatalf("delegation delegate onchange = %#v", commands)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{
		"model":"delegation",
		"method":"onchange",
		"args":[{"one_employee":false,"delegateTo_employee_id":%d},["one_employee"],{}]
	}`, delegateEmployeeID))
	payload = decodeJSON(t, []byte(body))
	values = payload["value"].(map[string]any)
	if values["delegateTo_employee_id"] != false || values["delegate_to_employee_id"] != false {
		t.Fatalf("delegation one_employee onchange = %#v", values)
	}
}

func TestWebSaveDelegationPersistsLineCommands(t *testing.T) {
	server := testDelegationHTTPServer(t)
	handler := server.Handler()
	groupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Delegable", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := server.Env.Model("res.users").Create(map[string]any{"id": int64(1), "login": "delegator", "name": "Delegator", "groups_id": []int64{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	employeeID, err := server.Env.Model("hr.employee").Create(map[string]any{"name": "Delegator", "user_id": userID})
	if err != nil {
		t.Fatal(err)
	}
	body := postCallKW(t, handler, fmt.Sprintf(`{
		"model":"delegation",
		"method":"web_save",
		"args":[[],{"employee_id":%d,"date_to":"2099-12-31","lines":[[0,false,{"group_id":%d}]]}],
		"kwargs":{"specification":{"employee_id":{},"lines":{"fields":{"group_id":{},"delegator_user_id":{}}}}}
	}`, employeeID, groupID))
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delegation web_save rows = %#v", rows)
	}
	lines := rows[0]["lines"].([]any)
	if len(lines) != 1 || firstID(lines[0].(map[string]any)["group_id"]) != groupID || firstID(lines[0].(map[string]any)["delegator_user_id"]) != userID {
		t.Fatalf("delegation web_save lines = %#v", rows)
	}
}

func TestResGroupsDefinitionsAndShowAllUsersAction(t *testing.T) {
	server, ids := testViewGroupXMLIDServer(t)
	handler := server.Handler()
	body := postCallKW(t, handler, `{"model":"res.groups","method":"_get_group_definitions","args":[]}`)
	payload := decodeJSON(t, []byte(body))
	groups := payload["groups"].(map[string]any)
	system := groups[strconv.FormatInt(ids["base.group_system"], 10)].(map[string]any)
	if system["ref"] != "base.group_system" || !testContainsRecordID(system["supersets"], ids["base.group_user"]) {
		t.Fatalf("group definitions system = %#v", system)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"res.groups","method":"action_show_all_users","args":[[%d]]}`, ids["base.group_system"]))
	action := decodeJSON(t, []byte(body))
	if action["type"] != "ir.actions.act_window" || action["res_model"] != "res.users" || action["target"] != "current" {
		t.Fatalf("action = %#v", action)
	}
	domain := action["domain"].([]any)
	if len(domain) != 1 || !testContainsRecordID(domain[0].([]any)[2], ids["base.group_system"]) {
		t.Fatalf("action domain = %#v", domain)
	}
}

func TestWebDatasetReadGroup(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Alpha","active":true}}`)
	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Beta","active":true}}`)
	postCallKW(t, handler, `{"model":"res.partner","method":"create","values":{"name":"Gamma","active":false}}`)

	body := postCallKW(t, handler, `{"model":"res.partner","method":"read_group","args":[[["name","ilike","a"]],["__count"],["active"]],"kwargs":{"context":{"active_test":false}}}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("read_group rows = %#v", rows)
	}
	var activeRow map[string]any
	for _, row := range rows {
		if row["active"] == true {
			activeRow = row
		}
	}
	if activeRow == nil || activeRow["active_count"] != float64(2) {
		t.Fatalf("active group = %#v", activeRow)
	}
	if _, hasRawCount := activeRow["__count"]; hasRawCount {
		t.Fatalf("legacy read_group leaked __count alias = %#v", activeRow)
	}
	groupDomain, ok := activeRow["__domain"].([]any)
	if !ok || len(groupDomain) != 2 {
		t.Fatalf("active domain = %#v", activeRow["__domain"])
	}
	groupCondition, ok := groupDomain[1].([]any)
	if !ok || len(groupCondition) != 3 || groupCondition[0] != "active" || groupCondition[1] != "=" || groupCondition[2] != true {
		t.Fatalf("group condition = %#v", groupDomain[1])
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"read_group","args":[[],["__count"],["active","name"]],"kwargs":{"context":{"active_test":false}}}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	contextValue, ok := rows[0]["__context"].(map[string]any)
	if !ok {
		t.Fatalf("lazy context = %#v", rows[0]["__context"])
	}
	remaining, ok := contextValue["group_by"].([]any)
	if !ok || len(remaining) != 1 || remaining[0] != "name" {
		t.Fatalf("lazy remaining groupby = %#v", contextValue["group_by"])
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"read_group","args":[[],["__count"],["active","name"],0,null,false,false],"kwargs":{"context":{"active_test":false}}}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0]["__count"] != float64(1) || rows[0]["active"] != false || rows[0]["name"] != "Gamma" {
		t.Fatalf("eager rows = %#v", rows)
	}
	body = postCallKW(t, handler, `{"model":"res.partner","method":"read_group","args":[[],["__count"],["active"],0,null,"active desc"],"kwargs":{"context":{"active_test":false}}}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["active"] != true || rows[0]["active_count"] != float64(2) {
		t.Fatalf("ordered rows = %#v", rows)
	}
}

func TestWebDatasetReadGroupAggregatesAndMany2OneLabels(t *testing.T) {
	reg := record.NewRegistry()
	company := model.New("res.company", "res_company")
	company.AddField(field.New("name", field.Char))
	if err := reg.Register(company); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.http.group.aggregate", "x_http_group_aggregate")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	sample.AddField(field.New("score", field.Int).WithAggregator("avg"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	alphaID, err := server.Env.Model("res.company").Create(map[string]any{"name": "Alpha Corp"})
	if err != nil {
		t.Fatal(err)
	}
	betaID, err := server.Env.Model("res.company").Create(map[string]any{"name": "Beta Corp"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "a", "company_id": alphaID, "amount": 10.5, "score": int64(2)},
		{"name": "b", "company_id": alphaID, "amount": 4.5, "score": int64(4)},
		{"name": "c", "company_id": betaID, "amount": 7.0, "score": int64(8)},
	} {
		if _, err := server.Env.Model("x.http.group.aggregate").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	body := postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"read_group","args":[[],["amount","score:avg","score_total:sum(score)"],["company_id"]]}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("read_group aggregate rows = %#v", rows)
	}
	companyValue, ok := rows[0]["company_id"].([]any)
	if !ok || len(companyValue) != 2 || companyValue[0] != float64(alphaID) || companyValue[1] != "Alpha Corp" {
		t.Fatalf("company group = %#v", rows[0]["company_id"])
	}
	if rows[0]["company_id_count"] != float64(2) || rows[0]["amount"] != 15.0 || rows[0]["score"] != 3.0 || rows[0]["score_total"] != float64(6) {
		t.Fatalf("aggregate row = %#v", rows[0])
	}
	domainValue := rows[0]["__domain"].([]any)
	condition := domainValue[0].([]any)
	if condition[0] != "company_id" || condition[1] != "=" || condition[2] != float64(alphaID) {
		t.Fatalf("company domain = %#v", domainValue)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"read_group","args":[[],["amount","score:avg","score_total:sum(score)"],["company_id"],0,null,"score desc"]}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	companyValue, ok = rows[0]["company_id"].([]any)
	if !ok || companyValue[0] != float64(betaID) || rows[0]["score"] != 8.0 {
		t.Fatalf("ordered read_group rows = %#v", rows)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"formatted_read_group","args":[[],["company_id"],["amount:sum","score:avg"]]}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["amount:sum"] != 15.0 || rows[0]["score:avg"] != 3.0 || rows[0]["__count"] != float64(2) {
		t.Fatalf("formatted aggregate rows = %#v", rows)
	}
	formattedCompany, ok := rows[0]["company_id"].([]any)
	if !ok || formattedCompany[0] != float64(alphaID) || formattedCompany[1] != "Alpha Corp" {
		t.Fatalf("formatted company = %#v", rows[0]["company_id"])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"formatted_read_group","args":[[],["company_id"],["amount:sum","score:avg"]],"kwargs":{"order":"amount:sum asc"}}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	formattedCompany, ok = rows[0]["company_id"].([]any)
	if !ok || formattedCompany[0] != float64(betaID) || rows[0]["amount:sum"] != 7.0 {
		t.Fatalf("ordered formatted rows = %#v", rows)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"formatted_read_group","args":[[],["company_id"],["amount:sum","score:avg"],[],0,null,"amount:sum asc"]}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	formattedCompany, ok = rows[0]["company_id"].([]any)
	if !ok || formattedCompany[0] != float64(betaID) || rows[0]["amount:sum"] != 7.0 {
		t.Fatalf("positionally ordered formatted rows = %#v", rows)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"web_read_group","kwargs":{"domain":[],"groupby":["company_id"],"aggregates":["amount:sum","score:avg"]}}`)
	var grouped map[string]any
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok := grouped["groups"].([]any)
	if !ok || len(groups) != 2 || grouped["length"] != float64(2) {
		t.Fatalf("web_read_group aggregate result = %#v", grouped)
	}
	first, ok := groups[0].(map[string]any)
	if !ok || first["amount:sum"] != 15.0 || first["score:avg"] != 3.0 {
		t.Fatalf("web_read_group aggregate row = %#v", groups[0])
	}
	webCompany, ok := first["company_id"].([]any)
	if !ok || webCompany[0] != float64(alphaID) || webCompany[1] != "Alpha Corp" {
		t.Fatalf("web_read_group company = %#v", first["company_id"])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"web_read_group","kwargs":{"domain":[],"groupby":["company_id"],"aggregates":["amount:sum","score:avg"],"order":"score:avg desc"}}`)
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok = grouped["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("ordered web_read_group result = %#v", grouped)
	}
	first, ok = groups[0].(map[string]any)
	if !ok || first["score:avg"] != 8.0 {
		t.Fatalf("ordered web_read_group first row = %#v", groups[0])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.aggregate","method":"web_read_group","kwargs":{"domain":[],"groupby":["company_id"],"aggregates":["amount:sum","score:avg"],"orderby":"score:avg desc"}}`)
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok = grouped["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("orderby web_read_group result = %#v", grouped)
	}
	first, ok = groups[0].(map[string]any)
	if !ok || first["score:avg"] != 8.0 {
		t.Fatalf("orderby web_read_group first row = %#v", groups[0])
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(`{"model":"x.http.group.aggregate","method":"formatted_read_group","kwargs":{"domain":[],"groupby":["company_id"],"aggregates":["amount:sum"],"order":"missing desc"}}`)))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "order term") {
		t.Fatalf("invalid formatted_read_group order response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebDatasetReadGroupRecordsetAggregate(t *testing.T) {
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	tag := model.New("x.http.group.recordset.tag", "x_http_group_recordset_tag")
	tag.AddField(field.New("name", field.Char))
	if err := reg.Register(tag); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.http.group.recordset", "x_http_group_recordset")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("category", field.Char))
	sample.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	sample.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.http.group.recordset.tag"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	partnerA, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := server.Env.Model("res.partner").Create(map[string]any{"name": "Babbage"})
	if err != nil {
		t.Fatal(err)
	}
	tagA, err := server.Env.Model("x.http.group.recordset.tag").Create(map[string]any{"name": "Tag A"})
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := server.Env.Model("x.http.group.recordset.tag").Create(map[string]any{"name": "Tag B"})
	if err != nil {
		t.Fatal(err)
	}
	records := server.Env.Model("x.http.group.recordset")
	rowA, err := records.Create(map[string]any{"name": "a", "category": "alpha", "partner_id": partnerA, "tag_ids": []int64{tagA, tagB}})
	if err != nil {
		t.Fatal(err)
	}
	rowB, err := records.Create(map[string]any{"name": "b", "category": "alpha", "partner_id": partnerA, "tag_ids": []int64{tagB}})
	if err != nil {
		t.Fatal(err)
	}
	rowC, err := records.Create(map[string]any{"name": "c", "category": "alpha", "partner_id": partnerB, "tag_ids": []int64{tagA}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(map[string]any{"name": "d", "category": "beta"}); err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, handler, `{"model":"x.http.group.recordset","method":"read_group","args":[[],["ids:recordset(id)","partner_records:recordset(partner_id)","tag_records:recordset(tag_ids)"],["category"]]}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	alpha := httpReadGroupFindScalar(rows, "category", "alpha")
	if len(alpha) == 0 {
		t.Fatalf("read_group rows = %#v", rows)
	}
	if !httpReadGroupIDsEqual(alpha["ids"], rowA, rowB, rowC) {
		t.Fatalf("ids = %#v", alpha["ids"])
	}
	if !httpReadGroupIDsEqual(alpha["partner_records"], partnerA, partnerB) {
		t.Fatalf("partner_records = %#v", alpha["partner_records"])
	}
	if !httpReadGroupIDsEqual(alpha["tag_records"], tagA, tagB) {
		t.Fatalf("tag_records = %#v", alpha["tag_records"])
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.recordset","method":"formatted_read_group","args":[[],["category"],["id:recordset"]]}`)
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	alpha = httpReadGroupFindScalar(rows, "category", "alpha")
	if len(alpha) == 0 {
		t.Fatalf("formatted_read_group rows = %#v", rows)
	}
	if _, ok := alpha["id:recordset"]; ok {
		t.Fatalf("formatted_read_group kept recordset key = %#v", alpha)
	}
	if !httpReadGroupIDsEqual(alpha["id:array_agg"], rowA, rowB, rowC) {
		t.Fatalf("formatted id:array_agg = %#v", alpha["id:array_agg"])
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.recordset","method":"web_read_group","kwargs":{"domain":[],"groupby":["category"],"aggregates":["id:recordset"]}}`)
	var grouped map[string]any
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok := grouped["groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("web_read_group result = %#v", grouped)
	}
	first, ok := groups[0].(map[string]any)
	if !ok || !httpReadGroupIDsEqual(first["id:array_agg"], rowA, rowB, rowC) {
		t.Fatalf("web_read_group first row = %#v", groups[0])
	}
}

func TestWebDatasetReadGroupStoredMany2Many(t *testing.T) {
	reg := record.NewRegistry()
	tag := model.New("x.http.group.tag", "x_http_group_tag")
	tag.AddField(field.New("name", field.Char))
	if err := reg.Register(tag); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.http.group.m2m", "x_http_group_m2m")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("tag_ids", field.Many2Many).WithRelation("x.http.group.tag"))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	tagA, err := server.Env.Model("x.http.group.tag").Create(map[string]any{"name": "Tag A"})
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := server.Env.Model("x.http.group.tag").Create(map[string]any{"name": "Tag B"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "ab", "tag_ids": []int64{tagA, tagB}, "amount": 10.0},
		{"name": "b", "tag_ids": []int64{tagB}, "amount": 5.0},
		{"name": "empty", "tag_ids": []int64{}, "amount": 1.0},
		{"name": "missing", "amount": 2.0},
	} {
		if _, err := server.Env.Model("x.http.group.m2m").Create(values); err != nil {
			t.Fatal(err)
		}
	}

	body := postCallKW(t, handler, `{"model":"x.http.group.m2m","method":"read_group","args":[[],["amount"],["tag_ids"]]}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	tagBRow := httpReadGroupFindPair(rows, "tag_ids", tagB)
	falseRow := httpReadGroupFindScalar(rows, "tag_ids", false)
	if int64Value(tagBRow["tag_ids_count"]) != 2 || tagBRow["amount"] != 15.0 {
		t.Fatalf("legacy tag B row = %#v rows=%#v", tagBRow, rows)
	}
	if int64Value(falseRow["tag_ids_count"]) != 2 || falseRow["amount"] != 3.0 {
		t.Fatalf("legacy false row = %#v rows=%#v", falseRow, rows)
	}
	node, err := domain.Parse(falseRow["__domain"])
	if err != nil {
		t.Fatal(err)
	}
	found, err := server.Env.Model("x.http.group.m2m").Search(node)
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 2 {
		t.Fatalf("legacy false domain count = %d domain=%#v", found.Len(), falseRow["__domain"])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.m2m","method":"read_group","args":[[["name","ilike","b"]],["amount"],["tag_ids"]]}`)
	rows = nil
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	filteredTagB := httpReadGroupFindPair(rows, "tag_ids", tagB)
	if int64Value(filteredTagB["tag_ids_count"]) != 2 || filteredTagB["amount"] != 15.0 {
		t.Fatalf("filtered tag B row = %#v rows=%#v", filteredTagB, rows)
	}
	filteredDomain, err := json.Marshal(filteredTagB["__domain"])
	if err != nil {
		t.Fatal(err)
	}
	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"x.http.group.m2m","method":"search_count","args":[%s]}`, filteredDomain))
	var filteredCount any
	if err := json.Unmarshal([]byte(body), &filteredCount); err != nil {
		t.Fatal(err)
	}
	if int64Value(filteredCount) != 2 {
		t.Fatalf("filtered domain search_count = %#v domain=%s", filteredCount, filteredDomain)
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.m2m","method":"formatted_read_group","args":[[],["tag_ids"],["amount:sum"]]}`)
	rows = nil
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	formattedTagB := httpReadGroupFindPair(rows, "tag_ids", tagB)
	formattedFalse := httpReadGroupFindScalar(rows, "tag_ids", false)
	if int64Value(formattedTagB["__count"]) != 2 || formattedTagB["amount:sum"] != 15.0 {
		t.Fatalf("formatted tag B row = %#v rows=%#v", formattedTagB, rows)
	}
	if int64Value(formattedFalse["__count"]) != 2 || formattedFalse["amount:sum"] != 3.0 {
		t.Fatalf("formatted false row = %#v rows=%#v", formattedFalse, rows)
	}
	if _, leaksDomain := formattedTagB["__domain"]; leaksDomain {
		t.Fatalf("formatted row leaked __domain = %#v", formattedTagB)
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.m2m","method":"web_read_group","kwargs":{"domain":[],"groupby":["tag_ids"],"aggregates":["amount:sum"]}}`)
	var grouped map[string]any
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok := grouped["groups"].([]any)
	if !ok || grouped["length"] != float64(3) {
		t.Fatalf("web m2m result = %#v", grouped)
	}
	webRows := make([]map[string]any, 0, len(groups))
	for _, item := range groups {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("web m2m row = %#v", item)
		}
		webRows = append(webRows, row)
	}
	webTagA := httpReadGroupFindPair(webRows, "tag_ids", tagA)
	webFalse := httpReadGroupFindScalar(webRows, "tag_ids", false)
	if int64Value(webTagA["__count"]) != 1 || webTagA["amount:sum"] != 10.0 || int64Value(webFalse["__count"]) != 2 {
		t.Fatalf("web rows = %#v", webRows)
	}
}

func TestWebDatasetReadGroupSumCurrencyAggregate(t *testing.T) {
	reg := record.NewRegistry()
	currency := model.New("res.currency", "res_currency")
	currency.AddField(field.New("name", field.Char))
	if err := reg.Register(currency); err != nil {
		t.Fatal(err)
	}
	rate := model.New("res.currency.rate", "res_currency_rate")
	rate.AddField(field.New("name", field.Date))
	rate.AddField(field.New("currency_id", field.Many2One).WithRelation("res.currency"))
	rate.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	rate.AddField(field.New("rate", field.Float))
	if err := reg.Register(rate); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.http.group.currency", "x_http_group_currency")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("category", field.Char))
	sample.AddField(field.New("currency_id", field.Many2One).WithRelation("res.currency"))
	sample.AddField(field.New("amount", field.Monetary).WithCurrencyField("currency_id").WithAggregator("sum"))
	sample.AddField(field.New("plain_amount", field.Float))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	bhdID, err := server.Env.Model("res.currency").Create(map[string]any{"name": "BHD"})
	if err != nil {
		t.Fatal(err)
	}
	usdID, err := server.Env.Model("res.currency").Create(map[string]any{"name": "USD"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"name": "2020-01-01", "currency_id": bhdID, "rate": 1.0},
		{"name": "2020-01-01", "currency_id": usdID, "rate": 0.5},
	} {
		if _, err := server.Env.Model("res.currency.rate").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	for _, values := range []map[string]any{
		{"name": "a", "category": "sales", "currency_id": bhdID, "amount": 100.0, "plain_amount": 100.0},
		{"name": "b", "category": "sales", "currency_id": usdID, "amount": 50.0, "plain_amount": 50.0},
		{"name": "c", "category": "cost", "currency_id": usdID, "amount": 20.0, "plain_amount": 20.0},
	} {
		if _, err := server.Env.Model("x.http.group.currency").Create(values); err != nil {
			t.Fatal(err)
		}
	}

	body := postCallKW(t, handler, `{"model":"x.http.group.currency","method":"read_group","args":[[],["amount_company:sum_currency(amount)"],["category"]]}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	sales := httpReadGroupFindScalar(rows, "category", "sales")
	cost := httpReadGroupFindScalar(rows, "category", "cost")
	if sales["amount_company"] != 200.0 || cost["amount_company"] != 40.0 {
		t.Fatalf("http sum_currency rows = %#v", rows)
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.currency","method":"formatted_read_group","args":[[],["category"],["amount:sum_currency"]]}`)
	rows = nil
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	formattedSales := httpReadGroupFindScalar(rows, "category", "sales")
	if formattedSales["amount:sum_currency"] != 200.0 || int64Value(formattedSales["__count"]) != 2 {
		t.Fatalf("http formatted sum_currency rows = %#v", rows)
	}

	body = postCallKW(t, handler, `{"model":"x.http.group.currency","method":"web_read_group","kwargs":{"domain":[],"groupby":["category"],"aggregates":["amount:sum_currency"]}}`)
	var grouped map[string]any
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok := grouped["groups"].([]any)
	if !ok || grouped["length"] != float64(2) {
		t.Fatalf("http web sum_currency result = %#v", grouped)
	}
	if _, ok := groups[0].(map[string]any)["amount:sum_currency"]; !ok {
		t.Fatalf("http web sum_currency groups = %#v", groups)
	}
}

func TestWebDatasetReadGroupDateIntervals(t *testing.T) {
	reg := record.NewRegistry()
	lang := model.New("res.lang", "res_lang")
	lang.AddField(field.New("code", field.Char))
	lang.AddField(field.New("week_start", field.Char))
	if err := reg.Register(lang); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.http.group.date", "x_http_group_date")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	if _, err := server.Env.Model("res.lang").Create(map[string]any{"code": "en_US", "week_start": "7"}); err != nil {
		t.Fatal(err)
	}
	for _, values := range []string{
		`{"name":"a","date_value":"2026-01-10","moment_value":"2026-01-05 08:00:00"}`,
		`{"name":"b","date_value":"2026-01-20","moment_value":"2026-01-07 09:00:00"}`,
		`{"name":"c","date_value":"2026-02-05","moment_value":"2026-01-12 10:00:00"}`,
		`{"name":"d","date_value":"2022-01-29","moment_value":"2022-01-29 08:00:00"}`,
		`{"name":"e","date_value":"2022-01-30","moment_value":"2022-01-30 08:00:00"}`,
		`{"name":"f","date_value":"2022-01-31","moment_value":"2022-01-31 08:00:00"}`,
	} {
		postCallKW(t, handler, fmt.Sprintf(`{"model":"x.http.group.date","method":"create","values":%s}`, values))
	}
	body := postCallKW(t, handler, `{"model":"x.http.group.date","method":"read_group","args":[[["name","in",["a","b","c"]]],["__count"],["date_value:month"]]}`)
	var months []map[string]any
	if err := json.Unmarshal([]byte(body), &months); err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 || months[0]["date_value:month"] != "January 2026" || months[0]["date_value_count"] != float64(2) || months[1]["date_value:month"] != "February 2026" {
		t.Fatalf("month rows = %#v", months)
	}
	monthDomain, ok := months[0]["__domain"].([]any)
	if !ok || len(monthDomain) != 3 {
		t.Fatalf("month domain = %#v", months[0]["__domain"])
	}
	monthStart, ok := monthDomain[1].([]any)
	if !ok || len(monthStart) != 3 || monthStart[0] != "date_value" || monthStart[1] != ">=" || monthStart[2] != "2026-01-01" {
		t.Fatalf("month start = %#v", monthDomain[1])
	}
	monthEnd, ok := monthDomain[2].([]any)
	if !ok || len(monthEnd) != 3 || monthEnd[0] != "date_value" || monthEnd[1] != "<" || monthEnd[2] != "2026-02-01" {
		t.Fatalf("month end = %#v", monthDomain[2])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"read_group","args":[[["name","in",["a","b","c"]]],["__count"],["moment_value:week"]]}`)
	var weeks []map[string]any
	if err := json.Unmarshal([]byte(body), &weeks); err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 2 || weeks[0]["moment_value:week"] != "W2 2026" || weeks[0]["moment_value_count"] != float64(2) || weeks[1]["moment_value:week"] != "W3 2026" {
		t.Fatalf("week rows = %#v", weeks)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"read_group","args":[[["name","in",["d","e","f"]]],["__count"],["date_value:week"]],"kwargs":{"context":{"lang":"en_US"}}}`)
	if err := json.Unmarshal([]byte(body), &weeks); err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 2 || weeks[0]["date_value:week"] != "W5 2022" || weeks[0]["date_value_count"] != float64(1) || weeks[1]["date_value:week"] != "W6 2022" || weeks[1]["date_value_count"] != float64(2) {
		t.Fatalf("context week rows = %#v", weeks)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"formatted_read_group","args":[[["name","in",["a","b","c"]]],["date_value:month"],["__count"]]}`)
	var formatted []map[string]any
	if err := json.Unmarshal([]byte(body), &formatted); err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 2 {
		t.Fatalf("formatted rows = %#v", formatted)
	}
	formattedMonth, ok := formatted[0]["date_value:month"].([]any)
	if !ok || len(formattedMonth) != 2 || formattedMonth[0] != "2026-01-01" || formattedMonth[1] != "January 2026" {
		t.Fatalf("formatted month = %#v", formatted[0]["date_value:month"])
	}
	if _, ok := formatted[0]["__domain"]; ok {
		t.Fatalf("formatted row leaked __domain = %#v", formatted[0])
	}
	extraDomain, ok := formatted[0]["__extra_domain"].([]any)
	if !ok || len(extraDomain) != 3 || extraDomain[0] != "&" {
		t.Fatalf("formatted extra domain = %#v", formatted[0]["__extra_domain"])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"read_group","args":[[["name","in",["a","b","c"]]],["__count"],["date_value:month_number"]]}`)
	var numericMonths []map[string]any
	if err := json.Unmarshal([]byte(body), &numericMonths); err != nil {
		t.Fatal(err)
	}
	if len(numericMonths) != 2 || numericMonths[0]["date_value:month_number"] != float64(1) || numericMonths[0]["date_value_count"] != float64(2) || numericMonths[1]["date_value:month_number"] != float64(2) {
		t.Fatalf("numeric month rows = %#v", numericMonths)
	}
	if _, hasRange := numericMonths[0]["__range"]; hasRange {
		t.Fatalf("numeric month row emitted range = %#v", numericMonths[0])
	}
	numericMonthDomain, ok := numericMonths[0]["__domain"].([]any)
	if !ok || len(numericMonthDomain) != 2 {
		t.Fatalf("numeric month domain = %#v", numericMonths[0]["__domain"])
	}
	numericMonthCondition, ok := numericMonthDomain[1].([]any)
	if !ok || len(numericMonthCondition) != 3 || numericMonthCondition[0] != "date_value.month_number" || numericMonthCondition[1] != "=" || numericMonthCondition[2] != float64(1) {
		t.Fatalf("numeric month condition = %#v", numericMonthDomain)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"formatted_read_group","args":[[["name","in",["a","b","c"]]],["date_value:month_number"],["__count"]]}`)
	var formattedNumeric []map[string]any
	if err := json.Unmarshal([]byte(body), &formattedNumeric); err != nil {
		t.Fatal(err)
	}
	if len(formattedNumeric) != 2 || formattedNumeric[0]["date_value:month_number"] != float64(1) || formattedNumeric[0]["__count"] != float64(2) {
		t.Fatalf("formatted numeric rows = %#v", formattedNumeric)
	}
	if _, hasRange := formattedNumeric[0]["__range"]; hasRange {
		t.Fatalf("formatted numeric row emitted range = %#v", formattedNumeric[0])
	}
	formattedNumericDomain, ok := formattedNumeric[0]["__extra_domain"].([]any)
	if !ok || len(formattedNumericDomain) != 1 {
		t.Fatalf("formatted numeric domain = %#v", formattedNumeric[0]["__extra_domain"])
	}
	formattedNumericCondition, ok := formattedNumericDomain[0].([]any)
	if !ok || len(formattedNumericCondition) != 3 || formattedNumericCondition[0] != "date_value.month_number" || formattedNumericCondition[1] != "=" || formattedNumericCondition[2] != float64(1) {
		t.Fatalf("formatted numeric condition = %#v", formattedNumericDomain)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"search_count","args":[[["date_value.month_number","=",1]]]}`)
	var numericSearchCount float64
	if err := json.Unmarshal([]byte(body), &numericSearchCount); err != nil {
		t.Fatal(err)
	}
	if numericSearchCount != 5 {
		t.Fatalf("numeric month search_count = %v", numericSearchCount)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"web_read_group","kwargs":{"domain":[["name","in",["a","b","c"]]],"groupby":["date_value:month"],"aggregates":["__count"]}}`)
	var webGrouped map[string]any
	if err := json.Unmarshal([]byte(body), &webGrouped); err != nil {
		t.Fatal(err)
	}
	webRows, ok := webGrouped["groups"].([]any)
	if !ok || len(webRows) != 2 || webGrouped["length"] != float64(2) {
		t.Fatalf("web_read_group result = %#v", webGrouped)
	}
	firstWebRow, ok := webRows[0].(map[string]any)
	if !ok {
		t.Fatalf("web_read_group first row = %#v", webRows[0])
	}
	firstWebValue, ok := firstWebRow["date_value:month"].([]any)
	if !ok || len(firstWebValue) != 2 || firstWebValue[1] != "January 2026" {
		t.Fatalf("web_read_group first value = %#v", firstWebRow["date_value:month"])
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.date","method":"web_read_group","kwargs":{"domain":[["name","in",["a","b","c"]]],"groupby":["date_value:month_number"],"aggregates":["__count"]}}`)
	if err := json.Unmarshal([]byte(body), &webGrouped); err != nil {
		t.Fatal(err)
	}
	webRows, ok = webGrouped["groups"].([]any)
	if !ok || len(webRows) != 2 || webGrouped["length"] != float64(2) {
		t.Fatalf("numeric web_read_group result = %#v", webGrouped)
	}
	firstWebRow, ok = webRows[0].(map[string]any)
	if !ok || firstWebRow["date_value:month_number"] != float64(1) || firstWebRow["__count"] != float64(2) {
		t.Fatalf("numeric web_read_group first row = %#v", webRows[0])
	}
	firstWebDomain, ok := firstWebRow["__extra_domain"].([]any)
	if !ok || len(firstWebDomain) != 1 {
		t.Fatalf("numeric web_read_group domain = %#v", firstWebRow["__extra_domain"])
	}
	firstWebCondition, ok := firstWebDomain[0].([]any)
	if !ok || firstWebCondition[0] != "date_value.month_number" || firstWebCondition[1] != "=" || firstWebCondition[2] != float64(1) {
		t.Fatalf("numeric web_read_group condition = %#v", firstWebDomain)
	}
}

func TestWebDatasetReadGroupDateTimeUsesContextTimezone(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.http.group.tz", "x_http_group_tz")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("moment_value", field.DateTime))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	server := Server{Env: record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})}
	handler := server.Handler()
	for _, values := range []string{
		`{"name":"a","moment_value":"2026-01-01 20:30:00"}`,
		`{"name":"b","moment_value":"2026-01-01 22:30:00"}`,
	} {
		postCallKW(t, handler, fmt.Sprintf(`{"model":"x.http.group.tz","method":"create","values":%s}`, values))
	}
	body := postCallKW(t, handler, `{"model":"x.http.group.tz","method":"read_group","args":[[],["__count"],["moment_value:day"]],"kwargs":{"context":{"tz":"Asia/Bahrain"}}}`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["moment_value:day"] != "01 Jan 2026" || rows[0]["moment_value_count"] != float64(1) || rows[1]["moment_value:day"] != "02 Jan 2026" || rows[1]["moment_value_count"] != float64(1) {
		t.Fatalf("timezone rows = %#v", rows)
	}
	secondDomain := rows[1]["__domain"].([]any)
	secondStart := secondDomain[0].([]any)
	secondEnd := secondDomain[1].([]any)
	if secondStart[0] != "moment_value" || secondStart[1] != ">=" || secondStart[2] != "2026-01-01 21:00:00" || secondEnd[0] != "moment_value" || secondEnd[1] != "<" || secondEnd[2] != "2026-01-02 21:00:00" {
		t.Fatalf("timezone second domain = %#v", secondDomain)
	}
	body = postCallKW(t, handler, `{"model":"x.http.group.tz","method":"formatted_read_group","args":[[],["moment_value:day"],["__count"]],"kwargs":{"context":{"tz":"Asia/Bahrain"}}}`)
	var formatted []map[string]any
	if err := json.Unmarshal([]byte(body), &formatted); err != nil {
		t.Fatal(err)
	}
	if len(formatted) != 2 {
		t.Fatalf("formatted timezone rows = %#v", formatted)
	}
	secondValue, ok := formatted[1]["moment_value:day"].([]any)
	if !ok || len(secondValue) != 2 || secondValue[0] != "2026-01-01 21:00:00" || secondValue[1] != "02 Jan 2026" {
		t.Fatalf("formatted timezone value = %#v", formatted[1]["moment_value:day"])
	}
	extraDomain := formatted[1]["__extra_domain"].([]any)
	extraStart := extraDomain[1].([]any)
	if extraStart[2] != "2026-01-01 21:00:00" {
		t.Fatalf("formatted timezone extra domain = %#v", extraDomain)
	}
}

func TestWebAliasesAndAssets(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "<title>Odoo</title>") || !strings.Contains(body, `data-view="apps"`) || !strings.Contains(body, "o-app-launcher-view") || !strings.Contains(body, "/web/dataset/call_kw") {
		t.Fatalf("web client response %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(body, "<h1>Gorp</h1>") || strings.Count(body, "function renderRows") != 1 || strings.Count(body, "function loadRuntime") != 1 {
		t.Fatalf("web client shell mismatch")
	}
	for _, needle := range []string{
		`<label class="theme-field" hidden>`,
		`class="field technical-field" hidden`,
		`id="loadRows" class="secondary o-debug-only" hidden`,
		`class="o_web_client"`,
		`class="o_main_navbar"`,
		`class="o_action_manager"`,
		`o_control_panel`,
		`o_app_launcher`,
		`button.className = "app-card o_app has-icon"`,
		`recordPanel.className = "panel record-panel o_form_view"`,
		`class="record-grid o-form-sheet o_form_sheet"`,
		`table.className = "o_list_renderer o_list_table"`,
		`tr.className = "o_data_row"`,
		`id="recordSearch"`,
		`applyTheme(requestedTheme || storedTheme || document.body.dataset.theme)`,
		`id="navApps" class="o-launcher-button" data-view="apps" aria-label="Apps"`,
		`id="topMenu" aria-label="Application"`,
		`appendAppCard("Apps", "A", () => {`,
		`async function loadActionViews(action, model)`,
		`callKW(model, "get_views"`,
		`callKW(model, "web_search_read"`,
		`callKW(model, "web_read"`,
		`callKW(workbench.openedRecord.model, "web_save"`,
		`domain: combinedDomain(actionDomain(workbench.action), document.getElementById("recordSearch").value)`,
		`context: readContext(workbench.action)`,
		`count_limit: 10001`,
		`action._web_context`,
		`action._web_domain`,
		`if (key.startsWith("search_default_")) delete context[key];`,
		`out.push([actionSearchViewID(action), "search"])`,
		`workbench.action = null;`,
		`showRecordForm(false)`,
		`viewArchFields(listView.arch)`,
		`function visibleFormFields(fields)`,
		`if (field === "id" || field.startsWith("__")) continue;`,
		`for (const field of formFields)`,
		`th.textContent = fieldLabel(field);`,
		`return label && label !== field ? label : humanFieldLabel(field);`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("web client missing %q", needle)
		}
	}
	for _, needle := range []string{"Create Demo Partner", "Demo Partner", "Backend connected.", "scrollIntoView", "Developer RPC", "Build dashboard", "linear-gradient", "bokeh", `id="navDeveloper"`, `>Install Apps</button>`, `<span class="technical"></span>`} {
		if strings.Contains(body, needle) {
			t.Fatalf("web client still exposes shell cue %q", needle)
		}
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "o-launcher-button") || strings.Contains(rec.Body.String(), "Developer RPC") {
		t.Fatalf("web client slash response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("web shell method status %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/webclient/load_menus", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Settings") {
		t.Fatalf("load_menus response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/bundle/web.assets_backend", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "webclient.js") {
		t.Fatalf("bundle response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/assets/manifest?bundle=web.assets_backend&debug=assets", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"/web/assets/debug/web.assets_backend/webclient.js`) || !strings.Contains(rec.Body.String(), `"checksum"`) {
		t.Fatalf("debug manifest response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/binary/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("binary status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAssetDebugFileServesBundleMember(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "addons", "web", "static", "src", "js", "webclient.js")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("console.log('webclient');"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := assets.NewRegistryWithResolver(assets.NewFilesystemResolver(root).WithInstalledAddons(map[string]bool{"web": true}))
	if err := reg.Apply(assets.Backend, assets.Operation{Kind: assets.Append, Path: "web/static/src/js/webclient.js"}); err != nil {
		t.Fatal(err)
	}
	handler := (Server{Assets: reg}).Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/assets/debug/web.assets_backend/web/static/src/js/webclient.js?v=test", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('webclient');" || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("debug asset response %d %q headers=%v", rec.Code, rec.Body.String(), rec.Header())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/web/assets/debug/web.assets_backend/web/static/src/js/webclient.js", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("debug asset head response %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/assets/debug/web.assets_backend/web/static/src/js/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing debug asset status %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/assets/debug/web.assets_backend/web/static/src/js/webclient.js", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("debug asset method status %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestSessionInfoOdooShape(t *testing.T) {
	server := testMailThreadServer(t)
	env := server.Env.WithContext(record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	currencyID, err := env.Model("res.currency").Create(map[string]any{"name": "BHD", "symbol": "BD", "position": "after", "rounding": 0.001, "decimal_places": int64(3), "active": true})
	if err != nil {
		t.Fatal(err)
	}
	parentCompanyID, err := env.Model("res.company").Create(map[string]any{"name": "Parent Co", "currency_id": currencyID})
	if err != nil {
		t.Fatal(err)
	}
	childCompanyID, err := env.Model("res.company").Create(map[string]any{"name": "Child Co", "parent_id": parentCompanyID, "currency_id": currencyID})
	if err != nil {
		t.Fatal(err)
	}
	groupUserID, err := env.Model("res.groups").Create(map[string]any{"name": "Internal User"})
	if err != nil {
		t.Fatal(err)
	}
	groupExportID, err := env.Model("res.groups").Create(map[string]any{"name": "Allow Export"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"module": "base", "name": "group_user", "complete_name": "base.group_user", "model": "res.groups", "res_id": groupUserID},
		{"module": "base", "name": "group_allow_export", "complete_name": "base.group_allow_export", "model": "res.groups", "res_id": groupExportID},
	} {
		if _, err := env.Model("ir.model.data").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.expiration_date", "value": "2099-12-31"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.expiration_reason", "value": "trial"}); err != nil {
		t.Fatal(err)
	}
	server.Env = env.WithContext(record.Context{
		UserID:     7,
		CompanyID:  childCompanyID,
		CompanyIDs: []int64{parentCompanyID, childCompanyID},
		Values: map[string]any{
			"db":        "demo",
			"lang":      "ar_001",
			"tz":        "Asia/Bahrain",
			"group_ids": []int64{groupUserID, groupExportID},
		},
	})
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/get_session_info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["uid"] != float64(7) || payload["db"] != "demo" || payload["registry_hash"] == "" || payload["server_version"] != "19.0" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["is_public"] != false || payload["is_internal_user"] != true || payload["support_url"] != "https://www.odoo.com/help" {
		t.Fatalf("session flags = %#v", payload)
	}
	context := payload["user_context"].(map[string]any)
	if context["lang"] != "ar_001" || context["tz"] != "Asia/Bahrain" {
		t.Fatalf("context = %#v", context)
	}
	bundleParams := payload["bundle_params"].(map[string]any)
	if bundleParams["lang"] != "ar_001" {
		t.Fatalf("bundle params = %#v", bundleParams)
	}
	companies := payload["user_companies"].(map[string]any)
	if int64Value(companies["current_company"]) != childCompanyID || !payload["display_switch_company_menu"].(bool) {
		t.Fatalf("companies = %#v", companies)
	}
	allowed := companies["allowed_companies"].(map[string]any)
	parent := allowed[strconv.FormatInt(parentCompanyID, 10)].(map[string]any)
	child := allowed[strconv.FormatInt(childCompanyID, 10)].(map[string]any)
	if parent["name"] != "Parent Co" || child["parent_id"] != float64(parentCompanyID) || int64Value(child["currency_id"]) != currencyID {
		t.Fatalf("allowed companies = %#v", allowed)
	}
	if !testContainsRecordID(parent["child_ids"], childCompanyID) {
		t.Fatalf("parent child ids = %#v", parent)
	}
	currencies := payload["currencies"].(map[string]any)
	currency := currencies[strconv.FormatInt(currencyID, 10)].(map[string]any)
	if currency["symbol"] != "BD" || int64Value(currency["decimal_places"]) != 3 {
		t.Fatalf("currencies = %#v", currencies)
	}
	groups := payload["groups"].(map[string]any)
	if groups[strconv.FormatInt(groupUserID, 10)] == nil || groups["base.group_allow_export"] != true || groups["base.group_user"] != true {
		t.Fatalf("groups = %#v", groups)
	}
	viewInfo := payload["view_info"].(map[string]any)
	if viewInfo["form"].(map[string]any)["multi_record"] != false || viewInfo["list"].(map[string]any)["icon"] != "oi oi-view-list" {
		t.Fatalf("view info = %#v", viewInfo)
	}
	userSettings := payload["user_settings"].(map[string]any)
	if userSettings["embedded_actions_config_ids"] == nil || userSettings["color_scheme"] != "system" {
		t.Fatalf("user settings = %#v", userSettings)
	}
	if payload["warning"] != "user" || payload["expiration_date"] != "2099-12-31" || payload["expiration_reason"] != "trial" {
		t.Fatalf("enterprise warning = %#v", payload)
	}
}

func TestSessionInfoWorkflowDisableEditFlag(t *testing.T) {
	server := testSequenceServer(t, 1)
	if _, err := server.Env.Model("ir.config_parameter").Create(map[string]any{"key": "disable_edit_on_non_approval", "value": "True"}); err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: 7, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{1}}})
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/get_session_info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["disable_edit_on_non_approval"] != true {
		t.Fatalf("disable_edit_on_non_approval = %#v", payload["disable_edit_on_non_approval"])
	}

	server.Env = server.Env.WithContext(record.Context{UserID: 0, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{}})
	handler = server.Handler()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/session/get_session_info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if _, ok := payload["disable_edit_on_non_approval"]; ok {
		t.Fatalf("portal payload exposed disable_edit_on_non_approval = %#v", payload)
	}
}

func TestWebSessionAuthenticateIssuesCookieAndRevokesLogout(t *testing.T) {
	server := testServer(t)
	engine := security.NewEngine()
	engine.Users[9] = security.User{
		ID:         9,
		Login:      "demo",
		Password:   "secret",
		Active:     true,
		CompanyID:  2,
		CompanyIDs: []int64{2, 3},
		PartnerID:  44,
		GroupIDs:   []int64{10},
	}
	server.Security = engine
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":21,"params":{"db":"demo","login":"demo","password":"secret"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/session/authenticate", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticate response %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "session_id" || cookies[0].Value == "" || !cookies[0].HttpOnly {
		t.Fatalf("session cookie = %+v", cookies)
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	if result["uid"] != float64(9) {
		t.Fatalf("authenticate result = %#v", result)
	}
	context := result["user_context"].(map[string]any)
	if context["uid"] != float64(9) {
		t.Fatalf("context = %#v", context)
	}

	checkReq := httptest.NewRequest(http.MethodGet, "/web/session/check", nil)
	checkReq.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, checkReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"uid":9`) {
		t.Fatalf("session check response %d %s", rec.Code, rec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/web/session/logout", bytes.NewBufferString(`{}`))
	logoutReq.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, logoutReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout response %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := engine.AuthenticateSession(cookies[0].Value); ok {
		t.Fatal("session token remained valid after logout")
	}
}

func TestProtectedWebRoutesRequireSecuritySessionCookie(t *testing.T) {
	server := testServer(t)
	engine := security.NewEngine()
	engine.Users[9] = security.User{
		ID:         9,
		Login:      "demo",
		Password:   "secret",
		Active:     true,
		CompanyID:  2,
		CompanyIDs: []int64{2},
		GroupIDs:   []int64{10},
	}
	engine.IssueSession(9, "sid", time.Now().Add(time.Hour))
	server.Security = engine
	policy := &capturePolicy{}
	server.Env.WithPolicy(policy)
	handler := server.Handler()

	protected := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodPost, "/web/dataset/call_kw", `{"model":"res.partner","method":"create","values":{"name":"Blocked"}}`},
		{http.MethodGet, "/web/action/load?id=1", ""},
		{http.MethodGet, "/web/view/load?model=res.partner", ""},
		{http.MethodGet, "/web/webclient/load_menus", ""},
		{http.MethodGet, "/web/session/modules", ""},
	}
	for _, route := range protected {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(route.method, route.target, strings.NewReader(route.body)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated response %d %s", route.target, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/action/load?id=1&session_id=sid", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("query session accepted: %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(`{"model":"res.partner","method":"create","values":{"name":"Allowed"}}`))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated call_kw response %d %s", rec.Code, rec.Body.String())
	}
	if len(policy.checks) == 0 || policy.checks[0].UserID != 9 || policy.checks[0].CompanyID != 2 {
		t.Fatalf("policy context = %+v", policy.checks)
	}

	for _, target := range []string{"/web/action/load?id=1", "/web/view/load?model=res.partner", "/web/webclient/load_menus", "/web/session/modules"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s authenticated response %d %s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestWebclientLoadMenusOdooShape(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/webclient/load_menus/test-hash", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("load_menus response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["root"] == nil || payload["1"] == nil || payload["2"] == nil || payload["children"] == nil || payload["all_menu_ids"] == nil {
		t.Fatalf("menu payload = %#v", payload)
	}
	root := payload["root"].(map[string]any)
	rootChildren := root["children"].([]any)
	if len(rootChildren) != 1 {
		t.Fatalf("root children = %#v", rootChildren)
	}
	settings := payload["1"].(map[string]any)
	if settings["name"] != "Settings" || settings["appID"] != float64(1) || settings["actionID"] != float64(1) || settings["actionModel"] != "ir.actions.act_window" || settings["actionPath"] != "partners" || settings["xmlid"] != "base.menu_settings" {
		t.Fatalf("settings = %#v", settings)
	}
	partners := payload["2"].(map[string]any)
	if partners["actionID"] != float64(1) || partners["action"] != "ir.actions.act_window,base.action_partner" || partners["parent_id"] != float64(1) || partners["webIconData"] != "iVBORw0KGgo=" || partners["webIconDataMimetype"] != "image/png" {
		t.Fatalf("partners = %#v", partners)
	}
	children := payload["children"].(map[string]any)
	if children["2"].(map[string]any)["actionModel"] != "ir.actions.act_window" {
		t.Fatalf("compat children = %#v", children)
	}
}

func TestWebclientLoadMenusUsesDelegatedGroupsAndActionReadAccess(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	server := testServer(t)
	server.Env = server.Env.WithContext(record.Context{UserID: 20, Values: map[string]any{"group_ids": []int64{1}}})
	securityEngine := security.NewEngine()
	securityEngine.SetNow(func() time.Time { return now })
	securityEngine.Groups[1] = security.Group{ID: 1, Name: "Employee"}
	securityEngine.Groups[2] = security.Group{ID: 2, Name: "Approver"}
	securityEngine.Users[10] = security.User{ID: 10, Login: "delegator", Active: true, GroupIDs: []int64{2}}
	securityEngine.Users[20] = security.User{ID: 20, Login: "delegate", Active: true, GroupIDs: []int64{1}}
	securityEngine.ACLs = []security.ACL{{Model: "purchase.order", GroupID: 2, Active: true, PermRead: true}}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	securityEngine.SetDelegationProvider(svc)
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines:           []delegation.LineInput{{GroupID: 2, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	server.Env.WithPolicy(securityEngine)
	allowedActionID, err := server.Actions.Add(action.Action{Name: "Orders", Kind: action.ActWindow, ResModel: "purchase.order"})
	if err != nil {
		t.Fatal(err)
	}
	deniedActionID, err := server.Actions.Add(action.Action{Name: "Secret", Kind: action.ActWindow, ResModel: "secret.model"})
	if err != nil {
		t.Fatal(err)
	}
	rootID := server.Menus.Add(menu.Menu{Name: "Delegated Root"})
	server.Menus.Add(menu.Menu{Name: "Delegated Orders", ParentID: rootID, Groups: []int64{2}, ActionID: allowedActionID})
	server.Menus.Add(menu.Menu{Name: "Delegated Secret", ParentID: rootID, Groups: []int64{2}, ActionID: deniedActionID})

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/webclient/load_menus", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("load_menus response %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Delegated Orders") {
		t.Fatalf("delegated menu missing: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Delegated Secret") {
		t.Fatalf("denied action menu visible: %s", rec.Body.String())
	}
}

func TestActionLoadOdooShapeAndJSONRPC(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":11,"params":{"action_id":"partners"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/load", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("action load response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	if payload["id"] != float64(11) || result["type"] != "ir.actions.act_window" || result["path"] != "partners" || result["limit"] != float64(25) {
		t.Fatalf("action result = %#v", payload)
	}
	views := result["views"].([]any)
	if len(views) != 2 || views[0].([]any)[1] != "list" || views[1].([]any)[1] != "form" {
		t.Fatalf("views = %#v", views)
	}
	searchView := result["search_view_id"].([]any)
	if searchView[0] != float64(9) {
		t.Fatalf("search_view_id = %#v", searchView)
	}
	webDomain, ok := result["_web_domain"].([]any)
	if !ok {
		t.Fatalf("_web_domain missing: %#v", result)
	}
	if len(webDomain) != 0 {
		t.Fatalf("_web_domain = %#v", webDomain)
	}
	webContext, ok := result["_web_context"].(map[string]any)
	if !ok {
		t.Fatalf("_web_context missing: %#v", result)
	}
	if webContext["search_default_customer"] != true {
		t.Fatalf("_web_context = %#v", webContext)
	}
}

func TestActionLoadNormalizesWindowDomainContextForWebShell(t *testing.T) {
	server := testServer(t)
	actionID, err := server.Actions.Add(action.Action{
		Name:         "Active Partners",
		Kind:         action.ActWindow,
		ResModel:     "res.partner",
		ViewMode:     "list,form",
		SearchViewID: 9,
		Domain:       "[('active', '=', True)]",
		Context:      map[string]any{"search_default_active": true, "lang": "en_US"},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/web/action/load?id=%d", actionID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("action load response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	webDomain := payload["_web_domain"].([]any)
	if len(webDomain) != 1 {
		t.Fatalf("_web_domain = %#v", webDomain)
	}
	condition := webDomain[0].([]any)
	if condition[0] != "active" || condition[1] != "=" || condition[2] != true {
		t.Fatalf("_web_domain condition = %#v", condition)
	}
	webContext := payload["_web_context"].(map[string]any)
	if webContext["search_default_active"] != true || webContext["lang"] != "en_US" {
		t.Fatalf("_web_context = %#v", webContext)
	}
}

func TestWebShellActionMetadataNormalizesPythonLiterals(t *testing.T) {
	server := testServer(t)
	payload := map[string]any{
		"type":      "ir.actions.act_window",
		"res_model": "res.partner",
		"domain":    "[('active', '=', True)]",
		"context":   "{'search_default_active': True, 'lang': 'en_US'}",
	}
	enrichWebShellActionPayload(server.Env, payload)

	webDomain := payload["_web_domain"].([]any)
	if len(webDomain) != 1 {
		t.Fatalf("_web_domain = %#v", webDomain)
	}
	condition := webDomain[0].([]any)
	if condition[0] != "active" || condition[1] != "=" || condition[2] != true {
		t.Fatalf("_web_domain condition = %#v", condition)
	}
	webContext := payload["_web_context"].(map[string]any)
	if webContext["search_default_active"] != true || webContext["lang"] != "en_US" {
		t.Fatalf("_web_context = %#v", webContext)
	}
}

func TestCallKWGetViewsOdooShape(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":12,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[8,"list"],[false,"form"],[9,"search"]],"options":{"toolbar":true,"load_filters":true,"action_id":1},"context":{"lang":"en_US"}}}}`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	views := result["views"].(map[string]any)
	listView := views["list"].(map[string]any)
	formView := views["form"].(map[string]any)
	searchView := views["search"].(map[string]any)
	if listView["id"] != float64(8) || listView["arch"] != `<list><field name="name"/></list>` {
		t.Fatalf("list view = %#v", listView)
	}
	if formView["id"] != float64(10) || formView["arch"] != `<form><field name="name"/></form>` {
		t.Fatalf("form view = %#v", formView)
	}
	if searchView["id"] != float64(9) || searchView["arch"] != `<search><field name="name"/></search>` {
		t.Fatalf("search view = %#v", searchView)
	}
	if _, ok := listView["toolbar"].(map[string]any); !ok {
		t.Fatalf("toolbar missing: %#v", listView)
	}
	if filters := searchView["filters"].([]any); len(filters) != 0 {
		t.Fatalf("filters = %#v", filters)
	}
	models := result["models"].(map[string]any)
	fields := models["res.partner"].(map[string]any)["fields"].(map[string]any)
	nameField := fields["name"].(map[string]any)
	if nameField["type"] != "char" || nameField["string"] != "name" {
		t.Fatalf("name field = %#v", nameField)
	}
}

func TestCallKWReadMethodsApplyBinSizeContext(t *testing.T) {
	registry := record.NewRegistry()
	doc := model.New("x.document", "x_document")
	doc.AddField(field.New("name", field.Char))
	doc.AddField(field.New("payload", field.Binary))
	if err := registry.Register(doc); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1})
	id, err := env.Model("x.document").Create(map[string]any{"name": "Manual", "payload": "content"})
	if err != nil {
		t.Fatal(err)
	}
	handler := Server{Env: env}.Handler()

	tests := []struct {
		name  string
		body  string
		value func(map[string]any) any
	}{
		{
			name: "read",
			body: fmt.Sprintf(`{"jsonrpc":"2.0","id":30,"params":{"model":"x.document","method":"read","args":[[%d],["payload"]],"kwargs":{"context":{"bin_size":true}}}}`, id),
			value: func(payload map[string]any) any {
				return payload["result"].([]any)[0].(map[string]any)["payload"]
			},
		},
		{
			name: "search_read",
			body: `{"jsonrpc":"2.0","id":31,"params":{"model":"x.document","method":"search_read","args":[[]],"kwargs":{"fields":["payload"],"context":{"bin_size":true}}}}`,
			value: func(payload map[string]any) any {
				return payload["result"].([]any)[0].(map[string]any)["payload"]
			},
		},
		{
			name: "web_read",
			body: fmt.Sprintf(`{"jsonrpc":"2.0","id":32,"params":{"model":"x.document","method":"web_read","args":[[%d]],"kwargs":{"specification":{"payload":{}},"context":{"bin_size":true}}}}`, id),
			value: func(payload map[string]any) any {
				return payload["result"].([]any)[0].(map[string]any)["payload"]
			},
		},
		{
			name: "web_search_read",
			body: `{"jsonrpc":"2.0","id":33,"params":{"model":"x.document","method":"web_search_read","kwargs":{"domain":[],"specification":{"payload":{}},"context":{"bin_size":true}}}}`,
			value: func(payload map[string]any) any {
				records := payload["result"].(map[string]any)["records"].([]any)
				return records[0].(map[string]any)["payload"]
			},
		},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(tt.body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s response %d %s", tt.name, rec.Code, rec.Body.String())
		}
		payload := decodeJSON(t, rec.Body.Bytes())
		if got := tt.value(payload); got != "7 bytes" {
			t.Fatalf("%s payload = %#v", tt.name, payload)
		}
	}
}

func TestCallKWGetViewsToolbarBindings(t *testing.T) {
	server := testToolbarBindingServer(t)
	handler := server.Handler()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":19,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[101,"list"],[102,"form"],[103,"search"]],"options":{"toolbar":true}}}}`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	views := payload["result"].(map[string]any)["views"].(map[string]any)
	listToolbar := views["list"].(map[string]any)["toolbar"].(map[string]any)
	formToolbar := views["form"].(map[string]any)["toolbar"].(map[string]any)
	searchToolbar := views["search"].(map[string]any)["toolbar"].(map[string]any)

	listActions := listToolbar["action"].([]any)
	if len(listActions) != 2 {
		t.Fatalf("list actions = %#v", listActions)
	}
	if listActions[0].(map[string]any)["name"] != "First Server Action" || listActions[1].(map[string]any)["name"] != "Second Server Action" {
		t.Fatalf("list action order = %#v", listActions)
	}
	firstAction := listActions[0].(map[string]any)
	if firstAction["binding_view_types"] != "list" || firstAction["sequence"] != float64(10) {
		t.Fatalf("server binding payload = %#v", firstAction)
	}
	firstActionID := int64Value(firstAction["id"])
	if firstActionID == 0 {
		t.Fatalf("server binding id = %#v", firstAction["id"])
	}
	if _, ok := firstAction["type"]; ok {
		t.Fatalf("server binding leaked type: %#v", firstAction)
	}
	if _, ok := firstAction["binding_type"]; ok {
		t.Fatalf("server binding leaked binding_type: %#v", firstAction)
	}
	loadBody := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":18,"params":{"action_id":%d}}`, firstActionID))
	loadRec := httptest.NewRecorder()
	handler.ServeHTTP(loadRec, httptest.NewRequest(http.MethodPost, "/web/action/load", loadBody))
	if loadRec.Code != http.StatusOK {
		t.Fatalf("server action load response %d %s", loadRec.Code, loadRec.Body.String())
	}
	loadedServer := decodeJSON(t, loadRec.Body.Bytes())["result"].(map[string]any)
	if loadedServer["type"] != "ir.actions.server" || loadedServer["name"] != "First Server Action" {
		t.Fatalf("loaded server action = %#v", loadedServer)
	}
	assertActionPayloadKeys(t, loadedServer, actionPayloadKeys("group_ids", "model_name")...)
	if _, ok := loadedServer["model_name"]; !ok {
		t.Fatalf("loaded server action missing model_name: %#v", loadedServer)
	}
	for _, leaked := range []string{"state", "active", "sequence", "code", "model_id"} {
		if _, ok := loadedServer[leaked]; ok {
			t.Fatalf("server action leaked %s: %#v", leaked, loadedServer)
		}
	}
	for _, item := range listActions {
		if item.(map[string]any)["name"] == "Hidden Server Action" {
			t.Fatalf("restricted server action leaked: %#v", listActions)
		}
	}
	listPrint := listToolbar["print"].([]any)
	if len(listPrint) != 1 {
		t.Fatalf("list print = %#v", listPrint)
	}
	report := listPrint[0].(map[string]any)
	if report["name"] != "Partner Label" || report["binding_view_types"] != "list,form" || report["domain"] != `[("active","=",True)]` {
		t.Fatalf("report binding payload = %#v", report)
	}
	reportID := int64Value(report["id"])
	if reportID == 0 {
		t.Fatalf("report binding id = %#v", report["id"])
	}
	if _, ok := report["report_name"]; ok {
		t.Fatalf("report binding leaked report fields: %#v", report)
	}

	formActions := formToolbar["action"].([]any)
	if len(formActions) != 1 {
		t.Fatalf("form actions = %#v", formActions)
	}
	formAction := formActions[0].(map[string]any)
	if formAction["name"] != "Partner Wizard" || formAction["binding_view_types"] != "form" {
		t.Fatalf("form action payload = %#v", formAction)
	}
	formActionID := int64Value(formAction["id"])
	if formActionID == 0 {
		t.Fatalf("form action id = %#v", formAction["id"])
	}
	if _, ok := formAction["res_model"]; ok {
		t.Fatalf("form action leaked res_model: %#v", formAction)
	}
	if len(searchToolbar) != 0 {
		t.Fatalf("search toolbar = %#v", searchToolbar)
	}

	loadBody = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":20,"params":{"action_id":%d}}`, reportID))
	loadRec = httptest.NewRecorder()
	handler.ServeHTTP(loadRec, httptest.NewRequest(http.MethodPost, "/web/action/load", loadBody))
	if loadRec.Code != http.StatusOK {
		t.Fatalf("report action load response %d %s", loadRec.Code, loadRec.Body.String())
	}
	loadedReport := decodeJSON(t, loadRec.Body.Bytes())["result"].(map[string]any)
	if loadedReport["type"] != "ir.actions.report" || loadedReport["report_name"] != "x_partner.label" || loadedReport["report_type"] != "qweb-pdf" || loadedReport["binding_view_types"] != "list,form" {
		t.Fatalf("loaded report action = %#v", loadedReport)
	}
	assertActionPayloadKeys(t, loadedReport, actionPayloadKeys("report_name", "report_type", "target", "context", "data", "close_on_report_download", "domain")...)
	for _, leaked := range []string{"model", "model_id", "report_file", "print_report_name", "attachment", "attachment_use", "multi", "paperformat_id", "groups_id", "is_invoice_report"} {
		if _, ok := loadedReport[leaked]; ok {
			t.Fatalf("report action leaked %s: %#v", leaked, loadedReport)
		}
	}

	loadBody = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":21,"params":{"action_id":%d}}`, formActionID))
	loadRec = httptest.NewRecorder()
	handler.ServeHTTP(loadRec, httptest.NewRequest(http.MethodPost, "/web/action/load", loadBody))
	if loadRec.Code != http.StatusOK {
		t.Fatalf("window action load response %d %s", loadRec.Code, loadRec.Body.String())
	}
	loadedWindow := decodeJSON(t, loadRec.Body.Bytes())["result"].(map[string]any)
	if loadedWindow["type"] != "ir.actions.act_window" || loadedWindow["res_model"] != "partner.wizard" || loadedWindow["target"] != "new" {
		t.Fatalf("loaded window action = %#v", loadedWindow)
	}
	if loadedWindow["view_mode"] != "list,form" || loadedWindow["mobile_view_mode"] != "kanban" || loadedWindow["context"] != "{}" || int64Value(loadedWindow["limit"]) != 80 || loadedWindow["cache"] != true || loadedWindow["filter"] != false {
		t.Fatalf("loaded window defaults = %#v", loadedWindow)
	}
	assertActionPayloadKeys(t, loadedWindow, actionPayloadKeys("context", "cache", "mobile_view_mode", "domain", "filter", "group_ids", "limit", "res_id", "res_model", "search_view_id", "target", "view_id", "view_mode", "views", "embedded_action_ids", "close_on_report_download", "_web_domain", "_web_context")...)
	baseRows, err := server.Env.Model("ir.actions.actions").Browse(firstActionID, reportID, formActionID).Read("id", "type", "name")
	if err != nil {
		t.Fatal(err)
	}
	typesByID := map[int64]string{}
	for _, row := range baseRows {
		typesByID[int64Value(row["id"])] = stringValue(row["type"])
	}
	if typesByID[firstActionID] != "ir.actions.server" || typesByID[reportID] != "ir.actions.report" || typesByID[formActionID] != "ir.actions.act_window" {
		t.Fatalf("global action base rows = %#v", baseRows)
	}
}

func TestActionLoadReportForcesBinSizeContext(t *testing.T) {
	registry := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := registry.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(registry, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	reportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":        "Partner Label",
		"type":        "ir.actions.report",
		"model":       "res.partner",
		"report_name": "x_partner.label",
		"report_type": "qweb-pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := &reportBinSizePolicy{}
	env.WithPolicy(policy)
	handler := Server{Env: env}.Handler()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":34,"params":{"action_id":%d,"context":{"bin_size":false}}}`, reportID))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/load", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("report action load response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	if payload["type"] != "ir.actions.report" || payload["report_name"] != "x_partner.label" {
		t.Fatalf("loaded report = %#v", payload)
	}
	if len(policy.reportReads) == 0 {
		t.Fatal("policy did not observe report read")
	}
}

func TestCallKWReportActionValidatesDomain(t *testing.T) {
	server := testToolbarBindingServer(t)
	env := server.Env
	activePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Active Partner", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	inactivePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Inactive Partner", "active": false})
	if err != nil {
		t.Fatal(err)
	}
	found, err := env.Model("ir.actions.report").SearchWithOptions(domain.Cond("report_name", domain.Equal, "x_partner.label"), record.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("report action not found")
	}
	domainReportID := int64Value(rows[0]["id"])
	server.ExternalIDs = map[string]data.ExternalID{
		"base.action_partner_label": {Module: "base", Name: "action_partner_label", Model: "ir.actions.report", ResID: domainReportID},
	}
	noDomainReportID, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":        "Always Available",
		"type":        "ir.actions.report",
		"model":       "res.partner",
		"report_name": "x_partner.always",
		"report_type": "qweb-pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	typedReportXMLID := "ir.actions.report,base.action_partner_label"

	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":22,"params":{"model":"ir.actions.report","method":"get_valid_action_reports","args":[[%q,%d],"res.partner",[%d]]}}`, typedReportXMLID, noDomainReportID, inactivePartnerID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.actions.report/get_valid_action_reports", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("inactive validation response %d %s", rec.Code, rec.Body.String())
	}
	inactiveResult := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	if len(inactiveResult) != 1 || int64Value(inactiveResult[0]) != noDomainReportID {
		t.Fatalf("inactive valid reports = %#v", inactiveResult)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":23,"params":{"model":"ir.actions.report","method":"get_valid_action_reports","args":[[%q,%d],"res.partner",[%d,%d]]}}`, typedReportXMLID, noDomainReportID, inactivePartnerID, activePartnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.actions.report/get_valid_action_reports", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("active validation response %d %s", rec.Code, rec.Body.String())
	}
	activeResult := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	if len(activeResult) != 2 || activeResult[0] != typedReportXMLID || int64Value(activeResult[1]) != noDomainReportID {
		t.Fatalf("active valid reports = %#v", activeResult)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":24,"params":{"model":"ir.actions.report","method":"get_valid_action_reports","args":[[%d],"res.partner",[%d]]}}`, domainReportID, activePartnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.actions.report/get_valid_action_reports", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("numeric validation response %d %s", rec.Code, rec.Body.String())
	}
	numericResult := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	if len(numericResult) != 1 || int64Value(numericResult[0]) != domainReportID {
		t.Fatalf("numeric valid reports = %#v", numericResult)
	}
}

func TestActionLoadGlobalURLAndClientActions(t *testing.T) {
	server := testToolbarBindingServer(t)
	env := server.Env
	urlID, err := env.Model("ir.actions.act_url").Create(map[string]any{
		"name":   "Docs",
		"url":    "https://example.test/docs",
		"target": "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := env.Model("ir.actions.client").Create(map[string]any{
		"name":      "Discuss",
		"type":      "ir.actions.client",
		"tag":       "mail.action_discuss",
		"res_model": "mail.channel",
		"target":    "current",
		"context":   `{"active_test": false}`,
		"params":    map[string]any{"default_active_id": int64(7)},
	})
	if err != nil {
		t.Fatal(err)
	}
	closeID, err := env.Model("ir.actions.act_window_close").Create(map[string]any{
		"name":   "Close Wizard",
		"type":   "ir.actions.act_window_close",
		"effect": map[string]any{"fadeout": "slow"},
		"infos":  map[string]any{"reload": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	server.ExternalIDs = map[string]data.ExternalID{
		"base.action_docs":          {Module: "base", Name: "action_docs", Model: "ir.actions.act_url", ResID: urlID},
		"base.action_discuss":       {Module: "base", Name: "action_discuss", Model: "ir.actions.client", ResID: clientID},
		"base.action_close_wizard":  {Module: "base", Name: "action_close_wizard", Model: "ir.actions.act_window_close", ResID: closeID},
		"base.action_close_generic": {Module: "base", Name: "action_close_generic", Model: "ir.actions.actions", ResID: closeID},
	}
	handler := server.Handler()
	for _, tc := range []struct {
		request  any
		id       int64
		wantType string
		wantKey  string
		want     any
	}{
		{urlID, urlID, "ir.actions.act_url", "url", "https://example.test/docs"},
		{"base.action_docs", urlID, "ir.actions.act_url", "url", "https://example.test/docs"},
		{"action_docs", urlID, "ir.actions.act_url", "url", "https://example.test/docs"},
		{"ir.actions.act_url,base.action_docs", urlID, "ir.actions.act_url", "url", "https://example.test/docs"},
		{"ir.actions.act_url,action_docs", urlID, "ir.actions.act_url", "url", "https://example.test/docs"},
		{clientID, clientID, "ir.actions.client", "tag", "mail.action_discuss"},
		{"base.action_discuss", clientID, "ir.actions.client", "tag", "mail.action_discuss"},
		{"ir.actions.client,base.action_discuss", clientID, "ir.actions.client", "tag", "mail.action_discuss"},
		{closeID, closeID, "ir.actions.act_window_close", "name", "Close Wizard"},
		{"base.action_close_wizard", closeID, "ir.actions.act_window_close", "name", "Close Wizard"},
		{"ir.actions.act_window_close,base.action_close_wizard", closeID, "ir.actions.act_window_close", "name", "Close Wizard"},
		{"base.action_close_generic", closeID, "ir.actions.act_window_close", "name", "Close Wizard"},
		{fmt.Sprintf("ir.actions.act_window_close,%d", closeID), closeID, "ir.actions.act_window_close", "name", "Close Wizard"},
	} {
		rawRequest, err := json.Marshal(tc.request)
		if err != nil {
			t.Fatal(err)
		}
		body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":25,"params":{"action_id":%s}}`, rawRequest))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/load", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("action load response %d %s", rec.Code, rec.Body.String())
		}
		loaded := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
		if loaded["id"] != float64(tc.id) || loaded["type"] != tc.wantType || loaded[tc.wantKey] != tc.want {
			t.Fatalf("loaded action = %#v", loaded)
		}
		switch tc.wantType {
		case "ir.actions.act_url":
			assertActionPayloadKeys(t, loaded, actionPayloadKeys("target", "url", "close")...)
		case "ir.actions.client":
			assertActionPayloadKeys(t, loaded, actionPayloadKeys("tag", "res_model", "target", "context", "params")...)
		case "ir.actions.act_window_close":
			assertActionPayloadKeys(t, loaded, actionPayloadKeys("effect", "infos")...)
		}
	}
}

var commonActionPayloadKeys = []string{"id", "name", "display_name", "type", "binding_model_id", "binding_type", "binding_view_types", "help", "path", "xml_id"}

func actionPayloadKeys(extra ...string) []string {
	out := append([]string{}, commonActionPayloadKeys...)
	return append(out, extra...)
}

func assertActionPayloadKeys(t *testing.T, payload map[string]any, keys ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, key := range keys {
		want[key] = true
		if _, ok := payload[key]; !ok {
			t.Fatalf("action payload missing %s: %#v", key, payload)
		}
	}
	for key := range payload {
		if !want[key] {
			t.Fatalf("action payload leaked %s: %#v", key, payload)
		}
	}
}

func TestCleanActionResultFiltersReadableModelFields(t *testing.T) {
	server := testToolbarBindingServer(t)
	reportAction := map[string]any{
		"type":                     "ir.actions.report",
		"name":                     "Report",
		"report_name":              "x.report",
		"report_type":              "qweb-pdf",
		"context":                  map[string]any{"active_id": int64(7)},
		"data":                     map[string]any{"ids": []int64{7}},
		"target":                   "new",
		"close_on_report_download": true,
		"domain":                   `[("active","=",True)]`,
		"model":                    "res.partner",
		"model_id":                 int64(11),
		"report_file":              "x.report",
		"print_report_name":        "'Report'",
		"attachment":               "attachment_expr",
		"attachment_use":           true,
		"multi":                    true,
		"paperformat_id":           int64(3),
		"groups_id":                []int64{1},
		"is_invoice_report":        true,
		"custom_payload":           "kept",
	}
	cleaned := server.cleanActionResult(server.Env, reportAction).(map[string]any)
	for _, key := range []string{"type", "name", "report_name", "report_type", "context", "data", "target", "close_on_report_download", "domain", "custom_payload"} {
		if _, ok := cleaned[key]; !ok {
			t.Fatalf("cleaned action missing %s: %#v", key, cleaned)
		}
	}
	for _, key := range []string{"model", "model_id", "report_file", "print_report_name", "attachment", "attachment_use", "multi", "paperformat_id", "groups_id", "is_invoice_report"} {
		if _, ok := cleaned[key]; ok {
			t.Fatalf("cleaned action leaked %s: %#v", key, cleaned)
		}
	}
	closeAction := map[string]any{
		"type":     "ir.actions.act_window_close",
		"effect":   map[string]any{"fadeout": "slow"},
		"infos":    map[string]any{"reload": true},
		"mail_ids": []int64{4, 5},
	}
	cleanedClose := server.cleanActionResult(server.Env, closeAction).(map[string]any)
	if _, ok := cleanedClose["mail_ids"]; !ok {
		t.Fatalf("cleaned close action dropped custom key: %#v", cleanedClose)
	}
	windowAction := map[string]any{
		"type":      "ir.actions.act_window",
		"name":      "Direct Window",
		"res_model": "res.partner",
		"view_mode": "form",
		"view_id":   int64(321),
		"usage":     "menu",
	}
	cleanedWindow := server.cleanActionResult(server.Env, windowAction).(map[string]any)
	views := cleanedWindow["views"].([]any)
	viewPair := views[0].([]any)
	if int64Value(viewPair[0]) != int64(321) || viewPair[1] != "form" {
		t.Fatalf("generated views = %#v", cleanedWindow["views"])
	}
	if _, ok := cleanedWindow["usage"]; ok {
		t.Fatalf("cleaned window leaked usage: %#v", cleanedWindow)
	}
	tupleWindowAction := map[string]any{
		"type":      "ir.actions.act_window",
		"name":      "Tuple Direct Window",
		"res_model": "res.partner",
		"view_mode": "form",
		"view_id":   []any{float64(654), "Partner Form"},
	}
	cleanedTupleWindow := server.cleanActionResult(server.Env, tupleWindowAction).(map[string]any)
	tupleViews := cleanedTupleWindow["views"].([]any)
	tupleViewPair := tupleViews[0].([]any)
	if int64Value(tupleViewPair[0]) != int64(654) || tupleViewPair[1] != "form" {
		t.Fatalf("generated tuple views = %#v", cleanedTupleWindow["views"])
	}
}

func TestCleanActionResultRejectsDirectWindowViewIDWithMultiViewMode(t *testing.T) {
	server := testToolbarBindingServer(t)
	action := map[string]any{
		"type":      "ir.actions.act_window",
		"name":      "Invalid Direct Window",
		"res_model": "res.partner",
		"view_mode": "list,form",
		"view_id":   int64(321),
	}
	_, err := server.cleanActionResultWithError(server.Env, action)
	if err == nil {
		t.Fatal("expected invalid view_id/view_mode error")
	}
	if !strings.Contains(err.Error(), "either multiple view modes or a single view mode and an optional view id") {
		t.Fatalf("error = %v", err)
	}
}

func TestActionLoadAppliesRequestContextToEmbeddedActions(t *testing.T) {
	server := testToolbarBindingServer(t)
	env := server.Env
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Context Partner"})
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":      "Partner Context",
		"res_model": "res.partner",
	})
	if err != nil {
		t.Fatal(err)
	}
	embeddedID, err := env.Model("ir.embedded.actions").Create(map[string]any{
		"name":             "Context Embedded",
		"parent_action_id": actionID,
		"parent_res_id":    partnerID,
		"parent_res_model": "res.partner",
		"python_method":    "action_context_embedded",
		"is_visible":       true,
		"domain":           "[]",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":31,"params":{"action_id":%d}}`, actionID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/load", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("action load response %d %s", rec.Code, rec.Body.String())
	}
	withoutContext := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	if ids := withoutContext["embedded_action_ids"].([]any); len(ids) != 0 {
		t.Fatalf("embedded actions without active_id = %#v", ids)
	}
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":32,"params":{"action_id":%d,"context":{"active_id":%d,"active_model":"res.partner"}}}`, actionID, partnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/load", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("context action load response %d %s", rec.Code, rec.Body.String())
	}
	withContext := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	ids := withContext["embedded_action_ids"].([]any)
	if len(ids) != 1 || int64Value(ids[0]) != embeddedID {
		t.Fatalf("embedded actions with active_id = %#v", ids)
	}
}

func TestActionRunExecutesServerActionAndCleansReturnedAction(t *testing.T) {
	server := testToolbarBindingServer(t)
	registry := internalactions.NewRegistry(internalactions.Hooks{})
	var captured internalactions.ExecutionContext
	if err := registry.RegisterGo("return.window", func(_ context.Context, _ internalactions.ServerAction, exec internalactions.ExecutionContext) (internalactions.Result, error) {
		captured = exec
		return internalactions.Result{
			Action: map[string]any{
				"type":      "ir.actions.act_window",
				"name":      "Returned Window",
				"res_model": "res.partner",
				"view_mode": "form",
				"view_id":   int64(88),
				"usage":     "server-only",
			},
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := registry.Register(internalactions.ServerAction{Name: "Return Window", Kind: internalactions.KindGo, GoActionName: "return.window", Model: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	falseActionID, err := registry.Register(internalactions.ServerAction{Name: "Return False", Kind: internalactions.KindGo, GoActionName: "return.false", Model: "res.partner"})
	if err != nil {
		t.Fatal(err)
	}
	warningActionID, err := registry.Register(internalactions.ServerAction{Name: "Warned Action", Kind: internalactions.KindGo, GoActionName: "return.false", Model: "res.partner", Warning: "unsafe configuration"})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterGo("return.false", func(_ context.Context, _ internalactions.ServerAction, _ internalactions.ExecutionContext) (internalactions.Result, error) {
		return internalactions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	server.ServerActions = registry
	handler := server.Handler()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":33,"params":{"action_id":%d,"context":{"active_model":"res.partner","active_id":99,"active_ids":[44],"group_ids":[7]}}}`, actionID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/run", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("action run response %d %s", rec.Code, rec.Body.String())
	}
	if captured.Model != "res.partner" || captured.RecordID != 44 || len(captured.RecordIDs) != 1 || captured.RecordIDs[0] != 44 || captured.UserID == 0 {
		t.Fatalf("captured execution = %+v", captured)
	}
	result := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	views := result["views"].([]any)
	viewPair := views[0].([]any)
	if result["type"] != "ir.actions.act_window" || result["res_model"] != "res.partner" || int64Value(viewPair[0]) != 88 || viewPair[1] != "form" {
		t.Fatalf("run result = %#v", result)
	}
	if _, ok := result["usage"]; ok {
		t.Fatalf("run result leaked usage: %#v", result)
	}
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":34,"params":{"action_id":%d,"context":{"active_model":"res.partner","active_id":44}}}`, falseActionID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/run", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("false action run response %d %s", rec.Code, rec.Body.String())
	}
	if result := decodeJSON(t, rec.Body.Bytes())["result"]; result != false {
		t.Fatalf("false run result = %#v", result)
	}
	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":35,"params":{"action_id":%d,"context":{"active_model":"res.partner","active_id":44}}}`, warningActionID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/action/run", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("warning action run response %d %s", rec.Code, rec.Body.String())
	}
	warningEnvelope := decodeJSON(t, rec.Body.Bytes())
	warningError := warningEnvelope["error"].(map[string]any)
	if int64Value(warningError["code"]) != 0 || warningError["message"] != "Odoo Server Error" {
		t.Fatalf("warning error = %#v", warningError)
	}
	warningData := warningError["data"].(map[string]any)
	if warningData["name"] != "odoo.addons.base.models.ir_actions.ServerActionWithWarningsError" {
		t.Fatalf("warning data name = %#v", warningData)
	}
	if warningData["message"] != "Server action Warned Action has one or more warnings, address them first." {
		t.Fatalf("warning data message = %#v", warningData)
	}
	warningArgs := warningData["arguments"].([]any)
	if len(warningArgs) != 1 || warningArgs[0] != warningData["message"] {
		t.Fatalf("warning arguments = %#v", warningArgs)
	}
	if warningContext := warningData["context"].(map[string]any); len(warningContext) != 0 {
		t.Fatalf("warning context = %#v", warningContext)
	}
	if debug := stringValue(warningData["debug"]); !strings.Contains(debug, "ServerActionWithWarningsError") || !strings.Contains(debug, "Warned Action") {
		t.Fatalf("warning debug = %q", debug)
	}
	if _, ok := warningEnvelope["result"]; ok {
		t.Fatalf("warning envelope leaked result: %#v", warningEnvelope)
	}
}

func TestCallKWGenericStaticActionMethods(t *testing.T) {
	server := testToolbarBindingServer(t)
	env := server.Env
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Nested Export Co"})
	if err != nil {
		t.Fatal(err)
	}
	countryID, err := env.Model("res.country").Create(map[string]any{"name": "Bahrain", "code": "BH"})
	if err != nil {
		t.Fatal(err)
	}
	otherCountryID, err := env.Model("res.country").Create(map[string]any{"name": "Qatar", "code": "QA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "x_export", "name": "country_bh", "complete_name": "x_export.country_bh", "model": "res.country", "res_id": countryID}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "x_export", "name": "country_qa", "complete_name": "x_export.country_qa", "model": "res.country", "res_id": otherCountryID}); err != nil {
		t.Fatal(err)
	}
	countryGroupID, err := env.Model("res.country.group").Create(map[string]any{"name": "GCC", "country_ids": []int64{countryID, otherCountryID}})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Static Partner", "active": true, "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{"module": "x_export", "name": "partner_static", "complete_name": "x_export.partner_static", "model": "res.partner", "res_id": partnerID}); err != nil {
		t.Fatal(err)
	}
	formulaPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "=2+2", "active": false, "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":24,"params":{"model":"res.partner","method":"action_archive","args":[[%d]]}}`, partnerID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/action_archive", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("archive response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := env.Model("res.partner").Browse(partnerID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != false {
		t.Fatalf("archived partner = %#v", rows[0])
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":25,"params":{"model":"res.partner","method":"action_unarchive","args":[[%d]]}}`, partnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/action_unarchive", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = env.Model("res.partner").Browse(partnerID).Read("active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != true {
		t.Fatalf("unarchived partner = %#v", rows[0])
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":26,"params":{"model":"res.partner","method":"export_data","args":[[%d],["name","active"]]}}`, partnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/export_data", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("export response %d %s", rec.Code, rec.Body.String())
	}
	exportResult := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	datas := exportResult["datas"].([]any)
	if len(datas) != 1 {
		t.Fatalf("export datas = %#v", datas)
	}
	line := datas[0].([]any)
	if line[0] != "Static Partner" || line[1] != true {
		t.Fatalf("export line = %#v", line)
	}
	inactiveExportID, err := env.Model("res.partner").Create(map[string]any{"name": "Inactive Export", "active": false, "company_id": companyID})
	if err != nil {
		t.Fatal(err)
	}

	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":27,"params":{"model":"res.partner","method":"export_data","args":[["name","active"]],"kwargs":{"context":{"active_test":false,"active_domain":[["name","=","Inactive Export"]]}}}}`)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/export_data", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("inactive export response %d %s", rec.Code, rec.Body.String())
	}
	inactiveExportResult := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	inactiveDatas := inactiveExportResult["datas"].([]any)
	if len(inactiveDatas) != 1 {
		t.Fatalf("inactive export datas = %#v", inactiveDatas)
	}
	inactiveLine := inactiveDatas[0].([]any)
	if inactiveLine[0] != "Inactive Export" || inactiveLine[1] != false || inactiveExportID == 0 {
		t.Fatalf("inactive export line = %#v", inactiveLine)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":36,"params":{"model":"res.country.group","method":"export_data","args":[[%d],["name","country_ids"]],"kwargs":{"context":{"import_compat":false}}}}`, countryGroupID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.country.group/export_data", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("x2many export_data response %d %s", rec.Code, rec.Body.String())
	}
	x2manyExportResult := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	x2manyDatas := x2manyExportResult["datas"].([]any)
	if len(x2manyDatas) != 2 {
		t.Fatalf("x2many export datas = %#v", x2manyDatas)
	}
	x2manyFirstLine := x2manyDatas[0].([]any)
	x2manySecondLine := x2manyDatas[1].([]any)
	if !reflect.DeepEqual(x2manyFirstLine, []any{"GCC", "Bahrain"}) || !reflect.DeepEqual(x2manySecondLine, []any{"", "Qatar"}) {
		t.Fatalf("x2many export lines = %#v %#v", x2manyFirstLine, x2manySecondLine)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":26,"params":{"model":"res.partner","method":"export_data","args":[[%d],["id",".id","company_id/id","company_id.id","company_id:id","company_id","company_id/display_name"]]}}`, partnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/export_data", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("export xml id response %d %s", rec.Code, rec.Body.String())
	}
	exportXMLIDResult := decodeJSON(t, rec.Body.Bytes())["result"].(map[string]any)
	exportXMLIDLine := exportXMLIDResult["datas"].([]any)[0].([]any)
	generatedCompanyXMLID := fmt.Sprint(exportXMLIDLine[2])
	if exportXMLIDLine[0] != "x_export.partner_static" ||
		exportXMLIDLine[1] != strconv.FormatInt(partnerID, 10) ||
		!strings.HasPrefix(generatedCompanyXMLID, fmt.Sprintf("__export__.res_company_%d_", companyID)) ||
		exportXMLIDLine[3] != strconv.FormatInt(companyID, 10) ||
		exportXMLIDLine[4] != generatedCompanyXMLID ||
		exportXMLIDLine[5] != "Nested Export Co" ||
		exportXMLIDLine[6] != "Nested Export Co" {
		t.Fatalf("export xml id line = %#v", exportXMLIDLine)
	}

	body = bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":27,"params":{"model":"res.partner","method":"copy","args":[[%d],{}]}}`, partnerID))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/copy", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("copy response %d %s", rec.Code, rec.Body.String())
	}
	copyResult := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	if len(copyResult) != 1 {
		t.Fatalf("copy result = %#v", copyResult)
	}
	copyID := int64Value(copyResult[0])
	rows, err = env.Model("res.partner").Browse(copyID).Read("name", "active")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["name"] != "Static Partner (copy)" || rows[0]["active"] != true {
		t.Fatalf("copied partner = %#v", rows[0])
	}

	formatsRec := httptest.NewRecorder()
	handler.ServeHTTP(formatsRec, httptest.NewRequest(http.MethodPost, "/web/export/formats", bytes.NewBufferString(`{"jsonrpc":"2.0","id":27,"params":{}}`)))
	if formatsRec.Code != http.StatusOK {
		t.Fatalf("formats response %d %s", formatsRec.Code, formatsRec.Body.String())
	}
	formatsPayload := decodeJSON(t, formatsRec.Body.Bytes())["result"].([]any)
	if len(formatsPayload) != 2 || formatsPayload[0].(map[string]any)["tag"] != "xlsx" || formatsPayload[1].(map[string]any)["tag"] != "csv" {
		t.Fatalf("formats = %#v", formatsPayload)
	}

	fieldsRec := httptest.NewRecorder()
	handler.ServeHTTP(fieldsRec, httptest.NewRequest(http.MethodPost, "/web/export/get_fields", bytes.NewBufferString(`{"jsonrpc":"2.0","id":28,"params":{"model":"res.partner","domain":[],"import_compat":true}}`)))
	if fieldsRec.Code != http.StatusOK {
		t.Fatalf("get_fields response %d %s", fieldsRec.Code, fieldsRec.Body.String())
	}
	fieldRows := decodeJSON(t, fieldsRec.Body.Bytes())["result"].([]any)
	hasExternalID := false
	hasDefaultName := false
	var companyField map[string]any
	for _, row := range fieldRows {
		field := row.(map[string]any)
		if field["id"] == "id" && field["string"] == "External ID" && field["value"] == "id" {
			hasExternalID = true
		}
		if field["id"] == "name" && field["default_export"] == true {
			hasDefaultName = true
		}
		if field["id"] == "company_id" {
			companyField = field
		}
	}
	if !hasExternalID || !hasDefaultName || companyField == nil || companyField["children"] != true {
		t.Fatalf("export fields = %#v", fieldRows)
	}
	params := companyField["params"].(map[string]any)
	if params["model"] != "res.company" || params["prefix"] != "company_id" {
		t.Fatalf("company field params = %#v", params)
	}

	nestedFieldsRec := httptest.NewRecorder()
	handler.ServeHTTP(nestedFieldsRec, httptest.NewRequest(http.MethodPost, "/web/export/get_fields", bytes.NewBufferString(`{"jsonrpc":"2.0","id":29,"params":{"model":"res.company","prefix":"company_id","parent_name":"Company","parent_field_type":"many2one","parent_field":{"string":"Company","type":"many2one","field_type":"many2one","relation":"res.company"},"import_compat":true}}`)))
	if nestedFieldsRec.Code != http.StatusOK {
		t.Fatalf("nested get_fields response %d %s", nestedFieldsRec.Code, nestedFieldsRec.Body.String())
	}
	nestedRows := decodeJSON(t, nestedFieldsRec.Body.Bytes())["result"].([]any)
	nestedIDs := make([]string, 0, len(nestedRows))
	nestedByID := map[string]map[string]any{}
	for _, row := range nestedRows {
		field := row.(map[string]any)
		id := field["id"].(string)
		nestedIDs = append(nestedIDs, id)
		nestedByID[id] = field
	}
	if !reflect.DeepEqual(nestedIDs, []string{"company_id/id", "company_id/name"}) {
		t.Fatalf("nested export ids = %#v rows=%#v", nestedIDs, nestedRows)
	}
	nestedIDField := nestedByID["company_id/id"]
	if nestedIDField["string"] != "Company/External ID" || nestedIDField["value"] != "company_id/id/id" || nestedIDField["field_type"] != "many2one" || nestedIDField["type"] != "many2one" || nestedIDField["children"] != true {
		t.Fatalf("nested id field = %#v", nestedIDField)
	}

	createTemplateRec := httptest.NewRecorder()
	handler.ServeHTTP(createTemplateRec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/ir.exports/create", bytes.NewBufferString(`{"jsonrpc":"2.0","id":30,"params":{"model":"ir.exports","method":"create","args":[[{"name":"Partner Template","resource":"res.partner","export_fields":[[0,0,{"name":"name"}],[0,0,{"name":"company_id/name"}],[0,0,{"name":"company_id/active"}]]}]]}}`)))
	if createTemplateRec.Code != http.StatusOK {
		t.Fatalf("create export template response %d %s", createTemplateRec.Code, createTemplateRec.Body.String())
	}
	createdTemplateIDs := decodeJSON(t, createTemplateRec.Body.Bytes())["result"].([]any)
	if len(createdTemplateIDs) != 1 {
		t.Fatalf("created template ids = %#v", createdTemplateIDs)
	}
	exportTemplateID := int64Value(createdTemplateIDs[0])
	namelistRec := httptest.NewRecorder()
	handler.ServeHTTP(namelistRec, httptest.NewRequest(http.MethodPost, "/web/export/namelist", bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":31,"params":{"model":"res.partner","export_id":%d}}`, exportTemplateID))))
	if namelistRec.Code != http.StatusOK {
		t.Fatalf("namelist response %d %s", namelistRec.Code, namelistRec.Body.String())
	}
	namelistRows := decodeJSON(t, namelistRec.Body.Bytes())["result"].([]any)
	gotNamelist := make([][]string, 0, len(namelistRows))
	for _, row := range namelistRows {
		item := row.(map[string]any)
		gotNamelist = append(gotNamelist, []string{item["id"].(string), item["string"].(string)})
	}
	if !reflect.DeepEqual(gotNamelist, [][]string{{"name", "name"}, {"company_id/name", "company_id/name"}, {"company_id/active", "company_id/active"}}) {
		t.Fatalf("namelist = %#v", namelistRows)
	}

	importNamelistRec := httptest.NewRecorder()
	handler.ServeHTTP(importNamelistRec, httptest.NewRequest(http.MethodPost, "/web/export/namelist", bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":32,"params":{"model":"res.partner","export_id":%d,"import_compat":true}}`, exportTemplateID))))
	if importNamelistRec.Code != http.StatusOK {
		t.Fatalf("import namelist response %d %s", importNamelistRec.Code, importNamelistRec.Body.String())
	}
	importNamelistRows := decodeJSON(t, importNamelistRec.Body.Bytes())["result"].([]any)
	gotImportNamelist := make([][]string, 0, len(importNamelistRows))
	for _, row := range importNamelistRows {
		item := row.(map[string]any)
		gotImportNamelist = append(gotImportNamelist, []string{item["id"].(string), item["string"].(string)})
	}
	if !reflect.DeepEqual(gotImportNamelist, [][]string{{"name", "name"}, {"company_id/name", "company_id/name"}, {"company_id/active", "company_id/active"}}) {
		t.Fatalf("import namelist = %#v", importNamelistRows)
	}

	form := url.Values{}
	form.Set("data", fmt.Sprintf(`{"model":"res.partner","ids":[%d],"domain":[],"fields":[{"name":"name","label":"Name"},{"name":"active","label":"Active"},{"name":"company_id/name","label":"Company"}],"context":{},"import_compat":false}`, partnerID))
	csvRec := httptest.NewRecorder()
	csvReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(form.Encode()))
	csvReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(csvRec, csvReq)
	if csvRec.Code != http.StatusOK {
		t.Fatalf("csv response %d %s", csvRec.Code, csvRec.Body.String())
	}
	if got := csvRec.Body.String(); !strings.Contains(got, "Name,Active,Company") || !strings.Contains(got, "Static Partner,True,Nested Export Co") {
		t.Fatalf("csv body = %q", got)
	}

	directRelationForm := url.Values{}
	directRelationForm.Set("data", fmt.Sprintf(`{"model":"res.partner","ids":[%d],"domain":[],"fields":[{"name":"company_id","label":"Company"}],"context":{},"import_compat":false}`, partnerID))
	directRelationCSVRec := httptest.NewRecorder()
	directRelationCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(directRelationForm.Encode()))
	directRelationCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(directRelationCSVRec, directRelationCSVReq)
	if directRelationCSVRec.Code != http.StatusOK {
		t.Fatalf("direct relation csv response %d %s", directRelationCSVRec.Code, directRelationCSVRec.Body.String())
	}
	if got := directRelationCSVRec.Body.String(); !strings.Contains(got, "Company") || !strings.Contains(got, "Nested Export Co") || strings.Contains(got, fmt.Sprintf("\n%d\n", companyID)) {
		t.Fatalf("direct relation csv body = %q", got)
	}

	many2ManyForm := url.Values{}
	many2ManyForm.Set("data", fmt.Sprintf(`{"model":"res.country.group","ids":[%d],"domain":[],"fields":[{"name":"country_ids","label":"Countries"}],"context":{},"import_compat":false}`, countryGroupID))
	many2ManyCSVRec := httptest.NewRecorder()
	many2ManyCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(many2ManyForm.Encode()))
	many2ManyCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(many2ManyCSVRec, many2ManyCSVReq)
	if many2ManyCSVRec.Code != http.StatusOK {
		t.Fatalf("many2many csv response %d %s", many2ManyCSVRec.Code, many2ManyCSVRec.Body.String())
	}
	if got := many2ManyCSVRec.Body.String(); !strings.Contains(got, "Countries") || !strings.Contains(got, "\nBahrain\nQatar\n") || strings.Contains(got, "Bahrain,Qatar") {
		t.Fatalf("many2many csv body = %q", got)
	}

	importMany2ManyForm := url.Values{}
	importMany2ManyForm.Set("data", fmt.Sprintf(`{"model":"res.country.group","ids":[%d],"domain":[],"fields":[{"name":"country_ids","label":"Countries"}],"context":{},"import_compat":true}`, countryGroupID))
	importMany2ManyCSVRec := httptest.NewRecorder()
	importMany2ManyCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(importMany2ManyForm.Encode()))
	importMany2ManyCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(importMany2ManyCSVRec, importMany2ManyCSVReq)
	if importMany2ManyCSVRec.Code != http.StatusOK {
		t.Fatalf("import many2many csv response %d %s", importMany2ManyCSVRec.Code, importMany2ManyCSVRec.Body.String())
	}
	if got := importMany2ManyCSVRec.Body.String(); !strings.Contains(got, "country_ids") || !strings.Contains(got, "Bahrain,Qatar") || strings.Contains(got, "Countries") {
		t.Fatalf("import many2many csv body = %q", got)
	}

	importMany2ManyIDForm := url.Values{}
	importMany2ManyIDForm.Set("data", fmt.Sprintf(`{"model":"res.country.group","ids":[%d],"domain":[],"fields":[{"name":"country_ids","label":"Countries"},{"name":"country_ids/id","label":"Country External IDs"}],"context":{},"import_compat":true}`, countryGroupID))
	importMany2ManyIDCSVRec := httptest.NewRecorder()
	importMany2ManyIDCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(importMany2ManyIDForm.Encode()))
	importMany2ManyIDCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(importMany2ManyIDCSVRec, importMany2ManyIDCSVReq)
	if importMany2ManyIDCSVRec.Code != http.StatusOK {
		t.Fatalf("import many2many id csv response %d %s", importMany2ManyIDCSVRec.Code, importMany2ManyIDCSVRec.Body.String())
	}
	if got := importMany2ManyIDCSVRec.Body.String(); !strings.Contains(got, "country_ids,country_ids/id") ||
		!strings.Contains(got, ",\"x_export.country_bh,x_export.country_qa\"") ||
		strings.Contains(got, "Bahrain") ||
		strings.Contains(got, "Countries") {
		t.Fatalf("import many2many id csv body = %q", got)
	}

	importHeaderForm := url.Values{}
	importHeaderForm.Set("data", fmt.Sprintf(`{"model":"res.partner","ids":[%d],"domain":[],"fields":[{"name":"id","label":"External ID"},{"name":".id","label":"Database ID"},{"name":"company_id/id","label":"Company External ID"}],"context":{},"import_compat":true}`, partnerID))
	importHeaderCSVRec := httptest.NewRecorder()
	importHeaderCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(importHeaderForm.Encode()))
	importHeaderCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(importHeaderCSVRec, importHeaderCSVReq)
	if importHeaderCSVRec.Code != http.StatusOK {
		t.Fatalf("import header csv response %d %s", importHeaderCSVRec.Code, importHeaderCSVRec.Body.String())
	}
	if got := importHeaderCSVRec.Body.String(); !strings.Contains(got, "id,.id,company_id/id") || !strings.Contains(got, fmt.Sprintf("x_export.partner_static,%d,__export__.res_company_%d_", partnerID, companyID)) || strings.Contains(got, "External ID,Database ID") {
		t.Fatalf("import header csv body = %q", got)
	}
	generatedExternalIDRows, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", domain.Equal, "__export__"),
		domain.Cond("model", domain.Equal, "res.company"),
		domain.Cond("res_id", domain.Equal, companyID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(generatedExternalIDRows.IDs()) != 1 {
		t.Fatalf("generated external ids = %#v", generatedExternalIDRows.IDs())
	}

	formulaForm := url.Values{}
	formulaForm.Set("data", fmt.Sprintf(`{"model":"res.partner","ids":[%d],"domain":[],"fields":[{"name":"name","label":"Name"}],"context":{},"import_compat":false}`, formulaPartnerID))
	formulaCSVRec := httptest.NewRecorder()
	formulaCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(formulaForm.Encode()))
	formulaCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(formulaCSVRec, formulaCSVReq)
	if formulaCSVRec.Code != http.StatusOK {
		t.Fatalf("formula csv response %d %s", formulaCSVRec.Code, formulaCSVRec.Body.String())
	}
	if got := formulaCSVRec.Body.String(); !strings.Contains(got, "'=2+2") {
		t.Fatalf("formula csv body = %q", got)
	}

	groupedForm := url.Values{}
	groupedForm.Set("data", fmt.Sprintf(`{"model":"res.partner","ids":[%d,%d],"domain":[],"fields":[{"name":"name","label":"Name"},{"name":"active","label":"Active"}],"groupby":["active"],"context":{},"import_compat":false}`, partnerID, formulaPartnerID))
	groupedCSVRec := httptest.NewRecorder()
	groupedCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(groupedForm.Encode()))
	groupedCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(groupedCSVRec, groupedCSVReq)
	if groupedCSVRec.Code != http.StatusForbidden || !strings.Contains(groupedCSVRec.Body.String(), "grouped data to csv is not supported") {
		t.Fatalf("grouped csv response %d %s", groupedCSVRec.Code, groupedCSVRec.Body.String())
	}

	xlsxRec := httptest.NewRecorder()
	xlsxReq := httptest.NewRequest(http.MethodPost, "/web/export/xlsx", strings.NewReader(form.Encode()))
	xlsxReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(xlsxRec, xlsxReq)
	if xlsxRec.Code != http.StatusOK {
		t.Fatalf("xlsx response %d %s", xlsxRec.Code, xlsxRec.Body.String())
	}
	if contentType := xlsxRec.Header().Get("Content-Type"); !strings.Contains(contentType, "spreadsheetml.sheet") {
		t.Fatalf("xlsx content type = %q", contentType)
	}
	sheetXML := testXLSXSheetXML(t, xlsxRec.Body.Bytes())
	if !strings.Contains(sheetXML, "Nested Export Co") {
		t.Fatalf("xlsx sheet = %s", sheetXML)
	}

	groupedXLSXRec := httptest.NewRecorder()
	groupedXLSXReq := httptest.NewRequest(http.MethodPost, "/web/export/xlsx", strings.NewReader(groupedForm.Encode()))
	groupedXLSXReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(groupedXLSXRec, groupedXLSXReq)
	if groupedXLSXRec.Code != http.StatusOK {
		t.Fatalf("grouped xlsx response %d %s", groupedXLSXRec.Code, groupedXLSXRec.Body.String())
	}
	groupedSheetXML := testXLSXSheetXML(t, groupedXLSXRec.Body.Bytes())
	for _, want := range []string{"Name", "Active", "True (1)", "False (1)", "Static Partner", "=2+2"} {
		if !strings.Contains(groupedSheetXML, want) {
			t.Fatalf("grouped xlsx missing %q:\n%s", want, groupedSheetXML)
		}
	}

	groupedX2ManyForm := url.Values{}
	groupedX2ManyForm.Set("data", fmt.Sprintf(`{"model":"res.country.group","ids":[%d],"domain":[],"fields":[{"name":"name","label":"Name"},{"name":"country_ids","label":"Countries"}],"groupby":["name"],"context":{},"import_compat":false}`, countryGroupID))
	groupedX2ManyXLSXRec := httptest.NewRecorder()
	groupedX2ManyXLSXReq := httptest.NewRequest(http.MethodPost, "/web/export/xlsx", strings.NewReader(groupedX2ManyForm.Encode()))
	groupedX2ManyXLSXReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(groupedX2ManyXLSXRec, groupedX2ManyXLSXReq)
	if groupedX2ManyXLSXRec.Code != http.StatusOK {
		t.Fatalf("grouped x2many xlsx response %d %s", groupedX2ManyXLSXRec.Code, groupedX2ManyXLSXRec.Body.String())
	}
	groupedX2ManySheetXML := testXLSXSheetXML(t, groupedX2ManyXLSXRec.Body.Bytes())
	for _, want := range []string{"Name", "Countries", "GCC (1)", "Bahrain", "Qatar"} {
		if !strings.Contains(groupedX2ManySheetXML, want) {
			t.Fatalf("grouped x2many xlsx missing %q:\n%s", want, groupedX2ManySheetXML)
		}
	}
}

func TestIsExportXMLIDCollision(t *testing.T) {
	if !isExportXMLIDCollision(fmt.Errorf("ir.model.data external id __export__.x already exists")) {
		t.Fatal("expected already-exists collision")
	}
	if !isExportXMLIDCollision(fmt.Errorf("duplicate key value violates unique constraint")) {
		t.Fatal("expected duplicate collision")
	}
	if isExportXMLIDCollision(fmt.Errorf("permission denied")) {
		t.Fatal("unexpected collision")
	}
}

func TestExportGetFieldsSupportsAccountantAnalyticLines(t *testing.T) {
	server := testAccountingDispatchServer(t)
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/export/get_fields", bytes.NewBufferString(`{"jsonrpc":"2.0","id":41,"params":{"model":"account.analytic.line","prefix":"analytic_line_ids","parent_name":"Analytic Lines","parent_field_type":"one2many","import_compat":false,"domain":[]}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_fields response %d %s", rec.Code, rec.Body.String())
	}
	rows := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	var accountField map[string]any
	hasAmount := false
	hasAutoAccount := false
	for _, row := range rows {
		field := row.(map[string]any)
		switch field["id"] {
		case "analytic_line_ids/account_id":
			accountField = field
		case "analytic_line_ids/amount":
			hasAmount = true
		case "analytic_line_ids/auto_account_id":
			hasAutoAccount = true
		}
	}
	if accountField == nil || !hasAmount || !hasAutoAccount {
		t.Fatalf("analytic export fields = %#v", rows)
	}
	if accountField["value"] != "analytic_line_ids/account_id/id" {
		t.Fatalf("account field value = %#v", accountField)
	}
	params := accountField["params"].(map[string]any)
	if params["model"] != "account.analytic.account" || params["prefix"] != "analytic_line_ids/account_id" {
		t.Fatalf("account field params = %#v", params)
	}
}

func TestExportPropertyFieldsMetadataAndValues(t *testing.T) {
	server, ids := testPropertyExportServer(t)
	handler := server.Handler()
	definitionRows, err := server.Env.Model("x.property.record").Browse(ids.record1).Read("record_definition_id")
	if err != nil {
		t.Fatal(err)
	}
	definitionID := int64Value(definitionRows[0]["record_definition_id"])
	if _, err := server.Env.Model("x.property.record").Create(map[string]any{
		"name":                 "Record JSON",
		"record_definition_id": definitionID,
		"properties":           `{"date_prop":"2026-01-25"}`,
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body := `{"jsonrpc":"2.0","id":91,"params":{"model":"x.property.record","import_compat":true,"domain":[]}}`
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/export/get_fields", bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_fields response %d %s", rec.Code, rec.Body.String())
	}
	rows := decodeJSON(t, rec.Body.Bytes())["result"].([]any)
	byID := map[string]map[string]any{}
	for _, row := range rows {
		item := row.(map[string]any)
		byID[stringValue(item["id"])] = item
	}
	if _, ok := byID["properties.separator_prop"]; ok {
		t.Fatalf("separator property should not be exportable: %#v", byID["properties.separator_prop"])
	}
	charField := byID["properties.char_prop"]
	if charField == nil || charField["string"] != "TextType (Definition A)" || charField["field_type"] != "char" {
		t.Fatalf("char property field = %#v", charField)
	}
	m2oField := byID["properties.m2o_prop"]
	if m2oField == nil || m2oField["relation"] != "res.partner" || m2oField["children"] != true || m2oField["value"] != "properties.m2o_prop/id" {
		t.Fatalf("many2one property field = %#v", m2oField)
	}
	if byID["properties.bool_prop"] == nil || byID["properties.tags_prop"] == nil || byID["properties.m2m_prop"] == nil {
		t.Fatalf("missing definition B property fields: %#v", byID)
	}

	filtered := httptest.NewRecorder()
	filteredBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":92,"params":{"model":"x.property.record","import_compat":true,"domain":[["id","in",[%d,%d]]]}}`, ids.record1, ids.record2)
	handler.ServeHTTP(filtered, httptest.NewRequest(http.MethodPost, "/web/export/get_fields", bytes.NewBufferString(filteredBody)))
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered get_fields response %d %s", filtered.Code, filtered.Body.String())
	}
	filteredRows := decodeJSON(t, filtered.Body.Bytes())["result"].([]any)
	filteredIDs := map[string]bool{}
	for _, row := range filteredRows {
		filteredIDs[stringValue(row.(map[string]any)["id"])] = true
	}
	if !filteredIDs["properties.char_prop"] || !filteredIDs["properties.m2o_prop"] || filteredIDs["properties.bool_prop"] || filteredIDs["properties.m2m_prop"] {
		t.Fatalf("filtered property fields = %#v", filteredIDs)
	}
	invalidSearch := httptest.NewRecorder()
	handler.ServeHTTP(invalidSearch, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(`{"model":"x.property.record","method":"search_count","args":[[["properties.date_prop.month_number","=",1]]]}`)))
	if invalidSearch.Code == http.StatusOK || !strings.Contains(invalidSearch.Body.String(), "unsupported property path") {
		t.Fatalf("invalid property date-part search response %d %s", invalidSearch.Code, invalidSearch.Body.String())
	}

	for _, name := range []string{"properties.char_prop", "properties.m2o_prop/name"} {
		if _, err := server.Env.Model("ir.exports.line").Create(map[string]any{"name": name, "export_id": int64(77)}); err != nil {
			t.Fatal(err)
		}
	}
	namelist := httptest.NewRecorder()
	handler.ServeHTTP(namelist, httptest.NewRequest(http.MethodPost, "/web/export/namelist", bytes.NewBufferString(`{"jsonrpc":"2.0","id":93,"params":{"model":"x.property.record","export_id":77}}`)))
	if namelist.Code != http.StatusOK {
		t.Fatalf("namelist response %d %s", namelist.Code, namelist.Body.String())
	}
	namelistRows := decodeJSON(t, namelist.Body.Bytes())["result"].([]any)
	gotNames := []string{}
	gotLabels := []string{}
	for _, row := range namelistRows {
		item := row.(map[string]any)
		gotNames = append(gotNames, stringValue(item["id"]))
		gotLabels = append(gotLabels, stringValue(item["string"]))
	}
	if !reflect.DeepEqual(gotNames, []string{"properties.char_prop", "properties.m2o_prop/name"}) ||
		!reflect.DeepEqual(gotLabels, []string{"TextType (Definition A)", "many2one (Definition A)/name"}) {
		t.Fatalf("namelist names=%#v labels=%#v", gotNames, gotLabels)
	}

	form := url.Values{}
	form.Set("data", fmt.Sprintf(`{"model":"x.property.record","ids":[%d,%d,%d,%d],"domain":[],"fields":[{"name":"properties.char_prop","label":"Text"},{"name":"properties.selection_prop","label":"Selection"},{"name":"properties.tags_prop","label":"Tags"},{"name":"properties.m2o_prop","label":"Partner"},{"name":"properties.m2m_prop","label":"Partners"}],"context":{},"import_compat":true}`, ids.record1, ids.record2, ids.record3, ids.record4))
	csvRec := httptest.NewRecorder()
	csvReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(form.Encode()))
	csvReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(csvRec, csvReq)
	if csvRec.Code != http.StatusOK {
		t.Fatalf("csv response %d %s", csvRec.Code, csvRec.Body.String())
	}
	csvRows, err := csv.NewReader(strings.NewReader(csvRec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v body=%q", err, csvRec.Body.String())
	}
	wantRows := [][]string{
		{"properties.char_prop", "properties.selection_prop", "properties.tags_prop", "properties.m2o_prop", "properties.m2m_prop"},
		{"Not the default", "bbbbbbb", "", "", ""},
		{"Def", "", "", "Name Partner 1", ""},
		{"", "", "AA,BB", "", ""},
		{"", "", "", "", "Name Partner 1,Name Partner 2"},
	}
	if !reflect.DeepEqual(csvRows, wantRows) {
		t.Fatalf("csv rows = %#v", csvRows)
	}

	m2mForm := url.Values{}
	m2mForm.Set("data", fmt.Sprintf(`{"model":"x.property.record","ids":[%d],"domain":[],"fields":[{"name":"properties.m2m_prop","label":"M2M"}],"context":{},"import_compat":false}`, ids.record4))
	m2mCSVRec := httptest.NewRecorder()
	m2mCSVReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(m2mForm.Encode()))
	m2mCSVReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(m2mCSVRec, m2mCSVReq)
	if m2mCSVRec.Code != http.StatusOK {
		t.Fatalf("m2m csv response %d %s", m2mCSVRec.Code, m2mCSVRec.Body.String())
	}
	m2mRows, err := csv.NewReader(strings.NewReader(m2mCSVRec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("m2m csv parse: %v body=%q", err, m2mCSVRec.Body.String())
	}
	if !reflect.DeepEqual(m2mRows, [][]string{{"M2M"}, {"Name Partner 1"}, {"Name Partner 2"}}) {
		t.Fatalf("m2m csv rows = %#v", m2mRows)
	}
}

func TestExportGroupedXLSXPropertyGroupBy(t *testing.T) {
	server, ids := testPropertyExportServer(t)

	content, _, err := exportXLSXBytes(server.Env, exportDownloadRequest{
		Model:   "x.property.record",
		IDs:     []int64{ids.record1, ids.record2},
		GroupBy: []string{"properties.date_prop:month"},
		Fields: []exportFieldRef{
			{Name: "name", Label: "Name"},
			{Name: "properties.date_prop", Label: "Property Date"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{"January 2026 (2)", "Record 1", "Record 2", "2026-01-10", "2026-01-20"} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped property date xlsx missing %q:\n%s", want, sheetXML)
		}
	}

	content, _, err = exportXLSXBytes(server.Env, exportDownloadRequest{
		Model:   "x.property.record",
		IDs:     []int64{ids.record1, ids.record2},
		GroupBy: []string{"properties.date_prop:week"},
		Fields: []exportFieldRef{
			{Name: "name", Label: "Name"},
			{Name: "properties.date_prop", Label: "Property Date"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML = testXLSXSheetXML(t, content)
	for _, want := range []string{"W2 2026 (1)", "W4 2026 (1)"} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped property week xlsx missing %q:\n%s", want, sheetXML)
		}
	}

	_, _, err = exportXLSXBytes(server.Env, exportDownloadRequest{
		Model:   "x.property.record",
		IDs:     []int64{ids.record1, ids.record2},
		GroupBy: []string{"properties.date_prop:month_number"},
		Fields: []exportFieldRef{
			{Name: "name", Label: "Name"},
			{Name: "properties.date_prop", Label: "Property Date"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `not supported for property field`) {
		t.Fatalf("grouped property numeric date xlsx error = %v", err)
	}

	content, _, err = exportXLSXBytes(server.Env, exportDownloadRequest{
		Model:   "x.property.record",
		IDs:     []int64{ids.record1, ids.record2},
		GroupBy: []string{"properties.selection_prop"},
		Fields: []exportFieldRef{
			{Name: "name", Label: "Name"},
			{Name: "properties.selection_prop", Label: "Selection"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML = testXLSXSheetXML(t, content)
	for _, want := range []string{"bbbbbbb (1)", "Undefined (1)", "Record 1", "Record 2"} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped property selection xlsx missing %q:\n%s", want, sheetXML)
		}
	}
}

func TestWebDatasetPropertyReadGroupFormatting(t *testing.T) {
	server, ids := testPropertyExportServer(t)
	handler := server.Handler()

	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"x.property.record","method":"read_group","args":[[["id","in",[%d,%d]]],["__count"],["properties.date_prop:month"]]}`, ids.record1, ids.record2))
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	dateRow := httpReadGroupFindScalar(rows, "properties.date_prop:month", "January 2026")
	if int64Value(dateRow["properties.date_prop_count"]) != 2 {
		t.Fatalf("property date read_group rows = %#v", rows)
	}
	if _, leaksExtra := dateRow["__extra_domain"]; leaksExtra {
		t.Fatalf("legacy read_group leaked __extra_domain = %#v", dateRow)
	}
	groupRange, ok := dateRow["__range"].(map[string]any)
	if !ok {
		t.Fatalf("property date range = %#v", dateRow["__range"])
	}
	monthRange, ok := groupRange["properties.date_prop:month"].(map[string]any)
	if !ok || monthRange["from"] != "2026-01-01" || monthRange["to"] != "2026-02-01" {
		t.Fatalf("property date month range = %#v", groupRange["properties.date_prop:month"])
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"x.property.record","method":"formatted_read_group","args":[[["id","in",[%d,%d]]],["properties.selection_prop"],["__count"]]}`, ids.record1, ids.record2))
	rows = nil
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	selectionRow := httpReadGroupFindScalar(rows, "properties.selection_prop", "selection_2")
	selectionFalse := httpReadGroupFindScalar(rows, "properties.selection_prop", false)
	if int64Value(selectionRow["__count"]) != 1 || int64Value(selectionFalse["__count"]) != 1 {
		t.Fatalf("property selection formatted rows = %#v", rows)
	}
	if _, leaksDomain := selectionRow["__domain"]; leaksDomain {
		t.Fatalf("formatted_read_group leaked __domain = %#v", selectionRow)
	}
	if _, ok := selectionFalse["__extra_domain"].([]any); !ok {
		t.Fatalf("selection false extra domain = %#v", selectionFalse["__extra_domain"])
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"x.property.record","method":"formatted_read_group","args":[[["id","in",[%d,%d]]],["properties.m2o_prop"],["__count"]]}`, ids.record1, ids.record2))
	rows = nil
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	m2oRow := httpReadGroupFindPair(rows, "properties.m2o_prop", 1)
	m2oFalse := httpReadGroupFindScalar(rows, "properties.m2o_prop", false)
	m2oValue, ok := m2oRow["properties.m2o_prop"].([]any)
	if !ok || len(m2oValue) != 2 || m2oValue[1] != "Name Partner 1" || int64Value(m2oRow["__count"]) != 1 || int64Value(m2oFalse["__count"]) != 1 {
		t.Fatalf("property m2o formatted rows = %#v", rows)
	}

	body = postCallKW(t, handler, fmt.Sprintf(`{"model":"x.property.record","method":"web_read_group","kwargs":{"domain":[["id","in",[%d,%d]]],"groupby":["properties.tags_prop"],"aggregates":["__count"]}}`, ids.record3, ids.record4))
	var grouped map[string]any
	if err := json.Unmarshal([]byte(body), &grouped); err != nil {
		t.Fatal(err)
	}
	groups, ok := grouped["groups"].([]any)
	if !ok || grouped["length"] != float64(3) {
		t.Fatalf("property tags web_read_group result = %#v", grouped)
	}
	webRows := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		row, ok := group.(map[string]any)
		if !ok {
			t.Fatalf("property tag group = %#v", group)
		}
		webRows = append(webRows, row)
	}
	tagAA := httpReadGroupFindPair(webRows, "properties.tags_prop", "aa")
	tagBB := httpReadGroupFindPair(webRows, "properties.tags_prop", "bb")
	tagFalse := httpReadGroupFindScalar(webRows, "properties.tags_prop", false)
	if int64Value(tagAA["__count"]) != 1 || int64Value(tagBB["__count"]) != 1 || int64Value(tagFalse["__count"]) != 1 {
		t.Fatalf("property tag web rows = %#v", webRows)
	}
	tagValue, ok := tagAA["properties.tags_prop"].([]any)
	if !ok || len(tagValue) != 3 || tagValue[1] != "AA" || int64Value(tagValue[2]) != 5 {
		t.Fatalf("property tag value = %#v", tagAA["properties.tags_prop"])
	}
}

func httpReadGroupFindScalar(rows []map[string]any, key string, value any) map[string]any {
	for _, row := range rows {
		if reflect.DeepEqual(row[key], value) {
			return row
		}
	}
	return map[string]any{}
}

func httpReadGroupFindPair(rows []map[string]any, key string, id any) map[string]any {
	for _, row := range rows {
		pair, ok := row[key].([]any)
		if !ok || len(pair) == 0 {
			continue
		}
		if reflect.DeepEqual(pair[0], id) {
			return row
		}
		leftID := int64Value(pair[0])
		rightID := int64Value(id)
		if leftID != 0 && rightID != 0 && leftID == rightID {
			return row
		}
	}
	return map[string]any{}
}

func httpReadGroupIDsEqual(value any, want ...int64) bool {
	values, ok := value.([]any)
	if !ok || len(values) != len(want) {
		return false
	}
	for index, item := range values {
		if int64Value(item) != want[index] {
			return false
		}
	}
	return true
}

func TestExportAccountantAnalyticLineValues(t *testing.T) {
	server := testAccountingDispatchServer(t)
	env := server.Env
	analyticAccountID, err := env.Model("account.analytic.account").Create(map[string]any{"name": "Consulting"})
	if err != nil {
		t.Fatal(err)
	}
	moveLineID, err := env.Model("account.move.line").Create(map[string]any{"name": "Service line", "debit": float64(10), "credit": float64(0), "balance": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	analyticLineID, err := env.Model("account.analytic.line").Create(map[string]any{"name": "Analytic service", "move_line_id": moveLineID, "account_id": analyticAccountID, "amount": float64(42.5)})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move.line").Browse(moveLineID).Write(map[string]any{"analytic_line_ids": []int64{analyticLineID}}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	form.Set("data", fmt.Sprintf(`{"model":"account.move.line","ids":[%d],"domain":[],"fields":[{"name":"name","label":"Label"},{"name":"analytic_line_ids/account_id/id","label":"Analytic Account ID"},{"name":"analytic_line_ids/amount","label":"Analytic Amount"}],"context":{},"import_compat":true}`, moveLineID))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv response %d %s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, "name,analytic_line_ids/account_id/id,analytic_line_ids/amount") || !strings.Contains(got, fmt.Sprintf("Service line,__export__.account_analytic_account_%d_", analyticAccountID)) || !strings.Contains(got, ",42.5") {
		t.Fatalf("csv body = %q", got)
	}
	secondAnalyticLineID, err := env.Model("account.analytic.line").Create(map[string]any{"name": "Analytic follow-up", "move_line_id": moveLineID, "account_id": analyticAccountID, "amount": float64(7.5)})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Model("account.move.line").Browse(moveLineID).Write(map[string]any{"analytic_line_ids": []int64{analyticLineID, secondAnalyticLineID}}); err != nil {
		t.Fatal(err)
	}
	normalForm := url.Values{}
	normalForm.Set("data", fmt.Sprintf(`{"model":"account.move.line","ids":[%d],"domain":[],"fields":[{"name":"name","label":"Label"},{"name":"analytic_line_ids","label":"Analytic Lines"},{"name":"analytic_line_ids/amount","label":"Analytic Amount"}],"context":{},"import_compat":false}`, moveLineID))
	normalRec := httptest.NewRecorder()
	normalReq := httptest.NewRequest(http.MethodPost, "/web/export/csv", strings.NewReader(normalForm.Encode()))
	normalReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(normalRec, normalReq)
	if normalRec.Code != http.StatusOK {
		t.Fatalf("normal csv response %d %s", normalRec.Code, normalRec.Body.String())
	}
	if got := normalRec.Body.String(); !strings.Contains(got, "Label,Analytic Lines,Analytic Amount") ||
		!strings.Contains(got, "Service line,Analytic service,42.5") ||
		!strings.Contains(got, ",Analytic follow-up,7.5") {
		t.Fatalf("normal csv body = %q", got)
	}
}

func TestExportXLSXNativeCellTypesAndStyles(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.sample", "x_xlsx_sample")
	sample.AddField(field.New("name", field.Char))
	sample.AddField(field.New("active", field.Bool))
	sample.AddField(field.New("quantity", field.Int))
	sample.AddField(field.New("amount", field.Float))
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("moment_value", field.DateTime))
	sample.AddField(field.New("payload", field.Binary))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	id, err := env.Model("x.xlsx.sample").Create(map[string]any{
		"name":         "Typed row",
		"active":       true,
		"quantity":     int64(7),
		"amount":       float64(42.5),
		"date_value":   "2026-06-18",
		"moment_value": "2026-06-18 12:34:56",
		"payload":      []byte("payload text"),
	})
	if err != nil {
		t.Fatal(err)
	}
	content, filename, err := exportXLSXBytes(env, exportDownloadRequest{
		Model: "x.xlsx.sample",
		IDs:   []int64{id},
		Fields: []exportFieldRef{
			{Name: "name", Label: "Name"},
			{Name: "active", Label: "Active"},
			{Name: "quantity", Label: "Quantity"},
			{Name: "amount", Label: "Amount"},
			{Name: "date_value", Label: "Date"},
			{Name: "moment_value", Label: "Moment"},
			{Name: "payload", Label: "Payload"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filename != "x_xlsx_sample.xlsx" {
		t.Fatalf("filename = %q", filename)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`<c r="A1" s="1" t="inlineStr"><is><t>Name</t></is></c>`,
		`<c r="B2" s="0" t="b"><v>1</v></c>`,
		`<c r="C2" s="0"><v>7</v></c>`,
		`<c r="D2" s="4"><v>42.5</v></c>`,
		`<c r="E2" s="2"><v>`,
		`<c r="F2" s="3"><v>`,
		`<c r="G2" s="0" t="inlineStr"><is><t>payload text</t></is></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("xlsx sheet missing %q:\n%s", want, sheetXML)
		}
	}
	stylesXML := testXLSXFileXML(t, content, "xl/styles.xml")
	for _, want := range []string{`formatCode="yyyy-mm-dd"`, `formatCode="yyyy-mm-dd hh:mm:ss"`, `formatCode="#,##0.00"`, `<b/>`, `wrapText="1"`} {
		if !strings.Contains(stylesXML, want) {
			t.Fatalf("xlsx styles missing %q:\n%s", want, stylesXML)
		}
	}
}

func TestExportXLSXInvalidBinaryErrorNamesColumn(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.binary", "x_xlsx_binary")
	sample.AddField(field.New("payload", field.Binary))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	id, err := env.Model("x.xlsx.binary").Create(map[string]any{"payload": []byte{0xff, 0xfe}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = exportXLSXBytes(env, exportDownloadRequest{
		Model: "x.xlsx.binary",
		IDs:   []int64{id},
		Fields: []exportFieldRef{
			{Name: "payload", Label: "Payload"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Binary fields can not be exported to Excel unless their content is base64-encoded. That does not seem to be the case for Payload.") {
		t.Fatalf("xlsx binary error = %v", err)
	}
}

func TestExportXLSXCurrencyDecimalPrecisionStyle(t *testing.T) {
	reg := record.NewRegistry()
	for _, item := range internalbase.Models() {
		if err := reg.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	sample := model.New("x.xlsx.money", "x_xlsx_money")
	sample.AddField(field.New("amount", field.Decimal))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := env.Model("res.currency").Create(map[string]any{"name": "TST", "decimal_places": int64(3), "active": true}); err != nil {
		t.Fatal(err)
	}
	id, err := env.Model("x.xlsx.money").Create(map[string]any{"amount": float64(12.345)})
	if err != nil {
		t.Fatal(err)
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:  "x.xlsx.money",
		IDs:    []int64{id},
		Fields: []exportFieldRef{{Name: "amount", Label: "Amount"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stylesXML := testXLSXFileXML(t, content, "xl/styles.xml")
	if !strings.Contains(stylesXML, `formatCode="#,##0.000"`) {
		t.Fatalf("styles xml = %s", stylesXML)
	}
}

func TestExportXLSXRowLimitMatchesOdoo(t *testing.T) {
	if err := validateXLSXRowLimit(xlsxMaxRows); err != nil {
		t.Fatalf("row limit at cap = %v", err)
	}
	err := validateXLSXRowLimit(xlsxMaxRows + 1)
	if err == nil || !strings.Contains(err.Error(), "There are too many rows (1048577 rows, limit: 1048576)") {
		t.Fatalf("row limit error = %v", err)
	}
}

func TestExportGroupedXLSXDateMonthAndTypedAggregates(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.group", "x_xlsx_group")
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("int_sum", field.Int).WithAggregator("sum"))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	sample.AddField(field.New("active", field.Bool).WithAggregator("bool_or"))
	sample.AddField(field.New("date_max", field.Date).WithAggregator("max"))
	sample.AddField(field.New("moment_max", field.DateTime).WithAggregator("max"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	for _, values := range []map[string]any{
		{"date_value": "2026-01-10", "int_sum": int64(3), "amount": float64(10.25), "active": true, "date_max": "2026-01-10", "moment_max": "2026-01-10 08:00:00"},
		{"date_value": "2026-01-20", "int_sum": int64(4), "amount": float64(2.75), "active": false, "date_max": "2026-01-20", "moment_max": "2026-01-20 09:30:00"},
		{"date_value": "2026-02-05", "int_sum": int64(5), "amount": float64(1.5), "active": false, "date_max": "2026-02-05", "moment_max": "2026-02-05 10:45:00"},
	} {
		if _, err := env.Model("x.xlsx.group").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.group",
		Context: map[string]any{"active_test": false},
		GroupBy: []string{"date_value:month"},
		Fields: []exportFieldRef{
			{Name: "date_value", Label: "Date"},
			{Name: "int_sum", Label: "Integer Sum"},
			{Name: "amount", Label: "Amount"},
			{Name: "active", Label: "Active"},
			{Name: "date_max", Label: "Date Max"},
			{Name: "moment_max", Label: "Moment Max"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`<c r="A2" s="5" t="inlineStr"><is><t>January 2026 (2)</t></is></c>`,
		`<c r="B2" s="5"><v>7</v></c>`,
		`<c r="C2" s="6"><v>13</v></c>`,
		`<c r="D2" s="5" t="b"><v>1</v></c>`,
		`<c r="E2" s="2"><v>`,
		`<c r="F2" s="3"><v>`,
		`<c r="A3" s="2"><v>`,
		`<c r="A5" s="5" t="inlineStr"><is><t>February 2026 (1)</t></is></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped xlsx sheet missing %q:\n%s", want, sheetXML)
		}
	}
}

func TestExportGroupedXLSXNumericDateParts(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.date.part", "x_xlsx_date_part")
	sample.AddField(field.New("date_value", field.Date))
	sample.AddField(field.New("moment_value", field.DateTime))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	for _, values := range []map[string]any{
		{"date_value": "2022-01-30", "moment_value": "2022-01-30 13:54:14", "amount": float64(2)},
		{"date_value": "2022-01-31", "moment_value": "2022-01-31 15:55:14", "amount": float64(3)},
		{"date_value": "2022-10-15", "moment_value": "2022-10-15 01:02:03", "amount": float64(5)},
		{"amount": float64(7)},
	} {
		if _, err := env.Model("x.xlsx.date.part").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.date.part",
		GroupBy: []string{"date_value:month_number"},
		Fields: []exportFieldRef{
			{Name: "date_value", Label: "Date"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`<c r="A2" s="5" t="inlineStr"><is><t>1 (2)</t></is></c>`,
		`<c r="B2" s="6"><v>5</v></c>`,
		`<c r="A5" s="5" t="inlineStr"><is><t>10 (1)</t></is></c>`,
		`<c r="B5" s="6"><v>5</v></c>`,
		`<c r="A7" s="5" t="inlineStr"><is><t>Undefined (1)</t></is></c>`,
		`<c r="B7" s="6"><v>7</v></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped numeric date-part xlsx missing %q:\n%s", want, sheetXML)
		}
	}

	content, _, err = exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.date.part",
		GroupBy: []string{"date_value:day_of_week"},
		Fields: []exportFieldRef{
			{Name: "date_value", Label: "Date"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML = testXLSXSheetXML(t, content)
	for _, want := range []string{
		`<c r="A2" s="5" t="inlineStr"><is><t>0 (1)</t></is></c>`,
		`<c r="A4" s="5" t="inlineStr"><is><t>1 (1)</t></is></c>`,
		`<c r="A6" s="5" t="inlineStr"><is><t>6 (1)</t></is></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped numeric weekday xlsx missing %q:\n%s", want, sheetXML)
		}
	}

	tzReg := record.NewRegistry()
	tzSample := model.New("x.xlsx.date.part.tz", "x_xlsx_date_part_tz")
	tzSample.AddField(field.New("moment_value", field.DateTime))
	tzSample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := tzReg.Register(tzSample); err != nil {
		t.Fatal(err)
	}
	tzEnv := record.NewEnv(tzReg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := tzEnv.Model("x.xlsx.date.part.tz").Create(map[string]any{"moment_value": "2023-02-05 23:55:00", "amount": float64(9)}); err != nil {
		t.Fatal(err)
	}
	content, _, err = exportXLSXBytes(tzEnv, exportDownloadRequest{
		Model:   "x.xlsx.date.part.tz",
		Context: map[string]any{"tz": "Pacific/Auckland"},
		GroupBy: []string{"moment_value:iso_week_number"},
		Fields: []exportFieldRef{
			{Name: "moment_value", Label: "Moment"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML = testXLSXSheetXML(t, content)
	for _, want := range []string{
		`<c r="A2" s="5" t="inlineStr"><is><t>6 (1)</t></is></c>`,
		`<c r="B2" s="6"><v>9</v></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped numeric timezone xlsx missing %q:\n%s", want, sheetXML)
		}
	}
}

func TestExportGroupedXLSXDateTimeWeek(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.week", "x_xlsx_week")
	sample.AddField(field.New("moment_value", field.DateTime))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	for _, values := range []map[string]any{
		{"moment_value": "2026-01-05 08:00:00", "amount": float64(1.25)},
		{"moment_value": "2026-01-07 09:00:00", "amount": float64(2.75)},
		{"moment_value": "2026-01-12 10:00:00", "amount": float64(3.5)},
	} {
		if _, err := env.Model("x.xlsx.week").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.week",
		GroupBy: []string{"moment_value:week"},
		Fields: []exportFieldRef{
			{Name: "moment_value", Label: "Moment"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`W2 2026 (2)`,
		`<c r="B2" s="6"><v>4</v></c>`,
		`W3 2026 (1)`,
		`<c r="A3" s="3"><v>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped week xlsx missing %q:\n%s", want, sheetXML)
		}
	}
}

func TestExportGroupedXLSXDateTimeWeekUsesLanguageWeekStart(t *testing.T) {
	reg := record.NewRegistry()
	lang := model.New("res.lang", "res_lang")
	lang.AddField(field.New("code", field.Char))
	lang.AddField(field.New("week_start", field.Char))
	if err := reg.Register(lang); err != nil {
		t.Fatal(err)
	}
	sample := model.New("x.xlsx.week.lang", "x_xlsx_week_lang")
	sample.AddField(field.New("moment_value", field.DateTime))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	if _, err := env.Model("res.lang").Create(map[string]any{"code": "en_US", "week_start": "7"}); err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"moment_value": "2022-01-29 08:00:00", "amount": float64(1)},
		{"moment_value": "2022-01-30 08:00:00", "amount": float64(2)},
		{"moment_value": "2022-01-31 08:00:00", "amount": float64(3)},
	} {
		if _, err := env.Model("x.xlsx.week.lang").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.week.lang",
		Context: map[string]any{"lang": "en_US"},
		GroupBy: []string{"moment_value:week"},
		Fields: []exportFieldRef{
			{Name: "moment_value", Label: "Moment"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`W5 2022 (1)`,
		`<c r="B2" s="6"><v>1</v></c>`,
		`W6 2022 (2)`,
		`<c r="B4" s="6"><v>5</v></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped language week xlsx missing %q:\n%s", want, sheetXML)
		}
	}
}

func TestExportGroupedXLSXDateTimeUsesContextTimezone(t *testing.T) {
	reg := record.NewRegistry()
	sample := model.New("x.xlsx.tz", "x_xlsx_tz")
	sample.AddField(field.New("moment_value", field.DateTime))
	sample.AddField(field.New("amount", field.Float).WithAggregator("sum"))
	if err := reg.Register(sample); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	for _, values := range []map[string]any{
		{"moment_value": "2026-01-01 20:30:00", "amount": float64(1.25)},
		{"moment_value": "2026-01-01 22:30:00", "amount": float64(2.75)},
	} {
		if _, err := env.Model("x.xlsx.tz").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	content, _, err := exportXLSXBytes(env, exportDownloadRequest{
		Model:   "x.xlsx.tz",
		Context: map[string]any{"tz": "Asia/Bahrain"},
		GroupBy: []string{"moment_value:day"},
		Fields: []exportFieldRef{
			{Name: "moment_value", Label: "Moment"},
			{Name: "amount", Label: "Amount"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheetXML := testXLSXSheetXML(t, content)
	for _, want := range []string{
		`January 1, 2026 (1)`,
		`<c r="B2" s="6"><v>1.25</v></c>`,
		`January 2, 2026 (1)`,
		`<c r="B4" s="6"><v>2.75</v></c>`,
	} {
		if !strings.Contains(sheetXML, want) {
			t.Fatalf("grouped timezone xlsx missing %q:\n%s", want, sheetXML)
		}
	}
}

func testXLSXSheetXML(t *testing.T, content []byte) string {
	return testXLSXFileXML(t, content, "xl/worksheets/sheet1.xml")
}

func testXLSXFileXML(t *testing.T, content []byte, name string) string {
	t.Helper()
	zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("xlsx zip error = %v", err)
	}
	for _, file := range zipReader.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open sheet: %v", err)
		}
		defer reader.Close()
		raw, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read sheet: %v", err)
		}
		return string(raw)
	}
	t.Fatalf("xlsx files = %#v", zipReader.File)
	return ""
}

func TestHTTPViewsReturnComposedInheritedArch(t *testing.T) {
	server := testServer(t)
	server.Views = view.NewRegistry()
	if err := server.Views.AddWithID(view.View{ID: 50, Name: "Partner Root Form", Model: "res.partner", Type: view.Form, Arch: `<form><group><field name="name"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := server.Views.AddWithID(view.View{ID: 51, Name: "Partner Email Extension", Model: "res.partner", Type: view.Form, InheritID: 50, Mode: "extension", Priority: 10, Arch: `<xpath expr="//field[@name='name']" position="after"><field name="email"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":16,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[50,"form"]],"options":{}}}}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	formArch := payload["result"].(map[string]any)["views"].(map[string]any)["form"].(map[string]any)["arch"].(string)
	assertComposedPartnerArch(t, formArch)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/view/load?model=res.partner", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("view/load response %d %s", rec.Code, rec.Body.String())
	}
	viewLoadArch := viewLoadArchByID(t, rec.Body.Bytes(), 50)
	assertComposedPartnerArch(t, viewLoadArch)
}

func TestCallKWGetViewsRejectsExplicitRestrictedView(t *testing.T) {
	server := testServer(t)
	if err := server.Views.AddWithID(view.View{
		ID:     11,
		Name:   "Restricted Partner Form",
		Model:  "res.partner",
		Type:   view.Form,
		Arch:   `<form><field name="name"/></form>`,
		Groups: []int64{2},
	}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":14,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[11,"form"]],"options":{}}}}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "current user groups") {
		t.Fatalf("restricted get_views response %d %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":15,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[false,"form"]],"options":{}}}}`)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("default get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	formView := payload["result"].(map[string]any)["views"].(map[string]any)["form"].(map[string]any)
	if formView["id"] != float64(10) {
		t.Fatalf("default form view = %#v", formView)
	}
}

func TestCallKWGetViewsRejectsExplicitRestrictedInheritedView(t *testing.T) {
	server := testServer(t)
	if err := server.Views.AddWithID(view.View{
		ID:        60,
		Name:      "Restricted Partner Extension",
		Model:     "res.partner",
		Type:      view.Form,
		InheritID: 10,
		Mode:      "extension",
		Arch:      `<xpath expr="//field[@name='name']" position="after"><field name="email"/></xpath>`,
		Groups:    []int64{2},
	}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":17,"params":{"model":"res.partner","method":"get_views","kwargs":{"views":[[60,"form"]],"options":{}}}}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/res.partner/get_views", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "current user groups") {
		t.Fatalf("restricted inherited get_views response %d %s", rec.Code, rec.Body.String())
	}
}

func TestCallKWGetViewsPrunesXMLNodeGroups(t *testing.T) {
	server, ids := testViewGroupXMLIDServer(t)
	if err := server.Views.AddWithID(view.View{
		ID:    30,
		Name:  "Partner Grouped Form",
		Model: "res.partner",
		Type:  view.Form,
		Arch:  `<form><sheet><field name="name"/><field name="system_only" groups="base.group_system"/><field name="user_only" groups="base.group_user,!base.group_system"/><t groups="base.group_system"><field name="lifted_system"/></t></sheet></form>`,
	}); err != nil {
		t.Fatal(err)
	}
	archFor := func(groupIDs ...int64) string {
		t.Helper()
		local := server
		ctx := local.Env.Context()
		ctx.Values = cloneContextValues(ctx.Values)
		if len(groupIDs) > 0 {
			ctx.Values["group_ids"] = append([]int64(nil), groupIDs...)
		} else {
			delete(ctx.Values, "group_ids")
		}
		local.Env = local.Env.WithContext(ctx)
		body := postCallKW(t, local.Handler(), `{"model":"res.partner","method":"get_views","kwargs":{"views":[[30,"form"]],"options":{}}}`)
		payload := decodeJSON(t, []byte(body))
		return payload["views"].(map[string]any)["form"].(map[string]any)["arch"].(string)
	}

	publicArch := archFor()
	if strings.Contains(publicArch, "system_only") || strings.Contains(publicArch, "user_only") || strings.Contains(publicArch, "lifted_system") || strings.Contains(publicArch, "groups=") {
		t.Fatalf("public arch = %s", publicArch)
	}
	userArch := archFor(ids["base.group_user"])
	if !strings.Contains(userArch, "user_only") || strings.Contains(userArch, "system_only") || strings.Contains(userArch, "lifted_system") || strings.Contains(userArch, "groups=") {
		t.Fatalf("user arch = %s", userArch)
	}
	systemArch := archFor(ids["base.group_system"])
	if !strings.Contains(systemArch, "system_only") || !strings.Contains(systemArch, "lifted_system") || strings.Contains(systemArch, "user_only") || strings.Contains(systemArch, "<t") || strings.Contains(systemArch, "groups=") {
		t.Fatalf("system arch = %s", systemArch)
	}
}

func TestCallKWGetViewsInjectsWorkflowViewIDForAdvancedLists(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	viewReg := view.NewRegistry()
	if err := viewReg.AddWithID(view.View{ID: 41, Name: "PO List", Model: "purchase.order", Type: view.List, Arch: `<list><field name="name"/></list>`}); err != nil {
		t.Fatal(err)
	}
	if err := viewReg.AddWithID(view.View{ID: 42, Name: "PO Kanban", Model: "purchase.order", Type: view.Kanban, Arch: `<kanban><field name="name"/></kanban>`}); err != nil {
		t.Fatal(err)
	}
	server.Views = viewReg
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":13,"params":{"model":"purchase.order","method":"get_views","kwargs":{"views":[[41,"list"],[42,"kanban"],[false,"form"]],"options":{}}}}`)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	views := result["views"].(map[string]any)
	listArch := views["list"].(map[string]any)["arch"].(string)
	kanbanArch := views["kanban"].(map[string]any)["arch"].(string)
	formArch := views["form"].(map[string]any)["arch"].(string)
	if !strings.Contains(listArch, `field name="workflow_view_id" invisible="1" column_invisible="1" readonly="1"`) {
		t.Fatalf("list arch = %s", listArch)
	}
	if !strings.Contains(kanbanArch, `field name="workflow_view_id" invisible="1" column_invisible="1" readonly="1"`) {
		t.Fatalf("kanban arch = %s", kanbanArch)
	}
	if strings.Contains(formArch, "workflow_view_id") {
		t.Fatalf("form arch = %s", formArch)
	}
}

func TestCallKWGetViewsAppliesOIWorkflowArchMutation(t *testing.T) {
	server := testWorkflowDispatchServerWithDefaultReadonly(t, "state != 'draft' and not user_can_approve")
	viewReg := view.NewRegistry()
	if err := viewReg.AddWithID(view.View{ID: 71, Name: "PO Form", Model: "purchase.order", Type: view.Form, Arch: `<form><header><field name="state"/></header><sheet><group><field name="name"/><field name="state"/><field name="amount_total" readonly="1"/><field name="last_state_update"/><field name="workflow_node_id" invisible="0"/></group></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := viewReg.AddWithID(view.View{ID: 72, Name: "PO List", Model: "purchase.order", Type: view.List, Arch: `<list><field name="name"/></list>`}); err != nil {
		t.Fatal(err)
	}
	server.Views = viewReg
	settingsID, err := server.Env.Model(internalworkflow.ModelSettings).Create(map[string]any{
		"name":                "PO Approval",
		"model":               "purchase.order",
		"active":              true,
		"state_field":         "state",
		"draft_state":         "draft",
		"approved_state":      "approved",
		"rejected_state":      "rejected",
		"cancelled_state":     "cancelled",
		"approval_all_groups": []int64{10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelButton).Create(map[string]any{
		"settings_id":     settingsID,
		"name":            "Approve",
		"action_type":     string(internalworkflow.ActionApprove),
		"active":          true,
		"button_class":    "btn-primary",
		"confirm_message": "Confirm approval?",
		"icon":            "fa-check",
		"hotkey":          "shift+a",
		"validate_form":   true,
		"group_ids":       []int64{10},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Hidden",
		"action_type": string(internalworkflow.ActionApprove),
		"active":      true,
		"group_ids":   []int64{99},
	}); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":17,"params":{"model":"purchase.order","method":"get_views","kwargs":{"views":[[71,"form"],[72,"list"]],"options":{}}}}`)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	result := payload["result"].(map[string]any)
	views := result["views"].(map[string]any)
	formArch := views["form"].(map[string]any)["arch"].(string)
	listArch := views["list"].(map[string]any)["arch"].(string)
	assertWorkflowMutatedFormArch(t, formArch)
	if !strings.Contains(listArch, `show_action_approve_all="true"`) {
		t.Fatalf("list arch = %s", listArch)
	}

	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/view/load?model=purchase.order", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("view load response %d %s", rec.Code, rec.Body.String())
	}
	loadedArch := viewLoadArchByID(t, rec.Body.Bytes(), 71)
	assertWorkflowMutatedFormArch(t, loadedArch)

	body = bytes.NewBufferString(`{"jsonrpc":"2.0","id":18,"params":{"model":"purchase.order","method":"get_views","kwargs":{"views":[[71,"form"],[72,"list"]],"options":{"studio":true}}}}`)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("studio get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	result = payload["result"].(map[string]any)
	views = result["views"].(map[string]any)
	studioFormArch := views["form"].(map[string]any)["arch"].(string)
	studioListArch := views["list"].(map[string]any)["arch"].(string)
	if !strings.Contains(studioFormArch, `name="approval_visible_button_ids" invisible="1" readonly="1"`) {
		t.Fatalf("studio form missing hidden fields: %s", studioFormArch)
	}
	for _, forbidden := range []string{"button_box", "approval_action_button", "approval_user_info", "statusbar_state_duration", "show_action_approve_all"} {
		if strings.Contains(studioFormArch, forbidden) || strings.Contains(studioListArch, forbidden) {
			t.Fatalf("studio arch contains runtime mutation %q: form=%s list=%s", forbidden, studioFormArch, studioListArch)
		}
	}
	if strings.Contains(studioListArch, "workflow_view_id") {
		t.Fatalf("studio list contains advanced workflow_view_id: %s", studioListArch)
	}
}

func assertWorkflowMutatedFormArch(t *testing.T, arch string) {
	t.Helper()
	for _, want := range []string{
		`<div name="button_box" class="oe_button_box">`,
		`name="action_open_canceled_record"`,
		`name="workflow_states" invisible="1" readonly="1"`,
		`name="user_can_approve" invisible="1" readonly="1"`,
		`name="approval_visible_button_ids" invisible="1" readonly="1"`,
		`id="approval_user_info"`,
		`name="approval_action_button"`,
		`string="Approve"`,
		`class="btn-primary"`,
		`confirm="Confirm approval?"`,
		`args="[1]"`,
		`icon="fa-check"`,
		`data-hotkey="shift+a"`,
		`validate_form="true"`,
		`widget="statusbar_state_duration"`,
		`statusbar_visible="WORKFLOW"`,
		`name="name" readonly="state != &#39;draft&#39; and not user_can_approve"`,
	} {
		if !strings.Contains(arch, want) {
			t.Fatalf("arch missing %q: %s", want, arch)
		}
	}
	for _, forbidden := range []string{
		`name="state" readonly="state != &#39;draft&#39; and not user_can_approve"`,
		`name="amount_total" readonly="state != &#39;draft&#39; and not user_can_approve"`,
		`name="last_state_update" readonly="state != &#39;draft&#39; and not user_can_approve"`,
		`name="workflow_node_id" invisible="0" readonly="state != &#39;draft&#39; and not user_can_approve"`,
	} {
		if strings.Contains(arch, forbidden) {
			t.Fatalf("arch contains forbidden readonly mutation %q: %s", forbidden, arch)
		}
	}
	if strings.Contains(arch, "Hidden") || strings.Contains(arch, "groups=") {
		t.Fatalf("arch contains restricted workflow markup: %s", arch)
	}
}

func TestCallKWGetViewsAppliesOIAdvancedWorkflowButtons(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	viewReg := view.NewRegistry()
	if err := viewReg.AddWithID(view.View{ID: 81, Name: "PO Advanced Form", Model: "purchase.order", Type: view.Form, Arch: `<form><header><field name="state"/></header><sheet><field name="name"/></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	server.Views = viewReg
	settingsID, err := server.Env.Model(internalworkflow.ModelSettings).Create(map[string]any{
		"name":        "PO Advanced Approval",
		"model":       "purchase.order",
		"active":      true,
		"advance":     true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelButton).Create(map[string]any{
		"settings_id":  settingsID,
		"name":         "Classic Approve",
		"action_type":  string(internalworkflow.ActionApprove),
		"active":       true,
		"button_class": "btn-secondary",
	}); err != nil {
		t.Fatal(err)
	}
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":                 "PO Advanced Workflow",
		"approval_settings_id": settingsID,
		"model":                "purchase.order",
		"active":               true,
		"sequence":             int64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	wizardNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":                 "Wizard Node",
		"workflow_id":          workflowID,
		"type":                 string(internalworkflow.NodeTypeUser),
		"button_type":          string(internalworkflow.ButtonTypeOne),
		"button_name":          "Route",
		"button_context":       "{'x': 1}",
		"button_icon":          "fa-random",
		"button_validate_form": true,
		"active":               true,
		"sequence":             int64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	multiNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Multi Node",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"button_type": string(internalworkflow.ButtonTypeMulti),
		"active":      true,
		"sequence":    int64(20),
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeEnd),
		"active":      true,
		"sequence":    int64(30),
	})
	if err != nil {
		t.Fatal(err)
	}
	transitionID, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":          "Approve Transition",
		"node_id":       multiNodeID,
		"workflow_id":   workflowID,
		"next_node_id":  doneNodeID,
		"groups_ids":    []int64{10},
		"button_class":  "btn-success",
		"context":       "{'ok': True}",
		"icon":          "fa-check",
		"validate_form": true,
		"active":        true,
		"sequence":      int64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":         "Hidden Transition",
		"node_id":      multiNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"groups_ids":   []int64{99},
		"active":       true,
		"sequence":     int64(20),
	}); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"jsonrpc":"2.0","id":181,"params":{"model":"purchase.order","method":"get_views","kwargs":{"views":[[81,"form"]],"options":{}}}}`)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/get_views", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_views response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	formArch := payload["result"].(map[string]any)["views"].(map[string]any)["form"].(map[string]any)["arch"].(string)
	for _, want := range []string{
		`name="workflow_transition_ids" invisible="1" readonly="1"`,
		`name="workflow_node_id" invisible="1" readonly="1"`,
		`name="approval_transition_wizard"`,
		`type="object" string="Route"`,
		`invisible="workflow_node_id == ` + fmt.Sprint(wizardNodeID) + ` and workflow_transition_ids"`,
		`class="btn-primary" args="[` + fmt.Sprint(wizardNodeID) + `]"`,
		`context="{&#39;x&#39;: 1}"`,
		`icon="fa-random"`,
		`id="approval_transition_wizard_` + fmt.Sprint(wizardNodeID) + `"`,
		`validate_form="true"`,
		`name="approval_transition_button"`,
		`string="Approve Transition"`,
		`invisible="` + fmt.Sprint(transitionID) + ` not in workflow_transition_ids"`,
		`class="btn-success" args="[` + fmt.Sprint(transitionID) + `]"`,
		`context="{&#39;ok&#39;: True}"`,
		`icon="fa-check"`,
		`id="approval_transition_button_` + fmt.Sprint(transitionID) + `"`,
	} {
		if !strings.Contains(formArch, want) {
			t.Fatalf("advanced arch missing %q: %s", want, formArch)
		}
	}
	if strings.Contains(formArch, "Hidden Transition") {
		t.Fatalf("advanced arch contains restricted transition: %s", formArch)
	}
	transitionIndex := strings.Index(formArch, `name="approval_transition_wizard"`)
	classicIndex := strings.Index(formArch, `name="approval_action_button"`)
	if transitionIndex < 0 || classicIndex < 0 || transitionIndex > classicIndex {
		t.Fatalf("advanced buttons not before classic approval buttons: %s", formArch)
	}
}

func TestLoginAsHTTPRoutesSwitchBackAndRedirect(t *testing.T) {
	server := testServer(t)
	server.Impersonation = testImpersonationService()
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/login_as/20?group_id=30&redirect=/web%23menu_id=1", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/web#menu_id=1" {
		t.Fatalf("login_as response %d %s", rec.Code, rec.Header().Get("Location"))
	}
	session, ok := server.Impersonation.Session("sid")
	if !ok || !session.Impersonating || session.UserID != 20 || session.OriginalUserID != 1 {
		t.Fatalf("session = %+v ok=%v", session, ok)
	}
	audit := server.Impersonation.AuditLog()
	if len(audit) != 1 || audit[0].Action != "login_as.start" {
		t.Fatalf("audit = %+v", audit)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_back?redirect=/web", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/web" {
		t.Fatalf("login_back response %d %s", rec.Code, rec.Header().Get("Location"))
	}
	session, ok = server.Impersonation.Session("sid")
	if !ok || session.Impersonating || session.UserID != 1 {
		t.Fatalf("back session = %+v ok=%v", session, ok)
	}
	audit = server.Impersonation.AuditLog()
	if len(audit) != 2 || audit[1].Action != "login_as.back" {
		t.Fatalf("back audit = %+v", audit)
	}
}

func TestCallButtonDispatchesWorkflowApproval(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	settingsID := createHTTPWorkflowSettings(t, server.Env)
	buttonID, err := server.Env.Model(internalworkflow.ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"name":        "Approve",
		"action_type": string(internalworkflow.ActionApprove),
		"state_value": "draft",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{"name": "PO HTTP", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d],%d]}`, recordID, buttonID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/purchase.order/approval_action_button", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tag":"soft_reload"`) {
		t.Fatalf("call_button response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("state = %+v", rows[0])
	}
	logs, err := server.Env.Model(internalworkflow.ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_state", "new_state")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || logRows[0]["model"] != "purchase.order" || logRows[0]["record_id"] != recordID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "approved" {
		t.Fatalf("logs = %+v", logRows)
	}
}

func TestCallButtonClassicEmailComposeCompletesNextAction(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	settingsID := createHTTPWorkflowSettings(t, server.Env)
	buttonID, err := server.Env.Model(internalworkflow.ModelButton).Create(map[string]any{
		"settings_id":          settingsID,
		"name":                 "Email Approve",
		"action_type":          string(internalworkflow.ActionEmail),
		"state_value":          "draft",
		"next_state":           "approved",
		"email_wizard_form_id": int64(321),
		"email_next_action":    string(internalworkflow.ActionApprove),
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{"name": "PO HTTP Email Button", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d],%d]}`, recordID, buttonID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/purchase.order/approval_action_button", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("call_button response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	contextValues := action["context"].(map[string]any)
	if action["res_model"] != "mail.compose.message" ||
		int64Value(action["view_id"]) != int64(321) ||
		int64Value(contextValues["approval_button_id"]) != buttonID ||
		contextValues["default_model"] != "purchase.order" {
		t.Fatalf("compose action = %+v", action)
	}
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "draft" {
		t.Fatalf("record changed before send = %+v", rows[0])
	}
	wizardID, err := server.Env.Model("mail.compose.message").Create(map[string]any{
		"model":     "purchase.order",
		"res_ids":   []int64{recordID},
		"subject":   "Approval mail",
		"body_html": "<p>Approve</p>",
		"email_to":  "approver@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	sendBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"action_send_mail","args":[[%d]],"kwargs":{"context":{"approval_button_id":%d}}}`, wizardID, buttonID))
	sendAction := decodeJSON(t, []byte(sendBody))
	if sendAction["type"] != "ir.actions.client" || sendAction["tag"] != "soft_reload" {
		t.Fatalf("send action = %+v", sendAction)
	}
	rows, err = server.Env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("record after send = %+v", rows[0])
	}
	logs, err := server.Env.Model(internalworkflow.ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("approval_button_id", "old_state", "new_state")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 || int64Value(logRows[0]["approval_button_id"]) != buttonID || logRows[0]["old_state"] != "draft" || logRows[0]["new_state"] != "approved" {
		t.Fatalf("logs = %+v", logRows)
	}
}

func TestWorkflowProcessWizardDefaultGetHTTPHydratesTransitionDomain(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":   "PO HTTP Wizard Defaults",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	transitionID, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"groups_ids":   []int64{10},
		"comment":      string(internalworkflow.CommentRequired),
		"active":       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":         "Denied",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"groups_ids":   []int64{99},
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{
		"name":             "PO HTTP Wizard Defaults",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processID, err := internalworkflow.NewProcessStore(server.Env).Save(internalworkflow.Process{
		WorkflowID: workflowID,
		Model:      "purchase.order",
		RecordID:   recordID,
		NodeID:     pendingNodeID,
		State:      "pending",
		Active:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"%s","method":"default_get","args":[["model","record_id","record_name","workflow_process_id","workflow_node_id","workflow_id","workflow_transition_ids","workflow_transition_id","comment_required"]],"kwargs":{"context":{"default_model":"purchase.order","default_record_id":%d,"default_workflow_transition_id":%d}}}`, internalworkflow.ModelWorkflowWizard, recordID, transitionID))
	values := decodeJSON(t, []byte(body))
	if values["model"] != "purchase.order" ||
		int64Value(values["record_id"]) != recordID ||
		values["record_name"] != "PO HTTP Wizard Defaults" ||
		int64Value(values["workflow_process_id"]) != processID ||
		int64Value(values["workflow_node_id"]) != pendingNodeID ||
		int64Value(values["workflow_id"]) != workflowID ||
		int64Value(values["workflow_transition_id"]) != transitionID ||
		values["comment_required"] != true {
		t.Fatalf("wizard defaults = %+v", values)
	}
	if ids := int64Slice(values["workflow_transition_ids"]); len(ids) != 1 || ids[0] != transitionID {
		t.Fatalf("workflow_transition_ids = %+v", values["workflow_transition_ids"])
	}
}

func TestStateUpdateWizardHTTPHydratesOnchangeAndSoftReloads(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	settingsID, err := server.Env.Model(internalworkflow.ModelSettings).Create(map[string]any{
		"name":        "PO Advanced HTTP",
		"model":       "purchase.order",
		"active":      true,
		"advance":     true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":                 "PO HTTP State Update",
		"model":                "purchase.order",
		"approval_settings_id": settingsID,
		"active":               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{
		"name":             "PO HTTP State Update",
		"state":            "pending",
		"workflow_id":      workflowID,
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := internalworkflow.NewProcessStore(server.Env).Save(internalworkflow.Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}

	body := postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"%s","method":"default_get","args":[["res_model","res_ids","workflow_model","workflow_id","workflow_node_id"]],"kwargs":{"context":{"default_res_model":"purchase.order","default_res_ids":[%d]}}}`, internalworkflow.ModelStateUpdateWizard, recordID))
	values := decodeJSON(t, []byte(body))
	if values["res_model"] != "purchase.order" ||
		values["workflow_model"] != true ||
		int64Value(values["workflow_id"]) != workflowID ||
		int64Value(values["workflow_node_id"]) != pendingNodeID {
		t.Fatalf("state update defaults = %+v", values)
	}

	body = postCallKW(t, server.Handler(), fmt.Sprintf(`{"model":"%s","method":"onchange","args":[{"workflow_node_id":%d},["workflow_node_id"],{}]}`, internalworkflow.ModelStateUpdateWizard, doneNodeID))
	values = decodeJSON(t, []byte(body))["value"].(map[string]any)
	if values["state"] != "approved" {
		t.Fatalf("state update onchange = %+v", values)
	}

	wizardID, err := server.Env.Model(internalworkflow.ModelStateUpdateWizard).Create(map[string]any{
		"res_model":        "purchase.order",
		"res_ids":          []int64{recordID},
		"state":            "approved",
		"workflow_model":   true,
		"workflow_id":      workflowID,
		"workflow_node_id": doneNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	body = postCallButton(t, server.Handler(), fmt.Sprintf(`{"model":"%s","method":"action_update","args":[[%d]]}`, internalworkflow.ModelStateUpdateWizard, wizardID))
	values = decodeJSON(t, []byte(body))
	if values["type"] != "ir.actions.client" || values["tag"] != "soft_reload" {
		t.Fatalf("state update action = %+v", values)
	}
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_old_workflow_node_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || int64Value(rows[0]["workflow_node_id"]) != doneNodeID || int64Value(rows[0]["_old_workflow_node_id"]) != pendingNodeID {
		t.Fatalf("state update row = %+v", rows[0])
	}
}

func TestCallButtonApprovalTransitionWizardHTTPReturnsWizardAction(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":   "PO HTTP Wizard Button",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":           "Pending",
		"workflow_id":    workflowID,
		"type":           string(internalworkflow.NodeTypeUser),
		"state":          "pending",
		"button_type":    string(internalworkflow.ButtonTypeOne),
		"wizard_view_id": int64(91),
		"active":         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":         "Approve",
		"node_id":      pendingNodeID,
		"workflow_id":  workflowID,
		"next_node_id": doneNodeID,
		"active":       true,
	}); err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{
		"name":             "PO HTTP Wizard Button",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := internalworkflow.NewProcessStore(server.Env)
	if _, err := store.Save(internalworkflow.Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d],%d]}`, recordID, pendingNodeID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/purchase.order/approval_transition_wizard", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("transition wizard response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	contextValues := action["context"].(map[string]any)
	if action["type"] != "ir.actions.act_window" ||
		action["res_model"] != internalworkflow.ModelWorkflowWizard ||
		action["target"] != "new" ||
		action["view_mode"] != "form" ||
		int64Value(action["view_id"]) != int64(91) ||
		contextValues["default_model"] != "purchase.order" ||
		int64Value(contextValues["default_record_id"]) != recordID ||
		int64Value(contextValues["default_workflow_transition_id"]) != 0 ||
		int64Value(contextValues["active_id"]) != recordID ||
		contextValues["active_model"] != "purchase.order" {
		t.Fatalf("wizard action = %+v", action)
	}
	if ids := int64Slice(contextValues["active_ids"]); len(ids) != 1 || ids[0] != recordID {
		t.Fatalf("active_ids = %+v", contextValues["active_ids"])
	}
}

func TestCallKWCreateAutoStartsAdvancedWorkflow(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	settingsID := createHTTPWorkflowSettings(t, server.Env)
	actionRegistry := internalactions.NewRegistry(internalactions.Hooks{})
	var captured []internalactions.ExecutionContext
	if err := actionRegistry.RegisterGo("capture.http.create", func(_ context.Context, _ internalactions.ServerAction, exec internalactions.ExecutionContext) (internalactions.Result, error) {
		captured = append(captured, exec)
		return internalactions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(internalactions.ServerAction{Name: "Capture HTTP Create", Kind: internalactions.KindGo, GoActionName: "capture.http.create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelAutomation).Create(map[string]any{
		"settings_id":       settingsID,
		"name":              "HTTP On Create",
		"sequence":          int64(10),
		"active":            true,
		"trigger":           string(internalworkflow.TriggerOnCreate),
		"server_action_ids": []int64{actionID},
	}); err != nil {
		t.Fatal(err)
	}
	server.Workflow.Actions = actionRegistry
	viewID, err := server.Env.Model("ir.ui.view").Create(map[string]any{
		"name":  "purchase.order.workflow.form",
		"model": "purchase.order",
		"type":  "form",
		"arch":  `<form><field name="name"/></form>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":      "PO Auto",
		"model":     "purchase.order",
		"active":    true,
		"view_id":   viewID,
		"on_create": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model(internalworkflow.ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": nodeID}); err != nil {
		t.Fatal(err)
	}
	server.Env.RegisterAfterCreateHook(server.Workflow.AutoStartCreateHook())
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"model":"purchase.order","method":"create","values":{"name":"PO HTTP Auto","state":"draft"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/create", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("create response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	recordID := int64Value(payload["id"])
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "workflow_view_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "pending" || rows[0]["workflow_node_id"] != nodeID || rows[0]["workflow_view_id"] != viewID {
		t.Fatalf("record row = %+v", rows[0])
	}
	readBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"purchase.order","method":"web_read","args":[[%d]],"kwargs":{"specification":{"workflow_view_id":{}}}}`, recordID))
	var readRows []map[string]any
	if err := json.Unmarshal([]byte(readBody), &readRows); err != nil {
		t.Fatal(err)
	}
	if len(readRows) != 1 {
		t.Fatalf("web_read rows = %+v", readRows)
	}
	viewValue, ok := readRows[0]["workflow_view_id"].([]any)
	if !ok || len(viewValue) != 2 || int64Value(viewValue[0]) != viewID || viewValue[1] != "purchase.order.workflow.form" {
		t.Fatalf("workflow_view_id web_read = %+v", readRows[0]["workflow_view_id"])
	}
	if len(captured) != 1 || captured[0].Model != "purchase.order" || captured[0].RecordID != recordID || captured[0].Trigger != "on_create" {
		t.Fatalf("captured = %+v", captured)
	}
	processes, err := server.Env.Model(internalworkflow.ModelProcess).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	processRows, err := processes.Read("workflow_id", "model", "record_id", "node_id", "state", "active")
	if err != nil {
		t.Fatal(err)
	}
	if len(processRows) != 1 ||
		processRows[0]["workflow_id"] != workflowID ||
		processRows[0]["model"] != "purchase.order" ||
		processRows[0]["record_id"] != recordID ||
		processRows[0]["node_id"] != nodeID ||
		processRows[0]["state"] != "pending" ||
		processRows[0]["active"] != true {
		t.Fatalf("process rows = %+v", processRows)
	}
}

func TestCallKWWebReadComputesWorkflowViewBeforeProcess(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	handler := server.Handler()
	createView := func(name string) int64 {
		t.Helper()
		id, err := server.Env.Model("ir.ui.view").Create(map[string]any{
			"name":  name,
			"model": "purchase.order",
			"type":  "form",
			"arch":  `<form><field name="name"/></form>`,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	skippedViewID := createView("purchase.order.workflow.skipped")
	matchedViewID := createView("purchase.order.workflow.matched")
	nodeViewID := createView("purchase.order.workflow.node")
	if _, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":      "Skipped",
		"model":     "purchase.order",
		"sequence":  int64(1),
		"active":    true,
		"condition": "amount_total > 5000",
		"view_id":   skippedViewID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":      "Matched",
		"model":     "purchase.order",
		"sequence":  int64(2),
		"active":    true,
		"condition": "amount_total >= 100",
		"view_id":   matchedViewID,
	}); err != nil {
		t.Fatal(err)
	}
	nodeWorkflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":      "Node",
		"model":     "purchase.order",
		"sequence":  int64(3),
		"active":    true,
		"condition": "amount_total > 5000",
		"view_id":   nodeViewID,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": nodeWorkflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{"name": "PO Preprocess", "state": "draft", "amount_total": float64(250)})
	if err != nil {
		t.Fatal(err)
	}
	nodeRecordID, err := server.Env.Model("purchase.order").Create(map[string]any{"name": "PO Node", "state": "draft", "amount_total": float64(10), "workflow_node_id": nodeID})
	if err != nil {
		t.Fatal(err)
	}
	rawRows, err := server.Env.Model("purchase.order").Browse(recordID).Read("workflow_node_id", "workflow_view_id")
	if err != nil {
		t.Fatal(err)
	}
	if rawRows[0]["workflow_node_id"] != nil || rawRows[0]["workflow_view_id"] != nil {
		t.Fatalf("raw row should not persist computed view = %+v", rawRows[0])
	}
	body := postCallKW(t, handler, fmt.Sprintf(`{"model":"purchase.order","method":"web_read","args":[[%d,%d]],"kwargs":{"specification":{"workflow_view_id":{},"name":{}}}}`, recordID, nodeRecordID))
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("web_read rows = %+v", rows)
	}
	viewValue, ok := rows[0]["workflow_view_id"].([]any)
	if !ok || len(viewValue) != 2 || int64Value(viewValue[0]) != matchedViewID || viewValue[1] != "purchase.order.workflow.matched" {
		t.Fatalf("matched workflow_view_id = %+v", rows[0]["workflow_view_id"])
	}
	nodeViewValue, ok := rows[1]["workflow_view_id"].([]any)
	if !ok || len(nodeViewValue) != 2 || int64Value(nodeViewValue[0]) != nodeViewID || nodeViewValue[1] != "purchase.order.workflow.node" {
		t.Fatalf("node workflow_view_id = %+v", rows[1]["workflow_view_id"])
	}
	body = postCallKW(t, handler, `{"model":"purchase.order","method":"web_search_read","kwargs":{"domain":[["name","=","PO Preprocess"]],"specification":{"workflow_view_id":{},"name":{}}}}`)
	payload := decodeJSON(t, []byte(body))
	searchRows := payload["records"].([]any)
	if len(searchRows) != 1 {
		t.Fatalf("web_search_read rows = %+v", payload)
	}
	searchViewValue := searchRows[0].(map[string]any)["workflow_view_id"].([]any)
	if int64Value(searchViewValue[0]) != matchedViewID || searchViewValue[1] != "purchase.order.workflow.matched" {
		t.Fatalf("web_search_read workflow_view_id = %+v", searchViewValue)
	}
}

func TestCallKWCreateApprovalAutoSubmitContextApprovesClassicWorkflow(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	createHTTPWorkflowSettings(t, server.Env)
	server.Env.RegisterAfterCreateHook(server.Workflow.AutoStartCreateHook())
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"model":"purchase.order","method":"create","values":{"name":"PO HTTP Auto Submit","state":"draft"},"kwargs":{"context":{"approval_auto_submit":true}}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/create", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("create response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	recordID := int64Value(payload["id"])
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" {
		t.Fatalf("record row = %+v", rows[0])
	}
	logs, err := server.Env.Model(internalworkflow.ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	logRows, err := logs.Read("model", "record_id", "old_state", "new_state", "approval_button_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(logRows) != 1 ||
		logRows[0]["model"] != "purchase.order" ||
		logRows[0]["record_id"] != recordID ||
		logRows[0]["old_state"] != "draft" ||
		logRows[0]["new_state"] != "approved" ||
		int64Value(logRows[0]["approval_button_id"]) != 0 {
		t.Fatalf("logs = %+v", logRows)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"purchase.order","method":"create","values":{"name":"PO HTTP Manual","state":"draft"}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/create", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manual create response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	plainID := int64Value(payload["id"])
	plainRows, err := server.Env.Model("purchase.order").Browse(plainID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if plainRows[0]["state"] != "draft" {
		t.Fatalf("plain row = %+v", plainRows[0])
	}
	logs, err = server.Env.Model(internalworkflow.ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 1 {
		t.Fatalf("log count = %d", logs.Len())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(`{"model":"purchase.order","method":"web_save","args":[[],{"name":"PO HTTP Web Save Auto","state":"draft"}],"kwargs":{"context":{"approval_auto_submit":true},"specification":{"state":{}}}}`)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw/purchase.order/web_save", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("web_save response %d %s", rec.Code, rec.Body.String())
	}
	var saved []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0]["state"] != "approved" {
		t.Fatalf("web_save payload = %+v", saved)
	}
	logs, err = server.Env.Model(internalworkflow.ModelLog).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 2 {
		t.Fatalf("web_save log count = %d", logs.Len())
	}
}

func TestWorkflowEmailTransitionCompletesAfterComposeSend(t *testing.T) {
	server := testWorkflowDispatchServer(t)
	composeViewID, err := server.Env.Model("ir.ui.view").Create(map[string]any{
		"name":  "mail.compose.message.form",
		"model": "mail.compose.message",
		"type":  "form",
		"mode":  "primary",
		"arch":  `<form string="Compose Email"><footer><button name="action_send_mail" type="object"/></footer></form>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "mail",
		"name":          "email_compose_message_wizard_form",
		"complete_name": "mail.email_compose_message_wizard_form",
		"model":         "ir.ui.view",
		"res_id":        composeViewID,
	}); err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{
		"name":      "Workflow Email",
		"model":     "purchase.order",
		"subject":   "Approve {{ name }}",
		"body_html": "<p>{{ name }}</p>",
		"email_to":  "approver@example.com",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := server.Env.Model(internalworkflow.ModelWorkflow).Create(map[string]any{
		"name":   "PO Email",
		"model":  "purchase.order",
		"active": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Pending",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeUser),
		"state":       "pending",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneNodeID, err := server.Env.Model(internalworkflow.ModelNode).Create(map[string]any{
		"name":        "Done",
		"workflow_id": workflowID,
		"type":        string(internalworkflow.NodeTypeEnd),
		"state":       "approved",
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model(internalworkflow.ModelWorkflow).Browse(workflowID).Write(map[string]any{"start_node_id": pendingNodeID}); err != nil {
		t.Fatal(err)
	}
	transitionID, err := server.Env.Model(internalworkflow.ModelTransition).Create(map[string]any{
		"name":              "Email Approve",
		"node_id":           pendingNodeID,
		"workflow_id":       workflowID,
		"next_node_id":      doneNodeID,
		"is_email":          true,
		"email_template_id": templateID,
		"active":            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := server.Env.Model("purchase.order").Create(map[string]any{
		"name":             "PO HTTP Email",
		"state":            "pending",
		"workflow_node_id": pendingNodeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := internalworkflow.NewProcessStore(server.Env)
	if _, err := store.Save(internalworkflow.Process{WorkflowID: workflowID, Model: "purchase.order", RecordID: recordID, NodeID: pendingNodeID, State: "pending", Active: true}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d],%d]}`, recordID, transitionID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/purchase.order/approval_transition_button", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("transition response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	contextValues := action["context"].(map[string]any)
	views := action["views"].([]any)
	viewPair := views[0].([]any)
	if action["res_model"] != "mail.compose.message" ||
		int64Value(action["view_id"]) != composeViewID ||
		int64Value(viewPair[0]) != composeViewID ||
		viewPair[1] != "form" ||
		contextValues["default_model"] != "purchase.order" ||
		int64Value(contextValues["default_template_id"]) != templateID ||
		int64Value(contextValues["workflow_transition_id"]) != transitionID {
		t.Fatalf("compose action = %+v", action)
	}
	process, ok, err := store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != pendingNodeID || process.State != "pending" || !process.Active {
		t.Fatalf("process before send = %+v ok=%v", process, ok)
	}
	defaults := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"default_get","args":[["model","res_id","res_ids","template_id","subject","body","email_to","body_is_html"]],"kwargs":{"context":{"active_model":"purchase.order","active_ids":[%d],"default_template_id":%d}}}`, recordID, templateID))
	composeValues := decodeJSON(t, []byte(defaults))
	wizardID, err := server.Env.Model("mail.compose.message").Create(composeValues)
	if err != nil {
		t.Fatal(err)
	}
	sendBody := postCallKW(t, handler, fmt.Sprintf(`{"model":"mail.compose.message","method":"action_send_mail","args":[[%d]],"kwargs":{"context":{"workflow_transition_id":%d}}}`, wizardID, transitionID))
	sendAction := decodeJSON(t, []byte(sendBody))
	if sendAction["type"] != "ir.actions.client" || sendAction["tag"] != "soft_reload" {
		t.Fatalf("send action = %+v", sendAction)
	}
	process, ok, err = store.Find("purchase.order", recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || process.NodeID != doneNodeID || process.State != "approved" || process.Active || process.LastTransitionID != transitionID {
		t.Fatalf("process after send = %+v ok=%v", process, ok)
	}
	rows, err := server.Env.Model("purchase.order").Browse(recordID).Read("state", "workflow_node_id", "_workflow_transition_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != "approved" || rows[0]["workflow_node_id"] != doneNodeID || rows[0]["_workflow_transition_id"] != transitionID {
		t.Fatalf("record row = %+v", rows[0])
	}
	mails, err := server.Env.Model("mail.message").Search(domain.And(
		domain.Cond("model", domain.Equal, "purchase.order"),
		domain.Cond("res_id", domain.Equal, recordID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 1 {
		t.Fatalf("mail count = %d", mails.Len())
	}
}

func TestCallButtonDispatchesAccountMoveReversalRefund(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, journalID := createHTTPAccountingMove(t, server.Env, "INV/001", "out_invoice", "posted")
	wizardID, err := server.Env.Model("account.move.reversal").Create(map[string]any{
		"move_ids":   []int64{moveID},
		"date":       dateValue(2026, 8, 1),
		"reason":     "credit note",
		"journal_id": journalID,
		"company_id": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.reversal/refund_moves", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("refund response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["res_model"] != "account.move" || payload["view_mode"] != "form" || payload["res_id"] == nil {
		t.Fatalf("refund action = %#v", payload)
	}
	newID := int64Value(payload["res_id"])
	rows, err := server.Env.Model("account.move").Browse(newID).Read("state", "move_type", "ref", "reversed_entry_id", "line_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "draft" || rows[0]["move_type"] != "out_refund" || rows[0]["reversed_entry_id"] != moveID {
		t.Fatalf("reversal move = %+v", rows)
	}
	if !strings.Contains(stringValue(rows[0]["ref"]), "Reversal of: INV/001, credit note") {
		t.Fatalf("reversal ref = %+v", rows[0]["ref"])
	}
	lineIDs := int64Slice(rows[0]["line_ids"])
	lineRows, err := server.Env.Model("account.move.line").Browse(lineIDs...).Read("debit", "credit")
	if err != nil {
		t.Fatal(err)
	}
	if len(lineRows) != 2 || lineRows[0]["debit"] != int64(0) || lineRows[0]["credit"] != int64(10000) || lineRows[1]["debit"] != int64(10000) {
		t.Fatalf("reversal lines = %+v", lineRows)
	}
	wizardRows, err := server.Env.Model("account.move.reversal").Browse(wizardID).Read("new_move_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := int64Slice(wizardRows[0]["new_move_ids"]); len(got) != 1 || got[0] != newID {
		t.Fatalf("new_move_ids = %+v", wizardRows)
	}
}

func TestAccountMoveDatasetWriteUnlinkApplyLockPolicy(t *testing.T) {
	server := testAccountingDispatchServer(t)
	companyID, err := server.Env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	moveWriteID, _ := createHTTPAccountingMove(t, server.Env, "INV/LOCK/WRITE", "out_invoice", "posted")
	moveUnlinkID, _ := createHTTPAccountingMove(t, server.Env, "INV/LOCK/UNLINK", "out_invoice", "posted")
	hardMoveID, _ := createHTTPAccountingMove(t, server.Env, "INV/LOCK/HARD", "out_invoice", "posted")
	moveRows, err := readAccountingMoves(server.Env, []int64{moveWriteID, moveUnlinkID})
	if err != nil {
		t.Fatal(err)
	}
	writePDFID, err := ensureInvoicePDFPlaceholder(server.Env, moveRows[0])
	if err != nil {
		t.Fatal(err)
	}
	unlinkPDFID, err := ensureInvoicePDFPlaceholder(server.Env, moveRows[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": dateValue(2026, 3, 31)}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move","method":"write","args":[[%d],{"state":"draft"}]}`, moveWriteID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrFiscalLockDate.Error()) {
		t.Fatalf("write without exception response %d %s", rec.Code, rec.Body.String())
	}
	if _, err := server.Env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(5), "fiscalyear_lock_date": dateValue(2025, 12, 31)}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move","method":"write","args":[[%d],{"state":"draft"}]}`, moveWriteID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("write with exception response %d %s", rec.Code, rec.Body.String())
	}
	pdfRows, err := server.Env.Model("ir.attachment").Browse(writePDFID).Read("name", "res_model", "res_id", "res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 1 || pdfRows[0]["res_model"] != "account.move" || pdfRows[0]["res_id"] != moveWriteID || stringValue(pdfRows[0]["res_field"]) != "" || !strings.Contains(stringValue(pdfRows[0]["name"]), "detached by user 5 on") {
		t.Fatalf("write detached pdf row = %+v", pdfRows)
	}
	moveFieldRows, err := server.Env.Model("account.move").Browse(moveWriteID).Read("invoice_pdf_report_id", "invoice_pdf_report_file")
	if err != nil {
		t.Fatal(err)
	}
	if len(moveFieldRows) != 1 || int64Value(moveFieldRows[0]["invoice_pdf_report_id"]) != 0 || len(byteValue(moveFieldRows[0]["invoice_pdf_report_file"])) != 0 {
		t.Fatalf("write reset pdf fields = %+v", moveFieldRows)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move","method":"unlink","args":[[%d]]}`, moveUnlinkID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("unlink with exception response %d %s", rec.Code, rec.Body.String())
	}
	pdfRows, err = server.Env.Model("ir.attachment").Browse(unlinkPDFID).Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 0 {
		t.Fatalf("unlink left invoice PDF attachment = %+v", pdfRows)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_attachments/%d", unlinkPDFID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted move attachment download response %d %s", rec.Code, rec.Body.String())
	}
	if err := server.Env.Model("res.company").Browse(companyID).Write(map[string]any{"hard_lock_date": dateValue(2026, 3, 31)}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move","method":"unlink","args":[[%d]]}`, hardMoveID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrHardLockDate.Error()) {
		t.Fatalf("unlink with hard lock response %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountMoveCallButtonLifecycleAppliesLockPolicy(t *testing.T) {
	server := testAccountingDispatchServer(t)
	companyID, err := server.Env.Model("res.company").Create(map[string]any{"name": "Company"})
	if err != nil {
		t.Fatal(err)
	}
	resetID, _ := createHTTPAccountingMove(t, server.Env, "INV/BTN/RESET", "out_invoice", "posted")
	cancelID, _ := createHTTPAccountingMove(t, server.Env, "INV/BTN/CANCEL", "out_invoice", "posted")
	hardID, _ := createHTTPAccountingMove(t, server.Env, "INV/BTN/HARD", "out_invoice", "posted")
	draftID, _ := createHTTPAccountingMove(t, server.Env, "INV/BTN/POST", "out_invoice", "draft")
	resetMoves, err := readAccountingMoves(server.Env, []int64{resetID})
	if err != nil {
		t.Fatal(err)
	}
	resetPDFID, err := ensureInvoicePDFPlaceholder(server.Env, resetMoves[0])
	if err != nil {
		t.Fatal(err)
	}
	cancelMoves, err := readAccountingMoves(server.Env, []int64{cancelID})
	if err != nil {
		t.Fatal(err)
	}
	cancelPDFID, err := ensureInvoicePDFPlaceholder(server.Env, cancelMoves[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("res.company").Browse(companyID).Write(map[string]any{"fiscalyear_lock_date": dateValue(2026, 3, 31)}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/button_draft", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, resetID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrFiscalLockDate.Error()) {
		t.Fatalf("button_draft without exception response %d %s", rec.Code, rec.Body.String())
	}
	pdfRows, err := server.Env.Model("ir.attachment").Browse(resetPDFID).Read("res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 1 || pdfRows[0]["res_field"] != "invoice_pdf_report_file" {
		t.Fatalf("pdf detached after failed reset = %+v", pdfRows)
	}
	if _, err := server.Env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "user_id": int64(5), "fiscalyear_lock_date": dateValue(2025, 12, 31)}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/button_draft", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, resetID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("button_draft with exception response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("account.move").Browse(resetID).Read("state", "invoice_pdf_report_id", "invoice_pdf_report_file", "message_main_attachment_id")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != string(coreaccounting.MoveDraft) {
		t.Fatalf("reset state = %+v", rows)
	}
	if int64Value(rows[0]["invoice_pdf_report_id"]) != 0 || len(byteValue(rows[0]["invoice_pdf_report_file"])) != 0 {
		t.Fatalf("reset pdf fields = %+v", rows)
	}
	pdfRows, err = server.Env.Model("ir.attachment").Browse(resetPDFID).Read("name", "res_model", "res_id", "res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 1 || pdfRows[0]["res_model"] != "account.move" || pdfRows[0]["res_id"] != resetID || stringValue(pdfRows[0]["res_field"]) != "" {
		t.Fatalf("detached pdf row = %+v", pdfRows)
	}
	if !strings.Contains(stringValue(pdfRows[0]["name"]), "detached by user 5 on") {
		t.Fatalf("detached pdf name = %+v", pdfRows)
	}
	resetMoves, err = readAccountingMoves(server.Env, []int64{resetID})
	if err != nil {
		t.Fatal(err)
	}
	regeneratedPDFID, err := ensureInvoicePDFPlaceholder(server.Env, resetMoves[0])
	if err != nil {
		t.Fatal(err)
	}
	if regeneratedPDFID == resetPDFID {
		t.Fatalf("regenerated pdf reused detached attachment %d", regeneratedPDFID)
	}
	pdfRows, err = server.Env.Model("ir.attachment").Browse(regeneratedPDFID).Read("res_model", "res_id", "res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 1 || pdfRows[0]["res_model"] != "account.move" || pdfRows[0]["res_id"] != resetID || pdfRows[0]["res_field"] != "invoice_pdf_report_file" {
		t.Fatalf("regenerated pdf row = %+v", pdfRows)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/button_cancel", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, cancelID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("button_cancel with exception response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("account.move").Browse(cancelID).Read("state", "invoice_pdf_report_id", "invoice_pdf_report_file")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != string(coreaccounting.MoveCancel) {
		t.Fatalf("cancel state = %+v", rows)
	}
	if int64Value(rows[0]["invoice_pdf_report_id"]) != 0 || len(byteValue(rows[0]["invoice_pdf_report_file"])) != 0 {
		t.Fatalf("cancel pdf fields = %+v", rows)
	}
	pdfRows, err = server.Env.Model("ir.attachment").Browse(cancelPDFID).Read("name", "res_model", "res_id", "res_field")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfRows) != 1 || pdfRows[0]["res_model"] != "account.move" || pdfRows[0]["res_id"] != cancelID || stringValue(pdfRows[0]["res_field"]) != "" || !strings.Contains(stringValue(pdfRows[0]["name"]), "detached by user 5 on") {
		t.Fatalf("cancel detached pdf row = %+v", pdfRows)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_post", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, draftID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("action_post with exception response %d %s", rec.Code, rec.Body.String())
	}
	rows, err = server.Env.Model("account.move").Browse(draftID).Read("state", "name")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["state"] != string(coreaccounting.MovePosted) || !strings.HasPrefix(stringValue(rows[0]["name"]), "SAJ/") {
		t.Fatalf("posted move = %+v", rows)
	}
	if err := server.Env.Model("res.company").Browse(companyID).Write(map[string]any{"hard_lock_date": dateValue(2026, 3, 31)}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/button_cancel", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, hardID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrHardLockDate.Error()) {
		t.Fatalf("button_cancel with hard lock response %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountLockExceptionActionRevokeRequiresManager(t *testing.T) {
	server := testAccountingDispatchServer(t)
	companyID, err := server.Env.Model("res.company").Create(map[string]any{"name": "Company", "fiscalyear_lock_date": dateValue(2026, 3, 31)})
	if err != nil {
		t.Fatal(err)
	}
	lockID, err := server.Env.Model("account.lock_exception").Create(map[string]any{"company_id": companyID, "fiscalyear_lock_date": dateValue(2025, 12, 31)})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.lock_exception/action_revoke", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, lockID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrLockExceptionAccess.Error()) {
		t.Fatalf("revoke without manager response %d %s", rec.Code, rec.Body.String())
	}
	managerGroupID, err := server.Env.Model("res.groups").Create(map[string]any{"name": "Accounting Manager"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "account",
		"name":          "group_account_manager",
		"complete_name": "account.group_account_manager",
		"model":         "res.groups",
		"res_id":        managerGroupID,
	}); err != nil {
		t.Fatal(err)
	}
	server.Env = server.Env.WithContext(record.Context{UserID: 5, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{managerGroupID}}})
	handler = server.Handler()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.lock_exception/action_revoke", bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, lockID))))
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke with manager response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("account.lock_exception").Browse(lockID).Read("active", "state", "end_datetime")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["active"] != false || rows[0]["state"] != string(coreaccounting.LockExceptionRevoked) || accountingDateValue(rows[0]["end_datetime"]).IsZero() {
		t.Fatalf("revoked row = %+v", rows[0])
	}
}

func TestCallButtonDispatchesAccountMoveReversalModifyWithSideEffect(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, journalID := createHTTPAccountingMove(t, server.Env, "BILL/001", "in_invoice", "posted")
	wizardID, err := server.Env.Model("account.move.reversal").Create(map[string]any{
		"move_ids":   []int64{moveID},
		"date":       dateValue(2026, 1, 5),
		"reason":     "vendor correction",
		"journal_id": journalID,
		"company_id": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"params":{"args":[[%d]]}}`, wizardID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.reversal/modify_moves", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("modify response %d %s", rec.Code, rec.Body.String())
	}
	envelope := decodeJSON(t, rec.Body.Bytes())
	result := envelope["result"].(map[string]any)
	replacementID := int64Value(result["res_id"])
	if envelope["id"] != float64(9) || result["view_mode"] != "form" || replacementID == 0 {
		t.Fatalf("modify action = %#v", envelope)
	}
	moveRows, err := server.Env.Model("account.move").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := moveRows.Read("id", "move_type", "reversed_entry_id", "payment_state")
	if err != nil {
		t.Fatal(err)
	}
	var sideEffectCount int
	for _, row := range rows {
		switch row["id"] {
		case moveID:
			if row["payment_state"] != "reversed" {
				t.Fatalf("original move not marked reversed: %+v", row)
			}
		case replacementID:
			if row["move_type"] != "in_invoice" || int64Value(row["reversed_entry_id"]) != 0 {
				t.Fatalf("replacement row = %+v", row)
			}
		default:
			if row["move_type"] == "in_refund" && row["reversed_entry_id"] == moveID {
				sideEffectCount++
			}
		}
	}
	if sideEffectCount != 1 {
		t.Fatalf("rows missing hidden reversal side effect: %+v", rows)
	}
	wizardRows, err := server.Env.Model("account.move.reversal").Browse(wizardID).Read("new_move_ids")
	if err != nil {
		t.Fatal(err)
	}
	if got := int64Slice(wizardRows[0]["new_move_ids"]); len(got) != 1 || got[0] != replacementID {
		t.Fatalf("modify new_move_ids = %+v", wizardRows)
	}
}

func TestAccountMoveReversalDefaultGetAndValidation(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, journalID := createHTTPAccountingMove(t, server.Env, "INV/002", "out_invoice", "posted")
	handler := server.Handler()

	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move.reversal","method":"default_get","args":[["company_id","move_ids","journal_id","available_journal_ids","residual","move_type"]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, moveID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("default_get response %d %s", rec.Code, rec.Body.String())
	}
	defaults := decodeJSON(t, rec.Body.Bytes())
	if defaults["company_id"] != float64(1) && defaults["company_id"] != int64(1) {
		t.Fatalf("defaults = %#v", defaults)
	}
	if got := int64Slice(defaults["move_ids"]); len(got) != 1 || got[0] != moveID {
		t.Fatalf("move defaults = %#v", defaults)
	}

	draftID, _ := createHTTPAccountingMove(t, server.Env, "INV/DRAFT", "out_invoice", "draft")
	wizardID, err := server.Env.Model("account.move.reversal").Create(map[string]any{
		"move_ids":   []int64{draftID},
		"date":       dateValue(2026, 8, 1),
		"journal_id": journalID,
		"company_id": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.reversal/refund_moves", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrReversalNoMoves.Error()) {
		t.Fatalf("draft reversal response %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountMoveReversalContextFallbackRequiresActiveMove(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, journalID := createHTTPAccountingMove(t, server.Env, "INV/003", "out_invoice", "posted")
	wizardID, err := server.Env.Model("account.move.reversal").Create(map[string]any{
		"date":       dateValue(2026, 8, 1),
		"journal_id": journalID,
		"company_id": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]],"kwargs":{"context":{"active_model":"res.partner","active_ids":[%d]}}}`, wizardID, moveID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.reversal/refund_moves", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected fallback response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, wizardID, moveID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.reversal/refund_moves", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("active move fallback response %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountPaymentRegisterDefaultGetAndCreatePayments(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, journalID := createHTTPAccountingMove(t, server.Env, "INV/PAY", "out_invoice", "posted")
	handler := server.Handler()

	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"account.payment.register","method":"default_get","args":[["line_ids","payment_date","amount","communication","journal_id","company_id","partner_id","payment_type","partner_type","source_amount","can_edit_wizard","total_payments_amount"]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, moveID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("payment default_get response %d %s", rec.Code, rec.Body.String())
	}
	defaults := decodeJSON(t, rec.Body.Bytes())
	if defaults["amount"] != float64(10000) && defaults["amount"] != int64(10000) {
		t.Fatalf("payment defaults = %#v", defaults)
	}
	if defaults["payment_type"] != "inbound" || defaults["partner_type"] != "customer" || defaults["journal_id"] != float64(journalID) && defaults["journal_id"] != journalID {
		t.Fatalf("payment defaults = %#v", defaults)
	}
	lineIDs := int64Slice(defaults["line_ids"])
	if len(lineIDs) != 1 {
		t.Fatalf("payment line defaults = %#v", defaults)
	}

	wizardID, err := server.Env.Model("account.payment.register").Create(map[string]any{
		"line_ids":           lineIDs,
		"payment_date":       dateValue(2026, 8, 1),
		"amount":             int64(10000),
		"communication":      "INV/PAY",
		"journal_id":         journalID,
		"company_id":         int64(1),
		"partner_id":         int64(7),
		"payment_type":       "inbound",
		"partner_type":       "customer",
		"can_edit_wizard":    true,
		"can_group_payments": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.payment.register/action_create_payments", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("create payment response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	paymentID := int64Value(action["res_id"])
	if action["res_model"] != "account.payment" || action["view_mode"] != "form" || paymentID == 0 {
		t.Fatalf("payment action = %#v", action)
	}
	paymentRows, err := server.Env.Model("account.payment").Browse(paymentID).Read("state", "amount", "payment_type", "partner_type", "is_reconciled", "reconciled_invoice_ids", "move_id")
	if err != nil {
		t.Fatal(err)
	}
	if paymentRows[0]["state"] != "paid" || paymentRows[0]["payment_type"] != "inbound" || paymentRows[0]["partner_type"] != "customer" || paymentRows[0]["is_reconciled"] != true {
		t.Fatalf("payment row = %+v", paymentRows[0])
	}
	paymentMoveID := int64Value(paymentRows[0]["move_id"])
	if paymentMoveID == 0 {
		t.Fatalf("payment move not linked: %+v", paymentRows[0])
	}
	moveRows, err := server.Env.Model("account.move").Browse(moveID).Read("amount_residual", "payment_state", "status_in_payment", "payment_count", "reconciled_payment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if moveRows[0]["amount_residual"] != int64(0) || moveRows[0]["payment_state"] != "paid" || moveRows[0]["payment_count"] != 1 {
		t.Fatalf("paid move row = %+v", moveRows[0])
	}
	lineRows, err := server.Env.Model("account.move.line").Browse(lineIDs...).Read("amount_residual", "reconciled", "payment_id", "full_reconcile_id", "matched_credit_ids")
	if err != nil {
		t.Fatal(err)
	}
	if lineRows[0]["amount_residual"] != int64(0) || lineRows[0]["reconciled"] != true || lineRows[0]["payment_id"] != paymentID {
		t.Fatalf("paid line row = %+v", lineRows[0])
	}
	if int64Value(lineRows[0]["full_reconcile_id"]) == 0 || len(int64Slice(lineRows[0]["matched_credit_ids"])) != 1 {
		t.Fatalf("source line reconciliation = %+v", lineRows[0])
	}
	partialRows, err := server.Env.Model("account.partial.reconcile").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	partials, err := partialRows.Read("debit_move_id", "credit_move_id", "amount", "full_reconcile_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(partials) != 1 || partials[0]["debit_move_id"] != lineIDs[0] || int64Value(partials[0]["credit_move_id"]) == 0 || partials[0]["amount"] != int64(10000) {
		t.Fatalf("partials = %+v", partials)
	}
	fullRows, err := server.Env.Model("account.full.reconcile").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	full, err := fullRows.Read("partial_reconcile_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 1 || len(int64Slice(full[0]["partial_reconcile_ids"])) != 1 {
		t.Fatalf("full reconcile = %+v", full)
	}
}

func TestAccountMoveActionRegisterPaymentOpensWizard(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/OPENPAY", "out_invoice", "posted")
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, moveID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_register_payment", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("action_register_payment response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["res_model"] != "account.payment.register" || action["view_mode"] != "form" || action["target"] != "new" {
		t.Fatalf("payment register action = %#v", action)
	}
	context := action["context"].(map[string]any)
	if context["active_model"] != "account.move.line" {
		t.Fatalf("payment context = %#v", context)
	}
	lineIDs := int64Slice(context["active_ids"])
	if len(lineIDs) != 1 {
		t.Fatalf("payment active lines = %#v", context)
	}
}

func TestAccountMoveLineActionRegisterPaymentOpensWizard(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/LINEPAY", "out_invoice", "posted")
	moveRows, err := server.Env.Model("account.move").Browse(moveID).Read("line_ids")
	if err != nil {
		t.Fatal(err)
	}
	lineID := int64Slice(moveRows[0]["line_ids"])[0]
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, lineID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.line/action_payment_items_register_payment", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("line action_register_payment response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	context := action["context"].(map[string]any)
	if action["res_model"] != "account.payment.register" || context["active_model"] != "account.move.line" || context["default_group_payment"] != true {
		t.Fatalf("line payment action = %#v", action)
	}
	if got := int64Slice(context["active_ids"]); len(got) != 1 || got[0] != lineID {
		t.Fatalf("line active ids = %#v", context)
	}
}

func TestAccountMoveSendWizardDefaultGetAndSend(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/SEND", "out_invoice", "posted")
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, moveID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_invoice_sent", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("action_invoice_sent response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["res_model"] != "account.move.send.wizard" || action["target"] != "new" {
		t.Fatalf("send action = %#v", action)
	}
	context := action["context"].(map[string]any)
	if context["active_model"] != "account.move" || context["allow_partners_without_mail"] != true {
		t.Fatalf("send context = %#v", context)
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move.send.wizard","method":"default_get","args":[["move_id","model","res_ids","render_model","sending_methods","subject","body","can_edit_body"]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, moveID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("send default_get response %d %s", rec.Code, rec.Body.String())
	}
	defaults := decodeJSON(t, rec.Body.Bytes())
	if defaults["move_id"] != float64(moveID) && defaults["move_id"] != moveID || defaults["model"] != "account.move" || defaults["render_model"] != "account.move" {
		t.Fatalf("send defaults = %#v", defaults)
	}
	wizardID, err := server.Env.Model("account.move.send.wizard").Create(map[string]any{
		"move_id":          moveID,
		"model":            "account.move",
		"res_ids":          fmt.Sprintf("%d", moveID),
		"render_model":     "account.move",
		"sending_methods":  `{"email":true}`,
		"subject":          "Invoice INV/SEND",
		"body":             "Body",
		"mail_partner_ids": []int64{7},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.wizard/action_send_and_print", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ir.actions.act_window_close") {
		t.Fatalf("send wizard response %d %s", rec.Code, rec.Body.String())
	}
	moveRows, err := server.Env.Model("account.move").Browse(moveID).Read("is_move_sent", "invoice_pdf_report_id", "invoice_pdf_report_file")
	if err != nil {
		t.Fatal(err)
	}
	attachmentID := int64Value(moveRows[0]["invoice_pdf_report_id"])
	if moveRows[0]["is_move_sent"] != true || attachmentID == 0 {
		t.Fatalf("sent move = %+v", moveRows[0])
	}
	messageRows, err := server.Env.Model("mail.message").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	messages, err := messageRows.Read("id", "model", "res_id", "subject", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	var invoiceMessage map[string]any
	for _, message := range messages {
		if message["subject"] == "Invoice INV/SEND" {
			invoiceMessage = message
			break
		}
	}
	if invoiceMessage == nil || invoiceMessage["model"] != "account.move" || invoiceMessage["res_id"] != moveID {
		t.Fatalf("messages = %+v", messages)
	}
	messageID := int64Value(invoiceMessage["id"])
	messageAttachmentIDs := int64Slice(invoiceMessage["attachment_ids"])
	if len(messageAttachmentIDs) != 1 || messageAttachmentIDs[0] == attachmentID {
		t.Fatalf("message attachment ids = %#v", invoiceMessage["attachment_ids"])
	}
	mailRows, err := server.Env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	mails, err := mailRows.Read("mail_message_id", "email_to", "attachment_ids", "state")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || mails[0]["state"] != "outgoing" || !strings.Contains(stringValue(mails[0]["email_to"]), "partner-7") {
		t.Fatalf("mails = %+v", mails)
	}
	if got := int64Slice(mails[0]["attachment_ids"]); len(got) != 1 || got[0] != messageAttachmentIDs[0] {
		t.Fatalf("mail attachment ids = %#v", mails[0]["attachment_ids"])
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	attachments, err := attachmentRows.Read("id", "res_model", "res_field", "res_id", "mimetype")
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 2 {
		t.Fatalf("attachments = %+v", attachments)
	}
	var foundMovePDF, foundMailPDF bool
	for _, attachment := range attachments {
		if int64Value(attachment["id"]) == attachmentID &&
			attachment["res_model"] == "account.move" &&
			attachment["res_field"] == "invoice_pdf_report_file" &&
			attachment["res_id"] == moveID &&
			attachment["mimetype"] == "application/pdf" {
			foundMovePDF = true
		}
		if int64Value(attachment["id"]) == messageAttachmentIDs[0] &&
			attachment["res_model"] == "mail.message" &&
			stringValue(attachment["res_field"]) == "" &&
			attachment["res_id"] == messageID &&
			attachment["mimetype"] == "application/pdf" {
			foundMailPDF = true
		}
	}
	if !foundMovePDF || !foundMailPDF {
		t.Fatalf("attachments = %+v", attachments)
	}
}

func TestAccountMoveSendWizardTemplateWidgetSelection(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/WIDGET", "out_invoice", "posted")
	templateAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "terms.pdf",
		"res_model": "mail.template",
		"type":      "binary",
		"mimetype":  "application/pdf",
		"datas":     []byte("template terms"),
	})
	if err != nil {
		t.Fatal(err)
	}
	invoiceReportID, err := server.Env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Invoice Report",
		"type":              "ir.actions.report",
		"model":             "account.move",
		"report_name":       "account.report_invoice",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Invoice Copy - %s' % (object.name)",
		"is_invoice_report": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	extraReportID, err := server.Env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Extra Report",
		"type":              "ir.actions.report",
		"model":             "account.move",
		"report_name":       "x.account_extra",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Extra - %s' % (object.name)",
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{
		"name":                "Invoice Widget Template",
		"model":               "account.move",
		"subject":             "Template {{ name }}",
		"body_html":           "<p>{{ object.name }}</p>",
		"attachment_ids":      []int64{templateAttachmentID},
		"report_template_ids": []int64{invoiceReportID, extraReportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Env.Model("ir.model.data").Create(map[string]any{
		"module":        "account",
		"name":          "email_template_edi_invoice",
		"complete_name": "account.email_template_edi_invoice",
		"model":         "mail.template",
		"res_id":        templateID,
	}); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(fmt.Sprintf(`{"model":"account.move.send.wizard","method":"default_get","args":[["move_id","model","res_ids","render_model","sending_methods","template_id","mail_partner_ids","mail_attachments_widget","display_attachments_widget","subject","body","can_edit_body"]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, moveID))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("send default_get response %d %s", rec.Code, rec.Body.String())
	}
	defaults := decodeJSON(t, rec.Body.Bytes())
	if int64Value(defaults["template_id"]) != templateID || defaults["subject"] != "Template INV/WIDGET" || defaults["body"] != "<p>INV/WIDGET</p>" || defaults["display_attachments_widget"] != true {
		t.Fatalf("send defaults = %#v", defaults)
	}
	if got := int64Slice(defaults["mail_partner_ids"]); len(got) != 1 || got[0] != int64(7) {
		t.Fatalf("default partners = %#v", defaults["mail_partner_ids"])
	}
	var widgetRows []map[string]any
	if err := json.Unmarshal([]byte(stringValue(defaults["mail_attachments_widget"])), &widgetRows); err != nil {
		t.Fatal(err)
	}
	var sawPlaceholder, sawTemplateAttachment bool
	for _, row := range widgetRows {
		if stringValue(row["name"]) == "INV/WIDGET.pdf" && row["placeholder"] == true {
			sawPlaceholder = true
		}
		if int64Value(row["id"]) == templateAttachmentID && int64Value(row["mail_template_id"]) == templateID {
			row["skip"] = true
			sawTemplateAttachment = true
		}
	}
	if !sawPlaceholder || !sawTemplateAttachment {
		t.Fatalf("widget rows = %+v", widgetRows)
	}
	manualAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":     "terms.pdf",
		"type":     "binary",
		"mimetype": "application/pdf",
		"datas":    []byte("manual terms"),
	})
	if err != nil {
		t.Fatal(err)
	}
	widgetRows = append(widgetRows, map[string]any{
		"id":       manualAttachmentID,
		"name":     "terms.pdf",
		"mimetype": "application/pdf",
		"manual":   true,
	})
	widgetJSON, err := json.Marshal(widgetRows)
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := server.Env.Model("account.move.send.wizard").Create(map[string]any{
		"move_id":                 moveID,
		"model":                   "account.move",
		"res_ids":                 fmt.Sprintf("%d", moveID),
		"render_model":            "account.move",
		"sending_methods":         `{"email":true}`,
		"template_id":             templateID,
		"subject":                 defaults["subject"],
		"body":                    defaults["body"],
		"mail_partner_ids":        []int64{7},
		"mail_attachments_widget": string(widgetJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.wizard/action_send_and_print", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("send wizard response %d %s", rec.Code, rec.Body.String())
	}
	moveRows, err := server.Env.Model("account.move").Browse(moveID).Read("invoice_pdf_report_id")
	if err != nil {
		t.Fatal(err)
	}
	invoiceAttachmentID := int64Value(moveRows[0]["invoice_pdf_report_id"])
	messages, err := server.Env.Model("mail.message").Search(domain.Cond("subject", "=", "Template INV/WIDGET"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 {
		t.Fatalf("messages = %+v", messageRows)
	}
	messageID := int64Value(messageRows[0]["id"])
	messageAttachmentIDs := int64Slice(messageRows[0]["attachment_ids"])
	if len(messageAttachmentIDs) != 3 || testContainsRecordID(messageAttachmentIDs, invoiceAttachmentID) || testContainsRecordID(messageAttachmentIDs, templateAttachmentID) || testContainsRecordID(messageAttachmentIDs, manualAttachmentID) {
		t.Fatalf("message attachment ids = %#v originals=%d,%d,%d", messageRows[0]["attachment_ids"], invoiceAttachmentID, templateAttachmentID, manualAttachmentID)
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Browse(messageAttachmentIDs...).Read("name", "datas", "res_model", "res_field", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	var sawInvoiceCopy, sawManualCopy, sawExtraReportCopy bool
	for _, row := range attachmentRows {
		if row["res_model"] != "mail.message" || stringValue(row["res_field"]) != "" || row["res_id"] != messageID {
			t.Fatalf("attachment owner = %+v", row)
		}
		if row["name"] == "INV/WIDGET.pdf" {
			sawInvoiceCopy = true
		}
		if row["name"] == "terms.pdf" && string(byteValue(row["datas"])) == "manual terms" {
			sawManualCopy = true
		}
		if row["name"] == "Extra - INV/WIDGET.pdf" {
			sawExtraReportCopy = true
		}
		if row["name"] == "Invoice Copy - INV/WIDGET.pdf" {
			t.Fatalf("invoice report dynamic duplicate was copied: %+v", row)
		}
		if row["name"] == "terms.pdf" && string(byteValue(row["datas"])) == "template terms" {
			t.Fatalf("skipped template attachment was copied: %+v", row)
		}
	}
	if !sawInvoiceCopy || !sawManualCopy || !sawExtraReportCopy {
		t.Fatalf("attachment rows = %+v", attachmentRows)
	}
	mailRows, err := server.Env.Model("mail.mail").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		t.Fatal(err)
	}
	mails, err := mailRows.Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || fmt.Sprint(mails[0]["attachment_ids"]) != fmt.Sprint(messageAttachmentIDs) {
		t.Fatalf("mail attachments = %+v message=%+v", mails, messageAttachmentIDs)
	}
}

func TestAccountMoveSendWizardCachesDynamicReportAttachment(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/CACHE", "out_invoice", "posted")
	extraReportID, err := server.Env.Model("ir.actions.report").Create(map[string]any{
		"name":              "Cached Extra Report",
		"type":              "ir.actions.report",
		"model":             "account.move",
		"report_name":       "x.account_cached_extra",
		"report_type":       "qweb-pdf",
		"print_report_name": "'Outgoing Extra - %s' % (object.name)",
		"attachment":        "'Cached Extra - %s.pdf' % (object.name)",
		"attachment_use":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{
		"name":                "Cached Invoice Template",
		"model":               "account.move",
		"subject":             "Cached {{ name }}",
		"body_html":           "<p>{{ object.name }}</p>",
		"report_template_ids": []int64{extraReportID},
		"active":              true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"Cache first", "Cache second"} {
		wizardID, err := server.Env.Model("account.move.send.wizard").Create(map[string]any{
			"move_id":          moveID,
			"model":            "account.move",
			"res_ids":          fmt.Sprintf("%d", moveID),
			"render_model":     "account.move",
			"sending_methods":  `{"email":true}`,
			"template_id":      templateID,
			"subject":          subject,
			"body":             "<p>Cache</p>",
			"mail_partner_ids": []int64{7},
		})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.wizard/action_send_and_print", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("send wizard response %d %s", rec.Code, rec.Body.String())
		}
		if subject == "Cache first" {
			cacheRows, err := server.Env.Model("ir.attachment").Search(domain.And(
				domain.Cond("name", domain.Equal, "Cached Extra - INV/CACHE.pdf"),
				domain.Cond("res_model", domain.Equal, "account.move"),
				domain.Cond("res_id", domain.Equal, moveID),
			))
			if err != nil {
				t.Fatal(err)
			}
			cacheIDs := cacheRows.IDs()
			if len(cacheIDs) != 1 {
				t.Fatalf("cache ids after first send = %+v", cacheIDs)
			}
			if err := server.Env.Model("ir.attachment").Browse(cacheIDs[0]).Write(map[string]any{"datas": []byte("%PDF-cached-extra"), "file_size": len("%PDF-cached-extra")}); err != nil {
				t.Fatal(err)
			}
		}
	}
	messages, err := server.Env.Model("mail.message").Search(domain.Cond("subject", "=", "Cache second"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 {
		t.Fatalf("messages = %+v", messageRows)
	}
	attachmentIDs := int64Slice(messageRows[0]["attachment_ids"])
	attachmentRows, err := server.Env.Model("ir.attachment").Browse(attachmentIDs...).Read("name", "res_model", "datas")
	if err != nil {
		t.Fatal(err)
	}
	var sawCachedExtra bool
	for _, row := range attachmentRows {
		if row["name"] == "Outgoing Extra - INV/CACHE.pdf" {
			sawCachedExtra = row["res_model"] == "mail.message" && string(byteValue(row["datas"])) == "%PDF-cached-extra"
		}
	}
	if !sawCachedExtra {
		t.Fatalf("second message attachments = %+v", attachmentRows)
	}
	cacheRows, err := server.Env.Model("ir.attachment").Search(domain.And(
		domain.Cond("name", domain.Equal, "Cached Extra - INV/CACHE.pdf"),
		domain.Cond("res_model", domain.Equal, "account.move"),
		domain.Cond("res_id", domain.Equal, moveID),
	))
	if err != nil {
		t.Fatal(err)
	}
	if cacheIDs := cacheRows.IDs(); len(cacheIDs) != 1 {
		t.Fatalf("cache ids after second send = %+v", cacheIDs)
	}
}

func TestAccountMoveSendWizardAddsUBLPostprocessAttachment(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/UBL", "out_invoice", "posted")
	xmlID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "INV-UBL.xml",
		"res_model": "account.move",
		"res_field": "ubl_cii_xml_file",
		"res_id":    moveID,
		"type":      "binary",
		"mimetype":  "application/xml",
		"datas":     []byte("<Invoice/>"),
		"file_size": len("<Invoice/>"),
	})
	if err != nil {
		t.Fatal(err)
	}
	templateID, err := server.Env.Model("mail.template").Create(map[string]any{
		"name":      "Invoice UBL Template",
		"model":     "account.move",
		"subject":   "UBL {{ name }}",
		"body_html": "<p>{{ object.name }}</p>",
		"active":    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wizardID, err := server.Env.Model("account.move.send.wizard").Create(map[string]any{
		"move_id":          moveID,
		"model":            "account.move",
		"res_ids":          fmt.Sprintf("%d", moveID),
		"render_model":     "account.move",
		"sending_methods":  `{"email":true}`,
		"template_id":      templateID,
		"subject":          "UBL INV/UBL",
		"body":             "<p>UBL</p>",
		"mail_partner_ids": []int64{7},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.wizard/action_send_and_print", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("send wizard response %d %s", rec.Code, rec.Body.String())
	}
	messages, err := server.Env.Model("mail.message").Search(domain.Cond("subject", "=", "UBL INV/UBL"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := messages.Read("id", "attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 {
		t.Fatalf("messages = %+v", messageRows)
	}
	messageID := int64Value(messageRows[0]["id"])
	attachmentIDs := int64Slice(messageRows[0]["attachment_ids"])
	if len(attachmentIDs) != 2 || testContainsRecordID(attachmentIDs, xmlID) {
		t.Fatalf("message attachment ids = %#v original xml=%d", messageRows[0]["attachment_ids"], xmlID)
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Browse(attachmentIDs...).Read("name", "res_model", "res_id", "res_field", "mimetype", "datas")
	if err != nil {
		t.Fatal(err)
	}
	var sawInvoicePDF, sawUBLXML bool
	for _, row := range attachmentRows {
		if row["res_model"] != "mail.message" || row["res_id"] != messageID || stringValue(row["res_field"]) != "" {
			t.Fatalf("message attachment owner = %+v", row)
		}
		if row["name"] == "INV/UBL.pdf" && row["mimetype"] == "application/pdf" {
			sawInvoicePDF = true
		}
		if row["name"] == "INV-UBL.xml" && row["mimetype"] == "application/xml" && string(byteValue(row["datas"])) == "<Invoice/>" {
			sawUBLXML = true
		}
	}
	if !sawInvoicePDF || !sawUBLXML {
		t.Fatalf("attachment rows = %+v", attachmentRows)
	}
	mailRows, err := server.Env.Model("mail.mail").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		t.Fatal(err)
	}
	mails, err := mailRows.Read("attachment_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(mails) != 1 || fmt.Sprint(mails[0]["attachment_ids"]) != fmt.Sprint(attachmentIDs) {
		t.Fatalf("mail attachments = %+v message=%+v", mails, attachmentIDs)
	}
}

func TestAccountingInvoiceReportRefresh(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/REPORT", "out_invoice", "posted")
	lines, err := server.Env.Model("account.move.line").Search(domain.And(
		domain.Cond("move_id", domain.Equal, moveID),
		domain.Cond("account_type", domain.Equal, string(coreaccounting.AccountIncome)),
	))
	if err != nil {
		t.Fatal(err)
	}
	if lines.Len() != 1 {
		t.Fatalf("income lines = %+v", lines.IDs())
	}
	if err := lines.Write(map[string]any{
		"display_type":        "product",
		"quantity":            float64(2),
		"price_subtotal":      int64(10000),
		"price_total":         int64(11000),
		"product_id":          int64(30),
		"product_uom_id":      int64(40),
		"product_category_id": int64(50),
	}); err != nil {
		t.Fatal(err)
	}
	if err := refreshAccountingInvoiceReport(server.Env, []int64{moveID}); err != nil {
		t.Fatal(err)
	}
	reportSet, err := server.Env.Model("account.invoice.report").Search(domain.Cond("move_id", domain.Equal, moveID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := reportSet.Read("move_id", "journal_id", "move_type", "state", "quantity", "account_id", "product_id", "product_categ_id", "price_subtotal", "price_total_currency", "price_average", "price_margin")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["move_id"] != moveID || rows[0]["move_type"] != "out_invoice" || rows[0]["state"] != "posted" {
		t.Fatalf("report identity rows = %+v", rows)
	}
	if rows[0]["quantity"] != float64(2) || rows[0]["product_id"] != int64(30) || rows[0]["product_categ_id"] != int64(50) || rows[0]["price_subtotal"] != int64(10000) || rows[0]["price_total_currency"] != int64(11000) || rows[0]["price_average"] != float64(5000) || rows[0]["price_margin"] != int64(10000) {
		t.Fatalf("report amounts = %+v", rows[0])
	}

	if err := lines.Write(map[string]any{"quantity": float64(4), "credit": int64(12000), "amount_currency": int64(-12000), "price_subtotal": int64(12000), "price_total": int64(13200)}); err != nil {
		t.Fatal(err)
	}
	if err := refreshAccountingInvoiceReport(server.Env, []int64{moveID}); err != nil {
		t.Fatal(err)
	}
	reportSet, err = server.Env.Model("account.invoice.report").Search(domain.Cond("move_id", domain.Equal, moveID))
	if err != nil {
		t.Fatal(err)
	}
	rows, err = reportSet.Read("quantity", "price_subtotal", "price_average")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["quantity"] != float64(4) || rows[0]["price_subtotal"] != int64(12000) || rows[0]["price_average"] != float64(3000) {
		t.Fatalf("refreshed rows = %+v", rows)
	}
}

func TestAccountMoveSendBatchWizard(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID1, _ := createHTTPAccountingMove(t, server.Env, "INV/BATCH1", "out_invoice", "posted")
	moveID2, _ := createHTTPAccountingMove(t, server.Env, "INV/BATCH2", "out_invoice", "posted")
	handler := server.Handler()

	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d,%d]]}`, moveID1, moveID2))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_send_and_print", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("batch send action response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["res_model"] != "account.move.send.batch.wizard" {
		t.Fatalf("batch send action = %#v", action)
	}
	wizardID, err := server.Env.Model("account.move.send.batch.wizard").Create(map[string]any{"move_ids": []int64{moveID1, moveID2}})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.batch.wizard/action_send_and_print", body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "display_notification") {
		t.Fatalf("batch send response %d %s", rec.Code, rec.Body.String())
	}
	rows, err := server.Env.Model("account.move").Browse(moveID1, moveID2).Read("is_move_sent")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["is_move_sent"] != true || rows[1]["is_move_sent"] != true {
		t.Fatalf("batch sent moves = %+v", rows)
	}
}

func TestAccountMoveSendRejectsVendorBill(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "BILL/SEND", "in_invoice", "posted")
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, moveID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_send_and_print", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("vendor send response %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountMoveSendManualReturnsDownloadAction(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID, _ := createHTTPAccountingMove(t, server.Env, "INV/MANUAL", "out_invoice", "posted")
	wizardID, err := server.Env.Model("account.move.send.wizard").Create(map[string]any{
		"move_id":         moveID,
		"model":           "account.move",
		"res_ids":         fmt.Sprintf("%d", moveID),
		"render_model":    "account.move",
		"sending_methods": `{"manual":true}`,
		"subject":         "Invoice INV/MANUAL",
		"body":            "Body",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, wizardID))
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move.send.wizard/action_send_and_print", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("manual send response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["type"] != "ir.actions.act_url" || action["target"] != "download" || action["url"] != fmt.Sprintf("/account/download_invoice_documents/%d/pdf", moveID) {
		t.Fatalf("manual action = %#v", action)
	}
	mailRows, err := server.Env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows.IDs()) != 0 {
		t.Fatalf("manual send created mail rows: %+v", mailRows.IDs())
	}
}

func TestAccountMoveDownloadInvoiceDocumentsPDFAndZip(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID1, _ := createHTTPAccountingMove(t, server.Env, "INV/DOC1", "out_invoice", "posted")
	moveID2, _ := createHTTPAccountingMove(t, server.Env, "INV/DOC2", "out_invoice", "posted")
	handler := server.Handler()
	existingPDFID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "INV/DOC1.pdf",
		"res_model": "account.move",
		"res_field": "invoice_pdf_report_file",
		"res_id":    moveID1,
		"type":      "binary",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_documents/%d/pdf", moveID1), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("single pdf response %d headers=%+v body=%q", rec.Code, rec.Header(), rec.Body.String())
	}
	moveRows, err := server.Env.Model("account.move").Browse(moveID1).Read("invoice_pdf_report_id", "invoice_pdf_report_file", "message_main_attachment_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(moveRows) != 1 || int64Value(moveRows[0]["invoice_pdf_report_id"]) != existingPDFID || int64Value(moveRows[0]["message_main_attachment_id"]) != existingPDFID || !bytes.HasPrefix(byteValue(moveRows[0]["invoice_pdf_report_file"]), []byte("%PDF-")) {
		t.Fatalf("move pdf fields = %+v", moveRows)
	}
	attachmentRows, err := server.Env.Model("ir.attachment").Search(domain.And(
		domain.Cond("res_model", domain.Equal, "account.move"),
		domain.Cond("res_id", domain.Equal, moveID1),
		domain.Cond("res_field", domain.Equal, "invoice_pdf_report_file"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if ids := attachmentRows.IDs(); len(ids) != 1 || ids[0] != existingPDFID {
		t.Fatalf("invoice pdf attachment ids = %+v", ids)
	}
	existingRows, err := server.Env.Model("ir.attachment").Browse(existingPDFID).Read("mimetype", "datas", "file_size")
	if err != nil {
		t.Fatal(err)
	}
	if len(existingRows) != 1 || existingRows[0]["mimetype"] != "application/pdf" || !bytes.HasPrefix(byteValue(existingRows[0]["datas"]), []byte("%PDF-")) || int64Value(existingRows[0]["file_size"]) != int64(len(byteValue(existingRows[0]["datas"]))) {
		t.Fatalf("backfilled attachment = %+v", existingRows)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_documents/%d,%d/pdf", moveID1, moveID2), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("zip response %d headers=%+v body=%q", rec.Code, rec.Header(), rec.Body.String())
	}
	names := zipEntryNames(t, rec.Body.Bytes())
	if len(names) != 2 || names[0] != "INV/DOC1.pdf" || names[1] != "INV/DOC2.pdf" {
		t.Fatalf("zip names = %+v", names)
	}
}

func TestEnsureInvoicePDFPlaceholderIgnoresWrongStoredAttachment(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID1, _ := createHTTPAccountingMove(t, server.Env, "INV/WRONG1", "out_invoice", "posted")
	moveID2, _ := createHTTPAccountingMove(t, server.Env, "INV/WRONG2", "out_invoice", "posted")
	wrongAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "wrong.pdf",
		"res_model": "account.move",
		"res_field": "invoice_pdf_report_file",
		"res_id":    moveID2,
		"type":      "binary",
		"mimetype":  "application/pdf",
		"datas":     validInvoicePDFBytes(coreaccounting.Move{ID: moveID2, Name: "INV/WRONG2"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Env.Model("account.move").Browse(moveID1).Write(map[string]any{"invoice_pdf_report_id": wrongAttachmentID}); err != nil {
		t.Fatal(err)
	}
	moves, err := readAccountingMoves(server.Env, []int64{moveID1})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, err := ensureInvoicePDFPlaceholder(server.Env, moves[0])
	if err != nil {
		t.Fatal(err)
	}
	if attachmentID == wrongAttachmentID {
		t.Fatalf("wrong attachment reused: %d", attachmentID)
	}
	wrongRows, err := server.Env.Model("ir.attachment").Browse(wrongAttachmentID).Read("res_model", "res_field", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(wrongRows) != 1 || wrongRows[0]["res_model"] != "account.move" || wrongRows[0]["res_field"] != "invoice_pdf_report_file" || wrongRows[0]["res_id"] != moveID2 {
		t.Fatalf("wrong attachment was mutated = %+v", wrongRows)
	}
	rightRows, err := server.Env.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_field", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rightRows) != 1 || rightRows[0]["res_model"] != "account.move" || rightRows[0]["res_field"] != "invoice_pdf_report_file" || rightRows[0]["res_id"] != moveID1 {
		t.Fatalf("created attachment = %+v", rightRows)
	}
}

func TestAccountMoveDownloadInvoiceAttachmentsAndMoveAll(t *testing.T) {
	server := testAccountingDispatchServer(t)
	moveID1, _ := createHTTPAccountingMove(t, server.Env, "INV/ATT1", "out_invoice", "posted")
	moveID2, _ := createHTTPAccountingMove(t, server.Env, "INV/ATT2", "out_invoice", "posted")
	handler := server.Handler()
	for _, moveID := range []int64{moveID1, moveID2} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_documents/%d/pdf", moveID), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("prepare pdf %d response %d %s", moveID, rec.Code, rec.Body.String())
		}
	}
	moveRows, err := server.Env.Model("account.move").Browse(moveID1, moveID2).Read("invoice_pdf_report_id")
	if err != nil {
		t.Fatal(err)
	}
	attachmentIDs := []int64{int64Value(moveRows[0]["invoice_pdf_report_id"]), int64Value(moveRows[1]["invoice_pdf_report_id"])}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_attachments/%d", attachmentIDs[0]), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("single attachment response %d headers=%+v", rec.Code, rec.Header())
	}
	genericAttachmentID, err := server.Env.Model("ir.attachment").Create(map[string]any{
		"name":      "generic.pdf",
		"res_model": "account.move",
		"res_id":    moveID1,
		"type":      "binary",
		"mimetype":  "application/pdf",
		"datas":     validInvoicePDFBytes(coreaccounting.Move{ID: moveID1, Name: "INV/ATT1"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_attachments/%d", genericAttachmentID), nil))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not an invoice PDF") {
		t.Fatalf("generic attachment response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/account/download_invoice_attachments/%d,%d", attachmentIDs[0], attachmentIDs[1]), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("attachment zip response %d headers=%+v body=%q", rec.Code, rec.Header(), rec.Body.String())
	}
	if names := zipEntryNames(t, rec.Body.Bytes()); len(names) != 2 || names[0] != "INV/ATT1.pdf" || names[1] != "INV/ATT2.pdf" {
		t.Fatalf("attachment zip names = %+v", names)
	}

	rec = httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d,%d]]}`, moveID1, moveID2))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_move_download_all", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("download all action response %d %s", rec.Code, rec.Body.String())
	}
	action := decodeJSON(t, rec.Body.Bytes())
	if action["url"] != fmt.Sprintf("/account/download_invoice_documents/%d,%d/all", moveID1, moveID2) || action["target"] != "download" {
		t.Fatalf("download all action = %#v", action)
	}
}

func TestAccountPaymentRegisterValidation(t *testing.T) {
	server := testAccountingDispatchServer(t)
	draftID, journalID := createHTTPAccountingMove(t, server.Env, "INV/DRAFTPAY", "out_invoice", "draft")
	wizardID, err := server.Env.Model("account.payment.register").Create(map[string]any{
		"payment_date": dateValue(2026, 8, 1),
		"amount":       int64(10000),
		"journal_id":   journalID,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]],"kwargs":{"context":{"active_model":"res.partner","active_ids":[%d]}}}`, wizardID, draftID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.payment.register/action_create_payments", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invalid active model response %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]],"kwargs":{"context":{"active_model":"account.move","active_ids":[%d]}}}`, wizardID, draftID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.payment.register/action_create_payments", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrPaymentRegisterNoMoves.Error()) {
		t.Fatalf("draft payment response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = bytes.NewBufferString(fmt.Sprintf(`{"args":[[%d]]}`, draftID))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button/account.move/action_register_payment", body))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), coreaccounting.ErrPaymentRegisterNoMoves.Error()) {
		t.Fatalf("draft action_register_payment response %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsSessionInfoIncludesImpersonationContext(t *testing.T) {
	server := testServer(t)
	server.Impersonation = testImpersonationService()
	if _, err := server.Impersonation.Start("sid", 1, 20, impersonation.SwitchOptions{GroupID: 30, ReturnTo: "/web#menu_id=1"}); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/session/get_session_info", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["uid"] != float64(20) || payload["impersonate"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	context := payload["user_context"].(map[string]any)
	if context["login_as"] != true || context["login_as_original_uid"] != float64(1) || context["login_as_back_route"] != "/web/login_back" {
		t.Fatalf("context = %#v", context)
	}
	loginAs := payload["login_as"].(map[string]any)
	if loginAs["active"] != true || loginAs["effective_uid"] != float64(20) {
		t.Fatalf("login_as = %#v", loginAs)
	}
}

func TestLoginAsDatasetRoutesUseEffectiveUserContext(t *testing.T) {
	server := testServer(t)
	server.Impersonation = impersonation.NewService()
	server.Impersonation.SetUser(impersonation.User{ID: 1, Login: "admin", Name: "Admin", Active: true, GroupIDs: []int64{1}, CompanyID: 1, CompanyIDs: []int64{1}})
	server.Impersonation.SetUser(impersonation.User{ID: 20, Login: "target", Name: "Target", Active: true, GroupIDs: []int64{30, 40}, CompanyID: 2, CompanyIDs: []int64{2, 3}})
	if _, err := server.Impersonation.Start("sid", 1, 20, impersonation.SwitchOptions{ReturnTo: "/web"}); err != nil {
		t.Fatal(err)
	}
	policy := &capturePolicy{}
	server.Env.WithPolicy(policy)
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(`{"model":"res.partner","method":"create","values":{"name":"Impersonated"}}`))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("call_kw response %d %s", rec.Code, rec.Body.String())
	}
	if len(policy.checks) == 0 {
		t.Fatal("policy did not observe dataset call")
	}
	ctx := policy.checks[0]
	if ctx.UserID != 20 || ctx.CompanyID != 2 {
		t.Fatalf("context = %+v", ctx)
	}
	if got := int64Slice(ctx.Values["group_ids"]); len(got) != 2 || got[0] != 30 || got[1] != 40 {
		t.Fatalf("group_ids = %+v", ctx.Values["group_ids"])
	}
	if ctx.Values["login_as"] != true || ctx.Values["login_as_original_uid"] != int64(1) {
		t.Fatalf("login_as context = %+v", ctx.Values)
	}

	server.Menus.Add(menu.Menu{Name: "Impersonated Menu", Groups: []int64{40}})
	noSessionMenus := httptest.NewRecorder()
	handler.ServeHTTP(noSessionMenus, httptest.NewRequest(http.MethodGet, "/web/webclient/load_menus", nil))
	if strings.Contains(noSessionMenus.Body.String(), "Impersonated Menu") {
		t.Fatalf("unimpersonated menus leaked restricted menu: %s", noSessionMenus.Body.String())
	}
	menuReq := httptest.NewRequest(http.MethodGet, "/web/webclient/load_menus", nil)
	menuReq.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	menuRec := httptest.NewRecorder()
	handler.ServeHTTP(menuRec, menuReq)
	if menuRec.Code != http.StatusOK || !strings.Contains(menuRec.Body.String(), "Impersonated Menu") {
		t.Fatalf("impersonated menus response %d %s", menuRec.Code, menuRec.Body.String())
	}

	checkReq := httptest.NewRequest(http.MethodGet, "/web/session/check", nil)
	checkReq.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, checkReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"uid":20`) {
		t.Fatalf("session check response %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsHTTPRejectsUnsafeInputs(t *testing.T) {
	server := testServer(t)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/login_as/not-an-id?session_id=sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing service status = %d", rec.Code)
	}

	server.Impersonation = testImpersonationService()
	handler = server.Handler()
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_as/not-an-id?session_id=sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_as/20?session_id=sid&redirect=https://example.com", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/web" {
		t.Fatalf("unsafe redirect response %d %s", rec.Code, rec.Header().Get("Location"))
	}

	unauthorized := testServer(t)
	unauthorized.Env = record.NewEnv(unauthorized.EnvRegistryForTest(t), record.Context{UserID: 50, CompanyID: 1})
	unauthorized.Impersonation = testImpersonationService()
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_as/20?session_id=sid", nil)
	unauthorized.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsHTTPPersistsAuditRows(t *testing.T) {
	server := testLoginAsAuditServer(t, 1, testImpersonationService())
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/login_as/20?session_id=sid&reason=ticket-1", nil)
	req.Header.Set("X-Real-IP", "192.0.2.10")
	req.Header.Set("User-Agent", "audit-test")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login_as response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_back?session_id=sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login_back response %d %s", rec.Code, rec.Body.String())
	}

	rows := readLoginAsAuditRows(t, server)
	if len(rows) != 2 {
		t.Fatalf("audit rows = %#v", rows)
	}
	if rows[0]["action"] != "login_as.start" || rows[0]["actor_id"] != int64(1) || rows[0]["target_user_id"] != int64(20) {
		t.Fatalf("start audit row = %#v", rows[0])
	}
	if rows[0]["ip_address"] != "192.0.2.10" || rows[0]["user_agent"] != "audit-test" || !strings.Contains(rows[0]["details"].(string), "ticket-1") {
		t.Fatalf("start audit metadata = %#v", rows[0])
	}
	if rows[1]["action"] != "login_as.back" || rows[1]["actor_id"] != int64(1) || rows[1]["effective_user_id"] != int64(20) {
		t.Fatalf("back audit row = %#v", rows[1])
	}
}

func TestLoginAsSecurityModeRequiresCookieSession(t *testing.T) {
	server := testLoginAsAuditServer(t, 50, testImpersonationService())
	server.Env = server.Env.WithContext(record.Context{UserID: 50, CompanyID: 1, CompanyIDs: []int64{1}})
	engine := security.NewEngine()
	engine.Users[1] = security.User{ID: 1, Login: "admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{1}}
	engine.Users[50] = security.User{ID: 50, Login: "base", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{99}}
	engine.ACLs = append(engine.ACLs, security.ACL{
		Model:      oi_login_as.ModelLoginAsAudit,
		GroupID:    99,
		Active:     true,
		PermRead:   true,
		PermCreate: true,
	})
	engine.IssueSession(1, "secure-sid", time.Now().Add(time.Hour))
	server.Security = engine
	server.Env.WithPolicy(engine)
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/login_as/20?session_id=secure-sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("query login_as response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_as/20?reason=security-mode", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "secure-sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("cookie login_as response %d %s", rec.Code, rec.Body.String())
	}
	session, ok := server.Impersonation.Session("secure-sid")
	if !ok || !session.Impersonating || session.UserID != 20 || session.OriginalUserID != 1 {
		t.Fatalf("session = %+v ok=%v", session, ok)
	}
	rows := readLoginAsAuditRows(t, server)
	if len(rows) != 1 || rows[0]["actor_id"] != int64(1) || rows[0]["target_user_id"] != int64(20) {
		t.Fatalf("audit rows = %#v", rows)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/session/info?session_id=secure-sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["uid"] != float64(0) || payload["impersonate"] == true {
		t.Fatalf("query session_info payload = %#v", payload)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/session/info", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "secure-sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload = decodeJSON(t, rec.Body.Bytes())
	if payload["uid"] != float64(20) || payload["impersonate"] != true {
		t.Fatalf("cookie session_info payload = %#v", payload)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_back?session_id=secure-sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("query login_back response %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/login_back", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "secure-sid"})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("cookie login_back response %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsDebugRouteSwitchesToSystemWhenEnabled(t *testing.T) {
	server := testLoginAsAuditServer(t, 12, testDebugImpersonationService(true))
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/become/debug?session_id=sid&redirect=/web%23debug=1&reason=inspect", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/web#debug=1" {
		t.Fatalf("debug response %d %s", rec.Code, rec.Header().Get("Location"))
	}
	session, ok := server.Impersonation.Session("sid")
	if !ok || !session.Impersonating || session.UserID != 1 || session.OriginalUserID != 12 {
		t.Fatalf("debug session = %+v ok=%v", session, ok)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/web/session/info?session_id=sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session_info response %d %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSON(t, rec.Body.Bytes())
	if payload["uid"] != float64(1) || payload["impersonate"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	loginAs := payload["login_as"].(map[string]any)
	if loginAs["active"] != true || loginAs["original_uid"] != float64(12) || loginAs["effective_uid"] != float64(1) {
		t.Fatalf("login_as = %#v", loginAs)
	}

	rows := readLoginAsAuditRows(t, server)
	if len(rows) != 1 || rows[0]["action"] != "login_as.start" || rows[0]["actor_id"] != int64(12) || rows[0]["target_user_id"] != int64(1) {
		t.Fatalf("debug audit rows = %#v", rows)
	}
	if !strings.Contains(rows[0]["details"].(string), "inspect") {
		t.Fatalf("debug audit details = %#v", rows[0])
	}
}

func TestLoginAsDebugRouteDisabledIsForbiddenAndAudited(t *testing.T) {
	server := testLoginAsAuditServer(t, 12, testDebugImpersonationService(false))
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/become/debug?session_id=sid", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("debug disabled response %d %s", rec.Code, rec.Body.String())
	}
	rows := readLoginAsAuditRows(t, server)
	if len(rows) != 1 || rows[0]["action"] != "login_as.debug_denied" || rows[0]["actor_id"] != int64(12) || rows[0]["target_user_id"] != int64(1) {
		t.Fatalf("debug disabled audit rows = %#v", rows)
	}
}

func TestActionPayloadIncludesEmbeddedActions(t *testing.T) {
	payload := actionPayload(action.Action{
		ID:       7,
		Name:     "Parent",
		Kind:     action.ActWindow,
		ResModel: "res.partner",
		ViewID:   12,
		ViewMode: "form",
		EmbeddedActions: []action.EmbeddedAction{{
			ID:              11,
			Name:            "Open Lines",
			ParentActionID:  7,
			ParentResID:     42,
			ParentResModel:  "res.partner",
			ActionID:        9,
			PythonMethod:    "action_open_lines",
			UserID:          3,
			IsDeletable:     true,
			DefaultViewMode: "list",
			FilterIDs:       []int64{4, 5},
			Domain:          `[["active","=",true]]`,
			Context:         `{"default_partner_id": 42}`,
			GroupIDs:        []int64{6},
		}},
	})
	embedded, ok := payload["embedded_action_ids"].([]map[string]any)
	if !ok || len(embedded) != 1 {
		t.Fatalf("embedded actions = %#v", payload["embedded_action_ids"])
	}
	item := embedded[0]
	if item["id"] != int64(11) || item["name"] != "Open Lines" || item["parent_res_id"] != int64(42) || item["parent_res_model"] != "res.partner" {
		t.Fatalf("embedded item = %#v", item)
	}
	views := payload["views"].([]any)
	viewPair := views[0].([]any)
	if int64Value(viewPair[0]) != 12 || viewPair[1] != "form" {
		t.Fatalf("views = %#v", views)
	}
	if item["python_method"] != "action_open_lines" || item["default_view_mode"] != "list" || item["domain"] != `[["active","=",true]]` || item["context"] != `{"default_partner_id": 42}` {
		t.Fatalf("embedded method/context = %#v", item)
	}
	if ids := item["filter_ids"].([]int64); len(ids) != 2 || ids[0] != 4 || ids[1] != 5 {
		t.Fatalf("filter ids = %#v", item["filter_ids"])
	}
	if groups := item["groups_ids"].([]int64); len(groups) != 1 || groups[0] != 6 {
		t.Fatalf("groups = %#v", item["groups_ids"])
	}
}

func testServer(t *testing.T) Server {
	t.Helper()
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	assetReg := assets.NewRegistry()
	if err := assetReg.Apply(assets.Backend, assets.Operation{Kind: assets.Append, Path: "webclient.js"}); err != nil {
		t.Fatal(err)
	}
	actionReg := action.NewRegistry()
	partnerActionID, err := actionReg.Add(action.Action{
		XMLID:            "base.action_partner",
		Name:             "Partners",
		Kind:             action.ActWindow,
		ResModel:         "res.partner",
		ViewMode:         "list,form",
		ViewID:           8,
		SearchViewID:     9,
		Views:            []action.ViewRef{{ID: 8, Mode: "list"}, {Mode: "form"}},
		Domain:           "[]",
		Context:          map[string]any{"search_default_customer": true},
		Target:           "current",
		Limit:            25,
		Help:             "<p>Create a partner</p>",
		Path:             "partners",
		BindingModelID:   3,
		BindingType:      "action",
		BindingViewTypes: "list,form",
	})
	if err != nil {
		t.Fatal(err)
	}
	menuReg := menu.NewRegistry()
	settingsID := menuReg.Add(menu.Menu{Name: "Settings", XMLID: "base.menu_settings", WebIcon: "fa-cog,#ffffff,#000000"})
	menuReg.Add(menu.Menu{
		Name:                "Partners",
		ParentID:            settingsID,
		Action:              "ir.actions.act_window,base.action_partner",
		ActionID:            partnerActionID,
		ActionModel:         "ir.actions.act_window",
		ActionPath:          "partners",
		Sequence:            10,
		WebIconData:         "iVBORw0KGgo=",
		WebIconDataMimetype: "image/png",
		XMLID:               "base.menu_partner",
	})
	viewReg := view.NewRegistry()
	if err := viewReg.AddWithID(view.View{ID: 8, Name: "Partner List", Model: "res.partner", Type: view.List, Arch: `<list><field name="name"/></list>`}); err != nil {
		t.Fatal(err)
	}
	if err := viewReg.AddWithID(view.View{ID: 9, Name: "Partner Search", Model: "res.partner", Type: view.Search, Arch: `<search><field name="name"/></search>`}); err != nil {
		t.Fatal(err)
	}
	if err := viewReg.AddWithID(view.View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: view.Form, Arch: `<form><field name="name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	return Server{Env: env, Assets: assetReg, Actions: actionReg, Menus: menuReg, Views: viewReg}
}

func testToolbarBindingServer(t *testing.T) Server {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 2, CompanyID: 1, CompanyIDs: []int64{1}})
	externalIDs := map[string]data.ExternalID{}
	if err := data.LoadModelMetadata(env, "base", internalbase.Models(), externalIDs); err != nil {
		t.Fatal(err)
	}
	partnerModelID := testModelRecordID(t, env, "res.partner")
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Hidden Group"})
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{
			"name":               "Second Server Action",
			"model_id":           partnerModelID,
			"binding_model_id":   partnerModelID,
			"binding_type":       "action",
			"binding_view_types": "list",
			"state":              "code",
			"sequence":           int64(20),
			"active":             true,
		},
		{
			"name":               "First Server Action",
			"model_id":           partnerModelID,
			"binding_model_id":   partnerModelID,
			"binding_type":       "action",
			"binding_view_types": "list",
			"state":              "code",
			"sequence":           int64(10),
			"active":             true,
		},
		{
			"name":               "Hidden Server Action",
			"model_id":           partnerModelID,
			"binding_model_id":   partnerModelID,
			"binding_type":       "action",
			"binding_view_types": "list",
			"state":              "code",
			"sequence":           int64(5),
			"group_ids":          []int64{groupID},
			"active":             true,
		},
	} {
		if _, err := env.Model("ir.actions.server").Create(values); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Model("ir.actions.act_window").Create(map[string]any{
		"name":               "Partner Wizard",
		"type":               "ir.actions.act_window",
		"res_model":          "partner.wizard",
		"target":             "new",
		"binding_model_id":   partnerModelID,
		"binding_type":       "action",
		"binding_view_types": "form",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.actions.report").Create(map[string]any{
		"name":               "Partner Label",
		"type":               "ir.actions.report",
		"model":              "res.partner",
		"report_name":        "x_partner.label",
		"report_type":        "qweb-pdf",
		"binding_model_id":   partnerModelID,
		"binding_type":       "report",
		"binding_view_types": "list,form",
		"domain":             `[("active","=",True)]`,
	}); err != nil {
		t.Fatal(err)
	}
	viewReg := view.NewRegistry()
	for _, item := range []view.View{
		{ID: 101, Name: "Partner List", Model: "res.partner", Type: view.List, Arch: `<list><field name="name"/></list>`},
		{ID: 102, Name: "Partner Form", Model: "res.partner", Type: view.Form, Arch: `<form><field name="name"/></form>`},
		{ID: 103, Name: "Partner Search", Model: "res.partner", Type: view.Search, Arch: `<search><field name="name"/></search>`},
	} {
		if err := viewReg.AddWithID(item); err != nil {
			t.Fatal(err)
		}
	}
	return Server{Env: env, Views: viewReg}
}

func testModelRecordID(t *testing.T, env *record.Env, modelName string) int64 {
	t.Helper()
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, modelName), record.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("model %s not found", modelName)
	}
	return int64Value(rows[0]["id"])
}

func testViewGroupXMLIDServer(t *testing.T) (Server, map[string]int64) {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	loader := data.NewLoader(env, "base")
	if err := loader.LoadXML(strings.NewReader(`<odoo>
  <record id="group_user" model="res.groups">
    <field name="name">Role / User</field>
  </record>
  <record id="group_system" model="res.groups">
    <field name="name">Role / Administrator</field>
    <field name="implied_ids" eval="[(4, ref('group_user'))]"/>
  </record>
</odoo>`)); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for key, item := range loader.ExternalIDs() {
		ids[key] = item.ResID
	}
	return Server{Env: env, Views: view.NewRegistry()}, ids
}

func testDelegationHTTPServer(t *testing.T) Server {
	t.Helper()
	models := map[string]model.Model{}
	order := []string{}
	add := func(m model.Model) {
		if existing, ok := models[m.Name]; ok {
			models[m.Name] = m.Compose(existing)
			return
		}
		models[m.Name] = m
		order = append(order, m.Name)
	}
	for _, m := range internalbase.Models() {
		add(m)
	}
	for _, m := range internalworkflow.Models() {
		add(m)
	}
	for _, m := range hraddon.Models() {
		add(m)
	}
	for _, m := range oidelegation.Models() {
		add(m)
	}
	for _, m := range oidelegation.ExtensionModels() {
		add(m)
	}
	reg := record.NewRegistry()
	for _, name := range order {
		if err := reg.Register(models[name]); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	return Server{Env: env, Views: view.NewRegistry()}
}

func TestCallKWDelegationLifecycleMethods(t *testing.T) {
	server := testDelegationHTTPServer(t)
	handler := server.Handler()
	env := server.Env
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Delegable", "allow_delegation": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{"login": "owner", "name": "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	delegatorID, err := env.Model("hr.employee").Create(map[string]any{"name": "Owner", "user_id": userID})
	if err != nil {
		t.Fatal(err)
	}
	delegateID, err := env.Model("hr.employee").Create(map[string]any{"name": "Delegate"})
	if err != nil {
		t.Fatal(err)
	}

	confirmID, err := env.Model("delegation").Create(map[string]any{
		"date_from":   "2099-01-01",
		"date_to":     "2099-01-31",
		"employee_id": delegatorID,
		"state":       "draft",
		"lines":       []any{[]any{int64(0), false, map[string]any{"group_id": groupID, "employee_id": delegateID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	postCallKW(t, handler, fmt.Sprintf(`{"model":"delegation","method":"action_confirm","args":[[%d]]}`, confirmID))
	confirmRows, err := env.Model("delegation").Browse(confirmID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if confirmRows[0]["state"] != "confirmed" {
		t.Fatalf("confirm state = %+v", confirmRows)
	}

	revokeID, err := env.Model("delegation").Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	postCallKW(t, handler, fmt.Sprintf(`{"model":"delegation","method":"action_revoked","args":[[%d]]}`, revokeID))
	revokeRows, err := env.Model("delegation").Browse(revokeID).Read("state", "isactive")
	if err != nil {
		t.Fatal(err)
	}
	if revokeRows[0]["state"] != "revoked" || revokeRows[0]["isactive"] != false {
		t.Fatalf("revoke state = %+v", revokeRows)
	}

	expireID, err := env.Model("delegation").Create(map[string]any{"date_from": "2020-01-01", "date_to": "2099-12-31", "employee_id": delegatorID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	postCallKW(t, handler, fmt.Sprintf(`{"model":"delegation","method":"expire_delegation","args":[[%d]]}`, expireID))
	expireRows, err := env.Model("delegation").Browse(expireID).Read("state")
	if err != nil {
		t.Fatal(err)
	}
	if expireRows[0]["state"] != "expired" {
		t.Fatalf("expire state = %+v", expireRows)
	}

	pastID, err := env.Model("delegation").Create(map[string]any{"date_from": "2000-01-01", "date_to": "2000-12-31", "employee_id": delegatorID, "state": "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(fmt.Sprintf(`{"model":"delegation","method":"action_revoked","args":[[%d]]}`, pastID))))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "old date") {
		t.Fatalf("past revoke response %d %s", rec.Code, rec.Body.String())
	}

	postCallKW(t, handler, `{"model":"delegation","method":"_clear_access_cache","args":[]}`)
	cacheEvents, err := env.Model("delegation.cache.event").Search(domain.Cond("reason", "=", "clear_access_cache"))
	if err != nil {
		t.Fatal(err)
	}
	if cacheEvents.Len() != 1 {
		t.Fatalf("cache events = %d, want 1", cacheEvents.Len())
	}
	assertHTTPDelegationApprovalLogStates(t, env, confirmID, [][2]string{{"draft", "confirmed"}})
	assertHTTPDelegationApprovalLogStates(t, env, revokeID, [][2]string{{"confirmed", "revoked"}})
	assertHTTPDelegationApprovalLogStates(t, env, expireID, [][2]string{{"confirmed", "expired"}})
}

func assertHTTPDelegationApprovalLogStates(t *testing.T, env *record.Env, delegationID int64, want [][2]string) {
	t.Helper()
	logs, err := env.Model(internalworkflow.ModelLog).Search(domain.And(
		domain.Cond("model", "=", "delegation"),
		domain.Cond("record_id", "=", delegationID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := logs.Read("old_state", "new_state", "user_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) {
		t.Fatalf("delegation %d approval logs = %+v, want %d", delegationID, rows, len(want))
	}
	for index, expected := range want {
		if rows[index]["old_state"] != expected[0] || rows[index]["new_state"] != expected[1] || rows[index]["user_id"] != int64(1) {
			t.Fatalf("delegation %d approval log[%d] = %+v, want %v", delegationID, index, rows[index], expected)
		}
	}
}

func testContainsRecordID(value any, target int64) bool {
	switch typed := value.(type) {
	case []int64:
		for _, id := range typed {
			if id == target {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if id, ok := item.(float64); ok && int64(id) == target {
				return true
			}
			if id, ok := item.(int64); ok && id == target {
				return true
			}
		}
	}
	return false
}

func testSequenceServer(t *testing.T, companyID int64) Server {
	t.Helper()
	reg := record.NewRegistry()
	for _, model := range internalbase.Models() {
		if err := reg.Register(model); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: companyID, CompanyIDs: []int64{companyID}})
	return Server{Env: env}
}

func testMailThreadServer(t *testing.T) Server {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Register(portalThreadHTTPTestModel()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(gatewayThreadHTTPTestModel()); err != nil {
		t.Fatal(err)
	}
	for _, m := range suggestedRecipientHTTPTestModels() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range projectPortalHTTPTestModels() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	return Server{Env: env}
}

type httpRecordingMailSender struct {
	sent []internalmail.Message
}

func (s *httpRecordingMailSender) Send(message internalmail.Message) error {
	s.sent = append(s.sent, message)
	return nil
}

func portalThreadHTTPTestModel() model.Model {
	m := model.New("portal.thread", "portal_thread")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("parent_id", field.Many2One).WithRelation("portal.thread"))
	m.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	m.AddField(field.New("access_token", field.Char))
	return m
}

func gatewayThreadHTTPTestModel() model.Model {
	m := model.New("gateway.thread", "gateway_thread")
	m.AddField(field.New("name", field.Char))
	m.AddField(field.New("active", field.Bool))
	m.AddField(field.New("email", field.Char))
	m.AddField(field.New("email_normalized", field.Char))
	m.AddField(field.New("description", field.Text))
	m.AddField(field.New("message_count", field.Int))
	m.AddField(field.New("gateway_user_id", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("campaign_id", field.Many2One).WithRelation("utm.campaign"))
	m.AddField(field.New("source_id", field.Many2One).WithRelation("utm.source"))
	m.AddField(field.New("medium_id", field.Many2One).WithRelation("utm.medium"))
	m.AddField(field.New("create_uid", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("create_date", field.DateTime))
	m.AddField(field.New("write_uid", field.Many2One).WithRelation("res.users"))
	m.AddField(field.New("write_date", field.DateTime))
	return m
}

func suggestedRecipientHTTPTestModels() []model.Model {
	employee := model.New("hr.employee", "hr_employee")
	employee.AddField(field.New("name", field.Char))
	employee.AddField(field.New("active", field.Bool))
	employee.AddField(field.New("email", field.Char))
	employee.AddField(field.New("work_email", field.Char))
	employee.AddField(field.New("work_contact_id", field.Many2One).WithRelation("res.partner"))
	employee.AddField(field.New("user_partner_id", field.Many2One).WithRelation("res.partner"))

	genericCC := model.New("cc.generic.thread", "cc_generic_thread")
	genericCC.AddField(field.New("name", field.Char))
	genericCC.AddField(field.New("active", field.Bool))
	genericCC.AddField(field.New("email", field.Char))
	genericCC.AddField(field.New("email_cc", field.Char))

	mixinCC := model.New("cc.mixin.thread", "cc_mixin_thread")
	mixinCC.AddField(field.New("name", field.Char))
	mixinCC.AddField(field.New("active", field.Bool))
	mixinCC.AddField(field.New("email", field.Char))
	mixinCC.AddField(field.New("email_cc", field.Char))

	return []model.Model{employee, genericCC, mixinCC}
}

func projectPortalHTTPTestModels() []model.Model {
	project := model.New("project.project", "project_project")
	project.AddField(field.New("name", field.Char))
	project.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	project.AddField(field.New("access_token", field.Char))
	task := model.New("project.task", "project_task")
	task.AddField(field.New("name", field.Char))
	task.AddField(field.New("project_id", field.Many2One).WithRelation("project.project"))
	task.AddField(field.New("parent_id", field.Many2One).WithRelation("project.task"))
	task.AddField(field.New("partner_id", field.Many2One).WithRelation("res.partner"))
	task.AddField(field.New("access_token", field.Char))
	return []model.Model{project, task}
}

func testWorkflowDispatchServer(t *testing.T) Server {
	t.Helper()
	return testWorkflowDispatchServerWithDefaultReadonly(t, "")
}

func testWorkflowDispatchServerWithDefaultReadonly(t *testing.T, defaultReadonly string) Server {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	workflowModels := map[string]model.Model{}
	var workflowOrder []string
	addWorkflowModel := func(m model.Model) {
		if existing, ok := workflowModels[m.Name]; ok {
			workflowModels[m.Name] = m.Compose(existing)
			return
		}
		workflowModels[m.Name] = m
		workflowOrder = append(workflowOrder, m.Name)
	}
	for _, m := range internalworkflow.Models() {
		addWorkflowModel(m)
	}
	for _, m := range internalworkflow.AdvancedExtensionModels() {
		if _, exists := reg.Model(m.Name); exists {
			continue
		}
		addWorkflowModel(m)
	}
	for _, name := range workflowOrder {
		if err := reg.Register(workflowModels[name]); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range internalworkflow.AdvancedModels() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	po := model.New("purchase.order", "purchase_order")
	po.DefaultFieldReadonly = defaultReadonly
	po.AddField(field.New("name", field.Char))
	po.AddField(field.New("state", field.Selection))
	po.AddField(field.New("amount_total", field.Float))
	lastStateUpdate := field.New("last_state_update", field.DateTime)
	if defaultReadonly != "" {
		lastStateUpdate.Readonly = true
	}
	po.AddField(lastStateUpdate)
	po.AddField(field.New("approval_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("approval_done_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("approval_forward_user_ids", field.Many2Many).WithRelation("res.users"))
	po.AddField(field.New("workflow_id", field.Many2One).WithRelation(internalworkflow.ModelWorkflow))
	po.AddField(field.New("workflow_transition_ids", field.Many2Many).WithRelation(internalworkflow.ModelTransition))
	po.AddField(field.New("workflow_node_id", field.Many2One).WithRelation(internalworkflow.ModelNode))
	po.AddField(field.New("workflow_view_id", field.Many2One).WithRelation("ir.ui.view"))
	po.AddField(field.New("_old_workflow_node_id", field.Many2One).WithRelation(internalworkflow.ModelNode))
	po.AddField(field.New("_workflow_transition_id", field.Many2One).WithRelation(internalworkflow.ModelTransition))
	if err := reg.Register(po); err != nil {
		t.Fatal(err)
	}
	return Server{
		Env:      record.NewEnv(reg, record.Context{UserID: 5, CompanyID: 1, CompanyIDs: []int64{1}, Values: map[string]any{"group_ids": []int64{10}}}),
		Workflow: &internalworkflow.Dispatcher{},
	}
}

func testAccountingDispatchServer(t *testing.T) Server {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range internalbase.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range coreaccounting.Models() {
		if _, exists := reg.Model(m.Name); exists {
			continue
		}
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return Server{
		Env: record.NewEnv(reg, record.Context{UserID: 5, CompanyID: 1, CompanyIDs: []int64{1}}),
	}
}

type propertyExportTestIDs struct {
	record1 int64
	record2 int64
	record3 int64
	record4 int64
}

func testPropertyExportServer(t *testing.T) (Server, propertyExportTestIDs) {
	t.Helper()
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	definition := model.New("x.property.definition", "x_property_definition")
	definition.AddField(field.New("name", field.Char))
	definitionField := field.New("properties_definition", field.PropertiesDefinition)
	definitionField.Label = "Properties"
	definition.AddField(definitionField)
	if err := reg.Register(definition); err != nil {
		t.Fatal(err)
	}
	recordModel := model.New("x.property.record", "x_property_record")
	recordModel.AddField(field.New("name", field.Char))
	recordDefinition := field.New("record_definition_id", field.Many2One).WithRelation("x.property.definition")
	recordDefinition.Label = "Record Definition Id"
	recordModel.AddField(recordDefinition)
	propertiesField := field.New("properties", field.Properties).WithPropertyDefinition("record_definition_id", "properties_definition")
	propertiesField.Label = "Properties"
	recordModel.AddField(propertiesField)
	if err := reg.Register(recordModel); err != nil {
		t.Fatal(err)
	}
	exportsLine := model.New("ir.exports.line", "ir_exports_line")
	exportsLine.AddField(field.New("name", field.Char))
	exportsLine.AddField(field.New("export_id", field.Many2One).WithRelation("ir.exports"))
	if err := reg.Register(exportsLine); err != nil {
		t.Fatal(err)
	}
	exports := model.New("ir.exports", "ir_exports")
	exports.AddField(field.New("name", field.Char))
	if err := reg.Register(exports); err != nil {
		t.Fatal(err)
	}

	env := record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
	partner1, err := env.Model("res.partner").Create(map[string]any{"name": "Name Partner 1"})
	if err != nil {
		t.Fatal(err)
	}
	partner2, err := env.Model("res.partner").Create(map[string]any{"name": "Name Partner 2"})
	if err != nil {
		t.Fatal(err)
	}
	definitionA, err := env.Model("x.property.definition").Create(map[string]any{
		"name": "Definition A",
		"properties_definition": []map[string]any{
			{"name": "char_prop", "type": "char", "string": "TextType", "default": "Def"},
			{"name": "date_prop", "type": "date", "string": "Date"},
			{"name": "separator_prop", "type": "separator", "string": "Separator"},
			{"name": "selection_prop", "type": "selection", "string": "One Selection", "selection": []any{[]any{"selection_1", "aaaaaaa"}, []any{"selection_2", "bbbbbbb"}}},
			{"name": "m2o_prop", "type": "many2one", "string": "many2one", "comodel": "res.partner"},
			{"name": "bad_m2o_prop", "type": "many2one", "string": "Bad M2O", "comodel": "x.missing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitionB, err := env.Model("x.property.definition").Create(map[string]any{
		"name": "Definition B",
		"properties_definition": []map[string]any{
			{"name": "bool_prop", "type": "boolean", "string": "CheckBox"},
			{"name": "tags_prop", "type": "tags", "string": "Tags", "tags": []any{[]any{"aa", "AA", 5}, []any{"bb", "BB", 6}}},
			{"name": "m2m_prop", "type": "many2many", "string": "M2M", "comodel": "res.partner"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record1, err := env.Model("x.property.record").Create(map[string]any{
		"name":                 "Record 1",
		"record_definition_id": definitionA,
		"properties": map[string]any{
			"char_prop":      "Not the default",
			"date_prop":      "2026-01-10",
			"selection_prop": "selection_2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record2, err := env.Model("x.property.record").Create(map[string]any{
		"name":                 "Record 2",
		"record_definition_id": definitionA,
		"properties": map[string]any{
			"date_prop": "2026-01-20",
			"m2o_prop":  partner1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record3, err := env.Model("x.property.record").Create(map[string]any{
		"name":                 "Record 3",
		"record_definition_id": definitionB,
		"properties": map[string]any{
			"tags_prop": []any{"aa", "bb"},
			"bool_prop": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record4, err := env.Model("x.property.record").Create(map[string]any{
		"name":                 "Record 4",
		"record_definition_id": definitionB,
		"properties":           map[string]any{"m2m_prop": []int64{partner1, partner2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Server{Env: env}, propertyExportTestIDs{record1: record1, record2: record2, record3: record3, record4: record4}
}

func createHTTPAccountingMove(t *testing.T, env *record.Env, name string, moveType string, state string) (int64, int64) {
	t.Helper()
	journalType := "sale"
	if strings.HasPrefix(moveType, "in_") {
		journalType = "purchase"
	}
	receivableID, err := getOrCreateHTTPAccountingAccount(t, env, "1100", "Receivable", coreaccounting.AccountReceivable, true)
	if err != nil {
		t.Fatal(err)
	}
	incomeID, err := getOrCreateHTTPAccountingAccount(t, env, "4000", "Income", coreaccounting.AccountIncome, false)
	if err != nil {
		t.Fatal(err)
	}
	journalID, err := env.Model("account.journal").Create(map[string]any{
		"name":        "Sales Journal",
		"code":        "SAJ",
		"type":        journalType,
		"company_id":  int64(1),
		"currency_id": int64(1),
		"active":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	moveEnv := env
	if state == string(coreaccounting.MovePosted) {
		moveEnv = env.WithAccountMovePost()
	}
	moveID, err := moveEnv.Model("account.move").Create(map[string]any{
		"name":                   name,
		"date":                   dateValue(2026, 1, 1),
		"invoice_date":           dateValue(2026, 1, 1),
		"state":                  state,
		"move_type":              moveType,
		"journal_id":             journalID,
		"company_id":             int64(1),
		"currency_id":            int64(1),
		"partner_id":             int64(7),
		"amount_total":           int64(10000),
		"amount_residual":        int64(10000),
		"amount_residual_signed": int64(10000),
		"payment_state":          "not_paid",
		"status_in_payment":      state,
		"posted_before":          state == "posted",
		"auto_post":              "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	line1, err := moveEnv.Model("account.move.line").Create(map[string]any{
		"move_id":                  moveID,
		"account_id":               receivableID,
		"account_type":             string(coreaccounting.AccountReceivable),
		"account_internal_group":   string(coreaccounting.AccountReceivable),
		"partner_id":               int64(7),
		"company_id":               int64(1),
		"currency_id":              int64(1),
		"name":                     "Receivable",
		"debit":                    int64(10000),
		"credit":                   int64(0),
		"amount_currency":          int64(10000),
		"amount_residual":          int64(10000),
		"amount_residual_currency": int64(10000),
	})
	if err != nil {
		t.Fatal(err)
	}
	line2, err := moveEnv.Model("account.move.line").Create(map[string]any{
		"move_id":                  moveID,
		"account_id":               incomeID,
		"account_type":             string(coreaccounting.AccountIncome),
		"account_internal_group":   string(coreaccounting.AccountIncome),
		"company_id":               int64(1),
		"currency_id":              int64(1),
		"name":                     "Income",
		"debit":                    int64(0),
		"credit":                   int64(10000),
		"amount_currency":          int64(-10000),
		"amount_residual":          int64(-10000),
		"amount_residual_currency": int64(-10000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := moveEnv.Model("account.move").Browse(moveID).Write(map[string]any{"line_ids": []int64{line1, line2}}); err != nil {
		t.Fatal(err)
	}
	return moveID, journalID
}

func getOrCreateHTTPAccountingAccount(t *testing.T, env *record.Env, code string, name string, kind coreaccounting.AccountKind, reconcile bool) (int64, error) {
	t.Helper()
	found, err := env.Model("account.account").Search(domain.And(
		domain.Cond("code", domain.Equal, code),
		domain.Cond("company_id", domain.Equal, int64(1)),
	))
	if err != nil {
		return 0, err
	}
	if ids := found.IDs(); len(ids) > 0 {
		return ids[0], nil
	}
	return env.Model("account.account").Create(map[string]any{
		"code":         code,
		"name":         name,
		"account_type": string(kind),
		"company_id":   int64(1),
		"reconcile":    reconcile,
	})
}

func dateValue(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func createHTTPWorkflowSettings(t *testing.T, env *record.Env) int64 {
	t.Helper()
	id, err := env.Model(internalworkflow.ModelSettings).Create(map[string]any{
		"name":            "HTTP Approval",
		"model":           "purchase.order",
		"active":          true,
		"state_field":     "state",
		"draft_state":     "draft",
		"approved_state":  "approved",
		"rejected_state":  "rejected",
		"cancelled_state": "cancelled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (s Server) EnvRegistryForTest(t *testing.T) *record.Registry {
	t.Helper()
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("active", field.Bool))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	return reg
}

func testImpersonationService() *impersonation.Service {
	svc := impersonation.NewService()
	svc.SetUser(impersonation.User{ID: 1, Login: "admin", Name: "Admin", Active: true, GroupIDs: []int64{1}})
	svc.SetUser(impersonation.User{ID: 20, Login: "portal", Name: "Portal", Active: true, Portal: true, GroupIDs: []int64{30}})
	svc.SetUser(impersonation.User{ID: 50, Login: "user", Name: "User", Active: true})
	return svc
}

func testDebugImpersonationService(enabled bool) *impersonation.Service {
	config := impersonation.DefaultConfig()
	config.DebugRouteEnabled = enabled
	svc := impersonation.NewService(impersonation.WithConfig(config))
	svc.SetUser(impersonation.User{ID: 1, Login: "root", Name: "System", Active: true, Superuser: true, GroupIDs: []int64{1}})
	svc.SetUser(impersonation.User{ID: 12, Login: "debugger", Name: "Debugger", Active: true, GroupIDs: []int64{2, 4, 5}})
	return svc
}

func testLoginAsAuditServer(t *testing.T, userID int64, svc *impersonation.Service) Server {
	t.Helper()
	reg := record.NewRegistry()
	if err := oi_login_as.RegisterRecordModels(reg); err != nil {
		t.Fatal(err)
	}
	return Server{
		Env:           record.NewEnv(reg, record.Context{UserID: userID, CompanyID: 1, CompanyIDs: []int64{1}}),
		Impersonation: svc,
	}
}

func readLoginAsAuditRows(t *testing.T, server Server) []map[string]any {
	t.Helper()
	found, err := server.Env.Model(oi_login_as.ModelLoginAsAudit).Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("action", "actor_id", "effective_user_id", "target_user_id", "session_id", "ip_address", "user_agent", "details", "created_at")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertComposedPartnerArch(t *testing.T, arch string) {
	t.Helper()
	for _, want := range []string{`name="name"`, `name="email"`} {
		if !strings.Contains(arch, want) {
			t.Fatalf("arch missing %q: %s", want, arch)
		}
	}
	if strings.Contains(arch, "<xpath") {
		t.Fatalf("arch contains raw xpath spec: %s", arch)
	}
	nameIndex := strings.Index(arch, `name="name"`)
	emailIndex := strings.Index(arch, `name="email"`)
	if nameIndex < 0 || emailIndex < 0 || emailIndex <= nameIndex {
		t.Fatalf("arch order mismatch: %s", arch)
	}
}

func viewLoadArchByID(t *testing.T, data []byte, id int64) string {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, item := range payload {
		if firstNonNil(item["ID"], item["id"]) == nil {
			continue
		}
		if int64Value(firstNonNil(item["ID"], item["id"])) == id {
			arch, _ := firstNonNil(item["Arch"], item["arch"]).(string)
			if arch == "" {
				t.Fatalf("view %d arch missing: %+v", id, item)
			}
			return arch
		}
	}
	t.Fatalf("view %d missing from /web/view/load: %+v", id, payload)
	return ""
}

func postCallKW(t *testing.T, handler http.Handler, payload string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_kw", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("call_kw status %d %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func postCallButton(t *testing.T, handler http.Handler, payload string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web/dataset/call_button", bytes.NewBufferString(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("call_button status %d %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func multipartUploadRequest(t *testing.T, target string, fields map[string]string, fileField string, filename string, content string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type capturePolicy struct {
	checks       []record.Context
	recordChecks []record.Context
}

func (p *capturePolicy) Check(ctx record.Context, _ string, _ record.Operation, _ map[string]any) error {
	p.checks = append(p.checks, ctx)
	return nil
}

func (p *capturePolicy) CheckRecord(ctx record.Context, _ string, _ record.Operation, _ map[string]any) (bool, error) {
	p.recordChecks = append(p.recordChecks, ctx)
	return true, nil
}

func (p *capturePolicy) FilterFields(_ record.Context, _ string, fields []string) []string {
	return fields
}

type reportBinSizePolicy struct {
	reportReads []record.Context
}

func (p *reportBinSizePolicy) Check(ctx record.Context, modelName string, op record.Operation, _ map[string]any) error {
	if modelName == "ir.actions.report" && op == record.OpRead {
		p.reportReads = append(p.reportReads, ctx)
		if ctx.Values["bin_size"] != true {
			return fmt.Errorf("report read without bin_size")
		}
	}
	return nil
}

func (p *reportBinSizePolicy) CheckRecord(ctx record.Context, modelName string, op record.Operation, _ map[string]any) (bool, error) {
	if modelName == "ir.actions.report" && op == record.OpRead && ctx.Values["bin_size"] != true {
		return false, fmt.Errorf("report record read without bin_size")
	}
	return true, nil
}

func (p *reportBinSizePolicy) FilterFields(_ record.Context, _ string, fields []string) []string {
	return fields
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func decodeJSONList(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func testRecipientEmailSeen(recipients []map[string]any, email string) bool {
	for _, recipient := range recipients {
		if stringValue(recipient["email"]) == email {
			return true
		}
	}
	return false
}

func nextHTTPBusEvent(t *testing.T, sub notifications.Subscription) notifications.Event {
	t.Helper()
	select {
	case event := <-sub.Events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bus event")
		return notifications.Event{}
	}
}

type httpAIStore struct {
	channels map[int64]aicontrollers.Channel
	messages map[int64]aicontrollers.Message
	history  map[int64][]aicontrollers.Message
	posted   map[int64][]string
	deleted  map[int64]bool
}

func newHTTPAIStore() *httpAIStore {
	return &httpAIStore{
		channels: map[int64]aicontrollers.Channel{},
		messages: map[int64]aicontrollers.Message{},
		history:  map[int64][]aicontrollers.Message{},
		posted:   map[int64][]string{},
		deleted:  map[int64]bool{},
	}
}

func httpAIChannel(id int64, member bool) aicontrollers.Channel {
	return aicontrollers.Channel{
		ID:       id,
		IsMember: member,
		Agent: agents.Agent{
			ID:     2,
			Name:   "Assistant",
			Model:  "mock-chat",
			Active: true,
		},
	}
}

func (s *httpAIStore) Channel(_ context.Context, id int64) (aicontrollers.Channel, bool) {
	channel, ok := s.channels[id]
	return channel, ok
}

func (s *httpAIStore) Message(_ context.Context, id int64) (aicontrollers.Message, bool) {
	message, ok := s.messages[id]
	return message, ok
}

func (s *httpAIStore) History(_ context.Context, channelID int64, limit int) []aicontrollers.Message {
	history := append([]aicontrollers.Message(nil), s.history[channelID]...)
	if limit > 0 && len(history) > limit {
		return history[len(history)-limit:]
	}
	return history
}

func (s *httpAIStore) DeleteChannel(_ context.Context, id int64) error {
	s.deleted[id] = true
	delete(s.channels, id)
	return nil
}

func (s *httpAIStore) PostAssistantMessage(_ context.Context, channelID int64, body string) error {
	s.posted[channelID] = append(s.posted[channelID], body)
	return nil
}

type httpAIResponder struct {
	response agents.Response
	err      error
	requests []agents.Request
}

func (r *httpAIResponder) Generate(_ context.Context, _ agents.Agent, request agents.Request) (agents.Response, error) {
	r.requests = append(r.requests, request)
	if r.err != nil {
		return agents.Response{}, r.err
	}
	return r.response, nil
}

type httpTranscriptionProvider struct {
	params   aicontrollers.RealtimeParameters
	response aicontrollers.TranscriptionSession
}

func (p *httpTranscriptionProvider) CreateTranscriptionSession(_ context.Context, params aicontrollers.RealtimeParameters) (aicontrollers.TranscriptionSession, error) {
	p.params = params
	if p.response != nil {
		return p.response, nil
	}
	return httpTranscriptionSession(), nil
}

func httpTranscriptionSession() aicontrollers.TranscriptionSession {
	return aicontrollers.TranscriptionSession{
		"value":      "secret",
		"expires_at": int64(7200),
		"session": map[string]any{
			"type": "transcription",
		},
	}
}

func testAttachmentOwnershipToken(secret string, attachmentID int64, expires time.Time) string {
	timestamp := fmt.Sprintf("0x%x", expires.Unix())
	message := fmt.Sprintf("('%s', %d, '%s', '%s')", "ir.attachment", attachmentID, "id", timestamp)
	payload := fmt.Sprintf("('%s', %s)", "attachment_ownership", message)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil)) + "o" + timestamp
}

func testPortalThreadHash(secret string, dbName string, accessToken string, partnerID int64) string {
	payload := fmt.Sprintf("('%s', '%s', %d)", dbName, accessToken, partnerID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func zipEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	return names
}
