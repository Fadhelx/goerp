package mail

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	netmail "net/mail"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

const (
	DefaultQueueBatchSize      = 1000
	mailProcessQueueActionName = "mail.process_email_queue"
)

var (
	mailBodyAttachmentLinkPattern    = regexp.MustCompile(`/web/(?:content|image)/([0-9]+)`)
	massMailingTrackedHrefURLPattern = regexp.MustCompile(`(?i)(href\s*=\s*["'])([^"']+)(["'])`)
)

type QueueOptions struct {
	EmailIDs  []int64
	BatchSize int
	Now       time.Time
}

type QueueResult struct {
	Processed int
	Sent      int
	Failed    int
	Skipped   int
}

type mailDeliveryItem struct {
	Message   Message
	PartnerID int64
	Emails    []string
}

func ProcessEmailQueue(ctx context.Context, env *record.Env, sender Sender, options QueueOptions) (QueueResult, error) {
	ids, err := DueMailIDs(env, options)
	if err != nil {
		return QueueResult{}, err
	}
	return SendMails(ctx, env, sender, ids, queueNow(options.Now))
}

func SendMails(ctx context.Context, env *record.Env, sender Sender, ids []int64, now time.Time) (QueueResult, error) {
	if env == nil {
		return QueueResult{}, fmt.Errorf("mail queue requires env")
	}
	now = queueNow(now)
	var result QueueResult
	limitRun := newPersonalMailLimitRun()
	for _, id := range uniqueIDs(ids) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return result, err
			}
		}
		rows, err := env.Model("mail.mail").Browse(id).Read("mail_message_id", "recipient_ids", "attachment_ids", "mail_server_id", "record_alias_domain_id", "record_company_id", "email_from", "email_to", "email_cc", "reply_to", "subject", "body_html", "state", "failure_type", "failure_reason", "scheduled_date", "retry_count", "max_retries", "auto_delete", "message_id", "references", "headers", "is_notification", "fetchmail_server_id", "mailing_id", "create_uid", "create_date")
		if err != nil {
			return result, err
		}
		if len(rows) == 0 {
			result.Skipped++
			continue
		}
		row := rows[0]
		if strings.TrimSpace(stringAny(row["state"])) != "outgoing" {
			result.Skipped++
			continue
		}
		result.Processed++
		message, err := mailMessageFromRow(env, id, row, now)
		if err != nil {
			if writeErr := markMailException(env, id, "unknown", "send failed"); writeErr != nil {
				return result, writeErr
			}
			result.Failed++
			continue
		}
		setDefaultReturnPathHeader(env, row, &message)
		setOutboundMessageID(env, id, row, &message)
		mailSender := sender
		var serverRow map[string]any
		selectedFrom := ""
		envelopeFrom := ""
		if mailSender == nil {
			mailSender, selectedFrom, envelopeFrom, serverRow, err = smtpSenderForMail(env, row, message)
			if err != nil {
				if writeErr := markMailException(env, id, mailFailureType(err), "send failed"); writeErr != nil {
					return result, writeErr
				}
				result.Failed++
				continue
			}
			if strings.TrimSpace(selectedFrom) != "" {
				message.From = selectedFrom
			}
			if strings.TrimSpace(envelopeFrom) != "" {
				message.EnvelopeFrom = envelopeFrom
			}
		} else if serverID := int64FromAny(row["mail_server_id"]); serverID != 0 {
			serverRow, err = mailServerThrottleRow(env, serverID)
			if err != nil {
				if writeErr := markMailException(env, id, mailFailureType(err), "send failed"); writeErr != nil {
					return result, writeErr
				}
				result.Failed++
				continue
			}
			if value, exists := serverRow["active"]; exists && value != nil && !boolAny(value) {
				if writeErr := markMailException(env, id, "mail_smtp", "send failed"); writeErr != nil {
					return result, writeErr
				}
				result.Failed++
				continue
			}
			if !mailServerAllowedForMail(env, row, serverRow, message) {
				if writeErr := markMailException(env, id, "mail_smtp", "send failed"); writeErr != nil {
					return result, writeErr
				}
				result.Failed++
				continue
			}
		}
		sendID, sendRow, delayed, err := preparePersonalMailForLimit(env, id, row, serverRow, now, limitRun)
		if err != nil {
			return result, err
		}
		if delayed {
			result.Skipped++
			continue
		}
		if sendID != id {
			id = sendID
			row = sendRow
			message, err = mailMessageFromRow(env, id, row, now)
			if err != nil {
				if writeErr := markMailException(env, id, "unknown", "send failed"); writeErr != nil {
					return result, writeErr
				}
				result.Failed++
				continue
			}
			setDefaultReturnPathHeader(env, row, &message)
			setOutboundMessageID(env, id, row, &message)
			if strings.TrimSpace(selectedFrom) != "" {
				message.From = selectedFrom
			}
			if strings.TrimSpace(envelopeFrom) != "" {
				message.EnvelopeFrom = envelopeFrom
			}
		}
		blacklisted, err := shouldCancelMassMailingBlacklistedMail(env, row)
		if err != nil {
			return result, err
		}
		if blacklisted {
			if err := markMailCanceled(env, id, "mail_bl", ""); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		duplicate, err := shouldCancelDuplicateMassMailingQueuedMail(env, id, row)
		if err != nil {
			return result, err
		}
		if duplicate {
			if err := markMailCanceled(env, id, "mail_dup", ""); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		suppression, err := massMailingListSuppression(env, id, row)
		if err != nil {
			return result, err
		}
		if !suppression.empty() {
			row = copyMailRow(row)
			message.To = filterRecipientAddressField(message.To, suppression.emails)
			message.CC = filterRecipientAddressField(message.CC, suppression.emails)
			row["email_to"] = message.To
			row["email_cc"] = message.CC
			recipientIDs, err := filterPartnerRecipientsByEmail(env, int64s(row["recipient_ids"]), suppression.emails)
			if err != nil {
				return result, err
			}
			row["recipient_ids"] = recipientIDs
			if !messageHasRecipient(message) && len(recipientIDs) == 0 {
				if err := markMailCanceled(env, id, "mail_optout", ""); err != nil {
					return result, err
				}
				result.Skipped++
				continue
			}
			if err := markMailingTraceCanceledEmails(env, id, suppression.emails, "mail_optout"); err != nil {
				return result, err
			}
		}
		if err := markMailingTraceMessageID(env, id, messageHeaderValue(message.Headers, "Message-Id")); err != nil {
			return result, err
		}
		if err := applyMassMailingTrackingBody(env, id, row, &message); err != nil {
			if writeErr := markMailException(env, id, "unknown", "send failed"); writeErr != nil {
				return result, writeErr
			}
			result.Failed++
			continue
		}
		if err := applyOutgoingAttachmentPayload(env, row, serverRow, &message); err != nil {
			if writeErr := markMailException(env, id, "unknown", "send failed"); writeErr != nil {
				return result, writeErr
			}
			result.Failed++
			continue
		}
		if err := markMailException(env, id, "unknown", "send failed"); err != nil {
			return result, err
		}
		items := mailDeliveryItems(env, row, message)
		sentCount := 0
		failedCount := 0
		failureType := "unknown"
		successPartnerIDs := []int64{}
		successEmails := []string{}
		if len(items) == 0 {
			failedCount++
			failureType = "mail_email_missing"
		}
		for _, item := range items {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return result, err
				}
			}
			if !messageHasRecipient(item.Message) {
				failedCount++
				failureType = "mail_email_missing"
				continue
			}
			err = mailSender.Send(item.Message)
			if err != nil {
				failedCount++
				failureType = mailFailureType(err)
				continue
			}
			sentCount++
			if item.PartnerID != 0 {
				successPartnerIDs = append(successPartnerIDs, item.PartnerID)
			} else {
				successEmails = append(successEmails, item.Emails...)
			}
		}
		if sentCount == 0 {
			if err := markMailException(env, id, failureType, "send failed"); err != nil {
				return result, err
			}
			result.Failed++
			continue
		}
		if failedCount == 0 {
			if err := updateLinkedNotifications(env, id, "sent", "", ""); err != nil {
				return result, err
			}
		} else if err := postprocessLinkedNotifications(env, id, successPartnerIDs, successEmails, failureType, "send failed"); err != nil {
			return result, err
		}
		messageID := messageHeaderValue(message.Headers, "Message-Id")
		if err := storeOutboundMessageID(env, id, int64FromAny(row["mail_message_id"]), messageID); err != nil {
			return result, err
		}
		if failedCount == 0 {
			if err := markMailingTraceSent(env, id, now); err != nil {
				return result, err
			}
		} else if err := markMailingTraceFailed(env, id, failureType); err != nil {
			return result, err
		}
		if boolAny(row["auto_delete"]) && mailAutoDeleteAllowed(failedCount, failureType) {
			if err := env.Model("mail.mail").Browse(id).Unlink(); err != nil {
				return result, err
			}
		} else if err := env.Model("mail.mail").Browse(id).Write(map[string]any{
			"state":          "sent",
			"failure_type":   "",
			"failure_reason": "",
			"message_id":     messageID,
		}); err != nil {
			return result, err
		}
		result.Sent++
	}
	if err := limitRun.triggerMailQueueCron(env); err != nil {
		return result, err
	}
	return result, nil
}

func mailDeliveryItems(env *record.Env, row map[string]any, message Message) []mailDeliveryItem {
	items := []mailDeliveryItem{}
	to, toEmails, toInvalid := recipientAddressList(message.To)
	cc, _, ccInvalid := recipientAddressList(message.CC)
	if len(to) > 0 || len(cc) > 0 || toInvalid || ccInvalid {
		rawMessage := message
		rawMessage.To = strings.Join(to, ",")
		rawMessage.CC = strings.Join(cc, ",")
		if len(to) == 0 && toInvalid {
			rawMessage.To = strings.TrimSpace(message.To)
		}
		if len(cc) == 0 && ccInvalid {
			rawMessage.CC = strings.TrimSpace(message.CC)
		}
		items = append(items, mailDeliveryItem{Message: rawMessage, Emails: toEmails})
	}
	for _, partnerID := range uniqueIDs(int64s(row["recipient_ids"])) {
		partnerMessage := message
		partnerMessage.To = partnerEmailList(env, partnerID)
		partnerMessage.CC = ""
		items = append(items, mailDeliveryItem{Message: partnerMessage, PartnerID: partnerID})
	}
	return items
}

