package controllers

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/providers"
	"gorp/internal/ai/rag"
)

const (
	DefaultTokenLifespanSeconds = 7200
	RouteGenerateResponse       = "/ai/generate_response"
	RouteCloseAIChat            = "/ai/close_ai_chat"
	RouteTranscriptionSession   = "/ai/transcription/session"
)

var (
	ErrChannelNotFound  = errors.New("ai channel not found")
	ErrMessageNotFound  = errors.New("ai message not found")
	ErrNotAIChannel     = errors.New("channel is not attached to an ai agent")
	ErrResponderMissing = errors.New("ai responder missing")
	ErrProviderMissing  = errors.New("ai transcription provider missing")
)

type Route struct {
	Name     string
	Path     string
	Methods  []string
	Type     string
	Auth     string
	Readonly bool
}

func Routes() []Route {
	return []Route{
		{Name: "ai.generate_response", Path: RouteGenerateResponse, Methods: []string{"POST"}, Type: "jsonrpc", Auth: "user"},
		{Name: "ai.close_ai_chat", Path: RouteCloseAIChat, Methods: []string{"POST"}, Type: "jsonrpc", Auth: "user"},
		{Name: "ai.transcription_session", Path: RouteTranscriptionSession, Methods: []string{"POST"}, Type: "jsonrpc", Auth: "user", Readonly: true},
	}
}

type Channel struct {
	ID           int64
	Name         string
	Type         string
	IsMember     bool
	Agent        agents.Agent
	AIEnvContext []string
}

type Message struct {
	ID            int64
	Body          string
	AuthorIsAgent bool
	CreatedAt     time.Time
}

type ChannelStore interface {
	Channel(context.Context, int64) (Channel, bool)
	Message(context.Context, int64) (Message, bool)
	History(context.Context, int64, int) []Message
	DeleteChannel(context.Context, int64) error
	PostAssistantMessage(context.Context, int64, string) error
}

type Responder interface {
	Generate(context.Context, agents.Agent, agents.Request) (agents.Response, error)
}

type GenerateFunc func(context.Context, agents.Agent, agents.Request) (agents.Response, error)

func (f GenerateFunc) Generate(ctx context.Context, agent agents.Agent, request agents.Request) (agents.Response, error) {
	return f(ctx, agent, request)
}

func ProviderResponder(provider providers.Provider) Responder {
	return GenerateFunc(func(ctx context.Context, agent agents.Agent, request agents.Request) (agents.Response, error) {
		return agents.Generate(ctx, agent, provider, request)
	})
}

type ChatService struct {
	Store     ChannelStore
	Responder Responder
	Retriever ContextRetriever
	BaseURL   string
}

type ContextRetriever interface {
	Retrieve(context.Context, rag.Request) ([]rag.RetrievedChunk, error)
}

type GenerateResponseRequest struct {
	UserID              int64          `json:"-"`
	CompanyID           int64          `json:"-"`
	MailMessageID       int64          `json:"mail_message_id"`
	ChannelID           int64          `json:"channel_id"`
	CurrentViewInfo     map[string]any `json:"current_view_info"`
	AISessionIdentifier string         `json:"ai_session_identifier"`
}

type GenerateResponseResult struct {
	ChannelID int64  `json:"channel_id"`
	MessageID int64  `json:"mail_message_id"`
	Text      string `json:"text"`
	Model     string `json:"model"`
	Posted    bool   `json:"posted"`
}

func (s ChatService) GenerateResponse(ctx context.Context, request GenerateResponseRequest) (GenerateResponseResult, error) {
	if s.Store == nil {
		return GenerateResponseResult{}, ErrChannelNotFound
	}
	channel, ok := s.Store.Channel(ctx, request.ChannelID)
	if !ok {
		return GenerateResponseResult{}, ErrChannelNotFound
	}
	if channel.Agent.ID == 0 {
		return GenerateResponseResult{}, ErrNotAIChannel
	}
	if !channel.IsMember {
		return GenerateResponseResult{}, ErrChannelNotFound
	}
	message, ok := s.Store.Message(ctx, request.MailMessageID)
	if !ok {
		return GenerateResponseResult{}, nil
	}
	if s.Responder == nil {
		return GenerateResponseResult{}, ErrResponderMissing
	}
	prompt := MessagePrompt(message.Body)
	contextParts := append([]string(nil), channel.AIEnvContext...)
	if sessionContext := BuildSessionInfoContext(request.CurrentViewInfo, request.AISessionIdentifier); sessionContext != "" {
		contextParts = append(contextParts, sessionContext)
	}
	var retrieved []rag.RetrievedChunk
	if s.Retriever != nil {
		var err error
		retrieved, err = s.Retriever.Retrieve(ctx, rag.Request{
			Agent:     channel.Agent,
			UserID:    request.UserID,
			CompanyID: request.CompanyID,
			Prompt:    prompt,
		})
		if err != nil {
			return GenerateResponseResult{}, err
		}
		if ragContext := rag.BuildContext(retrieved); ragContext != "" {
			contextParts = append(contextParts, ragContext)
		}
	}
	response, err := s.Responder.Generate(ctx, channel.Agent, agents.Request{
		UserID:       request.UserID,
		CompanyID:    request.CompanyID,
		Prompt:       prompt,
		Context:      contextParts,
		Conversation: historyMessagesWithoutCurrent(s.Store.History(ctx, channel.ID, 20), message.ID),
		ActiveModel:  strings.TrimSpace(toString(request.CurrentViewInfo["model"])),
		ActiveID:     int64FromAny(firstNonNil(request.CurrentViewInfo["active_id"], request.CurrentViewInfo["res_id"])),
		SessionID:    strings.TrimSpace(request.AISessionIdentifier),
	})
	if err != nil {
		return GenerateResponseResult{}, err
	}
	if len(retrieved) > 0 {
		response.Text = rag.ApplyNumericCitations(response.Text, rag.CitationSources(retrieved, s.BaseURL))
	}
	result := GenerateResponseResult{
		ChannelID: channel.ID,
		MessageID: message.ID,
		Text:      response.Text,
		Model:     response.Model,
	}
	if strings.TrimSpace(response.Text) != "" {
		if err := s.Store.PostAssistantMessage(ctx, channel.ID, response.Text); err != nil {
			return GenerateResponseResult{}, err
		}
		result.Posted = true
	}
	return result, nil
}

