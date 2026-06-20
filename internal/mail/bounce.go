package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/record"
)

var (
	messageIDPattern       = regexp.MustCompile(`<[^<>\s]+>`)
	emailAddressPattern    = regexp.MustCompile(`[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+`)
	htmlTagPattern         = regexp.MustCompile(`<[^>]+>`)
	repeatedBlankLineRegex = regexp.MustCompile(`\n{3,}`)
)

var inboundMessageIDLocks = struct {
	sync.Mutex
	active map[string]struct{}
}{active: map[string]struct{}{}}

var inboundAfterMessageIDLockHook func(string)

var inboundFallbackMessageIDCounter atomic.Int64

type InboundProcessResult struct {
	IsBounce        bool
	Duplicate       bool
	LoopDetected    bool
	Routed          bool
	MessageID       int64
	RFCMessageID    string
	Model           string
	ResID           int64
	ParentID        int64
	AuthorID        int64
	NotificationIDs []int64
	BouncedEmail    string
	BouncedPartners []int64
	FailureReason   string
}

type InboundProcessOptions struct {
	FallbackModel    string
	ThreadID         int64
	CustomValues     map[string]any
	SaveOriginal     bool
	StripAttachments bool
	Now              time.Time
	MessageIDLocker  InboundMessageIDLocker
}

type InboundMessageIDLocker interface {
	TryLockInboundMessageID(messageID string) (func(), bool, error)
}

type InboundMessageIDLockFunc func(string) (func(), bool, error)

func (f InboundMessageIDLockFunc) TryLockInboundMessageID(messageID string) (func(), bool, error) {
	return f(messageID)
}

type inboundBounceParts struct {
	DeliveryStatus  string
	OriginalHeaders string
	FirstText       string
	FirstHTML       string
	Attachments     []inboundAttachment
}

type inboundAttachment struct {
	Name     string
	Data     []byte
	Mimetype string
	CID      string
}

type inboundRoute struct {
	TargetModel         string
	TargetResID         int64
	ParentID            int64
	ParentInternal      bool
	CreateNew           bool
	UserID              int64
	CustomValues        map[string]any
	CatchallBounce      bool
	CatchallDomainID    int64
	CatchallEmail       string
	AliasMatched        bool
	AliasDenied         bool
	AliasDeniedConfig   bool
	AliasID             int64
	AliasName           string
	AliasDomainID       int64
	AliasContact        string
	AliasBouncedContent string
	AliasParentModel    string
	AliasParentThreadID int64
}

type inboundAliasRoute struct {
	Matched        bool
	Denied         bool
	DeniedConfig   bool
	AliasID        int64
	AliasName      string
	AliasDomainID  int64
	ModelName      string
	ForceThreadID  int64
	UserID         int64
	Defaults       map[string]any
	Contact        string
	BouncedContent string
	ParentModel    string
	ParentThreadID int64
}

type inboundCatchallRoute struct {
	Matched       bool
	AliasDomainID int64
	Email         string
}

func ProcessInboundEmail(env *record.Env, raw []byte) (InboundProcessResult, error) {
	return ProcessInboundEmailWithOptions(env, raw, InboundProcessOptions{})
}

func ProcessInboundEmailWithOptions(env *record.Env, raw []byte, options InboundProcessOptions) (InboundProcessResult, error) {
	if env == nil {
		return InboundProcessResult{}, fmt.Errorf("mail inbound processing requires env")
	}
	message, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return InboundProcessResult{}, err
	}
	body, err := io.ReadAll(message.Body)
	if err != nil {
		return InboundProcessResult{}, err
	}
	parts := inboundBounceParts{}
	collectInboundMIMEParts(message.Header.Get("Content-Type"), body, &parts)
	if options.StripAttachments {
		parts.Attachments = nil
	} else if options.SaveOriginal {
		parts.Attachments = append([]inboundAttachment{{
			Name:     "original_email.eml",
			Data:     append([]byte(nil), raw...),
			Mimetype: "message/rfc822",
		}}, parts.Attachments...)
	}
	rfcMessageID := inboundMessageID(message.Header, options.Now)
	if duplicateID, err := duplicateInboundMessageID(env, rfcMessageID); err != nil {
		return InboundProcessResult{}, err
	} else if duplicateID != 0 {
		return InboundProcessResult{Duplicate: true, MessageID: duplicateID, RFCMessageID: rfcMessageID}, nil
	}
	unlockMessageID, locked, err := tryLockInboundMessageID(env, rfcMessageID, options.MessageIDLocker)
	if err != nil {
		return InboundProcessResult{}, err
	}
	if !locked {
		return InboundProcessResult{Duplicate: true, RFCMessageID: rfcMessageID}, nil
	}
	defer unlockMessageID()
	if inboundAfterMessageIDLockHook != nil {
		inboundAfterMessageIDLockHook(rfcMessageID)
	}
	if duplicateID, err := duplicateInboundMessageID(env, rfcMessageID); err != nil {
		return InboundProcessResult{}, err
	} else if duplicateID != 0 {
		return InboundProcessResult{Duplicate: true, MessageID: duplicateID, RFCMessageID: rfcMessageID}, nil
	}
	if detectInboundLoopHeaders(message.Header) {
		return InboundProcessResult{LoopDetected: true, RFCMessageID: rfcMessageID}, nil
	}
	if !isInboundBounce(env, message.Header) {
		if from := firstEmailAddress(message.Header.Get("From")); from != "" {
			if err := resetPartnerBounceCounters(env, from); err != nil {
				return InboundProcessResult{}, err
			}
		}
		return routeInboundEmail(env, message.Header, parts, rfcMessageID, options)
	}

	bouncedEmail := finalRecipientEmail(parts.DeliveryStatus)
	references := originalMessageReferences(parts.OriginalHeaders)
	if len(references) == 0 {
		references = append(references, unfoldMessageIDs(message.Header.Get("In-Reply-To"))...)
		references = append(references, unfoldMessageIDs(message.Header.Get("References"))...)
	}
	messageID, err := findBouncedMessageID(env, references)
	if err != nil {
		return InboundProcessResult{}, err
	}
	partnerIDs, err := partnerIDsForEmail(env, bouncedEmail)
	if err != nil {
		return InboundProcessResult{}, err
	}
	if messageID != 0 && len(partnerIDs) == 0 {
		fallbackPartnerIDs, fallbackEmail, err := singleNotificationPartner(env, messageID)
		if err != nil {
			return InboundProcessResult{}, err
		}
		partnerIDs = fallbackPartnerIDs
		if bouncedEmail == "" {
			bouncedEmail = normalizedEmailAddress(fallbackEmail)
		}
	}
	reason := bounceFailureReason(parts)
	notificationIDs, err := markBounceNotifications(env, messageID, bouncedEmail, partnerIDs, reason)
	if err != nil {
		return InboundProcessResult{}, err
	}
	if err := markMailingTraceBounced(env, references, reason); err != nil {
		return InboundProcessResult{}, err
	}
	if bouncedEmail != "" {
		if err := incrementPartnerBounceCounters(env, bouncedEmail); err != nil {
			return InboundProcessResult{}, err
		}
		if err := autoBlacklistBouncedMailingEmail(env, bouncedEmail, partnerIDs, inboundSideEffectNow(options.Now)); err != nil {
			return InboundProcessResult{}, err
		}
	}
	return InboundProcessResult{
		IsBounce:        true,
		MessageID:       messageID,
		RFCMessageID:    rfcMessageID,
		NotificationIDs: notificationIDs,
		BouncedEmail:    bouncedEmail,
		BouncedPartners: partnerIDs,
		FailureReason:   reason,
	}, nil
}

func routeInboundEmail(env *record.Env, header netmail.Header, parts inboundBounceParts, rfcMessageID string, options InboundProcessOptions) (InboundProcessResult, error) {
	fromHeader := decodedHeader(header.Get("From"))
	authorIDs, err := partnerIDsForEmail(env, firstEmailAddress(fromHeader))
	if err != nil {
		return InboundProcessResult{}, err
	}
	authorID := int64(0)
	if len(authorIDs) != 0 {
		authorID = authorIDs[0]
	}
	routes, err := resolveInboundRoutes(env, header, options, authorID)
	if err != nil {
		return InboundProcessResult{}, err
	}
	if len(routes) != 0 {
		if err := markMailingTraceReplied(env, inboundReferenceMessageIDs(header), inboundSideEffectNow(options.Now)); err != nil {
			return InboundProcessResult{}, err
		}
	}
	result := InboundProcessResult{RFCMessageID: rfcMessageID, AuthorID: authorID}
	for _, route := range routes {
		result, err = processResolvedInboundRoute(env, header, parts, rfcMessageID, options, authorID, fromHeader, route)
		if err != nil {
			return InboundProcessResult{}, err
		}
	}
	return result, nil
}

func processResolvedInboundRoute(env *record.Env, header netmail.Header, parts inboundBounceParts, rfcMessageID string, options InboundProcessOptions, authorID int64, fromHeader string, route inboundRoute) (InboundProcessResult, error) {
	if route.CatchallBounce {
		if err := createInboundCatchallBounceMail(env, header, parts, rfcMessageID, route, options.Now); err != nil {
			return InboundProcessResult{}, err
		}
		return InboundProcessResult{RFCMessageID: rfcMessageID, AuthorID: authorID}, nil
	}
	if route.TargetModel == "" || route.TargetModel == "mail.thread" {
		return InboundProcessResult{RFCMessageID: rfcMessageID, AuthorID: authorID}, nil
	}
	if route.TargetResID != 0 && inboundRouteTargetMissing(env, route.TargetModel, route.TargetResID) {
		route.TargetResID = 0
		route.ParentID = 0
		route.ParentInternal = false
		route.CreateNew = strings.TrimSpace(route.TargetModel) != "" && strings.TrimSpace(route.TargetModel) != "mail.thread"
	}
	if route.AliasDenied || !inboundAliasContactAllowed(env, route, authorID) {
		if err := createInboundAliasBounceMail(env, header, parts, rfcMessageID, route); err != nil {
			return InboundProcessResult{}, err
		}
		return InboundProcessResult{
			RFCMessageID: rfcMessageID,
			Model:        route.TargetModel,
			ResID:        route.TargetResID,
			AuthorID:     authorID,
		}, nil
	}
	loop, err := detectInboundSenderLoop(env, header, route, options.Now)
	if err != nil {
		return InboundProcessResult{}, err
	}
	if loop {
		if err := createInboundLoopBounceMail(env, header, rfcMessageID, options.Now); err != nil {
			return InboundProcessResult{}, err
		}
		return InboundProcessResult{
			LoopDetected: true,
			RFCMessageID: rfcMessageID,
			Model:        route.TargetModel,
			ResID:        route.TargetResID,
			ParentID:     route.ParentID,
		}, nil
	}
	routeEnv := inboundRouteEnv(env, route.UserID)
	messageEnv := messageSystemEnv(env)
	incomingEmailTo, incomingEmailCC := inboundFilteredEmailHeaders(env, header)
	messageData := inboundMessageData(header, parts, rfcMessageID, authorID, route.ParentID, incomingEmailTo, incomingEmailCC, options.Now)
	if route.CreateNew {
		var err error
		route.TargetResID, err = createInboundThreadRecord(routeEnv, route.TargetModel, messageData, route.CustomValues)
		if err != nil {
			if route.AliasMatched {
				_ = markInboundAliasStatus(env, route.AliasID, "invalid")
				if bounceErr := createInboundAliasBounceMail(env, header, parts, rfcMessageID, inboundRoute{
					AliasMatched:        true,
					AliasDenied:         true,
					AliasDeniedConfig:   true,
					AliasID:             route.AliasID,
					AliasName:           route.AliasName,
					AliasDomainID:       route.AliasDomainID,
					AliasContact:        route.AliasContact,
					AliasBouncedContent: route.AliasBouncedContent,
					TargetModel:         route.TargetModel,
					TargetResID:         route.TargetResID,
					AliasParentModel:    route.AliasParentModel,
					AliasParentThreadID: route.AliasParentThreadID,
				}); bounceErr != nil {
					return InboundProcessResult{}, bounceErr
				}
			}
			return InboundProcessResult{}, err
		}
		if route.AliasMatched {
			if err := markInboundAliasStatus(env, route.AliasID, "valid"); err != nil {
				return InboundProcessResult{}, err
			}
		}
	} else if route.TargetResID != 0 {
		if err := updateInboundThreadRecord(routeEnv, route.TargetModel, route.TargetResID, messageData, nil); err != nil {
			return InboundProcessResult{}, err
		}
	}
	if route.TargetResID == 0 {
		return InboundProcessResult{RFCMessageID: rfcMessageID, Model: route.TargetModel}, nil
	}
	partnerIDs, err := partnerIDsForEmails(env, inboundRecipientEmails(header))
	if err != nil {
		return InboundProcessResult{}, err
	}
	bodyHTML := messageData.BodyHTML
	attachmentIDs, bodyHTML, err := createInboundAttachments(messageEnv, route.TargetModel, route.TargetResID, parts.Attachments, bodyHTML)
	if err != nil {
		return InboundProcessResult{}, err
	}
	subtype := "mail.mt_comment"
	if route.ParentInternal {
		subtype = "mail.mt_note"
	}
	postID, err := PostMessage(messageEnv, PostRequest{
		Model:           route.TargetModel,
		ResID:           route.TargetResID,
		Body:            bodyHTML,
		Subject:         decodedHeader(header.Get("Subject")),
		MessageType:     "email",
		EmailFrom:       fromHeader,
		MessageID:       rfcMessageID,
		IncomingEmailTo: incomingEmailTo,
		IncomingEmailCC: incomingEmailCC,
		AuthorID:        authorID,
		ParentID:        route.ParentID,
		SubtypeXMLID:    subtype,
		PartnerIDs:      partnerIDs,
		AttachmentIDs:   attachmentIDs,
		BodyIsHTML:      true,
		Now:             inboundDate(header, options.Now),
	})
	if err != nil {
		return InboundProcessResult{}, err
	}
	return InboundProcessResult{
		Routed:       true,
		MessageID:    postID,
		RFCMessageID: rfcMessageID,
		Model:        route.TargetModel,
		ResID:        route.TargetResID,
		ParentID:     route.ParentID,
		AuthorID:     authorID,
	}, nil
}

