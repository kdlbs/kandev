# Kandev Root Makefile
# Orchestrates both backend (Go) and web app (Next.js)

# Directories
BACKEND_DIR := apps/backend
WEB_DIR := apps/web
APPS_DIR := apps

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
	@echo "  dev              Run backend + web via CLI (auto ports)"
	@echo "  dev-backend      Run backend in development mode (port 8080)"
	@echo "  dev-web          Run web app in development mode (port 3000)"
	@echo "  dev              Note: Uses apps/cli launcher (auto ports)"
	@echo ""
	@echo "Production Commands:"
	@echo "  start            Install deps, build, and start backend + web in production mode"
	@echo "  start-verbose    Start in production mode with info logs from backend + web"
	@echo "  start VERBOSE=1  Same as start-verbose"
	@echo ""
	@echo "Build Commands:"
	@echo "  build            Build backend and web app"
	@echo "  build-backend    Build backend binary"
	@echo "  build-web        Build web app for production"
	@echo ""
	@echo "Installation:"
	@echo "  install          Install all dependencies (backend + web)"
	@echo "  install-backend  Install backend dependencies"
	@echo "  install-web      Install web dependencies (uses pnpm workspace)"
	@echo ""
	@echo "Testing:"
	@echo "  test             Run all tests (backend + web + cli)"
	@echo "  test-backend     Run backend tests"
	@echo "  test-web         Run web app tests"
	@echo "  test-cli         Run CLI tests"
	@echo "  test-e2e         Run E2E tests (headless, parallel)"
	@echo "  test-e2e-headed  Run E2E tests with visible browser"
	@echo "  test-e2e-ui      Run E2E tests in Playwright UI mode"
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

.PHONY: dev
dev:
	@echo "Building Linux agentctl for remote executors..."
	@$(MAKE) -C $(BACKEND_DIR) build-agentctl-linux
	@echo "Launching via CLI (auto ports)..."
	@cd $(APPS_DIR) && $(PNPM) -C cli dev -- dev

.PHONY: dev-backend
dev-backend:
	@echo "Starting backend on http://localhost:8080"
	@trap 'stty sane 2>/dev/null || true' EXIT INT TERM; \
	$(MAKE) -C $(BACKEND_DIR) run; \
	stty sane 2>/dev/null || true

.PHONY: dev-web
dev-web:
	@echo "Starting web app on http://localhost:3000"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web dev

#
# Build
#

.PHONY: build
build: build-backend build-web
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
	@$(MAKE) -s build-backend-quiet
	@$(MAKE) -s build-web-quiet
	$(call success,Build complete)
	$(call phase,Starting Server)
	@cd $(APPS_DIR) && $(PNPM) -C cli dev -- start $(if $(filter 1 true yes,$(VERBOSE)),--verbose,) $(if $(filter 1 true yes,$(DEBUG)),--debug,)

.PHONY: start-verbose
start-verbose:
	@$(MAKE) start VERBOSE=1

.PHONY: start-debug
start-debug:
	@$(MAKE) start DEBUG=1

.PHONY: build-backend
build-backend:
	@printf "$(CYAN)Building backend...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) build

.PHONY: build-backend-quiet
build-backend-quiet:
	@printf "  $(DIM)Backend$(RESET)\n"
	@$(MAKE) -s -C $(BACKEND_DIR) build >/dev/null 2>&1

.PHONY: build-web
build-web:
	@printf "$(CYAN)Building web app...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web build

.PHONY: build-web-quiet
build-web-quiet:
	@printf "  $(DIM)Web app$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web build 2>&1 | grep -v "Warning:" | grep -v "parseLineType" | grep -v "^$$" || true

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
	@cd $(APPS_DIR) && $(PNPM) install --silent 2>/dev/null || cd $(APPS_DIR) && $(PNPM) install
	@printf "$(CYAN)Installing Playwright browsers...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web exec playwright install chromium

#
# Testing
#

.PHONY: test
test: test-backend test-web test-cli
	@printf "\n$(GREEN)$(BOLD)✓ All tests complete!$(RESET)\n"

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

.PHONY: test-e2e
test-e2e: build-backend build-web
	@printf "$(CYAN)Running E2E tests (headless, parallel)...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e

.PHONY: test-e2e-headed
test-e2e-headed: build-backend build-web
	@printf "$(CYAN)Running E2E tests (headed)...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e:headed

.PHONY: test-e2e-ui
test-e2e-ui: build-backend build-web
	@printf "$(CYAN)Opening Playwright UI mode...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web e2e:ui

.PHONY: test-e2e-report
test-e2e-report:
	@cd $(WEB_DIR) && npx playwright show-report e2e/playwright-report

#
# Code Quality
#

.PHONY: lint
lint: lint-backend lint-web
	@printf "\n$(GREEN)$(BOLD)✓ Linting complete!$(RESET)\n"

.PHONY: lint-backend
lint-backend:
	@printf "$(CYAN)Linting backend...$(RESET)\n"
	@$(MAKE) -C $(BACKEND_DIR) lint

.PHONY: lint-web
lint-web:
	@printf "$(CYAN)Linting web app...$(RESET)\n"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web lint

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
	@$(RMDIR) $(WEB_DIR)/.next $(APPS_DIR)/node_modules
	@$(RMDIR) $(APPS_DIR)/packages/*/node_modules

.PHONY: clean-db
clean-db:
	@printf "$(CYAN)Removing local SQLite database...$(RESET)\n"
	@$(RM) kandev.db kandev.db-wal kandev.db-shm \
		$(BACKEND_DIR)/kandev.db $(BACKEND_DIR)/kandev.db-wal $(BACKEND_DIR)/kandev.db-shm
