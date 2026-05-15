#!/usr/bin/env bash

################################################################################
### This script generates mocks from interfaces using mockgen:
### https://github.com/uber-go/mock
################################################################################

set -o errexit
set -o nounset
set -o pipefail

cur_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"
source $cur_dir/../../hack/helpers.sh

# Call the function gen_mocks for each file containing interfaces to be mocked.
gen_mocks gen/yaghan/control_plane/v1alpha1/cluster_grpc.pb.go
gen_mocks gen/yaghan/control_plane/v1alpha1/sandbox_grpc.pb.go
gen_mocks gen/yaghan/control_plane/v1alpha1/snapshot_grpc.pb.go
gen_mocks gen/yaghan/data_plane/v1alpha1/daemon_grpc.pb.go

