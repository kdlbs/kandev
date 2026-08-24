#!/bin/sh
set -eu

cache_root=${NPM_CONFIG_CACHE:-${npm_config_cache:-"$HOME/.npm"}}
package_spec=${3:-}
preference=${2:-}
managed_package_spec=opencode-ai@1.18.18
real_npx=/usr/bin/npx

# This image replaces npx only to make the selected managed runtime failure
# deterministic. Let every other package or invocation use the image's real
# npm implementation.
if [ "$package_spec" != "$managed_package_spec" ]; then
	exec "$real_npx" "$@"
fi

key=$(printf '%s' "$package_spec" | sha512sum | cut -c1-16)
target_dir="$cache_root/_npx/$key"
sibling_dir="$cache_root/_npx/0123456789abcdef"
online_invocations="$cache_root/online-invocations"

if [ "$preference" = "--prefer-offline" ]; then
	mkdir -p "$target_dir" "$sibling_dir"
	printf 'stale\n' > "$target_dir/stale-marker"
	printf 'sibling\n' > "$sibling_dir/sibling-marker"
	printf 'npm error code ETARGET\n' >&2
	printf 'npm error notarget No matching version found for %s\n' "$package_spec" >&2
	exit 1
fi

if [ "$preference" = "--prefer-online" ]; then
	if [ -e "$target_dir/stale-marker" ]; then
		printf 'stale managed runtime marker was not removed for %s\n' "$package_spec" >&2
		exit 1
	fi
	mkdir -p "$target_dir"
	printf 'fresh\n' > "$target_dir/fresh-marker"
	printf '%s\n' "$package_spec" >> "$online_invocations"
	shift 3
	exec /usr/local/bin/mock-agent "$@"
fi

exec "$real_npx" "$@"
