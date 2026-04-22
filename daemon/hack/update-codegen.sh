#!/usr/bin/env bash

set -euo pipefail

cur_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"

cd $cur_dir/..

./hack/generate-mocks.sh
