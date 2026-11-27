---
created: 2026-09-29
status: implemented
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-005
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
legacy_specs: []
---

# Implementation plan: Sidebar delete loading

## Overview

Extend the existing archive loading projection to deletion in one sequential
work order. Reuse the task-removal coordinator and shared desktop/phone rows.
The requested outcome is confirmed; no material product question remains.

## Scope

Include active/archived delete targets, bulk/cascade membership, recovery,
refresh races, and desktop/phone rendering. Exclude API/cleanup changes, new
navigation or notification behavior, global menu locking, and new settings.

## Technical approach

Follow the pending delete section in the
[removal design](../../specs/tasks/system-design/removal-navigation.md).
Generalize archive-specific pending names through existing projections and row
components. Keep archive's active-only rule while allowing deletion of archived
rows. Preserve coordinator lifecycle and archived-view filtering.
The completed removal-navigation, immediate-sidebar-archive, and
archive-progress-feedback packages remain historical evidence; this work extends
only row presentation and does not reopen their work orders.

## ASCII UI preview

UI-01: Desktop sidebar / reopened phone picker, delete accepted.

```text
Before              Pending                 Success
[o Task A    ...]   [~ Task A       ]        [o Task B ...]
[o Task B    ...]   [o Task B    ...]

Failure: [o Task A ...] returns with current task data.
```

`~` represents the existing muted spinner; pending A is dimmed, busy, and
retains its row space. These states are required; ASCII spacing is illustrative.
Phone uses the existing inset picker drawer and scrolling list, with its fixed
header, safe areas, and visible overflow entry point unchanged. Reopening the
picker during deletion must show the same state. Maps to AC-005.1 and AC-005.2.

## Tests

Extend `hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts` with deletion projection cases: pending before settlement, refresh retention, success,
refusal, wrong workspace/stale token, archived target, bulk partial failure,
cascade membership, and non-cascade survivors (AC-005.1 through AC-005.3).
Extend desktop/phone item projection and pending row component tests to cover
active and archived delete markers, spinner, dimming and accessible busy state.
Keep archive assertions. `use-workspace-sidebar-tasks.removal.test.tsx` proves
real-store publication and release during a deferred operation; coordinator tests
remain the mutation lifecycle regression suite.

## E2E tests

Add `task/sidebar-immediate-delete.spec.ts` (chromium) and
`task/mobile-sidebar-immediate-delete.spec.ts` (mobile-chrome), sharing
`sidebar-immediate-delete-helpers.ts`. Use the existing archive helper as the
pattern. Hold DELETE before forwarding to the server; assert the pending row,
then exercise refusal and success, and cancellation with no request (AC-005.1,
AC-005.2). Reopen the phone drawer while pending. Inspect both pending renders
against UI-01. Run existing archive scenarios to protect AC-005.3. No fixed sleeps.

## Work orders

- [x] [Task 01: Extend pending removal rows](task-01-pending-removal-rows.md)

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.removal.test.tsx components/task/task-session-sidebar-item-pending-archive.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/task-item-archive-pending.test.tsx hooks/use-task-removal-coordinator.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --project=chromium e2e/tests/task/sidebar-immediate-delete.spec.ts e2e/tests/task/sidebar-immediate-archive.spec.ts)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/task/mobile-sidebar-immediate-delete.spec.ts e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implemented on 2026-09-30. Four expected regression failures were observed
before production changes. Final checks: 32 focused tests across six suites,
TypeScript, targeted ESLint and formatting, two desktop and two phone browser
scenarios, public-doc validation (47 pages and 62 validator tests), catalog
validation (331 decisions and 1248 specifications), full specification lint,
and diff checks passed. Both pending screenshots were inspected against UI-01.
See the work order for exact commands and environment notes.

## Risks

- Archived-row guards can silently suppress delete loading.
- Token release or cache refresh can cause a normal-row flash.
- Renaming the spinner selector can break existing archive browser tests.

## Documentation

Updated the pending-action description in `docs/public/tasks-and-workflows.md`.
No new ADR: the existing state ownership and coordinator are retained.
