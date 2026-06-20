package queue

import (
	"errors"
	"testing"
	"time"
)

func TestEnqueueSuppressesDuplicateByKey(t *testing.T) {
	q := New()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)

	first, created, err := q.Enqueue(Job{Key: "mail:1", Name: "send mail", NextRunAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first enqueue was not created")
	}

	second, created, err := q.Enqueue(Job{Key: "mail:1", Name: "send duplicate", NextRunAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate enqueue created a new job")
	}
	if second.ID != first.ID || second.Name != first.Name {
		t.Fatalf("duplicate returned %#v, want original %#v", second, first)
	}

	jobs := q.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
}

func TestRunDueRetriesAndDeadLetters(t *testing.T) {
	q := New(WithBackoff(func(Job) time.Duration { return time.Second }))
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)

	job, _, err := q.Enqueue(Job{Key: "sync:1", Name: "sync", MaxAttempts: 2, NextRunAt: now})
	if err != nil {
		t.Fatal(err)
	}

	fail := errors.New("temporary failure")
	result := q.RunDue(now, func(Job) error { return fail })
	if result.Ran != 1 || result.Retried != 1 || result.Dead != 0 {
		t.Fatalf("first result = %#v", result)
	}

	afterFirst, ok := q.Get(job.ID)
	if !ok {
		t.Fatal("job missing")
	}
	if afterFirst.Status != StatusPending || afterFirst.Attempts != 1 || !afterFirst.NextRunAt.Equal(now.Add(time.Second)) {
		t.Fatalf("after first run = %#v", afterFirst)
	}

	result = q.RunDue(now.Add(time.Second), func(Job) error { return fail })
	if result.Ran != 1 || result.Retried != 0 || result.Dead != 1 {
		t.Fatalf("second result = %#v", result)
	}

	afterSecond, ok := q.Get(job.ID)
	if !ok {
		t.Fatal("job missing")
	}
	if afterSecond.Status != StatusDead || afterSecond.Attempts != 2 || afterSecond.LastError != fail.Error() {
		t.Fatalf("after second run = %#v", afterSecond)
	}
	if dead := q.DeadLetters(); len(dead) != 1 || dead[0].ID != job.ID {
		t.Fatalf("dead letters = %#v", dead)
	}
}
