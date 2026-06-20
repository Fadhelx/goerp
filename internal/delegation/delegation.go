package delegation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateDraft     State = "draft"
	StateSubmitted State = "submitted"
	StateConfirmed State = "confirmed"
	StateRevoked   State = "revoked"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

var (
	ErrInvalidDateRange      = errors.New("invalid delegation date range")
	ErrPastStartDate         = errors.New("delegation start date is in the past")
	ErrPastEndDate           = errors.New("delegation end date is in the past")
	ErrDelegatorRequired     = errors.New("delegator user is required")
	ErrDelegateRequired      = errors.New("delegate user is required")
	ErrSelfDelegation        = errors.New("delegator and delegate must differ")
	ErrLineRequired          = errors.New("delegation requires at least one line")
	ErrGroupNotDelegable     = errors.New("group is not delegable")
	ErrOverlappingDelegation = errors.New("overlapping delegation")
	ErrRequestNotFound       = errors.New("delegation request not found")
	ErrInvalidState          = errors.New("invalid delegation state")
)

type Sequence struct {
	Prefix  string
	Next    int64
	Padding int
}

func DefaultSequence() Sequence {
	return Sequence{Prefix: "DEL", Next: 1, Padding: 5}
}

func (s *Sequence) NextValue() string {
	if s.Prefix == "" {
		s.Prefix = "DEL"
	}
	if s.Padding <= 0 {
		s.Padding = 5
	}
	if s.Next <= 0 {
		s.Next = 1
	}
	value := fmt.Sprintf("%s/%0*d", s.Prefix, s.Padding, s.Next)
	s.Next++
	return value
}

type GroupConfig struct {
	GroupID                 int64
	Name                    string
	DisplayName             string
	AllowDelegation         bool
	AllowMultipleDelegation bool
	RestrictedAccess        bool
	TemplateIDs             []int64
}

type RequestInput struct {
	DateFrom             time.Time
	DateTo               time.Time
	DelegatorEmployeeID  int64
	DelegatorUserID      int64
	OneEmployee          bool
	DelegateToEmployeeID int64
	DelegateToUserID     int64
	DepartmentIDs        []int64
	Lines                []LineInput
	SourceModel          string
	SourceRecordID       int64
	Metadata             map[string]string
}

type LineInput struct {
	GroupID            int64
	DelegateEmployeeID int64
	DelegateUserID     int64
}

type Request struct {
	ID                   int64
	Name                 string
	DateFrom             time.Time
	DateTo               time.Time
	DelegatorEmployeeID  int64
	DelegatorUserID      int64
	OneEmployee          bool
	DelegateToEmployeeID int64
	DelegateToUserID     int64
	DepartmentIDs        []int64
	Lines                []Line
	State                State
	CreatedAt            time.Time
	SubmittedAt          time.Time
	ConfirmedAt          time.Time
	RevokedAt            time.Time
	ExpiredAt            time.Time
	SourceModel          string
	SourceRecordID       int64
	Metadata             map[string]string
}

type Line struct {
	ID                  int64
	RequestID           int64
	GroupID             int64
	DelegateEmployeeID  int64
	DelegateUserID      int64
	DelegatorEmployeeID int64
	DelegatorUserID     int64
	DepartmentIDs       []int64
	DateFrom            time.Time
	DateTo              time.Time
	State               State
	Active              bool
}

type UserState struct {
	UserID   int64
	Active   bool
	GroupIDs []int64
}

type RevalidationResult struct {
	Deactivated     []Line
	AffectedUserIDs []int64
}

type AccessContext struct {
	UserID            int64
	DelegatorUserIDs  []int64
	DelegatedGroupIDs []int64
	EffectiveGroupIDs []int64
	Lines             []Line
	At                time.Time
}

type RuleDomain struct {
	GroupID         int64
	DelegatorUserID int64
	Model           string
	Operation       string
	Expression      string
	DepartmentIDs   []int64
}

type AccessResolver interface {
	CanAccess(AccessContext, string, string) (bool, error)
	RuleDomains(AccessContext, string, string) ([]RuleDomain, error)
}

