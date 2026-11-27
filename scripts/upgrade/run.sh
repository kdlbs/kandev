#!/usr/bin/env bash
set -euo pipefail
if [[ "${CI:-}" != true || "${GITHUB_ACTIONS:-}" != true || "${GITHUB_REF:-}" != refs/heads/main ]]; then
  echo "Release upgrade rehearsals run only in GitHub Actions on main." >&2
  exit 1
fi
cd "$(dirname "$0")/../.."
exec node scripts/upgrade/run.mjs "$@"
