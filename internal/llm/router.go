package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

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
//   - Qwen-Plus API (DashScope) — best Russian language, good price
//   - DeepSeek API — cheapest external provider
//   - Claude API — highest quality, most expensive
//   - Any OpenAI-compatible provider
type Router struct {
	// Fast local model for routine tasks (~80%)
	ollamaFast *ollama.Client
	// Heavy local model for complex tasks (optional, CPU-friendly)
	ollamaHeavy *ollama.Client
	// External providers for complex tasks, ordered by priority
	heavyProviders []Provider
	// API-only providers for batch mode (no local Ollama — it's too slow on CPU)
	apiProviders []Provider
	cfg   config.LLMConfig
	log   *slog.Logger
	Stats *StatsCollector
	// forceHeavy overrides routing — all tasks go through heavy chain (skip Ollama)
	forceHeavy bool
	// rrCounter round-robin counter for distributing across providers in batch mode
	rrCounter atomic.Uint64
}

// NewRouter creates a new multi-provider LLM task router.
func NewRouter(cfg config.LLMConfig, log *slog.Logger) *Router {
	r := &Router{
		ollamaFast: ollama.NewClient(cfg.Ollama),
		cfg:        cfg,
		log:        log,
		Stats:      NewStatsCollector(),
	}

	// Setup heavy Ollama model if configured separately
	if cfg.OllamaHeavy.BaseURL != "" && cfg.OllamaHeavy.Model != "" {
		r.ollamaHeavy = ollama.NewClient(cfg.OllamaHeavy)
	}

	// Build the priority chain for heavy tasks.
	// Order: OllamaHeavy → QwenPlus → Extra Qwen models (free quota) → DeepSeek (paid backup) → Claude
	// This ensures all free DashScope quotas are exhausted before falling back to DeepSeek.
	if r.ollamaHeavy != nil {
		r.heavyProviders = append(r.heavyProviders, &contextAwareProvider{client: r.ollamaHeavy})
	}

	// Qwen-Plus (DashScope, OpenAI-compatible — best for Russian)
	if cfg.QwenPlus.BaseURL != "" && cfg.QwenPlus.APIKey != "" {
		qp := openai.NewClient(openai.Config{
			BaseURL:    cfg.QwenPlus.BaseURL,
			APIKey:     cfg.QwenPlus.APIKey,
			Model:      cfg.QwenPlus.Model,
			MaxTokens:  cfg.QwenPlus.MaxTokens,
			TimeoutSec: cfg.QwenPlus.TimeoutSeconds,
		})
		r.heavyProviders = append(r.heavyProviders, qp)
		r.apiProviders = append(r.apiProviders, qp)
	}

	// Extra OpenAI-compatible providers (free Qwen quota models go BEFORE DeepSeek)
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
		r.apiProviders = append(r.apiProviders, client)
	}

	// DeepSeek (OpenAI-compatible) — last resort paid backup, after all free Qwen quotas
	if cfg.DeepSeek.BaseURL != "" && cfg.DeepSeek.APIKey != "" {
		ds := openai.NewClient(openai.Config{
			BaseURL:    cfg.DeepSeek.BaseURL,
			APIKey:     cfg.DeepSeek.APIKey,
			Model:      cfg.DeepSeek.Model,
			MaxTokens:  cfg.DeepSeek.MaxTokens,
			TimeoutSec: cfg.DeepSeek.TimeoutSeconds,
		})
		r.heavyProviders = append(r.heavyProviders, ds)
		r.apiProviders = append(r.apiProviders, ds)
	}

	// Claude
	claudeClient := claude.NewClient(cfg.Claude)
	if claudeClient.IsAvailable() {
		r.heavyProviders = append(r.heavyProviders, claudeClient)
		r.apiProviders = append(r.apiProviders, claudeClient)
	}

	// Log provider chain at startup
	var names []string
	for _, p := range r.apiProviders {
		names = append(names, p.ModelName())
	}
	log.Info("LLM провайдеры инициализированы",
		"fast", cfg.Ollama.Model,
		"api_chain", names,
		"total_heavy", len(r.heavyProviders),
		"total_api", len(r.apiProviders),
	)

	return r
}

// SetForceHeavy forces all tasks through the heavy provider chain (API), skipping Ollama.
func (r *Router) SetForceHeavy(force bool) {
	r.forceHeavy = force
}

