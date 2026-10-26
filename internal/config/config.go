// Package config loads runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the application's runtime configuration.
type Config struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string

	EmbeddingProvider   string
	EmbeddingModel      string
	EmbeddingAPIKey     string
	EmbeddingBaseURL    string
	EmbeddingDimensions int

	DatabaseURL string

	MaxTokens      int
	Temperature    float64
	MaxToolHops    int
	RequestTimeout time.Duration

	ChunkSize    int
	ChunkOverlap int
}

const (
	defaultProvider          = "ollama"
	defaultEmbeddingProvider = "ollama"
	defaultEmbeddingModel    = "nomic-embed-text"
	defaultEmbeddingDim      = 768
	defaultDatabaseURL       = "postgres://postgres:examgen@localhost:5432/examgen?sslmode=disable"
	defaultMaxTokens         = 8192
	defaultTemperature       = 0.4
	defaultMaxToolHops       = 24
	defaultRequestTimeout    = 20 * time.Minute
	defaultChunkSize         = 1000
	defaultChunkOverlap      = 200
)

// Load reads configuration from the environment.
func Load() (*Config, error) {
	var l loader

	cfg := &Config{
		Provider: l.str("EXAMGEN_PROVIDER", defaultProvider),
		Model:    l.str("EXAMGEN_MODEL", ""),
		APIKey:   l.str("EXAMGEN_API_KEY", ""),
		BaseURL:  l.str("EXAMGEN_BASE_URL", ""),

		EmbeddingProvider:   l.str("EXAMGEN_EMBEDDING_PROVIDER", defaultEmbeddingProvider),
		EmbeddingModel:      l.str("EXAMGEN_EMBEDDING_MODEL", defaultEmbeddingModel),
		EmbeddingAPIKey:     l.str("EXAMGEN_EMBEDDING_API_KEY", ""),
		EmbeddingBaseURL:    l.str("EXAMGEN_EMBEDDING_BASE_URL", ""),
		EmbeddingDimensions: l.int("EXAMGEN_EMBEDDING_DIM", defaultEmbeddingDim),

		DatabaseURL: l.str("EXAMGEN_DATABASE_URL", defaultDatabaseURL),

		MaxTokens:      l.int("EXAMGEN_MAX_TOKENS", defaultMaxTokens),
		Temperature:    l.float("EXAMGEN_TEMPERATURE", defaultTemperature),
		MaxToolHops:    l.int("EXAMGEN_MAX_TOOL_HOPS", defaultMaxToolHops),
		RequestTimeout: l.duration("EXAMGEN_REQUEST_TIMEOUT", defaultRequestTimeout),

		ChunkSize:    l.int("EXAMGEN_CHUNK_SIZE", defaultChunkSize),
		ChunkOverlap: l.int("EXAMGEN_CHUNK_OVERLAP", defaultChunkOverlap),
	}

	if err := cfg.Validate(); err != nil {
		l.errs = append(l.errs, err)
	}

	if len(l.errs) > 0 {
		return nil, errors.Join(l.errs...)
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var errs []error

	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))

	}
	if c.Provider != "ollama" && c.Provider != "nvidia" && c.Provider != "openai" {
		fail("EXAMGEN_PROVIDER must be one of ollama, nvidia, or openai, got %q", c.Provider)
	}
	if c.EmbeddingProvider != "ollama" && c.EmbeddingProvider != "openai" {
		fail("EXAMGEN_EMBEDDING_PROVIDER must be ollama or openai, got %q", c.EmbeddingProvider)
	}
	if c.EmbeddingModel == "" {
		fail("EXAMGEN_EMBEDDING_MODEL must not be empty")
	}
	if c.DatabaseURL == "" {
		fail("EXAMGEN_DATABASE_URL must not be empty")
	}
	if c.EmbeddingDimensions <= 0 {
		fail("EXAMGEN_EMBEDDING_DIM must be positive, got %d", c.EmbeddingDimensions)
	}
	if c.MaxTokens <= 0 {
		fail("EXAMGEN_MAX_TOKENS must be positive, got %d", c.MaxTokens)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		fail("EXAMGEN_TEMPERATURE must be in [0, 2], got %g", c.Temperature)
	}
	if c.MaxToolHops <= 0 {
		fail("EXAMGEN_MAX_TOOL_HOPS must be positive, got %d", c.MaxToolHops)
	}
	if c.RequestTimeout < 0 {
		fail("EXAMGEN_REQUEST_TIMEOUT must not be negative, got %s", c.RequestTimeout)
	}
	if c.ChunkSize <= 0 {
		fail("EXAMGEN_CHUNK_SIZE must be positive, got %d", c.ChunkSize)
	}
	if c.ChunkOverlap < 0 || c.ChunkOverlap >= c.ChunkSize {
		fail("EXAMGEN_CHUNK_OVERLAP must be in [0, EXAMGEN_CHUNK_SIZE), got %d with chunk size %d",
			c.ChunkOverlap, c.ChunkSize)
	}

	return errors.Join(errs...)
}

// ValidateGeneration checks settings needed only for generation.
func (c *Config) ValidateGeneration() error {
	var errs []error
	if c.Model == "" {
		errs = append(errs, errors.New("EXAMGEN_MODEL is required (a model supporting both images and tool calls)"))
	}
	if (c.Provider == "nvidia" || c.Provider == "openai") && c.APIKey == "" {
		errs = append(errs, fmt.Errorf("EXAMGEN_API_KEY is required for provider %q", c.Provider))
	}
	return errors.Join(errs...)
}

type loader struct {
	errs []error
}

func (l *loader) str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func (l *loader) int(key string, def int) int {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return n
}

func (l *loader) float(key string, def float64) float64 {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return f
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	secs, err := strconv.Atoi(raw)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: invalid duration %q (want %q, %q, or bare seconds)", key, raw, "20m", "90s"))
		return def
	}
	return time.Duration(secs) * time.Second
}
