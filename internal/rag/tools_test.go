package rag

import (
	"context"
	"testing"

	"github.com/simon-franke/examgenerator/internal/llm"
	"github.com/simon-franke/examgenerator/internal/store"
)

type testEmbedder struct{}

func (testEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 2}}, nil
}
func (testEmbedder) Dimensions() int { return 2 }

type testStore struct{ topK int }

func (s *testStore) Init(context.Context) error                           { return nil }
func (s *testStore) EnsureSchemaAndNotEmpty(context.Context) error        { return nil }
func (s *testStore) Store(context.Context, []store.Chunk) error           { return nil }
func (s *testStore) Replace(context.Context, string, []store.Chunk) error { return nil }
func (s *testStore) Clear(context.Context, string) error                  { return nil }
func (s *testStore) Close() error                                         { return nil }
func (s *testStore) Search(_ context.Context, _ []float32, topK int) ([]store.SearchResult, error) {
	s.topK = topK
	return []store.SearchResult{{Chunk: store.Chunk{Source: "week1", Page: 3, Content: "Balanced trees"}, Score: 0.75}}, nil
}

func TestHandleSearchSlidesCapsTopKAndFormatsResult(t *testing.T) {
	storage := &testStore{}
	result, err := NewHandler(storage, testEmbedder{}).Handle(context.Background(), llm.ToolCall{
		Name:  "search_slides",
		Input: []byte(`{"query":"trees","top_k":999}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if storage.topK != maxTopK {
		t.Fatalf("topK = %d", storage.topK)
	}
	if want := "[Result 1 | source: week1 | page: 3 | similarity: 0.750]\nBalanced trees"; result != want {
		t.Fatalf("result = %q, want %q", result, want)
	}
}

func TestHandleRejectsUnknownArguments(t *testing.T) {
	_, err := NewHandler(&testStore{}, testEmbedder{}).Handle(context.Background(), llm.ToolCall{
		Name:  "search_slides",
		Input: []byte(`{"query":"trees","extra":true}`),
	})
	if err == nil {
		t.Fatal("expected invalid input to fail")
	}
}
