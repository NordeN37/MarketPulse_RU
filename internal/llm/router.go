package llm

import (
	"context"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/claude"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/ollama"
)

// TaskType defines the kind of LLM task for routing purposes.
type TaskType string

const (
	TaskClassify     TaskType = "classify"      // Quick classification → Ollama
	TaskExtractNER   TaskType = "extract_ner"   // Entity extraction → Ollama
	TaskSentiment    TaskType = "sentiment"      // Sentiment analysis → Ollama
	TaskDeepAnalysis TaskType = "deep_analysis"  // Impact analysis → Claude
	TaskDigest       TaskType = "digest"         // Daily digest → Claude
	TaskChainAnalysis TaskType = "chain_analysis" // Event chain analysis → Claude
)

// Router directs LLM tasks to the appropriate backend (Ollama or Claude).
type Router struct {
	ollama *ollama.Client
	claude *claude.Client
	cfg    config.LLMConfig
	log    *slog.Logger
}

// NewRouter creates a new LLM task router.
func NewRouter(cfg config.LLMConfig, log *slog.Logger) *Router {
	return &Router{
		ollama: ollama.NewClient(cfg.Ollama),
		claude: claude.NewClient(cfg.Claude),
		cfg:    cfg,
		log:    log,
	}
}

// Generate routes the task to the appropriate LLM and returns the response.
func (r *Router) Generate(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	switch taskType {
	case TaskDeepAnalysis, TaskDigest, TaskChainAnalysis:
		// Use Claude for complex tasks if available and configured
		if r.claude.IsAvailable() && r.cfg.Claude.DeepAnalysis {
			r.log.Debug("routing to Claude", "task", taskType)
			resp, err := r.claude.Generate(ctx, system, prompt)
			if err != nil {
				r.log.Warn("Claude failed, falling back to Ollama",
					"task", taskType,
					"error", err,
				)
				// Fallback to Ollama
				resp, err := r.ollama.Generate(ctx, system, prompt)
				return resp, r.ollama.ModelName(), err
			}
			return resp, r.claude.ModelName(), nil
		}
		// Fallback to Ollama
		resp, err := r.ollama.Generate(ctx, system, prompt)
		return resp, r.ollama.ModelName(), err

	default:
		// Use Ollama for fast, routine tasks
		r.log.Debug("routing to Ollama", "task", taskType)
		resp, err := r.ollama.Generate(ctx, system, prompt)
		return resp, r.ollama.ModelName(), err
	}
}

// OllamaAvailable checks if the local Ollama instance is reachable.
func (r *Router) OllamaAvailable(ctx context.Context) bool {
	return r.ollama.IsAvailable(ctx)
}

// ClaudeAvailable checks if the Claude API is configured.
func (r *Router) ClaudeAvailable() bool {
	return r.claude.IsAvailable()
}
