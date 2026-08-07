#!/usr/bin/env bash
# Resolve one npm package/version spec. A missing package, version, or dist-tag
# is a successful empty result; registry and network failures are errors.
set -euo pipefail

if [[ "$#" -ne 1 || -z "$1" ]]; then
  echo "Usage: $0 <package-spec>" >&2
  exit 2
fi

SPEC="$1"
ERROR_FILE="$(mktemp)"
trap 'rm -f "$ERROR_FILE"' EXIT
MAX_ATTEMPTS=3
RETRY_DELAY_SECONDS=2

for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
  if VERSION="$(npm view "$SPEC" version --loglevel=error 2>"$ERROR_FILE")"; then
    printf '%s\n' "$VERSION"
    exit 0
  fi

  if grep -qiE '^npm (error|ERR!) (code E404|404([[:space:]]|$)|No match found for version|[^[:space:]]+ is not in this registry)' "$ERROR_FILE"; then
    exit 0
  fi

  if ((attempt < MAX_ATTEMPTS)); then
    echo "npm view attempt $attempt/$MAX_ATTEMPTS failed for $SPEC; retrying" >&2
    sleep "$RETRY_DELAY_SECONDS"
  fi
done

echo "npm view failed for $SPEC" >&2
exit 1
