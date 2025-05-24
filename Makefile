.PHONY: test build clean run fmt vet lint install-deps bench init-deps

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Initialize dependencies
init-deps:
	$(GOMOD) tidy
	$(GOMOD) download

# Build the project
build: init-deps
	$(GOBUILD) -v ./...

# Run tests
test: init-deps
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

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

# Run go vet
vet:
	$(GOVET) ./...

# Install dependencies (legacy)
install-deps: init-deps

# Run all checks
check: fmt vet test

# Initialize module (run once)
init:
	$(GOMOD) init pubsub-timeout
	$(GOMOD) tidy

# Help
help:
	@echo "Available commands:"
	@echo "  init-deps     - Initialize and download dependencies"
	@echo "  build         - Build the project"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  bench         - Run benchmarks"
	@echo "  run           - Run the example"
	@echo "  clean         - Clean build artifacts"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  install-deps  - Install dependencies (legacy)"
	@echo "  check         - Run fmt, vet, and test"
	@echo "  help          - Show this help message"
