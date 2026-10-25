---
id: "11-lifecycle-e2e"
title: "Lifecycle UI E2E coverage"
status: done
wave: 7
depends_on:
  - "09-backend-lifecycle-reliability"
  - "10-frontend-lifecycle-feedback"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 11: Lifecycle UI E2E coverage

## Acceptance

- Desktop Playwright coverage proves the connected-account switch label and a
  selected-PR runtime automation error are visible in the PR popover.
- `mobile-chrome` coverage proves the same user value in the existing PR status
  drawer, including internal scrolling and no document horizontal overflow.
- Existing toggle persistence and auto-fix prompt-editor coverage remains
  intact.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts tests/pr/mobile-ci-automation-options.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`
- This task file

## Dependencies

- Tasks 09 and 10.

## Inputs

- Spec: lifecycle UI/error scenarios.
- Plan: E2E scenarios and mobile design contract.
- Existing fixtures: `fixtures/test-base`, `helpers/api-client.ts`, and
  `pages/session-page.ts`.

## Constraints

- Use E2E TDD: prove the assertion fails before relying on the implementation.
- Use the managed headless runner and production build.
- Seed through existing mock GitHub/API fixtures and assert through the UI.
  Browser request interception may supply a `last_error` response when no
  existing seed API can establish it; do not add a production endpoint only
  for the test.
- Keep the phone spec name prefixed with `mobile-` so `mobile-chrome` selects
  it.
- Do not edit application code or `plan.md`; update only these E2E specs and
  this task file.

## Output contract

- Scenarios added or updated.
- Files changed.
- Exact managed-runner result and any environment blocker/artifact path.
- Blockers, divergence, and follow-up risks.
- Set this task file to `done` only after acceptance and targeted verification
  pass, or report the exact infrastructure blocker without claiming a pass.
