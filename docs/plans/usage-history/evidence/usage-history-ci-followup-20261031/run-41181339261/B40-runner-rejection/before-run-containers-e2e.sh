#!/usr/bin/env bash
set -euo pipefail
source /tmp/usage-history-luna-B38-prepared/gate.sh
b38_require containers-e2e
b38_require_web_build
b38_require_backend_artifacts
b38_protect_assets
b38_run_dir containers-e2e
readonly GO_BIN=/root/.local/share/mise/installs/go/1.26.0/bin/go
[[ -x "$GO_BIN" ]] || b38_die "pinned Go 1.26.0 executable is unavailable"
# Keep the normal Makefile flags/version; these targets only write helper binaries.
make -C "$B38_REPO/apps/backend" "GO=$GO_BIN" build-agentctl-linux build-mock-agent-linux 2>&1 | tee "$B38_RUN_DIR/container-helper-build.log"
b38_require_backend_artifacts
base=(env -u CAPTURE_PR_ASSETS -u KANDEV_E2E_SKIP_FRESHNESS GOMAXPROCS=1 E2E_PORT_OFFSET=38 pnpm --dir "$B38_REPO/apps/web" e2e:run --host --no-build --shards 1 --project containers)
"${base[@]}" tests/git/turn-changed-files-executors.spec.ts -- --grep 'keeps exported turn content after container removal and backend restart' --repeat-each=10 --retries=0 --reporter=list --output="$B38_RUN_DIR/containers-repeat10-results" 2>&1 | tee "$B38_RUN_DIR/containers-repeat10.log"
"${base[@]}" tests/git/turn-changed-files-executors.spec.ts -- --retries=0 --reporter=list --output="$B38_RUN_DIR/containers-fullfile-results" 2>&1 | tee "$B38_RUN_DIR/containers-fullfile.log"
