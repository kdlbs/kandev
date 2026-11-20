---
status: current
system: ui
requirements:
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002
---

# Workflow Settings Compact List System Design

## Purpose and boundaries

The [requirements](../requirements/workflow-settings-compact-list.md) add a
presentation layer to `/settings/workspaces/:id/workflows`. The change is
frontend-only and lives in `apps/web`. Workflow data, steps, the manual-save
contract ([settings manual save](../requirements/settings-manual-save.md)),
ordering, export, duplication, and deletion keep their current owners and
behavior.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `001.1`, `001.2` | [Compact row](#compact-row) |
| `001.3`, `001.4`, `001.5`, `001.8` | [Expansion state and mounting](#expansion-state-and-mounting) |
| `001.6` | [List and drag handle](#list-and-drag-handle) |
| `001.7` | [Accessibility and responsive layout](#accessibility-and-responsive-layout) |
| `002.1`-`002.5` | [View preference](#view-preference) |

## Components

- `useWorkflowListView` in
  `apps/web/hooks/domains/settings/use-workflow-list-view.ts` owns the view
  preference.
- `WorkflowListViewToggle` in
  `apps/web/components/settings/workflow-list-view-toggle.tsx` renders the
  control with `ToggleGroup` from `@kandev/ui/toggle-group`.
- `WorkflowCompactRow` in
  `apps/web/components/settings/workflow-compact-row.tsx` renders the row
  header. It receives data only and does not fetch.
- `WorkflowCard` in `apps/web/components/settings/workflow-card.tsx` gains a
  `compact` prop and uses the disclosure hook.
- `useWorkflowCardDisclosure`, `WorkflowCardDisclosureBody`, and
  `useExpandOnSettingsTarget` in
  `apps/web/components/settings/workflow-card-disclosure.tsx` own expansion
  state, lazy keep-mounted bodies, and deep-link expansion.
- `WorkflowList` and `SortableWorkflowItem` in
  `apps/web/app/settings/workspace/workspace-workflows-list.tsx` read the
  preference, render the toggle, and pass `compact` down.

## View preference

`useWorkflowListView()` returns `{ view, setView }`, where `view` is
`"compact" | "detailed"`. It reads `localStorage` key
`kandev:workflows:settingsListView:v1` once into React state. A missing,
unreadable, or unknown value yields `"compact"`. `setView` updates React state
first and then writes the value in a `try`/`catch`. A failed write is ignored,
so the view still switches for the current visit. The preference is not a
settings draft and never registers with `useSettingsSaveContributor`.
Cross-tab synchronization is not required. The storage key and values are
program data and carry an `// i18n-exempt` marker if the copy scanner flags
them.

`WorkflowListViewToggle` sits in a header row directly above
`workflow-order-list`, right-aligned, inside `WorkflowList`. The Workflows
section content is unframed and starts at its divider, so
`WorkspaceWorkflowsClient` wraps the section children in `pt-4`. The first
block, either the sync status or this control, keeps 16 px of space below the
divider. The sync status card and its separator span the full list width, so
they align with the workflow rows; drag handles sit inside the cards and need
no left gutter. It is outside
`WorkflowSectionActions`, so it also appears in the Improve Kandev workspace.
It is a single-selection `ToggleGroup` with two items. Each has an icon and a
visible localized label (`workflows:listViewCompact`,
`workflows:listViewDetailed`), and the group has the accessible name
`workflows:listViewLabel`. An empty selection is ignored so one view stays
selected. Test IDs: `workflow-list-view-toggle`,
`workflow-list-view-compact`, `workflow-list-view-detailed`.

## Expansion state and mounting

`useWorkflowCardDisclosure` in
`apps/web/components/settings/workflow-card-disclosure.tsx` holds the state
(`expanded`, `open`, `mounted`, `toggle`, `expand`). A card starts expanded
when its ID is a draft ID (`TEMP_WORKFLOW_PREFIX`) or when
`useIdsAddedAfterMount` in `WorkflowList` reports that the ID was not present
on the list's first render. Saving a draft replaces its ID, and with it the
React key, so the remounted card is new to the list and starts expanded
again. Import ends with `router.refresh()`, which reloads the page, so
imported workflows start a new visit and default to collapsed. The expansion
state is component-local and session-only.

The rendered state is `open = !compact || expanded`. In Detailed view the card
renders exactly as today, with no row header. In Compact view the card renders
`WorkflowCompactRow` as its first child and the existing body and actions
below it while `open` is true.

All card-level state, including steps, the draft contributor, delete state,
and dialogs, already lives in `useWorkflowCardState`. Collapsing does not
unmount `WorkflowCard`, so drafts and the shared save surface are unaffected.
The body subtree (`WorkflowCardBody` and `WorkflowCardHeaderActions`) mounts
lazily the first time it is shown, in either view. After that it stays
mounted and is hidden with the `hidden` attribute when collapsed, including
after a switch from Detailed to Compact. This preserves local pipeline editor
state such as the selected step and the session-config resolution flag. Use
`Collapsible`/`CollapsibleContent` with `forceMount` only if it gives the same
lazy, keep-mounted result. Otherwise use a plain wrapper.

`useExpandOnSettingsTarget`, which `useWorkflowCardDisclosure` calls with
`workflow-card-<id>`, calls `expand` when the URL fragment matches on mount,
`hashchange`, `popstate`, or `LOCATION_CHANGE_EVENT`, and on a matching
`SETTINGS_TARGET_REQUEST_EVENT`. These are the events
`SettingsTargetProvider` already uses, so the existing reveal scroll and
highlight still apply to the expanded card.

## Compact row

`WorkflowCompactRow` props: `workflow`, `steps`, `stepsLoading`,
`agentProfileLabel`, `readOnlyReason` (`"synced" | "improve" | null`),
`isDirty`, `expanded`, `onToggle`, and `bodyId`. Data comes from the existing
card state: `s.workflowSteps`, `s.workflowLoading`, `s.hasUnsavedChanges`, and
`s.readOnly`. The profile label comes from `useHealthyAgentProfiles` matched on
`workflow.agent_profile_id`, and is omitted when unset or unmatched. The row
adds no request. Steps render in `position` order as a color dot plus name,
with a chevron separator, inside one `min-w-0 truncate` container. The step
count uses plural keys (`workflows:compactStepCount_one`/`_other`). While steps
load, the step region shows `workflows:loadingWorkflowSteps`.

Markers are non-interactive `Badge`s inside the header button. The existing
`WorkflowSyncedBadge` has a focusable tooltip trigger, which would be invalid
inside a button, so the row uses plain badges: `workflows:synced`,
`workflows:compactReadOnly`, and `workflows:compactUnsaved`. The source path
stays available in the expanded card.

## List and drag handle

`SortableWorkflowItem` keeps the drag handle as an absolute sibling of the
card and receives `compact`. When it is true, the handle is vertically
centered on the row header (`top-1.5`, coarse `top-0`) instead of `top-4`. The
compact row reserves right padding for the handle so text never sits under it.
Sorting, `onReorder`, and the order dirty markers are unchanged.

## Accessibility and responsive layout

The row header is one native `<button type="button">` with `cursor-pointer`,
`aria-expanded={expanded}`, `aria-controls={bodyId}`, and the accessible name
`workflows:toggleWorkflowDetails` with `{{name}}`. Native button semantics
handle Enter and Space. Test ID: `workflow-compact-row-<id>`. The card keeps
`data-testid="workflow-card-<id>"` and adds `data-expanded`.

Layout uses `useResponsiveBreakpoint` only where markup cannot express the
rule with the matching Tailwind boundary (`md` = 768 px, `lg` = 1024 px):

- At 1024 px and wider: one line with chevron, name, markers, truncated
  description, steps, count, and profile.
- From 768 to 1023 px: one line with no description.
- Below 768 px: two lines, with the identity line (chevron, name, markers)
  first and the steps plus count second.

Fine-pointer desktop rows use settings density (about 40 px). Below 768 px
(any pointer) and on coarse pointers the header, the drag handle, and each view
control option are at least 44 px (`max-md:` and `[@media(pointer:coarse)]:`
sizing). The document has no horizontal
overflow. The steps container truncates inside the card.

## Failure, persistence, and observability

Storage failures degrade as described in [View preference](#view-preference).
A step-load failure keeps the existing toast, and the row shows zero steps. No
backend calls, metrics, or logs are added.

## Related records

- [Collapsible agent blocks](../../agents/requirements/collapsible-agent-blocks.md),
  the nearest shipped precedent.
- [Settings composition](../requirements/settings-composition.md).
- [Delivery plan](../../../plans/workflow-settings-compact-list/plan.md).
