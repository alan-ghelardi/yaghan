package controller

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

const (
	// resyncPageSize matches the proto's documented maximum (validated
	// upstream in ListSandboxesRequest.page_size).
	resyncPageSize int32 = 1000

	// resyncMaxPages caps a single resync pass at 100k sandboxes so a
	// pathological response (e.g. a continuation token loop on the
	// server side) cannot wedge the goroutine.
	resyncMaxPages int = 100

	// resyncCadenceJitterFactor controls the per-tick spread on the
	// configured ResyncInterval; ±10% breaks long-term cadence
	// synchronisation between nodes that started together.
	resyncCadenceJitterFactor float64 = 0.10

	// resyncStartupJitterCap upper-bounds the first-tick jitter so a
	// crashed daemon recovers within a small, predictable window.
	resyncStartupJitterCap = 30 * time.Second
)

// runResyncLoop is the goroutine entry point spawned from Run when
// Controller.ResyncInterval > 0. The first tick is offset by a small
// startup jitter so fleet-wide rolling restarts don't synchronise
// onto the api-server simultaneously; each subsequent tick is
// further jittered by ±resyncCadenceJitterFactor.
func (c *Controller) runResyncLoop(ctx context.Context) {
	interval := c.config.Controller.ResyncInterval

	if !sleepFor(ctx, startupJitter(interval)) {
		return
	}
	c.resync(ctx)

	for {
		if !sleepFor(ctx, jitteredInterval(interval, resyncCadenceJitterFactor)) {
			return
		}
		c.resync(ctx)
	}
}

// resync paginates SandboxService.ListSandboxes filtered to this
// node, newest-first, at the maximum allowed page size. Each row
// flows through the same indexer/queue path as a real-time event,
// so missed events are recovered the next time resync runs.
//
// Sandboxes whose Intent is nil are skipped: the reconciler
// short-circuits on those, and nothing else reads the indexer, so
// adding them costs without benefit.
//
// A failure on any page aborts the pass with a warning. The next
// tick will retry from the start; transient api-server errors
// don't accumulate state.
func (c *Controller) resync(ctx context.Context) {
	logger := ctxzap.Extract(ctx)
	var token string
	for page := 0; page < resyncMaxPages; page++ {
		resp, err := c.sandboxClient.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
			NodeId:            hardcodedNode().GetMetadata().GetId(),
			SortOrder:         cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
			PageSize:          resyncPageSize,
			ContinuationToken: token,
		})
		if err != nil {
			logger.Warn("controller: resync ListSandboxes failed",
				zap.Error(err), zap.Int("page", page))
			return
		}
		for _, sb := range resp.GetSandboxes() {
			if sb.GetIntent() == nil {
				continue
			}
			if c.indexer.Put(sb) {
				c.queue.Add(sb.GetMetadata().GetId())
			}
		}
		token = resp.GetContinuationToken()
		if token == "" {
			return
		}
	}
	logger.Warn("controller: resync hit max-page safety cap",
		zap.Int("max-pages", resyncMaxPages))
}

// startupJitter returns a duration uniformly drawn from
// [0, min(interval/4, resyncStartupJitterCap)]. The cap exists so a
// daemon that crashed and restarted recovers within a predictable
// window even when the configured interval is large.
//
//nolint:gosec // jitter is non-cryptographic; math/rand/v2 is sufficient.
func startupJitter(interval time.Duration) time.Duration {
	upper := interval / 4
	if upper > resyncStartupJitterCap {
		upper = resyncStartupJitterCap
	}
	if upper <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(upper) + 1))
}

// jitteredInterval returns a duration uniformly drawn from
// [interval*(1-factor), interval*(1+factor)]. Used to break long-term
// cadence synchronisation across the fleet.
//
//nolint:gosec // jitter is non-cryptographic; math/rand/v2 is sufficient.
func jitteredInterval(interval time.Duration, factor float64) time.Duration {
	spread := float64(interval) * factor
	delta := (rand.Float64()*2 - 1) * spread
	return interval + time.Duration(delta)
}

// sleepFor blocks for d or until ctx is cancelled. Returns false on
// cancellation so the caller can exit its loop.
func sleepFor(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
