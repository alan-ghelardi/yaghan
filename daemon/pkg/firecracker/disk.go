package firecracker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// resizeRootfs grows the ext4 image at path to sizeMiB. The operation
// is two steps:
//
//  1. `truncate -s <size>M path` extends the regular file to the new
//     size. ext4 sees no change until step 2.
//  2. `resize2fs -f path` grows the ext4 superblock + group descriptors
//     to cover the new range. The new blocks are added to the free list
//     but never written, so the on-disk file stays sparse — the host
//     pays nothing for the new space until the guest writes to it.
//
// Shrinks are refused: we have no use case for them, the destination
// would risk truncating live ext4 metadata, and resize2fs's shrink
// path requires an offline fsck pass we don't run. A zero or negative
// sizeMiB is a no-op (caller did not request a resize).
//
// The path must point at an unmounted ext4 image. The
// CreateMicroVM staging order guarantees this — assets are staged
// before firecracker is exec'd, so the per-VM rootfs file has no
// open handles when we resize it.
func resizeRootfs(path string, sizeMiB int64) error {
	if sizeMiB <= 0 {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	target := sizeMiB * 1024 * 1024
	if fi.Size() > target {
		return fmt.Errorf("refusing to shrink %s from %d MiB to %d MiB",
			path, fi.Size()/(1024*1024), sizeMiB)
	}
	if fi.Size() == target {
		return nil
	}
	// #nosec G204 -- path is the daemon-derived per-VM chroot rootfs
	// location (jailRoot + chroot-relative drive path), never user input.
	// exec.Command does not invoke a shell.
	if out, err := exec.Command("truncate", "-s", fmt.Sprintf("%dM", sizeMiB), path).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate %s to %d MiB: %w (output: %s)",
			path, sizeMiB, err, strings.TrimSpace(string(out)))
	}
	// `-f` lets resize2fs proceed without a prior e2fsck pass. The image
	// has just been copied from a known-good base; running e2fsck on
	// every provision is the price we'd otherwise pay for cosmetic
	// reassurance.
	// #nosec G204 -- see comment above on path provenance.
	if out, err := exec.Command("resize2fs", "-f", path).CombinedOutput(); err != nil {
		return fmt.Errorf("resize2fs %s: %w (output: %s)",
			path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resizeRootfsForInput finds the rootfs drive in input.Config.Drives
// and grows its on-disk file inside jailRoot to input.RootfsDiskMiB.
// No-op when RootfsDiskMiB is zero or no drive is flagged as root.
//
// Drives.PathOnHost is chroot-relative (jailer's POV); we join it
// with jailRoot to get the host-side path of the just-staged file.
func resizeRootfsForInput(jailRoot string, input *CreateMicroVMInput) error {
	if input.RootfsDiskMiB <= 0 || input.Config == nil {
		return nil
	}
	for _, d := range input.Config.Drives {
		if d == nil || d.IsRootDevice == nil || !*d.IsRootDevice {
			continue
		}
		if d.PathOnHost == "" {
			continue
		}
		host := jailRoot + "/" + strings.TrimPrefix(d.PathOnHost, "/")
		return resizeRootfs(host, input.RootfsDiskMiB)
	}
	return nil
}

// digHoles reclaims contiguous zero ranges in the file at path back
// to filesystem holes (`fallocate --dig-holes`). Required after a
// non-sparse copy of a sparse source (such as our base rootfs.ext4)
// so the destination doesn't permanently occupy the source's
// apparent size.
//
// Best-effort: a fallocate failure is logged via the returned error
// but does not abort caller's pipeline at the syscall level — the
// data is correct either way; the cost is host disk space. The
// caller chooses how strict to be.
//
// Requires util-linux >= 2.25 (Ubuntu 14.04+, every modern host).
func digHoles(path string) error {
	// #nosec G204 -- path is a daemon-staged file inside the per-VM chroot,
	// never user input. exec.Command does not invoke a shell.
	if out, err := exec.Command("fallocate", "--dig-holes", path).CombinedOutput(); err != nil {
		return fmt.Errorf("fallocate --dig-holes %s: %w (output: %s)",
			path, err, strings.TrimSpace(string(out)))
	}
	return nil
}
