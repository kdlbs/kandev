#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"

if [ ! -d "$TEST_HOME" ]; then
  echo "[self-update-test] no test home at $TEST_HOME"
  exit 0
fi

rm -rf "$TEST_HOME"
echo "[self-update-test] removed $TEST_HOME"
