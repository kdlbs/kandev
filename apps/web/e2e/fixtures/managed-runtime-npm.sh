#!/bin/sh
set -eu

real_npm=/usr/local/bin/npm-real
if [ ! -x "$real_npm" ]; then
	real_npm=/usr/bin/npm
fi

if [ -n "${E2E_NPM_EXEC_LOG:-}" ]; then
	mkdir -p "$(dirname "$E2E_NPM_EXEC_LOG")"
	printf '%s\n' "$*" >> "$E2E_NPM_EXEC_LOG"
fi

exec "$real_npm" "$@"
