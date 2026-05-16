GO := $(shell command -v go || echo /usr/local/go/bin/go)

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: lint
lint: ## Run golangci-lint linter
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	$(GO) vet ./...

.PHONY: binaries
binaries: ## Build api-server, daemon, and yag into ./bin/.
	./hack/build.sh

.PHONY: test
test: fmt vet binaries ## Run the e2e suite. Brings up docker compose stacks, starts the binaries, runs Ginkgo, tears everything down.
	# -count=1 bypasses the go test cache: the api-server/daemon/yag
	# binaries this suite drives are rebuilt outside Go's view (see
	# hack/build.sh), so a cached pass can mask a broken binary.
	# Coverage is intentionally not collected here — the code under
	# test runs in child processes, so `go test -cover` would only
	# report on e2e/internal/*, not on api-server/daemon/yag.
	$(GO) test -count=1 $$($(GO) list ./... | grep _test) -v -ginkgo.v

##@ Operational escape hatches

.PHONY: compose-up
compose-up: ## Bring up the api-server + daemon docker compose stacks (for debugging a flake).
	docker compose -p yaghan-e2e-apiserver -f ../api-server/dev/docker-compose.yml up -d
	docker compose -p yaghan-e2e-daemon    -f ../daemon/dev/docker-compose.yml    up -d

.PHONY: compose-down
compose-down: ## Tear down the docker compose stacks brought up by `compose-up`.
	docker compose -p yaghan-e2e-apiserver -f ../api-server/dev/docker-compose.yml down --remove-orphans
	docker compose -p yaghan-e2e-daemon    -f ../daemon/dev/docker-compose.yml    down --remove-orphans
