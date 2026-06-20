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
	"io"
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
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/web", s.webClient)
	mux.HandleFunc("/web/", s.webClient)
	mux.HandleFunc("/web/health", s.health)
	mux.HandleFunc("/web/session/info", s.sessionInfo)
	mux.HandleFunc("/web/session/get_session_info", s.sessionInfo)
	mux.HandleFunc("/web/session/authenticate", s.authenticate)
	mux.HandleFunc("/web/session/check", s.sessionCheck)
	mux.HandleFunc("/web/session/modules", s.sessionModules)
	mux.HandleFunc("/web/session/destroy", s.sessionDestroy)
	mux.HandleFunc("/web/session/logout", s.logout)
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
	mux.HandleFunc("/mail/thread/messages", s.mailThreadMessages)
	mux.HandleFunc("/mail/starred/messages", s.mailStarredMessages)
	mux.HandleFunc("/mail/chatter_fetch", s.mailChatterFetch)
	mux.HandleFunc("/portal/chatter_init", s.portalChatterInit)
	mux.HandleFunc("/mail/view", s.mailView)
	mux.HandleFunc("/mail/thread/recipients", s.mailThreadRecipients)
	mux.HandleFunc("/mail/thread/recipients/fields", s.mailThreadRecipientsFields)
	mux.HandleFunc("/mail/thread/recipients/get_suggested_recipients", s.mailThreadRecipientsGetSuggestedRecipients)
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
		Model:  req.ThreadModel,
		ResID:  req.ThreadID,
		Limit:  intValue(req.FetchParams["limit"]),
		Offset: intValue(req.FetchParams["offset"]),
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
	if r.URL.Path != "/web" && r.URL.Path != "/web/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, webClientShellHTML)
}

const webClientShellHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Odoo</title>
	<style>
	:root {
		--bg: #f4f5f7;
		--panel: #ffffff;
		--panel-soft: #f8f8f8;
		--text: #1f2933;
		--muted: #6f7682;
		--line: #d9dce1;
		--accent: #714b67;
		--accent-2: #017e84;
		--accent-text: #ffffff;
		--danger: #b42318;
		--radius: 3px;
		--topbar: #714b67;
		--topbar-hover: #604058;
		--sidebar: #f7f7f7;
	}
	body[data-theme="standard"] {
		--bg: #f5f5f5;
		--accent: #017e84;
		--topbar: #2b3940;
		--topbar-hover: #1f2b31;
		--text: #1f2933;
	}
	* { box-sizing: border-box; }
	html, body { min-height: 100%; }
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
		padding: 6px 10px;
		cursor: pointer;
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
		background: var(--panel);
		color: var(--text);
		border-color: var(--line);
	}
	button.secondary:hover {
		background: var(--panel-soft);
		border-color: #c7ccd3;
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
		border-bottom: 1px solid var(--line);
		padding: 7px 9px;
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		background: #f8f9fa;
	}
	tr:hover td { background: #f7fbfb; }
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
	.app-grid {
		display: grid;
		grid-template-columns: repeat(6, minmax(0, 1fr));
		gap: 12px;
		margin-bottom: 14px;
	}
	.app-card {
		display: grid;
		place-items: center;
		gap: 7px;
		min-height: 104px;
		text-align: center;
		border: 0;
		background: transparent;
		color: var(--text);
		padding: 10px 8px;
	}
	.app-card::before {
		content: "";
		width: 44px;
		height: 44px;
		border-radius: 10px;
		background: var(--accent);
		box-shadow: inset 0 -10px 20px rgba(0,0,0,.12);
	}
	.app-card:hover {
		background: rgba(113,75,103,.07);
		color: var(--text);
		border-color: transparent;
	}
	.app-card strong { font-size: 14px; font-weight: 500; }
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
	.menu-list {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 8px;
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
		min-width: 220px;
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
	}
	.o-launcher span {
		width: 4px;
		height: 4px;
		background: rgba(255,255,255,.88);
		border-radius: 1px;
	}
	.o-nav {
		display: flex;
		gap: 3px;
		flex: 1;
	}
	.o-nav:empty {
		display: none;
	}
	.o-nav button {
		background: transparent;
		border-color: transparent;
		color: rgba(255,255,255,.9);
	}
	.o-nav button:hover {
		background: rgba(255,255,255,.12);
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
	.o-user {
		padding: 5px 8px;
		border-radius: var(--radius);
		background: rgba(255,255,255,.11);
		white-space: nowrap;
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
	body {
		overflow-x: hidden;
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
	}
	body[data-view="apps"] aside {
		display: none;
	}
	body:not([data-view="apps"]) aside {
		display: none;
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
	.o-control-panel {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		min-height: 66px;
		padding: 10px 18px;
		background: #fff;
		border-bottom: 1px solid var(--line);
	}
	.o-control-panel h2 {
		margin: 0;
		font-size: 18px;
		font-weight: 500;
	}
	.o-control-panel .toolbar {
		padding: 0;
	}
	.o-control-panel .field {
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
		background: #eef0f3;
		color: var(--text);
		padding: 28px 24px 40px;
	}
	body[data-theme="standard"] .o-app-launcher-view {
		background: #eef0f3;
	}
	.o-app-shell {
		max-width: 980px;
		margin: 0 auto;
	}
	.o-app-search {
		max-width: 520px;
		margin: 0 auto 24px;
	}
	.o-app-search input {
		height: 38px;
		border-color: #cfd4dc;
		background: #fff;
		color: var(--text);
		text-align: left;
	}
	.o-app-search label {
		display: block;
	}
	.o-app-search input::placeholder {
		color: var(--muted);
	}
	.o-app-launcher-view .muted {
		color: var(--muted);
	}
	.o-app-launcher-view .app-grid {
		grid-template-columns: repeat(auto-fill, minmax(112px, 1fr));
		gap: 18px 14px;
		margin: 0 auto 18px;
		max-width: 880px;
	}
	.o-app-launcher-view .app-card {
		width: 112px;
		min-height: 118px;
		justify-self: center;
		border-radius: 8px;
		color: var(--text);
		padding: 8px;
		overflow: hidden;
	}
	.o-app-launcher-view .app-card::before {
		width: 54px;
		height: 54px;
		border-radius: 12px;
		background: #017e84;
		box-shadow: inset 0 -10px 18px rgba(0,0,0,.14), 0 8px 18px rgba(0,0,0,.16);
	}
	.o-app-launcher-view .app-card:nth-child(4n+2)::before {
		background: #875a7b;
	}
	.o-app-launcher-view .app-card:nth-child(4n+3)::before {
		background: #21b799;
	}
	.o-app-launcher-view .app-card:nth-child(4n+4)::before {
		background: #d5653e;
	}
	.o-app-launcher-view .app-card.has-icon::before {
		display: none;
	}
	.app-icon {
		display: inline-grid;
		place-items: center;
		width: 54px;
		height: 54px;
		border-radius: 12px;
		background: #875a7b;
		color: #fff;
		font-size: 20px;
		font-weight: 600;
		box-shadow: inset 0 -10px 18px rgba(0,0,0,.14), 0 8px 18px rgba(0,0,0,.16);
		overflow: hidden;
	}
	.o-app-launcher-view .app-card:nth-child(4n+2) .app-icon { background: #017e84; }
	.o-app-launcher-view .app-card:nth-child(4n+3) .app-icon { background: #5f6f94; }
	.o-app-launcher-view .app-card:nth-child(4n+4) .app-icon { background: #b05f4a; }
	.app-icon img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}
	.o-app-launcher-view .app-card:hover {
		background: rgba(113,75,103,.08);
		color: var(--text);
	}
	.o-app-launcher-view .app-card strong {
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 500;
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
		margin-top: 0;
		border: 1px solid var(--line);
	}
	.o-list-view th,
	.o-list-view td {
		background: #fff;
	}
	.o-list-view .o_data_row {
		cursor: pointer;
	}
	.o-list-view tr:hover td {
		background: #f2f7f7;
	}
	.module-grid {
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
	}
	.module-card {
		border-radius: 4px;
		min-height: 118px;
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
		min-width: 0;
	}
	.o-form-control .o-breadcrumbs button {
		padding: 0;
		background: transparent;
		border: 0;
		color: var(--accent);
		font-size: 18px;
		font-weight: 500;
	}
	.o-form-control .o-breadcrumbs span {
		color: var(--muted);
	}
	.o-form-content {
		max-width: 980px;
		margin: 0 auto;
	}
	#recordForm {
		background: #fff;
		border: 1px solid var(--line);
		padding: 20px;
	}
	#recordForm label {
		color: var(--muted);
		font-size: 12px;
	}
	#recordForm input {
		margin-top: 5px;
	}
	[hidden] { display: none !important; }
	@media (max-width: 900px) {
		.layout { grid-template-columns: 1fr; }
		aside { border-right: 0; border-bottom: 1px solid var(--line); }
		.grid { grid-template-columns: 1fr 1fr; }
		.app-grid, .module-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
		header { flex-wrap: wrap; padding: 8px 12px; }
	}
	@media (max-width: 620px) {
		header { display: grid; }
		.grid { grid-template-columns: 1fr; }
		.app-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.module-grid, .record-grid { grid-template-columns: 1fr; }
		.toolbar { display: grid; }
		.field.small { max-width: none; }
	}
  </style>
</head>
<body class="o_web_client" data-theme="enterprise" data-view="apps">
  <header class="o_main_navbar">
    <div class="o-brand">
      <button type="button" id="navApps" class="o-launcher-button" data-view="apps" aria-label="Apps"><span class="o-launcher" aria-hidden="true"><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span></span></button>
      <h1>Odoo</h1>
    </div>
    <nav class="o-nav" id="topMenu" aria-label="Application"></nav>
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
    <div class="o-user" id="topUser">Administrator</div>
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
          <h2>Records</h2>
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
            <label class="field">
              Search
              <input id="recordSearch" placeholder="Search">
            </label>
            <button id="loadRows" class="secondary o-debug-only" hidden>Load</button>
            <button id="createPartner" class="secondary">New</button>
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
		for (const panel of document.querySelectorAll(".view-panel")) {
			panel.classList.toggle("active", panel.dataset.view === name);
		}
		for (const button of document.querySelectorAll("button[data-view]")) {
			button.classList.toggle("active", button.dataset.view === name);
		}
	}
	for (const button of document.querySelectorAll("button[data-view]")) {
		button.addEventListener("click", () => setView(button.dataset.view));
	}
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
		workbench.formFields = [];
		workbench.openedRecord = null;
		showRecordForm(false);
		fieldsInput.value = defaultFields[modelSelect.value] || "id,display_name";
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
      const fields = ["id", ...(workbench.fields.length ? workbench.fields : fieldsInput.value.split(",").map((field) => field.trim()).filter(Boolean))].filter((value, index, list) => value && list.indexOf(value) === index);
      const limit = Number(document.getElementById("limit").value || 20);
      document.getElementById("rows").textContent = "Loading...";
      try {
        const payload = await callKW(model, "web_search_read", {
          kwargs: {
            domain: combinedDomain(actionDomain(workbench.action), document.getElementById("recordSearch").value),
            specification: fieldSpecification(fields),
            limit,
            count_limit: 10001,
            order: "id desc",
            context: readContext(workbench.action)
          }
        });
        renderRows((payload && payload.records) || [], fields);
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
      const fields = ["id", ...(workbench.fields.length ? workbench.fields : fieldsInput.value.split(",").map((field) => field.trim()).filter(Boolean))].filter((item, index, list) => item && list.indexOf(item) === index);
      document.getElementById("rows").textContent = "Loading...";
      try {
        const payload = await callKW(modelSelect.value, "web_search_read", {
          kwargs: {
            domain: combinedDomain(actionDomain(workbench.action), value),
            specification: fieldSpecification(fields),
            limit: 20,
            count_limit: 10001,
            order: "id desc",
            context: readContext(workbench.action)
          }
        });
        renderRows((payload && payload.records) || [], fields);
        setView("records");
      } catch (error) {
        if (await ensureSession(error)) return;
        document.getElementById("rows").textContent = "Search error: " + error.message;
        setView("records");
      }
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
      viewInfo: null,
      fields: [],
      fieldLabels: {},
      formFields: []
    };

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

    function viewArchFields(arch) {
      const out = [];
      if (typeof arch !== "string" || !arch) return out;
      try {
        const doc = new DOMParser().parseFromString(arch, "text/xml");
        for (const node of doc.querySelectorAll("field[name]")) {
          const name = node.getAttribute("name");
          if (name && !out.includes(name)) out.push(name);
        }
      } catch (_error) {}
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
      const formView = (((viewInfo || {}).views || {}).form) || {};
      const listFields = viewArchFields(listView.arch).filter((field) => field !== "id");
      const formFields = viewArchFields(formView.arch).filter((field) => field !== "id");
      const fallback = (defaultFields[model] || "id,display_name,name").split(",").map((field) => field.trim()).filter(Boolean);
      workbench.viewInfo = viewInfo || {};
      workbench.fields = (listFields.length ? listFields : fallback).filter((field) => field !== "id");
      workbench.formFields = (formFields.length ? formFields : workbench.fields).filter((field) => field !== "id");
      workbench.fieldLabels = viewFieldLabels(model, viewInfo);
      fieldsInput.value = ["id", ...workbench.fields].filter((value, index, list) => list.indexOf(value) === index).join(",");
      document.getElementById("recordFields").value = ["id", ...workbench.formFields].filter((value, index, list) => list.indexOf(value) === index).join(",");
    }

    function buildWorkbenchPanels() {
      const main = document.querySelector("main");
      const modelPanel = document.getElementById("rows").closest(".panel");

      const appPanel = document.createElement("section");
      appPanel.id = "appsView";
      appPanel.className = "panel view-panel active o-app-launcher-view o_app_launcher";
      appPanel.dataset.view = "apps";
      appPanel.innerHTML = '<div class="o-app-shell"><div class="o-app-search"><label><span class="sr-only">Search apps</span><input id="appSearch" placeholder="Search apps"></label></div><div id="appGrid" class="app-grid"></div><div id="menuStatus" class="o-app-message muted">Loading menus...</div><div id="menuList" class="menu-list o-app-message"></div></div>';
      main.insertBefore(appPanel, modelPanel);

      const modulePanel = document.createElement("section");
      modulePanel.id = "installView";
      modulePanel.className = "panel view-panel o-list-view";
      modulePanel.dataset.view = "install";
      modulePanel.innerHTML = '<div class="o-control-panel o_control_panel"><h2>Apps</h2><div class="toolbar"><label class="field">Search<input id="moduleSearch" placeholder="Filter apps"></label><button id="reloadApps" class="secondary">Reload</button></div></div><div class="o-list-content"><div id="moduleGrid" class="module-grid"></div></div>';
      main.insertBefore(modulePanel, modelPanel.nextSibling);

      const recordPanel = document.createElement("section");
      recordPanel.className = "panel record-panel o_form_view";
      recordPanel.id = "recordPanel";
      recordPanel.hidden = true;
      recordPanel.innerHTML = '<div class="o-control-panel o_control_panel o-form-control"><div class="o-breadcrumbs"><button id="recordBack" type="button" class="secondary">Records</button><span>/</span><h2 id="recordTitle">Record</h2></div><div class="toolbar"><button id="saveRecord">Save</button><button id="readRecord" class="secondary">Reload</button><input id="recordModel" hidden><input id="recordID" hidden><input id="recordFields" hidden></div></div><div class="o-list-content o-form-content o_form_sheet_bg"><div id="recordForm" class="record-grid o-form-sheet o_form_sheet"></div><pre id="recordRaw" hidden></pre></div>';
      modelPanel.append(recordPanel);

      document.getElementById("reloadApps").addEventListener("click", loadInstallApps);
      document.getElementById("moduleSearch").addEventListener("input", loadInstallApps);
      document.getElementById("appSearch").addEventListener("input", () => renderApps(workbench.menus));
      document.getElementById("recordBack").addEventListener("click", () => {
        workbench.openedRecord = null;
        showRecordForm(false);
      });
      document.getElementById("readRecord").addEventListener("click", () => {
        if (workbench.openedRecord) openRecord(workbench.openedRecord.model, workbench.openedRecord.id);
      });
      document.getElementById("saveRecord").addEventListener("click", saveRecord);
    }

    function showRecordForm(active) {
      const recordPanel = document.getElementById("recordPanel");
      if (recordPanel) recordPanel.hidden = !active;
      const listContent = document.querySelector("#recordsView > .o-list-content");
      if (listContent) listContent.hidden = active;
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

    function renderSidebarMenu(menu) {
      const moduleHost = document.getElementById("modules");
      const topMenu = document.getElementById("topMenu");
      moduleHost.replaceChildren();
      topMenu.replaceChildren();
      document.getElementById("sidebarTitle").textContent = menu && menu.name ? menu.name : "Menu";
      const childIDs = (menu && menu.children) || [];
      for (const childID of childIDs) {
        const child = menuEntry(childID);
        if (!child) continue;
        const item = document.createElement("li");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "secondary";
        button.textContent = child.name || "Menu";
        button.addEventListener("click", () => openMenu(child.id));
        item.append(button);
        moduleHost.append(item);
        const topButton = document.createElement("button");
        topButton.type = "button";
        topButton.textContent = child.name || "Menu";
        topButton.addEventListener("click", () => openMenu(child.id));
        topMenu.append(topButton);
      }
      if (!moduleHost.children.length && menu) {
        const item = document.createElement("li");
        const left = document.createElement("span");
        left.textContent = menu.name || "Menu";
        item.append(left);
        moduleHost.append(item);
      }
    }

    function menuEntry(menuID) {
      return workbench.menus[String(menuID)] || null;
    }

    function menuRootIDs(payload) {
      if (!payload) return [];
      if (Array.isArray(payload.menu_roots)) return payload.menu_roots;
      const root = payload.root || {};
      if (Array.isArray(root.children)) return root.children;
      return [];
    }

    function renderApps(payload) {
      workbench.menus = payload || {};
      const grid = document.getElementById("appGrid");
      const needle = ((document.getElementById("appSearch") || {}).value || "").toLowerCase();
      let found = 0;
      grid.replaceChildren();
      function appendAppCard(name, iconText, clickHandler, iconData, iconMimetype) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "app-card o_app has-icon";
        button.innerHTML = '<span class="app-icon o_app_icon"></span><strong class="o_app_name"></strong>';
        const icon = button.querySelector(".o_app_icon");
        if (typeof iconData === "string" && iconData && !iconData.startsWith("/web/static/img/default_icon_app")) {
          const img = document.createElement("img");
          img.alt = "";
          img.src = iconData.startsWith("/") ? iconData : "data:" + (iconMimetype || "image/png") + ";base64," + iconData;
          icon.append(img);
        } else {
          icon.textContent = iconText || name.trim().slice(0, 1).toUpperCase() || "A";
        }
        button.querySelector("strong").textContent = name;
        button.addEventListener("click", clickHandler);
        grid.append(button);
        found++;
      }
      for (const id of menuRootIDs(payload)) {
        const menu = menuEntry(id);
        if (!menu) continue;
        const name = menu.name || "App";
        if (needle && !name.toLowerCase().includes(needle)) continue;
        appendAppCard(name, "", () => openMenu(menu.id), menu.webIconData, menu.webIconDataMimetype);
      }
      if (!needle || "apps".includes(needle)) {
        appendAppCard("Apps", "A", () => {
          setView("install");
          loadInstallApps();
        }, "", "");
      }
      if (!found) {
        const empty = document.createElement("p");
        empty.className = "muted";
        empty.textContent = "No menus loaded.";
        grid.append(empty);
      }
      document.getElementById("menuStatus").textContent = found ? "" : "No menus loaded.";
    }

    async function openMenu(menuID) {
      const menu = menuEntry(menuID);
      if (!menu) return;
      document.getElementById("menuStatus").textContent = menu.name;
      renderSidebarMenu(menu);
      const list = document.getElementById("menuList");
      list.replaceChildren();
      for (const childID of menu.children || []) {
        const child = menuEntry(childID);
        if (!child) continue;
        const button = document.createElement("button");
        button.type = "button";
        button.className = child.actionID ? "" : "secondary";
        button.textContent = child.name || "Menu";
        button.addEventListener("click", () => openMenu(child.id));
        list.append(button);
      }
      if (menu.actionID) {
        await openAction(menu.actionID);
      }
    }

    async function openAction(actionID) {
      try {
        const action = await requestJSON("/web/action/load?id=" + encodeURIComponent(actionID));
        const model = action.res_model || "";
        document.getElementById("menuStatus").textContent = (action.name || "Action") + (model ? " / " + model : "");
        if (model) {
          document.querySelector("#recordsView .o-control-panel h2").textContent = action.name || model;
          workbench.action = action;
          workbench.openedRecord = null;
          document.getElementById("recordSearch").value = "";
          ensureModelOption(model);
          modelSelect.value = model;
          if (action.limit) document.getElementById("limit").value = String(action.limit);
          showRecordForm(false);
          await loadActionViews(action, model);
          await loadRows();
          setView("records");
        }
      } catch (error) {
        if (await ensureSession(error)) return openAction(actionID);
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
        for (const row of rows) {
          const name = String(row.name || "");
          if (needle && !name.toLowerCase().includes(needle)) continue;
          const card = document.createElement("div");
          card.className = "module-card";
          const title = document.createElement("strong");
          title.textContent = name || "module";
          const state = document.createElement("span");
          state.className = "badge";
          state.textContent = row.state || "unknown";
          const button = document.createElement("button");
          button.type = "button";
          button.textContent = row.state === "installed" ? "Installed" : "Install";
          button.disabled = row.state === "installed";
          button.className = row.state === "installed" ? "secondary" : "";
          button.addEventListener("click", () => installModule(row.id));
          card.append(title, state, button);
          grid.append(card);
        }
        if (!grid.children.length) {
          const empty = document.createElement("p");
          empty.className = "muted";
          empty.textContent = "No apps found.";
          grid.append(empty);
        }
      } catch (error) {
        if (await ensureSession(error)) return loadInstallApps();
        grid.textContent = "Apps error: " + error.message;
      }
    }

    async function installModule(id) {
      await callKW("ir.module.module", "write", {args: [[id], {state: "installed"}]});
      await loadInstallApps();
    }

    function renderRows(rows, fields) {
      const host = document.getElementById("rows");
      host.replaceChildren();
      if (!Array.isArray(rows) || rows.length === 0) {
        const empty = document.createElement("p");
        empty.className = "muted";
        empty.textContent = "No rows.";
        host.append(empty);
        return;
      }
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
        tr.className = "o_data_row";
        tr.dataset.id = row.id || "";
        if (row.id) {
          tr.addEventListener("click", () => openRecord(modelSelect.value, row.id));
        }
        for (const field of fields) {
          if (field === "id") continue;
          const td = document.createElement("td");
          const value = row[field];
          td.textContent = value === null || value === undefined ? "" : (typeof value === "object" ? JSON.stringify(value) : String(value));
          tr.append(td);
        }
        tbody.append(tr);
      }
      table.append(thead, tbody);
      host.append(table);
    }

    async function openRecord(model, id) {
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
          const input = document.createElement("input");
          input.dataset.field = field;
          const value = row[field];
          input.value = value === null || value === undefined ? "" : (typeof value === "object" ? JSON.stringify(value) : String(value));
          if (field === "id" || Array.isArray(value) || typeof value === "object") input.readOnly = true;
          label.append(input);
          form.append(label);
        }
        document.getElementById("recordRaw").textContent = pretty(row);
      } catch (error) {
        if (await ensureSession(error)) return openRecord(model, id);
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
      setText("topUser", session.name || session.username || ("uid " + session.uid));
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

    buildWorkbenchPanels();
    setView("apps");
    document.getElementById("loadRows").addEventListener("click", loadRows);
    document.getElementById("createPartner").addEventListener("click", createPartner);
    document.getElementById("recordSearch").addEventListener("keydown", (event) => {
      if (event.key === "Enter") searchRows(event.currentTarget.value);
    });
    document.getElementById("loginButton").addEventListener("click", login);
    loadRuntime().then(loadRows).catch((error) => {
      runtimeStatus.textContent = "Startup error: " + error.message;
      runtimeStatus.className = "status-error";
    });
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

func (s Server) sessionModules(w http.ResponseWriter, r *http.Request) {
	if s.Security != nil {
		if _, ok := s.requireWebSession(w, r, nil); !ok {
			return
		}
	}
	s.writeMaybeRPC(w, r, modulesPayload(s.Modules))
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
	auditStart := s.loginAsAuditLen()
	_, err := s.Impersonation.SwitchToSystem(sessionID, actorID, impersonation.SwitchOptions{
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
	return s.impersonatedEnv(s.Env, s.impersonationSessionID(r))
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
	return s.impersonatedEnv(s.envForSecurityUser(user), sessionID), true
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
	return security.User{}, false
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

func (s Server) executeCallKW(env *record.Env, req callKWRequest) (any, error) {
	if result, handled, err := s.dispatchResPartnerMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchResGroupsMethod(env, req); handled {
		return result, err
	}
	if result, handled, err := s.dispatchSequenceMethod(env, req); handled {
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
		UserGroupIDs: groupIDsFromContext(runEnv.Context()),
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
	if _, exists := actionPayload["views"]; exists {
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
		return fmt.Errorf("non-db action dictionaries should provide either multiple view modes or a single view mode and an optional view id: got view modes %v and view id %d", modes, viewID)
	}
	if len(modes) == 1 {
		views = append(views, []any{falseIfZero(viewID), modes[0]})
		actionPayload["views"] = views
	}
	return nil
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
	userGroups := map[int64]bool{}
	for _, groupID := range groupIDsFromContext(ctx) {
		userGroups[groupID] = true
	}
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
		payload["filters"] = []any{}
	}
	return payload, nil
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
			"allowed_companies":             sessionCompanyPayload(env, companyIDs),
			"disallowed_ancestor_companies": sessionDisallowedAncestorCompanyPayload(env, companyIDs),
		}
		payload["show_effect"] = true
		payload["display_switch_company_menu"] = len(companyIDs) > 1
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
	ids := append([]int64(nil), ctx.CompanyIDs...)
	if len(ids) == 0 && ctx.CompanyID != 0 {
		ids = append(ids, ctx.CompanyID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
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
	if len(modules) == 0 {
		return map[string]any{"installed_modules": []string{}, "modules": map[string]any{}}
	}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	items := map[string]any{}
	for _, name := range names {
		manifest := modules[name]
		items[name] = map[string]any{
			"name":           manifest.Name,
			"technical_name": manifest.TechnicalName,
			"version":        manifest.Version,
			"category":       manifest.Category,
			"state":          "installed",
			"application":    manifest.Application,
			"auto_install":   manifest.AutoInstall,
			"installable":    manifest.Installable,
			"depends":        append([]string(nil), manifest.Depends...),
		}
	}
	return map[string]any{"installed_modules": names, "modules": items}
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
