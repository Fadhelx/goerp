package actions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Kind string

const (
	KindGo              Kind = "go"
	KindCreate          Kind = "create"
	KindWrite           Kind = "write"
	KindSendMail        Kind = "send_mail"
	KindEnqueue         Kind = "enqueue"
	KindWebhook         Kind = "webhook"
	KindPython          Kind = "python"
	KindCode            Kind = "code"
	KindCopy            Kind = "object_copy"
	KindMulti           Kind = "multi"
	KindMailPost        Kind = "mail_post"
	KindFollowers       Kind = "followers"
	KindRemoveFollowers Kind = "remove_followers"
	KindNextActivity    Kind = "next_activity"
	KindSMS             Kind = "sms"
	KindWhatsApp        Kind = "whatsapp"
	KindDocumentAccount Kind = "documents_account_record_create"
	KindAI              Kind = "ai"
)

var (
	ErrActionNotFound         = errors.New("server action not found")
	ErrActionDisabled         = errors.New("server action disabled")
	ErrGoActionNotFound       = errors.New("registered go action not found")
	ErrCreateHookMissing      = errors.New("create hook missing")
	ErrWriteHookMissing       = errors.New("write hook missing")
	ErrMailHookMissing        = errors.New("mail hook missing")
	ErrEnqueueHookMissing     = errors.New("enqueue hook missing")
	ErrFollowerHookMissing    = errors.New("follower hook missing")
	ErrActivityHookMissing    = errors.New("activity hook missing")
	ErrSMSHookMissing         = errors.New("sms hook missing")
	ErrWhatsAppHookMissing    = errors.New("whatsapp hook missing")
	ErrDocumentHookMissing    = errors.New("document account hook missing")
	ErrWebhookDisabled        = errors.New("webhook execution disabled")
	ErrCodeExecutionDisabled  = errors.New("python/code execution disabled")
	ErrRecordSelectionMissing = errors.New("record selection missing")
	ErrAIRunnerMissing        = errors.New("ai action runner missing")
	ErrSequenceHookMissing    = errors.New("sequence hook missing")
	ErrObjectHookMissing      = errors.New("object action hook missing")
	ErrActionWarning          = errors.New("server action has warnings")
	ErrActionForbidden        = errors.New("server action forbidden")
	ErrActionRecursion        = errors.New("server action recursion")
)

type runStackContextKey struct{}

type RelationOperation string

const (
	RelationAdd    RelationOperation = "add"
	RelationRemove RelationOperation = "remove"
	RelationSet    RelationOperation = "set"
	RelationClear  RelationOperation = "clear"
)

type RelationCommand struct {
	Operation RelationOperation
	IDs       []int64
}

type ServerAction struct {
	ID                            int64
	Name                          string
	Model                         string
	Kind                          Kind
	Active                        bool
	Disabled                      bool
	Sequence                      int
	GoActionName                  string
	Values                        map[string]any
	Value                         string
	HTMLValue                     string
	EvaluationType                string
	SequenceID                    int64
	ResourceRef                   string
	SelectionValue                int64
	ParentID                      int64
	ChildIDs                      []int64
	CrudModelID                   int64
	CrudModelName                 string
	LinkFieldID                   int64
	LinkFieldName                 string
	LinkFieldType                 string
	GroupIDs                      []int64
	BaseAutomationID              int64
	UpdateFieldID                 int64
	UpdatePath                    string
	UpdateRelatedModelID          int64
	UpdateFieldType               string
	UpdateM2MOperation            string
	UpdateBooleanValue            string
	Warning                       string
	RecordIDs                     []int64
	MailTemplateID                int64
	TemplateID                    int64
	MailPostAutoFollow            bool
	MailPostMethod                string
	FollowersType                 string
	FollowersPartnerFieldName     string
	PartnerIDs                    []int64
	ActivityTypeID                int64
	ActivitySummary               string
	ActivityNote                  string
	ActivityDateDeadlineRange     int
	ActivityDateDeadlineRangeType string
	ActivityUserType              string
	ActivityUserID                int64
	ActivityUserFieldName         string
	SMSTemplateID                 int64
	SMSMethod                     string
	WhatsAppTemplateID            int64
	DocumentsAccountCreateModel   string
	DocumentsAccountJournalID     int64
	DocumentsAccountMoveType      string
	QueueKey                      string
	QueueName                     string
	Payload                       map[string]any
	WebhookURL                    string
	WebhookFieldIDs               []int64
	WebhookSamplePayload          string
	Code                          string
	AIActionPrompt                string
	AIToolIDs                     []int64
	UseInAI                       bool
	AIToolDescription             string
	AIToolSchema                  string
	AIToolAllowEndMessage         bool
	Metadata                      map[string]any
}

