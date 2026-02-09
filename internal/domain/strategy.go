package domain

// TradingMode determines which signal sources are active.
type TradingMode string

const (
	ModeNews     TradingMode = "news"
	ModeTA       TradingMode = "ta"
	ModeCombined TradingMode = "combined"
)

// MarketRegime describes current market behavior for weight adjustment.
type MarketRegime string

const (
	RegimeTrend  MarketRegime = "TREND"   // Directional move — news signals weigh more
	RegimeRange  MarketRegime = "RANGE"   // Sideways — TA signals weigh more
	RegimeCrisis MarketRegime = "CRISIS"  // High volatility — only strong news signals
)

// StrategyConfig holds the parameters for the combined decision engine.
type StrategyConfig struct {
	Mode TradingMode `yaml:"mode" json:"mode"`

	// Base weights for signal sources (adjusted by regime).
	NewsWeight float64 `yaml:"news_weight" json:"news_weight"`
	TAWeight   float64 `yaml:"ta_weight" json:"ta_weight"`

	// Minimum signal strength to generate an order.
	MinSignalStrength float64 `yaml:"min_signal_strength" json:"min_signal_strength"`

	// Confluence: require both sources to agree for stronger conviction.
	RequireConfluence bool `yaml:"require_confluence" json:"require_confluence"`

	// Indicator weights for TA scoring.
	IndicatorWeights map[string]float64 `yaml:"indicator_weights" json:"indicator_weights"`

	// Timeframes to analyze (e.g., ["5m", "1h", "1d"]).
	Timeframes []string `yaml:"timeframes" json:"timeframes"`
}

// RiskConfig holds risk management parameters.
type RiskConfig struct {
	// Maximum fraction of portfolio per single trade (e.g., 0.02 = 2%).
	MaxPositionSize float64 `yaml:"max_position_size" json:"max_position_size"`
	// Maximum total portfolio exposure (e.g., 0.5 = 50%).
	MaxExposure float64 `yaml:"max_exposure" json:"max_exposure"`
	// Maximum exposure in a single sector.
	MaxSectorExposure float64 `yaml:"max_sector_exposure" json:"max_sector_exposure"`
	// Maximum daily loss before stopping (fraction of portfolio).
	DailyLossLimit float64 `yaml:"daily_loss_limit" json:"daily_loss_limit"`
	// Default stop-loss percentage.
	DefaultStopLoss float64 `yaml:"default_stop_loss" json:"default_stop_loss"`
	// Default take-profit percentage.
	DefaultTakeProfit float64 `yaml:"default_take_profit" json:"default_take_profit"`
	// Maximum number of simultaneous open positions.
	MaxOpenPositions int `yaml:"max_open_positions" json:"max_open_positions"`
	// Maximum drawdown before circuit breaker (fraction of peak).
	MaxDrawdown float64 `yaml:"max_drawdown" json:"max_drawdown"`
}

// Candle represents an OHLCV bar.
type Candle struct {
	Ticker string  `json:"ticker" db:"ticker"`
	Open   float64 `json:"open" db:"open"`
	High   float64 `json:"high" db:"high"`
	Low    float64 `json:"low" db:"low"`
	Close  float64 `json:"close" db:"close"`
	Volume float64 `json:"volume" db:"volume"`
	// Interval: "1m", "10m", "1h", "1d"
	Interval string `json:"interval" db:"interval"`
	// Timestamp of the candle open.
	OpenTime int64 `json:"open_time" db:"open_time"`
}
