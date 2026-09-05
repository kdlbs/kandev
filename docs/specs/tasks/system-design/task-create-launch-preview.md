---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
---

# Task Create Launch Preview System Design

## Purpose and boundaries

The task system owns immediate-launch routing and workflow-step prompt
composition. The shared task creation dialog presents a pre-creation projection
of those rules. The projection follows the action currently available in the
form: an empty description exposes **Start Plan Mode**, while a nonempty
description exposes immediate agent-start actions.

This design adds no endpoint, persisted state, or runtime prompt behavior. It
uses the workflow data that the dialog already receives from boot state or the
workflow-step API.

## Requirement mapping

| Requirement                                | Design sections                                                                                                         |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001` | [Launch-step projection](#launch-step-projection), [Workflow selector](#workflow-selector)                              |
| `REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002` | [Prompt composition](#prompt-composition), [Prompt editor](#prompt-editor), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- A pure task-create helper selects the launch destination and composes the
  step-prompt preview.
- `useWorkflowStepsEffect` retains each fallback-fetched step prompt with its
  actions, position, and workflow identity.
- Workflow-step WebSocket handlers keep loaded workflow snapshots synchronized
  for step create, update, and delete events.
- `buildDialogFormBodyProps` selects steps for the effective workflow. It passes
  one launch-preview model to the shared create form.
- `WorkflowSelectorRow` shows the launch destination beside the selected
  workflow name.
- `TaskFormInputs` owns the local preview toggle because it already owns the
  current task prompt and editor state.
- A focused preview control renders the icon, tooltip, accessible state, and
  read-only preview surface.

## Data and contracts

The launch-preview model contains the destination step ID, step name, and step
prompt. It is derived data and is not stored in `DialogFormState`.

The resolver accepts steps from one effective workflow. A step supplies these
existing fields:

- `id`
- `title`
- `position`
- `events.on_enter`
- `is_start_step`
- `prompt`
- `workflowId` when the step came from a fallback fetch

The dialog uses matching fetched steps when they exist. A successful empty fetch
is authoritative and produces no destination. If the fetch is unavailable, or
contains only steps from another workflow, the dialog uses the selected workflow
snapshot. A fetched step from another workflow is never a candidate. When the
effective workflow is the visible context workflow, the dialog uses its live
snapshot; workflow-step WebSocket events keep that snapshot current after edits
made elsewhere. A different effective workflow is fetched by the existing
fallback path.

## Launch-step projection

For a nonempty description, the frontend resolver mirrors
`workflow.Service.ResolveAutoStartStep`:

1. Sort the effective workflow steps by `position`.
2. Select the first step whose `on_enter` actions contain
   `auto_start_agent`.
3. If none exists, select the first step with `is_start_step`.
4. If none exists, select the first positional step.

For an empty description, the resolver selects the first positional step. This
matches the **Start Plan Mode** path, which does not start an agent immediately.
The auto-start projection describes the **Start task** and **Start task in plan
mode** actions when a description is present. It does not describe **Create
without starting agent**, which uses the configured start step.

## Prompt composition

The preview helper mirrors the step-template part of
`orchestrator.Service.buildWorkflowPromptWithTrustedContext`.

If the step prompt contains `{{task_prompt}}`, the helper replaces its first
occurrence with the current editor value. The backend also replaces only the
first occurrence. If the token is absent, the helper returns the step prompt
without adding the editor value.

The helper does not resolve `{task_id}` because no task exists. It also leaves
`@name` references unchanged. Saved-prompt expansion remains server-owned under
[ADR-2026-09-01](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).

The preview excludes workflow-level instructions and runtime-only system
context. Its purpose is to show how the launch step applies its own template.

## Workflow selector

The selected workflow trigger renders only the workflow name and existing
chevron. The launch destination renders as a muted sibling outside the trigger,
aligned to its right, with the localized **Start step: {{step}}** label.
Truncation keeps the selector row within its available width.

The existing selector visibility rules remain unchanged. A single implicit
workflow can still omit the selector when it has no override information.

## Prompt editor

The preview button appears only in create mode when the launch destination has
a nonempty prompt. It follows **Enhance prompt with AI** in the toolbar.

The editor keeps its draft in the existing local description state. Preview
mode replaces the editable textarea with a read-only, wrapping text surface.
The preview surface uses the same height boundary and overflow ownership as the
textarea.

The button uses `aria-pressed` and localized labels for preview and edit modes.
The inactive preview label identifies the workflow step prompt and resolved
step, for example **Preview launch prompt with workflow step prompt: In
Progress**. If the workflow changes to a destination without a prompt, the
component closes preview mode and shows the unchanged editor value.

## Responsive behavior

Desktop and mobile use the same launch resolver, prompt value, and toggle state.
The existing task creation dialog remains the entry point and scroll owner.

On fine pointers, the preview icon keeps the compact composer density. On coarse
pointers, its active hit area is at least 44 CSS pixels. The read-only text wraps
inside the composer and owns only the same bounded overflow as the textarea.

The nearest mobile exemplar is the current Kanban FAB to task-create dialog
flow. This change adds no drawer, route, fixed control, or safe-area boundary.

## Failure and recovery

If workflow steps are unavailable, the dialog keeps the selected workflow
snapshot as a temporary fallback and the remaining form stays usable. A
successful empty response omits the destination and preview controls rather
than retaining stale snapshot steps. Workflow identity filtering prevents
another workflow's fetched steps from becoming candidates.

If steps arrive after the dialog opens, the derived model updates from those
steps. A workflow change removes any prior preview model before new steps can
replace it. Step create, update, and delete events update loaded workflow
snapshots, so the visible workflow does not require a component-level refresh
request to reflect edits made elsewhere.

## Localization and public documentation

All new labels use the task namespace in every supported locale. The public
task creation guide explains the launch label, preview boundary, and unresolved
server-owned values.

## Verification

- Unit tests cover action-sensitive launch routing, stale workflow filtering,
  workflow-snapshot step-event synchronization, and prompt composition.
- Component tests cover selector text, toggle state, draft preservation, and
  the no-prompt fallback.
- Desktop Playwright covers workflow switching and composed preview content.
- Mobile Playwright covers the same user value, the 44-pixel hit area, viewport
  containment, and horizontal overflow.

## Related decisions

- [Keep Saved-Prompt Expansion Server-Owned](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)
