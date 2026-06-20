package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/providers"
)

const (
	DefaultMaxChunkRunes = 800

	MetadataSourceID       = "source_id"
	MetadataSourceName     = "source_name"
	MetadataSourceType     = "source_type"
	MetadataAgentID        = "agent_id"
	MetadataCompanyID      = "company_id"
	MetadataRecordModel    = "record_model"
	MetadataRecordID       = "record_id"
	MetadataAttachmentID   = "attachment_id"
	MetadataSourceURL      = "source_url"
	MetadataContentHash    = "content_hash"
	MetadataEmbeddingModel = "embedding_model"
	MetadataProvider       = "provider"
	MetadataChunkIndex     = "chunk_index"
	MetadataChunkCount     = "chunk_count"
)

var (
	ErrEmptyContent       = errors.New("ai source content is empty")
	ErrEmbeddingModel     = errors.New("ai source embedding model is required")
	ErrEmbeddingProvider  = errors.New("ai source embedding provider is required")
	ErrEmbeddingVectorSet = errors.New("ai source embedding vector count mismatch")
)

type Processor struct {
	Provider       providers.Provider
	EmbeddingModel string
	MaxChunkRunes  int
}

type ProcessResult struct {
	Source  Source
	Chunks  []embeddings.Chunk
	Skipped bool
}

func (p Processor) Process(ctx context.Context, source Source, existing []embeddings.Chunk) (ProcessResult, error) {
	source = normalize(source)
	model := firstNonEmpty(source.EmbeddingModel, p.EmbeddingModel)
	storedHash := source.ContentHash
	source.EmbeddingModel = model
	hash := HashContent(source.Content)
	source.ContentHash = hash

	if strings.TrimSpace(source.Name) == "" {
		return fail(source, fmt.Errorf("source requires name"))
	}
	if hash == "" {
		return fail(source, ErrEmptyContent)
	}
	if model == "" {
		return fail(source, ErrEmbeddingModel)
	}
	if p.Provider == nil {
		return fail(source, ErrEmbeddingProvider)
	}
	if canReuse(source, existing, storedHash, hash, model) {
		source.State = StateReady
		source.LastError = ""
		return ProcessResult{Source: source, Chunks: cloneChunks(existing), Skipped: true}, nil
	}

	source.State = StateProcessing
	texts := Chunk(source.Content, p.MaxChunkRunes)
	response, err := p.Provider.Embed(ctx, providers.EmbeddingRequest{Model: model, Content: texts})
	if err != nil {
		return fail(source, err)
	}
	if len(response.Vectors) != len(texts) {
		return fail(source, fmt.Errorf("%w: got %d vectors for %d chunks", ErrEmbeddingVectorSet, len(response.Vectors), len(texts)))
	}

	chunks := make([]embeddings.Chunk, 0, len(texts))
	for idx, text := range texts {
		chunks = append(chunks, embeddings.Chunk{
			ID:       int64(idx + 1),
			SourceID: source.ID,
			Ref: embeddings.RecordRef{
				Model:     source.Model,
				ID:        source.RecordID,
				CompanyID: source.CompanyID,
			},
			Content:        text,
			Vector:         append([]float64(nil), response.Vectors[idx]...),
			EmbeddingModel: model,
			Metadata:       sourceMetadata(source, hash, model, response.Provider, idx, len(texts)),
		})
	}
	source.State = StateReady
	source.LastError = ""
	return ProcessResult{Source: source, Chunks: chunks}, nil
}

func normalize(source Source) Source {
	if source.Type == "" {
		source.Type = TypeText
	}
	if source.State == "" {
		source.State = StateDraft
	}
	return source
}

func fail(source Source, err error) (ProcessResult, error) {
	source.State = StateFailed
	source.LastError = err.Error()
	return ProcessResult{Source: source}, err
}

func canReuse(source Source, existing []embeddings.Chunk, storedHash string, hash string, model string) bool {
	if source.State != StateReady || storedHash != hash || source.EmbeddingModel != model || len(existing) == 0 {
		return false
	}
	for _, chunk := range existing {
		if chunk.SourceID != source.ID || chunk.EmbeddingModel != model || len(chunk.Vector) == 0 {
			return false
		}
		if chunk.Metadata[MetadataContentHash] != hash || chunk.Metadata[MetadataEmbeddingModel] != model {
			return false
		}
	}
	return true
}

func sourceMetadata(source Source, hash string, model string, provider providers.Kind, index int, count int) map[string]any {
	metadata := map[string]any{
		MetadataSourceID:       source.ID,
		MetadataSourceName:     source.Name,
		MetadataSourceType:     string(source.Type),
		MetadataAgentID:        source.AgentID,
		MetadataCompanyID:      source.CompanyID,
		MetadataRecordModel:    source.Model,
		MetadataRecordID:       source.RecordID,
		MetadataAttachmentID:   source.AttachmentID,
		MetadataSourceURL:      source.URL,
		MetadataContentHash:    hash,
		MetadataEmbeddingModel: model,
		MetadataChunkIndex:     index,
		MetadataChunkCount:     count,
	}
	if provider != "" {
		metadata[MetadataProvider] = string(provider)
	}
	return metadata
}

func cloneChunks(chunks []embeddings.Chunk) []embeddings.Chunk {
	out := make([]embeddings.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		chunk.Vector = append([]float64(nil), chunk.Vector...)
		chunk.Metadata = cloneMetadata(chunk.Metadata)
		out = append(out, chunk)
	}
	return out
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
