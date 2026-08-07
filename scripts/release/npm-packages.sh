#!/usr/bin/env bash
# Shared platform and package inventory for npm packaging, tests, preflight,
# and publication. Launcher mappings and package metadata remain independently
# checked against this inventory by release-config.test.ts.

RUNTIME_PLATFORMS=(
  "linux-x64"
  "linux-arm64"
  "macos-x64"
  "macos-arm64"
  "windows-x64"
)
declare -A RUNTIME_PACKAGE_BY_PLATFORM=(
  ["linux-x64"]="@kdlbs/runtime-linux-x64"
  ["linux-arm64"]="@kdlbs/runtime-linux-arm64"
  ["macos-x64"]="@kdlbs/runtime-darwin-x64"
  ["macos-arm64"]="@kdlbs/runtime-darwin-arm64"
  ["windows-x64"]="@kdlbs/runtime-win32-x64"
)
RUNTIME_PACKAGES=()
for runtime_platform in "${RUNTIME_PLATFORMS[@]}"; do
  RUNTIME_PACKAGES+=("${RUNTIME_PACKAGE_BY_PLATFORM[$runtime_platform]}")
done
unset runtime_platform
NIGHTLY_PACKAGES=("kandev" "${RUNTIME_PACKAGES[@]}")

readonly -a RUNTIME_PLATFORMS RUNTIME_PACKAGES NIGHTLY_PACKAGES
readonly -A RUNTIME_PACKAGE_BY_PLATFORM
