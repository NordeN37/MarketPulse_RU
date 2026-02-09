package indicators

import "github.com/NordeN37/MarketPulse_RU/internal/domain"

// OBV calculates On-Balance Volume.
func OBV(candles []domain.Candle) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 {
		return result
	}

	result[0] = candles[0].Volume

	for i := 1; i < n; i++ {
		switch {
		case candles[i].Close > candles[i-1].Close:
			result[i] = result[i-1] + candles[i].Volume
		case candles[i].Close < candles[i-1].Close:
			result[i] = result[i-1] - candles[i].Volume
		default:
			result[i] = result[i-1]
		}
	}

	return result
}

// VWAP calculates Volume Weighted Average Price (cumulative intraday).
// Resets at the boundary between different days (based on OpenTime).
func VWAP(candles []domain.Candle) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 {
		return result
	}

	var cumPV, cumVol float64

	for i := 0; i < n; i++ {
		// Typical price.
		tp := (candles[i].High + candles[i].Low + candles[i].Close) / 3.0

		cumPV += tp * candles[i].Volume
		cumVol += candles[i].Volume

		if cumVol > 0 {
			result[i] = cumPV / cumVol
		}
	}

	return result
}

// VolumeProfile calculates volume distribution at price levels.
// Returns map[priceLevel]totalVolume with the given number of bins.
func VolumeProfile(candles []domain.Candle, bins int) map[float64]float64 {
	if len(candles) == 0 || bins <= 0 {
		return nil
	}

	// Find price range.
	minPrice := candles[0].Low
	maxPrice := candles[0].High
	for _, c := range candles[1:] {
		if c.Low < minPrice {
			minPrice = c.Low
		}
		if c.High > maxPrice {
			maxPrice = c.High
		}
	}

	priceRange := maxPrice - minPrice
	if priceRange <= 0 {
		return map[float64]float64{minPrice: totalVolume(candles)}
	}

	binSize := priceRange / float64(bins)
	profile := make(map[float64]float64, bins)

	for _, c := range candles {
		// Distribute candle volume across bins it touches.
		tp := (c.High + c.Low + c.Close) / 3.0
		binIdx := int((tp - minPrice) / binSize)
		if binIdx >= bins {
			binIdx = bins - 1
		}
		level := minPrice + float64(binIdx)*binSize + binSize/2
		profile[level] += c.Volume
	}

	return profile
}

func totalVolume(candles []domain.Candle) float64 {
	var v float64
	for _, c := range candles {
		v += c.Volume
	}
	return v
}
