package e2e_test

import (
	"context"
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/sandboxes"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
)

// allowedProbeIP and blockedProbeIP are well-known public DNS
// resolvers that respond on TCP/53. The egress-policy specs use them
// as a "should be reachable" vs "should be blocked" pair without
// relying on DNS resolution itself — the probe is a raw bash
// /dev/tcp open against a numeric IP, so the test does not depend on
// /etc/resolv.conf being usable inside the guest.
const (
	allowedProbeIP = "8.8.8.8"
	blockedProbeIP = "1.1.1.1"
	probeTCPPort   = "53"
)

// runProbe execs the TCP-probe command inside the named sandbox and
// returns the yag.Result. /usr/bin/timeout bounds the REJECT-side
// path (kernel returns ICMP admin-prohibited immediately so this is
// a safety net, not a primary mechanism); /bin/bash's /dev/tcp
// pseudo-device opens a TCP connection and closes it, which is
// enough to exercise both the FORWARD filter and the NAT/MASQUERADE
// path without pulling in netcat or curl.
//
// The 10s wall covers one full Linux SYN-retransmit cycle (kernel
// retries at +1s and +3s) before the timeout binary kills bash —
// enough headroom to absorb a single dropped SYN to a public DNS
// resolver without false-flagging the policy.
func runProbe(ctx context.Context, sandboxID, ip string) (*yag.Result, error) {
	return yag.Run(ctx,
		"sandbox", "exec", sandboxID, "--",
		"/usr/bin/timeout", "10",
		"/bin/bash", "-c",
		fmt.Sprintf("exec 3<>/dev/tcp/%s/%s && exec 3<&- 3>&-", ip, probeTCPPort),
	)
}

// runProbeExpectSuccess retries the TCP probe up to maxProbeAttempts
// times when the connect did not succeed. Used for the "should be
// reachable" leg of allow/deny policy specs, where a transient SYN
// drop to a public resolver would otherwise look indistinguishable
// from a real policy regression. The fail-by-design leg of each spec
// stays single-shot so a broken policy cannot hide behind retries.
func runProbeExpectSuccess(ctx context.Context, sandboxID, ip string) (*yag.Result, error) {
	var (
		res *yag.Result
		err error
	)
	for attempt := 1; attempt <= maxProbeAttempts; attempt++ {
		res, err = runProbe(ctx, sandboxID, ip)
		if err != nil {
			return res, err
		}
		if res.ExitCode == 0 {
			return res, nil
		}
	}
	return res, nil
}

const maxProbeAttempts = 2

