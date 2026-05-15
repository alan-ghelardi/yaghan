package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	cpmocks "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testNodeID is the canonical node id threaded into resync calls in
// every test in this file. It must match the assertion in
// TestResync_PaginatesAndEnqueues.
const testNodeID = "test-node"

// resyncFixture spins up a Controller wired to the supplied mock
// SandboxServiceClient, with the cluster client and reconciler stubbed
// out: the resync code path doesn't touch them, but New requires
// non-nil values for the fields it stores. Tests can drive the
// indexer/queue directly through c.
func resyncFixture(t *testing.T) (*Controller, *cpmocks.MockSandboxServiceClient) {
	t.Helper()
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	mock := cpmocks.NewMockSandboxServiceClient(ctrl)
	clusterClient := cpmocks.NewMockClusterServiceClient(ctrl)
	c := New(clusterClient, mock, &fakeReconciler{}, bundle)
	return c, mock
}

// drainQueue pops every currently-queued id, returning them in pop
// order. Each Get is paired with Done so workqueue accounting stays
// consistent if more items get added later.
func drainQueue(c *Controller) []string {
	var out []string
	for c.queue.Len() > 0 {
		id, shutdown := c.queue.Get()
		if shutdown {
			break
		}
		out = append(out, id)
		c.queue.Done(id)
	}
	return out
}

func sandboxWithIntent(id string, version int64) *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{Id: id, Version: version},
		Status:   &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_PENDING},
		Intent:   &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING},
	}
}

func sandboxWithoutIntent(id string, version int64) *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{Id: id, Version: version},
		Status:   &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_RUNNING},
	}
}

func TestResync_PaginatesAndEnqueues(t *testing.T) {
	c, mock := resyncFixture(t)
	ctx := t.Context()

	page1 := []*cpv1.Sandbox{sandboxWithIntent("sb-1", 1), sandboxWithIntent("sb-2", 1)}
	page2 := []*cpv1.Sandbox{sandboxWithIntent("sb-3", 1)}

	// First call: no token, expect a token back.
	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.AssignableToTypeOf(&cpv1.ListSandboxesRequest{})).
		DoAndReturn(func(_ context.Context, req *cpv1.ListSandboxesRequest, _ ...any) (*cpv1.ListSandboxesResponse, error) {
			assert.Equal(t, testNodeID, req.GetNodeId(),
				"resync must filter by the local node id")
			assert.Equal(t, cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST, req.GetSortOrder(),
				"resync must request newest-first ordering")
			assert.Equal(t, int32(1000), req.GetPageSize(),
				"resync must request the maximum page size")
			assert.Empty(t, req.GetContinuationToken(),
				"first page must not carry a token")
			return &cpv1.ListSandboxesResponse{
				Sandboxes:         page1,
				ContinuationToken: "page2-token",
			}, nil
		})

	// Second call: must carry the token from page 1.
	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.AssignableToTypeOf(&cpv1.ListSandboxesRequest{})).
		DoAndReturn(func(_ context.Context, req *cpv1.ListSandboxesRequest, _ ...any) (*cpv1.ListSandboxesResponse, error) {
			assert.Equal(t, "page2-token", req.GetContinuationToken(),
				"second page must carry the token from page 1")
			return &cpv1.ListSandboxesResponse{Sandboxes: page2}, nil
		})

	c.resync(ctx, testNodeID)

	// Every sandbox must end up in the indexer and on the queue.
	for _, sb := range append(page1, page2...) {
		assert.NotNil(t, c.indexer.Get(sb.GetMetadata().GetId()),
			"sandbox %q must be indexed after resync", sb.GetMetadata().GetId())
	}
	assert.ElementsMatch(t, []string{"sb-1", "sb-2", "sb-3"}, drainQueue(c),
		"every paginated sandbox must be enqueued for the worker")
}

func TestResync_FiltersNilIntent(t *testing.T) {
	c, mock := resyncFixture(t)
	ctx := t.Context()

	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		Return(&cpv1.ListSandboxesResponse{
			Sandboxes: []*cpv1.Sandbox{
				sandboxWithIntent("sb-pending", 1),
				sandboxWithoutIntent("sb-converged", 1),
				sandboxWithIntent("sb-other", 1),
			},
		}, nil)

	c.resync(ctx, testNodeID)

	assert.Nil(t, c.indexer.Get("sb-converged"),
		"sandboxes without Intent must not be indexed")
	assert.NotNil(t, c.indexer.Get("sb-pending"))
	assert.NotNil(t, c.indexer.Get("sb-other"))
	assert.ElementsMatch(t, []string{"sb-pending", "sb-other"}, drainQueue(c),
		"only sandboxes with Intent may be enqueued")
}

