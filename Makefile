# Zero build/test/lint targets. AGENTS.md says "Build with `make`" and "Run `make
# lint` before opening a PR" — these targets back those instructions.
.DEFAULT_GOAL := build
GO_VERSION := $(shell go list -m -f "{{.GoVersion}}")
GO_TOOLCHAIN := go$(GO_VERSION)
DEADCODE_VERSION := v0.46.0
GOLANGCI_LINT_VERSION := v2.12.2
GOVULNCHECK_VERSION := v1.3.0

ifeq ($(OS),Windows_NT)
TOOLCHAIN_ENV := set "GOTOOLCHAIN=$(GO_TOOLCHAIN)" &&
else
TOOLCHAIN_ENV := GOTOOLCHAIN=$(GO_TOOLCHAIN)
endif

.PHONY: build build-all test test-race vet fmt fmt-check lint lint-static deadcode vulncheck tidy clean help

# Build the main CLI binary into ./zero.
build:
	go build -o zero ./cmd/zero

# Build every command in cmd/.
build-all:
	go build ./...

# Run the full test suite with the race detector (matches CI expectations).
test:
	go test ./... -race -count=1

# Faster, no race detector.
test-quick:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $(shell git ls-files '*.go')

# Fail if any tracked Go file is not gofmt-clean.
fmt-check:
	@out="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

# Lint = formatting check + vet (no extra tooling required).
lint: fmt-check vet

# Versioned tools select the toolchain from their own modules when invoked with
# package@version. Pin them to this module's Go version so they can load it.
lint-static:
	$(TOOLCHAIN_ENV) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --enable-only unused,ineffassign,staticcheck ./...

deadcode:
	$(TOOLCHAIN_ENV) go run golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION) -test=false ./...

vulncheck:
	$(TOOLCHAIN_ENV) go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

tidy:
	go mod tidy

clean:
	rm -f zero
	go clean ./...

help:
	@echo "Targets: build (default), build-all, test, test-quick, vet, fmt, fmt-check, lint, lint-static, deadcode, vulncheck, tidy, clean"
