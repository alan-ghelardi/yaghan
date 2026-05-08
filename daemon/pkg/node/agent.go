package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	ec2client "golang.nuinfra.net/commons/pkg/aws/ec2"
	ec2imdsclient "golang.nuinfra.net/commons/pkg/aws/ec2imds"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/node/metrics"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Agent owns the daemon's view of node-local state. It builds the Node
// identity sent to the api-server at session establishment, collects node
// resource metrics on a cadence, and (on EC2) polls the instance health API.
// Updates are pushed back to the api-server through a Reporter.
type Agent struct {
	config           *config.Bundle
	ec2Client        ec2client.Client
	imdsClient       ec2imdsclient.Client
	metricsCollector metrics.Collector
	firecracker      firecracker.Provider
	reporter         Reporter
}

// NewAgent returns an Agent wired with the given dependencies. The EC2 and
// IMDS clients are read from ctx; callers must attach them via ec2.With and
// ec2imds.With before calling NewAgent.
func NewAgent(ctx context.Context, config *config.Bundle, firecracker firecracker.Provider, metricsCollector metrics.Collector, reporter Reporter) *Agent {
	return &Agent{
		config:           config,
		ec2Client:        ec2client.Get(ctx),
		imdsClient:       ec2imdsclient.Get(ctx),
		firecracker:      firecracker,
		metricsCollector: metricsCollector,
		reporter:         reporter,
	}
}

// BuildNode constructs the Node identity that the controller sends to the
// api-server when establishing a session. In EC2 runtime it pulls instance
// metadata from IMDS and an initial health snapshot from the EC2 status API.
// In local runtime it generates a UUID and reports a healthy placeholder
// status, since no provider health source is available.
//
// BuildNode is one-shot: subsequent metric and status updates are pushed by
// ReportPeriodically.
func (a *Agent) BuildNode(ctx context.Context) (*controlplanev1alpha1.Node, error) {
	resources, metrics, err := a.capacityAndMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect node capacity and metrics: %w", err)
	}

	node := &controlplanev1alpha1.Node{
		Resources: resources,
		Metrics:   metrics,
	}

	if a.runtimeIsEC2() {
		ec2Meta, err := a.ec2InstanceMeta(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read EC2 instance metadata: %w", err)
		}
		node.Metadata = &controlplanev1alpha1.NodeMeta{Id: ec2Meta.InstanceId}
		node.ProviderMetadata = &controlplanev1alpha1.Node_AwsEc2{AwsEc2: ec2Meta}

		phase, message := a.statusPhaseAndMessage(ctx, ec2Meta.InstanceId)
		node.Status = &controlplanev1alpha1.NodeStatus{Phase: phase, Message: message}
	} else {
		node.Metadata = &controlplanev1alpha1.NodeMeta{Id: uuid.NewString()}
		node.Status = &controlplanev1alpha1.NodeStatus{
			Phase:   controlplanev1alpha1.NodeStatus_PHASE_HEALTHY,
			Message: "local runtime: no provider health source",
		}
	}

	return node, nil
}

// ReportPeriodically blocks until ctx is canceled, sampling node metrics on
// every config.NodeAgent.MetricsReportInterval tick and (in EC2 runtime)
// polling the instance health API on every config.NodeAgent.HealthReportInterval
// tick. Changes are pushed to the configured Reporter; transient collection
// or transport failures are logged and do not stop the loops.
//
// nodeID is the EC2 instance id used to query DescribeInstanceStatus and is
// ignored in local runtime.
func (a *Agent) ReportPeriodically(ctx context.Context, nodeID string) {
	var wg sync.WaitGroup

	wg.Go(func() {
		a.metricsLoop(ctx)
	})

	if a.runtimeIsEC2() {
		wg.Go(func() {
			a.ec2HealthCheckLoop(ctx, nodeID)
		})
	}

	wg.Wait()
}

func (a *Agent) runtimeIsEC2() bool {
	return a.config.NodeAgent != nil && a.config.NodeAgent.Runtime == config.NodeRuntimeEC2
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
	sample, err := a.metricsCollector.Collect(ctx)
	if err != nil {
		return nil, nil, err
	}

	nodeCapacity := &controlplanev1alpha1.NodeResources{
		CpuCapacityMillicores: sample.CPUCapacityMillicores,
		MemoryCapacityBytes:   sample.MemoryCapacityBytes,
		DiskCapacityBytes:     sample.DiskCapacityBytes,
	}

	nodeMetrics := &controlplanev1alpha1.NodeMetrics{
		SampledAt:          timestamppb.Now(),
		ActiveSandboxCount: uint32(a.firecracker.Len()),
		CpuUsedMillicores:  sample.CPUUsedMillicores,
		MemoryUsedBytes:    sample.MemoryUsedBytes,
		DiskUsedBytes:      sample.DiskUsedBytes,
	}

	return nodeCapacity, nodeMetrics, nil
}

