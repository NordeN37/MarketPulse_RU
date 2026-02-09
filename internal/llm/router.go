package llm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/claude"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/ollama"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/openai"
)

// TaskType defines the kind of LLM task for routing purposes.
type TaskType string

const (
	TaskClassify      TaskType = "classify"       // Quick classification → Ollama (fast model)
	TaskExtractNER    TaskType = "extract_ner"    // Entity extraction → Ollama (fast model)
	TaskSentiment     TaskType = "sentiment"       // Sentiment analysis → Ollama (fast model)
	TaskDeepAnalysis  TaskType = "deep_analysis"   // Impact analysis → heavy provider
	TaskDigest        TaskType = "digest"           // Daily digest → heavy provider
	TaskChainAnalysis TaskType = "chain_analysis"  // Event chain analysis → heavy provider
)

// Provider represents an LLM provider with a common interface.
type Provider interface {
	Generate(ctx context.Context, system, prompt string) (string, error)
	IsAvailable() bool
	ModelName() string
}

// contextAwareProvider wraps providers that need context for availability check.
type contextAwareProvider struct {
	client *ollama.Client
}

func (p *contextAwareProvider) Generate(ctx context.Context, system, prompt string) (string, error) {
	return p.client.Generate(ctx, system, prompt)
}

func (p *contextAwareProvider) IsAvailable() bool {
	return p.client.IsAvailable(context.Background())
}

func (p *contextAwareProvider) ModelName() string {
	return p.client.ModelName()
}

// Router directs LLM tasks to the appropriate backend.
//
// Supports multiple providers with priority-based fallback:
//   - Ollama fast model (qwen3:8b, thinking=off) — for routine tasks (~80%)
//   - Ollama heavy model (qwen3:14b, thinking=on) — for complex tasks on CPU
//   - DeepSeek API — cheap external provider for complex tasks
//   - Claude API — highest quality, most expensive
//   - Any OpenAI-compatible provider
type Router struct {
	// Fast local model for routine tasks (~80%)
	ollamaFast *ollama.Client
	// Heavy local model for complex tasks (optional, CPU-friendly)
	ollamaHeavy *ollama.Client
	// External providers for complex tasks, ordered by priority
	heavyProviders []Provider
	cfg            config.LLMConfig
	log            *slog.Logger
}

// NewRouter creates a new multi-provider LLM task router.
func NewRouter(cfg config.LLMConfig, log *slog.Logger) *Router {
	r := &Router{
		ollamaFast: ollama.NewClient(cfg.Ollama),
		cfg:        cfg,
		log:        log,
	}

	// Setup heavy Ollama model if configured separately
	if cfg.OllamaHeavy.BaseURL != "" && cfg.OllamaHeavy.Model != "" {
		r.ollamaHeavy = ollama.NewClient(cfg.OllamaHeavy)
	}

	// Build the priority chain for heavy tasks.
	// Order: OllamaHeavy → DeepSeek → Claude → other OpenAI-compat providers
	if r.ollamaHeavy != nil {
		r.heavyProviders = append(r.heavyProviders, &contextAwareProvider{client: r.ollamaHeavy})
	}

	// DeepSeek (OpenAI-compatible)
	if cfg.DeepSeek.BaseURL != "" && cfg.DeepSeek.APIKey != "" {
		ds := openai.NewClient(openai.Config{
			BaseURL:    cfg.DeepSeek.BaseURL,
			APIKey:     cfg.DeepSeek.APIKey,
			Model:      cfg.DeepSeek.Model,
			MaxTokens:  cfg.DeepSeek.MaxTokens,
			TimeoutSec: cfg.DeepSeek.TimeoutSeconds,
		})
		r.heavyProviders = append(r.heavyProviders, ds)
	}

	// Claude
	claudeClient := claude.NewClient(cfg.Claude)
	if claudeClient.IsAvailable() {
		r.heavyProviders = append(r.heavyProviders, claudeClient)
	}

	// Extra OpenAI-compatible providers
	for _, p := range cfg.ExtraProviders {
		if p.BaseURL == "" || p.Model == "" {
			continue
		}
		client := openai.NewClient(openai.Config{
			BaseURL:    p.BaseURL,
			APIKey:     p.APIKey,
			Model:      p.Model,
			MaxTokens:  p.MaxTokens,
			TimeoutSec: p.TimeoutSeconds,
		})
		r.heavyProviders = append(r.heavyProviders, client)
	}

	return r
}

// Generate routes the task to the appropriate LLM and returns (response, model_name, error).
func (r *Router) Generate(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	switch taskType {
	case TaskDeepAnalysis, TaskDigest, TaskChainAnalysis:
		return r.generateHeavy(ctx, taskType, system, prompt)
	default:
		return r.generateFast(ctx, taskType, system, prompt)
	}
}

// generateFast uses the local Ollama fast model for routine tasks.
func (r *Router) generateFast(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	r.log.Debug("routing to Ollama (fast)", "task", taskType, "model", r.ollamaFast.ModelName())
	resp, err := r.ollamaFast.Generate(ctx, system, prompt)
	if err != nil {
		// Try heavy providers as fallback
		r.log.Warn("Ollama fast failed, trying heavy providers",
			"task", taskType,
			"error", err,
		)
		return r.tryHeavyProviders(ctx, taskType, system, prompt)
	}
	return resp, r.ollamaFast.ModelName(), nil
}

// generateHeavy tries heavy providers in priority order with fallback.
func (r *Router) generateHeavy(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	r.log.Debug("routing to heavy provider", "task", taskType)

	if len(r.heavyProviders) > 0 {
		return r.tryHeavyProviders(ctx, taskType, system, prompt)
	}

	// No heavy providers configured — fall back to fast Ollama
	r.log.Warn("no heavy providers available, falling back to fast Ollama", "task", taskType)
	resp, err := r.ollamaFast.Generate(ctx, system, prompt)
	return resp, r.ollamaFast.ModelName(), err
}

// tryHeavyProviders iterates through providers with automatic fallback.
func (r *Router) tryHeavyProviders(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	var lastErr error
	for _, provider := range r.heavyProviders {
		if !provider.IsAvailable() {
			continue
		}

		r.log.Debug("trying provider", "model", provider.ModelName(), "task", taskType)
		resp, err := provider.Generate(ctx, system, prompt)
		if err != nil {
			r.log.Warn("provider failed, trying next",
				"model", provider.ModelName(),
				"task", taskType,
				"error", err,
			)
			lastErr = err
			continue
		}
		return resp, provider.ModelName(), nil
	}

	// All heavy providers failed, try fast Ollama as last resort
	r.log.Warn("all heavy providers failed, trying fast Ollama", "task", taskType)
	resp, err := r.ollamaFast.Generate(ctx, system, prompt)
	if err != nil {
		if lastErr != nil {
			return "", "", fmt.Errorf("all providers failed, last error: %w", lastErr)
		}
		return "", "", err
	}
	return resp, r.ollamaFast.ModelName(), nil
}

// OllamaAvailable checks if the local Ollama instance is reachable.
func (r *Router) OllamaAvailable(ctx context.Context) bool {
	return r.ollamaFast.IsAvailable(ctx)
}

// AvailableProviders returns a list of available provider names.
func (r *Router) AvailableProviders(ctx context.Context) []string {
	var providers []string
	if r.ollamaFast.IsAvailable(ctx) {
		providers = append(providers, "ollama-fast:"+r.ollamaFast.ModelName())
	}
	for _, p := range r.heavyProviders {
		if p.IsAvailable() {
			providers = append(providers, p.ModelName())
		}
	}
	return providers
}
