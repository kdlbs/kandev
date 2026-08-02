---
id: "03-pr-unlink-e2e"
title: "PR unlink E2E coverage"
status: done
wave: 3
depends_on: ["01-backend-unlink-contract", "02-frontend-multi-pr-unlink"]
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 03: PR Unlink E2E Coverage

## Acceptance

- Desktop Playwright coverage unlinks the older of two PRs from the hover
  popover and proves the sibling plus persisted single-PR state remain after
  reload.
- Mobile Playwright coverage performs the same action through the existing PR
  status drawer using a touch event, verifies the close target hitbox, and
  proves document containment.
- Shared page-object/API seed helpers identify associations without relying on
  unstable visible copy.

## Verification

```bash
cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/pr/pr-multi-popover.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/pr/mobile-pr-ci-chip.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/helpers/api-client.ts` only if the existing seed helpers cannot
  express the required association identity

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. The scenarios exercise the completed backend contract and shared
frontend component.

## Inputs

- Spec scenarios: desktop unlink and coarse-pointer unlink.
- Plan section: `E2E Tests` and the complete mobile design contract.
- Existing seed/open patterns in the two named PR specs.

## Risks

- The mobile project only discovers `mobile-*.spec.ts`; keep the existing file
  name and run the managed E2E runner so production frontend/backend artifacts
  are rebuilt.
- When the second PR disappears the drawer/popover may unmount; assert the
  resulting task state rather than waiting on a detached close control.

## Output contract

Report RED/GREEN Playwright evidence, screenshots or rendered inspection,
files changed, exact command result, remaining risks/blockers, and update this
task plus `plan.md` status in the same conversation.
