#!/usr/bin/env bash
set -euo pipefail
[[ $# == 2 && $2 =~ ^[1-9][0-9]*$ ]] || { echo 'Usage: prepare-full-worker-acceptance.sh manifest.json shard-index' >&2; exit 2; }
selected=$(node - "$1" "$2" <<'JS'
const fs = require('node:fs');
const manifest = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const shard = manifest.shards.find(row => row.index === Number(process.argv[3]));
if (!shard) throw new Error('Requested container shard is absent');
console.log(shard.files.includes('tests/kubernetes/kubernetes-session-resilience.spec.ts') ? 'yes' : 'no');
JS
)
[[ $selected == yes ]] || exit 0
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
: "${GITHUB_ENV:?Missing CI environment output}" "${RUNNER_TEMP:?Missing private build log directory}"
log="$RUNNER_TEMP/kandev-full-worker-acceptance-build.log"
# Build already enforces immutable inputs, 4 GiB/2 CPU and finite verification.
bash "$root/k8s/worker-images/full/build.sh" --build --verify | tee "$log"
image=$(sed -n 's/^IMAGE_ID=//p' "$log")
[[ $image =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Build did not return one exact verified image ID' >&2; exit 1; }
printf 'KANDEV_E2E_FULL_WORKER_IMAGE=%s\n' "$image" >> "$GITHUB_ENV"
