---
status: current
system: ui
requirements:
  - REQ-UI-LIST-STEP-GROUPING-001
---

# Task List Workflow Step Grouping System Design

## Purpose and boundaries

Replace List runtime-state sections with workflow-step sections. The owning
contract is the UI presentation preference; task placement and runtime status
remain separate task-system data. No workflow mutation is part of grouping.

Section projection lives in `apps/web/lib/tasks/task-list-sections.ts` and
uses actual workflow and step IDs. Both desktop
`TasksListControls` and phone `MobileTasksListOptions` resolve labels through
`GROUP_OPTION_LABEL_KEYS` in `lib/tasks/tasks-list-options.ts`.

## Requirement mapping

| Criteria under `REQ-UI-LIST-STEP-GROUPING-001` | Design section |
| --- | --- |
| .1, .5 | Preference compatibility and copy |
| .2, .3, .6 | Section projection |
| .4 | Metadata and recovery |
| .7 | Desktop and phone surfaces |

## Preference compatibility and copy

Use `workflow_step` as the canonical built-in option and default for
`tasks_list_group` and the List `group` URL parameter. Treat `state` as a legacy
input alias, not an additional visible option. Frontend `parseTasksListGroup`
and backend `NormalizeTasksListGroup` normalize the alias before producing a
canonical value; other built-in values remain valid. Backend
`applyTasksListPreferences` must validate legacy inputs and normalize before
assignment so old clients do not receive a validation error.

The SQLite settings JSON already uses normalization during read/write. Change
the normalizer and defaults without a schema migration or a bulk settings
rewrite. Reading legacy settings is side-effect free; a subsequent normal
settings write persists the canonical value. Keep settings revision ordering
and omitted-field patch semantics intact. Do not rewrite the URL on mount solely
to replace the alias; later user selections produce canonical links.

`lib/ssr/user-settings.ts` and the live SPA `TasksDataRoute` in
`src/spa-routes.tsx` already call the shared parser. Cover these entry paths;
editing only the legacy `app/tasks/page.tsx` does not update the Vite route.
Plugin facet values retain their existing selection path and ownership.

Introduce a `tasks:groupByWorkflowStep` label and neutral step metadata labels,
and update the phone explanation currently referenced as
`kanban:groupTasksIntoSectionsByState`. Add en, pt-pt, zh-cn, and ja values;
generate zh-hk/zh-tw with `i18n:zh-hant` and pseudo with `i18n:pseudo`. Resolve
copy during rendering or helper invocation, never at module scope. Remove an
obsolete key only after confirming every consumer and locale is migrated.

## Metadata and recovery

The paginated `Task` DTO contains `workflow_id` and `workflow_step_id`, but does
not contain a step name or position. The active board's `kanban.steps` alone
cannot name tasks from other workflows or serve a cold direct `/tasks` visit.

Use step-only reads through the established `listWorkflowSteps` API in a
domain hook. Reuse `useWorkflowOptionPreviews` for its bounded request set,
workspace scope, loading/error states, request generations, and retry behavior.
Enable it only for workflow-step grouping and only for authorized workflows
represented by the current result page. Consume step metadata from successful
responses; do not fetch full task snapshots merely to obtain names. Any cached
step metadata reused from the board must match the active workspace and
workflow, and must not override task placement from the accepted list result.

Use the current workspace's `workflows.items` store collection for authorized
workflow IDs, names, and ordering. Project its fields into `TaskListWorkflow`,
the minimal List metadata shape. Do not retain initial route workflows in local
state: workflow creation and workspace selection update the store while the
List can remain mounted. Both step reads and section labels consume this same
current projection.

Keep step metadata and the pure section projection outside row markup.
`TasksPageClient` owns the hook, passing metadata through `TasksPageContent` to
`TasksListView`. Revalidate step metadata during explicit list refresh and after
relevant workflow-step changes using the existing subscription infrastructure.
Fence late responses on workspace/membership changes and unmount.
`useTaskListWorkflowSteps` listens directly for step created, updated, and
deleted notifications through `useWebSocketClient`, scoped to the represented
workflows. It uses an explicit refresh key on `useWorkflowOptionPreviews`; no
parallel global cache is introduced. The list's current task refresh cadence remains unchanged.

Loading, read failure, or a removed step must never fall back to runtime-state
grouping. Preserve `(workflow_id, workflow_step_id)` and render localized
loading/unavailable headings, including workflow context when needed. A missing
step ID uses a separate no-step section. Failed metadata reads retain rows;
explicit refresh retries them. A successful retry replaces the heading and
ordering without changing tasks or preferences.

## Section projection

Keep `buildTaskTree`'s current parent-first hierarchy. Assign each root and its
displayed descendants to the root's section. A child without its parent in the
current page remains a root and uses its own placement.

For workflow-step grouping, use a collision-safe encoding of the workflow/step
ID pair as the section key. Display the step name, prefixed with workflow name
when more than one workflow occurs among roots. Never group by a name or
`Task.state`. Names supplied by workflows are domain data and are not translated.

Order known workflow sections by `Workflow.sort_order` and then step `position`,
using names and IDs for deterministic ties. Unknown metadata follows known
metadata within its workflow; the no-step section comes last. Within a section,
retain the supplied task order and hierarchy. Counts continue to describe the
current page's displayed roots and children. Other built-in and plugin grouping
branches retain their current behavior.

Extract the section projection into a focused helper under `lib/tasks` if needed
to stay within frontend size/complexity limits. Do not turn UI specification IDs
or historical bug explanations into production comments.

## Desktop and phone surfaces

Desktop keeps its existing toolbar Select. Phone keeps
`MobileTasksListOptions` inside the existing `mobile-menu-sheet.tsx` view-options
drawer, the nearest shipped mobile exemplar. This is a short temporary choice;
the shared option model serves both surfaces. Preserve the drawer's fixed
header, internal scroll owner, safe-area handling, 44px touch control, and focus
return. The desktop Select uses intrinsic width with a 150px minimum so the
longer translated label remains visible. Verify the closed Portuguese selector
and document width in addition to phone geometry.

## Documentation and verification

During implementation update the List grouping bullet in
`docs/public/tasks-and-workflows.md` to explain Workflow step, configured step
names, and the distinction from runtime-status icons. The public page is a
how-to guide.

Focused tests distinguish same-step/different-state and same-state/different-step
tasks, repeated names across workflows, cold metadata loading, missing/deleted
steps, stale workspace responses, legacy settings and links, and hierarchy.
Desktop and mobile E2E scenarios select Workflow step and verify headings and
reload persistence. No ADR is needed for this local presentation correction.

## Implementation plans

- [Workflow step grouping](../../../plans/task-list-workflow-step-grouping/plan.md).
