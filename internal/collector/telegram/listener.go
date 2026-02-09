package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// MessageHandler is called for each new message from monitored channels.
type MessageHandler func(ctx context.Context, news *domain.News) error

// Listener monitors Telegram channels for new messages.
// Uses the Bot API approach: the bot must be added to monitored channels as admin.
// For full MTProto userbot support (reading public channels without being admin),
// a separate implementation can be plugged in later using gotd/td.
type Listener struct {
	cfg     config.TelegramConfig
	handler MessageHandler
	log     *slog.Logger
}

// NewListener creates a new Telegram channel listener.
func NewListener(cfg config.TelegramConfig, handler MessageHandler, log *slog.Logger) *Listener {
	return &Listener{
		cfg:     cfg,
		handler: handler,
		log:     log,
	}
}

// Run starts listening for channel messages via Bot API. Blocks until context is cancelled.
func (l *Listener) Run(ctx context.Context) error {
	if l.cfg.BotToken == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}

	bot, err := tgbotapi.NewBotAPI(l.cfg.BotToken)
	if err != nil {
		return fmt.Errorf("creating telegram bot: %w", err)
	}

	l.log.Info("telegram bot authorized",
		"username", bot.Self.UserName,
		"channels", l.cfg.Channels,
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			l.log.Info("telegram listener shutting down")
			bot.StopReceivingUpdates()
			return nil

		case update := <-updates:
			if update.ChannelPost == nil {
				continue
			}

			msg := update.ChannelPost
			news := l.parseMessage(msg)
			if news == nil {
				continue
			}

			if err := l.handler(ctx, news); err != nil {
				l.log.Error("failed to handle telegram message",
					"channel", news.SourceChannel,
					"msg_id", msg.MessageID,
					"error", err,
				)
			}
		}
	}
}

func (l *Listener) parseMessage(msg *tgbotapi.Message) *domain.News {
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}
	if content == "" {
		return nil
	}

	channelName := ""
	if msg.Chat != nil {
		channelName = msg.Chat.UserName
		if channelName == "" {
			channelName = msg.Chat.Title
		}
	}

	rawData, _ := json.Marshal(map[string]any{
		"message_id": msg.MessageID,
		"channel":    channelName,
		"chat_id":    msg.Chat.ID,
		"date":       msg.Date,
	})

	return &domain.News{
		ExternalID:    strconv.Itoa(msg.MessageID) + "_" + strconv.FormatInt(msg.Chat.ID, 10),
		Source:        domain.SourceTelegram,
		SourceChannel: channelName,
		Title:         "",
		Content:       content,
		PublishedAt:   time.Unix(int64(msg.Date), 0),
		CollectedAt:   time.Now(),
		RawJSON:       rawData,
	}
}
