#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
export CHECK_TRACE="$scratch/trace"
cat > "$scratch/python3" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >> "$CHECK_TRACE"
EOF
cat > "$scratch/golangci-lint" <<'EOF'
#!/bin/sh
printf 'linter:%s\n' "$*" >> "$CHECK_TRACE"
EOF
chmod +x "$scratch/python3" "$scratch/golangci-lint"
export PATH="$scratch:$PATH"
export FULL_WORKER_CHECK_MODE=isolated
unset FULL_WORKER_CHECK_INSIDE
make --no-print-directory -C "$root/apps/backend" lint > "$scratch/output" 2>&1
if ! grep -Fxq -- '--kind' "$CHECK_TRACE" || ! grep -Fxq -- lint "$CHECK_TRACE"; then
  echo 'enabled lint ran without validation isolation' >&2; exit 1
fi
if grep -q '^linter:' "$CHECK_TRACE"; then echo 'linter ran in agent container' >&2; exit 1; fi
: > "$CHECK_TRACE"
make --no-print-directory -C "$root" build-backend > "$scratch/output" 2>&1
grep -Fxq -- build "$CHECK_TRACE"
: > "$CHECK_TRACE"
FULL_WORKER_CHECK_INSIDE=1 make --no-print-directory -C "$root/apps/backend" lint > "$scratch/output" 2>&1
grep -Fxq -- 'linter:run ./... --concurrency=2' "$CHECK_TRACE"
: > "$CHECK_TRACE"
FULL_WORKER_CHECK_MODE= make --no-print-directory -C "$root/apps/backend" lint > "$scratch/output" 2>&1
grep -Fxq -- 'linter:run ./...' "$CHECK_TRACE"
echo 'Worker Make dispatch and host compatibility passed'
