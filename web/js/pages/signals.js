/* ============================================================
   MarketPulse_RU — Signals Page
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var computed  = Vue.computed;
    var onMounted = Vue.onMounted;

    window.PageSignals = {
        name: 'PageSignals',
        setup: function () {
            var signals   = ref([]);
            var tickers   = ref([]);
            var mode      = ref('');
            var loading   = ref(true);

            /* Filters */
            var filterTicker    = ref('');
            var filterSource    = ref('');
            var filterDirection = ref('');

            var filteredSignals = computed(function () {
                var list = signals.value;
                if (filterTicker.value) {
                    list = list.filter(function (s) { return s.ticker === filterTicker.value; });
                }
                if (filterSource.value) {
                    list = list.filter(function (s) { return s.source === filterSource.value; });
                }
                if (filterDirection.value) {
                    list = list.filter(function (s) { return s.direction === filterDirection.value; });
                }
                return list;
            });

            var uniqueTickers = computed(function () {
                var set = {};
                signals.value.forEach(function (s) { set[s.ticker] = true; });
                return Object.keys(set).sort();
            });

            function fmtTime(ts) {
                if (!ts) return '';
                return new Date(ts).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
            }

            function strengthPercent(v) {
                return Math.round((v || 0) * 100);
            }

            function strengthColor(v, dir) {
                if (dir === 'BUY') return 'var(--mp-green)';
                if (dir === 'SELL') return 'var(--mp-red)';
                return 'var(--mp-text-muted)';
            }

            async function fetchSignals() {
                loading.value = true;
                try {
                    var data = await API.getSignals();
                    signals.value = (data && data.signals) ? data.signals : [];
                    tickers.value = (data && data.tickers) ? data.tickers : [];
                    mode.value = (data && data.mode) ? data.mode : '';
                } catch (err) {
                    signals.value = [];
                }
                loading.value = false;
            }

            onMounted(fetchSignals);

            return {
                signals: signals,
                filteredSignals: filteredSignals,
                tickers: tickers,
                mode: mode,
                loading: loading,
                filterTicker: filterTicker,
                filterSource: filterSource,
                filterDirection: filterDirection,
                uniqueTickers: uniqueTickers,
                fmtTime: fmtTime,
                strengthPercent: strengthPercent,
                strengthColor: strengthColor
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <h1><i class="bi bi-lightning-charge me-2"></i>Торговые сигналы</h1>
        <p class="text-muted mb-0" style="font-size:0.85rem;">
            Сигналы от новостного анализа, тех. анализа и комбинированной стратегии.
            <span v-if="mode" class="badge badge-info ms-1">Режим: {{ mode }}</span>
        </p>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка сигналов...
        </div>

        <div v-else>
            <!-- Filters -->
            <div class="d-flex align-items-center gap-2 mb-3 flex-wrap">
                <select class="form-select form-select-sm mp-search-input" style="max-width:160px;" v-model="filterTicker">
                    <option value="">Все тикеры</option>
                    <option v-for="t in (uniqueTickers.length ? uniqueTickers : tickers)" :key="t" :value="t">{{ t }}</option>
                </select>
                <select class="form-select form-select-sm mp-search-input" style="max-width:160px;" v-model="filterSource">
                    <option value="">Все источники</option>
                    <option value="NEWS">NEWS</option>
                    <option value="TA">TA</option>
                    <option value="COMBINED">COMBINED</option>
                </select>
                <select class="form-select form-select-sm mp-search-input" style="max-width:160px;" v-model="filterDirection">
                    <option value="">Все направления</option>
                    <option value="BUY">BUY</option>
                    <option value="SELL">SELL</option>
                    <option value="HOLD">HOLD</option>
                </select>
            </div>

            <!-- No signals message -->
            <div v-if="signals.length === 0" class="mp-card">
                <div class="mp-card-body text-center py-5">
                    <i class="bi bi-lightning-charge" style="font-size:3rem;color:var(--mp-text-muted);display:block;margin-bottom:1rem;"></i>
                    <h5 class="text-muted">Сигналов пока нет</h5>
                    <p class="text-muted mb-3" style="font-size:0.9rem;">
                        Сигналы появятся, когда торговый движок будет запущен.
                    </p>
                    <div v-if="tickers.length" class="mt-3">
                        <p class="text-muted mb-2" style="font-size:0.8rem;">Отслеживаемые тикеры:</p>
                        <div class="d-flex gap-1 flex-wrap justify-content-center">
                            <router-link v-for="t in tickers" :key="t" :to="'/stocks/' + t" class="badge badge-news text-decoration-none" style="font-size:0.75rem;">
                                {{ t }}
                            </router-link>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Signals table -->
            <div v-else class="mp-card">
                <div class="mp-card-body p-0">
                    <div class="table-responsive">
                        <table class="mp-table">
                            <thead>
                                <tr>
                                    <th>Время</th>
                                    <th>Тикер</th>
                                    <th>Направление</th>
                                    <th>Источник</th>
                                    <th>Сила</th>
                                    <th>Причина</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="s in filteredSignals" :key="s.id">
                                    <td class="text-muted">{{ fmtTime(s.created_at) }}</td>
                                    <td>
                                        <router-link :to="'/stocks/' + s.ticker" class="text-accent text-decoration-none fw-bold">{{ s.ticker }}</router-link>
                                    </td>
                                    <td>
                                        <span class="badge" :class="s.direction==='BUY' ? 'badge-buy' : s.direction==='SELL' ? 'badge-sell' : 'badge-hold'">
                                            {{ s.direction }}
                                        </span>
                                    </td>
                                    <td>
                                        <span class="badge" :class="'badge-' + (s.source || '').toLowerCase()">{{ s.source }}</span>
                                    </td>
                                    <td>
                                        <div class="d-flex align-items-center gap-2">
                                            <div class="mp-strength-bar" style="width:80px;">
                                                <div class="mp-strength-bar__fill"
                                                     :style="{ width: strengthPercent(s.strength) + '%', backgroundColor: strengthColor(s.strength, s.direction) }">
                                                </div>
                                            </div>
                                            <small class="text-muted">{{ strengthPercent(s.strength) }}%</small>
                                        </div>
                                    </td>
                                    <td style="font-size:0.8rem;max-width:300px;">{{ s.reason }}</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div v-if="filteredSignals.length === 0 && signals.length > 0" class="mp-empty mt-3">
                <i class="bi bi-funnel"></i>
                <div>Нет сигналов по выбранным фильтрам</div>
            </div>
        </div>
    </div>
</div>
`
    };
})();
