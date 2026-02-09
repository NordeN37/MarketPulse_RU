package signals

import (
	"fmt"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/indicators"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/patterns"
)

// TASignalGenerator creates trading signals from technical analysis.
type TASignalGenerator struct {
	// Weights for each indicator group.
	Weights IndicatorWeights
	// Minimum composite score to generate signal.
	MinScore  float64
	SignalTTL time.Duration
}

// IndicatorWeights controls how much each indicator contributes to the final score.
type IndicatorWeights struct {
	MA         float64 // Moving average crossovers
	RSI        float64 // RSI overbought/oversold
	MACD       float64 // MACD crossovers
	Bollinger  float64 // Bollinger band position
	ADX        float64 // Trend strength
	Stochastic float64 // Stochastic crossovers
	Patterns   float64 // Candlestick patterns
	Volume     float64 // Volume confirmation
}

// DefaultWeights returns balanced indicator weights.
func DefaultWeights() IndicatorWeights {
	return IndicatorWeights{
		MA:         0.15,
		RSI:        0.15,
		MACD:       0.15,
		Bollinger:  0.10,
		ADX:        0.10,
		Stochastic: 0.10,
		Patterns:   0.15,
		Volume:     0.10,
	}
}

// NewTASignalGenerator creates a generator with default parameters.
func NewTASignalGenerator() *TASignalGenerator {
	return &TASignalGenerator{
		Weights:   DefaultWeights(),
		MinScore:  0.3,
		SignalTTL: 1 * time.Hour,
	}
}

// TAAnalysis holds all computed indicator values for a ticker.
type TAAnalysis struct {
	Ticker    string
	Timeframe string
	Candles   []domain.Candle

	SMA20     []float64
	SMA50     []float64
	EMA12     []float64
	EMA26     []float64
	RSI       indicators.RSIResult
	MACD      indicators.MACDResult
	Bollinger indicators.BollingerResult
	ADX       indicators.ADXResult
	Stoch     indicators.StochasticResult
	OBV       []float64
	ATR       []float64

	CandlePatterns []patterns.Pattern
	Divergences    []patterns.Divergence
}

// Analyze computes all indicators for the given candles.
func (g *TASignalGenerator) Analyze(ticker, timeframe string, candles []domain.Candle) *TAAnalysis {
	if len(candles) < 50 {
		return nil
	}

	a := &TAAnalysis{
		Ticker:    ticker,
		Timeframe: timeframe,
		Candles:   candles,
	}

	a.SMA20 = indicators.SMA(candles, 20)
	a.SMA50 = indicators.SMA(candles, 50)
	a.EMA12 = indicators.EMA(candles, 12)
	a.EMA26 = indicators.EMA(candles, 26)
	a.RSI = indicators.RSI(candles, 14)
	a.MACD = indicators.MACD(candles, 12, 26, 9)
	a.Bollinger = indicators.Bollinger(candles, 20, 2.0)
	a.ADX = indicators.ADX(candles, 14)
	a.Stoch = indicators.Stochastic(candles, 14, 3)
	a.OBV = indicators.OBV(candles)
	a.ATR = indicators.ATR(candles, 14)

	a.CandlePatterns = patterns.DetectCandlePatterns(candles)
	a.Divergences = patterns.DetectRSIDivergence(candles, a.RSI.Values, 5)

	return a
}

// Generate creates a signal from the latest TA analysis.
func (g *TASignalGenerator) Generate(a *TAAnalysis) *domain.Signal {
	if a == nil || len(a.Candles) < 50 {
		return nil
	}

	last := len(a.Candles) - 1
	score := g.score(a, last)

	if score > g.MinScore || score < -g.MinScore {
		dir := domain.SignalBuy
		if score < 0 {
			dir = domain.SignalSell
		}

		strength := score
		if strength < 0 {
			strength = -strength
		}
		if strength > 1 {
			strength = 1
		}

		now := time.Now()

		indicatorMap := map[string]float64{
			"rsi":         a.RSI.Values[last],
			"macd":        a.MACD.MACD[last],
			"macd_signal": a.MACD.Signal[last],
			"adx":         a.ADX.ADX[last],
			"stoch_k":     a.Stoch.K[last],
			"stoch_d":     a.Stoch.D[last],
			"atr":         a.ATR[last],
			"sma20":       a.SMA20[last],
			"sma50":       a.SMA50[last],
		}

		var patternNames []string
		for _, p := range a.CandlePatterns {
			if p.Index >= last-2 {
				patternNames = append(patternNames, p.Name)
			}
		}

		return &domain.Signal{
			Ticker:    a.Ticker,
			Direction: dir,
			Source:    domain.SourceTA,
			Strength:  strength,
			Price:     a.Candles[last].Close,
			Reason:    fmt.Sprintf("TA [%s] score=%.3f rsi=%.1f macd=%.4f adx=%.1f", a.Timeframe, score, a.RSI.Values[last], a.MACD.MACD[last], a.ADX.ADX[last]),
			CreatedAt: now,
			ExpiresAt: now.Add(g.SignalTTL),
			TASignalMeta: &domain.TASignalMeta{
				Timeframe:  a.Timeframe,
				Indicators: indicatorMap,
				Patterns:   patternNames,
			},
		}
	}

	return nil
}

