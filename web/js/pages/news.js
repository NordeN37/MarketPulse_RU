/* ============================================================
   MarketPulse_RU — News Page
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var computed  = Vue.computed;
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
            var newsItems    = ref([]);
            var loading      = ref(true);
            var loadingMore  = ref(false);
            var hasMore      = ref(true);
            var pageSize     = 20;
            var offset       = ref(0);
            var searchQuery  = ref('');

            var filteredNews = computed(function () {
                var q = searchQuery.value.toLowerCase().trim();
                if (!q) return newsItems.value;
                return newsItems.value.filter(function (n) {
                    var text = ((n.title || '') + ' ' + (n.content || '') + ' ' + (n.source_channel || '')).toLowerCase();
                    return text.indexOf(q) >= 0;
                });
            });

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
                /* Try to detect sentiment from analysis if attached, otherwise neutral */
                if (item.sentiment !== undefined) {
                    if (item.sentiment > 0.1) return 'mp-sentiment-dot--positive';
                    if (item.sentiment < -0.1) return 'mp-sentiment-dot--negative';
                }
                return 'mp-sentiment-dot--neutral';
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

            async function fetchNews(append) {
                if (append) {
                    loadingMore.value = true;
                } else {
                    loading.value = true;
                }
                try {
                    var data = await API.getNews(pageSize, offset.value);
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

            function loadMore() {
                offset.value += pageSize;
                fetchNews(true);
            }

            onMounted(function () { fetchNews(false); });

            return {
                newsItems: newsItems,
                filteredNews: filteredNews,
                loading: loading,
                loadingMore: loadingMore,
                hasMore: hasMore,
                searchQuery: searchQuery,
                fmtTime: fmtTime,
                sourceLabel: sourceLabel,
                categoryLabel: categoryLabel,
                sentimentClass: sentimentClass,
                getExcerpt: getExcerpt,
                getTitle: getTitle,
                loadMore: loadMore
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <div class="d-flex align-items-center justify-content-between flex-wrap gap-2">
            <h1><i class="bi bi-newspaper me-2"></i>Новости</h1>
            <div style="max-width:300px;width:100%;">
                <input type="text" class="form-control form-control-sm mp-search-input"
                       placeholder="Поиск в новостях..."
                       v-model="searchQuery">
            </div>
        </div>
        <div class="mt-2 text-muted" style="font-size:0.8rem;">
            Загружено: {{ newsItems.length }} новостей
        </div>
    </div>

    <div class="mp-page-content">
        <div v-if="loading" class="mp-loading">
            <div class="spinner-border spinner-border-sm text-accent me-2"></div>
            Загрузка новостей...
        </div>

        <div v-else-if="filteredNews.length === 0" class="mp-empty">
            <i class="bi bi-newspaper"></i>
            <div v-if="searchQuery">Ничего не найдено по запросу "{{ searchQuery }}"</div>
            <div v-else>Нет новостей</div>
            <small class="text-muted">Новости появятся после запуска коллектора (Telegram / RSS)</small>
        </div>

        <div v-else>
            <div v-for="item in filteredNews" :key="item.id" class="mp-news-card">
                <div class="d-flex align-items-start justify-content-between">
                    <div class="flex-grow-1">
                        <div class="mp-news-card__title">
                            <span class="mp-sentiment-dot" :class="sentimentClass(item)"></span>
                            {{ getTitle(item) }}
                        </div>
                        <div class="mp-news-card__meta">
                            <span class="badge badge-news me-1" style="font-size:0.6rem;">{{ sourceLabel(item.source) }}</span>
                            <span v-if="item.category" class="badge badge-ta me-1" style="font-size:0.6rem;">{{ categoryLabel(item.category) }}</span>
                            <span v-if="item.source_channel" class="me-1">{{ item.source_channel }}</span>
                            <span>{{ fmtTime(item.published_at || item.collected_at) }}</span>
                        </div>
                        <div class="mp-news-card__content" v-if="item.content">
                            {{ getExcerpt(item.content, 200) }}
                        </div>
                    </div>
                </div>
            </div>

            <!-- Load more -->
            <div v-if="hasMore && !searchQuery" class="text-center mt-3">
                <button class="btn btn-outline-primary" @click="loadMore" :disabled="loadingMore">
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
