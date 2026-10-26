package generate

import (
	"context"
	"errors"
	"testing"

	"github.com/simon-franke/examgenerator/internal/llm"
	"github.com/simon-franke/examgenerator/internal/pdf"
	"github.com/simon-franke/examgenerator/internal/rag"
	"github.com/simon-franke/examgenerator/internal/store"
)

type scriptedClient struct {
	requests []llm.Request
	replies  []*llm.Response
}

func (c *scriptedClient) Name() string { return "scripted" }
func (c *scriptedClient) Chat(_ context.Context, request llm.Request) (*llm.Response, error) {
	c.requests = append(c.requests, request)
	if len(c.replies) == 0 {
		return nil, errors.New("no scripted reply")
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return reply, nil
}

type generatorEmbedder struct{}

func (generatorEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1}}, nil
}
func (generatorEmbedder) Dimensions() int { return 1 }

type generatorStore struct{}

func (generatorStore) Init(context.Context) error                           { return nil }
func (generatorStore) EnsureSchemaAndNotEmpty(context.Context) error        { return nil }
func (generatorStore) Store(context.Context, []store.Chunk) error           { return nil }
func (generatorStore) Replace(context.Context, string, []store.Chunk) error { return nil }
func (generatorStore) Clear(context.Context, string) error                  { return nil }
func (generatorStore) Close() error                                         { return nil }
func (generatorStore) Search(context.Context, []float32, int) ([]store.SearchResult, error) {
	return []store.SearchResult{{Chunk: store.Chunk{Source: "deck", Page: 1, Content: "grounding"}, Score: 1}}, nil
}

func TestGenerateRunsToolLoop(t *testing.T) {
	client := &scriptedClient{replies: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "one", Name: "search_slides", Input: []byte(`{"query":"graphs"}`)}}, StopReason: "tool_use"},
		{Content: `\documentclass{exam}\begin{document}Q\end{document}`, StopReason: "stop"},
	}}
	handler := rag.NewHandler(generatorStore{}, generatorEmbedder{})
	document, err := New(client, handler, "model", 1000, 0.2, 3).Generate(context.Background(), []pdf.Image{{Number: 1, Mime: "image/png", Data: []byte("image")}})
	if err != nil {
		t.Fatal(err)
	}
	if document == "" || len(client.requests) != 2 {
		t.Fatalf("document=%q requests=%d", document, len(client.requests))
	}
	messages := client.requests[1].Messages
	if messages[len(messages)-1].Role != llm.RoleTool || messages[len(messages)-1].ToolCallID != "one" {
		t.Fatalf("tool result message = %#v", messages[len(messages)-1])
	}
}

func TestGenerateRequiresGrounding(t *testing.T) {
	client := &scriptedClient{replies: []*llm.Response{{Content: "latex", StopReason: "stop"}}}
	_, err := New(client, nil, "model", 1000, 0.2, 3).Generate(context.Background(), []pdf.Image{{Number: 1, Data: []byte("image")}})
	if err == nil {
		t.Fatal("expected generation without a tool call to fail")
	}
}
