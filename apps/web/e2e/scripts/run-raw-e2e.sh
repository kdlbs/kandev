#!/usr/bin/env bash
# Run Playwright directly, while keeping the deprecated project alias usable.
set -euo pipefail

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

exec playwright test --config e2e/playwright.config.ts "${PW_ARGS[@]}"