func recipientAddressList(value string) ([]string, []string, bool) {
	formatted := []string{}
	emails := []string{}
	seen := map[string]bool{}
	invalid := false
	normalized := strings.NewReplacer(";", ",", "\n", ",").Replace(value)
	if addresses, err := netmail.ParseAddressList(normalized); err == nil {
		for _, address := range addresses {
			email := strings.ToLower(strings.TrimSpace(address.Address))
			if email == "" || emailDomain(email) == "" || seen[email] {
				continue
			}
			seen[email] = true
			address.Address = email
			formatted = append(formatted, mailAddressString(address))
			emails = append(emails, email)
		}
		return formatted, emails, false
	}
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		part := strings.TrimSpace(raw)
		if part == "" || !strings.Contains(part, "@") {
			continue
		}
		address, err := netmail.ParseAddress(part)
		if err != nil || strings.TrimSpace(address.Address) == "" || emailDomain(address.Address) == "" {
			invalid = true
			formatted = append(formatted, part)
			continue
		}
		email := strings.ToLower(strings.TrimSpace(address.Address))
		if seen[email] {
			continue
		}
		seen[email] = true
		address.Address = email
		formatted = append(formatted, mailAddressString(address))
		emails = append(emails, email)
	}
	return formatted, emails, invalid
}

func mailAddressString(address *netmail.Address) string {
	if address == nil {
		return ""
	}
	if strings.TrimSpace(address.Name) == "" {
		return strings.ToLower(strings.TrimSpace(address.Address))
	}
	return address.String()
}

func messageHasRecipient(message Message) bool {
	return strings.TrimSpace(message.To) != "" || strings.TrimSpace(message.CC) != ""
}

func mailAutoDeleteAllowed(failedCount int, failureType string) bool {
	if failedCount == 0 {
		return true
	}
	return failureType == "mail_email_missing" || failureType == "mail_email_invalid"
}

func mailMessageFromRow(env *record.Env, id int64, row map[string]any, now time.Time) (Message, error) {
	headers := mailHeadersFromAny(row["headers"])
	if references := strings.TrimSpace(stringAny(row["references"])); references != "" {
		if _, exists := headers["References"]; !exists {
			headers["References"] = references
		}
	}
	if messageID := strings.TrimSpace(stringAny(row["message_id"])); messageID != "" {
		if _, exists := headers["Message-Id"]; !exists {
			headers["Message-Id"] = messageID
		}
	}
	return Message{
		ID:        id,
		From:      stringAny(row["email_from"]),
		To:        stringAny(row["email_to"]),
		CC:        stringAny(row["email_cc"]),
		ReplyTo:   stringAny(row["reply_to"]),
		Subject:   stringAny(row["subject"]),
		Body:      stringAny(row["body_html"]),
		Headers:   headers,
		CreatedAt: now,
	}, nil
}

func applyMassMailingTrackingBody(env *record.Env, mailID int64, row map[string]any, message *Message) error {
	if message == nil || mailID == 0 || int64FromAny(row["mailing_id"]) == 0 || strings.TrimSpace(message.Body) == "" {
		return nil
	}
	traces, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(traces) == 0 {
		return err
	}
	traces = nonCanceledMailingTraceRows(traces)
	if len(traces) == 0 {
		return nil
	}
	sort.SliceStable(traces, func(i, j int) bool {
		return int64FromAny(traces[i]["id"]) < int64FromAny(traces[j]["id"])
	})
	traceID := int64FromAny(traces[0]["id"])
	if traceID == 0 {
		return nil
	}
	body := replaceMassMailingPortalLinks(env, mailID, row, traces[0], message.Body)
	body = appendMailingTraceIDToTrackedLinks(body, traceID)
	if trackingURL := massMailingOpenTrackingURL(env, mailID); trackingURL != "" {
		body = appendMassMailingOpenPixel(body, trackingURL)
	}
	message.Body = body
	applyMassMailingPortalHeaders(env, row, traces[0], message)
	return nil
}

func nonCanceledMailingTraceRows(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["trace_status"])) == "cancel" {
			continue
		}
		out = append(out, row)
	}
	return out
}

type massMailingSuppression struct {
	emails map[string]bool
}

func (s massMailingSuppression) empty() bool {
	return len(s.emails) == 0
}

func shouldCancelMassMailingBlacklistedMail(env *record.Env, row map[string]any) (bool, error) {
	if env == nil || int64FromAny(row["mailing_id"]) == 0 {
		return false, nil
	}
	mailingID := int64FromAny(row["mailing_id"])
	if !massMailingUseExclusionList(env, mailingID, nil) {
		return false, nil
	}
	if email := firstActiveBlacklistedMassMailingEmail(env, RenderedMessage{
		To: stringAny(row["email_to"]),
		CC: stringAny(row["email_cc"]),
	}, int64s(row["recipient_ids"])); email != "" {
		return true, nil
	}
	return false, nil
}

func shouldCancelDuplicateMassMailingQueuedMail(env *record.Env, mailID int64, row map[string]any) (bool, error) {
	if env == nil || mailID == 0 || int64FromAny(row["mailing_id"]) == 0 {
		return false, nil
	}
	mailingID := int64FromAny(row["mailing_id"])
	emails := massMailingRenderedEmails(env, RenderedMessage{
		To: stringAny(row["email_to"]),
		CC: stringAny(row["email_cc"]),
	}, int64s(row["recipient_ids"]))
	return massMailingEmailsAlreadySeen(env, mailingID, emails, nil, mailID)
}

func massMailingListSuppression(env *record.Env, mailID int64, row map[string]any) (massMailingSuppression, error) {
	if env == nil || mailID == 0 || int64FromAny(row["mailing_id"]) == 0 {
		return massMailingSuppression{}, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.mailing"); !ok {
		return massMailingSuppression{}, nil
	}
	mailingRows, err := systemEnv.Model("mailing.mailing").Browse(int64FromAny(row["mailing_id"])).Read("mailing_on_mailing_list", "contact_list_ids")
	if err != nil || len(mailingRows) == 0 {
		return massMailingSuppression{}, err
	}
	if !boolAny(mailingRows[0]["mailing_on_mailing_list"]) {
		return massMailingSuppression{}, nil
	}
	listIDs := uniqueIDs(int64s(mailingRows[0]["contact_list_ids"]))
	if len(listIDs) == 0 {
		return massMailingSuppression{}, nil
	}
	emails, contactIDs, err := massMailingListMailRecipientIdentities(systemEnv, mailID, row)
	if err != nil {
		return massMailingSuppression{}, err
	}
	contactIDsByEmail, err := mailingContactIDsByEmail(systemEnv, emails, contactIDs)
	if err != nil || len(contactIDsByEmail) == 0 {
		return massMailingSuppression{}, err
	}
	if _, ok := systemEnv.ModelMetadata("mailing.subscription"); !ok {
		return massMailingSuppression{}, nil
	}
	suppressed := map[string]bool{}
	for email, emailContactIDs := range contactIDsByEmail {
		optOut, optIn, err := mailingListSubscriptionState(systemEnv, emailContactIDs, listIDs)
		if err != nil {
			return massMailingSuppression{}, err
		}
		if optOut && !optIn {
			suppressed[email] = true
		}
	}
	return massMailingSuppression{emails: suppressed}, nil
}

func massMailingListMailRecipientIdentities(env *record.Env, mailID int64, row map[string]any) ([]string, []int64, error) {
	emails := []string{}
	contactIDs := []int64{}
	traces, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil {
		return nil, nil, err
	}
	for _, trace := range traces {
		if strings.TrimSpace(stringAny(trace["model"])) == "mailing.contact" {
			contactIDs = append(contactIDs, int64FromAny(trace["res_id"]))
		}
		if email := normalizedEmailAddress(stringAny(trace["email"])); email != "" {
			emails = append(emails, email)
		}
	}
	for _, fieldName := range []string{"email_to", "email_cc"} {
		_, parsedEmails, _ := recipientAddressList(stringAny(row[fieldName]))
		for _, email := range parsedEmails {
			emails = append(emails, email)
		}
	}
	for _, partnerID := range uniqueIDs(int64s(row["recipient_ids"])) {
		if email := normalizedEmailAddress(partnerEmail(env, partnerID)); email != "" {
			emails = append(emails, email)
		}
	}
	if len(contactIDs) > 0 {
		rows, err := env.Model("mailing.contact").Browse(uniqueIDs(contactIDs)...).Read("id", "email_normalized", "email")
		if err != nil {
			return nil, nil, err
		}
		for _, contact := range rows {
			if email := normalizedEmailAddress(firstNonEmptyMailString(stringAny(contact["email_normalized"]), stringAny(contact["email"]))); email != "" {
				emails = append(emails, email)
			}
		}
	}
	return uniqueStrings(emails), uniqueIDs(contactIDs), nil
}

