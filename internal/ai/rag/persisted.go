package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/providers"
	"gorp/internal/ai/sources"
	"gorp/internal/domain"
	"gorp/internal/record"
)

const ModelEmbedding = "ai.embedding"

type PersistedRetriever struct {
	Env            *record.Env
	Provider       providers.Provider
	Authorizer     embeddings.Authorizer
	EmbeddingModel string
	Limit          int
}

func StoreChunks(env *record.Env, chunks []embeddings.Chunk) ([]int64, error) {
	if env == nil || len(chunks) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(chunks))
	for index, chunk := range chunks {
		vector, err := json.Marshal(chunk.Vector)
		if err != nil {
			return nil, err
		}
		metadata := cloneMetadata(chunk.Metadata)
		mergeDefault(metadata, sources.MetadataSourceID, chunk.SourceID)
		mergeDefault(metadata, sources.MetadataRecordModel, chunk.Ref.Model)
		mergeDefault(metadata, sources.MetadataRecordID, chunk.Ref.ID)
		mergeDefault(metadata, sources.MetadataCompanyID, chunk.Ref.CompanyID)
		mergeDefault(metadata, sources.MetadataEmbeddingModel, chunk.EmbeddingModel)
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}
		sequence := int64Value(metadata[sources.MetadataChunkIndex]) + 1
		if sequence <= 0 {
			sequence = int64(index + 1)
		}
		id, err := env.Model(ModelEmbedding).Create(map[string]any{
			"agent_source_id":                 chunk.SourceID,
			"attachment_id":                   int64Value(metadata[sources.MetadataAttachmentID]),
			"checksum":                        stringValue(metadata[sources.MetadataContentHash]),
			"sequence":                        sequence,
			"res_model":                       chunk.Ref.Model,
			"res_id":                          chunk.Ref.ID,
			"content":                         chunk.Content,
			"chunk_index":                     int64Value(metadata[sources.MetadataChunkIndex]),
			"embedding_model":                 chunk.EmbeddingModel,
			"has_embedding_generation_failed": false,
			"embedding_vector":                string(vector),
			"metadata":                        string(metadataJSON),
			"company_id":                      chunk.Ref.CompanyID,
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (r PersistedRetriever) Retrieve(ctx context.Context, req Request) ([]RetrievedChunk, error) {
	if r.Env == nil || r.Provider == nil || strings.TrimSpace(req.Prompt) == "" {
		return nil, nil
	}
	model := firstNonEmpty(r.EmbeddingModel, embeddingModelFrom(req.Metadata))
	if model == "" {
		return nil, nil
	}
	response, err := r.Provider.Embed(ctx, providers.EmbeddingRequest{Model: model, Content: []string{req.Prompt}})
	if err != nil {
		return nil, err
	}
	if len(response.Vectors) == 0 {
		return nil, nil
	}
	chunks, err := r.loadChunks()
	if err != nil {
		return nil, err
	}
	metadata := cloneMetadata(req.Metadata)
	metadata[sources.MetadataEmbeddingModel] = model
	if req.Agent.ID != 0 && !req.AllowNoAgent {
		metadata[sources.MetadataAgentID] = req.Agent.ID
	}
	if req.CompanyID != 0 {
		metadata[sources.MetadataCompanyID] = req.CompanyID
	}
	limit := r.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	store := embeddings.NewStore(chunks...)
	candidates := store.Search(ctx, response.Vectors[0], candidateLimit(limit), r.Authorizer, metadata)
	return chunksFromCandidates(candidates, sourceSet(firstNonEmptyIDs(req.SourceIDs, req.Agent.SourceIDs)), limit), nil
}

func (r PersistedRetriever) loadChunks() ([]embeddings.Chunk, error) {
	found, err := r.Env.Model(ModelEmbedding).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "agent_source_id", "attachment_id", "res_model", "res_id", "content", "embedding_model", "has_embedding_generation_failed", "embedding_vector", "metadata", "company_id")
	if err != nil {
		return nil, err
	}
	chunks := make([]embeddings.Chunk, 0, len(rows))
	for _, row := range rows {
		if boolValue(row["has_embedding_generation_failed"]) {
			continue
		}
		vector, ok, err := vectorValue(row["embedding_vector"])
		if err != nil {
			return nil, fmt.Errorf("ai.embedding %d vector: %w", int64Value(row["id"]), err)
		}
		if !ok {
			continue
		}
		metadata, err := metadataValue(row["metadata"])
		if err != nil {
			return nil, fmt.Errorf("ai.embedding %d metadata: %w", int64Value(row["id"]), err)
		}
		mergeDefault(metadata, sources.MetadataSourceID, row["agent_source_id"])
		mergeDefault(metadata, sources.MetadataAttachmentID, row["attachment_id"])
		mergeDefault(metadata, sources.MetadataRecordModel, row["res_model"])
		mergeDefault(metadata, sources.MetadataRecordID, row["res_id"])
		mergeDefault(metadata, sources.MetadataCompanyID, row["company_id"])
		mergeDefault(metadata, sources.MetadataEmbeddingModel, row["embedding_model"])
		normalizeNumericMetadata(metadata, sources.MetadataSourceID, sources.MetadataAgentID, sources.MetadataCompanyID, sources.MetadataRecordID, sources.MetadataAttachmentID)
		chunks = append(chunks, embeddings.Chunk{
			ID:             int64Value(row["id"]),
			SourceID:       int64Value(metadata[sources.MetadataSourceID]),
			Ref:            embeddings.RecordRef{Model: stringValue(metadata[sources.MetadataRecordModel]), ID: int64Value(metadata[sources.MetadataRecordID]), CompanyID: int64Value(metadata[sources.MetadataCompanyID])},
			Content:        stringValue(row["content"]),
			Vector:         vector,
			EmbeddingModel: stringValue(row["embedding_model"]),
			Metadata:       metadata,
		})
	}
	return chunks, nil
}

func vectorValue(value any) ([]float64, bool, error) {
	switch typed := value.(type) {
	case nil:
		return nil, false, nil
	case []float64:
		if len(typed) == 0 {
			return nil, false, nil
		}
		return append([]float64(nil), typed...), true, nil
	case []any:
		out := make([]float64, 0, len(typed))
		for _, item := range typed {
			switch number := item.(type) {
			case float64:
				out = append(out, number)
			case int:
				out = append(out, float64(number))
			case int64:
				out = append(out, float64(number))
			default:
				return nil, false, fmt.Errorf("unsupported vector item %T", item)
			}
		}
		return out, len(out) > 0, nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false, nil
		}
		var out []float64
		if err := json.Unmarshal([]byte(typed), &out); err != nil {
			return nil, false, err
		}
		return out, len(out) > 0, nil
	default:
		return nil, false, fmt.Errorf("unsupported vector type %T", value)
	}
}

func metadataValue(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return cloneMetadata(typed), nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return map[string]any{}, nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(typed), &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported metadata type %T", value)
	}
}

func mergeDefault(metadata map[string]any, key string, value any) {
	if metadata[key] != nil {
		return
	}
	if value == nil {
		return
	}
	if stringValue(value) == "" {
		return
	}
	metadata[key] = value
}

func normalizeNumericMetadata(metadata map[string]any, keys ...string) {
	for _, key := range keys {
		if metadata[key] == nil {
			continue
		}
		metadata[key] = int64Value(metadata[key])
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
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
