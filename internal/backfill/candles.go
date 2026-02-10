package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// CandleBackfill downloads historical candles from MOEX and stores them in the DB.
type CandleBackfill struct {
	moex      *moex.Client
	repo      *postgres.CandleRepo
	log       *slog.Logger
}

// NewCandleBackfill creates a new candle backfill service.
func NewCandleBackfill(moexClient *moex.Client, repo *postgres.CandleRepo, log *slog.Logger) *CandleBackfill {
	return &CandleBackfill{
		moex: moexClient,
		repo: repo,
		log:  log,
	}
}

// BackfillAll downloads candles for all tickers at the given interval.
// It checks existing data and only fetches what's missing.
func (b *CandleBackfill) BackfillAll(ctx context.Context, tickers []string, interval int, daysBack int) error {
	intervalStr := moex.IntervalToString[interval]
	if intervalStr == "" {
		return fmt.Errorf("unknown interval: %d", interval)
	}

	b.log.Info("starting candle backfill",
		"tickers", len(tickers),
		"interval", intervalStr,
		"days_back", daysBack,
	)

	now := time.Now()
	globalFrom := now.AddDate(0, 0, -daysBack)
	var totalInserted int64

	for _, ticker := range tickers {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		inserted, err := b.backfillTicker(ctx, ticker, interval, intervalStr, globalFrom, now)
		if err != nil {
			b.log.Error("backfill error", "ticker", ticker, "error", err)
			continue
		}
		totalInserted += inserted

		// Rate limit between tickers.
		time.Sleep(300 * time.Millisecond)
	}

	b.log.Info("candle backfill complete",
		"total_inserted", totalInserted,
	)
	return nil
}

func (b *CandleBackfill) backfillTicker(ctx context.Context, ticker string, interval int, intervalStr string, from, till time.Time) (int64, error) {
	// Check if we already have data — start from the latest.
	latestTS, err := b.repo.LatestOpenTime(ctx, ticker, intervalStr)
	if err != nil {
		return 0, fmt.Errorf("checking latest candle for %s: %w", ticker, err)
	}

	fetchFrom := from
	if latestTS > 0 {
		existing := time.Unix(latestTS, 0)
		if existing.After(from) {
			fetchFrom = existing.Add(time.Second) // start just after latest
		}
	}

	if !fetchFrom.Before(till) {
		b.log.Debug("candles up to date", "ticker", ticker, "interval", intervalStr)
		return 0, nil
	}

	b.log.Info("fetching candles",
		"ticker", ticker,
		"interval", intervalStr,
		"from", fetchFrom.Format("2006-01-02"),
		"till", till.Format("2006-01-02"),
	)

	candles, err := b.moex.GetCandlesAll(ctx, ticker, interval, fetchFrom, till)
	if err != nil {
		return 0, fmt.Errorf("fetching candles for %s: %w", ticker, err)
	}

	if len(candles) == 0 {
		return 0, nil
	}

	inserted, err := b.repo.BulkInsert(ctx, candles)
	if err != nil {
		return 0, fmt.Errorf("storing candles for %s: %w", ticker, err)
	}

	b.log.Info("candles stored",
		"ticker", ticker,
		"count", inserted,
	)
	return inserted, nil
}

// GetCandlesFromDB returns cached candles, fetching from MOEX if needed.
func (b *CandleBackfill) GetCandlesFromDB(ctx context.Context, ticker, interval string) ([]domain.Candle, error) {
	candles, err := b.repo.GetByTicker(ctx, ticker, interval)
	if err != nil {
		return nil, err
	}
	if len(candles) == 0 {
		// Fallback: try fetching directly from MOEX.
		moexInterval, ok := moex.StringToInterval[interval]
		if !ok {
			return nil, fmt.Errorf("unknown interval: %s", interval)
		}
		now := time.Now()
		candles, err = b.moex.GetCandlesAll(ctx, ticker, moexInterval, now.AddDate(-1, 0, 0), now)
		if err != nil {
			return nil, err
		}
		// Store for future use.
		if len(candles) > 0 {
			b.repo.BulkInsert(ctx, candles)
		}
	}
	return candles, nil
}
