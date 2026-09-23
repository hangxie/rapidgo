.DEFAULT_GOAL := help

GO ?= go
BUILD_DIR := $(CURDIR)/build
BINARY := $(BUILD_DIR)/rapidgo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/hangxie/rapidgo/internal/buildinfo.Version=$(VERSION) \
	-X github.com/hangxie/rapidgo/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/hangxie/rapidgo/internal/buildinfo.Date=$(BUILD_DATE)

.PHONY: build
build: ## Build RapidGo for the current platform
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/rapidgo

.PHONY: test
test: ## Run unit tests with the race detector
	$(GO) test -race -cover ./...

.PHONY: format
format: ## Format Go source files
	$(GO) fmt ./...

.PHONY: lint
lint: ## Run standard static analysis
	$(GO) vet ./...

.PHONY: check
check: format lint test build ## Run the full local quality gate
	@git diff --exit-code

.PHONY: clean
clean: ## Remove generated build artifacts
	rm -rf $(BUILD_DIR) coverage.out

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

