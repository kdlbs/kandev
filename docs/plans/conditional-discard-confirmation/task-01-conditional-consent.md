---
id: "01-conditional-consent"
title: "Condition discard consent on inspected changes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.10
  - AC-TASKS-RUNTIME-CLEANUP-001.14
  - AC-TASKS-RUNTIME-CLEANUP-001.15
  - AC-TASKS-RUNTIME-CLEANUP-001.16
  - AC-TASKS-RUNTIME-CLEANUP-001.17
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.11
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.12
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.14
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-deletion.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
---

# Task 01: Condition discard consent on inspected changes

## Summary

Expose the existing task worktree inspection to the shared delete confirmation.
Require consent only for inspected local changes, with safe pending/error states.

## In scope

- Read-only authorized preflight, exact cascade scope, and all repository inventory.
- Shared API client, hook, dialog, callers, localization, and regression fixtures.
- Desktop and mobile clean/dirty/retry flows and clean archive regression
  coverage.

## Out of scope

Changes to archive cleanup policy, task ownership, unique-commit preservation,
mutation-time safeguards, or dialog-presentation primitives.

## Acceptance

1. Clean or empty inventories permit deletion with false consent; any dirty
   target requires an explicit unchecked selection. Missing inspection capability
   or a failed inventory read never produces a clean result.
2. Scope changes invalidate results and consent; stale responses cannot enable
   Delete. Pending/error states have no checkbox and disabled Delete, with retry
   for errors. Reopening refreshes state; typed dirty conflicts preserve tasks.
3. Existing desktop/mobile actions remain usable, clean archive needs no discard
   checkbox, and dirty consent plus all existing cleanup audits remain effective.

## ASCII UI preview

UI-01 and UI-02 from the [full preview](plan.md#ascii-ui-preview), cleanup AC
.14-.17 and confirmation AC .11-.14:

```text
Clean desktop: consequences                 [Cancel] [Delete]
Clean phone:  consequences
              [Cancel, full width]
              [Delete, full width]
Dirty body:   [ ] Permanently discard ...   Delete requires selection
Checking:     status, no checkbox           Delete disabled
Error:        explanation + Retry           Delete disabled
```

Keep the centered phone alert, single scrolling body, persistent footer,
viewport insets, and 44px touch actions. Use the existing shared confirmation
classes and the mobile task-switcher as the entry-point exemplar.

## Regression matrix

- Backend `TestTaskDeletePreflight`: clean, staged, unstaged, untracked,
  committed-only, no worktrees, retained worktrees without sessions, missing
  paths, mixed clean/dirty repositories, and inspection dependency/error paths.
- Backend scope tests: unauthorized roots/children, deduplicated bulk roots,
  dirty descendant excluded without cascade and included with cascade, including
  archived descendants. Assert no task/job/event/runtime mutation in every case.
- `task-delete-confirm-dialog.test.tsx`: first add `hides discard consent for a
  clean worktree and confirms without consent` and observe the expected failure.
  Add dirty, loading, retry, bulk, cascade, close/reopen, task-switch, and stale
  response cases. A previous checked selection must not survive a scope change.
- API/hook tests: request body, error envelope, response cancellation/ordering,
  request identity, and false consent for a clean result.
- Existing admission tests retain the dirty-after-preflight conflict behavior.
- Desktop card E2E completes clean delete and archive. Mobile E2E completes clean
  delete with `.tap()`, verifies dirty consent, unavailable-preflight retry,
  viewport containment, and touch action bounds. Seed real clean/dirty task
  worktrees in disposable fixtures.

## Verification

Run from repository root. Install dependencies once if absent. Run targeted RED
before implementation, then these final checks:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/worktree)
(cd apps/web && pnpm exec vitest run components/task/task-delete-confirm-dialog.test.tsx hooks/use-task-delete-preflight.test.ts lib/api/domains/kanban-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/card-menu-delete-archive.spec.ts tests/task/sidebar-delete-confirm.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-delete-discard-consent.spec.ts tests/kanban/mobile-card-archive-confirmation.spec.ts)
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before changing shared fixtures, inventory exact consumers with
`rg -l 'delete-discard-worktree-checkbox|requireDiscardConsent' apps/web`.
Record additional affected test files and their exact targeted commands here,
then run all changed suites before marking done. Do not make conditional clicks
that allow clean-state regressions to pass. Run focused lint for changed TS files.

