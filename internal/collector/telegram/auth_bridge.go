package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis keys for auth bridge.
const (
	keyAuthState = "telegram:auth:state"
	keyAuthError = "telegram:auth:error"
	keyAuthCode  = "telegram:auth:code_queue"
	keyAuthPass  = "telegram:auth:pass_queue"
)

// AuthState represents the current state of Telegram authentication.
type AuthState string

const (
	AuthStateIdle            AuthState = "idle"
	AuthStatePendingCode     AuthState = "pending_code"
	AuthStatePendingPassword AuthState = "pending_password"
	AuthStateAuthenticated   AuthState = "authenticated"
	AuthStateError           AuthState = "error"
)

// AuthBridge allows submitting Telegram auth codes via web API instead of stdin.
// Uses Redis for inter-process communication so the API server can relay codes
// to the collector service where the MTProto client runs.
type AuthBridge struct {
	rdb     *redis.Client
	phone   string
	log     *slog.Logger
	timeout time.Duration
}

// NewAuthBridge creates a Redis-backed auth bridge for the given phone number.
func NewAuthBridge(rdb *redis.Client, phone string, log *slog.Logger) *AuthBridge {
	return &AuthBridge{
		rdb:     rdb,
		phone:   phone,
		log:     log,
		timeout: 5 * time.Minute,
	}
}

// State returns the current auth state from Redis.
func (b *AuthBridge) State(ctx context.Context) AuthState {
	val, err := b.rdb.Get(ctx, keyAuthState).Result()
	if err != nil {
		return AuthStateIdle
	}
	return AuthState(val)
}

// LastError returns the last error message from Redis.
func (b *AuthBridge) LastError(ctx context.Context) string {
	val, _ := b.rdb.Get(ctx, keyAuthError).Result()
	return val
}

func (b *AuthBridge) setState(state AuthState) {
	ctx := context.Background()
	b.rdb.Set(ctx, keyAuthState, string(state), 10*time.Minute)
	if state != AuthStateError {
		b.rdb.Del(ctx, keyAuthError)
	}
}

func (b *AuthBridge) setError(msg string) {
	ctx := context.Background()
	b.rdb.Set(ctx, keyAuthState, string(AuthStateError), 10*time.Minute)
	b.rdb.Set(ctx, keyAuthError, msg, 10*time.Minute)
}

// SetAuthenticated marks authentication as complete.
func (b *AuthBridge) SetAuthenticated() {
	b.setState(AuthStateAuthenticated)
}

// SetError marks authentication as failed.
func (b *AuthBridge) SetError(msg string) {
	b.setError(msg)
}

// SubmitCode submits an auth code from the web API side.
// The collector's AuthBridge.Code() is blocking on BRPOP for this.
func (b *AuthBridge) SubmitCode(ctx context.Context, code string) error {
	state := b.State(ctx)
	if state != AuthStatePendingCode {
		return fmt.Errorf("auth code not expected (state: %s)", state)
	}
	return b.rdb.LPush(ctx, keyAuthCode, code).Err()
}

// SubmitPassword submits a 2FA password from the web API side.
func (b *AuthBridge) SubmitPassword(ctx context.Context, password string) error {
	state := b.State(ctx)
	if state != AuthStatePendingPassword {
		return fmt.Errorf("2FA password not expected (state: %s)", state)
	}
	return b.rdb.LPush(ctx, keyAuthPass, password).Err()
}

// --- mtproto.AuthDataProvider interface ---

func (b *AuthBridge) PhoneNumber() (string, error) {
	return b.phone, nil
}

func (b *AuthBridge) Code() (string, error) {
	b.setState(AuthStatePendingCode)

	// Clear any stale values in the queue.
	b.rdb.Del(context.Background(), keyAuthCode)

	b.log.Warn("telegram auth code requested — submit via admin panel /settings")

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	result, err := b.rdb.BRPop(ctx, b.timeout, keyAuthCode).Result()
	if err != nil {
		b.setError("auth code timeout (5 min)")
		return "", fmt.Errorf("auth code timeout: no code submitted within %v", b.timeout)
	}

	code := result[1]
	b.log.Info("auth code received via web")
	return code, nil
}

func (b *AuthBridge) Password() (string, error) {
	b.setState(AuthStatePendingPassword)

	// Clear any stale values in the queue.
	b.rdb.Del(context.Background(), keyAuthPass)

	b.log.Warn("telegram 2FA password requested — submit via admin panel /settings")

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	result, err := b.rdb.BRPop(ctx, b.timeout, keyAuthPass).Result()
	if err != nil {
		b.setError("2FA password timeout (5 min)")
		return "", fmt.Errorf("2FA password timeout: no password submitted within %v", b.timeout)
	}

	pass := result[1]
	b.log.Info("2FA password received via web")
	return pass, nil
}
