package mqtt

import (
	"encoding/json"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client mqtt.Client
	topic  string
}

func NewClient(brokerURL string, clientID string) (*Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.OnConnect = func(c mqtt.Client) {
		slog.Info("Connected to MQTT broker", "broker", brokerURL)
	}
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		slog.Error("Lost connection to MQTT broker", "error", err)
	}

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &Client{client: c}, nil
}

func (c *Client) PublishAlert(deviceID int, typeID int, isOnline bool, message string, severity string) error {
	topic := "/from-HMS"

	// Create payload specifically for alert-notification-service
	payload := map[string]interface{}{
		"time":      time.Now(), // ISO string automatically
		"device_id": deviceID,
		"type_id":   typeID,
		"is_online": isOnline,
		"severity":  severity,
		"agent_id":  "container-monitor-v1",
	}

	if message != "" {
		if !isOnline {
			payload["error_message"] = message
		} else {
			payload["message"] = message
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	token := c.client.Publish(topic, 1, false, body)
	token.Wait()
	return token.Error()
}

func (c *Client) Close() {
	if c.client.IsConnected() {
		c.client.Disconnect(250)
	}
}

