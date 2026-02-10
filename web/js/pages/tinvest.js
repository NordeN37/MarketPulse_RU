/* ============================================================
   MarketPulse_RU — T-Invest Trading Page (Admin)
   Token management, accounts, portfolio, orders, margin
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var reactive  = Vue.reactive;
    var computed  = Vue.computed;
    var onMounted = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var watch     = Vue.watch;

    window.PageTInvest = {
        name: 'PageTInvest',
        setup: function () {
            // ---- Connection state ----
            var connected   = ref(false);
            var mode        = ref('');
            var tokenMasked = ref('');
            var accounts    = ref([]);
            var strategies  = ref([]);
            var loading     = ref(true);
            var error       = ref('');

            // ---- Token form ----
            var tokenInput  = ref('');
            var sandboxMode = ref(false);
            var connecting  = ref(false);
            var connectError = ref('');

            // ---- Active tab ----
            var activeTab = ref('overview'); // overview, portfolio, orders, instruments

            // ---- Portfolio state ----
            var selectedAccountId = ref('');
            var portfolio = ref(null);
            var margin    = ref(null);
            var portfolioLoading = ref(false);
            var portfolioError   = ref('');

            // ---- Orders state ----
            var activeOrders     = ref([]);
            var ordersLoading    = ref(false);
            var ordersError      = ref('');

            // ---- Instruments ----
            var instruments      = ref([]);
            var instrLoading     = ref(false);
            var instrError       = ref('');
            var instrSearch      = ref('');

            // ---- Strategy assignment ----
            var savingStrategies = ref(false);

            // ---- Broker portfolios ----
            var brokerPortfolios = ref(null);
            var brokerLoading = ref(false);

            // ---- Withdrawal config ----
            var withdrawal = ref({ enabled: false, profit_percent: 10, day_of_week: 5, min_profit: 1000, account_id: '' });
            var savingWithdrawal = ref(false);
            var withdrawalSaved = ref(false);

            // ---- Streaming state ----
            var streaming = ref(false);

            var pollTimer = null;

            // Filtered instruments by search
            var filteredInstruments = computed(function () {
                if (!instrSearch.value) return instruments.value.slice(0, 100);
                var q = instrSearch.value.toLowerCase();
                return instruments.value.filter(function (i) {
                    return i.ticker.toLowerCase().indexOf(q) >= 0 ||
                           i.name.toLowerCase().indexOf(q) >= 0;
                }).slice(0, 100);
            });

            // ---- Fetch status ----
            async function fetchStatus() {
                try {
                    var data = await API.getTInvestStatus();
                    connected.value   = data.connected;
                    mode.value        = data.mode || '';
                    tokenMasked.value = data.token_masked || '';
                    accounts.value    = data.accounts || [];
                    strategies.value  = data.strategies || [];
                    streaming.value   = data.streaming || false;
                    error.value       = '';

                    // Auto-select first account if none selected
                    if (!selectedAccountId.value && accounts.value.length > 0) {
                        selectedAccountId.value = accounts.value[0].id;
                    }
                } catch (err) {
                    error.value = err.message || 'Ошибка получения статуса';
                }
                loading.value = false;
            }

            // ---- Connect ----
            async function doConnect() {
                if (!tokenInput.value.trim()) return;
                connecting.value = true;
                connectError.value = '';
                try {
                    var data = await API.tInvestConnect(tokenInput.value.trim(), sandboxMode.value);
                    tokenInput.value = '';
                    connected.value   = data.connected;
                    mode.value        = data.mode || '';
                    tokenMasked.value = data.token_masked || '';
                    accounts.value    = data.accounts || [];
                    if (accounts.value.length > 0 && !selectedAccountId.value) {
                        selectedAccountId.value = accounts.value[0].id;
                    }
                } catch (err) {
                    connectError.value = err.message || 'Не удалось подключиться';
                }
                connecting.value = false;
            }

            // ---- Disconnect ----
            async function doDisconnect() {
                try {
                    await API.tInvestDisconnect();
                    connected.value = false;
                    mode.value = '';
                    tokenMasked.value = '';
                    accounts.value = [];
                    portfolio.value = null;
                    margin.value = null;
                    activeOrders.value = [];
                } catch (err) {
                    error.value = err.message;
                }
            }

            // ---- Load portfolio + margin ----
            async function loadPortfolio() {
                if (!selectedAccountId.value || !connected.value) return;
                portfolioLoading.value = true;
                portfolioError.value = '';
                try {
                    var pData = await API.getTInvestPortfolio(selectedAccountId.value);
                    portfolio.value = pData;
                } catch (err) {
                    portfolioError.value = err.message;
                }
                try {
                    var mData = await API.getTInvestMargin(selectedAccountId.value);
                    margin.value = mData;
                } catch (_) {
                    // margin may not be available for all accounts
                }
                portfolioLoading.value = false;
            }

            // ---- Load orders ----
            async function loadOrders() {
                if (!selectedAccountId.value || !connected.value) return;
                ordersLoading.value = true;
                ordersError.value = '';
                try {
                    var data = await API.getTInvestOrders(selectedAccountId.value);
                    activeOrders.value = data || [];
                } catch (err) {
                    ordersError.value = err.message;
                }
                ordersLoading.value = false;
            }

            // ---- Cancel order ----
            async function cancelOrder(orderId) {
                if (!confirm('Отменить ордер ' + orderId + '?')) return;
                try {
                    await API.tInvestCancelOrder(selectedAccountId.value, orderId);
                    loadOrders();
                } catch (err) {
                    ordersError.value = err.message;
                }
            }

            // ---- Load instruments ----
            async function loadInstruments() {
                instrLoading.value = true;
                instrError.value = '';
                try {
                    var data = await API.getTInvestInstruments();
                    instruments.value = data || [];
                } catch (err) {
                    instrError.value = err.message;
                }
                instrLoading.value = false;
            }

            // ---- Strategy assignment helpers ----
            function getStrategy(accountId) {
                var s = strategies.value.find(function (s) { return s.account_id === accountId; });
                return s ? s.strategy : '';
            }

            function getStrategyEnabled(accountId) {
                var s = strategies.value.find(function (s) { return s.account_id === accountId; });
                return s ? s.enabled : false;
            }

            function setStrategy(accountId, strategy) {
                var idx = strategies.value.findIndex(function (s) { return s.account_id === accountId; });
                if (idx >= 0) {
                    strategies.value[idx].strategy = strategy;
                } else {
                    strategies.value.push({ account_id: accountId, strategy: strategy, enabled: false });
                }
            }

            function toggleStrategyEnabled(accountId) {
                var idx = strategies.value.findIndex(function (s) { return s.account_id === accountId; });
                if (idx >= 0) {
                    strategies.value[idx].enabled = !strategies.value[idx].enabled;
                }
            }

            async function saveStrategies() {
                savingStrategies.value = true;
                try {
                    await API.saveTInvestStrategies(strategies.value);
                } catch (err) {
                    error.value = err.message;
                }
                savingStrategies.value = false;
            }

            // ---- Broker portfolios ----
            async function loadBrokerPortfolios() {
                brokerLoading.value = true;
                try {
                    var data = await API.getBrokerPortfolios();
                    brokerPortfolios.value = data;
                } catch (err) {
                    portfolioError.value = err.message;
                }
                brokerLoading.value = false;
            }

            // ---- Withdrawal config ----
            async function loadWithdrawalConfig() {
                try {
                    var data = await API.getWithdrawalConfig();
                    if (data) {
                        withdrawal.value = data;
                    }
                } catch (_) {}
            }

            async function saveWithdrawalConfig() {
                savingWithdrawal.value = true;
                withdrawalSaved.value = false;
                try {
                    await API.saveWithdrawalConfig(withdrawal.value);
                    withdrawalSaved.value = true;
                    setTimeout(function () { withdrawalSaved.value = false; }, 3000);
                } catch (err) {
                    error.value = err.message;
                }
                savingWithdrawal.value = false;
            }

            // ---- Formatting ----
            function fmtMoney(v) {
                if (v == null || isNaN(v)) return '—';
                return v.toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' ₽';
            }

            function fmtPct(v) {
                if (v == null || isNaN(v)) return '—';
                var sign = v >= 0 ? '+' : '';
                return sign + v.toFixed(2) + '%';
            }

            function fmtYield(v) {
                if (v == null || isNaN(v)) return '—';
                var sign = v >= 0 ? '+' : '';
                return sign + v.toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
            }

            function yieldClass(v) {
                if (!v) return '';
                return v >= 0 ? 'text-success' : 'text-danger';
            }

            function accountTypeLabel(t) {
                var map = {
                    'ACCOUNT_TYPE_TINKOFF': 'Брокерский',
                    'ACCOUNT_TYPE_TINKOFF_IIS': 'ИИС',
                    'ACCOUNT_TYPE_INVEST_BOX': 'Копилка',
                    'ACCOUNT_TYPE_INVEST_FUND': 'Фонд'
                };
                return map[t] || t;
            }

            function statusLabel(s) {
                var map = {
                    'ACCOUNT_STATUS_OPEN': 'Открыт',
                    'ACCOUNT_STATUS_CLOSED': 'Закрыт'
                };
                return map[s] || s;
            }

            function accessLabel(a) {
                var map = {
                    'ACCOUNT_ACCESS_LEVEL_FULL_ACCESS': 'Полный',
                    'ACCOUNT_ACCESS_LEVEL_READ_ONLY': 'Только чтение',
                    'ACCOUNT_ACCESS_LEVEL_NO_ACCESS': 'Нет доступа'
                };
                return map[a] || a;
            }

            function directionLabel(d) {
                if (d.indexOf('BUY') >= 0) return 'Покупка';
                if (d.indexOf('SELL') >= 0) return 'Продажа';
                return d;
            }

            function orderStatusLabel(s) {
                var map = {
                    'EXECUTION_REPORT_STATUS_FILL': 'Исполнен',
                    'EXECUTION_REPORT_STATUS_REJECTED': 'Отклонён',
                    'EXECUTION_REPORT_STATUS_CANCELLED': 'Отменён',
                    'EXECUTION_REPORT_STATUS_NEW': 'Новый',
                    'EXECUTION_REPORT_STATUS_PARTIALLYFILL': 'Частично'
                };
                return map[s] || s;
            }

            // Watch account change → reload data
            watch(selectedAccountId, function () {
                if (activeTab.value === 'portfolio') loadPortfolio();
                if (activeTab.value === 'orders') loadOrders();
            });

            watch(activeTab, function (tab) {
                if (tab === 'portfolio') loadPortfolio();
                if (tab === 'orders') loadOrders();
                if (tab === 'instruments' && instruments.value.length === 0) loadInstruments();
                if (tab === 'broker') { loadBrokerPortfolios(); loadWithdrawalConfig(); }
            });

            onMounted(function () {
                fetchStatus();
                pollTimer = setInterval(fetchStatus, 15000);
            });

            onBeforeUnmount(function () {
                if (pollTimer) clearInterval(pollTimer);
            });

            return {
                connected: connected, mode: mode, tokenMasked: tokenMasked,
                accounts: accounts, strategies: strategies,
                loading: loading, error: error,
                tokenInput: tokenInput, sandboxMode: sandboxMode,
                connecting: connecting, connectError: connectError,
                doConnect: doConnect, doDisconnect: doDisconnect,
                activeTab: activeTab,
                selectedAccountId: selectedAccountId,
                portfolio: portfolio, margin: margin,
                portfolioLoading: portfolioLoading, portfolioError: portfolioError,
                loadPortfolio: loadPortfolio,
                activeOrders: activeOrders, ordersLoading: ordersLoading,
                ordersError: ordersError, loadOrders: loadOrders,
                cancelOrder: cancelOrder,
                instruments: instruments, instrLoading: instrLoading,
                instrError: instrError, instrSearch: instrSearch,
                filteredInstruments: filteredInstruments, loadInstruments: loadInstruments,
                getStrategy: getStrategy, setStrategy: setStrategy,
                getStrategyEnabled: getStrategyEnabled, toggleStrategyEnabled: toggleStrategyEnabled,
                saveStrategies: saveStrategies, savingStrategies: savingStrategies,
                brokerPortfolios: brokerPortfolios, brokerLoading: brokerLoading,
                loadBrokerPortfolios: loadBrokerPortfolios,
                withdrawal: withdrawal, savingWithdrawal: savingWithdrawal,
                withdrawalSaved: withdrawalSaved,
                saveWithdrawalConfig: saveWithdrawalConfig,
                streaming: streaming,
                fmtMoney: fmtMoney, fmtPct: fmtPct, fmtYield: fmtYield,
                yieldClass: yieldClass,
                accountTypeLabel: accountTypeLabel, statusLabel: statusLabel,
                accessLabel: accessLabel, directionLabel: directionLabel,
                orderStatusLabel: orderStatusLabel
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <h1><i class="bi bi-bank me-2"></i>Т-Инвестиции</h1>
    </div>

    <div class="mp-page-content">
        <!-- Loading -->
        <div v-if="loading" class="text-center py-5">
            <span class="spinner-border text-primary"></span>
        </div>

        <!-- Error -->
        <div v-if="error" class="alert alert-danger py-2 mb-3" style="font-size:0.85rem;">
            <i class="bi bi-exclamation-triangle me-1"></i>{{ error }}
        </div>

        <!-- ============ NOT CONNECTED ============ -->
        <div v-if="!loading && !connected">
            <div class="mp-settings-card">
                <div class="mp-settings-card__header">
                    <i class="bi bi-key me-2" style="color:var(--mp-accent);"></i>
                    Подключение к T-Invest API
                </div>
                <div class="mp-settings-card__body">
                    <p style="font-size:0.85rem; color:var(--mp-text-secondary);">
                        Введите API-токен T-Invest для активации торговых функций.
                        Токен можно получить в
                        <a href="https://www.tbank.ru/invest/settings/" target="_blank" rel="noopener">настройках T-Invest</a>.
                    </p>

                    <div v-if="connectError" class="alert alert-danger py-2 px-3 mb-3" style="font-size:0.85rem;">
                        <i class="bi bi-exclamation-triangle me-1"></i>{{ connectError }}
                    </div>

                    <div class="mb-3" style="max-width:500px;">
                        <label class="form-label" style="font-size:0.85rem;">API Токен</label>
                        <input type="password" class="form-control"
                               v-model="tokenInput"
                               placeholder="t.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                               @keyup.enter="doConnect"
                               :disabled="connecting">
                    </div>

                    <div class="form-check mb-3">
                        <input class="form-check-input" type="checkbox" id="sandboxCheck"
                               v-model="sandboxMode" :disabled="connecting">
                        <label class="form-check-label" for="sandboxCheck" style="font-size:0.85rem;">
                            Sandbox (тестовый режим)
                        </label>
                    </div>

                    <button class="btn btn-primary" @click="doConnect"
                            :disabled="connecting || !tokenInput.trim()">
                        <span v-if="connecting">
                            <span class="spinner-border spinner-border-sm me-1"></span>
                            Подключение...
                        </span>
                        <span v-else>
                            <i class="bi bi-plug me-1"></i>Подключить
                        </span>
                    </button>

                    <div class="mt-3" style="font-size:0.78rem; color:var(--mp-text-muted);">
                        <i class="bi bi-shield-check me-1"></i>
                        Токен хранится в Redis и используется только для API-запросов к T-Invest.
                    </div>
                </div>
            </div>
        </div>

        <!-- ============ CONNECTED ============ -->
        <div v-if="!loading && connected">
            <!-- Status bar -->
            <div class="mp-settings-card mb-3">
                <div class="mp-settings-card__body d-flex align-items-center justify-content-between flex-wrap gap-2">
                    <div class="d-flex align-items-center gap-3">
                        <span>
                            <i class="bi bi-circle-fill text-success me-1" style="font-size:0.5rem;"></i>
                            <strong>Подключено</strong>
                        </span>
                        <span class="badge" :class="mode === 'sandbox' ? 'bg-warning text-dark' : 'bg-success'">
                            {{ mode === 'sandbox' ? 'SANDBOX' : 'PROD' }}
                        </span>
                        <span class="text-muted" style="font-size:0.8rem;">
                            Токен: {{ tokenMasked }}
                        </span>
                        <span class="text-muted" style="font-size:0.8rem;">
                            Счетов: {{ accounts.length }}
                        </span>
                        <span v-if="streaming" class="badge bg-info">
                            <i class="bi bi-broadcast me-1"></i>Стрим
                        </span>
                    </div>
                    <button class="btn btn-sm btn-outline-danger" @click="doDisconnect">
                        <i class="bi bi-plug me-1"></i>Отключить
                    </button>
                </div>
            </div>

            <!-- Account selector (when multiple accounts) -->
            <div v-if="accounts.length > 1" class="mb-3">
                <label class="form-label" style="font-size:0.85rem;">Активный счёт</label>
                <select class="form-select" style="max-width:400px;" v-model="selectedAccountId">
                    <option v-for="acc in accounts" :key="acc.id" :value="acc.id">
                        {{ acc.name || acc.id }} — {{ accountTypeLabel(acc.type) }}
                    </option>
                </select>
            </div>

            <!-- Tabs -->
            <ul class="nav nav-tabs mb-3">
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'overview' }"
                       href="#" @click.prevent="activeTab = 'overview'">
                        <i class="bi bi-grid me-1"></i>Обзор
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'portfolio' }"
                       href="#" @click.prevent="activeTab = 'portfolio'">
                        <i class="bi bi-briefcase me-1"></i>Портфель
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'orders' }"
                       href="#" @click.prevent="activeTab = 'orders'">
                        <i class="bi bi-list-check me-1"></i>Ордера
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'broker' }"
                       href="#" @click.prevent="activeTab = 'broker'">
                        <i class="bi bi-wallet2 me-1"></i>Брокер
                    </a>
                </li>
                <li class="nav-item">
                    <a class="nav-link" :class="{ active: activeTab === 'instruments' }"
                       href="#" @click.prevent="activeTab = 'instruments'">
                        <i class="bi bi-search me-1"></i>Инструменты
                    </a>
                </li>
            </ul>

            <!-- ---- TAB: Overview ---- -->
            <div v-if="activeTab === 'overview'">
                <!-- Accounts table -->
                <div class="mp-settings-card mb-3">
                    <div class="mp-settings-card__header">
                        <i class="bi bi-person-badge me-2"></i>Счета и стратегии
                    </div>
                    <div class="mp-settings-card__body">
                        <div class="table-responsive">
                            <table class="mp-table" style="font-size:0.85rem;">
                                <thead>
                                    <tr>
                                        <th>ID</th>
                                        <th>Название</th>
                                        <th>Тип</th>
                                        <th>Статус</th>
                                        <th>Доступ</th>
                                        <th>Стратегия</th>
                                        <th>Активна</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <tr v-for="acc in accounts" :key="acc.id">
                                        <td><code style="font-size:0.75rem;">{{ acc.id }}</code></td>
                                        <td>{{ acc.name || '—' }}</td>
                                        <td>{{ accountTypeLabel(acc.type) }}</td>
                                        <td>
                                            <span class="badge"
                                                  :class="acc.status.indexOf('OPEN') >= 0 ? 'bg-success' : 'bg-secondary'">
                                                {{ statusLabel(acc.status) }}
                                            </span>
                                        </td>
                                        <td>{{ accessLabel(acc.access_level) }}</td>
                                        <td>
                                            <select class="form-select form-select-sm" style="width:160px;"
                                                    :value="getStrategy(acc.id)"
                                                    @change="setStrategy(acc.id, $event.target.value)">
                                                <option value="">— Не назначена —</option>
                                                <option value="news">Новостная</option>
                                                <option value="ta">Технический анализ</option>
                                                <option value="combined">Комбинированная</option>
                                            </select>
                                        </td>
                                        <td class="text-center">
                                            <div class="form-check form-switch d-inline-block">
                                                <input class="form-check-input" type="checkbox"
                                                       :checked="getStrategyEnabled(acc.id)"
                                                       @change="toggleStrategyEnabled(acc.id)"
                                                       :disabled="!getStrategy(acc.id)">
                                            </div>
                                        </td>
                                    </tr>
                                </tbody>
                            </table>
                        </div>
                        <button class="btn btn-sm btn-primary mt-2" @click="saveStrategies"
                                :disabled="savingStrategies">
                            <span v-if="savingStrategies">
                                <span class="spinner-border spinner-border-sm me-1"></span>
                            </span>
                            <i v-else class="bi bi-save me-1"></i>
                            Сохранить стратегии
                        </button>
                    </div>
                </div>

                <!-- Info card -->
                <div class="mp-settings-card">
                    <div class="mp-settings-card__header">
                        <i class="bi bi-info-circle me-2"></i>
                        Как работает торговля
                    </div>
                    <div class="mp-settings-card__body" style="font-size:0.85rem; color:var(--mp-text-secondary);">
                        <ol class="mb-0" style="padding-left:1.2rem;">
                            <li>Назначьте торговую стратегию на каждый счёт и активируйте переключатель</li>
                            <li><strong>Новостная</strong> — торговля по новостным сигналам системы</li>
                            <li><strong>Технический анализ</strong> — торговля по ТА индикаторам</li>
                            <li><strong>Комбинированная</strong> — совмещение новостей и ТА</li>
                            <li>Ордера исполняются автоматически через T-Invest API по сигналам</li>
                            <li>Маржинальная торговля поддерживается (доступна на счетах с плечом)</li>
                        </ol>
                    </div>
                </div>
            </div>

            <!-- ---- TAB: Portfolio ---- -->
            <div v-if="activeTab === 'portfolio'">
                <div v-if="portfolioLoading" class="text-center py-4">
                    <span class="spinner-border spinner-border-sm"></span> Загрузка портфеля...
                </div>
                <div v-if="portfolioError" class="alert alert-danger py-2" style="font-size:0.85rem;">
                    {{ portfolioError }}
                </div>

                <div v-if="portfolio && !portfolioLoading">
                    <!-- Summary cards -->
                    <div class="row g-3 mb-3">
                        <div class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Общая стоимость</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(portfolio.total_amount) }}</div>
                            </div>
                        </div>
                        <div class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Акции</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(portfolio.shares) }}</div>
                            </div>
                        </div>
                        <div class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Облигации</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(portfolio.bonds) }}</div>
                            </div>
                        </div>
                        <div class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Денежные средства</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(portfolio.currencies) }}</div>
                            </div>
                        </div>
                    </div>

                    <!-- Yield + Margin -->
                    <div class="row g-3 mb-3">
                        <div class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Доходность</div>
                                <div class="mp-stat-card__value" :class="yieldClass(portfolio.expected_yield)">
                                    {{ fmtYield(portfolio.expected_yield) }} ₽
                                </div>
                            </div>
                        </div>
                        <div v-if="margin" class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Ликвидный портфель</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(margin.liquid_portfolio) }}</div>
                            </div>
                        </div>
                        <div v-if="margin" class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Начальная маржа</div>
                                <div class="mp-stat-card__value">{{ fmtMoney(margin.starting_margin) }}</div>
                            </div>
                        </div>
                        <div v-if="margin" class="col-6 col-md-3">
                            <div class="mp-stat-card">
                                <div class="mp-stat-card__label">Доступно для торговли</div>
                                <div class="mp-stat-card__value text-success">{{ fmtMoney(margin.funds_available) }}</div>
                            </div>
                        </div>
                    </div>

                    <!-- Positions table -->
                    <div class="mp-settings-card">
                        <div class="mp-settings-card__header d-flex justify-content-between align-items-center">
                            <span><i class="bi bi-list-ul me-2"></i>Позиции ({{ portfolio.positions ? portfolio.positions.length : 0 }})</span>
                            <button class="btn btn-sm btn-outline-secondary" @click="loadPortfolio">
                                <i class="bi bi-arrow-clockwise me-1"></i>Обновить
                            </button>
                        </div>
                        <div class="mp-settings-card__body">
                            <div v-if="!portfolio.positions || portfolio.positions.length === 0"
                                 class="text-muted" style="font-size:0.85rem;">
                                Нет открытых позиций
                            </div>
                            <div v-else class="table-responsive">
                                <table class="mp-table" style="font-size:0.85rem;">
                                    <thead>
                                        <tr>
                                            <th>Тикер</th>
                                            <th>Тип</th>
                                            <th class="text-end">Кол-во</th>
                                            <th class="text-end">Ср. цена</th>
                                            <th class="text-end">Тек. цена</th>
                                            <th class="text-end">P&L</th>
                                            <th class="text-end">Дневной P&L</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr v-for="pos in portfolio.positions" :key="pos.instrument_uid">
                                            <td><strong>{{ pos.ticker || pos.figi || pos.instrument_uid.substring(0,8) }}</strong></td>
                                            <td>{{ pos.type }}</td>
                                            <td class="text-end">{{ pos.quantity }}</td>
                                            <td class="text-end">{{ fmtMoney(pos.avg_price) }}</td>
                                            <td class="text-end">{{ fmtMoney(pos.current_price) }}</td>
                                            <td class="text-end" :class="yieldClass(pos.expected_yield)">
                                                {{ fmtYield(pos.expected_yield) }}
                                            </td>
                                            <td class="text-end" :class="yieldClass(pos.daily_yield)">
                                                {{ fmtYield(pos.daily_yield) }}
                                            </td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- ---- TAB: Orders ---- -->
            <div v-if="activeTab === 'orders'">
                <div v-if="ordersLoading" class="text-center py-4">
                    <span class="spinner-border spinner-border-sm"></span> Загрузка ордеров...
                </div>
                <div v-if="ordersError" class="alert alert-danger py-2" style="font-size:0.85rem;">
                    {{ ordersError }}
                </div>

                <div class="mp-settings-card">
                    <div class="mp-settings-card__header d-flex justify-content-between align-items-center">
                        <span><i class="bi bi-list-check me-2"></i>Активные ордера ({{ activeOrders.length }})</span>
                        <button class="btn btn-sm btn-outline-secondary" @click="loadOrders">
                            <i class="bi bi-arrow-clockwise me-1"></i>Обновить
                        </button>
                    </div>
                    <div class="mp-settings-card__body">
                        <div v-if="activeOrders.length === 0 && !ordersLoading"
                             class="text-muted" style="font-size:0.85rem;">
                            Нет активных ордеров
                        </div>
                        <div v-else class="table-responsive">
                            <table class="mp-table" style="font-size:0.85rem;">
                                <thead>
                                    <tr>
                                        <th>ID ордера</th>
                                        <th>Направление</th>
                                        <th>Тип</th>
                                        <th class="text-end">Лоты</th>
                                        <th class="text-end">Исполнено</th>
                                        <th class="text-end">Цена</th>
                                        <th>Статус</th>
                                        <th class="text-end">Действие</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <tr v-for="o in activeOrders" :key="o.order_id">
                                        <td><code style="font-size:0.75rem;">{{ o.order_id.substring(0, 12) }}...</code></td>
                                        <td>
                                            <span :class="o.direction.indexOf('BUY') >= 0 ? 'text-success' : 'text-danger'">
                                                {{ directionLabel(o.direction) }}
                                            </span>
                                        </td>
                                        <td>{{ o.order_type }}</td>
                                        <td class="text-end">{{ o.lots_total }}</td>
                                        <td class="text-end">{{ o.lots_executed }}</td>
                                        <td class="text-end">{{ o.price ? fmtMoney(o.price) : 'Рыночная' }}</td>
                                        <td>{{ orderStatusLabel(o.status) }}</td>
                                        <td class="text-end">
                                            <button class="btn btn-sm btn-outline-danger" @click="cancelOrder(o.order_id)">
                                                <i class="bi bi-x-circle"></i>
                                            </button>
                                        </td>
                                    </tr>
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
            </div>

            <!-- ---- TAB: Broker ---- -->
            <div v-if="activeTab === 'broker'">
                <div v-if="brokerLoading" class="text-center py-4">
                    <span class="spinner-border spinner-border-sm"></span> Загрузка данных брокера...
                </div>

                <!-- Broker accounts with real portfolio data -->
                <div v-if="brokerPortfolios && brokerPortfolios.accounts && brokerPortfolios.accounts.length > 0">
                    <div v-for="acc in brokerPortfolios.accounts" :key="acc.account_id" class="mp-settings-card mb-3">
                        <div class="mp-settings-card__header d-flex justify-content-between align-items-center">
                            <span>
                                <i class="bi bi-wallet2 me-2"></i>
                                {{ acc.account_name || acc.account_id }}
                                <span v-if="acc.strategy" class="badge bg-primary ms-2">{{ acc.strategy }}</span>
                            </span>
                            <span class="text-muted" style="font-size:0.75rem;">
                                Обновлено: {{ new Date(acc.updated_at).toLocaleTimeString('ru-RU') }}
                            </span>
                        </div>
                        <div class="mp-settings-card__body">
                            <div class="row g-3 mb-3">
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Стоимость портфеля</div>
                                        <div class="mp-stat-card__value">{{ fmtMoney(acc.total_amount) }}</div>
                                    </div>
                                </div>
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Акции</div>
                                        <div class="mp-stat-card__value">{{ fmtMoney(acc.shares) }}</div>
                                    </div>
                                </div>
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Денежные средства</div>
                                        <div class="mp-stat-card__value">{{ fmtMoney(acc.currencies) }}</div>
                                    </div>
                                </div>
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Доходность</div>
                                        <div class="mp-stat-card__value" :class="yieldClass(acc.expected_yield)">
                                            {{ fmtYield(acc.expected_yield) }} ₽
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Margin info -->
                            <div v-if="acc.margin" class="row g-3 mb-3">
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Ликвидный портфель</div>
                                        <div class="mp-stat-card__value">{{ fmtMoney(acc.margin.liquid_portfolio) }}</div>
                                    </div>
                                </div>
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Начальная маржа</div>
                                        <div class="mp-stat-card__value">{{ fmtMoney(acc.margin.starting_margin) }}</div>
                                    </div>
                                </div>
                                <div class="col-6 col-md-3">
                                    <div class="mp-stat-card">
                                        <div class="mp-stat-card__label">Доступно для торговли</div>
                                        <div class="mp-stat-card__value text-success">{{ fmtMoney(acc.margin.funds_available) }}</div>
                                    </div>
                                </div>
                            </div>

                            <!-- Positions -->
                            <div v-if="acc.positions && acc.positions.length > 0" class="table-responsive">
                                <table class="mp-table" style="font-size:0.82rem;">
                                    <thead>
                                        <tr>
                                            <th>Тикер</th>
                                            <th>Тип</th>
                                            <th class="text-end">Кол-во</th>
                                            <th class="text-end">Ср. цена</th>
                                            <th class="text-end">Тек. цена</th>
                                            <th class="text-end">P&L</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr v-for="pos in acc.positions" :key="pos.instrument_uid">
                                            <td><strong>{{ pos.ticker || pos.figi || '—' }}</strong></td>
                                            <td>{{ pos.type }}</td>
                                            <td class="text-end">{{ pos.quantity }}</td>
                                            <td class="text-end">{{ fmtMoney(pos.avg_price) }}</td>
                                            <td class="text-end">{{ fmtMoney(pos.current_price) }}</td>
                                            <td class="text-end" :class="yieldClass(pos.expected_yield)">
                                                {{ fmtYield(pos.expected_yield) }}
                                            </td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                            <div v-else class="text-muted" style="font-size:0.85rem;">Нет открытых позиций</div>
                        </div>
                    </div>
                </div>

                <div v-if="!brokerLoading && (!brokerPortfolios || !brokerPortfolios.accounts || brokerPortfolios.accounts.length === 0)"
                     class="text-muted mb-3" style="font-size:0.85rem;">
                    Данные портфеля брокера пока не загружены. Они обновляются автоматически каждые 30 секунд.
                </div>

                <button class="btn btn-sm btn-outline-secondary mb-3" @click="loadBrokerPortfolios">
                    <i class="bi bi-arrow-clockwise me-1"></i>Обновить
                </button>

                <!-- Withdrawal configuration -->
                <div class="mp-settings-card mt-3">
                    <div class="mp-settings-card__header">
                        <i class="bi bi-cash-stack me-2"></i>
                        Настройка вывода прибыли
                    </div>
                    <div class="mp-settings-card__body">
                        <div class="form-check mb-3">
                            <input class="form-check-input" type="checkbox" id="withdrawEnabled"
                                   v-model="withdrawal.enabled">
                            <label class="form-check-label" for="withdrawEnabled" style="font-size:0.85rem;">
                                Включить автоматический вывод прибыли
                            </label>
                        </div>

                        <div class="row g-3 mb-3" v-if="withdrawal.enabled">
                            <div class="col-md-3">
                                <label class="form-label" style="font-size:0.85rem;">Процент от прибыли</label>
                                <div class="input-group">
                                    <input type="number" class="form-control" v-model.number="withdrawal.profit_percent"
                                           min="1" max="100" step="1">
                                    <span class="input-group-text">%</span>
                                </div>
                            </div>
                            <div class="col-md-3">
                                <label class="form-label" style="font-size:0.85rem;">Мин. прибыль (₽)</label>
                                <input type="number" class="form-control" v-model.number="withdrawal.min_profit"
                                       min="0" step="100">
                            </div>
                            <div class="col-md-3">
                                <label class="form-label" style="font-size:0.85rem;">День недели</label>
                                <select class="form-select" v-model.number="withdrawal.day_of_week">
                                    <option :value="1">Понедельник</option>
                                    <option :value="2">Вторник</option>
                                    <option :value="3">Среда</option>
                                    <option :value="4">Четверг</option>
                                    <option :value="5">Пятница</option>
                                </select>
                            </div>
                            <div class="col-md-3" v-if="accounts.length > 0">
                                <label class="form-label" style="font-size:0.85rem;">Счёт для вывода</label>
                                <select class="form-select" v-model="withdrawal.account_id">
                                    <option value="">— Не выбран —</option>
                                    <option v-for="acc in accounts" :key="acc.id" :value="acc.id">
                                        {{ acc.name || acc.id }}
                                    </option>
                                </select>
                            </div>
                        </div>

                        <button class="btn btn-sm btn-primary" @click="saveWithdrawalConfig"
                                :disabled="savingWithdrawal">
                            <span v-if="savingWithdrawal">
                                <span class="spinner-border spinner-border-sm me-1"></span>
                            </span>
                            <i v-else class="bi bi-save me-1"></i>
                            Сохранить
                        </button>
                        <span v-if="withdrawalSaved" class="text-success ms-2" style="font-size:0.85rem;">
                            <i class="bi bi-check-circle me-1"></i>Сохранено
                        </span>

                        <p class="text-muted mt-3 mb-0" style="font-size:0.78rem;">
                            <i class="bi bi-info-circle me-1"></i>
                            Система будет выводить указанный процент от чистой прибыли еженедельно в выбранный день.
                            Вывод произойдёт только если прибыль превышает минимальный порог.
                        </p>
                    </div>
                </div>
            </div>

            <!-- ---- TAB: Instruments ---- -->
            <div v-if="activeTab === 'instruments'">
                <div v-if="instrLoading" class="text-center py-4">
                    <span class="spinner-border spinner-border-sm"></span> Загрузка инструментов...
                </div>
                <div v-if="instrError" class="alert alert-danger py-2" style="font-size:0.85rem;">
                    {{ instrError }}
                </div>

                <div class="mp-settings-card">
                    <div class="mp-settings-card__header d-flex justify-content-between align-items-center">
                        <span><i class="bi bi-search me-2"></i>Инструменты ({{ instruments.length }})</span>
                        <button class="btn btn-sm btn-outline-secondary" @click="loadInstruments">
                            <i class="bi bi-arrow-clockwise me-1"></i>Обновить
                        </button>
                    </div>
                    <div class="mp-settings-card__body">
                        <div class="mb-3" style="max-width:400px;">
                            <input type="text" class="form-control form-control-sm"
                                   v-model="instrSearch"
                                   placeholder="Поиск по тикеру или названию...">
                        </div>
                        <div v-if="instruments.length > 0" class="table-responsive" style="max-height:500px; overflow-y:auto;">
                            <table class="mp-table" style="font-size:0.82rem;">
                                <thead>
                                    <tr>
                                        <th>Тикер</th>
                                        <th>Название</th>
                                        <th>Сектор</th>
                                        <th>Валюта</th>
                                        <th class="text-end">Лот</th>
                                        <th class="text-center">API-торговля</th>
                                        <th class="text-center">Шорт</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <tr v-for="inst in filteredInstruments" :key="inst.uid">
                                        <td><strong>{{ inst.ticker }}</strong></td>
                                        <td>{{ inst.name }}</td>
                                        <td>{{ inst.sector }}</td>
                                        <td>{{ inst.currency }}</td>
                                        <td class="text-end">{{ inst.lot }}</td>
                                        <td class="text-center">
                                            <i class="bi" :class="inst.api_trade_available ? 'bi-check-circle text-success' : 'bi-x-circle text-muted'"></i>
                                        </td>
                                        <td class="text-center">
                                            <i class="bi" :class="inst.short_enabled ? 'bi-check-circle text-success' : 'bi-x-circle text-muted'"></i>
                                        </td>
                                    </tr>
                                </tbody>
                            </table>
                        </div>
                        <div v-if="!instrLoading && instruments.length === 0" class="text-muted" style="font-size:0.85rem;">
                            Инструменты не загружены. Нажмите "Обновить" для загрузки.
                        </div>
                        <div v-if="filteredInstruments.length >= 100" class="text-muted mt-2" style="font-size:0.78rem;">
                            Показаны первые 100 результатов. Используйте поиск для уточнения.
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
`
    };
})();
