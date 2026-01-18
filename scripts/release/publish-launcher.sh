#!/usr/bin/env bash
# Publish the npm launcher package (apps/cli) with a specific version.
# Usage: scripts/release/publish-launcher.sh <version>
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 0.1.2"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="$1"
if [[ "$VERSION" == v* ]]; then
  VERSION="${VERSION#v}"
  echo "Stripped leading 'v', using version $VERSION"
fi

echo "Updating launcher version to $VERSION..."
(cd "$ROOT_DIR/apps/cli" && npm version --no-git-tag-version "$VERSION")

ACTUAL_VERSION="$(node -p "require('./apps/cli/package.json').version")"
if [ "$ACTUAL_VERSION" != "$VERSION" ]; then
  echo "Failed to set launcher version to $VERSION (found $ACTUAL_VERSION)."
  exit 1
fi

echo "Installing workspace dependencies..."
pnpm -C "$ROOT_DIR/apps" install --frozen-lockfile

echo "Building launcher (TypeScript)..."
pnpm -C "$ROOT_DIR/apps/cli" build

echo "Publishing launcher to npm..."
(cd "$ROOT_DIR/apps/cli" && npm publish --access public)

echo "Done. This only published the npm CLI package."
