package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/collector/scraper"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	redisclient "github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "show what would be updated without making changes")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	cache, err := redisclient.New(cfg.Redis)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	fetcher := scraper.NewArticleFetcher(log)

	// Find TG news with URL and short content (< 300 chars).
	rows, err := db.Pool.Query(ctx, `
		SELECT id, url, content
		FROM news
		WHERE source = 'telegram'
		  AND url IS NOT NULL AND url != ''
		  AND length(content) < 300
		ORDER BY id`)
	if err != nil {
		log.Error("query failed", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type item struct {
		id      int64
		url     string
		content string
	}

	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.url, &it.content); err != nil {
			log.Error("scan error", "error", err)
			os.Exit(1)
		}
		items = append(items, it)
	}
	rows.Close()

	log.Info("found TG news with URLs and short content", "count", len(items))
	if len(items) == 0 {
		log.Info("nothing to enrich")
		return
	}

	var enriched, failed, skipped int

	for _, it := range items {
		article := fetcher.FetchArticle(ctx, it.url)
		if article == "" {
			log.Info("no article extracted", "id", it.id, "url", it.url)
			skipped++
			continue
		}

		if len([]rune(article)) <= len([]rune(it.content)) {
			log.Info("scraped article not longer than original", "id", it.id)
			skipped++
			continue
		}

		if *dryRun {
			log.Info("would enrich",
				"id", it.id,
				"url", it.url,
				"original_len", len([]rune(it.content)),
				"scraped_len", len([]rune(article)),
			)
			enriched++
			continue
		}

		// Update content in DB.
		_, err := db.Pool.Exec(ctx, `UPDATE news SET content = $1 WHERE id = $2`, article, it.id)
		if err != nil {
			log.Error("update failed", "id", it.id, "error", err)
			failed++
			continue
		}

		// Delete old analysis so news gets re-analyzed.
		_, _ = db.Pool.Exec(ctx, `DELETE FROM news_analysis WHERE news_id = $1`, it.id)

		// Re-enqueue for analysis.
		if err := cache.EnqueueNews(ctx, it.id); err != nil {
			log.Warn("re-enqueue failed", "id", it.id, "error", err)
		}

		enriched++
		log.Info("enriched",
			"id", it.id,
			"url", it.url,
			"original_len", len([]rune(it.content)),
			"scraped_len", len([]rune(article)),
		)

		// Small delay to avoid hammering external sites.
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\nDone: enriched=%d  skipped=%d  failed=%d  total=%d\n", enriched, skipped, failed, len(items))
}
