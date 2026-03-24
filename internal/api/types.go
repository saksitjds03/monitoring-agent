package api

// APIResponse is the top-level response for /v1/containers
type APIResponse struct {
	GeneratedAt int64     `json:"generated_at"`
	Items       []APIItem `json:"items"`
}

// APIHealthResponse for /health
type APIHealthResponse struct {
	OK         bool   `json:"ok"`
	Version    string `json:"version"`
	LastPollTS int64  `json:"last_poll_ts"`
}

type APIItem struct {
	ContainerName string          `json:"container_name"`
	ID            string          `json:"id"`
	Status        string          `json:"status"`
	UptimeSeconds float64         `json:"uptime_seconds"`
	RestartCount  int             `json:"restart_count"`
	DockerHealth  string          `json:"docker_health"`
	HTTPHealth    *HTTPHealthInfo `json:"http_health"` // Nullable
	Resources     *ResourceInfo   `json:"resources"`
	Meta          *MetaInfo       `json:"meta"`
}

type HTTPHealthInfo struct {
	OK         bool   `json:"ok"`
	StatusCode *int   `json:"status_code"`
	LatencyMs  *int64 `json:"latency_ms"`
	CheckedAt  int64  `json:"checked_at"`
	LastError  string `json:"last_error,omitempty"`
}

type ResourceInfo struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemUsageBytes uint64  `json:"mem_usage_bytes"`
	MemLimitBytes uint64  `json:"mem_limit_bytes"`
	MemPercent    float64 `json:"mem_percent"`
	NetRxBytes    uint64  `json:"net_rx_bytes"`
	NetTxBytes    uint64  `json:"net_tx_bytes"`
	BlkReadBytes  uint64  `json:"blk_read_bytes"`
	BlkWriteBytes uint64  `json:"blk_write_bytes"`
}

type MetaInfo struct {
	Image          string `json:"image"`
	ComposeProject string `json:"compose_project,omitempty"`
	ComposeService string `json:"compose_service,omitempty"`
}
