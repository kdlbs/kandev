---
id: "01-pending-removal-rows"
title: "Extend pending removal rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-005
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-005.1
  - AC-TASKS-REMOVAL-NAVIGATION-005.2
  - AC-TASKS-REMOVAL-NAVIGATION-005.3
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 01: Extend pending removal rows

## Summary

Use the coordinator's pending delete membership to give visible delete targets
the existing archive loading treatment on desktop and phone.

## In scope

Shared pending projection, component plumbing, focused regression tests, and
public pending-action documentation.

## Out of scope

Backend cleanup, API contracts, new toasts, navigation redesign, persisted state,
and changes to unrelated task action menus.

## Acceptance

- Deferred deletion tests prove immediate busy rows, including archived,
  unselected, bulk/cascade targets and unaffected non-targets.
- Recovery uses current data and operation ownership; archive behavior stays intact.
- Desktop and phone delayed-delete rendered tests pass and match UI-01.

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

Full preview: [plan](plan.md#ascii-ui-preview).

## Verification

Start with failing deletion regressions, then implement. Read `/tdd`, `/e2e`,
`/mobile-parity`, `/docs-maintainer`, and scoped web guidance. If dependencies are
absent, run `(cd apps && pnpm install --frozen-lockfile)` once first. Commands
below run from repository root; the first managed E2E command builds assets.
Retain current test filenames when extending them; if a filename changes, update
these commands and all referencing tests in the same work order.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.removal.test.tsx components/task/task-session-sidebar-item-pending-archive.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/task-item-archive-pending.test.tsx hooks/use-task-removal-coordinator.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --project=chromium e2e/tests/task/sidebar-immediate-delete.spec.ts e2e/tests/task/sidebar-immediate-archive.spec.ts)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/task/mobile-sidebar-immediate-delete.spec.ts e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on every changed TS/TSX file and record exact paths/results.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts` and its test
- `apps/web/components/task/task-session-sidebar.tsx`, `task-session-sidebar-item.ts`
- `apps/web/components/task/task-switcher-types.ts`, `task-switcher-row.tsx`
- `apps/web/components/task/task-item.tsx`, `task-state-icon.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`, `session-task-switcher-sheet-item.ts`
- Existing item/pending projection tests named in Verification
- New `apps/web/e2e/tests/task/sidebar-immediate-delete-helpers.ts`,
  `sidebar-immediate-delete.spec.ts`, `mobile-sidebar-immediate-delete.spec.ts`
- Existing archive E2E helpers if spinner selectors change
- `docs/public/tasks-and-workflows.md`

## Dependencies

None. Existing removal coordinator and archive projection are shipped.

## Risks

Preserve archived delete membership, operation lifetime, and current navigation;
never restore stale task snapshots or add independent pending state.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/tasks/requirements/removal-navigation.md), REQ-005.
- [Design](../../specs/tasks/system-design/removal-navigation.md), pending delete projection.
- Existing sidebar immediate archive E2E helper and projection tests.

## Results

Completed on 2026-09-30.

- RED: four expected assertion failures proved missing active/archived delete
  membership and both archived row projection guards before production edits.
- Unit command in Verification: 32 tests passed across six suites, including the
  real-store subscription and deferred release test. Coordinator tests cover
  mutation lifecycle; projection tests cover refresh and bulk membership.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck`
  from `apps/web`. The initial default 2 GB heap exhausted memory.
- All changed TS/TSX files passed targeted ESLint and Prettier. Final test-helper
  check: `pnpm exec eslint e2e/tests/task/sidebar-immediate-delete-helpers.ts &&
  pnpm exec prettier --check e2e/tests/task/sidebar-immediate-delete-helpers.ts`.
- Managed backend, Vite production assets and plugin fixture builds passed with
  `/usr/local/go/bin` added to PATH. Final browser runs reused those fresh assets:

```bash
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=chromium e2e/tests/task/sidebar-immediate-delete.spec.ts e2e/tests/task/sidebar-immediate-archive.spec.ts --retries=0)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/task/mobile-sidebar-immediate-delete.spec.ts e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts --retries=0)
```

Both runs passed two tests each. Browser scenarios cover cancellation, pending
presentation, failure recovery, success, and unchanged selection. The desktop
scenario uses the existing right-click context menu: trace evidence showed the
hover trigger was pointer-intercepted and its keyboard path selected the row.
The phone scenario uses the visible touch action. Neither unrelated menu path
was changed in production. Captures wait for confirmation dismissal and complete
row viewport containment. Desktop and phone captures match UI-01.

Public docs: `node --test scripts/validate-public-docs.test.mjs` passed 62 tests;
`node scripts/validate-public-docs.mjs` validated 47 pages. Specification catalog,
full specification lint and diff checks passed. The environment switch
interrupted initial processes; completed results above come from resumed checks.
The namespace sandbox could not start, so final commands ran outside it against
isolated E2E data. No user data or persistent tasks were created during implementation verification.


## PR review remediation

Greptile identified a successful-delete/page-refresh race. A real-store
regression reproduced stale successful targets after pending membership cleared.
The coordinator and deletion event handler now publish confirmed IDs to the
existing sidebar page cache. Mounted pages prune those rows and replace their
query, fencing pre-deletion responses without retaining tombstones. Tests cover
cascade partial failure, two mounted consumers, cache eviction, unsubscribe,
and delayed sidebar refreshes in both browser viewports. Archive behavior stays
unchanged. This completes the existing AC-005.2 recovery/refresh contract.

Remediation verification passed: 72 tests in the ten focused sidebar/cache/
coordinator/handler suites, plus 134 tests across all task-event handlers and
removal hook consumers (`pnpm exec vitest run lib/ws/handlers/tasks
hooks/use-task-removal`). Typecheck, targeted ESLint/Prettier, catalog and spec
lint passed. The same desktop and phone E2E commands above passed two scenarios
each with retries disabled; deletion now holds the post-delete query to assert
immediate pruning. Both viewport screenshots were refreshed and inspected.
