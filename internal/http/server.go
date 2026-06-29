package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	coreaccounting "gorp/internal/accounting"
	serveractions "gorp/internal/actions"
	aicontrollers "gorp/internal/ai/controllers"
	aiproviders "gorp/internal/ai/providers"
	"gorp/internal/assets"
	"gorp/internal/data"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/impersonation"
	internalmail "gorp/internal/mail"
	"gorp/internal/meta/action"
	"gorp/internal/meta/menu"
	"gorp/internal/meta/view"
	"gorp/internal/model"
	"gorp/internal/module"
	modulelifecycle "gorp/internal/module/lifecycle"
	"gorp/internal/notifications"
	"gorp/internal/phone"
	"gorp/internal/record"
	"gorp/internal/security"
	"gorp/internal/sequences"
	internalworkflow "gorp/internal/workflow"
)

type Server struct {
	Env           *record.Env
	Assets        *assets.Registry
	Actions       *action.Registry
	ServerActions *serveractions.Registry
	Menus         *menu.Registry
	Views         *view.Registry
	Modules       map[string]module.Manifest
	ExternalIDs   map[string]data.ExternalID
	Security      *security.Engine
	FrontendDist  string

	Impersonation       *impersonation.Service
	Bus                 *notifications.Bus
	Workflow            *internalworkflow.Dispatcher
	MailSender          internalmail.Sender
	FetchmailConnector  internalmail.FetchmailConnector
	InboundMessageLock  internalmail.InboundMessageIDLocker
	FetchmailServerLock internalmail.FetchmailServerLocker
	AIChat              *aicontrollers.ChatService
	AIChatFactory       func(*record.Env) *aicontrollers.ChatService
	AITranscript        *aicontrollers.TranscriptionService
	ModuleLifecycleHook func(*record.Env, modulelifecycle.Result) error
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/web", s.webClient)
	mux.HandleFunc("/web/", s.webClient)
	mux.HandleFunc("/odoo", s.webClient)
	mux.HandleFunc("/odoo/", s.webClient)
	mux.HandleFunc("/web/health", s.health)
	mux.HandleFunc("/web/session/info", s.sessionInfo)
	mux.HandleFunc("/web/session/get_session_info", s.sessionInfo)
	mux.HandleFunc("/web/session/authenticate", s.authenticate)
	mux.HandleFunc("/web/session/check", s.sessionCheck)
	mux.HandleFunc("/web/session/switch_company", s.switchCompany)
	mux.HandleFunc("/web/session/modules", s.sessionModules)
	mux.HandleFunc("/web/session/destroy", s.sessionDestroy)
	mux.HandleFunc("/web/session/logout", s.logout)
	mux.HandleFunc("/web/static/frontend/", s.frontendAsset)
	mux.HandleFunc("/web_enterprise/static/img/background-dark.jpg", s.cleanRoomEnterpriseBackground)
	mux.HandleFunc("/web_enterprise/static/img/background-cleanroom.svg", s.cleanRoomEnterpriseBackground)
	mux.HandleFunc("/web/login_as/", s.loginAs)
	mux.HandleFunc("/web/login_back", s.loginBack)
	mux.HandleFunc("/web/become/debug", s.loginAsDebug)
	mux.HandleFunc("/web/dataset/call_kw", s.callKW)
	mux.HandleFunc("/web/dataset/call_kw/", s.callKW)
	mux.HandleFunc("/web/dataset/call_button", s.callButton)
	mux.HandleFunc("/web/dataset/call_button/", s.callButton)
	mux.HandleFunc("/web/action/load", s.actionLoad)
	mux.HandleFunc("/web/action/run", s.actionRun)
	mux.HandleFunc("/web/view/load", s.viewLoad)
	mux.HandleFunc("/web/menu/load", s.menuLoad)
	mux.HandleFunc("/web/webclient/load_menus", s.menuLoad)
	mux.HandleFunc("/web/webclient/load_menus/", s.menuLoad)
	mux.HandleFunc("/web/export/formats", s.exportFormats)
	mux.HandleFunc("/web/export/get_fields", s.exportGetFields)
	mux.HandleFunc("/web/export/namelist", s.exportNamelist)
	mux.HandleFunc("/web/export/csv", s.exportCSV)
	mux.HandleFunc("/web/export/xlsx", s.exportXLSX)
	mux.HandleFunc("/mail/message/post", s.mailMessagePost)
	mux.HandleFunc("/mail/message/update_content", s.mailMessageUpdateContent)
	mux.HandleFunc("/mail/attachment/upload", s.mailAttachmentUpload)
	mux.HandleFunc("/mail/attachment/delete", s.mailAttachmentDelete)
	mux.HandleFunc("/mail/message/reaction", s.mailMessageReaction)
	mux.HandleFunc("/mail/avatar/mail.message/", s.mailMessageAvatar)
	mux.HandleFunc("/im_livechat/cors/message/reaction", s.livechatMessageReaction)
	mux.HandleFunc("/mail/action", s.mailAction)
	mux.HandleFunc("/mail/data", s.mailData)
	mux.HandleFunc("/mail/thread/messages", s.mailThreadMessages)
	mux.HandleFunc("/mail/starred/messages", s.mailStarredMessages)
	mux.HandleFunc("/mail/chatter_fetch", s.mailChatterFetch)
	mux.HandleFunc("/portal/chatter_init", s.portalChatterInit)
	mux.HandleFunc("/mail/view", s.mailView)
	mux.HandleFunc("/mail/thread/recipients", s.mailThreadRecipients)
	mux.HandleFunc("/mail/thread/recipients/fields", s.mailThreadRecipientsFields)
	mux.HandleFunc("/mail/thread/recipients/get_suggested_recipients", s.mailThreadRecipientsGetSuggestedRecipients)
	mux.HandleFunc("/mail/read_subscription_data", s.mailReadSubscriptionData)
	mux.HandleFunc("/mail/thread/subscribe", s.mailThreadSubscribe)
	mux.HandleFunc("/mail/thread/unsubscribe", s.mailThreadUnsubscribe)
	mux.HandleFunc("/mail/partner/from_email", s.mailPartnerFromEmail)
	mux.HandleFunc("/mail/track/", s.mailTrackOpenPixel)
	mux.HandleFunc("/unsubscribe_from_list", s.mailingUnsubscribePlaceholder)
	mux.HandleFunc("/view", s.mailingViewPlaceholder)
	mux.HandleFunc("/mailing/confirm_unsubscribe", s.mailingConfirmUnsubscribePost)
	mux.HandleFunc("/mailing/report/unsubscribe", s.mailingReportDeactivate)
	mux.HandleFunc("/mailing/", s.mailingPortal)
	mux.HandleFunc("/digest/", s.digestPortal)
	mux.HandleFunc("/sms/status", s.smsStatus)
	mux.HandleFunc("/sms/", s.smsOptOutPortal)
	mux.HandleFunc("/whatsapp/webhook/", s.whatsAppWebhook)
	mux.HandleFunc("/r/", s.linkTrackerRedirect)
	mux.HandleFunc("/web/assets/debug/", s.assetDebugFile)
	mux.HandleFunc("/web/assets/manifest", s.assetManifest)
	mux.HandleFunc("/web/bundle/", s.bundle)
	mux.HandleFunc("/web/binary/", s.binary)
	mux.HandleFunc("/bus/poll", s.busPoll)
	mux.HandleFunc("/account/download_invoice_attachments/", s.downloadInvoiceAttachments)
	mux.HandleFunc("/account/download_invoice_documents/", s.downloadInvoiceDocuments)
	mux.HandleFunc("/account/download_move_attachments/", s.downloadMoveAttachments)
	mux.HandleFunc(aicontrollers.RouteGenerateResponse, s.aiGenerateResponse)
	mux.HandleFunc(aicontrollers.RouteCloseAIChat, s.aiCloseChat)
	mux.HandleFunc(aicontrollers.RouteTranscriptionSession, s.aiTranscriptionSession)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/reports/") {
			s.reportsFile(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) reportsFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/reports/")
	if rel == "" || strings.Contains(rel, "\x00") {
		http.NotFound(w, r)
		return
	}
	root, err := reportsRoot()
	if err != nil {
		http.Error(w, "reports root error", http.StatusInternalServerError)
		return
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.Clean(rel)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, target)
}

func reportsRoot() (string, error) {
	for _, candidate := range []string{"reports", filepath.Join("current", "reports"), filepath.Join("gorp", "reports")} {
		root, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root, nil
		}
	}
	return filepath.Abs("reports")
}

func (s Server) busPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Channels []string `json:"channels"`
		Last     int64    `json:"last"`
		LastID   int64    `json:"last_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	if s.Bus == nil {
		writeRPCOrJSON(w, envelope, []map[string]any{})
		return
	}
	channels := cleanBusChannels(req.Channels)
	if len(channels) == 0 && env != nil && env.Context().UserID != 0 {
		channels = append(channels, userBusChannel(env.Context().UserID))
	}
	lastID := req.LastID
	if req.Last > lastID {
		lastID = req.Last
	}
	events := make([]map[string]any, 0)
	for _, channel := range channels {
		sub := s.Bus.Subscribe(channel, lastID)
		for {
			select {
			case event := <-sub.Events:
				events = append(events, busEventPayload(event))
			default:
				sub.Close()
				goto nextChannel
			}
		}
	nextChannel:
	}
	writeRPCOrJSON(w, envelope, events)
}

func (s Server) mailMessagePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel      string         `json:"thread_model"`
		ThreadID         int64          `json:"thread_id"`
		PostData         map[string]any `json:"post_data"`
		Context          map[string]any `json:"context"`
		Token            any            `json:"token"`
		Hash             any            `json:"hash"`
		PID              any            `json:"pid"`
		ProjectSharingID any            `json:"project_sharing_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env := requestContextEnv(mailGuestContextEnv(s.publicRequestEnv(r), r), callKWRequest{Kwargs: map[string]any{"context": req.Context}})
	post := req.PostData
	if post == nil {
		post = map[string]any{}
	}
	bodyIsHTML := true
	if _, ok := post["body_is_html"]; ok {
		bodyIsHTML = accountingBoolValue(post["body_is_html"])
	}
	_, attachmentIDsSet := post["attachment_ids"]
	_, attachmentTokensSet := post["attachment_tokens"]
	partnerIDs, err := mailPostPartnerIDs(env, post)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	messageID, err := internalmail.PostMessage(env, internalmail.PostRequest{
		Model:               req.ThreadModel,
		ResID:               req.ThreadID,
		Body:                stringValue(post["body"]),
		Subject:             stringValue(post["subject"]),
		MessageType:         firstTextHTTP(post["message_type"], "comment"),
		EmailFrom:           stringValue(post["email_from"]),
		AuthorID:            int64Value(post["author_id"]),
		ParentID:            int64Value(post["parent_id"]),
		AccessToken:         stringValue(firstNonNil(req.Token, post["token"])),
		AccessHash:          stringValue(firstNonNil(req.Hash, post["hash"])),
		AccessPID:           int64Value(firstNonNil(req.PID, post["pid"])),
		ProjectSharingID:    int64Value(firstNonNil(req.ProjectSharingID, post["project_sharing_id"])),
		SubtypeXMLID:        stringValue(post["subtype_xmlid"]),
		SubtypeID:           int64Value(post["subtype_id"]),
		PartnerIDs:          partnerIDs,
		AttachmentIDs:       int64Slice(post["attachment_ids"]),
		AttachmentIDsSet:    attachmentIDsSet,
		AttachmentTokens:    stringSlice(post["attachment_tokens"]),
		AttachmentTokensSet: attachmentTokensSet,
		TrackingValues:      trackingValuesFromAny(post["tracking_value_ids"]),
		BodyIsHTML:          bodyIsHTML,
		AutoFollow:          boolKwargDefault(req.Context, "mail_post_autofollow", false),
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, map[string]any{
		"message_id": messageID,
		"store_data": map[string]any{"mail.message": []map[string]any{{"id": messageID}}},
	})
}

func mailPostPartnerIDs(env *record.Env, post map[string]any) ([]int64, error) {
	partnerIDs := int64Slice(post["partner_ids"])
	partnerEmails := stringSlice(post["partner_emails"])
	if len(partnerEmails) == 0 {
		return partnerIDs, nil
	}
	canCreatePartners := env.Context().UserID == 1 || sessionHasXMLIDGroup(env, "base.group_partner_manager") || sessionHasXMLIDGroup(env, "base.group_system")
	partners, err := internalmail.PartnersFromEmails(env, partnerEmails, canCreatePartners)
	if err != nil {
		return nil, err
	}
	for _, partner := range partners {
		partnerIDs = append(partnerIDs, int64Value(partner["id"]))
	}
	return uniqueInt64HTTP(partnerIDs), nil
}

func (s Server) mailMessageUpdateContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MessageID        int64          `json:"message_id"`
		UpdateData       map[string]any `json:"update_data"`
		Token            any            `json:"token"`
		Hash             any            `json:"hash"`
		PID              any            `json:"pid"`
		ProjectSharingID any            `json:"project_sharing_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	updateData := req.UpdateData
	if updateData == nil {
		updateData = map[string]any{}
	}
	bodyValue, bodySet := updateData["body"]
	bodySet = bodySet && bodyValue != nil
	_, attachmentIDsSet := updateData["attachment_ids"]
	_, attachmentTokensSet := updateData["attachment_tokens"]
	_, partnerIDsSet := updateData["partner_ids"]
	result, err := internalmail.UpdateMessageContent(mailGuestContextEnv(s.publicRequestEnv(r), r), internalmail.MessageContentUpdateRequest{
		MessageID:           req.MessageID,
		Body:                stringValue(bodyValue),
		BodySet:             bodySet,
		AccessToken:         stringValue(firstNonNil(req.Token, updateData["token"])),
		AccessHash:          stringValue(firstNonNil(req.Hash, updateData["hash"])),
		AccessPID:           int64Value(firstNonNil(req.PID, updateData["pid"])),
		ProjectSharingID:    int64Value(firstNonNil(req.ProjectSharingID, updateData["project_sharing_id"])),
		AttachmentIDs:       int64Slice(updateData["attachment_ids"]),
		AttachmentIDsSet:    attachmentIDsSet,
		AttachmentTokens:    stringSlice(updateData["attachment_tokens"]),
		AttachmentTokensSet: attachmentTokensSet,
		PartnerIDs:          int64Slice(updateData["partner_ids"]),
		PartnerIDsSet:       partnerIDsSet,
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) mailAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	modelName := strings.TrimSpace(r.FormValue("thread_model"))
	threadID := int64Value(r.FormValue("thread_id"))
	if modelName == "" || threadID == 0 {
		writeRPCError(w, nil, http.StatusBadRequest, fmt.Errorf("attachment upload requires thread model and id"))
		return
	}
	env := mailGuestContextEnv(s.publicRequestEnv(r), r)
	accessToken := firstTextHTTP(r.FormValue("token"), r.FormValue("access_token"))
	accessHash := firstTextHTTP(r.FormValue("hash"), r.FormValue("_hash"))
	accessPID := int64Value(r.FormValue("pid"))
	if accessToken != "" || accessHash != "" || accessPID != 0 {
		env = internalmail.PortalContextEnv(env, modelName, threadID, accessToken, accessHash, accessPID)
	}
	if !internalmail.ThreadAccessible(env, modelName, threadID) {
		http.Error(w, "attachment upload target not found", http.StatusNotFound)
		return
	}
	file, header, err := r.FormFile("ufile")
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	resModel := modelName
	resID := threadID
	if pending := strings.TrimSpace(r.FormValue("is_pending")); pending != "" && pending != "false" {
		resModel = "mail.compose.message"
		resID = 0
	}
	mimetype := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}
	systemEnv := attachmentSystemEnv(env)
	attachmentID, err := systemEnv.Model("ir.attachment").Create(map[string]any{
		"name":      header.Filename,
		"res_model": resModel,
		"res_id":    resID,
		"type":      "binary",
		"mimetype":  mimetype,
		"datas":     data,
	})
	if err != nil {
		writeRPCError(w, nil, http.StatusForbidden, err)
		return
	}
	item := mailAttachmentStoreItem(systemEnv, attachmentID, header.Filename, mimetype, len(data), resModel, resID, modelName, threadID)
	writeJSON(w, map[string]any{"data": map[string]any{
		"attachment_id": attachmentID,
		"store_data":    map[string]any{"ir.attachment": []map[string]any{item}},
	}})
}

func attachmentSystemEnv(env *record.Env) *record.Env {
	if env == nil {
		return env
	}
	ctx := env.Context()
	ctx.UserID = 1
	ctx.Values = cloneContextValues(ctx.Values)
	return env.WithContext(ctx)
}

func mailAttachmentStoreItem(env *record.Env, attachmentID int64, name string, mimetype string, fileSize int, resModel string, resID int64, threadModel string, threadID int64) map[string]any {
	return map[string]any{
		"checksum":               "",
		"file_size":              fileSize,
		"filename":               name,
		"has_thumbnail":          false,
		"id":                     attachmentID,
		"mimetype":               mimetype,
		"name":                   name,
		"ownership_token":        internalmail.AttachmentOwnershipToken(env, attachmentID),
		"raw_access_token":       internalmail.AttachmentRawAccessToken(env, attachmentID),
		"res_id":                 resID,
		"res_model":              resModel,
		"thread":                 map[string]any{"id": threadID, "model": threadModel},
		"thumbnail_access_token": internalmail.AttachmentThumbnailAccessToken(env, attachmentID),
		"type":                   "binary",
		"url":                    "",
	}
}

func (s Server) mailAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AttachmentID any `json:"attachment_id"`
		AccessToken  any `json:"access_token"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	attachmentID := int64Value(req.AttachmentID)
	if attachmentID == 0 {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("attachment not found"))
		return
	}
	env := mailGuestContextEnv(s.publicRequestEnv(r), r)
	if err := internalmail.CheckAttachmentOwnership(env, attachmentID, stringValue(req.AccessToken)); err != nil {
		s.publishAttachmentDeleteBus(env, attachmentID, 0)
		writeRPCError(w, envelope, http.StatusNotFound, err)
		return
	}
	systemEnv := attachmentSystemEnv(env)
	messageID := attachmentLinkedMessageID(systemEnv, attachmentID)
	if err := systemEnv.Model("ir.attachment").Browse(attachmentID).Unlink(); err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	s.publishAttachmentDeleteBus(env, attachmentID, messageID)
	writeRPCOrJSON(w, envelope, nil)
}

func attachmentLinkedMessageID(env *record.Env, attachmentID int64) int64 {
	if env == nil || attachmentID == 0 {
		return 0
	}
	found, err := env.Model("mail.message").Search(domain.And())
	if err != nil {
		return 0
	}
	rows, err := found.Read("id", "attachment_ids")
	if err != nil {
		return 0
	}
	for _, row := range rows {
		if containsAttachmentID(int64Slice(row["attachment_ids"]), attachmentID) {
			return int64Value(row["id"])
		}
	}
	return 0
}

func containsAttachmentID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func (s Server) publishAttachmentDeleteBus(env *record.Env, attachmentID int64, messageID int64) {
	if s.Bus == nil || env == nil {
		return
	}
	payload := map[string]any{"id": attachmentID}
	if messageID != 0 {
		payload["message"] = map[string]any{"id": messageID}
	}
	ctx := env.Context()
	if guestID := int64Value(firstNonNil(ctx.Values["guest_id"], ctx.Values["mail_guest_id"])); guestID != 0 {
		s.Bus.Publish(guestBusChannel(guestID), "ir.attachment/delete", payload, time.Now().UTC())
		return
	}
	if ctx.UserID != 0 {
		s.Bus.Publish(userBusChannel(ctx.UserID), "ir.attachment/delete", payload, time.Now().UTC())
	}
}

func (s Server) mailMessageReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MessageID any    `json:"message_id"`
		Content   string `json:"content"`
		Action    string `json:"action"`
		Token     any    `json:"token"`
		Hash      any    `json:"hash"`
		PID       any    `json:"pid"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env := mailGuestContextEnv(s.publicRequestEnv(r), r)
	result, err := internalmail.ReactMessage(env, internalmail.MessageReactionRequest{
		MessageID:   int64Value(req.MessageID),
		Content:     req.Content,
		Action:      req.Action,
		AccessToken: stringValue(req.Token),
		AccessHash:  stringValue(req.Hash),
		AccessPID:   int64Value(req.PID),
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	s.publishMailMessageBus(env, req.MessageID, "mail.record/insert", result)
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) mailMessageAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	messageID, ok := parseMailMessageAvatarPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	accessToken := strings.TrimSpace(query.Get("access_token"))
	accessHash := strings.TrimSpace(firstTextHTTP(query.Get("_hash"), query.Get("hash")))
	accessPID := int64Value(query.Get("pid"))
	if accessToken != "" || accessHash != "" || accessPID != 0 {
		env := internalmail.PortalMessageContextEnv(mailGuestContextEnv(s.publicRequestEnv(r), r), messageID, accessToken, accessHash, accessPID)
		if internalmail.PortalPartnerID(env) == 0 {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(defaultAvatarPNG)
}

func parseMailMessageAvatarPath(path string) (int64, bool) {
	const prefix = "/mail/avatar/mail.message/"
	rest := strings.TrimPrefix(path, prefix)
	if rest == path {
		return 0, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[1] != "author_avatar" || !strings.Contains(parts[2], "x") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	return id, err == nil && id != 0
}

var defaultAvatarPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x60, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func (s Server) mailView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	storeAuthQueryCookies(w, query)
	modelName := strings.TrimSpace(query.Get("model"))
	resID := int64Value(query.Get("res_id"))
	if resID == 0 {
		if messageID := int64Value(query.Get("message_id")); messageID != 0 {
			if messageModel, messageResID := s.mailViewMessageThread(messageID); messageModel != "" && messageResID != 0 {
				modelName = messageModel
				resID = messageResID
			}
		}
	}
	if modelName == "" || resID == 0 {
		http.Redirect(w, r, "/odoo/action-mail.action_discuss", http.StatusFound)
		return
	}
	accessToken := strings.TrimSpace(query.Get("access_token"))
	accessHash := strings.TrimSpace(query.Get("hash"))
	accessPID := int64Value(query.Get("pid"))
	env := mailGuestContextEnv(s.publicRequestEnv(r), r)
	if accessToken != "" || accessHash != "" || accessPID != 0 {
		env = internalmail.PortalContextEnv(env, modelName, resID, accessToken, accessHash, accessPID)
	}
	if !internalmail.ThreadAccessible(env, modelName, resID) {
		http.Redirect(w, r, "/web/login?"+url.Values{"redirect": []string{"/mail/view?" + mailViewAllowedParams(query).Encode()}}.Encode(), http.StatusFound)
		return
	}
	http.Redirect(w, r, mailViewRecordURL(modelName, resID, query), http.StatusFound)
}

func (s Server) mailViewMessageThread(messageID int64) (string, int64) {
	if s.Env == nil || messageID == 0 {
		return "", 0
	}
	rows, err := s.Env.WithContext(record.Context{UserID: 1, CompanyID: s.Env.Context().CompanyID, CompanyIDs: s.Env.Context().CompanyIDs}).Model("mail.message").Browse(messageID).Read("model", "res_id")
	if err != nil || len(rows) == 0 {
		return "", 0
	}
	return strings.TrimSpace(stringValue(rows[0]["model"])), int64Value(rows[0]["res_id"])
}

func storeAuthQueryCookies(w http.ResponseWriter, values url.Values) {
	for _, key := range []string{"auth_signup_token", "auth_login"} {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			http.SetCookie(w, &http.Cookie{Name: key, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		}
	}
}

func mailViewAllowedParams(values url.Values) url.Values {
	out := url.Values{}
	for _, key := range []string{"model", "res_id", "access_token", "auth_signup_token", "auth_login", "pid", "hash", "token", "action"} {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			out.Set(key, value)
		}
	}
	return out
}

func mailViewRecordURL(modelName string, resID int64, values url.Values) string {
	urlModel := modelName
	if !strings.Contains(urlModel, ".") {
		urlModel = "m-" + urlModel
	}
	params := url.Values{}
	for _, key := range []string{"pid", "hash", "highlight_message_id"} {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			params.Set(key, value)
		}
	}
	if len(params) == 0 {
		return fmt.Sprintf("/odoo/%s/%d", url.PathEscape(urlModel), resID)
	}
	return fmt.Sprintf("/odoo/%s/%d?%s", url.PathEscape(urlModel), resID, params.Encode())
}

func (s Server) livechatMessageReaction(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		GuestToken string `json:"guest_token"`
		MessageID  any    `json:"message_id"`
		Content    string `json:"content"`
		Action     string `json:"action"`
		Token      any    `json:"token"`
		Hash       any    `json:"hash"`
		PID        any    `json:"pid"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := mailGuestTokenContextEnv(s.publicRequestEnv(r), req.GuestToken)
	if !ok {
		writeRPCError(w, envelope, http.StatusNotFound, errors.New("guest not found"))
		return
	}
	result, err := internalmail.ReactMessage(env, internalmail.MessageReactionRequest{
		MessageID:   int64Value(req.MessageID),
		Content:     req.Content,
		Action:      req.Action,
		AccessToken: stringValue(req.Token),
		AccessHash:  stringValue(req.Hash),
		AccessPID:   int64Value(req.PID),
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	s.publishMailMessageBus(env, req.MessageID, "mail.record/insert", result)
	writeRPCOrJSON(w, envelope, result)
}

func mailGuestContextEnv(env *record.Env, r *http.Request) *record.Env {
	if env == nil || r == nil {
		return env
	}
	cookie, err := r.Cookie("dgid")
	if err != nil {
		return env
	}
	parts := strings.SplitN(cookie.Value, "|", 2)
	if len(parts) != 2 {
		return env
	}
	guestID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || guestID == 0 || !mailGuestTokenValid(env, guestID, parts[1]) {
		return env
	}
	ctx := env.Context()
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["guest_id"] = guestID
	return env.WithContext(ctx)
}

func mailGuestTokenContextEnv(env *record.Env, token string) (*record.Env, bool) {
	if env == nil {
		return env, false
	}
	parts := strings.SplitN(token, "|", 2)
	if len(parts) != 2 {
		return env, false
	}
	guestID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || guestID == 0 || !mailGuestTokenValid(env, guestID, parts[1]) {
		return env, false
	}
	ctx := env.Context()
	ctx.UserID = 0
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["guest_id"] = guestID
	return env.WithContext(ctx), true
}

func mailGuestTokenValid(env *record.Env, guestID int64, token string) bool {
	if env == nil || token == "" {
		return false
	}
	ctx := env.Context()
	ctx.UserID = 1
	rows, err := env.WithContext(ctx).Model("mail.guest").Browse(guestID).Read("access_token")
	if err != nil || len(rows) == 0 {
		return false
	}
	expected := stringValue(rows[0]["access_token"])
	return expected != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (s Server) mailAction(w http.ResponseWriter, r *http.Request) {
	s.mailStoreRequest(w, r)
}

func (s Server) mailData(w http.ResponseWriter, r *http.Request) {
	s.mailStoreRequest(w, r)
}

func (s Server) mailStoreRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FetchParams []any          `json:"fetch_params"`
		Context     map[string]any `json:"context"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env := requestContextEnvFromMap(mailGuestContextEnv(s.publicRequestEnv(r), r), req.Context)
	payload := map[string]any{}
	for _, raw := range req.FetchParams {
		name, params, dataID := mailStoreFetchParam(raw)
		if name == "" {
			continue
		}
		s.processMailStoreFetch(payload, env, name, params, dataID)
	}
	writeRPCOrJSON(w, envelope, payload)
}

func mailStoreFetchParam(raw any) (string, map[string]any, any) {
	if name := strings.TrimSpace(stringValue(raw)); name != "" {
		return name, map[string]any{}, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return "", map[string]any{}, nil
	}
	name := strings.TrimSpace(stringValue(items[0]))
	if name == "" {
		return "", map[string]any{}, nil
	}
	params := map[string]any{}
	if len(items) > 1 {
		params = mapValue(items[1])
	}
	var dataID any
	if len(items) > 2 {
		dataID = items[2]
	}
	return name, params, dataID
}

func (s Server) processMailStoreFetch(payload map[string]any, env *record.Env, name string, params map[string]any, dataID any) {
	if payload == nil || env == nil {
		return
	}
	switch name {
	case "init_messaging":
		s.addMailStoreInitMessaging(payload, env)
	case "mail.thread":
		s.addMailStoreThread(payload, env, params)
	case "failures":
		s.addMailStoreFailures(payload, env)
	}
	if env.Context().UserID != 0 && sessionInternalUser(env) {
		switch name {
		case "systray_get_activities":
			s.addMailStoreSystrayActivities(payload, env)
		case "mail.canned.response":
			s.addMailStoreCannedResponses(payload, env)
		case "avatar_card":
			s.addMailStoreAvatarCard(payload, env, params)
		}
	}
	if dataID != nil {
		mailStoreMergeRecords(payload, "DataResponse", []map[string]any{{
			"_resolve": true,
			"id":       dataID,
		}}, "id")
	}
}

func (s Server) addMailStoreInitMessaging(payload map[string]any, env *record.Env) {
	counterBusID := int64(0)
	if s.Bus != nil {
		counterBusID = s.Bus.LastID()
	}
	store := map[string]any{
		"action_discuss_id":             falseIfZero(httpXMLIDResID(env, "mail", "action_discuss", "ir.actions.client")),
		"channel_types_with_seen_infos": []string{"chat", "group"},
		"hasCannedResponses":            mailStoreHasCannedResponses(env),
		"hasGifPickerFeature":           configParameterValue(env, "discuss.tenor_api_key") != "",
		"hasLinkPreviewFeature":         true,
		"hasMessageTranslationFeature":  configParameterValue(env, "mail.google_translate_api_key") != "",
		"inbox":                         mailStoreInboxMailbox(env, counterBusID),
		"internalUserGroupId":           falseIfZero(httpXMLIDResID(env, "base", "group_user", "res.groups")),
		"mt_comment":                    falseIfZero(httpXMLIDResID(env, "mail", "mt_comment", "mail.message.subtype")),
		"mt_note":                       falseIfZero(httpXMLIDResID(env, "mail", "mt_note", "mail.message.subtype")),
		"settings":                      sessionUserSettingsPayload(env),
		"starred":                       internalmail.StarredMailboxPayload(env, counterBusID),
		"initChannelsUnreadCounter":     mailStoreUnreadChannelCounter(env),
		"activityCounter":               mailStoreActivityCounter(env),
		"activity_counter_bus_id":       counterBusID,
		"odoobot":                       falseIfZero(httpXMLIDResID(env, "base", "partner_root", "res.partner")),
		"self_partner":                  falseIfZero(mailStoreCurrentPartnerID(env)),
	}
	mailStoreMergeSingleton(payload, "Store", store)
	if partnerID := int64Value(store["odoobot"]); partnerID != 0 {
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, []int64{partnerID}), "id")
	}
	if partnerID := int64Value(store["self_partner"]); partnerID != 0 {
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, []int64{partnerID}), "id")
	}
	if guestID := mailStoreCurrentGuestID(env); guestID != 0 {
		mailStoreMergeSingleton(payload, "Store", map[string]any{"self_guest": guestID})
		mailStoreMergeRecords(payload, "mail.guest", mailStoreGuestRows(env, []int64{guestID}), "id")
	}
}

func (s Server) addMailStoreThread(payload map[string]any, env *record.Env, params map[string]any) {
	modelName := strings.TrimSpace(stringValue(params["thread_model"]))
	threadID := int64Value(params["thread_id"])
	if modelName == "" || threadID == 0 {
		return
	}
	row := mailStoreThreadRow(env, modelName, threadID, stringSlice(params["request_list"]))
	mailStoreMergeRecords(payload, "mail.thread", []map[string]any{row}, "model", "id")
	if activities := int64Slice(row["activities"]); len(activities) > 0 {
		if store, err := internalmail.ActivityFormat(env, activities); err == nil {
			mailStoreMerge(payload, store)
		}
	}
	if attachments := int64Slice(row["attachments"]); len(attachments) > 0 {
		mailStoreMergeRecords(payload, "ir.attachment", mailStoreAttachmentRows(env, attachments), "id")
	}
	if followers := int64Slice(row["followers"]); len(followers) > 0 {
		followerRows := mailStoreFollowerRows(env, followers)
		mailStoreMergeRecords(payload, "mail.followers", followerRows, "id")
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, mailStoreFollowerPartnerIDs(followerRows)), "id")
	}
	if recipients := int64Slice(row["recipients"]); len(recipients) > 0 {
		recipientRows := mailStoreFollowerRows(env, recipients)
		mailStoreMergeRecords(payload, "mail.followers", recipientRows, "id")
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, mailStoreFollowerPartnerIDs(recipientRows)), "id")
	}
	if selfFollowerID := int64Value(row["selfFollower"]); selfFollowerID != 0 {
		selfRows := mailStoreFollowerRows(env, []int64{selfFollowerID})
		mailStoreMergeRecords(payload, "mail.followers", selfRows, "id")
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, mailStoreFollowerPartnerIDs(selfRows)), "id")
	}
	if scheduled := int64Slice(row["scheduledMessages"]); len(scheduled) > 0 {
		mailStoreMergeRecords(payload, "mail.scheduled.message", mailStoreScheduledMessageRows(env, scheduled), "id")
	}
}

func (s Server) addMailStoreFailures(payload map[string]any, env *record.Env) {
	partnerID := mailStoreCurrentPartnerID(env)
	if partnerID == 0 || !modelHasField(env, "mail.notification", "notification_status") {
		return
	}
	found, err := env.Model("mail.notification").Search(domain.And(
		domain.Cond("author_id", "=", partnerID),
		domain.Cond("notification_status", "in", []string{"bounce", "exception"}),
	))
	if err != nil || found.Len() == 0 {
		return
	}
	rows, err := found.Read("mail_message_id")
	if err != nil {
		return
	}
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		messageIDs = append(messageIDs, int64Value(row["mail_message_id"]))
	}
	mailStoreMergeRecords(payload, "mail.notification", mailStoreNotificationRows(env, messageIDs), "id")
}

func (s Server) addMailStoreSystrayActivities(payload map[string]any, env *record.Env) {
	counterBusID := int64(0)
	if s.Bus != nil {
		counterBusID = s.Bus.LastID()
	}
	groups := mailStoreActivityGroups(env)
	total := int64(0)
	for _, group := range groups {
		total += int64Value(group["total_count"])
	}
	mailStoreMergeSingleton(payload, "Store", map[string]any{
		"activityCounter":         total,
		"activity_counter_bus_id": counterBusID,
		"activityGroups":          groups,
	})
}

func (s Server) addMailStoreCannedResponses(payload map[string]any, env *record.Env) {
	if _, ok := env.ModelMetadata("mail.canned.response"); !ok {
		mailStoreMergeRecords(payload, "mail.canned.response", nil, "id")
		return
	}
	found, err := env.Model("mail.canned.response").Search(domain.Or(
		domain.Cond("create_uid", "=", env.Context().UserID),
		domain.Cond("group_ids", "in", mailStoreCurrentUserGroupIDs(env)),
	))
	if err != nil {
		return
	}
	rows, err := found.Read(availableModelFields(env, "mail.canned.response", "id", "source", "substitution", "last_used", "group_ids", "is_shared", "is_editable")...)
	if err != nil {
		return
	}
	for _, row := range rows {
		if _, ok := row["is_shared"]; !ok {
			row["is_shared"] = len(int64Slice(row["group_ids"])) > 0
		}
		if _, ok := row["is_editable"]; !ok {
			row["is_editable"] = int64Value(row["create_uid"]) == env.Context().UserID || env.Context().UserID == 1
		}
	}
	mailStoreMergeRecords(payload, "mail.canned.response", rows, "id")
}

func (s Server) addMailStoreAvatarCard(payload map[string]any, env *record.Env, params map[string]any) {
	id := int64Value(params["id"])
	modelName := strings.TrimSpace(stringValue(params["model"]))
	if id == 0 || (modelName != "res.users" && modelName != "res.partner") {
		return
	}
	switch modelName {
	case "res.partner":
		mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, []int64{id}), "id")
	case "res.users":
		rows, err := env.Model("res.users").Browse(id).Read(availableModelFields(env, "res.users", "id", "name", "login", "partner_id", "share", "notification_type", "signature")...)
		if err != nil || len(rows) == 0 {
			return
		}
		mailStoreMergeRecords(payload, "res.users", rows, "id")
		if partnerID := int64Value(rows[0]["partner_id"]); partnerID != 0 {
			mailStoreMergeRecords(payload, "res.partner", mailStorePartnerRows(env, []int64{partnerID}), "id")
		}
	}
}

func mailStoreMerge(dst map[string]any, src map[string]any) {
	for modelName, value := range src {
		if modelName == "Store" {
			mailStoreMergeSingleton(dst, modelName, mapValue(value))
			continue
		}
		mailStoreMergeRecords(dst, modelName, mailStoreRows(value), mailStoreIDFields(modelName)...)
	}
}

func mailStoreMergeSingleton(dst map[string]any, modelName string, values map[string]any) {
	if dst == nil || len(values) == 0 {
		return
	}
	current := mapValue(dst[modelName])
	for key, value := range values {
		current[key] = value
	}
	dst[modelName] = current
}

func mailStoreMergeRecords(dst map[string]any, modelName string, rows []map[string]any, idFields ...string) {
	if dst == nil || modelName == "" {
		return
	}
	existing := mailStoreRows(dst[modelName])
	if len(rows) == 0 {
		if _, ok := dst[modelName]; !ok {
			dst[modelName] = []map[string]any{}
		}
		return
	}
	index := map[string]int{}
	for i, row := range existing {
		if key := mailStoreRecordKey(row, idFields); key != "" {
			index[key] = i
		}
	}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		key := mailStoreRecordKey(row, idFields)
		if key != "" {
			if i, ok := index[key]; ok {
				for fieldName, value := range row {
					existing[i][fieldName] = value
				}
				continue
			}
			index[key] = len(existing)
		}
		existing = append(existing, row)
	}
	dst[modelName] = existing
}

func mailStoreRows(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row := mapValue(item); len(row) > 0 {
				out = append(out, row)
			}
		}
		return out
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		return []map[string]any{typed}
	default:
		return nil
	}
}

func mailStoreIDFields(modelName string) []string {
	switch modelName {
	case "mail.thread":
		return []string{"model", "id"}
	case "MessageReactions":
		return []string{"message", "content"}
	default:
		return []string{"id"}
	}
}

func mailStoreRecordKey(row map[string]any, idFields []string) string {
	if len(idFields) == 0 {
		idFields = []string{"id"}
	}
	parts := make([]string, 0, len(idFields))
	for _, fieldName := range idFields {
		value, ok := row[fieldName]
		if !ok || value == nil || value == false || fmt.Sprint(value) == "" || fmt.Sprint(value) == "0" {
			return ""
		}
		parts = append(parts, fieldName+"="+fmt.Sprint(value))
	}
	return strings.Join(parts, "|")
}

func mailStoreCurrentPartnerID(env *record.Env) int64 {
	if env == nil || env.Context().UserID == 0 || !modelHasField(env, "res.users", "partner_id") {
		return 0
	}
	rows, err := env.Model("res.users").Browse(env.Context().UserID).Read("partner_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["partner_id"])
}

func mailStoreCurrentGuestID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	return int64Value(env.Context().Values["guest_id"])
}

func mailStoreCurrentUserGroupIDs(env *record.Env) []int64 {
	groups := menuGroupSet(env)
	out := make([]int64, 0, len(groups))
	for id := range groups {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func mailStoreRecordExists(env *record.Env, modelName string, id int64) bool {
	if env == nil || modelName == "" || id == 0 {
		return false
	}
	if _, ok := env.ModelMetadata(modelName); !ok {
		return false
	}
	rows, err := env.Model(modelName).Browse(id).Read("id")
	return err == nil && len(rows) > 0
}

func mailStoreThreadRow(env *record.Env, modelName string, id int64, requestList []string) map[string]any {
	row := map[string]any{
		"canPostOnReadonly": false,
		"hasReadAccess":     false,
		"hasWriteAccess":    false,
		"has_mail_thread":   true,
		"id":                id,
		"model":             modelName,
	}
	if !mailStoreRecordExists(env, modelName, id) {
		return row
	}
	row["hasReadAccess"] = true
	row["hasWriteAccess"] = true
	if containsHTTPString(requestList, "display_name") {
		row["display_name"] = mailStoreDisplayName(env, modelName, id)
	}
	if containsHTTPString(requestList, "contact_fields") {
		fields, err := internalmail.SuggestedRecipientFields(env, modelName)
		if err == nil {
			row["primary_email_field"] = fields["primary_email_field"]
			row["partner_fields"] = fields["partner_fields"]
		} else {
			row["primary_email_field"] = ""
			row["partner_fields"] = []string{}
		}
	}
	if containsHTTPString(requestList, "attachments") {
		ids := mailStoreAttachmentIDsForThread(env, modelName, id)
		row["attachments"] = ids
		row["areAttachmentsLoaded"] = true
		row["isLoadingAttachments"] = false
	}
	if containsHTTPString(requestList, "activities") {
		row["activities"] = mailStoreActivityIDsForThread(env, modelName, id, true)
	}
	if containsHTTPString(requestList, "followers") {
		followerIDs := mailStoreFollowerIDs(env, modelName, id, false)
		recipientIDs := mailStoreFollowerIDs(env, modelName, id, true)
		selfFollowerID := mailStoreSelfFollowerID(env, modelName, id)
		row["followers"] = followerIDs
		row["followersCount"] = mailStoreFollowerCount(env, modelName, id, false)
		row["recipients"] = recipientIDs
		row["recipientsCount"] = mailStoreFollowerCount(env, modelName, id, true)
		row["selfFollower"] = falseIfZero(selfFollowerID)
	}
	if containsHTTPString(requestList, "scheduledMessages") {
		row["scheduledMessages"] = mailStoreScheduledMessageIDs(env, modelName, id)
	}
	if containsHTTPString(requestList, "suggestedRecipients") {
		recipients, err := internalmail.SuggestedRecipients(env, internalmail.SuggestedRecipientsRequest{
			Model:           modelName,
			ResID:           id,
			ReplyDiscussion: true,
			NoCreate:        true,
		})
		if err == nil {
			row["suggestedRecipients"] = recipients
		} else {
			row["suggestedRecipients"] = []map[string]any{}
		}
	}
	return row
}

func mailStoreDisplayName(env *record.Env, modelName string, id int64) string {
	if env == nil || modelName == "" || id == 0 {
		return fmt.Sprintf("%s,%d", modelName, id)
	}
	if pairs, err := env.Model(modelName).Browse(id).NameGet(); err == nil && len(pairs) > 0 {
		return stringValue(pairs[0][1])
	}
	rows, err := env.Model(modelName).Browse(id).Read(availableModelFields(env, modelName, "name", "display_name")...)
	if err == nil && len(rows) > 0 {
		return firstTextHTTP(rows[0]["display_name"], rows[0]["name"], fmt.Sprintf("%s,%d", modelName, id))
	}
	return fmt.Sprintf("%s,%d", modelName, id)
}

func mailStorePartnerRows(env *record.Env, ids []int64) []map[string]any {
	ids = uniqueInt64Slice(ids)
	if env == nil || len(ids) == 0 {
		return nil
	}
	rows, err := env.Model("res.partner").Browse(ids...).Read(availableModelFields(env, "res.partner", "id", "name", "email", "active", "phone", "company_id", "parent_id", "commercial_partner_id", "partner_share")...)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		id := int64Value(row["id"])
		row["avatar_128"] = fmt.Sprintf("/web/image/res.partner/%d/avatar_128", id)
		row["display_name"] = firstTextHTTP(row["display_name"], row["name"], fmt.Sprintf("Partner %d", id))
	}
	return rows
}

func mailStoreGuestRows(env *record.Env, ids []int64) []map[string]any {
	ids = uniqueInt64Slice(ids)
	if env == nil || len(ids) == 0 || !modelHasField(env, "mail.guest", "name") {
		return nil
	}
	rows, err := env.Model("mail.guest").Browse(ids...).Read(availableModelFields(env, "mail.guest", "id", "name", "email", "country_id", "lang", "timezone")...)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		row["avatar_128"] = fmt.Sprintf("/mail/avatar/mail.guest/%d", int64Value(row["id"]))
	}
	return rows
}

func mailStoreAttachmentIDsForThread(env *record.Env, modelName string, id int64) []int64 {
	if env == nil || !modelHasField(env, "ir.attachment", "res_model") {
		return nil
	}
	found, err := env.Model("ir.attachment").SearchWithOptions(domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", id),
	), record.SearchOptions{Order: "id asc"})
	if err != nil {
		return nil
	}
	return found.IDs()
}

func mailStoreAttachmentRows(env *record.Env, ids []int64) []map[string]any {
	ids = uniqueInt64Slice(ids)
	if env == nil || len(ids) == 0 {
		return nil
	}
	rows, err := env.Model("ir.attachment").Browse(ids...).Read(availableModelFields(env, "ir.attachment", "id", "name", "res_model", "res_id", "mimetype", "checksum", "has_thumbnail", "file_size", "access_token")...)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		row["filename"] = stringValue(row["name"])
		if _, ok := row["has_thumbnail"]; !ok {
			row["has_thumbnail"] = false
		}
	}
	return rows
}

func mailStoreActivityIDsForThread(env *record.Env, modelName string, id int64, activeOnly bool) []int64 {
	if env == nil || !modelHasField(env, "mail.activity", "res_model") {
		return nil
	}
	node := domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", id),
	)
	if activeOnly && modelHasField(env, "mail.activity", "active") {
		node = domain.And(node, domain.Cond("active", "=", true))
	}
	found, err := env.Model("mail.activity").SearchWithOptions(node, record.SearchOptions{Order: "date_deadline asc, id asc"})
	if err != nil {
		return nil
	}
	return found.IDs()
}

func mailStoreFollowerIDs(env *record.Env, modelName string, id int64, recipientsOnly bool) []int64 {
	if env == nil || !modelHasField(env, "mail.followers", "res_model") {
		return nil
	}
	node := domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", id),
	)
	currentPartnerID := mailStoreCurrentPartnerID(env)
	if currentPartnerID != 0 {
		node = domain.And(node, domain.Cond("partner_id", "!=", currentPartnerID))
	}
	if recipientsOnly {
		commentSubtypeID := httpXMLIDResID(env, "mail", "mt_comment", "mail.message.subtype")
		if commentSubtypeID != 0 {
			node = domain.And(node, domain.Cond("subtype_ids", "in", []int64{commentSubtypeID}))
		}
	}
	found, err := env.Model("mail.followers").SearchWithOptions(node, record.SearchOptions{Order: "id asc", Limit: 100})
	if err != nil {
		return nil
	}
	return found.IDs()
}

func mailStoreFollowerCount(env *record.Env, modelName string, id int64, recipientsOnly bool) int64 {
	if env == nil || !modelHasField(env, "mail.followers", "res_model") {
		return 0
	}
	node := domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", id),
	)
	if recipientsOnly {
		currentPartnerID := mailStoreCurrentPartnerID(env)
		if currentPartnerID != 0 {
			node = domain.And(node, domain.Cond("partner_id", "!=", currentPartnerID))
		}
		commentSubtypeID := httpXMLIDResID(env, "mail", "mt_comment", "mail.message.subtype")
		if commentSubtypeID != 0 {
			node = domain.And(node, domain.Cond("subtype_ids", "in", []int64{commentSubtypeID}))
		}
	}
	found, err := env.Model("mail.followers").Search(node)
	if err != nil {
		return 0
	}
	return int64(found.Len())
}

func mailStoreSelfFollowerID(env *record.Env, modelName string, id int64) int64 {
	partnerID := mailStoreCurrentPartnerID(env)
	if partnerID == 0 || !modelHasField(env, "mail.followers", "partner_id") {
		return 0
	}
	found, err := env.Model("mail.followers").SearchWithOptions(domain.And(
		domain.Cond("res_model", "=", modelName),
		domain.Cond("res_id", "=", id),
		domain.Cond("partner_id", "=", partnerID),
	), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func mailStoreFollowerRows(env *record.Env, ids []int64) []map[string]any {
	ids = uniqueInt64Slice(ids)
	if env == nil || len(ids) == 0 {
		return nil
	}
	rows, err := env.Model("mail.followers").Browse(ids...).Read(availableModelFields(env, "mail.followers", "id", "res_model", "res_id", "partner_id", "subtype_ids")...)
	if err != nil {
		return nil
	}
	partnerIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		partnerIDs = append(partnerIDs, int64Value(row["partner_id"]))
	}
	partners := map[int64]map[string]any{}
	for _, partner := range mailStorePartnerRows(env, partnerIDs) {
		partners[int64Value(partner["id"])] = partner
	}
	for _, row := range rows {
		partnerID := int64Value(row["partner_id"])
		partner := partners[partnerID]
		row["display_name"] = firstTextHTTP(partner["display_name"], partner["name"])
		row["email"] = stringValue(partner["email"])
		row["is_active"] = partner["active"] != false
		row["name"] = firstTextHTTP(partner["name"], row["display_name"])
		row["thread"] = map[string]any{"id": int64Value(row["res_id"]), "model": stringValue(row["res_model"])}
	}
	return rows
}

func mailStoreFollowerPartnerIDs(rows []map[string]any) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, int64Value(row["partner_id"]))
	}
	return uniqueInt64Slice(ids)
}

func mailDefaultSubscriptionSubtypeIDs(env *record.Env, modelName string) []int64 {
	rows := mailSubscriptionSubtypeRows(env, modelName, true)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, int64Value(row["id"]))
	}
	return ids
}

func mailSubscriptionSubtypeRows(env *record.Env, modelName string, defaultOnly bool) []map[string]any {
	if env == nil || !modelHasField(env, "mail.message.subtype", "name") {
		return nil
	}
	found, err := env.Model("mail.message.subtype").Search(domain.Cond("id", "!=", int64(0)))
	if err != nil || found.Len() == 0 {
		return nil
	}
	rows, err := found.Read(availableModelFields(env, "mail.message.subtype", "id", "name", "description", "res_model", "default", "internal")...)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		resModel := stringValue(row["res_model"])
		if resModel != "" && resModel != modelName {
			continue
		}
		if defaultOnly && !accountingBoolValue(row["default"]) {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftModel := stringValue(out[i]["res_model"])
		rightModel := stringValue(out[j]["res_model"])
		if leftModel != rightModel {
			return leftModel < rightModel
		}
		leftInternal := accountingBoolValue(out[i]["internal"])
		rightInternal := accountingBoolValue(out[j]["internal"])
		if leftInternal != rightInternal {
			return !leftInternal
		}
		return int64Value(out[i]["id"]) < int64Value(out[j]["id"])
	})
	return out
}

func mailStoreScheduledMessageIDs(env *record.Env, modelName string, id int64) []int64 {
	if env == nil || !modelHasField(env, "mail.scheduled.message", "model") {
		return nil
	}
	found, err := env.Model("mail.scheduled.message").SearchWithOptions(domain.And(
		domain.Cond("model", "=", modelName),
		domain.Cond("res_id", "=", id),
	), record.SearchOptions{Order: "scheduled_date asc, id asc"})
	if err != nil {
		return nil
	}
	return found.IDs()
}

func mailStoreScheduledMessageRows(env *record.Env, ids []int64) []map[string]any {
	ids = uniqueInt64Slice(ids)
	if env == nil || len(ids) == 0 {
		return nil
	}
	rows, err := env.Model("mail.scheduled.message").Browse(ids...).Read(availableModelFields(env, "mail.scheduled.message", "id", "mail_message_id", "mail_mail_id", "mail_template_id", "model", "res_id", "scheduled_date", "author_id", "subject", "body", "partner_ids", "attachment_ids", "composition_comment_option", "is_note", "state")...)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		row["body"] = []any{"markup", stringValue(row["body"])}
	}
	return rows
}

func mailStoreNotificationRows(env *record.Env, messageIDs []int64) []map[string]any {
	messageIDs = uniqueInt64Slice(messageIDs)
	if env == nil || len(messageIDs) == 0 {
		return nil
	}
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_message_id", "in", messageIDs))
	if err != nil {
		return nil
	}
	rows, err := found.Read(availableModelFields(env, "mail.notification", "id", "mail_message_id", "mail_mail_id", "res_partner_id", "mail_email_address", "notification_type", "notification_status", "failure_type", "failure_reason", "is_read", "read_date", "author_id")...)
	if err != nil {
		return nil
	}
	return rows
}

func mailStoreHasCannedResponses(env *record.Env) bool {
	if env == nil {
		return false
	}
	if _, ok := env.ModelMetadata("mail.canned.response"); !ok {
		return false
	}
	found, err := env.Model("mail.canned.response").SearchWithOptions(domain.Or(
		domain.Cond("create_uid", "=", env.Context().UserID),
		domain.Cond("group_ids", "in", mailStoreCurrentUserGroupIDs(env)),
	), record.SearchOptions{Limit: 1})
	return err == nil && found.Len() > 0
}

func mailStoreInboxMailbox(env *record.Env, counterBusID int64) map[string]any {
	counter := int64(0)
	if partnerID := mailStoreCurrentPartnerID(env); partnerID != 0 && modelHasField(env, "mail.notification", "is_read") {
		found, err := env.Model("mail.notification").Search(domain.And(
			domain.Cond("res_partner_id", "=", partnerID),
			domain.Cond("is_read", "=", false),
		))
		if err == nil {
			counter = int64(found.Len())
		}
	}
	return map[string]any{
		"counter":        counter,
		"counter_bus_id": counterBusID,
		"id":             "inbox",
		"model":          "mail.box",
	}
}

func mailStoreUnreadChannelCounter(env *record.Env) int64 {
	if env == nil || !modelHasField(env, "discuss.channel.member", "partner_id") {
		return 0
	}
	partnerID := mailStoreCurrentPartnerID(env)
	if partnerID == 0 {
		return 0
	}
	found, err := env.Model("discuss.channel.member").Search(domain.Cond("partner_id", "=", partnerID))
	if err != nil {
		return 0
	}
	return int64(found.Len())
}

func mailStoreActivityCounter(env *record.Env) int64 {
	if env == nil || !modelHasField(env, "mail.activity", "user_id") || env.Context().UserID == 0 {
		return 0
	}
	node := domain.Cond("user_id", "=", env.Context().UserID)
	if modelHasField(env, "mail.activity", "active") {
		node = domain.And(node, domain.Cond("active", "=", true))
	}
	found, err := env.Model("mail.activity").Search(node)
	if err != nil {
		return 0
	}
	return int64(found.Len())
}

func mailStoreActivityGroups(env *record.Env) []map[string]any {
	if env == nil || !modelHasField(env, "mail.activity", "user_id") || env.Context().UserID == 0 {
		return nil
	}
	node := domain.Cond("user_id", "=", env.Context().UserID)
	if modelHasField(env, "mail.activity", "active") {
		node = domain.And(node, domain.Cond("active", "=", true))
	}
	found, err := env.Model("mail.activity").SearchWithOptions(node, record.SearchOptions{Order: "id desc", Limit: 1000})
	if err != nil {
		return nil
	}
	rows, err := found.Read("id", "res_model", "res_id", "date_deadline")
	if err != nil {
		return nil
	}
	type group struct {
		model        string
		activityIDs  []int64
		overdueCount int64
		todayCount   int64
		plannedCount int64
		totalCount   int64
	}
	groups := map[string]*group{}
	for _, row := range rows {
		modelName := firstTextHTTP(row["res_model"], "mail.activity")
		item := groups[modelName]
		if item == nil {
			item = &group{model: modelName}
			groups[modelName] = item
		}
		item.activityIDs = append(item.activityIDs, int64Value(row["id"]))
		item.totalCount++
		switch mailStoreActivityState(env, row) {
		case "overdue":
			item.overdueCount++
		case "today":
			item.todayCount++
		default:
			item.plannedCount++
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		item := groups[key]
		modelID, modelLabel := mailStoreModelInfo(env, item.model)
		row := map[string]any{
			"activity_ids":  append([]int64(nil), item.activityIDs...),
			"domain":        []any{},
			"icon":          "/base/static/description/icon.png",
			"id":            modelID,
			"model":         item.model,
			"name":          modelLabel,
			"overdue_count": item.overdueCount,
			"planned_count": item.plannedCount,
			"today_count":   item.todayCount,
			"total_count":   item.totalCount,
			"type":          "activity",
			"view_type":     "list",
		}
		if item.model == "mail.activity" {
			row["activity_ids"] = item.activityIDs
			row["name"] = "Other activities"
		}
		out = append(out, row)
	}
	return out
}

func mailStoreActivityState(env *record.Env, row map[string]any) string {
	deadline := strings.TrimSpace(stringValue(row["date_deadline"]))
	if deadline == "" {
		return "planned"
	}
	today := time.Now().UTC().Format("2006-01-02")
	if fixed, ok := env.Context().Values["mail_activity_today"]; ok {
		if value := strings.TrimSpace(stringValue(fixed)); value != "" {
			today = value
		}
	}
	switch {
	case deadline < today:
		return "overdue"
	case deadline == today:
		return "today"
	default:
		return "planned"
	}
}

func mailStoreModelInfo(env *record.Env, modelName string) (int64, string) {
	label := modelName
	id := int64(0)
	if env == nil || !modelHasField(env, "ir.model", "model") {
		return id, label
	}
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", "=", modelName), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return id, label
	}
	rows, err := found.Read("id", "name")
	if err != nil || len(rows) == 0 {
		return id, label
	}
	return int64Value(rows[0]["id"]), firstTextHTTP(rows[0]["name"], modelName)
}

func containsHTTPString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s Server) mailThreadMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel string         `json:"thread_model"`
		ThreadID    int64          `json:"thread_id"`
		FetchParams map[string]any `json:"fetch_params"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	result, err := internalmail.FetchThreadMessages(env, internalmail.ThreadMessagesRequest{
		Model:          req.ThreadModel,
		ResID:          req.ThreadID,
		Limit:          intValue(req.FetchParams["limit"]),
		Offset:         intValue(req.FetchParams["offset"]),
		Before:         int64Value(req.FetchParams["before"]),
		After:          int64Value(req.FetchParams["after"]),
		Around:         int64Value(req.FetchParams["around"]),
		SearchTerm:     stringValue(req.FetchParams["search_term"]),
		IsNotification: boolPointerValue(req.FetchParams["is_notification"]),
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) mailStarredMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FetchParams map[string]any `json:"fetch_params"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	result, err := internalmail.FetchStarredMessages(env, internalmail.ThreadMessagesRequest{
		Limit:          intValue(req.FetchParams["limit"]),
		Offset:         intValue(req.FetchParams["offset"]),
		Before:         int64Value(req.FetchParams["before"]),
		After:          int64Value(req.FetchParams["after"]),
		Around:         int64Value(req.FetchParams["around"]),
		SearchTerm:     stringValue(req.FetchParams["search_term"]),
		IsNotification: boolPointerValue(req.FetchParams["is_notification"]),
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) mailChatterFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel string         `json:"thread_model"`
		ThreadID    int64          `json:"thread_id"`
		FetchParams map[string]any `json:"fetch_params"`
		Token       any            `json:"token"`
		Hash        any            `json:"hash"`
		PID         any            `json:"pid"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	result, err := internalmail.FetchThreadMessages(mailGuestContextEnv(s.publicRequestEnv(r), r), internalmail.ThreadMessagesRequest{
		Model:          req.ThreadModel,
		ResID:          req.ThreadID,
		Limit:          intValue(req.FetchParams["limit"]),
		Offset:         intValue(req.FetchParams["offset"]),
		Before:         int64Value(req.FetchParams["before"]),
		After:          int64Value(req.FetchParams["after"]),
		Around:         int64Value(req.FetchParams["around"]),
		SearchTerm:     stringValue(req.FetchParams["search_term"]),
		IsNotification: boolPointerValue(req.FetchParams["is_notification"]),
		AccessToken:    stringValue(req.Token),
		AccessHash:     stringValue(req.Hash),
		AccessPID:      int64Value(req.PID),
		PortalOnly:     true,
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) portalChatterInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel      string `json:"thread_model"`
		ThreadID         int64  `json:"thread_id"`
		Token            any    `json:"token"`
		Hash             any    `json:"hash"`
		PID              any    `json:"pid"`
		ProjectSharingID any    `json:"project_sharing_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env := internalmail.PortalContextEnvWithAccess(mailGuestContextEnv(s.publicRequestEnv(r), r), req.ThreadModel, req.ThreadID, internalmail.PortalAccessRequest{
		AccessToken:      stringValue(req.Token),
		AccessHash:       stringValue(req.Hash),
		AccessPID:        int64Value(req.PID),
		ProjectSharingID: int64Value(req.ProjectSharingID),
	})
	if !internalmail.ThreadAccessible(env, req.ThreadModel, req.ThreadID) {
		writeRPCOrJSON(w, envelope, map[string]any{})
		return
	}
	portalPartnerID := internalmail.PortalPartnerID(env)
	thread := map[string]any{
		"id":            req.ThreadID,
		"model":         req.ThreadModel,
		"can_react":     env.Context().UserID != 0 || portalPartnerID != 0,
		"hasReadAccess": true,
	}
	result := map[string]any{"mail.thread": []map[string]any{thread}}
	if portalPartnerID != 0 {
		thread["portal_partner"] = portalPartnerID
		if rows := portalPartnerRows(env, portalPartnerID); len(rows) > 0 {
			result["res.partner"] = rows
		}
	}
	writeRPCOrJSON(w, envelope, result)
}

func portalPartnerRows(env *record.Env, partnerID int64) []map[string]any {
	if env == nil || partnerID == 0 {
		return nil
	}
	ctx := env.Context()
	ctx.UserID = 1
	rows, err := env.WithContext(ctx).Model("res.partner").Browse(partnerID).Read("name", "active", "email")
	if err != nil {
		return nil
	}
	return rows
}

func (s Server) mailThreadRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel string `json:"thread_model"`
		ThreadID    int64  `json:"thread_id"`
		MessageID   int64  `json:"message_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	recipients, err := internalmail.SuggestedRecipients(env, internalmail.SuggestedRecipientsRequest{
		Model:           req.ThreadModel,
		ResID:           req.ThreadID,
		MessageID:       req.MessageID,
		ReplyDiscussion: req.MessageID == 0,
		NoCreate:        false,
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	out := make([]map[string]any, 0, len(recipients))
	for _, recipient := range recipients {
		id := int64Value(recipient["partner_id"])
		if id == 0 {
			continue
		}
		out = append(out, map[string]any{
			"id":    id,
			"email": recipient["email"],
			"name":  recipient["name"],
		})
	}
	writeRPCOrJSON(w, envelope, out)
}

func (s Server) mailThreadRecipientsFields(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel string `json:"thread_model"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	fields, err := internalmail.SuggestedRecipientFields(env, req.ThreadModel)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, fields)
}

func (s Server) mailThreadRecipientsGetSuggestedRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadModel string `json:"thread_model"`
		ThreadID    int64  `json:"thread_id"`
		PartnerIDs  any    `json:"partner_ids"`
		MainEmail   any    `json:"main_email"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	partnerIDs := int64Slice(req.PartnerIDs)
	recipients, err := internalmail.SuggestedRecipients(env, internalmail.SuggestedRecipientsRequest{
		Model:                req.ThreadModel,
		ResID:                req.ThreadID,
		ReplyDiscussion:      true,
		NoCreate:             true,
		PrimaryEmail:         stringValue(req.MainEmail),
		AdditionalPartnerIDs: partnerIDs,
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	if len(partnerIDs) > 0 {
		oldCustomerIDs := oldSuggestedCustomerIDs(env, req.ThreadModel, req.ThreadID, partnerIDs)
		if len(oldCustomerIDs) > 0 {
			filtered := recipients[:0]
			for _, recipient := range recipients {
				if oldCustomerIDs[int64Value(recipient["partner_id"])] {
					continue
				}
				filtered = append(filtered, recipient)
			}
			recipients = filtered
		}
	}
	out := make([]map[string]any, 0, len(recipients))
	for _, recipient := range recipients {
		item := map[string]any{}
		for _, key := range []string{"name", "email", "partner_id"} {
			if value, ok := recipient[key]; ok {
				item[key] = value
			}
		}
		out = append(out, item)
	}
	writeRPCOrJSON(w, envelope, out)
}

func oldSuggestedCustomerIDs(env *record.Env, modelName string, resID int64, newPartnerIDs []int64) map[int64]bool {
	out := map[int64]bool{}
	fields, err := internalmail.SuggestedRecipientFields(env, modelName)
	if err != nil {
		return out
	}
	fieldNames := stringSlice(fields["partner_fields"])
	if len(fieldNames) == 0 {
		return out
	}
	rows, err := env.Model(modelName).Browse(resID).Read(fieldNames...)
	if err != nil || len(rows) == 0 {
		return out
	}
	keep := map[int64]bool{}
	for _, id := range newPartnerIDs {
		keep[id] = true
	}
	for _, fieldName := range fieldNames {
		for _, id := range int64Slice(rows[0][fieldName]) {
			if id != 0 && !keep[id] {
				out[id] = true
			}
		}
	}
	return out
}

func (s Server) mailReadSubscriptionData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FollowerID int64 `json:"follower_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	followerRows, err := env.Model("mail.followers").Browse(req.FollowerID).Read(availableModelFields(env, "mail.followers", "id", "res_model", "res_id", "partner_id", "subtype_ids")...)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	if len(followerRows) == 0 {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("mail.followers:%d not found", req.FollowerID))
		return
	}
	modelName := stringValue(followerRows[0]["res_model"])
	resID := int64Value(followerRows[0]["res_id"])
	if !mailStoreRecordExists(env, modelName, resID) {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("%s:%d not found", modelName, resID))
		return
	}
	subtypes := mailSubscriptionSubtypeRows(env, modelName, false)
	subtypeIDs := make([]int64, 0, len(subtypes))
	for _, subtype := range subtypes {
		subtypeIDs = append(subtypeIDs, int64Value(subtype["id"]))
	}
	store := map[string]any{}
	mailStoreMergeRecords(store, "mail.message.subtype", subtypes, "id")
	mailStoreMergeRecords(store, "mail.followers", mailStoreFollowerRows(env, []int64{req.FollowerID}), "id")
	writeRPCOrJSON(w, envelope, map[string]any{
		"store_data":  store,
		"subtype_ids": subtypeIDs,
	})
}

func (s Server) mailThreadSubscribe(w http.ResponseWriter, r *http.Request) {
	s.mailThreadSubscriptionUpdate(w, r, true)
}

func (s Server) mailThreadUnsubscribe(w http.ResponseWriter, r *http.Request) {
	s.mailThreadSubscriptionUpdate(w, r, false)
}

func (s Server) mailThreadSubscriptionUpdate(w http.ResponseWriter, r *http.Request, subscribe bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ResModel   string `json:"res_model"`
		ResID      int64  `json:"res_id"`
		PartnerIDs any    `json:"partner_ids"`
		SubtypeIDs any    `json:"subtype_ids"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	if !mailStoreRecordExists(env, req.ResModel, req.ResID) {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("%s:%d not found", req.ResModel, req.ResID))
		return
	}
	partnerIDs := int64Slice(req.PartnerIDs)
	if subscribe {
		subtypeIDs := int64Slice(req.SubtypeIDs)
		if len(subtypeIDs) == 0 {
			subtypeIDs = mailDefaultSubscriptionSubtypeIDs(env, req.ResModel)
		}
		err = internalmail.Subscribe(env, req.ResModel, req.ResID, partnerIDs, subtypeIDs)
	} else {
		err = internalmail.Unsubscribe(env, req.ResModel, req.ResID, partnerIDs)
	}
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	payload := map[string]any{}
	s.addMailStoreThread(payload, env, map[string]any{
		"thread_model": req.ResModel,
		"thread_id":    req.ResID,
		"request_list": []string{"followers", "suggestedRecipients"},
	})
	writeRPCOrJSON(w, envelope, payload)
}

func (s Server) mailPartnerFromEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Emails   any  `json:"emails"`
		NoCreate bool `json:"no_create"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	canCreatePartners := !req.NoCreate && (env.Context().UserID == 1 || sessionHasXMLIDGroup(env, "base.group_partner_manager") || sessionHasXMLIDGroup(env, "base.group_system"))
	partners, err := internalmail.PartnersFromEmails(env, stringSlice(req.Emails), canCreatePartners)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, partners)
}

func (s Server) mailTrackOpenPixel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mailID, token, ok := parseMailTrackOpenPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	env := s.systemRequestEnv()
	if env == nil || !massMailingOpenTokenValid(env, mailID, token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := markMailingTraceOpenedByMailID(env, mailID, time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(massMailingBlankGIF())
}

func (s Server) linkTrackerRedirect(w http.ResponseWriter, r *http.Request) {
	code, traceID, whatsappID, smsID, ok := parseLinkTrackerPath(r.URL.Path)
	if !ok || code == "" {
		http.NotFound(w, r)
		return
	}
	isWhatsAppRoute := linkTrackerPathIsWhatsApp(r.URL.Path)
	if isWhatsAppRoute && !odooSafeHTTPMethod(r.Method) {
		http.Error(w, "Session expired (invalid CSRF token)", http.StatusBadRequest)
		return
	}
	if !isWhatsAppRoute && r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	env := s.systemRequestEnv()
	if traceID == 0 && smsID != 0 {
		resolvedTraceID, err := mailingTraceIDBySMSID(env, smsID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		traceID = resolvedTraceID
	}
	redirectURL, linkID, campaignID, err := linkTrackerTarget(env, code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if linkID == 0 {
		http.NotFound(w, r)
		return
	}
	if isWhatsAppRoute || linkTrackerPathIsMailing(r.URL.Path) || !isHTTPRequestBot(r) {
		countryID, err := requestCountryIDHTTP(env, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := createLinkTrackerClick(env, linkID, campaignID, traceID, whatsappID, requestRemoteAddr(r), countryID, time.Now().UTC()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if redirectURL == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
}

func linkTrackerPathIsWhatsApp(path string) bool {
	return linkTrackerPathHasSegment(path, "w")
}

func odooSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func linkTrackerPathIsMailing(path string) bool {
	return linkTrackerPathHasSegment(path, "m")
}

func linkTrackerPathHasSegment(path string, segment string) bool {
	rest := strings.TrimPrefix(path, "/r/")
	if rest == path {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) == 3 && parts[1] == segment
}

func (s Server) mailingUnsubscribePlaceholder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/mailing/my", http.StatusMovedPermanently)
}

func (s Server) mailingViewPlaceholder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body><main>This message can be viewed in a browser.</main></body></html>`)
}

func (s Server) mailingPortal(w http.ResponseWriter, r *http.Request) {
	mailingID, action, ok := parseMailingPortalPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "view":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mailingViewInBrowser(w, r, mailingID)
	case "unsubscribe", "confirm_unsubscribe":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mailingUnsubscribe(w, r, mailingID, action == "confirm_unsubscribe")
	case "unsubscribe_oneclick":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.processMailingUnsubscribe(r, mailingID); err != nil {
			writeMailingPortalError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (s Server) smsOptOutPortal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mailingID, traceCode, unsubscribe, ok := parseSMSOptOutPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if unsubscribe {
		s.smsOptOutFinal(w, r, mailingID, traceCode)
		return
	}
	s.smsOptOutEntry(w, r, mailingID, traceCode)
}

type smsStatusRequest struct {
	MessageStatuses *[]smsStatusReport `json:"message_statuses"`
}

type smsStatusReport struct {
	SMSStatus string   `json:"sms_status"`
	UUIDs     []string `json:"uuids"`
}

func (s Server) smsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req smsStatusRequest
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, envelope, http.StatusBadRequest, err)
		return
	}
	if req.MessageStatuses == nil {
		writeRPCError(w, envelope, http.StatusBadRequest, errSMSStatusBadParameters)
		return
	}
	env := s.systemRequestEnv()
	updatedMessageIDs, err := processSMSStatusReports(env, *req.MessageStatuses, time.Now().UTC())
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errSMSStatusBadParameters) {
			status = http.StatusBadRequest
		}
		writeRPCError(w, envelope, status, err)
		return
	}
	s.publishSMSNotificationUpdateBus(env, updatedMessageIDs)
	writeRPCOrJSON(w, envelope, "OK")
}

func (s Server) digestPortal(w http.ResponseWriter, r *http.Request) {
	digestID, actionName, ok := parseDigestPortalPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch actionName {
	case "unsubscribe_oneclik":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := s.digestPortalUnsubscribe(w, r, digestID); !ok {
			return
		}
		w.WriteHeader(http.StatusOK)
	case "unsubscribe":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if accountingBoolValue(digestRequestValue(r, "one_click")) && r.Method != http.MethodPost {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		row, ok := s.digestPortalUnsubscribe(w, r, digestID)
		if !ok {
			return
		}
		writeDigestUnsubscribedPage(w, row)
	case "set_periodicity":
		s.digestPortalSetPeriodicity(w, r, digestID)
	default:
		http.NotFound(w, r)
	}
}

func parseDigestPortalPath(path string) (int64, string, bool) {
	path = strings.Trim(strings.TrimPrefix(path, "/digest/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return 0, "", false
	}
	digestID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	actionName := strings.TrimSpace(parts[1])
	if err != nil || digestID <= 0 || actionName == "" {
		return 0, "", false
	}
	return digestID, actionName, true
}

func (s Server) digestPortalUnsubscribe(w http.ResponseWriter, r *http.Request, digestID int64) (map[string]any, bool) {
	systemEnv := s.systemRequestEnv()
	row, ok, err := digestPortalRow(systemEnv, digestID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	token := strings.TrimSpace(digestRequestValue(r, "token"))
	userID := int64Value(digestRequestValue(r, "user_id"))
	if token != "" && userID != 0 {
		expected := internalmail.DigestUnsubscribeToken(systemEnv, digestID, userID)
		if expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			http.NotFound(w, r)
			return nil, false
		}
		if err := digestPortalUnsubscribeUsers(systemEnv, row, userID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return nil, false
		}
		return row, true
	}
	if token == "" && userID == 0 {
		requestEnv := s.publicRequestEnv(r)
		currentUserID := int64(0)
		if requestEnv != nil {
			currentUserID = requestEnv.Context().UserID
		}
		if currentUserID != 0 && !digestUserIsShare(systemEnv, currentUserID) {
			if err := digestPortalUnsubscribeUsers(systemEnv, row, currentUserID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil, false
			}
			return row, true
		}
	}
	http.NotFound(w, r)
	return nil, false
}

func (s Server) digestPortalSetPeriodicity(w http.ResponseWriter, r *http.Request, digestID int64) {
	requestEnv, ok := s.authenticatedRequestEnv(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	systemEnv := s.systemRequestEnv()
	if !digestEnvHasGroup(systemEnv, requestEnv, "base.group_erp_manager") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periodicity := strings.TrimSpace(digestRequestValue(r, "periodicity"))
	if periodicity == "" {
		periodicity = "weekly"
	}
	if !digestPeriodicityAllowed(periodicity) {
		http.Error(w, "invalid periodicity set on digest", http.StatusBadRequest)
		return
	}
	row, ok, err := digestPortalRow(systemEnv, digestID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := systemEnv.Model("digest.digest").Browse(int64Value(row["id"])).Write(map[string]any{"periodicity": periodicity}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/odoo/digest.digest/%d", digestID), http.StatusSeeOther)
}

func digestPortalRow(env *record.Env, digestID int64) (map[string]any, bool, error) {
	if env == nil || digestID <= 0 {
		return nil, false, nil
	}
	found, err := env.Model("digest.digest").SearchWithOptions(domain.Cond("id", domain.Equal, digestID), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return nil, false, err
	}
	rows, err := found.Read("id", "name", "user_ids")
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

func digestPortalUnsubscribeUsers(env *record.Env, digest map[string]any, userIDs ...int64) error {
	if env == nil || len(digest) == 0 {
		return nil
	}
	remove := map[int64]bool{}
	for _, userID := range userIDs {
		if userID != 0 {
			remove[userID] = true
		}
	}
	if len(remove) == 0 {
		return nil
	}
	current := int64Slice(digest["user_ids"])
	remaining := make([]int64, 0, len(current))
	for _, userID := range current {
		if userID != 0 && !remove[userID] {
			remaining = append(remaining, userID)
		}
	}
	return env.Model("digest.digest").Browse(int64Value(digest["id"])).Write(map[string]any{"user_ids": uniqueInt64Slice(remaining)})
}

func digestRequestValue(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.FormValue(key))
}

func digestUserIsShare(env *record.Env, userID int64) bool {
	if env == nil || userID == 0 {
		return true
	}
	rows, err := env.Model("res.users").Browse(userID).Read("share")
	if err != nil || len(rows) == 0 {
		return true
	}
	return accountingBoolValue(rows[0]["share"])
}

func digestEnvHasGroup(systemEnv *record.Env, requestEnv *record.Env, xmlID string) bool {
	if systemEnv == nil || requestEnv == nil || requestEnv.Context().UserID == 0 {
		return false
	}
	module, name, ok := strings.Cut(strings.TrimSpace(xmlID), ".")
	if !ok {
		return false
	}
	groupID := httpXMLIDResID(systemEnv, module, name, "res.groups")
	if groupID == 0 {
		return false
	}
	if containsHTTPInt64(int64Slice(requestEnv.Context().Values["group_ids"]), groupID) {
		return true
	}
	rows, err := systemEnv.Model("res.users").Browse(requestEnv.Context().UserID).Read("groups_id", "group_ids", "all_group_ids")
	if err != nil || len(rows) == 0 {
		return false
	}
	ids := uniqueInt64Slice(append(append(int64Slice(rows[0]["groups_id"]), int64Slice(rows[0]["group_ids"])...), int64Slice(rows[0]["all_group_ids"])...))
	return containsHTTPInt64(ids, groupID)
}

func digestPeriodicityAllowed(periodicity string) bool {
	switch periodicity {
	case "daily", "weekly", "monthly", "quarterly":
		return true
	default:
		return false
	}
}

func containsHTTPInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeDigestUnsubscribedPage(w http.ResponseWriter, digest map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><main class="o_digest_unsubscribed"><h1>Unsubscribed</h1><p>`))
	_, _ = w.Write([]byte(html.EscapeString(firstTextHTTP(digest["name"], "Digest"))))
	_, _ = w.Write([]byte(`</p></main></body></html>`))
}

func (s Server) whatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.whatsAppWebhookVerify(w, r)
	case http.MethodPost:
		s.whatsAppWebhookPost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s Server) whatsAppWebhookVerify(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	token := strings.TrimSpace(query.Get("hub.verify_token"))
	mode := strings.TrimSpace(query.Get("hub.mode"))
	challenge := query.Get("hub.challenge")
	if token == "" || mode == "" || challenge == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	env := s.systemRequestEnv()
	if !whatsAppWebhookVerifyTokenExists(env, token) || mode != "subscribe" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

func (s Server) whatsAppWebhookPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	env := s.systemRequestEnv()
	now := time.Now().UTC()
	for _, entry := range mapList(payload["entry"]) {
		accountUID := strings.TrimSpace(stringValue(entry["id"]))
		account, ok, err := whatsAppAccountByUID(env, accountUID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok || !whatsAppWebhookSignatureValid(r.Header.Get("X-Hub-Signature-256"), body, stringValue(account["app_secret"])) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		for _, change := range mapList(entry["changes"]) {
			fieldName := strings.TrimSpace(stringValue(change["field"]))
			value := mapValue(change["value"])
			if err := processWhatsAppMessagesWebhookChange(env, accountUID, fieldName, value, now); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := processWhatsAppTemplateWebhookChange(env, int64Value(account["id"]), fieldName, value); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	writeJSON(w, map[string]any{"result": "OK"})
}

func whatsAppWebhookVerifyTokenExists(env *record.Env, token string) bool {
	if env == nil || token == "" || !modelHasField(env, "whatsapp.account", "webhook_verify_token") {
		return false
	}
	found, err := env.Model("whatsapp.account").SearchWithOptions(domain.Cond("webhook_verify_token", domain.Equal, token), record.SearchOptions{Limit: 1})
	return err == nil && found.Len() != 0
}

func whatsAppAccountByUID(env *record.Env, accountUID string) (map[string]any, bool, error) {
	if env == nil || accountUID == "" || !modelHasField(env, "whatsapp.account", "account_uid") {
		return nil, false, nil
	}
	found, err := env.Model("whatsapp.account").SearchWithOptions(domain.Cond("account_uid", domain.Equal, accountUID), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return nil, false, err
	}
	rows, err := found.Read("id", "account_uid", "app_secret")
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

func whatsAppAccountByPhoneUID(env *record.Env, accountUID string, phoneUID string) (int64, bool, error) {
	accountUID = strings.TrimSpace(accountUID)
	phoneUID = strings.TrimSpace(phoneUID)
	if env == nil || accountUID == "" || phoneUID == "" || !modelHasField(env, "whatsapp.account", "account_uid") || !modelHasField(env, "whatsapp.account", "phone_uid") {
		return 0, false, nil
	}
	found, err := env.Model("whatsapp.account").SearchWithOptions(domain.And(
		domain.Cond("account_uid", domain.Equal, accountUID),
		domain.Cond("phone_uid", domain.Equal, phoneUID),
	), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0, false, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, false, err
	}
	return int64Value(rows[0]["id"]), true, nil
}

func whatsAppWebhookSignatureValid(header string, body []byte, appSecret string) bool {
	header = strings.TrimSpace(header)
	appSecret = strings.TrimSpace(appSecret)
	if appSecret == "" || !strings.HasPrefix(header, "sha256=") || len(header) != 71 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write(body)
	expected := fmt.Sprintf("sha256=%x", mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
}

var whatsAppRetryableErrorCodes = map[int64]bool{
	0: true, 1: true, 2: true, 3: true, 4: true, 10: true, 33: true, 190: true, 200: true, 299: true, 368: true,
	80007: true, 130429: true, 131000: true, 131005: true, 131008: true, 131009: true, 131016: true,
	131042: true, 131045: true, 131048: true, 131052: true, 131053: true, 131056: true, 132000: true,
	132001: true, 132012: true, 132015: true, 132016: true, 133004: true, 133006: true, 133010: true,
}

var whatsAppBouncedErrorCodes = map[int64]bool{
	131026: true, 131045: true, 131049: true, 131051: true, 131052: true, 131053: true, 131030: true,
}

func processWhatsAppMessagesWebhookChange(env *record.Env, accountUID string, fieldName string, value map[string]any, now time.Time) error {
	if env == nil || fieldName != "messages" || len(value) == 0 {
		return nil
	}
	phoneUID := whatsAppWebhookPhoneUID(value)
	if phoneUID == "" {
		return nil
	}
	if _, ok, err := whatsAppAccountByPhoneUID(env, accountUID, phoneUID); err != nil || !ok {
		return err
	}
	return processWhatsAppMessageStatuses(env, value, now)
}

func whatsAppWebhookPhoneUID(value map[string]any) string {
	if value == nil {
		return ""
	}
	phoneUID := strings.TrimSpace(stringValue(mapValue(value["metadata"])["phone_number_id"]))
	if phoneUID == "" {
		phoneUID = strings.TrimSpace(stringValue(mapValue(value["whatsapp_business_api_data"])["phone_number_id"]))
	}
	return phoneUID
}

func processWhatsAppMessageStatuses(env *record.Env, value map[string]any, now time.Time) error {
	if env == nil || !modelHasField(env, "whatsapp.message", "msg_uid") {
		return nil
	}
	for _, statusPayload := range mapList(value["statuses"]) {
		msgUID := strings.TrimSpace(stringValue(statusPayload["id"]))
		providerStatus := strings.ToLower(strings.TrimSpace(stringValue(statusPayload["status"])))
		if msgUID == "" || providerStatus == "" {
			continue
		}
		found, err := env.Model("whatsapp.message").SearchWithOptions(domain.Cond("msg_uid", domain.Equal, msgUID), record.SearchOptions{Limit: 1})
		if err != nil {
			return err
		}
		if found.Len() == 0 {
			continue
		}
		rows, err := found.Read("id")
		if err != nil || len(rows) == 0 {
			return err
		}
		messageID := int64Value(rows[0]["id"])
		if messageID == 0 {
			continue
		}
		finalState := whatsAppMessageStateFromProviderStatus(providerStatus)
		if err := env.Model("whatsapp.message").Browse(messageID).Write(map[string]any{"state": finalState}); err != nil {
			return err
		}
		if providerStatus == "failed" {
			state, err := handleWhatsAppMessageStatusError(env, messageID, statusPayload, now)
			if err != nil {
				return err
			}
			finalState = state
		}
		if err := processWhatsAppStatusMarketingEvent(env, messageID, finalState, providerStatus, now); err != nil {
			return err
		}
	}
	return nil
}

func whatsAppMessageStateFromProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed":
		return "error"
	case "cancelled":
		return "cancel"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func handleWhatsAppMessageStatusError(env *record.Env, messageID int64, statusPayload map[string]any, now time.Time) (string, error) {
	errorRows := mapList(statusPayload["errors"])
	if len(errorRows) == 0 {
		return "error", nil
	}
	errorRow := errorRows[0]
	code := int64Value(errorRow["code"])
	title := strings.TrimSpace(stringValue(errorRow["title"]))
	failureType := "unknown"
	if _, hasCode := errorRow["code"]; hasCode {
		if whatsAppRetryableErrorCodes[code] {
			failureType = "whatsapp_recoverable"
		} else {
			failureType = "whatsapp_unrecoverable"
		}
	}
	state := "error"
	if whatsAppBouncedErrorCodes[code] {
		state = "bounced"
	}
	values := map[string]any{"state": state}
	if modelHasField(env, "whatsapp.message", "failure_type") {
		values["failure_type"] = failureType
	}
	if modelHasField(env, "whatsapp.message", "failure_reason") && (code != 0 || title != "") {
		values["failure_reason"] = fmt.Sprintf("%d : %s", code, title)
	}
	if err := env.Model("whatsapp.message").Browse(messageID).Write(values); err != nil {
		return state, err
	}
	if err := cancelWhatsAppMarketingTraces(env, messageID, now, "WhatsApp canceled"); err != nil {
		return state, err
	}
	return state, nil
}

func processWhatsAppStatusMarketingEvent(env *record.Env, messageID int64, finalState string, providerStatus string, now time.Time) error {
	switch finalState {
	case "bounced":
		return processWhatsAppMessageMarketingTraceEvent(env, messageID, "whatsapp_bounced", now)
	case "read":
		return processWhatsAppMessageMarketingTraceEvent(env, messageID, "whatsapp_read", now)
	case "delivered":
		return processWhatsAppMessageMarketingTraceEvent(env, messageID, "whatsapp_delivered", now)
	}
	switch providerStatus {
	case "send", "sent":
		return processWhatsAppMessageMarketingTraceEvent(env, messageID, "whatsapp_send", now)
	}
	return nil
}

func processWhatsAppMessageMarketingTraceEvent(env *record.Env, messageID int64, event string, now time.Time) error {
	if env == nil || messageID == 0 || event == "" || !modelHasField(env, "marketing.trace", "whatsapp_message_id") {
		return nil
	}
	traceIDs, err := whatsAppMarketingTraceIDs(env, messageID)
	if err != nil || len(traceIDs) == 0 {
		return err
	}
	for _, traceID := range traceIDs {
		if err := processWhatsAppMarketingTraceEvent(env, traceID, event, now); err != nil {
			return err
		}
	}
	return nil
}

func processWhatsAppMarketingTraceEvent(env *record.Env, traceID int64, event string, now time.Time) error {
	if env == nil || traceID == 0 || event == "" || !modelHasField(env, "marketing.trace", "parent_id") || !modelHasField(env, "marketing.activity", "trigger_type") {
		return nil
	}
	allowed, err := marketingTraceEventAllowed(env, traceID)
	if err != nil || !allowed {
		return err
	}
	children, err := env.Model("marketing.trace").Search(domain.And(
		domain.Cond("parent_id", domain.Equal, traceID),
		domain.Cond("state", domain.Equal, "scheduled"),
	))
	if err != nil || children.Len() == 0 {
		return err
	}
	childRows, err := children.Read("id", "activity_id")
	if err != nil {
		return err
	}
	activityIDs := make([]int64, 0, len(childRows))
	for _, row := range childRows {
		if activityID := int64Value(row["activity_id"]); activityID != 0 {
			activityIDs = append(activityIDs, activityID)
		}
	}
	activityInfo, err := marketingActivityEventInfo(env, activityIDs)
	if err != nil {
		return err
	}
	for _, row := range childRows {
		childID := int64Value(row["id"])
		info := activityInfo[int64Value(row["activity_id"])]
		trigger := info.Trigger
		switch {
		case trigger == event:
			if err := scheduleMarketingEventChildTrace(env, childID, info, now); err != nil {
				return err
			}
		case event == "whatsapp_read" && trigger == "whatsapp_not_read":
			if err := env.Model("marketing.trace").Browse(childID).Write(map[string]any{
				"state":         "canceled",
				"schedule_date": now,
				"state_msg":     "Parent Whatsapp message got opened",
			}); err != nil {
				return err
			}
		case event == "whatsapp_replied" && trigger == "whatsapp_not_replied":
			if err := env.Model("marketing.trace").Browse(childID).Write(map[string]any{
				"state":         "canceled",
				"schedule_date": now,
				"state_msg":     "Parent Whatsapp was replied to",
			}); err != nil {
				return err
			}
		case event == "whatsapp_click" && trigger == "whatsapp_not_click":
			if err := env.Model("marketing.trace").Browse(childID).Write(map[string]any{
				"state":         "canceled",
				"schedule_date": now,
				"state_msg":     "Parent Whatsapp message was clicked",
			}); err != nil {
				return err
			}
		case event == "whatsapp_bounced" && trigger != "whatsapp_bounced":
			if err := env.Model("marketing.trace").Browse(childID).Write(map[string]any{
				"state":         "canceled",
				"schedule_date": now,
				"state_msg":     "Parent whatsapp was bounced",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func cancelWhatsAppMarketingTraces(env *record.Env, messageID int64, now time.Time, reason string) error {
	if env == nil || messageID == 0 || !modelHasField(env, "marketing.trace", "whatsapp_message_id") {
		return nil
	}
	traceIDs, err := whatsAppMarketingTraceIDs(env, messageID)
	if err != nil {
		return err
	}
	for _, traceID := range traceIDs {
		if err := env.Model("marketing.trace").Browse(traceID).Write(map[string]any{
			"state":         "canceled",
			"schedule_date": now,
			"state_msg":     reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

func whatsAppMarketingTraceIDs(env *record.Env, messageID int64) ([]int64, error) {
	if env == nil || messageID == 0 || !modelHasField(env, "marketing.trace", "whatsapp_message_id") {
		return nil, nil
	}
	traces, err := env.Model("marketing.trace").Search(domain.Cond("whatsapp_message_id", domain.Equal, messageID))
	if err != nil || traces.Len() == 0 {
		return nil, err
	}
	rows, err := traces.Read("id")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64Value(row["id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func processWhatsAppTemplateWebhookChange(env *record.Env, accountID int64, fieldName string, value map[string]any) error {
	if env == nil || len(value) == 0 || !modelHasField(env, "whatsapp.template", "wa_template_uid") {
		return nil
	}
	templateUID := strings.TrimSpace(stringValue(value["message_template_id"]))
	if templateUID == "" {
		return nil
	}
	node := domain.Cond("wa_template_uid", domain.Equal, templateUID)
	if accountID != 0 && modelHasField(env, "whatsapp.template", "wa_account_id") {
		node = domain.And(node, domain.Cond("wa_account_id", domain.Equal, accountID))
	}
	found, err := whatsappInactiveEnvHTTP(env).Model("whatsapp.template").SearchWithOptions(node, record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return err
	}
	templateID := int64Value(rows[0]["id"])
	switch fieldName {
	case "message_template_status_update":
		status := strings.ToLower(strings.TrimSpace(stringValue(value["event"])))
		if status == "" {
			return nil
		}
		if err := env.Model("whatsapp.template").Browse(templateID).Write(map[string]any{"status": status}); err != nil {
			return err
		}
		if status == "rejected" {
			return postWhatsAppTemplateRejectedMessage(env, templateID, value)
		}
	case "message_template_quality_update":
		quality := strings.ToLower(strings.TrimSpace(stringValue(value["new_quality_score"])))
		if quality == "unknown" {
			quality = "none"
		}
		if quality == "" {
			return nil
		}
		return env.Model("whatsapp.template").Browse(templateID).Write(map[string]any{"quality": quality})
	case "template_category_update":
		category := strings.ToLower(strings.TrimSpace(stringValue(value["new_category"])))
		if category == "" {
			return nil
		}
		return env.Model("whatsapp.template").Browse(templateID).Write(map[string]any{"template_type": category})
	}
	return nil
}

func postWhatsAppTemplateRejectedMessage(env *record.Env, templateID int64, value map[string]any) error {
	body := "Your Template has been rejected."
	description := strings.TrimSpace(stringValue(mapValue(value["other_info"])["description"]))
	if description == "" {
		description = strings.TrimSpace(stringValue(value["reason"]))
	}
	if description != "" {
		body += "<br/>Reason : " + html.EscapeString(description)
	}
	_, err := internalmail.PostMessage(env, internalmail.PostRequest{
		Model:        "whatsapp.template",
		ResID:        templateID,
		Body:         body,
		Subject:      "whatsapp",
		MessageType:  "comment",
		SubtypeXMLID: "mail.mt_note",
		BodyIsHTML:   true,
	})
	return err
}

var errSMSStatusBadParameters = errors.New("Bad parameters")

var smsIAPToSMSStateSuccessHTTP = map[string]string{
	"processing": "process",
	"success":    "pending",
	"sent":       "pending",
	"delivered":  "sent",
}

var smsStateToNotificationStatusHTTP = map[string]string{
	"canceled": "canceled",
	"process":  "process",
	"error":    "exception",
	"outgoing": "ready",
	"sent":     "sent",
	"pending":  "pending",
}

var smsStateToTraceStatusHTTP = map[string]string{
	"error":    "error",
	"process":  "process",
	"outgoing": "outgoing",
	"canceled": "cancel",
	"pending":  "pending",
	"sent":     "sent",
}

var smsDeliveryErrorsHTTP = map[string]bool{
	"sms_expired":             true,
	"sms_not_delivered":       true,
	"sms_invalid_destination": true,
	"sms_not_allowed":         true,
	"sms_rejected":            true,
}

var smsBounceDeliveryErrorsHTTP = map[string]bool{
	"sms_invalid_destination": true,
	"sms_not_allowed":         true,
	"sms_rejected":            true,
}

var smsNotificationStatusesToIgnoreHTTP = map[string]map[string]bool{
	"canceled":  {"canceled": true, "process": true, "pending": true, "sent": true},
	"ready":     {"ready": true, "process": true, "pending": true, "sent": true},
	"process":   {"process": true, "pending": true, "sent": true},
	"pending":   {"pending": true, "sent": true},
	"bounce":    {"bounce": true, "sent": true},
	"sent":      {"sent": true},
	"exception": {"exception": true},
}

var smsTraceStatusesToIgnoreHTTP = map[string]map[string]bool{
	"cancel":   {"cancel": true, "process": true, "pending": true, "sent": true},
	"outgoing": {"outgoing": true, "process": true, "pending": true, "sent": true},
	"process":  {"process": true, "pending": true, "sent": true},
	"pending":  {"pending": true, "sent": true},
	"bounce":   {"bounce": true},
	"sent":     {"sent": true},
	"error":    {"error": true},
}

func processSMSStatusReports(env *record.Env, reports []smsStatusReport, now time.Time) ([]int64, error) {
	if env == nil {
		return nil, nil
	}
	allUUIDs := make([]string, 0)
	updatedMessageIDs := make([]int64, 0)
	for _, report := range reports {
		uuids := normalizeSMSStatusUUIDs(report.UUIDs)
		status := strings.TrimSpace(report.SMSStatus)
		if !validSMSStatusPayload(uuids, status) {
			return nil, errSMSStatusBadParameters
		}
		allUUIDs = append(allUUIDs, uuids...)
		if _, ok := env.ModelMetadata("sms.tracker"); !ok {
			continue
		}
		trackers, err := env.Model("sms.tracker").Search(domain.Cond("sms_uuid", domain.In, uuids))
		if err != nil {
			return nil, err
		}
		if trackers.Len() == 0 {
			continue
		}
		rows, err := trackers.Read("sms_uuid", "mail_notification_id", "mailing_trace_id")
		if err != nil {
			return nil, err
		}
		if smsState := smsIAPToSMSStateSuccessHTTP[status]; smsState != "" {
			messageIDs, err := updateSMSTrackersFromStateHTTP(env, rows, smsState, "", "", now)
			if err != nil {
				return nil, err
			}
			updatedMessageIDs = append(updatedMessageIDs, messageIDs...)
			continue
		}
		notificationStatus, failureType, failureReason := smsProviderErrorValuesHTTP(status)
		messageIDs, err := updateSMSTrackersFromProviderErrorHTTP(env, rows, notificationStatus, failureType, failureReason, now)
		if err != nil {
			return nil, err
		}
		updatedMessageIDs = append(updatedMessageIDs, messageIDs...)
	}
	if err := markSMSToDeleteHTTP(env, allUUIDs); err != nil {
		return nil, err
	}
	return uniqueInt64HTTP(updatedMessageIDs), nil
}

func normalizeSMSStatusUUIDs(raw []string) []string {
	uuids := make([]string, 0, len(raw))
	for _, uuid := range raw {
		uuids = append(uuids, strings.TrimSpace(uuid))
	}
	return uuids
}

func validSMSStatusPayload(uuids []string, status string) bool {
	if len(uuids) == 0 || status == "" || !smsStatusWordHTTP(status) {
		return false
	}
	for _, uuid := range uuids {
		if !smsStatusUUIDHTTP(uuid) {
			return false
		}
	}
	return true
}

func smsStatusWordHTTP(value string) bool {
	for _, r := range value {
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		return false
	}
	return value != ""
}

func smsStatusUUIDHTTP(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' {
			continue
		}
		return false
	}
	return true
}

func smsProviderErrorValuesHTTP(providerError string) (string, string, string) {
	failureType := "sms_" + strings.TrimSpace(providerError)
	failureReason := ""
	errorStatus := "exception"
	if !smsDeliveryErrorsHTTP[failureType] {
		return "exception", "unknown", strings.TrimSpace(providerError)
	}
	if smsBounceDeliveryErrorsHTTP[failureType] {
		errorStatus = "bounce"
	}
	return errorStatus, failureType, failureReason
}

func updateSMSTrackersFromProviderErrorHTTP(env *record.Env, trackerRows []map[string]any, notificationStatus string, failureType string, failureReason string, now time.Time) ([]int64, error) {
	messageIDs, err := updateSMSNotificationsHTTP(env, trackerRows, notificationStatus, failureType, failureReason)
	if err != nil {
		return nil, err
	}
	traceStatus := notificationStatus
	if traceStatus == "exception" {
		traceStatus = "error"
	}
	if _, err := updateSMSTracesHTTP(env, trackerRows, traceStatus, failureType, failureReason, now); err != nil {
		return nil, err
	}
	return messageIDs, nil
}

func updateSMSTrackersFromStateHTTP(env *record.Env, trackerRows []map[string]any, smsState string, failureType string, failureReason string, now time.Time) ([]int64, error) {
	messageIDs, err := updateSMSNotificationsHTTP(env, trackerRows, smsStateToNotificationStatusHTTP[smsState], failureType, failureReason)
	if err != nil {
		return nil, err
	}
	traceStatus := smsStateToTraceStatusHTTP[smsState]
	updatedTraceIDs, err := updateSMSTracesHTTP(env, trackerRows, traceStatus, failureType, failureReason, now)
	if err != nil {
		return nil, err
	}
	if err := updateSMSMailingsFromTraceStatusHTTP(env, traceStatus, updatedTraceIDs, now); err != nil {
		return nil, err
	}
	return messageIDs, nil
}

func updateSMSNotificationsHTTP(env *record.Env, trackerRows []map[string]any, notificationStatus string, failureType string, failureReason string) ([]int64, error) {
	if env == nil || notificationStatus == "" || !modelHasField(env, "mail.notification", "notification_status") {
		return nil, nil
	}
	ignore := smsNotificationStatusesToIgnoreHTTP[notificationStatus]
	updatedMessageIDs := make([]int64, 0)
	for _, tracker := range trackerRows {
		notificationID := int64Value(tracker["mail_notification_id"])
		if notificationID == 0 {
			continue
		}
		rows, err := env.Model("mail.notification").Browse(notificationID).Read("notification_status", "mail_message_id")
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		if ignore[stringValue(rows[0]["notification_status"])] {
			continue
		}
		values := map[string]any{"notification_status": notificationStatus}
		if modelHasField(env, "mail.notification", "failure_type") {
			values["failure_type"] = failureType
		}
		if modelHasField(env, "mail.notification", "failure_reason") {
			values["failure_reason"] = failureReason
		}
		if err := env.Model("mail.notification").Browse(notificationID).Write(values); err != nil {
			return nil, err
		}
		updatedMessageIDs = append(updatedMessageIDs, int64Value(rows[0]["mail_message_id"]))
	}
	return uniqueInt64HTTP(updatedMessageIDs), nil
}

func (s Server) publishSMSNotificationUpdateBus(env *record.Env, messageIDs []int64) {
	if s.Bus == nil || env == nil {
		return
	}
	for _, messageID := range uniqueInt64HTTP(messageIDs) {
		s.publishMailMessageBus(env, messageID, "mail.record/insert", map[string]any{
			"mail.message": []map[string]any{{"id": messageID}},
		})
	}
}

func updateSMSTracesHTTP(env *record.Env, trackerRows []map[string]any, traceStatus string, failureType string, failureReason string, now time.Time) ([]int64, error) {
	if env == nil || traceStatus == "" || !modelHasField(env, "mailing.trace", "trace_status") {
		return nil, nil
	}
	ignore := smsTraceStatusesToIgnoreHTTP[traceStatus]
	updated := []int64{}
	for _, tracker := range trackerRows {
		traceID := int64Value(tracker["mailing_trace_id"])
		if traceID == 0 {
			continue
		}
		rows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "sent_datetime")
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		if ignore[stringValue(rows[0]["trace_status"])] {
			continue
		}
		values := map[string]any{"trace_status": traceStatus}
		if modelHasField(env, "mailing.trace", "failure_type") {
			values["failure_type"] = failureType
		}
		if modelHasField(env, "mailing.trace", "failure_reason") {
			values["failure_reason"] = failureReason
		}
		if traceStatus != "outgoing" && traceStatus != "process" && traceStatus != "error" && traceStatus != "cancel" && accountingDateValue(rows[0]["sent_datetime"]).IsZero() {
			values["sent_datetime"] = now
		}
		if err := env.Model("mailing.trace").Browse(traceID).Write(values); err != nil {
			return nil, err
		}
		updated = append(updated, traceID)
	}
	return updated, nil
}

func updateSMSMailingsFromTraceStatusHTTP(env *record.Env, traceStatus string, traceIDs []int64, now time.Time) error {
	if env == nil || len(traceIDs) == 0 || !modelHasField(env, "mailing.trace", "mass_mailing_id") || !modelHasField(env, "mailing.mailing", "state") {
		return nil
	}
	traceRows, err := env.Model("mailing.trace").Browse(traceIDs...).Read("mass_mailing_id")
	if err != nil {
		return err
	}
	mailingIDs := []int64{}
	for _, row := range traceRows {
		if mailingID := int64Value(row["mass_mailing_id"]); mailingID != 0 {
			mailingIDs = append(mailingIDs, mailingID)
		}
	}
	mailingIDs = uniqueInt64HTTP(mailingIDs)
	if len(mailingIDs) == 0 {
		return nil
	}
	if traceStatus == "process" {
		for _, mailingID := range mailingIDs {
			if err := env.Model("mailing.mailing").Browse(mailingID).Write(map[string]any{"state": "sending"}); err != nil {
				return err
			}
		}
		return nil
	}
	for _, mailingID := range mailingIDs {
		processTraces, err := env.Model("mailing.trace").Search(domain.And(
			domain.Cond("mass_mailing_id", domain.Equal, mailingID),
			domain.Cond("trace_status", domain.Equal, "process"),
		))
		if err != nil {
			return err
		}
		if processTraces.Len() != 0 {
			continue
		}
		rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("state", "sent_date")
		if err != nil || len(rows) == 0 {
			return err
		}
		if stringValue(rows[0]["state"]) == "done" {
			continue
		}
		values := map[string]any{"state": "done"}
		if modelHasField(env, "mailing.mailing", "sent_date") {
			values["sent_date"] = now
		}
		if modelHasField(env, "mailing.mailing", "kpi_mail_required") {
			values["kpi_mail_required"] = accountingDateValue(rows[0]["sent_date"]).IsZero()
		}
		if err := env.Model("mailing.mailing").Browse(mailingID).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func markSMSToDeleteHTTP(env *record.Env, uuids []string) error {
	if env == nil || len(uuids) == 0 || !modelHasField(env, "sms.sms", "to_delete") {
		return nil
	}
	found, err := env.Model("sms.sms").Search(domain.Cond("uuid", domain.In, uniqueStringHTTP(uuids)))
	if err != nil || found.Len() == 0 {
		return err
	}
	rows, err := found.Read("id", "to_delete")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if accountingBoolValue(row["to_delete"]) {
			continue
		}
		if err := env.Model("sms.sms").Browse(int64Value(row["id"])).Write(map[string]any{"to_delete": true}); err != nil {
			return err
		}
	}
	return nil
}

func uniqueStringHTTP(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uniqueInt64HTTP(values []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseSMSOptOutPath(path string) (int64, string, bool, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/"), "/"), "/")
	if len(parts) != 3 && len(parts) != 4 {
		return 0, "", false, false
	}
	if parts[0] != "sms" {
		return 0, "", false, false
	}
	mailingID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || mailingID <= 0 {
		return 0, "", false, false
	}
	if len(parts) == 3 {
		traceCode := strings.TrimSpace(parts[2])
		return mailingID, traceCode, false, traceCode != ""
	}
	if parts[2] != "unsubscribe" {
		return 0, "", false, false
	}
	traceCode := strings.TrimSpace(parts[3])
	return mailingID, traceCode, true, traceCode != ""
}

func (s Server) smsOptOutEntry(w http.ResponseWriter, r *http.Request, mailingID int64, traceCode string) {
	env := s.systemRequestEnv()
	_, traces, ok, err := smsOptOutMailingAndTraces(env, mailingID, traceCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok || len(traces) == 0 {
		http.Redirect(w, r, "/odoo", http.StatusFound)
		return
	}
	values := mailingPortalRequestValues(r)
	rawNumber := strings.TrimSpace(values.Get("sms_number"))
	number := normalizeHTTPPhoneNumberForRequest(env, r, rawNumber)
	if rawNumber != "" && number != "" && smsTraceWithNumber(traces, number) != nil {
		query := url.Values{"sms_number": []string{number}}
		http.Redirect(w, r, fmt.Sprintf("/sms/%d/unsubscribe/%s?%s", mailingID, url.PathEscape(traceCode), query.Encode()), http.StatusFound)
		return
	}
	errorText := ""
	if rawNumber != "" && number == "" {
		errorText = "Oops! The phone number seems to be incorrect. Please make sure to include the country code."
	} else if rawNumber != "" {
		errorText = "Oops! Number not found"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body><form method="post">`)
	_, _ = fmt.Fprintf(w, `<input name="sms_number" value="%s">`, html.EscapeString(rawNumber))
	if errorText != "" {
		_, _ = fmt.Fprintf(w, `<div class="error">%s</div>`, html.EscapeString(errorText))
	}
	_, _ = io.WriteString(w, `Confirm unsubscribe</form></body></html>`)
}

func (s Server) smsOptOutFinal(w http.ResponseWriter, r *http.Request, mailingID int64, traceCode string) {
	env := s.systemRequestEnv()
	mailing, traces, ok, err := smsOptOutMailingAndTraces(env, mailingID, traceCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok || len(traces) == 0 {
		http.Redirect(w, r, "/odoo", http.StatusFound)
		return
	}
	number := strings.TrimSpace(mailingPortalRequestValues(r).Get("sms_number"))
	trace := smsTraceWithNumber(traces, number)
	var listsOptOut []string
	if number != "" && trace != nil {
		listIDs := int64Slice(mailing["contact_list_ids"])
		if len(listIDs) != 0 {
			optOutListIDs, err := updateMailingListSubscriptionsFromPhoneHTTP(env, listIDs, number, true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			listsOptOut = mailingListOptOutLabelsHTTP(env, optOutListIDs)
		} else if err := upsertPhoneBlacklistHTTP(env, number, fmt.Sprintf("Blacklist through SMS Marketing unsubscribe (mailing ID: %d - model: %s)", mailingID, stringValue(mailing["mailing_model_real"]))); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body>`)
	if len(listsOptOut) != 0 {
		_, _ = fmt.Fprintf(w, `<p><strong>%s</strong> has been successfully removed from</p>`, html.EscapeString(number))
		for _, name := range listsOptOut {
			_, _ = fmt.Fprintf(w, `<div><strong>%s</strong></div>`, html.EscapeString(name))
		}
	} else {
		_, _ = fmt.Fprintf(w, `<p><strong>%s</strong> has been successfully blacklisted</p>`, html.EscapeString(number))
	}
	_, _ = io.WriteString(w, `</body></html>`)
}

func smsOptOutMailingAndTraces(env *record.Env, mailingID int64, traceCode string) (map[string]any, []map[string]any, bool, error) {
	if env == nil || mailingID == 0 || traceCode == "" {
		return nil, nil, false, nil
	}
	mailingRows, err := env.Model("mailing.mailing").Browse(mailingID).Read("contact_list_ids", "mailing_model_real")
	if err != nil || len(mailingRows) == 0 {
		return nil, nil, len(mailingRows) != 0, err
	}
	found, err := env.Model("mailing.trace").Search(domain.And(
		domain.Cond("trace_type", domain.Equal, "sms"),
		domain.Cond("mass_mailing_id", domain.Equal, mailingID),
		domain.Cond("sms_code", domain.Equal, traceCode),
	))
	if err != nil || found.Len() == 0 {
		return mailingRows[0], nil, true, err
	}
	rows, err := found.Read("id", "sms_number", "mass_mailing_id", "sms_code")
	if err != nil {
		return nil, nil, false, err
	}
	return mailingRows[0], rows, true, nil
}

func smsTraceWithNumber(traces []map[string]any, number string) map[string]any {
	if strings.TrimSpace(number) == "" {
		return nil
	}
	for _, trace := range traces {
		if strings.TrimSpace(stringValue(trace["sms_number"])) == number {
			return trace
		}
	}
	return nil
}

func parseMailingPortalPath(path string) (int64, string, bool) {
	rest := strings.TrimPrefix(path, "/mailing/")
	if rest == path {
		return 0, "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 || strings.TrimSpace(parts[1]) == "" {
		return 0, "", false
	}
	return id, strings.TrimSpace(parts[1]), true
}

func (s Server) mailingViewInBrowser(w http.ResponseWriter, r *http.Request, mailingID int64) {
	env := s.systemRequestEnv()
	row, params, err := mailingPortalRowAndParams(env, r, mailingID)
	if err != nil {
		writeMailingPortalError(w, err)
		return
	}
	body := stringValue(row["body_html"])
	if body == "" {
		body = fmt.Sprintf("<p>%s</p>", html.EscapeString(stringValue(row["name"])))
	}
	body = strings.ReplaceAll(body, "/unsubscribe_from_list", mailingUnsubscribeURL(mailingID, params.email, params.documentID, params.hashToken, true))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html><body>")
	_, _ = io.WriteString(w, body)
	_, _ = io.WriteString(w, "</body></html>")
}

func (s Server) mailingUnsubscribe(w http.ResponseWriter, r *http.Request, mailingID int64, confirm bool) {
	if confirm {
		row, params, err := mailingPortalRowAndParams(s.systemRequestEnv(), r, mailingID)
		if err != nil {
			writeMailingPortalError(w, err)
			return
		}
		listNames := mailingPublicListNamesHTTP(s.systemRequestEnv(), int64Slice(row["contact_list_ids"]))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><body><form method="post" action="/mailing/confirm_unsubscribe">`)
		_, _ = fmt.Fprintf(w, `<input type="hidden" name="mailing_id" value="%d">`, mailingID)
		_, _ = fmt.Fprintf(w, `<input type="hidden" name="document_id" value="%d">`, params.documentID)
		_, _ = fmt.Fprintf(w, `<input type="hidden" name="email" value="%s">`, html.EscapeString(params.email))
		_, _ = fmt.Fprintf(w, `<input type="hidden" name="hash_token" value="%s">`, html.EscapeString(params.hashToken))
		for _, name := range listNames {
			_, _ = fmt.Fprintf(w, `<div>%s</div>`, html.EscapeString(name))
		}
		_, _ = io.WriteString(w, `Confirm unsubscribe</form></body></html>`)
		return
	}
	if err := s.processMailingUnsubscribe(r, mailingID); err != nil {
		writeMailingPortalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body>You are no longer part of our services and will not be contacted again.</body></html>`)
}

func (s Server) mailingConfirmUnsubscribePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	values := mailingPortalRequestValues(r)
	mailingID := int64Value(firstNonEmptyStringHTTP(values.Get("mailing_id"), values.Get("mass_mailing_id")))
	if mailingID <= 0 {
		writeMailingPortalError(w, errMailingPortalBadRequest)
		return
	}
	if err := s.processMailingUnsubscribe(r, mailingID); err != nil {
		writeMailingPortalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body>You are no longer part of our services and will not be contacted again.</body></html>`)
}

func (s Server) mailingReportDeactivate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.Form.Get("token"))
	userID := int64Value(r.Form.Get("user_id"))
	if token == "" || userID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	env := s.systemRequestEnv()
	if !internalmail.MassMailingReportTokenValid(env, userID, token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := setConfigParameterValue(env, "mass_mailing.mass_mailing_reports", "False"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><body><h3>Mailing Reports Turned Off</h3><p>Mailing Reports have been turned off for all users.</p></body></html>`)
}

type mailingPortalParams struct {
	documentID int64
	email      string
	hashToken  string
}

func (s Server) processMailingUnsubscribe(r *http.Request, mailingID int64) error {
	env := s.systemRequestEnv()
	row, params, err := mailingPortalRowAndParams(env, r, mailingID)
	if err != nil {
		return err
	}
	if truthyHTTPValue(row["mailing_on_mailing_list"]) {
		return updateMailingListSubscriptionsFromEmailHTTP(env, int64Slice(row["contact_list_ids"]), params.email, true)
	}
	return upsertMailBlacklistHTTP(env, params.email, fmt.Sprintf("Blocklist request from unsubscribe link of mailing %d", mailingID))
}

func mailingPortalRowAndParams(env *record.Env, r *http.Request, mailingID int64) (map[string]any, mailingPortalParams, error) {
	values := mailingPortalRequestValues(r)
	params := mailingPortalParams{
		documentID: int64Value(firstNonEmptyStringHTTP(values.Get("document_id"), values.Get("res_id"))),
		email:      strings.TrimSpace(values.Get("email")),
		hashToken:  strings.TrimSpace(firstNonEmptyStringHTTP(values.Get("hash_token"), values.Get("token"))),
	}
	if env == nil || mailingID == 0 {
		return nil, params, errMailingPortalUnauthorized
	}
	if params.email == "" || params.documentID <= 0 || params.hashToken == "" {
		return nil, params, errMailingPortalBadRequest
	}
	rows, err := env.Model("mailing.mailing").Browse(mailingID).Read("name", "body_html", "mailing_on_mailing_list", "contact_list_ids")
	if err != nil {
		return nil, params, err
	}
	if len(rows) == 0 {
		return nil, params, errMailingPortalUnauthorized
	}
	expected := mailingRecipientToken(env, mailingID, params.documentID, params.email)
	if expected == "" || subtle.ConstantTimeCompare([]byte(params.hashToken), []byte(expected)) != 1 {
		return nil, params, errMailingPortalUnauthorized
	}
	return rows[0], params, nil
}

func mailingPortalRequestValues(r *http.Request) url.Values {
	if r == nil || r.URL == nil {
		return url.Values{}
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		return r.Form
	}
	return r.URL.Query()
}

var (
	errMailingPortalBadRequest   = errors.New("bad mailing portal request")
	errMailingPortalUnauthorized = errors.New("unauthorized mailing portal request")
)

func writeMailingPortalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errMailingPortalBadRequest):
		http.Error(w, "bad request", http.StatusBadRequest)
	case errors.Is(err, errMailingPortalUnauthorized):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func mailingRecipientToken(env *record.Env, mailingID int64, documentID int64, email string) string {
	secret := configParameterValue(env, "database.secret")
	if secret == "" || mailingID <= 0 || documentID <= 0 || strings.TrimSpace(email) == "" {
		return ""
	}
	dbName := "gorp"
	if env != nil {
		dbName = firstNonEmptyStringHTTP(stringContextValue(env.Context(), "db", ""), configParameterValue(env, "database.name"), "gorp")
	}
	payload := fmt.Sprintf("(%s, %d, %d, %s)", pythonReprHTTPString(dbName), mailingID, documentID, pythonReprHTTPString(email))
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func mailingUnsubscribeURL(mailingID int64, email string, documentID int64, hashToken string, confirm bool) string {
	action := "unsubscribe"
	if confirm {
		action = "confirm_unsubscribe"
	}
	values := url.Values{}
	values.Set("document_id", strconv.FormatInt(documentID, 10))
	values.Set("email", email)
	values.Set("hash_token", hashToken)
	return fmt.Sprintf("/mailing/%d/%s?%s", mailingID, action, values.Encode())
}

func upsertMailBlacklistHTTP(env *record.Env, email string, message string) error {
	if env == nil || strings.TrimSpace(email) == "" {
		return nil
	}
	if _, ok := env.ModelMetadata("mail.blacklist"); !ok {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	found, err := env.Model("mail.blacklist").Search(domain.Cond("email", domain.Equal, normalized))
	if err != nil {
		return err
	}
	if found.Len() > 0 {
		return found.Write(map[string]any{"active": true, "message": message})
	}
	_, err = env.Model("mail.blacklist").Create(map[string]any{"email": normalized, "active": true, "message": message})
	return err
}

func upsertPhoneBlacklistHTTP(env *record.Env, phone string, message string) error {
	if env == nil || strings.TrimSpace(phone) == "" {
		return nil
	}
	if _, ok := env.ModelMetadata("phone.blacklist"); !ok {
		return nil
	}
	number := normalizeHTTPPhoneNumberForEnv(env, phone)
	if number == "" {
		return nil
	}
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	found, err := env.WithContext(ctx).Model("phone.blacklist").Search(domain.Cond("number", domain.Equal, number))
	if err != nil {
		return err
	}
	if found.Len() > 0 {
		return found.Write(map[string]any{"active": true, "message": message})
	}
	_, err = env.Model("phone.blacklist").Create(map[string]any{"number": number, "active": true, "message": message})
	return err
}

func updateMailingListSubscriptionsFromEmailHTTP(env *record.Env, listIDs []int64, email string, optOut bool) error {
	if env == nil || len(listIDs) == 0 {
		return nil
	}
	if _, ok := env.ModelMetadata("mailing.contact"); !ok {
		return nil
	}
	normalized := normalizeHTTPEmailAddress(email)
	if normalized == "" {
		return nil
	}
	contacts, err := env.Model("mailing.contact").Search(domain.Cond("email_normalized", domain.Equal, normalized))
	if err != nil || contacts.Len() == 0 {
		return err
	}
	contactRows, err := contacts.Read("id")
	if err != nil {
		return err
	}
	listSet := map[int64]bool{}
	for _, id := range listIDs {
		if id != 0 {
			listSet[id] = true
		}
	}
	for _, contact := range contactRows {
		contactID := int64Value(contact["id"])
		found, err := env.Model("mailing.subscription").Search(domain.Cond("contact_id", domain.Equal, contactID))
		if err != nil {
			return err
		}
		rows, err := found.Read("id", "list_id", "opt_out")
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, row := range rows {
			listID := int64Value(row["list_id"])
			if !listSet[listID] {
				continue
			}
			seen[listID] = true
			if optOut && !truthyHTTPValue(row["opt_out"]) {
				if err := env.Model("mailing.subscription").Browse(int64Value(row["id"])).Write(map[string]any{"opt_out": true}); err != nil {
					return err
				}
			} else if !optOut && truthyHTTPValue(row["opt_out"]) {
				if err := env.Model("mailing.subscription").Browse(int64Value(row["id"])).Write(map[string]any{"opt_out": false}); err != nil {
					return err
				}
			}
		}
		if !optOut {
			for listID := range listSet {
				if seen[listID] {
					continue
				}
				if _, err := env.Model("mailing.subscription").Create(map[string]any{"contact_id": contactID, "list_id": listID}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func updateMailingListSubscriptionsFromPhoneHTTP(env *record.Env, listIDs []int64, phone string, optOut bool) ([]int64, error) {
	if env == nil || len(listIDs) == 0 {
		return nil, nil
	}
	if _, ok := env.ModelMetadata("mailing.contact"); !ok {
		return nil, nil
	}
	number := normalizeHTTPPhoneNumberForEnv(env, phone)
	if number == "" {
		return nil, nil
	}
	contacts, err := env.Model("mailing.contact").Search(domain.Cond("phone_sanitized", domain.Equal, number))
	if err != nil || contacts.Len() == 0 {
		return nil, err
	}
	contactRows, err := contacts.Read("id")
	if err != nil {
		return nil, err
	}
	listSet := map[int64]bool{}
	for _, id := range listIDs {
		if id != 0 {
			listSet[id] = true
		}
	}
	var matchedListIDs []int64
	matchedSeen := map[int64]bool{}
	for _, contact := range contactRows {
		contactID := int64Value(contact["id"])
		found, err := env.Model("mailing.subscription").Search(domain.Cond("contact_id", domain.Equal, contactID))
		if err != nil {
			return nil, err
		}
		rows, err := found.Read("id", "list_id", "opt_out")
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			listID := int64Value(row["list_id"])
			if !listSet[listID] {
				continue
			}
			if !matchedSeen[listID] {
				matchedSeen[listID] = true
				matchedListIDs = append(matchedListIDs, listID)
			}
			if optOut && !truthyHTTPValue(row["opt_out"]) {
				if err := env.Model("mailing.subscription").Browse(int64Value(row["id"])).Write(map[string]any{"opt_out": true}); err != nil {
					return nil, err
				}
			} else if !optOut && truthyHTTPValue(row["opt_out"]) {
				if err := env.Model("mailing.subscription").Browse(int64Value(row["id"])).Write(map[string]any{"opt_out": false}); err != nil {
					return nil, err
				}
			}
		}
	}
	return matchedListIDs, nil
}

func mailingListOptOutLabelsHTTP(env *record.Env, listIDs []int64) []string {
	if env == nil || len(listIDs) == 0 {
		return nil
	}
	orderedIDs := make([]int64, 0, len(listIDs))
	seen := map[int64]bool{}
	for _, id := range listIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		orderedIDs = append(orderedIDs, id)
	}
	if len(orderedIDs) == 0 {
		return nil
	}
	rows, err := env.Model("mailing.list").Browse(orderedIDs...).Read("id", "name", "is_public")
	if err != nil {
		return nil
	}
	byID := make(map[int64]map[string]any, len(rows))
	for _, row := range rows {
		byID[int64Value(row["id"])] = row
	}
	labels := make([]string, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		row, ok := byID[id]
		if !ok {
			continue
		}
		if truthyHTTPValue(row["is_public"]) {
			labels = append(labels, strings.TrimSpace(stringValue(row["name"])))
		} else {
			labels = append(labels, "Mailing List")
		}
	}
	return labels
}

func mailingPublicListNamesHTTP(env *record.Env, listIDs []int64) []string {
	if env == nil || len(listIDs) == 0 {
		return nil
	}
	rows, err := env.Model("mailing.list").Browse(listIDs...).Read("name", "active", "is_public")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if !truthyHTTPValue(row["is_public"]) {
			continue
		}
		if active, ok := row["active"]; ok && !truthyHTTPValue(active) {
			continue
		}
		if name := strings.TrimSpace(stringValue(row["name"])); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func normalizeHTTPEmailAddress(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.LastIndex(value, "<"); start >= 0 {
		if end := strings.Index(value[start:], ">"); end > 0 {
			value = value[start+1 : start+end]
		}
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHTTPPhoneNumber(value string) string {
	return phone.NormalizeE164(value, phone.Country{})
}

func normalizeHTTPPhoneNumberForRequest(env *record.Env, r *http.Request, value string) string {
	country := phone.Country{}
	if countryID, err := requestCountryIDHTTP(env, r); err == nil && countryID != 0 {
		country = phoneCountryByIDHTTP(env, countryID)
	}
	if strings.TrimSpace(country.Code) == "" && country.PhoneCode == 0 {
		country = companyPhoneCountryHTTP(env)
	}
	return phone.NormalizeE164(value, country)
}

func normalizeHTTPPhoneNumberForEnv(env *record.Env, value string) string {
	return phone.NormalizeE164(value, companyPhoneCountryHTTP(env))
}

func companyPhoneCountryHTTP(env *record.Env) phone.Country {
	if env == nil || env.Context().CompanyID == 0 || !modelHasField(env, "res.company", "country_id") {
		return phone.Country{}
	}
	rows, err := env.Model("res.company").Browse(env.Context().CompanyID).Read("country_id")
	if err != nil || len(rows) == 0 {
		return phone.Country{}
	}
	return phoneCountryByIDHTTP(env, int64Value(rows[0]["country_id"]))
}

func phoneCountryByIDHTTP(env *record.Env, countryID int64) phone.Country {
	if env == nil || countryID == 0 || !modelHasField(env, "res.country", "code") {
		return phone.Country{}
	}
	fields := []string{"code"}
	if modelHasField(env, "res.country", "phone_code") {
		fields = append(fields, "phone_code")
	}
	rows, err := env.Model("res.country").Browse(countryID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return phone.Country{}
	}
	return phone.Country{
		Code:      strings.ToUpper(strings.TrimSpace(stringValue(rows[0]["code"]))),
		PhoneCode: int64Value(rows[0]["phone_code"]),
	}
}

func firstNonEmptyStringHTTP(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truthyHTTPValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.TrimSpace(typed) != "" && !strings.EqualFold(strings.TrimSpace(typed), "false")
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return value != nil
	}
}

var httpBotUserAgentSubstrings = []string{"bot", "crawl", "slurp", "spider", "curl", "wget", "facebookexternalhit", "whatsapp", "trendsmapresolver", "pinterest", "instagram", "google-pagerenderer", "preview"}

func isHTTPRequestBot(r *http.Request) bool {
	if r == nil {
		return false
	}
	userAgent := strings.ToLower(r.UserAgent())
	for _, token := range httpBotUserAgentSubstrings {
		if strings.Contains(userAgent, token) {
			return true
		}
	}
	return false
}

func parseMailTrackOpenPath(path string) (int64, string, bool) {
	rest := strings.TrimPrefix(path, "/mail/track/")
	if rest == path {
		return 0, "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[2] != "blank.gif" {
		return 0, "", false
	}
	mailID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || mailID <= 0 || strings.TrimSpace(parts[1]) == "" {
		return 0, "", false
	}
	return mailID, parts[1], true
}

func parseLinkTrackerPath(path string) (string, int64, int64, int64, bool) {
	rest := strings.TrimPrefix(path, "/r/")
	if rest == path || rest == "" {
		return "", 0, 0, 0, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		code := strings.TrimSpace(parts[0])
		if code == "" {
			return "", 0, 0, 0, false
		}
		return code, 0, 0, 0, true
	}
	if len(parts) != 3 {
		return "", 0, 0, 0, false
	}
	code := strings.TrimSpace(parts[0])
	if code == "" || parts[1] == "" || strings.TrimSpace(parts[2]) == "" {
		return "", 0, 0, 0, false
	}
	routeID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || routeID < 0 {
		return "", 0, 0, 0, false
	}
	switch parts[1] {
	case "m":
		if routeID == 0 {
			return "", 0, 0, 0, false
		}
		return code, routeID, 0, 0, true
	case "w":
		return code, 0, routeID, 0, true
	case "s":
		if routeID == 0 {
			return "", 0, 0, 0, false
		}
		return code, 0, 0, routeID, true
	default:
		return "", 0, 0, 0, false
	}
}

func (s Server) systemRequestEnv() *record.Env {
	baseEnv := s.baseEnvWithSecurityPolicy()
	if baseEnv == nil {
		return nil
	}
	ctx := baseEnv.Context()
	ctx.UserID = 1
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["uid"] = int64(1)
	return baseEnv.WithContext(ctx)
}

func massMailingOpenTokenValid(env *record.Env, mailID int64, token string) bool {
	expected := massMailingOpenToken(env, mailID)
	return expected != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func massMailingOpenToken(env *record.Env, mailID int64) string {
	if env == nil || mailID <= 0 {
		return ""
	}
	secret := configParameterValue(env, "database.secret")
	if secret == "" {
		return ""
	}
	payload := fmt.Sprintf("(%s, %d)", pythonReprHTTPString("mass_mailing-mail_mail-open"), mailID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func markMailingTraceOpenedByMailID(env *record.Env, mailID int64, now time.Time) error {
	if env == nil || mailID == 0 {
		return nil
	}
	if _, ok := env.ModelMetadata("mailing.trace"); !ok {
		return nil
	}
	found, err := env.Model("mailing.trace").Search(domain.Cond("mail_mail_id_int", domain.Equal, mailID))
	if err != nil || found.Len() == 0 {
		return err
	}
	rows, err := found.Read("trace_status")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if shouldSkipMailingTraceOpen(stringValue(row["trace_status"])) {
			continue
		}
		if err := env.Model("mailing.trace").Browse(int64Value(row["id"])).Write(map[string]any{"trace_status": "open", "open_datetime": now}); err != nil {
			return err
		}
	}
	return nil
}

func linkTrackerTarget(env *record.Env, code string) (string, int64, int64, error) {
	if env == nil || strings.TrimSpace(code) == "" {
		return "", 0, 0, nil
	}
	if _, ok := env.ModelMetadata("link.tracker.code"); !ok {
		return "", 0, 0, nil
	}
	found, err := env.Model("link.tracker.code").SearchWithOptions(domain.Cond("code", domain.Equal, code), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return "", 0, 0, err
	}
	codeRows, err := found.Read("link_id")
	if err != nil || len(codeRows) == 0 {
		return "", 0, 0, err
	}
	linkID := int64Value(codeRows[0]["link_id"])
	if linkID == 0 {
		return "", 0, 0, nil
	}
	linkRows, err := env.Model("link.tracker").Browse(linkID).Read("url", "redirected_url", "campaign_id", "source_id", "medium_id")
	if err != nil || len(linkRows) == 0 {
		return "", 0, 0, err
	}
	redirectURL := strings.TrimSpace(firstTextHTTP(linkRows[0]["redirected_url"], linkRows[0]["url"]))
	if strings.TrimSpace(stringValue(linkRows[0]["redirected_url"])) == "" {
		redirectURL = linkTrackerRedirectedURLHTTP(env, stringValue(linkRows[0]["url"]), linkRows[0])
	}
	return redirectURL, linkID, int64Value(linkRows[0]["campaign_id"]), nil
}

func createLinkTrackerClick(env *record.Env, linkID int64, campaignID int64, traceID int64, whatsappID int64, ip string, countryID int64, now time.Time) (int64, error) {
	if env == nil || linkID == 0 {
		return 0, nil
	}
	if _, ok := env.ModelMetadata("link.tracker.click"); !ok {
		return 0, nil
	}
	values := map[string]any{"link_id": linkID}
	if strings.TrimSpace(ip) != "" {
		values["ip"] = strings.TrimSpace(ip)
	}
	if countryID != 0 {
		values["country_id"] = countryID
	}
	traceRow, traceExists, err := mailingTraceRow(env, traceID)
	if err != nil {
		return 0, err
	}
	if traceExists {
		values["mailing_trace_id"] = traceID
		if traceCampaignID := int64Value(traceRow["campaign_id"]); traceCampaignID != 0 {
			campaignID = traceCampaignID
		}
		if massMailingID := int64Value(traceRow["mass_mailing_id"]); massMailingID != 0 {
			values["mass_mailing_id"] = massMailingID
		}
	}
	_, whatsappExists, err := whatsappMessageRow(env, whatsappID)
	if err != nil {
		return 0, err
	}
	if whatsappExists {
		if modelHasField(env, "link.tracker.click", "whatsapp_message_id") {
			values["whatsapp_message_id"] = whatsappID
		}
		whatsappCampaignID, err := whatsappMarketingUTMCampaignID(env, whatsappID)
		if err != nil {
			return 0, err
		}
		if whatsappCampaignID != 0 {
			campaignID = whatsappCampaignID
		}
	}
	if campaignID != 0 {
		values["campaign_id"] = campaignID
	}
	clickID, err := env.Model("link.tracker.click").Create(values)
	if err != nil {
		return 0, err
	}
	if err := refreshLinkTrackerCount(env, linkID); err != nil {
		return 0, err
	}
	if traceExists {
		if err := markMailingTraceOpenedAndClicked(env, traceID, stringValue(traceRow["trace_status"]), now); err != nil {
			return 0, err
		}
	}
	if whatsappExists {
		if err := markWhatsAppMessageClicked(env, whatsappID, now); err != nil {
			return 0, err
		}
	}
	return clickID, nil
}

func mailingTraceRow(env *record.Env, traceID int64) (map[string]any, bool, error) {
	if env == nil || traceID == 0 {
		return nil, false, nil
	}
	if _, ok := env.ModelMetadata("mailing.trace"); !ok {
		return nil, false, nil
	}
	rows, err := env.Model("mailing.trace").Browse(traceID).Read("trace_status", "campaign_id", "mass_mailing_id")
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

func mailingTraceIDBySMSID(env *record.Env, smsID int64) (int64, error) {
	if env == nil || smsID == 0 || !modelHasField(env, "mailing.trace", "sms_id_int") {
		return 0, nil
	}
	found, err := env.Model("mailing.trace").SearchWithOptions(domain.Cond("sms_id_int", domain.Equal, smsID), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64Value(rows[0]["id"]), nil
}

func whatsappMessageRow(env *record.Env, whatsappID int64) (map[string]any, bool, error) {
	if env == nil || whatsappID == 0 {
		return nil, false, nil
	}
	if _, ok := env.ModelMetadata("whatsapp.message"); !ok {
		return nil, false, nil
	}
	rows, err := env.Model("whatsapp.message").Browse(whatsappID).Read("id")
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

func whatsappMarketingUTMCampaignID(env *record.Env, whatsappID int64) (int64, error) {
	if env == nil || whatsappID == 0 || !modelHasField(env, "marketing.trace", "whatsapp_message_id") {
		return 0, nil
	}
	traces, err := env.Model("marketing.trace").SearchWithOptions(domain.Cond("whatsapp_message_id", domain.Equal, whatsappID), record.SearchOptions{Limit: 1})
	if err != nil || traces.Len() == 0 {
		return 0, err
	}
	traceRows, err := traces.Read("activity_id")
	if err != nil || len(traceRows) == 0 {
		return 0, err
	}
	activityID := int64Value(traceRows[0]["activity_id"])
	if activityID == 0 || !modelHasField(env, "marketing.activity", "campaign_id") {
		return 0, nil
	}
	activityRows, err := env.Model("marketing.activity").Browse(activityID).Read("campaign_id")
	if err != nil || len(activityRows) == 0 {
		return 0, err
	}
	campaignID := int64Value(activityRows[0]["campaign_id"])
	if campaignID == 0 || !modelHasField(env, "marketing.campaign", "utm_campaign_id") {
		return 0, nil
	}
	campaignRows, err := env.Model("marketing.campaign").Browse(campaignID).Read("utm_campaign_id")
	if err != nil || len(campaignRows) == 0 {
		return 0, err
	}
	return int64Value(campaignRows[0]["utm_campaign_id"]), nil
}

func markWhatsAppMessageClicked(env *record.Env, whatsappID int64, now time.Time) error {
	if env == nil || whatsappID == 0 {
		return nil
	}
	if modelHasField(env, "whatsapp.message", "links_click_datetime") {
		if err := env.Model("whatsapp.message").Browse(whatsappID).Write(map[string]any{"links_click_datetime": now}); err != nil {
			return err
		}
	}
	if !modelHasField(env, "marketing.trace", "whatsapp_message_id") || !modelHasField(env, "marketing.trace", "links_click_datetime") {
		return nil
	}
	traces, err := env.Model("marketing.trace").Search(domain.Cond("whatsapp_message_id", domain.Equal, whatsappID))
	if err != nil || traces.Len() == 0 {
		return err
	}
	rows, err := traces.Read("id")
	if err != nil {
		return err
	}
	var traceIDs []int64
	for _, row := range rows {
		traceID := int64Value(row["id"])
		if traceID == 0 {
			continue
		}
		traceIDs = append(traceIDs, traceID)
		if err := env.Model("marketing.trace").Browse(traceID).Write(map[string]any{"links_click_datetime": now}); err != nil {
			return err
		}
	}
	return processWhatsAppClickChildTraces(env, traceIDs, now)
}

func processWhatsAppClickChildTraces(env *record.Env, parentTraceIDs []int64, now time.Time) error {
	if env == nil || len(parentTraceIDs) == 0 || !modelHasField(env, "marketing.trace", "parent_id") || !modelHasField(env, "marketing.trace", "state_msg") || !modelHasField(env, "marketing.activity", "trigger_type") {
		return nil
	}
	parentTraceIDs, err := marketingTraceEventAllowedIDs(env, parentTraceIDs)
	if err != nil || len(parentTraceIDs) == 0 {
		return err
	}
	children, err := env.Model("marketing.trace").Search(domain.And(
		domain.Cond("parent_id", domain.In, parentTraceIDs),
		domain.Cond("state", domain.Equal, "scheduled"),
	))
	if err != nil || children.Len() == 0 {
		return err
	}
	childRows, err := children.Read("id", "activity_id")
	if err != nil {
		return err
	}
	activityIDs := make([]int64, 0, len(childRows))
	for _, row := range childRows {
		if activityID := int64Value(row["activity_id"]); activityID != 0 {
			activityIDs = append(activityIDs, activityID)
		}
	}
	activityInfo, err := marketingActivityEventInfo(env, activityIDs)
	if err != nil {
		return err
	}
	for _, row := range childRows {
		info := activityInfo[int64Value(row["activity_id"])]
		switch info.Trigger {
		case "whatsapp_click":
			if err := scheduleMarketingEventChildTrace(env, int64Value(row["id"]), info, now); err != nil {
				return err
			}
		case "whatsapp_not_click":
			if err := env.Model("marketing.trace").Browse(int64Value(row["id"])).Write(map[string]any{
				"state":         "canceled",
				"schedule_date": now,
				"state_msg":     "Parent Whatsapp message was clicked",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

type marketingActivityEvent struct {
	Trigger        string
	IntervalNumber int64
	IntervalType   string
}

func marketingActivityEventInfo(env *record.Env, activityIDs []int64) (map[int64]marketingActivityEvent, error) {
	info := map[int64]marketingActivityEvent{}
	if env == nil || len(activityIDs) == 0 {
		return info, nil
	}
	fields := []string{"id", "trigger_type"}
	if modelHasField(env, "marketing.activity", "interval_number") {
		fields = append(fields, "interval_number")
	}
	if modelHasField(env, "marketing.activity", "interval_type") {
		fields = append(fields, "interval_type")
	}
	activities, err := env.Model("marketing.activity").Search(domain.Cond("id", domain.In, activityIDs))
	if err != nil || activities.Len() == 0 {
		return info, err
	}
	rows, err := activities.Read(fields...)
	if err != nil {
		return info, err
	}
	for _, row := range rows {
		intervalType := strings.TrimSpace(stringValue(row["interval_type"]))
		if intervalType == "" {
			intervalType = "hours"
		}
		info[int64Value(row["id"])] = marketingActivityEvent{
			Trigger:        stringValue(row["trigger_type"]),
			IntervalNumber: int64Value(row["interval_number"]),
			IntervalType:   intervalType,
		}
	}
	return info, nil
}

func scheduleMarketingEventChildTrace(env *record.Env, traceID int64, activity marketingActivityEvent, now time.Time) error {
	values := map[string]any{"schedule_date": marketingActivityEventDate(activity, now)}
	if activity.IntervalNumber == 0 {
		values["state"] = "processed"
	}
	return env.Model("marketing.trace").Browse(traceID).Write(values)
}

func marketingActivityEventDate(activity marketingActivityEvent, now time.Time) time.Time {
	switch strings.TrimSpace(activity.IntervalType) {
	case "months":
		return now.AddDate(0, int(activity.IntervalNumber), 0)
	case "weeks":
		return now.AddDate(0, 0, int(activity.IntervalNumber)*7)
	case "days":
		return now.AddDate(0, 0, int(activity.IntervalNumber))
	default:
		return now.Add(time.Duration(activity.IntervalNumber) * time.Hour)
	}
}

func marketingTraceEventAllowedIDs(env *record.Env, traceIDs []int64) ([]int64, error) {
	allowedIDs := make([]int64, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		allowed, err := marketingTraceEventAllowed(env, traceID)
		if err != nil {
			return nil, err
		}
		if allowed {
			allowedIDs = append(allowedIDs, traceID)
		}
	}
	return allowedIDs, nil
}

func marketingTraceEventAllowed(env *record.Env, traceID int64) (bool, error) {
	if env == nil || traceID == 0 || !modelHasField(env, "marketing.trace", "activity_id") || !modelHasField(env, "marketing.activity", "campaign_id") || !modelHasField(env, "marketing.campaign", "state") {
		return true, nil
	}
	traceRows, err := env.Model("marketing.trace").Browse(traceID).Read("activity_id")
	if err != nil || len(traceRows) == 0 {
		return true, err
	}
	activityID := int64Value(traceRows[0]["activity_id"])
	if activityID == 0 {
		return true, nil
	}
	activityRows, err := env.Model("marketing.activity").Browse(activityID).Read("campaign_id")
	if err != nil || len(activityRows) == 0 {
		return true, err
	}
	campaignID := int64Value(activityRows[0]["campaign_id"])
	if campaignID == 0 {
		return true, nil
	}
	campaignRows, err := env.Model("marketing.campaign").Browse(campaignID).Read("state")
	if err != nil || len(campaignRows) == 0 {
		return true, err
	}
	switch strings.TrimSpace(stringValue(campaignRows[0]["state"])) {
	case "", "draft", "running":
		return true, nil
	default:
		return false, nil
	}
}

func markMailingTraceOpenedAndClicked(env *record.Env, traceID int64, status string, now time.Time) error {
	values := map[string]any{"links_click_datetime": now}
	if !shouldSkipMailingTraceOpen(status) {
		values["trace_status"] = "open"
		values["open_datetime"] = now
	}
	return env.Model("mailing.trace").Browse(traceID).Write(values)
}

func shouldSkipMailingTraceOpen(status string) bool {
	status = strings.TrimSpace(status)
	return status == "open" || status == "reply"
}

func modelHasField(env *record.Env, modelName string, fieldName string) bool {
	if env == nil {
		return false
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return false
	}
	_, ok = meta.Fields[fieldName]
	return ok
}

func refreshLinkTrackerCount(env *record.Env, linkID int64) error {
	meta, ok := env.ModelMetadata("link.tracker")
	if !ok {
		return nil
	}
	if _, ok := meta.Fields["count"]; !ok {
		return nil
	}
	found, err := env.Model("link.tracker.click").Search(domain.Cond("link_id", domain.Equal, linkID))
	if err != nil {
		return err
	}
	return env.Model("link.tracker").Browse(linkID).Write(map[string]any{"count": int64(found.Len())})
}

func linkTrackerRedirectedURLHTTP(env *record.Env, rawURL string, row map[string]any) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if configParameterBool(env, "link_tracker.no_external_tracking") {
		baseParsed, _ := url.Parse(configParameterValue(env, "web.base.url"))
		if parsed.Host != "" && baseParsed != nil && parsed.Host != baseParsed.Host {
			return parsed.String()
		}
	}
	query := parsed.Query()
	if value := linkTrackerUTMNameHTTP(env, "utm.campaign", int64Value(row["campaign_id"])); value != "" {
		query.Set("utm_campaign", value)
	}
	if value := linkTrackerUTMNameHTTP(env, "utm.source", int64Value(row["source_id"])); value != "" {
		query.Set("utm_source", value)
	}
	if value := linkTrackerUTMNameHTTP(env, "utm.medium", int64Value(row["medium_id"])); value != "" {
		query.Set("utm_medium", value)
	}
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "...", "%2E%2E%2E")
	return parsed.String()
}

func linkTrackerUTMNameHTTP(env *record.Env, modelName string, id int64) string {
	if env == nil || id == 0 {
		return ""
	}
	rows, err := env.Model(modelName).Browse(id).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringValue(rows[0]["name"]))
}

func requestCountryIDHTTP(env *record.Env, r *http.Request) (int64, error) {
	if env == nil || r == nil {
		return 0, nil
	}
	code := requestCountryCodeHTTP(r)
	if code == "" || !modelHasField(env, "res.country", "code") {
		return 0, nil
	}
	found, err := env.Model("res.country").SearchWithOptions(domain.Cond("code", domain.Equal, code), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64Value(rows[0]["id"]), nil
}

func requestCountryCodeHTTP(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"X-GeoIP-Country-Code", "X-Odoo-GeoIP-Country-Code", "CF-IPCountry", "X-Country-Code"} {
		code := strings.ToUpper(strings.TrimSpace(r.Header.Get(header)))
		if code != "" && code != "--" && len(code) == 2 {
			return code
		}
	}
	return ""
}

func massMailingBlankGIF() []byte {
	data, _ := base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAANvf7wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw==")
	return data
}

func pythonReprHTTPString(value string) string {
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

func (s Server) aiGenerateResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req aicontrollers.GenerateResponseRequest
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	service := s.aiChatService(env)
	if service == nil {
		writeRPCError(w, envelope, http.StatusNotFound, aicontrollers.ErrChannelNotFound)
		return
	}
	if env != nil {
		req.UserID = env.Context().UserID
		req.CompanyID = env.Context().CompanyID
	}
	_, err = service.GenerateResponse(r.Context(), req)
	if err != nil {
		writeRPCError(w, envelope, aiHTTPStatus(err), err)
		return
	}
	writeRPCOrJSON(w, envelope, nil)
}

func (s Server) aiCloseChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ChannelID int64 `json:"channel_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	service := s.aiChatService(env)
	if service == nil {
		writeRPCError(w, envelope, http.StatusNotFound, aicontrollers.ErrChannelNotFound)
		return
	}
	_, err = service.CloseAIChat(r.Context(), req.ChannelID)
	if err != nil {
		writeRPCError(w, envelope, aiHTTPStatus(err), err)
		return
	}
	writeRPCOrJSON(w, envelope, nil)
}

func (s Server) aiTranscriptionSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Language string `json:"language"`
		Prompt   string `json:"prompt"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	if env == nil || env.Context().UserID == 0 {
		writeRPCError(w, envelope, http.StatusForbidden, errors.New("authentication required"))
		return
	}
	if s.AITranscript == nil {
		writeRPCError(w, envelope, http.StatusNotFound, aicontrollers.ErrProviderMissing)
		return
	}
	result, err := s.AITranscript.CreateSession(r.Context(), req.Language, req.Prompt)
	if err != nil {
		writeRPCError(w, envelope, aiHTTPStatus(err), err)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) aiChatService(env *record.Env) *aicontrollers.ChatService {
	if s.AIChatFactory != nil {
		return s.AIChatFactory(env)
	}
	return s.AIChat
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Params  json.RawMessage `json:"params"`
}

type callKWRequest struct {
	Model  string         `json:"model"`
	Method string         `json:"method"`
	Args   []any          `json:"args"`
	Kwargs map[string]any `json:"kwargs"`
	Values map[string]any `json:"values"`
}

func (s Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s Server) webClient(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web" && r.URL.Path != "/web/" && r.URL.Path != "/odoo" && r.URL.Path != "/odoo/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, s.webClientShellHTML(r))
}

func (s Server) cleanRoomEnterpriseBackground(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web_enterprise/static/img/background-cleanroom.svg" && r.URL.Path != "/web_enterprise/static/img/background-dark.jpg" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		if strings.HasSuffix(r.URL.Path, ".jpg") {
			w.Header().Set("Content-Type", "image/jpeg")
		} else {
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		}
		return
	}
	if strings.HasSuffix(r.URL.Path, ".jpg") {
		w.Header().Set("Content-Type", "image/jpeg")
		writeCleanRoomEnterpriseBackgroundJPEG(w)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = io.WriteString(w, cleanRoomEnterpriseBackgroundSVG)
}

func writeCleanRoomEnterpriseBackgroundJPEG(w io.Writer) {
	const width = 1920
	const height = 1080
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		ny := float64(y) / float64(height-1)
		for x := 0; x < width; x++ {
			nx := float64(x) / float64(width-1)
			base := 4 + 8*ny
			leftField := math.Exp(-math.Pow(nx-.26, 2)/0.20 - math.Pow(ny-.20, 2)/0.30)
			tealBand := math.Exp(-math.Pow(ny-(0.16+0.12*math.Sin(nx*math.Pi*1.05)), 2) / 0.052)
			rightVeil := math.Exp(-math.Pow(nx-.80, 2)/0.14 - math.Pow(ny-.42, 2)/0.34)
			lowerBand := math.Exp(-math.Pow(ny-(0.86-0.10*math.Sin((nx+.12)*math.Pi*1.1)), 2) / 0.032)
			vignette := 1 - 0.22*math.Hypot(nx-.52, ny-.44)
			if vignette < .78 {
				vignette = .78
			}
			if nx < .14+.18*ny {
				vignette *= .72
			}
			noise := float64(((x*37 + y*17 + x*y*3) % 11) - 5)
			red := (base + 3*leftField + 2*tealBand + 3*lowerBand + 14*rightVeil + noise*.18) * vignette
			green := (base + 22*leftField + 24*tealBand + 12*lowerBand + 6*rightVeil + noise*.22) * vignette
			blue := (17 + 34*leftField + 32*tealBand + 20*lowerBand + 31*rightVeil + noise*.22) * vignette
			img.SetRGBA(x, y, color.RGBA{R: byte(clampByte(red)), G: byte(clampByte(green)), B: byte(clampByte(blue)), A: 255})
		}
	}
	_ = jpeg.Encode(w, img, &jpeg.Options{Quality: 86})
}

func clampByte(value float64) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return int(value + .5)
}

func (s Server) webClientShellHTML(r *http.Request) string {
	entry := "apps/webclient/src/main.js"
	filename, ok := s.frontendDistFile(entry)
	if !ok || legacyWebClientRequested(r) {
		return webClientShellHTML
	}
	src := "/web/static/frontend/" + entry
	if version := frontendDistAssetVersion(filename); version != "" {
		src += "?v=" + url.QueryEscape(version)
	}
	script := `<script>globalThis.__goerpTSWebClientAvailable = true;</script>` + "\n" +
		fmt.Sprintf(`<script type="module" src="%s"></script>`, src)
	return strings.Replace(webClientShellHTML, "</body>", script+"\n</body>", 1)
}

const cleanRoomEnterpriseBackgroundSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080" preserveAspectRatio="none"><rect width="1920" height="1080" fill="#070b12"/><path d="M-120 0 C320 70 640 210 940 420 C1240 630 1540 560 2040 340 L2040 -120 L-120 -120 Z" fill="#17313d" opacity=".46"/><path d="M-220 900 C260 740 670 880 1040 1010 C1390 1132 1650 980 2040 920 L2040 1200 L-220 1200 Z" fill="#0e2634" opacity=".62"/><path d="M1340 -160 C1510 130 1600 340 1920 470" fill="none" stroke="#242337" stroke-width="190" opacity=".32"/><path d="M-120 440 C340 300 760 380 1120 520 C1460 650 1720 520 2040 440" fill="none" stroke="#111827" stroke-width="180" opacity=".38"/><g fill="#334155" opacity=".42"><circle cx="225" cy="260" r="10"/><circle cx="570" cy="835" r="7"/><circle cx="1125" cy="395" r="8"/><circle cx="1720" cy="225" r="7"/><circle cx="1830" cy="790" r="12"/></g></svg>`

func frontendDistAssetVersion(filename string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

func legacyWebClientRequested(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("legacy_webclient") == "1" || query.Get("ts_webclient") == "0"
}

func (s Server) frontendAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/web/static/frontend/")
	filename, ok := s.frontendDistFile(rel)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filename)
}

func (s Server) frontendDistFile(rel string) (string, bool) {
	root, ok := s.frontendDistRoot()
	if !ok {
		return "", false
	}
	filename, ok := safeFrontendDistPath(root, rel)
	if !ok {
		return "", false
	}
	info, err := os.Stat(filename)
	if err != nil || info.IsDir() {
		return "", false
	}
	return filename, true
}

func (s Server) frontendDistRoot() (string, bool) {
	candidates := []string{}
	if strings.TrimSpace(s.FrontendDist) != "" {
		candidates = append(candidates, s.FrontendDist)
	}
	candidates = append(candidates, "frontend/dist", "current/frontend/dist", "/opt/goerp/current/frontend/dist")
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func safeFrontendDistPath(root string, rel string) (string, bool) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, clean))
	if err != nil {
		return "", false
	}
	inside, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", false
	}
	return targetAbs, true
}

const webClientShellHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Odoo</title>
	<style>
	:root {
		--bg: #1b1d27;
		--panel: #282a35;
		--panel-soft: #343743;
		--text: #f5f5f7;
		--muted: #b0b5c4;
		--line: #3c404e;
		--line-soft: #303440;
		--hover-bg: #353846;
		--control-bg: #282a35;
		--control-shadow: 0 1px 0 rgba(0,0,0,.18);
		--dropdown-shadow: 0 14px 28px rgba(0,0,0,.46);
		--list-head: #1b1d27;
		--btn-secondary-bg: #333744;
		--btn-secondary-hover: #414454;
		--accent: #875a7b;
		--accent-2: #00a09d;
		--accent-text: #ffffff;
		--danger: #b42318;
		--radius: 3px;
		--topbar: #282a35;
		--topbar-hover: #353846;
		--sidebar: #1b1d27;
			--home-bg: #070b12;
			--home-bg-image: url("data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxOTIwIDEwODAiIHByZXNlcnZlQXNwZWN0UmF0aW89InhNaWRZTWlkIHNsaWNlIj48cmVjdCB3aWR0aD0iMTkyMCIgaGVpZ2h0PSIxMDgwIiBmaWxsPSIjMDcwYjEyIi8+PHBhdGggZD0iTS0xMjAgMCBDMzIwIDcwIDY0MCAyMTAgOTQwIDQyMCBDMTI0MCA2MzAgMTU0MCA1NjAgMjA0MCAzNDAgTDIwNDAgLTEyMCBMLTEyMCAtMTIwIFoiIGZpbGw9IiMxNzMxM2QiIG9wYWNpdHk9Ii40NiIvPjxwYXRoIGQ9Ik0tMjIwIDkwMCBDMjYwIDc0MCA2NzAgODgwIDEwNDAgMTAxMCBDMTM5MCAxMTMyIDE2NTAgOTgwIDIwNDAgOTIwIEwyMDQwIDEyMDAgTC0yMjAgMTIwMCBaIiBmaWxsPSIjMGUyNjM0IiBvcGFjaXR5PSIuNjIiLz48cGF0aCBkPSJNMTM0MCAtMTYwIEMxNTEwIDEzMCAxNjAwIDM0MCAxOTIwIDQ3MCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjMjQyMzM3IiBzdHJva2Utd2lkdGg9IjE5MCIgb3BhY2l0eT0iLjMyIi8+PHBhdGggZD0iTS0xMjAgNDQwIEMzNDAgMzAwIDc2MCAzODAgMTEyMCA1MjAgQzE0NjAgNjUwIDE3MjAgNTIwIDIwNDAgNDQwIiBmaWxsPSJub25lIiBzdHJva2U9IiMxMTE4MjciIHN0cm9rZS13aWR0aD0iMTgwIiBvcGFjaXR5PSIuMzgiLz48ZyBmaWxsPSIjMzM0MTU1IiBvcGFjaXR5PSIuNDIiPjxjaXJjbGUgY3g9IjIyNSIgY3k9IjI2MCIgcj0iMTAiLz48Y2lyY2xlIGN4PSI1NzAiIGN5PSI4MzUiIHI9IjciLz48Y2lyY2xlIGN4PSIxMTI1IiBjeT0iMzk1IiByPSI4Ii8+PGNpcmNsZSBjeD0iMTcyMCIgY3k9IjIyNSIgcj0iNyIvPjxjaXJjbGUgY3g9IjE4MzAiIGN5PSI3OTAiIHI9IjEyIi8+PC9nPjwvc3ZnPg==");
			--home-panel: rgba(255,255,255,.08);
			--home-line: rgba(255,255,255,.14);
			--home-text: #ffffff;
			--home-muted: #d7dce6;
	}
	body[data-theme="standard"] {
		--bg: #f5f5f5;
		--accent: #017e84;
		--topbar: #2b3940;
		--topbar-hover: #1f2b31;
		--text: #1f2933;
	}
	* { box-sizing: border-box; }
	html, body { min-height: 100%; width: 100%; overflow-x: hidden; }
	body {
		margin: 0;
		background: var(--bg);
		color: var(--text);
		font: 14px/1.45 "Segoe UI", Roboto, Arial, sans-serif;
	}
	button, input, select, textarea {
		font: inherit;
	}
	button {
		border: 1px solid var(--accent);
		border-radius: var(--radius);
		background: var(--accent);
		color: var(--accent-text);
		min-height: 30px;
		padding: 6px 10px;
		cursor: pointer;
		font-weight: 500;
		transition: background-color 160ms ease, border-color 160ms ease, transform 80ms ease;
	}
	button:hover {
		background: var(--topbar-hover);
		border-color: var(--topbar-hover);
	}
	button:active {
		transform: translateY(1px);
	}
	button.secondary {
		background: var(--btn-secondary-bg);
		color: var(--text);
		border-color: var(--line);
	}
	button.secondary:hover {
		background: var(--btn-secondary-hover);
		border-color: #c7ccd3;
	}
	.btn-secondary,
	.btn-outline-secondary {
		background: var(--btn-secondary-bg);
		color: var(--text);
		border-color: var(--line);
	}
	.btn-secondary:hover,
	.btn-outline-secondary:hover {
		background: var(--btn-secondary-hover);
		border-color: #c7ccd3;
		color: var(--text);
	}
	button:disabled {
		cursor: default;
		opacity: .65;
		transform: none;
	}
	input, select, textarea {
		width: 100%;
		border: 1px solid var(--line);
		border-radius: var(--radius);
		background: #fff;
		color: var(--text);
		padding: 6px 8px;
		outline: none;
	}
	input:focus, select:focus, textarea:focus, button:focus-visible {
		border-color: var(--accent-2);
		box-shadow: 0 0 0 2px rgba(1, 126, 132, .15);
	}
	textarea {
		min-height: 130px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		resize: vertical;
	}
	header {
		display: flex;
		gap: 14px;
		align-items: center;
		min-height: 46px;
		padding: 0 14px;
		background: var(--topbar);
		color: #fff;
		border-bottom: 1px solid rgba(0,0,0,.14);
		box-shadow: 0 1px 2px rgba(16, 24, 40, .12);
	}
	header.o_navbar {
		display: block;
		min-height: 46px;
		height: 46px;
		padding: 0;
		background: transparent;
		border-bottom: 0;
		box-shadow: none;
	}
	.o_navbar > .o_main_navbar {
		display: flex;
		flex-wrap: nowrap;
		gap: 14px;
		align-items: center;
		height: 46px;
		min-height: 46px;
		max-height: 46px;
		padding: 0 14px;
		background: var(--topbar);
		color: #fff;
		border-bottom: 1px solid rgba(0,0,0,.14);
		box-shadow: 0 1px 2px rgba(16, 24, 40, .12);
		overflow: visible;
	}
	h1, h2, h3, p { margin: 0; }
	h1 {
		font-size: 17px;
		font-weight: 600;
		line-height: 1;
	}
	h2 { font-size: 17px; font-weight: 500; margin-bottom: 10px; }
	a { color: var(--accent); }
	.muted { color: var(--muted); }
	header .muted { color: rgba(255,255,255,.72); }
	header label {
		color: rgba(255,255,255,.82);
		min-width: 154px;
	}
	header select {
		height: 30px;
		border-color: rgba(255,255,255,.26);
		background: rgba(255,255,255,.12);
		color: #fff;
	}
	header option { color: #1f2933; }
	.layout {
		display: grid;
		grid-template-columns: 236px 1fr;
		min-height: calc(100vh - 46px);
	}
	aside {
		padding: 14px 12px;
		border-right: 1px solid var(--line);
		background: var(--sidebar);
	}
	main {
		padding: 0;
		display: grid;
		gap: 0;
		align-content: start;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0;
		background: #fff;
		border-bottom: 1px solid var(--line);
	}
	.panel {
		background: var(--panel);
		border: 0;
		border-bottom: 1px solid var(--line);
		border-radius: 0;
		padding: 14px 18px;
	}
	.card {
		background: #fff;
		border: 0;
		border-right: 1px solid var(--line);
		border-radius: 0;
		padding: 10px 18px;
		min-height: 60px;
	}
	.card strong {
		display: block;
		font-size: 18px;
		font-weight: 500;
		margin-top: 3px;
		font-variant-numeric: tabular-nums;
	}
	.toolbar {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: end;
		padding: 9px 0 10px;
	}
    .field { min-width: 150px; flex: 1; }
    .field.small { max-width: 110px; }
    label {
      display: grid;
      gap: 4px;
      color: var(--muted);
      font-size: 12px;
    }
	table {
		width: 100%;
		border-collapse: collapse;
		margin-top: 10px;
		background: #fff;
	}
	th, td {
		border-bottom: 1px solid var(--line-soft);
		padding: 8px 10px;
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		background: var(--list-head);
	}
	tr:hover td { background: var(--hover-bg); }
	pre {
      margin: 0;
      overflow: auto;
      max-height: 420px;
      padding: 10px;
      border-radius: 4px;
      border: 1px solid var(--line);
      background: #101828;
      color: #e7edf7;
      font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .status-ok { color: var(--accent); }
    .status-error { color: var(--danger); }
	.login {
      display: none;
      margin-top: 14px;
      padding-top: 14px;
      border-top: 1px solid var(--line);
      gap: 8px;
    }
    .login.active { display: grid; }
	ul { list-style: none; padding: 0; margin: 10px 0 0; display: grid; gap: 6px; }
	li {
      display: flex;
      justify-content: space-between;
      gap: 8px;
      border-bottom: 1px solid var(--line);
      padding-bottom: 6px;
    }
	.badge {
      display: inline-flex;
      align-items: center;
      width: fit-content;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 2px 8px;
      color: var(--muted);
      font-size: 12px;
    }
	.text-bg-success { color: #0f5132; background: #d1e7dd; border-color: #badbcc; }
	.text-bg-info { color: #055160; background: #cff4fc; border-color: #b6effb; }
	.text-bg-warning { color: #664d03; background: #fff3cd; border-color: #ffecb5; }
	.text-bg-danger { color: #842029; background: #f8d7da; border-color: #f5c2c7; }
	.text-bg-primary { color: #084298; background: #cfe2ff; border-color: #b6d4fe; }
	.text-bg-300, .text-bg-muted, .text-bg-secondary { color: #41464b; background: #e2e3e5; border-color: #d3d6d8; }
	.o_field_many2one_avatar {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		min-width: 0;
	}
	.o_m2o_avatar {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		object-fit: cover;
		background: #d8dadd;
	}
	.o_field_many2one_avatar_name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.o-mail-Chatter {
		margin-top: 18px;
		border-top: 1px solid var(--line);
		padding-top: 12px;
	}
	.o-mail-Composer {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		margin: 8px 0;
	}
	.o-mail-Thread {
		display: grid;
		gap: 10px;
	}
	.o-mail-Message {
		display: grid;
		grid-template-columns: 32px minmax(0, 1fr);
		gap: 10px;
		padding: 8px 0;
		border-top: 1px solid var(--line);
	}
	.o-mail-Message:first-child {
		border-top: 0;
	}
	.o-mail-Message-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		object-fit: cover;
		background: var(--panel-soft);
	}
	.o-mail-Message-meta {
		display: flex;
		align-items: baseline;
		gap: 8px;
		font-size: 12px;
		color: var(--muted);
	}
	.o-mail-Message-author {
		font-weight: 600;
		color: var(--text);
	}
	.o-mail-Message-body {
		margin-top: 3px;
		white-space: pre-wrap;
	}
	.o-mail-AttachmentList,
	.o-mail-ReactionList {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 6px;
	}
	.o-mail-Attachment,
	.o-mail-Reaction {
		border: 1px solid var(--line);
		border-radius: var(--radius);
		padding: 2px 6px;
		background: var(--panel-soft);
		font-size: 12px;
	}
	.menu-list {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 8px;
    }
	.menu-list .o_menuitem {
		background: var(--panel);
		color: var(--text);
		border-color: var(--line);
	}
	.menu-list .o_menu_section {
		background: var(--panel-soft);
		color: var(--muted);
	}
	.o_app_menu_path {
		display: block;
		margin-top: 2px;
		color: var(--muted);
		font-size: 11px;
		font-weight: 500;
		line-height: 1.2;
	}
	.module-grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 10px;
		margin-top: 8px;
	}
	.module-card {
		border: 1px solid var(--line);
		border-radius: var(--radius);
      padding: 10px;
      display: grid;
      gap: 8px;
      background: var(--panel);
    }
    .record-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
      margin-top: 10px;
    }
    .record-actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      margin-top: 10px;
    }
	.open-cell { width: 1%; white-space: nowrap; }
	.o-brand {
		display: inline-flex;
		align-items: center;
		gap: 9px;
		min-width: 132px;
	}
	.o-launcher-button {
		display: inline-grid;
		place-items: center;
		width: 36px;
		height: 46px;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
	}
	.o_menu_toggle {
		color: #fff;
		text-decoration: none;
	}
	.o-launcher-button:hover,
	.o-launcher-button.active {
		background: rgba(0,0,0,.18);
	}
	.o-launcher {
		display: inline-grid;
		grid-template-columns: repeat(3, 4px);
		gap: 3px;
		width: 20px;
		height: 20px;
		align-content: center;
		position: relative;
	}
	.o-launcher span {
		width: 4px;
		height: 4px;
		background: rgba(255,255,255,.88);
		border-radius: 1px;
	}
	.o_menu_toggle_back .o-launcher {
		display: inline-block;
	}
	.o_menu_toggle_back .o-launcher span {
		display: none;
	}
	.o_menu_toggle_back .o-launcher::before,
	.o_menu_toggle_back .o-launcher::after {
		content: "";
		position: absolute;
		left: 2px;
		top: 9px;
		width: 16px;
		height: 2px;
		border-radius: 1px;
		background: currentColor;
		transform-origin: center;
	}
	.o_menu_toggle_back .o-launcher::before {
		transform: rotate(45deg);
	}
	.o_menu_toggle_back .o-launcher::after {
		transform: rotate(-45deg);
	}
	.o_menu_brand {
		display: inline-flex;
		align-items: center;
	}
	.o-nav {
		display: flex;
		flex-wrap: nowrap;
		gap: 3px;
		flex: 1;
		align-self: stretch;
		min-width: 0;
		overflow: hidden;
	}
	.o-nav:empty {
		display: none;
	}
	.o-nav button {
		height: 46px;
		min-height: 46px;
		padding: 0 10px;
		background: transparent;
		border-color: transparent;
		color: rgba(255,255,255,.9);
		white-space: nowrap;
	}
	.o-nav button:hover {
		background: rgba(255,255,255,.12);
	}
	.o-nav button.active,
	.o-nav button[aria-current="page"] {
		background: rgba(0,0,0,.18);
		color: #fff;
	}
	.o-search {
		max-width: 320px;
	}
	.o-search input {
		height: 30px;
		border-color: rgba(255,255,255,.22);
		background: rgba(255,255,255,.14);
		color: #fff;
	}
	.o-search input::placeholder { color: rgba(255,255,255,.7); }
		.o-menu-systray {
			display: inline-flex;
			align-items: stretch;
			align-self: stretch;
			height: 46px;
			max-height: 46px;
			margin-left: auto;
			min-width: 0;
			position: relative;
			overflow: visible;
		}
	.o-systray-item {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-width: 34px;
		min-height: 46px;
		padding: 0 9px;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: rgba(255,255,255,.9);
		white-space: nowrap;
	}
	.o-systray-item:hover,
	.o-systray-item:focus-visible {
		background: rgba(0,0,0,.16);
		color: #fff;
	}
	.o-systray-icon {
		display: inline-grid;
		place-items: center;
		position: relative;
		width: 18px;
		height: 18px;
		border: 0;
		border-radius: 0;
		font-style: normal;
		font-size: 13px;
		font-weight: 700;
		line-height: 1;
	}
	.o-systray-icon.oi-discuss::before {
		content: "";
		position: absolute;
		inset: 3px 2px 5px;
		border-radius: 9px;
		background: currentColor;
	}
	.o-systray-icon.oi-discuss::after {
		content: "";
		position: absolute;
		left: 6px;
		bottom: 2px;
		width: 6px;
		height: 6px;
		background: currentColor;
		clip-path: polygon(0 0, 100% 0, 0 100%);
	}
	.o-systray-icon.oi-clock::before {
		content: "";
		position: absolute;
		inset: 2px;
		border: 2px solid currentColor;
		border-radius: 50%;
	}
	.o-systray-icon.oi-clock::after {
		content: "";
		position: absolute;
		left: 8px;
		top: 5px;
		width: 5px;
		height: 5px;
		border-left: 2px solid currentColor;
		border-bottom: 2px solid currentColor;
		transform-origin: left bottom;
	}
		.o-systray-counter {
			min-width: 16px;
			height: 16px;
			border-radius: 999px;
			background: rgba(255,255,255,.2);
		padding: 0 5px;
		font-size: 11px;
			line-height: 16px;
			text-align: center;
		}
		.o-menu-systray .dropdown-menu {
			position: absolute;
			top: 100%;
			right: 0;
			z-index: 1050;
			display: none;
			min-width: 180px;
			margin: 0;
			padding: 6px 0;
			background: #fff;
			border: 1px solid var(--line);
			border-radius: 4px;
			box-shadow: var(--dropdown-shadow);
			color: #1f2933;
		}
		.o-menu-systray .dropdown-menu.show {
			display: block;
		}
		.o_menu_sections {
			position: relative;
		}
		.o_menu_sections .o_nav_entry {
			position: relative;
		}
		.o_menu_sections .o_nav_dropdown_toggle::after {
			content: "";
			display: inline-block;
			width: 0;
			height: 0;
			margin-left: 6px;
			border-left: 4px solid transparent;
			border-right: 4px solid transparent;
			border-top: 4px solid currentColor;
			vertical-align: middle;
		}
		.o_menu_sections .o_navbar_dropdown_menu {
			position: absolute;
			top: 100%;
			left: 0;
			z-index: 1050;
			display: none;
			min-width: 230px;
			max-height: min(70vh, 520px);
			margin: 0;
			padding: 6px 0;
			overflow: auto;
			background: #fff;
			border: 1px solid var(--line);
			border-radius: 4px;
			box-shadow: var(--dropdown-shadow);
			color: #1f2933;
		}
		.o_menu_sections .o_navbar_dropdown_menu.show {
			display: block;
		}
		.o_navbar_dropdown_group {
			position: relative;
		}
		.o_navbar_dropdown_header {
			display: block;
			padding: 8px 14px 4px;
			color: #6f7682;
			font-size: 12px;
			font-weight: 600;
			text-transform: uppercase;
			white-space: nowrap;
		}
		.o_navbar_dropdown_item {
			display: block;
			width: 100%;
			min-height: 32px;
			padding: 7px 14px;
			border: 0;
			background: transparent;
			color: #1f2933;
			font: inherit;
			text-align: left;
			white-space: nowrap;
		}
		.o_navbar_submenu_toggle {
			padding-right: 28px;
		}
		.o_navbar_submenu_toggle::after {
			position: absolute;
			right: 12px;
			top: 50%;
			width: 6px;
			height: 6px;
			margin: -3px 0 0;
			border-right: 1px solid currentColor;
			border-bottom: 1px solid currentColor;
			content: "";
			transform: rotate(-45deg);
		}
		.o_navbar_submenu_menu {
			position: absolute;
			top: -6px;
			left: calc(100% - 2px);
			z-index: 1051;
			display: none;
			min-width: 230px;
			max-height: min(70vh, 520px);
			margin: 0;
			padding: 6px 0;
			overflow: auto;
			background: #fff;
			border: 1px solid var(--line);
			border-radius: 4px;
			box-shadow: var(--dropdown-shadow);
			color: #1f2933;
		}
		.o_navbar_submenu_menu.show {
			display: block;
		}
		.o_navbar_dropdown_item[data-menu-level="1"] { padding-left: 26px; }
		.o_navbar_dropdown_item[data-menu-level="2"] { padding-left: 38px; }
		.o_navbar_dropdown_item[data-menu-level="3"] { padding-left: 50px; }
		.o_navbar_dropdown_item:hover,
		.o_navbar_dropdown_item:focus,
		.o_navbar_dropdown_item.active {
			background: var(--hover-bg);
			color: var(--accent);
		}
		.o-menu-systray .dropdown-item {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			column-gap: 12px;
			row-gap: 2px;
			align-items: center;
			width: 100%;
			min-height: 32px;
			padding: 7px 14px;
			border: 0;
			background: transparent;
			color: #1f2933;
			font: inherit;
			text-align: left;
			white-space: nowrap;
		}
		.o-menu-systray .dropdown-item.active {
			font-weight: 600;
			background: var(--hover-bg);
			color: var(--accent);
		}
		.o_systray_menu_label {
			min-width: 0;
			overflow: hidden;
			text-overflow: ellipsis;
		}
		.o_systray_menu_badge {
			min-width: 20px;
			padding: 1px 6px;
			border-radius: 999px;
			background: #eef0f3;
			color: var(--muted);
			font-size: 11px;
			line-height: 16px;
			text-align: center;
		}
		.o_systray_menu_description {
			grid-column: 1 / -1;
			color: var(--muted);
			font-size: 12px;
			line-height: 1.25;
			text-overflow: ellipsis;
			white-space: normal;
		}
		.o-menu-systray .dropdown-item:hover,
		.o-menu-systray .dropdown-item:focus-visible {
			background: var(--hover-bg);
		}
	.o-company-switcher span,
	.o-user-menu-button span {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.o-company-switcher {
		max-width: 180px;
	}
	.o-user-menu-button {
		max-width: 190px;
	}
	.o-mobile-menu-toggle {
		display: none;
		place-items: center;
		width: 36px;
		height: 46px;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
	}
	.o-mobile-menu-toggle span,
	.o-mobile-menu-toggle::before,
	.o-mobile-menu-toggle::after {
		content: "";
		display: block;
		width: 17px;
		height: 2px;
		background: rgba(255,255,255,.9);
		border-radius: 2px;
	}
	.o-mobile-menu-toggle {
		gap: 4px;
	}
	.o-sidebar-title {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 8px;
	}
	.o-sidebar-title h2 {
		margin: 0;
		font-size: 14px;
	}
	.o-debug-link {
		display: inline-block;
		margin-top: 14px;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
	.visually-hidden,
	.o_search_hidden {
		position: absolute !important;
		width: 1px !important;
		height: 1px !important;
		padding: 0 !important;
		margin: -1px !important;
		overflow: hidden !important;
		clip: rect(0, 0, 0, 0) !important;
		white-space: nowrap !important;
		border: 0 !important;
	}
	header {
		position: sticky;
		top: 0;
		z-index: 20;
		height: 46px;
		padding: 0 10px;
		gap: 8px;
	}
	.o-brand {
		min-width: 104px;
	}
	.o-brand h1 {
		font-size: 18px;
		font-weight: 500;
	}
	.o-nav {
		flex: none;
		height: 100%;
		align-items: stretch;
		min-width: 0;
	}
	.o-nav button {
		border-radius: 0;
		padding: 0 12px;
		border: 0;
		min-width: 0;
	}
	.o-nav button.active {
		background: rgba(0,0,0,.18);
		color: #fff;
	}
	.o-search {
		display: none;
	}
	header label {
		min-width: 0;
	}
	.theme-field {
		display: none;
	}
	.layout {
		grid-template-columns: minmax(0, 1fr);
		background: #eef0f3;
		min-height: calc(100vh - 46px);
	}
	body[data-view="apps"] .layout {
		display: block;
		background: var(--home-bg);
		min-height: 100vh;
	}
	body[data-view="apps"] aside {
		display: none;
	}
	body:not([data-view="apps"]) aside {
		display: none;
	}
	body[data-view="apps"] > .o_navbar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 20;
		background: transparent;
		border-bottom-color: transparent;
		box-shadow: none;
	}
	body[data-view="apps"] > .o_navbar > .o_main_navbar {
		background: transparent;
		border-bottom-color: transparent;
		box-shadow: none;
	}
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_sections,
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o-mobile-menu-toggle,
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o-search {
		display: none;
	}
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray {
		margin-left: auto;
		background: transparent;
	}
	body[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item {
		background: transparent;
		border-color: transparent;
		color: var(--home-text);
		text-shadow: none;
	}
	body[data-view="apps"] #appGrid.o_apps {
		margin-top: 104px;
	}
	aside {
		background: #f7f7f7;
		padding: 10px 0;
	}
	.o-sidebar-title {
		padding: 0 14px 8px;
	}
	#runtimeStatus {
		display: none;
	}
	body.modal-open {
		overflow: hidden;
	}
	.gorp-action-dialog {
		position: fixed;
		inset: 0;
		z-index: 1055;
		overflow: hidden auto;
		display: flex;
		align-items: flex-start;
		justify-content: center;
		padding: 56px 18px 24px;
		background: transparent;
	}
	.gorp-action-dialog .o_dialog_container {
		position: relative;
		z-index: 1056;
		width: 100%;
		pointer-events: none;
	}
	.gorp-action-dialog .modal-dialog {
		width: min(960px, 100%);
		margin: 0;
		pointer-events: auto;
	}
	.gorp-action-dialog .modal-content {
		display: flex;
		flex-direction: column;
		height: min(820px, calc(100vh - 80px));
		max-height: calc(100vh - 80px);
		background: var(--panel);
		border: 1px solid var(--line);
		border-radius: 6px;
		box-shadow: 0 18px 44px rgba(16, 24, 40, .28);
		overflow: hidden;
	}
	.gorp-action-dialog .modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		min-height: 52px;
		padding: 12px 16px;
		border-bottom: 1px solid var(--line);
		background: #fff;
	}
	.gorp-action-dialog .modal-title {
		margin: 0;
		color: #1f2937;
		font-size: 18px;
		font-weight: 500;
	}
	.gorp-action-dialog .btn-close {
		width: 32px;
		height: 32px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--muted);
	}
	.gorp-action-dialog .btn-close::before {
		content: "\00d7";
		font-size: 20px;
		line-height: 1;
	}
	.gorp-action-dialog .modal-body {
		flex: 1 1 auto;
		min-height: 0;
		overflow: auto;
		padding: 0;
		background: #eef0f3;
	}
	.gorp-action-dialog .gorp-dialog-window-action {
		min-height: 0;
		background: transparent;
	}
	.gorp-action-dialog .gorp-dialog-window-action > .gorp-form-view,
	.gorp-action-dialog .gorp-dialog-window-action > .gorp-settings-action,
	.gorp-action-dialog .gorp-dialog-window-action > .gorp-list-view,
	.gorp-action-dialog .gorp-dialog-window-action > .gorp-kanban-view {
		margin: 0;
	}
	.gorp-action-dialog .gorp-action-dialog-footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 8px;
		min-height: 52px;
		padding: 10px 16px;
		border-top: 1px solid var(--line);
		background: #fff;
	}
	.gorp-action-dialog .gorp-action-dialog-footer .text-muted {
		margin-right: auto;
		color: var(--muted);
	}
	.gorp-action-dialog-backdrop {
		position: fixed;
		inset: 0;
		z-index: 1050;
		background: rgba(17, 24, 39, .55);
		backdrop-filter: blur(1px);
	}
	#modules {
		margin: 0;
		gap: 0;
	}
	#modules li {
		border-bottom: 0;
		padding: 8px 14px;
		align-items: center;
	}
	#modules li:hover {
		background: rgba(113,75,103,.08);
	}
	.o-debug-link {
		display: none;
		padding: 0 14px;
		font-size: 12px;
	}
	.o-statusbar {
		display: none;
	}
	main {
		min-width: 0;
		background: #eef0f3;
	}
	main.o_web_client {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
		background: #eef0f3;
	}
	main.o_web_client > .o_action_manager {
		flex: 1 1 auto;
		min-height: calc(100vh - 46px);
		background: #eef0f3;
	}
	main.o_web_client > .o_navbar {
		position: relative;
		z-index: 2050;
		flex: 0 0 46px;
		height: 46px;
		min-height: 46px;
		overflow: visible;
	}
	main.o_web_client[data-view="apps"] > .o_navbar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 20;
		flex: 0 0 auto;
	}
	main.o_web_client:not([data-view="apps"]) > .o_navbar > .o_main_navbar {
		position: relative;
		background: var(--topbar);
		border-bottom: 1px solid rgba(0,0,0,.14);
		box-shadow: 0 1px 2px rgba(16, 24, 40, .12);
	}
	main.o_web_client > .o_action_manager > .o-app-launcher-view {
		min-height: calc(100vh - 46px);
	}
	main.o_web_client[data-view="apps"] {
		position: relative;
		background: var(--home-bg);
	}
	body.o_home_menu_background,
	body[data-view="apps"] {
		background: var(--home-bg) !important;
		color: var(--home-text);
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 20;
		background: transparent;
		border: 0;
		box-shadow: none;
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_sections,
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-mobile-menu-toggle {
		display: none;
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu {
		display: inline-flex;
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o_menu_toggle,
	main.o_web_client[data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o_menu_brand {
		color: var(--home-text);
		text-shadow: none;
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o-launcher span {
		background: var(--home-text);
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray {
		margin-left: auto;
		background: transparent;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item {
		background: transparent;
		border-color: transparent;
		color: var(--home-text);
		text-shadow: none;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item:hover {
		background: rgba(113,75,103,.08);
		border-color: rgba(113,75,103,.12);
	}
	main.o_web_client[data-view="apps"] > .o_action_manager {
		min-height: 100vh;
		background: var(--home-bg);
	}
	main.o_web_client[data-view="apps"] > .o_action_manager > .o-app-launcher-view {
		min-height: 100vh;
		padding-top: 70px;
	}
	.panel {
		background: transparent;
		border-bottom: 0;
		padding: 0;
	}
	.view-panel {
		display: none;
		min-height: calc(100vh - 46px);
	}
	.view-panel.active {
		display: block;
	}
	.o-control-panel,
	.o_control_panel {
		display: flex;
		flex-direction: column;
		align-items: stretch;
		justify-content: center;
		gap: 10px;
		min-height: 62px;
		padding: 9px 18px;
		background: var(--control-bg);
		border-bottom: 1px solid var(--line);
		box-shadow: var(--control-shadow);
	}
	.o_control_panel_main {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 8px 16px;
		width: 100%;
	}
	.o_control_panel_breadcrumbs,
	.o_control_panel_actions,
	.o_control_panel_navigation,
	.o_control_panel_main_buttons {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
	}
	.o_control_panel_breadcrumbs { flex: 1 1 260px; }
	.o_control_panel_actions { flex: 3 1 420px; justify-content: flex-start; }
	.o_control_panel_navigation { flex: 1 1 180px; justify-content: flex-end; }
	.o_control_panel_breadcrumbs {
		flex-wrap: nowrap;
		overflow: hidden;
	}
	.o_control_panel_breadcrumbs .o_control_panel_main_buttons {
		flex: 0 0 auto;
	}
	.o_control_panel_breadcrumbs .o_breadcrumb,
	.o_control_panel_breadcrumbs .o-breadcrumbs {
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
	}
	.o_control_panel_breadcrumbs .breadcrumb-item {
		border: 0;
		background: transparent;
		color: var(--text);
		padding: 0;
		font-weight: 500;
	}
	.o_control_panel_breadcrumbs .breadcrumb-item.active {
		background: transparent !important;
		color: var(--text);
	}
	.o_cp_pager,
	.o_pager {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		white-space: nowrap;
		color: var(--muted);
	}
	.o_cp_switch_buttons {
		display: inline-flex;
	}
	.o_cp_switch_buttons .o_switch_view,
	.o_pager .btn {
		display: inline-grid;
		place-items: center;
		width: 34px;
		height: 34px;
		min-width: 34px;
		padding: 0;
		border-color: var(--line);
	}
	.o_cp_switch_buttons .o_switch_view i,
	.o_pager .btn i {
		position: relative;
		display: block;
		width: 16px;
		height: 16px;
		font-size: 0;
		line-height: 0;
		color: currentColor;
	}
	.o_switch_view.o_list i::before,
	.o_switch_view.o_list i::after {
		content: "";
		position: absolute;
		left: 1px;
		right: 1px;
		height: 2px;
		border-radius: 1px;
		background: currentColor;
		box-shadow: 0 5px 0 currentColor, 0 10px 0 currentColor;
	}
	.o_switch_view.o_kanban i::before {
		content: "";
		position: absolute;
		inset: 1px;
		border-radius: 2px;
		box-shadow:
			0 0 0 2px currentColor inset,
			7px 0 0 -1px currentColor,
			0 7px 0 -1px currentColor,
			7px 7px 0 -1px currentColor;
	}
	.o_switch_view.o_form i::before {
		content: "";
		position: absolute;
		inset: 1px 2px;
		border: 2px solid currentColor;
		border-radius: 2px;
		box-shadow: inset 0 5px 0 rgba(255,255,255,.18);
	}
	.o_pager_previous i::before,
	.o_pager_next i::before {
		content: "";
		position: absolute;
		top: 3px;
		width: 8px;
		height: 8px;
		border-top: 2px solid currentColor;
		border-left: 2px solid currentColor;
	}
	.o_pager_previous i::before {
		left: 5px;
		transform: rotate(-45deg);
	}
	.o_pager_next i::before {
		right: 5px;
		transform: rotate(135deg);
	}
	.o_cp_searchview {
		display: inline-flex;
		align-items: stretch;
		width: min(440px, 100%);
		max-width: 100%;
	}
	.o_searchview {
		display: inline-flex;
		align-items: center;
		flex: 1;
		min-width: 0;
		min-height: 32px;
		border: 1px solid var(--line);
		border-right: 0;
		border-radius: 4px 0 0 4px;
		background: #fff;
		box-shadow: inset 0 1px 0 rgba(16,24,40,.02);
	}
	.o_cp_searchview:focus-within .o_searchview,
	.o_cp_searchview:focus-within .o_searchview_dropdown_toggler {
		border-color: var(--accent-2);
		box-shadow: 0 0 0 2px rgba(1,126,132,.12);
	}
	.o_searchview_input_container {
		display: flex;
		align-items: center;
		flex: 1;
		min-width: 0;
	}
	.o_searchview_input_container .field {
		display: flex;
		align-items: center;
		flex: 1;
		min-width: 0;
		margin: 0;
	}
	.o_searchview_input {
		width: 100%;
		min-width: 0;
		height: 28px;
		border: 0;
		background: transparent;
		outline: none;
	}
	.o_searchview_icon {
		position: relative;
		display: inline-block;
		width: 14px;
		height: 14px;
		font-style: normal;
		color: var(--muted);
	}
	.o_searchview_icon::before {
		content: "";
		position: absolute;
		top: 1px;
		left: 1px;
		width: 8px;
		height: 8px;
		border: 2px solid currentColor;
		border-radius: 50%;
	}
	.o_searchview_icon::after {
		content: "";
		position: absolute;
		top: 10px;
		left: 9px;
		width: 6px;
		height: 2px;
		background: currentColor;
		border-radius: 1px;
		transform: rotate(45deg);
		transform-origin: left center;
	}
	.o_searchview .btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--muted);
	}
	.o_searchview_dropdown_toggler {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		min-width: 32px;
		padding: 0;
		border: 1px solid var(--line);
		background: #fff;
		color: var(--muted);
		border-top-left-radius: 0;
		border-bottom-left-radius: 0;
	}
	.o_searchview_dropdown_toggler::before {
		content: "";
		width: 0;
		height: 0;
		border-left: 4px solid transparent;
		border-right: 4px solid transparent;
		border-top: 5px solid currentColor;
	}
	.o_searchview_facet_container {
		display: inline-flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 4px;
		max-width: 52%;
	}
	.o_searchview_facet {
		display: inline-flex;
		align-items: stretch;
		max-width: 180px;
		border: 1px solid #d8dadd;
		border-radius: 4px;
		background: #f1f3f5;
		color: var(--text);
		overflow: hidden;
	}
	.o_searchview_facet_label {
		display: inline-flex;
		align-items: center;
		padding: 0 6px;
		background: #e6f2f3;
		color: var(--accent-2);
		font-size: 12px;
		font-weight: 600;
	}
	.o_searchview_facet .o_facet_values {
		padding: 0 6px;
		font-size: 12px;
		line-height: 24px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.o_facet_remove {
		width: 24px;
		border: 0;
		background: transparent;
		color: var(--muted);
	}
	.o_facet_remove:hover {
		background: #e8eaee;
		color: var(--text);
	}
	.o_search_options {
		position: absolute;
		z-index: 30;
		top: calc(100% + 4px);
		right: 0;
		display: grid;
		grid-template-columns: repeat(3, minmax(160px, 1fr));
		min-width: min(720px, calc(100vw - 32px));
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		box-shadow: var(--dropdown-shadow);
		overflow: hidden;
	}
	.o_search_options[hidden] {
		display: none;
	}
	.o_dropdown_container {
		display: grid;
		align-content: start;
		gap: 4px;
		padding: 12px;
		border-right: 1px solid var(--line);
	}
	.o_dropdown_container:last-child {
		border-right: 0;
	}
	.o_dropdown_container h3 {
		margin: 0 0 6px;
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
	}
	.o_menu_item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		width: 100%;
		min-height: 30px;
		border: 0;
		background: transparent;
		color: var(--text);
		text-align: left;
	}
	.o_menu_item:hover,
	.o_menu_item.active {
		background: var(--hover-bg);
		color: var(--accent);
	}
	.o_menu_item .o_search_check {
		color: var(--accent);
		font-weight: 700;
	}
	.o_grouped_list {
		display: grid;
		gap: 0;
		border: 1px solid var(--line);
		background: #fff;
	}
	.o_group_header {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: center;
		padding: 9px 12px;
		border-bottom: 1px solid var(--line);
		background: #f8f9fa;
		cursor: pointer;
	}
	.o_group_header:hover {
		background: var(--hover-bg);
	}
	.o_group_name {
		font-weight: 600;
	}
	.o_group_count {
		color: var(--muted);
		font-size: 12px;
	}
	.o-control-panel h2,
	.o_control_panel h2 {
		margin: 0;
		font-size: 18px;
		font-weight: 500;
		min-width: 0;
	}
	.o-control-panel .toolbar,
	.o_control_panel .toolbar {
		padding: 0;
	}
	.o-control-panel .o_cp_searchview,
	.o_control_panel .o_cp_searchview {
		position: relative;
	}
	.o-control-panel .field,
	.o_control_panel .field {
		min-width: 180px;
	}
	.technical-field,
	.o-debug-only {
		display: none !important;
	}
	.o-list-content {
		padding: 14px 18px 24px;
	}
	.o-app-launcher-view {
		min-height: 100dvh;
		background-color: var(--home-bg) !important;
		background-image: var(--home-bg-image);
		background-attachment: fixed;
		background-position: center;
		background-repeat: no-repeat;
		background-size: cover;
		box-shadow: inset 0 1px 0 rgba(255,255,255,.66);
		color: var(--home-text);
		padding: 76px 24px 44px;
	}
	.o_home_menu_registration_banner {
		display: none !important;
	}
	body[data-theme="standard"] .o-app-launcher-view {
		background: #eef0f3;
		color: var(--text);
	}
	.o-app-shell {
		max-width: 818px;
		margin: 0 auto;
	}
	.o-app-search {
		max-width: 520px;
		height: 0;
		max-height: 0;
		margin: 0 auto 0;
		opacity: 0;
		overflow: hidden;
		pointer-events: none;
		transform: translateY(-8px);
		transition: opacity 160ms ease, transform 160ms ease, max-height 160ms ease, margin-bottom 160ms ease;
	}
	.o-app-search.is-active {
		height: auto;
		max-height: 48px;
		margin: 8px auto 48px;
		opacity: 1;
		overflow: visible;
		pointer-events: auto;
		transform: none;
	}
	.o-app-search input {
		height: 40px;
		border-color: var(--home-line);
		background: var(--home-panel);
		color: var(--home-text);
		text-align: left;
		box-shadow: 0 8px 22px rgba(0,0,0,.12);
	}
	.o-app-search label {
		display: block;
	}
	.o-app-search input::placeholder {
		color: var(--home-muted);
	}
	.o-app-launcher-view .muted {
		color: #4b5563;
	}
	.o-app-launcher-view .o_apps {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: flex-start;
		gap: 0;
		margin: 44px auto 18px;
		max-width: 818px;
		padding: 0;
	}
	.o-app-launcher-view .o_draggable {
		width: 16.666667%;
		margin-bottom: 16px;
		padding: 0;
	}
	.o-app-launcher-view .o_app {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: flex-start;
		width: 100%;
		min-height: 104px;
		border: 0;
		border-radius: 4px;
		background: transparent !important;
		color: var(--home-text);
		padding: 8px 4px;
		text-decoration: none;
		overflow: hidden;
		transition: color 160ms ease, transform 120ms ease, background 120ms ease;
	}
	.o-app-launcher-view .o_app:hover,
	.o-app-launcher-view .o_app:focus-visible {
		text-decoration: none;
	}
	.o-app-launcher-view .o_app_icon {
		display: inline-grid;
		place-items: center;
		flex: 0 0 70px;
		position: relative;
		width: 70px;
		min-width: 70px;
		height: 70px;
		min-height: 70px;
		border: 1px solid rgba(255,255,255,.10);
		border-radius: 6px;
		background: #875a7b;
		color: #fff;
		font-size: 0;
		font-weight: 600;
		box-shadow:
			inset 0 0 0 1px rgba(0,0,0,.18),
			0 1px 1px rgba(0,0,0,.02),
			0 2px 2px rgba(0,0,0,.02),
			0 4px 4px rgba(0,0,0,.02),
			0 8px 8px rgba(0,0,0,.02),
			0 16px 16px rgba(0,0,0,.02);
		overflow: hidden;
	}
	.o-app-launcher-view .o_app .o_caption,
	.o-app-launcher-view .o_app .o_app_name {
		color: var(--home-text);
		font-size: 14px;
		line-height: 1.18;
		font-weight: 500;
	}
	.o-app-launcher-view .o_app:nth-child(4n+2) .o_app_icon { background: #017e84; }
	.o-app-launcher-view .o_app:nth-child(4n+3) .o_app_icon { background: #5f6f94; }
	.o-app-launcher-view .o_app:nth-child(4n+4) .o_app_icon { background: #b05f4a; }
	.o-app-launcher-view .o_app_icon[data-icon-token="teal"] { background: #017e84; }
	.o-app-launcher-view .o_app_icon[data-icon-token="purple"] { background: #875a7b; }
	.o-app-launcher-view .o_app_icon[data-icon-token="blue"] { background: #5f6f94; }
	.o-app-launcher-view .o_app_icon[data-icon-token="terracotta"] { background: #b05f4a; }
	.o-app-launcher-view .o_app_icon[data-icon-token="green"] { background: #228b65; }
	.o-app-launcher-view .o_app_icon[data-icon-token="slate"] { background: #56616f; }
	.o-app-launcher-view .o_app_icon_fallback::before,
	.o-app-launcher-view .o_app_icon_fallback::after {
		content: "";
		position: absolute;
		pointer-events: none;
	}
	.o-app-launcher-view .o_app_icon_fallback::before {
		inset: 16px;
		border-radius: 8px;
		background: rgba(255,255,255,.88);
	}
	.o-app-launcher-view .o_app_icon_fallback::after {
		right: 13px;
		bottom: 13px;
		width: 24px;
		height: 12px;
		border-radius: 8px;
		background: rgba(255,255,255,.42);
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon {
		background: #b05f4a;
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::before {
		inset: 15px;
		border-radius: 50%;
		background: conic-gradient(#54c4c9 0 25%, #875a7b 0 50%, #ef6c56 0 75%, #6ca6d9 0);
		box-shadow: inset 0 0 0 2px rgba(255,255,255,.1);
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::after {
		inset: 29px;
		width: auto;
		height: auto;
		border-radius: 50%;
		background: #263445;
		box-shadow:
			-15px 0 0 -12px #263445,
			15px 0 0 -12px #263445,
			0 -15px 0 -12px #263445,
			0 15px 0 -12px #263445;
	}
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::before {
		inset: 12px 15px;
		border-radius: 0;
		background: #ee8543;
		clip-path: polygon(50% 0, 88% 22%, 88% 72%, 50% 100%, 12% 72%, 12% 22%);
	}
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::after {
		inset: 29px;
		width: auto;
		height: auto;
		border-radius: 50%;
		background: #263445;
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon,
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon {
		background: rgba(31,43,59,.78);
		border-color: rgba(255,255,255,.12);
		border-radius: 4px;
		box-shadow: inset 0 0 0 1px rgba(0,0,0,.22);
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::before {
		left: 13px;
		top: 13px;
		right: auto;
		bottom: auto;
		width: 44px;
		height: 44px;
		border: 0;
		border-radius: 50%;
		background: conic-gradient(#875a7b 0 25%, #4fc3c8 0 50%, #ef6c56 0 75%, #6ca6d9 0);
		box-shadow: inset 0 0 0 1px rgba(255,255,255,.14);
		transform: none;
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::after {
		left: 13px;
		top: 13px;
		right: auto;
		bottom: auto;
		width: 44px;
		height: 44px;
		border-radius: 50%;
		background:
			conic-gradient(from 0deg, transparent 0 24%, #263445 24% 26%, transparent 26% 74%, #263445 74% 76%, transparent 76% 100%),
			conic-gradient(from 90deg, transparent 0 24%, #263445 24% 26%, transparent 26% 74%, #263445 74% 76%, transparent 76% 100%);
		box-shadow: none;
		transform: none;
	}
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::before {
		left: 16px;
		top: 12px;
		right: auto;
		bottom: auto;
		width: 38px;
		height: 46px;
		border: 0;
		border-radius: 0;
		background: conic-gradient(from 315deg, #f7c462 0 50%, #ee8543 50% 100%);
		clip-path: polygon(50% 0, 88% 21%, 88% 72%, 50% 100%, 12% 72%, 12% 21%);
		box-shadow: 0 0 0 1px rgba(0,0,0,.12), inset 0 1px 0 rgba(255,255,255,.24);
		transform: none;
	}
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::after {
		left: 28px;
		top: 26px;
		right: auto;
		bottom: auto;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: #263445;
		box-shadow: 0 0 0 7px rgba(135,90,123,.86);
		transform: none;
	}
	.o-app-launcher-view .o_app[data-app-key="approvals"] .o_app_icon_fallback::before {
		inset: 16px;
		border-radius: 50%;
		background: transparent;
		border: 8px solid rgba(255,255,255,.9);
	}
	.o-app-launcher-view .o_app[data-app-key="approvals"] .o_app_icon_fallback::after {
		left: 36px;
		top: 31px;
		width: 18px;
		height: 9px;
		border-left: 4px solid rgba(255,255,255,.9);
		border-bottom: 4px solid rgba(255,255,255,.9);
		border-radius: 1px;
		background: transparent;
		transform: rotate(-45deg);
	}
	.o-app-launcher-view .o_app[data-app-key="delegation"] .o_app_icon_fallback::before {
		inset: 16px 22px 14px 13px;
		border-radius: 10px;
		background: rgba(255,255,255,.9);
		box-shadow: 12px 0 0 rgba(255,255,255,.38);
	}
	.o-app-launcher-view .o_app[data-app-key="delegation"] .o_app_icon_fallback::after {
		right: 12px;
		bottom: 15px;
		width: 28px;
		height: 14px;
		border-radius: 7px;
		background: rgba(255,255,255,.42);
	}
	.o-app-launcher-view img.o_app_icon {
		object-fit: cover;
		display: block;
	}
	.o-app-launcher-view img.o_app_icon::before,
	.o-app-launcher-view img.o_app_icon::after {
		display: none;
	}
	.o-app-launcher-view .o_app:hover {
		background: rgba(255,255,255,.08);
		border-color: transparent;
		color: var(--home-text);
	}
	body[data-theme="standard"] .o-app-search input {
		border-color: #cfd4dc;
		background: #fff;
		color: var(--text);
	}
	body[data-theme="standard"] .o-app-search input::placeholder,
	body[data-theme="standard"] .o-app-launcher-view .muted {
		color: var(--muted);
	}
	body[data-theme="standard"] .o-app-launcher-view .o_app,
	body[data-theme="standard"] .o-app-launcher-view .o_app:hover {
		color: var(--text);
	}
	.o-app-launcher-view .o_app strong {
		display: block;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 500;
		text-decoration: none;
		text-shadow: none;
	}
	.o-app-launcher-view .badge {
		display: none;
	}
	.o-app-message {
		max-width: 880px;
		margin: 0 auto;
		text-align: center;
	}
	.o-list-view table {
		width: 100%;
		margin-top: 0;
		border: 1px solid var(--line);
		box-shadow: 0 1px 0 rgba(16,24,40,.03);
	}
	.o-list-view .o_list_record_selector {
		width: 34px;
		text-align: center;
	}
	.o-list-view th.o_column_sortable {
		padding: 0;
	}
	.o_list_header_button {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		min-height: 34px;
		padding: 7px 10px;
		border: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		text-align: left;
	}
	.o_list_header_button:hover {
		background: #eef0f3;
		color: var(--accent);
	}
	.o_column_sortable[aria-sort="ascending"] .o_list_header_button::after,
	.o_column_sortable[aria-sort="descending"] .o_list_header_button::after {
		content: "▲";
		margin-left: 6px;
		color: var(--accent);
		font-size: 9px;
	}
	.o_column_sortable[aria-sort="descending"] .o_list_header_button::after {
		content: "▼";
	}
	.o_data_row_selected td {
		background: #e6f2f3 !important;
	}
	.gorp-list-toolbar.gorp-action-menus {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		margin: 0 0 8px;
	}
	.o_control_panel_main_buttons .gorp-list-toolbar.gorp-action-menus {
		flex-wrap: nowrap;
		gap: 4px;
		margin: 0;
	}
	.gorp-action-menu-section {
		position: relative;
		display: inline-flex;
	}
	.gorp-action-menu-toggle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 30px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		color: var(--text);
		padding: 5px 10px;
	}
	.gorp-action-menu-toggle:hover {
		background: var(--hover-bg);
		color: var(--accent);
	}
	.o_control_panel_main_buttons .gorp-list-toolbar .gorp-action-menu-toggle {
		justify-content: center;
		width: 34px;
		min-width: 34px;
		min-height: 34px;
		padding: 0;
		font-size: 0;
	}
	.o_control_panel_main_buttons .gorp-list-toolbar .gorp-action-menu-toggle i {
		font-size: 14px;
	}
	.o_control_panel_main_buttons .gorp-list-toolbar .gorp-action-menu-toggle::before {
		content: "\2699";
		font-size: 15px;
		line-height: 1;
	}
	.o_control_panel_main_buttons .gorp-list-toolbar .gorp-action-menu-toggle i {
		display: none;
	}
	.gorp-action-menu-items {
		display: none;
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		z-index: 25;
		min-width: 190px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		box-shadow: var(--dropdown-shadow);
		padding: 4px;
	}
	.gorp-action-menu-section.open .gorp-action-menu-items {
		display: grid;
		gap: 2px;
	}
	.gorp-action-menu-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		min-height: 30px;
		border: 0;
		border-radius: 3px;
		background: transparent;
		color: var(--text);
		padding: 5px 8px;
		text-align: left;
	}
	.gorp-action-menu-item:hover:not(:disabled) {
		background: var(--hover-bg);
		color: var(--accent);
	}
	.gorp-action-menu-item:disabled {
		color: var(--muted);
		opacity: .65;
	}
	.gorp-window-action[data-view="form"] .o_control_panel_actions {
		justify-content: flex-start;
	}
	.gorp-window-action[data-view="form"] .o_control_panel_actions .gorp-form-action-menu {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.gorp-window-action[data-view="form"] .gorp-form-action-menu .gorp-action-menu-toggle {
		min-width: 36px;
		height: 34px;
		min-height: 34px;
		padding: 0 10px;
		border-color: var(--line);
		background: var(--btn-secondary-bg);
		color: var(--text);
	}
	.o_mobile_list_cards {
		display: none;
	}
	.o_mobile_record_card {
		display: grid;
		gap: 6px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		padding: 12px 14px;
		box-shadow: 0 1px 0 rgba(16,24,40,.03);
		cursor: pointer;
	}
	.o_mobile_record_card:focus {
		outline: 2px solid var(--accent-2);
		outline-offset: -2px;
	}
	.o_mobile_record_header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		min-width: 0;
	}
	.o_mobile_record_title {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		line-height: 1.25;
	}
	.o_mobile_record_state {
		flex: 0 0 auto;
		max-width: 45%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		padding: 3px 7px;
		border: 1px solid rgba(1,126,132,.34);
		border-radius: 4px;
		background: rgba(1,126,132,.12);
		color: var(--accent-2);
		font-size: 12px;
		font-weight: 600;
	}
	.o_mobile_record_line {
		display: grid;
		grid-template-columns: minmax(78px, .38fr) minmax(0, 1fr);
		gap: 8px;
		min-width: 0;
	}
	.o_mobile_record_label {
		color: var(--muted);
		font-size: 12px;
	}
	.o_mobile_record_value {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.o-list-view th,
	.o-list-view td {
		background: #fff;
	}
	.o-list-view th {
		background: var(--list-head);
		color: #555e6b;
	}
	.o-list-view .o_data_row {
		cursor: pointer;
	}
	.o-list-view tr:hover td {
		background: var(--hover-bg);
	}
	.o_kanban_renderer {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 10px;
		padding: 12px 18px;
	}
	.o_kanban_renderer.o_kanban_grouped {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		overflow-x: auto;
		padding-bottom: 18px;
	}
	.o_kanban_group {
		flex: 0 0 280px;
		min-width: 260px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: rgba(255,255,255,.55);
	}
	.o_kanban_group.o_column_folded {
		flex-basis: 52px;
		min-width: 52px;
		max-width: 52px;
		overflow: hidden;
	}
	.o_kanban_header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		min-height: 42px;
		padding: 10px 12px;
		border-bottom: 1px solid var(--line);
	}
	.o_kanban_group.o_column_folded .o_kanban_header {
		display: grid;
		grid-template-rows: auto auto auto;
		justify-items: center;
		min-height: 180px;
		padding: 8px 6px;
	}
	.o_kanban_header_title {
		margin: 0;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}
	.o_kanban_group.o_column_folded .o_kanban_header_title {
		writing-mode: vertical-rl;
		max-height: 110px;
	}
	.o_kanban_counter {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 24px;
		height: 22px;
		border-radius: 999px;
		background: var(--soft);
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
	}
	.o_kanban_group_fold_toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex: 0 0 auto;
		width: 24px;
		height: 24px;
		padding: 0;
		border-radius: 4px;
		color: var(--muted);
		font-size: 18px;
		line-height: 1;
		text-decoration: none;
	}
	.o_kanban_group_fold_toggle:hover,
	.o_kanban_group_fold_toggle:focus {
		background: rgba(113,75,103,.08);
		color: var(--brand);
		text-decoration: none;
	}
	.o_kanban_records {
		display: grid;
		gap: 10px;
		padding: 10px;
	}
	.o_kanban_record[draggable="true"] {
		cursor: grab;
	}
	.o_kanban_record.o_kanban_record_dragging {
		cursor: grabbing;
		opacity: .58;
		transform: scale(.985);
	}
	.o_kanban_group.o_kanban_group_drop_target {
		border-color: rgba(113,75,103,.38);
		background: rgba(113,75,103,.055);
	}
	.o_kanban_records.o_kanban_records_drop_target {
		outline: 1px dashed rgba(113,75,103,.42);
		outline-offset: -6px;
	}
	.o_kanban_group_load_more {
		width: 100%;
		min-height: 34px;
		border: 1px dashed var(--line);
		border-radius: 4px;
		background: rgba(113,75,103,.035);
		color: var(--brand);
		font-weight: 600;
		text-align: center;
	}
	.o_kanban_template_details,
	.o_kanban_template_body {
		display: grid;
		gap: 6px;
		min-width: 0;
	}
	.o_kanban_template_body > * {
		min-width: 0;
	}
	.o_kanban_template_field output {
		display: inline;
		margin: 0;
	}
	.o_kanban_progressbar {
		display: grid;
		gap: 6px;
		min-width: 0;
		padding: 10px;
		border-bottom: 1px solid var(--line);
	}
	.o_kanban_renderer.o_kanban_ungrouped > .o_kanban_progressbar {
		grid-column: 1 / -1;
		padding: 0 0 2px;
		border-bottom: 0;
	}
	.o_kanban_progressbar_track {
		display: flex;
		width: 100%;
		height: 8px;
		overflow: hidden;
		border-radius: 999px;
		background: var(--soft);
	}
	.o_kanban_progressbar_segment {
		display: block;
		min-width: 2px;
		height: 100%;
		background: var(--kanban-progress-color);
	}
	.o_kanban_progressbar_legend {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 10px;
		min-width: 0;
	}
	.o_kanban_progressbar_legend_item {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		min-width: 0;
		max-width: 100%;
		color: var(--muted);
		font-size: 11px;
		line-height: 1.2;
	}
	.o_kanban_progressbar_legend_marker {
		flex: 0 0 auto;
		width: 7px;
		height: 7px;
		border-radius: 999px;
		background: var(--kanban-progress-color);
	}
	.o_kanban_progressbar_legend_text {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.o_kanban_progress_color_success { --kanban-progress-color: #00a09d; color: #017e84; }
	.o_kanban_progress_color_info { --kanban-progress-color: #2f80ed; color: #2367bf; }
	.o_kanban_progress_color_warning { --kanban-progress-color: #f0ad4e; color: #946200; }
	.o_kanban_progress_color_danger { --kanban-progress-color: #d9534f; color: #a94442; }
	.o_kanban_progress_color_primary { --kanban-progress-color: #714b67; color: #714b67; }
	.o_kanban_progress_color_muted { --kanban-progress-color: #9aa3af; color: #667085; }
	.o_kanban_quick_add {
		width: calc(100% - 20px);
		min-height: 34px;
		margin: 0 10px 10px;
		border: 1px dashed var(--line);
		border-radius: 4px;
		background: rgba(255,255,255,.62);
		color: var(--brand);
		font-size: 13px;
		font-weight: 500;
		text-align: left;
	}
	.o_kanban_renderer.o_kanban_ungrouped > .o_kanban_quick_add {
		width: 100%;
		margin: 0;
		padding: 12px;
	}
	.o_kanban_quick_add:hover,
	.o_kanban_quick_add:focus {
		border-color: rgba(113,75,103,.45);
		background: rgba(113,75,103,.06);
		color: var(--brand-strong);
		text-decoration: none;
	}
	.o_kanban_load_more_wrapper {
		grid-column: 1 / -1;
		display: flex;
		justify-content: center;
		padding: 6px 0 2px;
	}
	.o_kanban_grouped > .o_kanban_load_more_wrapper {
		flex: 0 0 220px;
		align-self: stretch;
		align-items: center;
	}
	.o_kanban_load_more {
		min-width: 132px;
		min-height: 32px;
		border-color: var(--line);
		background: #fff;
		color: var(--brand);
		font-size: 13px;
		font-weight: 500;
	}
	.o_kanban_load_more:hover,
	.o_kanban_load_more:focus {
		border-color: rgba(113,75,103,.45);
		background: rgba(113,75,103,.06);
		color: var(--brand-strong);
	}
	.o_kanban_record {
		position: relative;
		display: grid;
		gap: 8px;
		min-height: 92px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		padding: 12px;
		cursor: pointer;
		--kanban-card-color: transparent;
	}
	.o_kanban_record[data-kanban-color] {
		border-left: 3px solid var(--kanban-card-color);
		padding-left: 10px;
	}
	.o_kanban_color_0 { --kanban-card-color: transparent; }
	.o_kanban_color_1 { --kanban-card-color: #00a09d; }
	.o_kanban_color_2 { --kanban-card-color: #f0ad4e; }
	.o_kanban_color_3 { --kanban-card-color: #d9534f; }
	.o_kanban_color_4 { --kanban-card-color: #2f80ed; }
	.o_kanban_color_5 { --kanban-card-color: #714b67; }
	.o_kanban_color_6 { --kanban-card-color: #6f7d95; }
	.o_kanban_color_7 { --kanban-card-color: #20c997; }
	.o_kanban_color_8 { --kanban-card-color: #fd7e14; }
	.o_kanban_color_9 { --kanban-card-color: #e83e8c; }
	.o_kanban_color_10 { --kanban-card-color: #17a2b8; }
	.o_kanban_color_11 { --kanban-card-color: #6c757d; }
	.o_kanban_record:hover {
		border-color: rgba(113,75,103,.35);
		box-shadow: 0 2px 8px rgba(15,23,42,.06);
	}
	.o_kanban_record_menu {
		position: absolute;
		top: 6px;
		right: 6px;
		z-index: 2;
	}
	.o_kanban_record_menu_toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 26px;
		height: 26px;
		padding: 0;
		border-radius: 4px;
		color: var(--muted);
		font-size: 18px;
		line-height: 1;
		opacity: 0;
		text-decoration: none;
	}
	.o_kanban_record:hover .o_kanban_record_menu_toggle,
	.o_kanban_record_menu_toggle[aria-expanded="true"],
	.o_kanban_record_menu_toggle:focus {
		opacity: 1;
		background: rgba(113,75,103,.08);
		color: var(--brand);
		text-decoration: none;
	}
	.o_kanban_record_menu_dropdown {
		top: 28px;
		right: 0;
		left: auto;
		min-width: 120px;
		padding: 4px;
	}
	.o_kanban_record_menu_item {
		border-radius: 4px;
		font-size: 13px;
	}
	.o_kanban_record_title {
		display: block;
		margin-bottom: 4px;
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}
	.o_kanban_record_field {
		display: grid;
		grid-template-columns: minmax(74px, .42fr) minmax(0, 1fr);
		gap: 8px;
		align-items: center;
		min-width: 0;
		font-size: 13px;
	}
	.o_kanban_field_label {
		color: var(--muted);
	}
	.o_kanban_field_value {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.o_settings_view .o-control-panel,
	.o_settings_view .o_control_panel {
		min-height: 58px;
	}
	.o_settings_content {
		padding: 0;
		background: var(--bg);
	}
	.o_settings_container {
		display: grid;
		grid-template-columns: 182px minmax(0, 1fr);
		gap: 0;
		max-width: none;
		margin: 0;
	}
	.o_settings_search_panel {
		grid-column: 2;
		display: flex;
		justify-content: center;
		padding: 0 28px 16px;
	}
	.o_settings_search_wrapper {
		display: flex;
		align-items: center;
		width: min(414px, 100%);
		height: 34px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--panel);
		color: var(--muted);
		overflow: hidden;
	}
	.o_settings_search_icon {
		position: relative;
		flex: 0 0 34px;
		width: 34px;
		height: 100%;
	}
	.o_settings_search_icon::before {
		content: "";
		position: absolute;
		left: 13px;
		top: 10px;
		width: 10px;
		height: 10px;
		border: 2px solid currentColor;
		border-radius: 50%;
	}
	.o_settings_search_icon::after {
		content: "";
		position: absolute;
		left: 23px;
		top: 21px;
		width: 7px;
		height: 2px;
		background: currentColor;
		transform: rotate(45deg);
		transform-origin: left center;
	}
	.o_settings_search_panel .o_settings_search {
		width: 100%;
		min-width: 0;
		height: 32px;
		border: 0;
		border-radius: 0;
		background: transparent;
		padding: 0 8px 0 0;
		color: var(--text);
		box-shadow: none;
	}
	.o_settings_search_dropdown {
		position: relative;
		flex: 0 0 34px;
		width: 34px;
		height: 100%;
		border: 0;
		border-left: 1px solid var(--line);
		border-radius: 0;
		background: transparent;
		color: var(--muted);
	}
	.o_settings_search_dropdown::after {
		content: "";
		position: absolute;
		left: 50%;
		top: 50%;
		width: 0;
		height: 0;
		margin: -1px 0 0 -4px;
		border-left: 4px solid transparent;
		border-right: 4px solid transparent;
		border-top: 4px solid currentColor;
	}
	.o_settings_sidebar {
		display: grid;
		align-content: start;
		gap: 0;
		border-right: 1px solid var(--line);
		background: var(--sidebar);
		min-height: calc(100vh - 108px);
		padding: 0;
	}
	.o_settings_tab {
		min-height: 42px;
		border: 0;
		border-left: 3px solid transparent;
		border-radius: 0;
		background: transparent;
		color: var(--text);
		padding: 0 14px;
		text-align: left;
		font-weight: 600;
	}
	.o_settings_tab:hover,
	.o_settings_tab.active {
		background: rgba(0,160,157,.16);
		border-left-color: var(--accent-2);
		color: var(--text);
	}
	.o_setting_container {
		display: grid;
		gap: 0;
		min-width: 0;
		align-self: flex-start;
		align-content: flex-start;
		align-items: flex-start;
		grid-auto-rows: max-content;
	}
	.app_settings_block {
		display: grid;
		gap: 0;
		align-self: flex-start;
		align-content: flex-start;
		align-items: flex-start;
		grid-template-rows: max-content;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
		box-shadow: none;
	}
	.app_settings_block[hidden],
	.o_settings_block[hidden],
	.o_setting_box[hidden],
	.o_settings_no_result[hidden] {
		display: none !important;
	}
	.o_settings_app_title {
		display: none;
	}
	.app_settings_block h3 {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
	}
	.o_settings_block {
		display: grid;
		gap: 0;
		align-self: flex-start;
		align-content: flex-start;
		align-items: flex-start;
		grid-template-rows: max-content max-content;
	}
	.o_settings_block_title {
		min-height: 42px;
		margin: 0;
		padding: 12px 32px;
		background: var(--control-bg);
		border-top: 1px solid var(--line);
		border-bottom: 1px solid var(--line);
		color: var(--text);
		font-size: 14px;
		font-weight: 700;
		line-height: 18px;
	}
	.o_setting_grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(280px, 1fr));
		grid-auto-rows: minmax(104px, max-content);
		align-self: flex-start;
		align-items: flex-start;
		gap: 0;
		border-bottom: 1px solid var(--line);
	}
	.o_setting_box {
		display: grid;
		grid-template-columns: 28px minmax(0, 1fr);
		align-items: start;
		gap: 12px;
		min-height: 104px;
		padding: 18px 28px;
		border: 0;
		border-right: 1px solid var(--line);
		border-radius: 0;
		background: transparent;
	}
	.o_setting_grid .o_setting_box:nth-child(2n),
	.o_setting_grid .o_setting_box:last-child {
		border-right: 0;
	}
	.o_setting_left_pane {
		display: flex;
		align-items: flex-start;
		justify-content: center;
		width: 28px;
		min-height: 24px;
		margin-top: 1px;
		border: 0;
		border-radius: 0;
		background: transparent;
	}
	.o_setting_left_pane_empty {
		background: transparent;
	}
	.o_setting_right_pane {
		display: grid;
		gap: 6px;
		min-width: 0;
	}
	.o_setting_right_pane .o_form_label {
		font-weight: 600;
		color: var(--text);
	}
	.o_setting_right_pane .text-muted {
		color: var(--muted);
		font-size: 12px;
	}
	.o_setting_action {
		justify-self: start;
	}
	.o_setting_buttons {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		margin-top: 4px;
	}
	.o_setting_link {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 22px;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: var(--accent);
		font-size: 12px;
		font-weight: 600;
		line-height: 1.4;
		box-shadow: none;
	}
	.o_setting_link::before {
		content: "\2192";
		font-size: 14px;
		font-weight: 700;
		line-height: 1;
	}
	.o_setting_link:hover,
	.o_setting_link:focus-visible {
		background: transparent;
		color: var(--accent-2);
		text-decoration: none;
	}
	.o_setting_box[data-setting-id="invite_users"] .o_setting_fields {
		display: grid;
		grid-template-columns: minmax(180px, 1fr) auto;
		align-items: end;
		gap: 10px;
		max-width: 420px;
	}
	.o_setting_box[data-setting-id="invite_users"] .o_setting_field_label,
	.o_setting_box[data-setting-id="invite_users"] .o_setting_field[data-field="invite_email"] .o_setting_field_label {
		display: none;
	}
	.o_setting_box[data-setting-id="invite_users"] .o_setting_field {
		display: block;
		min-width: 0;
	}
	.o_setting_box[data-setting-id="invite_users"] .o_setting_field .o_input {
		width: 100%;
		border-width: 0 0 1px;
		border-radius: 0;
		background: transparent;
		padding-left: 0;
		box-shadow: none;
	}
	.o_setting_box[data-setting-id="invite_users"] .o_setting_buttons {
		margin: 0;
	}
	.o_setting_invite.btn {
		min-height: 33px;
		padding: 6px 16px;
		border-color: #714b67;
		background: #714b67;
		color: #fff;
	}
	.o_setting_box[data-setting-id="users"] .o_field_widget,
	.o_setting_box[data-setting-id="languages"] .o_field_widget,
	.o_setting_box[data-setting-id="companies"] .o_field_widget,
	.o_setting_box[data-setting-id="company_records"] .o_field_widget,
	.o_setting_box[data-setting-id="document_layout"] .o_field_widget {
		display: none;
	}
	.gorp-form-body.o_form_sheet_bg {
		padding: 16px 18px 26px;
		background: #eef0f3;
	}
	.gorp-form-sheet.o_form_sheet {
		max-width: 1248px;
		margin: 0 auto;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		padding: 20px 24px;
		box-shadow: 0 1px 0 rgba(16,24,40,.03);
	}
	.gorp-form-fields.o_inner_group {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px 24px;
	}
	.gorp-form-sheet .oe_title h1 {
		margin: 0 0 18px;
		font-size: 30px;
		line-height: 1.18;
		font-weight: 500;
		letter-spacing: 0;
	}
	.gorp-form-title-editor.oe_title {
		margin: 0 0 18px;
	}
	.gorp-form-title-input.o_input {
		width: 100%;
		min-height: 42px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--text);
		font-size: 30px;
		line-height: 1.18;
		font-weight: 500;
		box-shadow: none;
	}
	.gorp-form-title-input.o_input::placeholder {
		color: #6f7481;
		opacity: 1;
	}
	.gorp-scheduled-action-run.o_cron_run_manually {
		margin: 10px 0 8px;
		min-height: 33px;
		border-color: #714b67;
		background: #714b67;
		color: #fff;
	}
	.gorp-server-action-band.o_server_action_band {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 14px;
		align-items: center;
		margin: 0 0 18px;
		padding: 12px 14px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: rgba(255,255,255,.035);
	}
	.gorp-server-action-contextual.o_server_action_contextual,
	.gorp-server-action-smart-button.o_stat_button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: max-content;
		min-height: 32px;
		padding: 5px 10px;
		border-radius: 4px;
		font-size: 13px;
		font-weight: 500;
		line-height: 1.35;
	}
	.gorp-server-action-contextual.o_server_action_contextual {
		margin: 0 0 8px;
	}
	.gorp-server-action-identity,
	.gorp-server-action-meta,
	.gorp-server-action-meta-item {
		display: flex;
		align-items: center;
		min-width: 0;
	}
	.gorp-server-action-identity {
		gap: 8px;
	}
	.gorp-server-action-badge {
		display: inline-flex;
		align-items: center;
		min-height: 28px;
		padding: 4px 10px;
		border: 1px solid rgba(113,75,103,.28);
		border-radius: 4px;
		background: rgba(113,75,103,.09);
		color: var(--accent);
		font-weight: 600;
	}
	.gorp-server-action-state {
		display: inline-flex;
		align-items: center;
		min-height: 28px;
		padding: 4px 10px;
		border: 1px solid rgba(1,126,132,.28);
		border-radius: 4px;
		background: rgba(1,126,132,.1);
		color: var(--accent-2);
		font-weight: 600;
	}
	.gorp-server-action-meta {
		gap: 10px;
		flex-wrap: wrap;
		justify-content: flex-end;
	}
	.gorp-server-action-meta-item {
		gap: 6px;
		padding-left: 10px;
		border-left: 1px solid var(--line);
	}
	.gorp-server-action-meta-label {
		color: #aeb6c6;
		font-size: 12px;
	}
	.gorp-server-action-meta-value {
		color: #f4f6fb;
		font-weight: 600;
	}
	.gorp-user-list-identity {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
		font-weight: 600;
	}
	.gorp-user-avatar.o_avatar {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		border-radius: 4px;
		background: #2fbf71;
		color: #fff;
		font-size: 12px;
		font-weight: 800;
		line-height: 1;
	}
	.gorp-user-role-badge {
		display: inline-flex;
		align-items: center;
		min-height: 20px;
		padding: 2px 10px;
		border-radius: 999px;
		background: rgba(145,151,169,.22);
		color: var(--text);
		font-size: 12px;
		font-weight: 700;
		line-height: 16px;
	}
	.gorp-user-identity.o_user_identity_block {
		display: grid;
		grid-template-columns: 130px minmax(0, 1fr);
		gap: 16px;
		align-items: start;
		margin-bottom: 14px;
	}
	.gorp-user-identity-avatar.o_avatar {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 130px;
		height: 130px;
		border-radius: 14px;
		background: #2fbf71;
		color: #fff;
		font-size: 68px;
		font-weight: 500;
		line-height: 1;
	}
	.gorp-user-identity-content {
		display: grid;
		gap: 7px;
		min-width: 0;
	}
	.gorp-user-identity-content .gorp-form-title {
		margin: 2px 0 8px;
		color: var(--text);
		font-size: 31px;
		font-weight: 500;
		line-height: 1.14;
	}
	.gorp-user-identity-line {
		color: var(--text);
		font-weight: 600;
		line-height: 1.35;
	}
	.gorp-user-identity-line.muted {
		color: var(--muted);
		font-weight: 500;
	}
	.gorp-user-identity-input.o_input {
		width: min(220px, 28vw);
		min-height: 24px;
		padding: 0;
		border: 0 !important;
		border-radius: 0;
		background: transparent !important;
		color: var(--text) !important;
		font-size: 14px;
		font-weight: 600;
		line-height: 20px;
		box-shadow: none !important;
		outline: 0;
	}
	.gorp-user-identity-input.o_input::placeholder {
		color: var(--muted) !important;
		opacity: 1;
	}
	.gorp-user-related-partner {
		display: flex;
		gap: 10px;
		align-items: center;
		margin-top: 4px;
		color: var(--muted);
	}
	.gorp-user-related-partner output {
		color: var(--accent-2);
		font-weight: 600;
	}
	.gorp-access-smart-buttons.o_button_box {
		display: flex;
		flex-wrap: wrap;
		gap: 0;
		justify-content: center;
		margin: -4px 0 14px;
	}
	.gorp-access-smart-button.oe_stat_button {
		display: grid;
		grid-template-columns: 24px auto;
		grid-template-areas:
			"icon label"
			"icon value";
		column-gap: 8px;
		align-items: center;
		min-width: 124px;
		min-height: 34px;
		padding: 5px 10px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: transparent;
		color: var(--text);
		text-align: left;
		box-shadow: none;
	}
	.gorp-access-smart-button + .gorp-access-smart-button {
		margin-left: -1px;
	}
	.gorp-access-smart-icon {
		grid-area: icon;
		position: relative;
		width: 22px;
		height: 18px;
	}
	.gorp-access-smart-icon::before,
	.gorp-access-smart-icon::after {
		content: "";
		position: absolute;
		border-radius: 50%;
		background: var(--text);
		opacity: .92;
	}
	.gorp-access-smart-icon::before {
		left: 7px;
		top: 0;
		width: 8px;
		height: 8px;
		box-shadow: -7px 5px 0 -1px var(--text), 7px 5px 0 -1px var(--text);
	}
	.gorp-access-smart-icon::after {
		left: 3px;
		bottom: 0;
		width: 16px;
		height: 8px;
		border-radius: 8px 8px 3px 3px;
	}
	.gorp-access-smart-button .o_stat_text {
		grid-area: label;
		font-size: 12px;
		font-weight: 600;
		line-height: 1.1;
	}
	.gorp-access-smart-button .o_stat_value {
		grid-area: value;
		color: var(--accent);
		font-size: 12px;
		font-weight: 700;
		line-height: 1.1;
	}
	.gorp-res-user-group-ids.o_res_users_access_rights {
		display: grid;
		gap: 22px;
		width: 100%;
		min-inline-size: 0;
		box-sizing: border-box;
		margin: 0;
		padding: 0;
		border: 0;
		color: var(--text);
	}
	.gorp-res-user-access-legend {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
	.gorp-res-user-access-section {
		display: grid;
		gap: 10px;
		padding-top: 14px;
		border-top: 1px solid var(--line);
	}
	.gorp-res-user-access-section:first-of-type {
		padding-top: 0;
		border-top: 0;
	}
	.gorp-res-user-access-section h2 {
		margin: 0 0 4px;
		color: var(--muted);
		font-size: 12px;
		font-weight: 700;
		letter-spacing: .02em;
		line-height: 1.2;
		text-transform: uppercase;
	}
	.gorp-res-user-access-row {
		display: grid;
		grid-template-columns: minmax(170px, .36fr) minmax(0, 1fr);
		gap: 12px;
		align-items: center;
		min-height: 32px;
		color: var(--text);
	}
	.gorp-res-user-access-label {
		display: inline-flex;
		gap: 4px;
		align-items: center;
		color: var(--text);
		font-weight: 600;
		line-height: 1.2;
	}
	.gorp-res-user-access-help {
		color: var(--accent-2);
		font-size: 11px;
		font-weight: 700;
		line-height: 1;
	}
	.gorp-res-user-role-options {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 12px;
		align-items: center;
	}
	.gorp-res-user-role-option,
	.gorp-res-user-group-option {
		margin: 0;
		cursor: default;
	}
	.gorp-res-user-role-option {
		display: inline-flex;
		gap: 6px;
		align-items: center;
		font-weight: 600;
	}
	.gorp-res-user-role-option input,
	.gorp-res-user-group-option input {
		accent-color: var(--accent-2);
	}
	.gorp-res-user-access-select.o_input {
		width: min(100%, 360px);
		min-height: 30px;
		padding: 3px 24px 3px 0;
		border-color: transparent;
		background-color: transparent;
		color: var(--text);
		box-shadow: none;
	}
	.gorp-res-user-access-select.o_input:focus {
		border-color: var(--accent-2);
		background-color: var(--panel);
	}
	.gorp-res-user-access-extra-rights {
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 4px 36px;
	}
	.gorp-res-user-access-extra-rights h2 {
		grid-column: 1 / -1;
	}
	.gorp-res-user-access-extra-rights .gorp-res-user-access-row {
		grid-template-columns: minmax(0, 1fr) auto;
		min-height: 30px;
	}
	.gorp-form-view[data-model="res.users"] .gorp-res-user-group-ids.o_res_users_access_rights {
		grid-column: 1 / -1;
	}
	.gorp-selection-pills.o_field_selection {
		display: inline-flex;
		align-items: stretch;
		flex-wrap: wrap;
		gap: 4px;
		min-width: 0;
	}
	.gorp-selection-pill {
		display: inline-flex;
		align-items: center;
		min-height: 28px;
		padding: 4px 11px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--btn-secondary-bg);
		color: var(--text);
		font-weight: 600;
		line-height: 1.2;
	}
	.gorp-selection-pill.selected {
		border-color: var(--accent-2);
		background: rgba(1,126,132,.14);
		color: var(--accent-2);
		box-shadow: inset 0 0 0 1px rgba(1,126,132,.24);
	}
	.gorp-selection-radio-group.o_field_selection {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
		min-width: 0;
	}
	.gorp-selection-radio-pill {
		position: relative;
		display: inline-flex;
		align-items: center;
		min-height: 30px;
		padding: 5px 11px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--btn-secondary-bg);
		color: var(--text);
		font-weight: 600;
		line-height: 1.2;
		cursor: pointer;
		user-select: none;
	}
	.gorp-selection-radio-pill input {
		position: absolute;
		inset: 0;
		opacity: 0;
		cursor: pointer;
	}
	.gorp-selection-radio-pill:hover {
		border-color: rgba(113,75,103,.34);
		background: var(--hover-bg);
	}
	.gorp-selection-radio-pill.selected {
		border-color: var(--accent-2);
		background: rgba(1,126,132,.14);
		color: var(--accent-2);
		box-shadow: inset 0 0 0 1px rgba(1,126,132,.24);
	}
	.gorp-code-viewer,
	.gorp-code-editor {
		width: 100%;
		min-height: 240px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #111827;
		color: #e5edf7;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
		font-size: 13px;
		line-height: 1.5;
		tab-size: 4;
	}
	.gorp-code-viewer {
		margin: 0;
		padding: 12px;
		overflow: auto;
		white-space: pre-wrap;
	}
	.gorp-code-viewer code {
		color: inherit;
		background: transparent;
	}
	.gorp-code-editor.o_input {
		padding: 12px;
		resize: vertical;
		white-space: pre;
		overflow: auto;
		caret-color: #e5edf7;
	}
	.gorp-server-action-notebook .gorp-form-fields.o_inner_group {
		grid-template-columns: 1fr;
	}
	.gorp-server-action-help {
		display: grid;
		gap: 10px;
		padding: 12px;
		border: 1px solid var(--line-soft);
		border-radius: 4px;
		background: #fbfcfd;
	}
	.gorp-server-action-help h3 {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
	}
	.gorp-server-action-help-list {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.gorp-server-action-help-list code {
		padding: 3px 7px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		color: var(--text);
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
		font-size: 12px;
	}
	.gorp-form-notebook.o_notebook {
		margin-top: 18px;
		border-top: 1px solid var(--line);
	}
	.gorp-form-notebook-tabs.nav-tabs {
		display: flex;
		gap: 2px;
		margin: 0 0 14px;
		padding: 0;
		border-bottom: 1px solid var(--line);
		overflow-x: auto;
	}
	.gorp-form-notebook-tab.nav-link {
		position: relative;
		margin-bottom: -1px;
		padding: 10px 14px;
		border: 1px solid transparent;
		border-radius: 4px 4px 0 0;
		background: transparent;
		color: var(--muted);
		font: inherit;
		font-weight: 500;
		white-space: nowrap;
		cursor: pointer;
	}
	.gorp-form-notebook-tab.nav-link.active {
		border-color: var(--line);
		border-bottom-color: #fff;
		background: #fff;
		color: var(--text);
	}
	.gorp-form-notebook-page.tab-pane[hidden] {
		display: none !important;
	}
	.gorp-form-notebook-page .gorp-form-fields.o_inner_group {
		margin-top: 0;
	}
	.gorp-form-field.o_wrap_field {
		display: grid;
		grid-template-columns: minmax(110px, .38fr) minmax(0, 1fr);
		gap: 10px;
		align-items: start;
		min-width: 0;
		color: var(--muted);
	}
	.gorp-form-field .o_field_widget {
		min-width: 0;
		color: var(--text);
		overflow-wrap: anywhere;
	}
	.gorp-many2one-link.o_field_many2one {
		display: inline-flex;
		align-items: center;
		width: fit-content;
		max-width: 100%;
		color: #017e84;
		text-decoration: none;
		border-bottom: 1px solid transparent;
		transition: color .12s ease, border-color .12s ease;
	}
	.gorp-many2one-link.o_field_many2one:hover,
	.gorp-many2one-link.o_field_many2one:focus {
		color: #015f63;
		border-bottom-color: currentColor;
		outline: none;
	}
	.gorp-many2one-editor.o_field_many2one {
		position: relative;
		display: block;
		min-width: 0;
	}
	.gorp-settings-many2one.o_field_many2one {
		position: relative;
		display: block;
		min-width: 220px;
	}
	.gorp-many2one-editor .o_input {
		width: 100%;
		padding-right: 28px;
	}
	.gorp-many2one-editor:not([data-no-open="true"]) .o_input {
		padding-right: 56px;
	}
	.gorp-settings-many2one .o_input {
		width: 100%;
		padding-right: 28px;
	}
	.gorp-many2one-dropdown-toggle.o_dropdown_button {
		position: absolute;
		right: 0;
		top: 0;
		bottom: 0;
		width: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		border-radius: 0 4px 4px 0;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.gorp-settings-many2one-toggle.o_dropdown_button {
		position: absolute;
		right: 0;
		top: 0;
		bottom: 0;
		width: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		border-radius: 0 4px 4px 0;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.gorp-many2one-dropdown-toggle.o_dropdown_button:hover,
	.gorp-many2one-dropdown-toggle.o_dropdown_button:focus,
	.gorp-settings-many2one-toggle.o_dropdown_button:hover,
	.gorp-settings-many2one-toggle.o_dropdown_button:focus {
		color: var(--text);
		outline: none;
	}
	.gorp-many2one-open.o_external_button {
		position: absolute;
		right: 28px;
		top: 0;
		bottom: 0;
		width: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		border-left: 1px solid var(--line);
		border-radius: 0;
		background: transparent;
		color: var(--muted);
		box-shadow: none;
	}
	.gorp-many2one-open.o_external_button[hidden] {
		display: none !important;
	}
	.gorp-many2one-open.o_external_button:hover,
	.gorp-many2one-open.o_external_button:focus {
		color: var(--text);
		outline: none;
	}
	.gorp-many2one-open.o_external_button .fa {
		display: none;
	}
	.gorp-many2one-open.o_external_button::before {
		content: "";
		width: 11px;
		height: 11px;
		border: 1px solid currentColor;
		border-radius: 2px;
		box-shadow: 4px -4px 0 -2px var(--panel), 4px -4px 0 -1px currentColor;
	}
	.gorp-many2one-dropdown-toggle.o_dropdown_button::before,
	.gorp-settings-many2one-toggle.o_dropdown_button::before {
		content: "";
		width: 7px;
		height: 7px;
		border-right: 1px solid currentColor;
		border-bottom: 1px solid currentColor;
		opacity: .55;
		transform: translateY(-2px) rotate(45deg);
	}
	.gorp-many2one-dropdown.o_m2o_dropdown {
		position: absolute;
		z-index: 30;
		top: calc(100% + 2px);
		left: 0;
		right: auto;
		display: grid;
		gap: 0;
		width: max-content;
		min-width: min(100%, 160px);
		max-width: min(520px, calc(100vw - 32px));
		max-height: 320px;
		overflow: auto;
		padding: 4px 0;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--panel);
		color: var(--text);
		box-shadow: 0 12px 28px rgba(0,0,0,.14);
	}
	.gorp-many2one-option.dropdown-item,
	.gorp-many2one-create.dropdown-item,
	.gorp-many2one-create-edit.dropdown-item,
	.gorp-many2one-search-more.dropdown-item {
		width: 100%;
		min-height: 30px;
		justify-content: flex-start;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: inherit;
		padding: 6px 12px;
		text-align: left;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.gorp-many2one-search-more.dropdown-item {
		color: #017e84;
		font-weight: 500;
	}
	.gorp-many2one-option.dropdown-item:hover,
	.gorp-many2one-option.dropdown-item:focus,
	.gorp-many2one-option.dropdown-item.active,
	.gorp-many2one-option.dropdown-item.selected,
	.gorp-many2one-create.dropdown-item:hover,
	.gorp-many2one-create.dropdown-item:focus,
	.gorp-many2one-create.dropdown-item.active,
	.gorp-many2one-create-edit.dropdown-item:hover,
	.gorp-many2one-create-edit.dropdown-item:focus,
	.gorp-many2one-create-edit.dropdown-item.active,
	.gorp-many2one-search-more.dropdown-item:hover,
	.gorp-many2one-search-more.dropdown-item:focus,
	.gorp-many2one-search-more.dropdown-item.active {
		background: var(--hover-bg);
		outline: none;
	}
	.gorp-many2one-option.dropdown-item.selected {
		font-weight: 600;
	}
	.gorp-many2one-option.dropdown-item.active,
	.gorp-many2one-option.dropdown-item[data-active="true"] {
		background: rgba(1,126,132,.12);
		color: #017e84;
		outline: none;
	}
	.gorp-many2one-option.dropdown-item.selected,
	.gorp-many2one-option.dropdown-item[data-selected="true"] {
		font-weight: 600;
	}
	.gorp-many2one-option.dropdown-item.selected::before,
	.gorp-many2one-option.dropdown-item[data-selected="true"]::before {
		content: "\2713";
		display: inline-block;
		width: 16px;
		margin-right: 4px;
		color: #017e84;
		font-weight: 700;
	}
	.gorp-many2one-create.dropdown-item.active,
	.gorp-many2one-create-edit.dropdown-item.active,
	.gorp-many2one-search-more.dropdown-item.active,
	.gorp-many2one-create.dropdown-item[data-active="true"],
	.gorp-many2one-create-edit.dropdown-item[data-active="true"],
	.gorp-many2one-search-more.dropdown-item[data-active="true"] {
		background: rgba(1,126,132,.12);
		color: #017e84;
	}
	.gorp-many2one-empty {
		padding: 7px 12px;
		font-size: 13px;
	}
	.gorp-x2many-tags.o_field_widget {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		min-width: 0;
	}
	.gorp-x2many-editor.o_field_many2many_tags {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px;
		min-height: 34px;
		width: 100%;
		padding: 4px 6px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--panel);
		color: var(--text);
	}
	.gorp-x2many-editor-tags {
		display: contents;
	}
	.gorp-x2many-tag.o_tag {
		display: inline-flex;
		align-items: center;
		max-width: 100%;
		min-height: 22px;
		padding: 2px 8px;
		border: 1px solid #d9dadd;
		border-radius: 3px;
		background: #f5f6f7;
		color: var(--text);
		font-size: 12px;
		line-height: 16px;
		text-decoration: none;
		overflow-wrap: anywhere;
		transition: background .12s ease, border-color .12s ease, color .12s ease;
	}
	a.gorp-x2many-tag.o_tag:hover,
	a.gorp-x2many-tag.o_tag:focus {
		border-color: #017e84;
		background: #e8f6f7;
		color: #015f63;
		outline: none;
	}
	.gorp-x2many-editor-tag.o_tag {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		max-width: 100%;
		min-height: 22px;
		padding: 2px 5px 2px 8px;
		border: 1px solid #d9dadd;
		border-radius: 3px;
		background: #f5f6f7;
		color: var(--text);
		font-size: 12px;
		line-height: 1.25;
	}
	.gorp-x2many-editor-label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.gorp-x2many-editor-remove.o_delete {
		display: inline-grid;
		place-items: center;
		width: 16px;
		height: 16px;
		border: 0;
		border-radius: 50%;
		background: transparent;
		color: var(--muted);
		font: inherit;
		font-size: 12px;
		line-height: 1;
		cursor: pointer;
	}
	.gorp-x2many-editor-remove.o_delete:hover,
	.gorp-x2many-editor-remove.o_delete:focus {
		background: rgba(0,0,0,.08);
		color: var(--text);
		outline: none;
	}
	.gorp-x2many-editor-input.o_input {
		flex: 1 1 140px;
		min-width: 120px;
		border: 0;
		background: transparent;
		padding: 3px 2px;
		box-shadow: none;
	}
	.gorp-x2many-editor-input.o_input:focus {
		outline: none;
		box-shadow: none;
	}
	.gorp-x2many-dropdown.o_m2m_dropdown {
		position: absolute;
		z-index: 30;
		top: calc(100% + 2px);
		left: 0;
		right: 0;
		display: grid;
		gap: 0;
		max-height: 220px;
		overflow: auto;
		padding: 4px 0;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--panel);
		color: var(--text);
		box-shadow: 0 12px 28px rgba(0,0,0,.14);
	}
	.gorp-x2many-option.dropdown-item {
		width: 100%;
		min-height: 30px;
		justify-content: flex-start;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: inherit;
		padding: 6px 12px;
		text-align: left;
	}
	.gorp-x2many-option.dropdown-item:hover,
	.gorp-x2many-option.dropdown-item:focus {
		background: var(--hover-bg);
		outline: none;
	}
	.gorp-x2many-empty {
		padding: 7px 12px;
		font-size: 13px;
	}
	.gorp-one2many-editor.o_field_one2many {
		display: grid;
		gap: 6px;
		min-width: 0;
		width: 100%;
	}
	.gorp-one2many-table.o_list_table {
		width: 100%;
		border-collapse: collapse;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: var(--panel);
		overflow: hidden;
	}
	.gorp-one2many-table th,
	.gorp-one2many-table td {
		border-bottom: 1px solid var(--line);
		padding: 6px 8px;
		vertical-align: middle;
	}
	.gorp-one2many-table th {
		background: #f8f9fa;
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		text-align: left;
	}
	.gorp-one2many-table tr:last-child td {
		border-bottom: 0;
	}
	.gorp-one2many-input.o_input {
		width: 100%;
		min-width: 90px;
		border: 0;
		background: transparent;
		padding: 2px 0;
		box-shadow: none;
	}
	.gorp-one2many-input.o_input:focus {
		outline: none;
		box-shadow: inset 0 -1px 0 var(--accent);
	}
	.gorp-one2many-actions-head,
	.gorp-one2many-actions {
		width: 1%;
		white-space: nowrap;
		text-align: right;
	}
	.gorp-one2many-remove.btn,
	.gorp-one2many-add.btn {
		border: 0;
		background: transparent;
		color: var(--accent);
		padding: 2px 0;
		font-size: 13px;
	}
	.gorp-one2many-remove.btn:hover,
	.gorp-one2many-remove.btn:focus,
	.gorp-one2many-add.btn:hover,
	.gorp-one2many-add.btn:focus {
		color: #015f63;
		text-decoration: underline;
		outline: none;
	}
	.gorp-one2many-empty-row td {
		color: var(--muted);
		font-style: italic;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-body.o_form_sheet_bg {
		max-width: none;
		margin: 0;
		padding: 8px 16px 28px;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-sheet.o_form_sheet {
		width: min(100%, 1320px);
		max-width: none;
		margin: 0 auto;
		min-height: 560px;
		padding: 24px 24px 28px;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-access-smart-buttons.o_button_box {
		margin: -2px 0 26px;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-fields.o_inner_group {
		grid-template-columns: minmax(148px, .24fr) minmax(0, .76fr);
		max-width: 560px;
		gap: 10px 16px;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-notebook.o_notebook {
		margin-top: 28px;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-notebook-page .gorp-form-fields.o_inner_group {
		display: block;
		max-width: none;
	}
	.gorp-form-view[data-model="res.groups"] .gorp-form-field.gorp-groups-users-field.o_wrap_field {
		display: block;
		color: var(--text);
	}
	.gorp-form-view[data-model="res.groups"] .gorp-groups-users-field > .o_form_label {
		display: none;
	}
	.gorp-groups-users-list.o_field_one2many {
		display: block;
		width: 100%;
		min-width: 0;
	}
	.gorp-groups-users-table.o_list_table {
		width: 100%;
		border-collapse: collapse;
		border: 1px solid var(--line);
		border-radius: 0;
		background: var(--panel);
		color: var(--text);
	}
	.gorp-groups-users-table th,
	.gorp-groups-users-table td {
		height: 37px;
		padding: 7px 10px;
		border-bottom: 1px solid var(--line);
		vertical-align: middle;
		text-align: left;
	}
	.gorp-groups-users-table th {
		background: #f8f9fa;
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
	}
	.gorp-groups-users-table tr:last-child td {
		border-bottom: 0;
	}
	.gorp-groups-users-add.btn {
		border: 0;
		background: transparent;
		color: #017e84;
		padding: 0;
		font-size: 13px;
		font-weight: 500;
	}
	.gorp-groups-users-add.btn:hover,
	.gorp-groups-users-add.btn:focus {
		color: #015f63;
		text-decoration: underline;
		outline: none;
	}
	.gorp-groups-users-blank-row td {
		color: transparent;
	}
	.gorp-one2many-table td::before {
		display: none;
	}
	@media (max-width: 600px) {
		.gorp-x2many-editor.o_field_many2many_tags {
			align-items: flex-start;
			gap: 5px;
			min-height: 40px;
			padding: 6px;
		}
		.gorp-x2many-editor-tags {
			display: flex;
			flex-wrap: wrap;
			gap: 4px;
			min-width: 0;
		}
		.gorp-x2many-editor-tag.o_tag {
			max-width: 100%;
			min-height: 26px;
		}
		.gorp-x2many-editor-label {
			white-space: normal;
			overflow-wrap: anywhere;
		}
		.gorp-x2many-editor-remove.o_delete {
			width: 22px;
			height: 22px;
		}
		.gorp-x2many-editor-input.o_input {
			flex: 1 1 100%;
			min-width: 0;
			min-height: 28px;
		}
		.gorp-x2many-dropdown.o_m2m_dropdown {
			max-height: 260px;
		}
		.gorp-one2many-editor.o_field_one2many {
			gap: 8px;
		}
		.gorp-one2many-table.o_list_table {
			display: block;
			border: 0;
			background: transparent;
			overflow: visible;
		}
		.gorp-one2many-table.o_list_table thead {
			display: none;
		}
		.gorp-one2many-table.o_list_table tbody {
			display: grid;
			gap: 8px;
			width: 100%;
		}
		.gorp-one2many-table.o_list_table tr {
			display: block;
			width: 100%;
			box-sizing: border-box;
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-row {
			padding: 4px 8px;
			border: 1px solid var(--line);
			border-radius: 4px;
			background: var(--panel);
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-row td {
			display: grid;
			grid-template-columns: minmax(92px, 34%) minmax(0, 1fr);
			gap: 8px;
			align-items: center;
			width: 100%;
			padding: 8px 0;
			border-bottom: 1px solid var(--line);
			box-sizing: border-box;
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-row td::before {
			display: block;
			content: attr(data-label);
			min-width: 0;
			color: var(--muted);
			font-size: 12px;
			font-weight: 600;
			line-height: 16px;
			overflow-wrap: anywhere;
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-row td.gorp-one2many-actions {
			display: flex;
			justify-content: flex-end;
			padding-top: 6px;
			border-bottom: 0;
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-row td.gorp-one2many-actions::before {
			display: none;
			content: "";
		}
		.gorp-one2many-table.o_list_table tr.gorp-one2many-empty-row td {
			display: block;
			padding: 10px;
			border: 1px solid var(--line);
			border-radius: 4px;
			background: var(--panel);
		}
		.gorp-one2many-input.o_input,
		.gorp-one2many-readonly.o_field_widget {
			min-width: 0;
		}
		.gorp-one2many-remove.btn,
		.gorp-one2many-add.btn {
			min-height: 28px;
		}
	}
	.module-grid {
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
	}
	.module-card {
		border-radius: 4px;
		min-height: 118px;
	}
	.gorp-apps-catalog-content {
		display: grid;
		grid-template-columns: 204px minmax(0, 1fr) minmax(280px, 340px);
		gap: 16px;
		align-items: start;
		padding: 0 16px 16px;
	}
	.gorp-apps-catalog-content:has(.gorp-apps-catalog-detail[hidden]) {
		grid-template-columns: 204px minmax(0, 1fr);
	}
	.gorp-apps-searchview.o_searchview {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-width: min(440px, 44vw);
		min-height: 34px;
		padding: 0 0 0 10px;
		border: 1px solid var(--accent-2);
		border-radius: 4px;
		background: var(--control-bg);
	}
	.gorp-apps-searchview .o_searchview_icon {
		color: var(--muted);
		font-size: 18px;
		line-height: 1;
	}
	.gorp-apps-searchview .o_searchview_facet {
		display: inline-flex;
		align-items: center;
		min-height: 24px;
		padding: 2px 8px;
		border-radius: 3px;
		background: var(--accent);
		color: #fff;
		font-weight: 600;
	}
	.gorp-apps-searchview .o_searchview_input.o_input {
		flex: 1 1 auto;
		min-width: 90px;
		border: 0 !important;
		background: transparent !important;
		box-shadow: none !important;
	}
	.gorp-apps-searchview .o_searchview_dropdown_toggler {
		align-self: stretch;
		width: 28px;
		border: 0;
		border-left: 1px solid var(--line);
		border-radius: 0 3px 3px 0;
		background: transparent;
		color: var(--text);
	}
	.o_search_panel_filter,
	.o_search_panel_category {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		min-height: 30px;
		border: 0;
		border-radius: 4px;
		background: transparent;
		color: var(--text);
		padding: 5px 9px;
		text-align: left;
	}
	.o_search_panel_filter {
		border: 0;
		background: transparent;
	}
	.o_search_panel_filter:hover,
	.o_search_panel_filter.active,
	.o_search_panel_category:hover,
	.o_search_panel_category.active {
		background: var(--hover-bg);
		color: var(--accent);
	}
	.gorp-apps-catalog-sidebar {
		display: grid;
		gap: 3px;
		align-content: start;
		border: 0;
		border-right: 1px solid var(--line);
		border-radius: 0;
		background: transparent;
		padding: 0 10px 0 0;
	}
	.gorp-apps-catalog-sidebar .o_search_panel_section_header {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 30px;
		margin-top: 8px;
		padding: 5px 0;
		color: var(--text);
		font-size: 12px;
		font-weight: 800;
		letter-spacing: .02em;
		text-transform: uppercase;
	}
	.gorp-apps-catalog-sidebar .o_search_panel_section_header:first-child {
		margin-top: 0;
	}
	.gorp-apps-catalog-sidebar .o_search_panel_section_header::before {
		content: "";
		width: 12px;
		height: 10px;
		border-radius: 2px;
		background: var(--accent);
	}
	main.o_web_client .gorp-apps-catalog-sidebar {
		display: grid;
	}
	main.o_web_client .gorp-apps-catalog-detail[hidden] {
		display: none;
	}
	.o_search_panel_counter {
		color: var(--muted);
		font-size: 12px;
	}
	.gorp-apps-catalog-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 8px 16px;
		align-content: start;
		min-width: 0;
	}
	.gorp-apps-catalog-card {
		display: grid;
		grid-template-columns: 50px minmax(0, 1fr) auto;
		grid-template-areas:
			"icon title menu"
			"icon summary summary"
			"icon actions info";
		gap: 2px 12px;
		align-items: start;
		border: 1px solid var(--line);
		background: #fff;
		min-height: 94px;
		height: 94px;
		padding: 9px 10px;
	}
	.gorp-apps-catalog-card .app-icon {
		grid-area: icon;
		width: 50px;
		height: 50px;
		border-radius: 6px;
		font-size: 0;
		position: relative;
		overflow: hidden;
		box-shadow: inset 0 1px 0 rgba(255,255,255,.26);
	}
	.gorp-apps-catalog-card .app-icon[data-icon-token="sales"] { background: #ff9f43; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="website"] { background: #21d4bd; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="inventory"] { background: #ff8a5b; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="accounting"] { background: #2f80ed; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="services"] { background: #20c997; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="marketing"] { background: #22c1c3; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="hr"] { background: #ffc857; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="shipping"] { background: #00a09d; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="productivity"] { background: #714b67; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="customizations"] { background: #875a7b; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="technical"] { background: #2f80ed; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="administration"] { background: #fd7e14; }
	.gorp-apps-catalog-card .app-icon[data-icon-token="esg"] { background: #228b65; }
	.gorp-apps-catalog-card .app-icon::before {
		content: "";
		position: absolute;
		left: 11px;
		top: 10px;
		width: 24px;
		height: 28px;
		border-radius: 4px;
		background: rgba(255,255,255,.84);
		box-shadow: 10px 8px 0 rgba(255,255,255,.32);
	}
	.gorp-apps-catalog-card .app-icon::after {
		content: "";
		position: absolute;
		right: 9px;
		bottom: 9px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: rgba(25,35,55,.28);
		box-shadow: inset 0 0 0 5px rgba(255,255,255,.38);
	}
	.gorp-apps-catalog-card .app-icon {
		background: transparent;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="sale_management"] .app-icon::before {
		left: 6px;
		top: 18px;
		width: 11px;
		height: 24px;
		border-radius: 2px;
		background: #b35f9f;
		box-shadow: 11px -9px 0 #ffbf45, 22px -16px 0 #ff8a3d;
	}
	.gorp-apps-catalog-card[data-module-name="sale_management"] .app-icon::after {
		right: 6px;
		bottom: 6px;
		width: 17px;
		height: 17px;
		border-radius: 4px;
		background: rgba(255,255,255,.28);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="pos_restaurant"] .app-icon::before {
		left: 12px;
		top: 12px;
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: #a45a8d;
		box-shadow: 0 0 0 7px #ffc857;
	}
	.gorp-apps-catalog-card[data-module-name="pos_restaurant"] .app-icon::after {
		display: none;
	}
	.gorp-apps-catalog-card[data-module-name="pos_restaurant"] .app-icon {
		clip-path: polygon(0 0, 30% 0, 30% 12%, 70% 12%, 70% 0, 100% 0, 100% 30%, 88% 30%, 88% 70%, 100% 70%, 100% 100%, 70% 100%, 70% 88%, 30% 88%, 30% 100%, 0 100%, 0 70%, 12% 70%, 12% 30%, 0 30%);
		background: #ffc857;
	}
	.gorp-apps-catalog-card[data-module-name="pos_restaurant"] .app-icon::before {
		left: 12px;
		top: 12px;
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: #a45a8d;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="account"] .app-icon::before {
		left: 9px;
		top: 8px;
		width: 30px;
		height: 36px;
		border-radius: 4px;
		background: #1f9df3;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="account"] .app-icon::after {
		content: "$";
		left: 15px;
		top: 14px;
		width: 18px;
		height: 22px;
		border-radius: 50%;
		background: #34d5df;
		box-shadow: none;
		color: #fff;
		font-size: 18px;
		font-weight: 800;
		line-height: 23px;
		text-align: center;
	}
	.gorp-apps-catalog-card[data-module-name="crm"] .app-icon::before {
		left: 5px;
		top: 15px;
		width: 24px;
		height: 14px;
		border-radius: 3px;
		background: #2ed3c6;
		box-shadow: 16px 11px 0 #b45d9c;
		transform: rotate(38deg);
	}
	.gorp-apps-catalog-card[data-module-name="crm"] .app-icon::after {
		left: 18px;
		top: 18px;
		width: 18px;
		height: 12px;
		border-radius: 3px;
		background: #167abf;
		box-shadow: none;
		transform: rotate(-38deg);
	}
	.gorp-apps-catalog-card[data-module-name="website"] .app-icon::before {
		left: 4px;
		top: 9px;
		width: 40px;
		height: 30px;
		border-radius: 50%;
		background: #18d2c2;
		box-shadow: inset 0 -14px 0 #1c86d8;
	}
	.gorp-apps-catalog-card[data-module-name="website"] .app-icon::after {
		left: 5px;
		top: 19px;
		width: 38px;
		height: 10px;
		border-radius: 999px;
		background: #26566d;
		box-shadow: none;
		transform: rotate(2deg);
	}
	.gorp-apps-catalog-card[data-module-name="stock"] .app-icon::before,
	.gorp-apps-catalog-card[data-module-name="purchase"] .app-icon::before {
		left: 8px;
		top: 11px;
		width: 32px;
		height: 27px;
		border-radius: 4px;
		background: #ffbf45;
		box-shadow: inset 12px 0 0 #ff704d, inset 24px 0 0 #9b5a8c;
	}
	.gorp-apps-catalog-card[data-module-name="stock"] .app-icon::after,
	.gorp-apps-catalog-card[data-module-name="purchase"] .app-icon::after {
		left: 8px;
		top: 11px;
		width: 32px;
		height: 27px;
		border-radius: 4px;
		background: rgba(255,255,255,.18);
		box-shadow: inset 0 -10px 0 rgba(112,63,103,.82);
	}
	.gorp-apps-catalog-card[data-module-name="accountant"] .app-icon::before {
		left: 8px;
		top: 9px;
		width: 34px;
		height: 34px;
		border-radius: 50%;
		background:
			radial-gradient(circle at 9px 9px, #ffc857 0 8px, transparent 9px),
			radial-gradient(circle at 25px 25px, #22d3c5 0 9px, transparent 10px);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="accountant"] .app-icon::after {
		left: 8px;
		top: 22px;
		width: 34px;
		height: 7px;
		border-radius: 99px;
		background: #b45d9c;
		box-shadow: none;
		transform: rotate(-45deg);
	}
	.gorp-apps-catalog-card[data-module-name="equity"] .app-icon::before {
		left: 7px;
		top: 7px;
		width: 34px;
		height: 34px;
		border-radius: 50%;
		background: conic-gradient(#22cfd0 0 25%, #2f80ed 25% 50%, #ffc857 50% 82%, transparent 82%);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="equity"] .app-icon::after {
		left: 22px;
		top: 22px;
		width: 20px;
		height: 3px;
		border-radius: 99px;
		background: #252936;
		box-shadow: -16px 0 0 #252936, 0 -16px 0 #252936;
	}
	.gorp-apps-catalog-card[data-module-name="point_of_sale"] .app-icon::before {
		left: 6px;
		top: 15px;
		width: 10px;
		height: 28px;
		border-radius: 0 0 7px 7px;
		background: #ff9f2f;
		box-shadow: 10px 0 0 #ffc857, 20px 0 0 #9f5b92, 30px 0 0 #ff9f2f;
		transform: skewX(-7deg);
	}
	.gorp-apps-catalog-card[data-module-name="point_of_sale"] .app-icon::after {
		left: 6px;
		top: 13px;
		width: 36px;
		height: 6px;
		border-radius: 4px 4px 0 0;
		background: #ffc857;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="project"] .app-icon::before {
		left: 14px;
		top: 7px;
		width: 12px;
		height: 35px;
		border-radius: 2px;
		background: #23d2c5;
		box-shadow: none;
		transform: rotate(38deg);
	}
	.gorp-apps-catalog-card[data-module-name="project"] .app-icon::after {
		left: 8px;
		top: 26px;
		width: 17px;
		height: 13px;
		border-radius: 3px;
		background: #9f5b92;
		box-shadow: none;
		transform: rotate(38deg);
	}
	.gorp-apps-catalog-card[data-module-name="website_sale"] .app-icon::before {
		left: 9px;
		top: 16px;
		width: 30px;
		height: 25px;
		border-radius: 4px 4px 9px 9px;
		background: #a6538e;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="website_sale"] .app-icon::after {
		left: 17px;
		top: 10px;
		width: 14px;
		height: 14px;
		border: 3px solid #a6538e;
		border-bottom: 0;
		border-radius: 10px 10px 0 0;
		background: transparent;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="mrp"] .app-icon::before {
		left: 6px;
		top: 18px;
		width: 34px;
		height: 22px;
		border-radius: 0 0 3px 3px;
		background: #23d2c5;
		box-shadow: inset 13px 0 0 #1d9bb3, inset 27px 0 0 #ffbf45;
		transform: skewY(-18deg);
	}
	.gorp-apps-catalog-card[data-module-name="mrp"] .app-icon::after {
		left: 19px;
		top: 13px;
		width: 22px;
		height: 8px;
		border-radius: 2px;
		background: #ff704d;
		box-shadow: none;
		transform: skewY(-18deg);
	}
	.gorp-apps-catalog-card[data-module-name="mass_mailing"] .app-icon::before {
		left: 5px;
		top: 14px;
		width: 40px;
		height: 22px;
		border-radius: 2px;
		background: #1da1f2;
		clip-path: polygon(0 48%, 100% 0, 74% 100%);
		box-shadow: inset -13px -8px 0 #9b5a92;
	}
	.gorp-apps-catalog-card[data-module-name="mass_mailing"] .app-icon::after {
		left: 22px;
		top: 14px;
		width: 2px;
		height: 23px;
		border-radius: 99px;
		background: rgba(255,255,255,.55);
		box-shadow: none;
		transform: rotate(34deg);
	}
	.gorp-apps-catalog-card[data-module-name="timesheet_grid"] .app-icon::before {
		left: 8px;
		top: 8px;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: conic-gradient(#20d3c5 0 78%, #9d5b92 78% 100%);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="timesheet_grid"] .app-icon::after {
		left: 15px;
		top: 15px;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: #252936;
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="hr_expense"] .app-icon::before {
		left: 5px;
		top: 17px;
		width: 38px;
		height: 24px;
		border-radius: 11px 11px 3px 3px;
		background: #2fa7f3;
		box-shadow: inset 0 11px 0 #22c6c2;
	}
	.gorp-apps-catalog-card[data-module-name="hr_expense"] .app-icon::after {
		content: "$";
		left: 9px;
		top: 9px;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: #2fa7f3;
		box-shadow: 20px 9px 0 #2fa7f3;
		color: #fff;
		font-size: 11px;
		font-weight: 800;
		line-height: 18px;
		text-align: center;
	}
	.gorp-apps-catalog-card[data-module-name="web_studio"] .app-icon::before {
		left: 21px;
		top: 5px;
		width: 8px;
		height: 39px;
		border-radius: 2px;
		background: #2fa7f3;
		box-shadow: none;
		transform: rotate(45deg);
	}
	.gorp-apps-catalog-card[data-module-name="web_studio"] .app-icon::after {
		left: 9px;
		top: 20px;
		width: 30px;
		height: 8px;
		border-radius: 2px;
		background: #a95b92;
		box-shadow: 0 -8px 0 #a95b92;
		transform: rotate(45deg);
	}
	.gorp-apps-catalog-card[data-module-name="documents"] .app-icon::before {
		left: 12px;
		top: 9px;
		width: 24px;
		height: 31px;
		border-radius: 3px;
		background: #2fa7f3;
		box-shadow: -7px 6px 0 #22c6c2, 7px 7px 0 #ff704d;
		transform: rotate(12deg);
	}
	.gorp-apps-catalog-card[data-module-name="documents"] .app-icon::after {
		left: 18px;
		top: 12px;
		width: 16px;
		height: 24px;
		border-radius: 2px;
		background: rgba(255,255,255,.58);
		box-shadow: none;
		transform: rotate(12deg);
	}
	.gorp-apps-catalog-card[data-module-name="hr_holidays"] .app-icon::before {
		left: 5px;
		top: 10px;
		width: 38px;
		height: 24px;
		border-radius: 38px 38px 0 0;
		background: conic-gradient(from 210deg, #ff9f2f 0 33%, #ffc857 34% 50%, #1da1f2 51% 73%, #9b5a92 74%);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="hr_holidays"] .app-icon::after {
		left: 25px;
		top: 24px;
		width: 3px;
		height: 18px;
		border-radius: 99px;
		background: #9b5a92;
		box-shadow: none;
		transform: rotate(42deg);
	}
	.gorp-apps-catalog-card[data-module-name="hr_recruitment"] .app-icon::before {
		left: 8px;
		top: 8px;
		width: 30px;
		height: 30px;
		border-radius: 50%;
		background: radial-gradient(circle at 50% 50%, #252936 0 8px, #a95b92 9px 15px, #22c6c2 16px);
		box-shadow: none;
	}
	.gorp-apps-catalog-card[data-module-name="hr_recruitment"] .app-icon::after {
		left: 30px;
		top: 31px;
		width: 13px;
		height: 5px;
		border-radius: 99px;
		background: #22c6c2;
		box-shadow: none;
		transform: rotate(45deg);
	}
	.gorp-apps-catalog-card[data-module-name="hr"] .app-icon::before {
		left: 5px;
		top: 25px;
		width: 38px;
		height: 15px;
		border-radius: 10px 10px 5px 5px;
		background: #ffc857;
		box-shadow: inset -19px 0 0 #22c6c2;
	}
	.gorp-apps-catalog-card[data-module-name="hr"] .app-icon::after {
		left: 8px;
		top: 13px;
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: #ffc857;
		box-shadow: 15px -7px 0 #a95b92, 28px 1px 0 #22c6c2;
	}
	.gorp-apps-catalog-card[data-module-name="ai"] .app-icon::before {
		left: 5px;
		top: 7px;
		width: 9px;
		height: 36px;
		border-radius: 4px;
		background: #ffc857;
		box-shadow: 14px 0 0 #ff9f2f, 28px 0 0 #f6c6ef;
	}
	.gorp-apps-catalog-card[data-module-name="ai"] .app-icon::after {
		left: 12px;
		top: 29px;
		width: 24px;
		height: 5px;
		border-radius: 99px;
		background: #252936;
		box-shadow: none;
		transform: rotate(21deg);
	}
	.gorp-apps-catalog-card[data-module-name="data_recycle"] .app-icon::before {
		left: 11px;
		top: 10px;
		width: 27px;
		height: 7px;
		border-radius: 2px;
		background: #1da1f2;
		box-shadow: -2px 9px 0 #138bd6, -4px 18px 0 #0876bd, -6px 27px 0 #08659f;
		transform: skewX(-14deg);
	}
	.gorp-apps-catalog-card[data-module-name="data_recycle"] .app-icon::after {
		display: none;
	}
	.gorp-apps-catalog-card[data-module-name="databases"] .app-icon::before {
		left: 9px;
		top: 10px;
		width: 28px;
		height: 25px;
		border-radius: 50% / 28%;
		background: #ff704d;
		box-shadow: inset 0 10px 0 #ffbf45;
	}
	.gorp-apps-catalog-card[data-module-name="databases"] .app-icon::after {
		left: 10px;
		top: 22px;
		width: 28px;
		height: 20px;
		border-radius: 50% / 28%;
		background: #1da1f2;
		box-shadow: inset 0 -1px 0 rgba(0,0,0,.18);
	}
	.gorp-apps-catalog-card .o_app_name {
		grid-area: title;
		font-size: 14px;
		font-weight: 700;
		line-height: 1.2;
		color: var(--text);
	}
	.gorp-apps-catalog-card .o_app_technical_name {
		grid-area: tech;
		font-size: 12px;
		line-height: 1.25;
	}
	.gorp-apps-catalog-card .o_app_name,
	.gorp-apps-catalog-card .o_app_technical_name,
	.gorp-apps-catalog-card .o_app_summary {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.gorp-apps-catalog-card .o_app_summary {
		grid-area: summary;
		margin: 0;
		color: var(--muted);
		font-size: 12px;
		line-height: 1.2;
	}
	.gorp-apps-catalog-card .o_module_state {
		display: none;
		grid-area: state;
		justify-self: start;
		font-size: 11px;
		line-height: 18px;
		padding: 0 8px;
	}
	.gorp-apps-catalog-card .o_module_menu {
		grid-area: menu;
		justify-self: end;
		align-self: start;
		width: 18px;
		min-height: 24px;
		border: 0;
		background: transparent;
		color: var(--muted);
		font-size: 20px;
		line-height: 20px;
	}
	.gorp-apps-catalog-card .o_module_actions {
		grid-area: actions;
		display: flex;
		flex-wrap: wrap;
		gap: 4px 7px;
		align-items: center;
		margin-top: 4px;
	}
	.gorp-apps-catalog-card .o_module_actions .btn {
		min-height: 24px;
		padding: 3px 10px;
		border-radius: 2px;
		font-size: 12px;
		font-weight: 700;
		line-height: 16px;
	}
	.gorp-apps-catalog-card .o_module_info_button {
		grid-area: info;
		justify-self: end;
		align-self: end;
		min-height: 24px;
		padding: 3px 10px;
		border-radius: 2px;
		background: #f0f1f4;
		color: var(--text);
		font-size: 12px;
		font-weight: 700;
		text-decoration: none;
	}
	.gorp-apps-catalog-detail {
		display: grid;
		gap: 10px;
		border: 1px solid var(--line);
		border-radius: 4px;
		background: #fff;
		padding: 14px;
		box-shadow: 0 1px 0 rgba(16,24,40,.03);
	}
	.gorp-apps-catalog-detail h3,
	.gorp-apps-catalog-detail p {
		margin: 0;
	}
	.o_module_meta {
		display: grid;
		grid-template-columns: minmax(92px, .42fr) minmax(0, 1fr);
		gap: 5px 10px;
		margin: 0;
	}
	.o_module_meta dt {
		color: var(--muted);
		font-weight: 600;
	}
	.o_module_meta dd {
		margin: 0;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.record-panel {
		padding: 0;
		background: transparent;
	}
	.record-panel .o-list-content {
		padding-top: 0;
	}
	.o-form-control .o-breadcrumbs {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1 1 auto;
		min-width: 0;
	}
	.o-form-control .o-breadcrumbs button {
		flex: 0 1 auto;
		min-width: 0;
		max-width: 36vw;
		padding: 0;
		background: transparent;
		border: 0;
		color: var(--accent);
		font-size: 18px;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.o-form-control .o-breadcrumbs span {
		flex: 0 0 auto;
		color: var(--muted);
	}
	.o-form-control .o-breadcrumbs h2 {
		flex: 1 1 auto;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.record-panel .o_control_panel_main {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 8px 16px;
	}
	.record-panel .o_control_panel_main_buttons {
		grid-column: 1;
		grid-row: 1;
		flex-wrap: nowrap;
		white-space: nowrap;
	}
	.record-panel .o_control_panel_breadcrumbs {
		grid-column: 2;
		grid-row: 1;
		min-width: 0;
	}
	.record-panel .o_control_panel_actions {
		display: none;
	}
	.record-panel .o_control_panel_navigation {
		grid-column: 3;
		grid-row: 1;
		min-width: 0;
		justify-content: flex-end;
	}
	.record-panel .o_control_panel_meta {
		display: none;
	}
	.o-form-content {
		max-width: 980px;
		margin: 0 auto;
	}
	#recordForm {
		background: #fff;
		border: 1px solid var(--line);
		border-radius: 4px;
		padding: 20px;
		box-shadow: 0 1px 0 rgba(16,24,40,.03);
	}
	#recordForm label {
		color: var(--muted);
		font-size: 12px;
	}
	#recordForm input {
		margin-top: 5px;
	}
	main.o_web_client[data-theme="enterprise-like"],
	body[data-theme="enterprise"] {
		--bg: #1b1d27;
		--panel: #282a35;
		--panel-soft: #343743;
		--text: #f5f5f7;
		--muted: #b0b5c4;
		--line: #3c404e;
		--line-soft: #303440;
		--hover-bg: #353846;
		--control-bg: #282a35;
		--control-shadow: 0 1px 0 rgba(0,0,0,.18);
		--dropdown-shadow: 0 14px 28px rgba(0,0,0,.46);
		--list-head: #1b1d27;
		--btn-secondary-bg: #333744;
		--btn-secondary-hover: #414454;
		--accent: #875a7b;
		--accent-2: #00a09d;
		--topbar: #282a35;
		--topbar-hover: #353846;
		--sidebar: #1b1d27;
			--home-bg: #070b12;
			--home-bg-image: url("data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxOTIwIDEwODAiIHByZXNlcnZlQXNwZWN0UmF0aW89InhNaWRZTWlkIHNsaWNlIj48cmVjdCB3aWR0aD0iMTkyMCIgaGVpZ2h0PSIxMDgwIiBmaWxsPSIjMDcwYjEyIi8+PHBhdGggZD0iTS0xMjAgMCBDMzIwIDcwIDY0MCAyMTAgOTQwIDQyMCBDMTI0MCA2MzAgMTU0MCA1NjAgMjA0MCAzNDAgTDIwNDAgLTEyMCBMLTEyMCAtMTIwIFoiIGZpbGw9IiMxNzMxM2QiIG9wYWNpdHk9Ii40NiIvPjxwYXRoIGQ9Ik0tMjIwIDkwMCBDMjYwIDc0MCA2NzAgODgwIDEwNDAgMTAxMCBDMTM5MCAxMTMyIDE2NTAgOTgwIDIwNDAgOTIwIEwyMDQwIDEyMDAgTC0yMjAgMTIwMCBaIiBmaWxsPSIjMGUyNjM0IiBvcGFjaXR5PSIuNjIiLz48cGF0aCBkPSJNMTM0MCAtMTYwIEMxNTEwIDEzMCAxNjAwIDM0MCAxOTIwIDQ3MCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjMjQyMzM3IiBzdHJva2Utd2lkdGg9IjE5MCIgb3BhY2l0eT0iLjMyIi8+PHBhdGggZD0iTS0xMjAgNDQwIEMzNDAgMzAwIDc2MCAzODAgMTEyMCA1MjAgQzE0NjAgNjUwIDE3MjAgNTIwIDIwNDAgNDQwIiBmaWxsPSJub25lIiBzdHJva2U9IiMxMTE4MjciIHN0cm9rZS13aWR0aD0iMTgwIiBvcGFjaXR5PSIuMzgiLz48ZyBmaWxsPSIjMzM0MTU1IiBvcGFjaXR5PSIuNDIiPjxjaXJjbGUgY3g9IjIyNSIgY3k9IjI2MCIgcj0iMTAiLz48Y2lyY2xlIGN4PSI1NzAiIGN5PSI4MzUiIHI9IjciLz48Y2lyY2xlIGN4PSIxMTI1IiBjeT0iMzk1IiByPSI4Ii8+PGNpcmNsZSBjeD0iMTcyMCIgY3k9IjIyNSIgcj0iNyIvPjxjaXJjbGUgY3g9IjE4MzAiIGN5PSI3OTAiIHI9IjEyIi8+PC9nPjwvc3ZnPg==");
			--home-panel: rgba(255,255,255,.08);
			--home-line: rgba(255,255,255,.14);
			--home-text: #ffffff;
			--home-muted: #d7dce6;
	}
	main.o_web_client[data-theme="enterprise-like"] {
		color: var(--text);
		background: var(--bg);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-link.o_field_many2one {
		color: #017e84;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-link.o_field_many2one:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-link.o_field_many2one:focus {
		color: #015f63;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-dropdown.o_m2o_dropdown {
		border-color: var(--line);
		background: #fff;
		color: var(--text);
		box-shadow: var(--dropdown-shadow);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-search-more.dropdown-item {
		color: #017e84;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-option.dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-option.dropdown-item:focus,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-option.dropdown-item.active,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-option.dropdown-item.selected,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create.dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create.dropdown-item:focus,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create.dropdown-item.active,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create-edit.dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create-edit.dropdown-item:focus,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-create-edit.dropdown-item.active,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-search-more.dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-search-more.dropdown-item:focus,
	main.o_web_client[data-theme="enterprise-like"] .gorp-many2one-search-more.dropdown-item.active {
		background: var(--hover-bg);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-tag.o_tag {
		border-color: #cfd4dc;
		background: #eef2f3;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-editor.o_field_many2many_tags {
		border-color: var(--line);
		background: #fff;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-editor-tag.o_tag {
		border-color: #cfd4dc;
		background: #eef2f3;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-editor-remove.o_delete:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-editor-remove.o_delete:focus {
		background: #ddeff0;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-dropdown.o_m2m_dropdown {
		border-color: var(--line);
		background: #fff;
		color: var(--text);
		box-shadow: var(--dropdown-shadow);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-option.dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-x2many-option.dropdown-item:focus {
		background: var(--hover-bg);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-table.o_list_table {
		border-color: var(--line);
		background: #fff;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-table th,
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-table td {
		border-bottom-color: var(--line);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-table th {
		background: var(--list-head);
		color: var(--muted);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-input.o_input {
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-remove.btn,
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-add.btn {
		color: #017e84;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-remove.btn:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-remove.btn:focus,
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-add.btn:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-one2many-add.btn:focus {
		color: #015f63;
	}
	main.o_web_client[data-theme="enterprise-like"] a.gorp-x2many-tag.o_tag:hover,
	main.o_web_client[data-theme="enterprise-like"] a.gorp-x2many-tag.o_tag:focus {
		border-color: #017e84;
		background: #e6f2f3;
		color: #015f63;
	}
	main.o_web_client[data-theme="enterprise-like"] > .o_navbar > .o_main_navbar,
	body[data-theme="enterprise"] > header {
		background: var(--topbar);
		border-bottom-color: rgba(0,0,0,.16);
		box-shadow: 0 1px 2px rgba(16,24,40,.12);
	}
	main.o_web_client[data-theme="enterprise-like"] > .o_action_manager,
	main.o_web_client[data-theme="enterprise-like"] .o-list-content,
	main.o_web_client[data-theme="enterprise-like"] .o_settings_content,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-body.o_form_sheet_bg,
	body[data-theme="enterprise"] .o-list-content,
	body[data-theme="enterprise"] .o_settings_content,
	body[data-theme="enterprise"] .gorp-form-body.o_form_sheet_bg {
		background: var(--bg);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_action_manager {
		background: transparent;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-control-panel,
	main.o_web_client[data-theme="enterprise-like"] .o_control_panel,
	body[data-theme="enterprise"] .o-control-panel,
	body[data-theme="enterprise"] .o_control_panel {
		background: var(--control-bg);
		border-bottom-color: var(--line);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) .o_control_panel {
		min-height: 56px;
		padding: 8px 16px !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-control-panel h2,
	main.o_web_client[data-theme="enterprise-like"] .o_control_panel h2,
	main.o_web_client[data-theme="enterprise-like"] .o_breadcrumb,
	body[data-theme="enterprise"] .o-control-panel h2,
	body[data-theme="enterprise"] .o_control_panel h2,
	body[data-theme="enterprise"] .o_breadcrumb {
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] input,
	main.o_web_client[data-theme="enterprise-like"] select,
	main.o_web_client[data-theme="enterprise-like"] textarea,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview_dropdown_toggler,
	body[data-theme="enterprise"] input,
	body[data-theme="enterprise"] select,
	body[data-theme="enterprise"] textarea,
	body[data-theme="enterprise"] .o_searchview,
	body[data-theme="enterprise"] .o_searchview_dropdown_toggler {
		background: #202330;
		border-color: #4a5061;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] input::placeholder,
	body[data-theme="enterprise"] input::placeholder {
		color: #8d95a6;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_search_options,
	main.o_web_client[data-theme="enterprise-like"] .dropdown-menu,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-items,
	body[data-theme="enterprise"] .o_search_options,
	body[data-theme="enterprise"] .dropdown-menu,
	body[data-theme="enterprise"] .gorp-action-menu-items {
		background: #282b38;
		border-color: var(--line);
		color: var(--text);
		box-shadow: var(--dropdown-shadow);
	}
	main.o_web_client[data-theme="enterprise-like"] .dropdown-item,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-item,
	main.o_web_client[data-theme="enterprise-like"] .o_menu_item,
	body[data-theme="enterprise"] .dropdown-item,
	body[data-theme="enterprise"] .gorp-action-menu-item,
	body[data-theme="enterprise"] .o_menu_item {
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .dropdown-item.active,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-item:hover:not(:disabled),
	main.o_web_client[data-theme="enterprise-like"] .o_menu_item:hover,
	main.o_web_client[data-theme="enterprise-like"] .o_menu_item.active,
	body[data-theme="enterprise"] .dropdown-item:hover,
	body[data-theme="enterprise"] .dropdown-item.active,
	body[data-theme="enterprise"] .gorp-action-menu-item:hover:not(:disabled),
	body[data-theme="enterprise"] .o_menu_item:hover,
	body[data-theme="enterprise"] .o_menu_item.active {
		background: var(--hover-bg);
		color: #7dd3c7;
	}
	main.o_web_client[data-theme="enterprise-like"] .btn-secondary,
	main.o_web_client[data-theme="enterprise-like"] .btn-outline-secondary,
	main.o_web_client[data-theme="enterprise-like"] button.secondary,
	body[data-theme="enterprise"] .btn-secondary,
	body[data-theme="enterprise"] .btn-outline-secondary,
	body[data-theme="enterprise"] button.secondary {
		background: var(--btn-secondary-bg);
		border-color: var(--line);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .btn-secondary:hover,
	main.o_web_client[data-theme="enterprise-like"] .btn-outline-secondary:hover,
	main.o_web_client[data-theme="enterprise-like"] button.secondary:hover,
	body[data-theme="enterprise"] .btn-secondary:hover,
	body[data-theme="enterprise"] .btn-outline-secondary:hover,
	body[data-theme="enterprise"] button.secondary:hover {
		background: var(--btn-secondary-hover);
		border-color: #565c6f;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view table,
	body[data-theme="enterprise"] .o-list-view table {
		border-color: var(--line);
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view th,
	main.o_web_client[data-theme="enterprise-like"] .o-list-view td,
	body[data-theme="enterprise"] .o-list-view th,
	body[data-theme="enterprise"] .o-list-view td {
		background: #282b38;
		border-color: var(--line-soft);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view th,
	body[data-theme="enterprise"] .o-list-view th {
		background: #1c1f2a;
		color: #e5e8ef;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view tr:hover td,
	body[data-theme="enterprise"] .o-list-view tr:hover td {
		background: #323746;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_data_row_selected td,
	body[data-theme="enterprise"] .o_data_row_selected td {
		background: rgba(0,160,157,.16) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_sidebar,
	body[data-theme="enterprise"] .o_settings_sidebar {
		background: #181b25;
		border-color: var(--line);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab,
	body[data-theme="enterprise"] .o_settings_tab {
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab.active,
	body[data-theme="enterprise"] .o_settings_tab.active {
		background: rgba(0,160,157,.14);
		color: #f7fbff;
		box-shadow: inset 3px 0 0 #00a09d;
	}
	main.o_web_client[data-theme="enterprise-like"] .app_settings_block,
	main.o_web_client[data-theme="enterprise-like"] .o_setting_box,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record,
	main.o_web_client[data-theme="enterprise-like"] .o_mobile_record_card,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog-sidebar,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog-card,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog-detail,
	body[data-theme="enterprise"] .app_settings_block,
	body[data-theme="enterprise"] .o_setting_box,
	body[data-theme="enterprise"] .gorp-form-sheet.o_form_sheet,
	body[data-theme="enterprise"] .o_kanban_group,
	body[data-theme="enterprise"] .o_kanban_record,
	body[data-theme="enterprise"] .o_mobile_record_card,
	body[data-theme="enterprise"] .gorp-apps-catalog-sidebar,
	body[data-theme="enterprise"] .gorp-apps-catalog-card,
	body[data-theme="enterprise"] .gorp-apps-catalog-detail {
		background: var(--panel);
		border-color: var(--line);
		color: var(--text);
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_block_title,
	main.o_web_client[data-theme="enterprise-like"] .o_settings_app_title,
	main.o_web_client[data-theme="enterprise-like"] .o_setting_right_pane .o_form_label,
	body[data-theme="enterprise"] .o_settings_block_title,
	body[data-theme="enterprise"] .o_settings_app_title,
	body[data-theme="enterprise"] .o_setting_right_pane .o_form_label {
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .muted,
	main.o_web_client[data-theme="enterprise-like"] .text-muted,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_field_label,
	main.o_web_client[data-theme="enterprise-like"] .o_mobile_record_label,
	body[data-theme="enterprise"] .muted,
	body[data-theme="enterprise"] .text-muted,
	body[data-theme="enterprise"] .o_kanban_field_label,
	body[data-theme="enterprise"] .o_mobile_record_label {
		color: var(--muted);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_quick_add,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_load_more,
	body[data-theme="enterprise"] .o_kanban_quick_add,
	body[data-theme="enterprise"] .o_kanban_load_more {
		background: rgba(255,255,255,.04);
		border-color: var(--line);
		color: #00a09d;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_quick_add:hover,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_quick_add:focus,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_load_more:hover,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_load_more:focus,
	body[data-theme="enterprise"] .o_kanban_quick_add:hover,
	body[data-theme="enterprise"] .o_kanban_quick_add:focus,
	body[data-theme="enterprise"] .o_kanban_load_more:hover,
	body[data-theme="enterprise"] .o_kanban_load_more:focus {
		background: rgba(0,160,157,.1);
		border-color: rgba(0,160,157,.4);
		color: #45c4c1;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record_menu_toggle,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group_fold_toggle,
	body[data-theme="enterprise"] .o_kanban_record_menu_toggle {
		color: var(--muted);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record:hover .o_kanban_record_menu_toggle,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record_menu_toggle[aria-expanded="true"],
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record_menu_toggle:focus,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group_fold_toggle:hover,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group_fold_toggle:focus,
	body[data-theme="enterprise"] .o_kanban_record:hover .o_kanban_record_menu_toggle,
	body[data-theme="enterprise"] .o_kanban_record_menu_toggle[aria-expanded="true"],
	body[data-theme="enterprise"] .o_kanban_record_menu_toggle:focus,
	body[data-theme="enterprise"] .o_kanban_group_fold_toggle:hover,
	body[data-theme="enterprise"] .o_kanban_group_fold_toggle:focus {
		background: rgba(0,160,157,.1);
		color: #45c4c1;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group.o_kanban_group_drop_target,
	body[data-theme="enterprise"] .o_kanban_group.o_kanban_group_drop_target {
		border-color: rgba(0,160,157,.48);
		background: rgba(0,160,157,.08);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_records.o_kanban_records_drop_target,
	body[data-theme="enterprise"] .o_kanban_records.o_kanban_records_drop_target {
		outline-color: rgba(0,160,157,.52);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group_load_more,
	body[data-theme="enterprise"] .o_kanban_group_load_more {
		background: rgba(1,126,132,.06);
		border-color: rgba(1,126,132,.24);
		color: #017e84;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view {
		background-color: var(--home-bg) !important;
		background-image: var(--home-bg-image) !important;
		background-attachment: fixed;
		background-position: center;
		background-repeat: no-repeat;
		background-size: cover;
		box-shadow: inset 0 1px 0 rgba(255,255,255,.66);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-app-search { margin-bottom: 0; }
	main.o_web_client[data-theme="enterprise-like"] .o-app-search.is-active { margin: 8px auto 48px; }
	main.o_web_client[data-theme="enterprise-like"] .o-app-search input {
		background: #fff;
		border-color: #cfd4dc;
		color: #1f2933;
		box-shadow: 0 8px 18px rgba(16,24,40,.08);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_home_menu_registration_banner {
		display: none !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view .o_app {
		border-color: transparent;
		background: transparent;
		color: var(--home-text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view .o_app:hover,
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view .o_app:focus-visible {
		background: rgba(17,24,39,.06);
		border-color: transparent;
		color: var(--home-text);
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 20;
		background: transparent;
		border-bottom-color: transparent;
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_sections,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o-mobile-menu-toggle,
	body[data-theme="enterprise"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_sections,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o-mobile-menu-toggle {
		display: none;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	body[data-theme="enterprise"][data-view="apps"][data-home-menu-mode="overlay"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu {
		display: inline-flex;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray {
		height: 46px;
		padding-right: 14px;
		background: transparent;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item {
		min-height: 46px;
		background: transparent;
		border-color: transparent;
		color: var(--home-text);
		text-shadow: none;
	}
	@media (min-width: 621px) {
		main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_mail_systray_item,
		main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_activity_menu {
			display: inline-flex !important;
		}
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item:hover,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item:focus-visible,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item:hover,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item:focus-visible {
		background: rgba(255,255,255,.08);
		color: var(--home-text);
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o_user_menu_name,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o_user_menu_name {
		display: inline;
	}
	@media (max-width: 620px) {
		main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar .o_user_menu_name,
		body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar .o_user_menu_name {
			display: none;
		}
	}
	main.o_web_client[data-theme="enterprise-like"],
	body[data-theme="enterprise"] {
		--bg: #1b1d27;
		--panel: #282a35;
		--panel-soft: #343743;
		--text: #f5f5f7;
		--muted: #b0b5c4;
		--line: #3c404e;
		--line-soft: #303440;
		--hover-bg: #353846;
		--control-bg: #282a35;
		--dropdown-shadow: 0 14px 28px rgba(0,0,0,.46);
		--list-head: #1b1d27;
		--btn-secondary-bg: #333744;
		--btn-secondary-hover: #414454;
		--accent: #875a7b;
		--accent-2: #00a09d;
		--topbar: #282a35;
		--topbar-hover: #353846;
		--sidebar: #1b1d27;
		color: var(--text);
		background: var(--bg);
	}
	:root,
	main.o_web_client[data-theme="enterprise-like"],
	body[data-theme="enterprise"] {
		--bg: #1b1d27;
		--panel: #282a35;
		--panel-soft: #343743;
		--text: #f5f5f7;
		--muted: #b0b5c4;
		--line: #3c404e;
		--line-soft: #303440;
		--hover-bg: #353846;
		--control-bg: #282a35;
		--control-shadow: 0 1px 0 rgba(0,0,0,.18);
		--dropdown-shadow: 0 14px 28px rgba(0,0,0,.46);
		--list-head: #1b1d27;
		--btn-secondary-bg: #333744;
		--btn-secondary-hover: #414454;
		--accent: #875a7b;
		--accent-2: #00a09d;
		--accent-text: #ffffff;
		--topbar: #282a35;
		--topbar-hover: #353846;
		--sidebar: #1b1d27;
		--home-bg: #070b12;
		--home-bg-image: url("data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxOTIwIDEwODAiIHByZXNlcnZlQXNwZWN0UmF0aW89InhNaWRZTWlkIHNsaWNlIj48cmVjdCB3aWR0aD0iMTkyMCIgaGVpZ2h0PSIxMDgwIiBmaWxsPSIjMDcwYjEyIi8+PHBhdGggZD0iTS0xMjAgMCBDMzIwIDcwIDY0MCAyMTAgOTQwIDQyMCBDMTI0MCA2MzAgMTU0MCA1NjAgMjA0MCAzNDAgTDIwNDAgLTEyMCBMLTEyMCAtMTIwIFoiIGZpbGw9IiMxNzMxM2QiIG9wYWNpdHk9Ii40NiIvPjxwYXRoIGQ9Ik0tMjIwIDkwMCBDMjYwIDc0MCA2NzAgODgwIDEwNDAgMTAxMCBDMTM5MCAxMTMyIDE2NTAgOTgwIDIwNDAgOTIwIEwyMDQwIDEyMDAgTC0yMjAgMTIwMCBaIiBmaWxsPSIjMGUyNjM0IiBvcGFjaXR5PSIuNjIiLz48cGF0aCBkPSJNMTM0MCAtMTYwIEMxNTEwIDEzMCAxNjAwIDM0MCAxOTIwIDQ3MCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjMjQyMzM3IiBzdHJva2Utd2lkdGg9IjE5MCIgb3BhY2l0eT0iLjMyIi8+PHBhdGggZD0iTS0xMjAgNDQwIEMzNDAgMzAwIDc2MCAzODAgMTEyMCA1MjAgQzE0NjAgNjUwIDE3MjAgNTIwIDIwNDAgNDQwIiBmaWxsPSJub25lIiBzdHJva2U9IiMxMTE4MjciIHN0cm9rZS13aWR0aD0iMTgwIiBvcGFjaXR5PSIuMzgiLz48ZyBmaWxsPSIjMzM0MTU1IiBvcGFjaXR5PSIuNDIiPjxjaXJjbGUgY3g9IjIyNSIgY3k9IjI2MCIgcj0iMTAiLz48Y2lyY2xlIGN4PSI1NzAiIGN5PSI4MzUiIHI9IjciLz48Y2lyY2xlIGN4PSIxMTI1IiBjeT0iMzk1IiByPSI4Ii8+PGNpcmNsZSBjeD0iMTcyMCIgY3k9IjIyNSIgcj0iNyIvPjxjaXJjbGUgY3g9IjE4MzAiIGN5PSI3OTAiIHI9IjEyIi8+PC9nPjwvc3ZnPg==");
		--home-panel: rgba(255,255,255,.08);
		--home-line: rgba(255,255,255,.14);
		--home-text: #ffffff;
		--home-muted: #d7dce6;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar {
		background: #262a36;
		color: #e4e4e4;
		border-bottom: 1px solid var(--line);
		box-shadow: 0 1px 2px rgba(16,24,40,.08);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-nav,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_menu_systray {
		align-items: center;
		align-self: stretch;
		height: 46px;
		max-height: 46px;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_menu_toggle,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_entry,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-systray-item {
		background: transparent;
		border-color: transparent;
		color: #e4e4e4;
		height: 26px;
		min-height: 26px;
		max-height: 26px;
		align-self: center;
		padding-top: 0;
		padding-bottom: 0;
		line-height: 26px;
		text-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_menu_toggle {
		color: var(--accent);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-launcher span {
		background: var(--accent);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-launcher-button:hover,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-launcher-button.active {
		background: var(--hover-bg);
	}
	main.o_web_client[data-theme="enterprise-like"] > .o_action_manager,
	main.o_web_client[data-theme="enterprise-like"] .gorp-window-action,
	main.o_web_client[data-theme="enterprise-like"] .o-list-content,
	main.o_web_client[data-theme="enterprise-like"] .o-form-content,
	main.o_web_client[data-theme="enterprise-like"] .o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_view,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_renderer,
	main.o_web_client[data-theme="enterprise-like"] .o_apps_view,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog,
	main.o_web_client[data-theme="enterprise-like"] .o_settings_content {
		background: var(--bg) !important;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_control_panel,
	main.o_web_client[data-theme="enterprise-like"] .o-control-panel {
		background: var(--control-bg);
		border-bottom: 1px solid var(--line);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view .o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-statusbar,
	main.o_web_client[data-theme="enterprise-like"] .o_notebook,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog-card,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_group,
	main.o_web_client[data-theme="enterprise-like"] .o_module_card {
		background: var(--panel) !important;
		border-color: var(--line) !important;
		color: var(--text);
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"] table,
	main.o_web_client[data-theme="enterprise-like"] thead,
	main.o_web_client[data-theme="enterprise-like"] tbody,
	main.o_web_client[data-theme="enterprise-like"] .gorp-list-view,
	main.o_web_client[data-theme="enterprise-like"] .gorp-list-view th,
	main.o_web_client[data-theme="enterprise-like"] .gorp-list-view td {
		background: var(--panel) !important;
		border-color: var(--line) !important;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-list-view th,
	main.o_web_client[data-theme="enterprise-like"] .o_list_renderer thead {
		background: var(--list-head) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-list-view tr:hover td,
	main.o_web_client[data-theme="enterprise-like"] .o_data_row:hover td,
	main.o_web_client[data-theme="enterprise-like"] .o_kanban_record:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-apps-catalog-card:hover {
		background: var(--hover-bg) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] input,
	main.o_web_client[data-theme="enterprise-like"] select,
	main.o_web_client[data-theme="enterprise-like"] textarea,
	main.o_web_client[data-theme="enterprise-like"] .form-control,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview {
		background: var(--control-bg) !important;
		border-color: var(--line) !important;
		color: var(--text) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] input::placeholder,
	main.o_web_client[data-theme="enterprise-like"] textarea::placeholder {
		color: var(--muted);
	}
	main.o_web_client[data-theme="enterprise-like"] .btn-secondary,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-toggle,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview_dropdown_toggler,
	main.o_web_client[data-theme="enterprise-like"] .o_cp_switch_buttons button {
		background: var(--btn-secondary-bg) !important;
		border-color: var(--line) !important;
		color: var(--text) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .btn-secondary:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-toggle:hover,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview_dropdown_toggler:hover {
		background: var(--btn-secondary-hover) !important;
		color: var(--text) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .dropdown-menu,
	main.o_web_client[data-theme="enterprise-like"] .o-dropdown--menu,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-items,
	main.o_web_client[data-theme="enterprise-like"] .o_search_options,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview_autocomplete {
		background: var(--panel) !important;
		border-color: var(--line) !important;
		color: var(--text);
		box-shadow: var(--dropdown-shadow);
	}
	main.o_web_client[data-theme="enterprise-like"] .dropdown-item,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-item {
		color: var(--text) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_menu_sections .o_navbar_dropdown_menu {
		max-height: min(70vh, 630px);
		padding: 6px 0;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_menu_sections .o_navbar_dropdown_header {
		padding: 8px 18px 4px;
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		line-height: 1.2;
		text-transform: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_menu_sections .o_navbar_dropdown_item {
		height: 28px;
		min-height: 28px;
		padding: 4px 32px;
		font-size: 14px;
		font-weight: 400;
		line-height: 20px;
	}
	main.o_web_client[data-theme="enterprise-like"] .dropdown-item:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-item:hover:not(:disabled) {
		background: var(--hover-bg) !important;
		color: #45c4c1 !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .o_setting_container,
	main.o_web_client[data-theme="enterprise-like"] .o_setting_box,
	main.o_web_client[data-theme="enterprise-like"] .app_settings_block,
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab {
		background: transparent;
		color: var(--text);
		border-color: var(--line);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab[aria-pressed="true"],
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab.active {
		background: rgba(0,160,157,.12);
		box-shadow: inset 4px 0 0 var(--accent-2);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view .o_app:hover,
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view .o_app:focus-visible {
		background: rgba(255,255,255,.08) !important;
		color: var(--home-text);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_entry:hover,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_entry.active,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-systray-item:hover,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o-systray-item:focus-visible {
		background: var(--hover-bg);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] input,
	main.o_web_client[data-theme="enterprise-like"] select,
	main.o_web_client[data-theme="enterprise-like"] textarea,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview,
	main.o_web_client[data-theme="enterprise-like"] .o_searchview_dropdown_toggler,
	main.o_web_client[data-theme="enterprise-like"] .o_search_options,
	main.o_web_client[data-theme="enterprise-like"] .dropdown-menu,
	main.o_web_client[data-theme="enterprise-like"] .gorp-action-menu-items,
	body[data-theme="enterprise"] input,
	body[data-theme="enterprise"] select,
	body[data-theme="enterprise"] textarea,
	body[data-theme="enterprise"] .o_searchview,
	body[data-theme="enterprise"] .o_searchview_dropdown_toggler,
	body[data-theme="enterprise"] .o_search_options,
	body[data-theme="enterprise"] .dropdown-menu,
	body[data-theme="enterprise"] .gorp-action-menu-items {
		background: var(--control-bg);
		border-color: var(--line);
		color: var(--text);
		box-shadow: var(--control-shadow);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view th,
	body[data-theme="enterprise"] .o-list-view th {
		background: var(--list-head);
		color: var(--muted);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view td,
	body[data-theme="enterprise"] .o-list-view td {
		background: var(--panel);
		color: var(--text);
		border-color: var(--line-soft);
	}
	main.o_web_client[data-theme="enterprise-like"] .o-list-view tr:hover td,
	body[data-theme="enterprise"] .o-list-view tr:hover td {
		background: var(--hover-bg);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_sidebar,
	body[data-theme="enterprise"] .o_settings_sidebar {
		background: var(--sidebar);
		border-color: var(--line);
	}
	main.o_web_client[data-theme="enterprise-like"] .o_settings_tab.active,
	body[data-theme="enterprise"] .o_settings_tab.active {
		background: rgba(1,126,132,.10);
		color: var(--text);
		box-shadow: inset 3px 0 0 var(--accent-2);
	}
	.o-app-launcher-view .o_app_icon_with_glyph .o_app_icon_glyph {
		position: relative;
		z-index: 1;
		display: inline-grid;
		place-items: center;
		width: 42px;
		height: 42px;
		color: inherit;
		line-height: 1;
	}
	.o-app-launcher-view .o_app_icon_glyph::before,
	.o-app-launcher-view .o_app_icon_glyph::after {
		content: "";
		position: absolute;
		pointer-events: none;
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-cog::before {
		inset: 6px;
		background: currentColor;
		clip-path: polygon(50% 0, 62% 18%, 82% 12%, 88% 33%, 100% 50%, 88% 67%, 82% 88%, 62% 82%, 50% 100%, 38% 82%, 18% 88%, 12% 67%, 0 50%, 12% 33%, 18% 12%, 38% 18%);
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-cog::after {
		inset: 17px;
		border-radius: 50%;
		background: var(--app-icon-bg, transparent);
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-check-square-o::before {
		inset: 7px;
		border: 4px solid currentColor;
		border-radius: 8px;
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-check-square-o::after {
		left: 17px;
		top: 19px;
		width: 18px;
		height: 9px;
		border-left: 4px solid currentColor;
		border-bottom: 4px solid currentColor;
		border-radius: 1px;
		transform: rotate(-45deg);
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-exchange::before {
		left: 8px;
		top: 12px;
		width: 27px;
		height: 7px;
		border-top: 4px solid currentColor;
		border-right: 4px solid currentColor;
		transform: skewX(-25deg);
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-exchange::after {
		right: 8px;
		bottom: 12px;
		width: 27px;
		height: 7px;
		border-bottom: 4px solid currentColor;
		border-left: 4px solid currentColor;
		transform: skewX(-25deg);
	}
	.o-app-launcher-view .o_app_icon_glyph.fa-th-large::before {
		left: 8px;
		top: 8px;
		width: 11px;
		height: 11px;
		border-radius: 3px;
		background: currentColor;
		box-shadow: 15px 0 0 currentColor, 0 15px 0 currentColor, 15px 15px 0 currentColor;
	}
	main.o_web_client[data-view="apps"],
	body[data-view="apps"],
	body.o_home_menu_background {
		--home-bg: #080b15;
		--home-bg-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 1920 1080' preserveAspectRatio='none'%3E%3Cdefs%3E%3ClinearGradient id='g' x1='0' y1='0' x2='1' y2='1'%3E%3Cstop offset='0' stop-color='%231b4650'/%3E%3Cstop offset='.28' stop-color='%23122532'/%3E%3Cstop offset='.66' stop-color='%23111525'/%3E%3Cstop offset='1' stop-color='%23080b15'/%3E%3C/linearGradient%3E%3CradialGradient id='r' cx='.34' cy='.05' r='.55'%3E%3Cstop offset='0' stop-color='%23ffffff' stop-opacity='.11'/%3E%3Cstop offset='.48' stop-color='%23ffffff' stop-opacity='.025'/%3E%3Cstop offset='1' stop-color='%23000000' stop-opacity='0'/%3E%3C/radialGradient%3E%3ClinearGradient id='beam' x1='0' y1='0' x2='.55' y2='.35'%3E%3Cstop offset='0' stop-color='%23ffffff' stop-opacity='.08'/%3E%3Cstop offset='1' stop-color='%23ffffff' stop-opacity='0'/%3E%3C/linearGradient%3E%3ClinearGradient id='v' x1='0' y1='0' x2='0' y2='1'%3E%3Cstop offset='0' stop-color='%23ffffff' stop-opacity='.04'/%3E%3Cstop offset='1' stop-color='%23000000' stop-opacity='.42'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect width='1920' height='1080' fill='url(%23g)'/%3E%3Cpath d='M0 0h245L0 470Z' fill='url(%23beam)'/%3E%3Crect width='1920' height='1080' fill='url(%23r)'/%3E%3Crect width='1920' height='1080' fill='url(%23v)'/%3E%3C/svg%3E");
		--home-panel: rgba(255,255,255,.10);
		--home-line: rgba(255,255,255,.18);
		--home-text: #ffffff;
		--home-muted: #d7dce6;
		background: var(--home-bg) !important;
		color: var(--home-text);
	}
	main.o_web_client[data-view="apps"] > .o_navbar,
	body[data-view="apps"] > .o_navbar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 20;
		background: transparent !important;
		border: 0 !important;
		box-shadow: none !important;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar,
	body[data-view="apps"] > .o_navbar > .o_main_navbar,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"] > .o_navbar > .o_main_navbar,
	body[data-theme="enterprise"][data-view="apps"] > .o_navbar > .o_main_navbar {
		background: transparent !important;
		border: 0 !important;
		box-shadow: none !important;
		color: var(--home-text);
	}
	main.o_web_client[data-view="apps"] > .o_action_manager,
	main.o_web_client[data-view="apps"] .o-app-launcher-view,
	main.o_web_client[data-theme="enterprise-like"] .o-app-launcher-view {
		min-height: 100vh;
		background-color: var(--home-bg) !important;
		background-image: var(--home-bg-image) !important;
		background-attachment: fixed;
		background-position: center;
		background-repeat: no-repeat;
		background-size: cover;
		box-shadow: none;
		color: var(--home-text);
	}
	main.o_web_client[data-view="apps"] > .o_action_manager > .o-app-launcher-view {
		padding-top: 70px;
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="root"],
	main.o_web_client[data-view="apps"][data-home-menu-mode="root"] > .o_action_manager,
	main.o_web_client[data-view="apps"][data-home-menu-mode="root"] > .o_action_manager > .o-app-launcher-view {
		height: 100vh;
		max-height: 100vh;
		overflow: hidden;
	}
	.o_home_menu_registration_banner,
	main.o_web_client[data-theme="enterprise-like"] .o_home_menu_registration_banner,
	body[data-theme="enterprise"] .o_home_menu_registration_banner {
		position: relative;
		display: flex !important;
		align-items: center;
		justify-content: center;
		width: min(818px, calc(100vw - 48px));
		min-height: 54px;
		margin: 0 auto 54px;
		padding: 12px 48px 12px 18px;
		border: 1px solid rgba(120,160,205,.34);
		border-left: 3px solid #5aa7ff;
		border-radius: 4px;
		background: rgba(53,75,107,.82);
		box-shadow: 0 14px 34px rgba(0,0,0,.20);
		color: #fff;
		font-size: 14px;
		font-weight: 500;
		line-height: 1.35;
		text-align: center;
	}
	.o_home_menu_registration_banner[hidden] {
		display: none !important;
	}
	.o_home_menu_registration_text {
		display: block;
		max-width: 100%;
	}
	.o_home_menu_registration_close {
		position: absolute;
		top: 50%;
		right: 13px;
		width: 28px;
		height: 28px;
		margin-top: -14px;
		border: 0;
		background: transparent;
		color: #fff;
		font-size: 16px;
		font-weight: 700;
		line-height: 28px;
		text-align: center;
	}
	.o_home_menu_registration_close:hover,
	.o_home_menu_registration_close:focus-visible {
		background: rgba(255,255,255,.12);
		outline: none;
	}
	.o-app-launcher-view .o_apps {
		margin-top: 0;
	}
	main.o_web_client[data-view="apps"] .o-app-launcher-view .o_app:hover,
	main.o_web_client[data-view="apps"] .o-app-launcher-view .o_app:focus-visible {
		background: rgba(255,255,255,.08) !important;
		color: var(--home-text);
	}
	.o_user_avatar {
		display: inline-grid;
		place-items: center;
		width: 24px;
		height: 24px;
		flex: 0 0 24px;
		border-radius: 4px;
		background: #9cc63b;
		color: #fff;
		font-size: 13px;
		font-weight: 700;
		line-height: 1;
	}
	.o_user_menu_name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.o_database_name {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		max-width: 202px;
		height: 14px;
		margin-left: 28px;
		margin-top: -4px;
		padding: 0 4px;
		overflow: hidden;
		background: #f4d7b1;
		color: #5b351c;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
		font-size: 11px;
		font-weight: 500;
		line-height: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.o_database_icon {
		position: relative;
		display: inline-block;
		width: 9px;
		height: 10px;
		flex: 0 0 9px;
		color: #5b351c;
	}
	.o_database_icon::before,
	.o_database_icon::after {
		content: "";
		position: absolute;
		left: 1px;
		width: 7px;
		border: 1px solid currentColor;
	}
	.o_database_icon::before {
		top: 1px;
		height: 4px;
		border-radius: 50%;
		background: rgba(91,53,28,.08);
	}
	.o_database_icon::after {
		top: 3px;
		height: 5px;
		border-top: 0;
		border-radius: 0 0 45% 45%;
		box-shadow: inset 0 -2px 0 rgba(91,53,28,.18);
	}
	.o_database_label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	main.o_web_client[data-view="apps"] .o_user_menu,
	body[data-view="apps"] .o_user_menu {
		display: grid;
		grid-template-columns: 24px minmax(0, auto);
		column-gap: 8px;
		row-gap: 0;
		align-items: center;
		height: 42px;
	}
	main.o_web_client[data-view="apps"] .o_user_menu .o_user_avatar,
	body[data-view="apps"] .o_user_menu .o_user_avatar {
		grid-row: 1 / 3;
	}
	main.o_web_client[data-view="apps"] .o_user_menu .o_database_name,
	body[data-view="apps"] .o_user_menu .o_database_name {
		grid-column: 2;
		margin: -3px 0 0;
	}
	.o_debug_icon,
	.o_debug_tools_icon {
		position: relative;
		display: inline-block;
		width: 16px;
		height: 16px;
	}
	.o_debug_icon::before {
		content: "";
		position: absolute;
		left: 4px;
		top: 5px;
		width: 8px;
		height: 8px;
		border: 2px solid currentColor;
		border-radius: 50% 50% 45% 45%;
		box-shadow: -5px 1px 0 -3px currentColor, 5px 1px 0 -3px currentColor, -5px 6px 0 -3px currentColor, 5px 6px 0 -3px currentColor;
	}
	.o_debug_icon::after {
		content: "";
		position: absolute;
		left: 7px;
		top: 1px;
		width: 2px;
		height: 15px;
		background: currentColor;
		transform: rotate(90deg);
	}
	.o_debug_tools_icon::before,
	.o_debug_tools_icon::after {
		content: "";
		position: absolute;
		left: 7px;
		top: 1px;
		width: 2px;
		height: 15px;
		border-radius: 2px;
		background: currentColor;
	}
	.o_debug_tools_icon::before {
		transform: rotate(45deg);
	}
	.o_debug_tools_icon::after {
		transform: rotate(-45deg);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-body.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-body.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-body.o_form_sheet_bg {
		max-width: none;
		margin: 0;
		padding: 12px 16px 28px;
		background: var(--bg) !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-sheet.o_form_sheet {
		width: auto;
		max-width: none;
		margin: 0;
		padding: 28px 24px 24px;
		border-radius: 3px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="res.groups"] .gorp-form-body.o_form_sheet_bg {
		max-width: none;
		margin: 0;
		padding: 8px 16px 28px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="res.groups"] .gorp-form-sheet.o_form_sheet {
		width: calc(100vw - 32px);
		max-width: none;
		margin: 0;
		border-radius: 3px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet .oe_title h1,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-sheet .oe_title h1,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-sheet .oe_title h1 {
		margin-bottom: 18px;
		font-size: 30px;
		line-height: 1.16;
		font-weight: 500;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-title-input.o_input {
		color: var(--text);
		font-size: 30px;
		line-height: 1.16;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-title-input.o_input::placeholder {
		color: #777b86;
	}
	.gorp-readonly-boolean {
		position: relative;
		display: inline-block;
		width: 14px;
		height: 14px;
		vertical-align: middle;
		border: 1px solid #3f4554;
		border-radius: 1px;
		background: transparent;
	}
	.gorp-readonly-boolean[data-checked="true"] {
		border-color: #017e84;
		background: #017e84;
	}
	.gorp-readonly-boolean[data-checked="true"]::after {
		content: "";
		position: absolute;
		left: 3px;
		top: 1px;
		width: 5px;
		height: 8px;
		border: solid #fff;
		border-width: 0 2px 2px 0;
		transform: rotate(45deg);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-readonly-boolean {
		border-color: #4a5061;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-contextual.o_server_action_contextual {
		margin: 0 0 10px;
		border-color: #875a7b;
		background: #875a7b !important;
		color: #fff;
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-contextual.o_server_action_contextual:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-contextual.o_server_action_contextual:focus-visible,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-smart-button.o_stat_button:hover,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-smart-button.o_stat_button:focus-visible {
		border-color: #9d6b91;
		background: #343743 !important;
		color: #fff;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-smart-button.o_stat_button {
		min-height: 31px;
		border-color: #4a4f60;
		background: #282a35 !important;
		color: #fff;
		box-shadow: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-smart-button.o_stat_button::before {
		content: attr(data-count);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 16px;
		height: 16px;
		margin-right: 6px;
		border-radius: 999px;
		background: #714b67;
		color: #fff;
		font-size: 10px;
		font-weight: 600;
		line-height: 1;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-band.o_server_action_band,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-server-action-band.o_server_action_band,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-server-action-band.o_server_action_band {
		display: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-fields.o_inner_group,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-fields.o_inner_group,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-fields.o_inner_group {
		gap: 13px 34px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field.o_wrap_field,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field.o_wrap_field,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-field.o_wrap_field {
		grid-template-columns: minmax(72px, .25fr) minmax(0, 1fr);
		gap: 8px 14px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="model_id"] {
		grid-column: 1;
		grid-row: 1;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="group_ids"] {
		grid-column: 2;
		grid-row: 1;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="user_id"] {
		grid-column: 1;
		grid-row: 2;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="interval_number"] {
		grid-column: 1;
		grid-row: 3;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="active"] {
		grid-column: 1;
		grid-row: 4;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="nextcall"] {
		grid-column: 1;
		grid-row: 5;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-field[data-field="priority"] {
		grid-column: 1;
		grid-row: 6;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-execute-every {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-execute-every .o_input {
		width: auto;
		min-width: 64px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook.o_notebook,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook.o_notebook {
		margin-top: 18px;
		border-top: 0;
		background: transparent !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-notebook-tabs.nav-tabs,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-notebook-tabs.nav-tabs {
		margin: 0;
		border-bottom: 1px solid var(--line);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-notebook-tab.nav-link,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-notebook-tab.nav-link {
		min-height: 38px;
		padding: 9px 16px;
		border-radius: 3px 3px 0 0;
		border-color: transparent;
		background: var(--panel-soft);
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-notebook-tab.nav-link.active,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-notebook-tab.nav-link.active {
		border-color: var(--line);
		border-top: 3px solid var(--accent);
		border-bottom-color: var(--panel);
		background: var(--panel) !important;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-notebook-content.tab-content,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-notebook-content.tab-content {
		border: 1px solid var(--line);
		border-top: 0;
		padding: 14px 24px 28px;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-field[data-field="code"],
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-field[data-field="code"] {
		display: block;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-form-field[data-field="code"] > .o_form_label,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-form-field[data-field="code"] > .o_form_label {
		display: none;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-code-viewer,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-code-viewer {
		position: relative;
		min-height: 42px;
		padding: 9px 12px 9px 44px;
		border: 0;
		border-radius: 0;
		background: #20251d !important;
		color: #f4f7f2;
		box-shadow: inset 32px 0 0 rgba(255,255,255,.035);
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-code-viewer::before,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-code-viewer::before {
		content: "1";
		position: absolute;
		left: 18px;
		top: 9px;
		color: #a9b1b8;
		font-size: 12px;
		line-height: 1.5;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-code-editor.o_input,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-code-editor.o_input {
		min-height: 168px;
		padding: 10px 12px;
		border-radius: 2px;
		background: #20251d !important;
		color: #f4f7f2 !important;
		box-shadow: inset 32px 0 0 rgba(255,255,255,.035);
	}
	[hidden] { display: none !important; }
	@media (max-width: 900px) {
		.layout { grid-template-columns: minmax(0, 1fr); }
		aside { border-right: 0; border-bottom: 1px solid var(--line); }
		.grid { grid-template-columns: 1fr 1fr; }
		.o-app-launcher-view .o_draggable { width: 33.333333%; }
		.module-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
		header { padding: 0 8px; }
		.o-mobile-menu-toggle { display: inline-grid; }
		.o-nav { display: none; }
		body.o-mobile-menu-open .o-nav {
			display: flex;
			position: fixed;
			top: 46px;
			left: 0;
			right: 0;
			height: auto;
			max-height: calc(100dvh - 46px);
			overflow: auto;
			flex-direction: column;
			background: var(--topbar);
			border-top: 1px solid rgba(255,255,255,.16);
			box-shadow: 0 12px 24px rgba(16, 24, 40, .18);
		}
		body.o-mobile-menu-open .o-nav button {
			justify-content: flex-start;
			width: 100%;
			min-height: 42px;
		}
		.o-search,
		.o-company-switcher {
			display: none;
		}
	}
	@media (max-width: 620px) {
		header { display: flex; min-width: 0; }
		.o-brand { min-width: 0; flex: 1; }
		.o-brand h1 { font-size: 16px; }
		.o-menu-systray { flex: 0 1 auto; }
		.o-systray-item { padding: 0 7px; min-width: 32px; }
		.o-systray-counter,
		.o-user-menu-button span { display: none; }
		.grid { grid-template-columns: 1fr; }
		main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray {
			height: 46px;
			padding-right: 8px;
		}
		main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item {
			position: relative;
			justify-content: center;
			min-width: 34px;
			padding: 0 7px;
		}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-counter {
		position: absolute;
		top: 7px;
		right: 2px;
		display: inline-grid;
			place-items: center;
			min-width: 16px;
			height: 16px;
			padding: 0 4px;
			background: #ba4a55;
			color: #fff;
		font-size: 10px;
		line-height: 16px;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-user-menu-button {
		display: inline-grid;
		place-items: center;
		width: 36px;
			min-width: 36px;
			padding: 0 4px;
		}
		main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_user_avatar {
			display: inline-grid;
			width: 28px;
			height: 28px;
		}
		.o-app-launcher-view { padding: 68px 14px 32px; min-width: 0; }
		.o-app-launcher-view .o-app-shell { max-width: 370px; }
		.o-app-launcher-view .o_apps {
			display: grid;
			grid-template-columns: repeat(4, minmax(0, 1fr));
			justify-content: center;
			gap: 18px 10px;
			max-width: 360px;
			margin-bottom: 18px;
		}
		.o-app-launcher-view .o_draggable {
			width: auto;
			min-width: 0;
			margin-bottom: 0;
		}
		.o-app-launcher-view .o_app {
			width: 76px;
			min-width: 0;
			min-height: 104px;
			justify-self: center;
			padding: 0 1px;
			gap: 8px;
		}
		.o-app-launcher-view .o_app strong {
			white-space: normal;
			line-height: 1.15;
			font-size: 13px;
			font-weight: 600;
		}
		.o-app-launcher-view .o_app .o_app_icon {
			width: 68px;
			height: 68px;
			border-radius: 5px;
		}
		.module-grid, .record-grid { grid-template-columns: 1fr; }
		.gorp-apps-catalog-content {
			grid-template-columns: minmax(0, 1fr);
		}
		.gorp-apps-catalog-sidebar {
			display: flex;
			overflow-x: auto;
		}
		.gorp-apps-filterbar {
			margin-left: 0;
		}
		.o_settings_container { grid-template-columns: minmax(0, 1fr); }
		.o_settings_search_panel { justify-content: stretch; }
		.o_settings_search_panel .o_settings_search { max-width: none; }
		.o_setting_grid { grid-template-columns: minmax(0, 1fr); }
		.o_setting_box { border-right: 0; padding: 16px 18px; }
		.o_settings_sidebar {
			display: flex;
			overflow-x: auto;
			border-right: 0;
			border-bottom: 1px solid var(--line);
			padding: 0 0 8px;
		}
		.gorp-form-fields.o_inner_group,
		.gorp-form-field.o_wrap_field {
			grid-template-columns: minmax(0, 1fr);
		}
		.gorp-form-sheet.o_form_sheet {
			padding: 18px 16px;
		}
		.gorp-form-sheet .oe_title h1 {
			font-size: 28px;
			line-height: 1.2;
		}
		.gorp-res-user-access-row,
		.gorp-res-user-access-extra-rights,
		.gorp-res-user-access-extra-rights .gorp-res-user-access-row {
			grid-template-columns: minmax(0, 1fr);
		}
		.gorp-res-user-access-extra-rights {
			gap: 8px;
		}
		.gorp-res-user-access-select.o_input {
			width: 100%;
		}
		.gorp-server-action-band.o_server_action_band {
			grid-template-columns: minmax(0, 1fr);
			align-items: stretch;
		}
		.gorp-server-action-identity,
		.gorp-server-action-meta {
			justify-content: flex-start;
		}
		.gorp-server-action-meta {
			display: grid;
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 8px 10px;
		}
		.gorp-server-action-meta-item {
			display: grid;
			gap: 2px;
			padding-left: 0;
			border-left: 0;
		}
		.gorp-server-action-badge,
		.gorp-server-action-state,
		.gorp-selection-pill,
		.gorp-selection-radio-pill {
			min-height: 28px;
			padding: 4px 9px;
			font-size: 12px;
		}
		.gorp-code-viewer,
		.gorp-code-editor {
			min-height: 180px;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-body.o_form_sheet_bg {
			max-width: none;
			margin: 0;
			padding: 0 0 24px;
			background: var(--bg) !important;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet.o_form_sheet {
			width: 100vw;
			max-width: none;
			margin: 0;
			padding: 16px 16px 0;
			border: 0 !important;
			border-radius: 0;
			background: var(--bg) !important;
			box-shadow: none;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet .oe_title {
			margin: 0 0 14px;
			border-bottom: 1px solid #c7cbd6;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet .oe_title h1 {
			max-width: 100%;
			margin: 0;
			overflow: hidden;
			color: var(--text);
			font-size: 24px;
			font-weight: 500;
			line-height: 1.35;
			text-overflow: clip;
			white-space: nowrap;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-fields.o_inner_group {
			display: block;
			gap: 0;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field.o_wrap_field {
			display: block;
			margin: 0 0 16px;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="name"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="active"] {
			display: none;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field > .o_form_label {
			display: block;
			margin: 0 0 6px;
			color: #aeb6c6;
			font-size: 14px;
			font-weight: 500;
			line-height: 1.2;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-contextual.o_server_action_contextual {
			margin: 12px 16px 8px;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="model_id"] > .o_form_label::after,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="group_ids"] > .o_form_label::after,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="state"] > .o_form_label::after {
			content: " ?";
			color: #6bb7ff;
			font-size: 12px;
			font-weight: 600;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="model_id"] output,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="model_id"] .gorp-many2one-link,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="group_ids"] .gorp-x2many-tags {
			position: relative;
			display: flex;
			align-items: center;
			width: 100%;
			min-height: 29px;
			padding: 0 20px 5px 0;
			border-bottom: 1px solid #c7cbd6;
			color: var(--text);
			font-size: 15px;
			line-height: 1.35;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="model_id"] output::after,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="model_id"] .gorp-many2one-link::after,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field[data-field="group_ids"] .gorp-x2many-tags::after {
			content: "";
			position: absolute;
			right: 1px;
			top: 11px;
			width: 0;
			height: 0;
			border-left: 4px solid transparent;
			border-right: 4px solid transparent;
			border-top: 5px solid #d8dbe3;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] {
			display: flex;
			flex-wrap: wrap;
			gap: 5px;
			align-items: flex-start;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill {
			min-height: 28px;
			padding: 5px 9px;
			border-radius: 2px;
			font-size: 13px;
			font-weight: 600;
			line-height: 1.25;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="object_write"] { order: 1; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="object_create"] { order: 2; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="object_copy"] { order: 3; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="code"] { order: 4; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="webhook"] { order: 5; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="multi"] { order: 6; }
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="mail_post"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="followers"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="remove_followers"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="next_activity"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="sms"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="whatsapp"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="ai"],
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-selection-pills[data-field="state"] .gorp-selection-pill[data-value="documents_account_record_create"] {
			display: none;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook.o_notebook {
			margin: 34px -16px 0;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-form-notebook-tabs.nav-tabs {
			padding-left: 16px;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-form-notebook-tab.nav-link {
			min-height: 38px;
			padding: 9px 16px;
			border-radius: 2px 2px 0 0;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-form-notebook-content.tab-content {
			border: 0;
			border-bottom: 1px solid var(--line);
			padding: 16px 16px 72px;
			background: transparent !important;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-form-field[data-field="code"] > .o_form_label {
			display: none;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-code-viewer {
			min-height: 18px;
			max-height: 22px;
			padding: 0 0 0 44px;
			overflow: hidden;
			font-size: 13px;
			line-height: 18px;
			white-space: pre;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-code-viewer::before {
			top: 0;
			line-height: 18px;
		}
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-server-action-notebook .gorp-code-viewer code {
			white-space: pre;
		}
		.gorp-form-notebook-tab.nav-link {
			padding: 9px 12px;
		}
		.toolbar { display: grid; }
		.field.small { max-width: none; }
		.o-control-panel,
		.o_control_panel {
			display: grid;
			grid-template-columns: minmax(0, 1fr);
			align-items: stretch;
			gap: 10px;
			padding: 10px 12px;
		}
		.record-panel .o_control_panel_main {
			grid-template-columns: minmax(0, 1fr);
			align-items: stretch;
			gap: 8px;
		}
		.record-panel .o_control_panel_main_buttons,
		.record-panel .o_control_panel_breadcrumbs,
		.record-panel .o_control_panel_navigation {
			grid-column: 1;
			grid-row: auto;
			justify-content: flex-start;
		}
		.record-panel .o_control_panel_main_buttons {
			order: 1;
		}
		.record-panel .o_control_panel_breadcrumbs {
			order: 2;
		}
		.record-panel .o_control_panel_navigation {
			order: 3;
		}
		.gorp-window-action[data-view="form"] .o_cp_switch_buttons {
			display: none;
		}
			.gorp-window-action[data-view="form"] .o_control_panel_main_buttons .gorp-form-action-menu,
			.gorp-window-action[data-view="form"] .o_control_panel_actions .gorp-form-action-menu {
				display: inline-flex;
			}
			.gorp-window-action[data-view="form"] .o_control_panel_main_buttons .gorp-form-action-menu .gorp-action-menu-toggle,
			.gorp-window-action[data-view="form"] .o_control_panel_actions .gorp-form-action-menu .gorp-action-menu-toggle {
				justify-content: center;
				width: 36px;
				min-width: 36px;
			height: 34px;
			min-height: 34px;
			padding: 0;
			border-color: var(--line);
			background: var(--btn-secondary-bg);
				color: var(--text);
				font-size: 0;
			}
			.gorp-window-action[data-view="form"] .o_control_panel_main_buttons .gorp-form-action-menu .gorp-action-menu-toggle i,
			.gorp-window-action[data-view="form"] .o_control_panel_actions .gorp-form-action-menu .gorp-action-menu-toggle i {
				display: none;
			}
			.gorp-window-action[data-view="form"] .o_control_panel_main_buttons .gorp-form-action-menu .gorp-action-menu-toggle::before,
			.gorp-window-action[data-view="form"] .o_control_panel_actions .gorp-form-action-menu .gorp-action-menu-toggle::before {
				content: "";
				width: 16px;
				height: 16px;
			border: 2px solid currentColor;
			border-radius: 50%;
			box-shadow: inset 0 0 0 3px var(--btn-secondary-bg);
		}
		.o-form-control .o-breadcrumbs button {
			max-width: 42vw;
		}
		.o-list-content { padding: 12px; overflow-x: hidden; }
		.o-list-view table { display: none; }
		.o_mobile_list_cards { display: grid; gap: 8px; }
		.gorp-action-dialog {
			align-items: stretch;
			padding: 46px 0 0;
		}
		.gorp-action-dialog .o_dialog_container,
		.gorp-action-dialog .modal-dialog {
			min-height: calc(100dvh - 46px);
		}
		.gorp-action-dialog .modal-content {
			height: calc(100dvh - 46px);
			min-height: calc(100dvh - 46px);
			max-height: calc(100dvh - 46px);
			border-right: 0;
			border-bottom: 0;
			border-left: 0;
			border-radius: 0;
		}
		.gorp-action-dialog .gorp-action-dialog-footer {
			flex: 0 0 auto;
			justify-content: flex-start;
			max-height: 35dvh;
			overflow: auto;
		}
		#recordForm { padding: 14px; }
	}
	/* Clean-room Odoo light chrome parity overrides. Keep this block late in the cascade. */
	:root,
	body[data-theme="enterprise"],
	main.o_web_client[data-theme="enterprise-like"] {
		--bg: #f5f5f5;
		--panel: #ffffff;
		--panel-soft: #f8f9fa;
		--text: #1f2933;
		--muted: #6b7280;
		--line: #d8dadd;
		--line-soft: #e7e9ed;
		--hover-bg: #f1f3f5;
		--control-bg: #262a36;
		--control-shadow: none;
		--dropdown-shadow: 0 6px 18px rgba(0,0,0,.12);
		--list-head: #f7f7f7;
		--btn-secondary-bg: #ffffff;
		--btn-secondary-hover: #f5f5f5;
		--accent: #875a7b;
		--accent-2: #017e84;
		--accent-text: #ffffff;
		--topbar: #262a36;
		--topbar-hover: #303442;
		--sidebar: #f8f9fa;
		--home-bg: #000511;
		--home-bg-image:
			radial-gradient(ellipse at 50% 120%, rgba(0,5,17,.88) 0%, rgba(0,5,17,.60) 36%, rgba(0,5,17,.14) 66%, rgba(0,5,17,0) 100%),
			radial-gradient(ellipse at 18% 20%, rgba(10,38,52,.98) 0%, rgba(8,34,48,.86) 32%, rgba(4,12,25,.68) 68%, rgba(4,12,25,0) 100%),
			radial-gradient(circle at 31% 18%, rgba(46,78,96,.72) 0%, rgba(23,47,66,.36) 30%, rgba(0,5,17,0) 58%),
			radial-gradient(circle at 83% 52%, rgba(40,32,75,.58) 0%, rgba(14,10,35,.32) 34%, rgba(0,5,17,0) 62%),
			url("/web_enterprise/static/img/background-dark.jpg?v=cleanroom-20260628b");
		--home-panel: rgba(255,255,255,.92);
		--home-line: rgba(31,41,51,.12);
		--home-text: #ffffff;
		--home-muted: rgba(255,255,255,.72);
	}
	body,
	main.o_web_client,
	main.o_web_client > .o_action_manager {
		background: var(--bg) !important;
		color: var(--text);
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) {
		background: #1b1d26 !important;
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_action_manager {
		background: transparent !important;
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) .o_control_panel,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) .o-control-panel {
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) .gorp-window-action[data-model="res.config.settings"] > .o_control_panel {
		padding: 8px 16px 16px !important;
	}
	body.o_home_menu_background,
	body[data-view="apps"],
	main.o_web_client[data-view="apps"],
	main.o_web_client[data-view="apps"] > .o_action_manager,
	main.o_web_client[data-view="apps"] > .o_action_manager > .o-app-launcher-view,
	.o-app-launcher-view {
		background-color: var(--home-bg) !important;
		background-image: var(--home-bg-image) !important;
		color: var(--home-text) !important;
	}
	header,
	header.o_navbar,
	.o_navbar > .o_main_navbar,
	main.o_web_client:not([data-view="apps"]) > .o_navbar > .o_main_navbar {
		background: var(--topbar) !important;
		color: #e4e4e4 !important;
		border-bottom: 1px solid rgba(255,255,255,.08) !important;
		box-shadow: none !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar {
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar {
		background: transparent !important;
		color: #ffffff !important;
		border-bottom: 0 !important;
		box-shadow: none !important;
	}
	main.o_web_client[data-view="apps"] > .o_navbar {
		position: relative;
	}
	@media (min-width: 621px) {
		main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_action_manager {
			margin-top: -46px !important;
		}
		main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_action_manager > .o-app-launcher-view {
			min-height: 100vh !important;
			height: 100vh !important;
			padding-top: 70px !important;
		}
		main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] .o_home_menu_registration_banner {
			margin-bottom: 47px !important;
		}
		main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] .o-app-launcher-view .o_app {
			min-height: 115px !important;
			height: 115px !important;
			border-radius: 6px !important;
		}
	}
	main.o_web_client[data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_navbar_apps_menu,
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_navbar_sections,
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-mobile-menu-toggle {
		display: none !important;
	}
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o_menu_systray,
	main.o_web_client[data-view="apps"] > .o_navbar > .o_main_navbar .o-systray-item {
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar {
		background: transparent !important;
		border: 0 !important;
		box-shadow: none !important;
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_menu_systray,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-systray-item,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-user-menu-button,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-company-switcher {
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-systray-item:hover,
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-systray-item:focus-visible {
		background: rgba(255,255,255,.10) !important;
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o-systray-counter {
		background: rgba(255,255,255,.16) !important;
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"][data-view="apps"][data-home-menu-mode="root"] > .o_navbar > .o_main_navbar .o_database_name {
		display: none !important;
	}
	.o-control-panel,
	.o_control_panel {
		min-height: 58px;
		padding: 8px 16px !important;
		background: var(--control-bg) !important;
		border-bottom: 1px solid rgba(255,255,255,.08) !important;
		box-shadow: var(--control-shadow) !important;
		color: #ffffff !important;
	}
	.o_control_panel_main {
		gap: 6px 12px;
	}
	.o_control_panel_breadcrumbs .breadcrumb-item,
	.o_control_panel_breadcrumbs .breadcrumb-item.active,
	.o-control-panel h2,
	.o_control_panel h2 {
		color: #ffffff !important;
		font-size: 18px;
		font-weight: 500;
	}
	button,
	.btn,
	.btn-secondary,
	.btn-outline-secondary,
	.gorp-action-menu-toggle,
	.gorp-form-action-menu .gorp-action-menu-toggle,
	.o_cp_switch_buttons .o_switch_view,
	.o_pager .btn {
		border-color: var(--line) !important;
		box-shadow: none !important;
	}
	button.secondary,
	.btn-secondary,
	.btn-outline-secondary,
	.gorp-action-menu-toggle,
	.gorp-form-action-menu .gorp-action-menu-toggle,
	.o_cp_switch_buttons .o_switch_view,
	.o_pager .btn {
		background: #ffffff !important;
		color: var(--text) !important;
	}
	button.secondary:hover,
	.btn-secondary:hover,
	.btn-outline-secondary:hover,
	.gorp-action-menu-toggle:hover,
	.o_cp_switch_buttons .o_switch_view:hover,
	.o_pager .btn:hover {
		background: var(--hover-bg) !important;
		color: var(--accent) !important;
	}
	.o_searchview,
	.o_searchview_dropdown_toggler,
	input,
	select,
	textarea {
		background: #ffffff !important;
		color: var(--text) !important;
		border-color: var(--line) !important;
	}
	.o_control_panel .o_searchview,
	.o_control_panel .o_searchview_dropdown_toggler,
	.o_control_panel input,
	.o_control_panel select,
	.o_control_panel textarea {
		background: rgba(255,255,255,.10) !important;
		color: #ffffff !important;
		border-color: rgba(255,255,255,.18) !important;
	}
	.dropdown-menu,
	.o-dropdown-menu,
	.o_navbar_dropdown_menu,
	.o_navbar_submenu_menu,
	.gorp-action-menu-items,
	.gorp-many2one-dropdown,
	.gorp-x2many-dropdown,
	.o_search_options {
		background: #ffffff !important;
		color: var(--text) !important;
		border: 1px solid var(--line) !important;
		box-shadow: var(--dropdown-shadow) !important;
	}
	.o_navbar_dropdown_header {
		color: #6b7280 !important;
		background: #ffffff !important;
	}
	.o_navbar_dropdown_item,
	.dropdown-item,
	.gorp-action-menu-item,
	.gorp-many2one-option,
	.gorp-x2many-option,
	.gorp-many2one-create,
	.gorp-many2one-create-edit,
	.gorp-many2one-search-more,
	.gorp-x2many-create,
	.gorp-x2many-create-edit,
	.gorp-x2many-search-more {
		background: transparent !important;
		color: var(--text) !important;
	}
	.o_navbar_dropdown_item:hover,
	.o_navbar_dropdown_item:focus,
	.o_navbar_dropdown_item.active,
	.dropdown-item:hover,
	.dropdown-item.active,
	.gorp-action-menu-item:hover:not(:disabled),
	.gorp-many2one-option:hover,
	.gorp-many2one-option.active,
	.gorp-x2many-option:hover,
	.gorp-x2many-option.active {
		background: var(--hover-bg) !important;
		color: var(--accent) !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_entry.active,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_dropdown_toggle.active,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_nav_dropdown_toggle[aria-expanded="true"] {
		background: #17373b !important;
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_menu,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_submenu_menu {
		background: #262a36 !important;
		border-color: rgba(255,255,255,.12) !important;
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_header {
		background: #262a36 !important;
		color: rgba(228,228,228,.72) !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_item {
		background: transparent !important;
		color: #e4e4e4 !important;
	}
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_item:hover,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_item:focus,
	main.o_web_client[data-theme="enterprise-like"]:not([data-view="apps"]) > .o_navbar > .o_main_navbar .o_navbar_dropdown_item.active {
		background: #17373b !important;
		color: #ffffff !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-user-identity .gorp-user-identity-input.o_input {
		background: transparent !important;
		border-color: transparent !important;
		color: #1f2933 !important;
		box-shadow: none !important;
	}
	main.o_web_client[data-theme="enterprise-like"] .gorp-user-identity .gorp-user-identity-input.o_input::placeholder {
		color: #6b7280 !important;
		opacity: 1;
	}
	.o-list-content {
		padding: 12px 16px 24px !important;
		background: var(--bg) !important;
	}
	.o-list-view table,
	.gorp-list-view {
		border: 1px solid var(--line) !important;
		background: #ffffff !important;
		box-shadow: none !important;
	}
	.o-list-view th,
	.o-list-view td,
	.gorp-list-view th,
	.gorp-list-view td {
		background: #ffffff !important;
		color: var(--text) !important;
		border-color: var(--line-soft) !important;
	}
	.o-list-view th,
	.gorp-list-view th {
		background: #f7f7f7 !important;
		color: #4b5563 !important;
		font-weight: 600;
	}
	.o-list-view tr:hover td,
	.gorp-list-view tr:hover td {
		background: #f5f5f5 !important;
	}
	.o_mobile_record_card,
	.o_kanban_record,
	.o_kanban_group {
		background: #ffffff !important;
		color: var(--text) !important;
		border-color: var(--line) !important;
		box-shadow: none !important;
	}
	.gorp-form-body.o_form_sheet_bg,
	.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-body.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-body.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-body.o_form_sheet_bg,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="res.groups"] .gorp-form-body.o_form_sheet_bg {
		background: var(--bg) !important;
	}
	.gorp-form-sheet.o_form_sheet,
	.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.cron"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="base.automation"] .gorp-form-sheet.o_form_sheet,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="res.groups"] .gorp-form-sheet.o_form_sheet {
		background: #ffffff !important;
		color: var(--text) !important;
		border: 1px solid var(--line) !important;
		box-shadow: none !important;
	}
	.gorp-form-field > .o_form_label,
	main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-field > .o_form_label {
		color: #4b5563 !important;
	}
	.gorp-form-notebook-tabs.nav-tabs,
	.gorp-server-action-notebook .gorp-form-notebook-tabs.nav-tabs,
	.gorp-scheduled-action-notebook .gorp-form-notebook-tabs.nav-tabs {
		border-bottom: 1px solid var(--line) !important;
	}
	.gorp-form-notebook-tab.nav-link,
	.gorp-server-action-notebook .gorp-form-notebook-tab.nav-link,
	.gorp-scheduled-action-notebook .gorp-form-notebook-tab.nav-link {
		background: #f8f9fa !important;
		color: var(--text) !important;
		border-color: transparent !important;
	}
	.gorp-form-notebook-tab.nav-link.active,
	.gorp-server-action-notebook .gorp-form-notebook-tab.nav-link.active,
	.gorp-scheduled-action-notebook .gorp-form-notebook-tab.nav-link.active {
		background: #ffffff !important;
		border-color: var(--line) !important;
		border-bottom-color: #ffffff !important;
		color: var(--text) !important;
	}
	.gorp-form-notebook-content.tab-content,
	.gorp-server-action-notebook .gorp-form-notebook-content.tab-content,
	.gorp-scheduled-action-notebook .gorp-form-notebook-content.tab-content {
		background: #ffffff !important;
		border-color: var(--line) !important;
	}
	.gorp-server-action-contextual.o_server_action_contextual {
		background: var(--accent) !important;
		color: #ffffff !important;
		border-color: var(--accent) !important;
	}
	.gorp-server-action-smart-button.o_stat_button,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-smart-button.o_stat_button {
		background: #ffffff !important;
		color: var(--text) !important;
		border-color: var(--line) !important;
	}
	.gorp-code-viewer,
	.gorp-code-editor,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-code-viewer,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-code-viewer,
	main.o_web_client[data-theme="enterprise-like"] .gorp-server-action-notebook .gorp-code-editor.o_input,
	main.o_web_client[data-theme="enterprise-like"] .gorp-scheduled-action-notebook .gorp-code-editor.o_input {
		background: #f8f9fa !important;
		color: #1f2933 !important;
		border: 1px solid var(--line) !important;
		box-shadow: inset 32px 0 0 #f1f3f5 !important;
	}
	.o-app-launcher-view .o_app,
	.o-app-launcher-view .o_app:hover,
	.o-app-launcher-view .o_app .o_caption,
	.o-app-launcher-view .o_app .o_app_name,
	.o-app-launcher-view .o_app strong {
		color: var(--home-text) !important;
		text-shadow: none !important;
	}
	.o-app-launcher-view .o_app:hover,
	.o-app-launcher-view .o_app:focus-visible,
	main.o_web_client[data-view="apps"] .o-app-launcher-view .o_app:hover,
	main.o_web_client[data-view="apps"] .o-app-launcher-view .o_app:focus-visible {
		background: rgba(255,255,255,.08) !important;
	}
	.o-app-launcher-view .o_app_icon {
		background: transparent !important;
		border-color: rgba(255,255,255,.18) !important;
		box-shadow: 0 8px 18px rgba(0,0,0,.18) !important;
	}
	.o-app-launcher-view .o_app_icon_fallback::before {
		background: rgba(1,126,132,.92) !important;
		border-color: transparent !important;
	}
	.o-app-launcher-view .o_app_icon_fallback::after {
		background: rgba(38,52,69,.55) !important;
		border-color: transparent !important;
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::before {
		background: conic-gradient(#875a7b 0 25%, #4fc3c8 0 50%, #ef6c56 0 75%, #6ca6d9 0) !important;
	}
	.o-app-launcher-view .o_app[data-app-key="apps"] .o_app_icon_fallback::after,
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::after {
		background: #263445 !important;
	}
	.o-app-launcher-view .o_app[data-app-key="settings"] .o_app_icon_fallback::before {
		background: conic-gradient(from 315deg, #f7c462 0 50%, #ee8543 50% 100%) !important;
	}
	.o-app-launcher-view .o_app[data-app-key="approvals"] .o_app_icon_fallback::before {
		background: transparent !important;
		border-color: #ffffff !important;
		box-shadow: 0 0 0 12px #6e7da4 !important;
	}
	.o-app-launcher-view .o_app[data-app-key="approvals"] .o_app_icon_fallback::after {
		background: transparent !important;
		border-left-color: #ffffff !important;
		border-bottom-color: #ffffff !important;
	}
	.o-app-launcher-view .o_app[data-app-key="delegation"] .o_app_icon_fallback::before {
		background: #dfeff0 !important;
		box-shadow: 12px 0 0 #8fc9c2 !important;
	}
	.o-app-launcher-view .o_app[data-app-key="delegation"] .o_app_icon_fallback::after {
		background: rgba(25,160,160,.58) !important;
	}
	#appGrid .o_app .o_app_icon::before,
	#appGrid .o_app .o_app_icon::after {
		content: "";
		position: absolute;
		pointer-events: none;
	}
	#appGrid .o_app[data-app-key="apps"] .o_app_icon::before {
		left: 13px;
		top: 13px;
		width: 44px;
		height: 44px;
		border-radius: 50%;
		background: conic-gradient(#875a7b 0 25%, #4fc3c8 0 50%, #ef6c56 0 75%, #6ca6d9 0) !important;
	}
	#appGrid .o_app[data-app-key="apps"] .o_app_icon::after {
		left: 29px;
		top: 29px;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: #263445 !important;
		box-shadow:
			-15px 0 0 -12px #263445,
			15px 0 0 -12px #263445,
			0 -15px 0 -12px #263445,
			0 15px 0 -12px #263445;
	}
	#appGrid .o_app[data-app-key="settings"] .o_app_icon::before {
		left: 16px;
		top: 12px;
		width: 38px;
		height: 46px;
		clip-path: polygon(50% 0, 88% 21%, 88% 72%, 50% 100%, 12% 72%, 12% 21%);
		background: conic-gradient(from 315deg, #f7c462 0 50%, #ee8543 50% 100%) !important;
	}
	#appGrid .o_app[data-app-key="settings"] .o_app_icon::after {
		left: 28px;
		top: 26px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: #263445 !important;
		box-shadow: 0 0 0 7px rgba(135,90,123,.86);
	}
	#appGrid .o_app[data-app-key="approvals"] .o_app_icon::before {
		inset: 17px;
		border-radius: 50%;
		border: 7px solid #ffffff;
		background: #6e7da4 !important;
		box-shadow: 0 0 0 12px #6e7da4;
	}
	#appGrid .o_app[data-app-key="approvals"] .o_app_icon::after {
		left: 36px;
		top: 31px;
		width: 18px;
		height: 9px;
		border-left: 4px solid #ffffff;
		border-bottom: 4px solid #ffffff;
		border-radius: 1px;
		background: transparent !important;
		transform: rotate(-45deg);
	}
	#appGrid .o_app[data-app-key="delegation"] .o_app_icon::before {
		inset: 16px 22px 14px 13px;
		border-radius: 10px;
		background: #dfeff0 !important;
		box-shadow: 12px 0 0 #8fc9c2;
	}
	#appGrid .o_app[data-app-key="delegation"] .o_app_icon::after {
		right: 12px;
		bottom: 15px;
		width: 28px;
		height: 14px;
		border-radius: 7px;
		background: rgba(25,160,160,.58) !important;
	}
	@media (max-width: 900px) {
		body.o-mobile-menu-open .o-nav {
			background: var(--topbar) !important;
			border-top: 1px solid rgba(255,255,255,.18) !important;
		}
		.o-control-panel,
		.o_control_panel {
			padding: 8px 10px !important;
		}
	}
	@media (max-width: 620px) {
		.o-app-launcher-view {
			padding-top: 56px !important;
		}
		.gorp-form-sheet.o_form_sheet,
		main.o_web_client[data-theme="enterprise-like"] .gorp-form-view[data-model="ir.actions.server"] .gorp-form-sheet.o_form_sheet {
			width: 100% !important;
			border-left: 0 !important;
			border-right: 0 !important;
			border-radius: 0 !important;
		}
	}
	</style>
</head>
<body class="o_home_menu_background" data-theme="enterprise" data-view="apps" data-mobile-safe="true">
  <header class="o_navbar">
  <nav class="o_main_navbar d-print-none">
    <div class="o_navbar_apps_menu o-brand">
      <button type="button" id="navApps" class="o_menu_toggle o-launcher-button border-0" data-view="apps" aria-label="Apps" accesskey="h"><span class="o_menu_toggle_icon o-launcher" aria-hidden="true"><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span></span></button>
      <h1 class="o_menu_brand">Odoo</h1>
    </div>
    <button type="button" id="mobileMenu" class="o-mobile-menu-toggle o_mobile_menu_toggle" aria-label="Menu" aria-expanded="false"><span aria-hidden="true"></span></button>
    <nav class="o-nav o_navbar_sections o_menu_sections" id="topMenu" aria-label="Application"></nav>
    <label class="o-search">
      <span class="sr-only">Search</span>
      <input id="globalSearch" placeholder="Search records">
    </label>
    <label class="theme-field" hidden>
      <span class="sr-only">Theme</span>
      <select id="theme">
        <option value="enterprise">Enterprise</option>
        <option value="standard">Standard</option>
      </select>
    </label>
    <div class="o-menu-systray o_menu_systray d-flex flex-shrink-0 ms-auto bg-inherit" role="menu" aria-label="Systray">
      <button type="button" class="o-systray-item o_mail_systray_item dropdown-toggle" id="messageSystray" aria-label="Messages" role="menuitem">
        <i class="o-systray-icon oi oi-discuss" aria-label="Messages" title="Messages"></i>
        <span class="o-systray-counter" id="messageCounter">0</span>
      </button>
      <button type="button" class="o-systray-item o_activity_menu dropdown-toggle" id="activitySystray" aria-label="Activities" role="menuitem">
        <i class="o-systray-icon oi oi-clock" aria-label="Activities" title="Activities"></i>
        <span class="o-systray-counter" id="activityCounter">0</span>
      </button>
      <button type="button" class="o-systray-item o_switch_company_menu o-company-switcher dropdown-toggle" id="companySwitcher" aria-label="Company" role="menuitem">
        <span id="topCompany" class="oe_topbar_name">My Company</span>
      </button>
      <button type="button" class="o-systray-item o_debug_manager dropdown-toggle" id="debugIndicator" role="menuitem" hidden>Debug</button>
      <button type="button" class="o-systray-item o_user_menu o-user-menu-button dropdown-toggle" id="topUser" aria-label="User menu" role="menuitem"><span class="o_user_menu_name">Administrator</span></button>
    </div>
  </nav>
  </header>
  <div class="layout o_web_client_content">
    <aside>
      <div class="o-sidebar-title">
        <h2 id="sidebarTitle">Menu</h2>
        <span id="runtimeStatus" class="muted"></span>
      </div>
      <div id="loginPanel" class="login">
        <h2>Login</h2>
        <label>
          Login
          <input id="login" value="admin" autocomplete="username">
        </label>
        <label>
          Password
          <input id="password" type="password" value="admin" autocomplete="current-password">
        </label>
        <button id="loginButton">Login</button>
      </div>
      <ul id="modules"></ul>
    </aside>
    <main class="o_action_manager">
      <section class="grid o-statusbar">
        <div class="card"><span class="muted">Health</span><strong id="health">...</strong></div>
        <div class="card"><span class="muted">User</span><strong id="user">...</strong></div>
        <div class="card"><span class="muted">Modules</span><strong id="moduleCount">...</strong></div>
        <div class="card"><span class="muted">Menus</span><strong id="menuCount">...</strong></div>
      </section>

      <section id="recordsView" class="panel view-panel o-list-view o_list_view" data-view="records">
        <div class="o-control-panel o_control_panel">
          <div class="o_control_panel_main">
            <div class="o_control_panel_breadcrumbs">
              <div class="o_control_panel_main_buttons d-print-none">
                <button id="createPartner" class="btn btn-primary o_list_button_add">New</button>
              </div>
              <h2 class="o_breadcrumb active">Records</h2>
            </div>
            <div class="o_control_panel_actions">
              <div class="toolbar">
                <label class="field technical-field" hidden>
                  Model
                  <select id="model"></select>
                </label>
                <label class="field technical-field" hidden>
                  Fields
                  <input id="fields" value="id,display_name,name,email">
                </label>
                <label class="field small technical-field" hidden>
                  Limit
                  <input id="limit" type="number" min="1" max="200" value="20">
                </label>
                <div class="o_cp_searchview d-flex input-group" role="search" id="recordSearchRoot">
                  <div class="o_searchview form-control d-flex align-items-center py-1 border-end-0" role="search" aria-autocomplete="list">
                    <button type="button" class="d-print-none btn border-0 p-0" aria-label="Search..." title="Search..."><i class="o_searchview_icon oi oi-search" role="img"></i></button>
                    <div id="recordSearchFacets" class="o_searchview_facet_container"></div>
                    <div class="o_searchview_input_container">
                      <label class="field">
                        <span class="sr-only">Search</span>
                        <input id="recordSearch" class="o_searchview_input o_input d-print-none flex-grow-1 w-auto border-0" placeholder="Search..." role="searchbox">
                      </label>
                    </div>
                  </div>
                  <button type="button" id="recordSearchDropdown" class="o_searchview_dropdown_toggler d-print-none btn btn-outline-secondary o-dropdown-caret rounded-start-0" aria-label="Search options" aria-expanded="false"></button>
                  <div id="recordSearchMenu" class="o_search_bar_menu o_search_options dropdown-menu o-dropdown--menu" hidden>
                    <div class="o_dropdown_container o_filter_menu" id="recordFilterMenu"><h3>Filters</h3></div>
                    <div class="o_dropdown_container o_group_by_menu" id="recordGroupByMenu"><h3>Group By</h3></div>
                    <div class="o_dropdown_container o_favorite_menu" id="recordFavoriteMenu"><h3>Favorites</h3></div>
                  </div>
                </div>
                <button id="loadRows" class="secondary o-debug-only" hidden>Load</button>
              </div>
            </div>
            <div class="o_control_panel_navigation">
              <div class="o_cp_pager o_pager text-nowrap" id="recordPager"></div>
              <nav class="o_cp_switch_buttons d-print-none d-inline-flex btn-group" aria-label="Views">
                <button type="button" class="btn btn-secondary o_switch_view o_list active" aria-label="List View">List</button>
                <button type="button" id="kanbanViewButton" class="btn btn-secondary o_switch_view o_kanban" aria-label="Kanban View">Kanban</button>
                <button type="button" class="btn btn-secondary o_switch_view o_form" aria-label="Form View">Form</button>
              </nav>
            </div>
          </div>
        </div>
        <div class="o-list-content">
          <div id="rows"></div>
        </div>
      </section>
    </main>
  </div>
  <script>
    const defaultFields = {
      "res.partner": "id,display_name,name,email,phone",
      "res.users": "id,display_name,login,active",
      "res.groups": "id,display_name,name",
      "ir.actions.server": "id,display_name,name,state,model_name",
      "ir.cron": "id,display_name,name,active,nextcall",
      "ir.ui.view": "id,display_name,name,type,model",
      "ir.model": "id,display_name,model,name",
      "mail.message": "id,display_name,subject,message_type,model,res_id",
      "mail.mail": "id,display_name,subject,state,email_to",
      "fetchmail.server": "id,display_name,name,state,server_type,active",
      "sms.sms": "id,display_name,number,state",
      "whatsapp.message": "id,display_name,mobile_number,state",
      "digest.digest": "id,display_name,name,periodicity",
      "base.automation": "id,display_name,name,active",
      "ai.model": "id,display_name,name,provider"
    };
    const modelNames = Object.keys(defaultFields);
    const modelSelect = document.getElementById("model");
    const fieldsInput = document.getElementById("fields");
    const runtimeStatus = document.getElementById("runtimeStatus");
    let triedDevLogin = false;

    for (const name of modelNames) {
      const option = document.createElement("option");
      option.value = name;
      option.textContent = name;
      modelSelect.append(option);
    }

	function applyTheme(theme) {
		const next = theme === "standard" ? "standard" : "enterprise";
		document.body.dataset.theme = next;
		document.getElementById("theme").value = next;
		try { localStorage.setItem("web_theme", next); } catch (_error) {}
	}
	const requestedTheme = new URLSearchParams(location.search).get("theme");
	let storedTheme = "";
	try { storedTheme = localStorage.getItem("web_theme") || ""; } catch (_error) {}
	applyTheme(requestedTheme || storedTheme || document.body.dataset.theme);
	document.getElementById("theme").addEventListener("change", (event) => applyTheme(event.target.value));
	function setView(name) {
		document.body.dataset.view = name;
		document.body.classList.toggle("o_home_menu_background", name === "apps");
		document.body.classList.remove("o-mobile-menu-open");
		if (name !== "records") showRecordForm(false);
		const mobileMenu = document.getElementById("mobileMenu");
		if (mobileMenu) mobileMenu.setAttribute("aria-expanded", "false");
		for (const panel of document.querySelectorAll(".view-panel")) {
			panel.classList.toggle("active", panel.dataset.view === name);
		}
		for (const button of document.querySelectorAll("button[data-view]")) {
			button.classList.toggle("active", button.dataset.view === name);
		}
	}
	for (const button of document.querySelectorAll("button[data-view]")) {
		button.addEventListener("click", () => {
			setView(button.dataset.view);
			if (button.dataset.view === "apps") clearRouteState(false);
		});
	}
	document.getElementById("mobileMenu").addEventListener("click", (event) => {
		const open = !document.body.classList.contains("o-mobile-menu-open");
		document.body.classList.toggle("o-mobile-menu-open", open);
		event.currentTarget.setAttribute("aria-expanded", open ? "true" : "false");
	});
	document.getElementById("globalSearch").addEventListener("keydown", (event) => {
		if (event.key !== "Enter") return;
		const value = event.currentTarget.value.trim();
		if (!value) return;
		document.getElementById("recordSearch").value = value;
		searchRows(value);
	});
	modelSelect.addEventListener("change", () => {
		workbench.action = null;
		workbench.viewInfo = null;
		workbench.fields = [];
		workbench.fieldLabels = {};
		workbench.fieldMeta = {};
		workbench.listFieldAttrs = {};
		workbench.listViewAttrs = {};
		workbench.kanbanFieldAttrs = {};
		workbench.kanbanFields = [];
		workbench.formFieldAttrs = {};
		workbench.formFields = [];
		workbench.searchFacets = [];
		workbench.activeView = "list";
		workbench.openedRecord = null;
		showRecordForm(false);
		fieldsInput.value = defaultFields[modelSelect.value] || "id,display_name";
		renderSearchFacets();
		renderSearchMenu();
		writeRouteState(currentRouteState({action: null, menu_id: null, model: modelSelect.value, view_type: "list", id: null}), false);
    });

    async function requestJSON(path, options) {
      const response = await fetch(path, {
        credentials: "same-origin",
        headers: {"Content-Type": "application/json"},
        ...(options || {})
      });
      const text = await response.text();
      let data = null;
      try { data = text ? JSON.parse(text) : null; } catch (_error) { data = text; }
      if (!response.ok) {
        const error = new Error(typeof data === "string" ? data : JSON.stringify(data));
        error.status = response.status;
        throw error;
      }
      return data;
    }

    async function callKW(model, method, payload) {
      const body = Object.assign({model, method}, payload || {});
      return requestJSON("/web/dataset/call_kw", {
        method: "POST",
        body: JSON.stringify(body)
      });
    }

    async function login() {
      const login = document.getElementById("login").value;
      const password = document.getElementById("password").value;
      await requestJSON("/web/session/authenticate", {
        method: "POST",
        body: JSON.stringify({login, password})
      });
      document.getElementById("loginPanel").classList.remove("active");
      runtimeStatus.textContent = "";
      runtimeStatus.className = "status-ok";
      await loadRuntime();
      await loadRows();
    }

    async function ensureSession(error) {
      if (!error || error.status !== 401) return false;
      document.getElementById("loginPanel").classList.add("active");
      if (!triedDevLogin) {
        triedDevLogin = true;
        await login();
        return true;
      }
      return false;
    }

    function setText(id, value) {
      document.getElementById(id).textContent = value;
    }

    function pretty(value) {
      return JSON.stringify(value, null, 2);
    }

    async function loadRows() {
      const model = modelSelect.value;
      const fields = ["id", ...activeRecordFields()].filter((value, index, list) => value && list.indexOf(value) === index);
      const limit = Number(document.getElementById("limit").value || 20);
      document.getElementById("rows").textContent = "Loading...";
      try {
        await loadSearchResults(model, fields, limit, document.getElementById("recordSearch").value);
      } catch (error) {
        if (await ensureSession(error)) return;
        document.getElementById("rows").innerHTML = "";
        const pre = document.createElement("pre");
        pre.textContent = "Error " + (error.status || "") + "\\n" + error.message;
        document.getElementById("rows").append(pre);
        setView("records");
      }
    }

    async function searchRows(value) {
      const fields = ["id", ...activeRecordFields()].filter((item, index, list) => item && list.indexOf(item) === index);
      document.getElementById("rows").textContent = "Loading...";
      try {
        await loadSearchResults(modelSelect.value, fields, 20, value);
        setView("records");
      } catch (error) {
        if (await ensureSession(error)) return;
        document.getElementById("rows").textContent = "Search error: " + error.message;
        setView("records");
      }
    }

    async function loadSearchResults(model, fields, limit, query) {
      const searchState = currentSearchState(query);
      renderSearchFacets();
      renderSearchMenu();
      if (searchState.groupBy.length) {
        const payload = await callKW(model, "web_read_group", {
          kwargs: {
            domain: searchState.domain,
            groupby: searchState.groupBy,
            aggregates: ["__count"],
            limit,
            order: searchState.groupBy[0],
            context: searchState.context
          }
        });
        renderGroupedRows((payload && payload.groups) || [], searchState.groupBy[0]);
        return;
      }
      const payload = await callKW(model, "web_search_read", {
        kwargs: {
          domain: searchState.domain,
          specification: fieldSpecification(fields),
          limit,
          count_limit: 10001,
          order: "id desc",
          context: searchState.context
        }
      });
      renderRows((payload && payload.records) || [], fields);
    }

    async function createPartner() {
      const stamp = new Date().toISOString().replace(/[:.]/g, "-");
      const model = modelSelect.value;
      const knownFields = new Set((fieldsInput.value || defaultFields[model] || "").split(",").map((field) => field.trim()).filter(Boolean));
      const values = {};
      if (knownFields.has("name")) values.name = "New " + stamp;
      if (model === "res.partner" && knownFields.has("email")) values.email = "new-" + stamp + "@example.test";
      const created = await callKW(model, "create", {values, kwargs: {context: readContext(workbench.action)}});
      await loadRows();
      setView("records");
      const id = created && created.id;
      if (id) await openRecord(model, id);
    }

    const workbench = {
      menus: {},
      modules: {},
      openedRecord: null,
      action: null,
      currentMenuID: null,
      restoringRoute: false,
      viewInfo: null,
      fields: [],
      fieldLabels: {},
      fieldMeta: {},
      listFieldAttrs: {},
      listViewAttrs: {},
      kanbanFieldAttrs: {},
      kanbanFields: [],
      formFieldAttrs: {},
      formFields: [],
      searchFacets: [],
      activeView: "list"
    };

    function parseRouteState() {
      const params = new URLSearchParams(String(location.hash || "").replace(/^#/, ""));
      const state = {};
      for (const [key, value] of params.entries()) {
        if (!value) continue;
        state[key] = value;
      }
      return state;
    }

    function serializeRouteState(state) {
      const keys = ["action", "model", "view_type", "id", "menu_id", "debug"];
      const params = new URLSearchParams();
      for (const key of keys) {
        const value = state[key];
        if (value === undefined || value === null || value === false || value === "") continue;
        params.set(key, String(value));
      }
      const text = params.toString();
      return text ? "#" + text : "";
    }

    function currentRouteState(extra) {
      const state = {
        action: workbench.action && workbench.action.id,
        model: modelSelect.value,
        view_type: workbench.openedRecord ? "form" : workbench.activeView,
        id: workbench.openedRecord && workbench.openedRecord.id,
        menu_id: workbench.currentMenuID
      };
      return Object.assign(state, extra || {});
    }

    function writeRouteState(state, replace) {
      if (workbench.restoringRoute) return;
      const hash = serializeRouteState(state || {});
      const next = location.pathname + location.search + hash;
      if (replace) history.replaceState({goerpRoute: true}, "", next);
      else history.pushState({goerpRoute: true}, "", next);
      document.body.dataset.routeHash = hash ? hash.slice(1) : "";
    }

    function clearRouteState(replace) {
      writeRouteState({}, replace);
    }

    async function restoreRouteFromHash() {
      const state = parseRouteState();
      if (!state.action && !state.menu_id && !state.model) return false;
      workbench.restoringRoute = true;
      try {
        if (state.action) {
          await openAction(state.action, {menuID: state.menu_id, noRoute: true});
          if (state.view_type && state.view_type !== "form") {
            workbench.activeView = state.view_type === "kanban" ? "kanban" : "list";
            updateViewSwitchButtons();
            await loadRows();
          }
          if (state.id) await openRecord(state.model || modelSelect.value, state.id, {noRoute: true});
          return true;
        }
        if (state.menu_id) {
          await openMenu(state.menu_id, {noRoute: true});
          return true;
        }
        if (state.model) {
          ensureModelOption(state.model);
          modelSelect.value = state.model;
          fieldsInput.value = defaultFields[state.model] || "id,display_name";
          setView("records");
          await loadRows();
          return true;
        }
        return false;
      } finally {
        workbench.restoringRoute = false;
        if (!workbench.action && document.body.dataset.view === "apps") {
          writeRouteState({menu_id: state.menu_id}, true);
        } else {
          writeRouteState(currentRouteState({view_type: state.view_type || (workbench.openedRecord ? "form" : workbench.activeView), id: state.id || (workbench.openedRecord && workbench.openedRecord.id)}), true);
        }
      }
    }

    function currentSearchState(query) {
      return {
        domain: combinedDomain(actionDomain(workbench.action), query),
        context: readContext(workbench.action),
        groupBy: workbench.searchFacets.filter((facet) => facet.type === "groupBy").map((facet) => facet.field).filter(Boolean)
      };
    }

    function fieldAvailable(name) {
      if (!name) return false;
      if (workbench.fieldMeta && workbench.fieldMeta[name]) return true;
      return activeRecordFields().includes(name) || (defaultFields[modelSelect.value] || "").split(",").map((field) => field.trim()).includes(name);
    }

    function searchFilterItems() {
      const items = [];
      if (fieldAvailable("active")) {
        items.push({id: "active", type: "filter", label: "Active", domain: [["active", "=", true]]});
        items.push({id: "archived", type: "filter", label: "Archived", domain: [["active", "=", false]]});
      }
      if (modelSelect.value === "ir.actions.server" && fieldAvailable("state")) {
        items.push({id: "state-code", type: "filter", label: "Code", domain: [["state", "=", "code"]]});
      }
      if (modelSelect.value === "mail.mail" && fieldAvailable("state")) {
        items.push({id: "mail-outgoing", type: "filter", label: "Outgoing", domain: [["state", "!=", "sent"]]});
      }
      return items;
    }

    function searchGroupByItems() {
      const preferred = ["state", "model_name", "model", "active", "type", "message_type", "server_type", "periodicity", "provider", "user_id", "company_id"];
      return preferred.filter(fieldAvailable).slice(0, 8).map((field) => ({
        id: "group-" + field,
        type: "groupBy",
        label: fieldLabel(field),
        field
      }));
    }

    function favoriteStorageKey() {
      return "goerp.searchFavorites." + modelSelect.value;
    }

    function searchFavorites() {
      try {
        const parsed = JSON.parse(localStorage.getItem(favoriteStorageKey()) || "[]");
        return Array.isArray(parsed) ? parsed.filter((item) => item && item.label) : [];
      } catch (_error) {
        return [];
      }
    }

    function saveCurrentFavorite() {
      const query = document.getElementById("recordSearch").value || "";
      const facets = workbench.searchFacets.map((facet) => ({...facet}));
      const label = query || facets.map((facet) => facet.label).join(", ") || "Current Search";
      const favorite = {id: "favorite-" + Date.now(), label, query, facets};
      const favorites = [favorite, ...searchFavorites().filter((item) => item.label !== label)].slice(0, 8);
      try { localStorage.setItem(favoriteStorageKey(), JSON.stringify(favorites)); } catch (_error) {}
      renderSearchMenu();
    }

    function applyFavorite(favorite) {
      document.getElementById("recordSearch").value = favorite.query || "";
      workbench.searchFacets = Array.isArray(favorite.facets) ? favorite.facets.map((facet) => ({...facet})) : [];
      loadRows();
    }

    function clearSearchState() {
      document.getElementById("recordSearch").value = "";
      workbench.searchFacets = [];
      closeSearchMenu();
      loadRows();
    }

    function searchFacetActive(id) {
      return workbench.searchFacets.some((facet) => facet.id === id);
    }

    function toggleSearchFacet(facet) {
      const current = workbench.searchFacets;
      workbench.searchFacets = current.some((item) => item.id === facet.id)
        ? current.filter((item) => item.id !== facet.id)
        : [...current, {...facet}];
      loadRows();
    }

    function removeSearchFacet(id) {
      workbench.searchFacets = workbench.searchFacets.filter((facet) => facet.id !== id);
      loadRows();
    }

    function renderSearchFacets() {
      const host = document.getElementById("recordSearchFacets");
      if (!host) return;
      host.replaceChildren();
      for (const facet of workbench.searchFacets) {
        const item = document.createElement("div");
        item.className = "o_searchview_facet position-relative d-inline-flex";
        item.dataset.facetId = facet.id;
        const label = document.createElement("span");
        label.className = "o_searchview_facet_label";
        label.textContent = facet.type === "groupBy" ? "Group By" : "Filter";
        const value = document.createElement("span");
        value.className = "o_facet_values";
        value.textContent = facet.label;
        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "o_facet_remove";
        remove.setAttribute("aria-label", "Remove " + facet.label);
        remove.textContent = "x";
        remove.addEventListener("click", () => removeSearchFacet(facet.id));
        item.append(label, value, remove);
        host.append(item);
      }
    }

    function renderSearchMenu() {
      renderSearchMenuSection("recordFilterMenu", "Filters", searchFilterItems(), toggleSearchFacet);
      renderSearchMenuSection("recordGroupByMenu", "Group By", searchGroupByItems(), toggleSearchFacet);
      const favorites = searchFavorites();
      const favoriteHost = document.getElementById("recordFavoriteMenu");
      if (favoriteHost) {
        favoriteHost.replaceChildren();
        const heading = document.createElement("h3");
        heading.textContent = "Favorites";
        favoriteHost.append(heading);
        appendSearchMenuButton(favoriteHost, {id: "save-current", label: "Save current search"}, saveCurrentFavorite, false);
        appendSearchMenuButton(favoriteHost, {id: "clear-search", label: "Clear search"}, clearSearchState, false);
        for (const favorite of favorites) {
          appendSearchMenuButton(favoriteHost, favorite, () => applyFavorite(favorite), false);
        }
      }
    }

    function renderSearchMenuSection(id, title, items, handler) {
      const host = document.getElementById(id);
      if (!host) return;
      host.replaceChildren();
      const heading = document.createElement("h3");
      heading.textContent = title;
      host.append(heading);
      if (!items.length) {
        const empty = document.createElement("span");
        empty.className = "muted";
        empty.textContent = "No items";
        host.append(empty);
        return;
      }
      for (const item of items) appendSearchMenuButton(host, item, () => handler(item), searchFacetActive(item.id));
    }

    function appendSearchMenuButton(host, item, handler, active) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "dropdown-item o_menu_item" + (active ? " active" : "");
      button.dataset.searchItem = item.id || "";
      button.textContent = item.label || "Item";
      if (active) {
        const check = document.createElement("span");
        check.className = "o_search_check";
        check.textContent = "v";
        button.append(check);
      }
      button.addEventListener("click", handler);
      host.append(button);
    }

    function toggleSearchMenu() {
      const menu = document.getElementById("recordSearchMenu");
      const button = document.getElementById("recordSearchDropdown");
      if (!menu || !button) return;
      renderSearchMenu();
      const open = menu.hidden;
      menu.hidden = !open;
      button.setAttribute("aria-expanded", open ? "true" : "false");
    }

    function closeSearchMenu() {
      const menu = document.getElementById("recordSearchMenu");
      const button = document.getElementById("recordSearchDropdown");
      if (menu) menu.hidden = true;
      if (button) button.setAttribute("aria-expanded", "false");
    }

    function actionContext(action) {
      if (action && typeof action._web_context === "object" && !Array.isArray(action._web_context)) return action._web_context;
      return (action && typeof action.context === "object" && !Array.isArray(action.context)) ? action.context : {};
    }

    function readContext(action) {
      const context = {...actionContext(action), bin_size: true};
      for (const key of Object.keys(context)) {
        if (key.startsWith("search_default_")) delete context[key];
      }
      return context;
    }

    function viewLoadContext(action) {
      const context = {};
      for (const [key, value] of Object.entries(actionContext(action))) {
        if (key === "lang" || key.endsWith("_view_ref")) context[key] = value;
      }
      return context;
    }

    function searchDefaultDomain(action) {
      const out = [];
      for (const [key, value] of Object.entries(actionContext(action))) {
        if (!key.startsWith("search_default_") || value === false || value === null || value === undefined) continue;
        const field = key.slice("search_default_".length);
        if (!field) continue;
        out.push([field, "=", value === true ? true : value]);
      }
      return out;
    }

    function actionDomain(action) {
      if (action && Array.isArray(action._web_domain)) return action._web_domain;
      if (!action || action.domain === undefined || action.domain === null || action.domain === false || action.domain === "") return [];
      if (Array.isArray(action.domain)) return action.domain;
      if (typeof action.domain === "string") {
        try {
          const parsed = JSON.parse(action.domain.replaceAll("'", '"'));
          return Array.isArray(parsed) ? parsed : [];
        } catch (_error) {
          return [];
        }
      }
      return [];
    }

    function combinedDomain(base, needle) {
      const domain = Array.isArray(base) ? [...base] : [];
      for (const item of searchDefaultDomain(workbench.action)) domain.push(item);
      for (const facet of workbench.searchFacets) {
        if (facet.type === "filter" && Array.isArray(facet.domain)) domain.push(...facet.domain);
      }
      const term = (needle || "").trim();
      if (term) domain.push(["display_name", "ilike", term]);
      return domain;
    }

    function actionSearchViewID(action) {
      const value = action && action.search_view_id;
      if (Array.isArray(value)) return value[0] || false;
      return value || false;
    }

    function normalizeActionViews(action) {
      const views = Array.isArray(action && action.views) ? action.views : [];
      const out = [];
      for (const item of views) {
        if (!Array.isArray(item) || item.length < 2) continue;
        out.push([item[0] || false, item[1]]);
      }
      if (!out.some((item) => item[1] === "list")) out.push([false, "list"]);
      if (!out.some((item) => item[1] === "form")) out.push([false, "form"]);
      if (!out.some((item) => item[1] === "search")) out.push([actionSearchViewID(action), "search"]);
      return out;
    }

    function firstDisplayView(action) {
      for (const item of normalizeActionViews(action || {})) {
        if (item[1] && item[1] !== "search") return item[1];
      }
      return "list";
    }

    function activeRecordFields() {
      if (workbench.activeView === "kanban" && workbench.kanbanFields.length) return workbench.kanbanFields;
      return workbench.fields.length ? workbench.fields : fieldsInput.value.split(",").map((field) => field.trim()).filter(Boolean);
    }

    function updateViewSwitchButtons() {
      for (const button of document.querySelectorAll(".o_cp_switch_buttons .o_switch_view")) {
        const isKanban = button.classList.contains("o_kanban");
        const isList = button.classList.contains("o_list");
        const isForm = button.classList.contains("o_form");
        button.classList.toggle("active", (workbench.activeView === "kanban" && isKanban) || (workbench.activeView === "form" && isForm) || (workbench.activeView !== "kanban" && workbench.activeView !== "form" && isList));
      }
    }

    function viewArchFields(arch) {
      return viewArchFieldNodes(arch).map((node) => node.name);
    }

    function viewArchFieldNodes(arch) {
      const out = [];
      if (typeof arch !== "string" || !arch) return out;
      try {
        const doc = new DOMParser().parseFromString(arch, "text/xml");
        for (const node of doc.querySelectorAll("field[name]")) {
          const name = node.getAttribute("name");
          if (!name || out.some((item) => item.name === name)) continue;
          const attrs = {};
          for (const attr of node.attributes) attrs[attr.name] = attr.value;
          out.push({name, attrs});
        }
      } catch (_error) {}
      return out;
    }

    function viewRootAttrs(arch, tagName) {
      if (typeof arch !== "string" || !arch) return {};
      try {
        const doc = new DOMParser().parseFromString(arch, "text/xml");
        const root = doc.documentElement;
        if (!root || root.tagName.toLowerCase() !== tagName) return {};
        const attrs = {};
        for (const attr of root.attributes) attrs[attr.name] = attr.value;
        return attrs;
      } catch (_error) {
        return {};
      }
    }

    function fieldAttrMap(nodes) {
      const out = {};
      for (const node of nodes || []) {
        if (node && node.name && !out[node.name]) out[node.name] = node.attrs || {};
      }
      return out;
    }

    function viewFieldLabels(model, viewInfo) {
      const labels = {};
      const fields = (((viewInfo || {}).models || {})[model] || {}).fields || {};
      for (const [name, meta] of Object.entries(fields)) {
        labels[name] = (meta && (meta.string || meta.display_name)) || name;
      }
      return labels;
    }

    function fieldSpecification(fields) {
      const spec = {};
      for (const field of fields) spec[field] = {};
      if (!spec.id) spec.id = {};
      if (!spec.display_name) spec.display_name = {};
      return spec;
    }

    function humanFieldLabel(field) {
      const labels = {
        display_name: "Name",
        model_id: "Model",
        model_name: "Model",
        res_id: "Record",
        res_model: "Document model",
        email_to: "Email to",
        mobile_number: "Mobile",
        server_type: "Server type",
        nextcall: "Next run"
      };
      if (labels[field]) return labels[field];
      return field.split("_").filter(Boolean).map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(" ") || field;
    }

    function fieldLabel(field) {
      const label = workbench.fieldLabels[field];
      return label && label !== field ? label : humanFieldLabel(field);
    }

    function visibleFormFields(fields) {
      const out = [];
      for (const field of fields) {
        if (field === "id" || field.startsWith("__")) continue;
        if (field === "display_name" && fields.some((item) => item !== "id" && item !== "display_name")) continue;
        if (!out.includes(field)) out.push(field);
      }
      return out.length ? out : ["display_name"];
    }

    async function loadActionViews(action, model) {
      const viewInfo = await callKW(model, "get_views", {
          kwargs: {
            views: normalizeActionViews(action),
            options: {toolbar: true, load_filters: true, action_id: action.id || false},
            context: viewLoadContext(action)
          }
        });
      const listView = (((viewInfo || {}).views || {}).list) || {};
      const kanbanView = (((viewInfo || {}).views || {}).kanban) || {};
      const formView = (((viewInfo || {}).views || {}).form) || {};
      const listNodes = viewArchFieldNodes(listView.arch);
      const kanbanNodes = viewArchFieldNodes(kanbanView.arch);
      const formNodes = viewArchFieldNodes(formView.arch);
      const listFields = listNodes.map((node) => node.name).filter((field) => field !== "id");
      const kanbanFields = kanbanNodes.map((node) => node.name).filter((field) => field !== "id");
      const formFields = formNodes.map((node) => node.name).filter((field) => field !== "id");
      const fallback = (defaultFields[model] || "id,display_name,name").split(",").map((field) => field.trim()).filter(Boolean);
      workbench.viewInfo = viewInfo || {};
      workbench.activeView = firstDisplayView(action);
      workbench.fields = (listFields.length ? listFields : fallback).filter((field) => field !== "id");
      workbench.kanbanFields = (kanbanFields.length ? kanbanFields : workbench.fields).filter((field) => field !== "id");
      workbench.formFields = (formFields.length ? formFields : workbench.fields).filter((field) => field !== "id");
      workbench.fieldLabels = viewFieldLabels(model, viewInfo);
      workbench.fieldMeta = (((viewInfo || {}).models || {})[model] || {}).fields || {};
      workbench.listFieldAttrs = fieldAttrMap(listNodes);
      workbench.listViewAttrs = viewRootAttrs(listView.arch, "list");
      workbench.kanbanFieldAttrs = fieldAttrMap(kanbanNodes);
      workbench.formFieldAttrs = fieldAttrMap(formNodes);
      fieldsInput.value = ["id", ...activeRecordFields()].filter((value, index, list) => list.indexOf(value) === index).join(",");
      document.getElementById("recordFields").value = ["id", ...workbench.formFields].filter((value, index, list) => list.indexOf(value) === index).join(",");
      updateViewSwitchButtons();
    }

    function buildWorkbenchPanels() {
      const main = document.querySelector("main");
      const modelPanel = document.getElementById("rows").closest(".panel");

      const appPanel = document.createElement("section");
      appPanel.id = "appsView";
      appPanel.className = "panel view-panel active o-app-launcher-view o_app_launcher o_home_menu_background";
      appPanel.dataset.view = "apps";
      appPanel.innerHTML = '<div class="o-app-shell o_home_menu h-100 overflow-auto"><div class="container o_home_menu_container"><div class="o-app-search o_home_menu_search" data-search-active="false"><label><span class="sr-only">Search apps</span><input id="appSearch" type="text" class="o_app_search_stub o_search_hidden visually-hidden" data-allow-hotkeys="true" aria-label="Search apps and menus" role="combobox" aria-autocomplete="list" aria-haspopup="listbox" aria-expanded="false"></label></div><div id="appGrid" class="o_apps row user-select-none mt-5 mx-0" role="listbox"></div><div id="menuStatus" class="o-app-message muted">Loading menus...</div><div id="menuList" class="menu-list o-app-message"></div></div></div>';
      main.insertBefore(appPanel, modelPanel);

      const settingsPanel = document.createElement("section");
      settingsPanel.id = "settingsView";
      settingsPanel.className = "panel view-panel o_form_view o_settings_view";
      settingsPanel.dataset.view = "settings";
      settingsPanel.innerHTML = '<div class="o-control-panel o_control_panel o-form-control"><div class="o_control_panel_main"><div class="o_control_panel_breadcrumbs"><div class="o_control_panel_main_buttons d-print-none"><button id="settingsSave" class="btn btn-primary o_form_button_save">Save</button><button id="settingsDiscard" class="btn btn-secondary o_form_button_cancel">Discard</button></div><h2 class="o_breadcrumb active">Settings</h2></div><div class="o_control_panel_actions"><div class="o_cp_searchview d-flex input-group" role="search"><div class="o_searchview form-control d-flex align-items-center py-1 border-end-0" role="search" aria-autocomplete="list"><button type="button" class="d-print-none btn border-0 p-0" aria-label="Search..." title="Search..."><i class="o_searchview_icon oi oi-search" role="img"></i></button><div class="o_searchview_input_container"><label class="field"><span class="sr-only">Search settings</span><input id="settingsSearch" class="o_searchview_input o_input d-print-none flex-grow-1 w-auto border-0" placeholder="Search..." role="searchbox"></label></div></div><button type="button" class="o_searchview_dropdown_toggler d-print-none btn btn-outline-secondary o-dropdown-caret rounded-start-0" aria-label="Search options"></button></div></div><div class="o_control_panel_navigation"><div class="o_cp_pager o_pager text-nowrap" id="settingsPager"></div></div></div></div><div class="o_settings_content o_form_sheet_bg"><div id="settingsBlocks" class="o_setting_container o_legacy_settings_blocks"></div></div>';
      main.insertBefore(settingsPanel, modelPanel);

      const modulePanel = document.createElement("section");
      modulePanel.id = "installView";
      modulePanel.className = "panel view-panel o-list-view";
      modulePanel.dataset.view = "install";
      modulePanel.innerHTML = '<div class="o-control-panel o_control_panel"><div class="o_control_panel_main"><div class="o_control_panel_breadcrumbs"><div class="o_control_panel_main_buttons d-print-none"><button id="reloadApps" class="btn btn-secondary">Reload</button></div><h2 class="o_breadcrumb active">Apps</h2></div><div class="o_control_panel_actions"><div class="toolbar"><div class="o_cp_searchview d-flex input-group" role="search"><div class="o_searchview form-control d-flex align-items-center py-1 border-end-0" role="search" aria-autocomplete="list"><button type="button" class="d-print-none btn border-0 p-0" aria-label="Search..." title="Search..."><i class="o_searchview_icon oi oi-search" role="img"></i></button><div class="o_searchview_input_container"><label class="field"><span class="sr-only">Search</span><input id="moduleSearch" class="o_searchview_input o_input d-print-none flex-grow-1 w-auto border-0" placeholder="Search..." role="searchbox"></label></div></div><button type="button" class="o_searchview_dropdown_toggler d-print-none btn btn-outline-secondary o-dropdown-caret rounded-start-0" aria-label="Search options"></button></div></div></div><div class="o_control_panel_navigation"><div class="o_cp_pager o_pager text-nowrap" id="modulePager"></div></div></div></div><div class="o-list-content"><div id="moduleGrid" class="module-grid o_apps"></div></div>';
      main.insertBefore(modulePanel, modelPanel.nextSibling);

      const recordPanel = document.createElement("section");
      recordPanel.className = "panel record-panel o_form_view";
      recordPanel.id = "recordPanel";
      recordPanel.hidden = true;
      recordPanel.innerHTML = '<div class="o-control-panel o_control_panel o-form-control"><div class="o_control_panel_main"><div class="o_control_panel_main_buttons d-print-none"><button id="saveRecord" class="btn btn-primary o_form_button_save">Save</button><button id="readRecord" class="btn btn-secondary o_form_button_cancel">Discard</button></div><div class="o_control_panel_breadcrumbs"><div class="o-breadcrumbs o_breadcrumb"><button id="recordBack" type="button" class="secondary">Records</button><span>/</span><h2 id="recordTitle" class="active">Record</h2></div></div><div class="o_control_panel_actions"></div><div class="o_control_panel_navigation"><div class="o_cp_pager o_pager text-nowrap"><span class="o_pager_value">1</span><span>/</span><span class="o_pager_limit">1</span></div></div></div><div class="o_control_panel_meta"><input id="recordModel" hidden><input id="recordID" hidden><input id="recordFields" hidden></div></div><div class="o-list-content o-form-content o_form_sheet_bg"><div id="recordForm" class="record-grid o-form-sheet o_form_sheet"></div><pre id="recordRaw" hidden></pre></div>';
      modelPanel.append(recordPanel);

      document.getElementById("reloadApps").addEventListener("click", loadInstallApps);
      document.getElementById("moduleSearch").addEventListener("input", loadInstallApps);
      const appSearch = document.getElementById("appSearch");
      appSearch.addEventListener("input", () => {
        setLegacyAppSearchActive(Boolean(appSearch.value.trim()));
        renderApps(workbench.menus);
      });
      appSearch.addEventListener("blur", () => {
        if (!appSearch.value.trim()) setLegacyAppSearchActive(false);
      });
      appPanel.addEventListener("keydown", handleLegacyLauncherKeydown);
      document.getElementById("settingsSearch").addEventListener("input", () => renderSettingsView(workbench.action || {}));
      document.getElementById("settingsDiscard").addEventListener("click", () => renderSettingsView(workbench.action || {}));
      appSearch.addEventListener("keydown", (event) => {
        if (event.key !== "Enter") return;
        const cards = Array.from(document.querySelectorAll("#appGrid .o_app"));
        if (cards.length === 1) cards[0].click();
      });
      document.getElementById("recordBack").addEventListener("click", () => {
        workbench.openedRecord = null;
        if (workbench.activeView === "form") workbench.activeView = "list";
        updateViewSwitchButtons();
        showRecordForm(false);
        writeRouteState(currentRouteState({view_type: workbench.activeView === "form" ? "list" : workbench.activeView, id: null}), false);
      });
      document.getElementById("readRecord").addEventListener("click", () => {
        if (workbench.openedRecord) openRecord(workbench.openedRecord.model, workbench.openedRecord.id);
      });
      document.getElementById("saveRecord").addEventListener("click", saveRecord);
    }

    function showRecordForm(active) {
      const recordPanel = document.getElementById("recordPanel");
      if (recordPanel) recordPanel.hidden = !active;
      if (recordPanel) recordPanel.classList.toggle("active", active);
      const listContent = document.querySelector("#recordsView > .o-list-content");
      if (listContent) listContent.hidden = active;
      const listControl = document.querySelector("#recordsView > .o-control-panel");
      if (listControl) listControl.hidden = active;
      const listToolbar = document.querySelector("#recordsView > .o-control-panel .toolbar");
      if (listToolbar) listToolbar.hidden = active;
    }

    function ensureModelOption(model) {
      if (!model) return;
      for (const option of modelSelect.options) {
        if (option.value === model) return;
      }
      const option = document.createElement("option");
      option.value = model;
      option.textContent = model;
      modelSelect.append(option);
    }

    function installedModuleNames(payload) {
      if (Array.isArray(payload)) return payload;
      if (payload && Array.isArray(payload.installed_modules)) return payload.installed_modules;
      if (payload && payload.modules) return Object.keys(payload.modules);
      return [];
    }

    function renderSidebarModules(payload) {
      const names = installedModuleNames(payload);
      workbench.modules = (payload && payload.modules) || {};
      setText("moduleCount", String(names.length));
      const moduleHost = document.getElementById("modules");
      moduleHost.replaceChildren();
      for (const name of names.slice(0, 12)) {
        const item = document.createElement("li");
        const left = document.createElement("span");
        left.textContent = name;
        const right = document.createElement("span");
        right.className = "muted";
        right.textContent = "installed";
        item.append(left, right);
        moduleHost.append(item);
      }
    }

    function menuDisplayName(menu) {
      const name = (menu && typeof menu.name === "string" && menu.name.trim()) ? menu.name.trim() : "Menu";
      const technicalNames = {
        "Outgoing Mail Servers": "Outgoing Mail Servers",
        "Incoming Mail Servers": "Incoming Mail Server",
        "Window Actions": "Window Actions",
        "Client Actions": "Client Actions",
        "Server Actions": "Server Actions",
        "Report Actions": "Reports",
        "Reports": "Reports"
      };
      return technicalNames[name] || name;
    }

    function renderSidebarMenu(menu) {
      const moduleHost = document.getElementById("modules");
      const topMenu = document.getElementById("topMenu");
      moduleHost.replaceChildren();
      topMenu.replaceChildren();
      document.getElementById("sidebarTitle").textContent = menuDisplayName(menu);
      const childIDs = (menu && menu.children) || [];
      for (const childID of childIDs) {
        const child = menuEntry(childID);
        if (!child) continue;
        const item = document.createElement("li");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "secondary";
        button.textContent = menuDisplayName(child);
        button.addEventListener("click", () => openMenu(child.id));
        item.append(button);
        moduleHost.append(item);
        const topButton = document.createElement("button");
        topButton.type = "button";
        topButton.className = "o_nav_entry";
        topButton.textContent = menuDisplayName(child);
        topButton.dataset.menuId = String(child.id);
        if (child.xmlid) topButton.dataset.menuXmlid = child.xmlid;
        if ((child.children || []).length) {
          topButton.className = "o_nav_entry o_nav_dropdown_toggle dropdown-toggle";
          topButton.setAttribute("aria-haspopup", "menu");
          topButton.setAttribute("aria-expanded", "false");
          const dropdown = renderTopMenuDropdown(child);
          topButton.addEventListener("click", (event) => {
            event.stopPropagation();
            const open = topButton.getAttribute("aria-expanded") !== "true";
            closeTopMenuDropdowns(dropdown);
            setTopMenuDropdownOpen(topButton, dropdown, open);
          });
          topMenu.append(topButton, dropdown);
        } else {
          topButton.addEventListener("click", () => openMenu(child.id));
          topMenu.append(topButton);
        }
      }
      if (!moduleHost.children.length && menu) {
        const item = document.createElement("li");
        const left = document.createElement("span");
        left.textContent = menuDisplayName(menu);
        item.append(left);
        moduleHost.append(item);
      }
    }

    function renderTopMenuDropdown(menu) {
      const dropdown = document.createElement("div");
      dropdown.className = "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu";
      dropdown.dataset.navbarDropdown = String(menu.id);
      dropdown.hidden = true;
      dropdown.setAttribute("role", "menu");
      appendTopMenuDropdownItems(dropdown, menu.children || [], 0);
      return dropdown;
    }

    function appendTopMenuDropdownItems(dropdown, childIDs, level) {
      for (const childID of childIDs || []) {
        const child = menuEntry(childID);
        if (!child) continue;
        if ((child.children || []).length) {
          const group = document.createElement("div");
          group.className = "o_navbar_dropdown_group";
          group.dataset.menuId = String(child.id);
          group.dataset.menuLevel = String(level);
          group.setAttribute("role", "none");
          const item = document.createElement("button");
          item.type = "button";
          item.className = "dropdown-item o_navbar_dropdown_item o_navbar_submenu_toggle dropdown-toggle";
          item.dataset.menuId = String(child.id);
          item.dataset.menuLevel = String(level);
          if (child.xmlid) item.dataset.menuXmlid = child.xmlid;
          item.setAttribute("role", "menuitem");
          item.setAttribute("aria-haspopup", "menu");
          item.setAttribute("aria-expanded", "false");
          item.textContent = menuDisplayName(child);
          const submenu = document.createElement("div");
          submenu.className = "dropdown-menu o-dropdown-menu o_navbar_submenu_menu";
          submenu.dataset.navbarSubmenu = String(child.id);
          submenu.hidden = true;
          submenu.setAttribute("role", "menu");
          appendTopMenuDropdownItems(submenu, child.children || [], level + 1);
          item.addEventListener("click", (event) => {
            event.stopPropagation();
            const open = item.getAttribute("aria-expanded") !== "true";
            closeSiblingTopSubmenus(group);
            setTopSubmenuOpen(item, submenu, open);
          });
          group.append(item, submenu);
          dropdown.append(group);
        } else if (menuHasDirectAction(child)) {
          const item = document.createElement("button");
          item.type = "button";
          item.className = "dropdown-item o_navbar_dropdown_item";
          item.dataset.menuId = String(child.id);
          item.dataset.menuLevel = String(level);
          if (child.xmlid) item.dataset.menuXmlid = child.xmlid;
          item.setAttribute("role", "menuitem");
          item.textContent = menuDisplayName(child);
          item.addEventListener("click", () => {
            closeTopMenuDropdowns();
            openMenu(child.id);
          });
          dropdown.append(item);
        } else {
          const header = document.createElement("span");
          header.className = "dropdown-header o_navbar_dropdown_header";
          header.dataset.menuId = String(child.id);
          header.dataset.menuLevel = String(level);
          header.textContent = menuDisplayName(child);
          dropdown.append(header);
        }
      }
    }

    function setTopMenuDropdownOpen(button, dropdown, open) {
      button.setAttribute("aria-expanded", open ? "true" : "false");
      dropdown.hidden = !open;
      dropdown.className = open ? "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu show" : "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu";
    }

    function closeTopMenuDropdowns(except) {
      closeAllTopSubmenus();
      for (const dropdown of document.querySelectorAll("#topMenu .o_navbar_dropdown_menu")) {
        if (dropdown === except) continue;
        const button = dropdown.previousElementSibling;
        if (button) button.setAttribute("aria-expanded", "false");
        dropdown.hidden = true;
        dropdown.className = "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu";
      }
    }

    function setTopSubmenuOpen(button, submenu, open) {
      button.setAttribute("aria-expanded", open ? "true" : "false");
      submenu.hidden = !open;
      submenu.className = open ? "dropdown-menu o-dropdown-menu o_navbar_submenu_menu show" : "dropdown-menu o-dropdown-menu o_navbar_submenu_menu";
    }

    function closeAllTopSubmenus() {
      for (const submenu of document.querySelectorAll("#topMenu .o_navbar_submenu_menu")) {
        const button = submenu.previousElementSibling;
        if (button) button.setAttribute("aria-expanded", "false");
        submenu.hidden = true;
        submenu.className = "dropdown-menu o-dropdown-menu o_navbar_submenu_menu";
      }
    }

    function closeSiblingTopSubmenus(group) {
      const parent = group.parentElement;
      if (!parent) return;
      for (const sibling of parent.children || []) {
        if (sibling === group) continue;
        const submenu = sibling.querySelector && sibling.querySelector(".o_navbar_submenu_menu");
        const button = sibling.querySelector && sibling.querySelector(".o_navbar_submenu_toggle");
        if (submenu && button) setTopSubmenuOpen(button, submenu, false);
      }
    }

    function navigationMenuFor(menu) {
      if (!menu || (menu.children || []).length || !menuHasDirectAction(menu)) return menu;
      return menuEntry(menu.parent_id) || menu;
    }

    function menuEntry(menuID) {
      const key = String(menuID);
      return richerMenuEntry(workbench.menus[key], (workbench.menus.children || {})[key]) || null;
    }

    function richerMenuEntry(left, right) {
      if (!left || typeof left !== "object") return right;
      if (!right || typeof right !== "object") return left;
      const leftChildren = Array.isArray(left.children) ? left.children.length : 0;
      const rightChildren = Array.isArray(right.children) ? right.children.length : 0;
      return rightChildren > leftChildren ? right : left;
    }

    function menuRootIDs(payload) {
      if (!payload) return [];
      if (Array.isArray(payload.menu_roots)) return payload.menu_roots;
      const root = payload.root || {};
      if (Array.isArray(root.children)) return root.children;
      return [];
    }

    function cleanAppName(value) {
      return String(value || "App").replace(/\s+/g, " ").trim() || "App";
    }

    function appKey(name) {
      return cleanAppName(name).toLowerCase();
    }

    function appInitials(name) {
      const words = cleanAppName(name).split(" ").filter(Boolean);
      const picked = words.length > 1 ? words.slice(0, 2) : words;
      return picked.map((word) => word.slice(0, 1).toUpperCase()).join("") || "A";
    }

    function appIconToken(name) {
      const colors = ["teal", "purple", "blue", "terracotta", "green", "slate"];
      let hash = 0;
      for (const char of cleanAppName(name)) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
      return colors[hash % colors.length];
    }

    function launcherIconKind(name) {
      const key = appKey(name);
      if (["apps", "settings", "approvals", "delegation"].includes(key)) return key;
      return "generated";
    }

    function moduleIconToken(moduleName, displayName) {
      const key = String(moduleName || "").toLowerCase();
      const tokens = {
        sale_management: "sales",
        pos_restaurant: "sales",
        account: "accounting",
        crm: "sales",
        website: "website",
        stock: "inventory",
        accountant: "accounting",
        equity: "accounting",
        purchase: "inventory",
        point_of_sale: "sales",
        project: "services",
        website_sale: "website",
        mrp: "inventory",
        mass_mailing: "marketing",
        timesheet_grid: "services",
        hr_expense: "hr",
        web_studio: "customizations",
        documents: "productivity",
        hr_holidays: "hr",
        hr_recruitment: "hr",
        hr: "hr",
        ai: "technical",
        data_recycle: "technical",
        databases: "administration"
      };
      return tokens[key] || appIconToken(displayName || moduleName);
    }

    function setLegacyAppSearchActive(active) {
      const wrap = document.querySelector("#appsView .o_home_menu_search");
      const input = document.getElementById("appSearch");
      const grid = document.getElementById("appGrid");
      if (!wrap || !input) return;
      wrap.classList.toggle("is-active", active);
      wrap.dataset.searchActive = active ? "true" : "false";
      input.className = active ? "o_app_search_input" : "o_app_search_stub o_search_hidden visually-hidden";
      input.setAttribute("aria-expanded", active && grid && grid.querySelector(".o_app") ? "true" : "false");
    }

    function handleLegacyLauncherKeydown(event) {
      if (document.body.dataset.view !== "apps") return;
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      const tag = (event.target && event.target.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select") return;
      if (event.key === "Escape") {
        const input = document.getElementById("appSearch");
        if (input) {
          input.value = "";
          input.blur();
          renderApps(workbench.menus);
        }
        setLegacyAppSearchActive(false);
        return;
      }
      if (event.key.length !== 1 || !/\S/.test(event.key)) return;
      const input = document.getElementById("appSearch");
      if (!input) return;
      event.preventDefault();
      setLegacyAppSearchActive(true);
      input.focus();
      input.value += event.key;
      input.dispatchEvent(new Event("input", {bubbles: true}));
    }

    function appSearchText(menu) {
      const parts = [];
      collectMenuSearchText(menu, parts, new Set());
      return parts.join(" ").toLowerCase();
    }

    function collectMenuSearchText(menu, parts, seen) {
      if (!menu || seen.has(menu.id)) return;
      seen.add(menu.id);
      if (menu.name) parts.push(cleanAppName(menu.name));
      for (const childID of menu.children || []) {
        collectMenuSearchText(menuEntry(childID), parts, seen);
      }
    }

    function menuPath(menu) {
      const names = [];
      let current = menu;
      const seen = new Set();
      while (current && current.id && !seen.has(current.id)) {
        seen.add(current.id);
        if (current.name) names.unshift(cleanAppName(current.name));
        current = menuEntry(current.parent_id);
      }
      return names.join(" / ");
    }

    function allMenuEntries(payload) {
      const entries = [];
      const ids = Array.isArray(payload && payload.all_menu_ids) ? payload.all_menu_ids : Object.keys(payload || {});
      const seen = new Set();
      for (const rawID of ids) {
        const menu = menuEntry(rawID);
        if (!menu || seen.has(menu.id)) continue;
        seen.add(menu.id);
        entries.push(menu);
      }
      return entries.sort((left, right) => {
        const leftPath = menuPath(left);
        const rightPath = menuPath(right);
        return leftPath.localeCompare(rightPath);
      });
    }

    function matchingActionMenus(payload, needle) {
      if (!needle) return [];
      return allMenuEntries(payload).filter((menu) => {
        if (!menuHasDirectAction(menu)) return false;
        return menuPath(menu).toLowerCase().includes(needle);
      });
    }

	    function menuHasDirectAction(menu) {
	      return Boolean(menu && (menu.hasDirectAction || menu.directActionID));
	    }

	    function appsCatalogMenu(payload) {
	      return allMenuEntries(payload).find((menu) => {
	        if (!menuHasDirectAction(menu)) return false;
	        const name = cleanAppName(menu.name).toLowerCase();
	        const xmlid = String(menu.xmlid || "").toLowerCase();
	        const actionPath = String(menu.actionPath || menu.action_path || "").toLowerCase();
	        return xmlid === "base.menu_ir_module_module" || actionPath === "apps" || name === "apps";
	      }) || null;
	    }

	    function findActionMenu(names) {
	      const lowered = names.map((name) => cleanAppName(name).toLowerCase());
	      return allMenuEntries(workbench.menus).find((menu) => {
	        if (!menuHasDirectAction(menu)) return false;
	        const name = cleanAppName(menu.name).toLowerCase();
        return lowered.includes(name);
      }) || null;
    }

    function renderSettingsView(action) {
      const host = document.getElementById("settingsBlocks");
      if (!host) return;
      host.replaceChildren();
      const needle = ((document.getElementById("settingsSearch") || {}).value || "").toLowerCase();
      const sections = [
        {
          title: "Users & Companies",
          entries: [
            {label: "Users", names: ["Users"], actionLabel: "Manage Users"},
            {label: "Groups", names: ["Groups"], actionLabel: "Manage Groups"},
            {label: "Companies", names: ["Companies"], actionLabel: "Manage Companies"}
          ]
        },
        {
          title: "Technical",
          entries: [
            {label: "Server Actions", names: ["Server Actions"], actionLabel: "Server Actions"},
            {label: "Automated Actions", names: ["Automated Actions", "Automation Rules"], actionLabel: "Automation Rules"},
            {label: "Scheduled Actions", names: ["Scheduled Actions"], actionLabel: "Scheduled Actions"},
            {label: "Views", names: ["Views"], actionLabel: "Views"},
            {label: "Models", names: ["Models"], actionLabel: "Models"},
            {label: "Access Rights", names: ["Access Rights"], actionLabel: "Access Rights"},
            {label: "Record Rules", names: ["Record Rules"], actionLabel: "Record Rules"},
            {label: "Outgoing Mail Servers", names: ["Outgoing Mail Servers", "Mail Servers"], actionLabel: "Outgoing Mail Servers"},
            {label: "Email Templates", names: ["Email Templates"], actionLabel: "Email Templates"}
          ]
        },
        {
          title: "Apps",
          entries: [
            {label: "Apps", names: ["Apps", "Modules"], actionLabel: "Apps"},
            {label: "AI", names: ["Apps", "Modules"], actionLabel: "AI Apps"}
          ]
        }
      ];
      let visibleCount = 0;
      for (const section of sections) {
        const block = document.createElement("section");
        block.className = "app_settings_block o_settings_block";
        block.dataset.string = section.title;
        const heading = document.createElement("h3");
        heading.className = "o_settings_block_title";
        heading.textContent = section.title;
        const grid = document.createElement("div");
        grid.className = "o_setting_grid";
        for (const entry of section.entries) {
          const menu = findActionMenu(entry.names);
          const searchText = (section.title + " " + entry.label + " " + entry.names.join(" ")).toLowerCase();
          if (needle && !searchText.includes(needle)) continue;
          const box = document.createElement("div");
          box.className = "o_setting_box";
          box.dataset.setting = appKey(entry.label);
          const left = document.createElement("div");
          left.className = "o_setting_left_pane";
          const right = document.createElement("div");
          right.className = "o_setting_right_pane";
          const label = document.createElement("span");
          label.className = "o_form_label";
          label.textContent = entry.label;
          const description = document.createElement("span");
          description.className = "text-muted";
          description.textContent = menu ? menuPath(menu) : "Not available";
          const button = document.createElement("button");
          button.type = "button";
          button.className = "o_setting_action o_setting_link";
          button.textContent = menu ? (entry.actionLabel || entry.label) : "Unavailable";
          button.disabled = !menu;
          if (menu) {
            button.dataset.menuId = String(menu.id);
            if (menu.xmlid) button.dataset.menuXmlid = menu.xmlid;
            button.addEventListener("click", () => openMenu(menu.id));
          }
          right.append(label, description, button);
          box.append(left, right);
          grid.append(box);
          visibleCount++;
        }
        if (grid.children.length) {
          block.append(heading, grid);
          host.append(block);
        }
      }
      if (!host.children.length) {
        const empty = document.createElement("div");
        empty.className = "o_view_nocontent";
        empty.textContent = "No settings found";
        host.append(empty);
      }
      const pager = document.getElementById("settingsPager");
      if (pager) pager.textContent = visibleCount ? "1-" + visibleCount + " / " + visibleCount : "0 / 0";
    }

    function normalizedApps(payload) {
      const apps = [];
      const seen = new Map();
      let sequence = 0;
      for (const id of menuRootIDs(payload)) {
        const menu = menuEntry(id);
        if (!menu) continue;
        const name = cleanAppName(menu.name);
        const key = appKey(name);
        const candidate = {id: menu.id, menu, name, key, sequence: sequence++, searchText: appSearchText(menu)};
        const existing = seen.get(key);
        if (!existing) {
          seen.set(key, candidate);
          apps.push(candidate);
          continue;
        }
        const existingChildCount = ((existing.menu && existing.menu.children) || []).length;
        const nextChildCount = (menu.children || []).length;
        if ((!existing.menu.actionID && menu.actionID) || nextChildCount > existingChildCount) {
          seen.set(key, candidate);
          const index = apps.findIndex((app) => app.key === key);
          if (index >= 0) apps[index] = candidate;
        }
      }
      return apps.sort((left, right) => left.sequence - right.sequence);
    }

    function renderApps(payload) {
      workbench.menus = payload || {};
      const grid = document.getElementById("appGrid");
      const needle = ((document.getElementById("appSearch") || {}).value || "").toLowerCase();
      let found = 0;
      grid.replaceChildren();
      function appendAppCard(app, clickHandler) {
        const name = cleanAppName(app.name);
        const wrapper = document.createElement("div");
        wrapper.className = "col-3 col-md-2 o_draggable mb-3 px-0";
        const button = document.createElement("a");
        button.className = "o_app o_menuitem has-icon d-flex flex-column rounded-3 justify-content-start align-items-center w-100 p-1 p-md-2";
        button.setAttribute("role", "option");
        button.setAttribute("aria-selected", "false");
        const menuID = app.menu && app.menu.id ? app.menu.id : app.id;
        button.setAttribute("href", menuID ? "#menu_id=" + encodeURIComponent(String(menuID)) : "#");
        button.dataset.appName = name;
        button.dataset.appKey = app.key || appKey(name);
        button.dataset.iconKind = launcherIconKind(name);
        if (menuID) button.dataset.menuId = String(menuID);
        if (app.menu && app.menu.xmlid) button.dataset.menuXmlid = app.menu.xmlid;
        button.innerHTML = '<span class="o_app_icon" aria-hidden="true"></span><strong class="o_app_name"></strong>';
        const icon = button.querySelector(".o_app_icon");
        icon.dataset.iconToken = appIconToken(name);
        icon.dataset.iconKind = launcherIconKind(name);
        button.querySelector("strong").textContent = name;
        if (app.subtitle) {
          const path = document.createElement("small");
          path.className = "o_app_menu_path";
          path.textContent = app.subtitle;
          button.append(path);
        }
        button.addEventListener("click", (event) => {
          event.preventDefault();
          clickHandler();
        });
        wrapper.append(button);
        grid.append(wrapper);
        found++;
      }
      const apps = normalizedApps(payload);
      for (const app of apps) {
        if (needle && !app.searchText.includes(needle)) continue;
        appendAppCard(app, () => openMenu(app.menu.id));
      }
      if (needle) {
        for (const menu of matchingActionMenus(payload, needle).slice(0, 24)) {
          const path = menuPath(menu);
          appendAppCard({
            name: menu.name || path,
            key: "menu-" + menu.id,
            id: menu.id,
            menu,
            initials: appInitials(menu.name || path),
            subtitle: path
	          }, () => openMenu(menu.id));
	        }
	      }
	      const catalogMenu = appsCatalogMenu(payload);
	      if (catalogMenu && (!needle || "apps applications modules install".includes(needle)) && !apps.some((app) => app.key === "apps")) {
	        appendAppCard({name: "Apps", key: "apps", id: catalogMenu.id, menu: catalogMenu, initials: "A"}, () => openMenu(catalogMenu.id));
	      }
      if (!found) {
        const empty = document.createElement("p");
        empty.className = "muted";
        empty.textContent = needle ? "No apps found." : "No menus loaded.";
        grid.append(empty);
      }
      document.getElementById("menuStatus").textContent = found ? "" : "No menus loaded.";
    }

    async function openMenu(menuID, options) {
      options = options || {};
      const menu = menuEntry(menuID);
      if (!menu) return;
      workbench.currentMenuID = menu.id;
      document.getElementById("menuStatus").textContent = menu.name;
      const navigationMenu = navigationMenuFor(menu);
      renderSidebarMenu(navigationMenu);
      const list = document.getElementById("menuList");
      list.replaceChildren();
      function appendMenuButton(childID, depth) {
        const child = menuEntry(childID);
        if (!child) return;
        const button = document.createElement("button");
        button.type = "button";
        button.className = menuHasDirectAction(child) ? "o_menuitem" : "secondary o_menu_section";
        button.textContent = menuDisplayName(child);
        button.dataset.menuId = String(child.id);
        if (child.xmlid) button.dataset.menuXmlid = child.xmlid;
        button.style.marginLeft = Math.min(depth, 4) * 12 + "px";
        button.addEventListener("click", () => openMenu(child.id));
        list.append(button);
        for (const grandchildID of child.children || []) {
          appendMenuButton(grandchildID, depth + 1);
        }
      }
      for (const childID of navigationMenu.children || []) {
        appendMenuButton(childID, 0);
      }
      if (menuHasDirectAction(menu)) {
        await openAction(menu.actionID, {menuID: menu.id, noRoute: options.noRoute});
      } else if ((menu.children || []).length) {
        setView("apps");
        if (!options.noRoute) writeRouteState({menu_id: menu.id}, false);
      }
    }

    async function openAction(actionID, options) {
      options = options || {};
      if (options.menuID) workbench.currentMenuID = options.menuID;
      try {
        const action = await requestJSON("/web/action/load?id=" + encodeURIComponent(actionID));
        const model = action.res_model || "";
        document.getElementById("menuStatus").textContent = (action.name || "Action") + (model ? " / " + model : "");
        if (model) {
          document.querySelector("#recordsView .o-control-panel h2").textContent = action.name || model;
          workbench.action = action;
          workbench.openedRecord = null;
          workbench.searchFacets = [];
          document.getElementById("recordSearch").value = "";
          if (model === "res.config.settings") {
            renderSettingsView(action);
            setView("settings");
            if (!options.noRoute) writeRouteState(currentRouteState({action: action.id || actionID, model, view_type: "form", id: null, menu_id: options.menuID || workbench.currentMenuID}), false);
            return;
          }
          ensureModelOption(model);
          modelSelect.value = model;
          if (action.limit) document.getElementById("limit").value = String(action.limit);
          showRecordForm(false);
          await loadActionViews(action, model);
          renderSearchFacets();
          renderSearchMenu();
          await loadRows();
          setView("records");
          if (!options.noRoute) writeRouteState(currentRouteState({action: action.id || actionID, model, view_type: workbench.activeView, id: null, menu_id: options.menuID || workbench.currentMenuID}), false);
        }
      } catch (error) {
        if (await ensureSession(error)) return openAction(actionID, options);
        document.getElementById("menuStatus").textContent = "Action error: " + error.message;
      }
    }

    async function loadInstallApps() {
      const grid = document.getElementById("moduleGrid");
      const needle = (document.getElementById("moduleSearch").value || "").toLowerCase();
      grid.textContent = "Loading...";
      try {
        const rows = await callKW("ir.module.module", "search_read", {args: [[]], kwargs: {fields: ["id", "name", "state"], limit: 200, order: "name"}});
        grid.replaceChildren();
        const seenModules = new Set();
        for (const row of rows) {
          const name = String(row.name || "");
          const displayName = moduleDisplayName(name);
          const key = appKey(displayName);
          if (seenModules.has(key)) continue;
          seenModules.add(key);
          if (needle && !(name.toLowerCase().includes(needle) || displayName.toLowerCase().includes(needle))) continue;
          const card = document.createElement("div");
          card.className = "module-card o_app";
          card.dataset.appName = displayName;
          const icon = document.createElement("span");
          icon.className = "app-icon o_app_icon";
          icon.dataset.iconToken = moduleIconToken(name, displayName);
          icon.dataset.iconKind = moduleIconToken(name, displayName);
          icon.dataset.initials = appInitials(displayName);
          icon.setAttribute("aria-hidden", "true");
          const title = document.createElement("strong");
          title.className = "o_app_name";
          title.textContent = displayName || "Module";
          const state = document.createElement("span");
          state.className = "badge";
          state.textContent = row.state || "unknown";
          const button = document.createElement("button");
          button.type = "button";
          button.textContent = row.state === "installed" ? "Installed" : "Install";
          button.disabled = row.state === "installed";
          button.className = row.state === "installed" ? "secondary" : "";
          button.addEventListener("click", () => installModule(row.id));
          card.append(icon, title, state, button);
          grid.append(card);
        }
        if (!grid.children.length) {
          const empty = document.createElement("p");
          empty.className = "muted";
          empty.textContent = "No apps found.";
          grid.append(empty);
        }
        const pager = document.getElementById("modulePager");
        if (pager) pager.textContent = grid.children.length ? "1-" + grid.children.length + " / " + grid.children.length : "0 / 0";
      } catch (error) {
        if (await ensureSession(error)) return loadInstallApps();
        grid.textContent = "Apps error: " + error.message;
      }
    }

    function moduleDisplayName(name) {
      const cleaned = String(name || "").replace(/^oi_/, "").replace(/^base_/, "").replace(/_/g, " ").replace(/\s+/g, " ").trim();
      return cleaned ? cleaned.split(" ").map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(" ") : "Module";
    }

    async function installModule(id) {
      await callKW("ir.module.module", "button_immediate_install", {args: [[id]]});
      await loadInstallApps();
    }

    function fieldMeta(field) {
      return workbench.fieldMeta[field] || {};
    }

    function fieldType(field) {
      return fieldMeta(field).type || "";
    }

    function fieldRelation(field) {
      return fieldMeta(field).relation || "";
    }

    function many2OneDisplay(value) {
      if (Array.isArray(value)) return {id: value[0], name: String(value[1] || value[0] || "")};
      if (value && typeof value === "object") return {id: value.id, name: String(value.display_name || value.name || value.id || "")};
      return {id: typeof value === "number" ? value : null, name: value === null || value === undefined || value === false ? "" : String(value)};
    }

    function fieldDisplayValue(field, value) {
      if (fieldType(field) === "many2one" || fieldType(field) === "reference") return many2OneDisplay(value).name;
      if (fieldType(field) === "selection" && Array.isArray(fieldMeta(field).selection)) {
        const key = String(value || "");
        for (const option of fieldMeta(field).selection) {
          if (Array.isArray(option) && String(option[0]) === key) return String(option[1] || option[0]);
          if (option && typeof option === "object" && String(option.value) === key) return String(option.label || option.string || option.value);
        }
      }
      if (Array.isArray(value)) return value.map((item) => fieldDisplayValue(field, item)).filter(Boolean).join(", ");
      if (value === null || value === undefined || value === false) return "";
      if (typeof value === "object") return JSON.stringify(value);
      return String(value);
    }

    function decorationTruthy(expression, row) {
      const source = String(expression || "").trim();
      if (!source) return false;
      if (source === "1" || source === "True" || source === "true") return true;
      if (source === "0" || source === "False" || source === "false") return false;
      if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(source)) return Boolean(row[source]);
      let match = source.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*={1,2}\s*['"]([^'"]+)['"]$/);
      if (match) return String(row[match[1]] || "") === match[2];
      match = source.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*!=\s*['"]([^'"]+)['"]$/);
      if (match) return String(row[match[1]] || "") !== match[2];
      match = source.match(/^([A-Za-z_][A-Za-z0-9_]*)\s+in\s+\(([^)]+)\)$/);
      if (match) {
        const values = match[2].split(",").map((item) => item.trim().replace(/^['"]|['"]$/g, ""));
        return values.includes(String(row[match[1]] || ""));
      }
      return false;
    }

    function activeDecoration(attrs, row) {
      for (const name of ["danger", "warning", "success", "info", "primary", "muted"]) {
        if (decorationTruthy(attrs && attrs["decoration-" + name], row)) return name;
      }
      return "";
    }

    function rowDecorationClass(row) {
      const classes = ["o_data_row"];
      for (const name of ["danger", "warning", "success", "info", "primary", "muted"]) {
        if (!decorationTruthy(workbench.listViewAttrs && workbench.listViewAttrs["decoration-" + name], row)) continue;
        classes.push("text-bg-" + name);
        classes.push("o_list_record_" + name);
      }
      if (decorationTruthy(workbench.listViewAttrs && workbench.listViewAttrs["decoration-bf"], row)) classes.push("fw-bold");
      if (decorationTruthy(workbench.listViewAttrs && workbench.listViewAttrs["decoration-it"], row)) classes.push("fst-italic");
      return classes.join(" ");
    }

    function renderFieldValue(field, value, row, attrs) {
      const widget = attrs && attrs.widget;
      if (widget === "many2one_avatar_employee" && fieldType(field) === "many2one") {
        const data = many2OneDisplay(value);
        const root = document.createElement("span");
        root.className = "o_field_widget o_field_many2one_avatar";
        root.dataset.field = field;
        root.dataset.relation = fieldRelation(field) || "hr.employee";
        if (data.id) {
          root.dataset.resId = String(data.id);
          const image = document.createElement("img");
          image.className = "o_avatar o_m2o_avatar";
          image.src = "/web/image/" + root.dataset.relation + "/" + encodeURIComponent(String(data.id)) + "/avatar_128";
          image.alt = data.name;
          root.append(image);
        }
        const label = document.createElement("span");
        label.className = "o_field_many2one_avatar_name";
        label.textContent = data.name;
        root.append(label);
        return root;
      }
      if (widget === "badge" || widget === "selection_badge") {
        const badge = document.createElement("span");
        const decoration = activeDecoration(attrs || {}, row || {});
        badge.className = "badge rounded-pill " + (decoration && decoration !== "muted" ? "text-bg-" + decoration : "text-bg-300");
        badge.dataset.field = field;
        badge.dataset.widget = widget;
        if (decoration) badge.dataset.decoration = decoration;
        badge.textContent = fieldDisplayValue(field, value);
        return badge;
      }
      const out = document.createElement("span");
      out.textContent = fieldDisplayValue(field, value);
      return out;
    }

    function viewHasChatter() {
      const formArch = (((workbench.viewInfo || {}).views || {}).form || {}).arch || "";
      return /<chatter(?:\s|\/|>)/.test(formArch);
    }

    function renderChatter(model, id) {
      const chatter = document.createElement("aside");
      chatter.className = "o-mail-ChatterContainer o-mail-Form-chatter o-mail-Chatter";
      chatter.dataset.threadModel = model;
      chatter.dataset.threadId = String(id || "");
      const header = document.createElement("div");
      header.textContent = "Chatter";
      const composer = document.createElement("div");
      composer.className = "o-mail-Composer";
      for (const label of ["Send message", "Log note", "Activities"]) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "secondary";
        button.dataset.chatterAction = label.toLowerCase().replace(/\s+/g, "-");
        button.textContent = label;
        composer.append(button);
      }
      const thread = document.createElement("div");
      thread.className = "o-mail-Thread";
      thread.dataset.chatterThread = "true";
      chatter.append(header, composer, thread);
      if (id) loadChatterThread(thread, model, id);
      return chatter;
    }

    async function loadChatterThread(thread, model, id) {
      thread.textContent = "Loading...";
      try {
        const payload = await requestJSON("/mail/chatter_fetch", {
          method: "POST",
          body: JSON.stringify({thread_model: model, thread_id: Number(id), fetch_params: {limit: 30}})
        });
        renderChatterThread(thread, payload);
      } catch (_error) {
        thread.textContent = "Chatter unavailable";
      }
    }

    function renderChatterThread(thread, payload) {
      thread.replaceChildren();
      const messages = chatterMessages(payload);
      if (!messages.length) {
        const empty = document.createElement("p");
        empty.className = "muted";
        empty.textContent = "No messages.";
        thread.append(empty);
        return;
      }
      for (const message of messages) thread.append(renderChatterMessage(message));
    }

    function chatterMessages(payload) {
      const rows = payload && payload.data && Array.isArray(payload.data["mail.message"]) ? payload.data["mail.message"] : [];
      if (!Array.isArray(payload && payload.messages)) return rows;
      const byID = new Map(rows.map((row) => [String(row.id || ""), row]));
      const ordered = payload.messages.map((id) => byID.get(String(id))).filter(Boolean);
      return ordered.length ? ordered : rows;
    }

    function renderChatterMessage(message) {
      const article = document.createElement("article");
      article.className = "o-mail-Message" + (message.is_message_subtype_note ? " o-note" : "");
      if (message.id) article.dataset.messageId = String(message.id);
      const avatarURL = message.author_avatar_url || "";
      if (avatarURL) {
        const avatar = document.createElement("img");
        avatar.className = "o_avatar o-mail-Message-avatar";
        avatar.src = avatarURL;
        avatar.alt = chatterAuthorName(message);
        article.append(avatar);
      } else {
        const spacer = document.createElement("span");
        article.append(spacer);
      }
      const content = document.createElement("div");
      const meta = document.createElement("div");
      meta.className = "o-mail-Message-meta";
      const author = document.createElement("span");
      author.className = "o-mail-Message-author";
      author.textContent = chatterAuthorName(message);
      meta.append(author);
      if (message.published_date_str || message.date) {
        const time = document.createElement("time");
        time.className = "o-mail-Message-date";
        time.textContent = String(message.published_date_str || message.date);
        meta.append(time);
      }
      const body = document.createElement("div");
      body.className = "o-mail-Message-body";
      body.textContent = chatterBodyText(message.body);
      content.append(meta, body);
      const attachments = renderChatterAttachments(message.attachment_ids);
      if (attachments) content.append(attachments);
      const reactions = renderChatterReactions(message.reactions);
      if (reactions) content.append(reactions);
      article.append(content);
      return article;
    }

    function chatterAuthorName(message) {
      const author = message.author_id && typeof message.author_id === "object" ? message.author_id : message.author_guest_id && typeof message.author_guest_id === "object" ? message.author_guest_id : null;
      return String((author && author.name) || message.email_from || "OdooBot");
    }

    function chatterBodyText(value) {
      const body = Array.isArray(value) && value[0] === "markup" ? value[1] : value;
      if (body === null || body === undefined || body === false) return "";
      return String(body).replace(/<br\s*\/?>/gi, "\n").replace(/<\/p>/gi, "\n").replace(/<[^>]+>/g, "").replace(/\n{3,}/g, "\n\n").trim();
    }

    function renderChatterAttachments(value) {
      if (!Array.isArray(value) || !value.length) return null;
      const list = document.createElement("div");
      list.className = "o-mail-AttachmentList";
      for (const item of value) {
        const attachment = item && typeof item === "object" ? item : {id: item};
        const chip = document.createElement("span");
        chip.className = "o-mail-Attachment";
        chip.textContent = String(attachment.filename || attachment.name || attachment.id || "Attachment");
        list.append(chip);
      }
      return list;
    }

    function renderChatterReactions(value) {
      if (!Array.isArray(value) || !value.length) return null;
      const list = document.createElement("div");
      list.className = "o-mail-ReactionList";
      for (const reaction of value) {
        if (!reaction || typeof reaction !== "object") continue;
        const chip = document.createElement("span");
        chip.className = "o-mail-Reaction";
        chip.textContent = String(((reaction.content || "") + " " + (reaction.count || "")).trim());
        list.append(chip);
      }
      return list.children.length ? list : null;
    }

    function renderGroupedRows(groups, groupBy) {
      const host = document.getElementById("rows");
      host.replaceChildren();
      const pager = document.getElementById("recordPager");
      if (!Array.isArray(groups) || !groups.length) {
        const empty = document.createElement("div");
        empty.className = "o_view_nocontent";
        empty.textContent = "No records";
        host.append(empty);
        if (pager) pager.textContent = "0 / 0";
        return;
      }
      const wrapper = document.createElement("div");
      wrapper.className = "o_grouped_list o_list_renderer";
      wrapper.dataset.groupby = groupBy || "";
      let total = 0;
      for (const group of groups) {
        const count = Number(group.__count || group.count || group[groupBy + "_count"] || 0);
        total += count;
        const header = document.createElement("div");
        header.className = "o_group_header";
        header.dataset.groupby = groupBy || "";
        const name = document.createElement("span");
        name.className = "o_group_name";
        name.textContent = fieldDisplayValue(groupBy, group[groupBy]) || "Undefined";
        const countNode = document.createElement("span");
        countNode.className = "o_group_count";
        countNode.textContent = count + " records";
        header.append(name, countNode);
        const groupDomain = Array.isArray(group.__domain) ? group.__domain : (Array.isArray(group.__extra_domain) ? group.__extra_domain : null);
        if (groupDomain) {
          header.addEventListener("click", () => {
            workbench.searchFacets = [
              ...workbench.searchFacets.filter((facet) => facet.id !== "group-domain-" + groupBy),
              {id: "group-domain-" + groupBy, type: "filter", label: name.textContent, domain: groupDomain}
            ];
            workbench.searchFacets = workbench.searchFacets.filter((facet) => facet.type !== "groupBy");
            loadRows();
          });
        }
        wrapper.append(header);
      }
      host.append(wrapper);
      if (pager) pager.textContent = "1-" + groups.length + " / " + groups.length + " groups" + (total ? " (" + total + ")" : "");
    }

    function renderRows(rows, fields) {
      if (workbench.activeView === "kanban") {
        renderKanbanRows(rows, fields);
        return;
      }
      const host = document.getElementById("rows");
      host.replaceChildren();
      if (!Array.isArray(rows) || rows.length === 0) {
        const empty = document.createElement("p");
        empty.className = "o_view_nocontent";
        empty.textContent = "No records";
        host.append(empty);
        const pager = document.getElementById("recordPager");
        if (pager) pager.textContent = "0 / 0";
        return;
      }
      const pager = document.getElementById("recordPager");
      if (pager) pager.textContent = "1-" + rows.length + " / " + rows.length;
      const table = document.createElement("table");
      table.className = "o_list_renderer o_list_table";
      const thead = document.createElement("thead");
      const header = document.createElement("tr");
      for (const field of fields) {
        if (field === "id") continue;
        const th = document.createElement("th");
        th.textContent = fieldLabel(field);
        header.append(th);
      }
      thead.append(header);
      const tbody = document.createElement("tbody");
      for (const row of rows) {
        const tr = document.createElement("tr");
        tr.className = rowDecorationClass(row);
        tr.dataset.id = row.id || "";
        if (row.id) {
          tr.addEventListener("click", () => openRecord(modelSelect.value, row.id));
        }
        for (const field of fields) {
          if (field === "id") continue;
          const td = document.createElement("td");
          td.dataset.field = field;
          td.append(renderFieldValue(field, row[field], row, workbench.listFieldAttrs[field] || {}));
          tr.append(td);
        }
        tbody.append(tr);
      }
      table.append(thead, tbody);
      const mobileCards = document.createElement("div");
      mobileCards.className = "o_mobile_list_cards";
      for (const row of rows) {
        const card = document.createElement("article");
        card.className = "o_mobile_record_card";
        card.dataset.id = row.id || "";
        for (const field of fields) {
          if (field === "id") continue;
          const line = document.createElement("div");
          line.className = "o_mobile_record_line";
          const label = document.createElement("span");
          label.className = "o_mobile_record_label";
          label.textContent = fieldLabel(field);
          const value = document.createElement("span");
          value.className = "o_mobile_record_value";
          value.append(renderFieldValue(field, row[field], row, workbench.listFieldAttrs[field] || {}));
          line.append(label, value);
          card.append(line);
        }
        if (row.id) {
          const openButton = document.createElement("button");
          openButton.type = "button";
          openButton.className = "secondary";
          openButton.textContent = "Open";
          openButton.addEventListener("click", () => openRecord(modelSelect.value, row.id));
          card.append(openButton);
        }
        mobileCards.append(card);
      }
      host.append(table, mobileCards);
    }

    function renderKanbanRows(rows, fields) {
      const host = document.getElementById("rows");
      host.replaceChildren();
      const pager = document.getElementById("recordPager");
      if (!Array.isArray(rows) || rows.length === 0) {
        const empty = document.createElement("div");
        empty.className = "o_kanban_renderer o_renderer o_kanban_ungrouped";
        const message = document.createElement("div");
        message.className = "o_view_nocontent";
        message.textContent = "No records";
        empty.append(message);
        host.append(empty);
        if (pager) pager.textContent = "0 / 0";
        return;
      }
      if (pager) pager.textContent = "1-" + rows.length + " / " + rows.length;
      const renderer = document.createElement("div");
      renderer.className = "o_kanban_renderer o_renderer o_kanban_ungrouped";
      renderer.dataset.model = modelSelect.value;
      const fieldList = fields.filter((field) => field !== "id");
      const titleField = fieldList.includes("display_name") ? "display_name" : (fieldList.includes("name") ? "name" : fieldList[0]);
      for (const row of rows) {
        const card = document.createElement("article");
        card.className = "o_kanban_record oe_kanban_global_click o_kanban_global_click d-flex cursor-pointer o_record_selection_available";
        card.setAttribute("role", "link");
        card.tabIndex = 0;
        card.dataset.id = row.id || "";
        card.dataset.model = modelSelect.value;
        if (row.id) card.addEventListener("click", () => openRecord(modelSelect.value, row.id));
        const details = document.createElement("div");
        details.className = "oe_kanban_details";
        const title = document.createElement("strong");
        title.className = "o_kanban_record_title";
        title.textContent = fieldDisplayValue(titleField, row[titleField] || row.display_name || row.name || row.id || "");
        details.append(title);
        for (const field of fieldList) {
          if (field === titleField) continue;
          const line = document.createElement("div");
          line.className = "o_kanban_record_field";
          line.dataset.field = field;
          const label = document.createElement("span");
          label.className = "o_kanban_field_label";
          label.textContent = fieldLabel(field);
          const value = document.createElement("span");
          value.className = "o_kanban_field_value";
          value.append(renderFieldValue(field, row[field], row, workbench.kanbanFieldAttrs[field] || workbench.listFieldAttrs[field] || {}));
          line.append(label, value);
          details.append(line);
        }
        card.append(details);
        renderer.append(card);
      }
      host.append(renderer);
    }

    async function openRecord(model, id, options) {
      options = options || {};
      const fieldText = document.getElementById("recordFields").value || fieldsInput.value || defaultFields[model] || "id,display_name,name";
      const fields = fieldText.split(",").map((field) => field.trim()).filter(Boolean);
      workbench.openedRecord = {model, id};
      showRecordForm(true);
      setView("records");
      document.getElementById("recordModel").value = model;
      document.getElementById("recordID").value = id;
      document.getElementById("recordFields").value = fields.join(",");
      const actionTitle = document.querySelector("#recordsView > .o-control-panel h2");
      document.getElementById("recordBack").textContent = (actionTitle && actionTitle.textContent.trim()) || "Records";
      document.getElementById("recordTitle").textContent = "Loading";
      const form = document.getElementById("recordForm");
      form.textContent = "Loading...";
      try {
        const rows = await callKW(model, "web_read", {args: [[Number(id)]], kwargs: {specification: fieldSpecification(fields), context: readContext(workbench.action)}});
        const row = rows && rows[0] ? rows[0] : {};
        const formFields = visibleFormFields(fields);
        document.getElementById("recordTitle").textContent = row.display_name || row.name || (model + " / " + id);
        form.replaceChildren();
        for (const field of formFields) {
          const label = document.createElement("label");
          label.textContent = fieldLabel(field);
          const value = row[field];
          const attrs = workbench.formFieldAttrs[field] || {};
          if (attrs.widget === "many2one_avatar_employee" || attrs.widget === "badge" || attrs.widget === "selection_badge") {
            label.append(renderFieldValue(field, value, row, attrs));
          } else {
            const input = document.createElement("input");
            input.dataset.field = field;
            input.value = fieldDisplayValue(field, value);
            if (field === "id" || Array.isArray(value) || typeof value === "object") input.readOnly = true;
            label.append(input);
          }
          form.append(label);
        }
        if (viewHasChatter()) form.append(renderChatter(model, id));
        document.getElementById("recordRaw").textContent = pretty(row);
        if (!options.noRoute) writeRouteState(currentRouteState({model, view_type: "form", id}), false);
      } catch (error) {
        if (await ensureSession(error)) return openRecord(model, id, options);
        form.textContent = "Record error: " + error.message;
      }
    }

    async function saveRecord() {
      if (!workbench.openedRecord) return;
      const values = {};
      for (const input of document.querySelectorAll("#recordForm input[data-field]")) {
        if (input.readOnly) continue;
        values[input.dataset.field] = input.value;
      }
      await callKW(workbench.openedRecord.model, "web_save", {
        args: [[Number(workbench.openedRecord.id)], values],
        kwargs: {specification: fieldSpecification((document.getElementById("recordFields").value || "").split(",").map((field) => field.trim()).filter(Boolean)), context: readContext(workbench.action)}
      });
      await openRecord(workbench.openedRecord.model, workbench.openedRecord.id);
      await loadRows();
    }

    function sessionCompanyName(session) {
      const companies = session && session.user_companies;
      if (companies && companies.allowed_companies) {
        const current = String(companies.current_company || session.company_id || "");
        const allowed = companies.allowed_companies;
        const row = allowed[current] || allowed[Number(current)];
        if (row && row.name) return String(row.name);
      }
      return "My Company";
    }

    function updateSystray(session) {
      const userName = session.name || session.username || ("uid " + session.uid);
      const userButton = document.getElementById("topUser");
      userButton.innerHTML = "";
      const userLabel = document.createElement("span");
      userLabel.className = "oe_topbar_name";
      userLabel.textContent = userName;
      userButton.append(userLabel);
      document.getElementById("topCompany").textContent = sessionCompanyName(session);
      const debug = Boolean(session.debug || (session.bundle_params && session.bundle_params.debug));
      document.getElementById("debugIndicator").hidden = !debug;
      document.getElementById("messageCounter").textContent = "0";
      document.getElementById("activityCounter").textContent = "0";
    }

    async function loadRuntime() {
      try {
        const health = await requestJSON("/web/health");
        setText("health", health.status || "ok");
        document.getElementById("health").className = "status-ok";
      } catch (error) {
        setText("health", "error");
        document.getElementById("health").className = "status-error";
      }
      const session = await requestJSON("/web/session/info");
      setText("user", session.name || session.username || ("uid " + session.uid));
      updateSystray(session);
      let menus;
      try {
        menus = await requestJSON("/web/webclient/load_menus");
      } catch (error) {
        if (await ensureSession(error)) return;
        setText("menuCount", "auth");
        runtimeStatus.textContent = "Login required.";
        runtimeStatus.className = "status-error";
        return;
      }
      const ids = Array.isArray(menus && menus.all_menu_ids) ? menus.all_menu_ids : Object.keys((menus && menus.children) || {});
      setText("menuCount", String(ids.length));
      renderApps(menus);
      runtimeStatus.textContent = "";
      runtimeStatus.className = "status-ok";
    }

    const tsWebClientOwnsPage = Boolean(globalThis.__goerpTSWebClientAvailable);
    buildWorkbenchPanels();
    setView("apps");
    document.getElementById("loadRows").addEventListener("click", loadRows);
    document.getElementById("createPartner").addEventListener("click", createPartner);
    document.getElementById("recordSearchDropdown").addEventListener("click", (event) => {
      event.stopPropagation();
      toggleSearchMenu();
    });
    document.getElementById("recordSearchRoot").addEventListener("click", (event) => event.stopPropagation());
    document.addEventListener("click", closeSearchMenu);
    for (const button of document.querySelectorAll(".o_cp_switch_buttons .o_switch_view")) {
      button.addEventListener("click", async () => {
        if (button.classList.contains("o_form")) {
          const first = document.querySelector("#rows tr[data-id], #rows .o_kanban_record[data-id]");
          workbench.activeView = "form";
          updateViewSwitchButtons();
          if (first && first.dataset.id) await openRecord(modelSelect.value, first.dataset.id);
          return;
        }
        workbench.activeView = button.classList.contains("o_kanban") ? "kanban" : "list";
        updateViewSwitchButtons();
        await loadRows();
        writeRouteState(currentRouteState({view_type: workbench.activeView, id: null}), false);
      });
    }
    document.getElementById("recordSearch").addEventListener("keydown", (event) => {
      if (event.key === "Enter") searchRows(event.currentTarget.value);
    });
    document.getElementById("loginButton").addEventListener("click", login);
    window.addEventListener("popstate", () => {
      if (tsWebClientOwnsPage) return;
      restoreRouteFromHash().catch((error) => {
        runtimeStatus.textContent = "Route error: " + error.message;
        runtimeStatus.className = "status-error";
      });
    });
    window.addEventListener("hashchange", () => {
      if (tsWebClientOwnsPage) return;
      restoreRouteFromHash().catch((error) => {
        runtimeStatus.textContent = "Route error: " + error.message;
        runtimeStatus.className = "status-error";
      });
    });
    renderSearchFacets();
    renderSearchMenu();
    if (!tsWebClientOwnsPage) {
      loadRuntime().then(async () => {
        if (!(await restoreRouteFromHash())) await loadRows();
      }).catch((error) => {
        runtimeStatus.textContent = "Startup error: " + error.message;
        runtimeStatus.className = "status-error";
      });
    }
  </script>
</body>
</html>`

func (s Server) sessionInfo(w http.ResponseWriter, r *http.Request) {
	s.writeMaybeRPC(w, r, s.sessionInfoPayload(r))
}

func (s Server) authenticate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DB       string `json:"db"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if s.Security == nil {
		writeRPCOrJSON(w, envelope, s.sessionInfoPayload(r))
		return
	}
	user, ok := s.authenticateSecurityUser(req.Login, req.Password)
	if !ok {
		writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("invalid login or password"))
		return
	}
	token, err := newSessionToken()
	if err != nil {
		writeRPCError(w, envelope, http.StatusInternalServerError, err)
		return
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	s.Security.IssueSession(user.ID, token, expiresAt)
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeRPCOrJSON(w, envelope, s.sessionPayloadForEnv(s.envForSecurityUser(user)))
}

func (s Server) sessionCheck(w http.ResponseWriter, r *http.Request) {
	env, ok := s.authenticatedRequestEnv(r)
	if !ok {
		s.writeMaybeRPC(w, r, map[string]any{"uid": false, "ok": false})
		return
	}
	s.writeMaybeRPC(w, r, map[string]any{"uid": env.Context().UserID, "ok": true})
}

func (s Server) switchCompany(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CompanyID         any `json:"company_id"`
		CompanyIDs        any `json:"company_ids"`
		AllowedCompanyIDs any `json:"allowed_company_ids"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	companyID := int64Value(req.CompanyID)
	if companyID == 0 {
		writeRPCError(w, envelope, http.StatusBadRequest, errors.New("company_id is required"))
		return
	}
	requestedCompanyIDs := uniqueInt64Slice(int64Slice(firstNonNil(req.CompanyIDs, req.AllowedCompanyIDs)))
	if s.Security == nil {
		env := s.effectiveEnv(r)
		allowed := companyIDs(env.Context())
		if len(allowed) == 0 && env.Context().CompanyID != 0 {
			allowed = []int64{env.Context().CompanyID}
		}
		companyIDs, err := validatedSwitchCompanyIDs(allowed, companyID, requestedCompanyIDs)
		if err != nil {
			writeRPCError(w, envelope, http.StatusForbidden, err)
			return
		}
		setCompanyIDsCookie(w, companyIDs)
		writeRPCOrJSON(w, envelope, s.sessionPayloadForEnv(envWithCompanyContext(env, companyID, companyIDs)))
		return
	}
	sessionID := cookieSessionID(r)
	if sessionID == "" {
		writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	userID, ok := s.Security.AuthenticateSession(sessionID)
	if !ok {
		writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	user, ok := s.Security.Users[userID]
	if !ok || !user.Active {
		writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	allowed := allowedCompanyIDsForUser(user)
	companyIDs, err := validatedSwitchCompanyIDs(allowed, companyID, requestedCompanyIDs)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	if !s.Security.SetSessionCompanies(sessionID, companyID, companyIDs) {
		writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("authentication required"))
		return
	}
	setCompanyIDsCookie(w, companyIDs)
	writeRPCOrJSON(w, envelope, s.sessionPayloadForEnv(envWithCompanyContext(s.envForSecurityUser(user), companyID, companyIDs)))
}

func (s Server) sessionModules(w http.ResponseWriter, r *http.Request) {
	env := s.Env
	if s.Security != nil {
		sessionEnv, ok := s.requireWebSession(w, r, nil)
		if !ok {
			return
		}
		env = sessionEnv
	}
	s.writeMaybeRPC(w, r, modulesPayloadFromEnv(env, s.Modules))
}

func (s Server) sessionDestroy(w http.ResponseWriter, r *http.Request) {
	s.revokeWebSession(w, r)
	s.writeMaybeRPC(w, r, true)
}

func (s Server) logout(w http.ResponseWriter, r *http.Request) {
	s.revokeWebSession(w, r)
	s.writeMaybeRPC(w, r, map[string]any{"ok": true})
}

func (s Server) loginAs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Impersonation == nil {
		http.NotFound(w, r)
		return
	}
	sessionID, actorID, ok := s.loginAsSessionActor(w, r)
	if !ok {
		return
	}
	targetID, err := parseLoginAsTargetID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auditStart := s.loginAsAuditLen()
	_, err = s.Impersonation.Start(sessionID, actorID, targetID, impersonation.SwitchOptions{
		GroupID:  int64Value(r.URL.Query().Get("group_id")),
		ReturnTo: safeWebRedirect(r.URL.Query().Get("redirect")),
		Reason:   r.URL.Query().Get("reason"),
	})
	if auditErr := s.persistLoginAsAuditSince(auditStart, r); auditErr != nil {
		http.Error(w, auditErr.Error(), http.StatusInternalServerError)
		return
	}
	if err != nil {
		writeLoginAsError(w, err)
		return
	}
	http.Redirect(w, r, safeWebRedirect(r.URL.Query().Get("redirect")), http.StatusFound)
}

func (s Server) loginBack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Impersonation == nil {
		http.NotFound(w, r)
		return
	}
	sessionID, _, ok := s.loginAsSessionActor(w, r)
	if !ok {
		return
	}
	auditStart := s.loginAsAuditLen()
	if _, err := s.Impersonation.LoginBack(sessionID); err != nil {
		if auditErr := s.persistLoginAsAuditSince(auditStart, r); auditErr != nil {
			http.Error(w, auditErr.Error(), http.StatusInternalServerError)
			return
		}
		writeLoginAsError(w, err)
		return
	}
	if auditErr := s.persistLoginAsAuditSince(auditStart, r); auditErr != nil {
		http.Error(w, auditErr.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, safeWebRedirect(r.URL.Query().Get("redirect")), http.StatusFound)
}

func (s Server) loginAsDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Impersonation == nil {
		http.NotFound(w, r)
		return
	}
	sessionID, actorID, ok := s.loginAsSessionActor(w, r)
	if !ok {
		return
	}
	redirect := safeDebugRedirect(r.URL.Query().Get("redirect"))
	if s.Impersonation.IsSystemUser(actorID) {
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}
	auditStart := s.loginAsAuditLen()
	_, err := s.Impersonation.SwitchToSystem(sessionID, actorID, impersonation.SwitchOptions{
		ReturnTo: redirect,
		Reason:   r.URL.Query().Get("reason"),
	})
	if auditErr := s.persistLoginAsAuditSince(auditStart, r); auditErr != nil {
		http.Error(w, auditErr.Error(), http.StatusInternalServerError)
		return
	}
	if err != nil {
		writeLoginAsError(w, err)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s Server) callKW(w http.ResponseWriter, r *http.Request) {
	req, envelope, err := decodeCallKW(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	applyPathModelMethod(r.URL.Path, &req)
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	result, err := s.executeCallKW(env, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeRPCOrJSON(w, envelope, result)
}

func (s Server) callButton(w http.ResponseWriter, r *http.Request) {
	req, envelope, err := decodeCallKW(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	applyPathModelMethod(r.URL.Path, &req)
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	if s.Workflow != nil {
		result, handled, err := s.Workflow.DispatchCall(r.Context(), env, internalworkflow.DispatchRequest{
			Model:  req.Model,
			Method: req.Method,
			Args:   req.Args,
			Kwargs: req.Kwargs,
			Values: req.Values,
		})
		if handled {
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			cleaned, err := s.cleanActionResultWithError(env, result)
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			writeRPCOrJSON(w, envelope, cleaned)
			return
		}
	}
	if result, handled, err := s.dispatchAccountingButton(env, req); handled {
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		cleaned, err := s.cleanActionResultWithError(env, result)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		writeRPCOrJSON(w, envelope, cleaned)
		return
	}
	result, err := s.executeCallKW(env, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	cleaned, err := s.cleanActionResultWithError(env, result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeRPCOrJSON(w, envelope, cleaned)
}

func (s Server) dispatchAccountingButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	switch req.Model {
	case "account.move":
		return s.dispatchMovePaymentButton(env, req)
	case "account.move.line":
		return s.dispatchMoveLinePaymentButton(env, req)
	case "account.lock_exception":
		return s.dispatchAccountLockExceptionButton(env, req)
	case "account.move.reversal":
		return s.dispatchMoveReversalButton(env, req)
	case "account.payment.register":
		return s.dispatchPaymentRegisterButton(env, req)
	case "account.move.send.wizard":
		return s.dispatchMoveSendWizardButton(env, req)
	case "account.move.send.batch.wizard":
		return s.dispatchMoveSendBatchWizardButton(env, req)
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchAccountLockExceptionButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Method != "action_revoke" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(ids) == 0 {
		return nil, true, fmt.Errorf("%s requires lock exception ids", req.Method)
	}
	if err := env.Model("account.lock_exception").Browse(ids...).RevokeAccountLockExceptions(canManageAccountLockExceptions(env)); err != nil {
		return nil, true, err
	}
	return true, true, nil
}

func canManageAccountLockExceptions(env *record.Env) bool {
	if env == nil {
		return false
	}
	ctx := env.Context()
	if ctx.UserID == 1 {
		return true
	}
	groupIDs := map[int64]bool{}
	for _, groupID := range int64Slice(ctx.Values["group_ids"]) {
		groupIDs[groupID] = true
	}
	for _, groupID := range accountLockExceptionManagerGroupIDs(env) {
		if groupIDs[groupID] {
			return true
		}
	}
	return false
}

func accountLockExceptionManagerGroupIDs(env *record.Env) []int64 {
	if env == nil {
		return nil
	}
	ctx := env.Context()
	systemEnv := env.WithContext(record.Context{UserID: 1, CompanyID: ctx.CompanyID, CompanyIDs: ctx.CompanyIDs})
	ids := modelDataResIDs(systemEnv, "account.group_account_manager", "res.groups")
	if len(ids) > 0 {
		return ids
	}
	return groupIDsByName(systemEnv, "account.group_account_manager")
}

func modelDataResIDs(env *record.Env, xmlID string, modelName string) []int64 {
	moduleName, name, ok := strings.Cut(xmlID, ".")
	if !ok || moduleName == "" || name == "" {
		return nil
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", domain.Equal, moduleName),
		domain.Cond("name", domain.Equal, name),
		domain.Cond("model", domain.Equal, modelName),
	))
	if err != nil {
		return nil
	}
	rows, err := found.Read("res_id")
	if err != nil {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := int64Value(row["res_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return ids
	}
	found, err = env.Model("ir.model.data").Search(domain.And(
		domain.Cond("complete_name", domain.Equal, xmlID),
		domain.Cond("model", domain.Equal, modelName),
	))
	if err != nil {
		return nil
	}
	rows, err = found.Read("res_id")
	if err != nil {
		return nil
	}
	for _, row := range rows {
		if id := int64Value(row["res_id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func groupIDsByName(env *record.Env, names ...string) []int64 {
	seen := map[int64]bool{}
	ids := []int64{}
	for _, name := range names {
		found, err := env.Model("res.groups").Search(domain.Cond("name", domain.Equal, name))
		if err != nil {
			continue
		}
		rows, err := found.Read()
		if err != nil {
			continue
		}
		for _, row := range rows {
			id := int64Value(row["id"])
			if id != 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (s Server) dispatchMovePaymentButton(env *record.Env, req callKWRequest) (any, bool, error) {
	switch req.Method {
	case "action_post", "button_draft", "button_cancel", "button_request_cancel":
		return s.dispatchMoveLifecycleButton(env, req)
	}
	if req.Method == "action_send_and_print" || req.Method == "action_invoice_sent" {
		return s.dispatchMoveSendButton(env, req)
	}
	if req.Method == "action_invoice_download_pdf" || req.Method == "action_print_pdf" || req.Method == "action_move_download_all" {
		return s.dispatchMovePDFButton(env, req)
	}
	if req.Method != "action_register_payment" && req.Method != "action_force_register_payment" {
		return nil, false, nil
	}
	moveIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(moveIDs) == 0 {
		return nil, true, fmt.Errorf("%s requires move ids", req.Method)
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, true, err
	}
	lineIDs := make([]int64, 0, len(moves))
	for _, move := range moves {
		if req.Method == "action_register_payment" && move.State != coreaccounting.MovePosted {
			return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
		}
		if move.MoveType == "entry" {
			return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
		}
		for _, line := range move.Lines {
			if line.IsReceivablePayable() {
				lineIDs = appendUniqueHTTPID(lineIDs, line.ID)
			}
		}
	}
	if len(lineIDs) == 0 {
		return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
	}
	return accountingPaymentRegisterOpenAction(lineIDs, nil), true, nil
}

func (s Server) dispatchMoveLifecycleButton(env *record.Env, req callKWRequest) (any, bool, error) {
	moveIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(moveIDs) == 0 {
		return nil, true, fmt.Errorf("%s requires move ids", req.Method)
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, true, err
	}
	for _, move := range moves {
		locks := env.Model("account.move").EffectiveAccountLockPolicy(move.CompanyID)
		updated := move
		switch req.Method {
		case "action_post":
			sequence := accountingMoveSequence(updated.Journal)
			if err := coreaccounting.PostMove(&updated, sequence, locks); err != nil {
				return nil, true, err
			}
			if err := persistAccountingMoveLifecycle(env, updated); err != nil {
				return nil, true, err
			}
			if updated.Journal.ID != 0 {
				if err := env.Model("account.journal").Browse(updated.Journal.ID).Write(map[string]any{"sequence_number_next": sequence.Next}); err != nil {
					return nil, true, err
				}
			}
		case "button_draft":
			if err := coreaccounting.ButtonDraft(&updated, locks); err != nil {
				return nil, true, err
			}
			if err := persistAccountingMoveLifecycleState(env, updated); err != nil {
				return nil, true, err
			}
		case "button_cancel":
			if err := coreaccounting.ButtonCancel(&updated, locks); err != nil {
				return nil, true, err
			}
			if err := persistAccountingMoveLifecycleState(env, updated); err != nil {
				return nil, true, err
			}
		case "button_request_cancel":
			if err := coreaccounting.ButtonRequestCancel(updated); err != nil {
				return nil, true, err
			}
			if err := env.Model("account.move").Browse(updated.ID).Write(map[string]any{"show_reset_to_draft_button": true}); err != nil {
				return nil, true, err
			}
		}
	}
	return true, true, nil
}

func accountingMoveSequence(journal coreaccounting.Journal) *coreaccounting.Sequence {
	next := journal.NextSequence
	if next <= 0 {
		next = 1
	}
	prefix := "MISC/"
	if code := strings.TrimSpace(journal.Code); code != "" {
		prefix = code + "/"
	}
	return &coreaccounting.Sequence{Prefix: prefix, Next: next}
}

func persistAccountingMoveLifecycle(env *record.Env, move coreaccounting.Move) error {
	writeEnv := env
	if move.State == coreaccounting.MovePosted {
		writeEnv = env.WithAccountMovePost()
	}
	return writeEnv.Model("account.move").Browse(move.ID).Write(map[string]any{
		"name":                       move.Name,
		"date":                       move.Date,
		"invoice_date":               move.InvoiceDate,
		"state":                      string(move.State),
		"amount_total":               move.AmountTotal,
		"amount_residual":            move.AmountResidual,
		"amount_residual_signed":     move.AmountResidualSigned,
		"payment_state":              string(move.PaymentState),
		"status_in_payment":          move.StatusInPayment,
		"posted_before":              move.PostedBefore,
		"inalterable_hash":           move.InalterableHash,
		"sequence_prefix":            move.SequencePrefix,
		"sequence_number":            move.SequenceNumber,
		"made_sequence_gap":          move.MadeSequenceGap,
		"secure_sequence_number":     move.SecureSequenceNumber,
		"auto_post":                  move.AutoPost,
		"show_reset_to_draft_button": coreaccounting.ShowResetToDraftButton(move),
	})
}

func persistAccountingMoveLifecycleState(env *record.Env, move coreaccounting.Move) error {
	writeEnv := env
	if move.State == coreaccounting.MovePosted {
		writeEnv = env.WithAccountMovePost()
	}
	if err := writeEnv.Model("account.move").Browse(move.ID).Write(map[string]any{
		"state":                      string(move.State),
		"auto_post":                  move.AutoPost,
		"show_reset_to_draft_button": coreaccounting.ShowResetToDraftButton(move),
	}); err != nil {
		return err
	}
	return nil
}

func (s Server) dispatchMoveSendButton(env *record.Env, req callKWRequest) (any, bool, error) {
	moveIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(moveIDs) == 0 {
		return nil, true, fmt.Errorf("%s requires move ids", req.Method)
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, true, err
	}
	for _, move := range moves {
		if !isSendableAccountingMove(move) {
			return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
		}
	}
	action := accountingMoveSendOpenAction(moveIDs)
	if req.Method == "action_invoice_sent" {
		context := action["context"].(map[string]any)
		context["allow_partners_without_mail"] = true
	}
	return action, true, nil
}

func (s Server) dispatchMovePDFButton(env *record.Env, req callKWRequest) (any, bool, error) {
	moveIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(moveIDs) == 0 {
		return nil, true, fmt.Errorf("%s requires move ids", req.Method)
	}
	if req.Method == "action_print_pdf" && len(moveIDs) != 1 {
		return nil, true, fmt.Errorf("%s requires one move", req.Method)
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, true, err
	}
	for _, move := range moves {
		if !isSendableAccountingMove(move) {
			return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
		}
		if _, err := ensureInvoicePDFPlaceholder(env, move); err != nil {
			return nil, true, err
		}
	}
	filetype := "pdf"
	target := "download"
	if req.Method == "action_print_pdf" {
		target = "self"
	} else if req.Method == "action_move_download_all" {
		filetype = "all"
	} else if len(req.Args) > 1 {
		if requested := strings.TrimSpace(stringValue(req.Args[1])); requested != "" {
			target = requested
		}
	}
	return map[string]any{
		"type":   "ir.actions.act_url",
		"url":    fmt.Sprintf("/account/download_invoice_documents/%s/%s", joinIDs(moveIDs), filetype),
		"target": target,
	}, true, nil
}

func (s Server) dispatchMoveLinePaymentButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Method != "action_register_payment" && req.Method != "action_payment_items_register_payment" {
		return nil, false, nil
	}
	lineIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if len(lineIDs) == 0 {
		return nil, true, fmt.Errorf("%s requires line ids", req.Method)
	}
	context := map[string]any{}
	if req.Method == "action_payment_items_register_payment" {
		context["default_group_payment"] = true
	}
	return accountingPaymentRegisterOpenAction(lineIDs, context), true, nil
}

func (s Server) dispatchMoveReversalButton(env *record.Env, req callKWRequest) (any, bool, error) {
	var modify bool
	switch req.Method {
	case "refund_moves":
	case "modify_moves":
		modify = true
	default:
		return nil, false, nil
	}
	wizardID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if wizardID == 0 {
		return nil, true, fmt.Errorf("%s requires a wizard id", req.Method)
	}
	wizard, moves, err := accountingReversalWizard(env, req, wizardID)
	if err != nil {
		return nil, true, err
	}
	if modify {
		sideEffectWizard := wizard
		reversalMoves, err := coreaccounting.ReverseMoves(&sideEffectWizard, moves, 0, false)
		if err != nil {
			return nil, true, err
		}
		if _, err := persistAccountingMoves(env, reversalMoves); err != nil {
			return nil, true, err
		}
		if shouldMarkOriginalReversed(wizard) {
			if err := markAccountingMovesReversed(env, moves); err != nil {
				return nil, true, err
			}
		}
	}
	newMoves, err := coreaccounting.ReverseMoves(&wizard, moves, 0, modify)
	if err != nil {
		return nil, true, err
	}
	if newMoves, err = persistAccountingMoves(env, newMoves); err != nil {
		return nil, true, err
	}
	newIDs := make([]int64, 0, len(newMoves))
	for _, move := range newMoves {
		newIDs = append(newIDs, move.ID)
	}
	if err := env.Model("account.move.reversal").Browse(wizardID).Write(map[string]any{"new_move_ids": newIDs}); err != nil {
		return nil, true, err
	}
	return accountingActionPayload(coreaccounting.ReversalAction(newMoves)), true, nil
}

func (s Server) dispatchPaymentRegisterButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Method != "action_create_payments" {
		return nil, false, nil
	}
	wizardID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if wizardID == 0 {
		return nil, true, fmt.Errorf("%s requires a wizard id", req.Method)
	}
	register, moves, err := accountingPaymentRegister(env, req, wizardID)
	if err != nil {
		return nil, true, err
	}
	payments, paidMoves, err := coreaccounting.CreateRegisteredPayments(register, moves, 0)
	if err != nil {
		return nil, true, err
	}
	payments, err = persistAccountingPayments(env, payments)
	if err != nil {
		return nil, true, err
	}
	if err := createPaymentReconciliationRecords(env, payments, paidMoves); err != nil {
		return nil, true, err
	}
	paymentIDs := make([]int64, 0, len(payments))
	for _, payment := range payments {
		paymentIDs = append(paymentIDs, payment.ID)
	}
	if err := markAccountingMovesPaid(env, paidMoves, paymentIDs); err != nil {
		return nil, true, err
	}
	if contextBool(req, "dont_redirect_to_payments") {
		return true, true, nil
	}
	return accountingPaymentActionPayload(paymentIDs), true, nil
}

func (s Server) dispatchMoveSendWizardButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Method != "action_send_and_print" {
		return nil, false, nil
	}
	wizardID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if wizardID == 0 {
		return nil, true, fmt.Errorf("%s requires a wizard id", req.Method)
	}
	rows, err := env.Model("account.move.send.wizard").Browse(wizardID).Read("move_id", "subject", "body", "mail_partner_ids", "sending_methods", "template_id", "mail_attachments_widget")
	if err != nil {
		return nil, true, err
	}
	if len(rows) != 1 {
		return nil, true, fmt.Errorf("account.move.send.wizard %d not found", wizardID)
	}
	moveID := int64Value(rows[0]["move_id"])
	if moveID == 0 {
		return nil, true, fmt.Errorf("account.move.send.wizard requires move_id")
	}
	if manualSendSelected(stringValue(rows[0]["sending_methods"])) {
		moves, err := readAccountingMoves(env, []int64{moveID})
		if err != nil {
			return nil, true, err
		}
		if len(moves) != 1 || !isSendableAccountingMove(moves[0]) {
			return nil, true, coreaccounting.ErrPaymentRegisterNoMoves
		}
		if _, err := ensureInvoicePDFPlaceholder(env, moves[0]); err != nil {
			return nil, true, err
		}
		return map[string]any{
			"type":   "ir.actions.act_url",
			"url":    fmt.Sprintf("/account/download_invoice_documents/%d/pdf", moveID),
			"target": "download",
		}, true, nil
	}
	if err := sendAccountingMoves(env, []int64{moveID}, accountingSendOptions{
		Subject:               stringValue(rows[0]["subject"]),
		Body:                  stringValue(rows[0]["body"]),
		PartnerIDs:            int64Slice(rows[0]["mail_partner_ids"]),
		TemplateID:            int64Value(rows[0]["template_id"]),
		MailAttachmentsWidget: stringValue(rows[0]["mail_attachments_widget"]),
	}); err != nil {
		return nil, true, err
	}
	return map[string]any{"type": "ir.actions.act_window_close"}, true, nil
}

func (s Server) dispatchMoveSendBatchWizardButton(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Method != "action_send_and_print" {
		return nil, false, nil
	}
	wizardID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if wizardID == 0 {
		return nil, true, fmt.Errorf("%s requires a wizard id", req.Method)
	}
	rows, err := env.Model("account.move.send.batch.wizard").Browse(wizardID).Read("move_ids")
	if err != nil {
		return nil, true, err
	}
	if len(rows) != 1 {
		return nil, true, fmt.Errorf("account.move.send.batch.wizard %d not found", wizardID)
	}
	moveIDs := int64Slice(rows[0]["move_ids"])
	if len(moveIDs) == 0 {
		return nil, true, fmt.Errorf("account.move.send.batch.wizard requires move_ids")
	}
	if err := sendAccountingMoves(env, moveIDs, accountingSendOptions{}); err != nil {
		return nil, true, err
	}
	return map[string]any{
		"type": "ir.actions.client",
		"tag":  "display_notification",
		"params": map[string]any{
			"type":    "info",
			"title":   "Sending invoices",
			"message": "Invoices are being sent in the background.",
			"next":    map[string]any{"type": "ir.actions.act_window_close"},
		},
	}, true, nil
}

func persistAccountingMoves(env *record.Env, moves []coreaccounting.Move) ([]coreaccounting.Move, error) {
	out := append([]coreaccounting.Move(nil), moves...)
	for i := range out {
		id, err := createAccountingMove(env, out[i])
		if err != nil {
			return nil, err
		}
		out[i].ID = id
	}
	return out, nil
}

func shouldMarkOriginalReversed(wizard coreaccounting.MoveReversal) bool {
	if wizard.Date.IsZero() || wizard.Date.After(time.Now().UTC()) {
		return false
	}
	return wizard.MoveType == "entry" || wizard.MoveType == "out_invoice" || wizard.MoveType == "in_invoice"
}

func markAccountingMovesReversed(env *record.Env, moves []coreaccounting.Move) error {
	ids := make([]int64, 0, len(moves))
	for _, move := range moves {
		if move.ID != 0 {
			ids = append(ids, move.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return env.Model("account.move").Browse(ids...).Write(map[string]any{
		"payment_state":     string(coreaccounting.PaymentReversed),
		"status_in_payment": string(coreaccounting.PaymentReversed),
	})
}

func accountingReversalWizard(env *record.Env, req callKWRequest, wizardID int64) (coreaccounting.MoveReversal, []coreaccounting.Move, error) {
	rows, err := env.Model("account.move.reversal").Browse(wizardID).Read("id", "move_ids", "date", "reason", "journal_id", "company_id", "country_code", "residual", "currency_id", "move_type")
	if err != nil {
		return coreaccounting.MoveReversal{}, nil, err
	}
	if len(rows) != 1 {
		return coreaccounting.MoveReversal{}, nil, fmt.Errorf("account.move.reversal %d not found", wizardID)
	}
	row := rows[0]
	moveIDs := int64Slice(row["move_ids"])
	if len(moveIDs) == 0 {
		moveIDs = activeIDsFromCallContext(req)
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return coreaccounting.MoveReversal{}, nil, err
	}
	journal, err := readAccountingJournal(env, int64Value(row["journal_id"]))
	if err != nil {
		return coreaccounting.MoveReversal{}, nil, err
	}
	wizard, err := coreaccounting.NewMoveReversal(moves, journal, accountingDateValue(row["date"]), stringValue(row["reason"]))
	if err != nil {
		return coreaccounting.MoveReversal{}, nil, err
	}
	wizard.ID = wizardID
	if companyID := int64Value(row["company_id"]); companyID != 0 {
		wizard.CompanyID = companyID
	}
	wizard.CountryCode = stringValue(row["country_code"])
	wizard.Residual = int64Value(row["residual"])
	if currency := accountingScalarString(row["currency_id"]); currency != "" {
		wizard.Currency = currency
	}
	if moveType := stringValue(row["move_type"]); moveType != "" {
		wizard.MoveType = moveType
	}
	return wizard, moves, nil
}

func accountingReversalDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model("account.move.reversal").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	if context["active_model"] != "account.move" {
		return values, nil
	}
	moveIDs := int64Slice(context["active_ids"])
	if len(moveIDs) == 0 {
		return values, nil
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, err
	}
	wizard, err := coreaccounting.NewMoveReversal(moves, coreaccounting.Journal{}, time.Time{}, "")
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, fieldName := range fields {
		requested[fieldName] = true
	}
	if requested["company_id"] {
		values["company_id"] = wizard.CompanyID
	}
	if requested["move_ids"] {
		values["move_ids"] = wizard.MoveIDs
	}
	if requested["journal_id"] && wizard.Journal.ID != 0 {
		values["journal_id"] = wizard.Journal.ID
	}
	if requested["available_journal_ids"] {
		values["available_journal_ids"] = wizard.AvailableJournalIDs
	}
	if requested["residual"] {
		values["residual"] = wizard.Residual
	}
	if requested["move_type"] {
		values["move_type"] = wizard.MoveType
	}
	return values, nil
}

func accountingPaymentRegister(env *record.Env, req callKWRequest, wizardID int64) (coreaccounting.PaymentRegister, []coreaccounting.Move, error) {
	rows, err := env.Model("account.payment.register").Browse(wizardID).Read(
		"line_ids", "payment_date", "amount", "communication", "group_payment", "currency_id", "journal_id", "available_journal_ids",
		"partner_bank_id", "company_id", "partner_id", "payment_method_line_id", "available_payment_method_line_ids", "payment_type",
		"partner_type", "source_amount", "source_amount_currency", "source_currency_id", "can_edit_wizard", "can_group_payments",
		"payment_difference", "payment_difference_handling", "writeoff_account_id", "writeoff_label", "total_payments_amount",
	)
	if err != nil {
		return coreaccounting.PaymentRegister{}, nil, err
	}
	if len(rows) != 1 {
		return coreaccounting.PaymentRegister{}, nil, fmt.Errorf("account.payment.register %d not found", wizardID)
	}
	row := rows[0]
	lineIDs := int64Slice(row["line_ids"])
	moveIDs, err := moveIDsFromPaymentLines(env, lineIDs)
	if err != nil {
		return coreaccounting.PaymentRegister{}, nil, err
	}
	if len(moveIDs) == 0 {
		moveIDs, err = paymentRegisterActiveMoveIDs(env, req)
		if err != nil {
			return coreaccounting.PaymentRegister{}, nil, err
		}
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return coreaccounting.PaymentRegister{}, nil, err
	}
	journal, err := readAccountingJournal(env, int64Value(row["journal_id"]))
	if err != nil {
		return coreaccounting.PaymentRegister{}, nil, err
	}
	register, err := coreaccounting.NewPaymentRegister(moves, journal, accountingDateValue(row["payment_date"]), int64Value(row["amount"]), stringValue(row["communication"]), accountingBoolValue(row["group_payment"]))
	if err != nil {
		return coreaccounting.PaymentRegister{}, nil, err
	}
	register.ID = wizardID
	register.LineIDs = lineIDs
	register.AvailableJournalIDs = int64Slice(row["available_journal_ids"])
	register.PartnerBankID = int64Value(row["partner_bank_id"])
	if companyID := int64Value(row["company_id"]); companyID != 0 {
		register.CompanyID = companyID
	}
	if partnerID := int64Value(row["partner_id"]); partnerID != 0 {
		register.PartnerID = partnerID
	}
	register.PaymentMethodLineID = int64Value(row["payment_method_line_id"])
	register.AvailablePaymentMethodLineIDs = int64Slice(row["available_payment_method_line_ids"])
	if paymentType := stringValue(row["payment_type"]); paymentType != "" {
		register.PaymentType = paymentType
	}
	if partnerType := stringValue(row["partner_type"]); partnerType != "" {
		register.PartnerType = partnerType
	}
	if currency := accountingScalarString(row["currency_id"]); currency != "" {
		register.Currency = currency
	}
	if sourceCurrency := accountingScalarString(row["source_currency_id"]); sourceCurrency != "" {
		register.SourceCurrency = sourceCurrency
	}
	if amount := int64Value(row["source_amount"]); amount != 0 {
		register.SourceAmount = amount
	}
	if amount := int64Value(row["source_amount_currency"]); amount != 0 {
		register.SourceAmountCurrency = amount
	}
	register.CanEditWizard = accountingBoolValue(firstNonNil(row["can_edit_wizard"], true))
	register.CanGroupPayments = accountingBoolValue(row["can_group_payments"])
	register.PaymentDifference = int64Value(row["payment_difference"])
	if handling := stringValue(row["payment_difference_handling"]); handling != "" {
		register.PaymentDifferenceHandling = handling
	}
	register.WriteoffAccountID = int64Value(row["writeoff_account_id"])
	if label := stringValue(row["writeoff_label"]); label != "" {
		register.WriteoffLabel = label
	}
	if total := intValue(row["total_payments_amount"]); total > 0 {
		register.TotalPaymentsAmount = total
	}
	return register, moves, nil
}

func accountingPaymentRegisterDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model("account.payment.register").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	moveIDs, err := paymentRegisterMoveIDsFromContext(env, context)
	if err != nil {
		return nil, err
	}
	if len(moveIDs) == 0 {
		return values, nil
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return nil, err
	}
	register, err := coreaccounting.NewPaymentRegister(moves, coreaccounting.Journal{}, time.Time{}, 0, "", false)
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, fieldName := range fields {
		requested[fieldName] = true
	}
	setIfRequested := func(name string, value any) {
		if requested[name] {
			values[name] = value
		}
	}
	setIfRequested("line_ids", register.LineIDs)
	setIfRequested("payment_date", register.PaymentDate)
	setIfRequested("amount", register.Amount)
	setIfRequested("communication", register.Communication)
	setIfRequested("group_payment", register.GroupPayment)
	setIfRequested("currency_id", int64Value(register.Currency))
	setIfRequested("journal_id", register.Journal.ID)
	setIfRequested("available_journal_ids", register.AvailableJournalIDs)
	setIfRequested("company_id", register.CompanyID)
	setIfRequested("partner_id", register.PartnerID)
	setIfRequested("payment_type", register.PaymentType)
	setIfRequested("partner_type", register.PartnerType)
	setIfRequested("source_amount", register.SourceAmount)
	setIfRequested("source_amount_currency", register.SourceAmountCurrency)
	setIfRequested("source_currency_id", int64Value(register.SourceCurrency))
	setIfRequested("can_edit_wizard", register.CanEditWizard)
	setIfRequested("can_group_payments", register.CanGroupPayments)
	setIfRequested("payment_difference", register.PaymentDifference)
	setIfRequested("payment_difference_handling", register.PaymentDifferenceHandling)
	setIfRequested("writeoff_label", register.WriteoffLabel)
	setIfRequested("total_payments_amount", register.TotalPaymentsAmount)
	return values, nil
}

func accountingMoveSendDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model("account.move.send.wizard").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	moveIDs := sendActiveMoveIDs(context)
	if len(moveIDs) == 0 {
		return values, nil
	}
	moves, err := readAccountingMoves(env, []int64{moveIDs[0]})
	if err != nil {
		return nil, err
	}
	move := moves[0]
	requested := map[string]bool{}
	for _, fieldName := range fields {
		requested[fieldName] = true
	}
	setIfRequested := func(name string, value any) {
		if requested[name] {
			values[name] = value
		}
	}
	setIfRequested("move_id", move.ID)
	setIfRequested("company_id", move.CompanyID)
	setIfRequested("model", "account.move")
	setIfRequested("res_ids", fmt.Sprintf("%d", move.ID))
	setIfRequested("render_model", "account.move")
	setIfRequested("sending_methods", `{"email":true}`)
	setIfRequested("sending_method_checkboxes", `{"email":{"checked":true,"label":"Email"}}`)
	templateID := defaultAccountingMailTemplateID(env, move)
	setIfRequested("template_id", templateID)
	subject := defaultInvoiceSubject(move)
	body := defaultInvoiceBody(move)
	if templateID != 0 {
		rendered, err := internalmail.RenderTemplateForRecord(env, templateID, "account.move", move.ID, nil)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(rendered.Subject) != "" {
			subject = rendered.Subject
		}
		if strings.TrimSpace(rendered.Body) != "" {
			body = rendered.Body
		}
	}
	setIfRequested("subject", subject)
	setIfRequested("body", body)
	setIfRequested("mail_partner_ids", defaultAccountingMailPartnerIDs(move))
	setIfRequested("display_attachments_widget", true)
	if requested["mail_attachments_widget"] {
		widget, err := defaultAccountingMailAttachmentsWidget(env, move, 0, templateID)
		if err != nil {
			return nil, err
		}
		values["mail_attachments_widget"] = widget
	}
	setIfRequested("can_edit_body", true)
	return values, nil
}

func accountingMoveSendBatchDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model("account.move.send.batch.wizard").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	moveIDs := sendActiveMoveIDs(context)
	if len(moveIDs) == 0 {
		return values, nil
	}
	requested := map[string]bool{}
	for _, fieldName := range fields {
		requested[fieldName] = true
	}
	if requested["move_ids"] {
		values["move_ids"] = moveIDs
	}
	if requested["summary_data"] {
		values["summary_data"] = fmt.Sprintf(`{"email":{"count":%d,"label":"Email"}}`, len(moveIDs))
	}
	return values, nil
}

func readAccountingMoves(env *record.Env, ids []int64) ([]coreaccounting.Move, error) {
	if len(ids) == 0 {
		return nil, coreaccounting.ErrReversalNoMoves
	}
	rows, err := env.Model("account.move").Browse(ids...).Read(
		"name", "ref", "date", "invoice_date", "invoice_date_due", "state", "move_type", "journal_id", "company_id", "currency_id", "partner_id",
		"fiscal_position_id", "amount_total", "amount_residual", "amount_residual_signed", "payment_state", "status_in_payment", "is_move_sent", "origin_payment_id",
		"statement_line_id", "matched_payment_ids", "reconciled_payment_ids", "payment_count", "posted_before", "inalterable_hash", "sequence_prefix",
		"sequence_number", "made_sequence_gap", "secure_sequence_number", "auto_post", "need_cancel_request", "reversed_entry_id", "line_ids",
	)
	if err != nil {
		return nil, err
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[int64Value(row["id"])] = row
	}
	moves := make([]coreaccounting.Move, 0, len(ids))
	for _, id := range ids {
		row := byID[id]
		if row == nil {
			return nil, fmt.Errorf("account.move %d not found", id)
		}
		journal, err := readAccountingJournal(env, int64Value(row["journal_id"]))
		if err != nil {
			return nil, err
		}
		lines, err := readAccountingMoveLines(env, id, row["line_ids"])
		if err != nil {
			return nil, err
		}
		moves = append(moves, coreaccounting.Move{
			ID:                   id,
			Name:                 stringValue(row["name"]),
			Ref:                  stringValue(row["ref"]),
			Date:                 accountingDateValue(row["date"]),
			InvoiceDate:          accountingDateValue(row["invoice_date"]),
			InvoiceDateDue:       accountingDateValue(row["invoice_date_due"]),
			State:                coreaccounting.MoveState(stringValue(row["state"])),
			MoveType:             stringValue(row["move_type"]),
			Journal:              journal,
			CompanyID:            int64Value(row["company_id"]),
			Currency:             accountingScalarString(row["currency_id"]),
			CurrencyID:           int64Value(row["currency_id"]),
			PartnerID:            int64Value(row["partner_id"]),
			FiscalPositionID:     int64Value(row["fiscal_position_id"]),
			Lines:                lines,
			PostedBefore:         accountingBoolValue(row["posted_before"]),
			InalterableHash:      stringValue(row["inalterable_hash"]),
			SequencePrefix:       stringValue(row["sequence_prefix"]),
			SequenceNumber:       int64Value(row["sequence_number"]),
			MadeSequenceGap:      accountingBoolValue(row["made_sequence_gap"]),
			SecureSequenceNumber: int64Value(row["secure_sequence_number"]),
			AmountTotal:          int64Value(row["amount_total"]),
			AmountResidual:       int64Value(row["amount_residual"]),
			AmountResidualSigned: int64Value(row["amount_residual_signed"]),
			AutoPost:             stringValue(row["auto_post"]),
			PaymentState:         coreaccounting.PaymentState(stringValue(row["payment_state"])),
			StatusInPayment:      stringValue(row["status_in_payment"]),
			IsMoveSent:           accountingBoolValue(row["is_move_sent"]),
			OriginPaymentID:      int64Value(row["origin_payment_id"]),
			StatementLineID:      int64Value(row["statement_line_id"]),
			MatchedPaymentIDs:    int64Slice(row["matched_payment_ids"]),
			ReconciledPaymentIDs: int64Slice(row["reconciled_payment_ids"]),
			PaymentCount:         intValue(row["payment_count"]),
			NeedCancelRequest:    accountingBoolValue(row["need_cancel_request"]),
			ReversedEntryID:      int64Value(row["reversed_entry_id"]),
		})
	}
	return moves, nil
}

func readAccountingMoveLines(env *record.Env, moveID int64, rawLineIDs any) ([]coreaccounting.MoveLine, error) {
	lineIDs := int64Slice(rawLineIDs)
	if len(lineIDs) == 0 {
		found, err := env.Model("account.move.line").Search(domain.Cond("move_id", domain.Equal, moveID))
		if err != nil {
			return nil, err
		}
		lineIDs = found.IDs()
	}
	if len(lineIDs) == 0 {
		return nil, nil
	}
	rows, err := env.Model("account.move.line").Browse(lineIDs...).Read(
		"move_id", "account_id", "account_type", "account_internal_group", "partner_id", "company_id", "currency_id", "name",
		"debit", "credit", "quantity", "price_unit", "price_subtotal", "price_total", "discount", "display_type", "date_maturity",
		"product_id", "product_uom_id", "product_category_id", "amount_currency", "amount_residual", "amount_residual_currency", "reconciled", "payment_id", "full_reconcile_id",
		"matched_debit_ids", "matched_credit_ids", "tax_line_id", "tax_repartition_line_id",
	)
	if err != nil {
		return nil, err
	}
	lines := make([]coreaccounting.MoveLine, 0, len(rows))
	for _, row := range rows {
		account := coreaccounting.Account{
			ID:   int64Value(row["account_id"]),
			Kind: accountingAccountKind(row["account_type"], row["account_internal_group"]),
		}
		lines = append(lines, coreaccounting.MoveLine{
			ID:                   int64Value(row["id"]),
			Account:              account,
			PartnerID:            int64Value(row["partner_id"]),
			CompanyID:            int64Value(row["company_id"]),
			Currency:             accountingScalarString(row["currency_id"]),
			CurrencyID:           int64Value(row["currency_id"]),
			Name:                 stringValue(row["name"]),
			Debit:                int64Value(row["debit"]),
			Credit:               int64Value(row["credit"]),
			Quantity:             floatValue(row["quantity"]),
			PriceUnit:            int64Value(row["price_unit"]),
			PriceSubtotal:        int64Value(row["price_subtotal"]),
			PriceTotal:           int64Value(row["price_total"]),
			Discount:             floatValue(row["discount"]),
			DisplayType:          stringValue(row["display_type"]),
			DateMaturity:         accountingDateValue(row["date_maturity"]),
			ProductID:            int64Value(row["product_id"]),
			ProductUOMID:         int64Value(row["product_uom_id"]),
			ProductCategoryID:    int64Value(row["product_category_id"]),
			AmountCurrency:       int64Value(row["amount_currency"]),
			Residual:             int64Value(row["amount_residual"]),
			ResidualCurrency:     int64Value(row["amount_residual_currency"]),
			Reconciled:           accountingBoolValue(row["reconciled"]),
			PaymentID:            int64Value(row["payment_id"]),
			FullReconcileID:      int64Value(row["full_reconcile_id"]),
			MatchedDebitIDs:      int64Slice(row["matched_debit_ids"]),
			MatchedCreditIDs:     int64Slice(row["matched_credit_ids"]),
			TaxID:                int64Value(row["tax_line_id"]),
			TaxRepartitionLineID: int64Value(row["tax_repartition_line_id"]),
		})
	}
	return lines, nil
}

func refreshAccountingInvoiceReport(env *record.Env, moveIDs []int64) error {
	if len(moveIDs) == 0 {
		found, err := env.Model("account.move").Search(domain.Cond("move_type", domain.In, []string{"out_invoice", "in_invoice", "out_refund", "in_refund", "out_receipt", "in_receipt"}))
		if err != nil {
			return err
		}
		moveIDs = found.IDs()
	}
	if len(moveIDs) == 0 {
		return nil
	}
	existing, err := env.Model("account.invoice.report").Search(domain.Cond("move_id", domain.In, moveIDs))
	if err != nil {
		return err
	}
	if existing.Len() > 0 {
		if err := existing.Unlink(); err != nil {
			return err
		}
	}
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return err
	}
	for _, row := range coreaccounting.BuildInvoiceReportRows(moves) {
		if _, err := env.Model("account.invoice.report").Create(map[string]any{
			"move_id":                 row.MoveID,
			"journal_id":              row.JournalID,
			"company_id":              row.CompanyID,
			"company_currency_id":     row.CompanyCurrencyID,
			"partner_id":              row.PartnerID,
			"commercial_partner_id":   row.CommercialPartnerID,
			"country_id":              row.CountryID,
			"invoice_user_id":         row.InvoiceUserID,
			"move_type":               row.MoveType,
			"state":                   string(row.State),
			"payment_state":           string(row.PaymentState),
			"fiscal_position_id":      row.FiscalPositionID,
			"invoice_date":            row.InvoiceDate,
			"quantity":                row.Quantity,
			"product_id":              row.ProductID,
			"product_uom_id":          row.ProductUOMID,
			"product_categ_id":        row.ProductCategoryID,
			"invoice_date_due":        row.InvoiceDateDue,
			"account_id":              row.AccountID,
			"price_subtotal_currency": row.PriceSubtotalCurrency,
			"price_subtotal":          row.PriceSubtotal,
			"price_total":             row.PriceTotal,
			"price_total_currency":    row.PriceTotalCurrency,
			"price_average":           row.PriceAverage,
			"price_margin":            row.PriceMargin,
			"inventory_value":         row.InventoryValue,
			"currency_id":             row.CurrencyID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func readAccountingJournal(env *record.Env, id int64) (coreaccounting.Journal, error) {
	if id == 0 {
		return coreaccounting.Journal{}, nil
	}
	rows, err := env.Model("account.journal").Browse(id).Read("name", "code", "type", "company_id", "currency_id", "default_account_id", "restrict_mode_hash_table", "sequence_number_next")
	if err != nil {
		return coreaccounting.Journal{}, err
	}
	if len(rows) == 0 {
		return coreaccounting.Journal{}, fmt.Errorf("account.journal %d not found", id)
	}
	row := rows[0]
	return coreaccounting.Journal{
		ID:                    id,
		Name:                  stringValue(row["name"]),
		Code:                  stringValue(row["code"]),
		Type:                  coreaccounting.JournalType(stringValue(row["type"])),
		CompanyID:             int64Value(row["company_id"]),
		Currency:              accountingScalarString(row["currency_id"]),
		DefaultAccountID:      int64Value(row["default_account_id"]),
		RestrictModeHashTable: accountingBoolValue(row["restrict_mode_hash_table"]),
		NextSequence:          int64Value(row["sequence_number_next"]),
	}, nil
}

func createAccountingMove(env *record.Env, move coreaccounting.Move) (int64, error) {
	values := map[string]any{
		"name":                   move.Name,
		"ref":                    move.Ref,
		"date":                   move.Date,
		"invoice_date":           move.InvoiceDate,
		"invoice_date_due":       move.InvoiceDateDue,
		"state":                  string(move.State),
		"move_type":              move.MoveType,
		"journal_id":             move.Journal.ID,
		"company_id":             move.CompanyID,
		"currency_id":            int64Value(move.Currency),
		"partner_id":             move.PartnerID,
		"fiscal_position_id":     move.FiscalPositionID,
		"amount_total":           move.AmountTotal,
		"amount_residual":        move.AmountResidual,
		"amount_residual_signed": move.AmountResidualSigned,
		"payment_state":          string(move.PaymentState),
		"status_in_payment":      move.StatusInPayment,
		"is_move_sent":           move.IsMoveSent,
		"origin_payment_id":      move.OriginPaymentID,
		"statement_line_id":      move.StatementLineID,
		"matched_payment_ids":    move.MatchedPaymentIDs,
		"reconciled_payment_ids": move.ReconciledPaymentIDs,
		"payment_count":          move.PaymentCount,
		"posted_before":          move.PostedBefore,
		"inalterable_hash":       move.InalterableHash,
		"sequence_prefix":        move.SequencePrefix,
		"sequence_number":        move.SequenceNumber,
		"made_sequence_gap":      move.MadeSequenceGap,
		"secure_sequence_number": move.SecureSequenceNumber,
		"auto_post":              move.AutoPost,
		"need_cancel_request":    move.NeedCancelRequest,
		"reversed_entry_id":      move.ReversedEntryID,
	}
	createEnv := env
	if move.State == coreaccounting.MovePosted {
		createEnv = env.WithAccountMovePost()
	}
	id, err := createEnv.Model("account.move").Create(values)
	if err != nil {
		return 0, err
	}
	lineIDs := make([]int64, 0, len(move.Lines))
	for _, line := range move.Lines {
		lineID, err := env.Model("account.move.line").Create(map[string]any{
			"move_id":                  id,
			"account_id":               line.Account.ID,
			"account_type":             string(line.Account.Kind),
			"account_internal_group":   string(line.Account.Kind),
			"partner_id":               line.PartnerID,
			"company_id":               line.CompanyID,
			"currency_id":              int64Value(line.Currency),
			"name":                     line.Name,
			"debit":                    line.Debit,
			"credit":                   line.Credit,
			"amount_currency":          line.AmountCurrency,
			"amount_residual":          line.Residual,
			"amount_residual_currency": line.ResidualCurrency,
			"reconciled":               line.Reconciled,
			"payment_id":               line.PaymentID,
			"full_reconcile_id":        line.FullReconcileID,
			"matched_debit_ids":        line.MatchedDebitIDs,
			"matched_credit_ids":       line.MatchedCreditIDs,
			"tax_line_id":              line.TaxID,
			"tax_repartition_line_id":  line.TaxRepartitionLineID,
		})
		if err != nil {
			return 0, err
		}
		lineIDs = append(lineIDs, lineID)
	}
	if len(lineIDs) > 0 {
		if err := env.Model("account.move").Browse(id).Write(map[string]any{"line_ids": lineIDs}); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func persistAccountingPayments(env *record.Env, payments []coreaccounting.Payment) ([]coreaccounting.Payment, error) {
	out := append([]coreaccounting.Payment(nil), payments...)
	for i := range out {
		id, err := env.Model("account.payment").Create(map[string]any{
			"name":                   out[i].Name,
			"payment_type":           out[i].PaymentType,
			"partner_type":           out[i].PartnerType,
			"partner_id":             out[i].PartnerID,
			"amount":                 out[i].Amount,
			"company_id":             out[i].CompanyID,
			"currency_id":            int64Value(out[i].Currency),
			"journal_id":             out[i].JournalID,
			"state":                  out[i].State,
			"is_reconciled":          out[i].IsReconciled,
			"is_matched":             out[i].IsMatched,
			"invoice_ids":            out[i].InvoiceIDs,
			"reconciled_invoice_ids": out[i].ReconciledInvoiceIDs,
			"reconciled_bill_ids":    out[i].ReconciledBillIDs,
			"need_cancel_request":    out[i].NeedCancelRequest,
		})
		if err != nil {
			return nil, err
		}
		out[i].ID = id
	}
	return out, nil
}

func createPaymentReconciliationRecords(env *record.Env, payments []coreaccounting.Payment, moves []coreaccounting.Move) error {
	movesByID := map[int64]coreaccounting.Move{}
	for _, move := range moves {
		movesByID[move.ID] = move
	}
	for _, payment := range payments {
		sourceMoves := paymentSourceMoves(payment, movesByID)
		sourceLines := paymentSourceLines(sourceMoves)
		if len(sourceLines) == 0 {
			continue
		}
		paymentMoveID, paymentLineIDs, err := createPaymentMove(env, payment, sourceLines)
		if err != nil {
			return err
		}
		if err := env.Model("account.payment").Browse(payment.ID).Write(map[string]any{"move_id": paymentMoveID}); err != nil {
			return err
		}
		if err := createFullReconcileRows(env, payment, sourceLines, paymentLineIDs); err != nil {
			return err
		}
	}
	return nil
}

func paymentSourceMoves(payment coreaccounting.Payment, movesByID map[int64]coreaccounting.Move) []coreaccounting.Move {
	ids := append(append([]int64{}, payment.ReconciledInvoiceIDs...), payment.ReconciledBillIDs...)
	out := make([]coreaccounting.Move, 0, len(ids))
	for _, id := range ids {
		if move, ok := movesByID[id]; ok {
			out = append(out, move)
		}
	}
	return out
}

func paymentSourceLines(moves []coreaccounting.Move) []coreaccounting.MoveLine {
	var lines []coreaccounting.MoveLine
	for _, move := range moves {
		for _, line := range move.Lines {
			if line.IsReceivablePayable() {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

func createPaymentMove(env *record.Env, payment coreaccounting.Payment, sourceLines []coreaccounting.MoveLine) (int64, []int64, error) {
	var total int64
	counterpartLines := make([]coreaccounting.MoveLine, 0, len(sourceLines)+1)
	for _, source := range sourceLines {
		amount := absHTTP(source.Balance())
		total += amount
		line := coreaccounting.MoveLine{
			Account:          source.Account,
			PartnerID:        source.PartnerID,
			CompanyID:        source.CompanyID,
			Currency:         source.Currency,
			Name:             payment.Name,
			PaymentID:        payment.ID,
			Residual:         0,
			ResidualCurrency: 0,
			Reconciled:       true,
		}
		if source.Balance() >= 0 {
			line.Credit = amount
			line.AmountCurrency = -amount
		} else {
			line.Debit = amount
			line.AmountCurrency = amount
		}
		counterpartLines = append(counterpartLines, line)
	}
	liquidity := coreaccounting.MoveLine{
		Account:   coreaccounting.Account{ID: payment.JournalDefaultAccount.ID, Kind: coreaccounting.AccountCash, Reconcile: true},
		CompanyID: payment.CompanyID,
		Currency:  payment.Currency,
		Name:      payment.Name,
		PaymentID: payment.ID,
	}
	if payment.PaymentType == "outbound" {
		liquidity.Credit = total
		liquidity.AmountCurrency = -total
	} else {
		liquidity.Debit = total
		liquidity.AmountCurrency = total
	}
	move := coreaccounting.Move{
		Name:         payment.Name,
		Date:         time.Now().UTC(),
		State:        coreaccounting.MovePosted,
		MoveType:     "entry",
		CompanyID:    payment.CompanyID,
		Currency:     payment.Currency,
		PartnerID:    payment.PartnerID,
		Journal:      coreaccounting.Journal{ID: payment.JournalID, DefaultAccountID: payment.JournalDefaultAccount.ID},
		Lines:        append([]coreaccounting.MoveLine{liquidity}, counterpartLines...),
		PostedBefore: true,
		AutoPost:     "no",
	}
	coreaccounting.RefreshMoveAmounts(&move)
	moveID, err := createAccountingMove(env, move)
	if err != nil {
		return 0, nil, err
	}
	rows, err := env.Model("account.move").Browse(moveID).Read("line_ids")
	if err != nil {
		return 0, nil, err
	}
	lineIDs := int64Slice(rows[0]["line_ids"])
	if len(lineIDs) <= 1 {
		return moveID, nil, nil
	}
	return moveID, append([]int64(nil), lineIDs[1:]...), nil
}

func createFullReconcileRows(env *record.Env, payment coreaccounting.Payment, sourceLines []coreaccounting.MoveLine, paymentLineIDs []int64) error {
	for i, source := range sourceLines {
		if i >= len(paymentLineIDs) {
			break
		}
		fullID, err := env.Model("account.full.reconcile").Create(map[string]any{"name": fmt.Sprintf("P%06d-%d", payment.ID, i+1)})
		if err != nil {
			return err
		}
		partialValues := map[string]any{
			"amount":                 absHTTP(source.Balance()),
			"debit_amount_currency":  absHTTP(source.AmountCurrency),
			"credit_amount_currency": absHTTP(source.AmountCurrency),
			"full_reconcile_id":      fullID,
			"company_id":             source.CompanyID,
		}
		if source.Balance() >= 0 {
			partialValues["debit_move_id"] = source.ID
			partialValues["credit_move_id"] = paymentLineIDs[i]
		} else {
			partialValues["debit_move_id"] = paymentLineIDs[i]
			partialValues["credit_move_id"] = source.ID
		}
		partialID, err := env.Model("account.partial.reconcile").Create(partialValues)
		if err != nil {
			return err
		}
		if err := env.Model("account.full.reconcile").Browse(fullID).Write(map[string]any{"partial_reconcile_ids": []int64{partialID}}); err != nil {
			return err
		}
		if err := updateMatchedLine(env, source.ID, source.Balance() >= 0, partialID, fullID); err != nil {
			return err
		}
		if err := updateMatchedLine(env, paymentLineIDs[i], source.Balance() < 0, partialID, fullID); err != nil {
			return err
		}
	}
	return nil
}

func updateMatchedLine(env *record.Env, lineID int64, debitSide bool, partialID int64, fullID int64) error {
	rows, err := env.Model("account.move.line").Browse(lineID).Read("matched_credit_ids", "matched_debit_ids")
	if err != nil {
		return err
	}
	values := map[string]any{
		"full_reconcile_id":        fullID,
		"amount_residual":          int64(0),
		"amount_residual_currency": int64(0),
		"reconciled":               true,
	}
	var existing []int64
	if len(rows) > 0 {
		if debitSide {
			existing = int64Slice(rows[0]["matched_credit_ids"])
		} else {
			existing = int64Slice(rows[0]["matched_debit_ids"])
		}
	}
	existing = appendUniqueHTTPID(existing, partialID)
	if debitSide {
		values["matched_credit_ids"] = existing
	} else {
		values["matched_debit_ids"] = existing
	}
	return env.Model("account.move.line").Browse(lineID).Write(values)
}

func markAccountingMovesPaid(env *record.Env, moves []coreaccounting.Move, paymentIDs []int64) error {
	for _, move := range moves {
		if move.ID == 0 {
			continue
		}
		reconciledPaymentIDs := append([]int64(nil), move.ReconciledPaymentIDs...)
		matchedPaymentIDs := append([]int64(nil), move.MatchedPaymentIDs...)
		for _, paymentID := range paymentIDs {
			reconciledPaymentIDs = appendUniqueHTTPID(reconciledPaymentIDs, paymentID)
			matchedPaymentIDs = appendUniqueHTTPID(matchedPaymentIDs, paymentID)
		}
		if err := env.Model("account.move").Browse(move.ID).Write(map[string]any{
			"amount_residual":        int64(0),
			"amount_residual_signed": int64(0),
			"payment_state":          string(coreaccounting.PaymentPaid),
			"status_in_payment":      string(coreaccounting.PaymentPaid),
			"reconciled_payment_ids": reconciledPaymentIDs,
			"matched_payment_ids":    matchedPaymentIDs,
			"payment_count":          len(reconciledPaymentIDs),
		}); err != nil {
			return err
		}
		for _, line := range move.Lines {
			if line.ID == 0 || !line.IsReceivablePayable() {
				continue
			}
			if err := env.Model("account.move.line").Browse(line.ID).Write(map[string]any{
				"amount_residual":          int64(0),
				"amount_residual_currency": int64(0),
				"reconciled":               true,
				"payment_id":               firstID(paymentIDs),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func accountingActionPayload(action coreaccounting.ActionResult) map[string]any {
	payload := map[string]any{
		"name":      action.Name,
		"type":      action.Type,
		"res_model": action.ResModel,
		"view_mode": action.ViewMode,
		"context":   action.Context,
	}
	if action.ResID != 0 {
		payload["res_id"] = action.ResID
	}
	if len(action.Domain) > 0 {
		payload["domain"] = []any{[]any{"id", "in", action.Domain}}
	}
	return payload
}

func accountingPaymentActionPayload(paymentIDs []int64) map[string]any {
	payload := map[string]any{
		"name":      "Payments",
		"type":      "ir.actions.act_window",
		"res_model": "account.payment",
		"context":   map[string]any{"create": false},
	}
	if len(paymentIDs) == 1 {
		payload["view_mode"] = "form"
		payload["res_id"] = paymentIDs[0]
		return payload
	}
	payload["view_mode"] = "list,form"
	payload["domain"] = []any{[]any{"id", "in", paymentIDs}}
	return payload
}

func accountingPaymentRegisterOpenAction(lineIDs []int64, extraContext map[string]any) map[string]any {
	context := map[string]any{
		"active_model": "account.move.line",
		"active_ids":   lineIDs,
	}
	for key, value := range extraContext {
		context[key] = value
	}
	return map[string]any{
		"name":      "Pay",
		"res_model": "account.payment.register",
		"view_mode": "form",
		"views":     []any{[]any{false, "form"}},
		"context":   context,
		"target":    "new",
		"type":      "ir.actions.act_window",
	}
}

func accountingMoveSendOpenAction(moveIDs []int64) map[string]any {
	resModel := "account.move.send.batch.wizard"
	if len(moveIDs) == 1 {
		resModel = "account.move.send.wizard"
	}
	return map[string]any{
		"name":      "Send",
		"type":      "ir.actions.act_window",
		"view_mode": "form",
		"res_model": resModel,
		"target":    "new",
		"context": map[string]any{
			"active_model": "account.move",
			"active_ids":   moveIDs,
		},
	}
}

type accountingSendOptions struct {
	Subject               string
	Body                  string
	PartnerIDs            []int64
	TemplateID            int64
	MailAttachmentsWidget string
}

func sendAccountingMoves(env *record.Env, moveIDs []int64, opts accountingSendOptions) error {
	moves, err := readAccountingMoves(env, moveIDs)
	if err != nil {
		return err
	}
	for _, move := range moves {
		if !isSendableAccountingMove(move) {
			return coreaccounting.ErrPaymentRegisterNoMoves
		}
		messageSubject := opts.Subject
		messageBody := opts.Body
		if opts.TemplateID != 0 && (strings.TrimSpace(messageSubject) == "" || strings.TrimSpace(messageBody) == "") {
			rendered, err := internalmail.RenderTemplateForRecord(env, opts.TemplateID, "account.move", move.ID, nil)
			if err != nil {
				return err
			}
			if strings.TrimSpace(messageSubject) == "" {
				messageSubject = rendered.Subject
			}
			if strings.TrimSpace(messageBody) == "" {
				messageBody = rendered.Body
			}
		}
		if messageSubject == "" {
			messageSubject = defaultInvoiceSubject(move)
		}
		if messageBody == "" {
			messageBody = defaultInvoiceBody(move)
		}
		attachmentID, err := ensureInvoicePDFPlaceholder(env, move)
		if err != nil {
			return err
		}
		messageID, err := createMailMessage(env, move, messageSubject, messageBody)
		if err != nil {
			return err
		}
		selectedAttachmentIDs, err := selectedAccountingMailAttachmentIDs(env, attachmentID, opts.TemplateID, opts.MailAttachmentsWidget)
		if err != nil {
			return err
		}
		mailAttachmentIDs := make([]int64, 0, len(selectedAttachmentIDs))
		for _, selectedAttachmentID := range selectedAttachmentIDs {
			mailAttachmentID, err := createMailMessageAttachmentCopy(env, selectedAttachmentID, messageID)
			if err != nil {
				return err
			}
			mailAttachmentIDs = append(mailAttachmentIDs, mailAttachmentID)
		}
		dynamicReportAttachmentIDs, err := createAccountingTemplateReportAttachments(env, opts.TemplateID, move.ID, messageID)
		if err != nil {
			return err
		}
		mailAttachmentIDs = append(mailAttachmentIDs, dynamicReportAttachmentIDs...)
		postprocessAttachmentIDs, err := internalmail.CreateTemplatePostprocessAttachments(env, opts.TemplateID, "account.move", move.ID, "mail.message", messageID)
		if err != nil {
			return err
		}
		mailAttachmentIDs = append(mailAttachmentIDs, postprocessAttachmentIDs...)
		mailAttachmentIDs = uniqueInt64Slice(mailAttachmentIDs)
		if err := env.Model("mail.message").Browse(messageID).Write(map[string]any{"attachment_ids": mailAttachmentIDs}); err != nil {
			return err
		}
		if err := createOutgoingMail(env, messageID, messageSubject, messageBody, opts.PartnerIDs, mailAttachmentIDs); err != nil {
			return err
		}
		if err := env.Model("account.move").Browse(move.ID).Write(map[string]any{
			"is_move_sent":               true,
			"is_being_sent":              false,
			"sending_data":               "",
			"invoice_pdf_report_id":      attachmentID,
			"invoice_pdf_report_file":    validInvoicePDFBytes(move),
			"message_main_attachment_id": attachmentID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func createInvoicePDFPlaceholder(env *record.Env, move coreaccounting.Move) (int64, error) {
	name := invoicePDFName(move)
	data := validInvoicePDFBytes(move)
	return env.Model("ir.attachment").Create(map[string]any{
		"name":       name,
		"res_model":  "account.move",
		"res_field":  "invoice_pdf_report_file",
		"res_id":     move.ID,
		"company_id": move.CompanyID,
		"type":       "binary",
		"mimetype":   "application/pdf",
		"datas":      data,
		"file_size":  len(data),
	})
}

func selectedAccountingMailAttachmentIDs(env *record.Env, invoiceAttachmentID int64, templateID int64, widgetText string) ([]int64, error) {
	widgetRows, err := parseAccountingMailAttachmentWidget(widgetText)
	if err != nil {
		return nil, err
	}
	if len(widgetRows) == 0 {
		defaultIDs := []int64{invoiceAttachmentID}
		templateAttachmentIDs, err := accountingTemplateAttachmentIDs(env, templateID)
		if err != nil {
			return nil, err
		}
		defaultIDs = append(defaultIDs, templateAttachmentIDs...)
		return uniqueInt64Slice(defaultIDs), nil
	}
	skipNames := map[string]bool{}
	for _, row := range widgetRows {
		if row.Skip {
			skipNames[row.Name] = true
		}
	}
	candidates := make([]accountingAttachmentWidgetRow, 0, len(widgetRows)+1)
	if invoiceAttachmentID != 0 {
		invoiceRows, err := env.Model("ir.attachment").Browse(invoiceAttachmentID).Read("name")
		if err != nil {
			return nil, err
		}
		if len(invoiceRows) == 1 {
			candidates = append(candidates, accountingAttachmentWidgetRow{ID: invoiceAttachmentID, Name: stringValue(invoiceRows[0]["name"])})
		}
	}
	candidates = append(candidates, widgetRows...)
	selectedIDs := []int64{}
	for _, row := range candidates {
		if row.ID == 0 {
			continue
		}
		if skipNames[row.Name] && !row.Manual {
			continue
		}
		selectedIDs = append(selectedIDs, row.ID)
	}
	return uniqueInt64Slice(selectedIDs), nil
}

type accountingAttachmentWidgetRow struct {
	ID     int64
	Name   string
	Manual bool
	Skip   bool
}

func parseAccountingMailAttachmentWidget(text string) ([]accountingAttachmentWidgetRow, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	out := make([]accountingAttachmentWidgetRow, 0, len(raw))
	for _, item := range raw {
		out = append(out, accountingAttachmentWidgetRow{
			ID:     int64Value(item["id"]),
			Name:   stringValue(item["name"]),
			Manual: boolHTTPWithFallback(item["manual"], false),
			Skip:   boolHTTPWithFallback(item["skip"], false),
		})
	}
	return out, nil
}

func defaultAccountingMailAttachmentsWidget(env *record.Env, move coreaccounting.Move, invoiceAttachmentID int64, templateID int64) (string, error) {
	items := make([]map[string]any, 0, 2)
	if invoiceAttachmentID != 0 {
		rows, err := env.Model("ir.attachment").Browse(invoiceAttachmentID).Read("id", "name", "mimetype")
		if err != nil {
			return "", err
		}
		if len(rows) == 1 {
			items = append(items, map[string]any{
				"id":                    invoiceAttachmentID,
				"name":                  stringValue(rows[0]["name"]),
				"mimetype":              stringValue(rows[0]["mimetype"]),
				"placeholder":           false,
				"protect_from_deletion": true,
			})
		}
	} else {
		items = append(items, map[string]any{
			"id":          "placeholder_" + invoicePDFName(move),
			"name":        invoicePDFName(move),
			"mimetype":    "application/pdf",
			"placeholder": true,
		})
	}
	templateAttachmentIDs, err := accountingTemplateAttachmentIDs(env, templateID)
	if err != nil {
		return "", err
	}
	if len(templateAttachmentIDs) > 0 {
		rows, err := env.Model("ir.attachment").Browse(templateAttachmentIDs...).Read("id", "name", "mimetype")
		if err != nil {
			return "", err
		}
		for _, row := range rows {
			items = append(items, map[string]any{
				"id":                    int64Value(row["id"]),
				"name":                  stringValue(row["name"]),
				"mimetype":              stringValue(row["mimetype"]),
				"placeholder":           false,
				"mail_template_id":      templateID,
				"protect_from_deletion": true,
			})
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func accountingTemplateAttachmentIDs(env *record.Env, templateID int64) ([]int64, error) {
	if templateID == 0 {
		return nil, nil
	}
	rows, err := env.Model("mail.template").Browse(templateID).Read("attachment_ids")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return uniqueInt64Slice(int64Slice(rows[0]["attachment_ids"])), nil
}

func createAccountingTemplateReportAttachments(env *record.Env, templateID int64, moveID int64, messageID int64) ([]int64, error) {
	if templateID == 0 {
		return nil, nil
	}
	excludedReportIDs, err := accountingTemplateInvoiceReportIDs(env, templateID)
	if err != nil {
		return nil, err
	}
	return internalmail.CreateTemplateReportAttachmentsExcept(env, templateID, "account.move", moveID, "mail.message", messageID, excludedReportIDs)
}

func accountingTemplateInvoiceReportIDs(env *record.Env, templateID int64) ([]int64, error) {
	rows, err := env.Model("mail.template").Browse(templateID).Read("report_template_ids")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	reportIDs := uniqueInt64Slice(int64Slice(rows[0]["report_template_ids"]))
	if len(reportIDs) == 0 {
		return nil, nil
	}
	reportRows, err := env.Model("ir.actions.report").Browse(reportIDs...).Read("id", "report_name", "is_invoice_report")
	if err != nil {
		return nil, err
	}
	excluded := []int64{}
	for _, row := range reportRows {
		reportName := strings.TrimSpace(stringValue(row["report_name"]))
		if boolHTTPWithFallback(row["is_invoice_report"], false) || reportName == "account.report_invoice" || reportName == "account.report_invoice_document" {
			excluded = append(excluded, int64Value(row["id"]))
		}
	}
	return uniqueInt64Slice(excluded), nil
}

func defaultAccountingMailTemplateID(env *record.Env, move coreaccounting.Move) int64 {
	xmlID := "account.email_template_edi_invoice"
	if strings.Contains(move.MoveType, "refund") {
		xmlID = "account.email_template_edi_credit_note"
	}
	found, err := env.Model("ir.model.data").SearchWithOptions(domain.Cond("complete_name", "=", xmlID), record.SearchOptions{Limit: 1})
	if err != nil {
		return 0
	}
	rows, err := found.Read("model", "res_id")
	if err != nil || len(rows) == 0 || rows[0]["model"] != "mail.template" {
		return 0
	}
	return int64Value(rows[0]["res_id"])
}

func defaultAccountingMailPartnerIDs(move coreaccounting.Move) []int64 {
	if move.CommercialPartnerID != 0 {
		return []int64{move.CommercialPartnerID}
	}
	if move.PartnerID != 0 {
		return []int64{move.PartnerID}
	}
	return nil
}

func ensureInvoicePDFPlaceholder(env *record.Env, move coreaccounting.Move) (int64, error) {
	rows, err := env.Model("account.move").Browse(move.ID).Read("invoice_pdf_report_id")
	if err != nil {
		return 0, err
	}
	var attachmentID int64
	if len(rows) == 1 {
		attachmentID = int64Value(rows[0]["invoice_pdf_report_id"])
	}
	if attachmentID != 0 {
		ok, err := invoicePDFAttachmentLinkMatches(env, attachmentID, move.ID)
		if err != nil {
			return 0, err
		}
		if !ok {
			attachmentID = 0
		}
	}
	if attachmentID == 0 {
		found, err := env.Model("ir.attachment").Search(domain.And(
			domain.Cond("res_model", domain.Equal, "account.move"),
			domain.Cond("res_id", domain.Equal, move.ID),
			domain.Cond("res_field", domain.Equal, "invoice_pdf_report_file"),
		))
		if err != nil {
			return 0, err
		}
		if ids := found.IDs(); len(ids) > 0 {
			attachmentID = ids[0]
		}
	}
	if attachmentID != 0 {
		content, ok, err := backfillInvoicePDFPlaceholder(env, move, attachmentID)
		if err != nil {
			return 0, err
		}
		if ok {
			if err := env.Model("account.move").Browse(move.ID).Write(map[string]any{
				"invoice_pdf_report_id":      attachmentID,
				"invoice_pdf_report_file":    content,
				"message_main_attachment_id": attachmentID,
			}); err != nil {
				return 0, err
			}
			return attachmentID, nil
		}
	}
	attachmentID, err = createInvoicePDFPlaceholder(env, move)
	if err != nil {
		return 0, err
	}
	if err := env.Model("account.move").Browse(move.ID).Write(map[string]any{
		"invoice_pdf_report_id":      attachmentID,
		"invoice_pdf_report_file":    validInvoicePDFBytes(move),
		"message_main_attachment_id": attachmentID,
	}); err != nil {
		return 0, err
	}
	return attachmentID, nil
}

func invoicePDFAttachmentLinkMatches(env *record.Env, attachmentID int64, moveID int64) (bool, error) {
	rows, err := env.Model("ir.attachment").Browse(attachmentID).Read("res_model", "res_id", "res_field")
	if err != nil {
		return false, err
	}
	return len(rows) == 1 &&
		rows[0]["res_model"] == "account.move" &&
		int64Value(rows[0]["res_id"]) == moveID &&
		rows[0]["res_field"] == "invoice_pdf_report_file", nil
}

func backfillInvoicePDFPlaceholder(env *record.Env, move coreaccounting.Move, attachmentID int64) ([]byte, bool, error) {
	rows, err := env.Model("ir.attachment").Browse(attachmentID).Read("name", "res_model", "res_field", "res_id", "type", "mimetype", "datas", "file_size")
	if err != nil {
		return nil, false, err
	}
	if len(rows) != 1 {
		return nil, false, nil
	}
	row := rows[0]
	content := byteValue(row["datas"])
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		content = validInvoicePDFBytes(move)
	}
	values := map[string]any{}
	if stringValue(row["name"]) == "" {
		values["name"] = invoicePDFName(move)
	}
	if stringValue(row["res_model"]) != "account.move" {
		values["res_model"] = "account.move"
	}
	if stringValue(row["res_field"]) != "invoice_pdf_report_file" {
		values["res_field"] = "invoice_pdf_report_file"
	}
	if int64Value(row["res_id"]) != move.ID {
		values["res_id"] = move.ID
	}
	if int64Value(row["company_id"]) != move.CompanyID {
		values["company_id"] = move.CompanyID
	}
	if stringValue(row["type"]) != "binary" {
		values["type"] = "binary"
	}
	if stringValue(row["mimetype"]) != "application/pdf" {
		values["mimetype"] = "application/pdf"
	}
	if !bytes.Equal(byteValue(row["datas"]), content) {
		values["datas"] = content
	}
	if int64Value(row["file_size"]) != int64(len(content)) {
		values["file_size"] = len(content)
	}
	if len(values) != 0 {
		if err := env.Model("ir.attachment").Browse(attachmentID).Write(values); err != nil {
			return nil, false, err
		}
	}
	return content, true, nil
}

func invoicePDFName(move coreaccounting.Move) string {
	name := strings.TrimSpace(move.Name)
	if name == "" {
		name = fmt.Sprintf("account.move-%d", move.ID)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name += ".pdf"
	}
	return name
}

func validInvoicePDFBytes(move coreaccounting.Move) []byte {
	title := strings.TrimSpace(move.Name)
	if title == "" {
		title = fmt.Sprintf("account.move-%d", move.ID)
	}
	body := fmt.Sprintf("Invoice %s", title)
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", pdfEscape(body))
	prefix := "%PDF-1.4\n"
	obj1 := "1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n"
	obj2 := "2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n"
	obj3 := "3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n"
	obj4 := "4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n"
	obj5 := fmt.Sprintf("5 0 obj << /Length %d >> stream\n%s\nendstream endobj\n", len(stream), stream)
	parts := []string{prefix, obj1, obj2, obj3, obj4, obj5}
	var b strings.Builder
	offsets := []int{0}
	for _, part := range parts {
		offsets = append(offsets, b.Len())
		b.WriteString(part)
	}
	xrefOffset := b.Len()
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	b.WriteString(fmt.Sprintf("trailer << /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset))
	return []byte(b.String())
}

func pdfEscape(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`, "\n", " ")
	return replacer.Replace(text)
}

func createMailMessage(env *record.Env, move coreaccounting.Move, subject string, body string) (int64, error) {
	return env.Model("mail.message").Create(map[string]any{
		"subject":      subject,
		"body":         body,
		"message_type": "email",
		"model":        "account.move",
		"res_id":       move.ID,
		"date":         time.Now().UTC(),
	})
}

func createMailMessageAttachmentCopy(env *record.Env, sourceAttachmentID int64, messageID int64) (int64, error) {
	rows, err := env.Model("ir.attachment").Browse(sourceAttachmentID).Read("name", "type", "mimetype", "datas", "url", "file_size")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("invoice attachment %d not found", sourceAttachmentID)
	}
	row := rows[0]
	values := map[string]any{
		"name":      stringValue(row["name"]),
		"res_model": "mail.message",
		"res_id":    messageID,
		"type":      stringValue(row["type"]),
		"mimetype":  stringValue(row["mimetype"]),
		"datas":     row["datas"],
		"url":       stringValue(row["url"]),
		"file_size": int64Value(row["file_size"]),
	}
	if strings.TrimSpace(stringValue(values["type"])) == "" {
		values["type"] = "binary"
	}
	if strings.TrimSpace(stringValue(values["mimetype"])) == "" {
		values["mimetype"] = "application/pdf"
	}
	return env.Model("ir.attachment").Create(values)
}

func createOutgoingMail(env *record.Env, messageID int64, subject string, body string, partnerIDs []int64, attachmentIDs []int64) error {
	emailTo := "customer@example.invalid"
	if len(partnerIDs) > 0 {
		items := make([]string, 0, len(partnerIDs))
		for _, partnerID := range partnerIDs {
			items = append(items, fmt.Sprintf("partner-%d@example.invalid", partnerID))
		}
		emailTo = strings.Join(items, ",")
	}
	_, err := env.Model("mail.mail").Create(map[string]any{
		"mail_message_id": messageID,
		"email_to":        emailTo,
		"attachment_ids":  attachmentIDs,
		"subject":         subject,
		"body_html":       body,
		"state":           "outgoing",
		"max_retries":     int64(3),
	})
	return err
}

func sendActiveMoveIDs(context map[string]any) []int64 {
	if context["active_model"] != "account.move" {
		return nil
	}
	return int64Slice(context["active_ids"])
}

func defaultInvoiceSubject(move coreaccounting.Move) string {
	if move.Name != "" {
		return "Invoice " + move.Name
	}
	return fmt.Sprintf("Invoice %d", move.ID)
}

func defaultInvoiceBody(move coreaccounting.Move) string {
	if move.Name != "" {
		return "Please find attached invoice " + move.Name + "."
	}
	return fmt.Sprintf("Please find attached invoice %d.", move.ID)
}

func moveIDsFromPaymentLines(env *record.Env, lineIDs []int64) ([]int64, error) {
	if len(lineIDs) == 0 {
		return nil, nil
	}
	rows, err := env.Model("account.move.line").Browse(lineIDs...).Read("move_id")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = appendUniqueHTTPID(ids, int64Value(row["move_id"]))
	}
	return ids, nil
}

func paymentRegisterActiveMoveIDs(env *record.Env, req callKWRequest) ([]int64, error) {
	context := mapValue(kwarg(req.Kwargs, "context"))
	if len(context) == 0 {
		context = mapValue(req.Values["context"])
	}
	return paymentRegisterMoveIDsFromContext(env, context)
}

func paymentRegisterMoveIDsFromContext(env *record.Env, context map[string]any) ([]int64, error) {
	switch context["active_model"] {
	case "account.move":
		return int64Slice(context["active_ids"]), nil
	case "account.move.line":
		return moveIDsFromPaymentLines(env, int64Slice(context["active_ids"]))
	default:
		return nil, nil
	}
}

func contextBool(req callKWRequest, key string) bool {
	context := mapValue(kwarg(req.Kwargs, "context"))
	if len(context) == 0 {
		context = mapValue(req.Values["context"])
	}
	return accountingBoolValue(context[key])
}

func appendUniqueHTTPID(ids []int64, id int64) []int64 {
	if id == 0 {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func absHTTP(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func (s Server) effectiveEnv(r *http.Request) *record.Env {
	env := s.Env
	if env != nil {
		if cookieIDs := cookieCompanyIDs(r); len(cookieIDs) > 0 {
			allowed := companyIDs(env.Context())
			if selected, err := validatedSwitchCompanyIDs(allowed, cookieIDs[0], cookieIDs); err == nil {
				env = envWithCompanyContext(env, cookieIDs[0], selected)
			}
		}
	}
	return s.impersonatedEnv(env, s.impersonationSessionID(r))
}

func (s Server) impersonationSessionID(r *http.Request) string {
	if s.Security != nil {
		return cookieSessionID(r)
	}
	return loginAsSessionID(r)
}

func (s Server) loginAsSessionActor(w http.ResponseWriter, r *http.Request) (string, int64, bool) {
	if s.Security == nil {
		sessionID := loginAsSessionID(r)
		if sessionID == "" {
			http.Error(w, "missing session_id", http.StatusBadRequest)
			return "", 0, false
		}
		actorID := int64(0)
		if s.Env != nil {
			actorID = s.Env.Context().UserID
		}
		return sessionID, actorID, true
	}
	sessionID := cookieSessionID(r)
	if sessionID == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return "", 0, false
	}
	actorID, ok := s.Security.AuthenticateSession(sessionID)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return "", 0, false
	}
	user, ok := s.Security.Users[actorID]
	if !ok || !user.Active {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return "", 0, false
	}
	return sessionID, actorID, true
}

func (s Server) authenticatedRequestEnv(r *http.Request) (*record.Env, bool) {
	if s.Security == nil {
		return s.effectiveEnv(r), true
	}
	sessionID := cookieSessionID(r)
	if sessionID == "" {
		return nil, false
	}
	userID, ok := s.Security.AuthenticateSession(sessionID)
	if !ok {
		return nil, false
	}
	user, ok := s.Security.Users[userID]
	if !ok || !user.Active {
		return nil, false
	}
	env := s.envForSecurityUser(user)
	allowed := allowedCompanyIDsForUser(user)
	if cookieIDs := cookieCompanyIDs(r); len(cookieIDs) > 0 {
		if selected, err := validatedSwitchCompanyIDs(allowed, cookieIDs[0], cookieIDs); err == nil {
			env = envWithCompanyContext(env, cookieIDs[0], selected)
			return s.impersonatedEnv(env, sessionID), true
		}
	}
	if companyID, companyIDs, ok := s.Security.SessionCompanies(sessionID); ok {
		env = envWithCompanyContext(env, companyID, companyIDs)
	}
	return s.impersonatedEnv(env, sessionID), true
}

func (s Server) publicRequestEnv(r *http.Request) *record.Env {
	if s.Security == nil {
		return s.effectiveEnv(r)
	}
	if env, ok := s.authenticatedRequestEnv(r); ok {
		return env
	}
	return s.anonymousEnv()
}

func (s Server) requireWebSession(w http.ResponseWriter, r *http.Request, envelope *rpcEnvelope) (*record.Env, bool) {
	env, ok := s.authenticatedRequestEnv(r)
	if ok {
		return env, true
	}
	writeRPCError(w, envelope, http.StatusUnauthorized, errors.New("authentication required"))
	return nil, false
}

func (s Server) authenticateSecurityUser(login, password string) (security.User, bool) {
	if s.Security == nil {
		return security.User{}, false
	}
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return security.User{}, false
	}
	for _, user := range s.Security.Users {
		if !user.Active || user.Login != login || user.Password == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(user.Password), []byte(password)) == 1 {
			return user, true
		}
	}
	if user, ok := s.authenticateRecordUser(login, password); ok {
		return user, true
	}
	return security.User{}, false
}

func (s Server) authenticateRecordUser(login, password string) (security.User, bool) {
	if s.Security == nil || s.Env == nil || !modelHasField(s.Env, "res.users", "password") {
		return security.User{}, false
	}
	found, err := s.Env.Model("res.users").SearchWithOptions(domain.Cond("login", domain.Equal, login), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return security.User{}, false
	}
	fields := availableModelFields(s.Env, "res.users", "id", "login", "password", "email", "name", "active", "company_id", "company_ids", "partner_id", "commercial_partner_id", "groups_id", "group_ids", "all_group_ids")
	rows, err := found.Read(fields...)
	if err != nil || len(rows) == 0 {
		return security.User{}, false
	}
	row := rows[0]
	storedPassword := stringValue(row["password"])
	if storedPassword == "" || subtle.ConstantTimeCompare([]byte(storedPassword), []byte(password)) != 1 {
		return security.User{}, false
	}
	if active, ok := row["active"].(bool); ok && !active {
		return security.User{}, false
	}
	companyID := int64Value(row["company_id"])
	companyIDs := int64Slice(row["company_ids"])
	if len(companyIDs) == 0 && companyID != 0 {
		companyIDs = []int64{companyID}
	}
	groupIDs := uniqueInt64Slice(append(append(int64Slice(row["groups_id"]), int64Slice(row["group_ids"])...), int64Slice(row["all_group_ids"])...))
	user := security.User{
		ID:                  int64Value(row["id"]),
		Login:               firstTextHTTP(row["login"], login),
		Password:            storedPassword,
		Email:               stringValue(row["email"]),
		Name:                firstTextHTTP(row["name"], row["login"], login),
		Active:              true,
		CompanyID:           companyID,
		CompanyIDs:          companyIDs,
		PartnerID:           int64Value(row["partner_id"]),
		CommercialPartnerID: int64Value(row["commercial_partner_id"]),
		GroupIDs:            groupIDs,
	}
	s.Security.Users[user.ID] = user
	return user, true
}

func (s Server) envForSecurityUser(user security.User) *record.Env {
	baseEnv := s.baseEnvWithSecurityPolicy()
	if baseEnv == nil {
		return nil
	}
	ctx := baseEnv.Context()
	ctx.UserID = user.ID
	if user.CompanyID != 0 {
		ctx.CompanyID = user.CompanyID
	}
	if len(user.CompanyIDs) > 0 {
		ctx.CompanyIDs = append([]int64(nil), user.CompanyIDs...)
	}
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["uid"] = user.ID
	ctx.Values["all_allowed_company_ids"] = allowedCompanyIDsForUser(user)
	if user.Login != "" {
		ctx.Values["login"] = user.Login
	}
	if user.PartnerID != 0 {
		ctx.Values["partner_id"] = user.PartnerID
	}
	if len(user.GroupIDs) > 0 {
		ctx.Values["group_ids"] = append([]int64(nil), user.GroupIDs...)
	}
	return baseEnv.WithContext(ctx)
}

func (s Server) anonymousEnv() *record.Env {
	baseEnv := s.baseEnvWithSecurityPolicy()
	if baseEnv == nil {
		return nil
	}
	ctx := baseEnv.Context()
	ctx.UserID = 0
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["uid"] = int64(0)
	delete(ctx.Values, "group_ids")
	delete(ctx.Values, "login")
	delete(ctx.Values, "partner_id")
	return baseEnv.WithContext(ctx)
}

func (s Server) baseEnvWithSecurityPolicy() *record.Env {
	if s.Env == nil {
		return nil
	}
	if s.Security != nil && s.Env.Policy() == nil {
		return s.Env.WithPolicy(s.Security)
	}
	return s.Env
}

func (s Server) impersonatedEnv(env *record.Env, sessionID string) *record.Env {
	if env == nil || s.Impersonation == nil {
		return env
	}
	info, err := s.Impersonation.SessionInfo(sessionID)
	if err != nil {
		return env
	}
	ctx := env.Context()
	ctx.UserID = info.UserID
	ctx.Values = cloneContextValues(ctx.Values)
	for key, value := range info.Context {
		ctx.Values[key] = value
	}
	if user, ok := s.Impersonation.User(info.UserID); ok {
		if user.CompanyID != 0 {
			ctx.CompanyID = user.CompanyID
		}
		if len(user.CompanyIDs) > 0 {
			ctx.CompanyIDs = append([]int64(nil), user.CompanyIDs...)
		}
		if len(user.GroupIDs) > 0 {
			ctx.Values["group_ids"] = append([]int64(nil), user.GroupIDs...)
		}
	}
	if ctx.Values["uid"] == nil {
		ctx.Values["uid"] = info.UserID
	}
	return env.WithContext(ctx)
}

func (s Server) revokeWebSession(w http.ResponseWriter, r *http.Request) {
	sessionID := cookieSessionID(r)
	if sessionID != "" && s.Security != nil {
		s.Security.RevokeSession(sessionID)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cids",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}

func envWithCompanyContext(env *record.Env, companyID int64, companyIDs []int64) *record.Env {
	if env == nil {
		return nil
	}
	ctx := env.Context()
	if companyID != 0 {
		ctx.CompanyID = companyID
	}
	if len(companyIDs) > 0 {
		ctx.CompanyIDs = append([]int64(nil), companyIDs...)
	}
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["allowed_company_ids"] = append([]int64(nil), ctx.CompanyIDs...)
	ctx.Values["company_id"] = ctx.CompanyID
	return env.WithContext(ctx)
}

func allowedCompanyIDsForUser(user security.User) []int64 {
	ids := append([]int64{user.CompanyID}, user.CompanyIDs...)
	return uniqueInt64Slice(ids)
}

func validatedSwitchCompanyIDs(allowed []int64, companyID int64, requested []int64) ([]int64, error) {
	allowed = uniqueInt64Slice(allowed)
	if !containsHTTPInt64(allowed, companyID) {
		return nil, fmt.Errorf("company %d is not allowed", companyID)
	}
	selected := uniqueInt64Slice(requested)
	if len(selected) == 0 {
		selected = append([]int64(nil), allowed...)
	}
	for _, id := range selected {
		if !containsHTTPInt64(allowed, id) {
			return nil, fmt.Errorf("company %d is not allowed", id)
		}
	}
	if !containsHTTPInt64(selected, companyID) {
		selected = append([]int64{companyID}, selected...)
	}
	return orderedCompanyIDs(companyID, selected), nil
}

func newSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s Server) dispatchMailThreadMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	switch req.Method {
	case "message_process":
		fallbackModel := firstTextHTTP(kwarg(req.Kwargs, "model"), arg(req.Args, 0))
		raw := firstTextHTTP(
			kwarg(req.Kwargs, "message"),
			kwarg(req.Kwargs, "message_string"),
			kwarg(req.Kwargs, "raw_message"),
		)
		if raw == "" {
			if len(req.Args) > 1 {
				raw = stringValue(arg(req.Args, 1))
			} else {
				raw = stringValue(arg(req.Args, 0))
			}
		}
		if strings.TrimSpace(raw) == "" {
			return nil, true, fmt.Errorf("message_process requires raw message")
		}
		result, err := internalmail.ProcessInboundEmailWithOptions(env, []byte(raw), internalmail.InboundProcessOptions{
			FallbackModel:    fallbackModel,
			ThreadID:         int64Value(firstNonNil(kwarg(req.Kwargs, "thread_id"), arg(req.Args, 5))),
			CustomValues:     mapValue(firstNonNil(kwarg(req.Kwargs, "custom_values"), arg(req.Args, 2))),
			SaveOriginal:     accountingBoolValue(firstNonNil(kwarg(req.Kwargs, "save_original"), arg(req.Args, 3))),
			StripAttachments: accountingBoolValue(firstNonNil(kwarg(req.Kwargs, "strip_attachments"), arg(req.Args, 4))),
			MessageIDLocker:  s.InboundMessageLock,
		})
		if err != nil {
			return nil, true, err
		}
		if result.Routed && result.ResID != 0 {
			return result.ResID, true, nil
		}
		return false, true, nil
	case "message_post":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids")))
		resID := int64Value(kwarg(req.Kwargs, "res_id"))
		if len(ids) > 0 {
			resID = ids[0]
		}
		context := mapValue(kwarg(req.Kwargs, "context"))
		_, attachmentIDsSet := req.Kwargs["attachment_ids"]
		_, attachmentTokensSet := req.Kwargs["attachment_tokens"]
		partnerIDs, err := mailPostPartnerIDs(env, req.Kwargs)
		if err != nil {
			return nil, true, err
		}
		id, err := internalmail.PostMessage(env, internalmail.PostRequest{
			Model:               req.Model,
			ResID:               resID,
			Body:                stringValue(kwarg(req.Kwargs, "body")),
			Subject:             stringValue(kwarg(req.Kwargs, "subject")),
			MessageType:         stringValue(kwarg(req.Kwargs, "message_type")),
			EmailFrom:           stringValue(kwarg(req.Kwargs, "email_from")),
			AuthorID:            int64Value(kwarg(req.Kwargs, "author_id")),
			ParentID:            int64Value(kwarg(req.Kwargs, "parent_id")),
			AccessToken:         stringValue(kwarg(req.Kwargs, "token")),
			AccessHash:          stringValue(kwarg(req.Kwargs, "hash")),
			AccessPID:           int64Value(kwarg(req.Kwargs, "pid")),
			ProjectSharingID:    int64Value(kwarg(req.Kwargs, "project_sharing_id")),
			SubtypeXMLID:        stringValue(kwarg(req.Kwargs, "subtype_xmlid")),
			SubtypeID:           int64Value(kwarg(req.Kwargs, "subtype_id")),
			PartnerIDs:          partnerIDs,
			AttachmentIDs:       int64Slice(kwarg(req.Kwargs, "attachment_ids")),
			AttachmentIDsSet:    attachmentIDsSet,
			AttachmentTokens:    stringSlice(kwarg(req.Kwargs, "attachment_tokens")),
			AttachmentTokensSet: attachmentTokensSet,
			TrackingValues:      trackingValuesFromAny(kwarg(req.Kwargs, "tracking_value_ids")),
			BodyIsHTML:          accountingBoolValue(kwarg(req.Kwargs, "body_is_html")),
			AutoFollow:          accountingBoolValue(context["mail_post_autofollow"]),
		})
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"id": id}, true, nil
	case "message_subscribe":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids")))
		if len(ids) == 0 {
			return nil, true, fmt.Errorf("message_subscribe requires record ids")
		}
		for _, id := range ids {
			if err := internalmail.Subscribe(env, req.Model, id, int64Slice(kwarg(req.Kwargs, "partner_ids")), int64Slice(kwarg(req.Kwargs, "subtype_ids"))); err != nil {
				return nil, true, err
			}
		}
		return true, true, nil
	case "message_unsubscribe":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids")))
		if len(ids) == 0 {
			return nil, true, fmt.Errorf("message_unsubscribe requires record ids")
		}
		for _, id := range ids {
			if err := internalmail.Unsubscribe(env, req.Model, id, int64Slice(kwarg(req.Kwargs, "partner_ids"))); err != nil {
				return nil, true, err
			}
		}
		return true, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchMailActivityMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	env = requestContextEnv(env, req)
	contextValues := env.Context().Values
	switch req.Method {
	case "activity_format":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		result, err := internalmail.ActivityFormat(
			env,
			int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])),
		)
		return result, true, err
	case "get_activity_data":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		node, err := parseDomain(firstNonNil(kwarg(req.Kwargs, "domain"), arg(req.Args, 1)))
		if err != nil {
			return nil, true, err
		}
		result, err := internalmail.GetActivityData(
			env,
			stringValue(firstNonNil(kwarg(req.Kwargs, "res_model"), arg(req.Args, 0))),
			node,
			internalmail.ActivityDataOptions{
				Limit:     intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 2))),
				Offset:    intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 3))),
				FetchDone: boolKwargDefault(req.Kwargs, "fetch_done", accountingBoolValue(arg(req.Args, 4))),
			},
		)
		return result, true, err
	case "action_feedback":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		result, err := internalmail.ActivityActionFeedback(
			env,
			int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])),
			stringValue(firstNonNil(kwarg(req.Kwargs, "feedback"), arg(req.Args, 1))),
			int64Slice(firstNonNil(kwarg(req.Kwargs, "attachment_ids"), arg(req.Args, 2))),
		)
		return result, true, err
	case "action_feedback_schedule_next", "action_done_schedule_next":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		result, err := internalmail.ActivityActionFeedbackScheduleNext(
			env,
			int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])),
			stringValue(firstNonNil(kwarg(req.Kwargs, "feedback"), arg(req.Args, 1))),
			int64Slice(firstNonNil(kwarg(req.Kwargs, "attachment_ids"), arg(req.Args, 2))),
		)
		return result, true, err
	case "action_reschedule_today", "action_reschedule_tomorrow", "action_reschedule_nextweek":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		if err := internalmail.ActivityReschedule(
			env,
			int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])),
			mailActivityRescheduleDeadline(req.Method),
		); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "action_cancel":
		if req.Model != "mail.activity" {
			return nil, false, nil
		}
		if err := internalmail.ActivityCancel(
			env,
			int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])),
		); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "activity_schedule":
		automated := true
		if _, ok := req.Kwargs["automated"]; ok {
			automated = accountingBoolValue(kwarg(req.Kwargs, "automated"))
		}
		activityIDs, err := internalmail.ScheduleActivity(env, internalmail.ActivityScheduleRequest{
			Model:             req.Model,
			ResIDs:            int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids"))),
			ActivityTypeXMLID: stringValue(firstNonNil(kwarg(req.Kwargs, "act_type_xmlid"), arg(req.Args, 1))),
			ActivityTypeID:    int64Value(kwarg(req.Kwargs, "activity_type_id")),
			RecommendedTypeID: int64Value(firstNonNil(kwarg(req.Kwargs, "recommended_activity_type_id"), kwarg(req.Kwargs, "default_recommended_activity_type_id"), contextValues["default_recommended_activity_type_id"])),
			DateDeadline:      firstNonNil(kwarg(req.Kwargs, "date_deadline"), arg(req.Args, 2)),
			Summary:           stringValue(firstNonNil(kwarg(req.Kwargs, "summary"), arg(req.Args, 3))),
			Note:              stringValue(firstNonNil(kwarg(req.Kwargs, "note"), arg(req.Args, 4))),
			UserID:            int64Value(firstNonNil(kwarg(req.Kwargs, "user_id"), arg(req.Args, 5))),
			Automated:         automated,
			PreviousTypeID:    int64Value(firstNonNil(kwarg(req.Kwargs, "previous_activity_type_id"), kwarg(req.Kwargs, "default_previous_activity_type_id"), contextValues["default_previous_activity_type_id"])),
		})
		if err != nil {
			return nil, true, err
		}
		return activityIDs, true, nil
	case "activity_feedback":
		ok, err := internalmail.ActivityFeedback(env, internalmail.ActivitySelectRequest{
			Model:              req.Model,
			ResIDs:             int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids"))),
			ActivityTypeXMLIDs: stringSlice(firstNonNil(kwarg(req.Kwargs, "act_type_xmlids"), arg(req.Args, 1))),
			UserID:             int64Value(firstNonNil(kwarg(req.Kwargs, "user_id"), arg(req.Args, 2))),
			OnlyAutomated:      boolKwargDefault(req.Kwargs, "only_automated", true),
		}, stringValue(firstNonNil(kwarg(req.Kwargs, "feedback"), arg(req.Args, 3))), int64Slice(kwarg(req.Kwargs, "attachment_ids")))
		return ok, true, err
	case "activity_reschedule":
		activityIDs, ok, err := internalmail.ActivityRescheduleSelected(env, internalmail.ActivitySelectRequest{
			Model:              req.Model,
			ResIDs:             int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids"))),
			ActivityTypeXMLIDs: stringSlice(firstNonNil(kwarg(req.Kwargs, "act_type_xmlids"), arg(req.Args, 1))),
			UserID:             int64Value(firstNonNil(kwarg(req.Kwargs, "user_id"), arg(req.Args, 2))),
			OnlyAutomated:      boolKwargDefault(req.Kwargs, "only_automated", true),
		}, firstNonNil(kwarg(req.Kwargs, "date_deadline"), arg(req.Args, 3)), int64Value(firstNonNil(kwarg(req.Kwargs, "new_user_id"), arg(req.Args, 4))))
		if !ok || err != nil {
			return ok, true, err
		}
		return activityIDs, true, nil
	case "activity_unlink":
		ok, err := internalmail.ActivityUnlink(env, internalmail.ActivitySelectRequest{
			Model:              req.Model,
			ResIDs:             int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "res_ids"))),
			ActivityTypeXMLIDs: stringSlice(firstNonNil(kwarg(req.Kwargs, "act_type_xmlids"), arg(req.Args, 1))),
			UserID:             int64Value(firstNonNil(kwarg(req.Kwargs, "user_id"), arg(req.Args, 2))),
			OnlyAutomated:      boolKwargDefault(req.Kwargs, "only_automated", true),
		})
		return ok, true, err
	default:
		return nil, false, nil
	}
}

func mailActivityRescheduleDeadline(method string) string {
	now := time.Now().UTC()
	switch method {
	case "action_reschedule_tomorrow":
		return now.AddDate(0, 0, 1).Format("2006-01-02")
	case "action_reschedule_nextweek":
		return now.AddDate(0, 0, 7).Format("2006-01-02")
	default:
		return now.Format("2006-01-02")
	}
}

func (s Server) dispatchReportActionMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "ir.actions.report" {
		return nil, false, nil
	}
	switch req.Method {
	case "get_valid_action_reports":
		actions := s.reportActionRequests(arg(req.Args, 0))
		modelName := stringValue(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "model")))
		recordIDs := int64Slice(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "record_ids"), kwarg(req.Kwargs, "res_ids")))
		valid := make([]any, 0, len(actions))
		for _, action := range actions {
			if action.ID == 0 {
				continue
			}
			rows, err := env.Model("ir.actions.report").Browse(action.ID).Read("id", "domain")
			if err != nil {
				return nil, true, err
			}
			if len(rows) == 0 || int64Value(rows[0]["id"]) == 0 {
				continue
			}
			domainValue := strings.TrimSpace(stringValue(rows[0]["domain"]))
			if domainValue == "" {
				valid = append(valid, action.ReturnValue)
				continue
			}
			if modelName == "" || len(recordIDs) == 0 {
				continue
			}
			ok, err := reportDomainMatchesAny(env, modelName, recordIDs, domainValue)
			if err != nil {
				return nil, true, err
			}
			if ok {
				valid = append(valid, action.ReturnValue)
			}
		}
		return valid, true, nil
	default:
		return nil, false, nil
	}
}

type reportActionRequest struct {
	ID          int64
	ReturnValue any
}

func (s Server) reportActionRequests(value any) []reportActionRequest {
	items := []any{}
	switch typed := value.(type) {
	case []any:
		items = typed
	case []int64:
		for _, id := range typed {
			items = append(items, id)
		}
	case []int:
		for _, id := range typed {
			items = append(items, id)
		}
	default:
		if value != nil {
			items = append(items, value)
		}
	}
	out := make([]reportActionRequest, 0, len(items))
	for _, item := range items {
		if modelName, id, ok := s.concreteActionRef(stringValue(item)); ok && modelName == "ir.actions.report" {
			out = append(out, reportActionRequest{ID: id, ReturnValue: item})
			continue
		}
		id := int64Value(item)
		if id > 0 {
			out = append(out, reportActionRequest{ID: id, ReturnValue: id})
		}
	}
	return out
}

func reportDomainMatchesAny(env *record.Env, modelName string, recordIDs []int64, domainText string) (bool, error) {
	evaluated, err := data.SafeEvalExpression(domainText, data.SafeEvalOptions{
		Env:       env,
		Model:     modelName,
		RecordIDs: recordIDs,
		Locals: map[string]any{
			"active_ids": recordIDs,
		},
	})
	if err != nil {
		return false, fmt.Errorf("parse report domain: %w", err)
	}
	evaluated = normalizeReportDomainValue(evaluated)
	node, err := parseDomain(evaluated)
	if err != nil {
		return false, err
	}
	count, err := env.Model(modelName).SearchCount(domain.And(domain.Cond("id", domain.In, recordIDs), node), 1)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func normalizeReportDomainValue(value any) any {
	switch typed := value.(type) {
	case nil, string, bool, int, int64, float64:
		return value
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeReportDomainValue(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeReportDomainValue(item)
		}
		return out
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, 0, reflected.Len())
		for i := 0; i < reflected.Len(); i++ {
			out = append(out, normalizeReportDomainValue(reflected.Index(i).Interface()))
		}
		return out
	default:
		return value
	}
}

func (s Server) dispatchMailMessageMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "mail.message" {
		return nil, false, nil
	}
	switch req.Method {
	case "create":
		rawValues := req.Values
		if rawValues == nil {
			rawValues = mapArg(req.Args, 0)
		}
		values := withoutComputedMailMessageFields(rawValues)
		id, err := env.Model(req.Model).Create(values)
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"id": id}, true, nil
	case "read":
		ids := int64Slice(arg(req.Args, 0))
		fields := stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "fields")))
		rows, err := internalmail.ReadMessagesWithComputedStarred(env, ids, fields)
		return rows, true, err
	case "write":
		ids := int64Slice(arg(req.Args, 0))
		values := withoutComputedMailMessageFields(mapArg(req.Args, 1))
		return true, true, env.Model(req.Model).Browse(ids...).Write(values)
	case "search":
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain")))
		if err != nil {
			return nil, true, err
		}
		found, err := env.Model(req.Model).SearchWithOptions(internalmail.StarredSearchDomain(env, node), record.SearchOptions{
			Offset: intValue(kwarg(req.Kwargs, "offset")),
			Limit:  intValue(kwarg(req.Kwargs, "limit")),
			Order:  stringValue(kwarg(req.Kwargs, "order")),
		})
		if err != nil {
			return nil, true, err
		}
		return found.IDs(), true, nil
	case "search_count":
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain")))
		if err != nil {
			return nil, true, err
		}
		count, err := env.Model(req.Model).SearchCount(internalmail.StarredSearchDomain(env, node), intValue(kwarg(req.Kwargs, "limit")))
		return count, true, err
	case "search_read":
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain")))
		if err != nil {
			return nil, true, err
		}
		fields := stringSlice(firstNonNil(kwarg(req.Kwargs, "fields"), arg(req.Args, 1)))
		found, err := env.Model(req.Model).SearchWithOptions(internalmail.StarredSearchDomain(env, node), record.SearchOptions{
			Offset: intValue(kwarg(req.Kwargs, "offset")),
			Limit:  intValue(kwarg(req.Kwargs, "limit")),
			Order:  stringValue(kwarg(req.Kwargs, "order")),
		})
		if err != nil {
			return nil, true, err
		}
		rows, err := internalmail.ReadMessagesWithComputedStarred(env, found.IDs(), fields)
		return rows, true, err
	case "toggle_message_starred":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "message_id"), req.Values["ids"]))
		if len(ids) == 0 {
			return nil, true, fmt.Errorf("toggle_message_starred requires message id")
		}
		result, err := internalmail.ToggleMessageStarred(env, ids[0])
		if err == nil {
			s.publishUserBus(env, "mail.message/toggle_star", map[string]any{
				"message_ids": []int64{ids[0]},
				"starred":     mailMessageStarredResult(result),
			})
		}
		return result, true, err
	case "unstar_all":
		messageIDs, err := internalmail.UnstarAllMessages(env)
		if err == nil {
			s.publishUserBus(env, "mail.message/toggle_star", map[string]any{
				"message_ids": messageIDs,
				"starred":     false,
			})
		}
		return nil, true, err
	default:
		return nil, false, nil
	}
}

func withoutComputedMailMessageFields(value any) map[string]any {
	values := cloneHTTPMap(mapValue(value))
	delete(values, "starred")
	return values
}

func (s Server) dispatchMailTemplateMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "mail.template" {
		return nil, false, nil
	}
	switch req.Method {
	case "send_mail":
		templateIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "template_id")))
		resID := int64Value(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "res_id")))
		if len(templateIDs) == 0 {
			return nil, true, fmt.Errorf("send_mail requires template id")
		}
		mailIDs, err := internalmail.SendTemplateBatch(env, internalmail.TemplateSendRequest{
			TemplateID:  templateIDs[0],
			ResIDs:      []int64{resID},
			EmailValues: mapValue(kwarg(req.Kwargs, "email_values")),
		})
		if err != nil {
			return nil, true, err
		}
		if len(mailIDs) == 0 {
			return int64(0), true, nil
		}
		return mailIDs[0], true, nil
	case "send_mail_batch":
		templateIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "template_id")))
		resIDs := int64Slice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "res_ids")))
		if len(templateIDs) == 0 {
			return nil, true, fmt.Errorf("send_mail_batch requires template id")
		}
		mailIDs, err := internalmail.SendTemplateBatch(env, internalmail.TemplateSendRequest{
			TemplateID:  templateIDs[0],
			ResIDs:      resIDs,
			EmailValues: mapValue(kwarg(req.Kwargs, "email_values")),
		})
		return mailIDs, true, err
	case "generate_email":
		templateIDs := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "template_id")))
		resID := int64Value(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "res_id")))
		if len(templateIDs) == 0 {
			return nil, true, fmt.Errorf("generate_email requires template id")
		}
		rendered, err := internalmail.RenderTemplateForRecord(env, templateIDs[0], stringValue(kwarg(req.Kwargs, "model")), resID, mapValue(kwarg(req.Kwargs, "email_values")))
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"subject": rendered.Subject, "body_html": rendered.Body, "email_to": rendered.To, "email_cc": rendered.CC}, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchMailComposeMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "mail.compose.message" {
		return nil, false, nil
	}
	switch req.Method {
	case "action_send_mail", "action_send":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		mailIDs, err := internalmail.SendComposeMessages(env, ids, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		result := map[string]any{"type": "ir.actions.act_window_close", "mail_ids": mailIDs}
		if s.Workflow != nil {
			continuation, handled, err := s.Workflow.CompleteMailComposeTransition(env, ids, mapValue(kwarg(req.Kwargs, "context")))
			if err != nil {
				return nil, true, err
			}
			if handled && continuation != nil {
				return continuation, true, nil
			}
		}
		return result, true, nil
	case "action_schedule_message":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		scheduledIDs, err := internalmail.ScheduleComposeMessages(env, ids, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"type": "ir.actions.act_window_close", "scheduled_message_ids": scheduledIDs}, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchSMSComposerMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "sms.composer" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Method {
	case "action_send_sms":
		_, err := internalmail.SendSMSComposer(env, ids, false, time.Now().UTC())
		return false, true, err
	case "action_send_sms_mass_now":
		_, err := internalmail.SendSMSComposer(env, ids, true, time.Now().UTC())
		return false, true, err
	case "_action_send_sms":
		smsIDs, err := internalmail.SendSMSComposer(env, ids, false, time.Now().UTC())
		return smsIDs, true, err
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchMassMailingMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "mailing.mailing" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Method {
	case "action_send_mail", "_action_send_mail":
		resIDs := int64Slice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "res_ids")))
		result, err := internalmail.SendMassMailings(env, ids, resIDs, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"type": "ir.actions.act_window_close", "mail_ids": result.MailIDs, "mailing_ids": result.MailingIDs}, true, nil
	case "_process_mass_mailing_queue", "process_mass_mailing_queue":
		result, err := internalmail.ProcessMassMailingQueue(env, time.Now().UTC())
		return map[string]any{"mail_ids": result.MailIDs, "mailing_ids": result.MailingIDs, "done": result.Done, "skipped": result.Skipped}, true, err
	case "action_launch":
		return true, true, internalmail.LaunchMassMailings(env, ids)
	case "action_put_in_queue":
		return true, true, internalmail.QueueMassMailings(env, ids)
	case "action_test":
		return map[string]any{
			"name":      "Test Mailing",
			"type":      "ir.actions.act_window",
			"view_mode": "form",
			"res_model": "mailing.mailing.test",
			"target":    "new",
			"context":   map[string]any{"default_mass_mailing_id": firstID(ids), "dialog_size": "medium"},
		}, true, nil
	case "action_schedule":
		queued, err := internalmail.ScheduleMassMailings(env, ids, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		if queued {
			return true, true, nil
		}
		return map[string]any{
			"name":      "Schedule Mailing",
			"type":      "ir.actions.act_window",
			"view_mode": "form",
			"res_model": "mailing.mailing.schedule.date",
			"target":    "new",
			"context":   map[string]any{"default_mass_mailing_id": firstID(ids), "dialog_size": "medium"},
		}, true, nil
	case "action_cancel":
		return true, true, internalmail.CancelMassMailings(env, ids)
	case "action_retry_failed":
		return true, true, internalmail.RetryFailedMassMailings(env, ids)
	case "action_send_winner_mailing":
		result, err := internalmail.SendABWinnerMassMailing(env, ids, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		return map[string]any{
			"name":      "A/B Test Winner",
			"type":      "ir.actions.act_window",
			"view_mode": "form",
			"res_model": "mailing.mailing",
			"res_id":    result.WinnerMailingID,
		}, true, nil
	case "action_select_as_winner":
		result, err := internalmail.SelectABWinnerMassMailing(env, ids, time.Now().UTC())
		if err != nil {
			return nil, true, err
		}
		return map[string]any{
			"name":      "A/B Test Winner",
			"type":      "ir.actions.act_window",
			"view_mode": "form",
			"res_model": "mailing.mailing",
			"res_id":    result.WinnerMailingID,
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchDigestMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "digest.digest" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Method {
	case "action_send":
		result, err := internalmail.SendDigests(env, ids, time.Now().UTC(), true)
		return map[string]any{
			"type":        "ir.actions.act_window_close",
			"digest_ids":  result.DigestIDs,
			"mail_ids":    result.MailIDs,
			"sent":        result.Sent,
			"skipped":     result.Skipped,
			"slowed_down": result.SlowedDown,
		}, true, err
	case "action_send_manual":
		result, err := internalmail.SendDigests(env, ids, time.Now().UTC(), false)
		return map[string]any{
			"type":        "ir.actions.act_window_close",
			"digest_ids":  result.DigestIDs,
			"mail_ids":    result.MailIDs,
			"sent":        result.Sent,
			"skipped":     result.Skipped,
			"slowed_down": result.SlowedDown,
		}, true, err
	case "action_set_periodicity":
		periodicity := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "periodicity"))))
		if periodicity == "" {
			periodicity = "weekly"
		}
		if !digestPeriodicityAllowed(periodicity) {
			return nil, true, fmt.Errorf("invalid periodicity set on digest")
		}
		return true, true, env.Model("digest.digest").Browse(ids...).Write(map[string]any{"periodicity": periodicity})
	case "_cron_send_digest_email":
		result, err := internalmail.SendDueDigests(env, time.Now().UTC())
		return map[string]any{
			"digest_ids":  result.DigestIDs,
			"mail_ids":    result.MailIDs,
			"sent":        result.Sent,
			"skipped":     result.Skipped,
			"slowed_down": result.SlowedDown,
		}, true, err
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchMassMailingWizardMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Model {
	case "mailing.mailing.schedule.date":
		if req.Method != "action_schedule_date" {
			return nil, false, nil
		}
		rows, err := env.Model(req.Model).Browse(ids...).Read("schedule_date", "mass_mailing_id")
		if err != nil {
			return nil, true, err
		}
		for _, row := range rows {
			if err := internalmail.ScheduleMassMailingAt(env, int64Value(row["mass_mailing_id"]), accountingDateValue(row["schedule_date"])); err != nil {
				return nil, true, err
			}
		}
		return true, true, nil
	case "mailing.mailing.test":
		if req.Method != "send_mail_test" {
			return nil, false, nil
		}
		result, err := internalmail.SendMassMailingTests(env, ids, time.Now().UTC())
		return map[string]any{"mail_ids": result.MailIDs, "invalid": result.Invalid}, true, err
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchMailMailMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "mail.mail" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), kwarg(req.Kwargs, "email_ids"), req.Values["ids"]))
	switch req.Method {
	case "process_email_queue":
		batchSize := int(int64Value(firstNonNil(kwarg(req.Kwargs, "batch_size"), arg(req.Args, 0))))
		emailIDs := int64Slice(firstNonNil(kwarg(req.Kwargs, "email_ids"), kwarg(req.Kwargs, "ids")))
		result, err := internalmail.ProcessEmailQueue(context.Background(), env, s.MailSender, internalmail.QueueOptions{EmailIDs: emailIDs, BatchSize: batchSize, Now: time.Now().UTC()})
		return map[string]any{"processed": result.Processed, "sent": result.Sent, "failed": result.Failed, "skipped": result.Skipped}, true, err
	case "send":
		result, err := internalmail.SendMails(context.Background(), env, s.MailSender, ids, time.Now().UTC())
		return map[string]any{"processed": result.Processed, "sent": result.Sent, "failed": result.Failed, "skipped": result.Skipped}, true, err
	case "action_retry":
		if len(ids) == 0 {
			ids = int64Slice(kwarg(req.Kwargs, "email_ids"))
		}
		if err := internalmail.RetryMails(env, ids); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "cancel":
		if err := internalmail.CancelMails(env, ids); err != nil {
			return nil, true, err
		}
		return true, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchFetchmailServerMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "fetchmail.server" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Method {
	case "button_confirm_login":
		if err := internalmail.ConfirmFetchmailServers(context.Background(), env, s.FetchmailConnector, ids, time.Now().UTC()); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "set_draft":
		if err := env.Model("fetchmail.server").Browse(ids...).Write(map[string]any{"state": "draft"}); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "fetch_mail":
		result, err := internalmail.ProcessFetchmailServers(context.Background(), env, s.FetchmailConnector, internalmail.FetchmailOptions{
			ServerIDs:       ids,
			Now:             time.Now().UTC(),
			MessageIDLocker: s.InboundMessageLock,
			ServerLocker:    s.FetchmailServerLock,
		})
		return map[string]any{"servers": result.Servers, "checked": result.Checked, "fetched": result.Fetched, "processed": result.Processed, "failed": result.Failed, "skipped": result.Skipped, "remaining": result.Remaining}, true, err
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchDelegationMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "delegation" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	records := env.Model("delegation").Browse(ids...)
	switch req.Method {
	case "_clear_access_cache":
		return true, true, persistDelegationCacheEventHTTP(env, "clear_access_cache", time.Now().UTC())
	case "action_confirm":
		if err := records.ActionConfirmDelegation(); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "action_submit":
		if err := records.ActionSubmitDelegation(); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "action_revoked", "action_revoke":
		if err := records.ActionRevokeDelegation(); err != nil {
			return nil, true, err
		}
		return true, true, nil
	case "expire_delegation":
		if err := records.ExpireDelegation(); err != nil {
			return nil, true, err
		}
		return true, true, nil
	default:
		return nil, false, nil
	}
}

func persistDelegationCacheEventHTTP(env *record.Env, reason string, at time.Time) error {
	if env == nil {
		return nil
	}
	if _, ok := env.ModelMetadata("delegation.cache.event"); !ok {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := env.Model("delegation.cache.event").Create(map[string]any{
		"user_ids":   "[]",
		"reason":     reason,
		"created_at": at.UTC().Format(time.RFC3339Nano),
	})
	return err
}

func (s Server) executeCallKW(env *record.Env, req callKWRequest) (any, error) {
	if result, handled, err := s.dispatchResUsersMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchResPartnerMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchResGroupsMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchSequenceMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchModuleMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchAIModelMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailTemplateMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailComposeMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchSMSComposerMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMassMailingMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchDigestMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMassMailingWizardMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchLinkTrackerMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchWhatsAppAccountMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchWhatsAppTemplateMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailMailMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchFetchmailServerMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchDelegationMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchLoginAsMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailActivityMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailMessageMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchMailThreadMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchReportActionMethod(env, req); handled {
		return result, err
	}
	switch req.Method {
	case "create":
		if records := mapList(arg(req.Args, 0)); len(records) > 0 {
			ids := make([]int64, 0, len(records))
			modelSet := createContextEnv(env, req).Model(req.Model)
			for _, values := range records {
				id, err := modelSet.Create(values)
				if err != nil {
					return nil, err
				}
				ids = append(ids, id)
			}
			return ids, nil
		}
		values := req.Values
		if values == nil {
			values = mapArg(req.Args, 0)
		}
		id, err := createContextEnv(env, req).Model(req.Model).Create(values)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id}, nil
	case "read":
		readEnv := requestContextEnv(env, req)
		ids := int64Slice(arg(req.Args, 0))
		fields := stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "fields")))
		rows, err := readEnv.Model(req.Model).Browse(ids...).Read(fields...)
		if err != nil {
			return nil, err
		}
		return rows, nil
	case "web_read":
		readEnv := requestContextEnv(env, req)
		ids := int64Slice(arg(req.Args, 0))
		return webReadWithComputedWorkflow(readEnv, req.Model, ids, mapValue(kwarg(req.Kwargs, "specification")))
	case "write":
		ids := int64Slice(arg(req.Args, 0))
		values := mapArg(req.Args, 1)
		return true, env.Model(req.Model).Browse(ids...).Write(values)
	case "web_save":
		ids := int64Slice(arg(req.Args, 0))
		values := mapArg(req.Args, 1)
		modelSet := env.Model(req.Model)
		if len(ids) == 0 {
			id, err := createContextEnv(env, req).Model(req.Model).Create(values)
			if err != nil {
				return nil, err
			}
			ids = []int64{id}
		} else if err := modelSet.Browse(ids...).Write(values); err != nil {
			return nil, err
		}
		if nextID := int64Value(kwarg(req.Kwargs, "next_id")); nextID > 0 {
			ids = []int64{nextID}
		}
		return webReadWithComputedWorkflow(env, req.Model, ids, mapValue(kwarg(req.Kwargs, "specification")))
	case "web_save_multi":
		ids := int64Slice(arg(req.Args, 0))
		values := mapList(arg(req.Args, 1))
		if len(ids) != len(values) {
			return nil, fmt.Errorf("web_save_multi requires one values map per id")
		}
		modelSet := env.Model(req.Model)
		for index, id := range ids {
			if err := modelSet.Browse(id).Write(values[index]); err != nil {
				return nil, err
			}
		}
		return webReadWithComputedWorkflow(env, req.Model, ids, mapValue(kwarg(req.Kwargs, "specification")))
	case "copy":
		ids := positiveInt64Slice(int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])))
		defaults := mapValue(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "default")))
		newIDs, err := copyModelRecords(env, req.Model, ids, defaults)
		if err != nil {
			return nil, err
		}
		return newIDs, nil
	case "action_archive":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		return true, setModelActiveFlag(env, req.Model, ids, false)
	case "action_unarchive":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		return true, setModelActiveFlag(env, req.Model, ids, true)
	case "unlink":
		ids := int64Slice(arg(req.Args, 0))
		return true, env.Model(req.Model).Browse(ids...).Unlink()
	case "export_data":
		return exportModelData(env, req)
	case "fields_get":
		fields := stringSlice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "allfields"), kwarg(req.Kwargs, "fields")))
		attributes := stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "attributes")))
		return env.Model(req.Model).FieldsGet(fields, attributes)
	case "get_views":
		return s.getViews(requestContextEnv(env, req), req.Model, firstNonNil(kwarg(req.Kwargs, "views"), arg(req.Args, 0)), mapValue(kwarg(req.Kwargs, "options")))
	case "default_get":
		fields := stringSlice(arg(req.Args, 0))
		if req.Model == "account.move.reversal" {
			return accountingReversalDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "account.payment.register" {
			return accountingPaymentRegisterDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "account.move.send.wizard" {
			return accountingMoveSendDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "account.move.send.batch.wizard" {
			return accountingMoveSendBatchDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "mail.compose.message" {
			return internalmail.ComposeDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "sms.composer" {
			return internalmail.SMSComposerDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == internalworkflow.ModelWorkflowWizard {
			return internalworkflow.WorkflowWizardDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == internalworkflow.ModelStateUpdateWizard {
			return internalworkflow.StateUpdateWizardDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "login.as" {
			return loginAsDefaultGet(env, fields, mapValue(kwarg(req.Kwargs, "context")))
		}
		return env.Model(req.Model).DefaultGet(fields, mapValue(kwarg(req.Kwargs, "context")))
	case "name_get":
		ids := int64Slice(arg(req.Args, 0))
		return env.Model(req.Model).Browse(ids...).NameGet()
	case "name_search":
		readEnv := requestContextEnv(env, req)
		name := stringValue(firstNonNil(kwarg(req.Kwargs, "name"), arg(req.Args, 0)))
		node, err := parseDomain(firstNonNil(kwarg(req.Kwargs, "domain"), kwarg(req.Kwargs, "args"), arg(req.Args, 1)))
		if err != nil {
			return nil, err
		}
		op, err := domain.NormalizeOperator(stringValue(firstNonNil(kwarg(req.Kwargs, "operator"), arg(req.Args, 2), "ilike")))
		if err != nil {
			return nil, err
		}
		limit := intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 3), 100))
		pairs, err := readEnv.Model(req.Model).NameSearch(name, node, op, limit)
		if err != nil {
			return nil, err
		}
		return pairs, nil
	case "name_create":
		createEnv := createContextEnv(env, req)
		name := strings.TrimSpace(stringValue(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "name"))))
		if name == "" {
			return nil, fmt.Errorf("name_create requires a name")
		}
		createNameField := strings.TrimSpace(stringValue(firstNonNil(kwarg(req.Kwargs, "create_name_field"), "name")))
		if !validSimpleFieldName(createNameField) {
			return nil, fmt.Errorf("invalid name_create field %q", createNameField)
		}
		fields, err := createEnv.Model(req.Model).FieldsGet([]string{createNameField}, []string{"type"})
		if err != nil {
			return nil, err
		}
		if _, ok := fields[createNameField]; !ok {
			return nil, fmt.Errorf("unknown name_create field %s on %s", createNameField, req.Model)
		}
		id, err := createEnv.Model(req.Model).Create(map[string]any{createNameField: name})
		if err != nil {
			return nil, err
		}
		pairs, err := createEnv.Model(req.Model).Browse(id).NameGet()
		if err != nil {
			return nil, err
		}
		if len(pairs) == 0 {
			return nil, fmt.Errorf("name_create could not read created record %d", id)
		}
		return pairs[0], nil
	case "web_name_search":
		readEnv := requestContextEnv(env, req)
		name := stringValue(firstNonNil(kwarg(req.Kwargs, "name"), arg(req.Args, 0)))
		spec := mapValue(firstNonNil(kwarg(req.Kwargs, "specification"), arg(req.Args, 1)))
		node, err := parseDomain(firstNonNil(kwarg(req.Kwargs, "domain"), kwarg(req.Kwargs, "args"), arg(req.Args, 2)))
		if err != nil {
			return nil, err
		}
		op, err := domain.NormalizeOperator(stringValue(firstNonNil(kwarg(req.Kwargs, "operator"), arg(req.Args, 3), "ilike")))
		if err != nil {
			return nil, err
		}
		limit := intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 4), 100))
		pairs, err := readEnv.Model(req.Model).NameSearch(name, node, op, limit)
		if err != nil {
			return nil, err
		}
		if len(spec) == 1 {
			if _, ok := spec["display_name"]; ok {
				rows := make([]map[string]any, 0, len(pairs))
				for _, pair := range pairs {
					rows = append(rows, map[string]any{
						"id":                       pair[0],
						"display_name":             pair[1],
						"__formatted_display_name": pair[1],
					})
				}
				return rows, nil
			}
		}
		ids := make([]int64, 0, len(pairs))
		for _, pair := range pairs {
			ids = append(ids, int64Value(pair[0]))
		}
		return webReadWithComputedWorkflow(readEnv, req.Model, ids, spec)
	case "search":
		readEnv := requestContextEnv(env, req)
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain"), kwarg(req.Kwargs, "args")))
		if err != nil {
			return nil, err
		}
		found, err := readEnv.Model(req.Model).SearchWithOptions(node, record.SearchOptions{
			Offset: intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 1))),
			Limit:  intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 2))),
			Order:  stringValue(firstNonNil(kwarg(req.Kwargs, "order"), arg(req.Args, 3))),
		})
		if err != nil {
			return nil, err
		}
		return found.IDs(), nil
	case "search_count":
		readEnv := requestContextEnv(env, req)
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain")))
		if err != nil {
			return nil, err
		}
		return readEnv.Model(req.Model).SearchCount(node, intValue(kwarg(req.Kwargs, "limit")))
	case "formatted_read_group":
		readEnv := requestContextEnv(env, req)
		domainValue := firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain"))
		node, err := parseDomain(domainValue)
		if err != nil {
			return nil, err
		}
		rows, err := readEnv.Model(req.Model).FormattedReadGroup(node, record.ReadGroupOptions{
			Fields:  stringSlice(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "aggregates"), kwarg(req.Kwargs, "fields"))),
			GroupBy: stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "groupby"), kwarg(req.Kwargs, "group_by"))),
			Order:   stringValue(firstNonNil(kwarg(req.Kwargs, "order"), kwarg(req.Kwargs, "orderby"), arg(req.Args, 6))),
			Offset:  intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 4))),
			Limit:   intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 5))),
		})
		if err != nil {
			return nil, err
		}
		return rows, nil
	case "web_read_group":
		readEnv := requestContextEnv(env, req)
		domainValue := firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain"))
		node, err := parseDomain(domainValue)
		if err != nil {
			return nil, err
		}
		groupBy := stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "groupby"), kwarg(req.Kwargs, "group_by")))
		fields := stringSlice(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "aggregates"), kwarg(req.Kwargs, "fields")))
		rows, err := readEnv.Model(req.Model).FormattedReadGroup(node, record.ReadGroupOptions{
			Fields:  fields,
			GroupBy: groupBy,
			Order:   stringValue(firstNonNil(kwarg(req.Kwargs, "order"), kwarg(req.Kwargs, "orderby"), arg(req.Args, 5))),
			Offset:  intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 4))),
			Limit:   intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 3))),
		})
		if err != nil {
			return nil, err
		}
		length := len(rows)
		if intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 4))) > 0 || intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 3))) > 0 {
			allRows, err := readEnv.Model(req.Model).FormattedReadGroup(node, record.ReadGroupOptions{
				Fields:  fields,
				GroupBy: groupBy,
				Order:   stringValue(firstNonNil(kwarg(req.Kwargs, "order"), kwarg(req.Kwargs, "orderby"), arg(req.Args, 5))),
			})
			if err != nil {
				return nil, err
			}
			length = len(allRows)
		}
		return map[string]any{"groups": rows, "length": length}, nil
	case "read_group":
		readEnv := requestContextEnv(env, req)
		domainValue := firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain"))
		node, err := parseDomain(domainValue)
		if err != nil {
			return nil, err
		}
		rows, err := readEnv.Model(req.Model).ReadGroup(node, record.ReadGroupOptions{
			Fields:  stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "fields"))),
			GroupBy: stringSlice(firstNonNil(arg(req.Args, 2), kwarg(req.Kwargs, "groupby"), kwarg(req.Kwargs, "group_by"))),
			Order:   stringValue(firstNonNil(kwarg(req.Kwargs, "orderby"), kwarg(req.Kwargs, "order"), arg(req.Args, 5))),
			Offset:  intValue(firstNonNil(kwarg(req.Kwargs, "offset"), arg(req.Args, 3))),
			Limit:   intValue(firstNonNil(kwarg(req.Kwargs, "limit"), arg(req.Args, 4))),
			Lazy:    readGroupLazyPointer(firstNonNil(kwarg(req.Kwargs, "lazy"), arg(req.Args, 6))),
		})
		if err != nil {
			return nil, err
		}
		applyReadGroupDomain(rows, domainValue)
		return rows, nil
	case "search_read":
		readEnv := requestContextEnv(env, req)
		node, err := parseDomain(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "domain")))
		if err != nil {
			return nil, err
		}
		fields := stringSlice(firstNonNil(kwarg(req.Kwargs, "fields"), arg(req.Args, 1)))
		found, err := readEnv.Model(req.Model).SearchWithOptions(node, record.SearchOptions{
			Offset: intValue(kwarg(req.Kwargs, "offset")),
			Limit:  intValue(kwarg(req.Kwargs, "limit")),
			Order:  stringValue(kwarg(req.Kwargs, "order")),
		})
		if err != nil {
			return nil, err
		}
		readFormatEnv := withoutContextKeyEnv(readEnv, "active_test")
		return readFormatEnv.Model(req.Model).Browse(found.IDs()...).Read(fields...)
	case "web_search_read":
		readEnv := requestContextEnv(env, req)
		node, err := parseDomain(firstNonNil(kwarg(req.Kwargs, "domain"), arg(req.Args, 0)))
		if err != nil {
			return nil, err
		}
		modelSet := readEnv.Model(req.Model)
		found, err := modelSet.SearchWithOptions(node, record.SearchOptions{
			Offset: intValue(kwarg(req.Kwargs, "offset")),
			Limit:  intValue(kwarg(req.Kwargs, "limit")),
			Order:  stringValue(kwarg(req.Kwargs, "order")),
		})
		if err != nil {
			return nil, err
		}
		rows, err := found.WebRead(mapValue(kwarg(req.Kwargs, "specification")))
		if err != nil {
			return nil, err
		}
		if err := internalworkflow.ApplyComputedWorkflowViewIDs(readEnv, req.Model, rows); err != nil {
			return nil, err
		}
		length, err := modelSet.SearchCount(node, intValue(kwarg(req.Kwargs, "count_limit")))
		if err != nil {
			return nil, err
		}
		return map[string]any{"length": length, "records": rows}, nil
	case "onchange":
		values := mapValue(arg(req.Args, 0))
		if req.Model == "res.users" {
			values = resUsersOnchangeValues(env, values)
		}
		if req.Model == "delegation" {
			values = delegationOnchangeValues(env, values, stringSlice(arg(req.Args, 1)), mapValue(kwarg(req.Kwargs, "context")))
		}
		if req.Model == "login.as" {
			values = loginAsOnchangeValues(env, values)
		}
		if req.Model == internalworkflow.ModelStateUpdateWizard {
			var err error
			values, err = internalworkflow.StateUpdateWizardOnchange(env, values, stringSlice(arg(req.Args, 1)))
			if err != nil {
				return nil, err
			}
		}
		return map[string]any{"value": values}, nil
	default:
		return nil, fmt.Errorf("unsupported method %s", req.Method)
	}
}

func (s Server) dispatchModuleMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model == "base.module.update" && req.Method == "update_module" {
		if env.Context().UserID != 1 && !sessionHasXMLIDGroup(env, "base.group_system") {
			return nil, true, fmt.Errorf("System User Only")
		}
		update, err := modulelifecycle.New(env, s.Modules).UpdateList()
		if err != nil {
			return nil, true, err
		}
		ids := positiveInt64Slice(int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])))
		if len(ids) > 0 {
			if err := env.Model("base.module.update").Browse(ids...).Write(map[string]any{"updated": update.Updated, "added": update.Added, "state": "done"}); err != nil {
				return nil, true, err
			}
		}
		return false, true, nil
	}
	if req.Model != "ir.module.module" {
		return nil, false, nil
	}
	if env.Context().UserID != 1 && !sessionHasXMLIDGroup(env, "base.group_system") {
		return nil, true, fmt.Errorf("System User Only")
	}
	ids := positiveInt64Slice(int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"])))
	service := modulelifecycle.New(env, s.Modules)
	var result modulelifecycle.Result
	var err error
	switch req.Method {
	case "update_list":
		update, err := service.UpdateList()
		if err != nil {
			return nil, true, err
		}
		return []int{update.Updated, update.Added}, true, nil
	case "button_install":
		result, err = service.ButtonInstall(ids)
	case "button_immediate_install":
		result, err = service.ButtonImmediateInstall(ids)
	case "button_upgrade":
		result, err = service.ButtonUpgrade(ids)
	case "button_immediate_upgrade":
		result, err = service.ButtonImmediateUpgrade(ids)
	case "button_uninstall":
		result, err = service.ButtonUninstall(ids)
	case "button_immediate_uninstall":
		result, err = service.ButtonImmediateUninstall(ids)
	case "button_cancel_install", "button_install_cancel":
		result, err = service.ButtonCancelInstall(ids)
	case "button_cancel_uninstall", "button_uninstall_cancel":
		result, err = service.ButtonCancelUninstall(ids)
	case "button_cancel_upgrade", "button_upgrade_cancel":
		result, err = service.ButtonCancelUpgrade(ids)
	default:
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if s.ModuleLifecycleHook != nil && moduleLifecycleRunsDataHook(req.Method, result) {
		if err := s.ModuleLifecycleHook(env, result); err != nil {
			return nil, true, err
		}
	}
	return moduleLifecycleAction(result), true, nil
}

func moduleLifecycleRunsDataHook(method string, result modulelifecycle.Result) bool {
	if len(result.Modules) == 0 {
		return false
	}
	return method == "button_immediate_install" || method == "button_immediate_upgrade"
}

func moduleLifecycleAction(result modulelifecycle.Result) map[string]any {
	return map[string]any{
		"type": "ir.actions.client",
		"tag":  "reload",
		"params": map[string]any{
			"operation": result.Operation,
			"modules":   append([]string(nil), result.Modules...),
		},
	}
}

func (s Server) dispatchLoginAsMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model != "login.as" {
		return nil, false, nil
	}
	switch req.Method {
	case "switch_to_user", "action_login":
		action, err := s.loginAsWizardActURL(env, req)
		return action, true, err
	default:
		return nil, false, nil
	}
}

func loginAsDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	values, err := env.Model("login.as").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	return loginAsApplyComputedValues(env, values, fields), nil
}

func loginAsOnchangeValues(env *record.Env, values map[string]any) map[string]any {
	out := make(map[string]any, len(values)+3)
	for key, value := range values {
		out[key] = value
	}
	userID := loginAsValueID(out["user_id"])
	groupID := loginAsValueID(out["group_id"])
	if groupID != 0 && userID != 0 && !loginAsUserHasGroup(env, userID, groupID) {
		out["user_id"] = false
		userID = 0
	}
	return loginAsApplyComputedValues(env, out, nil)
}

func loginAsApplyComputedValues(env *record.Env, values map[string]any, fields []string) map[string]any {
	if values == nil {
		values = map[string]any{}
	}
	userID := loginAsValueID(values["user_id"])
	include := func(name string) bool {
		return len(fields) == 0 || containsString(fields, name)
	}
	user := loginAsUserRow(env, userID)
	if include("company_id") {
		values["company_id"] = false
		if user != nil {
			if companyID := int64Value(user["company_id"]); companyID != 0 {
				values["company_id"] = companyID
			}
		}
	}
	if include("company_ids") {
		values["company_ids"] = []int64{}
		if user != nil {
			values["company_ids"] = uniqueInt64Slice(int64Slice(user["company_ids"]))
		}
	}
	if include("group_ids") {
		values["group_ids"] = loginAsVisibleUserGroupIDs(env, user)
	}
	return values
}

func loginAsUserHasGroup(env *record.Env, userID int64, groupID int64) bool {
	user := loginAsUserRow(env, userID)
	if user == nil || groupID == 0 {
		return false
	}
	return containsHTTPInt64(uniqueInt64Slice(append(append(int64Slice(user["groups_id"]), int64Slice(user["group_ids"])...), int64Slice(user["all_group_ids"])...)), groupID)
}

func loginAsVisibleUserGroupIDs(env *record.Env, user map[string]any) []int64 {
	if env == nil || user == nil {
		return []int64{}
	}
	ids := uniqueInt64Slice(append(append(int64Slice(user["groups_id"]), int64Slice(user["group_ids"])...), int64Slice(user["all_group_ids"])...))
	sort.SliceStable(ids, func(i, j int) bool {
		return loginAsGroupFullName(env, ids[i]) < loginAsGroupFullName(env, ids[j])
	})
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if loginAsGroupVisible(env, id) {
			out = append(out, id)
		}
	}
	return out
}

func loginAsUserRow(env *record.Env, userID int64) map[string]any {
	if env == nil || userID == 0 {
		return nil
	}
	rows, err := env.Model("res.users").Browse(userID).Read("company_id", "company_ids", "groups_id", "group_ids", "all_group_ids")
	if err != nil || len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func loginAsGroupVisible(env *record.Env, groupID int64) bool {
	if env == nil || groupID == 0 {
		return false
	}
	rows, err := env.Model("res.groups").Browse(groupID).Read("category_id")
	if err != nil || len(rows) == 0 {
		return false
	}
	categoryID := int64Value(rows[0]["category_id"])
	if categoryID == 0 {
		return false
	}
	categoryRows, err := env.Model("ir.module.category").Browse(categoryID).Read("visible")
	return err == nil && len(categoryRows) > 0 && truthyHTTPValue(categoryRows[0]["visible"])
}

func loginAsGroupFullName(env *record.Env, groupID int64) string {
	if env == nil || groupID == 0 {
		return ""
	}
	rows, err := env.Model("res.groups").Browse(groupID).Read("full_name", "name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	if fullName := strings.TrimSpace(stringValue(rows[0]["full_name"])); fullName != "" {
		return fullName
	}
	return strings.TrimSpace(stringValue(rows[0]["name"]))
}

func loginAsValueID(value any) int64 {
	if id := int64Value(value); id != 0 {
		return id
	}
	if values, ok := value.([]any); ok && len(values) > 0 {
		return int64Value(values[0])
	}
	return 0
}

func (s Server) loginAsWizardActURL(env *record.Env, req callKWRequest) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("login.as.%s requires env", req.Method)
	}
	if env.Context().UserID != 1 && !sessionHasXMLIDGroup(env, "base.group_system") {
		return nil, fmt.Errorf("System User Only")
	}
	targetID := firstID(firstNonNil(kwarg(req.Kwargs, "user_id"), req.Values["user_id"], kwarg(req.Kwargs, "target_user_id"), req.Values["target_user_id"]))
	groupID := int64Value(firstNonNil(kwarg(req.Kwargs, "group_id"), req.Values["group_id"]))
	returnTo := firstTextHTTP(kwarg(req.Kwargs, "return_to"), req.Values["return_to"])
	reason := firstTextHTTP(kwarg(req.Kwargs, "reason"), req.Values["reason"])

	wizardID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if wizardID != 0 {
		rows, err := env.Model("login.as").Browse(wizardID).Read("user_id", "group_id", "return_to")
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("login.as wizard %d not found", wizardID)
		}
		if targetID == 0 {
			targetID = int64Value(rows[0]["user_id"])
		}
		if groupID == 0 {
			groupID = int64Value(rows[0]["group_id"])
		}
		if strings.TrimSpace(returnTo) == "" {
			returnTo = stringValue(rows[0]["return_to"])
		}
	}
	if targetID == 0 {
		return nil, fmt.Errorf("login.as.%s requires user_id", req.Method)
	}

	target := fmt.Sprintf("/web/login_as/%d", targetID)
	query := url.Values{}
	if groupID != 0 {
		query.Set("group_id", strconv.FormatInt(groupID, 10))
	}
	if strings.TrimSpace(returnTo) != "" {
		query.Set("redirect", safeWebRedirect(returnTo))
	}
	if reason != "" {
		query.Set("reason", reason)
	}
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return map[string]any{
		"type":   "ir.actions.act_url",
		"url":    target,
		"target": "self",
	}, nil
}

func (s Server) dispatchLinkTrackerMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model != "link.tracker" || req.Method != "search_or_create" {
		return nil, false, nil
	}
	values := mapList(arg(req.Args, 0))
	if len(values) == 0 {
		if single := mapValue(arg(req.Args, 0)); len(single) > 0 {
			values = []map[string]any{single}
		}
	}
	if len(values) == 0 {
		values = mapList(kwarg(req.Kwargs, "vals_list"))
	}
	ids, err := record.LinkTrackerSearchOrCreate(env, values)
	if err != nil {
		return nil, true, err
	}
	return ids, true, nil
}

func (s Server) dispatchWhatsAppAccountMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model != "whatsapp.account" {
		return nil, false, nil
	}
	if req.Method != "button_sync_whatsapp_account_templates" && req.Method != "sync_templates_from_response" && req.Method != "import_templates" {
		return nil, false, nil
	}
	if env == nil {
		return nil, true, fmt.Errorf("whatsapp.account.%s requires env", req.Method)
	}
	accountID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if accountID == 0 {
		accountID = int64Value(firstNonNil(kwarg(req.Kwargs, "id"), req.Values["id"]))
	}
	if accountID == 0 {
		return nil, true, fmt.Errorf("whatsapp.account.%s requires an account id", req.Method)
	}
	payload := whatsappTemplateSyncPayload(req)
	if len(payload.Data) == 0 {
		return nil, true, fmt.Errorf("whatsapp.account.%s requires provider template data", req.Method)
	}
	created, updated, err := syncWhatsAppAccountTemplatesHTTP(env, accountID, payload)
	if err != nil {
		return nil, true, err
	}
	message := fmt.Sprintf("%d templates created, %d templates updated", created, updated)
	return map[string]any{
		"type": "ir.actions.client",
		"tag":  "display_notification",
		"params": map[string]any{
			"title":   "Templates synchronized!",
			"type":    "success",
			"message": message,
			"next":    map[string]any{"type": "ir.actions.act_window_close"},
		},
	}, true, nil
}

func (s Server) dispatchWhatsAppTemplateMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model != "whatsapp.template" {
		return nil, false, nil
	}
	if req.Method == "button_sync_template" {
		templateID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		if templateID == 0 {
			return nil, true, fmt.Errorf("whatsapp.template.button_sync_template requires a template id")
		}
		payload := whatsappTemplateSyncPayload(req)
		if len(payload.Data) == 0 {
			return nil, true, fmt.Errorf("whatsapp.template.button_sync_template requires provider template data")
		}
		if err := syncSingleWhatsAppTemplateHTTP(env, templateID, payload.Data); err != nil {
			return nil, true, err
		}
		return map[string]any{"type": "ir.actions.client", "tag": "reload"}, true, nil
	}
	if req.Method != "_get_template_button_component" {
		return nil, false, nil
	}
	templateID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	if templateID == 0 {
		return nil, true, fmt.Errorf("whatsapp.template._get_template_button_component requires a template id")
	}
	if env == nil {
		return nil, true, fmt.Errorf("whatsapp.template._get_template_button_component requires env")
	}
	if _, ok := env.ModelMetadata("whatsapp.template.button"); !ok {
		return nil, true, fmt.Errorf("model whatsapp.template.button not found")
	}
	buttons, err := env.Model("whatsapp.template.button").Search(whatsappTemplateButtonDomainHTTP(env, templateID))
	if err != nil {
		return nil, true, err
	}
	if buttons.Len() == 0 {
		return nil, true, nil
	}
	rows, err := buttons.Read("id", "sequence", "button_type", "url_type", "website_url", "dynamic_url", "name", "text", "call_number")
	if err != nil {
		return nil, true, err
	}
	sort.Slice(rows, func(i, j int) bool {
		return int64Value(rows[i]["sequence"]) < int64Value(rows[j]["sequence"])
	})
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		buttonType := strings.TrimSpace(stringValue(row["button_type"]))
		if buttonType == "" {
			continue
		}
		item := map[string]any{
			"type": strings.ToUpper(buttonType),
			"text": firstTextHTTP(row["name"], row["text"]),
		}
		switch buttonType {
		case "url":
			applyWhatsAppTemplateURLButtonData(env, item, row)
		case "phone_number":
			item["phone_number"] = strings.TrimSpace(stringValue(row["call_number"]))
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, true, nil
	}
	return map[string]any{"type": "BUTTONS", "buttons": out}, true, nil
}

type whatsappTemplateSyncPayloadHTTP struct {
	Data        []map[string]any
	PhoneNumber string
}

type whatsappTemplateImportValuesHTTP struct {
	Template  map[string]any
	Variables []map[string]any
	Buttons   []whatsappTemplateButtonImportHTTP
}

type whatsappTemplateButtonImportHTTP struct {
	Values    map[string]any
	Variables []map[string]any
}

func whatsappTemplateSyncPayload(req callKWRequest) whatsappTemplateSyncPayloadHTTP {
	var payload whatsappTemplateSyncPayloadHTTP
	for _, value := range []any{
		arg(req.Args, 1),
		kwarg(req.Kwargs, "response"),
		kwarg(req.Kwargs, "template_response"),
		kwarg(req.Kwargs, "whatsapp_template_response"),
		kwarg(req.Kwargs, "provider_response"),
		req.Values["response"],
		req.Values["template_response"],
		req.Values["whatsapp_template_response"],
		req.Values["provider_response"],
	} {
		mergeWhatsAppTemplatePayloadHTTP(&payload, value)
	}
	context := mapValue(kwarg(req.Kwargs, "context"))
	for _, key := range []string{"response", "template_response", "whatsapp_template_response", "provider_response"} {
		mergeWhatsAppTemplatePayloadHTTP(&payload, context[key])
	}
	if payload.PhoneNumber == "" {
		payload.PhoneNumber = firstTextHTTP(
			kwarg(req.Kwargs, "phone_number"),
			kwarg(req.Kwargs, "display_phone_number"),
			req.Values["phone_number"],
			req.Values["display_phone_number"],
			context["phone_number"],
			context["display_phone_number"],
			context["wa_phone_number"],
		)
	}
	return payload
}

func mergeWhatsAppTemplatePayloadHTTP(payload *whatsappTemplateSyncPayloadHTTP, value any) {
	if payload == nil || value == nil {
		return
	}
	if data := whatsappProviderTemplateDataHTTP(value); len(data) > 0 {
		payload.Data = data
	}
	valueMap := mapValue(value)
	if len(valueMap) == 0 {
		return
	}
	if payload.PhoneNumber == "" {
		payload.PhoneNumber = firstTextHTTP(valueMap["phone_number"], valueMap["display_phone_number"], valueMap["wa_phone_number"])
	}
}

func whatsappProviderTemplateDataHTTP(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return filterNonEmptyMapsHTTP(typed)
	case []any:
		return filterNonEmptyMapsHTTP(mapList(typed))
	case map[string]any:
		for _, key := range []string{"data", "templates", "message_templates"} {
			if data := whatsappProviderTemplateDataHTTP(typed[key]); len(data) > 0 {
				return data
			}
		}
		if strings.TrimSpace(stringValue(typed["id"])) != "" || strings.TrimSpace(stringValue(typed["name"])) != "" {
			return []map[string]any{typed}
		}
		return nil
	default:
		return nil
	}
}

func filterNonEmptyMapsHTTP(values []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if len(value) > 0 {
			out = append(out, value)
		}
	}
	return out
}

func syncWhatsAppAccountTemplatesHTTP(env *record.Env, accountID int64, payload whatsappTemplateSyncPayloadHTTP) (int, int, error) {
	if env == nil {
		return 0, 0, fmt.Errorf("whatsapp account sync requires env")
	}
	accountRows, err := env.Model("whatsapp.account").Browse(accountID).Read("id")
	if err != nil {
		return 0, 0, err
	}
	if len(accountRows) == 0 {
		return 0, 0, fmt.Errorf("whatsapp.account:%d not found", accountID)
	}
	existing, err := whatsappTemplatesByUIDHTTP(env, accountID)
	if err != nil {
		return 0, 0, err
	}
	created := 0
	updated := 0
	for _, remote := range payload.Data {
		importValues, err := whatsappTemplateImportValuesFromProviderHTTP(remote, accountID)
		if err != nil {
			return created, updated, err
		}
		uid := strings.TrimSpace(stringValue(importValues.Template["wa_template_uid"]))
		if uid == "" {
			return created, updated, fmt.Errorf("whatsapp provider template id is required")
		}
		if templateID := existing[uid]; templateID != 0 {
			if err := updateWhatsAppTemplateFromImportHTTP(env, templateID, importValues); err != nil {
				return created, updated, err
			}
			updated++
			continue
		}
		templateID, err := createWhatsAppTemplateFromImportHTTP(env, importValues)
		if err != nil {
			return created, updated, err
		}
		existing[uid] = templateID
		created++
	}
	if err := updateWhatsAppAccountSyncStatsHTTP(env, accountID, payload.PhoneNumber); err != nil {
		return created, updated, err
	}
	return created, updated, nil
}

func syncSingleWhatsAppTemplateHTTP(env *record.Env, templateID int64, data []map[string]any) error {
	if env == nil {
		return fmt.Errorf("whatsapp template sync requires env")
	}
	rows, err := env.Model("whatsapp.template").Browse(templateID).Read("id", "wa_account_id", "wa_template_uid")
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("whatsapp.template:%d not found", templateID)
	}
	currentUID := strings.TrimSpace(stringValue(rows[0]["wa_template_uid"]))
	selected := map[string]any{}
	for _, remote := range data {
		if currentUID == "" || strings.TrimSpace(stringValue(remote["id"])) == currentUID {
			selected = remote
			break
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("whatsapp provider response does not include template %s", currentUID)
	}
	accountID := int64Value(rows[0]["wa_account_id"])
	importValues, err := whatsappTemplateImportValuesFromProviderHTTP(selected, accountID)
	if err != nil {
		return err
	}
	return updateWhatsAppTemplateFromImportHTTP(env, templateID, importValues)
}

func whatsappTemplatesByUIDHTTP(env *record.Env, accountID int64) (map[string]int64, error) {
	searchEnv := whatsappInactiveEnvHTTP(env)
	found, err := searchEnv.Model("whatsapp.template").Search(domain.Cond("wa_account_id", domain.Equal, accountID))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "wa_template_uid")
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		uid := strings.TrimSpace(stringValue(row["wa_template_uid"]))
		if uid != "" && out[uid] == 0 {
			out[uid] = int64Value(row["id"])
		}
	}
	return out, nil
}

func updateWhatsAppAccountSyncStatsHTTP(env *record.Env, accountID int64, phoneNumber string) error {
	values := map[string]any{}
	if phoneNumber != "" && modelHasField(env, "whatsapp.account", "phone_number") {
		values["phone_number"] = phoneNumber
	}
	if modelHasField(env, "whatsapp.account", "templates_count") {
		count, err := whatsappInactiveEnvHTTP(env).Model("whatsapp.template").SearchCount(domain.Cond("wa_account_id", domain.Equal, accountID), 0)
		if err != nil {
			return err
		}
		values["templates_count"] = int64(count)
	}
	if len(values) == 0 {
		return nil
	}
	return env.Model("whatsapp.account").Browse(accountID).Write(values)
}

func whatsappInactiveEnvHTTP(env *record.Env) *record.Env {
	if env == nil {
		return env
	}
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = false
	ctx.Values = values
	return env.WithContext(ctx)
}

func whatsappTemplateImportValuesFromProviderHTTP(remote map[string]any, accountID int64) (whatsappTemplateImportValuesHTTP, error) {
	templateName := strings.TrimSpace(stringValue(remote["name"]))
	templateUID := strings.TrimSpace(stringValue(remote["id"]))
	if templateName == "" || templateUID == "" {
		return whatsappTemplateImportValuesHTTP{}, fmt.Errorf("whatsapp provider template requires name and id")
	}
	quality := strings.ToLower(strings.TrimSpace(stringValue(mapValue(remote["quality_score"])["score"])))
	if quality == "" || quality == "unknown" {
		quality = "none"
	}
	out := whatsappTemplateImportValuesHTTP{
		Template: map[string]any{
			"body":            "",
			"button_ids":      []int64{},
			"footer_text":     "",
			"header_text":     "",
			"header_type":     "none",
			"lang_code":       strings.TrimSpace(stringValue(remote["language"])),
			"name":            whatsappTemplateDisplayNameHTTP(templateName),
			"quality":         quality,
			"status":          strings.ToLower(strings.TrimSpace(stringValue(remote["status"]))),
			"template_name":   templateName,
			"template_type":   strings.ToLower(strings.TrimSpace(stringValue(remote["category"]))),
			"wa_account_id":   accountID,
			"wa_template_uid": templateUID,
			"active":          true,
		},
	}
	if out.Template["lang_code"] == "" {
		out.Template["lang_code"] = "en"
	}
	if out.Template["status"] == "" {
		out.Template["status"] = "draft"
	}
	if out.Template["template_type"] == "" {
		out.Template["template_type"] = "marketing"
	}
	for _, component := range mapList(remote["components"]) {
		componentType := strings.ToUpper(strings.TrimSpace(stringValue(component["type"])))
		switch componentType {
		case "HEADER":
			if err := applyWhatsAppProviderHeaderHTTP(&out, component); err != nil {
				return whatsappTemplateImportValuesHTTP{}, err
			}
		case "BODY":
			out.Template["body"] = stringValue(component["text"])
			for index, exampleValue := range firstNestedListHTTP(mapValue(component["example"])["body_text"]) {
				out.Variables = append(out.Variables, map[string]any{
					"name":       fmt.Sprintf("{{%d}}", index+1),
					"demo_value": stringValue(exampleValue),
					"line_type":  "body",
					"field_type": "free_text",
				})
			}
		case "FOOTER":
			out.Template["footer_text"] = stringValue(component["text"])
		case "BUTTONS":
			for index, button := range mapList(component["buttons"]) {
				importButton, ok := whatsappProviderButtonImportHTTP(button, index)
				if ok {
					out.Buttons = append(out.Buttons, importButton)
				}
			}
		}
	}
	return out, nil
}

func applyWhatsAppProviderHeaderHTTP(out *whatsappTemplateImportValuesHTTP, component map[string]any) error {
	format := strings.ToUpper(strings.TrimSpace(stringValue(component["format"])))
	switch format {
	case "TEXT":
		out.Template["header_type"] = "text"
		out.Template["header_text"] = stringValue(component["text"])
		for index, exampleValue := range anyListHTTP(mapValue(component["example"])["header_text"]) {
			out.Variables = append(out.Variables, map[string]any{
				"name":       fmt.Sprintf("{{%d}}", index+1),
				"demo_value": stringValue(exampleValue),
				"line_type":  "header",
				"field_type": "free_text",
			})
		}
	case "LOCATION":
		out.Template["header_type"] = "location"
		for _, name := range []string{"name", "address", "latitude", "longitude"} {
			out.Variables = append(out.Variables, map[string]any{
				"name":      name,
				"line_type": "location",
			})
		}
	case "IMAGE", "VIDEO", "DOCUMENT":
		out.Template["header_type"] = strings.ToLower(format)
	case "":
		return nil
	default:
		return fmt.Errorf("unsupported whatsapp provider header format %s", format)
	}
	return nil
}

func whatsappProviderButtonImportHTTP(button map[string]any, index int) (whatsappTemplateButtonImportHTTP, bool) {
	buttonType := strings.ToUpper(strings.TrimSpace(stringValue(button["type"])))
	switch buttonType {
	case "URL", "PHONE_NUMBER", "QUICK_REPLY":
	default:
		return whatsappTemplateButtonImportHTTP{}, false
	}
	name := strings.TrimSpace(stringValue(button["text"]))
	values := map[string]any{
		"sequence":    int64(index),
		"name":        name,
		"text":        name,
		"button_type": strings.ToLower(buttonType),
		"call_number": stringValue(button["phone_number"]),
		"url_type":    "static",
	}
	if buttonType == "URL" {
		websiteURL := strings.ReplaceAll(stringValue(button["url"]), "{{1}}", "")
		values["website_url"] = websiteURL
		examples := anyListHTTP(button["example"])
		if len(examples) > 0 {
			values["url_type"] = "dynamic"
			demo := stringValue(examples[0])
			if demo == "" {
				demo = websiteURL + "???"
			}
			return whatsappTemplateButtonImportHTTP{
				Values: values,
				Variables: []map[string]any{{
					"name":       name,
					"demo_value": demo,
					"line_type":  "button",
					"field_type": "free_text",
				}},
			}, true
		}
	}
	return whatsappTemplateButtonImportHTTP{Values: values}, true
}

func createWhatsAppTemplateFromImportHTTP(env *record.Env, importValues whatsappTemplateImportValuesHTTP) (int64, error) {
	values := filterModelValuesHTTP(env, "whatsapp.template", importValues.Template)
	actualHeaderType := strings.TrimSpace(stringValue(values["header_type"]))
	if actualHeaderType == "location" {
		values["header_type"] = "none"
	}
	templateID, err := env.Model("whatsapp.template").Create(values)
	if err != nil {
		return 0, err
	}
	if err := createWhatsAppTemplateVariablesHTTP(env, templateID, importValues.Variables); err != nil {
		return 0, err
	}
	if actualHeaderType == "location" {
		if err := env.Model("whatsapp.template").Browse(templateID).Write(map[string]any{"header_type": "location"}); err != nil {
			return 0, err
		}
	}
	if err := createWhatsAppTemplateButtonsHTTP(env, templateID, importValues.Buttons); err != nil {
		return 0, err
	}
	return templateID, nil
}

func updateWhatsAppTemplateFromImportHTTP(env *record.Env, templateID int64, importValues whatsappTemplateImportValuesHTTP) error {
	importValues = preserveTrackedWhatsAppTemplateButtonsHTTP(env, templateID, importValues)
	templateFields := []string{"body", "header_type", "header_text", "footer_text", "lang_code", "template_type", "status", "quality"}
	updateValues := map[string]any{}
	for _, fieldName := range templateFields {
		if _, ok := importValues.Template[fieldName]; ok && modelHasField(env, "whatsapp.template", fieldName) {
			updateValues[fieldName] = importValues.Template[fieldName]
		}
	}
	nextHeaderType := strings.TrimSpace(stringValue(updateValues["header_type"]))
	if nextHeaderType == "location" {
		if err := reconcileWhatsAppTemplateVariablesHTTP(env, templateID, importValues.Variables, map[string]bool{"location": true}); err != nil {
			return err
		}
		if err := deleteWhatsAppTemplateVariablesByLineHTTP(env, templateID, "header"); err != nil {
			return err
		}
		if err := env.Model("whatsapp.template").Browse(templateID).Write(updateValues); err != nil {
			return err
		}
		if err := reconcileWhatsAppTemplateVariablesHTTP(env, templateID, importValues.Variables, map[string]bool{"body": true}); err != nil {
			return err
		}
	} else {
		if err := deleteWhatsAppTemplateVariablesByLineHTTP(env, templateID, "location"); err != nil {
			return err
		}
		if nextHeaderType != "text" {
			if err := deleteWhatsAppTemplateVariablesByLineHTTP(env, templateID, "header"); err != nil {
				return err
			}
		}
		if err := env.Model("whatsapp.template").Browse(templateID).Write(updateValues); err != nil {
			return err
		}
		if err := reconcileWhatsAppTemplateVariablesHTTP(env, templateID, importValues.Variables, map[string]bool{"header": true, "body": true}); err != nil {
			return err
		}
	}
	return replaceWhatsAppTemplateButtonsHTTP(env, templateID, importValues.Buttons)
}

func preserveTrackedWhatsAppTemplateButtonsHTTP(env *record.Env, templateID int64, importValues whatsappTemplateImportValuesHTTP) whatsappTemplateImportValuesHTTP {
	if env == nil || templateID == 0 || len(importValues.Buttons) == 0 {
		return importValues
	}
	existing, err := env.Model("whatsapp.template.button").Search(whatsappTemplateButtonDomainHTTP(env, templateID))
	if err != nil || existing.Len() == 0 {
		return importValues
	}
	rows, err := existing.Read("sequence", "button_type", "url_type", "website_url")
	if err != nil {
		return importValues
	}
	trackedBySequence := map[int64]string{}
	for _, row := range rows {
		if strings.TrimSpace(stringValue(row["button_type"])) != "url" || strings.TrimSpace(stringValue(row["url_type"])) != "tracked" {
			continue
		}
		trackedBySequence[int64Value(row["sequence"])] = stringValue(row["website_url"])
	}
	if len(trackedBySequence) == 0 {
		return importValues
	}
	for index := range importValues.Buttons {
		button := importValues.Buttons[index]
		if strings.TrimSpace(stringValue(button.Values["button_type"])) != "url" {
			continue
		}
		sequence := int64Value(button.Values["sequence"])
		websiteURL, ok := trackedBySequence[sequence]
		if !ok {
			continue
		}
		if button.Values == nil {
			button.Values = map[string]any{}
		}
		button.Values["url_type"] = "tracked"
		button.Values["website_url"] = websiteURL
		importValues.Buttons[index] = button
	}
	return importValues
}

func createWhatsAppTemplateVariablesHTTP(env *record.Env, templateID int64, variables []map[string]any) error {
	for _, variable := range variables {
		values := filterModelValuesHTTP(env, "whatsapp.template.variable", variable)
		values["wa_template_id"] = templateID
		if _, err := env.Model("whatsapp.template.variable").Create(values); err != nil {
			return err
		}
	}
	return nil
}

func reconcileWhatsAppTemplateVariablesHTTP(env *record.Env, templateID int64, desired []map[string]any, lineTypes map[string]bool) error {
	if len(lineTypes) == 0 {
		return nil
	}
	existingRows, err := whatsappTemplateVariableRowsHTTP(env, templateID)
	if err != nil {
		return err
	}
	existing := map[string]map[string]any{}
	for _, row := range existingRows {
		if int64Value(row["button_id"]) != 0 {
			continue
		}
		lineType := strings.TrimSpace(stringValue(row["line_type"]))
		if !lineTypes[lineType] {
			continue
		}
		existing[whatsappTemplateVariableKeyHTTP(row)] = row
	}
	desiredKeys := map[string]bool{}
	for _, variable := range desired {
		lineType := strings.TrimSpace(stringValue(variable["line_type"]))
		if !lineTypes[lineType] {
			continue
		}
		key := whatsappTemplateVariableKeyHTTP(variable)
		if key == "" {
			continue
		}
		desiredKeys[key] = true
		if existing[key] != nil {
			continue
		}
		values := filterModelValuesHTTP(env, "whatsapp.template.variable", variable)
		values["wa_template_id"] = templateID
		if _, err := env.Model("whatsapp.template.variable").Create(values); err != nil {
			return err
		}
	}
	for key, row := range existing {
		if desiredKeys[key] {
			continue
		}
		if err := env.Model("whatsapp.template.variable").Browse(int64Value(row["id"])).Unlink(); err != nil {
			return err
		}
	}
	return nil
}

func deleteWhatsAppTemplateVariablesByLineHTTP(env *record.Env, templateID int64, lineType string) error {
	rows, err := whatsappTemplateVariableRowsHTTP(env, templateID)
	if err != nil {
		return err
	}
	var ids []int64
	for _, row := range rows {
		if int64Value(row["button_id"]) == 0 && strings.TrimSpace(stringValue(row["line_type"])) == lineType {
			ids = append(ids, int64Value(row["id"]))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return env.Model("whatsapp.template.variable").Browse(ids...).Unlink()
}

func whatsappTemplateVariableRowsHTTP(env *record.Env, templateID int64) ([]map[string]any, error) {
	found, err := env.Model("whatsapp.template.variable").Search(domain.Cond("wa_template_id", domain.Equal, templateID))
	if err != nil {
		return nil, err
	}
	return found.Read("id", "name", "line_type", "button_id")
}

func whatsappTemplateVariableKeyHTTP(values map[string]any) string {
	name := strings.TrimSpace(stringValue(values["name"]))
	lineType := strings.TrimSpace(stringValue(values["line_type"]))
	if name == "" || lineType == "" {
		return ""
	}
	return lineType + "\x00" + name
}

func replaceWhatsAppTemplateButtonsHTTP(env *record.Env, templateID int64, buttons []whatsappTemplateButtonImportHTTP) error {
	if err := clearWhatsAppTemplateButtonsHTTP(env, templateID); err != nil {
		return err
	}
	return createWhatsAppTemplateButtonsHTTP(env, templateID, buttons)
}

func clearWhatsAppTemplateButtonsHTTP(env *record.Env, templateID int64) error {
	buttons, err := env.Model("whatsapp.template.button").Search(whatsappTemplateButtonDomainHTTP(env, templateID))
	if err != nil {
		return err
	}
	rows, err := buttons.Read("id")
	if err != nil {
		return err
	}
	for _, row := range rows {
		buttonID := int64Value(row["id"])
		if buttonID == 0 {
			continue
		}
		if vars, err := env.Model("whatsapp.template.variable").Search(domain.Cond("button_id", domain.Equal, buttonID)); err != nil {
			return err
		} else if err := vars.Unlink(); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		return nil
	}
	var ids []int64
	for _, row := range rows {
		ids = append(ids, int64Value(row["id"]))
	}
	return env.Model("whatsapp.template.button").Browse(ids...).Unlink()
}

func createWhatsAppTemplateButtonsHTTP(env *record.Env, templateID int64, buttons []whatsappTemplateButtonImportHTTP) error {
	for _, button := range buttons {
		values := filterModelValuesHTTP(env, "whatsapp.template.button", button.Values)
		values["wa_template_id"] = templateID
		buttonID, err := env.Model("whatsapp.template.button").Create(values)
		if err != nil {
			return err
		}
		for _, variable := range button.Variables {
			variableValues := filterModelValuesHTTP(env, "whatsapp.template.variable", variable)
			variableValues["wa_template_id"] = templateID
			variableValues["button_id"] = buttonID
			if _, err := env.Model("whatsapp.template.variable").Create(variableValues); err != nil {
				return err
			}
		}
	}
	return nil
}

func filterModelValuesHTTP(env *record.Env, modelName string, values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		if modelHasField(env, modelName, key) {
			out[key] = value
		}
	}
	return out
}

func whatsappTemplateDisplayNameHTTP(templateName string) string {
	parts := strings.Fields(strings.ReplaceAll(templateName, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func anyListHTTP(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case nil:
		return nil
	default:
		return []any{typed}
	}
}

func firstNestedListHTTP(value any) []any {
	items := anyListHTTP(value)
	if len(items) == 0 {
		return nil
	}
	switch items[0].(type) {
	case []any, []string:
		nested := anyListHTTP(items[0])
		return nested
	}
	return items
}

func whatsappTemplateButtonDomainHTTP(env *record.Env, templateID int64) domain.Node {
	var conditions []domain.Node
	if modelHasField(env, "whatsapp.template.button", "wa_template_id") {
		conditions = append(conditions, domain.Cond("wa_template_id", domain.Equal, templateID))
	}
	if modelHasField(env, "whatsapp.template.button", "template_id") {
		conditions = append(conditions, domain.Cond("template_id", domain.Equal, templateID))
	}
	switch len(conditions) {
	case 0:
		return domain.Bool(false)
	case 1:
		return conditions[0]
	default:
		return domain.Or(conditions...)
	}
}

func applyWhatsAppTemplateURLButtonData(env *record.Env, item map[string]any, row map[string]any) {
	websiteURL := stringValue(row["website_url"])
	if strings.HasPrefix(websiteURL, "/") {
		baseURL := strings.TrimRight(firstTextHTTP(configParameterValue(env, "web.base.url"), "http://localhost"), "/")
		websiteURL = baseURL + websiteURL
	}
	item["url"] = websiteURL
	switch strings.TrimSpace(stringValue(row["url_type"])) {
	case "dynamic":
		baseURL := firstTextHTTP(websiteURL, row["dynamic_url"])
		item["url"] = baseURL + "{{1}}"
		item["example"] = whatsappTemplateButtonVariableDemoHTTP(env, int64Value(row["id"]), baseURL+"???")
	case "tracked":
		baseURL := strings.TrimRight(firstTextHTTP(configParameterValue(env, "web.base.url"), "http://localhost"), "/")
		item["url"] = baseURL + "/{{1}}"
		item["example"] = baseURL + "/???"
	}
}

func whatsappTemplateButtonVariableDemoHTTP(env *record.Env, buttonID int64, fallback string) string {
	if env == nil || buttonID == 0 || !modelHasField(env, "whatsapp.template.variable", "button_id") || !modelHasField(env, "whatsapp.template.variable", "demo_value") {
		return fallback
	}
	variables, err := env.Model("whatsapp.template.variable").SearchWithOptions(domain.Cond("button_id", domain.Equal, buttonID), record.SearchOptions{Limit: 1})
	if err != nil || variables.Len() == 0 {
		return fallback
	}
	rows, err := variables.Read("demo_value")
	if err != nil || len(rows) == 0 {
		return fallback
	}
	if demo := strings.TrimSpace(stringValue(rows[0]["demo_value"])); demo != "" {
		return demo
	}
	return fallback
}

func copyModelRecords(env *record.Env, modelName string, ids []int64, defaults map[string]any) ([]int64, error) {
	ids = positiveInt64Slice(ids)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("model %s not found", modelName)
	}
	fields := copyableFieldNames(meta, defaults)
	rows, err := env.Model(modelName).Browse(ids...).Read(fields...)
	if err != nil {
		return nil, err
	}
	newIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		values := copyValuesForCreate(meta, row, defaults)
		id, err := env.Model(modelName).Create(values)
		if err != nil {
			return nil, err
		}
		newIDs = append(newIDs, id)
	}
	return newIDs, nil
}

func copyableFieldNames(meta model.Model, defaults map[string]any) []string {
	names := make([]string, 0, len(meta.Fields))
	for name, f := range meta.Fields {
		if name == "id" || name == "display_name" || name == "parent_path" {
			continue
		}
		if _, overridden := defaults[name]; overridden {
			continue
		}
		if !f.Store || f.Kind == field.Computed || f.Kind == field.Related || f.Kind == field.One2Many {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func copyValuesForCreate(meta model.Model, row map[string]any, defaults map[string]any) map[string]any {
	values := map[string]any{}
	for name, f := range meta.Fields {
		if _, overridden := defaults[name]; overridden {
			continue
		}
		value, ok := row[name]
		if !ok {
			continue
		}
		if f.Kind == field.Many2Many {
			values[name] = int64Slice(value)
			continue
		}
		values[name] = value
	}
	for name, value := range defaults {
		if _, ok := meta.Fields[name]; ok {
			values[name] = value
		}
	}
	if meta.Name == "res.partner" {
		recName := strings.TrimSpace(meta.RecName)
		if recName == "" {
			recName = "name"
		}
		if _, overridden := defaults[recName]; !overridden {
			if original := strings.TrimSpace(stringValue(row[recName])); original != "" {
				values[recName] = original + " (copy)"
			}
		}
	}
	return values
}

func setModelActiveFlag(env *record.Env, modelName string, ids []int64, active bool) error {
	ids = positiveInt64Slice(ids)
	if len(ids) == 0 {
		return nil
	}
	fieldName, err := modelActiveField(env, modelName)
	if err != nil {
		return err
	}
	rows, err := env.Model(modelName).Browse(ids...).Read(fieldName)
	if err != nil {
		return err
	}
	targetIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		if accountingBoolValue(row[fieldName]) != active {
			targetIDs = append(targetIDs, id)
		}
	}
	if len(targetIDs) == 0 {
		return nil
	}
	return env.Model(modelName).Browse(targetIDs...).Write(map[string]any{fieldName: active})
}

func modelActiveField(env *record.Env, modelName string) (string, error) {
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return "", fmt.Errorf("model %s not found", modelName)
	}
	for _, name := range []string{"active", "x_active"} {
		if meta.Fields[name].Name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no active field on model %s", modelName)
}

func exportModelData(env *record.Env, req callKWRequest) (map[string]any, error) {
	if !exportAllowed(env) {
		return nil, fmt.Errorf("export access denied")
	}
	exportEnv := requestContextEnv(env, req)
	contextValues := mapValue(kwarg(req.Kwargs, "context"))
	fields := stringSlice(arg(req.Args, 0))
	ids := positiveInt64Slice(int64Slice(firstNonNil(kwarg(req.Kwargs, "ids"), contextValues["active_ids"])))
	if len(fields) == 0 {
		ids = positiveInt64Slice(int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), contextValues["active_ids"])))
		fields = stringSlice(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "fields"), kwarg(req.Kwargs, "fields_to_export")))
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("export_data requires fields")
	}
	if len(ids) == 0 {
		node, err := parseDomain(firstNonNil(kwarg(req.Kwargs, "domain"), contextValues["active_domain"]))
		if err != nil {
			return nil, err
		}
		found, err := exportEnv.Model(req.Model).SearchWithOptions(node, record.SearchOptions{Limit: intValue(contextValues["active_ids_limit"])})
		if err != nil {
			return nil, err
		}
		ids = found.IDs()
	}
	importCompat := boolHTTPWithFallback(firstNonNil(contextValues["import_compat"], true), true)
	rows, err := exportAnyFlatRows(exportEnv, req.Model, ids, fields, importCompat)
	if err != nil {
		return nil, err
	}
	return map[string]any{"datas": rows}, nil
}

func exportFieldUsesResolver(env *record.Env, modelName string, name string) bool {
	name = normalizeExportFieldName(name)
	if name == "id" || name == ".id" || name == "display_name" || strings.Contains(name, "/") {
		return true
	}
	if _, _, _, ok := exportPropertyPath(env, modelName, name); ok {
		return true
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return false
	}
	fieldMeta, ok := meta.Fields[name]
	return ok && fieldMeta.Relation != ""
}

func exportAllowed(env *record.Env) bool {
	if env == nil {
		return false
	}
	if env.Context().UserID == 1 {
		return true
	}
	groupID := httpXMLIDResID(env, "base", "group_allow_export", "res.groups")
	if groupID == 0 {
		return true
	}
	return menuGroupSet(env)[groupID]
}

func positiveInt64Slice(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func (s Server) dispatchResPartnerMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "res.partner" {
		return nil, false, nil
	}
	ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
	switch req.Method {
	case "signup_prepare", "action_signup_prepare":
		signupType := firstTextHTTP(firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "signup_type")), "signup")
		return true, true, internalmail.SignupPrepare(env, ids, signupType)
	case "signup_cancel":
		return true, true, internalmail.SignupCancel(env, ids)
	case "signup_get_auth_param":
		params, err := internalmail.SignupAuthParams(env, ids)
		return params, true, err
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchResUsersMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if req.Model != "res.users" {
		return nil, false, nil
	}
	switch req.Method {
	case "action_get":
		return s.resUsersPreferencesAction(env), true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) resUsersPreferencesAction(env *record.Env) map[string]any {
	uid := int64(0)
	if env != nil {
		uid = env.Context().UserID
	}
	viewID := s.externalIDRecordID("base.view_users_preferences_form", "ir.ui.view")
	context := map[string]any{
		"active_model":            "res.users",
		"gorp_preferences_dialog": true,
	}
	if uid > 0 {
		context["active_id"] = uid
		context["active_ids"] = []int64{uid}
	}
	return map[string]any{
		"type":      "ir.actions.act_window",
		"name":      "Change My Preferences",
		"res_model": "res.users",
		"res_id":    falseIfZero(uid),
		"view_mode": "form",
		"views":     []any{[]any{falseIfZero(viewID), "form"}},
		"view_id":   falseIfZero(viewID),
		"target":    "new",
		"context":   context,
	}
}

func (s Server) externalIDRecordID(xmlID string, modelName string) int64 {
	if len(s.ExternalIDs) == 0 {
		return 0
	}
	external, ok := s.ExternalIDs[xmlID]
	if !ok || external.ResID <= 0 {
		return 0
	}
	if modelName != "" && external.Model != modelName {
		return 0
	}
	return external.ResID
}

func (s Server) dispatchResGroupsMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "res.groups" {
		return nil, false, nil
	}
	switch req.Method {
	case "_get_group_definitions":
		definitions, err := resGroupsDefinitions(env)
		return definitions, true, err
	case "action_show_all_users":
		ids := int64Slice(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "ids"), req.Values["ids"]))
		if len(ids) == 0 {
			return nil, true, fmt.Errorf("action_show_all_users requires at least one group id")
		}
		rows, err := env.Model("res.groups").Browse(ids[0]).Read("name", "full_name")
		if err != nil {
			return nil, true, err
		}
		name := "Users and implied users"
		if len(rows) > 0 {
			if value := firstTextHTTP(rows[0]["full_name"], rows[0]["name"]); value != "" {
				name = "Users and implied users of " + value
			}
		}
		return map[string]any{
			"name":      name,
			"view_mode": "list,form",
			"res_model": "res.users",
			"type":      "ir.actions.act_window",
			"context":   map[string]any{"create": false, "delete": false, "form_view_ref": "base.view_users_form"},
			"domain":    []any{[]any{"all_group_ids", "in", ids}},
			"target":    "current",
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (s Server) dispatchSequenceMethod(env *record.Env, req callKWRequest) (any, bool, error) {
	if env == nil || req.Model != "ir.sequence" {
		return nil, false, nil
	}
	env = sequenceRequestEnv(env, req)
	service := sequences.Service{Env: env}
	switch req.Method {
	case "next_by_id":
		sequenceID := firstID(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "id"), kwarg(req.Kwargs, "ids")))
		if sequenceID == 0 {
			return nil, true, fmt.Errorf("next_by_id requires a sequence id")
		}
		ctx := sequences.ContextWithSequenceDate(context.Background(), firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "sequence_date")))
		value, err := service.NextByID(ctx, sequenceID)
		return value, true, err
	case "next_by_code":
		code := stringValue(firstNonNil(arg(req.Args, 0), kwarg(req.Kwargs, "sequence_code"), kwarg(req.Kwargs, "code")))
		sequenceDate := firstNonNil(arg(req.Args, 1), kwarg(req.Kwargs, "sequence_date"))
		value, ok, err := service.NextByCode(context.Background(), code, sequenceDate)
		if err != nil || !ok {
			return false, true, err
		}
		return value, true, nil
	default:
		return nil, false, nil
	}
}

func sequenceRequestEnv(env *record.Env, req callKWRequest) *record.Env {
	return requestContextEnv(env, req)
}

func requestContextEnv(env *record.Env, req callKWRequest) *record.Env {
	requestContext := mapValue(kwarg(req.Kwargs, "context"))
	return requestContextEnvFromMap(env, requestContext)
}

func requestContextEnvFromMap(env *record.Env, requestContext map[string]any) *record.Env {
	if len(requestContext) == 0 {
		return env
	}
	ctx := env.Context()
	ctx.Values = cloneContextValues(ctx.Values)
	for key, value := range requestContext {
		if requestContextSecurityKey(key) {
			continue
		}
		ctx.Values[key] = value
	}
	if allowedCompanyIDs := int64Slice(requestContext["allowed_company_ids"]); len(allowedCompanyIDs) > 0 {
		ctx.CompanyIDs = allowedCompanyIDs
		ctx.CompanyID = allowedCompanyIDs[0]
	}
	if companyID := int64Value(requestContext["company_id"]); companyID != 0 {
		ctx.CompanyID = companyID
		if len(ctx.CompanyIDs) == 0 {
			ctx.CompanyIDs = []int64{companyID}
		}
	}
	return env.WithContext(ctx)
}

func requestContextSecurityKey(key string) bool {
	switch key {
	case "group_ids", "groups_id", "all_group_ids":
		return true
	default:
		return false
	}
}

func withoutContextKeyEnv(env *record.Env, key string) *record.Env {
	ctx := env.Context()
	if _, ok := ctx.Values[key]; !ok {
		return env
	}
	ctx.Values = cloneContextValues(ctx.Values)
	delete(ctx.Values, key)
	return env.WithContext(ctx)
}

func createContextEnv(env *record.Env, req callKWRequest) *record.Env {
	requestContext := mapValue(kwarg(req.Kwargs, "context"))
	value, ok := requestContext["approval_auto_submit"]
	if !ok {
		return env
	}
	ctx := env.Context()
	ctx.Values = cloneContextValues(ctx.Values)
	ctx.Values["approval_auto_submit"] = value
	return env.WithContext(ctx)
}

func webReadWithComputedWorkflow(env *record.Env, modelName string, ids []int64, specification map[string]any) ([]map[string]any, error) {
	rows, err := env.Model(modelName).Browse(ids...).WebRead(specification)
	if err != nil {
		return nil, err
	}
	if err := internalworkflow.ApplyComputedWorkflowViewIDs(env, modelName, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s Server) actionLoad(w http.ResponseWriter, r *http.Request) {
	actionKey := r.URL.Query().Get("id")
	requestContext := map[string]any{}
	var envelope *rpcEnvelope
	if r.Method == http.MethodPost {
		var req struct {
			ActionID any            `json:"action_id"`
			Context  map[string]any `json:"context"`
		}
		var err error
		envelope, err = decodeRPCParams(r, &req)
		if err != nil {
			writeRPCError(w, nil, http.StatusBadRequest, err)
			return
		}
		actionKey = strings.TrimSpace(fmt.Sprint(req.ActionID))
		requestContext = req.Context
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	loader := s
	loader.Env = requestContextEnvFromMap(env, requestContext)
	actionPayload, ok := loader.findActionPayload(actionKey)
	if !ok {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("action %q not found", actionKey))
		return
	}
	writeRPCOrJSON(w, envelope, actionPayload)
}

func (s Server) actionRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActionID any            `json:"action_id"`
		Context  map[string]any `json:"context"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	env, ok := s.requireWebSession(w, r, envelope)
	if !ok {
		return
	}
	if s.ServerActions == nil {
		writeRPCError(w, envelope, http.StatusNotFound, fmt.Errorf("server action registry not available"))
		return
	}
	actionID := int64Value(req.ActionID)
	if actionID == 0 {
		writeRPCError(w, envelope, http.StatusBadRequest, fmt.Errorf("action_id is required"))
		return
	}
	actionRow, actionExists := s.ServerActions.Get(actionID)
	runEnv := requestContextEnvFromMap(env, req.Context)
	contextValues := runEnv.Context().Values
	modelName := strings.TrimSpace(stringValue(contextValues["active_model"]))
	if modelName == "" && actionExists {
		modelName = actionRow.Model
	}
	activeIDs := positiveInt64Slice(int64Slice(contextValues["active_ids"]))
	activeID := int64Value(contextValues["active_id"])
	if len(activeIDs) > 0 {
		activeID = activeIDs[0]
	}
	if len(activeIDs) == 0 && activeID != 0 {
		activeIDs = []int64{activeID}
	}
	result, err := s.ServerActions.Run(r.Context(), actionID, serveractions.ExecutionContext{
		Model:        modelName,
		RecordID:     activeID,
		RecordIDs:    activeIDs,
		UserID:       runEnv.Context().UserID,
		UserGroupIDs: groupIDsFromSet(menuGroupSet(env)),
		Values:       cloneHTTPMap(contextValues),
		Metadata:     cloneHTTPMap(contextValues),
		Now:          time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, serveractions.ErrActionWarning) {
			writeRPCException(w, envelope, http.StatusOK, serverActionWarningExceptionName, serverActionWarningMessage(actionRow), []any{serverActionWarningMessage(actionRow)})
			return
		}
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	response, err := actionRunResultPayload(s, runEnv, result)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, response)
}

func (s Server) exportFormats(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	envelope, err := decodeRPCParams(r, &struct{}{})
	if err != nil && !errors.Is(err, io.EOF) {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if !exportAllowed(env) {
		writeRPCError(w, envelope, http.StatusForbidden, fmt.Errorf("export access denied"))
		return
	}
	writeRPCOrJSON(w, envelope, []map[string]any{
		{"tag": "xlsx", "label": "XLSX"},
		{"tag": "csv", "label": "CSV"},
	})
}

func (s Server) exportGetFields(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	var req struct {
		Model           string         `json:"model"`
		Domain          any            `json:"domain"`
		Prefix          string         `json:"prefix"`
		ParentName      string         `json:"parent_name"`
		ImportCompat    bool           `json:"import_compat"`
		ParentFieldType string         `json:"parent_field_type"`
		ParentField     map[string]any `json:"parent_field"`
		Exclude         []string       `json:"exclude"`
	}
	req.ImportCompat = true
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if !exportAllowed(env) {
		writeRPCError(w, envelope, http.StatusForbidden, fmt.Errorf("export access denied"))
		return
	}
	fields, err := exportFields(env, exportFieldsRequest{
		Model:           req.Model,
		Domain:          req.Domain,
		Prefix:          req.Prefix,
		ParentName:      req.ParentName,
		ImportCompat:    req.ImportCompat,
		ParentFieldType: req.ParentFieldType,
		ParentField:     req.ParentField,
		Exclude:         req.Exclude,
	})
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, fields)
}

func (s Server) exportNamelist(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	var req struct {
		Model    string `json:"model"`
		ExportID int64  `json:"export_id"`
	}
	envelope, err := decodeRPCParams(r, &req)
	if err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if !exportAllowed(env) {
		writeRPCError(w, envelope, http.StatusForbidden, fmt.Errorf("export access denied"))
		return
	}
	fields, err := exportNamelistFields(env, req.Model, req.ExportID)
	if err != nil {
		writeRPCError(w, envelope, http.StatusForbidden, err)
		return
	}
	writeRPCOrJSON(w, envelope, fields)
}

func (s Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	raw := r.FormValue("data")
	if raw == "" {
		raw = r.FormValue("payload")
	}
	var req exportDownloadRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if !exportAllowed(env) {
		writeRPCError(w, nil, http.StatusForbidden, fmt.Errorf("export access denied"))
		return
	}
	content, filename, err := exportCSVBytes(env, req)
	if err != nil {
		writeRPCError(w, nil, http.StatusForbidden, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	_, _ = w.Write(content)
}

func (s Server) exportXLSX(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	raw := r.FormValue("data")
	if raw == "" {
		raw = r.FormValue("payload")
	}
	var req exportDownloadRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		writeRPCError(w, nil, http.StatusBadRequest, err)
		return
	}
	if !exportAllowed(env) {
		writeRPCError(w, nil, http.StatusForbidden, fmt.Errorf("export access denied"))
		return
	}
	content, filename, err := exportXLSXBytes(env, req)
	if err != nil {
		writeFileExportException(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	_, _ = w.Write(content)
}

type exportDownloadRequest struct {
	ImportCompat bool             `json:"import_compat"`
	Context      map[string]any   `json:"context"`
	Domain       any              `json:"domain"`
	Fields       []exportFieldRef `json:"fields"`
	GroupBy      []string         `json:"groupby"`
	IDs          any              `json:"ids"`
	Model        string           `json:"model"`
}

type exportFieldRef struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Store bool   `json:"store"`
	Type  string `json:"type"`
}

type exportFieldsRequest struct {
	Model           string
	Domain          any
	Prefix          string
	ParentName      string
	ImportCompat    bool
	ParentFieldType string
	ParentField     map[string]any
	Exclude         []string
}

func exportFields(env *record.Env, req exportFieldsRequest) ([]map[string]any, error) {
	descriptions, err := env.Model(req.Model).FieldsGet(nil, []string{"string", "type", "required", "store", "relation", "relation_field", "definition_record", "definition_record_field", "exportable", "readonly", "default_export_compatible"})
	if err != nil {
		return nil, err
	}
	if req.ImportCompat && (req.ParentFieldType == "many2one" || req.ParentFieldType == "many2many") {
		filtered := map[string]map[string]any{}
		if idField, ok := descriptions["id"]; ok {
			filtered["id"] = idField
		}
		recName := exportRecName(env, req.Model)
		if nameField, ok := descriptions[recName]; ok {
			filtered[recName] = nameField
		}
		descriptions = filtered
	} else if !req.ImportCompat {
		if idField, ok := descriptions["id"]; ok {
			descriptions[".id"] = cloneHTTPMap(idField)
		}
	}
	if idField, ok := descriptions["id"]; ok {
		idField["string"] = "External ID"
	}
	if len(req.ParentField) != 0 {
		parentField := cloneHTTPMap(req.ParentField)
		parentField["string"] = "External ID"
		if fieldType := firstNonEmptyHTTPString(stringValue(parentField["field_type"]), stringValue(parentField["type"])); fieldType != "" {
			parentField["type"] = fieldType
			parentField["field_type"] = fieldType
		}
		descriptions["id"] = parentField
	}
	propertyFields, err := exportPropertyFields(env, req.Model, descriptions, req.Domain)
	if err != nil {
		return nil, err
	}
	for name, item := range propertyFields {
		descriptions[name] = item
	}
	excluded := map[string]bool{}
	for _, name := range req.Exclude {
		excluded[name] = true
	}
	names := make([]string, 0, len(descriptions))
	for name, item := range descriptions {
		if name == "display_name" || excluded[name] {
			continue
		}
		if req.ImportCompat && name != "id" && accountingBoolValue(item["readonly"]) {
			continue
		}
		if exportable, ok := item["exportable"].(bool); ok && !exportable {
			continue
		}
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		left := strings.ToLower(firstNonEmptyHTTPString(stringValue(descriptions[names[i]]["string"]), names[i]))
		right := strings.ToLower(firstNonEmptyHTTPString(stringValue(descriptions[names[j]]["string"]), names[j]))
		if left != right {
			return left < right
		}
		return names[i] < names[j]
	})
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, exportFieldDescription(req, name, descriptions[name]))
	}
	return out, nil
}

func exportFieldDescription(req exportFieldsRequest, fieldName string, item map[string]any) map[string]any {
	ident := joinExportPath(req.Prefix, fieldName)
	value := ident
	fieldType := stringValue(item["type"])
	if fieldName == "name" && req.ImportCompat && (req.ParentFieldType == "many2one" || req.ParentFieldType == "many2many") {
		value = req.Prefix
	}
	label := firstNonEmptyHTTPString(stringValue(item["string"]), fieldName)
	name := joinExportPath(req.ParentName, label)
	fieldInfo := map[string]any{
		"id":             ident,
		"name":           ident,
		"string":         name,
		"label":          name,
		"value":          value,
		"children":       false,
		"type":           fieldType,
		"field_type":     fieldType,
		"required":       accountingBoolValue(item["required"]),
		"relation_field": item["relation_field"],
		"default_export": req.ImportCompat && accountingBoolValue(item["default_export_compatible"]),
		"store":          firstNonNil(item["store"], true),
	}
	if relation := stringValue(item["relation"]); relation != "" {
		fieldInfo["relation"] = relation
		if len(strings.Split(ident, "/")) < 3 {
			fieldInfo["value"] = value + "/id"
			fieldInfo["children"] = true
			fieldInfo["params"] = map[string]any{
				"model":        relation,
				"prefix":       ident,
				"name":         name,
				"parent_field": exportParentFieldMetadata(item),
			}
		}
	}
	return fieldInfo
}

func exportParentFieldMetadata(item map[string]any) map[string]any {
	out := cloneHTTPMap(item)
	if _, ok := out["field_type"]; !ok {
		out["field_type"] = stringValue(item["type"])
	}
	return out
}

type exportPropertyDefinitionRecord struct {
	ID          int64
	DisplayName string
	Definitions []map[string]any
}

func exportPropertyFields(env *record.Env, modelName string, descriptions map[string]map[string]any, rawDomain any) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for fieldName, item := range descriptions {
		if stringValue(item["type"]) != string(field.Properties) {
			continue
		}
		definitionRecord := stringValue(item["definition_record"])
		definitionField := stringValue(item["definition_record_field"])
		if definitionRecord == "" || definitionField == "" {
			continue
		}
		records, err := exportPropertyDefinitionRecords(env, modelName, definitionRecord, definitionField, rawDomain)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			for _, definition := range record.Definitions {
				propName := strings.TrimSpace(stringValue(definition["name"]))
				propType := strings.TrimSpace(stringValue(definition["type"]))
				if propName == "" || propType == "" || propType == "separator" {
					continue
				}
				relation := strings.TrimSpace(firstNonEmptyHTTPString(stringValue(definition["comodel"]), stringValue(definition["relation"])))
				if (propType == "many2one" || propType == "many2many") && !exportModelExists(env, relation) {
					continue
				}
				label := firstNonEmptyHTTPString(stringValue(definition["string"]), propName)
				if record.DisplayName != "" {
					label = fmt.Sprintf("%s (%s)", label, record.DisplayName)
				}
				description := map[string]any{
					"type":                      propType,
					"string":                    label,
					"default_export_compatible": accountingBoolValue(item["default_export_compatible"]),
					"store":                     true,
				}
				if relation != "" && (propType == "many2one" || propType == "many2many") {
					description["relation"] = relation
				}
				out[fieldName+"."+propName] = description
			}
		}
	}
	return out, nil
}

func exportPropertyDefinitionRecords(env *record.Env, modelName string, definitionRecord string, definitionField string, rawDomain any) ([]exportPropertyDefinitionRecord, error) {
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("model %s not found", modelName)
	}
	recordField, ok := meta.Fields[definitionRecord]
	if !ok || recordField.Relation == "" {
		return nil, nil
	}
	allowedIDs, err := exportPropertyAllowedDefinitionIDs(env, modelName, definitionRecord, rawDomain)
	if err != nil {
		return nil, err
	}
	found, err := env.Model(recordField.Relation).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read(definitionField)
	if err != nil {
		return nil, err
	}
	out := make([]exportPropertyDefinitionRecord, 0, len(rows))
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 || (allowedIDs != nil && !allowedIDs[id]) {
			continue
		}
		definitions := exportPropertyDefinitionMaps(row[definitionField])
		if len(definitions) == 0 {
			continue
		}
		displayName, err := exportRecordDisplayName(env, recordField.Relation, id)
		if err != nil {
			return nil, err
		}
		out = append(out, exportPropertyDefinitionRecord{ID: id, DisplayName: displayName, Definitions: definitions})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func exportPropertyAllowedDefinitionIDs(env *record.Env, modelName string, definitionRecord string, rawDomain any) (map[int64]bool, error) {
	if exportDomainIsEmpty(rawDomain) {
		return nil, nil
	}
	node, err := parseDomain(rawDomain)
	if err != nil {
		return nil, err
	}
	found, err := env.Model(modelName).Search(node)
	if err != nil {
		return nil, err
	}
	rows, err := found.Read(definitionRecord)
	if err != nil {
		return nil, err
	}
	allowed := map[int64]bool{}
	for _, row := range rows {
		if id := firstPropertyID(row[definitionRecord]); id > 0 {
			allowed[id] = true
		}
	}
	return allowed, nil
}

func exportDomainIsEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case []domain.Node:
		return len(typed) == 0
	case domain.Node:
		return typed.Kind == "" || (typed.Kind == domain.All && len(typed.Children) == 0)
	default:
		return false
	}
}

func exportPropertyDefinitionMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := mapValue(item); len(mapped) != 0 {
				out = append(out, mapped)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{typed}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var maps []map[string]any
		if err := json.Unmarshal([]byte(text), &maps); err == nil {
			return maps
		}
		var items []any
		if err := json.Unmarshal([]byte(text), &items); err == nil {
			return exportPropertyDefinitionMaps(items)
		}
		return nil
	default:
		return nil
	}
}

func exportModelExists(env *record.Env, modelName string) bool {
	if strings.TrimSpace(modelName) == "" {
		return false
	}
	_, ok := env.ModelMetadata(modelName)
	return ok
}

func exportNamelistFields(env *record.Env, modelName string, exportID int64) ([]map[string]any, error) {
	if exportID == 0 {
		return nil, fmt.Errorf("export_id is required")
	}
	lines, err := env.Model("ir.exports.line").Search(domain.Cond("export_id", domain.Equal, exportID))
	if err != nil {
		return nil, err
	}
	rows, err := lines.Read("name")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := stringValue(row["name"]); name != "" {
			names = append(names, name)
		}
	}
	return exportFieldsInfo(env, modelName, names, exportFieldsInfoOptions{})
}

type exportFieldsInfoOptions struct {
	ImportCompat    bool
	ParentFieldType string
}

func exportFieldsInfo(env *record.Env, modelName string, names []string, opts exportFieldsInfoOptions) ([]map[string]any, error) {
	descriptions, err := env.Model(modelName).FieldsGet(nil, []string{"string", "type", "relation", "required", "relation_field", "definition_record", "definition_record_field", "default_export_compatible", "exportable", "readonly"})
	if err != nil {
		return nil, err
	}
	if _, ok := descriptions[".id"]; !ok {
		if idField, ok := descriptions["id"]; ok {
			descriptions[".id"] = cloneHTTPMap(idField)
		}
	}
	propertyFields, err := exportPropertyFields(env, modelName, descriptions, nil)
	if err != nil {
		return nil, err
	}
	for name, item := range propertyFields {
		descriptions[name] = item
	}
	if opts.ImportCompat && (opts.ParentFieldType == "many2one" || opts.ParentFieldType == "many2many") {
		filtered := map[string]map[string]any{}
		if idField, ok := descriptions["id"]; ok {
			filtered["id"] = idField
		}
		recName := exportRecName(env, modelName)
		if nameField, ok := descriptions[recName]; ok {
			filtered[recName] = nameField
		}
		descriptions = filtered
	}
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		info, err := exportFieldInfo(env, modelName, descriptions, name, opts)
		if err != nil {
			return nil, err
		}
		if info != nil {
			out = append(out, info)
		}
	}
	return out, nil
}

func exportFieldInfo(env *record.Env, modelName string, descriptions map[string]map[string]any, name string, opts exportFieldsInfoOptions) (map[string]any, error) {
	if name == "" {
		return nil, nil
	}
	base, rest, nested := strings.Cut(name, "/")
	item, ok := descriptions[base]
	if !ok {
		return nil, nil
	}
	if opts.ImportCompat && base != "id" && accountingBoolValue(item["readonly"]) {
		return nil, nil
	}
	if exportable, ok := item["exportable"].(bool); ok && !exportable {
		return nil, nil
	}
	label := firstNonEmptyHTTPString(stringValue(item["string"]), base)
	if !nested {
		fieldType := stringValue(item["type"])
		return map[string]any{
			"id":             base,
			"name":           base,
			"string":         label,
			"label":          label,
			"value":          base,
			"type":           fieldType,
			"field_type":     fieldType,
			"required":       accountingBoolValue(item["required"]),
			"relation_field": item["relation_field"],
		}, nil
	}
	relation := stringValue(item["relation"])
	if relation == "" {
		return nil, nil
	}
	children, err := exportFieldsInfo(env, relation, []string{rest}, exportFieldsInfoOptions{
		ImportCompat:    opts.ImportCompat,
		ParentFieldType: stringValue(item["type"]),
	})
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return nil, nil
	}
	child := children[0]
	childID := stringValue(child["id"])
	childString := firstNonEmptyHTTPString(stringValue(child["string"]), childID)
	child["id"] = base + "/" + childID
	child["name"] = child["id"]
	child["string"] = label + "/" + childString
	child["label"] = child["string"]
	child["value"] = child["id"]
	return child, nil
}

func exportRecName(env *record.Env, modelName string) string {
	meta, ok := env.ModelMetadata(modelName)
	if !ok || meta.RecName == "" {
		return "name"
	}
	return meta.RecName
}

func joinExportPath(prefix string, name string) string {
	prefix = strings.Trim(prefix, "/")
	name = strings.Trim(name, "/")
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "/" + name
}

func exportCSVBytes(env *record.Env, req exportDownloadRequest) ([]byte, string, error) {
	env = exportDownloadEnv(env, req)
	if len(req.GroupBy) > 0 {
		return nil, "", fmt.Errorf("exporting grouped data to csv is not supported")
	}
	matrix, filename, err := exportStringMatrix(env, req, ".csv")
	if err != nil {
		return nil, "", err
	}
	for rowIndex := range matrix {
		if rowIndex == 0 {
			continue
		}
		for colIndex, value := range matrix[rowIndex] {
			matrix[rowIndex][colIndex] = exportCSVSafeCell(value)
		}
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.WriteAll(matrix); err != nil {
		return nil, "", err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), filename, nil
}

func exportXLSXBytes(env *record.Env, req exportDownloadRequest) ([]byte, string, error) {
	env = exportDownloadEnv(env, req)
	matrix, filename, err := exportXLSXMatrix(env, req)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	zipper := zip.NewWriter(&buf)
	sheetXML, err := exportSheetXML(matrix)
	if err != nil {
		return nil, "", err
	}
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`</Relationships>`,
		"xl/styles.xml":            exportStylesXML(exportXLSXDecimalPlaces(env)),
		"xl/worksheets/sheet1.xml": sheetXML,
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writer, err := zipper.Create(name)
		if err != nil {
			_ = zipper.Close()
			return nil, "", err
		}
		if _, err := writer.Write([]byte(files[name])); err != nil {
			_ = zipper.Close()
			return nil, "", err
		}
	}
	if err := zipper.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), filename, nil
}

func exportDownloadEnv(env *record.Env, req exportDownloadRequest) *record.Env {
	return requestContextEnvFromMap(env, req.Context)
}

const (
	xlsxMaxRows         = 1048576
	xlsxMaxStringLength = 32767
)

const (
	xlsxStyleBase = iota
	xlsxStyleHeader
	xlsxStyleDate
	xlsxStyleDateTime
	xlsxStyleFloat
	xlsxStyleGroupHeader
	xlsxStyleGroupHeaderFloat
)

type exportXLSXCell struct {
	Value any
	Kind  field.Kind
	Style int
}

func exportXLSXMatrix(env *record.Env, req exportDownloadRequest) ([][]exportXLSXCell, string, error) {
	fields, labels, err := exportDownloadFields(req)
	if err != nil {
		return nil, "", err
	}
	ids, err := exportDownloadIDs(env, req)
	if err != nil {
		return nil, "", err
	}
	var matrix [][]exportXLSXCell
	if len(req.GroupBy) > 0 && !req.ImportCompat {
		matrix, err = exportGroupedXLSXMatrix(env, req.Model, ids, fields, labels, req.GroupBy)
		if err != nil {
			return nil, "", err
		}
	} else {
		rows, err := exportAnyFlatRows(env, req.Model, ids, fields, req.ImportCompat)
		if err != nil {
			return nil, "", err
		}
		if err := validateXLSXRowLimit(len(rows)); err != nil {
			return nil, "", err
		}
		fieldMetas := exportXLSXFieldMetadata(env, req.Model, fields)
		matrix = make([][]exportXLSXCell, 0, len(rows)+1)
		matrix = append(matrix, exportXLSXHeaderRow(labels))
		for _, row := range rows {
			line := make([]exportXLSXCell, len(row))
			for index, value := range row {
				meta := field.Field{}
				if index < len(fieldMetas) {
					meta = fieldMetas[index]
				}
				line[index] = exportXLSXBodyCell(value, meta, false)
			}
			matrix = append(matrix, line)
		}
	}
	return matrix, exportDownloadFilename(req.Model, ".xlsx"), nil
}

func validateXLSXRowLimit(rowCount int) error {
	if rowCount > xlsxMaxRows {
		return fmt.Errorf("There are too many rows (%d rows, limit: %d) to export as Excel 2007-2013 (.xlsx) format. Consider splitting the export.", rowCount, xlsxMaxRows)
	}
	return nil
}

func exportDownloadFilename(modelName string, extension string) string {
	filename := strings.ReplaceAll(strings.TrimSpace(modelName), ".", "_")
	if filename == "" {
		filename = "export"
	}
	return filename + extension
}

func exportStringMatrix(env *record.Env, req exportDownloadRequest, extension string) ([][]string, string, error) {
	fields, labels, err := exportDownloadFields(req)
	if err != nil {
		return nil, "", err
	}
	ids, err := exportDownloadIDs(env, req)
	if err != nil {
		return nil, "", err
	}
	var matrix [][]string
	if len(req.GroupBy) > 0 && !req.ImportCompat {
		matrix, err = exportGroupedStringMatrix(env, req.Model, ids, fields, labels, req.GroupBy)
	} else {
		matrix, err = exportFlatStringMatrix(env, req.Model, ids, fields, labels, req.ImportCompat)
	}
	if err != nil {
		return nil, "", err
	}
	return matrix, exportDownloadFilename(req.Model, extension), nil
}

func exportDownloadFields(req exportDownloadRequest) ([]string, []string, error) {
	fields := make([]string, 0, len(req.Fields))
	labels := make([]string, 0, len(req.Fields))
	for _, field := range req.Fields {
		name := normalizeExportFieldName(field.Name)
		if name == "" {
			continue
		}
		fields = append(fields, name)
		if req.ImportCompat {
			labels = append(labels, name)
		} else if strings.TrimSpace(field.Label) != "" {
			labels = append(labels, field.Label)
		} else {
			labels = append(labels, name)
		}
	}
	if len(fields) == 0 {
		return nil, nil, fmt.Errorf("export requires fields")
	}
	return fields, labels, nil
}

func exportDownloadIDs(env *record.Env, req exportDownloadRequest) ([]int64, error) {
	ids := positiveInt64Slice(int64Slice(req.IDs))
	if len(ids) == 0 {
		node, err := parseDomain(req.Domain)
		if err != nil {
			return nil, err
		}
		found, err := env.Model(req.Model).Search(node)
		if err != nil {
			return nil, err
		}
		ids = found.IDs()
	}
	return ids, nil
}

func exportAnyFlatRows(env *record.Env, modelName string, ids []int64, fields []string, importCompat bool) ([][]any, error) {
	readFields := exportReadFields(env, modelName, fields)
	rows, err := env.Model(modelName).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		lines, err := exportRecordAnyLines(env, modelName, row, fields, importCompat)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
}

type exportAnyResolvedRow struct {
	ID    int64
	Raw   map[string]any
	Cells []any
}

func exportAnyResolvedFlatRows(env *record.Env, modelName string, ids []int64, fields []string, importCompat bool, extraFields ...[]string) ([]exportAnyResolvedRow, error) {
	fieldSets := make([][]string, 0, len(extraFields)+1)
	fieldSets = append(fieldSets, fields)
	fieldSets = append(fieldSets, extraFields...)
	readFields := exportReadFields(env, modelName, fieldSets...)
	rows, err := env.Model(modelName).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	out := make([]exportAnyResolvedRow, 0, len(rows))
	for _, row := range rows {
		lines, err := exportRecordAnyLines(env, modelName, row, fields, importCompat)
		if err != nil {
			return nil, err
		}
		for _, cells := range lines {
			out = append(out, exportAnyResolvedRow{ID: int64Value(row["id"]), Raw: row, Cells: cells})
		}
	}
	return out, nil
}

func exportRecordAnyLines(env *record.Env, modelName string, row map[string]any, fields []string, importCompat bool) ([][]any, error) {
	current := newExportAnyLine(len(fields))
	extra := [][]any{}
	primaryDone := map[string]bool{}
	for index, raw := range fields {
		name := strings.Trim(normalizeExportFieldName(raw), "/")
		if name == "" {
			continue
		}
		primary := exportPrimaryFieldName(name)
		if primaryDone[primary] {
			continue
		}
		propValue, propMeta, isProperty, err := exportPropertyFieldValue(env, modelName, row, primary)
		if err != nil {
			return nil, err
		}
		if isProperty && !importCompat && propMeta.Type == "many2many" && propMeta.Relation != "" {
			primaryDone[primary] = true
			childLines, subfields, err := exportX2ManyChildAnyLines(env, propMeta.Relation, propertyRelationIDs(propValue), fields, primary)
			if err != nil {
				return nil, err
			}
			if len(childLines) == 0 {
				current[index] = ""
				continue
			}
			for col, cell := range childLines[0] {
				if subfields[col] != "" {
					current[col] = cell
				}
			}
			for _, child := range childLines[1:] {
				line := newExportAnyLine(len(fields))
				for col, cell := range child {
					if subfields[col] != "" {
						line[col] = cell
					}
				}
				extra = append(extra, line)
			}
			continue
		}
		fieldMeta, isX2Many := exportX2ManyField(env, modelName, primary)
		if isX2Many {
			primaryDone[primary] = true
			if importCompat && fieldMeta.Kind == field.Many2Many {
				target, cell, err := exportImportCompatibleMany2ManyCell(env, fieldMeta.Relation, row[primary], fields, primary, index)
				if err != nil {
					return nil, err
				}
				current[target] = cell
				continue
			}
			childLines, subfields, err := exportX2ManyChildAnyLines(env, fieldMeta.Relation, row[primary], fields, primary)
			if err != nil {
				return nil, err
			}
			if len(childLines) == 0 {
				current[index] = ""
				continue
			}
			for col, cell := range childLines[0] {
				if subfields[col] != "" {
					current[col] = cell
				}
			}
			for _, child := range childLines[1:] {
				line := newExportAnyLine(len(fields))
				for col, cell := range child {
					if subfields[col] != "" {
						line[col] = cell
					}
				}
				extra = append(extra, line)
			}
			continue
		}
		cell, err := exportAnyFieldCell(env, modelName, row, name)
		if err != nil {
			return nil, err
		}
		current[index] = cell
	}
	return append([][]any{current}, extra...), nil
}

func newExportAnyLine(size int) []any {
	line := make([]any, size)
	for index := range line {
		line[index] = ""
	}
	return line
}

func exportFlatStringMatrix(env *record.Env, modelName string, ids []int64, fields []string, labels []string, importCompat bool) ([][]string, error) {
	rows, err := exportResolvedFlatRows(env, modelName, ids, fields, importCompat)
	if err != nil {
		return nil, err
	}
	matrix := make([][]string, 0, len(rows)+1)
	matrix = append(matrix, labels)
	for _, row := range rows {
		matrix = append(matrix, row.Cells)
	}
	return matrix, nil
}

func exportResolvedFlatRows(env *record.Env, modelName string, ids []int64, fields []string, importCompat bool, extraFields ...[]string) ([]exportResolvedRow, error) {
	fieldSets := make([][]string, 0, len(extraFields)+1)
	fieldSets = append(fieldSets, fields)
	fieldSets = append(fieldSets, extraFields...)
	readFields := exportReadFields(env, modelName, fieldSets...)
	rows, err := env.Model(modelName).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	out := make([]exportResolvedRow, 0, len(rows))
	for _, row := range rows {
		lines, err := exportRecordCellLines(env, modelName, row, fields, importCompat)
		if err != nil {
			return nil, err
		}
		for _, cells := range lines {
			out = append(out, exportResolvedRow{ID: int64Value(row["id"]), Raw: row, Cells: cells})
		}
	}
	return out, nil
}

func exportRecordCellLines(env *record.Env, modelName string, row map[string]any, fields []string, importCompat bool) ([][]string, error) {
	current := make([]string, len(fields))
	extra := [][]string{}
	primaryDone := map[string]bool{}
	for index, raw := range fields {
		name := strings.Trim(normalizeExportFieldName(raw), "/")
		if name == "" {
			continue
		}
		primary := exportPrimaryFieldName(name)
		if primaryDone[primary] {
			continue
		}
		propValue, propMeta, isProperty, err := exportPropertyFieldValue(env, modelName, row, primary)
		if err != nil {
			return nil, err
		}
		if isProperty && !importCompat && propMeta.Type == "many2many" && propMeta.Relation != "" {
			primaryDone[primary] = true
			childLines, subfields, err := exportX2ManyChildLines(env, propMeta.Relation, propertyRelationIDs(propValue), fields, primary)
			if err != nil {
				return nil, err
			}
			if len(childLines) == 0 {
				current[index] = ""
				continue
			}
			for col, cell := range childLines[0] {
				if subfields[col] != "" {
					current[col] = cell
				}
			}
			for _, child := range childLines[1:] {
				line := make([]string, len(fields))
				for col, cell := range child {
					if subfields[col] != "" {
						line[col] = cell
					}
				}
				extra = append(extra, line)
			}
			continue
		}
		fieldMeta, isX2Many := exportX2ManyField(env, modelName, primary)
		if isX2Many {
			primaryDone[primary] = true
			if importCompat && fieldMeta.Kind == field.Many2Many {
				target, cell, err := exportImportCompatibleMany2ManyCell(env, fieldMeta.Relation, row[primary], fields, primary, index)
				if err != nil {
					return nil, err
				}
				current[target] = cell
				continue
			}
			childLines, subfields, err := exportX2ManyChildLines(env, fieldMeta.Relation, row[primary], fields, primary)
			if err != nil {
				return nil, err
			}
			if len(childLines) == 0 {
				current[index] = ""
				continue
			}
			for col, cell := range childLines[0] {
				if subfields[col] != "" {
					current[col] = cell
				}
			}
			for _, child := range childLines[1:] {
				line := make([]string, len(fields))
				for col, cell := range child {
					if subfields[col] != "" {
						line[col] = cell
					}
				}
				extra = append(extra, line)
			}
			continue
		}
		value, err := exportFieldValue(env, modelName, row, name)
		if err != nil {
			return nil, err
		}
		current[index] = exportCellString(value)
	}
	return append([][]string{current}, extra...), nil
}

func exportPrimaryFieldName(name string) string {
	if base, _, nested := strings.Cut(strings.Trim(name, "/"), "/"); nested {
		return strings.TrimSpace(base)
	}
	return strings.TrimSpace(name)
}

func exportX2ManyField(env *record.Env, modelName string, fieldName string) (field.Field, bool) {
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return field.Field{}, false
	}
	fieldMeta, ok := meta.Fields[fieldName]
	if !ok || fieldMeta.Relation == "" {
		return field.Field{}, false
	}
	return fieldMeta, fieldMeta.Kind == field.One2Many || fieldMeta.Kind == field.Many2Many
}

func exportX2ManyChildLines(env *record.Env, relation string, value any, fields []string, primary string) ([][]string, []string, error) {
	subfields := make([]string, len(fields))
	for index, raw := range fields {
		name := strings.Trim(normalizeExportFieldName(raw), "/")
		base, rest, nested := strings.Cut(name, "/")
		if strings.TrimSpace(base) != primary {
			continue
		}
		if nested {
			subfields[index] = strings.TrimSpace(rest)
		} else {
			subfields[index] = "display_name"
		}
	}
	ids := positiveInt64Slice(int64Slice(value))
	if len(ids) == 0 {
		return nil, subfields, nil
	}
	readFields := exportReadFields(env, relation, subfields)
	rows, err := env.Model(relation).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, nil, err
	}
	lines := make([][]string, 0, len(rows))
	for _, row := range rows {
		line := make([]string, len(fields))
		for index, subfield := range subfields {
			if subfield == "" {
				continue
			}
			value, err := exportFieldValue(env, relation, row, subfield)
			if err != nil {
				return nil, nil, err
			}
			line[index] = exportCellString(value)
		}
		lines = append(lines, line)
	}
	return lines, subfields, nil
}

func exportX2ManyChildAnyLines(env *record.Env, relation string, value any, fields []string, primary string) ([][]any, []string, error) {
	subfields := make([]string, len(fields))
	for index, raw := range fields {
		name := strings.Trim(normalizeExportFieldName(raw), "/")
		base, rest, nested := strings.Cut(name, "/")
		if strings.TrimSpace(base) != primary {
			continue
		}
		if nested {
			subfields[index] = strings.TrimSpace(rest)
		} else {
			subfields[index] = "display_name"
		}
	}
	ids := positiveInt64Slice(int64Slice(value))
	if len(ids) == 0 {
		return nil, subfields, nil
	}
	readFields := exportReadFields(env, relation, subfields)
	rows, err := env.Model(relation).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, nil, err
	}
	lines := make([][]any, 0, len(rows))
	for _, row := range rows {
		line := newExportAnyLine(len(fields))
		for index, subfield := range subfields {
			if subfield == "" {
				continue
			}
			value, err := exportAnyFieldCell(env, relation, row, subfield)
			if err != nil {
				return nil, nil, err
			}
			line[index] = value
		}
		lines = append(lines, line)
	}
	return lines, subfields, nil
}

func exportAnyFieldCell(env *record.Env, modelName string, row map[string]any, raw string) (any, error) {
	name := normalizeExportFieldName(raw)
	if exportFieldUsesResolver(env, modelName, name) {
		value, err := exportFieldValue(env, modelName, row, name)
		if err != nil {
			return nil, err
		}
		return exportCellString(value), nil
	}
	return firstNonNil(row[name], false), nil
}

func exportImportCompatibleMany2ManyCell(env *record.Env, relation string, value any, fields []string, primary string, fallbackIndex int) (int, string, error) {
	targetIndex := fallbackIndex
	targetName := ""
	for _, candidate := range []string{"id", "name", "display_name"} {
		for index, raw := range fields {
			subfield, ok := exportRelationSubfield(raw, primary)
			if !ok || subfield != candidate {
				continue
			}
			targetIndex = index
			targetName = candidate
			break
		}
		if targetName != "" {
			break
		}
	}
	ids := positiveInt64Slice(int64Slice(value))
	if targetName == "id" {
		xmlIDs, err := exportXMLIDs(env, relation, ids)
		return targetIndex, xmlIDs, err
	}
	displayNames, err := exportRelationDisplayNames(env, relation, ids)
	return targetIndex, displayNames, err
}

func exportRelationSubfield(raw string, primary string) (string, bool) {
	name := strings.Trim(normalizeExportFieldName(raw), "/")
	base, rest, nested := strings.Cut(name, "/")
	if strings.TrimSpace(base) != primary {
		return "", false
	}
	if nested {
		return strings.TrimSpace(rest), true
	}
	return "display_name", true
}

type exportResolvedRow struct {
	ID    int64
	Raw   map[string]any
	Cells []string
}

func exportResolvedRows(env *record.Env, modelName string, ids []int64, fields []string, extraFields []string) ([]exportResolvedRow, error) {
	readFields := exportReadFields(env, modelName, fields, extraFields)
	rows, err := env.Model(modelName).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	out := make([]exportResolvedRow, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(fields))
		for _, name := range fields {
			value, err := exportFieldValue(env, modelName, row, name)
			if err != nil {
				return nil, err
			}
			cells = append(cells, exportCellString(value))
		}
		out = append(out, exportResolvedRow{
			ID:    int64Value(row["id"]),
			Raw:   row,
			Cells: cells,
		})
	}
	return out, nil
}

func exportReadFields(env *record.Env, modelName string, fieldNames ...[]string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, names := range fieldNames {
		for _, raw := range names {
			if fieldName, _, fieldMeta, ok := exportPropertyPath(env, modelName, raw); ok {
				add(fieldName)
				add(fieldMeta.DefinitionRecord)
				continue
			}
			add(exportTopLevelField(raw))
		}
	}
	return out
}

func exportTopLevelField(raw string) string {
	name := normalizeExportFieldName(raw)
	if name == "" {
		return ""
	}
	if name == ".id" {
		return "id"
	}
	if base, _, nested := strings.Cut(name, "/"); nested {
		return strings.TrimSpace(base)
	}
	return name
}

func normalizeExportFieldName(raw string) string {
	name := strings.TrimSpace(raw)
	name = replaceExportIDAlias(name, ".id", "/.id")
	name = replaceExportIDAlias(name, ":id", "/id")
	return name
}

func replaceExportIDAlias(name string, alias string, replacement string) string {
	if name == "" || alias == "" {
		return name
	}
	var out strings.Builder
	start := 0
	for {
		index := strings.Index(name[start:], alias)
		if index < 0 {
			out.WriteString(name[start:])
			return out.String()
		}
		index += start
		if index > 0 && name[index-1] != '/' {
			out.WriteString(name[start:index])
			out.WriteString(replacement)
			start = index + len(alias)
			continue
		}
		out.WriteString(name[start : index+len(alias)])
		start = index + len(alias)
	}
}

func exportFieldValue(env *record.Env, modelName string, row map[string]any, raw string) (any, error) {
	name := strings.Trim(normalizeExportFieldName(raw), "/")
	if name == "" {
		return "", nil
	}
	if name == "display_name" {
		return exportRecordDisplayName(env, modelName, int64Value(row["id"]))
	}
	if name == ".id" {
		return row["id"], nil
	}
	if name == "id" {
		return exportXMLID(env, modelName, int64Value(row["id"]))
	}
	base, rest, nested := strings.Cut(name, "/")
	if value, propMeta, ok, err := exportPropertyFieldValue(env, modelName, row, base); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if !nested {
			return exportPropertyValueForExport(env, propMeta, value, false)
		}
		if propMeta.Relation == "" {
			return "", nil
		}
		return exportRelatedFieldValue(env, propMeta.Relation, value, rest)
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return "", fmt.Errorf("model %s not found", modelName)
	}
	fieldMeta, ok := meta.Fields[base]
	if !ok {
		return "", nil
	}
	if !nested {
		if fieldMeta.Relation != "" {
			return exportRelationDisplayNames(env, fieldMeta.Relation, row[base])
		}
		return row[base], nil
	}
	if fieldMeta.Relation == "" {
		return "", nil
	}
	return exportRelatedFieldValue(env, fieldMeta.Relation, row[base], rest)
}

type exportPropertyMetadata struct {
	FieldName string
	Name      string
	Type      string
	Relation  string
	Default   any
	Selection map[string]string
	Tags      map[string]string
}

func exportPropertyPath(env *record.Env, modelName string, raw string) (string, string, field.Field, bool) {
	name := strings.Trim(normalizeExportFieldName(raw), "/")
	if strings.Contains(name, "/") {
		name, _, _ = strings.Cut(name, "/")
	}
	fieldName, propertyName, ok := strings.Cut(name, ".")
	if !ok || fieldName == "" || propertyName == "" || strings.Contains(propertyName, ".") {
		return "", "", field.Field{}, false
	}
	meta, exists := env.ModelMetadata(modelName)
	if !exists {
		return "", "", field.Field{}, false
	}
	fieldMeta, exists := meta.Fields[fieldName]
	if !exists || fieldMeta.Kind != field.Properties {
		return "", "", field.Field{}, false
	}
	return fieldName, propertyName, fieldMeta, true
}

func exportPropertyFieldValue(env *record.Env, modelName string, row map[string]any, raw string) (any, exportPropertyMetadata, bool, error) {
	fieldName, propertyName, fieldMeta, ok := exportPropertyPath(env, modelName, raw)
	if !ok {
		return nil, exportPropertyMetadata{}, false, nil
	}
	propMeta, found, err := exportPropertyMetadataForRecord(env, modelName, row, fieldName, propertyName, fieldMeta)
	if err != nil || !found {
		return "", propMeta, true, err
	}
	values := exportPropertyValueMap(row[fieldName])
	value, exists := values[propertyName]
	if !exists {
		value = propMeta.Default
	}
	return value, propMeta, true, nil
}

func exportPropertyMetadataForRecord(env *record.Env, modelName string, row map[string]any, fieldName string, propertyName string, fieldMeta field.Field) (exportPropertyMetadata, bool, error) {
	out := exportPropertyMetadata{FieldName: fieldName, Name: propertyName, Type: "char"}
	if fieldMeta.DefinitionRecord == "" || fieldMeta.DefinitionField == "" {
		return out, true, nil
	}
	definitionID := firstPropertyID(row[fieldMeta.DefinitionRecord])
	if definitionID == 0 {
		return out, true, nil
	}
	modelMeta, ok := env.ModelMetadata(modelName)
	if !ok {
		return out, false, fmt.Errorf("model %s not found", modelName)
	}
	definitionRecordMeta, ok := modelMeta.Fields[fieldMeta.DefinitionRecord]
	if !ok || definitionRecordMeta.Relation == "" {
		return out, true, nil
	}
	rows, err := env.Model(definitionRecordMeta.Relation).Browse(definitionID).Read(fieldMeta.DefinitionField)
	if err != nil {
		return out, false, err
	}
	if len(rows) == 0 {
		return out, true, nil
	}
	for _, definition := range exportPropertyDefinitionMaps(rows[0][fieldMeta.DefinitionField]) {
		if strings.TrimSpace(stringValue(definition["name"])) != propertyName {
			continue
		}
		propType := strings.TrimSpace(stringValue(definition["type"]))
		if propType != "" {
			out.Type = propType
		}
		out.Relation = strings.TrimSpace(firstNonEmptyHTTPString(stringValue(definition["comodel"]), stringValue(definition["relation"])))
		out.Default = definition["default"]
		out.Selection = exportPropertyLabelMap(definition["selection"])
		out.Tags = exportPropertyTagsMap(definition["tags"])
		return out, true, nil
	}
	return out, true, nil
}

func firstPropertyID(value any) int64 {
	switch typed := value.(type) {
	case []any:
		if len(typed) > 0 {
			return int64Value(typed[0])
		}
	case []int64:
		if len(typed) > 0 {
			return typed[0]
		}
	case [2]any:
		return int64Value(typed[0])
	}
	return int64Value(value)
}

func exportPropertyValueMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err == nil && out != nil {
			return out
		}
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

func exportPropertyValueForExport(env *record.Env, meta exportPropertyMetadata, value any, raw bool) (any, error) {
	if raw {
		return value, nil
	}
	switch meta.Type {
	case "many2one", "many2many":
		if meta.Relation == "" {
			return "", nil
		}
		return exportRelationDisplayNames(env, meta.Relation, propertyRelationIDs(value))
	case "selection":
		if label := meta.Selection[stringValue(value)]; label != "" {
			return label, nil
		}
	case "tags":
		ids := stringSliceValue(value)
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			if label := meta.Tags[id]; label != "" {
				parts = append(parts, label)
			}
		}
		return strings.Join(parts, ","), nil
	}
	return value, nil
}

func propertyRelationIDs(value any) []int64 {
	switch typed := value.(type) {
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			if pair, ok := item.([]any); ok && len(pair) > 0 {
				out = append(out, int64Value(pair[0]))
				continue
			}
			out = append(out, int64Value(item))
		}
		return positiveInt64Slice(out)
	case [][2]any:
		out := make([]int64, 0, len(typed))
		for _, pair := range typed {
			out = append(out, int64Value(pair[0]))
		}
		return positiveInt64Slice(out)
	default:
		return positiveInt64Slice(int64Slice(value))
	}
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringValue(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func exportPropertyLabelMap(value any) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]any:
		for key, label := range typed {
			out[key] = stringValue(label)
		}
	case []any:
		for _, item := range typed {
			if pair, ok := item.([]any); ok && len(pair) >= 2 {
				out[stringValue(pair[0])] = stringValue(pair[1])
			}
		}
	case [][2]string:
		for _, pair := range typed {
			out[pair[0]] = pair[1]
		}
	}
	return out
}

func exportPropertyTagsMap(value any) map[string]string {
	out := map[string]string{}
	for _, item := range exportPropertyDefinitionMaps(value) {
		key := stringValue(firstNonNil(item["id"], item["name"], item["value"]))
		label := stringValue(firstNonNil(item["label"], item["string"], item["display_name"]))
		if key != "" {
			out[key] = label
		}
	}
	if len(out) != 0 {
		return out
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if pair, ok := item.([]any); ok && len(pair) >= 2 {
				out[stringValue(pair[0])] = stringValue(pair[1])
			}
		}
	}
	return out
}

func exportRecordDisplayName(env *record.Env, modelName string, id int64) (string, error) {
	if env == nil || id <= 0 {
		return "", nil
	}
	names, err := env.Model(modelName).Browse(id).NameGet()
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return fmt.Sprint(names[0][1]), nil
}

func exportRelationDisplayNames(env *record.Env, modelName string, value any) (string, error) {
	ids := positiveInt64Slice(int64Slice(value))
	if len(ids) == 0 {
		return "", nil
	}
	names, err := env.Model(modelName).Browse(ids...).NameGet()
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(names))
	for _, pair := range names {
		text := strings.TrimSpace(fmt.Sprint(pair[1]))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, ","), nil
}

func exportRelatedFieldValue(env *record.Env, modelName string, value any, raw string) (any, error) {
	name := strings.Trim(normalizeExportFieldName(raw), "/")
	if name == "" {
		return "", nil
	}
	ids := positiveInt64Slice(int64Slice(value))
	if name == ".id" {
		return ids, nil
	}
	if name == "id" {
		return exportXMLIDs(env, modelName, ids)
	}
	if len(ids) == 0 {
		return "", nil
	}
	readFields := exportReadFields(env, modelName, []string{name})
	rows, err := env.Model(modelName).Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		resolved, err := exportFieldValue(env, modelName, row, name)
		if err != nil {
			return nil, err
		}
		text := exportCellString(resolved)
		if text != "" {
			values = append(values, text)
		}
	}
	return strings.Join(values, ","), nil
}

func exportXMLIDs(env *record.Env, modelName string, ids []int64) (string, error) {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		xmlID, err := exportXMLID(env, modelName, id)
		if err != nil {
			return "", err
		}
		if xmlID != "" {
			parts = append(parts, xmlID)
		}
	}
	return strings.Join(parts, ","), nil
}

func exportXMLID(env *record.Env, modelName string, id int64) (string, error) {
	if env == nil || id <= 0 {
		return "", nil
	}
	sudo := env.WithPolicy(nil)
	if _, err := sudo.Model("ir.model.data").FieldsGet([]string{"module"}, nil); err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.data") {
			return strconv.FormatInt(id, 10), nil
		}
		return "", err
	}
	if xmlID, err := exportFindXMLID(sudo, modelName, id); err != nil || xmlID != "" {
		return xmlID, err
	}
	return exportCreateXMLID(sudo, modelName, id)
}

func exportFindXMLID(env *record.Env, modelName string, id int64) (string, error) {
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("model", domain.Equal, modelName),
		domain.Cond("res_id", domain.Equal, id),
	))
	if err != nil {
		return "", err
	}
	rows, err := found.Read("module", "name")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	moduleName := strings.TrimSpace(stringValue(rows[0]["module"]))
	name := strings.TrimSpace(stringValue(rows[0]["name"]))
	if name == "" {
		return "", nil
	}
	if moduleName == "" {
		return name, nil
	}
	return moduleName + "." + name, nil
}

func exportCreateXMLID(env *record.Env, modelName string, id int64) (string, error) {
	meta, ok := env.ModelMetadata(modelName)
	table := strings.ReplaceAll(modelName, ".", "_")
	if ok && strings.TrimSpace(meta.Table) != "" {
		table = strings.TrimSpace(meta.Table)
	}
	for attempt := 0; attempt < 8; attempt++ {
		name := fmt.Sprintf("%s_%d_%s", table, id, exportRandomHex8())
		completeName := "__export__." + name
		exists, err := env.Model("ir.model.data").SearchWithOptions(domain.And(
			domain.Cond("module", domain.Equal, "__export__"),
			domain.Cond("name", domain.Equal, name),
		), record.SearchOptions{Limit: 1})
		if err != nil {
			return "", err
		}
		if len(exists.IDs()) != 0 {
			continue
		}
		_, err = env.Model("ir.model.data").Create(map[string]any{
			"module":        "__export__",
			"name":          name,
			"complete_name": completeName,
			"model":         modelName,
			"res_id":        id,
			"noupdate":      false,
		})
		if err != nil {
			if isExportXMLIDCollision(err) {
				continue
			}
			return "", err
		}
		return completeName, nil
	}
	return "", fmt.Errorf("could not allocate export xml id for %s:%d", modelName, id)
}

func isExportXMLIDCollision(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "already exists") || strings.Contains(text, "duplicate")
}

func exportRandomHex8() string {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(sum[:4])
	}
	return hex.EncodeToString(raw[:])
}

type exportGroupNode struct {
	Label    string
	Count    int
	Rows     []exportResolvedRow
	LeafRows []exportResolvedRow
	AnyRows  []exportAnyResolvedRow
	Children []*exportGroupNode
	index    map[string]*exportGroupNode
}

type exportGroupField struct {
	Name      string
	Interval  string
	Property  bool
	WeekStart int
	Timezone  *time.Location
}

func exportGroupedStringMatrix(env *record.Env, modelName string, ids []int64, fields []string, labels []string, groupBy []string) ([][]string, error) {
	groupSpecs, err := exportGroupFields(env, modelName, groupBy)
	if err != nil {
		return nil, err
	}
	if len(groupSpecs) == 0 {
		return exportFlatStringMatrix(env, modelName, ids, fields, labels, false)
	}
	groupFields := exportGroupFieldNames(groupSpecs)
	rows, err := exportResolvedRows(env, modelName, ids, fields, groupFields)
	if err != nil {
		return nil, err
	}
	expandedRows, err := exportResolvedFlatRows(env, modelName, ids, fields, false, groupFields)
	if err != nil {
		return nil, err
	}
	expandedByID := map[int64][]exportResolvedRow{}
	for _, row := range expandedRows {
		expandedByID[row.ID] = append(expandedByID[row.ID], row)
	}
	root := &exportGroupNode{}
	for _, row := range rows {
		leafRows := expandedByID[row.ID]
		if len(leafRows) == 0 {
			leafRows = []exportResolvedRow{row}
		}
		if err := root.insert(env, modelName, groupSpecs, row, leafRows, nil, 0); err != nil {
			return nil, err
		}
	}
	matrix := make([][]string, 0, len(rows)+len(root.Children)+1)
	matrix = append(matrix, labels)
	for _, child := range root.Children {
		matrix = appendGroupedRows(matrix, env, modelName, fields, child, 0)
	}
	return matrix, nil
}

func exportGroupedXLSXMatrix(env *record.Env, modelName string, ids []int64, fields []string, labels []string, groupBy []string) ([][]exportXLSXCell, error) {
	groupSpecs, err := exportGroupFields(env, modelName, groupBy)
	if err != nil {
		return nil, err
	}
	if len(groupSpecs) == 0 {
		rows, err := exportAnyFlatRows(env, modelName, ids, fields, false)
		if err != nil {
			return nil, err
		}
		if err := validateXLSXRowLimit(len(rows)); err != nil {
			return nil, err
		}
		fieldMetas := exportXLSXFieldMetadata(env, modelName, fields)
		matrix := make([][]exportXLSXCell, 0, len(rows)+1)
		matrix = append(matrix, exportXLSXHeaderRow(labels))
		for _, row := range rows {
			line := make([]exportXLSXCell, len(row))
			for index, value := range row {
				line[index] = exportXLSXBodyCell(value, fieldMetas[index], false)
			}
			matrix = append(matrix, line)
		}
		return matrix, nil
	}
	groupFields := exportGroupFieldNames(groupSpecs)
	rows, err := exportResolvedRows(env, modelName, ids, fields, groupFields)
	if err != nil {
		return nil, err
	}
	expandedRows, err := exportAnyResolvedFlatRows(env, modelName, ids, fields, false, groupFields)
	if err != nil {
		return nil, err
	}
	expandedByID := map[int64][]exportAnyResolvedRow{}
	for _, row := range expandedRows {
		expandedByID[row.ID] = append(expandedByID[row.ID], row)
	}
	root := &exportGroupNode{}
	for _, row := range rows {
		leafRows := expandedByID[row.ID]
		if len(leafRows) == 0 {
			leafRows = []exportAnyResolvedRow{{ID: row.ID, Raw: row.Raw, Cells: stringSliceToAny(row.Cells)}}
		}
		if err := root.insert(env, modelName, groupSpecs, row, nil, leafRows, 0); err != nil {
			return nil, err
		}
	}
	fieldMetas := exportXLSXFieldMetadata(env, modelName, fields)
	matrix := [][]exportXLSXCell{exportXLSXHeaderRow(labels)}
	for _, child := range root.Children {
		matrix = appendGroupedXLSXRows(matrix, env, modelName, fields, fieldMetas, child, 0)
	}
	if err := validateXLSXRowLimit(len(matrix) - 1); err != nil {
		return nil, err
	}
	return matrix, nil
}

func stringSliceToAny(values []string) []any {
	out := make([]any, len(values))
	for index, value := range values {
		out[index] = value
	}
	return out
}

func exportGroupFields(env *record.Env, modelName string, groupBy []string) ([]exportGroupField, error) {
	seen := map[string]bool{}
	out := []exportGroupField{}
	weekStart := exportGroupWeekStart(env)
	timezone := exportGroupTimezone(env)
	for _, raw := range groupBy {
		name := strings.TrimSpace(raw)
		interval := ""
		if cut := strings.Index(name, ":"); cut >= 0 {
			interval = strings.TrimSpace(name[cut+1:])
			name = name[:cut]
		}
		_, _, _, isProperty := exportPropertyPath(env, modelName, name)
		if isProperty && exportGroupNumberInterval(interval) {
			return nil, fmt.Errorf("export groupby interval %q is not supported for property field %s.%s", interval, modelName, name)
		}
		if !isProperty {
			if cut := strings.Index(name, "."); cut >= 0 {
				name = name[:cut]
			}
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, exportGroupField{Name: name, Interval: interval, Property: isProperty, WeekStart: weekStart, Timezone: timezone})
	}
	return out, nil
}

func exportGroupWeekStart(env *record.Env) int {
	if env == nil {
		return 7
	}
	langCode := strings.TrimSpace(fmt.Sprint(env.Context().Values["lang"]))
	if langCode == "" {
		langCode = "en_US"
	}
	if _, ok := env.ModelMetadata("res.lang"); !ok {
		return 7
	}
	found, err := env.Model("res.lang").SearchWithOptions(domain.Cond("code", domain.Equal, langCode), record.SearchOptions{Limit: 1})
	if err != nil || len(found.IDs()) == 0 {
		return 7
	}
	rows, err := found.Read("week_start")
	if err != nil || len(rows) == 0 {
		return 7
	}
	weekStart, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(rows[0]["week_start"])))
	if err != nil || weekStart < 1 || weekStart > 7 {
		return 7
	}
	return weekStart
}

func exportGroupTimezone(env *record.Env) *time.Location {
	if env == nil {
		return nil
	}
	timezone := strings.TrimSpace(fmt.Sprint(env.Context().Values["tz"]))
	if timezone == "" {
		return nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil
	}
	return location
}

func exportGroupFieldNames(groupFields []exportGroupField) []string {
	out := make([]string, 0, len(groupFields))
	for _, groupField := range groupFields {
		out = append(out, groupField.Name)
	}
	return out
}

func (node *exportGroupNode) insert(env *record.Env, modelName string, groupFields []exportGroupField, row exportResolvedRow, leafRows []exportResolvedRow, anyRows []exportAnyResolvedRow, depth int) error {
	node.Count++
	node.Rows = append(node.Rows, row)
	if depth >= len(groupFields) {
		node.LeafRows = append(node.LeafRows, leafRows...)
		node.AnyRows = append(node.AnyRows, anyRows...)
		return nil
	}
	groupField := groupFields[depth]
	rawValue, propMeta, err := exportGroupRawValue(env, modelName, groupField, row.Raw)
	if err != nil {
		return err
	}
	bucketValue := exportGroupBucketValue(env, modelName, groupField, rawValue, propMeta)
	label, err := exportGroupLabel(env, modelName, groupField, bucketValue, propMeta)
	if err != nil {
		return err
	}
	key := exportGroupKey(bucketValue)
	if node.index == nil {
		node.index = map[string]*exportGroupNode{}
	}
	child, ok := node.index[key]
	if !ok {
		child = &exportGroupNode{Label: label}
		node.index[key] = child
		node.Children = append(node.Children, child)
	}
	return child.insert(env, modelName, groupFields, row, leafRows, anyRows, depth+1)
}

func exportGroupRawValue(env *record.Env, modelName string, groupField exportGroupField, row map[string]any) (any, exportPropertyMetadata, error) {
	if !groupField.Property {
		return row[groupField.Name], exportPropertyMetadata{}, nil
	}
	value, propMeta, ok, err := exportPropertyFieldValue(env, modelName, row, groupField.Name)
	if err != nil || !ok {
		return nil, propMeta, err
	}
	return value, propMeta, nil
}

func exportGroupPropertyKind(meta exportPropertyMetadata) field.Kind {
	switch strings.ToLower(strings.TrimSpace(meta.Type)) {
	case "bool", "boolean":
		return field.Bool
	case "int", "integer":
		return field.Int
	case "float":
		return field.Float
	case "date":
		return field.Date
	case "datetime":
		return field.DateTime
	case "selection":
		return field.Selection
	case "many2one":
		return field.Many2One
	case "many2many":
		return field.Many2Many
	default:
		return field.Char
	}
}

func exportGroupFieldMeta(env *record.Env, modelName string, groupField exportGroupField, propMeta exportPropertyMetadata) (field.Field, bool) {
	if groupField.Property {
		return field.Field{Name: groupField.Name, Kind: exportGroupPropertyKind(propMeta), Relation: propMeta.Relation}, true
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return field.Field{}, false
	}
	fieldMeta, ok := meta.Fields[groupField.Name]
	return fieldMeta, ok
}

func exportGroupPropertyLabel(env *record.Env, meta exportPropertyMetadata, value any) (string, error) {
	switch strings.ToLower(strings.TrimSpace(meta.Type)) {
	case "many2one":
		if meta.Relation == "" {
			return "Undefined", nil
		}
		id := int64Value(value)
		if id == 0 {
			return "Undefined", nil
		}
		names, err := env.Model(meta.Relation).Browse(id).NameGet()
		if err != nil {
			return "", err
		}
		if len(names) > 0 {
			return stringValue(names[0][1]), nil
		}
	case "many2many":
		if meta.Relation == "" {
			return "Undefined", nil
		}
		label, err := exportRelationDisplayNames(env, meta.Relation, propertyRelationIDs(value))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(label) != "" {
			return label, nil
		}
	case "selection":
		if label := meta.Selection[stringValue(value)]; label != "" {
			return label, nil
		}
	case "tags":
		ids := stringSliceValue(value)
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			if label := meta.Tags[id]; label != "" {
				parts = append(parts, label)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ","), nil
		}
	}
	label := exportCellString(value)
	if strings.TrimSpace(label) == "" {
		return "Undefined", nil
	}
	return label, nil
}

func exportGroupBucketValue(env *record.Env, modelName string, groupField exportGroupField, value any, propMeta exportPropertyMetadata) any {
	if strings.TrimSpace(groupField.Interval) == "" || isEmptyGroupValue(value) {
		return value
	}
	fieldMeta, ok := exportGroupFieldMeta(env, modelName, groupField, propMeta)
	if !ok || (fieldMeta.Kind != field.Date && fieldMeta.Kind != field.DateTime) {
		return value
	}
	dateValue, ok := exportXLSXDateValue(value, fieldMeta.Kind)
	if !ok || dateValue.IsZero() {
		return value
	}
	location := time.UTC
	if fieldMeta.Kind == field.DateTime && groupField.Timezone != nil {
		dateValue = dateValue.UTC().In(groupField.Timezone)
		location = groupField.Timezone
	} else {
		dateValue = dateValue.UTC()
	}
	if exportGroupNumberInterval(groupField.Interval) {
		return exportGroupDatePartNumber(dateValue, groupField.Interval, fieldMeta.Kind)
	}
	switch strings.ToLower(strings.TrimSpace(groupField.Interval)) {
	case "day":
		return time.Date(dateValue.Year(), dateValue.Month(), dateValue.Day(), 0, 0, 0, 0, location)
	case "week":
		start := exportGroupWeekBucketStart(dateValue, groupField.WeekStart)
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	case "month":
		return time.Date(dateValue.Year(), dateValue.Month(), 1, 0, 0, 0, 0, location)
	case "quarter":
		month := time.Month(((int(dateValue.Month()) - 1) / 3 * 3) + 1)
		return time.Date(dateValue.Year(), month, 1, 0, 0, 0, 0, location)
	case "year":
		return time.Date(dateValue.Year(), 1, 1, 0, 0, 0, 0, location)
	default:
		return value
	}
}

func exportGroupNumberInterval(interval string) bool {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "year_number", "quarter_number", "month_number", "iso_week_number", "day_of_year", "day_of_month", "day_of_week", "hour_number", "minute_number", "second_number":
		return true
	default:
		return false
	}
}

func exportGroupDatePartNumber(value time.Time, interval string, kind field.Kind) int {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "year_number":
		return value.Year()
	case "quarter_number":
		return ((int(value.Month()) - 1) / 3) + 1
	case "month_number":
		return int(value.Month())
	case "iso_week_number":
		_, week := value.ISOWeek()
		return week
	case "day_of_year":
		return value.YearDay()
	case "day_of_month":
		return value.Day()
	case "day_of_week":
		return int(value.Weekday())
	case "hour_number":
		if kind == field.Date {
			return 0
		}
		return value.Hour()
	case "minute_number":
		if kind == field.Date {
			return 0
		}
		return value.Minute()
	case "second_number":
		if kind == field.Date {
			return 0
		}
		return value.Second()
	default:
		return 0
	}
}

func exportGroupLabel(env *record.Env, modelName string, groupField exportGroupField, value any, propMeta exportPropertyMetadata) (string, error) {
	fieldMeta, ok := exportGroupFieldMeta(env, modelName, groupField, propMeta)
	if !ok {
		return "", fmt.Errorf("unknown groupby field %s.%s", modelName, groupField.Name)
	}
	if fieldMeta.Kind == field.Bool {
		return exportCellString(firstNonNil(value, false)), nil
	}
	if isEmptyGroupValue(value) {
		return "Undefined", nil
	}
	if (fieldMeta.Kind == field.Date || fieldMeta.Kind == field.DateTime) && strings.TrimSpace(groupField.Interval) != "" {
		if dateValue, ok := exportXLSXDateValue(value, fieldMeta.Kind); ok {
			return exportTemporalGroupLabel(dateValue, groupField.Interval, groupField.WeekStart, exportGroupLabelTimezone(fieldMeta.Kind, groupField)), nil
		}
	}
	if groupField.Property {
		return exportGroupPropertyLabel(env, propMeta, value)
	}
	if fieldMeta.Kind == field.Many2One && fieldMeta.Relation != "" {
		id := int64Value(value)
		if id == 0 {
			return "Undefined", nil
		}
		names, err := env.Model(fieldMeta.Relation).Browse(id).NameGet()
		if err != nil {
			return "", err
		}
		if len(names) > 0 {
			return stringValue(names[0][1]), nil
		}
	}
	label := exportCellString(value)
	if strings.TrimSpace(label) == "" {
		return "Undefined", nil
	}
	return label, nil
}

func appendGroupedXLSXRows(matrix [][]exportXLSXCell, env *record.Env, modelName string, fields []string, fieldMetas []field.Field, node *exportGroupNode, depth int) [][]exportXLSXCell {
	line := make([]exportXLSXCell, len(fields))
	if len(line) > 0 {
		line[0] = exportXLSXCell{Value: fmt.Sprintf("%s%s (%d)", strings.Repeat("    ", depth), node.Label, node.Count), Kind: field.Char, Style: xlsxStyleGroupHeader}
	}
	for colIndex := 1; colIndex < len(fields); colIndex++ {
		if aggregate, meta, ok := exportAggregateAnyValue(env, modelName, fields[colIndex], node.Rows); ok {
			style := xlsxStyleGroupHeader
			if meta.Kind == field.Float || meta.Kind == field.Decimal {
				style = xlsxStyleGroupHeaderFloat
			} else if meta.Kind == field.Date {
				style = xlsxStyleDate
			} else if meta.Kind == field.DateTime {
				style = xlsxStyleDateTime
			}
			line[colIndex] = exportXLSXCell{Value: aggregate, Kind: meta.Kind, Style: style}
		}
	}
	matrix = append(matrix, line)
	for _, child := range node.Children {
		matrix = appendGroupedXLSXRows(matrix, env, modelName, fields, fieldMetas, child, depth+1)
	}
	for _, row := range node.AnyRows {
		line := make([]exportXLSXCell, len(row.Cells))
		for index, value := range row.Cells {
			meta := field.Field{}
			if index < len(fieldMetas) {
				meta = fieldMetas[index]
			}
			line[index] = exportXLSXBodyCell(value, meta, false)
		}
		matrix = append(matrix, line)
	}
	return matrix
}

func appendGroupedRows(matrix [][]string, env *record.Env, modelName string, fields []string, node *exportGroupNode, depth int) [][]string {
	line := make([]string, len(fields))
	if len(line) > 0 {
		line[0] = fmt.Sprintf("%s%s (%d)", strings.Repeat("    ", depth), node.Label, node.Count)
	}
	for colIndex := 1; colIndex < len(fields); colIndex++ {
		if aggregate, ok := exportAggregateValue(env, modelName, fields[colIndex], node.Rows); ok {
			line[colIndex] = aggregate
		}
	}
	matrix = append(matrix, line)
	for _, child := range node.Children {
		matrix = appendGroupedRows(matrix, env, modelName, fields, child, depth+1)
	}
	for _, row := range node.LeafRows {
		matrix = append(matrix, row.Cells)
	}
	return matrix
}

func exportGroupKey(value any) string {
	return fmt.Sprintf("%T:%v", value, value)
}

func isEmptyGroupValue(value any) bool {
	if value == nil {
		return true
	}
	if typed, ok := value.(bool); ok && !typed {
		return true
	}
	if typed, ok := value.(string); ok && typed == "" {
		return true
	}
	return false
}

func exportGroupWeekBucketStart(value time.Time, weekStart int) time.Time {
	if weekStart < 1 || weekStart > 7 {
		weekStart = 7
	}
	target := time.Weekday(weekStart % 7)
	diff := (int(value.Weekday()) - int(target) + 7) % 7
	return value.AddDate(0, 0, -diff)
}

func exportGroupLabelTimezone(kind field.Kind, groupField exportGroupField) *time.Location {
	if kind != field.DateTime || groupField.Timezone == nil {
		return nil
	}
	return groupField.Timezone
}

func exportTemporalGroupLabel(value time.Time, interval string, weekStart int, timezone *time.Location) string {
	if timezone != nil {
		value = value.In(timezone)
	} else {
		value = value.UTC()
	}
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "day":
		return value.Format("January 2, 2006")
	case "week":
		year, week := exportGroupWeekNumber(value, weekStart)
		return fmt.Sprintf("W%d %04d", week, year)
	case "month":
		return value.Format("January 2006")
	case "quarter":
		return fmt.Sprintf("Q%d %d", ((int(value.Month())-1)/3)+1, value.Year())
	case "year":
		return strconv.Itoa(value.Year())
	default:
		return exportCellString(value)
	}
}

func exportGroupWeekNumber(value time.Time, weekStart int) (int, int) {
	if weekStart < 1 || weekStart > 7 {
		weekStart = 7
	}
	if weekStart == 1 {
		year, week := value.ISOWeek()
		return year, week
	}
	location := value.Location()
	if location == nil {
		location = time.UTC
	}
	nextYearStart := time.Date(value.Year()+1, 1, 1, 0, 0, 0, 0, location)
	firstNextYearWeekStart := exportGroupWeekBucketStart(nextYearStart, weekStart)
	if !value.Before(firstNextYearWeekStart) {
		return value.Year() + 1, 1
	}
	currentYearStart := time.Date(value.Year(), 1, 1, 0, 0, 0, 0, location)
	firstCurrentYearWeekStart := exportGroupWeekBucketStart(currentYearStart, weekStart)
	days := exportCalendarDayDiff(firstCurrentYearWeekStart, value)
	return value.Year(), days/7 + 1
}

func exportCalendarDayDiff(start time.Time, end time.Time) int {
	startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(endDate.Sub(startDate).Hours() / 24)
}

func exportAggregateValue(env *record.Env, modelName string, fieldName string, rows []exportResolvedRow) (string, bool) {
	value, _, ok := exportAggregateAnyValue(env, modelName, fieldName, rows)
	if !ok {
		return "", false
	}
	return exportCellString(value), true
}

func exportAggregateAnyValue(env *record.Env, modelName string, fieldName string, rows []exportResolvedRow) (any, field.Field, bool) {
	if fieldName == "" || strings.Contains(fieldName, "/") || fieldName == ".id" {
		return nil, field.Field{}, false
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, field.Field{}, false
	}
	fieldMeta, ok := meta.Fields[fieldName]
	if !ok || fieldMeta.Aggregator == "" {
		return nil, field.Field{}, false
	}
	switch fieldMeta.Aggregator {
	case "sum":
		total, _, integral, ok := exportNumericAggregate(rows, fieldName)
		if !ok {
			return nil, field.Field{}, false
		}
		if integral {
			return int64(total), fieldMeta, true
		}
		return total, fieldMeta, true
	case "avg":
		total, count, _, ok := exportNumericAggregate(rows, fieldName)
		if !ok || count == 0 {
			return nil, field.Field{}, false
		}
		return total / float64(count), fieldMeta, true
	case "max", "min":
		value, ok := exportMinMaxAggregate(rows, fieldName, fieldMeta.Aggregator == "max")
		if !ok {
			return nil, field.Field{}, false
		}
		return value, fieldMeta, true
	case "bool_and", "bool_or":
		value, ok := exportBoolAggregate(rows, fieldName, fieldMeta.Aggregator == "bool_and")
		if !ok {
			return nil, field.Field{}, false
		}
		return value, fieldMeta, true
	default:
		return nil, field.Field{}, false
	}
}

func exportNumericAggregate(rows []exportResolvedRow, fieldName string) (float64, int, bool, bool) {
	total := 0.0
	integral := true
	seen := false
	count := 0
	for _, row := range rows {
		value, ok := exportFloat64(row.Raw[fieldName])
		if !ok {
			continue
		}
		if _, ok := row.Raw[fieldName].(float64); ok {
			integral = false
		}
		total += value
		seen = true
		count++
	}
	return total, count, integral, seen
}

func exportMinMaxAggregate(rows []exportResolvedRow, fieldName string, maximum bool) (any, bool) {
	var selected any
	selectedNumber := 0.0
	seenNumber := false
	for _, row := range rows {
		value := row.Raw[fieldName]
		number, isNumber := exportFloat64(value)
		if isNumber {
			if !seenNumber || (maximum && number > selectedNumber) || (!maximum && number < selectedNumber) {
				selectedNumber = number
				selected = value
				seenNumber = true
			}
			continue
		}
		if selected == nil {
			selected = value
		}
	}
	return selected, selected != nil
}

func exportBoolAggregate(rows []exportResolvedRow, fieldName string, and bool) (bool, bool) {
	if len(rows) == 0 {
		return false, false
	}
	value := and
	for _, row := range rows {
		current := accountingBoolValue(row.Raw[fieldName])
		if and {
			value = value && current
		} else {
			value = value || current
		}
	}
	return value, true
}

func exportFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func exportNumberString(value float64, integral bool) string {
	if integral {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func exportCSVSafeCell(value string) string {
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return "'" + value
	}
	return value
}

func exportXLSXHeaderRow(labels []string) []exportXLSXCell {
	row := make([]exportXLSXCell, len(labels))
	for index, label := range labels {
		row[index] = exportXLSXCell{Value: label, Kind: field.Char, Style: xlsxStyleHeader}
	}
	return row
}

func exportStringXLSXMatrix(matrix [][]string) [][]exportXLSXCell {
	out := make([][]exportXLSXCell, len(matrix))
	for rowIndex, row := range matrix {
		out[rowIndex] = make([]exportXLSXCell, len(row))
		for colIndex, value := range row {
			style := xlsxStyleBase
			if rowIndex == 0 {
				style = xlsxStyleHeader
			} else if colIndex == 0 && strings.Contains(value, " (") {
				style = xlsxStyleGroupHeader
			}
			out[rowIndex][colIndex] = exportXLSXCell{Value: value, Kind: field.Char, Style: style}
		}
	}
	return out
}

func exportXLSXFieldMetadata(env *record.Env, modelName string, fields []string) []field.Field {
	metas := make([]field.Field, len(fields))
	for index, name := range fields {
		if meta, ok := exportXLSXFieldMeta(env, modelName, name); ok {
			metas[index] = meta
		}
	}
	return metas
}

func exportXLSXFieldMeta(env *record.Env, modelName string, raw string) (field.Field, bool) {
	name := strings.Trim(normalizeExportFieldName(raw), "/")
	switch name {
	case "":
		return field.Field{}, false
	case ".id":
		return field.Field{Name: ".id", Kind: field.Int}, true
	case "id", "display_name":
		return field.Field{Name: name, Kind: field.Char}, true
	}
	if _, _, _, ok := exportPropertyPath(env, modelName, name); ok {
		return field.Field{Name: name, Kind: field.Char}, true
	}
	base, rest, nested := strings.Cut(name, "/")
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return field.Field{}, false
	}
	fieldMeta, ok := meta.Fields[base]
	if !ok {
		return field.Field{}, false
	}
	if nested {
		if fieldMeta.Relation == "" {
			return field.Field{Name: name, Kind: field.Char}, true
		}
		return exportXLSXFieldMeta(env, fieldMeta.Relation, rest)
	}
	if fieldMeta.Relation != "" {
		return field.Field{Name: fieldMeta.Name, Kind: field.Char}, true
	}
	return fieldMeta, true
}

func exportXLSXBodyCell(value any, meta field.Field, groupedHeader bool) exportXLSXCell {
	style := xlsxStyleBase
	if groupedHeader {
		style = xlsxStyleGroupHeader
	} else {
		switch meta.Kind {
		case field.Date:
			style = xlsxStyleDate
		case field.DateTime:
			style = xlsxStyleDateTime
		case field.Float, field.Decimal:
			style = xlsxStyleFloat
		}
	}
	return exportXLSXCell{Value: value, Kind: meta.Kind, Style: style}
}

func exportXLSXDecimalPlaces(env *record.Env) int {
	if env == nil {
		return 2
	}
	rows, err := env.Model("res.currency").Search(domain.And())
	if err != nil {
		return 2
	}
	values, err := rows.Read("decimal_places")
	if err != nil {
		return 2
	}
	maxPlaces := int64(0)
	for _, row := range values {
		places := int64Value(row["decimal_places"])
		if places > maxPlaces {
			maxPlaces = places
		}
	}
	if maxPlaces <= 0 {
		return 2
	}
	return int(maxPlaces)
}

func exportStylesXML(decimalPlaces int) string {
	if decimalPlaces <= 0 {
		decimalPlaces = 2
	}
	decimalFormat := "#,##0." + strings.Repeat("0", decimalPlaces)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<numFmts count="3">` +
		`<numFmt numFmtId="164" formatCode="yyyy-mm-dd"/>` +
		`<numFmt numFmtId="165" formatCode="yyyy-mm-dd hh:mm:ss"/>` +
		`<numFmt numFmtId="166" formatCode="` + decimalFormat + `"/>` +
		`</numFmts>` +
		`<fonts count="2">` +
		`<font><sz val="11"/><name val="Calibri"/></font>` +
		`<font><b/><sz val="11"/><name val="Calibri"/></font>` +
		`</fonts>` +
		`<fills count="3">` +
		`<fill><patternFill patternType="none"/></fill>` +
		`<fill><patternFill patternType="gray125"/></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FFE9ECEF"/><bgColor indexed="64"/></patternFill></fill>` +
		`</fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="7">` +
		`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`<xf numFmtId="166" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1" applyNumberFormat="1" applyAlignment="1"><alignment wrapText="1"/></xf>` +
		`</cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
		`</styleSheet>`
}

func exportSheetXML(matrix [][]exportXLSXCell) (string, error) {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	builder.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	if columns := exportXLSXMaxColumns(matrix); columns > 0 {
		builder.WriteString(`<cols><col min="1" max="`)
		builder.WriteString(strconv.Itoa(columns))
		builder.WriteString(`" width="30" customWidth="1"/></cols>`)
	}
	builder.WriteString(`<sheetData>`)
	for rowIndex, row := range matrix {
		builder.WriteString(`<row r="`)
		builder.WriteString(strconv.Itoa(rowIndex + 1))
		builder.WriteString(`">`)
		for colIndex, cell := range row {
			if err := exportXLSXCellXML(&builder, colIndex, rowIndex, cell, exportXLSXHeaderLabel(matrix, colIndex)); err != nil {
				return "", err
			}
		}
		builder.WriteString(`</row>`)
	}
	builder.WriteString(`</sheetData></worksheet>`)
	return builder.String(), nil
}

func exportXLSXMaxColumns(matrix [][]exportXLSXCell) int {
	maxColumns := 0
	for _, row := range matrix {
		if len(row) > maxColumns {
			maxColumns = len(row)
		}
	}
	return maxColumns
}

func exportXLSXHeaderLabel(matrix [][]exportXLSXCell, colIndex int) string {
	if len(matrix) == 0 || colIndex < 0 || colIndex >= len(matrix[0]) {
		return ""
	}
	text, err := exportXLSXText(matrix[0][colIndex].Value)
	if err != nil {
		return ""
	}
	return text
}

func exportXLSXCellXML(builder *strings.Builder, colIndex int, rowIndex int, cell exportXLSXCell, headerLabel string) error {
	builder.WriteString(`<c r="`)
	builder.WriteString(xlsxCellRef(colIndex, rowIndex))
	builder.WriteString(`" s="`)
	builder.WriteString(strconv.Itoa(cell.Style))
	builder.WriteString(`"`)
	if cell.Kind == field.Date || cell.Kind == field.DateTime {
		if serial, ok := exportXLSXDateSerial(cell.Value, cell.Kind); ok {
			builder.WriteString(`><v>`)
			builder.WriteString(strconv.FormatFloat(serial, 'f', -1, 64))
			builder.WriteString(`</v></c>`)
			return nil
		}
	}
	if value, ok := exportXLSXNumericValue(cell.Value); ok {
		builder.WriteString(`><v>`)
		builder.WriteString(value)
		builder.WriteString(`</v></c>`)
		return nil
	}
	if typed, ok := cell.Value.(bool); ok {
		builder.WriteString(` t="b"><v>`)
		if typed {
			builder.WriteString(`1`)
		} else {
			builder.WriteString(`0`)
		}
		builder.WriteString(`</v></c>`)
		return nil
	}
	builder.WriteString(` t="inlineStr"><is><t`)
	text, err := exportXLSXText(cell.Value)
	if err != nil {
		if strings.TrimSpace(headerLabel) != "" {
			return fmt.Errorf("%s for %s.", err.Error(), headerLabel)
		}
		return err
	}
	if text != strings.TrimSpace(text) {
		builder.WriteString(` xml:space="preserve"`)
	}
	builder.WriteString(`>`)
	xml.EscapeText(builder, []byte(text))
	builder.WriteString(`</t></is></c>`)
	return nil
}

func exportXLSXNumericValue(value any) (string, bool) {
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

func exportXLSXText(value any) (string, error) {
	var text string
	switch typed := value.(type) {
	case []byte:
		if !utf8.Valid(typed) {
			return "", fmt.Errorf("Binary fields can not be exported to Excel unless their content is base64-encoded. That does not seem to be the case")
		}
		text = string(typed)
	default:
		text = exportCellString(value)
	}
	text = strings.ReplaceAll(text, "\r", " ")
	if len([]rune(text)) > xlsxMaxStringLength {
		text = fmt.Sprintf("The content of this cell is too long for an XLSX file (more than %d characters). Please use the CSV format for this export.", xlsxMaxStringLength)
	}
	return text, nil
}

func exportXLSXDateSerial(value any, kind field.Kind) (float64, bool) {
	dateValue, ok := exportXLSXDateValue(value, kind)
	if !ok || dateValue.IsZero() {
		return 0, false
	}
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return dateValue.UTC().Sub(epoch).Hours() / 24, true
}

func exportXLSXDateValue(value any, kind field.Kind) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}, false
		}
		layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
		if kind == field.Date {
			layouts = []string{"2006-01-02"}
		}
		for _, layout := range layouts {
			parsed, err := time.Parse(layout, text)
			if err == nil {
				return parsed, true
			}
		}
		return time.Time{}, false
	default:
		return time.Time{}, false
	}
}

func xlsxCellRef(colIndex int, rowIndex int) string {
	col := ""
	n := colIndex
	for {
		col = string(rune('A'+(n%26))) + col
		n = n/26 - 1
		if n < 0 {
			break
		}
	}
	return col + strconv.Itoa(rowIndex+1)
}

func exportCellString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case []int64:
		parts := make([]string, 0, len(typed))
		for _, id := range typed {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
		return strings.Join(parts, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, exportCellString(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(value)
	}
}

func (s Server) findActionPayload(key string) (map[string]any, bool) {
	key = strings.TrimSpace(key)
	if modelName, id, ok := s.concreteActionRef(key); ok {
		return s.concreteActionPayload(modelName, id)
	}
	if modelName, id, ok := s.globalActionRef(key); ok {
		return s.concreteActionPayload(modelName, id)
	}
	action, ok := s.findAction(key)
	if ok {
		return actionPayload(action), true
	}
	if modelName, id, ok := s.externalIDActionRef(key); ok {
		return s.concreteActionPayload(modelName, id)
	}
	return nil, false
}

func (s Server) externalIDActionRef(key string) (string, int64, bool) {
	external, ok := s.actionExternalID(key, "")
	if !ok {
		return "", 0, false
	}
	if external.Model == "ir.actions.actions" {
		return s.globalActionRef(strconv.FormatInt(external.ResID, 10))
	}
	switch external.Model {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return external.Model, external.ResID, true
	default:
		return "", 0, false
	}
}

func (s Server) concreteActionRef(key string) (string, int64, bool) {
	modelName, rawID, ok := splitConcreteActionRef(key)
	if !ok {
		return "", 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err == nil && id > 0 {
		return modelName, id, true
	}
	external, ok := s.actionExternalID(rawID, modelName)
	if !ok {
		return "", 0, false
	}
	if external.Model == "ir.actions.actions" {
		actualModel, id, ok := s.globalActionRef(strconv.FormatInt(external.ResID, 10))
		if ok && actualModel == modelName {
			return modelName, id, true
		}
		return "", 0, false
	}
	if external.Model != modelName || external.ResID <= 0 {
		return "", 0, false
	}
	return modelName, external.ResID, true
}

func (s Server) actionExternalID(key string, modelName string) (data.ExternalID, bool) {
	key = strings.TrimSpace(key)
	if key == "" || len(s.ExternalIDs) == 0 {
		return data.ExternalID{}, false
	}
	if strings.Contains(key, ".") {
		external, ok := s.ExternalIDs[key]
		if !ok || !s.actionExternalIDMatches(external, modelName) {
			return data.ExternalID{}, false
		}
		return external, true
	}
	names := make([]string, 0, len(s.ExternalIDs))
	for name := range s.ExternalIDs {
		names = append(names, name)
	}
	sort.Strings(names)
	var matched data.ExternalID
	for _, name := range names {
		external := s.ExternalIDs[name]
		if external.Name != key || !s.actionExternalIDMatches(external, modelName) {
			continue
		}
		if matched.ResID != 0 {
			return data.ExternalID{}, false
		}
		matched = external
	}
	return matched, matched.ResID != 0
}

func (s Server) actionExternalIDMatches(external data.ExternalID, modelName string) bool {
	if external.ResID <= 0 {
		return false
	}
	if modelName != "" {
		if external.Model == modelName {
			return true
		}
		if external.Model == "ir.actions.actions" {
			actualModel, _, ok := s.globalActionRef(strconv.FormatInt(external.ResID, 10))
			return ok && actualModel == modelName
		}
		return false
	}
	if external.Model == "ir.actions.actions" {
		_, _, ok := s.globalActionRef(strconv.FormatInt(external.ResID, 10))
		return ok
	}
	return isConcreteActionModelName(external.Model)
}

func (s Server) globalActionRef(key string) (string, int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(key), 10, 64)
	if err != nil || id <= 0 || s.Env == nil {
		return "", 0, false
	}
	if _, ok := s.Env.ModelMetadata("ir.actions.actions"); !ok {
		return "", 0, false
	}
	rows, err := s.Env.Model("ir.actions.actions").Browse(id).Read("id", "type")
	if err != nil || len(rows) == 0 || int64Value(rows[0]["id"]) == 0 {
		return "", 0, false
	}
	modelName := stringValue(rows[0]["type"])
	switch modelName {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return modelName, id, true
	default:
		return "", 0, false
	}
}

func (s Server) findAction(key string) (action.Action, bool) {
	key = strings.TrimSpace(key)
	if s.Actions == nil || key == "" || key == "<nil>" {
		return action.Action{}, false
	}
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		return s.Actions.Get(id)
	}
	if strings.Contains(key, ".") {
		return s.Actions.FindByXMLID(key)
	}
	return s.Actions.FindByPath(key)
}

func parseConcreteActionRef(key string) (string, int64, bool) {
	modelName, rawID, ok := splitConcreteActionRef(key)
	if !ok {
		return "", 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return modelName, id, true
}

func splitConcreteActionRef(key string) (string, string, bool) {
	modelName, rawID, ok := strings.Cut(strings.TrimSpace(key), ",")
	if !ok {
		return "", "", false
	}
	modelName = strings.TrimSpace(modelName)
	rawID = strings.TrimSpace(rawID)
	if !isConcreteActionModelName(modelName) || rawID == "" {
		return "", "", false
	}
	return modelName, rawID, true
}

func isConcreteActionModelName(modelName string) bool {
	switch modelName {
	case "ir.actions.act_window", "ir.actions.act_window_close", "ir.actions.act_url", "ir.actions.server", "ir.actions.report", "ir.actions.client":
		return true
	default:
		return false
	}
}

func (s Server) concreteActionPayload(modelName string, id int64) (map[string]any, bool) {
	if s.Env == nil {
		return nil, false
	}
	reader := s
	if modelName == "ir.actions.report" {
		ctx := s.Env.Context()
		ctx.Values = cloneContextValues(ctx.Values)
		ctx.Values["bin_size"] = true
		reader.Env = s.Env.WithContext(ctx)
	}
	meta, ok := reader.Env.ModelMetadata(modelName)
	if !ok {
		return nil, false
	}
	fields := concreteActionFields(meta, modelName)
	rows, err := reader.Env.Model(modelName).Browse(id).Read(fields...)
	if err != nil || len(rows) == 0 || int64Value(rows[0]["id"]) == 0 {
		return nil, false
	}
	row := rows[0]
	payload := reader.actionCommonPayload(modelName, id, row)
	for _, name := range fields {
		if name == "id" || actionCommonReadableField(name) {
			continue
		}
		if value, exists := row[name]; exists {
			payload[name] = reader.actionReadableFieldValue(modelName, name, value)
		}
	}
	if modelName == "ir.actions.act_window" {
		payload["embedded_action_ids"] = reader.contextEmbeddedActionIDs(id)
		if err := generatePersistedActionViews(payload); err != nil {
			return nil, false
		}
	}
	enrichWebShellActionPayload(reader.Env, payload)
	return payload, true
}

func enrichWebShellActionPayload(env *record.Env, payload map[string]any) {
	if payload == nil {
		return
	}
	if strings.TrimSpace(stringValue(payload["type"])) != "ir.actions.act_window" {
		return
	}
	modelName := strings.TrimSpace(stringValue(payload["res_model"]))
	payload["_web_domain"] = normalizedActionDomainValue(env, modelName, payload["domain"])
	payload["_web_context"] = normalizedActionContextValue(env, modelName, payload["context"])
}

func normalizedActionDomainValue(env *record.Env, modelName string, raw any) []any {
	switch typed := raw.(type) {
	case nil:
		return []any{}
	case bool:
		return []any{}
	case []any:
		if _, err := parseDomain(typed); err == nil {
			return webShellDomainItems(typed)
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []any{}
		}
		if value, err := data.SafeEvalExpression(text, data.SafeEvalOptions{Env: env, Model: modelName}); err == nil {
			if _, err := parseDomain(value); err == nil {
				return webShellDomainItems(value)
			}
		}
		if value, err := domain.ParseLiteralValue(text); err == nil {
			if _, err := parseDomain(value); err == nil {
				return webShellDomainItems(value)
			}
		}
	default:
		if _, err := parseDomain(raw); err == nil {
			return webShellDomainItems(raw)
		}
	}
	return []any{}
}

func webShellDomainItems(value any) []any {
	items := domainItems(value)
	if items == nil {
		return []any{}
	}
	return items
}

func normalizedActionContextValue(env *record.Env, modelName string, raw any) map[string]any {
	switch typed := raw.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return cloneHTTPMap(typed)
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(text), &decoded); err == nil {
			return cloneHTTPMap(decoded)
		}
		if value, err := data.SafeEvalExpression(text, data.SafeEvalOptions{Env: env, Model: modelName}); err == nil {
			return cloneHTTPMap(mapValue(value))
		}
	}
	return map[string]any{}
}

func (s Server) cleanActionResult(env *record.Env, result any) any {
	cleaned, _ := s.cleanActionResultWithError(env, result)
	return cleaned
}

func (s Server) cleanActionResultWithError(env *record.Env, result any) (any, error) {
	payload, ok := result.(map[string]any)
	if !ok {
		return result, nil
	}
	actionType := strings.TrimSpace(stringValue(payload["type"]))
	if actionType == "" {
		return result, nil
	}
	cleaner := s
	if env != nil {
		cleaner.Env = env
	}
	return cleaner.cleanActionPayloadMap(actionType, payload)
}

func actionRunResultPayload(server Server, env *record.Env, result serveractions.Result) (any, error) {
	if actionPayload := mapValue(result.Action); len(actionPayload) > 0 {
		return server.cleanActionResultWithError(env, actionPayload)
	}
	if actionPayload := mapValue(result.Metadata["action"]); len(actionPayload) > 0 {
		return server.cleanActionResultWithError(env, actionPayload)
	}
	if actionPayload := mapValue(result.Metadata["result"]); len(actionPayload) > 0 && stringValue(actionPayload["type"]) != "" {
		return server.cleanActionResultWithError(env, actionPayload)
	}
	if stringValue(result.Metadata["type"]) != "" {
		return server.cleanActionResultWithError(env, result.Metadata)
	}
	return false, nil
}

func (s Server) cleanActionPayloadMap(modelName string, payload map[string]any) (map[string]any, error) {
	if s.Env == nil {
		return payload, nil
	}
	meta, ok := s.Env.ModelMetadata(modelName)
	if !ok {
		return payload, nil
	}
	working := cloneHTTPMap(payload)
	if modelName == "ir.actions.act_window" {
		if err := generateActionViews(working); err != nil {
			return nil, err
		}
	}
	out := make(map[string]any, len(working))
	for name, value := range working {
		if _, exists := meta.Fields[name]; exists && !actionReadableFieldAllowed(modelName, name) {
			continue
		}
		out[name] = value
	}
	return out, nil
}

func generateActionViews(actionPayload map[string]any) error {
	return generateActionViewsWithMode(actionPayload, true)
}

func generatePersistedActionViews(actionPayload map[string]any) error {
	return generateActionViewsWithMode(actionPayload, false)
}

func generateActionViewsWithMode(actionPayload map[string]any, strictViewID bool) error {
	if raw, exists := actionPayload["views"]; exists && !emptyActionViews(raw) {
		return nil
	}
	viewMode := firstNonEmptyHTTPString(stringValue(actionPayload["view_mode"]), "list,form")
	modes := splitViewModes(viewMode)
	if len(modes) == 0 {
		return nil
	}
	viewID := actionViewID(actionPayload["view_id"])
	views := make([]any, 0, len(modes))
	if len(modes) > 1 && viewID == 0 {
		for _, mode := range modes {
			views = append(views, []any{false, mode})
		}
		actionPayload["views"] = views
		return nil
	}
	if len(modes) > 1 && viewID != 0 {
		if strictViewID {
			return fmt.Errorf("non-db action dictionaries should provide either multiple view modes or a single view mode and an optional view id: got view modes %v and view id %d", modes, viewID)
		}
		for _, mode := range modes {
			views = append(views, []any{false, mode})
		}
		actionPayload["views"] = views
		return nil
	}
	if len(modes) == 1 {
		views = append(views, []any{falseIfZero(viewID), modes[0]})
		actionPayload["views"] = views
	}
	return nil
}

func emptyActionViews(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case bool:
		return !typed
	case string:
		text := strings.TrimSpace(typed)
		return text == "" || strings.EqualFold(text, "false") || text == "[]"
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}

func splitViewModes(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		mode := strings.TrimSpace(part)
		if mode != "" {
			out = append(out, mode)
		}
	}
	return out
}

func actionViewID(value any) int64 {
	switch typed := value.(type) {
	case []any:
		if len(typed) == 0 {
			return 0
		}
		return int64Value(typed[0])
	case []int64:
		if len(typed) == 0 {
			return 0
		}
		return typed[0]
	default:
		return int64Value(value)
	}
}

func (s Server) contextEmbeddedActionIDs(parentActionID int64) []any {
	if s.Env == nil || parentActionID == 0 {
		return []any{}
	}
	if _, ok := s.Env.ModelMetadata("ir.embedded.actions"); !ok {
		return []any{}
	}
	ctx := s.Env.Context()
	activeID := int64Value(ctx.Values["active_id"])
	if activeID == 0 {
		return []any{}
	}
	found, err := s.Env.Model("ir.embedded.actions").Search(domain.Cond("parent_action_id", domain.Equal, parentActionID))
	if err != nil {
		return []any{}
	}
	rows, err := found.Read("id", "parent_res_id", "parent_res_model", "user_id", "groups_ids", "domain", "is_visible")
	if err != nil {
		return []any{}
	}
	userGroups := menuGroupSet(s.Env)
	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		if row["is_visible"] == false {
			continue
		}
		parentResID := int64Value(row["parent_res_id"])
		if parentResID != 0 && parentResID != activeID {
			continue
		}
		userID := int64Value(row["user_id"])
		if userID != 0 && userID != ctx.UserID {
			continue
		}
		groupIDs := int64Slice(row["groups_ids"])
		if len(groupIDs) > 0 && ctx.UserID != 1 {
			matched := false
			for _, groupID := range groupIDs {
				if userGroups[groupID] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		parentModel := stringValue(row["parent_res_model"])
		if parentModel != "" {
			if ok := s.embeddedActionDomainMatches(parentModel, activeID, row["domain"]); !ok {
				continue
			}
		}
		if id := int64Value(row["id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s Server) embeddedActionDomainMatches(modelName string, activeID int64, rawDomain any) bool {
	if modelName == "" || activeID == 0 {
		return true
	}
	if _, ok := s.Env.ModelMetadata(modelName); !ok {
		return false
	}
	if domainText := strings.TrimSpace(stringValue(rawDomain)); domainText != "" {
		evaluated, err := data.SafeEvalExpression(domainText, data.SafeEvalOptions{
			Env:       s.Env,
			Model:     modelName,
			RecordID:  activeID,
			RecordIDs: []int64{activeID},
			Locals: map[string]any{
				"active_id":    activeID,
				"active_ids":   []int64{activeID},
				"active_model": modelName,
			},
		})
		if err != nil {
			return false
		}
		rawDomain = evaluated
	}
	node, err := parseDomain(rawDomain)
	if err != nil {
		return false
	}
	active, err := s.Env.Model(modelName).Browse(activeID).Read("id")
	if err != nil || len(active) == 0 {
		return false
	}
	found, err := s.Env.Model(modelName).Search(domain.And(domain.Cond("id", domain.Equal, activeID), node))
	return err == nil && len(found.IDs()) > 0
}

func (s Server) actionCommonPayload(modelName string, id int64, fallback map[string]any) map[string]any {
	payload := map[string]any{
		"id":           id,
		"name":         stringValue(fallback["name"]),
		"display_name": firstNonEmptyHTTPString(stringValue(fallback["display_name"]), stringValue(fallback["name"])),
		"type":         firstNonEmptyHTTPString(stringValue(fallback["type"]), modelName),
	}
	baseRows, ok := s.actionBaseRows(id)
	if ok {
		base := baseRows[0]
		payload["name"] = stringValue(firstNonNil(base["name"], payload["name"]))
		payload["display_name"] = firstNonEmptyHTTPString(stringValue(firstNonNil(base["display_name"], base["name"])), stringValue(payload["name"]))
		payload["type"] = firstNonEmptyHTTPString(stringValue(firstNonNil(base["type"], payload["type"])), modelName)
		for _, fieldName := range []string{"binding_model_id", "binding_type", "binding_view_types", "help", "path", "xml_id"} {
			if value, exists := base[fieldName]; exists {
				payload[fieldName] = s.actionReadableFieldValue("ir.actions.actions", fieldName, value)
			}
		}
	} else {
		for _, fieldName := range []string{"binding_model_id", "binding_type", "binding_view_types", "help", "path", "xml_id"} {
			if value, exists := fallback[fieldName]; exists {
				payload[fieldName] = s.actionReadableFieldValue(modelName, fieldName, value)
			}
		}
	}
	return payload
}

func (s Server) actionReadableFieldValue(modelName string, fieldName string, value any) any {
	if s.Env == nil {
		return value
	}
	meta, ok := s.Env.ModelMetadata(modelName)
	if !ok {
		return value
	}
	fieldMeta, ok := meta.Fields[fieldName]
	if !ok {
		return value
	}
	switch fieldMeta.Kind {
	case field.Many2One:
		return s.actionMany2OneValue(fieldMeta, value)
	case field.One2Many, field.Many2Many:
		ids := int64Slice(value)
		out := make([]any, 0, len(ids))
		for _, id := range ids {
			if id > 0 {
				out = append(out, id)
			}
		}
		return out
	default:
		if value == nil {
			return false
		}
		return value
	}
}

func (s Server) actionMany2OneValue(fieldMeta field.Field, value any) any {
	id := int64Value(value)
	if id == 0 {
		return false
	}
	if fieldMeta.Relation == "" || s.Env == nil {
		return []any{id, ""}
	}
	names, err := s.Env.Model(fieldMeta.Relation).Browse(id).NameGet()
	if err != nil || len(names) == 0 {
		return []any{id, ""}
	}
	return []any{id, names[0][1]}
}

func (s Server) actionBaseRows(id int64) ([]map[string]any, bool) {
	if s.Env == nil || id <= 0 {
		return nil, false
	}
	meta, ok := s.Env.ModelMetadata("ir.actions.actions")
	if !ok {
		return nil, false
	}
	fields := actionExistingFields(meta, []string{"id", "name", "type", "display_name", "binding_model_id", "binding_type", "binding_view_types", "help", "path", "xml_id"})
	rows, err := s.Env.Model("ir.actions.actions").Browse(id).Read(fields...)
	if err != nil || len(rows) == 0 || int64Value(rows[0]["id"]) == 0 {
		return nil, false
	}
	return rows, true
}

func concreteActionFields(meta model.Model, modelName string) []string {
	return actionExistingFields(meta, readableActionFieldNames(modelName))
}

func readableActionFieldNames(modelName string) []string {
	wanted := []string{"id", "name", "type", "binding_model_id", "binding_type", "binding_view_types", "display_name", "help", "path", "xml_id"}
	switch modelName {
	case "ir.actions.act_window":
		wanted = append(wanted, "context", "cache", "mobile_view_mode", "domain", "filter", "group_ids", "limit", "res_id", "res_model", "search_view_id", "target", "view_id", "view_mode", "views", "embedded_action_ids", "close_on_report_download")
	case "ir.actions.act_window_close":
		wanted = append(wanted, "effect", "infos")
	case "ir.actions.act_url":
		wanted = append(wanted, "target", "url", "close")
	case "ir.actions.server":
		wanted = append(wanted, "group_ids", "model_name")
	case "ir.actions.report":
		wanted = append(wanted, "report_name", "report_type", "target", "context", "data", "close_on_report_download", "domain")
	case "ir.actions.client":
		wanted = append(wanted, "tag", "res_model", "target", "context", "params")
	}
	return wanted
}

func actionExistingFields(meta model.Model, wanted []string) []string {
	out := make([]string, 0, len(wanted))
	seen := map[string]bool{}
	for _, name := range wanted {
		if seen[name] {
			continue
		}
		seen[name] = true
		if name == "id" || meta.Fields[name].Name != "" {
			out = append(out, name)
		}
	}
	return out
}

func actionCommonReadableField(name string) bool {
	switch name {
	case "name", "type", "binding_model_id", "binding_type", "binding_view_types", "display_name", "help", "path", "xml_id":
		return true
	default:
		return false
	}
}

func actionReadableFieldAllowed(modelName string, name string) bool {
	for _, allowed := range readableActionFieldNames(modelName) {
		if name == allowed {
			return true
		}
	}
	return false
}

func (s Server) viewLoad(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	model := r.URL.Query().Get("model")
	groups := menuGroupSet(env)
	views, err := s.Views.ForModelComposed(model, groups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	for idx := range views {
		mutated, err := internalworkflow.ApplyApprovalViewMutation(env, model, views[idx].Type, views[idx].Arch, groups, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		views[idx].Arch = pruneViewGroupNodes(env, injectWorkflowViewIDField(env, model, views[idx].Type, mutated), groups)
	}
	writeJSON(w, views)
}

func (s Server) getViews(env *record.Env, modelName string, refsValue any, options map[string]any) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("environment is required")
	}
	refs := parseViewRequests(refsValue)
	if len(refs) == 0 {
		refs = []viewRequest{{Type: view.List}, {Type: view.Form}}
	}
	fields, err := env.Model(modelName).FieldsGet(nil, nil)
	if err != nil {
		return nil, err
	}
	views := map[string]any{}
	groups := menuGroupSet(env)
	for _, ref := range refs {
		if ref.Type == "" {
			return nil, fmt.Errorf("view type is required")
		}
		payload, err := s.viewDescription(env, modelName, ref, groups, options)
		if err != nil {
			return nil, err
		}
		views[string(ref.Type)] = payload
	}
	return map[string]any{
		"views": views,
		"models": map[string]any{
			modelName: map[string]any{"fields": fields},
		},
	}, nil
}

type viewRequest struct {
	ID   int64
	Type view.Type
}

func parseViewRequests(value any) []viewRequest {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]viewRequest, 0, len(items))
	for _, item := range items {
		pair, ok := item.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		typ := view.Type(strings.TrimSpace(stringValue(pair[1])))
		if typ == "" {
			continue
		}
		out = append(out, viewRequest{ID: int64Value(pair[0]), Type: typ})
	}
	return out
}

func (s Server) viewDescription(env *record.Env, modelName string, ref viewRequest, groups map[int64]bool, options map[string]any) (map[string]any, error) {
	var found view.View
	var ok bool
	if ref.ID > 0 {
		if s.Views == nil {
			return nil, fmt.Errorf("view registry is required")
		}
		found, ok = s.Views.Get(ref.ID)
		if !ok {
			return nil, fmt.Errorf("view %d not found", ref.ID)
		}
		if found.Model != modelName {
			return nil, fmt.Errorf("view %d model %q does not match %q", ref.ID, found.Model, modelName)
		}
		if found.Type != ref.Type {
			return nil, fmt.Errorf("view %d type %q does not match %q", ref.ID, found.Type, ref.Type)
		}
		if !found.Allowed(groups) {
			return nil, fmt.Errorf("view %d is not available for current user groups", ref.ID)
		}
	} else if s.Views != nil {
		found, ok = s.Views.Default(modelName, ref.Type, groups)
	}
	arch := ""
	id := int64(0)
	name := ""
	if ok {
		if s.Views != nil {
			combined, err := s.Views.CombinedView(found.ID, groups)
			if err != nil {
				return nil, err
			}
			found = combined
		}
		arch = found.Arch
		id = found.ID
		name = found.Name
	} else {
		var defaultOK bool
		arch, defaultOK = defaultViewArch(ref.Type)
		if !defaultOK {
			return nil, fmt.Errorf("no default view of type %q could be found for %s", ref.Type, modelName)
		}
	}
	studio := boolOption(options, "studio")
	mutated, err := internalworkflow.ApplyApprovalViewMutation(env, modelName, ref.Type, arch, groups, studio)
	if err != nil {
		return nil, err
	}
	arch = mutated
	if !studio {
		arch = injectWorkflowViewIDField(env, modelName, ref.Type, arch)
	}
	arch = pruneViewGroupNodes(env, arch, groups)
	payload := map[string]any{
		"arch":  arch,
		"id":    falseIfZero(id),
		"model": modelName,
		"type":  string(ref.Type),
		"name":  name,
	}
	if boolOption(options, "toolbar") {
		payload["toolbar"] = toolbarBindings(env, modelName, ref.Type, groups)
	}
	if boolOption(options, "load_filters") && ref.Type == view.Search {
		payload["filters"] = searchViewFilterPayload(env, modelName, options)
	}
	return payload, nil
}

func searchViewFilterPayload(env *record.Env, modelName string, options map[string]any) []any {
	if env == nil {
		return []any{}
	}
	if _, ok := env.ModelMetadata("ir.filters"); !ok {
		return []any{}
	}
	found, err := env.Model("ir.filters").SearchWithOptions(
		domain.And(
			domain.Cond("model_id", domain.Equal, modelName),
			domain.Cond("active", domain.Equal, true),
		),
		record.SearchOptions{Limit: 200, Order: "name asc, id asc"},
	)
	if err != nil || len(found.IDs()) == 0 {
		return []any{}
	}
	rows, err := env.Model("ir.filters").Browse(found.IDs()...).Read("id", "name", "model_id", "domain", "context", "sort", "user_id", "action_id", "embedded_action_id", "is_default", "active")
	if err != nil {
		return []any{}
	}
	actionID := int64Value(options["action_id"])
	embeddedActionID := int64Value(options["embedded_action_id"])
	userID := env.Context().UserID
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		filterUserID := int64Value(row["user_id"])
		if filterUserID != 0 && filterUserID != userID {
			continue
		}
		filterActionID := int64Value(row["action_id"])
		if filterActionID != 0 && filterActionID != actionID {
			continue
		}
		filterEmbeddedActionID := int64Value(row["embedded_action_id"])
		if filterEmbeddedActionID != 0 && filterEmbeddedActionID != embeddedActionID {
			continue
		}
		out = append(out, map[string]any{
			"id":                 int64Value(row["id"]),
			"name":               stringValue(row["name"]),
			"model_id":           stringValue(row["model_id"]),
			"domain":             firstNonEmptyHTTPString(stringValue(row["domain"]), "[]"),
			"context":            firstNonEmptyHTTPString(stringValue(row["context"]), "{}"),
			"sort":               firstNonEmptyHTTPString(stringValue(row["sort"]), "[]"),
			"user_id":            falseIfZero(filterUserID),
			"action_id":          falseIfZero(filterActionID),
			"embedded_action_id": falseIfZero(filterEmbeddedActionID),
			"is_default":         truthyHTTPValue(row["is_default"]),
		})
	}
	return out
}

func toolbarBindings(env *record.Env, modelName string, viewType view.Type, groups map[int64]bool) map[string]any {
	payload := map[string]any{}
	modelID := toolbarModelID(env, modelName)
	if modelID == 0 {
		return payload
	}
	actionItems := []map[string]any{}
	printItems := []map[string]any{}
	for _, item := range toolbarBindingRows(env, "ir.actions.act_window", modelID, viewType, groups, "action",
		"name", "type", "res_model", "view_mode", "views", "domain", "context", "target", "binding_type", "binding_view_types", "group_ids") {
		actionItems = append(actionItems, item)
	}
	for _, item := range toolbarBindingRows(env, "ir.actions.act_url", modelID, viewType, groups, "action",
		"name", "type", "url", "target", "binding_type", "binding_view_types") {
		actionItems = append(actionItems, item)
	}
	for _, item := range toolbarBindingRows(env, "ir.actions.server", modelID, viewType, groups, "action",
		"name", "type", "state", "model_id", "model_name", "sequence", "binding_type", "binding_view_types", "group_ids") {
		actionItems = append(actionItems, item)
	}
	for _, item := range toolbarBindingRows(env, "ir.actions.report", modelID, viewType, groups, "report",
		"name", "type", "model", "report_name", "report_type", "report_file", "print_report_name", "attachment", "attachment_use", "multi", "binding_type", "groups_id", "domain", "is_invoice_report") {
		printItems = append(printItems, item)
	}
	sortToolbarItems(actionItems)
	if len(actionItems) > 0 {
		payload["action"] = actionItems
	}
	if len(printItems) > 0 {
		payload["print"] = printItems
	}
	return payload
}

func toolbarModelID(env *record.Env, modelName string) int64 {
	if env == nil || strings.TrimSpace(modelName) == "" {
		return 0
	}
	if _, ok := env.ModelMetadata("ir.model"); !ok {
		return 0
	}
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, modelName), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["id"])
}

func toolbarBindingRows(env *record.Env, actionModel string, bindingModelID int64, viewType view.Type, groups map[int64]bool, defaultBindingType string, fields ...string) []map[string]any {
	meta, ok := env.ModelMetadata(actionModel)
	if !ok {
		return nil
	}
	readFields := []string{"binding_model_id", "binding_type", "binding_view_types"}
	if meta.Fields["active"].Name != "" {
		readFields = append(readFields, "active")
	}
	for _, name := range fields {
		if name != "id" && meta.Fields[name].Name != "" {
			readFields = append(readFields, name)
		}
	}
	if meta.Fields["groups_id"].Name != "" && !containsString(readFields, "groups_id") {
		readFields = append(readFields, "groups_id")
	}
	if meta.Fields["group_ids"].Name != "" && !containsString(readFields, "group_ids") {
		readFields = append(readFields, "group_ids")
	}
	found, err := env.Model(actionModel).Search(domain.Cond("binding_model_id", domain.Equal, bindingModelID))
	if err != nil || found.Len() == 0 {
		return nil
	}
	rows, err := found.Read(readFields...)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if active, ok := row["active"].(bool); ok && !active {
			continue
		}
		bindingType := firstNonEmptyHTTPString(stringValue(row["binding_type"]), defaultBindingType)
		if defaultBindingType == "report" {
			if bindingType != "report" {
				continue
			}
		} else if bindingType != "" && bindingType != "action" {
			continue
		}
		if !toolbarViewTypeAllowed(stringValue(row["binding_view_types"]), viewType) {
			continue
		}
		requiredGroups := int64Slice(firstNonNil(row["groups_id"], row["group_ids"]))
		if !toolbarGroupsAllowed(env, requiredGroups, groups) {
			continue
		}
		resModel := strings.TrimSpace(stringValue(row["res_model"]))
		if resModel != "" && env.Policy() != nil && env.Policy().Check(env.Context(), resModel, record.OpRead, nil) != nil {
			continue
		}
		item := map[string]any{
			"id":                 toolbarActionRef(actionModel, int64Value(row["id"])),
			"name":               stringValue(row["name"]),
			"binding_view_types": stringValue(row["binding_view_types"]),
		}
		if sequence, ok := row["sequence"]; ok {
			item["sequence"] = sequence
		}
		if domainValue := stringValue(row["domain"]); strings.TrimSpace(domainValue) != "" {
			item["domain"] = domainValue
		}
		out = append(out, item)
	}
	return out
}

func toolbarViewTypeAllowed(bindingViewTypes string, viewType view.Type) bool {
	typ := string(viewType)
	if typ == "" {
		return false
	}
	if bindingViewTypes == "" {
		return true
	}
	for _, part := range strings.Split(bindingViewTypes, ",") {
		if part == typ {
			return true
		}
	}
	return false
}

func toolbarGroupsAllowed(env *record.Env, required []int64, groups map[int64]bool) bool {
	if len(required) == 0 || (env != nil && env.Context().UserID == 1) {
		return true
	}
	for _, groupID := range required {
		if groups[groupID] {
			return true
		}
	}
	return false
}

func sortToolbarItems(items []map[string]any) {
	sort.SliceStable(items, func(i, j int) bool {
		leftSeq := int64Value(items[i]["sequence"])
		rightSeq := int64Value(items[j]["sequence"])
		if leftSeq != rightSeq {
			return leftSeq < rightSeq
		}
		return toolbarActionIDValue(items[i]["id"]) < toolbarActionIDValue(items[j]["id"])
	})
}

func toolbarActionRef(_ string, id int64) any {
	if id == 0 {
		return false
	}
	return id
}

func toolbarActionIDValue(value any) int64 {
	if text, ok := value.(string); ok {
		if _, raw, found := strings.Cut(text, ","); found {
			return int64Value(raw)
		}
	}
	return int64Value(value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func pruneViewGroupNodes(env *record.Env, arch string, groups map[int64]bool) string {
	if !strings.Contains(arch, "groups") {
		return arch
	}
	pruned, changed, err := pruneXMLGroupNodes(env, arch, groups)
	if err != nil || !changed {
		return arch
	}
	return pruned
}

func pruneXMLGroupNodes(env *record.Env, arch string, groups map[int64]bool) (string, bool, error) {
	decoder := xml.NewDecoder(strings.NewReader(arch))
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	resolver := viewGroupXMLIDResolver{env: env, cache: map[string][]int64{}}
	liftStack := []bool{}
	skipDepth := 0
	changed := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", false, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			groupExpr, attrs, hadGroups := splitGroupsAttribute(typed.Attr)
			if hadGroups {
				changed = true
				if !viewGroupExpressionAllowed(groupExpr, groups, resolver.Resolve) {
					skipDepth = 1
					continue
				}
			}
			lift := hadGroups && typed.Name.Local == "t" && len(attrs) == 0
			liftStack = append(liftStack, lift)
			if lift {
				continue
			}
			typed.Attr = attrs
			if err := encoder.EncodeToken(typed); err != nil {
				return "", false, err
			}
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			lift := false
			if len(liftStack) > 0 {
				lift = liftStack[len(liftStack)-1]
				liftStack = liftStack[:len(liftStack)-1]
			}
			if lift {
				continue
			}
			if err := encoder.EncodeToken(typed); err != nil {
				return "", false, err
			}
		default:
			if skipDepth > 0 {
				continue
			}
			if err := encoder.EncodeToken(token); err != nil {
				return "", false, err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", false, err
	}
	return buffer.String(), changed, nil
}

func splitGroupsAttribute(attrs []xml.Attr) (string, []xml.Attr, bool) {
	out := make([]xml.Attr, 0, len(attrs))
	value := ""
	found := false
	for _, attr := range attrs {
		if attr.Name.Local == "groups" {
			value = attr.Value
			found = true
			continue
		}
		out = append(out, attr)
	}
	return value, out, found
}

func viewGroupExpressionAllowed(expr string, groups map[int64]bool, resolve func(string) []int64) bool {
	hasPositive := false
	matchedPositive := false
	for _, raw := range strings.Split(expr, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		negated := strings.HasPrefix(token, "!")
		if negated {
			token = strings.TrimSpace(strings.TrimPrefix(token, "!"))
		}
		ids := resolve(token)
		if negated {
			for _, id := range ids {
				if groups[id] {
					return false
				}
			}
			continue
		}
		hasPositive = true
		for _, id := range ids {
			if groups[id] {
				matchedPositive = true
			}
		}
	}
	return !hasPositive || matchedPositive
}

type viewGroupXMLIDResolver struct {
	env   *record.Env
	cache map[string][]int64
}

func (r viewGroupXMLIDResolver) Resolve(token string) []int64 {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if id, err := strconv.ParseInt(token, 10, 64); err == nil && id > 0 {
		return []int64{id}
	}
	if ids, ok := r.cache[token]; ok {
		return ids
	}
	ids := modelDataResIDs(r.env, token, "res.groups")
	r.cache[token] = ids
	return ids
}

func injectWorkflowViewIDField(env *record.Env, modelName string, typ view.Type, arch string) string {
	if env == nil || modelName == "" || (typ != view.List && typ != view.Kanban) {
		return arch
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return arch
	}
	if _, ok := meta.Fields["workflow_view_id"]; !ok {
		return arch
	}
	if strings.Contains(arch, `name="workflow_view_id"`) || strings.Contains(arch, `name='workflow_view_id'`) {
		return arch
	}
	root := string(typ)
	return injectFieldIntoRootArch(arch, root, `<field name="workflow_view_id" invisible="1" column_invisible="1" readonly="1"/>`)
}

func injectFieldIntoRootArch(arch string, root string, fieldNode string) string {
	trimmed := strings.TrimSpace(arch)
	if trimmed == "" || root == "" || fieldNode == "" {
		return arch
	}
	closing := "</" + root + ">"
	if index := strings.LastIndex(arch, closing); index >= 0 {
		return arch[:index] + fieldNode + arch[index:]
	}
	openPrefix := "<" + root
	if !strings.HasPrefix(trimmed, openPrefix) {
		return arch
	}
	if strings.HasSuffix(trimmed, "/>") {
		selfCloseIndex := strings.LastIndex(arch, "/>")
		if selfCloseIndex >= 0 {
			return arch[:selfCloseIndex] + ">" + fieldNode + closing + arch[selfCloseIndex+2:]
		}
	}
	return arch
}

func defaultViewArch(typ view.Type) (string, bool) {
	switch typ {
	case view.Form:
		return "<form/>", true
	case view.List:
		return "<list/>", true
	case view.Search:
		return "<search/>", true
	case view.Kanban:
		return "<kanban/>", true
	default:
		return "", false
	}
}

func boolOption(options map[string]any, key string) bool {
	value, _ := options[key].(bool)
	return value
}

func (s Server) menuLoad(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireWebSession(w, r, nil)
	if !ok {
		return
	}
	writeJSON(w, s.menuPayload(env))
}

func (s Server) bundle(w http.ResponseWriter, r *http.Request) {
	bundle := strings.TrimPrefix(r.URL.Path, "/web/bundle/")
	if bundle == "" {
		bundle = assets.Backend
	}
	s.writeAssetManifest(w, bundle, assetDebug(r))
}

func (s Server) assetManifest(w http.ResponseWriter, r *http.Request) {
	bundle := r.URL.Query().Get("bundle")
	if bundle == "" {
		bundle = assets.Backend
	}
	s.writeAssetManifest(w, bundle, assetDebug(r))
}

func (s Server) assetDebugFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Assets == nil {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/web/assets/debug/")
	bundle, assetPath, ok := strings.Cut(rest, "/")
	if !ok || bundle == "" || assetPath == "" {
		http.NotFound(w, r)
		return
	}
	filename, found, err := s.Assets.DebugFile(bundle, assetPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filename)
}

func (s Server) writeAssetManifest(w http.ResponseWriter, bundle string, debug bool) {
	data, err := s.Assets.ManifestWithOptions(bundle, assets.ManifestOptions{Debug: debug})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func assetDebug(r *http.Request) bool {
	debug := r.URL.Query().Get("debug")
	return debug == "1" || debug == "true" || debug == "assets"
}

func (s Server) binary(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "binary attachment not found", http.StatusNotFound)
}

func (s Server) downloadInvoiceDocuments(w http.ResponseWriter, r *http.Request) {
	if s.Env == nil {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/account/download_invoice_documents/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	moveIDs := parseCommaIDs(parts[0])
	filetype := parts[1]
	if len(moveIDs) == 0 || (filetype != "pdf" && filetype != "all") {
		http.NotFound(w, r)
		return
	}
	docs, err := s.invoiceDocumentData(moveIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if len(docs) == 0 {
		http.NotFound(w, r)
		return
	}
	if len(docs) == 1 {
		doc := docs[0]
		w.Header().Set("Content-Type", doc.Filetype)
		w.Header().Set("Content-Disposition", contentDisposition(doc.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(doc.Content)
		return
	}
	content, err := zipDocuments(docs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition("invoices.zip"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(content)
}

func (s Server) downloadInvoiceAttachments(w http.ResponseWriter, r *http.Request) {
	if s.Env == nil {
		http.NotFound(w, r)
		return
	}
	attachmentIDs := parseCommaIDs(strings.TrimPrefix(r.URL.Path, "/account/download_invoice_attachments/"))
	if len(attachmentIDs) == 0 {
		http.NotFound(w, r)
		return
	}
	docs, err := s.attachmentDocumentData(attachmentIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeDocumentsResponse(w, docs, "invoices.zip")
}

func (s Server) downloadMoveAttachments(w http.ResponseWriter, r *http.Request) {
	if s.Env == nil {
		http.NotFound(w, r)
		return
	}
	moveIDs := parseCommaIDs(strings.TrimPrefix(r.URL.Path, "/account/download_move_attachments/"))
	if len(moveIDs) == 0 {
		http.NotFound(w, r)
		return
	}
	docs, err := s.invoiceDocumentData(moveIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeDocumentsResponse(w, docs, "Invoices.zip")
}

type invoiceDocument struct {
	Filename string
	Filetype string
	Content  []byte
}

func (s Server) invoiceDocumentData(moveIDs []int64) ([]invoiceDocument, error) {
	moves, err := readAccountingMoves(s.Env, moveIDs)
	if err != nil {
		return nil, err
	}
	out := make([]invoiceDocument, 0, len(moves))
	for _, move := range moves {
		attachmentID, err := ensureInvoicePDFPlaceholder(s.Env, move)
		if err != nil {
			return nil, err
		}
		rows, err := s.Env.Model("ir.attachment").Browse(attachmentID).Read("name", "mimetype", "datas")
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, fmt.Errorf("invoice attachment %d not found", attachmentID)
		}
		content := byteValue(rows[0]["datas"])
		filetype := stringValue(rows[0]["mimetype"])
		if !bytes.HasPrefix(content, []byte("%PDF-")) {
			content = validInvoicePDFBytes(move)
			filetype = "application/pdf"
			if err := s.Env.Model("ir.attachment").Browse(attachmentID).Write(map[string]any{
				"datas":     content,
				"file_size": len(content),
				"mimetype":  "application/pdf",
			}); err != nil {
				return nil, err
			}
		}
		if filetype == "" {
			filetype = "application/pdf"
		}
		filename := stringValue(rows[0]["name"])
		if filename == "" {
			filename = invoicePDFName(move)
		}
		out = append(out, invoiceDocument{Filename: filename, Filetype: filetype, Content: content})
	}
	return out, nil
}

func (s Server) attachmentDocumentData(attachmentIDs []int64) ([]invoiceDocument, error) {
	rows, err := s.Env.Model("ir.attachment").Browse(attachmentIDs...).Read("id", "name", "res_model", "res_field", "res_id", "mimetype", "datas")
	if err != nil {
		return nil, err
	}
	out := make([]invoiceDocument, 0, len(rows))
	for _, row := range rows {
		if stringValue(row["res_model"]) != "account.move" || int64Value(row["res_id"]) == 0 {
			return nil, fmt.Errorf("attachment is not linked to an invoice")
		}
		if stringValue(row["res_field"]) != "invoice_pdf_report_file" || stringValue(row["mimetype"]) != "application/pdf" {
			return nil, fmt.Errorf("attachment is not an invoice PDF")
		}
		moveRows, err := s.Env.Model("account.move").Browse(int64Value(row["res_id"])).Read("id")
		if err != nil {
			return nil, err
		}
		if len(moveRows) != 1 {
			return nil, fmt.Errorf("invoice for attachment was not found")
		}
		filename := stringValue(row["name"])
		if filename == "" {
			filename = fmt.Sprintf("attachment-%d", int64Value(row["id"]))
		}
		filetype := stringValue(row["mimetype"])
		if filetype == "" {
			filetype = "application/octet-stream"
		}
		out = append(out, invoiceDocument{Filename: filename, Filetype: filetype, Content: byteValue(row["datas"])})
	}
	return out, nil
}

func writeDocumentsResponse(w http.ResponseWriter, docs []invoiceDocument, zipName string) {
	if len(docs) == 0 {
		http.NotFound(w, nil)
		return
	}
	if len(docs) == 1 {
		doc := docs[0]
		w.Header().Set("Content-Type", doc.Filetype)
		w.Header().Set("Content-Disposition", contentDisposition(doc.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(doc.Content)
		return
	}
	content, err := zipDocuments(docs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(zipName))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(content)
}

func zipDocuments(docs []invoiceDocument) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	seen := map[string]int{}
	for _, doc := range docs {
		filename := uniqueDocumentName(doc.Filename, seen)
		file, err := writer.Create(filename)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := file.Write(doc.Content); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func uniqueDocumentName(name string, seen map[string]int) string {
	if name == "" {
		name = "document"
	}
	count := seen[name]
	seen[name] = count + 1
	if count == 0 {
		return name
	}
	base := name
	ext := ""
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		base = name[:dot]
		ext = name[dot:]
	}
	candidate := fmt.Sprintf("%s (%d)%s", base, count, ext)
	for seen[candidate] > 0 {
		count++
		candidate = fmt.Sprintf("%s (%d)%s", base, count, ext)
	}
	seen[candidate] = 1
	return candidate
}

func contentDisposition(filename string) string {
	filename = strings.ReplaceAll(filename, `"`, "")
	if filename == "" {
		filename = "download"
	}
	return fmt.Sprintf(`attachment; filename="%s"`, filename)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (s Server) writeMaybeRPC(w http.ResponseWriter, r *http.Request, value any) {
	envelope, ok := readEnvelope(r)
	if !ok {
		writeJSON(w, value)
		return
	}
	writeRPCOrJSON(w, envelope, value)
}

func writeRPCOrJSON(w http.ResponseWriter, envelope *rpcEnvelope, value any) {
	if envelope == nil {
		writeJSON(w, value)
		return
	}
	writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": envelope.ID, "result": value})
}

func writeRPCError(w http.ResponseWriter, envelope *rpcEnvelope, status int, err error) {
	if status == 0 {
		status = http.StatusForbidden
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	message := ""
	if err != nil {
		message = err.Error()
	}
	if envelope != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": envelope.ID, "error": message})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}

func writeFileExportException(w http.ResponseWriter, status int, err error) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    0,
		"message": "Odoo Server Error",
		"data": map[string]any{
			"name":      "odoo.exceptions.UserError",
			"message":   message,
			"arguments": []any{message},
			"context":   map[string]any{},
			"debug":     "odoo.exceptions.UserError: " + message,
		},
	})
}

const serverActionWarningExceptionName = "odoo.addons.base.models.ir_actions.ServerActionWithWarningsError"

func serverActionWarningMessage(action serveractions.ServerAction) string {
	name := strings.TrimSpace(action.Name)
	if name == "" {
		name = strconv.FormatInt(action.ID, 10)
	}
	return fmt.Sprintf("Server action %s has one or more warnings, address them first.", name)
}

func writeRPCException(w http.ResponseWriter, envelope *rpcEnvelope, status int, name string, message string, arguments []any) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data := map[string]any{
		"name":      name,
		"message":   message,
		"arguments": arguments,
		"context":   map[string]any{},
		"debug":     fmt.Sprintf("%s: %s", name, message),
	}
	errorPayload := map[string]any{
		"code":    0,
		"message": "Odoo Server Error",
		"data":    data,
	}
	if envelope != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": envelope.ID, "error": errorPayload})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": errorPayload})
}

func readEnvelope(r *http.Request) (*rpcEnvelope, bool) {
	if r.Body == nil {
		return nil, false
	}
	body, _ := readBody(r)
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, false
	}
	var envelope rpcEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.JSONRPC == "" {
		return nil, false
	}
	return &envelope, true
}

func decodeCallKW(r *http.Request) (callKWRequest, *rpcEnvelope, error) {
	body, err := readBody(r)
	if err != nil {
		return callKWRequest{}, nil, err
	}
	var envelope rpcEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.JSONRPC != "" {
		var req callKWRequest
		if len(envelope.Params) > 0 {
			if err := json.Unmarshal(envelope.Params, &req); err != nil {
				return callKWRequest{}, nil, err
			}
		}
		return req, &envelope, nil
	}
	var req callKWRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return callKWRequest{}, nil, err
	}
	return req, nil, nil
}

func decodeRPCParams(r *http.Request, out any) (*rpcEnvelope, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	var envelope rpcEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.JSONRPC != "" {
		if len(envelope.Params) > 0 {
			if err := json.Unmarshal(envelope.Params, out); err != nil {
				return nil, err
			}
		}
		return &envelope, nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, err
	}
	return nil, nil
}

func aiHTTPStatus(err error) int {
	switch {
	case errors.Is(err, aicontrollers.ErrChannelNotFound), errors.Is(err, aicontrollers.ErrMessageNotFound), errors.Is(err, aicontrollers.ErrNotAIChannel):
		return http.StatusNotFound
	case errors.Is(err, aicontrollers.ErrProviderMissing), errors.Is(err, aicontrollers.ErrResponderMissing):
		return http.StatusServiceUnavailable
	case errors.Is(err, aiproviders.ErrProviderRequest):
		return http.StatusBadGateway
	default:
		return http.StatusForbidden
	}
}

func readBody(r *http.Request) ([]byte, error) {
	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	r.Body = http.NoBody
	return body.Bytes(), nil
}

func applyPathModelMethod(path string, req *callKWRequest) {
	const prefix = "/web/dataset/call_kw/"
	if !strings.HasPrefix(path, prefix) {
		const buttonPrefix = "/web/dataset/call_button/"
		if !strings.HasPrefix(path, buttonPrefix) {
			return
		}
		path = strings.TrimPrefix(path, buttonPrefix)
	} else {
		path = strings.TrimPrefix(path, prefix)
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 1 && parts[0] != "" {
		req.Model = parts[0]
	}
	if len(parts) >= 2 && parts[1] != "" {
		req.Method = parts[1]
	}
}

func (s Server) sessionInfoPayload(r *http.Request) map[string]any {
	env := s.publicRequestEnv(r)
	payload := s.sessionPayloadForEnv(env)
	if s.Impersonation == nil {
		return payload
	}
	info, err := s.Impersonation.SessionInfo(s.impersonationSessionID(r))
	if err != nil {
		return payload
	}
	payload["uid"] = info.UserID
	if info.Impersonating {
		payload["impersonate"] = true
		payload["login_as"] = map[string]any{
			"active":        true,
			"original_uid":  info.OriginalUserID,
			"effective_uid": info.UserID,
			"banner":        info.Banner,
			"return_to":     info.ReturnTo,
			"back_route":    info.LoginBackRoute,
		}
	}
	if userContext, ok := payload["user_context"].(map[string]any); ok {
		for key, value := range info.Context {
			userContext[key] = value
		}
		userContext["uid"] = info.UserID
	}
	return payload
}

func (s Server) sessionPayloadForEnv(env *record.Env) map[string]any {
	payload := sessionInfoPayload(env)
	s.addMailStorePayload(payload, env)
	return payload
}

func (s Server) addMailStorePayload(payload map[string]any, env *record.Env) {
	if payload == nil || env == nil || env.Context().UserID == 0 {
		return
	}
	counterBusID := int64(0)
	if s.Bus != nil {
		counterBusID = s.Bus.LastID()
	}
	store, _ := payload["Store"].(map[string]any)
	if store == nil {
		store = map[string]any{}
		payload["Store"] = store
	}
	store["starred"] = internalmail.StarredMailboxPayload(env, counterBusID)
}

func (s Server) loginAsAuditLen() int {
	if s.Impersonation == nil {
		return 0
	}
	return len(s.Impersonation.AuditLog())
}

func (s Server) persistLoginAsAuditSince(start int, r *http.Request) error {
	if s.Env == nil || s.Impersonation == nil {
		return nil
	}
	events := s.Impersonation.AuditLog()
	if start < 0 || start > len(events) {
		start = len(events)
	}
	for _, event := range events[start:] {
		if err := s.persistLoginAsAudit(event, r); err != nil {
			return err
		}
	}
	return nil
}

func (s Server) persistLoginAsAudit(event impersonation.AuditEvent, r *http.Request) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return err
	}
	ipAddress := event.IPAddress
	userAgent := event.UserAgent
	if r != nil {
		if ipAddress == "" {
			ipAddress = requestIP(r)
		}
		if userAgent == "" {
			userAgent = r.UserAgent()
		}
	}
	_, err = s.Env.Model("login.as.audit").Create(map[string]any{
		"action":            event.Action,
		"actor_id":          event.ActorID,
		"effective_user_id": event.EffectiveUserID,
		"target_user_id":    event.TargetUserID,
		"session_id":        event.SessionID,
		"model":             event.Model,
		"record_id":         event.RecordID,
		"ip_address":        ipAddress,
		"user_agent":        userAgent,
		"details":           string(details),
		"created_at":        event.At.UTC().Format(time.RFC3339Nano),
	})
	if err != nil && !isUnknownLoginAsAuditModel(err) {
		return err
	}
	return nil
}

func requestIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		if index := strings.Index(forwardedFor, ","); index >= 0 {
			forwardedFor = forwardedFor[:index]
		}
		return strings.TrimSpace(forwardedFor)
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func requestRemoteAddr(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func isUnknownLoginAsAuditModel(err error) bool {
	return strings.HasPrefix(err.Error(), "unknown model login.as.audit")
}

func sessionInfoPayload(env *record.Env) map[string]any {
	ctx := env.Context()
	lang := stringContextValue(ctx, "lang", "en_US")
	tz := stringContextValue(ctx, "tz", "UTC")
	companyIDs := companyIDs(ctx)
	allowedCompanyIDs := uniqueInt64Slice(int64Slice(ctx.Values["all_allowed_company_ids"]))
	if len(allowedCompanyIDs) == 0 {
		allowedCompanyIDs = append([]int64(nil), companyIDs...)
	}
	companyID := ctx.CompanyID
	if companyID == 0 && len(companyIDs) > 0 {
		companyID = companyIDs[0]
	}
	dbName := stringContextValue(ctx, "db", "gorp")
	edition := stringContextValue(ctx, "edition", "e")
	isInternal := sessionInternalUser(env)
	userInfo := sessionUserInfoPayload(env)
	groups := groupPayloadForEnv(env)
	bundleParams := map[string]any{"lang": lang}
	if debug := strings.TrimSpace(fmt.Sprint(ctx.Values["debug"])); debug != "" && debug != "<nil>" {
		bundleParams["debug"] = debug
	}
	payload := map[string]any{
		"uid":              ctx.UserID,
		"is_system":        sessionHasXMLIDGroup(env, "base.group_system") || ctx.UserID == 1,
		"is_admin":         sessionHasXMLIDGroup(env, "base.group_system") || ctx.UserID == 1,
		"is_public":        ctx.UserID == 0,
		"is_internal_user": isInternal,
		"db":               dbName,
		"db_name":          dbName,
		"dbname":           dbName,
		"database_name":    dbName,
		"server_version":   "19.0",
		"server_version_info": []any{
			19,
			0,
			0,
			"final",
			0,
			edition,
		},
		"registry_hash": registryHash(ctx),
		"company_id":    companyID,
		"user_context": map[string]any{
			"uid":                 ctx.UserID,
			"lang":                lang,
			"tz":                  tz,
			"allowed_company_ids": companyIDs,
		},
		"bundle_params":        bundleParams,
		"user_settings":        sessionUserSettingsPayload(env),
		"currencies":           sessionCurrencyPayload(env),
		"groups":               groups,
		"view_info":            defaultViewInfoPayload(),
		"support_url":          "https://www.odoo.com/help",
		"name":                 firstTextHTTP(userInfo["name"], fmt.Sprintf("User %d", ctx.UserID)),
		"username":             firstTextHTTP(userInfo["login"], fmt.Sprintf("user%d", ctx.UserID)),
		"partner_write_date":   userInfo["partner_write_date"],
		"partner_display_name": firstTextHTTP(userInfo["partner_display_name"], userInfo["name"]),
		"partner_id":           falseIfZero(int64Value(userInfo["partner_id"])),
		"web.base.url":         firstTextHTTP(configParameterValue(env, "web.base.url"), "http://localhost"),
		"active_ids_limit":     20000,
		"profile_session":      ctx.Values["profile_session"],
		"profile_collectors":   ctx.Values["profile_collectors"],
		"profile_params":       ctx.Values["profile_params"],
		"max_file_upload_size": 134217728,
		"home_action_id":       false,
		"quick_login":          sessionConfigBoolDefault(env, "web.quick_login", true),
		"web_tours":            []any{},
		"test_mode":            false,
		"enterprise": map[string]any{
			"edition":              edition,
			"subscription_warning": false,
		},
	}
	if isInternal {
		payload["user_companies"] = map[string]any{
			"current_company":               companyID,
			"allowed_companies":             sessionCompanyPayload(env, allowedCompanyIDs),
			"disallowed_ancestor_companies": sessionDisallowedAncestorCompanyPayload(env, allowedCompanyIDs),
		}
		payload["show_effect"] = true
		payload["display_switch_company_menu"] = len(allowedCompanyIDs) > 1
	}
	if warning := sessionEnterpriseWarning(env, isInternal, payload["is_system"] == true); warning != false {
		payload["warning"] = warning
		payload["expiration_date"] = falseIfEmpty(configParameterValue(env, "database.expiration_date"))
		payload["expiration_reason"] = falseIfEmpty(configParameterValue(env, "database.expiration_reason"))
		if message := sessionSysadminMessage(env); message != nil {
			payload["sysadmin_message"] = message
		}
	}
	if sessionInternalUser(env) {
		payload["disable_edit_on_non_approval"] = configParameterBool(env, "disable_edit_on_non_approval")
	}
	return payload
}

func sessionUserInfoPayload(env *record.Env) map[string]any {
	out := map[string]any{
		"name":                 "",
		"login":                "",
		"partner_id":           int64(0),
		"partner_display_name": "",
		"partner_write_date":   false,
	}
	if env == nil || env.Context().UserID == 0 || !modelHasField(env, "res.users", "name") {
		return out
	}
	userFields := availableModelFields(env, "res.users", "id", "name", "login", "partner_id")
	rows, err := env.Model("res.users").Browse(env.Context().UserID).Read(userFields...)
	if err != nil || len(rows) == 0 {
		return out
	}
	row := rows[0]
	out["name"] = stringValue(row["name"])
	out["login"] = stringValue(row["login"])
	out["partner_id"] = int64Value(row["partner_id"])
	partnerID := int64Value(out["partner_id"])
	if partnerID == 0 || !modelHasField(env, "res.partner", "name") {
		return out
	}
	partnerFields := availableModelFields(env, "res.partner", "name", "display_name", "write_date")
	partnerRows, err := env.Model("res.partner").Browse(partnerID).Read(partnerFields...)
	if err != nil || len(partnerRows) == 0 {
		return out
	}
	partner := partnerRows[0]
	out["partner_display_name"] = firstTextHTTP(partner["display_name"], partner["name"])
	if writeDate, ok := partner["write_date"].(time.Time); ok && !writeDate.IsZero() {
		out["partner_write_date"] = writeDate.UTC().Format("2006-01-02 15:04:05")
	}
	return out
}

func sessionUserSettingsPayload(env *record.Env) map[string]any {
	payload := map[string]any{
		"id":      false,
		"user_id": false,
		"is_discuss_sidebar_category_channel_open": true,
		"is_discuss_sidebar_category_chat_open":    true,
		"embedded_actions_config_ids":              map[string]any{},
		"homemenu_config":                          nil,
		"color_scheme":                             "system",
		"home_action_id":                           false,
		"show_effect":                              true,
	}
	if userID := contextUserID(env); userID != 0 {
		payload["user_id"] = map[string]any{"id": userID}
	}
	if env == nil || contextUserID(env) == 0 || !modelHasField(env, "res.users.settings", "user_id") {
		return payload
	}
	found, err := env.Model("res.users.settings").SearchWithOptions(domain.Cond("user_id", domain.Equal, contextUserID(env)), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return payload
	}
	fields := availableModelFields(env, "res.users.settings", "id", "user_id", "is_discuss_sidebar_category_channel_open", "is_discuss_sidebar_category_chat_open", "color_scheme")
	rows, err := found.Read(fields...)
	if err != nil || len(rows) == 0 {
		return payload
	}
	for key, value := range rows[0] {
		payload[key] = value
	}
	return payload
}

func sessionCompanyPayload(env *record.Env, ids []int64) map[string]any {
	companies := companyPayload(ids)
	if env == nil || len(ids) == 0 || !modelHasField(env, "res.company", "name") {
		return companies
	}
	fields := availableModelFields(env, "res.company", "id", "name", "parent_id", "currency_id")
	rows, err := env.Model("res.company").Browse(ids...).Read(fields...)
	if err != nil {
		return companies
	}
	children := sessionCompanyChildren(env, ids)
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		companies[strconv.FormatInt(id, 10)] = map[string]any{
			"id":          id,
			"name":        firstTextHTTP(row["name"], fmt.Sprintf("Company %d", id)),
			"sequence":    int(id),
			"child_ids":   children[id],
			"parent_id":   falseIfZero(int64Value(row["parent_id"])),
			"currency_id": falseIfZero(int64Value(row["currency_id"])),
		}
	}
	return companies
}

func sessionDisallowedAncestorCompanyPayload(env *record.Env, allowedIDs []int64) map[string]any {
	out := map[string]any{}
	if env == nil || len(allowedIDs) == 0 || !modelHasField(env, "res.company", "parent_id") {
		return out
	}
	found, err := env.Model("res.company").Search(domain.And())
	if err != nil {
		return out
	}
	rows, err := found.Read("id", "name", "parent_id")
	if err != nil {
		return out
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		if id := int64Value(row["id"]); id != 0 {
			byID[id] = row
		}
	}
	allowed := map[int64]bool{}
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	ancestorIDs := map[int64]bool{}
	for _, id := range allowedIDs {
		for parentID := int64Value(byID[id]["parent_id"]); parentID != 0 && !allowed[parentID]; parentID = int64Value(byID[parentID]["parent_id"]) {
			if ancestorIDs[parentID] {
				break
			}
			ancestorIDs[parentID] = true
		}
	}
	if len(ancestorIDs) == 0 {
		return out
	}
	hierarchyIDs := map[int64]bool{}
	for _, id := range allowedIDs {
		hierarchyIDs[id] = true
	}
	for id := range ancestorIDs {
		hierarchyIDs[id] = true
	}
	children := map[int64][]int64{}
	for _, row := range byID {
		id := int64Value(row["id"])
		parentID := int64Value(row["parent_id"])
		if id != 0 && parentID != 0 && hierarchyIDs[id] && hierarchyIDs[parentID] {
			children[parentID] = append(children[parentID], id)
		}
	}
	for id := range children {
		sort.Slice(children[id], func(i, j int) bool { return children[id][i] < children[id][j] })
	}
	for id := range ancestorIDs {
		row := byID[id]
		out[strconv.FormatInt(id, 10)] = map[string]any{
			"id":        id,
			"name":      firstTextHTTP(row["name"], fmt.Sprintf("Company %d", id)),
			"sequence":  int(id),
			"child_ids": children[id],
			"parent_id": falseIfZero(int64Value(row["parent_id"])),
		}
	}
	return out
}

func sessionCompanyChildren(env *record.Env, allowedIDs []int64) map[int64][]int64 {
	out := map[int64][]int64{}
	for _, id := range allowedIDs {
		out[id] = []int64{}
	}
	if env == nil || !modelHasField(env, "res.company", "parent_id") {
		return out
	}
	found, err := env.Model("res.company").Search(domain.And())
	if err != nil {
		return out
	}
	rows, err := found.Read("id", "parent_id")
	if err != nil {
		return out
	}
	allowed := map[int64]bool{}
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	for _, row := range rows {
		id := int64Value(row["id"])
		parentID := int64Value(row["parent_id"])
		if id != 0 && parentID != 0 && allowed[id] && allowed[parentID] {
			out[parentID] = append(out[parentID], id)
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i] < out[id][j] })
	}
	return out
}

func sessionCurrencyPayload(env *record.Env) map[string]any {
	if env == nil || !modelHasField(env, "res.currency", "name") {
		return defaultCurrencies()
	}
	found, err := env.Model("res.currency").Search(domain.And())
	if err != nil || found.Len() == 0 {
		return defaultCurrencies()
	}
	fields := availableModelFields(env, "res.currency", "id", "name", "symbol", "position", "rounding", "decimal_places", "iso_numeric", "active")
	rows, err := found.Read(fields...)
	if err != nil || len(rows) == 0 {
		return defaultCurrencies()
	}
	out := map[string]any{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		decimalPlaces := int64Value(row["decimal_places"])
		if decimalPlaces == 0 {
			decimalPlaces = 2
		}
		item := map[string]any{
			"id":                  id,
			"name":                firstTextHTTP(row["name"], fmt.Sprintf("Currency %d", id)),
			"symbol":              firstTextHTTP(row["symbol"], row["name"]),
			"position":            firstTextHTTP(row["position"], "after"),
			"digits":              []int{69, int(decimalPlaces)},
			"rounding":            firstNonZeroFloat(floatValue(row["rounding"]), 0.01),
			"decimal_places":      decimalPlaces,
			"iso_numeric":         row["iso_numeric"],
			"active":              row["active"] != false,
			"decimal_separator":   ".",
			"thousands_separator": ",",
		}
		out[strconv.FormatInt(id, 10)] = item
	}
	if len(out) == 0 {
		return defaultCurrencies()
	}
	return out
}

func groupPayloadForEnv(env *record.Env) map[string]any {
	if env == nil {
		return groupPayload(nil)
	}
	groups := menuGroupSet(env)
	ids := make([]int64, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	payload := groupPayload(ids)
	xmlIDs := sessionGroupXMLIDs(env)
	for xmlID, id := range xmlIDs {
		payload[xmlID] = containsHTTPInt64(ids, id)
	}
	if _, ok := payload["base.group_allow_export"]; !ok {
		payload["base.group_allow_export"] = false
	}
	return payload
}

func sessionGroupXMLIDs(env *record.Env) map[string]int64 {
	out := map[string]int64{}
	if env == nil || !modelHasField(env, "ir.model.data", "model") {
		return out
	}
	found, err := env.Model("ir.model.data").Search(domain.Cond("model", domain.Equal, "res.groups"))
	if err != nil || found.Len() == 0 {
		return out
	}
	rows, err := found.Read("module", "name", "res_id")
	if err != nil {
		return out
	}
	for _, row := range rows {
		module := strings.TrimSpace(stringValue(row["module"]))
		name := strings.TrimSpace(stringValue(row["name"]))
		id := int64Value(row["res_id"])
		if module != "" && name != "" && id != 0 {
			out[module+"."+name] = id
		}
	}
	return out
}

func sessionHasXMLIDGroup(env *record.Env, xmlID string) bool {
	module, name, ok := strings.Cut(strings.TrimSpace(xmlID), ".")
	if !ok || env == nil {
		return false
	}
	groupID := httpXMLIDResID(env, module, name, "res.groups")
	return groupID != 0 && menuGroupSet(env)[groupID]
}

func sessionConfigBoolDefault(env *record.Env, key string, fallback bool) bool {
	value := strings.TrimSpace(configParameterValue(env, key))
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true") || value == "1"
}

func sessionEnterpriseWarning(env *record.Env, isInternal bool, isSystem bool) any {
	if env == nil || contextUserID(env) == 0 || !isInternal {
		return false
	}
	if isSystem {
		return "admin"
	}
	return "user"
}

func sessionSysadminMessage(env *record.Env) map[string]any {
	value := strings.TrimSpace(configParameterValue(env, "sysadmin.message"))
	if value == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func defaultViewInfoPayload() map[string]any {
	return map[string]any{
		"list":     map[string]any{"display_name": "List", "icon": "oi oi-view-list", "multi_record": true},
		"form":     map[string]any{"display_name": "Form", "icon": "fa fa-address-card", "multi_record": false},
		"graph":    map[string]any{"display_name": "Graph", "icon": "fa fa-area-chart", "multi_record": true},
		"pivot":    map[string]any{"display_name": "Pivot", "icon": "oi oi-view-pivot", "multi_record": true},
		"kanban":   map[string]any{"display_name": "Kanban", "icon": "oi oi-view-kanban", "multi_record": true},
		"calendar": map[string]any{"display_name": "Calendar", "icon": "fa fa-calendar", "multi_record": true},
		"search":   map[string]any{"display_name": "Search", "icon": "oi oi-search", "multi_record": true},
	}
}

func availableModelFields(env *record.Env, modelName string, names ...string) []string {
	fields := make([]string, 0, len(names))
	for _, name := range names {
		if name == "id" || modelHasField(env, modelName, name) {
			fields = append(fields, name)
		}
	}
	return fields
}

func contextUserID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	return env.Context().UserID
}

func firstNonZeroFloat(value float64, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func sessionInternalUser(env *record.Env) bool {
	if env == nil {
		return false
	}
	ctx := env.Context()
	if ctx.UserID == 0 {
		return false
	}
	if ctx.UserID == 1 {
		return true
	}
	if sessionHasXMLIDGroup(env, "base.group_user") || sessionHasXMLIDGroup(env, "base.group_system") {
		return true
	}
	groups := menuGroupSet(env)
	return groups[1] || groups[3]
}

func configParameterBool(env *record.Env, key string) bool {
	value := strings.TrimSpace(configParameterValue(env, key))
	return strings.EqualFold(value, "true") || value == "1"
}

func configParameterValue(env *record.Env, key string) string {
	if env == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	if _, ok := env.ModelMetadata("ir.config_parameter"); !ok {
		return ""
	}
	found, err := env.Model("ir.config_parameter").SearchWithOptions(domain.Cond("key", domain.Equal, key), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return ""
	}
	rows, err := found.Read("value")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringValue(rows[0]["value"])
}

func setConfigParameterValue(env *record.Env, key string, value string) error {
	if env == nil {
		return fmt.Errorf("config parameter requires env")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("config parameter requires key")
	}
	found, err := env.Model("ir.config_parameter").SearchWithOptions(domain.Cond("key", domain.Equal, key), record.SearchOptions{Limit: 1})
	if err != nil {
		return err
	}
	if found.Len() > 0 {
		return found.Write(map[string]any{"value": value})
	}
	_, err = env.Model("ir.config_parameter").Create(map[string]any{"key": key, "value": value})
	return err
}

func (s Server) menuPayload(env *record.Env) map[string]any {
	legacyChildren := map[string]any{}
	payload := map[string]any{}
	allIDs := []int64{}
	rootIDs := []int64{}
	groups := map[int64]bool{}
	if env != nil {
		groups = menuGroupSet(env)
	}
	if s.Menus != nil {
		for _, node := range s.Menus.TreeFiltered(groups, s.menuAllowed(env)) {
			rootIDs = append(rootIDs, node.Menu.ID)
			flattenMenuNode(node, node.Menu.ID, payload, legacyChildren, &allIDs, s.Actions)
		}
	}
	payload["root"] = map[string]any{
		"id":                  "root",
		"name":                "root",
		"children":            rootIDs,
		"appID":               false,
		"xmlid":               "",
		"actionID":            false,
		"actionModel":         false,
		"actionPath":          false,
		"webIcon":             nil,
		"webIconData":         nil,
		"webIconDataMimetype": nil,
	}
	payload["children"] = legacyChildren
	payload["all_menu_ids"] = allIDs
	payload["menu_roots"] = rootIDs
	return payload
}

func (s Server) menuAllowed(env *record.Env) func(menu.Menu) bool {
	return func(item menu.Menu) bool {
		if env == nil || env.Policy() == nil || s.Actions == nil || item.ActionID == 0 {
			return true
		}
		act, ok := s.Actions.Get(item.ActionID)
		if !ok || act.Kind != action.ActWindow || strings.TrimSpace(act.ResModel) == "" {
			return true
		}
		return env.Policy().Check(env.Context(), act.ResModel, record.OpRead, nil) == nil
	}
}

func actionPayload(act action.Action) map[string]any {
	views := actionViewsPayload(act)
	payload := map[string]any{
		"id":                  act.ID,
		"name":                act.Name,
		"display_name":        act.Name,
		"type":                string(act.Kind),
		"xml_id":              act.XMLID,
		"res_model":           act.ResModel,
		"res_id":              falseIfZero(act.ResID),
		"view_mode":           firstNonEmptyHTTPString(act.ViewMode, "list,form"),
		"mobile_view_mode":    firstNonEmptyHTTPString(act.MobileViewMode, "kanban"),
		"view_id":             many2OnePayload(act.ViewID),
		"views":               views,
		"search_view_id":      many2OnePayload(act.SearchViewID),
		"domain":              firstNonEmptyHTTPString(act.Domain, "[]"),
		"context":             act.Context,
		"target":              firstNonEmptyHTTPString(act.Target, "current"),
		"limit":               intWithHTTPFallback(act.Limit, 80),
		"help":                act.Help,
		"path":                falseIfEmpty(act.Path),
		"group_ids":           int64ListPayload(act.Groups),
		"binding_model_id":    many2OnePayload(act.BindingModelID),
		"binding_type":        falseIfEmpty(act.BindingType),
		"binding_view_types":  firstNonEmptyHTTPString(act.BindingViewTypes, "list,form"),
		"filter":              act.Filter,
		"cache":               act.Cache,
		"tag":                 falseIfEmpty(act.Tag),
		"embedded_action_ids": embeddedActionPayloads(act.EmbeddedActions),
		"multi_workflow_view": falseIfEmpty(act.MultiWorkflowView),
	}
	enrichWebShellActionPayload(nil, payload)
	return payload
}

func embeddedActionPayloads(actions []action.EmbeddedAction) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, item := range actions {
		out = append(out, map[string]any{
			"id":                item.ID,
			"name":              item.Name,
			"parent_action_id":  many2OnePayload(item.ParentActionID),
			"parent_res_id":     falseIfZero(item.ParentResID),
			"parent_res_model":  item.ParentResModel,
			"action_id":         many2OnePayload(item.ActionID),
			"python_method":     falseIfEmpty(item.PythonMethod),
			"user_id":           many2OnePayload(item.UserID),
			"is_deletable":      item.IsDeletable,
			"default_view_mode": falseIfEmpty(item.DefaultViewMode),
			"filter_ids":        int64ListPayload(item.FilterIDs),
			"domain":            firstNonEmptyHTTPString(item.Domain, "[]"),
			"context":           firstNonEmptyHTTPString(item.Context, "{}"),
			"groups_ids":        int64ListPayload(item.GroupIDs),
		})
	}
	return out
}

func actionViewsPayload(act action.Action) []any {
	refs := append([]action.ViewRef(nil), act.Views...)
	if len(refs) == 0 {
		modes := splitViewModes(firstNonEmptyHTTPString(act.ViewMode, "list,form"))
		if len(modes) == 1 && act.ViewID != 0 {
			refs = append(refs, action.ViewRef{ID: act.ViewID, Mode: modes[0]})
		} else {
			for _, mode := range modes {
				refs = append(refs, action.ViewRef{Mode: mode})
			}
		}
	}
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, []any{falseIfZero(ref.ID), ref.Mode})
	}
	return out
}

func flattenMenuNode(node menu.Node, appID int64, payload map[string]any, legacyChildren map[string]any, allIDs *[]int64, actions *action.Registry) {
	childIDs := make([]int64, 0, len(node.Children))
	for _, child := range node.Children {
		childIDs = append(childIDs, child.Menu.ID)
	}
	actionID, actionModel, actionPath := menuEffectiveAction(node, actions)
	directActionID := node.Menu.ActionID
	webIcon := falseIfEmpty(node.Menu.WebIcon)
	webIconData := falseIfEmpty(node.Menu.WebIconData)
	if node.Menu.ID == appID && webIconData == false && node.Menu.WebIcon == "" {
		webIconData = "/web/static/img/default_icon_app.png"
	}
	entry := map[string]any{
		"id":                  node.Menu.ID,
		"name":                node.Menu.Name,
		"children":            childIDs,
		"appID":               appID,
		"xmlid":               node.Menu.XMLID,
		"actionID":            falseIfZero(actionID),
		"directActionID":      falseIfZero(directActionID),
		"hasDirectAction":     directActionID != 0,
		"actionModel":         falseIfEmpty(actionModel),
		"actionPath":          falseIfEmpty(actionPath),
		"webIcon":             webIcon,
		"webIconData":         webIconData,
		"webIconDataMimetype": falseIfEmpty(node.Menu.WebIconDataMimetype),
		"sequence":            node.Menu.Sequence,
		"parent_id":           false,
		"action":              false,
		"groups":              node.Menu.Groups,
		"is_app":              node.Menu.ParentID == 0,
		"parent_menu":         node.Menu.ParentID,
		"action_model":        falseIfEmpty(actionModel),
		"action_path":         falseIfEmpty(actionPath),
	}
	if node.Menu.ParentID != 0 {
		entry["parent_id"] = node.Menu.ParentID
	}
	if actionID != 0 {
		entry["action"] = firstNonEmptyHTTPString(node.Menu.Action, actionModel+","+strconv.FormatInt(actionID, 10))
	}
	payload[strconv.FormatInt(node.Menu.ID, 10)] = entry
	legacyChildren[strconv.FormatInt(node.Menu.ID, 10)] = entry
	*allIDs = append(*allIDs, node.Menu.ID)
	for _, child := range node.Children {
		flattenMenuNode(child, appID, payload, legacyChildren, allIDs, actions)
	}
}

func menuEffectiveAction(node menu.Node, actions *action.Registry) (int64, string, string) {
	if node.Menu.ActionID != 0 {
		return node.Menu.ActionID, menuActionModel(node.Menu), menuActionPath(node.Menu, actions)
	}
	for _, child := range node.Children {
		if id, model, path := menuEffectiveAction(child, actions); id != 0 {
			return id, model, path
		}
	}
	return 0, "", ""
}

func menuActionModel(menu menu.Menu) string {
	if menu.ActionModel != "" {
		return menu.ActionModel
	}
	if menu.ActionID != 0 {
		return string(action.ActWindow)
	}
	return ""
}

func menuActionPath(menu menu.Menu, actions *action.Registry) string {
	if menu.ActionPath != "" {
		return menu.ActionPath
	}
	if actions == nil || menu.ActionID == 0 {
		return ""
	}
	if action, ok := actions.Get(menu.ActionID); ok {
		return action.Path
	}
	return ""
}

func many2OnePayload(id int64) any {
	if id == 0 {
		return false
	}
	return []any{id, ""}
}

func falseIfZero(id int64) any {
	if id == 0 {
		return false
	}
	return id
}

func falseIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return false
	}
	return value
}

func intWithHTTPFallback(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func int64ListPayload(values []int64) []int64 {
	if values == nil {
		return []int64{}
	}
	return append([]int64(nil), values...)
}

func companyIDs(ctx record.Context) []int64 {
	ids := uniqueInt64Slice(ctx.CompanyIDs)
	if ctx.CompanyID != 0 && !containsHTTPInt64(ids, ctx.CompanyID) {
		ids = append(ids, ctx.CompanyID)
	}
	return orderedCompanyIDs(ctx.CompanyID, ids)
}

func orderedCompanyIDs(activeID int64, ids []int64) []int64 {
	ids = uniqueInt64Slice(ids)
	if len(ids) == 0 {
		if activeID != 0 {
			return []int64{activeID}
		}
		return []int64{}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if activeID == 0 {
		return ids
	}
	out := make([]int64, 0, len(ids))
	if containsHTTPInt64(ids, activeID) {
		out = append(out, activeID)
	}
	for _, id := range ids {
		if id != activeID {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		out = append(out, activeID)
	}
	return out
}

func companyPayload(ids []int64) map[string]any {
	companies := map[string]any{}
	for _, id := range ids {
		companies[strconv.FormatInt(id, 10)] = map[string]any{
			"id":       id,
			"name":     fmt.Sprintf("Company %d", id),
			"sequence": int(id),
		}
	}
	return companies
}

func defaultCurrencies() map[string]any {
	return map[string]any{
		"1": map[string]any{
			"id":                  1,
			"name":                "USD",
			"symbol":              "$",
			"position":            "before",
			"digits":              []int{69, 2},
			"rounding":            0.01,
			"decimal_separator":   ".",
			"thousands_separator": ",",
		},
	}
}

func registryHash(ctx record.Context) string {
	payload, _ := json.Marshal(map[string]any{
		"uid":         ctx.UserID,
		"company_id":  ctx.CompanyID,
		"company_ids": companyIDs(ctx),
		"groups":      groupIDsFromContext(ctx),
		"db":          stringContextValue(ctx, "db", "gorp"),
		"edition":     stringContextValue(ctx, "edition", "e"),
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func modulesPayload(modules map[string]module.Manifest) map[string]any {
	states := map[string]string{}
	for name := range modules {
		states[name] = "installed"
	}
	return modulesPayloadWithStates(modules, states)
}

func modulesPayloadFromEnv(env *record.Env, modules map[string]module.Manifest) map[string]any {
	states := moduleStatesFromEnv(env)
	return modulesPayloadWithStates(modules, states)
}

func moduleStatesFromEnv(env *record.Env) map[string]string {
	if env == nil {
		return map[string]string{}
	}
	found, err := env.Model("ir.module.module").Search(domain.And())
	if err != nil {
		return map[string]string{}
	}
	rows, err := found.Read("name", "state")
	if err != nil {
		return map[string]string{}
	}
	states := map[string]string{}
	for _, row := range rows {
		name := stringValue(row["name"])
		if name == "" {
			continue
		}
		state := stringValue(row["state"])
		if state == "" {
			state = "uninstalled"
		}
		states[name] = state
	}
	return states
}

func modulesPayloadWithStates(modules map[string]module.Manifest, states map[string]string) map[string]any {
	if len(modules) == 0 {
		return map[string]any{"installed_modules": []string{}, "modules": map[string]any{}}
	}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	installedNames := make([]string, 0, len(names))
	items := map[string]any{}
	for _, name := range names {
		manifest := modules[name]
		state := states[name]
		if state == "" {
			state = "uninstalled"
		}
		if state == "installed" {
			installedNames = append(installedNames, name)
		}
		items[name] = map[string]any{
			"name":           manifest.Name,
			"technical_name": manifest.TechnicalName,
			"version":        manifest.Version,
			"category":       manifest.Category,
			"state":          state,
			"application":    manifest.Application,
			"auto_install":   manifest.AutoInstall,
			"installable":    manifest.Installable,
			"depends":        append([]string(nil), manifest.Depends...),
		}
	}
	return map[string]any{"installed_modules": installedNames, "modules": items}
}

func stringContextValue(ctx record.Context, key string, fallback string) string {
	if value, ok := ctx.Values[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

func groupSetFromContext(ctx record.Context) map[int64]bool {
	set := map[int64]bool{}
	for _, id := range groupIDsFromContext(ctx) {
		set[id] = true
	}
	return set
}

func groupIDsFromSet(set map[int64]bool) []int64 {
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func menuGroupSet(env *record.Env) map[int64]bool {
	if env == nil {
		return map[int64]bool{}
	}
	ctx := env.Context()
	set := groupSetFromContext(ctx)
	if provider, ok := env.Policy().(interface {
		EffectiveGroupIDs(int64) map[int64]bool
	}); ok && ctx.UserID != 0 {
		for id := range provider.EffectiveGroupIDs(ctx.UserID) {
			set[id] = true
		}
	}
	return set
}

func groupPayload(ids []int64) map[string]any {
	groups := map[string]any{}
	for _, id := range ids {
		groups[strconv.FormatInt(id, 10)] = map[string]any{"id": id}
	}
	return groups
}

func groupIDsFromContext(ctx record.Context) []int64 {
	raw, ok := ctx.Values["group_ids"]
	if !ok {
		return nil
	}
	ids := int64Slice(raw)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func cloneContextValues(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func loginAsSessionID(r *http.Request) string {
	if value := cookieSessionID(r); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("session_id"))
}

func cookieSessionID(r *http.Request) string {
	if cookie, err := r.Cookie("session_id"); err == nil {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			return value
		}
	}
	return ""
}

func cookieCompanyIDs(r *http.Request) []int64 {
	if r == nil {
		return nil
	}
	cookie, err := r.Cookie("cids")
	if err != nil {
		return nil
	}
	return parseCompanyIDsCookie(cookie.Value)
}

func parseCompanyIDsCookie(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "-")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]bool{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func setCompanyIDsCookie(w http.ResponseWriter, ids []int64) {
	value := companyIDsCookieValue(ids)
	if value == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "cids",
		Value:    value,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
}

func companyIDsCookieValue(ids []int64) string {
	ids = uniqueInt64Slice(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, "-")
}

func parseLoginAsTargetID(path string) (int64, error) {
	raw := strings.TrimPrefix(path, "/web/login_as/")
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, fmt.Errorf("invalid login_as target")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid login_as target")
	}
	return id, nil
}

func safeWebRedirect(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/web"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "/web"
	}
	if value == "/web" || strings.HasPrefix(value, "/web/") || strings.HasPrefix(value, "/web?") || strings.HasPrefix(value, "/web#") {
		return value
	}
	return "/web"
}

func safeDebugRedirect(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/odoo?debug=1"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "/web"
	}
	if value == "/odoo" || strings.HasPrefix(value, "/odoo/") || strings.HasPrefix(value, "/odoo?") || strings.HasPrefix(value, "/odoo#") {
		return value
	}
	return safeWebRedirect(value)
}

func writeLoginAsError(w http.ResponseWriter, err error) {
	switch err {
	case impersonation.ErrSessionNotFound:
		http.Error(w, err.Error(), http.StatusBadRequest)
	case impersonation.ErrUserNotFound:
		http.Error(w, err.Error(), http.StatusNotFound)
	case impersonation.ErrUnauthorized,
		impersonation.ErrSelfImpersonation,
		impersonation.ErrTargetInactive,
		impersonation.ErrTargetSuperuser,
		impersonation.ErrPortalDisabled,
		impersonation.ErrGroupMismatch,
		impersonation.ErrDebugRouteDisabled,
		impersonation.ErrNotImpersonating:
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func arg(args []any, index int) any {
	if index < 0 || index >= len(args) {
		return nil
	}
	return args[index]
}

func kwarg(kwargs map[string]any, key string) any {
	if kwargs == nil {
		return nil
	}
	return kwargs[key]
}

func mapArg(args []any, index int) map[string]any {
	value, _ := arg(args, index).(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func mapValue(value any) map[string]any {
	typed, _ := value.(map[string]any)
	if typed == nil {
		return map[string]any{}
	}
	return typed
}

func mapList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, mapValue(item))
	}
	return out
}

func resUsersOnchangeValues(env *record.Env, values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	role := stringValue(out["role"])
	if role != "group_system" && role != "group_user" {
		return out
	}
	groupUserID := httpXMLIDResID(env, "base", "group_user", "res.groups")
	groupSystemID := httpXMLIDResID(env, "base", "group_system", "res.groups")
	if groupUserID == 0 || groupSystemID == 0 {
		return out
	}
	current := httpGroupIDsFromValue(firstNonNil(out["group_ids"], out["groups_id"]))
	if !httpIDsContain(httpEffectiveGroupIDs(env, current), groupUserID) {
		return out
	}
	next := []int64{}
	for _, id := range current {
		if id != groupUserID && id != groupSystemID {
			next = appendUniqueHTTPID(next, id)
		}
	}
	if role == "group_system" {
		next = appendUniqueHTTPID(next, groupSystemID)
	} else {
		next = appendUniqueHTTPID(next, groupUserID)
	}
	out["group_ids"] = []any{[]any{int64(6), false, next}}
	return out
}

func delegationOnchangeValues(env *record.Env, values map[string]any, changed []string, context map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	if onchangeIncludes(changed, "user_id") || onchangeIncludes(changed, "employee_id") {
		userID := firstID(firstNonNil(out["user_id"], out["user_ids"]))
		if userID == 0 {
			userID = delegationUserIDFromEmployee(env, firstID(out["employee_id"]))
			if userID != 0 {
				out["user_id"] = userID
			}
		}
		if userID != 0 {
			out["lines"] = delegationLineSeedCommands(env, userID, truthyHTTPValue(context["delegation"]))
		}
	}
	if onchangeIncludes(changed, "delegateTo_employee_id") || onchangeIncludes(changed, "delegate_to_employee_id") {
		delegateID := firstID(firstNonNil(out["delegateTo_employee_id"], out["delegate_to_employee_id"]))
		out["lines"] = delegationLineEmployeeCommands(out["lines"], delegateID)
	}
	if onchangeIncludes(changed, "one_employee") && !truthyHTTPValue(out["one_employee"]) {
		out["delegateTo_employee_id"] = false
		out["delegate_to_employee_id"] = false
	}
	return out
}

func onchangeIncludes(changed []string, fieldName string) bool {
	if len(changed) == 0 {
		return true
	}
	for _, item := range changed {
		if item == fieldName {
			return true
		}
	}
	return false
}

func delegationUserIDFromEmployee(env *record.Env, employeeID int64) int64 {
	if env == nil || employeeID == 0 || !modelHasField(env, "hr.employee", "user_id") {
		return 0
	}
	rows, err := env.Model("hr.employee").Browse(employeeID).Read("user_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["user_id"])
}

func delegationLineSeedCommands(env *record.Env, userID int64, delegationContext bool) []any {
	groupIDs := delegationAllowedGroupIDs(env, userID, delegationContext)
	commands := make([]any, 0, len(groupIDs)+1)
	commands = append(commands, []any{int64(5), false, false})
	for _, groupID := range groupIDs {
		commands = append(commands, []any{int64(0), false, map[string]any{"group_id": groupID}})
	}
	return commands
}

func delegationAllowedGroupIDs(env *record.Env, userID int64, delegationContext bool) []int64 {
	if env == nil || userID == 0 {
		return nil
	}
	userFields := availableModelFields(env, "res.users", "groups_id", "group_ids")
	rows, err := env.Model("res.users").Browse(userID).Read(userFields...)
	if err != nil || len(rows) == 0 {
		return nil
	}
	userGroups := httpGroupIDsFromValue(firstNonNil(rows[0]["groups_id"], rows[0]["group_ids"]))
	if len(userGroups) == 0 || !modelHasField(env, "res.groups", "allow_delegation") {
		return nil
	}
	groupRows, err := env.Model("res.groups").Browse(userGroups...).Read(availableModelFields(env, "res.groups", "id", "name", "full_name", "name_delegation", "allow_delegation")...)
	if err != nil {
		return nil
	}
	type groupSort struct {
		id   int64
		name string
	}
	groups := []groupSort{}
	for _, row := range groupRows {
		if !truthyHTTPValue(row["allow_delegation"]) {
			continue
		}
		label := firstTextHTTP(row["full_name"], row["name"])
		if delegationContext {
			label = firstTextHTTP(row["name_delegation"], row["full_name"], row["name"])
		}
		groups = append(groups, groupSort{id: int64Value(row["id"]), name: label})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].name == groups[j].name {
			return groups[i].id < groups[j].id
		}
		return groups[i].name < groups[j].name
	})
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.id)
	}
	return ids
}

func delegationLineEmployeeCommands(raw any, employeeID int64) []any {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		command, ok := item.([]any)
		if !ok || len(command) == 0 {
			if id := int64Value(item); id != 0 {
				out = append(out, []any{int64(1), id, map[string]any{"employee_id": falseIfZero(employeeID)}})
			}
			continue
		}
		switch int64Value(command[0]) {
		case 0:
			next := append([]any(nil), command...)
			for len(next) < 3 {
				next = append(next, false)
			}
			values := mapValue(next[2])
			values["employee_id"] = falseIfZero(employeeID)
			next[2] = values
			out = append(out, next)
		case 1:
			next := append([]any(nil), command...)
			for len(next) < 3 {
				next = append(next, false)
			}
			values := mapValue(next[2])
			values["employee_id"] = falseIfZero(employeeID)
			next[2] = values
			out = append(out, next)
		case 4:
			if len(command) > 1 {
				out = append(out, []any{int64(1), int64Value(command[1]), map[string]any{"employee_id": falseIfZero(employeeID)}})
			}
		default:
			out = append(out, command)
		}
	}
	return out
}

func resGroupsDefinitions(env *record.Env) (map[string]any, error) {
	found, err := env.Model("res.groups").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "implied_ids", "disjoint_ids")
	if err != nil {
		return nil, err
	}
	refs, err := resGroupExternalRefs(env)
	if err != nil {
		return nil, err
	}
	groups := map[string]any{}
	for _, row := range rows {
		id := int64Value(row["id"])
		if id == 0 {
			continue
		}
		ref := refs[id]
		if ref == "" {
			ref = strconv.FormatInt(id, 10)
		}
		groups[strconv.FormatInt(id, 10)] = map[string]any{
			"id":        id,
			"ref":       ref,
			"supersets": int64Slice(row["implied_ids"]),
			"disjoints": int64Slice(row["disjoint_ids"]),
		}
	}
	return map[string]any{"groups": groups}, nil
}

func resGroupExternalRefs(env *record.Env) (map[int64]string, error) {
	found, err := env.Model("ir.model.data").Search(domain.Cond("model", domain.Equal, "res.groups"))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("module", "name", "complete_name", "res_id")
	if err != nil {
		return nil, err
	}
	refs := map[int64]string{}
	for _, row := range rows {
		id := int64Value(row["res_id"])
		if id == 0 {
			continue
		}
		ref := stringValue(row["complete_name"])
		if ref == "" {
			moduleName := stringValue(row["module"])
			name := stringValue(row["name"])
			if moduleName != "" && name != "" {
				ref = moduleName + "." + name
			}
		}
		if ref != "" && refs[id] == "" {
			refs[id] = ref
		}
	}
	return refs, nil
}

func httpXMLIDResID(env *record.Env, moduleName string, name string, modelName string) int64 {
	if env == nil {
		return 0
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", domain.Equal, moduleName),
		domain.Cond("name", domain.Equal, name),
		domain.Cond("model", domain.Equal, modelName),
	))
	if err != nil || found.Len() == 0 {
		return 0
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64Value(rows[0]["res_id"])
}

func httpEffectiveGroupIDs(env *record.Env, groupIDs []int64) []int64 {
	if env == nil {
		return nil
	}
	seen := map[int64]bool{}
	var visit func(int64)
	visit = func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		rows, err := env.Model("res.groups").Browse(id).Read("implied_ids")
		if err != nil || len(rows) == 0 {
			return
		}
		for _, impliedID := range httpGroupIDsFromValue(rows[0]["implied_ids"]) {
			visit(impliedID)
		}
	}
	for _, groupID := range groupIDs {
		visit(groupID)
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func httpGroupIDsFromValue(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		out := []int64{}
		for _, id := range typed {
			out = appendUniqueHTTPID(out, id)
		}
		return out
	case []any:
		if len(typed) == 0 {
			return nil
		}
		if httpIsX2ManySetCommand(typed) {
			return httpGroupIDsFromValue(typed[2])
		}
		if httpX2ManyCommandItems(typed) {
			ids := []int64{}
			for _, item := range typed {
				command := item.([]any)
				switch int64Value(command[0]) {
				case 2, 3:
					if len(command) > 1 {
						ids = httpRemoveID(ids, int64Value(command[1]))
					}
				case 4:
					if len(command) > 1 {
						ids = appendUniqueHTTPID(ids, int64Value(command[1]))
					}
				case 5:
					ids = []int64{}
				case 6:
					if len(command) > 2 {
						ids = httpGroupIDsFromValue(command[2])
					}
				}
			}
			return ids
		}
		if len(typed) >= 2 {
			if id := int64Value(typed[0]); id != 0 {
				if _, ok := typed[1].(string); ok {
					return []int64{id}
				}
			}
		}
		out := []int64{}
		for _, item := range typed {
			if id := int64Value(item); id != 0 {
				out = appendUniqueHTTPID(out, id)
				continue
			}
			for _, id := range httpGroupIDsFromValue(item) {
				out = appendUniqueHTTPID(out, id)
			}
		}
		return out
	default:
		if id := int64Value(value); id != 0 {
			return []int64{id}
		}
		return nil
	}
}

func httpIsX2ManySetCommand(items []any) bool {
	return len(items) >= 3 && int64Value(items[0]) == 6
}

func httpX2ManyCommandItems(items []any) bool {
	for _, item := range items {
		command, ok := item.([]any)
		if !ok || len(command) == 0 {
			return false
		}
	}
	return true
}

func httpIDsContain(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func httpRemoveID(ids []int64, target int64) []int64 {
	out := ids[:0]
	for _, id := range ids {
		if id != target {
			out = append(out, id)
		}
	}
	return out
}

func trackingValuesFromAny(value any) []internalmail.TrackingValue {
	rows := trackingValueMaps(value)
	out := make([]internalmail.TrackingValue, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		out = append(out, internalmail.TrackingValue{
			FieldID:          int64Value(row["field_id"]),
			FieldInfo:        trackingTextValue(row["field_info"]),
			FieldName:        firstTextHTTP(row["field_name"], row["name"]),
			FieldDesc:        firstTextHTTP(row["field_desc"], row["field_description"], row["string"]),
			FieldType:        firstTextHTTP(row["field_type"], row["type"]),
			OldValueInteger:  int64Value(row["old_value_integer"]),
			OldValueFloat:    floatValue(row["old_value_float"]),
			OldValueChar:     stringValue(row["old_value_char"]),
			OldValueText:     stringValue(row["old_value_text"]),
			OldValueDatetime: accountingDateValue(row["old_value_datetime"]),
			NewValueInteger:  int64Value(row["new_value_integer"]),
			NewValueFloat:    floatValue(row["new_value_float"]),
			NewValueChar:     stringValue(row["new_value_char"]),
			NewValueText:     stringValue(row["new_value_text"]),
			NewValueDatetime: accountingDateValue(row["new_value_datetime"]),
			CurrencyID:       int64Value(row["currency_id"]),
		})
	}
	return out
}

func trackingValueMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case map[string]any:
		return []map[string]any{typed}
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row := trackingValueMap(item); row != nil {
				out = append(out, row)
			}
		}
		return out
	default:
		return nil
	}
}

func trackingValueMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case []any:
		if len(typed) >= 3 && int64Value(typed[0]) == 0 {
			return mapValue(typed[2])
		}
		return nil
	default:
		return nil
	}
}

func trackingTextValue(value any) string {
	if text := stringValue(value); text != "" {
		return text
	}
	switch value.(type) {
	case map[string]any, []any:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
	}
	return ""
}

func int64Slice(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			out = append(out, int64Value(item))
		}
		return out
	case float64:
		return []int64{int64(typed)}
	case int:
		return []int64{int64(typed)}
	case int64:
		return []int64{typed}
	default:
		return nil
	}
}

func uniqueInt64Slice(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseCommaIDs(text string) []int64 {
	parts := strings.Split(strings.TrimSpace(text), ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func joinIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return id
	default:
		return 0
	}
}

func firstID(value any) int64 {
	ids := int64Slice(value)
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func intValue(value any) int {
	return int(int64Value(value))
}

func boolPointerValue(value any) *bool {
	if value == nil {
		return nil
	}
	parsed := boolHTTPWithFallback(value, false)
	return &parsed
}

func readGroupLazyPointer(value any) *bool {
	if value == nil {
		return nil
	}
	parsed := accountingBoolValue(value)
	return &parsed
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		out, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return out
	default:
		return 0
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func validSimpleFieldName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || ('a' <= char && char <= 'z') || ('A' <= char && char <= 'Z') || (index > 0 && '0' <= char && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func firstTextHTTP(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(stringValue(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func byteValue(value any) []byte {
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...)
	case string:
		return []byte(typed)
	default:
		return nil
	}
}

func isSendableAccountingMove(move coreaccounting.Move) bool {
	if move.State != coreaccounting.MovePosted {
		return false
	}
	switch move.MoveType {
	case "out_invoice", "out_refund", "out_receipt":
		return true
	default:
		return false
	}
}

func manualSendSelected(methods string) bool {
	var values map[string]any
	if err := json.Unmarshal([]byte(methods), &values); err == nil {
		return accountingBoolValue(values["manual"])
	}
	compact := strings.ToLower(strings.ReplaceAll(methods, " ", ""))
	return compact == "manual" || compact == `["manual"]`
}

func accountingBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func boolKwargDefault(kwargs map[string]any, key string, fallback bool) bool {
	if kwargs == nil {
		return fallback
	}
	value, ok := kwargs[key]
	if !ok {
		return fallback
	}
	return accountingBoolValue(value)
}

func accountingDateValue(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed
			}
		}
		return time.Time{}
	default:
		return time.Time{}
	}
}

func accountingScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int:
		if typed == 0 {
			return ""
		}
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		if typed == 0 {
			return ""
		}
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == 0 {
			return ""
		}
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func accountingAccountKind(values ...any) coreaccounting.AccountKind {
	for _, value := range values {
		text := strings.TrimSpace(stringValue(value))
		if text != "" {
			return coreaccounting.AccountKind(text)
		}
	}
	return ""
}

func activeIDsFromCallContext(req callKWRequest) []int64 {
	context := mapValue(kwarg(req.Kwargs, "context"))
	if len(context) == 0 {
		context = mapValue(req.Values["context"])
	}
	if context["active_model"] != "account.move" {
		return nil
	}
	if ids := int64Slice(context["active_ids"]); len(ids) > 0 {
		return ids
	}
	if id := int64Value(context["active_id"]); id != 0 {
		return []int64{id}
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func applyReadGroupDomain(rows []map[string]any, base any) {
	baseDomain := domainItems(base)
	if len(baseDomain) == 0 {
		return
	}
	for _, row := range rows {
		merged := append([]any{}, baseDomain...)
		merged = append(merged, domainItems(row["__domain"])...)
		row["__domain"] = merged
	}
}

func domainItems(value any) []any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return append([]any(nil), typed...)
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return []any{typed}
	}
}

func parseDomain(value any) (domain.Node, error) {
	return domain.Parse(value)
}

func cleanBusChannels(channels []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		channel = strings.TrimSpace(channel)
		if channel == "" || seen[channel] {
			continue
		}
		seen[channel] = true
		out = append(out, channel)
	}
	return out
}

func busEventPayload(event notifications.Event) map[string]any {
	return map[string]any{
		"id":      event.ID,
		"channel": event.Channel,
		"type":    event.Name,
		"name":    event.Name,
		"payload": cloneHTTPMap(event.Payload),
	}
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, Range")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
}

func (s Server) publishUserBus(env *record.Env, name string, payload map[string]any) {
	if s.Bus == nil || env == nil || env.Context().UserID == 0 {
		return
	}
	s.Bus.Publish(userBusChannel(env.Context().UserID), name, payload, time.Now().UTC())
}

func (s Server) publishMailMessageBus(env *record.Env, messageID any, name string, payload map[string]any) {
	if s.Bus == nil {
		return
	}
	channel := mailMessageBusChannel(env, int64Value(messageID))
	if channel == "" {
		return
	}
	s.Bus.Publish(channel, name, payload, time.Now().UTC())
}

func mailMessageBusChannel(env *record.Env, messageID int64) string {
	if env == nil || messageID == 0 {
		return ""
	}
	ctx := env.Context()
	systemCtx := ctx
	systemCtx.UserID = 1
	rows, err := env.WithContext(systemCtx).Model("mail.message").Browse(messageID).Read("model", "res_id")
	if err == nil && len(rows) > 0 && stringValue(rows[0]["model"]) == "discuss.channel" {
		if channelID := int64Value(rows[0]["res_id"]); channelID != 0 {
			return fmt.Sprintf("discuss.channel/%d", channelID)
		}
	}
	if guestID := int64Value(firstNonNil(ctx.Values["guest_id"], ctx.Values["mail_guest_id"])); guestID != 0 {
		return guestBusChannel(guestID)
	}
	if ctx.UserID != 0 {
		return userBusChannel(ctx.UserID)
	}
	return ""
}

func mailMessageStarredResult(result any) bool {
	values, ok := result.(map[string]any)
	if !ok {
		return false
	}
	switch rows := values["mail.message"].(type) {
	case []map[string]any:
		return len(rows) > 0 && accountingBoolValue(rows[0]["starred"])
	case []any:
		if len(rows) == 0 {
			return false
		}
		return accountingBoolValue(mapValue(rows[0])["starred"])
	default:
		return false
	}
}

func userBusChannel(userID int64) string {
	return fmt.Sprintf("user/%d", userID)
}

func guestBusChannel(guestID int64) string {
	return fmt.Sprintf("guest/%d", guestID)
}

func cloneHTTPMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