type MenuResolver interface {
	VisibleMenuIDs(AccessContext, bool) ([]int64, error)
}

type MailContext struct {
	TemplateID           int64
	TemplateGroupIDs     []int64
	Model                string
	RecordID             int64
	UserID               int64
	InitialCC            []string
	DelegatedUsers       []int64
	DelegatedGroupIDs    []int64
	DelegatedUserGroupID map[int64][]int64
	At                   time.Time
}

type MailResolver interface {
	ExpandCC(MailContext) ([]string, error)
}

type CacheInvalidator interface {
	InvalidateDelegationCache([]int64)
}

type WorkflowHooks interface {
	OnSubmitted(Request) error
	OnConfirmed(Request) error
	OnRevoked(Request) error
	OnExpired(Request) error
}

type MailTemplateMetadata struct {
	ID                 int64
	XMLID              string
	Name               string
	Subject            string
	Purpose            string
	DelegationGroupIDs []int64
	Active             bool
}

type Option func(*Service)

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithSequence(sequence Sequence) Option {
	return func(s *Service) {
		s.sequence = sequence
	}
}

func WithAccessResolver(resolver AccessResolver) Option {
	return func(s *Service) {
		s.access = resolver
	}
}

func WithMenuResolver(resolver MenuResolver) Option {
	return func(s *Service) {
		s.menus = resolver
	}
}

func WithMailResolver(resolver MailResolver) Option {
	return func(s *Service) {
		s.mail = resolver
	}
}

func WithRestrictedAccess(enabled bool) Option {
	return func(s *Service) {
		s.restrictedAccess = enabled
	}
}

func WithCacheInvalidator(invalidator CacheInvalidator) Option {
	return func(s *Service) {
		s.cacheInvalidator = invalidator
	}
}

func WithWorkflowHooks(hooks WorkflowHooks) Option {
	return func(s *Service) {
		s.workflow = hooks
	}
}

type Service struct {
	mu               sync.RWMutex
	nextRequestID    int64
	nextLineID       int64
	sequence         Sequence
	requests         map[int64]Request
	groups           map[int64]GroupConfig
	restrictedAccess bool
	cache            map[cacheKey]cacheEntry
	cacheVersion     int64
	now              func() time.Time
	access           AccessResolver
	menus            MenuResolver
	mail             MailResolver
	cacheInvalidator CacheInvalidator
	workflow         WorkflowHooks
}

type cacheKey struct {
	UserID int64
	Day    string
}

type cacheEntry struct {
	Version int64
	Lines   []Line
}

func NewService(options ...Option) *Service {
	s := &Service{
		nextRequestID: 1,
		nextLineID:    1,
		sequence:      DefaultSequence(),
		requests:      map[int64]Request{},
		groups:        map[int64]GroupConfig{},
		cache:         map[cacheKey]cacheEntry{},
		now:           func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Service) SetGroupConfig(config GroupConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.TemplateIDs = cloneInt64s(config.TemplateIDs)
	s.groups[config.GroupID] = config
	s.cache = map[cacheKey]cacheEntry{}
	s.cacheVersion++
}

func (s *Service) SetNow(now func() time.Time) {
	if now == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *Service) GroupConfig(groupID int64) (GroupConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config, ok := s.groups[groupID]
	config.TemplateIDs = cloneInt64s(config.TemplateIDs)
	return config, ok
}

func (s *Service) SetMenuResolver(resolver MenuResolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.menus = resolver
}

func (s *Service) SetRestrictedAccess(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restrictedAccess == enabled {
		return
	}
	s.restrictedAccess = enabled
	s.cache = map[cacheKey]cacheEntry{}
	s.cacheVersion++
}

func (s *Service) RestrictedAccess() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.restrictedAccess
}

