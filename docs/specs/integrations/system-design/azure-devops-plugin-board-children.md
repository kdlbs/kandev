---
status: draft
system: integrations
created: 2026-11-11
owners:
  - kandev
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001
---

# Azure DevOps Plugin Board Card Children System Design

## Purpose and boundaries

Implements [the board card children requirements](../requirements/azure-devops-plugin-board-children.md)
inside the Azure DevOps connector plugin. It adds one browse action and
extends the Board described in the
[plugin system design](azure-devops-plugin.md#browse-screens). No Kandev host
contract changes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001` | [Azure DevOps calls and action](#azure-devops-calls-and-action), [Board UI](#board-ui) |

## Azure DevOps calls and action

Azure DevOps calls used (all `api-version=7.1`):

| Need | Call |
| --- | --- |
| Parent to child links | `POST {project}/_apis/wit/wiql` with a link query: `SELECT [System.Id] FROM WorkItemLinks WHERE [Source].[System.Id] IN (...) AND [System.Links.LinkType] = 'System.LinkTypes.Hierarchy-Forward' MODE (MustContain)`, at most 200 parent IDs per query |
| Child fields | the existing work-items batch read, in pages of 200, plus `Microsoft.VSTS.Common.BacklogPriority` and `Microsoft.VSTS.Common.StackRank` |
| State categories | `GET {project}/_apis/wit/workitemtypes/{type}/states` once per child work-item type in the response |

Action:

| Action | Input | Output |
| --- | --- | --- |
| `boards.children` | `projectId`, `ids` (the board's card IDs) | `children[]` (`parentId`, `items[]` child cards with `stateCategory`), `truncated` |

`boards.children` reads the links of every card on the board (the link
queries run concurrently, like sprint reads), then batch-reads the children
and the state categories of their types. Children in the `Removed` category
are dropped. Children keep backlog order (`BacklogPriority`, then `StackRank`,
then ID). The response stays below 1 MiB by bounding children to 1,000 per
call, summarized like list payloads, and reporting `truncated` beyond that.
The mock client serves children from parent links in its seed, and state
categories from a per-type map.

## Board UI

The Board requests `boards.children` with the snapshot's card IDs after the
snapshot renders, so the counts never delay the cards; a new set of cards
requests them again (moving a card does not), and a stale request is aborted.
The UI sends at most 1,000 card IDs, which `boards.children` enforces, and
reports `truncated` itself beyond that. A card with children shows a toggle in
its footer, `{type}: {done}/{total}` when all children share a work-item type
and the localized "Child items: {done}/{total}" otherwise. The toggle and
the child lines stop pointer and keyboard events, so they never open or drag
the card. Child lines reuse the sprint card fields (number, type, title,
state, assignee), open `AzureDevOpsWorkItemDetail`, and carry
`WorkItemTaskActions`.

Every child line whose own children are known shows the same toggle and
expands them below itself, indented with a left border, so an epic opens to
its features, their stories, and their tasks. The Board keeps one children
index per set of cards: the card level is read first, and whenever cards or
lines are expanded, the items they show whose children were never requested
are read in one `boards.children` call (at most 1,000 IDs). Items without
children are recorded as such, so no level is read twice; a failed level
leaves its items without toggles and shows the unavailable notice.

Expansion is per card and per line in the page session. A Board header
control expands or collapses the first level of every card with children; its value is stored per board in the
Board browse preference (`host.storage`, per user) as `expandChildren`, and
applies when the board opens. Phone cards use the same component inside the
column navigator.

Failure handling: when `boards.children` fails, cards render without
toggles and an inline notice says the child items are unavailable; when it
reports `truncated`, the notice says some child items are not shown.

## Testing

- Unit: link-query chunking, `Removed` filtering, backlog ordering, done
  counts, the toggle label with one or several types.
- Plugin action tests against the mock client for `boards.children`,
  including truncation and a failed read.
- Packaged E2E on desktop and phone: counts, expand and collapse, child detail
  and task actions without opening the card, expand all remembered after a
  reload, cards shown while children are delayed.
