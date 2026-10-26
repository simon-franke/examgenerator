package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompat is a chat adapter for OpenAI-compatible endpoints.
type OpenAICompat struct {
	apiKey  string
	model   string
	baseURL string
	timeout time.Duration
	http    *http.Client
}

// NewOpenAICompat creates a chat adapter.
func NewOpenAICompat(apiKey, model, baseURL string, timeout time.Duration) *OpenAICompat {
	return &OpenAICompat{
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		timeout: timeout,
		http:    &http.Client{},
	}
}

func (o *OpenAICompat) Name() string { return "openai-compatible" }

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	} `json:"function"`
}

type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (o *OpenAICompat) Chat(ctx context.Context, req Request) (*Response, error) {
	if o.baseURL == "" {
		return nil, errors.New("LLM base URL must not be empty")
	}
	model := req.Model
	if model == "" {
		model = o.model
	}
	temperature := req.Temperature
	body, err := json.Marshal(oaiRequest{
		Model:       model,
		Messages:    toOpenAIMessages(req),
		Tools:       toOpenAITools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: &temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if o.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	httpResp, err := o.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
		return nil, fmt.Errorf("chat request returned %s: %s", httpResp.Status, strings.TrimSpace(string(message)))
	}

	var decoded oaiResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("chat API error (%s): %s", decoded.Error.Type, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return nil, errors.New("chat response contained no choices")
	}
	choice := decoded.Choices[0]
	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: json.RawMessage(call.Function.Arguments),
		})
	}
	stopReason := choice.FinishReason
	if len(toolCalls) > 0 {
		stopReason = "tool_use"
	}
	content, _ := choice.Message.Content.(string)
	return &Response{Content: content, ToolCalls: toolCalls, StopReason: stopReason}, nil
}

func toOpenAIMessages(req Request) []oaiMessage {
	messages := make([]oaiMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, oaiMessage{Role: "system", Content: req.System})
	}
	for _, message := range req.Messages {
		mapped := oaiMessage{Role: string(message.Role)}
		switch message.Role {
		case RoleUser:
			if len(message.Images) == 0 {
				mapped.Content = message.Content
			} else {
				parts := make([]oaiContentPart, 0, len(message.Images)+1)
				if message.Content != "" {
					parts = append(parts, oaiContentPart{Type: "text", Text: message.Content})
				}
				for _, image := range message.Images {
					mime := image.Mime
					if mime == "" {
						mime = "image/png"
					}
					parts = append(parts, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{
						URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
					}})
				}
				mapped.Content = parts
			}
		case RoleAssistant:
			mapped.Content = message.Content
			for _, call := range message.ToolCalls {
				wireCall := oaiToolCall{ID: call.ID, Type: "function"}
				wireCall.Function.Name = call.Name
				wireCall.Function.Arguments = string(call.Input)
				mapped.ToolCalls = append(mapped.ToolCalls, wireCall)
			}
		case RoleTool:
			mapped.Content = message.ToolResult
			mapped.ToolCallID = message.ToolCallID
		default:
			mapped.Content = message.Content
		}
		messages = append(messages, mapped)
	}
	return messages
}

func toOpenAITools(tools []Tool) []oaiTool {
	mapped := make([]oaiTool, 0, len(tools))
	for _, tool := range tools {
		wireTool := oaiTool{Type: "function"}
		wireTool.Function.Name = tool.Name
		wireTool.Function.Description = tool.Description
		wireTool.Function.Parameters = tool.InputSchema
		mapped = append(mapped, wireTool)
	}
	return mapped
}
