#!/usr/bin/env bash
# Regenerate the firecracker API client from the upstream swagger
# spec at the version pinned by ${repo_root}/firecracker-version.
#
# The spec is fetched directly from the firecracker GitHub repo over
# HTTPS — no git clone required. The single-file spec has no
# relative refs, so a curl is sufficient and faster/lighter than
# cloning a multi-MB Rust repo just for one YAML.
#
# Usage:
#   ./hack/generate.sh
#
# Requires the swagger CLI. Install with:
#   go install github.com/go-swagger/go-swagger/cmd/swagger@latest

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd)"
client_root="$(cd "${here}/.." > /dev/null && pwd)"
repo_root="$(cd "${client_root}/.." > /dev/null && pwd)"
version_file="${repo_root}/firecracker-version"

if [[ ! -f "${version_file}" ]]; then
    echo "missing ${version_file}: this file pins the firecracker release the client targets" >&2
    exit 1
fi
version="$(tr -d '[:space:]' < "${version_file}")"
if [[ -z "${version}" ]]; then
    echo "${version_file} is empty" >&2
    exit 1
fi

if ! command -v swagger >/dev/null 2>&1; then
    echo "swagger CLI not found in PATH" >&2
    echo "install with: go install github.com/go-swagger/go-swagger/cmd/swagger@latest" >&2
    exit 1
fi

spec_url="https://raw.githubusercontent.com/firecracker-microvm/firecracker/${version}/src/firecracker/swagger/firecracker.yaml"

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

spec="${workdir}/firecracker.yaml"
echo "==> fetching swagger spec for firecracker ${version}"
echo "    ${spec_url}"
curl -fsSL -o "${spec}" "${spec_url}"

# Wipe stale generated code before regenerating so types removed
# upstream don't linger in the working tree.
echo "==> cleaning stale generated client + models"
rm -rf "${client_root}/client" "${client_root}/models"

echo "==> running swagger generate client"
swagger generate client \
    --spec="${spec}" \
    --target="${client_root}" \
    --skip-validation

echo
echo "done. regenerated ${client_root}/{client,models} from firecracker ${version}"
echo "      verify with: cd ${client_root} && go build ./..."
