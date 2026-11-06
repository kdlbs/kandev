---
id: "01-coordinate-rapid-archives"
title: "Coordinate rapid sidebar archives"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
  - REQ-TASKS-REMOVAL-NAVIGATION-003
  - REQ-TASKS-REMOVAL-NAVIGATION-004
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-001.6
  - AC-TASKS-REMOVAL-NAVIGATION-002.2
  - AC-TASKS-REMOVAL-NAVIGATION-002.3
  - AC-TASKS-REMOVAL-NAVIGATION-002.6
  - AC-TASKS-REMOVAL-NAVIGATION-003.1
  - AC-TASKS-REMOVAL-NAVIGATION-003.2
  - AC-TASKS-REMOVAL-NAVIGATION-003.3
  - AC-TASKS-REMOVAL-NAVIGATION-004.2
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 01: Coordinate rapid sidebar archives

## Summary

Keep rapid individual archive actions inside the existing removal architecture.
Use shared pending membership for replacement eligibility and desktop/phone busy controls.
Retain the existing row presentation and backend lifecycle.

## In scope

- Add small pending-membership selectors beside the existing pure removal transitions.
- Reject newly pending candidates after HTTP and session awaits, before destination writes.
- Derive per-target archive controls from existing removal state in desktop and phone adapters.
- Preserve invocation compatibility, progress counters, confirmation, and later user navigation.
- Add the delayed-candidate regression and overlapping-action browser scenarios from the plan.
- Record the exact frontend flow before and after the correction with temporary tracing.

## Out of scope

Offline storage, a new store, a generic command or request queue, backend changes,
new copy or animations, a broad task-overview refactor, and PR publication.

## Acceptance

1. Neither a delayed liveness reply nor a delayed session reply can select a task under another pending removal.
   Preserve existing candidate order, authoritative checks, and user-navigation ownership.
2. With several independent archives pending, each target retains its own busy controls and duplicate protection until its operation releases.
   Settling A does not clear B. Unrelated task actions and navigation remain usable on desktop and phone.
3. All targeted regressions and existing compatibility scenarios pass.
   Record the frontend trace without claiming an unmeasured backend CPU improvement.

## ASCII UI preview

UI-01 from the [full preview](plan.md#ascii-ui-preview), for AC-001.6, AC-002.6, and AC-003.1:

```text
Desktop sidebar              Phone Tasks drawer (reopened)
[~ A          ]              [Tasks                 Close]
[~ B          ]              [~ A                       ]
[o C       ...]              [~ B                       ]
                             [o C                    ...]
```

Keep the current spinner, dimming, row positions, drawer dismissal, and scroll owner.
The main content selects an eligible task or the overview, never pending B.
Short IDs use the `AC-TASKS-REMOVAL-NAVIGATION` prefix.

## Verification

Run the permanent delayed-B regression first and record its expected RED failure.
The temporary diagnostic already failed at the intended selected-B assertion.
Use the name `does not select another independently pending archive after a delayed candidate read`.
Use the existing fixtures and causal waits for browser checks.

```bash
(cd apps/web && pnpm exec vitest run lib/state/task-removal.test.ts hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts hooks/use-task-removal-sidebar-fallback.test.ts hooks/use-task-menu-actions.test.ts hooks/use-task-actions.test.ts components/task/task-session-sidebar-archive-actions.test.tsx components/task/mobile/session-task-switcher-sheet-archive-actions.test.ts components/task/task-item-archive-pending.test.tsx components/task/task-switcher-row-render-identity.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-management-surface.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/state/task-removal.ts hooks/use-task-removal.ts hooks/task-removal-sidebar-fallback.ts components/task/task-session-sidebar.tsx components/task/task-switcher-row.tsx components/task/mobile/session-task-switcher-sheet-hooks.ts)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/task/sidebar-immediate-archive.spec.ts e2e/tests/task/archive-task-redirect.spec.ts e2e/tests/task/sidebar-immediate-delete.spec.ts e2e/tests/task/threads-task-actions.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts e2e/tests/task/mobile-archive-task-redirect.spec.ts e2e/tests/task/mobile-sidebar-immediate-delete.spec.ts e2e/tests/task/mobile-threads-task-actions.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

`task-session-sidebar-archive-actions.test.tsx` is a planned new test file.
Other named suites already exist. Extend lint paths to include every changed source and test file.
Run desktop and phone commands sequentially. The first managed command rebuilds the runtime and web assets.
If source changes before the phone run, remove `--no-build`.
Run `pnpm run i18n:check` from `apps/web` only if copy changes become necessary.
If public docs change, run both public-doc validators from the repository root.
Record the trace under the browser test output directory; remove temporary instrumentation afterward.

## Files likely touched

- `apps/web/lib/state/task-removal.ts` and its tests.
- `apps/web/hooks/use-task-removal.ts`, `task-removal-sidebar-fallback.ts`, and their tests.
- `apps/web/hooks/task-removal-candidates.ts` and `use-task-removal-completed-removals.test.ts`.
- `apps/web/components/task/task-session-sidebar.tsx`, its archive-action test, and shared row adapters.
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts` and its archive-action tests.
- `apps/web/components/task/task-switcher-row.tsx` and its rendered tests.
- `apps/web/e2e/tests/task/sidebar-immediate-archive-helpers.ts` and both entry specs.
- `docs/public/tasks-and-workflows.md`, only if the existing concurrent-action guidance needs correction.

## Dependencies

None. Read the current owning requirements and design before implementation.

## Risks

Keep the `useTaskMenuActions.pendingTaskId` contract for its task-management caller.
Do not reinterpret an unknown HTTP result as a rejected archive or restore stale task data.
Shared coordinator changes must preserve delete, bulk, cascade, and archived-view behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/removal-navigation.md).
- [Design](../../specs/tasks/system-design/removal-navigation.md), Rapid independent sidebar archives.
- [Plan evidence and browser matrix](plan.md).
- Existing deferred coordinator tests and `sidebar-immediate-archive-helpers.ts`.

