package node

import (
	"context"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
)

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
