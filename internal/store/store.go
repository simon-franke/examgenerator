// Package store defines vector storage.
package store

import "context"

// Chunk is embedded slide content.
type Chunk struct {
	ID        int64
	Source    string
	Page      int
	Content   string
	Embedding []float32
}

// SearchResult is a chunk with a similarity score.
type SearchResult struct {
	Chunk
	Score float64
}

// VectorStore stores and searches embedded chunks.
type VectorStore interface {
	Init(ctx context.Context) error
	EnsureSchemaAndNotEmpty(ctx context.Context) error
	Store(ctx context.Context, chunks []Chunk) error
	Replace(ctx context.Context, source string, chunks []Chunk) error
	Search(ctx context.Context, embedding []float32, topK int) ([]SearchResult, error)
	Clear(ctx context.Context, source string) error
	Close() error
}
