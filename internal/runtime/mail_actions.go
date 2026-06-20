package runtime

import (
	"context"
	"time"

	serveractions "gorp/internal/actions"
	internalmail "gorp/internal/mail"
	"gorp/internal/record"
)

const (
	mailProcessQueueAction      = "mail.process_email_queue"
	mailModelProcessQueueAction = "mail.mail.process_email_queue"
	mailFetchmailAction         = "mail.fetchmail"
	fetchmailModelFetchAction   = "fetchmail.server._fetch_mails"
	massMailingQueueAction      = "mailing.mailing.process_mass_mailing_queue"
	massMailingModelQueueAction = "mailing.mailing._process_mass_mailing_queue"
	digestCronAction            = "digest.digest._cron_send_digest_email"
)

func registerMailRuntimeActions(reg *serveractions.Registry, env *record.Env, app *App) error {
	if reg == nil || env == nil {
		return nil
	}
	action := mailRuntimeProcessQueue(env, app)
	for _, name := range []string{mailProcessQueueAction, mailModelProcessQueueAction} {
		if err := reg.RegisterGo(name, action); err != nil {
			return err
		}
	}
	fetchmailAction := mailRuntimeFetchmail(env, app)
	for _, name := range []string{mailFetchmailAction, fetchmailModelFetchAction} {
		if err := reg.RegisterGo(name, fetchmailAction); err != nil {
			return err
		}
	}
	massMailingAction := mailRuntimeProcessMassMailingQueue(env)
	for _, name := range []string{massMailingQueueAction, massMailingModelQueueAction} {
		if err := reg.RegisterGo(name, massMailingAction); err != nil {
			return err
		}
	}
	digestAction := mailRuntimeSendDueDigests(env)
	if err := reg.RegisterGo(digestCronAction, digestAction); err != nil {
		return err
	}
	return nil
}

func mailRuntimeProcessQueue(env *record.Env, app *App) serveractions.GoAction {
	return func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		var sender internalmail.Sender
		if app != nil {
			sender = app.MailSender
		}
		result, err := internalmail.ProcessEmailQueue(ctx, env, sender, internalmail.QueueOptions{
			BatchSize: internalmail.DefaultQueueBatchSize,
			Now:       now,
		})
		return serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: mailProcessQueueAction,
			Metadata: map[string]any{
				"processed": result.Processed,
				"sent":      result.Sent,
				"failed":    result.Failed,
				"skipped":   result.Skipped,
			},
		}, err
	}
}

func mailRuntimeProcessMassMailingQueue(env *record.Env) serveractions.GoAction {
	return func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return serveractions.Result{}, err
			}
		}
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		result, err := internalmail.ProcessMassMailingQueue(env, now)
		return serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: massMailingQueueAction,
			Metadata: map[string]any{
				"mail_ids":       result.MailIDs,
				"mailing_ids":    result.MailingIDs,
				"kpi_report_ids": result.KPIReportIDs,
				"kpi_reports":    result.KPIReports,
				"done":           result.Done,
				"skipped":        result.Skipped,
			},
		}, err
	}
}

func mailRuntimeSendDueDigests(env *record.Env) serveractions.GoAction {
	return func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return serveractions.Result{}, err
			}
		}
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		result, err := internalmail.SendDueDigests(env, now)
		return serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: digestCronAction,
			Metadata: map[string]any{
				"digest_ids":  result.DigestIDs,
				"mail_ids":    result.MailIDs,
				"sent":        result.Sent,
				"skipped":     result.Skipped,
				"slowed_down": result.SlowedDown,
			},
		}, err
	}
}

func mailRuntimeFetchmail(env *record.Env, app *App) serveractions.GoAction {
	return func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		var connector internalmail.FetchmailConnector
		if app != nil {
			connector = app.FetchmailConnector
		}
		var progress internalmail.FetchmailProgressFunc
		if exec.Trigger == "cron" && exec.CommitProgress != nil {
			progress = func(processed int, remaining *int, deactivate bool) bool {
				return exec.CommitProgress(processed, remaining, deactivate)
			}
		}
		result, err := internalmail.ProcessFetchmailServers(ctx, env, connector, internalmail.FetchmailOptions{
			BatchLimit:      internalmail.DefaultFetchmailBatchLimit,
			Now:             now,
			MessageIDLocker: appInboundMessageLock(app),
			ServerLocker:    appFetchmailServerLock(app),
			Progress:        progress,
			NotifyAdmin:     func(message string) error { return internalmail.NotifyAdminChannel(env, message) },
			CronEndTime:     fetchmailCronEndTime(exec, now),
		})
		activeServers, countErr := internalmail.ActiveFetchmailServerCount(env)
		if err == nil && countErr != nil {
			err = countErr
		}
		progressDone := result.Checked + result.Fetched
		metadata := map[string]any{
			"servers":   result.Servers,
			"checked":   result.Checked,
			"fetched":   result.Fetched,
			"processed": result.Processed,
			"failed":    result.Failed,
			"skipped":   result.Skipped,
			"remaining": result.Remaining,
		}
		if progress == nil {
			metadata["cron_progress_done"] = progressDone
			metadata["cron_progress_remaining"] = result.Remaining
			metadata["cron_progress_deactivate"] = activeServers == 0
		} else if activeServers == 0 {
			remaining := 0
			progress(0, &remaining, true)
		}
		actionResult := serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: mailFetchmailAction,
			Metadata:     metadata,
		}
		if err != nil && exec.Trigger == "cron" {
			return actionResult, nil
		}
		return actionResult, err
	}
}

func fetchmailCronEndTime(exec serveractions.ExecutionContext, now time.Time) time.Time {
	for _, values := range []map[string]any{exec.Metadata, exec.Values} {
		if values == nil {
			continue
		}
		if end := timeValue(values["cron_end_time"]); !end.IsZero() {
			return end
		}
		for _, key := range []string{"cron_time_budget_seconds", "cron_budget_seconds"} {
			if seconds := int64Value(values[key]); seconds > 0 {
				return now.Add(time.Duration(seconds) * time.Second)
			}
		}
	}
	if exec.Trigger == "cron" {
		return now.Add(10 * time.Second)
	}
	return time.Time{}
}

func appInboundMessageLock(app *App) internalmail.InboundMessageIDLocker {
	if app == nil {
		return nil
	}
	return app.InboundMessageLock
}

func appFetchmailServerLock(app *App) internalmail.FetchmailServerLocker {
	if app == nil {
		return nil
	}
	return app.FetchmailServerLock
}
