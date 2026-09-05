#!/usr/bin/env bash

set -euo pipefail

if (( $# != 1 )); then
  echo "usage: $0 <image>" >&2
  exit 2
fi

image="$1"
manifest_file="$(mktemp)"
trap 'rm -f "$manifest_file"' EXIT

for attempt in 1 2 3 4 5; do
  : > "$manifest_file"
  if docker buildx imagetools inspect --raw "$image" > "$manifest_file" \
    && [[ -s "$manifest_file" ]] \
    && jq -e '.schemaVersion == 2' "$manifest_file" > /dev/null; then
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
