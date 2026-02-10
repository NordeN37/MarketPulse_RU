package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/aggregator"
	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/classifier"
	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/scorer"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/llm"
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Connect to storage
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

	cache, err := redisclient.New(cfg.Redis)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	// Setup LLM router
	router := llm.NewRouter(cfg.LLM, log)

	providers := router.AvailableProviders(ctx)
	if len(providers) > 0 {
		log.Info("LLM providers available", "providers", providers)
	} else {
		log.Warn("no LLM providers available — analysis will fail")
	}

	// Setup repositories and components
	newsRepo := postgres.NewNewsRepo(db)
	companyRepo := postgres.NewCompanyRepo(db)
	heatRepo := postgres.NewHeatRepo(db)

	cls := classifier.NewClassifier(router, log)
	sc := scorer.NewScorer(heatRepo, log)
	agg := aggregator.NewAggregator(newsRepo, companyRepo, cls, sc, log)

	// Start worker pool
	workers := cfg.Analyzer.Workers
	if workers <= 0 {
		workers = 4
	}

	log.Info("starting analyzer workers", "count", workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runWorker(ctx, workerID, cache, newsRepo, agg, log)
		}(i)
	}

	wg.Wait()
	log.Info("analyzer service stopped")
}

func runWorker(ctx context.Context, id int, cache *redisclient.Client, newsRepo *postgres.NewsRepo, agg *aggregator.Aggregator, log *slog.Logger) {
	log = log.With("worker", id)
	log.Info("worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info("worker stopping")
			return
		default:
		}

		// Dequeue news ID from Redis
		newsID, err := cache.DequeueNews(ctx, 5*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("dequeue error", "error", err)
			time.Sleep(time.Second)
			continue
		}

		if newsID == 0 {
			continue // timeout, no items
		}

		// Get the full news item
		news, err := newsRepo.GetByID(ctx, newsID)
		if err != nil {
			log.Error("failed to get news", "news_id", newsID, "error", err)
			continue
		}
		if news == nil {
			log.Warn("news not found", "news_id", newsID)
			continue
		}

		// Process through aggregator
		if err := agg.ProcessNews(ctx, news); err != nil {
			log.Error("failed to process news",
				"news_id", newsID,
				"error", err,
			)
		}
	}
}
