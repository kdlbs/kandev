#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat >&2 <<EOF
Usage:
  $0 prepare --scheduled-sha <sha> --output <path>
  $0 publish --stable-at-start <version> --nightly-at-start <version> \\
    --tags-at-start <snapshot> --version <version> --assets-dir <path>
EOF
  exit 2
}

npm_version() {
  bash "$ROOT_DIR/scripts/release/npm-view-version.sh" "$1"
}

write_output() {
  printf '%s=%s\n' "$1" "$2" >> "$OUTPUT_PATH"
}

resolve_nightly_commit() {
  local version="$1"
  local label="$2"
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-nightly\.sha([0-9a-f]{12})$ ]]; then
    echo "$label has unsupported version '$version'; refusing to infer publication order." >&2
    return 1
  fi

  local sha="${BASH_REMATCH[1]}"
  local commit
  if ! commit="$(git rev-parse --verify "${sha}^{commit}" 2>/dev/null)"; then
    echo "Cannot resolve $label commit prefix $sha in main history." >&2
    return 1
  fi
  printf '%s\n' "$commit"
}

prepare() {
  local scheduled_sha=""
  OUTPUT_PATH=""
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --scheduled-sha)
        [[ "$#" -ge 2 ]] || usage
        scheduled_sha="$2"
        shift 2
        ;;
      --output)
        [[ "$#" -ge 2 ]] || usage
        OUTPUT_PATH="$2"
        shift 2
        ;;
      *) usage ;;
    esac
  done
  [[ -n "$scheduled_sha" && -n "$OUTPUT_PATH" ]] || usage

  write_output should_publish false

  local main_sha
  main_sha="$(git rev-parse HEAD)"
  if [[ "$main_sha" != "$scheduled_sha" ]]; then
    echo "Checked out $main_sha instead of scheduled SHA $scheduled_sha." >&2
    exit 1
  fi

  local latest_stable
  if ! latest_stable="$(npm_version kandev@latest)"; then
    echo "Could not resolve npm @latest; skipping this best-effort Nightly run."
    return
  fi
  if [[ -z "$latest_stable" ]]; then
    echo "npm @latest is missing; skipping this best-effort Nightly run."
    return
  fi
  write_output stable_version "$latest_stable"

  local nightly_version stable_commit
  nightly_version="$(node "$ROOT_DIR/scripts/release/nightly-version.mjs" "$latest_stable" "$main_sha")"
  stable_commit="$(git rev-parse "v${latest_stable}^{commit}")"
  if ! git merge-base --is-ancestor "$stable_commit" "$main_sha"; then
    echo "Latest stable v$latest_stable is not an ancestor of scheduled main $main_sha." >&2
    exit 1
  fi

  write_output version "$nightly_version"
  write_output tag "v$nightly_version"
  write_output ref "$main_sha"
  if [[ "$main_sha" == "$stable_commit" ]]; then
    echo "No commits on main since stable v$latest_stable; skipping nightly."
    return
  fi

  local published_nightly
  if ! published_nightly="$(npm_version kandev@nightly)"; then
    echo "Could not resolve npm @nightly; skipping this best-effort Nightly run."
    return
  fi
  write_output nightly_version_at_start "$published_nightly"

  # shellcheck source=npm-packages.sh
  source "$ROOT_DIR/scripts/release/npm-packages.sh"

  local published_commit=""
  if [[ -n "$published_nightly" ]]; then
    if ! published_commit="$(resolve_nightly_commit "$published_nightly" "kandev@nightly")"; then
      exit 1
    fi
    if [[ "$published_commit" != "$main_sha" ]] &&
      git merge-base --is-ancestor "$main_sha" "$published_commit"; then
      echo "A Nightly from a newer main commit is already published; skipping superseded commit $main_sha."
      return
    fi
    if ! git merge-base --is-ancestor "$published_commit" "$main_sha"; then
      echo "Published Nightly commit $published_commit is not an ancestor of scheduled main $main_sha." >&2
      exit 1
    fi
  fi

  local all_packages_published=true
  local nightly_tags_at_start=""
  local package published_version tagged_version tagged_commit
  for package in "${NIGHTLY_PACKAGES[@]}"; do
    if ! published_version="$(npm_version "${package}@$nightly_version")"; then
      echo "Could not verify ${package}@$nightly_version; skipping this best-effort Nightly run."
      return
    fi
    if ! tagged_version="$(npm_version "${package}@nightly")"; then
      echo "Could not verify ${package}@nightly; skipping this best-effort Nightly run."
      return
    fi
    nightly_tags_at_start+="${package}=${tagged_version};"

    if [[ -n "$published_version" ]]; then
      if [[ "$tagged_version" != "$nightly_version" ]]; then
        echo "${package}@$nightly_version exists but @nightly resolves to '${tagged_version:-nothing}'." >&2
        exit 1
      fi
      continue
    fi

    all_packages_published=false
    if [[ -n "$tagged_version" && "$tagged_version" != "$published_nightly" ]]; then
      if ! tagged_commit="$(resolve_nightly_commit "$tagged_version" "${package}@nightly")"; then
        exit 1
      fi
      if ! git merge-base --is-ancestor "$tagged_commit" "$main_sha"; then
        echo "${package}@nightly commit $tagged_commit is not an ancestor of scheduled main $main_sha." >&2
        exit 1
      fi
      echo "${package}@nightly is an older partial publication; the current publish will repair it."
    fi
  done
  write_output nightly_tags_at_start "$nightly_tags_at_start"

  if [[ -n "$published_nightly" && "$all_packages_published" == "true" ]]; then
    if [[ "$published_commit" != "$main_sha" ]]; then
      echo "Nightly version identity does not resolve to scheduled commit $main_sha." >&2
      exit 1
    fi
    echo "All Nightly packages are already published under @nightly; skipping."
    return
  fi

  write_output should_publish true
}

