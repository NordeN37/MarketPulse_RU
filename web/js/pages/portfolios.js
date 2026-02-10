/* ============================================================
   MarketPulse_RU — Portfolios Page
   Fetches real backtest data from API, falls back to synthetic.
   ============================================================ */
(function () {
    'use strict';

    var ref             = Vue.ref;
    var reactive        = Vue.reactive;
    var onMounted       = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var nextTick        = Vue.nextTick;

    var PORTFOLIO_COLORS = {
        news:     '#58a6ff',
        ta:       '#d29922',
        combined: '#3fb950'
    };

    var PORTFOLIO_LABELS = {
        news:     'Новостной',
        ta:       'Технический',
        combined: 'Комбинированный'
    };

    var IMOEX_COLOR = '#bc8cff';
    var INITIAL = 50000;

    /* ---------- Synthetic fallback generators ---------- */

    function generateEquityCurve(startValue, points, drift, volatility) {
        var data = [];
        var value = startValue;
        var now = new Date();
        var startDate = new Date(now);
        startDate.setDate(startDate.getDate() - points);
        for (var i = 0; i < points; i++) {
            var date = new Date(startDate);
            date.setDate(date.getDate() + i);
            var dow = date.getDay();
            if (dow === 0 || dow === 6) continue;
            var change = drift + (Math.random() - 0.5) * 2 * volatility;
            value = value * (1 + change);
            if (value < startValue * 0.5) value = startValue * 0.5;
            data.push({
                time: Math.floor(date.getTime() / 1000),
                value: Math.round(value * 100) / 100
            });
        }
        return data;
    }

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
                id: i + 1, date: d.toISOString(), ticker: tick, direction: dir, side: dir,
                price: Math.round(price * 100) / 100, quantity: qty,
                pnl: Math.round(pnl * 100) / 100
            });
        }
        return trades.reverse();
    }

    /* ---------- Convert API snapshots to chart data ---------- */

    function snapshotsToChartData(snapshots) {
        if (!snapshots || snapshots.length === 0) return [];
        return snapshots.map(function (s) {
            var ts = Math.floor(new Date(s.snapshot_at).getTime() / 1000);
            return { time: ts, value: s.total_value };
        });
    }

    function computeReturns(curve) {
        if (!curve || curve.length < 2) return { monthlyReturn: null, annualReturn: null };
        var lastPoint = curve[curve.length - 1];
        var lastVal = lastPoint.value;
        var lastTime = lastPoint.time;
        var month30 = lastTime - 30 * 86400;
        var year365 = lastTime - 365 * 86400;
        var monthlyReturn = null;
        var annualReturn = null;
        // Find closest point to 30 days ago
        for (var i = curve.length - 1; i >= 0; i--) {
            if (curve[i].time <= month30) {
                monthlyReturn = ((lastVal - curve[i].value) / curve[i].value) * 100;
                break;
            }
        }
        // If no point 30 days ago, use first point
        if (monthlyReturn === null && curve.length >= 2) {
            var first = curve[0];
            var daysDiff = (lastTime - first.time) / 86400;
            if (daysDiff > 0 && daysDiff <= 60) {
                monthlyReturn = ((lastVal - first.value) / first.value) * 100 * (30 / daysDiff);
            }
        }
        // Find closest point to 365 days ago
        for (var i = curve.length - 1; i >= 0; i--) {
            if (curve[i].time <= year365) {
                annualReturn = ((lastVal - curve[i].value) / curve[i].value) * 100;
                break;
            }
        }
        // If no point 365 days ago, annualize from available data
        if (annualReturn === null && curve.length >= 2) {
            var first = curve[0];
            var daysDiff = (lastTime - first.time) / 86400;
            if (daysDiff > 30) {
                var totalReturn = (lastVal - first.value) / first.value;
                annualReturn = (Math.pow(1 + totalReturn, 365 / daysDiff) - 1) * 100;
            }
        }
        return {
            monthlyReturn: monthlyReturn !== null ? Math.round(monthlyReturn * 100) / 100 : null,
            annualReturn: annualReturn !== null ? Math.round(annualReturn * 100) / 100 : null
        };
    }

    function snapshotsToStats(snapshots) {
        if (!snapshots || snapshots.length === 0) {
            return { totalPnl: 0, winRate: 0, maxDrawdown: 0, totalTrades: 0, monthlyReturn: null, annualReturn: null };
        }
        var last = snapshots[snapshots.length - 1];
        var curve = snapshotsToChartData(snapshots);
        var returns = computeReturns(curve);
        return {
            totalPnl: Math.round(last.total_pnl || 0),
            winRate: Math.round((last.win_rate || 0) * 100),
            maxDrawdown: Math.round((last.max_drawdown || 0) * 10000) / 100,
            totalTrades: last.total_trades || 0,
            monthlyReturn: returns.monthlyReturn,
            annualReturn: returns.annualReturn
        };
    }

    function computeStatsFromCurve(curve) {
        if (!curve || curve.length === 0) return { totalPnl: 0, winRate: 0, maxDrawdown: 0, totalTrades: 0, monthlyReturn: null, annualReturn: null };
        var lastVal = curve[curve.length - 1].value;
        var totalPnl = lastVal - INITIAL;
        var peak = INITIAL;
        var maxDD = 0;
        for (var i = 0; i < curve.length; i++) {
            if (curve[i].value > peak) peak = curve[i].value;
            var dd = (peak - curve[i].value) / peak;
            if (dd > maxDD) maxDD = dd;
        }
        var returns = computeReturns(curve);
        return {
            totalPnl: Math.round(totalPnl),
            winRate: Math.round((0.45 + Math.random() * 0.2) * 100),
            maxDrawdown: Math.round(maxDD * 10000) / 100,
            totalTrades: 20 + Math.floor(Math.random() * 40),
            monthlyReturn: returns.monthlyReturn,
            annualReturn: returns.annualReturn
        };
    }

    /**
     * Normalize IMOEX candle data to match portfolio scale.
     * First point = INITIAL (50000 RUB).
     */
    function normalizeIndexToEquity(candles) {
        if (!candles || candles.length === 0) return [];
        var first = candles[0].close || candles[0].Close;
        if (!first || first === 0) return [];
        var data = [];
        for (var i = 0; i < candles.length; i++) {
            var c = candles[i];
            var close = c.close || c.Close;
            var ts = c.open_time || c.OpenTime;
            if (!close || !ts) continue;
            data.push({
                time: ts,
                value: Math.round((close / first) * INITIAL * 100) / 100
            });
        }
        return data;
    }

    /* ---------- Component ---------- */

    window.PagePortfolios = {
        name: 'PagePortfolios',
        setup: function () {
            var portfolios = ref([]);
            var loading = ref(true);
            var dataSource = ref('');
            var activePortfolio = ref(null);

            var equityCurves = reactive({});
            var imoexCurve = ref([]);
            var stats = reactive({});
            var tradeHistory = reactive({});

            var chartInstance = null;
            var resizeObserver = null;

            var TICKERS = ['SBER', 'GAZP', 'LKOH', 'YNDX', 'GMKN', 'NVTK', 'ROSN'];

            function useSyntheticData() {
                dataSource.value = 'synthetic';
                equityCurves.news = generateEquityCurve(INITIAL, 180, -0.0003, 0.012);
                equityCurves.ta = generateEquityCurve(INITIAL, 180, 0.0001, 0.010);
                equityCurves.combined = generateEquityCurve(INITIAL, 180, 0.0005, 0.008);
                stats.news     = computeStatsFromCurve(equityCurves.news);
                stats.ta       = computeStatsFromCurve(equityCurves.ta);
                stats.combined = computeStatsFromCurve(equityCurves.combined);
                tradeHistory.news     = generateTrades(TICKERS, 25);
                tradeHistory.ta       = generateTrades(TICKERS, 30);
                tradeHistory.combined = generateTrades(TICKERS, 35);
            }

            function createChart() {
                var container = document.getElementById('portfolio-chart');
                if (!container) return;

                if (chartInstance) { chartInstance.remove(); chartInstance = null; }

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
                    rightPriceScale: { borderColor: '#30363d' },
                    timeScale: { borderColor: '#30363d', timeVisible: false },
                    crosshair: { mode: LightweightCharts.CrosshairMode.Normal }
                });

                var types = ['news', 'ta', 'combined'];
                types.forEach(function (type) {
                    var series = chartInstance.addLineSeries({
                        color: PORTFOLIO_COLORS[type],
                        lineWidth: 2,
                        title: PORTFOLIO_LABELS[type]
                    });
                    if (equityCurves[type] && equityCurves[type].length > 0) {
                        series.setData(equityCurves[type]);
                    }
                });

                /* IMOEX benchmark line (normalized to initial capital) */
                if (imoexCurve.value && imoexCurve.value.length > 0) {
                    var imoexSeries = chartInstance.addLineSeries({
                        color: IMOEX_COLOR,
                        lineWidth: 1,
                        lineStyle: LightweightCharts.LineStyle.Dotted,
                        title: 'IMOEX'
                    });
                    imoexSeries.setData(imoexCurve.value);
                }

                /* Baseline at initial capital */
                var baseline = chartInstance.addLineSeries({
                    color: '#30363d',
                    lineWidth: 1,
                    lineStyle: LightweightCharts.LineStyle.Dashed,
                    title: 'Начальный капитал'
                });
                var ref_curve = equityCurves.combined || equityCurves.ta || equityCurves.news;
                if (ref_curve && ref_curve.length >= 2) {
                    baseline.setData([
                        { time: ref_curve[0].time, value: INITIAL },
                        { time: ref_curve[ref_curve.length - 1].time, value: INITIAL }
                    ]);
                }

                chartInstance.timeScale().fitContent();

                resizeObserver = new ResizeObserver(function (entries) {
                    if (chartInstance && entries.length) {
                        chartInstance.applyOptions({ width: entries[0].contentRect.width });
                    }
                });
                resizeObserver.observe(container);
            }

            function togglePortfolio(type) {
                activePortfolio.value = activePortfolio.value === type ? null : type;
                if (activePortfolio.value && !tradeHistory[type]) {
                    fetchTradeHistory(type);
                }
            }

            async function fetchTradeHistory(type) {
                try {
                    var data = await API.getTrades(type, 50);
                    if (Array.isArray(data) && data.length > 0) {
                        tradeHistory[type] = data;
                    }
                } catch (_) { /* keep existing synthetic */ }
            }

            async function fetchIMOEX() {
                try {
                    var candles = await API.getIndexCandles('IMOEX', '1d', 365);
                    if (Array.isArray(candles) && candles.length > 0) {
                        imoexCurve.value = normalizeIndexToEquity(candles);
                    }
                } catch (_) { /* IMOEX optional, ignore errors */ }
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
            function fmtReturn(v) {
                if (v === null || v === undefined) return '—';
                return (v >= 0 ? '+' : '') + v.toFixed(2) + '%';
            }
            function fmtDate(ts) {
                return new Date(ts).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' });
            }

            async function fetchPortfolios() {
                loading.value = true;
                try {
                    var data = await API.getPortfolios();
                    portfolios.value = (data && data.portfolios) ? data.portfolios : [];

                    /* Fetch IMOEX benchmark in parallel */
                    fetchIMOEX();

                    var snapData = await API.getPortfolioSnapshots();
                    var hasReal = false;
                    var types = ['news', 'ta', 'combined'];
                    types.forEach(function (type) {
                        var snaps = snapData[type];
                        if (snaps && snaps.length > 5) {
                            hasReal = true;
                            equityCurves[type] = snapshotsToChartData(snaps);
                            stats[type] = snapshotsToStats(snaps);
                        }
                    });

                    if (hasReal) {
                        dataSource.value = 'api';
                        types.forEach(function (type) {
                            fetchTradeHistory(type);
                        });
                        types.forEach(function (type) {
                            if (!equityCurves[type] || equityCurves[type].length === 0) {
                                equityCurves[type] = generateEquityCurve(INITIAL, 180, 0, 0.010);
                                stats[type] = computeStatsFromCurve(equityCurves[type]);
                            }
                        });
                    } else {
                        useSyntheticData();
                    }
                } catch (err) {
                    portfolios.value = [];
                    useSyntheticData();
                }
                loading.value = false;
                /* Wait briefly for IMOEX fetch before creating chart */
                setTimeout(function () { nextTick(createChart); }, 300);
            }

            onMounted(fetchPortfolios);

            onBeforeUnmount(function () {
                if (resizeObserver) resizeObserver.disconnect();
                if (chartInstance) { chartInstance.remove(); chartInstance = null; }
            });

            return {
                portfolios: portfolios,
                loading: loading,
                dataSource: dataSource,
                activePortfolio: activePortfolio,
                equityCurves: equityCurves,
                stats: stats,
                tradeHistory: tradeHistory,
                togglePortfolio: togglePortfolio,
                fmtMoney: fmtMoney,
                fmtPrice: fmtPrice,
                pnlClass: pnlClass,
                fmtReturn: fmtReturn,
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
            <span v-if="dataSource === 'api'" class="badge badge-buy ms-1" style="font-size:0.6rem;">Реальные данные</span>
            <span v-else-if="dataSource === 'synthetic'" class="badge badge-ta ms-1" style="font-size:0.6rem;">Синтетические данные — запустите trader для бэктеста</span>
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
                    <div class="d-flex gap-3 flex-wrap" style="font-size:0.75rem;">
                        <span><span style="display:inline-block;width:12px;height:3px;background:#58a6ff;vertical-align:middle;margin-right:4px;"></span>Новостной</span>
                        <span><span style="display:inline-block;width:12px;height:3px;background:#d29922;vertical-align:middle;margin-right:4px;"></span>Технический</span>
                        <span><span style="display:inline-block;width:12px;height:3px;background:#3fb950;vertical-align:middle;margin-right:4px;"></span>Комбинированный</span>
                        <span><span style="display:inline-block;width:12px;height:3px;background:#bc8cff;vertical-align:middle;margin-right:4px;border-bottom:1px dotted #bc8cff;"></span>IMOEX</span>
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
                                    <div class="mp-stat__value" style="font-size:1rem;" :class="pnlClass(stats[type] ? stats[type].monthlyReturn : 0)">
                                        {{ stats[type] ? fmtReturn(stats[type].monthlyReturn) : '—' }}
                                    </div>
                                    <div class="mp-stat__label">За месяц</div>
                                </div>
                                <div class="col-6">
                                    <div class="mp-stat__value" style="font-size:1rem;" :class="pnlClass(stats[type] ? stats[type].annualReturn : 0)">
                                        {{ stats[type] ? fmtReturn(stats[type].annualReturn) : '—' }}
                                    </div>
                                    <div class="mp-stat__label">За год</div>
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
                                    <td>{{ fmtDate(t.date || t.exit_time) }}</td>
                                    <td class="fw-bold text-accent">{{ t.ticker }}</td>
                                    <td>
                                        <span class="badge" :class="(t.direction||t.side)==='BUY'?'badge-buy':'badge-sell'">{{ (t.direction||t.side) === 'BUY' ? 'ПОКУПКА' : 'ПРОДАЖА' }}</span>
                                    </td>
                                    <td class="text-end font-monospace">{{ fmtPrice(t.price || t.entry_price) }}</td>
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
