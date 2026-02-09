package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// MOEX ISS candle intervals.
const (
	Interval1Min   = 1  // 1 minute
	Interval10Min  = 10 // 10 minutes
	Interval1Hour  = 60 // 1 hour
	Interval1Day   = 24 // 1 day
	Interval1Week  = 7  // 1 week
	Interval1Month = 31 // 1 month
)

// IntervalToString maps MOEX ISS interval codes to human-readable names.
var IntervalToString = map[int]string{
	Interval1Min:   "1m",
	Interval10Min:  "10m",
	Interval1Hour:  "1h",
	Interval1Day:   "1d",
	Interval1Week:  "1w",
	Interval1Month: "1M",
}

// StringToInterval maps interval strings to MOEX ISS codes.
var StringToInterval = map[string]int{
	"1m":  Interval1Min,
	"10m": Interval10Min,
	"1h":  Interval1Hour,
	"1d":  Interval1Day,
	"1w":  Interval1Week,
	"1M":  Interval1Month,
}

// candleResponse represents the MOEX ISS candles endpoint response.
type candleResponse struct {
	Candles struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"candles"`
}

// GetCandles fetches historical OHLCV candles from MOEX ISS.
// interval: use Interval* constants (1, 10, 60, 24, 7, 31).
// MOEX ISS returns max ~500 candles per request.
func (c *Client) GetCandles(ctx context.Context, ticker string, interval int, from, till time.Time) ([]domain.Candle, error) {
	intervalStr, ok := IntervalToString[interval]
	if !ok {
		return nil, fmt.Errorf("unsupported interval: %d", interval)
	}

	url := fmt.Sprintf(
		"%s/engines/stock/markets/shares/boards/TQBR/securities/%s/candles.json?from=%s&till=%s&interval=%d&iss.meta=off",
		c.baseURL, ticker,
		from.Format("2006-01-02"), till.Format("2006-01-02"),
		interval,
	)

	data, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching candles for %s: %w", ticker, err)
	}

	var resp candleResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing candle response: %w", err)
	}

	if len(resp.Candles.Data) == 0 {
		return nil, nil
	}

	colIdx := makeColumnIndex(resp.Candles.Columns)
	candles := make([]domain.Candle, 0, len(resp.Candles.Data))

	for _, row := range resp.Candles.Data {
		candle := domain.Candle{
			Ticker:   ticker,
			Interval: intervalStr,
		}

		if idx, ok := colIdx["open"]; ok && row[idx] != nil {
			candle.Open, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["high"]; ok && row[idx] != nil {
			candle.High, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["low"]; ok && row[idx] != nil {
			candle.Low, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["close"]; ok && row[idx] != nil {
			candle.Close, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["volume"]; ok && row[idx] != nil {
			candle.Volume, _ = row[idx].(float64)
		}
		if idx, ok := colIdx["begin"]; ok && row[idx] != nil {
			if ts, ok := row[idx].(string); ok {
				t, err := time.Parse("2006-01-02 15:04:05", ts)
				if err == nil {
					candle.OpenTime = t.Unix()
				}
			}
		}

		candles = append(candles, candle)
	}

	c.log.Debug("fetched candles", "ticker", ticker, "interval", intervalStr, "count", len(candles))
	return candles, nil
}

// GetCandlesAll fetches candles with pagination for large date ranges.
// MOEX ISS returns ~500 candles max per request, so we paginate.
func (c *Client) GetCandlesAll(ctx context.Context, ticker string, interval int, from, till time.Time) ([]domain.Candle, error) {
	var all []domain.Candle
	cursor := from

	for cursor.Before(till) {
		batch, err := c.GetCandles(ctx, ticker, interval, cursor, till)
		if err != nil {
			return all, err
		}
		if len(batch) == 0 {
			break
		}

		all = append(all, batch...)

		// Move cursor past the last candle we received.
		lastTime := time.Unix(batch[len(batch)-1].OpenTime, 0)
		nextCursor := lastTime.Add(time.Second)
		if !nextCursor.After(cursor) {
			break // safety: avoid infinite loop
		}
		cursor = nextCursor
	}

	return all, nil
}
