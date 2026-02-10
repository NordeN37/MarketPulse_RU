/* ============================================================
   MarketPulse_RU — Stocks List Page
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var computed  = Vue.computed;
    var onMounted = Vue.onMounted;
    var watch     = Vue.watch;

    var SECTOR_LABELS = {
        'OIL_GAS':     'Нефть и газ',
        'BANKS':       'Банки',
        'RETAIL':      'Ритейл',
        'TELECOM':     'Телеком',
        'METALS':      'Металлы',
        'CHEMISTRY':   'Химия',
        'ENERGY':      'Энергетика',
        'IT':          'ИТ',
        'REAL_ESTATE': 'Недвижимость',
        'TRANSPORT':   'Транспорт',
        'AGRICULTURE': 'Сельское хозяйство',
        'FINANCE':     'Финансы'
    };

    window.PageStocks = {
        name: 'PageStocks',
        setup: function () {
            var router = VueRouter.useRouter();
            var companies = ref([]);
            var quotes = ref({});
            var search = ref('');
            var loading = ref(true);
            var sortCol = ref('ticker');
            var sortDir = ref(1); // 1=asc, -1=desc

            var filteredCompanies = computed(function () {
                var q = search.value.toLowerCase().trim();
                var list = companies.value;
                if (q) {
                    list = list.filter(function (c) {
                        return c.ticker.toLowerCase().indexOf(q) >= 0 ||
                               c.name.toLowerCase().indexOf(q) >= 0 ||
                               (c.sector && c.sector.toLowerCase().indexOf(q) >= 0);
                    });
                }
                /* sort */
                var col = sortCol.value;
                var dir = sortDir.value;
                list = list.slice().sort(function (a, b) {
                    var va, vb;
                    if (col === 'price') {
                        va = getQuotePrice(a.ticker);
                        vb = getQuotePrice(b.ticker);
                    } else if (col === 'change') {
                        va = getQuoteChange(a.ticker);
                        vb = getQuoteChange(b.ticker);
                    } else {
                        va = a[col] || '';
                        vb = b[col] || '';
                    }
                    if (typeof va === 'string') va = va.toLowerCase();
                    if (typeof vb === 'string') vb = vb.toLowerCase();
                    if (va < vb) return -1 * dir;
                    if (va > vb) return 1 * dir;
                    return 0;
                });
                return list;
            });

            function toggleSort(col) {
                if (sortCol.value === col) {
                    sortDir.value *= -1;
                } else {
                    sortCol.value = col;
                    sortDir.value = 1;
                }
            }

            function sortIcon(col) {
                if (sortCol.value !== col) return 'bi-arrow-down-up';
                return sortDir.value === 1 ? 'bi-sort-alpha-down' : 'bi-sort-alpha-up';
            }

            function sectorLabel(code) {
                return SECTOR_LABELS[code] || code || '';
            }

            function getQuotePrice(ticker) {
                var q = quotes.value[ticker];
                if (!q) return null;
                return q.last || q.LAST || q.price || null;
            }

            function getQuoteChange(ticker) {
                var q = quotes.value[ticker];
                if (!q) return null;
                return q.change || q.CHANGE || q.lasttoprevprice || null;
            }

            function fmtPrice(ticker) {
                var p = getQuotePrice(ticker);
                if (p === null) return '...';
                return Number(p).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
            }

            function fmtChange(ticker) {
                var c = getQuoteChange(ticker);
                if (c === null) return '';
                var n = Number(c);
                if (isNaN(n)) return '';
                var sign = n >= 0 ? '+' : '';
                return sign + n.toFixed(2) + '%';
            }

            function changeClass(ticker) {
                var c = getQuoteChange(ticker);
                if (c === null) return 'text-flat';
                var n = Number(c);
                if (isNaN(n) || n === 0) return 'text-flat';
                return n > 0 ? 'text-up' : 'text-down';
            }

            function goToStock(ticker) {
                router.push('/stocks/' + ticker);
            }

            async function fetchData() {
                loading.value = true;
                try {
                    var data = await API.getCompanies();
                    companies.value = Array.isArray(data) ? data : [];

                    /* Fetch quotes in parallel */
                    var promises = companies.value.map(function (c) {
                        return API.getQuote(c.ticker).then(function (q) {
                            quotes.value[c.ticker] = q;
                        }).catch(function () {});
                    });
                    await Promise.allSettled(promises);
                } catch (err) {
                    companies.value = [];
                }
                loading.value = false;
            }

            onMounted(fetchData);

            return {
                companies: companies,
                filteredCompanies: filteredCompanies,
                quotes: quotes,
                search: search,
                loading: loading,
                sortCol: sortCol,
                sortDir: sortDir,
                toggleSort: toggleSort,
                sortIcon: sortIcon,
                sectorLabel: sectorLabel,
                fmtPrice: fmtPrice,
                fmtChange: fmtChange,
                changeClass: changeClass,
                goToStock: goToStock
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <div class="d-flex align-items-center justify-content-between flex-wrap gap-2">
            <h1><i class="bi bi-bar-chart-line me-2"></i>Акции</h1>
            <div style="max-width:300px;width:100%;">
                <input type="text" class="form-control form-control-sm mp-search-input"
                       placeholder="Поиск по тикеру или названию..."
                       v-model="search">
            </div>
        </div>
        <div class="mt-2 text-muted" style="font-size:0.8rem;">
            Всего компаний: {{ filteredCompanies.length }}
        </div>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка списка компаний...
        </div>

        <div v-else-if="filteredCompanies.length === 0" class="mp-empty">
            <i class="bi bi-search"></i>
            <div>Компании не найдены</div>
            <small v-if="search" class="text-muted">Попробуйте изменить поисковый запрос</small>
        </div>

        <div v-else class="table-responsive">
            <table class="mp-table mp-table-clickable">
                <thead>
                    <tr>
                        <th class="cursor-pointer" @click="toggleSort('ticker')">
                            Тикер <i class="bi" :class="sortIcon('ticker')"></i>
                        </th>
                        <th class="cursor-pointer" @click="toggleSort('name')">
                            Название <i class="bi" :class="sortIcon('name')"></i>
                        </th>
                        <th class="cursor-pointer" @click="toggleSort('sector')">
                            Сектор <i class="bi" :class="sortIcon('sector')"></i>
                        </th>
                        <th class="text-end cursor-pointer" @click="toggleSort('price')">
                            Цена <i class="bi" :class="sortIcon('price')"></i>
                        </th>
                        <th class="text-end cursor-pointer" @click="toggleSort('change')">
                            Изм. <i class="bi" :class="sortIcon('change')"></i>
                        </th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="c in filteredCompanies" :key="c.ticker" @click="goToStock(c.ticker)">
                        <td class="fw-bold text-accent">{{ c.ticker }}</td>
                        <td>{{ c.name }}</td>
                        <td><span class="text-muted">{{ sectorLabel(c.sector) }}</span></td>
                        <td class="text-end font-monospace">{{ fmtPrice(c.ticker) }}</td>
                        <td class="text-end font-monospace" :class="changeClass(c.ticker)">{{ fmtChange(c.ticker) }}</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>
</div>
`
    };
})();
