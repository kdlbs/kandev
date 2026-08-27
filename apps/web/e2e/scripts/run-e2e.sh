#!/usr/bin/env bash
#
# Unified E2E runner. Builds what's needed, runs Playwright, and tears down
# cleanly — on the host or inside the kandev-ci docker image (the same one CI
# uses). Handles the sharp edges this would otherwise hit by hand:
#
#   • Auto-selects docker vs host (docker if the daemon is up AND the runtime
#     image is available; override with --docker / --host).
#   • Docker mode builds the backend on the HOST and runs it in the runtime
#     image — a host glibc that's the same or older than the image's (the usual
#     case) is forward-compatible, so no build image is needed. It smoke-tests
#     the binary in the image first and only falls back to the BUILD image
#     (KANDEV_CI_BUILD_IMAGE) if the host glibc is newer. FE Vite builds
#     on the host and are served by the Go backend.
#   • Runs N shards concurrently (--shards N): N isolated containers in docker
#     mode, or N host processes with distinct E2E_PORT_OFFSET + output dirs.
#   • Never leaves root-owned junk in the repo: points normal Playwright output
#     at a container-local dir. With CAPTURE_PR_ASSETS, captures are written to
#     the mounted workspace and ownership is restored after the run. `clean`
#     removes any pre-existing root-owned normal artifacts via a throwaway
#     container.
#
# Usage:
#   run-e2e.sh [run] [options] [-- <playwright args>]   # default subcommand: run
#   run-e2e.sh clean                                    # remove build/test artifacts (incl. root-owned)
#
# Options:
#   --docker | --host     Force runner (default: auto-detect)
#   --shards N            Run the chromium project as N concurrent shards
#   --no-build           Skip the build step (reuse existing artifacts)
#   --no-strict          Don't set KANDEV_E2E_WS_ASSERT=1 (it's on by default here, matching CI)
#   --project NAME       Playwright project (default: chromium)
#   Anything after `--` (or any unrecognized arg) is passed straight to Playwright,
#   e.g. tests/chat/foo.spec.ts, --grep "name", --repeat-each=3, --workers=1.
#
# Env overrides:
#   KANDEV_CI_BUILD_IMAGE   (default: ghcr.io/kdlbs/kandev-ci:build-latest)
#   KANDEV_CI_RUNTIME_IMAGE (default: kandev-ci:runtime-local, falls back to ghcr…:runtime-latest)
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
WEB_DIR="$REPO_ROOT/apps/web"
BACKEND_DIR="$REPO_ROOT/apps/backend"
BUILD_IMAGE="${KANDEV_CI_BUILD_IMAGE:-ghcr.io/kdlbs/kandev-ci:build-latest}"
RUNTIME_IMAGE="${KANDEV_CI_RUNTIME_IMAGE:-}"

log() { printf '\033[36m[e2e]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[31m[e2e] %s\033[0m\n' "$*" >&2; exit 1; }

docker_up() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }

resolve_runtime_image() {
  if [[ -n "$RUNTIME_IMAGE" ]]; then echo "$RUNTIME_IMAGE"; return; fi
  if docker image inspect kandev-ci:runtime-local >/dev/null 2>&1; then
    echo kandev-ci:runtime-local
  else
    echo ghcr.io/kdlbs/kandev-ci:runtime-latest
  fi
}

# --- clean: remove build/test artifacts, including root-owned ones from prior
# docker runs (host user can't rm root-owned files, so shell out to a container).
clean_artifacts() {
  log "cleaning e2e/test-results, blob-report, PR assets, and /tmp shard logs"
  rm -rf "$WEB_DIR"/e2e/test-results* "$WEB_DIR"/e2e/blob-report "$WEB_DIR"/.pr-assets 2>/dev/null
  rm -f /tmp/e2e-host-shard-*.log /tmp/e2e-docker-shard-*.log 2>/dev/null
  if docker_up; then
    docker run --rm -v "$WEB_DIR":/web alpine sh -c \
      'rm -rf /web/e2e/test-results* /web/e2e/blob-report /web/.pr-assets' \
      2>/dev/null || true
  fi
}

