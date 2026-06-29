package security

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gorp/internal/base"
	"gorp/internal/data"
	"gorp/internal/delegation"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
)

func TestUsersGroupsCompanies(t *testing.T) {
	engine := testEngine()
	groups := engine.EffectiveGroupIDs(10)
	if !groups[1] || !groups[2] {
		t.Fatalf("effective groups = %+v", groups)
	}
	if engine.Users[10].CompanyID != 1 || len(engine.Users[10].CompanyIDs) != 2 {
		t.Fatalf("unexpected user companies: %+v", engine.Users[10])
	}
}

func TestTokens(t *testing.T) {
	engine := testEngine()
	expiry := time.Now().Add(time.Hour)
	engine.IssueSession(10, "raw-session", expiry)
	if _, ok := engine.Sessions["raw-session"]; ok {
		t.Fatal("raw token stored")
	}
	userID, ok := engine.AuthenticateSession("raw-session")
	if !ok || userID != 10 {
		t.Fatalf("session auth failed: %d %v", userID, ok)
	}
	engine.RevokeSession("raw-session")
	if _, ok := engine.AuthenticateSession("raw-session"); ok {
		t.Fatal("revoked session accepted")
	}

	engine.IssueAPIToken(10, "ci", "raw-api", []string{"read"}, expiry)
	for _, token := range engine.APITokens {
		if token.Hash == "raw-api" {
			t.Fatal("raw api token stored")
		}
		if !TokensEqual("raw-api", token.Hash) {
			t.Fatal("token hash mismatch")
		}
	}
}

func TestSessionCompanyContext(t *testing.T) {
	engine := NewEngine()
	engine.SetNow(func() time.Time { return time.Unix(100, 0) })
	engine.IssueSession(10, "raw-session", time.Unix(200, 0))
	if _, _, ok := engine.SessionCompanies("raw-session"); ok {
		t.Fatal("empty session company context returned")
	}
	if !engine.SetSessionCompanies("raw-session", 3, []int64{2, 3, 3, 0}) {
		t.Fatal("session company context not stored")
	}
	companyID, companyIDs, ok := engine.SessionCompanies("raw-session")
	if !ok || companyID != 3 || len(companyIDs) != 2 || companyIDs[0] != 2 || companyIDs[1] != 3 {
		t.Fatalf("session companies = %d %+v %v", companyID, companyIDs, ok)
	}
	engine.RevokeSession("raw-session")
	if engine.SetSessionCompanies("raw-session", 2, []int64{2}) {
		t.Fatal("revoked session accepted company context")
	}
	if _, _, ok := engine.SessionCompanies("raw-session"); ok {
		t.Fatal("revoked session returned company context")
	}
}

