package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/collector"
	"github.com/NordeN37/MarketPulse_RU/internal/collector/rss"
	"github.com/NordeN37/MarketPulse_RU/internal/collector/telegram"
	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/stream"
	redisclient "github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	db, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.RunMigrations(ctx, log); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	cache, err := redisclient.New(cfg.Redis)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	newsRepo := postgres.NewNewsRepo(db)
	companyRepo := postgres.NewCompanyRepo(db)
	heatRepo := postgres.NewHeatRepo(db)
	alertRepo := postgres.NewAlertRepo(db)
	signalRepo := postgres.NewSignalRepo(db)
	portfolioRepo := postgres.NewPortfolioRepo(db)
	tradeRepo := postgres.NewTradeRepo(db)
	moexClient := moex.NewClient(cfg.MOEX, log)

	// Start SSE quote broadcaster (polls MOEX every 10s, pushes via SSE)
	quoteBroadcaster := stream.NewQuoteBroadcaster(moexClient, companyRepo, 10*time.Second, log)
	go quoteBroadcaster.Run(ctx)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// =====================================================
	// API Endpoints
	// =====================================================

	// Health check
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		queueLen, _ := cache.QueueLen(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"queue_size": queueLen,
		})
	})

	// News endpoints — supports category, ticker (via impacts), search, related
	mux.HandleFunc("GET /api/news", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		ticker := r.URL.Query().Get("ticker")
		category := r.URL.Query().Get("category")
		search := r.URL.Query().Get("q")
		related := r.URL.Query().Get("related") == "1"

		// If ticker specified, search via news_impacts.
		if ticker != "" {
			news, err := newsRepo.GetNewsByCompanyTicker(r.Context(), ticker, limit, related)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if news == nil {
				news = []postgres.NewsWithAnalysis{}
			}
			writeJSON(w, http.StatusOK, news)
			return
		}

		// Otherwise return news with analysis (optional category/search filter).
		news, err := newsRepo.GetRecentWithAnalysis(r.Context(), limit, offset, category, search)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if news == nil {
			news = []postgres.NewsWithAnalysis{}
		}
		writeJSON(w, http.StatusOK, news)
	})

	mux.HandleFunc("GET /api/news/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		news, err := newsRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if news == nil {
			writeError(w, http.StatusNotFound, "news not found")
			return
		}
		writeJSON(w, http.StatusOK, news)
	})

	// News categories (for filter dropdown)
	mux.HandleFunc("GET /api/news/categories", func(w http.ResponseWriter, r *http.Request) {
		cats, err := newsRepo.GetCategories(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cats == nil {
			cats = []string{}
		}
		writeJSON(w, http.StatusOK, cats)
	})

	// Company endpoints
	mux.HandleFunc("GET /api/companies", func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("q")
		var companies []domain.Company
		var compErr error
		if search != "" {
			companies, compErr = companyRepo.SearchByName(r.Context(), search)
		} else {
			companies, compErr = companyRepo.GetAll(r.Context())
		}
		if compErr != nil {
			writeError(w, http.StatusInternalServerError, compErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, companies)
	})

	mux.HandleFunc("GET /api/companies/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		ticker := r.PathValue("ticker")
		company, err := companyRepo.GetByTicker(r.Context(), ticker)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if company == nil {
			writeError(w, http.StatusNotFound, "company not found")
			return
		}
		writeJSON(w, http.StatusOK, company)
	})

	// Heat map endpoints
	mux.HandleFunc("GET /api/heatmap", func(w http.ResponseWriter, r *http.Request) {
		entityType := domain.EntityType(r.URL.Query().Get("type"))
		if entityType == "" {
			entityType = domain.EntityCompany
		}
		timeframe := r.URL.Query().Get("timeframe")
		if timeframe == "" {
			timeframe = "1d"
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 50 {
			limit = 20
		}

		today := time.Now().Truncate(24 * time.Hour)
		scores, err := heatRepo.GetTopHeat(r.Context(), entityType, timeframe, today, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, scores)
	})

	// Alerts endpoints
	mux.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		minSeverity := domain.AlertSeverity(r.URL.Query().Get("min_severity"))
		if minSeverity == "" {
			minSeverity = domain.SeverityInfo
		}

		alerts, err := alertRepo.GetRecent(r.Context(), limit, minSeverity)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, alerts)
	})

	// MOEX quote proxy
	mux.HandleFunc("GET /api/quote/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		ticker := r.PathValue("ticker")
		quote, err := moexClient.GetQuote(r.Context(), ticker)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, quote)
	})

	mux.HandleFunc("GET /api/index/{index}", func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		quote, err := moexClient.GetIndex(r.Context(), index)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, quote)
	})

	// =====================================================
	// Candle data (proxied from MOEX ISS)
	// =====================================================
	mux.HandleFunc("GET /api/candles/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		ticker := r.PathValue("ticker")
		intervalStr := r.URL.Query().Get("interval")
		if intervalStr == "" {
			intervalStr = "1h"
		}
		interval, ok := moex.StringToInterval[intervalStr]
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid interval, use: 1m, 10m, 1h, 1d, 1w, 1M")
			return
		}
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days <= 0 || days > 365 {
			days = 30
		}

		now := time.Now()
		from := now.AddDate(0, 0, -days)

		candles, err := moexClient.GetCandlesAll(r.Context(), ticker, interval, from, now)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, candles)
	})

	// =====================================================
	// Index candle data (IMOEX, etc.)
	// =====================================================
	mux.HandleFunc("GET /api/index-candles/{index}", func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		intervalStr := r.URL.Query().Get("interval")
		if intervalStr == "" {
			intervalStr = "1d"
		}
		interval, ok := moex.StringToInterval[intervalStr]
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid interval")
			return
		}
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days <= 0 || days > 730 {
			days = 365
		}

		now := time.Now()
		from := now.AddDate(0, 0, -days)

		candles, err := moexClient.GetIndexCandlesAll(r.Context(), index, interval, from, now)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, candles)
	})

	// =====================================================
	// SSE: real-time quote stream
	// =====================================================
	mux.Handle("GET /api/stream/quotes", quoteBroadcaster)

	// REST fallback: get cached quotes (for initial page load)
	mux.HandleFunc("GET /api/quotes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, quoteBroadcaster.GetLatest())
	})

	// =====================================================
	// Universe: full monitoring universe from DB
	// =====================================================
	mux.HandleFunc("GET /api/universe", func(w http.ResponseWriter, r *http.Request) {
		companies, err := companyRepo.GetAll(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Group by sector.
		sectors := make(map[string][]domain.Company)
		for _, c := range companies {
			sectors[c.Sector] = append(sectors[c.Sector], c)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total":     len(companies),
			"companies": companies,
			"sectors":   sectors,
			"monitoring": map[string]any{
				"universe":        cfg.Monitoring.Universe,
				"candle_days_back": cfg.Monitoring.CandleDaysBack,
			},
			"trading": map[string]any{
				"max_positions":    cfg.Trading.MaxPositions,
				"explicit_tickers": cfg.Trading.Tickers,
				"mode":             cfg.Trading.Mode,
			},
		})
	})

	// =====================================================
	// Trading signals (from DB, populated by backtest/live)
	// =====================================================
	mux.HandleFunc("GET /api/signals", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		ticker := r.URL.Query().Get("ticker")
		source := r.URL.Query().Get("source")

		var sigs []domain.Signal
		var sigErr error

		switch {
		case ticker != "":
			sigs, sigErr = signalRepo.GetByTicker(r.Context(), ticker, limit)
		case source != "":
			sigs, sigErr = signalRepo.GetBySource(r.Context(), source, limit)
		default:
			sigs, sigErr = signalRepo.GetRecent(r.Context(), limit)
		}
		if sigErr != nil {
			writeError(w, http.StatusInternalServerError, sigErr.Error())
			return
		}
		if sigs == nil {
			sigs = []domain.Signal{}
		}
		tradingTickers := cfg.Trading.Tickers
		if len(tradingTickers) == 0 {
			tradingTickers = []string{} // dynamic selection, no fixed tickers
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"signals":        sigs,
			"tickers":        tradingTickers,
			"max_positions":  cfg.Trading.MaxPositions,
			"mode":           cfg.Trading.Mode,
		})
	})

	// =====================================================
	// Portfolios (returns config for 3 portfolio types)
	// =====================================================
	mux.HandleFunc("GET /api/portfolios", func(w http.ResponseWriter, r *http.Request) {
		tradingTickers := cfg.Trading.Tickers
		if len(tradingTickers) == 0 {
			tradingTickers = []string{} // dynamic
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"portfolios": []map[string]any{
				{
					"type":         "news",
					"name":         "Новостной",
					"description":  "Торговля только по новостным сигналам",
					"initial_cash": cfg.Trading.InitialCash,
					"tickers":      tradingTickers,
				},
				{
					"type":         "ta",
					"name":         "Технический",
					"description":  "Торговля только по техническому анализу",
					"initial_cash": cfg.Trading.InitialCash,
					"tickers":      tradingTickers,
				},
				{
					"type":         "combined",
					"name":         "Комбинированный",
					"description":  "Совмещение новостей и технического анализа",
					"initial_cash": cfg.Trading.InitialCash,
					"tickers":      tradingTickers,
				},
			},
			"max_positions": cfg.Trading.MaxPositions,
			"risk":          cfg.Trading.Risk,
			"strategy":      cfg.Trading.Strategy,
		})
	})

	// =====================================================
	// Portfolio equity curves (from backtesting)
	// =====================================================
	mux.HandleFunc("GET /api/portfolio-snapshots", func(w http.ResponseWriter, r *http.Request) {
		strat := r.URL.Query().Get("strategy")
		if strat == "" {
			// Return all 3 strategies.
			result := make(map[string]any)
			for _, s := range []string{"news", "ta", "combined"} {
				snaps, err := portfolioRepo.GetByStrategy(r.Context(), s)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				result[s] = snaps
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		snaps, err := portfolioRepo.GetByStrategy(r.Context(), strat)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if snaps == nil {
			snaps = []postgres.PortfolioSnapshot{}
		}
		writeJSON(w, http.StatusOK, snaps)
	})

	// Portfolio summary (latest snapshot per strategy).
	mux.HandleFunc("GET /api/portfolio-summary", func(w http.ResponseWriter, r *http.Request) {
		latest, err := portfolioRepo.GetLatest(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, latest)
	})

	// =====================================================
	// Trade history
	// =====================================================
	mux.HandleFunc("GET /api/trades", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		strat := r.URL.Query().Get("strategy")
		var trades []postgres.TradeRecord
		var trErr error
		if strat != "" {
			trades, trErr = tradeRepo.GetByStrategy(r.Context(), strat, limit)
		} else {
			trades, trErr = tradeRepo.GetRecent(r.Context(), limit)
		}
		if trErr != nil {
			writeError(w, http.StatusInternalServerError, trErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, trades)
	})

	// =====================================================
	// Admin: Telegram auth management
	// =====================================================
	var authBridge *telegram.AuthBridge
	if cfg.Telegram.Phone != "" {
		authBridge = telegram.NewAuthBridge(cache.RDB(), cfg.Telegram.Phone, log)
	}

	mux.HandleFunc("GET /api/admin/telegram-status", func(w http.ResponseWriter, r *http.Request) {
		if authBridge == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"state":     "not_configured",
				"message":   "Telegram MTProto не настроен (нет phone)",
				"has_session": false,
			})
			return
		}
		state := authBridge.State(r.Context())
		resp := map[string]any{
			"state": string(state),
		}
		if state == telegram.AuthStateError {
			resp["error"] = authBridge.LastError(r.Context())
		}
		if state == telegram.AuthStatePendingCode {
			resp["message"] = "Введите код авторизации Telegram"
		}
		if state == telegram.AuthStatePendingPassword {
			resp["message"] = "Введите пароль двухфакторной авторизации"
		}
		if state == telegram.AuthStateAuthenticated {
			resp["message"] = "Telegram авторизован"
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /api/admin/telegram-code", func(w http.ResponseWriter, r *http.Request) {
		if authBridge == nil {
			writeError(w, http.StatusBadRequest, "Telegram не настроен")
			return
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
			writeError(w, http.StatusBadRequest, "укажите code в теле запроса")
			return
		}
		if err := authBridge.SubmitCode(r.Context(), body.Code); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "code submitted"})
	})

	mux.HandleFunc("POST /api/admin/telegram-password", func(w http.ResponseWriter, r *http.Request) {
		if authBridge == nil {
			writeError(w, http.StatusBadRequest, "Telegram не настроен")
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
			writeError(w, http.StatusBadRequest, "укажите password в теле запроса")
			return
		}
		if err := authBridge.SubmitPassword(r.Context(), body.Password); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password submitted"})
	})

	// =====================================================
	// Admin: Source management & re-read
	// =====================================================

	// Pipeline for RSS re-reads (dedup handles duplicates)
	rereadPipeline := collector.NewPipeline(newsRepo, cache, log)

	// List all sources (RSS feeds + Telegram channels)
	mux.HandleFunc("GET /api/admin/sources", func(w http.ResponseWriter, r *http.Request) {
		var sources []map[string]any

		// RSS feeds
		for _, f := range rss.DefaultFeeds() {
			sources = append(sources, map[string]any{
				"type":    "rss",
				"name":    f.Name,
				"channel": f.Channel,
				"url":     f.URL,
			})
		}

		// Telegram channels
		for _, ch := range cfg.Telegram.Channels {
			sources = append(sources, map[string]any{
				"type":    "telegram",
				"name":    "@" + ch,
				"channel": ch,
			})
		}

		writeJSON(w, http.StatusOK, sources)
	})

	// Trigger re-read of a source (RSS: direct fetch, Telegram: via Redis to collector)
	mux.HandleFunc("POST /api/admin/reread", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type    string `json:"type"`    // "rss" or "telegram"
			Channel string `json:"channel"` // channel name
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.Channel == "" {
			writeError(w, http.StatusBadRequest, "channel is required")
			return
		}

		switch body.Type {
		case "rss":
			feed, found := rss.FindFeedByChannel(body.Channel)
			if !found {
				writeError(w, http.StatusNotFound, "RSS feed not found: "+body.Channel)
				return
			}

			processed, total, err := rss.FetchOneFeed(r.Context(), feed, rereadPipeline.HandleNews, log)
			if err != nil {
				writeError(w, http.StatusBadGateway, "fetch failed: "+err.Error())
				return
			}
			log.Info("admin re-read RSS completed",
				"feed", feed.Name,
				"total_items", total,
				"processed", processed,
			)
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    "ok",
				"feed":      feed.Name,
				"total":     total,
				"processed": processed,
				"message":   fmt.Sprintf("Перечитано %d элементов из %s (дубли отсеяны)", total, feed.Name),
			})

		case "telegram":
			// Send command to collector via Redis pub/sub
			if err := cache.PublishReread(r.Context(), "telegram", body.Channel); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to send reread command: "+err.Error())
				return
			}
			log.Info("admin re-read Telegram command sent", "channel", body.Channel)
			writeJSON(w, http.StatusOK, map[string]any{
				"status":  "ok",
				"message": fmt.Sprintf("Команда на перечитку @%s отправлена коллектору", body.Channel),
			})

		default:
			writeError(w, http.StatusBadRequest, "type must be 'rss' or 'telegram'")
		}
	})

	// =====================================================
	// Static files — serve Vue.js SPA from web/
	// =====================================================
	webFS := http.FileServer(http.Dir("web"))
	mux.Handle("GET /js/", webFS)
	mux.Handle("GET /css/", webFS)
	mux.Handle("GET /assets/", webFS)
	// SPA fallback: serve index.html for any non-API route
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})

	// CORS middleware
	handler := corsMiddleware(cfg.API.CORSOrigins, mux)

	server := &http.Server{
		Addr:    cfg.API.Addr(),
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Info("API server starting", "addr", cfg.API.Addr(), "web_ui", "http://"+cfg.API.Addr())
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
	log.Info("API server stopped")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func corsMiddleware(origins []string, next http.Handler) http.Handler {
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if originSet[origin] || origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
