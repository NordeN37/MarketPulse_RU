package tinvest

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/russianinvestments/invest-api-go-sdk/investgo"
	pb "github.com/russianinvestments/invest-api-go-sdk/proto"
	"go.uber.org/zap"
)

const (
	ProdEndpoint    = "invest-public-api.tinkoff.ru:443"
	SandboxEndpoint = "sandbox-invest-public-api.tinkoff.ru:443"
)

// Manager manages the T-Invest API connection lifecycle.
// Token can be set/changed at runtime from the admin UI.
type Manager struct {
	mu     sync.RWMutex
	client *investgo.Client
	log    *slog.Logger
	token  string
	sandbox bool

	// Service clients (re-created on reconnect)
	md       *investgo.MarketDataServiceClient
	mdStream *investgo.MarketDataStreamClient
	orders   *investgo.OrdersServiceClient
	ops      *investgo.OperationsServiceClient
	users    *investgo.UsersServiceClient
	instr    *investgo.InstrumentsServiceClient
	sbx      *investgo.SandboxServiceClient
}

// NewManager creates a new T-Invest manager (not connected until SetToken is called).
func NewManager(log *slog.Logger) *Manager {
	return &Manager{log: log}
}

// Connected reports whether the client is connected to the API.
func (m *Manager) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client != nil
}

// Token returns the current API token (masked).
func (m *Manager) Token() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.token) < 8 {
		return ""
	}
	return m.token[:4] + "****" + m.token[len(m.token)-4:]
}

// IsSandbox reports whether the client is in sandbox mode.
func (m *Manager) IsSandbox() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sandbox
}

// SetToken connects to T-Invest API with the given token.
// If sandbox is true, connects to the sandbox endpoint.
// Disconnects any existing connection first.
func (m *Manager) SetToken(ctx context.Context, token string, sandbox bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Disconnect existing client
	if m.client != nil {
		m.client.Stop()
		m.client = nil
		m.md = nil
		m.mdStream = nil
		m.orders = nil
		m.ops = nil
		m.users = nil
		m.instr = nil
		m.sbx = nil
	}

	if token == "" {
		m.token = ""
		m.log.Info("T-Invest: disconnected (token cleared)")
		return nil
	}

	endpoint := ProdEndpoint
	if sandbox {
		endpoint = SandboxEndpoint
	}

	cfg := investgo.Config{
		EndPoint:                      endpoint,
		Token:                         token,
		AppName:                       "MarketPulse_RU",
		DisableResourceExhaustedRetry: false,
		DisableAllRetry:               false,
		MaxRetries:                    3,
	}

	// investgo needs a zap logger
	zapLog, _ := zap.NewProduction()
	client, err := investgo.NewClient(ctx, cfg, zapLog.Sugar())
	if err != nil {
		return fmt.Errorf("T-Invest connect: %w", err)
	}

	m.client = client
	m.token = token
	m.sandbox = sandbox

	// Initialize service clients
	m.md = client.NewMarketDataServiceClient()
	m.mdStream = client.NewMarketDataStreamClient()
	m.orders = client.NewOrdersServiceClient()
	m.ops = client.NewOperationsServiceClient()
	m.users = client.NewUsersServiceClient()
	m.instr = client.NewInstrumentsServiceClient()
	if sandbox {
		m.sbx = client.NewSandboxServiceClient()
	}

	mode := "PROD"
	if sandbox {
		mode = "SANDBOX"
	}
	m.log.Info("T-Invest: connected", "mode", mode, "endpoint", endpoint)
	return nil
}

// Disconnect shuts down the connection.
func (m *Manager) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		m.client.Stop()
		m.client = nil
		m.log.Info("T-Invest: disconnected")
	}
}

// ---- Account methods ----

// AccountInfo represents a broker account.
type AccountInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	AccessLevel string `json:"access_level"`
	OpenedAt    string `json:"opened_at,omitempty"`
}

