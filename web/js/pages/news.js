/* ============================================================
   MarketPulse_RU — News Page
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var computed  = Vue.computed;
    var watch     = Vue.watch;
    var onMounted = Vue.onMounted;

    var SOURCE_LABELS = {
        telegram: 'Telegram',
        rss:      'RSS',
        web:      'Web',
        api:      'API'
    };

    var CATEGORY_LABELS = {
        'CORP_EARNINGS':    'Отчётность',
        'CORP_DIVIDEND':    'Дивиденды',
        'CORP_MA':          'M&A',
        'CORP_MANAGEMENT':  'Менеджмент',
        'CORP_LEGAL':       'Юридическое',
        'CORP_DEBT':        'Долг',
        'CORP_RATING':      'Рейтинг',
        'CB_RATE':          'Ставка ЦБ',
        'CB_POLICY':        'Политика ЦБ',
        'MACRO_INFLATION':  'Инфляция',
        'MACRO_GDP':        'ВВП',
        'MACRO_EMPLOYMENT': 'Занятость',
        'GEO_SANCTIONS':    'Санкции',
        'GEO_DIPLOMACY':    'Дипломатия',
        'GEO_CONFLICT':     'Конфликт',
        'GEO_TRADE':        'Торговля',
        'COMMODITY_OIL':    'Нефть',
        'COMMODITY_GAS':    'Газ',
        'COMMODITY_METAL':  'Металлы',
        'COMMODITY_AGRO':   'Агро',
        'REGULATION':       'Регулирование',
        'WEATHER':          'Погода',
        'TECH':             'Технологии'
    };

    window.PageNews = {
        name: 'PageNews',
        setup: function () {
            var newsItems       = ref([]);
            var loading         = ref(true);
            var loadingMore     = ref(false);
            var hasMore         = ref(true);
            var pageSize        = 20;
            var offset          = ref(0);
            var expandedIds     = ref({});

            // Filters
            var searchQuery     = ref('');
            var tickerQuery     = ref('');
            var selectedCategory = ref('');
            var showRelated     = ref(false);
            var categories      = ref([]);

            // Debounce timer for text inputs
            var debounceTimer   = null;

            function toggleExpand(id) {
                var copy = Object.assign({}, expandedIds.value);
                if (copy[id]) {
                    delete copy[id];
                } else {
                    copy[id] = true;
                }
                expandedIds.value = copy;
            }

            function isExpanded(id) {
                return !!expandedIds.value[id];
            }

            function fmtTime(ts) {
                if (!ts) return '';
                var d = new Date(ts);
                var now = new Date();
                var diff = now - d;
                if (diff < 3600000) {
                    var mins = Math.floor(diff / 60000);
                    return mins <= 1 ? 'Только что' : mins + ' мин. назад';
                }
                if (diff < 86400000) {
                    var hours = Math.floor(diff / 3600000);
                    return hours + ' ч. назад';
                }
                return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' });
            }

            function sourceLabel(src) {
                return SOURCE_LABELS[src] || src || 'N/A';
            }

            function categoryLabel(cat) {
                return CATEGORY_LABELS[cat] || cat || '';
            }

            function sentimentClass(item) {
                if (item.sentiment !== undefined) {
                    if (item.sentiment > 0.1) return 'mp-sentiment-dot--positive';
                    if (item.sentiment < -0.1) return 'mp-sentiment-dot--negative';
                }
                return 'mp-sentiment-dot--neutral';
            }

            function sentimentText(item) {
                if (item.sentiment === undefined || item.sentiment === 0) return '';
                var v = item.sentiment.toFixed(2);
                return v > 0 ? '+' + v : v;
            }

            function getExcerpt(content, maxLen) {
                if (!content) return '';
                if (content.length <= maxLen) return content;
                return content.substring(0, maxLen) + '...';
            }

            function getTitle(item) {
                if (item.title) return item.title;
                if (item.content) return item.content.substring(0, 80) + (item.content.length > 80 ? '...' : '');
                return 'Без заголовка';
            }

            function hasLongContent(item) {
                return item.content && item.content.length > 200;
            }

            async function fetchCategories() {
                try {
                    var data = await API.getNewsCategories();
                    categories.value = Array.isArray(data) ? data : [];
                } catch (err) {
                    categories.value = [];
                }
            }

            async function fetchNews(append) {
                if (append) {
                    loadingMore.value = true;
                } else {
                    loading.value = true;
                }
                try {
                    var opts = {};
                    if (selectedCategory.value) opts.category = selectedCategory.value;
                    if (tickerQuery.value.trim()) opts.ticker = tickerQuery.value.trim();
                    if (searchQuery.value.trim()) opts.q = searchQuery.value.trim();
                    if (showRelated.value && tickerQuery.value.trim()) opts.related = true;

                    var data = await API.getNews(pageSize, offset.value, opts);
                    var items = Array.isArray(data) ? data : [];
                    if (append) {
                        newsItems.value = newsItems.value.concat(items);
                    } else {
                        newsItems.value = items;
                    }
                    hasMore.value = items.length >= pageSize;
                } catch (err) {
                    if (!append) newsItems.value = [];
                    hasMore.value = false;
                }
                loading.value = false;
                loadingMore.value = false;
            }

            function resetAndFetch() {
                offset.value = 0;
                fetchNews(false);
            }

            function loadMore() {
                offset.value += pageSize;
                fetchNews(true);
            }

            function onFilterChange() {
                resetAndFetch();
            }

            function onTextInput() {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(function () {
                    resetAndFetch();
                }, 400);
            }

            function clearFilters() {
                searchQuery.value = '';
                tickerQuery.value = '';
                selectedCategory.value = '';
                showRelated.value = false;
                resetAndFetch();
            }

            var hasActiveFilters = computed(function () {
                return !!(searchQuery.value.trim() || tickerQuery.value.trim() || selectedCategory.value);
            });

            onMounted(function () {
                fetchCategories();
                fetchNews(false);
            });

            return {
                newsItems: newsItems,
                loading: loading,
                loadingMore: loadingMore,
                hasMore: hasMore,
                searchQuery: searchQuery,
                tickerQuery: tickerQuery,
                selectedCategory: selectedCategory,
                showRelated: showRelated,
                categories: categories,
                hasActiveFilters: hasActiveFilters,
                expandedIds: expandedIds,
                toggleExpand: toggleExpand,
                isExpanded: isExpanded,
                fmtTime: fmtTime,
                sourceLabel: sourceLabel,
                categoryLabel: categoryLabel,
                sentimentClass: sentimentClass,
                sentimentText: sentimentText,
                getExcerpt: getExcerpt,
                getTitle: getTitle,
                hasLongContent: hasLongContent,
                loadMore: loadMore,
                onFilterChange: onFilterChange,
                onTextInput: onTextInput,
                clearFilters: clearFilters
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <div class="d-flex align-items-center justify-content-between flex-wrap gap-2">
            <h1><i class="bi bi-newspaper me-2"></i>Новости</h1>
        </div>

        <!-- Filter bar -->
        <div class="mp-news-filters mt-2">
            <div class="row g-2 align-items-end">
                <!-- Text search -->
                <div class="col-md-3 col-sm-6">
                    <label class="form-label mb-0" style="font-size:0.7rem;color:var(--mp-text-muted);">Поиск по тексту</label>
                    <input type="text" class="form-control form-control-sm mp-search-input"
                           placeholder="Ключевые слова..."
                           v-model="searchQuery"
                           @input="onTextInput">
                </div>

                <!-- Ticker / company search -->
                <div class="col-md-3 col-sm-6">
                    <label class="form-label mb-0" style="font-size:0.7rem;color:var(--mp-text-muted);">Компания / тикер</label>
                    <input type="text" class="form-control form-control-sm mp-search-input"
                           placeholder="SBER, Газпром..."
                           v-model="tickerQuery"
                           @input="onTextInput">
                </div>

                <!-- Category filter -->
                <div class="col-md-3 col-sm-6">
                    <label class="form-label mb-0" style="font-size:0.7rem;color:var(--mp-text-muted);">Категория</label>
                    <select class="form-select form-select-sm mp-search-input"
                            v-model="selectedCategory"
                            @change="onFilterChange">
                        <option value="">Все категории</option>
                        <option v-for="cat in categories" :key="cat" :value="cat">
                            {{ categoryLabel(cat) || cat }}
                        </option>
                    </select>
                </div>

                <!-- Related news checkbox + clear -->
                <div class="col-md-3 col-sm-6 d-flex align-items-center gap-3" style="min-height:31px;">
                    <div class="form-check mb-0" v-if="tickerQuery.trim()">
                        <input class="form-check-input" type="checkbox" id="chkRelated"
                               v-model="showRelated" @change="onFilterChange">
                        <label class="form-check-label" for="chkRelated" style="font-size:0.75rem;">
                            Связанные новости
                        </label>
                    </div>
                    <button v-if="hasActiveFilters" class="btn btn-sm btn-outline-secondary"
                            @click="clearFilters" style="font-size:0.7rem;">
                        <i class="bi bi-x-circle me-1"></i>Сбросить
                    </button>
                </div>
            </div>
        </div>

        <div class="mt-2 text-muted" style="font-size:0.8rem;">
            Загружено: {{ newsItems.length }} новостей
            <span v-if="tickerQuery.trim() && showRelated" class="ms-2">
                <i class="bi bi-diagram-3 me-1"></i>включая связанные
            </span>
        </div>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка новостей...
        </div>

        <div v-else-if="newsItems.length === 0" class="mp-empty">
            <i class="bi bi-newspaper"></i>
            <div v-if="hasActiveFilters">Ничего не найдено по заданным фильтрам</div>
            <div v-else>Нет новостей</div>
            <small class="text-muted">Новости появятся после запуска коллектора (Telegram / RSS)</small>
        </div>

        <div v-else>
            <div v-for="item in newsItems" :key="item.id"
                 class="mp-news-card"
                 :class="{ 'mp-news-card--expanded': isExpanded(item.id) }"
                 @click="toggleExpand(item.id)"
                 style="cursor:pointer;">
                <div class="d-flex align-items-start justify-content-between">
                    <div class="flex-grow-1">
                        <div class="mp-news-card__title">
                            <span class="mp-sentiment-dot" :class="sentimentClass(item)"></span>
                            {{ getTitle(item) }}
                            <i v-if="hasLongContent(item)"
                               class="bi ms-1"
                               :class="isExpanded(item.id) ? 'bi-chevron-up' : 'bi-chevron-down'"
                               style="font-size:0.75rem; color:var(--mp-text-muted);"></i>
                        </div>
                        <div class="mp-news-card__meta">
                            <span class="badge badge-news me-1" style="font-size:0.6rem;">{{ sourceLabel(item.source) }}</span>
                            <span v-if="item.category" class="badge badge-ta me-1" style="font-size:0.6rem;">{{ categoryLabel(item.category) }}</span>
                            <span v-if="item.analyzed" class="badge me-1" style="font-size:0.55rem; background:var(--mp-accent); color:#fff;">LLM</span>
                            <span v-if="sentimentText(item)" class="me-1" style="font-size:0.7rem;"
                                  :style="{ color: item.sentiment > 0 ? 'var(--mp-green)' : item.sentiment < 0 ? 'var(--mp-red)' : 'var(--mp-text-muted)' }">
                                {{ sentimentText(item) }}
                            </span>
                            <span v-if="item.source_channel" class="me-1">{{ item.source_channel }}</span>
                            <span>{{ fmtTime(item.published_at || item.collected_at) }}</span>
                        </div>

                        <!-- Collapsed: show excerpt -->
                        <div class="mp-news-card__content" v-if="item.content && !isExpanded(item.id)">
                            {{ getExcerpt(item.content, 200) }}
                        </div>

                        <!-- Expanded: show summary if available -->
                        <div v-if="isExpanded(item.id) && item.summary_ru" class="mp-news-card__summary">
                            <strong style="font-size:0.7rem; color:var(--mp-text-muted);">Краткое содержание (LLM):</strong><br>
                            {{ item.summary_ru }}
                        </div>

                        <!-- Expanded: show full content -->
                        <div class="mp-news-card__body" v-if="isExpanded(item.id) && item.content">
                            <pre class="mp-news-card__fulltext">{{ item.content }}</pre>
                        </div>

                        <!-- Expanded: show link to original -->
                        <div v-if="isExpanded(item.id) && item.url" class="mt-2">
                            <a :href="item.url" target="_blank" rel="noopener" class="mp-news-card__link"
                               @click.stop>
                                <i class="bi bi-box-arrow-up-right me-1"></i>Открыть оригинал
                            </a>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Load more -->
            <div v-if="hasMore" class="text-center mt-3">
                <button class="btn btn-outline-primary" @click.stop="loadMore" :disabled="loadingMore">
                    <span v-if="loadingMore">
                        <span class="spinner-border spinner-border-sm me-1"></span> Загрузка...
                    </span>
                    <span v-else>
                        <i class="bi bi-arrow-down-circle me-1"></i> Загрузить ещё
                    </span>
                </button>
            </div>
        </div>
    </div>
</div>
`
    };
})();
