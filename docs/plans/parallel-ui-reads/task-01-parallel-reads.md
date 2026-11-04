---
id: "01-parallel-reads"
title: "Parallelize independent UI reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/composer-file-search-ownership.md
---

# Task 01: Parallelize independent UI reads

## Summary

Start independent reads together in diff expansion, saved-file restoration,
mention lookup, and accepted-message refreshes. Preserve the existing ownership
and fallback contracts referenced above.

## In scope

- Parallel read admission with existing response guards and error handling.
- Four-file Dockview batches and four center-panel workers with stable result order.
- File-search cancellation through the existing client signal argument.
- Deferred-request regression coverage at the existing production boundaries.

## Out of scope

New product behavior, backend changes, write ordering, generic concurrency
frameworks, persistent caches, and visual or responsive layout changes.

## Acceptance

1. Independent requests start before the first response settles.
2. Restored files retain saved order, omit failed center-panel reads, and reject
   old-session Dockview writes after a session switch.
3. Diff error precedence, composer fallback, and accepted-message refresh
   behavior remain compatible with their existing callers.

## Verification

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run \
  components/diff/use-expandable-diff.test.ts \
  components/task/task-center-panel-restoration.test.ts \
  hooks/use-message-handler.plan-comments.test.ts \
  hooks/use-file-editors.open-action.test.tsx \
  components/task/chat/tiptap-input-file-search-ownership.test.tsx \
  hooks/use-message-handler.test.ts \
  hooks/use-file-editors.test.tsx \
  hooks/use-file-editors.mutation-ownership.test.tsx \
  hooks/use-file-rename-continuity.test.tsx \
  hooks/use-file-rename-promotion.test.tsx \
  components/task/task-center-panel-mutation-ownership.test.tsx \
  components/diff/diff-viewer.test.tsx \
  components/task/chat/composer-disclosure.test.tsx \
  lib/ws/workspace-files.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Normal commit hooks run changed-file ESLint and formatting.

## Files likely touched

- `apps/web/components/diff/use-expandable-diff.ts` and its test.
- `apps/web/hooks/use-file-editors.ts` and its open-action test.
- `apps/web/components/task/task-center-panel-restoration.ts` and its test.
- `apps/web/components/task/chat/tiptap-input.tsx` and its file-search ownership test.
- `apps/web/hooks/use-message-handler.ts` and its plan-comments test.

- `apps/web/lib/ws/workspace-files.ts` and its test.

## Dependencies

None. Existing UI ownership and fallback requirements remain authoritative.

## Risks

Late responses can target a retired session. Parallel failures can change error
precedence. The tests use controlled responses to exercise both boundaries.

## Parallelism

`sequential`

## Inputs

- [Task navigation design](../../specs/ui/system-design/task-navigation-responsiveness.md),
  scope and stale-response protection.
- [Composer search design](../../specs/ui/system-design/composer-file-search-ownership.md),
  settlement and current fallback.
- [Plan](plan.md).

## Results

Seven concurrency cases failed on the sequential implementation and passed
after the refactor. The changed suites passed 61 tests and the related suites
passed 107 tests, for 168 distinct passing cases. Typecheck, changed-file ESLint,
and the i18n ratchet passed. The affected message suite passed 19 tests after
commit-hook formatting. Catalog validation and specification lint passed. The documentation coverage
preflight passed against the actual PR file list plus these two new records.
Hosted CI and review completion remain pending. No browser E2E or latency
benchmark was run.


### Review remediation

A held first file no longer prevents free center-panel workers from starting
later files. Query, session, and unmount tests check signal retirement through
the real client. Both message-refresh promises remain deferred until each
request starts, so either sequential order fails the regression.
Final verification passed: 175 tests across 14 files, changed-file ESLint,
typecheck with an 8 GB Node heap, the i18n ratchet, catalog validation,
specification lint, and documentation coverage preflight. Hosted checks and
review resolution remain external delivery gates.


### CodeRabbit summary follow-up

The review body included five test suggestions without inline threads. The
existing regressions now check successful current-session restoration, preview
publication before comment completion, both diff reads settling before failure,
worker capacity after an early settlement, and no duplicate file read after
mention-task completion. The five affected suites passed 65 tests. Together
with the unchanged validated suites, final local coverage is 176 tests across
14 files. No production behavior changed in this follow-up.

### CI E2E remediation

The mobile turn-error action test measured its menu item during the opening
zoom animation. CI and the local production build both reported 43.07 px for
the 44 px item. The test now waits for the menu's finite animations before
checking its height. The size assertion is unchanged. Both tests in the mobile
spec passed without retries. The background-process test passed in isolation,
but its CI group exposed a resume race. Task 02 records that repair.

Base commit `a1013e89` also includes the animation wait. The merge keeps its
context-menu selector and removes the duplicate import and wait.
