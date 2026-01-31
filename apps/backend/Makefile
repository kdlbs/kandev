# Kandev Makefile

# Variables
BINARY_NAME := kandev
BUILD_DIR := bin
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

# Build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS += -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

.PHONY: all build build-all build-agentctl clean run dev test lint fmt vet help

## Default target
all: build

## Build the unified kandev binary
build: build-agentctl
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/kandev

## Build all binaries (unified + individual services + agentctl)
build-all: build build-agentctl
	@echo "Building individual services..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agent-manager ./cmd/agent-manager

## Build the agentctl sidecar binary (includes embedded MCP server)
build-agentctl:
	@echo "Building agentctl..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl ./cmd/agentctl

## Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	$(GO) clean

## Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

## Run in development mode
dev: build
	@echo "Running $(BINARY_NAME) in development mode..."
	@exec ./$(BUILD_DIR)/$(BINARY_NAME)

## Run tests
test:
	@echo "Running tests..."
	$(GO) test ./...

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## Run go vet
vet:
	@echo "Running vet..."
	$(GO) vet ./...

## Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GO) mod tidy

## Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

## Build for multiple platforms
build-cross:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/kandev
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/kandev
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/kandev
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/kandev

## Show help
help:
	@echo "Kandev - AI Agent Kanban Board"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build           Build the unified kandev binary (default)"
	@echo "  build-all       Build all binaries (unified + individual services + agentctl)"
	@echo "  build-agentctl  Build the agentctl sidecar binary (includes embedded MCP server)"
	@echo "  clean         Remove build artifacts"
	@echo "  run           Build and run the application"
	@echo "  dev           Run in development mode with a local database"
	@echo "  test          Run tests"
	@echo "  test-coverage Run tests with coverage report"
	@echo "  lint          Run golangci-lint"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  tidy          Tidy go.mod dependencies"
	@echo "  deps          Download dependencies"
	@echo "  build-cross   Build for Linux, macOS, and Windows"
	@echo "  help          Show this help message"
