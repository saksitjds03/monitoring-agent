package alerts

import (
	"sync"
	"time"
)

type AlertLevel string

const (
	LevelInfo    AlertLevel = "info"
	LevelWarning AlertLevel = "warning"
	LevelError   AlertLevel = "error"
)

type Alert struct {
	Timestamp int64      `json:"ts"`
	Level     AlertLevel `json:"level"`
	Key       string     `json:"key"`
	Message   string     `json:"message"`
}

type AlertManager struct {
	buffer []Alert
	active map[string]Alert // Key -> Active Alert
	size   int
	head   int
	mu     sync.RWMutex
}

func NewAlertManager(size int) *AlertManager {
	return &AlertManager{
		buffer: make([]Alert, 0, size),
		active: make(map[string]Alert),
		size:   size,
	}
}

// SetActive adds or updates an active alert
func (am *AlertManager) SetActive(level AlertLevel, key, message string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert := Alert{
		Timestamp: time.Now().Unix(),
		Level:     level,
		Key:       key,
		Message:   message,
	}

	// Update Active Map
	am.active[key] = alert

	// Also add to history buffer
	am.addToBuffer(alert)
}

// Resolve removes an active alert
func (am *AlertManager) Resolve(key string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.active, key)
}

// GetActive retrieves an active alert if it exists
func (am *AlertManager) GetActive(key string) (Alert, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	alert, exists := am.active[key]
	return alert, exists
}

// IsActive checks if an alert key is currently active
func (am *AlertManager) IsActive(key string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	_, exists := am.active[key]
	return exists
}

// Add adds to history only (legacy support)
func (am *AlertManager) Add(level AlertLevel, key, message string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert := Alert{
		Timestamp: time.Now().Unix(),
		Level:     level,
		Key:       key,
		Message:   message,
	}
	am.addToBuffer(alert)
}

func (am *AlertManager) addToBuffer(alert Alert) {
	if len(am.buffer) < am.size {
		am.buffer = append(am.buffer, alert)
	} else {
		am.buffer = append(am.buffer[1:], alert)
	}
}

func (am *AlertManager) GetAlerts() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Return Active Alerts only (As requested for Real-time view)
	// If history is needed, we can make another method or return struct
	result := make([]Alert, 0, len(am.active))
	for _, v := range am.active {
		result = append(result, v)
	}
	return result
}
