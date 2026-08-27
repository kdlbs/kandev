# Kandev Makefile

# Variables
BINARY_NAME := kandev
BUILD_DIR := bin
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

# Debug launches get a distinct browser identity by default. An explicit
# environment or command-line value still wins through Make's `?=` semantics.
KANDEV_WEB_TITLE_PREFIX ?= Debug

# Quote the debug title prefix for the POSIX recipe. The replacement keeps
# embedded single quotes safe while preserving spaces and shell metacharacters.
single_quote := '
single_quote_escape := '\''
shell_quote = $(single_quote)$(subst $(single_quote),$(single_quote_escape),$(1))$(single_quote)

# Cross-platform commands
#
# Two independent axes decide these variables; conflating them breaks Git
# Bash/MSYS, which sets OS=Windows_NT yet runs recipes through sh:
#
#  1. Executable suffix (EXE) — keyed on $(OS). Go emits a `.exe` for
#     GOOS=windows regardless of the calling shell, so native cmd/PowerShell
#     AND Git Bash both need it. Go does NOT auto-append it when `-o` is set.
#  2. Shell syntax — keyed on $(OS)$(MSYSTEM). Only native cmd/PowerShell needs
#     `set NAME=VAL&` (single ampersand chains a command in the same shell,
#     scoping the env var to one recipe line — the closest cmd analog to the
#     Unix `NAME=VAL cmd` prefix), backslashed paths, and no `exec`. Under Git
#     Bash/MSYS $(MSYSTEM) is set (e.g. MINGW64), so the concatenation differs
#     from "Windows_NT" and recipes take the POSIX branch — matching the shell
#     they run in. Mirrors the doctor guard in the root Makefile.
#
# Install GNU Make on Windows with `winget install ezwinports.make` (or via
# scoop/chocolatey). build-cross remains Unix-only — it cross-compiles with
# CGO for other OSes, which needs Unix toolchains.
ifeq ($(OS),Windows_NT)
  EXE := .exe
else
  EXE :=
endif

ifeq ($(OS)$(MSYSTEM),Windows_NT)
  RM = cmd /c del /s /q
  RMDIR = cmd /c rmdir /s /q
  # cmd's `mkdir` errors and prints "A subdirectory or file <dir> already
  # exists." when the dir is already there, unlike Unix `mkdir -p`. Check for
  # the directory first so that expected repeats stay quiet while permission
  # and invalid-path errors keep their stderr and exit status.
  define MKDIR
	cmd /c if not exist "$(subst /,\,$(1))" mkdir "$(subst /,\,$(1))"
  endef
  CGO_PREFIX = set CGO_ENABLED=1&
  DEADCODE_ENV = set GOTOOLCHAIN=$(DEADCODE_GO_TOOLCHAIN)&
  # Cross-compile prefixes for agentctl helpers bundled with the host build.
  # cmd has no Unix env-var-prefix syntax; `set X=Y&` chains `set` and the next
  # command in the same shell.
  GO_LINUX_AMD64 = set CGO_ENABLED=0& set GOOS=linux& set GOARCH=amd64&
  GO_LINUX_ARM64 = set CGO_ENABLED=0& set GOOS=linux& set GOARCH=arm64&
  GO_DARWIN_ARM64 = set CGO_ENABLED=0& set GOOS=darwin& set GOARCH=arm64&
  GO_DARWIN_AMD64 = set CGO_ENABLED=0& set GOOS=darwin& set GOARCH=amd64&
  # Run-target env prefixes and binary invocation. cmd.exe has no `NAME=VAL cmd`
  # syntax (use `set NAME=VAL&`) and no `exec`; the binary path needs backslashes.
  DEV_ENV = set KANDEV_DEBUG_DEV_MODE=true&
  DEBUG_ENV = set KANDEV_DEBUG_PPROF_ENABLED=true& set KANDEV_LOG_LEVEL=debug& set "KANDEV_WEB_TITLE_PREFIX=$(KANDEV_WEB_TITLE_PREFIX)"&
  EXEC =
  RUN_BIN = "$(subst /,\,$(BUILD_DIR)/$(BINARY_NAME)$(EXE))"
  # check-make-shells is a bash script; native cmd.exe can't run its shebang, so
  # print a skip note instead (mirrors the root Makefile's doctor guard). CI runs
  # the real check on Linux, and Git Bash/WSL run it locally.
  CHECK_SHELLS = @echo check-make-shells is bash-only - run it from Git Bash/WSL or the Linux CI
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
  define MKDIR
	mkdir -p "$(1)"
  endef
  CGO_PREFIX = CGO_ENABLED=1
  DEADCODE_ENV = GOTOOLCHAIN=$(DEADCODE_GO_TOOLCHAIN)
  GO_LINUX_AMD64 = CGO_ENABLED=0 GOOS=linux GOARCH=amd64
  GO_LINUX_ARM64 = CGO_ENABLED=0 GOOS=linux GOARCH=arm64
  GO_DARWIN_ARM64 = CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
  GO_DARWIN_AMD64 = CGO_ENABLED=0 GOOS=darwin GOARCH=amd64
  DEV_ENV = KANDEV_DEBUG_DEV_MODE=true
  DEBUG_ENV = KANDEV_DEBUG_PPROF_ENABLED=true KANDEV_LOG_LEVEL=debug KANDEV_WEB_TITLE_PREFIX=$(call shell_quote,$(KANDEV_WEB_TITLE_PREFIX))
  EXEC = exec
  RUN_BIN = ./$(BUILD_DIR)/$(BINARY_NAME)$(EXE)
  CHECK_SHELLS = @../../scripts/check-make-shells
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

