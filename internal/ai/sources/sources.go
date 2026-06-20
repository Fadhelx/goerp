package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Type string

const (
	TypeAttachment Type = "attachment"
	TypeURL        Type = "url"
	TypeRecord     Type = "record"
	TypeText       Type = "text"
)

type State string

const (
	StateDraft      State = "draft"
	StateProcessing State = "processing"
	StateReady      State = "ready"
	StateFailed     State = "failed"
)

type Source struct {
	ID             int64
	AgentID        int64
	Name           string
	Type           Type
	Model          string
	RecordID       int64
	AttachmentID   int64
	URL            string
	Content        string
	ContentHash    string
	State          State
	EmbeddingModel string
	CompanyID      int64
	LastError      string
}

func New(source Source) (Source, error) {
	if strings.TrimSpace(source.Name) == "" {
		return Source{}, fmt.Errorf("source requires name")
	}
	if source.Type == "" {
		source.Type = TypeText
	}
	if source.State == "" {
		source.State = StateDraft
	}
	source.ContentHash = HashContent(source.Content)
	return source, nil
}

func (s Source) NeedsEmbedding(previous Source) bool {
	return s.State == StateReady && s.ContentHash != "" && (s.ContentHash != previous.ContentHash || s.EmbeddingModel != previous.EmbeddingModel)
}

func HashContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func Chunk(content string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 800
	}
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 {
		return nil
	}
	var chunks []string
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}
