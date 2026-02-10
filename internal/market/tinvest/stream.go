package tinvest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	pb "github.com/russianinvestments/invest-api-go-sdk/proto"
)

// PriceUpdate represents a real-time price update pushed via SSE.
type PriceUpdate struct {
	InstrumentID string  `json:"instrument_id"`
	Price        float64 `json:"price"`
	Timestamp    int64   `json:"ts"` // unix ms
}

// OrderBookUpdate represents a real-time order book update pushed via SSE.
type OrderBookUpdate struct {
	InstrumentID string           `json:"instrument_id"`
	Depth        int32            `json:"depth"`
	Bids         []OrderBookLevel `json:"bids"`
	Asks         []OrderBookLevel `json:"asks"`
	Timestamp    int64            `json:"ts"`
}

// StreamEvent wraps different types of streaming events.
type StreamEvent struct {
	Type      string      `json:"type"` // "price", "orderbook", "trade"
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"ts"`
}

// Streamer manages a gRPC market data stream and broadcasts to SSE clients.
type Streamer struct {
	manager *Manager
	log     *slog.Logger

	mu      sync.RWMutex
	stream  *investgo.MarketDataStream
	cancel  context.CancelFunc
	running bool

	// SSE clients
	clientsMu sync.RWMutex
	clients   map[chan []byte]struct{}

	// Latest prices cache (instrument_uid → price)
	pricesMu sync.RWMutex
	prices   map[string]float64

	// Subscribed instrument IDs
	subscribedIDs []string
}

// NewStreamer creates a new market data streamer.
func NewStreamer(manager *Manager, log *slog.Logger) *Streamer {
	return &Streamer{
		manager: manager,
		log:     log,
		clients: make(map[chan []byte]struct{}),
		prices:  make(map[string]float64),
	}
}

// Start begins the gRPC stream and subscribes to the given instrument UIDs.
// Non-blocking — runs in background goroutines.
func (s *Streamer) Start(ctx context.Context, instrumentIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil // already running
	}

	s.manager.mu.RLock()
	mdStream := s.manager.mdStream
	s.manager.mu.RUnlock()

	if mdStream == nil {
		return fmt.Errorf("T-Invest not connected")
	}

	if len(instrumentIDs) == 0 {
		return nil
	}

	stream, err := mdStream.MarketDataStream()
	if err != nil {
		return fmt.Errorf("creating market data stream: %w", err)
	}

	s.stream = stream
	s.subscribedIDs = instrumentIDs
	s.running = true

	streamCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Subscribe to last prices
	lastPriceCh, err := stream.SubscribeLastPrice(instrumentIDs)
	if err != nil {
		s.log.Warn("T-Invest stream: failed to subscribe last prices", "error", err)
	}

	// Process last price updates
	if lastPriceCh != nil {
		go s.processLastPrices(streamCtx, lastPriceCh)
	}

	// Listen blocks and dispatches to channels
	go func() {
		if err := stream.Listen(); err != nil {
			s.log.Warn("T-Invest stream ended", "error", err)
		}
		s.mu.Lock()
		s.running = false
		s.stream = nil
		s.mu.Unlock()
		s.log.Info("T-Invest gRPC stream stopped")

		// Auto-restart after delay if still connected
		time.Sleep(5 * time.Second)
		if s.manager.Connected() && streamCtx.Err() == nil {
			s.log.Info("T-Invest stream: auto-restarting...")
			s.Start(ctx, instrumentIDs)
		}
	}()

	s.log.Info("T-Invest gRPC stream started", "instruments", len(instrumentIDs))
	return nil
}

// Stop terminates the gRPC stream.
func (s *Streamer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.stream != nil {
		s.stream.Stop()
		s.stream = nil
	}
	s.running = false
	s.log.Info("T-Invest streamer stopped")
}

// Running reports whether the stream is active.
func (s *Streamer) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetLatestPrices returns cached latest prices.
func (s *Streamer) GetLatestPrices() map[string]float64 {
	s.pricesMu.RLock()
	defer s.pricesMu.RUnlock()
	cp := make(map[string]float64, len(s.prices))
	for k, v := range s.prices {
		cp[k] = v
	}
	return cp
}

func (s *Streamer) processLastPrices(ctx context.Context, ch <-chan *pb.LastPrice) {
	for {
		select {
		case <-ctx.Done():
			return
		case lp, ok := <-ch:
			if !ok {
				return
			}
			if lp == nil || lp.GetPrice() == nil {
				continue
			}

			uid := lp.GetInstrumentUid()
			price := lp.GetPrice().ToFloat()

			// Update cache
			s.pricesMu.Lock()
			s.prices[uid] = price
			s.pricesMu.Unlock()

			// Build SSE event
			event := StreamEvent{
				Type: "price",
				Data: PriceUpdate{
					InstrumentID: uid,
					Price:        price,
					Timestamp:    time.Now().UnixMilli(),
				},
				Timestamp: time.Now().UnixMilli(),
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			// Broadcast to SSE clients
			s.clientsMu.RLock()
			for ch := range s.clients {
				select {
				case ch <- data:
				default:
					// slow client, skip
				}
			}
			s.clientsMu.RUnlock()
		}
	}
}

// broadcast sends data to all connected SSE clients.
func (s *Streamer) broadcast(data []byte) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for ch := range s.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

func (s *Streamer) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	s.clientsMu.Lock()
	s.clients[ch] = struct{}{}
	s.clientsMu.Unlock()
	return ch
}

func (s *Streamer) unsubscribe(ch chan []byte) {
	s.clientsMu.Lock()
	delete(s.clients, ch)
	s.clientsMu.Unlock()
	close(ch)
}

func (s *Streamer) clientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// ServeHTTP implements http.Handler for the SSE endpoint.
func (s *Streamer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send current cached prices immediately
	if latest := s.GetLatestPrices(); len(latest) > 0 {
		event := StreamEvent{
			Type:      "snapshot",
			Data:      latest,
			Timestamp: time.Now().UnixMilli(),
		}
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "event: tinvest\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// Send stream status
	statusData, _ := json.Marshal(map[string]any{
		"type":      "status",
		"connected": s.manager.Connected(),
		"streaming": s.Running(),
		"clients":   s.clientCount(),
	})
	fmt.Fprintf(w, "event: tinvest\ndata: %s\n\n", statusData)
	flusher.Flush()

	ch := s.subscribe()
	defer s.unsubscribe(ch)

	ctx := r.Context()
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
			fmt.Fprintf(w, "event: tinvest\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
