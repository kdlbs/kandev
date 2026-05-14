# Kandev Server Dockerfile
# Multi-stage build: Go backend + Next.js frontend + CLI launcher
#
# Build:
#   docker build -t kandev:latest .
#
# Run:
#   docker run -p 38429:38429 -v kandev-data:/data kandev:latest

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
    go build -ldflags "-s -w" -o /out/agentctl ./cmd/agentctl && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/agentctl-linux-amd64 ./cmd/agentctl

# ---------------------------------------------------------------------------
# Stage 2: Web + CLI builder — build Next.js standalone + CLI
# ---------------------------------------------------------------------------
FROM node:24-slim AS web-builder

ARG PNPM_VERSION=9.15.9
RUN corepack enable && corepack prepare pnpm@${PNPM_VERSION} --activate

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

# CHANGELOG.md lives at the repo root in source; the web build scripts
# (generate-changelog.mjs, generate-release-notes.mjs) resolve it via
# `${WEB_ROOT}/../../CHANGELOG.md`. Mirror that layout inside the builder.
COPY CHANGELOG.md /build/CHANGELOG.md

# Build shared packages, web app, and CLI
RUN pnpm --filter @kandev/web build && \
    pnpm --filter kandev build

# ---------------------------------------------------------------------------
# Stage 3: Runtime — minimal image with both services
# ---------------------------------------------------------------------------
FROM node:24-bookworm-slim AS runtime

# Install only essential runtime dependencies, then clean up.
# gh is included because the GitHub integration (PR review, webhooks) shells out
# to it for auth fallback when GITHUB_TOKEN is not set.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        gh \
        ca-certificates \
        gosu \
        tini \
        python3 \
        python3-venv \
        pipx && \
    rm -rf /var/lib/apt/lists/* && \
    PIPX_HOME=/opt/pipx PIPX_BIN_DIR=/usr/local/bin pipx install apprise

# Replace the node base image's default user so we own uid 1000.
# Home is placed under /data so agent CLI auth state (gh, claude, codex, auggie,
# copilot, amp, ...) lives on the PV and survives pod restarts and image upgrades.
RUN userdel -r node && groupadd -r kandev && useradd -r -g kandev -u 1000 -d /data/home -M kandev

# Create app directory structure matching what the CLI expects:
#   /app/apps/backend/bin/kandev
#   /app/apps/web/.next/standalone/web/server.js
RUN mkdir -p /app/apps/backend/bin /app/apps/web/.next /data/worktrees

# Copy Go binaries. The -linux-amd64 variant of agentctl is bind-mounted into
# Docker-executor sandboxes by the lifecycle manager; ship it next to kandev
# so the AgentctlResolver finds it without manual configuration.
COPY --from=go-builder /out/kandev /app/apps/backend/bin/kandev
COPY --from=go-builder /out/agentctl-linux-amd64 /app/apps/backend/bin/agentctl-linux-amd64
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

# Kandev home directory (DB, worktrees, sessions, repos)
VOLUME ["/data"]

# Environment defaults for containerized operation.
# NPM_CONFIG_PREFIX points npm global installs at the PV so user-installed
# agent CLIs (claude-code, codex, auggie, ...) survive pod restarts.
ENV KANDEV_NO_BROWSER=1 \
    KANDEV_HOME_DIR=/data \
    KANDEV_DOCKER_ENABLED=false \
    HOME=/data/home \
    NPM_CONFIG_PREFIX=/data/.npm-global \
    PATH=/data/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    HOSTNAME=0.0.0.0 \
    NODE_ENV=production

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Set build-time ownership of /app and /data
RUN chown -R kandev:kandev /app /data

WORKDIR /app

# Only the backend port is exposed — it reverse-proxies the Next.js frontend
# (which listens on 37429 internally), so users hit a single port.
EXPOSE 38429

# tini as PID 1 for signal handling; entrypoint handles privilege drop
ENTRYPOINT ["tini", "--", "docker-entrypoint.sh"]
CMD ["kandev", "start", "--backend-port", "38429", "--web-port", "37429", "--verbose"]
