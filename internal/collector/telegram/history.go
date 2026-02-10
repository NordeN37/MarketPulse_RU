package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/3bl3gamer/tgclient"
	"github.com/3bl3gamer/tgclient/mtproto"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// HistoryReader reads historical messages from Telegram channels via MTProto.
type HistoryReader struct {
	cfg        config.TelegramConfig
	handler    MessageHandler
	log        *slog.Logger
	authBridge *AuthBridge
	sessPath   string
}

// NewHistoryReader creates a reader for fetching channel history.
func NewHistoryReader(cfg config.TelegramConfig, handler MessageHandler, log *slog.Logger) *HistoryReader {
	return &HistoryReader{
		cfg:      cfg,
		handler:  handler,
		log:      log,
		sessPath: "data/tg.session",
	}
}

// WithAuthBridge sets a shared AuthBridge for web-based auth code input.
func (h *HistoryReader) WithAuthBridge(ab *AuthBridge) *HistoryReader {
	h.authBridge = ab
	return h
}

// WithSessionPath overrides the default session file path.
func (h *HistoryReader) WithSessionPath(path string) *HistoryReader {
	h.sessPath = path
	return h
}

// ReadHistory connects to Telegram, reads message history from all configured channels,
// and passes each message through the handler. Fetches up to `maxMessages` per channel
// (going back in time from newest). Set maxMessages=0 for unlimited.
func (h *HistoryReader) ReadHistory(ctx context.Context, maxMessages int) error {
	if h.cfg.APIID == 0 || h.cfg.APIHash == "" {
		return fmt.Errorf("telegram api_id and api_hash required for history reading")
	}

	h.log.Info("starting telegram history reader",
		"channels", h.cfg.Channels,
		"max_per_channel", maxMessages,
		"session_path", h.sessPath,
	)

	client := newClientWithSession(int32(h.cfg.APIID), h.cfg.APIHash, h.sessPath, h.log)

	if err := client.InitAndConnect(); err != nil {
		return fmt.Errorf("connecting to telegram: %w", err)
	}
	defer client.Disconnect()

	var authData mtproto.AuthDataProvider
	if h.authBridge != nil {
		authData = h.authBridge
	} else {
		authData = &phoneAuth{phone: h.cfg.Phone, log: h.log}
	}
	if err := client.AuthAndInitEvents(authData); err != nil {
		if h.authBridge != nil {
			h.authBridge.SetError(err.Error())
		}
		return fmt.Errorf("authenticating: %w", err)
	}
	if h.authBridge != nil {
		h.authBridge.SetAuthenticated()
	}

	h.log.Info("telegram history reader authenticated")

	var totalMessages int
	for _, ch := range h.cfg.Channels {
		if ctx.Err() != nil {
			break
		}
		count, err := h.readChannelHistory(ctx, client, ch, maxMessages)
		if err != nil {
			h.log.Error("error reading channel history", "channel", ch, "error", err)
			continue
		}
		totalMessages += count
		h.log.Info("channel history read", "channel", ch, "messages", count)
	}

	h.log.Info("history reading complete", "total_messages", totalMessages)
	return nil
}

// readChannelHistory fetches history from a single channel.
func (h *HistoryReader) readChannelHistory(ctx context.Context, client *tgclient.TGClient, username string, maxMessages int) (int, error) {
	// Resolve channel.
	res := client.SendSync(mtproto.TL_contacts_resolveUsername{
		Username: username,
	})

	resolved, ok := res.(mtproto.TL_contacts_resolvedPeer)
	if !ok {
		return 0, fmt.Errorf("could not resolve channel @%s: %T", username, res)
	}

	var channelID int64
	var accessHash int64
	for _, chat := range resolved.Chats {
		if channel, ok := chat.(mtproto.TL_channel); ok {
			channelID = channel.ID
			if channel.AccessHash != nil {
				accessHash = *channel.AccessHash
			}
			break
		}
	}
	if channelID == 0 {
		return 0, fmt.Errorf("channel @%s not found in resolved peers", username)
	}

	inputChannel := mtproto.TL_inputChannel{
		ChannelID:  channelID,
		AccessHash: accessHash,
	}

	var total int
	var offsetID int32

	for {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		if maxMessages > 0 && total >= maxMessages {
			break
		}

		batchSize := 100
		if maxMessages > 0 && (maxMessages-total) < batchSize {
			batchSize = maxMessages - total
		}

		// Use messages.getHistory to fetch batches of messages.
		res := client.SendSync(mtproto.TL_messages_getHistory{
			Peer: mtproto.TL_inputPeerChannel{
				ChannelID:  channelID,
				AccessHash: accessHash,
			},
			OffsetID:  offsetID,
			Limit:     int32(batchSize),
			AddOffset: 0,
		})

		messages, err := extractMessages(res)
		if err != nil {
			return total, fmt.Errorf("extracting messages: %w", err)
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			if ctx.Err() != nil {
				return total, ctx.Err()
			}
			if msg.Message == "" {
				continue
			}

			rawData, _ := json.Marshal(map[string]any{
				"message_id": msg.ID,
				"channel":    username,
				"channel_id": channelID,
				"history":    true,
			})

			news := &domain.News{
				ExternalID:    fmt.Sprintf("%d_%d", channelID, msg.ID),
				Source:        domain.SourceTelegram,
				SourceChannel: username,
				Title:         "",
				Content:       msg.Message,
				PublishedAt:   time.Unix(int64(msg.Date), 0),
				CollectedAt:   time.Now(),
				RawJSON:       rawData,
			}

			if err := h.handler(ctx, news); err != nil {
				h.log.Debug("handler error for history message", "msg_id", msg.ID, "error", err)
			}
			total++
		}

		// Set offset to oldest message ID for pagination.
		offsetID = messages[len(messages)-1].ID

		h.log.Debug("history batch processed",
			"channel", username,
			"batch_size", len(messages),
			"total", total,
			"oldest_id", offsetID,
		)

		// Rate limit: small delay between batches.
		time.Sleep(500 * time.Millisecond)

		_ = inputChannel // used for resolving
	}

	return total, nil
}

// extractMessages pulls TL_message objects from a messages response.
func extractMessages(res mtproto.TL) ([]mtproto.TL_message, error) {
	var messages []mtproto.TL_message

	switch r := res.(type) {
	case mtproto.TL_messages_messages:
		for _, m := range r.Messages {
			if msg, ok := m.(mtproto.TL_message); ok {
				messages = append(messages, msg)
			}
		}
	case mtproto.TL_messages_messagesSlice:
		for _, m := range r.Messages {
			if msg, ok := m.(mtproto.TL_message); ok {
				messages = append(messages, msg)
			}
		}
	case mtproto.TL_messages_channelMessages:
		for _, m := range r.Messages {
			if msg, ok := m.(mtproto.TL_message); ok {
				messages = append(messages, msg)
			}
		}
	default:
		return nil, fmt.Errorf("unexpected messages response type: %T", res)
	}

	return messages, nil
}