type ExecutionContext struct {
	Model          string
	RecordID       int64
	RecordIDs      []int64
	Record         map[string]any
	Values         map[string]any
	Fields         []string
	UserID         int64
	UserGroupIDs   []int64
	Trigger        string
	Now            time.Time
	LastRunAt      time.Time
	CurrentRunAt   time.Time
	Metadata       map[string]any
	CommitProgress CronProgressFunc
}

type CronProgressFunc func(processed int, remaining *int, deactivate bool) bool

type Result struct {
	ActionID       int64
	Kind           Kind
	GoActionName   string
	Action         any
	CreatedID      int64
	WrittenIDs     []int64
	MailSent       bool
	SMSSent        bool
	WhatsAppSent   bool
	Enqueued       bool
	DisabledReason string
	Metadata       map[string]any
}

type Creator interface {
	Create(context.Context, string, map[string]any) (int64, error)
}

type Writer interface {
	Write(context.Context, string, []int64, map[string]any) error
}

type Sequencer interface {
	NextSequence(context.Context, int64) (string, error)
}

type ObjectOperator interface {
	CreateObject(context.Context, ServerAction, ExecutionContext, map[string]any) (int64, error)
	WriteObject(context.Context, ServerAction, ExecutionContext, map[string]any) ([]int64, error)
	CopyObject(context.Context, ServerAction, ExecutionContext) (int64, error)
}

type Evaluator interface {
	EvaluateActionValue(context.Context, ServerAction, ExecutionContext) (any, error)
}

type MailRequest struct {
	TemplateID int64
	Model      string
	RecordIDs  []int64
	Values     map[string]any
	Metadata   map[string]any
}

type Mailer interface {
	SendMail(context.Context, MailRequest) error
}

type FollowersRequest struct {
	Model            string
	RecordIDs        []int64
	PartnerIDs       []int64
	PartnerFieldName string
	Remove           bool
	Metadata         map[string]any
}

type FollowerUpdater interface {
	UpdateFollowers(context.Context, FollowersRequest) error
}

type ActivityRequest struct {
	Model             string
	RecordIDs         []int64
	ActivityTypeID    int64
	Summary           string
	Note              string
	DeadlineRange     int
	DeadlineRangeType string
	UserType          string
	UserID            int64
	UserFieldName     string
	Now               time.Time
	Metadata          map[string]any
}

type ActivityScheduler interface {
	ScheduleActivity(context.Context, ActivityRequest) error
}

type SMSRequest struct {
	TemplateID int64
	Method     string
	Model      string
	RecordIDs  []int64
	Metadata   map[string]any
}

type SMSSender interface {
	SendSMS(context.Context, SMSRequest) error
}

type WhatsAppRequest struct {
	TemplateID int64
	Model      string
	RecordIDs  []int64
	Metadata   map[string]any
}

type WhatsAppSender interface {
	SendWhatsApp(context.Context, WhatsAppRequest) error
}

type DocumentAccountRecordRequest struct {
	Model       string
	RecordIDs   []int64
	CreateModel string
	MoveType    string
	JournalID   int64
	Metadata    map[string]any
}

type DocumentAccountRecordCreator interface {
	CreateDocumentAccountRecord(context.Context, DocumentAccountRecordRequest) (any, error)
}

