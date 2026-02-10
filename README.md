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
│  3 провайдера (см. ниже) │     ┌──────────────┐
│  batch + round-robin     │────▶│  PostgreSQL   │
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
│  SSE real-time quotes    │
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

### 2. Миграции

Применяются автоматически при запуске любого сервиса (embed.FS + migrations_log). Ручной запуск не нужен.

### 3. Установка Ollama + модели

```bash
# Установить Ollama: https://ollama.com/download
ollama pull qwen3:8b     # ~5GB, быстрая классификация
ollama pull qwen3:14b    # ~10GB, batch + deep analysis
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

## LLM провайдеры

### Стандартный режим (`make run-analyzer`)

```
Классификация (80%)  →  Ollama qwen3:8b (локально, бесплатно, thinking=off)
                              ↓ fallback при ошибке
Тяжёлые задачи (20%) →  Ollama qwen3:14b → Qwen-Plus API → DeepSeek API
```

### Batch режим (`make run-analyzer-batch`)

Для первичной прогонки всей базы. 8 воркеров, round-robin по всем провайдерам одновременно:

```
┌─ Ollama qwen3:14b (локально, бесплатно)
├─ Qwen-Plus API    ($0.40/1M input, $1.20/1M output)
└─ DeepSeek API     ($0.28/1M input, $0.42/1M output)
```

Если один провайдер падает — запрос автоматически уходит на другой.
429 rate-limit обрабатывается с exponential backoff.

Можно менять воркеров: `go run ./cmd/analyzer -config configs/config.yaml -batch -workers 16`

### Расчёт стоимости batch (15,000 новостей)

| Провайдер | Доля | Стоимость | Скорость |
|-----------|------|-----------|----------|
| Ollama 14b | 33% | $0 | ~7/мин |
| Qwen-Plus | 33% | ~$2.3 | ~25/мин |
| DeepSeek | 33% | ~$1.1 | ~25/мин |
| **Итого** | | **~$3.4** | **~4-5 ч** |

## Необходимые креды

### Обязательные

| Переменная | Где получить | Описание |
|------------|-------------|----------|
| `TELEGRAM_API_ID` | [my.telegram.org](https://my.telegram.org) | MTProto API ID для userbot |
| `TELEGRAM_API_HASH` | [my.telegram.org](https://my.telegram.org) | MTProto API Hash |
| `TELEGRAM_PHONE` | — | Номер телефона (формат: +7XXXXXXXXXX) |

### Для отправки алертов

| Переменная | Где получить | Описание |
|------------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | [@BotFather](https://t.me/BotFather) | Токен бота для алертов |
| `TELEGRAM_ALERT_CHAT_ID` | Написать боту, посмотреть через API | ID чата/канала |

### LLM провайдеры (опциональные, fallback)

| Переменная | Где получить | Цена | Описание |
|------------|-------------|------|----------|
| `DASHSCOPE_API_KEY` | [Alibaba Cloud Model Studio](https://www.alibabacloud.com/product/modelstudio) | $0.40/1M in | Qwen-Plus, лучший русский |
| `DEEPSEEK_API_KEY` | [platform.deepseek.com](https://platform.deepseek.com) | $0.28/1M in | DeepSeek, самый дешёвый |

> Ollama работает локально и не требует API-ключей. Внешние провайдеры — fallback + batch ускорение.

### Пример .env файла

```bash
# === Telegram MTProto (обязательно) ===
export TELEGRAM_API_ID=12345678
export TELEGRAM_API_HASH="your_api_hash_here"
export TELEGRAM_PHONE="+79001234567"

# === Telegram Bot (для алертов) ===
export TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
export TELEGRAM_ALERT_CHAT_ID=-1001234567890

# === LLM Providers (опционально) ===
export DASHSCOPE_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
export DEEPSEEK_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
```

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

Dashboard: `http://localhost:8080` после `make run-api`.

- **Дашборд**: виджеты, сигналы, heatmap, индекс IMOEX
- **Акции**: список с real-time котировками (SSE), детальная страница с TradingView-графиком
- **Новости**: лента с LLM-анализом, авто-обновление 30с
- **Портфели**: 3 портфеля с бэктестированием
- **Настройки**: Telegram авторизация, управление источниками (re-read RSS/TG каналов)

### Real-time обновления

- **Котировки**: SSE (Server-Sent Events) через `/api/stream/quotes`, обновление каждые 10с
- **Новости**: авто-обновление каждые 30с на первой странице
- Единый `QuoteStream` на фронте — один SSE на все страницы

### Технологии UI
- Vue 3 (CDN, без Node.js)
- TradingView Lightweight Charts (свечные графики, маркеры сигналов)
- Bootstrap 5 (dark theme)

## API Endpoints

