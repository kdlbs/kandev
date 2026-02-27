# Kandev Server Dockerfile
# Multi-stage build: Go backend + Next.js frontend + CLI launcher
#
# Build:
#   docker build -t kandev:latest .
#
# Run:
#   docker run -p 8080:8080 -p 3000:3000 -v kandev-data:/data kandev:latest

# ---------------------------------------------------------------------------
# Stage 1: Go builder — compile kandev + agentctl binaries
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS go-builder

WORKDIR /build

# Cache Go module downloads
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download

# Copy backend source and build
COPY apps/backend/ ./

RUN go build -ldflags "-s -w" -o /out/kandev ./cmd/kandev && \
    go build -ldflags "-s -w" -o /out/agentctl ./cmd/agentctl

# ---------------------------------------------------------------------------
# Stage 2: Web + CLI builder — build Next.js standalone + CLI
# ---------------------------------------------------------------------------
FROM node:24-slim AS web-builder

RUN corepack enable && corepack prepare pnpm@latest --activate

WORKDIR /build/apps

# Copy workspace config and all package.json files for dependency caching
COPY apps/package.json apps/pnpm-workspace.yaml apps/pnpm-lock.yaml ./
COPY apps/web/package.json ./web/package.json
COPY apps/cli/package.json ./cli/package.json
COPY apps/packages/ui/package.json ./packages/ui/package.json
COPY apps/packages/theme/package.json ./packages/theme/package.json
COPY apps/packages/types/package.json ./packages/types/package.json

RUN pnpm install --frozen-lockfile

# Copy full source for build
COPY apps/ ./

# Build shared packages, web app, and CLI
RUN pnpm --filter @kandev/web build && \
    pnpm --filter kandev build

# ---------------------------------------------------------------------------
# Stage 3: Runtime — minimal image with both services
# ---------------------------------------------------------------------------
FROM node:24-bookworm-slim AS runtime

# Install only essential runtime dependencies, then clean up
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        ca-certificates \
        tini && \
    rm -rf /var/lib/apt/lists/*

# Create app directory structure matching what the CLI expects:
#   /app/apps/backend/bin/kandev
#   /app/apps/web/.next/standalone/web/server.js
RUN mkdir -p /app/apps/backend/bin /app/apps/web/.next /data/worktrees

# Copy Go binaries
COPY --from=go-builder /out/kandev /app/apps/backend/bin/kandev
COPY --from=go-builder /out/agentctl /usr/local/bin/agentctl

# Copy Next.js standalone output
COPY --from=web-builder /build/apps/web/.next/standalone/ /app/apps/web/.next/standalone/

# Copy static assets and public directory into standalone output
COPY --from=web-builder /build/apps/web/.next/static/ /app/apps/web/.next/standalone/web/.next/static/
COPY --from=web-builder /build/apps/web/public/ /app/apps/web/.next/standalone/web/public/

# Install CLI: copy built output + package.json, install prod deps, link binary
COPY --from=web-builder /build/apps/cli/dist/ /usr/local/lib/kandev-cli/dist/
COPY --from=web-builder /build/apps/cli/bin/ /usr/local/lib/kandev-cli/bin/
COPY --from=web-builder /build/apps/cli/package.json /usr/local/lib/kandev-cli/
RUN cd /usr/local/lib/kandev-cli && npm install --omit=dev && \
    chmod +x bin/cli.js && \
    ln -s /usr/local/lib/kandev-cli/bin/cli.js /usr/local/bin/kandev

# Data directory for SQLite and worktrees
VOLUME ["/data"]

# Environment defaults for containerized operation
ENV KANDEV_NO_BROWSER=1 \
    KANDEV_DATABASE_PATH=/data/kandev.db \
    KANDEV_WORKTREE_BASEPATH=/data/worktrees \
    KANDEV_DOCKER_ENABLED=false \
    HOSTNAME=0.0.0.0 \
    NODE_ENV=production

# Run as non-root
RUN chown -R node:node /app /data
USER node

WORKDIR /app

EXPOSE 8080 3000

# tini as PID 1 for signal handling; CLI manages backend + web processes
ENTRYPOINT ["tini", "--"]
CMD ["kandev", "start", "--backend-port", "8080", "--web-port", "3000", "--verbose"]