// ec2InstanceMeta reads the EC2 instance identity document via IMDS and maps
// it onto the EC2InstanceMeta proto.
func (a *Agent) ec2InstanceMeta(ctx context.Context) (*controlplanev1alpha1.EC2InstanceMeta, error) {
	out, err := a.imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return nil, err
	}
	return &controlplanev1alpha1.EC2InstanceMeta{
		InstanceId:       out.InstanceID,
		InstanceType:     out.InstanceType,
		ImageId:          out.ImageID,
		AccountId:        out.AccountID,
		Region:           out.Region,
		AvailabilityZone: out.AvailabilityZone,
		PrivateIp:        out.PrivateIP,
		KernelId:         out.KernelID,
		Architecture:     out.Architecture,
	}, nil
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

// statusPhaseAndMessage queries EC2 for the system / instance / EBS status
// checks of the given instance and renders the result as a NodeStatus phase
// and human-readable message.
func (a *Agent) statusPhaseAndMessage(ctx context.Context, instanceID string) (controlplanev1alpha1.NodeStatus_Phase, string) {
	resp, err := a.ec2Client.DescribeInstanceStatus(ctx, &ec2.DescribeInstanceStatusInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN,
			fmt.Sprintf("failed to query EC2 instance status: %v", err)
	}
	if len(resp.InstanceStatuses) == 0 {
		return controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN,
			fmt.Sprintf("EC2 returned no status entries for instance %s", instanceID)
	}

	is := resp.InstanceStatuses[0]
	var failures []string
	if msg := summarizeStatusCheck("system", is.SystemStatus); msg != "" {
		failures = append(failures, msg)
	}
	if msg := summarizeStatusCheck("instance", is.InstanceStatus); msg != "" {
		failures = append(failures, msg)
	}
	if msg := summarizeEbsCheck(is.AttachedEbsStatus); msg != "" {
		failures = append(failures, msg)
	}

	if len(failures) == 0 {
		return controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, "all EC2 status checks passed"
	}
	return controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY,
		"EC2 status checks failed:\n" + strings.Join(failures, "\n")
}

// summarizeStatusCheck renders a failing system or instance status check as a
// multi-line string, or returns "" when the check is healthy.
func summarizeStatusCheck(kind string, summary *ec2types.InstanceStatusSummary) string {
	if summary == nil || isHealthyStatus(summary.Status) {
		return ""
	}
	details := make([]string, 0, len(summary.Details))
	for _, d := range summary.Details {
		details = append(details, formatDetail(string(d.Name), string(d.Status), d.ImpairedSince))
	}
	return formatCheck(kind+" status", string(summary.Status), details)
}

// summarizeEbsCheck renders a failing attached-EBS status check; the AWS type
// differs from InstanceStatusSummary, but the rendered shape is identical.
func summarizeEbsCheck(summary *ec2types.EbsStatusSummary) string {
	if summary == nil || isHealthyStatus(summary.Status) {
		return ""
	}
	details := make([]string, 0, len(summary.Details))
	for _, d := range summary.Details {
		details = append(details, formatDetail(string(d.Name), string(d.Status), d.ImpairedSince))
	}
	return formatCheck("ebs status", string(summary.Status), details)
}

func isHealthyStatus(s ec2types.SummaryStatus) bool {
	switch s {
	case ec2types.SummaryStatusOk, ec2types.SummaryStatusInitializing, ec2types.SummaryStatusNotApplicable:
		return true
	}
	return false
}

func formatCheck(name, status string, details []string) string {
	if len(details) == 0 {
		return fmt.Sprintf("  - %s: %s", name, status)
	}
	return fmt.Sprintf("  - %s: %s\n    %s", name, status, strings.Join(details, "\n    "))
}

func formatDetail(name, status string, impairedSince *time.Time) string {
	if impairedSince != nil {
		return fmt.Sprintf("- %s: %s (since %s)", name, status, impairedSince.Format(time.RFC3339))
	}
	return fmt.Sprintf("- %s: %s", name, status)
}
