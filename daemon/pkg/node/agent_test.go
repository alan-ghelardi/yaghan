package node

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	ec2client "github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2"
	ec2mocks "github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2/mocks"
	ec2imdsclient "github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2imds"
	imdsmocks "github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2imds/mocks"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/config"
	fcmocks "github.com/alan-ghelardi/yaghan/daemon/pkg/firecracker/mocks"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/node/metrics"
	metricsmocks "github.com/alan-ghelardi/yaghan/daemon/pkg/node/metrics/mocks"
	nodemocks "github.com/alan-ghelardi/yaghan/daemon/pkg/node/mocks"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type agentFixture struct {
	ctrl     *gomock.Controller
	metrics  *metricsmocks.MockCollector
	reporter *nodemocks.MockReporter
	provider *fcmocks.MockProvider
	ec2API   *ec2mocks.MockClient
	imdsAPI  *imdsmocks.MockClient
	agent    *Agent
}

func newAgentFixture(t *testing.T, runtime config.NodeRuntime) *agentFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &agentFixture{
		ctrl:     ctrl,
		metrics:  metricsmocks.NewMockCollector(ctrl),
		reporter: nodemocks.NewMockReporter(ctrl),
		provider: fcmocks.NewMockProvider(ctrl),
		ec2API:   ec2mocks.NewMockClient(ctrl),
		imdsAPI:  imdsmocks.NewMockClient(ctrl),
	}
	cfg := &config.Bundle{
		NodeAgent: &config.NodeAgent{
			Runtime:               runtime,
			NodeIDFile:            filepath.Join(t.TempDir(), "node.id"),
			MetricsReportInterval: 5 * time.Millisecond,
			HealthReportInterval:  5 * time.Millisecond,
		},
	}
	ctx := ec2client.With(t.Context(), f.ec2API)
	ctx = ec2imdsclient.With(ctx, f.imdsAPI)
	f.agent = NewAgent(ctx, cfg, f.provider, f.metrics, f.reporter)
	return f
}

func sampleMetrics() *metrics.Sample {
	return &metrics.Sample{
		CPUCapacityMillicores: 4000,
		CPUUsedMillicores:     500,
		MemoryCapacityBytes:   8 << 30,
		MemoryUsedBytes:       1 << 30,
		DiskCapacityBytes:     100 << 30,
		DiskUsedBytes:         10 << 30,
	}
}

// ---------------------------- BuildNode ------------------------------------

func TestAgent_BuildNode_LocalRuntime(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
	f.provider.EXPECT().Len().Return(3)

	got, err := f.agent.BuildNode(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, got.Metadata)
	_, parseErr := uuid.Parse(got.Metadata.Id)
	require.NoError(t, parseErr, "metadata.id should be a UUID, got %q", got.Metadata.Id)

	require.NotNil(t, got.Resources)
	assert.Equal(t, uint32(4000), got.Resources.CpuCapacityMillicores)
	assert.Equal(t, uint64(8<<30), got.Resources.MemoryCapacityBytes)
	assert.Equal(t, uint64(100<<30), got.Resources.DiskCapacityBytes)

	require.NotNil(t, got.Metrics)
	assert.Equal(t, uint32(500), got.Metrics.CpuUsedMillicores)
	assert.Equal(t, uint64(1<<30), got.Metrics.MemoryUsedBytes)
	assert.Equal(t, uint32(3), got.Metrics.ActiveSandboxCount)
	assert.NotNil(t, got.Metrics.SampledAt)

	require.NotNil(t, got.Status)
	assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, got.Status.Phase)
	assert.NotEmpty(t, got.Status.Message)

	assert.Nil(t, got.ProviderMetadata, "local runtime must not set provider metadata")
}

// TestAgent_BuildNode_LocalRuntime_StableAcrossCalls verifies that
// repeated BuildNode invocations on the same Agent return the same
// node id — the persisted file is the source of truth after the first
// call. This is the inverted assertion of the previous "unique ids"
// test, which encoded the bug we are now fixing.
func TestAgent_BuildNode_LocalRuntime_StableAcrossCalls(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil).Times(2)
	f.provider.EXPECT().Len().Return(0).Times(2)

	first, err := f.agent.BuildNode(t.Context())
	require.NoError(t, err)
	second, err := f.agent.BuildNode(t.Context())
	require.NoError(t, err)
	assert.Equal(t, first.Metadata.Id, second.Metadata.Id,
		"node id must survive across BuildNode calls (file persistence)")
}

