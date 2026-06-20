package embeddings

import (
	"context"
	"math"
	"sort"
	"strings"
)

type RecordRef struct {
	Model     string
	ID        int64
	CompanyID int64
}

type Chunk struct {
	ID             int64
	SourceID       int64
	Ref            RecordRef
	Content        string
	Vector         []float64
	EmbeddingModel string
	Metadata       map[string]any
}

type Candidate struct {
	Chunk Chunk
	Score float64
}

type Authorizer interface {
	CanRead(context.Context, RecordRef) bool
}

type Store struct {
	chunks []Chunk
}

func NewStore(chunks ...Chunk) *Store {
	store := &Store{}
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	return store
}

func (s *Store) Add(chunk Chunk) {
	chunk.Vector = append([]float64(nil), chunk.Vector...)
	chunk.Metadata = cloneMap(chunk.Metadata)
	s.chunks = append(s.chunks, chunk)
}

func (s *Store) Search(ctx context.Context, query []float64, limit int, auth Authorizer, metadata map[string]any) []Candidate {
	if limit <= 0 {
		limit = 5
	}
	var candidates []Candidate
	for _, chunk := range s.chunks {
		if auth != nil && !auth.CanRead(ctx, chunk.Ref) {
			continue
		}
		if !metadataMatches(chunk.Metadata, metadata) {
			continue
		}
		candidates = append(candidates, Candidate{Chunk: cloneChunk(chunk), Score: cosine(query, chunk.Vector)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Chunk.ID < candidates[j].Chunk.ID
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}

type NoVectorStore struct {
	chunks []Chunk
}

func NewNoVectorStore(chunks ...Chunk) *NoVectorStore {
	return &NoVectorStore{chunks: append([]Chunk(nil), chunks...)}
}

func (s *NoVectorStore) Search(ctx context.Context, query string, limit int, auth Authorizer, metadata map[string]any) []Candidate {
	if limit <= 0 {
		limit = 5
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var out []Candidate
	for _, chunk := range s.chunks {
		if auth != nil && !auth.CanRead(ctx, chunk.Ref) {
			continue
		}
		if !metadataMatches(chunk.Metadata, metadata) {
			continue
		}
		score := 0.0
		if query != "" && strings.Contains(strings.ToLower(chunk.Content), query) {
			score = 1
		}
		out = append(out, Candidate{Chunk: cloneChunk(chunk), Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Chunk.ID < out[j].Chunk.ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func cosine(left []float64, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for idx := range left {
		dot += left[idx] * right[idx]
		leftNorm += left[idx] * left[idx]
		rightNorm += right[idx] * right[idx]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func metadataMatches(have map[string]any, want map[string]any) bool {
	for key, value := range want {
		if have[key] != value {
			return false
		}
	}
	return true
}

func cloneChunk(chunk Chunk) Chunk {
	chunk.Vector = append([]float64(nil), chunk.Vector...)
	chunk.Metadata = cloneMap(chunk.Metadata)
	return chunk
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
