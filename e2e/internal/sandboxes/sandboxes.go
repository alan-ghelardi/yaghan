// Package sandboxes provides helpers the e2e specs share when
// driving sandbox lifecycle scenarios: polling a sandbox to a target
// phase, waiting for a snapshot to land, deleting and waiting for the
// terminal state.
//
// All helpers take a SandboxServiceClient (provided by the suite)
// rather than shelling out through yag — direct gRPC is faster and
// gives us typed proto fields to assert on.
package sandboxes

import (
	"context"
	"fmt"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pollInterval is the cadence at which sandbox state is polled. Half
// a second is comfortably below the reconciler's resync interval
// while keeping idle waits cheap.
const pollInterval = 500 * time.Millisecond

// WaitForPhase polls GetSandbox until the sandbox's status phase
// equals expected, or ctx is done. PHASE_FAILED short-circuits with
// an error — no point waiting for a phase the daemon will never
// reach.
func WaitForPhase(
	ctx context.Context,
	cli controlplanev1alpha1.SandboxServiceClient,
	sandboxID string,
	expected controlplanev1alpha1.SandboxStatus_Phase,
) error {
	return pollSandbox(ctx, cli, sandboxID, func(s *controlplanev1alpha1.Sandbox) (bool, error) {
		phase := s.GetStatus().GetPhase()
		if phase == controlplanev1alpha1.SandboxStatus_PHASE_FAILED {
			return false, fmt.Errorf("sandbox %s entered PHASE_FAILED: %s",
				sandboxID, s.GetStatus().GetMessage())
		}
		return phase == expected, nil
	})
}

// WaitForSnapshotCompleted polls until the sandbox has a non-empty
// LastSnapshot.SnapshotId AND is back at PHASE_RUNNING. Returns the
// snapshot id on success.
func WaitForSnapshotCompleted(
	ctx context.Context,
	cli controlplanev1alpha1.SandboxServiceClient,
	sandboxID string,
) (string, error) {
	var snapshotID string
	err := pollSandbox(ctx, cli, sandboxID, func(s *controlplanev1alpha1.Sandbox) (bool, error) {
		if s.GetStatus().GetPhase() == controlplanev1alpha1.SandboxStatus_PHASE_FAILED {
			return false, fmt.Errorf("sandbox %s entered PHASE_FAILED during snapshot: %s",
				sandboxID, s.GetStatus().GetMessage())
		}
		id := s.GetLastSnapshot().GetSnapshotId()
		if id == "" {
			return false, nil
		}
		if errStatus := s.GetLastSnapshot().GetError(); errStatus != nil && errStatus.GetCode() != 0 {
			return false, fmt.Errorf("sandbox %s snapshot failed: %s",
				sandboxID, errStatus.GetMessage())
		}
		if s.GetStatus().GetPhase() != controlplanev1alpha1.SandboxStatus_PHASE_RUNNING {
			return false, nil
		}
		snapshotID = id
		return true, nil
	})
	return snapshotID, err
}

// DeleteAndWait issues DeleteSandbox (auto-resolving the version via
// GetSandbox) and polls until the sandbox reaches PHASE_DELETED or
// vanishes (NotFound). Safe to call on a sandbox that was never
// created — it short-circuits when Get returns NotFound.
func DeleteAndWait(
	ctx context.Context,
	cli controlplanev1alpha1.SandboxServiceClient,
	sandboxID string,
) error {
	sandbox, err := cli.GetSandbox(ctx, &controlplanev1alpha1.GetSandboxRequest{SandboxId: sandboxID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("get sandbox %s for delete: %w", sandboxID, err)
	}

	_, err = cli.DeleteSandbox(ctx, &controlplanev1alpha1.DeleteSandboxRequest{
		SandboxId: sandboxID,
		Version:   sandbox.GetSandbox().GetMetadata().GetVersion(),
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("delete sandbox %s: %w", sandboxID, err)
	}

	return pollSandbox(ctx, cli, sandboxID, func(s *controlplanev1alpha1.Sandbox) (bool, error) {
		return s.GetStatus().GetPhase() == controlplanev1alpha1.SandboxStatus_PHASE_DELETED, nil
	})
}

// pollSandbox calls GetSandbox in a loop, handing the response to
// predicate. Returns nil as soon as predicate returns (true, nil),
// returns predicate's error if non-nil, returns ctx.Err() wrapped if
// the deadline trips. A NotFound from GetSandbox is treated as
// "predicate satisfied" only for the DELETED case: the predicate
// itself signals this by returning (true, nil) — pollSandbox can't
// distinguish, so NotFound short-circuits with success here because
// the only callers that poll past a NotFound are looking for
// DELETED.
func pollSandbox(
	ctx context.Context,
	cli controlplanev1alpha1.SandboxServiceClient,
	sandboxID string,
	predicate func(*controlplanev1alpha1.Sandbox) (bool, error),
) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	check := func() (done bool, lastErr error) {
		resp, err := cli.GetSandbox(ctx, &controlplanev1alpha1.GetSandboxRequest{SandboxId: sandboxID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return true, nil
			}
			return false, err
		}
		return predicate(resp.GetSandbox())
	}

	if done, err := check(); err != nil {
		return err
	} else if done {
		return nil
	}

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("poll sandbox %s: %w (last: %w)", sandboxID, ctx.Err(), lastErr)
			}
			return fmt.Errorf("poll sandbox %s: %w", sandboxID, ctx.Err())
		case <-ticker.C:
			done, err := check()
			if err != nil {
				lastErr = err
				continue
			}
			if done {
				return nil
			}
		}
	}
}
