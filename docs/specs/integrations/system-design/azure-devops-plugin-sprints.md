---
status: draft
system: integrations
created: 2026-11-10
owners:
  - kandev
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001
---

# Azure DevOps Plugin Teams and Sprints System Design

## Purpose and boundaries

Implements [the teams and sprints requirements](../requirements/azure-devops-plugin-sprints.md)
inside the Azure DevOps connector plugin. It extends the plugin's browse
actions, browse preferences, and browse page described in the
[plugin system design](azure-devops-plugin.md#browse-screens). No Kandev host
contract changes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001` | [Azure DevOps calls and actions](#azure-devops-calls-and-actions), [Filters and preferences](#filters-and-preferences), [Taskboard UI](#taskboard-ui) |

## Azure DevOps calls and actions

Azure DevOps calls used (all `api-version=7.1`, team-scoped ones under
`{org}/{project}/{team}/_apis/work`):

| Need | Call |
| --- | --- |
| Teams of the connected identity | `GET _apis/projects/{project}/teams?$mine=true`, paged like `teams.list` |
| Team areas | `teamsettings/teamfieldvalues` (`values[].value`, `includeChildren`) |
| Team sprints | `teamsettings/iterations` (`id`, `name`, `path`, `attributes.startDate`, `finishDate`, `timeFrame`) |
| Working days and days off | `teamsettings` (`workingDays`), `teamsettings/iterations/{id}/teamdaysoff` |
| Sprint contents | `teamsettings/iterations/{id}/workitems` (`workItemRelations` with `System.LinkTypes.Hierarchy-Forward` parent-child pairs) |
| Taskboard columns | `taskboardcolumns` (column order and the work-item type and state mapped to each column); when it returns no columns (the team never customized its taskboard), `backlogs` (the `task` level's work-item types and default type) and `_apis/wit/workitemtypes/{type}/states` |
| Card fields | the existing work-items batch read, in pages of 200 |

Actions:

| Action | Input | Output |
| --- | --- | --- |
| `teams.list` (extended) | `projectId` | `teams` plus `mineIds`; the UI groups **My teams** first |
| `teams.settings` | `projectId`, `teamId` | `areas[]` (`path`, `includeChildren`), `iterations[]` (`id`, `name`, `path`, `start`, `finish`, `timeFrame`) |
| `sprints.board` | `projectId`, `teamId`, `iterationId` | `iteration`, `workingDaysLeft`, `columns[]`, `rows[]` (`parent` card or none, `tasks[]` cards with `columnId`), `people[]` |

`sprints.board` reads the iteration's work-item relations, batch-reads the
cards, assigns each task to the first column whose mapping matches its type and
state (else the first column mapping the state, else the first column), and
orders rows by `Microsoft.VSTS.Common.BacklogPriority` (then `StackRank`, then
ID). A taskboard the team never customized has no columns in Azure DevOps;
the plugin then uses Azure's default, one column per state of the task
level's default type in workflow order without removed states, mapping the
other task types by state name, else by state category. The UI shows a task
without a known column in the first column. A work item whose type the
taskboard columns map is a task; any other
top-level item is a backlog item row. Tasks whose parent is outside the sprint,
and tasks without a parent, go to the unparented row, shown first. The six
sprint reads run concurrently. `teams.list` reads the identity's teams next to
the full list; when Azure DevOps refuses that read, it still lists every team
with no `mineIds`. Working days left count the
team's working days from today to `finishDate`, minus team days off. The
response stays below 1 MiB by bounding cards to 1,000 per sprint and reporting
`truncated` beyond that.

## Filters and preferences

Work-item filters add WIQL conditions to the generated query:
`[System.AreaPath] UNDER '<path>'` (or `=` when `includeChildren` is false),
ORed across the team's areas, and `[System.IterationPath] = '<path>'` for the
sprint. Quotes are doubled as in the title search. Saved views store
`teamId` and `iterationId` and rebuild the conditions when they run.

Saved views also gain `teamId` and `iterationId` on the server side
(`SavedView`); a sprint without its team is rejected. The UI resolves the
team's settings when a search runs, so a saved view rebuilds its conditions
from the current team areas.

Browse preferences (`host.storage`, per user) gain
`teamByProject: Record<projectId, teamId>` and the last Sprints selection
(`iterationId`, person). The Board, Work items, and Sprints team selectors write
the same map; the Board and Sprints read it to preselect a team. The sprint
selector opens on the stored sprint while it exists, else the current one.

## Taskboard UI

Sprints is a mode tab after Board. Desktop rows render a parent card column
and one column per taskboard column; phone rows render the parent card and,
under it, one group per column. Sprint cards open
`AzureDevOpsWorkItemDetail` and, like Board cards, carry the work-item task
actions menu, whose events never open or drag the card. Kandev's combobox has
no option groups, so **My teams** and the sprint tag and dates are option
descriptions shown under the name; the closed selector shows the name only.

Failure handling: a team without iterations or a sprint without items renders
an empty state under the selectors; a failed call keeps the selectors usable
and shows the request error in place of the rows.

## Testing

- Unit: column mapping, row grouping and ordering, unparented row, working
  days left, WIQL area and iteration conditions, my-teams ordering.
- Plugin action tests against the mock client for `teams.list` `mineIds`,
  `teams.settings`, and `sprints.board`, including truncation.
- Packaged E2E on desktop and phone with mock teams, sprints, and tasks.
