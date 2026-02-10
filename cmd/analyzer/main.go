package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
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
	batch := flag.Bool("batch", false, "batch mode: force API providers (skip Ollama)")
	batchWorkers := flag.Int("workers", 0, "override worker count (0 = use config)")
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

	if *batch {
		router.SetForceHeavy(true)
		router.Stats.SetMode("batch")
		log.Info("BATCH MODE: all tasks routed to API providers (Qwen-Plus → DeepSeek)")
	}

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
	signalRepo := postgres.NewSignalRepo(db)

	cls := classifier.NewClassifier(router, log)
	sc := scorer.NewScorer(heatRepo, log)
	agg := aggregator.NewAggregator(newsRepo, companyRepo, signalRepo, cls, router, sc, log)

	// Start worker pool
	workers := cfg.Analyzer.Workers
	if *batchWorkers > 0 {
		workers = *batchWorkers
	}
	if workers <= 0 {
		workers = 4
	}

	log.Info("starting analyzer workers", "count", workers)

	var stats analyzerStats
	stats.startedAt = time.Now()

	// Periodic stats reporter + Redis stats writer
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ok := stats.processed.Load()
				fail := stats.errors.Load()
				total := ok + fail
				if total == 0 {
					log.Info("analyzer: ожидание новостей в очереди...")
				} else {
					elapsed := time.Since(stats.startedAt).Seconds()
					log.Info("analyzer: статистика",
						"обработано", ok,
						"ошибок", fail,
						"скорость", fmt.Sprintf("%.1f/мин", float64(total)/elapsed*60),
					)
				}
				// Write LLM stats to Redis for the admin UI
				if data, err := router.Stats.ToJSON(); err == nil {
					if wErr := cache.WriteLLMStats(ctx, data); wErr != nil {
						log.Debug("failed to write LLM stats to Redis", "error", wErr)
					}
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runWorker(ctx, workerID, cache, newsRepo, agg, &stats, log)
		}(i)
	}

	wg.Wait()

	ok := stats.processed.Load()
	fail := stats.errors.Load()
	log.Info("analyzer service stopped", "обработано", ok, "ошибок", fail)
}

type analyzerStats struct {
	processed atomic.Int64
	errors    atomic.Int64
	startedAt time.Time
}

func runWorker(ctx context.Context, id int, cache *redisclient.Client, newsRepo *postgres.NewsRepo, agg *aggregator.Aggregator, stats *analyzerStats, log *slog.Logger) {
	log = log.With("w", id)
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
			stats.errors.Add(1)
			continue
		}
		if news == nil {
			log.Warn("news not found", "news_id", newsID)
			continue
		}

		// Compact preview for logging
		preview := truncate(news.Title, 60)
		if preview == "" {
			preview = truncate(news.Content, 60)
		}

		start := time.Now()
		log.Info("анализ",
			"id", newsID,
			"src", news.SourceChannel,
			"text", preview,
		)

		// Process through aggregator
		if err := agg.ProcessNews(ctx, news); err != nil {
			log.Error("ошибка анализа",
				"id", newsID,
				"error", err,
				"ms", time.Since(start).Milliseconds(),
			)
			stats.errors.Add(1)
			continue
		}

		stats.processed.Add(1)
		log.Info("готово",
			"id", newsID,
			"ms", time.Since(start).Milliseconds(),
		)
	}
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
