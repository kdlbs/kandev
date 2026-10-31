#!/usr/bin/env bash
set -euo pipefail
source /tmp/usage-history-luna-B38-prepared/gate.sh
b38_require mobile-plan-e2e
b38_require_web_build
b38_require_backend_artifacts
b38_protect_assets
b38_run_dir mobile-plan-e2e
base=(env -u CAPTURE_PR_ASSETS -u KANDEV_E2E_SKIP_FRESHNESS GOMAXPROCS=1 E2E_PORT_OFFSET=38 pnpm --dir "$B38_REPO/apps/web" e2e:run --host --no-build --shards 1 --project mobile-chrome)
"${base[@]}" tests/task/mobile-plan-download.spec.ts -- --grep 'keeps the download control touch-sized on a coarse-pointer workbench' --repeat-each=10 --retries=0 --reporter=list --output="$B38_RUN_DIR/mobile-plan-repeat10-results" 2>&1 | tee "$B38_RUN_DIR/mobile-plan-repeat10.log"
"${base[@]}" tests/task/mobile-plan-download.spec.ts -- --retries=0 --reporter=list --output="$B38_RUN_DIR/mobile-plan-fullfile-results" 2>&1 | tee "$B38_RUN_DIR/mobile-plan-fullfile.log"
