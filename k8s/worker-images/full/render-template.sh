#!/usr/bin/env bash
set -euo pipefail
image=${1:-}
[[ $# == 1 && "$image" =~ ^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$ ]] || {
  echo 'Usage: render-template.sh registry/image@sha256:<64 lowercase hex digits>' >&2
  exit 2
}
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
sed "s|FULL_WORKER_IMAGE_REQUIRED|$image|g" "$here/pod-template.yaml"
