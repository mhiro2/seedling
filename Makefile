SHELL := /bin/bash

GO := go
GOLANGCI_LINT := golangci-lint
GOTESTSUM := $(GO) tool gotest.tools/gotestsum --format=testdox --hide-summary=skipped
GO_LICENSES := $(GO) tool github.com/google/go-licenses/v2

COVERPROFILE := coverage.out

.PHONY: help tools fmt fmt-check lint lint-root lint-integration check-licenses test test-integration-postgres test-integration-mysql test-integration-sqlite test-integration bench fuzz clean

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make tools            Install pinned local tooling' \
		'  make fmt              Run golangci-lint fmt on all packages' \
		'  make fmt-check        Check formatting (fails if unformatted)' \
		'  make lint             Run golangci-lint' \
		'  make check-licenses   Check dependency licenses' \
		'  make test             Run unit tests with gotestsum' \
		'  make test-integration-postgres Run PostgreSQL integration tests' \
		'  make test-integration-mysql    Run MySQL integration tests' \
		'  make test-integration-sqlite   Run SQLite integration tests' \
		'  make test-integration          Run all integration tests' \
		'  make bench            Run benchmarks with memory metrics' \
		'  make fuzz             Run fuzz targets for parser and reflect-heavy APIs' \
		'  make clean            Remove generated artifacts'

tools:
	mise install

fmt:
	$(GOLANGCI_LINT) fmt ./...

fmt-check:
	@OUT=$$($(GOLANGCI_LINT) fmt ./... --diff 2>&1); \
	if [ -n "$$OUT" ]; then \
		echo "$$OUT"; \
		echo "Run 'make fmt'"; \
		exit 1; \
	fi

lint: lint-root lint-integration

lint-root:
	$(GOLANGCI_LINT) run ./...

lint-integration:
	cd integration && $(GOLANGCI_LINT) run --build-tags=integration ./...

check-licenses:
	$(GO_LICENSES) check ./... --include_tests --disallowed_types=unknown,restricted,forbidden

test:
	$(GOTESTSUM) -- -race -shuffle=on -count=1 -covermode=atomic -coverprofile=$(COVERPROFILE) ./...

test-integration-postgres:
	cd integration && $(GOTESTSUM) -- -race --shuffle=on -tags=integration -count=1 ./postgres/...

test-integration-mysql:
	cd integration && $(GOTESTSUM) -- -race --shuffle=on -tags=integration -count=1 ./mysql/...

test-integration-sqlite:
	cd integration && $(GOTESTSUM) -- -race --shuffle=on -tags=integration -count=1 ./sqlite/...

test-integration: test-integration-postgres test-integration-mysql test-integration-sqlite

bench:
	$(GO) test -bench=. -benchmem -count=3 ./...

fuzz:
	$(GO) test -fuzz=FuzzSet -fuzztime=10s .
	$(GO) test -fuzz=FuzzUse -fuzztime=10s .
	$(GO) test -fuzz=FuzzParseSchemaWithDialect -fuzztime=10s ./cmd/seedling-gen
	$(GO) test -fuzz=FuzzSetField -fuzztime=10s ./internal/field
	$(GO) test -fuzz=FuzzLookupField -fuzztime=10s ./internal/field

clean:
	rm -f $(COVERPROFILE)
