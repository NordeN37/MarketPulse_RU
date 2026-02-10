package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/classifier"
	"github.com/NordeN37/MarketPulse_RU/internal/analyzer/scorer"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/llm"
	"github.com/NordeN37/MarketPulse_RU/internal/llm/prompts"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
)

// Aggregator processes news through classification, impact assessment, and scoring.
type Aggregator struct {
	newsRepo    *postgres.NewsRepo
	companyRepo *postgres.CompanyRepo
	signalRepo  *postgres.SignalRepo
	classifier  *classifier.Classifier
	router      *llm.Router
	scorer      *scorer.Scorer
	log         *slog.Logger
}

// NewAggregator creates a new news aggregator.
func NewAggregator(
	newsRepo *postgres.NewsRepo,
	companyRepo *postgres.CompanyRepo,
	signalRepo *postgres.SignalRepo,
	cls *classifier.Classifier,
	router *llm.Router,
	sc *scorer.Scorer,
	log *slog.Logger,
) *Aggregator {
	return &Aggregator{
		newsRepo:    newsRepo,
		companyRepo: companyRepo,
		signalRepo:  signalRepo,
		classifier:  cls,
		router:      router,
		scorer:      sc,
		log:         log,
	}
}

// DeepImpact represents a single instrument impact from deep analysis.
type DeepImpact struct {
	Ticker          string  `json:"ticker"`
	EntityType      string  `json:"entity_type"`
	ImpactDirection int     `json:"impact_direction"` // -1, 0, 1
	ImpactMagnitude float64 `json:"impact_magnitude"`
	ImpactTimeframe string  `json:"impact_timeframe"`
	Confidence      float64 `json:"confidence"`
	Reasoning       string  `json:"reasoning"`
}

// ProcessNews runs the full analysis pipeline for a single news item.
func (a *Aggregator) ProcessNews(ctx context.Context, news *domain.News) error {
	// 1. Classify the news (fast LLM — 80%)
	result, model, err := a.classifier.Classify(ctx, news)
	if err != nil {
		a.log.Error("classification failed",
			"news_id", news.ID,
			"error", err,
		)
		return err
	}

	a.log.Debug("classified",
		"news_id", news.ID,
		"category", result.Category,
		"sentiment", result.Sentiment,
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

	// 3. Deep analysis for high-importance news (heavy LLM — 20%)
	// Criteria: urgency >= 4 OR abs(sentiment) >= 0.7
	isHighImportance := result.Urgency >= 4 || abs(result.Sentiment) >= 0.7
	var deepImpacts []DeepImpact

	if isHighImportance && len(result.Tickers) > 0 && a.router != nil {
		deepImpacts = a.runDeepAnalysis(ctx, news, result)
	}

	// 4. Create impacts — use deep analysis if available, otherwise use classification
	if len(deepImpacts) > 0 {
		a.saveDeepImpacts(ctx, news, deepImpacts)
	} else {
		a.saveClassificationImpacts(ctx, news, result)
	}

	// 5. Generate trading signals from impacts
	a.generateSignals(ctx, news, analysis, result)

	return nil
}

// runDeepAnalysis calls the heavy LLM for detailed impact assessment.
func (a *Aggregator) runDeepAnalysis(ctx context.Context, news *domain.News, result *classifier.ClassificationResult) []DeepImpact {
	title := news.Title
	if title == "" {
		runes := []rune(news.Content)
		if len(runes) > 100 {
			title = string(runes[:100])
		} else {
			title = news.Content
		}
	}

	prompt := prompts.AnalyzeImpact(title, news.Content, string(result.Category), result.Tickers)
	resp, deepModel, err := a.router.Generate(ctx, llm.TaskDeepAnalysis, prompts.SystemImpactAnalyzer, prompt)
	if err != nil {
		a.log.Warn("deep analysis failed, falling back to classification",
			"news_id", news.ID,
			"error", err,
		)
		return nil
	}

	a.log.Info("deep analysis completed",
		"news_id", news.ID,
		"model", deepModel,
	)

	// Parse deep analysis response
	resp = extractJSON(resp)
	var impacts []DeepImpact
	if err := json.Unmarshal([]byte(resp), &impacts); err != nil {
		a.log.Warn("failed to parse deep analysis JSON",
			"news_id", news.ID,
			"error", err,
		)
		return nil
	}
	return impacts
}

// saveDeepImpacts persists impacts from deep analysis.
func (a *Aggregator) saveDeepImpacts(ctx context.Context, news *domain.News, deepImpacts []DeepImpact) {
	for _, di := range deepImpacts {
		company, err := a.companyRepo.GetByTicker(ctx, di.Ticker)
		if err != nil || company == nil {
			continue
		}

		direction := domain.ImpactNeutral
		if di.ImpactDirection > 0 {
			direction = domain.ImpactPositive
		} else if di.ImpactDirection < 0 {
			direction = domain.ImpactNegative
		}

		timeframe := domain.TimeframeShort
		switch strings.ToUpper(di.ImpactTimeframe) {
		case "IMMEDIATE":
			timeframe = domain.TimeframeImmediate
		case "MEDIUM":
			timeframe = domain.TimeframeMedium
		case "LONG":
			timeframe = domain.TimeframeLong
		}

		impact := &domain.NewsImpact{
			NewsID:     news.ID,
			EntityType: domain.EntityCompany,
			EntityID:   company.ID,
			Direction:  direction,
			Magnitude:  di.ImpactMagnitude,
			Timeframe:  timeframe,
			Confidence: di.Confidence,
			Reasoning:  di.Reasoning,
		}

		if err := a.newsRepo.SaveImpact(ctx, impact); err != nil {
			a.log.Error("failed to save deep impact",
				"news_id", news.ID,
				"ticker", di.Ticker,
				"error", err,
			)
			continue
		}

		if err := a.scorer.UpdateFromImpact(ctx, impact); err != nil {
			a.log.Error("failed to update heat score",
				"news_id", news.ID,
				"ticker", di.Ticker,
				"error", err,
			)
		}
	}
}

// saveClassificationImpacts persists impacts from basic classification.
func (a *Aggregator) saveClassificationImpacts(ctx context.Context, news *domain.News, result *classifier.ClassificationResult) {
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

		if err := a.scorer.UpdateFromImpact(ctx, impact); err != nil {
			a.log.Error("failed to update heat score",
				"news_id", news.ID,
				"ticker", ticker,
				"error", err,
			)
		}
	}
}

