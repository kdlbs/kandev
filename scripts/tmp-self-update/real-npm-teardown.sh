#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"

if command -v kandev >/dev/null 2>&1; then
  kandev service uninstall || true
fi

rm -rf "$TEST_HOME"
echo "[self-update-real-npm] removed $TEST_HOME"
