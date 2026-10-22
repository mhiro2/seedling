SHELL := /bin/bash

GO := go
GOLANGCI_LINT := golangci-lint
ROOT_GO_MOD := $(abspath go.mod)
GOLANGCI_LINT_CONFIG := $(abspath .golangci.yaml)
GOTESTSUM := $(GO) tool -modfile=$(ROOT_GO_MOD) gotest.tools/gotestsum --format=testdox --hide-summary=skipped
GO_LICENSES := $(GO) tool -modfile=$(ROOT_GO_MOD) github.com/google/go-licenses/v2
GO_LICENSES_FLAGS := --include_tests --disallowed_types=unknown,restricted,forbidden

COVERPROFILE := coverage.out

.PHONY: help tools fmt fmt-root fmt-integration fmt-seedlingpgx fmt-check fmt-check-root fmt-check-integration fmt-check-seedlingpgx lint lint-root lint-integration lint-seedlingpgx check-licenses check-licenses-root check-licenses-integration check-licenses-seedlingpgx check-seedlingpgx-parent-version test test-root test-seedlingpgx test-integration-postgres test-integration-mysql test-integration-sqlite test-integration bench fuzz clean

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make tools            Install pinned local tooling' \
		'  make fmt              Format all maintained modules' \
		'  make fmt-check        Check formatting in all maintained modules' \
		'  make lint             Lint all maintained modules' \
		'  make check-licenses   Check licenses in all maintained modules' \
		'  make check-seedlingpgx-parent-version [VERSION=vX.Y.Z]  Check seedlingpgx requires the seedling release being cut' \
		'  make test             Run root and seedlingpgx unit tests' \
		'  make test-integration-postgres Run PostgreSQL integration tests' \
		'  make test-integration-mysql    Run MySQL integration tests' \
		'  make test-integration-sqlite   Run SQLite integration tests' \
		'  make test-integration          Run all integration tests' \
		'  make bench            Run benchmarks with memory metrics' \
		'  make fuzz             Run fuzz targets for parser and reflect-heavy APIs' \
		'  make clean            Remove generated artifacts'

tools:
	mise install

fmt: fmt-root fmt-integration fmt-seedlingpgx

fmt-root:
	$(GOLANGCI_LINT) fmt --config=$(GOLANGCI_LINT_CONFIG) ./...

fmt-integration:
	cd integration && $(GOLANGCI_LINT) fmt --config=$(GOLANGCI_LINT_CONFIG) ./...

fmt-seedlingpgx:
	cd seedlingpgx && $(GOLANGCI_LINT) fmt --config=$(GOLANGCI_LINT_CONFIG) ./...

fmt-check: fmt-check-root fmt-check-integration fmt-check-seedlingpgx

# fmt_check runs the formatter in diff mode inside $(1). It fails when the
# formatter reports a diff, and also when the formatter itself exits non-zero,
# so a broken or missing formatter cannot pass as "nothing to format".
define fmt_check
	@out=$$(cd $(1) && $(GOLANGCI_LINT) fmt --config=$(GOLANGCI_LINT_CONFIG) ./... --diff 2>&1); status=$$?; \
	if [ $$status -ne 0 ] || [ -n "$$out" ]; then \
		if [ -n "$$out" ]; then echo "$$out"; fi; \
		echo "fmt-check failed in $(1) (exit status $$status); run 'make fmt'"; \
		exit 1; \
	fi
endef

fmt-check-root:
	$(call fmt_check,.)

fmt-check-integration:
	$(call fmt_check,integration)

fmt-check-seedlingpgx:
	$(call fmt_check,seedlingpgx)

lint: lint-root lint-integration lint-seedlingpgx

lint-root:
	$(GOLANGCI_LINT) run --config=$(GOLANGCI_LINT_CONFIG) ./...

lint-integration:
	cd integration && $(GOLANGCI_LINT) run --config=$(GOLANGCI_LINT_CONFIG) --build-tags=integration ./...

lint-seedlingpgx:
	cd seedlingpgx && $(GOLANGCI_LINT) run --config=$(GOLANGCI_LINT_CONFIG) ./...

check-licenses: check-licenses-root check-licenses-integration check-licenses-seedlingpgx

check-licenses-root:
	$(GO_LICENSES) check ./... $(GO_LICENSES_FLAGS) --ignore=github.com/mhiro2/seedling

check-licenses-integration:
	cd integration && GOFLAGS=-tags=integration $(GO_LICENSES) check ./... $(GO_LICENSES_FLAGS) --ignore=github.com/mhiro2/seedling/integration

check-licenses-seedlingpgx:
	cd seedlingpgx && $(GO_LICENSES) check ./... $(GO_LICENSES_FLAGS) --ignore=github.com/mhiro2/seedling/seedlingpgx

check-seedlingpgx-parent-version:
	bash scripts/check-seedlingpgx-parent-version.sh $(VERSION)

test: test-root test-seedlingpgx

test-root:
	$(GOTESTSUM) -- -race -shuffle=on -count=1 -covermode=atomic -coverprofile=$(COVERPROFILE) ./...

test-seedlingpgx:
	cd seedlingpgx && GOWORK=off $(GOTESTSUM) -- -race -shuffle=on -count=1 -covermode=atomic -coverprofile=$(COVERPROFILE) ./...

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
	rm -f $(COVERPROFILE) seedlingpgx/$(COVERPROFILE)
