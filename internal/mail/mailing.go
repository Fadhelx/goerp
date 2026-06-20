package mail

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"html"
	"math/rand"
	"sort"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type MassMailingSendResult struct {
	MailingIDs   []int64
	MailIDs      []int64
	KPIReportIDs []int64
	KPIReports   int
	Done         int
	Skipped      int
}

type MassMailingWinnerResult struct {
	SourceMailingID int64
	WinnerMailingID int64
}

type MassMailingTestResult struct {
	MailIDs []int64
	Invalid []string
}

func SendMassMailings(env *record.Env, ids []int64, resIDs []int64, now time.Time) (MassMailingSendResult, error) {
	if env == nil {
		return MassMailingSendResult{}, fmt.Errorf("mailing send requires env")
	}
	now = queueNow(now)
	var result MassMailingSendResult
	for _, mailingID := range uniqueIDs(ids) {
		row, err := massMailingRow(env, mailingID)
		if err != nil {
			return result, err
		}
		if row == nil {
			result.Skipped++
			continue
		}
		targetIDs := uniqueIDs(resIDs)
		if len(targetIDs) == 0 {
			targetIDs, err = remainingMassMailingRecipientIDs(env, mailingID, row)
			if err != nil {
				return result, err
			}
		}
		if len(targetIDs) == 0 {
			return result, fmt.Errorf("there are no recipients selected")
		}
		mailIDs, err := generateMassMailingMails(env, mailingID, row, targetIDs, now)
		if err != nil {
			return result, err
		}
		if err := markMassMailingDone(env, mailingID, now); err != nil {
			return result, err
		}
		result.MailingIDs = append(result.MailingIDs, mailingID)
		result.MailIDs = append(result.MailIDs, mailIDs...)
		result.Done++
	}
	return result, nil
}

func SendMassMailingKPIReports(env *record.Env, now time.Time) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mailing KPI report requires env")
	}
	now = queueNow(now)
	if !configParameterBoolMail(messageSystemEnv(env), "mass_mailing.mass_mailing_reports") {
		return nil, nil
	}
	found, err := env.Model("mailing.mailing").Search(domain.And(
		domain.Cond("kpi_mail_required", "=", true),
		domain.Cond("state", "=", "done"),
	))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "name", "subject", "sent_date", "user_id", "write_uid")
	if err != nil {
		return nil, err
	}
	reportIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		sentDate := timeValue(row["sent_date"])
		if sentDate.IsZero() || sentDate.After(now.Add(-24*time.Hour)) || sentDate.Before(now.Add(-5*24*time.Hour)) {
			continue
		}
		mailValues, err := massMailingKPIReportMailValues(env, row)
		if err != nil {
			return reportIDs, err
		}
		mailID, err := env.Model("mail.mail").Create(mailValues)
		if err != nil {
			return reportIDs, err
		}
		if err := env.Model("mailing.mailing").Browse(int64FromAny(row["id"])).Write(map[string]any{"kpi_mail_required": false}); err != nil {
			return reportIDs, err
		}
		reportIDs = append(reportIDs, mailID)
	}
	return reportIDs, nil
}

func ProcessMassMailingQueue(env *record.Env, now time.Time) (MassMailingSendResult, error) {
	if env == nil {
		return MassMailingSendResult{}, fmt.Errorf("mailing queue requires env")
	}
	now = queueNow(now)
	found, err := env.Model("mailing.mailing").Search(domain.Cond("state", "in", []any{"in_queue", "sending"}))
	if err != nil {
		return MassMailingSendResult{}, err
	}
	rows, err := found.Read("id", "schedule_date")
	if err != nil {
		return MassMailingSendResult{}, err
	}
	var result MassMailingSendResult
	for _, row := range rows {
		scheduleDate := timeValue(row["schedule_date"])
		if !scheduleDate.IsZero() && scheduleDate.After(now) {
			result.Skipped++
			continue
		}
		mailingID := int64FromAny(row["id"])
		mailingRow, err := massMailingRow(env, mailingID)
		if err != nil {
			return result, err
		}
		targetIDs, err := remainingMassMailingRecipientIDs(env, mailingID, mailingRow)
		if err != nil {
			return result, err
		}
		if len(targetIDs) == 0 {
			if err := markMassMailingDone(env, mailingID, now); err != nil {
				return result, err
			}
			result.MailingIDs = append(result.MailingIDs, mailingID)
			result.Done++
			continue
		}
		if err := env.Model("mailing.mailing").Browse(mailingID).Write(map[string]any{"state": "sending"}); err != nil {
			return result, err
		}
		mailIDs, err := generateMassMailingMails(env, mailingID, mailingRow, targetIDs, now)
		if err != nil {
			return result, err
		}
		if err := markMassMailingDone(env, mailingID, now); err != nil {
			return result, err
		}
		result.MailingIDs = append(result.MailingIDs, mailingID)
		result.MailIDs = append(result.MailIDs, mailIDs...)
		result.Done++
	}
	kpiReportIDs, err := SendMassMailingKPIReports(env, now)
	if err != nil {
		return result, err
	}
	result.KPIReportIDs = append(result.KPIReportIDs, kpiReportIDs...)
	result.KPIReports = len(kpiReportIDs)
	return result, nil
}

func QueueMassMailings(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mailing queue requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return fmt.Errorf("mailing queue requires ids")
	}
	if err := env.Model("mailing.mailing").Browse(ids...).Write(map[string]any{"state": "in_queue"}); err != nil {
		return err
	}
	rows, err := env.Model("mailing.mailing").Browse(ids...).Read("schedule_date")
	if err != nil {
		return err
	}
	for _, row := range rows {
		at := timeValue(row["schedule_date"])
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if err := triggerMassMailingQueueCronAt(env, at); err != nil {
			return err
		}
	}
	return nil
}

