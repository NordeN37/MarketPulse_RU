# MarketPulse_RU

Аналитическая платформа для российского финансового рынка. Собирает новости из Telegram-каналов и RSS, анализирует через LLM, генерирует торговые сигналы и управляет портфелями.

## Архитектура

```
┌──────────────┐  ┌──────────────┐
│  Telegram    │  │    RSS       │
│  (MTProto)   │  │  (5 feeds)   │
└──────┬───────┘  └──────┬───────┘
       │                 │
       └────────┬────────┘
                ▼
┌──────────────────────────┐     ┌──────────────┐
│      COLLECTOR           │────▶│    Redis      │
│  dedup + queue           │     │  (queue+cache)│
└──────────────────────────┘     └──────┬───────┘
                                        │
┌──────────────────────────┐            │
│      ANALYZER            │◀───────────┘
│  LLM classify + score    │
│  qwen3:8b (fast, 80%)    │     ┌──────────────┐
│  qwen3:14b (heavy, 20%)  │────▶│  PostgreSQL   │
└──────────────────────────┘     └──────┬───────┘
                                        │
┌──────────────────────────┐            │
│      TRADER              │◀───────────┘
│  3 modes: news/ta/combo  │
│  15 TA indicators        │     ┌──────────────┐
│  risk management         │────▶│  MOEX ISS    │
└──────────────────────────┘     └──────────────┘
                                        │
┌──────────────────────────┐            │
│      API + WEB UI        │◀───────────┘
│  REST API + Vue.js SPA   │
│  TradingView charts      │
└──────────────────────────┘
```

## 5 микросервисов

| Сервис | Команда | Описание |
|--------|---------|----------|
| **collector** | `make run-collector` | Telegram MTProto userbot + RSS сборщик |
| **analyzer** | `make run-analyzer` | LLM-анализ новостей (классификация, sentiment, impact) |
| **alerter** | `make run-alerter` | Генерация и отправка алертов в Telegram |
| **trader** | `make run-trader` | Торговая система (TA + News сигналы, risk management) |
| **api** | `make run-api` | REST API + Web-интерфейс (Vue.js) |

## Быстрый старт

### 1. Запуск инфраструктуры

```bash
make infra-up  # PostgreSQL 16 + Redis 7
```

### 2. Применение миграций

```bash
psql -h localhost -U marketpulse -d marketpulse -f internal/storage/postgres/migrations/001_initial_schema.sql
psql -h localhost -U marketpulse -d marketpulse -f internal/storage/postgres/migrations/002_trading_system.sql
```

### 3. Установка Ollama + модели

```bash
# Установить Ollama: https://ollama.com/download
ollama pull qwen3:8b     # ~5GB, быстрая классификация
ollama pull qwen3:14b    # ~10GB, глубокий анализ
```

### 4. Настройка переменных окружения

```bash
cp .env.example .env
# Заполнить креды (см. ниже)
source .env
```

### 5. Запуск сервисов

```bash
make run-api         # Web UI: http://localhost:8080
make run-collector   # в отдельном терминале
make run-analyzer    # в отдельном терминале
make run-trader      # в отдельном терминале
make run-alerter     # в отдельном терминале
```

## Необходимые креды

### Обязательные

