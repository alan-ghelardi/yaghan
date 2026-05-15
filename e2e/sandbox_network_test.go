package e2e_test

import (
	"context"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

var _ = Describe("Sandbox network connectivity", Ordered, func() {
	const sandboxID = "net-d"

	BeforeAll(func() {
		requireRoot()
		if !suite.EgressEnabled {
			Skip("egress disabled in the daemon config; this only runs when the suite renders egress-enabled: true")
		}
	})

	It("reaches an external endpoint from inside the guest",
		func(specCtx SpecContext) {
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			createRes, err := yag.Run(ctx, "sandbox", "create", sandboxID)
			Expect(err).NotTo(HaveOccurred())
			Expect(createRes.ExitCode).To(Equal(0), "stderr:\n%s", createRes.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			// Ubuntu Base ships glibc but neither curl nor wget, so
			// we drive the host-resolution + UDP-53 path through
			// `getent hosts <name>`. That exercises:
			//   * /etc/resolv.conf is wired (daemon-side responsibility)
			//   * the guest's network namespace can reach the upstream
			//     resolver (UDP egress over the TAP + masquerade)
			//   * DNS responds in time
			// example.com is an RFC 2606 / IANA-maintained reference
			// domain — about as stable as external hostnames get.
			// One retry covers transient DNS / route hiccups; if both
			// attempts fail, the egress path is broken.
			var (
				res     *yag.Result
				lastOut []byte
				lastErr []byte
			)
			for attempt := 1; attempt <= 2; attempt++ {
				res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
					"/usr/bin/getent", "hosts", "example.com")
				lastOut = res.Stdout
				lastErr = res.Stderr
				if err == nil && res.ExitCode == 0 {
					return
				}
			}
			Expect(err).NotTo(HaveOccurred(),
				"getent from guest failed after 2 attempts\nstdout:\n%s\nstderr:\n%s",
				lastOut, lastErr)
			Expect(res.ExitCode).To(Equal(0),
				"getent from guest failed after 2 attempts\nstdout:\n%s\nstderr:\n%s",
				lastOut, lastErr)
		})
})
