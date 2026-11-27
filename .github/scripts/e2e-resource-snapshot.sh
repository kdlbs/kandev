#!/usr/bin/env bash
set -euo pipefail

# Raw counters keep unsupported or unreadable resources distinguishable from zero.
printf '[observed_at_utc]\n'
date -u '+%Y-%m-%dT%H:%M:%SZ'
printf '\n[units]\nmemory.current, memory.peak, memory.max: bytes or max\ncpu.stat *_usec, cpu.max quota/period: microseconds or max\n'
printf '\n[available_cpu_count]\n'
nproc
printf '\n[cgroup_scope]\n'
cat /proc/self/cgroup
scope="$(awk -F: '$1 == "0" && $2 == "" { print $3; exit }' /proc/self/cgroup)"
counter_root=""
if [[ "$scope" == /* && -d "/sys/fs/cgroup$scope" ]]; then
  counter_root="/sys/fs/cgroup$scope"
fi
printf '\n[cgroup_counter_root]\n%s\n' "${counter_root:-unavailable}"
printf '\n[host_memory_kib]\n'
awk '/^(MemTotal|MemAvailable|SwapTotal|SwapFree):/' /proc/meminfo

for counter in cpu.max cpu.stat memory.current memory.peak memory.max memory.events memory.swap.current memory.swap.peak; do
  printf '\n[cgroup_%s]\n' "$counter"
  if [[ -n "$counter_root" && -r "$counter_root/$counter" ]]; then
    if ! cat "$counter_root/$counter"; then
      printf 'unavailable\n'
    fi
  else
    printf 'unavailable\n'
  fi
done
