#!/usr/bin/env bash
# Return success when desktop signing inputs are complete for a platform family.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: desktop-signing-ready.sh <macos|windows>

Checks whether the current environment has enough signing/notarization inputs
to produce signed desktop artifacts for the requested platform family. Missing
inputs are reported, but incomplete signing is not a workflow error by itself.
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

PLATFORM="${1:-}"
missing=()

require_secret() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    missing+=("$name")
  fi
}

case "$PLATFORM" in
  macos)
    require_secret APPLE_CERTIFICATE
    require_secret APPLE_CERTIFICATE_PASSWORD
    require_secret KEYCHAIN_PASSWORD

    apple_id_notarization=false
    if [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_PASSWORD:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ]; then
      apple_id_notarization=true
    fi

    api_key_notarization=false
    if [ -n "${APPLE_API_KEY:-}" ] && [ -n "${APPLE_API_ISSUER:-}" ] && [ -n "${APPLE_API_KEY_P8:-}" ]; then
      api_key_notarization=true
    fi

    if [ "$apple_id_notarization" = "false" ] && [ "$api_key_notarization" = "false" ]; then
      missing+=("APPLE_ID/APPLE_PASSWORD/APPLE_TEAM_ID or APPLE_API_KEY/APPLE_API_ISSUER/APPLE_API_KEY_P8")
    fi
    ;;
  windows)
    require_secret WINDOWS_CERTIFICATE
    require_secret WINDOWS_CERTIFICATE_PASSWORD
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [ "${#missing[@]}" -gt 0 ]; then
  printf 'Desktop signing inputs incomplete for %s: %s\n' "$PLATFORM" "${missing[*]}" >&2
  exit 1
fi

printf 'Desktop signing inputs complete for %s.\n' "$PLATFORM"
