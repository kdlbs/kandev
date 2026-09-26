#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

# Match the native bundle matrix (CGO enabled) and all four remote helper
# targets (CGO disabled). Platform build constraints can hide imports from a
# host-only `go list` check.
release_targets=(
  "linux/amd64/0"
  "linux/arm64/0"
  "darwin/amd64/0"
  "darwin/arm64/0"
  "linux/amd64/1"
  "linux/arm64/1"
  "darwin/amd64/1"
  "darwin/arm64/1"
  "windows/amd64/1"
)

for target in "${release_targets[@]}"; do
  IFS=/ read -r target_os target_arch target_cgo <<<"$target"
  deps_file="$temp_dir/deps-$target_os-$target_arch-$target_cgo.txt"
  error_file="$temp_dir/error-$target_os-$target_arch-$target_cgo.txt"
  printf 'Checking agentctl dependency graph for %s (CGO_ENABLED=%s)\n' "$target_os/$target_arch" "$target_cgo"

  if ! (
    cd "$repo_root/apps/backend"
    GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED="$target_cgo" go list -deps ./cmd/agentctl
  ) >"$deps_file" 2>"$error_file"; then
    cat "$error_file" >&2
    printf 'failed to list agentctl dependencies for %s (CGO_ENABLED=%s)\n' "$target_os/$target_arch" "$target_cgo" >&2
    exit 1
  fi

  forbidden="$(grep -E '(^|/)internal/task/service$|^k8s\.io/' "$deps_file" || true)"
  if [[ -n "$forbidden" ]]; then
    forbidden_count="$(printf '%s\n' "$forbidden" | wc -l | tr -d ' ')"
    printf 'agentctl dependency graph for %s (CGO_ENABLED=%s) includes %s forbidden packages; first matches:\n' "$target_os/$target_arch" "$target_cgo" "$forbidden_count" >&2
    printf '%s\n' "$forbidden" | sed -n '1,12p' >&2
    exit 1
  fi
done

printf 'all release agentctl dependency graphs exclude task/service and Kubernetes packages\n'
