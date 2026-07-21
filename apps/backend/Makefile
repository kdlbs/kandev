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
# start-debug, build-cross) remain Unix-only.
ifeq ($(OS),Windows_NT)
  RM = cmd /c del /s /q
  RMDIR = cmd /c rmdir /s /q
  # cmd's `mkdir` errors when the dir already exists, unlike Unix `mkdir -p`.
  # Call sites use `-@$(MKDIR) $(BUILD_DIR)` (note the `-` prefix) so the
  # "already exists" failure is tolerated; on Unix the prefix is a no-op.
  MKDIR = cmd /c mkdir
  CGO_PREFIX = set CGO_ENABLED=1&
  # Cross-compile prefixes for agentctl helpers bundled with the host build.
  # cmd has no Unix env-var-prefix syntax; `set X=Y&` chains `set` and the next
  # command in the same shell.
  GO_LINUX_AMD64 = set CGO_ENABLED=0& set GOOS=linux& set GOARCH=amd64&
  GO_LINUX_ARM64 = set CGO_ENABLED=0& set GOOS=linux& set GOARCH=arm64&
  GO_DARWIN_ARM64 = set CGO_ENABLED=0& set GOOS=darwin& set GOARCH=arm64&
  GO_DARWIN_AMD64 = set CGO_ENABLED=0& set GOOS=darwin& set GOARCH=amd64&
  EXE = .exe
  # Windows recipes run under cmd.exe, which has no POSIX `if command -v ...; then`
  # syntax. Neither codesign nor rcodesign ships on Windows anyway, so ad-hoc
  # signing is a no-op here: darwin/arm64 stays Go-signed at link time, and
  # darwin/amd64 runs only on Intel Macs (which don't enforce signing at exec).
  define adhoc_sign_darwin
	@echo   (skipping darwin ad-hoc sign on Windows: darwin/arm64 is Go-signed, darwin/amd64 runs on Intel)
  endef
else
  RM = rm -f
  RMDIR = rm -rf
  MKDIR = mkdir -p
  CGO_PREFIX = CGO_ENABLED=1
  GO_LINUX_AMD64 = CGO_ENABLED=0 GOOS=linux GOARCH=amd64
  GO_LINUX_ARM64 = CGO_ENABLED=0 GOOS=linux GOARCH=arm64
  GO_DARWIN_ARM64 = CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
  GO_DARWIN_AMD64 = CGO_ENABLED=0 GOOS=darwin GOARCH=amd64
  EXE =
  # Ad-hoc sign a darwin Mach-O so Apple Silicon (AMFI) will run it; ad-hoc needs
  # no Apple identity. Go already ad-hoc-signs darwin/arm64 at link time, so this
  # is mainly to (a) sign darwin/amd64 too and (b) re-sign uniformly. Prefer
  # codesign on macOS, fall back to rcodesign on Linux/CI. If neither is present,
  # warn but don't fail (darwin/arm64 stays Go-signed; darwin/amd64 runs on Intel).
  define adhoc_sign_darwin
	@if command -v codesign >/dev/null 2>&1; then \
		echo "  codesign --sign - $(1)"; codesign --force --sign - "$(1)"; \
	elif command -v rcodesign >/dev/null 2>&1; then \
		echo "  rcodesign sign (ad-hoc) $(1)"; rcodesign sign "$(1)"; \
	else \
		echo "  WARNING: no codesign/rcodesign found; $(1) left as-is (darwin/arm64 is Go-signed; darwin/amd64 runs only on Intel Macs)" >&2; \
	fi
  endef
endif

# Build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || powershell -c "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'" 2>/dev/null || echo "unknown")

LDFLAGS += -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

.PHONY: all build build-all build-agentctl build-agentctl-remote build-agentctl-linux build-agentctl-linux-arm64 build-agentctl-darwin-arm64 build-agentctl-darwin-amd64 build-acpdbg acpdbg build-mock-agent build-mock-agent-linux build-preview build-winjob e2e-plugin-package clean run dev start-debug test test-e2e test-sprites-e2e test-lifecycle-goleak lint fmt vet help

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

