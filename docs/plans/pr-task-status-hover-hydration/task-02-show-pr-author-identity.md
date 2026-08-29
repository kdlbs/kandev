---
id: "02-show-pr-author-identity"
title: "Show PR author identity"
status: done
wave: 2
depends_on:
  - "01-hydrate-pr-details-on-disclosure"
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.2
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.7
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.8
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.17
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.18
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 02: Show PR Author Identity

## Summary

Show the GitHub author login below the PR title in the task summary. Show the
same identity in the existing coarse-pointer PR-status drawer and prove both
user flows end to end.

## In scope

- Add optional author data to the shared task-summary presentation.
- Derive the author from `TaskPR.author_login` and omit empty values.
- Add optional author content to the shared change-request popover header.
- Pass the GitHub author into `PRCIPopover` for desktop and drawer content.
- Add desktop and mobile Playwright coverage.

## Out of scope

- GitLab and registered-provider author derivation.
- New mobile navigation or touch controls.
- Provider refresh, association, or persistence behavior.

## Acceptance

- The hydrated task tooltip shows the non-empty GitHub author below each PR
  title and omits an empty author line.
- The existing mobile PR-status drawer shows the same author without changing
  task-row tap behavior.
- Desktop and mobile surfaces remain inside their viewports and preserve the
  existing structured status rows.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run components/github/pr-task-status-summary.test.ts components/integrations/change-request-task-status-summary.test.tsx components/github/pr-ci-popover.test.ts components/github/pr-status-chip.test.tsx)
(cd apps/web && pnpm e2e:run tests/pr/pr-sidebar-hover-hydration.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-status-summary.spec.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/components/integrations/change-request-task-status-summary.tsx`
- `apps/web/components/integrations/change-request-task-status-summary.test.tsx`
- `apps/web/components/integrations/change-request-ci-anatomy.tsx`
- `apps/web/components/github/pr-task-status-summary.tsx`
- `apps/web/components/github/pr-task-status-summary.test.ts`
- `apps/web/components/github/pr-ci-popover.tsx`
- `apps/web/components/github/pr-ci-popover.test.ts`
- `apps/web/components/github/pr-status-chip.test.tsx`
- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`

## Dependencies

- Task 01 must provide the compact-to-full disclosure path.

## Risks

- Shared optional author markup must not change providers that omit the value.
- Longer author identities can wrap and increase overlay height.
- Hidden Radix portals can make unscoped browser assertions ambiguous.

## Parallelism

`sequential`

## Inputs

- Requirement criteria `.2`, `.7`, `.8`, `.17`, and `.18`.
- Author presentation and mobile behavior in the system design.
- Existing task-summary, CI popover, status-chip drawer, and mobile sidebar
  tests.

## Results

- Added optional GitHub author presentation below task-summary and CI-popover
  titles, while omitting blank identities.
- Reused the existing mobile PR-status drawer and preserved task-row
  navigation.
- Added summary, popover, drawer, desktop E2E, and mobile E2E coverage.
- Kept one mounted tooltip trigger across compact-to-full hydration so keyboard
  focus and an open disclosure survive the store update.
- Focused unit tests, typecheck, full web lint, desktop E2E, mobile E2E, and the
  E2E build passed.
