package embeddings

import (
	"context"
	"testing"
)

func TestPermissionFilteredRetrieval(t *testing.T) {
	store := NewStore(
		Chunk{ID: 1, SourceID: 10, Ref: RecordRef{Model: "res.partner", ID: 1, CompanyID: 1}, Content: "allowed", Vector: []float64{1, 0}, EmbeddingModel: "model-a", Metadata: map[string]any{"topic": "policy", "source_id": int64(10), "embedding_model": "model-a"}},
		Chunk{ID: 2, SourceID: 10, Ref: RecordRef{Model: "res.partner", ID: 2, CompanyID: 2}, Content: "denied", Vector: []float64{1, 0}, EmbeddingModel: "model-a", Metadata: map[string]any{"topic": "policy", "source_id": int64(10), "embedding_model": "model-a"}},
		Chunk{ID: 3, SourceID: 11, Ref: RecordRef{Model: "res.partner", ID: 3, CompanyID: 1}, Content: "other", Vector: []float64{0, 1}, EmbeddingModel: "model-a", Metadata: map[string]any{"topic": "other", "source_id": int64(11), "embedding_model": "model-a"}},
		Chunk{ID: 4, SourceID: 10, Ref: RecordRef{Model: "res.partner", ID: 4, CompanyID: 1}, Content: "old model", Vector: []float64{1, 0}, EmbeddingModel: "model-b", Metadata: map[string]any{"topic": "policy", "source_id": int64(10), "embedding_model": "model-b"}},
	)
	results := store.Search(context.Background(), []float64{1, 0}, 10, companyAuthorizer{companyID: 1}, map[string]any{"topic": "policy", "source_id": int64(10), "embedding_model": "model-a"})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Chunk.ID != 1 || results[0].Score <= 0 {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestNoVectorFallbackStillFiltersPermissions(t *testing.T) {
	store := NewNoVectorStore(
		Chunk{ID: 1, Ref: RecordRef{Model: "res.partner", ID: 1, CompanyID: 1}, Content: "company handbook"},
		Chunk{ID: 2, Ref: RecordRef{Model: "res.partner", ID: 2, CompanyID: 2}, Content: "company handbook"},
	)
	results := store.Search(context.Background(), "handbook", 5, companyAuthorizer{companyID: 1}, nil)
	if len(results) != 1 || results[0].Chunk.ID != 1 {
		t.Fatalf("results = %+v", results)
	}
}

type companyAuthorizer struct {
	companyID int64
}

func (a companyAuthorizer) CanRead(_ context.Context, ref RecordRef) bool {
	return ref.CompanyID == a.companyID
}
