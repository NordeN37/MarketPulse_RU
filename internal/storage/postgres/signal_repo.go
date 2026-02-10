package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// SignalRepo handles trading signal persistence.
type SignalRepo struct {
	db *DB
}

func NewSignalRepo(db *DB) *SignalRepo {
	return &SignalRepo{db: db}
}

// Insert saves a signal and returns its ID.
func (r *SignalRepo) Insert(ctx context.Context, s *domain.Signal) (int64, error) {
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO trading_signals (ticker, direction, source, strength, price, reason, executed, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		s.Ticker, s.Direction, s.Source, s.Strength, s.Price,
		s.Reason, s.Executed, s.CreatedAt, s.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting signal: %w", err)
	}
	s.ID = id
	return id, nil
}

// GetByTicker returns signals for a ticker.
func (r *SignalRepo) GetByTicker(ctx context.Context, ticker string, limit int) ([]domain.Signal, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, direction, source, strength, price, reason, executed, created_at, expires_at
		FROM trading_signals
		WHERE ticker = $1
		ORDER BY created_at DESC
		LIMIT $2`, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("querying signals: %w", err)
	}
	defer rows.Close()
	return scanSignals(rows)
}

// GetRecent returns the latest signals across all tickers.
func (r *SignalRepo) GetRecent(ctx context.Context, limit int) ([]domain.Signal, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, direction, source, strength, price, reason, executed, created_at, expires_at
		FROM trading_signals
		ORDER BY created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent signals: %w", err)
	}
	defer rows.Close()
	return scanSignals(rows)
}

// GetBySource returns signals filtered by source (NEWS, TA, COMBINED).
func (r *SignalRepo) GetBySource(ctx context.Context, source string, limit int) ([]domain.Signal, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, direction, source, strength, price, reason, executed, created_at, expires_at
		FROM trading_signals
		WHERE source = $1
		ORDER BY created_at DESC
		LIMIT $2`, source, limit)
	if err != nil {
		return nil, fmt.Errorf("querying signals by source: %w", err)
	}
	defer rows.Close()
	return scanSignals(rows)
}

// CountSince returns signal count since a time.
func (r *SignalRepo) CountSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM trading_signals WHERE created_at >= $1`, since).Scan(&count)
	return count, err
}

func scanSignals(rows pgx.Rows) ([]domain.Signal, error) {
	var result []domain.Signal
	for rows.Next() {
		var s domain.Signal
		if err := rows.Scan(&s.ID, &s.Ticker, &s.Direction, &s.Source,
			&s.Strength, &s.Price, &s.Reason, &s.Executed,
			&s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning signal: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
