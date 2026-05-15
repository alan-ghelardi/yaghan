# e2e

End-to-end test suite that boots the real `api-server` and `daemon` binaries
against their dev-style backing services (DynamoDB Local, Redis, MinIO) and
drives them through the `yag` CLI — the same path a user takes.

## Prerequisites

1. `hack/setup-dev.sh` has been run on this machine. `assets/` must contain
   `firecracker`, `jailer`, `vmlinux`, and `rootfs.ext4`. CI runs the same
   script in a cache-restored prehook.
2. Docker available, with the `docker compose` v2 plugin.
3. No instance of `api-server/dev/start.sh` or `daemon/dev/start.sh` running —
   the suite reuses the same host ports (8000, 6379, 9000, 9090, 9091, 9092)
   and will conflict.

Sudo is **not** required for the current scenarios. It will become a
requirement when sandbox-lifecycle scenarios land (jailer needs
`CAP_NET_ADMIN`).

## Running

```sh
make test
```

This builds the three binaries into `./bin/`, brings up both docker compose
stacks under the `yaghan-e2e-apiserver` / `yaghan-e2e-daemon` project names,
starts the api-server and daemon as child processes, runs the Ginkgo suite,
then tears everything down. Per-run artefacts (binary stdout/stderr, rendered
config files) land under `./run-<timestamp>/` so a flake is debuggable
without re-running.

## Layout

```
hack/build.sh              builds api-server, daemon, yag into ./bin/
testdata/*.yaml.tmpl       config templates rendered per run
internal/infra/            compose lifecycle, dynamodb/s3 setup, readiness polling
internal/apiserver/        start/stop the api-server binary
internal/daemon/           start/stop the daemon binary
internal/yag/              CLI exec helper (Run, RunYAML)
suite_test.go              Ginkgo bootstrap + BeforeSuite + DeferCleanup chain
*_test.go                  scenarios
```
