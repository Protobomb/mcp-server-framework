# Variables
BINARY_NAME=mcp-server
PACKAGE=github.com/protobomb/mcp-server-framework
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build targets
.PHONY: all build clean test coverage lint fmt vet deps docker docker-run help

all: clean deps test build ## Build everything

build: ## Build the server binary
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) cmd/mcp-server/main.go

build-client: ## Build the client binary
	$(GOBUILD) $(LDFLAGS) -o mcp-client cmd/mcp-client/main.go

build-both: build build-client ## Build both server and client

build-all: ## Build for all platforms
	@echo "Building for multiple platforms..."
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 cmd/mcp-server/main.go
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 cmd/mcp-server/main.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 cmd/mcp-server/main.go
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 cmd/mcp-server/main.go
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe cmd/mcp-server/main.go
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/mcp-client-linux-amd64 cmd/mcp-client/main.go
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/mcp-client-linux-arm64 cmd/mcp-client/main.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/mcp-client-darwin-amd64 cmd/mcp-client/main.go
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/mcp-client-darwin-arm64 cmd/mcp-client/main.go
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/mcp-client-windows-amd64.exe cmd/mcp-client/main.go

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME) mcp-client
	rm -rf dist/
	rm -f coverage.out coverage.html

test: ## Run unit tests
	$(GOTEST) -v -race ./pkg/...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -race -coverprofile=coverage.out ./pkg/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

coverage: test-coverage ## Generate coverage report
	$(GOCMD) tool cover -func=coverage.out

lint: ## Run linter
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		$(shell go env GOPATH)/bin/golangci-lint run; \
	fi

fmt: ## Format code
	$(GOCMD) fmt ./...

vet: ## Run go vet
	$(GOCMD) vet ./...

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

deps-update: ## Update dependencies
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

docker: ## Build Docker image
	docker build -t $(BINARY_NAME):latest .
	docker build -t $(BINARY_NAME):$(VERSION) .

docker-run: ## Run Docker container
	docker run -it --rm -p 8080:8080 $(BINARY_NAME):latest -transport=sse -addr=8080

docker-run-stdio: ## Run Docker container with STDIO
	docker run -it --rm $(BINARY_NAME):latest

install: build ## Install binary to GOPATH/bin
	cp $(BINARY_NAME) $(GOPATH)/bin/

run: build ## Build and run the server
	./$(BINARY_NAME)

run-sse: build ## Build and run the server with SSE transport
	./$(BINARY_NAME) -transport=sse -addr=8080

run-client: build-client ## Build and run the test client
	./mcp-client -help

demo: build build-client ## Run the client-server demo
	go run examples/client-server-demo.go

test-client-stdio: build build-client ## Test client with STDIO server
	./mcp-client -transport=stdio -command="./$(BINARY_NAME)"

test-client-http: build build-client ## Test client with HTTP server (requires server running)
	./mcp-client -transport=http -addr=http://localhost:8080

dev: ## Run in development mode with hot reload
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

test-integration: build ## Run integration tests
	@echo "Running integration tests..."
	@./$(BINARY_NAME) -transport=sse -addr=8081 > /dev/null 2>&1 & \
	SERVER_PID=$$!; \
	sleep 2; \
	python3 scripts/test_sse_integration.py 8081; \
	TEST_RESULT=$$?; \
	kill $$SERVER_PID 2>/dev/null || true; \
	exit $$TEST_RESULT

test-all: test test-integration ## Run all tests (unit + integration)

benchmark: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem ./...

security: ## Run security scan
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest)
	gosec ./...

mod-graph: ## Show module dependency graph
	$(GOMOD) graph

mod-why: ## Show why a module is needed
	@read -p "Enter module name: " module; \
	$(GOMOD) why $$module

check: deps fmt vet lint test-all ## Run all checks

ci: check build ## Run CI pipeline locally

release: clean check build-all ## Prepare release

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Default target
.DEFAULT_GOAL := help