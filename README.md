# MarketPulse_RU

Аналитическая платформа для российского финансового рынка с автоматической торговлей через Т-Инвестиции. Собирает новости из Telegram-каналов и RSS, анализирует через LLM, генерирует торговые сигналы, исполняет ордера через T-Invest API.

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
│  80% fast LLM classify   │
│  20% heavy deep analysis │     ┌──────────────┐
│  → trading signals в DB  │────▶│  PostgreSQL   │
└──────────────────────────┘     └──────┬───────┘
                                        │
┌──────────────────────────┐            │
│      TRADER              │◀───────────┘
│  reads signals from DB   │     ┌──────────────┐
│  3 modes: news/ta/combo  │────▶│  T-Invest    │
│  15 TA indicators        │     │  (gRPC API)  │
│  risk mgmt + positions   │     │  primary     │
└──────────────────────────┘     └──────┬───────┘
                                        │
┌──────────────────────────┐     ┌──────────────┐
│      API + WEB UI        │────▶│  MOEX ISS    │
│  REST API + Vue.js SPA   │     │  (fallback)  │
│  SSE real-time quotes    │     └──────────────┘
│  TradingView charts      │
│  T-Invest admin panel    │
└──────────────────────────┘
```

> **T-Invest API** — основной источник данных и исполнения ордеров. **MOEX ISS API** — бесплатный fallback для котировок и свечей когда T-Invest не подключен.

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

## Бизнес-логика: полный пайплайн

### 1. Сбор новостей (Collector)

```
Telegram MTProto  ──┐
RSS (5 фидов)     ──┤──▶ dedup (Redis set) ──▶ PostgreSQL ──▶ Redis queue (BRPOP)
                    │
```

- Телеграм-каналы подключаются через MTProto userbot (история + real-time)
- Дедупликация по хешу текста (Redis `news_dedup` set, TTL 48h)
- ID новости ставится в очередь Redis `news:queue` для обработки

### 2. Анализ новостей (Analyzer)

```
Redis queue ──▶ Worker Pool (N воркеров) ──▶ LLM ──▶ PostgreSQL
```

**Этап 1: Быстрая классификация (80% запросов → Ollama qwen3:8b)**
- Категория (20 типов: корпоративные, макро, ЦБ, геополитика, ...)
- Sentiment (-1.0 ... +1.0)
- Urgency (1-5)
- Reliability (0-1)
- Тикеры (SBER, GAZP, ...)
- Ключевые факты + summary

**Этап 2: Глубокий анализ (20% запросов → тяжёлый LLM)**
- Критерий: urgency >= 4 ИЛИ |sentiment| >= 0.7
- Детальная оценка влияния на каждый тикер: direction, magnitude, timeframe, confidence
- Провайдеры: Ollama qwen3:14b → Qwen-Plus API → DeepSeek API (fallback)

**Этап 3: Торговые сигналы**
- Создаются для новостей с urgency >= 3 и |sentiment| >= 0.4
- Direction: BUY (sentiment > 0.1), SELL (sentiment < -0.1)
- Strength = 0.3×(urgency/5) + 0.4×|sentiment| + 0.3×reliability
- Сигналы записываются в таблицу `trading_signals` (поле `executed=false`)

### 3. Heat scoring

- Каждый impact обновляет `heat_scores` — скользящий показатель "горячести" тикера
- Используется при формировании портфеля и на heatmap UI

### 4. Торговый движок (Trader)

```
DB Signals ──▶ Engine tick loop ──▶ Risk Check ──▶ Execute ──▶ Position
```

**Цикл (каждые N минут):**
1. **SL/TP**: проверка стоп-лосс/тейк-профит по открытым позициям
2. **News signals**: чтение из БД (таблица `trading_signals` WHERE executed=false)
3. **TA signals**: генерация по индикаторам (в режиме ta/combined)
4. **Risk check**: размер позиции, exposure, сектор, дневной лимит, drawdown
5. **Execute**: размещение ордера (dry-run или T-Invest broker)
6. **Persist**: позиция сохраняется в PostgreSQL, восстанавливается при рестарте

**Формирование портфеля:**
- Если тикеры не указаны в конфиге — динамический отбор
- Selector ранжирует universe по сигнальной силе → top-N в портфель

### 5. Исполнение ордеров

| Режим | Описание |
|-------|----------|
| **Dry-run** | Логируется без реального исполнения |
| **Broker** | T-Invest PostOrder → рыночный/лимитный ордер |

- BrokerExecutor проверяет маржу, конвертирует лоты, отправляет через T-Invest gRPC
- Мульти-аккаунт: разные стратегии (news/ta/combined) на разные счета

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

## Т-Инвестиции (T-Invest API)

Интеграция с брокером Т-Инвестиции через официальный gRPC SDK (`invest-api-go-sdk v1.40.1`).

### Возможности

- **Динамическое подключение**: токен вводится через Web UI, хранится в Redis
- **Мульти-аккаунт**: привязка стратегий (news/ta/combined) к разным счетам
- **Реальная торговля**: исполнение сигналов через рыночные ордера
- **gRPC streaming**: real-time цены через подписку → SSE на фронт
- **Портфель брокера**: периодическая синхронизация (30с) → кэш в Redis
- **Маржинальность**: проверка доступных средств перед выставлением ордера
- **SL/TP по live-ценам**: проверка стоп-лосс/тейк-профит через T-Invest API
- **Вывод прибыли**: настраиваемый % от чистой прибыли, по расписанию

### Настройка

1. Получить токен на [T-Invest](https://www.tbank.ru/invest/)
2. Открыть Web UI → страница **Т-Инвест**
3. Ввести токен, выбрать режим (production/sandbox)
4. Назначить стратегии на счета
5. Стриминг цен запускается автоматически

> Токен хранится только в Redis, не в конфиг-файлах. При перезапуске API сервер автоматически восстанавливает подключение из Redis.

## Web UI

Dashboard: `http://localhost:8080` после `make run-api`.

