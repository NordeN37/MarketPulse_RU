package domain

import "time"

// PositionSide indicates long or short.
type PositionSide string

const (
	SideLong  PositionSide = "LONG"
	SideShort PositionSide = "SHORT"
)

// PositionStatus tracks position lifecycle.
type PositionStatus string

const (
	PositionOpen   PositionStatus = "OPEN"
	PositionClosed PositionStatus = "CLOSED"
)

// Position represents an open or closed trading position.
type Position struct {
	ID       int64          `json:"id" db:"id"`
	Ticker   string         `json:"ticker" db:"ticker"`
	Side     PositionSide   `json:"side" db:"side"`
	Status   PositionStatus `json:"status" db:"status"`
	Quantity int            `json:"quantity" db:"quantity"`

	EntryPrice float64   `json:"entry_price" db:"entry_price"`
	EntryTime  time.Time `json:"entry_time" db:"entry_time"`
	EntryOrder int64     `json:"entry_order" db:"entry_order"`

	ExitPrice *float64   `json:"exit_price,omitempty" db:"exit_price"`
	ExitTime  *time.Time `json:"exit_time,omitempty" db:"exit_time"`
	ExitOrder *int64     `json:"exit_order,omitempty" db:"exit_order"`

	StopLoss   *float64 `json:"stop_loss,omitempty" db:"stop_loss"`
	TakeProfit *float64 `json:"take_profit,omitempty" db:"take_profit"`

	// Realized PnL (set on close).
	RealizedPnL *float64 `json:"realized_pnl,omitempty" db:"realized_pnl"`
	// Percentage return.
	ReturnPct *float64 `json:"return_pct,omitempty" db:"return_pct"`

	SignalID  int64     `json:"signal_id" db:"signal_id"`
	Strategy  string    `json:"strategy" db:"strategy"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// UnrealizedPnL calculates current PnL for an open position.
func (p *Position) UnrealizedPnL(currentPrice float64) float64 {
	if p.Side == SideLong {
		return (currentPrice - p.EntryPrice) * float64(p.Quantity)
	}
	return (p.EntryPrice - currentPrice) * float64(p.Quantity)
}

// UnrealizedReturnPct calculates percentage return.
func (p *Position) UnrealizedReturnPct(currentPrice float64) float64 {
	if p.EntryPrice == 0 {
		return 0
	}
	if p.Side == SideLong {
		return (currentPrice - p.EntryPrice) / p.EntryPrice * 100
	}
	return (p.EntryPrice - currentPrice) / p.EntryPrice * 100
}
