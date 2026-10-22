---
created: 2026-10-22
status: done
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
legacy_specs: []
---
# Cross-step move rollback ownership

## Overview

Repair the shipped producer's unconditional stage rollback. Parent causal proof
at base `0424e7885814b0e4a7c8950e05f957fe6c6aa5a7` has two behavioral failures
(different and equal authoritative target) and a passing ordinary-failure control.
User authorizes this one package and same-session TDD execution.

## Scope

Own only client cross-step rollback, AC-TASKS-KANBAN-TASK-REORDERING-001.13
and .28. Reuse the existing task-overview journal; no new ordering protocol,
Query migration, API/schema/backend changes, reorder bands, lifecycle, WIP,
Jira/settings changes, orphan hook repair or producer unification.

## Technical approach

Capture the current task and original scope, apply only the target stage, then
open an existing bounded journal. A failure can revert only an unchanged,
still-present task in that scope. Any accepted newer stage projection supersedes the
write, including equal target/unchanged timestamps and away/back updates. Keep
other tasks, unrelated fields and positions intact. Always release the journal.

## ASCII UI preview

### UI-01: Cross-step move after a newer task event

Desktop entry: drag a card body onto another column. Phone entry: visible card
More options > Move to > destination, using the existing inset menu and focused
column navigator. Layout, scroll owners, safe areas and touch targets stay as
shipped (`MobileColumnTabs`, card menu and `SwipeableColumns`).

```text
Desktop: [Source] [Optimistic] [Authoritative: Remote title]
Phone:   [Workflow / Authoritative v]
         [Remote title] [More options]
         [Existing move error]
```

Before: rejecting the old request returns the card to Source.
After: the accepted authoritative projection remains, including when its stage
is Optimistic. Without a newer projection, failure returns the card to Source.
The structure is existing UI; spacing is illustrative. Covers AC .13 and .28.

## Tests

`use-cross-step-move.test.tsx` exercises real providers, action/API/fetchJson and
registered WS handlers. Deferred fetch owns ordering. Cover newer different/equal
stages, away/back, partial unrelated fields, other tasks, independent moves,
deletion/membership/workflow/workspace replacement, original error identity,
ordinary failure/success and absent/same-step no-ops.

## E2E tests

`cross-step-rollback.spec.ts` (chromium) and
`mobile-cross-step-rollback.spec.ts` (mobile-chrome) share isolated fixture logic.
Exercise real desktop pointer drag and phone card-menu taps. Hold only the old
HTTP response, publish an authoritative move through the real API/WS path, then
reject the old request and verify the rendered stage/title and persisted task.
Use unique disposable workspace/workflow/repository/offline origin and explicit
Worktree executor. Guard worker seed and owned source HEAD/full raw porcelain;
clean in finally and prove cleanup also runs after an injected callback failure.

## Work orders

- [x] [Task 01: Preserve current step](task-01-preserve-current-step.md)

## Verification results

Implemented and verified in Task 01: 20 causal tests, the full Kanban hook
package (42 files, 384 tests), desktop browser 3/3 and phone browser 2/2,
typecheck, focused lint, localization, public docs and specification gates.
The immutable-base replay fails only the two expected stage assertions while
its ordinary-failure control passes. Final focused GREEN passes all 20 cases.

Stop after the normal-hook clean commit and terminal owned handles at
`ready_for_parent_validation`. No push/PR without a separate same-session parent
delivery grant. Complete actual diff, hook receipt and detailed command logs
are in the external attempt26 checkpoint packet.

## Risks

Only accepted stage writes revoke stage ownership; partial unrelated fields
survive an owned rollback. Journal scope/overflow invalidation must suppress
rollback; no event ordering or durable backend data-loss claim is made.
