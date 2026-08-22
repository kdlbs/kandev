#!/usr/bin/env bash
# Boot an isolated Kandev instance with authentication enabled and seed a small
# team, so workspace visibility and membership can be clicked through.
#
# The instance is deliberately hermetic: `env -i` plus an explicit
# KANDEV_HOME_DIR, because an inherited KANDEV_DATABASE_PATH points at the
# developer's real database.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${KANDEV_DEMO_PORT:-8231}"
HOME_DIR="${KANDEV_DEMO_HOME:-/tmp/kandev-team-access-demo}"
# Bind loopback plus any extra host (e.g. a Tailscale IP) so the instance is
# reachable from another device on the tailnet. Seeding always talks to
# loopback; BASE is only what the seeder and probes use.
# Loopback is always bound: the health probe and the seeder talk to BASE on
# 127.0.0.1, so an extra host must be added to loopback rather than replace it.
BIND_HOSTS="127.0.0.1${KANDEV_DEMO_BIND:+,${KANDEV_DEMO_BIND}}"
BASE="http://127.0.0.1:${PORT}"
COOKIE_JAR="${HOME_DIR}/admin-cookies.txt"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

api() { # api METHOD PATH [JSON]
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "${BASE}${path}"
  else
    curl -sS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X "$method" "${BASE}${path}"
  fi
}

start_backend() {
  # Refuse to boot onto an occupied port. Without this the health probe below
  # happily passes against a STALE server still holding the port, and every
  # later check silently measures the old binary.
  if ss -lptn "sport = :${PORT}" 2>/dev/null | grep -q LISTEN; then
    echo "port ${PORT} is already in use; run '$0 stop' or free it first" >&2
    ss -lptn "sport = :${PORT}" >&2
    exit 1
  fi
  rm -rf "$HOME_DIR"
  mkdir -p "$HOME_DIR"
  log "booting kandev on ${BASE} (home ${HOME_DIR})"
  env -i \
    HOME="$HOME_DIR" \
    PATH="${REPO_ROOT}/apps/backend/bin:/usr/local/bin:/usr/bin:/bin" \
    KANDEV_HOME_DIR="$HOME_DIR" \
    KANDEV_PORT="$PORT" \
    KANDEV_SERVER_HOST="$BIND_HOSTS" \
    KANDEV_FEATURES_AUTH=true \
    KANDEV_WEB_DIST_DIR="${REPO_ROOT}/apps/web/dist" \
    "${REPO_ROOT}/apps/backend/bin/kandev" __backend \
    >"${HOME_DIR}/backend.log" 2>&1 &
  echo $! >"${HOME_DIR}/backend.pid"

  for _ in $(seq 1 90); do
    if curl -sf "${BASE}/health" >/dev/null 2>&1; then
      log "backend is up (pid $(cat "${HOME_DIR}/backend.pid"))"
      return 0
    fi
    sleep 1
  done
  echo "backend did not become healthy; last log lines:" >&2
  tail -40 "${HOME_DIR}/backend.log" >&2
  exit 1
}

seed() {
  log "seeding the team"
  python3 "${REPO_ROOT}/scripts/demo_team_access_seed.py" "$BASE"
}

case "${1:-up}" in
  up)   start_backend; seed ;;
  stop)
    # Kill by the port's actual listener, not just the pid file: a crashed or
    # replaced launch leaves the pid file naming a process that is not the one
    # serving requests.
    for pid in $(ss -lptn "sport = :${PORT}" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | sort -u); do
      if tr '\0' ' ' <"/proc/${pid}/environ" 2>/dev/null | grep -q "$HOME_DIR"; then
        kill "$pid" 2>/dev/null || true
        log "stopped pid ${pid}"
      fi
    done
    ;;
  *)    echo "usage: $0 [up|stop]" >&2; exit 2 ;;
esac