Additional affected consumer commands run:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/cascade-subtasks-toggle.spec.ts tests/kanban/task-multi-select.spec.ts tests/kanban/pipeline-view.spec.ts tests/kanban/dialog-enter-confirms.spec.ts tests/kanban/task-actions-menu-preview.spec.ts tests/task/sidebar-multi-select.spec.ts tests/task/confirmation-text-hierarchy.spec.ts tests/task/delete-task-redirect.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-confirmation-text-hierarchy.spec.ts)
```

The first combined Chromium run passed 45 tests and exposed one clean-fixture
assumption. The corrected hierarchy test passed both tests. The first mobile
consent run passed three tests and exposed an assertion during the entrance
animation. After waiting for finite animations, the required mobile run passed
all four tests. The PR fixup mobile run passed three tests, including a real
clean worktree and unavailable-preflight Retry flow; its first attempt exposed
the missing 44px Retry target and the shared action class fixed it.

## Files likely touched

- `apps/backend/internal/task/service/task_delete_preflight.go` and `_test.go` (new)
- `apps/backend/internal/task/handlers/task_delete_preflight.go` and `_test.go` (new)
- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/service/service_tasks.go`, `handoff_cascade.go`
- `apps/web/lib/api/domains/kanban-api.ts`, `kanban-api.test.ts`, public API export
- `apps/web/hooks/use-task-delete-preflight.ts` and `.test.ts` (new)
- `apps/web/components/task/task-delete-confirm-dialog.tsx` and `.test.tsx`
- Existing `requireDiscardConsent` callers and affected checkbox test fixtures
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- Desktop and mobile E2E files named above

## Dependencies

None. Read the applicable backend/web AGENTS.md and TDD/E2E skills before execution.

## Risks

Preflight is advisory; preserve deletion's authoritative recheck. Do not reuse
cached session summaries or deletion preparation as a read-only inspection.

## Parallelism

`sequential`

## Inputs

- [Task design](../../specs/tasks/system-design/dirty-worktree-deletion.md)
- [Task requirements](../../specs/tasks/requirements/runtime-cleanup.md)
- [UI design](../../specs/ui/system-design/confirmation-warning-hierarchy.md)
- `InspectDirtyWorktrees`, `ValidateTaskDeleteWorktrees`, and existing cascade tests
- `card-menu-delete-archive.spec.ts` and `sidebar-delete-confirm.spec.ts`

## Results

Implemented the read-only authorized preflight endpoint, API client, hook, and
shared dialog integration. Delete consent is conditional on inspected dirty
worktrees. Preflight is fail-closed and does not prepare cleanup or mutate task,
job, event, runtime, or worktree state. Mutation-time cleanup safeguards remain
authoritative.

Added backend service and handler regression coverage for exact direct/cascade
scope, archived descendants, duplicate roots, unauthorized targets, empty and
error inventories, unavailable inspection, and read-only behavior. Added web
API, hook, dialog, caller, localization, desktop, and mobile coverage. Archive
behavior remains unchanged.

Final verification:

- `go test ./internal/task/service ./internal/task/handlers ./internal/worktree`:
  2,827 passed.
- Focused Vitest command in the verification block: 52 passed.
- PR fixup Vitest command including the action-message delete test: 35 passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm --filter @kandev/web run build:vite`: passed.
- Focused ESLint and Prettier checks: passed with no errors.
- Required and affected Chromium/mobile E2E commands: passed after the
  fixture/assertion corrections recorded above. The final affected Chromium
  run passed 45 tests, and the PR fixup mobile run passed 3 tests.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`: passed.
