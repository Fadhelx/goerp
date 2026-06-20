package controllers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/rag"
)

func TestRoutesMatchOdooAIControllers(t *testing.T) {
	routes := Routes()
	assertRoute(t, routes, RouteGenerateResponse, "user", false)
	assertRoute(t, routes, RouteCloseAIChat, "user", false)
	assertRoute(t, routes, RouteTranscriptionSession, "user", true)
}

func TestTranscriptionSessionParameters(t *testing.T) {
	provider := &captureTranscriptionProvider{}
	service := TranscriptionService{Provider: provider}
	session, err := service.CreateSession(context.Background(), "en", "Summarize")
	if err != nil {
		t.Fatal(err)
	}
	if provider.params.ExpiresAfter.Anchor != "created_at" || provider.params.ExpiresAfter.Seconds != DefaultTokenLifespanSeconds {
		t.Fatalf("expires_after = %+v", provider.params.ExpiresAfter)
	}
	input := provider.params.Session.Audio.Input
	if provider.params.Session.Type != "transcription" ||
		input.Transcription.Language != "en" ||
		input.Transcription.Model != "whisper-1" ||
		input.Transcription.Prompt != "Summarize" ||
		input.TurnDetection.Type != "server_vad" ||
		input.NoiseReduction.Type != "far_field" {
		t.Fatalf("params = %+v", provider.params)
	}
	if session["value"] != "secret" || session["expires_at"] != int64(7200) {
		t.Fatalf("session = %#v", session)
	}
	if _, ok := session["parameters"]; ok {
		t.Fatalf("session mutated with parameters: %#v", session)
	}
}

func TestGenerateResponsePostsAssistantMessage(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{
		ID:       10,
		Type:     "ai_chat",
		IsMember: true,
		Agent: agents.Agent{
			ID:     2,
			Name:   "AI",
			Model:  "mock-chat",
			Active: true,
		},
		AIEnvContext: []string{"record context"},
	}
	store.messages[30] = Message{ID: 30, Body: "<p>Hello&nbsp;AI</p>"}
	store.history[10] = []Message{{ID: 29, Body: "<p>Earlier</p>"}}
	responder := &captureResponder{response: agents.Response{Text: "Answer", Model: "mock-chat"}}
	service := ChatService{Store: store, Responder: responder}
	result, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{
		UserID:              4,
		CompanyID:           5,
		MailMessageID:       30,
		ChannelID:           10,
		CurrentViewInfo:     map[string]any{"model": "res.partner", "view_type": "form"},
		AISessionIdentifier: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Posted || result.Text != "Answer" || store.posted[10][0] != "Answer" {
		t.Fatalf("result=%+v posted=%+v", result, store.posted)
	}
	request := responder.requests[0]
	if request.Prompt != "Hello AI" {
		t.Fatalf("prompt = %q", request.Prompt)
	}
	if len(request.Context) != 2 || request.Context[0] != "record context" {
		t.Fatalf("context = %+v", request.Context)
	}
	if len(request.Conversation) != 1 || request.Conversation[0].Content != "Earlier" {
		t.Fatalf("conversation = %+v", request.Conversation)
	}
}

func TestGenerateResponseExcludesCurrentMessageFromHistory(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{
		ID:       10,
		IsMember: true,
		Agent: agents.Agent{
			ID:     2,
			Name:   "AI",
			Model:  "mock-chat",
			Active: true,
		},
	}
	store.messages[30] = Message{ID: 30, Body: "Current"}
	store.history[10] = []Message{{ID: 29, Body: "Previous"}, {ID: 30, Body: "Current"}}
	responder := &captureResponder{response: agents.Response{Text: "Answer", Model: "mock-chat"}}
	service := ChatService{Store: store, Responder: responder}
	_, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{MailMessageID: 30, ChannelID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(responder.requests) != 1 || len(responder.requests[0].Conversation) != 1 || responder.requests[0].Conversation[0].Content != "Previous" {
		t.Fatalf("conversation = %+v", responder.requests)
	}
}

func TestGenerateResponseRequiresAIChannel(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{ID: 10, Type: "channel"}
	service := ChatService{Store: store, Responder: &captureResponder{}}
	_, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{ChannelID: 10})
	if !errors.Is(err, ErrNotAIChannel) {
		t.Fatalf("expected ErrNotAIChannel, got %v", err)
	}
}

func TestGenerateResponseRequiresChannelMembership(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{ID: 10, Agent: agents.Agent{ID: 2, Name: "AI", Model: "mock-chat", Active: true}, IsMember: false}
	store.messages[30] = Message{ID: 30, Body: "Hello"}
	responder := &captureResponder{response: agents.Response{Text: "Answer"}}
	service := ChatService{Store: store, Responder: responder}
	_, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{ChannelID: 10, MailMessageID: 30})
	if !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}
	if len(responder.requests) != 0 {
		t.Fatalf("responder called: %+v", responder.requests)
	}
}

