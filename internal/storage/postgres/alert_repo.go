package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// AlertRepo handles alert persistence.
type AlertRepo struct {
	db *DB
}

func NewAlertRepo(db *DB) *AlertRepo {
	return &AlertRepo{db: db}
}

// Insert creates a new alert.
func (r *AlertRepo) Insert(ctx context.Context, a *domain.Alert) (int64, error) {
	var id int64
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO alerts (alert_type, entity_type, entity_id, severity, title, description, source_news_ids)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		a.AlertType, a.EntityType, a.EntityID,
		a.Severity, a.Title, a.Description, a.SourceNewsIDs,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting alert: %w", err)
	}
	return id, nil
}

// GetUnsent returns alerts that haven't been sent to Telegram yet.
func (r *AlertRepo) GetUnsent(ctx context.Context, limit int) ([]domain.Alert, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, created_at, alert_type, entity_type, entity_id, severity,
		       title, description, source_news_ids, sent_to_telegram, sent_at
		FROM alerts
		WHERE sent_to_telegram = FALSE
		ORDER BY
			CASE severity
				WHEN 'CRITICAL' THEN 0
				WHEN 'URGENT' THEN 1
				WHEN 'IMPORTANT' THEN 2
				ELSE 3
			END,
			created_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying unsent alerts: %w", err)
	}
	defer rows.Close()

	var result []domain.Alert
	for rows.Next() {
		var a domain.Alert
		if err := rows.Scan(
			&a.ID, &a.CreatedAt, &a.AlertType, &a.EntityType, &a.EntityID,
			&a.Severity, &a.Title, &a.Description, &a.SourceNewsIDs,
			&a.SentToTelegram, &a.SentAt,
		); err != nil {
			return nil, fmt.Errorf("scanning alert: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// MarkSent marks an alert as sent to Telegram.
func (r *AlertRepo) MarkSent(ctx context.Context, id int64) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE alerts SET sent_to_telegram = TRUE, sent_at = NOW()
		WHERE id = $1`, id)
	return err
}

// GetRecent returns the most recent alerts.
func (r *AlertRepo) GetRecent(ctx context.Context, limit int, minSeverity domain.AlertSeverity) ([]domain.Alert, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, created_at, alert_type, entity_type, entity_id, severity,
		       title, description, source_news_ids, sent_to_telegram, sent_at
		FROM alerts
		WHERE CASE $2
			WHEN 'INFO' THEN TRUE
			WHEN 'IMPORTANT' THEN severity IN ('IMPORTANT', 'URGENT', 'CRITICAL')
			WHEN 'URGENT' THEN severity IN ('URGENT', 'CRITICAL')
			WHEN 'CRITICAL' THEN severity = 'CRITICAL'
			ELSE TRUE
		END
		ORDER BY created_at DESC
		LIMIT $1`, limit, minSeverity)
	if err != nil {
		return nil, fmt.Errorf("querying recent alerts: %w", err)
	}
	defer rows.Close()

	var result []domain.Alert
	for rows.Next() {
		var a domain.Alert
		if err := rows.Scan(
			&a.ID, &a.CreatedAt, &a.AlertType, &a.EntityType, &a.EntityID,
			&a.Severity, &a.Title, &a.Description, &a.SourceNewsIDs,
			&a.SentToTelegram, &a.SentAt,
		); err != nil {
			return nil, fmt.Errorf("scanning alert: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetByID retrieves a single alert.
func (r *AlertRepo) GetByID(ctx context.Context, id int64) (*domain.Alert, error) {
	var a domain.Alert
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, created_at, alert_type, entity_type, entity_id, severity,
		       title, description, source_news_ids, sent_to_telegram, sent_at
		FROM alerts WHERE id = $1`, id).Scan(
		&a.ID, &a.CreatedAt, &a.AlertType, &a.EntityType, &a.EntityID,
		&a.Severity, &a.Title, &a.Description, &a.SourceNewsIDs,
		&a.SentToTelegram, &a.SentAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting alert by id: %w", err)
	}
	return &a, nil
}

// CountBySeveritySince counts alerts by severity since a given time.
func (r *AlertRepo) CountBySeveritySince(ctx context.Context, since time.Time) (map[domain.AlertSeverity]int, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT severity, COUNT(*)
		FROM alerts
		WHERE created_at >= $1
		GROUP BY severity`, since)
	if err != nil {
		return nil, fmt.Errorf("counting alerts: %w", err)
	}
	defer rows.Close()

	result := make(map[domain.AlertSeverity]int)
	for rows.Next() {
		var sev domain.AlertSeverity
		var count int
		if err := rows.Scan(&sev, &count); err != nil {
			return nil, err
		}
		result[sev] = count
	}
	return result, rows.Err()
}
