package domain

import "time"

// Trade represents a completed (closed) trade for analytics.
type Trade struct {
	ID         int64     `json:"id" db:"id"`
	Ticker     string    `json:"ticker" db:"ticker"`
	Side       string    `json:"side" db:"side"`
	Strategy   string    `json:"strategy" db:"strategy"`
	Quantity   int       `json:"quantity" db:"quantity"`
	EntryPrice float64   `json:"entry_price" db:"entry_price"`
	ExitPrice  float64   `json:"exit_price" db:"exit_price"`
	EntryTime  time.Time `json:"entry_time" db:"entry_time"`
	ExitTime   time.Time `json:"exit_time" db:"exit_time"`
	PnL        float64   `json:"pnl" db:"pnl"`
	ReturnPct  float64   `json:"return_pct" db:"return_pct"`
	Commission float64   `json:"commission" db:"commission"`
	// Duration of the trade.
	HoldingTime time.Duration `json:"holding_time" db:"-"`
	PositionID  int64         `json:"position_id" db:"position_id"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
}
