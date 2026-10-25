---
id: "02-frontend-dismissed-review-action"
title: "Dismissed-review action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 02: Dismissed-review action

## Acceptance

- An open PR's latest dismissed review exposes one reviewer-named re-request
  action; other states and non-open PRs do not.
- Current requested-reviewer state wins over dismissed history
  case-insensitively, renders once as pending, and blocks duplicate requests.
- The action sends the fixed API payload, exposes busy state, refreshes after
  success, and shows success/error toasts with retry preserved on failure.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- --run \
  lib/api/domains/github-api.test.ts \
  components/github/pr-reviews-section.test.tsx \
  components/github/pr-detail-panel.test.ts
```

## Files likely touched

- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/components/github/pr-detail-panel.tsx`
- `apps/web/components/github/pr-detail-panel.test.ts`
- `apps/web/components/github/pr-reviews-section.tsx`
- `apps/web/components/github/pr-reviews-section.test.tsx`
- `apps/web/components/github/pr-shared.tsx`

## Dependencies

None. The route/payload contract is fixed by the spec; browser integration
waits for Task 01.

## Inputs

- Spec `What`, failure modes, and desktop scenarios.
- `ApproveButton` mutation/toast/refresh pattern.
- Mobile-parity touch-target rules, though phone composition belongs to Task
  03.

## Output contract

Report summary, files changed, RED/GREEN/REFACTOR evidence, commands/results,
blockers, risks, and set only this task file's status to `done`. Do not edit
`plan.md`.