func mailingContactIDsByEmail(env *record.Env, emails []string, contactIDs []int64) (map[string][]int64, error) {
	out := map[string][]int64{}
	if env == nil {
		return out, nil
	}
	if _, ok := env.ModelMetadata("mailing.contact"); !ok {
		return out, nil
	}
	for _, email := range uniqueStrings(emails) {
		found, err := env.Model("mailing.contact").Search(domain.Cond("email_normalized", "=", email))
		if err != nil {
			return nil, err
		}
		rows, err := found.Read("id")
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if id := int64FromAny(row["id"]); id != 0 {
				out[email] = append(out[email], id)
			}
		}
	}
	if len(contactIDs) > 0 {
		rows, err := env.Model("mailing.contact").Browse(uniqueIDs(contactIDs)...).Read("id", "email_normalized", "email")
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			email := normalizedEmailAddress(firstNonEmptyMailString(stringAny(row["email_normalized"]), stringAny(row["email"])))
			id := int64FromAny(row["id"])
			if email != "" && id != 0 {
				out[email] = append(out[email], id)
			}
		}
	}
	for email, ids := range out {
		out[email] = uniqueIDs(ids)
	}
	return out, nil
}

func mailingListSubscriptionState(env *record.Env, contactIDs []int64, listIDs []int64) (bool, bool, error) {
	optOut := false
	optIn := false
	for _, contactID := range uniqueIDs(contactIDs) {
		found, err := env.Model("mailing.subscription").Search(domain.Cond("contact_id", "=", contactID))
		if err != nil {
			return false, false, err
		}
		rows, err := found.Read("list_id", "opt_out")
		if err != nil {
			return false, false, err
		}
		for _, subscription := range rows {
			if !containsInt64(listIDs, int64FromAny(subscription["list_id"])) {
				continue
			}
			if boolAny(subscription["opt_out"]) {
				optOut = true
			} else {
				optIn = true
			}
		}
	}
	return optOut, optIn, nil
}

func filterRecipientAddressField(value string, suppressed map[string]bool) string {
	if strings.TrimSpace(value) == "" || len(suppressed) == 0 {
		return value
	}
	normalized := strings.NewReplacer(";", ",", "\n", ",").Replace(value)
	if addresses, err := netmail.ParseAddressList(normalized); err == nil {
		out := make([]string, 0, len(addresses))
		for _, address := range addresses {
			email := normalizedEmailAddress(address.Address)
			if email == "" || suppressed[email] {
				continue
			}
			address.Address = email
			out = append(out, mailAddressString(address))
		}
		return strings.Join(out, ",")
	}
	out := []string{}
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		address, err := netmail.ParseAddress(part)
		if err != nil {
			out = append(out, part)
			continue
		}
		email := normalizedEmailAddress(address.Address)
		if email == "" || suppressed[email] {
			continue
		}
		address.Address = email
		out = append(out, mailAddressString(address))
	}
	return strings.Join(out, ",")
}

func filterPartnerRecipientsByEmail(env *record.Env, partnerIDs []int64, suppressed map[string]bool) ([]int64, error) {
	if len(partnerIDs) == 0 || len(suppressed) == 0 {
		return uniqueIDs(partnerIDs), nil
	}
	out := []int64{}
	for _, partnerID := range uniqueIDs(partnerIDs) {
		if email := normalizedEmailAddress(partnerEmail(env, partnerID)); email != "" && suppressed[email] {
			continue
		}
		out = append(out, partnerID)
	}
	return out, nil
}

func copyMailRow(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for key, value := range row {
		out[key] = value
	}
	return out
}

func replaceMassMailingPortalLinks(env *record.Env, _ int64, row map[string]any, trace map[string]any, body string) string {
	mailingID := int64FromAny(row["mailing_id"])
	email := firstNonEmptyMailString(stringAny(trace["email"]), firstMailQueueEmailAddress(stringAny(row["email_to"])))
	documentID := int64FromAny(trace["res_id"])
	if mailingID == 0 || documentID == 0 || email == "" {
		return body
	}
	baseURL := strings.TrimRight(configParameter(messageSystemEnv(env), "web.base.url"), "/")
	if baseURL == "" {
		baseURL = "http://localhost"
	}
	token := massMailingRecipientToken(env, mailingID, documentID, email)
	if token == "" {
		return body
	}
	values := url.Values{}
	values.Set("document_id", strconv.FormatInt(documentID, 10))
	values.Set("email", email)
	values.Set("hash_token", token)
	unsub := fmt.Sprintf("%s/mailing/%d/confirm_unsubscribe?%s", baseURL, mailingID, values.Encode())
	view := fmt.Sprintf("%s/mailing/%d/view?%s", baseURL, mailingID, values.Encode())
	replacer := strings.NewReplacer(
		`href="/unsubscribe_from_list"`, `href="`+html.EscapeString(unsub)+`"`,
		`href='/unsubscribe_from_list'`, `href='`+html.EscapeString(unsub)+`'`,
		`href="/view"`, `href="`+html.EscapeString(view)+`"`,
		`href='/view'`, `href='`+html.EscapeString(view)+`'`,
	)
	return replacer.Replace(body)
}

func applyMassMailingPortalHeaders(env *record.Env, row map[string]any, trace map[string]any, message *Message) {
	if message == nil {
		return
	}
	mailingID := int64FromAny(row["mailing_id"])
	email := firstNonEmptyMailString(stringAny(trace["email"]), firstMailQueueEmailAddress(stringAny(row["email_to"])))
	documentID := int64FromAny(trace["res_id"])
	token := massMailingRecipientToken(env, mailingID, documentID, email)
	if mailingID == 0 || documentID == 0 || email == "" || token == "" {
		return
	}
	baseURL := strings.TrimRight(configParameter(messageSystemEnv(env), "web.base.url"), "/")
	if baseURL == "" {
		baseURL = "http://localhost"
	}
	values := url.Values{}
	values.Set("document_id", strconv.FormatInt(documentID, 10))
	values.Set("email", email)
	values.Set("hash_token", token)
	oneClick := fmt.Sprintf("%s/mailing/%d/unsubscribe_oneclick?%s", baseURL, mailingID, values.Encode())
	unsubscribe := fmt.Sprintf("%s/mailing/%d/confirm_unsubscribe?%s", baseURL, mailingID, values.Encode())
	if message.Headers == nil {
		message.Headers = map[string]string{}
	}
	if _, ok := message.Headers["List-Unsubscribe"]; !ok {
		message.Headers["List-Unsubscribe"] = fmt.Sprintf("<%s>, <%s>", oneClick, unsubscribe)
	}
	if _, ok := message.Headers["List-Unsubscribe-Post"]; !ok {
		message.Headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}
	if _, ok := message.Headers["Precedence"]; !ok {
		message.Headers["Precedence"] = "list"
	}
	if _, ok := message.Headers["X-Auto-Response-Suppress"]; !ok {
		message.Headers["X-Auto-Response-Suppress"] = "OOF"
	}
}

func massMailingRecipientToken(env *record.Env, mailingID int64, documentID int64, email string) string {
	secret := configParameter(messageSystemEnv(env), "database.secret")
	if secret == "" || mailingID == 0 || documentID == 0 || strings.TrimSpace(email) == "" {
		return ""
	}
	payload := fmt.Sprintf("(%s, %d, %d, %s)", pythonReprString(portalDBName(env)), mailingID, documentID, pythonReprString(email))
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func firstNonEmptyMailString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMailQueueEmailAddress(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		addr, err := netmail.ParseAddress(strings.TrimSpace(part))
		if err == nil && strings.TrimSpace(addr.Address) != "" {
			return strings.TrimSpace(addr.Address)
		}
		if strings.Contains(part, "@") {
			return strings.TrimSpace(part)
		}
	}
	return ""
}

func appendMailingTraceIDToTrackedLinks(body string, traceID int64) string {
	if traceID == 0 || strings.TrimSpace(body) == "" {
		return body
	}
	return massMailingTrackedHrefURLPattern.ReplaceAllStringFunc(body, func(match string) string {
		parts := massMailingTrackedHrefURLPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		linkURL := parts[2]
		if !massMailingTrackedURLNeedsTrace(linkURL) {
			return match
		}
		return parts[1] + linkURL + fmt.Sprintf("/m/%d", traceID) + parts[3]
	})
}

func massMailingTrackedURLNeedsTrace(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsed, err := url.Parse(html.UnescapeString(rawURL))
	if err == nil && strings.HasPrefix(parsed.Path, "/r/") {
		return true
	}
	return false
}

func appendMassMailingOpenPixel(body string, trackingURL string) string {
	if strings.TrimSpace(body) == "" || strings.TrimSpace(trackingURL) == "" {
		return body
	}
	pixel := `<img src="` + html.EscapeString(trackingURL) + `"/>`
	lower := strings.ToLower(body)
	if idx := strings.LastIndex(lower, "</body>"); idx >= 0 {
		return body[:idx] + pixel + body[idx:]
	}
	if idx := strings.LastIndex(lower, "</html>"); idx >= 0 {
		return body[:idx] + pixel + body[idx:]
	}
	return body + pixel
}

func massMailingOpenTrackingURL(env *record.Env, mailID int64) string {
	token := massMailingOpenToken(env, mailID)
	if token == "" {
		return ""
	}
	path := fmt.Sprintf("/mail/track/%d/%s/blank.gif", mailID, token)
	baseURL := strings.TrimRight(configParameter(messageSystemEnv(env), "web.base.url"), "/")
	return baseURL + path
}

