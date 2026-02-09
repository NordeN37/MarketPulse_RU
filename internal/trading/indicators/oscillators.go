package indicators

import (
	"math"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// RSIResult holds RSI data.
type RSIResult struct {
	Values []float64
}

// RSI calculates Relative Strength Index using Wilder's smoothing.
func RSI(candles []domain.Candle, period int) RSIResult {
	n := len(candles)
	result := RSIResult{Values: make([]float64, n)}
	if n < period+1 || period <= 0 {
		return result
	}

	// Calculate initial average gains/losses.
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss += math.Abs(change)
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	if avgLoss == 0 {
		result.Values[period] = 100
	} else {
		rs := avgGain / avgLoss
		result.Values[period] = 100 - 100/(1+rs)
	}

	// Wilder's smoothing for subsequent values.
	for i := period + 1; i < n; i++ {
		change := candles[i].Close - candles[i-1].Close
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = math.Abs(change)
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			result.Values[i] = 100
		} else {
			rs := avgGain / avgLoss
			result.Values[i] = 100 - 100/(1+rs)
		}
	}

	return result
}

// MACDResult holds MACD data.
type MACDResult struct {
	MACD      []float64 // MACD line (fast EMA - slow EMA)
	Signal    []float64 // Signal line (EMA of MACD)
	Histogram []float64 // MACD - Signal
}

// MACD calculates Moving Average Convergence Divergence.
// Default parameters: fast=12, slow=26, signal=9.
func MACD(candles []domain.Candle, fast, slow, signal int) MACDResult {
	n := len(candles)
	result := MACDResult{
		MACD:      make([]float64, n),
		Signal:    make([]float64, n),
		Histogram: make([]float64, n),
	}

	fastEMA := EMA(candles, fast)
	slowEMA := EMA(candles, slow)

	// MACD line = fast EMA - slow EMA.
	for i := slow - 1; i < n; i++ {
		result.MACD[i] = fastEMA[i] - slowEMA[i]
	}

	// Signal line = EMA(MACD, signal period).
	result.Signal = EMAFromValues(result.MACD, signal)

	// Histogram = MACD - Signal.
	for i := 0; i < n; i++ {
		if result.MACD[i] != 0 && result.Signal[i] != 0 {
			result.Histogram[i] = result.MACD[i] - result.Signal[i]
		}
	}

	return result
}

// StochasticResult holds Stochastic oscillator data.
type StochasticResult struct {
	K []float64 // %K (fast)
	D []float64 // %D (slow, SMA of %K)
}

// Stochastic calculates the Stochastic Oscillator.
// Default parameters: kPeriod=14, dPeriod=3.
func Stochastic(candles []domain.Candle, kPeriod, dPeriod int) StochasticResult {
	n := len(candles)
	result := StochasticResult{
		K: make([]float64, n),
		D: make([]float64, n),
	}
	if n < kPeriod || kPeriod <= 0 {
		return result
	}

	// Calculate %K.
	for i := kPeriod - 1; i < n; i++ {
		high := math.Inf(-1)
		low := math.Inf(1)
		for j := i - kPeriod + 1; j <= i; j++ {
			if candles[j].High > high {
				high = candles[j].High
			}
			if candles[j].Low < low {
				low = candles[j].Low
			}
		}
		if high-low > 0 {
			result.K[i] = (candles[i].Close - low) / (high - low) * 100
		} else {
			result.K[i] = 50
		}
	}

	// %D = SMA(%K, dPeriod).
	for i := kPeriod - 1 + dPeriod - 1; i < n; i++ {
		var sum float64
		for j := i - dPeriod + 1; j <= i; j++ {
			sum += result.K[j]
		}
		result.D[i] = sum / float64(dPeriod)
	}

	return result
}
