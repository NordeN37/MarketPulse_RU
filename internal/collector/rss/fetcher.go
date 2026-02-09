package rss

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// Feed defines an RSS feed source.
type Feed struct {
	Name    string
	URL     string
	Channel string // logical channel name for grouping
}

// DefaultFeeds returns standard Russian financial RSS feeds.
func DefaultFeeds() []Feed {
	return []Feed{
		{Name: "РБК", URL: "https://rssexport.rbc.ru/rbcnews/news/30/full.rss", Channel: "rbc"},
		{Name: "Интерфакс", URL: "https://www.interfax.ru/rss.asp", Channel: "interfax"},
		{Name: "ТАСС Экономика", URL: "https://tass.ru/rss/v2.xml", Channel: "tass"},
		{Name: "Ведомости", URL: "https://www.vedomosti.ru/rss/news", Channel: "vedomosti"},
		{Name: "Банк России", URL: "https://cbr.ru/rss/eventrss", Channel: "cbr"},
	}
}

// MessageHandler is called for each fetched item.
type MessageHandler func(ctx context.Context, news *domain.News) error

// Fetcher polls RSS feeds on a schedule.
type Fetcher struct {
	feeds    []Feed
	handler  MessageHandler
	interval time.Duration
	parser   *gofeed.Parser
	log      *slog.Logger
}

// NewFetcher creates a new RSS feed fetcher.
func NewFetcher(feeds []Feed, handler MessageHandler, interval time.Duration, log *slog.Logger) *Fetcher {
	return &Fetcher{
		feeds:    feeds,
		handler:  handler,
		interval: interval,
		parser:   gofeed.NewParser(),
		log:      log,
	}
}

// Run starts polling feeds periodically. Blocks until context is cancelled.
func (f *Fetcher) Run(ctx context.Context) error {
	f.log.Info("starting RSS fetcher", "feeds", len(f.feeds), "interval", f.interval)

	// Initial fetch
	f.fetchAll(ctx)

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			f.log.Info("RSS fetcher shutting down")
			return nil
		case <-ticker.C:
			f.fetchAll(ctx)
		}
	}
}

func (f *Fetcher) fetchAll(ctx context.Context) {
	for _, feed := range f.feeds {
		if ctx.Err() != nil {
			return
		}
		if err := f.fetchFeed(ctx, feed); err != nil {
			f.log.Error("failed to fetch RSS feed",
				"feed", feed.Name,
				"url", feed.URL,
				"error", err,
			)
		}
	}
}

func (f *Fetcher) fetchFeed(ctx context.Context, feed Feed) error {
	parsedFeed, err := f.parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return fmt.Errorf("parsing feed %s: %w", feed.Name, err)
	}

	f.log.Debug("fetched RSS feed",
		"feed", feed.Name,
		"items", len(parsedFeed.Items),
	)

	for _, item := range parsedFeed.Items {
		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		news := &domain.News{
			ExternalID:    item.GUID,
			Source:        domain.SourceRSS,
			SourceChannel: feed.Channel,
			Title:         item.Title,
			Content:       item.Description,
			URL:           item.Link,
			PublishedAt:   publishedAt,
			CollectedAt:   time.Now(),
		}

		if err := f.handler(ctx, news); err != nil {
			f.log.Error("failed to handle RSS item",
				"feed", feed.Name,
				"guid", item.GUID,
				"error", err,
			)
		}
	}

	return nil
}