func massMailingOpenToken(env *record.Env, mailID int64) string {
	if env == nil || mailID <= 0 {
		return ""
	}
	secret := configParameter(messageSystemEnv(env), "database.secret")
	if secret == "" {
		return ""
	}
	payload := fmt.Sprintf("(%s, %d)", pythonReprString("mass_mailing-mail_mail-open"), mailID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func setOutboundMessageID(env *record.Env, mailID int64, row map[string]any, message *Message) {
	if message == nil {
		return
	}
	if message.Headers == nil {
		message.Headers = map[string]string{}
	}
	if messageHeaderValue(message.Headers, "Message-Id") != "" {
		return
	}
	messageID := outboundMessageID(env, mailID, row)
	if messageID != "" {
		message.Headers["Message-Id"] = messageID
	}
}

func outboundMessageID(env *record.Env, mailID int64, row map[string]any) string {
	if value := normalizeMessageID(stringAny(row["message_id"])); value != "" {
		return value
	}
	mailMessageID := int64FromAny(row["mail_message_id"])
	if env != nil && mailMessageID != 0 {
		if rows, err := messageSystemEnv(env).Model("mail.message").Browse(mailMessageID).Read("message_id"); err == nil && len(rows) != 0 {
			if value := normalizeMessageID(stringAny(rows[0]["message_id"])); value != "" {
				return value
			}
		}
		return fmt.Sprintf("<mail.message-%d@local>", mailMessageID)
	}
	if mailID != 0 {
		return fmt.Sprintf("<mail-%d@local>", mailID)
	}
	return ""
}

func storeOutboundMessageID(env *record.Env, _ int64, mailMessageID int64, messageID string) error {
	messageID = normalizeMessageID(messageID)
	if env == nil || mailMessageID == 0 || messageID == "" {
		return nil
	}
	if _, ok := messageSystemEnv(env).ModelMetadata("mail.message"); !ok {
		return nil
	}
	return messageSystemEnv(env).Model("mail.message").Browse(mailMessageID).Write(map[string]any{"message_id": messageID})
}

func normalizeMessageID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return value
	}
	if strings.Contains(value, "@") && !strings.ContainsAny(value, " \t\r\n<>") {
		return "<" + value + ">"
	}
	return value
}

func mailHeadersFromAny(value any) map[string]string {
	headers := map[string]string{}
	text := strings.TrimSpace(stringAny(value))
	if text == "" {
		return headers
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		for key, value := range decoded {
			if strings.TrimSpace(key) != "" {
				headers[key] = stringAny(value)
			}
		}
		return headers
	}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return headers
}

func applyOutgoingAttachmentPayload(env *record.Env, mailRow map[string]any, serverRow map[string]any, message *Message) error {
	if message == nil {
		return nil
	}
	rows, err := mailAttachmentRows(env, mailAttachmentIDs(env, mailRow))
	if err != nil {
		return err
	}
	rows = removeBodyLinkedAttachmentRows(rows, message.Body)
	rows, linkedRows, err := convertURLAttachmentRows(env, rows)
	if err != nil {
		return err
	}
	rows, oversizedRows, err := convertOversizedRecordAttachmentRows(env, rows, serverRow, message.Body, message.Headers)
	if err != nil {
		return err
	}
	if len(linkedRows) != 0 || len(oversizedRows) != 0 {
		message.Body = appendMailAttachmentLinks(env, message.Body, append(linkedRows, oversizedRows...))
	}
	attachments, err := mailAttachmentsFromRows(rows)
	if err != nil {
		return err
	}
	message.Attachments = attachments
	return nil
}

func mailAttachmentRows(env *record.Env, ids []int64) ([]map[string]any, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := messageSystemEnv(env).Model("ir.attachment").Browse(ids...).Read("id", "name", "res_model", "res_id", "type", "url", "mimetype", "datas", "file_size", "access_token")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return int64FromAny(rows[i]["id"]) < int64FromAny(rows[j]["id"])
	})
	return rows, nil
}

func mailAttachmentsFromRows(rows []map[string]any) ([]Attachment, error) {
	out := make([]Attachment, 0, len(rows))
	for _, row := range rows {
		data, err := attachmentData(row["datas"])
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(stringAny(row["name"]))
		if name == "" {
			name = fmt.Sprintf("attachment-%d", int64FromAny(row["id"]))
		}
		contentType := strings.TrimSpace(stringAny(row["mimetype"]))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		out = append(out, Attachment{Name: name, ContentType: contentType, Data: data})
	}
	return out, nil
}

func removeBodyLinkedAttachmentRows(rows []map[string]any, body string) []map[string]any {
	if strings.TrimSpace(body) == "" || len(rows) == 0 {
		return rows
	}
	linked := map[int64]bool{}
	for _, match := range mailBodyAttachmentLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil && id != 0 {
			linked[id] = true
		}
	}
	if len(linked) == 0 {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if !linked[int64FromAny(row["id"])] {
			out = append(out, row)
		}
	}
	return out
}

func convertURLAttachmentRows(env *record.Env, rows []map[string]any) ([]map[string]any, []map[string]any, error) {
	if len(rows) == 0 {
		return rows, nil, nil
	}
	remaining := make([]map[string]any, 0, len(rows))
	linked := []map[string]any{}
	for _, row := range rows {
		if mailAttachmentURLOnly(row) {
			token, err := ensureAttachmentAccessToken(env, row)
			if err != nil {
				return nil, nil, err
			}
			row["access_token"] = token
			linked = append(linked, row)
			continue
		}
		remaining = append(remaining, row)
	}
	return remaining, linked, nil
}

func convertOversizedRecordAttachmentRows(env *record.Env, rows []map[string]any, serverRow map[string]any, body string, headers map[string]string) ([]map[string]any, []map[string]any, error) {
	if len(rows) == 0 {
		return rows, nil, nil
	}
	recordOwned := []map[string]any{}
	for _, row := range rows {
		if mailAttachmentRecordOwned(row) {
			recordOwned = append(recordOwned, row)
		}
	}
	if len(recordOwned) == 0 {
		return rows, nil, nil
	}
	if mailEstimatedEmailSize(headers, body, rows) <= mailMaxEmailBytes(env, serverRow) {
		return rows, nil, nil
	}
	ownedIDs := map[int64]bool{}
	for _, row := range recordOwned {
		token, err := ensureAttachmentAccessToken(env, row)
		if err != nil {
			return nil, nil, err
		}
		row["access_token"] = token
		ownedIDs[int64FromAny(row["id"])] = true
	}
	remaining := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if !ownedIDs[int64FromAny(row["id"])] {
			remaining = append(remaining, row)
		}
	}
	return remaining, recordOwned, nil
}

func mailAttachmentURLOnly(row map[string]any) bool {
	rawURL := strings.TrimSpace(stringAny(row["url"]))
	if rawURL == "" || attachmentSize(row) != 0 {
		return false
	}
	lower := strings.ToLower(rawURL)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ftp://")
}

func mailAttachmentRecordOwned(row map[string]any) bool {
	return strings.TrimSpace(stringAny(row["res_model"])) != "" && int64FromAny(row["res_id"]) != 0 && strings.TrimSpace(stringAny(row["res_model"])) != "mail.message"
}

func attachmentSize(row map[string]any) int64 {
	if size := int64FromAny(row["file_size"]); size > 0 {
		return size
	}
	data, err := attachmentData(row["datas"])
	if err != nil {
		return 0
	}
	return int64(len(data))
}

func mailEstimatedEmailSize(headers map[string]string, body string, rows []map[string]any) int64 {
	headersData, _ := json.Marshal(headers)
	size := int64(len(headersData)) + int64(len([]byte(body))) + 10*1024
	for _, row := range rows {
		size += attachmentSize(row) * 8 / 6
	}
	return size
}

func mailMaxEmailBytes(env *record.Env, serverRow map[string]any) int64 {
	maxMB := float64FromAny(serverRow["max_email_size"])
	if maxMB <= 0 {
		maxMB = float64FromAny(configParameter(messageSystemEnv(env), "base.default_max_email_size"))
	}
	if maxMB <= 0 {
		maxMB = 10
	}
	return int64(maxMB * 1024 * 1024)
}

func ensureAttachmentAccessToken(env *record.Env, row map[string]any) (string, error) {
	token := strings.TrimSpace(stringAny(row["access_token"]))
	if token != "" {
		return token, nil
	}
	token = randomAttachmentAccessToken()
	if err := messageSystemEnv(env).Model("ir.attachment").Browse(int64FromAny(row["id"])).Write(map[string]any{"access_token": token}); err != nil {
		return "", err
	}
	return token, nil
}

func randomAttachmentAccessToken() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", data[0:4], data[4:6], data[6:8], data[8:10], data[10:16])
}

