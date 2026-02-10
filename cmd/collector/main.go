package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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

	if err := db.RunMigrations(ctx, log); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

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

	// Shared auth bridge (used by both live listener and reread)
	var authBridge *telegram.AuthBridge
	if cfg.Telegram.Phone != "" {
		authBridge = telegram.NewAuthBridge(cache.RDB(), cfg.Telegram.Phone, log)
	}

	// Start Telegram userbot (MTProto) for channel monitoring
	if cfg.Telegram.APIID != 0 && cfg.Telegram.APIHash != "" {
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

	// Listen for admin re-read commands via Redis pub/sub
	rereadCh, err := cache.SubscribeReread(ctx)
	if err != nil {
		log.Error("failed to subscribe to reread commands", "error", err)
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for payload := range rereadCh {
				if ctx.Err() != nil {
					return
				}
				// Format: "source_type:channel_name"
				parts := splitOnce(payload, ":")
				if len(parts) != 2 {
					log.Warn("invalid reread command", "payload", payload)
					continue
				}
				srcType, channel := parts[0], parts[1]
				log.Info("received reread command", "type", srcType, "channel", channel)

				switch srcType {
				case "rss":
					feed, found := rss.FindFeedByChannel(channel)
					if !found {
						log.Warn("reread: RSS feed not found", "channel", channel)
						continue
					}
					_, total, fetchErr := rss.FetchOneFeed(ctx, feed, pipeline.HandleNews, log)
					if fetchErr != nil {
						log.Error("reread RSS failed", "feed", feed.Name, "error", fetchErr)
					} else {
						log.Info("reread RSS completed", "feed", feed.Name, "items", total)
					}

				case "telegram":
					if cfg.Telegram.APIID == 0 || cfg.Telegram.APIHash == "" {
						log.Warn("reread: Telegram not configured")
						continue
					}
					// Override channels to just the one requested
					singleCfg := cfg.Telegram
					singleCfg.Channels = []string{channel}
					histReader := telegram.NewHistoryReader(singleCfg, pipeline.HandleNews, log)
					if authBridge != nil {
						histReader = histReader.WithAuthBridge(authBridge)
					}
					if histErr := histReader.ReadHistory(ctx, 200); histErr != nil {
						log.Error("reread Telegram failed", "channel", channel, "error", histErr)
					} else {
						log.Info("reread Telegram completed", "channel", channel)
					}
				}
			}
		}()
		log.Info("reread command listener started")
	}

	log.Info("collector service running")
	wg.Wait()
	log.Info("collector service stopped")
}

func splitOnce(s, sep string) []string {
	idx := strings.Index(s, sep)
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+len(sep):]}
}
