// Package rag provides slide-search tools for generation.
package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/simon-franke/examgenerator/internal/embedding"
	"github.com/simon-franke/examgenerator/internal/llm"
	"github.com/simon-franke/examgenerator/internal/store"
)

const (
	defaultTopK = 5
	maxTopK     = 20
)

// Handler executes retrieval tool calls.
type Handler struct {
	store    store.VectorStore
	embedder embedding.Embedder
}

func NewHandler(s store.VectorStore, e embedding.Embedder) *Handler {
	return &Handler{store: s, embedder: e}
}

func Tools() []llm.Tool {
	return []llm.Tool{{
		Name:        "search_slides",
		Description: "Search the ingested lecture slides for material to use in an exam question.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "A topic or concept to find in the lecture slides.",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "Number of relevant chunks to return (1-20).",
					"minimum":     1,
					"maximum":     maxTopK,
					"default":     defaultTopK,
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}}
}

func (h *Handler) Handle(ctx context.Context, call llm.ToolCall) (string, error) {
	if call.Name != "search_slides" {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	var input struct {
		Query string `json:"query"`
		TopK  *int   `json:"top_k"`
	}
	if len(call.Input) == 0 {
		return "", errors.New("search_slides input is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(string(call.Input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return "", fmt.Errorf("decode search_slides input: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("decode search_slides input: multiple JSON values")
		}
		return "", fmt.Errorf("decode search_slides input: %w", err)
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return "", errors.New("search_slides query must not be empty")
	}
	topK := defaultTopK
	if input.TopK != nil {
		topK = *input.TopK
	}
	if topK <= 0 {
		return "", fmt.Errorf("search_slides top_k must be positive, got %d", topK)
	}
	if topK > maxTopK {
		topK = maxTopK
	}
	return h.searchSlides(ctx, input.Query, topK)
}

func (h *Handler) searchSlides(ctx context.Context, query string, topK int) (string, error) {
	embeddings, err := h.embedder.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("embed slide search query: %w", err)
	}
	if len(embeddings) != 1 {
		return "", fmt.Errorf("query embedder returned %d vectors, want 1", len(embeddings))
	}
	results, err := h.store.Search(ctx, embeddings[0], topK)
	if err != nil {
		return "", fmt.Errorf("search slide corpus: %w", err)
	}
	if len(results) == 0 {
		return "No relevant slide content was found for this query.", nil
	}
	var output strings.Builder
	for index, result := range results {
		if index > 0 {
			output.WriteString("\n\n")
		}
		fmt.Fprintf(&output, "[Result %d | source: %s | page: %d | similarity: %.3f]\n%s",
			index+1, result.Source, result.Page, result.Score, strings.TrimSpace(result.Content))
	}
	return output.String(), nil
}
