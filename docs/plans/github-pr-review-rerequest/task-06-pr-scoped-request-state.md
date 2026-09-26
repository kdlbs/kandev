---
id: "06-pr-scoped-request-state"
title: "PR-scoped optimistic request state"
status: done
wave: 4
depends_on:
  - "02-frontend-dismissed-review-action"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 06: PR-scoped optimistic request state

## Acceptance

- A successful re-request immediately reconciles the reviewer to pending and
  cannot be replayed while refreshed feedback is delayed or fails.
- Failed writes clear only the in-flight state and remain retryable.
- Mutation state cannot survive a PR identity change.
- Extracted code keeps frontend file and function complexity within enforced
  limits.
- Deferred-success, refresh-failure, write-failure, and identity-reset behavior
  have focused tests.

## Ownership

- `apps/web/components/github/pr-detail-panel.tsx`
- `apps/web/components/github/pr-detail-panel.test.ts`
- A new focused hook/helper and test beside the GitHub PR components, if useful.

Do not edit the review-row or mobile-layout files owned by Task 07.

## Output contract

Use TDD. Report RED/GREEN evidence, exact checks, files changed, residual risks,
and set only this task file's status to `done`.
