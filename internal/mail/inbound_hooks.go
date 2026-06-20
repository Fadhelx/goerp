package mail

import (
	netmail "net/mail"
	"strings"
	"sync"
	"time"

	"gorp/internal/record"
)

type InboundMessageAttachment struct {
	Name     string
	Data     []byte
	Mimetype string
	CID      string
}

type InboundMessageData struct {
	MessageID       string
	Subject         string
	BodyHTML        string
	EmailFrom       string
	AuthorID        int64
	ParentID        int64
	IncomingEmailTo string
	IncomingEmailCC string
	To              string
	CC              string
	Date            time.Time
	Header          netmail.Header
	Attachments     []InboundMessageAttachment
}

type InboundMessageNewRequest struct {
	Model        string
	Message      InboundMessageData
	CustomValues map[string]any
}

type InboundMessageUpdateRequest struct {
	Model        string
	ResID        int64
	Message      InboundMessageData
	UpdateValues map[string]any
}

type InboundMessageHandler struct {
	MessageNew    func(env *record.Env, req InboundMessageNewRequest) (int64, error)
	MessageUpdate func(env *record.Env, req InboundMessageUpdateRequest) error
}

var inboundMessageHandlers = struct {
	sync.RWMutex
	byModel map[string]InboundMessageHandler
}{byModel: map[string]InboundMessageHandler{}}

func RegisterInboundMessageHandler(modelName string, handler InboundMessageHandler) func() {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return func() {}
	}
	inboundMessageHandlers.Lock()
	previous, hadPrevious := inboundMessageHandlers.byModel[modelName]
	if handler.MessageNew == nil && handler.MessageUpdate == nil {
		delete(inboundMessageHandlers.byModel, modelName)
	} else {
		inboundMessageHandlers.byModel[modelName] = handler
	}
	inboundMessageHandlers.Unlock()
	return func() {
		inboundMessageHandlers.Lock()
		if hadPrevious {
			inboundMessageHandlers.byModel[modelName] = previous
		} else {
			delete(inboundMessageHandlers.byModel, modelName)
		}
		inboundMessageHandlers.Unlock()
	}
}

func inboundMessageHandler(modelName string) InboundMessageHandler {
	inboundMessageHandlers.RLock()
	handler := inboundMessageHandlers.byModel[strings.TrimSpace(modelName)]
	inboundMessageHandlers.RUnlock()
	return handler
}

func inboundMessageData(header netmail.Header, parts inboundBounceParts, rfcMessageID string, authorID int64, parentID int64, incomingEmailTo string, incomingEmailCC string, now time.Time) InboundMessageData {
	return InboundMessageData{
		MessageID:       strings.TrimSpace(rfcMessageID),
		Subject:         decodedHeader(header.Get("Subject")),
		BodyHTML:        inboundBodyHTML(parts),
		EmailFrom:       decodedHeader(header.Get("From")),
		AuthorID:        authorID,
		ParentID:        parentID,
		IncomingEmailTo: incomingEmailTo,
		IncomingEmailCC: incomingEmailCC,
		To:              decodedHeader(header.Get("To")),
		CC:              decodedHeader(header.Get("Cc")),
		Date:            inboundDate(header, now),
		Header:          cloneMailHeader(header),
		Attachments:     publicInboundAttachments(parts.Attachments),
	}
}

func publicInboundAttachments(attachments []inboundAttachment) []InboundMessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]InboundMessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, InboundMessageAttachment{
			Name:     attachment.Name,
			Data:     append([]byte(nil), attachment.Data...),
			Mimetype: attachment.Mimetype,
			CID:      attachment.CID,
		})
	}
	return out
}

func cloneMailHeader(header netmail.Header) netmail.Header {
	out := netmail.Header{}
	for key, values := range header {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func cloneInboundValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
