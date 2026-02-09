package risk

import (
	"fmt"
	"math"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// Manager enforces risk management rules before placing trades.
type Manager struct {
	cfg domain.RiskConfig
}

// NewManager creates a new risk manager.
func NewManager(cfg domain.RiskConfig) *Manager {
	return &Manager{cfg: cfg}
}

// CheckResult holds the risk evaluation outcome.
type CheckResult struct {
	Allowed     bool
	Reason      string
	Quantity    int     // recommended position size
	StopLoss    float64 // recommended stop-loss price
	TakeProfit  float64 // recommended take-profit price
}

// Evaluate checks whether a signal can be traded under current risk constraints.
func (m *Manager) Evaluate(
	signal *domain.Signal,
	portfolio *domain.Portfolio,
	openPositions []domain.Position,
	atr float64,
) CheckResult {
	// 1. Circuit breaker: max drawdown.
	if portfolio.MaxDrawdown > 0 && portfolio.MaxDrawdown >= m.cfg.MaxDrawdown {
		return CheckResult{Allowed: false, Reason: fmt.Sprintf("max drawdown reached: %.2f%%", portfolio.MaxDrawdown*100)}
	}

	// 2. Daily loss limit.
	if portfolio.TotalValue > 0 {
		dailyLossFrac := -portfolio.DailyPnL / portfolio.TotalValue
		if dailyLossFrac >= m.cfg.DailyLossLimit {
			return CheckResult{Allowed: false, Reason: fmt.Sprintf("daily loss limit reached: %.2f%%", dailyLossFrac*100)}
		}
	}

	// 3. Max open positions.
	if len(openPositions) >= m.cfg.MaxOpenPositions {
		return CheckResult{Allowed: false, Reason: fmt.Sprintf("max open positions reached: %d", m.cfg.MaxOpenPositions)}
	}

	// 4. No duplicate ticker.
	for _, pos := range openPositions {
		if pos.Ticker == signal.Ticker && pos.Status == domain.PositionOpen {
			return CheckResult{Allowed: false, Reason: fmt.Sprintf("already have open position for %s", signal.Ticker)}
		}
	}

	// 5. Total exposure check.
	totalExposure := m.calculateExposure(openPositions, portfolio.TotalValue)
	if totalExposure >= m.cfg.MaxExposure {
		return CheckResult{Allowed: false, Reason: fmt.Sprintf("max exposure reached: %.1f%%", totalExposure*100)}
	}

	// 6. Calculate position size.
	quantity := m.calculatePositionSize(signal.Price, atr, portfolio)
	if quantity <= 0 {
		return CheckResult{Allowed: false, Reason: "calculated position size is 0"}
	}

	// 7. Calculate stop-loss and take-profit.
	stopLoss, takeProfit := m.calculateLevels(signal, atr)

	return CheckResult{
		Allowed:    true,
		Quantity:   quantity,
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
	}
}

// calculatePositionSize uses ATR-based sizing (risk per trade).
func (m *Manager) calculatePositionSize(price, atr float64, portfolio *domain.Portfolio) int {
	if price <= 0 || portfolio.TotalValue <= 0 {
		return 0
	}

	// Max capital for this position.
	maxCapital := portfolio.TotalValue * m.cfg.MaxPositionSize

	// ATR-based sizing: risk per share = 2 * ATR (stop distance).
	riskPerShare := 2 * atr
	if riskPerShare <= 0 {
		riskPerShare = price * m.cfg.DefaultStopLoss
	}
	if riskPerShare <= 0 {
		return 0
	}

	// Risk budget = max position size fraction of portfolio.
	riskBudget := portfolio.TotalValue * m.cfg.MaxPositionSize * m.cfg.DefaultStopLoss
	if riskBudget <= 0 {
		riskBudget = maxCapital * 0.02 // 2% default risk
	}

	// Number of shares based on risk.
	qtyByRisk := int(riskBudget / riskPerShare)

	// Number of shares based on max capital.
	qtyByCapital := int(maxCapital / price)

	// Take the smaller.
	qty := qtyByRisk
	if qtyByCapital < qty {
		qty = qtyByCapital
	}

	if qty <= 0 {
		qty = 1 // minimum 1 share
	}

	return qty
}

// calculateLevels sets stop-loss and take-profit based on ATR.
func (m *Manager) calculateLevels(signal *domain.Signal, atr float64) (stopLoss, takeProfit float64) {
	slPct := m.cfg.DefaultStopLoss
	tpPct := m.cfg.DefaultTakeProfit

	if atr > 0 && signal.Price > 0 {
		// Dynamic SL: 2x ATR, but at least defaultStopLoss%.
		atrSL := 2 * atr / signal.Price
		if atrSL > slPct {
			slPct = atrSL
		}
		// TP = 3x ATR for 1.5:1 risk/reward.
		atrTP := 3 * atr / signal.Price
		if atrTP > tpPct {
			tpPct = atrTP
		}
	}

	if signal.Direction == domain.SignalBuy {
		stopLoss = signal.Price * (1 - slPct)
		takeProfit = signal.Price * (1 + tpPct)
	} else {
		stopLoss = signal.Price * (1 + slPct)
		takeProfit = signal.Price * (1 - tpPct)
	}

	stopLoss = math.Round(stopLoss*100) / 100
	takeProfit = math.Round(takeProfit*100) / 100
	return
}

// calculateExposure returns total position value as fraction of portfolio.
func (m *Manager) calculateExposure(positions []domain.Position, portfolioValue float64) float64 {
	if portfolioValue <= 0 {
		return 0
	}
	var total float64
	for _, p := range positions {
		if p.Status == domain.PositionOpen {
			total += p.EntryPrice * float64(p.Quantity)
		}
	}
	return total / portfolioValue
}

// SectorExposure calculates exposure per sector.
func (m *Manager) SectorExposure(positions []domain.Position, tickerSectors map[string]string, portfolioValue float64) map[string]float64 {
	sectors := make(map[string]float64)
	if portfolioValue <= 0 {
		return sectors
	}
	for _, p := range positions {
		if p.Status != domain.PositionOpen {
			continue
		}
		sector := tickerSectors[p.Ticker]
		if sector == "" {
			sector = "unknown"
		}
		sectors[sector] += p.EntryPrice * float64(p.Quantity) / portfolioValue
	}
	return sectors
}
