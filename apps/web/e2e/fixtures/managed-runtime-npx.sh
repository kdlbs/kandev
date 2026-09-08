#!/bin/sh
set -eu

cache_root=${NPM_CONFIG_CACHE:-${npm_config_cache:-"$HOME/.npm"}}
package_spec=${3:-}
preference=${2:-}
managed_package_name=opencode-ai
real_npx=${KANDEV_E2E_REAL_NPX:-/usr/bin/npx}
mock_agent=${KANDEV_E2E_MOCK_AGENT_PATH:-/usr/local/bin/mock-agent}

if [ "${KANDEV_E2E_NPX_BYPASS_FAILURE:-false}" = "true" ]; then
	shift 3
	exec "$mock_agent" "$@"
fi

# This fixture replaces npx only to make the selected managed runtime failure
# deterministic. Let every other package or invocation use the environment's
# real npm implementation unless the host test explicitly mocks it.
case "$package_spec" in
	"$managed_package_name"@*) ;;
	*)
		if [ "${KANDEV_E2E_NPX_MOCK_OTHERS:-false}" = "true" ]; then
			shift 3
			exec "$mock_agent" "$@"
		fi
		exec "$real_npx" "$@"
		;;
esac

case "$(uname -s)" in
	Darwin) key=$(printf '%s' "$package_spec" | shasum -a 512 | cut -c1-16) ;;
	*) key=$(printf '%s' "$package_spec" | sha512sum | cut -c1-16) ;;
esac
target_dir="$cache_root/_npx/$key"
sibling_dir="$cache_root/_npx/0123456789abcdef"
online_invocations="$cache_root/online-invocations"
offline_invocations="$cache_root/offline-invocations"

if [ "$preference" = "--prefer-offline" ]; then
	if [ -e "$target_dir/fresh-marker" ]; then
		shift 3
		exec "$mock_agent" "$@"
	fi
	mkdir -p "$target_dir" "$sibling_dir"
	printf 'stale\n' > "$target_dir/stale-marker"
	printf 'sibling\n' > "$sibling_dir/sibling-marker"
	printf '%s\n' "$package_spec" >> "$offline_invocations"
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
	exec "$mock_agent" "$@"
fi

exec "$real_npx" "$@"
