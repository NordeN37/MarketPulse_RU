package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OrderBookEntry represents a single price level in the order book.
type OrderBookEntry struct {
	Price    float64 `json:"price"`
	Quantity int64   `json:"quantity"`
}

// OrderBook represents the full order book (стакан) for a security.
type OrderBook struct {
	SecID     string           `json:"secid"`
	Bids      []OrderBookEntry `json:"bids"` // buy orders, sorted by price desc
	Asks      []OrderBookEntry `json:"asks"` // sell orders, sorted by price asc
	Spread    float64          `json:"spread"`
	SpreadPct float64          `json:"spread_pct"`
	BidVolume int64            `json:"bid_volume"`
	AskVolume int64            `json:"ask_volume"`
	Imbalance float64          `json:"imbalance"` // (bidVol - askVol) / (bidVol + askVol)
	UpdatedAt time.Time        `json:"updated_at"`
}

// OrderBookAnomaly describes a detected anomaly in the order book.
type OrderBookAnomaly struct {
	Ticker    string    `json:"ticker"`
	Type      string    `json:"type"`      // "imbalance", "wall_bid", "wall_ask", "spread"
	Severity  string    `json:"severity"`  // "info", "warning", "critical"
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	DetectedAt time.Time `json:"detected_at"`
}

// Authenticate logs in to MOEX Passport and stores auth cookies.
// Required for order book access.
// Extracts raw Set-Cookie values and injects them manually on every ISS request,
// bypassing Go's cookie jar domain matching (passport.moex.com ≠ iss.moex.com).
func (c *Client) Authenticate(ctx context.Context, login, password string) error {
	if login == "" || password == "" {
		return fmt.Errorf("MOEX passport credentials not configured")
	}

	authClient := &http.Client{
		Timeout: 15 * time.Second,
		// Don't follow redirects — we need the raw Set-Cookie headers
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://passport.moex.com/authenticate", nil)
	if err != nil {
		return fmt.Errorf("creating auth request: %w", err)
	}
	req.SetBasicAuth(login, password)

	resp, err := authClient.Do(req)
	if err != nil {
		return fmt.Errorf("MOEX auth request: %w", err)
	}
	resp.Body.Close()

	// 200 = success, 302 = redirect with cookies (also ok)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return fmt.Errorf("MOEX auth failed: status %d", resp.StatusCode)
	}

	// Extract all Set-Cookie headers and build a raw Cookie string.
	// This bypasses Go's strict cookie domain matching —
	// passport.moex.com cookies will be sent to iss.moex.com.
	var parts []string
	for _, setCookie := range resp.Header["Set-Cookie"] {
		c.log.Info("MOEX auth cookie", "raw", setCookie)
		// Parse "name=value; Path=...; Domain=..." — extract "name=value" part
		nameVal := strings.SplitN(setCookie, ";", 2)[0]
		nameVal = strings.TrimSpace(nameVal)
		if nameVal != "" && !strings.HasPrefix(nameVal, "=") {
			parts = append(parts, nameVal)
		}
	}

	if len(parts) == 0 {
		return fmt.Errorf("MOEX auth: no cookies received (wrong credentials?)")
	}

	c.authCookie = strings.Join(parts, "; ")
	c.log.Info("MOEX passport authenticated", "login", login, "cookie_count", len(parts))
	return nil
}

