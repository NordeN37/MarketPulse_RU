package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// CandleRepo handles OHLCV candle cache persistence.
type CandleRepo struct {
	db *DB
}

func NewCandleRepo(db *DB) *CandleRepo {
	return &CandleRepo{db: db}
}

// BulkInsert writes candles using ON CONFLICT upsert.
func (r *CandleRepo) BulkInsert(ctx context.Context, candles []domain.Candle) (int64, error) {
	if len(candles) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, c := range candles {
		batch.Queue(`
			INSERT INTO candles (ticker, interval, open_time, open, high, low, close, volume)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (ticker, interval, open_time) DO UPDATE SET
				open = EXCLUDED.open, high = EXCLUDED.high,
				low = EXCLUDED.low, close = EXCLUDED.close, volume = EXCLUDED.volume`,
			c.Ticker, c.Interval, c.OpenTime,
			c.Open, c.High, c.Low, c.Close, c.Volume,
		)
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	var inserted int64
	for range candles {
		_, err := br.Exec()
		if err != nil {
			return inserted, fmt.Errorf("inserting candle: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

// GetByTicker returns candles for a ticker/interval sorted by time ascending.
func (r *CandleRepo) GetByTicker(ctx context.Context, ticker, interval string) ([]domain.Candle, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT ticker, interval, open_time, open, high, low, close, volume
		FROM candles
		WHERE ticker = $1 AND interval = $2
		ORDER BY open_time ASC`, ticker, interval)
	if err != nil {
		return nil, fmt.Errorf("querying candles: %w", err)
	}
	defer rows.Close()
	return scanCandles(rows)
}

// GetByTickerRange returns candles within a time range.
func (r *CandleRepo) GetByTickerRange(ctx context.Context, ticker, interval string, fromTS, toTS int64) ([]domain.Candle, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT ticker, interval, open_time, open, high, low, close, volume
		FROM candles
		WHERE ticker = $1 AND interval = $2 AND open_time >= $3 AND open_time <= $4
		ORDER BY open_time ASC`, ticker, interval, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("querying candles range: %w", err)
	}
	defer rows.Close()
	return scanCandles(rows)
}

// Count returns total candle records for a ticker/interval.
func (r *CandleRepo) Count(ctx context.Context, ticker, interval string) (int64, error) {
	var count int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM candles WHERE ticker = $1 AND interval = $2`,
		ticker, interval).Scan(&count)
	return count, err
}

// LatestOpenTime returns the latest open_time for a ticker/interval.
func (r *CandleRepo) LatestOpenTime(ctx context.Context, ticker, interval string) (int64, error) {
	var ts int64
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(open_time), 0) FROM candles WHERE ticker = $1 AND interval = $2`,
		ticker, interval).Scan(&ts)
	return ts, err
}

func scanCandles(rows pgx.Rows) ([]domain.Candle, error) {
	var result []domain.Candle
	for rows.Next() {
		var c domain.Candle
		if err := rows.Scan(&c.Ticker, &c.Interval, &c.OpenTime,
			&c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, fmt.Errorf("scanning candle: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