func TestAuthenticateSessionConcurrent(t *testing.T) {
	engine := NewEngine()
	engine.SetNow(func() time.Time { return time.Unix(100, 0) })
	engine.IssueSession(10, "raw-session", time.Unix(200, 0))

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 256; j++ {
				userID, ok := engine.AuthenticateSession("raw-session")
				if !ok || userID != 10 {
					t.Errorf("session auth failed: %d %v", userID, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestACL(t *testing.T) {
	engine := testEngine()
	ctx := record.Context{UserID: 10}
	if err := engine.Check(ctx, "res.partner", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(ctx, "res.partner", record.OpUnlink, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected unlink denied, got %v", err)
	}
}

func TestRecordRules(t *testing.T) {
	engine := testEngine()
	engine.Rules = []Rule{
		{Model: "res.partner", Global: true, Active: true, PermRead: true, Domain: domain.Cond("company_id", domain.In, "user.company_ids")},
	}

	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(2)})
	if err != nil || !ok {
		t.Fatalf("expected allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected company 3 denied")
	}
}

func TestAllowedByRecordRulesSupportsAnyAndNotAny(t *testing.T) {
	engine := testEngine()
	engine.Rules = []Rule{
		{Model: "approval.request", Global: true, Active: true, PermRead: true, Domain: domain.Cond("line_ids", domain.AnyOf, domain.Cond("state", domain.Equal, "done"))},
	}
	ok, err := engine.AllowedByRecordRules(10, "approval.request", record.OpRead, map[string]any{
		"line_ids": []map[string]any{{"state": "draft"}, {"state": "done"}},
	})
	if err != nil || !ok {
		t.Fatalf("expected any rule allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "approval.request", record.OpRead, map[string]any{
		"line_ids": []map[string]any{{"state": "draft"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected any rule denied")
	}

	engine.Rules = []Rule{
		{Model: "approval.request", Global: true, Active: true, PermRead: true, Domain: domain.Cond("line_ids", domain.NotAnyOf, domain.Cond("state", domain.Equal, "cancel"))},
	}
	ok, err = engine.AllowedByRecordRules(10, "approval.request", record.OpRead, map[string]any{
		"line_ids": []map[string]any{{"state": "draft"}},
	})
	if err != nil || !ok {
		t.Fatalf("expected not any rule allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "approval.request", record.OpRead, map[string]any{
		"line_ids": []map[string]any{{"state": "cancel"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected not any rule denied")
	}
}

func TestRecordRulesUseCompanyHierarchyForChildAndParentOf(t *testing.T) {
	engine := testEngine()
	engine.Companies[3] = Company{ID: 3, Name: "Sub Branch", ParentID: 2, Active: true}
	engine.Rules = []Rule{
		{Model: "res.partner", Global: true, Active: true, PermRead: true, Domain: domain.Cond("company_id", domain.ChildOf, []int64{1})},
	}

	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(3)})
	if err != nil || !ok {
		t.Fatalf("expected child company allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(99)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected unrelated company denied")
	}

	engine.Rules = []Rule{
		{Model: "res.company", Global: true, Active: true, PermRead: true, Domain: domain.Cond("id", domain.ParentOf, []int64{3})},
	}
	for _, id := range []int64{1, 2, 3} {
		ok, err = engine.AllowedByRecordRules(10, "res.company", record.OpRead, map[string]any{"id": id})
		if err != nil || !ok {
			t.Fatalf("expected company %d parent_of allowed, got %v %v", id, ok, err)
		}
	}
	ok, err = engine.AllowedByRecordRules(10, "res.company", record.OpRead, map[string]any{"id": int64(4)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected non-ancestor company denied")
	}
}

func TestFieldGroups(t *testing.T) {
	engine := testEngine()
	engine.FieldGroups["res.partner"] = map[string][]int64{"secret": {3}}
	ctx := record.Context{UserID: 10}
	fields := engine.FilterFields(ctx, "res.partner", []string{"name", "secret"})
	if len(fields) != 1 || fields[0] != "name" {
		t.Fatalf("fields = %+v", fields)
	}
	err := engine.Check(ctx, "res.partner", record.OpWrite, map[string]any{"secret": "x"})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected field write denied, got %v", err)
	}
}

func TestDelegationProviderExpandsRuntimeSecurityAndRevocationStopsAccess(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[1] = Group{ID: 1, Name: "Employee"}
	engine.Groups[2] = Group{ID: 2, Name: "Approver", ImpliedIDs: []int64{3}}
	engine.Groups[3] = Group{ID: 3, Name: "Sensitive Fields"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, GroupIDs: []int64{1}}
	engine.ACLs = []ACL{{Model: "secret.model", GroupID: 2, PermRead: true, PermWrite: true}}
	engine.Rules = []Rule{{Model: "secret.model", GroupIDs: []int64{2}, Active: true, PermRead: true, Domain: domain.Cond("flag", domain.Equal, true)}}
	engine.FieldGroups["secret.model"] = map[string][]int64{"secret": {3}}

	svc := delegation.NewService(
		delegation.WithNow(func() time.Time { return now }),
		delegation.WithCacheInvalidator(engine),
	)
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	engine.SetDelegationProvider(svc)

	if groups := engine.EffectiveGroupIDs(20); groups[2] || groups[3] {
		t.Fatalf("groups before activation = %+v", groups)
	}
	if err := engine.Check(record.Context{UserID: 20}, "secret.model", record.OpRead, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected inactive delegation denied, got %v", err)
	}
	if err := engine.Check(record.Context{UserID: 20, Sudo: true}, "secret.model", record.OpRead, nil); err != nil {
		t.Fatalf("sudo check denied: %v", err)
	}
	if fields := engine.FilterFields(record.Context{UserID: 20, Sudo: true}, "secret.model", []string{"name", "secret"}); len(fields) != 2 {
		t.Fatalf("sudo fields = %+v", fields)
	}
	ok, err := engine.CheckRecord(record.Context{UserID: 20, Sudo: true}, "secret.model", record.OpRead, map[string]any{"flag": false})
	if err != nil || !ok {
		t.Fatalf("sudo record rule ok=%v err=%v", ok, err)
	}

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
	if groups := engine.EffectiveGroupIDs(20); !groups[2] || !groups[3] {
		t.Fatalf("groups after activation = %+v", groups)
	}
	if err := engine.Check(record.Context{UserID: 20}, "secret.model", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	if fields := engine.FilterFields(record.Context{UserID: 20}, "secret.model", []string{"name", "secret"}); len(fields) != 2 {
		t.Fatalf("fields after activation = %+v", fields)
	}
	ok, err = engine.AllowedByRecordRules(20, "secret.model", record.OpRead, map[string]any{"flag": false})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected delegated group rule to apply")
	}

	if _, err := svc.Revoke(req.ID); err != nil {
		t.Fatal(err)
	}
	if groups := engine.EffectiveGroupIDs(20); groups[2] || groups[3] {
		t.Fatalf("groups after revocation = %+v", groups)
	}
	if err := engine.Check(record.Context{UserID: 20}, "secret.model", record.OpRead, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected revoked delegation denied, got %v", err)
	}
	if fields := engine.FilterFields(record.Context{UserID: 20}, "secret.model", []string{"name", "secret"}); len(fields) != 1 || fields[0] != "name" {
		t.Fatalf("fields after revocation = %+v", fields)
	}
}

func TestDelegatedRecordRuleDomainsUseDelegatorContextAndNarrowing(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[1] = Group{ID: 1, Name: "Employee"}
	engine.Groups[2] = Group{ID: 2, Name: "Approver"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{1}}
	engine.Rules = []Rule{
		{Model: "purchase.order", GroupIDs: []int64{2}, Active: true, PermRead: true, Domain: domain.Cond("owner_id", domain.Equal, "user.id")},
	}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	engine.SetDelegationProvider(svc)
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		DepartmentIDs:   []int64{30},
		Lines:           []delegation.LineInput{{GroupID: 2, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}

	ok, err := engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"owner_id": int64(10), "company_id": int64(1), "department_id": int64(30)})
	if err != nil || !ok {
		t.Fatalf("delegator-owned record denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"owner_id": int64(20), "company_id": int64(1), "department_id": int64(30)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("delegate-owned record allowed through delegated rule")
	}
	ok, err = engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"owner_id": int64(10), "company_id": int64(2), "department_id": int64(30)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("record outside delegator company allowed")
	}
	ok, err = engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"owner_id": int64(10), "company_id": int64(1), "department_id": int64(40)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("record outside delegation department allowed")
	}
}

func TestDelegatedRecordRuleDomainsUseCompanyIntersectionForDomainVariables(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[1] = Group{ID: 1, Name: "Employee"}
	engine.Groups[2] = Group{ID: 2, Name: "Approver"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, CompanyID: 2, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{1}}
	engine.Rules = []Rule{
		{Model: "approval.request", GroupIDs: []int64{2}, Active: true, PermRead: true, Domain: domain.And(
			domain.Cond("owner_id", domain.Equal, "user.id"),
			domain.Cond("approval_company_id", domain.In, "company_ids"),
		)},
	}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	engine.SetDelegationProvider(svc)
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

	ok, err := engine.AllowedByRecordRules(20, "approval.request", record.OpRead, map[string]any{"owner_id": int64(10), "approval_company_id": int64(1)})
	if err != nil || !ok {
		t.Fatalf("record inside company intersection denied: %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(20, "approval.request", record.OpRead, map[string]any{"owner_id": int64(10), "approval_company_id": int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("record outside delegate/delegator company intersection allowed")
	}

	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, CompanyID: 3, CompanyIDs: []int64{3}, GroupIDs: []int64{1}}
	ok, err = engine.AllowedByRecordRules(20, "approval.request", record.OpRead, map[string]any{"owner_id": int64(10), "approval_company_id": int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("delegated rule allowed despite empty company intersection")
	}
}

func TestDelegatedRecordRuleDepartmentsUseSourceFieldOrderAndHierarchy(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.SetDepartmentResolvers(nil, func(ids []int64) []int64 {
		out := append([]int64(nil), ids...)
		if containsID(ids, 30) {
			out = append(out, 31)
		}
		return out
	})
	engine.Groups[2] = Group{ID: 2, Name: "Approver"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, CompanyID: 1, CompanyIDs: []int64{1}, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, CompanyID: 1, CompanyIDs: []int64{1}}
	engine.Rules = []Rule{
		{Model: "purchase.order", GroupIDs: []int64{2}, Active: true, PermRead: true, Domain: domain.Cond("owner_id", domain.Equal, "user.id")},
	}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	engine.SetDelegationProvider(svc)
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		DepartmentIDs:   []int64{30},
		Lines:           []delegation.LineInput{{GroupID: 2, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}

	base := map[string]any{"owner_id": int64(10), "company_id": int64(1)}
	for name, row := range map[string]map[string]any{
		"employee":     withValues(base, map[string]any{"employee_id": map[string]any{"department_id": int64(31)}}),
		"x_employee":   withValues(base, map[string]any{"x_employee_id": map[string]any{"department_id": int64(31)}}),
		"department":   withValues(base, map[string]any{"department_id": int64(31)}),
		"x_department": withValues(base, map[string]any{"x_department_id": int64(31)}),
	} {
		ok, err := engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, row)
		if err != nil || !ok {
			t.Fatalf("%s department denied: %v %v", name, ok, err)
		}
	}
	precedence := withValues(base, map[string]any{
		"employee_id":   map[string]any{"department_id": int64(40)},
		"department_id": int64(31),
	})
	ok, err := engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, precedence)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("department fallback allowed despite employee_id precedence")
	}
	sibling := withValues(base, map[string]any{"department_id": int64(40)})
	ok, err = engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, sibling)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("sibling department allowed")
	}
}

func TestDelegatedRecordRulesMatchImpliedGroups(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[2] = Group{ID: 2, Name: "Delegated Role", ImpliedIDs: []int64{3}}
	engine.Groups[3] = Group{ID: 3, Name: "Implied Approver"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true}
	engine.Rules = []Rule{
		{Model: "purchase.order", GroupIDs: []int64{3}, Active: true, PermRead: true, Domain: domain.Cond("owner_id", domain.Equal, "user.id")},
	}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Delegated Role", AllowDelegation: true})
	engine.SetDelegationProvider(svc)
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
	ok, err := engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"owner_id": int64(10)})
	if err != nil || !ok {
		t.Fatalf("implied delegated group denied: %v %v", ok, err)
	}
}

