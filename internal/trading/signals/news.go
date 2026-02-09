package signals

import (
	"fmt"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// NewsSignalGenerator creates trading signals from news analysis.
type NewsSignalGenerator struct {
	// Minimum urgency level to generate a signal (1-5).
	MinUrgency int
	// Minimum absolute sentiment to generate a signal (0.0-1.0).
	MinSentiment float64
	// Signal validity duration.
	SignalTTL time.Duration
}

// NewNewsSignalGenerator creates a generator with default parameters.
func NewNewsSignalGenerator() *NewsSignalGenerator {
	return &NewsSignalGenerator{
		MinUrgency:   3,
		MinSentiment: 0.4,
		SignalTTL:    2 * time.Hour,
	}
}

// Generate creates signals from a news analysis result.
func (g *NewsSignalGenerator) Generate(analysis *domain.NewsAnalysis, impacts []domain.NewsImpact, currentPrices map[string]float64) []domain.Signal {
	if analysis == nil {
		return nil
	}

	// Skip low-urgency or weak-sentiment news.
	if analysis.Urgency < g.MinUrgency {
		return nil
	}
	absSentiment := analysis.Sentiment
	if absSentiment < 0 {
		absSentiment = -absSentiment
	}
	if absSentiment < g.MinSentiment {
		return nil
	}

	var signals []domain.Signal
	now := time.Now()

	for _, impact := range impacts {
		if impact.EntityType != domain.EntityCompany {
			continue
		}

		price := currentPrices[impact.EntityName]

		dir := domain.SignalHold
		if impact.Direction == domain.ImpactPositive {
			dir = domain.SignalBuy
		} else if impact.Direction == domain.ImpactNegative {
			dir = domain.SignalSell
		}

		if dir == domain.SignalHold {
			continue
		}

		// Signal strength: combine urgency, sentiment, and impact magnitude.
		strength := g.calculateStrength(analysis, &impact)

		sig := domain.Signal{
			Ticker:    impact.EntityName,
			Direction: dir,
			Source:    domain.SourceNews,
			Strength:  strength,
			Price:     price,
			Reason:    fmt.Sprintf("News [%s] urgency=%d sentiment=%.2f impact=%s", analysis.Category, analysis.Urgency, analysis.Sentiment, impact.Direction),
			CreatedAt: now,
			ExpiresAt: now.Add(g.SignalTTL),
			NewsSignalMeta: &domain.NewsSignalMeta{
				NewsID:    analysis.NewsID,
				Sentiment: analysis.Sentiment,
				Urgency:   analysis.Urgency,
				Category:  string(analysis.Category),
			},
		}

		signals = append(signals, sig)
	}

	return signals
}

// calculateStrength combines urgency, sentiment, and magnitude into [0, 1].
func (g *NewsSignalGenerator) calculateStrength(analysis *domain.NewsAnalysis, impact *domain.NewsImpact) float64 {
	// Normalize urgency (1-5) to [0.2, 1.0].
	urgencyScore := float64(analysis.Urgency) / 5.0

	// Absolute sentiment [0, 1].
	sentimentScore := analysis.Sentiment
	if sentimentScore < 0 {
		sentimentScore = -sentimentScore
	}

	// Impact magnitude [0, 1].
	magnitudeScore := impact.Magnitude

	// Weighted combination.
	strength := 0.3*urgencyScore + 0.4*sentimentScore + 0.3*magnitudeScore

	// Clamp to [0, 1].
	if strength > 1 {
		strength = 1
	}
	if strength < 0 {
		strength = 0
	}
	return strength
}
