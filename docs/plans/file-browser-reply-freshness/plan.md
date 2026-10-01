---
created: 2026-10-01
status: done
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
legacy_specs: []
---

# Implementation Plan: File Browser Reply Freshness

## Overview

Fence obsolete Files search and same-path file-watch replies in one focused
PR. One sequential work order adds permanent deferred regressions, implements
local publication ownership, verifies the affected hooks, and records results.
Publication and normal merge are authorized by the parent; implementation starts
after the repository-required design handoff to the parent coordinator.

## Evidence and root cause

Refreshed `origin/main` and remote main both equal
`08e4ffdb99caf40b0df5baa67b29cf4313188f15`. The clean task branch was already
based on that revision. A temporary test ran the real `useFileBrowserSearch`
and `applyFileChanges` functions with deferred promises on October 1:

- An `old` reply resolved after `current` and replaced `current.ts` with `old.ts`.
- A pending reply repopulated a cleared query with `old.ts`.
- A reply from session A published `A-only.ts` after switching to B.
- A create snapshot resolved after the newer empty deletion snapshot and
  resurrected `ghost.ts`.

All four intended-behavior assertions failed; the existing apply-changes and
desktop/touch search-context suites passed all 13 tests. The temporary test was
removed. Search has no completion ownership guard. File-watch refreshes check
owner lifetime but accept competing replies within that owner.

## Ownership and assumption check

This is an implementation regression under
`AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5`, preserving `.3`, `.4`, and `.6`.
The active requirement is reused without new requirement identities. A focused
design supplement defines search intent and per-folder publication ordering.
The approved scope settles the material choices: latest intent wins for the
same resource; independent siblings and loaded descendants remain available.
No unresolved product, security, API, or persistence decision blocks execution.

Related-package inventory: [task navigation responsiveness](../task-navigation-responsiveness/plan.md)
and its Files restoration work orders own retained trees and progressive loads.
They retain their existing results and scope. This package owns only search and
file-watch completion races; it does not reopen route, loader, or virtualizer
work. Other concurrent fix scopes and their artifacts are independently owned.

## Scope

### In scope

- Query/session/context generations; invalidate success/error/finally on clear,
  close, unmount, and owner retirement.
- Per-folder file-watch ordering within the active owner and filtering of
  partially superseded batches.
- Functional merges against the latest tree that retain loaded subtrees and
  independent sibling results.
- Permanent hook/apply-changes regression tests and targeted verification.

### Out of scope

- Tree loader/restoration scheduling redesign, global caching, dependencies,
  schema/API changes, localization copy, and responsive layout changes.
- Live instances/data, broad verification suites, and additional workers.
- The separate workflow, notification, board matching, and glob-parser fixes.

## Technical approach

Keep existing `useFileBrowserSearch` and `applyFileChanges` callers. Supply
search with the existing binding/reset context in `file-browser-data.ts`.
Add owner/intent guards in the shared hook, with optional focused extraction
only when file/function limits require it. Give each file-watch subscription
its own per-path tokens and check them before publication and in the reducer.
Retain unaffected paths from mixed batches and current loaded descendants.

## Tests

The work order maps `.5` to search and tree freshness tests; `.3`/`.4` to
sibling, placeholder, and loaded-subtree tests; `.6` to the shared logic and
existing desktop/touch component suite. Tests exercise production hooks and
subscription wiring with deferred transport promises, not a copied guard.

## E2E tests and mobile parity

`apps/web/e2e/tests/task/file-tree-search.spec.ts` already proves that clearing
restores the tree and Escape closes search. It remains the existing E2E contract.
The race regressions are deterministic hook tests. As explicitly approved for
this state-only repair, use `/mobile-parity`'s state-normalization exception:
shared hook and desktop/touch component tests suffice. No browser rebuild or
new mobile E2E is required because no layout, touch, scroll, navigation, or
viewport interaction changes. There is no rendered UI change needing an ASCII
composition preview.

## Documentation impact

Internal design and plan records document reply ownership. Public docs already
describe Files behavior during refresh in `docs/public/tasks-and-workflows.md`;
the correction restores that behavior. No public command, setting, label,
workflow, or screenshot changes, so no public docs update is needed.

## Work orders

- [x] [Task 01: Fence obsolete Files replies](task-01-fence-replies.md)

## Verification results

Task 01 implementation and local validation are complete. The six targeted
suites pass all 65 tests, including 29 new race/ownership/retention cases.
Typecheck, changed-file ESLint (zero warnings), Prettier, staged localization
ratchet, catalog validation (339 decisions / 1282 specifications), spec lint,
diff checks, and actual changed-file PR documentation coverage all pass.
Task 01 records exact commands and permanent RED evidence. Publication,
current-head remote checks/reviews, and authorized normal merge remain pending
in the external task plan; local completion does not declare the task merged.

## Risks

- Guarding only final batch publication can lose current sibling results.
- Guarding only promise resolution can allow a queued React updater to publish
  after a newer intent; check again inside state reducers where applicable.
- Retiring a session must also suppress its debounce and finally paths.
- Depth-one placeholders must not discard still-loaded descendants, while an
  authoritative empty folder must clear its direct children.
