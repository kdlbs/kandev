---
created: 2026-09-15
status: implemented
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-003
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
legacy_specs: []
---

# Implementation plan: Immediate sidebar archive

## Overview

Hide accepted archive targets through the existing shared sidebar projection.
Tasks owns this extension because pending archive intent and recovery are task
lifecycle state. One sequential work order delivers the complete change.

## Scope

Include selected/unselected archive, bulk and known cascade targets, failure,
refresh races, and desktop/phone parity. Exclude delete semantics, cleanup/API
changes, new animations, and navigation redesign.

## Technical approach

The existing coordinator publishes pending state immediately but retains caches
via `switchOnly`; the sidebar reads those caches without observing pending state.
Extend `useWorkspaceSidebarTasks` with archive-only visibility filtering backed
by `taskRemoval`, before downstream tree/group projections. Preserve canonical
rows for recovery and authoritative archived-inclusive saved-view behavior.
See the linked design's Immediate sidebar archive projection section.

## ASCII UI preview

UI-01: Active task navigation, archive A accepted (AC-003.1 through AC-003.3).

```text
Desktop sidebar              Phone task picker (reopened)
Before      Pending/Success  Before      Pending/Success
[A ...]     [B ...]          [A ...]     [B ...]
[B ...]                     [B ...]

Failure, A still active: [A ...] [B ...]
Last row archived: existing empty task list
```

Row removal and gap collapse are structural requirements; spacing is illustrative.
Phone keeps its existing sheet, fixed header, safe-area padding, and single
scrolling list. Its visible menu is the archive entry point; acceptance retains
existing dismissal. No new mobile surface is needed for this frequent navigation
action. Existing `session-task-switcher-sheet.tsx` is the shipped exemplar.

## Tests

Add `use-workspace-sidebar-tasks.archive.test.ts` with a real removal-state transition
and deferred mutation: assert absence before settlement, retention across cache
refresh, restoration after rejection, success after release, unselected archive,
non-cascade surviving children, cascade/bulk membership, and archived views.
Use `use-task-removal-coordinator.test.ts` to cover operation release and mixed
bulk success/failure where needed. These map to AC-003.1 through AC-003.3.

## E2E tests

Add `task/sidebar-immediate-archive.spec.ts` (chromium) and
`task/mobile-sidebar-immediate-archive.spec.ts` (mobile-chrome). Hold the archive
request before server processing, accept archive, and assert row absence before
release; withholding only the response is insufficient because WS may win.
Reopen the phone picker during the hold. Include cancellation, rejection and
successful completion cases. The real-store last-row test proves the empty projection; existing archive redirect specs cover the final empty destination. Use isolated fixtures and explicit deferred request
release, never fixed sleeps. All three new acceptance criteria need coverage.

## Work orders

- [x] [Task 01: Hide pending archive rows](task-01-hide-pending-archive-rows.md)

## Verification results

Design validation passed on 2026-09-15: catalog validation (272 decisions,
934 specifications), full specification lint, specification-linter tests
(36 passed), and `git diff --check`. Implementation complete: 44 targeted tests, typecheck, targeted ESLint,
formatting, and both desktop/phone browser tests passed. Production backend and
web builds passed. Both pending-state screenshots were inspected against UI-01.
The browser runs required local socket permission outside the sandbox and reused
fresh build assets with `--no-build`. See the work order for commands and results.

## Risks

- Filtering only the selected task misses unselected/bulk actions.
- Pruning caches optimistically risks overwriting newer events on rollback.
- Filtering archived rows indiscriminately breaks saved archive views.
- A test holding only HTTP responses can pass from early WS removal.

## Related package and docs

The completed [removal navigation package](../task-removal-navigation/plan.md)
provides the coordinator and remains historical evidence; this adds a new scope.
The archive section in `docs/public/tasks-and-workflows.md` explains immediate pending removal and recovery. No new ADR is needed:
this reuses the existing local-operation/state ownership boundary.
