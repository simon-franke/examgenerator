// Package llm defines chat and tool-use interfaces.
package llm

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Tool describes a function the model may call.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolCall is a request from the model to invoke a tool.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Image is an inline image message part.
type Image struct {
	Mime string
	Data []byte
}

// Message is a chat turn.
type Message struct {
	Role       Role
	Content    string
	Images     []Image
	ToolCalls  []ToolCall
	ToolCallID string
	ToolResult string
}

// Request is a single chat completion request.
type Request struct {
	System      string
	Messages    []Message
	Tools       []Tool
	Model       string
	MaxTokens   int
	Temperature float64
}

// Response is the model's reply.
type Response struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason string
}

// Client is a chat provider.
type Client interface {
	Name() string
	Chat(ctx context.Context, req Request) (*Response, error)
}
