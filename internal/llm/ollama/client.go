package ollama

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

// Client communicates with a local Ollama instance.
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewClient creates a new Ollama API client.
func NewClient(cfg config.OllamaConfig) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// GenerateRequest is the Ollama /api/generate request body.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	Stream bool   `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

// GenerateResponse is the Ollama /api/generate response.
type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	TotalDuration  int64 `json:"total_duration"`
	EvalCount      int   `json:"eval_count"`
}

// Generate sends a prompt to Ollama and returns the response.
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	req := GenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		System: system,
		Stream: false,
		Options: map[string]any{
			"temperature": 0.1,
			"num_predict": 2048,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
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

	var genResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return genResp.Response, nil
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
