package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/backfill"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/engine"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	skipBacktest := flag.Bool("skip-backtest", false, "skip startup backfill and backtest")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.App.LogLevel == "debug" {
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	log.Info("starting MarketPulse trader",
		"mode", cfg.Trading.Mode,
		"tickers", cfg.Trading.Tickers,
		"dry_run", cfg.Trading.DryRun,
	)

	// Graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// MOEX client for quotes and candles.
	moexClient := moex.NewClient(cfg.MOEX, log)

	// Parse tickers.
	tickers := cfg.Trading.Tickers
	if len(tickers) == 0 {
		log.Error("no tickers configured for trading")
		os.Exit(1)
	}

	// Connect to PostgreSQL for backfill and persistence.
	db, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// ---- Startup Pipeline: backfill candles + read history + run backtests ----
	if !*skipBacktest {
		startupCfg := backfill.DefaultStartupConfig(cfg)
		if err := backfill.StartupPipeline(ctx, cfg, startupCfg, db, moexClient, log); err != nil {
			log.Error("startup pipeline failed", "error", err)
			// Continue to live trading anyway.
		}
	}

	// Build risk config.
	riskCfg := domain.RiskConfig{
		MaxPositionSize:   cfg.Trading.Risk.MaxPositionSize,
		MaxExposure:       cfg.Trading.Risk.MaxExposure,
		MaxSectorExposure: cfg.Trading.Risk.MaxSectorExposure,
		DailyLossLimit:    cfg.Trading.Risk.DailyLossLimit,
		DefaultStopLoss:   cfg.Trading.Risk.DefaultStopLoss,
		DefaultTakeProfit: cfg.Trading.Risk.DefaultTakeProfit,
		MaxOpenPositions:  cfg.Trading.Risk.MaxOpenPositions,
		MaxDrawdown:       cfg.Trading.Risk.MaxDrawdown,
	}

	// Build strategy config.
	strategyCfg := domain.StrategyConfig{
		Mode:              domain.TradingMode(cfg.Trading.Mode),
		NewsWeight:        cfg.Trading.Strategy.NewsWeight,
		TAWeight:          cfg.Trading.Strategy.TAWeight,
		MinSignalStrength: cfg.Trading.Strategy.MinSignalStrength,
		RequireConfluence: cfg.Trading.Strategy.RequireConfluence,
		IndicatorWeights:  cfg.Trading.Strategy.IndicatorWeights,
		Timeframes:        cfg.Trading.Strategy.Timeframes,
	}

	// Parse check interval.
	checkInterval, _ := time.ParseDuration(cfg.Trading.CheckInterval)
	if checkInterval == 0 {
		checkInterval = 5 * time.Minute
	}

	// Create engine.
	eng := engine.NewEngine(engine.Config{
		Mode:           domain.TradingMode(cfg.Trading.Mode),
		Tickers:        tickers,
		RiskConfig:     riskCfg,
		StrategyConfig: strategyCfg,
		CheckInterval:  checkInterval,
		DryRun:         cfg.Trading.DryRun,
		InitialCash:    cfg.Trading.InitialCash,
	}, moexClient, log)

	log.Info("trader engine running",
		"mode", cfg.Trading.Mode,
		"check_interval", checkInterval,
		"tickers", strings.Join(tickers, ", "),
	)

	if err := eng.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("trader engine error", "error", err)
		os.Exit(1)
	}
}
