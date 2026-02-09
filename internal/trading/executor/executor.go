package executor

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
)

// Executor handles order placement and position management.
type Executor struct {
	moex *moex.Client
	log  *slog.Logger
	// dryRun: if true, simulate orders without real execution.
	dryRun bool
}

// NewExecutor creates a new order executor.
func NewExecutor(moexClient *moex.Client, log *slog.Logger, dryRun bool) *Executor {
	return &Executor{
		moex:   moexClient,
		log:    log,
		dryRun: dryRun,
	}
}

// PlaceOrder creates and "executes" an order.
// In dryRun mode, it simulates a fill at the current market price.
func (e *Executor) PlaceOrder(ctx context.Context, signal *domain.Signal, quantity int, stopLoss, takeProfit float64) (*domain.Order, error) {
	side := domain.OrderBuy
	if signal.Direction == domain.SignalSell {
		side = domain.OrderSell
	}

	order := &domain.Order{
		Ticker:    signal.Ticker,
		Side:      side,
		Type:      domain.OrderMarket,
		Status:    domain.OrderPending,
		Quantity:  quantity,
		SignalID:  &signal.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if e.dryRun {
		// Simulate: get current price from MOEX.
		quote, err := e.moex.GetQuote(ctx, signal.Ticker)
		if err != nil {
			return nil, fmt.Errorf("getting quote for %s: %w", signal.Ticker, err)
		}

		fillPrice := quote.Last
		if fillPrice == 0 {
			return nil, fmt.Errorf("zero price for %s", signal.Ticker)
		}

		order.Status = domain.OrderFilled
		order.FilledQty = quantity
		order.FilledPrice = &fillPrice
		now := time.Now()
		order.FilledAt = &now
		order.ExternalID = fmt.Sprintf("DRY-%d", now.UnixNano())

		// Simulate commission (0.05% MOEX typical).
		order.Commission = math.Round(fillPrice*float64(quantity)*0.0005*100) / 100

		e.log.Info("DRY RUN: order filled",
			"ticker", signal.Ticker,
			"side", side,
			"qty", quantity,
			"price", fillPrice,
			"commission", order.Commission,
			"stop_loss", stopLoss,
			"take_profit", takeProfit,
		)
	} else {
		// TODO: Real broker API integration.
		// For now, treat as dry run with warning.
		e.log.Warn("LIVE TRADING NOT IMPLEMENTED — falling back to dry run",
			"ticker", signal.Ticker,
		)
		return e.PlaceOrder(ctx, signal, quantity, stopLoss, takeProfit)
	}

	return order, nil
}

// CheckStopLoss evaluates if any open positions need to be closed.
func (e *Executor) CheckStopLoss(ctx context.Context, positions []domain.Position) []domain.Position {
	var toClose []domain.Position

	for _, pos := range positions {
		if pos.Status != domain.PositionOpen {
			continue
		}
		if pos.StopLoss == nil && pos.TakeProfit == nil {
			continue
		}

		quote, err := e.moex.GetQuote(ctx, pos.Ticker)
		if err != nil {
			e.log.Warn("failed to get quote for SL/TP check",
				"ticker", pos.Ticker,
				"error", err,
			)
			continue
		}

		price := quote.Last
		if price == 0 {
			continue
		}

		shouldClose := false
		reason := ""

		if pos.Side == domain.SideLong {
			if pos.StopLoss != nil && price <= *pos.StopLoss {
				shouldClose = true
				reason = fmt.Sprintf("stop-loss hit: %.2f <= %.2f", price, *pos.StopLoss)
			}
			if pos.TakeProfit != nil && price >= *pos.TakeProfit {
				shouldClose = true
				reason = fmt.Sprintf("take-profit hit: %.2f >= %.2f", price, *pos.TakeProfit)
			}
		} else {
			if pos.StopLoss != nil && price >= *pos.StopLoss {
				shouldClose = true
				reason = fmt.Sprintf("stop-loss hit: %.2f >= %.2f", price, *pos.StopLoss)
			}
			if pos.TakeProfit != nil && price <= *pos.TakeProfit {
				shouldClose = true
				reason = fmt.Sprintf("take-profit hit: %.2f <= %.2f", price, *pos.TakeProfit)
			}
		}

		if shouldClose {
			e.log.Info("position SL/TP triggered",
				"ticker", pos.Ticker,
				"reason", reason,
				"position_id", pos.ID,
			)
			toClose = append(toClose, pos)
		}
	}

	return toClose
}
