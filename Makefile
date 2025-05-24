.PHONY: test build clean run fmt vet lint bench coverage help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet
GOFMT=gofmt

# Project settings
BINARY_NAME=pubsub-example
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Build flags
BUILD_FLAGS=-v
TEST_FLAGS=-v -race -coverprofile=$(COVERAGE_FILE)
BENCH_FLAGS=-bench=. -benchmem

# Ensure Go modules are enabled
export GO111MODULE=on

# Default target
all: clean fmt vet test build

# Download and tidy dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Verify dependencies
deps-verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Build the project
build: deps
	@echo "Building project..."
	$(GOBUILD) $(BUILD_FLAGS) ./...

# Build the example binary
build-example: deps
	@echo "Building example..."
	@rm -f $(BINARY_NAME)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) ./example

# Run tests
test: deps
	@echo "Running tests..."
	$(GOTEST) $(TEST_FLAGS) ./...

# Run tests without race detection (faster)
test-fast: deps
	@echo "Running fast tests..."
	$(GOTEST) -v ./...

# Run tests with coverage report
coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

# Display coverage in terminal
coverage-text: test
	@echo "Coverage summary:"
	$(GOCMD) tool cover -func=$(COVERAGE_FILE)

# Run benchmarks
bench: deps
	@echo "Running benchmarks..."
	$(GOTEST) $(BENCH_FLAGS) ./...

# Run the example
run: build-example
	@echo "Running example..."
	./$(BINARY_NAME)

# Run example without building binary
run-direct: deps
	@echo "Running example directly..."
	$(GOCMD) run ./example

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Check if code is formatted
fmt-check:
	@echo "Checking code formatting..."
	@if [ "$$($(GOFMT) -s -l . | wc -l)" -gt 0 ]; then \
		echo "Code is not formatted. Run 'make fmt' to fix:"; \
		$(GOFMT) -s -l .; \
		exit 1; \
	fi

# Run go vet
vet: deps
	@echo "Running go vet..."
	$(GOVET) ./...

# Run static analysis
lint: fmt-check vet
	@echo "Running static analysis..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Install development tools
tools:
	@echo "Installing development tools..."
	@echo "Installing golangci-lint..."
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Installing govulncheck..."
	$(GOCMD) install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Installing gosec..."
	$(GOCMD) install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@echo "Verifying installations..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "✓ golangci-lint installed successfully"; \
		golangci-lint version; \
	else \
		echo "✗ golangci-lint installation failed"; \
	fi
	@if command -v govulncheck >/dev/null 2>&1; then \
		echo "✓ govulncheck installed successfully"; \
		govulncheck -version; \
	else \
		echo "✗ govulncheck installation failed"; \
	fi
	@if command -v gosec >/dev/null 2>&1; then \
		echo "✓ gosec installed successfully"; \
		gosec -version; \
	else \
		echo "✗ gosec installation failed"; \
	fi

# Security vulnerability check with better error handling
security: deps
	@echo "Checking for security vulnerabilities..."
	@echo "Running govulncheck..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not found. Installing..."; \
		$(GOCMD) install golang.org/x/vuln/cmd/govulncheck@latest; \
		if command -v govulncheck >/dev/null 2>&1; then \
			echo "Installation successful. Running govulncheck..."; \
			govulncheck ./...; \
		else \
			echo "Failed to install govulncheck. Skipping vulnerability check."; \
		fi \
	fi
	@echo "Running gosec..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not found. Installing..."; \
		$(GOCMD) install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; \
		if command -v gosec >/dev/null 2>&1; then \
			echo "Installation successful. Running gosec..."; \
			gosec ./...; \
		else \
			echo "Failed to install gosec. Skipping security scan."; \
		fi \
	fi

# Update dependencies to latest versions
deps-update:
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

# Check for outdated dependencies
deps-outdated:
	@echo "Checking for outdated dependencies..."
	$(GOCMD) list -u -m all

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
	rm -f $(COVERAGE_HTML)
	rm -f gosec-results.sarif

# Deep clean (including module cache)
clean-all: clean
	@echo "Cleaning module cache..."
	$(GOCMD) clean -modcache

# Run all quality checks
check: fmt-check vet lint test

# Run CI pipeline locally
ci: clean deps-verify check coverage bench security

# Development workflow
dev: clean fmt vet test run

# Release build with optimizations
release: clean deps
	@echo "Building release..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME)-linux-amd64 ./example
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME)-windows-amd64.exe ./example
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME)-darwin-amd64 ./example
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME)-darwin-arm64 ./example

# Watch for file changes and run tests (requires entr)
watch:
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -c make test; \
	else \
		echo "entr not installed. Install with your package manager for file watching."; \
	fi

# Generate documentation
docs:
	@echo "Generating documentation..."
	$(GOCMD) doc -all .

# Show project statistics
stats:
	@echo "Project statistics:"
	@echo "Lines of Go code:"
	@find . -name "*.go" -not -path "./vendor/*" | xargs wc -l | tail -1
	@echo ""
	@echo "Dependencies:"
	@$(GOMOD) graph | wc -l
	@echo ""
	@echo "Test coverage:"
	@make coverage-text 2>/dev/null | grep "total:" || echo "Run 'make test' first"

# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "Building:"
	@echo "  build         - Build the project"
	@echo "  build-example - Build example binary"
	@echo "  release       - Cross-compile release binaries"
	@echo ""
	@echo "Testing:"
	@echo "  test          - Run all tests with coverage"
	@echo "  test-fast     - Run tests without race detection"
	@echo "  bench         - Run benchmarks"
	@echo "  coverage      - Generate HTML coverage report"
	@echo "  coverage-text - Show coverage summary"
	@echo ""
	@echo "Code Quality:"
	@echo "  fmt           - Format code"
	@echo "  fmt-check     - Check code formatting"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run static analysis"
	@echo "  check         - Run all quality checks"
	@echo "  security      - Check for vulnerabilities"
	@echo ""
	@echo "Dependencies:"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  deps-verify   - Verify dependencies"
	@echo "  deps-update   - Update dependencies to latest"
	@echo "  deps-outdated - Check for outdated dependencies"
	@echo ""
	@echo "Running:"
	@echo "  run           - Build and run example"
	@echo "  run-direct    - Run example without building binary"
	@echo ""
	@echo "Development:"
	@echo "  tools         - Install development tools"
	@echo "  watch         - Watch files and run tests (requires entr)"
	@echo "  dev           - Development workflow (clean, fmt, vet, test, run)"
	@echo "  ci            - Run CI pipeline locally"
	@echo ""
	@echo "Utilities:"
	@echo "  clean         - Clean build artifacts"
	@echo "  clean-all     - Clean everything including module cache"
	@echo "  docs          - Generate documentation"
	@echo "  stats         - Show project statistics"
	@echo "  help          - Show this help message"

# Install dependencies with better error handling
install-deps: deps

# Safe vet that doesn't fail on warnings
vet-safe: deps
	@echo "Running go vet (safe mode)..."
	-$(GOVET) ./...
