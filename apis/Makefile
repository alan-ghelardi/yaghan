SHELL := /bin/bash

.PHONY: generate generate-rest-docs clean help lint

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

generate: ## Generate protobuf code and all documentation
	@./hack/update-codegen.sh

generate-rest-docs: ## Generate REST API documentation from OpenAPI files
	@./hack/generate-rest-docs.sh

lint: ## Lint protobuf files
	@buf lint

clean: ## Clean generated files
	@echo "Cleaning generated files..."
	@rm -rf gen/
	@echo "Done."
