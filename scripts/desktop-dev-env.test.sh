#!/usr/bin/env bash
# Verify the repo-local state passed to the Tauri dev process.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HARNESS="$(mktemp)"
PROBE="$(mktemp)"
trap 'rm -f "$HARNESS" "$PROBE"' EXIT

cat >"$HARNESS" <<'MAKE'
include Makefile

.PHONY: desktop-runtime
desktop-runtime: ; @:
MAKE

cat >"$PROBE" <<'SH'
#!/usr/bin/env bash
printf '%s|%s|%s\n' "$KANDEV_HOME_DIR" "$KANDEV_DATABASE_PATH" "$KANDEV_DEBUG_DEV_MODE"
SH
chmod +x "$PROBE"

probe() {
	env "$@" make -C "$ROOT_DIR" --no-print-directory -f "$HARNESS" \
		"PNPM=$PROBE" desktop-dev 2>/dev/null
}

expect_eq() {
	local label=$1 want=$2 got=$3
	if [ "$got" = "$want" ]; then
		printf 'ok    %-35s %s\n' "$label" "$got"
	else
		printf 'FAIL  %-35s\n        want: %s\n        got:  %s\n' "$label" "$want" "$got"
		return 1
	fi
}

expected="${ROOT_DIR}/.kandev-dev|${ROOT_DIR}/.kandev-dev/data/kandev.db|true"

expect_eq "clean environment" "$expected" \
	"$(probe -u KANDEV_HOME_DIR -u KANDEV_DATABASE_PATH -u KANDEV_DEBUG_DEV_MODE)"
expect_eq "inherited production paths" "$expected" \
	"$(probe KANDEV_HOME_DIR=/production/kandev KANDEV_DATABASE_PATH=/production/kandev.db KANDEV_DEBUG_DEV_MODE=false)"

desktop_build_recipe="$(env KANDEV_HOME_DIR=/production/kandev KANDEV_DATABASE_PATH=/production/kandev.db KANDEV_DEBUG_DEV_MODE=false \
	make -C "$ROOT_DIR" --no-print-directory --dry-run MAKE=: desktop-build 2>&1)"
if printf '%s\n' "$desktop_build_recipe" | grep -Eq '(^|[[:space:]])KANDEV_(HOME_DIR|DATABASE_PATH|DEBUG_DEV_MODE)='; then
	echo "FAIL  desktop-build has no desktop-dev environment assignments" >&2
	exit 1
fi
echo "ok    desktop-build keeps existing environment"