- **Дашборд**: виджеты, сигналы, heatmap, индекс IMOEX
- **Акции**: список с real-time котировками (SSE), детальная страница с TradingView-графиком
- **Новости**: лента с LLM-анализом, авто-обновление 30с
- **Портфели**: 3 портфеля с бэктестированием
- **Настройки**: Telegram авторизация, управление источниками (re-read RSS/TG каналов)
- **Т-Инвестиции**: подключение токена, счета, стратегии, портфель брокера, ордера, инструменты, вывод прибыли

### Real-time обновления

- **Котировки MOEX**: SSE через `/api/stream/quotes`, обновление каждые 10с
- **Котировки T-Invest**: gRPC streaming → SSE через `/api/stream/tinvest`, real-time цены
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
| | | |
| | **T-Invest** | |
| GET | `/api/admin/tinvest/status` | Статус подключения + стратегии |
| POST | `/api/admin/tinvest/connect` | Подключить токен |
| POST | `/api/admin/tinvest/disconnect` | Отключить |
| GET | `/api/admin/tinvest/accounts` | Список счетов |
| GET | `/api/admin/tinvest/strategies` | Привязки стратегий |
| POST | `/api/admin/tinvest/strategies` | Сохранить привязки |
| POST | `/api/admin/tinvest/stream/start` | Запустить gRPC стриминг |
| POST | `/api/admin/tinvest/stream/stop` | Остановить стриминг |
| GET | `/api/admin/tinvest/withdrawal` | Конфиг вывода прибыли |
| POST | `/api/admin/tinvest/withdrawal` | Сохранить конфиг вывода |
| GET | `/api/stream/tinvest` | SSE real-time цены T-Invest |
| GET | `/api/tinvest/instruments` | Список акций (кэш 5мин) |
| GET | `/api/tinvest/instrument/{ticker}` | Инструмент по тикеру |
| GET | `/api/tinvest/prices` | Последние цены |
| GET | `/api/tinvest/orderbook/{id}` | Стакан |
| GET | `/api/tinvest/candles/{id}` | Свечи |
| GET | `/api/tinvest/portfolio/{accountId}` | Портфель счёта |
| GET | `/api/tinvest/margin/{accountId}` | Маржинальные показатели |
| GET | `/api/tinvest/orders/{accountId}` | Активные ордера |
| POST | `/api/tinvest/orders` | Выставить ордер |
| POST | `/api/tinvest/orders/cancel` | Отменить ордер |
| GET | `/api/tinvest/broker-portfolios` | Кэшированные портфели брокера |

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
│   ├── market/tinvest/        — T-Invest gRPC клиент + streaming
│   ├── stream/sse.go          — SSE broadcaster (real-time quotes)
│   └── trading/
│       ├── engine/            — Торговый движок
│       ├── executor/          — Исполнение ордеров (dry-run + broker)
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
        └── pages/             — Dashboard, Stocks, News, Settings, TInvest...
```

## Лицензия

MIT
