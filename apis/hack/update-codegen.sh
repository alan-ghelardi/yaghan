#!/usr/bin/env bash
set -euo pipefail

cur_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"
cd $cur_dir/..

buf format -w

buf generate --clean

./hack/generate-mocks.sh 

./hack/generate-rest-docs.sh
