package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

var _ = Describe("Sandbox snapshot round-trip", Ordered, func() {
	const (
		originalID = "snap-c"
		restoredID = "snap-c-restored"
	)

	BeforeAll(func() {
		requireRoot()
	})

	It("snapshots a running sandbox and restores its state into a new sandbox",
		func(specCtx SpecContext) {
			// Wider budget — two VM boots (one from rootfs, one from
			// snapshot) plus an S3 round-trip dominate.
			ctx, cancel := context.WithTimeout(specCtx, 3*time.Minute)
			defer cancel()

			// --- create original, write a marker file -----------------
			res, err := yag.Run(ctx, "sandbox", "create", originalID)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, originalID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, originalID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			res, err = yag.Run(ctx, "sandbox", "exec", originalID, "--",
				"/bin/sh", "-c", "echo marker > /tmp/marker")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)

			// --- trigger the snapshot ---------------------------------
			res, err = yag.Run(ctx, "sandbox", "snapshot", originalID,
				"--description", "e2e")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)

			snapshotID, err := sandboxes.WaitForSnapshotCompleted(ctx, suite.SandboxCli, originalID)
			Expect(err).NotTo(HaveOccurred(), "snapshot never completed")
			Expect(snapshotID).NotTo(BeEmpty())

			// --- 5f03dbb regression: exec on the *original* after a
			// snapshot must not panic the daemon. Before that fix the
			// memoised vsock transport stayed cached after the
			// snapshot's pause/resume cycle and the next Exec
			// crashed.
			res, err = yag.Run(ctx, "sandbox", "exec", originalID, "--",
				"/bin/cat", "/tmp/marker")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0),
				"5f03dbb regression: exec on the original sandbox failed post-snapshot. stderr:\n%s",
				res.Stderr)
			Expect(string(res.Stdout)).To(Equal("marker\n"))

			// --- restore into a new sandbox ----------------------------
			res, err = yag.Run(ctx, "sandbox", "create", restoredID,
				"--source", "snapshot:"+snapshotID)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, restoredID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, restoredID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed(),
				"snapshot-derived sandbox never reached RUNNING")

			// --- marker survived the S3 round-trip --------------------
			res, err = yag.Run(ctx, "sandbox", "exec", restoredID, "--",
				"/bin/cat", "/tmp/marker")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			Expect(string(res.Stdout)).To(Equal("marker\n"),
				"restored sandbox should observe the marker we wrote before snapshotting")

			// --- cp from the restored sandbox works -------------------
			out := filepath.Join(suite.RunDir, "snapshot-marker.out")
			res, err = yag.Run(ctx, "sandbox", "cp",
				restoredID+":/tmp/marker", out)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			contents, err := os.ReadFile(out)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal("marker\n"))
		})
})
