package mail

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"hash/adler32"
	"html"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type PostRequest struct {
	Model               string
	ResID               int64
	Body                string
	Subject             string
	MessageType         string
	EmailFrom           string
	MessageID           string
	IncomingEmailTo     string
	IncomingEmailCC     string
	OutgoingEmailTo     string
	ReplyTo             string
	ReplyToForceNew     bool
	MailServerID        int64
	EmailLayoutXMLID    string
	EmailAddSignature   bool
	AuthorID            int64
	AuthorGuestID       int64
	ParentID            int64
	AccessToken         string
	AccessHash          string
	AccessPID           int64
	ProjectSharingID    int64
	SubtypeXMLID        string
	SubtypeID           int64
	MailActivityTypeID  int64
	PartnerIDs          []int64
	AttachmentIDs       []int64
	AttachmentIDsSet    bool
	PreserveAttachments bool
	AttachmentTokens    []string
	AttachmentTokensSet bool
	TrackingValues      []TrackingValue
	BodyIsHTML          bool
	AutoFollow          bool
	Now                 time.Time
}

type TrackingValue struct {
	FieldID          int64
	FieldInfo        string
	FieldName        string
	FieldDesc        string
	FieldType        string
	OldValueInteger  int64
	OldValueFloat    float64
	OldValueChar     string
	OldValueText     string
	OldValueDatetime time.Time
	NewValueInteger  int64
	NewValueFloat    float64
	NewValueChar     string
	NewValueText     string
	NewValueDatetime time.Time
	CurrencyID       int64
}

type MessageContentUpdateRequest struct {
	MessageID           int64
	Body                string
	BodySet             bool
	AccessToken         string
	AccessHash          string
	AccessPID           int64
	ProjectSharingID    int64
	AttachmentIDs       []int64
	AttachmentIDsSet    bool
	AttachmentTokens    []string
	AttachmentTokensSet bool
	PartnerIDs          []int64
	PartnerIDsSet       bool
}