func TestRestrictedAccessFiltersDelegatedRecordRules(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[1] = Group{ID: 1, Name: "Employee"}
	engine.Groups[2] = Group{ID: 2, Name: "Unrestricted Delegated"}
	engine.Groups[3] = Group{ID: 3, Name: "Restricted Delegated"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, GroupIDs: []int64{2, 3}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true, GroupIDs: []int64{1}}
	engine.Rules = []Rule{
		{Model: "purchase.order", GroupIDs: []int64{2}, Active: true, PermRead: true, Domain: domain.Cond("bucket", domain.Equal, "unrestricted")},
		{Model: "purchase.order", GroupIDs: []int64{3}, Active: true, PermRead: true, Domain: domain.Cond("bucket", domain.Equal, "restricted")},
	}
	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }), delegation.WithRestrictedAccess(true))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Unrestricted", AllowDelegation: true})
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 3, Name: "Restricted", AllowDelegation: true, RestrictedAccess: true})
	engine.SetDelegationProvider(svc)
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines: []delegation.LineInput{
			{GroupID: 2, DelegateUserID: 20},
			{GroupID: 3, DelegateUserID: 20},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if groups := engine.EffectiveGroupIDs(20); groups[2] || !groups[3] {
		t.Fatalf("restricted effective groups = %+v", groups)
	}
	ok, err := engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"bucket": "unrestricted"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unrestricted delegated rule allowed in restricted mode")
	}
	ok, err = engine.AllowedByRecordRules(20, "purchase.order", record.OpRead, map[string]any{"bucket": "restricted"})
	if err != nil || !ok {
		t.Fatalf("restricted delegated rule denied: %v %v", ok, err)
	}
}

