package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	App       AppConfig       `yaml:"app"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Telegram  TelegramConfig  `yaml:"telegram"`
	LLM       LLMConfig       `yaml:"llm"`
	Collector CollectorConfig `yaml:"collector"`
	Analyzer  AnalyzerConfig  `yaml:"analyzer"`
	Alerts    AlertsConfig    `yaml:"alerts"`
	API       APIConfig       `yaml:"api"`
	MOEX      MOEXConfig      `yaml:"moex"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	Env      string `yaml:"env"`
	LogLevel string `yaml:"log_level"`
}

type DatabaseConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	Name         string `yaml:"name"`
	SSLMode      string `yaml:"sslmode"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type TelegramConfig struct {
	// MTProto credentials for userbot mode (from https://my.telegram.org)
	APIID   int    `yaml:"api_id"`
	APIHash string `yaml:"api_hash"`
	Phone   string `yaml:"phone"`
	// Channels to monitor (userbot subscribes to them via MTProto)
	Channels []string `yaml:"channels"`
	// Bot token for sending alerts (separate from userbot)
	BotToken    string `yaml:"bot_token"`
	AlertChatID int64  `yaml:"alert_chat_id"`
}

// LLMConfig supports multiple providers with priority-based fallback.
type LLMConfig struct {
	// Ollama fast model for routine tasks (classify, NER, sentiment)
	Ollama OllamaConfig `yaml:"ollama"`
	// Ollama heavy model for complex tasks on CPU (optional)
	OllamaHeavy OllamaConfig `yaml:"ollama_heavy"`
	// DeepSeek API (OpenAI-compatible, cheap)
	DeepSeek OpenAIProviderConfig `yaml:"deepseek"`
	// Claude API (highest quality)
	Claude ClaudeConfig `yaml:"claude"`
	// Extra OpenAI-compatible providers (OpenRouter, local vLLM, etc.)
	ExtraProviders []OpenAIProviderConfig `yaml:"extra_providers"`
}

type OllamaConfig struct {
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	// Thinking controls Qwen3's reasoning mode.
	// false = fast non-thinking (for classification), true = deep reasoning (for analysis).
	Thinking *bool `yaml:"thinking"`
}

// ThinkingEnabled returns whether thinking mode is enabled (default: false).
func (o OllamaConfig) ThinkingEnabled() bool {
	if o.Thinking == nil {
		return false
	}
	return *o.Thinking
}

func (o OllamaConfig) Timeout() time.Duration {
	if o.TimeoutSeconds == 0 {
		return 60 * time.Second
	}
	return time.Duration(o.TimeoutSeconds) * time.Second
}

type ClaudeConfig struct {
	APIKey    string `yaml:"api_key"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// OpenAIProviderConfig works with any OpenAI-compatible API.
type OpenAIProviderConfig struct {
	Name           string `yaml:"name"`
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key"`
	Model          string `yaml:"model"`
	MaxTokens      int    `yaml:"max_tokens"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type CollectorConfig struct {
	RSSPollInterval string `yaml:"rss_poll_interval"`
	DedupWindow     string `yaml:"dedup_window"`
	BatchSize       int    `yaml:"batch_size"`
}

func (c CollectorConfig) PollInterval() time.Duration {
	d, _ := time.ParseDuration(c.RSSPollInterval)
	if d == 0 {
		return 5 * time.Minute
	}
	return d
}

func (c CollectorConfig) DedupDuration() time.Duration {
	d, _ := time.ParseDuration(c.DedupWindow)
	if d == 0 {
		return 24 * time.Hour
	}
	return d
}

type AnalyzerConfig struct {
	Workers    int    `yaml:"workers"`
	LLMTimeout string `yaml:"llm_timeout"`
}

func (a AnalyzerConfig) Timeout() time.Duration {
	d, _ := time.ParseDuration(a.LLMTimeout)
	if d == 0 {
		return 30 * time.Second
	}
	return d
}

type AlertsConfig struct {
	CheckInterval              string  `yaml:"check_interval"`
	CriticalSentimentThreshold float64 `yaml:"critical_sentiment_threshold"`
	UrgentSentimentThreshold   float64 `yaml:"urgent_sentiment_threshold"`
}

func (a AlertsConfig) Interval() time.Duration {
	d, _ := time.ParseDuration(a.CheckInterval)
	if d == 0 {
		return 30 * time.Second
	}
	return d
}

type APIConfig struct {
	Host        string   `yaml:"host"`
	Port        int      `yaml:"port"`
	CORSOrigins []string `yaml:"cors_origins"`
}

func (a APIConfig) Addr() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

type MOEXConfig struct {
	BaseURL            string `yaml:"base_url"`
	RequestTimeout     string `yaml:"request_timeout"`
	RateLimitPerSecond int    `yaml:"rate_limit_per_second"`
}

func (m MOEXConfig) Timeout() time.Duration {
	d, _ := time.ParseDuration(m.RequestTimeout)
	if d == 0 {
		return 10 * time.Second
	}
	return d
}

// Load reads and parses the configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables in the config
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}
