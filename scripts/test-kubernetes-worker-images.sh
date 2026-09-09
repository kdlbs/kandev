#!/usr/bin/env bash
set -euo pipefail

script_dir=$(dirname "${BASH_SOURCE[0]}")
repo_root=$(cd "$script_dir/.." && pwd)
image_dir="$repo_root/k8s/worker-images"
pin_file="$image_dir/pins.env"
template_dir="$repo_root/k8s/presets"
targets=(minimal node-pnpm python)

mode=check
platform=

usage() {
  cat <<'EOF'
Usage: scripts/test-kubernetes-worker-images.sh [--check | --smoke] [--platform PLATFORM]

--check   validate pins, targets, and source PodTemplates without Docker
--smoke   build each target and run bounded non-root toolchain checks
EOF
}

die() {
  echo "worker image validation: $*" >&2
  exit 1
}

while (($# > 0)); do
  case "$1" in
    --check) mode=check ;;
    --smoke) mode=smoke ;;
    --platform)
      shift
      (($# > 0)) || die "--platform requires a value"
      platform=$1
      ;;
    --platform=*) platform=${1#--platform=} ;;
    -h|--help)
      usage
      exit 0
      ;;
    *) die "unknown argument $1" ;;
  esac
  shift
done

[[ -f "$pin_file" ]] || die "missing $pin_file"
[[ -f "$image_dir/Dockerfile" ]] || die "missing worker Dockerfile"
# shellcheck disable=SC1090
source "$pin_file"
platform=${platform:-${PLATFORM:-}}

if [[ -z "${BASE_IMAGE:-}" || ! "$BASE_IMAGE" =~ @sha256:[0-9a-f]{64}$ ]]; then
  die "BASE_IMAGE must end in an immutable sha256 digest"
fi
if [[ "$BASE_IMAGE" == *":latest"* ]]; then die "BASE_IMAGE must not use latest"; fi
if [[ -z "${PNPM_VERSION:-}" || ! "$PNPM_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "PNPM_VERSION must be a pinned semantic version"
fi
if [[ "$platform" != linux/amd64 ]]; then
  die "this validation currently supports linux/amd64 only"
fi

assert_file_contains() {
  local file=$1
  local pattern=$2
  grep -Fq -- "$pattern" "$file" || die "$file does not contain $pattern"
}

for target in "${targets[@]}"; do
  assert_file_contains "$image_dir/Dockerfile" "FROM base AS $target"
  template="$template_dir/$target.yaml"
  [[ -f "$template" ]] || die "missing $template"
  assert_file_contains "$template" "apiVersion: v1"
  assert_file_contains "$template" "kind: PodTemplate"
  assert_file_contains "$template" "name: kandev-agent"
  assert_file_contains "$template" "image: ghcr.io/kdlbs/kandev-worker@sha256:REPLACE_WITH_"
  assert_file_contains "$template" "runAsNonRoot: true"
  assert_file_contains "$template" "runAsUser: 1000"
  assert_file_contains "$template" "fsGroup: 1000"
  assert_file_contains "$template" "automountServiceAccountToken: false"
  assert_file_contains "$template" "allowPrivilegeEscalation: false"
  assert_file_contains "$template" "- ALL"
  assert_file_contains "$template" "cpu: 250m"
  assert_file_contains "$template" "memory: 512Mi"
  assert_file_contains "$template" "memory: 2Gi"
  if grep -nE '^[[:space:]]+(command|args|restartPolicy|volumeMounts|ports|HOME|KANDEV_)' "$template"; then
    die "$template contains a Kandev-owned field"
  fi
done

if [[ "$mode" == check ]]; then
  echo "worker image source check passed: ${targets[*]} ($platform)"
  exit 0
fi

command -v docker >/dev/null 2>&1 || die "Docker is required for --smoke"
docker info >/dev/null 2>&1 || die "Docker daemon is not available for --smoke"

declare -a created_images=()
declare -a created_volumes=()
cleanup() {
  set +e
  for volume in "${created_volumes[@]}"; do docker volume rm "$volume" >/dev/null 2>&1; done
  for image in "${created_images[@]}"; do docker image rm --force "$image" >/dev/null 2>&1; done
}
trap cleanup EXIT

for target in "${targets[@]}"; do
  tag="kandev-worker-smoke:${target}-${BASHPID}"
  volume="kandev-worker-smoke-${target}-${BASHPID}"
  created_images+=("$tag")
  created_volumes+=("$volume")
  docker build \
    --platform "$platform" \
    --build-arg "BASE_IMAGE=$BASE_IMAGE" \
    --build-arg "PNPM_VERSION=$PNPM_VERSION" \
    --target "$target" \
    --tag "$tag" \
    --file "$image_dir/Dockerfile" \
    "$repo_root" >/dev/null
  docker volume create "$volume" >/dev/null
  docker run --rm --network=none --platform "$platform" --user 1000:1000 \
    --env "PRESET=$target" --mount "type=volume,src=$volume,dst=/workspace" \
    --entrypoint /bin/sh "$tag" -ceu '
      test "$(id -u)" = 1000
      mkdir -p /workspace/.runtime /workspace/.cache/pip
      touch /workspace/.runtime/marker
      node --version
      npm --version
      python3 --version
      git --version
      git init /workspace/git-smoke >/dev/null
      git -C /workspace/git-smoke config user.email smoke@example.invalid
      git -C /workspace/git-smoke config user.name kandev-smoke
      printf "worker smoke\n" > /workspace/git-smoke/README.md
      git -C /workspace/git-smoke add README.md
      git -C /workspace/git-smoke commit -m smoke >/dev/null
      case "$PRESET" in
        node-pnpm)
          pnpm --version
          mkdir -p /tmp/smoke-package
          printf "{\"name\":\"kandev-smoke-package\",\"version\":\"1.0.0\",\"bin\":{\"kandev-smoke\":\"index.js\"}}\n" > /tmp/smoke-package/package.json
          printf "#!/bin/sh\nexit 0\n" > /tmp/smoke-package/index.js
          chmod +x /tmp/smoke-package/index.js
          npm install --global --prefix /workspace/.npm-global --no-audit --no-fund /tmp/smoke-package >/dev/null
          test -x /workspace/.npm-global/bin/kandev-smoke
          mkdir -p /workspace/pnpm-project
          printf "{\"name\":\"kandev-pnpm-smoke\",\"private\":true}\n" > /workspace/pnpm-project/package.json
          pnpm --dir /workspace/pnpm-project install --offline --store-dir /workspace/.pnpm-store >/dev/null
          ;;
        python)
          python3 -m venv /workspace/.venv
          /workspace/.venv/bin/python -c "import sys; assert sys.version_info.major == 3"
          test -d /workspace/.cache/pip
          ;;
        minimal)
          test -x "$(command -v git)"
          ;;
      esac
    '
  image_id=$(docker image inspect --format '{{.Id}}' "$tag")
  echo "worker image smoke passed: target=$target platform=$platform image_id=$image_id"
done