// score computes a weighted composite score from all indicators at index i.
// Positive = bullish, negative = bearish, range roughly [-1, 1].
func (g *TASignalGenerator) score(a *TAAnalysis, i int) float64 {
	var totalScore float64
	w := g.Weights

	// --- MA Score ---
	maScore := 0.0
	if a.SMA20[i] > 0 && a.SMA50[i] > 0 {
		if a.Candles[i].Close > a.SMA20[i] && a.SMA20[i] > a.SMA50[i] {
			maScore = 1.0
		} else if a.Candles[i].Close < a.SMA20[i] && a.SMA20[i] < a.SMA50[i] {
			maScore = -1.0
		} else if indicators.CrossOver(a.SMA20, a.SMA50, i) {
			maScore = 0.8
		} else if indicators.CrossUnder(a.SMA20, a.SMA50, i) {
			maScore = -0.8
		}
	}
	totalScore += w.MA * maScore

	// --- RSI Score ---
	rsiScore := 0.0
	rsi := a.RSI.Values[i]
	if rsi > 0 {
		if rsi < 30 {
			rsiScore = 1.0
		} else if rsi < 40 {
			rsiScore = 0.5
		} else if rsi > 70 {
			rsiScore = -1.0
		} else if rsi > 60 {
			rsiScore = -0.5
		}
	}
	totalScore += w.RSI * rsiScore

	// --- MACD Score ---
	macdScore := 0.0
	if a.MACD.MACD[i] != 0 {
		if indicators.CrossOver(a.MACD.MACD[:], a.MACD.Signal[:], i) {
			macdScore = 1.0
		} else if indicators.CrossUnder(a.MACD.MACD[:], a.MACD.Signal[:], i) {
			macdScore = -1.0
		} else if a.MACD.Histogram[i] > 0 {
			macdScore = 0.3
		} else if a.MACD.Histogram[i] < 0 {
			macdScore = -0.3
		}
	}
	totalScore += w.MACD * macdScore

	// --- Bollinger Score ---
	bbScore := 0.0
	if a.Bollinger.Upper[i] > 0 {
		price := a.Candles[i].Close
		if price <= a.Bollinger.Lower[i] {
			bbScore = 1.0
		} else if price >= a.Bollinger.Upper[i] {
			bbScore = -1.0
		}
	}
	totalScore += w.Bollinger * bbScore

	// --- ADX Score (trend strength modifier) ---
	adxMod := 1.0
	adx := a.ADX.ADX[i]
	if adx > 0 {
		if adx < 20 {
			adxMod = 0.5 // weak trend, reduce confidence
		} else if adx > 40 {
			adxMod = 1.3 // strong trend, boost confidence
		}
		// Direction from +DI/-DI.
		adxScore := 0.0
		if a.ADX.PlusDI[i] > a.ADX.MinusDI[i] {
			adxScore = 0.5
		} else if a.ADX.MinusDI[i] > a.ADX.PlusDI[i] {
			adxScore = -0.5
		}
		totalScore += w.ADX * adxScore
	}

	// --- Stochastic Score ---
	stochScore := 0.0
	if a.Stoch.K[i] > 0 {
		if a.Stoch.K[i] < 20 && indicators.CrossOver(a.Stoch.K[:], a.Stoch.D[:], i) {
			stochScore = 1.0
		} else if a.Stoch.K[i] > 80 && indicators.CrossUnder(a.Stoch.K[:], a.Stoch.D[:], i) {
			stochScore = -1.0
		} else if a.Stoch.K[i] < 20 {
			stochScore = 0.5
		} else if a.Stoch.K[i] > 80 {
			stochScore = -0.5
		}
	}
	totalScore += w.Stochastic * stochScore

	// --- Pattern Score ---
	patternScore := 0.0
	for _, p := range a.CandlePatterns {
		if p.Index >= i-2 && p.Index <= i {
			patternScore += float64(p.Direction) * p.Strength
		}
	}
	if patternScore > 1 {
		patternScore = 1
	} else if patternScore < -1 {
		patternScore = -1
	}
	totalScore += w.Patterns * patternScore

	// --- Volume Score (OBV trend confirmation) ---
	volScore := 0.0
	if i >= 5 && a.OBV[i] != 0 {
		if a.OBV[i] > a.OBV[i-5] && a.Candles[i].Close > a.Candles[i-5].Close {
			volScore = 0.5 // volume confirms up move
		} else if a.OBV[i] < a.OBV[i-5] && a.Candles[i].Close < a.Candles[i-5].Close {
			volScore = -0.5 // volume confirms down move
		}
	}
	totalScore += w.Volume * volScore

	// --- Divergence boost ---
	for _, d := range a.Divergences {
		if d.Index >= i-3 && d.Index <= i {
			if d.Type == "BULLISH_DIVERGENCE" {
				totalScore += 0.1
			} else if d.Type == "BEARISH_DIVERGENCE" {
				totalScore -= 0.1
			}
		}
	}

	// Apply ADX modifier.
	totalScore *= adxMod

	return totalScore
}
