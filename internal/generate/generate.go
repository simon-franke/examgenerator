// Package generate produces an exam from rendered pages and slide retrieval.
package generate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/simon-franke/examgenerator/internal/llm"
	"github.com/simon-franke/examgenerator/internal/pdf"
	"github.com/simon-franke/examgenerator/internal/rag"
	"github.com/simon-franke/examgenerator/prompts"
)

// Generator coordinates model and retrieval calls.
type Generator struct {
	client      llm.Client
	tools       *rag.Handler
	model       string
	maxTokens   int
	temperature float64
	maxToolHops int
	progressf   func(format string, args ...any)
	debug       io.Writer
}

func (g *Generator) SetProgress(progressf func(format string, args ...any)) {
	g.progressf = progressf
}

func (g *Generator) SetDebug(w io.Writer) { g.debug = w }

func New(client llm.Client, tools *rag.Handler, model string, maxTokens int, temperature float64, maxToolHops int) *Generator {
	return &Generator{
		client:      client,
		tools:       tools,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		maxToolHops: maxToolHops,
	}
}

// Generate returns the new exam as LaTeX.
func (g *Generator) Generate(ctx context.Context, examPages []pdf.Image) (string, error) {
	if len(examPages) == 0 {
		return "", errors.New("old exam contains no rendered pages")
	}
	if g.client == nil {
		return "", errors.New("LLM client is nil")
	}
	if g.maxToolHops <= 0 {
		return "", fmt.Errorf("max tool hops must be positive, got %d", g.maxToolHops)
	}

	images := make([]llm.Image, 0, len(examPages))
	for _, page := range examPages {
		if len(page.Data) == 0 {
			return "", fmt.Errorf("rendered exam page %d is empty", page.Number)
		}
		images = append(images, llm.Image{Mime: page.Mime, Data: page.Data})
	}
	messages := []llm.Message{{
		Role:    llm.RoleUser,
		Content: "These images show the complete reference exam in page order. Follow the system instructions to write a new one.",
		Images:  images,
	}}

	completedCalls := 0
	for round := 1; ; round++ {
		g.progress("model round %d", round)
		response, err := g.client.Chat(ctx, llm.Request{
			System:      prompts.Generate,
			Messages:    messages,
			Tools:       rag.Tools(),
			Model:       g.model,
			MaxTokens:   g.maxTokens,
			Temperature: g.temperature,
		})
		if err != nil {
			return "", fmt.Errorf("generate exam (round %d): %w", round, err)
		}
		if response == nil {
			return "", fmt.Errorf("generate exam (round %d): model returned a nil response", round)
		}
		g.trace("round %d: stop=%q content_bytes=%d tool_calls=%d\n", round, response.StopReason, len(response.Content), len(response.ToolCalls))
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		if len(response.ToolCalls) == 0 {
			if completedCalls == 0 {
				return "", errors.New("model produced a final answer without calling search_slides")
			}
			content := strings.TrimSpace(response.Content)
			if content == "" {
				return "", errors.New("model produced an empty final answer")
			}
			return content, nil
		}
		if g.tools == nil {
			return "", errors.New("model requested a tool but the RAG handler is nil")
		}
		if completedCalls+len(response.ToolCalls) > g.maxToolHops {
			return "", fmt.Errorf("model exceeded the limit of %d tool calls", g.maxToolHops)
		}
		for _, call := range response.ToolCalls {
			if strings.TrimSpace(call.ID) == "" {
				return "", fmt.Errorf("model returned tool call %q without an ID", call.Name)
			}
			g.progress("searching slides (%d/%d)", completedCalls+1, g.maxToolHops)
			g.trace("tool call %s %s: %s\n", call.ID, call.Name, string(call.Input))
			result, err := g.tools.Handle(ctx, call)
			if err != nil {
				return "", fmt.Errorf("handle tool call %q: %w", call.Name, err)
			}
			g.trace("tool result %s: %s\n", call.ID, result)
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: call.ID,
				ToolResult: result,
			})
			completedCalls++
		}
	}
}

func (g *Generator) progress(format string, args ...any) {
	if g.progressf != nil {
		g.progressf(format, args...)
	}
}

func (g *Generator) trace(format string, args ...any) {
	if g.debug != nil {
		_, _ = fmt.Fprintf(g.debug, format, args...)
	}
}
