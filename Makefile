# Kandev Root Makefile
# Orchestrates both backend (Go) and web app (Vite/React)

# Directories
BACKEND_DIR := apps/backend
WEB_DIR := apps/web
APPS_DIR := apps
DESKTOP_DIR := apps/desktop
DESKTOP_RUNTIME_DIR := $(DESKTOP_DIR)/src-tauri/resources/kandev
EMBEDDED_WEB_DIR := $(BACKEND_DIR)/internal/webapp/embedded/generated

# Tools
PNPM := pnpm
GOFLAGS ?= -v
MAKE := make

# Cross-platform commands
ifeq ($(OS),Windows_NT)
  RM = cmd /c del /s /q
  RMDIR = cmd /c rmdir /s /q
  # Go emits a `.exe` for GOOS=windows regardless of the calling shell, so
  # native cmd/PowerShell AND Git Bash both need it (mirrors apps/backend/Makefile).
  EXE = .exe
else
  RM = rm -f
  RMDIR = rm -rf
  EXE =
endif

# stderr redirect for $(shell ...) probes. Keyed on $(OS)$(MSYSTEM) rather than
# $(OS) alone: Git Bash sets OS=Windows_NT yet runs commands through sh, so it
# needs the POSIX path. Under native cmd there is no /dev/null, and cmd resolves
# redirections before running the command — `2>/dev/null` would abort the probe
# and silently force its fallback. Holds the whole token so callers can blank it.
# Mirrors apps/backend/Makefile.
ifeq ($(OS)$(MSYSTEM),Windows_NT)
  NULL_REDIR := 2>NUL
else
  NULL_REDIR := 2>/dev/null
endif

