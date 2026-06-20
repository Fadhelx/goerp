package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorp/internal/actions"
	"gorp/internal/domain"
	"gorp/internal/record"
)

type IntervalType string

const (
	IntervalMinutes IntervalType = "minutes"
	IntervalHours   IntervalType = "hours"
	IntervalDays    IntervalType = "days"
	IntervalWeeks   IntervalType = "weeks"
	IntervalMonths  IntervalType = "months"
)

type Cron struct {
	ID             int64
	Name           string
	Active         bool
	UserID         int64
	Context        map[string]any
	Model          string
	ActionID       int64
	ActionName     string
	Code           string
	IntervalNumber int
	IntervalType   IntervalType
	NextCall       time.Time
	LastCall       time.Time
	Priority       int
	FailureCount   int
	FirstFailureAt time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TriggerStatus string

const (
	TriggerPending TriggerStatus = "pending"
	TriggerDone    TriggerStatus = "done"
	TriggerFailed  TriggerStatus = "failed"
)

type Trigger struct {
	ID        int64
	CronID    int64
	At        time.Time
	Status    TriggerStatus
	UserID    int64
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Progress struct {
	ID              int64
	CronID          int64
	Done            int
	Remaining       int
	Deactivate      bool
	TimedOutCounter int
	StartedAt       time.Time
	UpdatedAt       time.Time
}

type Action func(Cron) error

type BackoffFunc func(Cron) time.Duration

var ErrCronActionMissing = errors.New("cron action missing")
var ErrCronTimedOut = errors.New("cron timed out")

type ActionRunner interface {
	Run(context.Context, int64, actions.ExecutionContext) (actions.Result, error)
}

type NamedActionRunner interface {
	RunNamed(context.Context, string, actions.ExecutionContext) (actions.Result, error)
}

type Option func(*Scheduler)

func WithRetryBackoff(backoff BackoffFunc) Option {
	return func(s *Scheduler) {
		if backoff != nil {
			s.retryBackoff = backoff
		}
	}
}

type Scheduler struct {
	mu             sync.Mutex
	nextCronID     int64
	nextTriggerID  int64
	nextProgressID int64
	crons          map[int64]Cron
	triggers       map[int64]Trigger
	progress       map[int64]Progress
	running        map[int64]bool
	retryBackoff   BackoffFunc
}

type Result struct {
	Ran           int
	Succeeded     int
	Failed        int
	SkippedLocked int
	Errors        []error
}

type Snapshot struct {
	NextCronID     int64
	NextTriggerID  int64
	NextProgressID int64
	Crons          []Cron
	Triggers       []Trigger
	Progress       []Progress
}

func New(options ...Option) *Scheduler {
	s := &Scheduler{
		nextCronID:     1,
		nextTriggerID:  1,
		nextProgressID: 1,
		crons:          map[int64]Cron{},
		triggers:       map[int64]Trigger{},
		progress:       map[int64]Progress{},
		running:        map[int64]bool{},
		retryBackoff:   defaultRetryBackoff,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Scheduler) AddCron(cron Cron) (Cron, error) {
	if cron.Name == "" {
		return Cron{}, errors.New("cron requires name")
	}
	if err := validateInterval(cron.IntervalNumber, cron.IntervalType); err != nil {
		return Cron{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cron.ID = s.nextCronID
	s.nextCronID++
	if cron.NextCall.IsZero() {
		cron.NextCall = now
	}
	cron.Context = cloneContext(cron.Context)
	cron.CreatedAt = now
	cron.UpdatedAt = now
	s.crons[cron.ID] = cron
	return cloneCron(cron), nil
}

func (s *Scheduler) UpsertCron(cron Cron) (Cron, error) {
	if cron.ID == 0 {
		return s.AddCron(cron)
	}
	if cron.Name == "" {
		return Cron{}, errors.New("cron requires name")
	}
	if err := validateInterval(cron.IntervalNumber, cron.IntervalType); err != nil {
		return Cron{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	existing, exists := s.crons[cron.ID]
	if exists && !existing.CreatedAt.IsZero() {
		cron.CreatedAt = existing.CreatedAt
	} else if cron.CreatedAt.IsZero() {
		cron.CreatedAt = now
	}
	if cron.NextCall.IsZero() {
		cron.NextCall = now
	}
	cron.Context = cloneContext(cron.Context)
	cron.UpdatedAt = now
	s.crons[cron.ID] = cron
	if cron.ID >= s.nextCronID {
		s.nextCronID = cron.ID + 1
	}
	return cloneCron(cron), nil
}

func (s *Scheduler) Cron(id int64) (Cron, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cron, ok := s.crons[id]
	return cloneCron(cron), ok
}

func (s *Scheduler) AddTrigger(trigger Trigger) (Trigger, error) {
	if trigger.CronID == 0 {
		return Trigger{}, errors.New("trigger requires cron id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.crons[trigger.CronID]; !exists {
		return Trigger{}, fmt.Errorf("unknown cron %d", trigger.CronID)
	}
	return s.addTriggerLocked(trigger), nil
}

func (s *Scheduler) addTriggerLocked(trigger Trigger) Trigger {
	now := time.Now().UTC()
	trigger.ID = s.nextTriggerID
	s.nextTriggerID++
	if trigger.At.IsZero() {
		trigger.At = now
	}
	if trigger.Status == "" {
		trigger.Status = TriggerPending
	}
	trigger.CreatedAt = now
	trigger.UpdatedAt = now
	s.triggers[trigger.ID] = trigger
	return trigger
}

func (s *Scheduler) Triggers(cronID int64) []Trigger {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Trigger
	for _, trigger := range s.triggers {
		if trigger.CronID == cronID {
			out = append(out, trigger)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Scheduler) SetProgress(cronID int64, done int, remaining int) Progress {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	progress, exists := s.progress[cronID]
	if !exists {
		progress = Progress{ID: s.nextProgressID, CronID: cronID, StartedAt: now}
		s.nextProgressID++
	}
	if progress.StartedAt.IsZero() {
		progress.StartedAt = now
	}
	progress.Done = done
	progress.Remaining = remaining
	progress.UpdatedAt = now
	s.progress[cronID] = progress
	return progress
}

func (s *Scheduler) CommitProgress(cronID int64, processed int, remaining *int, deactivate bool) (Progress, error) {
	if processed < 0 {
		return Progress{}, errors.New("processed must be positive")
	}
	if remaining != nil && *remaining < 0 {
		return Progress{}, errors.New("remaining must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	progress, exists := s.progress[cronID]
	if !exists {
		progress = Progress{ID: s.nextProgressID, CronID: cronID, StartedAt: now}
		s.nextProgressID++
	}
	if progress.StartedAt.IsZero() {
		progress.StartedAt = now
	}
	progress.Done += processed
	if remaining != nil {
		progress.Remaining = *remaining
	} else if progress.Remaining > processed {
		progress.Remaining -= processed
	} else {
		progress.Remaining = 0
	}
	if deactivate {
		progress.Deactivate = true
	}
	progress.UpdatedAt = now
	s.progress[cronID] = progress
	return progress, nil
}

func (s *Scheduler) Progress(cronID int64) (Progress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress, ok := s.progress[cronID]
	return progress, ok
}

func (s *Scheduler) RunDue(now time.Time, action Action) Result {
	if action == nil {
		return Result{Errors: []error{errors.New("action is nil")}}
	}

	due, skipped := s.claimDue(now)
	result := Result{Ran: len(due), SkippedLocked: skipped}
	for _, claimed := range due {
		if s.failByTimeout(claimed.cron.ID, now) {
			s.finish(claimed, now, ErrCronTimedOut, &result)
			continue
		}
		s.startRunProgress(claimed.cron.ID, now)
		err := action(claimed.cron)
		s.finish(claimed, now, err, &result)
	}
	return result
}

func (s *Scheduler) RunDueActions(ctx context.Context, now time.Time, runner ActionRunner) Result {
	if runner == nil {
		return Result{Errors: []error{errors.New("action runner is nil")}}
	}
	return s.RunDue(now, func(cron Cron) error {
		exec := cronExecutionContext(cron, now)
		exec.CommitProgress = func(processed int, remaining *int, deactivate bool) bool {
			_, err := s.CommitProgress(cron.ID, processed, remaining, deactivate)
			return err == nil
		}
		if cron.ActionID != 0 {
			result, err := runner.Run(ctx, cron.ActionID, exec)
			if err == nil {
				s.commitActionResultProgress(cron.ID, result)
			}
			return err
		}
		if cron.ActionName != "" {
			named, ok := runner.(NamedActionRunner)
			if !ok {
				return ErrCronActionMissing
			}
			result, err := named.RunNamed(ctx, cron.ActionName, exec)
			if err == nil {
				s.commitActionResultProgress(cron.ID, result)
			}
			return err
		}
		return ErrCronActionMissing
	})
}

func (s *Scheduler) commitActionResultProgress(cronID int64, result actions.Result) {
	if result.Metadata == nil {
		return
	}
	_, hasDone := result.Metadata["cron_progress_done"]
	_, hasRemaining := result.Metadata["cron_progress_remaining"]
	_, hasDeactivate := result.Metadata["cron_progress_deactivate"]
	if !hasDone && !hasRemaining && !hasDeactivate {
		return
	}
	done := int(int64Value(result.Metadata["cron_progress_done"]))
	var remaining *int
	if hasRemaining {
		value := int(int64Value(result.Metadata["cron_progress_remaining"]))
		remaining = &value
	}
	_, _ = s.CommitProgress(cronID, done, remaining, boolValue(result.Metadata["cron_progress_deactivate"]))
}

func (s *Scheduler) LoadFromEnv(env *record.Env) error {
	snapshot, err := SnapshotFromEnv(env)
	if err != nil {
		return err
	}
	return s.Restore(snapshot)
}

func (s *Scheduler) SyncToEnv(env *record.Env) error {
	snapshot := s.Snapshot()
	for _, cron := range snapshot.Crons {
		values := map[string]any{
			"nextcall":           cron.NextCall,
			"lastcall":           cron.LastCall,
			"failure_count":      cron.FailureCount,
			"first_failure_date": cron.FirstFailureAt,
			"active":             cron.Active,
		}
		if err := env.Model("ir.cron").Browse(cron.ID).Write(values); err != nil {
			return err
		}
	}
	if err := s.syncTriggersToEnv(env, snapshot.Triggers); err != nil {
		return err
	}
	if err := s.syncProgressToEnv(env, snapshot.Progress); err != nil {
		return err
	}
	return nil
}

func (s *Scheduler) syncTriggersToEnv(env *record.Env, triggers []Trigger) error {
	for _, trigger := range triggers {
		exists, err := envRecordExists(env, "ir.cron.trigger", trigger.ID)
		if err != nil {
			return err
		}
		if trigger.Status != TriggerPending {
			if exists {
				if err := env.Model("ir.cron.trigger").Browse(trigger.ID).Unlink(); err != nil {
					return err
				}
			}
			continue
		}
		values := map[string]any{"cron_id": trigger.CronID, "call_at": trigger.At}
		if exists {
			if err := env.Model("ir.cron.trigger").Browse(trigger.ID).Write(values); err != nil {
				return err
			}
			continue
		}
		id, err := env.Model("ir.cron.trigger").Create(values)
		if err != nil {
			return err
		}
		s.setTriggerID(trigger.ID, id)
	}
	return nil
}

func (s *Scheduler) syncProgressToEnv(env *record.Env, progress []Progress) error {
	for _, item := range progress {
		values := map[string]any{
			"cron_id":           item.CronID,
			"done":              item.Done,
			"remaining":         item.Remaining,
			"deactivate":        item.Deactivate,
			"timed_out_counter": item.TimedOutCounter,
			"started_at":        item.StartedAt,
			"updated_at":        item.UpdatedAt,
		}
		id := item.ID
		exists, err := envRecordExists(env, "ir.cron.progress", id)
		if err != nil {
			return err
		}
		if !exists {
			id, exists, err = latestProgressIDFromEnv(env, item.CronID)
			if err != nil {
				return err
			}
		}
		if exists {
			if err := env.Model("ir.cron.progress").Browse(id).Write(values); err != nil {
				return err
			}
			s.setProgressID(item.CronID, id)
			continue
		}
		id, err = env.Model("ir.cron.progress").Create(values)
		if err != nil {
			return err
		}
		s.setProgressID(item.CronID, id)
	}
	return nil
}

func SnapshotFromEnv(env *record.Env) (Snapshot, error) {
	modelNames, err := cronModelNamesByID(env)
	if err != nil {
		return Snapshot{}, err
	}
	crons, err := cronsFromEnv(env, modelNames)
	if err != nil {
		return Snapshot{}, err
	}
	triggers, err := triggersFromEnv(env)
	if err != nil {
		return Snapshot{}, err
	}
	progress, err := progressFromEnv(env)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Crons: crons, Triggers: triggers, Progress: progress}
	for _, cron := range crons {
		if cron.ID >= snapshot.NextCronID {
			snapshot.NextCronID = cron.ID + 1
		}
	}
	for _, trigger := range triggers {
		if trigger.ID >= snapshot.NextTriggerID {
			snapshot.NextTriggerID = trigger.ID + 1
		}
	}
	for _, item := range progress {
		if item.ID >= snapshot.NextProgressID {
			snapshot.NextProgressID = item.ID + 1
		}
	}
	if snapshot.NextCronID == 0 {
		snapshot.NextCronID = 1
	}
	if snapshot.NextTriggerID == 0 {
		snapshot.NextTriggerID = 1
	}
	if snapshot.NextProgressID == 0 {
		snapshot.NextProgressID = 1
	}
	return snapshot, nil
}

func (s *Scheduler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Snapshot{
		NextCronID:     s.nextCronID,
		NextTriggerID:  s.nextTriggerID,
		NextProgressID: s.nextProgressID,
		Crons:          make([]Cron, 0, len(s.crons)),
		Triggers:       make([]Trigger, 0, len(s.triggers)),
		Progress:       make([]Progress, 0, len(s.progress)),
	}
	cronIDs := make([]int64, 0, len(s.crons))
	for id := range s.crons {
		cronIDs = append(cronIDs, id)
	}
	sort.Slice(cronIDs, func(i, j int) bool { return cronIDs[i] < cronIDs[j] })
	for _, id := range cronIDs {
		snapshot.Crons = append(snapshot.Crons, cloneCron(s.crons[id]))
	}
	triggerIDs := make([]int64, 0, len(s.triggers))
	for id := range s.triggers {
		triggerIDs = append(triggerIDs, id)
	}
	sort.Slice(triggerIDs, func(i, j int) bool { return triggerIDs[i] < triggerIDs[j] })
	for _, id := range triggerIDs {
		snapshot.Triggers = append(snapshot.Triggers, cloneTrigger(s.triggers[id]))
	}
	progressIDs := make([]int64, 0, len(s.progress))
	for id := range s.progress {
		progressIDs = append(progressIDs, id)
	}
	sort.Slice(progressIDs, func(i, j int) bool { return progressIDs[i] < progressIDs[j] })
	for _, id := range progressIDs {
		snapshot.Progress = append(snapshot.Progress, s.progress[id])
	}
	return snapshot
}

func envRecordExists(env *record.Env, modelName string, id int64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	rows, err := env.Model(modelName).Browse(id).Read("id")
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func latestProgressIDFromEnv(env *record.Env, cronID int64) (int64, bool, error) {
	found, err := env.Model("ir.cron.progress").Search(domain.Cond("cron_id", domain.Equal, cronID))
	if err != nil {
		return 0, false, err
	}
	rows, err := found.Read("cron_id")
	if err != nil {
		return 0, false, err
	}
	var latest int64
	for _, row := range rows {
		id := int64Value(row["id"])
		if id > latest {
			latest = id
		}
	}
	return latest, latest != 0, nil
}

func (s *Scheduler) setProgressID(cronID int64, id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress, ok := s.progress[cronID]
	if !ok {
		return
	}
	progress.ID = id
	s.progress[cronID] = progress
	if id >= s.nextProgressID {
		s.nextProgressID = id + 1
	}
}

func (s *Scheduler) setTriggerID(oldID int64, newID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	trigger, ok := s.triggers[oldID]
	if !ok {
		return
	}
	delete(s.triggers, oldID)
	trigger.ID = newID
	s.triggers[newID] = trigger
	if newID >= s.nextTriggerID {
		s.nextTriggerID = newID + 1
	}
}

func (s *Scheduler) Restore(snapshot Snapshot) error {
	crons := map[int64]Cron{}
	triggers := map[int64]Trigger{}
	progress := map[int64]Progress{}
	nextCronID := snapshot.NextCronID
	nextTriggerID := snapshot.NextTriggerID
	nextProgressID := snapshot.NextProgressID
	for _, cron := range snapshot.Crons {
		if cron.ID == 0 {
			return errors.New("snapshot cron requires id")
		}
		if cron.Name == "" {
			return errors.New("snapshot cron requires name")
		}
		if err := validateInterval(cron.IntervalNumber, cron.IntervalType); err != nil {
			return err
		}
		crons[cron.ID] = cloneCron(cron)
		if cron.ID >= nextCronID {
			nextCronID = cron.ID + 1
		}
	}
	for _, trigger := range snapshot.Triggers {
		if trigger.ID == 0 {
			return errors.New("snapshot trigger requires id")
		}
		if trigger.CronID == 0 {
			return errors.New("snapshot trigger requires cron id")
		}
		if _, ok := crons[trigger.CronID]; !ok {
			return fmt.Errorf("snapshot trigger references unknown cron %d", trigger.CronID)
		}
		if trigger.Status == "" {
			trigger.Status = TriggerPending
		}
		triggers[trigger.ID] = cloneTrigger(trigger)
		if trigger.ID >= nextTriggerID {
			nextTriggerID = trigger.ID + 1
		}
	}
	for _, item := range snapshot.Progress {
		if item.CronID == 0 {
			return errors.New("snapshot progress requires cron id")
		}
		if _, ok := crons[item.CronID]; !ok {
			return fmt.Errorf("snapshot progress references unknown cron %d", item.CronID)
		}
		progress[item.CronID] = item
		if item.ID >= nextProgressID {
			nextProgressID = item.ID + 1
		}
	}
	if nextCronID <= 0 {
		nextCronID = 1
	}
	if nextTriggerID <= 0 {
		nextTriggerID = 1
	}
	if nextProgressID <= 0 {
		nextProgressID = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextCronID = nextCronID
	s.nextTriggerID = nextTriggerID
	s.nextProgressID = nextProgressID
	s.crons = crons
	s.triggers = triggers
	s.progress = progress
	s.running = map[int64]bool{}
	return nil
}

type claimedCron struct {
	cron       Cron
	triggerIDs []int64
}

func (s *Scheduler) claimDue(now time.Time) ([]claimedCron, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []claimedCron
	skipped := 0
	ids := make([]int64, 0, len(s.crons))
	for id := range s.crons {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left := s.crons[ids[i]]
		right := s.crons[ids[j]]
		if left.FailureCount != right.FailureCount {
			return left.FailureCount < right.FailureCount
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})

	for _, id := range ids {
		cron := s.crons[id]
		if !cron.Active {
			continue
		}
		triggerIDs := s.dueTriggerIDsLocked(id, now)
		if cron.NextCall.After(now) && len(triggerIDs) == 0 {
			continue
		}
		if s.running[id] {
			skipped++
			continue
		}
		s.running[id] = true
		due = append(due, claimedCron{cron: cloneCron(cron), triggerIDs: triggerIDs})
	}
	return due, skipped
}

func (s *Scheduler) failByTimeout(cronID int64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress, ok := s.progress[cronID]
	if !ok || progress.TimedOutCounter < 3 || progress.Done != 0 {
		return false
	}
	progress.TimedOutCounter = 0
	progress.UpdatedAt = now
	s.progress[cronID] = progress
	return true
}

func (s *Scheduler) startRunProgress(cronID int64, now time.Time) Progress {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.progress[cronID]
	progress := Progress{
		ID:              s.nextProgressID,
		CronID:          cronID,
		TimedOutCounter: previous.TimedOutCounter + 1,
		StartedAt:       now,
		UpdatedAt:       now,
	}
	s.nextProgressID++
	s.progress[cronID] = progress
	return progress
}

func (s *Scheduler) finish(claimed claimedCron, now time.Time, err error, result *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	defer delete(s.running, claimed.cron.ID)

	cron, ok := s.crons[claimed.cron.ID]
	if !ok {
		return
	}
	cron.UpdatedAt = now
	if err == nil {
		progress := s.progress[cron.ID]
		if progress.CronID != 0 {
			progress.TimedOutCounter = 0
			progress.UpdatedAt = now
			s.progress[cron.ID] = progress
		}
		cron.FailureCount = 0
		cron.FirstFailureAt = time.Time{}
		cron.LastError = ""
		if progress.CronID != 0 && progress.Remaining > 0 {
			cron.LastCall = now
			s.addTriggerLocked(Trigger{CronID: cron.ID, At: now, Status: TriggerPending})
		} else {
			cron.LastCall = now
			cron.NextCall = advance(cron.NextCall, cron.IntervalNumber, cron.IntervalType, now)
			if progress.CronID != 0 && progress.Deactivate {
				cron.Active = false
			}
		}
		result.Succeeded++
		for _, id := range claimed.triggerIDs {
			trigger := s.triggers[id]
			trigger.Status = TriggerDone
			trigger.LastError = ""
			trigger.UpdatedAt = now
			s.triggers[id] = trigger
		}
		s.crons[cron.ID] = cron
		return
	}

	cron.FailureCount++
	if cron.FirstFailureAt.IsZero() {
		cron.FirstFailureAt = now
	}
	cron.LastError = err.Error()
	cron.NextCall = now.Add(s.retryBackoff(cron))
	result.Failed++
	result.Errors = append(result.Errors, fmt.Errorf("cron %s failed: %w", cron.Name, err))
	if progress := s.progress[cron.ID]; progress.CronID != 0 && err != ErrCronTimedOut {
		progress.TimedOutCounter = 0
		progress.UpdatedAt = now
		s.progress[cron.ID] = progress
	}
	for _, id := range claimed.triggerIDs {
		trigger := s.triggers[id]
		trigger.Status = TriggerFailed
		trigger.LastError = err.Error()
		trigger.UpdatedAt = now
		s.triggers[id] = trigger
	}
	s.crons[cron.ID] = cron
}

func cronExecutionContext(cron Cron, now time.Time) actions.ExecutionContext {
	metadata := cloneContext(cron.Context)
	metadata["cron_id"] = cron.ID
	metadata["cron_name"] = cron.Name
	return actions.ExecutionContext{
		Model:        cron.Model,
		UserID:       cron.UserID,
		Trigger:      "cron",
		Now:          now,
		LastRunAt:    firstTime(cron.LastCall, cron.NextCall),
		CurrentRunAt: now,
		Values:       cloneContext(cron.Context),
		Metadata:     metadata,
	}
}

func cronModelNamesByID(env *record.Env) (map[int64]string, error) {
	found, err := env.Model("ir.model").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("model")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		name, _ := row["model"].(string)
		if id != 0 && name != "" {
			out[id] = name
		}
	}
	return out, nil
}

func cronsFromEnv(env *record.Env, modelNames map[int64]string) ([]Cron, error) {
	env = schedulerActiveTestDisabledEnv(env)
	found, err := env.Model("ir.cron").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "cron_name", "ir_actions_server_id", "active", "interval_number", "interval_type", "nextcall", "lastcall", "user_id", "model_id", "state", "code", "priority", "failure_count", "first_failure_date", "action_name")
	if err != nil {
		return nil, err
	}
	crons := make([]Cron, 0, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		nextCall, err := timeValue(row["nextcall"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron %d nextcall: %w", id, err)
		}
		lastCall, err := timeValue(row["lastcall"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron %d lastcall: %w", id, err)
		}
		firstFailure, err := timeValue(row["first_failure_date"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron %d first_failure_date: %w", id, err)
		}
		actionName := stringValue(row["action_name"])
		code := stringValue(row["code"])
		if actionName == "" {
			actionName = code
		}
		name := firstNonEmpty(stringValue(row["name"]), stringValue(row["cron_name"]))
		crons = append(crons, Cron{
			ID:             id,
			Name:           name,
			Active:         boolValue(row["active"]),
			UserID:         int64Value(row["user_id"]),
			Model:          modelNames[int64Value(row["model_id"])],
			ActionID:       int64Value(row["ir_actions_server_id"]),
			ActionName:     actionName,
			Code:           code,
			IntervalNumber: int(int64Value(row["interval_number"])),
			IntervalType:   IntervalType(stringValue(row["interval_type"])),
			NextCall:       nextCall,
			LastCall:       lastCall,
			Priority:       int(int64Value(row["priority"])),
			FailureCount:   int(int64Value(row["failure_count"])),
			FirstFailureAt: firstFailure,
		})
	}
	return crons, nil
}

func schedulerActiveTestDisabledEnv(env *record.Env) *record.Env {
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	return env.WithContext(ctx)
}

func triggersFromEnv(env *record.Env) ([]Trigger, error) {
	found, err := env.Model("ir.cron.trigger").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("cron_id", "call_at")
	if err != nil {
		return nil, err
	}
	triggers := make([]Trigger, 0, len(rows))
	for _, row := range rows {
		at, err := timeValue(row["call_at"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron.trigger %d call_at: %w", int64Value(row["id"]), err)
		}
		triggers = append(triggers, Trigger{
			ID:     int64Value(row["id"]),
			CronID: int64Value(row["cron_id"]),
			At:     at,
			Status: TriggerPending,
		})
	}
	return triggers, nil
}

func progressFromEnv(env *record.Env) ([]Progress, error) {
	found, err := env.Model("ir.cron.progress").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("cron_id", "done", "remaining", "deactivate", "timed_out_counter", "started_at", "updated_at")
	if err != nil {
		return nil, err
	}
	latest := map[int64]Progress{}
	for _, row := range rows {
		id := int64Value(row["id"])
		cronID := int64Value(row["cron_id"])
		startedAt, err := timeValue(row["started_at"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron.progress %d started_at: %w", id, err)
		}
		updatedAt, err := timeValue(row["updated_at"])
		if err != nil {
			return nil, fmt.Errorf("ir.cron.progress %d updated_at: %w", id, err)
		}
		item := Progress{
			ID:              id,
			CronID:          cronID,
			Done:            int(int64Value(row["done"])),
			Remaining:       int(int64Value(row["remaining"])),
			Deactivate:      boolValue(row["deactivate"]),
			TimedOutCounter: int(int64Value(row["timed_out_counter"])),
			StartedAt:       startedAt,
			UpdatedAt:       updatedAt,
		}
		if existing, ok := latest[cronID]; !ok || item.ID > existing.ID {
			latest[cronID] = item
		}
	}
	progress := make([]Progress, 0, len(latest))
	for _, item := range latest {
		progress = append(progress, item)
	}
	sort.Slice(progress, func(i, j int) bool { return progress[i].CronID < progress[j].CronID })
	return progress, nil
}

func timeValue(value any) (time.Time, error) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return typed, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}, nil
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"} {
			parsed, err := time.ParseInLocation(layout, text, time.UTC)
			if err == nil {
				return parsed.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("unsupported time %q", text)
	default:
		return time.Time{}, fmt.Errorf("unsupported time value %T", value)
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return id
	default:
		return 0
	}
}

func (s *Scheduler) dueTriggerIDsLocked(cronID int64, now time.Time) []int64 {
	var ids []int64
	for id, trigger := range s.triggers {
		if trigger.CronID == cronID && trigger.Status == TriggerPending && !trigger.At.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func validateInterval(number int, intervalType IntervalType) error {
	if number <= 0 {
		return errors.New("interval number must be positive")
	}
	switch intervalType {
	case IntervalMinutes, IntervalHours, IntervalDays, IntervalWeeks, IntervalMonths:
		return nil
	default:
		return fmt.Errorf("unsupported interval type %q", intervalType)
	}
}

func advance(next time.Time, number int, intervalType IntervalType, now time.Time) time.Time {
	if next.IsZero() || next.After(now) {
		return next
	}
	for !next.After(now) {
		switch intervalType {
		case IntervalMinutes:
			next = next.Add(time.Duration(number) * time.Minute)
		case IntervalHours:
			next = next.Add(time.Duration(number) * time.Hour)
		case IntervalDays:
			next = next.AddDate(0, 0, number)
		case IntervalWeeks:
			next = next.AddDate(0, 0, number*7)
		case IntervalMonths:
			next = next.AddDate(0, number, 0)
		}
	}
	return next
}

func defaultRetryBackoff(cron Cron) time.Duration {
	if cron.FailureCount <= 0 {
		return time.Minute
	}
	return time.Duration(cron.FailureCount) * time.Minute
}

func cloneCron(cron Cron) Cron {
	cron.Context = cloneContext(cron.Context)
	return cron
}

func cloneTrigger(trigger Trigger) Trigger {
	return trigger
}

func cloneContext(context map[string]any) map[string]any {
	if context == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(context))
	for key, value := range context {
		out[key] = value
	}
	return out
}
