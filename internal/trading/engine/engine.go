package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/executor"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/risk"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/signals"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/strategy"
)

// SignalReader reads pending signals from the database.
type SignalReader interface {
	GetPendingBySource(ctx context.Context, source string, limit int) ([]domain.Signal, error)
	MarkExecuted(ctx context.Context, id int64) error
}

// PositionWriter persists positions to the database.
type PositionWriter interface {
	Insert(ctx context.Context, p *domain.Position) (int64, error)
	Close(ctx context.Context, p *domain.Position) error
	GetOpen(ctx context.Context) ([]domain.Position, error)
}

// Engine is the main trading engine orchestrating signals, risk, and execution.
type Engine struct {
	mode      domain.TradingMode
	moex      *moex.Client
	taGen     *signals.TASignalGenerator
	newsGen   *signals.NewsSignalGenerator
	combGen   *signals.CombinedSignalGenerator
	riskMgr   *risk.Manager
	executor  *executor.Executor
	regime    *strategy.RegimeDetector
	log       *slog.Logger

	// Persistence.
	signalReader   SignalReader
	positionWriter PositionWriter

	// State.
	portfolio     *domain.Portfolio
	openPositions []domain.Position
	tickers       []string

	// Config.
	checkInterval time.Duration
}

// Config holds trading engine configuration.
type Config struct {
	Mode           domain.TradingMode
	Tickers        []string
	RiskConfig     domain.RiskConfig
	StrategyConfig domain.StrategyConfig
	CheckInterval  time.Duration
	DryRun         bool
	InitialCash    float64
}

// NewEngine creates a new trading engine.
func NewEngine(cfg Config, moexClient *moex.Client, log *slog.Logger) *Engine {
	e := &Engine{
		mode:    cfg.Mode,
		moex:    moexClient,
		riskMgr: risk.NewManager(cfg.RiskConfig),
		executor: executor.NewExecutor(moexClient, log, cfg.DryRun),
		regime:  strategy.NewRegimeDetector(),
		log:     log,
		tickers: cfg.Tickers,
		checkInterval: cfg.CheckInterval,
		portfolio: &domain.Portfolio{
			Cash:       cfg.InitialCash,
			TotalValue: cfg.InitialCash,
			SnapshotAt: time.Now(),
		},
	}

	e.newsGen = signals.NewNewsSignalGenerator()
	e.taGen = signals.NewTASignalGenerator()

	if cfg.StrategyConfig.IndicatorWeights != nil {
		e.taGen.Weights = signals.IndicatorWeights{
			MA:         cfg.StrategyConfig.IndicatorWeights["ma"],
			RSI:        cfg.StrategyConfig.IndicatorWeights["rsi"],
			MACD:       cfg.StrategyConfig.IndicatorWeights["macd"],
			Bollinger:  cfg.StrategyConfig.IndicatorWeights["bollinger"],
			ADX:        cfg.StrategyConfig.IndicatorWeights["adx"],
			Stochastic: cfg.StrategyConfig.IndicatorWeights["stochastic"],
			Patterns:   cfg.StrategyConfig.IndicatorWeights["patterns"],
			Volume:     cfg.StrategyConfig.IndicatorWeights["volume"],
		}
	}

	e.combGen = signals.NewCombinedSignalGenerator()
	e.combGen.NewsWeight = cfg.StrategyConfig.NewsWeight
	e.combGen.TAWeight = cfg.StrategyConfig.TAWeight
	e.combGen.RequireConfluence = cfg.StrategyConfig.RequireConfluence

	return e
}

// SetSignalReader sets the signal reader for consuming DB signals.
func (e *Engine) SetSignalReader(sr SignalReader) {
	e.signalReader = sr
}

// SetPositionWriter sets the position writer for persistence.
func (e *Engine) SetPositionWriter(pw PositionWriter) {
	e.positionWriter = pw
}

// RestorePositions loads open positions from DB on startup.
func (e *Engine) RestorePositions(ctx context.Context) {
	if e.positionWriter == nil {
		return
	}
	positions, err := e.positionWriter.GetOpen(ctx)
	if err != nil {
		e.log.Error("failed to restore positions", "error", err)
		return
	}
	e.openPositions = positions
	e.portfolio.OpenPositions = len(positions)
	e.log.Info("positions restored from DB", "count", len(positions))
}

