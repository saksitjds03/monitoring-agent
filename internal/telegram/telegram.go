package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Client struct {
	token  string
	chatID string
	client *http.Client
}

type sendMessagePayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func NewClient(token, chatID string) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SendAlert(title, message string) error {
	if c.token == "" || c.chatID == "" {
		return fmt.Errorf("telegram token or chat ID is missing")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)

	// Format as a simple alert
	formattedMessage := fmt.Sprintf("%s\n\n%s", title, message)

	payload := sendMessagePayload{
		ChatID:    c.chatID,
		Text:      formattedMessage,
		ParseMode: "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Failed to send Telegram message", "status", resp.Status)
		return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
	}

	slog.Info("Telegram alert sent successfully")
	return nil
}
