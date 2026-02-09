package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/postgres"
	"github.com/NordeN37/MarketPulse_RU/internal/storage/redis"
)

// Sender delivers alerts to Telegram.
type Sender struct {
	bot       *tgbotapi.BotAPI
	chatID    int64
	alertRepo *postgres.AlertRepo
	cache     *redis.Client
	log       *slog.Logger
}

// NewSender creates a new Telegram alert sender.
func NewSender(cfg config.TelegramConfig, alertRepo *postgres.AlertRepo, cache *redis.Client, log *slog.Logger) (*Sender, error) {
	if cfg.BotToken == "" {
		return &Sender{
			chatID:    cfg.AlertChatID,
			alertRepo: alertRepo,
			cache:     cache,
			log:       log,
		}, nil
	}

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}

	log.Info("telegram bot authorized", "username", bot.Self.UserName)

	return &Sender{
		bot:       bot,
		chatID:    cfg.AlertChatID,
		alertRepo: alertRepo,
		cache:     cache,
		log:       log,
	}, nil
}

// Run starts the alert sending loop. It blocks until context is cancelled.
func (s *Sender) Run(ctx context.Context, interval time.Duration) error {
	s.log.Info("starting alert sender", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.sendPending(ctx)
		}
	}
}

func (s *Sender) sendPending(ctx context.Context) {
	alerts, err := s.alertRepo.GetUnsent(ctx, 20)
	if err != nil {
		s.log.Error("failed to get unsent alerts", "error", err)
		return
	}

	for _, alert := range alerts {
		if err := s.sendAlert(ctx, &alert); err != nil {
			s.log.Error("failed to send alert",
				"alert_id", alert.ID,
				"error", err,
			)
			continue
		}

		if err := s.alertRepo.MarkSent(ctx, alert.ID); err != nil {
			s.log.Error("failed to mark alert sent",
				"alert_id", alert.ID,
				"error", err,
			)
		}
	}
}

func (s *Sender) sendAlert(ctx context.Context, alert *domain.Alert) error {
	if s.bot == nil {
		s.log.Warn("telegram bot not configured, skipping alert send", "alert_id", alert.ID)
		return nil
	}

	text := formatAlertMessage(alert)

	msg := tgbotapi.NewMessage(s.chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true

	_, err := s.bot.Send(msg)
	return err
}

func formatAlertMessage(alert *domain.Alert) string {
	var b strings.Builder

	// Severity emoji
	switch alert.Severity {
	case domain.SeverityCritical:
		b.WriteString("🚨 <b>CRITICAL</b>\n")
	case domain.SeverityUrgent:
		b.WriteString("⚠️ <b>URGENT</b>\n")
	case domain.SeverityImportant:
		b.WriteString("📌 <b>IMPORTANT</b>\n")
	default:
		b.WriteString("ℹ️ <b>INFO</b>\n")
	}

	b.WriteString(fmt.Sprintf("\n<b>%s</b>\n", alert.Title))
	b.WriteString(fmt.Sprintf("\n%s\n", alert.Description))
	b.WriteString(fmt.Sprintf("\n<i>Тип: %s</i>", alert.AlertType))
	b.WriteString(fmt.Sprintf("\n<i>%s</i>", alert.CreatedAt.Format("02.01.2006 15:04 MSK")))

	return b.String()
}
