package mail

import (
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

var (
	massMailingHTMLTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)
	massMailingHTMLComment    = regexp.MustCompile(`(?is)<!--.*?-->`)
	massMailingWhitespace     = regexp.MustCompile(`\s+`)
)

type TemplateSendRequest struct {
	TemplateID      int64
	Model           string
	ResIDs          []int64
	EmailValues     map[string]any
	Now             time.Time
	UserID          int64
	CCExpander      CCExpander
	CopyAttachments bool
}

type CCExpansionRequest struct {
	TemplateID       int64
	TemplateGroupIDs []int64
	Model            string
	RecordID         int64
	UserID           int64
	InitialCC        []string
	RecipientEmails  []string
	PartnerIDs       []int64
	At               time.Time
}

type CCExpander func(CCExpansionRequest) ([]string, error)

type TemplateReportAttachment struct {
	ReportID       int64
	Name           string
	Mimetype       string
	Data           []byte
	CacheName      string
	UseCache       bool
	SourceModel    string
	SourceID       int64
	SourceReportID int64
}

type ThreadMessagesRequest struct {
	Model          string
	ResID          int64
	Limit          int
	Offset         int
	Before         int64
	After          int64
	Around         int64
	SearchTerm     string
	IsNotification *bool
	AccessToken    string
	AccessHash     string
	AccessPID      int64
	PortalOnly     bool
}

func SendTemplateBatch(env *record.Env, req TemplateSendRequest) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail template requires env")
	}
	if req.TemplateID == 0 {
		return nil, fmt.Errorf("mail template requires template id")
	}
	if len(req.ResIDs) == 0 {
		return nil, fmt.Errorf("mail template requires record ids")
	}
	template, templateModel, err := loadTemplate(env, req.TemplateID)
	if err != nil {
		return nil, err
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" {
		modelName = templateModel
	}
	if modelName == "" {
		return nil, fmt.Errorf("mail template requires model")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	mailIDs := make([]int64, 0, len(req.ResIDs))
	duplicateSeen := map[string]bool{}
	for _, resID := range uniqueIDs(req.ResIDs) {
		if resID == 0 {
			continue
		}
		mailID, err := sendTemplateForRecord(env, template, modelName, resID, req.EmailValues, now, req.UserID, req.CCExpander, req.CopyAttachments, duplicateSeen)
		if err != nil {
			return nil, err
		}
		if mailID == 0 {
			continue
		}
		mailIDs = append(mailIDs, mailID)
	}
	return mailIDs, nil
}

func RenderTemplateForRecord(env *record.Env, templateID int64, modelName string, resID int64, emailValues map[string]any) (RenderedMessage, error) {
	template, templateModel, err := loadTemplate(env, templateID)
	if err != nil {
		return RenderedMessage{}, err
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = templateModel
	}
	return renderLoadedTemplateForRecord(env, template, modelName, resID, emailValues)
}

func renderLoadedTemplateForRecord(env *record.Env, template Template, modelName string, resID int64, emailValues map[string]any) (RenderedMessage, error) {
	partnerTo := firstText(emailValues["partner_to"], template.PartnerTo)
	values, err := renderValues(env, modelName, resID, template.Subject, template.Body, template.To, template.CC, partnerTo)
	if err != nil {
		return RenderedMessage{}, err
	}
	raw := Template{
		ID:      template.ID,
		Name:    template.Name,
		To:      firstText(emailValues["email_to"], template.To),
		CC:      firstText(emailValues["email_cc"], template.CC),
		Subject: firstText(emailValues["subject"], template.Subject),
		Body:    firstText(firstNonNil(emailValues["body_html"], emailValues["body"]), template.Body),
	}
	return raw.Render(values), nil
}

func ComposeDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail compose requires env")
	}
	values, err := env.Model("mail.compose.message").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	modelName := firstText(context["default_model"], context["active_model"])
	resIDs := int64s(firstNonNil(context["default_res_ids"], context["active_ids"]))
	if len(resIDs) == 0 {
		if id := int64FromAny(firstNonNil(context["default_res_id"], context["active_id"])); id != 0 {
			resIDs = []int64{id}
		}
	}
	templateID := int64FromAny(firstNonNil(context["default_template_id"], context["template_id"]))
	putDefault(values, "composition_mode", firstText(context["default_composition_mode"], "comment"))
	putDefault(values, "model", modelName)
	if len(resIDs) > 0 {
		putDefault(values, "res_id", resIDs[0])
		putDefault(values, "res_ids", resIDs)
	}
	if templateID != 0 {
		template, templateModel, err := loadTemplate(env, templateID)
		if err != nil {
			return nil, err
		}
		putDefault(values, "template_id", templateID)
		if modelName == "" {
			modelName = templateModel
			putDefault(values, "model", modelName)
		}
		if len(template.AttachmentIDs) > 0 {
			putDefault(values, "attachment_ids", append([]int64(nil), template.AttachmentIDs...))
		}
		if modelName != "" && len(resIDs) > 0 {
			rendered, err := renderLoadedTemplateForRecord(env, template, modelName, resIDs[0], values)
			if err != nil {
				return nil, err
			}
			putDefault(values, "subject", rendered.Subject)
			putDefault(values, "body", rendered.Body)
			putDefault(values, "body_html", rendered.Body)
			putDefault(values, "email_to", rendered.To)
			putDefault(values, "email_cc", rendered.CC)
		}
	}
	putDefault(values, "body_is_html", true)
	putDefault(values, "notify", true)
	if len(fields) == 0 {
		return values, nil
	}
	filtered := map[string]any{}
	for _, field := range fields {
		if value, ok := values[field]; ok {
			filtered[field] = value
		}
	}
	return filtered, nil
}

func SendComposeMessages(env *record.Env, ids []int64, now time.Time) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail compose requires env")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("mail compose requires ids")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := env.Model("mail.compose.message").Browse(ids...).Read(
		"composition_mode", "model", "res_id", "res_ids", "template_id", "subject", "body", "body_html",
		"email_from", "email_to", "email_cc", "reply_to", "partner_ids", "attachment_ids", "parent_id",
		"subtype_id", "scheduled_date", "author_id", "auto_delete", "mass_mailing_id", "use_exclusion_list", "notify", "body_is_html",
	)
	if err != nil {
		return nil, err
	}
	mailIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		modelName := strings.TrimSpace(stringAny(row["model"]))
		resIDs := int64s(row["res_ids"])
		if len(resIDs) == 0 {
			if id := int64FromAny(row["res_id"]); id != 0 {
				resIDs = []int64{id}
			}
		}
		if modelName == "" || len(resIDs) == 0 {
			return nil, fmt.Errorf("mail compose requires model and record ids")
		}
		emailValues := composeEmailValues(row)
		if templateID := int64FromAny(row["template_id"]); templateID != 0 {
			ids, err := SendTemplateBatch(env, TemplateSendRequest{
				TemplateID:      templateID,
				Model:           modelName,
				ResIDs:          resIDs,
				EmailValues:     emailValues,
				Now:             now,
				CopyAttachments: true,
			})
			if err != nil {
				return nil, err
			}
			mailIDs = append(mailIDs, ids...)
			continue
		}
		ids, err := sendDirectCompose(env, row, modelName, resIDs, now)
		if err != nil {
			return nil, err
		}
		mailIDs = append(mailIDs, ids...)
	}
	return mailIDs, nil
}

func ScheduleComposeMessages(env *record.Env, ids []int64, now time.Time) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail compose requires env")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("mail compose requires ids")
	}
	rows, err := env.Model("mail.compose.message").Browse(ids...).Read(
		"composition_mode", "composition_comment_option", "message_type", "subtype_is_log", "model", "res_id", "res_ids",
		"template_id", "subject", "body", "body_html", "email_from", "email_to", "email_cc", "reply_to", "partner_ids",
		"attachment_ids", "parent_id", "subtype_id", "scheduled_date", "author_id", "auto_delete", "mass_mailing_id", "use_exclusion_list", "notify", "body_is_html",
	)
	if err != nil {
		return nil, err
	}
	scheduledIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		modelName := strings.TrimSpace(stringAny(row["model"]))
		resIDs := int64s(row["res_ids"])
		if len(resIDs) == 0 {
			if id := int64FromAny(row["res_id"]); id != 0 {
				resIDs = []int64{id}
			}
		}
		if modelName == "" || len(resIDs) == 0 {
			return nil, fmt.Errorf("mail compose requires model and record ids")
		}
		scheduledDate := timeValue(row["scheduled_date"])
		if scheduledDate.IsZero() {
			return nil, fmt.Errorf("mail compose schedule requires scheduled_date")
		}
		emailValues := composeEmailValues(row)
		mailingID := int64FromAny(firstNonNil(emailValues["mailing_id"], emailValues["mass_mailing_id"]))
		templateID := int64FromAny(row["template_id"])
		duplicateSeen := map[string]bool{}
		for _, resID := range uniqueIDs(resIDs) {
			var rendered RenderedMessage
			var err error
			if templateID != 0 {
				rendered, err = RenderTemplateForRecord(env, templateID, modelName, resID, emailValues)
				if err != nil {
					return nil, err
				}
			} else {
				rendered = RenderedMessage{
					To:      stringAny(row["email_to"]),
					CC:      stringAny(row["email_cc"]),
					Subject: stringAny(row["subject"]),
					Body:    firstText(row["body_html"], row["body"]),
				}
			}
			partnerIDs := int64s(row["partner_ids"])
			if strings.TrimSpace(rendered.To) == "" && len(partnerIDs) == 0 && !composeAllowsNoRecipient(row) && mailingID == 0 {
				return nil, fmt.Errorf("mail compose requires recipient")
			}
			if canceled, err := cancelMassMailingBlacklistTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
				return nil, err
			} else if canceled {
				continue
			}
			if canceled, err := cancelMassMailingRecipientFailureTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
				return nil, err
			} else if canceled {
				continue
			}
			if canceled, err := cancelMassMailingOptOutTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
				return nil, err
			} else if canceled {
				continue
			}
			if canceled, err := cancelMassMailingDuplicateTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues, duplicateSeen); err != nil {
				return nil, err
			} else if canceled {
				continue
			}
			rememberMassMailingDuplicateEmails(mailingID, massMailingRenderedEmails(env, rendered, partnerIDs), duplicateSeen)
			attachmentIDs := uniqueIDs(int64s(emailValues["attachment_ids"]))
			recordEmailValues := copyEmailValues(emailValues)
			if len(attachmentIDs) > 0 {
				if err := validateAttachmentOwnership(env, attachmentIDs, nil, false); err != nil {
					return nil, err
				}
				recordEmailValues["attachment_ids"] = []int64{}
			}
			scheduledID, err := createScheduledMessage(env, templateID, 0, 0, modelName, resID, rendered, partnerIDs, scheduledDate, recordEmailValues)
			if err != nil {
				return nil, err
			}
			finalAttachmentIDs := []int64{}
			if len(attachmentIDs) > 0 {
				copiedAttachmentIDs, err := copyAttachmentsToRecord(env, attachmentIDs, "mail.scheduled.message", scheduledID)
				if err != nil {
					return nil, err
				}
				finalAttachmentIDs = append(finalAttachmentIDs, copiedAttachmentIDs...)
			}
			if templateID != 0 {
				reportAttachmentIDs, err := CreateTemplateReportAttachments(env, templateID, modelName, resID, "mail.scheduled.message", scheduledID)
				if err != nil {
					return nil, err
				}
				finalAttachmentIDs = append(finalAttachmentIDs, reportAttachmentIDs...)
				postprocessAttachmentIDs, err := CreateTemplatePostprocessAttachments(env, templateID, modelName, resID, "mail.scheduled.message", scheduledID)
				if err != nil {
					return nil, err
				}
				finalAttachmentIDs = append(finalAttachmentIDs, postprocessAttachmentIDs...)
			}
			finalAttachmentIDs = uniqueIDs(finalAttachmentIDs)
			if len(finalAttachmentIDs) > 0 {
				if err := messageSystemEnv(env).Model("mail.scheduled.message").Browse(scheduledID).Write(map[string]any{"attachment_ids": finalAttachmentIDs}); err != nil {
					return nil, err
				}
			}
			scheduledIDs = append(scheduledIDs, scheduledID)
		}
	}
	return scheduledIDs, nil
}

