package domain

import "time"

// Portfolio represents the current state of the trading account.
type Portfolio struct {
	ID            int64     `json:"id" db:"id"`
	Cash          float64   `json:"cash" db:"cash"`
	TotalValue    float64   `json:"total_value" db:"total_value"`
	OpenPositions int       `json:"open_positions" db:"open_positions"`
	DailyPnL      float64   `json:"daily_pnl" db:"daily_pnl"`
	TotalPnL      float64   `json:"total_pnl" db:"total_pnl"`
	MaxDrawdown   float64   `json:"max_drawdown" db:"max_drawdown"`
	WinRate       float64   `json:"win_rate" db:"win_rate"`
	TotalTrades   int       `json:"total_trades" db:"total_trades"`
	SnapshotAt    time.Time `json:"snapshot_at" db:"snapshot_at"`
}

// PortfolioRisk holds real-time risk metrics.
type PortfolioRisk struct {
	// Total exposure as fraction of portfolio value.
	Exposure float64 `json:"exposure"`
	// Maximum exposure in a single sector.
	MaxSectorExposure float64 `json:"max_sector_exposure"`
	// Sectors with open positions.
	SectorWeights map[string]float64 `json:"sector_weights"`
	// Current drawdown from peak.
	CurrentDrawdown float64 `json:"current_drawdown"`
	// Daily PnL so far.
	DailyPnL float64 `json:"daily_pnl"`
	// Number of open positions.
	OpenPositionCount int `json:"open_position_count"`
}
