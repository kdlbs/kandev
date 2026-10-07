---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
  - REQ-UI-SIDEBAR-GROUP-INDENT-001
---

# Sidebar sort chain and color ranking system design

## Purpose and boundaries

Extend the existing sidebar sort object with ordered secondary rules. UI owns
this preference and effective marker color. Task summaries remain authoritative
for primary-session state and semantic activity. User settings remain authoritative
for personal manual colors and automatic rules.

The directory and requirement ID remain stable for continuation. This revision
replaces the fixed `runningFirstActivity` preset design. Partial backend changes
for that preset exist; reuse valid running/activity projections during implementation.
Do not expose the preset as the final multi-sort interface.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001` | Sort contract; Evaluation; Effective color; Server and local paths; Editor; Compatibility and recovery |
| `REQ-UI-SIDEBAR-GROUP-INDENT-001` | Group indentation preference |

## Sort contract

Retain `sort.key` and `sort.direction` as the primary rule. Add optional `color`
and `then_by` fields to the wire object. `then_by` contains flat criteria, never
recursive sort objects. Frontend state uses `thenBy`; API conversion maps it to
`then_by`. A single-sort object stays valid and retains its old semantics.

Example wire value:

```json
{
  "sort": {
    "key": "running",
    "direction": "desc",
    "then_by": [
      { "key": "color", "color": "red", "direction": "desc" },
      { "key": "lastActivityAt", "direction": "desc" }
    ]
  }
}
```

The logical chain is `[primary, ...then_by]`. Supported ordinary keys are
`state`, `updatedAt`, `lastActivityAt`, `createdAt`, `title`, and `running`.
`color` requires a named token from the ten-color automatic palette.
`desc` puts running or matching-color rows first; `asc` puts the other set first.
For date/text fields, retain existing direction semantics.

Allow one to ten criteria. Reject duplicate ordinary keys and duplicate color
preferences. Allow several color criteria with different preferred tokens.
`custom` is valid only as a standalone rule; the editor treats manual order as
an alternative mode and restores a standard one-rule chain when leaving it.
Changing to manual order is a draft change and never erases stored colors or
manual task order. All fields in a rule must pass typed validation.

Extend frontend `SortKey`/`SortSpec`, Go `SidebarTaskViewSort`, and user-settings
`SidebarViewSort`/draft models. Add flat criterion types where needed; avoid a
recursive wire type. Update query validation, API types, DTO conversion, settings
mapping, boot hydration, migrations, query identity, and copied/duplicated drafts.

## Evaluation

Compare criteria in order and stop at the first nonzero result. Only after the
whole chain ties can the final canonical task-updated/title/ID order apply.
A primary comparator must not apply an implicit timestamp tiebreak before later
criteria. Preserve the old full comparator for a legacy one-rule view where
that consumer has a historical tiebreak contract.

Running uses strict `primary_session.state === RUNNING`, aggregated with OR over
included subtree members. Do not use `state_aggregate.has_active`, which also
accepts workflow `IN_PROGRESS`. Activity uses the existing maximum over the
included subtree. Color uses the compared row's own marker token, without
subtree aggregation. This avoids giving a purple parent an invisible red rank.

Reuse filter-before-tree evaluation, strict nanosecond timestamp handling,
cycle guards, and existing malformed-value fallbacks. Preserve pins, manual
children, and group-heading precedence. Generic complete-data `applySort`
retains stable input ordinal after all criteria tie. Server pages stay authoritative.

## Effective color

The ranking value must match `resolveTaskItemColor` and `resolveAutomaticTaskColor`.
Resolve enabled automatic rules in stored order. The first match wins, including
workflow-step output. Otherwise, use the backend-owned manual color map.
A clear tombstone supplies no marker. Never use an underlying manual red value
when an automatic blue rule determines the visible marker.

The backend needs the same seven condition dimensions as the frontend:
workflow step, workflow, repository identity, executor profile, task state,
priority, and origin. Repository matching includes normalized provider host,
scope, provider repository ID, workspace identity, or normalized local path.
Use the existing frontend normalization rules as the contract, including missing
origin fallback to `kanban` and matching any attached repository.

Normalize fixed tokens and workflow color names/legacy classes through static
mappings equivalent to `task-color-presentation.ts`. Unsupported workflow color
maps to gray; strict hexadecimal values map to `custom`. A preferred named red
token does not match a custom hexadecimal value. Do not estimate visual similarity.

Create shared JSON color conformance fixtures for the existing TS resolver and
backend SQL evaluation. Cover every condition dimension, first-match precedence,
disabled/incomplete/unavailable rules, manual fallback, tombstones, aliases,
unsupported values, and custom hexadecimal output. Color resolution does not
mutate task records or copy personal priority into shared metadata.

## Server and local paths

`SidebarTaskViewQuery.Validate` validates the entire chain before query execution.
Projection needs inspect every criterion, not just `sort.key`. Secondary Running
and Last activity still require complete ancestor traversal and appropriate joins.
Build SQL ORDER BY from allowlisted expressions and directions. Parameterize
selected colors and rule targets; never interpolate submitted strings as SQL.

Retain bounded query execution, SQLite's pinned reader/scratch relation, and
PostgreSQL's single-statement path. Add an own-row effective color projection
before global ranking whenever a color criterion appears. Do not hydrate all
candidate tasks into Go or compute color only for returned page rows.

Extend `SidebarTaskViewPreferences` with a typed internal color-settings snapshot.
The authenticated settings reader in `sidebar_task_http.go` supplies it, using
`SidebarTaskColors` and `SidebarTaskColorAutomation`. Do not accept personal color
maps or automation settings from the query request. Task read types carry a
normalized snapshot; user models retain durable ownership. Use an explicit adapter
rather than introducing a second persisted color model.

Stage bounded manual color entries as indexed data, not thousands of SQL CASE
branches. Reuse SQLite's connection-local temporary preference staging and cleanup.
For PostgreSQL, use bounded parameter data relations. Compile ordered automatic
rule predicates from validated dimensions and parameterized targets. Match the
first enabled complete rule, then fall back to manual color. Add candidate facts
and repository/workflow/executor joins only when the active rules need them.
Keep current 50-rule, 10,000-manual-entry, and encoded-size limits.

All ranking uses one consistent task/settings snapshot. Include the complete
normalized chain and relevant personal settings identity in page/cache identity.
Use a bounded digest for color preferences rather than exposing the color map
in `query_key`. Color/settings changes invalidate affected cached pages and local
results. Workspace/user/request generation guards reject late responses as today.
Task, workflow-step, executor, or repository changes that alter color facts also
invalidate color-based ordering; invalidation cannot depend only on the current page.

Covered local evaluation needs complete task scope plus all facts used by enabled
color rules. Extend `requiredFields`, local projection, and source eligibility.
Require workflow-step colors, repository identities, priority, origin, and executor
profile as needed. Missing coverage falls back to the server. A known absent value
remains valid covered data and follows the color resolver's fallback semantics.

`localTaskComparator` evaluates the normalized criteria list using precomputed
running/activity maps and effective own-row colors. Extend generic `applySort`
with the same lexicographic behavior. Do not multiply the whole result by the
primary direction; each criterion applies its own direction independently.

## Editor

Replace the single sidebar `SortPicker` presentation with an ordered rule editor.
Keep the generic `TypedSortPicker` for other surfaces. The sidebar editor uses
existing Select/Button primitives, plus explicit Move up, Move down, Remove,
and Add sort controls. Changes enter the existing saved-view draft.

A color rule shows a labelled swatch selector and order, such as Red first.
Running shows Running first or Others first. Date fields show Newest first or
Oldest first. The collapsed Sort summary lists the ordered rules and each order.
At ten rules, disable Add sort with localized explanation. The final rule cannot
be removed. Incomplete additions stay as transient editor state until valid;
they never broaden a server query. The editor identifies invalid stored rules.

Desktop uses compact horizontal rule rows in the current anchored popover.
Phone/tablet uses the existing inset view-editor drawer with vertically stacked
rule cards. This surface already handles automatic-color rule editing and is
the nearest mobile exemplar. Keep one editor scroll body, a fixed drawer header,
dynamic viewport containment, safe areas, focus return, and keyboard dismissal.
Use at least 44px touch hit areas and compact 28px fine-pointer controls.
Explicit reorder buttons make every action available without drag or hover.

All copy uses the task locale catalog in English, pt-pt, zh-cn, zh-hk, zh-tw,
ja, and ko, plus generated pseudo copy. Keep color names accessible in text.
Any activity criterion enables own-row activity time in desktop switcher props,
`MobileTaskList`, and `TaskRowSettings`, regardless of its position in the chain.

## Group indentation preference

Add `groupIndent` to normalized `SidebarView`, `SidebarViewDraft`, and editor
state. Map it to optional `group_indent` in saved-view/draft API and Go user models.
Normalize absence or invalid values to `true`; preserve explicit `false`.
Use a presence-aware Go representation so default handling cannot turn false into true.
Include the field in view creation, duplication, draft comparison, conversions,
workspace state, boot hydration, and settings synchronization.

Place a labelled Checkbox or Switch below `GroupPicker`, inside the existing
`sidebar-group-settings` disclosure content. The control is present only when
Group by is expanded. The collapsed summary remains the selected grouping label.
Reuse existing primitives and localized task catalog copy in all seven languages.
The default control is checked. Choosing None retains the preference without
adding a grouping inset. Expansion/collapse remains transient editor state.

Pass the effective preference through desktop `buildTaskSwitcherProps` and phone
`MobileTaskList` into the shared group renderer. In `GroupSection` within
`task-switcher-tree.tsx`, replace unconditional `showHeader`-based `ml-5` with
`showHeader && groupIndent`. Keep true as the compatibility default for callers
without the prop. Preserve the group body role, accessible header relationship,
row-local marker spacing, and child `depth` offsets. The unchecked mode removes
only the group wrapper's margin; it does not flatten or re-parent subtasks.

This is presentation state, not a query criterion. Do not add it to
`SidebarTaskViewQuery`, ranking SQL, membership/page identity, or cache keys.
Toggling it reuses the current page and never selects another task. Continue
showing group headers/counts/continuation and use the unchanged sort chain.
Test both default and disabled geometry, including nested children and selected
row backgrounds, in desktop and phone renders.

## Compatibility and recovery

Legacy objects without `then_by` normalize to a one-rule chain. Preserve stored
filters, names, IDs, grouping, row presentation, directions, and defaults.
If a partial implementation persisted `runningFirstActivity`, expand it to
Running descending then Last activity descending on read. Accept that legacy
composite key temporarily as an input adapter; new clients do not write it.

For malformed stored data, normalize valid criteria in their original order.
Drop invalid criteria and duplicates with a visible editor explanation. If none
remain, use the existing Status fallback. A malformed query is an error instead
of a silent rewrite; return a bounded zero-based sort-rule index in safe error
metadata and localize its correction. For unknown fields on older clients, the
existing primary-key fallback remains; full multi-sort downgrade behavior is
not guaranteed.

No database schema migration, new endpoint, feature flag, activity event, or
session subscription is required. Settings writes use existing revision/CAS and
rollback paths. Reading settings can fail; keep the existing query-error behavior
rather than ranking from an unauthorized or incomplete color snapshot.

## Related decisions and contracts

- [Composable sidebar sort decision](../../../decisions/2026-10-06-composable-sidebar-sort-rules.md)
- [Activity source decision](../../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md)
- [Effective task colors](sidebar-automatic-task-colors.md)
- [Task tree activity](sidebar-task-tree-activity-sort.md)
- [Bounded query and scratch relation](sidebar-archived-filter.md)
- [Shared task data](sidebar-shared-task-state.md)
- [Workspace views](workspace-sidebar-task-views.md)

## Delivery

- [Plan and work orders](../../../plans/sidebar-running-first-activity-sort/plan.md)
