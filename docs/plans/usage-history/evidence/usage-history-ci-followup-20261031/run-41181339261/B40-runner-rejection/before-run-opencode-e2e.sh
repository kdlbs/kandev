#!/usr/bin/env bash
set -euo pipefail
source /tmp/usage-history-luna-B38-prepared/gate.sh
b38_require opencode-e2e
b38_require_web_build
b38_require_backend_artifacts
b38_protect_assets
b38_run_dir opencode-e2e
base=(env -u CAPTURE_PR_ASSETS -u KANDEV_E2E_SKIP_FRESHNESS GOMAXPROCS=1 E2E_PORT_OFFSET=38 pnpm --dir "$B38_REPO/apps/web" e2e:run --host --no-build --shards 1 --project chromium)
"${base[@]}" tests/session/opencode-capacity-retry.spec.ts -- --grep 'desktop preserves the episode when the OpenCode continuation fails without new tools' --repeat-each=10 --retries=0 --reporter=list --output="$B38_RUN_DIR/opencode-repeat10-results" 2>&1 | tee "$B38_RUN_DIR/opencode-repeat10.log"
"${base[@]}" tests/session/opencode-capacity-retry.spec.ts -- --retries=0 --reporter=list --output="$B38_RUN_DIR/opencode-fullfile-results" 2>&1 | tee "$B38_RUN_DIR/opencode-fullfile.log"
"${base[@]}" tests/session/new-session-advanced-settings.spec.ts tests/session/opencode-capacity-retry.spec.ts tests/settings/executor-profile-card-keyboard.spec.ts -- --retries=0 --reporter=list --output="$B38_RUN_DIR/opencode-ordered-three-results" 2>&1 | tee "$B38_RUN_DIR/opencode-ordered-three.log"
