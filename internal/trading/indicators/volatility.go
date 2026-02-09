package indicators

import (
	"math"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// BollingerResult holds Bollinger Bands data.
type BollingerResult struct {
	Upper  []float64
	Middle []float64 // SMA
	Lower  []float64
	Width  []float64 // (Upper-Lower)/Middle — bandwidth
}

// Bollinger calculates Bollinger Bands.
// Default: period=20, stdDevMult=2.0.
func Bollinger(candles []domain.Candle, period int, stdDevMult float64) BollingerResult {
	n := len(candles)
	result := BollingerResult{
		Upper:  make([]float64, n),
		Middle: make([]float64, n),
		Lower:  make([]float64, n),
		Width:  make([]float64, n),
	}
	if n < period {
		return result
	}

	sma := SMA(candles, period)
	result.Middle = sma

	for i := period - 1; i < n; i++ {
		var sumSq float64
		for j := i - period + 1; j <= i; j++ {
			diff := candles[j].Close - sma[i]
			sumSq += diff * diff
		}
		stdDev := math.Sqrt(sumSq / float64(period))

		result.Upper[i] = sma[i] + stdDevMult*stdDev
		result.Lower[i] = sma[i] - stdDevMult*stdDev
		if sma[i] > 0 {
			result.Width[i] = (result.Upper[i] - result.Lower[i]) / sma[i]
		}
	}

	return result
}

// ATR calculates Average True Range (Wilder's smoothing).
func ATR(candles []domain.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n < period+1 || period <= 0 {
		return result
	}

	tr := TrueRange(candles)

	// Initial ATR = simple average of first 'period' true ranges.
	var sum float64
	for i := 1; i <= period; i++ {
		sum += tr[i]
	}
	result[period] = sum / float64(period)

	// Wilder's smoothing.
	for i := period + 1; i < n; i++ {
		result[i] = (result[i-1]*float64(period-1) + tr[i]) / float64(period)
	}

	return result
}

// TrueRange calculates True Range for each candle.
func TrueRange(candles []domain.Candle) []float64 {
	n := len(candles)
	result := make([]float64, n)

	for i := 1; i < n; i++ {
		highLow := candles[i].High - candles[i].Low
		highClose := math.Abs(candles[i].High - candles[i-1].Close)
		lowClose := math.Abs(candles[i].Low - candles[i-1].Close)
		result[i] = math.Max(highLow, math.Max(highClose, lowClose))
	}

	return result
}

// ADXResult holds ADX, +DI and -DI data.
type ADXResult struct {
	ADX      []float64
	PlusDI   []float64
	MinusDI  []float64
}

// ADX calculates Average Directional Index.
// Default period=14.
func ADX(candles []domain.Candle, period int) ADXResult {
	n := len(candles)
	result := ADXResult{
		ADX:     make([]float64, n),
		PlusDI:  make([]float64, n),
		MinusDI: make([]float64, n),
	}
	if n < period*2+1 {
		return result
	}

	tr := TrueRange(candles)

	plusDM := make([]float64, n)
	minusDM := make([]float64, n)

	for i := 1; i < n; i++ {
		upMove := candles[i].High - candles[i-1].High
		downMove := candles[i-1].Low - candles[i].Low

		if upMove > downMove && upMove > 0 {
			plusDM[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i] = downMove
		}
	}

	// Wilder's smoothing for TR, +DM, -DM.
	var smoothTR, smoothPlusDM, smoothMinusDM float64
	for i := 1; i <= period; i++ {
		smoothTR += tr[i]
		smoothPlusDM += plusDM[i]
		smoothMinusDM += minusDM[i]
	}

	p := float64(period)
	if smoothTR > 0 {
		result.PlusDI[period] = smoothPlusDM / smoothTR * 100
		result.MinusDI[period] = smoothMinusDM / smoothTR * 100
	}

	for i := period + 1; i < n; i++ {
		smoothTR = smoothTR - smoothTR/p + tr[i]
		smoothPlusDM = smoothPlusDM - smoothPlusDM/p + plusDM[i]
		smoothMinusDM = smoothMinusDM - smoothMinusDM/p + minusDM[i]

		if smoothTR > 0 {
			result.PlusDI[i] = smoothPlusDM / smoothTR * 100
			result.MinusDI[i] = smoothMinusDM / smoothTR * 100
		}
	}

	// DX and ADX.
	dx := make([]float64, n)
	for i := period; i < n; i++ {
		diSum := result.PlusDI[i] + result.MinusDI[i]
		if diSum > 0 {
			dx[i] = math.Abs(result.PlusDI[i]-result.MinusDI[i]) / diSum * 100
		}
	}

	// First ADX = average of first 'period' DX values.
	adxStart := period * 2
	if adxStart >= n {
		return result
	}
	var dxSum float64
	for i := period; i < adxStart; i++ {
		dxSum += dx[i]
	}
	result.ADX[adxStart-1] = dxSum / p

	for i := adxStart; i < n; i++ {
		result.ADX[i] = (result.ADX[i-1]*(p-1) + dx[i]) / p
	}

	return result
}
