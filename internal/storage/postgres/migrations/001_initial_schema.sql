-- MarketPulse_RU: Initial database schema
-- Migration 001

BEGIN;

-- ============================================================
-- Reference tables
-- ============================================================

CREATE TABLE IF NOT EXISTS sectors (
    id   BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

INSERT INTO sectors (code, name) VALUES
    ('OIL_GAS', 'Нефть и газ'),
    ('BANKS', 'Банки'),
    ('RETAIL', 'Ритейл'),
    ('TELECOM', 'Телеком'),
    ('METALS', 'Металлургия'),
    ('CHEMISTRY', 'Химия'),
    ('ENERGY', 'Энергетика'),
    ('IT', 'IT'),
    ('REAL_ESTATE', 'Недвижимость'),
    ('TRANSPORT', 'Транспорт'),
    ('AGRICULTURE', 'Сельское хозяйство'),
    ('FINANCE', 'Финансы')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS regions (
    id                BIGSERIAL PRIMARY KEY,
    code              TEXT NOT NULL UNIQUE,
    name              TEXT NOT NULL,
    importance_weight DOUBLE PRECISION NOT NULL DEFAULT 1.0
);

INSERT INTO regions (code, name, importance_weight) VALUES
    ('RU', 'Россия', 1.0),
    ('US', 'США', 0.8),
    ('EU', 'Европа', 0.7),
    ('CN', 'Китай', 0.6),
    ('UK', 'Великобритания', 0.5)
ON CONFLICT (code) DO NOTHING;

-- ============================================================
-- Core entity tables
-- ============================================================

CREATE TABLE IF NOT EXISTS companies (
    id           BIGSERIAL PRIMARY KEY,
    ticker       TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    sector       TEXT REFERENCES sectors(code),
    country      TEXT NOT NULL DEFAULT 'RU',
    moex_id      TEXT,
    smartlab_id  TEXT,
    market_cap   DOUBLE PRECISION DEFAULT 0,
    is_watchlist BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_companies_sector ON companies(sector);
CREATE INDEX IF NOT EXISTS idx_companies_country ON companies(country);

CREATE TABLE IF NOT EXISTS bonds (
    id            BIGSERIAL PRIMARY KEY,
    isin          TEXT NOT NULL UNIQUE,
    issuer_id     BIGINT REFERENCES companies(id),
    name          TEXT NOT NULL,
    coupon_rate   DOUBLE PRECISION,
    coupon_freq   INT DEFAULT 2,
    maturity      DATE,
    rating        TEXT,
    rating_agency TEXT,
    nominal       DOUBLE PRECISION DEFAULT 1000,
    currency      TEXT NOT NULL DEFAULT 'RUB',
    current_price DOUBLE PRECISION,
    ytm           DOUBLE PRECISION,
    moex_id       TEXT,
    is_watchlist  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bonds_issuer ON bonds(issuer_id);
CREATE INDEX IF NOT EXISTS idx_bonds_maturity ON bonds(maturity);

CREATE TABLE IF NOT EXISTS commodities (
    id        BIGSERIAL PRIMARY KEY,
    code      TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL,
    exchange  TEXT,
    currency  TEXT NOT NULL DEFAULT 'USD',
    unit      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO commodities (code, name, exchange, currency, unit) VALUES
    ('BRENT', 'Нефть Brent', 'ICE', 'USD', 'barrel'),
    ('WTI', 'Нефть WTI', 'NYMEX', 'USD', 'barrel'),
    ('NG', 'Природный газ', 'NYMEX', 'USD', 'MMBtu'),
    ('GOLD', 'Золото', 'COMEX', 'USD', 'troy oz'),
    ('SILVER', 'Серебро', 'COMEX', 'USD', 'troy oz'),
    ('PALLADIUM', 'Палладий', 'NYMEX', 'USD', 'troy oz'),
    ('PLATINUM', 'Платина', 'NYMEX', 'USD', 'troy oz'),
    ('COPPER', 'Медь', 'LME', 'USD', 'ton'),
    ('ALUMINIUM', 'Алюминий', 'LME', 'USD', 'ton'),
    ('NICKEL', 'Никель', 'LME', 'USD', 'ton'),
    ('WHEAT', 'Пшеница', 'CBOT', 'USD', 'bushel')
ON CONFLICT (code) DO NOTHING;

-- ============================================================
-- News tables
-- ============================================================

CREATE TABLE IF NOT EXISTS news (
    id             BIGSERIAL PRIMARY KEY,
    external_id    TEXT NOT NULL,
    source         TEXT NOT NULL, -- telegram, rss, web, api
    source_channel TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    content        TEXT NOT NULL,
    url            TEXT,
    media_urls     TEXT[],
    published_at   TIMESTAMPTZ NOT NULL,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw_json       JSONB,
    UNIQUE(source, external_id)
);

CREATE INDEX IF NOT EXISTS idx_news_published ON news(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_news_source ON news(source, source_channel);
CREATE INDEX IF NOT EXISTS idx_news_collected ON news(collected_at DESC);

CREATE TABLE IF NOT EXISTS news_analysis (
    id                BIGSERIAL PRIMARY KEY,
    news_id           BIGINT NOT NULL REFERENCES news(id) ON DELETE CASCADE,
    analyzed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    category          TEXT NOT NULL,
    sentiment         DOUBLE PRECISION NOT NULL DEFAULT 0, -- -1.0 to +1.0
    urgency           INT NOT NULL DEFAULT 1,               -- 1-5
    reliability_score DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    summary_ru        TEXT NOT NULL DEFAULT '',
    key_facts         JSONB,
    llm_model         TEXT NOT NULL DEFAULT '',
    UNIQUE(news_id)
);

CREATE INDEX IF NOT EXISTS idx_news_analysis_category ON news_analysis(category);
CREATE INDEX IF NOT EXISTS idx_news_analysis_urgency ON news_analysis(urgency DESC);

CREATE TABLE IF NOT EXISTS news_impacts (
    id               BIGSERIAL PRIMARY KEY,
    news_id          BIGINT NOT NULL REFERENCES news(id) ON DELETE CASCADE,
    entity_type      TEXT NOT NULL, -- company, bond, commodity, sector, region
    entity_id        BIGINT NOT NULL,
    impact_direction INT NOT NULL DEFAULT 0,       -- -1, 0, +1
    impact_magnitude DOUBLE PRECISION DEFAULT 0.0, -- 0.0 to 1.0
    impact_timeframe TEXT DEFAULT 'SHORT',          -- IMMEDIATE, SHORT, MEDIUM, LONG
    confidence       DOUBLE PRECISION DEFAULT 0.5,
    reasoning        TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_news_impacts_entity ON news_impacts(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_news_impacts_news ON news_impacts(news_id);

-- ============================================================
-- Aggregation tables
-- ============================================================

CREATE TABLE IF NOT EXISTS heat_scores (
    id               BIGSERIAL PRIMARY KEY,
    entity_type      TEXT NOT NULL,
    entity_id        BIGINT NOT NULL,
    date             DATE NOT NULL,
    timeframe        TEXT NOT NULL DEFAULT '1d', -- 1d, 7d, 30d
    score            DOUBLE PRECISION NOT NULL DEFAULT 0,
    score_change     DOUBLE PRECISION NOT NULL DEFAULT 0,
    positive_signals INT NOT NULL DEFAULT 0,
    negative_signals INT NOT NULL DEFAULT 0,
    top_news_ids     BIGINT[],
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(entity_type, entity_id, date, timeframe)
);

CREATE INDEX IF NOT EXISTS idx_heat_scores_entity ON heat_scores(entity_type, entity_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_heat_scores_date ON heat_scores(date DESC);

-- ============================================================
-- Alert tables
-- ============================================================

CREATE TABLE IF NOT EXISTS alerts (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    alert_type      TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       BIGINT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'INFO', -- INFO, IMPORTANT, URGENT, CRITICAL
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    source_news_ids BIGINT[],
    sent_to_telegram BOOLEAN NOT NULL DEFAULT FALSE,
    sent_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_entity ON alerts(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_alerts_unsent ON alerts(sent_to_telegram) WHERE sent_to_telegram = FALSE;

-- ============================================================
-- User tables
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    telegram_id   BIGINT UNIQUE,
    telegram_user TEXT,
    email         TEXT UNIQUE,
    tier          TEXT NOT NULL DEFAULT 'FREE',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS watchlist (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id   BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, entity_type, entity_id)
);

CREATE TABLE IF NOT EXISTS alert_preferences (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    min_severity     TEXT NOT NULL DEFAULT 'IMPORTANT',
    enable_telegram  BOOLEAN NOT NULL DEFAULT TRUE,
    quiet_hours_start INT,
    quiet_hours_end   INT
);

-- ============================================================
-- Populate some well-known Russian companies
-- ============================================================

INSERT INTO companies (ticker, name, sector, country) VALUES
    ('SBER', 'Сбербанк', 'BANKS', 'RU'),
    ('GAZP', 'Газпром', 'OIL_GAS', 'RU'),
    ('LKOH', 'Лукойл', 'OIL_GAS', 'RU'),
    ('GMKN', 'Норникель', 'METALS', 'RU'),
    ('ROSN', 'Роснефть', 'OIL_GAS', 'RU'),
    ('YNDX', 'Яндекс', 'IT', 'RU'),
    ('NVTK', 'Новатэк', 'OIL_GAS', 'RU'),
    ('MTSS', 'МТС', 'TELECOM', 'RU'),
    ('MGNT', 'Магнит', 'RETAIL', 'RU'),
    ('VTBR', 'ВТБ', 'BANKS', 'RU'),
    ('PLZL', 'Полюс', 'METALS', 'RU'),
    ('ALRS', 'АЛРОСА', 'METALS', 'RU'),
    ('CHMF', 'Северсталь', 'METALS', 'RU'),
    ('NLMK', 'НЛМК', 'METALS', 'RU'),
    ('MAGN', 'ММК', 'METALS', 'RU'),
    ('TATN', 'Татнефть', 'OIL_GAS', 'RU'),
    ('SNGS', 'Сургутнефтегаз', 'OIL_GAS', 'RU'),
    ('MOEX', 'Мосбиржа', 'FINANCE', 'RU'),
    ('TCSG', 'Т-Банк (Тинькофф)', 'BANKS', 'RU'),
    ('PHOR', 'ФосАгро', 'CHEMISTRY', 'RU'),
    ('RUAL', 'РУСАЛ', 'METALS', 'RU'),
    ('AFKS', 'АФК Система', 'FINANCE', 'RU'),
    ('PIKK', 'ПИК', 'REAL_ESTATE', 'RU'),
    ('HYDR', 'РусГидро', 'ENERGY', 'RU'),
    ('IRAO', 'Интер РАО', 'ENERGY', 'RU'),
    ('FIVE', 'X5 Group', 'RETAIL', 'RU'),
    ('POLY', 'Полиметалл', 'METALS', 'RU'),
    ('FLOT', 'Совкомфлот', 'TRANSPORT', 'RU'),
    ('OZON', 'Озон', 'IT', 'RU'),
    ('VKCO', 'ВК', 'IT', 'RU')
ON CONFLICT (ticker) DO NOTHING;

COMMIT;
