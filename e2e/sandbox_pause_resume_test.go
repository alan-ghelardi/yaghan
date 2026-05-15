package e2e_test

import (
	"context"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

var _ = Describe("Sandbox pause and resume", Ordered, func() {
	const sandboxID = "pause-b"

	BeforeAll(func() {
		requireRoot()
	})

	It("runs a sandbox through Running → Paused → Running and stays functional",
		func(specCtx SpecContext) {
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			res, err := yag.Run(ctx, "sandbox", "create", sandboxID)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed(),
				"sandbox never reached RUNNING")

			res, err = yag.Run(ctx, "sandbox", "pause", sandboxID)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_PAUSED)).To(Succeed(),
				"sandbox never reached PAUSED")

			res, err = yag.Run(ctx, "sandbox", "resume", sandboxID)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed(),
				"sandbox never returned to RUNNING after resume")

			// One exec post-resume proves the agent's vsock
			// conversation survived the pause/resume cycle.
			res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/bin/sh", "-c", "echo alive")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			Expect(string(res.Stdout)).To(Equal("alive\n"))
		})
})
