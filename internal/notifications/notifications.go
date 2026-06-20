package notifications

import (
	"fmt"
	"sync"
	"time"
)

type ActivityStatus string

const (
	ActivityOpen     ActivityStatus = "open"
	ActivityDone     ActivityStatus = "done"
	ActivityCanceled ActivityStatus = "canceled"
)

type ActivityType struct {
	ID      int64
	Name    string
	Summary string
}

type Activity struct {
	ID         int64
	TypeID     int64
	Model      string
	RecordID   int64
	UserID     int64
	Summary    string
	Note       string
	Deadline   time.Time
	Status     ActivityStatus
	CreatedAt  time.Time
	DoneAt     time.Time
	CanceledAt time.Time
}

func NewActivity(activity Activity, now time.Time) Activity {
	if activity.Status == "" {
		activity.Status = ActivityOpen
	}
	if activity.CreatedAt.IsZero() {
		activity.CreatedAt = now
	}
	return activity
}

func (a *Activity) MarkDone(now time.Time) error {
	if a.Status == ActivityCanceled {
		return fmt.Errorf("activity canceled")
	}
	if a.Status == ActivityDone {
		return nil
	}
	a.Status = ActivityDone
	a.DoneAt = now
	return nil
}

func (a *Activity) Cancel(now time.Time) error {
	if a.Status == ActivityDone {
		return fmt.Errorf("activity done")
	}
	if a.Status == ActivityCanceled {
		return nil
	}
	a.Status = ActivityCanceled
	a.CanceledAt = now
	return nil
}

type Notification struct {
	ID        int64
	Channel   string
	UserID    int64
	Subject   string
	Body      string
	Read      bool
	CreatedAt time.Time
}

type Event struct {
	ID        int64
	Channel   string
	Name      string
	Payload   map[string]any
	CreatedAt time.Time
}

type Subscription struct {
	Channel string
	Events  <-chan Event
	cancel  func()
}

func (s Subscription) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

type Bus struct {
	mu          sync.Mutex
	nextID      int64
	bufferLimit int
	events      map[string][]Event
	subscribers map[string]map[int]chan Event
	nextSubID   int
}

func NewBus(bufferLimit int) *Bus {
	if bufferLimit <= 0 {
		bufferLimit = 100
	}
	return &Bus{
		nextID:      1,
		bufferLimit: bufferLimit,
		events:      map[string][]Event{},
		subscribers: map[string]map[int]chan Event{},
	}
}

func (b *Bus) Publish(channel, name string, payload map[string]any, now time.Time) Event {
	b.mu.Lock()
	event := Event{
		ID:        b.nextID,
		Channel:   channel,
		Name:      name,
		Payload:   clonePayload(payload),
		CreatedAt: now,
	}
	b.nextID++
	b.events[channel] = append(b.events[channel], event)
	if len(b.events[channel]) > b.bufferLimit {
		b.events[channel] = b.events[channel][len(b.events[channel])-b.bufferLimit:]
	}
	var subscribers []chan Event
	for _, ch := range b.subscribers[channel] {
		subscribers = append(subscribers, ch)
	}
	b.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
	return event
}

func (b *Bus) LastID() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextID - 1
}

func (b *Bus) Subscribe(channel string, afterID int64) Subscription {
	ch := make(chan Event, b.bufferLimit)
	b.mu.Lock()
	b.nextSubID++
	id := b.nextSubID
	if b.subscribers[channel] == nil {
		b.subscribers[channel] = map[int]chan Event{}
	}
	b.subscribers[channel][id] = ch
	replay := append([]Event(nil), b.events[channel]...)
	b.mu.Unlock()

	for _, event := range replay {
		if event.ID > afterID {
			ch <- event
		}
	}
	return Subscription{
		Channel: channel,
		Events:  ch,
		cancel: func() {
			b.mu.Lock()
			if subs := b.subscribers[channel]; subs != nil {
				delete(subs, id)
				if len(subs) == 0 {
					delete(b.subscribers, channel)
				}
			}
			b.mu.Unlock()
			close(ch)
		},
	}
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}
