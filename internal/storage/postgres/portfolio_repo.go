package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PortfolioSnapshot represents a point on the equity curve.
type PortfolioSnapshot struct {
	ID            int64     `json:"id"`
	Strategy      string    `json:"strategy"` // "news", "ta", "combined"
	Cash          float64   `json:"cash"`
	TotalValue    float64   `json:"total_value"`
	OpenPositions int       `json:"open_positions"`
	DailyPnL      float64   `json:"daily_pnl"`
	TotalPnL      float64   `json:"total_pnl"`
	MaxDrawdown   float64   `json:"max_drawdown"`
	WinRate       float64   `json:"win_rate"`
	TotalTrades   int       `json:"total_trades"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

// PortfolioRepo handles portfolio snapshot persistence.
type PortfolioRepo struct {
	db *DB
}

func NewPortfolioRepo(db *DB) *PortfolioRepo {
	return &PortfolioRepo{db: db}
}

// InsertSnapshot saves a portfolio snapshot.
func (r *PortfolioRepo) InsertSnapshot(ctx context.Context, s *PortfolioSnapshot) (int64, error) {
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO portfolio_snapshots (strategy, cash, total_value, open_positions,
		                                  daily_pnl, total_pnl, max_drawdown, win_rate,
		                                  total_trades, snapshot_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		s.Strategy, s.Cash, s.TotalValue, s.OpenPositions,
		s.DailyPnL, s.TotalPnL, s.MaxDrawdown, s.WinRate,
		s.TotalTrades, s.SnapshotAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting portfolio snapshot: %w", err)
	}
	return id, nil
}

// BulkInsertSnapshots inserts multiple snapshots efficiently.
func (r *PortfolioRepo) BulkInsertSnapshots(ctx context.Context, snapshots []PortfolioSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range snapshots {
		batch.Queue(`
			INSERT INTO portfolio_snapshots (strategy, cash, total_value, open_positions,
			                                  daily_pnl, total_pnl, max_drawdown, win_rate,
			                                  total_trades, snapshot_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			s.Strategy, s.Cash, s.TotalValue, s.OpenPositions,
			s.DailyPnL, s.TotalPnL, s.MaxDrawdown, s.WinRate,
			s.TotalTrades, s.SnapshotAt,
		)
	}
	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range snapshots {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("inserting snapshot batch: %w", err)
		}
	}
	return nil
}

// GetByStrategy returns snapshots for a strategy ordered by time.
func (r *PortfolioRepo) GetByStrategy(ctx context.Context, strategy string) ([]PortfolioSnapshot, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, strategy, cash, total_value, open_positions,
		       daily_pnl, total_pnl, max_drawdown, win_rate,
		       total_trades, snapshot_at
		FROM portfolio_snapshots
		WHERE strategy = $1
		ORDER BY snapshot_at ASC`, strategy)
	if err != nil {
		return nil, fmt.Errorf("querying portfolio snapshots: %w", err)
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// GetLatest returns the latest snapshot for each strategy.
func (r *PortfolioRepo) GetLatest(ctx context.Context) ([]PortfolioSnapshot, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT ON (strategy) id, strategy, cash, total_value, open_positions,
		       daily_pnl, total_pnl, max_drawdown, win_rate, total_trades, snapshot_at
		FROM portfolio_snapshots
		ORDER BY strategy, snapshot_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("querying latest snapshots: %w", err)
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// CountByStrategy returns the number of snapshots for a strategy.
func (r *PortfolioRepo) CountByStrategy(ctx context.Context, strategy string) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM portfolio_snapshots WHERE strategy = $1`, strategy).Scan(&count)
	return count, err
}

// DeleteByStrategy removes all snapshots for a strategy (for re-running backtest).
func (r *PortfolioRepo) DeleteByStrategy(ctx context.Context, strategy string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM portfolio_snapshots WHERE strategy = $1`, strategy)
	return err
}

func scanSnapshots(rows pgx.Rows) ([]PortfolioSnapshot, error) {
	var result []PortfolioSnapshot
	for rows.Next() {
		var s PortfolioSnapshot
		if err := rows.Scan(&s.ID, &s.Strategy, &s.Cash, &s.TotalValue,
			&s.OpenPositions, &s.DailyPnL, &s.TotalPnL, &s.MaxDrawdown,
			&s.WinRate, &s.TotalTrades, &s.SnapshotAt); err != nil {
			return nil, fmt.Errorf("scanning snapshot: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