func FetchThreadMessages(env *record.Env, req ThreadMessagesRequest) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail thread requires env")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" || req.ResID == 0 {
		return nil, fmt.Errorf("mail thread messages require model and record id")
	}
	if req.AccessToken != "" || req.AccessHash != "" || req.AccessPID != 0 {
		env = PortalContextEnv(env, req.Model, req.ResID, req.AccessToken, req.AccessHash, req.AccessPID)
	}
	if err := ensureRecordExists(env, req.Model, req.ResID); err != nil {
		return nil, err
	}
	messageEnv := env
	if req.PortalOnly && env.Context().UserID == 0 && portalContextMatches(env, req.Model, req.ResID) {
		messageEnv = messageSystemEnv(env)
	}
	filters := []domain.Node{
		domain.Cond("model", "=", req.Model),
		domain.Cond("res_id", "=", req.ResID),
	}
	found, err := messageEnv.Model("mail.message").Search(domain.And(filters...))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("subject", "body", "message_type", "model", "res_id", "author_id", "author_guest_id", "email_from", "date", "parent_id", "subtype_id", "partner_ids", "attachment_ids", "body_is_html", "is_internal", "reaction_ids", "starred", "starred_partner_ids", "tracking_value_ids")
	if err != nil {
		return nil, err
	}
	rows = ComputeMessageStarred(env, rows)
	rows = filterThreadMessageRows(messageSystemEnv(env), rows, req)
	count := len(rows)
	rows = windowThreadMessageRows(rows, req)
	trackingRows, err := attachTrackingValues(messageEnv, rows)
	if err != nil {
		return nil, err
	}
	if req.PortalOnly {
		rows = portalMessageRows(env, rows, req)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, int64FromAny(row["id"]))
	}
	if !req.PortalOnly {
		markMessageNotificationsRead(messageSystemEnv(env), ids, currentUserPartnerID(env, env.Context().UserID))
	}
	data := map[string]any{}
	if req.PortalOnly {
		data["mail.message"] = rows
	} else {
		partnerID := starredPartnerID(env)
		partnerIDs, guestIDs, reactionRows := messageReactionStoreGroups(messageSystemEnv(env), ids, "")
		authorPartnerIDs, authorGuestIDs := messageActorIDs(rows)
		partnerIDs = uniqueIDs(append(partnerIDs, authorPartnerIDs...))
		guestIDs = uniqueIDs(append(guestIDs, authorGuestIDs...))
		data["mail.message"] = mailboxMessageRows(rows, partnerID, messageReactionGroupsByMessage(reactionRows))
		data["MessageReactions"] = reactionRows
		data["mail.thread"] = messageThreadStoreRows(messageSystemEnv(env), req.Model, req.ResID)
		if partnerRows := actorRows(messageSystemEnv(env), "res.partner", partnerIDs); len(partnerRows) > 0 {
			data["res.partner"] = partnerRows
		}
		if guestRows := actorRows(messageSystemEnv(env), "mail.guest", guestIDs); len(guestRows) > 0 {
			data["mail.guest"] = guestRows
		}
		if attachmentRows := messageAttachmentStoreRows(messageSystemEnv(env), rows); len(attachmentRows) > 0 {
			data["ir.attachment"] = attachmentRows
		}
		if subtypeRows := messageSubtypeStoreRows(messageSystemEnv(env), rows); len(subtypeRows) > 0 {
			data["mail.message.subtype"] = subtypeRows
		}
		if notificationRows := messageNotificationStoreRows(messageSystemEnv(env), ids); len(notificationRows) > 0 {
			data["mail.notification"] = notificationRows
		}
	}
	if trackingRows == nil {
		trackingRows = []map[string]any{}
	}
	data["mail.tracking.value"] = trackingRows
	result := map[string]any{
		"messages": ids,
		"data":     data,
	}
	if strings.TrimSpace(req.SearchTerm) != "" {
		result["count"] = count
	}
	return result, nil
}

func markMessageNotificationsRead(env *record.Env, messageIDs []int64, partnerID int64) {
	messageIDs = uniqueIDs(messageIDs)
	if env == nil || partnerID == 0 || len(messageIDs) == 0 {
		return
	}
	found, err := env.Model("mail.notification").Search(domain.And(
		domain.Cond("mail_message_id", "in", messageIDs),
		domain.Cond("res_partner_id", "=", partnerID),
		domain.Cond("is_read", "=", false),
	))
	if err != nil || found.Len() == 0 {
		return
	}
	_ = found.Write(map[string]any{"is_read": true, "read_date": time.Now().UTC()})
}

func messageReactionGroupsByMessage(groups []map[string]any) map[int64][]map[string]any {
	out := map[int64][]map[string]any{}
	for _, group := range groups {
		messageID := int64FromAny(group["message"])
		if messageID == 0 {
			continue
		}
		out[messageID] = append(out[messageID], group)
	}
	return out
}

func messageActorIDs(rows []map[string]any) ([]int64, []int64) {
	partnerIDs := make([]int64, 0, len(rows))
	guestIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["author_id"]); id != 0 {
			partnerIDs = append(partnerIDs, id)
		}
		if id := int64FromAny(row["author_guest_id"]); id != 0 {
			guestIDs = append(guestIDs, id)
		}
	}
	return uniqueIDs(partnerIDs), uniqueIDs(guestIDs)
}

func messageThreadStoreRows(env *record.Env, modelName string, resID int64) []map[string]any {
	displayName := fmt.Sprintf("%s,%d", modelName, resID)
	if env != nil {
		if pairs, err := env.Model(modelName).Browse(resID).NameGet(); err == nil && len(pairs) > 0 {
			displayName = stringAny(pairs[0][1])
		}
	}
	return []map[string]any{{
		"display_name":    displayName,
		"has_mail_thread": true,
		"id":              resID,
		"model":           modelName,
	}}
}

func messageAttachmentStoreRows(env *record.Env, rows []map[string]any) []map[string]any {
	ids := make([]int64, 0)
	for _, row := range rows {
		ids = append(ids, int64s(row["attachment_ids"])...)
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(ids...).Read("name", "res_model", "res_id", "mimetype", "checksum", "has_thumbnail")
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(attachmentRows))
	for _, row := range attachmentRows {
		out = append(out, map[string]any{
			"checksum":      stringAny(row["checksum"]),
			"filename":      stringAny(row["name"]),
			"has_thumbnail": boolAny(row["has_thumbnail"]),
			"id":            int64FromAny(row["id"]),
			"mimetype":      stringAny(row["mimetype"]),
			"name":          stringAny(row["name"]),
			"res_id":        int64FromAny(row["res_id"]),
			"res_model":     stringAny(row["res_model"]),
		})
	}
	return out
}

func messageSubtypeStoreRows(env *record.Env, rows []map[string]any) []map[string]any {
	ids := make([]int64, 0)
	for _, row := range rows {
		if id := int64FromAny(row["subtype_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	subtypeRows, err := env.Model("mail.message.subtype").Browse(ids...).Read("name", "description", "internal")
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(subtypeRows))
	for _, row := range subtypeRows {
		out = append(out, map[string]any{
			"description": stringAny(row["description"]),
			"id":          int64FromAny(row["id"]),
			"internal":    boolAny(row["internal"]),
			"name":        stringAny(row["name"]),
		})
	}
	return out
}

func messageNotificationStoreRows(env *record.Env, messageIDs []int64) []map[string]any {
	messageIDs = uniqueIDs(messageIDs)
	if len(messageIDs) == 0 {
		return nil
	}
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_message_id", "in", messageIDs))
	if err != nil {
		return nil
	}
	rows, err := found.Read("mail_message_id", "mail_mail_id", "res_partner_id", "mail_email_address", "notification_type", "notification_status", "failure_type", "failure_reason", "is_read", "read_date", "author_id")
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"author_id":           int64FromAny(row["author_id"]),
			"failure_reason":      stringAny(row["failure_reason"]),
			"failure_type":        stringAny(row["failure_type"]),
			"id":                  int64FromAny(row["id"]),
			"is_read":             boolAny(row["is_read"]),
			"mail_email_address":  stringAny(row["mail_email_address"]),
			"mail_mail_id":        int64FromAny(row["mail_mail_id"]),
			"mail_message_id":     int64FromAny(row["mail_message_id"]),
			"notification_status": stringAny(row["notification_status"]),
			"notification_type":   stringAny(row["notification_type"]),
			"read_date":           row["read_date"],
			"res_partner_id":      int64FromAny(row["res_partner_id"]),
		})
	}
	return out
}

func filterThreadMessageRows(env *record.Env, rows []map[string]any, req ThreadMessagesRequest) []map[string]any {
	sort.Slice(rows, func(i, j int) bool {
		return int64FromAny(rows[i]["id"]) > int64FromAny(rows[j]["id"])
	})
	commentSubtypeID := int64(0)
	if req.PortalOnly {
		commentSubtypeID = resolveSubtypeXMLID(env, "mail.mt_comment")
	}
	searchTerm := strings.ToLower(strings.TrimSpace(req.SearchTerm))
	includeTrackingSearch := req.IsNotification == nil || *req.IsNotification
	searchIndex := threadMessageSearchIndex(env, rows, searchTerm, includeTrackingSearch)
	out := rows[:0]
	for _, row := range rows {
		messageType := stringAny(row["message_type"])
		if messageType == "user_notification" {
			continue
		}
		if req.IsNotification != nil {
			isNotification := messageType == "notification"
			if isNotification != *req.IsNotification {
				continue
			}
		}
		if req.PortalOnly {
			if commentSubtypeID != 0 && int64FromAny(row["subtype_id"]) != commentSubtypeID {
				continue
			}
			body := strings.TrimSpace(stringAny(row["body"]))
			if (body == "" || body == `<span class="o-mail-Message-edited"></span>`) && len(int64s(row["attachment_ids"])) == 0 {
				continue
			}
		}
		if searchTerm != "" && !threadMessageSearchMatch(searchIndex[int64FromAny(row["id"])], searchTerm) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func threadMessageSearchIndex(env *record.Env, rows []map[string]any, searchTerm string, includeTracking bool) map[int64]string {
	out := map[int64]string{}
	if searchTerm == "" {
		return out
	}
	attachmentNames := messageAttachmentNames(env, rows)
	subtypeText := messageSubtypeText(env, rows)
	trackingText := map[int64]string{}
	if includeTracking {
		trackingText = messageTrackingText(env, rows)
	}
	for _, row := range rows {
		messageID := int64FromAny(row["id"])
		parts := []string{
			stringAny(row["body"]),
			stringAny(row["subject"]),
			subtypeText[int64FromAny(row["subtype_id"])],
			trackingText[messageID],
		}
		for _, attachmentID := range int64s(row["attachment_ids"]) {
			parts = append(parts, attachmentNames[attachmentID])
		}
		out[messageID] = strings.ToLower(strings.Join(parts, " "))
	}
	return out
}

func messageAttachmentNames(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0)
	for _, row := range rows {
		ids = append(ids, int64s(row["attachment_ids"])...)
	}
	ids = uniqueIDs(ids)
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	attachmentRows, err := env.Model("ir.attachment").Browse(ids...).Read("name")
	if err != nil {
		return out
	}
	for _, row := range attachmentRows {
		out[int64FromAny(row["id"])] = stringAny(row["name"])
	}
	return out
}

func messageSubtypeText(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["subtype_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	ids = uniqueIDs(ids)
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	subtypeRows, err := env.Model("mail.message.subtype").Browse(ids...).Read("name", "description")
	if err != nil {
		return out
	}
	for _, row := range subtypeRows {
		out[int64FromAny(row["id"])] = strings.Join([]string{stringAny(row["name"]), stringAny(row["description"])}, " ")
	}
	return out
}

func messageTrackingText(env *record.Env, rows []map[string]any) map[int64]string {
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["id"]); id != 0 {
			messageIDs = append(messageIDs, id)
		}
	}
	messageIDs = uniqueIDs(messageIDs)
	out := map[int64]string{}
	if len(messageIDs) == 0 {
		return out
	}
	found, err := env.Model("mail.tracking.value").Search(domain.Cond("mail_message_id", "in", messageIDs))
	if err != nil {
		return out
	}
	trackingRows, err := found.Read(
		"field_name",
		"field_desc",
		"old_value_integer",
		"old_value_float",
		"old_value_char",
		"old_value_text",
		"old_value_datetime",
		"new_value_integer",
		"new_value_float",
		"new_value_char",
		"new_value_text",
		"new_value_datetime",
		"mail_message_id",
	)
	if err != nil {
		return out
	}
	for _, row := range trackingRows {
		messageID := int64FromAny(row["mail_message_id"])
		out[messageID] = strings.TrimSpace(out[messageID] + " " + trackingValueSearchText(row))
	}
	return out
}

