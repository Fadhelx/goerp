package rag

import (
	"context"
	"strings"
	"testing"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/providers"
	"gorp/internal/ai/sources"
)

func TestRetrieverEmbedsPromptAndFiltersSources(t *testing.T) {
	provider := &recordingProvider{vector: []float64{1, 0}}
	store := embeddings.NewStore(
		ragChunk(1, 10, 2, 1, 100, "Policy", "https://example.test/policy", "company policy", []float64{1, 0}),
		ragChunk(2, 11, 2, 1, 101, "Other", "", "other policy", []float64{1, 0}),
		ragChunk(3, 10, 3, 1, 102, "Wrong Agent", "", "wrong agent", []float64{1, 0}),
		ragChunk(4, 10, 2, 2, 103, "Wrong Company", "", "wrong company", []float64{1, 0}),
		inactiveRAGChunk(5, 10, 2, 1, 104, "Inactive", "inactive policy", []float64{1, 0}),
	)
	retriever := Retriever{
		Provider:       provider,
		Store:          store,
		Authorizer:     companyAuthorizer{companyID: 1},
		EmbeddingModel: "embed-a",
	}

	chunks, err := retriever.Retrieve(context.Background(), Request{
		Agent:     agents.Agent{ID: 2, SourceIDs: []int64{10}},
		CompanyID: 1,
		Prompt:    "What is the policy?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || provider.requests[0].Model != "embed-a" || provider.requests[0].Content[0] != "What is the policy?" {
		t.Fatalf("embedding requests = %+v", provider.requests)
	}
	if len(chunks) != 1 || chunks[0].SourceID != 10 || chunks[0].AttachmentID != 100 || chunks[0].SourceName != "Policy" {
		t.Fatalf("chunks = %+v", chunks)
	}
}

func TestRetrieverKeepsValidLowScoreChunkBehindDisallowedCandidates(t *testing.T) {
	provider := &recordingProvider{vector: []float64{1, 0}}
	var chunks []embeddings.Chunk
	for idx := int64(1); idx <= 25; idx++ {
		chunks = append(chunks, ragChunk(idx, 11, 2, 1, 200+idx, "Wrong Source", "", "wrong source", []float64{1, 0}))
	}
	chunks = append(chunks, ragChunk(100, 10, 2, 1, 100, "Allowed Source", "", "allowed source", []float64{0, 1}))
	retriever := Retriever{
		Provider:       provider,
		Store:          embeddings.NewStore(chunks...),
		EmbeddingModel: "embed-a",
		Limit:          1,
	}
	got, err := retriever.Retrieve(context.Background(), Request{
		Agent:     agents.Agent{ID: 2, SourceIDs: []int64{10}},
		CompanyID: 1,
		Prompt:    "policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SourceID != 10 || got[0].SourceName != "Allowed Source" {
		t.Fatalf("chunks = %+v", got)
	}
}

func inactiveRAGChunk(id, sourceID, agentID, companyID, attachmentID int64, name, content string, vector []float64) embeddings.Chunk {
	chunk := ragChunk(id, sourceID, agentID, companyID, attachmentID, name, "", content, vector)
	chunk.Metadata["is_active"] = false
	return chunk
}

func TestBuildContextUsesOdooRAGShape(t *testing.T) {
	context := BuildContext([]RetrievedChunk{{
		SourceID:     10,
		SourceName:   "Policy",
		AttachmentID: 100,
		Content:      "Use approved vendors.",
	}})
	for _, want := range []string{
		"##RAG context information:",
		"(Source Chunk Policy)",
		"(attachment_id: 100)",
		"Use approved vendors.",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("context missing %q: %s", want, context)
		}
	}
}

func TestApplyNumericCitationsConvertsKnownSources(t *testing.T) {
	text := ApplyNumericCitations(
		"Use approved vendors [SOURCE:100, 101]. Repeat [SOURCE:100]. Unknown [SOURCE:999].",
		map[int64]CitationSource{
			100: {Name: "Policy", URL: "https://example.test/policy"},
			101: {Name: "Guide", URL: "https://example.test/guide"},
		},
	)
	if strings.Contains(text, "[SOURCE:") {
		t.Fatalf("source tokens remain: %s", text)
	}
	if strings.Count(text, "[1]") != 2 || strings.Count(text, "[2]") != 1 {
		t.Fatalf("citation numbering = %s", text)
	}
	if !strings.Contains(text, `href="https://example.test/policy"`) || !strings.Contains(text, `title="Policy"`) {
		t.Fatalf("citation links = %s", text)
	}
}

func TestCitationSourcesSkipsInaccessibleSources(t *testing.T) {
	sourcesByAttachment := CitationSources([]RetrievedChunk{
		{SourceName: "Allowed", AttachmentID: 100, URL: "https://example.test/allowed", Metadata: map[string]any{"user_has_access": true}},
		{SourceName: "Denied", AttachmentID: 101, URL: "https://example.test/denied", Metadata: map[string]any{"user_has_access": false}},
	}, "https://example.test")
	if _, ok := sourcesByAttachment[100]; !ok {
		t.Fatalf("missing allowed citation source: %+v", sourcesByAttachment)
	}
	if _, ok := sourcesByAttachment[101]; ok {
		t.Fatalf("inaccessible citation source exposed: %+v", sourcesByAttachment)
	}
}

func ragChunk(id, sourceID, agentID, companyID, attachmentID int64, name, url, content string, vector []float64) embeddings.Chunk {
	return embeddings.Chunk{
		ID:             id,
		SourceID:       sourceID,
		Ref:            embeddings.RecordRef{Model: "ai.agent.source", ID: sourceID, CompanyID: companyID},
		Content:        content,
		Vector:         vector,
		EmbeddingModel: "embed-a",
		Metadata: map[string]any{
			sources.MetadataSourceID:       sourceID,
			sources.MetadataSourceName:     name,
			sources.MetadataAgentID:        agentID,
			sources.MetadataCompanyID:      companyID,
			sources.MetadataAttachmentID:   attachmentID,
			sources.MetadataSourceURL:      url,
			sources.MetadataEmbeddingModel: "embed-a",
		},
	}
}

type recordingProvider struct {
	vector   []float64
	requests []providers.EmbeddingRequest
}

func (p *recordingProvider) Kind() providers.Kind {
	return providers.KindMock
}

func (p *recordingProvider) Models() []providers.Model {
	return []providers.Model{{ID: "embed-a", Kind: providers.ModelEmbedding, Provider: providers.KindMock}}
}

func (p *recordingProvider) Chat(context.Context, providers.ChatRequest) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, nil
}

func (p *recordingProvider) Embed(_ context.Context, request providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	p.requests = append(p.requests, request)
	return providers.EmbeddingResponse{Vectors: [][]float64{append([]float64(nil), p.vector...)}, Model: request.Model, Provider: providers.KindMock}, nil
}

type companyAuthorizer struct {
	companyID int64
}

func (a companyAuthorizer) CanRead(_ context.Context, ref embeddings.RecordRef) bool {
	return ref.CompanyID == a.companyID
}
