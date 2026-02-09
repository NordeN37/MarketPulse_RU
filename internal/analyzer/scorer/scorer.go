package scorer

import (
	"context"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// Scorer recalculates heat scores based on news impacts.
type Scorer struct {
	heatRepo *postgres.HeatRepo
	log      *slog.Logger
}

// NewScorer creates a new heat score calculator.
func NewScorer(heatRepo *postgres.HeatRepo, log *slog.Logger) *Scorer {
	return &Scorer{
		heatRepo: heatRepo,
		log:      log,
	}
}

// UpdateFromImpact recalculates the heat score for an entity after a new impact.
func (s *Scorer) UpdateFromImpact(ctx context.Context, impact *domain.NewsImpact) error {
	today := time.Now().Truncate(24 * time.Hour)

	// Get current score for today
	scores, err := s.heatRepo.GetEntityHistory(ctx, impact.EntityType, impact.EntityID, "1d", 1)
	if err != nil {
		return err
	}

	var current domain.HeatScore
	if len(scores) > 0 && scores[0].Date.Equal(today) {
		current = scores[0]
	} else {
		current = domain.HeatScore{
			EntityType: impact.EntityType,
			EntityID:   impact.EntityID,
			Date:       today,
			Timeframe:  "1d",
		}
	}

	// Update score based on impact
	delta := float64(impact.Direction) * impact.Magnitude * impact.Confidence
	current.Score += delta

	if impact.Direction > 0 {
		current.PositiveSignals++
	} else if impact.Direction < 0 {
		current.NegativeSignals++
	}

	// Add news to top news IDs (keep last 10)
	current.TopNewsIDs = append(current.TopNewsIDs, impact.NewsID)
	if len(current.TopNewsIDs) > 10 {
		current.TopNewsIDs = current.TopNewsIDs[len(current.TopNewsIDs)-10:]
	}

	// Calculate score change vs yesterday
	yesterday, err := s.heatRepo.GetEntityHistory(ctx, impact.EntityType, impact.EntityID, "1d", 2)
	if err == nil && len(yesterday) >= 2 {
		current.ScoreChange = current.Score - yesterday[1].Score
	}

	return s.heatRepo.Upsert(ctx, &current)
}
