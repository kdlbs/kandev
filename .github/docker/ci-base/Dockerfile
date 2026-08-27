# Kandev CI base image — split into three tags.
#
# Three stages published as separate tags on the same GHCR repo:
#
#   - `runtime` (~700 MB): Playwright + Node + pnpm + Docker CLI. Used by the
#     `e2e` shards and `e2e-report` — they consume pre-built backend artifacts
#     and don't need the Go toolchain. Smaller pull = faster shard startup
#     (~20s per shard saved across 10 shards).
#
#   - `build`  (~1.3 GB): runtime + Go + golangci-lint + go-licenses. Used by
#     the `build` job (compiles backend + web), `backend-tests`, and
#     `frontend-tests`. These all need the Go toolchain.
#
#   - `desktop`: runtime + Tauri Linux dependencies + Rust. Used by the
#     desktop smoke job, so hosted-runner package mirrors are not in its setup.
#
# Pinned versions (keep in sync with apps/backend/go.mod, apps/web/package.json,
# and the matching workflow inputs):
#   - Playwright base : v1.61.1-noble  (matches @playwright/test ^1.61.1)
#   - Go              : 1.26.0         (matches apps/backend/go.mod)
#   - Node            : 24             (matches actions/setup-node value)
#   - pnpm            : 9.15.9         (matches pnpm/action-setup value)
#   - golangci-lint   : v2.9.0         (matches v2.9 floating pin)
#   - go-licenses     : v1.6.0
#   - Docker CLI      : 27.3.1
#   - Rust            : 1.97.1         (pinned for reproducible desktop CI)

# =============================================================================
# Stage 1: runtime — Playwright + Node + pnpm + Docker CLI (no Go)
# =============================================================================
FROM mcr.microsoft.com/playwright:v1.61.1-noble AS runtime

ENV DEBIAN_FRONTEND=noninteractive \
    PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH

# --- System packages -------------------------------------------------------
# Docker CLI is fetched as a static binary below to avoid pulling the daemon.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        git \
        make \
        jq \
        tar \
        unzip \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

# --- Node 24 (replaces the base image's Node 22) ---------------------------
RUN curl -fsSL https://deb.nodesource.com/setup_24.x -o /tmp/nodesource_setup.sh \
    && bash /tmp/nodesource_setup.sh \
    && rm /tmp/nodesource_setup.sh \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# --- pnpm 9.15.9 via corepack ---------------------------------------------
RUN corepack enable \
    && corepack prepare pnpm@9.15.9 --activate

# --- Docker CLI (no daemon) ------------------------------------------------
RUN curl -fsSL https://download.docker.com/linux/static/stable/x86_64/docker-27.3.1.tgz \
        -o /tmp/docker.tgz \
    && tar -xzf /tmp/docker.tgz -C /tmp \
    && mv /tmp/docker/docker /usr/local/bin/docker \
    && rm -rf /tmp/docker /tmp/docker.tgz

WORKDIR /work

# --- Runtime smoke checks --------------------------------------------------
RUN set -eux; \
    node --version | grep -E '^v24\.'; \
    pnpm --version | grep -E '^9\.15\.9$'; \
    docker --version | grep -F '27.3.1'

# =============================================================================
# Stage 2: build — runtime + Go + golangci-lint + go-licenses
# =============================================================================
FROM runtime AS build

ENV GOPATH=/root/go \
    PATH=/pnpm:/usr/local/go/bin:/root/go/bin:$PATH

# build-essential is only needed by Go's CGO path (sqlite via fts5 tag).
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential \
    && rm -rf /var/lib/apt/lists/*

# --- Go 1.26.0 -------------------------------------------------------------
RUN curl -fsSL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz \
        -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz

# --- golangci-lint v2.9.0 --------------------------------------------------
RUN curl -fsSL \
        https://raw.githubusercontent.com/golangci/golangci-lint/v2.9.0/install.sh \
        -o /tmp/install-golangci-lint.sh \
    && sh /tmp/install-golangci-lint.sh -b /usr/local/bin v2.9.0 \
    && rm /tmp/install-golangci-lint.sh

# --- go-licenses v1.6.0 ----------------------------------------------------
RUN go install github.com/google/go-licenses@v1.6.0

# --- Build smoke checks ----------------------------------------------------
RUN set -eux; \
    go version | grep -F 'go1.26.0'; \
    golangci-lint --version | grep -F '2.9.0'; \
    go version -m "$(which go-licenses)" | grep -F 'v1.6.0'

# =============================================================================
# Stage 3: desktop — runtime + Tauri Linux dependencies + Rust
# =============================================================================
FROM runtime AS desktop

ARG RUST_VERSION=1.97.1

ENV CARGO_HOME=/root/.cargo \
    RUSTUP_HOME=/root/.rustup \
    PATH=/pnpm:/root/.cargo/bin:$PATH

# Keep hosted-runner package mirrors out of the desktop smoke job. This stage
# also contains the compiler and rpm tooling used by the Tauri bundle step.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        libglib2.0-dev \
        libwebkit2gtk-4.1-dev \
        libgtk-3-dev \
        libayatana-appindicator3-dev \
        librsvg2-dev \
        patchelf \
        rpm \
        xvfb \
    && rm -rf /var/lib/apt/lists/*

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
        | sh -s -- -y --default-toolchain none --profile minimal \
    && rustup toolchain install "${RUST_VERSION}" --profile minimal \
    && rustup default "${RUST_VERSION}"

# --- Desktop smoke checks --------------------------------------------------
RUN set -eux; \
    rustc --version | grep -F "${RUST_VERSION}"; \
    cargo --version; \
    pkg-config --exists webkit2gtk-4.1; \
    command -v patchelf; \
    command -v xvfb-run
