.DEFAULT_GOAL=help

# Required for globs to work correctly.
SHELL:=/bin/bash

BUILD_TIME	= $(shell date +%FT%T%z)
BUILD_DIR	= $(CURDIR)/build
PKG_PREFIX	= github.com/hangxie/rapidgo
VERSION		= $(shell git describe --tags --always --dirty)

# Go options.
CGO_ENABLED	:= 0
GO			?= go
GOBIN		= $(shell $(GO) env GOPATH)/bin
GOFLAGS		:= -trimpath
GOSOURCES	:= $(shell find . -type f -name '*.go')
LDFLAGS		:= -w -s \
				-X $(PKG_PREFIX)/internal/buildinfo.Version=$(VERSION) \
				-X $(PKG_PREFIX)/internal/buildinfo.Commit=$(shell git rev-parse --short HEAD) \
				-X $(PKG_PREFIX)/internal/buildinfo.Date=$(BUILD_TIME)

.EXPORT_ALL_VARIABLES:

.PHONY: all
all: deps tools format lint test build  ## Build all common targets

.PHONY: format
format: tools  ## Format source code
	@echo "==> Formatting source code"
	@$(GOBIN)/gofumpt -w -extra $(GOSOURCES)
	@$(GOBIN)/goimports -w -local $(PKG_PREFIX) $(GOSOURCES)

.PHONY: lint
lint: tools  ## Run static code analysis
	@echo "==> Running static code analysis"
	@$(GOBIN)/golangci-lint cache clean
	@$(GOBIN)/golangci-lint run ./... \
		--timeout 5m \
		--enable gocognit

.PHONY: deps
deps:  ## Install build prerequisites
	@echo "==> Installing build prerequisites"
	@$(GO) mod tidy

.PHONY: tools
tools:  ## Install development tools
	@echo "==> Installing development tools"
	@(cd /tmp; \
		$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
		$(GO) install mvdan.cc/gofumpt@latest; \
		$(GO) install golang.org/x/tools/cmd/goimports@latest; \
	)

.PHONY: build
build: deps  ## Build RapidGo for the current platform
	@echo "==> Building executable"
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=$(CGO_ENABLED) \
		$(GO) build $(GOFLAGS) \
			-ldflags '$(LDFLAGS)' \
			-o $(BUILD_DIR)/rapidgo ./cmd/rapidgo

.PHONY: test
test: deps tools  ## Run unit tests with race detection and coverage
	@echo "==> Running unit tests"
	@mkdir -p $(BUILD_DIR)/test
	@set -euo pipefail; \
		cd $(BUILD_DIR)/test; \
		CGO_ENABLED=1 $(GO) test -parallel 4 -race -count 1 -trimpath -coverprofile=coverage.out $(CURDIR)/...; \
		$(GO) tool cover -html=coverage.out -o coverage.html; \
		$(GO) tool cover -func=coverage.out -o coverage.txt; \
		cat coverage.txt

.PHONY: check
check: all  ## Run the full local quality gate
	@git diff --exit-code

.PHONY: clean
clean:  ## Remove generated build artifacts
	@echo "==> Cleaning build artifacts"
	@rm -rf $(BUILD_DIR) vendor

.PHONY: help
help:  ## Print available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		cut -d ":" -f1- | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
