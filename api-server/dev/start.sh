#!/usr/bin/env bash
#
# Brings up the api-server for local development:
#   1. docker compose up the DynamoDB Local + Redis stack.
#   2. wait for DynamoDB to be reachable.
#   3. create any DynamoDB tables declared under api-server/dynamodb-tables/
#      (idempotent — describe-table first).
#   4. exec the api-server binary with the dev config.
#
# All resources are torn down on exit.

set -o errexit
set -o nounset
set -o pipefail

dev_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null && pwd)"
project_dir="$(cd "${dev_dir}/.." >/dev/null && pwd)"
tables_dir="${project_dir}/dynamodb-tables"
config_file="${dev_dir}/config.yaml"
compose_file="${dev_dir}/docker-compose.yml"

# Dummy AWS credentials are enough for DynamoDB Local; the AWS CLI
# refuses to run without something in the environment.
export AWS_ACCESS_KEY_ID=local
export AWS_SECRET_ACCESS_KEY=local
export AWS_REGION=us-east-1

dynamodb_endpoint="http://localhost:8000"

cleanup() {
  echo "[api-server/dev] tearing down stack"
  docker compose -f "${compose_file}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "[api-server/dev] starting docker compose stack"
docker compose -f "${compose_file}" up -d

# DynamoDB Local has no in-image healthcheck (the upstream image lacks
# curl/wget/nc). Poll with the AWS CLI we use a few lines down for table
# creation — list-tables succeeding is an end-to-end readiness signal.
echo "[api-server/dev] waiting for DynamoDB to accept requests"
for _ in $(seq 1 60); do
  if aws dynamodb list-tables \
      --endpoint-url "${dynamodb_endpoint}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! aws dynamodb list-tables \
    --endpoint-url "${dynamodb_endpoint}" >/dev/null 2>&1; then
  echo "[api-server/dev] dynamodb did not become ready" >&2
  exit 1
fi

echo "[api-server/dev] ensuring DynamoDB tables exist"
shopt -s nullglob
for schema in "${tables_dir}"/*.json; do
  table_name=$(jq -r '.TableName' "${schema}")
  if [[ -z "${table_name}" || "${table_name}" == "null" ]]; then
    echo "[api-server/dev] skipping ${schema}: no TableName field" >&2
    continue
  fi
  if aws dynamodb describe-table \
      --endpoint-url "${dynamodb_endpoint}" \
      --table-name "${table_name}" >/dev/null 2>&1; then
    echo "[api-server/dev]   ${table_name} already exists"
    continue
  fi
  echo "[api-server/dev]   creating ${table_name}"
  aws dynamodb create-table \
      --endpoint-url "${dynamodb_endpoint}" \
      --cli-input-json "file://${schema}" >/dev/null
done

echo "[api-server/dev] starting api-server (config: ${config_file})"
cd "${project_dir}"
exec go run ./cmd/api-server -config "${config_file}"
