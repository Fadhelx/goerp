package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"gorp/internal/actions"
	"gorp/internal/domain"
)

var (
	ErrSettingsNotFound  = errors.New("workflow settings not found")
	ErrButtonNotFound    = errors.New("workflow button not found")
	ErrButtonHidden      = errors.New("workflow button not visible")
	ErrCommentRequired   = errors.New("workflow comment required")
	ErrTargetUserMissing = errors.New("workflow target user missing")
	ErrMailerMissing     = errors.New("workflow mailer missing")
	ErrActionMissing     = errors.New("workflow action registry missing")
	ErrMethodMissing     = errors.New("workflow method missing")
)

type ButtonAction string

const (
	ActionApprove            ButtonAction = "approve"
	ActionDraft              ButtonAction = "draft"
	ActionReject             ButtonAction = "reject"
	ActionReturn             ButtonAction = "return"
	ActionCancel             ButtonAction = "cancel"
	ActionCancelWorkflow     ButtonAction = "cancel_workflow"
	ActionTransfer           ButtonAction = "transfer"
	ActionForward            ButtonAction = "forward"
	ActionEmail              ButtonAction = "email"
	ActionServerAction       ButtonAction = "action"
	ActionLegacyServerAction ButtonAction = "server_action"
	ActionMethod             ButtonAction = "method"
	ActionVote               ButtonAction = "vote"
)

type AutomationTrigger string

const (
	TriggerOnSubmit            AutomationTrigger = "on_submit"
	TriggerOnEnterApproval     AutomationTrigger = "on_enter_approval"
	TriggerOnApprove           AutomationTrigger = "on_approve"
	TriggerOnApproval          AutomationTrigger = "on_approval"
	TriggerOnReject            AutomationTrigger = "on_reject"
	TriggerOnReturn            AutomationTrigger = "on_return"
	TriggerOnCancel            AutomationTrigger = "on_cancel"
	TriggerOnDraft             AutomationTrigger = "on_draft"
	TriggerOnForward           AutomationTrigger = "on_forward"
	TriggerOnTransfer          AutomationTrigger = "on_transfer"
	TriggerOnStateUpdated      AutomationTrigger = "on_state_updated"
	TriggerOnCreate            AutomationTrigger = "on_create"
	TriggerOnCommitteeApproval AutomationTrigger = "on_committee_approval"
	TriggerStateChange         AutomationTrigger = "state_change"
)

type User struct {
	ID       int64
	GroupIDs []int64
}

type Settings struct {
	ID             int64
	Name           string
	Model          string
	Active         bool
	StateField     string
	DraftState     string
	ApprovedState  string
	RejectedState  string
	CancelledState string
	CompanyID      int64
}

type State struct {
	ID                  int64
	SettingsID          int64
	Value               string
	Name                string
	Sequence            int
	GroupIDs            []int64
	TagIDs              []int64
	Kind                string
	ActivityDelayDays   int
	ActivitySummary     string
	Condition           domain.Node
	RequireAllApprovers bool
}

type ApprovalConfig struct {
	ID                      int64
	SettingsID              int64
	State                   string
	Name                    string
	Active                  bool
	Sequence                int
	GroupIDs                []int64
	UserIDs                 []int64
	UserPythonCode          string
	Condition               domain.Node
	AutoApprove             bool
	Committee               bool
	CommitteeLimit          int
	CommitteeVotePercentage float64
	IsVoting                bool
	ScheduleActivity        bool
	ScheduleActivityField   string
	ScheduleActivityDays    int
}

type Button struct {
	ID                int64
	SettingsID        int64
	StateValue        string
	Name              string
	Action            ButtonAction
	NextState         string
	ReturnState       string
	TransferState     string
	MethodName        string
	ServerActionID    int64
	EmailTemplateID   int64
	EmailWizardFormID int64
	EmailNextAction   ButtonAction
	GroupIDs          []int64
	VisibleTo         string
	VisibleDomain     domain.Node
	CommentRequired   bool
	ConfirmMessage    string
	ButtonClass       string
	Sequence          int
	VoteThreshold     int
	VotingType        string
	RunAsSuperuser    bool
}

type Record struct {
	Model                string
	ID                   int64
	State                string
	OwnerUserID          int64
	CompanyID            int64
	ApprovalStateID      int64
	WorkflowNodeID       int64
	LastStateUpdate      time.Time
	ApprovalUserIDs      []int64
	DoneUserIDs          []int64
	ForwardedToUserIDs   []int64
	DelegationID         int64
	DelegationEmployeeID int64
	Values               map[string]any
}

func (r Record) Row(stateField string) map[string]any {
	row := cloneMap(r.Values)
	if row == nil {
		row = map[string]any{}
	}
	row["id"] = r.ID
	row["model"] = r.Model
	row["owner_user_id"] = r.OwnerUserID
	row["company_id"] = r.CompanyID
	field := stateField
	if field == "" {
		field = "state"
	}
	row[field] = r.State
	if _, ok := row["state"]; !ok {
		row["state"] = r.State
	}
	return row
}

type Log struct {
	Model                string
	RecordID             int64
	UserID               int64
	ButtonID             int64
	Action               ButtonAction
	Comment              string
	OldState             string
	NewState             string
	Duration             time.Duration
	BulkApproval         bool
	DelegationID         int64
	DelegationEmployeeID int64
	At                   time.Time
}

type Forward struct {
	Model           string
	RecordID        int64
	StateValue      string
	ApprovalStateID int64
	WorkflowNodeID  int64
	UserID          int64
	ForwarderUserID int64
	Active          bool
	At              time.Time
}

