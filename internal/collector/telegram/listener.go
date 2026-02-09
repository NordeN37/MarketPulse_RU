package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/3bl3gamer/tgclient"
	"github.com/3bl3gamer/tgclient/mtproto"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// MessageHandler is called for each new message from monitored channels.
type MessageHandler func(ctx context.Context, news *domain.News) error

// Userbot monitors Telegram channels as a regular user via MTProto.
// This allows reading public channels without being added as admin.
// Requires api_id/api_hash from https://my.telegram.org and phone number.
type Userbot struct {
	cfg     config.TelegramConfig
	handler MessageHandler
	log     *slog.Logger
	// resolved channel IDs for filtering updates
	channelIDs map[int64]string // channelID -> username
}

// NewUserbot creates a new MTProto-based channel listener.
func NewUserbot(cfg config.TelegramConfig, handler MessageHandler, log *slog.Logger) *Userbot {
	return &Userbot{
		cfg:        cfg,
		handler:    handler,
		log:        log,
		channelIDs: make(map[int64]string),
	}
}

// slogHandler adapts slog.Logger to tgclient's mtproto.LogHandler interface.
type slogHandler struct {
	log *slog.Logger
}

func (h *slogHandler) Log(level mtproto.LogLevel, err error, msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	switch level {
	case mtproto.ERROR:
		h.log.Error(formatted, "error", err)
	case mtproto.WARN:
		h.log.Warn(formatted)
	case mtproto.INFO:
		h.log.Info(formatted)
	default:
		h.log.Debug(formatted)
	}
}

func (h *slogHandler) Message(isIncoming bool, msg mtproto.TL, id int64) {
	// Suppress verbose protocol-level messages
}

// Run starts the MTProto client, resolves channels, and listens for updates.
// Blocks until context is cancelled.
func (u *Userbot) Run(ctx context.Context) error {
	if u.cfg.APIID == 0 || u.cfg.APIHash == "" {
		return fmt.Errorf("telegram api_id and api_hash are required for userbot mode")
	}

	u.log.Info("starting telegram userbot (MTProto)",
		"api_id", u.cfg.APIID,
		"channels", u.cfg.Channels,
	)

	client := tgclient.NewTGClient(int32(u.cfg.APIID), u.cfg.APIHash, &slogHandler{log: u.log})

	// Set update handler for incoming channel messages
	client.SetUpdateHandler(func(update mtproto.TL) {
		u.handleUpdate(ctx, update)
	})

	// Connect and authenticate
	if err := client.InitAndConnect(); err != nil {
		return fmt.Errorf("connecting to telegram: %w", err)
	}
	defer client.Disconnect()

	// Authenticate using phone number
	authData := &phoneAuth{phone: u.cfg.Phone, log: u.log}
	if err := client.AuthAndInitEvents(authData); err != nil {
		return fmt.Errorf("authenticating: %w", err)
	}

	u.log.Info("telegram userbot authenticated")

	// Resolve channel usernames to IDs and join them
	u.resolveAndJoinChannels(client)

	u.log.Info("userbot listening for channel updates",
		"tracked_channels", len(u.channelIDs),
	)

	// Block until context is done
	<-ctx.Done()
	u.log.Info("telegram userbot shutting down")
	return nil
}

