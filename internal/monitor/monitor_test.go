package monitor

import (
	"agent-service-monitoring/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHTTP(t *testing.T) {
	// 1. Setup a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 2. Test successful check
	code, latency, err := checkHTTP(server.URL, 1000)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if code != 200 {
		t.Errorf("Expected status 200, got %d", code)
	}
	if latency < 0 {
		t.Errorf("Expected positive latency, got %d", latency)
	}

	// 3. Test failure check (invalid URL)
	code, _, err = checkHTTP("http://invalid-url-that-does-not-exist.local", 100)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
	if code != 0 {
		t.Errorf("Expected status 0 for connection error, got %d", code)
	}
}

func TestMonitorInit_AlertManager(t *testing.T) {
	cfg := &config.Config{
		PollIntervalMs: 100,
	}
	// Nil clients for unit test logic only (not integration)
	mon := NewMonitor(cfg, nil, nil, nil)

	if mon.alerts == nil {
		t.Error("Expected AlertManager to be initialized")
	}
	if mon.data == nil {
		t.Error("Expected data map to be initialized")
	}
}
