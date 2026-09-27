#!/usr/bin/env bash
# Return success when the SignPath inputs for Windows code signing are complete
# and the configured policy may be used for what this run does with the bundle.
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: signpath-signing-ready.sh [publish|validate|nightly]

Checks whether the current environment has enough SignPath inputs to submit a
signing request for the Windows runtime binaries, and whether the configured
signing policy is acceptable for this run:

  publish   the bundle is released as a stable version; a test-signing policy
            is refused, because its certificate is not trusted by Windows
  validate  nothing is published (desktop_validation_only); any policy is fine
  nightly   the nightly channel; signing is skipped, because release signing
            needs a manual approval per request and nightlies run unattended

The default is publish. Exit status 1 means incomplete inputs, 2 a test-signing
policy on a publishing run, 3 an unsupported purpose, 4 the nightly channel.
None of them is a workflow error by itself: the release still publishes an
unsigned bundle.
USAGE
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

purpose="${1:-publish}"
case "$purpose" in
  publish|validate) ;;
  nightly)
    printf 'Nightly channel: SignPath signing is skipped; release signing needs a manual approval per request and nightlies run unattended.\n' >&2
    exit 4
    ;;
  *)
    printf 'Unsupported run purpose: %s (expected one of: publish, validate, nightly).\n' "$purpose" >&2
    exit 3
    ;;
esac

missing=()

# The signing action takes all four inputs and fails the request when it is
# handed a blank one, so a partial configuration has to read as "not
# configured" rather than fail the release. Whitespace counts as blank: the
# action cannot tell it apart from an unset variable either.
require_input() {
  local name="$1"
  local value="${!name:-}"
  if [ -z "${value//[[:space:]]/}" ]; then
    missing+=("$name")
  fi
}

require_input SIGNPATH_API_TOKEN
require_input SIGNPATH_ORGANIZATION_ID
require_input SIGNPATH_PROJECT_SLUG
require_input SIGNPATH_SIGNING_POLICY_SLUG

if [ "${#missing[@]}" -gt 0 ]; then
  printf 'SignPath signing inputs incomplete: %s.\n' "${missing[*]}" >&2
  exit 1
fi

# SignPath creates a test-signing policy on every project, bound to a
# self-signed certificate. Windows does not trust it, so a bundle signed with
# it is worse than an unsigned one; only a run that publishes nothing may use it.
case "${SIGNPATH_SIGNING_POLICY_SLUG,,}" in
  test|test-*)
    if [ "$purpose" = publish ]; then
      printf "SignPath policy '%s' is a test-signing policy and this run publishes; refusing to sign.\n" \
        "$SIGNPATH_SIGNING_POLICY_SLUG" >&2
      exit 2
    fi
    ;;
esac

printf 'SignPath signing inputs complete.\n'
