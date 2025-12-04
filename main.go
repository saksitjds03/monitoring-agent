package main

import (
	"fmt"
	"time"
)

type Config struct {
	TargetIPs      []string
	TargetURLs     []string
	TargetKeywords []string
}
type MonitorResult struct {
	Target    string
	Status    bool
	Message   string
	Timestamp time.Time
}

func main() {
	fmt.Println("--- Monitoring Agent ---")
	config := Config{
		TargetIPs:      []string{"8.8.8.8", "google.com"},
		TargetURLs:     []string{"http://localhost:3003", "http://localhost:3001", "http://localhost:3002"},
		TargetKeywords: []string{"alert-notification-service", "status-manager-service", "health-monitoring-service"},
	}
	runChecks(config)
	fmt.Println("\nPress Enter to exit.")
	fmt.Scanln()
}
func runChecks(cfg Config) {
	fmt.Println("\n[1. Network Check]")
	for _, ip := range cfg.TargetIPs {
		printResult(CheckPing(ip))
	}
	fmt.Println("\n[2. HTTP Service Check]")
	for _, url := range cfg.TargetURLs {
		printResult(CheckHTTP(url))
	}
	fmt.Println("\n[3. Process Check]")
	for _, kw := range cfg.TargetKeywords {
		printResult(CheckProcess(kw))
	}
}
func printResult(res MonitorResult) {
	statusIcon := "❌ DOWN"
	if res.Status {
		statusIcon = "✅ UP"
	}
	fmt.Printf("%-35s | %s | %s\n", res.Target, statusIcon, res.Message)
}
