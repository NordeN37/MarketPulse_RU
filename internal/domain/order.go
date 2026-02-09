package domain

import "time"

// OrderType defines the order execution strategy.
type OrderType string

const (
	OrderMarket     OrderType = "MARKET"
	OrderLimit      OrderType = "LIMIT"
	OrderStopLoss   OrderType = "STOP_LOSS"
	OrderTakeProfit OrderType = "TAKE_PROFIT"
)

// OrderSide is buy or sell.
type OrderSide string

const (
	OrderBuy  OrderSide = "BUY"
	OrderSell OrderSide = "SELL"
)

// OrderStatus tracks order lifecycle.
type OrderStatus string

const (
	OrderPending   OrderStatus = "PENDING"
	OrderFilled    OrderStatus = "FILLED"
	OrderPartial   OrderStatus = "PARTIAL"
	OrderCancelled OrderStatus = "CANCELLED"
	OrderRejected  OrderStatus = "REJECTED"
)

// Order represents a trade order sent to the exchange/broker.
type Order struct {
	ID         int64       `json:"id" db:"id"`
	Ticker     string      `json:"ticker" db:"ticker"`
	Side       OrderSide   `json:"side" db:"side"`
	Type       OrderType   `json:"type" db:"order_type"`
	Status     OrderStatus `json:"status" db:"status"`
	Quantity   int         `json:"quantity" db:"quantity"`
	Price      *float64    `json:"price,omitempty" db:"price"`
	StopPrice  *float64    `json:"stop_price,omitempty" db:"stop_price"`
	FilledQty  int         `json:"filled_qty" db:"filled_qty"`
	FilledPrice *float64   `json:"filled_price,omitempty" db:"filled_price"`
	Commission  float64    `json:"commission" db:"commission"`
	SignalID   *int64      `json:"signal_id,omitempty" db:"signal_id"`
	PositionID *int64      `json:"position_id,omitempty" db:"position_id"`
	ExternalID string      `json:"external_id" db:"external_id"`
	CreatedAt  time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at" db:"updated_at"`
	FilledAt   *time.Time  `json:"filled_at,omitempty" db:"filled_at"`
}
