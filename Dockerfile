# Kandev Server Dockerfile
#
# Single-stage build that consumes prebuilt artifacts. The release workflow
# (.github/workflows/release.yml) extracts the per-arch release bundle
# (`kandev-linux-{x64,arm64}.tar.gz`) into the build context, then this file
# just COPYs the native binaries into the runtime layout.
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
# is not set. Azure CLI + azure-devops extension support agentctl Azure Repos
# PR creation (az repos pr create). apprise is installed via pipx for
# notification fan-out.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        gh \
        ca-certificates \
        curl \
        gosu \
        tini \
        python3 \
        python3-venv \
        pipx && \
    curl -sL https://aka.ms/InstallAzureCLIDeb | bash && \
    rm -rf /var/lib/apt/lists/* && \
    PIPX_HOME=/opt/pipx PIPX_BIN_DIR=/usr/local/bin pipx install apprise

# Replace the node base image's default user so we own uid 1000.
# Home is placed under /data so agent CLI auth state (gh, claude, codex, auggie,
# copilot, amp, ...) lives on the PV and survives pod restarts and image upgrades.
RUN userdel -r node && groupadd -r kandev && useradd -r -g kandev -u 1000 -d /data/home -M kandev

# Install azure-devops extension under the runtime user's Azure config dir on /data.
RUN mkdir -p /data/home/.azure && \
    AZURE_CONFIG_DIR=/data/home/.azure az extension add --name azure-devops && \
    chown -R kandev:kandev /data/home/.azure

# Create app directory structure matching what `kandev start` expects:
#   /app/apps/backend/bin/kandev
RUN mkdir -p /app/apps/backend/bin /data/worktrees

# Build context layout (prepared by the release workflow from the extracted
# bundle tarball):
#   bundle/bin/kandev                  - native Go backend (per-arch)
#   bundle/bin/agentctl                - native agentctl (per-arch)
#   bundle/bin/agentctl-linux-amd64    - amd64 agentctl helper (always amd64)
#
# The bundle's `bin/agentctl-linux-amd64` variant is bind-mounted into
# Docker-executor sandboxes by the lifecycle manager; ship it next to kandev
# so the AgentctlResolver finds it without manual configuration.
COPY bundle/bin/kandev               /app/apps/backend/bin/kandev
COPY bundle/bin/agentctl-linux-amd64 /app/apps/backend/bin/agentctl-linux-amd64
COPY bundle/bin/agentctl             /usr/local/bin/agentctl
COPY docker-entrypoint.sh            /usr/local/bin/docker-entrypoint.sh

# Re-apply executable bits stripped by tar/COPY edge cases, then link the native
# launcher onto PATH.
RUN chmod +x \
        /app/apps/backend/bin/kandev \
        /app/apps/backend/bin/agentctl-linux-amd64 \
        /usr/local/bin/agentctl \
        /usr/local/bin/docker-entrypoint.sh && \
    ln -s /app/apps/backend/bin/kandev /usr/local/bin/kandev && \
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

# Only the backend port is exposed; the Go backend serves API, WebSocket, and
# embedded SPA assets on the same listener.
EXPOSE 38429

# tini as PID 1 for signal handling; entrypoint handles privilege drop
ENTRYPOINT ["tini", "--", "docker-entrypoint.sh"]
CMD ["kandev", "start", "--backend-port", "38429", "--verbose"]
