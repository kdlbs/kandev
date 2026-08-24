---
id: "02-add-claude-experiment-flag"
title: "Add Claude background prompt handoff experiment"
status: completed
wave: 2
depends_on: ["01-restore-coarse-running-policy"]
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 02: Add Claude background prompt handoff experiment

## Acceptance

- `features.claudeBackgroundPromptHandoff` is experimental, high risk,
  restart-required, and off in prod, dev, and E2E unless explicitly overridden.
- Flag off preserves Task 01's coarse admission and activity behavior.
- Flag on restores every ADR-0049 background mode for `claude-acp` sessions.
- Flag on never relaxes admission or activity for another real provider.
- Desktop and mobile Playwright cover both the default and opt-in contracts.

## Verification

- Targeted config, runtime-flag, orchestrator, handler, and frontend tests.
- Targeted desktop and mobile Playwright for coarse and experimental behavior.
- `make fmt`
- `make typecheck test lint`
- Public documentation validation.

## Files likely touched

- `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/orchestrator/`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/web/lib/state/slices/features/`
- `apps/web/e2e/tests/chat/`
- `docs/decisions/2026-07-28-coarse-running-busy-signal.md`
- `docs/specs/platform/requirements/background-work-liveness.md`
- `docs/public/configuration.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/websocket-api.md`

## Dependencies

Task 01 supplies the safe default and retains the internal tracker.

## Parallelism

Sequential. The config, admission, publication, and client contracts must use
one policy and be proven together.

## Verification Results

- `make fmt` — passed.
- `make typecheck test lint` — passed:
  - Go suite, including orchestrator concurrency tests.
  - Web: 922 files passed; 7,050 tests passed; 4 skipped.
  - CLI: 30 files and 280 tests passed.
  - Backend, web, and harness lint passed.
- `cd apps && pnpm --filter @kandev/web e2e:run --host tests/chat/busy-signal.spec.ts`
  — default-off desktop
  cases passed; the first opt-in run found and drove the persisted provider
  discriminator fix (`agent_name`, not the database UUID in `agent_id`).
- `cd apps && pnpm --filter @kandev/web e2e:run --host --no-build tests/chat/busy-signal.spec.ts -- --grep "Claude background prompt handoff experiment"`
  — 2 passed.
- `cd apps && pnpm --filter @kandev/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-busy-signal.spec.ts`
  — default-off mobile passed; the opt-in test exposed a desktop-keyboard test
  helper mismatch.
- `cd apps && pnpm --filter @kandev/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-busy-signal.spec.ts -- --grep "background-only work"`
  — 1 passed after switching the mobile flow to touch submission.
- Public documentation validation passed as part of `make test`.
