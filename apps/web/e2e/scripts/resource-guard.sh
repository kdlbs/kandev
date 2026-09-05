#!/usr/bin/env bash

# Shared resource limits for local Playwright entry points. Playwright's
# backend fixture is worker-scoped, so one worker per shard is the safe default.

e2e_allow_unsafe_parallelism() {
  [[ "${KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM:-}" == "1" ]]
}

e2e_available_memory_kib() {
  local meminfo_path="${1:-/proc/meminfo}"
  local cgroup_root="${2:-/sys/fs/cgroup}"
  local host_available=0
  if [[ -r "$meminfo_path" ]]; then
    host_available="$(awk '/^MemAvailable:/ { print $2; exit }' "$meminfo_path")"
  fi

  local cgroup_remaining=""
  cgroup_remaining="$(e2e_cgroup_remaining_kib "$cgroup_root" 2>/dev/null || true)"
  if [[ ! "$host_available" =~ ^[0-9]+$ ]]; then
    host_available=""
  fi
  if [[ ! "$cgroup_remaining" =~ ^[0-9]+$ ]]; then
    cgroup_remaining=""
  fi

  if [[ -n "$host_available" && -n "$cgroup_remaining" ]]; then
    if (( host_available < cgroup_remaining )); then
      printf '%s' "$host_available"
    else
      printf '%s' "$cgroup_remaining"
    fi
  elif [[ -n "$host_available" ]]; then
    printf '%s' "$host_available"
  elif [[ -n "$cgroup_remaining" ]]; then
    printf '%s' "$cgroup_remaining"
  else
    printf '0'
  fi
}

e2e_cgroup_remaining_kib() {
  local cgroup_root="$1"
  local limit_file="" usage_file="" limit="" usage=""
  if [[ -r "$cgroup_root/memory.max" && -r "$cgroup_root/memory.current" ]]; then
    limit_file="$cgroup_root/memory.max"
    usage_file="$cgroup_root/memory.current"
  elif [[ -r "$cgroup_root/memory/memory.limit_in_bytes" && -r "$cgroup_root/memory/memory.usage_in_bytes" ]]; then
    limit_file="$cgroup_root/memory/memory.limit_in_bytes"
    usage_file="$cgroup_root/memory/memory.usage_in_bytes"
  else
    return 1
  fi

  limit="$(<"$limit_file")"
  usage="$(<"$usage_file")"
  limit="${limit%%[[:space:]]*}"
  usage="${usage%%[[:space:]]*}"
  if [[ "$limit" == "max" || ! "$limit" =~ ^[0-9]+$ || ! "$usage" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  # cgroup v1 uses a very large number for an unlimited limit.
  if (( limit > 1152921504606846976 )); then
    return 1
  fi
  if (( usage >= limit )); then
    printf '0'
  else
    printf '%s' "$(( (limit - usage) / 1024 ))"
  fi
}

e2e_max_local_shards() {
  local available_kib
  available_kib="$(e2e_available_memory_kib "${1:-/proc/meminfo}" "${2:-/sys/fs/cgroup}")"

  # Unknown memory is not permission to start several browser stacks. Keep
  # the safe single-shard default until an actual host or cgroup budget is
  # available.
  local maximum=1
  if [[ "$available_kib" =~ ^[0-9]+$ && "$available_kib" != 0 ]]; then
    maximum=3
    (( available_kib < 12582912 )) && maximum=2
    (( available_kib < 6291456 )) && maximum=1
  fi
  printf '%s' "$maximum"
}

e2e_validate_shards() {
  local requested="$1"
  e2e_allow_unsafe_parallelism && return 0

  local maximum
  maximum="$(e2e_max_local_shards)"
  if (( requested > maximum )); then
    printf '[e2e] local shard limit is %s on this host; requested %s. Set KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM=1 only for a deliberate resource experiment.\n' "$maximum" "$requested" >&2
    return 1
  fi
}

e2e_validate_playwright_args() {
  e2e_allow_unsafe_parallelism && return 0

  local args=("$@")
  local index=0 argument value
  while (( index < ${#args[@]} )); do
    argument="${args[index]}"
    case "$argument" in
      -j=*)
        value="${argument#-j=}"
        ;;
      -j[0-9]*)
        value="${argument#-j}"
        ;;
      -j|--workers)
        index=$((index + 1))
        value="${args[index]:-}"
        ;;
      --workers=*)
        value="${argument#--workers=}"
        ;;
      *)
        index=$((index + 1))
        continue
        ;;
    esac

    if [[ "$value" != "1" ]]; then
      printf '[e2e] Playwright worker limit is 1 per shard; requested %s. Set KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM=1 only for a deliberate resource experiment.\n' "${value:-<missing>}" >&2
      return 1
    fi
    index=$((index + 1))
  done
}