func appendMailAttachmentLinks(env *record.Env, body string, rows []map[string]any) string {
	if len(rows) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString(body)
	b.WriteString(`<div style="max-width: 900px; width: 100%;"><hr style="background-color:rgb(204,204,204);border:medium none;clear:both;display:block;font-size:0px;min-height:1px;line-height:0; margin: 16px 0px 10px 0px;"/>`)
	for _, row := range rows {
		name := strings.TrimSpace(stringAny(row["name"]))
		if name == "" {
			name = fmt.Sprintf("attachment-%d", int64FromAny(row["id"]))
		}
		href := mailAttachmentDownloadURL(env, row)
		b.WriteString(`<div><a href="`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`" style="font-size: 12px; color: #875A7B; text-decoration:none !important; text-decoration:none; font-weight: 400;">Download `)
		b.WriteString(html.EscapeString(name))
		b.WriteString(`</a></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func mailAttachmentDownloadURL(env *record.Env, row map[string]any) string {
	baseURL := ""
	if env != nil {
		baseURL = strings.TrimRight(configParameter(messageSystemEnv(env), "web.base.url"), "/")
	}
	path := fmt.Sprintf("/web/content/%d?download=1", int64FromAny(row["id"]))
	if token := strings.TrimSpace(stringAny(row["access_token"])); token != "" {
		path += "&access_token=" + url.QueryEscape(token)
	}
	return baseURL + path
}

func mailAttachmentIDs(env *record.Env, row map[string]any) []int64 {
	ids := int64s(row["attachment_ids"])
	messageID := int64FromAny(row["mail_message_id"])
	if messageID == 0 {
		return uniqueIDs(ids)
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil || len(rows) == 0 {
		return uniqueIDs(ids)
	}
	ids = append(ids, int64s(rows[0]["attachment_ids"])...)
	return uniqueIDs(ids)
}

func attachmentData(value any) ([]byte, error) {
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...), nil
	case string:
		text := strings.TrimSpace(typed)
		if _, data, ok := strings.Cut(text, ","); ok {
			text = strings.TrimSpace(data)
		}
		if text == "" {
			return nil, nil
		}
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err == nil {
			return decoded, nil
		}
		return []byte(typed), nil
	default:
		return nil, nil
	}
}

type mailServerSelection struct {
	Row          map[string]any
	From         string
	EnvelopeFrom string
}

type mailAliasDomainContext struct {
	NotificationEmail string
	BounceEmail       string
}

func smtpSenderForMail(env *record.Env, mailRow map[string]any, message Message) (Sender, string, string, map[string]any, error) {
	selection, err := mailServerSelectionForMail(env, mailRow, message)
	if err != nil {
		return nil, "", "", nil, err
	}
	serverRow := selection.Row
	from := strings.TrimSpace(selection.From)
	if from == "" {
		from = strings.TrimSpace(stringAny(serverRow["smtp_user"]))
	}
	if from == "" {
		return nil, "", "", nil, fmt.Errorf("mail sender from required")
	}
	port := int(int64FromAny(serverRow["smtp_port"]))
	if port == 0 {
		port = defaultSMTPPort(stringAny(serverRow["smtp_encryption"]))
	}
	sender, err := NewSMTPSender(SMTPConfig{
		Host:     stringAny(serverRow["smtp_host"]),
		Port:     port,
		Username: stringAny(serverRow["smtp_user"]),
		Password: stringAny(serverRow["smtp_pass"]),
		From:     from,
		TLSMode:  smtpTLSMode(stringAny(serverRow["smtp_encryption"])),
	})
	if err != nil {
		return nil, "", "", nil, err
	}
	return sender, from, selection.EnvelopeFrom, serverRow, nil
}

func mailServerSelectionForMail(env *record.Env, mailRow map[string]any, message Message) (mailServerSelection, error) {
	serverID := int64FromAny(mailRow["mail_server_id"])
	fields := mailServerQueueFields()
	aliasContext := mailAliasDomainContextForMail(env, mailRow)
	defaultFrom := defaultNotificationEmailForContext(env, aliasContext)
	if serverID != 0 {
		rows, err := messageSystemEnv(env).Model("ir.mail_server").Browse(serverID).Read(fields...)
		if err != nil {
			return mailServerSelection{}, err
		}
		if len(rows) == 0 {
			return mailServerSelection{}, fmt.Errorf("mail server unavailable")
		}
		row := rows[0]
		if value, exists := row["active"]; exists && value != nil && !boolAny(value) {
			return mailServerSelection{}, fmt.Errorf("mail server unavailable")
		}
		if !mailServerAllowedForMail(env, mailRow, row, message) {
			return mailServerSelection{}, fmt.Errorf("mail server unauthorized")
		}
		selected := mailServerSelection{Row: row, From: firstNonEmpty(message.From, defaultFrom)}
		selected.EnvelopeFrom = mailEnvelopeFromForContext(env, aliasContext, row, message, selected.From)
		return selected, nil
	}
	systemEnv := messageSystemEnv(env)
	found, err := systemEnv.Model("ir.mail_server").SearchWithOptions(domain.And(), record.SearchOptions{Order: "sequence,id"})
	if err != nil {
		return mailServerSelection{}, err
	}
	rows, err := found.Read(fields...)
	if err != nil {
		return mailServerSelection{}, err
	}
	if len(rows) == 0 {
		return mailServerSelection{}, fmt.Errorf("mail server unavailable")
	}
	rows = activeMailServerRows(rows)
	rows = publicMailServerRows(rows)
	if len(rows) == 0 {
		return mailServerSelection{}, fmt.Errorf("mail server unavailable")
	}
	selection := bestMailServerSelection(rows, message.From, defaultFrom)
	selection.EnvelopeFrom = mailEnvelopeFromForContext(env, aliasContext, selection.Row, message, selection.From)
	return selection, nil
}

func mailServerQueueFields() []string {
	return []string{"smtp_host", "smtp_port", "smtp_encryption", "smtp_authentication", "smtp_user", "smtp_pass", "active", "sequence", "from_filter", "max_email_size", "owner_user_id", "owner_limit_time", "owner_limit_count"}
}

func mailServerThrottleRow(env *record.Env, serverID int64) (map[string]any, error) {
	rows, err := messageSystemEnv(env).Model("ir.mail_server").Browse(serverID).Read(mailServerQueueFields()...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mail server unavailable")
	}
	return rows[0], nil
}

func activeMailServerRows(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		value, exists := row["active"]
		if !exists || value == nil || boolAny(value) {
			out = append(out, row)
		}
	}
	return out
}

func publicMailServerRows(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if int64FromAny(row["owner_user_id"]) == 0 {
			out = append(out, row)
		}
	}
	return out
}

func bestMailServerSelection(rows []map[string]any, from string, notificationFrom string) mailServerSelection {
	if len(rows) == 0 {
		return mailServerSelection{}
	}
	fromAddress := normalizedEmailAddress(from)
	for _, row := range rows {
		if mailServerMatchesExact(row, fromAddress) {
			return mailServerSelection{Row: row, From: strings.TrimSpace(from)}
		}
	}
	domain := emailDomain(fromAddress)
	if domain != "" {
		for _, row := range rows {
			if mailServerMatchesDomain(row, domain) {
				return mailServerSelection{Row: row, From: strings.TrimSpace(from)}
			}
		}
	}
	notificationAddress := normalizedEmailAddress(notificationFrom)
	for _, row := range rows {
		if mailServerMatchesExact(row, notificationAddress) {
			return mailServerSelection{Row: row, From: strings.TrimSpace(notificationFrom)}
		}
	}
	notificationDomain := emailDomain(notificationAddress)
	if notificationDomain != "" {
		for _, row := range rows {
			if mailServerMatchesDomain(row, notificationDomain) {
				return mailServerSelection{Row: row, From: strings.TrimSpace(notificationFrom)}
			}
		}
	}
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["from_filter"])) == "" {
			return mailServerSelection{Row: row, From: firstNonEmpty(notificationFrom, from)}
		}
	}
	return mailServerSelection{Row: rows[0], From: firstNonEmpty(notificationFrom, from)}
}

func mailServerMatchesExact(row map[string]any, address string) bool {
	if address == "" {
		return false
	}
	for _, filter := range mailServerFromFilterParts(row) {
		if strings.Contains(filter, "@") && normalizedEmailAddress(filter) == address {
			return true
		}
	}
	return false
}

func mailServerMatchesDomain(row map[string]any, domain string) bool {
	if domain == "" {
		return false
	}
	for _, filter := range mailServerFromFilterParts(row) {
		filter = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(filter)), "@")
		if filter != "" && !strings.Contains(filter, "@") && filter == domain {
			return true
		}
	}
	return false
}

func mailServerAllowsAddress(row map[string]any, address string) bool {
	if strings.TrimSpace(stringAny(row["from_filter"])) == "" {
		return true
	}
	normalized := normalizedEmailAddress(address)
	return mailServerMatchesExact(row, normalized) || mailServerMatchesDomain(row, emailDomain(normalized))
}

func mailServerFromFilterParts(row map[string]any) []string {
	raw := stringAny(row["from_filter"])
	parts := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	}) {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func mailServerAllowedForMail(env *record.Env, mailRow map[string]any, serverRow map[string]any, message Message) bool {
	ownerUserID := int64FromAny(serverRow["owner_user_id"])
	if ownerUserID == 0 {
		return true
	}
	serverID := int64FromAny(serverRow["id"])
	if !personalMailServerMatchesFrom(serverRow, message.From) {
		return false
	}
	ownerOutgoingServerID := userOutgoingMailServerID(env, ownerUserID)
	if ownerOutgoingServerID != serverID {
		return false
	}
	messageOwnerID := mailAuthorUserID(env, int64FromAny(mailRow["mail_message_id"]))
	if messageOwnerID == 0 && env != nil {
		messageOwnerID = env.Context().UserID
	}
	return ownerUserID == messageOwnerID
}

func personalMailServerMatchesFrom(row map[string]any, from string) bool {
	fromAddress := normalizedEmailAddress(from)
	if fromAddress == "" {
		return false
	}
	for _, filter := range mailServerFromFilterParts(row) {
		if strings.Contains(filter, "@") && normalizedEmailAddress(filter) == fromAddress {
			return true
		}
	}
	return false
}

func userOutgoingMailServerID(env *record.Env, userID int64) int64 {
	if env == nil || userID == 0 {
		return 0
	}
	rows, err := messageSystemEnv(env).Model("res.users").Browse(userID).Read("outgoing_mail_server_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["outgoing_mail_server_id"])
}

type personalMailLimitRun struct {
	servers      map[int64]*personalMailLimitServerRun
	nextTrigger  time.Time
	hasTriggerAt bool
}

type personalMailLimitServerRun struct {
	minute time.Time
	count  int64
}

func newPersonalMailLimitRun() *personalMailLimitRun {
	return &personalMailLimitRun{servers: map[int64]*personalMailLimitServerRun{}}
}

func (r *personalMailLimitRun) delayedAt(serverID int64, serverMinute time.Time, serverCount int64, limit int64, weight int64) time.Time {
	if r == nil {
		return serverMinute.Add(time.Minute)
	}
	if weight <= 0 {
		weight = 1
	}
	state := r.servers[serverID]
	if state == nil {
		state = &personalMailLimitServerRun{minute: serverMinute, count: serverCount}
		r.servers[serverID] = state
	}
	if state.count < limit {
		state.count += weight
	} else {
		state.minute = state.minute.Add(time.Minute)
		state.count = weight
	}
	if !r.hasTriggerAt || state.minute.Before(r.nextTrigger) {
		r.nextTrigger = state.minute
		r.hasTriggerAt = true
	}
	return state.minute
}

func (r *personalMailLimitRun) triggerMailQueueCron(env *record.Env) error {
	if r == nil || !r.hasTriggerAt {
		return nil
	}
	return triggerMailQueueCronAt(env, r.nextTrigger.Add(59*time.Second))
}

func preparePersonalMailForLimit(env *record.Env, mailID int64, mailRow map[string]any, serverRow map[string]any, now time.Time, limitRun *personalMailLimitRun) (int64, map[string]any, bool, error) {
	if env == nil || mailID == 0 || int64FromAny(serverRow["owner_user_id"]) == 0 {
		return mailID, mailRow, false, nil
	}
	serverID := int64FromAny(serverRow["id"])
	if serverID == 0 {
		return mailID, mailRow, false, nil
	}
	limit := personalMailServerLimit(env)
	currentMinute := queueNow(now).Truncate(time.Minute)
	serverMinute := timeValue(serverRow["owner_limit_time"])
	count := int64FromAny(serverRow["owner_limit_count"])
	needsServerWrite := false
	if serverMinute.IsZero() || serverMinute.Before(currentMinute) || serverMinute.After(currentMinute) {
		serverMinute = currentMinute
		count = 0
		needsServerWrite = true
	}
	recipientIDs := uniqueIDs(int64s(mailRow["recipient_ids"]))
	recipientCount := int64(len(recipientIDs))
	if recipientCount == 0 {
		if count >= limit {
			if needsServerWrite {
				if err := env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{
					"owner_limit_time":  serverMinute,
					"owner_limit_count": count,
				}); err != nil {
					return 0, nil, false, err
				}
			}
			delayedAt := limitRun.delayedAt(serverID, serverMinute, count, limit, 1)
			return mailID, mailRow, true, env.Model("mail.mail").Browse(mailID).Write(map[string]any{
				"scheduled_date": delayedAt,
			})
		}
		return mailID, mailRow, false, env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{
			"owner_limit_time":  serverMinute,
			"owner_limit_count": count + 1,
		})
	}
	remaining := limit - count
	if remaining <= 0 {
		if needsServerWrite {
			if err := env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{
				"owner_limit_time":  serverMinute,
				"owner_limit_count": count,
			}); err != nil {
				return 0, nil, false, err
			}
		}
		weight := recipientCount
		if weight <= 0 {
			weight = 1
		}
		delayedAt := limitRun.delayedAt(serverID, serverMinute, count, limit, weight)
		return mailID, mailRow, true, env.Model("mail.mail").Browse(mailID).Write(map[string]any{
			"scheduled_date": delayedAt,
		})
	}
	if recipientCount > remaining {
		sendIDs := recipientIDs[:remaining]
		deferredIDs := recipientIDs[remaining:]
		delayedAt := limitRun.delayedAt(serverID, serverMinute, limit, limit, int64(len(deferredIDs)))
		sendID, sendRow, err := splitPersonalMailForLimit(env, mailID, mailRow, sendIDs, deferredIDs, delayedAt)
		if err != nil {
			return 0, nil, false, err
		}
		if err := env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{
			"owner_limit_time":  serverMinute,
			"owner_limit_count": limit,
		}); err != nil {
			return 0, nil, false, err
		}
		return sendID, sendRow, false, nil
	}
	return mailID, mailRow, false, env.Model("ir.mail_server").Browse(serverID).Write(map[string]any{
		"owner_limit_time":  serverMinute,
		"owner_limit_count": count + recipientCount,
	})
}

func splitPersonalMailForLimit(env *record.Env, mailID int64, mailRow map[string]any, sendRecipientIDs []int64, deferredRecipientIDs []int64, deferredAt time.Time) (int64, map[string]any, error) {
	createEnv := env
	if creatorID := int64FromAny(mailRow["create_uid"]); creatorID != 0 && env != nil {
		ctx := env.Context()
		ctx.UserID = creatorID
		createEnv = env.WithContext(ctx)
	}
	copyID, err := createEnv.Model("mail.mail").Create(personalMailLimitCopyValues(mailRow, sendRecipientIDs))
	if err != nil {
		return 0, nil, err
	}
	if err := env.Model("mail.mail").Browse(mailID).Write(map[string]any{
		"recipient_ids":  deferredRecipientIDs,
		"email_to":       "",
		"email_cc":       "",
		"scheduled_date": deferredAt,
		"state":          "outgoing",
		"failure_type":   "",
		"failure_reason": "",
		"message_id":     "",
	}); err != nil {
		return 0, nil, err
	}
	if err := reassignSplitMailNotifications(env, mailID, copyID, sendRecipientIDs, splitMailRawEmails(mailRow)); err != nil {
		return 0, nil, err
	}
	rows, err := env.Model("mail.mail").Browse(copyID).Read("mail_message_id", "recipient_ids", "attachment_ids", "mail_server_id", "record_alias_domain_id", "record_company_id", "email_from", "email_to", "email_cc", "reply_to", "subject", "body_html", "state", "failure_type", "failure_reason", "scheduled_date", "retry_count", "max_retries", "auto_delete", "message_id", "references", "headers", "is_notification", "fetchmail_server_id", "mailing_id", "create_uid", "create_date")
	if err != nil {
		return 0, nil, err
	}
	if len(rows) == 0 {
		return 0, nil, fmt.Errorf("split mail unavailable")
	}
	return copyID, rows[0], nil
}

func personalMailLimitCopyValues(row map[string]any, recipientIDs []int64) map[string]any {
	values := map[string]any{}
	for _, fieldName := range []string{"mail_message_id", "attachment_ids", "mail_server_id", "record_alias_domain_id", "record_company_id", "email_from", "email_to", "email_cc", "reply_to", "subject", "body_html", "state", "retry_count", "max_retries", "auto_delete", "references", "headers", "is_notification", "fetchmail_server_id", "mailing_id", "create_uid"} {
		if value, ok := row[fieldName]; ok {
			values[fieldName] = value
		}
	}
	values["recipient_ids"] = append([]int64(nil), recipientIDs...)
	values["state"] = "outgoing"
	values["failure_type"] = ""
	values["failure_reason"] = ""
	values["scheduled_date"] = nil
	values["message_id"] = ""
	return values
}

func reassignSplitMailNotifications(env *record.Env, sourceMailID int64, targetMailID int64, partnerIDs []int64, rawEmails []string) error {
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_mail_id", "=", sourceMailID))
	if err != nil {
		return err
	}
	rows, err := found.Read("res_partner_id", "mail_email_address", "notification_type", "notification_status")
	if err != nil {
		return err
	}
	partners := map[int64]bool{}
	for _, id := range partnerIDs {
		if id != 0 {
			partners[id] = true
		}
	}
	emails := map[string]bool{}
	for _, email := range rawEmails {
		if normalized := normalizedEmailAddress(email); normalized != "" {
			emails[normalized] = true
		}
	}
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id == 0 {
			continue
		}
		if !mailNotificationPostprocessable(row) {
			continue
		}
		partnerID := int64FromAny(row["res_partner_id"])
		email := normalizedEmailAddress(stringAny(row["mail_email_address"]))
		if partners[partnerID] || emails[email] {
			if err := env.Model("mail.notification").Browse(id).Write(map[string]any{"mail_mail_id": targetMailID}); err != nil {
				return err
			}
		}
	}
	return nil
}

func splitMailRawEmails(row map[string]any) []string {
	out := []string{}
	for _, fieldName := range []string{"email_to", "email_cc"} {
		_, emails, _ := recipientAddressList(stringAny(row[fieldName]))
		out = append(out, emails...)
	}
	return out
}

func personalMailServerLimit(env *record.Env) int64 {
	raw := strings.TrimSpace(configParameter(messageSystemEnv(env), "mail.server.personal.limit.minutes"))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 30
	}
	return value
}

func triggerMailQueueCronAt(env *record.Env, at time.Time) error {
	if env == nil || at.IsZero() {
		return nil
	}
	cronID, err := mailQueueCronID(env)
	if err != nil || cronID == 0 {
		return err
	}
	_, err = env.Model("ir.cron.trigger").Create(map[string]any{
		"cron_id": cronID,
		"call_at": at,
	})
	if err != nil && strings.Contains(err.Error(), "unknown model ir.cron.trigger") {
		return nil
	}
	return err
}

func mailQueueCronID(env *record.Env) (int64, error) {
	id, err := mailQueueCronIDByXMLID(env)
	if err != nil || id != 0 {
		return id, err
	}
	return mailQueueCronIDByActionName(env)
}

func mailQueueCronIDByXMLID(env *record.Env) (int64, error) {
	found, err := env.Model("ir.model.data").Search(domain.Cond("complete_name", "=", "mail.ir_cron_mail_scheduler_action"))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.data") {
			return 0, nil
		}
		return 0, err
	}
	rows, err := found.Read("model", "res_id")
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if stringAny(row["model"]) == "ir.cron" {
			return int64FromAny(row["res_id"]), nil
		}
	}
	return 0, nil
}

func mailQueueCronIDByActionName(env *record.Env) (int64, error) {
	found, err := env.Model("ir.cron").Search(domain.Cond("action_name", "=", mailProcessQueueActionName))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ir.cron") {
			return 0, nil
		}
		return 0, err
	}
	rows, err := found.Read("action_name")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int64FromAny(rows[0]["id"]), nil
}

func mailAuthorUserID(env *record.Env, messageID int64) int64 {
	if env == nil || messageID == 0 {
		return 0
	}
	systemEnv := messageSystemEnv(env)
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("create_uid", "author_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	if createUID := int64FromAny(rows[0]["create_uid"]); createUID != 0 {
		return createUID
	}
	partnerID := int64FromAny(rows[0]["author_id"])
	if partnerID == 0 {
		return 0
	}
	found, err := systemEnv.Model("res.users").Search(domain.Cond("partner_id", "=", partnerID))
	if err != nil {
		return 0
	}
	userRows, err := found.Read("id")
	if err != nil || len(userRows) == 0 {
		return 0
	}
	return int64FromAny(userRows[0]["id"])
}

func setDefaultReturnPathHeader(env *record.Env, mailRow map[string]any, message *Message) {
	if message == nil {
		return
	}
	if message.Headers == nil {
		message.Headers = map[string]string{}
	}
	if messageHeaderValue(message.Headers, "Return-Path") != "" {
		return
	}
	if bounce := defaultBounceEmailForContext(env, mailAliasDomainContextForMail(env, mailRow)); bounce != "" {
		message.Headers["Return-Path"] = bounce
	}
}

func mailEnvelopeFrom(env *record.Env, serverRow map[string]any, message Message, selectedFrom string) string {
	return mailEnvelopeFromForContext(env, mailAliasDomainContext{}, serverRow, message, selectedFrom)
}

func mailEnvelopeFromForContext(env *record.Env, aliasContext mailAliasDomainContext, serverRow map[string]any, message Message, selectedFrom string) string {
	bounce := firstNonEmpty(messageHeaderValue(message.Headers, "Return-Path"), defaultBounceEmailForContext(env, aliasContext))
	if validEmailAddress(bounce) && mailServerAllowsAddress(serverRow, bounce) {
		return bounce
	}
	return firstNonEmpty(selectedFrom, message.From, bounce)
}

func defaultNotificationEmail(env *record.Env) string {
	return defaultNotificationEmailForContext(env, mailAliasDomainContext{})
}

func defaultNotificationEmailForContext(env *record.Env, aliasContext mailAliasDomainContext) string {
	if env == nil {
		return ""
	}
	if validEmailAddress(aliasContext.NotificationEmail) {
		return strings.TrimSpace(aliasContext.NotificationEmail)
	}
	if value := contextString(env, "domain_notifications_email"); validEmailAddress(value) {
		return strings.TrimSpace(value)
	}
	systemEnv := messageSystemEnv(env)
	if value := companyAliasDomainDefaultFromEmail(systemEnv, env.Context().CompanyID); value != "" {
		return value
	}
	catchallDomain := strings.TrimSpace(configParameter(systemEnv, "mail.catchall.domain"))
	for _, key := range []string{"mail.default.from", "email_from"} {
		if value := configEmailValue(configParameter(systemEnv, key), catchallDomain); value != "" {
			return value
		}
	}
	if catchallDomain != "" {
		return "notifications@" + catchallDomain
	}
	return ""
}

func defaultBounceEmail(env *record.Env) string {
	return defaultBounceEmailForContext(env, mailAliasDomainContext{})
}

func defaultBounceEmailForContext(env *record.Env, aliasContext mailAliasDomainContext) string {
	if env == nil {
		return ""
	}
	if validEmailAddress(aliasContext.BounceEmail) {
		return strings.TrimSpace(aliasContext.BounceEmail)
	}
	if value := contextString(env, "domain_bounce_address"); validEmailAddress(value) {
		return strings.TrimSpace(value)
	}
	systemEnv := messageSystemEnv(env)
	if value := companyAliasDomainAddress(systemEnv, env.Context().CompanyID, "bounce_email", "bounce_alias"); value != "" {
		return value
	}
	catchallDomain := strings.TrimSpace(configParameter(systemEnv, "mail.catchall.domain"))
	if value := configEmailValue(configParameter(systemEnv, "mail.bounce.alias"), catchallDomain); value != "" {
		return value
	}
	if value := configEmailValue(configParameter(systemEnv, "email_from"), catchallDomain); value != "" {
		return value
	}
	return ""
}

func contextString(env *record.Env, key string) string {
	if env == nil || env.Context().Values == nil {
		return ""
	}
	return strings.TrimSpace(stringAny(env.Context().Values[key]))
}

func companyAliasDomainDefaultFromEmail(env *record.Env, companyID int64) string {
	return companyAliasDomainAddress(env, companyID, "default_from_email", "default_from")
}

func companyAliasDomainAddress(env *record.Env, companyID int64, emailField string, aliasField string) string {
	if env == nil || companyID == 0 {
		return ""
	}
	if _, ok := env.ModelMetadata("mail.alias.domain"); !ok {
		return ""
	}
	aliasDomainID := companyAliasDomainID(env, companyID)
	if aliasDomainID == 0 {
		return ""
	}
	return aliasDomainAddress(env, aliasDomainID, emailField, aliasField)
}

func companyAliasDomainID(env *record.Env, companyID int64) int64 {
	if env == nil || companyID == 0 {
		return 0
	}
	rows, err := env.Model("res.company").Browse(companyID).Read("alias_domain_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["alias_domain_id"])
}

func firstAliasDomainID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	found, err := env.Model("mail.alias.domain").SearchWithOptions(domain.And(), record.SearchOptions{Order: "sequence,id", Limit: 1})
	if err != nil {
		return 0
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["id"])
}

func aliasDomainAddress(env *record.Env, aliasDomainID int64, emailField string, aliasField string) string {
	if env == nil || aliasDomainID == 0 {
		return ""
	}
	domainRows, err := env.Model("mail.alias.domain").Browse(aliasDomainID).Read(emailField, aliasField, "name")
	if err != nil || len(domainRows) == 0 {
		return ""
	}
	row := domainRows[0]
	if value := configEmailValue(stringAny(row[emailField]), stringAny(row["name"])); value != "" {
		return value
	}
	return configEmailValue(stringAny(row[aliasField]), stringAny(row["name"]))
}

func aliasDomainContext(env *record.Env, aliasDomainID int64) mailAliasDomainContext {
	if env == nil || aliasDomainID == 0 {
		return mailAliasDomainContext{}
	}
	return mailAliasDomainContext{
		NotificationEmail: aliasDomainAddress(env, aliasDomainID, "default_from_email", "default_from"),
		BounceEmail:       aliasDomainAddress(env, aliasDomainID, "bounce_email", "bounce_alias"),
	}
}

func companyAliasDomainContext(env *record.Env, companyID int64) mailAliasDomainContext {
	if env == nil || companyID == 0 {
		return mailAliasDomainContext{}
	}
	return mailAliasDomainContext{
		NotificationEmail: companyAliasDomainAddress(env, companyID, "default_from_email", "default_from"),
		BounceEmail:       companyAliasDomainAddress(env, companyID, "bounce_email", "bounce_alias"),
	}
}

func mailAliasDomainContextForMail(env *record.Env, mailRow map[string]any) mailAliasDomainContext {
	if env == nil {
		return mailAliasDomainContext{}
	}
	systemEnv := messageSystemEnv(env)
	if aliasDomainID := int64FromAny(mailRow["record_alias_domain_id"]); aliasDomainID != 0 {
		if ctx := aliasDomainContext(systemEnv, aliasDomainID); ctx.NotificationEmail != "" || ctx.BounceEmail != "" {
			return ctx
		}
	}
	if companyID := int64FromAny(mailRow["record_company_id"]); companyID != 0 {
		if ctx := companyAliasDomainContext(systemEnv, companyID); ctx.NotificationEmail != "" || ctx.BounceEmail != "" {
			return ctx
		}
	}
	messageID := int64FromAny(mailRow["mail_message_id"])
	if messageID != 0 {
		rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("record_alias_domain_id", "record_company_id", "model", "res_id")
		if err == nil && len(rows) != 0 {
			messageRow := rows[0]
			if aliasDomainID := int64FromAny(messageRow["record_alias_domain_id"]); aliasDomainID != 0 {
				if ctx := aliasDomainContext(systemEnv, aliasDomainID); ctx.NotificationEmail != "" || ctx.BounceEmail != "" {
					return ctx
				}
			}
			if companyID := int64FromAny(messageRow["record_company_id"]); companyID != 0 {
				if ctx := companyAliasDomainContext(systemEnv, companyID); ctx.NotificationEmail != "" || ctx.BounceEmail != "" {
					return ctx
				}
			}
			if companyID := mailRecordCompanyID(systemEnv, stringAny(messageRow["model"]), int64FromAny(messageRow["res_id"])); companyID != 0 {
				if ctx := companyAliasDomainContext(systemEnv, companyID); ctx.NotificationEmail != "" || ctx.BounceEmail != "" {
					return ctx
				}
			}
		}
	}
	return mailAliasDomainContext{}
}

func mailRecordCompanyID(env *record.Env, modelName string, resID int64) int64 {
	modelName = strings.TrimSpace(modelName)
	if env == nil || modelName == "" || resID == 0 {
		return 0
	}
	if modelName == "res.company" {
		return resID
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return 0
	}
	if _, ok := meta.Fields["company_id"]; !ok {
		return 0
	}
	rows, err := env.Model(modelName).Browse(resID).Read("company_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["company_id"])
}

func configEmailValue(value string, domain string) string {
	value = strings.TrimSpace(value)
	domain = strings.TrimSpace(domain)
	if value == "" {
		return ""
	}
	if validEmailAddress(value) {
		return value
	}
	if strings.Contains(value, "@") || domain == "" {
		return ""
	}
	candidate := strings.Trim(value, " @") + "@" + strings.TrimPrefix(domain, "@")
	if validEmailAddress(candidate) {
		return candidate
	}
	return ""
}

func validEmailAddress(value string) bool {
	address := normalizedEmailAddress(value)
	return address != "" && emailDomain(address) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func float64FromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func normalizedEmailAddress(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	address, err := parseSingleAddress(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(address)
}

func emailDomain(address string) string {
	_, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(address)), "@")
	if !ok {
		return ""
	}
	return domain
}

func smtpTLSMode(value string) SMTPTLSMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ssl", "tls":
		return SMTPTLSImplicit
	case "starttls":
		return SMTPTLSStartTLS
	default:
		return SMTPTLSNone
	}
}

func defaultSMTPPort(value string) int {
	if smtpTLSMode(value) == SMTPTLSImplicit {
		return 465
	}
	return 25
}

func DueMailIDs(env *record.Env, options QueueOptions) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail queue requires env")
	}
	now := queueNow(options.Now)
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultQueueBatchSize
	}
	found, err := env.Model("mail.mail").Search(domain.Cond("state", "=", "outgoing"))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("scheduled_date", "create_date")
	if err != nil {
		return nil, err
	}
	allowed := map[int64]bool{}
	for _, id := range options.EmailIDs {
		if id != 0 {
			allowed[id] = true
		}
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if len(allowed) > 0 && !allowed[id] {
			continue
		}
		scheduled := timeValue(row["scheduled_date"])
		if !scheduled.IsZero() && scheduled.After(now) {
			continue
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		leftDate := dueMailCreateDate(rows, out[i])
		rightDate := dueMailCreateDate(rows, out[j])
		if !leftDate.Equal(rightDate) {
			return leftDate.Before(rightDate)
		}
		return out[i] < out[j]
	})
	if len(out) > batchSize {
		out = out[:batchSize]
	}
	return out, nil
}

func dueMailCreateDate(rows []map[string]any, id int64) time.Time {
	for _, row := range rows {
		if int64FromAny(row["id"]) == id {
			return timeValue(row["create_date"])
		}
	}
	return time.Time{}
}

func RetryMails(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mail queue requires env")
	}
	for _, id := range uniqueIDs(ids) {
		rows, err := env.Model("mail.mail").Browse(id).Read("state")
		if err != nil {
			return err
		}
		if len(rows) == 0 || stringAny(rows[0]["state"]) != "exception" {
			continue
		}
		if err := env.Model("mail.mail").Browse(id).Write(map[string]any{"state": "outgoing"}); err != nil {
			return err
		}
	}
	return nil
}

func CancelMails(env *record.Env, ids []int64) error {
	if env == nil {
		return fmt.Errorf("mail queue requires env")
	}
	for _, id := range uniqueIDs(ids) {
		if err := env.Model("mail.mail").Browse(id).Write(map[string]any{"state": "cancel"}); err != nil {
			return err
		}
	}
	return nil
}

func markMailCanceled(env *record.Env, id int64, failureType string, failureReason string) error {
	failureType = firstText(failureType, "mail_bl")
	if err := env.Model("mail.mail").Browse(id).Write(map[string]any{
		"state":          "cancel",
		"failure_type":   failureType,
		"failure_reason": failureReason,
	}); err != nil {
		return err
	}
	if err := markMailingTraceCanceled(env, id, failureType); err != nil {
		return err
	}
	return updateLinkedNotifications(env, id, "cancel", failureType, failureReason)
}

func markMailException(env *record.Env, id int64, failureType string, failureReason string) error {
	failureType = firstText(failureType, "unknown")
	failureReason = safeFailureReason(failureReason)
	if err := env.Model("mail.mail").Browse(id).Write(map[string]any{
		"state":          "exception",
		"failure_type":   failureType,
		"failure_reason": failureReason,
	}); err != nil {
		return err
	}
	if err := markMailingTraceFailed(env, id, failureType); err != nil {
		return err
	}
	return updateLinkedNotifications(env, id, "exception", failureType, failureReason)
}

func markMailingTraceSent(env *record.Env, mailID int64, now time.Time) error {
	rows, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(rows) == 0 {
		return err
	}
	values := map[string]any{
		"trace_status":  "sent",
		"sent_datetime": queueNow(now),
		"failure_type":  "",
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["trace_status"])) == "cancel" {
			continue
		}
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func markMailingTraceMessageID(env *record.Env, mailID int64, messageID string) error {
	messageID = normalizeMessageID(messageID)
	rows, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(rows) == 0 || messageID == "" {
		return err
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["trace_status"])) == "cancel" {
			continue
		}
		if strings.TrimSpace(stringAny(row["message_id"])) != "" {
			continue
		}
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(map[string]any{"message_id": messageID}); err != nil {
			return err
		}
	}
	return nil
}

func markMailingTraceFailed(env *record.Env, mailID int64, failureType string) error {
	rows, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(rows) == 0 {
		return err
	}
	values := map[string]any{
		"trace_status": "error",
		"failure_type": firstText(failureType, "unknown"),
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["trace_status"])) == "cancel" {
			continue
		}
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func markMailingTraceCanceledEmails(env *record.Env, mailID int64, emails map[string]bool, failureType string) error {
	if len(emails) == 0 {
		return nil
	}
	rows, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(rows) == 0 {
		return err
	}
	values := map[string]any{
		"trace_status": "cancel",
		"failure_type": firstText(failureType, "mail_optout"),
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if !emails[normalizedEmailAddress(stringAny(row["email"]))] {
			continue
		}
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func markMailingTraceCanceled(env *record.Env, mailID int64, failureType string) error {
	rows, err := mailingTraceRowsForMailID(env, mailID)
	if err != nil || len(rows) == 0 {
		return err
	}
	values := map[string]any{
		"trace_status": "cancel",
		"failure_type": firstText(failureType, "mail_bl"),
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func mailingTraceRowsForMailID(env *record.Env, mailID int64) ([]map[string]any, error) {
	if env == nil || mailID == 0 {
		return nil, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.trace"); !ok {
		return nil, nil
	}
	found, err := systemEnv.Model("mailing.trace").Search(domain.Or(
		domain.Cond("mail_mail_id", "=", mailID),
		domain.Cond("mail_mail_id_int", "=", mailID),
	))
	if err != nil {
		return nil, err
	}
	return found.Read("id", "mail_mail_id", "mail_mail_id_int", "email", "model", "res_id", "mass_mailing_id", "message_id", "trace_status", "failure_type", "failure_reason", "sent_datetime")
}

func createEmailNotifications(env *record.Env, messageID int64, mailID int64, partnerIDs []int64) error {
	if len(partnerIDs) == 0 {
		return nil
	}
	authorID := int64(0)
	if rows, err := env.Model("mail.message").Browse(messageID).Read("author_id"); err == nil && len(rows) > 0 {
		authorID = int64FromAny(rows[0]["author_id"])
	}
	for _, partnerID := range uniqueIDs(partnerIDs) {
		if partnerID == 0 {
			continue
		}
		if _, err := env.Model("mail.notification").Create(map[string]any{
			"mail_message_id":     messageID,
			"mail_mail_id":        mailID,
			"res_partner_id":      partnerID,
			"mail_email_address":  partnerEmail(env, partnerID),
			"notification_type":   "email",
			"notification_status": "ready",
			"is_read":             true,
			"author_id":           authorID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func updateLinkedNotifications(env *record.Env, mailID int64, status string, failureType string, failureReason string) error {
	return updateMailNotification(env, mailID, 0, status, failureType, failureReason)
}

func postprocessLinkedNotifications(env *record.Env, mailID int64, successPartnerIDs []int64, successEmails []string, failureType string, failureReason string) error {
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_mail_id", "=", mailID))
	if err != nil {
		return err
	}
	rows, err := found.Read("res_partner_id", "mail_email_address", "notification_type", "notification_status")
	if err != nil {
		return err
	}
	partnerSuccess := map[int64]bool{}
	for _, partnerID := range successPartnerIDs {
		if partnerID != 0 {
			partnerSuccess[partnerID] = true
		}
	}
	emailSuccess := map[string]bool{}
	for _, email := range successEmails {
		if normalized := normalizedEmailAddress(email); normalized != "" {
			emailSuccess[normalized] = true
		}
	}
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id == 0 || !mailNotificationPostprocessable(row) {
			continue
		}
		values := notificationValues("exception", firstText(failureType, "unknown"), safeFailureReason(failureReason))
		partnerID := int64FromAny(row["res_partner_id"])
		email := normalizedEmailAddress(stringAny(row["mail_email_address"]))
		if partnerSuccess[partnerID] || emailSuccess[email] {
			values = notificationValues("sent", "", "")
		}
		if err := env.Model("mail.notification").Browse(id).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func updateMailNotification(env *record.Env, mailID int64, partnerID int64, status string, failureType string, failureReason string) error {
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_mail_id", "=", mailID))
	if err != nil {
		return err
	}
	rows, err := found.Read("res_partner_id", "mail_email_address", "notification_type", "notification_status")
	if err != nil {
		return err
	}
	values := notificationValues(status, failureType, failureReason)
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id == 0 || !mailNotificationPostprocessable(row) {
			continue
		}
		if partnerID != 0 && int64FromAny(row["res_partner_id"]) != partnerID {
			continue
		}
		if err := env.Model("mail.notification").Browse(id).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func mailNotificationPostprocessable(row map[string]any) bool {
	notificationType := strings.TrimSpace(stringAny(row["notification_type"]))
	if notificationType != "" && notificationType != "email" {
		return false
	}
	switch strings.TrimSpace(stringAny(row["notification_status"])) {
	case "sent", "canceled", "cancel":
		return false
	default:
		return true
	}
}

func notificationValues(status string, failureType string, failureReason string) map[string]any {
	values := map[string]any{"notification_status": status}
	if failureType != "" || status == "sent" {
		values["failure_type"] = failureType
	}
	if failureReason != "" || status == "sent" {
		values["failure_reason"] = failureReason
	}
	return values
}

func partnerEmail(env *record.Env, partnerID int64) string {
	rows, err := env.Model("res.partner").Browse(partnerID).Read("email")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringAny(rows[0]["email"])
}

func partnerEmailList(env *record.Env, partnerID int64) string {
	addresses := normalizeEmails([]string{partnerEmail(env, partnerID)})
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address.Address) != "" {
			out = append(out, mailAddressString(address))
		}
	}
	return strings.Join(out, ",")
}

func safeFailureReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "send failed"
	}
	return "send failed"
}

func mailFailureType(err error) string {
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(err)))
	switch {
	case strings.Contains(text, "recipients required"):
		return "mail_email_missing"
	case strings.Contains(text, "recipients invalid"):
		return "mail_email_invalid"
	case strings.Contains(text, "smtp"), strings.Contains(text, "mail server unavailable"), strings.Contains(text, "mail server unauthorized"):
		return "mail_smtp"
	default:
		return "unknown"
	}
}

func queueNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
