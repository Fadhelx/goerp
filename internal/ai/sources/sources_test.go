package sources

import "testing"

func TestSourceHashChunkAndEmbeddingNeed(t *testing.T) {
	source, err := New(Source{Name: "Policy", Type: TypeText, Content: "abcdef", State: StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if source.ContentHash == "" {
		t.Fatal("missing content hash")
	}
	previous := source
	previous.ContentHash = HashContent("old")
	if !source.NeedsEmbedding(previous) {
		t.Fatal("content change should require embedding")
	}
	chunks := Chunk("abcdef", 2)
	if len(chunks) != 3 || chunks[0] != "ab" || chunks[2] != "ef" {
		t.Fatalf("chunks = %+v", chunks)
	}
}
