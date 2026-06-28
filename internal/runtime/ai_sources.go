package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	aiaddon "gorp/addons/ai"
	serveractions "gorp/internal/actions"
	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/rag"
	"gorp/internal/ai/sources"
	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/record"
)

const (
	aiProcessSourcesAction = "ai.process_sources"
	aiSourceFetchLimit     = 1 << 20
)

var aiSourceHTMLTagPattern = regexp.MustCompile(`<[^>]+>`)

func registerAISourceRuntimeActions(reg *serveractions.Registry, env *record.Env, app *App) error {
	if reg == nil || env == nil {
		return nil
	}
	return reg.RegisterGo(aiProcessSourcesAction, aiRuntimeProcessSources(env, app))
}

func aiRuntimeProcessSources(env *record.Env, app *App) serveractions.GoAction {
	return func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		startedAt := time.Now()
		runEnv := aiSourceSystemEnv(env)
		settings := aiRuntimeSettingsFromEnv(runEnv)
		resolver := app.aiProviderResolver(runEnv, settings)
		explicitEmbeddingModel := firstNonEmptyString(app.AIEmbeddingModel, settings.defaultEmbeddingModel)
		if resolver.registry == nil {
			return serveractions.Result{}, fmt.Errorf("ai provider registry is unavailable")
		}
		out, err := processAISources(ctx, runEnv, resolver, explicitEmbeddingModel, exec)
		persistAIAuditLog(runEnv, aiAuditLogEvent{
			EventType:        "source.process",
			UserID:           exec.UserID,
			CompanyID:        runEnv.Context().CompanyID,
			Model:            explicitEmbeddingModel,
			LatencyMillis:    time.Since(startedAt).Milliseconds(),
			PermissionResult: "allowed",
			Status:           aiAuditStatus(err),
			Error:            aiAuditErrorString(err),
			Metadata: map[string]any{
				"trigger":       exec.Trigger,
				"processed":     out.Processed,
				"ready":         out.Ready,
				"failed":        out.Failed,
				"skipped":       out.Skipped,
				"embedding_ids": out.EmbeddingIDs,
				"source_ids":    out.SourceIDs,
			},
		})
		return serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: aiProcessSourcesAction,
			Metadata: map[string]any{
				"processed":     out.Processed,
				"ready":         out.Ready,
				"failed":        out.Failed,
				"skipped":       out.Skipped,
				"embedding_ids": out.EmbeddingIDs,
				"source_ids":    out.SourceIDs,
			},
		}, err
	}
}

type aiSourceProcessSummary struct {
	Processed    int
	Ready        int
	Failed       int
	Skipped      int
	EmbeddingIDs []int64
	SourceIDs    []int64
	agentIDs     map[int64]bool
}

func processAISources(ctx context.Context, env *record.Env, resolver aiProviderResolver, explicitEmbeddingModel string, exec serveractions.ExecutionContext) (aiSourceProcessSummary, error) {
	summary := aiSourceProcessSummary{agentIDs: map[int64]bool{}}
	rows, err := aiSourceRows(env, exec)
	if err != nil {
		return summary, err
	}
	for index, row := range rows {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		sourceID := int64Value(row["id"])
		if sourceID == 0 || !boolWithFallback(row["is_active"], true) {
			continue
		}
		source, err := aiSourceFromRow(ctx, env, row)
		if err != nil {
			if writeErr := aiWriteSourceFailure(env, sourceID, err); writeErr != nil {
				return summary, writeErr
			}
			summary.Processed++
			summary.Failed++
			summary.SourceIDs = append(summary.SourceIDs, sourceID)
			continue
		}
		existing, err := aiSourceExistingChunks(env, sourceID)
		if err != nil {
			return summary, err
		}
		if source.ContentHash == "" {
			source.ContentHash = aiSourceStoredHash(existing)
		}
		provider, model, err := resolver.embeddingProvider("", firstNonEmptyString(source.EmbeddingModel, explicitEmbeddingModel))
		if err != nil {
			if writeErr := aiWriteSourceFailure(env, sourceID, err); writeErr != nil {
				return summary, writeErr
			}
			summary.Processed++
			summary.Failed++
			summary.SourceIDs = append(summary.SourceIDs, sourceID)
			continue
		}
		result, processErr := (sources.Processor{Provider: provider, EmbeddingModel: model}).Process(ctx, source, existing)
		summary.Processed++
		summary.SourceIDs = append(summary.SourceIDs, sourceID)
		if result.Source.AgentID != 0 {
			summary.agentIDs[result.Source.AgentID] = true
		}
		if processErr != nil {
			if writeErr := aiWriteProcessedSource(env, result.Source); writeErr != nil {
				return summary, writeErr
			}
			summary.Failed++
			continue
		}
		if result.Skipped {
			if writeErr := aiWriteProcessedSource(env, result.Source); writeErr != nil {
				return summary, writeErr
			}
			summary.Skipped++
			summary.Ready++
		} else {
			if err := aiReplaceSourceChunks(env, sourceID, result.Chunks); err != nil {
				return summary, err
			}
			ids, err := rag.StoreChunks(env, result.Chunks)
			if err != nil {
				return summary, err
			}
			if writeErr := aiWriteProcessedSource(env, result.Source); writeErr != nil {
				return summary, writeErr
			}
			summary.EmbeddingIDs = append(summary.EmbeddingIDs, ids...)
			summary.Ready++
		}
		remaining := len(rows) - index - 1
		if exec.Trigger == "cron" && exec.CommitProgress != nil {
			if !exec.CommitProgress(summary.Processed, &remaining, false) {
				break
			}
		}
	}
	if err := aiUpdateAgentSourceFlags(env, summary.agentIDs); err != nil {
		return summary, err
	}
	return summary, nil
}

