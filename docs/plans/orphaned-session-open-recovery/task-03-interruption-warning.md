---
id: "03-interruption-warning"
title: "Keep interruption warnings until recovery succeeds"
status: done
wave: 3
depends_on:
  - "02-recover-orphan-conversations"
plan: "plan.md"
requirements:
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
acceptance_criteria:
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.2
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.4
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.5
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.6
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.7
system_design:
  - ../../specs/tasks/system-design/interrupted-task-indicator.md
---

# Task 03: Keep interruption warnings until recovery succeeds

## Summary

Show a warning triangle for interrupted tasks until successful agent recovery.
Preserve the marker through delayed, failed, or blocked resume attempts.

## In scope

- Change both shared interruption icon paths from red circle to warning triangle.
- Replace STARTING-based marker clearing with confirmed recovery success.
  Include prompt-free recovery to WAITING_FOR_INPUT and stale callback protection.
- Extend tests for restart marking, missing executor rows, repeated restart,
  failed attempts, live removal, and existing icon precedence.
- Update public recovery documentation and affected scoped guidance with implementation.

## Out of scope

Other error icons, new task states, manual warning dismissal, and new mobile controls.

## Acceptance

1. Sidebar and task cards show the warning triangle while interrupted work remains unresumed.
2. Attempts retain the marker; successful recovery clears it across clients. Stale callbacks cannot clear newer interruption state.
3. Desktop and phone coverage proves shape, warning style, accessible meaning, persistence, and recovery lifecycle.

## ASCII UI preview

UI-02, [full plan](plan.md#ascii-ui-preview), covers all criteria in this work order.

```text
Before, sidebar/card: (red !) Task title
After, sidebar/card:  /!\     Task title
                      warning color
Phone drawer/card:   /!\     Task title
During startup:      [normal progress; marker retained]
After success:       [normal task status] Task title
Failed attempt:      [interruption retained + existing recovery error]
```

The warning occupies the existing status slot. Desktop and phone share the
semantic change, with existing layouts and scroll owners. Triangle shape and
accessible label are required; ASCII spacing is illustrative. No new touch
control is added. Use the phone task drawer as the shipped mobile exemplar.

## Verification

Write failing marker-lifecycle and shared-icon tests before implementation.
From the repository root, after the package's dependency installation:

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Interrupted|Startup|Retracked|Reconcile)' -count=1)
(cd apps/web && pnpm exec vitest run lib/ui/state-icons.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/ui/state-icons.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-interrupted-icon.spec.ts tests/session/session-resume-recovery.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-interrupted-icon.spec.ts tests/session/mobile-session-resume-recovery.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Add the new mobile file in this work order. Use the configured mobile project
and touch task selection. Extend existing desktop coverage to include actual
board cards, not only the sidebar. Capture both views and assert shape/style,
accessible label, and removal after successful recovery. Keep delayed-resume
and failure cases in the existing recovery suites. Run projects sequentially.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/orchestrator/service.go` and restart tests.
- `apps/backend/internal/task/repository/sqlite/` if conditional metadata removal is needed.
- `apps/web/lib/ui/state-icons.tsx` and `state-icons.test.tsx`
- `apps/web/e2e/tests/task/task-interrupted-icon.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-interrupted-icon.spec.ts` (new)
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 02 provides the recovery path whose success clears the marker.

## Risks

Clearing on STARTING hides failed recovery. Clearing on every WAITING_FOR_INPUT
transition would also erase markers during restart reconciliation. Use confirmed
execution readiness, not state names alone. Preserve unrelated metadata keys.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/interrupted-task-indicator.md)
- [Design](../../specs/tasks/system-design/interrupted-task-indicator.md)
- `task-interrupted-icon.spec.ts` and `mobile-sidebar-workflow-completion-icon.spec.ts`.

## Results

Implemented warning-triangle rendering for the shared sidebar and task-card
state icon, including the accessible “Interrupted by restart” label and warning
focus styling on desktop and phone. The durable marker is retained through
STARTING entry, request acceptance, failed recovery, and stale snapshots. A
compare-and-set metadata removal clears it only after confirmed provider
recovery, so a late callback cannot erase a newer interruption marker.

Added backend marker-lifecycle and metadata-CAS tests, frontend state-icon and
workflow-snapshot tests, and desktop/mobile E2E assertions for the rendered
triangle and warning color. Typecheck and targeted lint pass.

The review remediation keeps the marker generation through failed, cancelled,
and tombstoned attempts. Empty or unreadable attempt snapshots cannot clear a
newer marker. Reconciliation publishes the committed marker through
`task.updated`, including its explicit interruption projection, so connected
clients see the warning immediately without a reload. The client task merge
also records every explicit marker update, preserving a newer false marker
after a complete interruption episode races an in-flight workflow snapshot.
