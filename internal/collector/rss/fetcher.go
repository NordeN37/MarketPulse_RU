package rss

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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

// DefaultFeeds returns standard financial RSS feeds.
// Russian sources provide full-text articles; international sources provide headlines/excerpts (paywall).
// Bloomberg and Reuters have no native RSS; we use Google News RSS filtered by domain.
func DefaultFeeds() []Feed {
	return []Feed{
		// ── Российские источники (полные тексты) ─────────────────
		{Name: "РБК", URL: "https://rssexport.rbc.ru/rbcnews/news/30/full.rss", Channel: "rbc"},
		{Name: "Интерфакс", URL: "https://www.interfax.ru/rss.asp", Channel: "interfax"},
		{Name: "ТАСС Экономика", URL: "https://tass.ru/rss/v2.xml", Channel: "tass"},
		{Name: "Ведомости", URL: "https://www.vedomosti.ru/rss/news", Channel: "vedomosti"},
		{Name: "Банк России", URL: "https://cbr.ru/rss/eventrss", Channel: "cbr"},

		// ПРАЙМ (1prime.ru) — агентство экономической информации
		{Name: "ПРАЙМ", URL: "https://1prime.ru/export/rss2/index.xml", Channel: "prime"},
		{Name: "ПРАЙМ Рынки", URL: "https://1prime.ru/export/rss2/Financial_market/index.xml", Channel: "prime_markets"},
		{Name: "ПРАЙМ Экономика", URL: "https://1prime.ru/export/rss2/state_regulation/index.xml", Channel: "prime_economy"},
		{Name: "ПРАЙМ Энергетика", URL: "https://1prime.ru/export/rss2/energy/index.xml", Channel: "prime_energy"},

		// Коммерсантъ
		{Name: "Коммерсантъ", URL: "https://www.kommersant.ru/RSS/main.xml", Channel: "kommersant"},
		{Name: "Коммерсантъ Новости", URL: "https://www.kommersant.ru/RSS/news.xml", Channel: "kommersant_news"},

		// РИА Новости (единый фид, экономика включена)
		{Name: "РИА Новости", URL: "https://ria.ru/export/rss2/archive/index.xml", Channel: "ria"},

		// Известия
		{Name: "Известия", URL: "https://iz.ru/xml/rss/all.xml", Channel: "izvestia"},

		// ── Международные источники (заголовки, англ.) ───────────
		// Wall Street Journal
		{Name: "WSJ Markets", URL: "https://feeds.a.dj.com/rss/RSSMarketsMain.xml", Channel: "wsj_markets"},
		{Name: "WSJ Business", URL: "https://feeds.a.dj.com/rss/WSJcomUSBusiness.xml", Channel: "wsj_business"},
		{Name: "WSJ World", URL: "https://feeds.a.dj.com/rss/RSSWorldNews.xml", Channel: "wsj_world"},

		// Financial Times
		{Name: "FT Markets", URL: "https://www.ft.com/markets?format=rss", Channel: "ft_markets"},
		{Name: "FT Companies", URL: "https://www.ft.com/companies?format=rss", Channel: "ft_companies"},
		{Name: "FT Economy", URL: "https://www.ft.com/global-economy?format=rss", Channel: "ft_economy"},
		{Name: "FT Emerging Markets", URL: "https://www.ft.com/emerging-markets?format=rss", Channel: "ft_emerging"},

		// Bloomberg (нет нативного RSS — используем Google News как прокси)
		{Name: "Bloomberg via Google", URL: "https://news.google.com/rss/search?q=when:24h+allinurl:bloomberg.com&ceid=US:en&hl=en-US&gl=US", Channel: "bloomberg"},

		// Reuters (убрали RSS в 2020 — используем Google News как прокси)
		{Name: "Reuters via Google", URL: "https://news.google.com/rss/search?q=when:24h+allinurl:reuters.com&ceid=US:en&hl=en-US&gl=US", Channel: "reuters"},
		// Reuters Россия — фильтр по русскоязычным финансовым новостям Reuters
		{Name: "Reuters Россия via Google", URL: "https://news.google.com/rss/search?q=when:24h+allinurl:reuters.com+%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F+OR+%D1%80%D1%83%D0%B1%D0%BB%D1%8C+OR+%D0%9C%D0%BE%D1%81%D0%BA%D0%B2%D0%B0&ceid=US:en&hl=ru&gl=RU", Channel: "reuters_russia"},
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
	// Custom HTTP client with aggressive timeouts to avoid hanging on slow/blocked hosts.
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	parser := gofeed.NewParser()
	parser.Client = httpClient

	return &Fetcher{
		feeds:    feeds,
		handler:  handler,
		interval: interval,
		parser:   parser,
		log:      log,
	}
}

// FetchOneFeed fetches a single RSS feed and passes items through the handler.
// Returns (new items count, total items, error).
// Designed for one-shot admin re-reads — deduplication is handled by the handler/pipeline.
func FetchOneFeed(ctx context.Context, feed Feed, handler MessageHandler, log *slog.Logger) (int, int, error) {
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	parser := gofeed.NewParser()
	parser.Client = httpClient

	parsedFeed, err := parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing feed %s: %w", feed.Name, err)
	}

	var newCount int
	for _, item := range parsedFeed.Items {
		if ctx.Err() != nil {
			break
		}

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

		if err := handler(ctx, news); err != nil {
			log.Warn("handler error during reread",
				"feed", feed.Name,
				"guid", item.GUID,
				"error", err,
			)
		}
		newCount++ // actual new vs dup is tracked by pipeline
	}

	return newCount, len(parsedFeed.Items), nil
}

// FindFeedByChannel finds a feed by its channel name from DefaultFeeds.
func FindFeedByChannel(channel string) (Feed, bool) {
	for _, f := range DefaultFeeds() {
		if f.Channel == channel {
			return f, true
		}
	}
	return Feed{}, false
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
	var ok, fail int
	for _, feed := range f.feeds {
		if ctx.Err() != nil {
			return
		}
		if err := f.fetchFeed(ctx, feed); err != nil {
			fail++
			f.log.Warn("failed to fetch RSS feed (will retry next cycle)",
				"feed", feed.Name,
				"error", err,
			)
		} else {
			ok++
		}
	}
	f.log.Info("RSS fetch cycle complete", "ok", ok, "failed", fail, "total", ok+fail)
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
