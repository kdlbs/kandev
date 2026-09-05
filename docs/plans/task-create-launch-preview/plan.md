---
created: 2026-09-05
status: complete
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
legacy_specs: []
---

# Implementation Plan: Task Create Launch Preview

## Overview

Add a shared frontend projection of the backend immediate-launch rules. Use that
projection to show the destination step and to preview its composed step prompt.
The projection must follow the available create action: empty descriptions use
the plan-mode first step, while nonempty descriptions use immediate agent-start
routing.
The pure projection lands first, the UI follows, and responsive E2E tests prove
the complete flow.

## Scope

### In scope

- Show the immediate launch destination beside the selected workflow.
- Preview a nonempty destination step prompt with the current task prompt.
- Preserve the task prompt while the preview is visible.
- Localize the new controls in all supported locales.
- Update the public task creation guide.
- Prove desktop and mobile behavior.

### Out of scope

- Changing launch routing or backend prompt composition.
- Selecting a workflow destination step.
- Expanding task IDs, saved prompts, or runtime-only context before creation.
- Adding the preview outside the primary task creation dialog.

## Technical approach

### Launch projection

Add `task-create-dialog-launch-preview.ts` with pure launch-step and prompt
composition helpers. The step resolver uses the first positional step for the
empty-description plan-mode action, mirrors `ResolveAutoStartStep` for
nonempty descriptions, and filters fetched steps by the effective workflow ID.

Extend `StepType` and `useWorkflowStepsEffect` so fetched step data retains
`prompt` and refreshes even for the visible context workflow. Build one derived
launch-preview model in the dialog prop assembly. Do not store derived preview
data in form state.

### Dialog presentation

Extend `WorkflowSelectorRow` with the selected launch destination. Render the
step after the workflow name with muted, truncating text.

Add a focused prompt-preview control to the create composer. Place its icon
after **Enhance prompt with AI**. The control toggles the existing editor between
the unchanged task prompt and a read-only composed preview.

Keep the task dialog as the outer scroll owner. Give the new icon a 44-pixel
coarse-pointer hit area and retain compact desktop density.

### Localization and documentation

Add task-namespace labels for preview mode, edit mode, and the preview surface.
Update all supported locale catalogs, using `i18n:zh-hant` for Traditional
Chinese catalogs.

Update `docs/public/tasks-and-workflows.md`. Explain that the displayed step is
the immediate launch destination and that server-owned values remain unresolved
until task creation.

## Tests

- `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2` and `.5` map to resolver and
  prop-builder tests for plan-mode, auto-start, configured-start, and positional
  fallbacks.
- `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.3` maps to stale fetched-step and
  workflow-switch tests.
- `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.2` through `.5` map to composition
  and component toggle tests.
- `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.6` maps to accessible-state and
  responsive hit-area tests.

## E2E tests

- Extend `apps/web/e2e/tests/task/create-task.spec.ts` with a custom workflow.
  Its configured start step differs from its first auto-start step. Assert the
  displayed launch destination and composed preview.
- Add
  `apps/web/e2e/tests/task/mobile-create-task-launch-preview.spec.ts`. Assert the
  same preview flow, a 44-pixel touch target, viewport containment, and no
  document-level horizontal overflow in `mobile-chrome`.

## Work orders

- [x] [Task 01: Resolve launch preview data](task-01-resolve-launch-preview-data.md)
- [x] [Task 02: Present launch preview controls](task-02-present-launch-preview-controls.md)
- [x] [Task 03: Prove responsive launch previews](task-03-prove-responsive-launch-previews.md)

## Verification results

- Targeted launch-preview, effects, prop-builder, and selector tests passed
  (67 tests after review fixup coverage).
- Task-create component tests passed (43 tests); focused ESLint, typecheck,
  i18n checks, pseudo-catalog sync, and public-doc validators passed.
- Desktop and mobile focused E2E tests passed (1 each) in host mode.
- `git diff --check` passed.

## Risks

- The frontend resolver can drift from backend launch routing. Focused fallback
  tests must encode the backend precedence.
- A workflow switch can briefly expose the snapshot while a fresh fetch is in
  flight. Every fetched step must match the current effective workflow before
  use, and a successful empty fetch is authoritative.
- The preview cannot show server-owned task IDs or saved-prompt expansions.
  Public copy must state this boundary without implying full runtime expansion.
- Long workflow and step names can crowd the selector on phones. Truncation and
  the mobile browser test must protect the dialog width.
