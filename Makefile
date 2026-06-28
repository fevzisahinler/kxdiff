BINARY := kubectl-kxdiff
PKG    := ./...

# Tool not on PATH but runnable without a global install, pinned at run time.
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@latest

.DEFAULT_GOAL := help

.PHONY: help build run test cover fmt vet lint vuln check hooks pre-commit snapshot release-check tidy clean

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | \
		awk -F': ' '{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# --- build & run -------------------------------------------------------------

## build: compile the kubectl plugin into ./bin
build:
	go build -o bin/$(BINARY) ./cmd/kxdiff

## run: build then print --help
run: build
	./bin/$(BINARY) --help

# --- tests -------------------------------------------------------------------

## test: run all tests with the race detector
test:
	go test $(PKG) -race -count=1

## cover: run tests and print a per-function coverage report
cover:
	go test $(PKG) -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

# --- quality -----------------------------------------------------------------

## fmt: format code (gofmt + goimports via golangci-lint)
fmt:
	golangci-lint fmt

## vet: run go vet
vet:
	go vet $(PKG)

## lint: run golangci-lint (.golangci.yml; includes gosec)
lint:
	golangci-lint run

## vuln: scan dependencies for known vulnerabilities
vuln:
	$(GOVULNCHECK) $(PKG)

## check: full local gate — fmt + vet + lint + test + vuln (run before commit)
check: fmt vet lint test vuln
	@echo "✓ all checks passed"

# --- git hooks (pre-commit framework) ----------------------------------------

## hooks: install pre-commit git hooks (one-time setup)
hooks:
	pre-commit install
	pre-commit install --hook-type commit-msg

## pre-commit: run all pre-commit hooks against every file
pre-commit:
	pre-commit run --all-files

# --- release (.goreleaser.yaml; used in Faz 5) -------------------------------

## snapshot: build release artifacts locally, no publish
snapshot:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed → brew install goreleaser"; exit 1; }
	goreleaser build --snapshot --clean

## release-check: validate .goreleaser.yaml
release-check:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed → brew install goreleaser"; exit 1; }
	goreleaser check

# --- housekeeping ------------------------------------------------------------

## tidy: sync go.mod / go.sum
tidy:
	go mod tidy

## clean: remove build & coverage artifacts
clean:
	rm -rf bin dist coverage.out
