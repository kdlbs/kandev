---
id: "04-e2e-rerequest-review"
title: "Desktop and mobile E2E"
status: done
wave: 3
depends_on:
  - "01-backend-review-request"
  - "02-frontend-dismissed-review-action"
  - "03-mobile-github-pr-review"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 04: Desktop and mobile E2E

## Acceptance

- Desktop E2E proves dismissed review to requested/pending transition through
  the user-visible PR panel.
- Mobile E2E enters through **Review**, proves the same transition, and checks
  touch target, viewport containment, and no document horizontal overflow.
- Tests seed via mock APIs, act only through the UI, and use stable
  reviewer-specific selectors.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/pr/pr-rerequest-review.spec.ts tests/pr/mobile-pr-rerequest-review.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/pr/pr-rerequest-review.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-rerequest-review.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/helpers/api-client.ts` (only if existing seed helpers cannot
  express the scenario)

## Dependencies

Tasks 01, 02, and 03.

## Inputs

- All spec scenarios.
- `apps/web/e2e/tests/pr/pr-approve-button.spec.ts`.
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`.
- Mobile parity and E2E skills.

## Output contract

Report summary, files changed, RED/GREEN/REFACTOR evidence, commands/results,
failure-artifact paths when relevant, blockers, risks, and set only this task
file's status to `done`. Do not edit `plan.md`.
