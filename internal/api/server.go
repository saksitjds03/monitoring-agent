package api

import (
	"encoding/json"
	"net/http"
	"time"

	"agent-service-monitoring/internal/monitor"
)

type Server struct {
	mon *monitor.Monitor
}

func NewServer(mon *monitor.Monitor) *Server {
	return &Server{mon: mon}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/containers", s.handleContainers) // Changed to /v1/containers to match PRD
	mux.HandleFunc("/v1/alerts", s.handleAlerts)
	// Alias old endpoint for backward compatibility during dev if needed?
	mux.HandleFunc("/api/containers", s.handleContainers)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp := APIHealthResponse{
		OK:         true,
		Version:    "1.0.0",
		LastPollTS: s.mon.LastPollTime.Unix(),
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	data := s.mon.GetData()

	var items []APIItem
	for _, v := range data {
		// Map monitor.ContainerData to api.APIItem

		var httpHealth *HTTPHealthInfo
		if v.IsMonitored {
			isOK := false
			if v.HTTPStatus != nil && *v.HTTPStatus == 200 {
				isOK = true
			}
			httpHealth = &HTTPHealthInfo{
				OK:         isOK,
				StatusCode: v.HTTPStatus,
				LatencyMs:  v.HTTPLatency,
				CheckedAt:  v.LastUpdated.Unix(),
				LastError:  v.LastErrorMsg,
			}
		}

		resources := &ResourceInfo{
			CPUPercent:    v.Stats.CPUPercent,
			MemUsageBytes: v.Stats.MemoryUsage,
			MemLimitBytes: v.Stats.MemoryLimit,
			MemPercent:    v.Stats.MemoryPercent,
			NetRxBytes:    v.Stats.NetRx,
			NetTxBytes:    v.Stats.NetTx,
			BlkReadBytes:  v.Stats.BlockRead,
			BlkWriteBytes: v.Stats.BlockWrite,
		}

		// Extract Compose Labels
		composeProject := v.Metadata.Labels["com.docker.compose.project"]
		composeService := v.Metadata.Labels["com.docker.compose.service"]

		items = append(items, APIItem{
			ContainerName: v.Metadata.Names[0], // simplified
			ID:            v.Metadata.ID,
			Status:        v.Metadata.State, // running, exited...
			UptimeSeconds: v.Metadata.Uptime.Seconds(),
			RestartCount:  v.Metadata.RestartCount,
			DockerHealth:  v.Metadata.HealthStatus,
			HTTPHealth:    httpHealth,
			Resources:     resources,
			Meta: &MetaInfo{
				Image:          v.Metadata.Image,
				ComposeProject: composeProject,
				ComposeService: composeService,
			},
		})
	}

	resp := APIResponse{
		GeneratedAt: time.Now().Unix(),
		Items:       items,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.mon.GetAlerts()
	response := map[string]interface{}{
		"items": alerts,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
