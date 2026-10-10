#!/usr/bin/env bash
# Run Playwright through the workspace package, while keeping the deprecated
# project alias usable and enforcing the local resource budget.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ ${FULL_WORKER_CHECK_MODE:-} == isolated ]]; then
  source "$SCRIPT_DIR/worker-isolation.sh"
  worker_isolation_dispatch run-raw-e2e.sh "$@" || exit $?
fi
source "$SCRIPT_DIR/resource-guard.sh"

PW_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' "--project requires a project name" >&2
        exit 2
      fi
      if [[ "$2" == docker ]]; then
        PW_ARGS+=(--project=containers)
      else
        PW_ARGS+=("$1" "$2")
      fi
      shift 2
      ;;
    --project=docker)
      PW_ARGS+=(--project=containers)
      shift
      ;;
    *)
      PW_ARGS+=("$1")
      shift
      ;;
  esac
done

e2e_validate_playwright_args "${PW_ARGS[@]}" || exit 2
exec pnpm exec playwright test --config e2e/playwright.config.ts --workers=1 "${PW_ARGS[@]}"