func inboundRouteTargetMissing(env *record.Env, modelName string, resID int64) bool {
	if env == nil || strings.TrimSpace(modelName) == "" || resID == 0 {
		return false
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata(modelName); !ok {
		return false
	}
	return ensureRecordExists(systemEnv, modelName, resID) != nil
}

func inboundRouteEnv(env *record.Env, userID int64) *record.Env {
	if env == nil || userID == 0 || env.Context().UserID == userID {
		return env
	}
	ctx := env.Context()
	ctx.UserID = userID
	return env.WithContext(ctx)
}

func inboundGatewayUserID(env *record.Env, authorID int64) int64 {
	if env == nil {
		return 0
	}
	if userID := inboundUserIDForPartner(env, authorID); userID != 0 {
		return userID
	}
	return env.Context().UserID
}

func inboundUserIDForPartner(env *record.Env, partnerID int64) int64 {
	if env == nil || partnerID == 0 {
		return 0
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("res.users"); !ok {
		return 0
	}
	found, err := systemEnv.Model("res.users").SearchWithOptions(domain.Cond("partner_id", "=", partnerID), record.SearchOptions{Order: "id"})
	if err != nil {
		return 0
	}
	rows, err := found.Read("id", "share", "active")
	if err != nil {
		return 0
	}
	firstActive := int64(0)
	for _, row := range rows {
		if active, exists := row["active"]; exists && active != nil && !boolAny(active) {
			continue
		}
		id := int64FromAny(row["id"])
		if id == 0 {
			continue
		}
		if firstActive == 0 {
			firstActive = id
		}
		if share, exists := row["share"]; !exists || share == nil || !boolAny(share) {
			return id
		}
	}
	return firstActive
}

func resolveInboundRoutes(env *record.Env, header netmail.Header, options InboundProcessOptions, authorID int64) ([]inboundRoute, error) {
	parentID, parentModel, parentResID, _, err := parentMessageRoute(env, header)
	if err != nil {
		return nil, err
	}
	recipients := inboundRecipientEmails(header)
	if parentModel != "" && parentResID != 0 {
		toRecipients := inboundRecipientEmailsForHeaders(header, "Delivered-To", "To")
		if routes := inboundAliasRoutesForRecipientsExceptModel(env, toRecipients, parentModel, authorID); len(routes) > 0 {
			return routes, nil
		}
	} else if routes := inboundAliasRoutesForRecipients(env, recipients, authorID); len(routes) > 0 {
		return routes, nil
	}
	route, err := resolveInboundRoute(env, header, options, authorID)
	if err != nil {
		return nil, err
	}
	if parentID != 0 && route.ParentID == parentID {
		return []inboundRoute{route}, nil
	}
	return []inboundRoute{route}, nil
}

func resolveInboundRoute(env *record.Env, header netmail.Header, options InboundProcessOptions, authorID int64) (inboundRoute, error) {
	parentID, parentModel, parentResID, parentInternal, err := parentMessageRoute(env, header)
	if err != nil {
		return inboundRoute{}, err
	}
	targetModel := parentModel
	targetResID := parentResID
	customValues := options.CustomValues
	aliasMatched := false
	aliasDenied := false
	aliasDeniedConfig := false
	aliasID := int64(0)
	aliasName := ""
	aliasDomainID := int64(0)
	aliasContact := ""
	aliasBouncedContent := ""
	aliasParentModel := ""
	aliasParentThreadID := int64(0)
	routeUserID := inboundGatewayUserID(env, authorID)
	recipients := inboundRecipientEmails(header)
	applyAliasRoute := func(aliasRoute inboundAliasRoute, useAliasTarget bool) {
		if useAliasTarget {
			targetModel = aliasRoute.ModelName
			targetResID = aliasRoute.ForceThreadID
			customValues = aliasRoute.Defaults
		}
		aliasMatched = true
		aliasDenied = aliasRoute.Denied
		aliasDeniedConfig = aliasRoute.DeniedConfig
		aliasID = aliasRoute.AliasID
		aliasName = aliasRoute.AliasName
		aliasDomainID = aliasRoute.AliasDomainID
		aliasContact = aliasRoute.Contact
		aliasBouncedContent = aliasRoute.BouncedContent
		aliasParentModel = aliasRoute.ParentModel
		aliasParentThreadID = aliasRoute.ParentThreadID
		if aliasRoute.UserID != 0 {
			routeUserID = aliasRoute.UserID
		}
	}
	if targetModel != "" && targetResID != 0 {
		toRecipients := inboundRecipientEmailsForHeaders(header, "Delivered-To", "To")
		if aliasRoute := inboundAliasRouteForRecipientsExceptModel(env, toRecipients, targetModel, authorID); aliasRoute.Matched {
			applyAliasRoute(aliasRoute, true)
			parentID = 0
			parentInternal = false
		} else if aliasRoute := inboundAliasRouteForRecipientsMatchingModel(env, recipients, targetModel, authorID); aliasRoute.Matched {
			applyAliasRoute(aliasRoute, false)
		}
	}
	if targetModel == "" || targetResID == 0 {
		if aliasRoute := inboundAliasRouteForRecipients(env, recipients, authorID); aliasRoute.Matched {
			applyAliasRoute(aliasRoute, true)
		} else if catchallRoute := inboundCatchallRouteForRecipients(env, recipients, false); catchallRoute.Matched {
			return inboundRoute{
				CatchallBounce:   true,
				CatchallDomainID: catchallRoute.AliasDomainID,
				CatchallEmail:    catchallRoute.Email,
			}, nil
		} else {
			targetModel = strings.TrimSpace(options.FallbackModel)
			targetResID = options.ThreadID
			if targetModel == "" {
				if catchallRoute := inboundCatchallRouteForRecipients(env, recipients, true); catchallRoute.Matched {
					return inboundRoute{
						CatchallBounce:   true,
						CatchallDomainID: catchallRoute.AliasDomainID,
						CatchallEmail:    catchallRoute.Email,
					}, nil
				}
			}
		}
		parentID = 0
		parentInternal = false
	}
	return inboundRoute{
		TargetModel:         strings.TrimSpace(targetModel),
		TargetResID:         targetResID,
		ParentID:            parentID,
		ParentInternal:      parentInternal,
		CreateNew:           strings.TrimSpace(targetModel) != "" && strings.TrimSpace(targetModel) != "mail.thread" && targetResID == 0,
		UserID:              routeUserID,
		CustomValues:        customValues,
		CatchallBounce:      false,
		AliasMatched:        aliasMatched,
		AliasDenied:         aliasDenied,
		AliasDeniedConfig:   aliasDeniedConfig,
		AliasID:             aliasID,
		AliasName:           aliasName,
		AliasDomainID:       aliasDomainID,
		AliasContact:        aliasContact,
		AliasBouncedContent: aliasBouncedContent,
		AliasParentModel:    aliasParentModel,
		AliasParentThreadID: aliasParentThreadID,
	}, nil
}

func inboundMessageID(header netmail.Header, now time.Time) string {
	if value := strings.TrimSpace(decodedHeader(header.Get("Message-Id"))); value != "" {
		return value
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return fmt.Sprintf("<%d.%d@localhost>", now.UnixNano(), inboundFallbackMessageIDCounter.Add(1))
}

func duplicateInboundMessageID(env *record.Env, rfcMessageID string) (int64, error) {
	rfcMessageID = strings.TrimSpace(rfcMessageID)
	if env == nil || rfcMessageID == "" {
		return 0, nil
	}
	found, err := messageSystemEnv(env).Model("mail.message").SearchWithOptions(
		domain.Cond("message_id", "=", rfcMessageID),
		record.SearchOptions{Order: "id desc", Limit: 1},
	)
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64FromAny(rows[0]["id"]), nil
}

func tryLockInboundMessageID(env *record.Env, rfcMessageID string, locker InboundMessageIDLocker) (func(), bool, error) {
	rfcMessageID = strings.TrimSpace(rfcMessageID)
	if env == nil || rfcMessageID == "" {
		return func() {}, true, nil
	}
	if locker != nil {
		unlockExternal, locked, err := locker.TryLockInboundMessageID(rfcMessageID)
		if err != nil || !locked {
			return func() {}, locked, err
		}
		unlockMemory, memoryLocked := tryLockInboundMessageIDMemory(env, rfcMessageID)
		if !memoryLocked {
			unlockExternal()
			return func() {}, false, nil
		}
		return func() {
			unlockMemory()
			unlockExternal()
		}, true, nil
	}
	key := env.SequenceNamespace("") + ":" + rfcMessageID
	inboundMessageIDLocks.Lock()
	defer inboundMessageIDLocks.Unlock()
	unlockRow, rowLocked, err := tryLockInboundMessageIDRow(env, rfcMessageID)
	if err != nil || !rowLocked {
		return func() {}, rowLocked, err
	}
	if _, exists := inboundMessageIDLocks.active[key]; exists {
		unlockRow()
		return func() {}, false, nil
	}
	inboundMessageIDLocks.active[key] = struct{}{}
	return func() {
		inboundMessageIDLocks.Lock()
		delete(inboundMessageIDLocks.active, key)
		inboundMessageIDLocks.Unlock()
		unlockRow()
	}, true, nil
}

func tryLockInboundMessageIDMemory(env *record.Env, rfcMessageID string) (func(), bool) {
	rfcMessageID = strings.TrimSpace(rfcMessageID)
	if env == nil || rfcMessageID == "" {
		return func() {}, true
	}
	key := env.SequenceNamespace("") + ":" + rfcMessageID
	inboundMessageIDLocks.Lock()
	defer inboundMessageIDLocks.Unlock()
	if _, exists := inboundMessageIDLocks.active[key]; exists {
		return func() {}, false
	}
	inboundMessageIDLocks.active[key] = struct{}{}
	return func() {
		inboundMessageIDLocks.Lock()
		delete(inboundMessageIDLocks.active, key)
		inboundMessageIDLocks.Unlock()
	}, true
}

func tryLockInboundMessageIDRow(env *record.Env, rfcMessageID string) (func(), bool, error) {
	rfcMessageID = strings.TrimSpace(rfcMessageID)
	if env == nil || rfcMessageID == "" {
		return func() {}, true, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.inbound.message.lock"); !ok {
		return func() {}, true, nil
	}
	lockModel := systemEnv.Model("mail.inbound.message.lock")
	found, err := lockModel.Search(domain.Cond("message_id", "=", rfcMessageID))
	if err != nil {
		return func() {}, false, err
	}
	if found.Len() != 0 {
		return func() {}, false, nil
	}
	lockID, err := lockModel.Create(map[string]any{"message_id": rfcMessageID})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate message_id") {
			return func() {}, false, nil
		}
		return func() {}, false, err
	}
	return func() {
		_ = lockModel.Browse(lockID).Unlink()
	}, true, nil
}

func detectInboundLoopHeaders(header netmail.Header) bool {
	for _, name := range []string{"References", "In-Reply-To"} {
		for _, value := range headerValues(header, name) {
			if strings.Contains(decodedHeader(value), "-loop-detection-bounce-email@") {
				return true
			}
		}
	}
	return false
}

func detectInboundSenderLoop(env *record.Env, header netmail.Header, route inboundRoute, now time.Time) (bool, error) {
	if env == nil || route.TargetModel == "" || route.TargetModel == "mail.thread" {
		return false, nil
	}
	fromHeader := decodedHeader(header.Get("From"))
	emailFrom := normalizedEmailAddress(firstNonEmpty(firstEmailAddress(fromHeader), fromHeader))
	if emailFrom == "" {
		return false, nil
	}
	allowed, err := inboundGatewaySenderAllowed(env, emailFrom)
	if err != nil || allowed {
		return false, err
	}
	threshold := inboundGatewayConfigInt(env, "mail.gateway.loop.threshold", 20)
	if threshold <= 0 {
		return true, nil
	}
	minutes := inboundGatewayConfigInt(env, "mail.gateway.loop.minutes", 120)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	since := now.Add(-time.Duration(minutes) * time.Minute)
	if route.CreateNew {
		count, err := inboundLoopNewRecordCount(env, route.TargetModel, emailFrom, since, threshold)
		if err != nil || count >= threshold {
			return count >= threshold, err
		}
	}
	if route.TargetResID != 0 {
		count, err := inboundLoopUpdateMessageCount(env, route.TargetModel, route.TargetResID, emailFrom, since, threshold)
		if err != nil || count >= threshold {
			return count >= threshold, err
		}
	}
	return false, nil
}

func inboundGatewaySenderAllowed(env *record.Env, email string) (bool, error) {
	email = normalizedEmailAddress(email)
	if env == nil || email == "" {
		return false, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.gateway.allowed"); !ok {
		return false, nil
	}
	found, err := systemEnv.Model("mail.gateway.allowed").Search(domain.And())
	if err != nil {
		return false, err
	}
	rows, err := found.Read("email", "email_normalized")
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if normalizedEmailAddress(firstNonEmpty(stringAny(row["email_normalized"]), stringAny(row["email"]))) == email {
			return true, nil
		}
	}
	return false, nil
}