// TestAgent_BuildNode_LocalRuntime_StableAcrossAgentInstances covers
// the daemon-restart scenario: a fresh Agent constructed against the
// same NodeIDFile path must adopt the previously-persisted id rather
// than minting a new one.
func TestAgent_BuildNode_LocalRuntime_StableAcrossAgentInstances(t *testing.T) {
	nodeIDFile := filepath.Join(t.TempDir(), "node.id")

	buildOnce := func() string {
		ctrl := gomock.NewController(t)
		metricsMock := metricsmocks.NewMockCollector(ctrl)
		reporterMock := nodemocks.NewMockReporter(ctrl)
		providerMock := fcmocks.NewMockProvider(ctrl)
		metricsMock.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
		providerMock.EXPECT().Len().Return(0)

		cfg := &config.Bundle{
			NodeAgent: &config.NodeAgent{
				Runtime:               config.NodeRuntimeLocal,
				NodeIDFile:            nodeIDFile,
				MetricsReportInterval: 5 * time.Millisecond,
				HealthReportInterval:  5 * time.Millisecond,
			},
		}
		agent := NewAgent(t.Context(), cfg, providerMock, metricsMock, reporterMock)
		got, err := agent.BuildNode(t.Context())
		require.NoError(t, err)
		return got.Metadata.Id
	}

	first := buildOnce()
	second := buildOnce()
	assert.Equal(t, first, second,
		"daemon restart must re-register under the persisted node id")
}

func TestAgent_BuildNode_CollectError(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(nil, errors.New("boom"))

	got, err := f.agent.BuildNode(t.Context())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to collect node capacity and metrics")
	assert.Contains(t, err.Error(), "boom")
}

func TestAgent_BuildNode_EC2Runtime_Healthy(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
	f.provider.EXPECT().Len().Return(1)
	f.imdsAPI.EXPECT().GetInstanceIdentityDocument(gomock.Any(), gomock.Any()).Return(
		&imds.GetInstanceIdentityDocumentOutput{
			InstanceIdentityDocument: imds.InstanceIdentityDocument{
				InstanceID:       "i-0123456789abcdef",
				InstanceType:     "m5.large",
				ImageID:          "ami-12345",
				AccountID:        "111122223333",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
				PrivateIP:        "10.0.0.42",
				KernelID:         "aki-1",
				Architecture:     "x86_64",
			},
		}, nil)
	f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).Return(
		&ec2.DescribeInstanceStatusOutput{
			InstanceStatuses: []ec2types.InstanceStatus{{
				SystemStatus:      &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
				InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
				AttachedEbsStatus: &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusOk},
			}},
		}, nil)

	got, err := f.agent.BuildNode(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, got.Metadata)
	assert.Equal(t, "i-0123456789abcdef", got.Metadata.Id)

	ec2Meta := got.GetAwsEc2()
	require.NotNil(t, ec2Meta)
	assert.Equal(t, "i-0123456789abcdef", ec2Meta.InstanceId)
	assert.Equal(t, "m5.large", ec2Meta.InstanceType)
	assert.Equal(t, "ami-12345", ec2Meta.ImageId)
	assert.Equal(t, "111122223333", ec2Meta.AccountId)
	assert.Equal(t, "us-east-1", ec2Meta.Region)
	assert.Equal(t, "us-east-1a", ec2Meta.AvailabilityZone)
	assert.Equal(t, "10.0.0.42", ec2Meta.PrivateIp)
	assert.Equal(t, "aki-1", ec2Meta.KernelId)
	assert.Equal(t, "x86_64", ec2Meta.Architecture)

	require.NotNil(t, got.Status)
	assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, got.Status.Phase)
}

func TestAgent_BuildNode_EC2Runtime_IMDSError(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
	f.provider.EXPECT().Len().Return(0)
	f.imdsAPI.EXPECT().GetInstanceIdentityDocument(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("imds unreachable"))

	got, err := f.agent.BuildNode(t.Context())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to read EC2 instance metadata")
	assert.Contains(t, err.Error(), "imds unreachable")
}

func TestAgent_BuildNode_EC2Runtime_StatusAPIErrorDegradesToUnknown(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
	f.provider.EXPECT().Len().Return(0)
	f.imdsAPI.EXPECT().GetInstanceIdentityDocument(gomock.Any(), gomock.Any()).Return(
		&imds.GetInstanceIdentityDocumentOutput{
			InstanceIdentityDocument: imds.InstanceIdentityDocument{InstanceID: "i-fail"},
		}, nil)
	f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("ec2 unreachable"))

	got, err := f.agent.BuildNode(t.Context())
	require.NoError(t, err, "describe-status failure must not fail BuildNode; status degrades to UNKNOWN")
	require.NotNil(t, got)
	assert.Equal(t, "i-fail", got.Metadata.Id)
	require.NotNil(t, got.Status)
	assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "ec2 unreachable")
}

