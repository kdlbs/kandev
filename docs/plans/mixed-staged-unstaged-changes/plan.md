---
created: 2026-08-27
status: completed
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Implementation Plan: Mixed Staged and Unstaged Changes

## Overview

Preserve both Git layers for one path, then project them into accurate Changes rows and diff targets.
Implementation starts with red backend, desktop, and mobile regressions; publishes the additive
agentctl contract next; then consumes it in the shared frontend so both platform layouts turn green.

## Scope

### In scope

- Preserve staged and unstaged facets for ordinary mixed porcelain states such as `MM` and `AM`.
- Keep one raw repository/path identity while rendering facet-specific rows, stats, and diffs.
- Route desktop and mobile diff selection by change layer.
- Preserve current file-level stage, unstage, discard, Review, and unique changed-file counts.
- Keep facet diff content inside the established status-snapshot budget.

### Out of scope

- Hunk-level staging.
- New conflict or unmerged-state semantics.
- A Changes panel visual redesign.
- A new on-demand diff API.

## Technical approach

### Agentctl status contract

Add `FileChangeFacet` plus optional `StagedChange` and `UnstagedChange` fields to
`apps/backend/internal/agentctl/types/streams/git.go`. Refactor `applyPorcelainLine` in
`workspace_git_status.go` to interpret the `X` and `Y` columns independently while preserving the
one-path compatibility projection.

Extend `workspace_git_diff.go` so a mixed path retains its existing `HEAD`-relative flattened diff,
gets a staged `git diff --cached HEAD -- <path>` facet, and gets an unstaged
`git diff -- <path>` facet. Teach diff-budget accounting and same-HEAD carry-forward to traverse the
optional facets. No facet diff may bypass the shared threshold or existing skip-reason vocabulary.

### Frontend projection

Mirror the optional facet type in both frontend wire models. Add a pure projection helper under
`apps/web/hooks/domains/session/` that returns staged and unstaged `FileInfo` views for Changes while
keeping `allFiles` unique. `useSessionGit` and multi-repository summaries use those projected arrays
for section and action availability. Store equality, editor synchronization, and Changes focus
fingerprints include facet data so a layer-only update is observable.

Review's aggregate uncommitted source continues to use the flattened compatibility file. Mutation
dispatch continues to receive only repository plus path, even when both projected rows exist.

### Layer-qualified diff selection

Add a staged/unstaged layer to `OpenDiffOptions` and file-mode `DiffSheetMode`. Propagate it through
Changes rows, TaskChangesPanel selection, Dockview preview/pinned IDs, and the mobile diff drawer.
Layer-qualified selections project the matching facet; legacy and non-layer selections keep the
combined uncommitted view. The same shared Changes body remains in desktop and mobile layouts.

This approach implements
[ADR-2026-08-27-mixed-git-change-facets](../../decisions/2026-08-27-mixed-git-change-facets.md).

## Tests

- `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7` and `.9`: add
  `apps/backend/internal/agentctl/server/process/workspace_git_mixed_changes_test.go` covering `MM`,
  `AM`, independent numstat/diff content, carry-forward, and shared budget accounting.
- `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10` and `.11`: add
  `apps/web/hooks/domains/session/git-change-facets.test.ts` for unique raw identity, two projections,
  layer stats, and mutation-path deduplication; extend Git summary and status equality tests.
- `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10`: extend
  `apps/web/components/task/task-changes-panel.test.ts` and
  `apps/web/lib/state/dockview-panel-actions.test.ts` for layer-specific content and panel identity.
- `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12`: add focused mobile component coverage proving the same
  facet target reaches the existing full-height diff drawer.

## E2E tests

- Desktop, `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10` and `.11`: extend
  `apps/web/e2e/tests/git/git-changes-panel.spec.ts` with a tracked `MM` file. Assert the same path is
  present in both sections, each row shows its own line count and diff, and unstage leaves the
  unstaged facet.
- Mobile, `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12`: extend
  `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts` with the same setup and assert both sections,
  layer-specific drawer content, and touch staging behavior.

## Work orders

- [x] [Task 01: Capture mixed-change regression](task-01-capture-mixed-change-regression.md)
- [x] [Task 02: Publish mixed-change facets](task-02-publish-mixed-change-facets.md)
- [x] [Task 03: Render layer-specific changes](task-03-render-layer-specific-changes.md)

## Verification results

- Task 01 RED evidence captured. Agentctl omitted `staged_change`; desktop and mobile could not find
  the staged section for the mixed path.
- Task 02 published additive staged and unstaged facets with independent diffs, bounded shared
  accounting, and budget-aware carry-forward. The focused and package-level Go suites passed.
- Task 03 projected one raw path into two layer-qualified rows and diff targets while retaining
  unique totals and mutation identity. Focused frontend tests, typecheck, lint guards, and fresh
  desktop/mobile Playwright regressions passed, including the unstage transition.

## Risks

- Compatibility and facet diffs duplicate content for mixed paths; incorrect budget accounting could
  exceed the bounded snapshot contract.
- A path shown twice can accidentally double-count badges or dispatch the same mutation twice unless
  raw identity and projected presentation remain separate.
- Omitting the layer from a Dockview or mobile selection key would reopen the bug by showing the
  combined or wrong facet diff.
- Rename/delete combinations use different porcelain and numstat path shapes; parser tests must prove
  that ordinary supported mixed states retain the correct final path.