// generateSignals creates trading signals from analyzed news.
func (a *Aggregator) generateSignals(ctx context.Context, news *domain.News, analysis *domain.NewsAnalysis, result *classifier.ClassificationResult) {
	if a.signalRepo == nil {
		return
	}

	// Only generate signals for urgent/impactful news
	if result.Urgency < 3 {
		return
	}
	absSentiment := abs(result.Sentiment)
	if absSentiment < 0.4 {
		return
	}

	now := time.Now()

	for _, ticker := range result.Tickers {
		direction := domain.SignalHold
		if result.Sentiment > 0.1 {
			direction = domain.SignalBuy
		} else if result.Sentiment < -0.1 {
			direction = domain.SignalSell
		}
		if direction == domain.SignalHold {
			continue
		}

		// Strength: combine urgency, sentiment, reliability
		strength := 0.3*float64(result.Urgency)/5.0 + 0.4*absSentiment + 0.3*result.Reliability
		if strength > 1 {
			strength = 1
		}

		sig := &domain.Signal{
			Ticker:    ticker,
			Direction: direction,
			Source:    domain.SourceNews,
			Strength:  strength,
			Reason:    fmt.Sprintf("[%s] %s (urgency=%d sentiment=%.2f)", result.Category, result.Summary, result.Urgency, result.Sentiment),
			CreatedAt: now,
			ExpiresAt: now.Add(2 * time.Hour),
		}

		if _, err := a.signalRepo.Insert(ctx, sig); err != nil {
			a.log.Error("failed to save signal",
				"news_id", news.ID,
				"ticker", ticker,
				"error", err,
			)
		} else {
			a.log.Info("news signal generated",
				"news_id", news.ID,
				"ticker", ticker,
				"direction", direction,
				"strength", fmt.Sprintf("%.2f", strength),
			)
		}
	}
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
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
