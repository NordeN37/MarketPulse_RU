package domain

import "time"

// Commodity represents a raw material or primary product.
type Commodity struct {
	ID        int64     `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Exchange  string    `json:"exchange" db:"exchange"`
	Currency  string    `json:"currency" db:"currency"`
	Unit      string    `json:"unit" db:"unit"` // barrel, troy oz, ton, etc.
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Predefined commodity codes.
const (
	CommodityBrent  = "BRENT"
	CommodityWTI    = "WTI"
	CommodityNG     = "NG"    // natural gas
	CommodityGold   = "GOLD"
	CommoditySilver = "SILVER"
	CommodityPalladium = "PALLADIUM"
	CommodityPlatinum  = "PLATINUM"
	CommodityCopper = "COPPER"
	CommodityAluminium = "ALUMINIUM"
	CommodityNickel = "NICKEL"
	CommodityWheat  = "WHEAT"
)
