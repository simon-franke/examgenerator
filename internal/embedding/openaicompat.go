package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAICompat calls an OpenAI-compatible embeddings endpoint.
type OpenAICompat struct {
	apiKey  string
	model   string
	baseURL string
	dims    int
	http    *http.Client
}

func NewOpenAICompat(apiKey, model, baseURL string, dims int) *OpenAICompat {
	return &OpenAICompat{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		dims:    dims,
		http:    http.DefaultClient,
	}
}

type OpenAICompatRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type OpenAICompatResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (o *OpenAICompat) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	url := fmt.Sprintf("%s/embeddings", o.baseURL)

	req := OpenAICompatRequest{
		Model: o.model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("embedding request returned %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}

	var ollamaResp OpenAICompatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(ollamaResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response count mismatch: got %d, want %d", len(ollamaResp.Data), len(texts))
	}
	embeddings := make([][]float32, len(texts))
	for _, data := range ollamaResp.Data {
		if data.Index < 0 || data.Index >= len(texts) {
			return nil, fmt.Errorf("embedding response index %d is out of range", data.Index)
		}
		if len(data.Embedding) != o.dims {
			return nil, fmt.Errorf("embedding %d has %d dimensions, want %d", data.Index, len(data.Embedding), o.dims)
		}
		if embeddings[data.Index] != nil {
			return nil, fmt.Errorf("embedding response repeats index %d", data.Index)
		}
		embeddings[data.Index] = data.Embedding
	}

	return embeddings, nil
}

func (o *OpenAICompat) Dimensions() int { return o.dims }
