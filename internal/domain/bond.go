package domain

import "time"

// Bond represents a fixed-income instrument.
type Bond struct {
	ID           int64     `json:"id" db:"id"`
	ISIN         string    `json:"isin" db:"isin"`
	IssuerID     int64     `json:"issuer_id" db:"issuer_id"`
	Name         string    `json:"name" db:"name"`
	CouponRate   float64   `json:"coupon_rate" db:"coupon_rate"`
	CouponFreq   int       `json:"coupon_freq" db:"coupon_freq"` // payments per year
	Maturity     time.Time `json:"maturity" db:"maturity"`
	Rating       string    `json:"rating" db:"rating"`
	RatingAgency string    `json:"rating_agency" db:"rating_agency"`
	Nominal      float64   `json:"nominal" db:"nominal"`
	Currency     string    `json:"currency" db:"currency"`
	CurrentPrice float64   `json:"current_price" db:"current_price"`
	YTM          float64   `json:"ytm" db:"ytm"` // yield to maturity
	MoexID       string    `json:"moex_id,omitempty" db:"moex_id"`
	IsWatchlist  bool      `json:"is_watchlist" db:"is_watchlist"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
