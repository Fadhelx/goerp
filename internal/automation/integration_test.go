package automation

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gorp/internal/actions"
	"gorp/internal/mail"
	"gorp/internal/notifications"
	"gorp/internal/queue"
	"gorp/internal/scheduler"
)

func TestAutomationMailIntegrationCreatesActivity(t *testing.T) {
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	var activities []notifications.Activity

	actionRegistry := actions.NewRegistry(actions.Hooks{})
	if err := actionRegistry.RegisterGo("activity.todo", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		activities = append(activities, notifications.NewActivity(notifications.Activity{
			TypeID:   1,
			Model:    exec.Model,
			RecordID: exec.RecordID,
			UserID:   7,
			Summary:  "Review new partner",
			Deadline: exec.Now.Add(24 * time.Hour),
		}, exec.Now))
		return actions.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Create Activity", Kind: actions.KindGo, GoActionName: "activity.todo"})
	if err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(actionRegistry)
	if _, err := engine.Add(Automation{Name: "Partner Created", Model: "res.partner", Trigger: TriggerCreate, ActionID: actionID}); err != nil {
		t.Fatal(err)
	}
	results, err := engine.Run(context.Background(), Event{Trigger: TriggerCreate, Model: "res.partner", RecordID: 42, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{actionID}) {
		t.Fatalf("action ids = %+v", got)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	if activities[0].Status != notifications.ActivityOpen || activities[0].Model != "res.partner" || activities[0].RecordID != 42 {
		t.Fatalf("activity = %+v", activities[0])
	}
}

func TestAutomationMailIntegrationCronSendsQueuedEmail(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	outbox := mail.NewOutbox()
	sender := &integrationSender{}
	mailer := &outboxMailer{
		templates: map[int64]mail.Template{
			10: {ID: 10, To: "{{ email }}", Subject: "Welcome {{ name }}", Body: "<p>{{ name }}</p>"},
		},
		outbox: outbox,
		now:    now,
	}
	actionRegistry := actions.NewRegistry(actions.Hooks{Mailer: mailer})
	actionID, err := actionRegistry.Register(actions.ServerAction{
		Name:           "Send Welcome",
		Kind:           actions.KindSendMail,
		Model:          "res.partner",
		MailTemplateID: 10,
		Values:         map[string]any{"email": "ada@example.com", "name": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}

	s := scheduler.New()
	if _, err := s.AddCron(scheduler.Cron{
		Name:           "mail queue manager",
		Active:         true,
		IntervalNumber: 1,
		IntervalType:   scheduler.IntervalHours,
		NextCall:       now,
	}); err != nil {
		t.Fatal(err)
	}
	cronResult := s.RunDue(now, func(scheduler.Cron) error {
		if _, err := actionRegistry.Run(context.Background(), actionID, actions.ExecutionContext{Model: "res.partner", RecordID: 1, Now: now}); err != nil {
			return err
		}
		sendResult := outbox.SendDue(sender, now)
		if sendResult.Sent != 1 {
			return errors.New("expected one sent message")
		}
		return nil
	})
	if cronResult.Ran != 1 || cronResult.Succeeded != 1 {
		t.Fatalf("cron result = %+v", cronResult)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %+v", sender.sent)
	}
	if sender.sent[0].To != "ada@example.com" || sender.sent[0].Subject != "Welcome Ada" {
		t.Fatalf("message = %+v", sender.sent[0])
	}
}

func TestAutomationMailIntegrationFailedSMTPRetries(t *testing.T) {
	now := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC)
	outbox := mail.NewOutbox()
	outbox.SetBackoff(func(int) time.Duration { return time.Second })
	id, err := outbox.Enqueue(mail.Message{To: "user@example.com", MaxAttempts: 2}, now)
	if err != nil {
		t.Fatal(err)
	}

	failing := failingIntegrationSender{}
	first := outbox.SendDue(failing, now)
	if first.Retried != 1 || first.Dead != 0 {
		t.Fatalf("first result = %+v", first)
	}
	second := outbox.SendDue(failing, now.Add(time.Second))
	if second.Dead != 1 || second.Retried != 0 {
		t.Fatalf("second result = %+v", second)
	}
	message, ok := outbox.Get(id)
	if !ok {
		t.Fatal("message missing")
	}
	if message.Status != mail.MessageDead || message.LastError != "send failed" {
		t.Fatalf("message = %+v", message)
	}
}

func TestAutomationMailIntegrationTimeWindowAndDuplicateQueue(t *testing.T) {
	first := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	q := queue.New(queue.WithBackoff(func(queue.Job) time.Duration { return time.Second }))
	var windows []actions.ExecutionContext

	actionRegistry := actions.NewRegistry(actions.Hooks{
		Enqueuer: queueEnqueuer{queue: q},
	})
	if err := actionRegistry.RegisterGo("capture.window", func(_ context.Context, _ actions.ServerAction, exec actions.ExecutionContext) (actions.Result, error) {
		windows = append(windows, exec)
		_, _, err := q.Enqueue(queue.Job{
			Key:       "automation:window",
			Name:      "automation.window",
			NextRunAt: exec.Now,
			Payload:   map[string]any{"current_run_at": exec.CurrentRunAt},
		})
		return actions.Result{}, err
	}); err != nil {
		t.Fatal(err)
	}
	actionID, err := actionRegistry.Register(actions.ServerAction{Name: "Capture Window", Kind: actions.KindGo, GoActionName: "capture.window"})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(actionRegistry)
	if _, err := engine.Add(Automation{Name: "Timed", Model: "res.partner", Trigger: TriggerTime, ActionID: actionID, LastRunAt: first}); err != nil {
		t.Fatal(err)
	}

	results, err := engine.RunTime(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultIDs(results); !reflect.DeepEqual(got, []int64{actionID}) {
		t.Fatalf("action ids = %+v", got)
	}
	if len(windows) != 1 || !windows[0].LastRunAt.Equal(first) || !windows[0].CurrentRunAt.Equal(second) {
		t.Fatalf("windows = %+v", windows)
	}

	_, created, err := q.Enqueue(queue.Job{Key: "automation:window", Name: "duplicate", NextRunAt: second})
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate queue job was created")
	}
	if jobs := q.Jobs(); len(jobs) != 1 {
		t.Fatalf("jobs = %+v", jobs)
	}
}

type outboxMailer struct {
	templates map[int64]mail.Template
	outbox    *mail.Outbox
	now       time.Time
}

func (m outboxMailer) SendMail(_ context.Context, request actions.MailRequest) error {
	tpl, ok := m.templates[request.TemplateID]
	if !ok {
		return errors.New("template missing")
	}
	rendered := tpl.Render(request.Values)
	_, err := m.outbox.Enqueue(mail.Message{
		To:      rendered.To,
		Subject: rendered.Subject,
		Body:    rendered.Body,
	}, m.now)
	return err
}

type integrationSender struct {
	sent []mail.Message
}

func (s *integrationSender) Send(message mail.Message) error {
	s.sent = append(s.sent, message)
	return nil
}

type failingIntegrationSender struct{}

func (failingIntegrationSender) Send(mail.Message) error {
	return errors.New("smtp password secret")
}

type queueEnqueuer struct {
	queue *queue.Queue
}

func (e queueEnqueuer) Enqueue(_ context.Context, job actions.QueueJob) error {
	_, _, err := e.queue.Enqueue(queue.Job{
		Key:     job.Key,
		Name:    job.Name,
		Payload: job.Payload,
	})
	return err
}
