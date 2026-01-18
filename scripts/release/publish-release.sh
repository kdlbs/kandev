#!/usr/bin/env bash
# Tag and push a release version to trigger the GitHub Actions bundle build.
# Usage: scripts/release/publish-release.sh <version>
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 0.1.2"
  exit 1
fi

VERSION="$1"

LAUNCHER_VERSION="$(node -p "require('./apps/cli/package.json').version")"
if [ "$LAUNCHER_VERSION" != "$VERSION" ]; then
  echo "Launcher version mismatch: apps/cli/package.json is $LAUNCHER_VERSION"
  echo "Run scripts/release/publish-launcher.sh $VERSION first."
  exit 1
fi

if git rev-parse "v$VERSION" >/dev/null 2>&1; then
  echo "Tag v$VERSION already exists."
  exit 1
fi

echo "Creating tag v$VERSION..."
git tag "v$VERSION"

echo "Pushing tag v$VERSION..."
git push origin "v$VERSION"

echo "Done. GitHub Actions will build and upload runtime bundles."