// ---------------------- statusPhaseAndMessage ------------------------------

func TestAgent_StatusPhaseAndMessage(t *testing.T) {
	impairedSince := time.Date(2026, 5, 7, 12, 34, 56, 0, time.UTC)
	healthySystem := &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk}
	healthyEbs := &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusOk}

	cases := []struct {
		name            string
		resp            *ec2.DescribeInstanceStatusOutput
		err             error
		wantPhase       controlplanev1alpha1.NodeStatus_Phase
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "all checks healthy with mixed transitional statuses",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{
					SystemStatus:      &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
					InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusInitializing},
					AttachedEbsStatus: &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusNotApplicable},
				}},
			},
			wantPhase:    controlplanev1alpha1.NodeStatus_PHASE_HEALTHY,
			wantContains: []string{"all EC2 status checks passed"},
		},
		{
			name: "nil summaries are tolerated as healthy",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{}},
			},
			wantPhase:    controlplanev1alpha1.NodeStatus_PHASE_HEALTHY,
			wantContains: []string{"all EC2 status checks passed"},
		},
		{
			name:         "empty response returns UNKNOWN naming the instance",
			resp:         &ec2.DescribeInstanceStatusOutput{InstanceStatuses: nil},
			wantPhase:    controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN,
			wantContains: []string{"i-test", "no status entries"},
		},
		{
			name:         "API error returns UNKNOWN with wrapped message",
			err:          errors.New("aws fail"),
			wantPhase:    controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN,
			wantContains: []string{"aws fail", "failed to query"},
		},
		{
			name: "system check impaired with detail and timestamp",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{
					SystemStatus: &ec2types.InstanceStatusSummary{
						Status: ec2types.SummaryStatusImpaired,
						Details: []ec2types.InstanceStatusDetails{{
							Name:          ec2types.StatusName("reachability"),
							Status:        ec2types.StatusType("failed"),
							ImpairedSince: &impairedSince,
						}},
					},
					InstanceStatus:    healthySystem,
					AttachedEbsStatus: healthyEbs,
				}},
			},
			wantPhase: controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY,
			wantContains: []string{
				"EC2 status checks failed",
				"system status: impaired",
				"reachability: failed",
				"2026-05-07T12:34:56Z",
			},
			wantNotContains: []string{"instance status", "ebs status"},
		},
		{
			name: "instance check impaired without details",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{
					SystemStatus:      healthySystem,
					InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusImpaired},
					AttachedEbsStatus: healthyEbs,
				}},
			},
			wantPhase:       controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY,
			wantContains:    []string{"instance status: impaired"},
			wantNotContains: []string{"system status", "ebs status", "since"},
		},
		{
			name: "ebs check impaired with detail",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{
					SystemStatus:   healthySystem,
					InstanceStatus: healthySystem,
					AttachedEbsStatus: &ec2types.EbsStatusSummary{
						Status: ec2types.SummaryStatusImpaired,
						Details: []ec2types.EbsStatusDetails{{
							Name:   ec2types.StatusName("reachability"),
							Status: ec2types.StatusType("failed"),
						}},
					},
				}},
			},
			wantPhase:       controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY,
			wantContains:    []string{"ebs status: impaired", "reachability: failed"},
			wantNotContains: []string{"system status", "instance status"},
		},
		{
			name: "system + ebs impaired, system rendered first",
			resp: &ec2.DescribeInstanceStatusOutput{
				InstanceStatuses: []ec2types.InstanceStatus{{
					SystemStatus:   &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusImpaired},
					InstanceStatus: healthySystem,
					AttachedEbsStatus: &ec2types.EbsStatusSummary{
						Status: ec2types.SummaryStatusImpaired,
					},
				}},
			},
			wantPhase:    controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY,
			wantContains: []string{"system status", "ebs status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAgentFixture(t, config.NodeRuntimeEC2)
			f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).Return(tc.resp, tc.err)

			phase, message := f.agent.statusPhaseAndMessage(t.Context(), "i-test")

			assert.Equal(t, tc.wantPhase, phase)
			for _, sub := range tc.wantContains {
				assert.Contains(t, message, sub, "message: %q", message)
			}
			for _, sub := range tc.wantNotContains {
				assert.NotContains(t, message, sub, "message: %q", message)
			}

			if tc.name == "system + ebs impaired, system rendered first" {
				assert.Less(t, strings.Index(message, "system status"), strings.Index(message, "ebs status"),
					"system check should appear before ebs check in the message")
			}
		})
	}
}

