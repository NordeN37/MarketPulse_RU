package patterns

import (
	"math"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// Pattern represents a detected candlestick or chart pattern.
type Pattern struct {
	Name      string  // e.g., "HAMMER", "ENGULFING_BULL"
	Direction int     // 1 = bullish, -1 = bearish, 0 = neutral
	Strength  float64 // 0.0 to 1.0
	Index     int     // candle index where pattern was detected
}

// bodySize returns the absolute difference between open and close.
func bodySize(c domain.Candle) float64 {
	return math.Abs(c.Close - c.Open)
}

// totalRange returns high - low.
func totalRange(c domain.Candle) float64 {
	return c.High - c.Low
}

// isBullish returns true if the candle closed above its open.
func isBullish(c domain.Candle) bool {
	return c.Close > c.Open
}

// upperWick returns the upper shadow size.
func upperWick(c domain.Candle) float64 {
	if isBullish(c) {
		return c.High - c.Close
	}
	return c.High - c.Open
}

// lowerWick returns the lower shadow size.
func lowerWick(c domain.Candle) float64 {
	if isBullish(c) {
		return c.Open - c.Low
	}
	return c.Close - c.Low
}

// avgBody returns the average body size for the last n candles before index i.
func avgBody(candles []domain.Candle, i, n int) float64 {
	if i < n {
		return 0
	}
	var sum float64
	for j := i - n; j < i; j++ {
		sum += bodySize(candles[j])
	}
	return sum / float64(n)
}

// DetectCandlePatterns scans all candles and returns detected patterns.
func DetectCandlePatterns(candles []domain.Candle) []Pattern {
	var patterns []Pattern
	n := len(candles)

	for i := 1; i < n; i++ {
		c := candles[i]
		r := totalRange(c)
		b := bodySize(c)
		if r == 0 {
			continue
		}
		ab := avgBody(candles, i, 5)

		// --- Doji ---
		if b < r*0.1 {
			patterns = append(patterns, Pattern{
				Name: "DOJI", Direction: 0, Strength: 0.5, Index: i,
			})
		}

		// --- Hammer (bullish, at bottom) ---
		if lowerWick(c) >= b*2 && upperWick(c) < b*0.5 && b > 0 {
			patterns = append(patterns, Pattern{
				Name: "HAMMER", Direction: 1, Strength: 0.7, Index: i,
			})
		}

		// --- Hanging Man (bearish, at top) ---
		if lowerWick(c) >= b*2 && upperWick(c) < b*0.5 && b > 0 && i >= 5 {
			// Check if preceded by uptrend (last 5 candles moving up).
			if candles[i-1].Close > candles[i-5].Close {
				patterns = append(patterns, Pattern{
					Name: "HANGING_MAN", Direction: -1, Strength: 0.6, Index: i,
				})
			}
		}

		// --- Shooting Star (bearish) ---
		if upperWick(c) >= b*2 && lowerWick(c) < b*0.5 && b > 0 {
			patterns = append(patterns, Pattern{
				Name: "SHOOTING_STAR", Direction: -1, Strength: 0.7, Index: i,
			})
		}

		// --- Bullish Engulfing ---
		if i >= 1 && isBullish(c) && !isBullish(candles[i-1]) {
			if c.Open < candles[i-1].Close && c.Close > candles[i-1].Open &&
				b > bodySize(candles[i-1]) {
				patterns = append(patterns, Pattern{
					Name: "ENGULFING_BULL", Direction: 1, Strength: 0.8, Index: i,
				})
			}
		}

		// --- Bearish Engulfing ---
		if i >= 1 && !isBullish(c) && isBullish(candles[i-1]) {
			if c.Open > candles[i-1].Close && c.Close < candles[i-1].Open &&
				b > bodySize(candles[i-1]) {
				patterns = append(patterns, Pattern{
					Name: "ENGULFING_BEAR", Direction: -1, Strength: 0.8, Index: i,
				})
			}
		}

		// --- Morning Star (3-candle bullish reversal) ---
		if i >= 2 {
			first := candles[i-2]
			second := candles[i-1]
			third := c
			if !isBullish(first) && bodySize(first) > ab*0.8 &&
				bodySize(second) < ab*0.3 &&
				isBullish(third) && bodySize(third) > ab*0.8 {
				patterns = append(patterns, Pattern{
					Name: "MORNING_STAR", Direction: 1, Strength: 0.85, Index: i,
				})
			}
		}

		// --- Evening Star (3-candle bearish reversal) ---
		if i >= 2 {
			first := candles[i-2]
			second := candles[i-1]
			third := c
			if isBullish(first) && bodySize(first) > ab*0.8 &&
				bodySize(second) < ab*0.3 &&
				!isBullish(third) && bodySize(third) > ab*0.8 {
				patterns = append(patterns, Pattern{
					Name: "EVENING_STAR", Direction: -1, Strength: 0.85, Index: i,
				})
			}
		}

		// --- Three White Soldiers ---
		if i >= 2 {
			a, b2, c2 := candles[i-2], candles[i-1], c
			if isBullish(a) && isBullish(b2) && isBullish(c2) &&
				b2.Close > a.Close && c2.Close > b2.Close &&
				bodySize(a) > ab*0.6 && bodySize(b2) > ab*0.6 && bodySize(c2) > ab*0.6 {
				patterns = append(patterns, Pattern{
					Name: "THREE_WHITE_SOLDIERS", Direction: 1, Strength: 0.9, Index: i,
				})
			}
		}

		// --- Three Black Crows ---
		if i >= 2 {
			a, b2, c2 := candles[i-2], candles[i-1], c
			if !isBullish(a) && !isBullish(b2) && !isBullish(c2) &&
				b2.Close < a.Close && c2.Close < b2.Close &&
				bodySize(a) > ab*0.6 && bodySize(b2) > ab*0.6 && bodySize(c2) > ab*0.6 {
				patterns = append(patterns, Pattern{
					Name: "THREE_BLACK_CROWS", Direction: -1, Strength: 0.9, Index: i,
				})
			}
		}
	}

	return patterns
}
