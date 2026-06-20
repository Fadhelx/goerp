package impersonation

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestLoginAsSessionRoutesBannerAndAudit(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := testService(now, DefaultConfig())

	routes := svc.Routes()
	if routes[0].Path != "/web/login_as/<user_id>" || routes[1].Path != "/web/login_back" {
		t.Fatalf("routes = %+v", routes)
	}
	if routes[2].Path != "/web/become/debug" || routes[2].Enabled {
		t.Fatalf("debug route = %+v", routes[2])
	}

	action, err := svc.WizardAction(10, 20, SwitchOptions{GroupID: 30, ReturnTo: "/web#menu_id=1"})
	if err != nil {
		t.Fatal(err)
	}
	if action.Type != "ir.actions.act_url" || action.Route != "/web/login_as/20" || action.Context["group_id"] != int64(30) {
		t.Fatalf("action = %+v", action)
	}

	session, err := svc.Start("sid", 10, 20, SwitchOptions{GroupID: 30, ReturnTo: "/web#menu_id=1", Reason: "support"})
	if err != nil {
		t.Fatal(err)
	}
	if !session.Impersonating || session.UserID != 20 || session.OriginalUserID != 10 || session.ReturnTo == "" {
		t.Fatalf("session = %+v", session)
	}

	info, err := svc.SessionInfo("sid")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Impersonating || info.Banner != "Impersonating Portal User" || info.Context["login_as_original_uid"] != int64(10) {
		t.Fatalf("info = %+v", info)
	}
	if err := svc.RecordAction("sid", "write", "res.partner", 55, map[string]string{"password": "raw", "note": "ok"}); err != nil {
		t.Fatal(err)
	}
	audit := svc.AuditLog()
	if len(audit) != 2 {
		t.Fatalf("audit = %+v", audit)
	}
	if audit[1].ActorID != 10 || audit[1].EffectiveUserID != 20 || audit[1].Details["password"] != "[redacted]" {
		t.Fatalf("action audit = %+v", audit[1])
	}
	audit[1].Details["note"] = "mutated"
	if svc.AuditLog()[1].Details["note"] != "ok" {
		t.Fatal("audit log was mutable through returned slice")
	}
	if err := svc.ReplaceAuditEvent(AuditEvent{}); !errors.Is(err, ErrImmutableAuditEvent) {
		t.Fatalf("expected immutable audit error, got %v", err)
	}

	back, err := svc.LoginBack("sid")
	if err != nil {
		t.Fatal(err)
	}
	if back.Impersonating || back.UserID != 10 || back.OriginalUserID != 0 {
		t.Fatalf("back = %+v", back)
	}
	if got := svc.AuditLog(); got[len(got)-1].Action != "login_as.back" {
		t.Fatalf("audit = %+v", got)
	}
}

func TestLoginAsRestrictionsAndExplicitPermissions(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := testService(now, DefaultConfig())

	if err := svc.CanImpersonate(50, 20, SwitchOptions{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if err := svc.CanImpersonate(10, 10, SwitchOptions{}); !errors.Is(err, ErrSelfImpersonation) {
		t.Fatalf("expected self error, got %v", err)
	}
	if err := svc.CanImpersonate(10, 40, SwitchOptions{}); !errors.Is(err, ErrTargetInactive) {
		t.Fatalf("expected inactive denied, got %v", err)
	}
	if err := svc.CanImpersonate(10, 1, SwitchOptions{}); !errors.Is(err, ErrTargetSuperuser) {
		t.Fatalf("expected superuser denied, got %v", err)
	}
	if err := svc.CanImpersonate(10, 20, SwitchOptions{GroupID: 999}); !errors.Is(err, ErrGroupMismatch) {
		t.Fatalf("expected group mismatch, got %v", err)
	}

	if err := svc.CanImpersonate(11, 40, SwitchOptions{AllowInactive: true}); err != nil {
		t.Fatalf("inactive explicit permission failed: %v", err)
	}
	if err := svc.CanImpersonate(12, 1, SwitchOptions{AllowSuperuser: true}); err != nil {
		t.Fatalf("superuser explicit permission failed: %v", err)
	}
	targets, err := svc.AllowedTargetsForGroup(10, 30)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	if !reflect.DeepEqual(ids, []int64{20, 50}) {
		t.Fatalf("targets = %+v", ids)
	}
}

func TestDebugRouteRequiresSettingAndPermission(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	config := DefaultConfig()
	svc := testService(now, config)
	if _, err := svc.SwitchToSystem("sid", 12, SwitchOptions{Reason: "debug"}); !errors.Is(err, ErrDebugRouteDisabled) {
		t.Fatalf("expected debug disabled, got %v", err)
	}

	config.DebugRouteEnabled = true
	svc = testService(now, config)
	routes := svc.Routes()
	if !routes[2].Enabled || routes[2].RequiresSetting == "" {
		t.Fatalf("debug route = %+v", routes[2])
	}
	if _, err := svc.SwitchToSystem("sid", 10, SwitchOptions{}); !errors.Is(err, ErrTargetSuperuser) {
		t.Fatalf("expected explicit superuser permission, got %v", err)
	}
	session, err := svc.SwitchToSystem("sid", 12, SwitchOptions{AllowSuperuser: true})
	if err != nil {
		t.Fatal(err)
	}
	if !session.Impersonating || session.UserID != 1 || session.OriginalUserID != 12 {
		t.Fatalf("system session = %+v", session)
	}
}

func testService(now time.Time, config Config) *Service {
	svc := NewService(WithNow(func() time.Time { return now }), WithConfig(config))
	svc.SetUser(User{ID: 1, Login: "root", Name: "System", Active: true, Superuser: true, GroupIDs: []int64{1, 4, 5}})
	svc.SetUser(User{ID: 10, Login: "admin", Name: "Admin", Active: true, GroupIDs: []int64{1}})
	svc.SetUser(User{ID: 11, Login: "inactive-admin", Name: "Inactive Admin", Active: true, GroupIDs: []int64{1, 3}})
	svc.SetUser(User{ID: 12, Login: "system-admin", Name: "System Admin", Active: true, GroupIDs: []int64{1, 4, 5}})
	svc.SetUser(User{ID: 20, Login: "portal", Name: "Portal User", Active: true, Portal: true, GroupIDs: []int64{30}})
	svc.SetUser(User{ID: 40, Login: "inactive", Name: "Inactive", Active: false, GroupIDs: []int64{30}})
	svc.SetUser(User{ID: 50, Login: "employee", Name: "Employee", Active: true, GroupIDs: []int64{30}})
	return svc
}
