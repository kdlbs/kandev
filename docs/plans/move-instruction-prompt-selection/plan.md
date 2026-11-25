---
created: 2026-11-23
status: implemented
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
legacy_specs:
  - ../../specs/workflow-step-move-overrides/spec.md
---

# Implementation plan for saved prompts in move instructions

The [saved-prompt requirement](../../specs/tasks/requirements/saved-prompt-delivery.md)
owns this extension. The [system design](../../specs/tasks/system-design/saved-prompt-delivery.md#one-time-move-instructions)
defines the editor and delivery changes. The task system owns delivery because
the references travel with one workflow transition to its agent session.

## Scope and approach

Reuse the task prompt reference editor in the shared move field. Keep the
existing options payload and server-owned prompt resolver. Float desktop suggestions above the field. Keep phone results inline. Preserve direct moves, reset, skip, preview, and
existing submission controls. Keep workflow settings and prompt management
outside the work order. The selected prompt appears as an editable reference
chip next to surrounding plain text.

One work order covers the whole result. Start with focused editor and backend
regressions. Connect the shared field, then verify both entry points and update
the one-time move section in the public Workflow Tips page.

## ASCII UI preview

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

The [work order](task-01-select-and-deliver-saved-prompts.md#verification)
lists exact commands and tests. Desktop Chromium covers the next-step form
and the task context menu. Mobile Chrome covers the existing touch paths.
Both use the rebuilt production app and a saved prompt with a deterministic
mock-agent response.

## Work orders

- [x] [Task 01. Select and deliver saved prompts](task-01-select-and-deliver-saved-prompts.md)

## Risks

- Overlay focus handlers can consume Escape before the editor sees it.
  Check dismissal and focus return in the anchored form and move drawer.
- Fresh launch and queued handoff use separate prompt preparation paths.
  Verify the persisted and dispatched definitions match in each path.

## Verification results

The [work order results](task-01-select-and-deliver-saved-prompts.md#results)
record the verification runs and acceptance evidence.
