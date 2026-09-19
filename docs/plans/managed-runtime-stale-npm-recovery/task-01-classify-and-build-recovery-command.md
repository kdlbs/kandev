---
id: "01-classify-and-build-recovery-command"
title: "Classify stale npm metadata"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 01: Classify stale npm metadata

Create the strict error signal and trusted recovery primitives before changing
the launch lifecycle.

- **Acceptance:** Classification requires npm `ETARGET` and the matching
  missing `package@version` message in bounded stderr. Generic ACP disconnects,
  network errors, and unrelated npm failures do not match.
- **Acceptance:** Normal managed runtime commands keep `--prefer-offline`. A
  trusted recovery option changes only that flag to `--prefer-online` and keeps
  the same package, version, and ACP arguments.
- **Acceptance:** One shared cache helper derives and removes only the exact
  `_npx` execution key for the trusted package spec. It rejects broad, escaped,
  and symlinked targets and is idempotent when the key is absent.
- **Acceptance:** Runtime update jobs use the shared helper without changing
  their current one-retry behavior.
- **Verification:** Add failing tests first, then run:

  ```bash
  cd apps/backend
  go test ./internal/agent/runtime/routingerr ./internal/agent/agents ./internal/agent/managedruntime
  go test ./internal/agent/settings/controller -run 'Test.*ExecutionCache|Test.*AgentUpdate'
  ```

- **Files likely touched:**
  `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`,
  `apps/backend/internal/agent/runtime/routingerr/runtime_rules_test.go`,
  `apps/backend/internal/agent/agents/managed_npm_runtime.go`,
  `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`,
  `apps/backend/internal/agent/managedruntime/cache.go`,
  `apps/backend/internal/agent/managedruntime/cache_test.go`, and
  `apps/backend/internal/agent/settings/controller/agent_update.go`.
- **Dependencies:** None.
- **Parallelism:** sequential foundation.
- **Inputs:** Launch-time stale metadata recovery and failure behavior in the
  managed runtime spec.
- **Output contract:** Report files changed, RED and GREEN commands and results,
  classifier false-positive evidence, cache deletion safety evidence, and
  synchronized task and plan status.
