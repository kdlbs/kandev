#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/resource-guard.sh"

manifest_path="${1:-}"
if [[ -z "$manifest_path" ]]; then
  echo "usage: run-planned-shard.sh <manifest.json>" >&2
  exit 2
fi
shift

e2e_validate_playwright_args "$@" || exit 2
exec pnpm exec tsx e2e/scripts/run-planned-shard.ts --manifest "$manifest_path" "$@"
