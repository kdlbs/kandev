---
id: "01-bulk-session-removal"
title: "Implement task-scoped bulk session removal"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BULK-SESSION-REMOVAL-001
  - REQ-TASKS-BULK-SESSION-REMOVAL-002
acceptance_criteria:
  - AC-TASKS-BULK-SESSION-REMOVAL-001.1
  - AC-TASKS-BULK-SESSION-REMOVAL-001.2
  - AC-TASKS-BULK-SESSION-REMOVAL-001.3
  - AC-TASKS-BULK-SESSION-REMOVAL-001.4
  - AC-TASKS-BULK-SESSION-REMOVAL-001.5
  - AC-TASKS-BULK-SESSION-REMOVAL-001.6
  - AC-TASKS-BULK-SESSION-REMOVAL-002.1
  - AC-TASKS-BULK-SESSION-REMOVAL-002.2
  - AC-TASKS-BULK-SESSION-REMOVAL-002.3
system_design:
  - ../../specs/tasks/system-design/bulk-session-removal.md
---

# Task 01: Implement Task-Scoped Bulk Session Removal

## Summary

Add safe, permanent Remove Others and Remove All flows for persisted task
sessions. The work delivers shared client orchestration, desktop and phone
surfaces, localization, documentation, and focused browser evidence as one
vertical result.

## In scope

- Write failing pure tests before target selection and ordered deletion code.
- Revalidate a confirmed snapshot before dispatching existing `session.delete`
  requests, clean up only successful targets, and surface partial results.
- Add desktop menu actions and a stable confirmation owner.
- Add phone picker actions and hosted, touch-sized confirmation.
- Localize all new copy, update user documentation, and add focused desktop
  and mobile Playwright coverage.

## Out of scope

- New server APIs, atomic rollback, or changes to backend deletion eligibility.
- Preview-tab, task, workspace, worktree, branch, and Quick Chat deletion.

## Acceptance

- The persisted target snapshot, eligibility gates, ordering, duplicate guard,
  and partial-failure result satisfy `AC-TASKS-BULK-SESSION-REMOVAL-001.2`
  through `.6`.
- Desktop satisfies the `UI-01` order and destructive confirmation in the
  [plan preview](plan.md#ascii-ui-preview) without changing Close Others.
- Phone satisfies `UI-02`, preserves the picker on cancellation, has physical
  touch targets of at least 44 CSS pixels, and has no horizontal overflow.

## ASCII UI preview

See [`UI-01` and `UI-02` in the plan](plan.md#ascii-ui-preview). This task
owns both rendered views and their error, disabled, and empty-session states.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/session/use-session-actions.test.ts components/task/session-bulk-removal.test.ts components/task/mobile/mobile-sessions-section.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/session/session-tab-management.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/session/mobile-bulk-session-removal.spec.ts)
```

## Files likely touched

- `apps/web/hooks/domains/session/use-session-actions.ts`
- `apps/web/components/task/session-bulk-removal.ts`
- `apps/web/components/task/session-bulk-removal.test.ts`
- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/session-tab-menu.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`
- `apps/web/e2e/tests/session/mobile-bulk-session-removal.spec.ts`
- `apps/web/src/locales/*/task.json`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None.

## Risks

- A selected tab can disappear while Remove All is in progress, so request
  ownership cannot live solely inside that tab.
- State changes between confirmation and submission require a new snapshot,
  not a stale count.
- Sequential success can be followed by a failed request; the UI must leave
  completed deletion final and make the remaining set clear.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-BULK-SESSION-REMOVAL-001` and
  `REQ-TASKS-BULK-SESSION-REMOVAL-002`.
- The paired [system design](../../specs/tasks/system-design/bulk-session-removal.md).
- Existing `useSessionActions.remove`, `SessionContextMenuItems`, and
  `MobileSessionsPicker` behavior.

## Results

Pending.