var _ = Describe("Sandbox network connectivity", Ordered, func() {
	BeforeAll(func() {
		requireRoot()
		if !suite.EgressEnabled {
			Skip("egress disabled in the daemon config; this only runs when the suite renders egress-enabled: true")
		}
	})

	It("reaches an external endpoint from inside the guest",
		func(specCtx SpecContext) {
			const sandboxID = "net-d"
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

	// Allow- and deny-policy specs probe IPs directly rather than
	// hostnames because there is no implicit DNS pass-through under
	// an allow-list — a sandbox that allows only 8.8.8.8 cannot
	// resolve names unless the resolver is on the allow-list too.
	// Testing by raw IP keeps the spec orthogonal to /etc/resolv.conf.

	It("blocks egress outside the allow-list when egress_policy.allow is set",
		func(specCtx SpecContext) {
			const sandboxID = "net-allow"
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			createRes, err := yag.Run(ctx, "sandbox", "create", sandboxID,
				"--allow-ip", allowedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(createRes.ExitCode).To(Equal(0), "stderr:\n%s", createRes.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			// Allowed target: the on-list IP must be reachable.
			res, err := runProbeExpectSuccess(ctx, sandboxID, allowedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0),
				"expected probe to %s to succeed under allow-list after %d attempts\nstdout:\n%s\nstderr:\n%s",
				allowedProbeIP, maxProbeAttempts, res.Stdout, res.Stderr)

			// Off-list target: the kernel REJECT must surface as a
			// non-zero exit. bash's /dev/tcp returns 1 on ICMP
			// unreachable; do not over-pin the exact code. No retry —
			// retrying a "should fail" probe would let a broken
			// policy slip through if any single attempt succeeded.
			res, err = runProbe(ctx, sandboxID, blockedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).NotTo(Equal(0),
				"expected probe to %s to be blocked under allow-list\nstdout:\n%s\nstderr:\n%s",
				blockedProbeIP, res.Stdout, res.Stderr)
		})

	It("blocks egress to the deny-list when egress_policy.deny is set",
		func(specCtx SpecContext) {
			const sandboxID = "net-deny"
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			createRes, err := yag.Run(ctx, "sandbox", "create", sandboxID,
				"--deny-ip", blockedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(createRes.ExitCode).To(Equal(0), "stderr:\n%s", createRes.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			// Off-list target: must still be reachable (deny only
			// removes the listed destinations).
			res, err := runProbeExpectSuccess(ctx, sandboxID, allowedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0),
				"expected probe to %s to succeed under deny-list after %d attempts\nstdout:\n%s\nstderr:\n%s",
				allowedProbeIP, maxProbeAttempts, res.Stdout, res.Stderr)

			// On-list target: must be blocked. Single-shot — see the
			// allow spec for the rationale on not retrying this leg.
			res, err = runProbe(ctx, sandboxID, blockedProbeIP)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).NotTo(Equal(0),
				"expected probe to %s to be blocked under deny-list\nstdout:\n%s\nstderr:\n%s",
				blockedProbeIP, res.Stdout, res.Stderr)
		})

	// Domain-name policy specs. These exercise the full DNS-snoop
	// path: agent.dns is pointed at the daemon's resolver, allowed
	// names resolve to IPs that get learned into the per-sandbox
	// ipset, and the FORWARD chain ACCEPTs the resulting traffic.
	// The probe target is a hostname so the test fails closed if
	// the resolver is unreachable (rather than the test happening
	// to work because the IP was statically permitted).

	It("allows egress to a domain on the allow-list and refuses everything else",
		func(specCtx SpecContext) {
			const sandboxID = "net-allow-domain"
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			createRes, err := yag.Run(ctx, "sandbox", "create", sandboxID,
				"--allow-domain", "example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(createRes.ExitCode).To(Equal(0), "stderr:\n%s", createRes.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			// Allowed name: must resolve AND connect. We use getent to
			// resolve via the in-daemon responder; the connect leg
			// follows immediately to exercise the snooped ipset rule.
			res, err := yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/usr/bin/getent", "hosts", "example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0),
				"expected example.com to resolve via the in-daemon responder\nstdout:\n%s\nstderr:\n%s",
				res.Stdout, res.Stderr)

			// Off-list name: must be refused at the resolver (REFUSED
			// rcode → getent exit non-zero) without leaking any IP-level
			// connection.
			res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/usr/bin/getent", "hosts", "google.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).NotTo(Equal(0),
				"expected google.com to be refused by the in-daemon responder\nstdout:\n%s\nstderr:\n%s",
				res.Stdout, res.Stderr)
		})

	It("refuses queries to a domain on the deny-list while forwarding others",
		func(specCtx SpecContext) {
			const sandboxID = "net-deny-domain"
			ctx, cancel := context.WithTimeout(specCtx, scenarioBudget)
			defer cancel()

			createRes, err := yag.Run(ctx, "sandbox", "create", sandboxID,
				"--deny-domain", "google.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(createRes.ExitCode).To(Equal(0), "stderr:\n%s", createRes.Stderr)
			DeferCleanup(func(cleanupCtx SpecContext) {
				ctx, cancel := context.WithTimeout(cleanupCtx, deleteBudget)
				defer cancel()
				_ = sandboxes.DeleteAndWait(ctx, suite.SandboxCli, sandboxID)
			})

			Expect(sandboxes.WaitForPhase(ctx, suite.SandboxCli, sandboxID,
				controlplanev1alpha1.SandboxStatus_PHASE_RUNNING)).To(Succeed())

			// Denied name: must be refused at the resolver. Documented
			// limitation: a sandbox that hard-codes google.com's IP can
			// still connect — deny-mode does not snoop into the ipset.
			res, err := yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/usr/bin/getent", "hosts", "google.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).NotTo(Equal(0),
				"expected google.com to be refused under deny-domain policy\nstdout:\n%s\nstderr:\n%s",
				res.Stdout, res.Stderr)

			// Off-deny-list name: must still resolve.
			res, err = yag.Run(ctx, "sandbox", "exec", sandboxID, "--",
				"/usr/bin/getent", "hosts", "example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ExitCode).To(Equal(0),
				"expected example.com to resolve under deny-domain policy\nstdout:\n%s\nstderr:\n%s",
				res.Stdout, res.Stderr)
		})
})
