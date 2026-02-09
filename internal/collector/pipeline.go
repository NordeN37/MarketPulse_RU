package collector

import (
	"context"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// Pipeline is the unified news ingestion pipeline.
// It deduplicates incoming news, stores them, and enqueues for analysis.
type Pipeline struct {
	newsRepo *postgres.NewsRepo
	cache    *redis.Client
	log      *slog.Logger
}

// NewPipeline creates a new unified news processing pipeline.
func NewPipeline(newsRepo *postgres.NewsRepo, cache *redis.Client, log *slog.Logger) *Pipeline {
	return &Pipeline{
		newsRepo: newsRepo,
		cache:    cache,
		log:      log,
	}
}

// HandleNews is the main entry point for all news sources.
// It deduplicates, normalizes, stores, and enqueues the news item.
func (p *Pipeline) HandleNews(ctx context.Context, news *domain.News) error {
	// 1. Dedup via Redis (fast check)
	isDup, err := p.cache.Dedup(ctx, string(news.Source), news.ExternalID, 0)
	if err != nil {
		p.log.Warn("dedup check failed, falling back to DB",
			"source", news.Source,
			"external_id", news.ExternalID,
			"error", err,
		)
	}
	if isDup {
		p.log.Debug("duplicate news skipped",
			"source", news.Source,
			"external_id", news.ExternalID,
		)
		return nil
	}

	// 2. Store in PostgreSQL
	id, err := p.newsRepo.Insert(ctx, news)
	if err != nil {
		return err
	}
	if id == 0 {
		p.log.Debug("news already exists in DB",
			"source", news.Source,
			"external_id", news.ExternalID,
		)
		return nil
	}

	p.log.Info("new news item stored",
		"id", id,
		"source", news.Source,
		"channel", news.SourceChannel,
		"title", truncate(news.Title, 80),
	)

	// 3. Enqueue for LLM analysis
	if err := p.cache.EnqueueNews(ctx, id); err != nil {
		p.log.Error("failed to enqueue news for analysis",
			"news_id", id,
			"error", err,
		)
		// Don't return error — the news is already stored
	}

	return nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
