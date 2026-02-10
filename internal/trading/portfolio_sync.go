package trading

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/market/tinvest"
	redisclient "github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// PortfolioSync periodically fetches real broker portfolio data
// and caches it in Redis for the admin UI.
type PortfolioSync struct {
	manager *tinvest.Manager
	cache   *redisclient.Client
	log     *slog.Logger
}

// NewPortfolioSync creates a new portfolio sync service.
func NewPortfolioSync(mgr *tinvest.Manager, cache *redisclient.Client, log *slog.Logger) *PortfolioSync {
	return &PortfolioSync{
		manager: mgr,
		cache:   cache,
		log:     log,
	}
}

// BrokerPortfolioCache is the cached portfolio data for all accounts.
type BrokerPortfolioCache struct {
	UpdatedAt  time.Time                    `json:"updated_at"`
	Accounts   []BrokerAccountSnapshot      `json:"accounts"`
}

// BrokerAccountSnapshot represents a point-in-time snapshot of a broker account.
type BrokerAccountSnapshot struct {
	AccountID    string               `json:"account_id"`
	AccountName  string               `json:"account_name"`
	Strategy     string               `json:"strategy,omitempty"`
	TotalAmount  float64              `json:"total_amount"`
	Currencies   float64              `json:"currencies"`
	Shares       float64              `json:"shares"`
	Bonds        float64              `json:"bonds"`
	Yield        float64              `json:"expected_yield"`
	Margin       *tinvest.MarginData  `json:"margin,omitempty"`
	Positions    []tinvest.PositionData `json:"positions"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// Run starts the periodic portfolio sync loop.
func (ps *PortfolioSync) Run(ctx context.Context, interval time.Duration) {
	ps.log.Info("portfolio sync started", "interval", interval)

	// Initial sync
	ps.sync(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ps.log.Info("portfolio sync stopped")
			return
		case <-ticker.C:
			ps.sync(ctx)
		}
	}
}

func (ps *PortfolioSync) sync(ctx context.Context) {
	if !ps.manager.Connected() {
		return
	}

	accounts, err := ps.manager.GetAccounts()
	if err != nil {
		ps.log.Warn("portfolio sync: failed to get accounts", "error", err)
		return
	}

	// Get strategy mappings
	strategies, _ := ps.cache.ReadTInvestStrategies(ctx)
	stratMap := make(map[string]string)
	for _, s := range strategies {
		stratMap[s.AccountID] = s.Strategy
	}

	var snapshots []BrokerAccountSnapshot
	for _, acc := range accounts {
		// Skip closed accounts
		if acc.Status != "ACCOUNT_STATUS_OPEN" {
			continue
		}

		snap := BrokerAccountSnapshot{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Strategy:    stratMap[acc.ID],
			UpdatedAt:   time.Now(),
		}

		// Fetch portfolio
		portfolio, err := ps.manager.GetPortfolio(acc.ID)
		if err != nil {
			ps.log.Debug("portfolio sync: skip account", "id", acc.ID, "error", err)
			continue
		}

		snap.TotalAmount = portfolio.TotalAmount
		snap.Currencies = portfolio.Currencies
		snap.Shares = portfolio.Shares
		snap.Bonds = portfolio.Bonds
		snap.Yield = portfolio.ExpectedYield
		snap.Positions = portfolio.Positions

		// Fetch margin (optional — may fail for non-margin accounts)
		marginData, err := ps.manager.GetMarginAttributes(acc.ID)
		if err == nil {
			snap.Margin = marginData
		}

		snapshots = append(snapshots, snap)
	}

	cache := BrokerPortfolioCache{
		UpdatedAt: time.Now(),
		Accounts:  snapshots,
	}

	data, err := json.Marshal(cache)
	if err != nil {
		ps.log.Warn("portfolio sync: marshal failed", "error", err)
		return
	}

	// Cache for 2 minutes
	if err := ps.cache.CacheSet(ctx, "broker:portfolios", json.RawMessage(data), 2*time.Minute); err != nil {
		ps.log.Warn("portfolio sync: cache write failed", "error", err)
	}

	ps.log.Debug("portfolio sync complete", "accounts", len(snapshots))
}

// GetCachedPortfolios returns the cached broker portfolio data.
func (ps *PortfolioSync) GetCachedPortfolios(ctx context.Context) (*BrokerPortfolioCache, error) {
	var data json.RawMessage
	found, err := ps.cache.CacheGet(ctx, "broker:portfolios", &data)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	var cache BrokerPortfolioCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("unmarshal cached portfolios: %w", err)
	}
	return &cache, nil
}

// ---- Withdrawal configuration ----

// WithdrawalConfig stores withdrawal settings.
type WithdrawalConfig struct {
	Enabled        bool    `json:"enabled"`
	ProfitPercent  float64 `json:"profit_percent"`  // % of net profit to withdraw
	DayOfWeek      int     `json:"day_of_week"`     // 0=Sunday, 1=Monday, ..., 5=Friday
	MinProfit      float64 `json:"min_profit"`       // minimum profit threshold (RUB)
	AccountID      string  `json:"account_id"`       // which account to withdraw from
}

const withdrawalConfigKey = "tinvest:withdrawal"

// WriteWithdrawalConfig saves withdrawal configuration.
func WriteWithdrawalConfig(ctx context.Context, cache *redisclient.Client, cfg WithdrawalConfig) error {
	return cache.CacheSet(ctx, withdrawalConfigKey, cfg, 0)
}

// ReadWithdrawalConfig reads withdrawal configuration.
func ReadWithdrawalConfig(ctx context.Context, cache *redisclient.Client) (*WithdrawalConfig, error) {
	var cfg WithdrawalConfig
	found, err := cache.CacheGet(ctx, withdrawalConfigKey, &cfg)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &cfg, nil
}
