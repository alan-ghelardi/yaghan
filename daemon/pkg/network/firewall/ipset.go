package firewall

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ipsetBinary is the userspace tool used to manage netfilter ip
// sets. The daemon shells out for parity with the iptables path
// (also a shell-out via go-iptables) and to dodge the lack of a
// maintained Go ipset library.
const ipsetBinary = "ipset"

// sandboxIPSetPrefix is the per-sandbox set name prefix. The kernel
// caps set names at 31 bytes; "yaghan-egress-" + a five-digit index
// stays comfortably under the limit even at the allocator's ceiling.
const sandboxIPSetPrefix = "yaghan-egress-"

// sandboxIPSetType is hash:ip with per-entry timeouts. The DNS
// handler pins each entry's lifetime to its answer TTL, so the set
// has no set-level default — entries arrive with explicit timeouts.
const sandboxIPSetType = "hash:ip"

// SandboxIPSetName returns the canonical ipset name for the given
// sandbox index. Stable across daemon restarts.
func SandboxIPSetName(index int) string {
	return fmt.Sprintf("%s%d", sandboxIPSetPrefix, index)
}

// IPSet manages per-sandbox ipsets. Implementations must be safe for
// concurrent use; the default backend shells out to the system
// ipset(8) tool, so concurrency is bounded by the OS process spawn
// cost rather than any in-process lock.
type IPSet interface {
	// EnsureSandboxSet creates the per-sandbox set inside the
	// current network namespace. Idempotent — calling on an
	// already-existing set is not an error.
	EnsureSandboxSet(index int) error

	// AddSandboxEntry adds (or refreshes) an entry in the per-sandbox
	// set inside the namespace named by nsName. ttl is the desired
	// entry lifetime; the kernel rounds down to seconds.
	AddSandboxEntry(nsName string, index int, addr netip.Addr, ttl time.Duration) error
}

// NewIPSet returns the default IPSet backed by the system ipset(8)
// tool. Construction does not touch the kernel — callers that need
// to fail fast on a missing binary should issue a probe operation.
func NewIPSet() IPSet { return &shellIPSet{} }

type shellIPSet struct{}

func (s *shellIPSet) EnsureSandboxSet(index int) error {
	name := SandboxIPSetName(index)
	// `create -exist` makes the call idempotent. `timeout 0` enables
	// per-entry timeouts on a hash:ip set (without this option the
	// kernel rejects `add … timeout N`). 0 here means "no default
	// timeout for entries that omit the flag" — every add still
	// supplies its own ttl.
	return run(ipsetBinary, "create", name, sandboxIPSetType, "timeout", "0", "-exist")
}

func (s *shellIPSet) AddSandboxEntry(nsName string, index int, addr netip.Addr, ttl time.Duration) error {
	if nsName == "" {
		return fmt.Errorf("namespace name must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("ttl must be positive (got %s)", ttl)
	}
	name := SandboxIPSetName(index)
	secs := int(ttl.Seconds())
	if secs < 1 {
		secs = 1 // kernel rejects timeout=0 here (would mean "permanent")
	}
	return run("ip", "netns", "exec", nsName,
		ipsetBinary, "add", name, addr.String(),
		"timeout", strconv.Itoa(secs), "-exist")
}

// run shells out and folds stderr into the error so kernel-side
// failure reasons (set-full, family-mismatch, etc.) reach the
// caller's logs.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)",
			name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
