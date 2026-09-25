---
status: draft
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Workflow Move Preview Requirements

## Overview

Before moving a task, users can see which conversation will receive the step
and which model and settings it is expected to use. The tasks system owns this
capability because it owns workflow recipient selection and step entry behavior.

A profile identity and a running model are different facts. A session launched
with a Luna profile can retain a later Astra model selection when reused.

## Terminology

- **Current session:** The conversation selected by the move's routing context,
  not necessarily a separately viewed or pinned chat tab.
- **Expected configuration:** The selected session's effective settings after
  applicable entry rules, subject to provider acceptance and execution-time state.
- **Preview:** A read-only prediction; it does not reserve a session or execute a move.

## Requirements

### REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001: Session and settings prediction

**Intent:** Expose conversation continuity and model changes before a move.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.1:** Opening a movable step disclosure
  shall show current-session reuse, other-session reuse with its name, new
  session creation, or an idle task with no recipient according to task-specific
  routing. Explicit initial and earlier-step targets, same-profile policies,
  terminal candidates, and the destination's no-session launch gates shall
  follow the existing routing contract.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.2:** An unchanged session shall show its
  effective model. Reuse of a Luna-profile session running Astra shall show
  Astra and disclose the retained override. A new session shall show its
  expected launch model without copying the source session's overrides.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.3:** Applicable conditional settings shall
  produce a before/after model summary without implying a new conversation.
  Set shall retain unnamed fields; keep and no-match shall retain current
  values; restore-original shall use the original effective snapshot. Rules
  inapplicable to a different session or passthrough shall not predict application.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.4:** Additional effective setting changes
  shall appear as a count with accessible details showing field names and
  before/after values. Context reset and source-session retirement shall be
  disclosed. Identical values shall not count as changes. Model changes shall
  not be counted again among additional changes.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.5:** Missing original snapshots, ambiguous
  rules, unavailable capability data, and provider-dependent outcomes shall
  be identified with distinct reason states such as unavailable or skipped,
  rather than one generic message. Unknown values shall not be presented as
  confirmed models. Best-effort application remains best-effort.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.6:** Opening or refreshing the preview
  shall not create sessions, start agents, change settings, write conversation
  warnings, or trigger transitions. Existing authorization shall prevent
  disclosure of another workspace's sessions or sensitive configuration.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.7:** Changing the task, destination, move
  options, known routing/configuration state, or the connection state shall
  invalidate the preview. Late responses shall not overwrite a newer selection.
  Actual moves shall retain execution-time validation, including deferred moves.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8:** While a disclosure remains open,
  updates that do not affect its prediction shall preserve the displayed result
  without another preview request. Examples include task descriptions, read
  cursors, command counts, and activity-summary bookkeeping. Timestamp changes
  that leave session selection unchanged shall also preserve the result.
  Changes that alter candidate eligibility or selection shall still invalidate it.

### REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002: Compact accessible disclosure

**Intent:** Provide useful feedback within the existing step move surface.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.1:** The collapsed desktop preview shall
  use two centered summary lines below actions and capabilities: session outcome,
  then model/settings with an icon-only details button.
  Details shall remain opt-in. Long labels shall truncate with full accessible
  text. Existing Options and capability/progress content shall remain usable.
  The next-step options popover and touch drawer shall show the same footer
  below their Move action, using the destination and draft of that action.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.2:** Hover, keyboard focus, and activation
  shall expose desktop feedback. Phones and coarse pointers shall expose the
  same information and details in the step drawer, without requiring hover.
  Touch actions shall measure at least 44 CSS pixels; content shall remain
  within the viewport with one internal vertical scroll owner and focus return.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.3:** Loading shall show Checking session.
  Failure shall show Preview unavailable with Retry. Preview loading or failure
  alone shall not disable an otherwise allowed move. Known move restrictions
  shall continue to use the existing move gates. No stale prediction shall look current.
- **AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.4:** All labels, details, statuses, and
  accessible names shall support the five shipped locales, including field,
  value, and diagnostic reason codes returned by the preview API. Model and
  session names remain product data. The interface shall distinguish planned
  settings from completed changes without adding a confirmation dialog.

## Existing contracts

- [Session lifecycle](workflow-profile-session-lifecycle.md)
- [Conditional settings](workflow-session-settings.md)
- [Design](../system-design/workflow-move-preview.md)
- [Implementation plan](../../../plans/workflow-move-preview/plan.md)
- [Preview stability repair](../../../plans/workflow-move-preview-stability/plan.md)

## Out of scope

Changing workflow defaults, editing profiles from the preview, automatically
repairing model overrides, routing native subagents, and adding mandatory
confirmation. This package targets the task stepper and its compact disclosure,
and the next-step control above chat, not every board drag, bulk move, or move menu.
