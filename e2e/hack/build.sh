#!/usr/bin/env bash
#
# Build the api-server, daemon, and yag binaries into e2e/bin/.
#
# Idempotent: skips a binary when it is newer than every .go file in its
# module. Force a rebuild with `FORCE=1 ./hack/build.sh`.

set -o errexit
set -o nounset
set -o pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null && pwd)"
e2e_dir="$(cd "${here}/.." >/dev/null && pwd)"
repo_root="$(cd "${e2e_dir}/.." >/dev/null && pwd)"
bin_dir="${e2e_dir}/bin"

mkdir -p "${bin_dir}"

# Each entry: <output-binary>:<module-dir>:<package-path-relative-to-module>
targets=(
  "api-server:${repo_root}/api-server:./cmd/api-server"
  "daemon:${repo_root}/daemon:./cmd/daemon"
  "yag:${repo_root}/ctl:./cmd"
)

needs_rebuild() {
  local out="$1" module_dir="$2"
  if [[ "${FORCE:-0}" == "1" ]] || [[ ! -f "${out}" ]]; then
    return 0
  fi
  # Rebuild if any .go file under the module is newer than the binary.
  if find "${module_dir}" -name '*.go' -newer "${out}" -print -quit | grep -q .; then
    return 0
  fi
  return 1
}

for entry in "${targets[@]}"; do
  IFS=':' read -r name module_dir pkg <<<"${entry}"
  out="${bin_dir}/${name}"
  if ! needs_rebuild "${out}" "${module_dir}"; then
    echo "[e2e/build] ${name}: up to date"
    continue
  fi
  echo "[e2e/build] building ${name}"
  (cd "${module_dir}" && go build -o "${out}" "${pkg}")
done

echo "[e2e/build] binaries ready under ${bin_dir}"
