#!/usr/bin/env bash
#
# Brings up the daemon for local development:
#
#   1. preflight: validate that ./assets contains the firecracker /
#      jailer binaries plus the guest kernel and rootfs;
#   2. exec the daemon with the dev config (under sudo, since jailer
#      needs CAP_NET_ADMIN and namespace privileges).
#
# The api-server stack is NOT started by this script — bring it up
# separately via api-server/dev/start.sh in another terminal before
# running this one.

set -o errexit
set -o nounset
set -o pipefail

dev_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null && pwd)"
project_dir="$(cd "${dev_dir}/.." >/dev/null && pwd)"
repo_root="$(cd "${project_dir}/.." >/dev/null && pwd)"
config_file="${dev_dir}/config.yaml"

assets_dir="${ASSETS_DIR:-${repo_root}/assets}"

required_assets=(firecracker jailer vmlinux rootfs.ext4)

# 1. preflight ----------------------------------------------------------------

if [[ ! -d "${assets_dir}" ]]; then
  echo "[daemon/dev] assets dir ${assets_dir} does not exist" >&2
  echo "[daemon/dev] populate it with: ${required_assets[*]}" >&2
  exit 1
fi

missing=()
for asset in "${required_assets[@]}"; do
  if [[ ! -e "${assets_dir}/${asset}" ]]; then
    missing+=("${asset}")
  fi
done
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "[daemon/dev] missing assets in ${assets_dir}: ${missing[*]}" >&2
  exit 1
fi

# 2. exec daemon (sudo if needed) --------------------------------------------

# The chroot-base-dir is created lazily under /srv by jailer. Make sure
# /var/lib/nuinfra (where the controller persists its session id) is
# writable by root before the daemon starts.
sudo_args=()
if [[ "${EUID}" -ne 0 ]]; then
  sudo_args=(sudo --preserve-env=ASSETS_DIR)
fi

go_cmd="$(which go)"

echo "[daemon/dev] starting daemon (config: ${config_file})"
cd "${project_dir}"
"${sudo_args[@]}" $go_cmd run ./cmd/daemon -config "${config_file}"