func inboundGatewayConfigInt(env *record.Env, key string, fallback int) int {
	value := strings.TrimSpace(configParameter(messageSystemEnv(env), key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func inboundLoopNewRecordCount(env *record.Env, modelName string, email string, since time.Time, limit int) (int, error) {
	if env == nil || modelName == "" || email == "" {
		return 0, nil
	}
	systemEnv := messageSystemEnv(env)
	meta, ok := systemEnv.ModelMetadata(modelName)
	if !ok {
		return 0, nil
	}
	emailField := inboundLoopEmailField(meta.Fields)
	if emailField == "" {
		return 0, nil
	}
	fields := []string{emailField}
	hasCreateDate := false
	if _, ok := meta.Fields["create_date"]; ok {
		fields = append(fields, "create_date")
		hasCreateDate = true
	}
	found, err := systemEnv.Model(modelName).Search(domain.And())
	if err != nil {
		return 0, err
	}
	rows, err := found.Read(fields...)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		if !inboundLoopRowEmailMatches(row, emailField, email) {
			continue
		}
		if hasCreateDate && !inboundLoopRowIsRecent(row, since) {
			continue
		}
		count++
		if limit > 0 && count >= limit {
			return count, nil
		}
	}
	return count, nil
}

func inboundLoopUpdateMessageCount(env *record.Env, modelName string, resID int64, email string, since time.Time, limit int) (int, error) {
	if env == nil || modelName == "" || resID == 0 || email == "" {
		return 0, nil
	}
	systemEnv := messageSystemEnv(env)
	authorIDs, err := partnerIDsForEmail(systemEnv, email)
	if err != nil {
		return 0, err
	}
	node := domain.And(
		domain.Cond("model", domain.Equal, modelName),
		domain.Cond("res_id", domain.Equal, resID),
		domain.Cond("message_type", domain.Equal, "email"),
	)
	found, err := systemEnv.Model("mail.message").Search(node)
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("author_id", "email_from", "create_date")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		if !inboundLoopRowIsRecent(row, since) {
			continue
		}
		if len(authorIDs) != 0 {
			if !containsInt64(authorIDs, int64FromAny(row["author_id"])) {
				continue
			}
		} else if normalizedEmailAddress(stringAny(row["email_from"])) != email {
			continue
		}
		count++
		if limit > 0 && count >= limit {
			return count, nil
		}
	}
	return count, nil
}

func inboundLoopEmailField(fields map[string]field.Field) string {
	for _, name := range []string{"email_normalized", "email", "email_from"} {
		if _, ok := fields[name]; ok {
			return name
		}
	}
	return ""
}

func inboundLoopRowEmailMatches(row map[string]any, fieldName string, email string) bool {
	value := stringAny(row[fieldName])
	if fieldName == "email_normalized" {
		return normalizedEmailAddress(value) == email
	}
	return normalizedEmailAddress(value) == email || strings.Contains(strings.ToLower(value), strings.ToLower(email))
}

func inboundLoopRowIsRecent(row map[string]any, since time.Time) bool {
	if since.IsZero() {
		return true
	}
	value := timeValue(row["create_date"])
	return !value.IsZero() && !value.Before(since)
}

func createInboundLoopBounceMail(env *record.Env, header netmail.Header, rfcMessageID string, now time.Time) error {
	if env == nil {
		return nil
	}
	recipient := firstNonEmpty(
		firstEmailAddress(decodedHeader(header.Get("Return-Path"))),
		decodedHeader(header.Get("Return-Path")),
		decodedHeader(header.Get("From")),
	)
	if recipient == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	bounceEmail := defaultBounceEmail(env)
	emailFrom := "MAILER-DAEMON <mailer-daemon@localhost>"
	if bounceEmail != "" {
		emailFrom = (&netmail.Address{Name: "MAILER-DAEMON", Address: bounceEmail}).String()
	}
	_, err := messageSystemEnv(env).Model("mail.mail").Create(map[string]any{
		"email_from":      emailFrom,
		"email_to":        recipient,
		"subject":         "Mail delivery failed: loop detected",
		"body_html":       "<p>Too many emails received in a short time. The incoming message was ignored.</p>",
		"state":           "outgoing",
		"auto_delete":     true,
		"message_id":      inboundLoopBounceMessageID(now),
		"references":      strings.TrimSpace(rfcMessageID + " " + inboundLoopBounceMessageID(now)),
		"is_notification": true,
	})
	return err
}

func createInboundCatchallBounceMail(env *record.Env, header netmail.Header, parts inboundBounceParts, rfcMessageID string, route inboundRoute, now time.Time) error {
	if env == nil {
		return nil
	}
	recipient := firstNonEmpty(
		firstEmailAddress(decodedHeader(header.Get("Return-Path"))),
		decodedHeader(header.Get("Return-Path")),
		decodedHeader(header.Get("From")),
	)
	if strings.TrimSpace(recipient) == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	bounceMessageID := inboundLoopBounceMessageID(now)
	systemEnv := messageSystemEnv(env)
	values := map[string]any{
		"email_from":      inboundAliasBounceFrom(systemEnv, route.CatchallDomainID, header),
		"email_to":        recipient,
		"subject":         "Re: " + decodedHeader(header.Get("Subject")),
		"body_html":       inboundCatchallBounceBody(env, header, parts, route),
		"state":           "outgoing",
		"auto_delete":     true,
		"message_id":      bounceMessageID,
		"references":      strings.TrimSpace(rfcMessageID + " " + bounceMessageID),
		"is_notification": true,
	}
	if route.CatchallDomainID != 0 {
		values["record_alias_domain_id"] = route.CatchallDomainID
	}
	if replyTo := defaultNotificationEmail(systemEnv); replyTo != "" {
		values["reply_to"] = replyTo
	}
	_, err := systemEnv.Model("mail.mail").Create(values)
	return err
}

func inboundCatchallBounceBody(env *record.Env, header netmail.Header, parts inboundBounceParts, route inboundRoute) string {
	quoted := inboundBodyHTML(parts)
	if strings.TrimSpace(quoted) == "" {
		quoted = "<p></p>"
	}
	emailTo := decodedHeader(header.Get("To"))
	if strings.TrimSpace(emailTo) == "" {
		emailTo = route.CatchallEmail
	}
	companyName := inboundCompanyName(env)
	contactEmail := defaultNotificationEmail(messageSystemEnv(env))
	body := fmt.Sprintf(
		"<p>Hello %s,</p><p>The email sent to %s cannot be processed. This address is used to collect replies and should not be used to directly contact %s.</p>",
		html.EscapeString(firstNonEmpty(decodedHeader(header.Get("From")), "Sender")),
		html.EscapeString(emailTo),
		html.EscapeString(companyName),
	)
	if contactEmail != "" {
		body += fmt.Sprintf("<p>Please contact us instead using <a href=\"mailto:%s\">%s</a></p>", html.EscapeString(contactEmail), html.EscapeString(contactEmail))
	}
	body += fmt.Sprintf("<p>Regards,</p><p>The %s team.</p>", html.EscapeString(companyName))
	return "<div>" + body + "</div><blockquote>" + quoted + "</blockquote>"
}

func inboundLoopBounceMessageID(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return fmt.Sprintf("<%d-loop-detection-bounce-email@localhost>", now.UnixNano())
}

func parentMessageRoute(env *record.Env, header netmail.Header) (int64, string, int64, bool, error) {
	parentID, err := findParentMessageByReferences(env, unfoldMessageIDs(decodedHeader(header.Get("In-Reply-To"))))
	if err != nil {
		return 0, "", 0, false, err
	}
	if parentID != 0 {
		return parentRouteFromMessage(env, parentID)
	}
	references := unfoldMessageIDs(decodedHeader(header.Get("References")))
	if len(references) > 32 {
		references = references[len(references)-32:]
	}
	parentID, err = findParentMessageByReferences(env, references)
	if err != nil || parentID == 0 {
		return 0, "", 0, false, err
	}
	return parentRouteFromMessage(env, parentID)
}

func findParentMessageByReferences(env *record.Env, references []string) (int64, error) {
	references = uniqueStrings(references)
	if env == nil || len(references) == 0 {
		return 0, nil
	}
	found, err := messageSystemEnv(env).Model("mail.message").SearchWithOptions(
		domain.Cond("message_id", "in", references),
		record.SearchOptions{Order: "id desc", Limit: 1},
	)
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64FromAny(rows[0]["id"]), nil
}

func parentRouteFromMessage(env *record.Env, parentID int64) (int64, string, int64, bool, error) {
	if env == nil || parentID == 0 {
		return 0, "", 0, false, nil
	}
	rows, err := messageSystemEnv(env).Model("mail.message").Browse(parentID).Read("model", "res_id", "subtype_id")
	if err != nil || len(rows) == 0 {
		return 0, "", 0, false, err
	}
	return parentID, stringAny(rows[0]["model"]), int64FromAny(rows[0]["res_id"]), subtypeIsInternal(env, int64FromAny(rows[0]["subtype_id"])), nil
}

func subtypeIsInternal(env *record.Env, subtypeID int64) bool {
	if env == nil || subtypeID == 0 {
		return false
	}
	rows, err := messageSystemEnv(env).Model("mail.message.subtype").Browse(subtypeID).Read("internal")
	return err == nil && len(rows) != 0 && boolAny(rows[0]["internal"])
}

func createInboundThreadRecord(env *record.Env, modelName string, messageData InboundMessageData, customValues map[string]any) (int64, error) {
	modelName = strings.TrimSpace(modelName)
	if env == nil || modelName == "" {
		return 0, nil
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return 0, fmt.Errorf("unknown model %s", modelName)
	}
	customValues = mergeInboundCustomValues(mailingTraceInboundDefaults(env, meta.Fields, messageData.Header), customValues)
	if handler := inboundMessageHandler(modelName); handler.MessageNew != nil {
		id, err := handler.MessageNew(env, InboundMessageNewRequest{
			Model:        modelName,
			Message:      messageData,
			CustomValues: cloneInboundValues(customValues),
		})
		if err != nil {
			return 0, err
		}
		if id == 0 {
			return 0, fmt.Errorf("message_new for %s returned no record", modelName)
		}
		return id, nil
	}
	values := map[string]any{}
	for key, value := range customValues {
		values[key] = value
	}
	recName := meta.RecName
	if recName == "" {
		recName = "name"
	}
	if _, ok := meta.Fields[recName]; ok && strings.TrimSpace(stringAny(values[recName])) == "" {
		values[recName] = firstNonEmpty(messageData.Subject, "(no subject)")
	}
	if _, ok := meta.Fields["email"]; ok && strings.TrimSpace(stringAny(values["email"])) == "" {
		values["email"] = firstNonEmpty(firstEmailAddress(messageData.EmailFrom), messageData.EmailFrom)
	}
	if _, ok := meta.Fields["email_normalized"]; ok && strings.TrimSpace(stringAny(values["email_normalized"])) == "" {
		if email := firstEmailAddress(messageData.EmailFrom); email != "" {
			values["email_normalized"] = email
		}
	}
	if _, ok := meta.Fields["active"]; ok {
		if _, exists := values["active"]; !exists {
			values["active"] = true
		}
	}
	return env.Model(modelName).Create(values)
}

func updateInboundThreadRecord(env *record.Env, modelName string, resID int64, messageData InboundMessageData, updateValues map[string]any) error {
	modelName = strings.TrimSpace(modelName)
	if env == nil || modelName == "" || resID == 0 {
		return nil
	}
	handler := inboundMessageHandler(modelName)
	if handler.MessageUpdate == nil {
		return nil
	}
	return handler.MessageUpdate(env, InboundMessageUpdateRequest{
		Model:        modelName,
		ResID:        resID,
		Message:      messageData,
		UpdateValues: cloneInboundValues(updateValues),
	})
}

func inboundAliasRouteForRecipients(env *record.Env, recipients []string, authorID int64) inboundAliasRoute {
	return inboundAliasRouteForRecipientsFiltered(env, recipients, authorID, nil)
}

func inboundAliasRoutesForRecipients(env *record.Env, recipients []string, authorID int64) []inboundRoute {
	return inboundAliasRoutesForRecipientsFiltered(env, recipients, authorID, nil)
}

func inboundAliasRouteForRecipientsExceptModel(env *record.Env, recipients []string, excludedModel string, authorID int64) inboundAliasRoute {
	excludedModel = strings.TrimSpace(excludedModel)
	if excludedModel == "" {
		return inboundAliasRoute{}
	}
	return inboundAliasRouteForRecipientsFiltered(env, recipients, authorID, func(modelName string) bool {
		return strings.TrimSpace(modelName) != excludedModel
	})
}

func inboundAliasRoutesForRecipientsExceptModel(env *record.Env, recipients []string, excludedModel string, authorID int64) []inboundRoute {
	excludedModel = strings.TrimSpace(excludedModel)
	if excludedModel == "" {
		return nil
	}
	return inboundAliasRoutesForRecipientsFiltered(env, recipients, authorID, func(modelName string) bool {
		return strings.TrimSpace(modelName) != excludedModel
	})
}

func inboundAliasRouteForRecipientsMatchingModel(env *record.Env, recipients []string, modelName string, authorID int64) inboundAliasRoute {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return inboundAliasRoute{}
	}
	return inboundAliasRouteForRecipientsFiltered(env, recipients, authorID, func(candidate string) bool {
		return strings.TrimSpace(candidate) == modelName
	})
}

