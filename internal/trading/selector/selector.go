package selector

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/signals"
)

// TickerScore holds a ticker with its composite ranking score.
type TickerScore struct {
	Ticker   string
	TAScore  float64 // absolute TA signal strength [0..1]
	HeatScore float64 // news heat score (normalized)
	Composite float64 // weighted combination
}

// Selector dynamically picks top-N tickers from the full universe.
type Selector struct {
	moex     *moex.Client
	taGen    *signals.TASignalGenerator
	log      *slog.Logger
	taWeight   float64 // how much TA score matters (0..1)
	heatWeight float64 // how much news heat matters (0..1)
}

// NewSelector creates a ticker selector.
func NewSelector(moexClient *moex.Client, log *slog.Logger) *Selector {
	return &Selector{
		moex:       moexClient,
		taGen:      signals.NewTASignalGenerator(),
		log:        log,
		taWeight:   0.6,
		heatWeight: 0.4,
	}
}

// SelectTopN ranks all universe tickers and returns the top N by signal strength.
// It uses TA analysis (from recent candles) to score each ticker.
// If heatScores map is provided, it blends news heat into the ranking.
func (s *Selector) SelectTopN(
	ctx context.Context,
	universeTickers []string,
	n int,
	heatScores map[string]float64,
) []string {
	if n <= 0 || len(universeTickers) == 0 {
		return nil
	}
	if n >= len(universeTickers) {
		return universeTickers
	}

	var scored []TickerScore
	for _, ticker := range universeTickers {
		ts := s.scoreTicker(ctx, ticker, heatScores)
		scored = append(scored, ts)
	}

	// Sort by composite score descending.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Composite > scored[j].Composite
	})

	result := make([]string, 0, n)
	for i := 0; i < n && i < len(scored); i++ {
		result = append(result, scored[i].Ticker)
		s.log.Debug("ticker ranked",
			"rank", i+1,
			"ticker", scored[i].Ticker,
			"composite", scored[i].Composite,
			"ta", scored[i].TAScore,
			"heat", scored[i].HeatScore,
		)
	}

	return result
}

// scoreTicker computes a composite score for a single ticker.
func (s *Selector) scoreTicker(ctx context.Context, ticker string, heatScores map[string]float64) TickerScore {
	ts := TickerScore{Ticker: ticker}

	// TA score: analyze recent 1h candles.
	now := time.Now()
	candles, err := s.moex.GetCandlesAll(ctx, ticker, moex.Interval1Hour, now.AddDate(0, -1, 0), now)
	if err == nil && len(candles) >= 50 {
		analysis := s.taGen.Analyze(ticker, "1h", candles)
		sig := s.taGen.Generate(analysis)
		if sig != nil {
			// Use absolute strength — we want tickers with strong signals, regardless of direction.
			ts.TAScore = sig.Strength
			if ts.TAScore < 0 {
				ts.TAScore = -ts.TAScore
			}
		}
	}

	// Heat score from news analysis.
	if heatScores != nil {
		if h, ok := heatScores[ticker]; ok {
			ts.HeatScore = h
			if ts.HeatScore < 0 {
				ts.HeatScore = -ts.HeatScore
			}
		}
	}

	// Composite: weighted combination.
	ts.Composite = s.taWeight*ts.TAScore + s.heatWeight*ts.HeatScore

	return ts
}
