# Kandev CI base image.
#
# Bundles every tool the CI workflows need so jobs can run via `container:`
# without re-downloading Playwright browsers, Go, Node, pnpm, golangci-lint,
# and go-licenses on every run.
#
# Pinned versions (keep in sync with the matching workflow inputs and
# apps/backend/go.mod, apps/web/package.json):
#   - Playwright base : v1.58.2-noble  (matches @playwright/test ^1.58.2)
#   - Go              : 1.26.0         (matches apps/backend/go.mod)
#   - Node            : 24             (matches actions/setup-node value)
#   - pnpm            : 9.15.9         (matches pnpm/action-setup value)
#   - golangci-lint   : v2.9.0         (matches v2.9 floating pin)

FROM mcr.microsoft.com/playwright:v1.58.2-noble

ENV DEBIAN_FRONTEND=noninteractive \
    PNPM_HOME=/pnpm \
    GOPATH=/root/go \
    PATH=/pnpm:/usr/local/go/bin:/root/go/bin:$PATH

# --- System packages -------------------------------------------------------
# Docker CLI is fetched as a static binary below to avoid pulling the daemon.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        git \
        make \
        build-essential \
        jq \
        tar \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

# --- Node 24 (replaces the base image's Node 22) ---------------------------
RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# --- pnpm 9.15.9 via corepack ---------------------------------------------
RUN corepack enable \
    && corepack prepare pnpm@9.15.9 --activate

# --- Go 1.26.0 -------------------------------------------------------------
RUN curl -fsSL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz \
        -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz

# --- golangci-lint v2.9.0 --------------------------------------------------
# Fetch the install script from the matching tagged ref (not `master`) so the
# script itself is reproducible across rebuilds.
RUN curl -fsSL https://raw.githubusercontent.com/golangci/golangci-lint/v2.9.0/install.sh \
        | sh -s -- -b /usr/local/bin v2.9.0

# --- go-licenses v1.6.0 ----------------------------------------------------
RUN go install github.com/google/go-licenses@v1.6.0

# --- Docker CLI (no daemon) ------------------------------------------------
# Static binary release lets jobs talk to a mounted host socket without
# dragging containerd/dockerd into the image.
RUN curl -fsSL https://download.docker.com/linux/static/stable/x86_64/docker-27.3.1.tgz \
        -o /tmp/docker.tgz \
    && tar -xzf /tmp/docker.tgz -C /tmp \
    && mv /tmp/docker/docker /usr/local/bin/docker \
    && rm -rf /tmp/docker /tmp/docker.tgz

WORKDIR /work

# --- Smoke checks ----------------------------------------------------------
# Fail the build if any pinned tool is missing or mismatched.
RUN set -eux; \
    go version | grep -F 'go1.26.0'; \
    node --version | grep -E '^v24\.'; \
    pnpm --version | grep -E '^9\.15\.9$'; \
    npx playwright --version; \
    golangci-lint --version | grep -F '2.9.0'; \
    go-licenses --help >/dev/null; \
    docker --version