# CGO locale for Git Bash/MSYS on Windows.
#
# This mingw (WinLibs) gcc resolves its own install path, then passes it to ld/
# collect2 across a locale-sensitive boundary. Under a UTF-8 locale, ld mis-decodes
# the ANSI-encoded path, so a non-ASCII segment (e.g. an accented Windows
# username under C:\Users\...) collapses to U+FFFD and the link cannot find
# crt2.o / crtbegin.o / -lm. Forcing the C locale keeps the raw path bytes, so the
# CGO link resolves. See docs/public/windows-support.md.
#
# Use LC_ALL=C, not LANG=C: LANG is the lowest-precedence locale variable
# (LC_ALL > LC_CTYPE > LANG), so LANG=C is silently ignored when the shell already
# exports LC_ALL or LC_CTYPE as UTF-8. LC_ALL=C overrides them all.
#
# Scope mirrors the shell-syntax block above: Windows but NOT native cmd, i.e.
# $(MSYSTEM) is set, so this is Git Bash/MSYS. It is empty only on Unix and native
# cmd. On Git Bash/MSYS the prefix is always applied: a harmless no-op on an ASCII
# toolchain path, and the actual fix on a non-ASCII one.
ifeq ($(OS),Windows_NT)
  ifneq ($(OS)$(MSYSTEM),Windows_NT)
    CGO_LOCALE := LC_ALL=C
  endif
endif

# Build info
#
# Keyed on $(OS)$(MSYSTEM) like the shell-syntax block above. cmd.exe has no
# /dev/null and resolves redirections *before* running the command, so under
# native cmd/PowerShell `2>/dev/null` aborts each command outright and hands
# every value to its fallback — stamping local Windows builds dev/unknown/unknown
# while printing "The system cannot find the path specified." per assignment.
# cmd's own `date` is no substitute: given arguments it prompts for input and
# still exits 0, so it would swallow the `||`. Git Bash sets MSYSTEM and runs
# recipes through sh, so it takes the POSIX branch.
#
# Fallbacks are unquoted: sh strips the quotes, cmd's `echo` would keep them.
#
# NULL_REDIR holds the whole redirect token, not just the device path, so a
# caller simulating another platform can blank it (`make NULL_REDIR=`) instead
# of naming a device this shell would misread — see scripts/check-make-shells.
ifeq ($(OS)$(MSYSTEM),Windows_NT)
  NULL_REDIR := 2>NUL
  # `Get-Date -Format` renders *local* time with a literal Z; UtcNow is real UTC.
  # InvariantCulture is not optional: ToString uses the current culture's
  # calendar, so a th-TH host stamps year 2569 and ar-SA stamps 1448.
  BUILD_TIME := $(shell powershell -NoProfile -Command "[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ',[Globalization.CultureInfo]::InvariantCulture)" $(NULL_REDIR) || echo unknown)
else
  NULL_REDIR := 2>/dev/null
  BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' $(NULL_REDIR) || echo unknown)
endif

VERSION ?= $(shell git describe --tags --always --dirty $(NULL_REDIR) || echo dev)
COMMIT := $(shell git rev-parse --short HEAD $(NULL_REDIR) || echo unknown)

LDFLAGS += -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

# Development only needs the native agentctl plus the linux/amd64 helper when
# the control plane is not linux/amd64. A same-platform Linux/amd64 remote can
# use the native helper through AgentctlResolver's dev fallback.
DEV_HOST_GOOS := $(shell $(GO) env GOHOSTOS $(NULL_REDIR))
DEV_HOST_GOARCH := $(shell $(GO) env GOHOSTARCH $(NULL_REDIR))
ifeq ($(DEV_HOST_GOOS),linux)
ifeq ($(DEV_HOST_GOARCH),amd64)
  DEV_AGENTCTL_TARGETS := build-agentctl