func TestGenerateResponseMissingMessageNoops(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{ID: 10, IsMember: true, Agent: agents.Agent{ID: 2, Active: true, Model: "mock-chat"}}
	responder := &captureResponder{response: agents.Response{Text: "Answer"}}
	service := ChatService{Store: store, Responder: responder}
	result, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{ChannelID: 10, MailMessageID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if result.Posted || len(responder.requests) != 0 || len(store.posted) != 0 {
		t.Fatalf("result=%+v requests=%+v posted=%+v", result, responder.requests, store.posted)
	}
}

func TestGenerateResponseInjectsRAGContextAndAppliesCitations(t *testing.T) {
	store := newMemoryStore()
	store.channels[10] = Channel{
		ID:       10,
		IsMember: true,
		Agent: agents.Agent{
			ID:        2,
			Name:      "AI",
			Model:     "mock-chat",
			Active:    true,
			SourceIDs: []int64{77},
		},
	}
	store.messages[30] = Message{ID: 30, Body: "What is the policy?"}
	retriever := &captureRetriever{chunks: []rag.RetrievedChunk{{
		ID:           1,
		SourceID:     77,
		SourceName:   "Policy",
		AttachmentID: 100,
		URL:          "https://example.test/policy",
		Content:      "Use approved vendors.",
	}}}
	responder := &captureResponder{response: agents.Response{Text: "Use approved vendors [SOURCE:100].", Model: "mock-chat"}}
	service := ChatService{Store: store, Responder: responder, Retriever: retriever, BaseURL: "https://example.test"}

	result, err := service.GenerateResponse(context.Background(), GenerateResponseRequest{UserID: 4, CompanyID: 5, MailMessageID: 30, ChannelID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(retriever.requests) != 1 || retriever.requests[0].Prompt != "What is the policy?" || retriever.requests[0].Agent.ID != 2 {
		t.Fatalf("retriever requests = %+v", retriever.requests)
	}
	if len(responder.requests) != 1 || len(responder.requests[0].Context) != 1 {
		t.Fatalf("responder requests = %+v", responder.requests)
	}
	if got := responder.requests[0].Context[0]; !containsAll(got, "##RAG context information:", "(Source Chunk Policy)", "(attachment_id: 100)", "Use approved vendors.") {
		t.Fatalf("rag context = %s", got)
	}
	if result.Text != `Use approved vendors<sup><a href="https://example.test/policy" target="_blank" rel="noreferrer noopener" title="Policy" style="text-decoration: none;"> [1] </a></sup>.` {
		t.Fatalf("result text = %s", result.Text)
	}
	if store.posted[10][0] != result.Text {
		t.Fatalf("posted = %+v", store.posted)
	}
}

func TestCloseAIChatOnlyDeletesMemberAIChannel(t *testing.T) {
	store := newMemoryStore()
	store.channels[1] = Channel{ID: 1, IsMember: true, Agent: agents.Agent{ID: 9}}
	store.channels[2] = Channel{ID: 2, IsMember: false, Agent: agents.Agent{ID: 9}}
	service := ChatService{Store: store}
	deleted, err := service.CloseAIChat(context.Background(), 1)
	if err != nil || !deleted || !store.deleted[1] {
		t.Fatalf("deleted=%v err=%v store=%+v", deleted, err, store.deleted)
	}
	deleted, err = service.CloseAIChat(context.Background(), 2)
	if err != nil || deleted || store.deleted[2] {
		t.Fatalf("deleted=%v err=%v store=%+v", deleted, err, store.deleted)
	}
}

func assertRoute(t *testing.T, routes []Route, path string, auth string, readonly bool) {
	t.Helper()
	for _, route := range routes {
		if route.Path == path {
			if route.Auth != auth || route.Type != "jsonrpc" || route.Readonly != readonly {
				t.Fatalf("route = %+v", route)
			}
			return
		}
	}
	t.Fatalf("missing route %s", path)
}

type captureTranscriptionProvider struct {
	params RealtimeParameters
}

func (p *captureTranscriptionProvider) CreateTranscriptionSession(_ context.Context, params RealtimeParameters) (TranscriptionSession, error) {
	p.params = params
	return TranscriptionSession{"value": "secret", "expires_at": int64(7200), "session": map[string]any{"type": "transcription"}}, nil
}

type captureResponder struct {
	response agents.Response
	requests []agents.Request
}

func (r *captureResponder) Generate(_ context.Context, _ agents.Agent, request agents.Request) (agents.Response, error) {
	r.requests = append(r.requests, request)
	return r.response, nil
}

type captureRetriever struct {
	chunks   []rag.RetrievedChunk
	requests []rag.Request
}

func (r *captureRetriever) Retrieve(_ context.Context, request rag.Request) ([]rag.RetrievedChunk, error) {
	r.requests = append(r.requests, request)
	return append([]rag.RetrievedChunk(nil), r.chunks...), nil
}

func containsAll(value string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}

type memoryStore struct {
	channels map[int64]Channel
	messages map[int64]Message
	history  map[int64][]Message
	posted   map[int64][]string
	deleted  map[int64]bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		channels: map[int64]Channel{},
		messages: map[int64]Message{},
		history:  map[int64][]Message{},
		posted:   map[int64][]string{},
		deleted:  map[int64]bool{},
	}
}

func (s *memoryStore) Channel(_ context.Context, id int64) (Channel, bool) {
	channel, ok := s.channels[id]
	return channel, ok
}

func (s *memoryStore) Message(_ context.Context, id int64) (Message, bool) {
	message, ok := s.messages[id]
	return message, ok
}

func (s *memoryStore) History(_ context.Context, channelID int64, limit int) []Message {
	history := append([]Message(nil), s.history[channelID]...)
	if limit > 0 && len(history) > limit {
		return history[len(history)-limit:]
	}
	return history
}

func (s *memoryStore) DeleteChannel(_ context.Context, id int64) error {
	s.deleted[id] = true
	delete(s.channels, id)
	return nil
}

func (s *memoryStore) PostAssistantMessage(_ context.Context, channelID int64, body string) error {
	s.posted[channelID] = append(s.posted[channelID], body)
	return nil
}
