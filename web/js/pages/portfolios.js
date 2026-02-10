/* ============================================================
   MarketPulse_RU — Portfolios Page
   ============================================================ */
(function () {
    'use strict';

    var ref             = Vue.ref;
    var reactive        = Vue.reactive;
    var onMounted       = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var nextTick        = Vue.nextTick;

    var PORTFOLIO_COLORS = {
        news:     '#58a6ff',   /* blue */
        ta:       '#d29922',   /* orange */
        combined: '#3fb950'    /* green */
    };

    var PORTFOLIO_LABELS = {
        news:     'Новостной',
        ta:       'Технический',
        combined: 'Комбинированный'
    };

    /**
     * Generate synthetic equity curve data.
     * @param {number} startValue  — starting portfolio value
     * @param {number} points      — number of data points
     * @param {number} drift       — daily drift factor (positive = upward trend)
     * @param {number} volatility  — daily volatility
     * @returns {Array<{time: number, value: number}>}
     */
    function generateEquityCurve(startValue, points, drift, volatility) {
        var data = [];
        var value = startValue;
        var now = new Date();
        /* Start from `points` trading days ago */
        var startDate = new Date(now);
        startDate.setDate(startDate.getDate() - points);

        for (var i = 0; i < points; i++) {
            var date = new Date(startDate);
            date.setDate(date.getDate() + i);
            /* Skip weekends */
            var dow = date.getDay();
            if (dow === 0 || dow === 6) continue;

            var change = drift + (Math.random() - 0.5) * 2 * volatility;
            value = value * (1 + change);
            if (value < startValue * 0.5) value = startValue * 0.5; /* floor */

            data.push({
                time: Math.floor(date.getTime() / 1000),
                value: Math.round(value * 100) / 100
            });
        }
        return data;
    }

    /**
     * Generate synthetic trade history.
     */
    function generateTrades(ticker_list, count) {
        var trades = [];
        var directions = ['BUY', 'SELL'];
        var now = Date.now();
        for (var i = 0; i < count; i++) {
            var d = new Date(now - (count - i) * 86400000 * (1 + Math.random()));
            var dir = directions[Math.floor(Math.random() * 2)];
            var tick = ticker_list[Math.floor(Math.random() * ticker_list.length)];
            var price = 100 + Math.random() * 500;
            var qty = Math.floor(1 + Math.random() * 10) * 10;
            var pnl = (Math.random() - 0.45) * price * qty * 0.05;
            trades.push({
                id: i + 1,
                date: d.toISOString(),
                ticker: tick,
                direction: dir,
                price: Math.round(price * 100) / 100,
                quantity: qty,
                pnl: Math.round(pnl * 100) / 100
            });
        }
        return trades.reverse();
    }

    /**
     * Compute summary stats from equity curve.
     */
    function computeStats(curve, startValue) {
        if (!curve || curve.length === 0) return { totalPnl: 0, winRate: 0, maxDrawdown: 0, totalTrades: 0 };
        var lastVal = curve[curve.length - 1].value;
        var totalPnl = lastVal - startValue;
        var peak = startValue;
        var maxDD = 0;
        for (var i = 0; i < curve.length; i++) {
            if (curve[i].value > peak) peak = curve[i].value;
            var dd = (peak - curve[i].value) / peak;
            if (dd > maxDD) maxDD = dd;
        }
        /* Simulated win rate */
        var winRate = 0.45 + Math.random() * 0.2;
        var totalTrades = 20 + Math.floor(Math.random() * 40);
        return {
            totalPnl: Math.round(totalPnl),
            winRate: Math.round(winRate * 100),
            maxDrawdown: Math.round(maxDD * 10000) / 100,
            totalTrades: totalTrades
        };
    }

    window.PagePortfolios = {
        name: 'PagePortfolios',
        setup: function () {
            var portfolios = ref([]);
            var loading = ref(true);
            var activePortfolio = ref(null);

            /* Synthetic data */
            var equityCurves = reactive({});
            var stats = reactive({});
            var tradeHistory = reactive({});

            /* Chart */
            var chartInstance = null;
            var resizeObserver = null;

            var INITIAL = 50000;
            var TICKERS = ['SBER', 'GAZP', 'LKOH', 'YNDX', 'GMKN', 'NVTK', 'ROSN'];

            function generateAllData() {
                /* News portfolio: slight negative drift (harder to trade on news alone) */
                equityCurves.news = generateEquityCurve(INITIAL, 180, -0.0003, 0.012);
                /* TA portfolio: neutral drift */
                equityCurves.ta = generateEquityCurve(INITIAL, 180, 0.0001, 0.010);
                /* Combined: slight positive drift */
                equityCurves.combined = generateEquityCurve(INITIAL, 180, 0.0005, 0.008);

                stats.news     = computeStats(equityCurves.news, INITIAL);
                stats.ta       = computeStats(equityCurves.ta, INITIAL);
                stats.combined = computeStats(equityCurves.combined, INITIAL);

                tradeHistory.news     = generateTrades(TICKERS, 25);
                tradeHistory.ta       = generateTrades(TICKERS, 30);
                tradeHistory.combined = generateTrades(TICKERS, 35);
            }

            function createChart() {
                var container = document.getElementById('portfolio-chart');
                if (!container) return;

                if (chartInstance) {
                    chartInstance.remove();
                    chartInstance = null;
                }

                chartInstance = LightweightCharts.createChart(container, {
                    width: container.clientWidth,
                    height: 350,
                    layout: {
                        background: { type: 'solid', color: '#0d1117' },
                        textColor: '#8b949e'
                    },
                    grid: {
                        vertLines: { color: '#21262d' },
                        horzLines: { color: '#21262d' }
                    },
                    rightPriceScale: {
                        borderColor: '#30363d'
                    },
                    timeScale: {
                        borderColor: '#30363d',
                        timeVisible: false
                    },
                    crosshair: {
                        mode: LightweightCharts.CrosshairMode.Normal
                    }
                });

                /* Add line series for each portfolio */
                var types = ['news', 'ta', 'combined'];
                types.forEach(function (type) {
                    var series = chartInstance.addLineSeries({
                        color: PORTFOLIO_COLORS[type],
                        lineWidth: 2,
                        title: PORTFOLIO_LABELS[type]
                    });
                    if (equityCurves[type]) {
                        series.setData(equityCurves[type]);
                    }
                });

                /* Add baseline at starting value */
                var baseline = chartInstance.addLineSeries({
                    color: '#30363d',
                    lineWidth: 1,
                    lineStyle: LightweightCharts.LineStyle.Dashed,
                    title: 'Начальный капитал'
                });
                if (equityCurves.combined && equityCurves.combined.length >= 2) {
                    baseline.setData([
                        { time: equityCurves.combined[0].time, value: INITIAL },
                        { time: equityCurves.combined[equityCurves.combined.length - 1].time, value: INITIAL }
                    ]);
                }

                chartInstance.timeScale().fitContent();

                /* Auto-resize */
                resizeObserver = new ResizeObserver(function (entries) {
                    if (chartInstance && entries.length) {
                        chartInstance.applyOptions({ width: entries[0].contentRect.width });
                    }
                });
                resizeObserver.observe(container);
            }

            function togglePortfolio(type) {
                activePortfolio.value = activePortfolio.value === type ? null : type;
            }

            function fmtMoney(v) {
                return Number(v).toLocaleString('ru-RU', { minimumFractionDigits: 0, maximumFractionDigits: 0 });
            }

            function fmtPrice(v) {
                return Number(v).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
            }

            function pnlClass(v) {
                if (v > 0) return 'text-up';
                if (v < 0) return 'text-down';
                return 'text-flat';
            }

            function fmtDate(ts) {
                return new Date(ts).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' });
            }

            async function fetchPortfolios() {
                loading.value = true;
                try {
                    var data = await API.getPortfolios();
                    portfolios.value = (data && data.portfolios) ? data.portfolios : [];
                } catch (err) {
                    portfolios.value = [];
                }
                generateAllData();
                loading.value = false;
                nextTick(createChart);
            }

            onMounted(fetchPortfolios);

            onBeforeUnmount(function () {
                if (resizeObserver) resizeObserver.disconnect();
                if (chartInstance) { chartInstance.remove(); chartInstance = null; }
            });

            return {
                portfolios: portfolios,
                loading: loading,
                activePortfolio: activePortfolio,
                equityCurves: equityCurves,
                stats: stats,
                tradeHistory: tradeHistory,
                togglePortfolio: togglePortfolio,
                fmtMoney: fmtMoney,
                fmtPrice: fmtPrice,
                pnlClass: pnlClass,
                fmtDate: fmtDate,
                PORTFOLIO_COLORS: PORTFOLIO_COLORS,
                PORTFOLIO_LABELS: PORTFOLIO_LABELS
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <h1><i class="bi bi-briefcase me-2"></i>Портфели</h1>
        <p class="text-muted mb-0" style="font-size:0.85rem;">
            Сравнение трёх стратегий: новостная, техническая и комбинированная. Начальный капитал 50 000 RUB.
        </p>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка...
        </div>

        <div v-else>
            <!-- Equity curve chart -->
            <div class="mp-card mb-4">
                <div class="mp-card-header">
                    <h6 class="mp-card-header__title">Кривые доходности</h6>
                    <div class="d-flex gap-3" style="font-size:0.75rem;">
                        <span><span style="display:inline-block;width:12px;height:3px;background:#58a6ff;vertical-align:middle;margin-right:4px;"></span>Новостной</span>
                        <span><span style="display:inline-block;width:12px;height:3px;background:#d29922;vertical-align:middle;margin-right:4px;"></span>Технический</span>
                        <span><span style="display:inline-block;width:12px;height:3px;background:#3fb950;vertical-align:middle;margin-right:4px;"></span>Комбинированный</span>
                    </div>
                </div>
                <div id="portfolio-chart" class="mp-chart-container-full"></div>
            </div>

            <!-- Portfolio cards -->
            <div class="row g-3 mb-4">
                <div class="col-md-4" v-for="type in ['news', 'ta', 'combined']" :key="type">
                    <div class="mp-portfolio-card" :class="{ active: activePortfolio === type }" @click="togglePortfolio(type)">
                        <div class="mp-portfolio-card__header d-flex align-items-center justify-content-between">
                            <div>
                                <span class="fw-bold" :style="{ color: PORTFOLIO_COLORS[type] }">{{ PORTFOLIO_LABELS[type] }}</span>
                            </div>
                            <i class="bi" :class="activePortfolio === type ? 'bi-chevron-up' : 'bi-chevron-down'" style="font-size:0.8rem;"></i>
                        </div>
                        <div class="mp-portfolio-card__body">
                            <div class="row g-2 text-center">
                                <div class="col-6">
                                    <div class="mp-stat__value" style="font-size:1.1rem;" :class="pnlClass(stats[type] ? stats[type].totalPnl : 0)">
                                        {{ stats[type] ? (stats[type].totalPnl >= 0 ? '+' : '') + fmtMoney(stats[type].totalPnl) : '0' }}
                                        <span style="font-size:0.7rem;">RUB</span>
                                    </div>
                                    <div class="mp-stat__label">Общий P&L</div>
                                </div>
                                <div class="col-6">
                                    <div class="mp-stat__value" style="font-size:1.1rem;">
                                        {{ stats[type] ? stats[type].winRate : 0 }}%
                                    </div>
                                    <div class="mp-stat__label">Win Rate</div>
                                </div>
                                <div class="col-6">
                                    <div class="mp-stat__value text-down" style="font-size:1.1rem;">
                                        -{{ stats[type] ? stats[type].maxDrawdown : 0 }}%
                                    </div>
                                    <div class="mp-stat__label">Макс. просадка</div>
                                </div>
                                <div class="col-6">
                                    <div class="mp-stat__value" style="font-size:1.1rem;">
                                        {{ stats[type] ? stats[type].totalTrades : 0 }}
                                    </div>
                                    <div class="mp-stat__label">Сделки</div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Trade history (expanded card) -->
            <div v-if="activePortfolio" class="mp-card">
                <div class="mp-card-header">
                    <h6 class="mp-card-header__title">
                        <span :style="{ color: PORTFOLIO_COLORS[activePortfolio] }">{{ PORTFOLIO_LABELS[activePortfolio] }}</span> — История сделок
                    </h6>
                </div>
                <div class="mp-card-body p-0">
                    <div class="table-responsive">
                        <table class="mp-table">
                            <thead>
                                <tr>
                                    <th>#</th>
                                    <th>Дата</th>
                                    <th>Тикер</th>
                                    <th>Направление</th>
                                    <th class="text-end">Цена</th>
                                    <th class="text-end">Кол-во</th>
                                    <th class="text-end">P&L</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="t in tradeHistory[activePortfolio]" :key="t.id">
                                    <td class="text-muted">{{ t.id }}</td>
                                    <td>{{ fmtDate(t.date) }}</td>
                                    <td class="fw-bold text-accent">{{ t.ticker }}</td>
                                    <td>
                                        <span class="badge" :class="t.direction==='BUY'?'badge-buy':'badge-sell'">{{ t.direction === 'BUY' ? 'ПОКУПКА' : 'ПРОДАЖА' }}</span>
                                    </td>
                                    <td class="text-end font-monospace">{{ fmtPrice(t.price) }}</td>
                                    <td class="text-end">{{ t.quantity }}</td>
                                    <td class="text-end font-monospace" :class="pnlClass(t.pnl)">
                                        {{ t.pnl >= 0 ? '+' : '' }}{{ fmtPrice(t.pnl) }}
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
`
    };
})();
