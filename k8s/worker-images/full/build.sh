#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "$here/pins.env"
mode=${1:---check}
[[ "$mode" == --check || "$mode" == --build ]] || { echo 'Usage: build.sh --check|--build [--verify]' >&2; exit 2; }
[[ $# -le 2 && (${2:-} == '' || ${2:-} == --verify) ]] || exit 2
for key in BASE_IMAGE DOCKER_IMAGE BUILDKIT_IMAGE; do
  [[ ${!key} =~ @sha256:[a-f0-9]{64}$ ]] || { echo "Missing immutable $key" >&2; exit 1; }
done
for key in GOLANGCI_SHA256 PLAYWRIGHT_SHA256 CHROMIUM_SHA256; do
  [[ ${!key} =~ ^[a-f0-9]{64}$ ]] || { echo "Missing checksum $key" >&2; exit 1; }
done
for key in GOLANGCI_VERSION PLAYWRIGHT_VERSION CHROMIUM_VERSION CHROMIUM_REVISION PNPM_VERSION; do
  [[ ${!key} =~ ^[0-9]+(\.[0-9]+)*$ ]] || { echo "Missing version $key" >&2; exit 1; }
done
[[ "$PLATFORM" == linux/amd64 ]]
grep -Fq "$DOCKER_IMAGE" "$here/pod-template.yaml"
for file in Dockerfile smoke.sh prepare.sh pod-template.yaml; do test -s "$here/$file"; done
if [[ "$mode" == --check ]]; then echo 'Full worker immutable inputs and recipe files valid'; exit; fi
# The dedicated builder enforces limits; shell variables alone do not bound builds.
free_kb=$(df -Pk "$here" | awk 'END {print $4}')
(( free_kb >= 20*1024*1024 )) || { echo 'Build requires at least 20 GiB free disk' >&2; exit 1; }
scratch=$(mktemp -d)
builder="kandev-full-$(basename "$scratch" | tr '[:upper:].' '[:lower:]-')"
tag=${FULL_WORKER_TAG:-"kandev-full-worker:$builder"}
if docker image inspect "$tag" >/dev/null 2>&1; then echo 'Refusing to overwrite an existing image tag' >&2; exit 1; fi
container="buildx_buildkit_${builder}0"
verify_started=false
verify_container="${builder}-verify"
cleanup() {
  if [[ "$verify_started" == true ]]; then
    docker rm --force "$verify_container" >/dev/null 2>&1 || true
  fi
  docker buildx rm "$builder" >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
printf '[worker.oci]\n  max-parallelism = 1\n  gc = true\n  reservedSpace = "1GB"\n  maxUsedSpace = "12GB"\n' > "$scratch/buildkitd.toml"
docker buildx create --name "$builder" --driver docker-container \
 --driver-opt "image=$BUILDKIT_IMAGE,memory=4g,memory-swap=4g,cpu-period=100000,cpu-quota=200000" \
 --buildkitd-config "$scratch/buildkitd.toml" >/dev/null
timeout 120 docker buildx inspect "$builder" --bootstrap
limits=$(docker inspect --format '{{.HostConfig.Memory}} {{.HostConfig.MemorySwap}} {{.HostConfig.CpuPeriod}} {{.HostConfig.CpuQuota}}' "$container")
[[ "$limits" == '4294967296 4294967296 100000 200000' ]] || { echo "Builder limits not enforced: $limits" >&2; exit 1; }
args=()
for key in BASE_IMAGE DOCKER_IMAGE GOLANGCI_VERSION GOLANGCI_SHA256 PLAYWRIGHT_VERSION PLAYWRIGHT_SHA256 CHROMIUM_VERSION CHROMIUM_REVISION CHROMIUM_SHA256; do args+=(--build-arg "$key=${!key}"); done
timeout 1800 docker buildx build --builder "$builder" --platform "$PLATFORM" --load --progress plain \
 "${args[@]}" -t "$tag" "$here"
id=$(docker image inspect --format '{{.Id}}' "$tag")
printf 'FULL_WORKER_IMAGE=%s\nIMAGE_ID=%s\nPLATFORM=%s\nBUILD_LIMITS=%s\n' "$tag" "$id" "$PLATFORM" "$limits"
if [[ ${2:-} == --verify ]]; then
  verify_started=true
  timeout 600 docker run --rm --name "$verify_container" --init --cpus=2 --memory=4g --memory-swap=4g --pids-limit=512 \
    --user 1000:1000 --cap-drop=ALL --security-opt=no-new-privileges \
    --tmpfs /workspace:rw,exec,uid=1000,gid=1000,size=2g --tmpfs /run/kandev:rw,uid=1000,gid=1000,size=16m \
    -e HOME=/run/kandev/home "$id" bash -ceu 'mkdir -p "$HOME"; /opt/full-worker/smoke.sh --tools'
fi