func trackingValueSearchText(row map[string]any) string {
	parts := []string{
		stringAny(row["field_name"]),
		stringAny(row["field_desc"]),
		stringAny(row["old_value_char"]),
		stringAny(row["old_value_text"]),
		stringAny(row["old_value_datetime"]),
		stringAny(row["new_value_char"]),
		stringAny(row["new_value_text"]),
		stringAny(row["new_value_datetime"]),
	}
	for _, fieldName := range []string{"old_value_integer", "old_value_float", "new_value_integer", "new_value_float"} {
		if !emptyAny(row[fieldName]) {
			parts = append(parts, stringAny(row[fieldName]))
		}
	}
	return strings.Join(parts, " ")
}

func threadMessageSearchMatch(haystack string, searchTerm string) bool {
	if strings.Contains(haystack, searchTerm) {
		return true
	}
	parts := strings.Split(strings.ReplaceAll(searchTerm, " ", "%"), "%")
	pos := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		next := strings.Index(haystack[pos:], part)
		if next < 0 {
			return false
		}
		pos += next + len(part)
	}
	return true
}

func windowThreadMessageRows(rows []map[string]any, req ThreadMessagesRequest) []map[string]any {
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}
	if req.Around != 0 {
		return aroundThreadMessageRows(rows, req.Around, limit)
	}
	if req.After != 0 {
		return afterThreadMessageRows(rows, req.After, limit)
	}
	if req.Before != 0 {
		rows = filterMessageRowsBefore(rows, req.Before)
	}
	return paginateThreadMessageRows(rows, limit, req.Offset)
}

func aroundThreadMessageRows(rows []map[string]any, around int64, limit int) []map[string]any {
	half := limit / 2
	before := make([]map[string]any, 0, half)
	after := make([]map[string]any, 0, half)
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id <= around {
			if half == 0 || len(before) < half {
				before = append(before, row)
			}
			continue
		}
		after = append(after, row)
	}
	if half > 0 && len(after) > half {
		after = after[len(after)-half:]
	}
	out := make([]map[string]any, 0, len(after)+len(before))
	out = append(out, after...)
	out = append(out, before...)
	sort.Slice(out, func(i, j int) bool {
		return int64FromAny(out[i]["id"]) > int64FromAny(out[j]["id"])
	})
	return out
}

func afterThreadMessageRows(rows []map[string]any, after int64, limit int) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if int64FromAny(row["id"]) > after {
			out = append(out, row)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func filterMessageRowsBefore(rows []map[string]any, before int64) []map[string]any {
	out := rows[:0]
	for _, row := range rows {
		if int64FromAny(row["id"]) < before {
			out = append(out, row)
		}
	}
	return out
}

func paginateThreadMessageRows(rows []map[string]any, limit int, offset int) []map[string]any {
	if offset > 0 {
		if offset >= len(rows) {
			return nil
		}
		rows = rows[offset:]
	}
	if limit > 0 && limit < len(rows) {
		return rows[:limit]
	}
	return rows
}

func portalMessageRows(env *record.Env, rows []map[string]any, req ThreadMessagesRequest) []map[string]any {
	systemEnv := messageSystemEnv(env)
	noteSubtypeID := resolveSubtypeXMLID(env, "mail.mt_note")
	authorNames := portalAuthorNames(systemEnv, rows)
	guestNames := portalGuestNames(systemEnv, rows)
	reactionGroups := portalReactionGroups(systemEnv, rows)
	starredPartnerID := portalStarredPartnerID(env)
	portalPartnerID := currentPortalPartnerID(env)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		formatted := make(map[string]any, len(row)+8)
		for key, value := range row {
			formatted[key] = value
		}
		messageID := int64FromAny(row["id"])
		authorID := int64FromAny(row["author_id"])
		if authorID != 0 {
			formatted["author_id"] = map[string]any{"id": authorID, "name": authorNames[authorID]}
		} else {
			formatted["author_id"] = false
		}
		guestID := int64FromAny(row["author_guest_id"])
		if guestID != 0 {
			formatted["author_guest_id"] = map[string]any{"id": guestID, "name": guestNames[guestID]}
		} else {
			formatted["author_guest_id"] = false
		}
		includeOwnership := portalPartnerID != 0 && portalPartnerID == int64FromAny(row["author_id"])
		formatted["attachment_ids"] = portalAttachmentRows(systemEnv, int64s(row["attachment_ids"]), includeOwnership)
		formatted["author_avatar_url"] = portalAuthorAvatarURL(messageID, req)
		formatted["body"] = []any{"markup", stringAny(row["body"])}
		formatted["is_internal"] = boolAny(row["is_internal"])
		formatted["is_message_subtype_note"] = noteSubtypeID != 0 && int64FromAny(row["subtype_id"]) == noteSubtypeID
		formatted["published_date_str"] = portalPublishedDateString(row["date"])
		formatted["reactions"] = reactionGroups[messageID]
		if formatted["reactions"] == nil {
			formatted["reactions"] = []map[string]any{}
		}
		formatted["starred"] = starredPartnerID != 0 && containsID(int64s(row["starred_partner_ids"]), starredPartnerID)
		formatted["thread"] = map[string]any{
			"has_mail_thread": true,
			"id":              int64FromAny(row["res_id"]),
			"model":           stringAny(row["model"]),
		}
		out = append(out, formatted)
	}
	return out
}

func portalReactionGroups(env *record.Env, rows []map[string]any) map[int64][]map[string]any {
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["id"]); id != 0 {
			messageIDs = append(messageIDs, id)
		}
	}
	messageIDs = uniqueIDs(messageIDs)
	out := map[int64][]map[string]any{}
	if len(messageIDs) == 0 {
		return out
	}
	found, err := env.Model("mail.message.reaction").Search(domain.Cond("message_id", "in", messageIDs))
	if err != nil {
		return out
	}
	reactionRows, err := found.Read("message_id", "content", "partner_id", "guest_id")
	if err != nil {
		return out
	}
	partnerNames := reactionPartnerNames(env, reactionRows)
	guestNames := reactionGuestNames(env, reactionRows)
	grouped := map[int64]map[string]map[string]any{}
	for _, row := range reactionRows {
		messageID := int64FromAny(row["message_id"])
		content := stringAny(row["content"])
		if messageID == 0 || strings.TrimSpace(content) == "" {
			continue
		}
		byContent := grouped[messageID]
		if byContent == nil {
			byContent = map[string]map[string]any{}
			grouped[messageID] = byContent
		}
		group := byContent[content]
		if group == nil {
			group = map[string]any{
				"content":  content,
				"count":    0,
				"guests":   []map[string]any{},
				"message":  messageID,
				"partners": []map[string]any{},
			}
			byContent[content] = group
		}
		group["count"] = int64FromAny(group["count"]) + 1
		if partnerID := int64FromAny(row["partner_id"]); partnerID != 0 {
			group["partners"] = append(group["partners"].([]map[string]any), map[string]any{"id": partnerID, "name": partnerNames[partnerID]})
		}
		if guestID := int64FromAny(row["guest_id"]); guestID != 0 {
			group["guests"] = append(group["guests"].([]map[string]any), map[string]any{"id": guestID, "name": guestNames[guestID]})
		}
	}
	for messageID, byContent := range grouped {
		contents := make([]string, 0, len(byContent))
		for content := range byContent {
			contents = append(contents, content)
		}
		sort.Strings(contents)
		for _, content := range contents {
			out[messageID] = append(out[messageID], byContent[content])
		}
	}
	return out
}

func reactionPartnerNames(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["partner_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return namesByID(env, "res.partner", ids)
}

func reactionGuestNames(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["guest_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return namesByID(env, "mail.guest", ids)
}

func portalStarredPartnerID(env *record.Env) int64 {
	if partnerID := currentPortalPartnerID(env); partnerID != 0 {
		return partnerID
	}
	return currentUserPartnerID(env, env.Context().UserID)
}

func containsID(ids []int64, id int64) bool {
	for _, item := range ids {
		if item == id {
			return true
		}
	}
	return false
}

func portalGuestNames(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["author_guest_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	ids = uniqueIDs(ids)
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	return namesByID(env, "mail.guest", ids)
}

func portalAuthorNames(env *record.Env, rows []map[string]any) map[int64]string {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64FromAny(row["author_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	ids = uniqueIDs(ids)
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	return namesByID(env, "res.partner", ids)
}

func namesByID(env *record.Env, modelName string, ids []int64) map[int64]string {
	ids = uniqueIDs(ids)
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	partnerRows, err := env.Model(modelName).Browse(ids...).Read("name")
	if err != nil {
		return out
	}
	for _, row := range partnerRows {
		out[int64FromAny(row["id"])] = stringAny(row["name"])
	}
	return out
}

func portalAttachmentRows(env *record.Env, ids []int64, includeOwnership bool) []map[string]any {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return []map[string]any{}
	}
	rows, err := env.Model("ir.attachment").Browse(ids...).Read("name", "res_model", "res_id", "mimetype", "access_token", "checksum", "has_thumbnail")
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := stringAny(row["name"])
		id := int64FromAny(row["id"])
		rawToken := AttachmentRawAccessToken(env, id)
		if rawToken == "" {
			rawToken = stringAny(row["access_token"])
		}
		thumbnailToken := AttachmentThumbnailAccessToken(env, id)
		if thumbnailToken == "" {
			thumbnailToken = rawToken
		}
		item := map[string]any{
			"filename":               name,
			"checksum":               stringAny(row["checksum"]),
			"has_thumbnail":          boolAny(row["has_thumbnail"]),
			"id":                     id,
			"mimetype":               stringAny(row["mimetype"]),
			"name":                   name,
			"raw_access_token":       rawToken,
			"res_id":                 int64FromAny(row["res_id"]),
			"res_model":              stringAny(row["res_model"]),
			"thumbnail_access_token": thumbnailToken,
		}
		if includeOwnership {
			item["ownership_token"] = AttachmentOwnershipToken(env, id)
		}
		out = append(out, item)
	}
	return out
}

func portalAuthorAvatarURL(messageID int64, req ThreadMessagesRequest) string {
	if req.AccessToken != "" {
		return fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50?access_token=%s", messageID, url.QueryEscape(req.AccessToken))
	}
	if req.AccessHash != "" && req.AccessPID != 0 {
		values := url.Values{}
		values.Set("_hash", req.AccessHash)
		values.Set("pid", strconv.FormatInt(req.AccessPID, 10))
		return fmt.Sprintf("/mail/avatar/mail.message/%d/author_avatar/50x50?%s", messageID, values.Encode())
	}
	return fmt.Sprintf("/web/image/mail.message/%d/author_avatar/50x50", messageID)
}

func portalPublishedDateString(value any) string {
	date := timeValue(value)
	if date.IsZero() {
		return ""
	}
	return date.Format("2006-01-02 15:04:05")
}

func PartnersFromEmails(env *record.Env, emails []string, createMissing bool) ([]map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail partner lookup requires env")
	}
	out := make([]map[string]any, 0, len(emails))
	for _, item := range normalizeEmails(emails) {
		row, found, err := partnerFromEmailAddress(env, item, createMissing)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, map[string]any{"id": row["id"], "name": row["name"], "email": row["email"]})
		}
	}
	return out, nil
}