type Vote struct {
	Model       string
	RecordID    int64
	StateValue  string
	UserID      int64
	ButtonID    int64
	VoteType    string
	Approved    bool
	Comment     string
	ButtonClass string
	At          time.Time
}

type CancellationRecord struct {
	Model       string
	RecordID    int64
	RequesterID int64
	Reason      string
	At          time.Time
}

type Automation struct {
	ID              int64
	SettingsID      int64
	Name            string
	Model           string
	Sequence        int
	Active          bool
	Trigger         AutomationTrigger
	FromStates      []string
	ToStates        []string
	Filter          domain.Node
	Code            string
	ServerActionIDs []int64
	TemplateIDs     []int64
}

type Escalation struct {
	ID           int64
	SettingsID   int64
	Name         string
	StateValue   string
	After        time.Duration
	ToUserID     int64
	ServerAction int64
	Active       bool
}

type Input struct {
	Comment        string
	TargetUserID   int64
	Values         map[string]any
	BulkApproval   bool
	MailComposed   bool
	RunAsSuperuser bool
}

type TransitionResult struct {
	Button        Button
	OldState      string
	NewState      string
	Log           Log
	ActionResults []actions.Result
}

type ButtonDescriptor struct {
	ID             int64
	Name           string
	Action         ButtonAction
	ButtonClass    string
	ConfirmMessage string
	Sequence       int
}

type LogDescriptor struct {
	UserID   int64
	Action   ButtonAction
	OldState string
	NewState string
	Comment  string
	At       time.Time
}

type ViewMetadata struct {
	Model          string
	RecordID       int64
	State          string
	VisibleButtons []ButtonDescriptor
	Logs           []LogDescriptor
}

type EmailRequest struct {
	TemplateID int64
	Model      string
	RecordID   int64
	UserID     int64
	Values     map[string]any
}

type Mailer interface {
	SendWorkflowEmail(context.Context, EmailRequest) error
}

type MethodFunc func(context.Context, *Record, Button, Input) error

type AutomationCodeRequest struct {
	Automation Automation
	Code       string
	Exec       actions.ExecutionContext
}

type AutomationCodeRunner func(context.Context, AutomationCodeRequest) (actions.Result, bool, error)

type Engine struct {
	Settings      map[int64]Settings
	States        map[int64]State
	Configs       map[int64]ApprovalConfig
	Buttons       map[int64]Button
	Automations   map[int64]Automation
	Escalations   map[int64]Escalation
	Logs          []Log
	Forwards      []Forward
	Votes         []Vote
	Cancellations []CancellationRecord
	Actions       *actions.Registry
	Mailer        Mailer
	Methods       map[string]MethodFunc
	CodeRunner    AutomationCodeRunner
	now           func() time.Time
	nextID        int64
}

func NewEngine() *Engine {
	return &Engine{
		Settings:    map[int64]Settings{},
		States:      map[int64]State{},
		Configs:     map[int64]ApprovalConfig{},
		Buttons:     map[int64]Button{},
		Automations: map[int64]Automation{},
		Escalations: map[int64]Escalation{},
		Methods:     map[string]MethodFunc{},
		now:         time.Now,
		nextID:      1,
	}
}

func (e *Engine) SetNow(now func() time.Time) {
	if now == nil {
		e.now = time.Now
		return
	}
	e.now = now
}

func (e *Engine) RegisterMethod(name string, fn MethodFunc) error {
	if name == "" {
		return fmt.Errorf("workflow method requires name")
	}
	if fn == nil {
		return fmt.Errorf("workflow method %s requires function", name)
	}
	e.Methods[name] = fn
	return nil
}

func (e *Engine) AddSettings(settings Settings) int64 {
	if settings.ID == 0 {
		settings.ID = e.next()
	}
	if settings.StateField == "" {
		settings.StateField = "state"
	}
	if settings.DraftState == "" {
		settings.DraftState = "draft"
	}
	if settings.ApprovedState == "" {
		settings.ApprovedState = "approved"
	}
	if settings.RejectedState == "" {
		settings.RejectedState = "rejected"
	}
	if settings.CancelledState == "" {
		settings.CancelledState = "cancelled"
	}
	e.Settings[settings.ID] = settings
	return settings.ID
}

func (e *Engine) AddState(state State) int64 {
	if state.ID == 0 {
		state.ID = e.next()
	}
	e.States[state.ID] = state
	return state.ID
}

func (e *Engine) AddConfig(config ApprovalConfig) int64 {
	if config.ID == 0 {
		config.ID = e.next()
	}
	if config.Sequence == 0 {
		config.Sequence = 10
	}
	config.Active = true
	e.Configs[config.ID] = config
	return config.ID
}

func (e *Engine) AddButton(button Button) int64 {
	if button.ID == 0 {
		button.ID = e.next()
	}
	e.Buttons[button.ID] = button
	return button.ID
}

func (e *Engine) AddAutomation(automation Automation) int64 {
	if automation.ID == 0 {
		automation.ID = e.next()
	}
	e.Automations[automation.ID] = automation
	return automation.ID
}

func (e *Engine) AddEscalation(escalation Escalation) int64 {
	if escalation.ID == 0 {
		escalation.ID = e.next()
	}
	e.Escalations[escalation.ID] = escalation
	return escalation.ID
}

func (e *Engine) SettingsForModel(modelName string) (Settings, bool) {
	var selected Settings
	for _, settings := range e.Settings {
		if settings.Model != modelName || !settings.Active {
			continue
		}
		if selected.ID == 0 || settings.ID < selected.ID {
			selected = settings
		}
	}
	return selected, selected.ID != 0
}

