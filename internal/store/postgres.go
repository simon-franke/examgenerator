package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is a pgvector-backed VectorStore.
type Postgres struct {
	pool       *pgxpool.Pool
	dimensions int
}

// NewPostgres connects to Postgres.
func NewPostgres(ctx context.Context, databaseURL string, dimensions int) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("new postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Postgres{
		pool:       pool,
		dimensions: dimensions,
	}, nil
}

// Init creates the vector extension and chunk schema.
func (p *Postgres) Init(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector;")
	if err != nil {
		return fmt.Errorf("create extension: %w", err)
	}

	_, err = p.pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS chunks (
			uuid		bigserial PRIMARY KEY,
			source		text NOT NULL,
			page_number	int NOT NULL,
			body 		text NOT NULL,
			embedding	vector(%d) NOT NULL
		)`, p.dimensions))
	if err != nil {
		return fmt.Errorf("create table %w", err)
	}

	_, err = p.pool.Exec(ctx, "CREATE INDEX IF NOT EXISTS chunks_vector_idx ON chunks USING hnsw (embedding vector_cosine_ops);")
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	_, err = p.pool.Exec(ctx, "CREATE INDEX IF NOT EXISTS chunks_sources_idx ON chunks (source);")
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	return nil
}

// EnsureSchemaAndNotEmpty initializes storage and checks for chunks.
func (p *Postgres) EnsureSchemaAndNotEmpty(ctx context.Context) error {
	if err := p.Init(ctx); err != nil {
		return err
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM chunks LIMIT 1)").Scan(&exists); err != nil {
		return fmt.Errorf("check slide corpus: %w", err)
	}
	if !exists {
		return errors.New("slide corpus is empty; run `examgen ingest` before generating an exam")
	}
	return nil
}

func (p *Postgres) Store(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin storing chunks: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := p.storeChunks(ctx, tx, chunks); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chunks: %w", err)
	}
	return nil
}

func (p *Postgres) Replace(ctx context.Context, source string, chunks []Chunk) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source must not be empty")
	}
	for index := range chunks {
		if chunks[index].Source != source {
			return fmt.Errorf("chunk %d belongs to source %q, want %q", index, chunks[index].Source, source)
		}
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replacing source: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "DELETE FROM chunks WHERE source = $1", source); err != nil {
		return fmt.Errorf("clear source %q: %w", source, err)
	}
	if err := p.storeChunks(ctx, tx, chunks); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source replacement: %w", err)
	}
	return nil
}

func (p *Postgres) Search(ctx context.Context, embedding []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		return nil, fmt.Errorf("topK must be positive, got %d", topK)
	}
	if err := p.validateVector(embedding); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT uuid, source, page_number, body, 1 - (embedding <=> $1::vector) AS score
		FROM chunks
		ORDER BY embedding <=> $1::vector
		LIMIT $2`, encodeVector(embedding), topK)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, topK)
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ID, &result.Source, &result.Page, &result.Content, &result.Score); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read search results: %w", err)
	}
	return results, nil
}

func (p *Postgres) Clear(ctx context.Context, source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source must not be empty")
	}
	if _, err := p.pool.Exec(ctx, "DELETE FROM chunks WHERE source = $1", source); err != nil {
		return fmt.Errorf("clear source %q: %w", source, err)
	}
	return nil
}

func (p *Postgres) Close() error {
	p.pool.Close()
	return nil
}

func encodeVector(vector []float32) string {
	var encoded strings.Builder
	encoded.Grow(len(vector) * 10)
	encoded.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			encoded.WriteByte(',')
		}
		encoded.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	encoded.WriteByte(']')
	return encoded.String()
}

func (p *Postgres) validateVector(vector []float32) error {
	if len(vector) != p.dimensions {
		return fmt.Errorf("embedding has %d dimensions, want %d", len(vector), p.dimensions)
	}
	for index, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("embedding contains a non-finite value at index %d", index)
		}
	}
	return nil
}

type chunkExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (p *Postgres) storeChunks(ctx context.Context, executor chunkExecutor, chunks []Chunk) error {
	for index, chunk := range chunks {
		if strings.TrimSpace(chunk.Source) == "" {
			return fmt.Errorf("chunk %d has an empty source", index)
		}
		if chunk.Page <= 0 {
			return fmt.Errorf("chunk %d has invalid page %d", index, chunk.Page)
		}
		if err := p.validateVector(chunk.Embedding); err != nil {
			return fmt.Errorf("chunk %d: %w", index, err)
		}
		_, err := executor.Exec(ctx, `
			INSERT INTO chunks (source, page_number, body, embedding)
			VALUES ($1, $2, $3, $4::vector)`,
			chunk.Source, chunk.Page, chunk.Content, encodeVector(chunk.Embedding))
		if err != nil {
			return fmt.Errorf("insert chunk %d: %w", index, err)
		}
	}
	return nil
}

var _ VectorStore = (*Postgres)(nil)
