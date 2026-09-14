---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STEP-PROGRESS-001
---

# Workflow step progress design

## Purpose and boundaries

Present existing task and session lifecycle data in workflow navigation.
The backend remains authoritative for movement, session ownership, and startup.
No new API, event, database field, or architecture boundary is required.

This extends [compact navigation](../../ui/system-design/compact-workflow-step-navigation.md) and preserves its controls and responsive surfaces.
Use the existing [runtime publication ordering](runtime-state-publication-order.md) when consuming snapshots.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-TASKS-WORKFLOW-STEP-PROGRESS-001.1, .5, .6, .8, .9 | State derivation |
| AC-TASKS-WORKFLOW-STEP-PROGRESS-001.2, .3, .4 | Marker and disclosure |
| AC-TASKS-WORKFLOW-STEP-PROGRESS-001.7 | Failure and recovery |
| AC-TASKS-WORKFLOW-STEP-PROGRESS-001.10, .11 | Responsive and accessible behavior |

## Components and responsibilities

- `useWorkflowStepMove` retains move-request identity and presentation-token protection.
- Add `useWorkflowStepProgress` beside that hook, with a small pure derivation helper if needed for tests.
- `WorkflowStepper` and the preview step indicator consume the same derived state.
- `StepCircleIndicator` renders progress within its existing marker footprint.
- `StepHoverContent` receives status independently of `canMove`, so details survive when the destination becomes current.
- `MinimalWorkflowStepper` and `StepDisclosureRow` receive the same state for compact and touch presentations.
- `StepChoices` in `task-management-drawer.tsx` exposes current-step details inside the existing phone Move to drawer.

## State derivation

Use the existing task projection and `taskSessionsByTask`/session selectors.
Use the task's primary session identity, not the session tab currently selected by the user.
Reject a session from another task or a known different workflow step.
Prefer the existing merged session state; use task primary-session projection when details are unloaded.
Do not introduce independent snapshot ordering or copy lifecycle state into a second store.

| Evidence | Marker | Disclosure status |
| --- | --- | --- |
| Latest local move request remains unsettled | Destination spinner | Moving to this step |
| Task is SCHEDULING | Current-step spinner | Preparing agent |
| Relevant primary session is STARTING | Current-step spinner | Starting agent |
| Primary session has `cancellation_pending` | Current-step marker settles | Stopping agent |
| Relevant primary session is RUNNING | Normal current marker | Agent running |
| Relevant primary session is WAITING_FOR_INPUT | Normal current marker | Waiting for input |
| Relevant primary session is terminal | Normal current marker | Completed, stopped, or failed, matching state |
| CREATED alone, no scheduling or request | Normal marker | Not started |
| No reliable lifecycle evidence | Normal marker | Existing current-step content only |

A newer known cancellation or terminal session result overrides stale preparation evidence for that session.
Task/session precedence must follow the existing merged projections and their ownership, rather than raw arrival order.
Local move state controls only its request target. Server lifecycle state controls the current step.
If targets differ during an overlapping move, each marker describes its own evidence; do not attribute the old session to the new target.
On successful response, consume available response/store state and clear request ownership as today.
Do not wait for a new session ID, first message, or guessed delay.
The lifecycle-derived spinner remains independent of request settlement and survives remounts when server state supports it.

Show the actual model only when existing runtime configuration for the relevant session supplies it.
Otherwise show the available agent/profile display name. Do not infer an applied model from the step's configured profile.
Do not show ACP initialization, prompt delivery, or other detailed phases that the existing projections cannot prove.

## Marker and disclosure

The current `StepCircleIndicator` uses an 8px in-flow dot and an absolute 14px current-step ring.
Preserve the 8px layout footprint. Keep current-step decoration within the existing 14px visual bounds.
For a non-current move target, keep the spinner within its existing 8px visual bounds.
Use a circular partial stroke and existing colors; do not insert a default 16px icon.
Keep label coordinates, connectors, row height, and compact-mode threshold unchanged.

Add the status block inside the existing hover card, after the current-step label or eligible move/options controls and before capability icons.
The status block renders even when `canMove` becomes false.
Do not add inline status outside the disclosure or another tooltip.
Preserve archived presentation and manual movement policy.
The preview indicator uses the same marker and detail rules.

## Responsive and accessible behavior

Desktop entry remains the existing step hover card. Keyboard focus opens that same surface; Escape and focus return retain existing behavior.
Make its trigger keyboard reachable without changing its layout dimensions or introducing a second overlay.
The marker has accessible pending text; visual details remain inside the disclosure.
Use polite status announcements in an open disclosure without repeated announcements on every render.
Under reduced motion, remove rotation but retain the partial-ring pending shape and accessible status.

Tablet entry remains the compact trigger and existing touch Drawer.
Phone entry remains Task actions > Move to in `TaskManagementDrawer`; no top-bar stepper is added.
Place the current-step status beneath its label inside that drawer, outside any disabled button's inaccessible content if necessary.
The existing drawer is the shipped exemplar: temporary inspection and step choice do not need another route or sheet.
Retain its scroll owner, safe-area clearance, dismissal, and at least 44px touch controls.
Fine-pointer controls retain their compact sizing.

## Failure and recovery

Use existing move error reporting. Clear local request state only for its owning request and presentation.
A failed primary session gets a concise translated status in the existing disclosure.
Do not expose raw backend errors or add retry mutations.
Navigation and superseding responses retain `usePresentationToken` protection.
Reload uses existing hydration; reconnect uses existing reconciliation. Add no polling or persisted progress latch.

## Verification

Unit tests cover state/ownership precedence, no-auto-start, missing data, cancellation, terminal state, and stale responses.
Component tests cover disclosure continuity after a step becomes current, running details, translations, and accessible reduced-motion state.
Browser tests compare marker, label, connector, and row bounds before and during progress.
Use controlled response/event gates, not fixed sleeps, for delayed startup and error cases.
Cover full stepper, compact disclosure, preview, tablet Drawer, and the phone Move to path.

## Documentation

Public documentation remains unchanged during design. Implementation adds a short explanation to the existing workflow navigation documentation if applicable.
No new public page or screenshot is required solely for this marker change.
