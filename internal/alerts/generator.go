package alerts

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// Generator creates alerts based on news analysis results.
type Generator struct {
	alertRepo *postgres.AlertRepo
	cache     *redis.Client
	log       *slog.Logger
}

// NewGenerator creates a new alert generator.
func NewGenerator(alertRepo *postgres.AlertRepo, cache *redis.Client, log *slog.Logger) *Generator {
	return &Generator{
		alertRepo: alertRepo,
		cache:     cache,
		log:       log,
	}
}

// EvaluateNews checks if a news analysis should trigger an alert.
func (g *Generator) EvaluateNews(ctx context.Context, news *domain.News, analysis *domain.NewsAnalysis, impacts []domain.NewsImpact) error {
	severity := determineSeverity(analysis)
	if severity == "" {
		return nil // no alert needed
	}

	for _, impact := range impacts {
		alert := &domain.Alert{
			AlertType:     string(analysis.Category),
			EntityType:    impact.EntityType,
			EntityID:      impact.EntityID,
			Severity:      severity,
			Title:         generateAlertTitle(analysis, &impact),
			Description:   analysis.SummaryRU,
			SourceNewsIDs: []int64{news.ID},
		}

		id, err := g.alertRepo.Insert(ctx, alert)
		if err != nil {
			g.log.Error("failed to insert alert",
				"news_id", news.ID,
				"error", err,
			)
			continue
		}

		g.log.Info("alert created",
			"alert_id", id,
			"severity", severity,
			"entity_type", impact.EntityType,
			"entity_id", impact.EntityID,
		)

		// Publish for real-time delivery
		if err := g.cache.PublishAlert(ctx, id); err != nil {
			g.log.Error("failed to publish alert", "alert_id", id, "error", err)
		}
	}

	return nil
}

// determineSeverity maps urgency and category to alert severity.
func determineSeverity(analysis *domain.NewsAnalysis) domain.AlertSeverity {
	// Critical: sanctions, defaults, major CB rate changes
	criticalCategories := map[domain.NewsCategory]bool{
		domain.CategoryGeoSanctions: true,
		domain.CategoryCorpDebt:     true,
	}

	if criticalCategories[analysis.Category] && analysis.Urgency >= 4 {
		return domain.SeverityCritical
	}

	if analysis.Category == domain.CategoryCBRate && analysis.Urgency >= 4 {
		return domain.SeverityCritical
	}

	// Urgent
	if analysis.Urgency >= 4 {
		return domain.SeverityUrgent
	}

	// Important
	urgentCategories := map[domain.NewsCategory]bool{
		domain.CategoryCorpEarnings:   true,
		domain.CategoryCorpDividend:   true,
		domain.CategoryCorpMA:         true,
		domain.CategoryCorpRating:     true,
		domain.CategoryCBPolicy:       true,
		domain.CategoryGeoDiplomacy:   true,
	}

	if urgentCategories[analysis.Category] && analysis.Urgency >= 3 {
		return domain.SeverityImportant
	}

	// INFO for urgency >= 2
	if analysis.Urgency >= 2 {
		return domain.SeverityInfo
	}

	return "" // no alert
}

func generateAlertTitle(analysis *domain.NewsAnalysis, impact *domain.NewsImpact) string {
	direction := "⚪"
	switch impact.Direction {
	case domain.ImpactPositive:
		direction = "🟢"
	case domain.ImpactNegative:
		direction = "🔴"
	}

	return fmt.Sprintf("%s [%s] %s", direction, analysis.Category, analysis.SummaryRU)
}
