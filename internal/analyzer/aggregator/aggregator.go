package aggregator

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/classifier"
	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/scorer"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// Aggregator processes news through classification, impact assessment, and scoring.
type Aggregator struct {
	newsRepo    *postgres.NewsRepo
	companyRepo *postgres.CompanyRepo
	classifier  *classifier.Classifier
	scorer      *scorer.Scorer
	log         *slog.Logger
}

// NewAggregator creates a new news aggregator.
func NewAggregator(
	newsRepo *postgres.NewsRepo,
	companyRepo *postgres.CompanyRepo,
	cls *classifier.Classifier,
	sc *scorer.Scorer,
	log *slog.Logger,
) *Aggregator {
	return &Aggregator{
		newsRepo:    newsRepo,
		companyRepo: companyRepo,
		classifier:  cls,
		scorer:      sc,
		log:         log,
	}
}

// ProcessNews runs the full analysis pipeline for a single news item.
func (a *Aggregator) ProcessNews(ctx context.Context, news *domain.News) error {
	// 1. Classify the news
	result, model, err := a.classifier.Classify(ctx, news)
	if err != nil {
		a.log.Error("classification failed",
			"news_id", news.ID,
			"error", err,
		)
		return err
	}

	a.log.Info("news classified",
		"news_id", news.ID,
		"category", result.Category,
		"sentiment", result.Sentiment,
		"urgency", result.Urgency,
		"tickers", result.Tickers,
		"model", model,
	)

	// 2. Save analysis
	keyFacts, _ := json.Marshal(result.KeyFacts)
	analysis := &domain.NewsAnalysis{
		NewsID:           news.ID,
		AnalyzedAt:       time.Now(),
		Category:         result.Category,
		Sentiment:        result.Sentiment,
		Urgency:          result.Urgency,
		ReliabilityScore: result.Reliability,
		SummaryRU:        result.Summary,
		KeyFacts:         keyFacts,
		LLMModel:         model,
	}

	if err := a.newsRepo.SaveAnalysis(ctx, analysis); err != nil {
		return err
	}

	// 3. Create impacts for mentioned tickers
	for _, ticker := range result.Tickers {
		company, err := a.companyRepo.GetByTicker(ctx, ticker)
		if err != nil || company == nil {
			continue
		}

		impact := &domain.NewsImpact{
			NewsID:     news.ID,
			EntityType: domain.EntityCompany,
			EntityID:   company.ID,
			Direction:  sentimentToDirection(result.Sentiment),
			Magnitude:  abs(result.Sentiment),
			Timeframe:  categoryToTimeframe(result.Category),
			Confidence: result.Reliability,
			Reasoning:  result.Summary,
		}

		if err := a.newsRepo.SaveImpact(ctx, impact); err != nil {
			a.log.Error("failed to save impact",
				"news_id", news.ID,
				"ticker", ticker,
				"error", err,
			)
			continue
		}

		// 4. Update heat score
		if err := a.scorer.UpdateFromImpact(ctx, impact); err != nil {
			a.log.Error("failed to update heat score",
				"news_id", news.ID,
				"ticker", ticker,
				"error", err,
			)
		}
	}

	return nil
}

func sentimentToDirection(sentiment float64) domain.ImpactDirection {
	if sentiment > 0.1 {
		return domain.ImpactPositive
	}
	if sentiment < -0.1 {
		return domain.ImpactNegative
	}
	return domain.ImpactNeutral
}

func categoryToTimeframe(cat domain.NewsCategory) domain.ImpactTimeframe {
	switch cat {
	case domain.CategoryGeoSanctions, domain.CategoryCorpMA, domain.CategoryCBRate:
		return domain.TimeframeImmediate
	case domain.CategoryCorpEarnings, domain.CategoryCorpDividend, domain.CategoryCorpLegal:
		return domain.TimeframeShort
	case domain.CategoryMacroInflation, domain.CategoryMacroGDP, domain.CategoryRegulation:
		return domain.TimeframeMedium
	default:
		return domain.TimeframeShort
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
