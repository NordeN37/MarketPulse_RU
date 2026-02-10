package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/signals"
	"github.com/NordeN37/MarketPulse_RU/internal/trading/strategy"
)

// BacktestService runs backtests for all 3 strategies and stores equity curves.
type BacktestService struct {
	candleBackfill *CandleBackfill
	portfolioRepo  *postgres.PortfolioRepo
	tradeRepo      *postgres.TradeRepo
	signalRepo     *postgres.SignalRepo
	log            *slog.Logger
	initialCash    float64
	stopLossPct    float64
	takeProfitPct  float64
}

// NewBacktestService creates a backtest service.
func NewBacktestService(
	candleBackfill *CandleBackfill,
	portfolioRepo *postgres.PortfolioRepo,
	tradeRepo *postgres.TradeRepo,
	signalRepo *postgres.SignalRepo,
	log *slog.Logger,
	initialCash float64,
	stopLossPct float64,
	takeProfitPct float64,
) *BacktestService {
	return &BacktestService{
		candleBackfill: candleBackfill,
		portfolioRepo:  portfolioRepo,
		tradeRepo:      tradeRepo,
		signalRepo:     signalRepo,
		log:            log,
		initialCash:    initialCash,
		stopLossPct:    stopLossPct,
		takeProfitPct:  takeProfitPct,
	}
}

// BacktestResult holds results and equity curve for one strategy.
type BacktestResult struct {
	Strategy    string
	Result      strategy.BacktestResult
	EquityCurve []postgres.PortfolioSnapshot
}

// RunAll executes backtests for all 3 strategies on each ticker and aggregates results.
func (s *BacktestService) RunAll(ctx context.Context, tickers []string) ([]BacktestResult, error) {
	s.log.Info("running backtests for all strategies",
		"tickers", tickers,
		"initial_cash", s.initialCash,
	)

	strategies := []struct {
		name string
		mode domain.TradingMode
	}{
		{"ta", domain.ModeTA},
		{"news", domain.ModeNews},
		{"combined", domain.ModeCombined},
	}

	var results []BacktestResult

	for _, strat := range strategies {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}

		s.log.Info("running backtest", "strategy", strat.name)

		result, err := s.runStrategy(ctx, strat.name, strat.mode, tickers)
		if err != nil {
			s.log.Error("backtest error", "strategy", strat.name, "error", err)
			continue
		}

		results = append(results, *result)
		s.log.Info("backtest complete",
			"strategy", strat.name,
			"total_trades", result.Result.TotalTrades,
			"total_pnl", result.Result.TotalPnL,
			"win_rate", result.Result.WinRate,
			"max_drawdown", result.Result.MaxDrawdown,
		)
	}

	return results, nil
}

func (s *BacktestService) runStrategy(ctx context.Context, name string, mode domain.TradingMode, tickers []string) (*BacktestResult, error) {
	// Clear previous snapshots for this strategy.
	if err := s.portfolioRepo.DeleteByStrategy(ctx, name); err != nil {
		s.log.Warn("could not delete old snapshots", "strategy", name, "error", err)
	}

	taGen := signals.NewTASignalGenerator()
	var allSnapshots []postgres.PortfolioSnapshot
	var aggregatedResult strategy.BacktestResult
	aggregatedResult.Strategy = name

	cashPerTicker := s.initialCash / float64(len(tickers))

	for _, ticker := range tickers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Get cached daily candles.
		candles, err := s.candleBackfill.GetCandlesFromDB(ctx, ticker, "1d")
		if err != nil || len(candles) < 60 {
			s.log.Debug("insufficient candle data", "ticker", ticker, "count", len(candles))
			continue
		}

		// Create signal function based on strategy mode.
		signalFn := s.makeSignalFunc(mode, taGen, ticker, candles)

		// Run backtest.
		result := strategy.Backtest(candles, signalFn, s.stopLossPct, s.takeProfitPct)
		result.Strategy = name
		result.Ticker = ticker

		// Aggregate.
		aggregatedResult.Winners += result.Winners
		aggregatedResult.Losers += result.Losers
		aggregatedResult.TotalPnL += result.TotalPnL
		aggregatedResult.Trades = append(aggregatedResult.Trades, result.Trades...)
		if result.MaxDrawdown > aggregatedResult.MaxDrawdown {
			aggregatedResult.MaxDrawdown = result.MaxDrawdown
		}

		// Generate equity curve snapshots from trades.
		tickerSnapshots := s.buildEquityCurve(name, ticker, candles, result, cashPerTicker)
		allSnapshots = append(allSnapshots, tickerSnapshots...)

		// Save signals from backtest trades.
		for _, trade := range result.Trades {
			if trade.EntryIdx >= 0 && trade.EntryIdx < len(candles) {
				sig := &domain.Signal{
					Ticker:    ticker,
					Direction: trade.Direction,
					Source:    s.modeToSource(mode),
					Strength:  0.5,
					Price:     trade.EntryPrice,
					Reason:    fmt.Sprintf("backtest %s", name),
					CreatedAt: time.Unix(candles[trade.EntryIdx].OpenTime, 0),
					ExpiresAt: time.Unix(candles[trade.EntryIdx].OpenTime, 0).Add(24 * time.Hour),
					Executed:  true,
				}
				s.signalRepo.Insert(ctx, sig)
			}
		}
	}

	aggregatedResult.TotalTrades = aggregatedResult.Winners + aggregatedResult.Losers
	if aggregatedResult.TotalTrades > 0 {
		aggregatedResult.WinRate = float64(aggregatedResult.Winners) / float64(aggregatedResult.TotalTrades)
		aggregatedResult.AvgReturn = aggregatedResult.TotalPnL / float64(aggregatedResult.TotalTrades)
	}

	// Merge equity curves from all tickers into a combined daily curve.
	mergedSnapshots := s.mergeEquityCurves(name, allSnapshots, tickers)

	// Save merged snapshots.
	if err := s.portfolioRepo.BulkInsertSnapshots(ctx, mergedSnapshots); err != nil {
		return nil, fmt.Errorf("saving portfolio snapshots: %w", err)
	}

	return &BacktestResult{
		Strategy:    name,
		Result:      aggregatedResult,
		EquityCurve: mergedSnapshots,
	}, nil
}