## Build the unified kandev binary. Always includes remote agentctl helpers
## because SSH/Docker/Sprites executors upload or bind-mount platform-specific
## sidecars into remote environments.
build: build-agentctl build-agentctl-remote build-mock-agent build-acpdbg build-winjob
	@echo "Building $(BINARY_NAME)..."
	-@$(MKDIR) $(BUILD_DIR)
	$(CGO_PREFIX) $(GO) build $(GOFLAGS) -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)$(EXE) ./cmd/kandev

## Build all binaries (unified + individual services + agentctl)
build-all: build build-agentctl
	@echo "Building individual services..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agent-manager$(EXE) ./cmd/agent-manager

## Build the agentctl sidecar binary (includes embedded MCP server)
build-agentctl:
	@echo "Building agentctl..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl$(EXE) ./cmd/agentctl

## Build all remote agentctl helpers used by SSH/Docker/Sprites executors.
build-agentctl-remote: build-agentctl-linux build-agentctl-linux-arm64 build-agentctl-darwin-arm64 build-agentctl-darwin-amd64

## Build agentctl for linux/amd64 (used by Docker/Sprites and Linux SSH hosts)
build-agentctl-linux:
	@echo "Building agentctl for linux/amd64..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO_LINUX_AMD64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-linux-amd64 ./cmd/agentctl

## Build agentctl for linux/arm64 (used by ARM Linux SSH hosts, e.g. Graviton)
build-agentctl-linux-arm64:
	@echo "Building agentctl for linux/arm64..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO_LINUX_ARM64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-linux-arm64 ./cmd/agentctl

## Build agentctl for darwin/arm64 (used by Apple Silicon SSH hosts)
build-agentctl-darwin-arm64:
	@echo "Building agentctl for darwin/arm64..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO_DARWIN_ARM64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-darwin-arm64 ./cmd/agentctl
	$(call adhoc_sign_darwin,$(BUILD_DIR)/agentctl-darwin-arm64)