func aiSourceRows(env *record.Env, exec serveractions.ExecutionContext) ([]map[string]any, error) {
	found, err := env.Model(aiaddon.ModelAgentSource).Search(domain.And())
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "name", "agent_id", "type", "source_type", "res_model", "res_id", "status", "is_active", "error_details", "attachment_id", "mimetype", "url", "content", "state", "embedding_model", "company_id")
	if err != nil {
		return nil, err
	}
	limit := int(int64Value(exec.Metadata["limit"]))
	if limit <= 0 {
		limit = 50
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func aiSourceFromRow(ctx context.Context, env *record.Env, row map[string]any) (sources.Source, error) {
	sourceID := int64Value(row["id"])
	sourceType := sources.Type(firstNonEmptyString(stringValue(row["type"]), stringValue(row["source_type"])))
	modelName := strings.TrimSpace(stringValue(row["res_model"]))
	recordID := int64Value(row["res_id"])
	attachmentID := int64Value(row["attachment_id"])
	content := strings.TrimSpace(stringValue(row["content"]))
	companyID := int64Value(row["company_id"])
	if content == "" && attachmentID != 0 {
		text, refModel, refID, refCompany, err := aiAttachmentContent(env, attachmentID)
		if err != nil {
			return sources.Source{}, err
		}
		content = text
		if modelName == "" && refModel != "" {
			modelName = refModel
			recordID = refID
		}
		if companyID == 0 {
			companyID = refCompany
		}
	}
	if content == "" && modelName != "" && recordID != 0 {
		text, err := aiRecordContent(env, modelName, recordID)
		if err != nil {
			return sources.Source{}, err
		}
		content = text
	}
	if content == "" && sourceType == sources.TypeURL {
		text, err := aiURLContent(ctx, stringValue(row["url"]))
		if err != nil {
			return sources.Source{}, err
		}
		content = text
	}
	if modelName == "" || recordID == 0 {
		modelName = aiaddon.ModelAgentSource
		recordID = sourceID
	}
	state := sources.State(firstNonEmptyString(stringValue(row["state"]), stringValue(row["status"])))
	if state == "" {
		state = sources.StateDraft
	}
	return sources.Source{
		ID:             sourceID,
		AgentID:        int64Value(row["agent_id"]),
		Name:           stringValue(row["name"]),
		Type:           sourceType,
		Model:          modelName,
		RecordID:       recordID,
		AttachmentID:   attachmentID,
		URL:            stringValue(row["url"]),
		Content:        content,
		State:          state,
		EmbeddingModel: strings.TrimSpace(stringValue(row["embedding_model"])),
		CompanyID:      companyID,
		LastError:      stringValue(row["error_details"]),
	}, nil
}

func aiAttachmentContent(env *record.Env, id int64) (string, string, int64, int64, error) {
	rows, err := env.Model("ir.attachment").Browse(id).Read("name", "datas", "url", "res_model", "res_id", "company_id")
	if err != nil {
		return "", "", 0, 0, err
	}
	if len(rows) == 0 {
		return "", "", 0, 0, fmt.Errorf("attachment %d not found", id)
	}
	row := rows[0]
	text := strings.TrimSpace(aiBinaryText(row["datas"]))
	if text == "" {
		text = strings.TrimSpace(stringValue(row["url"]))
	}
	if text == "" {
		text = strings.TrimSpace(stringValue(row["name"]))
	}
	return aiNormalizeText(text), strings.TrimSpace(stringValue(row["res_model"])), int64Value(row["res_id"]), int64Value(row["company_id"]), nil
}

func aiRecordContent(env *record.Env, modelName string, id int64) (string, error) {
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return "", fmt.Errorf("unknown model %s", modelName)
	}
	fields := make([]string, 0, len(meta.Fields))
	for name, fieldDef := range meta.Fields {
		switch fieldDef.Kind {
		case field.Char, field.Text, field.Selection:
			fields = append(fields, name)
		}
	}
	if len(fields) == 0 {
		return "", nil
	}
	rows, err := env.Model(modelName).Browse(id).Read(fields...)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("%s %d not found", modelName, id)
	}
	parts := make([]string, 0, len(fields))
	for _, name := range fields {
		text := strings.TrimSpace(stringValue(rows[0][name]))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return aiNormalizeText(strings.Join(parts, "\n")), nil
}

