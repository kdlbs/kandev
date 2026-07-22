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
MAKE := make

# Cross-platform commands
ifeq ($(OS),Windows_NT)
  RM = cmd /c del /s /q
  RMDIR = cmd /c rmdir /s /q
else
  RM = rm -f
  RMDIR = rm -rf
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
NODE ?= $(shell command -v node 2>/dev/null || echo node)
SERVICE_LAUNCHER := $(CURDIR)/dist/kandev/bin/kandev
SERVICE_BUNDLE_DIR := $(CURDIR)/dist/kandev
SERVICE_VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
SERVICE_ENV := KANDEV_BUNDLE_DIR="$(SERVICE_BUNDLE_DIR)" KANDEV_VERSION="$(SERVICE_VERSION)"
SERVICE_PORT_FLAG := $(if $(PORT),--port $(PORT),)
SERVICE_HOME_DIR_FLAG := $(if $(HOME_DIR),--home-dir "$(HOME_DIR)",)
SERVICE_NO_BOOT_START_FLAG := $(if $(filter 1 true yes,$(NO_BOOT_START)),--no-boot-start,)
SERVICE_INSTALL_FLAGS := $(SERVICE_PORT_FLAG) $(SERVICE_HOME_DIR_FLAG) $(SERVICE_NO_BOOT_START_FLAG)
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
	@echo "  dev              Run backend + web via local CLI (auto ports)"
	@echo "  dev-prod-db      Run dev mode against the production db at ~/.kandev"
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
	@echo "  lint-format      Check formatting with Prettier (web/cli/packages)"
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
ifeq ($(OS),Windows_NT)
	@echo pre-commit hooks skipped on native Windows - run scripts/doctor from Git Bash to enable
else
	@scripts/doctor
endif

.PHONY: dev
dev: doctor
	@echo "Building remote agentctl helpers..."
	@$(MAKE) -C $(BACKEND_DIR) build-agentctl-remote
ifeq ($(OS),Windows_NT)
	@echo "Building winjob (Ctrl-C-safe wrapper for Windows)..."
	@$(MAKE) -C $(BACKEND_DIR) build-winjob
endif
	@echo "Launching via CLI (auto ports)..."
	@cd $(APPS_DIR) && $(PNPM) -C cli dev -- dev

.PHONY: dev-prod-db
dev-prod-db: export KANDEV_DATABASE_PATH := $(HOME)/.kandev/data/kandev.db
dev-prod-db:
	@echo "⚠  dev mode against PRODUCTION db at $(KANDEV_DATABASE_PATH)"
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

#
# Service
#

.PHONY: service-bundle
service-bundle: install build
	$(call phase,Packaging Service Bundle)
	@test -n "$(SERVICE_BUNDLE_DIR)" || { echo "SERVICE_BUNDLE_DIR is empty; aborting."; exit 1; }
	@test "$(SERVICE_BUNDLE_DIR)" != "/" || { echo "SERVICE_BUNDLE_DIR must not be /; aborting."; exit 1; }
	@$(MAKE) -C $(BACKEND_DIR) build-agentctl-remote
	@$(RMDIR) "$(SERVICE_BUNDLE_DIR)/bin"
	@mkdir -p "$(SERVICE_BUNDLE_DIR)/bin"
	@cp "$(BACKEND_DIR)/bin/kandev" "$(BACKEND_DIR)/bin/agentctl" \
		"$(BACKEND_DIR)/bin/agentctl-linux-amd64" \
		"$(BACKEND_DIR)/bin/agentctl-linux-arm64" \
		"$(BACKEND_DIR)/bin/agentctl-darwin-arm64" \
		"$(BACKEND_DIR)/bin/agentctl-darwin-amd64" \
		"$(SERVICE_BUNDLE_DIR)/bin/"
	@scripts/release/package-bundle.sh
	$(call success,Service bundle packaged at $(SERVICE_BUNDLE_DIR))

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
	@bash scripts/run-quiet.test.sh
	@bash scripts/opencode-code-review.test.sh
	@python3 scripts/opencode-code-review.test.py
	@python3 scripts/lint-harness-files.test.py
	@bash scripts/release-desktop.test.sh
	@node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs
	@node --test scripts/validate-public-docs.test.mjs

.PHONY: test-e2e
test-e2e: build-backend build-web build-e2e-plugin-package
	@printf "$(CYAN)Running E2E tests (headless, parallel)...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e

.PHONY: test-e2e-headed
test-e2e-headed: build-backend build-web build-e2e-plugin-package
	@printf "$(CYAN)Running E2E tests (headed)...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e:headed

.PHONY: test-e2e-ui
test-e2e-ui: build-backend build-web build-e2e-plugin-package
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
lint: lint-backend lint-web lint-harness
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

.PHONY: lint-format
lint-format:
	@printf "$(CYAN)Checking formatting...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) run format:check

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
	@cd $(APPS_DIR) && $(PNPM) -r exec tsc -p tsconfig.json --noEmit

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