func (s *Service) CreateRequest(input RequestInput) (Request, error) {
	now := s.now()
	req, err := s.buildRequest(input, now)
	if err != nil {
		return Request{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateGroupsLocked(req); err != nil {
		return Request{}, err
	}
	req.ID = s.nextRequestID
	s.nextRequestID++
	req.Name = s.sequence.NextValue()
	req.State = StateDraft
	req.CreatedAt = now.UTC()
	for i := range req.Lines {
		req.Lines[i].ID = s.nextLineID
		s.nextLineID++
		req.Lines[i].RequestID = req.ID
		req.Lines[i].State = StateDraft
		req.Lines[i].Active = false
	}
	s.requests[req.ID] = cloneRequest(req)
	return cloneRequest(req), nil
}

func (s *Service) Submit(id int64) (Request, error) {
	s.mu.Lock()
	req, ok := s.requests[id]
	if !ok {
		s.mu.Unlock()
		return Request{}, ErrRequestNotFound
	}
	if req.State != StateDraft {
		s.mu.Unlock()
		return Request{}, ErrInvalidState
	}
	if err := s.validateReadyLocked(req, true); err != nil {
		s.mu.Unlock()
		return Request{}, err
	}
	now := s.now().UTC()
	req.State = StateSubmitted
	req.SubmittedAt = now
	for i := range req.Lines {
		req.Lines[i].State = StateSubmitted
	}
	s.requests[id] = cloneRequest(req)
	out := cloneRequest(req)
	hooks := s.workflow
	s.mu.Unlock()

	if hooks != nil {
		if err := hooks.OnSubmitted(out); err != nil {
			return Request{}, err
		}
	}
	return out, nil
}

func (s *Service) Confirm(id int64) (Request, error) {
	s.mu.Lock()
	req, ok := s.requests[id]
	if !ok {
		s.mu.Unlock()
		return Request{}, ErrRequestNotFound
	}
	if req.State != StateDraft && req.State != StateSubmitted {
		s.mu.Unlock()
		return Request{}, ErrInvalidState
	}
	if err := s.validateReadyLocked(req, true); err != nil {
		s.mu.Unlock()
		return Request{}, err
	}
	now := s.now().UTC()
	req.State = StateConfirmed
	req.ConfirmedAt = now
	for i := range req.Lines {
		req.Lines[i].State = StateConfirmed
		req.Lines[i].Active = true
	}
	s.requests[id] = cloneRequest(req)
	affected := affectedUsers(req)
	s.invalidateLocked(affected)
	out := cloneRequest(req)
	hooks := s.workflow
	s.mu.Unlock()

	if hooks != nil {
		if err := hooks.OnConfirmed(out); err != nil {
			return Request{}, err
		}
	}
	return out, nil
}

func (s *Service) Revoke(id int64) (Request, error) {
	s.mu.Lock()
	req, ok := s.requests[id]
	if !ok {
		s.mu.Unlock()
		return Request{}, ErrRequestNotFound
	}
	if req.State == StateRevoked || req.State == StateExpired || req.State == StateCancelled {
		s.mu.Unlock()
		return Request{}, ErrInvalidState
	}
	now := s.now().UTC()
	if dateOnly(req.DateTo).Before(dateOnly(now)) {
		s.mu.Unlock()
		return Request{}, ErrPastEndDate
	}
	req.State = StateRevoked
	req.RevokedAt = now
	for i := range req.Lines {
		req.Lines[i].State = StateRevoked
		req.Lines[i].Active = false
	}
	s.requests[id] = cloneRequest(req)
	affected := affectedUsers(req)
	s.invalidateLocked(affected)
	out := cloneRequest(req)
	hooks := s.workflow
	s.mu.Unlock()

	if hooks != nil {
		if err := hooks.OnRevoked(out); err != nil {
			return Request{}, err
		}
	}
	return out, nil
}

func (s *Service) ExpireDue(at time.Time) ([]Request, error) {
	s.mu.Lock()
	date := dateOnly(at)
	var expired []Request
	var affected []int64
	for id, req := range s.requests {
		if req.State != StateConfirmed || !dateOnly(req.DateTo).Before(date) {
			continue
		}
		req.State = StateExpired
		req.ExpiredAt = at.UTC()
		for i := range req.Lines {
			req.Lines[i].State = StateExpired
			req.Lines[i].Active = false
		}
		s.requests[id] = cloneRequest(req)
		expired = append(expired, cloneRequest(req))
		affected = append(affected, affectedUsers(req)...)
	}
	s.invalidateLocked(affected)
	hooks := s.workflow
	s.mu.Unlock()

	if hooks != nil {
		for _, req := range expired {
			if err := hooks.OnExpired(req); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].ID < expired[j].ID })
	return expired, nil
}

func (s *Service) RevokeExpired(at time.Time) ([]Request, error) {
	return s.ExpireDue(at)
}

func (s *Service) RevalidateActiveLines(users []UserState, at time.Time) RevalidationResult {
	if len(users) == 0 {
		return RevalidationResult{}
	}
	if at.IsZero() {
		at = s.now().UTC()
	}
	snapshots := map[int64]struct {
		active bool
		groups map[int64]bool
	}{}
	for _, user := range users {
		if user.UserID == 0 {
			continue
		}
		groups := map[int64]bool{}
		for _, groupID := range user.GroupIDs {
			if groupID != 0 {
				groups[groupID] = true
			}
		}
		snapshots[user.UserID] = struct {
			active bool
			groups map[int64]bool
		}{active: user.Active, groups: groups}
	}
	if len(snapshots) == 0 {
		return RevalidationResult{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	date := dateOnly(at)
	var deactivated []Line
	var affected []int64
	for requestID, req := range s.requests {
		if req.State != StateConfirmed {
			continue
		}
		changed := false
		for i := range req.Lines {
			line := req.Lines[i]
			user, ok := snapshots[line.DelegatorUserID]
			if !ok || !line.Active || line.State != StateConfirmed || dateOnly(line.DateTo).Before(date) {
				continue
			}
			if user.active && user.groups[line.GroupID] {
				continue
			}
			req.Lines[i].Active = false
			deactivated = append(deactivated, req.Lines[i])
			affected = append(affected, line.DelegatorUserID, line.DelegateUserID)
			changed = true
		}
		if changed {
			s.requests[requestID] = cloneRequest(req)
		}
	}
	s.invalidateLocked(affected)
	sort.Slice(deactivated, func(i, j int) bool { return deactivated[i].ID < deactivated[j].ID })
	return RevalidationResult{
		Deactivated:     cloneLines(deactivated),
		AffectedUserIDs: mergeInt64s(affected, nil),
	}
}

func (s *Service) Request(id int64) (Request, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.requests[id]
	return cloneRequest(req), ok
}

func (s *Service) Requests() []Request {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Request, 0, len(s.requests))
	for _, req := range s.requests {
		out = append(out, cloneRequest(req))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) ActiveLines(userID int64, at time.Time) []Line {
	key := cacheKey{UserID: userID, Day: dateOnly(at).Format("2006-01-02")}

	s.mu.RLock()
	if entry, ok := s.cache[key]; ok && entry.Version == s.cacheVersion {
		lines := cloneLines(entry.Lines)
		s.mu.RUnlock()
		return lines
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.cache[key]; ok && entry.Version == s.cacheVersion {
		return cloneLines(entry.Lines)
	}
	lines := s.activeLinesLocked(userID, at)
	s.cache[key] = cacheEntry{Version: s.cacheVersion, Lines: cloneLines(lines)}
	return cloneLines(lines)
}

func (s *Service) DelegatedGroupIDs(userID int64, at time.Time) []int64 {
	lines := s.ActiveLines(userID, at)
	seen := map[int64]bool{}
	var groupIDs []int64
	for _, line := range lines {
		if !seen[line.GroupID] {
			seen[line.GroupID] = true
			groupIDs = append(groupIDs, line.GroupID)
		}
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	return groupIDs
}

func (s *Service) EffectiveGroupIDs(userID int64, directGroupIDs []int64, at time.Time) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range directGroupIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range s.DelegatedGroupIDs(userID, at) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) DelegatorUserIDs(delegateUserID int64, at time.Time) []int64 {
	lines := s.ActiveLines(delegateUserID, at)
	seen := map[int64]bool{}
	var out []int64
	for _, line := range lines {
		if !seen[line.DelegatorUserID] {
			seen[line.DelegatorUserID] = true
			out = append(out, line.DelegatorUserID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) DelegatedUserIDs(delegatorUserID int64, at time.Time) []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[int64]bool{}
	var out []int64
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if line.DelegatorUserID == delegatorUserID && s.lineVisibleLocked(line) && lineActiveAt(line, at) && !seen[line.DelegateUserID] {
				seen[line.DelegateUserID] = true
				out = append(out, line.DelegateUserID)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) DelegatedApprovalUserIDs(delegatorUserIDs []int64, approvalGroupIDs []int64, departmentIDs []int64, at time.Time) []int64 {
	delegators := int64Set(delegatorUserIDs)
	approvalGroups := int64Set(approvalGroupIDs)
	if len(delegators) == 0 || len(approvalGroups) == 0 {
		return nil
	}
	departments := int64Set(departmentIDs)
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[int64]bool{}
	var out []int64
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if !delegators[line.DelegatorUserID] || !s.lineVisibleLocked(line) || !lineActiveAt(line, at) || line.DelegateUserID == 0 {
				continue
			}
			if !approvalGroups[line.GroupID] || delegators[line.DelegateUserID] {
				continue
			}
			if len(departments) > 0 && len(line.DepartmentIDs) > 0 && !containsAnyInt64(departments, line.DepartmentIDs) {
				continue
			}
			if seen[line.DelegateUserID] {
				continue
			}
			seen[line.DelegateUserID] = true
			out = append(out, line.DelegateUserID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) ActiveApprovalDelegationID(delegateUserID int64, delegatorUserIDs []int64, approvalGroupIDs []int64, departmentIDs []int64, at time.Time) int64 {
	if delegateUserID == 0 {
		return 0
	}
	delegators := int64Set(delegatorUserIDs)
	approvalGroups := int64Set(approvalGroupIDs)
	if len(delegators) == 0 || len(approvalGroups) == 0 {
		return 0
	}
	departments := int64Set(departmentIDs)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var requestIDs []int64
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if line.DelegateUserID != delegateUserID || !delegators[line.DelegatorUserID] || !s.lineVisibleLocked(line) || !lineActiveAt(line, at) {
				continue
			}
			if !approvalGroups[line.GroupID] || delegators[line.DelegateUserID] {
				continue
			}
			if len(departments) > 0 && len(line.DepartmentIDs) > 0 && !containsAnyInt64(departments, line.DepartmentIDs) {
				continue
			}
			if line.RequestID != 0 {
				requestIDs = append(requestIDs, line.RequestID)
			}
		}
	}
	if len(requestIDs) == 0 {
		return 0
	}
	sort.Slice(requestIDs, func(i, j int) bool { return requestIDs[i] < requestIDs[j] })
	return requestIDs[0]
}

func (s *Service) DelegatedGroupIDsForDelegator(delegatorUserID int64, at time.Time) []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[int64]bool{}
	var out []int64
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if line.DelegatorUserID == delegatorUserID && s.lineVisibleLocked(line) && lineActiveAt(line, at) && !seen[line.GroupID] {
				seen[line.GroupID] = true
				out = append(out, line.GroupID)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Service) DelegatedUserGroupIDsForDelegator(delegatorUserID int64, at time.Time) map[int64][]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[int64][]int64{}
	seen := map[int64]map[int64]bool{}
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if line.DelegatorUserID != delegatorUserID || !s.lineVisibleLocked(line) || !lineActiveAt(line, at) || line.DelegateUserID == 0 || line.GroupID == 0 {
				continue
			}
			if seen[line.DelegateUserID] == nil {
				seen[line.DelegateUserID] = map[int64]bool{}
			}
			if seen[line.DelegateUserID][line.GroupID] {
				continue
			}
			seen[line.DelegateUserID][line.GroupID] = true
			out[line.DelegateUserID] = append(out[line.DelegateUserID], line.GroupID)
		}
	}
	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool { return out[userID][i] < out[userID][j] })
	}
	return cloneUserGroupMap(out)
}

func (s *Service) BuildAccessContext(userID int64, directGroupIDs []int64, at time.Time) AccessContext {
	lines := s.ActiveLines(userID, at)
	delegatedGroups := lineGroupIDs(lines)
	return AccessContext{
		UserID:            userID,
		DelegatorUserIDs:  lineDelegatorUserIDs(lines),
		DelegatedGroupIDs: delegatedGroups,
		EffectiveGroupIDs: mergeInt64s(directGroupIDs, delegatedGroups),
		Lines:             lines,
		At:                at.UTC(),
	}
}

func (s *Service) CanAccess(userID int64, directGroupIDs []int64, modelName string, operation string, at time.Time) (bool, error) {
	if s.access == nil {
		return false, nil
	}
	return s.access.CanAccess(s.BuildAccessContext(userID, directGroupIDs, at), modelName, operation)
}

func (s *Service) RuleDomains(userID int64, directGroupIDs []int64, modelName string, operation string, at time.Time) ([]RuleDomain, error) {
	if s.access == nil {
		return nil, nil
	}
	domains, err := s.access.RuleDomains(s.BuildAccessContext(userID, directGroupIDs, at), modelName, operation)
	if err != nil {
		return nil, err
	}
	return cloneRuleDomains(domains), nil
}

func (s *Service) VisibleMenuIDs(userID int64, directGroupIDs []int64, debug bool, at time.Time) ([]int64, error) {
	if s.menus == nil {
		return nil, nil
	}
	ids, err := s.menus.VisibleMenuIDs(s.BuildAccessContext(userID, directGroupIDs, at), debug)
	if err != nil {
		return nil, err
	}
	return cloneSortedUnique(ids), nil
}

func (s *Service) ExpandMailCC(ctx MailContext) ([]string, error) {
	ctx.InitialCC = uniqueStrings(ctx.InitialCC)
	ctx.At = ctx.At.UTC()
	if ctx.At.IsZero() {
		ctx.At = s.now().UTC()
	}
	ctx.DelegatedUsers = s.DelegatedUserIDs(ctx.UserID, ctx.At)
	ctx.DelegatedGroupIDs = s.DelegatedGroupIDsForDelegator(ctx.UserID, ctx.At)
	ctx.DelegatedUserGroupID = s.DelegatedUserGroupIDsForDelegator(ctx.UserID, ctx.At)
	if s.mail == nil {
		return ctx.InitialCC, nil
	}
	cc, err := s.mail.ExpandCC(ctx)
	if err != nil {
		return nil, err
	}
	return uniqueStrings(append(ctx.InitialCC, cc...)), nil
}

func (s *Service) CacheVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cacheVersion
}