func (e *Engine) NextApprovalConfig(record Record) (ApprovalConfig, bool, error) {
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return ApprovalConfig{}, false, ErrSettingsNotFound
	}
	return e.nextApprovalConfig(settings, record)
}

func (e *Engine) NextApprovalState(user User, record Record) (string, bool, error) {
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return "", false, ErrSettingsNotFound
	}
	state, config, err := e.resolveApproveState(user, settings, record, Button{})
	return state, config.ID != 0, err
}

func (e *Engine) VisibleButtons(user User, record Record) ([]Button, error) {
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return nil, ErrSettingsNotFound
	}
	buttons := make([]Button, 0, len(e.Buttons))
	for _, button := range e.Buttons {
		if button.SettingsID != settings.ID {
			continue
		}
		if button.StateValue != "" && button.StateValue != record.State {
			continue
		}
		visible, err := e.buttonVisible(user, record, settings, button)
		if err != nil {
			return nil, err
		}
		if visible {
			buttons = append(buttons, button)
		}
	}
	sort.Slice(buttons, func(i, j int) bool {
		if buttons[i].Sequence == buttons[j].Sequence {
			return buttons[i].ID < buttons[j].ID
		}
		return buttons[i].Sequence < buttons[j].Sequence
	})
	return buttons, nil
}

func (e *Engine) RunButton(ctx context.Context, user User, record *Record, buttonID int64, input Input) (TransitionResult, error) {
	if record == nil {
		return TransitionResult{}, fmt.Errorf("workflow record is nil")
	}
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return TransitionResult{}, ErrSettingsNotFound
	}
	button, ok := e.Buttons[buttonID]
	if !ok || button.SettingsID != settings.ID {
		return TransitionResult{}, ErrButtonNotFound
	}
	visible, err := e.buttonVisible(user, *record, settings, button)
	if err != nil {
		return TransitionResult{}, err
	}
	if !visible {
		return TransitionResult{}, ErrButtonHidden
	}
	if button.CommentRequired && strings.TrimSpace(input.Comment) == "" {
		return TransitionResult{}, ErrCommentRequired
	}
	input.RunAsSuperuser = input.RunAsSuperuser || button.RunAsSuperuser

	oldState := record.State
	newState := oldState
	var triggers []AutomationTrigger
	var actionResults []actions.Result