// GetAccounts returns all available broker accounts.
func (m *Manager) GetAccounts() ([]AccountInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.users == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.users.GetAccounts(nil)
	if err != nil {
		return nil, fmt.Errorf("GetAccounts: %w", err)
	}

	var accounts []AccountInfo
	for _, a := range resp.GetAccounts() {
		acc := AccountInfo{
			ID:          a.GetId(),
			Name:        a.GetName(),
			Type:        a.GetType().String(),
			Status:      a.GetStatus().String(),
			AccessLevel: a.GetAccessLevel().String(),
		}
		if a.GetOpenedDate() != nil {
			acc.OpenedAt = a.GetOpenedDate().AsTime().Format(time.RFC3339)
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// ---- Market data methods ----

// QuoteData represents a real-time quote for a security.
type QuoteData struct {
	InstrumentID string  `json:"instrument_id"`
	Ticker       string  `json:"ticker,omitempty"`
	Last         float64 `json:"last"`
	Close        float64 `json:"close"`
	LimitUp      float64 `json:"limit_up"`
	LimitDown    float64 `json:"limit_down"`
}

// GetLastPrices fetches current prices for the given instrument IDs.
func (m *Manager) GetLastPrices(instrumentIDs []string) (map[string]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.md == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.md.GetLastPrices(instrumentIDs)
	if err != nil {
		return nil, fmt.Errorf("GetLastPrices: %w", err)
	}

	prices := make(map[string]float64, len(resp.GetLastPrices()))
	for _, lp := range resp.GetLastPrices() {
		if lp.GetPrice() != nil {
			prices[lp.GetInstrumentUid()] = lp.GetPrice().ToFloat()
		}
	}
	return prices, nil
}

// OrderBookData represents the order book for a security.
type OrderBookData struct {
	InstrumentID string            `json:"instrument_id"`
	Depth        int32             `json:"depth"`
	Bids         []OrderBookLevel  `json:"bids"`
	Asks         []OrderBookLevel  `json:"asks"`
	LastPrice    float64           `json:"last_price"`
	ClosePrice   float64           `json:"close_price"`
	LimitUp      float64           `json:"limit_up"`
	LimitDown    float64           `json:"limit_down"`
	Spread       float64           `json:"spread"`
	SpreadPct    float64           `json:"spread_pct"`
	BidVolume    int64             `json:"bid_volume"`
	AskVolume    int64             `json:"ask_volume"`
	Imbalance    float64           `json:"imbalance"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// OrderBookLevel represents a single price level.
type OrderBookLevel struct {
	Price    float64 `json:"price"`
	Quantity int64   `json:"quantity"`
}

// GetOrderBook fetches the order book for a given instrument UID.
func (m *Manager) GetOrderBook(instrumentID string, depth int32) (*OrderBookData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.md == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	if depth <= 0 || depth > 50 {
		depth = 20
	}

	resp, err := m.md.GetOrderBook(instrumentID, depth)
	if err != nil {
		return nil, fmt.Errorf("GetOrderBook: %w", err)
	}

	ob := &OrderBookData{
		InstrumentID: instrumentID,
		Depth:        resp.GetDepth(),
		UpdatedAt:    time.Now(),
	}

	if resp.GetLastPrice() != nil {
		ob.LastPrice = resp.GetLastPrice().ToFloat()
	}
	if resp.GetClosePrice() != nil {
		ob.ClosePrice = resp.GetClosePrice().ToFloat()
	}
	if resp.GetLimitUp() != nil {
		ob.LimitUp = resp.GetLimitUp().ToFloat()
	}
	if resp.GetLimitDown() != nil {
		ob.LimitDown = resp.GetLimitDown().ToFloat()
	}

	for _, bid := range resp.GetBids() {
		lvl := OrderBookLevel{
			Price:    bid.GetPrice().ToFloat(),
			Quantity: bid.GetQuantity(),
		}
		ob.Bids = append(ob.Bids, lvl)
		ob.BidVolume += bid.GetQuantity()
	}

	for _, ask := range resp.GetAsks() {
		lvl := OrderBookLevel{
			Price:    ask.GetPrice().ToFloat(),
			Quantity: ask.GetQuantity(),
		}
		ob.Asks = append(ob.Asks, lvl)
		ob.AskVolume += ask.GetQuantity()
	}

	// Calculate spread
	if len(ob.Bids) > 0 && len(ob.Asks) > 0 {
		ob.Spread = ob.Asks[0].Price - ob.Bids[0].Price
		mid := (ob.Asks[0].Price + ob.Bids[0].Price) / 2
		if mid > 0 {
			ob.SpreadPct = (ob.Spread / mid) * 100
		}
	}

	// Calculate imbalance
	total := ob.BidVolume + ob.AskVolume
	if total > 0 {
		ob.Imbalance = float64(ob.BidVolume-ob.AskVolume) / float64(total)
	}

	return ob, nil
}

// CandleData represents an OHLCV candle.
type CandleData struct {
	OpenTime int64   `json:"open_time"` // unix seconds
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   int64   `json:"volume"`
}

// CandleInterval maps string intervals to proto enum values.
var CandleIntervalMap = map[string]pb.CandleInterval{
	"1m":  pb.CandleInterval_CANDLE_INTERVAL_1_MIN,
	"5m":  pb.CandleInterval_CANDLE_INTERVAL_5_MIN,
	"15m": pb.CandleInterval_CANDLE_INTERVAL_15_MIN,
	"1h":  pb.CandleInterval_CANDLE_INTERVAL_HOUR,
	"1d":  pb.CandleInterval_CANDLE_INTERVAL_DAY,
	"1w":  pb.CandleInterval_CANDLE_INTERVAL_WEEK,
	"1M":  pb.CandleInterval_CANDLE_INTERVAL_MONTH,
}

// GetCandles fetches historical candles.
func (m *Manager) GetCandles(instrumentID string, interval string, from, to time.Time) ([]CandleData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.md == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	pbInterval, ok := CandleIntervalMap[interval]
	if !ok {
		pbInterval = pb.CandleInterval_CANDLE_INTERVAL_HOUR
	}

	resp, err := m.md.GetCandles(instrumentID, pbInterval, from, to,
		pb.GetCandlesRequest_CANDLE_SOURCE_UNSPECIFIED, 0)
	if err != nil {
		return nil, fmt.Errorf("GetCandles: %w", err)
	}

	var candles []CandleData
	for _, c := range resp.GetCandles() {
		cd := CandleData{
			Volume: c.GetVolume(),
		}
		if c.GetOpen() != nil {
			cd.Open = c.GetOpen().ToFloat()
		}
		if c.GetHigh() != nil {
			cd.High = c.GetHigh().ToFloat()
		}
		if c.GetLow() != nil {
			cd.Low = c.GetLow().ToFloat()
		}
		if c.GetClose() != nil {
			cd.Close = c.GetClose().ToFloat()
		}
		if c.GetTime() != nil {
			cd.OpenTime = c.GetTime().AsTime().Unix()
		}
		candles = append(candles, cd)
	}
	return candles, nil
}

// ---- Instruments ----

// InstrumentInfo represents basic instrument info.
type InstrumentInfo struct {
	UID            string  `json:"uid"`
	Figi           string  `json:"figi"`
	Ticker         string  `json:"ticker"`
	ClassCode      string  `json:"class_code"`
	Name           string  `json:"name"`
	Currency       string  `json:"currency"`
	Lot            int32   `json:"lot"`
	Exchange       string  `json:"exchange"`
	Sector         string  `json:"sector"`
	APITradeFlag   bool    `json:"api_trade_available"`
	ShortEnabled   bool    `json:"short_enabled"`
	MinPriceIncr   float64 `json:"min_price_increment"`
	InstrumentType string  `json:"instrument_type"`
}

// GetShares returns all available shares (stocks).
func (m *Manager) GetShares() ([]InstrumentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.instr == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.instr.Shares(pb.InstrumentStatus_INSTRUMENT_STATUS_BASE)
	if err != nil {
		return nil, fmt.Errorf("GetShares: %w", err)
	}

	var instruments []InstrumentInfo
	for _, s := range resp.GetInstruments() {
		inst := InstrumentInfo{
			UID:            s.GetUid(),
			Figi:           s.GetFigi(),
			Ticker:         s.GetTicker(),
			ClassCode:      s.GetClassCode(),
			Name:           s.GetName(),
			Currency:       s.GetCurrency(),
			Lot:            s.GetLot(),
			Exchange:       s.GetExchange(),
			Sector:         s.GetSector(),
			APITradeFlag:   s.GetApiTradeAvailableFlag(),
			ShortEnabled:   s.GetShortEnabledFlag(),
			InstrumentType: "share",
		}
		if s.GetMinPriceIncrement() != nil {
			inst.MinPriceIncr = s.GetMinPriceIncrement().ToFloat()
		}
		instruments = append(instruments, inst)
	}
	return instruments, nil
}

// FindInstrumentByTicker looks up an instrument by ticker.
func (m *Manager) FindInstrumentByTicker(ticker, classCode string) (*InstrumentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.instr == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.instr.InstrumentByTicker(ticker, classCode)
	if err != nil {
		return nil, fmt.Errorf("FindInstrument %s: %w", ticker, err)
	}

	s := resp.GetInstrument()
	if s == nil {
		return nil, fmt.Errorf("instrument %s not found", ticker)
	}

	inst := &InstrumentInfo{
		UID:          s.GetUid(),
		Figi:         s.GetFigi(),
		Ticker:       s.GetTicker(),
		ClassCode:    s.GetClassCode(),
		Name:         s.GetName(),
		Currency:     s.GetCurrency(),
		Lot:          s.GetLot(),
		Exchange:     s.GetExchange(),
		APITradeFlag: s.GetApiTradeAvailableFlag(),
		ShortEnabled: s.GetShortEnabledFlag(),
	}
	if s.GetMinPriceIncrement() != nil {
		inst.MinPriceIncr = s.GetMinPriceIncrement().ToFloat()
	}
	return inst, nil
}

// ---- Portfolio ----

// PortfolioData represents a full portfolio snapshot.
type PortfolioData struct {
	AccountID    string          `json:"account_id"`
	TotalAmount  float64         `json:"total_amount"`
	Currencies   float64         `json:"currencies"`
	Shares       float64         `json:"shares"`
	Bonds        float64         `json:"bonds"`
	ExpectedYield float64        `json:"expected_yield"`
	Positions    []PositionData  `json:"positions"`
}

// PositionData represents a single portfolio position.
type PositionData struct {
	InstrumentUID string  `json:"instrument_uid"`
	Figi          string  `json:"figi"`
	Ticker        string  `json:"ticker"`
	Type          string  `json:"type"`
	Quantity      float64 `json:"quantity"`
	AvgPrice      float64 `json:"avg_price"`
	CurrentPrice  float64 `json:"current_price"`
	ExpectedYield float64 `json:"expected_yield"`
	DailyYield    float64 `json:"daily_yield"`
	Currency      string  `json:"currency"`
	Blocked       bool    `json:"blocked"`
}

// GetPortfolio returns the full portfolio for an account.
func (m *Manager) GetPortfolio(accountID string) (*PortfolioData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ops == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.ops.GetPortfolio(accountID, pb.PortfolioRequest_RUB)
	if err != nil {
		return nil, fmt.Errorf("GetPortfolio: %w", err)
	}

	pd := &PortfolioData{
		AccountID: accountID,
	}

	if resp.GetTotalAmountPortfolio() != nil {
		pd.TotalAmount = resp.GetTotalAmountPortfolio().ToFloat()
	}
	if resp.GetTotalAmountCurrencies() != nil {
		pd.Currencies = resp.GetTotalAmountCurrencies().ToFloat()
	}
	if resp.GetTotalAmountShares() != nil {
		pd.Shares = resp.GetTotalAmountShares().ToFloat()
	}
	if resp.GetTotalAmountBonds() != nil {
		pd.Bonds = resp.GetTotalAmountBonds().ToFloat()
	}
	if resp.GetExpectedYield() != nil {
		pd.ExpectedYield = resp.GetExpectedYield().ToFloat()
	}

	for _, pos := range resp.GetPositions() {
		p := PositionData{
			InstrumentUID: pos.GetInstrumentUid(),
			Figi:          pos.GetFigi(),
			Ticker:        pos.GetTicker(),
			Type:          pos.GetInstrumentType(),
			Blocked:       pos.GetBlocked(),
		}
		if pos.GetQuantity() != nil {
			p.Quantity = pos.GetQuantity().ToFloat()
		}
		if pos.GetAveragePositionPrice() != nil {
			p.AvgPrice = pos.GetAveragePositionPrice().ToFloat()
			p.Currency = pos.GetAveragePositionPrice().GetCurrency()
		}
		if pos.GetCurrentPrice() != nil {
			p.CurrentPrice = pos.GetCurrentPrice().ToFloat()
		}
		if pos.GetExpectedYield() != nil {
			p.ExpectedYield = pos.GetExpectedYield().ToFloat()
		}
		if pos.GetDailyYield() != nil {
			p.DailyYield = pos.GetDailyYield().ToFloat()
		}
		pd.Positions = append(pd.Positions, p)
	}

	return pd, nil
}

// ---- Margin info ----

// MarginData represents margin attributes for an account.
type MarginData struct {
	LiquidPortfolio    float64 `json:"liquid_portfolio"`
	StartingMargin     float64 `json:"starting_margin"`
	MinimalMargin      float64 `json:"minimal_margin"`
	SufficientAmount   float64 `json:"sufficient_amount"`
	MissingFunds       float64 `json:"missing_funds"`
	FundsAvailable     float64 `json:"funds_available"`
}

// GetMarginAttributes returns margin data for an account.
func (m *Manager) GetMarginAttributes(accountID string) (*MarginData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.users == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.users.GetMarginAttributes(accountID)
	if err != nil {
		return nil, fmt.Errorf("GetMarginAttributes: %w", err)
	}

	md := &MarginData{}
	if resp.GetLiquidPortfolio() != nil {
		md.LiquidPortfolio = resp.GetLiquidPortfolio().ToFloat()
	}
	if resp.GetStartingMargin() != nil {
		md.StartingMargin = resp.GetStartingMargin().ToFloat()
	}
	if resp.GetMinimalMargin() != nil {
		md.MinimalMargin = resp.GetMinimalMargin().ToFloat()
	}
	if resp.GetFundsSufficiencyLevel() != nil {
		md.SufficientAmount = resp.GetFundsSufficiencyLevel().ToFloat()
	}
	if resp.GetAmountOfMissingFunds() != nil {
		md.MissingFunds = resp.GetAmountOfMissingFunds().ToFloat()
	}
	// Available for new positions = liquid - starting margin
	md.FundsAvailable = md.LiquidPortfolio - md.StartingMargin
	if md.FundsAvailable < 0 {
		md.FundsAvailable = 0
	}

	return md, nil
}

// ---- Orders ----

// OrderRequest represents a trade order to be placed.
type OrderRequest struct {
	AccountID    string  `json:"account_id"`
	InstrumentID string  `json:"instrument_id"`
	Quantity     int64   `json:"quantity"`
	Direction    string  `json:"direction"` // "BUY" or "SELL"
	OrderType    string  `json:"order_type"` // "MARKET", "LIMIT", "BESTPRICE"
	Price        float64 `json:"price,omitempty"` // required for LIMIT
}

// OrderResult represents the result of an order placement.
type OrderResult struct {
	OrderID          string  `json:"order_id"`
	ExecutionStatus  string  `json:"execution_status"`
	LotsRequested    int64   `json:"lots_requested"`
	LotsExecuted     int64   `json:"lots_executed"`
	InitialPrice     float64 `json:"initial_price"`
	ExecutedPrice    float64 `json:"executed_price"`
	TotalCommission  float64 `json:"total_commission"`
	Direction        string  `json:"direction"`
	InstrumentID     string  `json:"instrument_id"`
	Message          string  `json:"message,omitempty"`
}

// PostOrder places a new order.
func (m *Manager) PostOrder(req OrderRequest) (*OrderResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.orders == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	direction := pb.OrderDirection_ORDER_DIRECTION_BUY
	if req.Direction == "SELL" {
		direction = pb.OrderDirection_ORDER_DIRECTION_SELL
	}

	orderType := pb.OrderType_ORDER_TYPE_MARKET
	switch req.OrderType {
	case "LIMIT":
		orderType = pb.OrderType_ORDER_TYPE_LIMIT
	case "BESTPRICE":
		orderType = pb.OrderType_ORDER_TYPE_BESTPRICE
	}

	pbReq := &investgo.PostOrderRequest{
		InstrumentId: req.InstrumentID,
		Quantity:     req.Quantity,
		Direction:    direction,
		AccountId:    req.AccountID,
		OrderType:    orderType,
		OrderId:      fmt.Sprintf("mp_%d", time.Now().UnixNano()),
	}

	if orderType == pb.OrderType_ORDER_TYPE_LIMIT && req.Price > 0 {
		pbReq.Price = &pb.Quotation{
			Units: int64(req.Price),
			Nano:  int32((req.Price - float64(int64(req.Price))) * 1e9),
		}
	}

	resp, err := m.orders.PostOrder(pbReq)
	if err != nil {
		return nil, fmt.Errorf("PostOrder: %w", err)
	}

	result := &OrderResult{
		OrderID:         resp.GetOrderId(),
		ExecutionStatus: resp.GetExecutionReportStatus().String(),
		LotsRequested:   resp.GetLotsRequested(),
		LotsExecuted:    resp.GetLotsExecuted(),
		Direction:       req.Direction,
		InstrumentID:    req.InstrumentID,
		Message:         resp.GetMessage(),
	}
	if resp.GetInitialOrderPrice() != nil {
		result.InitialPrice = resp.GetInitialOrderPrice().ToFloat()
	}
	if resp.GetExecutedOrderPrice() != nil {
		result.ExecutedPrice = resp.GetExecutedOrderPrice().ToFloat()
	}
	if resp.GetInitialCommission() != nil {
		result.TotalCommission = resp.GetInitialCommission().ToFloat()
	}

	m.log.Info("T-Invest order placed",
		"order_id", result.OrderID,
		"instrument", req.InstrumentID,
		"direction", req.Direction,
		"lots", req.Quantity,
		"status", result.ExecutionStatus,
	)

	return result, nil
}

// CancelOrder cancels an existing order.
func (m *Manager) CancelOrder(accountID, orderID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.orders == nil {
		return fmt.Errorf("T-Invest not connected")
	}

	_, err := m.orders.CancelOrder(accountID, orderID, nil)
	if err != nil {
		return fmt.Errorf("CancelOrder: %w", err)
	}
	m.log.Info("T-Invest order cancelled", "order_id", orderID)
	return nil
}

// ActiveOrder represents an active order.
type ActiveOrder struct {
	OrderID      string  `json:"order_id"`
	InstrumentID string  `json:"instrument_id"`
	Direction    string  `json:"direction"`
	OrderType    string  `json:"order_type"`
	LotsTotal    int64   `json:"lots_total"`
	LotsExecuted int64   `json:"lots_executed"`
	Price        float64 `json:"price"`
	Status       string  `json:"status"`
}

// GetActiveOrders returns all active orders for an account.
func (m *Manager) GetActiveOrders(accountID string) ([]ActiveOrder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.orders == nil {
		return nil, fmt.Errorf("T-Invest not connected")
	}

	resp, err := m.orders.GetOrders(accountID, nil)
	if err != nil {
		return nil, fmt.Errorf("GetOrders: %w", err)
	}

	var orders []ActiveOrder
	for _, o := range resp.GetOrders() {
		ao := ActiveOrder{
			OrderID:      o.GetOrderId(),
			InstrumentID: o.GetInstrumentUid(),
			Direction:    o.GetDirection().String(),
			OrderType:    o.GetOrderType().String(),
			LotsTotal:    o.GetLotsRequested(),
			LotsExecuted: o.GetLotsExecuted(),
			Status:       o.GetExecutionReportStatus().String(),
		}
		if o.GetInitialSecurityPrice() != nil {
			ao.Price = o.GetInitialSecurityPrice().ToFloat()
		}
		orders = append(orders, ao)
	}
	return orders, nil
}

// ---- Status ----

// Status returns a summary of the T-Invest connection.
type Status struct {
	Connected   bool         `json:"connected"`
	Mode        string       `json:"mode"` // "prod", "sandbox", ""
	TokenMasked string       `json:"token_masked"`
	Accounts    []AccountInfo `json:"accounts,omitempty"`
}

// GetStatus returns connection status and account list.
func (m *Manager) GetStatus() *Status {
	s := &Status{
		Connected:   m.Connected(),
		TokenMasked: m.Token(),
	}
	if m.IsSandbox() {
		s.Mode = "sandbox"
	} else if s.Connected {
		s.Mode = "prod"
	}
	if s.Connected {
		if accs, err := m.GetAccounts(); err == nil {
			s.Accounts = accs
		}
	}
	return s
}