func (s *Service) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = map[cacheKey]cacheEntry{}
	s.cacheVersion++
}

func DefaultMailTemplates() []MailTemplateMetadata {
	return []MailTemplateMetadata{
		{
			ID:      1,
			XMLID:   "oi_delegation.mail_template_delegation_assigned",
			Name:    "Delegation Assigned",
			Subject: "Delegation assigned",
			Purpose: "notify delegates when a delegation request is confirmed",
			Active:  true,
		},
		{
			ID:      2,
			XMLID:   "oi_delegation.mail_template_delegation_revoked",
			Name:    "Delegation Revoked",
			Subject: "Delegation revoked",
			Purpose: "notify delegates when a delegation request is revoked",
			Active:  true,
		},
		{
			ID:      3,
			XMLID:   "oi_delegation.mail_template_delegation_expired",
			Name:    "Delegation Expired",
			Subject: "Delegation expired",
			Purpose: "notify delegates when a delegation request expires",
			Active:  true,
		},
	}
}

func (s *Service) buildRequest(input RequestInput, now time.Time) (Request, error) {
	req := Request{
		DateFrom:             dateOnly(input.DateFrom),
		DateTo:               dateOnly(input.DateTo),
		DelegatorEmployeeID:  input.DelegatorEmployeeID,
		DelegatorUserID:      input.DelegatorUserID,
		OneEmployee:          input.OneEmployee,
		DelegateToEmployeeID: input.DelegateToEmployeeID,
		DelegateToUserID:     input.DelegateToUserID,
		DepartmentIDs:        cloneInt64s(input.DepartmentIDs),
		SourceModel:          input.SourceModel,
		SourceRecordID:       input.SourceRecordID,
		Metadata:             cloneStringMap(input.Metadata),
	}
	if req.DateFrom.IsZero() {
		req.DateFrom = dateOnly(now)
	}
	if req.DateTo.IsZero() || req.DateTo.Before(req.DateFrom) {
		return Request{}, ErrInvalidDateRange
	}
	if req.DelegatorUserID == 0 {
		return Request{}, ErrDelegatorRequired
	}
	if len(input.Lines) == 0 {
		return Request{}, ErrLineRequired
	}
	for _, lineInput := range input.Lines {
		line := Line{
			GroupID:             lineInput.GroupID,
			DelegateEmployeeID:  lineInput.DelegateEmployeeID,
			DelegateUserID:      lineInput.DelegateUserID,
			DelegatorEmployeeID: req.DelegatorEmployeeID,
			DelegatorUserID:     req.DelegatorUserID,
			DepartmentIDs:       cloneInt64s(req.DepartmentIDs),
			DateFrom:            req.DateFrom,
			DateTo:              req.DateTo,
		}
		if req.OneEmployee {
			if line.DelegateEmployeeID == 0 {
				line.DelegateEmployeeID = req.DelegateToEmployeeID
			}
			if line.DelegateUserID == 0 {
				line.DelegateUserID = req.DelegateToUserID
			}
		}
		if line.DelegateUserID == 0 {
			return Request{}, ErrDelegateRequired
		}
		if line.DelegateUserID == req.DelegatorUserID {
			return Request{}, ErrSelfDelegation
		}
		req.Lines = append(req.Lines, line)
	}
	return req, nil
}

