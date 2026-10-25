#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <stable-tag>" >&2
  exit 2
fi

tag=$1
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "stable tag must be an explicit SemVer tag such as v0.93.0" >&2
  exit 2
fi

root=$(git rev-parse --show-toplevel)
commit=$(git rev-parse "$tag^{commit}")
fixture_dir="$root/apps/backend/internal/persistence/storeconformance/testdata/upgrades/$tag"
manifest="$fixture_dir/manifest.json"

if [[ ! -f "$manifest" || ! -f "$fixture_dir/sqlite.sql" || ! -f "$fixture_dir/postgres.sql" ]]; then
  echo "fixture files are missing for $tag" >&2
  exit 1
fi

FIXTURE_ROOT="$root" FIXTURE_MANIFEST="$manifest" FIXTURE_TAG="$tag" FIXTURE_COMMIT="$commit" \
python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

manifest_path = Path(os.environ["FIXTURE_MANIFEST"])
fixture_root = Path(os.environ["FIXTURE_ROOT"])
tag = os.environ["FIXTURE_TAG"]
commit = os.environ["FIXTURE_COMMIT"]
document = json.loads(manifest_path.read_text())
if document.get("tag") != tag:
    raise SystemExit(f"manifest tag is {document.get('tag')!r}, expected {tag!r}")
document["source_commit"] = commit
for fixture in document.get("fixtures", []):
    path = fixture_root / "apps/backend/internal/persistence/storeconformance/testdata/upgrades" / tag / fixture["file"]
    fixture["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
manifest_path.write_text(json.dumps(document, indent=2) + "\n")
PY

echo "updated provenance and checksums for $tag at $commit"