func PostMessage(env *record.Env, req PostRequest) (int64, error) {
	if env == nil {
		return 0, fmt.Errorf("mail thread requires env")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" || req.Model == "mail.thread" {
		return 0, fmt.Errorf("posting a message requires a business document")
	}
	if req.ResID == 0 {
		return 0, fmt.Errorf("posting a message requires record id")
	}
	if req.AccessToken != "" || req.AccessHash != "" || req.AccessPID != 0 || req.ProjectSharingID != 0 {
		env = PortalContextEnvWithAccess(env, req.Model, req.ResID, PortalAccessRequest{
			AccessToken:      req.AccessToken,
			AccessHash:       req.AccessHash,
			AccessPID:        req.AccessPID,
			ProjectSharingID: req.ProjectSharingID,
		})
	}
	if err := ensureRecordExists(env, req.Model, req.ResID); err != nil {
		return 0, err
	}
	if env.Context().UserID == 0 && req.AuthorID == 0 {
		req.AuthorID = currentPortalPartnerID(env)
	}
	if req.AuthorGuestID == 0 && req.AuthorID == 0 {
		req.AuthorGuestID = currentGuestID(env)
	}
	messageType := strings.TrimSpace(req.MessageType)
	if messageType == "" {
		messageType = "notification"
	}
	if messageType == "user_notification" {
		return 0, fmt.Errorf("use message_notify for user notifications")
	}
	messageEnv := env
	if env.Context().UserID == 0 && portalContextMatches(env, req.Model, req.ResID) {
		messageEnv = messageSystemEnv(env)
	}
	partnerIDs := uniqueIDs(req.PartnerIDs)
	if req.AutoFollow && len(partnerIDs) > 0 {
		if err := Subscribe(messageEnv, req.Model, req.ResID, partnerIDs, nil); err != nil {
			return 0, err
		}
	}
	subtypeID := req.SubtypeID
	if subtypeID == 0 && strings.TrimSpace(req.SubtypeXMLID) != "" {
		subtypeID = resolveSubtypeXMLID(env, req.SubtypeXMLID)
	}
	if subtypeID == 0 {
		subtypeID = resolveSubtypeXMLID(env, "mail.mt_note")
	}
	attachmentIDs := uniqueIDs(req.AttachmentIDs)
	if req.AttachmentIDsSet || len(req.AttachmentIDs) > 0 {
		if err := validateAttachmentOwnership(env, req.AttachmentIDs, req.AttachmentTokens, req.AttachmentTokensSet); err != nil {
			return 0, err
		}
		if !req.PreserveAttachments {
			if err := linkAttachmentsToThread(messageSystemEnv(env), attachmentIDs, req.Model, req.ResID); err != nil {
				return 0, err
			}
		}
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	body := req.Body
	if !req.BodyIsHTML {
		body = html.EscapeString(body)
	}
	values := map[string]any{
		"subject":             req.Subject,
		"body":                body,
		"message_type":        messageType,
		"model":               req.Model,
		"res_id":              req.ResID,
		"author_id":           req.AuthorID,
		"author_guest_id":     req.AuthorGuestID,
		"email_from":          req.EmailFrom,
		"message_id":          strings.TrimSpace(req.MessageID),
		"incoming_email_to":   strings.TrimSpace(req.IncomingEmailTo),
		"incoming_email_cc":   strings.TrimSpace(req.IncomingEmailCC),
		"outgoing_email_to":   strings.TrimSpace(req.OutgoingEmailTo),
		"reply_to":            strings.TrimSpace(req.ReplyTo),
		"reply_to_force_new":  req.ReplyToForceNew,
		"mail_server_id":      req.MailServerID,
		"email_layout_xmlid":  strings.TrimSpace(req.EmailLayoutXMLID),
		"email_add_signature": true,
		"date":                now.UTC(),
		"parent_id":           req.ParentID,
		"subtype_id":          subtypeID,
		"partner_ids":         partnerIDs,
		"attachment_ids":      attachmentIDs,
		"body_is_html":        req.BodyIsHTML,
	}
	if meta, ok := messageEnv.ModelMetadata("mail.message"); ok {
		systemEnv := messageSystemEnv(env)
		recordCompanyID := mailRecordCompanyID(systemEnv, req.Model, req.ResID)
		if recordCompanyID == 0 {
			recordCompanyID = env.Context().CompanyID
		}
		if strings.TrimSpace(stringAny(values["reply_to"])) == "" {
			values["reply_to"] = defaultMessageReplyTo(env, systemEnv, recordCompanyID, req.EmailFrom)
		}
		if _, ok := meta.Fields["record_company_id"]; ok && recordCompanyID != 0 {
			values["record_company_id"] = recordCompanyID
		}
		if _, ok := meta.Fields["record_alias_domain_id"]; ok {
			aliasDomainID := companyAliasDomainID(systemEnv, recordCompanyID)
			if aliasDomainID == 0 && env.Context().CompanyID != recordCompanyID {
				aliasDomainID = companyAliasDomainID(systemEnv, env.Context().CompanyID)
			}
			if aliasDomainID == 0 {
				aliasDomainID = firstAliasDomainID(systemEnv)
			}
			if aliasDomainID != 0 {
				values["record_alias_domain_id"] = aliasDomainID
			}
		}
		if _, ok := meta.Fields["mail_activity_type_id"]; ok {
			values["mail_activity_type_id"] = req.MailActivityTypeID
		}
	}
	messageID, err := messageEnv.Model("mail.message").Create(values)
	if err != nil {
		return 0, err
	}
	trackingIDs, err := createTrackingValues(messageEnv, messageID, req.TrackingValues)
	if err != nil {
		cleanupMessagePost(messageEnv, messageID, nil)
		return 0, err
	}
	if len(trackingIDs) > 0 {
		if err := messageEnv.Model("mail.message").Browse(messageID).Write(map[string]any{"tracking_value_ids": trackingIDs}); err != nil {
			cleanupMessagePost(messageEnv, messageID, trackingIDs)
			return 0, err
		}
	}
	if err := createNotifications(messageEnv, messageID, uniqueIDs(append(partnerIDs, followerPartnerIDs(messageEnv, req.Model, req.ResID)...))); err != nil {
		cleanupMessagePost(messageEnv, messageID, trackingIDs)
		return 0, err
	}
	return messageID, nil
}

func defaultMessageReplyTo(env *record.Env, systemEnv *record.Env, companyID int64, fallback string) string {
	if systemEnv == nil {
		systemEnv = messageSystemEnv(env)
	}
	if value := companyAliasDomainAddress(systemEnv, companyID, "catchall_email", "catchall_alias"); value != "" {
		return value
	}
	if env != nil && env.Context().CompanyID != companyID {
		if value := companyAliasDomainAddress(systemEnv, env.Context().CompanyID, "catchall_email", "catchall_alias"); value != "" {
			return value
		}
	}
	catchallDomain := strings.TrimSpace(configParameter(systemEnv, "mail.catchall.domain"))
	if value := configEmailValue(configParameter(systemEnv, "mail.catchall.alias"), catchallDomain); value != "" {
		return value
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return defaultNotificationEmail(env)
}

func UpdateMessageContent(env *record.Env, req MessageContentUpdateRequest) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail message update requires env")
	}
	if req.MessageID == 0 {
		return nil, fmt.Errorf("mail message update requires message id")
	}
	if req.AccessToken != "" || req.AccessHash != "" || req.AccessPID != 0 || req.ProjectSharingID != 0 {
		env = PortalMessageContextEnvWithAccess(env, req.MessageID, PortalAccessRequest{
			AccessToken:      req.AccessToken,
			AccessHash:       req.AccessHash,
			AccessPID:        req.AccessPID,
			ProjectSharingID: req.ProjectSharingID,
		})
	}
	systemEnv := messageSystemEnv(env)
	messageSet := systemEnv.Model("mail.message").Browse(req.MessageID)
	rows, err := messageSet.Read("body", "message_type", "model", "res_id", "author_id", "author_guest_id", "tracking_value_ids", "attachment_ids", "partner_ids")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mail.message:%d not found", req.MessageID)
	}
	row := rows[0]
	if !canAccessMessageForContentUpdate(env, row) {
		return nil, fmt.Errorf("mail message update forbidden")
	}
	if !canEditMessageContent(env, row) {
		return nil, fmt.Errorf("not allowed to edit message")
	}
	isDiscussChannel := strings.TrimSpace(stringFromAny(row["model"])) == "discuss.channel"
	if !isDiscussChannel && len(int64SliceFromAny(row["tracking_value_ids"])) > 0 {
		return nil, fmt.Errorf("messages with tracking values cannot be modified")
	}
	if strings.TrimSpace(stringFromAny(row["message_type"])) != "comment" {
		if isDiscussChannel {
			return nil, fmt.Errorf("only messages type comment can have their content updated on model 'discuss.channel'")
		}
		return nil, fmt.Errorf("only messages type comment can have their content updated")
	}
	if req.AttachmentIDsSet {
		if err := validateAttachmentOwnership(env, req.AttachmentIDs, req.AttachmentTokens, req.AttachmentTokensSet); err != nil {
			return nil, err
		}
	}
	values := map[string]any{}
	if req.BodySet {
		values["body"] = editedBody(req.Body)
	}
	if req.AttachmentIDsSet {
		attachmentIDs := uniqueIDs(req.AttachmentIDs)
		if len(attachmentIDs) == 0 {
			if err := unlinkMessageAttachments(systemEnv, int64SliceFromAny(row["attachment_ids"])); err != nil {
				return nil, err
			}
			values["attachment_ids"] = []int64{}
		} else {
			if err := linkAttachmentsToThread(systemEnv, attachmentIDs, stringFromAny(row["model"]), int64FromAny(row["res_id"])); err != nil {
				return nil, err
			}
			values["attachment_ids"] = uniqueIDs(append(int64SliceFromAny(row["attachment_ids"]), attachmentIDs...))
		}
	}
	if req.PartnerIDsSet {
		if isDiscussChannel {
			if len(req.PartnerIDs) > 0 {
				partnerIDs, err := allowedDiscussMessagePartnerIDs(systemEnv, int64FromAny(row["res_id"]), req.PartnerIDs)
				if err != nil {
					return nil, err
				}
				values["partner_ids"] = partnerIDs
			}
		} else {
			values["partner_ids"] = uniqueIDs(req.PartnerIDs)
		}
	}
	if len(values) > 0 {
		if err := messageSet.Write(values); err != nil {
			return nil, err
		}
	}
	updated, err := messageSet.Read("body", "message_type", "model", "res_id", "author_id", "author_guest_id", "email_from", "date", "parent_id", "subtype_id", "partner_ids", "attachment_ids", "body_is_html", "tracking_value_ids")
	if err != nil {
		return nil, err
	}
	if _, err := attachTrackingValues(systemEnv, updated); err != nil {
		return nil, err
	}
	return map[string]any{"mail.message": updated}, nil
}

