package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// HeatRepo handles heat score persistence.
type HeatRepo struct {
	db *DB
}

func NewHeatRepo(db *DB) *HeatRepo {
	return &HeatRepo{db: db}
}

// Upsert inserts or updates a heat score for a given entity/date/timeframe.
func (r *HeatRepo) Upsert(ctx context.Context, hs *domain.HeatScore) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO heat_scores (entity_type, entity_id, date, timeframe, score, score_change, positive_signals, negative_signals, top_news_ids, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (entity_type, entity_id, date, timeframe) DO UPDATE SET
			score = EXCLUDED.score,
			score_change = EXCLUDED.score_change,
			positive_signals = EXCLUDED.positive_signals,
			negative_signals = EXCLUDED.negative_signals,
			top_news_ids = EXCLUDED.top_news_ids,
			updated_at = NOW()`,
		hs.EntityType, hs.EntityID, hs.Date, hs.Timeframe,
		hs.Score, hs.ScoreChange, hs.PositiveSignals, hs.NegativeSignals,
		hs.TopNewsIDs,
	)
	return err
}

// GetTopHeat returns the entities with highest absolute heat scores.
// JOINs with companies/commodities to resolve entity names.
func (r *HeatRepo) GetTopHeat(ctx context.Context, entityType domain.EntityType, timeframe string, date time.Time, limit int) ([]domain.HeatScore, error) {
	var query string
	switch entityType {
	case domain.EntityCompany:
		query = `
			SELECT hs.id, hs.entity_type, hs.entity_id,
			       COALESCE(c.ticker, '') AS entity_name,
			       hs.date, hs.timeframe, hs.score, hs.score_change,
			       hs.positive_signals, hs.negative_signals, hs.top_news_ids, hs.updated_at
			FROM heat_scores hs
			LEFT JOIN companies c ON c.id = hs.entity_id
			WHERE hs.entity_type = $1 AND hs.timeframe = $2 AND hs.date = $3
			ORDER BY ABS(hs.score) DESC
			LIMIT $4`
	case domain.EntityCommodity:
		query = `
			SELECT hs.id, hs.entity_type, hs.entity_id,
			       COALESCE(cm.code, '') AS entity_name,
			       hs.date, hs.timeframe, hs.score, hs.score_change,
			       hs.positive_signals, hs.negative_signals, hs.top_news_ids, hs.updated_at
			FROM heat_scores hs
			LEFT JOIN commodities cm ON cm.id = hs.entity_id
			WHERE hs.entity_type = $1 AND hs.timeframe = $2 AND hs.date = $3
			ORDER BY ABS(hs.score) DESC
			LIMIT $4`
	default:
		query = `
			SELECT hs.id, hs.entity_type, hs.entity_id,
			       '' AS entity_name,
			       hs.date, hs.timeframe, hs.score, hs.score_change,
			       hs.positive_signals, hs.negative_signals, hs.top_news_ids, hs.updated_at
			FROM heat_scores hs
			WHERE hs.entity_type = $1 AND hs.timeframe = $2 AND hs.date = $3
			ORDER BY ABS(hs.score) DESC
			LIMIT $4`
	}

	rows, err := r.db.Pool.Query(ctx, query, entityType, timeframe, date, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top heat: %w", err)
	}
	defer rows.Close()

	var result []domain.HeatScore
	for rows.Next() {
		var hs domain.HeatScore
		if err := rows.Scan(
			&hs.ID, &hs.EntityType, &hs.EntityID, &hs.EntityName,
			&hs.Date, &hs.Timeframe,
			&hs.Score, &hs.ScoreChange, &hs.PositiveSignals, &hs.NegativeSignals,
			&hs.TopNewsIDs, &hs.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning heat score: %w", err)
		}
		result = append(result, hs)
	}
	return result, rows.Err()
}

// GetEntityHistory returns heat score history for a specific entity.
func (r *HeatRepo) GetEntityHistory(ctx context.Context, entityType domain.EntityType, entityID int64, timeframe string, days int) ([]domain.HeatScore, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, entity_type, entity_id, '' AS entity_name, date, timeframe, score, score_change,
		       positive_signals, negative_signals, top_news_ids, updated_at
		FROM heat_scores
		WHERE entity_type = $1 AND entity_id = $2 AND timeframe = $3
		  AND date >= CURRENT_DATE - $4::int
		ORDER BY date DESC`, entityType, entityID, timeframe, days)
	if err != nil {
		return nil, fmt.Errorf("querying entity history: %w", err)
	}
	defer rows.Close()

	var result []domain.HeatScore
	for rows.Next() {
		var hs domain.HeatScore
		if err := rows.Scan(
			&hs.ID, &hs.EntityType, &hs.EntityID, &hs.EntityName, &hs.Date, &hs.Timeframe,
			&hs.Score, &hs.ScoreChange, &hs.PositiveSignals, &hs.NegativeSignals,
			&hs.TopNewsIDs, &hs.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning heat score: %w", err)
		}
		result = append(result, hs)
	}
	return result, rows.Err()
}
