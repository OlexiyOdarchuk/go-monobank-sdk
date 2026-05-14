# Common dev tasks for go-monobank-sdk.
# Run `make help` (default) for the list.

.DEFAULT_GOAL := help
.PHONY: help test test-race test-all cover cover-html lint fmt fmt-check vet bench fuzz fuzz-all integration tidy ci

GO          ?= go
PKGS        := ./...
COVER_OUT   := coverage.txt
COVER_HTML  := coverage.html
FUZZTIME    ?= 30s

help: ## Show this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests (no race detector — faster).
	$(GO) test $(PKGS)

test-race: ## Run unit tests with the race detector. CI default.
	$(GO) test -race $(PKGS)

test-all: ## Run tests across every workspace module (root + otelmonobank).
	$(GO) test -race $(PKGS)
	cd otelmonobank && $(GO) test -race ./...

cover: ## Generate coverage profile at $(COVER_OUT).
	$(GO) test -race -coverprofile=$(COVER_OUT) $(PKGS)
	@$(GO) tool cover -func=$(COVER_OUT) | tail -1

cover-html: cover ## Open the HTML coverage report in the default browser.
	$(GO) tool cover -html=$(COVER_OUT) -o $(COVER_HTML)
	@echo "open $(COVER_HTML)"

vet: ## go vet across all packages.
	$(GO) vet $(PKGS)

fmt: ## Format every Go file in place.
	gofmt -w .

fmt-check: ## Fail if anything is unformatted (CI gate).
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "unformatted:"; echo "$$files"; exit 1; \
	fi

lint: ## Run golangci-lint with the project config.
	golangci-lint run

bench: ## Run all benchmarks once.
	$(GO) test -run='^$$' -bench=. -benchmem $(PKGS)

# `make fuzz TARGET=FuzzVerify PKG=./webhook` — single target for FUZZTIME.
TARGET ?= Fuzz
PKG    ?= ./...
fuzz: ## Run a single fuzz target. TARGET=FuzzXxx PKG=./pkg FUZZTIME=30s
	$(GO) test -run='^$$' -fuzz=$(TARGET) -fuzztime=$(FUZZTIME) $(PKG)

fuzz-all: ## Quickly smoke-fuzz every Fuzz* target for $(FUZZTIME) each.
	@for pkg in $$(grep -rl '^func Fuzz' --include='*_test.go' . | xargs -n1 dirname | sort -u); do \
		for fn in $$(grep -h '^func Fuzz' $$pkg/*_test.go | sed 's/func \(Fuzz[A-Za-z0-9_]*\).*/\1/'); do \
			echo "==> $$pkg :: $$fn"; \
			$(GO) test -run='^$$' -fuzz=$$fn -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
		done; \
	done

integration: ## Run integration tests against the live monobank API.
	$(GO) test -tags=integration -race -run Integration ./...

tidy: ## go mod tidy in the root and otelmonobank submodule.
	$(GO) mod tidy
	cd otelmonobank && $(GO) mod tidy

ci: fmt-check vet test-race ## Replicate the core CI checks locally.
