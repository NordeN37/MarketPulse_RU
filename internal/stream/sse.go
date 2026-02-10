// Package stream provides Server-Sent Events (SSE) broadcasting for real-time data.
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/market/moex"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// QuoteBroadcaster polls MOEX for all quotes and pushes updates via SSE.
type QuoteBroadcaster struct {
	moex     *moex.Client
	compRepo *postgres.CompanyRepo
	log      *slog.Logger
	interval time.Duration

	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	latest  map[string]*moex.Quote // last known quotes
}

// NewQuoteBroadcaster creates a new broadcaster.
func NewQuoteBroadcaster(moexClient *moex.Client, compRepo *postgres.CompanyRepo, interval time.Duration, log *slog.Logger) *QuoteBroadcaster {
	return &QuoteBroadcaster{
		moex:     moexClient,
		compRepo: compRepo,
		log:      log,
		interval: interval,
		clients:  make(map[chan []byte]struct{}),
		latest:   make(map[string]*moex.Quote),
	}
}

// Run starts the background polling loop. Blocks until ctx is cancelled.
func (qb *QuoteBroadcaster) Run(ctx context.Context) {
	qb.log.Info("quote broadcaster started", "interval", qb.interval)

	// Initial fetch
	qb.poll(ctx)

	ticker := time.NewTicker(qb.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			qb.log.Info("quote broadcaster stopped")
			return
		case <-ticker.C:
			qb.poll(ctx)
		}
	}
}

func (qb *QuoteBroadcaster) poll(ctx context.Context) {
	// Get all tickers we care about
	tickers, err := qb.compRepo.GetAllTickers(ctx)
	if err != nil {
		qb.log.Warn("quote poll: failed to get tickers", "error", err)
		return
	}
	if len(tickers) == 0 {
		return
	}

	// Single MOEX API call for all securities
	allQuotes, err := qb.moex.GetAllQuotes(ctx)
	if err != nil {
		qb.log.Warn("quote poll: MOEX fetch failed", "error", err)
		return
	}

	// Filter to only our tickers
	filtered := make(map[string]*moex.Quote, len(tickers))
	for _, t := range tickers {
		if q, ok := allQuotes[t]; ok {
			filtered[t] = q
		}
	}

	// Also fetch IMOEX index
	if idxQuote, err := qb.moex.GetIndex(ctx, "IMOEX"); err == nil {
		filtered["IMOEX"] = idxQuote
	}

	qb.mu.Lock()
	qb.latest = filtered
	qb.mu.Unlock()

	// Broadcast to all connected SSE clients
	data, err := json.Marshal(filtered)
	if err != nil {
		return
	}

	qb.mu.RLock()
	for ch := range qb.clients {
		select {
		case ch <- data:
		default:
			// Client too slow, skip this update
		}
	}
	qb.mu.RUnlock()

	qb.log.Debug("quotes broadcast", "tickers", len(filtered), "clients", qb.clientCount())
}

// GetLatest returns the latest cached quotes (for initial SSE payload).
func (qb *QuoteBroadcaster) GetLatest() map[string]*moex.Quote {
	qb.mu.RLock()
	defer qb.mu.RUnlock()
	cp := make(map[string]*moex.Quote, len(qb.latest))
	for k, v := range qb.latest {
		cp[k] = v
	}
	return cp
}

func (qb *QuoteBroadcaster) clientCount() int {
	qb.mu.RLock()
	defer qb.mu.RUnlock()
	return len(qb.clients)
}

// subscribe registers a new SSE client.
func (qb *QuoteBroadcaster) subscribe() chan []byte {
	ch := make(chan []byte, 4)
	qb.mu.Lock()
	qb.clients[ch] = struct{}{}
	qb.mu.Unlock()
	return ch
}

// unsubscribe removes an SSE client.
func (qb *QuoteBroadcaster) unsubscribe(ch chan []byte) {
	qb.mu.Lock()
	delete(qb.clients, ch)
	qb.mu.Unlock()
	close(ch)
}

// ServeHTTP is the SSE endpoint handler.
func (qb *QuoteBroadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send latest quotes immediately so client doesn't wait
	if latest := qb.GetLatest(); len(latest) > 0 {
		data, _ := json.Marshal(latest)
		fmt.Fprintf(w, "event: quotes\ndata: %s\n\n", data)
		flusher.Flush()
	}

	ch := qb.subscribe()
	defer qb.unsubscribe(ch)

	ctx := r.Context()
	// Keepalive comment every 15s to prevent proxy/browser timeout
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: quotes\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
