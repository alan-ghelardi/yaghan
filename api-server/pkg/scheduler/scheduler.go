// Package scheduler defines the contract for assigning sandboxes to
// nodes. The api-server invokes a Scheduler exactly once per
// CreateSandbox, before persistence: the implementation mutates the
// sandbox's Node in place, and the caller persists the result.
//
// Concrete implementations live in subpackages. Today only a random
// scheduler exists (intended for integration tests and local
// development); a production-grade scheduler with capacity awareness,
// retries, and async placement is a future iteration.
package scheduler

import (
	"context"
	"errors"

	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// ErrNoHealthyNodes is returned by [Scheduler.Schedule] when the
// cluster has no node available for placement (e.g. zero registered
// nodes, or none in PHASE_HEALTHY). Callers translate this to gRPC
// FailedPrecondition.
var ErrNoHealthyNodes = errors.New("no healthy nodes available for scheduling")

// Scheduler picks a node for the supplied sandbox and assigns it via
// sb.Node. Implementations must mutate sb.Node in place; sb's other
// fields are owned by the caller and must not be touched.
//
// Schedule is invoked synchronously on the CreateSandbox path. A
// future async-placement scheduler would either return immediately
// with a sentinel "deferred" outcome or buffer sandboxes for a
// separate worker — for now, every implementation in this repo is
// synchronous and returns either nil (with sb.Node set) or an error.
type Scheduler interface {
	Schedule(ctx context.Context, sb *cpv1.Sandbox) error
}
