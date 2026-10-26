package ingest

import (
	"context"
	"reflect"
	"testing"

	"github.com/simon-franke/examgenerator/internal/pdf"
	"github.com/simon-franke/examgenerator/internal/store"
)

type fakeExtractor struct{ pages []pdf.Page }

func (f fakeExtractor) Extract(context.Context, string) ([]pdf.Page, error) { return f.pages, nil }

type fakeEmbedder struct{ texts []string }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.texts = append([]string(nil), texts...)
	vectors := make([][]float32, len(texts))
	for index := range vectors {
		vectors[index] = []float32{float32(index)}
	}
	return vectors, nil
}
func (f *fakeEmbedder) Dimensions() int { return 1 }

type fakeStore struct {
	source string
	chunks []store.Chunk
}

func (f *fakeStore) Init(context.Context) error                    { return nil }
func (f *fakeStore) EnsureSchemaAndNotEmpty(context.Context) error { return nil }
func (f *fakeStore) Store(context.Context, []store.Chunk) error    { return nil }
func (f *fakeStore) Search(context.Context, []float32, int) ([]store.SearchResult, error) {
	return nil, nil
}
func (f *fakeStore) Clear(context.Context, string) error { return nil }
func (f *fakeStore) Close() error                        { return nil }
func (f *fakeStore) Replace(_ context.Context, source string, chunks []store.Chunk) error {
	f.source = source
	f.chunks = append([]store.Chunk(nil), chunks...)
	return nil
}

func TestIngestChunksAndReplacesSource(t *testing.T) {
	embedder := &fakeEmbedder{}
	storage := &fakeStore{}
	ingester := New(fakeExtractor{pages: []pdf.Page{{Number: 2, Text: "abcdefghij"}}}, embedder, storage, 6, 2)
	count, err := ingester.Ingest(context.Background(), "slides.pdf", "course")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || storage.source != "course" {
		t.Fatalf("count=%d source=%q", count, storage.source)
	}
	if got, want := embedder.texts, []string{"abcdef", "efghij"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}
	if storage.chunks[1].Page != 2 || !reflect.DeepEqual(storage.chunks[1].Embedding, []float32{1}) {
		t.Fatalf("stored chunk = %#v", storage.chunks[1])
	}
}
