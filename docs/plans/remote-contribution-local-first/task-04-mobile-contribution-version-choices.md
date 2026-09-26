---
id: "04-mobile-contribution-version-choices"
title: "Mobile contribution version choices"
status: completed
wave: 3
depends_on: ["02-local-first-relation-and-shared-actions"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 04: Mobile Contribution Version Choices

Add the same version choices to the mobile Git menu. Use a phone-native confirmation drawer for each
destructive action.

## Inputs

- Spec scenario: Provide the same choice on mobile.
- Plan section: Mobile design contract.
- Mobile exemplars: `session-mobile-top-bar-git-controls.tsx` and
  `components/kanban/mobile-menu-sheet.tsx`.
- Task 02 relation and resolution state.

## Acceptance

1. The mobile Git menu exposes **Replace PR branch**, **Use PR version**, and **PR #<number> version** for a
   diverged contribution.
2. The destructive confirmations use an inset bottom drawer with 44px actions, one scroll owner,
   safe-area clearance, and focus return.
3. Mobile uses the same expected head, repository scope, loading state, and result handling as desktop.

## Files Likely Touched

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx`
- `apps/web/components/task/mobile/mobile-git-push-submenu.tsx`
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.tsx` (new)
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.test.tsx` (new)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/mobile/session-mobile-top-bar-git-controls.test.tsx components/task/mobile/mobile-contribution-resolution-drawer.test.tsx && cd web && pnpm run typecheck
```

## Dependencies

Task 02.

## Parallelism

Parallel-safe with Task 03 after Task 02. The two tasks own disjoint presentation files. The primary
conversation remains sequential unless the user authorizes subagents.

## Risks

- Do not add a nested vertical scroller to the existing mobile Changes surface.
- Do not mount the desktop dialog and hide it on phone.
- Do not leave force-push reachable only through a submenu that still uses the old disabled policy.

## Output Contract

Report the mobile entry point, drawer geometry, accessibility behavior, rendered phone evidence, and
exact test results. Update this task and `plan.md` in the same conversation.

## Results

Completed 2026-08-12.

- The mobile Git menu keeps Commit first and exposes repository-scoped **Replace PR branch**, **Use PR
  version**, and **PR #<number> version** actions for diverged contributions. Generic Push, force-push, and
  Pull entries are not reachable for the selected drift state.
- Destructive confirmation uses an inset bottom drawer with safe-area padding, 44px action targets,
  one inner error scroll owner, and shared resolution state with the desktop dialog.
- Mobile uses the exact provider head and explicit repository scope for both operations, with translated
  stale-lease, dirty-tree, and recovery-branch feedback.
- Verification passed: the shared focused desktop/mobile suite (26 tests in 7 files),
  `pnpm run typecheck`, and the Pixel 5 local-first Playwright scenario (1 passed, retries disabled).
