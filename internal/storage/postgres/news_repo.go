package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// NewsRepo handles news persistence.
type NewsRepo struct {
	db *DB
}

func NewNewsRepo(db *DB) *NewsRepo {
	return &NewsRepo{db: db}
}

// Insert saves a new news item returning its ID. Duplicate (source, external_id) is ignored.
func (r *NewsRepo) Insert(ctx context.Context, n *domain.News) (int64, error) {
	rawJSON, _ := json.Marshal(n.RawJSON)
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO news (external_id, source, source_channel, title, content, url, media_urls, published_at, collected_at, raw_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (source, external_id) DO NOTHING
		RETURNING id`,
		n.ExternalID, n.Source, n.SourceChannel,
		n.Title, n.Content, n.URL, n.MediaURLs,
		n.PublishedAt, n.CollectedAt, rawJSON,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil // duplicate
	}
	if err != nil {
		return 0, fmt.Errorf("inserting news: %w", err)
	}
	return id, nil
}

// GetUnanalyzed returns news items that have no corresponding analysis.
func (r *NewsRepo) GetUnanalyzed(ctx context.Context, limit int) ([]domain.News, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT n.id, n.external_id, n.source, n.source_channel, n.title, n.content,
		       n.url, n.media_urls, n.published_at, n.collected_at
		FROM news n
		LEFT JOIN news_analysis na ON na.news_id = n.id
		WHERE na.id IS NULL
		ORDER BY n.collected_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying unanalyzed news: %w", err)
	}
	defer rows.Close()

	var result []domain.News
	for rows.Next() {
		var n domain.News
		if err := rows.Scan(
			&n.ID, &n.ExternalID, &n.Source, &n.SourceChannel,
			&n.Title, &n.Content, &n.URL, &n.MediaURLs,
			&n.PublishedAt, &n.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning news row: %w", err)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// GetByID retrieves a single news item.
func (r *NewsRepo) GetByID(ctx context.Context, id int64) (*domain.News, error) {
	var n domain.News
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, external_id, source, source_channel, title, content,
		       url, media_urls, published_at, collected_at
		FROM news WHERE id = $1`, id).Scan(
		&n.ID, &n.ExternalID, &n.Source, &n.SourceChannel,
		&n.Title, &n.Content, &n.URL, &n.MediaURLs,
		&n.PublishedAt, &n.CollectedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting news by id: %w", err)
	}
	return &n, nil
}

// GetRecent returns the latest news items.
func (r *NewsRepo) GetRecent(ctx context.Context, limit int, offset int) ([]domain.News, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, external_id, source, source_channel, title, content,
		       url, media_urls, published_at, collected_at
		FROM news
		ORDER BY published_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying recent news: %w", err)
	}
	defer rows.Close()

	var result []domain.News
	for rows.Next() {
		var n domain.News
		if err := rows.Scan(
			&n.ID, &n.ExternalID, &n.Source, &n.SourceChannel,
			&n.Title, &n.Content, &n.URL, &n.MediaURLs,
			&n.PublishedAt, &n.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning news: %w", err)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// ExistsByExternalID checks if a news item already exists.
func (r *NewsRepo) ExistsByExternalID(ctx context.Context, source domain.NewsSource, externalID string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM news WHERE source = $1 AND external_id = $2)`,
		source, externalID,
	).Scan(&exists)
	return exists, err
}

// SaveAnalysis persists an LLM analysis for a news item.
func (r *NewsRepo) SaveAnalysis(ctx context.Context, a *domain.NewsAnalysis) error {
	keyFacts, _ := json.Marshal(a.KeyFacts)
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO news_analysis (news_id, analyzed_at, category, sentiment, urgency, reliability_score, summary_ru, key_facts, llm_model)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (news_id) DO UPDATE SET
			analyzed_at = EXCLUDED.analyzed_at,
			category = EXCLUDED.category,
			sentiment = EXCLUDED.sentiment,
			urgency = EXCLUDED.urgency,
			reliability_score = EXCLUDED.reliability_score,
			summary_ru = EXCLUDED.summary_ru,
			key_facts = EXCLUDED.key_facts,
			llm_model = EXCLUDED.llm_model`,
		a.NewsID, a.AnalyzedAt, a.Category, a.Sentiment,
		a.Urgency, a.ReliabilityScore, a.SummaryRU, keyFacts, a.LLMModel,
	)
	return err
}