func sendTemplateForRecord(env *record.Env, template Template, modelName string, resID int64, emailValues map[string]any, now time.Time, userID int64, ccExpander CCExpander, copyAttachments bool, duplicateSeen map[string]bool) (int64, error) {
	_, explicitAttachmentIDs := emailValues["attachment_ids"]
	emailValues = templateEmailValues(template, emailValues)
	rawPartnerTo := firstText(emailValues["partner_to"], template.PartnerTo)
	values, err := renderValues(env, modelName, resID, template.Subject, template.Body, template.To, template.CC, rawPartnerTo)
	if err != nil {
		return 0, err
	}
	raw := Template{
		ID:      template.ID,
		Name:    template.Name,
		To:      firstText(emailValues["email_to"], template.To),
		CC:      firstText(emailValues["email_cc"], template.CC),
		Subject: firstText(emailValues["subject"], template.Subject),
		Body:    firstText(firstNonNil(emailValues["body_html"], emailValues["body"]), template.Body),
	}
	rendered := raw.Render(values)
	partnerIDs := uniqueIDs(append(int64s(firstNonNil(emailValues["partner_ids"], emailValues["recipient_ids"])), parseIDList(renderText(rawPartnerTo, values))...))
	mailingID := int64FromAny(firstNonNil(emailValues["mailing_id"], emailValues["mass_mailing_id"]))
	if strings.TrimSpace(rendered.To) == "" && len(partnerIDs) == 0 {
		rendered.To = stringAny(values["email"])
	}
	if strings.TrimSpace(rendered.To) == "" && len(partnerIDs) == 0 && mailingID == 0 {
		return 0, fmt.Errorf("mail template requires recipient")
	}
	if ccExpander != nil {
		cc, err := ccExpander(CCExpansionRequest{
			TemplateID:       template.ID,
			TemplateGroupIDs: append([]int64(nil), template.DelegationGroupIDs...),
			Model:            modelName,
			RecordID:         resID,
			UserID:           userID,
			InitialCC:        splitRecipientList(rendered.CC),
			RecipientEmails:  splitRecipientList(rendered.To + ";" + rendered.CC),
			PartnerIDs:       append([]int64(nil), partnerIDs...),
			At:               now,
		})
		if err != nil {
			return 0, err
		}
		rendered.CC = joinRecipientList(cc)
	}
	if canceled, err := cancelMassMailingBlacklistTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
		return 0, err
	} else if canceled {
		return 0, nil
	}
	if canceled, err := cancelMassMailingRecipientFailureTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
		return 0, err
	} else if canceled {
		return 0, nil
	}
	if canceled, err := cancelMassMailingOptOutTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
		return 0, err
	} else if canceled {
		return 0, nil
	}
	if canceled, err := cancelMassMailingDuplicateTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues, duplicateSeen); err != nil {
		return 0, err
	} else if canceled {
		return 0, nil
	}
	scheduledDate := timeValue(firstNonNil(emailValues["scheduled_date"], values["scheduled_date"]))
	attachmentIDs := uniqueIDs(int64s(emailValues["attachment_ids"]))
	postprocessAttachmentIDs, err := TemplatePostprocessAttachmentIDs(env, template.ID, modelName, resID)
	if err != nil {
		return 0, err
	}
	attachmentIDs = uniqueIDs(append(attachmentIDs, postprocessAttachmentIDs...))
	postAttachmentIDs := attachmentIDs
	preserveAttachments := !explicitAttachmentIDs && len(template.AttachmentIDs) > 0 && len(attachmentIDs) > 0
	if copyAttachments && len(attachmentIDs) > 0 {
		if err := validateAttachmentOwnership(env, attachmentIDs, nil, false); err != nil {
			return 0, err
		}
		postAttachmentIDs = nil
		preserveAttachments = false
	}
	messageID, err := PostMessage(env, PostRequest{
		Model:               modelName,
		ResID:               resID,
		Body:                rendered.Body,
		Subject:             rendered.Subject,
		MessageType:         "email",
		EmailFrom:           stringAny(emailValues["email_from"]),
		AuthorID:            int64FromAny(emailValues["author_id"]),
		ParentID:            int64FromAny(emailValues["parent_id"]),
		SubtypeID:           int64FromAny(emailValues["subtype_id"]),
		PartnerIDs:          partnerIDs,
		AttachmentIDs:       postAttachmentIDs,
		PreserveAttachments: preserveAttachments,
		BodyIsHTML:          true,
		AutoFollow:          boolAny(emailValues["mail_post_autofollow"]),
		Now:                 now,
	})
	if err != nil {
		return 0, err
	}
	finalAttachmentIDs := attachmentIDs
	if copyAttachments && len(attachmentIDs) > 0 {
		copiedAttachmentIDs, err := copyAttachmentsToMessage(env, attachmentIDs, messageID)
		if err != nil {
			return 0, err
		}
		finalAttachmentIDs = copiedAttachmentIDs
	}
	if len(template.ReportTemplateIDs) > 0 {
		reportAttachmentIDs, err := createTemplateReportAttachments(env, template, modelName, resID, "mail.message", messageID, values)
		if err != nil {
			return 0, err
		}
		finalAttachmentIDs = append(finalAttachmentIDs, reportAttachmentIDs...)
	}
	finalAttachmentIDs = uniqueIDs(finalAttachmentIDs)
	if copyAttachments || len(template.ReportTemplateIDs) > 0 || len(postprocessAttachmentIDs) > 0 {
		emailValues["attachment_ids"] = finalAttachmentIDs
		if err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Write(map[string]any{"attachment_ids": finalAttachmentIDs}); err != nil {
			return 0, err
		}
	}
	mailID, err := createMailRow(env, messageID, rendered, emailValues, scheduledDate, modelName, resID)
	if err != nil {
		return 0, err
	}
	if mailID == 0 {
		return 0, nil
	}
	rememberMassMailingDuplicateEmails(mailingID, massMailingRenderedEmails(env, rendered, partnerIDs), duplicateSeen)
	if !scheduledDate.IsZero() {
		if _, err := createScheduledMessage(env, template.ID, messageID, mailID, modelName, resID, rendered, partnerIDs, scheduledDate, emailValues); err != nil {
			return 0, err
		}
	}
	return mailID, nil
}

func sendDirectCompose(env *record.Env, row map[string]any, modelName string, resIDs []int64, now time.Time) ([]int64, error) {
	emailValues := composeEmailValues(row)
	rendered := RenderedMessage{
		To:      stringAny(row["email_to"]),
		CC:      stringAny(row["email_cc"]),
		Subject: stringAny(row["subject"]),
		Body:    firstText(row["body_html"], row["body"]),
	}
	partnerIDs := int64s(row["partner_ids"])
	mailingID := int64FromAny(firstNonNil(emailValues["mailing_id"], emailValues["mass_mailing_id"]))
	if strings.TrimSpace(rendered.To) == "" && len(partnerIDs) == 0 && mailingID == 0 {
		return nil, fmt.Errorf("mail compose requires recipient")
	}
	scheduledDate := timeValue(row["scheduled_date"])
	mailIDs := make([]int64, 0, len(resIDs))
	duplicateSeen := map[string]bool{}
	for _, resID := range uniqueIDs(resIDs) {
		rowAttachmentIDs := uniqueIDs(int64s(row["attachment_ids"]))
		if canceled, err := cancelMassMailingBlacklistTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
			return nil, err
		} else if canceled {
			continue
		}
		if canceled, err := cancelMassMailingRecipientFailureTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
			return nil, err
		} else if canceled {
			continue
		}
		if canceled, err := cancelMassMailingOptOutTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues); err != nil {
			return nil, err
		} else if canceled {
			continue
		}
		if canceled, err := cancelMassMailingDuplicateTrace(env, mailingID, rendered, partnerIDs, modelName, resID, emailValues, duplicateSeen); err != nil {
			return nil, err
		} else if canceled {
			continue
		}
		messageID, err := PostMessage(env, PostRequest{
			Model:       modelName,
			ResID:       resID,
			Body:        rendered.Body,
			Subject:     rendered.Subject,
			MessageType: "email",
			EmailFrom:   stringAny(row["email_from"]),
			AuthorID:    int64FromAny(row["author_id"]),
			ParentID:    int64FromAny(row["parent_id"]),
			SubtypeID:   int64FromAny(row["subtype_id"]),
			PartnerIDs:  partnerIDs,
			BodyIsHTML:  boolDefault(row["body_is_html"], true),
			AutoFollow:  boolAny(row["notify"]),
			Now:         now,
		})
		if err != nil {
			return nil, err
		}
		recordEmailValues := copyEmailValues(emailValues)
		if len(rowAttachmentIDs) > 0 {
			if err := validateAttachmentOwnership(env, rowAttachmentIDs, nil, false); err != nil {
				return nil, err
			}
			copiedAttachmentIDs, err := copyAttachmentsToMessage(env, rowAttachmentIDs, messageID)
			if err != nil {
				return nil, err
			}
			recordEmailValues["attachment_ids"] = copiedAttachmentIDs
			if err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Write(map[string]any{"attachment_ids": copiedAttachmentIDs}); err != nil {
				return nil, err
			}
		}
		mailID, err := createMailRow(env, messageID, rendered, recordEmailValues, scheduledDate, modelName, resID)
		if err != nil {
			return nil, err
		}
		if mailID == 0 {
			continue
		}
		rememberMassMailingDuplicateEmails(mailingID, massMailingRenderedEmails(env, rendered, partnerIDs), duplicateSeen)
		if !scheduledDate.IsZero() {
			if _, err := createScheduledMessage(env, 0, messageID, mailID, modelName, resID, rendered, partnerIDs, scheduledDate, recordEmailValues); err != nil {
				return nil, err
			}
		}
		mailIDs = append(mailIDs, mailID)
	}
	return mailIDs, nil
}

func createMailRow(env *record.Env, messageID int64, rendered RenderedMessage, emailValues map[string]any, scheduledDate time.Time, modelName string, resID int64) (int64, error) {
	recipientIDs := uniqueIDs(int64s(firstNonNil(emailValues["recipient_ids"], emailValues["partner_ids"])))
	mailingID := int64FromAny(firstNonNil(emailValues["mailing_id"], emailValues["mass_mailing_id"]))
	if mailingID != 0 && massMailingUseExclusionList(env, mailingID, emailValues) {
		if email := firstActiveBlacklistedMassMailingEmail(env, rendered, recipientIDs); email != "" {
			cancelValues := copyEmailValues(emailValues)
			cancelValues["state"] = "cancel"
			cancelValues["failure_type"] = "mail_bl"
			if err := createMailingTraceForMail(env, 0, mailingID, rendered, recipientIDs, modelName, resID, cancelValues); err != nil {
				return 0, err
			}
			return 0, nil
		}
	}
	if mailingID != 0 {
		body, err := shortenMassMailingRenderedLinks(env, rendered.Body, mailingID)
		if err != nil {
			return 0, err
		}
		rendered.Body = body
	}
	values := map[string]any{
		"mail_message_id": messageID,
		"recipient_ids":   recipientIDs,
		"attachment_ids":  uniqueIDs(int64s(emailValues["attachment_ids"])),
		"mail_server_id":  int64FromAny(emailValues["mail_server_id"]),
		"email_from":      stringAny(emailValues["email_from"]),
		"email_to":        rendered.To,
		"email_cc":        rendered.CC,
		"reply_to":        stringAny(emailValues["reply_to"]),
		"subject":         rendered.Subject,
		"body_html":       rendered.Body,
		"state":           "outgoing",
		"max_retries":     int64(3),
		"auto_delete":     boolAny(emailValues["auto_delete"]),
		"is_notification": true,
	}
	if mailingID != 0 {
		values["mailing_id"] = mailingID
	}
	if !scheduledDate.IsZero() {
		values["scheduled_date"] = scheduledDate.UTC()
	}
	mailID, err := env.Model("mail.mail").Create(values)
	if err != nil {
		return 0, err
	}
	if err := createEmailNotifications(env, messageID, mailID, recipientIDs); err != nil {
		return 0, err
	}
	if err := createMailingTraceForMail(env, mailID, mailingID, rendered, recipientIDs, modelName, resID, emailValues); err != nil {
		return 0, err
	}
	return mailID, nil
}