runAction:
	switch button.Action {
	case ActionApprove:
		currentConfig, inApprovalState := e.configForState(settings.ID, oldState)
		triggerOnSubmit := !inApprovalState
		if currentConfig.Committee {
			result, decision, err := e.applyCommitteePartialApproval(ctx, user, record, settings, button, currentConfig, oldState, input)
			if err != nil || decision == committeeApprovalPartial {
				return result, err
			}
			if decision == committeeApprovalReject {
				newState = settings.RejectedState
				triggers = append(triggers, TriggerOnCommitteeApproval, TriggerOnReject)
				break
			}
		}
		nextState, nextConfig, err := e.resolveApproveState(user, settings, *record, button)
		if err != nil {
			return TransitionResult{}, err
		}
		newState = nextState
		if currentConfig.Committee {
			triggers = append(triggers, TriggerOnCommitteeApproval)
		}
		if triggerOnSubmit {
			triggers = append(triggers, TriggerOnSubmit)
		} else {
			triggers = append(triggers, TriggerOnApproval)
		}
		if nextConfig.ID != 0 {
			triggers = append(triggers, TriggerOnEnterApproval)
		} else {
			triggers = append(triggers, TriggerOnApprove)
		}
		appendUnique(&record.DoneUserIDs, user.ID)
	case ActionDraft:
		newState = fallback(button.NextState, settings.DraftState)
		triggers = append(triggers, TriggerOnDraft)
	case ActionReject:
		newState = fallback(button.NextState, settings.RejectedState)
		triggers = append(triggers, TriggerOnReject)
	case ActionReturn:
		newState = fallback(button.ReturnState, settings.DraftState)
		triggers = append(triggers, TriggerOnReturn)
	case ActionCancel:
		newState = fallback(button.NextState, settings.CancelledState)
		triggers = append(triggers, TriggerOnCancel)
		e.Cancellations = append(e.Cancellations, CancellationRecord{
			Model:       record.Model,
			RecordID:    record.ID,
			RequesterID: user.ID,
			Reason:      input.Comment,
			At:          e.now(),
		})
	case ActionCancelWorkflow:
		newState = fallback(button.NextState, settings.DraftState)
		record.ApprovalUserIDs = nil
		record.DoneUserIDs = nil
		record.ForwardedToUserIDs = nil
		e.Cancellations = append(e.Cancellations, CancellationRecord{
			Model:       record.Model,
			RecordID:    record.ID,
			RequesterID: user.ID,
			Reason:      input.Comment,
			At:          e.now(),
		})
	case ActionTransfer:
		if input.TargetUserID == 0 {
			return TransitionResult{}, ErrTargetUserMissing
		}
		if len(record.ApprovalUserIDs) > 0 && !containsInt64(record.ApprovalUserIDs, user.ID) {
			record.ApprovalUserIDs = []int64{input.TargetUserID}
		} else {
			record.ApprovalUserIDs = replaceOrAppend(record.ApprovalUserIDs, user.ID, input.TargetUserID)
		}
		record.ForwardedToUserIDs = nil
		newState = fallback(button.TransferState, oldState)
		triggers = append(triggers, TriggerOnTransfer)
	case ActionForward:
		if input.TargetUserID == 0 {
			return TransitionResult{}, ErrTargetUserMissing
		}
		currentConfig, _ := e.configForState(settings.ID, oldState)
		record.ApprovalUserIDs = []int64{input.TargetUserID}
		record.ForwardedToUserIDs = []int64{input.TargetUserID}
		e.Forwards = append(e.Forwards, Forward{
			Model:           record.Model,
			RecordID:        record.ID,
			StateValue:      oldState,
			ApprovalStateID: currentConfig.ID,
			WorkflowNodeID:  record.WorkflowNodeID,
			UserID:          input.TargetUserID,
			ForwarderUserID: user.ID,
			Active:          true,
			At:              e.now(),
		})
		newState = fallback(button.NextState, oldState)
		triggers = append(triggers, TriggerOnForward)
	case ActionEmail:
		if input.MailComposed {
			if button.EmailNextAction == "" {
				break
			}
			button.Action = button.EmailNextAction
			goto runAction
		}
		if e.Mailer == nil {
			return TransitionResult{}, ErrMailerMissing
		}
		if err := e.Mailer.SendWorkflowEmail(ctx, EmailRequest{
			TemplateID: button.EmailTemplateID,
			Model:      record.Model,
			RecordID:   record.ID,
			UserID:     user.ID,
			Values:     cloneMap(input.Values),
		}); err != nil {
			return TransitionResult{}, err
		}
		newState = fallback(button.NextState, oldState)
	case ActionServerAction, ActionLegacyServerAction:
		if e.Actions == nil {
			return TransitionResult{}, ErrActionMissing
		}
		result, err := e.Actions.Run(ctx, button.ServerActionID, actions.ExecutionContext{
			Model:        record.Model,
			RecordID:     record.ID,
			RecordIDs:    []int64{record.ID},
			Record:       record.Row(settings.StateField),
			Values:       cloneMap(input.Values),
			UserID:       user.ID,
			UserGroupIDs: append([]int64(nil), user.GroupIDs...),
			Sudo:         button.RunAsSuperuser,
			Trigger:      "workflow_button",
			Now:          e.now(),
			Metadata: map[string]any{
				"button_id":        button.ID,
				"action":           string(button.Action),
				"user_id":          user.ID,
				"run_as_superuser": button.RunAsSuperuser,
				"sudo":             button.RunAsSuperuser,
			},
		})
		if err != nil {
			return TransitionResult{}, err
		}
		actionResults = append(actionResults, result)
		newState = fallback(button.NextState, oldState)
	case ActionMethod:
		method := e.Methods[button.MethodName]
		if method == nil {
			return TransitionResult{}, ErrMethodMissing
		}
		if err := method(ctx, record, button, input); err != nil {
			return TransitionResult{}, err
		}
		newState = fallback(button.NextState, record.State)
	case ActionVote:
		e.recordVote(user, record, oldState, button, input)
		if button.VoteThreshold > 0 && e.voteCount(record.Model, record.ID, oldState, button.ID) >= button.VoteThreshold {
			newState = fallback(button.NextState, settings.ApprovedState)
		}
	default:
		return TransitionResult{}, fmt.Errorf("unsupported workflow action %q", button.Action)
	}

	if newState != oldState {
		if currentConfig, ok := e.configForState(settings.ID, oldState); ok && currentConfig.Committee {
			record.ApprovalUserIDs = nil
			record.DoneUserIDs = nil
		}
		record.ForwardedToUserIDs = nil
		record.State = newState
		if !record.LastStateUpdate.IsZero() {
			record.Values = setValue(record.Values, settings.StateField, newState)
		}
	}
	log := e.appendLog(user.ID, *record, button, oldState, newState, input)
	if newState != oldState {
		record.LastStateUpdate = log.At
		triggers = append(triggers, TriggerOnStateUpdated, TriggerStateChange)
	}
	results, err := e.runAutomationTriggers(ctx, triggers, settings, *record, oldState, newState, user, input)
	if err != nil {
		return TransitionResult{}, err
	}
	actionResults = append(actionResults, results...)
	return TransitionResult{
		Button:        button,
		OldState:      oldState,
		NewState:      newState,
		Log:           log,
		ActionResults: actionResults,
	}, nil
}

func (e *Engine) ApproveRecord(ctx context.Context, user User, record *Record, input Input) (TransitionResult, error) {
	if record == nil {
		return TransitionResult{}, fmt.Errorf("workflow record is nil")
	}
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return TransitionResult{}, ErrSettingsNotFound
	}
	oldState := record.State
	currentConfig, inApprovalState := e.configForState(settings.ID, oldState)
	triggerOnSubmit := !inApprovalState
	button := Button{Action: ActionApprove}
	if currentConfig.Committee {
		result, decision, err := e.applyCommitteePartialApproval(ctx, user, record, settings, button, currentConfig, oldState, input)
		if err != nil || decision == committeeApprovalPartial {
			return result, err
		}
		if decision == committeeApprovalReject {
			newState := settings.RejectedState
			triggers := []AutomationTrigger{TriggerOnCommitteeApproval, TriggerOnReject}
			if newState != oldState {
				record.ApprovalUserIDs = nil
				record.DoneUserIDs = nil
				record.ForwardedToUserIDs = nil
				record.State = newState
				if !record.LastStateUpdate.IsZero() {
					record.Values = setValue(record.Values, settings.StateField, newState)
				}
			}
			log := e.appendLog(user.ID, *record, button, oldState, newState, input)
			if newState != oldState {
				record.LastStateUpdate = log.At
				triggers = append(triggers, TriggerOnStateUpdated, TriggerStateChange)
			}
			results, err := e.runAutomationTriggers(ctx, triggers, settings, *record, oldState, newState, user, input)
			if err != nil {
				return TransitionResult{}, err
			}
			return TransitionResult{Button: button, OldState: oldState, NewState: newState, Log: log, ActionResults: results}, nil
		}
	}
	newState, nextConfig, err := e.resolveApproveState(user, settings, *record, button)
	if err != nil {
		return TransitionResult{}, err
	}
	triggers := []AutomationTrigger{}
	if currentConfig.Committee {
		triggers = append(triggers, TriggerOnCommitteeApproval)
	}
	if triggerOnSubmit {
		triggers = append(triggers, TriggerOnSubmit)
	} else {
		triggers = append(triggers, TriggerOnApproval)
	}
	if nextConfig.ID != 0 {
		triggers = append(triggers, TriggerOnEnterApproval)
	} else {
		triggers = append(triggers, TriggerOnApprove)
	}
	appendUnique(&record.DoneUserIDs, user.ID)
	if newState != oldState {
		if currentConfig.Committee {
			record.ApprovalUserIDs = nil
			record.DoneUserIDs = nil
		}
		record.ForwardedToUserIDs = nil
		record.State = newState
		if !record.LastStateUpdate.IsZero() {
			record.Values = setValue(record.Values, settings.StateField, newState)
		}
	}
	log := e.appendLog(user.ID, *record, button, oldState, newState, input)
	if newState != oldState {
		record.LastStateUpdate = log.At
		triggers = append(triggers, TriggerOnStateUpdated, TriggerStateChange)
	}
	actionResults, err := e.runAutomationTriggers(ctx, triggers, settings, *record, oldState, newState, user, input)
	if err != nil {
		return TransitionResult{}, err
	}
	return TransitionResult{
		Button:        button,
		OldState:      oldState,
		NewState:      newState,
		Log:           log,
		ActionResults: actionResults,
	}, nil
}

