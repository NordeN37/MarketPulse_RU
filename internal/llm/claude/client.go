package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
)

const apiBaseURL = "https://api.anthropic.com/v1"

// Client communicates with the Anthropic Claude API.
type Client struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// NewClient creates a new Claude API client.
func NewClient(cfg config.ClaudeConfig) *Client {
	return &Client{
		apiKey:    cfg.APIKey,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Message represents a single message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MessagesRequest is the Claude /v1/messages request body.
type MessagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

// ContentBlock represents a block of content in the response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MessagesResponse is the Claude /v1/messages response.
type MessagesResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Generate sends a prompt to Claude and returns the response text.
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	req := MessagesRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    system,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("sending request to claude: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("claude returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var msgResp MessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(msgResp.Content) == 0 {
		return "", fmt.Errorf("empty response from claude")
	}

	return msgResp.Content[0].Text, nil
}

// IsAvailable checks if the Claude API is accessible.
func (c *Client) IsAvailable() bool {
	return c.apiKey != ""
}

// ModelName returns the configured model name.
func (c *Client) ModelName() string {
	return c.model
}