func (s *Service) validateGroupsLocked(req Request) error {
	for _, line := range req.Lines {
		config, ok := s.groups[line.GroupID]
		if !ok || !config.AllowDelegation {
			return fmt.Errorf("%w: %d", ErrGroupNotDelegable, line.GroupID)
		}
	}
	return nil
}

func (s *Service) validateReadyLocked(req Request, enforceDates bool) error {
	if len(req.Lines) == 0 {
		return ErrLineRequired
	}
	if !req.DateTo.IsZero() && req.DateTo.Before(req.DateFrom) {
		return ErrInvalidDateRange
	}
	if enforceDates && req.DateFrom.Before(dateOnly(s.now())) {
		return ErrPastStartDate
	}
	if err := s.validateGroupsLocked(req); err != nil {
		return err
	}
	return s.validateOverlapLocked(req)
}

func (s *Service) validateOverlapLocked(req Request) error {
	for _, candidate := range req.Lines {
		config := s.groups[candidate.GroupID]
		if config.AllowMultipleDelegation {
			continue
		}
		for _, existing := range s.requests {
			if existing.ID == req.ID || existing.State != StateConfirmed {
				continue
			}
			for _, line := range existing.Lines {
				if !line.Active || line.State != StateConfirmed {
					continue
				}
				if line.GroupID != candidate.GroupID || line.DelegatorUserID != candidate.DelegatorUserID {
					continue
				}
				if rangesOverlap(candidate.DateFrom, candidate.DateTo, line.DateFrom, line.DateTo) {
					return fmt.Errorf("%w: group %d", ErrOverlappingDelegation, candidate.GroupID)
				}
			}
		}
	}
	return nil
}

