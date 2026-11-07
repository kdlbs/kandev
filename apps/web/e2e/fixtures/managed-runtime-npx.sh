#!/bin/sh
set -eu

cache_root=${NPM_CONFIG_CACHE:-${npm_config_cache:-"$HOME/.npm"}}
package_spec=${3:-}
preference=${2:-}
shift_count=3
real_npx=${E2E_REAL_NPX:-${KANDEV_E2E_REAL_NPX:-/usr/bin/npx}}
mock_agent=${E2E_MOCK_AGENT_PATH:-/usr/local/bin/mock-agent}
failure_mode=${E2E_NPX_MODE:-stale-etarget}
launch_id=${E2E_NPX_LAUNCH_ID:-default}
runtime_prefix=

# Parse npm options before the exact package, keeping agent arguments intact.
argument_index=0
expect_value=
for argument in "$@"; do
	argument_index=$((argument_index + 1))
	if [ -n "$expect_value" ]; then
		case "$expect_value" in
			prefix) runtime_prefix=$argument ;;
			cache) : ;;
		esac
		expect_value=
		continue
	fi
	case "$argument" in
		--prefix) expect_value=prefix ;;
		--cache) expect_value=cache ;;
		opencode-ai@*|@opencode/cli@*)
			package_spec=$argument
			shift_count=$argument_index
			break
			;;
	esac
done

case "$runtime_prefix" in
	*/kandev-managed-runtime-*/project)
		# Execute the package installed by real npm from its isolated cache.
		if [ -n "${E2E_PRIVATE_ROOT_MARKER:-}" ]; then
			mkdir -p "$(dirname "$E2E_PRIVATE_ROOT_MARKER")"
			printf '%s\n' "$runtime_prefix" >> "$E2E_PRIVATE_ROOT_MARKER"
		fi
		if [ -x "$real_npx" ]; then exec "$real_npx" "$@"; fi
		if [ -x /usr/local/lib/node_modules/npm/bin/npx-cli.js ]; then
			exec /usr/local/lib/node_modules/npm/bin/npx-cli.js "$@"
		fi
		exec npm exec "$@"
		;;
esac

if [ "${KANDEV_E2E_NPX_BYPASS_FAILURE:-false}" = "true" ]; then
	shift "$shift_count"
	exec "$mock_agent" "$@"
fi

# This fixture replaces npx only to make the selected managed runtime failure
# deterministic. Let every other package or invocation use the environment's
# real npm implementation unless the host test explicitly mocks it.
case "$package_spec" in
	opencode-ai@*|@opencode/cli@*) ;;
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
		missing-root-manifest)
			package_name=${package_spec%@*}
			package_version=${package_spec##*@}
			package_base=${package_name##*/}
			case "$package_name" in
				opencode-ai|@opencode/cli) bin_name=opencode ;;
				@openai/codex) bin_name=codex ;;
				*) bin_name=$package_base ;;
			esac
			package_dir="$startup_tree/node_modules/$package_name"
			if [ ! -f "$package_dir/package.json" ]; then
				mkdir -p "$package_dir/bin" "$startup_tree/node_modules/.bin" "$startup_sibling"
				printf '{"name":"%s","version":"%s","bin":{"%s":"bin/%s.js"}}\n' \
					"$package_name" "$package_version" "$bin_name" "$bin_name" > "$package_dir/package.json"
				printf '#!/bin/sh\nexit 1\n' > "$package_dir/bin/$bin_name.js"
				chmod 755 "$package_dir/bin/$bin_name.js"
				ln -s "../$package_name/bin/$bin_name.js" "$startup_tree/node_modules/.bin/$bin_name"
				printf 'preserved\n' > "$startup_tree/retry-sentinel-$safe_launch_id"
				printf 'preserved\n' > "$sibling_sentinel"
			fi
			if [ ! -f "$startup_tree/package.json" ]; then
				printf 'npm error code ENOENT\n' >&2
				printf 'npm error syscall open\n' >&2
				printf 'npm error path %s\n' "$startup_tree/package.json" >&2
				printf 'npm error errno -2\n' >&2
				printf "npm error enoent Could not read package.json: Error: ENOENT: no such file or directory, open '%s'\n" "$startup_tree/package.json" >&2
				exit 1
			fi
			shift "$shift_count"
			exec "$mock_agent" "$@"
			;;
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
	if [ -n "${E2E_NPX_RETRY_RELEASE_FILE:-}" ]; then
		: > "$E2E_NPX_RETRY_RELEASE_FILE.waiting"
		remaining=600
		while [ ! -f "$E2E_NPX_RETRY_RELEASE_FILE" ]; do
			remaining=$((remaining - 1))
			if [ "$remaining" -eq 0 ]; then
				printf 'managed runtime retry fixture was not released\n' >&2
				exit 98
			fi
			sleep 0.1
		done
		rm -f "$E2E_NPX_RETRY_RELEASE_FILE.waiting"
	fi
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
