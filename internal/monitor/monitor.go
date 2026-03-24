package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"agent-service-monitoring/internal/alerts"
	"agent-service-monitoring/internal/config"
	"agent-service-monitoring/internal/docker"
	"agent-service-monitoring/internal/mqtt"
	"agent-service-monitoring/internal/telegram"
)

type ContainerData struct {
	Metadata    docker.ContainerMetadata `json:"metadata"`
	Stats       docker.ContainerStats    `json:"stats"`
	LastUpdated time.Time                `json:"last_updated"`

	// MVP Fields
	IsMonitored  bool   `json:"is_monitored"`   // True if listed in config targets
	MainStatus   string `json:"main_status"`    // OK, STOPPED, UNHEALTHY, HTTP_FAIL, HTTP_TIMEOUT, HTTP_CONN_ERR
	HTTPStatus   *int   `json:"http_status"`    // 200, 500, etc. (null if not checked)
	HTTPLatency  *int64 `json:"http_latency"`   // in ms (null if not checked)
	LastErrorMsg string `json:"last_error_msg"` // Context for Telegram alerts
}

type Monitor struct {
	cfg             *config.Config
	dockerClient    *docker.DockerClient
	mqttClient      *mqtt.Client
	telegramClient  *telegram.Client
	data            map[string]*ContainerData
	alerts          *alerts.AlertManager
	pendingAlerts   map[string]time.Time
	lastNotifyTimes map[string]time.Time // Track last Telegram notification per alertKey
	mu              sync.RWMutex
	LastPollTime    time.Time
}

func NewMonitor(cfg *config.Config, cli *docker.DockerClient, mqttCli *mqtt.Client, tgCli *telegram.Client) *Monitor {
	return &Monitor{
		cfg:             cfg,
		dockerClient:    cli,
		mqttClient:      mqttCli,
		telegramClient:  tgCli,
		data:            make(map[string]*ContainerData),
		alerts:          alerts.NewAlertManager(100),
		pendingAlerts:   make(map[string]time.Time),
		lastNotifyTimes: make(map[string]time.Time),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	pollTicker := time.NewTicker(time.Duration(m.cfg.PollIntervalMs) * time.Millisecond)
	statsTicker := time.NewTicker(time.Duration(m.cfg.StatsIntervalMs) * time.Millisecond)
	// Initial poll
	m.pollContainers(ctx)

	// Poll Loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				pollTicker.Stop()
				return
			case <-pollTicker.C:
				m.pollContainers(ctx)
			}
		}
	}()

	// Stats Loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				statsTicker.Stop()
				return
			case <-statsTicker.C:
				m.collectStats(ctx)
			}
		}
	}()
}