## Build agentctl for darwin/amd64 (used by Intel Mac SSH hosts)
build-agentctl-darwin-amd64:
	@echo "Building agentctl for darwin/amd64..."
	-@$(MKDIR) $(BUILD_DIR)
	$(GO_DARWIN_AMD64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-darwin-amd64 ./cmd/agentctl
	$(call adhoc_sign_darwin,$(BUILD_DIR)/agentctl-darwin-amd64)

## Build mock-agent for linux/amd64 (used by Docker E2E tests)
build-mock-agent-linux:
	@echo "Building mock-agent for linux/amd64..."
	@$(MKDIR) $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/mock-agent-linux-amd64 ./cmd/mock-agent


## Package the plugin-fixture SDK plugin (cmd/plugin-fixture) as a kandev
## plugin tar.gz for backend integration tests and Playwright e2e: stages
## fixture-package/ (manifest.yaml + ui/) into a temp dir, builds the
## plugin-fixture binary for the HOST platform into <stage>/server/, then
## packs it with cmd/plugin-pack (-platform-only: host binary only, smaller
## package) into .build/kandev-plugin-e2e-1.0.0.tar.gz. e2e tests upload
## this file to install the plugin.
FIXTURE_PACKAGE_DIR := cmd/plugin-fixture/fixture-package
FIXTURE_STAGE_DIR := .build/fixture-stage
FIXTURE_PACKAGE_OUT := .build/kandev-plugin-e2e-1.0.0.tar.gz
HOST_GOOS := $(shell $(GO) env GOOS)
HOST_GOARCH := $(shell $(GO) env GOARCH)

.PHONY: e2e-plugin-package
e2e-plugin-package:
	@echo "Packaging plugin-fixture for e2e ($(HOST_GOOS)-$(HOST_GOARCH))..."
	@$(RMDIR) $(FIXTURE_STAGE_DIR)
	@$(MKDIR) $(FIXTURE_STAGE_DIR)/server
	cp $(FIXTURE_PACKAGE_DIR)/manifest.yaml $(FIXTURE_STAGE_DIR)/manifest.yaml
	cp -r $(FIXTURE_PACKAGE_DIR)/ui $(FIXTURE_STAGE_DIR)/ui
	$(GO) build $(GOFLAGS) -o $(FIXTURE_STAGE_DIR)/server/plugin-$(HOST_GOOS)-$(HOST_GOARCH)$(EXE) ./cmd/plugin-fixture
	$(GO) run ./cmd/plugin-pack -dir $(FIXTURE_STAGE_DIR) -out $(FIXTURE_PACKAGE_OUT) -platform-only
	@$(RMDIR) $(FIXTURE_STAGE_DIR)
	@echo "Wrote $(FIXTURE_PACKAGE_OUT)"

## Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(RMDIR) $(BUILD_DIR)
	$(GO) clean

## Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)$(EXE) __backend

## Run in development mode (with pprof enabled)
dev: build
	@echo "Running $(BINARY_NAME) in development mode..."
	@KANDEV_DEBUG_PPROF_ENABLED=true exec ./$(BUILD_DIR)/$(BINARY_NAME)$(EXE) __backend

## Run in debug mode (pprof + debug logging)
start-debug: build
	@echo "Running $(BINARY_NAME) in debug mode (pprof + debug logging)..."
	@KANDEV_DEBUG_PPROF_ENABLED=true KANDEV_LOG_LEVEL=debug exec ./$(BUILD_DIR)/$(BINARY_NAME)$(EXE) __backend

## Run tests
test:
	@echo "Running tests..."
	$(CGO_PREFIX) $(GO) test -tags fts5 ./...

## Run only the Windows-sensitive packages — mirrors the test-windows job in
## .github/workflows/backend-tests.yml. The broader suite uses Unix-only
## shell commands in test fixtures and would need GOOS guards to be portable;
## this narrow scope is the regression coverage for the conpty/job-object
## bug class. Tests that genuinely cannot run on Windows skip themselves via
## runtime.GOOS guards in source.
test-windows:
	@echo "Running Windows-sensitive packages..."
	$(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/agentctl/server/process/... ./internal/agent/runtime/agentctl/launcher/...

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
		$(GO) test -v -count=1 -tags sprites_e2e -run TestSpritesE2E -timeout 10m ./internal/agent/runtime/lifecycle/

## Stress-run the lifecycle goleak suite — repro target for the CI-only flake
## where StreamManager/WorkspaceStream goroutines linger past TestMain on
## slow runners. Local hardware almost never reproduces the leak under a
## single pass; -count=20 (configurable) plus -race is the smallest cadence
## that consistently surfaces the race during development.
##
## Override the loop count with `make test-lifecycle-goleak LIFECYCLE_GOLEAK_COUNT=N`.
LIFECYCLE_GOLEAK_COUNT ?= 20
test-lifecycle-goleak:
	@echo "Stress-running lifecycle goleak suite ($(LIFECYCLE_GOLEAK_COUNT)x)..."
	$(CGO_PREFIX) $(GO) test -tags fts5 -race -count=$(LIFECYCLE_GOLEAK_COUNT) -timeout 600s ./internal/agent/runtime/lifecycle/ ./internal/agent/runtime/agentctl/

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(CGO_PREFIX) $(GO) test -tags fts5 -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## Regenerate Go stubs from proto/kandev/plugin/v1/plugin.proto (§3 of
## docs/plans/plugins/GRPC-CONTRACT.md). Installs protoc-gen-go and
## protoc-gen-go-grpc into a local .tools/bin (not committed) and drives them
## via `go run` buf — no system protoc required. Requires network access for
## the first run (tool + buf module downloads); output is committed so
## `go build` never needs this target.
PROTO_TOOLS_BIN := $(CURDIR)/.tools/bin
BUF_VERSION := v1.47.2
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1

.PHONY: proto
proto:
	@echo "Generating proto stubs..."
	@$(MKDIR) $(PROTO_TOOLS_BIN)
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	cd proto && PATH="$(PROTO_TOOLS_BIN):$$PATH" $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

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
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/kandev
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 $(GO) build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/kandev

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
	@echo "  e2e-plugin-package  Package the plugin-fixture SDK plugin into .build/kandev-plugin-e2e-1.0.0.tar.gz"
	@echo "  test-lifecycle-goleak  Stress-run lifecycle goleak suite (override LIFECYCLE_GOLEAK_COUNT=N)"
	@echo "  test-coverage Run tests with coverage report"
	@echo "  lint          Run golangci-lint"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  tidy          Tidy go.mod dependencies"
	@echo "  deps          Download dependencies"
	@echo "  build-cross   Build for Linux, macOS, and Windows"
	@echo "  help          Show this help message"
