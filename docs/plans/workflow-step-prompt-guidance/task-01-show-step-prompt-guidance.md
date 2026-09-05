---
id: "01-show-step-prompt-guidance"
title: "Show step prompt guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Show Step Prompt Guidance

## Summary

Show a warning when a step prompt has no automatic start trigger. Explain the
task-description replacement rule in visible text.

## In scope

- Add the derived warning state and inline warning panel.
- Update all workflow locale catalogs.
- Update the public workflow reference.
- Add desktop and mobile Playwright coverage.

## Out of scope

- Change backend workflow behavior.
- Add a blocking validation rule.
- Change the manual start path.

## Acceptance

- A non-empty prompt without `auto_start_agent` shows the warning immediately.
- Clearing the prompt or enabling `auto_start_agent` removes the warning.
- The editor states the `{{task_prompt}}` replacement rule on desktop and mobile.

## Verification

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
pnpm --dir apps/web e2e:run tests/workflow/workflow-settings.spec.ts -- --grep "warns when a step prompt has no automatic start"
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts -- --grep "warns when a step prompt has no automatic start"
```

## Files likely touched

- `apps/web/components/settings/workflow-step-prompt-section.tsx`
- `apps/web/src/locales/en/workflows.json`
- `apps/web/src/locales/pt-pt/workflows.json`
- `apps/web/src/locales/zh-cn/workflows.json`
- `apps/web/src/locales/zh-hk/workflows.json`
- `apps/web/src/locales/zh-tw/workflows.json`
- `apps/web/src/locales/pseudo/workflows.json`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `docs/public/workflow-tips.md`

## Dependencies

None.

## Risks

- The test must wait for Monaco to accept input before it checks the warning.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005`
- `docs/specs/tasks/system-design/workflow-step-agent-start-ownership.md`
- `apps/web/components/settings/workflow-session-config-carry-warning.tsx`
- Existing workflow settings Playwright scenarios.

## Results

- Added a derived inline warning for a non-empty step prompt without the
  `auto_start_agent` entry action. The warning updates when either setting
  changes and does not block saving.
- Added visible replacement guidance and complete translations for every
  supported locale.
- Updated the public workflow reference with the trigger and replacement rules.
- Added desktop and mobile production E2E coverage. The mobile scenario also
  verifies viewport bounds and horizontal overflow.
- Passed the web type check and localization check.
- Passed both public documentation validators and both specification validators.
- Passed the focused desktop and mobile E2E scenarios.
- Passed `git diff --check`.
