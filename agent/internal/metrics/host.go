// Package metrics samples host and container resource usage in the
// background so requests are answered from memory.
package metrics

import (
	"context"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"dockpit/agent/protocol"
)

// HostCollector reads OS-level metrics. It hides the OS-specific parts.
type HostCollector interface {
	Collect(ctx context.Context) (protocol.HostMetrics, error)
}

// SystemCollector reads metrics of the local machine via gopsutil, which
// works on Linux and macOS without cgo.
type SystemCollector struct {
	DiskPath string
}

// Collect implements HostCollector. CPU usage is measured since the
// previous call, so call it at a steady interval.
func (c SystemCollector) Collect(ctx context.Context) (protocol.HostMetrics, error) {
	var m protocol.HostMetrics
	pct, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return m, err
	}
	if len(pct) > 0 {
		m.CPUPercent = pct[0]
	}
	if n, err := cpu.CountsWithContext(ctx, true); err == nil {
		m.CPUCores = n
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return m, err
	}
	m.MemUsed, m.MemTotal = vm.Used, vm.Total

	m.DiskPath = c.DiskPath
	if d, err := disk.UsageWithContext(ctx, c.DiskPath); err == nil {
		m.DiskUsed, m.DiskTotal = d.Used, d.Total
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		m.UptimeSeconds = up
	}
	return m, nil
}
