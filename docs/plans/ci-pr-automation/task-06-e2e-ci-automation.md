---
id: "06-e2e-ci-automation"
title: "E2E CI automation"
status: done
wave: 4
depends_on: ["03-backend-automation-execution", "05-frontend-popover-controls"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 06: E2E CI Automation

## Acceptance

- Playwright covers desktop popover and mobile drawer control visibility.
- Playwright or backend integration coverage verifies persisted toggles and prompt override reset.
- Playwright covers the automation help affordance and the task prompt editor's Settings > Prompts link.
- Automation behavior is covered end-to-end where practical, or by documented backend integration tests where direct UI observation is not reliable.

## Verification

```bash
cd apps/web && rtk pnpm e2e:run tests/pr/ci-automation-options.spec.ts tests/pr/mobile-ci-automation-options.spec.ts -- --project=chromium --project=mobile-chrome
cd apps/web && rtk pnpm typecheck
cd apps/web && rtk pnpm lint
```

## Progress

- Added component-level mobile drawer coverage in `apps/web/components/github/pr-status-chip.test.tsx`
  for automation control visibility, prompt editor opening, and Settings > Prompts link.
- Added Playwright desktop popover coverage in `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  for control visibility, persisted toggles, help copy, task prompt save/reset, and Settings > Prompts link.
- Added Playwright mobile drawer coverage in `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`
  for control visibility, persisted auto-fix toggle, prompt editor opening, and Settings > Prompts link.
- Verified both specs with the focused Chromium and mobile Chrome projects.

## Files Likely Touched

- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/web/e2e/helpers/api-client.ts`
- Existing GitHub mock/e2e helpers under `apps/web/e2e/`
- Backend mock control routes if the existing GitHub mock cannot express required PR states

## Dependencies

- `03-backend-automation-execution`
- `05-frontend-popover-controls`

## Inputs

- Spec sections: Scenarios.
- Plan sections: E2E Tests.
- Existing patterns: PR status chip E2E helpers, mobile drawer tests, GitHub mock controls.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 4 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
