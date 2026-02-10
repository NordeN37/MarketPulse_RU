/* LLM Stats — admin page showing model usage, costs, and status */
(function () {
    'use strict';

    var ref = Vue.ref;
    var onMounted = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var computed = Vue.computed;

    window.PageLLMStats = {
        name: 'PageLLMStats',
        setup: function () {
            var stats = ref(null);
            var loading = ref(true);
            var error = ref('');
            var timer = null;

            function statusBadge(status) {
                if (status === 'active') return 'bg-success';
                if (status === 'quota_exhausted') return 'bg-danger';
                return 'bg-secondary';
            }

            function statusLabel(status) {
                if (status === 'active') return 'Активна';
                if (status === 'quota_exhausted') return 'Квота исчерпана';
                return status;
            }

            function formatTokens(n) {
                if (!n) return '0';
                if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
                if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
                return n.toString();
            }

            function formatCost(usd) {
                if (!usd) return '$0.00';
                return '$' + usd.toFixed(4);
            }

            function formatLatency(ms) {
                if (!ms) return '-';
                if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
                return Math.round(ms) + 'ms';
            }

            function timeAgo(isoStr) {
                if (!isoStr) return 'нет данных';
                var diff = Math.floor((Date.now() - new Date(isoStr).getTime()) / 1000);
                if (diff < 60) return diff + ' сек назад';
                if (diff < 3600) return Math.floor(diff / 60) + ' мин назад';
                return Math.floor(diff / 3600) + ' ч назад';
            }

            var modeLabel = computed(function () {
                if (!stats.value) return '-';
                return stats.value.mode === 'batch' ? 'Batch (API)' : 'Стандартный';
            });

            var activeModels = computed(function () {
                if (!stats.value || !stats.value.models) return 0;
                return stats.value.models.filter(function (m) { return m.status === 'active'; }).length;
            });

            var successRate = computed(function () {
                if (!stats.value || !stats.value.total_requests) return '-';
                var rate = ((stats.value.total_requests - stats.value.total_errors) / stats.value.total_requests * 100);
                return rate.toFixed(1) + '%';
            });

            async function loadStats() {
                try {
                    stats.value = await API.getLLMStats();
                    error.value = '';
                } catch (e) {
                    error.value = e.message;
                } finally {
                    loading.value = false;
                }
            }

            onMounted(function () {
                loadStats();
                timer = setInterval(loadStats, 15000);
            });

            onBeforeUnmount(function () {
                if (timer) clearInterval(timer);
            });

            return {
                stats: stats,
                loading: loading,
                error: error,
                modeLabel: modeLabel,
                activeModels: activeModels,
                successRate: successRate,
                statusBadge: statusBadge,
                statusLabel: statusLabel,
                formatTokens: formatTokens,
                formatCost: formatCost,
                formatLatency: formatLatency,
                timeAgo: timeAgo
            };
        },
        template: `
<div class="container-fluid py-3">
    <h4 class="mb-3"><i class="bi bi-cpu me-2"></i>LLM Провайдеры</h4>

    <div v-if="loading" class="text-center py-5">
        <div class="spinner-border text-primary"></div>
    </div>

    <div v-else-if="error" class="alert alert-danger">{{ error }}</div>

    <template v-else>
        <!-- Summary cards -->
        <div class="row g-3 mb-4">
            <div class="col-6 col-md-3">
                <div class="card bg-dark border-secondary text-center p-3">
                    <div class="text-muted small">Режим</div>
                    <div class="fs-5 fw-bold">{{ modeLabel }}</div>
                </div>
            </div>
            <div class="col-6 col-md-3">
                <div class="card bg-dark border-secondary text-center p-3">
                    <div class="text-muted small">Запросов</div>
                    <div class="fs-5 fw-bold">{{ stats ? stats.total_requests : 0 }}</div>
                </div>
            </div>
            <div class="col-6 col-md-3">
                <div class="card bg-dark border-secondary text-center p-3">
                    <div class="text-muted small">Токенов</div>
                    <div class="fs-5 fw-bold">{{ stats ? formatTokens(stats.total_tokens) : 0 }}</div>
                </div>
            </div>
            <div class="col-6 col-md-3">
                <div class="card bg-dark border-secondary text-center p-3">
                    <div class="text-muted small">Стоимость</div>
                    <div class="fs-5 fw-bold text-warning">{{ stats ? formatCost(stats.total_cost_usd) : '$0' }}</div>
                </div>
            </div>
        </div>

        <!-- Secondary stats row -->
        <div class="row g-3 mb-4">
            <div class="col-4">
                <div class="card bg-dark border-secondary text-center p-2">
                    <div class="text-muted small">Активных моделей</div>
                    <div class="fw-bold">{{ activeModels }}</div>
                </div>
            </div>
            <div class="col-4">
                <div class="card bg-dark border-secondary text-center p-2">
                    <div class="text-muted small">Успешность</div>
                    <div class="fw-bold text-success">{{ successRate }}</div>
                </div>
            </div>
            <div class="col-4">
                <div class="card bg-dark border-secondary text-center p-2">
                    <div class="text-muted small">Обновлено</div>
                    <div class="fw-bold">{{ stats ? timeAgo(stats.updated_at) : '-' }}</div>
                </div>
            </div>
        </div>

        <!-- Models table -->
        <div class="card bg-dark border-secondary">
            <div class="card-header border-secondary">
                <h6 class="mb-0">Модели</h6>
            </div>
            <div class="table-responsive">
                <table class="table table-dark table-hover mb-0">
                    <thead>
                        <tr>
                            <th>Модель</th>
                            <th>Статус</th>
                            <th class="text-end">Запросы</th>
                            <th class="text-end">Ошибки</th>
                            <th class="text-end">Input</th>
                            <th class="text-end">Output</th>
                            <th class="text-end">Всего</th>
                            <th class="text-end">Ср. время</th>
                            <th class="text-end">Стоимость</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr v-if="!stats || !stats.models || stats.models.length === 0">
                            <td colspan="9" class="text-center text-muted py-4">
                                Анализатор ещё не запущен или нет данных
                            </td>
                        </tr>
                        <tr v-for="m in (stats ? stats.models : [])" :key="m.model"
                            :class="{ 'text-muted': m.status === 'quota_exhausted' }">
                            <td>
                                <strong>{{ m.model }}</strong>
                                <br><small class="text-muted">{{ m.provider }}</small>
                            </td>
                            <td>
                                <span class="badge" :class="statusBadge(m.status)">
                                    {{ statusLabel(m.status) }}
                                </span>
                            </td>
                            <td class="text-end">{{ m.requests }}</td>
                            <td class="text-end">
                                <span :class="m.errors > 0 ? 'text-danger' : ''">{{ m.errors }}</span>
                            </td>
                            <td class="text-end">{{ formatTokens(m.prompt_tokens) }}</td>
                            <td class="text-end">{{ formatTokens(m.completion_tokens) }}</td>
                            <td class="text-end fw-bold">{{ formatTokens(m.total_tokens) }}</td>
                            <td class="text-end">{{ formatLatency(m.avg_latency_ms) }}</td>
                            <td class="text-end text-warning">{{ formatCost(m.estimated_cost_usd) }}</td>
                        </tr>
                    </tbody>
                </table>
            </div>
        </div>

        <!-- Pricing reference -->
        <div class="card bg-dark border-secondary mt-3">
            <div class="card-header border-secondary">
                <h6 class="mb-0">Тарифы (за 1M токенов)</h6>
            </div>
            <div class="table-responsive">
                <table class="table table-dark table-sm mb-0">
                    <thead>
                        <tr>
                            <th>Модель</th>
                            <th class="text-end">Input</th>
                            <th class="text-end">Output</th>
                            <th>Примечание</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>qwen3-max</td>
                            <td class="text-end">$2.00</td>
                            <td class="text-end">$8.00</td>
                            <td><span class="badge bg-success">1M бесплатно</span></td>
                        </tr>
                        <tr>
                            <td>qwen-plus</td>
                            <td class="text-end">$0.40</td>
                            <td class="text-end">$1.20</td>
                            <td>Платный (Free Quota Off)</td>
                        </tr>
                        <tr>
                            <td>deepseek-chat</td>
                            <td class="text-end">$0.27</td>
                            <td class="text-end">$1.10</td>
                            <td>Запасной</td>
                        </tr>
                        <tr>
                            <td>Ollama (qwen3:8b/14b)</td>
                            <td class="text-end">$0</td>
                            <td class="text-end">$0</td>
                            <td>Локально</td>
                        </tr>
                    </tbody>
                </table>
            </div>
        </div>
    </template>
</div>`
    };
})();
