package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/NordeN37/MarketPulse_RU/internal/collector"
	"github.com/NordeN37/MarketPulse_RU/internal/collector/rss"
	"github.com/NordeN37/MarketPulse_RU/internal/collector/telegram"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	redisclient "github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Connect to PostgreSQL
	db, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Connect to Redis
	cache, err := redisclient.New(cfg.Redis)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	// Create pipeline
	newsRepo := postgres.NewNewsRepo(db)
	pipeline := collector.NewPipeline(newsRepo, cache, log)

	var wg sync.WaitGroup

	// Start Telegram listener (if configured)
	if cfg.Telegram.BotToken != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener := telegram.NewListener(cfg.Telegram, pipeline.HandleNews, log)
			if err := listener.Run(ctx); err != nil {
				log.Error("telegram listener error", "error", err)
			}
		}()
		log.Info("telegram listener started")
	} else {
		log.Warn("telegram not configured, skipping")
	}

	// Start RSS fetcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		feeds := rss.DefaultFeeds()
		fetcher := rss.NewFetcher(feeds, pipeline.HandleNews, cfg.Collector.PollInterval(), log)
		if err := fetcher.Run(ctx); err != nil {
			log.Error("RSS fetcher error", "error", err)
		}
	}()
	log.Info("RSS fetcher started")

	log.Info("collector service running")
	wg.Wait()
	log.Info("collector service stopped")
}
