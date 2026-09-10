---
created: 2026-09-05
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Prompt Automatic-Start Default

## Overview

Replace missing-trigger recovery guidance with a one-way automatic-start
default when a user first authors a step prompt. Keep the persisted prompt and
entry action independently editable after that default is applied.

## Scope

### In scope

- Enable `auto_start_agent` when an editable prompt changes from empty to
  non-empty through typing, pasting, or a template.
- Preserve an explicit later disable while the prompt remains non-empty.
- Keep automatic start enabled when the prompt is cleared.
- Remove the missing-auto-start warning and its translations.
- State the `{{task_prompt}}` replacement rule in visible editor text.
- Add complete translations for all supported locales.
- Update the public workflow reference with the default and independence rules.
- Prove the same outcome on desktop and mobile.

### Out of scope

- Block workflow saves.
- Change backend workflow dispatch or prompt composition.
- Change manual workflow-step launch behavior.
- Migrate existing non-empty prompts that do not have `auto_start_agent`.
- Couple prompt clearing to automatic-start removal.

## Technical approach

Add a pure helper to `workflow-pipeline-editor-helpers.tsx` that returns an
`on_enter` update only for an empty-to-non-empty prompt transition without an
existing `auto_start_agent` action. Unit tests cover action preservation,
deduplication, clearing, non-empty edits after an explicit disable, and existing
saved prompts.

Move prompt-change orchestration into `StepConfigPanel`. Route both Monaco
changes and template selection through one callback. Update local prompt state
immediately. Publish the prompt and inferred entry-action update together when
the default applies. Publish later prompt edits without changing entry actions.

Remove the inline warning from `StepPromptSection` and delete its now-unused
locale key. Keep the consolidated `{{task_prompt}}` and saved-prompt guidance.
Update `docs/public/workflow-tips.md` to describe the editor default and clarify
that automatic start remains independently editable afterward.

Change the desktop Playwright scenario in `workflow-settings.spec.ts` to enter
a prompt, observe the checked Auto-start agent control, save, and verify the
persisted action. Then disable automatic start, edit the still-non-empty prompt,
and prove that it stays disabled. Change the mobile scenario to apply a prompt
template and prove the same automatic enablement without horizontal overflow.

The phone test uses the existing workflow settings card, which remains the
single scroll owner. This change adds no control, overlay, navigation, or
viewport-specific state.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1` through `.6` | Helper unit tests and desktop scenario in `apps/web/e2e/tests/workflow/workflow-settings.spec.ts` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.7` | Phone scenario in `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` |

The public documentation validators cover the workflow reference update.

## E2E tests

- Desktop Chromium: author a prompt, observe automatic start become checked,
  save and verify persistence, then prove a later explicit disable survives a
  non-empty prompt edit.
- Mobile Chromium: select a prompt template, observe the same automatic-start
  default, and assert that the page has no horizontal overflow.

## Work orders

- [x] [Task 01: Show step prompt guidance](task-01-show-step-prompt-guidance.md)
- [x] [Task 02: Auto-enable prompted steps](task-02-auto-enable-prompted-steps.md)

## Verification results

- Passed the focused helper and prompt-section unit tests.
- Passed the web type check, lint, and localization checks.
- Passed both public documentation validators and both specification validators.
- Passed the focused desktop and mobile production E2E scenarios.
- Passed `git diff --check`.

## Risks

- A stale step snapshot could replace unrelated `on_enter` actions if the
  inferred update is not derived from the current draft.
- The empty-to-non-empty transition must be shared by editor and template paths
  without making mount-time data look like a user edit.
- The tests must distinguish a default from permanent coupling by exercising a
  later explicit disable.