func inboundAliasRoutesForRecipientsFiltered(env *record.Env, recipients []string, authorID int64, acceptModel func(string) bool) []inboundRoute {
	if env == nil || len(recipients) == 0 {
		return nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.alias"); !ok {
		return nil
	}
	found, err := systemEnv.Model("mail.alias").Search(domain.And())
	if err != nil {
		return nil
	}
	rows, err := found.Read("alias_name", "alias_domain", "alias_domain_id", "alias_full_name", "model_name", "alias_model_id", "alias_force_thread_id", "alias_defaults", "alias_contact", "alias_bounced_content", "alias_parent_model_id", "alias_parent_thread_id", "alias_incoming_local", "active")
	if err != nil {
		return nil
	}
	recipientSet := map[string]bool{}
	localSet := map[string]bool{}
	allowedDomains, restrictLocalDomains := inboundAllowedLocalPartDomains(systemEnv)
	for _, recipient := range recipients {
		email := normalizedEmailAddress(recipient)
		if email == "" {
			continue
		}
		recipientSet[email] = true
		local, domainName, ok := strings.Cut(email, "@")
		if ok && local != "" && inboundLocalPartDomainAllowed(domainName, allowedDomains, restrictLocalDomains) {
			localSet[local] = true
		}
	}
	routes := []inboundRoute{}
	seenAliases := map[int64]bool{}
	for _, row := range rows {
		if value, exists := row["active"]; exists && value != nil && !boolAny(value) {
			continue
		}
		modelName := inboundAliasModelName(systemEnv, row)
		aliasName := strings.ToLower(strings.TrimSpace(stringAny(row["alias_name"])))
		if modelName == "" || aliasName == "" {
			continue
		}
		if acceptModel != nil && !acceptModel(modelName) {
			continue
		}
		matched := false
		if fullName := normalizedEmailAddress(stringAny(row["alias_full_name"])); fullName != "" && recipientSet[fullName] {
			matched = true
		}
		domainName := firstNonEmpty(stringAny(row["alias_domain"]), aliasDomainName(env, int64FromAny(row["alias_domain_id"])))
		if !matched && domainName != "" && recipientSet[normalizedEmailAddress(aliasName+"@"+domainName)] {
			matched = true
		}
		if !matched && domainName == "" && localSet[aliasName] {
			matched = true
		}
		if !matched && domainName != "" && boolAny(row["alias_incoming_local"]) && localSet[aliasName] {
			matched = true
		}
		if !matched {
			continue
		}
		route, allowed := matchedInboundAliasRouteFromRow(systemEnv, modelName, row, authorID)
		if !allowed || seenAliases[route.AliasID] {
			continue
		}
		seenAliases[route.AliasID] = true
		routes = append(routes, inboundRoute{
			TargetModel:         route.ModelName,
			TargetResID:         route.ForceThreadID,
			CreateNew:           strings.TrimSpace(route.ModelName) != "" && strings.TrimSpace(route.ModelName) != "mail.thread" && route.ForceThreadID == 0,
			UserID:              route.UserID,
			CustomValues:        route.Defaults,
			AliasMatched:        true,
			AliasID:             route.AliasID,
			AliasName:           route.AliasName,
			AliasDomainID:       route.AliasDomainID,
			AliasContact:        route.Contact,
			AliasBouncedContent: route.BouncedContent,
			AliasParentModel:    route.ParentModel,
			AliasParentThreadID: route.ParentThreadID,
		})
	}
	return routes
}

func inboundAliasRouteForRecipientsFiltered(env *record.Env, recipients []string, authorID int64, acceptModel func(string) bool) inboundAliasRoute {
	if env == nil || len(recipients) == 0 {
		return inboundAliasRoute{}
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.alias"); !ok {
		return inboundAliasRoute{}
	}
	found, err := systemEnv.Model("mail.alias").Search(domain.And())
	if err != nil {
		return inboundAliasRoute{}
	}
	rows, err := found.Read("alias_name", "alias_domain", "alias_domain_id", "alias_full_name", "model_name", "alias_model_id", "alias_force_thread_id", "alias_defaults", "alias_contact", "alias_bounced_content", "alias_parent_model_id", "alias_parent_thread_id", "alias_incoming_local", "active")
	if err != nil {
		return inboundAliasRoute{}
	}
	recipientSet := map[string]bool{}
	localSet := map[string]bool{}
	allowedDomains, restrictLocalDomains := inboundAllowedLocalPartDomains(systemEnv)
	for _, recipient := range recipients {
		email := normalizedEmailAddress(recipient)
		if email == "" {
			continue
		}
		recipientSet[email] = true
		local, domainName, ok := strings.Cut(email, "@")
		if ok && local != "" && inboundLocalPartDomainAllowed(domainName, allowedDomains, restrictLocalDomains) {
			localSet[local] = true
		}
	}
	deniedRoute := inboundAliasRoute{}
	for _, row := range rows {
		if value, exists := row["active"]; exists && value != nil && !boolAny(value) {
			continue
		}
		modelName := inboundAliasModelName(systemEnv, row)
		aliasName := strings.ToLower(strings.TrimSpace(stringAny(row["alias_name"])))
		if modelName == "" || aliasName == "" {
			continue
		}
		if acceptModel != nil && !acceptModel(modelName) {
			continue
		}
		if fullName := normalizedEmailAddress(stringAny(row["alias_full_name"])); fullName != "" && recipientSet[fullName] {
			if route, allowed := matchedInboundAliasRouteFromRow(systemEnv, modelName, row, authorID); allowed {
				return route
			} else if !deniedRoute.Matched {
				deniedRoute = route
			}
			continue
		}
		domainName := firstNonEmpty(stringAny(row["alias_domain"]), aliasDomainName(env, int64FromAny(row["alias_domain_id"])))
		if domainName != "" {
			if recipientSet[normalizedEmailAddress(aliasName+"@"+domainName)] {
				if route, allowed := matchedInboundAliasRouteFromRow(systemEnv, modelName, row, authorID); allowed {
					return route
				} else if !deniedRoute.Matched {
					deniedRoute = route
				}
				continue
			}
		} else if localSet[aliasName] {
			if route, allowed := matchedInboundAliasRouteFromRow(systemEnv, modelName, row, authorID); allowed {
				return route
			} else if !deniedRoute.Matched {
				deniedRoute = route
			}
			continue
		}
		if domainName != "" && boolAny(row["alias_incoming_local"]) && localSet[aliasName] {
			if route, allowed := matchedInboundAliasRouteFromRow(systemEnv, modelName, row, authorID); allowed {
				return route
			} else if !deniedRoute.Matched {
				deniedRoute = route
			}
			continue
		}
	}
	if deniedRoute.Matched {
		return deniedRoute
	}
	return inboundAliasRoute{}
}

