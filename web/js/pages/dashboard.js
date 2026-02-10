/* ============================================================
   MarketPulse_RU — Dashboard Page (Gridstack widgets)
   ============================================================ */
(function () {
    'use strict';

    var ref      = Vue.ref;
    var reactive = Vue.reactive;
    var onMounted  = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var nextTick = Vue.nextTick;
    var watch    = Vue.watch;

    var STORAGE_KEY = 'mp_dashboard_layout';
    var STORAGE_VIS = 'mp_dashboard_visible';

    /* Default grid layout: { id, x, y, w, h } */
    var DEFAULT_LAYOUT = [
        { id: 'market',     x: 0, y: 0, w: 4, h: 2 },
        { id: 'signals',    x: 4, y: 0, w: 4, h: 4 },
        { id: 'alerts',     x: 8, y: 0, w: 4, h: 4 },
        { id: 'news',       x: 0, y: 2, w: 4, h: 4 },
        { id: 'heatmap',    x: 0, y: 6, w: 6, h: 4 },
        { id: 'portfolios', x: 6, y: 6, w: 3, h: 3 },
        { id: 'quotes',     x: 9, y: 6, w: 3, h: 3 }
    ];

    var WIDGET_META = {
        market:     { title: 'Обзор рынка',          icon: 'bi-graph-up' },
        signals:    { title: 'Последние сигналы',     icon: 'bi-lightning-charge' },
        alerts:     { title: 'Оповещения',            icon: 'bi-bell' },
        news:       { title: 'Лента новостей',        icon: 'bi-newspaper' },
        heatmap:    { title: 'Тепловая карта',        icon: 'bi-grid-3x3-gap' },
        portfolios: { title: 'Портфели',              icon: 'bi-briefcase' },
        quotes:     { title: 'Котировки',             icon: 'bi-currency-exchange' }
    };

    function loadLayout() {
        try {
            var raw = localStorage.getItem(STORAGE_KEY);
            if (raw) return JSON.parse(raw);
        } catch (_) {}
        return null;
    }

    function saveLayout(items) {
        try {
            var data = items.map(function (n) {
                return { id: n.id, x: n.x, y: n.y, w: n.w, h: n.h };
            });
            localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
        } catch (_) {}
    }

    function loadVisibility() {
        try {
            var raw = localStorage.getItem(STORAGE_VIS);
            if (raw) return JSON.parse(raw);
        } catch (_) {}
        return null;
    }

    function saveVisibility(map) {
        try { localStorage.setItem(STORAGE_VIS, JSON.stringify(map)); } catch (_) {}
    }

    function severityClass(s) {
        if (!s) return 'badge-info';
        var lc = s.toLowerCase();
        if (lc === 'critical') return 'badge-critical';
        if (lc === 'urgent')   return 'badge-urgent';
        if (lc === 'important') return 'badge-important';
        return 'badge-info';
    }

    function heatColor(score) {
        if (score > 0.5)  return 'rgba(63,185,80,0.6)';
        if (score > 0.2)  return 'rgba(63,185,80,0.3)';
        if (score > -0.2) return 'rgba(139,148,158,0.2)';
        if (score > -0.5) return 'rgba(248,81,73,0.3)';
        return 'rgba(248,81,73,0.6)';
    }

    function fmtTime(ts) {
        if (!ts) return '';
        var d = new Date(ts);
        return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
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

    window.PageDashboard = {
        name: 'PageDashboard',
        setup: function () {
            var grid = ref(null);
            var loading = ref(true);

            /* Data stores */
            var indexData   = reactive({ last: null, change: null, name: 'IMOEX' });
            var signals     = ref([]);
            var news        = ref([]);
            var alerts      = ref([]);
            var heatmap     = ref([]);
            var portfolios  = ref([]);
            var quoteTickers = ref([]);
            var quotes      = ref({});
            var universeCount = ref(0);

            /* Widget visibility */
            var defaultVis = {};
            Object.keys(WIDGET_META).forEach(function (k) { defaultVis[k] = true; });
            var visibility = reactive(loadVisibility() || Object.assign({}, defaultVis));

            var collapsed = reactive({});

            function toggleWidget(id) {
                visibility[id] = !visibility[id];
                saveVisibility(visibility);
                nextTick(function () { rebuildGrid(); });
            }

            function toggleCollapse(id) {
                collapsed[id] = !collapsed[id];
            }

            function removeWidget(id) {
                visibility[id] = false;
                saveVisibility(visibility);
                nextTick(function () { rebuildGrid(); });
            }

            var gridInstance = null;

            function rebuildGrid() {
                if (gridInstance) {
                    gridInstance.destroy(false);
                    gridInstance = null;
                }
                nextTick(function () { initGrid(); });
            }

            function initGrid() {
                var el = document.querySelector('.grid-stack');
                if (!el) return;
                var saved = loadLayout();
                var layout = saved || DEFAULT_LAYOUT;

                gridInstance = GridStack.init({
                    column: 12,
                    cellHeight: 70,
                    margin: 8,
                    animate: true,
                    float: false,
                    handle: '.mp-widget-header',
                    disableResize: false,
                    removable: false
                }, el);

                gridInstance.on('change', function (event, items) {
                    if (items && items.length) saveLayout(gridInstance.getGridItems().map(function (el) {
                        return { id: el.getAttribute('gs-id'), x: parseInt(el.getAttribute('gs-x')), y: parseInt(el.getAttribute('gs-y')), w: parseInt(el.getAttribute('gs-w')), h: parseInt(el.getAttribute('gs-h')) };
                    }));
                });
            }

            var lastUpdated = ref(null);

            /* Handle incoming SSE quotes */
            function onQuotesUpdate(allQuotes) {
                // Update index
                if (allQuotes['IMOEX']) {
                    var iq = allQuotes['IMOEX'];
                    indexData.last = iq.last || null;
                    indexData.change = iq.change || null;
                }
                // Update ticker quotes
                for (var t in allQuotes) {
                    if (t === 'IMOEX') continue;
                    quotes.value[t] = allQuotes[t];
                }
                // Update quoteTickers if not set yet (from SSE data)
                if (quoteTickers.value.length === 0) {
                    var tickers = Object.keys(allQuotes).filter(function (t) { return t !== 'IMOEX'; });
                    quoteTickers.value = tickers.slice(0, 10);
                }
                lastUpdated.value = new Date();
            }

            /* Fetch non-quote data (silent=true skips loading spinner) */
            async function fetchAll(silent) {
                if (!silent) loading.value = true;
                var promises = [];

                promises.push(
                    API.getSignals().then(function (d) {
                        signals.value = (d && d.signals) ? d.signals : [];
                    }).catch(function () { signals.value = []; })
                );

                promises.push(
                    API.getNews(5, 0).then(function (d) {
                        news.value = Array.isArray(d) ? d : [];
                    }).catch(function () { news.value = []; })
                );

                promises.push(
                    API.getAlerts(5).then(function (d) {
                        alerts.value = Array.isArray(d) ? d : [];
                    }).catch(function () { alerts.value = []; })
                );

                promises.push(
                    API.getHeatmap('company', '1d', 20).then(function (d) {
                        heatmap.value = Array.isArray(d) ? d : [];
                    }).catch(function () { heatmap.value = []; })
                );

                promises.push(
                    API.getPortfolios().then(function (d) {
                        portfolios.value = (d && d.portfolios) ? d.portfolios : [];
                    }).catch(function () { portfolios.value = []; })
                );

                /* Load companies list (for universe count + ticker names) */
                if (universeCount.value === 0) {
                    promises.push(
                        API.getCompanies().then(function (companies) {
                            if (!Array.isArray(companies)) return;
                            universeCount.value = companies.length;
                            /* If SSE hasn't set tickers yet, pick top 10 by market cap */
                            if (quoteTickers.value.length === 0) {
                                var sorted = companies.slice().sort(function (a, b) {
                                    return (b.market_cap || 0) - (a.market_cap || 0);
                                });
                                quoteTickers.value = sorted.slice(0, 10).map(function (c) { return c.ticker; });
                            }
                        }).catch(function () {})
                    );
                }

                await Promise.allSettled(promises);
                loading.value = false;
            }

            var refreshInterval = null;
            var unsubQuotes = null;

            onMounted(function () {
                // Subscribe to real-time quotes via SSE
                unsubQuotes = QuoteStream.subscribe(onQuotesUpdate);
                fetchAll().then(function () {
                    nextTick(function () { initGrid(); });
                });
                // Refresh non-quote data every 30s
                refreshInterval = setInterval(function () { fetchAll(true); }, 30000);
            });

            onBeforeUnmount(function () {
                if (unsubQuotes) unsubQuotes();
                if (refreshInterval) clearInterval(refreshInterval);
                if (gridInstance) { gridInstance.destroy(false); gridInstance = null; }
            });

            return {
                loading: loading,
                indexData: indexData,
                signals: signals,
                news: news,
                alerts: alerts,
                heatmap: heatmap,
                portfolios: portfolios,
                quoteTickers: quoteTickers,
                quotes: quotes,
                universeCount: universeCount,
                visibility: visibility,
                collapsed: collapsed,
                toggleWidget: toggleWidget,
                toggleCollapse: toggleCollapse,
                removeWidget: removeWidget,
                WIDGET_META: WIDGET_META,
                severityClass: severityClass,
                heatColor: heatColor,
                fmtTime: fmtTime,
                lastUpdated: lastUpdated,
                fmtChange: fmtChange,
                changeClass: changeClass
            };
        },
        template: `
<div>
    <div class="mp-page-header d-flex align-items-center justify-content-between flex-wrap gap-2">
        <div>
            <h1 class="d-inline"><i class="bi bi-speedometer2 me-2"></i>Панель управления</h1>
            <span v-if="lastUpdated" class="ms-3 text-muted" style="font-size:0.7rem;">
                <span class="mp-live-dot"></span>
                {{ lastUpdated.toLocaleTimeString('ru-RU', {hour:'2-digit',minute:'2-digit',second:'2-digit'}) }}
            </span>
        </div>
        <div class="dropdown">
            <button class="btn btn-sm btn-outline-secondary dropdown-toggle" type="button" data-bs-toggle="dropdown">
                <i class="bi bi-grid-3x3-gap me-1"></i>Виджеты
            </button>
            <ul class="dropdown-menu dropdown-menu-end mp-widget-toggle-list" style="background:var(--mp-bg-secondary);border-color:var(--mp-border);">
                <li v-for="(meta, id) in WIDGET_META" :key="id">
                    <label class="dropdown-item d-flex align-items-center" style="cursor:pointer;color:var(--mp-text-primary);">
                        <input type="checkbox" class="form-check-input me-2" :checked="visibility[id]" @change="toggleWidget(id)">
                        <i class="bi me-2" :class="meta.icon"></i> {{ meta.title }}
                    </label>
                </li>
            </ul>
        </div>
    </div>

    <div class="mp-page-content">
        <!-- Loading -->
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2" role="status"></div>
            Загрузка данных...
        </div>

        <!-- Grid -->
        <div class="grid-stack" v-show="!loading">

            <!-- Market Overview -->
            <div v-if="visibility.market" class="grid-stack-item" gs-id="market" gs-x="0" gs-y="0" gs-w="4" gs-h="2">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-graph-up me-1"></i>Обзор рынка</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('market')"><i class="bi" :class="collapsed.market?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('market')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.market">
                        <div class="text-center">
                            <div class="mb-1 text-muted" style="font-size:0.75rem;">ИНДЕКС МОСБИРЖИ</div>
                            <div class="mp-stat__value" :class="changeClass(indexData.change)">
                                {{ indexData.last !== null ? Number(indexData.last).toLocaleString('ru-RU') : '—' }}
                            </div>
                            <div class="mt-1" :class="changeClass(indexData.change)" style="font-size:0.9rem;">
                                {{ indexData.change !== null ? fmtChange(indexData.change) : '' }}
                            </div>
                            <div class="mt-2 text-muted" style="font-size:0.7rem;" v-if="universeCount > 0">
                                <i class="bi bi-globe me-1"></i>Мониторинг: {{ universeCount }} компаний
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Latest Signals -->
            <div v-if="visibility.signals" class="grid-stack-item" gs-id="signals" gs-x="4" gs-y="0" gs-w="4" gs-h="4">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-lightning-charge me-1"></i>Последние сигналы</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('signals')"><i class="bi" :class="collapsed.signals?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('signals')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.signals">
                        <div v-if="signals.length === 0" class="mp-empty">
                            <i class="bi bi-lightning-charge"></i>
                            <div>Сигналов пока нет</div>
                            <small class="text-muted">Появятся при запуске торгового движка</small>
                        </div>
                        <table v-else class="mp-table">
                            <thead><tr><th>Тикер</th><th>Направление</th><th>Сила</th><th>Источник</th></tr></thead>
                            <tbody>
                                <tr v-for="s in signals" :key="s.id">
                                    <td class="fw-bold">{{ s.ticker }}</td>
                                    <td><span class="badge" :class="s.direction==='BUY'?'badge-buy':'badge-sell'">{{ s.direction }}</span></td>
                                    <td>
                                        <div class="mp-strength-bar" style="width:60px;">
                                            <div class="mp-strength-bar__fill" :style="{width: (s.strength*100)+'%', backgroundColor: s.direction==='BUY'?'var(--mp-green)':'var(--mp-red)'}"></div>
                                        </div>
                                    </td>
                                    <td><span class="badge" :class="'badge-'+s.source.toLowerCase()">{{ s.source }}</span></td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Alerts -->
            <div v-if="visibility.alerts" class="grid-stack-item" gs-id="alerts" gs-x="8" gs-y="0" gs-w="4" gs-h="4">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-bell me-1"></i>Оповещения</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('alerts')"><i class="bi" :class="collapsed.alerts?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('alerts')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.alerts">
                        <div v-if="alerts.length === 0" class="mp-empty">
                            <i class="bi bi-bell-slash"></i>
                            <div>Нет оповещений</div>
                        </div>
                        <div v-else>
                            <div v-for="a in alerts" :key="a.id" class="d-flex align-items-start mb-2 pb-2" style="border-bottom:1px solid var(--mp-border-light);">
                                <span class="badge me-2 mt-1" :class="severityClass(a.severity)" style="font-size:0.65rem;">{{ a.severity }}</span>
                                <div>
                                    <div style="font-size:0.85rem;" class="fw-semibold">{{ a.title }}</div>
                                    <div style="font-size:0.75rem;" class="text-muted">{{ fmtTime(a.created_at) }}</div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- News Feed -->
            <div v-if="visibility.news" class="grid-stack-item" gs-id="news" gs-x="0" gs-y="2" gs-w="4" gs-h="4">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-newspaper me-1"></i>Лента новостей</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('news')"><i class="bi" :class="collapsed.news?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('news')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.news">
                        <div v-if="news.length === 0" class="mp-empty">
                            <i class="bi bi-newspaper"></i>
                            <div>Новостей пока нет</div>
                        </div>
                        <div v-else>
                            <div v-for="n in news" :key="n.id" class="mb-2 pb-2" style="border-bottom:1px solid var(--mp-border-light);">
                                <div style="font-size:0.85rem;" class="fw-semibold">{{ n.title || (n.content && n.content.substring(0, 80) + '...') || 'Без заголовка' }}</div>
                                <div style="font-size:0.7rem;" class="text-muted">
                                    <span class="badge badge-news me-1" style="font-size:0.6rem;">{{ n.source || 'N/A' }}</span>
                                    {{ fmtTime(n.published_at || n.collected_at) }}
                                </div>
                            </div>
                            <router-link to="/news" class="btn btn-sm btn-outline-primary w-100 mt-1">Все новости</router-link>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Heatmap -->
            <div v-if="visibility.heatmap" class="grid-stack-item" gs-id="heatmap" gs-x="0" gs-y="6" gs-w="6" gs-h="4">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-grid-3x3-gap me-1"></i>Тепловая карта</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('heatmap')"><i class="bi" :class="collapsed.heatmap?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('heatmap')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.heatmap">
                        <div v-if="heatmap.length === 0" class="mp-empty">
                            <i class="bi bi-grid-3x3-gap"></i>
                            <div>Нет данных для тепловой карты</div>
                        </div>
                        <div v-else class="mp-heatmap">
                            <div v-for="h in heatmap" :key="h.id || h.entity_id"
                                 class="mp-heatmap__cell"
                                 :style="{ backgroundColor: heatColor(h.score) }"
                                 :title="'Score: ' + (h.score ? h.score.toFixed(2) : '0')">
                                <span class="ticker">{{ h.entity_name || ('ID:' + h.entity_id) }}</span>
                                <span class="score">{{ h.score ? h.score.toFixed(1) : '0' }}</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Portfolios -->
            <div v-if="visibility.portfolios" class="grid-stack-item" gs-id="portfolios" gs-x="6" gs-y="6" gs-w="3" gs-h="3">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-briefcase me-1"></i>Портфели</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('portfolios')"><i class="bi" :class="collapsed.portfolios?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('portfolios')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.portfolios">
                        <div v-if="portfolios.length === 0" class="mp-empty">
                            <i class="bi bi-briefcase"></i>
                            <div>Портфели не настроены</div>
                        </div>
                        <div v-else>
                            <div v-for="p in portfolios" :key="p.type" class="d-flex justify-content-between align-items-center mb-2 pb-2" style="border-bottom:1px solid var(--mp-border-light);">
                                <div>
                                    <div class="fw-semibold" style="font-size:0.85rem;">{{ p.name }}</div>
                                    <div class="text-muted" style="font-size:0.7rem;">{{ p.description }}</div>
                                </div>
                                <div class="text-end">
                                    <div class="fw-bold" style="font-size:0.85rem;">{{ Number(p.initial_cash).toLocaleString('ru-RU') }}</div>
                                    <div class="text-muted" style="font-size:0.65rem;">RUB</div>
                                </div>
                            </div>
                            <router-link to="/portfolios" class="btn btn-sm btn-outline-primary w-100 mt-1">Подробнее</router-link>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Quick Quotes -->
            <div v-if="visibility.quotes" class="grid-stack-item" gs-id="quotes" gs-x="9" gs-y="6" gs-w="3" gs-h="3">
                <div class="grid-stack-item-content">
                    <div class="mp-widget-header">
                        <span class="mp-widget-header__title"><i class="bi bi-currency-exchange me-1"></i>Котировки</span>
                        <span class="mp-widget-header__actions">
                            <button class="btn btn-outline-secondary" @click.stop="toggleCollapse('quotes')"><i class="bi" :class="collapsed.quotes?'bi-chevron-down':'bi-chevron-up'"></i></button>
                            <button class="btn btn-outline-secondary" @click.stop="removeWidget('quotes')"><i class="bi bi-x-lg"></i></button>
                        </span>
                    </div>
                    <div class="mp-widget-body" v-show="!collapsed.quotes">
                        <table class="mp-table" style="font-size:0.8rem;">
                            <thead><tr><th>Тикер</th><th class="text-end">Цена</th><th class="text-end">Изм.</th></tr></thead>
                            <tbody>
                                <tr v-for="t in quoteTickers" :key="t">
                                    <td>
                                        <router-link :to="'/stocks/' + t" class="text-accent text-decoration-none fw-bold">{{ t }}</router-link>
                                    </td>
                                    <td class="text-end">{{ quotes[t] ? (quotes[t].last || quotes[t].LAST || quotes[t].price || '—') : '...' }}</td>
                                    <td class="text-end" :class="quotes[t] ? changeClass(quotes[t].change || quotes[t].CHANGE || quotes[t].lasttoprevprice) : ''">
                                        {{ quotes[t] ? fmtChange(quotes[t].change || quotes[t].CHANGE || quotes[t].lasttoprevprice) : '' }}
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
