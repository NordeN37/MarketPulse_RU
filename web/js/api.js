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

    window.API = {
        /** Raw GET returning parsed JSON */
        get: function (url) {
            return request(url);
        },

        /** Health check */
        getHealth: function () {
            return request('/api/health');
        },

        /** News — paginated list */
        getNews: function (limit, offset) {
            return request('/api/news' + qs({ limit: limit, offset: offset }));
        },

        /** Single news item by ID */
        getNewsById: function (id) {
            return request('/api/news/' + id);
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

        /** Heatmap data */
        getHeatmap: function (type, timeframe, limit) {
            return request('/api/heatmap' + qs({ type: type, timeframe: timeframe, limit: limit }));
        },

        /** Recent alerts */
        getAlerts: function (limit, minSeverity) {
            return request('/api/alerts' + qs({ limit: limit, min_severity: minSeverity }));
        },

        /** Trading signals */
        getSignals: function () {
            return request('/api/signals');
        },

        /** Portfolios */
        getPortfolios: function () {
            return request('/api/portfolios');
        }
    };
})();