func inboundCatchallRouteForRecipients(env *record.Env, recipients []string, anyMatch bool) inboundCatchallRoute {
	if env == nil || len(recipients) == 0 {
		return inboundCatchallRoute{}
	}
	catchalls := inboundCatchallRoutes(env)
	if len(catchalls) == 0 {
		return inboundCatchallRoute{}
	}
	recipientEmails := []string{}
	for _, recipient := range recipients {
		if email := normalizedEmailAddress(firstNonEmpty(firstEmailAddress(recipient), recipient)); email != "" {
			recipientEmails = append(recipientEmails, email)
		}
	}
	recipientEmails = uniqueStrings(recipientEmails)
	if len(recipientEmails) == 0 {
		return inboundCatchallRoute{}
	}
	catchallByEmail := map[string]inboundCatchallRoute{}
	for _, route := range catchalls {
		if route.Email != "" {
			catchallByEmail[route.Email] = route
		}
	}
	matched := inboundCatchallRoute{}
	for _, email := range recipientEmails {
		route, ok := catchallByEmail[email]
		if !ok {
			if anyMatch {
				continue
			}
			return inboundCatchallRoute{}
		}
		if !matched.Matched {
			matched = route
		}
		if anyMatch {
			return route
		}
	}
	return matched
}

func inboundCatchallRoutes(env *record.Env) []inboundCatchallRoute {
	routes := []inboundCatchallRoute{}
	if env == nil {
		return routes
	}
	systemEnv := messageSystemEnv(env)
	seen := map[string]bool{}
	if _, ok := systemEnv.ModelMetadata("mail.alias.domain"); ok {
		if found, err := systemEnv.Model("mail.alias.domain").Search(domain.And()); err == nil {
			if rows, err := found.Read("id", "catchall_email", "catchall_alias", "name"); err == nil {
				for _, row := range rows {
					email := configEmailValue(stringAny(row["catchall_email"]), stringAny(row["name"]))
					if email == "" {
						email = configEmailValue(stringAny(row["catchall_alias"]), stringAny(row["name"]))
					}
					email = normalizedEmailAddress(email)
					if email == "" || seen[email] {
						continue
					}
					seen[email] = true
					routes = append(routes, inboundCatchallRoute{
						Matched:       true,
						AliasDomainID: int64FromAny(row["id"]),
						Email:         email,
					})
				}
			}
		}
	}
	catchallDomain := strings.TrimSpace(configParameter(systemEnv, "mail.catchall.domain"))
	if email := normalizedEmailAddress(configEmailValue(configParameter(systemEnv, "mail.catchall.alias"), catchallDomain)); email != "" && !seen[email] {
		routes = append(routes, inboundCatchallRoute{Matched: true, Email: email})
	}
	return routes
}

func inboundAllowedLocalPartDomains(env *record.Env) (map[string]bool, bool) {
	allowed := map[string]bool{}
	if env == nil {
		return allowed, false
	}
	for _, value := range strings.Split(configParameter(messageSystemEnv(env), "mail.catchall.domain.allowed"), ",") {
		if domainName := strings.ToLower(strings.TrimSpace(value)); domainName != "" {
			allowed[domainName] = true
		}
	}
	if len(allowed) == 0 {
		return allowed, false
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.alias.domain"); ok {
		if found, err := systemEnv.Model("mail.alias.domain").Search(domain.And()); err == nil {
			if rows, err := found.Read("name"); err == nil {
				for _, row := range rows {
					if domainName := strings.ToLower(strings.TrimSpace(stringAny(row["name"]))); domainName != "" {
						allowed[domainName] = true
					}
				}
			}
		}
	}
	return allowed, true
}

func inboundLocalPartDomainAllowed(domainName string, allowed map[string]bool, restricted bool) bool {
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" {
		return false
	}
	return !restricted || allowed[domainName]
}

func inboundAliasRouteFromRow(env *record.Env, modelName string, row map[string]any) inboundAliasRoute {
	forceThreadID := int64FromAny(row["alias_force_thread_id"])
	if forceThreadID != 0 && inboundRouteTargetMissing(env, modelName, forceThreadID) {
		forceThreadID = 0
	}
	return inboundAliasRoute{
		Matched:        true,
		AliasID:        int64FromAny(row["id"]),
		AliasName:      stringAny(row["alias_name"]),
		AliasDomainID:  int64FromAny(row["alias_domain_id"]),
		ModelName:      modelName,
		ForceThreadID:  forceThreadID,
		Defaults:       parseInboundAliasDefaults(row["alias_defaults"]),
		Contact:        firstNonEmpty(strings.TrimSpace(stringAny(row["alias_contact"])), "everyone"),
		BouncedContent: strings.TrimSpace(stringAny(row["alias_bounced_content"])),
		ParentModel:    inboundAliasModelNameFromID(env, int64FromAny(row["alias_parent_model_id"])),
		ParentThreadID: int64FromAny(row["alias_parent_thread_id"]),
	}
}

func matchedInboundAliasRouteFromRow(env *record.Env, modelName string, row map[string]any, authorID int64) (inboundAliasRoute, bool) {
	route := inboundAliasRouteFromRow(env, modelName, row)
	route.UserID = inboundGatewayUserID(env, authorID)
	if allowed, configError := inboundAliasRouteContactAllowed(env, route, authorID); allowed {
		return route, true
	} else {
		route.DeniedConfig = configError
	}
	route.Denied = true
	return route, false
}

func inboundAliasModelName(env *record.Env, row map[string]any) string {
	if modelName := strings.TrimSpace(stringAny(row["model_name"])); modelName != "" {
		return modelName
	}
	modelID := int64FromAny(row["alias_model_id"])
	if env == nil || modelID == 0 {
		return ""
	}
	return inboundAliasModelNameFromID(env, modelID)
}

func inboundAliasModelNameFromID(env *record.Env, modelID int64) string {
	if env == nil || modelID == 0 {
		return ""
	}
	rows, err := env.Model("ir.model").Browse(modelID).Read("model")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(stringAny(rows[0]["model"]))
}

func inboundAliasContactAllowed(env *record.Env, route inboundRoute, authorID int64) bool {
	if !route.AliasMatched {
		return true
	}
	allowed, _ := inboundAliasRouteContactAllowed(env, inboundAliasRoute{
		Matched:        true,
		ModelName:      route.TargetModel,
		ForceThreadID:  route.TargetResID,
		Contact:        route.AliasContact,
		ParentModel:    route.AliasParentModel,
		ParentThreadID: route.AliasParentThreadID,
	}, authorID)
	return allowed
}

func inboundAliasRouteContactAllowed(env *record.Env, route inboundAliasRoute, authorID int64) (allowed bool, configError bool) {
	switch strings.TrimSpace(route.Contact) {
	case "", "everyone":
		return true, false
	case "partners":
		return authorID != 0, false
	case "followers":
		checkModel := route.ModelName
		checkID := route.ForceThreadID
		if checkID == 0 && route.ParentModel != "" && route.ParentThreadID != 0 {
			checkModel = route.ParentModel
			checkID = route.ParentThreadID
		}
		if checkModel == "" || checkID == 0 {
			return false, true
		}
		if env == nil {
			return false, true
		}
		if authorID == 0 {
			return false, false
		}
		return followerExists(messageSystemEnv(env), checkModel, checkID, authorID), false
	default:
		return false, true
	}
}

func createInboundAliasBounceMail(env *record.Env, header netmail.Header, parts inboundBounceParts, rfcMessageID string, route inboundRoute) error {
	if env == nil || !route.AliasMatched || route.AliasID == 0 {
		return nil
	}
	if route.AliasDeniedConfig {
		if err := markInboundAliasStatus(env, route.AliasID, "invalid"); err != nil {
			return err
		}
	}
	recipient := firstNonEmpty(
		firstEmailAddress(decodedHeader(header.Get("Return-Path"))),
		decodedHeader(header.Get("Return-Path")),
		decodedHeader(header.Get("From")),
	)
	if strings.TrimSpace(recipient) == "" {
		return nil
	}
	emailFrom := inboundAliasBounceFrom(env, route.AliasDomainID, header)
	subject := "Re: " + decodedHeader(header.Get("Subject"))
	body := inboundAliasBounceBody(env, route, parts)
	values := map[string]any{
		"email_from":      emailFrom,
		"email_to":        recipient,
		"subject":         subject,
		"body_html":       body,
		"state":           "outgoing",
		"auto_delete":     true,
		"references":      strings.TrimSpace(rfcMessageID),
		"is_notification": true,
	}
	_, err := messageSystemEnv(env).Model("mail.mail").Create(values)
	return err
}

func markInboundAliasStatus(env *record.Env, aliasID int64, status string) error {
	if env == nil || aliasID == 0 || strings.TrimSpace(status) == "" {
		return nil
	}
	if _, ok := messageSystemEnv(env).ModelMetadata("mail.alias"); !ok {
		return nil
	}
	return messageSystemEnv(env).Model("mail.alias").Browse(aliasID).Write(map[string]any{"alias_status": status})
}

func inboundAliasBounceFrom(env *record.Env, aliasDomainID int64, header netmail.Header) string {
	systemEnv := messageSystemEnv(env)
	bounceEmail := defaultBounceEmailForContext(systemEnv, aliasDomainContext(systemEnv, aliasDomainID))
	if bounceEmail == "" {
		bounceEmail = defaultBounceEmail(systemEnv)
	}
	if bounceEmail != "" {
		return (&netmail.Address{Name: "MAILER-DAEMON", Address: bounceEmail}).String()
	}
	if to := decodedHeader(header.Get("To")); strings.TrimSpace(to) != "" {
		return to
	}
	return "MAILER-DAEMON <mailer-daemon@localhost>"
}

func inboundAliasBounceBody(env *record.Env, route inboundRoute, parts inboundBounceParts) string {
	quoted := inboundBodyHTML(parts)
	if strings.TrimSpace(quoted) == "" {
		quoted = "<p></p>"
	}
	var body string
	if route.AliasDeniedConfig {
		body = fmt.Sprintf(
			"<p>Dear Sender,<br /><br />The message below could not be accepted by the address %s. Please try again later or contact %s instead.<br /><br />Kind Regards</p>",
			html.EscapeString(inboundAliasDisplayName(env, route)),
			html.EscapeString(inboundCompanyName(env)),
		)
	} else if strings.TrimSpace(route.AliasBouncedContent) != "" {
		body = route.AliasBouncedContent
	} else {
		body = fmt.Sprintf(
			"<p>Dear Sender,<br /><br />The message below could not be accepted by the address %s. Only %s are allowed to contact it.<br /><br />Kind Regards</p>",
			html.EscapeString(inboundAliasDisplayName(env, route)),
			html.EscapeString(inboundAliasContactDescription(route.AliasContact)),
		)
	}
	return "<div>" + body + "</div><blockquote>" + quoted + "</blockquote>"
}

