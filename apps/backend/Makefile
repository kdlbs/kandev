# Kandev Makefile

# Variables
BINARY_NAME := kandev
BUILD_DIR := bin
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

# Cross-platform commands
#
# On Windows, GNU Make invokes commands through cmd.exe, which does not
# support Unix `NAME=VAL cmd` env-var-prefix syntax. Instead, `set NAME=VAL&`
# (single ampersand) chains a subsequent command in the same shell, scoping
# the env var to one recipe line — the closest cmd analog to the Unix form.
# `EXE` adds the .exe suffix to build outputs; Go does NOT auto-append it
# when `-o` is explicit.
#
# Install GNU Make on Windows with `winget install ezwinports.make` (or via
# scoop/chocolatey). Targets that rely on Unix builtins (run, dev,
# start-debug, build-cross, build-agentctl-linux) remain Unix-only.
ifeq ($(OS),Windows_NT)
  RM = cmd /c del /s /q
  RMDIR = cmd /c rmdir /s /q
  # cmd's `mkdir` errors when the dir already exists, unlike Unix `mkdir -p`.
  # Call sites use `-@$(MKDIR) $(BUILD_DIR)` (note the `-` prefix) so the
  # "already exists" failure is tolerated; on Unix the prefix is a no-op.
  MKDIR = cmd /c mkdir
  CGO_PREFIX = set CGO_ENABLED=1&
  # Cross-compile prefix for the linux/amd64 agentctl bundled with the host
  # build (used by remote-Sprites executors). cmd has no Unix env-var-prefix
  # syntax; `set X=Y&` chains `set` and the next command in the same shell.
  GO_LINUX_AMD64 = set CGO_ENABLED=0& set GOOS=linux& set GOARCH=amd64&
  EXE = .exe
else
  RM = rm -f
  RMDIR = rm -rf
  MKDIR = mkdir -p
  CGO_PREFIX = CGO_ENABLED=1
  GO_LINUX_AMD64 = CGO_ENABLED=0 GOOS=linux GOARCH=amd64
  EXE =
endif

# Build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || powershell -c "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'" 2>/dev/null || echo "unknown")

LDFLAGS += -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

.PHONY: all build build-all build-agentctl build-agentctl-linux build-acpdbg acpdbg build-mock-agent build-mock-agent-linux build-preview build-winjob clean run dev start-debug test test-e2e test-sprites-e2e lint fmt vet help

## Default target
all: build

## Build the preview CLI for deploying PR preview environments to Sprites
build-preview:
	@echo "Building preview..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/preview$(EXE) ./cmd/preview

## Build the mock-agent binary for testing
build-mock-agent:
	@echo "Building mock-agent..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/mock-agent$(EXE) ./cmd/mock-agent

## Build the winjob helper — wraps a subprocess in a Windows Job Object so
## Ctrl-C at the top of the chain reliably terminates the entire subtree
## (kandev, agentctl, etc.) instead of leaking them. No-op passthrough on Unix.
## See apps/cli/src/dev.ts for how the CLI launcher uses it.
build-winjob:
	@echo "Building winjob..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/winjob$(EXE) ./cmd/winjob

## Build and run acpdbg — usage: make acpdbg ARGS="probe auggie"
acpdbg: build-acpdbg
	@$(BUILD_DIR)/acpdbg$(EXE) $(ARGS)

## Build the acpdbg binary — raw ACP JSON-RPC debugger (see .agents/skills/acp-debug)
build-acpdbg:
	@echo "Building acpdbg..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/acpdbg$(EXE) ./cmd/acpdbg

## Build the unified kandev binary. Always includes the linux/amd64 agentctl
## variant because remote executors (local Docker, remote Docker, Sprites)
## bind-mount it into containers — without it, the first Docker session on a
## non-Linux host fails with "agentctl linux binary not found".
build: build-agentctl build-agentctl-linux build-mock-agent build-acpdbg build-winjob
	@echo "Building $(BINARY_NAME)..."
	-@$(MKDIR) $(BUILD_DIR)
	$(CGO_PREFIX) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)$(EXE) ./cmd/kandev

## Build all binaries (unified + individual services + agentctl)
build-all: build build-agentctl
	@echo "Building individual services..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agent-manager$(EXE) ./cmd/agent-manager

