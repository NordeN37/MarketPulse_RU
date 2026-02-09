package strategy

import (
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// BacktestResult holds the outcome of a strategy backtest.
type BacktestResult struct {
	Strategy    string
	Ticker      string
	Period      string
	TotalTrades int
	Winners     int
	Losers      int
	WinRate     float64
	TotalPnL    float64
	MaxDrawdown float64
	SharpeRatio float64
	AvgReturn   float64
	AvgHold     time.Duration
	Trades      []BacktestTrade
}

// BacktestTrade represents a single trade in backtesting.
type BacktestTrade struct {
	EntryIdx   int
	ExitIdx    int
	Direction  domain.SignalDirection
	EntryPrice float64
	ExitPrice  float64
	PnL        float64
	ReturnPct  float64
}

// Backtest runs a simple signal-based backtest on historical candles.
// signalFn receives candles up to index i and returns a signal direction and strength.
// Returns performance metrics.
func Backtest(
	candles []domain.Candle,
	signalFn func(candles []domain.Candle, i int) (domain.SignalDirection, float64),
	stopLossPct, takeProfitPct float64,
) BacktestResult {
	result := BacktestResult{
		Trades: make([]BacktestTrade, 0),
	}
	n := len(candles)
	if n < 50 {
		return result
	}

	var inPosition bool
	var trade BacktestTrade
	var peakEquity, equity, maxDD float64
	equity = 100000.0 // starting capital for normalization
	peakEquity = equity

	for i := 50; i < n; i++ {
		if inPosition {
			// Check stop-loss / take-profit.
			var exitPrice float64
			shouldExit := false

			if trade.Direction == domain.SignalBuy {
				if candles[i].Low <= trade.EntryPrice*(1-stopLossPct) {
					exitPrice = trade.EntryPrice * (1 - stopLossPct)
					shouldExit = true
				} else if candles[i].High >= trade.EntryPrice*(1+takeProfitPct) {
					exitPrice = trade.EntryPrice * (1 + takeProfitPct)
					shouldExit = true
				}
			} else {
				if candles[i].High >= trade.EntryPrice*(1+stopLossPct) {
					exitPrice = trade.EntryPrice * (1 + stopLossPct)
					shouldExit = true
				} else if candles[i].Low <= trade.EntryPrice*(1-takeProfitPct) {
					exitPrice = trade.EntryPrice * (1 - takeProfitPct)
					shouldExit = true
				}
			}

			// Check for opposing signal.
			if !shouldExit {
				dir, _ := signalFn(candles[:i+1], i)
				if (trade.Direction == domain.SignalBuy && dir == domain.SignalSell) ||
					(trade.Direction == domain.SignalSell && dir == domain.SignalBuy) {
					exitPrice = candles[i].Close
					shouldExit = true
				}
			}

			if shouldExit {
				trade.ExitIdx = i
				trade.ExitPrice = exitPrice
				if trade.Direction == domain.SignalBuy {
					trade.PnL = exitPrice - trade.EntryPrice
				} else {
					trade.PnL = trade.EntryPrice - exitPrice
				}
				trade.ReturnPct = trade.PnL / trade.EntryPrice * 100

				equity += trade.PnL * 100 // simplified
				if equity > peakEquity {
					peakEquity = equity
				}
				dd := (peakEquity - equity) / peakEquity
				if dd > maxDD {
					maxDD = dd
				}

				result.Trades = append(result.Trades, trade)
				if trade.PnL > 0 {
					result.Winners++
				} else {
					result.Losers++
				}
				result.TotalPnL += trade.PnL

				inPosition = false
			}
			continue
		}

		// Look for new signal.
		dir, strength := signalFn(candles[:i+1], i)
		if (dir == domain.SignalBuy || dir == domain.SignalSell) && strength > 0.3 {
			trade = BacktestTrade{
				EntryIdx:   i,
				Direction:  dir,
				EntryPrice: candles[i].Close,
			}
			inPosition = true
		}
	}

	// Close any open position at end.
	if inPosition {
		trade.ExitIdx = n - 1
		trade.ExitPrice = candles[n-1].Close
		if trade.Direction == domain.SignalBuy {
			trade.PnL = trade.ExitPrice - trade.EntryPrice
		} else {
			trade.PnL = trade.EntryPrice - trade.ExitPrice
		}
		trade.ReturnPct = trade.PnL / trade.EntryPrice * 100
		result.Trades = append(result.Trades, trade)
		if trade.PnL > 0 {
			result.Winners++
		} else {
			result.Losers++
		}
		result.TotalPnL += trade.PnL
	}

	result.TotalTrades = result.Winners + result.Losers
	if result.TotalTrades > 0 {
		result.WinRate = float64(result.Winners) / float64(result.TotalTrades)
		result.AvgReturn = result.TotalPnL / float64(result.TotalTrades)
	}
	result.MaxDrawdown = maxDD

	return result
}
