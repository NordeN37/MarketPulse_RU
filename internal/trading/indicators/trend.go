package indicators

import (
	"math"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// ParabolicSARResult holds Parabolic SAR data.
type ParabolicSARResult struct {
	Values []float64
	// Trend: 1 = bullish, -1 = bearish
	Trend []int
}

// ParabolicSAR calculates Parabolic Stop and Reverse.
// Default: afStep=0.02, afMax=0.2.
func ParabolicSAR(candles []domain.Candle, afStep, afMax float64) ParabolicSARResult {
	n := len(candles)
	result := ParabolicSARResult{
		Values: make([]float64, n),
		Trend:  make([]int, n),
	}
	if n < 2 {
		return result
	}

	isLong := candles[1].Close > candles[0].Close
	af := afStep
	ep := candles[0].Low // extreme point
	sar := candles[0].High
	if isLong {
		ep = candles[0].High
		sar = candles[0].Low
	}

	for i := 1; i < n; i++ {
		prevSAR := sar
		sar = prevSAR + af*(ep-prevSAR)

		if isLong {
			// Clamp SAR to not be above prior two lows.
			if i >= 2 {
				sar = math.Min(sar, math.Min(candles[i-1].Low, candles[i-2].Low))
			} else {
				sar = math.Min(sar, candles[i-1].Low)
			}

			if candles[i].Low < sar {
				// Reversal to bearish.
				isLong = false
				sar = ep
				ep = candles[i].Low
				af = afStep
			} else {
				if candles[i].High > ep {
					ep = candles[i].High
					af = math.Min(af+afStep, afMax)
				}
			}
		} else {
			// Clamp SAR to not be below prior two highs.
			if i >= 2 {
				sar = math.Max(sar, math.Max(candles[i-1].High, candles[i-2].High))
			} else {
				sar = math.Max(sar, candles[i-1].High)
			}

			if candles[i].High > sar {
				// Reversal to bullish.
				isLong = true
				sar = ep
				ep = candles[i].High
				af = afStep
			} else {
				if candles[i].Low < ep {
					ep = candles[i].Low
					af = math.Min(af+afStep, afMax)
				}
			}
		}

		result.Values[i] = sar
		if isLong {
			result.Trend[i] = 1
		} else {
			result.Trend[i] = -1
		}
	}

	return result
}

// PivotPoints calculates classic pivot points from a daily candle.
// Returns: Pivot, S1, S2, S3, R1, R2, R3.
func PivotPoints(daily domain.Candle) (pivot, s1, s2, s3, r1, r2, r3 float64) {
	pivot = (daily.High + daily.Low + daily.Close) / 3.0
	r1 = 2*pivot - daily.Low
	s1 = 2*pivot - daily.High
	r2 = pivot + (daily.High - daily.Low)
	s2 = pivot - (daily.High - daily.Low)
	r3 = daily.High + 2*(pivot-daily.Low)
	s3 = daily.Low - 2*(daily.High-pivot)
	return
}

// FibonacciLevels calculates Fibonacci retracement levels between high and low.
// Returns levels at 0%, 23.6%, 38.2%, 50%, 61.8%, 78.6%, 100%.
func FibonacciLevels(high, low float64) [7]float64 {
	diff := high - low
	return [7]float64{
		low,                   // 0%
		low + 0.236*diff,      // 23.6%
		low + 0.382*diff,      // 38.2%
		low + 0.500*diff,      // 50%
		low + 0.618*diff,      // 61.8%
		low + 0.786*diff,      // 78.6%
		high,                  // 100%
	}
}

// SuperTrend calculates the SuperTrend indicator.
// Default: period=10, multiplier=3.0.
func SuperTrend(candles []domain.Candle, period int, multiplier float64) ([]float64, []int) {
	n := len(candles)
	values := make([]float64, n)
	trend := make([]int, n) // 1 = up, -1 = down

	atr := ATR(candles, period)
	if n < period+1 {
		return values, trend
	}

	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	for i := period; i < n; i++ {
		hl2 := (candles[i].High + candles[i].Low) / 2.0
		upperBand[i] = hl2 + multiplier*atr[i]
		lowerBand[i] = hl2 - multiplier*atr[i]

		if i == period {
			trend[i] = 1
			values[i] = lowerBand[i]
			continue
		}

		// Adjust bands.
		if lowerBand[i] < lowerBand[i-1] && candles[i-1].Close > lowerBand[i-1] {
			lowerBand[i] = lowerBand[i-1]
		}
		if upperBand[i] > upperBand[i-1] && candles[i-1].Close < upperBand[i-1] {
			upperBand[i] = upperBand[i-1]
		}

		// Determine trend.
		if trend[i-1] == 1 {
			if candles[i].Close < lowerBand[i] {
				trend[i] = -1
				values[i] = upperBand[i]
			} else {
				trend[i] = 1
				values[i] = lowerBand[i]
			}
		} else {
			if candles[i].Close > upperBand[i] {
				trend[i] = 1
				values[i] = lowerBand[i]
			} else {
				trend[i] = -1
				values[i] = upperBand[i]
			}
		}
	}

	return values, trend
}
