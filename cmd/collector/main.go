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
	"github.com/NordeN37/MarketPulse_RU/internal/collector/scraper"
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

	// Create pipeline with article scraper for fetching full article bodies.
	newsRepo := postgres.NewNewsRepo(db)
	articleFetcher := scraper.NewArticleFetcher(log)
	pipeline := collector.NewPipeline(newsRepo, cache, log).WithScraper(articleFetcher)

	var wg sync.WaitGroup

	// Start Telegram userbot (MTProto) for channel monitoring
	if cfg.Telegram.APIID != 0 && cfg.Telegram.APIHash != "" {
		authBridge := telegram.NewAuthBridge(cache.RDB(), cfg.Telegram.Phone, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			userbot := telegram.NewUserbot(cfg.Telegram, pipeline.HandleNews, log).WithAuthBridge(authBridge)
			if err := userbot.Run(ctx); err != nil {
				log.Error("telegram userbot error", "error", err)
			}
		}()
		log.Info("telegram userbot (MTProto) started — auth code via admin panel if needed")
	} else {
		log.Warn("telegram MTProto not configured (set api_id, api_hash, phone), skipping channel monitoring")
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
