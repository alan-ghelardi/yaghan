SHELL := /bin/bash

.PHONY: generate clean help

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

generate: ## Regenerate the firecracker API client from the pinned swagger spec
	@./hack/generate.sh

clean: ## Clean generated files
	@echo "Cleaning generated client + models..."
	@rm -rf client/ models/
	@echo "Done."
