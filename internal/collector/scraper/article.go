package scraper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

const (
	maxBodySize     = 2 * 1024 * 1024 // 2 MB HTML limit
	maxContentLen   = 10000            // max chars of extracted text
	defaultTimeout  = 8 * time.Second
	minContentForFetch = 300 // fetch full article only if existing content is shorter than this
)

// ArticleFetcher downloads and extracts article text from URLs.
type ArticleFetcher struct {
	client *http.Client
	log    *slog.Logger
}

// NewArticleFetcher creates a fetcher with the given timeout.
func NewArticleFetcher(log *slog.Logger) *ArticleFetcher {
	return &ArticleFetcher{
		client: &http.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		log: log,
	}
}

// FetchArticle downloads the URL and extracts article body text.
// Returns empty string on any error (non-fatal).
func (f *ArticleFetcher) FetchArticle(ctx context.Context, url string) string {
	if url == "" {
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		f.log.Debug("scraper: invalid URL", "url", url, "error", err)
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketPulse/1.0; +https://github.com/NordeN37/MarketPulse_RU)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.5")

	resp, err := f.client.Do(req)
	if err != nil {
		f.log.Debug("scraper: fetch failed", "url", url, "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.log.Debug("scraper: non-200 status", "url", url, "status", resp.StatusCode)
		return ""
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		f.log.Debug("scraper: not HTML", "url", url, "content_type", ct)
		return ""
	}

	body := io.LimitReader(resp.Body, maxBodySize)
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		f.log.Debug("scraper: parse error", "url", url, "error", err)
		return ""
	}

	text := extractArticleText(doc)
	if len([]rune(text)) > maxContentLen {
		text = string([]rune(text)[:maxContentLen])
	}
	return text
}

// ShouldFetch returns true if the existing content is short enough to warrant fetching the full article.
func ShouldFetch(url, existingContent string) bool {
	if url == "" {
		return false
	}
	// Only fetch for HTTP(S) URLs.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	return len([]rune(existingContent)) < minContentForFetch
}

// extractArticleText tries to find article content in common HTML structures.
func extractArticleText(doc *goquery.Document) string {
	// Remove noise elements.
	doc.Find("script, style, nav, header, footer, aside, .sidebar, .menu, .nav, .comments, .social, .share, .ad, .banner, .cookie, noscript, iframe").Remove()

	// Try article-specific selectors in priority order.
	selectors := []string{
		"article .article-body",
		"article .article__body",
		"article .article-text",
		"article .article__text",
		".article-body",
		".article__body",
		".article-text",
		".article__text",
		".story-body",
		".post-content",
		".entry-content",
		"[itemprop=articleBody]",
		"article",
		"main",
		".content",
		"#content",
	}

	for _, sel := range selectors {
		s := doc.Find(sel)
		if s.Length() > 0 {
			text := cleanText(s.First().Text())
			if len(text) > 100 {
				return text
			}
		}
	}

	// Fallback: get body text.
	text := cleanText(doc.Find("body").Text())
	return text
}

// cleanText normalizes whitespace in extracted text.
func cleanText(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	prevSpace := false
	prevNewline := false
	for _, r := range s {
		if r == '\n' || r == '\r' {
			if !prevNewline {
				b.WriteRune('\n')
				prevNewline = true
				prevSpace = true
			}
			continue
		}
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
		prevNewline = false
	}

	// Collapse multiple newlines to max 2.
	text := b.String()
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}