type committeeApprovalDecision int

const (
	committeeApprovalContinue committeeApprovalDecision = iota
	committeeApprovalPartial
	committeeApprovalReject
)

func (e *Engine) applyCommitteePartialApproval(ctx context.Context, user User, record *Record, settings Settings, button Button, config ApprovalConfig, oldState string, input Input) (TransitionResult, committeeApprovalDecision, error) {
	appendUnique(&record.DoneUserIDs, user.ID)
	if config.IsVoting {
		e.recordVote(user, record, oldState, button, input)
	}
	record.ApprovalUserIDs = withoutIDs(record.ApprovalUserIDs, record.DoneUserIDs)
	if config.CommitteeLimit > 0 && len(record.DoneUserIDs) >= config.CommitteeLimit {
		record.ApprovalUserIDs = nil
		return TransitionResult{}, committeeApprovalContinue, nil
	}
	if len(record.ApprovalUserIDs) == 0 {
		if config.IsVoting && e.approvalVotePercentage(record.Model, record.ID, oldState, record.DoneUserIDs) < committeeVotePercentage(config) {
			return TransitionResult{}, committeeApprovalReject, nil
		}
		return TransitionResult{}, committeeApprovalContinue, nil
	}
	log := e.appendLog(user.ID, *record, button, oldState, oldState, input)
	actionResults, err := e.runAutomationTriggers(ctx, []AutomationTrigger{TriggerOnCommitteeApproval}, settings, *record, oldState, oldState, user, input)
	if err != nil {
		return TransitionResult{}, committeeApprovalPartial, err
	}
	return TransitionResult{
		Button:        button,
		OldState:      oldState,
		NewState:      oldState,
		Log:           log,
		ActionResults: actionResults,
	}, committeeApprovalPartial, nil
}

func (e *Engine) MassApprove(ctx context.Context, user User, records []*Record, input Input) ([]TransitionResult, error) {
	results := make([]TransitionResult, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		button, ok, err := e.firstButton(user, *record, ActionApprove)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrButtonNotFound
		}
		input.BulkApproval = true
		result, err := e.RunButton(ctx, user, record, button.ID, input)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *Engine) Metadata(user User, record Record) (ViewMetadata, error) {
	buttons, err := e.VisibleButtons(user, record)
	if err != nil {
		return ViewMetadata{}, err
	}
	meta := ViewMetadata{
		Model:    record.Model,
		RecordID: record.ID,
		State:    record.State,
	}
	for _, button := range buttons {
		meta.VisibleButtons = append(meta.VisibleButtons, ButtonDescriptor{
			ID:             button.ID,
			Name:           button.Name,
			Action:         button.Action,
			ButtonClass:    button.ButtonClass,
			ConfirmMessage: button.ConfirmMessage,
			Sequence:       button.Sequence,
		})
	}
	for _, log := range e.Logs {
		if log.Model != record.Model || log.RecordID != record.ID {
			continue
		}
		meta.Logs = append(meta.Logs, LogDescriptor{
			UserID:   log.UserID,
			Action:   log.Action,
			OldState: log.OldState,
			NewState: log.NewState,
			Comment:  log.Comment,
			At:       log.At,
		})
	}
	return meta, nil
}

func (e *Engine) RunCreateAutomations(ctx context.Context, user User, record Record, input Input) ([]actions.Result, error) {
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return nil, ErrSettingsNotFound
	}
	return e.runAutomationTriggers(ctx, []AutomationTrigger{TriggerOnCreate}, settings, record, "", record.State, user, input)
}

func (e *Engine) RunStateUpdatedAutomations(ctx context.Context, user User, record Record, oldState string, input Input) ([]actions.Result, error) {
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return nil, ErrSettingsNotFound
	}
	return e.runAutomationTriggers(ctx, []AutomationTrigger{TriggerOnStateUpdated, TriggerStateChange}, settings, record, oldState, record.State, user, input)
}