func ScheduleMassMailings(env *record.Env, ids []int64, now time.Time) (bool, error) {
	if env == nil {
		return false, fmt.Errorf("mailing schedule requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return false, fmt.Errorf("mailing schedule requires ids")
	}
	now = queueNow(now)
	rows, err := env.Model("mailing.mailing").Browse(ids...).Read("schedule_date")
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		scheduleDate := timeValue(row["schedule_date"])
		if scheduleDate.IsZero() || !scheduleDate.After(now) {
			return false, nil
		}
	}
	if err := QueueMassMailings(env, ids); err != nil {
		return false, err
	}
	return true, nil
}

func LaunchMassMailings(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mailing launch requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return fmt.Errorf("mailing launch requires ids")
	}
	if err := env.Model("mailing.mailing").Browse(ids...).Write(map[string]any{"schedule_type": "now"}); err != nil {
		return err
	}
	return QueueMassMailings(env, ids)
}

func ScheduleMassMailingAt(env *record.Env, mailingID int64, scheduleDate time.Time) error {
	if env == nil {
		return fmt.Errorf("mailing schedule requires env")
	}
	if mailingID == 0 {
		return fmt.Errorf("mailing schedule requires mailing id")
	}
	if scheduleDate.IsZero() {
		return fmt.Errorf("mailing schedule requires schedule_date")
	}
	if err := env.Model("mailing.mailing").Browse(mailingID).Write(map[string]any{
		"schedule_type": "scheduled",
		"schedule_date": scheduleDate.UTC(),
	}); err != nil {
		return err
	}
	return QueueMassMailings(env, []int64{mailingID})
}

func CancelMassMailings(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mailing cancel requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return fmt.Errorf("mailing cancel requires ids")
	}
	return env.Model("mailing.mailing").Browse(ids...).Write(map[string]any{
		"state":         "draft",
		"schedule_date": nil,
		"schedule_type": "now",
	})
}

func SendMassMailingTests(env *record.Env, wizardIDs []int64, now time.Time) (MassMailingTestResult, error) {
	if env == nil {
		return MassMailingTestResult{}, fmt.Errorf("mailing test requires env")
	}
	wizardIDs = uniqueIDs(wizardIDs)
	if len(wizardIDs) == 0 {
		return MassMailingTestResult{}, fmt.Errorf("mailing test requires ids")
	}
	now = queueNow(now)
	rows, err := env.Model("mailing.mailing.test").Browse(wizardIDs...).Read("email_to", "mass_mailing_id")
	if err != nil {
		return MassMailingTestResult{}, err
	}
	var result MassMailingTestResult
	for _, wizard := range rows {
		mailingID := int64FromAny(wizard["mass_mailing_id"])
		row, err := massMailingRow(env, mailingID)
		if err != nil {
			return result, err
		}
		if row == nil {
			return result, fmt.Errorf("mailing test source not found")
		}
		valid, invalid := massMailingTestEmails(stringAny(wizard["email_to"]))
		result.Invalid = append(result.Invalid, invalid...)
		if len(valid) == 0 {
			continue
		}
		recordID := massMailingFirstRecordID(env, row)
		mailIDs, err := generateMassMailingTestMails(env, mailingID, row, recordID, valid, now)
		if err != nil {
			return result, err
		}
		result.MailIDs = append(result.MailIDs, mailIDs...)
	}
	return result, nil
}

func RetryFailedMassMailings(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mailing retry requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return fmt.Errorf("mailing retry requires ids")
	}
	found, err := env.Model("mail.mail").Search(domain.And(
		domain.Cond("mailing_id", "in", int64sToAny(ids)),
		domain.Cond("state", "=", "exception"),
	))
	if err != nil {
		return err
	}
	rows, err := found.Read("id")
	if err != nil {
		return err
	}
	for _, row := range rows {
		mailID := int64FromAny(row["id"])
		traces, err := mailingTraceRowsForMailID(env, mailID)
		if err != nil {
			return err
		}
		traceIDs := make([]int64, 0, len(traces))
		for _, trace := range traces {
			traceIDs = append(traceIDs, int64FromAny(trace["id"]))
		}
		if len(traceIDs) > 0 {
			if err := env.Model("mailing.trace").Browse(traceIDs...).Unlink(); err != nil {
				return err
			}
		}
		if err := env.Model("mail.mail").Browse(mailID).Unlink(); err != nil {
			return err
		}
	}
	return QueueMassMailings(env, ids)
}