func shortenMassMailingRenderedLinks(env *record.Env, body string, mailingID int64) (string, error) {
	if env == nil || mailingID == 0 || strings.TrimSpace(body) == "" {
		return body, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("link.tracker"); !ok {
		return body, nil
	}
	baseURL := strings.TrimRight(configParameter(systemEnv, "web.base.url"), "/")
	if baseURL == "" {
		baseURL = "http://localhost"
	}
	trackerVals := massMailingLinkTrackerValues(systemEnv, mailingID)
	var firstErr error
	rewritten := rewriteMassMailingAnchors(body, func(anchor massMailingAnchor) string {
		if firstErr != nil {
			return anchor.Raw
		}
		trackerURL, ok := massMailingLinkTrackerURL(anchor.Href, baseURL)
		if !ok {
			return anchor.Raw
		}
		linkVals := copyEmailValues(trackerVals)
		linkVals["label"] = massMailingLinkLabel(anchor.Inner)
		shortURL, err := searchOrCreateLinkTrackerShortURL(systemEnv, trackerURL, linkVals, baseURL)
		if err != nil {
			firstErr = err
			return anchor.Raw
		}
		if shortURL == "" {
			return anchor.Raw
		}
		return strings.Replace(anchor.Raw, anchor.Href, shortURL, 1)
	})
	if firstErr != nil {
		return body, firstErr
	}
	return rewritten, nil
}

type massMailingAnchor struct {
	Raw   string
	Href  string
	Inner string
}

func rewriteMassMailingAnchors(body string, replace func(massMailingAnchor) string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	lower := strings.ToLower(body)
	var out strings.Builder
	out.Grow(len(body))
	pos := 0
	for pos < len(body) {
		idxRel := strings.Index(lower[pos:], "<a")
		if idxRel < 0 {
			out.WriteString(body[pos:])
			break
		}
		start := pos + idxRel
		if !massMailingAnchorNameBoundary(lower, start+2) {
			out.WriteString(body[pos : start+2])
			pos = start + 2
			continue
		}
		out.WriteString(body[pos:start])
		tagEnd := massMailingStartTagEnd(body, start)
		if tagEnd < 0 {
			out.WriteString(body[start:])
			break
		}
		tagName, attrs, selfClosing, _, ok := massMailingParseStartTag(body[start : tagEnd+1])
		if !ok || tagName != "a" {
			out.WriteString(body[start : tagEnd+1])
			pos = tagEnd + 1
			continue
		}
		href := massMailingHTMLAttr(attrs, "href")
		inner := ""
		end := tagEnd + 1
		if !selfClosing {
			closeStart, closeEnd := massMailingClosingTag(body[tagEnd+1:], "a")
			if closeStart < 0 || closeEnd < 0 {
				out.WriteString(body[start : tagEnd+1])
				pos = tagEnd + 1
				continue
			}
			inner = body[tagEnd+1 : tagEnd+1+closeStart]
			end = tagEnd + 1 + closeEnd
		}
		anchor := massMailingAnchor{Raw: body[start:end], Href: href, Inner: inner}
		if strings.TrimSpace(anchor.Href) == "" {
			out.WriteString(anchor.Raw)
		} else {
			out.WriteString(replace(anchor))
		}
		pos = end
	}
	return out.String()
}

func massMailingAnchorNameBoundary(lower string, idx int) bool {
	if idx >= len(lower) {
		return true
	}
	c := lower[idx]
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func massMailingStartTagEnd(fragment string, start int) int {
	var quote byte
	for idx := start; idx < len(fragment); idx++ {
		c := fragment[idx]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '>' {
			return idx
		}
	}
	return -1
}

func massMailingLinkTrackerValues(env *record.Env, mailingID int64) map[string]any {
	values := map[string]any{"mass_mailing_id": mailingID}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("campaign_id", "medium_id", "source_id")
	if err != nil || len(rows) == 0 {
		return values
	}
	for _, fieldName := range []string{"campaign_id", "medium_id", "source_id"} {
		if id := int64FromAny(rows[0][fieldName]); id != 0 {
			values[fieldName] = id
		}
	}
	return values
}

func massMailingLinkTrackerURL(rawURL string, baseURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "mailto" || scheme == "tel" || scheme == "sms" {
		return "", false
	}
	if strings.HasPrefix(rawURL, baseURL+"/r/") || strings.HasPrefix(rawURL, "/r/") {
		return "", false
	}
	for _, blocked := range []string{"/unsubscribe_from_list", "/view", "/cards"} {
		if massMailingPathBlocked(parsed.Path, blocked) {
			return "", false
		}
	}
	if scheme == "http" || scheme == "https" {
		return rawURL, true
	}
	if scheme != "" {
		return "", false
	}
	if strings.HasPrefix(rawURL, "/") {
		return baseURL + rawURL, true
	}
	if strings.HasPrefix(rawURL, "?") || strings.HasPrefix(rawURL, "#") {
		return baseURL + rawURL, true
	}
	return "http://" + rawURL, true
}

func massMailingPathBlocked(path string, blocked string) bool {
	idx := strings.Index(path, blocked)
	for idx >= 0 {
		after := idx + len(blocked)
		if after == len(path) {
			return true
		}
		switch path[after] {
		case '#', '?', '/':
			return true
		}
		next := strings.Index(path[after:], blocked)
		if next < 0 {
			return false
		}
		idx = after + next
	}
	return false
}

func searchOrCreateLinkTrackerShortURL(env *record.Env, trackerURL string, trackerVals map[string]any, baseURL string) (string, error) {
	linkID, err := findLinkTracker(env, trackerURL, trackerVals)
	if err != nil {
		return "", err
	}
	redirectedURL := linkTrackerRedirectedURL(env, trackerURL, trackerVals, baseURL)
	if linkID == 0 {
		values := copyEmailValues(trackerVals)
		values["url"] = trackerURL
		values["redirected_url"] = redirectedURL
		values["title"] = trackerURL
		linkID, err = env.Model("link.tracker").Create(values)
		if err != nil {
			return "", err
		}
	}
	code, err := ensureLinkTrackerCode(env, linkID)
	if err != nil || code == "" {
		return "", err
	}
	shortURL := strings.TrimRight(baseURL, "/") + "/r/" + code
	if err := env.Model("link.tracker").Browse(linkID).Write(map[string]any{"code": code, "short_url": shortURL, "short_url_host": strings.TrimRight(baseURL, "/") + "/r/", "redirected_url": redirectedURL}); err != nil {
		return "", err
	}
	return shortURL, nil
}

func SearchOrCreateLinkTrackerShortURL(env *record.Env, trackerURL string, trackerVals map[string]any, baseURL string) (string, error) {
	return searchOrCreateLinkTrackerShortURL(env, trackerURL, trackerVals, baseURL)
}

func findLinkTracker(env *record.Env, trackerURL string, trackerVals map[string]any) (int64, error) {
	found, err := env.Model("link.tracker").Search(domain.And())
	if err != nil || found.Len() == 0 {
		return 0, err
	}
	rows, err := found.Read("url", "campaign_id", "medium_id", "source_id", "label")
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["url"])) == trackerURL &&
			int64FromAny(row["campaign_id"]) == int64FromAny(trackerVals["campaign_id"]) &&
			int64FromAny(row["medium_id"]) == int64FromAny(trackerVals["medium_id"]) &&
			int64FromAny(row["source_id"]) == int64FromAny(trackerVals["source_id"]) &&
			strings.TrimSpace(stringAny(row["label"])) == strings.TrimSpace(stringAny(trackerVals["label"])) {
			return int64FromAny(row["id"]), nil
		}
	}
	return 0, nil
}

func massMailingLinkLabel(innerHTML string) string {
	cleaned := massMailingHTMLComment.ReplaceAllString(innerHTML, "")
	if label := massMailingNormalizeLinkLabel(massMailingDirectText(cleaned)); label != "" {
		return label
	}
	return massMailingNormalizeLinkLabel(massMailingChildElementLabel(cleaned))
}

func massMailingDirectText(fragment string) string {
	if idx := strings.Index(fragment, "<"); idx >= 0 {
		return fragment[:idx]
	}
	return fragment
}

func massMailingChildElementLabel(fragment string) string {
	rest := fragment
	for {
		idx := strings.Index(rest, "<")
		if idx < 0 {
			return ""
		}
		rest = rest[idx:]
		tagName, attrs, selfClosing, contentStart, ok := massMailingParseStartTag(rest)
		if !ok {
			rest = rest[1:]
			continue
		}
		switch tagName {
		case "img":
			if alt := massMailingHTMLAttr(attrs, "alt"); strings.TrimSpace(alt) != "" {
				return "[media] " + alt
			}
			if src := massMailingHTMLAttr(attrs, "src"); strings.TrimSpace(src) != "" {
				parts := strings.Split(src, "/")
				return "[media] " + parts[len(parts)-1]
			}
			return ""
		case "p":
			closeStart, closeEnd := massMailingClosingTag(rest[contentStart:], tagName)
			if massMailingHTMLAttr(attrs, "class") == "o_outlook_hack" && closeStart >= 0 {
				if label := massMailingChildElementLabel(rest[contentStart : contentStart+closeStart]); label != "" {
					return label
				}
			}
			if closeEnd >= 0 {
				rest = rest[contentStart+closeEnd:]
			} else {
				rest = rest[contentStart:]
			}
		default:
			if selfClosing {
				rest = rest[contentStart:]
				continue
			}
			_, closeEnd := massMailingClosingTag(rest[contentStart:], tagName)
			if closeEnd >= 0 {
				rest = rest[contentStart+closeEnd:]
			} else {
				rest = rest[contentStart:]
			}
		}
	}
}

func massMailingNormalizeLinkLabel(label string) string {
	label = html.UnescapeString(label)
	label = massMailingHTMLTagPattern.ReplaceAllString(label, " ")
	label = strings.TrimSpace(massMailingWhitespace.ReplaceAllString(label, " "))
	runes := []rune(label)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return label
}

func massMailingParseStartTag(fragment string) (string, string, bool, int, bool) {
	if !strings.HasPrefix(fragment, "<") || len(fragment) < 2 {
		return "", "", false, 0, false
	}
	if strings.ContainsAny(fragment[1:2], "/!?") {
		return "", "", false, 0, false
	}
	end := strings.Index(fragment, ">")
	if end < 0 {
		return "", "", false, 0, false
	}
	body := strings.TrimSpace(fragment[1:end])
	selfClosing := strings.HasSuffix(body, "/")
	body = strings.TrimSpace(strings.TrimSuffix(body, "/"))
	if body == "" {
		return "", "", false, end + 1, false
	}
	nameEnd := len(body)
	for idx, r := range body {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			nameEnd = idx
			break
		}
	}
	name := strings.ToLower(body[:nameEnd])
	attrs := ""
	if nameEnd < len(body) {
		attrs = body[nameEnd:]
	}
	return name, attrs, selfClosing, end + 1, true
}

func massMailingHTMLAttr(attrs string, attr string) string {
	attr = strings.ToLower(attr)
	for _, pattern := range []string{
		`(?is)\b` + regexp.QuoteMeta(attr) + `\s*=\s*"([^"]*)"`,
		`(?is)\b` + regexp.QuoteMeta(attr) + `\s*=\s*'([^']*)'`,
		`(?is)\b` + regexp.QuoteMeta(attr) + `\s*=\s*([^\s>]+)`,
	} {
		matches := regexp.MustCompile(pattern).FindStringSubmatch(attrs)
		if len(matches) == 2 {
			return html.UnescapeString(matches[1])
		}
	}
	return ""
}

func massMailingClosingTag(fragment string, tagName string) (int, int) {
	lower := strings.ToLower(fragment)
	prefix := "</" + strings.ToLower(tagName)
	start := strings.Index(lower, prefix)
	if start < 0 {
		return -1, -1
	}
	end := strings.Index(lower[start:], ">")
	if end < 0 {
		return start, -1
	}
	return start, start + end + 1
}

