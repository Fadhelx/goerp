package audit

import (
	"strings"
	"time"
)

type Event struct {
	At               time.Time
	UserID           int64
	CompanyID        int64
	AgentID          int64
	PromptID         int64
	Model            string
	InputTokens      int
	OutputTokens     int
	LatencyMillis    int64
	Tools            []string
	PermissionResult string
	Error            string
	Details          map[string]string
}

type Log struct {
	events []Event
}

func NewLog() *Log {
	return &Log{}
}

func (l *Log) Append(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.Tools = append([]string(nil), event.Tools...)
	event.Details = redactDetails(event.Details)
	l.events = append(l.events, event)
}

func (l *Log) Events() []Event {
	out := make([]Event, len(l.events))
	for idx, event := range l.events {
		event.Tools = append([]string(nil), event.Tools...)
		event.Details = redactDetails(event.Details)
		out[idx] = event
	}
	return out
}

func redactDetails(details map[string]string) map[string]string {
	if details == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(details))
	for key, value := range details {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
			out[key] = "[redacted]"
			continue
		}
		out[key] = value
	}
	return out
}