// GetOrderBook fetches the order book (стакан) for a specific ticker.
// Requires prior authentication via Authenticate().
func (c *Client) GetOrderBook(ctx context.Context, ticker string) (*OrderBook, error) {
	reqURL := fmt.Sprintf("%s/engines/stock/markets/shares/boards/TQBR/securities/%s/orderbook.json?iss.meta=off",
		c.baseURL, ticker)

	data, err := c.doRequest(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("order book %s: %w", ticker, err)
	}

	var resp struct {
		Orderbook struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"orderbook"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		// Show start of response for debugging
		snippet := string(data)
		if len(snippet) > 100 {
			snippet = snippet[:100]
		}
		return nil, fmt.Errorf("parsing order book for %s: %w (response: %s)", ticker, err, snippet)
	}

	if len(resp.Orderbook.Data) == 0 {
		// Return empty order book instead of error (possible outside trading hours)
		return &OrderBook{
			SecID:     ticker,
			UpdatedAt: time.Now(),
		}, nil
	}

	colIdx := makeColumnIndex(resp.Orderbook.Columns)
	ob := &OrderBook{
		SecID:     ticker,
		UpdatedAt: time.Now(),
	}

	for _, row := range resp.Orderbook.Data {
		var buysell string
		var price float64
		var quantity int64

		if idx, ok := colIdx["BUYSELL"]; ok && idx < len(row) && row[idx] != nil {
			buysell, _ = row[idx].(string)
		}
		if idx, ok := colIdx["PRICE"]; ok && idx < len(row) && row[idx] != nil {
			price, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["QUANTITY"]; ok && idx < len(row) && row[idx] != nil {
			if v, ok := row[idx].(float64); ok {
				quantity = int64(v)
			}
		}

		if price == 0 || quantity == 0 {
			continue
		}

		entry := OrderBookEntry{Price: price, Quantity: quantity}
		if buysell == "B" {
			ob.Bids = append(ob.Bids, entry)
			ob.BidVolume += quantity
		} else if buysell == "S" {
			ob.Asks = append(ob.Asks, entry)
			ob.AskVolume += quantity
		}
	}

	// Sort: bids desc, asks asc
	sort.Slice(ob.Bids, func(i, j int) bool { return ob.Bids[i].Price > ob.Bids[j].Price })
	sort.Slice(ob.Asks, func(i, j int) bool { return ob.Asks[i].Price < ob.Asks[j].Price })

	// Calculate spread
	if len(ob.Bids) > 0 && len(ob.Asks) > 0 {
		ob.Spread = ob.Asks[0].Price - ob.Bids[0].Price
		mid := (ob.Asks[0].Price + ob.Bids[0].Price) / 2
		if mid > 0 {
			ob.SpreadPct = (ob.Spread / mid) * 100
		}
	}

	// Calculate imbalance: positive = more buying pressure, negative = more selling
	total := ob.BidVolume + ob.AskVolume
	if total > 0 {
		ob.Imbalance = float64(ob.BidVolume-ob.AskVolume) / float64(total)
	}

	return ob, nil
}

// DetectAnomalies analyzes the order book and returns any detected anomalies.
func DetectAnomalies(ob *OrderBook) []OrderBookAnomaly {
	if ob == nil {
		return nil
	}

	var anomalies []OrderBookAnomaly
	now := time.Now()

	// 1. Volume imbalance > 70% — strong directional pressure
	if math.Abs(ob.Imbalance) > 0.7 {
		direction := "покупку"
		if ob.Imbalance < 0 {
			direction = "продажу"
		}
		sev := "warning"
		if math.Abs(ob.Imbalance) > 0.85 {
			sev = "critical"
		}
		anomalies = append(anomalies, OrderBookAnomaly{
			Ticker:     ob.SecID,
			Type:       "imbalance",
			Severity:   sev,
			Message:    fmt.Sprintf("Сильный дисбаланс стакана на %s (%.0f%%)", direction, math.Abs(ob.Imbalance)*100),
			Value:      ob.Imbalance,
			Threshold:  0.7,
			DetectedAt: now,
		})
	}

	// 2. Detect bid walls — single level with > 5x average bid volume
	if len(ob.Bids) > 3 {
		avgBid := float64(ob.BidVolume) / float64(len(ob.Bids))
		for _, bid := range ob.Bids {
			if float64(bid.Quantity) > avgBid*5 {
				anomalies = append(anomalies, OrderBookAnomaly{
					Ticker:     ob.SecID,
					Type:       "wall_bid",
					Severity:   "warning",
					Message:    fmt.Sprintf("Крупная заявка на покупку: %d лотов @ %.2f (среднее: %.0f)", bid.Quantity, bid.Price, avgBid),
					Value:      float64(bid.Quantity),
					Threshold:  avgBid * 5,
					DetectedAt: now,
				})
				break // report only the biggest wall
			}
		}
	}

	// 3. Detect ask walls — single level with > 5x average ask volume
	if len(ob.Asks) > 3 {
		avgAsk := float64(ob.AskVolume) / float64(len(ob.Asks))
		for _, ask := range ob.Asks {
			if float64(ask.Quantity) > avgAsk*5 {
				anomalies = append(anomalies, OrderBookAnomaly{
					Ticker:     ob.SecID,
					Type:       "wall_ask",
					Severity:   "warning",
					Message:    fmt.Sprintf("Крупная заявка на продажу: %d лотов @ %.2f (среднее: %.0f)", ask.Quantity, ask.Price, avgAsk),
					Value:      float64(ask.Quantity),
					Threshold:  avgAsk * 5,
					DetectedAt: now,
				})
				break
			}
		}
	}

	// 4. Wide spread > 0.5% — low liquidity signal
	if ob.SpreadPct > 0.5 {
		sev := "info"
		if ob.SpreadPct > 1.0 {
			sev = "warning"
		}
		anomalies = append(anomalies, OrderBookAnomaly{
			Ticker:     ob.SecID,
			Type:       "spread",
			Severity:   sev,
			Message:    fmt.Sprintf("Широкий спред: %.2f%% (%.2f руб)", ob.SpreadPct, ob.Spread),
			Value:      ob.SpreadPct,
			Threshold:  0.5,
			DetectedAt: now,
		})
	}

	return anomalies
}
