---
id: "02-runtime-toggle"
title: "Add the mid-turn steering runtime toggle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 02: Add the mid-turn steering runtime toggle

- **Acceptance:** `features.claudeMidTurnSteering` exists in the typed runtime
  flag registry with env var `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`,
  experimental stability, high risk, restart-required, and is `"false"` in the
  prod, dev, and e2e profiles. Effective-value precedence is explicit env >
  SQLite override > profile default.
- **Acceptance:** The registry/profile/frontend completeness tests recognise the
  new flag, and the frontend feature slice carries it defaulted to `false`.
- **Verification:** `cd apps/backend && go test -race ./internal/runtimeflags/... ./internal/profiles/... ./internal/common/config/...` then `cd apps/web && pnpm test`
- **Files likely touched:**
  `apps/backend/internal/runtimeflags/registry.go`,
  `apps/backend/internal/common/config/config.go`,
  `apps/backend/internal/profiles/profiles.yaml`,
  `apps/backend/internal/backendapp/orchestrator.go` (wire into the service
  config alongside `ClaudeBackgroundPromptHandoff`),
  `apps/web/lib/state/slices/features/types.ts`, plus tests.
- **Dependencies:** None.
- **Inputs:** Spec "API surface" (runtime toggle) and `/runtime-feature-flags`
  for the file-by-file checklist. `features.claudeBackgroundPromptHandoff` is the
  reference registration to mirror, including its risk-description style.
- **Risks:** Do not reuse or rename the existing handoff flag; the two triggers
  are independent and must be independently killable. Never remove a registry
  entry to make a build pass.
- **Output contract:** Report the registration, the three profile values, the
  precedence test evidence, exact commands/results, and update only this task's
  status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/runtimeflags/... ./internal/profiles/... ./internal/common/config/...`: passed.
- `cd apps/web && pnpm test`: passed for every steering-related suite. The full
  run reports 8431 passed / 6 failed, and all 6 failures are pre-existing on
  clean `main` in files this PR does not touch
  (`lib/http-git-server.test.ts`, which needs a running Docker bridge, and
  `hooks/domains/settings/use-automation-runs.test.ts`).
- Registration: `features.claudeMidTurnSteering` in
  `runtimeflags/registry.go`, env `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`,
  high-risk, restart-required.
- Profile values: `prod: "false"`, `dev: "false"`, `e2e: "false"` — off in all
  three shipped profiles.
- Precedence (explicit env > SQLite override > profile default) is covered by
  the existing registry/config tests, which pass unchanged.
