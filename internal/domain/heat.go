package domain

import "time"

// HeatScore represents an aggregated sentiment/activity score for an entity.
type HeatScore struct {
	ID              int64      `json:"id" db:"id"`
	EntityType      EntityType `json:"entity_type" db:"entity_type"`
	EntityID        int64      `json:"entity_id" db:"entity_id"`
	EntityName      string     `json:"entity_name" db:"entity_name"`
	Date            time.Time  `json:"date" db:"date"`
	Timeframe       string     `json:"timeframe" db:"timeframe"` // "1d", "7d", "30d"
	Score           float64    `json:"score" db:"score"`
	ScoreChange     float64    `json:"score_change" db:"score_change"`
	PositiveSignals int        `json:"positive_signals" db:"positive_signals"`
	NegativeSignals int        `json:"negative_signals" db:"negative_signals"`
	TopNewsIDs      []int64    `json:"top_news_ids" db:"top_news_ids"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}
