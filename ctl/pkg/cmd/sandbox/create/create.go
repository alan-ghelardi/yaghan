// Package create implements `sindri sandbox create`. The command builds a
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

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/cli"
)

const (
	flagNamespace = "namespace"
	flagVCPU      = "vcpu"
	flagMemory    = "memory"
	flagSource    = "source"

	defaultNamespace = "default"
	defaultVCPU      = int32(1)
	// 128 MiB matches the proto's lower bound for memory_mib — the
	// smallest sandbox the api-server will accept.
	defaultMemory = "128MiB"

	sourceTypeSnapshot = "snapshot"
	sourceTypeImage    = "image"
)

// memoryPattern accepts "<integer><unit>" where unit is MiB or GiB. The
// integer is captured in group 1 and the unit in group 2; whitespace is
// not permitted.
var memoryPattern = regexp.MustCompile(`^(\d+)(MiB|GiB)$`)

// New constructs the `sandbox create` command. ctx is threaded through so
// tests can swap the gRPC clients and I/O streams via
// cli/testing.NewContext.
func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create [id]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Long: `Create a sandbox in the Sindri platform.

A sandbox is a lightweight microVM scheduled on one of the cluster's
worker nodes. The api-server returns as soon as the request is
persisted; the sandbox then transitions through PENDING into RUNNING
as the target node boots it. Use 'sindri sandbox get' to observe the
current phase.

The id argument is optional — when omitted, a fresh UUID is generated
client-side and printed back.`,
		Example: `  # Create a sandbox with a generated id, default namespace and minimum
  # resources (1 vCPU, 128 MiB).
  sindri sandbox create

  # Create a sandbox with an explicit id and larger resources.
  sindri sandbox create my-sandbox --vcpu 2 --memory 1GiB

  # Create a sandbox in a non-default namespace.
  sindri sandbox create --namespace tenant-a --memory 512MiB

  # Restore a sandbox from a snapshot (the snapshot must live in the same namespace).
  sindri sandbox create --source snapshot:my-snapshot-id`,
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
	cmd.Flags().StringP(flagSource, "s", "",
		"Seed the sandbox from a referenced artifact. Format: <type>:<id> where type is one of (snapshot, image). "+
			"Example: --source snapshot:abc123. Image sources are forward-compatible — the api-server returns "+
			"Unimplemented today. Empty (default) creates a fresh sandbox.")

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
	memoryMiB, err := parseMemory(memoryStr)
	if err != nil {
		return err
	}
	sourceStr, err := cmd.Flags().GetString(flagSource)
	if err != nil {
		return err
	}
	source, err := parseSource(sourceStr)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Creating sandbox %q in namespace %q (vcpu=%d, memory=%dMiB%s)...\n",
		id, namespace, vcpu, memoryMiB, sourceStatusSuffix(sourceStr))

	resp, err := ctx.ClientSet.SandboxService.CreateSandbox(cmd.Context(),
		&controlplanev1alpha1.CreateSandboxRequest{
			Sandbox: &controlplanev1alpha1.Sandbox{
				Metadata: &controlplanev1alpha1.SandboxMeta{
					Id:        id,
					Namespace: namespace,
					Source:    source,
				},
				Resources: &controlplanev1alpha1.Resources{
					VcpuCount: uint32(vcpu),
					MemoryMib: memoryMiB,
				},
			},
		})
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Sandbox %q created. Run 'sindri sandbox get %s' to follow its phase.\n",
		resp.GetSandbox().GetMetadata().GetId(),
		resp.GetSandbox().GetMetadata().GetId())
	return nil
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

// parseMemory converts "<integer><unit>" (unit ∈ {MiB, GiB}) into MiB,
// the unit the Sandbox proto expects.
func parseMemory(s string) (uint64, error) {
	matches := memoryPattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid memory %q: expected format <integer><unit>, unit one of MiB or GiB", s)
	}
	n, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q: %w", s, err)
	}
	switch matches[2] {
	case "MiB":
		return n, nil
	case "GiB":
		return n * 1024, nil
	default:
		// Unreachable: the regex already constrains the unit.
		return 0, fmt.Errorf("invalid memory %q: unknown unit %q", s, matches[2])
	}
}
