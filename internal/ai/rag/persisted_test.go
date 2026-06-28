package rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	addonai "gorp/addons/ai"
	"gorp/internal/ai/agents"
	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/sources"
	"gorp/internal/record"
)

func TestStoreChunksPersistsEmbeddingRowsForRetrieval(t *testing.T) {
	env := newRAGEnv(t)
	ids, err := StoreChunks(env, []embeddings.Chunk{{
		ID:             1,
		SourceID:       10,
		Ref:            embeddings.RecordRef{Model: "ai.agent.source", ID: 10, CompanyID: 1},
		Content:        "stored source chunk",
		Vector:         []float64{1, 0},
		EmbeddingModel: "embed-a",
		Metadata: map[string]any{
			sources.MetadataSourceName:     "Stored Policy",
			sources.MetadataAgentID:        int64(2),
			sources.MetadataAttachmentID:   int64(100),
			sources.MetadataContentHash:    "checksum-a",
			sources.MetadataChunkIndex:     0,
			sources.MetadataEmbeddingModel: "embed-a",
			"is_active":                    true,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids = %+v", ids)
	}
	rows, err := env.Model(ModelEmbedding).Browse(ids...).Read("agent_source_id", "attachment_id", "checksum", "sequence", "embedding_vector", "metadata")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["agent_source_id"] != int64(10) || rows[0]["attachment_id"] != int64(100) || rows[0]["checksum"] != "checksum-a" || rows[0]["sequence"] != int64(1) {
		t.Fatalf("row = %+v", rows[0])
	}
	chunks, err := (PersistedRetriever{Env: env, Provider: &recordingProvider{vector: []float64{1, 0}}, EmbeddingModel: "embed-a"}).
		Retrieve(context.Background(), Request{Agent: agents.Agent{ID: 2, SourceIDs: []int64{10}}, CompanyID: 1, Prompt: "stored"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].SourceName != "Stored Policy" || chunks[0].Content != "stored source chunk" {
		t.Fatalf("chunks = %+v", chunks)
	}
}

func TestPersistedRetrieverLoadsAndFiltersEmbeddingRows(t *testing.T) {
	env := newRAGEnv(t)
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":                 int64(10),
		"attachment_id":                   int64(100),
		"res_model":                       "ai.agent.source",
		"res_id":                          int64(10),
		"content":                         "approved vendor policy",
		"embedding_model":                 "embed-a",
		"embedding_vector":                `[1,0]`,
		"company_id":                      int64(1),
		"has_embedding_generation_failed": false,
		"metadata": mustJSON(t, map[string]any{
			"source_name": "Policy",
			"source_url":  "https://example.test/policy",
			"agent_id":    int64(2),
			"is_active":   true,
		}),
	})
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":                 int64(11),
		"attachment_id":                   int64(101),
		"res_model":                       "ai.agent.source",
		"res_id":                          int64(11),
		"content":                         "other source",
		"embedding_model":                 "embed-a",
		"embedding_vector":                `[1,0]`,
		"company_id":                      int64(1),
		"has_embedding_generation_failed": false,
		"metadata":                        mustJSON(t, map[string]any{"source_name": "Other", "agent_id": int64(2), "is_active": true}),
	})
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":                 int64(10),
		"attachment_id":                   int64(102),
		"res_model":                       "ai.agent.source",
		"res_id":                          int64(10),
		"content":                         "wrong company",
		"embedding_model":                 "embed-a",
		"embedding_vector":                `[1,0]`,
		"company_id":                      int64(2),
		"has_embedding_generation_failed": false,
		"metadata":                        mustJSON(t, map[string]any{"source_name": "Wrong Company", "agent_id": int64(2), "is_active": true}),
	})
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":                 int64(10),
		"attachment_id":                   int64(103),
		"res_model":                       "ai.agent.source",
		"res_id":                          int64(10),
		"content":                         "failed embedding",
		"embedding_model":                 "embed-a",
		"embedding_vector":                `not-json-and-skipped`,
		"company_id":                      int64(1),
		"has_embedding_generation_failed": true,
		"metadata":                        mustJSON(t, map[string]any{"source_name": "Failed", "agent_id": int64(2), "is_active": true}),
	})

	provider := &recordingProvider{vector: []float64{1, 0}}
	chunks, err := (PersistedRetriever{
		Env:            env,
		Provider:       provider,
		Authorizer:     companyAuthorizer{companyID: 1},
		EmbeddingModel: "embed-a",
	}).Retrieve(context.Background(), Request{
		Agent:     agents.Agent{ID: 2, SourceIDs: []int64{10}},
		CompanyID: 1,
		Prompt:    "vendor policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || provider.requests[0].Content[0] != "vendor policy" {
		t.Fatalf("provider requests = %+v", provider.requests)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v", chunks)
	}
	if chunks[0].SourceID != 10 || chunks[0].AttachmentID != 100 || chunks[0].SourceName != "Policy" || chunks[0].URL != "https://example.test/policy" {
		t.Fatalf("chunk = %+v", chunks[0])
	}
}

