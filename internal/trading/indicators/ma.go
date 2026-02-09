package indicators

import "github.com/NordeN37/MarketPulse_RU/internal/domain"

// SMA calculates Simple Moving Average for the given period.
// Returns a slice aligned to the input (first period-1 values are 0).
func SMA(candles []domain.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n < period || period <= 0 {
		return result
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += candles[i].Close
	}
	result[period-1] = sum / float64(period)

	for i := period; i < n; i++ {
		sum += candles[i].Close - candles[i-period].Close
		result[i] = sum / float64(period)
	}
	return result
}

// EMA calculates Exponential Moving Average for the given period.
func EMA(candles []domain.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n < period || period <= 0 {
		return result
	}

	// Seed with SMA.
	var sum float64
	for i := 0; i < period; i++ {
		sum += candles[i].Close
	}
	result[period-1] = sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < n; i++ {
		result[i] = (candles[i].Close-result[i-1])*multiplier + result[i-1]
	}
	return result
}

// EMAFromValues calculates EMA on raw float64 values (for chaining, e.g., MACD signal).
func EMAFromValues(values []float64, period int) []float64 {
	n := len(values)
	result := make([]float64, n)
	if n < period || period <= 0 {
		return result
	}

	var sum float64
	start := -1
	count := 0
	for i := 0; i < n; i++ {
		if values[i] == 0 && count == 0 {
			continue
		}
		if count == 0 {
			start = i
		}
		sum += values[i]
		count++
		if count == period {
			result[i] = sum / float64(period)
			break
		}
	}
	if count < period {
		return result
	}

	multiplier := 2.0 / float64(period+1)
	seedIdx := start + period - 1
	for i := seedIdx + 1; i < n; i++ {
		result[i] = (values[i]-result[i-1])*multiplier + result[i-1]
	}
	return result
}

// WMA calculates Weighted Moving Average for the given period.
func WMA(candles []domain.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n < period || period <= 0 {
		return result
	}

	denominator := float64(period * (period + 1) / 2)

	for i := period - 1; i < n; i++ {
		var sum float64
		for j := 0; j < period; j++ {
			weight := float64(j + 1)
			sum += candles[i-period+1+j].Close * weight
		}
		result[i] = sum / denominator
	}
	return result
}

// CrossOver returns true if fast crossed above slow at index i.
func CrossOver(fast, slow []float64, i int) bool {
	if i < 1 || i >= len(fast) || i >= len(slow) {
		return false
	}
	return fast[i-1] <= slow[i-1] && fast[i] > slow[i]
}

// CrossUnder returns true if fast crossed below slow at index i.
func CrossUnder(fast, slow []float64, i int) bool {
	if i < 1 || i >= len(fast) || i >= len(slow) {
		return false
	}
	return fast[i-1] >= slow[i-1] && fast[i] < slow[i]
}
