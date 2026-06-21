package impersonation

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrSessionNotFound     = errors.New("session not found")
	ErrUnauthorized        = errors.New("impersonation unauthorized")
	ErrSelfImpersonation   = errors.New("cannot impersonate same user")
	ErrTargetInactive      = errors.New("target user is inactive")
	ErrTargetSuperuser     = errors.New("target user is superuser")
	ErrPortalDisabled      = errors.New("portal impersonation disabled")
	ErrGroupMismatch       = errors.New("target user is not in selected group")
	ErrDebugRouteDisabled  = errors.New("debug impersonation route disabled")
	ErrNotImpersonating    = errors.New("session is not impersonating")
	ErrImmutableAuditEvent = errors.New("audit events are append-only")
)

type User struct {
	ID         int64
	Login      string
	Name       string
	Active     bool
	Superuser  bool
	Portal     bool
	CompanyID  int64
	CompanyIDs []int64
	GroupIDs   []int64
}

type Config struct {
	AdminGroupID           int64
	ImpersonatorGroupID    int64
	AllowInactiveGroupID   int64
	AllowSuperuserGroupID  int64
	DebugGroupID           int64
	DebugRouteEnabled      bool
	PortalSupport          bool
	SystemUserID           int64
	LoginAsRoute           string
	LoginBackRoute         string
	DebugRoute             string
	ImpersonationBannerKey string
}

func DefaultConfig() Config {
	return Config{
		AdminGroupID:           1,
		ImpersonatorGroupID:    2,
		AllowInactiveGroupID:   3,
		AllowSuperuserGroupID:  4,
		DebugGroupID:           5,
		PortalSupport:          true,
		SystemUserID:           1,
		LoginAsRoute:           "/web/login_as/<user_id>",
		LoginBackRoute:         "/web/login_back",
		DebugRoute:             "/web/become/debug",
		ImpersonationBannerKey: "login_as_banner",
	}
}

type SwitchOptions struct {
	GroupID        int64
	ReturnTo       string
	AllowInactive  bool
	AllowSuperuser bool
	Reason         string
}

type Session struct {
	ID             string
	UserID         int64
	OriginalUserID int64
	Impersonating  bool
	GroupID        int64
	ReturnTo       string
	StartedAt      time.Time
	UpdatedAt      time.Time
}

type WizardAction struct {
	Type    string
	Name    string
	Route   string
	Context map[string]any
}

type RouteDescriptor struct {
	Name            string
	Path            string
	Method          string
	Auth            string
	Enabled         bool
	RequiresSetting string
}

type SessionInfo struct {
	UserID          int64
	OriginalUserID  int64
	Impersonating   bool
	Banner          string
	LoginBackRoute  string
	ReturnTo        string
	Context         map[string]any
	CurrentUserName string
	OriginalName    string
}

type AuditEvent struct {
	ID              int64
	At              time.Time
	Action          string
	ActorID         int64
	EffectiveUserID int64
	TargetUserID    int64
	SessionID       string
	Model           string
	RecordID        int64
	IPAddress       string
	UserAgent       string
	Details         map[string]string
}

type Service struct {
	mu       sync.RWMutex
	config   Config
	users    map[int64]User
	sessions map[string]Session
	audit    []AuditEvent
	nextID   int64
	now      func() time.Time
}

type Option func(*Service)

func WithConfig(config Config) Option {
	return func(s *Service) {
		s.config = normalizeConfig(config)
	}
}

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(options ...Option) *Service {
	s := &Service{
		config:   DefaultConfig(),
		users:    map[int64]User{},
		sessions: map[string]Session{},
		nextID:   1,
		now:      func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(s)
	}
	s.config = normalizeConfig(s.config)
	return s
}

func (s *Service) SetUser(user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user.CompanyIDs = cloneInt64s(user.CompanyIDs)
	user.GroupIDs = cloneInt64s(user.GroupIDs)
	s.users[user.ID] = user
}

func (s *Service) User(id int64) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	return cloneUser(user), ok
}

func (s *Service) CanImpersonate(actorID int64, targetID int64, options SwitchOptions) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	actor, target, err := s.lookupPairLocked(actorID, targetID)
	if err != nil {
		return err
	}
	return s.canImpersonateLocked(actor, target, options)
}