func SendABWinnerMassMailing(env *record.Env, ids []int64, now time.Time) (MassMailingWinnerResult, error) {
	if env == nil {
		return MassMailingWinnerResult{}, fmt.Errorf("mailing winner requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return MassMailingWinnerResult{}, fmt.Errorf("mailing winner requires ids")
	}
	rows, err := env.Model("mailing.mailing").Browse(ids...).Read("id", "campaign_id", "ab_testing_winner_selection")
	if err != nil {
		return MassMailingWinnerResult{}, err
	}
	campaignID := int64(0)
	for _, row := range rows {
		rowCampaignID := int64FromAny(row["campaign_id"])
		if rowCampaignID == 0 {
			return MassMailingWinnerResult{}, fmt.Errorf("no mailing campaign has been found")
		}
		if campaignID == 0 {
			campaignID = rowCampaignID
		}
		if campaignID != rowCampaignID {
			return MassMailingWinnerResult{}, fmt.Errorf("to send the winner mailing the same campaign should be used by the mailings")
		}
	}
	if massMailingCampaignCompleted(env, campaignID) {
		return MassMailingWinnerResult{}, fmt.Errorf("to send the winner mailing the campaign should not have been completed")
	}
	selection := massMailingCampaignWinnerSelection(env, campaignID, rows[0])
	finalID := ids[0]
	if selection != "manual" {
		selectedID, err := massMailingAutomaticWinnerID(env, campaignID, selection)
		if err != nil {
			return MassMailingWinnerResult{}, err
		}
		finalID = selectedID
	}
	return SelectABWinnerMassMailing(env, []int64{finalID}, now)
}

func SelectABWinnerMassMailing(env *record.Env, ids []int64, now time.Time) (MassMailingWinnerResult, error) {
	if env == nil {
		return MassMailingWinnerResult{}, fmt.Errorf("mailing winner requires env")
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return MassMailingWinnerResult{}, fmt.Errorf("mailing winner requires ids")
	}
	sourceID := ids[0]
	row, err := massMailingRow(env, sourceID)
	if err != nil {
		return MassMailingWinnerResult{}, err
	}
	if row == nil {
		return MassMailingWinnerResult{}, fmt.Errorf("mailing winner source not found")
	}
	if !boolAny(row["ab_testing_enabled"]) {
		return MassMailingWinnerResult{}, fmt.Errorf("A/B test option has not been enabled")
	}
	campaignID := int64FromAny(row["campaign_id"])
	if campaignID == 0 {
		return MassMailingWinnerResult{}, fmt.Errorf("no mailing campaign has been found")
	}
	values := massMailingWinnerCopyValues(row, now)
	winnerID, err := env.Model("mailing.mailing").Create(values)
	if err != nil {
		return MassMailingWinnerResult{}, err
	}
	if err := env.Model("utm.campaign").Browse(campaignID).Write(map[string]any{
		"ab_testing_winner_mailing_id": winnerID,
		"ab_testing_completed":         true,
	}); err != nil {
		return MassMailingWinnerResult{}, err
	}
	if err := env.Model("mailing.mailing").Browse(winnerID).Write(map[string]any{"ab_testing_is_winner_mailing": true, "schedule_type": "now"}); err != nil {
		return MassMailingWinnerResult{}, err
	}
	if err := QueueMassMailings(env, []int64{winnerID}); err != nil {
		return MassMailingWinnerResult{}, err
	}
	return MassMailingWinnerResult{SourceMailingID: sourceID, WinnerMailingID: winnerID}, nil
}

func massMailingRow(env *record.Env, mailingID int64) (map[string]any, error) {
	if mailingID == 0 {
		return nil, nil
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read(
		"name", "subject", "preview", "body_html", "email_from", "reply_to", "reply_to_mode",
		"mail_server_id", "attachment_ids", "keep_archives", "mailing_model_real",
		"mailing_domain", "mailing_on_mailing_list", "contact_list_ids", "use_exclusion_list",
		"campaign_id", "source_id", "medium_id", "ab_testing_enabled", "ab_testing_pc",
		"ab_testing_is_winner_mailing", "ab_testing_schedule_datetime", "ab_testing_winner_selection",
		"sent_date", "user_id",
	)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func generateMassMailingMails(env *record.Env, mailingID int64, row map[string]any, resIDs []int64, now time.Time) ([]int64, error) {
	modelName := massMailingModelName(row)
	if modelName == "" {
		return nil, fmt.Errorf("mailing requires recipient model")
	}
	template := Template{
		Name:          firstText(row["name"], row["subject"]),
		To:            "{{ email }}",
		EmailFrom:     stringAny(row["email_from"]),
		ReplyTo:       stringAny(row["reply_to"]),
		Subject:       firstText(row["subject"], row["name"]),
		Body:          prependMassMailingPreview(stringAny(row["body_html"]), stringAny(row["preview"])),
		AttachmentIDs: uniqueIDs(int64s(row["attachment_ids"])),
	}
	emailValues := map[string]any{
		"mass_mailing_id":      mailingID,
		"mailing_id":           mailingID,
		"use_exclusion_list":   boolDefault(row["use_exclusion_list"], true),
		"auto_delete":          !boolAny(row["keep_archives"]),
		"mail_post_autofollow": false,
	}
	for _, key := range []string{"email_from", "reply_to", "mail_server_id"} {
		if value := row[key]; value != nil && stringAny(value) != "" {
			emailValues[key] = value
		}
	}
	if replyTo := strings.TrimSpace(stringAny(row["reply_to"])); replyTo != "" && stringAny(row["reply_to_mode"]) == "new" {
		emailValues["reply_to"] = replyTo
	}
	if len(template.AttachmentIDs) > 0 {
		emailValues["attachment_ids"] = append([]int64(nil), template.AttachmentIDs...)
	}
	mailIDs := make([]int64, 0, len(resIDs))
	duplicateSeen := map[string]bool{}
	for _, resID := range uniqueIDs(resIDs) {
		mailID, err := sendTemplateForRecord(env, template, modelName, resID, emailValues, now, 0, nil, true, duplicateSeen)
		if err != nil {
			return nil, err
		}
		if mailID != 0 {
			mailIDs = append(mailIDs, mailID)
		}
	}
	return mailIDs, nil
}

func remainingMassMailingRecipientIDs(env *record.Env, mailingID int64, row map[string]any) ([]int64, error) {
	recipientIDs, err := massMailingRecipientIDs(env, row)
	if err != nil || len(recipientIDs) == 0 {
		return recipientIDs, err
	}
	recipientIDs, err = massMailingABTestingRecipientIDs(env, mailingID, row, recipientIDs)
	if err != nil || len(recipientIDs) == 0 {
		return recipientIDs, err
	}
	if _, ok := env.ModelMetadata("mailing.trace"); !ok {
		return recipientIDs, nil
	}
	traceDomain := domain.Cond("mass_mailing_id", "=", mailingID)
	if boolAny(row["ab_testing_enabled"]) && boolAny(row["ab_testing_is_winner_mailing"]) {
		traceDomain = domain.Cond("mass_mailing_id", "in", int64sToAny(massMailingABSiblings(env, int64FromAny(row["campaign_id"]))))
	}
	found, err := env.Model("mailing.trace").Search(traceDomain)
	if err != nil {
		return nil, err
	}
	traceRows, err := found.Read("model", "res_id")
	if err != nil {
		return nil, err
	}
	modelName := massMailingModelName(row)
	done := map[int64]bool{}
	for _, trace := range traceRows {
		if strings.TrimSpace(stringAny(trace["model"])) == modelName {
			done[int64FromAny(trace["res_id"])] = true
		}
	}
	out := make([]int64, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		if id != 0 && !done[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func massMailingRecipientIDs(env *record.Env, row map[string]any) ([]int64, error) {
	modelName := massMailingModelName(row)
	if modelName == "" {
		return nil, fmt.Errorf("mailing requires recipient model")
	}
	if boolAny(row["mailing_on_mailing_list"]) && modelName == "mailing.contact" {
		return mailingContactIDsForListsAndDomain(env, int64s(row["contact_list_ids"]), row["mailing_domain"])
	}
	node, err := parseMassMailingDomain(row["mailing_domain"])
	if err != nil {
		return nil, err
	}
	found, err := env.Model(modelName).Search(node)
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id")
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, int64FromAny(row["id"]))
	}
	return uniqueIDs(out), nil
}

func mailingContactIDsForListsAndDomain(env *record.Env, listIDs []int64, rawDomain any) ([]int64, error) {
	listContactIDs, err := mailingContactIDsForLists(env, listIDs)
	if err != nil || len(listContactIDs) == 0 || massMailingDomainUnset(rawDomain) {
		return listContactIDs, err
	}
	node, err := parseMassMailingDomain(rawDomain)
	if err != nil {
		return nil, err
	}
	found, err := env.Model("mailing.contact").Search(node)
	if err != nil {
		return nil, err
	}
	domainIDs := found.IDs()
	allowed := make(map[int64]bool, len(domainIDs))
	for _, id := range domainIDs {
		allowed[id] = true
	}
	out := make([]int64, 0, len(listContactIDs))
	for _, id := range listContactIDs {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return uniqueIDs(out), nil
}

func mailingContactIDsForLists(env *record.Env, listIDs []int64) ([]int64, error) {
	listIDs = uniqueIDs(listIDs)
	if len(listIDs) == 0 {
		return nil, nil
	}
	found, err := env.Model("mailing.subscription").Search(domain.Cond("list_id", "in", int64sToAny(listIDs)))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("contact_id")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, int64FromAny(row["contact_id"]))
	}
	return uniqueIDs(ids), nil
}

func massMailingDomainUnset(value any) bool {
	text := strings.TrimSpace(stringAny(value))
	return text == "" || strings.EqualFold(text, "none")
}

func parseMassMailingDomain(value any) (domain.Node, error) {
	switch typed := value.(type) {
	case domain.Node:
		return domain.Parse(typed)
	case []any:
		return domain.Parse(typed)
	case []string:
		return domain.Parse(typed)
	}
	text := strings.TrimSpace(stringAny(value))
	if massMailingDomainUnset(value) {
		return domain.Cond("id", domain.In, []any{}), nil
	}
	node, err := domain.ParseLiteral(text)
	if err != nil {
		return domain.Cond("id", domain.In, []any{}), nil
	}
	return node, nil
}

func markMassMailingDone(env *record.Env, mailingID int64, now time.Time) error {
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("sent_date")
	if err != nil {
		return err
	}
	values := map[string]any{
		"state":     "done",
		"sent_date": queueNow(now),
	}
	if len(rows) == 0 || timeValue(rows[0]["sent_date"]).IsZero() {
		values["kpi_mail_required"] = true
	}
	return env.Model("mailing.mailing").Browse(mailingID).Write(values)
}

type massMailingKPIStats struct {
	Expected  int
	Canceled  int
	Pending   int
	Delivered int
	Opened    int
	Clicked   int
	Replied   int
	Bounced   int
	Failed    int
	Sent      int

	ReceivedRatio float64
	OpenedRatio   float64
	RepliedRatio  float64
	BouncedRatio  float64
}

func massMailingKPIReportMailValues(env *record.Env, mailing map[string]any) (map[string]any, error) {
	mailingID := int64FromAny(mailing["id"])
	userID := massMailingReportUserID(env, mailing)
	user, err := massMailingReportUser(env, userID)
	if err != nil {
		return nil, err
	}
	stats := massMailingKPIStatsFor(env, mailingID)
	title := fmt.Sprintf(`24H Stats of Emails "%s"`, firstText(mailing["subject"], mailing["name"]))
	body := massMailingKPIReportBody(env, mailingID, title, stats, userID)
	return map[string]any{
		"auto_delete": true,
		"email_from":  user.EmailFormatted,
		"email_to":    user.EmailFormatted,
		"reply_to":    user.EmailFormatted,
		"state":       "outgoing",
		"subject":     title,
		"body_html":   body,
	}, nil
}

type massMailingReportUserInfo struct {
	ID             int64
	Name           string
	Email          string
	EmailFormatted string
}

func massMailingReportUserID(env *record.Env, mailing map[string]any) int64 {
	if id := int64FromAny(mailing["user_id"]); id != 0 {
		return id
	}
	if env != nil && env.Context().UserID != 0 {
		return env.Context().UserID
	}
	return 1
}

func massMailingReportUser(env *record.Env, userID int64) (massMailingReportUserInfo, error) {
	info := massMailingReportUserInfo{ID: userID, Name: "Administrator", Email: "admin@example.com"}
	if env == nil || userID == 0 {
		info.EmailFormatted = formatMailingReportEmail(info.Name, info.Email)
		return info, nil
	}
	rows, err := messageSystemEnv(env).Model("res.users").Browse(userID).Read("name", "email", "partner_id")
	if err != nil {
		return info, err
	}
	if len(rows) == 0 {
		info.EmailFormatted = formatMailingReportEmail(info.Name, info.Email)
		return info, nil
	}
	info.Name = firstText(rows[0]["name"], info.Name)
	info.Email = strings.TrimSpace(stringAny(rows[0]["email"]))
	if info.Email == "" {
		partnerID := int64FromAny(rows[0]["partner_id"])
		if partnerID != 0 {
			partnerRows, err := messageSystemEnv(env).Model("res.partner").Browse(partnerID).Read("email")
			if err != nil {
				return info, err
			}
			if len(partnerRows) > 0 {
				info.Email = strings.TrimSpace(stringAny(partnerRows[0]["email"]))
			}
		}
	}
	if info.Email == "" {
		info.Email = strings.TrimSpace(info.Name)
	}
	info.EmailFormatted = formatMailingReportEmail(info.Name, info.Email)
	return info, nil
}

func formatMailingReportEmail(name string, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name != "" && email != "" && !strings.Contains(email, "<") {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	if email != "" {
		return email
	}
	return name
}

func massMailingKPIStatsFor(env *record.Env, mailingID int64) massMailingKPIStats {
	stats := massMailingKPIStats{}
	if env == nil || mailingID == 0 {
		return stats
	}
	found, err := messageSystemEnv(env).Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		return stats
	}
	rows, err := found.Read("trace_status", "failure_type", "sent_datetime", "open_datetime", "links_click_datetime", "reply_datetime")
	if err != nil {
		return stats
	}
	stats.Expected = len(rows)
	for _, row := range rows {
		status := strings.TrimSpace(stringAny(row["trace_status"]))
		switch status {
		case "sent", "open", "reply":
			stats.Delivered++
		case "cancel":
			stats.Canceled++
		case "pending":
			stats.Pending++
		case "bounce":
			stats.Bounced++
		case "error":
			stats.Failed++
		}
		if status == "open" || status == "reply" {
			stats.Opened++
		}
		if !timeValue(row["links_click_datetime"]).IsZero() {
			stats.Clicked++
		}
		if status == "reply" {
			stats.Replied++
		}
		if !timeValue(row["sent_datetime"]).IsZero() {
			stats.Sent++
		}
	}
	total := stats.Expected - stats.Canceled
	if total == 0 {
		total = 1
	}
	totalNoError := stats.Expected - stats.Canceled - stats.Bounced - stats.Failed
	if totalNoError == 0 {
		totalNoError = 1
	}
	totalSent := stats.Expected - stats.Canceled - stats.Failed
	if totalSent == 0 {
		totalSent = 1
	}
	stats.ReceivedRatio = roundMailingKPI(100 * float64(stats.Delivered) / float64(total))
	stats.OpenedRatio = roundMailingKPI(100 * float64(stats.Opened) / float64(totalNoError))
	stats.RepliedRatio = roundMailingKPI(100 * float64(stats.Replied) / float64(totalNoError))
	stats.BouncedRatio = roundMailingKPI(100 * float64(stats.Bounced) / float64(totalSent))
	return stats
}

func roundMailingKPI(value float64) float64 {
	if value < 0 {
		value = 0
	}
	return float64(int(value*100+0.5)) / 100
}

func formatMailingKPI(value float64) string {
	formatted := fmt.Sprintf("%.2f", value)
	formatted = strings.TrimRight(formatted, "0")
	if strings.HasSuffix(formatted, ".") {
		formatted += "0"
	}
	return formatted
}

func massMailingKPIReportBody(env *record.Env, mailingID int64, title string, stats massMailingKPIStats, userID int64) string {
	var b strings.Builder
	b.WriteString(`<div class="o_mail_wrapper"><div class="o_mail_content o_digest_mail_main">`)
	b.WriteString(`<table class="global_layout" cellspacing="0" cellpadding="0"><tbody><tr><td>`)
	b.WriteString(`<h1 class="header_title">`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h1>`)
	b.WriteString(`<p><a class="button" id="button_connect" href="/odoo/mailing.mailing/`)
	b.WriteString(fmt.Sprint(mailingID))
	b.WriteString(`">More Info</a></p>`)
	if tip := massMailingKPIDigestTipHTML(env); strings.TrimSpace(tip) != "" {
		b.WriteString(`<div class="digest_tip">`)
		b.WriteString(tip)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<table class="kpi_table" data-field="mail" cellspacing="0" cellpadding="10"><tbody>`)
	b.WriteString(`<tr class="kpi_header"><td colspan="3"><span>`)
	b.WriteString(html.EscapeString(fmt.Sprintf("Engagement on %d Emails Sent", stats.Expected)))
	b.WriteString(`</span></td></tr><tr>`)
	for _, item := range []struct {
		Value    string
		Subtitle string
	}{
		{formatMailingKPI(stats.ReceivedRatio) + "%", fmt.Sprintf("RECEIVED (%d)", stats.Delivered)},
		{formatMailingKPI(stats.OpenedRatio) + "%", fmt.Sprintf("OPENED (%d)", stats.Opened)},
		{formatMailingKPI(stats.RepliedRatio) + "%", fmt.Sprintf("REPLIED (%d)", stats.Replied)},
	} {
		b.WriteString(`<td class="kpi_cell"><div class="kpi_value">`)
		b.WriteString(html.EscapeString(item.Value))
		b.WriteString(`</div><div class="kpi_subtitle">`)
		b.WriteString(html.EscapeString(item.Subtitle))
		b.WriteString(`</div></td>`)
	}
	b.WriteString(`</tr></tbody></table>`)
	b.WriteString(massMailingKPIReportLinkTrackerHTML(env, mailingID, stats))
	b.WriteString(`<p class="by_odoo">Sent by <a href="https://www.odoo.com" target="_blank">Odoo</a>`)
	if token := MassMailingReportToken(env, userID); token != "" && MassMailingReportUserAllowed(env, userID) {
		b.WriteString(` – <a href="/mailing/report/unsubscribe?token=`)
		b.WriteString(html.EscapeString(token))
		b.WriteString(`&amp;user_id=`)
		b.WriteString(fmt.Sprint(userID))
		b.WriteString(`">Turn off Mailing Reports</a>`)
	}
	b.WriteString(`</p></td></tr></tbody></table></div></div>`)
	return b.String()
}

func massMailingKPIDigestTipHTML(env *record.Env) string {
	if env == nil {
		return ""
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("digest.tip"); !ok {
		return ""
	}
	found, err := systemEnv.Model("digest.tip").Search(domain.And())
	if err != nil || found.Len() == 0 {
		return ""
	}
	rows, err := found.Read("tip_description", "group_id")
	if err != nil || len(rows) == 0 {
		return ""
	}
	emailMarketingPrivilegeID := resolveXMLID(systemEnv, "mass_mailing.res_groups_privilege_email_marketing", "res.groups.privilege")
	mailingGroupID := resolveXMLID(systemEnv, "mass_mailing.group_mass_mailing_user", "res.groups")
	var tips []string
	for _, row := range rows {
		groupID := int64FromAny(row["group_id"])
		tip := strings.TrimSpace(stringAny(row["tip_description"]))
		if tip == "" {
			continue
		}
		if emailMarketingPrivilegeID != 0 {
			if groupID != 0 && groupHasPrivilege(systemEnv, groupID, emailMarketingPrivilegeID) {
				tips = append(tips, tip)
			}
			continue
		}
		if mailingGroupID == 0 || groupID == 0 || groupID == mailingGroupID {
			tips = append(tips, tip)
		}
	}
	if len(tips) == 0 {
		return ""
	}
	return tips[rand.Intn(len(tips))]
}

func groupHasPrivilege(env *record.Env, groupID int64, privilegeID int64) bool {
	if env == nil || groupID == 0 || privilegeID == 0 {
		return false
	}
	rows, err := env.Model("res.groups").Browse(groupID).Read("privilege_id")
	if err != nil || len(rows) == 0 {
		return false
	}
	return int64FromAny(rows[0]["privilege_id"]) == privilegeID
}

func massMailingKPIReportLinkTrackerHTML(env *record.Env, mailingID int64, stats massMailingKPIStats) string {
	if env == nil || mailingID == 0 {
		return ""
	}
	found, err := messageSystemEnv(env).Model("link.tracker").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil || found.Len() == 0 {
		return ""
	}
	rows, err := found.Read("label", "url", "absolute_url", "count")
	if err != nil || len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool { return int64FromAny(rows[i]["count"]) > int64FromAny(rows[j]["count"]) })
	var b strings.Builder
	b.WriteString(`<table class="global_table_layout" cellspacing="0" cellpadding="0"><tbody>`)
	b.WriteString(`<tr><td><span>`)
	b.WriteString(html.EscapeString(fmt.Sprintf("Click Rate Report on %d Emails Sent", stats.Expected)))
	b.WriteString(`</span></td></tr>`)
	b.WriteString(`<tr><td><table cellspacing="0" cellpadding="0"><tbody>`)
	b.WriteString(`<tr><td>Button Label</td><td>%Click (Total)</td></tr>`)
	for _, row := range rows {
		count := int64FromAny(row["count"])
		ratio := int64(0)
		if stats.Sent > 0 {
			ratio = count * 100 / int64(stats.Sent)
		}
		label := firstText(row["label"], row["url"], row["absolute_url"])
		href := firstText(row["absolute_url"], row["url"])
		b.WriteString(`<tr><td><a target="_blank" href="`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</a></td><td>`)
		b.WriteString(fmt.Sprintf("%d%% (%d)", ratio, count))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></td></tr></tbody></table>`)
	return b.String()
}

func MassMailingReportToken(env *record.Env, userID int64) string {
	if env == nil || userID == 0 {
		return ""
	}
	secret := configParameter(messageSystemEnv(env), "database.secret")
	if secret == "" {
		return ""
	}
	payload := fmt.Sprintf("(%s, %d)", pythonReprString("mailing-report-deactivated"), userID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func MassMailingReportTokenValid(env *record.Env, userID int64, token string) bool {
	expected := MassMailingReportToken(env, userID)
	return expected != "" &&
		strings.TrimSpace(token) != "" &&
		MassMailingReportUserAllowed(env, userID) &&
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(expected)) == 1
}

func MassMailingReportUserAllowed(env *record.Env, userID int64) bool {
	if env == nil || userID == 0 {
		return false
	}
	groupID := resolveXMLID(messageSystemEnv(env), "mass_mailing.group_mass_mailing_user", "res.groups")
	return groupID != 0 && userHasGroupID(env, userID, groupID)
}

func massMailingModelName(row map[string]any) string {
	modelName := strings.TrimSpace(stringAny(row["mailing_model_real"]))
	if modelName == "" && boolAny(row["mailing_on_mailing_list"]) {
		modelName = "mailing.contact"
	}
	return modelName
}

func int64sToAny(values []int64) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func massMailingABTestingRecipientIDs(env *record.Env, mailingID int64, row map[string]any, recipientIDs []int64) ([]int64, error) {
	if !boolAny(row["ab_testing_enabled"]) || boolAny(row["ab_testing_is_winner_mailing"]) {
		return recipientIDs, nil
	}
	contactCount := len(recipientIDs)
	if contactCount == 0 {
		return nil, nil
	}
	topick := int(float64(contactCount) / 100.0 * float64(int64FromAny(row["ab_testing_pc"])))
	if topick < 1 {
		topick = 1
	}
	already, err := massMailingCampaignRecipientSet(env, int64FromAny(row["campaign_id"]))
	if err != nil {
		return nil, err
	}
	remaining := make([]int64, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		if id != 0 && !already[id] {
			remaining = append(remaining, id)
		}
	}
	if topick > len(remaining) || (len(remaining) > 0 && topick == 0) {
		topick = len(remaining)
	}
	return massMailingRandomSample(mailingID, remaining, topick), nil
}

func massMailingCampaignRecipientSet(env *record.Env, campaignID int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if campaignID == 0 {
		return out, nil
	}
	if _, ok := env.ModelMetadata("mailing.trace"); !ok {
		return out, nil
	}
	found, err := env.Model("mailing.trace").Search(domain.Cond("campaign_id", "=", campaignID))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("res_id")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if id := int64FromAny(row["res_id"]); id != 0 {
			out[id] = true
		}
	}
	return out, nil
}

func massMailingRandomSample(mailingID int64, ids []int64, count int) []int64 {
	ids = uniqueIDs(ids)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if count <= 0 {
		return nil
	}
	if count >= len(ids) {
		return ids
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + mailingID))
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	out := append([]int64(nil), ids[:count]...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func massMailingABSiblings(env *record.Env, campaignID int64) []int64 {
	if campaignID == 0 {
		return nil
	}
	found, err := env.Model("mailing.mailing").Search(domain.And(
		domain.Cond("campaign_id", "=", campaignID),
		domain.Cond("ab_testing_enabled", "=", true),
	))
	if err != nil {
		return nil
	}
	return found.IDs()
}

func massMailingCampaignCompleted(env *record.Env, campaignID int64) bool {
	if campaignID == 0 {
		return false
	}
	rows, err := env.Model("utm.campaign").Browse(campaignID).Read("ab_testing_completed", "ab_testing_winner_mailing_id")
	if err != nil || len(rows) == 0 {
		return false
	}
	return boolAny(rows[0]["ab_testing_completed"]) || int64FromAny(rows[0]["ab_testing_winner_mailing_id"]) != 0
}

func massMailingCampaignWinnerSelection(env *record.Env, campaignID int64, fallback map[string]any) string {
	if campaignID != 0 {
		rows, err := env.Model("utm.campaign").Browse(campaignID).Read("ab_testing_winner_selection")
		if err == nil && len(rows) > 0 {
			if selection := strings.TrimSpace(stringAny(rows[0]["ab_testing_winner_selection"])); selection != "" {
				return selection
			}
		}
	}
	if selection := strings.TrimSpace(stringAny(fallback["ab_testing_winner_selection"])); selection != "" {
		return selection
	}
	return "opened_ratio"
}

func massMailingAutomaticWinnerID(env *record.Env, campaignID int64, selection string) (int64, error) {
	siblingIDs := massMailingABSiblings(env, campaignID)
	if len(siblingIDs) == 0 {
		return 0, fmt.Errorf("no mailing for this A/B testing campaign has been sent yet")
	}
	rows, err := env.Model("mailing.mailing").Browse(siblingIDs...).Read("id", "state")
	if err != nil {
		return 0, err
	}
	bestID := int64(0)
	bestScore := -1.0
	for _, row := range rows {
		if stringAny(row["state"]) != "done" {
			continue
		}
		mailingID := int64FromAny(row["id"])
		score, err := massMailingMetric(env, mailingID, selection)
		if err != nil {
			return 0, err
		}
		if bestID == 0 || score > bestScore || (score == bestScore && mailingID < bestID) {
			bestID = mailingID
			bestScore = score
		}
	}
	if bestID == 0 {
		return 0, fmt.Errorf("no mailing for this A/B testing campaign has been sent yet")
	}
	return bestID, nil
}

func massMailingMetric(env *record.Env, mailingID int64, selection string) (float64, error) {
	found, err := env.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("trace_status", "open_datetime", "reply_datetime", "links_click_datetime", "failure_type")
	if err != nil {
		return 0, err
	}
	total := 0
	hit := 0
	for _, row := range rows {
		if stringAny(row["trace_status"]) == "cancel" {
			continue
		}
		total++
		switch selection {
		case "clicks_ratio":
			if !timeValue(row["links_click_datetime"]).IsZero() {
				hit++
			}
		case "replied_ratio":
			if stringAny(row["trace_status"]) == "reply" || !timeValue(row["reply_datetime"]).IsZero() {
				hit++
			}
		default:
			if stringAny(row["trace_status"]) == "open" || stringAny(row["trace_status"]) == "reply" || !timeValue(row["open_datetime"]).IsZero() {
				hit++
			}
		}
	}
	if total == 0 {
		return 0, nil
	}
	return 100.0 * float64(hit) / float64(total), nil
}

func massMailingWinnerCopyValues(row map[string]any, now time.Time) map[string]any {
	name := strings.TrimSpace(firstText(row["name"], row["subject"]))
	if name == "" {
		name = "Mailing"
	}
	values := map[string]any{
		"name":                         name + " (final)",
		"subject":                      row["subject"],
		"preview":                      row["preview"],
		"body_html":                    row["body_html"],
		"email_from":                   row["email_from"],
		"reply_to":                     row["reply_to"],
		"reply_to_mode":                row["reply_to_mode"],
		"mail_server_id":               row["mail_server_id"],
		"attachment_ids":               uniqueIDs(int64s(row["attachment_ids"])),
		"keep_archives":                boolAny(row["keep_archives"]),
		"state":                        "draft",
		"schedule_type":                "now",
		"schedule_date":                nil,
		"kpi_mail_required":            false,
		"user_id":                      int64FromAny(row["user_id"]),
		"mailing_model_real":           row["mailing_model_real"],
		"mailing_domain":               row["mailing_domain"],
		"mailing_on_mailing_list":      boolAny(row["mailing_on_mailing_list"]),
		"use_exclusion_list":           boolDefault(row["use_exclusion_list"], true),
		"contact_list_ids":             uniqueIDs(int64s(row["contact_list_ids"])),
		"campaign_id":                  int64FromAny(row["campaign_id"]),
		"source_id":                    int64FromAny(row["source_id"]),
		"medium_id":                    int64FromAny(row["medium_id"]),
		"ab_testing_enabled":           true,
		"ab_testing_pc":                int64(100),
		"ab_testing_is_winner_mailing": true,
		"ab_testing_schedule_datetime": queueNow(now),
		"ab_testing_winner_selection":  firstText(row["ab_testing_winner_selection"], "opened_ratio"),
	}
	return values
}

func generateMassMailingTestMails(env *record.Env, mailingID int64, row map[string]any, recordID int64, emails []string, now time.Time) ([]int64, error) {
	modelName := massMailingModelName(row)
	if modelName == "" {
		return nil, fmt.Errorf("mailing requires recipient model")
	}
	values, err := renderValues(env, modelName, recordID, firstText(row["subject"], row["name"]), stringAny(row["body_html"]), stringAny(row["preview"]))
	if err != nil {
		return nil, err
	}
	subject := renderText(firstText(row["subject"], row["name"]), values)
	body := renderText(stringAny(row["body_html"]), values)
	preview := renderText(stringAny(row["preview"]), values)
	wrappedBody := wrapMassMailingBody(prependMassMailingPreview(body, preview))
	attachmentIDs := uniqueIDs(int64s(row["attachment_ids"]))
	mailIDs := make([]int64, 0, len(emails))
	for _, email := range uniqueRecipientList(emails) {
		mailID, err := env.Model("mail.mail").Create(map[string]any{
			"email_from":      row["email_from"],
			"reply_to":        row["reply_to"],
			"email_to":        email,
			"subject":         "[TEST] " + subject,
			"body_html":       wrappedBody,
			"state":           "outgoing",
			"auto_delete":     false,
			"is_notification": false,
			"mailing_id":      mailingID,
			"mail_server_id":  int64FromAny(row["mail_server_id"]),
			"attachment_ids":  attachmentIDs,
			"create_date":     queueNow(now),
		})
		if err != nil {
			return mailIDs, err
		}
		mailIDs = append(mailIDs, mailID)
	}
	return mailIDs, nil
}

func massMailingTestEmails(raw string) ([]string, []string) {
	valid := []string{}
	invalid := []string{}
	for _, line := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items := normalizeEmails([]string{line})
		if len(items) == 0 || !strings.Contains(items[0].Address, "@") {
			invalid = append(invalid, line)
			continue
		}
		valid = append(valid, items[0].Address)
	}
	return uniqueRecipientList(valid), invalid
}

func massMailingFirstRecordID(env *record.Env, row map[string]any) int64 {
	modelName := massMailingModelName(row)
	if modelName == "" {
		return 0
	}
	found, err := env.Model(modelName).Search(domain.And())
	if err != nil {
		return 0
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func prependMassMailingPreview(body string, preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return body
	}
	preheader := `<div style="display:none;font-size:1px;height:0px;width:0px;opacity:0;">` + html.EscapeString(preview) + `</div>`
	return preheader + body
}

func wrapMassMailingBody(body string) string {
	if strings.Contains(body, `class="o_mail_wrapper"`) {
		return body
	}
	return `<div class="o_mail_wrapper"><div class="o_mail_content">` + body + `</div></div>`
}

func triggerMassMailingQueueCronAt(env *record.Env, at time.Time) error {
	if env == nil || at.IsZero() {
		return nil
	}
	cronID, err := massMailingQueueCronID(env)
	if err != nil || cronID == 0 {
		return err
	}
	_, err = env.Model("ir.cron.trigger").Create(map[string]any{"cron_id": cronID, "call_at": at.UTC()})
	if err != nil && strings.Contains(err.Error(), "unknown model ir.cron.trigger") {
		return nil
	}
	return err
}

func massMailingQueueCronID(env *record.Env) (int64, error) {
	if id, err := massMailingQueueCronIDByXMLID(env); err != nil || id != 0 {
		return id, err
	}
	found, err := env.Model("ir.cron").Search(domain.Cond("action_name", "=", "mailing.mailing.process_mass_mailing_queue"))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ir.cron") {
			return 0, nil
		}
		return 0, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func massMailingQueueCronIDByXMLID(env *record.Env) (int64, error) {
	found, err := env.Model("ir.model.data").Search(domain.Cond("complete_name", "=", "mass_mailing.ir_cron_mass_mailing_queue"))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.data") {
			return 0, nil
		}
		return 0, err
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64FromAny(rows[0]["res_id"]), nil
}
