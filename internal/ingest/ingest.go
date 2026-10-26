// Package ingest turns lecture-slide PDFs into searchable, embedded chunks.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/simon-franke/examgenerator/internal/embedding"
	"github.com/simon-franke/examgenerator/internal/pdf"
	"github.com/simon-franke/examgenerator/internal/store"
)

// Ingester extracts, embeds, and stores slide content.
type Ingester struct {
	extractor pdf.Extractor
	embedder  embedding.Embedder
	store     store.VectorStore
	chunkSize int
	overlap   int
}

func New(e pdf.Extractor, emb embedding.Embedder, s store.VectorStore, chunkSize, overlap int) *Ingester {
	return &Ingester{
		extractor: e,
		embedder:  emb,
		store:     s,
		chunkSize: chunkSize,
		overlap:   overlap,
	}
}

func (i *Ingester) Ingest(ctx context.Context, path, source string) (int, error) {
	return i.IngestMany(ctx, []string{path}, source)
}

// IngestMany stores chunks from paths under one source.
func (i *Ingester) IngestMany(ctx context.Context, paths []string, source string) (int, error) {
	if len(paths) == 0 {
		return 0, errors.New("no slide PDFs supplied")
	}
	if strings.TrimSpace(source) == "" {
		return 0, errors.New("source must not be empty")
	}
	if i.chunkSize <= 0 || i.overlap < 0 || i.overlap >= i.chunkSize {
		return 0, fmt.Errorf("invalid chunk settings: size=%d overlap=%d", i.chunkSize, i.overlap)
	}

	var pending []store.Chunk
	var texts []string
	for _, path := range paths {
		pages, err := i.extractor.Extract(ctx, path)
		if err != nil {
			return 0, fmt.Errorf("extract %q: %w", path, err)
		}
		for _, page := range pages {
			for _, content := range chunkText(page.Text, i.chunkSize, i.overlap) {
				pending = append(pending, store.Chunk{Source: source, Page: page.Number, Content: content})
				texts = append(texts, content)
			}
		}
	}
	if len(texts) == 0 {
		return 0, errors.New("the supplied slide PDFs contain no extractable text")
	}

	embeddings, err := i.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed slide chunks: %w", err)
	}
	if len(embeddings) != len(pending) {
		return 0, fmt.Errorf("embedder returned %d vectors for %d chunks", len(embeddings), len(pending))
	}
	for index := range pending {
		pending[index].Embedding = embeddings[index]
	}
	if err := i.store.Replace(ctx, source, pending); err != nil {
		return 0, fmt.Errorf("replace slide chunks: %w", err)
	}
	return len(pending), nil
}

func chunkText(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}
	step := size - overlap
	chunks := make([]string, 0, (len(runes)+step-1)/step)
	for start := 0; start < len(runes); {
		end := min(start+size, len(runes))
		if end < len(runes) {
			end = preferredBreak(runes, start, end)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + step
		}
		start = next
	}
	return chunks
}

func preferredBreak(runes []rune, start, end int) int {
	minimum := start + (end-start)*3/4
	for index := end; index > minimum; index-- {
		if runes[index-1] == '\n' || runes[index-1] == ' ' || runes[index-1] == '\t' {
			return index
		}
	}
	return end
}
