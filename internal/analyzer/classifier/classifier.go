package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/llm"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/prompts"
)

// ClassificationResult holds the parsed LLM classification output.
type ClassificationResult struct {
	Category    domain.NewsCategory `json:"category"`
	Sentiment   float64             `json:"sentiment"`
	Urgency     int                 `json:"urgency"`
	Reliability float64             `json:"reliability"`
	Summary     string              `json:"summary"`
	KeyFacts    []string            `json:"key_facts"`
	Tickers     []string            `json:"tickers"`
	Sectors     []string            `json:"sectors"`
	Regions     []string            `json:"regions"`
}

// Classifier uses LLM to classify and extract info from news.
type Classifier struct {
	router *llm.Router
	log    *slog.Logger
}

// NewClassifier creates a new news classifier.
func NewClassifier(router *llm.Router, log *slog.Logger) *Classifier {
	return &Classifier{
		router: router,
		log:    log,
	}
}

// Classify sends a news item to the LLM for classification and returns structured results.
func (c *Classifier) Classify(ctx context.Context, news *domain.News) (*ClassificationResult, string, error) {
	title := news.Title
	if title == "" {
		// For Telegram messages without titles, use first 100 chars of content
		runes := []rune(news.Content)
		if len(runes) > 100 {
			title = string(runes[:100])
		} else {
			title = news.Content
		}
	}

	prompt := prompts.ClassifyNews(title, news.Content)
	resp, model, err := c.router.Generate(ctx, llm.TaskClassify, prompts.SystemClassifier, prompt)
	if err != nil {
		return nil, "", fmt.Errorf("LLM classification: %w", err)
	}

	// Parse JSON from the response (handle possible markdown wrapping)
	resp = extractJSON(resp)

	var result ClassificationResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.log.Warn("failed to parse classification JSON, retrying with cleanup",
			"response", resp[:min(len(resp), 200)],
			"error", err,
		)
		return nil, model, fmt.Errorf("parsing classification response: %w", err)
	}

	// Validate ranges
	result.Sentiment = clamp(result.Sentiment, -1.0, 1.0)
	result.Urgency = clampInt(result.Urgency, 1, 5)
	result.Reliability = clamp(result.Reliability, 0.0, 1.0)

	return &result, model, nil
}

// extractJSON tries to extract a JSON object from a response that might contain markdown.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Remove markdown code fences
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