# Colors for terminal output
RESET := \033[0m
BOLD := \033[1m
DIM := \033[2m
GREEN := \033[32m
BLUE := \033[34m
CYAN := \033[36m
YELLOW := \033[33m
MAGENTA := \033[35m

VERBOSE ?= 0
NODE ?= $(shell command -v node $(NULL_REDIR) || echo node)
RUNTIME_BUNDLE_DIR ?= $(CURDIR)/dist/kandev
RUNTIME_VERSION ?= $(shell git describe --tags --always --dirty $(NULL_REDIR) || echo dev)
SERVICE_BUNDLE_DIR ?= $(CURDIR)/dist/kandev
SERVICE_LAUNCHER = $(SERVICE_BUNDLE_DIR)/bin/kandev
SERVICE_VERSION ?= $(RUNTIME_VERSION)
SERVICE_ENV = KANDEV_BUNDLE_DIR="$(SERVICE_BUNDLE_DIR)" KANDEV_VERSION="$(SERVICE_VERSION)"
PORT_FLAG := $(if $(PORT),--port $(PORT),)
SERVICE_HOME_DIR_FLAG := $(if $(HOME_DIR),--home-dir "$(HOME_DIR)",)
SERVICE_NO_BOOT_START_FLAG := $(if $(filter 1 true yes,$(NO_BOOT_START)),--no-boot-start,)
SERVICE_INSTALL_FLAGS := $(PORT_FLAG) $(SERVICE_HOME_DIR_FLAG) $(SERVICE_NO_BOOT_START_FLAG)
DEV_WEB_PORT_FLAG := $(if $(WEB_PORT),--web-internal-port $(WEB_PORT),)
DEV_FLAGS := $(PORT_FLAG) $(DEV_WEB_PORT_FLAG) $(DEV_ARGS)
# $(if …) does not strip its condition, so an all-whitespace DEV_FLAGS reads as
# true. Test and print this instead, and forward DEV_FLAGS itself — stripping
# what reaches the CLI would collapse whitespace inside a quoted DEV_ARGS value.
DEV_FLAGS_DISPLAY := $(strip $(DEV_FLAGS))
DESKTOP_BUNDLES ?= dmg

# Phase headers
define phase
	@printf "\n$(BOLD)$(BLUE)━━━ $(1) ━━━$(RESET)\n\n"
endef

# Success message
define success
	@printf "$(GREEN)✓$(RESET) $(1)\n"
endef

# Default target
.DEFAULT_GOAL := help

#
# Help
#

.PHONY: help
help:
	@echo "Kandev - AI Agent Kanban Board"
	@echo ""
	@echo "Development Commands:"
	@echo "  bootstrap        Install mise tools, workspace deps, and git hooks"
	@echo "  bootstrap-e2e    Bootstrap plus Playwright browser/system deps"
	@echo "  dev              Run backend + web via the native Go launcher (auto ports)"
	@echo "  dev PORT=38430 WEB_PORT=37430   PORT beats KANDEV_BACKEND_PORT/KANDEV_PORT, WEB_PORT beats KANDEV_WEB_PORT"
	@echo "  dev DEV_ARGS='--verbose'        Pass extra flags through to the native launcher"
	@echo "  dev-prod-db      Run dev mode against the production db (KANDEV_DATABASE_PATH, else KANDEV_HOME_DIR, else ~/.kandev)"
	@echo "  dev-backend      Run backend in development mode (port 38429)"
	@echo "  dev-web          Run web app in development mode (port 37429)"
	@echo "  desktop-dev      Run macOS Tauri app in dev mode with bundled runtime"
	@echo "  doctor           Idempotently wire up pre-commit hooks (runs automatically before dev)"
	@echo ""
	@echo "Production Commands:"
	@echo "  start            Install deps, build, and start backend + web in production mode"
	@echo "  start-verbose    Start in production mode with info logs from backend + web"
	@echo "  start VERBOSE=1  Same as start-verbose"
	@echo "  start-windows    cmd.exe-safe start for native Windows (no printf/find/cp/exec)"
	@echo ""
	@echo "Service Commands:"
	@echo "  service-install          Install deps, build current checkout, install user service"
	@echo "  service-install-system   Install deps, build current checkout, install system service"
	@echo "  service-status           Show current user service status"
	@echo "  service-logs             Show current user service logs"
	@echo "  service-logs-follow      Follow current user service logs"
	@echo "  service-start            Start current user service"
	@echo "  service-stop             Stop current user service"
	@echo "  service-restart          Restart current user service"
	@echo "  service-uninstall        Uninstall current user service"
	@echo "  service-config           Show service launcher/config paths"
	@echo "  service-install PORT=3000 HOME_DIR=/path  Optional install overrides"
	@echo "  service-install NO_BOOT_START=1  Skip Linux user-service boot hint"
	@echo ""
	@echo "Build Commands:"
	@echo "  build            Build backend and web app"
	@echo "  build-backend    Build backend binary"
	@echo "  build-web        Build web app for production"
	@echo "  runtime-bundle   Build the package-manager runtime bundle (deps must exist)"
	@echo "  desktop-runtime  Build/copy runtime resources for the macOS desktop app"
	@echo "  desktop-build    Build the macOS Tauri app bundle/DMG"
	@echo "  desktop-open     Build and open the macOS app"
	@echo "  desktop-launch   Alias for desktop-open"
	@echo ""
	@echo "Installation:"
	@echo "  install          Install all dependencies (backend + web)"
	@echo "  install-backend  Install backend dependencies"
	@echo "  install-web      Install web dependencies (uses pnpm workspace)"
	@echo ""
	@echo "Testing:"
	@echo "  test             Run all tests (backend + web + cli)"
	@echo "  test-windows     Run Windows-clean subset (curated backend + web + cli)"
	@echo "  test-backend     Run backend tests"
	@echo "  test-web         Run web app tests"
	@echo "  test-cli         Run CLI tests"
	@echo "  test-e2e         Run E2E tests (headless, parallel)"
	@echo "  test-e2e-headed  Run E2E tests with visible browser"
	@echo "  test-e2e-ui      Run E2E tests in Playwright UI mode"
	@echo "  test-e2e-ci      Run E2E tests in Docker with CI-like Linux + resource limits"
	@echo "  test-e2e-report  Open Playwright HTML report"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint             Run linters for both components"
	@echo "  lint-backend     Run Go linters"
	@echo "  lint-web         Run ESLint"
	@echo "  lint-architecture  Enforce architecture budgets and compatibility expiry"
	@echo "  lint-format      Check formatting with Prettier (web/cli/packages)"
	@echo "  dead-code-workspaces Find unused TypeScript workspace files, exports, and dependencies"
	@echo "  dead-code-go     Find unreachable Go functions (host config; verify other targets before deletion)"
	@echo "  fmt              Format all code"
	@echo "  fmt-backend      Format Go code"
	@echo "  fmt-web          Format web/cli/packages with Prettier, then ESLint --fix (web)"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean            Remove all build artifacts"
	@echo "  clean-backend    Remove backend build artifacts"
	@echo "  clean-web        Remove web build artifacts"
	@echo "  clean-db         Remove local SQLite database"

#
# Development
#

.PHONY: bootstrap
bootstrap:
	@scripts/bootstrap-dev-env

.PHONY: bootstrap-e2e
bootstrap-e2e:
	@scripts/bootstrap-dev-env --with-e2e

.PHONY: doctor
doctor:
# Native Windows (cmd/PowerShell) has no POSIX shell for the bash-only doctor
# script. Under Git Bash/MSYS ($(MSYSTEM) is set) it runs fine, so skip only
# when OS is Windows_NT AND MSYSTEM is empty — the concatenation equals
# "Windows_NT" only in that native case.
ifeq ($(OS)$(MSYSTEM),Windows_NT)
	@echo pre-commit hooks skipped on native Windows - run scripts/doctor from Git Bash to enable
else
	@scripts/doctor
endif

.PHONY: dev
dev: doctor
	# POSIX-only (cp/exec): on Windows run from Git Bash/MSYS, like `make start`.
	@echo "Building dev launcher..."
	@$(MAKE) -C $(BACKEND_DIR) build-kandev
	@cp $(BACKEND_DIR)/bin/kandev$(EXE) $(BACKEND_DIR)/bin/kandev-launcher$(EXE)
	@echo "Launching via native Go launcher$(if $(DEV_FLAGS_DISPLAY), ($(DEV_FLAGS_DISPLAY)), (auto ports))..."
	@exec $(BACKEND_DIR)/bin/kandev-launcher$(EXE) dev $(DEV_FLAGS)

.PHONY: dev-prod-db
# Resolve the production db the way the launcher would
# (resolveDatabasePath/resolveHomeDir in apps/backend/internal/launcher/constants.go):
# an environment KANDEV_DATABASE_PATH wins, then KANDEV_HOME_DIR, then
# $HOME/.kandev. A plain makefile assignment outranks the environment in GNU
# Make, so the earlier `:=` of the $HOME default ignored a relocated install and
# ran dev mode against the wrong db while the launcher still reported backing up
# the production one.
#
# `?=` does not express this either: Make counts a variable the shell exported
# blank as defined, so `?=` would forward that blank onward and the launcher —
# which trims before testing for empty — would quietly fall back to the isolated
# .kandev-dev db while this target announced production.
#
# $(if $(strip …),…) tests emptiness on the trimmed value but substitutes the
# original, so a blank falls through to the next source while a real path keeps
# the internal spaces $(strip …) would otherwise collapse. Export the Make
# variable so both environment and command-line assignments reach $(shell).
# Trim only the outer padding before the database suffix is appended.
export KANDEV_HOME_DIR
KANDEV_PROD_HOME_DIR := $(if $(strip $(KANDEV_HOME_DIR)),$(shell printf '%s' "$$KANDEV_HOME_DIR" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$$//'),$(HOME)/.kandev)
KANDEV_PROD_DB_PATH := $(if $(strip $(KANDEV_DATABASE_PATH)),$(KANDEV_DATABASE_PATH),$(KANDEV_PROD_HOME_DIR)/data/kandev.db)
dev-prod-db: export KANDEV_DATABASE_PATH := $(KANDEV_PROD_DB_PATH)
dev-prod-db:
	@echo "⚠  dev mode against PRODUCTION db at $(KANDEV_PROD_DB_PATH)"
	@$(MAKE) dev

.PHONY: dev-backend
dev-backend:
	@echo "Starting backend on http://localhost:38429"
	@trap 'stty sane 2>/dev/null || true' EXIT INT TERM; \
	$(MAKE) -C $(BACKEND_DIR) run; \
	stty sane 2>/dev/null || true

.PHONY: dev-web
dev-web:
	@echo "Starting web app on http://localhost:37429"
	@cd $(APPS_DIR) && PORT=37429 $(PNPM) --filter @kandev/web dev

.PHONY: desktop-runtime
desktop-runtime:
	@test "$$(uname -s)" = "Darwin" || { echo "desktop-* targets require macOS."; exit 1; }
	@$(MAKE) -s service-bundle
	@platform="$(DESKTOP_PLATFORM)"; \
	if [ -z "$$platform" ]; then \
		case "$$(uname -m)" in \
			arm64|aarch64) platform="macos-arm64" ;; \
			x86_64|amd64) platform="macos-x64" ;; \
			*) echo "Unsupported macOS architecture: $$(uname -m)" >&2; exit 1 ;; \
		esac; \
	fi; \
	scripts/release/prepare-desktop-runtime.sh \
		--bundle-dir "$(SERVICE_BUNDLE_DIR)" \
		--platform "$$platform" \
		--output-dir "$(DESKTOP_RUNTIME_DIR)"

.PHONY: desktop-dev
desktop-dev: desktop-runtime
	@KANDEV_DESKTOP_RUNTIME_DIR="$(CURDIR)/$(DESKTOP_RUNTIME_DIR)" \
		$(PNPM) -C $(APPS_DIR) --filter @kandev/desktop dev

.PHONY: desktop-build
desktop-build: desktop-runtime
	@cd $(DESKTOP_DIR) && $(PNPM) tauri build --features desktop-runtime --bundles "$(DESKTOP_BUNDLES)"

.PHONY: desktop-open
desktop-open: desktop-build
	@app_path="$$(find "$(DESKTOP_DIR)/src-tauri/target" -path '*/release/bundle/macos/Kandev.app' -print -quit)"; \
	if [ -z "$$app_path" ]; then \
		echo "Missing built app under $(DESKTOP_DIR)/src-tauri/target"; \
		exit 1; \
	fi; \
	open "$$app_path"

.PHONY: desktop-launch
desktop-launch: desktop-open

#
# Build
#

.PHONY: build
build: build-web sync-embedded-web build-backend
	@printf "\n$(GREEN)$(BOLD)✓ Build complete!$(RESET)\n"

#
# Production Start
#

.PHONY: start
start:
	$(call phase,Installing Dependencies)
	@$(MAKE) -s install-backend
	@$(MAKE) -s install-web
	$(call success,Dependencies installed)
	$(call phase,Building)
	@$(MAKE) -s build-web-quiet
	@$(MAKE) -s sync-embedded-web
	@$(MAKE) -s build-backend-quiet
	$(call success,Build complete)
	$(call phase,Starting Server)
	@exec $(BACKEND_DIR)/bin/kandev start $(if $(filter 1 true yes,$(VERBOSE)),--verbose,) $(if $(filter 1 true yes,$(DEBUG)),--debug,)

.PHONY: start-verbose
start-verbose:
	@$(MAKE) start VERBOSE=1

.PHONY: start-debug
start-debug:
	@$(MAKE) start DEBUG=1

# Windows-native production start. Mirrors `start` but avoids the Unix-only
# tooling that target relies on (printf, find, cp, exec) and would break under
# cmd.exe. Run it from a shell where GNU Make invokes cmd.exe — the default on
# Windows when sh.exe is not on PATH — because the backend build uses cmd's
# `set VAR=VAL&` env-var syntax. Skips the Playwright browser install (only
# needed for e2e, not to run the server).
.PHONY: start-windows
start-windows:
	@echo Installing backend dependencies...
	@$(MAKE) -s -C $(BACKEND_DIR) deps
	@echo Installing web dependencies...
	@cd $(APPS_DIR) && $(PNPM) install
	@echo Building web app...
	@cd $(APPS_DIR) && set "VITE_KANDEV_API_PORT=" && set "VITE_KANDEV_DEBUG=" && $(PNPM) --filter @kandev/web build
	@$(MAKE) -s sync-embedded-web-windows
	@echo Building backend...
	@$(MAKE) -s -C $(BACKEND_DIR) build
	@echo Starting server...
	@"$(subst /,\,$(BACKEND_DIR)/bin/kandev.exe)" start $(if $(filter 1 true yes,$(VERBOSE)),--verbose,) $(if $(filter 1 true yes,$(DEBUG)),--debug,)

# Windows-native counterparts of start-verbose / start-debug. Same cmd.exe
# constraints as start-windows.
.PHONY: start-windows-verbose
start-windows-verbose:
	@$(MAKE) start-windows VERBOSE=1

.PHONY: start-windows-debug
start-windows-debug:
	@$(MAKE) start-windows DEBUG=1

#
# Service
#

.PHONY: runtime-bundle
runtime-bundle:
	$(call phase,Packaging Runtime Bundle)
	@test -n "$(RUNTIME_BUNDLE_DIR)" || { echo "RUNTIME_BUNDLE_DIR is empty; aborting."; exit 1; }
	@test "$(RUNTIME_BUNDLE_DIR)" != "/" || { echo "RUNTIME_BUNDLE_DIR must not be /; aborting."; exit 1; }
	@$(MAKE) -s build-web
	@$(MAKE) -s sync-embedded-web
	@$(MAKE) -C $(BACKEND_DIR) build-runtime VERSION="$(RUNTIME_VERSION)" GOFLAGS="$(GOFLAGS)"
	@set -eu; \
		requested_bundle_dir="$(RUNTIME_BUNDLE_DIR)"; \
		mkdir -p "$$requested_bundle_dir"; \
		resolved_bundle_dir="$$(cd "$$requested_bundle_dir" && pwd -P)"; \
		test -n "$$resolved_bundle_dir" || { echo "RUNTIME_BUNDLE_DIR could not be resolved; aborting."; exit 1; }; \
		test "$$resolved_bundle_dir" != "/" || { echo "RUNTIME_BUNDLE_DIR must not resolve to /; aborting."; exit 1; }; \
		staging_bundle_dir="$$(mktemp -d "$$resolved_bundle_dir/.runtime-bundle.XXXXXX")"; \
		trap 'rm -rf "$$staging_bundle_dir"' EXIT; \
		mkdir -p "$$staging_bundle_dir/bin"; \
		cp "$(BACKEND_DIR)/bin/kandev" "$(BACKEND_DIR)/bin/agentctl" \
		"$(BACKEND_DIR)/bin/agentctl-linux-amd64" \
		"$(BACKEND_DIR)/bin/agentctl-linux-arm64" \
		"$(BACKEND_DIR)/bin/agentctl-darwin-arm64" \
		"$(BACKEND_DIR)/bin/agentctl-darwin-amd64" \
		"$$staging_bundle_dir/bin/"; \
		scripts/release/package-bundle.sh --bundle-dir "$$staging_bundle_dir"; \
		rm -rf "$$resolved_bundle_dir/bin"; \
		mv "$$staging_bundle_dir/bin" "$$resolved_bundle_dir/bin"; \
		rmdir "$$staging_bundle_dir"; \
		trap - EXIT
	$(call success,Runtime bundle packaged at $(RUNTIME_BUNDLE_DIR))

.PHONY: service-bundle
service-bundle: install
	@$(MAKE) -s runtime-bundle \
		RUNTIME_BUNDLE_DIR="$(SERVICE_BUNDLE_DIR)" \
		RUNTIME_VERSION="$(SERVICE_VERSION)"

.PHONY: service-cli-check
service-cli-check:
	@test -f "$(SERVICE_LAUNCHER)" || { echo "Missing $(SERVICE_LAUNCHER). Run 'make service-install' first."; exit 1; }

.PHONY: service-install
service-install: service-bundle
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service install $(SERVICE_INSTALL_FLAGS)

.PHONY: service-install-system
service-install-system: service-bundle
	@sudo env $(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service install --system $(SERVICE_INSTALL_FLAGS)

.PHONY: service-uninstall service-start service-stop service-restart service-status service-logs service-logs-follow service-config
service-uninstall: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service uninstall

service-start: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service start

service-stop: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service stop

service-restart: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service restart

service-status: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service status

service-logs: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service logs

service-logs-follow: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service logs -f

service-config: service-cli-check
	@$(SERVICE_ENV) "$(SERVICE_LAUNCHER)" service config

.PHONY: build-backend
build-backend:
	@printf "$(CYAN)Building backend...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) build

.PHONY: build-backend-remote-helpers build-backend-linux-helpers
build-backend-remote-helpers:
	@printf "$(CYAN)Building remote helper binaries (agentctl helpers + mock-agent) for executor E2E...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) build-agentctl-remote build-mock-agent-linux

build-backend-linux-helpers: build-backend-remote-helpers

.PHONY: acpdbg
acpdbg:
	@$(MAKE) -s -C $(BACKEND_DIR) acpdbg ARGS="$(ARGS)"

.PHONY: build-backend-quiet
build-backend-quiet:
	@printf "  $(DIM)Backend$(RESET)\n"
	@$(MAKE) -s -C $(BACKEND_DIR) build >/dev/null 2>&1

## Package the plugin-fixture SDK plugin used by tests/plugins/plugins.spec.ts.
## e2e/global-setup.ts checks the resulting tar.gz exists (like it does for
## the kandev/mock-agent binaries) but does not build it itself — this target
## is the "make it exist" step, wired into test-e2e* below.
.PHONY: build-e2e-plugin-package
build-e2e-plugin-package:
	@printf "$(CYAN)Packaging e2e fixture plugin...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) e2e-plugin-package

.PHONY: build-web
build-web:
	@printf "$(CYAN)Building web app...$(RESET)\n"
	@cd $(APPS_DIR) && VITE_KANDEV_API_PORT= VITE_KANDEV_DEBUG= $(PNPM) --filter @kandev/web build

## Web build for the E2E harness. Identical to build-web except that it keeps the
## pseudo QA catalog, which a production build drops (see
## apps/web/lib/i18n/bundling.ts). e2e/tests/i18n/pseudo-coverage.spec.ts is the
## only oracle for copy the jsx-only eslint guard cannot see, and it needs the
## catalog present in the artifact it runs against.
.PHONY: build-web-e2e
build-web-e2e:
	@printf "$(CYAN)Building web app (with the pseudo QA locale)...$(RESET)\n"
	@cd $(APPS_DIR) && VITE_KANDEV_API_PORT= VITE_KANDEV_DEBUG= $(PNPM) --filter @kandev/web build:e2e

.PHONY: build-web-quiet
build-web-quiet:
	@printf "  $(DIM)Web app$(RESET)\n"
	@cd $(APPS_DIR) && VITE_KANDEV_API_PORT= VITE_KANDEV_DEBUG= $(PNPM) --filter @kandev/web build 2>&1 | grep -v "Warning:" | grep -v "parseLineType" | grep -v "^$$" || true

.PHONY: sync-embedded-web
sync-embedded-web:
	@test -f "$(WEB_DIR)/dist/index.html" || { echo "Missing $(WEB_DIR)/dist/index.html; run 'make build-web' first."; exit 1; }
	@mkdir -p "$(EMBEDDED_WEB_DIR)"
	@find "$(EMBEDDED_WEB_DIR)" -mindepth 1 ! -name .gitignore ! -name keep.txt -exec rm -rf {} +
	@cp -R "$(WEB_DIR)/dist/." "$(EMBEDDED_WEB_DIR)/"
	@printf "  $(DIM)Embedded web assets$(RESET)\n"

# cmd.exe-safe counterpart of sync-embedded-web (used by start-windows).
# robocopy /MIR mirrors the Vite dist into the embedded dir; /XF keeps the
# committed .gitignore and keep.txt while purging stale generated assets.
# robocopy exit codes below 8 all mean success, so normalize them to 0 —
# otherwise make reads robocopy's "files copied" code (1) as a failure.
.PHONY: sync-embedded-web-windows
sync-embedded-web-windows:
	@if not exist "$(subst /,\,$(WEB_DIR)/dist/index.html)" (echo Missing $(WEB_DIR)/dist/index.html - run 'make start-windows' to build everything. & exit /b 1)
	@robocopy "$(subst /,\,$(WEB_DIR)/dist)" "$(subst /,\,$(EMBEDDED_WEB_DIR))" /MIR /XF .gitignore keep.txt /NFL /NDL /NJH /NJS /NC /NS >nul & if errorlevel 8 (exit /b 1) else (exit /b 0)
	@echo   Embedded web assets synced.

#
# Installation
#

.PHONY: install
install: install-backend install-web
	@printf "\n$(GREEN)$(BOLD)✓ All dependencies installed!$(RESET)\n"

.PHONY: install-backend
install-backend:
	@printf "$(CYAN)Installing backend dependencies...$(RESET)\n"
	@$(MAKE) -s -C $(BACKEND_DIR) deps

.PHONY: install-web
install-web:
	@printf "$(CYAN)Installing web dependencies...$(RESET)\n"
	@(cd $(APPS_DIR) && $(PNPM) install --silent 2>/dev/null) || (cd $(APPS_DIR) && $(PNPM) install)
	@printf "$(CYAN)Installing Playwright browsers...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web exec playwright install chromium

#
# Testing
#

.PHONY: test
test: test-backend test-web test-cli test-scripts
	@printf "\n$(GREEN)$(BOLD)✓ All tests complete!$(RESET)\n"

# Curated Windows-clean test run. Mirrors the test-windows job in
# .github/workflows/backend-tests.yml: the backend portion skips ~24 tests
# with Unix-only fixtures (sleep/cat/echo in test inputs, POSIX symlinks,
# delete-while-open). Web and CLI use vitest, which is cross-platform.
# Shrink the backend skip list as fixtures get cleaned up.
#
# Deliberately uses plain `echo` and inlines pnpm invocations (rather than
# depending on test-backend/test-web/test-cli) so it does not pull in the
# `@printf` and `$(shell uname ...)` calls used by other targets — those
# fail on cmd.exe (no printf.exe, no uname.exe) and would break the run on
# Windows even though they are cosmetic on Unix.
.PHONY: test-windows
test-windows:
	@echo "[backend] Running Windows-clean subset..."
	@$(MAKE) -C $(BACKEND_DIR) test-windows
	@echo "[web] Running tests..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web test
	@echo "[cli] Running tests..."
	@cd $(APPS_DIR) && $(PNPM) --filter kandev test
	@echo "Windows-clean test subset complete."

.PHONY: test-sprites-e2e
test-sprites-e2e:
	@$(MAKE) -C $(BACKEND_DIR) test-sprites-e2e

.PHONY: test-backend
test-backend:
	@printf "$(CYAN)Running backend tests...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) test

.PHONY: test-web
test-web:
	@printf "$(CYAN)Running web app tests...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web test

.PHONY: test-cli
test-cli:
	@printf "$(CYAN)Running CLI tests...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter kandev test

.PHONY: test-scripts
test-scripts:
	@printf "$(CYAN)Running script tests...$(RESET)\n"
	@python3 .github/scripts/lint-action-pinning_test.py
	@bash scripts/pr-state.test.sh
	@bash scripts/pr-await.test.sh
	@bash scripts/run-quiet.test.sh
	@bash scripts/dev-prod-db-path.test.sh
	@bash scripts/opencode-code-review.test.sh
	@python3 scripts/opencode-code-review.test.py
	@python3 scripts/lint-harness-files.test.py
	@python3 scripts/lint-spec-files.test.py
	@python3 scripts/lint-architecture.test.py
	@python3 scripts/playwright-blob-audit.test.py
	@bash scripts/release-desktop.test.sh
	@bash scripts/release/runtime-bundle.test.sh
	@bash scripts/release/retry-ghcr-command.test.sh
	@node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs
	@python3 .github/scripts/release-workflow-contract_test.py
	@node --test scripts/release/nightly-version.test.mjs scripts/release/nightly-release.test.mjs scripts/release/npm-view-version.test.mjs scripts/release/publish-npm.test.mjs scripts/release/update-scoop-bucket.test.mjs
	@node --test scripts/validate-public-docs.test.mjs scripts/generic-plugin-host-boundary.test.mjs

.PHONY: test-e2e
test-e2e: build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package
	@printf "$(CYAN)Running E2E tests (headless, parallel, managed runner)...$(RESET)\n"
	@cd $(WEB_DIR) && status=0; for project in routing auth chromium mobile-chrome containers; do \
		printf "$(CYAN)-- project: $$project --$(RESET)\n"; \
		e2e/scripts/run-e2e.sh --host --no-build --no-strict --shards 1 --project "$$project" -- --output="e2e/test-results-$$project" || status=1; \
	done; exit $$status

.PHONY: test-e2e-headed
test-e2e-headed: build-backend build-web-e2e build-e2e-plugin-package
	@printf "$(CYAN)Running E2E tests (headed)...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e:headed

.PHONY: test-e2e-ui
test-e2e-ui: build-backend build-web-e2e build-e2e-plugin-package
	@printf "$(CYAN)Opening Playwright UI mode...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e:ui

.PHONY: test-e2e-report
test-e2e-report:
	@cd $(WEB_DIR) && npx playwright show-report e2e/playwright-report

# Run E2E tests inside Docker to simulate CI conditions (Linux + resource limits).
# Configurable via env vars; defaults match GitHub Actions ubuntu-latest runners.
E2E_CI_CPUS ?= 4
E2E_CI_MEMORY ?= 16g
E2E_CI_SHM_SIZE ?= 1g
E2E_CI_IMAGE ?= kandev-e2e

.PHONY: test-e2e-ci
test-e2e-ci:
	@printf "$(CYAN)Building E2E Docker image...$(RESET)\n"
	@docker build -f e2e.Dockerfile -t $(E2E_CI_IMAGE) .
	@printf "$(CYAN)Running E2E tests in Docker (cpus=$(E2E_CI_CPUS), memory=$(E2E_CI_MEMORY))...$(RESET)\n"
	@docker run --rm \
		--cpus=$(E2E_CI_CPUS) \
		--memory=$(E2E_CI_MEMORY) \
		--shm-size=$(E2E_CI_SHM_SIZE) \
		$(E2E_CI_IMAGE) $(E2E_ARGS)

#
# Code Quality
#

.PHONY: lint
lint: lint-backend lint-web lint-harness lint-specs lint-architecture
	@printf "\n$(GREEN)$(BOLD)✓ Linting complete!$(RESET)\n"

.PHONY: lint-backend
lint-backend:
	@printf "$(CYAN)Linting backend...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) lint

.PHONY: lint-web
lint-web:
	@printf "$(CYAN)Linting web app...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web lint

.PHONY: lint-harness
lint-harness:
	@printf "$(CYAN)Linting harness files...$(RESET)\n"
	@python3 .github/scripts/lint-harness-files.py --all

.PHONY: lint-specs
lint-specs:
	@printf "$(CYAN)Linting specification files...$(RESET)\n"
	@python3 scripts/lint-spec-files.py --all

.PHONY: lint-architecture
lint-architecture:
	@printf "$(CYAN)Linting architecture...$(RESET)\n"
	@python3 scripts/lint-architecture.py --all

.PHONY: lint-format
lint-format:
	@printf "$(CYAN)Checking formatting...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) run format:check

.PHONY: dead-code-workspaces
dead-code-workspaces:
	@printf "$(CYAN)Auditing TypeScript workspace dead code...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) run dead-code

.PHONY: dead-code-go
dead-code-go:
	@printf "$(CYAN)Auditing Go dead code...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) deadcode

.PHONY: fmt
fmt: fmt-backend fmt-web
	@printf "\n$(GREEN)$(BOLD)✓ Code formatting complete!$(RESET)\n"

.PHONY: fmt-backend
fmt-backend:
	@printf "$(CYAN)Formatting backend code...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) fmt

.PHONY: fmt-web
fmt-web:
	@printf "$(CYAN)Formatting web code...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) run format

.PHONY: typecheck-web
typecheck-web:
	@printf "$(CYAN)Type-checking web app...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web exec tsc -p tsconfig.json --noEmit

.PHONY: typecheck
typecheck:
	@printf "$(CYAN)Type-checking all apps...$(RESET)\n"
	# apps/cli has no tsconfig (publish-only shim), so the workspace must be
	# listed explicitly — add new TypeScript packages here.
	@cd $(APPS_DIR) && $(PNPM) -r --filter @kandev/web --filter @kandev/desktop --filter @kandev/theme --filter @kandev/types --filter @kandev/ui exec tsc -p tsconfig.json --noEmit

#
# Cleanup
#

.PHONY: clean
clean: clean-backend clean-web
	@printf "\n$(GREEN)$(BOLD)✓ Cleanup complete!$(RESET)\n"

.PHONY: clean-backend
clean-backend:
	@printf "$(CYAN)Cleaning backend artifacts...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) clean

.PHONY: clean-web
clean-web:
	@printf "$(CYAN)Cleaning web artifacts...$(RESET)\n"
	@$(RMDIR) $(WEB_DIR)/dist $(WEB_DIR)/.next $(APPS_DIR)/node_modules
	@$(RMDIR) $(APPS_DIR)/packages/*/node_modules

.PHONY: clean-db
clean-db:
	@printf "$(CYAN)Removing dev database (.kandev-dev/)...$(RESET)\n"
	@$(RMDIR) .kandev-dev