// ---------------------- capacityAndMetrics ---------------------------------

func TestAgent_CapacityAndMetrics_MapsFieldsAndSandboxCount(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil)
	f.provider.EXPECT().Len().Return(7)

	resources, nodeMetrics, err := f.agent.capacityAndMetrics(t.Context())
	require.NoError(t, err)

	assert.Equal(t, uint32(4000), resources.CpuCapacityMillicores)
	assert.Equal(t, uint64(8<<30), resources.MemoryCapacityBytes)
	assert.Equal(t, uint64(100<<30), resources.DiskCapacityBytes)

	assert.Equal(t, uint32(500), nodeMetrics.CpuUsedMillicores)
	assert.Equal(t, uint64(1<<30), nodeMetrics.MemoryUsedBytes)
	assert.Equal(t, uint64(10<<30), nodeMetrics.DiskUsedBytes)
	assert.Equal(t, uint32(7), nodeMetrics.ActiveSandboxCount)
	require.NotNil(t, nodeMetrics.SampledAt)
}

// ---------------------- metricsLoop ----------------------------------------

func TestAgent_MetricsLoop_ReportsAndExitsOnContextCancel(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)

	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil).AnyTimes()
	f.provider.EXPECT().Len().Return(0).AnyTimes()

	reported := make(chan struct{}, 1)
	f.reporter.EXPECT().ReportNodeMetrics(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *controlplanev1alpha1.NodeMetrics) error {
			select {
			case reported <- struct{}{}:
			default:
			}
			return nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	finished := make(chan struct{})
	go func() {
		f.agent.metricsLoop(ctx)
		close(finished)
	}()

	select {
	case <-reported:
	case <-time.After(time.Second):
		t.Fatal("ReportNodeMetrics was not called within 1s")
	}

	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("metricsLoop did not return after context cancel")
	}
}

func TestAgent_MetricsLoop_CollectorErrorDoesNotStopLoop(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)

	var collects atomic.Int32
	twoCollects := make(chan struct{}, 1)
	f.metrics.EXPECT().Collect(gomock.Any()).DoAndReturn(
		func(context.Context) (*metrics.Sample, error) {
			if collects.Add(1) == 2 {
				select {
				case twoCollects <- struct{}{}:
				default:
				}
			}
			return nil, errors.New("collect failed")
		}).AnyTimes()
	// reporter must NOT be called when collect errors

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.metricsLoop(ctx)
		close(finished)
	}()

	select {
	case <-twoCollects:
	case <-time.After(time.Second):
		t.Fatal("collector was not called twice within 1s; loop appears to have stopped")
	}

	cancel()
	<-finished
}

func TestAgent_MetricsLoop_ReporterErrorDoesNotStopLoop(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil).AnyTimes()
	f.provider.EXPECT().Len().Return(0).AnyTimes()

	var reports atomic.Int32
	twoReports := make(chan struct{}, 1)
	f.reporter.EXPECT().ReportNodeMetrics(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *controlplanev1alpha1.NodeMetrics) error {
			if reports.Add(1) == 2 {
				select {
				case twoReports <- struct{}{}:
				default:
				}
			}
			return errors.New("report failed")
		}).AnyTimes()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.metricsLoop(ctx)
		close(finished)
	}()

	select {
	case <-twoReports:
	case <-time.After(time.Second):
		t.Fatal("reporter was not called twice within 1s; loop appears to have stopped")
	}
	cancel()
	<-finished
}

// ---------------------- ec2HealthCheckLoop ---------------------------------

func TestAgent_EC2HealthCheckLoop_ReportsOnPhaseTransition(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)

	healthyResp := &ec2.DescribeInstanceStatusOutput{
		InstanceStatuses: []ec2types.InstanceStatus{{
			SystemStatus:      &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
			InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
			AttachedEbsStatus: &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusOk},
		}},
	}
	unhealthyResp := &ec2.DescribeInstanceStatusOutput{
		InstanceStatuses: []ec2types.InstanceStatus{{
			SystemStatus:      &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusImpaired},
			InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
			AttachedEbsStatus: &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusOk},
		}},
	}

	var describeCalls atomic.Int32
	f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *ec2.DescribeInstanceStatusInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceStatusOutput, error) {
			n := describeCalls.Add(1)
			if n <= 2 {
				return healthyResp, nil
			}
			return unhealthyResp, nil
		}).AnyTimes()

	phaseTransitions := make(chan controlplanev1alpha1.NodeStatus_Phase, 4)
	f.reporter.EXPECT().ReportStatusPhase(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, phase controlplanev1alpha1.NodeStatus_Phase, _ string) error {
			phaseTransitions <- phase
			return nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.ec2HealthCheckLoop(ctx, "i-test")
		close(finished)
	}()

	got := make([]controlplanev1alpha1.NodeStatus_Phase, 0, 2)
	deadline := time.After(time.Second)