func TestDelegationProviderStopsAccessAfterDateExpiry(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.SetNow(func() time.Time { return now })
	engine.Groups[2] = Group{ID: 2, Name: "Approver"}
	engine.Users[10] = User{ID: 10, Login: "delegator", Active: true, GroupIDs: []int64{2}}
	engine.Users[20] = User{ID: 20, Login: "delegate", Active: true}
	engine.ACLs = []ACL{{Model: "secret.model", GroupID: 2, PermRead: true}}

	svc := delegation.NewService(delegation.WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(delegation.GroupConfig{GroupID: 2, Name: "Approver", AllowDelegation: true})
	engine.SetDelegationProvider(svc)
	req, err := svc.CreateRequest(delegation.RequestInput{
		DateFrom:        now,
		DateTo:          now,
		DelegatorUserID: 10,
		Lines:           []delegation.LineInput{{GroupID: 2, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 20}, "secret.model", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}

	engine.SetNow(func() time.Time { return now.AddDate(0, 0, 1) })
	if err := engine.Check(record.Context{UserID: 20}, "secret.model", record.OpRead, nil); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected expired delegation denied, got %v", err)
	}
}

func TestRecordRulesFilterORM(t *testing.T) {
	engine := testEngine()
	engine.Rules = []Rule{
		{Model: "res.partner", Global: true, Active: true, PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true, Domain: domain.Cond("company_id", domain.In, "user.company_ids")},
	}
	reg := record.NewRegistry()
	partner := model.New("res.partner", "res_partner")
	partner.AddField(field.New("name", field.Char))
	partner.AddField(field.New("company_id", field.Many2One).WithRelation("res.company"))
	if err := reg.Register(partner); err != nil {
		t.Fatal(err)
	}
	env := record.NewEnv(reg, record.Context{UserID: 10}).WithPolicy(engine)
	if _, err := env.Model("res.partner").Create(map[string]any{"name": "Allowed", "company_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.partner").Create(map[string]any{"name": "Denied", "company_id": int64(3)}); err == nil {
		t.Fatal("expected create denied by record rule")
	}
	found, err := env.Model("res.partner").Search(domain.Cond("name", domain.Equal, "Allowed"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("found = %d", found.Len())
	}
}

func TestPersistedSecurityLoadsACLsAndRules(t *testing.T) {
	env := testBaseEnv(t)
	if _, err := env.Model("ir.model.access").Create(map[string]any{
		"name":        "partner global",
		"model":       "res.partner",
		"active":      true,
		"perm_read":   true,
		"perm_write":  true,
		"perm_create": true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.rule").Create(map[string]any{
		"name":         "company read",
		"model":        "res.partner",
		"active":       true,
		"domain_force": `[["company_id","in","user.company_ids"]]`,
		"perm_read":    true,
		"perm_write":   false,
		"perm_create":  false,
		"perm_unlink":  false,
	}); err != nil {
		t.Fatal(err)
	}

	engine := testEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}
	if err := engine.Check(record.Context{UserID: 10}, "res.partner", record.OpRead, nil); err != nil {
		t.Fatal(err)
	}
	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(2)})
	if err != nil || !ok {
		t.Fatalf("expected company 2 allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected company 3 denied")
	}
	ok, err = engine.AllowedByRecordRules(10, "res.partner", record.OpWrite, map[string]any{"company_id": int64(3)})
	if err != nil || !ok {
		t.Fatalf("write should ignore read-only rule, got %v %v", ok, err)
	}
}

func TestParseDomainForceSupportsOdoo19BaseSecurityDomains(t *testing.T) {
	tests := []string{
		"[('create_uid','=', user.id)]",
		"['|', '|', ('partner_share', '=', False), ('company_id', 'parent_of', company_ids), ('company_id', '=', False)]",
		"[('company_id', 'in', company_ids + [False])]",
		"[('company_id', 'in', [False] + company_ids)]",
		"['&', ('company_id', 'in', [False] + company_ids), '|', ('pricelist_id', '=', False), ('pricelist_id.company_id', 'in', [False] + company_ids)]",
		"[('user_id','in',[False,user.id])]",
		"[('user_id', 'in', user.ids)]",
		"[('commercial_partner_id', '=', user.commercial_partner_id.id)]",
		"[('id', 'child_of', user.commercial_partner_id.id)]",
	}
	for _, text := range tests {
		if _, err := ParseDomainForce(text); err != nil {
			t.Fatalf("parse %s: %v", text, err)
		}
	}
}

func TestParseDomainForceSupportsOdoo19AnyDomains(t *testing.T) {
	tests := map[string]domain.Operator{
		"[('line_ids', 'any', [('state', '=', 'done')])]":       domain.AnyOf,
		"[('line_ids', 'not any', [('state', '=', 'cancel')])]": domain.NotAnyOf,
	}
	for text, want := range tests {
		node, err := ParseDomainForce(text)
		if err != nil {
			t.Fatalf("parse %s: %v", text, err)
		}
		if node.Kind != domain.Condition {
			t.Fatalf("parsed %s = %#v", text, node)
		}
		if node.Operator != want {
			t.Fatalf("operator for %s = %s, want %s", text, node.Operator, want)
		}
	}
}

func TestParsedOdoo19BaseSecurityDomainsEvaluate(t *testing.T) {
	user := User{ID: 10, PartnerID: 20, CommercialPartnerID: 30, CompanyID: 1, CompanyIDs: []int64{1, 2}}
	tests := []struct {
		expr string
		row  map[string]any
	}{
		{"[('create_uid','=', user.id)]", map[string]any{"create_uid": int64(10)}},
		{"[('company_id', 'in', company_ids + [False])]", map[string]any{"company_id": int64(2)}},
		{"[('company_id', 'in', company_ids + [False])]", map[string]any{"company_id": false}},
		{"[('company_id', 'in', [False] + company_ids)]", map[string]any{"company_id": int64(2)}},
		{"[('company_id', 'in', [False] + company_ids)]", map[string]any{"company_id": nil}},
		{"['&', ('company_id', 'in', [False] + company_ids), '|', ('pricelist_id', '=', False), ('pricelist_id.company_id', 'in', [False] + company_ids)]", map[string]any{"company_id": int64(2), "pricelist_id": false}},
		{"['&', ('company_id', 'in', [False] + company_ids), '|', ('pricelist_id', '=', False), ('pricelist_id.company_id', 'in', [False] + company_ids)]", map[string]any{"company_id": false, "pricelist_id": map[string]any{"company_id": int64(1)}}},
		{"[('user_id','in',[False,user.id])]", map[string]any{"user_id": int64(10)}},
		{"[('user_id','in',[False,user.id])]", map[string]any{"user_id": false}},
		{"[('user_id', 'in', user.ids)]", map[string]any{"user_id": int64(10)}},
		{"[('commercial_partner_id', '=', user.commercial_partner_id.id)]", map[string]any{"commercial_partner_id": int64(30)}},
		{"[('id', 'child_of', user.commercial_partner_id.id)]", map[string]any{"id": int64(31), "commercial_partner_id": int64(30)}},
	}
	for _, tt := range tests {
		node, err := ParseDomainForce(tt.expr)
		if err != nil {
			t.Fatalf("parse %s: %v", tt.expr, err)
		}
		ok, err := EvalDomain(tt.row, user, node)
		if err != nil || !ok {
			t.Fatalf("eval %s = %v %v", tt.expr, ok, err)
		}
	}
}

func TestPartnerPortalRuleUsesPartnerHierarchy(t *testing.T) {
	engine := NewEngine()
	engine.Users[10] = User{ID: 10, Login: "portal", Active: true, PartnerID: 101, CommercialPartnerID: 100}
	engine.Hierarchies["res.partner"] = map[int64]int64{
		100: 0,
		101: 100,
		102: 100,
		200: 0,
	}
	node, err := ParseDomainForce("[('id', 'child_of', user.commercial_partner_id.id)]")
	if err != nil {
		t.Fatal(err)
	}
	engine.Rules = []Rule{{Model: "res.partner", Active: true, PermRead: true, Domain: node}}

	for _, id := range []int64{100, 101, 102} {
		ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"id": id})
		if err != nil || !ok {
			t.Fatalf("partner %d should be allowed, got %v %v", id, ok, err)
		}
	}
	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"id": int64(200)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected unrelated partner denied")
	}

	moveNode, err := ParseDomainForce("[('partner_id', 'child_of', user.commercial_partner_id.id)]")
	if err != nil {
		t.Fatal(err)
	}
	engine.Rules = []Rule{{Model: "account.move", Active: true, PermRead: true, Domain: moveNode}}
	ok, err = engine.AllowedByRecordRules(10, "account.move", record.OpRead, map[string]any{"partner_id": int64(102)})
	if err != nil || !ok {
		t.Fatalf("move for child partner should be allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(10, "account.move", record.OpRead, map[string]any{"partner_id": int64(200)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected unrelated move partner denied")
	}
}

