package node

import (
	"context"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

type MetricsCollector interface {
	Collect(ctx context.Context) (*Metrics, error)
}

type Metrics struct {
	CPUCapacityMillicores uint32
	CPUUsedMillicores     uint32
	MemoryCapacityBytes   uint64
	MemoryUsedBytes       uint64
	DiskCapacityBytes     uint64
	DiskUsedBytes         uint64
}

type Reporter interface {
	ReportNodeMetrics(ctx context.Context, nodeMetrics *controlplanev1alpha1.NodeMetrics) error

	ReportStatusPhase(ctx context.Context, phase controlplanev1alpha1.NodeStatus_Phase, message string) error
}
