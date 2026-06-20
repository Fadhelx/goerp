package delegation

import (
	"errors"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestDelegationLifecycleEffectiveGroupsAndAdapters(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	access := &accessSpy{}
	menus := &menuSpy{}
	mail := &mailSpy{}
	cache := &cacheSpy{}
	hooks := &workflowSpy{}
	svc := NewService(
		WithNow(func() time.Time { return now }),
		WithAccessResolver(access),
		WithMenuResolver(menus),
		WithMailResolver(mail),
		WithCacheInvalidator(cache),
		WithWorkflowHooks(hooks),
	)
	svc.SetGroupConfig(GroupConfig{GroupID: 200, Name: "Approver", AllowDelegation: true})

	req, err := svc.CreateRequest(RequestInput{
		DateFrom:            now,
		DateTo:              now.AddDate(0, 0, 2),
		DelegatorEmployeeID: 1000,
		DelegatorUserID:     10,
		OneEmployee:         true,
		DelegateToUserID:    20,
		Lines:               []LineInput{{GroupID: 200}},
		SourceModel:         "purchase.order",
		SourceRecordID:      99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Name != "DEL/00001" || req.State != StateDraft || req.Lines[0].DelegateUserID != 20 {
		t.Fatalf("unexpected request: %+v", req)
	}
	if _, err := svc.Submit(req.ID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.Confirm(req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.State != StateConfirmed || !confirmed.Lines[0].Active {
		t.Fatalf("not confirmed: %+v", confirmed)
	}
	if !reflect.DeepEqual(hooks.events, []string{"submitted:1", "confirmed:1"}) {
		t.Fatalf("hooks = %+v", hooks.events)
	}
	if !reflect.DeepEqual(cache.ids, []int64{10, 20}) {
		t.Fatalf("cache ids = %+v", cache.ids)
	}

	if got := svc.DelegatedGroupIDs(20, now); !reflect.DeepEqual(got, []int64{200}) {
		t.Fatalf("delegated groups = %+v", got)
	}
	if got := svc.EffectiveGroupIDs(20, []int64{100}, now); !reflect.DeepEqual(got, []int64{100, 200}) {
		t.Fatalf("effective groups = %+v", got)
	}
	if got := svc.DelegatorUserIDs(20, now); !reflect.DeepEqual(got, []int64{10}) {
		t.Fatalf("delegator users = %+v", got)
	}
	if got := svc.DelegatedUserIDs(10, now); !reflect.DeepEqual(got, []int64{20}) {
		t.Fatalf("delegated users = %+v", got)
	}

	ok, err := svc.CanAccess(20, []int64{100}, "purchase.order", "write", now)
	if err != nil || !ok {
		t.Fatalf("can access = %v %v", ok, err)
	}
	if access.ctx.UserID != 20 || !reflect.DeepEqual(access.ctx.DelegatorUserIDs, []int64{10}) || !reflect.DeepEqual(access.ctx.DelegatedGroupIDs, []int64{200}) {
		t.Fatalf("access ctx = %+v", access.ctx)
	}
	domains, err := svc.RuleDomains(20, nil, "purchase.order", "read", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0].DelegatorUserID != 10 || domains[0].GroupID != 200 {
		t.Fatalf("domains = %+v", domains)
	}
	menuIDs, err := svc.VisibleMenuIDs(20, nil, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(menuIDs, []int64{5, 7}) {
		t.Fatalf("menus = %+v", menuIDs)
	}
	cc, err := svc.ExpandMailCC(MailContext{TemplateID: 1, Model: "purchase.order", RecordID: 99, UserID: 10, InitialCC: []string{"owner@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cc, []string{"delegate20@example.com", "owner@example.com"}) {
		t.Fatalf("cc = %+v", cc)
	}
	if !reflect.DeepEqual(mail.ctx.DelegatedUsers, []int64{20}) || !reflect.DeepEqual(mail.ctx.DelegatedGroupIDs, []int64{200}) {
		t.Fatalf("mail ctx = %+v", mail.ctx)
	}
}

func TestDelegatedApprovalUserIDsFiltersDepartments(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := NewService(WithNow(func() time.Time { return now }))
	svc.SetGroupConfig(GroupConfig{GroupID: 200, Name: "Approver", AllowDelegation: true, AllowMultipleDelegation: true})
	req, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		DepartmentIDs:   []int64{30},
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	globalReq, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 21}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(globalReq.ID); err != nil {
		t.Fatal(err)
	}

	if got := svc.DelegatedApprovalUserIDs([]int64{10}, []int64{200}, []int64{30}, now); !reflect.DeepEqual(got, []int64{20, 21}) {
		t.Fatalf("matching department delegates = %+v", got)
	}
	if got := svc.DelegatedApprovalUserIDs([]int64{10}, []int64{200}, []int64{40}, now); !reflect.DeepEqual(got, []int64{21}) {
		t.Fatalf("nonmatching department delegates = %+v", got)
	}
	if got := svc.DelegatedApprovalUserIDs([]int64{10}, []int64{200}, nil, now); !reflect.DeepEqual(got, []int64{20, 21}) {
		t.Fatalf("empty record department delegates = %+v", got)
	}
	if got := svc.DelegatedApprovalUserIDs([]int64{99}, []int64{200}, []int64{30}, now); len(got) != 0 {
		t.Fatalf("other delegator delegates = %+v", got)
	}
	if got := svc.DelegatedApprovalUserIDs([]int64{10}, []int64{201}, []int64{30}, now); len(got) != 0 {
		t.Fatalf("other approval group delegates = %+v", got)
	}
	if got := svc.DelegatedApprovalUserIDs([]int64{10, 20}, []int64{200}, []int64{30}, now); !reflect.DeepEqual(got, []int64{21}) {
		t.Fatalf("existing approver delegates = %+v", got)
	}
	if got := svc.ActiveApprovalDelegationID(20, []int64{10}, []int64{200}, []int64{30}, now); got != req.ID {
		t.Fatalf("matching delegation id = %d", got)
	}
	if got := svc.ActiveApprovalDelegationID(21, []int64{10}, []int64{200}, []int64{40}, now); got != globalReq.ID {
		t.Fatalf("global delegation id = %d", got)
	}
	if got := svc.ActiveApprovalDelegationID(20, []int64{10}, []int64{200}, []int64{40}, now); got != 0 {
		t.Fatalf("nonmatching delegation id = %d", got)
	}
}

func TestDelegationValidationOverlapRevokeAndExpiration(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	hooks := &workflowSpy{}
	svc := NewService(WithNow(func() time.Time { return now }), WithWorkflowHooks(hooks))
	svc.SetGroupConfig(GroupConfig{GroupID: 200, Name: "Approver", AllowDelegation: true})
	svc.SetGroupConfig(GroupConfig{GroupID: 201, Name: "Reviewer", AllowDelegation: true, AllowMultipleDelegation: true})

	if _, err := svc.CreateRequest(RequestInput{DateFrom: now, DateTo: now.AddDate(0, 0, 1), DelegatorUserID: 10, Lines: []LineInput{{GroupID: 999, DelegateUserID: 20}}}); !errors.Is(err, ErrGroupNotDelegable) {
		t.Fatalf("expected group error, got %v", err)
	}
	if _, err := svc.CreateRequest(RequestInput{DateFrom: now, DateTo: now.AddDate(0, 0, 1), DelegatorUserID: 10, Lines: []LineInput{{GroupID: 200, DelegateUserID: 10}}}); !errors.Is(err, ErrSelfDelegation) {
		t.Fatalf("expected self delegation error, got %v", err)
	}
	past, err := svc.CreateRequest(RequestInput{
		DateFrom:        now.AddDate(0, 0, -1),
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(past.ID); !errors.Is(err, ErrPastStartDate) {
		t.Fatalf("expected past start submit error, got %v", err)
	}

	first, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 2),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateRequest(RequestInput{
		DateFrom:        now.AddDate(0, 0, 1),
		DateTo:          now.AddDate(0, 0, 3),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 30}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(second.ID); !errors.Is(err, ErrOverlappingDelegation) {
		t.Fatalf("expected overlap, got %v", err)
	}

	multiple, err := svc.CreateRequest(RequestInput{
		DateFrom:        now.AddDate(0, 0, 1),
		DateTo:          now.AddDate(0, 0, 3),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 201, DelegateUserID: 30}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(multiple.ID); err != nil {
		t.Fatal(err)
	}

	version := svc.CacheVersion()
	if got := svc.ActiveLines(20, now); len(got) != 1 {
		t.Fatalf("active lines = %+v", got)
	}
	if _, err := svc.Revoke(first.ID); err != nil {
		t.Fatal(err)
	}
	if svc.CacheVersion() <= version {
		t.Fatal("cache version did not advance")
	}
	if got := svc.ActiveLines(20, now); len(got) != 0 {
		t.Fatalf("revoked lines still active: %+v", got)
	}

	ended, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 11,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 40}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ended.ID); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	if _, err := svc.Revoke(ended.ID); !errors.Is(err, ErrPastEndDate) {
		t.Fatalf("expected past end revoke error, got %v", err)
	}
	now = now.AddDate(0, 0, -2)

	expired, err := svc.ExpireDue(now.AddDate(0, 0, 4))
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 2 || expired[0].ID != multiple.ID || expired[0].State != StateExpired || expired[1].ID != ended.ID || expired[1].State != StateExpired {
		t.Fatalf("expired = %+v", expired)
	}
	wantEvents := []string{"confirmed:2", "confirmed:4", "revoked:2", "confirmed:5", "expired:4", "expired:5"}
	gotEvents := append([]string(nil), hooks.events...)
	sort.Strings(gotEvents)
	sort.Strings(wantEvents)
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("events = %+v", hooks.events)
	}
}

func TestRevalidateActiveLinesDeactivatesInvalidDelegatorState(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	cache := &cacheSpy{}
	svc := NewService(WithNow(func() time.Time { return now }), WithCacheInvalidator(cache))
	svc.SetGroupConfig(GroupConfig{GroupID: 200, Name: "Approver", AllowDelegation: true})

	req, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 2),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if got := svc.ActiveLines(20, now); len(got) != 1 {
		t.Fatalf("active lines = %+v", got)
	}

	cache.ids = nil
	result := svc.RevalidateActiveLines([]UserState{{UserID: 10, Active: true, GroupIDs: []int64{999}}}, now)
	if len(result.Deactivated) != 1 || result.Deactivated[0].GroupID != 200 || result.Deactivated[0].DelegatorUserID != 10 || result.Deactivated[0].DelegateUserID != 20 {
		t.Fatalf("deactivated = %+v", result.Deactivated)
	}
	if !reflect.DeepEqual(result.AffectedUserIDs, []int64{10, 20}) {
		t.Fatalf("affected user ids = %+v", result.AffectedUserIDs)
	}
	if !reflect.DeepEqual(cache.ids, []int64{10, 20}) {
		t.Fatalf("cache ids = %+v", cache.ids)
	}
	stored, ok := svc.Request(req.ID)
	if !ok || stored.State != StateConfirmed || stored.Lines[0].State != StateConfirmed || stored.Lines[0].Active {
		t.Fatalf("stored request = %+v", stored)
	}
	if got := svc.ActiveLines(20, now); len(got) != 0 {
		t.Fatalf("deactivated line still active = %+v", got)
	}

	overlap, err := svc.CreateRequest(RequestInput{
		DateFrom:        now.AddDate(0, 0, 1),
		DateTo:          now.AddDate(0, 0, 3),
		DelegatorUserID: 10,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 30}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(overlap.ID); err != nil {
		t.Fatalf("inactive line should not block overlap: %v", err)
	}

	if result := svc.RevalidateActiveLines([]UserState{{UserID: 10, Active: true, GroupIDs: []int64{200}}}, now); len(result.Deactivated) != 0 {
		t.Fatalf("unexpected reactivation/deactivation result = %+v", result)
	}
	stored, ok = svc.Request(req.ID)
	if !ok || stored.Lines[0].Active {
		t.Fatalf("line reactivated unexpectedly = %+v", stored)
	}

	inactiveReq, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 2),
		DelegatorUserID: 11,
		Lines:           []LineInput{{GroupID: 200, DelegateUserID: 40}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(inactiveReq.ID); err != nil {
		t.Fatal(err)
	}
	cache.ids = nil
	result = svc.RevalidateActiveLines([]UserState{{UserID: 11, Active: false, GroupIDs: []int64{200}}}, now)
	if len(result.Deactivated) != 1 || result.Deactivated[0].DelegatorUserID != 11 || result.Deactivated[0].DelegateUserID != 40 {
		t.Fatalf("inactive delegator deactivated = %+v", result.Deactivated)
	}
	if !reflect.DeepEqual(cache.ids, []int64{11, 40}) {
		t.Fatalf("inactive delegator cache ids = %+v", cache.ids)
	}
	if got := svc.ActiveLines(40, now); len(got) != 0 {
		t.Fatalf("inactive delegator line still active = %+v", got)
	}
}

func TestRestrictedAccessFiltersDelegatedGroups(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	svc := NewService(WithNow(func() time.Time { return now }), WithRestrictedAccess(true))
	svc.SetGroupConfig(GroupConfig{GroupID: 200, Name: "Approver", AllowDelegation: true})
	svc.SetGroupConfig(GroupConfig{GroupID: 201, Name: "Reviewer", AllowDelegation: true, RestrictedAccess: true})
	req, err := svc.CreateRequest(RequestInput{
		DateFrom:        now,
		DateTo:          now.AddDate(0, 0, 1),
		DelegatorUserID: 10,
		Lines: []LineInput{
			{GroupID: 200, DelegateUserID: 20},
			{GroupID: 201, DelegateUserID: 20},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(req.ID); err != nil {
		t.Fatal(err)
	}
	if got := svc.DelegatedGroupIDs(20, now); !reflect.DeepEqual(got, []int64{201}) {
		t.Fatalf("restricted delegated groups = %+v", got)
	}
	if got := svc.EffectiveGroupIDs(20, []int64{100}, now); !reflect.DeepEqual(got, []int64{100, 201}) {
		t.Fatalf("restricted effective groups = %+v", got)
	}
	if got := svc.DelegatedUserGroupIDsForDelegator(10, now); !reflect.DeepEqual(got, map[int64][]int64{20: {201}}) {
		t.Fatalf("restricted delegated user groups = %+v", got)
	}
	svc.SetRestrictedAccess(false)
	if got := svc.DelegatedGroupIDs(20, now); !reflect.DeepEqual(got, []int64{200, 201}) {
		t.Fatalf("unrestricted delegated groups = %+v", got)
	}
}

func TestDelegationTemplatesAreMetadataOnly(t *testing.T) {
	templates := DefaultMailTemplates()
	if len(templates) != 3 {
		t.Fatalf("templates = %+v", templates)
	}
	for _, template := range templates {
		if template.XMLID == "" || template.Name == "" || template.Purpose == "" || !template.Active {
			t.Fatalf("bad template: %+v", template)
		}
	}
}

type accessSpy struct {
	ctx AccessContext
}

func (a *accessSpy) CanAccess(ctx AccessContext, modelName string, operation string) (bool, error) {
	a.ctx = ctx
	return modelName == "purchase.order" && operation == "write", nil
}

func (a *accessSpy) RuleDomains(ctx AccessContext, modelName string, operation string) ([]RuleDomain, error) {
	a.ctx = ctx
	return []RuleDomain{{GroupID: ctx.DelegatedGroupIDs[0], DelegatorUserID: ctx.DelegatorUserIDs[0], Model: modelName, Operation: operation, Expression: "delegated_user"}}, nil
}

type menuSpy struct {
	ctx AccessContext
}

func (m *menuSpy) VisibleMenuIDs(ctx AccessContext, debug bool) ([]int64, error) {
	m.ctx = ctx
	return []int64{7, 5, 5}, nil
}

type mailSpy struct {
	ctx MailContext
}

func (m *mailSpy) ExpandCC(ctx MailContext) ([]string, error) {
	m.ctx = ctx
	return []string{"delegate20@example.com"}, nil
}

type cacheSpy struct {
	ids []int64
}

func (c *cacheSpy) InvalidateDelegationCache(ids []int64) {
	c.ids = append([]int64(nil), ids...)
}

type workflowSpy struct {
	events []string
}

func (w *workflowSpy) OnSubmitted(req Request) error {
	w.events = append(w.events, "submitted:"+itoa(req.ID))
	return nil
}

func (w *workflowSpy) OnConfirmed(req Request) error {
	w.events = append(w.events, "confirmed:"+itoa(req.ID))
	return nil
}

func (w *workflowSpy) OnRevoked(req Request) error {
	w.events = append(w.events, "revoked:"+itoa(req.ID))
	return nil
}

func (w *workflowSpy) OnExpired(req Request) error {
	w.events = append(w.events, "expired:"+itoa(req.ID))
	return nil
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
