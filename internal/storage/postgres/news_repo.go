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
