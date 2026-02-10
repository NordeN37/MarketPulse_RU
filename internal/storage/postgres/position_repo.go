package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// PositionRepo persists trading positions to PostgreSQL.
type PositionRepo struct {
	db *DB
}

func NewPositionRepo(db *DB) *PositionRepo {
	return &PositionRepo{db: db}
}

// Insert saves a new position and returns its ID.
func (r *PositionRepo) Insert(ctx context.Context, p *domain.Position) (int64, error) {
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO positions (ticker, side, status, quantity, entry_price, entry_time, entry_order,
			stop_loss, take_profit, signal_id, strategy, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id`,
		p.Ticker, p.Side, p.Status, p.Quantity, p.EntryPrice, p.EntryTime, p.EntryOrder,
		p.StopLoss, p.TakeProfit, p.SignalID, p.Strategy, p.CreatedAt, p.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting position: %w", err)
	}
	p.ID = id
	return id, nil
}

// Close marks a position as closed with exit data.
func (r *PositionRepo) Close(ctx context.Context, p *domain.Position) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE positions SET status=$1, exit_price=$2, exit_time=$3, exit_order=$4,
			realized_pnl=$5, return_pct=$6, updated_at=NOW()
		WHERE id=$7`,
		domain.PositionClosed, p.ExitPrice, p.ExitTime, p.ExitOrder,
		p.RealizedPnL, p.ReturnPct, p.ID,
	)
	if err != nil {
		return fmt.Errorf("closing position: %w", err)
	}
	return nil
}

// GetOpen returns all open positions.
func (r *PositionRepo) GetOpen(ctx context.Context) ([]domain.Position, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, side, status, quantity, entry_price, entry_time, entry_order,
			exit_price, exit_time, exit_order, stop_loss, take_profit,
			realized_pnl, return_pct, signal_id, strategy, created_at, updated_at
		FROM positions WHERE status = 'OPEN'
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("querying open positions: %w", err)
	}
	defer rows.Close()
	return scanPositions(rows)
}

// GetByTicker returns positions for a ticker (both open and closed).
func (r *PositionRepo) GetByTicker(ctx context.Context, ticker string, limit int) ([]domain.Position, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, side, status, quantity, entry_price, entry_time, entry_order,
			exit_price, exit_time, exit_order, stop_loss, take_profit,
			realized_pnl, return_pct, signal_id, strategy, created_at, updated_at
		FROM positions WHERE ticker = $1
		ORDER BY created_at DESC LIMIT $2`, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("querying positions by ticker: %w", err)
	}
	defer rows.Close()
	return scanPositions(rows)
}

func scanPositions(rows pgx.Rows) ([]domain.Position, error) {
	var result []domain.Position
	for rows.Next() {
		var p domain.Position
		if err := rows.Scan(&p.ID, &p.Ticker, &p.Side, &p.Status, &p.Quantity,
			&p.EntryPrice, &p.EntryTime, &p.EntryOrder,
			&p.ExitPrice, &p.ExitTime, &p.ExitOrder, &p.StopLoss, &p.TakeProfit,
			&p.RealizedPnL, &p.ReturnPct, &p.SignalID, &p.Strategy,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning position: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
