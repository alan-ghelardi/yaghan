#!/usr/bin/env bash

# Script to run 'go mod tidy' on all Go modules in the repository

set -euo pipefail

cur_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"
cd $cur_dir/..

echo "Finding and tidying all Go modules..."

# Find all go.mod files and run go mod tidy in their directories 
while IFS= read -r -d '' gomod_file; do
    module_dir=$(dirname "$gomod_file")
    echo "Tidying module in: $module_dir"
    (cd "$module_dir" && go mod tidy)
done < <(find . -name go.mod -not -path "./iac/*" -print0)
