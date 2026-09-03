---
status: current
system: ui
requirements:
  - REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001
---

# Command Panel Task Activity Icons System Design

## Purpose and boundaries

This design adds the sidebar task-state icon to command-panel task results. It does not change task-state or activity ownership.

The command panel continues to use the workspace task-list API. The bounded task status summary remains the live activity source.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001` | [Shared icon presentation](#shared-icon-presentation), [Live data selection](#live-data-selection), [Responsive behavior](#responsive-behavior), [Tests](#tests) |

## Components and responsibilities

- A shared task-state icon component owns the icon type, color, motion, size, test identifiers, and accessible description.
- `TaskItem` continues to supply sidebar state to the shared icon component.
- `CommandPanel` supplies search-result state and live task projections to each result row.
- `TaskResultItem` shows the shared icon before the task title. It keeps the workflow-step badge as separate metadata and derives its content from the same effective task placement used by the live activity resolver.
- `pickFreshestStatusSummary` prevents an older HTTP result from replacing a newer live status summary.

## Shared icon presentation

The implementation extracts the task-state icon rules from `TaskItem`. Both surfaces use one state input and one icon resolver.

The shared input contains these fields:

- task state
- primary-session state
- task-level foreground activity
- pending clarification or permission
- interruption state
- final workflow-step state

The shared component keeps the sidebar icon priority. Generating activity shows the active spinner. Preparing activity shows the muted spinner.

Pending input, interruption, review, completion, and idle states keep their sidebar icons. The command panel does not use `workflow_step_id` to infer activity.

For review tasks, the command panel receives the final step ID for each workflow from the same visible workflow-step list used by the sidebar. The shared component therefore selects the workflow-complete icon only when the effective task step is that workflow's final step.

The command-panel result uses its existing compact leading-icon slot. The icon remains passive and cannot receive selection separately from the result row.

## Live data selection

The workspace task-list response provides the initial task state and bounded status summary. This response includes primary-session and foreground-activity fields.

The command panel also reads the current task projection from the application store. WebSocket task events keep this projection current.

For each result, the UI compares the HTTP summary with the live summary. The summary with the highest revision supplies activity and session data, while an accepted live task projection remains authoritative for its lifecycle and foreground-activity fields. This preserves an explicit live clear (`null`) instead of falling back to a stale search response. Legacy live projections without an update timestamp are accepted as current WebSocket-backed readings so their cleared fields are honored.

A newer task update supplies lifecycle state and interruption state. If no live task exists, the result uses the HTTP fields.

The resolver also selects the effective workflow and workflow-step IDs from that same accepted live projection. `TaskResultItem` uses the effective workflow-step ID to read the step name and color from its existing step map. This keeps the badge current after a task move without a second freshness decision. If the live projection is absent or rejected as stale, the HTTP placement remains authoritative.

This flow does not subscribe to session-detail streams. It follows the bounded task-status delivery contract and its revision rules.

## Responsive behavior

The command panel keeps the same result-row composition on desktop and phone layouts. This change replaces one leading icon only.

The command panel remains the scroll owner. The task title keeps the flexible width, and metadata can remain hidden on a phone.

The nearest mobile example is the existing command-panel task row in `mobile-command-palette-scopes.spec.ts`. No overlay or touch target changes.

## Failure and recovery

If live task data is absent, the HTTP result supplies the icon state. If all activity fields are absent, the shared resolver shows its idle icon.

An older status summary cannot replace a newer live summary. A malformed optional status summary cannot remove the task result or navigation action.

## Tests

Component tests cover active, preparing, idle, review, completed, and missing-live-data states. They also cover a live update after search results render.

The desktop command-panel test proves that a running task has the sidebar spinner. The test also proves that an idle task has no spinner and that a newer live workflow placement replaces the stale search-result badge.

A component regression test covers both an accepted newer live step and a stale live step, so the badge follows the same timestamp guard as the activity icon.

The phone command-panel test proves the same icon state and title width. It also proves that the row causes no horizontal overflow.

## Related decisions

- [Separate task activity from summary freshness](../../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md)
- [Separate task-summary delivery from session streams](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)