func TestResync_StaleVersionDoesNotEnqueue(t *testing.T) {
	c, mock := resyncFixture(t)
	ctx := t.Context()

	// Pre-seed the indexer with a newer version than the server returns.
	c.indexer.Put(sandboxWithIntent("sb-stale", 5))

	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		Return(&cpv1.ListSandboxesResponse{
			Sandboxes: []*cpv1.Sandbox{sandboxWithIntent("sb-stale", 4)},
		}, nil)

	c.resync(ctx, testNodeID)

	stored := c.indexer.Get("sb-stale")
	require.NotNil(t, stored)
	assert.Equal(t, int64(5), stored.GetMetadata().GetVersion(),
		"indexer must keep the newer locally-known version")
	assert.Empty(t, drainQueue(c),
		"stale-version resync rows must not feed the worker")
}

func TestResync_ListSandboxesErrorAbortsThePass(t *testing.T) {
	c, mock := resyncFixture(t)
	ctx := t.Context()

	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("temporary api-server error"))

	// Must not panic; must not enqueue anything.
	c.resync(ctx, testNodeID)
	assert.Empty(t, drainQueue(c))

	// A subsequent successful pass still works — one bad pass does not
	// poison the resync loop.
	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		Return(&cpv1.ListSandboxesResponse{
			Sandboxes: []*cpv1.Sandbox{sandboxWithIntent("sb-recovered", 1)},
		}, nil)

	c.resync(ctx, testNodeID)
	assert.Equal(t, []string{"sb-recovered"}, drainQueue(c))
}

func TestResync_HitsMaxPageSafetyCap(t *testing.T) {
	c, mock := resyncFixture(t)
	ctx := t.Context()

	// Always return a non-empty token so resync would loop forever
	// without the safety cap.
	mock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *cpv1.ListSandboxesRequest, _ ...any) (*cpv1.ListSandboxesResponse, error) {
			return &cpv1.ListSandboxesResponse{
				Sandboxes:         []*cpv1.Sandbox{sandboxWithIntent("sb-loop", 1)},
				ContinuationToken: "never-empty",
			}, nil
		}).
		Times(resyncMaxPages)

	c.resync(ctx, testNodeID)
	// The fact that we got here without timing out is the assertion;
	// the .Times(resyncMaxPages) above pins the call count.
}

func TestStartupJitter_BoundedByCap(t *testing.T) {
	const samples = 1000
	cases := []struct {
		name     string
		interval time.Duration
		wantMax  time.Duration
	}{
		// interval/4 < cap → jitter bounded by interval/4.
		{"short interval", 1 * time.Minute, 15 * time.Second},
		// interval/4 > cap → jitter bounded by cap.
		{"long interval", 1 * time.Hour, resyncStartupJitterCap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < samples; i++ {
				got := startupJitter(tc.interval)
				assert.GreaterOrEqual(t, got, time.Duration(0))
				assert.LessOrEqual(t, got, tc.wantMax,
					"startup jitter must not exceed %s for interval %s", tc.wantMax, tc.interval)
			}
		})
	}
}

func TestJitteredInterval_StaysWithinFactor(t *testing.T) {
	const (
		samples  = 1000
		interval = 10 * time.Minute
		factor   = 0.10
	)
	lower := time.Duration(float64(interval) * (1 - factor))
	upper := time.Duration(float64(interval) * (1 + factor))

	var sawLowHalf, sawHighHalf bool
	for i := 0; i < samples; i++ {
		got := jitteredInterval(interval, factor)
		assert.GreaterOrEqual(t, got, lower,
			"jittered interval must not fall below %s, got %s", lower, got)
		assert.LessOrEqual(t, got, upper,
			"jittered interval must not exceed %s, got %s", upper, got)
		if got < interval {
			sawLowHalf = true
		}
		if got > interval {
			sawHighHalf = true
		}
	}
	// Sanity check: across 1000 draws we should observe both halves.
	assert.True(t, sawLowHalf && sawHighHalf,
		"jittered interval must span both halves of the spread (low=%v, high=%v)", sawLowHalf, sawHighHalf)
}

func TestSleepFor_ReturnsFalseOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan bool, 1)
	go func() {
		done <- sleepFor(ctx, 10*time.Second)
	}()

	cancel()
	select {
	case got := <-done:
		assert.False(t, got, "sleepFor must return false when ctx is cancelled")
	case <-time.After(2 * time.Second):
		t.Fatal("sleepFor did not return after ctx cancellation")
	}
}

func TestSleepFor_ReturnsTrueAfterDuration(t *testing.T) {
	got := sleepFor(t.Context(), 5*time.Millisecond)
	assert.True(t, got, "sleepFor must return true when the timer fires")
}
