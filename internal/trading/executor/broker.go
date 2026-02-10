package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/tinvest"
	redisclient "github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// BrokerExecutor executes trades via T-Invest API.
// It resolves tickers to instrument UIDs and routes orders to the correct accounts
// based on strategy → account mappings stored in Redis.
type BrokerExecutor struct {
	manager *tinvest.Manager
	cache   *redisclient.Client
	log     *slog.Logger

	// Instrument UID cache: ticker → UID
	uidCache map[string]string
}

// NewBrokerExecutor creates a new broker executor.
func NewBrokerExecutor(mgr *tinvest.Manager, cache *redisclient.Client, log *slog.Logger) *BrokerExecutor {
	return &BrokerExecutor{
		manager:  mgr,
		cache:    cache,
		log:      log,
		uidCache: make(map[string]string),
	}
}

// Connected reports whether the broker is connected.
func (be *BrokerExecutor) Connected() bool {
	return be.manager.Connected()
}

// resolveUID maps a ticker to its T-Invest instrument UID.
func (be *BrokerExecutor) resolveUID(ticker string) (string, error) {
	if uid, ok := be.uidCache[ticker]; ok {
		return uid, nil
	}
	inst, err := be.manager.FindInstrumentByTicker(ticker, "TQBR")
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ticker, err)
	}
	be.uidCache[ticker] = inst.UID
	return inst.UID, nil
}

// GetAccountsForStrategy returns account IDs that have the given strategy enabled.
func (be *BrokerExecutor) GetAccountsForStrategy(ctx context.Context, strategy string) ([]string, error) {
	strategies, err := be.cache.ReadTInvestStrategies(ctx)
	if err != nil {
		return nil, err
	}
	var accounts []string
	for _, s := range strategies {
		if s.Enabled && s.Strategy == strategy {
			accounts = append(accounts, s.AccountID)
		}
	}
	return accounts, nil
}

