---
id: multi-pr-review-03
title: Route inline review through exact selected PR
status: done
wave: 2
depends_on: [multi-pr-review-01]
plan: ./plan.md
---

## Acceptance

- Inline review sources and progress use selected PR instead of task primary.
- Every PR timeline file carries stable PR identity through dockview and mobile sheet routes.
- Same-repository, same-path sibling rows render the clicked PR patch.

## Files

- `apps/web/hooks/domains/session/use-review-sources.ts`
- `apps/web/components/task/task-changes-panel.tsx`
- `apps/web/components/task/changes-top-bar.tsx`
- Changes data/helper/timeline/PR-file modules and tests
- `apps/web/components/task/changes-diff-target.ts`
- Dockview panel/store routing modules and tests
- Mobile changes panel/diff sheet

## Inputs

- Task 01 hook/state
- Existing `useActiveTaskPRsWithFiles()` fan-out
- PR-only raw file behavior in `TaskChangesPanel`

## Method

Use RED → GREEN → REFACTOR for selection, progress, and identity propagation.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/use-review-sources.test.ts components/task/changes-panel-helpers.test.ts components/task/changes-panel-pr-files.test.tsx components/task/dockview-panel-content.diff.test.tsx lib/state/dockview-panel-actions.test.ts`

## Output contract

Report summary, files, RED and GREEN results, blockers, and risks; mark status done only after focused verification passes.
