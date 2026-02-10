package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ErrQuotaExhausted is returned when the provider's free quota is depleted (403).
// The router should permanently disable this provider and switch to alternatives.
var ErrQuotaExhausted = errors.New("quota exhausted")

// Client communicates with any OpenAI-compatible API (DeepSeek, OpenRouter, etc.).
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
	disabled   atomic.Bool // set to true when quota is exhausted

	// Last response usage (updated after each successful call)
	lastPromptTokens     atomic.Int64
	lastCompletionTokens atomic.Int64
}

// Config holds settings for an OpenAI-compatible provider.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	MaxTokens  int
	TimeoutSec int
}

// NewClient creates a new OpenAI-compatible API client.
func NewClient(cfg Config) *Client {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		baseURL:   cfg.BaseURL,
		apiKey:    cfg.APIKey,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the OpenAI-compatible /v1/chat/completions request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// ChatChoice represents one completion choice.
type ChatChoice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

// ChatResponse is the OpenAI-compatible /v1/chat/completions response.
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Generate sends a prompt to the provider and returns the response text.
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	if c.disabled.Load() {
		return "", fmt.Errorf("%w: model %s", ErrQuotaExhausted, c.model)
	}

	messages := []ChatMessage{}
	if system != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: system})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: prompt})

	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: 0.1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	var chatResp ChatResponse
	maxRetries := 5
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Re-create request body for retry
			httpReq, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return "", fmt.Errorf("creating request: %w", err)
			}
			httpReq.Header.Set("Content-Type", "application/json")
			if c.apiKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
			}
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return "", fmt.Errorf("sending request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			wait := time.Duration(2<<uint(attempt)) * time.Second // 2s, 4s, 8s, 16s, 32s
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode == http.StatusForbidden {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			bodyStr := string(respBody)
			// DashScope returns 403 with "AllocationQuota" when free tier is exhausted
			if strings.Contains(bodyStr, "AllocationQuota") ||
				strings.Contains(bodyStr, "quota") ||
				strings.Contains(bodyStr, "Quota") {
				c.disabled.Store(true)
				return "", fmt.Errorf("%w: model %s — %s", ErrQuotaExhausted, c.model, bodyStr)
			}
			return "", fmt.Errorf("provider returned status 403: %s", bodyStr)
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
		}

		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("empty response from provider")
		}

		// Track usage for stats
		c.lastPromptTokens.Store(int64(chatResp.Usage.PromptTokens))
		c.lastCompletionTokens.Store(int64(chatResp.Usage.CompletionTokens))

		return chatResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("rate limited after %d retries", maxRetries)
}

// IsAvailable checks if the API is configured and not quota-exhausted.
func (c *Client) IsAvailable() bool {
	return c.baseURL != "" && c.model != "" && !c.disabled.Load()
}

// ModelName returns the configured model name.
func (c *Client) ModelName() string {
	return c.model
}

// LastUsage returns token counts from the most recent successful call.
func (c *Client) LastUsage() (promptTokens, completionTokens int) {
	return int(c.lastPromptTokens.Load()), int(c.lastCompletionTokens.Load())
}