func linkTrackerRedirectedURL(env *record.Env, trackerURL string, trackerVals map[string]any, baseURL string) string {
	parsed, err := url.Parse(trackerURL)
	if err != nil {
		return trackerURL
	}
	if configParameterBoolMail(env, "link_tracker.no_external_tracking") {
		baseParsed, _ := url.Parse(baseURL)
		if parsed.Host != "" && baseParsed != nil && parsed.Host != baseParsed.Host {
			return parsed.String()
		}
	}
	query := parsed.Query()
	if value := linkTrackerUTMName(env, "utm.campaign", int64FromAny(trackerVals["campaign_id"])); value != "" {
		query.Set("utm_campaign", value)
	}
	if value := linkTrackerUTMName(env, "utm.source", int64FromAny(trackerVals["source_id"])); value != "" {
		query.Set("utm_source", value)
	}
	if value := linkTrackerUTMName(env, "utm.medium", int64FromAny(trackerVals["medium_id"])); value != "" {
		query.Set("utm_medium", value)
	}
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "...", "%2E%2E%2E")
	return parsed.String()
}

func configParameterBoolMail(env *record.Env, key string) bool {
	value := strings.TrimSpace(configParameter(env, key))
	return value == "1" || strings.EqualFold(value, "true")
}

func linkTrackerUTMName(env *record.Env, modelName string, id int64) string {
	if env == nil || id == 0 {
		return ""
	}
	rows, err := env.Model(modelName).Browse(id).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringAny(rows[0]["name"]))
}

func ensureLinkTrackerCode(env *record.Env, linkID int64) (string, error) {
	found, err := env.Model("link.tracker.code").Search(domain.Cond("link_id", "=", linkID))
	if err != nil {
		return "", err
	}
	if found.Len() > 0 {
		rows, err := found.Read("code")
		if err != nil || len(rows) == 0 {
			return "", err
		}
		code := strings.TrimSpace(stringAny(rows[0]["code"]))
		if code != "" {
			return code, nil
		}
	}
	code, err := record.NextLinkTrackerCode(env)
	if err != nil {
		return "", err
	}
	if _, err := env.Model("link.tracker.code").Create(map[string]any{"code": code, "link_id": linkID}); err != nil {
		return "", err
	}
	return code, nil
}

func createMailingTraceForMail(env *record.Env, mailID int64, mailingID int64, rendered RenderedMessage, recipientIDs []int64, modelName string, resID int64, emailValues map[string]any) error {
	if env == nil || mailingID == 0 {
		return nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.trace"); !ok {
		return nil
	}
	traceStatus := "outgoing"
	if state := strings.TrimSpace(stringAny(emailValues["state"])); state == "cancel" {
		traceStatus = "cancel"
	} else if state == "exception" {
		traceStatus = "error"
	}
	values := map[string]any{
		"email":           firstText(stringAny(emailValues["trace_email"]), firstMailingTraceEmail(env, rendered, recipientIDs)),
		"mass_mailing_id": mailingID,
		"model":           strings.TrimSpace(modelName),
		"res_id":          resID,
		"trace_status":    traceStatus,
	}
	if mailID != 0 {
		values["mail_mail_id"] = mailID
	}
	if messageID := normalizeMessageID(stringAny(emailValues["message_id"])); messageID != "" {
		values["message_id"] = messageID
	}
	if failureType := strings.TrimSpace(stringAny(emailValues["failure_type"])); failureType != "" {
		values["failure_type"] = failureType
	}
	_, err := systemEnv.Model("mailing.trace").Create(values)
	return err
}

func firstMailingTraceEmail(env *record.Env, rendered RenderedMessage, recipientIDs []int64) string {
	for _, raw := range []string{rendered.To, rendered.CC} {
		_, emails, _ := recipientAddressList(raw)
		if len(emails) > 0 {
			return emails[0]
		}
	}
	for _, partnerID := range recipientIDs {
		if email := normalizedEmailAddress(partnerEmail(env, partnerID)); email != "" {
			return email
		}
	}
	return ""
}

func firstActiveBlacklistedMassMailingEmail(env *record.Env, rendered RenderedMessage, recipientIDs []int64) string {
	if env == nil {
		return ""
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.blacklist"); !ok {
		return ""
	}
	for _, email := range massMailingRenderedEmails(env, rendered, recipientIDs) {
		found, err := systemEnv.Model("mail.blacklist").Search(domain.Cond("email", "=", email))
		if err != nil {
			return ""
		}
		rows, err := found.Read("active")
		if err != nil {
			return ""
		}
		for _, row := range rows {
			if boolDefault(row["active"], true) {
				return email
			}
		}
	}
	return ""
}

func massMailingRenderedEmails(env *record.Env, rendered RenderedMessage, recipientIDs []int64) []string {
	emails := []string{}
	for _, raw := range []string{rendered.To, rendered.CC} {
		_, parsed, _ := recipientAddressList(raw)
		emails = append(emails, parsed...)
	}
	for _, partnerID := range uniqueIDs(recipientIDs) {
		if email := normalizedEmailAddress(partnerEmail(env, partnerID)); email != "" {
			emails = append(emails, email)
		}
	}
	return uniqueStrings(emails)
}

