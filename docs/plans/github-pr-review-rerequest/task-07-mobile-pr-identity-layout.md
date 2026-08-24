---
id: "07-mobile-pr-identity-layout"
title: "Mobile PR identity and narrow-row safety"
status: done
wave: 4
depends_on:
  - "03-mobile-github-pr-review"
  - "04-e2e-rerequest-review"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 07: Mobile PR identity and narrow-row safety

## Acceptance

- Switching the phone PR selector remounts or synchronously resets the detail
  surface before the next PR can receive actions.
- A delayed-fetch multi-PR test proves PR A's dismissed action cannot target PR
  B after selection changes.
- A maximum-practical-length reviewer login at a 320px phone viewport keeps the
  complete action inside the viewport with no document overflow.
- Desktop row density and accessible reviewer naming remain intact.

## Ownership

- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/github/pr-shared.tsx`
- `apps/web/components/github/pr-reviews-section.tsx`
- `apps/web/components/github/pr-reviews-section.test.tsx`
- `apps/web/e2e/tests/pr/mobile-pr-rerequest-review.spec.ts`
- Relevant E2E page/helper files only when required.

Do not edit PR mutation-state files owned by Task 06.

## Output contract

Use TDD. Report RED/GREEN evidence, exact checks, screenshots/traces on failure,
files changed, residual risks, and set only this task file's status to `done`.
