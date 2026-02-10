package collector

import (
	"context"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/collector/scraper"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// Pipeline is the unified news ingestion pipeline.
// It deduplicates incoming news, stores them, and enqueues for analysis.
type Pipeline struct {
	newsRepo *postgres.NewsRepo
	cache    *redis.Client
	scraper  *scraper.ArticleFetcher
	log      *slog.Logger
}

// NewPipeline creates a new unified news processing pipeline.
// If scraper is nil, article body fetching is disabled.
func NewPipeline(newsRepo *postgres.NewsRepo, cache *redis.Client, log *slog.Logger) *Pipeline {
	return &Pipeline{
		newsRepo: newsRepo,
		cache:    cache,
		log:      log,
	}
}

// WithScraper enables automatic article body fetching for news with URLs.
func (p *Pipeline) WithScraper(s *scraper.ArticleFetcher) *Pipeline {
	p.scraper = s
	return p
}

// HandleNews is the main entry point for all news sources.
// It deduplicates, normalizes, stores, and enqueues the news item.
func (p *Pipeline) HandleNews(ctx context.Context, news *domain.News) error {
	// 1. Dedup via Redis (fast check), skip if cache is nil.
	if p.cache != nil {
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
	}

	// 2. Enrich: fetch full article body if URL present and content is short.
	if p.scraper != nil && scraper.ShouldFetch(news.URL, news.Content) {
		article := p.scraper.FetchArticle(ctx, news.URL)
		if article != "" {
			p.log.Debug("scraped full article body",
				"url", news.URL,
				"original_len", len(news.Content),
				"scraped_len", len(article),
			)
			news.Content = article
		}
	}

	// 3. Store in PostgreSQL
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

	// 4. Enqueue for LLM analysis (skip if cache is nil).
	if p.cache != nil {
		if err := p.cache.EnqueueNews(ctx, id); err != nil {
			p.log.Error("failed to enqueue news for analysis",
				"news_id", id,
				"error", err,
			)
			// Don't return error — the news is already stored
		}
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
