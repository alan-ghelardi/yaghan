#!/usr/bin/env bash

set -euo pipefail

RED='\e[1;31m'
RESET='\e[0m'

# Doc: Prints a message to stderr and exits with an error code.
# Arguments:
# $@: message to be shown.
function error() {
    >&2  echo -e "${RED}ERROR: $@"
    exit 1
}

# Doc: Invokes a command with root privileges. When the current user
# is already root the command runs directly; otherwise it is wrapped
# with `sudo`. Used by the hack/ scripts so that only the few syscalls
# that need CAP_SYS_ADMIN (loop mounts and the writes inside them)
# escalate, while the rest of the script keeps running as the
# invoking user — leaving go caches, downloaded tarballs, etc. owned
# by that user.
#
# Caveats:
#  * Shell redirections (`>`, `<<`) happen in the parent shell, so
#    `need_root cmd > path` writes via the unprivileged user. Use
#    `printf ... | need_root tee path > /dev/null` (or similar) when
#    the destination requires privilege.
#  * `sudo` may prompt for a password on first use; subsequent calls
#    within the sudo timestamp window reuse the cached credential.
function need_root() {
    if [[ "${EUID}" -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

# Doc: Invokes mockgen tool with the correct flags.
# Installs mockgen automatically if it isn't present in the system.
#
# Arguments:
# $1: file containing the interfaces to be mocked.
# $2 (optional): destination of the file containing the generated mocks. If
# omitted, mocks will be written in a file with the same name as the source,
# created in a folder named mocks under the same directory as the source resides.
gen_mocks() {
    if ! command -v mockgen &>/dev/null; then
        go install go.uber.org/mock/mockgen@latest
    fi

    local source=$1
    local         destination="$(dirname $source)/mocks/$(basename $source)"

    if [ ! -z ${2+x} ]; then
        destination=$2
    fi

    echo "Generating mocks for interfaces declared at $source (mocks will be written at $destination)..."
    cd $cur_dir/..
    mockgen \
        -source=$source \
        -destination=$destination \
        -package=mocks
    echo "Done"
}
