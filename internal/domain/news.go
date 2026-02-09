package domain

import (
	"encoding/json"
	"time"
)

// NewsSource identifies where a news item came from.
type NewsSource string

const (
	SourceTelegram NewsSource = "telegram"
	SourceRSS      NewsSource = "rss"
	SourceWeb      NewsSource = "web"
	SourceAPI      NewsSource = "api"
)

// News represents a collected news item before or after analysis.
type News struct {
	ID            int64           `json:"id" db:"id"`
	ExternalID    string          `json:"external_id" db:"external_id"`
	Source        NewsSource      `json:"source" db:"source"`
	SourceChannel string          `json:"source_channel" db:"source_channel"`
	Title         string          `json:"title" db:"title"`
	Content       string          `json:"content" db:"content"`
	URL           string          `json:"url,omitempty" db:"url"`
	MediaURLs     []string        `json:"media_urls,omitempty" db:"media_urls"`
	PublishedAt   time.Time       `json:"published_at" db:"published_at"`
	CollectedAt   time.Time       `json:"collected_at" db:"collected_at"`
	RawJSON       json.RawMessage `json:"raw_json,omitempty" db:"raw_json"`
}

// NewsCategory classifies the type of news event.
type NewsCategory string

const (
	// Corporate events
	CategoryCorpEarnings   NewsCategory = "CORP_EARNINGS"
	CategoryCorpDividend   NewsCategory = "CORP_DIVIDEND"
	CategoryCorpMA         NewsCategory = "CORP_MA"
	CategoryCorpManagement NewsCategory = "CORP_MANAGEMENT"
	CategoryCorpLegal      NewsCategory = "CORP_LEGAL"
	CategoryCorpDebt       NewsCategory = "CORP_DEBT"
	CategoryCorpRating     NewsCategory = "CORP_RATING"

	// Macro
	CategoryCBRate          NewsCategory = "CB_RATE"
	CategoryCBPolicy        NewsCategory = "CB_POLICY"
	CategoryMacroInflation  NewsCategory = "MACRO_INFLATION"
	CategoryMacroGDP        NewsCategory = "MACRO_GDP"
	CategoryMacroEmployment NewsCategory = "MACRO_EMPLOYMENT"

	// Geopolitics
	CategoryGeoSanctions NewsCategory = "GEO_SANCTIONS"
	CategoryGeoDiplomacy NewsCategory = "GEO_DIPLOMACY"
	CategoryGeoConflict  NewsCategory = "GEO_CONFLICT"
	CategoryGeoTrade     NewsCategory = "GEO_TRADE"

	// Commodities
	CategoryCommodityOil   NewsCategory = "COMMODITY_OIL"
	CategoryCommodityGas   NewsCategory = "COMMODITY_GAS"
	CategoryCommodityMetal NewsCategory = "COMMODITY_METAL"
	CategoryCommodityAgro  NewsCategory = "COMMODITY_AGRO"

	// Other
	CategoryRegulation NewsCategory = "REGULATION"
	CategoryWeather    NewsCategory = "WEATHER"
	CategoryTech       NewsCategory = "TECH"
)

// NewsAnalysis contains the LLM-generated analysis for a news item.
type NewsAnalysis struct {
	ID               int64           `json:"id" db:"id"`
	NewsID           int64           `json:"news_id" db:"news_id"`
	AnalyzedAt       time.Time       `json:"analyzed_at" db:"analyzed_at"`
	Category         NewsCategory    `json:"category" db:"category"`
	Sentiment        float64         `json:"sentiment" db:"sentiment"` // -1.0 to +1.0
	Urgency          int             `json:"urgency" db:"urgency"`     // 1-5
	ReliabilityScore float64         `json:"reliability_score" db:"reliability_score"`
	SummaryRU        string          `json:"summary_ru" db:"summary_ru"`
	KeyFacts         json.RawMessage `json:"key_facts" db:"key_facts"`
	LLMModel         string          `json:"llm_model" db:"llm_model"` // which model produced this
}

// EntityType identifies the kind of entity impacted by news.
type EntityType string

const (
	EntityCompany   EntityType = "company"
	EntityBond      EntityType = "bond"
	EntityCommodity EntityType = "commodity"
	EntitySector    EntityType = "sector"
	EntityRegion    EntityType = "region"
)

// ImpactDirection indicates whether the impact is positive, negative, or neutral.
type ImpactDirection int

const (
	ImpactNegative ImpactDirection = -1
	ImpactNeutral  ImpactDirection = 0
	ImpactPositive ImpactDirection = 1
)

// ImpactTimeframe indicates how quickly the impact is expected.
type ImpactTimeframe string

const (
	TimeframeImmediate ImpactTimeframe = "IMMEDIATE"
	TimeframeShort     ImpactTimeframe = "SHORT"  // days
	TimeframeMedium    ImpactTimeframe = "MEDIUM"  // weeks
	TimeframeLong      ImpactTimeframe = "LONG"    // months+
)

// NewsImpact describes how a news item affects a specific entity.
type NewsImpact struct {
	ID              int64           `json:"id" db:"id"`
	NewsID          int64           `json:"news_id" db:"news_id"`
	EntityType      EntityType      `json:"entity_type" db:"entity_type"`
	EntityID        int64           `json:"entity_id" db:"entity_id"`
	EntityName      string          `json:"entity_name" db:"entity_name"` // ticker or commodity code
	Direction       ImpactDirection `json:"impact_direction" db:"impact_direction"`
	Magnitude       float64         `json:"impact_magnitude" db:"impact_magnitude"` // 0.0 to 1.0
	Timeframe       ImpactTimeframe `json:"impact_timeframe" db:"impact_timeframe"`
	Confidence      float64         `json:"confidence" db:"confidence"`
	Reasoning       string          `json:"reasoning" db:"reasoning"`
}