func aiURLContent(ctx context.Context, raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("ai source url must be http or https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ai source url returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, aiSourceFetchLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > aiSourceFetchLimit {
		return "", fmt.Errorf("ai source url content exceeds %d bytes", aiSourceFetchLimit)
	}
	return aiNormalizeText(string(data)), nil
}

func aiSourceExistingChunks(env *record.Env, sourceID int64) ([]embeddings.Chunk, error) {
	found, err := env.Model(aiaddon.ModelEmbedding).Search(domain.Cond("agent_source_id", domain.Equal, sourceID))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "agent_source_id", "attachment_id", "res_model", "res_id", "content", "embedding_model", "embedding_vector", "metadata", "company_id")
	if err != nil {
		return nil, err
	}
	chunks := make([]embeddings.Chunk, 0, len(rows))
	for _, row := range rows {
		vector, err := aiVectorValue(row["embedding_vector"])
		if err != nil {
			return nil, err
		}
		metadata, err := aiMetadataValue(row["metadata"])
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, embeddings.Chunk{
			ID:       int64Value(row["id"]),
			SourceID: int64Value(row["agent_source_id"]),
			Ref: embeddings.RecordRef{
				Model:     stringValue(row["res_model"]),
				ID:        int64Value(row["res_id"]),
				CompanyID: int64Value(row["company_id"]),
			},
			Content:        stringValue(row["content"]),
			Vector:         vector,
			EmbeddingModel: stringValue(row["embedding_model"]),
			Metadata:       metadata,
		})
	}
	return chunks, nil
}

func aiReplaceSourceChunks(env *record.Env, sourceID int64, chunks []embeddings.Chunk) error {
	found, err := env.Model(aiaddon.ModelEmbedding).Search(domain.Cond("agent_source_id", domain.Equal, sourceID))
	if err != nil {
		return err
	}
	if found.Len() > 0 {
		if err := found.Unlink(); err != nil {
			return err
		}
	}
	return nil
}

func aiWriteProcessedSource(env *record.Env, source sources.Source) error {
	return env.Model(aiaddon.ModelAgentSource).Browse(source.ID).Write(map[string]any{
		"state":           string(source.State),
		"status":          string(source.State),
		"error_details":   source.LastError,
		"embedding_model": source.EmbeddingModel,
	})
}

func aiWriteSourceFailure(env *record.Env, sourceID int64, err error) error {
	return env.Model(aiaddon.ModelAgentSource).Browse(sourceID).Write(map[string]any{
		"state":         string(sources.StateFailed),
		"status":        string(sources.StateFailed),
		"error_details": err.Error(),
	})
}

func aiUpdateAgentSourceFlags(env *record.Env, agentIDs map[int64]bool) error {
	for agentID := range agentIDs {
		found, err := env.Model(aiaddon.ModelAgentSource).Search(domain.Cond("agent_id", domain.Equal, agentID))
		if err != nil {
			return err
		}
		rows, err := found.Read("state", "status", "is_active")
		if err != nil {
			return err
		}
		allReady := len(rows) > 0
		for _, row := range rows {
			if !boolWithFallback(row["is_active"], true) {
				continue
			}
			state := firstNonEmptyString(stringValue(row["state"]), stringValue(row["status"]))
			if state != string(sources.StateReady) {
				allReady = false
				break
			}
		}
		if err := env.Model(aiaddon.ModelAgent).Browse(agentID).Write(map[string]any{"sources_fully_processed": allReady}); err != nil {
			return err
		}
	}
	return nil
}

func aiSourceStoredHash(chunks []embeddings.Chunk) string {
	for _, chunk := range chunks {
		if hash := strings.TrimSpace(stringValue(chunk.Metadata[sources.MetadataContentHash])); hash != "" {
			return hash
		}
	}
	return ""
}

func aiSourceSystemEnv(env *record.Env) *record.Env {
	if env == nil {
		return nil
	}
	ctx := env.Context()
	ctx.UserID = 1
	ctx.Sudo = true
	return env.WithContext(ctx)
}

func aiBinaryText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return ""
		}
		if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
			return string(decoded)
		}
		return text
	default:
		return stringValue(value)
	}
}

func aiNormalizeText(text string) string {
	text = aiSourceHTMLTagPattern.ReplaceAllString(text, " ")
	text = stdhtml.UnescapeString(text)
	return strings.Join(strings.Fields(text), " ")
}

func aiVectorValue(value any) ([]float64, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []float64:
		return append([]float64(nil), typed...), nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		var out []float64
		if err := json.Unmarshal([]byte(typed), &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported vector type %T", value)
	}
}

func aiMetadataValue(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return cloneStringAnyMap(typed), nil
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
