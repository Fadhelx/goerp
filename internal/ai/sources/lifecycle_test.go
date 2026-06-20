package sources

import (
	"context"
	"errors"
	"testing"

	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/providers"
)

func TestProcessorCreatesChunksAndMarksReady(t *testing.T) {
	provider := &recordingProvider{}
	source := Source{
		ID:             7,
		AgentID:        11,
		Name:           "Policy",
		Type:           TypeText,
		Model:          "res.partner",
		RecordID:       42,
		AttachmentID:   99,
		URL:            "https://example.test/policy",
		Content:        "abcdefghij",
		EmbeddingModel: "embed-a",
		CompanyID:      3,
	}

	result, err := (Processor{Provider: provider, MaxChunkRunes: 5}).Process(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source.State != StateReady || result.Source.LastError != "" || result.Source.ContentHash == "" {
		t.Fatalf("source = %+v", result.Source)
	}
	if result.Skipped {
		t.Fatal("fresh source should not skip")
	}
	if len(provider.requests) != 1 || provider.requests[0].Model != "embed-a" {
		t.Fatalf("requests = %+v", provider.requests)
	}
	if len(provider.requests[0].Content) != 2 || provider.requests[0].Content[0] != "abcde" || provider.requests[0].Content[1] != "fghij" {
		t.Fatalf("content = %+v", provider.requests[0].Content)
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("chunks = %+v", result.Chunks)
	}
	first := result.Chunks[0]
	if first.ID != 1 || first.SourceID != 7 || first.Content != "abcde" || first.EmbeddingModel != "embed-a" {
		t.Fatalf("first chunk = %+v", first)
	}
	if first.Ref != (embeddings.RecordRef{Model: "res.partner", ID: 42, CompanyID: 3}) {
		t.Fatalf("ref = %+v", first.Ref)
	}
	if first.Metadata[MetadataContentHash] != result.Source.ContentHash || first.Metadata[MetadataProvider] != string(providers.KindMock) {
		t.Fatalf("metadata = %+v", first.Metadata)
	}
	if first.Metadata[MetadataChunkIndex] != 0 || first.Metadata[MetadataChunkCount] != 2 {
		t.Fatalf("chunk metadata = %+v", first.Metadata)
	}
	if first.Metadata[MetadataAttachmentID] != int64(99) || first.Metadata[MetadataSourceURL] != "https://example.test/policy" {
		t.Fatalf("citation metadata = %+v", first.Metadata)
	}
}

func TestProcessorSkipsUnchangedReadySource(t *testing.T) {
	provider := &recordingProvider{}
	hash := HashContent("abcdef")
	existing := []embeddings.Chunk{{
		ID:             99,
		SourceID:       7,
		Content:        "abcdef",
		Vector:         []float64{1, 0},
		EmbeddingModel: "embed-a",
		Metadata: map[string]any{
			MetadataContentHash:    hash,
			MetadataEmbeddingModel: "embed-a",
		},
	}}
	source := Source{ID: 7, Name: "Policy", Content: "abcdef", ContentHash: hash, State: StateReady, EmbeddingModel: "embed-a"}

	result, err := (Processor{Provider: provider}).Process(context.Background(), source, existing)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || len(provider.requests) != 0 {
		t.Fatalf("result=%+v requests=%+v", result, provider.requests)
	}
	result.Chunks[0].Vector[0] = 9
	result.Chunks[0].Metadata[MetadataContentHash] = "changed"
	if existing[0].Vector[0] != 1 || existing[0].Metadata[MetadataContentHash] != hash {
		t.Fatalf("existing chunk was mutated: %+v", existing[0])
	}
}

func TestProcessorMarksFailedOnEmptyContent(t *testing.T) {
	provider := &recordingProvider{}
	result, err := (Processor{Provider: provider, EmbeddingModel: "embed-a"}).Process(context.Background(), Source{Name: "Empty", Content: "  "}, nil)
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("err = %v", err)
	}
	if result.Source.State != StateFailed || result.Source.LastError == "" {
		t.Fatalf("source = %+v", result.Source)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider should not be called: %+v", provider.requests)
	}
}

func TestProcessorMarksFailedOnProviderError(t *testing.T) {
	providerErr := errors.New("embed failed")
	provider := &recordingProvider{err: providerErr}

	result, err := (Processor{Provider: provider, EmbeddingModel: "embed-a"}).Process(context.Background(), Source{Name: "Policy", Content: "abcdef"}, nil)
	if !errors.Is(err, providerErr) {
		t.Fatalf("err = %v", err)
	}
	if result.Source.State != StateFailed || result.Source.LastError != providerErr.Error() {
		t.Fatalf("source = %+v", result.Source)
	}
}

func TestProcessorCopiesRecordCompanyAndModelMetadata(t *testing.T) {
	provider := &recordingProvider{}
	source := Source{
		ID:             17,
		AgentID:        31,
		Name:           "Order Source",
		Type:           TypeRecord,
		Model:          "sale.order",
		RecordID:       55,
		Content:        "order policy",
		EmbeddingModel: "embed-a",
		CompanyID:      9,
	}

	result, err := (Processor{Provider: provider}).Process(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	chunk := result.Chunks[0]
	if chunk.Ref != (embeddings.RecordRef{Model: "sale.order", ID: 55, CompanyID: 9}) {
		t.Fatalf("ref = %+v", chunk.Ref)
	}
	for key, want := range map[string]any{
		MetadataSourceID:       int64(17),
		MetadataSourceName:     "Order Source",
		MetadataSourceType:     string(TypeRecord),
		MetadataAgentID:        int64(31),
		MetadataCompanyID:      int64(9),
		MetadataRecordModel:    "sale.order",
		MetadataRecordID:       int64(55),
		MetadataEmbeddingModel: "embed-a",
	} {
		if got := chunk.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %#v, want %#v", key, got, want)
		}
	}
}

func TestSourceNeedsEmbeddingWhenEmbeddingModelChanges(t *testing.T) {
	source := Source{State: StateReady, ContentHash: HashContent("same"), EmbeddingModel: "new"}
	previous := Source{ContentHash: source.ContentHash, EmbeddingModel: "old"}
	if !source.NeedsEmbedding(previous) {
		t.Fatal("embedding model change should require embedding")
	}
}

type recordingProvider struct {
	err      error
	requests []providers.EmbeddingRequest
}

func (p *recordingProvider) Kind() providers.Kind {
	return providers.KindMock
}

func (p *recordingProvider) Models() []providers.Model {
	return []providers.Model{{ID: "embed-a", Label: "Embed A", Kind: providers.ModelEmbedding, Provider: providers.KindMock, Dimensions: 2}}
}

func (p *recordingProvider) Chat(context.Context, providers.ChatRequest) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, nil
}

func (p *recordingProvider) Embed(_ context.Context, request providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return providers.EmbeddingResponse{}, p.err
	}
	vectors := make([][]float64, 0, len(request.Content))
	for idx := range request.Content {
		vectors = append(vectors, []float64{float64(idx + 1), 1})
	}
	return providers.EmbeddingResponse{Vectors: vectors, Model: request.Model, Provider: providers.KindMock}, nil
}