prepare_pr_assets() {
  [[ -n "${CAPTURE_PR_ASSETS:-}" ]] || return
  log "preparing PR asset directory"
  rm -rf "$WEB_DIR/.pr-assets" 2>/dev/null || true
  if [[ -e "$WEB_DIR/.pr-assets" ]]; then
    docker_up || die "cannot remove root-owned PR assets without Docker; run e2e:clean where Docker is available"
    docker run --rm -v "$WEB_DIR":/web alpine sh -c 'rm -rf /web/.pr-assets' \
      >/dev/null || die "failed to remove root-owned PR assets"
  fi
  mkdir -p "$WEB_DIR/.pr-assets"
}

restore_pr_asset_ownership() {
  [[ -n "${CAPTURE_PR_ASSETS:-}" ]] || return 0
  docker run --rm -v "$WEB_DIR":/web alpine sh -c "chown -R $(id -u):$(id -g) /web/.pr-assets" \
    >/dev/null || {
      log "failed to restore PR asset ownership"
      return 1
    }
}

# `build:e2e`, not `build`: a plain `vite build` drops the pseudo catalog from
# the bundle (it is the largest locale and no production user can select it), and
# tests/i18n/pseudo-coverage.spec.ts is the oracle that needs it. See
# apps/web/lib/i18n/bundling.ts. Mirror any change here in the CI e2e build job
# (.github/workflows/e2e-tests.yml -> make build-web-e2e).
build_fe() {
  log "building Vite web assets (with the pseudo QA locale)"
  ( cd "$REPO_ROOT/apps" && pnpm --filter @kandev/web build:e2e ) \
    || die "FE build failed"
}

build_backend_host() {
  log "building backend (host)"
  local targets=(build)
  # PROJECT is normalized above, so this also covers the deprecated `docker` alias.
  [[ "$PROJECT" == containers ]] && targets+=(build-agentctl-linux build-mock-agent-linux)
  make -C "$BACKEND_DIR" "${targets[@]}" >/dev/null || die "backend build failed"
}

# Packages the plugin-fixture SDK plugin (cmd/plugin-fixture) into
# .build/kandev-plugin-e2e-1.0.0.tar.gz for tests/plugins/plugins.spec.ts.
# Always built on the host — like mock-agent, e2e/global-setup.ts only checks
# this file exists (not that it's current), so it must be rebuilt any time
# cmd/plugin-fixture changes (this runner always rebuilds it via build_fe's
# sibling call below, unless --no-build is set).
build_plugin_package() {
  log "packaging e2e fixture plugin"
  make -C "$BACKEND_DIR" e2e-plugin-package >/dev/null || die "e2e plugin package build failed"
}

backend_runs_in() {  # $1=image — does the host-built binary run there? (glibc check, ~2s)
  # Mount to a distinct path, not /bin — overlaying /bin would shadow the
  # container's system utilities (/bin/sh etc.).
  docker run --rm -v "$REPO_ROOT/apps/backend/bin":/test-bin:ro "$1" \
    /test-bin/kandev --version >/dev/null 2>&1
}

build_backend_in_build_image() {
  log "building backend in $BUILD_IMAGE (glibc-matched to runtime image)"
  docker image inspect "$BUILD_IMAGE" >/dev/null 2>&1 \
    || die "host-built backend is incompatible with the runtime image's glibc and $BUILD_IMAGE isn't available. Pull it (docker pull $BUILD_IMAGE) or run with --host."
  local uid_gid; uid_gid="$(id -u):$(id -g)"
  docker run --rm -v "$REPO_ROOT":/work -w /work/apps/backend \
    -v kandev-gocache:/root/.cache/go-build -v kandev-gomod:/go/pkg/mod \
    "$BUILD_IMAGE" \
    bash -lc "git config --global --add safe.directory /work 2>/dev/null; make build VERSION=dev-docker && chown -R $uid_gid /work/apps/backend/bin" \
    >/dev/null || die "in-container backend build failed"
}