func canAccessMessageForContentUpdate(env *record.Env, row map[string]any) bool {
	if env == nil || env.Context().UserID == 1 || env.Policy() == nil {
		return true
	}
	policy := env.Policy()
	ctx := env.Context()
	if policy.Check(ctx, "mail.message", record.OpCreate, nil) == nil {
		allowed, err := policy.CheckRecord(ctx, "mail.message", record.OpCreate, row)
		if err == nil && allowed {
			return true
		}
	}
	modelName := strings.TrimSpace(stringFromAny(row["model"]))
	resID := int64FromAny(row["res_id"])
	if modelName == "" || resID == 0 {
		return false
	}
	if portalContextMatches(env, modelName, resID) {
		return true
	}
	operation := record.OpWrite
	if modelName == "discuss.channel" {
		if canAccessDiscussChannel(env, resID) {
			return true
		}
		operation = record.OpRead
	}
	if policy.Check(ctx, modelName, operation, nil) != nil {
		return false
	}
	allowed, err := policy.CheckRecord(ctx, modelName, operation, policyRow(messageSystemEnv(env), modelName, resID))
	return err == nil && allowed
}

func canEditMessageContent(env *record.Env, row map[string]any) bool {
	if env == nil {
		return false
	}
	ctx := env.Context()
	if ctx.UserID == 1 {
		return true
	}
	if userHasAdminGroup(env, ctx.UserID) {
		return true
	}
	if ctx.UserID == 0 {
		portalPartnerID := currentPortalPartnerID(env)
		if portalPartnerID != 0 && portalPartnerID == int64FromAny(row["author_id"]) {
			return true
		}
	}
	if guestID := currentGuestID(env); guestID != 0 && guestID == int64FromAny(row["author_guest_id"]) {
		return true
	}
	authorID := int64FromAny(row["author_id"])
	return authorID != 0 && authorID == currentUserPartnerID(env, ctx.UserID)
}

func currentGuestID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	if guestID := int64FromAny(env.Context().Values["guest_id"]); guestID != 0 {
		return guestID
	}
	return int64FromAny(env.Context().Values["mail_guest_id"])
}

const (
	portalContextModelKey     = "mail_portal_model"
	portalContextResIDKey     = "mail_portal_res_id"
	portalContextPartnerIDKey = "mail_portal_partner_id"
)

type PortalAccessRequest struct {
	AccessToken      string
	AccessHash       string
	AccessPID        int64
	ProjectSharingID int64
}

func PortalContextEnv(env *record.Env, modelName string, resID int64, accessToken string, accessHash string, accessPID int64) *record.Env {
	return PortalContextEnvWithAccess(env, modelName, resID, PortalAccessRequest{
		AccessToken: accessToken,
		AccessHash:  accessHash,
		AccessPID:   accessPID,
	})
}