func (s *Service) Start(sessionID string, actorID int64, targetID int64, options SwitchOptions) (Session, error) {
	if strings.TrimSpace(sessionID) == "" {
		return Session{}, ErrSessionNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	actor, target, err := s.lookupPairLocked(actorID, targetID)
	if err != nil {
		return Session{}, err
	}
	if err := s.canImpersonateLocked(actor, target, options); err != nil {
		s.appendAuditLocked("login_as.denied", actorID, actorID, targetID, sessionID, "", 0, map[string]string{"reason": err.Error()})
		return Session{}, err
	}
	now := s.now().UTC()
	originalUserID := actorID
	if existing, ok := s.sessions[sessionID]; ok && existing.Impersonating && existing.OriginalUserID != 0 {
		originalUserID = existing.OriginalUserID
	}
	session := Session{
		ID:             sessionID,
		UserID:         targetID,
		OriginalUserID: originalUserID,
		Impersonating:  true,
		GroupID:        options.GroupID,
		ReturnTo:       options.ReturnTo,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	s.sessions[sessionID] = session
	s.appendAuditLocked("login_as.start", originalUserID, targetID, targetID, sessionID, "", 0, map[string]string{"reason": options.Reason})
	return cloneSession(session), nil
}

func (s *Service) LoginBack(sessionID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if !session.Impersonating || session.OriginalUserID == 0 {
		return Session{}, ErrNotImpersonating
	}
	previousEffective := session.UserID
	now := s.now().UTC()
	session.UserID = session.OriginalUserID
	session.OriginalUserID = 0
	session.Impersonating = false
	session.GroupID = 0
	session.UpdatedAt = now
	s.sessions[sessionID] = session
	s.appendAuditLocked("login_as.back", session.UserID, previousEffective, previousEffective, sessionID, "", 0, nil)
	return cloneSession(session), nil
}

func (s *Service) SwitchToSystem(sessionID string, actorID int64, options SwitchOptions) (Session, error) {
	s.mu.RLock()
	config := s.config
	actor, ok := s.users[actorID]
	s.mu.RUnlock()
	if !ok {
		return Session{}, ErrUserNotFound
	}
	if !config.DebugRouteEnabled {
		s.mu.Lock()
		s.appendAuditLocked("login_as.debug_denied", actorID, actorID, config.SystemUserID, sessionID, "", 0, map[string]string{"reason": ErrDebugRouteDisabled.Error()})
		s.mu.Unlock()
		return Session{}, ErrDebugRouteDisabled
	}
	if !hasGroup(actor, config.DebugGroupID) && !hasGroup(actor, config.AdminGroupID) && !actor.Superuser {
		return Session{}, ErrUnauthorized
	}
	options.AllowSuperuser = true
	return s.Start(sessionID, actorID, config.SystemUserID, options)
}

func (s *Service) WizardAction(actorID int64, targetID int64, options SwitchOptions) (WizardAction, error) {
	if err := s.CanImpersonate(actorID, targetID, options); err != nil {
		return WizardAction{}, err
	}
	route := strings.ReplaceAll(s.config.LoginAsRoute, "<user_id>", strconv.FormatInt(targetID, 10))
	return WizardAction{
		Type:  "ir.actions.act_url",
		Name:  "Login as",
		Route: route,
		Context: map[string]any{
			"target_user_id": targetID,
			"group_id":       options.GroupID,
			"return_to":      options.ReturnTo,
		},
	}, nil
}

func (s *Service) Routes() []RouteDescriptor {
	config := s.config
	return []RouteDescriptor{
		{Name: "login_as", Path: config.LoginAsRoute, Method: "GET", Auth: "user", Enabled: true},
		{Name: "login_back", Path: config.LoginBackRoute, Method: "GET", Auth: "user", Enabled: true},
		{Name: "login_as_debug", Path: config.DebugRoute, Method: "GET", Auth: "user", Enabled: config.DebugRouteEnabled, RequiresSetting: "login_as.debug_route_enabled"},
	}
}

func (s *Service) IsSystemUser(userID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[userID]
	return ok && (user.Superuser || user.ID == s.config.SystemUserID)
}

func (s *Service) Session(sessionID string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	return cloneSession(session), ok
}

func (s *Service) SessionInfo(sessionID string) (SessionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return SessionInfo{}, ErrSessionNotFound
	}
	current := s.users[session.UserID]
	original := s.users[session.OriginalUserID]
	info := SessionInfo{
		UserID:          session.UserID,
		OriginalUserID:  session.OriginalUserID,
		Impersonating:   session.Impersonating,
		LoginBackRoute:  s.config.LoginBackRoute,
		ReturnTo:        session.ReturnTo,
		CurrentUserName: current.Name,
		OriginalName:    original.Name,
		Context: map[string]any{
			"login_as":                session.Impersonating,
			"login_as_user_id":        session.UserID,
			"login_as_original_uid":   session.OriginalUserID,
			"login_as_return_to":      session.ReturnTo,
			"login_as_back_route":     s.config.LoginBackRoute,
			"login_as_effective_name": current.Name,
		},
	}
	if session.Impersonating {
		info.Banner = fmt.Sprintf("Impersonating %s", displayName(current))
		info.Context[s.config.ImpersonationBannerKey] = info.Banner
	}
	return info, nil
}

func (s *Service) RecordAction(sessionID string, action string, model string, recordID int64, details map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	actorID := session.UserID
	if session.Impersonating && session.OriginalUserID != 0 {
		actorID = session.OriginalUserID
	}
	s.appendAuditLocked(action, actorID, session.UserID, 0, sessionID, model, recordID, details)
	return nil
}

func (s *Service) AuditLog() []AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditEvent, len(s.audit))
	for i, event := range s.audit {
		out[i] = cloneAudit(event)
	}
	return out
}

