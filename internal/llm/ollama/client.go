package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
)

// Client communicates with a local Ollama instance via /api/chat.
// Supports Qwen3 thinking mode control.
type Client struct {
	baseURL  string
	model    string
	thinking bool
	httpClient *http.Client
}

// NewClient creates a new Ollama API client.
func NewClient(cfg config.OllamaConfig) *Client {
	return &Client{
		baseURL:  cfg.BaseURL,
		model:    cfg.Model,
		thinking: cfg.ThinkingEnabled(),
		httpClient: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the Ollama /api/chat request body.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Think    *bool         `json:"think,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

// ChatResponse is the Ollama /api/chat response.
type ChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done          bool  `json:"done"`
	TotalDuration int64 `json:"total_duration"`
	EvalCount     int   `json:"eval_count"`
}

// thinkTagRegex strips <think>...</think> blocks from Qwen3 responses.
var thinkTagRegex = regexp.MustCompile(`(?s)<think>.*?</think>\s*`)

// Generate sends a prompt to Ollama and returns the response.
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	messages := []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: prompt},
	}

	think := c.thinking
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Think:    &think,
		Options: map[string]any{
			"temperature": 0.1,
			"num_predict": 2048,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("sending request to ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	content := chatResp.Message.Content

	// Safety net: strip any leaked <think>...</think> tags
	content = thinkTagRegex.ReplaceAllString(content, "")
	content = strings.TrimSpace(content)

	return content, nil
}

// IsAvailable checks if the Ollama service is running.
func (c *Client) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// ModelName returns the configured model name.
func (c *Client) ModelName() string {
	return c.model
}

// ThinkingEnabled returns whether thinking mode is enabled.
func (c *Client) ThinkingEnabled() bool {
	return c.thinking
}