publish() {
  local stable_at_start=""
  local nightly_at_start=""
  local tags_at_start=""
  local version=""
  local assets_dir=""
  local nightly_set=false
  local tags_set=false
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --stable-at-start)
        [[ "$#" -ge 2 ]] || usage
        stable_at_start="$2"
        shift 2
        ;;
      --nightly-at-start)
        [[ "$#" -ge 2 ]] || usage
        nightly_at_start="$2"
        nightly_set=true
        shift 2
        ;;
      --tags-at-start)
        [[ "$#" -ge 2 ]] || usage
        tags_at_start="$2"
        tags_set=true
        shift 2
        ;;
      --version)
        [[ "$#" -ge 2 ]] || usage
        version="$2"
        shift 2
        ;;
      --assets-dir)
        [[ "$#" -ge 2 ]] || usage
        assets_dir="$2"
        shift 2
        ;;
      *) usage ;;
    esac
  done
  if [[ -z "$stable_at_start" || -z "$version" || -z "$assets_dir" ||
    "$nightly_set" != "true" || "$tags_set" != "true" ]]; then
    usage
  fi

  local current_latest
  if ! current_latest="$(npm_version kandev@latest)"; then
    echo "Could not revalidate npm @latest; skipping Nightly publication."
    return
  fi
  if [[ -z "$current_latest" ]]; then
    echo "npm @latest is missing; skipping Nightly publication."
    return
  fi
  if [[ "$current_latest" != "$stable_at_start" ]]; then
    echo "Stable npm baseline moved from $stable_at_start to $current_latest while this run was building; skipping stale Nightly publication."
    return
  fi

  local current_nightly
  if ! current_nightly="$(npm_version kandev@nightly)"; then
    echo "Could not revalidate npm @nightly; skipping Nightly publication."
    return
  fi
  if [[ "$current_nightly" != "$nightly_at_start" ]]; then
    echo "npm @nightly moved from ${nightly_at_start:-nothing} to ${current_nightly:-nothing} while this run was building; skipping superseded publication."
    return
  fi

  # shellcheck source=npm-packages.sh
  source "$ROOT_DIR/scripts/release/npm-packages.sh"
  local current_nightly_tags=""
  local package tagged_version
  for package in "${NIGHTLY_PACKAGES[@]}"; do
    if ! tagged_version="$(npm_version "${package}@nightly")"; then
      echo "Could not revalidate ${package}@nightly; skipping Nightly publication."
      return
    fi
    current_nightly_tags+="${package}=${tagged_version};"
  done
  if [[ "$current_nightly_tags" != "$tags_at_start" ]]; then
    echo "One or more npm @nightly tags moved while this run was building; skipping superseded publication."
    return
  fi

  bash "$ROOT_DIR/scripts/release/publish-npm.sh" \
    --version "$version" \
    --dist-tag nightly \
    --assets-dir "$assets_dir"
}

cd "$ROOT_DIR"
command="${1:-}"
[[ -n "$command" ]] || usage
shift
case "$command" in
  prepare) prepare "$@" ;;
  publish) publish "$@" ;;
  *) usage ;;
esac
