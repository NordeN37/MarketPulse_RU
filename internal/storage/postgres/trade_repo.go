package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TradeRecord represents a completed trade in the database.
type TradeRecord struct {
	ID         int64     `json:"id"`
	Ticker     string    `json:"ticker"`
	Side       string    `json:"side"`
	Strategy   string    `json:"strategy"`
	Quantity   int       `json:"quantity"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	EntryTime  time.Time `json:"entry_time"`
	ExitTime   time.Time `json:"exit_time"`
	PnL        float64   `json:"pnl"`
	ReturnPct  float64   `json:"return_pct"`
	Commission float64   `json:"commission"`
	PositionID int64     `json:"position_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// TradeRepo handles trade persistence.
type TradeRepo struct {
	db *DB
}

func NewTradeRepo(db *DB) *TradeRepo {
	return &TradeRepo{db: db}
}

// Insert saves a trade record.
func (r *TradeRepo) Insert(ctx context.Context, t *TradeRecord) (int64, error) {
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO trades (ticker, side, strategy, quantity, entry_price, exit_price,
		                     entry_time, exit_time, pnl, return_pct, commission, position_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		t.Ticker, t.Side, t.Strategy, t.Quantity,
		t.EntryPrice, t.ExitPrice, t.EntryTime, t.ExitTime,
		t.PnL, t.ReturnPct, t.Commission, t.PositionID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting trade: %w", err)
	}
	return id, nil
}

// GetByStrategy returns trades for a given strategy.
func (r *TradeRepo) GetByStrategy(ctx context.Context, strategy string, limit int) ([]TradeRecord, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, side, strategy, quantity, entry_price, exit_price,
		       entry_time, exit_time, pnl, return_pct, commission, position_id, created_at
		FROM trades
		WHERE strategy = $1
		ORDER BY exit_time DESC
		LIMIT $2`, strategy, limit)
	if err != nil {
		return nil, fmt.Errorf("querying trades: %w", err)
	}
	defer rows.Close()
	return scanTrades(rows)
}

// GetRecent returns the latest trades.
func (r *TradeRepo) GetRecent(ctx context.Context, limit int) ([]TradeRecord, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, side, strategy, quantity, entry_price, exit_price,
		       entry_time, exit_time, pnl, return_pct, commission, position_id, created_at
		FROM trades
		ORDER BY exit_time DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent trades: %w", err)
	}
	defer rows.Close()
	return scanTrades(rows)
}

func scanTrades(rows pgx.Rows) ([]TradeRecord, error) {
	var result []TradeRecord
	for rows.Next() {
		var t TradeRecord
		if err := rows.Scan(&t.ID, &t.Ticker, &t.Side, &t.Strategy,
			&t.Quantity, &t.EntryPrice, &t.ExitPrice,
			&t.EntryTime, &t.ExitTime, &t.PnL, &t.ReturnPct,
			&t.Commission, &t.PositionID, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning trade: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