type QueueJob struct {
	Key       string
	Name      string
	ActionID  int64
	Model     string
	RecordIDs []int64
	Payload   map[string]any
	Metadata  map[string]any
}

type Enqueuer interface {
	Enqueue(context.Context, QueueJob) error
}

type AIActionRequest struct {
	Action    ServerAction
	Tools     []ServerAction
	Execution ExecutionContext
}

type AIRunner interface {
	RunAI(context.Context, AIActionRequest) (Result, error)
}

type Hooks struct {
	Creator           Creator
	Writer            Writer
	Sequencer         Sequencer
	ObjectOperator    ObjectOperator
	Evaluator         Evaluator
	Mailer            Mailer
	FollowerUpdater   FollowerUpdater
	ActivityScheduler ActivityScheduler
	SMSSender         SMSSender
	WhatsAppSender    WhatsAppSender
	DocumentCreator   DocumentAccountRecordCreator
	Enqueuer          Enqueuer
	AIRunner          AIRunner
}

type GoAction func(context.Context, ServerAction, ExecutionContext) (Result, error)

type Registry struct {
	mu        sync.RWMutex
	nextID    int64
	actions   map[int64]ServerAction
	goActions map[string]GoAction
	hooks     Hooks
}

func NewRegistry(hooks Hooks) *Registry {
	return &Registry{
		nextID:    1,
		actions:   map[int64]ServerAction{},
		goActions: map[string]GoAction{},
		hooks:     hooks,
	}
}

func (r *Registry) SetHooks(hooks Hooks) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = hooks
}