func (m *Monitor) pollContainers(ctx context.Context) {
	containers, err := m.dockerClient.Poll(ctx)
	if err != nil {
		slog.Error("Error polling containers", "error", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Track existing IDs to clean up removed containers?
	// For now, just update.
	currentIDs := make(map[string]bool)

	for _, c := range containers {
		currentIDs[c.ID] = true
		if _, exists := m.data[c.ID]; !exists {
			m.data[c.ID] = &ContainerData{}
		}

		// Check if monitored
		var targetURL string
		var isMonitored bool

		var logKeywords []string

		for _, cfgContainer := range m.cfg.Containers {
			// Check against all names of the container
			for _, name := range c.Names {
				// Docker names often start with /
				// Config might be "/foo" or "foo"
				// Normalize comparison
				cleanName := strings.TrimPrefix(name, "/")
				cleanCfgName := strings.TrimPrefix(cfgContainer.ContainerName, "/")

				// Flexible matching: check if config name is part of docker name
				// e.g. "status-manager-service" matches "agent-service-monitoring-status-manager-service-1"
				if cleanName == cleanCfgName || strings.Contains(cleanName, cleanCfgName) {
					targetURL = cfgContainer.HealthCheckURL
					logKeywords = cfgContainer.LogKeywords
					isMonitored = true
					break
				}
			}
			if isMonitored {
				break
			}
		}

		// Prepare for concurrent HTTP checks
		var httpStatusPtr *int
		var latencyPtr *int64
		// 4 = HTTP_TIMEOUT
		// 5 = HTTP_CONN_ERR
		// 6 = LOG_ERROR (Specific Type)
		var checkResultStatus int
		foundLogKeyword := ""
		capturedErrMsg := ""
		// 0 = Not Checked / Docker Only
		// 1 = OK (200)
		// 2 = HTTP_CLIENT_ERR (4xx)
		// 3 = HTTP_SERVER_ERR (5xx)
		// 4 = HTTP_TIMEOUT
		// 5 = HTTP_CONN_ERR
		checkResultStatus = 0

		if isMonitored && targetURL != "" && c.State == "running" {
			// Launch HTTP check in background (non-blocking)
			go func(url string, containerID string) {
				code, lat, err := checkHTTP(url, m.cfg.HTTPTimeoutMs)
				errMsg := ""
				if err != nil {
					slog.Error("HTTP check failed", "url", url, "error", err.Error())
					// Simplify error message for the UI/Telegram
					if os.IsTimeout(err) {
						errMsg = "HTTP Request Timeout"
					} else if strings.Contains(err.Error(), "connection refused") {
						errMsg = "Connection Refused"
					} else if strings.Contains(err.Error(), "no such host") {
						errMsg = "DNS Lookup Failed (No such host)"
					} else {
						errMsg = "HTTP check failed"
					}
				}

				m.mu.Lock()
				if entry, exists := m.data[containerID]; exists {
					entry.HTTPStatus = &code
					entry.HTTPLatency = &lat

					// Determine specific HTTP status for this check
					statusStr := "OK"
					if err != nil {
						if os.IsTimeout(err) {
							statusStr = "HTTP_TIMEOUT"
						} else if isConnectionRefused(err) {
							statusStr = "HTTP_CONN_ERR"
						} else {
							// Generic error treated as connection/network error
							statusStr = "HTTP_CONN_ERR"
						}
					} else {
						if code >= 500 {
							statusStr = "HTTP_SERVER_ERR"
						} else if code >= 400 {
							statusStr = "HTTP_CLIENT_ERR"
						}
					}

					// Update MainStatus ONLY if Docker state is otherwise healthy
					// (If container is stopped, status should remain STOPPED)
					if entry.Metadata.State == "running" && entry.Metadata.HealthStatus != "unhealthy" {
						if statusStr != "OK" {
							entry.MainStatus = statusStr
						} else if isHTTPError(entry.MainStatus) {
							// Connection recovered, reset status to OK
							entry.MainStatus = "OK"
						}
					}

					// Always update Error Msg if there is one, or clear if OK
					if errMsg != "" {
						entry.LastErrorMsg = errMsg
					} else if statusStr == "OK" {
						// Only clear if we are now OK (avoid clearing log error keywords here)
						if isHTTPError(entry.MainStatus) || entry.MainStatus == "OK" {
							entry.LastErrorMsg = ""
						}
					}
				}
				m.mu.Unlock()
			}(targetURL, c.ID)

			checkResultStatus = 1 // Assume OK initially while async check runs (prevents flickering)
		} else if isMonitored {
			// Monitored but no URL specified means "Docker check only"
			checkResultStatus = 1
		}

		// LOG CHECK Logic (Sync)
		if isMonitored && len(logKeywords) > 0 && c.State == "running" {
			// Check logs since last poll
			since := m.LastPollTime
			if since.IsZero() {
				since = time.Now().Add(-1 * time.Minute) // Default lookback
			}

			logs, err := m.dockerClient.GetLogs(ctx, c.ID, since)
			if err == nil && logs != "" {
				for _, kw := range logKeywords {
					if strings.Contains(logs, kw) {
						// Found a keyword!
						slog.Warn("Found Error Keyword in Logs", "container", c.Names, "keyword", kw)
						// We can set a special status or just Trigger an Alert
						// For now, let's trigger an Alert but keep MainStatus as is (unless it's OK)

						// NOTE: Log errors are events. We trigger a "LOG_ERR" status transiently.
						// Or maybe we just fire the alert directly here?
						// Let's force MainStatus to LOG_ERR if it was OK
						checkResultStatus = 6                 // LOG_ERR
						foundLogKeyword = strings.ToUpper(kw) // Capture the keyword (e.g. FATAL, PANIC)

						// Extract a line snippet for context
						lines := strings.Split(logs, "\n")
						for _, line := range lines {
							if strings.Contains(line, kw) {
								if len(line) > 500 {
									capturedErrMsg = line[:500] + "..."
								} else {
									capturedErrMsg = line
								}
								capturedErrMsg = stripANSI(capturedErrMsg)
								break
							}
						}
						break
					}
				}
			}
		}

		// Determine Main Status based on priorities:
		// Priority 1: Docker State (STOPPED etc)
		// Priority 2: Log Errors (FATAL/ERROR keywords)
		// Priority 3: HTTP Health (UNHEALTHY or OK vs Error)
		mainStatus := "OK"
		if c.State != "running" {
			mainStatus = "STOPPED"
		} else if checkResultStatus == 6 {
			// Use the found keyword to make the status specific
			// e.g. LOG_FATAL, LOG_PANIC
			if foundLogKeyword != "" {
				mainStatus = "LOG_" + foundLogKeyword
			} else {
				mainStatus = "LOG_ERR"
			}
		} else if c.HealthStatus == "unhealthy" {
			mainStatus = "UNHEALTHY"
		} else if isMonitored && checkResultStatus == 0 {
			// This case is transient or initial state
			mainStatus = "OK"
		}

		// Note: HTTP failures are updated asynchronously in the goroutine above
		// We set simple status here, but if MainStatus was ALREADY set to an error by the async routine
		// from a previous poll, we want to preserve it UNLESS Docker status overrides it (e.g. stopped)

		// If current calc is OK, check if we should keep existing HTTP Error or reset?
		// Actually, simpler to let the previous value stick until the async update overwrites it,
		// BUT we must enforce STOPPED/UNHEALTHY/LOG_ERR immediately.

		if mainStatus == "OK" {
			// If Docker says OK, retain existing status if it's an HTTP error type,
			// otherwise set to OK. This prevents resetting to OK every poll before async check finishes.
			// Same for LOG errors, although they should ideally resolve themselves over time or manual clear.
			currentStatus := m.data[c.ID].MainStatus
			if isHTTPError(currentStatus) || strings.HasPrefix(currentStatus, "LOG_") {
				mainStatus = currentStatus
			}
		}

		// Real-time Alert Logic with Cooldown
		// ONLY for monitored containers
		if isMonitored {
			alertKey := namesToString(c.Names)
			const alertCooldown = 1 * time.Second

			if mainStatus != "OK" {
				firstSeen, pending := m.pendingAlerts[alertKey]

				// Reset cooldown timer if the status changed from a different error state
				// or if it's a newly discovered log entry error (which are discrete events)
				isNewLogEvent := strings.HasPrefix(mainStatus, "LOG_") && m.data[c.ID].MainStatus != mainStatus

				if pending && (m.data[c.ID].MainStatus != mainStatus || isNewLogEvent) {
					m.pendingAlerts[alertKey] = time.Now()
					firstSeen = m.pendingAlerts[alertKey]
				} else if !pending {
					m.pendingAlerts[alertKey] = time.Now()
					firstSeen = m.pendingAlerts[alertKey]
				}

				if time.Since(firstSeen) >= alertCooldown {
					msg := fmt.Sprintf("Container %s is %s", alertKey, mainStatus)

					// Only trigger if not already active to prevent spam
					// OR if the active alert message has fundamentally changed (e.g. from HTTP error to STOPPED)
					// OR if 1 minute has passed since the last notification (Nagging Alert)
					activeAlert, isActive := m.alerts.GetActive(alertKey)
					const nagInterval = 30 * time.Second
					isNagTick := isActive && time.Since(m.lastNotifyTimes[alertKey]) >= nagInterval

					if !isActive || activeAlert.Message != msg || isNagTick {
						m.alerts.SetActive(alerts.LevelError, alertKey, msg)
						slog.Error("Alert Triggered", "container", alertKey, "status", mainStatus, "is_nag", isNagTick)

						if m.mqttClient != nil {
							if err := m.mqttClient.PublishAlert(m.cfg.DeviceID, m.cfg.TypeID, false, msg, "high"); err != nil {
								slog.Error("Failed to publish MQTT alert", "error", err)
							}
						}

						if m.telegramClient != nil {
							var tgMsg string
							nowStr := time.Now().Format("15:04:05")
							prefix := fmt.Sprintf("🚨 <b>Container Monitor Alert</b>\n🕒 <b>Time:</b> %s", nowStr)

							if m.data[c.ID].LastErrorMsg != "" {
								// html escape safe format
								safeErr := strings.ReplaceAll(m.data[c.ID].LastErrorMsg, "<", "&lt;")
								safeErr = strings.ReplaceAll(safeErr, ">", "&gt;")
								tgMsg = fmt.Sprintf("<b>Container:</b> %s\n🔴 <b>Severity:</b> High\n🚦 <b>Status:</b> %s\n\n<b>Context:</b> <code>%s</code>", alertKey, mainStatus, safeErr)
							} else {
								tgMsg = fmt.Sprintf("<b>Container:</b> %s\n🔴 <b>Severity:</b> High\n🚦 <b>Status:</b> %s", alertKey, mainStatus)
							}

							err := m.telegramClient.SendAlert(prefix, tgMsg)
							// Update last notify time regardless of success to avoid spamming retries every poll.
							// This ensures we respect the nagInterval (1m) even if Telegram API is slow or failing.
							m.lastNotifyTimes[alertKey] = time.Now()

							if err != nil {
								slog.Error("Failed to send Telegram alert", "error", err)
							}
						}
					}
				}
			} else {
				// If OK, clear any existing alert, pending cooldown, and nag timer
				if m.alerts.IsActive(alertKey) {
					msg := fmt.Sprintf("Container %s recovered and is now OK", alertKey)
					slog.Info("Alert Resolved", "container", alertKey, "status", "OK")

					if m.mqttClient != nil {
						if err := m.mqttClient.PublishAlert(m.cfg.DeviceID, m.cfg.TypeID, true, msg, "info"); err != nil {
							slog.Error("Failed to publish MQTT resolve state", "error", err)
						}
					}

					if m.telegramClient != nil {
						nowStr := time.Now().Format("15:04:05")
						tgMsg := fmt.Sprintf("🚦 <b>Status:</b> Resolved (OK)\n<b>Container:</b> %s\n<b>Event:</b> Recovered back to normal.", alertKey)
						if err := m.telegramClient.SendAlert(fmt.Sprintf("✅ <b>Container Monitor Recovery</b>\n🕒 <b>Time:</b> %s", nowStr), tgMsg); err != nil {
							slog.Error("Failed to send Telegram resolve alert", "error", err)
						}
					}
				}
				m.alerts.Resolve(alertKey)
				delete(m.pendingAlerts, alertKey)
				delete(m.lastNotifyTimes, alertKey)
			}
		}

		m.data[c.ID].Metadata = c
		m.data[c.ID].LastUpdated = time.Now()
		m.data[c.ID].IsMonitored = isMonitored
		m.data[c.ID].MainStatus = mainStatus
		m.data[c.ID].HTTPStatus = httpStatusPtr
		m.data[c.ID].HTTPLatency = latencyPtr

		if capturedErrMsg != "" {
			m.data[c.ID].LastErrorMsg = capturedErrMsg
		} else if mainStatus == "OK" {
			m.data[c.ID].LastErrorMsg = ""
		}
	}

	// Optional: Remove stale
	for id := range m.data {
		if !currentIDs[id] {
			delete(m.data, id)
		}
	}

	m.LastPollTime = time.Now()
}

func (m *Monitor) collectStats(ctx context.Context) {
	m.mu.RLock()
	ids := make([]string, 0, len(m.data))
	for id := range m.data {
		// Only collect stats for running containers
		if m.data[id].Metadata.State == "running" {
			ids = append(ids, id)
		}
	}
	m.mu.RUnlock()

	// Parallel stats collection using goroutines
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()

			stats, err := m.dockerClient.GetStats(ctx, containerID)
			if err != nil {
				slog.Error("Error getting stats", "container_id", containerID, "error", err)
				return
			}

			m.mu.Lock()
			if entry, exists := m.data[containerID]; exists {
				entry.Stats = stats
			}
			m.mu.Unlock()
		}(id)
	}
	wg.Wait()
}

