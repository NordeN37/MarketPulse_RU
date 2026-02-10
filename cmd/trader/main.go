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
	"github.com/NordeN37/MarketPulse_RU/internal/trading/selector"
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

	// Connect to PostgreSQL.
	db, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.RunMigrations(ctx, log); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// MOEX client for quotes and candles.
	moexClient := moex.NewClient(cfg.MOEX, log)

	// Build startup config — loads universe tickers from DB.
	startupCfg := backfill.DefaultStartupConfig(ctx, cfg, db)
	tickers := startupCfg.Tickers // trading tickers (subset of universe)

	log.Info("starting MarketPulse trader",
		"mode", cfg.Trading.Mode,
		"universe_tickers", len(startupCfg.UniverseTickers),
		"trading_tickers", tickers,
		"dry_run", cfg.Trading.DryRun,
	)

	// ---- Startup Pipeline: backfill candles + read history + run backtests ----
	if !*skipBacktest {
		if err := backfill.StartupPipeline(ctx, cfg, startupCfg, db, moexClient, log); err != nil {
			log.Error("startup pipeline failed", "error", err)
			// Continue to live trading anyway.
		}
	}

	// Dynamic portfolio selection: if no explicit tickers in config,
	// rank the universe by signal strength and pick top-N.
	if len(cfg.Trading.Tickers) == 0 {
		maxPos := cfg.Trading.MaxPositions
		if maxPos <= 0 {
			maxPos = 10
		}
		sel := selector.NewSelector(moexClient, log)
		dynamicTickers := sel.SelectTopN(ctx, startupCfg.UniverseTickers, maxPos, nil)
		if len(dynamicTickers) > 0 {
			tickers = dynamicTickers
			log.Info("dynamic portfolio selected",
				"count", len(tickers),
				"tickers", strings.Join(tickers, ", "),
			)
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

	// Repositories for signal consumption and position persistence.
	signalRepo := postgres.NewSignalRepo(db)
	positionRepo := postgres.NewPositionRepo(db)

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

	// Wire DB persistence: consume news signals from analyzer, persist positions.
	eng.SetSignalReader(signalRepo)
	eng.SetPositionWriter(positionRepo)
	eng.RestorePositions(ctx)

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
