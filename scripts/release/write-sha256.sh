#!/usr/bin/env bash
# Write a SHA-256 checksum file for a release artifact.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: write-sha256.sh <artifact> [checksum-file]

Writes a checksum file using the portable "<sha256>  <filename>" format. The
filename in the checksum is always the artifact basename so the file can be
verified from the artifact directory on Linux, macOS, or Windows runners.
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  usage >&2
  exit 2
fi

artifact="$1"
checksum_file="${2:-$artifact.sha256}"

if [ ! -f "$artifact" ]; then
  echo "Missing artifact: $artifact" >&2
  exit 1
fi

artifact_dir="$(cd "$(dirname "$artifact")" && pwd -P)"
artifact_name="$(basename "$artifact")"
checksum_dir="$(dirname "$checksum_file")"
mkdir -p "$checksum_dir"

tmp="${checksum_file}.tmp.$$"
trap 'rm -f "$tmp"' EXIT

if command -v shasum >/dev/null 2>&1; then
  (cd "$artifact_dir" && shasum -a 256 "$artifact_name") > "$tmp"
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$artifact_dir" && sha256sum "$artifact_name") > "$tmp"
else
  echo "Missing checksum tool: need shasum or sha256sum" >&2
  exit 1
fi

mv "$tmp" "$checksum_file"
trap - EXIT
