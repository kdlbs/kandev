#!/usr/bin/env bash

set -euo pipefail

if (( $# != 1 )); then
  echo "usage: $0 <image>" >&2
  exit 2
fi

image="$1"
attempt_timeout="${IMAGE_RESOLVE_ATTEMPT_TIMEOUT:-30s}"
manifest_file="$(mktemp)"
trap 'rm -f "$manifest_file"' EXIT

for attempt in 1 2 3 4 5; do
  : > "$manifest_file"
  inspect_status=0
  timeout --kill-after=5s "$attempt_timeout" \
    docker buildx imagetools inspect --raw "$image" \
    > "$manifest_file" || inspect_status=$?
  if (( inspect_status == 124 || inspect_status == 137 )); then
    echo "::warning::Registry lookup for $image timed out after $attempt_timeout on attempt $attempt/5" >&2
  fi
  if (( inspect_status == 0 )) \
    && [[ -s "$manifest_file" ]] \
    && jq -e '
      .schemaVersion == 2
      and (
        (.manifests? | type == "array")
        or ((.config? | type == "object") and (.layers? | type == "array"))
      )
    ' "$manifest_file" > /dev/null; then
    printf 'sha256:%s\n' "$(sha256sum "$manifest_file" | cut -d ' ' -f 1)"
    exit 0
  fi

  if (( attempt < 5 )); then
    echo "::warning::Could not resolve $image on attempt $attempt/5; retrying" >&2
    sleep "$((attempt * 2))"
  fi
done

echo "::error::Could not resolve an immutable digest for $image" >&2
exit 1
