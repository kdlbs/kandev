#!/usr/bin/env bash
set -euo pipefail
repo_root=$(git rev-parse --show-toplevel)
exec node "$repo_root/scripts/release/publish-plugin-sdk.mjs" "$@"
