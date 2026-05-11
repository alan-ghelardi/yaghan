#!/usr/bin/env bash
#
# Brings up the daemon for local development:
#
#   1. preflight: validate that ./assets contains the firecracker /
#      jailer binaries plus the guest kernel and rootfs;
#   2. docker compose up the MinIO stack and wait for it to accept
#      requests;
#   3. create the snapshot bucket (idempotent — head-bucket first);
#   4. exec the daemon with the dev config (under sudo, since jailer
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
compose_file="${dev_dir}/docker-compose.yml"

assets_dir="${ASSETS_DIR:-${repo_root}/assets}"

required_assets=(firecracker jailer vmlinux rootfs.ext4)

# Credentials are matched to MINIO_ROOT_USER/PASSWORD in
# docker-compose.yml. The same env vars are picked up by the daemon's
# AWS SDK credential chain when it boots (start.sh is the daemon's
# parent process, so sudo preserves them — see further down).
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_REGION=us-east-1

s3_endpoint="http://localhost:9000"
bucket_name="microvm-snapshots"

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

# 2. docker compose stack ------------------------------------------------------

cleanup() {
  echo "[daemon/dev] tearing down stack"
  docker compose -f "${compose_file}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "[daemon/dev] starting docker compose stack"
docker compose -f "${compose_file}" up -d

# The minio image has no curl/wget/nc in the base layer, so we poll
# from the host using the AWS CLI we already need for bucket creation.
# list-buckets succeeding is an end-to-end readiness signal.
echo "[daemon/dev] waiting for MinIO to accept requests"
for _ in $(seq 1 60); do
  if aws s3api list-buckets \
      --endpoint-url "${s3_endpoint}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! aws s3api list-buckets \
    --endpoint-url "${s3_endpoint}" >/dev/null 2>&1; then
  echo "[daemon/dev] MinIO did not become ready" >&2
  exit 1
fi

# 3. snapshot bucket -----------------------------------------------------------

if aws s3api head-bucket \
    --endpoint-url "${s3_endpoint}" \
    --bucket "${bucket_name}" >/dev/null 2>&1; then
  echo "[daemon/dev] bucket ${bucket_name} already exists"
else
  echo "[daemon/dev] creating bucket ${bucket_name}"
  aws s3api create-bucket \
      --endpoint-url "${s3_endpoint}" \
      --bucket "${bucket_name}" >/dev/null
fi

# 4. exec daemon (sudo if needed) ---------------------------------------------

# The chroot-base-dir is created lazily under /srv by jailer. Make sure
# /var/lib/nuinfra (where the controller persists its session id) is
# writable by root before the daemon starts.
#
# AWS_* are forwarded so the daemon's SDK credential chain finds the
# MinIO root credentials we set above; ASSETS_DIR is forwarded for the
# preflight-shared assets path.
sudo_args=()
if [[ "${EUID}" -ne 0 ]]; then
  sudo_args=(sudo
    --preserve-env=ASSETS_DIR
    --preserve-env=AWS_ACCESS_KEY_ID
    --preserve-env=AWS_SECRET_ACCESS_KEY
    --preserve-env=AWS_REGION)
fi

go_cmd="$(which go)"

echo "[daemon/dev] starting daemon (config: ${config_file})"
cd "${project_dir}"
"${sudo_args[@]}" $go_cmd run ./cmd/daemon -config "${config_file}"
