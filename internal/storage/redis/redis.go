package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
)

// Client wraps the Redis client with application-specific methods.
type Client struct {
	rdb *redis.Client
}

// New creates a new Redis client.
func New(cfg config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Close shuts down the Redis client.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// RDB returns the underlying go-redis client (for components that need direct access).
func (c *Client) RDB() *redis.Client {
	return c.rdb
}

// Dedup checks if a news item has been seen recently (returns true if duplicate).
func (c *Client) Dedup(ctx context.Context, source, externalID string, window time.Duration) (bool, error) {
	key := fmt.Sprintf("dedup:%s:%s", source, externalID)
	set, err := c.rdb.SetNX(ctx, key, "1", window).Result()
	if err != nil {
		return false, fmt.Errorf("dedup check: %w", err)
	}
	return !set, nil // if SetNX returns false, key already existed = duplicate
}

// EnqueueNews adds a news ID to the analysis queue.
func (c *Client) EnqueueNews(ctx context.Context, newsID int64) error {
	return c.rdb.LPush(ctx, "queue:news:analyze", newsID).Err()
}

// DequeueNews gets the next news ID from the analysis queue.
func (c *Client) DequeueNews(ctx context.Context, timeout time.Duration) (int64, error) {
	result, err := c.rdb.BRPop(ctx, timeout, "queue:news:analyze").Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("dequeue news: %w", err)
	}
	var id int64
	if _, err := fmt.Sscanf(result[1], "%d", &id); err != nil {
		return 0, fmt.Errorf("parsing news id: %w", err)
	}
	return id, nil
}

// CacheSet stores a value in cache with TTL.
func (c *Client) CacheSet(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshaling cache value: %w", err)
	}
	return c.rdb.Set(ctx, "cache:"+key, data, ttl).Err()
}

// CacheGet retrieves a cached value.
func (c *Client) CacheGet(ctx context.Context, key string, dest any) (bool, error) {
	data, err := c.rdb.Get(ctx, "cache:"+key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getting cache: %w", err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("unmarshaling cache: %w", err)
	}
	return true, nil
}

// PublishAlert publishes an alert event for real-time processing.
func (c *Client) PublishAlert(ctx context.Context, alertID int64) error {
	return c.rdb.Publish(ctx, "alerts:new", alertID).Err()
}

// SubscribeAlerts returns a channel that receives new alert IDs.
func (c *Client) SubscribeAlerts(ctx context.Context) (<-chan int64, error) {
	pubsub := c.rdb.Subscribe(ctx, "alerts:new")
	ch := make(chan int64, 100)

	go func() {
		defer close(ch)
		defer pubsub.Close()

		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			var id int64
			if _, err := fmt.Sscanf(msg.Payload, "%d", &id); err == nil {
				ch <- id
			}
		}
	}()

	return ch, nil
}

// QueueLen returns the length of the analysis queue.
func (c *Client) QueueLen(ctx context.Context) (int64, error) {
	return c.rdb.LLen(ctx, "queue:news:analyze").Result()
}

// WriteLLMStats stores LLM usage stats (written by analyzer, read by API).
func (c *Client) WriteLLMStats(ctx context.Context, statsJSON []byte) error {
	return c.rdb.Set(ctx, "llm:stats", statsJSON, 5*time.Minute).Err()
}

// ReadLLMStats reads LLM usage stats (written by analyzer).
func (c *Client) ReadLLMStats(ctx context.Context) ([]byte, error) {
	data, err := c.rdb.Get(ctx, "llm:stats").Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return data, err
}

// PublishReread sends a reread command to the collector for a specific source/channel.
func (c *Client) PublishReread(ctx context.Context, sourceType, channel string) error {
	payload := sourceType + ":" + channel
	return c.rdb.Publish(ctx, "collector:reread", payload).Err()
}

// SubscribeReread returns a channel that receives reread commands (format: "source_type:channel_name").
func (c *Client) SubscribeReread(ctx context.Context) (<-chan string, error) {
	pubsub := c.rdb.Subscribe(ctx, "collector:reread")
	ch := make(chan string, 10)

	go func() {
		defer close(ch)
		defer pubsub.Close()

		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			ch <- msg.Payload
		}
	}()

	return ch, nil
}

// ---- T-Invest token management ----

// TInvestConfig stores T-Invest API configuration.
type TInvestConfig struct {
	Token   string `json:"token"`
	Sandbox bool   `json:"sandbox"`
}

// WriteTInvestConfig saves T-Invest configuration (token + mode).
func (c *Client) WriteTInvestConfig(ctx context.Context, cfg TInvestConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, "tinvest:config", data, 0).Err() // no expiry
}

// ReadTInvestConfig reads stored T-Invest configuration.
func (c *Client) ReadTInvestConfig(ctx context.Context) (*TInvestConfig, error) {
	data, err := c.rdb.Get(ctx, "tinvest:config").Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg TInvestConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DeleteTInvestConfig removes stored T-Invest configuration.
func (c *Client) DeleteTInvestConfig(ctx context.Context) error {
	return c.rdb.Del(ctx, "tinvest:config").Err()
}

// ---- T-Invest account-strategy mapping ----

// TInvestAccountStrategy maps an account to a trading strategy.
type TInvestAccountStrategy struct {
	AccountID string `json:"account_id"`
	Strategy  string `json:"strategy"` // "news", "ta", "combined", "" (disabled)
	Enabled   bool   `json:"enabled"`
}

// WriteTInvestStrategies saves account-strategy mappings.
func (c *Client) WriteTInvestStrategies(ctx context.Context, strategies []TInvestAccountStrategy) error {
	data, err := json.Marshal(strategies)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, "tinvest:strategies", data, 0).Err()
}

// ReadTInvestStrategies reads account-strategy mappings.
func (c *Client) ReadTInvestStrategies(ctx context.Context) ([]TInvestAccountStrategy, error) {
	data, err := c.rdb.Get(ctx, "tinvest:strategies").Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var strategies []TInvestAccountStrategy
	if err := json.Unmarshal(data, &strategies); err != nil {
		return nil, err
	}
	return strategies, nil
}
