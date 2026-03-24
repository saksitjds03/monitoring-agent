package docker

import "time"

type ContainerMetadata struct {
	ID           string            `json:"id"`
	Names        []string          `json:"names"`
	Image        string            `json:"image"`
	State        string            `json:"state"`  // e.g., running
	Status       string            `json:"status"` // e.g., Up 10 hours
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   time.Time         `json:"finished_at"`
	RestartCount int               `json:"restart_count"`
	HealthStatus string            `json:"health_status"` // healthy, unhealthy, or empty
	Uptime       time.Duration     `json:"uptime"`
	Labels       map[string]string `json:"labels"`
}

type ContainerStats struct {
	ID            string  `json:"id"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetRx         uint64  `json:"net_rx"`
	NetTx         uint64  `json:"net_tx"`
	BlockRead     uint64  `json:"block_read"`
	BlockWrite    uint64  `json:"block_write"`
}

// DockerStatsJSON is a local subset of fields we need from the Docker Stats API
type DockerStatsJSON struct {
	PreCPUStats CPUStats                `json:"precpu_stats"`
	CPUStats    CPUStats                `json:"cpu_stats"`
	MemoryStats MemoryStats             `json:"memory_stats"`
	Networks    map[string]NetworkStats `json:"networks"`
	BlkioStats  BlkioStats              `json:"blkio_stats"`
}

type CPUStats struct {
	CPUUsage    CPUUsage `json:"cpu_usage"`
	SystemUsage uint64   `json:"system_cpu_usage"`
	OnlineCPUs  uint32   `json:"online_cpus"`
}

type CPUUsage struct {
	TotalUsage  uint64   `json:"total_usage"`
	PercpuUsage []uint64 `json:"percpu_usage"`
}

type MemoryStats struct {
	Usage uint64            `json:"usage"`
	Limit uint64            `json:"limit"`
	Stats map[string]uint64 `json:"stats"`
}

type NetworkStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type BlkioStats struct {
	IoServiceBytesRecursive []BlkioStatEntry `json:"io_service_bytes_recursive"`
}

type BlkioStatEntry struct {
	Major uint64 `json:"major"`
	Minor uint64 `json:"minor"`
	Op    string `json:"op"`
	Value uint64 `json:"value"`
}
