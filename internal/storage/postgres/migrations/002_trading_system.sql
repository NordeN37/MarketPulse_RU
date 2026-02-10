-- 002_trading_system.sql
-- Trading system: signals, positions, orders, trades, portfolio snapshots, candle cache.

-- =====================================================
-- Trading signals
-- =====================================================
CREATE TABLE IF NOT EXISTS trading_signals (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(20)  NOT NULL,
    direction     VARCHAR(10)  NOT NULL,  -- BUY, SELL, HOLD
    source        VARCHAR(20)  NOT NULL,  -- NEWS, TA, COMBINED
    strength      DOUBLE PRECISION NOT NULL DEFAULT 0,
    price         DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason        TEXT         NOT NULL DEFAULT '',
    executed      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW() + INTERVAL '1 hour'
);

CREATE INDEX IF NOT EXISTS idx_signals_ticker_created ON trading_signals (ticker, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_signals_source ON trading_signals (source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_signals_active ON trading_signals (executed, expires_at) WHERE NOT executed;

-- =====================================================
-- Positions (open & closed)
-- =====================================================
CREATE TABLE IF NOT EXISTS positions (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(20)      NOT NULL,
    side          VARCHAR(10)      NOT NULL,  -- LONG, SHORT
    status        VARCHAR(10)      NOT NULL DEFAULT 'OPEN',
    quantity      INTEGER          NOT NULL,
    entry_price   DOUBLE PRECISION NOT NULL,
    entry_time    TIMESTAMPTZ      NOT NULL,
    entry_order   BIGINT           NOT NULL,
    exit_price    DOUBLE PRECISION,
    exit_time     TIMESTAMPTZ,
    exit_order    BIGINT,
    stop_loss     DOUBLE PRECISION,
    take_profit   DOUBLE PRECISION,
    realized_pnl  DOUBLE PRECISION,
    return_pct    DOUBLE PRECISION,
    signal_id     BIGINT           NOT NULL,
    strategy      VARCHAR(50)      NOT NULL DEFAULT 'combined',
    created_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_positions_status ON positions (status);
CREATE INDEX IF NOT EXISTS idx_positions_ticker ON positions (ticker, status);

-- =====================================================
-- Orders
-- =====================================================
CREATE TABLE IF NOT EXISTS orders (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(20)      NOT NULL,
    side          VARCHAR(10)      NOT NULL,  -- BUY, SELL
    order_type    VARCHAR(20)      NOT NULL,  -- MARKET, LIMIT, STOP_LOSS, TAKE_PROFIT
    status        VARCHAR(20)      NOT NULL DEFAULT 'PENDING',
    quantity      INTEGER          NOT NULL,
    price         DOUBLE PRECISION,
    stop_price    DOUBLE PRECISION,
    filled_qty    INTEGER          NOT NULL DEFAULT 0,
    filled_price  DOUBLE PRECISION,
    commission    DOUBLE PRECISION NOT NULL DEFAULT 0,
    signal_id     BIGINT,
    position_id   BIGINT,
    external_id   VARCHAR(100)     NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    filled_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
CREATE INDEX IF NOT EXISTS idx_orders_position ON orders (position_id);

-- =====================================================
-- Completed trades (for analytics)
-- =====================================================
CREATE TABLE IF NOT EXISTS trades (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(20)      NOT NULL,
    side          VARCHAR(10)      NOT NULL,
    strategy      VARCHAR(50)      NOT NULL,
    quantity      INTEGER          NOT NULL,
    entry_price   DOUBLE PRECISION NOT NULL,
    exit_price    DOUBLE PRECISION NOT NULL,
    entry_time    TIMESTAMPTZ      NOT NULL,
    exit_time     TIMESTAMPTZ      NOT NULL,
    pnl           DOUBLE PRECISION NOT NULL,
    return_pct    DOUBLE PRECISION NOT NULL,
    commission    DOUBLE PRECISION NOT NULL DEFAULT 0,
    position_id   BIGINT           NOT NULL,
    created_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trades_ticker ON trades (ticker, exit_time DESC);
CREATE INDEX IF NOT EXISTS idx_trades_strategy ON trades (strategy, exit_time DESC);
CREATE INDEX IF NOT EXISTS idx_trades_pnl ON trades (pnl);

-- =====================================================
-- Portfolio snapshots (daily, for tracking equity curve)
-- =====================================================
CREATE TABLE IF NOT EXISTS portfolio_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    cash            DOUBLE PRECISION NOT NULL,
    total_value     DOUBLE PRECISION NOT NULL,
    open_positions  INTEGER          NOT NULL DEFAULT 0,
    daily_pnl       DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_pnl       DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_drawdown    DOUBLE PRECISION NOT NULL DEFAULT 0,
    win_rate        DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_trades    INTEGER          NOT NULL DEFAULT 0,
    snapshot_at     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_portfolio_snapshot_time ON portfolio_snapshots (snapshot_at DESC);

-- =====================================================
-- OHLCV candle cache
-- =====================================================
CREATE TABLE IF NOT EXISTS candles (
    ticker    VARCHAR(20)      NOT NULL,
    interval  VARCHAR(10)      NOT NULL,  -- 1m, 10m, 1h, 1d
    open_time BIGINT           NOT NULL,  -- unix timestamp
    open      DOUBLE PRECISION NOT NULL,
    high      DOUBLE PRECISION NOT NULL,
    low       DOUBLE PRECISION NOT NULL,
    close     DOUBLE PRECISION NOT NULL,
    volume    DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (ticker, interval, open_time)
);

CREATE INDEX IF NOT EXISTS idx_candles_lookup ON candles (ticker, interval, open_time DESC);