| Переменная | Где получить | Описание |
|------------|-------------|----------|
| `TELEGRAM_API_ID` | [my.telegram.org](https://my.telegram.org) → API development tools | MTProto API ID для userbot |
| `TELEGRAM_API_HASH` | [my.telegram.org](https://my.telegram.org) → API development tools | MTProto API Hash |
| `TELEGRAM_PHONE` | — | Номер телефона аккаунта (формат: +7XXXXXXXXXX) |

### Для отправки алертов

| Переменная | Где получить | Описание |
|------------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | [@BotFather](https://t.me/BotFather) → /newbot | Токен бота для отправки алертов |
| `TELEGRAM_ALERT_CHAT_ID` | Написать боту, посмотреть через API | ID чата/канала для алертов |

### LLM провайдеры (опциональные, fallback)

| Переменная | Где получить | Цена | Описание |
|------------|-------------|------|----------|
| `DASHSCOPE_API_KEY` | [Alibaba Cloud Model Studio](https://www.alibabacloud.com/product/modelstudio) | $0.40/1M input | Qwen-Plus API, лучший русский язык |
| `DEEPSEEK_API_KEY` | [platform.deepseek.com](https://platform.deepseek.com) | $0.28/1M input | DeepSeek V3, самый дешёвый |
| `CLAUDE_API_KEY` | [console.anthropic.com](https://console.anthropic.com) | $3.00/1M input | Claude, высшее качество |

> **Примечание**: Ollama работает локально и не требует API-ключей. Внешние провайдеры используются только как fallback, когда Ollama недоступен.

### Пример .env файла

```bash
# === Telegram MTProto (обязательно) ===
export TELEGRAM_API_ID=12345678
export TELEGRAM_API_HASH="your_api_hash_here"
export TELEGRAM_PHONE="+79001234567"

# === Telegram Bot (для алертов) ===
export TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
export TELEGRAM_ALERT_CHAT_ID=-1001234567890

# === LLM Providers (опционально, fallback) ===
export DASHSCOPE_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
export DEEPSEEK_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
export CLAUDE_API_KEY="sk-ant-xxxxxxxxxxxxxxxxxxxxxxxx"
```

## LLM цепочка (приоритет fallback)

```
Ollama qwen3:14b (локальный, бесплатно, thinking=on)
    ↓ fallback
Qwen-Plus API (DashScope, $0.40/1M input, лучший русский)
    ↓ fallback
DeepSeek V3 ($0.28/1M input, самый дешёвый)
    ↓ fallback
Claude ($3.00/1M input, максимальное качество)
```

Рутинные задачи (80%) обрабатываются **Ollama qwen3:8b** (thinking=off, быстрый JSON).

## Торговая система

### 3 режима (`configs/config.yaml` → `trading.mode`)

| Режим | Конфиг | Источник сигналов |
|-------|--------|-------------------|
| Новостной | `mode: "news"` | HeatScore + Sentiment → Signal |
| Технический | `mode: "ta"` | 15 индикаторов + паттерны → Score |
| Комбинированный | `mode: "combined"` | News × weight + TA × weight |

### Индикаторы TA

- **Moving Averages**: SMA, EMA, WMA (20/50 периоды)
- **Осцилляторы**: RSI(14), MACD(12,26,9), Stochastic(14,3)
- **Волатильность**: Bollinger Bands(20,2), ATR(14), ADX(14)
- **Объём**: OBV, VWAP, Volume Profile
- **Тренд**: Parabolic SAR, SuperTrend(10,3)
- **Уровни**: Pivot Points, Fibonacci retracement
- **Паттерны**: 10 свечных (Hammer, Engulfing, Morning/Evening Star, etc.)
- **Дивергенции**: RSI bullish/bearish divergence

### Risk Management

| Параметр | Значение по умолчанию |
|----------|----------------------|
| Max позиция | 5% портфеля |
| Max exposure | 50% |
| Max на сектор | 20% |
| Stop-loss | 3% (динамический по ATR) |
| Take-profit | 6% (2:1 risk/reward) |
| Дневной лимит потерь | 2% |
| Max drawdown (circuit breaker) | 10% |
| Max открытых позиций | 10 |

## Web UI

Dashboard доступен по адресу `http://localhost:8080` после запуска `make run-api`.

- **Главный дашборд**: настраиваемые виджеты (drag & drop), сигналы, дайджесты, heatmap
- **Акции**: список с мини-графиками, детальная страница с TradingView-графиком
- **Портфели**: 3 портфеля с бэктестированием, сравнительный график
- **Сигналы**: история всех торговых сигналов
- **Новости**: лента с LLM-анализом

### Технологии UI
- Vue 3 (CDN, без Node.js)
- TradingView Lightweight Charts (свечные графики, маркеры сигналов)
- Gridstack.js (drag & drop виджеты)
- Bootstrap 5 (dark theme)

## API Endpoints

| Метод | URL | Описание |
|-------|-----|----------|
| GET | `/api/health` | Health check + размер очереди |
| GET | `/api/news?limit=20&offset=0` | Лента новостей |
| GET | `/api/news/{id}` | Новость по ID |
| GET | `/api/companies` | Список компаний |
| GET | `/api/companies/{ticker}` | Компания по тикеру |
| GET | `/api/heatmap?type=company&timeframe=1d` | Heat map |
| GET | `/api/alerts?limit=20&min_severity=INFO` | Алерты |
| GET | `/api/quote/{ticker}` | Текущая котировка MOEX |
| GET | `/api/index/{index}` | Значение индекса (IMOEX) |
| GET | `/api/candles/{ticker}?interval=1h&days=30` | OHLCV свечи |
| GET | `/api/signals?ticker=SBER&source=TA` | Торговые сигналы |
| GET | `/api/portfolios` | Состояние 3 портфелей |
| GET | `/api/portfolios/{type}/backtest` | Результат бэктеста |

## Серверные требования

| Ресурс | Минимум | Рекомендуется |
|--------|---------|---------------|
| CPU | 4 ядра | 8 ядер |
| RAM | 16 GB | 32 GB |
| Диск | 20 GB | 50 GB (для истории) |
| GPU | не требуется | не требуется |

### Распределение RAM (8 ядер / 32 GB)

| Компонент | RAM |
|-----------|-----|
| Ollama qwen3:8b | ~5 GB |
| Ollama qwen3:14b | ~10 GB |
| PostgreSQL | ~2 GB |
| Redis | ~500 MB |
| Go сервисы (5 шт.) | ~1 GB |
| TA Engine (свечи в памяти) | ~1 GB |
| **Свободно** | **~12.5 GB** |

## Расчёт нагрузки

- ~470 уникальных новостей/день (после дедупликации)
- ~376 рутинных задач/день → Ollama qwen3:8b (~2.5 часа inference)
- ~94-174 сложных задач/день → Ollama qwen3:14b
- Стоимость API fallback: **~$2-3/месяц** (Qwen-Plus + DeepSeek)

## Структура проекта

```
MarketPulse_RU/
├── cmd/
│   ├── collector/main.go      — Telegram + RSS сборщик
│   ├── analyzer/main.go       — LLM-анализатор
│   ├── api/main.go            — REST API + Web UI
│   ├── alerter/main.go        — Отправка алертов
│   └── trader/main.go         — Торговая система
├── configs/
│   └── config.yaml            — Конфигурация (YAML + env vars)
├── deployments/
│   └── docker-compose.yml     — PostgreSQL + Redis
├── internal/
│   ├── domain/                — Доменные модели
│   ├── config/                — Загрузка конфига
│   ├── storage/postgres/      — PostgreSQL репозитории
│   ├── storage/redis/         — Redis клиент
│   ├── collector/telegram/    — MTProto userbot
│   ├── collector/rss/         — RSS фетчер
│   ├── collector/pipeline.go  — Конвейер обработки
│   ├── llm/ollama/            — Ollama клиент (/api/chat)
│   ├── llm/claude/            — Claude API клиент
│   ├── llm/openai/            — OpenAI-совместимый клиент
│   ├── llm/router.go          — Multi-provider роутер
│   ├── llm/prompts/           — Промпты на русском
│   ├── analyzer/              — Классификатор + скорер
│   ├── alerts/                — Генерация + отправка алертов
│   ├── market/moex/           — MOEX ISS API + свечи
│   └── trading/
│       ├── engine/            — Торговый движок
│       ├── executor/          — Исполнение ордеров
│       ├── indicators/        — TA индикаторы (15 шт.)
│       ├── patterns/          — Свечные паттерны + дивергенции
│       ├── signals/           — Генераторы сигналов
│       ├── strategy/          — Режим рынка + бэктестер
│       └── risk/              — Risk management
└── web/                       — Vue.js SPA (CDN, без Node.js)
    ├── index.html
    ├── css/style.css
    └── js/
        ├── app.js
        ├── api.js
        └── pages/
```

## Лицензия

MIT