else
  DEV_AGENTCTL_TARGETS := build-agentctl build-agentctl-linux
endif
else
  DEV_AGENTCTL_TARGETS := build-agentctl build-agentctl-linux
endif

# Force the application module's Go version; go run otherwise selects the
# analyzer module's minimum toolchain before go/packages loads this module.
DEADCODE_VERSION ?= v0.48.0
DEADCODE_GO_TOOLCHAIN ?= go1.26.0

.PHONY: all build build-dev build-runtime build-kandev build-all build-agentctl build-agentctl-dev build-agentctl-remote build-agentctl-linux build-agentctl-linux-arm64 build-agentctl-darwin-arm64 build-agentctl-darwin-amd64 build-acpdbg acpdbg build-mock-agent build-mock-agent-linux build-preview build-winjob e2e-plugin-package clean run dev start-debug test test-e2e test-sprites-e2e test-lifecycle-goleak lint deadcode fmt vet check-make-shells help

## Default target
all: build

## Build the preview CLI for deploying PR preview environments to Sprites
build-preview:
	@echo "Building preview..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/preview$(EXE) ./cmd/preview

## Build the mock-agent binary for testing
build-mock-agent:
	@echo "Building mock-agent..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/mock-agent$(EXE) ./cmd/mock-agent

## Build the winjob helper — wraps a subprocess in a Windows Job Object so
## Ctrl-C at the top of the chain reliably terminates the entire subtree
## (kandev, agentctl, etc.) instead of leaking them. No-op passthrough on Unix.
## The native Go launcher replaces this on the `make dev` path with its own
## process-group handling (internal/launcher/process_group_windows.go); the
## helper stays for any other consumer.
build-winjob:
	@echo "Building winjob..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/winjob$(EXE) ./cmd/winjob

## Build and run acpdbg — usage: make acpdbg ARGS="probe auggie"
acpdbg: build-acpdbg
	@$(BUILD_DIR)/acpdbg$(EXE) $(ARGS)

## Build the acpdbg binary — raw ACP JSON-RPC debugger (see .agents/skills/acp-debug)
build-acpdbg:
	@echo "Building acpdbg..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/acpdbg$(EXE) ./cmd/acpdbg

## Build the complete development binary set.
build: build-runtime build-mock-agent build-acpdbg build-winjob

## Build the lean development binary set. Unlike `build`, this does not build
## every cross-platform remote helper; dev needs the host helper and, for a
## host other than Linux/amd64, one linux/amd64 helper.
build-dev: build-kandev build-agentctl-dev build-mock-agent build-acpdbg build-winjob

## Build only the binaries shipped in a runtime bundle. Always includes remote
## agentctl helpers because SSH/Docker/Sprites executors upload or bind-mount
## platform-specific sidecars into remote environments.
build-runtime: build-agentctl build-agentctl-remote build-kandev

## Build the native kandev binary used both by runtime bundles and by the
## copied make-dev launcher. Keep this separate from build-runtime so the
## launcher prebuild does not pull in agentctl helpers it does not use yet.
build-kandev:
	@echo "Building $(BINARY_NAME)..."
	@$(call MKDIR,$(BUILD_DIR))
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) build $(GOFLAGS) -tags fts5 -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)$(EXE) ./cmd/kandev

## Build all binaries (unified + individual services + agentctl)
build-all: build build-agentctl
	@echo "Building individual services..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agent-manager$(EXE) ./cmd/agent-manager

## Build the agentctl sidecar binary (includes embedded MCP server). Carries
## $(CGO_LOCALE) because agentctl imports cgo (unlike mock-agent/acpdbg/winjob),
## so its native `go build` invokes gcc; see the CGO_LOCALE note above.
build-agentctl:
	@echo "Building agentctl..."
	@$(call MKDIR,$(BUILD_DIR))
	$(CGO_LOCALE) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl$(EXE) ./cmd/agentctl

## Build only the agentctl helpers needed by the development profile.
build-agentctl-dev: $(DEV_AGENTCTL_TARGETS)

## Build all remote agentctl helpers used by SSH/Docker/Sprites executors.
build-agentctl-remote: build-agentctl-linux build-agentctl-linux-arm64 build-agentctl-darwin-arm64 build-agentctl-darwin-amd64

## Build agentctl for linux/amd64 (used by Docker/Sprites and Linux SSH hosts)
build-agentctl-linux:
	@echo "Building agentctl for linux/amd64..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO_LINUX_AMD64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-linux-amd64 ./cmd/agentctl

## Build agentctl for linux/arm64 (used by ARM Linux SSH hosts, e.g. Graviton)
build-agentctl-linux-arm64:
	@echo "Building agentctl for linux/arm64..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO_LINUX_ARM64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-linux-arm64 ./cmd/agentctl

