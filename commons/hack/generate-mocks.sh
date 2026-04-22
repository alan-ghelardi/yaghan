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
gen_mocks pkg/aws/dynamodb/dynamodb.go
gen_mocks pkg/aws/s3/s3.go
gen_mocks pkg/aws/sns/sns.go
gen_mocks pkg/aws/secretsmanager/secretsmanager.go
gen_mocks pkg/aws/sqs/sqs.go
