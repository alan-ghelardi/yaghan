#!/usr/bin/env bash
# One-command setup for local development:
#
#   1. Creates the assets directory (if missing).
#   2. Downloads the firecracker + jailer binaries pinned by
#      ${repo_root}/firecracker-version and installs them under
#      assets/ with their canonical names so the daemon's defaults
#      pick them up.
#   3. Runs hack/fetch-kernel.sh to download a firecracker-tested
#      vmlinux into assets/.
#   4. Runs hack/fetch-rootfs.sh to build the Alpine rootfs.
#   5. Runs hack/build-and-embed-agent.sh to build the agent and
#      embed it into the rootfs as /init.
#
# Future contributors should run this once before hacking on the
# microvm stack. Every step is idempotent — the firecracker tarball,
# the vmlinux binary, and the Alpine minirootfs tarball are all
# cached under assets/ and reused on re-run.
#
# Runs as your normal user. The few steps that require CAP_SYS_ADMIN
# (loop mount + writes inside the mount, inside fetch-rootfs.sh and
# build-and-embed-agent.sh) escalate per-command via `need_root`
# (defined in hack/helpers.sh) which transparently shells out to
# `sudo`. You will be prompted for the sudo password once unless
# your timestamp is still cached.
#
#   ./hack/setup-dev.sh

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
repo_root="$(cd "${here}/.." > /dev/null && pwd)"
assets_dir="${repo_root}/assets"
version_file="${repo_root}/firecracker-version"

# shellcheck disable=SC1091
. "${here}/helpers.sh"

if [[ ! -f "${version_file}" ]]; then
    echo "missing ${version_file}: this file pins the firecracker release the daemon targets" >&2
    exit 1
fi
version="$(tr -d '[:space:]' < "${version_file}")"
if [[ -z "${version}" ]]; then
    echo "${version_file} is empty" >&2
    exit 1
fi

# Legacy migration: earlier versions of this script demanded `sudo`
# at the top level, so a previously-set-up tree has assets/ owned by
# root and our unprivileged writes (tarball download, binary
# install) would fail. Detect and tell the user how to recover.
if [[ -d "${assets_dir}" && ! -w "${assets_dir}" ]]; then
    echo "${assets_dir} exists but is not writable by you (probably from an older sudo run of this script)." >&2
    echo "fix once with:" >&2
    echo "    sudo chown -R \"\$(id -u):\$(id -g)\" \"${assets_dir}\"" >&2
    echo "then re-run this script." >&2
    exit 1
fi

arch="x86_64"
tarball_name="firecracker-${version}-${arch}.tgz"
tarball_url="https://github.com/firecracker-microvm/firecracker/releases/download/${version}/${tarball_name}"
tarball_cached="${assets_dir}/${tarball_name}"

mkdir -p "${assets_dir}"

echo "==> firecracker version: ${version}"

if [[ ! -f "${tarball_cached}" ]]; then
    echo "==> downloading ${tarball_url}"
    curl -fsSL -o "${tarball_cached}.tmp" "${tarball_url}"
    mv "${tarball_cached}.tmp" "${tarball_cached}"
else
    echo "==> using cached ${tarball_cached}"
fi

# The release tarball expands into release-${version}-${arch}/ containing
# firecracker-${version}-${arch} and jailer-${version}-${arch} (plus debug
# binaries we don't need). Install the two binaries under their canonical
# names so the daemon's Firecracker.BinaryName / JailerBinaryName defaults
# pick them up without further configuration.
workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

echo "==> extracting binaries"
tar -xzf "${tarball_cached}" -C "${workdir}"
release_dir="${workdir}/release-${version}-${arch}"
if [[ ! -d "${release_dir}" ]]; then
    # Older release layouts were flat; fall back to the workdir root
    # so this script keeps working if the upstream packaging changes.
    release_dir="${workdir}"
fi

install -m 0755 "${release_dir}/firecracker-${version}-${arch}" "${assets_dir}/firecracker"
install -m 0755 "${release_dir}/jailer-${version}-${arch}"      "${assets_dir}/jailer"

echo "==> downloading kernel image (delegating to hack/fetch-kernel.sh)"
"${here}/fetch-kernel.sh"

echo "==> building rootfs (delegating to hack/fetch-rootfs.sh)"
"${here}/fetch-rootfs.sh"

echo "==> building agent and embedding it into the rootfs (delegating to hack/build-and-embed-agent.sh)"
"${here}/build-and-embed-agent.sh"

echo
echo "done. local development assets ready under ${assets_dir}:"
echo "  firecracker   $(${assets_dir}/firecracker --version 2>&1 | head -n1)"
echo "  jailer        $(${assets_dir}/jailer --version 2>&1 | head -n1)"
echo "  vmlinux       $(du -h "${assets_dir}/vmlinux"      | cut -f1)"
echo "  rootfs.ext4   $(du -h "${assets_dir}/rootfs.ext4"  | cut -f1)"
