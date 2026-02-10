package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
)

// Client communicates with the MOEX ISS API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	log        *slog.Logger
	authCookie string // raw Cookie header value from MOEX Passport auth
}

// NewClient creates a new MOEX ISS API client.
func NewClient(cfg config.MOEXConfig, log *slog.Logger) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout(),
		},
		log: log,
	}
}

// SecurityInfo contains basic security information from MOEX.
type SecurityInfo struct {
	SecID     string  `json:"secid"`
	ShortName string  `json:"shortname"`
	Name      string  `json:"name"`
	ISIN      string  `json:"isin"`
	Type      string  `json:"type"`
	Group     string  `json:"group"`
	LastPrice float64 `json:"last_price"`
	Change    float64 `json:"change"`
	Volume    int64   `json:"volume"`
}

// Quote contains current market data for a security.
type Quote struct {
	SecID     string    `json:"secid"`
	Last      float64   `json:"last"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Change    float64   `json:"change"`
	Volume    int64     `json:"volume"`
	Value     float64   `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ISSResponse represents the generic MOEX ISS API response structure.
type ISSResponse struct {
	Securities struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"securities"`
	MarketData struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"marketdata"`
}

// GetQuote fetches current market data for a ticker.
func (c *Client) GetQuote(ctx context.Context, ticker string) (*Quote, error) {
	url := fmt.Sprintf("%s/engines/stock/markets/shares/boards/TQBR/securities/%s.json?iss.meta=off", c.baseURL, ticker)

	data, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp ISSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing MOEX response: %w", err)
	}

	if len(resp.MarketData.Data) == 0 {
		return nil, fmt.Errorf("no market data for %s", ticker)
	}

	q := &Quote{
		SecID:     ticker,
		UpdatedAt: time.Now(),
	}

	// Parse market data columns
	colIdx := makeColumnIndex(resp.MarketData.Columns)
	row := resp.MarketData.Data[0]

	if idx, ok := colIdx["LAST"]; ok && row[idx] != nil {
		q.Last, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["OPEN"]; ok && row[idx] != nil {
		q.Open, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["HIGH"]; ok && row[idx] != nil {
		q.High, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["LOW"]; ok && row[idx] != nil {
		q.Low, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["LASTTOPREVPRICE"]; ok && row[idx] != nil {
		q.Change, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["VOLTODAY"]; ok && row[idx] != nil {
		if v, ok := row[idx].(float64); ok {
			q.Volume = int64(v)
		}
	}
	if idx, ok := colIdx["VALTODAY"]; ok && row[idx] != nil {
		q.Value, _ = row[idx].(float64)
	}

	return q, nil
}

// GetIndex fetches current index value (e.g., IMOEX).
func (c *Client) GetIndex(ctx context.Context, index string) (*Quote, error) {
	url := fmt.Sprintf("%s/engines/stock/markets/index/boards/SNDX/securities/%s.json?iss.meta=off", c.baseURL, index)

	data, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp ISSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing MOEX response: %w", err)
	}

	if len(resp.MarketData.Data) == 0 {
		return nil, fmt.Errorf("no data for index %s", index)
	}

	q := &Quote{
		SecID:     index,
		UpdatedAt: time.Now(),
	}

	colIdx := makeColumnIndex(resp.MarketData.Columns)
	row := resp.MarketData.Data[0]

	if idx, ok := colIdx["CURRENTVALUE"]; ok && row[idx] != nil {
		q.Last, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["OPENVALUE"]; ok && row[idx] != nil {
		q.Open, _ = row[idx].(float64)
	}
	if idx, ok := colIdx["LASTCHANGE"]; ok && row[idx] != nil {
		q.Change, _ = row[idx].(float64)
	}

	return q, nil
}

// GetAllQuotes fetches market data for ALL securities on TQBR board in a single API call.
// Much more efficient than calling GetQuote per ticker.
func (c *Client) GetAllQuotes(ctx context.Context) (map[string]*Quote, error) {
	url := fmt.Sprintf("%s/engines/stock/markets/shares/boards/TQBR/securities.json?iss.meta=off&iss.only=marketdata&marketdata.columns=SECID,LAST,OPEN,HIGH,LOW,LASTTOPREVPRICE,VOLTODAY,VALTODAY", c.baseURL)

	data, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp struct {
		MarketData struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"marketdata"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing MOEX batch response: %w", err)
	}

	colIdx := makeColumnIndex(resp.MarketData.Columns)
	now := time.Now()
	result := make(map[string]*Quote, len(resp.MarketData.Data))

	for _, row := range resp.MarketData.Data {
		secIdx, ok := colIdx["SECID"]
		if !ok || secIdx >= len(row) || row[secIdx] == nil {
			continue
		}
		secID, _ := row[secIdx].(string)
		if secID == "" {
			continue
		}

		q := &Quote{SecID: secID, UpdatedAt: now}
		if idx, ok := colIdx["LAST"]; ok && idx < len(row) && row[idx] != nil {
			q.Last, _ = row[idx].(float64)
		}
		if q.Last == 0 {
			continue // skip securities with no last price
		}
		if idx, ok := colIdx["OPEN"]; ok && idx < len(row) && row[idx] != nil {
			q.Open, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["HIGH"]; ok && idx < len(row) && row[idx] != nil {
			q.High, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["LOW"]; ok && idx < len(row) && row[idx] != nil {
			q.Low, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["LASTTOPREVPRICE"]; ok && idx < len(row) && row[idx] != nil {
			q.Change, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["VOLTODAY"]; ok && idx < len(row) && row[idx] != nil {
			if v, ok := row[idx].(float64); ok {
				q.Volume = int64(v)
			}
		}
		if idx, ok := colIdx["VALTODAY"]; ok && idx < len(row) && row[idx] != nil {
			q.Value, _ = row[idx].(float64)
		}
		result[secID] = q
	}

	return result, nil
}

func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Inject MOEX Passport auth cookie if available
	if c.authCookie != "" {
		req.Header.Set("Cookie", c.authCookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MOEX request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MOEX returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading MOEX response: %w", err)
	}

	// Detect HTML responses (auth redirect or error pages)
	if len(data) > 0 && data[0] == '<' {
		snippet := string(data)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("MOEX returned HTML instead of JSON (auth required?): %s...", snippet)
	}

	return data, nil
}

func makeColumnIndex(columns []string) map[string]int {
	idx := make(map[string]int, len(columns))
	for i, col := range columns {
		idx[col] = i
	}
	return idx
}
