---
status: current
system: tasks
requirements:
  - REQ-TASKS-CREATE-WORKFLOW-STEPS-001
---

# Task creation workflow step previews

## Purpose and boundaries

The task creation selector owns the loading of its option previews. It reads
step definitions through the existing `listWorkflowSteps` API. It does not
require task-bearing Kanban snapshots to populate those previews.

The [launch preview design](task-create-launch-preview.md) remains authoritative
for the destination label and prompt composition. This repair leaves
`useWorkflowStepsEffect`, `fetchedSteps`, and launch resolution unchanged.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-CREATE-WORKFLOW-STEPS-001` | Loading lifecycle; Rendering and recovery; Responsive behavior |

## Components and contracts

Existing components are `WorkflowSelectorRow`, `WorkflowSection`, and
`DialogFormBody`. Add an optional `previewWorkspaceId` prop to the selector.
Thread the task dialog's existing workspace ID through `WorkflowSection`.
Only task creation supplies this prop. The automation caller in
`components/automations/config-section.tsx` retains snapshot-backed rendering.

Add `hooks/use-workflow-option-previews.ts` as a local React hook. Its inputs
are the preview workspace ID, selector open state, and visible workflow IDs.
Its output maps workflow IDs to loading, success, or error state and exposes
retry for one workflow. Successful results include an ordered step array;
`success` with an empty array is distinct from loading and failure.

Use `listWorkflowSteps(workflowId, { cache: "no-store" })` from
`lib/api/domains/workflow-api.ts`. It calls the existing authorized endpoint
`/api/v1/workflows/:id/workflow/steps`. Map `name` to `title` and retain `id`,
`position`, `color`, `is_start_step`, and `agent_profile_id` for `InlineSteps`.
Do not replace this with `useWorkflowSteps`: that hook drops display metadata
and does not distinguish failure from a successful empty response.

## Loading lifecycle

1. While closed, do not fetch preview data.
2. On opening, request steps for every visible workflow in the supplied scope.
   Do not use snapshot presence as a completeness signal. Publish each result
   independently so a slow or failed workflow cannot block successful rows.
3. Keep an open-cycle generation and per-workflow request identity. Stable,
   sorted workflow IDs define membership; new array instances do not refetch.
4. On close, unmount, workspace change, or membership change, invalidate old
   requests. Clear or mask old state synchronously by scope before effects run.
   No previous workspace's result may flash during the first new render.
5. Retry only the failed row. Ignore earlier attempts that finish after retry.
   A retry must neither call `onWorkflowChange` nor close the selector.
6. Reopening starts a fresh cycle. Successful empty responses replace prior
   data. Keep requests and results local; do not write `kanbanMulti`, hydrate
   board tasks, or change workspace selection.

Use cancellation guards even if the request transport supports abort. Do not
add polling or a module-level cache. Concurrent requests must settle separately;
ordinary rerenders must not start repeated batches. Live step events within an
already open picker are outside this repair; reopening refreshes definitions.

## Rendering and recovery

In task-create mode, the hook is the sole source for option previews. Display
localized loading, empty, and error text below the workflow name. Use a
noninteractive row wrapper containing the selection button and a sibling retry
button. Never place a retry button inside the selection button.

Keep successful options selectable. Preview loading and failure do not add a
new task-submission gate. Preserve the checkmark, name, profile badge, and
existing clear option. Other selector consumers keep their existing behavior
when `previewWorkspaceId` is absent.

Use a localized status announcement for asynchronous feedback. Error text must
not contain raw server errors. Add new copy in all six supported languages and
use the Traditional Chinese generation command for zh-tw and zh-hk.

## Responsive behavior

Keep the existing click-open Popover and task-create dialog entry point. This
is a short choice list, not a new navigation surface. The mobile language guide
permits a contained, touch-usable click picker. The nearest shipped exemplar
is the mobile task-create selector exercised by
`e2e/tests/task/mobile-create-task-launch-preview.spec.ts`.

Constrain the popover to available viewport width and height. Its option list
owns vertical scrolling. Wrap step groups in positional order; do not clip them
or introduce a second horizontal scroll area. Preserve focus return on dismissal.
The primary action is selecting a workflow; retry is secondary. Keep phone and
coarse-pointer targets at least 44 CSS pixels. Ordinary desktop retry buttons
use standard control sizing. Do not change safe-area handling of the parent dialog.

## Persistence, security, and diagnostics

No schema, persisted preference, API authorization, or runtime flag changes are
required. The caller passes only workspace-filtered, visible workflow IDs.
Existing API authorization remains authoritative. Local request guards prevent
stale display; they are not a substitute for server authorization.

No new metrics or logs are needed. Tests observe requests, state, and rendered
results. No new ADR is required: this is a local consumer correction using an
existing API, with no new cross-system ownership boundary.

## Verification

Hook tests cover partial caches, independent failures, retry, authoritative
empty results, duplicate prevention, and lifecycle isolation. Component tests
cover labels, selection, metadata, and the unchanged automation fallback.
Desktop and phone E2E tests enter task creation directly from a task page and
prove all workflow previews without first visiting the board.

## Implementation plans

- [Workflow step preview repair](../../../plans/task-create-workflow-step-previews/plan.md)
