---
created: 2026-09-10
status: done
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-deletion.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
legacy_specs: []
---

# Implementation Plan: Conditional discard confirmation

## Overview

Hide discard consent for clean task workspaces. Deliver one vertical work order
that exposes existing backend inspection and connects the shared delete dialog.
The task system owns the condition because it owns cleanup inventory and consent.

## Evidence and root cause

`TaskDeleteConfirmDialog` calls `hasPotentialWorktree` and
`shouldRequireDiscardConsent`. These require consent for a worktree executor,
unknown executor metadata, or any subtasks, without reading Git status or even
checking whether cascade is selected. Rendering an open dialog with
`executorType="worktree"` reproduces the requirement regardless of file state.
Its existing test, `requires explicit discard consent for worktree cleanup`,
encodes that heuristic. This is a requested refinement of the previous UI
criteria, not a failure of backend dirty-worktree admission.

Archive already has no discard option. Its checkbox selects descendants.
`InspectDirtyWorktrees` already reads tracked and untracked Git status from
task-owned inventory. Preserve its audits and the mutation-time conflict guard.

## Scope

In scope: direct, bulk, cascade, retained/no-session, clean and dirty worktrees;
pending/error states; fresh inspection on reopen; desktop and phone regressions.

Out of scope: archive discard semantics, unique-commit cleanup policy, automatic
commit/push, new persistence, executor lifecycle changes, and unrelated UI layout.

## Technical approach

Add the inspection endpoint and shared hook described in the task design.
Register it through `task/handlers/task_handlers.go`; keep implementation in
focused `task_delete_preflight.go` files in handlers and service. Reuse inventory
inspection and cascade target resolution from `service_tasks.go` and
`handoff_cascade.go`, without invoking cleanup preparation.

Add `getTaskDeletePreflight` to `lib/api/domains/kanban-api.ts` and its public
export. Add `hooks/use-task-delete-preflight.ts`. Replace the shared dialog's
executor heuristic with that hook. Inspect every `requireDiscardConsent` caller
and every test that unconditionally clicks `delete-discard-worktree-checkbox`.
Update fixtures to explicitly represent clean or dirty state; do not weaken
assertions with optional checkbox clicks.

Localize any new loading, failure, or retry copy in all supported catalogs.
Keep archive callbacks and the separate cascade choice unchanged.

## ASCII UI preview

UI-01: Delete from card/sidebar/preview, clean workspace.

```text
Before                           After (desktop)
Delete task                      Delete task
Task and cleanup consequences    Task and cleanup consequences
[ ] Permanently discard ...
[Cancel] [Delete disabled]        [Cancel] [Delete]

After (phone, centered inset alert)
Delete task
Task and cleanup consequences
[          Cancel          ]
[          Delete          ]
```

UI-02: Shared desktop/phone body states.

```text
Checking: Checking workspace changes...  Delete disabled
Failed:   Unable to check changes. [Retry] Delete disabled
Dirty:    [ ] Permanently discard ...    Delete disabled until selected
```

Copy is illustrative and must be localized. Absence of the clean-state discard
row is required. Title and footer remain fixed; the existing body owns overflow.
Phone keeps centered viewport insets and full-width actions at least 44px high.
Archive retains its existing surface and independent descendant checkbox.
UI-01/02 map to cleanup AC .14-.17 and confirmation AC .11-.14.

## Tests

The work order owns exact commands and the complete regression matrix. Backend
tests must prove inspection is read-only and authorized. Component tests must
first fail on a clean worktree that currently requires consent, then cover scope
changes, unavailable state, stale responses, and reset consent.

## E2E tests

Extend `e2e/tests/kanban/card-menu-delete-archive.spec.ts` (chromium) to complete
clean deletion without consent and preserve clean archive behavior. Add
`e2e/tests/task/mobile-delete-discard-consent.spec.ts` (mobile-chrome) to complete
clean deletion and verify dirty consent with touch input. Use isolated fixtures.
Inventory and run other affected checkbox consumers as described in the work order.

## Work orders

- [x] [Task 01: Condition discard consent on inspected changes](task-01-conditional-consent.md)

## Verification results

Implementation is complete. Delete preflight now inspects the authorized task
scope and its worktree inventory before rendering consent. Clean tasks hide the
discard row and enable Delete after inspection. Dirty tasks require explicit
consent. Missing inspection capability and inspection errors fail closed.

Verification passed:

- Backend: `go test ./internal/task/service ./internal/task/handlers ./internal/worktree`
  reported 2,826 passing tests.
- Web: 52 focused Vitest tests, typecheck, production build, i18n checks, and
  focused lint passed. Lint reported three existing-style warnings and no errors.
- Browser: the required desktop delete/archive set passed 6 tests; the final
  affected Chromium set passed 45 tests plus the corrected hierarchy pair; the
  required mobile set passed 4 tests; the existing mobile hierarchy set passed
  4 tests.
- Documents: `python3 scripts/lint-spec-files.py --all` and `git diff --check`
  passed.

The work order records the transient browser failures and their fixes: the
mobile assertion now waits for the dialog entrance animation, and the clean
desktop hierarchy fixture no longer clicks a heuristic checkbox.

## Risks

- A file may change after inspection. Mutation-time admission remains required.
- Optional backend inspection dependencies must not turn unknown state into clean.
- Bulk or cascade results must not omit a dirty repository or archived child.
- Existing UI tests assume every task needs consent and require explicit fixtures.