// Run starts the main trading loop.
func (e *Engine) Run(ctx context.Context) error {
	e.log.Info("trading engine started",
		"mode", e.mode,
		"tickers", e.tickers,
		"interval", e.checkInterval,
	)

	ticker := time.NewTicker(e.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.log.Info("trading engine stopped")
			return ctx.Err()
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

// tick performs one iteration of the trading loop.
func (e *Engine) tick(ctx context.Context) {
	// 1. Check SL/TP on open positions.
	toClose := e.executor.CheckStopLoss(ctx, e.openPositions)
	for _, pos := range toClose {
		e.closePosition(ctx, &pos)
	}

	// 2. Consume pending news signals from DB (written by analyzer).
	e.consumeNewsSignals(ctx)

	// 3. Generate TA/combined signals based on mode.
	if e.mode != domain.ModeNews {
		for _, tkr := range e.tickers {
			var sig *domain.Signal
			switch e.mode {
			case domain.ModeTA:
				sig = e.generateTASignal(ctx, tkr)
			case domain.ModeCombined:
				sig = e.generateCombinedSignal(ctx, tkr)
			}
			if sig == nil {
				continue
			}
			e.processSignal(ctx, sig, tkr)
		}
	}
}

// consumeNewsSignals reads pending news signals from DB and processes them.
func (e *Engine) consumeNewsSignals(ctx context.Context) {
	if e.signalReader == nil {
		return
	}

	pending, err := e.signalReader.GetPendingBySource(ctx, string(domain.SourceNews), 50)
	if err != nil {
		e.log.Warn("failed to read pending news signals", "error", err)
		return
	}

	for i := range pending {
		sig := &pending[i]
		e.log.Info("consuming news signal from DB",
			"id", sig.ID,
			"ticker", sig.Ticker,
			"direction", sig.Direction,
			"strength", sig.Strength,
		)

		// Mark as executed immediately (regardless of whether risk allows it).
		if markErr := e.signalReader.MarkExecuted(ctx, sig.ID); markErr != nil {
			e.log.Warn("failed to mark signal executed", "id", sig.ID, "error", markErr)
		}

		// Route through the appropriate processing path.
		e.ProcessNewsSignal(ctx, sig)
	}
}

// generateTASignal computes TA for a ticker.
func (e *Engine) generateTASignal(ctx context.Context, ticker string) *domain.Signal {
	now := time.Now()
	candles, err := e.moex.GetCandlesAll(ctx, ticker, moex.Interval1Hour, now.AddDate(0, -1, 0), now)
	if err != nil {
		e.log.Warn("failed to get candles", "ticker", ticker, "error", err)
		return nil
	}

	analysis := e.taGen.Analyze(ticker, "1h", candles)
	return e.taGen.Generate(analysis)
}

// generateCombinedSignal merges TA signal with latest news state.
func (e *Engine) generateCombinedSignal(ctx context.Context, ticker string) *domain.Signal {
	taSig := e.generateTASignal(ctx, ticker)

	// Detect current market regime from index.
	now := time.Now()
	indexCandles, err := e.moex.GetCandlesAll(ctx, "IMOEX", moex.Interval1Day, now.AddDate(0, -3, 0), now)
	regime := domain.RegimeRange
	if err == nil && len(indexCandles) > 60 {
		regime = e.regime.Detect(indexCandles)
	}

	// In combined mode without a fresh news signal, use TA only.
	// News signals are injected via ProcessNewsSignal.
	if taSig != nil {
		return e.combGen.Combine(nil, taSig, regime)
	}
	return nil
}

// ProcessNewsSignal is called externally when a news signal arrives.
// It tries to combine with existing TA analysis.
func (e *Engine) ProcessNewsSignal(ctx context.Context, newsSig *domain.Signal) {
	if newsSig == nil {
		return
	}

	e.log.Info("processing news signal",
		"ticker", newsSig.Ticker,
		"direction", newsSig.Direction,
		"strength", newsSig.Strength,
	)

	// In news-only mode, trade directly.
	if e.mode == domain.ModeNews {
		e.processSignal(ctx, newsSig, newsSig.Ticker)
		return
	}

	// In combined mode, merge with TA.
	if e.mode == domain.ModeCombined {
		taSig := e.generateTASignal(ctx, newsSig.Ticker)

		now := time.Now()
		indexCandles, err := e.moex.GetCandlesAll(ctx, "IMOEX", moex.Interval1Day, now.AddDate(0, -3, 0), now)
		regime := domain.RegimeRange
		if err == nil && len(indexCandles) > 60 {
			regime = e.regime.Detect(indexCandles)
		}

		combined := e.combGen.Combine(newsSig, taSig, regime)
		if combined != nil {
			e.processSignal(ctx, combined, newsSig.Ticker)
		}
	}
}

// processSignal evaluates risk and executes a trade if allowed.
func (e *Engine) processSignal(ctx context.Context, sig *domain.Signal, ticker string) {
	// Get ATR for risk calculation.
	now := time.Now()
	candles, err := e.moex.GetCandlesAll(ctx, ticker, moex.Interval1Day, now.AddDate(0, 0, -30), now)
	atr := 0.0
	if err == nil && len(candles) > 15 {
		atrValues := make([]float64, len(candles))
		for i := 1; i < len(candles); i++ {
			tr := candles[i].High - candles[i].Low
			hc := candles[i].High - candles[i-1].Close
			if hc < 0 {
				hc = -hc
			}
			lc := candles[i].Low - candles[i-1].Close
			if lc < 0 {
				lc = -lc
			}
			if hc > tr {
				tr = hc
			}
			if lc > tr {
				tr = lc
			}
			atrValues[i] = tr
		}
		// Simple average of last 14.
		var sum float64
		count := 0
		for i := len(atrValues) - 14; i < len(atrValues); i++ {
			if i >= 0 && atrValues[i] > 0 {
				sum += atrValues[i]
				count++
			}
		}
		if count > 0 {
			atr = sum / float64(count)
		}
	}

	result := e.riskMgr.Evaluate(sig, e.portfolio, e.openPositions, atr)
	if !result.Allowed {
		e.log.Info("signal blocked by risk manager",
			"ticker", ticker,
			"direction", sig.Direction,
			"reason", result.Reason,
		)
		return
	}

	order, err := e.executor.PlaceOrder(ctx, sig, result.Quantity, result.StopLoss, result.TakeProfit)
	if err != nil {
		e.log.Error("failed to place order", "ticker", ticker, "error", err)
		return
	}

	if order.Status == domain.OrderFilled && order.FilledPrice != nil {
		sl := result.StopLoss
		tp := result.TakeProfit
		pos := domain.Position{
			Ticker:     ticker,
			Side:       domain.SideLong,
			Status:     domain.PositionOpen,
			Quantity:   order.FilledQty,
			EntryPrice: *order.FilledPrice,
			EntryTime:  time.Now(),
			EntryOrder: order.ID,
			StopLoss:   &sl,
			TakeProfit: &tp,
			SignalID:   sig.ID,
			Strategy:   string(e.mode),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if sig.Direction == domain.SignalSell {
			pos.Side = domain.SideShort
		}

		// Persist to DB.
		if e.positionWriter != nil {
			if _, dbErr := e.positionWriter.Insert(ctx, &pos); dbErr != nil {
				e.log.Error("failed to persist position to DB", "ticker", ticker, "error", dbErr)
			}
		}

		e.openPositions = append(e.openPositions, pos)
		e.portfolio.OpenPositions = len(e.openPositions)
		e.portfolio.Cash -= *order.FilledPrice * float64(order.FilledQty)

		e.log.Info("position opened",
			"ticker", ticker,
			"side", pos.Side,
			"qty", pos.Quantity,
			"price", pos.EntryPrice,
			"stop_loss", sl,
			"take_profit", tp,
		)
	}
}

// closePosition closes a position.
func (e *Engine) closePosition(ctx context.Context, pos *domain.Position) {
	quote, err := e.moex.GetQuote(ctx, pos.Ticker)
	if err != nil {
		e.log.Error("failed to get quote for close", "ticker", pos.Ticker, "error", err)
		return
	}

	exitPrice := quote.Last
	pnl := pos.UnrealizedPnL(exitPrice)
	returnPct := pos.UnrealizedReturnPct(exitPrice)

	pos.Status = domain.PositionClosed
	pos.ExitPrice = &exitPrice
	now := time.Now()
	pos.ExitTime = &now
	pos.RealizedPnL = &pnl
	pos.ReturnPct = &returnPct

	e.portfolio.Cash += exitPrice * float64(pos.Quantity)
	e.portfolio.TotalPnL += pnl
	e.portfolio.DailyPnL += pnl
	e.portfolio.TotalTrades++
	if pnl > 0 {
		e.portfolio.WinRate = float64(e.portfolio.TotalTrades) // will be recalculated
	}

	// Remove from open positions.
	var remaining []domain.Position
	for _, p := range e.openPositions {
		if p.Ticker != pos.Ticker || p.Status != domain.PositionClosed {
			remaining = append(remaining, p)
		}
	}
	e.openPositions = remaining
	e.portfolio.OpenPositions = len(e.openPositions)

	// Persist close to DB.
	if e.positionWriter != nil {
		if dbErr := e.positionWriter.Close(ctx, pos); dbErr != nil {
			e.log.Error("failed to persist position close to DB", "ticker", pos.Ticker, "error", dbErr)
		}
	}

	e.log.Info("position closed",
		"ticker", pos.Ticker,
		"side", pos.Side,
		"pnl", pnl,
		"return_pct", returnPct,
	)
}

// GetPortfolio returns the current portfolio state.
func (e *Engine) GetPortfolio() *domain.Portfolio {
	return e.portfolio
}

// GetOpenPositions returns all open positions.
func (e *Engine) GetOpenPositions() []domain.Position {
	return e.openPositions
}
