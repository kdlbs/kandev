---
created: 2026-10-22
status: completed
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
its ordinary-failure control passes. Initial focused GREEN passed all 20 cases; post-review evidence is below.

Stop after the normal-hook clean commit and terminal owned handles at
`ready_for_parent_validation`. No push/PR without a separate same-session parent
delivery grant. Complete actual diff, hook receipt and detailed command logs
are in the external attempt26 checkpoint packet.

## Risks

Only accepted stage writes revoke stage ownership; partial unrelated fields
survive an owned rollback. Journal scope/overflow invalidation must suppress
rollback; no event ordering or durable backend data-loss claim is made.

## Review remediation

PR #4692 first head passed terminal CI (64 passed, 25 skipped, one neutral) but
has valid bounded review findings. Keep the same owning requirement/design and
work order. Distinguish rejected stale events from accepted stage assertions,
including independent fresh status summaries, and explicitly journal replacement
snapshot rows at the existing write boundary even when their stage is equal.
Preserve unchanged row references, partial WS omission, scope and bounded overflow
semantics. Add real-store/registered-handler RED cases before production edits.
Also strengthen the causal browser wait and explicitly retire the owned executor
profile; retain required worker/source HEAD and full raw-status guards. Tighten
public wording to cross-step moves. No global revisions, new framework or package.
New head needs normal hooks, affected shared-state suites, browser proofs, fresh
assets if affected, current-base combined validation and actual new-head reviews.

## Reviewed delivery update

Parent independently reviewed clean `1f5cde91` and reran its 25 focused tests,
then issued the separate same-session delivery grant. PR 4692 is open. Its
initial CI completed with no failed checks; eight native tests recovered through
retries (artifacts inspected), so this is not zero-flake evidence.

Review remediation keeps the same package and journal boundary: rejected stale
events do not assert cached stages, accepted replacement snapshot rows assert
equal stages, independently fresh status summaries survive stale task data,
and identity-retained sibling rows do not claim ownership. The browser now
waits for the actual authoritative move event and explicitly deletes its owned
executor profile. Public guidance is limited to cross-step failures.
Detailed RED/GREEN and final local receipts are in Task 01. Delivery still
requires fresh exact-head checks/reviews and current-base combined validation.


## Pending-request rollback chain correction

Parent real-provider proof found that two admitted same-task moves, both rejected,
leave an optimistic intermediate stage. The newer-first schedule passed with the
original producer and regresses with the journal ownership guard. Keep both native
moves admitted. Link only pending producer writes that still own the displayed
stage before the next write. On rejection, skip failed linked writes, preserve
successful or accepted external stages, and re-arm a still-valid predecessor
journal after removing the rejected optimism. Never re-arm missing/overflowed or
out-of-scope evidence. This is bounded request-local ownership, with all records,
journals and subscriptions released at settlement; no global revision framework.
Add both rejection orders, later accepted/successful-stage controls and lifetime
coverage before the production correction. Original a8cc delivery evidence is
historical after the next normal-hook commit. No CI waiter currently active.


Real HTTP producer RED also confirms equal-stage response identity reuse hides
admission. Extend existing read completion with optional original admitted rows;
record stage-only claims after existing freshness/in-flight guards, despite
canonical reuse, for the board snapshot readers and successful sidebar reads.
Cached retention, cancellation/finally closure and mobile context-switch rejection
remain unchanged. Test pre-optimism shadowed reads and stale responses as controls.


Pending-chain local implementation is complete: both rejection orders pass via
actual desktop drags and phone card-menu taps (5 desktop and 4 phone cases,
retries zero). Shared unit and causal HTTP/sidebar controls pass; see Task 01.
Final exact-head delivery evidence remains external until fresh remote reviews,
checks and current-base combined validation complete under the retained grant.
