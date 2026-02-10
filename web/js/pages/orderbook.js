/* ============================================================
   MarketPulse_RU — Order Book Page (Стакан)
   ============================================================ */
(function () {
    'use strict';

    var ref = Vue.ref;
    var onMounted = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var watch = Vue.watch;

    function imbalanceColor(val) {
        if (val > 0.3) return 'var(--mp-green)';
        if (val < -0.3) return 'var(--mp-red)';
        return 'var(--mp-text-secondary)';
    }

    function severityBadge(sev) {
        if (sev === 'critical') return 'badge-critical';
        if (sev === 'warning') return 'badge-urgent';
        return 'badge-info';
    }

    function fmtPrice(v) {
        return Number(v).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    }

    function fmtVolume(v) {
        if (v >= 1000000) return (v / 1000000).toFixed(1) + 'M';
        if (v >= 1000) return (v / 1000).toFixed(1) + 'K';
        return String(v);
    }

    window.PageOrderBook = {
        name: 'PageOrderBook',
        setup: function () {
            var ticker = ref('SBER');
            var loading = ref(false);
            var error = ref('');
            var orderbook = ref(null);
            var anomalies = ref([]);
            var refreshTimer = ref(null);
            var companies = ref([]);

            var POPULAR = ['SBER', 'GAZP', 'LKOH', 'YNDX', 'GMKN', 'NVTK', 'ROSN', 'VTBR', 'MGNT', 'PLZL'];

            async function loadCompanies() {
                try {
                    var data = await API.getCompanies();
                    if (Array.isArray(data)) companies.value = data;
                } catch (_) {}
            }

            async function fetchOrderBook() {
                if (!ticker.value) return;
                loading.value = true;
                error.value = '';
                try {
                    var data = await API.getOrderBook(ticker.value);
                    orderbook.value = data.orderbook || null;
                    anomalies.value = data.anomalies || [];
                } catch (err) {
                    error.value = err.message || 'Ошибка загрузки стакана';
                    orderbook.value = null;
                    anomalies.value = [];
                }
                loading.value = false;
            }

            function selectTicker(t) {
                ticker.value = t;
                fetchOrderBook();
            }

            function startAutoRefresh() {
                stopAutoRefresh();
                refreshTimer.value = setInterval(fetchOrderBook, 10000);
            }

            function stopAutoRefresh() {
                if (refreshTimer.value) {
                    clearInterval(refreshTimer.value);
                    refreshTimer.value = null;
                }
            }

            onMounted(function () {
                loadCompanies();
                fetchOrderBook();
                startAutoRefresh();
            });

            onBeforeUnmount(stopAutoRefresh);

            return {
                ticker: ticker,
                loading: loading,
                error: error,
                orderbook: orderbook,
                anomalies: anomalies,
                companies: companies,
                POPULAR: POPULAR,
                selectTicker: selectTicker,
                fetchOrderBook: fetchOrderBook,
                imbalanceColor: imbalanceColor,
                severityBadge: severityBadge,
                fmtPrice: fmtPrice,
                fmtVolume: fmtVolume
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <h1><i class="bi bi-bar-chart-steps me-2"></i>Стакан заявок</h1>
        <p class="text-muted mb-0" style="font-size:0.85rem;">
            Стакан котировок MOEX с автоматическим обнаружением аномалий (дисбаланс, крупные заявки, спред).
            <span class="text-warning" style="font-size:0.75rem;">Требуется авторизация MOEX Passport</span>
        </p>
    </div>

    <div class="mp-page-content">
        <!-- Ticker selector -->
        <div class="mp-card mb-3">
            <div class="mp-card-body d-flex flex-wrap align-items-center gap-2">
                <label class="text-muted me-2" style="font-size:0.85rem;">Тикер:</label>
                <input v-model="ticker" class="form-control form-control-sm" style="width:100px;background:var(--mp-bg-secondary);color:var(--mp-text-primary);border-color:var(--mp-border);"
                       @keyup.enter="fetchOrderBook" placeholder="SBER">
                <button class="btn btn-sm btn-accent" @click="fetchOrderBook" :disabled="loading">
                    <i class="bi bi-arrow-clockwise me-1"></i>Загрузить
                </button>
                <span class="ms-2 text-muted" style="font-size:0.75rem;">Популярные:</span>
                <button v-for="t in POPULAR" :key="t"
                        class="btn btn-sm" :class="ticker === t ? 'btn-accent' : 'btn-outline-secondary'"
                        @click="selectTicker(t)" style="font-size:0.75rem;padding:2px 8px;">
                    {{ t }}
                </button>
            </div>
        </div>

        <!-- Error -->
        <div v-if="error" class="alert alert-danger" style="background:rgba(248,81,73,0.15);border-color:var(--mp-red);color:var(--mp-red);">
            <i class="bi bi-exclamation-triangle me-1"></i>{{ error }}
            <div class="mt-1 text-muted" style="font-size:0.75rem;">
                Стакан требует авторизации MOEX Passport. Укажите MOEX_PASSPORT_LOGIN и MOEX_PASSPORT_PASSWORD в .env
            </div>
        </div>

        <!-- Loading -->
        <div v-if="loading && !orderbook" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка стакана...
        </div>

        <!-- Anomalies -->
        <div v-if="anomalies.length > 0" class="mp-card mb-3">
            <div class="mp-card-header">
                <h6 class="mp-card-header__title"><i class="bi bi-exclamation-diamond me-1"></i>Аномалии</h6>
            </div>
            <div class="mp-card-body">
                <div v-for="(a, idx) in anomalies" :key="idx" class="d-flex align-items-start mb-2 pb-2" style="border-bottom:1px solid var(--mp-border-light);">
                    <span class="badge me-2 mt-1" :class="severityBadge(a.severity)" style="font-size:0.65rem;">{{ a.severity }}</span>
                    <div>
                        <div style="font-size:0.85rem;">{{ a.message }}</div>
                        <div class="text-muted" style="font-size:0.7rem;">{{ a.type }}</div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Order book -->
        <div v-if="orderbook" class="row g-3">
            <!-- Summary -->
            <div class="col-12">
                <div class="mp-card">
                    <div class="mp-card-body">
                        <div class="row g-3 text-center">
                            <div class="col-md-2">
                                <div class="mp-stat__value" style="font-size:1.2rem;">{{ orderbook.secid }}</div>
                                <div class="mp-stat__label">Тикер</div>
                            </div>
                            <div class="col-md-2">
                                <div class="mp-stat__value" style="font-size:1.2rem;">{{ fmtPrice(orderbook.spread) }}</div>
                                <div class="mp-stat__label">Спред ({{ orderbook.spread_pct ? orderbook.spread_pct.toFixed(3) + '%' : '—' }})</div>
                            </div>
                            <div class="col-md-2">
                                <div class="mp-stat__value text-up" style="font-size:1.2rem;">{{ fmtVolume(orderbook.bid_volume) }}</div>
                                <div class="mp-stat__label">Объём покупок</div>
                            </div>
                            <div class="col-md-2">
                                <div class="mp-stat__value text-down" style="font-size:1.2rem;">{{ fmtVolume(orderbook.ask_volume) }}</div>
                                <div class="mp-stat__label">Объём продаж</div>
                            </div>
                            <div class="col-md-2">
                                <div class="mp-stat__value" style="font-size:1.2rem;" :style="{ color: imbalanceColor(orderbook.imbalance) }">
                                    {{ orderbook.imbalance ? (orderbook.imbalance * 100).toFixed(1) + '%' : '0%' }}
                                </div>
                                <div class="mp-stat__label">Дисбаланс</div>
                            </div>
                            <div class="col-md-2">
                                <div class="mp-stat__value" style="font-size:1.2rem;">{{ (orderbook.bids || []).length + (orderbook.asks || []).length }}</div>
                                <div class="mp-stat__label">Уровней</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Bids / Asks table -->
            <div class="col-md-6">
                <div class="mp-card">
                    <div class="mp-card-header">
                        <h6 class="mp-card-header__title text-up"><i class="bi bi-arrow-up-circle me-1"></i>Покупки (Bid)</h6>
                    </div>
                    <div class="mp-card-body p-0">
                        <table class="mp-table" style="font-size:0.8rem;">
                            <thead><tr><th class="text-end">Цена</th><th class="text-end">Объём</th><th>Визуально</th></tr></thead>
                            <tbody>
                                <tr v-for="(b, i) in (orderbook.bids || []).slice(0, 20)" :key="'b'+i">
                                    <td class="text-end font-monospace text-up">{{ fmtPrice(b.price) }}</td>
                                    <td class="text-end font-monospace">{{ fmtVolume(b.quantity) }}</td>
                                    <td>
                                        <div style="height:14px;background:rgba(63,185,80,0.3);border-radius:2px;"
                                             :style="{ width: Math.min(b.quantity / (orderbook.bid_volume / (orderbook.bids||[]).length || 1) * 30, 100) + '%' }"></div>
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
            <div class="col-md-6">
                <div class="mp-card">
                    <div class="mp-card-header">
                        <h6 class="mp-card-header__title text-down"><i class="bi bi-arrow-down-circle me-1"></i>Продажи (Ask)</h6>
                    </div>
                    <div class="mp-card-body p-0">
                        <table class="mp-table" style="font-size:0.8rem;">
                            <thead><tr><th class="text-end">Цена</th><th class="text-end">Объём</th><th>Визуально</th></tr></thead>
                            <tbody>
                                <tr v-for="(a, i) in (orderbook.asks || []).slice(0, 20)" :key="'a'+i">
                                    <td class="text-end font-monospace text-down">{{ fmtPrice(a.price) }}</td>
                                    <td class="text-end font-monospace">{{ fmtVolume(a.quantity) }}</td>
                                    <td>
                                        <div style="height:14px;background:rgba(248,81,73,0.3);border-radius:2px;"
                                             :style="{ width: Math.min(a.quantity / (orderbook.ask_volume / (orderbook.asks||[]).length || 1) * 30, 100) + '%' }"></div>
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
