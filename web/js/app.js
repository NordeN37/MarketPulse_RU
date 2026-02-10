/* ============================================================
   MarketPulse_RU — Main Vue 3 App
   ============================================================ */
(function () {
    'use strict';

    var createApp  = Vue.createApp;
    var ref        = Vue.ref;
    var onMounted  = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;
    var watch      = Vue.watch;

    var createRouter    = VueRouter.createRouter;
    var createWebHistory = VueRouter.createWebHistory;

    /* ---- Routes ---- */
    var routes = [
        { path: '/',                component: window.PageDashboard,   meta: { title: 'Панель управления' } },
        { path: '/stocks',          component: window.PageStocks,      meta: { title: 'Акции' } },
        { path: '/stocks/:ticker',  component: window.PageStockDetail, meta: { title: 'Детали акции' } },
        { path: '/portfolios',      component: window.PagePortfolios,  meta: { title: 'Портфели' } },
        { path: '/signals',         component: window.PageSignals,     meta: { title: 'Сигналы' } },
        { path: '/news',            component: window.PageNews,        meta: { title: 'Новости' } },
        { path: '/llm-stats',       component: window.PageLLMStats,    meta: { title: 'LLM Провайдеры' } },
        { path: '/tinvest',         component: window.PageTInvest,     meta: { title: 'Т-Инвестиции' } },
        { path: '/settings',        component: window.PageSettings,    meta: { title: 'Настройки' } }
    ];

    var router = createRouter({
        history: createWebHistory(),
        routes: routes
    });

    /* Update document title on navigation */
    router.afterEach(function (to) {
        var base = 'MarketPulse_RU';
        document.title = to.meta && to.meta.title ? to.meta.title + ' | ' + base : base;
    });

    /* ---- App ---- */
    var app = createApp({
        setup: function () {
            var sidebarCollapsed = ref(true); /* collapsed by default on mobile */
            var windowWidth = ref(window.innerWidth);
            var apiOnline = ref(false);

            var navLinks = [
                { path: '/',           label: 'Панель',     icon: 'bi-speedometer2' },
                { path: '/stocks',     label: 'Акции',      icon: 'bi-bar-chart-line' },
                { path: '/portfolios', label: 'Портфели',   icon: 'bi-briefcase' },
                { path: '/signals',    label: 'Сигналы',    icon: 'bi-lightning-charge' },
                { path: '/news',       label: 'Новости',    icon: 'bi-newspaper' },
                { path: '/llm-stats',  label: 'LLM',        icon: 'bi-cpu' },
                { path: '/tinvest',    label: 'Т-Инвест',  icon: 'bi-bank' },
                { path: '/settings',   label: 'Настройки',  icon: 'bi-gear' }
            ];

            function isActive(path) {
                var current = router.currentRoute.value.path;
                if (path === '/') return current === '/';
                return current.startsWith(path);
            }

            function handleResize() {
                windowWidth.value = window.innerWidth;
                if (windowWidth.value >= 768) {
                    sidebarCollapsed.value = false;
                }
            }

            async function checkHealth() {
                try {
                    await API.getHealth();
                    apiOnline.value = true;
                } catch (_) {
                    apiOnline.value = false;
                }
            }

            var healthInterval = null;

            onMounted(function () {
                handleResize();
                window.addEventListener('resize', handleResize);
                checkHealth();
                healthInterval = setInterval(checkHealth, 30000);
            });

            onBeforeUnmount(function () {
                window.removeEventListener('resize', handleResize);
                if (healthInterval) clearInterval(healthInterval);
            });

            return {
                sidebarCollapsed: sidebarCollapsed,
                windowWidth: windowWidth,
                apiOnline: apiOnline,
                navLinks: navLinks,
                isActive: isActive
            };
        }
    });

    app.use(router);

    /* Global error handler */
    app.config.errorHandler = function (err, vm, info) {
        console.error('[MarketPulse_RU] Error:', err, info);
    };

    app.config.warnHandler = function (msg, vm, trace) {
        /* Suppress warnings in production */
    };

    app.mount('#app');
})();
