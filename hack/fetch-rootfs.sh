#!/usr/bin/env bash
# Build a minimal Alpine-based ext4 rootfs for the microVM agent.
# Downloads Alpine's minirootfs tarball, writes a blank ext4 image,
# extracts the tarball into it, and applies a few sandbox-friendly
# tweaks. Produces assets/rootfs.ext4 by default.
#
# Loop-mounting the image requires root, so invoke with sudo:
#   sudo ./hack/fetch-rootfs.sh [--version 3.21.3] [--size 256M] [--out PATH]
#
# The downloaded tarball is cached in assets/ so repeat runs are offline.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
repo_root="$(cd "${here}/.." > /dev/null && pwd)"
assets_dir="${repo_root}/assets"

version="3.21.3"
arch="x86_64"
size="256M"
out_path="${assets_dir}/rootfs.ext4"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) version="$2"; shift 2;;
        --arch)    arch="$2";    shift 2;;
        --size)    size="$2";    shift 2;;
        --out)     out_path="$2"; shift 2;;
        -h|--help) sed -n '2,11p' "$0"; exit 0;;
        *) echo "unknown flag: $1" >&2; exit 2;;
    esac
done

if [[ "${EUID}" -ne 0 ]]; then
    echo "must run as root (loop mount requires CAP_SYS_ADMIN)" >&2
    exit 1
fi

major_minor="${version%.*}"
tarball_url="https://dl-cdn.alpinelinux.org/alpine/v${major_minor}/releases/${arch}/alpine-minirootfs-${version}-${arch}.tar.gz"
tarball_cached="${assets_dir}/alpine-minirootfs-${version}-${arch}.tar.gz"

workdir="$(mktemp -d)"
mnt="${workdir}/mnt"
mkdir -p "${mnt}"

cleanup() {
    if mountpoint -q "${mnt}"; then
        for _ in 1 2 3 4 5; do
            umount "${mnt}" 2>/dev/null && break
            sleep 0.5
        done
        if mountpoint -q "${mnt}"; then
            umount -l "${mnt}" 2>/dev/null || true
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

echo "==> extracting Alpine minirootfs into the image"
mount -o loop,rw "${out_path}" "${mnt}"
tar -xzf "${tarball_cached}" -C "${mnt}"

# Post-install tweaks. The agent's PID-1 startup handles /tmp etc.,
# but we set them up in the image too so everything works before the
# agent has had a chance to run its ensureBaseDirs pass.
chmod 1777 "${mnt}/tmp"
printf 'nameserver 1.1.1.1\nnameserver 1.0.0.1\n' > "${mnt}/etc/resolv.conf"
# Empty root password so ssh-over-network (if ever enabled) and any
# diagnostic login path doesn't block on authentication. The sandbox
# is ephemeral — no secrets live here.
sed -i 's#^root:[^:]*:#root::#' "${mnt}/etc/shadow" 2>/dev/null || true

echo "==> syncing"
sync

echo "done: ${out_path} ($(du -h "${out_path}" | cut -f1)) from alpine-minirootfs-${version}"
echo "next: sudo ./agent/hack/build-and-embed.sh --rootfs ${out_path}"
