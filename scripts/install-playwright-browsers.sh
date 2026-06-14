#!/usr/bin/env bash
# install-playwright-browsers.sh — install Playwright chromium browser artifacts
# with a workaround for Firecracker VMs where Node.js io_uring-based
# zip extraction hangs indefinitely.
#
# Strategy: run `playwright install chromium` with a timeout. If it
# completes, great. If it hangs (extraction stuck), kill it and
# manually extract the already-downloaded zips with `unzip`.

set -euo pipefail

PW_CACHE="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"
TIMEOUT_SECS=90

cd "$(dirname "$0")/../apps"

PW_ARGS=(chromium chromium-headless-shell)

dry_run=$(pnpm --filter @kandev/web exec playwright install --dry-run "${PW_ARGS[@]}" 2>&1 || true)
missing=0
while IFS= read -r loc; do
  [ -n "$loc" ] || continue
  if [ ! -f "$loc/INSTALLATION_COMPLETE" ]; then
    missing=1
    break
  fi
done < <(printf '%s\n' "$dry_run" | awk -F'Install location:[[:space:]]*' 'NF > 1 {print $2}' | sort -u)

if [ "$missing" -eq 0 ]; then
  echo "[playwright] browsers already installed"
  exit 0
fi

# Install system dependencies (idempotent, needs sudo)
pnpm --filter @kandev/web exec playwright install-deps chromium 2>/dev/null || true

# Attempt normal install with timeout
rm -rf "$PW_CACHE/__dirlock"
if timeout "$TIMEOUT_SECS" pnpm --filter @kandev/web exec playwright install "${PW_ARGS[@]}" 2>&1; then
  echo "[playwright] install completed normally"
  exit 0
fi

echo "[playwright] install timed out — falling back to manual extraction"

# Kill any leftover oopDownload processes
pkill -f oopDownloadBrowserMain 2>/dev/null || true
sleep 1

# Manually extract any downloaded zips that weren't fully extracted
for comp_dir in "$PW_CACHE"/chromium-* "$PW_CACHE"/chromium_headless_shell-* "$PW_CACHE"/ffmpeg-*; do
  [ -d "$comp_dir" ] || continue
  [ -f "$comp_dir/INSTALLATION_COMPLETE" ] && continue

  comp_name=$(basename "$comp_dir" | sed 's/-[0-9]*$//')
  comp_name_alt=${comp_name//_/-}
  zipfile=$(find /tmp \( -name "playwright-download-${comp_name}-*" -o -name "playwright-download-${comp_name_alt}-*" \) -type f 2>/dev/null | sort | tail -1)

  if [ -z "$zipfile" ]; then
    echo "[playwright] no zip found for $comp_name — skipping"
    continue
  fi

  if ! unzip -t "$zipfile" >/dev/null 2>&1; then
    echo "[playwright] zip corrupt for $comp_name — skipping"
    continue
  fi

  echo "[playwright] extracting $comp_name from $zipfile"
  rm -rf "$comp_dir"
  mkdir -p "$comp_dir"
  (cd "$comp_dir" && unzip -q "$zipfile")
  echo '{}' > "$comp_dir/INSTALLATION_COMPLETE"
done

echo "[playwright] manual extraction complete"
