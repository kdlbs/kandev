---
id: TASK-MOVE-INSTRUCTION-PROMPT-SELECTION-01
title: Select and deliver saved prompts in one-time move instructions
status: done
wave: 1
depends_on: []
plan: ./plan.md
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.15
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.16
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.17
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.18
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
---

# Task 01. Select and deliver saved prompts

## Summary

Add saved-prompt suggestions and chips to the shared one-time instructions
field. Deliver referenced definitions through the workflow move's existing
session and queue paths.

## In scope

- Share the editor across the next-step button and task Move options.
  Cover the shared stepper field where it inherits the editor.
- Prepare the full visible instructions before saved-prompt resolution on
  fresh launches. Reuse drain-time preparation for queued move instructions.
- Localize the list label if a new label is required. Update the one-time
  move section of [Workflow Tips](../../public/workflow-tips.md).

## Out of scope

- Prompt management, move eligibility, and workflow configuration editing.
- Office run prompt composition and terminal expansion.

## Acceptance

1. The user can type or select a highlighted reference in both move paths.
   Suggestion keyboard events keep the move draft open.
2. Structured delivery expands the current saved definition once. Plain text
   and passthrough keep their existing behavior.
3. A failed move preserves the draft. A successful move applies the
   instructions only to that entry.

## ASCII UI preview

[Full preview and structural notes](plan.md#ascii-ui-preview)

### UI-01: Desktop move options with matches

The composer next-step action opens its anchored form. The sidebar Move menu
shows its form in the destination submenu. Suggestions float above the field
without changing the form height.

```text
Move to Review
[ ] Reset agent context
  +-- Saved prompts -------------------+
  | > @review-checklist                 |
  |   @review-summary                   |
  +-------------------------------------+
[ Check @rev|                           ]
                              [Move]
```

### UI-02: Phone move drawer with a selected reference

Open the next-step options or use **Task actions > Move**. The drawer header
stays visible. Its body scrolls. Matches appear below the editor as touch rows
in the same drawer. The brackets around the selected reference represent a chip.

```text
+---------------------------+
| Move to Review            | fixed header
|---------------------------|
| [ ] Reset agent context   |
| [ ] Skip the step prompt  |
| One-time instructions     |
| Check [@review-checklist] |
|                           |
| [Cancel]           [Move] |
+---------------------------+
```

The result list appears only during a query. A selected reference stays
highlighted after the list closes. Labels and spacing are illustrative.
The interaction order and single phone scroll region are required.
Check both views for AC-TASKS-SAVED-PROMPT-DELIVERY-001.15 and .16.

## Verification

- Add cases to `task-prompt-reference-editor.test.tsx` for known typed names,
  selection within adjacent text, and accessible naming. Extend the move
  form and shortcut tests for selected text serialization, failure retention,
  and Escape, Enter, Tab, and Ctrl/Cmd+Enter routing. This covers .15 and .16.
  Update textarea-specific assertions to use the editable textbox and its
  serialized move payload.
- Add orchestrator cases named `TestMoveInstructionSavedPrompt*`. Cover
  reused, fresh, deferred, and queued handoff paths. Check skip-step,
  unknown name, passthrough, and a subsequent move without overrides.
  Assert one visible instructions block and one server expansion where
  applicable. Compare persisted text with agent dispatch. This covers .17
  and .18.
- Extend the desktop workflow override spec with composer and right-click
  selection tests. Extend the mobile spec with next-step drawer and task
  actions selection tests. Seed a saved definition with a unique mock-agent
  response. Assert the highlight, the submitted reference, and the response.
  On desktop, open the full suggestion list. Check that it sits above the
  field and that the move form keeps the same height.
  On phone, check viewport containment, the scroll region, 44 px selection
  targets, and keyboard visibility. This covers .15 through .18.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test components/task-prompt-reference-editor.test.tsx components/task/workflow-move-options-form.test.tsx components/task/workflow-move-shortcut.test.tsx components/task/workflow-move-proceed-button.test.tsx components/task/workflow-stepper-keyboard.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx hooks/use-inline-mention.test.ts)
(cd apps && pnpm --filter @kandev/web typecheck)
(cd apps && pnpm --filter @kandev/web i18n:check)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'Test.*(MoveInstructionSavedPrompt|WorkflowEntrySavedPrompt|ExecuteQueuedWorkflowPrompt_MissingExecutionKeepsDrainExpansion|ProcessOnEnter|BuildWorkflowPrompt|ApplyWorkflowAndPlanMode|LaunchSession_ExpandsSavedPrompts|StartCreatedSession_ExpandsSavedPrompts)' -count=1)
(cd apps/web && e2e/scripts/run-e2e.sh --host --shards 1 --project chromium -- tests/workflow/workflow-step-move-overrides.spec.ts tests/task/command-palette-task-actions.spec.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --shards 1 --project mobile-chrome -- tests/workflow/mobile-workflow-step-move-overrides.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/task/workflow-move-options.tsx`
- `apps/web/components/task-prompt-reference-editor.tsx`
- `apps/web/components/task/chat/mention-menu.tsx`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/move_instruction_saved_prompt_test.go`
- `apps/web/e2e/tests/workflow/{mobile-,}workflow-step-move-overrides.spec.ts`
- `docs/public/workflow-tips.md`

## Dependencies

None.

## Risks

See [plan risks](plan.md#risks).

## Parallelism

`sequential`

## Inputs

- [Saved Prompt Delivery](../../specs/tasks/requirements/saved-prompt-delivery.md)
- [Move instruction design](../../specs/tasks/system-design/saved-prompt-delivery.md#one-time-move-instructions)
- [Saved-prompt authority decision](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)

## Results

- The frozen dependency install passed. Vitest passed 119 tests in eight
  files. Type checking, localization checks, and scoped frontend lint passed.
- The targeted orchestrator tests passed. They verified fresh, deferred, and
  queued delivery, task-title composition, and literal passthrough input.
  The queued handoff test uses SQLite for enqueue and drain.
- Desktop Chromium passed 11 Playwright tests. Mobile Chrome passed six.
  Both runs checked selection and delivery through the two requested entry
  points. The phone run also checked drawer containment and touch targets.
- The local Playwright fixture used a clean environment, a loopback bind, and
  the host's bundled Chromium libraries. The last phone run rebuilt the
  Vite bundle after the confirmation Escape regression fix.
- Specification catalog validation, specification lint, PR documentation
  coverage, public documentation validation, and diff checks passed.

After the desktop placement correction, 48 focused frontend tests passed.
Two desktop browser tests verified dropdown placement and stable form height.
Two phone browser tests verified selection in the drawer.

After review fixes, 49 focused frontend tests, four desktop browser tests,
two phone browser tests, and the targeted orchestrator suite passed.