# Build a backend binary that runs in the runtime image. Prefer the host build
# (fast, no 5GB build image): a binary linked against the host's glibc runs in
# the container as long as the host's glibc is the same or OLDER than the
# image's (the usual case — the runtime image tracks recent Ubuntu). Only when
# the host is NEWER do we fall back to the build image.
build_backend_for_docker() {  # $1=runtime image
  build_backend_host
  if backend_runs_in "$1"; then
    log "host-built backend runs in $1 (glibc OK)"
  else
    log "host-built backend won't run in $1 (host glibc newer than image?) — using $BUILD_IMAGE"
    build_backend_in_build_image
  fi
}

# --- arg parsing
SUBCMD=run
MODE=auto
SHARDS=1
DO_BUILD=1
STRICT=1
PROJECT=chromium
PW_ARGS=()
[[ "${1:-}" == clean ]] && { SUBCMD=clean; shift; }
[[ "${1:-}" == run ]] && shift
while [[ $# -gt 0 ]]; do
  case "$1" in
    --docker) MODE=docker ;;
    --host) MODE=host ;;
    --shards) [[ "${2:-}" =~ ^[1-9][0-9]*$ ]] || die "--shards must be a positive integer (got '${2:-}')"; SHARDS="$2"; shift ;;
    --no-build) DO_BUILD=0 ;;
    --no-strict) STRICT=0 ;;
    --project) [[ "${2:-}" =~ ^[a-zA-Z0-9_-]+$ ]] || die "invalid --project name: '${2:-}'"; PROJECT="$2"; shift ;;
    --project=*) PROJECT="${1#--project=}"; [[ "$PROJECT" =~ ^[a-zA-Z0-9_-]+$ ]] || die "invalid --project name: '$PROJECT'" ;;
    --) shift; PW_ARGS+=("$@"); break ;;
    *) PW_ARGS+=("$1") ;;
  esac
  shift
done

if [[ "$SUBCMD" == clean ]]; then clean_artifacts; log "clean done"; exit 0; fi

# `docker` is a deprecated alias for the `containers` Playwright project
# (playwright.config.ts only defines `containers`); normalize once so every
# downstream check — build targets, env export, and the `--project` value
# handed to Playwright itself — only ever has to know about `containers`.
if [[ "$PROJECT" == docker ]]; then
  log "--project docker is deprecated; using containers"
  PROJECT=containers
fi

# Resolve mode
if [[ "$MODE" == auto ]]; then
  if docker_up && { [[ -n "$RUNTIME_IMAGE" ]] || docker image inspect kandev-ci:runtime-local >/dev/null 2>&1 || docker image inspect ghcr.io/kdlbs/kandev-ci:runtime-latest >/dev/null 2>&1; }; then
    MODE=docker
  else
    MODE=host
  fi
fi
log "mode=$MODE  shards=$SHARDS  project=$PROJECT  strict=$STRICT"

STRICT_ENV=()
[[ "$STRICT" == 1 ]] && STRICT_ENV=(KANDEV_E2E_WS_ASSERT=1)
CONTAINER_ENV=()
[[ "$PROJECT" == containers ]] && CONTAINER_ENV=(KANDEV_E2E_CONTAINERS=1)

