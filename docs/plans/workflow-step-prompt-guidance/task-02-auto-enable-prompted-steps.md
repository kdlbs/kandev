---
id: "02-auto-enable-prompted-steps"
title: "Auto-enable prompted steps"
status: done
wave: 1
depends_on:
  - "01-show-step-prompt-guidance"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 02: Auto-enable Prompted Steps

## Summary

Make the workflow editor enable Auto-start agent when a user first creates a
non-empty step prompt. Remove the recovery warning while keeping automatic
start independently editable after the default is applied.

## In scope

- Add and test the pure empty-to-non-empty prompt transition helper.
- Route editor and prompt-template changes through one orchestration callback.
- Remove the missing-auto-start warning and its locale key.
- Update desktop and mobile Playwright coverage.
- Align the public workflow reference with the revised behavior.

## Out of scope

- Change backend launch or prompt composition semantics.
- Migrate existing non-empty prompts without automatic start.
- Disable automatic start when a prompt is cleared.
- Prevent users from explicitly disabling automatic start.

## Acceptance

- Typing, pasting, or applying a template to an empty prompt enables
  `auto_start_agent` once without disturbing other entry actions.
- Clearing a prompt retains automatic start, and editing a still-non-empty
  prompt does not reverse an explicit user disable.
- The warning is absent, prompt replacement guidance remains, and the behavior
  is verified on desktop and mobile.

## Verification

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web test -- --run components/settings/workflow-pipeline-editor-helpers.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
pnpm --dir apps/web e2e:run --host tests/workflow/workflow-settings.spec.ts -- --grep "enables automatic start for a new step prompt"
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts -- --grep "enables automatic start for a prompt template"
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/settings/workflow-pipeline-editor-helpers.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-helpers.test.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-step-prompt-section.tsx`
- `apps/web/src/locales/*/workflows.json`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `docs/public/workflow-tips.md`

## Dependencies

Task 01 established the current guidance and regression scenarios that this
work replaces.

## Risks

- An inferred action update must preserve every unrelated `on_enter` action.
- Mounting an existing prompt must not be mistaken for user authoring.
- The first prompt update and automatic-start action must enter one saved
  workflow draft.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005`
- `docs/specs/tasks/system-design/workflow-step-agent-start-ownership.md`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- Existing workflow settings desktop and mobile Playwright scenarios.

## Results

- Added a shared prompt-change path for editor input and prompt templates.
- Added a pure helper that enables `auto_start_agent` on an empty-to-non-empty
  prompt transition. The helper preserves other entry actions.
- Published the prompt and automatic-start action in one draft update.
- Removed the warning component and its translations.
- Updated the public workflow reference.
- Added helper unit coverage and desktop/mobile production E2E coverage.
- Passed every verification command in this work order.
