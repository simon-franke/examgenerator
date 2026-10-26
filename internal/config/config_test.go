package config

import (
	"strings"
	"testing"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	keys := []string{
		"EXAMGEN_PROVIDER", "EXAMGEN_MODEL", "EXAMGEN_API_KEY", "EXAMGEN_BASE_URL",
		"EXAMGEN_EMBEDDING_PROVIDER", "EXAMGEN_EMBEDDING_MODEL", "EXAMGEN_EMBEDDING_API_KEY",
		"EXAMGEN_EMBEDDING_BASE_URL", "EXAMGEN_EMBEDDING_DIM", "EXAMGEN_DATABASE_URL",
		"EXAMGEN_MAX_TOKENS", "EXAMGEN_TEMPERATURE", "EXAMGEN_MAX_TOOL_HOPS",
		"EXAMGEN_REQUEST_TIMEOUT", "EXAMGEN_CHUNK_SIZE", "EXAMGEN_CHUNK_OVERLAP",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestLoadAllowsIngestWithoutGenerationModel(t *testing.T) {
	clearConfigEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "" {
		t.Fatalf("model = %q", cfg.Model)
	}
	if err := cfg.ValidateGeneration(); err == nil {
		t.Fatal("generation validation should require a model")
	}
}

func TestLoadReturnsValidationErrors(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("EXAMGEN_CHUNK_SIZE", "10")
	t.Setenv("EXAMGEN_CHUNK_OVERLAP", "10")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "EXAMGEN_CHUNK_OVERLAP") {
		t.Fatalf("error = %v", err)
	}
}
