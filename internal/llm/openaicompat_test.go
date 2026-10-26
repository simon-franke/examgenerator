package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatChatMapsImagesAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		var request oaiRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Model != "vision-model" {
			t.Errorf("model = %q", request.Model)
		}
		if len(request.Messages) != 2 {
			t.Errorf("message count = %d, want 2", len(request.Messages))
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		parts, ok := request.Messages[1].Content.([]any)
		if !ok || len(parts) != 2 {
			t.Errorf("multimodal content = %#v", request.Messages[1].Content)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		textPart, ok := parts[0].(map[string]any)
		if !ok || textPart["type"] != "text" || textPart["text"] != "exam" {
			t.Errorf("text part = %#v", parts[0])
		}
		imagePart, ok := parts[1].(map[string]any)
		imageURL, urlOK := imagePart["image_url"].(map[string]any)
		if !ok || !urlOK || imagePart["type"] != "image_url" || imageURL["url"] != "data:image/png;base64,cG5n" {
			t.Errorf("image part = %#v", parts[1])
		}
		if len(request.Tools) != 1 || request.Tools[0].Type != "function" || request.Tools[0].Function.Name != "search_slides" || request.Tools[0].Function.Parameters["type"] != "object" {
			t.Errorf("tools = %#v", request.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"search_slides","arguments":"{\"query\":\"trees\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompat("secret", "vision-model", server.URL+"/v1/", 0)
	response, err := client.Chat(context.Background(), Request{
		System: "system",
		Messages: []Message{{
			Role:    RoleUser,
			Content: "exam",
			Images:  []Image{{Mime: "image/png", Data: []byte("png")}},
		}},
		Tools: []Tool{{Name: "search_slides", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StopReason != "tool_use" || len(response.ToolCalls) != 1 {
		t.Fatalf("response = %#v", response)
	}
	if response.ToolCalls[0].ID != "call-1" || response.ToolCalls[0].Name != "search_slides" {
		t.Errorf("tool call = %#v", response.ToolCalls[0])
	}
	if got := string(response.ToolCalls[0].Input); got != `{"query":"trees"}` {
		t.Errorf("tool input = %s", got)
	}
}

func TestOpenAICompatChatReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := NewOpenAICompat("", "model", server.URL, 0).Chat(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected an HTTP error")
	}
	if !strings.Contains(err.Error(), "503 Service Unavailable") || !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("error = %v", err)
	}
}
