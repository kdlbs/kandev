---
created: 2026-09-13
status: implemented
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
legacy_specs: []
---

# Implementation Plan: Plan comment foreground preservation

## Overview

Keep unfinished comments intact when returning to the browser triggers plan
refresh. One sequential work order owns the panel correction and component,
loader, desktop, and phone evidence. Tasks owns the contract because these
comments are feedback on its current plan.

## Confirmed defect and evidence

Read-only source trace on 2026-09-13:

1. `use-plan-comments.ts` registers `useForegroundRefresh(wake, ...)`.
2. `use-foreground-refresh.ts` handles focus, visible visibilitychange,
   pageshow, and online. `PlanCommentLoader.wake()` calls `load(true)`.
3. `plan-comment-loading.ts:resolvePlan` sets task-plan loading to true even
   with a cached plan, retaining the cached plan during the request.
4. `task-plan-panel.tsx:TaskPlanPanel` returns only a spinner for any loading
   state, unmounting `PlanPanelContent` and the comment input. Its parent hook
   `usePlanSelection` retains the selection for the same task and plan.
5. After loading, both variants in `plan-selection-popover.tsx` remount with
   local text initialized from `editingComment` or an empty string. New text
   disappears; an unsaved edit reverts to saved text.

Reproduce: open a plan, select text, open Comment, type without Add, switch
browser focus away and back. The refresh spinner destroys the draft. Editing
an existing comment takes the same path. This sequence was established by source trace and subsequently reproduced by
the component tests and a Chromium regression before the correction.

## Scope

### In scope

- Preserve new-comment and edit-comment text through same-plan foreground,
  reconnect, and background reads, including failure.
- Keep initial loading and task/plan identity isolation correct.
- Preserve existing desktop Popover and phone Drawer behavior.

### Out of scope

Draft recovery across reload, explicit dismissal, or task navigation; backend,
API, storage, delivery, mutation concurrency, and layout changes.

## Technical approach

In `apps/web/components/task/task-plan-panel.tsx`, restrict the full-panel
loading placeholder to loading without a current plan. Keep the content
subtree mounted when the current task has a cached plan. Leave shared loader
network-state semantics intact. Use current task plan state, not a sticky
"ever loaded" flag that could show task A's content for task B.
Keep `usePlanSelection` resets for task change and confirmed deletion or
replacement. While loading, keep plan content read-only and hide its editing
controls; leave the comment input editable. Restore plan editing on settlement.
Do not add autosave for unfinished comments or disable refresh.

This follows completed [task-owned comments](../task-owned-plan-comments/plan.md)
and [recovery](../plan-comment-recovery/plan.md) packages. Their results remain
historical. Existing affected desktop/phone suites are rerun for this package;
no backend or migration work orders are reopened.

## ASCII UI preview

UI-01: Desktop, Plan > select text > Comment, after browser return.

```text
Before: [Plan] -> [Loading plan...] -> [Comment: empty]
After:
Plan remains visible
  +----------------------------------+
  | "Selected plan text"             |
  | Keep this unfinished feedback... |
  |                     [Add] [Run]  |
  +----------------------------------+
```

UI-02: Phone, Plan destination > Comment, after foreground return.

```text
| Plan                         |
|                              |
| +--------------------------+ |
| | Comment                  | |
| | "Selected plan text"     | |
| | Unfinished feedback...   | |
| |              [Add] [Run] | |
| +--------------------------+ |
|       safe-area clearance    |
```

Both views map to AC-001.9 through .11 below. Editing retains Update/Delete
and existing inline failure feedback. Continuous input identity, preserved
text/selection, and explicit actions are requirements; spacing is illustrative.

The shipped `PlanSelectionDrawer` is the phone exemplar: a focused bottom
drawer for a short comment, header, one internal scroll owner, dynamic height,
safe-area padding, and 44 px actions. Keep the desktop anchored Popover.
Shared panel/loader logic owns preservation. Do not forcibly reclaim focus
from another application; the user can resume typing once foregrounded.

## Tests

New `apps/web/components/task/task-plan-panel.refresh.test.tsx`:

- `preserves a new comment during plan refresh` and `preserves unsaved edits
  during plan refresh`, parameterized over desktop/phone and success/failure.
  Hold the read pending and assert the same textarea node, exact text, and
  selected plan text before and after settlement (AC-001.9).
- No create/update/Run during refresh; one explicit Add/Update receives the
  preserved body. Existing popover mutation-failure tests remain green (.10).
- Initial loading retains its placeholder. Task change and confirmed plan
  deletion/replacement clear outgoing selection without stale submission (.11).

Keep the actual panel and comment input; mock heavyweight editor/transport
boundaries. Add a real-hook/loader focus case to
`hooks/domains/comments/use-plan-comments.test.tsx` proving a request actually
occurs. Rerun loader and same-task session-switch coverage.
All abbreviated ACs in this package refer to `AC-TASKS-PLAN-COMMENTS-001`.

## E2E tests

Extend `apps/web/e2e/tests/session/task-plan-comments.spec.ts` (`chromium`)
and `mobile-task-plan-comments.spec.ts` (`mobile-chrome`) for new and edited
comments surviving pending, successful, and failed foreground reads (.9, .10).
Use correlated transport interception and causal waits to hold/release the
refresh response. Desktop uses a second page and `bringToFront` where
supported; deterministic foreground events supplement the loader evidence.
A synthetic event alone does not prove OS Alt-Tab behavior. Phone uses the
actual Drawer and proves preserved text, reachable actions, and no overflow.

## Work orders

- [x] [Task 01: Preserve the mounted comment editor](task-01-preserve-comment-editor.md)

## Verification results

Pre-implementation artifact checks passed on 2026-09-13: `python3 scripts/list-docs.py validate`
(267 decisions, 868 specifications), `python3 scripts/lint-spec-files.py --all`,
and `git diff --check`. The added design section was shortened to satisfy the
existing size limit. Work-order requirement IDs, design paths, and existing
verification inputs were checked. That design-only step changed no production
or permanent tests; implementation subsequently changed the panel and added
the refresh regression file.
Implementation complete: 48 targeted tests across five files, TypeScript,
zero-warning changed-file ESLint, and both full browser suites pass. Chromium
passes 8 scenarios (2.1 minutes); Pixel 5 passes 8 (1.3 minutes), both with
zero retries and `E2E_PORT_OFFSET=17`. The managed RED run built backend and
fixture artifacts; a fresh `build:e2e` after the frontend-only fix supplied
both final `--no-build` browser runs. Desktop and phone screenshots were
inspected. See the [work-order results](task-01-preserve-comment-editor.md#results)
for exact commands, initial test-setup corrections, and headless limitations.
Public docs are unchanged because this package records implementation intent.

## Risks

- Mocking the input hides its local-state loss. Assert actual DOM identity.
- An overbroad loading guard can expose stale content for another task.
- Headless focus differs from OS Alt-Tab; record the event mechanism tested
  and a manual return check when available.

### Review remediation

Plan content is temporarily read-only during background reads, while comment
input remains editable. Formatting, slash, and drag controls pause until the
read settles. A browser regression failed before this guard; all 16 desktop/
phone scenarios and 69 focused tests now pass. See the work order for exact
checks. PR CI and review completion are tracked separately.
