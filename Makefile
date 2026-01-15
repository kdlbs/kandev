# Kandev Root Makefile
# Orchestrates both backend (Go) and web app (Next.js)

# Directories
BACKEND_DIR := apps/backend
WEB_DIR := apps/web
LANDING_DIR := apps/landing
APPS_DIR := apps

# Tools
PNPM := pnpm
MAKE := make

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
	@echo "  dev-backend      Run backend in development mode (port 8080)"
	@echo "  dev-web          Run web app in development mode (port 3000)"
	@echo "  dev-landing      Run landing page in development mode (port 3001)"
	@echo "  dev              Note: Run dev-backend and dev-web in separate terminals"
	@echo ""
	@echo "Build Commands:"
	@echo "  build            Build backend, web app, and landing page"
	@echo "  build-backend    Build backend binary"
	@echo "  build-web        Build web app for production"
	@echo "  build-landing    Build landing page for production"
	@echo ""
	@echo "Installation:"
	@echo "  install          Install all dependencies (backend + web + landing)"
	@echo "  install-backend  Install backend dependencies"
	@echo "  install-web      Install web dependencies (uses pnpm workspace)"
	@echo ""
	@echo "Testing:"
	@echo "  test             Run all tests (backend + web)"
	@echo "  test-backend     Run backend tests"
	@echo "  test-web         Run web app tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint             Run linters for both components"
	@echo "  lint-backend     Run Go linters"
	@echo "  lint-web         Run ESLint"
	@echo "  fmt              Format all code"
	@echo "  fmt-backend      Format Go code"
	@echo "  fmt-web          Format web code with ESLint"
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
	@echo "╔════════════════════════════════════════════════════════════════╗"
	@echo "║  TIP: For better log visibility, run in separate terminals:   ║"
	@echo "║                                                                ║"
	@echo "║    Terminal 1: make dev-backend                                ║"
	@echo "║    Terminal 2: make dev-web                                    ║"
	@echo "╚════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "Starting backend and web app..."
	@echo "Backend: http://localhost:8080"
	@echo "Web app: http://localhost:3000"
	@echo ""
	@bash -c 'set -euo pipefail; \
	$(MAKE) -C $(BACKEND_DIR) run & backend_pid=$$!; \
	cd $(APPS_DIR); $(PNPM) --filter @kandev/web dev & web_pid=$$!; \
	cleanup() { kill $$backend_pid $$web_pid 2>/dev/null || true; }; \
	trap cleanup EXIT INT TERM; \
	wait -n $$backend_pid $$web_pid; status=$$?; \
	if [ $$status -eq 0 ]; then status=1; fi; \
	exit $$status'

.PHONY: dev-backend
dev-backend:
	@echo "Starting backend on http://localhost:8080"
	@$(MAKE) -C $(BACKEND_DIR) run

.PHONY: dev-web
dev-web:
	@echo "Starting web app on http://localhost:3000"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web dev

.PHONY: dev-landing
dev-landing:
	@echo "Starting landing page on http://localhost:3001"
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/landing dev

#
# Build
#

.PHONY: build
build: build-backend build-web build-landing
	@echo ""
	@echo "✓ Build complete!"
	@echo "  Backend binary: $(BACKEND_DIR)/bin/kandev"
	@echo "  Web app: $(WEB_DIR)/.next"
	@echo "  Landing page: $(LANDING_DIR)/.next"

.PHONY: build-backend
build-backend:
	@echo "Building backend..."
	@$(MAKE) -C $(BACKEND_DIR) build

.PHONY: build-web
build-web:
	@echo "Building web app..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web build

.PHONY: build-landing
build-landing:
	@echo "Building landing page..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/landing build

#
# Installation
#

.PHONY: install
install: install-backend install-web
	@echo ""
	@echo "✓ All dependencies installed!"

.PHONY: install-backend
install-backend:
	@echo "Installing backend dependencies..."
	@$(MAKE) -C $(BACKEND_DIR) deps

.PHONY: install-web
install-web:
	@echo "Installing web and landing dependencies..."
	@cd $(APPS_DIR) && $(PNPM) install

#
# Testing
#

.PHONY: test
test: test-backend test-web
	@echo ""
	@echo "✓ All tests complete!"

.PHONY: test-backend
test-backend:
	@echo "Running backend tests..."
	@$(MAKE) -C $(BACKEND_DIR) test

.PHONY: test-web
test-web:
	@echo "Running web app tests..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web test

#
# Code Quality
#

.PHONY: lint
lint: lint-backend lint-web
	@echo ""
	@echo "✓ Linting complete!"

.PHONY: lint-backend
lint-backend:
	@echo "Linting backend..."
	@$(MAKE) -C $(BACKEND_DIR) lint

.PHONY: lint-web
lint-web:
	@echo "Linting web app..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web lint

.PHONY: fmt
fmt: fmt-backend fmt-web
	@echo ""
	@echo "✓ Code formatting complete!"

.PHONY: fmt-backend
fmt-backend:
	@echo "Formatting backend code..."
	@$(MAKE) -C $(BACKEND_DIR) fmt

.PHONY: fmt-web
fmt-web:
	@echo "Formatting web code..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web lint -- --fix || true

.PHONY: typecheck-web
typecheck-web:
	@echo "Type-checking web app..."
	@cd $(APPS_DIR) && $(PNPM) --filter @kandev/web exec tsc -p tsconfig.json --noEmit

.PHONY: typecheck
typecheck:
	@echo "Type-checking all apps..."
	@cd $(APPS_DIR) && $(PNPM) -r exec tsc -p tsconfig.json --noEmit

#
# Cleanup
#

.PHONY: clean
clean: clean-backend clean-web
	@echo ""
	@echo "✓ Cleanup complete!"

.PHONY: clean-backend
clean-backend:
	@echo "Cleaning backend artifacts..."
	@$(MAKE) -C $(BACKEND_DIR) clean

.PHONY: clean-web
clean-web:
	@echo "Cleaning web and landing artifacts..."
	@rm -rf $(WEB_DIR)/.next $(LANDING_DIR)/.next $(APPS_DIR)/node_modules
	@rm -rf $(APPS_DIR)/packages/*/node_modules

.PHONY: clean-db
clean-db:
	@echo "Removing local SQLite database..."
	@rm -f kandev.db $(BACKEND_DIR)/kandev.db
