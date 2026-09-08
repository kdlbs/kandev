---
id: "03-mobile-github-pr-review"
title: "Mobile GitHub PR review surface"
status: done
wave: 2
depends_on: ["02-frontend-dismissed-review-action"]
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 03: Mobile GitHub PR review surface

## Acceptance

- A phone task with a linked GitHub PR shows **Review** in bottom navigation
  and opens the shared GitHub PR detail content.
- Multi-PR tasks use shared PR selection; tasks with no GitHub PR retain the
  existing GitLab MR Review behavior.
- Surface keeps one scroll owner, safe-area/bottom-nav clearance, touch-sized
  actions, and no viewport-wide horizontal overflow.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- --run \
  components/task/mobile/session-mobile-layout.test.tsx \
  hooks/domains/github/use-review-pr-selection.test.ts
```

## Files likely touched

- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/review/review-pr-selector.tsx` (only if composition
  needs an existing selector prop/test hook)
- `apps/web/hooks/domains/github/use-review-pr-selection.ts`
- `apps/web/hooks/domains/github/use-review-pr-selection.test.ts`

## Dependencies

Task 02.

## Inputs

- Plan mobile design contract.
- Existing GitLab mobile Review implementation in
  `session-mobile-layout.tsx`.
- Existing shared `ReviewPRSelector` and `useReviewPRSelection`.

## Output contract

Report summary, files changed, RED/GREEN/REFACTOR evidence, commands/results,
blockers, risks, and set only this task file's status to `done`. Do not edit
`plan.md`.