// SaveImpact persists an impact assessment.
func (r *NewsRepo) SaveImpact(ctx context.Context, imp *domain.NewsImpact) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO news_impacts (news_id, entity_type, entity_id, impact_direction, impact_magnitude, impact_timeframe, confidence, reasoning)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		imp.NewsID, imp.EntityType, imp.EntityID,
		imp.Direction, imp.Magnitude, imp.Timeframe,
		imp.Confidence, imp.Reasoning,
	)
	return err
}

// NewsWithAnalysis combines a news item with its optional LLM analysis and impacts.
type NewsWithAnalysis struct {
	domain.News
	Category  string  `json:"category,omitempty"`
	Sentiment float64 `json:"sentiment"`
	Urgency   int     `json:"urgency,omitempty"`
	SummaryRU string  `json:"summary_ru,omitempty"`
	LLMModel  string  `json:"llm_model,omitempty"`
	Analyzed  bool    `json:"analyzed"`
}

// GetRecentWithAnalysis returns news with LEFT JOIN on analysis for category/sentiment.
// Supports optional category filter and text search.
func (r *NewsRepo) GetRecentWithAnalysis(ctx context.Context, limit, offset int, category, search string) ([]NewsWithAnalysis, error) {
	query := `
		SELECT n.id, n.external_id, n.source, n.source_channel, n.title, n.content,
		       n.url, n.media_urls, n.published_at, n.collected_at,
		       COALESCE(na.category, ''), COALESCE(na.sentiment, 0),
		       COALESCE(na.urgency, 0), COALESCE(na.summary_ru, ''),
		       COALESCE(na.llm_model, ''), (na.id IS NOT NULL) AS analyzed
		FROM news n
		LEFT JOIN news_analysis na ON na.news_id = n.id
		WHERE 1=1`
	args := []any{}
	argN := 1

	if category != "" {
		query += fmt.Sprintf(" AND na.category = $%d", argN)
		args = append(args, category)
		argN++
	}
	if search != "" {
		query += fmt.Sprintf(" AND (n.title ILIKE '%%' || $%d || '%%' OR n.content ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, search)
		argN++
	}

	query += " ORDER BY n.published_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, limit, offset)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying news with analysis: %w", err)
	}
	defer rows.Close()

	var result []NewsWithAnalysis
	for rows.Next() {
		var nw NewsWithAnalysis
		if err := rows.Scan(
			&nw.ID, &nw.ExternalID, &nw.Source, &nw.SourceChannel,
			&nw.Title, &nw.Content, &nw.URL, &nw.MediaURLs,
			&nw.PublishedAt, &nw.CollectedAt,
			&nw.Category, &nw.Sentiment, &nw.Urgency,
			&nw.SummaryRU, &nw.LLMModel, &nw.Analyzed,
		); err != nil {
			return nil, fmt.Errorf("scanning news with analysis: %w", err)
		}
		result = append(result, nw)
	}
	return result, rows.Err()
}

