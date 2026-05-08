package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"

	"golang.nuinfra.net/daemon/pkg/config"
)

type defaultMetricsCollector struct {
	config *config.Bundle
}

var _ MetricsCollector = (*defaultMetricsCollector)(nil)

// Collect implements [MetricsCollector].
func (d *defaultMetricsCollector) Collect(ctx context.Context) (*Metrics, error) {
	metrics := &Metrics{}

	logicalCPUCount, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to count logical cpus: %w", err)
	}
	metrics.CPUCapacityMillicores = uint32(logicalCPUCount) * 1000

	cpuPercent, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("failed to collect cpu usage samples: %w", err)
	}
	if len(cpuPercent) == 0 {
		return nil, errors.New("no cpu usage samples are available")
	}
	metrics.CPUUsedMillicores = uint32((cpuPercent[0] / 100.0) * float64(metrics.CPUCapacityMillicores))

	virtualMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect virtual memory information: %w", err)
	}
	metrics.MemoryCapacityBytes = virtualMem.Total
	metrics.MemoryUsedBytes = virtualMem.Used

	diskPath := "/"
	if d.config != nil && d.config.Firecracker != nil && len(d.config.Firecracker.ChrootBaseDir) != 0 {
		diskPath = d.config.Firecracker.ChrootBaseDir
	}
	du, err := disk.UsageWithContext(ctx, diskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to collect disk usage information: %w", err)
	}
	metrics.DiskCapacityBytes = du.Total
	metrics.DiskUsedBytes = du.Used

	return metrics, nil
}

// NewMetricsCollector returns the default MetricsCollector, backed by
// gopsutil. Disk usage is measured against config.Firecracker.ChrootBaseDir
// when set, falling back to "/" otherwise.
func NewMetricsCollector(config *config.Bundle) MetricsCollector {
	return &defaultMetricsCollector{config: config}
}
