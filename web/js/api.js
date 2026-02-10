/* ============================================================
   MarketPulse_RU — API Helper
   ============================================================ */
(function () {
    'use strict';

    /**
     * Generic fetch wrapper that returns parsed JSON.
     * On non-2xx responses, throws an error with the body message.
     */
    async function request(url) {
        try {
            const resp = await fetch(url);
            if (!resp.ok) {
                let msg = resp.statusText;
                try {
                    const body = await resp.json();
                    msg = body.error || body.message || msg;
                } catch (_) { /* ignore parse errors */ }
                throw new Error(msg);
            }
            return await resp.json();
        } catch (err) {
            if (err.name === 'TypeError' && err.message === 'Failed to fetch') {
                throw new Error('API недоступен — проверьте соединение');
            }
            throw err;
        }
    }

    function qs(params) {
        const parts = [];
        for (const [k, v] of Object.entries(params)) {
            if (v !== undefined && v !== null && v !== '') {
                parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(v));
            }
        }
        return parts.length ? '?' + parts.join('&') : '';
    }

    async function postJSON(url, body) {
        try {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            if (!resp.ok) {
                let msg = resp.statusText;
                try {
                    const b = await resp.json();
                    msg = b.error || b.message || msg;
                } catch (_) {}
                throw new Error(msg);
            }
            return await resp.json();
        } catch (err) {
            if (err.name === 'TypeError' && err.message === 'Failed to fetch') {
                throw new Error('API недоступен — проверьте соединение');
            }
            throw err;
        }
    }

    window.API = {
        /** Raw GET returning parsed JSON */
        get: function (url) {
            return request(url);
        },

        /** Raw POST returning parsed JSON */
        post: function (url, body) {
            return postJSON(url, body);
        },

        /** Health check */
        getHealth: function () {
            return request('/api/health');
        },

        /** News — paginated list with optional filters */
        getNews: function (limit, offset, opts) {
            var params = { limit: limit, offset: offset };
            if (opts) {
                if (opts.category) params.category = opts.category;
                if (opts.ticker) params.ticker = opts.ticker;
                if (opts.q) params.q = opts.q;
                if (opts.related) params.related = '1';
            }
            return request('/api/news' + qs(params));
        },

        /** Single news item by ID */
        getNewsById: function (id) {
            return request('/api/news/' + id);
        },

        /** Available news categories */
        getNewsCategories: function () {
            return request('/api/news/categories');
        },

        /** All companies (optionally filtered by search query) */
        getCompanies: function (query) {
            return request('/api/companies' + qs({ q: query }));
        },

        /** Single company by ticker */
        getCompany: function (ticker) {
            return request('/api/companies/' + encodeURIComponent(ticker));
        },

        /** MOEX quote for a ticker */
        getQuote: function (ticker) {
            return request('/api/quote/' + encodeURIComponent(ticker));
        },

        /** MOEX index (e.g. IMOEX) */
        getIndex: function (index) {
            return request('/api/index/' + encodeURIComponent(index));
        },

        /** Candle data for TradingView charts */
        getCandles: function (ticker, interval, days) {
            return request(
                '/api/candles/' + encodeURIComponent(ticker) +
                qs({ interval: interval, days: days })
            );
        },

        /** Index candle data (IMOEX, etc.) */
        getIndexCandles: function (index, interval, days) {
            return request(
                '/api/index-candles/' + encodeURIComponent(index) +
                qs({ interval: interval, days: days })
            );
        },

        /** Heatmap data */
        getHeatmap: function (type, timeframe, limit) {
            return request('/api/heatmap' + qs({ type: type, timeframe: timeframe, limit: limit }));
        },

        /** Recent alerts */
        getAlerts: function (limit, minSeverity) {
            return request('/api/alerts' + qs({ limit: limit, min_severity: minSeverity }));
        },

        /** Trading signals */
        getSignals: function (ticker, source, limit) {
            return request('/api/signals' + qs({ ticker: ticker, source: source, limit: limit }));
        },

        /** Portfolios config */
        getPortfolios: function () {
            return request('/api/portfolios');
        },

        /** Portfolio equity curve snapshots (from backtest) */
        getPortfolioSnapshots: function (strategy) {
            return request('/api/portfolio-snapshots' + qs({ strategy: strategy }));
        },

        /** Portfolio summary (latest per strategy) */
        getPortfolioSummary: function () {
            return request('/api/portfolio-summary');
        },

        /** Trade history */
        getTrades: function (strategy, limit) {
            return request('/api/trades' + qs({ strategy: strategy, limit: limit }));
        },

        /** Universe info (all monitored companies + sector breakdown) */
        getUniverse: function () {
            return request('/api/universe');
        }
    };
})();
