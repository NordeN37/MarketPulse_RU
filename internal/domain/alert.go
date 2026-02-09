package domain

import "time"

// AlertSeverity indicates how urgent the alert is.
type AlertSeverity string

const (
	SeverityInfo      AlertSeverity = "INFO"
	SeverityImportant AlertSeverity = "IMPORTANT"
	SeverityUrgent    AlertSeverity = "URGENT"
	SeverityCritical  AlertSeverity = "CRITICAL"
)

// Alert represents a notification generated from news analysis.
type Alert struct {
	ID             int64         `json:"id" db:"id"`
	CreatedAt      time.Time     `json:"created_at" db:"created_at"`
	AlertType      string        `json:"alert_type" db:"alert_type"`
	EntityType     EntityType    `json:"entity_type" db:"entity_type"`
	EntityID       int64         `json:"entity_id" db:"entity_id"`
	Severity       AlertSeverity `json:"severity" db:"severity"`
	Title          string        `json:"title" db:"title"`
	Description    string        `json:"description" db:"description"`
	SourceNewsIDs  []int64       `json:"source_news_ids" db:"source_news_ids"`
	SentToTelegram bool          `json:"sent_to_telegram" db:"sent_to_telegram"`
	SentAt         *time.Time    `json:"sent_at,omitempty" db:"sent_at"`
}