// makeSignalFunc creates a signal function appropriate for the given mode.
func (s *BacktestService) makeSignalFunc(mode domain.TradingMode, taGen *signals.TASignalGenerator, ticker string, allCandles []domain.Candle) func(candles []domain.Candle, i int) (domain.SignalDirection, float64) {
	switch mode {
	case domain.ModeTA:
		return func(candles []domain.Candle, i int) (domain.SignalDirection, float64) {
			if i < 50 {
				return domain.SignalHold, 0
			}
			analysis := taGen.Analyze(ticker, "1d", candles)
			sig := taGen.Generate(analysis)
			if sig == nil {
				return domain.SignalHold, 0
			}
			return sig.Direction, sig.Strength
		}

	case domain.ModeNews:
		// For news-only backtest without historical analyzed news,
		// use a simplified sentiment proxy based on price momentum.
		return func(candles []domain.Candle, i int) (domain.SignalDirection, float64) {
			if i < 20 {
				return domain.SignalHold, 0
			}
			// 5-day momentum as sentiment proxy.
			momentum := (candles[i].Close - candles[i-5].Close) / candles[i-5].Close
			// Volume surge as news catalyst proxy.
			avgVol := 0.0
			for j := i - 10; j < i; j++ {
				avgVol += candles[j].Volume
			}
			avgVol /= 10
			volSurge := 1.0
			if avgVol > 0 {
				volSurge = candles[i].Volume / avgVol
			}

			// Strong momentum + volume surge = signal.
			if momentum > 0.02 && volSurge > 1.5 {
				return domain.SignalBuy, clamp(momentum*5, 0.3, 1.0)
			}
			if momentum < -0.02 && volSurge > 1.5 {
				return domain.SignalSell, clamp(-momentum*5, 0.3, 1.0)
			}
			return domain.SignalHold, 0
		}

	case domain.ModeCombined:
		return func(candles []domain.Candle, i int) (domain.SignalDirection, float64) {
			if i < 50 {
				return domain.SignalHold, 0
			}
			// TA component.
			analysis := taGen.Analyze(ticker, "1d", candles)
			taSig := taGen.Generate(analysis)

			// News proxy component (momentum + volume).
			var newsDir domain.SignalDirection = domain.SignalHold
			newsStr := 0.0
			if i >= 20 {
				momentum := (candles[i].Close - candles[i-5].Close) / candles[i-5].Close
				avgVol := 0.0
				for j := i - 10; j < i; j++ {
					avgVol += candles[j].Volume
				}
				avgVol /= 10
				volSurge := 1.0
				if avgVol > 0 {
					volSurge = candles[i].Volume / avgVol
				}
				if momentum > 0.015 && volSurge > 1.3 {
					newsDir = domain.SignalBuy
					newsStr = clamp(momentum*4, 0.2, 0.8)
				} else if momentum < -0.015 && volSurge > 1.3 {
					newsDir = domain.SignalSell
					newsStr = clamp(-momentum*4, 0.2, 0.8)
				}
			}

			// Combine with 60% news / 40% TA.
			taScore := 0.0
			if taSig != nil {
				taScore = dirScore(taSig.Direction) * taSig.Strength
			}
			newsScore := dirScore(newsDir) * newsStr

			combined := 0.6*newsScore + 0.4*taScore

			if combined > 0.15 {
				return domain.SignalBuy, clamp(combined, 0.3, 1.0)
			}
			if combined < -0.15 {
				return domain.SignalSell, clamp(-combined, 0.3, 1.0)
			}
			return domain.SignalHold, 0
		}
	}

	return func(candles []domain.Candle, i int) (domain.SignalDirection, float64) {
		return domain.SignalHold, 0
	}
}

