// Package random provides a non-production [scheduler.Scheduler] that
// picks a healthy node uniformly at random from the first page of
// ListNodes results.
//
// It exists for integration tests and local development: clusters
// that fit in a single page of the all-healthy listing get a uniform
// random pick; clusters larger than schedulerPageSize bias toward
// whichever segment of the lmt-sorted set lands on the first page.
// That bias is acceptable for this scheduler's audience — production
// placement will use a different implementation entirely.
package random

import (
	"context"
	"fmt"
	"math/rand/v2"

	nodedb "github.com/alan-ghelardi/yaghan/api-server/pkg/db/node"
	"github.com/alan-ghelardi/yaghan/api-server/pkg/scheduler"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
)

// schedulerPageSize matches the proto's documented maximum, so the
// scheduler considers as many candidates as a single ListNodes call
// will yield.
const schedulerPageSize int32 = 1000

// Scheduler picks a healthy node uniformly at random from the first
// page of all-healthy ListNodes results.
type Scheduler struct {
	nodeDB nodedb.DB
}

// New returns a random Scheduler backed by the supplied node DB.
func New(nodeDB nodedb.DB) *Scheduler {
	return &Scheduler{nodeDB: nodeDB}
}

var _ scheduler.Scheduler = (*Scheduler)(nil)

// Schedule implements [scheduler.Scheduler].
//
// The selection is uniform across the candidates returned by a single
// page of ListNodes(StatusPhase=HEALTHY, page_size=1000). When the
// cluster has no healthy nodes the call returns
// [scheduler.ErrNoHealthyNodes]; transient DB failures are wrapped
// and propagated.
func (s *Scheduler) Schedule(ctx context.Context, sb *cpv1.Sandbox) error {
	candidates, _, err := s.nodeDB.List(ctx, nodedb.ListOptions{
		StatusPhase: cpv1.NodeStatus_PHASE_HEALTHY,
		SortOrder:   cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
		PageSize:    schedulerPageSize,
	})
	if err != nil {
		return fmt.Errorf("random scheduler: list healthy nodes: %w", err)
	}
	if len(candidates) == 0 {
		return scheduler.ErrNoHealthyNodes
	}

	chosen := candidates[pickIndex(len(candidates))]
	sb.Node = &cpv1.NodeRef{Id: chosen.GetMetadata().GetId()}
	return nil
}

// pickIndex returns a uniform random index in [0, n). Wrapped in a
// helper so the gosec exception covers exactly the call site that
// needs it; the source of randomness is non-cryptographic by design.
//
//nolint:gosec // placement randomness is non-cryptographic.
func pickIndex(n int) int {
	return rand.IntN(n)
}
