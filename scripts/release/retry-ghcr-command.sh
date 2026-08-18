#!/usr/bin/env bash
# Retry a GHCR mutation only when the registry reports transient throttling.
set -euo pipefail

if (( $# == 0 )); then
  echo "Usage: $0 <command> [args...]" >&2
  exit 2
fi

MAX_ATTEMPTS="${RETRY_GHCR_MAX_ATTEMPTS:-4}"
INITIAL_DELAY_SECONDS="${RETRY_GHCR_INITIAL_DELAY_SECONDS:-60}"
SLEEP_COMMAND="${RETRY_GHCR_SLEEP_COMMAND:-sleep}"

if [[ ! "$MAX_ATTEMPTS" =~ ^[1-9][0-9]*$ ]]; then
  echo "RETRY_GHCR_MAX_ATTEMPTS must be a positive integer" >&2
  exit 2
fi
if [[ ! "$INITIAL_DELAY_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "RETRY_GHCR_INITIAL_DELAY_SECONDS must be a non-negative integer" >&2
  exit 2
fi

OUTPUT_FILE="$(mktemp)"
trap 'rm -f "$OUTPUT_FILE"' EXIT

retryable_failure() {
  grep -Eiq \
    'secondary[[:space:]-]+rate[[:space:]-]*limit|too many requests|status code 429|TOOMANYREQUESTS' \
    "$OUTPUT_FILE"
}

retry_delay() {
  local delay="$INITIAL_DELAY_SECONDS"
  local step

  for ((step = 1; step < $1; step++)); do
    delay=$((delay * 2))
  done
  printf '%s\n' "$delay"
}

for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
  : >"$OUTPUT_FILE"
  set +e
  "$@" 2>&1 | tee "$OUTPUT_FILE"
  status="${PIPESTATUS[0]}"
  set -e

  if (( status == 0 )); then
    exit 0
  fi

  if (( attempt == MAX_ATTEMPTS )) || ! retryable_failure; then
    exit "$status"
  fi

  delay="$(retry_delay "$attempt")"
  echo "::warning::GHCR command attempt $attempt/$MAX_ATTEMPTS hit a transient rate limit; retrying in ${delay}s." >&2
  if (( delay > 0 )); then
    "$SLEEP_COMMAND" "$delay"
  fi
done