## Build the agentctl sidecar binary (includes embedded MCP server)
build-agentctl:
	@echo "Building agentctl..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl$(EXE) ./cmd/agentctl

## Build agentctl for linux/amd64 (used by Sprites remote executor)
build-agentctl-linux:
	@echo "Building agentctl for linux/amd64..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO_LINUX_AMD64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-linux-amd64 ./cmd/agentctl

## Build mock-agent for linux/amd64 (used by Docker E2E tests)
build-mock-agent-linux:
	@echo "Building mock-agent for linux/amd64..."
	@$(MKDIR) $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/mock-agent-linux-amd64 ./cmd/mock-agent


## Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(RMDIR) $(BUILD_DIR)
	$(GO) clean

## Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)$(EXE)

## Run in development mode (with pprof enabled)
dev: build
	@echo "Running $(BINARY_NAME) in development mode..."
	@KANDEV_DEBUG_PPROF_ENABLED=true exec ./$(BUILD_DIR)/$(BINARY_NAME)$(EXE)

## Run in debug mode (pprof + debug logging)
start-debug: build
	@echo "Running $(BINARY_NAME) in debug mode (pprof + debug logging)..."
	@KANDEV_DEBUG_PPROF_ENABLED=true KANDEV_LOG_LEVEL=debug exec ./$(BUILD_DIR)/$(BINARY_NAME)$(EXE)

## Run tests
test:
	@echo "Running tests..."
	$(CGO_PREFIX) $(GO) test ./...

## Run only the Windows-sensitive packages — mirrors the test-windows job in
## .github/workflows/backend-tests.yml. The broader suite uses Unix-only
## shell commands in test fixtures and would need GOOS guards to be portable;
## this narrow scope is the regression coverage for the conpty/job-object
## bug class. Tests that genuinely cannot run on Windows skip themselves via
## runtime.GOOS guards in source.
test-windows:
	@echo "Running Windows-sensitive packages..."
	$(CGO_PREFIX) $(GO) test -v ./internal/agentctl/server/process/... ./internal/agentctl/client/launcher/...

## Run E2E tests (real agents, costs money — manual only)
## Set agent commands via env vars: E2E_CLAUDE_CODE_COMMAND, E2E_AMP_COMMAND
## Enable debug: KANDEV_DEBUG_AGENT_MESSAGES=true KANDEV_DEBUG_LOG_DIR=/tmp/e2e-debug
test-e2e:
	@echo "Running E2E adapter tests..."
	$(GO) test -tags e2e -v -timeout 10m ./internal/agentctl/server/adapter/e2e/

## Run sprites e2e test (requires SPRITES_API_TOKEN env var)
## Sprites VMs are always x86_64/amd64 regardless of host architecture.
test-sprites-e2e: build-agentctl-linux
	@echo "Running sprites e2e test..."
	KANDEV_AGENTCTL_LINUX_BINARY=$(CURDIR)/$(BUILD_DIR)/agentctl-linux-amd64 \
		$(GO) test -v -count=1 -tags sprites_e2e -run TestSpritesE2E -timeout 10m ./internal/agent/lifecycle/

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(CGO_PREFIX) $(GO) test -v -coverprofile=coverage.out ./...
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
	-@$(MKDIR) $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/kandev

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
	@echo "  build-mock-agent Build the mock-agent binary for testing"
	@echo "  build-preview   Build the preview CLI for PR preview environments"
	@echo "  clean         Remove build artifacts"
	@echo "  run           Build and run the application"
	@echo "  dev           Run in development mode (pprof enabled)"
	@echo "  start-debug   Run in debug mode (pprof + debug logging)"
	@echo "  test          Run tests (full suite — fails on Windows pending fixture cleanup)"
	@echo "  test-windows  Run only the Windows-clean subset (matches CI windows-latest job)"
	@echo "  test-e2e      Run E2E adapter tests (real agents, costs money)"
	@echo "  test-coverage Run tests with coverage report"
	@echo "  lint          Run golangci-lint"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  tidy          Tidy go.mod dependencies"
	@echo "  deps          Download dependencies"
	@echo "  build-cross   Build for Linux, macOS, and Windows"
	@echo "  help          Show this help message"
