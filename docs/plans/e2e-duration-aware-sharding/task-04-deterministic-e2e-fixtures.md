---
id: "04-deterministic-e2e-fixtures"
title: "Repair deterministic E2E fixtures"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 04: Repair deterministic E2E fixtures

## Acceptance

- The disabled-integrations navigation assertion targets one intended link and
  no longer fails because several visible links match `/GitHub/`.
- Desktop and mobile multi-PR review tests derive the expected repository name
  from the seeded repository/task fixture, including dynamically generated
  names.
- Prompt-autocomplete coverage starts from a verified empty or intentionally
  seeded draft and cannot inherit an unrelated session-storage draft from a
  prior test.

## Verification

```sh
cd apps/web
pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts e2e/tests/review/review-multi-pr.spec.ts e2e/tests/task/task-create-prompt-autocomplete.spec.ts
pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/review/mobile-review-multi-pr.spec.ts
```

Each regression must fail for the original selector, repository-name, or
restored-draft defect before the repair and pass without increasing retries or
timeouts.

## Files likely touched

- `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`
- `apps/web/e2e/helpers/multi-pr-review.ts`
- `apps/web/e2e/tests/review/review-multi-pr.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-multi-pr.spec.ts`
- `apps/web/e2e/tests/task/task-create-prompt-autocomplete.spec.ts`
- `apps/web/components/task-create-dialog-state.ts` or
  `apps/web/e2e/fixtures/test-base.ts` if the smallest isolation boundary is
  there

## Dependencies

None. The changes are independent of the shard planner.

## Parallelism

Parallel candidate with Tasks 01, 05, and 06 because it changes deterministic
test fixtures and selectors only.

## Inputs

- The seven retry groups observed in PR #2471.
- Dynamic repository creation in `apps/web/e2e/helpers/multi-pr-review.ts`.
- Draft restore/save behavior in
  `apps/web/components/task-create-dialog-state.ts`.

## Output contract

Report each original failure mechanism, the deterministic fixture or locator
change, targeted test results for desktop and mobile, and any state-cleanup
contract added for future tests.

## Implementation result

The desktop regression set passed 5/5 and the mobile multi-PR test passed as
part of the 3/3 mobile run. The fixes use an exact integration href, derive
review repositories from the seeded ID, and clear all task-create draft keys
before autocomplete assertions.
