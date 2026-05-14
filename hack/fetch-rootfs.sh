#!/usr/bin/env bash
# Build a minimal Ubuntu-based ext4 rootfs for the microVM agent.
# Downloads Canonical's Ubuntu Base tarball (the container/microVM
# equivalent of Alpine's minirootfs), writes a blank ext4 image,
# extracts the tarball into it, and applies a few sandbox-friendly
# tweaks. Produces assets/rootfs.ext4 by default.
#
# Ubuntu 24.04 LTS (Noble Numbat) is the default: glibc, GNU coreutils,
# bash, apt — familiar surface for developers debugging a sandbox,
# at the cost of ~80 MB on-disk vs Alpine's ~5 MB. Boot-time impact
# under Firecracker is negligible.
#
# Runs as your normal user; the few steps that require CAP_SYS_ADMIN
# (loop mounting the image and writing inside the mount) are
# wrapped in `need_root` (defined in hack/helpers.sh) so they only
# escalate via sudo. You will be prompted for the sudo password once
# unless your timestamp is still cached.
#
#   ./hack/fetch-rootfs.sh [--version 24.04.4] [--size 256M] [--out PATH]
#
# The downloaded tarball is cached in assets/ so repeat runs are offline.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
repo_root="$(cd "${here}/.." > /dev/null && pwd)"
assets_dir="${repo_root}/assets"

# shellcheck disable=SC1091
. "${here}/helpers.sh"

version="24.04.4"
# Ubuntu uses Debian-style arch names ("amd64", not "x86_64"). Bump
# the default here and in the help text if you start serving arm64
# guests too — Ubuntu Base publishes "arm64" tarballs alongside amd64.
arch="amd64"
size="256M"
out_path="${assets_dir}/rootfs.ext4"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) version="$2"; shift 2;;
        --arch)    arch="$2";    shift 2;;
        --size)    size="$2";    shift 2;;
        --out)     out_path="$2"; shift 2;;
        -h|--help) sed -n '2,20p' "$0"; exit 0;;
        *) echo "unknown flag: $1" >&2; exit 2;;
    esac
done

# Canonical's cdimage layout: the directory is keyed by major.minor
# (e.g. "24.04") and the filename carries the full point release
# (e.g. "24.04.2"). `${version%.*}` strips the trailing patch
# component so both work from a single --version flag.
major_minor="${version%.*}"
tarball_url="https://cdimage.ubuntu.com/ubuntu-base/releases/${major_minor}/release/ubuntu-base-${version}-base-${arch}.tar.gz"
tarball_cached="${assets_dir}/ubuntu-base-${version}-base-${arch}.tar.gz"

workdir="$(mktemp -d)"
mnt="${workdir}/mnt"
mkdir -p "${mnt}"

cleanup() {
    if mountpoint -q "${mnt}"; then
        for _ in 1 2 3 4 5; do
            need_root umount "${mnt}" 2>/dev/null && break
            sleep 0.5
        done
        if mountpoint -q "${mnt}"; then
            need_root umount -l "${mnt}" 2>/dev/null || true
        fi
    fi
    rm -rf "${workdir}" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "${assets_dir}"

if [[ ! -f "${tarball_cached}" ]]; then
    echo "==> downloading ${tarball_url}"
    curl -fsSL -o "${tarball_cached}.tmp" "${tarball_url}"
    mv "${tarball_cached}.tmp" "${tarball_cached}"
else
    echo "==> using cached ${tarball_cached}"
fi

echo "==> creating blank ${size} ext4 image at ${out_path}"
rm -f "${out_path}"
truncate -s "${size}" "${out_path}"
mkfs.ext4 -F -q -L rootfs "${out_path}"

echo "==> extracting Ubuntu Base into the image"
need_root mount -o loop,rw "${out_path}" "${mnt}"
need_root tar -xzf "${tarball_cached}" -C "${mnt}"

# Post-install tweaks. The agent's PID-1 startup handles /tmp etc.,
# but we set them up in the image too so everything works before the
# agent has had a chance to run its ensureBaseDirs pass.
need_root chmod 1777 "${mnt}/tmp"
# Ubuntu Base ships /etc/resolv.conf as a symlink to
# /run/systemd/resolve/stub-resolv.conf (a path that doesn't exist
# in our sandboxes — we don't run systemd). If left in place the
# agent's writeResolvConf would follow the symlink into a missing
# directory and silently fail, leaving the guest with no DNS.
# Unlinking it here lets the agent create a regular file at boot.
need_root rm -f "${mnt}/etc/resolv.conf"
# Empty root password so ssh-over-network (if ever enabled) and any
# diagnostic login path doesn't block on authentication. The sandbox
# is ephemeral — no secrets live here.
need_root sed -i 's#^root:[^:]*:#root::#' "${mnt}/etc/shadow" 2>/dev/null || true

echo "==> syncing"
sync

echo "done: ${out_path} ($(du -h "${out_path}" | cut -f1)) from ubuntu-base-${version}"
echo "next: sudo ./hack/build-and-embed-agent.sh --rootfs ${out_path}"