## Build agentctl for darwin/arm64 (used by Apple Silicon SSH hosts)
build-agentctl-darwin-arm64:
	@echo "Building agentctl for darwin/arm64..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO_DARWIN_ARM64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-darwin-arm64 ./cmd/agentctl
	$(call adhoc_sign_darwin,$(BUILD_DIR)/agentctl-darwin-arm64)

## Build agentctl for darwin/amd64 (used by Intel Mac SSH hosts)
build-agentctl-darwin-amd64:
	@echo "Building agentctl for darwin/amd64..."
	@$(call MKDIR,$(BUILD_DIR))
	$(GO_DARWIN_AMD64) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/agentctl-darwin-amd64 ./cmd/agentctl
	$(call adhoc_sign_darwin,$(BUILD_DIR)/agentctl-darwin-amd64)

## Build mock-agent for linux/amd64 (used by Docker E2E tests)
build-mock-agent-linux:
	@echo "Building mock-agent for linux/amd64..."
	@$(call MKDIR,$(BUILD_DIR))
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/mock-agent-linux-amd64 ./cmd/mock-agent


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
	@$(call MKDIR,$(FIXTURE_STAGE_DIR)/server)
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
	$(RUN_BIN) __backend

## Run in development mode (with pprof enabled)
dev: build-dev
	@echo "Running $(BINARY_NAME) in development mode..."
	@$(DEV_ENV) $(EXEC) $(RUN_BIN) __backend

## Run in debug mode (pprof + debug logging)
start-debug: build
	@echo "Running $(BINARY_NAME) in debug mode (pprof + debug logging)..."
	@$(DEBUG_ENV) $(EXEC) $(RUN_BIN) __backend

## Run tests
test:
	@echo "Running tests..."
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 ./...

## Run only the Windows-sensitive packages — mirrors the test-windows job in
## .github/workflows/backend-tests.yml. The broader suite uses Unix-only
## shell commands in test fixtures and would need GOOS guards to be portable;
## this narrow scope is the regression coverage for Windows-specific behavior.
## Tests that genuinely cannot run on Windows skip themselves via runtime.GOOS
## guards in source.
test-windows:
	@echo "Running Windows-sensitive packages..."
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/agentctl/server/process/... ./internal/agent/runtime/agentctl/launcher/... ./internal/backendapp/ownershiplock/...
	@echo "Running Windows managed runtime cache tests..."
	# Scoped to the Windows-tagged test only: untagged tests in this package use os.Symlink
	# which requires Developer Mode on Windows. Add new //go:build windows tests here.
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/agent/managedruntime -run TestRemoveNpxExecutionTreeTreatsMissingWindowsChildAsAbsent
	@echo "Running Windows port-collision tests..."
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/common/netutil -run TestIsAddrInUse
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/agentctl/server/instance -run TestAllocatePortAndListenerRetriesAddressInUse
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v ./internal/gateway/websocket -run TestResolveAndBindReportsOccupiedPort

## Verify run/dev/start-debug expand to the right per-shell command line across
## native Windows (cmd), Git Bash/MSYS, and Unix — the Make-target dispatch the
## Go CI jobs don't exercise. The harness is a bash script, so it runs under Git
## Bash/WSL or the Linux CI; on native cmd/PowerShell it prints a skip note.
check-make-shells:
	$(CHECK_SHELLS)

## Run E2E tests (real agents, costs money — manual only)
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
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -race -count=$(LIFECYCLE_GOLEAK_COUNT) -timeout 600s ./internal/agent/runtime/lifecycle/ ./internal/agent/runtime/agentctl/

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(CGO_LOCALE) $(CGO_PREFIX) $(GO) test -tags fts5 -v -coverprofile=coverage.out ./...
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
	@$(call MKDIR,$(PROTO_TOOLS_BIN))
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	cd proto && PATH="$(PROTO_TOOLS_BIN):$$PATH" $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

## Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## Report unreachable functions in the host production Go program.
# deadcode results are specific to one GOOS/GOARCH/build-tag configuration.
# This target intentionally audits the host configuration only; verify every
# supported target that could reach a finding before removing reported code.
deadcode:
	@echo "Scanning for unreachable Go functions (host configuration only; verify other targets before deleting findings; toolchain=$(DEADCODE_GO_TOOLCHAIN), override with DEADCODE_GO_TOOLCHAIN=go1.XY.Z)..."
	$(CGO_LOCALE) $(DEADCODE_ENV) $(GO) run golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION) -tags fts5 ./...

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
	@$(call MKDIR,$(BUILD_DIR))
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
	@echo "  check-make-shells  Assert run/dev/start-debug expand correctly per shell (make -n)"
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