collect:
	for len(got) < 2 {
		select {
		case p := <-phaseTransitions:
			got = append(got, p)
		case <-deadline:
			break collect
		}
	}
	cancel()
	<-finished

	require.GreaterOrEqual(t, len(got), 2,
		"expected at least 2 phase transitions (HEALTHY then UNHEALTHY), got %d: %v", len(got), got)
	assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, got[0])
	assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY, got[1])
}

func TestAgent_EC2HealthCheckLoop_DescribeErrorReportsUnknown(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)
	f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("ec2 down")).AnyTimes()

	reported := make(chan controlplanev1alpha1.NodeStatus_Phase, 1)
	f.reporter.EXPECT().ReportStatusPhase(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, phase controlplanev1alpha1.NodeStatus_Phase, _ string) error {
			select {
			case reported <- phase:
			default:
			}
			return nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.ec2HealthCheckLoop(ctx, "i-test")
		close(finished)
	}()

	select {
	case phase := <-reported:
		assert.Equal(t, controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN, phase)
	case <-time.After(time.Second):
		t.Fatal("expected a PHASE_UNKNOWN report within 1s")
	}
	cancel()
	<-finished
}

// ---------------------- ReportPeriodically --------------------------------

func TestAgent_ReportPeriodically_LocalRuntime_DoesNotPollEC2(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeLocal)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil).AnyTimes()
	f.provider.EXPECT().Len().Return(0).AnyTimes()

	reported := make(chan struct{}, 1)
	f.reporter.EXPECT().ReportNodeMetrics(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *controlplanev1alpha1.NodeMetrics) error {
			select {
			case reported <- struct{}{}:
			default:
			}
			return nil
		}).AnyTimes()
	// no expectations on ec2API or reporter.ReportStatusPhase: any call would fail the test

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.ReportPeriodically(ctx, "ignored-in-local")
		close(finished)
	}()

	select {
	case <-reported:
	case <-time.After(time.Second):
		t.Fatal("metrics loop did not run")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("ReportPeriodically did not return after cancel")
	}
}

func TestAgent_ReportPeriodically_EC2Runtime_StartsBothLoops(t *testing.T) {
	f := newAgentFixture(t, config.NodeRuntimeEC2)
	f.metrics.EXPECT().Collect(gomock.Any()).Return(sampleMetrics(), nil).AnyTimes()
	f.provider.EXPECT().Len().Return(0).AnyTimes()
	f.ec2API.EXPECT().DescribeInstanceStatus(gomock.Any(), gomock.Any()).Return(
		&ec2.DescribeInstanceStatusOutput{
			InstanceStatuses: []ec2types.InstanceStatus{{
				SystemStatus:      &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
				InstanceStatus:    &ec2types.InstanceStatusSummary{Status: ec2types.SummaryStatusOk},
				AttachedEbsStatus: &ec2types.EbsStatusSummary{Status: ec2types.SummaryStatusOk},
			}},
		}, nil).AnyTimes()

	metricsReported := make(chan struct{}, 1)
	statusReported := make(chan struct{}, 1)
	f.reporter.EXPECT().ReportNodeMetrics(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *controlplanev1alpha1.NodeMetrics) error {
			select {
			case metricsReported <- struct{}{}:
			default:
			}
			return nil
		}).AnyTimes()
	f.reporter.EXPECT().ReportStatusPhase(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, controlplanev1alpha1.NodeStatus_Phase, string) error {
			select {
			case statusReported <- struct{}{}:
			default:
			}
			return nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		f.agent.ReportPeriodically(ctx, "i-test")
		close(finished)
	}()

	deadline := time.After(time.Second)
	gotMetrics, gotStatus := false, false
	for !gotMetrics || !gotStatus {
		select {
		case <-metricsReported:
			gotMetrics = true
		case <-statusReported:
			gotStatus = true
		case <-deadline:
			t.Fatalf("did not observe both loops within 1s (metrics=%v, status=%v)", gotMetrics, gotStatus)
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("ReportPeriodically did not return after cancel")
	}
}
