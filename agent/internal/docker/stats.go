package docker

import (
	"context"

	"dockpit/agent/protocol"
)

// Info returns a summary of the Docker Engine.
func (c *Client) Info(ctx context.Context) (protocol.DockerInfo, error) {
	var v struct {
		ServerVersion     string `json:"ServerVersion"`
		OperatingSystem   string `json:"OperatingSystem"`
		Containers        int    `json:"Containers"`
		ContainersRunning int    `json:"ContainersRunning"`
		ContainersPaused  int    `json:"ContainersPaused"`
		ContainersStopped int    `json:"ContainersStopped"`
		Images            int    `json:"Images"`
		NCPU              int    `json:"NCPU"`
		MemTotal          int64  `json:"MemTotal"`
	}
	if err := c.get(ctx, "/info", &v); err != nil {
		return protocol.DockerInfo{}, err
	}
	return protocol.DockerInfo{
		Version:         v.ServerVersion,
		OperatingSystem: v.OperatingSystem,
		Containers:      v.Containers,
		Running:         v.ContainersRunning,
		Paused:          v.ContainersPaused,
		Stopped:         v.ContainersStopped,
		Images:          v.Images,
		NCPU:            v.NCPU,
		MemTotal:        v.MemTotal,
	}, nil
}

// Stats is one resource usage sample of a container.
type Stats struct {
	CPUTotal   uint64 // container CPU time, ns
	SystemCPU  uint64 // host CPU time, ns
	OnlineCPUs int
	MemUsage   uint64 // excluding page cache, like `docker stats`
	MemLimit   uint64
}

// ContainerStats takes a single stats sample. one-shot=true skips Docker's
// built-in one-second wait; CPU percent is computed from two samples by the
// caller instead.
func (c *Client) ContainerStats(ctx context.Context, id string) (Stats, error) {
	var v struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     int    `json:"online_cpus"`
		} `json:"cpu_stats"`
		MemoryStats struct {
			Usage uint64            `json:"usage"`
			Limit uint64            `json:"limit"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
	}
	if err := c.get(ctx, containerPath(id, "/stats")+"?stream=false&one-shot=true", &v); err != nil {
		return Stats{}, err
	}
	mem := v.MemoryStats.Usage
	// cgroup v2 reports inactive_file, v1 total_inactive_file.
	cache, ok := v.MemoryStats.Stats["inactive_file"]
	if !ok {
		cache = v.MemoryStats.Stats["total_inactive_file"]
	}
	if cache < mem {
		mem -= cache
	}
	return Stats{
		CPUTotal:   v.CPUStats.CPUUsage.TotalUsage,
		SystemCPU:  v.CPUStats.SystemCPUUsage,
		OnlineCPUs: v.CPUStats.OnlineCPUs,
		MemUsage:   mem,
		MemLimit:   v.MemoryStats.Limit,
	}, nil
}

// CPUPercent computes CPU usage between two samples the way `docker stats`
// does: 100% means one full core.
func CPUPercent(prev, cur Stats) (float64, bool) {
	if cur.CPUTotal < prev.CPUTotal || cur.SystemCPU <= prev.SystemCPU {
		return 0, false
	}
	cpus := cur.OnlineCPUs
	if cpus == 0 {
		cpus = 1
	}
	return float64(cur.CPUTotal-prev.CPUTotal) / float64(cur.SystemCPU-prev.SystemCPU) * float64(cpus) * 100, true
}