func (r *Registry) RegisterGo(name string, fn GoAction) error {
	if name == "" {
		return fmt.Errorf("go action requires name")
	}
	if fn == nil {
		return fmt.Errorf("go action %s requires function", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goActions[name] = fn
	return nil
}

func (r *Registry) Register(action ServerAction) (int64, error) {
	if action.Name == "" {
		return 0, fmt.Errorf("server action requires name")
	}
	if !validKind(action.Kind) {
		return 0, fmt.Errorf("unsupported server action kind %q", action.Kind)
	}
	if action.Kind == KindGo && action.GoActionName == "" {
		return 0, fmt.Errorf("go server action requires go action name")
	}
	if action.Kind == KindCreate && action.Model == "" {
		return 0, fmt.Errorf("create server action requires model")
	}
	if action.Disabled {
		action.Active = false
	} else {
		action.Active = true
	}
	action.Values = cloneMap(action.Values)
	action.Payload = cloneMap(action.Payload)
	action.Metadata = cloneMap(action.Metadata)
	action.RecordIDs = cloneIDs(action.RecordIDs)
	action.AIToolIDs = cloneIDs(action.AIToolIDs)
	action.ChildIDs = cloneIDs(action.ChildIDs)
	action.GroupIDs = cloneIDs(action.GroupIDs)
	action.PartnerIDs = cloneIDs(action.PartnerIDs)
	action.WebhookFieldIDs = cloneIDs(action.WebhookFieldIDs)

	r.mu.Lock()
	defer r.mu.Unlock()
	if action.ID == 0 {
		action.ID = r.nextID
		r.nextID++
	} else {
		if _, exists := r.actions[action.ID]; exists {
			return 0, fmt.Errorf("server action %d already registered", action.ID)
		}
		if action.ID >= r.nextID {
			r.nextID = action.ID + 1
		}
	}
	r.actions[action.ID] = action
	return action.ID, nil
}

func (r *Registry) Get(id int64) (ServerAction, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	action, ok := r.actions[id]
	return cloneAction(action), ok
}

func (r *Registry) Run(ctx context.Context, id int64, exec ExecutionContext) (Result, error) {
	stack, _ := ctx.Value(runStackContextKey{}).(map[int64]bool)
	if stack == nil {
		stack = map[int64]bool{}
		ctx = context.WithValue(ctx, runStackContextKey{}, stack)
	}
	if stack[id] {
		return Result{ActionID: id, DisabledReason: "recursive server action"}, ErrActionRecursion
	}
	stack[id] = true
	defer delete(stack, id)

	r.mu.RLock()
	action, ok := r.actions[id]
	hooks := r.hooks
	goAction := r.goActions[action.GoActionName]
	var aiTools []ServerAction
	if ok && action.Kind == KindAI {
		aiTools = make([]ServerAction, 0, len(action.AIToolIDs))
		for _, toolID := range action.AIToolIDs {
			if tool, ok := r.actions[toolID]; ok && tool.UseInAI {
				aiTools = append(aiTools, cloneAction(tool))
			}
		}
	}
	r.mu.RUnlock()
	if !ok {
		return Result{}, ErrActionNotFound
	}
	if action.Disabled || !action.Active {
		return Result{ActionID: action.ID, Kind: action.Kind, DisabledReason: "action disabled"}, ErrActionDisabled
	}
	if strings.TrimSpace(action.Warning) != "" {
		return Result{ActionID: action.ID, Kind: action.Kind, DisabledReason: action.Warning}, ErrActionWarning
	}
	if len(action.GroupIDs) > 0 && exec.UserID != 1 && !hasAnyID(action.GroupIDs, executionGroupIDs(exec)) {
		return Result{ActionID: action.ID, Kind: action.Kind, DisabledReason: "forbidden server action"}, ErrActionForbidden
	}

	if shouldSplitSingletonAction(action, exec) {
		return r.runSingletonAction(ctx, action, hooks, goAction, aiTools, exec)
	}
	return r.runAction(ctx, action, hooks, goAction, aiTools, exec)
}

func (r *Registry) runSingletonAction(ctx context.Context, action ServerAction, hooks Hooks, goAction GoAction, aiTools []ServerAction, exec ExecutionContext) (Result, error) {
	var out Result
	for _, recordID := range targetIDs(action, exec) {
		singleAction := cloneAction(action)
		singleAction.RecordIDs = []int64{recordID}
		singleExec := singletonExecution(exec, recordID)
		result, err := r.runAction(ctx, singleAction, hooks, goAction, aiTools, singleExec)
		out = result
		if err != nil {
			return out, err
		}
	}
	if out.ActionID == 0 {
		out.ActionID = action.ID
	}
	if out.Kind == "" {
		out.Kind = action.Kind
	}
	if out.GoActionName == "" {
		out.GoActionName = action.GoActionName
	}
	return out, nil
}

func (r *Registry) runAction(ctx context.Context, action ServerAction, hooks Hooks, goAction GoAction, aiTools []ServerAction, exec ExecutionContext) (Result, error) {
	result := Result{ActionID: action.ID, Kind: action.Kind, GoActionName: action.GoActionName}
	switch action.Kind {
	case KindGo:
		if goAction == nil {
			return result, ErrGoActionNotFound
		}
		out, err := goAction(ctx, cloneAction(action), cloneExecution(exec))
		if out.ActionID == 0 {
			out.ActionID = action.ID
		}
		if out.Kind == "" {
			out.Kind = action.Kind
		}
		if out.GoActionName == "" {
			out.GoActionName = action.GoActionName
		}
		return out, err
	case KindCreate:
		values, err := operationValues(ctx, action, exec, hooks)
		if err != nil {
			return result, err
		}
		if hooks.ObjectOperator != nil {
			createdID, err := hooks.ObjectOperator.CreateObject(ctx, cloneAction(action), cloneExecution(exec), values)
			result.CreatedID = createdID
			return result, err
		}
		if hooks.Creator == nil {
			return result, ErrCreateHookMissing
		}
		createdID, err := hooks.Creator.Create(ctx, targetModel(action, exec), values)
		result.CreatedID = createdID
		return result, err
	case KindWrite:
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, ErrRecordSelectionMissing
		}
		if hooks.ObjectOperator != nil {
			values := cloneMap(action.Values)
			writtenIDs, err := hooks.ObjectOperator.WriteObject(ctx, cloneAction(action), cloneExecution(exec), values)
			result.WrittenIDs = writtenIDs
			return result, err
		}
		values, err := operationValues(ctx, action, exec, hooks)
		if err != nil {
			return result, err
		}
		if hooks.Writer == nil {
			return result, ErrWriteHookMissing
		}
		if err := hooks.Writer.Write(ctx, targetModel(action, exec), ids, values); err != nil {
			return result, err
		}
		result.WrittenIDs = ids
		return result, nil
	case KindSendMail:
		if hooks.Mailer == nil {
			return result, ErrMailHookMissing
		}
		err := hooks.Mailer.SendMail(ctx, MailRequest{
			TemplateID: firstNonZero(action.MailTemplateID, action.TemplateID),
			Model:      targetModel(action, exec),
			RecordIDs:  targetIDs(action, exec),
			Values:     mergeMaps(exec.Values, action.Values),
			Metadata:   cloneMap(action.Metadata),
		})
		result.MailSent = err == nil
		return result, err
	case KindMailPost:
		if hooks.Mailer == nil {
			return result, ErrMailHookMissing
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, ErrRecordSelectionMissing
		}
		metadata := cloneMap(action.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["mail_post_method"] = action.MailPostMethod
		metadata["mail_post_autofollow"] = action.MailPostAutoFollow
		err := hooks.Mailer.SendMail(ctx, MailRequest{
			TemplateID: firstNonZero(action.TemplateID, action.MailTemplateID),
			Model:      targetModel(action, exec),
			RecordIDs:  ids,
			Values:     mergeMaps(exec.Values, action.Values),
			Metadata:   metadata,
		})
		result.MailSent = err == nil
		return result, err
	case KindFollowers, KindRemoveFollowers:
		if hooks.FollowerUpdater == nil {
			return result, ErrFollowerHookMissing
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, ErrRecordSelectionMissing
		}
		err := hooks.FollowerUpdater.UpdateFollowers(ctx, FollowersRequest{
			Model:            targetModel(action, exec),
			RecordIDs:        ids,
			PartnerIDs:       cloneIDs(action.PartnerIDs),
			PartnerFieldName: action.FollowersPartnerFieldName,
			Remove:           action.Kind == KindRemoveFollowers,
			Metadata:         cloneMap(action.Metadata),
		})
		return result, err
	case KindNextActivity:
		if hooks.ActivityScheduler == nil {
			return result, ErrActivityHookMissing
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, ErrRecordSelectionMissing
		}
		err := hooks.ActivityScheduler.ScheduleActivity(ctx, ActivityRequest{
			Model:             targetModel(action, exec),
			RecordIDs:         ids,
			ActivityTypeID:    action.ActivityTypeID,
			Summary:           action.ActivitySummary,
			Note:              action.ActivityNote,
			DeadlineRange:     action.ActivityDateDeadlineRange,
			DeadlineRangeType: action.ActivityDateDeadlineRangeType,
			UserType:          action.ActivityUserType,
			UserID:            action.ActivityUserID,
			UserFieldName:     action.ActivityUserFieldName,
			Now:               exec.Now,
			Metadata:          cloneMap(action.Metadata),
		})
		return result, err
	case KindSMS:
		if action.SMSTemplateID == 0 {
			return result, nil
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, nil
		}
		if hooks.SMSSender == nil {
			return result, ErrSMSHookMissing
		}
		err := hooks.SMSSender.SendSMS(ctx, SMSRequest{
			TemplateID: action.SMSTemplateID,
			Method:     firstNonEmpty(action.SMSMethod, "sms"),
			Model:      targetModel(action, exec),
			RecordIDs:  ids,
			Metadata:   mergeMaps(exec.Metadata, action.Metadata),
		})
		result.SMSSent = err == nil
		return result, err
	case KindWhatsApp:
		if action.WhatsAppTemplateID == 0 {
			return result, nil
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, nil
		}
		if hooks.WhatsAppSender == nil {
			return result, ErrWhatsAppHookMissing
		}
		err := hooks.WhatsAppSender.SendWhatsApp(ctx, WhatsAppRequest{
			TemplateID: action.WhatsAppTemplateID,
			Model:      targetModel(action, exec),
			RecordIDs:  ids,
			Metadata:   mergeMaps(exec.Metadata, action.Metadata),
		})
		result.WhatsAppSent = err == nil
		return result, err
	case KindDocumentAccount:
		if targetModel(action, exec) != "documents.document" {
			return result, nil
		}
		ids := targetIDs(action, exec)
		if len(ids) == 0 {
			return result, nil
		}
		if hooks.DocumentCreator == nil {
			return result, ErrDocumentHookMissing
		}
		out, err := hooks.DocumentCreator.CreateDocumentAccountRecord(ctx, DocumentAccountRecordRequest{
			Model:       targetModel(action, exec),
			RecordIDs:   ids,
			CreateModel: action.DocumentsAccountCreateModel,
			MoveType:    action.DocumentsAccountMoveType,
			JournalID:   action.DocumentsAccountJournalID,
			Metadata:    cloneMap(action.Metadata),
		})
		result.Action = out
		return result, err
	case KindEnqueue:
		if hooks.Enqueuer == nil {
			return result, ErrEnqueueHookMissing
		}
		err := hooks.Enqueuer.Enqueue(ctx, QueueJob{
			Key:       action.QueueKey,
			Name:      action.QueueName,
			ActionID:  action.ID,
			Model:     targetModel(action, exec),
			RecordIDs: targetIDs(action, exec),
			Payload:   mergeMaps(exec.Values, action.Payload),
			Metadata:  cloneMap(action.Metadata),
		})
		result.Enqueued = err == nil
		return result, err
	case KindWebhook:
		result.DisabledReason = "webhook execution disabled"
		return result, ErrWebhookDisabled
	case KindPython, KindCode:
		result.DisabledReason = "python/code execution disabled"
		return result, ErrCodeExecutionDisabled
	case KindCopy:
		if hooks.ObjectOperator == nil {
			return result, ErrObjectHookMissing
		}
		createdID, err := hooks.ObjectOperator.CopyObject(ctx, cloneAction(action), cloneExecution(exec))
		result.CreatedID = createdID
		return result, err
	case KindMulti:
		out := result
		for _, childID := range r.sortedChildIDs(action.ChildIDs) {
			childResult, err := r.Run(ctx, childID, exec)
			out = childResult
			if err != nil {
				return out, err
			}
		}
		if out.ActionID == 0 {
			out.ActionID = action.ID
		}
		if out.Kind == "" {
			out.Kind = action.Kind
		}
		return out, nil
	case KindAI:
		if hooks.AIRunner == nil {
			return result, ErrAIRunnerMissing
		}
		out, err := hooks.AIRunner.RunAI(ctx, AIActionRequest{
			Action:    cloneAction(action),
			Tools:     aiTools,
			Execution: cloneExecution(exec),
		})
		if out.ActionID == 0 {
			out.ActionID = action.ID
		}
		if out.Kind == "" {
			out.Kind = action.Kind
		}
		return out, err
	default:
		return result, fmt.Errorf("unsupported server action kind %q", action.Kind)
	}
}

func (r *Registry) sortedChildIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	r.mu.RLock()
	defer r.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		left, leftOK := r.actions[out[i]]
		right, rightOK := r.actions[out[j]]
		if !leftOK || !rightOK {
			return leftOK
		}
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})
	return out
}