func (s *Service) ReplaceAuditEvent(AuditEvent) error {
	return ErrImmutableAuditEvent
}

func (s *Service) AllowedTargetsForGroup(actorID int64, groupID int64) ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	actor, ok := s.users[actorID]
	if !ok {
		return nil, ErrUserNotFound
	}
	if !s.authorizedActorLocked(actor) {
		return nil, ErrUnauthorized
	}
	var users []User
	for _, user := range s.users {
		if groupID != 0 && !hasGroup(user, groupID) {
			continue
		}
		if user.ID == actorID {
			continue
		}
		if !user.Active || user.Superuser {
			continue
		}
		if user.Portal && !s.config.PortalSupport {
			continue
		}
		users = append(users, cloneUser(user))
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	return users, nil
}

func (s *Service) lookupPairLocked(actorID int64, targetID int64) (User, User, error) {
	actor, ok := s.users[actorID]
	if !ok {
		return User{}, User{}, ErrUserNotFound
	}
	target, ok := s.users[targetID]
	if !ok {
		return User{}, User{}, ErrUserNotFound
	}
	return actor, target, nil
}

func (s *Service) canImpersonateLocked(actor User, target User, options SwitchOptions) error {
	if actor.ID == target.ID {
		return ErrSelfImpersonation
	}
	if !actor.Active {
		return ErrUnauthorized
	}
	if !s.authorizedActorLocked(actor) {
		return ErrUnauthorized
	}
	if options.GroupID != 0 && !hasGroup(target, options.GroupID) {
		return ErrGroupMismatch
	}
	if target.Portal && !s.config.PortalSupport {
		return ErrPortalDisabled
	}
	if !target.Active && !(options.AllowInactive && hasGroup(actor, s.config.AllowInactiveGroupID)) {
		return ErrTargetInactive
	}
	if (target.Superuser || target.ID == s.config.SystemUserID) && !(options.AllowSuperuser && hasGroup(actor, s.config.AllowSuperuserGroupID)) {
		return ErrTargetSuperuser
	}
	return nil
}

func (s *Service) authorizedActorLocked(actor User) bool {
	return actor.Superuser || hasGroup(actor, s.config.AdminGroupID) || hasGroup(actor, s.config.ImpersonatorGroupID)
}

func (s *Service) appendAuditLocked(action string, actorID int64, effectiveUserID int64, targetUserID int64, sessionID string, model string, recordID int64, details map[string]string) {
	event := AuditEvent{
		ID:              s.nextID,
		At:              s.now().UTC(),
		Action:          action,
		ActorID:         actorID,
		EffectiveUserID: effectiveUserID,
		TargetUserID:    targetUserID,
		SessionID:       sessionID,
		Model:           model,
		RecordID:        recordID,
		Details:         redact(cloneStringMap(details)),
	}
	s.nextID++
	s.audit = append(s.audit, event)
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.AdminGroupID == 0 {
		config.AdminGroupID = defaults.AdminGroupID
	}
	if config.ImpersonatorGroupID == 0 {
		config.ImpersonatorGroupID = defaults.ImpersonatorGroupID
	}
	if config.AllowInactiveGroupID == 0 {
		config.AllowInactiveGroupID = defaults.AllowInactiveGroupID
	}
	if config.AllowSuperuserGroupID == 0 {
		config.AllowSuperuserGroupID = defaults.AllowSuperuserGroupID
	}
	if config.DebugGroupID == 0 {
		config.DebugGroupID = defaults.DebugGroupID
	}
	if config.SystemUserID == 0 {
		config.SystemUserID = defaults.SystemUserID
	}
	if config.LoginAsRoute == "" {
		config.LoginAsRoute = defaults.LoginAsRoute
	}
	if config.LoginBackRoute == "" {
		config.LoginBackRoute = defaults.LoginBackRoute
	}
	if config.DebugRoute == "" {
		config.DebugRoute = defaults.DebugRoute
	}
	if config.ImpersonationBannerKey == "" {
		config.ImpersonationBannerKey = defaults.ImpersonationBannerKey
	}
	return config
}

func hasGroup(user User, groupID int64) bool {
	if groupID == 0 {
		return false
	}
	for _, id := range user.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

func cloneUser(user User) User {
	user.CompanyIDs = cloneInt64s(user.CompanyIDs)
	user.GroupIDs = cloneInt64s(user.GroupIDs)
	return user
}

func cloneSession(session Session) Session {
	return session
}

func cloneAudit(event AuditEvent) AuditEvent {
	event.Details = cloneStringMap(event.Details)
	return event
}

func cloneInt64s(ids []int64) []int64 {
	if ids == nil {
		return nil
	}
	out := make([]int64, len(ids))
	copy(out, ids)
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

func redact(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	for key := range details {
		switch strings.ToLower(key) {
		case "password", "token", "api_key", "secret":
			details[key] = "[redacted]"
		}
	}
	return details
}

func displayName(user User) string {
	if strings.TrimSpace(user.Name) != "" {
		return user.Name
	}
	return user.Login
}