## Results

Done on 2026-10-29.

- Added `pendingTaskRemovalAction`, `isTaskRemovalPending`, and
  `pendingTaskRemovalTaskIds` beside the removal transitions. Membership resolves
  through the token map and its operation, so a stale token or another captured
  workspace is not pending.
- `switchAfterRemoval` now combines the operation's exclusions with current
  pending removals, rechecks the candidate after liveness and after session
  loading, and continues through the existing candidate order and bounded
  server-page fallback when a candidate enters another removal. Lost navigation
  ownership stops without another automatic selection.
- Candidate ordering and selection moved to `hooks/task-removal-candidates.ts`
  so `use-task-removal.ts` stays within the file limit. `selectNextTaskAfterRemoval`
  is re-exported through the new module; callers are unchanged.
- The desktop sidebar and phone sheet derive archive busy state from
  `usePendingTaskRemovalIds`. The dialog's busy state follows its captured
  target, and row menus read the row's own `isPendingRemoval`. The competing
  archive booleans were removed from `TaskSwitcherProps`, `TaskRowProps`, and the
  mobile list props; `useTaskMenuActions.pendingTaskId` is unchanged for
  task-management.
- Each selection attempt observes accepted removals for its whole lifetime. A
  candidate seen under an independent archive or delete stays excluded for that
  attempt even after its token is released, so a delayed liveness or session
  reply cannot resurrect an already removed task. The observation reuses the
  store's removal transitions and is disposed when the attempt settles.
- Cascade removals record their captured task ids on the accepted operation. The
  selection attempt keeps that coverage for its lifetime, validates candidate
  ancestry against it during selection and again immediately before the switch,
  and therefore rejects a descendant that appears only in the server-page
  fallback even when the descendant has no token of its own.
- Added `hooks/task-removal-candidates.test.ts`,
  `hooks/use-task-removal-completed-removals.test.ts`,
  `hooks/use-task-removal-cascade-ancestry.test.ts`,
  `components/task/task-session-sidebar-archive-actions.test.tsx`, and a row-menu
  archive boundary test. The completed-removal suite awaits an independent
  archive, deletion, and delayed session reply to completion before delivering
  the stale candidate reply. Extended the store, coordinator, fallback, and
  mobile archive-action suites.
- Extended the desktop and phone archive specs with five held archive requests
  and the mixed-result release, plus the delayed/pending candidate scenario. The
  phone completion check now awaits the forwarded archive response and the
  confirmed backend archive state, reopens the phone picker, and asserts the
  viewport-aware row surface while an eligible row is visible.
- Added a brief concurrent-archive note to the public archive guidance.

Verification:

- RED before the first fix: `(cd apps/web && pnpm exec vitest run
  hooks/use-task-removal-coordinator.test.ts -t "independently pending archive")`
  failed both permanent regressions with `expected 'task-b' not to be 'task-b'`.
  They pass after the correction.
- RED before the attempt-membership fix: `(cd apps/web && pnpm exec vitest run
  hooks/use-task-removal-completed-removals.test.ts)` failed all three cases with
  `expected 'task-b' to be 'task-c'`. They pass after the attempt retains every
  observed removal, including a completed archive, deletion, and delayed session
  reply.
- RED before the cascade-ancestry fix: `(cd apps/web && pnpm exec vitest run
  hooks/use-task-removal-cascade-ancestry.test.ts)` failed all four cases with
  `expected 'task-child' to be 'task-cached-candidate'`. They pass after cascade
  coverage is retained and checked during selection and before the switch.
- `(cd apps/web && pnpm exec vitest run lib/state/task-removal.test.ts
  hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts
  hooks/use-task-removal-completed-removals.test.ts hooks/task-removal-candidates.test.ts
  hooks/use-task-removal-sidebar-fallback.test.ts hooks/use-task-menu-actions.test.ts
  hooks/use-task-actions.test.ts components/task/task-session-sidebar-archive-actions.test.tsx
  components/task/mobile/session-task-switcher-sheet-archive-actions.test.ts
  components/task/task-item-archive-pending.test.tsx
  components/task/task-switcher-row-render-identity.test.tsx
  components/task/task-switcher-context-menu.test.tsx
  components/task/task-management-surface.test.tsx)`:
  15 files and 125 tests passed.
- `(cd apps/web && pnpm exec tsc --noEmit)`: clean.
- `(cd apps/web && pnpm exec eslint <changed files>)`: clean, no warnings.
- Desktop archive/redirect/delete/Threads specs: 14 tests passed.
- Phone archive/redirect/delete/Threads specs: 12 tests passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `git diff --check`: passed.
- One Chromium performance trace per state was recorded. The corrected run
  measured lower totals, but the runs were not load-controlled, so no CPU
  improvement is claimed. See [plan results](plan.md#verification-results).

