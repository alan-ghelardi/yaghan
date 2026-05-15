package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

var _ = Describe("Sandbox lifecycle (create → exec → cp → delete)", Ordered, func() {
	const sandboxID = "lifecycle-a"

	// BeforeAll (not BeforeEach) creates the sandbox once and the
	// Ordered Its share it. A DeferCleanup registered here runs at
	// the end of the whole container, so cleanup survives Its
	// running in sequence — DeferCleanup inside an It would tear
	// the sandbox down between Its, which is what was breaking the
	// scenario in the first run.
	BeforeAll(func(specCtx SpecContext) {
		requireRoot()

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
			controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).
			To(Succeed(), "sandbox %s never reached RUNNING", sandboxID)
	})

	It("propagates exec stdout and exit codes through the bidi stream",
		func(specCtx SpecContext) {
			ctx, cancel := context.WithTimeout(specCtx, execBudget)
			defer cancel()

			res, err := yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/bin/sh", "-c", "echo hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			Expect(string(res.Stdout)).To(Equal("hello\n"))

			res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/bin/sh", "-c", "exit 42")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(42),
				"expected guest exit 42 to propagate; stderr:\n%s", res.Stderr)
		})

	It("uploads a file, runs cat to verify, then downloads and asserts contents",
		func(specCtx SpecContext) {
			ctx, cancel := context.WithTimeout(specCtx, execBudget)
			defer cancel()

			payload := "yaghan-e2e-marker\n"
			local := filepath.Join(suite.RunDir, "lifecycle-upload.txt")
			Expect(os.WriteFile(local, []byte(payload), 0o600)).To(Succeed())

			res, err := yag.Run(ctx, "sandbox", "cp", local, sandboxID+":/tmp/uploaded")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)

			res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/bin/cat", "/tmp/uploaded")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			Expect(string(res.Stdout)).To(Equal(payload),
				"uploaded content did not survive the round-trip")

			out := filepath.Join(suite.RunDir, "lifecycle-os-release.out")
			res, err = yag.Run(ctx, "sandbox", "cp",
				sandboxID+":/etc/os-release", out)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			contents, err := os.ReadFile(out)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.ToLower(string(contents))).To(ContainSubstring("ubuntu"),
				"downloaded /etc/os-release should identify Ubuntu base rootfs")
		})

	It("deletes the sandbox and reaches PHASE_DELETED", func(specCtx SpecContext) {
		ctx, cancel := context.WithTimeout(specCtx, deleteBudget)
		defer cancel()

		Expect(sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)).
			To(Succeed())
	})
})

// Spec-level timeouts. VM boot + agent init dominates; the long
// budget covers a cold rootfs page-in on first boot. Subsequent
// per-spec actions are quick but still need slack for vsock setup.
const (
	scenarioBudget = 90 * time.Second
	execBudget     = 30 * time.Second
	deleteBudget   = 45 * time.Second
)

// requireRoot skips the current spec when not running under sudo —
// jailer + netlink require CAP_NET_ADMIN, so every VM-launching
// scenario shares this gate.
func requireRoot() {
	GinkgoHelper()
	if os.Geteuid() != 0 {
		Skip("requires root for jailer + netlink TAP setup; run `sudo -E make test`")
	}
}
