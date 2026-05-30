#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
PORT="${KANDEV_TEST_PORT:-38429}"

if [ "${KANDEV_REAL_NPM_CONFIRM:-}" != "1" ]; then
  echo "[self-update-real-npm] this mutates your global npm kandev install and user service." >&2
  echo "[self-update-real-npm] re-run with KANDEV_REAL_NPM_CONFIRM=1 when ready." >&2
  exit 2
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "[self-update-real-npm] npm is required" >&2
  exit 1
fi

VERSIONS_JSON="$(npm view kandev versions --json)"
LATEST_VERSION="${KANDEV_TEST_TARGET_VERSION:-$(node -e 'const v=JSON.parse(process.argv[1]); console.log(v[v.length - 1]);' "$VERSIONS_JSON")}"
CURRENT_VERSION="${KANDEV_TEST_CURRENT_VERSION:-$(node -e 'const v=JSON.parse(process.argv[1]); console.log(v[v.length - 2]);' "$VERSIONS_JSON")}"

if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
  echo "[self-update-real-npm] current and latest are both $CURRENT_VERSION" >&2
  exit 1
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) RUNTIME_NAME="@kdlbs/runtime-linux-x64"; RUNTIME_OS='["linux"]'; RUNTIME_CPU='["x64"]' ;;
  Linux-aarch64|Linux-arm64) RUNTIME_NAME="@kdlbs/runtime-linux-arm64"; RUNTIME_OS='["linux"]'; RUNTIME_CPU='["arm64"]' ;;
  Darwin-x86_64) RUNTIME_NAME="@kdlbs/runtime-darwin-x64"; RUNTIME_OS='["darwin"]'; RUNTIME_CPU='["x64"]' ;;
  Darwin-arm64) RUNTIME_NAME="@kdlbs/runtime-darwin-arm64"; RUNTIME_OS='["darwin"]'; RUNTIME_CPU='["arm64"]' ;;
  *)
    echo "[self-update-real-npm] unsupported platform $(uname -s)-$(uname -m)" >&2
    exit 1
    ;;
esac

rm -rf "$TEST_HOME"
mkdir -p "$TEST_HOME/packages" "$TEST_HOME/tarballs"

if [ ! -d "$ROOT/apps/node_modules" ]; then
  echo "[self-update-real-npm] installing pnpm workspace deps"
  (cd "$ROOT/apps" && pnpm install --frozen-lockfile)
fi

echo "[self-update-real-npm] building branch runtime as v$CURRENT_VERSION"
(cd "$ROOT" && VERSION="v$CURRENT_VERSION" make build-backend build-web)
(cd "$ROOT" && scripts/release/package-web.sh)
(cd "$ROOT" && scripts/release/package-cli.sh)

rm -rf "$ROOT/dist/kandev/bin" "$ROOT/dist/kandev/web"
mkdir -p "$ROOT/dist/kandev/bin"
cp "$ROOT/apps/backend/bin/kandev" "$ROOT/dist/kandev/bin/kandev"
cp "$ROOT/apps/backend/bin/agentctl" "$ROOT/dist/kandev/bin/agentctl"
(cd "$ROOT" && scripts/release/package-bundle.sh)

RUNTIME_DIR="$TEST_HOME/packages/runtime"
mkdir -p "$RUNTIME_DIR"
cp -R "$ROOT/dist/kandev/bin" "$RUNTIME_DIR/bin"
cp -R "$ROOT/dist/kandev/web" "$RUNTIME_DIR/web"
cat >"$RUNTIME_DIR/package.json" <<EOF
{
  "name": "$RUNTIME_NAME",
  "version": "$CURRENT_VERSION",
  "description": "Temporary Kandev runtime bundle for real self-update test",
  "license": "AGPL-3.0-only",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/kdlbs/kandev.git"
  },
  "os": $RUNTIME_OS,
  "cpu": $RUNTIME_CPU,
  "files": ["bin", "web"]
}
EOF

RUNTIME_TGZ="$(cd "$TEST_HOME/tarballs" && npm pack "$RUNTIME_DIR" --silent)"
RUNTIME_TGZ="$TEST_HOME/tarballs/$RUNTIME_TGZ"

CLI_DIR="$TEST_HOME/packages/kandev"
mkdir -p "$CLI_DIR/bin" "$CLI_DIR/dist"
cp "$ROOT/apps/cli/bin/cli.js" "$CLI_DIR/bin/cli.js"
cp -R "$ROOT/apps/cli/dist/." "$CLI_DIR/dist/"
node - "$ROOT/apps/cli/package.json" "$CLI_DIR/package.json" "$CURRENT_VERSION" "$RUNTIME_NAME" "$RUNTIME_TGZ" <<'NODE'
const fs = require("fs");
const [src, dst, version, runtimeName, runtimeTgz] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(src, "utf8"));
pkg.version = version;
pkg.private = false;
pkg.optionalDependencies = { [runtimeName]: `file:${runtimeTgz}` };
delete pkg.devDependencies;
fs.writeFileSync(dst, `${JSON.stringify(pkg, null, 2)}\n`);
NODE

CLI_TGZ="$(cd "$TEST_HOME/tarballs" && npm pack "$CLI_DIR" --silent)"
CLI_TGZ="$TEST_HOME/tarballs/$CLI_TGZ"

echo "[self-update-real-npm] installing temporary global kandev@$CURRENT_VERSION"
npm install -g "$CLI_TGZ"

KANDEV_BIN="$(command -v kandev)"
if [ -z "$KANDEV_BIN" ]; then
  echo "[self-update-real-npm] kandev is not on PATH after npm install -g" >&2
  exit 1
fi

echo "[self-update-real-npm] installing user service from $KANDEV_BIN"
"$KANDEV_BIN" service install --home-dir "$TEST_HOME" --port "$PORT" --no-boot-start

cat >"$TEST_HOME/real-npm-env.sh" <<EOF
export TEST_HOME="$TEST_HOME"
export KANDEV_TEST_PORT="$PORT"
export KANDEV_TEST_CURRENT_VERSION="$CURRENT_VERSION"
export KANDEV_TEST_TARGET_VERSION="$LATEST_VERSION"
export KANDEV_TEST_KANDEV_BIN="$KANDEV_BIN"
EOF

echo "[self-update-real-npm] service is running branch code stamped v$CURRENT_VERSION"
echo "[self-update-real-npm] target npm latest is v$LATEST_VERSION"
echo "[self-update-real-npm] next: scripts/tmp-self-update/real-npm-seed-latest.sh"
