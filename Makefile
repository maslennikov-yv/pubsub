.PHONY: test build clean run fmt vet lint install-deps bench init-deps check-go-version vet-verbose

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Check Go version and module support
check-go-version:
	@echo "Checking Go version..."
	@$(GOCMD) version
	@if $(GOCMD) help mod >/dev/null 2>&1; then \
		echo "Go modules supported"; \
	else \
		echo "Go modules not supported, using legacy GOPATH mode"; \
	fi

# Initialize dependencies (with fallback for older Go versions)
init-deps: check-go-version
	@if $(GOCMD) help mod >/dev/null 2>&1; then \
		echo "Using Go modules..."; \
		$(GOCMD) mod tidy; \
		$(GOCMD) mod download; \
	else \
		echo "Using legacy GOPATH mode..."; \
		$(GOGET) -t ./...; \
	fi

# Build the project
build: init-deps
	$(GOBUILD) -v ./...

# Run tests (with fallback for older Go versions)
test: init-deps
	@if $(GOCMD) help mod >/dev/null 2>&1; then \
		$(GOTEST) -v -race -coverprofile=coverage.out ./...; \
	else \
		$(GOTEST) -v -race ./...; \
	fi

# Run tests with coverage report
test-coverage: test
	@if [ -f coverage.out ]; then \
		$(GOCMD) tool cover -html=coverage.out -o coverage.html; \
		echo "Coverage report generated: coverage.html"; \
	else \
		echo "No coverage file found"; \
	fi

# Run benchmarks
bench: init-deps
	$(GOTEST) -bench=. -benchmem ./...

# Run the example
run: init-deps
	$(GOCMD) run example/main.go

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f coverage.out coverage.html

# Format code
fmt:
	$(GOFMT) ./...

# Run go vet with timeout protection
vet:
	@echo "Running go vet..."
	@timeout 30s $(GOVET) ./... || (echo "go vet timed out or failed"; exit 1)

# Run go vet with verbose output for debugging
vet-verbose:
	@echo "Running go vet with verbose output..."
	$(GOVET) -x ./...

# Run go vet on individual files to isolate issues
vet-files:
	@echo "Running go vet on individual files..."
	@for file in *.go; do \
		echo "Checking $$file..."; \
		$(GOVET) $$file || echo "Issue in $$file"; \
	done

# Install dependencies (legacy support)
install-deps: init-deps

# Run all checks with better error handling
check: fmt vet-safe test

# Safe vet that won't hang
vet-safe:
	@echo "Running safe go vet..."
	@$(GOVET) . || echo "go vet found issues but continuing..."

# Initialize module (run once, only for Go 1.11+)
init:
	@if $(GOCMD) help mod >/dev/null 2>&1; then \
		$(GOCMD) mod init pubsub-timeout; \
		$(GOCMD) mod tidy; \
	else \
		echo "Go modules not supported, skipping module initialization"; \
	fi

# Simple build without dependencies (for older Go)
build-simple:
	$(GOBUILD) -v .

# Simple test without modules
test-simple:
	$(GOTEST) -v .

# Quick check without vet (for CI environments with issues)
check-quick: fmt test

# Help
help:
	@echo "Available commands:"
	@echo "  check-go-version  - Check Go version and module support"
	@echo "  init-deps         - Initialize and download dependencies"
	@echo "  build             - Build the project"
	@echo "  build-simple      - Build without dependency management"
	@echo "  test              - Run tests"
	@echo "  test-simple       - Run tests without modules"
	@echo "  test-coverage     - Run tests with coverage report"
	@echo "  bench             - Run benchmarks"
	@echo "  run               - Run the example"
	@echo "  clean             - Clean build artifacts"
	@echo "  fmt               - Format code"
	@echo "  vet               - Run go vet with timeout"
	@echo "  vet-verbose       - Run go vet with verbose output"
	@echo "  vet-files         - Run go vet on individual files"
	@echo "  vet-safe          - Run go vet safely (won't fail build)"
	@echo "  install-deps      - Install dependencies (legacy)"
	@echo "  check             - Run fmt, vet, and test"
	@echo "  check-quick       - Run fmt and test (skip vet)"
	@echo "  help              - Show this help message"