func PortalContextEnvWithAccess(env *record.Env, modelName string, resID int64, access PortalAccessRequest) *record.Env {
	if env == nil {
		return env
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || resID == 0 {
		return env
	}
	access = normalizedPortalAccessRequest(env, modelName, resID, access)
	partnerID := portalPartnerID(env, modelName, resID, access.AccessHash, access.AccessPID, access.AccessToken)
	if partnerID == 0 {
		return env
	}
	ctx := env.Context()
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values[portalContextModelKey] = modelName
	ctx.Values[portalContextResIDKey] = resID
	ctx.Values[portalContextPartnerIDKey] = partnerID
	return env.WithContext(ctx)
}

func PortalMessageContextEnv(env *record.Env, messageID int64, accessToken string, accessHash string, accessPID int64) *record.Env {
	return PortalMessageContextEnvWithAccess(env, messageID, PortalAccessRequest{
		AccessToken: accessToken,
		AccessHash:  accessHash,
		AccessPID:   accessPID,
	})
}

func PortalMessageContextEnvWithAccess(env *record.Env, messageID int64, access PortalAccessRequest) *record.Env {
	if env == nil || messageID == 0 {
		return env
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(messageID).Read("model", "res_id")
	if err != nil || len(rows) == 0 {
		return env
	}
	return PortalContextEnvWithAccess(env, stringFromAny(rows[0]["model"]), int64FromAny(rows[0]["res_id"]), access)
}

func PortalPartnerID(env *record.Env) int64 {
	return currentPortalPartnerID(env)
}

func ThreadAccessible(env *record.Env, modelName string, resID int64) bool {
	return ensureRecordExists(env, modelName, resID) == nil
}

func normalizedPortalAccessRequest(env *record.Env, modelName string, resID int64, access PortalAccessRequest) PortalAccessRequest {
	access.AccessToken = strings.TrimSpace(access.AccessToken)
	access.AccessHash = strings.TrimSpace(access.AccessHash)
	if access.ProjectSharingID != 0 {
		token := projectSharingTaskToken(env, modelName, resID, access.ProjectSharingID, access.AccessToken)
		if token == "" {
			access.AccessToken = ""
			access.AccessHash = ""
			access.AccessPID = 0
			return access
		}
		access.AccessToken = token
	}
	return access
}

func projectSharingTaskToken(env *record.Env, modelName string, resID int64, projectID int64, projectToken string) string {
	if strings.TrimSpace(modelName) != "project.task" || resID == 0 || projectID == 0 || strings.TrimSpace(projectToken) == "" {
		return ""
	}
	if !portalTokenValid(env, "project.project", projectID, projectToken) {
		return ""
	}
	rows, err := messageSystemEnv(env).Model("project.task").Browse(resID).Read("project_id", "access_token")
	if err != nil || len(rows) == 0 || int64FromAny(rows[0]["project_id"]) != projectID {
		return ""
	}
	return strings.TrimSpace(stringFromAny(rows[0]["access_token"]))
}

func portalPartnerID(env *record.Env, modelName string, resID int64, accessHash string, accessPID int64, accessToken string) int64 {
	if portalHashPIDValid(env, modelName, resID, accessHash, accessPID) {
		return accessPID
	}
	if portalTokenValid(env, modelName, resID, accessToken) {
		partnerIDs := portalThreadPartnerIDs(env, modelName, resID)
		if len(partnerIDs) > 0 {
			return partnerIDs[0]
		}
	}
	return 0
}

func portalHashPIDValid(env *record.Env, modelName string, resID int64, accessHash string, accessPID int64) bool {
	if accessHash == "" || accessPID == 0 {
		return false
	}
	token := portalThreadAccessToken(env, modelName, resID)
	if token != "" {
		expected := portalAccessHash(env, token, accessPID)
		if expected != "" && subtle.ConstantTimeCompare([]byte(accessHash), []byte(expected)) == 1 {
			return true
		}
	}
	parentToken := portalParentAccessToken(env, modelName, resID)
	if parentToken == "" {
		return false
	}
	expected := portalAccessHash(env, parentToken, accessPID)
	return expected != "" && subtle.ConstantTimeCompare([]byte(accessHash), []byte(expected)) == 1
}

func portalTokenValid(env *record.Env, modelName string, resID int64, accessToken string) bool {
	if accessToken == "" {
		return false
	}
	expected := portalThreadAccessToken(env, modelName, resID)
	return expected != "" && subtle.ConstantTimeCompare([]byte(accessToken), []byte(expected)) == 1
}

func portalAccessHash(env *record.Env, accessToken string, partnerID int64) string {
	secret := configParameter(messageSystemEnv(env), "database.secret")
	if secret == "" || accessToken == "" || partnerID == 0 {
		return ""
	}
	payload := fmt.Sprintf("(%s, %s, %d)", pythonReprString(portalDBName(env)), pythonReprString(accessToken), partnerID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func portalDBName(env *record.Env) string {
	if env == nil {
		return "gorp"
	}
	if db := strings.TrimSpace(stringFromAny(env.Context().Values["db"])); db != "" {
		return db
	}
	return "gorp"
}

func pythonReprString(value string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func portalThreadAccessToken(env *record.Env, modelName string, resID int64) string {
	systemEnv := messageSystemEnv(env)
	if !modelHasField(systemEnv, modelName, "access_token") {
		return ""
	}
	rows, err := systemEnv.Model(modelName).Browse(resID).Read("access_token")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(rows[0]["access_token"]))
}

func portalParentAccessToken(env *record.Env, modelName string, resID int64) string {
	if token, handled := portalModelSpecificParentAccessToken(env, modelName, resID); handled {
		return token
	}
	systemEnv := messageSystemEnv(env)
	if !modelHasField(systemEnv, modelName, "parent_id") {
		return ""
	}
	rows, err := systemEnv.Model(modelName).Browse(resID).Read("parent_id")
	if err != nil || len(rows) == 0 {
		return ""
	}
	parentID := int64FromAny(rows[0]["parent_id"])
	if parentID == 0 {
		return ""
	}
	return portalThreadAccessToken(env, modelName, parentID)
}

func portalModelSpecificParentAccessToken(env *record.Env, modelName string, resID int64) (string, bool) {
	switch strings.TrimSpace(modelName) {
	case "project.task":
		return portalRelatedParentAccessToken(env, modelName, resID, "project_id", "project.project"), true
	default:
		return "", false
	}
}

func portalRelatedParentAccessToken(env *record.Env, modelName string, resID int64, fieldName string, expectedParentModel string) string {
	systemEnv := messageSystemEnv(env)
	modelMeta, ok := systemEnv.ModelMetadata(modelName)
	if !ok {
		return ""
	}
	fieldMeta, ok := modelMeta.Fields[fieldName]
	if !ok {
		return ""
	}
	parentModel := strings.TrimSpace(fieldMeta.Relation)
	if parentModel == "" || (expectedParentModel != "" && parentModel != expectedParentModel) {
		return ""
	}
	rows, err := systemEnv.Model(modelName).Browse(resID).Read(fieldName)
	if err != nil || len(rows) == 0 {
		return ""
	}
	parentID := int64FromAny(rows[0][fieldName])
	if parentID == 0 {
		return ""
	}
	return portalThreadAccessToken(env, parentModel, parentID)
}

func portalThreadPartnerIDs(env *record.Env, modelName string, resID int64) []int64 {
	systemEnv := messageSystemEnv(env)
	if modelHasField(systemEnv, modelName, "partner_id") {
		rows, err := systemEnv.Model(modelName).Browse(resID).Read("partner_id")
		if err == nil && len(rows) > 0 {
			if partnerID := int64FromAny(rows[0]["partner_id"]); partnerID != 0 {
				return []int64{partnerID}
			}
		}
	}
	return followerPartnerIDs(systemEnv, modelName, resID)
}

func modelHasField(env *record.Env, modelName string, fieldName string) bool {
	if env == nil || strings.TrimSpace(modelName) == "" || strings.TrimSpace(fieldName) == "" {
		return false
	}
	fields, err := env.Model(modelName).FieldsGet([]string{fieldName}, nil)
	if err != nil {
		return false
	}
	_, ok := fields[fieldName]
	return ok
}

func currentPortalPartnerID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	return int64FromAny(env.Context().Values[portalContextPartnerIDKey])
}

func portalContextMatches(env *record.Env, modelName string, resID int64) bool {
	if env == nil {
		return false
	}
	ctx := env.Context()
	return strings.TrimSpace(stringFromAny(ctx.Values[portalContextModelKey])) == strings.TrimSpace(modelName) && int64FromAny(ctx.Values[portalContextResIDKey]) == resID && int64FromAny(ctx.Values[portalContextPartnerIDKey]) != 0
}

func cloneContextValues(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func canAccessDiscussChannel(env *record.Env, channelID int64) bool {
	if env == nil || channelID == 0 {
		return false
	}
	if env.Context().UserID == 1 {
		return true
	}
	rows, err := messageSystemEnv(env).Model("discuss.channel").Browse(channelID).Read("channel_type", "group_public_id")
	if err != nil || len(rows) == 0 {
		return false
	}
	channelType := strings.TrimSpace(stringFromAny(rows[0]["channel_type"]))
	groupID := int64FromAny(rows[0]["group_public_id"])
	if channelType == "channel" {
		return groupID == 0 || userHasGroupID(env, env.Context().UserID, groupID)
	}
	return isDiscussChannelMember(env, channelID)
}

func isDiscussChannelMember(env *record.Env, channelID int64) bool {
	systemEnv := messageSystemEnv(env)
	if guestID := currentGuestID(env); guestID != 0 {
		found, err := systemEnv.Model("discuss.channel.member").Search(domain.And(
			domain.Cond("channel_id", "=", channelID),
			domain.Cond("guest_id", "=", guestID),
		))
		if err == nil && found.Len() > 0 {
			return true
		}
	}
	userID := env.Context().UserID
	if userID == 0 {
		return false
	}
	found, err := systemEnv.Model("discuss.channel.member").Search(domain.And(
		domain.Cond("channel_id", "=", channelID),
		domain.Cond("user_id", "=", userID),
	))
	if err == nil && found.Len() > 0 {
		return true
	}
	partnerID := currentUserPartnerID(env, userID)
	if partnerID == 0 {
		return false
	}
	found, err = systemEnv.Model("discuss.channel.member").Search(domain.And(
		domain.Cond("channel_id", "=", channelID),
		domain.Cond("partner_id", "=", partnerID),
	))
	return err == nil && found.Len() > 0
}

func allowedDiscussMessagePartnerIDs(env *record.Env, channelID int64, partnerIDs []int64) ([]int64, error) {
	partnerIDs = uniqueIDs(partnerIDs)
	if len(partnerIDs) == 0 {
		return nil, nil
	}
	rows, err := env.Model("discuss.channel").Browse(channelID).Read("channel_type", "group_public_id")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("discuss.channel:%d not found", channelID)
	}
	if strings.TrimSpace(stringFromAny(rows[0]["channel_type"])) == "channel" {
		groupID := int64FromAny(rows[0]["group_public_id"])
		if groupID == 0 {
			return partnerIDs, nil
		}
		return partnersWithGroup(env, partnerIDs, groupID), nil
	}
	return channelMemberPartnerIDs(env, channelID, partnerIDs), nil
}

func partnersWithGroup(env *record.Env, partnerIDs []int64, groupID int64) []int64 {
	found, err := env.Model("res.users").Search(domain.Cond("partner_id", "in", partnerIDs))
	if err != nil {
		return nil
	}
	rows, err := found.Read("id", "partner_id", "groups_id")
	if err != nil {
		return nil
	}
	allowed := map[int64]bool{}
	for _, row := range rows {
		userID := int64FromAny(row["id"])
		partnerID := int64FromAny(row["partner_id"])
		if partnerID == 0 {
			continue
		}
		if policy, ok := env.Policy().(interface {
			EffectiveGroupIDs(userID int64) map[int64]bool
		}); ok {
			if policy.EffectiveGroupIDs(userID)[groupID] {
				allowed[partnerID] = true
			}
			continue
		}
		if containsInt64(int64SliceFromAny(row["groups_id"]), groupID) {
			allowed[partnerID] = true
		}
	}
	return filterIDs(partnerIDs, allowed)
}

func channelMemberPartnerIDs(env *record.Env, channelID int64, partnerIDs []int64) []int64 {
	found, err := env.Model("discuss.channel.member").Search(domain.And(
		domain.Cond("channel_id", "=", channelID),
		domain.Cond("partner_id", "in", partnerIDs),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("partner_id")
	if err != nil {
		return nil
	}
	allowed := map[int64]bool{}
	for _, row := range rows {
		if partnerID := int64FromAny(row["partner_id"]); partnerID != 0 {
			allowed[partnerID] = true
		}
	}
	return filterIDs(partnerIDs, allowed)
}

func filterIDs(ids []int64, allowed map[int64]bool) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

func validateAttachmentOwnership(env *record.Env, attachmentIDs []int64, tokens []string, tokensSet bool) error {
	rawAttachmentIDs := append([]int64(nil), attachmentIDs...)
	if tokensSet && len(tokens) > 0 && len(tokens) != len(rawAttachmentIDs) {
		return fmt.Errorf("an access token must be provided for each attachment")
	}
	if len(rawAttachmentIDs) == 0 {
		return nil
	}
	if len(tokens) == 0 {
		tokens = make([]string, len(rawAttachmentIDs))
	}
	systemEnv := messageSystemEnv(env)
	rows, err := systemEnv.Model("ir.attachment").Browse(uniqueIDs(rawAttachmentIDs)...).Read("id", "res_model", "res_id")
	if err != nil {
		return err
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[int64FromAny(row["id"])] = row
	}
	for idx, attachmentID := range rawAttachmentIDs {
		row, ok := byID[attachmentID]
		if !ok {
			return fmt.Errorf("one or more attachments do not exist, or you do not have the rights to access them")
		}
		if canWriteAttachment(env, row) {
			continue
		}
		if idx >= len(tokens) || !verifyLimitedFieldAccessToken(systemEnv, "ir.attachment", attachmentID, "id", tokens[idx], "attachment_ownership") {
			return fmt.Errorf("one or more attachments do not exist, or you do not have the rights to access them")
		}
	}
	return nil
}

func CheckAttachmentOwnership(env *record.Env, attachmentID int64, accessToken string) error {
	return validateAttachmentOwnership(env, []int64{attachmentID}, []string{accessToken}, true)
}

func canWriteAttachment(env *record.Env, row map[string]any) bool {
	if env == nil || env.Context().UserID == 1 || env.Policy() == nil {
		return true
	}
	policy := env.Policy()
	ctx := env.Context()
	if policy.Check(ctx, "ir.attachment", record.OpWrite, nil) != nil {
		return false
	}
	allowed, err := policy.CheckRecord(ctx, "ir.attachment", record.OpWrite, row)
	return err == nil && allowed
}

func linkAttachmentsToThread(env *record.Env, attachmentIDs []int64, modelName string, resID int64) error {
	if len(attachmentIDs) == 0 || strings.TrimSpace(modelName) == "" || resID == 0 {
		return nil
	}
	rows, err := env.Model("ir.attachment").Browse(attachmentIDs...).Read("id", "res_model", "res_id")
	if err != nil {
		return err
	}
	for _, row := range rows {
		currentModel := strings.TrimSpace(stringFromAny(row["res_model"]))
		currentResID := int64FromAny(row["res_id"])
		if currentModel != "" && currentModel != "mail.compose.message" && currentModel != "mail.scheduled.message" && currentResID != 0 {
			continue
		}
		if err := env.Model("ir.attachment").Browse(int64FromAny(row["id"])).Write(map[string]any{"res_model": modelName, "res_id": resID}); err != nil {
			return err
		}
	}
	return nil
}

func unlinkMessageAttachments(env *record.Env, attachmentIDs []int64) error {
	attachmentIDs = uniqueIDs(attachmentIDs)
	if len(attachmentIDs) == 0 {
		return nil
	}
	return env.Model("ir.attachment").Browse(attachmentIDs...).Unlink()
}

func verifyLimitedFieldAccessToken(env *record.Env, modelName string, id int64, fieldName string, accessToken string, scope string) bool {
	if accessToken == "" {
		return false
	}
	tokenPart, timestamp, ok := strings.Cut(accessToken, "o")
	if !ok || tokenPart == "" || timestamp == "" {
		return false
	}
	expires, err := strconv.ParseInt(strings.TrimPrefix(timestamp, "0x"), 16, 64)
	if err != nil || time.Now().Unix() >= expires {
		return false
	}
	expected := limitedFieldAccessToken(env, modelName, id, fieldName, timestamp, scope)
	return subtle.ConstantTimeCompare([]byte(accessToken), []byte(expected)) == 1
}

func AttachmentOwnershipToken(env *record.Env, attachmentID int64) string {
	return attachmentLimitedFieldAccessToken(env, attachmentID, "id", "attachment_ownership")
}

func AttachmentRawAccessToken(env *record.Env, attachmentID int64) string {
	return attachmentLimitedFieldAccessToken(env, attachmentID, "raw", "binary")
}

func AttachmentThumbnailAccessToken(env *record.Env, attachmentID int64) string {
	return attachmentLimitedFieldAccessToken(env, attachmentID, "thumbnail", "binary")
}

func attachmentLimitedFieldAccessToken(env *record.Env, attachmentID int64, fieldName string, scope string) string {
	if attachmentID == 0 {
		return ""
	}
	timestamp := limitedFieldAccessTimestamp("ir.attachment", attachmentID, fieldName, time.Now())
	return limitedFieldAccessToken(messageSystemEnv(env), "ir.attachment", attachmentID, fieldName, timestamp, scope)
}

func limitedFieldAccessTimestamp(modelName string, id int64, fieldName string, now time.Time) string {
	period := int64(14 * 24 * 60 * 60)
	start := now.Unix() / period * period
	unique := fmt.Sprintf("('%s', %d, '%s')", modelName, id, fieldName)
	jitter := period * int64(adler32.Checksum([]byte(unique))) / 4294967295
	return fmt.Sprintf("0x%x", start+2*period+jitter)
}

func limitedFieldAccessToken(env *record.Env, modelName string, id int64, fieldName string, timestamp string, scope string) string {
	message := fmt.Sprintf("('%s', %d, '%s', '%s')", modelName, id, fieldName, timestamp)
	payload := fmt.Sprintf("('%s', %s)", scope, message)
	mac := hmac.New(sha256.New, []byte(configParameter(env, "database.secret")))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil)) + "o" + timestamp
}

func configParameter(env *record.Env, key string) string {
	if env == nil {
		return ""
	}
	found, err := env.Model("ir.config_parameter").Search(domain.Cond("key", "=", key))
	if err != nil {
		return ""
	}
	rows, err := found.Read("value")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringFromAny(rows[0]["value"])
}

func userHasAdminGroup(env *record.Env, userID int64) bool {
	adminGroupID := resolveXMLID(messageSystemEnv(env), "base.group_erp_manager", "res.groups")
	if adminGroupID == 0 {
		return false
	}
	return userHasGroupID(env, userID, adminGroupID)
}

func userHasGroupID(env *record.Env, userID int64, groupID int64) bool {
	if env == nil || userID == 0 || groupID == 0 {
		return false
	}
	if policy, ok := env.Policy().(interface {
		EffectiveGroupIDs(userID int64) map[int64]bool
	}); ok {
		return policy.EffectiveGroupIDs(userID)[groupID]
	}
	return containsInt64(messageEditGroupIDs(messageSystemEnv(env), userID), groupID)
}

func messageEditGroupIDs(env *record.Env, userID int64) []int64 {
	ids := int64SliceFromAny(env.Context().Values["group_ids"])
	if userID == 0 {
		return uniqueIDs(ids)
	}
	rows, err := env.Model("res.users").Browse(userID).Read("groups_id")
	if err != nil || len(rows) == 0 {
		return uniqueIDs(ids)
	}
	return uniqueIDs(append(ids, int64SliceFromAny(rows[0]["groups_id"])...))
}

func currentUserPartnerID(env *record.Env, userID int64) int64 {
	if userID == 0 {
		return 0
	}
	rows, err := messageSystemEnv(env).Model("res.users").Browse(userID).Read("partner_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["partner_id"])
}

func messageSystemEnv(env *record.Env) *record.Env {
	ctx := env.Context()
	ctx.UserID = 1
	return env.WithContext(ctx)
}

func policyRow(env *record.Env, modelName string, id int64) map[string]any {
	if env == nil || modelName == "" || id == 0 {
		return map[string]any{"id": id}
	}
	fields, err := env.Model(modelName).FieldsGet(nil, nil)
	if err != nil {
		return map[string]any{"id": id}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		if name == "id" || name == "display_name" {
			continue
		}
		names = append(names, name)
	}
	rows, err := env.Model(modelName).Browse(id).Read(names...)
	if err != nil || len(rows) == 0 {
		return map[string]any{"id": id}
	}
	return rows[0]
}

func Subscribe(env *record.Env, modelName string, resID int64, partnerIDs []int64, subtypeIDs []int64) error {
	if env == nil {
		return fmt.Errorf("mail subscribe requires env")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || resID == 0 {
		return fmt.Errorf("mail subscribe requires model and record id")
	}
	for _, partnerID := range uniqueIDs(partnerIDs) {
		if partnerID == 0 || followerExists(env, modelName, resID, partnerID) {
			continue
		}
		if _, err := env.Model("mail.followers").Create(map[string]any{
			"res_model":   modelName,
			"res_id":      resID,
			"partner_id":  partnerID,
			"subtype_ids": uniqueIDs(subtypeIDs),
		}); err != nil {
			return err
		}
	}
	return nil
}

func Unsubscribe(env *record.Env, modelName string, resID int64, partnerIDs []int64) error {
	if env == nil {
		return fmt.Errorf("mail unsubscribe requires env")
	}
	found, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", resID),
		domain.Cond("partner_id", "in", uniqueIDs(partnerIDs)),
	))
	if err != nil {
		return err
	}
	return found.Unlink()
}

func ensureRecordExists(env *record.Env, modelName string, id int64) error {
	rows, err := env.Model(modelName).Browse(id).Read("id")
	if (err != nil || len(rows) == 0) && portalContextMatches(env, modelName, id) {
		systemRows, systemErr := messageSystemEnv(env).Model(modelName).Browse(id).Read("id")
		if systemErr == nil && len(systemRows) > 0 {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%s:%d not found", modelName, id)
	}
	return nil
}

func resolveSubtypeXMLID(env *record.Env, xmlID string) int64 {
	return resolveXMLID(env, xmlID, "mail.message.subtype")
}

func resolveXMLID(env *record.Env, xmlID string, modelName string) int64 {
	module, name := splitXMLID(xmlID)
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", "=", module),
		domain.Cond("name", "=", name),
		domain.Cond("model", "=", modelName),
	))
	if err != nil {
		return 0
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["res_id"])
}

func followerPartnerIDs(env *record.Env, modelName string, resID int64) []int64 {
	found, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", resID),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("partner_id")
	if err != nil {
		return nil
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, int64FromAny(row["partner_id"]))
	}
	return uniqueIDs(out)
}

func followerExists(env *record.Env, modelName string, resID int64, partnerID int64) bool {
	found, err := env.Model("mail.followers").Search(domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", resID),
		domain.Cond("partner_id", "=", partnerID),
	))
	return err == nil && found.Len() > 0
}

