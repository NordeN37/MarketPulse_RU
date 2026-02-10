/* ============================================================
   MarketPulse_RU — Stock Detail Page (TradingView chart)
   ============================================================ */
(function () {
    'use strict';

    var ref             = Vue.ref;
    var reactive        = Vue.reactive;
    var onMounted       = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var nextTick        = Vue.nextTick;
    var watch           = Vue.watch;

    var INTERVALS = [
        { value: '1h',  label: '1 час' },
        { value: '1d',  label: '1 день' },
        { value: '1w',  label: '1 неделя' },
        { value: '1M',  label: '1 месяц' }
    ];

    var DAYS_OPTIONS = [
        { value: 7,   label: '7д' },
        { value: 30,  label: '30д' },
        { value: 90,  label: '90д' },
        { value: 180, label: '180д' },
        { value: 365, label: '1г' }
    ];

    function fmtTime(ts) {
        if (!ts) return '';
        return new Date(ts).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' });
    }

    function fmtChange(val) {
        if (val === undefined || val === null) return '';
        var n = Number(val);
        if (isNaN(n)) return '';
        var sign = n >= 0 ? '+' : '';
        return sign + n.toFixed(2) + '%';
    }

    function changeClass(val) {
        var n = Number(val);
        if (isNaN(n) || n === 0) return 'text-flat';
        return n > 0 ? 'text-up' : 'text-down';
    }

    window.PageStockDetail = {
        name: 'PageStockDetail',
        setup: function () {
            var route  = VueRouter.useRoute();
            var router = VueRouter.useRouter();

            var ticker    = ref(route.params.ticker || '');
            var company   = ref(null);
            var quote     = ref(null);
            var loading   = ref(true);
            var chartLoading = ref(false);
            var interval  = ref('1d');
            var days      = ref(90);
            var activeTab = ref('news');

            /* Sub-tab data */
            var newsItems  = ref([]);
            var signals    = ref([]);
            var newsLoading = ref(false);
            var signalsLoading = ref(false);

            /* Chart refs */
            var chartInstance = null;
            var candleSeries  = null;
            var resizeObserver = null;

            async function fetchCompany() {
                loading.value = true;
                try {
                    var data = await API.getCompany(ticker.value);
                    company.value = data;
                } catch (err) {
                    company.value = null;
                }
            }

            async function fetchQuote() {
                try {
                    var data = await API.getQuote(ticker.value);
                    quote.value = data;
                } catch (err) {
                    quote.value = null;
                }
            }

            async function fetchNews() {
                newsLoading.value = true;
                try {
                    var data = await API.getNews(20, 0);
                    /* Filter by ticker mention in title or content */
                    var t = ticker.value.toUpperCase();
                    newsItems.value = (Array.isArray(data) ? data : []).filter(function (n) {
                        var text = ((n.title || '') + ' ' + (n.content || '')).toUpperCase();
                        return text.indexOf(t) >= 0;
                    });
                } catch (err) {
                    newsItems.value = [];
                }
                newsLoading.value = false;
            }

            async function fetchSignals() {
                signalsLoading.value = true;
                try {
                    var data = await API.getSignals();
                    var all = (data && data.signals) ? data.signals : [];
                    signals.value = all.filter(function (s) { return s.ticker === ticker.value; });
                } catch (err) {
                    signals.value = [];
                }
                signalsLoading.value = false;
            }

            function createChart() {
                var container = document.getElementById('stock-chart');
                if (!container) return;

                if (chartInstance) {
                    chartInstance.remove();
                    chartInstance = null;
                }

                chartInstance = LightweightCharts.createChart(container, {
                    width: container.clientWidth,
                    height: 450,
                    layout: {
                        background: { type: 'solid', color: '#0d1117' },
                        textColor: '#8b949e'
                    },
                    grid: {
                        vertLines: { color: '#21262d' },
                        horzLines: { color: '#21262d' }
                    },
                    crosshair: {
                        mode: LightweightCharts.CrosshairMode.Normal
                    },
                    rightPriceScale: {
                        borderColor: '#30363d'
                    },
                    timeScale: {
                        borderColor: '#30363d',
                        timeVisible: true,
                        secondsVisible: false
                    }
                });

                candleSeries = chartInstance.addCandlestickSeries({
                    upColor: '#3fb950',
                    downColor: '#f85149',
                    borderDownColor: '#f85149',
                    borderUpColor: '#3fb950',
                    wickDownColor: '#f85149',
                    wickUpColor: '#3fb950'
                });

                /* Auto-resize */
                resizeObserver = new ResizeObserver(function (entries) {
                    if (chartInstance && entries.length) {
                        var cr = entries[0].contentRect;
                        chartInstance.applyOptions({ width: cr.width });
                    }
                });
                resizeObserver.observe(container);
            }

            async function loadCandles() {
                if (!candleSeries) return;
                chartLoading.value = true;
                try {
                    var rawData = await API.getCandles(ticker.value, interval.value, days.value);
                    var candles = Array.isArray(rawData) ? rawData : [];

                    var chartData = candles.map(function (c) {
                        /* domain.Candle JSON: open_time (unix sec), open, high, low, close, volume */
                        var t;
                        if (c.open_time) {
                            t = typeof c.open_time === 'number' ? c.open_time : Math.floor(new Date(c.open_time).getTime() / 1000);
                        } else if (c.begin) {
                            t = Math.floor(new Date(c.begin).getTime() / 1000);
                        } else if (c.time) {
                            t = typeof c.time === 'number' ? c.time : Math.floor(new Date(c.time).getTime() / 1000);
                        } else {
                            t = 0;
                        }
                        return {
                            time: t,
                            open: Number(c.open),
                            high: Number(c.high),
                            close: Number(c.close),
                            low: Number(c.low)
                        };
                    }).filter(function (c) {
                        return c.time > 0 && !isNaN(c.open) && !isNaN(c.high) && !isNaN(c.close) && !isNaN(c.low);
                    }).sort(function (a, b) { return a.time - b.time; });

                    /* Remove duplicates by time */
                    var seen = {};
                    chartData = chartData.filter(function (c) {
                        if (seen[c.time]) return false;
                        seen[c.time] = true;
                        return true;
                    });

                    candleSeries.setData(chartData);

                    /* Add signal markers */
                    if (signals.value.length > 0) {
                        var markers = signals.value.map(function (s) {
                            return {
                                time: Math.floor(new Date(s.created_at).getTime() / 1000),
                                position: s.direction === 'BUY' ? 'belowBar' : 'aboveBar',
                                color: s.direction === 'BUY' ? '#3fb950' : '#f85149',
                                shape: s.direction === 'BUY' ? 'arrowUp' : 'arrowDown',
                                text: s.direction
                            };
                        }).sort(function (a, b) { return a.time - b.time; });
                        candleSeries.setMarkers(markers);
                    }

                    chartInstance.timeScale().fitContent();
                } catch (err) {
                    candleSeries.setData([]);
                }
                chartLoading.value = false;
            }

            function getPrice() {
                if (!quote.value) return '...';
                var p = quote.value.last || quote.value.LAST || quote.value.price;
                if (!p) return '...';
                return Number(p).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
            }

            function getChange() {
                if (!quote.value) return '';
                return quote.value.change || quote.value.CHANGE || quote.value.lasttoprevprice || null;
            }

            /* Watchers */
            watch([interval, days], function () { loadCandles(); });
            watch(function () { return route.params.ticker; }, function (newTicker) {
                if (newTicker && newTicker !== ticker.value) {
                    ticker.value = newTicker;
                    init();
                }
            });

            async function init() {
                loading.value = true;
                await Promise.all([fetchCompany(), fetchQuote(), fetchSignals()]);
                loading.value = false;
                fetchNews();
                nextTick(function () {
                    createChart();
                    loadCandles();
                });
            }

            onMounted(init);

            onBeforeUnmount(function () {
                if (resizeObserver) resizeObserver.disconnect();
                if (chartInstance) { chartInstance.remove(); chartInstance = null; }
            });

            return {
                ticker: ticker,
                company: company,
                quote: quote,
                loading: loading,
                chartLoading: chartLoading,
                interval: interval,
                days: days,
                activeTab: activeTab,
                newsItems: newsItems,
                signals: signals,
                newsLoading: newsLoading,
                signalsLoading: signalsLoading,
                INTERVALS: INTERVALS,
                DAYS_OPTIONS: DAYS_OPTIONS,
                getPrice: getPrice,
                getChange: getChange,
                fmtChange: fmtChange,
                changeClass: changeClass,
                fmtTime: fmtTime,
                loadCandles: loadCandles
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <div class="d-flex align-items-center gap-2 mb-2">
            <router-link to="/stocks" class="btn btn-sm btn-outline-secondary">
                <i class="bi bi-arrow-left"></i>
            </router-link>
            <h1 class="mb-0">
                <span class="text-accent">{{ ticker }}</span>
                <span v-if="company" class="text-muted ms-2" style="font-size:1rem;font-weight:400;">{{ company.name }}</span>
            </h1>
        </div>
        <div class="d-flex align-items-center gap-3 flex-wrap" v-if="!loading">
            <div>
                <span class="fs-4 fw-bold" :class="changeClass(getChange())">{{ getPrice() }}</span>
                <span class="ms-2" :class="changeClass(getChange())" style="font-size:0.9rem;">{{ fmtChange(getChange()) }}</span>
                <span class="text-muted ms-1" style="font-size:0.75rem;">RUB</span>
            </div>
            <div v-if="company && company.sector" class="text-muted" style="font-size:0.8rem;">
                <i class="bi bi-building me-1"></i>{{ company.sector }}
            </div>
        </div>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка...
        </div>

        <div v-else>
            <!-- Chart controls -->
            <div class="d-flex align-items-center gap-2 mb-3 flex-wrap">
                <div class="btn-group btn-group-sm">
                    <button v-for="iv in INTERVALS" :key="iv.value"
                            class="btn" :class="interval===iv.value ? 'btn-primary' : 'btn-outline-secondary'"
                            @click="interval = iv.value">
                        {{ iv.label }}
                    </button>
                </div>
                <div class="btn-group btn-group-sm">
                    <button v-for="d in DAYS_OPTIONS" :key="d.value"
                            class="btn" :class="days===d.value ? 'btn-primary' : 'btn-outline-secondary'"
                            @click="days = d.value">
                        {{ d.label }}
                    </button>
                </div>
                <div v-if="chartLoading" class="ms-2">
                    <div class="spinner-border spinner-border-sm text-accent"></div>
                </div>
            </div>

            <!-- TradingView Chart -->
            <div class="mp-card mb-4">
                <div id="stock-chart" class="mp-chart-container-full"></div>
            </div>

            <!-- Tabs: News / Signals / Analysis -->
            <ul class="nav nav-tabs mb-3">
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'news' }" href="#" @click.prevent="activeTab='news'">
                        <i class="bi bi-newspaper me-1"></i>Новости
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'signals' }" href="#" @click.prevent="activeTab='signals'">
                        <i class="bi bi-lightning-charge me-1"></i>Сигналы
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'analysis' }" href="#" @click.prevent="activeTab='analysis'">
                        <i class="bi bi-file-earmark-text me-1"></i>Анализ
                    </a>
                </li>
            </ul>

            <!-- News Tab -->
            <div v-if="activeTab === 'news'">
                <div v-if="newsLoading" class="mp-loading">
                    <div class="spinner-border spinner-border-sm text-accent me-2"></div>
                    Загрузка новостей...
                </div>
                <div v-else-if="newsItems.length === 0" class="mp-empty">
                    <i class="bi bi-newspaper"></i>
                    <div>Нет новостей для {{ ticker }}</div>
                    <small class="text-muted">Новости будут появляться по мере сбора из Telegram и RSS</small>
                </div>
                <div v-else>
                    <div v-for="n in newsItems" :key="n.id" class="mp-news-card">
                        <div class="mp-news-card__title">{{ n.title || 'Без заголовка' }}</div>
                        <div class="mp-news-card__meta">
                            <span class="badge badge-news me-1" style="font-size:0.65rem;">{{ n.source }}</span>
                            {{ fmtTime(n.published_at || n.collected_at) }}
                            <span v-if="n.source_channel" class="ms-1 text-muted">| {{ n.source_channel }}</span>
                        </div>
                        <div class="mp-news-card__content">{{ (n.content || '').substring(0, 200) }}{{ (n.content || '').length > 200 ? '...' : '' }}</div>
                    </div>
                </div>
            </div>

            <!-- Signals Tab -->
            <div v-if="activeTab === 'signals'">
                <div v-if="signalsLoading" class="mp-loading">
                    <div class="spinner-border spinner-border-sm text-accent me-2"></div>
                    Загрузка сигналов...
                </div>
                <div v-else-if="signals.length === 0" class="mp-empty">
                    <i class="bi bi-lightning-charge"></i>
                    <div>Нет сигналов для {{ ticker }}</div>
                    <small class="text-muted">Сигналы появятся при запуске торгового движка</small>
                </div>
                <div v-else class="table-responsive">
                    <table class="mp-table">
                        <thead>
                            <tr>
                                <th>Время</th>
                                <th>Направление</th>
                                <th>Источник</th>
                                <th>Сила</th>
                                <th>Причина</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="s in signals" :key="s.id">
                                <td>{{ fmtTime(s.created_at) }}</td>
                                <td><span class="badge" :class="s.direction==='BUY'?'badge-buy':'badge-sell'">{{ s.direction }}</span></td>
                                <td><span class="badge" :class="'badge-'+(s.source||'').toLowerCase()">{{ s.source }}</span></td>
                                <td>
                                    <div class="d-flex align-items-center gap-2">
                                        <div class="mp-strength-bar" style="width:60px;">
                                            <div class="mp-strength-bar__fill" :style="{ width: (s.strength*100)+'%', backgroundColor: s.direction==='BUY'?'var(--mp-green)':'var(--mp-red)' }"></div>
                                        </div>
                                        <small class="text-muted">{{ (s.strength * 100).toFixed(0) }}%</small>
                                    </div>
                                </td>
                                <td style="font-size:0.8rem;">{{ s.reason }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Analysis Tab -->
            <div v-if="activeTab === 'analysis'">
                <div class="mp-empty">
                    <i class="bi bi-file-earmark-text"></i>
                    <div>Аналитические отчёты</div>
                    <small class="text-muted">Отчёты LLM-анализа будут доступны после обработки новостей</small>
                </div>
            </div>
        </div>
    </div>
</div>
`
    };
})();
