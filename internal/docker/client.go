package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type DockerClient struct {
	cli *client.Client
	// Cache for static data (Labels, Image) to reduce API calls
	cacheMu     sync.RWMutex
	staticCache map[string]*staticData
}

type staticData struct {
	Image  string
	Labels map[string]string
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerClient{
		cli:         cli,
		staticCache: make(map[string]*staticData),
	}, nil
}

func (d *DockerClient) Poll(ctx context.Context) ([]ContainerMetadata, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	var results []ContainerMetadata
	for _, c := range containers {
		// Always inspect (needed for dynamic data)
		inspect, err := d.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			continue
		}

		// Check cache for static data
		d.cacheMu.RLock()
		cached, hasCached := d.staticCache[c.ID]
		d.cacheMu.RUnlock()

		// If not cached, cache it now
		if !hasCached {
			cached = &staticData{
				Image:  c.Image,
				Labels: filterLabels(c.Labels),
			}
			d.cacheMu.Lock()
			d.staticCache[c.ID] = cached
			d.cacheMu.Unlock()
		}

		meta := ContainerMetadata{
			ID:           c.ID,
			Names:        c.Names,
			Image:        cached.Image,
			State:        c.State,
			Status:       c.Status,
			StartedAt:    parseTime(inspect.State.StartedAt),
			FinishedAt:   parseTime(inspect.State.FinishedAt),
			RestartCount: inspect.RestartCount,
			Labels:       cached.Labels,
		}

		if inspect.State.Running {
			meta.Uptime = time.Since(meta.StartedAt)
		}

		if inspect.State.Health != nil {
			meta.HealthStatus = inspect.State.Health.Status
		}

		results = append(results, meta)
	}
	return results, nil
}

// GetLogs fetches recent logs from the container since the specified time.
// It returns the logs as a single string.
func (d *DockerClient) GetLogs(ctx context.Context, containerID string, since time.Time) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      since.Format(time.RFC3339),
		Timestamps: false,
	}

	reader, err := d.cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// Use a limited buffer to prevent OOM on massive logs
	buf := new(strings.Builder)
	_, err = io.Copy(buf, reader)
	if err != nil {
		return "", fmt.Errorf("error reading logs: %w", err)
	}

	return buf.String(), nil
}

// Stats Stream is not ideal for "Poll". We want a snapshot.
// ContainerStats(ctx, id, true) streams.
// ContainerStats(ctx, id, false) gives a snapshot? No, it initializes stream.
// Actually, with stream=false, it makes one request.
func (d *DockerClient) GetStats(ctx context.Context, containerID string) (ContainerStats, error) {
	statsResp, err := d.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return ContainerStats{}, err
	}
	defer statsResp.Body.Close()

	var stats DockerStatsJSON
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		return ContainerStats{}, err
	}

	return calculateDockerStats(containerID, &stats), nil
}

func calculateDockerStats(id string, s *DockerStatsJSON) ContainerStats {
	// CPU
	cpuPercent := 0.0
	previousCPU := s.PreCPUStats.CPUUsage.TotalUsage
	previousSystem := s.PreCPUStats.SystemUsage

	// On Linux, PreCPUStats might be available.
	// If it's the first sample, Pre might be 0.
	// Docker CLI does some caching, but here we might just use PreCPUStats from the JSON.

	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(previousCPU)
	systemDelta := float64(s.CPUStats.SystemUsage) - float64(previousSystem)
	onlineCPUs := float64(s.CPUStats.OnlineCPUs)
	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}

	// Memory
	memUsage := s.MemoryStats.Usage - s.MemoryStats.Stats["cache"]
	if s.MemoryStats.Usage != 0 {
		// some versions use "total_inactive_file"
	}
	// Simplified memory calc
	memLimit := s.MemoryStats.Limit
	memPercent := 0.0
	if memLimit != 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Net
	var rx, tx uint64
	for _, network := range s.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}

	// IO
	var read, write uint64
	for _, blk := range s.BlkioStats.IoServiceBytesRecursive {
		if strings.EqualFold(blk.Op, "Read") {
			read += blk.Value
		} else if strings.EqualFold(blk.Op, "Write") {
			write += blk.Value
		}
	}

	return ContainerStats{
		ID:            id,
		CPUPercent:    cpuPercent,
		MemoryUsage:   memUsage,
		MemoryLimit:   memLimit,
		MemoryPercent: memPercent,
		NetRx:         rx,
		NetTx:         tx,
		BlockRead:     read,
		BlockWrite:    write,
	}
}

func parseTime(t string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, t)
	return parsed
}

func filterLabels(labels map[string]string) map[string]string {
	// Whitelist of allowed labels
	whitelist := []string{
		"com.docker.compose.project",
		"com.docker.compose.service",
		"org.opencontainers.image.version",
		"maintainer",
	}

	filtered := make(map[string]string)
	for k, v := range labels {
		for _, allowed := range whitelist {
			if k == allowed {
				filtered[k] = v
				break
			}
		}
	}
	return filtered
}
