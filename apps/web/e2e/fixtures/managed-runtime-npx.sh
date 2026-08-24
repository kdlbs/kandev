#!/bin/sh
set -eu

cache_root=${NPM_CONFIG_CACHE:-${npm_config_cache:-"$HOME/.npm"}}
package_spec=${3:-}
preference=${2:-}

if [ "$preference" = "--prefer-offline" ]; then
  key=$(printf '%s' "$package_spec" | sha512sum | cut -c1-16)
  mkdir -p "$cache_root/_npx/$key" "$cache_root/_npx/0123456789abcdef"
  printf 'npm error code ETARGET\n' >&2
  printf 'npm error notarget No matching version found for %s\n' "$package_spec" >&2
  exit 1
fi

if [ "$preference" = "--prefer-online" ]; then
  shift 4
  exec /usr/local/bin/mock-agent "$@"
fi

printf 'unexpected npx preference: %s\n' "$preference" >&2
exit 1
