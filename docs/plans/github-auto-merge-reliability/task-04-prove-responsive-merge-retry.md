---
id: "04-prove-responsive-merge-retry"
title: "Prove responsive merge retry"
status: done
wave: 4
depends_on:
  - "03-expose-explicit-merge-retry"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.4
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.9
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---
# Task 04: Prove Responsive Merge Retry

## Summary

Extend mock GitHub behavior and prove the complete retry and reconciliation
outcome in the existing desktop popover and phone drawer.

## In scope

- Add mock outcomes for pending, failure, head mismatch, queue, and merge.
- Expose request counts and target pull-request identity to E2E tests.
- Cover the automatic failure and explicit retry flow on desktop.
- Cover the same outcome with touch input on mobile.
- Prove unchanged state does not retry automatically.
- Prove an active queue or merged observation clears only the merge error.
- Prove a loading error performs state refresh only.
- Assert the mobile target size, internal scrolling, and page overflow contract.

## Out of scope

- Product behavior beyond the approved requirements.
- A new E2E page object for unrelated pull-request actions.

## Acceptance

- Each authorization produces at most one provider request.
- Retry targets the selected pull request on desktop and mobile.
- Queue and merged observations remove obsolete merge errors.
- Responsive behavior meets the existing drawer and touch contracts.

## Verification

```bash
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/pr/ci-automation-options.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/pr/mobile-ci-automation-options.spec.ts
```

## Files likely touched

- `apps/backend/internal/github/mock_controller.go`
- `apps/web/e2e/api/github.ts`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`

## Dependencies

- Task 03 exposes the backend and frontend retry path.

## Risks

- E2E request counts need stable reset and isolation between tests.
- Mock pending behavior must not add real one-second delays to unrelated tests.

## Parallelism

`parallel-safe-with-task-05`

## Inputs

- Task 03 retry command and responsive controls.
- Existing automation E2E fixtures and mobile status drawer.

## Results

- Extended the mock provider with deterministic merged, queued, failed,
  pending-exhaustion, and head-mismatch outcomes plus expected-head attempt
  evidence.
- Added desktop and mobile flows that prove unchanged failures do not retry,
  explicit retry targets the same PR and reviewed head once, and accepted
  results clear the typed merge error.
- Proved lifecycle errors retain Refresh, the mobile retry target is at least
  44 by 44 CSS pixels, the drawer keeps internal scrolling, and the document
  does not overflow horizontally.
- Fixed the evaluator integration exposed by E2E: explicit retry now overrides
  the same-head queue guard, and concurrent PR events coalesce one follow-up
  evaluation instead of dropping it.
- Passed all 6 desktop Chromium cases and all 5 mobile Chrome cases in the
  task-defined specifications.
