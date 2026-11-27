#!/usr/bin/env bash
set -euo pipefail

ASSETS_DIR="${1:?Usage: $0 <assets-dir> <output-file>}"
REPORT="${2:?Usage: $0 <assets-dir> <output-file>}"
# Tags cut before the Windows helper shipped publish only four helpers.
WINDOWS_HELPER="${WINDOWS_HELPER:-true}"
if [[ "$WINDOWS_HELPER" != "true" && "$WINDOWS_HELPER" != "false" ]]; then
  echo "WINDOWS_HELPER must be true or false" >&2
  exit 1
fi
HELPER_ASSETS=(agentctl-linux-amd64.gz agentctl-linux-arm64.gz agentctl-darwin-amd64.gz agentctl-darwin-arm64.gz)
if [[ "$WINDOWS_HELPER" = "true" ]]; then
  HELPER_ASSETS+=(agentctl-windows-amd64.exe.gz)
fi
mkdir -p "$(dirname "$REPORT")"

asset_size() {
  wc -c < "$ASSETS_DIR/$1" | tr -d ' '
}

asset_mib() {
  awk -v bytes="$1" 'BEGIN { printf "%.2f", bytes / 1048576 }'
}

{
  printf '# Runtime asset sizes\n\n'
  printf 'Measured from this release build. Archive sizes are compressed download bytes.\n\n'
  printf '| Variant | Platform or asset | Bytes | MiB |\n'
  printf '| --- | --- | ---: | ---: |\n'
  for platform in linux-x64 linux-arm64 macos-x64 macos-arm64 windows-x64; do
    for variant in standard full; do
      name="kandev-${platform}"
      if [[ "$variant" = "full" ]]; then
        name="${name}-full"
      fi
      asset="${name}.tar.gz"
      bytes="$(asset_size "$asset")"
      printf '| %s | %s (`%s`) | %s | %s |\n' \
        "$variant" "$platform" "$asset" "$bytes" "$(asset_mib "$bytes")"
      if [[ "$platform" = "windows-x64" ]]; then
        asset="${name}.zip"
        bytes="$(asset_size "$asset")"
        printf '| %s | %s (`%s`) | %s | %s |\n' \
          "$variant" "$platform" "$asset" "$bytes" "$(asset_mib "$bytes")"
      fi
    done
  done
  for asset in "${HELPER_ASSETS[@]}"; do
    bytes="$(asset_size "$asset")"
    printf '| helper | `%s` | %s | %s |\n' "$asset" "$bytes" "$(asset_mib "$bytes")"
  done
} > "$REPORT"
