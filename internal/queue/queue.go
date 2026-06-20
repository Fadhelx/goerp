package queue

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusDead    Status = "dead"
)

const defaultMaxAttempts = 3

type Job struct {
	ID          int64
	Key         string
	Name        string
	Payload     map[string]any
	Status      Status
	Attempts    int
	MaxAttempts int
	NextRunAt   time.Time
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FinishedAt  time.Time
}

type Worker func(Job) error

type BackoffFunc func(Job) time.Duration

type Option func(*Queue)

func WithBackoff(backoff BackoffFunc) Option {
	return func(q *Queue) {
		if backoff != nil {
			q.backoff = backoff
		}
	}
}

type Queue struct {
	mu      sync.Mutex
	nextID  int64
	jobs    map[int64]Job
	byKey   map[string]int64
	backoff BackoffFunc
}

type Result struct {
	Ran       int
	Succeeded int
	Retried   int
	Dead      int
	Errors    []error
}

func New(options ...Option) *Queue {
	q := &Queue{
		nextID:  1,
		jobs:    map[int64]Job{},
		byKey:   map[string]int64{},
		backoff: defaultBackoff,
	}
	for _, option := range options {
		option(q)
	}
	return q
}

func (q *Queue) Enqueue(job Job) (Job, bool, error) {
	if job.Key == "" {
		return Job{}, false, errors.New("job requires key")
	}
	if job.Name == "" {
		return Job{}, false, errors.New("job requires name")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if id, exists := q.byKey[job.Key]; exists {
		return cloneJob(q.jobs[id]), false, nil
	}

	now := time.Now().UTC()
	job.ID = q.nextID
	q.nextID++
	job.Status = StatusPending
	job.Attempts = 0
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultMaxAttempts
	}
	if job.NextRunAt.IsZero() {
		job.NextRunAt = now
	}
	job.Payload = clonePayload(job.Payload)
	job.CreatedAt = now
	job.UpdatedAt = now
	q.jobs[job.ID] = job
	q.byKey[job.Key] = job.ID

	return cloneJob(job), true, nil
}

func (q *Queue) Get(id int64) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	return cloneJob(job), ok
}

func (q *Queue) GetByKey(key string) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	id, ok := q.byKey[key]
	if !ok {
		return Job{}, false
	}
	return cloneJob(q.jobs[id]), true
}

func (q *Queue) Jobs(statuses ...Status) []Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	filter := map[Status]bool{}
	for _, status := range statuses {
		filter[status] = true
	}
	out := make([]Job, 0, len(q.jobs))
	for _, job := range q.jobs {
		if len(filter) > 0 && !filter[job.Status] {
			continue
		}
		out = append(out, cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (q *Queue) DeadLetters() []Job {
	return q.Jobs(StatusDead)
}

func (q *Queue) RunDue(now time.Time, worker Worker) Result {
	if worker == nil {
		return Result{Errors: []error{errors.New("worker is nil")}}
	}

	due := q.claimDue(now)
	result := Result{Ran: len(due)}
	for _, job := range due {
		err := worker(job)
		q.finish(job.ID, now, err, &result)
	}
	return result
}

func (q *Queue) claimDue(now time.Time) []Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	var due []Job
	for id, job := range q.jobs {
		if job.Status != StatusPending || job.NextRunAt.After(now) {
			continue
		}
		job.Status = StatusRunning
		job.Attempts++
		job.UpdatedAt = now
		q.jobs[id] = job
		due = append(due, cloneJob(job))
	}
	sort.Slice(due, func(i, j int) bool { return due[i].ID < due[j].ID })
	return due
}

func (q *Queue) finish(id int64, now time.Time, err error, result *Result) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return
	}
	job.UpdatedAt = now
	if err == nil {
		job.Status = StatusDone
		job.LastError = ""
		job.FinishedAt = now
		result.Succeeded++
		q.jobs[id] = job
		return
	}

	job.LastError = err.Error()
	if job.Attempts >= job.MaxAttempts {
		job.Status = StatusDead
		job.FinishedAt = now
		result.Dead++
		result.Errors = append(result.Errors, fmt.Errorf("job %s dead after %d attempts: %w", job.Key, job.Attempts, err))
		q.jobs[id] = job
		return
	}

	job.Status = StatusPending
	job.NextRunAt = now.Add(q.backoff(job))
	result.Retried++
	result.Errors = append(result.Errors, fmt.Errorf("job %s attempt %d failed: %w", job.Key, job.Attempts, err))
	q.jobs[id] = job
}

func defaultBackoff(job Job) time.Duration {
	if job.Attempts <= 0 {
		return time.Minute
	}
	return time.Duration(job.Attempts) * time.Minute
}

func cloneJob(job Job) Job {
	job.Payload = clonePayload(job.Payload)
	return job
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
