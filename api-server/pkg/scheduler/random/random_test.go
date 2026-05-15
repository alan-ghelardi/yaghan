package random_test

import (
	"context"
	"errors"
	"testing"

	nodedb "github.com/alan-ghelardi/yaghan/api-server/pkg/db/node"
	nodemocks "github.com/alan-ghelardi/yaghan/api-server/pkg/db/node/mocks"
	"github.com/alan-ghelardi/yaghan/api-server/pkg/scheduler"
	"github.com/alan-ghelardi/yaghan/api-server/pkg/scheduler/random"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newSandbox() *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{Id: "sb-1", Namespace: "team-alpha"},
	}
}

func healthyNode(id string) *cpv1.Node {
	return &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: id},
		Status:   &cpv1.NodeStatus{Phase: cpv1.NodeStatus_PHASE_HEALTHY},
	}
}

func TestSchedule_PicksOneOfTheReturnedNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := nodemocks.NewMockDB(ctrl)
	candidates := []*cpv1.Node{
		healthyNode("node-a"),
		healthyNode("node-b"),
		healthyNode("node-c"),
	}

	mock.EXPECT().
		List(gomock.Any(), gomock.AssignableToTypeOf(nodedb.ListOptions{})).
		DoAndReturn(func(_ context.Context, opts nodedb.ListOptions) ([]*cpv1.Node, string, error) {
			assert.Equal(t, cpv1.NodeStatus_PHASE_HEALTHY, opts.StatusPhase,
				"scheduler must filter by HEALTHY phase")
			assert.Equal(t, int32(1000), opts.PageSize,
				"scheduler must request the maximum page size")
			return candidates, "", nil
		})

	s := random.New(mock)
	sb := newSandbox()
	require.NoError(t, s.Schedule(t.Context(), sb))

	require.NotNil(t, sb.GetNode(), "Schedule must set sb.Node on success")
	assert.Contains(t, []string{"node-a", "node-b", "node-c"}, sb.GetNode().GetId(),
		"chosen node must be one of the candidates")
}

func TestSchedule_NoCandidatesReturnsErrNoHealthyNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := nodemocks.NewMockDB(ctrl)

	mock.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(nil, "", nil)

	s := random.New(mock)
	sb := newSandbox()
	err := s.Schedule(t.Context(), sb)
	require.Error(t, err)
	assert.True(t, errors.Is(err, scheduler.ErrNoHealthyNodes),
		"empty candidate list must wrap ErrNoHealthyNodes, got: %v", err)
	assert.Nil(t, sb.GetNode(),
		"sb.Node must remain unset when scheduling fails")
}

func TestSchedule_DBErrorIsPropagated(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := nodemocks.NewMockDB(ctrl)

	dbErr := errors.New("transient dynamodb failure")
	mock.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(nil, "", dbErr)

	s := random.New(mock)
	sb := newSandbox()
	err := s.Schedule(t.Context(), sb)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dbErr),
		"DB error must propagate (wrapped) so callers can distinguish it from ErrNoHealthyNodes")
	assert.False(t, errors.Is(err, scheduler.ErrNoHealthyNodes),
		"a DB failure must NOT be misreported as ErrNoHealthyNodes")
	assert.Nil(t, sb.GetNode())
}

func TestSchedule_SingleCandidateAlwaysPicked(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := nodemocks.NewMockDB(ctrl)

	mock.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return([]*cpv1.Node{healthyNode("only-node")}, "", nil)

	s := random.New(mock)
	sb := newSandbox()
	require.NoError(t, s.Schedule(t.Context(), sb))
	assert.Equal(t, "only-node", sb.GetNode().GetId())
}

// TestSchedule_DistributionCoversAllCandidates is a probabilistic
// sanity check: with three candidates and 200 trials, the probability
// of any single candidate never being picked is (2/3)^200 ≈ 10^-35,
// so a non-flaky implementation always covers all three.
func TestSchedule_DistributionCoversAllCandidates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := nodemocks.NewMockDB(ctrl)

	candidates := []*cpv1.Node{
		healthyNode("node-a"),
		healthyNode("node-b"),
		healthyNode("node-c"),
	}
	mock.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(candidates, "", nil).
		AnyTimes()

	s := random.New(mock)
	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		sb := newSandbox()
		require.NoError(t, s.Schedule(t.Context(), sb))
		seen[sb.GetNode().GetId()]++
	}

	for _, id := range []string{"node-a", "node-b", "node-c"} {
		assert.Greater(t, seen[id], 0,
			"node %q was never picked across 200 trials — selection is not random across candidates", id)
	}
}