func inboundAliasDisplayName(env *record.Env, route inboundRoute) string {
	name := strings.TrimSpace(route.AliasName)
	if name == "" {
		name = "alias"
	}
	domainName := aliasDomainName(env, route.AliasDomainID)
	if strings.Contains(name, "@") || domainName == "" {
		return name
	}
	return name + "@" + domainName
}

func inboundAliasContactDescription(contact string) string {
	if strings.TrimSpace(contact) == "partners" {
		return "addresses linked to registered partners"
	}
	return "some specific addresses"
}

func inboundCompanyName(env *record.Env) string {
	if env == nil || env.Context().CompanyID == 0 {
		return "the company"
	}
	rows, err := messageSystemEnv(env).Model("res.company").Browse(env.Context().CompanyID).Read("name")
	if err != nil || len(rows) == 0 {
		return "the company"
	}
	if name := strings.TrimSpace(stringAny(rows[0]["name"])); name != "" {
		return name
	}
	return "the company"
}

func parseInboundAliasDefaults(value any) map[string]any {
	if values, ok := value.(map[string]any); ok {
		out := make(map[string]any, len(values))
		for key, item := range values {
			out[key] = item
		}
		return out
	}
	text := strings.TrimSpace(stringAny(value))
	if text == "" || text == "{}" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err == nil && out != nil {
		return out
	}
	if parsed, ok := parseInboundPythonDict(text); ok {
		return parsed
	}
	normalized := strings.NewReplacer("'", `"`, "True", "true", "False", "false", "None", "null").Replace(text)
	if err := json.Unmarshal([]byte(normalized), &out); err == nil && out != nil {
		return out
	}
	return map[string]any{}
}

func parseInboundPythonDict(text string) (map[string]any, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, false
	}
	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return map[string]any{}, true
	}
	out := map[string]any{}
	for _, item := range splitInboundTopLevel(body, ',') {
		keyExpr, valueExpr, ok := splitInboundPair(item)
		if !ok {
			return nil, false
		}
		key, ok := parseInboundPythonKey(keyExpr)
		if !ok || key == "" {
			return nil, false
		}
		out[key] = parseInboundPythonValue(valueExpr)
	}
	return out, true
}

func splitInboundTopLevel(text string, sep rune) []string {
	var parts []string
	start := 0
	depth := 0
	var quote rune
	escaped := false
	for i, r := range text {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			if depth > 0 {
				depth--
			}
		default:
			if r == sep && depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + len(string(r))
			}
		}
	}
	parts = append(parts, strings.TrimSpace(text[start:]))
	return parts
}

func splitInboundPair(text string) (string, string, bool) {
	parts := splitInboundTopLevel(text, ':')
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func parseInboundPythonKey(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if value, ok := parseInboundPythonString(text); ok {
		return value, true
	}
	return text, true
}

func parseInboundPythonValue(text string) any {
	text = strings.TrimSpace(text)
	if value, ok := parseInboundPythonString(text); ok {
		return value
	}
	switch text {
	case "True", "true":
		return true
	case "False", "false":
		return false
	case "None", "null":
		return nil
	}
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		if parsed, ok := parseInboundPythonDict(text); ok {
			return parsed
		}
	}
	if (strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")) || (strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")")) {
		body := strings.TrimSpace(text[1 : len(text)-1])
		if body == "" {
			return []any{}
		}
		values := []any{}
		for _, item := range splitInboundTopLevel(body, ',') {
			if item != "" {
				values = append(values, parseInboundPythonValue(item))
			}
		}
		return values
	}
	if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(text, 64); err == nil {
		return parsed
	}
	return text
}

