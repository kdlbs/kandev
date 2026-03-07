# e2e.Dockerfile — Run E2E tests in CI-like Linux environment with resource limits
#
# Usage:
#   make test-e2e-ci                                    # All tests
#   make test-e2e-ci E2E_ARGS="--shard=1/4"            # Single shard
#   make test-e2e-ci E2E_ARGS="tests/diff-update.spec.ts"  # Specific test
#   make test-e2e-ci E2E_CI_CPUS=2 E2E_CI_MEMORY=4g   # Custom limits
#
# Note: Builds for native architecture (ARM64 on Apple Silicon). To match CI
# exactly (x86_64), add --platform=linux/amd64, but expect ~10x slower builds
# due to QEMU emulation. The default is fine for reproducing timing/resource issues.

# ---------------------------------------------------------------------------
# Stage 1: Go builder — compile kandev + mock-agent binaries
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS go-builder

WORKDIR /build

COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download

COPY apps/backend/ ./

RUN go build -ldflags "-s -w" -o /out/kandev ./cmd/kandev && \
    go build -ldflags "-s -w" -o /out/agentctl ./cmd/agentctl && \
    go build -ldflags "-s -w" -o /out/mock-agent ./cmd/mock-agent

# ---------------------------------------------------------------------------
# Stage 2: Node builder — install deps + build web app
# ---------------------------------------------------------------------------
FROM node:24-bookworm AS node-builder

RUN corepack enable && corepack prepare pnpm@9 --activate

WORKDIR /build/apps

# Copy workspace config and package.jsons for dependency caching
COPY apps/package.json apps/pnpm-workspace.yaml apps/pnpm-lock.yaml ./
COPY apps/web/package.json ./web/package.json
COPY apps/cli/package.json ./cli/package.json
COPY apps/packages/ui/package.json ./packages/ui/package.json
COPY apps/packages/theme/package.json ./packages/theme/package.json
COPY apps/packages/types/package.json ./packages/types/package.json

RUN pnpm install --frozen-lockfile

# Copy full source for build
COPY apps/ ./

# Build web app (produces .next/standalone/ needed by E2E fixture)
RUN pnpm --filter @kandev/web build

# ---------------------------------------------------------------------------
# Stage 3: Test runner — everything needed to run E2E tests
# ---------------------------------------------------------------------------
FROM node:24-bookworm AS runner

RUN corepack enable && corepack prepare pnpm@9 --activate

# Install git (backend fixture creates test repos) and CA certs
RUN apt-get update && \
    apt-get install -y --no-install-recommends git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Prefer IPv4 over IPv6 for localhost — matches GitHub Actions ubuntu-latest behavior.
# Debian Bookworm's gai.conf defaults to IPv6-first, so Node's server.listen('localhost')
# binds to ::1 while undici's fetch('http://localhost:port') tries 127.0.0.1 → ECONNREFUSED.
RUN echo 'precedence ::ffff:0:0/96  100' >> /etc/gai.conf

WORKDIR /app

# Copy node_modules + source + built output from node-builder
COPY --from=node-builder /build/apps/ ./apps/

# Copy Go binaries into the location global-setup.ts expects
COPY --from=go-builder /out/kandev ./apps/backend/bin/kandev
COPY --from=go-builder /out/mock-agent ./apps/backend/bin/mock-agent
# agentctl must be in PATH — kandev spawns it as a subprocess
COPY --from=go-builder /out/agentctl /usr/local/bin/agentctl

# Create static/public symlinks for standalone server (mirrors global-setup.ts)
RUN mkdir -p /app/apps/web/.next/standalone/web/.next && \
    ln -sf /app/apps/web/.next/static /app/apps/web/.next/standalone/web/.next/static && \
    ln -sf /app/apps/web/public /app/apps/web/.next/standalone/web/public

# Store Playwright browsers in a shared location accessible by all users
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers

# Install Playwright chromium with all system dependencies
RUN cd /app/apps/web && npx playwright install --with-deps chromium

# Configure git system-wide so backend fixture's temp dirs inherit it
RUN git config --system user.name "E2E Test" && \
    git config --system user.email "e2e@test.local" && \
    git config --system commit.gpgsign false && \
    git config --system tag.gpgsign false && \
    git config --system init.defaultBranch main

# Use the built-in node user (uid 1000) as non-root user
RUN chown -R node:node /app

USER node

# CI=true enables retries=2 in playwright.config.ts
ENV CI=true

WORKDIR /app/apps/web

# --reporter=list overrides blob reporter for real-time terminal output.
# Extra args (shard, test file, --grep) can be appended via CMD.
ENTRYPOINT ["npx", "playwright", "test", "--config", "e2e/playwright.config.ts", "--reporter=list"]
CMD []
