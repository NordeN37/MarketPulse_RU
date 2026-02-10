-- MarketPulse_RU: Expand monitoring universe
-- Migration 002: Add more MOEX companies + sectors

BEGIN;

-- Add missing sectors
INSERT INTO sectors (code, name) VALUES
    ('CONSTRUCTION', 'Строительство'),
    ('FOOD', 'Пищевая промышленность'),
    ('MINING', 'Добыча'),
    ('PHARMA', 'Фармацевтика'),
    ('INSURANCE', 'Страхование')
ON CONFLICT (code) DO NOTHING;

-- Expand company universe to ~50 (MOEX index + widely tracked)
INSERT INTO companies (ticker, name, sector, country) VALUES
    -- Нефтегаз (дополнительно)
    ('SIBN', 'Газпром нефть', 'OIL_GAS', 'RU'),
    ('TRNFP', 'Транснефть', 'OIL_GAS', 'RU'),
    ('BANE', 'Башнефть', 'OIL_GAS', 'RU'),
    -- Банки / Финансы
    ('BSPB', 'Банк Санкт-Петербург', 'BANKS', 'RU'),
    ('CBOM', 'МКБ', 'BANKS', 'RU'),
    ('RENI', 'Ренессанс Страхование', 'INSURANCE', 'RU'),
    ('SPBE', 'СПБ Биржа', 'FINANCE', 'RU'),
    -- Металлургия / Добыча
    ('TRMK', 'ТМК', 'METALS', 'RU'),
    ('MTLR', 'Мечел', 'METALS', 'RU'),
    ('RASP', 'Распадская', 'MINING', 'RU'),
    -- Энергетика
    ('FEES', 'ФСК ЕЭС', 'ENERGY', 'RU'),
    ('UPRO', 'Юнипро', 'ENERGY', 'RU'),
    ('MSNG', 'Мосэнерго', 'ENERGY', 'RU'),
    ('OGKB', 'ОГК-2', 'ENERGY', 'RU'),
    -- Телеком
    ('RTKM', 'Ростелеком', 'TELECOM', 'RU'),
    -- Ритейл / Потребительский
    ('LENT', 'Лента', 'RETAIL', 'RU'),
    ('FIXP', 'Fix Price', 'RETAIL', 'RU'),
    -- IT / Технологии
    ('HHRU', 'HeadHunter', 'IT', 'RU'),
    ('POSI', 'Positive Technologies', 'IT', 'RU'),
    ('CIAN', 'Циан', 'IT', 'RU'),
    ('ASTR', 'Астра', 'IT', 'RU'),
    -- Строительство / Недвижимость
    ('SMLT', 'Самолёт', 'REAL_ESTATE', 'RU'),
    ('LSRG', 'ЛСР', 'REAL_ESTATE', 'RU'),
    -- Химия / Удобрения
    ('AKRN', 'Акрон', 'CHEMISTRY', 'RU'),
    -- Транспорт
    ('AFLT', 'Аэрофлот', 'TRANSPORT', 'RU'),
    ('NMTP', 'НМТП', 'TRANSPORT', 'RU'),
    ('GLTR', 'Глобалтранс', 'TRANSPORT', 'RU'),
    -- Потребительский / Пищевая
    ('AGRO', 'РусАгро', 'AGRICULTURE', 'RU'),
    ('MDMG', 'MD Medical (Мать и дитя)', 'PHARMA', 'RU')
ON CONFLICT (ticker) DO NOTHING;

-- Add more commodities for broader monitoring
INSERT INTO commodities (code, name, exchange, currency, unit) VALUES
    ('IRON', 'Железная руда', 'SGX', 'USD', 'ton'),
    ('UREA', 'Карбамид (удобрения)', 'CME', 'USD', 'ton'),
    ('COAL', 'Уголь энерг.', 'ICE', 'USD', 'ton'),
    ('RUBBER', 'Каучук', 'SGX', 'USD', 'ton'),
    ('CORN', 'Кукуруза', 'CBOT', 'USD', 'bushel')
ON CONFLICT (code) DO NOTHING;

COMMIT;