func TestPersistedRetrieverHonorsAuthorizer(t *testing.T) {
	env := newRAGEnv(t)
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":                 int64(10),
		"attachment_id":                   int64(100),
		"res_model":                       "res.partner",
		"res_id":                          int64(42),
		"content":                         "private policy",
		"embedding_model":                 "embed-a",
		"embedding_vector":                `[1,0]`,
		"company_id":                      int64(1),
		"has_embedding_generation_failed": false,
		"metadata":                        mustJSON(t, map[string]any{"source_name": "Private", "agent_id": int64(2), "is_active": true}),
	})

	chunks, err := (PersistedRetriever{
		Env:            env,
		Provider:       &recordingProvider{vector: []float64{1, 0}},
		Authorizer:     companyAuthorizer{companyID: 2},
		EmbeddingModel: "embed-a",
	}).Retrieve(context.Background(), Request{
		Agent:     agents.Agent{ID: 2, SourceIDs: []int64{10}},
		CompanyID: 1,
		Prompt:    "private policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Fatalf("unauthorized chunks = %+v", chunks)
	}
}

func TestPersistedRetrieverErrorsOnMalformedVector(t *testing.T) {
	env := newRAGEnv(t)
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":  int64(10),
		"content":          "bad vector",
		"embedding_model":  "embed-a",
		"embedding_vector": `not-json`,
		"metadata":         `{}`,
	})
	_, err := (PersistedRetriever{Env: env, Provider: &recordingProvider{vector: []float64{1, 0}}, EmbeddingModel: "embed-a"}).
		Retrieve(context.Background(), Request{Agent: agents.Agent{ID: 2}, Prompt: "query"})
	if err == nil || !strings.Contains(err.Error(), "vector") {
		t.Fatalf("err = %v", err)
	}
}

func TestPersistedRetrieverErrorsOnMalformedMetadata(t *testing.T) {
	env := newRAGEnv(t)
	createEmbeddingRow(t, env, map[string]any{
		"agent_source_id":  int64(10),
		"content":          "bad metadata",
		"embedding_model":  "embed-a",
		"embedding_vector": `[1,0]`,
		"metadata":         `{`,
	})
	_, err := (PersistedRetriever{Env: env, Provider: &recordingProvider{vector: []float64{1, 0}}, EmbeddingModel: "embed-a"}).
		Retrieve(context.Background(), Request{Agent: agents.Agent{ID: 2}, Prompt: "query"})
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("err = %v", err)
	}
}

func newRAGEnv(t *testing.T) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	if err := addonai.RegisterRecordModels(reg); err != nil {
		t.Fatal(err)
	}
	return record.NewEnv(reg, record.Context{UserID: 1, CompanyID: 1, CompanyIDs: []int64{1}})
}

func createEmbeddingRow(t *testing.T, env *record.Env, values map[string]any) {
	t.Helper()
	if _, err := env.Model(ModelEmbedding).Create(values); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
