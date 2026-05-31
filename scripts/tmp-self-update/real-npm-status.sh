#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
ENV_PATH="$TEST_HOME/real-npm-env.sh"
if [ -f "$ENV_PATH" ]; then
  # shellcheck disable=SC1090
  source "$ENV_PATH"
fi

PORT="${KANDEV_TEST_PORT:-38429}"
NPM_PREFIX="${KANDEV_TEST_NPM_PREFIX:-$TEST_HOME/npm-global}"
KANDEV_BIN="${KANDEV_TEST_KANDEV_BIN:-$NPM_PREFIX/bin/kandev}"
METADATA_PATH="${KANDEV_TEST_METADATA_PATH:-$TEST_HOME/service/install.json}"
PLIST_PATH="$HOME/Library/LaunchAgents/com.kdlbs.kandev.plist"
export PATH="$NPM_PREFIX/bin:$PATH"
export npm_config_prefix="$NPM_PREFIX"
export NPM_CONFIG_PREFIX="$NPM_PREFIX"

metadata_port() {
  if [ ! -f "$METADATA_PATH" ]; then
    return 0
  fi
  node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); if (m.port) console.log(m.port);' "$METADATA_PATH"
}

plist_port() {
  if [ ! -f "$PLIST_PATH" ]; then
    return 0
  fi
  node - "$PLIST_PATH" <<'NODE'
const fs = require("fs");
const plist = fs.readFileSync(process.argv[2], "utf8");
const match = /<key>KANDEV_SERVER_PORT<\/key>\s*<string>(\d+)<\/string>/.exec(plist);
if (match) console.log(match[1]);
NODE
}

candidate_ports() {
  {
    echo "$PORT"
    metadata_port
    plist_port
    echo 38429
  } | awk 'NF && !seen[$0]++'
}

print_diagnostics() {
  echo "[self-update-real-npm] no candidate port answered" >&2
  echo "[self-update-real-npm] tried ports: $(candidate_ports | paste -sd ', ' -)" >&2
  echo >&2
  case "$(uname -s)" in
    Darwin)
      if [ -f "$PLIST_PATH" ]; then
        echo "[self-update-real-npm] launchd plist port:" >&2
        plist_port >&2 || true
        echo "[self-update-real-npm] launchd plist:" >&2
        sed -n '1,220p' "$PLIST_PATH" >&2 || true
      else
        echo "[self-update-real-npm] missing plist: $PLIST_PATH" >&2
      fi
      echo >&2
      launchctl print "gui/$(id -u)/com.kdlbs.kandev" >&2 || true
      launchctl print-disabled "gui/$(id -u)" 2>/dev/null | grep com.kdlbs.kandev >&2 || true
      echo >&2
      echo "[self-update-real-npm] listening kandev processes:" >&2
      lsof -nP -iTCP -sTCP:LISTEN 2>/dev/null | grep -i kandev >&2 || true
      echo >&2
      echo "[self-update-real-npm] recent service logs:" >&2
      tail -n 120 "$TEST_HOME/logs/service.err" "$TEST_HOME/logs/service.out" >&2 || true
      echo >&2
      echo "[self-update-real-npm] recent macOS unified logs mentioning kandev:" >&2
      log show --last 10m --style compact --predicate 'eventMessage CONTAINS[c] "kandev" OR process CONTAINS[c] "kandev"' >&2 || true
      ;;
    Linux)
      echo "[self-update-real-npm] systemd user status:" >&2
      systemctl --user status kandev --no-pager >&2 || true
      echo >&2
      echo "[self-update-real-npm] recent systemd user logs:" >&2
      journalctl --user-unit kandev -n 160 --no-pager >&2 || true
      echo >&2
      echo "[self-update-real-npm] listening ports:" >&2
      if command -v ss >/dev/null 2>&1; then
        ss -ltnp >&2 || true
      elif command -v lsof >/dev/null 2>&1; then
        lsof -nP -iTCP -sTCP:LISTEN >&2 || true
      fi
      ;;
  esac
}

echo "[self-update-real-npm] backend info:"
INFO=""
FOUND_PORT=""
for candidate in $(candidate_ports); do
  echo "[self-update-real-npm] probing http://localhost:$candidate"
  for _ in $(seq 1 10); do
    if INFO="$(curl -fsS "http://localhost:$candidate/api/v1/system/info" 2>/dev/null)"; then
      FOUND_PORT="$candidate"
      break 2
    fi
    sleep 1
  done
done
if [ -z "$INFO" ]; then
  print_diagnostics
  exit 1
fi
echo "[self-update-real-npm] reachable at http://localhost:$FOUND_PORT"
echo "$INFO"
echo
if [ -f "$METADATA_PATH" ]; then
  echo "[self-update-real-npm] service install metadata:"
  node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); console.log(JSON.stringify({kind:m.kind, manager:m.manager, mode:m.mode, port:m.port, cli_entry:m.cli_entry}, null, 2));' "$METADATA_PATH"
  echo
fi
echo "[self-update-real-npm] isolated npm kandev:"
npm list -g --prefix "$NPM_PREFIX" kandev --depth=0 || true
echo "[self-update-real-npm] service status:"
if [ -x "$KANDEV_BIN" ]; then
  "$KANDEV_BIN" service status || true
else
  echo "[self-update-real-npm] missing $KANDEV_BIN" >&2
fi
