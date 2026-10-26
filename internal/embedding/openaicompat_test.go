package embedding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEmbedPreservesResponseIndexes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[3,4],"index":1},{"embedding":[1,2],"index":0}]}`))
	}))
	defer server.Close()

	vectors, err := NewOpenAICompat("", "embed", server.URL, 2).Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]float32{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(vectors, want) {
		t.Fatalf("vectors = %#v, want %#v", vectors, want)
	}
}

func TestEmbedRejectsWrongDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1],"index":0}]}`))
	}))
	defer server.Close()

	if _, err := NewOpenAICompat("", "embed", server.URL, 2).Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected a dimension error")
	}
}