# ---------------------------------------------------------------------------
# HOST mode
# ---------------------------------------------------------------------------
run_host() {
  prepare_pr_assets
  [[ "$DO_BUILD" == 1 ]] && { build_backend_host; build_fe; build_plugin_package; }
  local base_args=(playwright test --config e2e/playwright.config.ts --project="$PROJECT")
  # `env FOO=bar cmd` inherits the rest of the parent shell's environment as-is,
  # so a KANDEV_E2E_CONTAINERS/KANDEV_E2E_DOCKER left over from an earlier
  # containers run in the same shell would otherwise leak into an ordinary
  # run and make global setup demand Linux helper binaries it never built.
  if [[ "$SHARDS" -le 1 ]]; then
    ( cd "$WEB_DIR" && env -u KANDEV_E2E_CONTAINERS -u KANDEV_E2E_DOCKER ${STRICT_ENV[@]+"${STRICT_ENV[@]}"} ${CONTAINER_ENV[@]+"${CONTAINER_ENV[@]}"} pnpm exec "${base_args[@]}" ${PW_ARGS[@]+"${PW_ARGS[@]}"} )
    return $?
  fi
  log "running $SHARDS host shards (distinct E2E_PORT_OFFSET + output dirs)"
  local pids=() rc=0 i
  for ((i=1; i<=SHARDS; i++)); do
    ( cd "$WEB_DIR" && env -u KANDEV_E2E_CONTAINERS -u KANDEV_E2E_DOCKER ${STRICT_ENV[@]+"${STRICT_ENV[@]}"} ${CONTAINER_ENV[@]+"${CONTAINER_ENV[@]}"} E2E_PORT_OFFSET=$((i-1)) \
        pnpm exec "${base_args[@]}" --shard="$i/$SHARDS" --output="e2e/test-results-shard-$i" ${PW_ARGS[@]+"${PW_ARGS[@]}"} \
        > "/tmp/e2e-host-shard-$i.log" 2>&1 ) &
    pids+=("$!")
  done
  for p in "${pids[@]}"; do wait "$p" || rc=1; done
  for ((i=1; i<=SHARDS; i++)); do
    printf '  shard %s: %s\n' "$i" "$(grep -Eo '[0-9]+ (passed|failed|flaky)' "/tmp/e2e-host-shard-$i.log" | tr '\n' ' ')" >&2
  done
  log "host shard logs: /tmp/e2e-host-shard-*.log"
  return $rc
}

# ---------------------------------------------------------------------------
# DOCKER mode (one container per shard; output stays container-local so the
# mounted repo never gets root-owned junk)
# ---------------------------------------------------------------------------
run_docker() {
  local img; img="$(resolve_runtime_image)"
  log "runtime image: $img"
  clean_artifacts
  prepare_pr_assets
  [[ "$DO_BUILD" == 1 ]] && { build_backend_for_docker "$img"; build_fe; build_plugin_package; }

  local strict_flag=()
  [[ "$STRICT" == 1 ]] && strict_flag=(-e KANDEV_E2E_WS_ASSERT=1)
  local container_flag=()
  [[ "$PROJECT" == containers ]] && container_flag=(-e KANDEV_E2E_CONTAINERS=1)
  local capture_flag=()
  [[ -n "${CAPTURE_PR_ASSETS:-}" ]] && capture_flag=(-e CAPTURE_PR_ASSETS)
  local pw="git config --global --add safe.directory /work 2>/dev/null; cd /work/apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=\"$PROJECT\""

  run_one() {  # $1=shard index (or 0 for unsharded)
    local i="$1" shardflag="" out="/tmp/pw-out"
    [[ "$i" != 0 ]] && shardflag="--shard=$i/$SHARDS"
    docker run --rm --ipc=host \
      -v "$REPO_ROOT":/work -w /work/apps/web \
      ${strict_flag[@]+"${strict_flag[@]}"} \
      ${container_flag[@]+"${container_flag[@]}"} \
      ${capture_flag[@]+"${capture_flag[@]}"} \
      -e NODE_OPTIONS=--dns-result-order=ipv4first \
      -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
      "$img" \
      bash -lc "$pw $shardflag --output=$out --reporter=list \"\$@\"" e2e-runner ${PW_ARGS[@]+"${PW_ARGS[@]}"}
  }

  if [[ "$SHARDS" -le 1 ]]; then
    run_one 0
    local rc=$?
    restore_pr_asset_ownership || return 1
    return $rc
  fi
  log "running $SHARDS isolated containers"
  local pids=() rc=0 i
  for ((i=1; i<=SHARDS; i++)); do
    run_one "$i" > "/tmp/e2e-docker-shard-$i.log" 2>&1 &
    pids+=("$!")
  done
  for p in "${pids[@]}"; do wait "$p" || rc=1; done
  for ((i=1; i<=SHARDS; i++)); do
    printf '  shard %s: %s\n' "$i" "$(grep -Eo '[0-9]+ (passed|failed|flaky)' "/tmp/e2e-docker-shard-$i.log" | tr '\n' ' ')" >&2
  done
  restore_pr_asset_ownership || return 1
  log "docker shard logs: /tmp/e2e-docker-shard-*.log"
  return $rc
}

if [[ "$MODE" == docker ]]; then run_docker; else run_host; fi
