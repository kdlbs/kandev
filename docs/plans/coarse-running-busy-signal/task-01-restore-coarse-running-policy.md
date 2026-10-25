---
id: "01-restore-coarse-running-policy"
title: "Restore coarse running policy"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 01: Restore coarse running policy

## Acceptance

- Every `RUNNING` session rejects direct prompt admission even when the private
  tracker reports foreground-idle background work.
- Boot/REST/WS/task activity surfaces expose `generating` for `RUNNING` and do
  not expose a settled background tier.
- Desktop and mobile composer flows queue input during a held-open background
  turn and retain that behavior after a fresh reload.

## Verification

- `make -C apps/backend test`
- `cd apps && pnpm --filter @kandev/web e2e:run -- tests/chat/busy-signal.spec.ts -- --grep "held-open background turn remains busy"`
- `cd apps && pnpm --filter @kandev/web e2e:run -- tests/chat/mobile-busy-signal.spec.ts -- --grep "held-open background turn remains busy"`
- `make fmt`
- `make typecheck test lint`

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/turn_activity.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/foreground_busy_signal_test.go`
- `apps/backend/internal/orchestrator/foreground_activity_signal_test.go`
- `apps/web/e2e/tests/chat/busy-signal.spec.ts`
- `apps/web/e2e/tests/chat/mobile-busy-signal.spec.ts`
- `docs/specs/platform/requirements/background-work-liveness.md`
- `docs/decisions/0049-fine-grained-foreground-idle-busy-signal.md`
- `docs/decisions/2026-07-28-coarse-running-busy-signal.md`
- `docs/decisions/INDEX.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/websocket-api.md`

## Dependencies

None.

## Parallelism

Sequential. Admission and all activity surfaces share one policy boundary.

## Inputs

- Background work liveness spec: coarse admission and display scenarios.
- ADR-2026-07-28: retain dormant accounting while disabling policy use.
- Existing tracker and busy-signal tests around ADR-0049.

## Output contract

Report the root cause, files changed, RED/GREEN evidence, E2E results, required
verification results, commit receipt, remaining risks, and task/plan status.

## Results

- RED: targeted orchestrator tests failed because background-idle was admitted
  and exported as `background`; desktop and mobile E2E timed out waiting for the
  coarse `generating` value.
- GREEN: targeted orchestrator and mock-agent regressions passed.
- GREEN: desktop Chromium and mobile Chrome held-open background scenarios
  queued input and remained generating across reload.
- GREEN: `make fmt`.
- GREEN: `make typecheck test lint`.
- GREEN: public documentation validation tests and source validation.
