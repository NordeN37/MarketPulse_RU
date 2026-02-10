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
	// Energy
	CommodityBrent  = "BRENT"
	CommodityWTI    = "WTI"
	CommodityUrals  = "URALS"
	CommodityNG     = "NG"    // natural gas (Henry Hub)
	CommodityTTF    = "TTF"   // natural gas (EU)
	CommodityLNG    = "LNG"   // LNG (Asia JKM)
	CommodityCoal   = "COAL"
	CommodityUranium = "URANIUM"

	// Precious metals
	CommodityGold      = "GOLD"
	CommoditySilver    = "SILVER"
	CommodityPalladium = "PALLADIUM"
	CommodityPlatinum  = "PLATINUM"
	CommodityRhodium   = "RHODIUM"

	// Industrial metals
	CommodityCopper    = "COPPER"
	CommodityAluminium = "ALUMINIUM"
	CommodityNickel    = "NICKEL"
	CommodityZinc      = "ZINC"
	CommodityTin       = "TIN"
	CommodityLead      = "LEAD"
	CommodityIron      = "IRON"
	CommodityLithium   = "LITHIUM"
	CommodityCobalt    = "COBALT"

	// Agriculture
	CommodityWheat   = "WHEAT"
	CommodityCorn    = "CORN"
	CommoditySoybean = "SOYBEAN"
	CommodityCocoa   = "COCOA"
	CommodityCoffee  = "COFFEE"
	CommoditySugar   = "SUGAR"
	CommodityCotton  = "COTTON"
	CommodityRice    = "RICE"
	CommodityPalm    = "PALM"

	// Fertilizers
	CommodityUrea   = "UREA"
	CommodityPotash = "POTASH"
)
