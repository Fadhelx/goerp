package mail

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"
)

type MessageStatus string

const (
	MessagePending MessageStatus = "pending"
	MessageSent    MessageStatus = "sent"
	MessageDead    MessageStatus = "dead"
)

type Template struct {
	ID                 int64
	Name               string
	To                 string
	CC                 string
	EmailFrom          string
	ReplyTo            string
	PartnerTo          string
	ScheduledDate      time.Time
	Subject            string
	Body               string
	AttachmentIDs      []int64
	ReportTemplateIDs  []int64
	DelegationGroupIDs []int64
}

type RenderedMessage struct {
	To      string
	CC      string
	Subject string
	Body    string
}

func (t Template) Render(values map[string]any) RenderedMessage {
	return RenderedMessage{
		To:      renderText(t.To, values),
		CC:      renderText(t.CC, values),
		Subject: renderText(t.Subject, values),
		Body:    renderText(t.Body, values),
	}
}

func EnqueueTemplate(outbox *Outbox, template Template, values map[string]any, now time.Time) (int64, error) {
	if outbox == nil {
		return 0, fmt.Errorf("outbox unavailable")
	}
	rendered := template.Render(values)
	return outbox.Enqueue(Message{
		To:      rendered.To,
		CC:      rendered.CC,
		Subject: rendered.Subject,
		Body:    rendered.Body,
	}, now)
}

type Message struct {
	ID            int64
	From          string
	EnvelopeFrom  string
	To            string
	CC            string
	ReplyTo       string
	Subject       string
	Body          string
	Headers       map[string]string
	Attachments   []Attachment
	Status        MessageStatus
	Attempts      int
	MaxAttempts   int
	NextAttemptAt time.Time
	CreatedAt     time.Time
	SentAt        time.Time
	DeadAt        time.Time
	LastError     string
}

type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

type Sender interface {
	Send(Message) error
}

type Outbox struct {
	mu          sync.Mutex
	nextID      int64
	messages    map[int64]Message
	backoff     func(int) time.Duration
	maxAttempts int
}

type SendResult struct {
	Sent    int
	Retried int
	Dead    int
}

func NewOutbox() *Outbox {
	return &Outbox{
		nextID:      1,
		messages:    map[int64]Message{},
		backoff:     defaultBackoff,
		maxAttempts: 3,
	}
}

func (o *Outbox) SetBackoff(backoff func(int) time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if backoff == nil {
		o.backoff = defaultBackoff
		return
	}
	o.backoff = backoff
}

func (o *Outbox) Enqueue(message Message, now time.Time) (int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if strings.TrimSpace(message.To) == "" {
		return 0, fmt.Errorf("message requires recipient")
	}
	if message.ID == 0 {
		message.ID = o.nextID
		o.nextID++
	} else if _, exists := o.messages[message.ID]; exists {
		return 0, fmt.Errorf("message %d already exists", message.ID)
	} else if message.ID >= o.nextID {
		o.nextID = message.ID + 1
	}
	if message.MaxAttempts <= 0 {
		message.MaxAttempts = o.maxAttempts
	}
	if message.Status == "" {
		message.Status = MessagePending
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.NextAttemptAt.IsZero() {
		message.NextAttemptAt = now
	}
	message.LastError = ""
	o.messages[message.ID] = message
	return message.ID, nil
}

func (o *Outbox) SendDue(sender Sender, now time.Time) SendResult {
	ids := o.dueIDs(now)
	var result SendResult
	for _, id := range ids {
		message, ok := o.getPending(id, now)
		if !ok {
			continue
		}
		var err error
		if sender == nil {
			err = fmt.Errorf("sender unavailable")
		} else {
			err = sender.Send(message)
		}
		o.mu.Lock()
		current, ok := o.messages[id]
		if !ok || current.Status != MessagePending {
			o.mu.Unlock()
			continue
		}
		if err == nil {
			current.Status = MessageSent
			current.SentAt = now
			current.LastError = ""
			o.messages[id] = current
			result.Sent++
			o.mu.Unlock()
			continue
		}
		current.Attempts++
		current.LastError = "send failed"
		if current.Attempts >= current.MaxAttempts {
			current.Status = MessageDead
			current.DeadAt = now
			current.NextAttemptAt = time.Time{}
			result.Dead++
		} else {
			current.NextAttemptAt = now.Add(o.backoff(current.Attempts))
			result.Retried++
		}
		o.messages[id] = current
		o.mu.Unlock()
	}
	return result
}

func (o *Outbox) Get(id int64) (Message, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	message, ok := o.messages[id]
	return message, ok
}

func (o *Outbox) List() []Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	messages := make([]Message, 0, len(o.messages))
	for _, message := range o.messages {
		messages = append(messages, message)
	}
	return messages
}

func (o *Outbox) dueIDs(now time.Time) []int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	var ids []int64
	for id, message := range o.messages {
		if message.Status == MessagePending && !message.NextAttemptAt.After(now) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (o *Outbox) getPending(id int64, now time.Time) (Message, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	message, ok := o.messages[id]
	if !ok || message.Status != MessagePending || message.NextAttemptAt.After(now) {
		return Message{}, false
	}
	return message, true
}

func defaultBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(attempt) * time.Minute
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

func renderText(text string, values map[string]any) string {
	if values == nil {
		values = map[string]any{}
	}
	return placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		value, ok := values[key]
		if !ok || value == nil {
			return ""
		}
		return html.EscapeString(fmt.Sprint(value))
	})
}