func (e *Engine) DueEscalations(record Record, at time.Time) []Escalation {
	if record.LastStateUpdate.IsZero() {
		return nil
	}
	settings, ok := e.SettingsForModel(record.Model)
	if !ok {
		return nil
	}
	var due []Escalation
	for _, escalation := range e.Escalations {
		if !escalation.Active || escalation.SettingsID != settings.ID {
			continue
		}
		if escalation.StateValue != "" && escalation.StateValue != record.State {
			continue
		}
		if !record.LastStateUpdate.Add(escalation.After).After(at) {
			due = append(due, escalation)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].ID < due[j].ID })
	return due
}

func (e *Engine) firstButton(user User, record Record, action ButtonAction) (Button, bool, error) {
	buttons, err := e.VisibleButtons(user, record)
	if err != nil {
		return Button{}, false, err
	}
	for _, button := range buttons {
		if button.Action == action {
			return button, true, nil
		}
	}
	return Button{}, false, nil
}

func (e *Engine) resolveApproveState(user User, settings Settings, record Record, button Button) (string, ApprovalConfig, error) {
	if button.NextState != "" {
		return button.NextState, ApprovalConfig{}, nil
	}
	next, ok, err := e.nextApprovalConfig(settings, record)
	if err != nil {
		return "", ApprovalConfig{}, err
	}
	if !ok {
		return settings.ApprovedState, ApprovalConfig{}, nil
	}
	selected := next
	for selected.AutoApprove && e.userCanApproveConfig(user, record, selected) {
		autoRecord := record
		autoRecord.State = selected.State
		next, ok, err = e.nextApprovalConfig(settings, autoRecord)
		if err != nil {
			return "", ApprovalConfig{}, err
		}
		if !ok {
			return settings.ApprovedState, selected, nil
		}
		selected = next
	}
	return selected.State, selected, nil
}

func (e *Engine) nextApprovalConfig(settings Settings, record Record) (ApprovalConfig, bool, error) {
	configs := e.orderedConfigs(settings.ID)
	if len(configs) == 0 {
		return ApprovalConfig{}, false, nil
	}
	start := 0
	for i, config := range configs {
		if config.State == record.State {
			start = i + 1
			break
		}
	}
	row := record.Row(settings.StateField)
	for _, config := range configs[start:] {
		if config.Condition.Kind != "" {
			ok, err := MatchDomain(row, config.Condition)
			if err != nil || !ok {
				if err != nil {
					return ApprovalConfig{}, false, err
				}
				continue
			}
		}
		return config, true, nil
	}
	return ApprovalConfig{}, false, nil
}

func (e *Engine) orderedConfigs(settingsID int64) []ApprovalConfig {
	configs := make([]ApprovalConfig, 0, len(e.Configs))
	for _, config := range e.Configs {
		if !config.Active || config.SettingsID != settingsID || config.State == "" {
			continue
		}
		configs = append(configs, config)
	}
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Sequence == configs[j].Sequence {
			return configs[i].ID < configs[j].ID
		}
		return configs[i].Sequence < configs[j].Sequence
	})
	return configs
}

func (e *Engine) hasConfigForState(settingsID int64, state string) bool {
	_, ok := e.configForState(settingsID, state)
	return ok
}

func (e *Engine) configForState(settingsID int64, state string) (ApprovalConfig, bool) {
	for _, config := range e.Configs {
		if config.Active && config.SettingsID == settingsID && config.State == state {
			return config, true
		}
	}
	return ApprovalConfig{}, false
}

func (e *Engine) userCanApproveConfig(user User, record Record, config ApprovalConfig) bool {
	if user.ID != 0 && containsInt64(record.DoneUserIDs, user.ID) {
		return false
	}
	if len(record.ApprovalUserIDs) > 0 {
		return containsInt64(record.ApprovalUserIDs, user.ID)
	}
	if len(config.UserIDs) > 0 && !containsInt64(config.UserIDs, user.ID) {
		return false
	}
	if len(config.GroupIDs) > 0 && !hasAnyGroup(user.GroupIDs, config.GroupIDs) {
		return false
	}
	return true
}

