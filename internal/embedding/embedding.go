// Package embedding turns text into vectors for RAG ingestion and retrieval.
package embedding

import (
	"context"
	"fmt"
	"strings"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

type ErrUnknownProvider struct {
	Provider string
}

func (e ErrUnknownProvider) Error() string {
	return fmt.Sprintf("unknown provider: %s", e.Provider)
}

// New creates an embedding client for provider.
func New(provider, apiKey, model, baseURL string, dims int) (Embedder, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("embedding model must not be empty")
	}
	if dims <= 0 {
		return nil, fmt.Errorf("embedding dimensions must be positive, got %d", dims)
	}
	switch provider {
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return NewOpenAICompat(apiKey, model, baseURL, dims), nil
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAICompat(apiKey, model, baseURL, dims), nil
	default:
		return nil, ErrUnknownProvider{Provider: provider}
	}
}
