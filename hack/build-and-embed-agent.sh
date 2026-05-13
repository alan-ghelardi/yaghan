#!/usr/bin/env bash
# Build the agent as a static Linux/amd64 binary and install it
# inside the provided ext4 rootfs at the path the daemon's init=
# boot arg expects.
#
# Runs as your normal user — the `go build` step deliberately stays
# unprivileged so the Go cache lands in your home directory, not in
# root's. The few steps that need CAP_SYS_ADMIN (loop mount + writes
# inside the mount) escalate via `need_root` (defined in
# hack/helpers.sh).
#
# Usage:
#   ./hack/build-and-embed-agent.sh [--rootfs PATH] [--target PATH]

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
repo_root="$(cd "${here}/.." > /dev/null && pwd)"
agent_dir="${repo_root}/agent"

# shellcheck disable=SC1091
. "${here}/helpers.sh"

go="/usr/local/go/bin/go"
rootfs="${repo_root}/assets/rootfs.ext4"
# Root-level path dodges usr-merge symlinks (/sbin → usr/sbin) that
# can confuse the kernel's init= lookup on some distros.
target="/init"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --rootfs)
            rootfs="$2"
            shift 2
            ;;
        --target)
            target="$2"
            shift 2
            ;;
        -h|--help)
            sed -n '2,13p' "$0"
            exit 0
            ;;
        *)
            echo "unknown flag: $1" >&2
            exit 2
            ;;
    esac
done

if [[ ! -f "${rootfs}" ]]; then
    echo "rootfs image not found: ${rootfs}" >&2
    exit 1
fi

workdir="$(mktemp -d)"
mnt="${workdir}/mnt"
bin="${workdir}/agent"
mkdir -p "${mnt}"

cleanup() {
    if mountpoint -q "${mnt}"; then
        # Desktop automounters (udisks2, systemd-tmpfiles) sometimes
        # hold a just-created mount for a moment and produce
        # "target is busy". Retry briefly, then fall back to a lazy
        # unmount that detaches immediately and lets the kernel
        # finish when the stragglers let go.
        for _ in 1 2 3 4 5; do
            if need_root umount "${mnt}" 2>/dev/null; then
                break
            fi
            sleep 0.5
        done
        if mountpoint -q "${mnt}"; then
            need_root umount -l "${mnt}" 2>/dev/null || true
        fi
    fi
    rm -rf "${workdir}" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> building agent (static, linux/amd64)"
(
    cd "${agent_dir}"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        $go build -trimpath -ldflags="-s -w" -o "${bin}" ./cmd
)

echo "==> mounting ${rootfs} at ${mnt}"
need_root mount -o loop,rw "${rootfs}" "${mnt}"

echo "==> installing agent to ${target} inside rootfs"
need_root install -D -m 0755 "${bin}" "${mnt}${target}"

echo "==> syncing"
sync

echo "done: ${target} (size=$(stat -c %s "${mnt}${target}") bytes) in ${rootfs}"