// ExecuteSignal places orders on all accounts assigned to the signal's strategy.
// Returns order results per account.
func (be *BrokerExecutor) ExecuteSignal(ctx context.Context, sig *domain.Signal, quantity int) ([]BrokerOrderResult, error) {
	if !be.Connected() {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	// Map signal source to strategy name
	strategy := sourceToStrategy(sig.Source)
	if strategy == "" {
		return nil, fmt.Errorf("unknown signal source: %s", sig.Source)
	}

	// Get accounts for this strategy
	accounts, err := be.GetAccountsForStrategy(ctx, strategy)
	if err != nil {
		return nil, fmt.Errorf("getting accounts for strategy %s: %w", strategy, err)
	}
	if len(accounts) == 0 {
		be.log.Info("no accounts assigned for strategy", "strategy", strategy, "signal", sig.Ticker)
		return nil, nil
	}

	// Resolve ticker to instrument UID
	uid, err := be.resolveUID(sig.Ticker)
	if err != nil {
		return nil, err
	}

	direction := "BUY"
	if sig.Direction == domain.SignalSell {
		direction = "SELL"
	}

	var results []BrokerOrderResult
	for _, accountID := range accounts {
		// Check margin before placing order
		marginData, err := be.manager.GetMarginAttributes(accountID)
		if err != nil {
			be.log.Warn("failed to check margin", "account", accountID, "error", err)
			// Continue anyway — the API will reject if insufficient
		} else if marginData.FundsAvailable < sig.Price*float64(quantity)*0.3 {
			// Skip if available margin is less than 30% of order value (rough check)
			be.log.Warn("insufficient margin, skipping",
				"account", accountID,
				"available", marginData.FundsAvailable,
				"needed", sig.Price*float64(quantity),
			)
			results = append(results, BrokerOrderResult{
				AccountID: accountID,
				Success:   false,
				Error:     "insufficient margin",
			})
			continue
		}

		req := tinvest.OrderRequest{
			AccountID:    accountID,
			InstrumentID: uid,
			Quantity:     int64(quantity),
			Direction:    direction,
			OrderType:    "MARKET",
		}

		result, err := be.manager.PostOrder(req)
		if err != nil {
			be.log.Error("broker order failed",
				"account", accountID,
				"ticker", sig.Ticker,
				"direction", direction,
				"error", err,
			)
			results = append(results, BrokerOrderResult{
				AccountID: accountID,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}

		br := BrokerOrderResult{
			AccountID:    accountID,
			Success:      true,
			OrderID:      result.OrderID,
			Status:       result.ExecutionStatus,
			LotsExecuted: result.LotsExecuted,
			Price:        result.ExecutedPrice,
			Commission:   result.TotalCommission,
			Direction:    direction,
		}
		results = append(results, br)

		be.log.Info("broker order placed",
			"account", accountID,
			"ticker", sig.Ticker,
			"direction", direction,
			"order_id", result.OrderID,
			"status", result.ExecutionStatus,
			"lots_exec", result.LotsExecuted,
			"price", result.ExecutedPrice,
		)
	}

	return results, nil
}

// ClosePosition closes a position by placing an opposite market order.
func (be *BrokerExecutor) ClosePosition(ctx context.Context, accountID, ticker string, quantity int, side domain.PositionSide) (*tinvest.OrderResult, error) {
	uid, err := be.resolveUID(ticker)
	if err != nil {
		return nil, err
	}

	// Opposite direction
	direction := "SELL"
	if side == domain.SideShort {
		direction = "BUY"
	}

	req := tinvest.OrderRequest{
		AccountID:    accountID,
		InstrumentID: uid,
		Quantity:     int64(quantity),
		Direction:    direction,
		OrderType:    "MARKET",
	}

	return be.manager.PostOrder(req)
}

// GetPortfolioForAccount returns the portfolio snapshot from the broker.
func (be *BrokerExecutor) GetPortfolioForAccount(accountID string) (*tinvest.PortfolioData, error) {
	return be.manager.GetPortfolio(accountID)
}

// BrokerOrderResult represents the outcome of a broker order placement.
type BrokerOrderResult struct {
	AccountID    string  `json:"account_id"`
	Success      bool    `json:"success"`
	OrderID      string  `json:"order_id,omitempty"`
	Status       string  `json:"status,omitempty"`
	LotsExecuted int64   `json:"lots_executed,omitempty"`
	Price        float64 `json:"price,omitempty"`
	Commission   float64 `json:"commission,omitempty"`
	Direction    string  `json:"direction,omitempty"`
	Error        string  `json:"error,omitempty"`
}

func sourceToStrategy(src domain.SignalSource) string {
	switch src {
	case domain.SourceNews:
		return "news"
	case domain.SourceTA:
		return "ta"
	case domain.SourceCombined:
		return "combined"
	default:
		return ""
	}
}

// MarkSignalExecuted is a helper to update the signal as executed in the DB.
// (Called by the engine after successful order placement.)
func MarkSignalExecuted(ctx context.Context, signalID int64, execFunc func(ctx context.Context, id int64) error) {
	if err := execFunc(ctx, signalID); err != nil {
		// Log but don't fail — the order is already placed
		_ = err
	}
}

// CheckStopLossViaAPI checks stop loss/take profit using live T-Invest prices.
func (be *BrokerExecutor) CheckStopLossViaAPI(ctx context.Context, positions []domain.Position) []domain.Position {
	if !be.Connected() {
		return nil
	}

	// Collect instrument UIDs
	uidMap := make(map[string]string) // ticker → uid
	var uids []string
	for _, pos := range positions {
		if pos.Status != domain.PositionOpen {
			continue
		}
		uid, err := be.resolveUID(pos.Ticker)
		if err != nil {
			continue
		}
		uidMap[pos.Ticker] = uid
		uids = append(uids, uid)
	}

	if len(uids) == 0 {
		return nil
	}

	prices, err := be.manager.GetLastPrices(uids)
	if err != nil {
		be.log.Warn("failed to get prices for SL/TP check", "error", err)
		return nil
	}

	var toClose []domain.Position
	_ = time.Now()

	for _, pos := range positions {
		if pos.Status != domain.PositionOpen {
			continue
		}
		uid, ok := uidMap[pos.Ticker]
		if !ok {
			continue
		}
		price, ok := prices[uid]
		if !ok || price == 0 {
			continue
		}

		shouldClose := false

		if pos.Side == domain.SideLong {
			if pos.StopLoss != nil && price <= *pos.StopLoss {
				shouldClose = true
			}
			if pos.TakeProfit != nil && price >= *pos.TakeProfit {
				shouldClose = true
			}
		} else {
			if pos.StopLoss != nil && price >= *pos.StopLoss {
				shouldClose = true
			}
			if pos.TakeProfit != nil && price <= *pos.TakeProfit {
				shouldClose = true
			}
		}

		if shouldClose {
			be.log.Info("SL/TP triggered (live)",
				"ticker", pos.Ticker,
				"price", price,
				"side", pos.Side,
			)
			toClose = append(toClose, pos)
		}
	}

	return toClose
}
