#!/usr/bin/env bash
# Download a Linux kernel image (vmlinux) suitable for firecracker
# microVMs and install it at assets/vmlinux. The default targets the
# kernel firecracker's own CI tests against — it's a known-good
# baseline for local development; production deployments will want
# to swap in a kernel built to their security / feature posture.
#
# Usage:
#   ./hack/fetch-kernel.sh [--version 5.10.225] [--arch x86_64] [--out PATH]
#
# No root required: this script only downloads + copies. The cached
# versioned binary stays under assets/vmlinux-<version> so re-runs
# are offline.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
repo_root="$(cd "${here}/.." > /dev/null && pwd)"
assets_dir="${repo_root}/assets"

# 5.10.225 is the long-LTS kernel firecracker's CI exercises across
# the v1.x series. Stable, mature, widely-used; safe default for
# dev/test.
version="5.10.225"
arch="x86_64"
out_path="${assets_dir}/vmlinux"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) version="$2"; shift 2;;
        --arch)    arch="$2";    shift 2;;
        --out)     out_path="$2"; shift 2;;
        -h|--help) sed -n '2,12p' "$0"; exit 0;;
        *) echo "unknown flag: $1" >&2; exit 2;;
    esac
done

mkdir -p "${assets_dir}"

# Firecracker publishes test kernels under s3 at
# spec.ccfc.min/firecracker-ci/v1.10/<arch>/vmlinux-<version>. The
# "v1.10" segment refers to the CI artifact set, not the firecracker
# release; the set is stable across firecracker v1.x releases.
url="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.10/${arch}/vmlinux-${version}"
cached="${assets_dir}/vmlinux-${version}"

if [[ ! -f "${cached}" ]]; then
    echo "==> downloading ${url}"
    curl -fsSL -o "${cached}.tmp" "${url}"
    mv "${cached}.tmp" "${cached}"
else
    echo "==> using cached ${cached}"
fi

# Copy (not symlink) so the daemon can ingest the file from a
# stable path regardless of which version produced it.
install -m 0644 "${cached}" "${out_path}"

echo "done: ${out_path} ($(du -h "${out_path}" | cut -f1)) from vmlinux-${version}"
