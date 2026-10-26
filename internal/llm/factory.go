package llm

import (
	"fmt"
	"strings"
	"time"
)

// New creates a chat client for provider.
func New(provider, apiKey, model, baseURL string, timeout time.Duration) (Client, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model must not be empty")
	}
	if baseURL == "" {
		switch provider {
		case "ollama":
			baseURL = "http://localhost:11434/v1"
		case "nvidia":
			baseURL = "https://integrate.api.nvidia.com/v1"
		case "openai":
			baseURL = "https://api.openai.com/v1"
		default:
			return nil, fmt.Errorf("unknown LLM provider %q", provider)
		}
	}
	if provider != "ollama" && provider != "nvidia" && provider != "openai" {
		return nil, fmt.Errorf("unknown LLM provider %q", provider)
	}
	return NewOpenAICompat(apiKey, model, baseURL, timeout), nil
}
