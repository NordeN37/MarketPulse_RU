package signals

import (
	"fmt"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// CombinedSignalGenerator merges news and TA signals with conflict resolution.
type CombinedSignalGenerator struct {
	NewsWeight float64
	TAWeight   float64
	// RequireConfluence: if true, both sources must agree for a trade signal.
	RequireConfluence bool
	SignalTTL         time.Duration
}

// NewCombinedSignalGenerator creates a generator with default parameters.
func NewCombinedSignalGenerator() *CombinedSignalGenerator {
	return &CombinedSignalGenerator{
		NewsWeight:        0.6,
		TAWeight:          0.4,
		RequireConfluence: false,
		SignalTTL:         1 * time.Hour,
	}
}

// Combine merges a news signal and a TA signal for the same ticker.
// If one signal is nil, the other is used with reduced strength.
// Returns nil if the combined signal is too weak or conflicting.
func (g *CombinedSignalGenerator) Combine(newsSig, taSig *domain.Signal, regime domain.MarketRegime) *domain.Signal {
	if newsSig == nil && taSig == nil {
		return nil
	}

	// Adjust weights based on market regime.
	nw, tw := g.adjustWeights(regime)

	var newsScore, taScore float64
	var ticker string
	var price float64

	if newsSig != nil {
		newsScore = directionScore(newsSig.Direction) * newsSig.Strength
		ticker = newsSig.Ticker
		price = newsSig.Price
	}
	if taSig != nil {
		taScore = directionScore(taSig.Direction) * taSig.Strength
		if ticker == "" {
			ticker = taSig.Ticker
		}
		if price == 0 {
			price = taSig.Price
		}
	}

	// Confluence check: if required, both must agree on direction.
	if g.RequireConfluence && newsSig != nil && taSig != nil {
		if (newsScore > 0 && taScore < 0) || (newsScore < 0 && taScore > 0) {
			return nil // conflict, skip
		}
	}

	// Weighted combination.
	combinedScore := nw*newsScore + tw*taScore

	// Determine direction.
	dir := domain.SignalHold
	if combinedScore > 0.15 {
		dir = domain.SignalBuy
	} else if combinedScore < -0.15 {
		dir = domain.SignalSell
	}

	if dir == domain.SignalHold {
		return nil
	}

	strength := combinedScore
	if strength < 0 {
		strength = -strength
	}
	if strength > 1 {
		strength = 1
	}

	now := time.Now()

	reason := fmt.Sprintf("Combined [regime=%s] news=%.2f*%.2f + ta=%.2f*%.2f = %.3f",
		regime, nw, newsScore, tw, taScore, combinedScore)

	return &domain.Signal{
		Ticker:    ticker,
		Direction: dir,
		Source:    domain.SourceCombined,
		Strength:  strength,
		Price:     price,
		Reason:    reason,
		CreatedAt: now,
		ExpiresAt: now.Add(g.SignalTTL),
	}
}

// adjustWeights modifies the base weights based on market regime.
func (g *CombinedSignalGenerator) adjustWeights(regime domain.MarketRegime) (newsW, taW float64) {
	switch regime {
	case domain.RegimeTrend:
		// In trend, news catalysts are more important.
		newsW = g.NewsWeight * 1.2
		taW = g.TAWeight * 0.8
	case domain.RegimeRange:
		// In range, TA (mean reversion) is more reliable.
		newsW = g.NewsWeight * 0.7
		taW = g.TAWeight * 1.3
	case domain.RegimeCrisis:
		// In crisis, only strong news signals matter.
		newsW = g.NewsWeight * 1.5
		taW = g.TAWeight * 0.3
	default:
		newsW = g.NewsWeight
		taW = g.TAWeight
	}

	// Normalize.
	total := newsW + taW
	if total > 0 {
		newsW /= total
		taW /= total
	}
	return
}

func directionScore(dir domain.SignalDirection) float64 {
	switch dir {
	case domain.SignalBuy:
		return 1.0
	case domain.SignalSell:
		return -1.0
	default:
		return 0
	}
}
