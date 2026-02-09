package domain

import "time"

// SubscriptionTier defines the user's subscription level.
type SubscriptionTier string

const (
	TierFree       SubscriptionTier = "FREE"
	TierBasic      SubscriptionTier = "BASIC"
	TierPro        SubscriptionTier = "PRO"
	TierEnterprise SubscriptionTier = "ENTERPRISE"
)

// User represents a system user.
type User struct {
	ID             int64            `json:"id" db:"id"`
	TelegramID     int64            `json:"telegram_id,omitempty" db:"telegram_id"`
	TelegramUser   string           `json:"telegram_user,omitempty" db:"telegram_user"`
	Email          string           `json:"email,omitempty" db:"email"`
	Tier           SubscriptionTier `json:"tier" db:"tier"`
	IsActive       bool             `json:"is_active" db:"is_active"`
	CreatedAt      time.Time        `json:"created_at" db:"created_at"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty" db:"expires_at"`
}

// WatchlistItem links a user to a specific entity they want to track.
type WatchlistItem struct {
	ID         int64      `json:"id" db:"id"`
	UserID     int64      `json:"user_id" db:"user_id"`
	EntityType EntityType `json:"entity_type" db:"entity_type"`
	EntityID   int64      `json:"entity_id" db:"entity_id"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// AlertPreference stores user's notification preferences.
type AlertPreference struct {
	ID            int64         `json:"id" db:"id"`
	UserID        int64         `json:"user_id" db:"user_id"`
	MinSeverity   AlertSeverity `json:"min_severity" db:"min_severity"`
	EnableTelegram bool         `json:"enable_telegram" db:"enable_telegram"`
	QuietHoursStart *int        `json:"quiet_hours_start,omitempty" db:"quiet_hours_start"` // hour 0-23
	QuietHoursEnd   *int        `json:"quiet_hours_end,omitempty" db:"quiet_hours_end"`
}
