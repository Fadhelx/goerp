package notifications

import (
	"testing"
	"time"
)

func TestActivityDoneAndCancel(t *testing.T) {
	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	activity := NewActivity(Activity{
		ID:       1,
		TypeID:   2,
		Model:    "res.partner",
		RecordID: 10,
		UserID:   20,
		Deadline: now.Add(24 * time.Hour),
	}, now)
	if activity.Status != ActivityOpen || activity.CreatedAt != now {
		t.Fatalf("activity = %+v", activity)
	}
	if err := activity.MarkDone(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if activity.Status != ActivityDone || activity.DoneAt != now.Add(time.Hour) {
		t.Fatalf("done activity = %+v", activity)
	}
	if err := activity.Cancel(now.Add(2 * time.Hour)); err == nil {
		t.Fatal("expected done activity cancel to fail")
	}

	activity = NewActivity(Activity{ID: 2}, now)
	if err := activity.Cancel(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if activity.Status != ActivityCanceled || activity.CanceledAt != now.Add(time.Hour) {
		t.Fatalf("canceled activity = %+v", activity)
	}
	if err := activity.MarkDone(now.Add(2 * time.Hour)); err == nil {
		t.Fatal("expected canceled activity done to fail")
	}
}

func TestBusPublishSubscribeReplay(t *testing.T) {
	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	bus := NewBus(10)
	first := bus.Publish("mail.channel/1", "message", map[string]any{"body": "first"}, now)
	sub := bus.Subscribe("mail.channel/1", 0)
	defer sub.Close()

	got := readEvent(t, sub.Events)
	if got.ID != first.ID || got.Payload["body"] != "first" {
		t.Fatalf("replay event = %+v", got)
	}

	second := bus.Publish("mail.channel/1", "message", map[string]any{"body": "second"}, now.Add(time.Second))
	got = readEvent(t, sub.Events)
	if got.ID != second.ID || got.Payload["body"] != "second" {
		t.Fatalf("published event = %+v", got)
	}

	later := bus.Subscribe("mail.channel/1", first.ID)
	defer later.Close()
	got = readEvent(t, later.Events)
	if got.ID != second.ID {
		t.Fatalf("after replay event = %+v", got)
	}
}

func readEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	return Event{}
}
