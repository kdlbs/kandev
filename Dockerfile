# Kandev Server Dockerfile
#
# Single-stage build that consumes prebuilt artifacts. The release workflow
# (.github/workflows/release.yml) extracts the per-arch release bundle
# (`kandev-linux-{x64,arm64}.tar.gz`) into the build context, then this file
# just COPYs the binaries + web standalone + CLI into the runtime layout.
#
# Building this file outside CI (manual `docker build .`) will fail because
# the `bundle/` directory isn't present in the build context. To build
# locally, extract a release tarball into ./ctx/bundle/ alongside
# ./ctx/docker-entrypoint.sh and run:
#   docker build -f Dockerfile ./ctx
#
# Run:
#   docker run -p 38429:38429 -v kandev-data:/data ghcr.io/kdlbs/kandev:latest

FROM node:24-bookworm-slim

# Install runtime dependencies. gh is included because the GitHub integration
# (PR review, webhooks) shells out to it for auth fallback when GITHUB_TOKEN
# is not set. apprise is installed via pipx for notification fan-out.
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

# Create app directory structure matching what `kandev start` expects:
#   /app/apps/backend/bin/kandev
#   /app/apps/web/.next/standalone/web/server.js
RUN mkdir -p /app/apps/backend/bin /app/apps/web/.next/standalone /usr/local/lib/kandev-cli /data/worktrees

# Build context layout (prepared by the release workflow from the extracted
# bundle tarball):
#   bundle/bin/kandev                  - native Go backend (per-arch)
#   bundle/bin/agentctl                - native agentctl (per-arch)
#   bundle/bin/agentctl-linux-amd64    - amd64 agentctl helper (always amd64)
#   bundle/web/...                     - Next.js standalone + static + public
#   bundle/cli/bin/cli.js              - CLI entrypoint (calls cli.bundle.js)
#   bundle/cli/dist/cli.bundle.js      - self-contained CLI bundle (deps inlined)
#   bundle/cli/package.json            - package metadata (version, etc.)
#
# The bundle's `bin/agentctl-linux-amd64` variant is bind-mounted into
# Docker-executor sandboxes by the lifecycle manager; ship it next to kandev
# so the AgentctlResolver finds it without manual configuration.
COPY bundle/bin/kandev               /app/apps/backend/bin/kandev
COPY bundle/bin/agentctl-linux-amd64 /app/apps/backend/bin/agentctl-linux-amd64
COPY bundle/bin/agentctl             /usr/local/bin/agentctl
COPY bundle/web/                     /app/apps/web/.next/standalone/
COPY bundle/cli/                     /usr/local/lib/kandev-cli/
COPY docker-entrypoint.sh            /usr/local/bin/docker-entrypoint.sh

# Re-apply executable bits stripped by tar/COPY edge cases, then link the
# CLI launcher onto PATH. The CLI's deps are inlined into dist/cli.bundle.js
# by the release bundle step, so no `npm install` is needed here.
RUN chmod +x \
        /app/apps/backend/bin/kandev \
        /app/apps/backend/bin/agentctl-linux-amd64 \
        /usr/local/bin/agentctl \
        /usr/local/lib/kandev-cli/bin/cli.js \
        /usr/local/bin/docker-entrypoint.sh && \
    ln -s /usr/local/lib/kandev-cli/bin/cli.js /usr/local/bin/kandev && \
    chown -R kandev:kandev /app /data

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

WORKDIR /app

# Only the backend port is exposed — it reverse-proxies the Next.js frontend
# (which listens on 37429 internally), so users hit a single port.
EXPOSE 38429

# tini as PID 1 for signal handling; entrypoint handles privilege drop
ENTRYPOINT ["tini", "--", "docker-entrypoint.sh"]
CMD ["kandev", "start", "--backend-port", "38429", "--web-port", "37429", "--verbose"]
