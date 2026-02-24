#!/usr/bin/env bash
# Tag and push a release version to trigger the GitHub Actions bundle build.
# Usage: scripts/release/publish-release.sh <version>
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 v0.1.2"
  exit 1
fi

VERSION="$1"

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "Tag $VERSION already exists."
  exit 1
fi

echo "Creating tag $VERSION..."
git tag "$VERSION"

echo "Pushing tag $VERSION..."
git push origin "$VERSION"

echo "Done. GitHub Actions will build and upload runtime bundles."
