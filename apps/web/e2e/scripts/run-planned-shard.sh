#!/usr/bin/env bash
set -euo pipefail

manifest_path="${1:-}"
if [[ -z "$manifest_path" ]]; then
  echo "usage: run-planned-shard.sh <manifest.json>" >&2
  exit 2
fi

exec pnpm exec tsx e2e/scripts/run-planned-shard.ts --manifest "$manifest_path"
