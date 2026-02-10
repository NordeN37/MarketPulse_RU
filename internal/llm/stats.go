package llm

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// ModelStats holds usage statistics for a single LLM model.
type ModelStats struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	Status           string  `json:"status"` // active, quota_exhausted, disabled
	Requests         int64   `json:"requests"`
	Errors           int64   `json:"errors"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	// internal
	totalLatencyNs atomic.Int64
}

// LLMStatsSnapshot is the full stats view for the API.
type LLMStatsSnapshot struct {
	UpdatedAt      time.Time     `json:"updated_at"`
	Mode           string        `json:"mode"` // standard, batch
	TotalRequests  int64         `json:"total_requests"`
	TotalErrors    int64         `json:"total_errors"`
	TotalTokens    int64         `json:"total_tokens"`
	TotalCostUSD   float64       `json:"total_cost_usd"`
	Models         []ModelStats  `json:"models"`
}

// modelPricing holds per-model cost rates (USD per 1M tokens).
type modelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// Known pricing for models.
var knownPricing = map[string]modelPricing{
	"qwen3-max":             {InputPer1M: 2.00, OutputPer1M: 8.00},
	"qwen-plus":             {InputPer1M: 0.40, OutputPer1M: 1.20},
	"qwen-plus-2025-07-28":  {InputPer1M: 0.40, OutputPer1M: 1.20},
	"deepseek-chat":         {InputPer1M: 0.27, OutputPer1M: 1.10},
}

// StatsCollector tracks per-model LLM usage with thread-safe counters.
type StatsCollector struct {
	mu     sync.RWMutex
	models map[string]*modelCounter
	mode   string
}

type modelCounter struct {
	provider         string
	requests         atomic.Int64
	errors           atomic.Int64
	promptTokens     atomic.Int64
	completionTokens atomic.Int64
	totalLatencyNs   atomic.Int64
	disabled         bool
}

// NewStatsCollector creates a new stats collector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		models: make(map[string]*modelCounter),
		mode:   "standard",
	}
}

// SetMode sets the current operating mode (standard/batch).
func (s *StatsCollector) SetMode(mode string) {
	s.mode = mode
}

// getOrCreate returns the counter for a model, creating if needed.
func (s *StatsCollector) getOrCreate(model, provider string) *modelCounter {
	s.mu.RLock()
	mc, ok := s.models[model]
	s.mu.RUnlock()
	if ok {
		return mc
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after lock
	if mc, ok = s.models[model]; ok {
		return mc
	}
	mc = &modelCounter{provider: provider}
	s.models[model] = mc
	return mc
}

// RecordSuccess records a successful LLM call.
func (s *StatsCollector) RecordSuccess(model, provider string, promptTokens, completionTokens int, latency time.Duration) {
	mc := s.getOrCreate(model, provider)
	mc.requests.Add(1)
	mc.promptTokens.Add(int64(promptTokens))
	mc.completionTokens.Add(int64(completionTokens))
	mc.totalLatencyNs.Add(int64(latency))
}

// RecordError records a failed LLM call.
func (s *StatsCollector) RecordError(model, provider string, latency time.Duration) {
	mc := s.getOrCreate(model, provider)
	mc.requests.Add(1)
	mc.errors.Add(1)
	mc.totalLatencyNs.Add(int64(latency))
}

// MarkDisabled marks a model as quota-exhausted.
func (s *StatsCollector) MarkDisabled(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mc, ok := s.models[model]; ok {
		mc.disabled = true
	}
}

// Snapshot returns the current stats as a serializable struct.
func (s *StatsCollector) Snapshot() LLMStatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := LLMStatsSnapshot{
		UpdatedAt: time.Now(),
		Mode:      s.mode,
	}

	for model, mc := range s.models {
		reqs := mc.requests.Load()
		errs := mc.errors.Load()
		prompt := mc.promptTokens.Load()
		completion := mc.completionTokens.Load()
		total := prompt + completion
		latNs := mc.totalLatencyNs.Load()

		var avgMs float64
		if reqs > 0 {
			avgMs = float64(latNs) / float64(reqs) / 1e6
		}

		status := "active"
		if mc.disabled {
			status = "quota_exhausted"
		}

		cost := estimateCost(model, prompt, completion)

		snap.Models = append(snap.Models, ModelStats{
			Model:            model,
			Provider:         mc.provider,
			Status:           status,
			Requests:         reqs,
			Errors:           errs,
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      total,
			AvgLatencyMs:     avgMs,
			EstimatedCostUSD: cost,
		})

		snap.TotalRequests += reqs
		snap.TotalErrors += errs
		snap.TotalTokens += total
		snap.TotalCostUSD += cost
	}

	return snap
}

// ToJSON serializes the snapshot to JSON bytes.
func (s *StatsCollector) ToJSON() ([]byte, error) {
	return json.Marshal(s.Snapshot())
}

func estimateCost(model string, promptTokens, completionTokens int64) float64 {
	pricing, ok := knownPricing[model]
	if !ok {
		return 0 // local/unknown = free
	}
	inputCost := float64(promptTokens) / 1_000_000 * pricing.InputPer1M
	outputCost := float64(completionTokens) / 1_000_000 * pricing.OutputPer1M
	return inputCost + outputCost
}
