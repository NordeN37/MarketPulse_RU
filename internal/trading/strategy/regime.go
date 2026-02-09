package strategy

import (
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/indicators"
)

// RegimeDetector determines the current market regime algorithmically.
type RegimeDetector struct {
	// ADX threshold for trending market.
	TrendADXThreshold float64
	// ATR expansion ratio for crisis detection.
	CrisisATRRatio float64
}

// NewRegimeDetector creates a detector with default parameters.
func NewRegimeDetector() *RegimeDetector {
	return &RegimeDetector{
		TrendADXThreshold: 25.0,
		CrisisATRRatio:    2.5,
	}
}

// Detect determines market regime from index candles (e.g., IMOEX).
func (d *RegimeDetector) Detect(candles []domain.Candle) domain.MarketRegime {
	if len(candles) < 60 {
		return domain.RegimeRange
	}

	last := len(candles) - 1

	// ADX for trend strength.
	adx := indicators.ADX(candles, 14)
	currentADX := adx.ADX[last]

	// ATR for volatility.
	atr := indicators.ATR(candles, 14)
	currentATR := atr[last]

	// Average ATR over last 50 candles for baseline.
	var atrSum float64
	var atrCount int
	for i := last - 50; i < last; i++ {
		if atr[i] > 0 {
			atrSum += atr[i]
			atrCount++
		}
	}
	avgATR := 0.0
	if atrCount > 0 {
		avgATR = atrSum / float64(atrCount)
	}

	// Crisis: ATR is way above average (extreme volatility).
	if avgATR > 0 && currentATR/avgATR >= d.CrisisATRRatio {
		return domain.RegimeCrisis
	}

	// Trend: ADX above threshold.
	if currentADX >= d.TrendADXThreshold {
		return domain.RegimeTrend
	}

	// Default: range-bound.
	return domain.RegimeRange
}
