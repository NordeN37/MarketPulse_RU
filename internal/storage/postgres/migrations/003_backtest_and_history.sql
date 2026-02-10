-- 003_backtest_and_history.sql
-- Add strategy column to portfolio_snapshots and backtest results table.

-- Add strategy to portfolio_snapshots for per-strategy equity curves.
ALTER TABLE portfolio_snapshots ADD COLUMN IF NOT EXISTS strategy VARCHAR(20) NOT NULL DEFAULT 'combined';
CREATE INDEX IF NOT EXISTS idx_portfolio_strategy ON portfolio_snapshots (strategy, snapshot_at ASC);

-- Backtest results cache.
CREATE TABLE IF NOT EXISTS backtest_results (
    id              BIGSERIAL PRIMARY KEY,
    strategy        VARCHAR(20)      NOT NULL,  -- "news", "ta", "combined"
    ticker          VARCHAR(20)      NOT NULL DEFAULT '',  -- '' = all tickers
    period_from     TIMESTAMPTZ      NOT NULL,
    period_to       TIMESTAMPTZ      NOT NULL,
    initial_cash    DOUBLE PRECISION NOT NULL,
    final_value     DOUBLE PRECISION NOT NULL,
    total_pnl       DOUBLE PRECISION NOT NULL,
    total_trades    INTEGER          NOT NULL DEFAULT 0,
    winners         INTEGER          NOT NULL DEFAULT 0,
    losers          INTEGER          NOT NULL DEFAULT 0,
    win_rate        DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_drawdown    DOUBLE PRECISION NOT NULL DEFAULT 0,
    sharpe_ratio    DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_return      DOUBLE PRECISION NOT NULL DEFAULT 0,
    computed_at     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_backtest_strategy ON backtest_results (strategy, computed_at DESC);