func parseInboundPythonString(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return "", false
	}
	quote := text[0]
	if (quote != '\'' && quote != '"') || text[len(text)-1] != quote {
		return "", false
	}
	quoted := `"` + strings.ReplaceAll(strings.ReplaceAll(text[1:len(text)-1], `\`, `\\`), `"`, `\"`) + `"`
	var out string
	if err := json.Unmarshal([]byte(quoted), &out); err == nil {
		return out, true
	}
	return text[1 : len(text)-1], true
}

func aliasDomainName(env *record.Env, aliasDomainID int64) string {
	if env == nil || aliasDomainID == 0 {
		return ""
	}
	rows, err := messageSystemEnv(env).Model("mail.alias.domain").Browse(aliasDomainID).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringAny(rows[0]["name"])
}

func inboundRecipientEmails(header netmail.Header) []string {
	return inboundRecipientEmailsForHeaders(header, "Delivered-To", "To", "Cc", "Resent-To", "Resent-Cc")
}

func inboundRecipientEmailsForHeaders(header netmail.Header, names ...string) []string {
	out := []string{}
	for _, name := range names {
		for _, value := range headerValues(header, name) {
			out = append(out, extractEmailAddresses(decodedHeader(value))...)
		}
	}
	return uniqueStrings(out)
}

func inboundFilteredEmailHeaders(env *record.Env, header netmail.Header) (string, string) {
	to := formattedHeaderEmails(header, "Delivered-To", "To")
	cc := formattedHeaderEmails(header, "Cc")
	aliases := inboundAliasEmails(env, append(append([]string{}, to...), cc...))
	return strings.Join(filterAliasFormattedEmails(to, aliases), ","), strings.Join(filterAliasFormattedEmails(cc, aliases), ",")
}

func formattedHeaderEmails(header netmail.Header, names ...string) []string {
	out := []string{}
	for _, name := range names {
		for _, value := range headerValues(header, name) {
			value = decodedHeader(value)
			if value == "" {
				continue
			}
			normalized := strings.NewReplacer(";", ",", "\n", ",").Replace(value)
			if addresses, err := netmail.ParseAddressList(normalized); err == nil {
				for _, address := range addresses {
					if email := normalizedEmailAddress(address.Address); validEmailAddress(email) {
						if strings.TrimSpace(address.Name) == "" {
							out = append(out, email)
						} else {
							out = append(out, address.String())
						}
					}
				}
				continue
			}
			for _, email := range extractEmailAddresses(value) {
				out = append(out, email)
			}
		}
	}
	return uniqueStrings(out)
}

func headerValues(header netmail.Header, name string) []string {
	values := header[textproto.CanonicalMIMEHeaderKey(name)]
	if len(values) == 0 {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			values = []string{value}
		}
	}
	return values
}

func filterAliasFormattedEmails(emails []string, aliases map[string]bool) []string {
	if len(emails) == 0 || len(aliases) == 0 {
		return emails
	}
	out := []string{}
	for _, email := range emails {
		normalized := normalizedEmailAddress(firstNonEmpty(firstEmailAddress(email), email))
		if normalized != "" && aliases[normalized] {
			continue
		}
		out = append(out, email)
	}
	return out
}

func inboundAliasEmails(env *record.Env, recipients []string) map[string]bool {
	out := map[string]bool{}
	if env == nil || len(recipients) == 0 {
		return out
	}
	recipientSet := map[string]bool{}
	localRecipients := map[string][]string{}
	allowedDomains, restrictLocalDomains := inboundAllowedLocalPartDomains(env)
	for _, recipient := range recipients {
		if email := normalizedEmailAddress(firstNonEmpty(firstEmailAddress(recipient), recipient)); email != "" {
			recipientSet[email] = true
			local, domainName, ok := strings.Cut(email, "@")
			if ok && local != "" && inboundLocalPartDomainAllowed(domainName, allowedDomains, restrictLocalDomains) {
				localRecipients[local] = append(localRecipients[local], email)
			}
		}
	}
	if len(recipientSet) == 0 {
		return out
	}
	systemEnv := messageSystemEnv(env)
	for email := range inboundAliasDomainEmails(systemEnv) {
		if recipientSet[email] {
			out[email] = true
		}
	}
	if _, ok := systemEnv.ModelMetadata("mail.alias"); !ok {
		return out
	}
	found, err := systemEnv.Model("mail.alias").Search(domain.And())
	if err != nil {
		return out
	}
	rows, err := found.Read("alias_name", "alias_domain", "alias_domain_id", "alias_full_name", "alias_incoming_local", "active")
	if err != nil {
		return out
	}
	for _, row := range rows {
		if value, exists := row["active"]; exists && value != nil && !boolAny(value) {
			continue
		}
		for _, email := range aliasRowEmails(env, row) {
			if recipientSet[email] {
				out[email] = true
			}
		}
		if boolAny(row["alias_incoming_local"]) {
			aliasName := strings.ToLower(strings.TrimSpace(stringAny(row["alias_name"])))
			for _, email := range localRecipients[aliasName] {
				out[email] = true
			}
		}
	}
	return out
}

func inboundAliasDomainEmails(env *record.Env) map[string]bool {
	out := map[string]bool{}
	if env == nil {
		return out
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.alias.domain"); !ok {
		return out
	}
	found, err := systemEnv.Model("mail.alias.domain").Search(domain.And())
	if err != nil {
		return out
	}
	rows, err := found.Read("bounce_email", "bounce_alias", "catchall_email", "catchall_alias", "default_from_email", "default_from", "name")
	if err != nil {
		return out
	}
	for _, row := range rows {
		domainName := stringAny(row["name"])
		for _, pair := range [][2]string{
			{"bounce_email", "bounce_alias"},
			{"catchall_email", "catchall_alias"},
			{"default_from_email", "default_from"},
		} {
			email := normalizedEmailAddress(stringAny(row[pair[0]]))
			if !validEmailAddress(email) {
				email = normalizedEmailAddress(configEmailValue(stringAny(row[pair[1]]), domainName))
			}
			if validEmailAddress(email) {
				out[email] = true
			}
		}
	}
	return out
}

func aliasRowEmails(env *record.Env, row map[string]any) []string {
	out := []string{}
	if fullName := normalizedEmailAddress(stringAny(row["alias_full_name"])); fullName != "" {
		out = append(out, fullName)
	}
	aliasName := strings.ToLower(strings.TrimSpace(stringAny(row["alias_name"])))
	if aliasName == "" {
		return out
	}
	domainName := firstNonEmpty(stringAny(row["alias_domain"]), aliasDomainName(env, int64FromAny(row["alias_domain_id"])))
	if domainName != "" {
		if email := normalizedEmailAddress(aliasName + "@" + domainName); email != "" {
			out = append(out, email)
		}
	}
	return uniqueStrings(out)
}

func partnerIDsForEmails(env *record.Env, emails []string) ([]int64, error) {
	out := []int64{}
	for _, email := range uniqueStrings(emails) {
		ids, err := partnerIDsForEmail(env, email)
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
	}
	return uniqueIDs(out), nil
}

func inboundBodyHTML(parts inboundBounceParts) string {
	if body := strings.TrimSpace(parts.FirstHTML); body != "" {
		return body
	}
	if body := strings.TrimSpace(parts.FirstText); body != "" {
		return "<pre>" + html.EscapeString(body) + "</pre>"
	}
	return ""
}

func createInboundAttachments(env *record.Env, modelName string, resID int64, attachments []inboundAttachment, body string) ([]int64, string, error) {
	if env == nil || len(attachments) == 0 {
		return nil, body, nil
	}
	systemEnv := messageSystemEnv(env)
	ids := make([]int64, 0, len(attachments))
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = "attachment"
		}
		mimetype := strings.TrimSpace(attachment.Mimetype)
		if mimetype == "" {
			mimetype = "application/octet-stream"
		}
		values := map[string]any{
			"name":      name,
			"res_model": modelName,
			"res_id":    resID,
			"type":      "binary",
			"mimetype":  mimetype,
			"datas":     append([]byte(nil), attachment.Data...),
			"file_size": len(attachment.Data),
		}
		attachmentID, err := systemEnv.Model("ir.attachment").Create(values)
		if err != nil {
			return nil, body, err
		}
		ids = append(ids, attachmentID)
		cid := strings.Trim(attachment.CID, " <>\t\r\n")
		if cid == "" || !strings.Contains(body, "cid:"+cid) {
			continue
		}
		token := AttachmentRawAccessToken(systemEnv, attachmentID)
		if token != "" {
			if err := systemEnv.Model("ir.attachment").Browse(attachmentID).Write(map[string]any{"access_token": token}); err != nil {
				return nil, body, err
			}
		}
		replacement := fmt.Sprintf("/web/image/%d", attachmentID)
		if token != "" {
			replacement += "?access_token=" + token
		}
		body = strings.ReplaceAll(body, "cid:"+cid, replacement)
	}
	return ids, body, nil
}

func inboundDate(header netmail.Header, fallback time.Time) time.Time {
	if value := strings.TrimSpace(header.Get("Date")); value != "" {
		if parsed, err := netmail.ParseDate(value); err == nil {
			return parsed.UTC()
		}
	}
	return fallback
}

func decodedHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}

func isInboundBounce(env *record.Env, header netmail.Header) bool {
	toValues := strings.Join([]string{
		header.Get("Delivered-To"),
		header.Get("X-Original-To"),
		header.Get("To"),
	}, ",")
	bounceEmails := knownBounceEmails(env)
	if len(bounceEmails) != 0 {
		for _, email := range extractEmailAddresses(toValues) {
			if bounceEmails[email] {
				return true
			}
		}
	}
	if from := firstEmailAddress(header.Get("From")); from != "" {
		local, _, _ := strings.Cut(from, "@")
		if strings.EqualFold(local, "mailer-daemon") {
			return true
		}
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil {
		if mediaType == "multipart/report" || strings.EqualFold(params["report-type"], "delivery-status") {
			return true
		}
	}
	return strings.Contains(contentType, "report-type=delivery-status")
}

func knownBounceEmails(env *record.Env) map[string]bool {
	out := map[string]bool{}
	if env == nil {
		return out
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.alias.domain"); ok {
		if found, err := systemEnv.Model("mail.alias.domain").Search(domain.And()); err == nil {
			if rows, err := found.Read("bounce_email", "bounce_alias", "name"); err == nil {
				for _, row := range rows {
					if email := normalizedEmailAddress(stringAny(row["bounce_email"])); validEmailAddress(email) {
						out[email] = true
						continue
					}
					if email := normalizedEmailAddress(configEmailValue(stringAny(row["bounce_alias"]), stringAny(row["name"]))); validEmailAddress(email) {
						out[email] = true
					}
				}
			}
		}
	}
	if email := normalizedEmailAddress(defaultBounceEmail(env)); validEmailAddress(email) {
		out[email] = true
	}
	return out
}

func collectInboundMIMEParts(contentType string, body []byte, parts *inboundBounceParts) {
	collectInboundMIMEPart(contentType, "", "", "", "", body, parts)
}

func collectInboundMIMEPart(contentType string, disposition string, transferEncoding string, contentID string, filename string, body []byte, parts *inboundBounceParts) {
	if parts == nil {
		return
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{}
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			data, err := io.ReadAll(part)
			_ = part.Close()
			if err != nil {
				return
			}
			collectInboundMIMEPart(
				part.Header.Get("Content-Type"),
				part.Header.Get("Content-Disposition"),
				part.Header.Get("Content-Transfer-Encoding"),
				part.Header.Get("Content-Id"),
				inboundPartFilename(part.Header.Get("Content-Disposition"), part.Header.Get("Content-Type")),
				data,
				parts,
			)
		}
	}
	decoded := decodeInboundPartBody(transferEncoding, body)
	text := string(decoded)
	if filename == "" {
		filename = inboundPartFilename(disposition, contentType)
	}
	cid := strings.Trim(contentID, " <>\t\r\n")
	if inboundPartIsAttachment(mediaType, disposition, filename, cid) {
		name := decodedHeader(filename)
		if strings.TrimSpace(name) == "" {
			name = "attachment"
		}
		mimetype := mediaType
		if mimetype == "" {
			mimetype = "application/octet-stream"
		}
		parts.Attachments = append(parts.Attachments, inboundAttachment{
			Name:     name,
			Data:     decoded,
			Mimetype: mimetype,
			CID:      cid,
		})
		if strings.TrimSpace(parts.OriginalHeaders) == "" && strings.Contains(strings.ToLower(text), "message-id:") {
			parts.OriginalHeaders = text
		}
		return
	}
	switch mediaType {
	case "message/delivery-status":
		if strings.TrimSpace(parts.DeliveryStatus) == "" {
			parts.DeliveryStatus = text
		}
	case "message/rfc822", "text/rfc822-headers":
		if strings.TrimSpace(parts.OriginalHeaders) == "" {
			parts.OriginalHeaders = text
		}
	case "text/html":
		if strings.TrimSpace(parts.FirstHTML) == "" {
			parts.FirstHTML = text
		}
	case "text/plain", "":
		if strings.TrimSpace(parts.FirstText) == "" {
			parts.FirstText = text
		}
	default:
		if strings.TrimSpace(parts.OriginalHeaders) == "" && strings.Contains(strings.ToLower(text), "message-id:") {
			parts.OriginalHeaders = text
		}
	}
}

func inboundPartFilename(disposition string, contentType string) string {
	for _, header := range []string{disposition, contentType} {
		_, params, err := mime.ParseMediaType(strings.TrimSpace(header))
		if err != nil {
			continue
		}
		for _, key := range []string{"filename", "name"} {
			if value := strings.TrimSpace(params[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func inboundPartIsAttachment(mediaType string, disposition string, filename string, cid string) bool {
	disp, _, _ := mime.ParseMediaType(strings.TrimSpace(disposition))
	disp = strings.ToLower(strings.TrimSpace(disp))
	if strings.TrimSpace(filename) != "" {
		return true
	}
	if disp == "attachment" || disp == "inline" && cid != "" {
		return true
	}
	if strings.HasPrefix(mediaType, "text/") || mediaType == "message/delivery-status" || mediaType == "message/rfc822" || mediaType == "text/rfc822-headers" || mediaType == "" {
		return false
	}
	return true
}

func decodeInboundPartBody(encoding string, body []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		compact := strings.NewReplacer("\r", "", "\n", "", "\t", "", " ", "").Replace(string(body))
		decoded, err := base64.StdEncoding.DecodeString(compact)
		if err == nil {
			return decoded
		}
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		if err == nil {
			return decoded
		}
	}
	return append([]byte(nil), body...)
}

func finalRecipientEmail(deliveryStatus string) string {
	value := dsnHeaderValue(deliveryStatus, "Final-Recipient")
	if value == "" {
		value = dsnHeaderValue(deliveryStatus, "Original-Recipient")
	}
	if before, after, ok := strings.Cut(value, ";"); ok && strings.TrimSpace(before) != "" {
		value = after
	}
	value = strings.Trim(value, " <>\"\t\r\n")
	return normalizedEmailAddress(value)
}

func originalMessageReferences(original string) []string {
	if strings.TrimSpace(original) == "" {
		return nil
	}
	if !strings.Contains(original, "\n\n") && !strings.Contains(original, "\r\n\r\n") {
		original += "\r\n\r\n"
	}
	message, err := netmail.ReadMessage(strings.NewReader(original))
	if err != nil {
		return nil
	}
	refs := []string{}
	for _, name := range []string{"Message-Id", "X-Microsoft-Original-Message-ID"} {
		refs = append(refs, unfoldMessageIDs(message.Header.Get(name))...)
	}
	return uniqueStrings(refs)
}

func inboundReferenceMessageIDs(header netmail.Header) []string {
	refs := append([]string{}, unfoldMessageIDs(decodedHeader(header.Get("References")))...)
	refs = append(refs, unfoldMessageIDs(decodedHeader(header.Get("In-Reply-To")))...)
	if len(refs) > 32 {
		refs = refs[len(refs)-32:]
	}
	return uniqueStrings(refs)
}

func inboundSideEffectNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func mergeInboundCustomValues(defaults map[string]any, custom map[string]any) map[string]any {
	if len(defaults) == 0 && len(custom) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range defaults {
		out[key] = value
	}
	for key, value := range custom {
		out[key] = value
	}
	return out
}

func mailingTraceInboundDefaults(env *record.Env, targetFields map[string]field.Field, header netmail.Header) map[string]any {
	if env == nil || len(targetFields) == 0 {
		return nil
	}
	wantsUTM := false
	for _, fieldName := range []string{"campaign_id", "source_id", "medium_id"} {
		if _, ok := targetFields[fieldName]; ok {
			wantsUTM = true
			break
		}
	}
	if !wantsUTM {
		return nil
	}
	trace, err := firstMailingTraceForReferences(env, inboundReferenceMessageIDs(header))
	if err != nil || len(trace) == 0 {
		return nil
	}
	defaults := map[string]any{}
	for _, fieldName := range []string{"campaign_id", "source_id", "medium_id"} {
		if _, ok := targetFields[fieldName]; !ok {
			continue
		}
		if id := int64FromAny(trace[fieldName]); id != 0 {
			defaults[fieldName] = id
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	return defaults
}

func firstMailingTraceForReferences(env *record.Env, references []string) (map[string]any, error) {
	rows, err := mailingTraceRowsForReferences(env, references)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func markMailingTraceReplied(env *record.Env, references []string, now time.Time) error {
	rows, err := mailingTraceRowsForReferences(env, references)
	if err != nil || len(rows) == 0 {
		return err
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		values := map[string]any{
			"trace_status":   "reply",
			"reply_datetime": now,
		}
		status := strings.TrimSpace(stringAny(row["trace_status"]))
		if status != "open" && status != "reply" {
			values["open_datetime"] = now
		}
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func markMailingTraceBounced(env *record.Env, references []string, reason string) error {
	rows, err := mailingTraceRowsForReferences(env, references)
	if err != nil || len(rows) == 0 {
		return err
	}
	systemEnv := messageSystemEnv(env)
	for _, row := range rows {
		if err := systemEnv.Model("mailing.trace").Browse(int64FromAny(row["id"])).Write(map[string]any{
			"failure_reason": cleanBounceText(reason),
			"failure_type":   "mail_bounce",
			"trace_status":   "bounce",
		}); err != nil {
			return err
		}
	}
	return nil
}

func mailingTraceRowsForReferences(env *record.Env, references []string) ([]map[string]any, error) {
	references = uniqueStrings(references)
	if env == nil || len(references) == 0 {
		return nil, nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.trace"); !ok {
		return nil, nil
	}
	refSet := map[string]bool{}
	for _, ref := range references {
		if normalized := normalizeMessageID(ref); normalized != "" {
			refSet[normalized] = true
		}
	}
	if len(refSet) == 0 {
		return nil, nil
	}
	mailIDSet, err := mailMailIDsForMessageIDs(systemEnv, refSet)
	if err != nil {
		return nil, err
	}
	found, err := systemEnv.Model("mailing.trace").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("message_id", "mail_mail_id", "mail_mail_id_int", "campaign_id", "source_id", "medium_id", "trace_status", "email", "write_date")
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	seen := map[int64]bool{}
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id == 0 || seen[id] {
			continue
		}
		messageID := normalizeMessageID(stringAny(row["message_id"]))
		mailID := int64FromAny(firstNonZeroMailValue(row["mail_mail_id"], row["mail_mail_id_int"]))
		if (messageID != "" && refSet[messageID]) || (mailID != 0 && mailIDSet[mailID]) {
			seen[id] = true
			out = append(out, row)
		}
	}
	return out, nil
}

func mailMailIDsForMessageIDs(env *record.Env, references map[string]bool) (map[int64]bool, error) {
	out := map[int64]bool{}
	if env == nil || len(references) == 0 {
		return out, nil
	}
	if _, ok := env.ModelMetadata("mail.mail"); !ok {
		return out, nil
	}
	found, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("message_id")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if references[normalizeMessageID(stringAny(row["message_id"]))] {
			if id := int64FromAny(row["id"]); id != 0 {
				out[id] = true
			}
		}
	}
	return out, nil
}

func firstNonZeroMailValue(values ...any) any {
	for _, value := range values {
		if int64FromAny(value) != 0 {
			return value
		}
	}
	return nil
}

func autoBlacklistBouncedMailingEmail(env *record.Env, email string, partnerIDs []int64, now time.Time) error {
	email = normalizedEmailAddress(email)
	if env == nil || email == "" {
		return nil
	}
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mailing.trace"); !ok {
		return nil
	}
	if _, ok := systemEnv.ModelMetadata("mail.blacklist"); !ok {
		return nil
	}
	found, err := systemEnv.Model("mailing.trace").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("email", "trace_status", "write_date")
	if err != nil {
		return err
	}
	cutoff := now.Add(-13 * 7 * 24 * time.Hour)
	var minDate time.Time
	var maxDate time.Time
	count := 0
	for _, row := range rows {
		if strings.TrimSpace(stringAny(row["trace_status"])) != "bounce" || normalizedEmailAddress(stringAny(row["email"])) != email {
			continue
		}
		writeDate := timeValue(row["write_date"])
		if writeDate.IsZero() {
			writeDate = now
		}
		if writeDate.Before(cutoff) {
			continue
		}
		count++
		if minDate.IsZero() || writeDate.Before(minDate) {
			minDate = writeDate
		}
		if maxDate.IsZero() || writeDate.After(maxDate) {
			maxDate = writeDate
		}
	}
	if count < 5 || !maxDate.After(minDate.Add(7*24*time.Hour)) {
		return nil
	}
	if len(partnerIDs) != 0 && !anyPartnerBounceAtLeast(systemEnv, partnerIDs, 5) {
		return nil
	}
	return upsertMailBlacklist(systemEnv, email, "This email has been automatically added in blocklist because of too much bounced.")
}

func anyPartnerBounceAtLeast(env *record.Env, partnerIDs []int64, threshold int64) bool {
	for _, partnerID := range uniqueIDs(partnerIDs) {
		rows, err := env.Model("res.partner").Browse(partnerID).Read("message_bounce")
		if err == nil && len(rows) != 0 && int64FromAny(rows[0]["message_bounce"]) >= threshold {
			return true
		}
	}
	return false
}

func upsertMailBlacklist(env *record.Env, email string, message string) error {
	found, err := env.Model("mail.blacklist").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("email", "active")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if normalizedEmailAddress(stringAny(row["email"])) != email {
			continue
		}
		return env.Model("mail.blacklist").Browse(int64FromAny(row["id"])).Write(map[string]any{
			"active":  true,
			"message": message,
		})
	}
	_, err = env.Model("mail.blacklist").Create(map[string]any{
		"email":   email,
		"active":  true,
		"message": message,
	})
	return err
}

func unfoldMessageIDs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	matches := messageIDPattern.FindAllString(value, -1)
	if len(matches) != 0 {
		out := make([]string, 0, len(matches))
		for _, match := range matches {
			if normalized := normalizeMessageID(match); normalized != "" {
				out = append(out, normalized)
			}
		}
		return uniqueStrings(out)
	}
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, " ,;")
		if normalized := normalizeMessageID(field); normalized != "" {
			out = append(out, normalized)
		}
	}
	return uniqueStrings(out)
}

func findBouncedMessageID(env *record.Env, references []string) (int64, error) {
	references = uniqueStrings(references)
	if env == nil || len(references) == 0 {
		return 0, nil
	}
	systemEnv := messageSystemEnv(env)
	found, err := systemEnv.Model("mail.message").SearchWithOptions(
		domain.Cond("message_id", "in", references),
		record.SearchOptions{Order: "create_date desc,id desc", Limit: 1},
	)
	if err != nil {
		return 0, err
	}
	rows, err := found.Read("id")
	if err != nil {
		return 0, err
	}
	if len(rows) != 0 {
		return int64FromAny(rows[0]["id"]), nil
	}
	found, err = systemEnv.Model("mail.mail").SearchWithOptions(
		domain.Cond("message_id", "in", references),
		record.SearchOptions{Order: "create_date desc,id desc", Limit: 1},
	)
	if err != nil {
		return 0, err
	}
	rows, err = found.Read("mail_message_id")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64FromAny(rows[0]["mail_message_id"]), nil
}

func partnerIDsForEmail(env *record.Env, email string) ([]int64, error) {
	email = normalizedEmailAddress(email)
	if env == nil || email == "" {
		return nil, nil
	}
	found, err := messageSystemEnv(env).Model("res.partner").Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("email", "email_normalized")
	if err != nil {
		return nil, err
	}
	out := []int64{}
	for _, row := range rows {
		if partnerRowMatchesEmail(row, email) {
			out = append(out, int64FromAny(row["id"]))
		}
	}
	return uniqueIDs(out), nil
}

func singleNotificationPartner(env *record.Env, messageID int64) ([]int64, string, error) {
	if env == nil || messageID == 0 {
		return nil, "", nil
	}
	found, err := messageSystemEnv(env).Model("mail.notification").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		return nil, "", err
	}
	rows, err := found.Read("res_partner_id", "mail_email_address")
	if err != nil {
		return nil, "", err
	}
	partners := []int64{}
	email := ""
	for _, row := range rows {
		if id := int64FromAny(row["res_partner_id"]); id != 0 {
			partners = append(partners, id)
		}
		if email == "" {
			email = stringAny(row["mail_email_address"])
		}
	}
	partners = uniqueIDs(partners)
	if len(partners) != 1 {
		return nil, "", nil
	}
	if email == "" {
		email = partnerEmail(env, partners[0])
	}
	return partners, email, nil
}

func markBounceNotifications(env *record.Env, messageID int64, email string, partnerIDs []int64, reason string) ([]int64, error) {
	if env == nil || messageID == 0 {
		return nil, nil
	}
	email = normalizedEmailAddress(email)
	partnerSet := map[int64]bool{}
	for _, id := range partnerIDs {
		if id != 0 {
			partnerSet[id] = true
		}
	}
	if email == "" && len(partnerSet) == 0 {
		return nil, nil
	}
	found, err := messageSystemEnv(env).Model("mail.notification").Search(domain.Cond("mail_message_id", "=", messageID))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("res_partner_id", "mail_email_address", "notification_type")
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"failure_reason":      cleanBounceText(reason),
		"failure_type":        "mail_bounce",
		"notification_status": "bounce",
	}
	updated := []int64{}
	for _, row := range rows {
		notificationType := strings.TrimSpace(stringAny(row["notification_type"]))
		if notificationType != "" && notificationType != "email" {
			continue
		}
		partnerID := int64FromAny(row["res_partner_id"])
		rowEmail := normalizedEmailAddress(stringAny(row["mail_email_address"]))
		if !partnerSet[partnerID] && (email == "" || rowEmail != email) {
			continue
		}
		id := int64FromAny(row["id"])
		if id == 0 {
			continue
		}
		if err := messageSystemEnv(env).Model("mail.notification").Browse(id).Write(values); err != nil {
			return nil, err
		}
		updated = append(updated, id)
	}
	return updated, nil
}

func incrementPartnerBounceCounters(env *record.Env, email string) error {
	return updatePartnerBounceCounters(env, email, true)
}

func resetPartnerBounceCounters(env *record.Env, email string) error {
	return updatePartnerBounceCounters(env, email, false)
}

func updatePartnerBounceCounters(env *record.Env, email string, increment bool) error {
	email = normalizedEmailAddress(email)
	if env == nil || email == "" {
		return nil
	}
	systemEnv := messageSystemEnv(env)
	found, err := systemEnv.Model("res.partner").Search(domain.And())
	if err != nil {
		return err
	}
	rows, err := found.Read("email", "email_normalized", "message_bounce")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !partnerRowMatchesEmail(row, email) {
			continue
		}
		next := int64(0)
		if increment {
			next = int64FromAny(row["message_bounce"]) + 1
		}
		values := map[string]any{"message_bounce": next}
		if strings.TrimSpace(stringAny(row["email_normalized"])) == "" {
			values["email_normalized"] = email
		}
		if err := systemEnv.Model("res.partner").Browse(int64FromAny(row["id"])).Write(values); err != nil {
			return err
		}
	}
	return nil
}

func partnerRowMatchesEmail(row map[string]any, email string) bool {
	if email == "" {
		return false
	}
	return normalizedEmailAddress(stringAny(row["email_normalized"])) == email ||
		normalizedEmailAddress(stringAny(row["email"])) == email
}

func bounceFailureReason(parts inboundBounceParts) string {
	text := cleanBounceText(firstNonEmpty(parts.FirstText, parts.FirstHTML))
	diagnostic := cleanBounceText(firstNonEmpty(
		dsnHeaderValue(parts.DeliveryStatus, "Diagnostic-Code"),
		dsnHeaderValue(parts.DeliveryStatus, "Status"),
		dsnHeaderValue(parts.DeliveryStatus, "Action"),
	))
	switch {
	case text != "" && diagnostic != "" && !strings.Contains(text, diagnostic):
		return text + "\n" + diagnostic
	case text != "":
		return text
	case diagnostic != "":
		return diagnostic
	default:
		return "Delivery failed"
	}
}

func dsnHeaderValue(text string, name string) string {
	prefix := strings.ToLower(strings.TrimSpace(name)) + ":"
	for _, line := range unfoldedHeaderLines(text) {
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}

func unfoldedHeaderLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(lines) != 0 {
				lines[len(lines)-1] += " " + strings.TrimSpace(line)
			}
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
}

func extractEmailAddresses(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	normalized := strings.NewReplacer(";", ",", "\n", ",").Replace(value)
	out := []string{}
	if addresses, err := netmail.ParseAddressList(normalized); err == nil {
		for _, address := range addresses {
			if email := normalizedEmailAddress(address.Address); validEmailAddress(email) {
				out = append(out, email)
			}
		}
		return uniqueStrings(out)
	}
	for _, match := range emailAddressPattern.FindAllString(value, -1) {
		if email := normalizedEmailAddress(match); validEmailAddress(email) {
			out = append(out, email)
		}
	}
	return uniqueStrings(out)
}

func firstEmailAddress(value string) string {
	emails := extractEmailAddresses(value)
	if len(emails) == 0 {
		return ""
	}
	return emails[0]
}

func cleanBounceText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = html.UnescapeString(value)
	value = htmlTagPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	value = strings.TrimSpace(strings.Join(lines, "\n"))
	value = repeatedBlankLineRegex.ReplaceAllString(value, "\n\n")
	return value
}

func uniqueStrings(values []string) []string {
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
