// Package create implements `yag sandbox create`. The command builds a
// CreateSandboxRequest from a small set of flags and forwards it to the
// SandboxService on the api-server. The server applies its own validation
// (buf.validate) — this command stays thin and only normalises user input
// (UUID generation, memory unit parsing).
package create

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	flagNamespace = "namespace"
	flagVCPU      = "vcpu"
	flagMemory    = "memory"
	flagDisk      = "disk"
	flagSource    = "source"

	flagAllowIP     = "allow-ip"
	flagAllowCIDR   = "allow-cidr"
	flagAllowDomain = "allow-domain"
	flagDenyIP      = "deny-ip"
	flagDenyCIDR    = "deny-cidr"
	flagDenyDomain  = "deny-domain"

	defaultNamespace = "default"
	defaultVCPU      = int32(1)
	// 128 MiB matches the proto's lower bound for memory_mib — the
	// smallest sandbox the api-server will accept.
	defaultMemory = "128MiB"

	sourceTypeSnapshot = "snapshot"
	sourceTypeImage    = "image"
)

// sizePattern accepts "<integer><unit>" where unit is MiB or GiB. The
// integer is captured in group 1 and the unit in group 2; whitespace is
// not permitted. Shared by --memory and --disk.
var sizePattern = regexp.MustCompile(`^(\d+)(MiB|GiB)$`)

