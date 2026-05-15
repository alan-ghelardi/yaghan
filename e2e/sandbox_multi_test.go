package e2e_test

import (
	"context"
	"strings"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

var _ = Describe("Two sandboxes coexist on one daemon", Ordered, func() {
	const (
		idP = "multi-p"
		idQ = "multi-q"
	)

	BeforeAll(func() {
		requireRoot()
	})

	It("creates two sandboxes, distinguishes them by exec output, and lists both",
		func(specCtx SpecContext) {
			// Two VM boots back-to-back; the daemon's reconciler is
			// single-threaded so they serialise.
			ctx, cancel := context.WithTimeout(specCtx, 3*scenarioBudget/2)
			defer cancel()

			for _, id := range []string{idP, idQ} {
				res, err := yag.Run(ctx, "sandbox", "create", id)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
				id := id // capture for closure
				DeferCleanup(func(cleanupCtx SpecContext) {
					ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
					defer cancel()
					_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, id)
				})
			}

			for _, id := range []string{idP, idQ} {
				Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, id,
					controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).
					To(Succeed(), "sandbox %s never reached RUNNING", id)
			}

			// Write a sandbox-specific marker in each, then read it
			// back from each. If the daemon ever routed an exec to
			// the wrong namespace the read would surface the other
			// sandbox's marker (or a NotFound). The guest hostname
			// can't be used as a distinguisher — the agent stamps it
			// to the TAP IP, not the sandbox id.
			for _, id := range []string{idP, idQ} {
				res, err := yag.Run(ctx, "sandbox", "exec", id, "--",
					"/bin/sh", "-c", "echo "+id+" > /tmp/marker")
				Expect(err).NotTo(HaveOccurred())
				Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
			}
			for _, id := range []string{idP, idQ} {
				res, err := yag.Run(ctx, "sandbox", "exec", id, "--",
					"/bin/cat", "/tmp/marker")
				Expect(err).NotTo(HaveOccurred())
				Expect(res.ExitCode).To(Equal(0), "stderr:\n%s", res.Stderr)
				Expect(strings.TrimSpace(string(res.Stdout))).To(Equal(id),
					"sandbox %s read marker %q — exec/cp crossed namespaces",
					id, strings.TrimSpace(string(res.Stdout)))
			}

			// Both sandboxes should show up in a single list scoped
			// to this daemon's node (DynamoDB Local is shared across
			// runs, so scoping by node_id keeps us honest).
			var listResp controlplanev1alpha1.ListSandboxesResponse
			Expect(yag.RunYAML(ctx, &listResp,
				"sandbox", "list",
				"--node-id", suite.DaemonNodeID,
				"--no-pagination")).To(Succeed())
			ids := map[string]bool{}
			for _, s := range listResp.GetSandboxes() {
				ids[s.GetMetadata().GetId()] = true
			}
			Expect(ids).To(HaveKey(idP))
			Expect(ids).To(HaveKey(idQ))
		})
})
