-- MarketPulse_RU: Expand commodities universe
-- Migration 005: Add precious metals, industrial metals, energy, agriculture

BEGIN;

INSERT INTO commodities (code, name, exchange, currency, unit) VALUES
    -- Промышленные металлы (LME)
    ('TIN', 'Олово', 'LME', 'USD', 'ton'),
    ('ZINC', 'Цинк', 'LME', 'USD', 'ton'),
    ('LEAD', 'Свинец', 'LME', 'USD', 'ton'),
    ('COBALT', 'Кобальт', 'LME', 'USD', 'ton'),

    -- Драгоценные металлы (дополнительно)
    ('RHODIUM', 'Родий', 'NYMEX', 'USD', 'troy oz'),

    -- Энергоносители
    ('TTF', 'Газ TTF (Европа)', 'ICE', 'EUR', 'MWh'),
    ('LNG', 'СПГ JKM (Азия)', 'CME', 'USD', 'MMBtu'),
    ('URALS', 'Нефть Urals', 'СПбМТСБ', 'USD', 'barrel'),

    -- Сельскохозяйственные товары (CBOT/ICE/NYBOT)
    ('SOYBEAN', 'Соя', 'CBOT', 'USD', 'bushel'),
    ('SOYMEAL', 'Соевый шрот', 'CBOT', 'USD', 'ton'),
    ('SOYOIL', 'Соевое масло', 'CBOT', 'USD', 'lb'),
    ('COCOA', 'Какао-бобы', 'ICE', 'USD', 'ton'),
    ('COFFEE', 'Кофе арабика', 'ICE', 'USD', 'lb'),
    ('SUGAR', 'Сахар-сырец', 'ICE', 'USD', 'lb'),
    ('COTTON', 'Хлопок', 'ICE', 'USD', 'lb'),
    ('RICE', 'Рис', 'CBOT', 'USD', 'cwt'),
    ('OATS', 'Овёс', 'CBOT', 'USD', 'bushel'),
    ('LUMBER', 'Пиломатериалы', 'CME', 'USD', 'board feet'),
    ('PALM', 'Пальмовое масло', 'MDEX', 'MYR', 'ton'),
    ('RAPESEED', 'Рапс', 'Euronext', 'EUR', 'ton'),

    -- Удобрения / химия
    ('POTASH', 'Хлорид калия', 'OTC', 'USD', 'ton'),

    -- Редкоземельные / стратегические
    ('LITHIUM', 'Литий', 'CME', 'USD', 'kg'),
    ('URANIUM', 'Уран', 'UxC', 'USD', 'lb')
ON CONFLICT (code) DO NOTHING;

COMMIT;