func executionGroupIDs(exec ExecutionContext) []int64 {
	if len(exec.UserGroupIDs) > 0 {
		return cloneIDs(exec.UserGroupIDs)
	}
	if ids := idsFromAny(exec.Values["group_ids"]); len(ids) > 0 {
		return ids
	}
	return idsFromAny(exec.Metadata["group_ids"])
}

func hasAnyID(required []int64, available []int64) bool {
	if len(required) == 0 {
		return true
	}
	seen := map[int64]bool{}
	for _, id := range available {
		seen[id] = true
	}
	for _, id := range required {
		if seen[id] {
			return true
		}
	}
	return false
}

func (r *Registry) RunNamed(ctx context.Context, name string, exec ExecutionContext) (Result, error) {
	r.mu.RLock()
	goAction := r.goActions[name]
	r.mu.RUnlock()
	if goAction == nil {
		return Result{Kind: KindGo, GoActionName: name}, ErrGoActionNotFound
	}
	action := ServerAction{Name: name, Kind: KindGo, Active: true, GoActionName: name}
	result, err := goAction(ctx, action, cloneExecution(exec))
	if result.Kind == "" {
		result.Kind = KindGo
	}
	if result.GoActionName == "" {
		result.GoActionName = name
	}
	return result, err
}

func validKind(kind Kind) bool {
	switch kind {
	case KindGo, KindCreate, KindWrite, KindSendMail, KindMailPost, KindFollowers, KindRemoveFollowers, KindNextActivity, KindSMS, KindWhatsApp, KindDocumentAccount, KindEnqueue, KindWebhook, KindPython, KindCode, KindCopy, KindMulti, KindAI:
		return true
	default:
		return false
	}
}