// buildEquityCurve generates portfolio snapshots from backtest trades.
func (s *BacktestService) buildEquityCurve(strategyName, ticker string, candles []domain.Candle, result strategy.BacktestResult, startCash float64) []postgres.PortfolioSnapshot {
	if len(candles) < 50 {
		return nil
	}

	var snapshots []postgres.PortfolioSnapshot
	equity := startCash
	peak := equity
	tradeIdx := 0
	wins := 0
	totalTrades := 0

	for i := 50; i < len(candles); i++ {
		// Process trades that happened at this index.
		for tradeIdx < len(result.Trades) && result.Trades[tradeIdx].ExitIdx <= i {
			t := result.Trades[tradeIdx]
			// Scale PnL relative to position size (simplified).
			positionValue := equity * 0.02 // 2% per position
			pnl := (t.ReturnPct / 100) * positionValue
			equity += pnl
			totalTrades++
			if t.PnL > 0 {
				wins++
			}
			tradeIdx++
		}

		if equity > peak {
			peak = equity
		}
		dd := 0.0
		if peak > 0 {
			dd = (peak - equity) / peak
		}

		wr := 0.0
		if totalTrades > 0 {
			wr = float64(wins) / float64(totalTrades)
		}

		t := time.Unix(candles[i].OpenTime, 0)
		snapshots = append(snapshots, postgres.PortfolioSnapshot{
			Strategy:    strategyName,
			Cash:        equity,
			TotalValue:  equity,
			TotalPnL:    equity - startCash,
			MaxDrawdown: dd,
			WinRate:     wr,
			TotalTrades: totalTrades,
			SnapshotAt:  t,
		})
	}

	return snapshots
}

// mergeEquityCurves aggregates per-ticker snapshots into a single daily equity curve.
func (s *BacktestService) mergeEquityCurves(strategyName string, allSnapshots []postgres.PortfolioSnapshot, tickers []string) []postgres.PortfolioSnapshot {
	if len(allSnapshots) == 0 {
		return nil
	}

	// Group by date.
	daily := make(map[string]*postgres.PortfolioSnapshot)

	for _, snap := range allSnapshots {
		dateKey := snap.SnapshotAt.Format("2006-01-02")
		existing, ok := daily[dateKey]
		if !ok {
			cp := snap
			daily[dateKey] = &cp
		} else {
			existing.TotalValue += snap.TotalValue - s.initialCash/float64(len(tickers))
			existing.Cash += snap.Cash - s.initialCash/float64(len(tickers))
			existing.TotalPnL += snap.TotalPnL
			existing.TotalTrades += snap.TotalTrades
			if snap.MaxDrawdown > existing.MaxDrawdown {
				existing.MaxDrawdown = snap.MaxDrawdown
			}
		}
	}

	// Sort by date and build final list.
	var dates []string
	for d := range daily {
		dates = append(dates, d)
	}
	sortStrings(dates)

	var merged []postgres.PortfolioSnapshot
	for _, d := range dates {
		snap := daily[d]
		snap.Strategy = strategyName
		merged = append(merged, *snap)
	}

	return merged
}

func (s *BacktestService) modeToSource(mode domain.TradingMode) domain.SignalSource {
	switch mode {
	case domain.ModeTA:
		return domain.SourceTA
	case domain.ModeNews:
		return domain.SourceNews
	default:
		return domain.SourceCombined
	}
}

func dirScore(dir domain.SignalDirection) float64 {
	switch dir {
	case domain.SignalBuy:
		return 1.0
	case domain.SignalSell:
		return -1.0
	default:
		return 0
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
