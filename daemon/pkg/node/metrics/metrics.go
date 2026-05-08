// Package metrics samples the host's CPU, memory, and disk capacity and
// usage. It is consumed by the node Agent on every reporting tick.
package metrics

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

// Sample is a single snapshot of node-local resource capacity and current
// usage. The Agent maps it onto the NodeResources / NodeMetrics protos before
// reporting.
type Sample struct {
	CPUCapacityMillicores uint32
	CPUUsedMillicores     uint32
	MemoryCapacityBytes   uint64
	MemoryUsedBytes       uint64
	DiskCapacityBytes     uint64
	DiskUsedBytes         uint64
}

// Collector samples the host's CPU, memory, and disk metrics. The default
// implementation reads via gopsutil; tests inject fakes through node.NewAgent.
type Collector interface {
	Collect(ctx context.Context) (*Sample, error)
}

type defaultCollector struct {
	config *config.Bundle
}

var _ Collector = (*defaultCollector)(nil)

// Collect implements [Collector].
func (d *defaultCollector) Collect(ctx context.Context) (*Sample, error) {
	sample := &Sample{}

	logicalCPUCount, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to count logical cpus: %w", err)
	}
	sample.CPUCapacityMillicores = uint32(logicalCPUCount) * 1000

	cpuPercent, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("failed to collect cpu usage samples: %w", err)
	}
	if len(cpuPercent) == 0 {
		return nil, errors.New("no cpu usage samples are available")
	}
	sample.CPUUsedMillicores = uint32((cpuPercent[0] / 100.0) * float64(sample.CPUCapacityMillicores))

	virtualMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect virtual memory information: %w", err)
	}
	sample.MemoryCapacityBytes = virtualMem.Total
	sample.MemoryUsedBytes = virtualMem.Used

	diskPath := "/"
	if d.config != nil && d.config.Firecracker != nil && len(d.config.Firecracker.ChrootBaseDir) != 0 {
		diskPath = d.config.Firecracker.ChrootBaseDir
	}
	du, err := disk.UsageWithContext(ctx, diskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to collect disk usage information: %w", err)
	}
	sample.DiskCapacityBytes = du.Total
	sample.DiskUsedBytes = du.Used

	return sample, nil
}

// NewCollector returns the default Collector, backed by gopsutil. Disk usage
// is measured against config.Firecracker.ChrootBaseDir when set, falling back
// to "/" otherwise.
func NewCollector(config *config.Bundle) Collector {
	return &defaultCollector{config: config}
}
