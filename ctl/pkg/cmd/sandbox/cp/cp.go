// Package cp implements `sindri sandbox cp` — copy a single file
// between the local filesystem and a sandbox. It wraps the daemon's
// UploadFile / DownloadFile RPCs and applies docker-shaped argument
// parsing: arguments of the form `<sandbox-id>:<path>` reference a
// sandbox, anything else is a local path.
package cp

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/cli"
)

// localFileMode is the mode used when writing a downloaded file. The
// upload path doesn't carry mode information through the proto, so
// downloads can't recover the source's mode either. 0o644 matches the
// Go stdlib default for os.WriteFile.
const localFileMode = 0o644

// reference is a parsed cp argument. Sandbox-side iff Sandbox != "".
type reference struct {
	Sandbox string // empty for local args
	Path    string // always non-empty after parseReference
}

// IsSandbox reports whether the reference points at a sandbox path.
func (r reference) IsSandbox() bool { return r.Sandbox != "" }

// parseReference splits an argument into a sandbox reference or a
// local path, applying docker-style disambiguation:
//
//  1. Empty argument is rejected.
//  2. A leading "/", "./", "../", or "~/" forces local interpretation
//     (so a literal local file with a colon in its name like
//     "./foo:bar" stays local).
//  3. Otherwise the first ":" splits sandbox-id from path; both
//     halves must be non-empty.
//  4. With no colon the argument is a local path.
func parseReference(arg string) (reference, error) {
	if arg == "" {
		return reference{}, errors.New("argument cannot be empty")
	}
	if hasLocalPrefix(arg) {
		return reference{Path: arg}, nil
	}
	idx := strings.IndexByte(arg, ':')
	if idx < 0 {
		return reference{Path: arg}, nil
	}
	sandbox, p := arg[:idx], arg[idx+1:]
	if sandbox == "" {
		return reference{}, fmt.Errorf("invalid argument %q: empty sandbox id before ':'", arg)
	}
	if p == "" {
		return reference{}, fmt.Errorf("invalid argument %q: empty path after ':'", arg)
	}
	return reference{Sandbox: sandbox, Path: p}, nil
}

// hasLocalPrefix reports whether arg unambiguously denotes a local
// path because of its leading characters.
func hasLocalPrefix(arg string) bool {
	switch {
	case strings.HasPrefix(arg, "/"),
		strings.HasPrefix(arg, "./"),
		strings.HasPrefix(arg, "../"),
		strings.HasPrefix(arg, "~/"):
		return true
	}
	return false
}

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cp <source> <target>",
		Aliases: []string{"copy"},
		Short:   "Copy a file to or from a sandbox",
		Long: `Copy a single file between the local filesystem and a sandbox.

Each argument is either a local path or a sandbox reference of the form
<sandbox-id>:<path>. Exactly one of source and target must reference a
sandbox — cross-sandbox copies and pure-local copies are rejected.

When the destination ends with '/' (or, for local destinations, refers
to an existing directory) the source's basename is appended
automatically, matching POSIX cp.`,
		Example: `  # Download a file from the sandbox into the current directory.
  sindri sandbox cp my-sandbox:/var/log/app.log .

  # Upload a local file into the sandbox.
  sindri sandbox cp ./build.tar.gz my-sandbox:/srv/build.tar.gz

  # Upload into a remote directory (basename appended).
  sindri sandbox cp ./report.txt my-sandbox:/srv/reports/`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}
	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	src, err := parseReference(args[0])
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	dst, err := parseReference(args[1])
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}

	switch {
	case !src.IsSandbox() && !dst.IsSandbox():
		return errors.New(
			"neither argument references a sandbox; nothing to copy. " +
				"Prefix one path with `<sandbox-id>:` to indicate the sandbox side")
	case src.IsSandbox() && dst.IsSandbox():
		return errors.New(
			"cross-sandbox copy is not supported; only one of source / target may carry a sandbox id")
	case !src.IsSandbox() && dst.IsSandbox():
		return runUpload(ctx, cmd, src, dst)
	default:
		return runDownload(ctx, cmd, src, dst)
	}
}

// runUpload reads the local source file and forwards it as an
// UploadFile RPC. Trailing-slash destinations are expanded to
// `<dest>/<basename(source)>` so `cp ./x.txt sb:/srv/` lands at
// `/srv/x.txt` rather than failing because `/srv/` doesn't exist as a
// regular file.
func runUpload(ctx *cli.Context, cmd *cobra.Command, src, dst reference) error {
	content, err := os.ReadFile(src.Path) // #nosec G304 -- caller-supplied path is the feature.
	if err != nil {
		return fmt.Errorf("read local source: %w", err)
	}

	dest := dst.Path
	if endsWithSlash(dest) {
		dest = path.Join(dest, filepath.Base(src.Path))
	}

	if _, err := ctx.ClientSet.DaemonService.UploadFile(cmd.Context(),
		&dataplanev1alpha1.UploadFileRequest{
			SandboxId: dst.Sandbox,
			Source:    content,
			Dest:      dest,
		}); err != nil {
		return fmt.Errorf("upload file: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"copied %s → %s:%s (%d bytes)\n",
		src.Path, dst.Sandbox, dest, len(content))
	return nil
}

// runDownload fetches the remote file via DownloadFile and writes it
// to the local target. Trailing-slash and existing-directory targets
// are expanded to `<target>/<basename(source)>`.
func runDownload(ctx *cli.Context, cmd *cobra.Command, src, dst reference) error {
	resp, err := ctx.ClientSet.DaemonService.DownloadFile(cmd.Context(),
		&dataplanev1alpha1.DownloadFileRequest{
			SandboxId: src.Sandbox,
			Source:    src.Path,
		})
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}

	target := localTarget(dst.Path, src.Path)
	if err := os.WriteFile(target, resp.GetFileContent(), localFileMode); err != nil {
		return fmt.Errorf("write local target: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"copied %s:%s → %s (%d bytes)\n",
		src.Sandbox, src.Path, target, len(resp.GetFileContent()))
	return nil
}

// localTarget resolves the actual local file path, applying the two
// "directory destination" rules: explicit trailing slash always wins,
// and an existing directory triggers basename-append even without a
// trailing slash (matching POSIX cp).
func localTarget(target, sandboxSource string) string {
	if endsWithSlash(target) {
		return filepath.Join(target, path.Base(sandboxSource))
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return filepath.Join(target, path.Base(sandboxSource))
	}
	return target
}

// endsWithSlash reports whether the user explicitly asked for a
// directory destination via a trailing path separator. We check both
// "/" (sandbox/Linux convention) and the OS-specific separator so the
// helper composes with both sides.
func endsWithSlash(p string) bool {
	if p == "" {
		return false
	}
	last := p[len(p)-1]
	return last == '/' || last == filepath.Separator
}
