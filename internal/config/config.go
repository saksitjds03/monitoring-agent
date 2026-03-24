package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	PollIntervalMs  int               `json:"poll_interval_ms"`
	StatsIntervalMs int               `json:"stats_interval_ms"`
	HTTPTimeoutMs   int               `json:"http_timeout_ms"`
	MQTTBrokerURL   string            `json:"mqtt_broker_url"`
	DeviceID        int               `json:"device_id"`
	TypeID          int               `json:"type_id"`
	TelegramBotToken string            `json:"telegram_bot_token"`
	TelegramChatID   string            `json:"telegram_chat_id"`
	Containers      []ContainerConfig `json:"containers"`
}

type ContainerConfig struct {
	ContainerName  string   `json:"container_name"`
	HealthCheckURL string   `json:"healthcheck_url"`
	LogKeywords    []string `json:"log_keywords"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&cfg)
	if err != nil {
		return nil, err
	}

	// Expand environment variables in healthcheck URLs
	for i := range cfg.Containers {
		cfg.Containers[i].HealthCheckURL = os.ExpandEnv(cfg.Containers[i].HealthCheckURL)
	}

	// Override Telegram config with Env vars if present
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		cfg.TelegramBotToken = token
	}
	if chatID := os.Getenv("TELEGRAM_CHAT_ID"); chatID != "" {
		cfg.TelegramChatID = chatID
	}

	return &cfg, nil
}
