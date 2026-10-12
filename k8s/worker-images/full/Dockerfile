ARG DOCKER_IMAGE
ARG BASE_IMAGE
FROM ${DOCKER_IMAGE} AS docker_tools
FROM ${BASE_IMAGE}
USER root
ARG GOLANGCI_VERSION
ARG GOLANGCI_SHA256
ARG PLAYWRIGHT_VERSION
ARG PLAYWRIGHT_SHA256
ARG CHROMIUM_VERSION
ARG CHROMIUM_REVISION
ARG CHROMIUM_SHA256
COPY --from=docker_tools /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker_tools /usr/local/libexec/docker/cli-plugins/ /usr/local/libexec/docker/cli-plugins/
RUN set -eu; \
    curl -fsSL --max-time 180 "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_VERSION}/golangci-lint-${GOLANGCI_VERSION}-linux-amd64.tar.gz" -o /tmp/lint.tgz; \
    echo "${GOLANGCI_SHA256}  /tmp/lint.tgz" | sha256sum -c -; \
    tar -xzf /tmp/lint.tgz -C /tmp; \
    cp "/tmp/golangci-lint-${GOLANGCI_VERSION}-linux-amd64/golangci-lint" /usr/local/bin/golangci-lint; \
    rm -rf /tmp/lint.tgz "/tmp/golangci-lint-${GOLANGCI_VERSION}-linux-amd64"
RUN set -eu; \
    mkdir -p /opt/full-worker/playwright-core /opt/full-worker/browsers; \
    curl -fsSL --max-time 180 "https://registry.npmjs.org/playwright-core/-/playwright-core-${PLAYWRIGHT_VERSION}.tgz" -o /tmp/playwright.tgz; \
    echo "${PLAYWRIGHT_SHA256}  /tmp/playwright.tgz" | sha256sum -c -; \
    tar -xzf /tmp/playwright.tgz --strip-components=1 -C /opt/full-worker/playwright-core; \
    curl -fsSL --max-time 180 "https://cdn.playwright.dev/builds/cft/${CHROMIUM_VERSION}/linux64/chrome-headless-shell-linux64.zip" -o /tmp/chromium.zip; \
    echo "${CHROMIUM_SHA256}  /tmp/chromium.zip" | sha256sum -c -; \
    python3 -m zipfile -e /tmp/chromium.zip "/opt/full-worker/browsers/chromium_headless_shell-${CHROMIUM_REVISION}"; \
    chmod -R a+rX /opt/full-worker; \
    chmod a+rx "/opt/full-worker/browsers/chromium_headless_shell-${CHROMIUM_REVISION}/chrome-headless-shell-linux64/chrome-headless-shell"; \
    rm /tmp/playwright.tgz /tmp/chromium.zip
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/full-worker/browsers \
    npm_config_prefix=/workspace/.npm-global \
    npm_config_cache=/workspace/.cache/npm \
    PNPM_HOME=/workspace/.pnpm \
    npm_config_store_dir=/workspace/.pnpm-store \
    PIP_CACHE_DIR=/workspace/.cache/pip \
    GOCACHE=/workspace/.cache/go-build \
    GOMODCACHE=/workspace/.cache/go-mod \
    CARGO_HOME=/workspace/.cache/cargo \
    RUSTUP_HOME=/usr/local/rustup \
    GOMAXPROCS=2 \
    GOFLAGS=-p=1 \
    CARGO_BUILD_JOBS=2
ENV PATH=/workspace/.npm-global/bin:/workspace/.pnpm:/usr/local/cargo/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
COPY smoke.sh /opt/full-worker/smoke.sh
RUN chmod 0755 /opt/full-worker/smoke.sh
USER 1000:1000
WORKDIR /workspace
ENTRYPOINT []
CMD ["sleep", "infinity"]
