---
id: "03-e2e"
title: "Review status E2E coverage"
status: done
wave: 3
depends_on: ["02-review-ui"]
plan: "plan.md"
spec: "../../specs/ui/review-file-status.md"
---

# Task 03: Review status E2E coverage

Verify the complete Review workflow against a production Vite build on desktop and mobile.

## Acceptance

- Desktop Review visibly distinguishes added, modified, deleted, and renamed files and keeps the marker visible at the 160 px sidebar minimum.
- A pure rename with no textual patch remains in Review and shows the moved-without-textual-changes state.
- Mobile Review exposes the status in the sticky diff header, keeps controls usable, and introduces no document-level horizontal overflow.

## Verification

- `cd apps && pnpm --filter @kandev/web e2e:run tests/review/review-file-status.spec.ts`
- `cd apps && pnpm --filter @kandev/web e2e:run --project mobile-chrome tests/review/mobile-review-file-status.spec.ts`

## Files likely touched

- `apps/web/e2e/tests/review/review-file-status.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-file-status.spec.ts`

## Inputs

- Spec: desktop, patchless, narrow-sidebar, and mobile scenarios.
- Task 02 DOM/accessibility contracts; select markers by semantic label or stable data attribute, never by color class.
- Patterns: `GitHelper` in `apps/web/e2e/helpers/git-helper.ts`, Review opening in `review-cumulative-diff.spec.ts`, sidebar sizing in `review-sidebar-resize.spec.ts`, and mobile Review entry in `task/mobile-changes-panel.spec.ts`.

## Dependencies

Task 02.

## Output contract

Report the files changed, exact E2E command/result, failure artifacts or blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
