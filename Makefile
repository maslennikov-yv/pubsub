# Development tasks for github.com/maslennikov-yv/pubsub (a library: no build artifacts).

GOLANGCI_LINT_VERSION ?= v2.13.2
LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
COVERAGE_FILE = coverage.out

.PHONY: help test cover bench fmt fmt-check vet lint vuln tidy-check check example clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

test: ## Run tests with the race detector and collect coverage
	go test -race -count=1 -coverprofile=$(COVERAGE_FILE) ./...

cover: test ## Show per-function coverage
	go tool cover -func=$(COVERAGE_FILE)

bench: ## Run benchmarks once (smoke run)
	go test -run='^$$' -bench=. -benchmem -benchtime=1x ./...

fmt: ## Format code
	gofmt -s -w .

fmt-check: ## Fail if code is not formatted
	@unformatted="$$(gofmt -s -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (pinned version, via go run)
	$(LINT) run

vuln: ## Run govulncheck
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy-check: ## Fail if go.mod is not tidy
	go mod tidy -diff

check: fmt-check vet tidy-check lint test bench ## The CI gates (govulncheck runs separately: make vuln)

example: ## Run the sensors example
	go run ./examples/sensors

clean: ## Remove test output
	rm -f $(COVERAGE_FILE) coverage.html