func createNotifications(env *record.Env, messageID int64, partnerIDs []int64) error {
	for _, partnerID := range partnerIDs {
		if partnerID == 0 {
			continue
		}
		if _, err := env.Model("mail.notification").Create(map[string]any{
			"mail_message_id":     messageID,
			"res_partner_id":      partnerID,
			"notification_type":   "inbox",
			"notification_status": "ready",
		}); err != nil {
			return err
		}
	}
	return nil
}

func createTrackingValues(env *record.Env, messageID int64, values []TrackingValue) ([]int64, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		row := map[string]any{
			"field_id":           value.FieldID,
			"field_info":         value.FieldInfo,
			"field_name":         value.FieldName,
			"field_desc":         value.FieldDesc,
			"field_type":         value.FieldType,
			"old_value_integer":  value.OldValueInteger,
			"old_value_float":    value.OldValueFloat,
			"old_value_char":     value.OldValueChar,
			"old_value_text":     value.OldValueText,
			"old_value_datetime": value.OldValueDatetime,
			"new_value_integer":  value.NewValueInteger,
			"new_value_float":    value.NewValueFloat,
			"new_value_char":     value.NewValueChar,
			"new_value_text":     value.NewValueText,
			"new_value_datetime": value.NewValueDatetime,
			"currency_id":        value.CurrencyID,
			"mail_message_id":    messageID,
		}
		id, err := env.Model("mail.tracking.value").Create(row)
		if err != nil {
			cleanupMessagePost(env, 0, ids)
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func cleanupMessagePost(env *record.Env, messageID int64, trackingIDs []int64) {
	if env == nil {
		return
	}
	if len(trackingIDs) > 0 {
		_ = env.Model("mail.tracking.value").Browse(trackingIDs...).Unlink()
	}
	if messageID != 0 {
		if notifications, err := env.Model("mail.notification").Search(domain.Cond("mail_message_id", "=", messageID)); err == nil {
			_ = notifications.Unlink()
		}
		_ = env.Model("mail.message").Browse(messageID).Unlink()
	}
}

func splitXMLID(xmlID string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(xmlID), ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "base", strings.TrimSpace(xmlID)
}

func uniqueIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func editedBody(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return body + `<span class="o-mail-Message-edited"></span>`
}

func int64SliceFromAny(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id := int64FromAny(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	case int, int64, int32, float64:
		if id := int64FromAny(typed); id != 0 {
			return []int64{id}
		}
	default:
		return nil
	}
	return nil
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
