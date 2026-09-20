# OpenUSSD development tasks.

GO ?= go
PACKAGES := ./...

.PHONY: help
help: ## Show this help.
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all binaries into bin/.
	$(GO) build -o bin/gateway ./gateway/cmd/gateway
	$(GO) build -o bin/fediverse ./adapters/fediverse
	$(GO) build -o bin/ussdsim ./cmd/ussdsim

.PHONY: test
test: ## Run the test suite with the race detector.
	$(GO) test -race $(PACKAGES)

.PHONY: cover
cover: ## Run tests and report coverage per package.
	$(GO) test -cover $(PACKAGES)

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet $(PACKAGES)

.PHONY: fmt
fmt: ## Format the tree.
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if anything is unformatted.
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: lint
lint: ## Run golangci-lint if it is installed.
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; \
	else echo "golangci-lint not installed; skipping (see https://golangci-lint.run)"; fi

.PHONY: check
check: fmt-check vet test ## Everything CI runs.

.PHONY: demo
demo: ## Build and start the gateway and Fediverse adapter under Docker.
	@test -n "$$FEDIVERSE_WEBHOOK_SECRET" || { echo "set FEDIVERSE_WEBHOOK_SECRET first, e.g. export FEDIVERSE_WEBHOOK_SECRET=$$(openssl rand -hex 32)"; exit 1; }
	docker compose up --build

.PHONY: clean
clean: ## Remove build output.
	rm -rf bin/
