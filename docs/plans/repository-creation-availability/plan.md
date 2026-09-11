---
created: 2026-09-10
status: implemented
requirements:
  - REQ-WORKSPACES-CREATE-LOCAL-REPOSITORY-001
system_design:
  - ../../specs/workspaces/system-design/create-local-repository.md
legacy_specs: []
---

# Implementation plan: Repository creation availability

## Overview

Expose creation in every editable local repository row. One sequential work order
updates the picker, creation context, success handler, documentation, and focused tests.

## Evidence and root cause

`apps/web/components/task-create-dialog-workspace-repo-chips.tsx:135` supplies
`onCreateRepository` only when `rows.length === 1`. The adjacent refresh callback
has the same condition, explaining why both icons disappear in the screenshot.
The condition counts task rows, not registered repositories. The existing test
`routes repository creation to the only row` supplies two registered repositories
and still exercises creation. `Pill` itself does not condition its action on options.

The old restriction is explicit in the original requirements and implementation
plan. It is no longer suitable for the requested behavior. Backend initialization
already supplies an initial commit, but `applyCreatedLocalRepository` still always
selects a direct-local executor. Removing only the visibility gate would therefore
select an executor that cannot run a multi-repository task.

Reproduction from source: supply two task rows and an opted-in creation callback,
open either local picker, and observe that no creation callback reaches its Pill.
The supplied screenshots corroborate the toolbar difference. No live instance was
mutated and no browser reproduction or product tests ran during this design turn.

## Scope

- Every editable local row offers Refresh and Create across empty, populated, and filtered lists.
- Creation selects the originating row and preserves siblings and multi-row executors.
- Desktop and phone complete the same create-and-submit outcome.
- Update public wording that limits creation to a single row.

Excluded: remote-host creation, locked/edit-mode selectors, Quick Chat opt-in,
backend initialization changes, executor capability
expansion, and a global picker redesign. Retain single-row executor behavior.

## Technical approach

Follow the [system design](../../specs/workspaces/system-design/create-local-repository.md).
Remove the creation and refresh row-count gates in `WorkspaceRepoChips`. Derive
explicit multi-row context in `RepoChips` and pass it to the creation surface.
Separate row/cache updates from optional executor changes in the handler.
Multi-row creation must work without a direct-local profile and preserve the
selected profile; normal submission guards remain authoritative.

Use semantic button queries in tests. Existing negative tests query tooltip text,
which can be absent even when the icon button exists. Cover accessible button
presence and activation instead. Reuse current translation keys when suitable.

## ASCII UI preview

UI-01: Local picker, second task row, populated list.

```text
Before: [Search repositories...          ]
        [test                          v]

After:  [Search repositories... ] [Refresh] [Create +]
        [test                          v]
```

Both actions stay above the list with zero matches or zero repositories too.
Refresh precedes Create and stays visible but disabled while refreshing.
The illustration expands the icon's accessible label; existing localized copy
and icon remain authoritative.

UI-02: Phone creation flow after tapping the same picker action.

```text
[Create new repository              Close]
[Repository name                        ]
[Parent directory                       ]
[Target path                            ]
[Directory list: internal scroll        ]
[             Create repository         ]
```

Retain the shipped phone Drawer: fixed title/footer, one scroll owner, safe-area
clearance, and touch targets at least 44px. Desktop opens the existing Dialog.
The picker remains viewport-contained. Return focus to the initiating row on
dismissal. Drawings specify order and persistence, not exact spacing.

## Tests

AC references below use prefix `AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001`.

| Evidence | Acceptance |
| --- | --- |
| `task-create-dialog-workspace-repo-chips.test.tsx`: action remains available in every row | .1 |
| Same file: zero/two repository options, zero-match search, caller not opted in | .1 |
| Same file: refresh from each row, unchanged selection, visible disabled loading state | .1 |
| `task-create-dialog-handlers.test.ts`: target-row/cache update, sibling preservation, executor preservation | .7, .8 |
| `create-local-repository-surface.test.tsx`: multi-row creation with no direct-local profile | .8 |
| `task-create-dialog-prop-builders.test.ts`: creation context and locked/edit exclusions | .1, .8 |

The first multi-row button assertion is the required RED test before production edits.
Also cover stale target-row removal and preserve single-row executor-switch tests.

The desktop E2E also exposed a premature loading reset in `useRepositories`.
Its deferred-response regression in `hooks/domains/workspace/use-repositories.test.ts`
proves the cached workspace stays loading until the request finishes (.1).

## E2E tests

Extend `apps/web/e2e/tests/task/create-task-new-local-repository.spec.ts` and
`mobile-create-task-new-local-repository.spec.ts` for .1, .7, and .8. Start with
an existing repository and Worktree, add a second row, create another repository,
assert the first row and executor remain unchanged, and submit. Verify both
persisted repository identities and `main` for the created repository. Reopen a
picker and verify both actions remain available. Refresh from the second row and
assert updated options without changing selections. Keep existing single-row and conflict
scenarios. Phone coverage uses touch and checks action hitbox, drawer containment,
focus return, and no horizontal document overflow.

## Companion plans and documentation

The [original plan](../create-local-repository/plan.md) and its selector work order
record the old restriction as shipped history. Add a follow-up pointer during
implementation; do not rewrite their historical verification results. The
[descriptor repair](../fix-local-repository-darwin-init/plan.md) has no affected
backend work or checks.

Public how-to pages `docs/public/use-kandev.md` and
`docs/public/tasks-and-workflows.md` contain explicit single-row-only wording.
Update them with the implementation, including the multi-row executor distinction.
This design turn updates internal documents only.

## Work orders

- [x] [Task 01: Keep repository creation available](task-01-keep-creation-available.md)

## Verification results

Implementation complete. Exact commands and environment adjustments are recorded
in [Task 01 results](task-01-keep-creation-available.md#results).

- Seven focused Vitest files: 108 tests passed.
- Desktop E2E: 3 tests passed. Phone E2E: 2 tests passed.
- TypeScript, targeted ESLint, localization, and the fresh Vite build passed.
- Public-doc validators passed: 62 tests and 46 pages.
- Specification validation passed: 36 linter tests and all specification files.
- Diff checks passed. No commit or push was requested.

Design validation:

- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `git status --short -- docs/plans/repository-creation-availability`: confirmed
  the new package is untracked; no commit or push was requested.
- Workspace index: 2,655 bytes, within the size limit.

Desktop and phone checks cover the assigned toolbar order, touch sizing, creation
surface, preserved repository rows and executor, refresh loading, and task submission.

Post-PR fixup validation (2026-09-11) also covers submission-bound completion
callbacks and shared repository request ownership: the focused Vitest suite passed
130 tests, `pnpm run typecheck`, `pnpm run i18n:check`, targeted ESLint, the fresh
frontend build, desktop E2E (3 passed), mobile E2E (2 passed), public documentation
validation (62 tests and 46 pages), specification validation (36 tests), and
`git diff --check` all passed.

## Risks

- A direct-local switch after creation would invalidate a multi-row draft.
- Index-based callbacks could select the wrong row after removal; use stable keys.
- Hidden tooltip text is not evidence that an action button is absent.
- Shared surface context must not change existing workspace creation behavior.
