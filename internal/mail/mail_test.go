package mail

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTemplateRenderEscapesSubstitutions(t *testing.T) {
	tpl := Template{
		To:      "{{ email }}",
		CC:      "{{ manager_email }}",
		Subject: "Hello {{ name }}",
		Body:    "<p>{{ name }}</p><span>{{ missing }}</span>",
	}
	rendered := tpl.Render(map[string]any{
		"email":         "user@example.com",
		"manager_email": "manager@example.com",
		"name":          `<script>alert("x")</script>`,
	})

	if rendered.To != "user@example.com" {
		t.Fatalf("to = %q", rendered.To)
	}
	if rendered.CC != "manager@example.com" {
		t.Fatalf("cc = %q", rendered.CC)
	}
	if strings.Contains(rendered.Subject, "<script>") || !strings.Contains(rendered.Subject, "&lt;script&gt;") {
		t.Fatalf("subject not escaped: %q", rendered.Subject)
	}
	if strings.Contains(rendered.Body, "<script>") || !strings.Contains(rendered.Body, "&lt;script&gt;") {
		t.Fatalf("body not escaped: %q", rendered.Body)
	}
	if !strings.Contains(rendered.Body, "<span></span>") {
		t.Fatalf("missing key not blanked: %q", rendered.Body)
	}
}

func TestEnqueueTemplateRendersIntoOutbox(t *testing.T) {
	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	outbox := NewOutbox()
	id, err := EnqueueTemplate(outbox, Template{
		To:      "{{ email }}",
		CC:      "{{ cc }}",
		Subject: "Delegation {{ state }}",
		Body:    "<p>{{ name }}</p>",
	}, map[string]any{
		"email": "delegate@example.com",
		"cc":    "owner@example.com",
		"state": "assigned",
		"name":  "Delegate",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	message, ok := outbox.Get(id)
	if !ok {
		t.Fatal("message missing")
	}
	if message.To != "delegate@example.com" || message.CC != "owner@example.com" || message.Subject != "Delegation assigned" || message.Body != "<p>Delegate</p>" {
		t.Fatalf("message = %+v", message)
	}
}

func TestOutboxSuccessfulSend(t *testing.T) {
	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	outbox := NewOutbox()
	id, err := outbox.Enqueue(Message{To: "user@example.com", Subject: "S", Body: "B"}, now)
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{}
	result := outbox.SendDue(sender, now)
	if result.Sent != 1 || result.Retried != 0 || result.Dead != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(sender.sent) != 1 || sender.sent[0].ID != id {
		t.Fatalf("sent = %+v", sender.sent)
	}
	message, ok := outbox.Get(id)
	if !ok {
		t.Fatal("message missing")
	}
	if message.Status != MessageSent || message.SentAt != now || message.LastError != "" {
		t.Fatalf("message = %+v", message)
	}
}

func TestOutboxRetryAndDeadLetter(t *testing.T) {
	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	outbox := NewOutbox()
	outbox.SetBackoff(func(int) time.Duration { return time.Second })
	id, err := outbox.Enqueue(Message{To: "user@example.com", MaxAttempts: 2}, now)
	if err != nil {
		t.Fatal(err)
	}

	sender := failingSender{err: errors.New("smtp password secret leaked")}
	result := outbox.SendDue(sender, now)
	if result.Retried != 1 || result.Dead != 0 {
		t.Fatalf("first result = %+v", result)
	}
	message, _ := outbox.Get(id)
	if message.Status != MessagePending || message.Attempts != 1 || message.NextAttemptAt != now.Add(time.Second) {
		t.Fatalf("after retry = %+v", message)
	}
	if strings.Contains(message.LastError, "password") || strings.Contains(message.LastError, "secret") {
		t.Fatalf("unsafe error stored: %q", message.LastError)
	}

	result = outbox.SendDue(sender, now.Add(time.Second))
	if result.Dead != 1 || result.Retried != 0 {
		t.Fatalf("second result = %+v", result)
	}
	message, _ = outbox.Get(id)
	if message.Status != MessageDead || message.Attempts != 2 || message.DeadAt != now.Add(time.Second) {
		t.Fatalf("after dead letter = %+v", message)
	}
	if strings.Contains(message.LastError, "password") || strings.Contains(message.LastError, "secret") {
		t.Fatalf("unsafe dead-letter error stored: %q", message.LastError)
	}
}

type recordingSender struct {
	sent []Message
}

func (s *recordingSender) Send(message Message) error {
	s.sent = append(s.sent, message)
	return nil
}

type failingSender struct {
	err error
}

func (s failingSender) Send(Message) error {
	return s.err
}
