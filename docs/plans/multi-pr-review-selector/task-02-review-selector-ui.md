---
id: multi-pr-review-02
title: Add selected-PR controls to expanded Review
status: done
wave: 2
depends_on: [multi-pr-review-01]
plan: ./plan.md
---

## Acceptance

- Review shows selector only for two or more PRs and switches files atomically with loading, empty, error, and retry states.
- Switch resets stale file/filter/scroll state and never closes Review during fetch.
- Desktop, phone, and tablet share behavior; phone menu has 44px targets and bounded internal scrolling.

## Files

- `apps/web/components/task/use-review-dialog.ts`
- `apps/web/components/task/dockview-review-dialog.tsx`
- Desktop/mobile/tablet task layout mounts
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/review/review-top-bar.tsx`
- New `apps/web/components/review/review-pr-selector.tsx`
- Focused Review unit tests

## Inputs

- Task 01 hook/state
- `pr-topbar-button.tsx` menu pattern
- Mobile global menu containment and mobile-parity contract

## Method

Use RED → GREEN → REFACTOR. Test pure state/file behavior first; use Playwright in Task 04 for rendered interaction.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run components/task/use-review-dialog.test.ts components/review/review-dialog.build-files.test.ts`

## Output contract

Report summary, files, RED and GREEN results, responsive choices, blockers, and risks; mark status done only after focused verification passes.
