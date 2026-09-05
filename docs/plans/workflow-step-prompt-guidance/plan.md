---
created: 2026-09-05
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Prompt Guidance

## Overview

Add visible guidance to the workflow step editor. Keep prompt content separate
from the `auto_start_agent` entry action.

## Scope

### In scope

- Show an inline warning for a non-empty step prompt without automatic start.
- Remove the warning when the prompt is empty or automatic start is enabled.
- State the `{{task_prompt}}` replacement rule in visible editor text.
- Add complete translations for all supported locales.
- Update the public workflow reference with the same trigger and replacement rules.
- Prove the same outcome on desktop and mobile.

### Out of scope

- Infer `auto_start_agent` from a non-empty prompt.
- Block workflow saves.
- Change backend workflow dispatch or prompt composition.
- Change manual workflow-step launch behavior.

## Technical approach

Update `StepPromptSection` to derive the warning from `localPrompt` and the
step entry actions. Reuse the inline amber warning style from
`SessionConfigCarryWarningPanel`.

Add localized copy to each `workflows.json` catalog. Update the visible usage
hint so it names the task-description replacement rule.

Update the step settings table in `docs/public/workflow-tips.md`. State that a
step prompt supplies content and does not trigger an automatic run.

Add one desktop Playwright scenario to `workflow-settings.spec.ts`. Add the
matching phone scenario to `mobile-workflow-settings.spec.ts`. Each scenario
creates a disposable workflow, enters a step prompt, and observes the warning.
It then enables automatic start and observes that the warning disappears.

The phone test uses the existing workflow settings page. The workflow card
remains the single scroll owner. The warning adds no control, overlay, or
viewport-specific state.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1` through `.4` | Desktop scenario in `apps/web/e2e/tests/workflow/workflow-settings.spec.ts` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5` | Phone scenario in `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` |

The public documentation validators cover the workflow reference update.

## E2E tests

- Desktop Chromium: configure a prompt without automatic start, read both
  guidance messages, enable automatic start, and observe warning removal.
- Mobile Chromium: repeat the same flow with touch input and assert that the
  page has no horizontal overflow.

## Work orders

- [x] [Task 01: Show step prompt guidance](task-01-show-step-prompt-guidance.md)

## Verification results

- Desktop Chromium prompt-guidance scenario passed against a production build.
- Mobile Chromium prompt-guidance scenario passed against the same build and
  captured the phone-width warning state.
- Web TypeScript and localization checks passed.
- Public documentation tests and validation passed.
- Specification linter tests and full specification validation passed.
- `git diff --check` passed.

## Risks

- Monaco input needs a stable browser interaction before the debounced step
  update completes.
- Long translations can expose narrow-screen wrapping errors.
