package node

import (
	"context"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// MetricsCollector samples the host's CPU, memory, and disk capacity and
// usage. The default implementation reads the values via gopsutil; tests use
// fakes injected through NewAgent.
type MetricsCollector interface {
	Collect(ctx context.Context) (*Metrics, error)
}

// Metrics is a single snapshot of node-local resource capacity and current
// usage. The Agent maps it onto the NodeResources / NodeMetrics protos before
// reporting.
type Metrics struct {
	CPUCapacityMillicores uint32
	CPUUsedMillicores     uint32
	MemoryCapacityBytes   uint64
	MemoryUsedBytes       uint64
	DiskCapacityBytes     uint64
	DiskUsedBytes         uint64
}

// Reporter publishes node updates back to the api-server. Production
// implementations are typically backed by the controller's bidirectional
// EstablishSession stream; tests use generated mocks.
type Reporter interface {
	// ReportNodeMetrics sends a fresh NodeMetrics sample. Called by the
	// Agent on every metrics tick when the sample differs from the
	// previous one.
	ReportNodeMetrics(ctx context.Context, nodeMetrics *controlplanev1alpha1.NodeMetrics) error

	// ReportStatusPhase sends a NodeStatus phase transition and its
	// human-readable message. Called by the Agent only when the phase
	// changes from the previous tick.
	ReportStatusPhase(ctx context.Context, phase controlplanev1alpha1.NodeStatus_Phase, message string) error
}