func (e *Engine) buttonVisible(user User, record Record, settings Settings, button Button) (bool, error) {
	if button.StateValue != "" && button.StateValue != record.State {
		return false, nil
	}
	if len(button.GroupIDs) > 0 && !hasAnyGroup(user.GroupIDs, button.GroupIDs) {
		return false, nil
	}
	if (strings.EqualFold(button.VisibleTo, "owner") || strings.EqualFold(button.VisibleTo, "requester")) && record.OwnerUserID != user.ID {
		return false, nil
	}
	if strings.EqualFold(button.VisibleTo, "approval") {
		if user.ID != 0 && containsInt64(record.DoneUserIDs, user.ID) {
			return false, nil
		}
		config, ok := e.configForState(settings.ID, record.State)
		if ok {
			return e.userCanApproveConfig(user, record, config), nil
		}
		if len(record.ApprovalUserIDs) > 0 && !containsInt64(record.ApprovalUserIDs, user.ID) {
			return false, nil
		}
	}
	if button.VisibleDomain.Kind != "" {
		ok, err := MatchDomain(record.Row(settings.StateField), button.VisibleDomain)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func (e *Engine) appendLog(userID int64, record Record, button Button, oldState, newState string, input Input) Log {
	now := e.now()
	var duration time.Duration
	if !record.LastStateUpdate.IsZero() {
		duration = now.Sub(record.LastStateUpdate)
	}
	log := Log{
		Model:                record.Model,
		RecordID:             record.ID,
		UserID:               userID,
		ButtonID:             button.ID,
		Action:               button.Action,
		Comment:              input.Comment,
		OldState:             oldState,
		NewState:             newState,
		Duration:             duration,
		BulkApproval:         input.BulkApproval,
		DelegationID:         record.DelegationID,
		DelegationEmployeeID: record.DelegationEmployeeID,
		At:                   now,
	}
	e.Logs = append(e.Logs, log)
	return log
}

func (e *Engine) runAutomationTriggers(ctx context.Context, triggers []AutomationTrigger, settings Settings, record Record, oldState, newState string, user User, input Input) ([]actions.Result, error) {
	triggers = uniqueTriggers(triggers)
	var results []actions.Result
	for _, trigger := range triggers {
		next, err := e.runAutomations(ctx, trigger, settings, record, oldState, newState, user, input)
		if err != nil {
			return results, err
		}
		results = append(results, next...)
	}
	return results, nil
}

func (e *Engine) runAutomations(ctx context.Context, trigger AutomationTrigger, settings Settings, record Record, oldState, newState string, user User, input Input) ([]actions.Result, error) {
	var results []actions.Result
	for _, automation := range e.matchingAutomations(trigger, settings.ID, oldState, newState) {
		if automation.Filter.Kind != "" {
			ok, err := MatchDomain(record.Row(settings.StateField), automation.Filter)
			if err != nil {
				return results, err
			}
			if !ok {
				continue
			}
		}
		exec := automationExecutionContext(automation, trigger, settings, record, oldState, newState, user, input, e.now())
		if strings.TrimSpace(automation.Code) != "" {
			if e.CodeRunner == nil {
				return results, fmt.Errorf("approval.automation %d code: %w", automation.ID, actions.ErrCodeExecutionDisabled)
			}
			result, ok, err := e.CodeRunner(ctx, AutomationCodeRequest{Automation: automation, Code: automation.Code, Exec: exec})
			if err != nil {
				return results, err
			}
			if ok {
				results = append(results, result)
			}
		}
		if len(automation.ServerActionIDs) > 0 {
			if e.Actions == nil {
				return results, ErrActionMissing
			}
			for _, actionID := range automation.ServerActionIDs {
				result, err := e.Actions.Run(ctx, actionID, exec)
				if err != nil {
					return results, err
				}
				results = append(results, result)
			}
		}
		if len(automation.TemplateIDs) > 0 {
			if e.Mailer == nil {
				return results, ErrMailerMissing
			}
			for _, templateID := range automation.TemplateIDs {
				if err := e.Mailer.SendWorkflowEmail(ctx, EmailRequest{
					TemplateID: templateID,
					Model:      record.Model,
					RecordID:   record.ID,
					UserID:     user.ID,
					Values:     cloneMap(input.Values),
				}); err != nil {
					return results, err
				}
			}
		}
	}
	return results, nil
}

func (e *Engine) matchingAutomations(trigger AutomationTrigger, settingsID int64, oldState, newState string) []Automation {
	automations := make([]Automation, 0, len(e.Automations))
	for _, automation := range e.Automations {
		if !automation.Active || automation.SettingsID != settingsID {
			continue
		}
		if automation.Trigger != trigger {
			continue
		}
		if len(automation.FromStates) > 0 && !containsString(automation.FromStates, oldState) {
			continue
		}
		if len(automation.ToStates) > 0 && !containsString(automation.ToStates, newState) {
			continue
		}
		automations = append(automations, automation)
	}
	sort.Slice(automations, func(i, j int) bool {
		if automations[i].Sequence == automations[j].Sequence {
			return automations[i].ID < automations[j].ID
		}
		return automations[i].Sequence < automations[j].Sequence
	})
	return automations
}

func automationExecutionContext(automation Automation, trigger AutomationTrigger, settings Settings, record Record, oldState, newState string, user User, input Input, now time.Time) actions.ExecutionContext {
	values := cloneMap(input.Values)
	metadata := map[string]any{
		"active_model":     record.Model,
		"active_id":        record.ID,
		"active_ids":       []int64{record.ID},
		"automation_id":    automation.ID,
		"old_state":        oldState,
		"new_state":        newState,
		"user_id":          user.ID,
		"trigger":          string(trigger),
		"run_as_superuser": input.RunAsSuperuser,
		"sudo":             input.RunAsSuperuser,
	}
	return actions.ExecutionContext{
		Model:        record.Model,
		RecordID:     record.ID,
		RecordIDs:    []int64{record.ID},
		Record:       record.Row(settings.StateField),
		Values:       values,
		UserID:       user.ID,
		UserGroupIDs: append([]int64(nil), user.GroupIDs...),
		Sudo:         input.RunAsSuperuser,
		Trigger:      string(trigger),
		Now:          now,
		Metadata:     metadata,
	}
}

func uniqueTriggers(triggers []AutomationTrigger) []AutomationTrigger {
	seen := map[AutomationTrigger]bool{}
	out := make([]AutomationTrigger, 0, len(triggers))
	for _, trigger := range triggers {
		if trigger == "" || seen[trigger] {
			continue
		}
		seen[trigger] = true
		out = append(out, trigger)
	}
	return out
}

func (e *Engine) voteCount(modelName string, recordID int64, state string, buttonID int64) int {
	seen := map[int64]bool{}
	for _, vote := range e.Votes {
		if vote.Model == modelName && vote.RecordID == recordID && vote.StateValue == state && vote.ButtonID == buttonID && voteIsApprove(vote) {
			seen[vote.UserID] = true
		}
	}
	return len(seen)
}

func (e *Engine) recordVote(user User, record *Record, state string, button Button, input Input) {
	if record == nil {
		return
	}
	voteType := normalizedVoteType(button.VotingType)
	for index, vote := range e.Votes {
		if vote.Model == record.Model && vote.RecordID == record.ID && vote.StateValue == state && vote.UserID == user.ID {
			e.Votes[index].ButtonID = button.ID
			e.Votes[index].VoteType = voteType
			e.Votes[index].Approved = voteType == "approve"
			e.Votes[index].Comment = input.Comment
			e.Votes[index].ButtonClass = button.ButtonClass
			e.Votes[index].At = e.now()
			return
		}
	}
	e.Votes = append(e.Votes, Vote{
		Model:       record.Model,
		RecordID:    record.ID,
		StateValue:  state,
		UserID:      user.ID,
		ButtonID:    button.ID,
		VoteType:    voteType,
		Approved:    voteType == "approve",
		Comment:     input.Comment,
		ButtonClass: button.ButtonClass,
		At:          e.now(),
	})
}

func (e *Engine) approvalVotePercentage(modelName string, recordID int64, state string, doneUserIDs []int64) float64 {
	total := len(uniqueSortedIDs(doneUserIDs))
	if total == 0 {
		return 0
	}
	approvals := map[int64]bool{}
	for _, vote := range e.Votes {
		if vote.Model == modelName && vote.RecordID == recordID && vote.StateValue == state && voteIsApprove(vote) {
			approvals[vote.UserID] = true
		}
	}
	return float64(len(approvals)) / float64(total) * 100
}

func committeeVotePercentage(config ApprovalConfig) float64 {
	if config.CommitteeVotePercentage == 0 {
		return 50
	}
	return config.CommitteeVotePercentage
}

func normalizedVoteType(value string) string {
	switch strings.TrimSpace(value) {
	case "reject":
		return "reject"
	case "abstain":
		return "abstain"
	default:
		return "approve"
	}
}

func voteIsApprove(vote Vote) bool {
	if vote.VoteType != "" {
		return vote.VoteType == "approve"
	}
	return vote.Approved
}

func (e *Engine) next() int64 {
	id := e.nextID
	e.nextID++
	return id
}

func MatchDomain(row map[string]any, node domain.Node) (bool, error) {
	switch node.Kind {
	case "":
		return true, nil
	case domain.Condition:
		left := row[node.Field]
		switch node.Operator {
		case domain.Equal:
			return reflect.DeepEqual(left, node.Value), nil
		case domain.NotEqual:
			return !reflect.DeepEqual(left, node.Value), nil
		case domain.In:
			return containsAny(node.Value, left)
		case domain.NotIn:
			ok, err := containsAny(node.Value, left)
			return !ok, err
		case domain.Less, domain.LessEqual, domain.Greater, domain.GreaterEqual:
			return compare(left, node.Value, node.Operator)
		case domain.Like, domain.ILike:
			return like(fmt.Sprint(left), fmt.Sprint(node.Value), node.Operator == domain.ILike), nil
		default:
			return false, fmt.Errorf("workflow domain operator %s not implemented", node.Operator)
		}
	case domain.All:
		for _, child := range node.Children {
			ok, err := MatchDomain(row, child)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	case domain.Any:
		for _, child := range node.Children {
			ok, err := MatchDomain(row, child)
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
			return false, fmt.Errorf("workflow not domain requires one child")
		}
		ok, err := MatchDomain(row, node.Children[0])
		return !ok, err
	default:
		return false, fmt.Errorf("unsupported workflow domain kind %s", node.Kind)
	}
}

func compare(left any, right any, op domain.Operator) (bool, error) {
	lf, lok := number(left)
	rf, rok := number(right)
	if !lok || !rok {
		return false, fmt.Errorf("operator %s requires numeric values", op)
	}
	switch op {
	case domain.Less:
		return lf < rf, nil
	case domain.LessEqual:
		return lf <= rf, nil
	case domain.Greater:
		return lf > rf, nil
	case domain.GreaterEqual:
		return lf >= rf, nil
	default:
		return false, fmt.Errorf("unsupported comparison %s", op)
	}
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func containsAny(container any, value any) (bool, error) {
	if container == nil {
		return false, fmt.Errorf("in operator requires slice value")
	}
	rv := reflect.ValueOf(container)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false, fmt.Errorf("in operator requires slice value")
	}
	for i := 0; i < rv.Len(); i++ {
		if reflect.DeepEqual(rv.Index(i).Interface(), value) {
			return true, nil
		}
	}
	return false, nil
}

func like(left string, pattern string, insensitive bool) bool {
	if insensitive {
		left = strings.ToLower(left)
		pattern = strings.ToLower(pattern)
	}
	pattern = strings.Trim(pattern, "%")
	return strings.Contains(left, pattern)
}

func hasAnyGroup(userGroups []int64, required []int64) bool {
	groups := map[int64]bool{}
	for _, groupID := range userGroups {
		groups[groupID] = true
	}
	for _, groupID := range required {
		if groups[groupID] {
			return true
		}
	}
	return false
}

func appendUnique(ids *[]int64, id int64) {
	for _, existing := range *ids {
		if existing == id {
			return
		}
	}
	*ids = append(*ids, id)
}

func replaceOrAppend(ids []int64, oldID, newID int64) []int64 {
	out := append([]int64(nil), ids...)
	replaced := false
	for i, id := range out {
		if id == oldID {
			out[i] = newID
			replaced = true
		}
	}
	if !replaced {
		out = append(out, newID)
	}
	return out
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func fallback(value string, fallbackValue string) string {
	if value != "" {
		return value
	}
	return fallbackValue
}

func setValue(values map[string]any, key string, value any) map[string]any {
	if key == "" {
		key = "state"
	}
	out := cloneMap(values)
	if out == nil {
		out = map[string]any{}
	}
	out[key] = value
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