// GetNewsByCompanyTicker returns news that impact a specific company (via news_impacts).
func (r *NewsRepo) GetNewsByCompanyTicker(ctx context.Context, ticker string, limit int, includeRelated bool) ([]NewsWithAnalysis, error) {
	query := `
		SELECT DISTINCT ON (n.id)
		       n.id, n.external_id, n.source, n.source_channel, n.title, n.content,
		       n.url, n.media_urls, n.published_at, n.collected_at,
		       COALESCE(na.category, ''), COALESCE(na.sentiment, 0),
		       COALESCE(na.urgency, 0), COALESCE(na.summary_ru, ''),
		       COALESCE(na.llm_model, ''), (na.id IS NOT NULL) AS analyzed
		FROM news n
		LEFT JOIN news_analysis na ON na.news_id = n.id
		JOIN news_impacts ni ON ni.news_id = n.id
		WHERE ni.entity_name = $1
		ORDER BY n.id, n.published_at DESC
		LIMIT $2`
	rows, err := r.db.Pool.Query(ctx, query, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("querying news by company ticker: %w", err)
	}
	defer rows.Close()

	var result []NewsWithAnalysis
	for rows.Next() {
		var nw NewsWithAnalysis
		if err := rows.Scan(
			&nw.ID, &nw.ExternalID, &nw.Source, &nw.SourceChannel,
			&nw.Title, &nw.Content, &nw.URL, &nw.MediaURLs,
			&nw.PublishedAt, &nw.CollectedAt,
			&nw.Category, &nw.Sentiment, &nw.Urgency,
			&nw.SummaryRU, &nw.LLMModel, &nw.Analyzed,
		); err != nil {
			return nil, fmt.Errorf("scanning company news: %w", err)
		}
		result = append(result, nw)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	if !includeRelated || len(result) == 0 {
		return result, nil
	}

	// Find related news: news that share impacts with the same sectors/commodities
	// as the news already found for this ticker.
	newsIDs := make([]int64, len(result))
	for i, nw := range result {
		newsIDs[i] = nw.ID
	}

	relQuery := `
		SELECT DISTINCT ON (n.id)
		       n.id, n.external_id, n.source, n.source_channel, n.title, n.content,
		       n.url, n.media_urls, n.published_at, n.collected_at,
		       COALESCE(na.category, ''), COALESCE(na.sentiment, 0),
		       COALESCE(na.urgency, 0), COALESCE(na.summary_ru, ''),
		       COALESCE(na.llm_model, ''), (na.id IS NOT NULL) AS analyzed
		FROM news n
		LEFT JOIN news_analysis na ON na.news_id = n.id
		JOIN news_impacts ni ON ni.news_id = n.id
		WHERE ni.entity_type IN ('sector', 'commodity')
		  AND ni.entity_id IN (
		      SELECT entity_id FROM news_impacts
		      WHERE news_id = ANY($1) AND entity_type IN ('sector', 'commodity')
		  )
		  AND n.id != ALL($1)
		ORDER BY n.id, n.published_at DESC
		LIMIT $2`
	relRows, err := r.db.Pool.Query(ctx, relQuery, newsIDs, limit)
	if err != nil {
		return result, nil // return direct results even if related query fails
	}
	defer relRows.Close()

	for relRows.Next() {
		var nw NewsWithAnalysis
		if err := relRows.Scan(
			&nw.ID, &nw.ExternalID, &nw.Source, &nw.SourceChannel,
			&nw.Title, &nw.Content, &nw.URL, &nw.MediaURLs,
			&nw.PublishedAt, &nw.CollectedAt,
			&nw.Category, &nw.Sentiment, &nw.Urgency,
			&nw.SummaryRU, &nw.LLMModel, &nw.Analyzed,
		); err != nil {
			break
		}
		result = append(result, nw)
	}

	return result, nil
}

// GetCategories returns all distinct categories from analyzed news.
func (r *NewsRepo) GetCategories(ctx context.Context) ([]string, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT category FROM news_analysis
		WHERE category != '' ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// ImpactRow is a simplified impact record for bulk loading.
type ImpactRow struct {
	NewsID     int64
	EntityName string
	Direction  domain.ImpactDirection
	Magnitude  float64
}

// QueryImpactsByTickers returns all impacts for the given entity names (tickers).
func (r *NewsRepo) QueryImpactsByTickers(ctx context.Context, tickers []string) ([]ImpactRow, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT news_id, entity_name, impact_direction, impact_magnitude
		FROM news_impacts
		WHERE entity_name = ANY($1)
		ORDER BY news_id`, tickers)
	if err != nil {
		return nil, fmt.Errorf("querying impacts by tickers: %w", err)
	}
	defer rows.Close()

	var result []ImpactRow
	for rows.Next() {
		var row ImpactRow
		if err := rows.Scan(&row.NewsID, &row.EntityName, &row.Direction, &row.Magnitude); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetAnalyzedNewsSince returns analyzed news with their analysis from a given time.
func (r *NewsRepo) GetAnalyzedNewsSince(ctx context.Context, since time.Time, limit int) ([]domain.News, []domain.NewsAnalysis, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT n.id, n.external_id, n.source, n.source_channel, n.title, n.content,
		       n.url, n.media_urls, n.published_at, n.collected_at,
		       na.id, na.category, na.sentiment, na.urgency, na.reliability_score,
		       na.summary_ru, na.llm_model
		FROM news n
		JOIN news_analysis na ON na.news_id = n.id
		WHERE n.published_at >= $1
		ORDER BY n.published_at DESC
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("querying analyzed news: %w", err)
	}
	defer rows.Close()

	var news []domain.News
	var analyses []domain.NewsAnalysis
	for rows.Next() {
		var n domain.News
		var a domain.NewsAnalysis
		if err := rows.Scan(
			&n.ID, &n.ExternalID, &n.Source, &n.SourceChannel,
			&n.Title, &n.Content, &n.URL, &n.MediaURLs,
			&n.PublishedAt, &n.CollectedAt,
			&a.ID, &a.Category, &a.Sentiment, &a.Urgency,
			&a.ReliabilityScore, &a.SummaryRU, &a.LLMModel,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning analyzed news: %w", err)
		}
		a.NewsID = n.ID
		news = append(news, n)
		analyses = append(analyses, a)
	}
	return news, analyses, rows.Err()
}