| Метод | URL | Описание |
|-------|-----|----------|
| GET | `/api/health` | Health check + размер очереди |
| GET | `/api/news?limit=20&offset=0` | Лента новостей |
| GET | `/api/news/{id}` | Новость по ID |
| GET | `/api/news/categories` | Доступные категории |
| GET | `/api/companies` | Список компаний |
| GET | `/api/companies/{ticker}` | Компания по тикеру |
| GET | `/api/heatmap?type=company&timeframe=1d` | Heat map |
| GET | `/api/alerts?limit=20&min_severity=INFO` | Алерты |
| GET | `/api/quote/{ticker}` | Текущая котировка MOEX |
| GET | `/api/quotes` | Все кэшированные котировки |
| GET | `/api/index/{index}` | Значение индекса (IMOEX) |
| GET | `/api/candles/{ticker}?interval=1h&days=30` | OHLCV свечи |
| GET | `/api/signals?ticker=SBER&source=TA` | Торговые сигналы |
| GET | `/api/portfolios` | Состояние портфелей |
| GET | `/api/portfolio-summary` | Сводка по стратегиям |
| GET | `/api/trades?strategy=combined&limit=50` | История сделок |
| GET | `/api/universe` | Мониторинг компаний + секторы |
| GET | `/api/stream/quotes` | SSE real-time котировки |
| GET | `/api/admin/sources` | Список источников новостей |
| POST | `/api/admin/reread` | Перечитать RSS/TG канал |
| GET | `/api/admin/telegram-status` | Статус Telegram авторизации |
| POST | `/api/admin/telegram-code` | Отправить код авторизации |
| POST | `/api/admin/telegram-password` | Отправить пароль 2FA |

## Серверные требования

| Ресурс | Минимум | Рекомендуется |
|--------|---------|---------------|
| CPU | 4 ядра | 8 ядер |
| RAM | 16 GB | 32 GB |
| Диск | 20 GB | 50 GB (для истории) |
| GPU | не требуется | Apple Silicon ускоряет Ollama |

### Распределение RAM (8 ядер / 32 GB)

| Компонент | RAM |
|-----------|-----|
| Ollama qwen3:8b | ~5 GB |
| Ollama qwen3:14b | ~10 GB (auto-unload) |
| PostgreSQL | ~2 GB |
| Redis | ~500 MB |
| Go сервисы (5 шт.) | ~1 GB |
| TA Engine (свечи в памяти) | ~1 GB |
| **Свободно** | **~12.5 GB** |

> Ollama автоматически выгружает неиспользуемые модели. В стандартном режиме загружена только 8b (~5 GB).

## Make команды

| Команда | Описание |
|---------|----------|
| `make build` | Собрать все бинарники |
| `make run-collector` | Запустить сборщик новостей |
| `make run-analyzer` | Анализатор (Ollama, 2 воркера) |
| `make run-analyzer-batch` | Batch-анализ (3 провайдера, 8 воркеров, round-robin) |
| `make run-api` | API + Web UI |
| `make run-alerter` | Отправка алертов |
| `make run-trader` | Торговая система |
| `make run-trader-skip` | Трейдер без бэктеста при старте |
| `make infra-up` | Поднять PostgreSQL + Redis |
| `make infra-down` | Остановить инфраструктуру |
| `make infra-reset` | Сбросить данные и пересоздать |
| `make tidy` | go mod tidy |
| `make test` | Запустить тесты |
| `make lint` | golangci-lint |

## Структура проекта

```
MarketPulse_RU/
├── cmd/
│   ├── collector/main.go      — Telegram + RSS сборщик
│   ├── analyzer/main.go       — LLM-анализатор (batch + standard)
│   ├── api/main.go            — REST API + SSE + Web UI
│   ├── alerter/main.go        — Отправка алертов
│   └── trader/main.go         — Торговая система
├── configs/
│   └── config.yaml            — Конфигурация (YAML + env vars)
├── deployments/
│   └── docker-compose.yml     — PostgreSQL + Redis
├── internal/
│   ├── domain/                — Доменные модели
│   ├── config/                — Загрузка конфига
│   ├── storage/postgres/      — PostgreSQL репозитории + миграции (embed.FS)
│   ├── storage/redis/         — Redis: очередь, dedup, pub/sub
│   ├── collector/telegram/    — MTProto userbot + history reader
│   ├── collector/rss/         — RSS фетчер (5 источников)
│   ├── collector/pipeline.go  — Конвейер: dedup → store → enqueue
│   ├── llm/ollama/            — Ollama клиент (/api/chat)
│   ├── llm/openai/            — OpenAI-совместимый клиент (429 retry)
│   ├── llm/router.go          — Multi-provider роутер (round-robin + fallback)
│   ├── llm/prompts/           — Промпты на русском
│   ├── analyzer/              — Классификатор + скорер
│   ├── alerts/                — Генерация + отправка алертов
│   ├── market/moex/           — MOEX ISS API + свечи + batch quotes
│   ├── stream/sse.go          — SSE broadcaster (real-time quotes)
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
        ├── api.js             — API + QuoteStream (SSE)
        └── pages/             — Dashboard, Stocks, News, Settings...
```

## Лицензия

MIT
