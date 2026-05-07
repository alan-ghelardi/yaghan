package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	ec2client "golang.nuinfra.net/commons/pkg/aws/ec2"
	ec2imdsclient "golang.nuinfra.net/commons/pkg/aws/ec2imds"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Agent struct {
	config           *config.Bundle
	ec2Client        ec2client.Client
	imdsClient       ec2imdsclient.Client
	metricsCollector MetricsCollector
	firecracker      firecracker.Provider
	reporter         Reporter
}

// NewAgent ...
func NewAgent(ctx context.Context, config *config.Bundle, firecracker firecracker.Provider, metricsCollector MetricsCollector, reporter Reporter) *Agent {
	return &Agent{
		config:           config,
		ec2Client:        ec2client.Get(ctx),
		imdsClient:       ec2imdsclient.Get(ctx),
		firecracker:      firecracker,
		metricsCollector: metricsCollector,
		reporter:         reporter,
	}
}

// Run ...
func (a *Agent) Run(ctx context.Context, nodeID string) {
	var wg sync.WaitGroup

	wg.Go(func() {
		a.metricsLoop(ctx)
	})

	if a.config.NodeAgent != nil && a.config.NodeAgent.Runtime == config.NodeRuntimeEC2 {
		wg.Go(func() {
			a.ec2HealthCheckLoop(ctx, nodeID)
		})
	}

	wg.Wait()
}

func (a *Agent) metricsLoop(ctx context.Context) {
	zapLogger := ctxzap.Extract(ctx)
	ticker := time.NewTicker(a.config.NodeAgent.MetricsReportInterval)
	defer ticker.Stop()

	oldMetrics := &controlplanev1alpha1.NodeMetrics{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, newMetrics, err := a.capacityAndMetrics(ctx)
			if err != nil {
				zapLogger.Error("unable to collect node metrics", zap.Error(err))
				continue
			}

			if proto.Equal(oldMetrics, newMetrics) {
				continue
			}

			oldMetrics = newMetrics
			if err := a.reporter.ReportNodeMetrics(ctx, newMetrics); err != nil {
				zapLogger.Error("unable to report node metrics", zap.Error(err))
			}
		}
	}
}

func (a *Agent) capacityAndMetrics(ctx context.Context) (*controlplanev1alpha1.NodeResources, *controlplanev1alpha1.NodeMetrics, error) {
	metrics, err := a.metricsCollector.Collect(ctx)
	if err != nil {
		return nil, nil, err
	}

	nodeCapacity := &controlplanev1alpha1.NodeResources{
		CpuCapacityMillicores: metrics.CPUCapacityMillicores,
		MemoryCapacityBytes:   metrics.MemoryCapacityBytes,
		DiskCapacityBytes:     metrics.DiskCapacityBytes,
	}

	nodeMetrics := &controlplanev1alpha1.NodeMetrics{
		SampledAt:          timestamppb.Now(),
		ActiveSandboxCount: uint32(a.firecracker.Len()),
		CpuUsedMillicores:  metrics.CPUUsedMillicores,
		MemoryUsedBytes:    metrics.MemoryUsedBytes,
		DiskUsedBytes:      metrics.DiskUsedBytes,
	}

	return nodeCapacity, nodeMetrics, nil
}

func (a *Agent) ec2HealthCheckLoop(ctx context.Context, instanceID string) {
	zapLogger := ctxzap.Extract(ctx)
	ticker := time.NewTicker(a.config.NodeAgent.HealthReportInterval)
	defer ticker.Stop()

	var oldPhase controlplanev1alpha1.NodeStatus_Phase
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newPhase, message := a.statusPhaseAndMessage(ctx, instanceID)
			if oldPhase == newPhase {
				continue
			}

			oldPhase = newPhase
			if err := a.reporter.ReportStatusPhase(ctx, newPhase, message); err != nil {
				zapLogger.Error("unable to report node status",
					zap.String("ec2.instance_id", instanceID),
					zap.Error(err))
				continue
			}
		}
	}
}

func (a *Agent) statusPhaseAndMessage(ctx context.Context, instanceID string) (controlplanev1alpha1.NodeStatus_Phase, string) {
	resp, err := a.ec2Client.DescribeInstanceStatus(ctx, &ec2.DescribeInstanceStatusInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN, fmt.Sprintf("failed to determine EC2 instance health status: %v", err)
	}

	failedChecks := 0
	var message strings.Builder
	for _, instanceStatus := range resp.InstanceStatuses {
		ebsStatus := instanceStatus.AttachedEbsStatus
		if ebsStatus != nil {
			switch ebsStatus.Status {
			case ec2types.SummaryStatusOk, ec2types.SummaryStatusInitializing, ec2types.SummaryStatusNotApplicable:
			default:
				failedChecks++
				for _, detail := range ebsStatus.Details {
					fmt.Fprintf(&message, "- %s: %s", detail.Name, detail.Status)
					if ts := detail.ImpairedSince; ts != nil {
						fmt.Fprintf(&message, " at %s", ts.Format(time.RFC3339))
					}
					fmt.Fprintln(&message)
				}
			}
		}
	}

	if failedChecks != 0 {
		return controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY, message.String()
	}
	return controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, "all checks have passed"
}