func (s ChatService) CloseAIChat(ctx context.Context, channelID int64) (bool, error) {
	if s.Store == nil {
		return false, nil
	}
	channel, ok := s.Store.Channel(ctx, channelID)
	if !ok || channel.Agent.ID == 0 || !channel.IsMember {
		return false, nil
	}
	if err := s.Store.DeleteChannel(ctx, channelID); err != nil {
		return false, err
	}
	return true, nil
}

type RealtimeExpiration struct {
	Anchor  string `json:"anchor"`
	Seconds int    `json:"seconds"`
}

type RealtimeTranscription struct {
	Language string `json:"language"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
}

type RealtimeTurnDetection struct {
	Type string `json:"type"`
}

type RealtimeNoiseReduction struct {
	Type string `json:"type"`
}

type RealtimeAudioInput struct {
	Transcription  RealtimeTranscription  `json:"transcription"`
	TurnDetection  RealtimeTurnDetection  `json:"turn_detection"`
	NoiseReduction RealtimeNoiseReduction `json:"noise_reduction"`
}

type RealtimeAudio struct {
	Input RealtimeAudioInput `json:"input"`
}

type RealtimeSessionSpec struct {
	Type  string        `json:"type"`
	Audio RealtimeAudio `json:"audio"`
}

type RealtimeParameters struct {
	ExpiresAfter RealtimeExpiration  `json:"expires_after"`
	Session      RealtimeSessionSpec `json:"session"`
}

type TranscriptionSession map[string]any

type TranscriptionProvider interface {
	CreateTranscriptionSession(context.Context, RealtimeParameters) (TranscriptionSession, error)
}

type TranscriptionService struct {
	Provider TranscriptionProvider
	Now      func() time.Time
}

func (s TranscriptionService) CreateSession(ctx context.Context, language string, prompt string) (TranscriptionSession, error) {
	if s.Provider == nil {
		return nil, ErrProviderMissing
	}
	params := DefaultRealtimeParameters(language, prompt)
	session, err := s.Provider.CreateTranscriptionSession(ctx, params)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func DefaultRealtimeParameters(language string, prompt string) RealtimeParameters {
	return RealtimeParameters{
		ExpiresAfter: RealtimeExpiration{Anchor: "created_at", Seconds: DefaultTokenLifespanSeconds},
		Session: RealtimeSessionSpec{
			Type: "transcription",
			Audio: RealtimeAudio{
				Input: RealtimeAudioInput{
					Transcription:  RealtimeTranscription{Language: language, Model: "whisper-1", Prompt: prompt},
					TurnDetection:  RealtimeTurnDetection{Type: "server_vad"},
					NoiseReduction: RealtimeNoiseReduction{Type: "far_field"},
				},
			},
		},
	}
}

func BuildSessionInfoContext(view map[string]any, sessionIdentifier string) string {
	var lines []string
	if strings.TrimSpace(sessionIdentifier) != "" {
		lines = append(lines, "session_identifier="+strings.TrimSpace(sessionIdentifier))
	}
	if len(view) > 0 {
		keys := make([]string, 0, len(view))
		for key := range view {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, key+"="+strings.TrimSpace(toString(view[key])))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "<session_info_context>\n" + strings.Join(lines, "\n") + "\n</session_info_context>"
}

func MessagePrompt(body string) string {
	text := tagPattern.ReplaceAllString(body, " ")
	return strings.Join(strings.Fields(html.UnescapeString(text)), " ")
}

func historyMessages(messages []Message) []providers.Message {
	return historyMessagesWithoutCurrent(messages, 0)
}

func historyMessagesWithoutCurrent(messages []Message, currentID int64) []providers.Message {
	out := make([]providers.Message, 0, len(messages))
	for _, message := range messages {
		if currentID != 0 && message.ID == currentID {
			continue
		}
		role := "user"
		if message.AuthorIsAgent {
			role = "assistant"
		}
		out = append(out, providers.Message{Role: role, Content: MessagePrompt(message.Body)})
	}
	return out
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		return strings.Join(typed, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, toString(item))
		}
		return strings.Join(parts, ",")
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case string:
		var out int64
		_, _ = fmt.Sscan(strings.TrimSpace(typed), &out)
		return out
	default:
		return 0
	}
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)