// New constructs the `sandbox create` command. ctx is threaded through so
// tests can swap the gRPC clients and I/O streams via
// cli/testing.NewContext.
func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create [id]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Long: `Create a sandbox in the Yaghan platform.

A sandbox is a lightweight microVM scheduled on one of the cluster's
worker nodes. The api-server returns as soon as the request is
persisted; the sandbox then transitions through PENDING into RUNNING
as the target node boots it. Use 'yag sandbox get' to observe the
current phase.

The id argument is optional — when omitted, a fresh UUID is generated
client-side and printed back.`,
		Example: `  # Create a sandbox with a generated id, default namespace and minimum
  # resources (1 vCPU, 128 MiB).
  yag sandbox create

  # Create a sandbox with an explicit id and larger resources.
  yag sandbox create my-sandbox --vcpu 2 --memory 1GiB

  # Create a sandbox in a non-default namespace.
  yag sandbox create --namespace tenant-a --memory 512MiB

  # Restore a sandbox from a snapshot (the snapshot must live in the same namespace).
  yag sandbox create --source snapshot:my-snapshot-id

  # Restrict egress to a small allow-list (IP, CIDR, and a wildcard domain).
  yag sandbox create --allow-ip 8.8.8.8 --allow-cidr 10.0.0.0/24 --allow-domain "*.internal.corp"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	cmd.Flags().StringP(flagNamespace, "n", defaultNamespace,
		"Namespace the sandbox belongs to.")
	cmd.Flags().Int32P(flagVCPU, "v", defaultVCPU,
		"Number of virtual CPUs assigned to the sandbox.")
	cmd.Flags().StringP(flagMemory, "m", defaultMemory,
		"Memory size as <integer><unit>, where unit is MiB or GiB (e.g. 512MiB, 2GiB).")
	cmd.Flags().StringP(flagDisk, "d", "",
		"Root disk size as <integer><unit>, where unit is MiB or GiB (e.g. 4GiB, 8192MiB). "+
			"Unset means the daemon's configured default.")
	cmd.Flags().StringP(flagSource, "s", "",
		"Seed the sandbox from a referenced artifact. Format: <type>:<id> where type is one of (snapshot, image). "+
			"Example: --source snapshot:abc123. Image sources are forward-compatible — the api-server returns "+
			"Unimplemented today. Empty (default) creates a fresh sandbox.")

	cmd.Flags().StringSlice(flagAllowIP, nil,
		"IP address allowed for sandbox egress. May be repeated. Mutually exclusive with --deny-* flags.")
	cmd.Flags().StringSlice(flagAllowCIDR, nil,
		"CIDR block allowed for sandbox egress. May be repeated. Mutually exclusive with --deny-* flags.")
	cmd.Flags().StringSlice(flagAllowDomain, nil,
		"Domain name allowed for sandbox egress (literal hostname or wildcard like *.example.com). "+
			"May be repeated. Mutually exclusive with --deny-* flags.")
	cmd.Flags().StringSlice(flagDenyIP, nil,
		"IP address denied for sandbox egress. May be repeated. Mutually exclusive with --allow-* flags.")
	cmd.Flags().StringSlice(flagDenyCIDR, nil,
		"CIDR block denied for sandbox egress. May be repeated. Mutually exclusive with --allow-* flags.")
	cmd.Flags().StringSlice(flagDenyDomain, nil,
		"Domain name denied for sandbox egress (literal hostname or wildcard like *.example.com). "+
			"May be repeated. Mutually exclusive with --allow-* flags.")

	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := resolveID(args)

	namespace, err := cmd.Flags().GetString(flagNamespace)
	if err != nil {
		return err
	}
	vcpu, err := cmd.Flags().GetInt32(flagVCPU)
	if err != nil {
		return err
	}
	memoryStr, err := cmd.Flags().GetString(flagMemory)
	if err != nil {
		return err
	}
	memoryMiB, err := parseSize(flagMemory, memoryStr)
	if err != nil {
		return err
	}
	diskStr, err := cmd.Flags().GetString(flagDisk)
	if err != nil {
		return err
	}
	var diskMiB uint64
	if diskStr != "" {
		if diskMiB, err = parseSize(flagDisk, diskStr); err != nil {
			return err
		}
	}
	sourceStr, err := cmd.Flags().GetString(flagSource)
	if err != nil {
		return err
	}
	source, err := parseSource(sourceStr)
	if err != nil {
		return err
	}
	egressPolicy, err := parseEgressPolicy(cmd)
	if err != nil {
		return err
	}

	// Snapshot-sourced sandboxes inherit vCPU, memory AND disk size
	// from the snapshot: Firecracker bakes vCPU/memory into the
	// snapshot state and the disk geometry is baked into the drive
	// file the snapshot references. Refuse the request early when
	// the user explicitly passed -v / -m / -d alongside --source
	// snapshot, and otherwise omit Resources from the request so the
	// api-server stamps the snapshot's values onto the sandbox.
	vcpuSet := cmd.Flags().Changed(flagVCPU)
	memorySet := cmd.Flags().Changed(flagMemory)
	diskSet := cmd.Flags().Changed(flagDisk)
	isSnapshotSource := source.GetSnapshotId() != ""
	if isSnapshotSource && (vcpuSet || memorySet || diskSet) {
		return fmt.Errorf(
			"--%s/--%s/--%s cannot be set when --%s is a snapshot: the snapshot's vCPU, memory and disk are immutable on restore",
			flagVCPU, flagMemory, flagDisk, flagSource)
	}

	resources := &controlplanev1alpha1.Resources{
		VcpuCount: uint32(vcpu),
		MemoryMib: memoryMiB,
		DiskMib:   diskMiB,
	}
	if isSnapshotSource {
		resources = nil
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Creating sandbox %q in namespace %q (%s%s)...\n",
		id, namespace, resourceStatusFragment(resources, vcpu, memoryMiB, diskMiB), sourceStatusSuffix(sourceStr))

	resp, err := ctx.ClientSet.SandboxService.CreateSandbox(cmd.Context(),
		&controlplanev1alpha1.CreateSandboxRequest{
			Sandbox: &controlplanev1alpha1.Sandbox{
				Metadata: &controlplanev1alpha1.SandboxMeta{
					Id:        id,
					Namespace: namespace,
					Source:    source,
				},
				Resources:    resources,
				EgressPolicy: egressPolicy,
			},
		})
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Sandbox %q created. Run 'yag sandbox get %s' to follow its phase.\n",
		resp.GetSandbox().GetMetadata().GetId(),
		resp.GetSandbox().GetMetadata().GetId())
	return nil
}

// resourceStatusFragment renders the resource portion of the "Creating
// sandbox..." status line. For snapshot-sourced sandboxes (where the
// CLI does not attach Resources to the request) it surfaces the
// inherited intent rather than the flag defaults, so the user sees that
// vCPU/memory/disk will be supplied by the snapshot.
func resourceStatusFragment(resources *controlplanev1alpha1.Resources, vcpu int32, memoryMiB, diskMiB uint64) string {
	if resources == nil {
		return "resources inherited from snapshot"
	}
	base := fmt.Sprintf("vcpu=%d, memory=%dMiB", vcpu, memoryMiB)
	if diskMiB > 0 {
		return base + fmt.Sprintf(", disk=%dMiB", diskMiB)
	}
	return base + ", disk=default"
}

// resolveID returns args[0] when supplied; otherwise generates a UUID.
// Empty-string ids surface as a server-side InvalidArgument from
// buf.validate, which is fine — we keep client-side checks minimal.
func resolveID(args []string) string {
	if len(args) == 1 && args[0] != "" {
		return args[0]
	}
	return uuid.NewString()
}

// parseSource translates the --source flag value into a SandboxSource
// proto. Empty input returns (nil, nil) — no source means "create
// fresh", the legacy behaviour. A non-empty value must be
// "<type>:<id>" where type is one of {snapshot, image}; anything else
// is rejected client-side with a format-pointing message. The
// api-server has the final say on whether the referenced artifact
// resolves (snapshot existence / namespace match, image_id today
// surfaces as Unimplemented).
func parseSource(s string) (*controlplanev1alpha1.SandboxSource, error) {
	if s == "" {
		return nil, nil
	}
	kind, id, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf(
			"invalid --%s %q: expected format <type>:<id>, e.g. snapshot:abc123",
			flagSource, s)
	}
	if kind == "" || id == "" {
		return nil, fmt.Errorf(
			"invalid --%s %q: both type and id are required, e.g. snapshot:abc123",
			flagSource, s)
	}
	switch kind {
	case sourceTypeSnapshot:
		return &controlplanev1alpha1.SandboxSource{
			Reference: &controlplanev1alpha1.SandboxSource_SnapshotId{SnapshotId: id},
		}, nil
	case sourceTypeImage:
		return &controlplanev1alpha1.SandboxSource{
			Reference: &controlplanev1alpha1.SandboxSource_ImageId{ImageId: id},
		}, nil
	default:
		return nil, fmt.Errorf(
			"invalid --%s %q: unknown type %q, expected one of (%s, %s)",
			flagSource, s, kind, sourceTypeSnapshot, sourceTypeImage)
	}
}

// sourceStatusSuffix returns the " source=<raw>" fragment for the
// "Creating sandbox..." status line, or "" when no source was
// supplied. Surfaces what the user typed (not the parsed proto) so the
// echo matches their input verbatim.
func sourceStatusSuffix(raw string) string {
	if raw == "" {
		return ""
	}
	return ", source=" + raw
}

// parseEgressPolicy folds the six --allow-*/--deny-* flag groups into
// an EgressPolicy. Returns (nil, nil) when no egress flag was supplied
// — the proto's default of an unconstrained egress is conveyed by an
// absent field. Returns an error when both an allow-* and a deny-*
// flag are set; the proto's oneof admits only one arm and surfacing
// the conflict client-side gives a clearer message than the
// api-server's generic InvalidArgument.
//
// No syntax checks on individual values — buf.validate at the
// api-server is the source of truth.
func parseEgressPolicy(cmd *cobra.Command) (*controlplanev1alpha1.EgressPolicy, error) {
	slice := func(name string) ([]string, error) { return cmd.Flags().GetStringSlice(name) }

	allowIP, err := slice(flagAllowIP)
	if err != nil {
		return nil, err
	}
	allowCIDR, err := slice(flagAllowCIDR)
	if err != nil {
		return nil, err
	}
	allowDomain, err := slice(flagAllowDomain)
	if err != nil {
		return nil, err
	}
	denyIP, err := slice(flagDenyIP)
	if err != nil {
		return nil, err
	}
	denyCIDR, err := slice(flagDenyCIDR)
	if err != nil {
		return nil, err
	}
	denyDomain, err := slice(flagDenyDomain)
	if err != nil {
		return nil, err
	}

	hasAllow := len(allowIP) > 0 || len(allowCIDR) > 0 || len(allowDomain) > 0
	hasDeny := len(denyIP) > 0 || len(denyCIDR) > 0 || len(denyDomain) > 0

	switch {
	case !hasAllow && !hasDeny:
		return nil, nil
	case hasAllow && hasDeny:
		return nil, fmt.Errorf("--allow-* and --deny-* flags are mutually exclusive")
	case hasAllow:
		return &controlplanev1alpha1.EgressPolicy{
			Rules: &controlplanev1alpha1.EgressPolicy_Allow{
				Allow: &controlplanev1alpha1.EgressTargets{
					IpAddresses: allowIP,
					CidrBlocks:  allowCIDR,
					DomainNames: allowDomain,
				},
			},
		}, nil
	default:
		return &controlplanev1alpha1.EgressPolicy{
			Rules: &controlplanev1alpha1.EgressPolicy_Deny{
				Deny: &controlplanev1alpha1.EgressTargets{
					IpAddresses: denyIP,
					CidrBlocks:  denyCIDR,
					DomainNames: denyDomain,
				},
			},
		}, nil
	}
}

// parseSize converts "<integer><unit>" (unit ∈ {MiB, GiB}) into MiB,
// the unit the Sandbox proto's size fields all use. label appears in
// error messages so the caller can keep flag-specific phrasing
// ("invalid memory ...", "invalid disk ...").
func parseSize(label, s string) (uint64, error) {
	matches := sizePattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid %s %q: expected format <integer><unit>, unit one of MiB or GiB", label, s)
	}
	n, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", label, s, err)
	}
	switch matches[2] {
	case "MiB":
		return n, nil
	case "GiB":
		return n * 1024, nil
	default:
		// Unreachable: the regex already constrains the unit.
		return 0, fmt.Errorf("invalid %s %q: unknown unit %q", label, s, matches[2])
	}
}
