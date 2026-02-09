package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
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
	moexClient := moex.NewClient(cfg.MOEX, log)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		queueLen, _ := cache.QueueLen(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"queue_size": queueLen,
		})
	})

	// News endpoints
	mux.HandleFunc("GET /api/news", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		news, err := newsRepo.GetRecent(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
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

	// Company endpoints
	mux.HandleFunc("GET /api/companies", func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("q")
		var companies []domain.Company
		var err error
		if search != "" {
			companies, err = companyRepo.SearchByName(r.Context(), search)
		} else {
			companies, err = companyRepo.GetAll(r.Context())
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
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

	log.Info("API server starting", "addr", cfg.API.Addr())
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
		if originSet[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
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