// Generate routes the task to the appropriate LLM and returns (response, model_name, error).
func (r *Router) Generate(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	if r.forceHeavy {
		return r.generateRoundRobin(ctx, taskType, system, prompt)
	}
	switch taskType {
	case TaskDeepAnalysis, TaskDigest, TaskChainAnalysis:
		return r.generateHeavy(ctx, taskType, system, prompt)
	default:
		return r.generateFast(ctx, taskType, system, prompt)
	}
}

// generateRoundRobin distributes requests across API providers evenly (no local Ollama).
// If the chosen provider fails, falls back to others.
func (r *Router) generateRoundRobin(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	providers := r.apiProviders
	if len(providers) == 0 {
		// Fallback to all heavy providers if no API-only providers configured
		providers = r.heavyProviders
	}
	n := len(providers)
	if n == 0 {
		return "", "", fmt.Errorf("no API providers available for batch mode")
	}

	idx := int(r.rrCounter.Add(1)-1) % n
	var lastErr error

	// Try starting from the round-robin pick, then rotate through others
	for i := 0; i < n; i++ {
		p := providers[(idx+i)%n]
		if !p.IsAvailable() {
			continue
		}
		start := time.Now()
		resp, err := p.Generate(ctx, system, prompt)
		latency := time.Since(start)
		r.recordProviderStats(p, "api", err, latency)
		if err != nil {
			if errors.Is(err, openai.ErrQuotaExhausted) {
				r.log.Error("КВОТА ИСЧЕРПАНА — провайдер отключён",
					"model", p.ModelName(),
					"active_providers", r.countAvailable(providers),
				)
			} else {
				r.log.Warn("provider failed, trying next",
					"model", p.ModelName(),
					"task", taskType,
					"error", err,
				)
			}
			lastErr = err
			continue
		}
		return resp, p.ModelName(), nil
	}

	return "", "", fmt.Errorf("all providers failed: %w", lastErr)
}

// countAvailable returns how many providers are still available.
func (r *Router) countAvailable(providers []Provider) int {
	n := 0
	for _, p := range providers {
		if p.IsAvailable() {
			n++
		}
	}
	return n
}

// generateFast uses the local Ollama fast model for routine tasks.
func (r *Router) generateFast(ctx context.Context, taskType TaskType, system, prompt string) (string, string, error) {
	r.log.Debug("routing to Ollama (fast)", "task", taskType, "model", r.ollamaFast.ModelName())
	start := time.Now()
	resp, err := r.ollamaFast.Generate(ctx, system, prompt)
	latency := time.Since(start)
	if err != nil {
		r.Stats.RecordError(r.ollamaFast.ModelName(), "ollama", latency)
		// Try heavy providers as fallback
		r.log.Warn("Ollama fast failed, trying heavy providers",
			"task", taskType,
			"error", err,
		)
		return r.tryHeavyProviders(ctx, taskType, system, prompt)
	}
	r.Stats.RecordSuccess(r.ollamaFast.ModelName(), "ollama", 0, 0, latency)
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
		start := time.Now()
		resp, err := provider.Generate(ctx, system, prompt)
		latency := time.Since(start)
		r.recordProviderStats(provider, "api", err, latency)
		if err != nil {
			if errors.Is(err, openai.ErrQuotaExhausted) {
				r.log.Error("КВОТА ИСЧЕРПАНА — провайдер отключён",
					"model", provider.ModelName(),
					"active_providers", r.countAvailable(r.heavyProviders),
				)
			} else {
				r.log.Warn("provider failed, trying next",
					"model", provider.ModelName(),
					"task", taskType,
					"error", err,
				)
			}
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

// usageProvider is optionally implemented by providers that track token usage.
type usageProvider interface {
	LastUsage() (promptTokens, completionTokens int)
}

// recordProviderStats records stats for a provider call.
func (r *Router) recordProviderStats(p Provider, providerType string, err error, latency time.Duration) {
	model := p.ModelName()
	if err != nil {
		r.Stats.RecordError(model, providerType, latency)
		if errors.Is(err, openai.ErrQuotaExhausted) {
			r.Stats.MarkDisabled(model)
		}
		return
	}
	var prompt, completion int
	if up, ok := p.(usageProvider); ok {
		prompt, completion = up.LastUsage()
	}
	r.Stats.RecordSuccess(model, providerType, prompt, completion, latency)
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
