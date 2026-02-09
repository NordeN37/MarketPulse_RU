package patterns

import "github.com/NordeN37/MarketPulse_RU/internal/domain"

// Divergence represents a price-indicator divergence.
type Divergence struct {
	Type      string // "BULLISH_DIVERGENCE", "BEARISH_DIVERGENCE"
	Indicator string // "RSI", "MACD", "OBV"
	Strength  float64
	Index     int
}

// DetectRSIDivergence finds bullish/bearish divergences between price and RSI.
// lookback: how many candles back to search for swing points.
func DetectRSIDivergence(candles []domain.Candle, rsi []float64, lookback int) []Divergence {
	var divs []Divergence
	n := len(candles)
	if n < lookback*2 || len(rsi) != n {
		return divs
	}

	// Find local lows and highs in price and RSI.
	for i := lookback; i < n-1; i++ {
		// Check for swing low in price.
		isPriceLow := isSwingLow(candles, i, lookback)
		// Check for swing high in price.
		isPriceHigh := isSwingHigh(candles, i, lookback)

		if isPriceLow {
			// Look for prior swing low within lookback*3 range.
			for j := i - lookback; j >= lookback && j >= i-lookback*3; j-- {
				if isSwingLow(candles, j, lookback) {
					// Bullish divergence: price lower low, RSI higher low.
					if candles[i].Low < candles[j].Low && rsi[i] > rsi[j] && rsi[i] > 0 && rsi[j] > 0 {
						divs = append(divs, Divergence{
							Type:      "BULLISH_DIVERGENCE",
							Indicator: "RSI",
							Strength:  0.75,
							Index:     i,
						})
					}
					break
				}
			}
		}

		if isPriceHigh {
			for j := i - lookback; j >= lookback && j >= i-lookback*3; j-- {
				if isSwingHigh(candles, j, lookback) {
					// Bearish divergence: price higher high, RSI lower high.
					if candles[i].High > candles[j].High && rsi[i] < rsi[j] && rsi[i] > 0 && rsi[j] > 0 {
						divs = append(divs, Divergence{
							Type:      "BEARISH_DIVERGENCE",
							Indicator: "RSI",
							Strength:  0.75,
							Index:     i,
						})
					}
					break
				}
			}
		}
	}

	return divs
}

// isSwingLow checks if candle at index i is a local low within window.
func isSwingLow(candles []domain.Candle, i, window int) bool {
	if i < window || i+window >= len(candles) {
		return false
	}
	for j := i - window; j <= i+window; j++ {
		if j == i {
			continue
		}
		if candles[j].Low <= candles[i].Low {
			return false
		}
	}
	return true
}

// isSwingHigh checks if candle at index i is a local high within window.
func isSwingHigh(candles []domain.Candle, i, window int) bool {
	if i < window || i+window >= len(candles) {
		return false
	}
	for j := i - window; j <= i+window; j++ {
		if j == i {
			continue
		}
		if candles[j].High >= candles[i].High {
			return false
		}
	}
	return true
}