func shouldSplitSingletonAction(action ServerAction, exec ExecutionContext) bool {
	if !sourceSingletonActionKind(action.Kind) {
		return false
	}
	return len(targetIDs(action, exec)) > 1
}

func sourceSingletonActionKind(kind Kind) bool {
	switch kind {
	case KindCreate, KindWrite, KindCopy, KindWebhook, KindMulti, KindNextActivity:
		return true
	default:
		return false
	}
}

func singletonExecution(exec ExecutionContext, recordID int64) ExecutionContext {
	out := cloneExecution(exec)
	out.RecordID = recordID
	out.RecordIDs = []int64{recordID}
	if out.Values == nil {
		out.Values = map[string]any{}
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	out.Values["active_id"] = recordID
	out.Values["active_ids"] = []int64{recordID}
	out.Metadata["active_id"] = recordID
	out.Metadata["active_ids"] = []int64{recordID}
	if out.Model != "" {
		if _, ok := out.Values["active_model"]; !ok {
			out.Values["active_model"] = out.Model
		}
		if _, ok := out.Metadata["active_model"]; !ok {
			out.Metadata["active_model"] = out.Model
		}
	}
	return out
}

func targetModel(action ServerAction, exec ExecutionContext) string {
	if action.Kind == KindCreate && action.CrudModelName != "" {
		return action.CrudModelName
	}
	if action.Model != "" {
		return action.Model
	}
	return exec.Model
}

func targetIDs(action ServerAction, exec ExecutionContext) []int64 {
	if len(action.RecordIDs) > 0 {
		return cloneIDs(action.RecordIDs)
	}
	if len(exec.RecordIDs) > 0 {
		return cloneIDs(exec.RecordIDs)
	}
	if exec.RecordID != 0 {
		return []int64{exec.RecordID}
	}
	return nil
}

func operationValues(ctx context.Context, action ServerAction, exec ExecutionContext, hooks Hooks) (map[string]any, error) {
	if len(action.Values) > 0 {
		return cloneMap(action.Values), nil
	}
	if action.Kind == KindWrite && action.UpdatePath != "" {
		fieldName := updateFieldName(action.UpdatePath)
		if fieldName != "" {
			value, err := evaluatedActionValue(ctx, action, exec, hooks)
			if err != nil {
				return nil, err
			}
			return map[string]any{fieldName: value}, nil
		}
	}
	if action.Kind == KindCreate && action.Value != "" {
		return map[string]any{"name": action.Value}, nil
	}
	return cloneMap(exec.Values), nil
}

func updateFieldName(path string) string {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func evaluatedActionValue(ctx context.Context, action ServerAction, exec ExecutionContext, hooks Hooks) (any, error) {
	switch action.EvaluationType {
	case "sequence":
		if action.SequenceID != 0 {
			if hooks.Sequencer == nil {
				return nil, ErrSequenceHookMissing
			}
			return hooks.Sequencer.NextSequence(ctx, action.SequenceID)
		}
	case "equation":
		if hooks.Evaluator != nil {
			return hooks.Evaluator.EvaluateActionValue(ctx, action, exec)
		}
		return action.Value, nil
	}
	fieldType := strings.TrimSpace(action.UpdateFieldType)
	switch fieldType {
	case "boolean", "bool":
		return action.UpdateBooleanValue == "true" || strings.EqualFold(action.Value, "true"), nil
	case "many2one", "integer", "int":
		if id := idFromReference(action.ResourceRef); id != 0 {
			if fieldType == "many2one" {
				return id, nil
			}
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(action.Value), 10, 64); err == nil {
			return id, nil
		}
	case "float":
		if value, err := strconv.ParseFloat(strings.TrimSpace(action.Value), 64); err == nil {
			return value, nil
		}
	case "html":
		return action.HTMLValue, nil
	case "one2many", "many2many":
		ids := relationValueIDs(action)
		switch action.UpdateM2MOperation {
		case "clear":
			return RelationCommand{Operation: RelationClear}, nil
		case "set":
			return RelationCommand{Operation: RelationSet, IDs: ids}, nil
		case "add":
			return RelationCommand{Operation: RelationAdd, IDs: ids}, nil
		case "remove":
			return RelationCommand{Operation: RelationRemove, IDs: ids}, nil
		}
	}
	if action.ResourceRef != "" {
		return idFromReference(action.ResourceRef), nil
	}
	return action.Value, nil
}

func relationValueIDs(action ServerAction) []int64 {
	if id := idFromReference(action.ResourceRef); id != 0 {
		return []int64{id}
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(action.Value), 10, 64); err == nil && id != 0 {
		return []int64{id}
	}
	return nil
}

func idFromReference(value string) int64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0
	}
	if _, raw, ok := strings.Cut(text, ","); ok {
		text = raw
	}
	id, _ := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	return id
}