func cancelMassMailingBlacklistTrace(env *record.Env, mailingID int64, rendered RenderedMessage, recipientIDs []int64, modelName string, resID int64, emailValues map[string]any) (bool, error) {
	if mailingID == 0 || !massMailingUseExclusionList(env, mailingID, emailValues) {
		return false, nil
	}
	if email := firstActiveBlacklistedMassMailingEmail(env, rendered, recipientIDs); email != "" {
		cancelValues := copyEmailValues(emailValues)
		cancelValues["state"] = "cancel"
		cancelValues["failure_type"] = "mail_bl"
		if err := createMailingTraceForMail(env, 0, mailingID, rendered, recipientIDs, modelName, resID, cancelValues); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func cancelMassMailingRecipientFailureTrace(env *record.Env, mailingID int64, rendered RenderedMessage, recipientIDs []int64, modelName string, resID int64, emailValues map[string]any) (bool, error) {
	if mailingID == 0 {
		return false, nil
	}
	info := massMailingPrimaryRecipientInfo(env, rendered, recipientIDs)
	failureType := ""
	switch {
	case len(info.raw) == 0:
		failureType = "mail_email_missing"
	case len(info.normalized) == 0:
		failureType = "mail_email_invalid"
	default:
		return false, nil
	}
	cancelValues := copyEmailValues(emailValues)
	cancelValues["state"] = "cancel"
	cancelValues["failure_type"] = failureType
	if info.firstRaw != "" {
		cancelValues["trace_email"] = info.firstRaw
	}
	if err := createMailingTraceForMail(env, 0, mailingID, rendered, recipientIDs, modelName, resID, cancelValues); err != nil {
		return false, err
	}
	return true, nil
}

func cancelMassMailingOptOutTrace(env *record.Env, mailingID int64, rendered RenderedMessage, recipientIDs []int64, modelName string, resID int64, emailValues map[string]any) (bool, error) {
	if mailingID == 0 {
		return false, nil
	}
	emails := massMailingPrimaryRecipientInfo(env, rendered, recipientIDs).normalized
	optedOut, err := massMailingSelectedListEmailsOptedOut(env, mailingID, emails)
	if err != nil || !optedOut {
		return false, err
	}
	cancelValues := copyEmailValues(emailValues)
	cancelValues["state"] = "cancel"
	cancelValues["failure_type"] = "mail_optout"
	if err := createMailingTraceForMail(env, 0, mailingID, rendered, recipientIDs, modelName, resID, cancelValues); err != nil {
		return false, err
	}
	return true, nil
}

func cancelMassMailingDuplicateTrace(env *record.Env, mailingID int64, rendered RenderedMessage, recipientIDs []int64, modelName string, resID int64, emailValues map[string]any, seen map[string]bool) (bool, error) {
	if mailingID == 0 {
		return false, nil
	}
	emails := massMailingRenderedEmails(env, rendered, recipientIDs)
	seenAll, err := massMailingEmailsAlreadySeen(env, mailingID, emails, seen, 0)
	if err != nil || !seenAll {
		return false, err
	}
	cancelValues := copyEmailValues(emailValues)
	cancelValues["state"] = "cancel"
	cancelValues["failure_type"] = "mail_dup"
	if err := createMailingTraceForMail(env, 0, mailingID, rendered, recipientIDs, modelName, resID, cancelValues); err != nil {
		return false, err
	}
	return true, nil
}

type massMailingRecipientInfo struct {
	raw        []string
	normalized []string
	firstRaw   string
}

func massMailingPrimaryRecipientInfo(env *record.Env, rendered RenderedMessage, recipientIDs []int64) massMailingRecipientInfo {
	info := massMailingRecipientInfo{}
	if raw := strings.TrimSpace(rendered.To); raw != "" {
		_, emails, _ := recipientAddressList(raw)
		if len(emails) == 0 {
			info.raw = append(info.raw, raw)
			info.firstRaw = raw
		} else {
			info.raw = append(info.raw, emails...)
			info.normalized = append(info.normalized, emails...)
			info.firstRaw = emails[0]
		}
	}
	for _, partnerID := range uniqueIDs(recipientIDs) {
		email := strings.TrimSpace(partnerEmail(env, partnerID))
		if email == "" {
			continue
		}
		if info.firstRaw == "" {
			info.firstRaw = email
		}
		info.raw = append(info.raw, email)
		if normalized := normalizedEmailAddress(email); normalized != "" {
			info.normalized = append(info.normalized, normalized)
		}
	}
	info.raw = uniqueStrings(info.raw)
	info.normalized = uniqueStrings(info.normalized)
	return info
}

func massMailingSelectedListEmailsOptedOut(env *record.Env, mailingID int64, emails []string) (bool, error) {
	emails = uniqueStrings(emails)
	if env == nil || mailingID == 0 || len(emails) == 0 {
		return false, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.mailing"); !ok {
		return false, nil
	}
	rows, err := systemEnv.Model("mailing.mailing").Browse(mailingID).Read("mailing_on_mailing_list", "contact_list_ids")
	if err != nil || len(rows) == 0 {
		return false, err
	}
	if !boolAny(rows[0]["mailing_on_mailing_list"]) {
		return false, nil
	}
	listIDs := uniqueIDs(int64s(rows[0]["contact_list_ids"]))
	if len(listIDs) == 0 {
		return false, nil
	}
	contactIDsByEmail, err := mailingContactIDsByEmail(systemEnv, emails, nil)
	if err != nil || len(contactIDsByEmail) == 0 {
		return false, err
	}
	for _, email := range emails {
		contactIDs := contactIDsByEmail[email]
		if len(contactIDs) == 0 {
			return false, nil
		}
		optOut, optIn, err := mailingListSubscriptionState(systemEnv, contactIDs, listIDs)
		if err != nil {
			return false, err
		}
		if !optOut || optIn {
			return false, nil
		}
	}
	return true, nil
}

func massMailingEmailsAlreadySeen(env *record.Env, mailingID int64, emails []string, seen map[string]bool, beforeMailID int64) (bool, error) {
	emails = uniqueStrings(emails)
	if mailingID == 0 || len(emails) == 0 {
		return false, nil
	}
	for _, email := range emails {
		if seen != nil && seen[massMailingDuplicateKey(mailingID, email)] {
			continue
		}
		traceSeen, err := massMailingTraceEmailSeen(env, mailingID, email, beforeMailID)
		if err != nil {
			return false, err
		}
		if !traceSeen {
			return false, nil
		}
	}
	return true, nil
}

func rememberMassMailingDuplicateEmails(mailingID int64, emails []string, seen map[string]bool) {
	if mailingID == 0 || seen == nil {
		return
	}
	for _, email := range uniqueStrings(emails) {
		seen[massMailingDuplicateKey(mailingID, email)] = true
	}
}

func massMailingDuplicateKey(mailingID int64, email string) string {
	return fmt.Sprintf("%d|%s", mailingID, normalizedEmailAddress(email))
}

func massMailingTraceEmailSeen(env *record.Env, mailingID int64, email string, beforeMailID int64) (bool, error) {
	if env == nil || mailingID == 0 {
		return false, nil
	}
	email = normalizedEmailAddress(email)
	if email == "" {
		return false, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.trace"); !ok {
		return false, nil
	}
	found, err := systemEnv.Model("mailing.trace").Search(domain.Cond("mass_mailing_id", "=", mailingID))
	if err != nil {
		return false, err
	}
	rows, err := found.Read("email", "mail_mail_id", "mail_mail_id_int", "trace_status", "failure_type")
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if normalizedEmailAddress(stringAny(row["email"])) != email {
			continue
		}
		if strings.TrimSpace(stringAny(row["trace_status"])) == "cancel" {
			continue
		}
		switch strings.TrimSpace(stringAny(row["failure_type"])) {
		case "mail_bl", "mail_optout", "mail_dup":
			continue
		}
		if beforeMailID != 0 {
			traceMailID := int64FromAny(row["mail_mail_id"])
			if traceMailID == 0 {
				traceMailID = int64FromAny(row["mail_mail_id_int"])
			}
			if traceMailID == 0 || traceMailID >= beforeMailID {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}

func massMailingUseExclusionList(env *record.Env, mailingID int64, emailValues map[string]any) bool {
	if value, ok := emailValues["use_exclusion_list"]; ok {
		return boolDefault(value, true)
	}
	if env == nil || mailingID == 0 {
		return true
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.mailing"); !ok {
		return true
	}
	rows, err := systemEnv.Model("mailing.mailing").Browse(mailingID).Read("use_exclusion_list")
	if err != nil || len(rows) == 0 {
		return true
	}
	if value, ok := rows[0]["use_exclusion_list"]; ok {
		return boolDefault(value, true)
	}
	return true
}

func createScheduledMessage(env *record.Env, templateID int64, messageID int64, mailID int64, modelName string, resID int64, rendered RenderedMessage, partnerIDs []int64, scheduledDate time.Time, emailValues map[string]any) (int64, error) {
	values := map[string]any{
		"mail_message_id":            messageID,
		"mail_mail_id":               mailID,
		"mail_template_id":           templateID,
		"model":                      modelName,
		"res_id":                     resID,
		"scheduled_date":             scheduledDate.UTC(),
		"author_id":                  int64FromAny(emailValues["author_id"]),
		"subject":                    rendered.Subject,
		"body":                       rendered.Body,
		"partner_ids":                uniqueIDs(partnerIDs),
		"attachment_ids":             uniqueIDs(int64s(emailValues["attachment_ids"])),
		"composition_comment_option": stringAny(emailValues["composition_comment_option"]),
		"is_note":                    boolAny(emailValues["is_note"]),
		"notification_parameters":    stringAny(emailValues["notification_parameters"]),
		"send_context":               stringAny(emailValues["send_context"]),
		"state":                      "scheduled",
	}
	if !boolAny(values["is_note"]) && stringAny(values["composition_comment_option"]) == "log" {
		values["is_note"] = true
	}
	return env.Model("mail.scheduled.message").Create(values)
}

func copyEmailValues(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func copyAttachmentsToMessage(env *record.Env, attachmentIDs []int64, messageID int64) ([]int64, error) {
	return copyAttachmentsToRecord(env, attachmentIDs, "mail.message", messageID)
}

func copyAttachmentsToRecord(env *record.Env, attachmentIDs []int64, modelName string, resID int64) ([]int64, error) {
	attachmentIDs = uniqueIDs(attachmentIDs)
	if len(attachmentIDs) == 0 {
		return nil, nil
	}
	systemEnv := messageSystemEnv(env)
	rows, err := systemEnv.Model("ir.attachment").Browse(attachmentIDs...).Read("id", "name", "type", "mimetype", "datas", "url", "file_size", "public", "checksum", "has_thumbnail")
	if err != nil {
		return nil, err
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[int64FromAny(row["id"])] = row
	}
	copiedIDs := make([]int64, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		row, ok := byID[attachmentID]
		if !ok {
			return nil, fmt.Errorf("attachment %d not found", attachmentID)
		}
		values := map[string]any{
			"name":          stringAny(row["name"]),
			"res_model":     modelName,
			"res_id":        resID,
			"type":          stringAny(row["type"]),
			"mimetype":      stringAny(row["mimetype"]),
			"datas":         row["datas"],
			"url":           stringAny(row["url"]),
			"file_size":     int64FromAny(row["file_size"]),
			"public":        boolAny(row["public"]),
			"checksum":      stringAny(row["checksum"]),
			"has_thumbnail": boolAny(row["has_thumbnail"]),
		}
		if strings.TrimSpace(stringAny(values["type"])) == "" {
			values["type"] = "binary"
		}
		copiedID, err := systemEnv.Model("ir.attachment").Create(values)
		if err != nil {
			return nil, err
		}
		copiedIDs = append(copiedIDs, copiedID)
	}
	return copiedIDs, nil
}

func loadTemplate(env *record.Env, templateID int64) (Template, string, error) {
	fields := []string{"name", "model", "subject", "body_html", "email_from", "email_to", "email_cc", "reply_to", "partner_to", "attachment_ids", "report_template_ids", "scheduled_date"}
	if meta, ok := env.ModelMetadata("mail.template"); ok {
		if _, ok := meta.Fields["delegation_group_ids"]; ok {
			fields = append(fields, "delegation_group_ids")
		}
	}
	rows, err := env.Model("mail.template").Browse(templateID).Read(fields...)
	if err != nil {
		return Template{}, "", err
	}
	if len(rows) == 0 {
		return Template{}, "", fmt.Errorf("mail.template:%d not found", templateID)
	}
	row := rows[0]
	return Template{
		ID:                 templateID,
		Name:               stringAny(row["name"]),
		To:                 stringAny(row["email_to"]),
		CC:                 stringAny(row["email_cc"]),
		EmailFrom:          stringAny(row["email_from"]),
		ReplyTo:            stringAny(row["reply_to"]),
		PartnerTo:          stringAny(row["partner_to"]),
		ScheduledDate:      timeValue(row["scheduled_date"]),
		Subject:            stringAny(row["subject"]),
		Body:               stringAny(row["body_html"]),
		AttachmentIDs:      uniqueIDs(int64s(row["attachment_ids"])),
		ReportTemplateIDs:  uniqueIDs(int64s(row["report_template_ids"])),
		DelegationGroupIDs: int64s(row["delegation_group_ids"]),
	}, stringAny(row["model"]), nil
}

func CreateTemplateReportAttachments(env *record.Env, templateID int64, modelName string, resID int64, targetModel string, targetID int64) ([]int64, error) {
	return CreateTemplateReportAttachmentsExcept(env, templateID, modelName, resID, targetModel, targetID, nil)
}

func CreateTemplatePostprocessAttachments(env *record.Env, templateID int64, modelName string, resID int64, targetModel string, targetID int64) ([]int64, error) {
	attachmentIDs, err := TemplatePostprocessAttachmentIDs(env, templateID, modelName, resID)
	if err != nil {
		return nil, err
	}
	return copyAttachmentsToRecord(env, attachmentIDs, targetModel, targetID)
}

func TemplatePostprocessAttachmentIDs(env *record.Env, templateID int64, modelName string, resID int64) ([]int64, error) {
	if env == nil || templateID == 0 || resID == 0 {
		return nil, nil
	}
	switch strings.TrimSpace(modelName) {
	case "account.move":
		return accountMoveTemplatePostprocessAttachmentIDs(env, resID)
	case "account.payment":
		return accountPaymentTemplatePostprocessAttachmentIDs(env, resID)
	default:
		return nil, nil
	}
}

func accountMoveTemplatePostprocessAttachmentIDs(env *record.Env, moveID int64) ([]int64, error) {
	found, err := messageSystemEnv(env).Model("ir.attachment").Search(domain.And(
		domain.Cond("res_model", domain.Equal, "account.move"),
		domain.Cond("res_id", domain.Equal, moveID),
		domain.Cond("res_field", domain.Equal, "ubl_cii_xml_file"),
	))
	if err != nil {
		return nil, err
	}
	if found.Len() == 0 {
		return nil, nil
	}
	rows, err := found.Read("id", "type", "datas", "url")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if stringAny(row["type"]) == "url" && strings.TrimSpace(stringAny(row["url"])) == "" {
			continue
		}
		if stringAny(row["type"]) != "url" && len(bytesAny(row["datas"])) == 0 {
			continue
		}
		ids = append(ids, int64FromAny(row["id"]))
	}
	return uniqueIDs(ids), nil
}

func accountPaymentTemplatePostprocessAttachmentIDs(env *record.Env, paymentID int64) ([]int64, error) {
	meta, ok := env.ModelMetadata("account.payment")
	if !ok {
		return nil, nil
	}
	if _, ok := meta.Fields["l10n_mx_edi_cfdi_attachment_id"]; !ok {
		return nil, nil
	}
	rows, err := env.Model("account.payment").Browse(paymentID).Read("l10n_mx_edi_cfdi_attachment_id")
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, nil
	}
	id := int64FromAny(rows[0]["l10n_mx_edi_cfdi_attachment_id"])
	if id == 0 {
		return nil, nil
	}
	return []int64{id}, nil
}

func CreateTemplateReportAttachmentsExcept(env *record.Env, templateID int64, modelName string, resID int64, targetModel string, targetID int64, excludedReportIDs []int64) ([]int64, error) {
	template, templateModel, err := loadTemplate(env, templateID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = templateModel
	}
	if len(excludedReportIDs) > 0 {
		excluded := map[int64]bool{}
		for _, id := range excludedReportIDs {
			excluded[id] = true
		}
		reportIDs := template.ReportTemplateIDs[:0]
		for _, reportID := range template.ReportTemplateIDs {
			if !excluded[reportID] {
				reportIDs = append(reportIDs, reportID)
			}
		}
		template.ReportTemplateIDs = reportIDs
	}
	return createTemplateReportAttachments(env, template, modelName, resID, targetModel, targetID, nil)
}

func createTemplateReportAttachments(env *record.Env, template Template, modelName string, resID int64, targetModel string, targetID int64, values map[string]any) ([]int64, error) {
	specs, err := templateReportAttachmentSpecs(env, template, modelName, resID, values)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, nil
	}
	systemEnv := messageSystemEnv(env)
	ids := make([]int64, 0, len(specs))
	for _, spec := range specs {
		if err := prepareReportAttachmentCache(systemEnv, spec); err != nil {
			return nil, err
		}
		id, err := systemEnv.Model("ir.attachment").Create(map[string]any{
			"name":      spec.Name,
			"res_model": targetModel,
			"res_id":    targetID,
			"type":      "binary",
			"mimetype":  spec.Mimetype,
			"datas":     spec.Data,
			"file_size": len(spec.Data),
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func prepareReportAttachmentCache(env *record.Env, spec TemplateReportAttachment) error {
	if env == nil || strings.TrimSpace(spec.CacheName) == "" || strings.TrimSpace(spec.SourceModel) == "" || spec.SourceID == 0 || boolAny(env.Context().Values["report_pdf_no_attachment"]) {
		return nil
	}
	cacheID, err := findReportAttachmentCache(env, spec)
	if err != nil {
		return err
	}
	if cacheID != 0 {
		return nil
	}
	_, err = env.Model("ir.attachment").Create(map[string]any{
		"name":      spec.CacheName,
		"res_model": spec.SourceModel,
		"res_id":    spec.SourceID,
		"type":      "binary",
		"mimetype":  spec.Mimetype,
		"datas":     spec.Data,
		"file_size": len(spec.Data),
	})
	return err
}

func findReportAttachmentCache(env *record.Env, spec TemplateReportAttachment) (int64, error) {
	found, err := env.Model("ir.attachment").SearchWithOptions(domain.And(
		domain.Cond("name", domain.Equal, spec.CacheName),
		domain.Cond("res_model", domain.Equal, spec.SourceModel),
		domain.Cond("res_id", domain.Equal, spec.SourceID),
	), record.SearchOptions{Limit: 1})
	if err != nil {
		return 0, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func reportAttachmentCacheData(env *record.Env, spec TemplateReportAttachment) ([]byte, string, bool, error) {
	if env == nil || !spec.UseCache || strings.TrimSpace(spec.CacheName) == "" || strings.TrimSpace(spec.SourceModel) == "" || spec.SourceID == 0 || boolAny(env.Context().Values["report_pdf_no_attachment"]) {
		return nil, "", false, nil
	}
	cacheID, err := findReportAttachmentCache(env, spec)
	if err != nil {
		return nil, "", false, err
	}
	if cacheID == 0 {
		return nil, "", false, nil
	}
	rows, err := env.Model("ir.attachment").Browse(cacheID).Read("datas", "mimetype")
	if err != nil {
		return nil, "", false, err
	}
	if len(rows) != 1 {
		return nil, "", false, nil
	}
	data := bytesAny(rows[0]["datas"])
	if len(data) == 0 {
		return nil, "", false, nil
	}
	return data, stringAny(rows[0]["mimetype"]), true, nil
}

func templateReportAttachmentSpecs(env *record.Env, template Template, modelName string, resID int64, values map[string]any) ([]TemplateReportAttachment, error) {
	reportIDs := uniqueIDs(template.ReportTemplateIDs)
	if len(reportIDs) == 0 {
		return nil, nil
	}
	rows, err := env.Model("ir.actions.report").Browse(reportIDs...).Read("id", "name", "model", "report_name", "report_type", "print_report_name", "attachment", "attachment_use")
	if err != nil {
		return nil, err
	}
	printExpressions := make([]string, 0, len(rows))
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[int64FromAny(row["id"])] = row
		printExpressions = append(printExpressions, stringAny(row["print_report_name"]))
	}
	if renderedValues, err := renderValues(env, modelName, resID, printExpressions...); err != nil {
		return nil, err
	} else {
		if values == nil {
			values = renderedValues
		} else {
			for key, value := range renderedValues {
				if _, exists := values[key]; !exists {
					values[key] = value
				}
			}
		}
	}
	specs := make([]TemplateReportAttachment, 0, len(reportIDs))
	for _, reportID := range reportIDs {
		row, ok := byID[reportID]
		if !ok {
			return nil, fmt.Errorf("ir.actions.report:%d not found", reportID)
		}
		reportModel := strings.TrimSpace(stringAny(row["model"]))
		if reportModel != "" && modelName != "" && reportModel != modelName {
			return nil, fmt.Errorf("report %d model %s does not match template model %s", reportID, reportModel, modelName)
		}
		format := reportAttachmentFormat(stringAny(row["report_type"]))
		name := renderReportAttachmentName(stringAny(row["print_report_name"]), values)
		if name == "" {
			name = firstText(row["name"], row["report_name"], "Report")
		}
		name = ensureReportAttachmentExtension(name, format)
		reportName := firstText(row["report_name"], row["name"], fmt.Sprintf("report-%d", reportID))
		cacheName := renderReportAttachmentName(stringAny(row["attachment"]), values)
		if format != "pdf" {
			cacheName = ""
		}
		spec := TemplateReportAttachment{
			ReportID:       reportID,
			Name:           name,
			Mimetype:       reportAttachmentMimetype(format),
			Data:           deterministicReportBytes(reportName, modelName, resID, format),
			CacheName:      cacheName,
			UseCache:       boolAny(row["attachment_use"]),
			SourceModel:    modelName,
			SourceID:       resID,
			SourceReportID: reportID,
		}
		if data, mimetype, ok, err := reportAttachmentCacheData(env, spec); err != nil {
			return nil, err
		} else if ok {
			spec.Data = data
			if strings.TrimSpace(mimetype) != "" {
				spec.Mimetype = mimetype
			}
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

var reportPercentExpressionPattern = regexp.MustCompile(`^['"]([^'"]*)['"]\s*%\s*\((.*)\)$`)
var objectFieldExpressionPattern = regexp.MustCompile(`object\.([A-Za-z0-9_]+)`)

func renderReportAttachmentName(expression string, values map[string]any) string {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return ""
	}
	if match := reportPercentExpressionPattern.FindStringSubmatch(expression); len(match) == 3 {
		format := match[1]
		args := reportExpressionArgs(match[2], values)
		if len(args) == 1 {
			return strings.TrimSpace(fmt.Sprintf(format, args[0]))
		}
		return strings.TrimSpace(fmt.Sprintf(format, args...))
	}
	if quoted, ok := unquoteSimpleExpression(expression); ok {
		return strings.TrimSpace(renderText(quoted, values))
	}
	return strings.TrimSpace(renderText(expression, values))
}

func reportExpressionArgs(expression string, values map[string]any) []any {
	parts := strings.Split(expression, ",")
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if match := objectFieldExpressionPattern.FindStringSubmatch(part); len(match) == 2 {
			if value, ok := values["object."+match[1]]; ok {
				out = append(out, value)
				continue
			}
			out = append(out, values[match[1]])
			continue
		}
		if quoted, ok := unquoteSimpleExpression(part); ok {
			out = append(out, quoted)
			continue
		}
		out = append(out, renderText(part, values))
	}
	return out
}

func unquoteSimpleExpression(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return "", false
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
		return value[1 : len(value)-1], true
	}
	return "", false
}

func reportAttachmentFormat(reportType string) string {
	reportType = strings.TrimSpace(strings.ToLower(reportType))
	if reportType == "" || reportType == "qweb-pdf" || reportType == "qweb-html" || strings.Contains(reportType, "pdf") {
		return "pdf"
	}
	if index := strings.LastIndex(reportType, "-"); index >= 0 && index < len(reportType)-1 {
		return reportType[index+1:]
	}
	return reportType
}

func reportAttachmentMimetype(format string) string {
	switch strings.TrimSpace(strings.ToLower(format)) {
	case "pdf":
		return "application/pdf"
	case "html":
		return "text/html"
	case "txt", "text":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func ensureReportAttachmentExtension(name string, format string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Report"
	}
	extension := "." + strings.TrimPrefix(strings.ToLower(strings.TrimSpace(format)), ".")
	if extension == "." {
		return name
	}
	if strings.HasSuffix(strings.ToLower(name), extension) {
		return name
	}
	return name + extension
}

func deterministicReportBytes(reportName string, modelName string, resID int64, format string) []byte {
	body := fmt.Sprintf("%s %s %d", reportName, modelName, resID)
	if strings.TrimSpace(strings.ToLower(format)) != "pdf" {
		return []byte(body)
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", pdfEscapeText(body))
	length := len(stream)
	return []byte(fmt.Sprintf("%%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >> endobj\n4 0 obj << /Length %d >> stream\n%s\nendstream endobj\n5 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\ntrailer << /Root 1 0 R >>\n%%%%EOF\n", length, stream))
}

func pdfEscapeText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(text)
}

func splitRecipientList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		email := strings.TrimSpace(part)
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, email)
	}
	return out
}

func joinRecipientList(values []string) string {
	return strings.Join(uniqueRecipientList(values), ", ")
}

func uniqueRecipientList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func templateEmailValues(template Template, values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	if _, ok := out["email_from"]; !ok && strings.TrimSpace(template.EmailFrom) != "" {
		out["email_from"] = template.EmailFrom
	}
	if _, ok := out["reply_to"]; !ok && strings.TrimSpace(template.ReplyTo) != "" {
		out["reply_to"] = template.ReplyTo
	}
	if _, ok := out["partner_to"]; !ok && strings.TrimSpace(template.PartnerTo) != "" {
		out["partner_to"] = template.PartnerTo
	}
	if _, ok := out["scheduled_date"]; !ok && !template.ScheduledDate.IsZero() {
		out["scheduled_date"] = template.ScheduledDate
	}
	if _, ok := out["attachment_ids"]; !ok && len(template.AttachmentIDs) > 0 {
		out["attachment_ids"] = append([]int64(nil), template.AttachmentIDs...)
	}
	return out
}

func renderValues(env *record.Env, modelName string, resID int64, texts ...string) (map[string]any, error) {
	values := map[string]any{
		"id":        resID,
		"res_id":    resID,
		"object.id": resID,
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || resID == 0 {
		return values, nil
	}
	fields := placeholderFields(texts...)
	if len(fields) == 0 {
		fields = []string{"name", "display_name", "email", "scheduled_date"}
	}
	rows, err := env.Model(modelName).Browse(resID).Read(fields...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s:%d not found", modelName, resID)
	}
	for key, value := range rows[0] {
		values[key] = value
		values["object."+key] = value
	}
	if _, ok := values["display_name"]; !ok {
		if pairs, err := env.Model(modelName).Browse(resID).NameGet(); err == nil && len(pairs) > 0 {
			values["display_name"] = pairs[0][1]
			values["object.display_name"] = pairs[0][1]
		}
	}
	return values, nil
}

func placeholderFields(texts ...string) []string {
	seen := map[string]bool{}
	for _, text := range texts {
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			key := strings.TrimSpace(match[1])
			key = strings.TrimPrefix(key, "object.")
			if dot := strings.IndexByte(key, '.'); dot >= 0 {
				key = key[:dot]
			}
			if key == "" || key == "id" || key == "res_id" {
				continue
			}
			seen[key] = true
		}
		for _, match := range objectFieldExpressionPattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			key := strings.TrimSpace(match[1])
			if key == "" || key == "id" || key == "res_id" {
				continue
			}
			seen[key] = true
		}
	}
	fields := make([]string, 0, len(seen))
	for field := range seen {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func emailsForPartners(env *record.Env, partnerIDs []int64) string {
	if len(partnerIDs) == 0 {
		return ""
	}
	rows, err := env.Model("res.partner").Browse(uniqueIDs(partnerIDs)...).Read("email")
	if err != nil {
		return ""
	}
	emails := make([]string, 0, len(rows))
	for _, row := range rows {
		if email := strings.TrimSpace(stringAny(row["email"])); email != "" {
			emails = append(emails, email)
		}
	}
	return strings.Join(emails, ",")
}

func composeEmailValues(row map[string]any) map[string]any {
	values := map[string]any{}
	for _, key := range []string{"subject", "body", "body_html", "email_from", "email_to", "email_cc", "reply_to", "partner_ids", "recipient_ids", "attachment_ids", "mail_server_id", "parent_id", "subtype_id", "scheduled_date", "author_id", "auto_delete", "mail_post_autofollow", "composition_comment_option", "notification_parameters", "send_context", "is_note", "mass_mailing_id", "mailing_id", "use_exclusion_list", "state", "failure_type", "message_id"} {
		if value, ok := row[key]; ok && !emptyAny(value) {
			values[key] = value
		}
	}
	return values
}

func composeAllowsNoRecipient(row map[string]any) bool {
	return stringAny(row["composition_comment_option"]) == "log" ||
		stringAny(row["message_type"]) == "comment" ||
		boolAny(row["subtype_is_log"])
}

func normalizeEmails(values []string) []*netmail.Address {
	out := []*netmail.Address{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			address, err := netmail.ParseAddress(part)
			if err != nil {
				address = &netmail.Address{Address: part}
			}
			address.Address = strings.TrimSpace(strings.ToLower(address.Address))
			if address.Address != "" {
				out = append(out, address)
			}
		}
	}
	return out
}

func putDefault(values map[string]any, key string, value any) {
	if _, ok := values[key]; ok || emptyAny(value) {
		return
	}
	values[key] = value
}

func firstText(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(stringAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func int64s(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []int:
		out := make([]int64, 0, len(typed))
		for _, id := range typed {
			out = append(out, int64(id))
		}
		return out
	case []float64:
		out := make([]int64, 0, len(typed))
		for _, id := range typed {
			out = append(out, int64(id))
		}
		return out
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if command, ok := item.([]any); ok && len(command) >= 2 {
				switch int64FromAny(command[0]) {
				case 4:
					out = append(out, int64FromAny(command[1]))
					continue
				case 6:
					out = append(out, int64s(command[len(command)-1])...)
					continue
				}
			}
			if id := int64FromAny(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	case string:
		return parseIDList(typed)
	default:
		if id := int64FromAny(value); id != 0 {
			return []int64{id}
		}
		return nil
	}
}

func parseIDList(text string) []int64 {
	parts := strings.FieldsFunc(strings.TrimSpace(text), func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func timeValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func boolAny(value any) bool {
	return boolDefault(value, false)
}

func bytesAny(value any) []byte {
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...)
	case string:
		return []byte(typed)
	default:
		return nil
	}
}

func boolDefault(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		default:
			return fallback
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return fallback
	}
}

func emptyAny(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []int64:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}