func (m *Monitor) GetData() map[string]ContainerData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copyData := make(map[string]ContainerData)
	for k, v := range m.data {
		copyData[k] = *v
	}
	return copyData
}

func (m *Monitor) GetAlerts() []alerts.Alert {
	return m.alerts.GetAlerts()
}

func checkHTTP(url string, timeoutMs int) (int, int64, error) {
	if timeoutMs <= 0 {
		timeoutMs = 2000 // Default to 2s if missing
	}
	client := http.Client{
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
	}
	start := time.Now()
	resp, err := client.Get(url)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return 0, 0, err // Return error for analysis
	}
	defer resp.Body.Close()
	return resp.StatusCode, duration, nil
}

func namesToString(names []string) string {
	if len(names) > 0 {
		// Example: /agent-service-monitoring-status-manager-service-1
		name := names[0]
		// Remove leading slash
		name = strings.TrimPrefix(name, "/")
		// Remove project prefix if present (adjust based on your project folder name)
		name = strings.TrimPrefix(name, "agent-service-monitoring-")
		// Remove suffix like "-1" (common in docker compose)
		// Simpler approach: if it ends with -digits, trim it?
		// Or just hardcode for this project structure:
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			// Check if chars after last dash are digits
			suffix := name[idx+1:]
			isDigit := true
			for _, r := range suffix {
				if r < '0' || r > '9' {
					isDigit = false
					break
				}
			}
			if isDigit {
				name = name[:idx]
			}
		}
		return name
	}
	return ""
}

// Helper to check for connection refused
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	// Basic string check is often most reliable across OSes for "connection refused"
	return strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host")
}

func isHTTPError(status string) bool {
	return strings.HasPrefix(status, "HTTP_")
}

var ansiRegex = regexp.MustCompile("[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}