// resolveAndJoinChannels resolves channel usernames and joins them.
func (u *Userbot) resolveAndJoinChannels(client *tgclient.TGClient) {
	for _, ch := range u.cfg.Channels {
		username := strings.TrimPrefix(ch, "@")
		if username == "" {
			continue
		}

		res := client.SendSync(mtproto.TL_contacts_resolveUsername{
			Username: username,
		})

		switch resolved := res.(type) {
		case mtproto.TL_contacts_resolvedPeer:
			// Extract channel from the resolved peer
			for _, chat := range resolved.Chats {
				if channel, ok := chat.(mtproto.TL_channel); ok {
					u.channelIDs[channel.ID] = username
					u.log.Info("resolved channel",
						"username", username,
						"id", channel.ID,
						"title", channel.Title,
					)

					// Join channel if not already a member
					var accessHash int64
					if channel.AccessHash != nil {
						accessHash = *channel.AccessHash
					}
					joinRes := client.SendSync(mtproto.TL_channels_joinChannel{
						Channel: mtproto.TL_inputChannel{
							ChannelID:  channel.ID,
							AccessHash: accessHash,
						},
					})
					if mtproto.IsError(joinRes, "") {
						u.log.Warn("could not join channel (may already be joined)",
							"username", username,
							"result", fmt.Sprintf("%T", joinRes),
						)
					} else {
						u.log.Info("joined channel", "username", username)
					}
				}
			}
		default:
			u.log.Error("failed to resolve channel",
				"username", username,
				"result", fmt.Sprintf("%T: %v", res, res),
			)
		}
	}
}

// handleUpdate processes incoming Telegram updates.
func (u *Userbot) handleUpdate(ctx context.Context, update mtproto.TL) {
	switch upd := update.(type) {
	case mtproto.TL_updateNewChannelMessage:
		u.handleChannelMessage(ctx, upd)
	}
}

func (u *Userbot) handleChannelMessage(ctx context.Context, upd mtproto.TL_updateNewChannelMessage) {
	msg, ok := upd.Message.(mtproto.TL_message)
	if !ok {
		return
	}

	// Extract channel ID from PeerID
	peer, ok := msg.PeerID.(mtproto.TL_peerChannel)
	if !ok {
		return
	}

	// Check if this channel is in our monitored list
	channelName, tracked := u.channelIDs[peer.ChannelID]
	if !tracked {
		return
	}

	if msg.Message == "" {
		return
	}

	views := 0
	if msg.Views != nil {
		views = int(*msg.Views)
	}
	forwards := 0
	if msg.Forwards != nil {
		forwards = int(*msg.Forwards)
	}

	rawData, _ := json.Marshal(map[string]any{
		"message_id": msg.ID,
		"channel":    channelName,
		"channel_id": peer.ChannelID,
		"views":      views,
		"forwards":   forwards,
		"post":       msg.Post,
	})

	news := &domain.News{
		ExternalID:    fmt.Sprintf("%d_%d", peer.ChannelID, msg.ID),
		Source:        domain.SourceTelegram,
		SourceChannel: channelName,
		Title:         "",
		Content:       msg.Message,
		PublishedAt:   time.Unix(int64(msg.Date), 0),
		CollectedAt:   time.Now(),
		RawJSON:       rawData,
	}

	if err := u.handler(ctx, news); err != nil {
		u.log.Error("failed to handle channel message",
			"channel", channelName,
			"msg_id", msg.ID,
			"error", err,
		)
	}
}

// phoneAuth provides phone-based authentication data for MTProto.
type phoneAuth struct {
	phone string
	log   *slog.Logger
}

func (a *phoneAuth) PhoneNumber() (string, error) {
	return a.phone, nil
}

func (a *phoneAuth) Code() (string, error) {
	// In production, this should read from stdin or a callback.
	// For automated operation, the session file is reused after first auth.
	a.log.Warn("telegram auth code requested — enter code via stdin")
	var code string
	fmt.Print("Enter Telegram auth code: ")
	_, err := fmt.Scanln(&code)
	return code, err
}

func (a *phoneAuth) Password() (string, error) {
	a.log.Warn("telegram 2FA password requested — enter via stdin")
	var password string
	fmt.Print("Enter 2FA password: ")
	_, err := fmt.Scanln(&password)
	return password, err
}

// --- Bot API fallback (for sending alerts) ---

// BotSender wraps the Bot API for sending messages (alerts).
// Channel reading uses MTProto (Userbot), alert delivery uses Bot API.
type BotSender struct {
	// Kept minimal — alert sending is handled in internal/alerts/sender.go
}

// FormatExternalID creates a unique ID from channel+message.
func FormatExternalID(channelID int64, messageID int32) string {
	return strconv.FormatInt(channelID, 10) + "_" + strconv.Itoa(int(messageID))
}
