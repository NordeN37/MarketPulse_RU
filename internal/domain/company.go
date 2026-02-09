package domain

import "time"

// Company represents a publicly traded company.
type Company struct {
	ID        int64     `json:"id" db:"id"`
	Ticker    string    `json:"ticker" db:"ticker"`
	Name      string    `json:"name" db:"name"`
	Sector    string    `json:"sector" db:"sector"`
	Country   string    `json:"country" db:"country"`
	MoexID    string    `json:"moex_id,omitempty" db:"moex_id"`
	SmartlabID string   `json:"smartlab_id,omitempty" db:"smartlab_id"`
	MarketCap  float64  `json:"market_cap" db:"market_cap"`
	IsWatchlist bool    `json:"is_watchlist" db:"is_watchlist"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Sector represents an industry sector.
type Sector struct {
	ID   int64  `json:"id" db:"id"`
	Code string `json:"code" db:"code"`
	Name string `json:"name" db:"name"`
}

// Predefined sector codes.
const (
	SectorOilGas    = "OIL_GAS"
	SectorBanks     = "BANKS"
	SectorRetail    = "RETAIL"
	SectorTelecom   = "TELECOM"
	SectorMetals    = "METALS"
	SectorChemistry = "CHEMISTRY"
	SectorEnergy    = "ENERGY"
	SectorIT        = "IT"
	SectorReal      = "REAL_ESTATE"
	SectorTransport = "TRANSPORT"
	SectorAgriculture = "AGRICULTURE"
	SectorFinance   = "FINANCE"
)

// Region represents a geographic region.
type Region struct {
	ID               int64   `json:"id" db:"id"`
	Code             string  `json:"code" db:"code"`
	Name             string  `json:"name" db:"name"`
	ImportanceWeight float64 `json:"importance_weight" db:"importance_weight"`
}

// Predefined region codes.
const (
	RegionRU = "RU"
	RegionUS = "US"
	RegionEU = "EU"
	RegionCN = "CN"
	RegionUK = "UK"
)
