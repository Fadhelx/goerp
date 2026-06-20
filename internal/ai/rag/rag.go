package rag

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"gorp/internal/ai/agents"
	"gorp/internal/ai/embeddings"
	"gorp/internal/ai/providers"
	"gorp/internal/ai/sources"
)

const (
	DefaultLimit = 5

	RestrictToSourcesInstruction = "Use only the provided source context. If no source information is provided, say that no source information has been provided for reference. Cite source chunks when used."
)

type Searcher interface {
	Search(context.Context, []float64, int, embeddings.Authorizer, map[string]any) []embeddings.Candidate
}

type Retriever struct {
	Provider       providers.Provider
	Store          Searcher
	Authorizer     embeddings.Authorizer
	EmbeddingModel string
	Limit          int
}

type Request struct {
	Agent        agents.Agent
	UserID       int64
	CompanyID    int64
	Prompt       string
	SourceIDs    []int64
	Metadata     map[string]any
	AllowNoAgent bool
}

type RetrievedChunk struct {
	ID           int64
	SourceID     int64
	SourceName   string
	AttachmentID int64
	URL          string
	Content      string
	Score        float64
	Metadata     map[string]any
}

func (r Retriever) Retrieve(ctx context.Context, req Request) ([]RetrievedChunk, error) {
	if r.Provider == nil || r.Store == nil || strings.TrimSpace(req.Prompt) == "" {
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
	candidates := r.Store.Search(ctx, response.Vectors[0], candidateLimit(limit), r.Authorizer, metadata)
	allowedSources := sourceSet(firstNonEmptyIDs(req.SourceIDs, req.Agent.SourceIDs))
	return chunksFromCandidates(candidates, allowedSources, limit), nil
}

func candidateLimit(limit int) int {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit*4 < 100 {
		return 100
	}
	return limit * 4
}

func BuildContext(chunks []RetrievedChunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("##RAG context information:\n\n")
	for _, chunk := range chunks {
		name := strings.TrimSpace(chunk.SourceName)
		if name == "" {
			name = fmt.Sprintf("Source %d", chunk.SourceID)
		}
		b.WriteString("(Source Chunk ")
		b.WriteString(name)
		b.WriteString(")\n")
		if chunk.AttachmentID != 0 {
			b.WriteString(" (attachment_id: ")
			b.WriteString(strconv.FormatInt(chunk.AttachmentID, 10))
			b.WriteString(")\n")
		}
		b.WriteString(strings.TrimSpace(chunk.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

type CitationSource struct {
	Name string
	URL  string
}

func ApplyNumericCitations(text string, sourcesByAttachment map[int64]CitationSource) string {
	if strings.TrimSpace(text) == "" || len(sourcesByAttachment) == 0 {
		return text
	}
	var b strings.Builder
	pieces := citationPattern.FindAllStringSubmatchIndex(text, -1)
	if len(pieces) == 0 {
		return text
	}
	resolved := map[int64]int{}
	cursor := 0
	for _, match := range pieces {
		b.WriteString(strings.TrimRight(text[cursor:match[0]], " "))
		ids := text[match[2]:match[3]]
		for _, raw := range strings.Split(ids, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				continue
			}
			source, ok := sourcesByAttachment[id]
			if !ok {
				continue
			}
			number, ok := resolved[id]
			if !ok {
				number = len(resolved) + 1
				resolved[id] = number
			}
			b.WriteString(citationLink(number, source))
		}
		cursor = match[1]
	}
	b.WriteString(text[cursor:])
	return b.String()
}

func CitationSources(chunks []RetrievedChunk, baseURL string) map[int64]CitationSource {
	out := map[int64]CitationSource{}
	for _, chunk := range chunks {
		if chunk.AttachmentID == 0 {
			continue
		}
		if !boolDefaultTrue(chunk.Metadata["user_has_access"]) {
			continue
		}
		url := strings.TrimSpace(chunk.URL)
		if url == "" && strings.TrimSpace(baseURL) != "" {
			url = strings.TrimRight(baseURL, "/") + "/web/content/" + strconv.FormatInt(chunk.AttachmentID, 10)
		}
		out[chunk.AttachmentID] = CitationSource{Name: chunk.SourceName, URL: url}
	}
	return out
}

func chunksFromCandidates(candidates []embeddings.Candidate, allowedSources map[int64]bool, limit int) []RetrievedChunk {
	out := make([]RetrievedChunk, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if !boolDefaultTrue(candidate.Chunk.Metadata["source_active"]) || !boolDefaultTrue(candidate.Chunk.Metadata["is_active"]) {
			continue
		}
		sourceID := int64Value(candidate.Chunk.Metadata[sources.MetadataSourceID])
		if len(allowedSources) > 0 && !allowedSources[sourceID] {
			continue
		}
		out = append(out, RetrievedChunk{
			ID:           candidate.Chunk.ID,
			SourceID:     sourceID,
			SourceName:   stringValue(candidate.Chunk.Metadata[sources.MetadataSourceName]),
			AttachmentID: int64Value(candidate.Chunk.Metadata[sources.MetadataAttachmentID]),
			URL:          stringValue(candidate.Chunk.Metadata[sources.MetadataSourceURL]),
			Content:      candidate.Chunk.Content,
			Score:        candidate.Score,
			Metadata:     cloneMetadata(candidate.Chunk.Metadata),
		})
		if len(out) == limit {
			break
		}
	}
	return out
}

func citationLink(number int, source CitationSource) string {
	href := html.EscapeString(source.URL)
	title := html.EscapeString(source.Name)
	if title != "" {
		title = ` title="` + title + `"`
	}
	return fmt.Sprintf(`<sup><a href="%s" target="_blank" rel="noreferrer noopener"%s style="text-decoration: none;"> [%d] </a></sup>`, href, title, number)
}

func sourceSet(ids []int64) map[int64]bool {
	if len(ids) == 0 {
		return nil
	}
	out := map[int64]bool{}
	for _, id := range ids {
		if id != 0 {
			out[id] = true
		}
	}
	return out
}

func firstNonEmptyIDs(values ...[]int64) []int64 {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func embeddingModelFrom(metadata map[string]any) string {
	return stringValue(metadata[sources.MetadataEmbeddingModel])
}

func cloneMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func boolDefaultTrue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "", "1", "true", "yes":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return true
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

var citationPattern = regexp.MustCompile(`\[SOURCE:([0-9]+(?:\s*,\s*[0-9]+)*)\]`)