func TestOdoo19BaseSecurityRuleEffects(t *testing.T) {
	partnerRule, err := ParseDomainForce("['|', '|', ('partner_share', '=', False), ('company_id', 'parent_of', company_ids), ('company_id', '=', False)]")
	if err != nil {
		t.Fatal(err)
	}
	usersRule, err := ParseDomainForce("['|', ('share', '=', False), ('company_ids', 'in', company_ids)]")
	if err != nil {
		t.Fatal(err)
	}
	usersPortalRule, err := ParseDomainForce("[('commercial_partner_id', '=', user.commercial_partner_id.id)]")
	if err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	engine.Companies[1] = Company{ID: 1, Name: "Parent"}
	engine.Companies[2] = Company{ID: 2, Name: "Child", ParentID: 1}
	engine.Companies[3] = Company{ID: 3, Name: "Other"}
	engine.Users[10] = User{ID: 10, Login: "internal", Active: true, CompanyID: 2, CompanyIDs: []int64{2}, CommercialPartnerID: 100}
	engine.Users[20] = User{ID: 20, Login: "portal", Active: true, CompanyID: 2, CompanyIDs: []int64{2}, CommercialPartnerID: 100, GroupIDs: []int64{9}}
	engine.Groups[9] = Group{ID: 9, Name: "Portal"}

	engine.Rules = []Rule{{Model: "res.partner", Active: true, Global: true, PermRead: true, Domain: partnerRule}}
	for _, row := range []map[string]any{
		{"partner_share": false, "company_id": int64(3)},
		{"partner_share": true, "company_id": int64(1)},
		{"partner_share": true, "company_id": false},
	} {
		ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, row)
		if err != nil || !ok {
			t.Fatalf("partner row should be allowed: row=%+v ok=%v err=%v", row, ok, err)
		}
	}
	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"partner_share": true, "company_id": int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected unrelated shared partner denied")
	}

	engine.Rules = []Rule{{Model: "res.users", Active: true, Global: true, PermRead: true, Domain: usersRule}}
	for _, row := range []map[string]any{
		{"share": false, "company_ids": []int64{3}},
		{"share": true, "company_ids": []int64{2}},
	} {
		ok, err := engine.AllowedByRecordRules(10, "res.users", record.OpRead, row)
		if err != nil || !ok {
			t.Fatalf("user row should be allowed: row=%+v ok=%v err=%v", row, ok, err)
		}
	}
	ok, err = engine.AllowedByRecordRules(10, "res.users", record.OpRead, map[string]any{"share": true, "company_ids": []int64{3}})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected shared user outside companies denied")
	}

	engine.Rules = []Rule{
		{Model: "res.users", Active: true, Global: true, PermRead: true, Domain: usersRule},
		{Model: "res.users", Active: true, GroupIDs: []int64{9}, PermRead: true, Domain: usersPortalRule},
	}
	ok, err = engine.AllowedByRecordRules(20, "res.users", record.OpRead, map[string]any{"share": false, "company_ids": []int64{2}, "commercial_partner_id": int64(100)})
	if err != nil || !ok {
		t.Fatalf("portal same commercial partner should be allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(20, "res.users", record.OpRead, map[string]any{"share": false, "company_ids": []int64{2}, "commercial_partner_id": int64(200)})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected portal different commercial partner denied")
	}
}