func mergeMaps(base map[string]any, overrides map[string]any) map[string]any {
	out := cloneMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func cloneAction(action ServerAction) ServerAction {
	action.Values = cloneMap(action.Values)
	action.Payload = cloneMap(action.Payload)
	action.Metadata = cloneMap(action.Metadata)
	action.RecordIDs = cloneIDs(action.RecordIDs)
	action.AIToolIDs = cloneIDs(action.AIToolIDs)
	action.ChildIDs = cloneIDs(action.ChildIDs)
	action.GroupIDs = cloneIDs(action.GroupIDs)
	action.PartnerIDs = cloneIDs(action.PartnerIDs)
	action.WebhookFieldIDs = cloneIDs(action.WebhookFieldIDs)
	return action
}

func cloneExecution(exec ExecutionContext) ExecutionContext {
	exec.RecordIDs = cloneIDs(exec.RecordIDs)
	exec.Record = cloneMap(exec.Record)
	exec.Values = cloneMap(exec.Values)
	exec.Metadata = cloneMap(exec.Metadata)
	exec.Fields = append([]string(nil), exec.Fields...)
	exec.UserGroupIDs = cloneIDs(exec.UserGroupIDs)
	return exec
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

func cloneIDs(in []int64) []int64 {
	return append([]int64(nil), in...)
}

func idsFromAny(value any) []int64 {
	switch v := value.(type) {
	case []int64:
		return cloneIDs(v)
	case []int:
		out := make([]int64, 0, len(v))
		for _, id := range v {
			if id != 0 {
				out = append(out, int64(id))
			}
		}
		return out
	case []any:
		out := make([]int64, 0, len(v))
		for _, item := range v {
			if id := idFromAny(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	default:
		if id := idFromAny(value); id != 0 {
			return []int64{id}
		}
		return nil
	}
}

func idFromAny(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return id
	default:
		return 0
	}
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