func (s *Service) activeLinesLocked(userID int64, at time.Time) []Line {
	var lines []Line
	for _, req := range s.requests {
		for _, line := range req.Lines {
			if line.DelegateUserID == userID && s.lineVisibleLocked(line) && lineActiveAt(line, at) {
				lines = append(lines, line)
			}
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].ID < lines[j].ID })
	return cloneLines(lines)
}

func (s *Service) invalidateLocked(userIDs []int64) {
	userSet := map[int64]bool{}
	for _, id := range userIDs {
		if id != 0 {
			userSet[id] = true
		}
	}
	if len(userSet) == 0 {
		return
	}
	for key := range s.cache {
		if userSet[key.UserID] {
			delete(s.cache, key)
		}
	}
	s.cacheVersion++
	if s.cacheInvalidator != nil {
		ids := make([]int64, 0, len(userSet))
		for id := range userSet {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		s.cacheInvalidator.InvalidateDelegationCache(ids)
	}
}

func lineActiveAt(line Line, at time.Time) bool {
	date := dateOnly(at)
	return line.Active &&
		line.State == StateConfirmed &&
		!date.Before(dateOnly(line.DateFrom)) &&
		!date.After(dateOnly(line.DateTo))
}

func (s *Service) lineVisibleLocked(line Line) bool {
	if !s.restrictedAccess {
		return true
	}
	config, ok := s.groups[line.GroupID]
	return ok && config.RestrictedAccess
}

func affectedUsers(req Request) []int64 {
	var ids []int64
	if req.DelegatorUserID != 0 {
		ids = append(ids, req.DelegatorUserID)
	}
	for _, line := range req.Lines {
		ids = append(ids, line.DelegateUserID)
	}
	return ids
}

func rangesOverlap(aFrom, aTo, bFrom, bTo time.Time) bool {
	aFrom, aTo, bFrom, bTo = dateOnly(aFrom), dateOnly(aTo), dateOnly(bFrom), dateOnly(bTo)
	return !aTo.Before(bFrom) && !bTo.Before(aFrom)
}

func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func cloneRequest(req Request) Request {
	req.DateFrom = dateOnly(req.DateFrom)
	req.DateTo = dateOnly(req.DateTo)
	req.DepartmentIDs = cloneInt64s(req.DepartmentIDs)
	req.Lines = cloneLines(req.Lines)
	req.Metadata = cloneStringMap(req.Metadata)
	return req
}

func cloneLines(lines []Line) []Line {
	if lines == nil {
		return nil
	}
	out := make([]Line, len(lines))
	copy(out, lines)
	for i := range out {
		out[i].DepartmentIDs = cloneInt64s(out[i].DepartmentIDs)
	}
	return out
}

func cloneInt64s(ids []int64) []int64 {
	if ids == nil {
		return nil
	}
	out := make([]int64, len(ids))
	copy(out, ids)
	return out
}

func cloneSortedUnique(ids []int64) []int64 {
	out := mergeInt64s(ids, nil)
	return out
}

func mergeInt64s(a, b []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range append(cloneInt64s(a), b...) {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneUserGroupMap(in map[int64][]int64) map[int64][]int64 {
	if len(in) == 0 {
		return nil
	}
	out := map[int64][]int64{}
	for userID, groupIDs := range in {
		out[userID] = cloneInt64s(groupIDs)
	}
	return out
}

func cloneRuleDomains(domains []RuleDomain) []RuleDomain {
	if domains == nil {
		return nil
	}
	out := make([]RuleDomain, len(domains))
	copy(out, domains)
	for i := range out {
		out[i].DepartmentIDs = cloneInt64s(out[i].DepartmentIDs)
	}
	return out
}

func int64Set(values []int64) map[int64]bool {
	out := map[int64]bool{}
	for _, value := range values {
		if value != 0 {
			out[value] = true
		}
	}
	return out
}

func containsAnyInt64(set map[int64]bool, values []int64) bool {
	for _, value := range values {
		if set[value] {
			return true
		}
	}
	return false
}

func lineGroupIDs(lines []Line) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, line := range lines {
		if !seen[line.GroupID] {
			seen[line.GroupID] = true
			out = append(out, line.GroupID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func lineDelegatorUserIDs(lines []Line) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, line := range lines {
		if !seen[line.DelegatorUserID] {
			seen[line.DelegatorUserID] = true
			out = append(out, line.DelegatorUserID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
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