func TestLoadedOdoo19BaseACLAndRecordRules(t *testing.T) {
	env, ids := loadedBaseSecurityEnv(t)
	mainCompanyID := ids["base.main_company"].ResID
	userGroupID := ids["base.group_user"].ResID
	systemGroupID := ids["base.group_system"].ResID
	portalGroupID := ids["base.group_portal"].ResID
	publicGroupID := ids["base.group_public"].ResID
	adminID := ids["base.user_admin"].ResID

	portalCommercialID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal Company", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	portalPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Portal Contact", "parent_id": portalCommercialID})
	if err != nil {
		t.Fatal(err)
	}
	publicCommercialID, err := env.Model("res.partner").Create(map[string]any{"name": "Public Company", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	publicPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Public Contact", "parent_id": publicCommercialID})
	if err != nil {
		t.Fatal(err)
	}
	otherPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Other Company", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	systemPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "System Contact"})
	if err != nil {
		t.Fatal(err)
	}
	employeePartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Employee Contact"})
	if err != nil {
		t.Fatal(err)
	}
	systemUserID, err := env.Model("res.users").Create(map[string]any{
		"login":       "system-active",
		"name":        "System Active",
		"active":      true,
		"company_id":  mainCompanyID,
		"company_ids": []int64{mainCompanyID},
		"group_ids":   []int64{userGroupID, systemGroupID},
		"partner_id":  systemPartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	employeeUserID, err := env.Model("res.users").Create(map[string]any{
		"login":       "employee-active",
		"name":        "Employee Active",
		"active":      true,
		"company_id":  mainCompanyID,
		"company_ids": []int64{mainCompanyID},
		"group_ids":   []int64{userGroupID},
		"partner_id":  employeePartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	portalUserID, err := env.Model("res.users").Create(map[string]any{
		"login":       "portal-active",
		"name":        "Portal Active",
		"active":      true,
		"company_id":  mainCompanyID,
		"company_ids": []int64{mainCompanyID},
		"group_ids":   []int64{portalGroupID},
		"partner_id":  portalPartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicUserID, err := env.Model("res.users").Create(map[string]any{
		"login":       "public-active",
		"name":        "Public Active",
		"active":      true,
		"company_id":  mainCompanyID,
		"company_ids": []int64{mainCompanyID},
		"group_ids":   []int64{publicGroupID},
		"partner_id":  publicPartnerID,
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name  string
		user  int64
		model string
		op    record.Operation
		err   error
	}{
		{"internal partner read", adminID, "res.partner", record.OpRead, nil},
		{"internal partner write through partner manager", adminID, "res.partner", record.OpWrite, nil},
		{"portal partner read", portalUserID, "res.partner", record.OpRead, nil},
		{"portal partner write denied", portalUserID, "res.partner", record.OpWrite, ErrAccessDenied},
		{"public partner read", publicUserID, "res.partner", record.OpRead, nil},
		{"public partner write denied", publicUserID, "res.partner", record.OpWrite, ErrAccessDenied},
		{"portal users read", portalUserID, "res.users", record.OpRead, nil},
		{"portal users write denied", portalUserID, "res.users", record.OpWrite, ErrAccessDenied},
	} {
		err := engine.Check(record.Context{UserID: tt.user}, tt.model, tt.op, nil)
		if !errors.Is(err, tt.err) {
			t.Fatalf("%s error = %v, want %v", tt.name, err, tt.err)
		}
	}

	assertFilteredFields(t, engine, systemUserID, "ir.attachment", []string{"name", "access_token"}, []string{"name", "access_token"})
	assertFilteredFields(t, engine, portalUserID, "ir.attachment", []string{"name", "access_token"}, []string{"name"})
	assertFilteredFields(t, engine, publicUserID, "ir.attachment", []string{"name", "access_token"}, []string{"name"})
	assertFilteredFields(t, engine, systemUserID, "ir.actions.server", []string{"name", "code"}, []string{"name", "code"})
	assertFilteredFields(t, engine, portalUserID, "ir.actions.server", []string{"name", "code"}, []string{"name"})
	assertFilteredFields(t, engine, systemUserID, "ir.mail_server", []string{"name", "smtp_pass", "smtp_ssl_private_key"}, []string{"name", "smtp_pass", "smtp_ssl_private_key"})
	assertFilteredFields(t, engine, portalUserID, "ir.mail_server", []string{"name", "smtp_pass", "smtp_ssl_private_key"}, []string{"name"})
	assertFilteredFields(t, engine, systemUserID, "res.users.identitycheck", []string{"user_id", "request"}, []string{"user_id"})
	assertFilteredFields(t, engine, 1, "res.users.identitycheck", []string{"user_id", "request"}, []string{"user_id", "request"})
	policyEnv := env.WithPolicy(engine)
	portalFieldsGet, err := policyEnv.WithContext(record.Context{UserID: portalUserID}).Model("res.users.identitycheck").FieldsGet([]string{"user_id", "request"}, []string{"type"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := portalFieldsGet["request"]; ok {
		t.Fatalf("portal fields_get exposes request: %+v", portalFieldsGet)
	}
	superFieldsGet, err := policyEnv.WithContext(record.Context{UserID: 1}).Model("res.users.identitycheck").FieldsGet([]string{"user_id", "request"}, []string{"type"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := superFieldsGet["request"]; !ok {
		t.Fatalf("superuser fields_get hides request: %+v", superFieldsGet)
	}

	assertRecordRule(t, engine, portalUserID, "res.partner", map[string]any{"id": portalPartnerID, "company_id": false, "partner_share": true}, true)
	assertRecordRule(t, engine, portalUserID, "res.partner", map[string]any{"id": otherPartnerID, "company_id": false, "partner_share": true}, false)
	assertRecordRule(t, engine, publicUserID, "res.partner", map[string]any{"id": publicPartnerID, "company_id": false, "partner_share": true}, true)
	assertRecordRule(t, engine, publicUserID, "res.partner", map[string]any{"id": otherPartnerID, "company_id": false, "partner_share": true}, false)
	assertRecordRule(t, engine, portalUserID, "res.users", map[string]any{"share": false, "company_ids": []int64{mainCompanyID}, "commercial_partner_id": portalCommercialID}, true)
	assertRecordRule(t, engine, portalUserID, "res.users", map[string]any{"share": false, "company_ids": []int64{mainCompanyID}, "commercial_partner_id": publicCommercialID}, false)
	assertRecordRule(t, engine, adminID, "res.partner", map[string]any{"id": otherPartnerID, "company_id": int64(999), "partner_share": true}, false)
	assertRecordRule(t, engine, employeeUserID, "ir.filters", map[string]any{"user_id": false}, true)
	assertRecordRule(t, engine, employeeUserID, "ir.filters", map[string]any{"user_id": employeeUserID}, true)
	assertRecordRule(t, engine, employeeUserID, "ir.filters", map[string]any{"user_id": portalUserID}, false)
}

func TestPersistedSecurityLoadsPrincipals(t *testing.T) {
	env := testBaseEnv(t)
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Main", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	portalID, err := env.Model("res.groups").Create(map[string]any{"name": "Portal"})
	if err != nil {
		t.Fatal(err)
	}
	userGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Internal", "implied_ids": []int64{portalID}})
	if err != nil {
		t.Fatal(err)
	}
	parentPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Main Partner", "is_company": true})
	if err != nil {
		t.Fatal(err)
	}
	childPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Child Partner", "parent_id": parentPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{
		"login":       "admin",
		"email":       "admin@example.test",
		"name":        "Admin",
		"active":      true,
		"company_id":  companyID,
		"company_ids": []int64{companyID},
		"partner_id":  childPartnerID,
		"groups_id":   []int64{userGroupID},
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	if err := engine.LoadPersistedSecurity(env); err != nil {
		t.Fatal(err)
	}

	if engine.Companies[companyID].Name != "Main" || !engine.Companies[companyID].Active {
		t.Fatalf("companies = %+v", engine.Companies)
	}
	if engine.Groups[userGroupID].Name != "Internal" || len(engine.Groups[userGroupID].ImpliedIDs) != 1 || engine.Groups[userGroupID].ImpliedIDs[0] != portalID {
		t.Fatalf("groups = %+v", engine.Groups)
	}
	user := engine.Users[userID]
	if user.Login != "admin" || user.Email != "admin@example.test" || user.CompanyID != companyID || len(user.CompanyIDs) != 1 || user.GroupIDs[0] != userGroupID || user.PartnerID != childPartnerID || user.CommercialPartnerID != parentPartnerID {
		t.Fatalf("user = %+v", user)
	}
	if engine.Hierarchies["res.partner"][childPartnerID] != parentPartnerID {
		t.Fatalf("partner hierarchy = %+v", engine.Hierarchies["res.partner"])
	}
	if groups := engine.EffectiveGroupIDs(userID); !groups[userGroupID] || !groups[portalID] {
		t.Fatalf("effective groups = %+v", groups)
	}
}

func TestGroupRulesAreORedAndCombinedWithGlobals(t *testing.T) {
	engine := testEngine()
	engine.Users[20] = User{ID: 20, Login: "reviewer", Name: "Reviewer", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{3}}
	engine.Rules = []Rule{
		{Model: "res.partner", Active: true, PermRead: true, Domain: domain.Cond("company_id", domain.In, "company_ids")},
		{Model: "res.partner", Active: true, PermRead: true, GroupIDs: []int64{3}, Domain: domain.Cond("flag", domain.Equal, true)},
	}

	ok, err := engine.AllowedByRecordRules(10, "res.partner", record.OpRead, map[string]any{"company_id": int64(1), "flag": false})
	if err != nil || !ok {
		t.Fatalf("user without matching group should only need globals, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(20, "res.partner", record.OpRead, map[string]any{"company_id": int64(1), "flag": false})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected grouped user denied by unmatched group rule")
	}
	ok, err = engine.AllowedByRecordRules(20, "res.partner", record.OpRead, map[string]any{"company_id": int64(1), "flag": true})
	if err != nil || !ok {
		t.Fatalf("expected grouped user allowed, got %v %v", ok, err)
	}
	ok, err = engine.AllowedByRecordRules(20, "res.partner", record.OpRead, map[string]any{"company_id": int64(3), "flag": true})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected global company rule to deny")
	}
}

func TestParseDomainForceSupportsOdooBooleanLeaf(t *testing.T) {
	node, err := ParseDomainForce("[(1, '=', 1)]")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := EvalDomain(map[string]any{}, User{}, node)
	if err != nil || !ok {
		t.Fatalf("expected true leaf, got %v %v", ok, err)
	}
}

func TestEvalDomainSupportsOdooOperators(t *testing.T) {
	row := map[string]any{
		"name":       "Administrator",
		"code":       "ADM-001",
		"score":      int64(10),
		"company_id": int64(2),
		"tag_ids":    []int64{4, 7},
		"partner": map[string]any{
			"company_id": int64(2),
		},
	}
	user := User{ID: 10, CompanyID: 2, CompanyIDs: []int64{1, 2}}
	tests := []struct {
		name string
		node domain.Node
	}{
		{"optional false", domain.Cond("name", domain.OptionalEqual, false)},
		{"less equal", domain.Cond("score", domain.LessEqual, 10)},
		{"greater", domain.Cond("score", domain.Greater, 5)},
		{"ilike", domain.Cond("name", domain.ILike, "admin")},
		{"not ilike", domain.Cond("name", domain.NotILike, "demo")},
		{"equal like", domain.Cond("code", domain.EqualLike, "ADM-%")},
		{"equal ilike", domain.Cond("code", domain.EqualILike, "adm-%")},
		{"slice in", domain.Cond("tag_ids", domain.In, []int{7})},
		{"user company in", domain.Cond("company_id", domain.In, "user.company_ids")},
		{"nested field", domain.Cond("partner.company_id", domain.Equal, "company_id")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := EvalDomain(row, user, tt.node)
			if err != nil || !ok {
				t.Fatalf("expected match, got %v %v", ok, err)
			}
		})
	}
}

func TestEvalDomainSupportsAnyAndNotAnyOnRowCollections(t *testing.T) {
	row := map[string]any{
		"line_ids": []map[string]any{
			{"state": "draft", "amount": int64(50)},
			{"state": "done", "amount": int64(100)},
		},
		"empty_ids": []map[string]any{},
	}
	tests := []struct {
		name string
		node domain.Node
		want bool
	}{
		{"any nested domain", domain.Cond("line_ids", domain.AnyOf, domain.Cond("state", domain.Equal, "done")), true},
		{"not any nested domain", domain.Cond("line_ids", domain.NotAnyOf, domain.Cond("state", domain.Equal, "cancel")), true},
		{"any empty subdomain nonempty", domain.Cond("line_ids", domain.AnyOf, domain.And()), true},
		{"any empty subdomain empty", domain.Cond("empty_ids", domain.AnyOf, domain.And()), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := EvalDomain(row, User{}, tt.node)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tt.want {
				t.Fatalf("EvalDomain = %v, want %v", ok, tt.want)
			}
		})
	}

	_, err := EvalDomain(map[string]any{"line_ids": int64(9)}, User{}, domain.Cond("line_ids", domain.AnyOf, domain.Cond("state", domain.Equal, "done")))
	if err == nil || !strings.Contains(err.Error(), "row-resident collection") {
		t.Fatalf("expected unsupported relational any error, got %v", err)
	}
}

func TestAuditLog(t *testing.T) {
	engine := testEngine()
	engine.Log(AuditEvent{ActorID: 10, Action: "password_change", Details: map[string]string{"password": "raw", "note": "ok"}})
	last := engine.Audit[len(engine.Audit)-1]
	if last.Details["password"] != "[redacted]" || last.Details["note"] != "ok" {
		t.Fatalf("unexpected audit details: %+v", last.Details)
	}
}

func testBaseEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, m := range base.Models() {
		if err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1})
}

func loadedBaseSecurityEnv(t *testing.T) (*record.Env, map[string]data.ExternalID) {
	t.Helper()
	env := testBaseEnv(t)
	manifest := base.Manifest()
	externalIDs := map[string]data.ExternalID{}
	loader := data.NewLoaderWithExternalIDs(env, manifest.TechnicalName, externalIDs)
	baseDir := basePackageDir(t)
	loader.SetBaseDir(baseDir)
	if err := data.LoadModelMetadata(env, manifest.TechnicalName, base.Models(), externalIDs); err != nil {
		t.Fatal(err)
	}
	for _, name := range manifest.Data {
		file, err := os.Open(filepath.Join(baseDir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		switch filepath.Ext(name) {
		case ".xml":
			err = loader.LoadXML(file)
		case ".csv":
			err = loader.LoadCSV(strings.TrimSuffix(filepath.Base(name), ".csv"), file)
		}
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
	}
	return env, loader.ExternalIDs()
}

func basePackageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "base"))
}

func assertRecordRule(t *testing.T, engine *Engine, userID int64, modelName string, row map[string]any, want bool) {
	t.Helper()
	ok, err := engine.CheckRecord(record.Context{UserID: userID}, modelName, record.OpRead, row)
	if err != nil {
		t.Fatal(err)
	}
	if ok != want {
		t.Fatalf("%s rule for user %d row %+v = %v, want %v", modelName, userID, row, ok, want)
	}
}

func assertFilteredFields(t *testing.T, engine *Engine, userID int64, modelName string, fields []string, want []string) {
	t.Helper()
	got := engine.FilterFields(record.Context{UserID: userID}, modelName, append([]string(nil), fields...))
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%s fields for user %d = %+v, want %+v", modelName, userID, got, want)
	}
}

func testEngine() *Engine {
	engine := NewEngine()
	engine.Companies[1] = Company{ID: 1, Name: "Main", Active: true}
	engine.Companies[2] = Company{ID: 2, Name: "Branch", ParentID: 1, Active: true}
	engine.Groups[1] = Group{ID: 1, Name: "Internal", ImpliedIDs: []int64{2}}
	engine.Groups[2] = Group{ID: 2, Name: "Portal"}
	engine.Groups[3] = Group{ID: 3, Name: "Secret"}
	engine.Users[10] = User{ID: 10, Login: "admin", Name: "Admin", Active: true, CompanyID: 1, CompanyIDs: []int64{1, 2}, GroupIDs: []int64{1}}
	engine.ACLs = []ACL{
		{Model: "res.partner", GroupID: 1, PermRead: true, PermWrite: true, PermCreate: true},
	}
	return engine
}

func withValues(base map[string]any, values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range values {
		out[key] = value
	}
	return out
}
