#!/bin/sh
set -eu

cache_root=${NPM_CONFIG_CACHE:-${npm_config_cache:-"$HOME/.npm"}}
package_spec=${3:-}
preference=${2:-}
shift_count=3
managed_package_name=opencode-ai
real_npx=${KANDEV_E2E_REAL_NPX:-/usr/bin/npx}
mock_agent=${E2E_MOCK_AGENT_PATH:-/usr/local/bin/mock-agent}
failure_mode=${E2E_NPX_MODE:-stale-etarget}
launch_id=${E2E_NPX_LAUNCH_ID:-default}

if [ "${3:-}" = "--prefix" ]; then
	case "${4:-}" in
		"~/.kandev/managed-npm-runtime"|*/kandev-managed-npm-runtime-????????????????) ;;
		*)
			exec "$real_npx" "$@"
			;;
	esac
	package_spec=${5:-}
	shift_count=5
fi

if [ "${KANDEV_E2E_NPX_BYPASS_FAILURE:-false}" = "true" ]; then
	shift "$shift_count"
	exec "$mock_agent" "$@"
fi

# This fixture replaces npx only to make the selected managed runtime failure
# deterministic. Let every other package or invocation use the environment's
# real npm implementation unless the host test explicitly mocks it.
case "$package_spec" in
	"$managed_package_name"@*) ;;
	*)
		if [ "${KANDEV_E2E_NPX_MOCK_OTHERS:-false}" = "true" ]; then
			shift "$shift_count"
			exec "$mock_agent" "$@"
		fi
		exec "$real_npx" "$@"
		;;
esac

attempt_root="$cache_root/kandev-e2e-attempts"
safe_launch_id=$(printf '%s' "$launch_id" | tr -c 'A-Za-z0-9_-' '_')
attempt_file="$attempt_root/$safe_launch_id"
attempt_lock="$attempt_file.lock"
mkdir -p "$attempt_root"
while ! mkdir "$attempt_lock" 2>/dev/null; do sleep 0.01; done
attempt=0
if [ -f "$attempt_file" ]; then
	read -r attempt < "$attempt_file"
fi
attempt=$((attempt + 1))
printf '%s\n' "$attempt" > "$attempt_file.tmp.$$"
mv "$attempt_file.tmp.$$" "$attempt_file"
rmdir "$attempt_lock"

if [ "$failure_mode" != "stale-etarget" ]; then
	case "$(uname -s)" in
		Darwin) startup_key=$(printf '%s' "$package_spec" | shasum -a 512 | cut -c1-16) ;;
		*) startup_key=$(printf '%s' "$package_spec" | sha512sum | cut -c1-16) ;;
	esac
	startup_tree="$cache_root/_npx/$startup_key"
	startup_sibling="$cache_root/_npx/0123456789abcdef"
	startup_sentinel="$startup_tree/retry-sentinel-$safe_launch_id"
	sibling_sentinel="$startup_sibling/sibling-sentinel-$safe_launch_id"
	if [ "$attempt" = "1" ]; then
		mkdir -p "$startup_tree" "$startup_sibling"
		printf 'preserved\n' > "$startup_sentinel"
		printf 'preserved\n' > "$sibling_sentinel"
	fi
	printf '%s\t%s\t%s\t%s\n' "$safe_launch_id" "$attempt" "$failure_mode" "$preference" >> "$cache_root/kandev-e2e-launches"
	if [ "$attempt" -gt 1 ] && { [ ! -e "$startup_sentinel" ] || [ ! -e "$sibling_sentinel" ]; }; then
		printf 'managed runtime tree was not preserved across retry\n' >&2
		exit 97
	fi
	case "$failure_mode" in
		success)
			shift "$shift_count"
			exec "$mock_agent" "$@"
			;;
		transient)
			if [ "$attempt" = "1" ]; then
				printf 'npm error code ECONNRESET\n' >&2
				exit 1
			fi
			;;
		silent-exit)
			if [ "$attempt" = "1" ]; then exit 1; fi
			;;
		permanent)
			printf 'npm error code E401\n' >&2
			exit 1
			;;
		repeat-failure)
			printf 'npm error code ECONNRESET\n' >&2
			exit 1
			;;
		*)
			printf 'unknown managed runtime fixture mode: %s\n' "$failure_mode" >&2
			exit 2
			;;
	esac
	shift "$shift_count"
	exec "$mock_agent" "$@"
fi

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
		shift "$shift_count"
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
	mkdir -p "$target_dir"
	printf 'fresh\n' > "$target_dir/fresh-marker"
	printf '%s\n' "$package_spec" >> "$online_invocations"
	shift "$shift_count"
	exec "$mock_agent" "$@"
fi

exec "$real_npx" "$@"
