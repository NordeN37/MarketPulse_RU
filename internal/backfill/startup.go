package backfill

import (
	"context"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/collector"
	"github.com/NordeN37/MarketPulse_RU/internal/collector/telegram"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// StartupConfig holds configuration for the startup pipeline.
type StartupConfig struct {
	Tickers          []string
	CandleDaysBack   int     // How many days of candle history to fetch.
	TelegramMaxMsgs  int     // Max messages to read per channel from Telegram history.
	InitialCash      float64 // Starting capital for backtesting.
	StopLossPct      float64
	TakeProfitPct    float64
}

// DefaultStartupConfig returns sensible defaults for the startup pipeline.
func DefaultStartupConfig(cfg *config.Config) StartupConfig {
	tickers := cfg.Trading.Tickers
	if len(tickers) == 0 {
		tickers = []string{"SBER", "GAZP", "LKOH", "YNDX", "GMKN"}
	}
	cash := cfg.Trading.InitialCash
	if cash <= 0 {
		cash = 50000
	}
	sl := cfg.Trading.Risk.DefaultStopLoss
	if sl <= 0 {
		sl = 0.03
	}
	tp := cfg.Trading.Risk.DefaultTakeProfit
	if tp <= 0 {
		tp = 0.06
	}
	return StartupConfig{
		Tickers:         tickers,
		CandleDaysBack:  365,
		TelegramMaxMsgs: 500,
		InitialCash:     cash,
		StopLossPct:     sl,
		TakeProfitPct:   tp,
	}
}

// StartupPipeline runs the full data ingestion and backtesting pipeline at startup.
// Steps:
// 1. Backfill MOEX candles (1 year daily)
// 2. Read Telegram channel history (optional, requires MTProto credentials)
// 3. Run backtests for all 3 strategies
// 4. Save equity curves and signals to DB
func StartupPipeline(
	ctx context.Context,
	cfg *config.Config,
	startupCfg StartupConfig,
	db *postgres.DB,
	moexClient *moex.Client,
	log *slog.Logger,
) error {
	candleRepo := postgres.NewCandleRepo(db)
	portfolioRepo := postgres.NewPortfolioRepo(db)
	tradeRepo := postgres.NewTradeRepo(db)
	signalRepo := postgres.NewSignalRepo(db)

	// Check if backtests already exist.
	existing, _ := portfolioRepo.CountByStrategy(ctx, "ta")
	if existing > 0 {
		log.Info("backtest data already exists, skipping startup pipeline",
			"existing_snapshots", existing,
		)
		return nil
	}

	// ---- Step 1: Backfill candles ----
	log.Info("step 1/3: backfilling MOEX candles...")
	candleBackfill := NewCandleBackfill(moexClient, candleRepo, log)
	if err := candleBackfill.BackfillAll(ctx, startupCfg.Tickers, moex.Interval1Day, startupCfg.CandleDaysBack); err != nil {
		log.Error("candle backfill failed", "error", err)
		// Continue anyway — we might have partial data.
	}

	// Also fetch 1h candles for more granular analysis (last 90 days).
	if err := candleBackfill.BackfillAll(ctx, startupCfg.Tickers, moex.Interval1Hour, 90); err != nil {
		log.Warn("1h candle backfill failed", "error", err)
	}

	// ---- Step 2: Read Telegram history (optional) ----
	if cfg.Telegram.APIID != 0 && cfg.Telegram.APIHash != "" && startupCfg.TelegramMaxMsgs > 0 {
		log.Info("step 2/3: reading Telegram channel history...")
		newsRepo := postgres.NewNewsRepo(db)
		pipeline := collector.NewPipeline(newsRepo, nil, log) // nil cache = no dedup via Redis
		histReader := telegram.NewHistoryReader(cfg.Telegram, pipeline.HandleNews, log)
		if err := histReader.ReadHistory(ctx, startupCfg.TelegramMaxMsgs); err != nil {
			log.Warn("telegram history reading failed (continuing without)", "error", err)
		}
	} else {
		log.Info("step 2/3: skipping Telegram history (MTProto not configured)")
	}

	// ---- Step 3: Run backtests ----
	log.Info("step 3/3: running backtests for all strategies...")
	btService := NewBacktestService(
		candleBackfill, portfolioRepo, tradeRepo, signalRepo, log,
		startupCfg.InitialCash, startupCfg.StopLossPct, startupCfg.TakeProfitPct,
	)

	results, err := btService.RunAll(ctx, startupCfg.Tickers)
	if err != nil {
		return err
	}

	for _, r := range results {
		log.Info("backtest result",
			"strategy", r.Strategy,
			"trades", r.Result.TotalTrades,
			"pnl", r.Result.TotalPnL,
			"win_rate", r.Result.WinRate,
			"max_dd", r.Result.MaxDrawdown,
			"equity_points", len(r.EquityCurve),
		)
	}

	log.Info("startup pipeline complete")
	return nil
}
