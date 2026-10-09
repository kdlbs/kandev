---
id: coordinator-permissions-ui-design
title: Coordinator permissions and Watches UI design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
---

# Coordinator permissions and Watches UI System Design

## Purpose and boundaries

The May do and Watches sections of the coordinator page and the client-side
Watches filter of Needs you, Queue and the count strip. Storage, the settings
routes, the tool profile and the server-side guard are in
[permissions](permissions.md); the page shell and Sections row in
[coordinators](coordinators.md#configure-sections).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PERMISSIONS-001` (`001.6` to `001.10`) | [May do UI](#may-do-ui) |
| `REQ-COORDINATOR-PERMISSIONS-003` (`003.4` to `003.7`) | [Client watch filter](#client-watch-filter), [Watches UI](#watches-ui) |
| `REQ-COORDINATOR-PERMISSIONS-004` (`004.3`) | [May do UI](#may-do-ui) |

## May do UI

Files: `apps/web/components/coordinators/sections/may-do-section.tsx`,
`watches-section.tsx` and `control-draft.ts(x)` (the shared draft below),
registered as entries `may-do` and `watches` in
`coordinator-sections.tsx` (Sections row, [coordinators](coordinators.md#configure-sections)).
The sections and their hooks live under `components/coordinators/sections/`;
the Configure page that mounts them is
`apps/web/app/settings/workspace/[id]/coordinators/[coordinatorId]/page.tsx`
(public path `/settings/workspaces/:id/coordinators/:cid`), and the Needs you,
Queue and What it did screens are under `apps/web/app/coordinator/`.

```text
May do
  Create a task     (o) Denied  (*) Requires approval  ( ) Automatic
                    Last 30 days: 12 approved, 2 rejected  Review the last 30 days
  Start an agent    (*) Denied  ( ) Requires approval  ( ) Automatic
  ...
  Stop a task       (*) Denied   Stopping is not available yet.
  Merge a pull request      Always human
  Move a task to Done       Always human
```

- **One draft, one contributor, one PUT (`004.3`).** `CoordinatorSections`
  mounts the hook `useControlDraft(workspaceId, coordinatorId)` once, above
  the Sections row, and passes its value to both sections; neither section
  calls `useSettingsSaveContributor`. The hook loads `GET .../settings` and
  holds `{policy, watches}` drafts beside the last loaded (stored) values. It
  registers the single contributor `coordinator-control`, dirty when either
  member differs from stored by the normalised comparison of
  [Settings routes](permissions.md#settings-routes) (policy map; for `selected` the id set).
  Its save sends one PUT carrying only the members that differ, so an
  unvisited section (which the Sections row has not mounted) still saves
  correctly and one Save raises `policy_revision` once and resets the
  conversation once. After a 200 the drafts and stored values reload from
  the response, except that a draft member the manager changed after the
  PUT was sent (compared with what was sent) is kept and stays dirty against
  the new stored value; Save is disabled while that PUT is in flight, the
  controls are not. "Once" holds within this contributor: when Identity is
  also dirty, the save provider runs its contributors in sequence, so that one
  Save sends the Identity request and the control request separately and the
  conversation resets once per request that changes something.
- **Settings read.** While `GET .../settings` loads, both sections show a
  skeleton and no control is enabled and the contributor is not dirty. A
  failed read shows "Could not load these settings" with **Try again** in
  place of the controls, no Save is possible, and no draft exists; a
  response without `policy.actions` for all six actions, or without `watches`,
  is a failed read. A 404 means the coordinator was deleted: the page's
  existing not-found state replaces the sections.
- Automatic is a disabled radio with the note of
  `AC-COORDINATOR-PERMISSIONS-001.6`.
- **Per-row line.** The activity summary is read once per page mount with
  `days=30` (`activity/summary`, [activity log](activity-log.md#summary)),
  and again after each `coordinator.updated` event for this coordinator. While
  it is loading a row shows a skeleton in place of the line. Loaded, a row
  shows "N approved, M rejected" from `classes.<action>.approved` and
  `.rejected`, or "Nothing yet" only when both are zero. When the read has
  failed the row shows "Could not load the last 30 days" with **Try again**
  (one shared retry that re-issues the read); it never shows "Nothing yet" or
  a stale count for a failed read. A `stop` row shows its counts too (always
  zero for a coordinator that has only ever been `denied`). Every class
  appears in a valid response ([activity log](activity-log.md#summary)), so a
  response missing a row's class, or holding a null or non-integer count, is a
  failed read for every row, and so is a 404 or any other error. Reads are
  numbered per page mount; only the response of the latest read is applied and
  earlier ones are dropped, so a late older response never replaces a newer
  count or a newer failure.
- **Review link.** Each action row carries its own **Review the last 30
  days** link (`001.8`), enabled whether or not the counts are zero. It
  routes to `/workspaces/:id/coordinator/:cid/queue?class=<action>` and the
  Queue page scrolls its What it did section into view on arrival when the
  `class` parameter is present ([what it did UI](what-it-did-ui.md)); the
  filter preselect is that document's contract. The link opens the class
  filter without a date bound, so the list may hold older rows than the 30
  days the counts cover; the link text names the last 30 days only as the
  reason to look, and the list is newest first as What it did defines.
- When Start an agent is not Denied in the draft, the note of `001.9` shows
  under its row.
- **Reader.** A reader sees the six rows with their stored choice as
  disabled radios and the counts and links; the save bar shows no Save
  (readers hold no contributor that is dirty, and no control can change a
  draft).
- Leaving with an unsaved draft uses the phase-1 unsaved-changes guard, which
  covers the one contributor.
- **A `coordinator.updated` refetch** of the settings (another manager's
  save) never replaces an unsaved draft: it updates the stored baseline
  only. Policy merges per action: an action the manager changed stays as the
  draft has it, every other action follows the new stored value, and the PUT
  sends the six actions composed that way (the PUT names all six). Watches
  is one unit: a dirty Watches draft stays as is. The last committed save
  wins on the server (`004.2`), so a save racing another manager's may still
  overwrite an action changed in the window between the refetch and the PUT.
  Settings reads are numbered like the summary reads: only the latest
  response is applied.
- **Save error.** There is one error region, above the save bar, visible
  on every section, so the message is seen whichever section the manager is on.
  A PUT 400 puts one message there, prefixed by the section it belongs to
  ("May do: ..." for `policy.actions.<action>`, naming the action;
  "Watches: ..." for `watches`), and the draft is kept; the error clears on
  the next edit or Save. A 403 or 5xx shows the phase-1 save-failure toast
  and keeps the draft.
  A `watches_foreign_workflow` (a board deleted after the page loaded)
  refreshes the board list and keeps the draft with that board removed; when
  that leaves a `selected` draft with no board, the draft is invalid (see
  Watches UI, Last board) and the error region says "Watches: Keep at least
  one board in scope."
- The page note says saving starts the next conversation fresh (`004.3`).

## Watches UI

`sections/watches-section.tsx` shows the switch "Watch every board, including
new ones", on when the draft scope is `all`.

- **Board list.** Off lists every workflow of the coordinator's workspace,
  hidden ones included (a hidden workflow is watchable, `WatchSet`), in the
  workspace's existing order, each with its state In scope or Out and **Put
  this board in scope** / **Take this board out of scope**. A hidden board
  carries a "Hidden" tag. The list is read from
  `listWorkflows(workspaceId, {includeHidden: true})` for the workspace the
  settings page shows (not from the active-workspace workflows store, which
  holds only the active workspace); while it loads the section shows a
  skeleton and its controls are disabled; a failed read shows "Could not load
  boards" with **Try again**. Until the list has loaded the switch cannot be
  turned off (turning it off needs the list) and a `selected` draft cannot be
  edited; turning it on stays possible.
- **Switching off.** Turning the switch off starts a `selected` draft with
  every listed board in scope, in workspace order (nothing changes for the
  coordinator until Save; unchanged from what it watched). Two limits apply
  to that start and to Put this board in scope, both from the 50-board cap of
  [Settings routes](permissions.md#settings-routes): a workspace with no board
  refuses the switch-off with the inline message "This workspace has no
  boards to choose from." and the switch stays on; with more than 50 boards
  the draft starts with the first 50 in workspace order in scope and the rest
  Out, the message "At most 50 boards can be watched." shows under the
  switch, and Put this board in scope on a further board is refused with that
  message while 50 are in scope. Turning the switch on sets scope `all` (the
  ids are ignored on save). Turning it on and off again restarts from
  every board, not from the earlier draft.
- **Last board.** **Take this board out of scope** on the only in-scope board
  of the draft shows the inline error "Keep at least one board in scope."
  and leaves it in scope (`003.6`). A `selected` draft therefore never has
  zero boards through the switch or the buttons. The two ways it can
  (a deleted board dropped from the draft after `watches_foreign_workflow`,
  or an unedited stored empty set left by a workflow deletion) are
  distinguished: an unedited stored empty set is not sent by a save that did
  not touch Watches (see [Settings routes](permissions.md#settings-routes)), so
  it never blocks Save; an edited draft with no board is invalid, Save is
  disabled while it is, and the error region above the save bar says
  "Watches: Keep at least one board in scope." until a board is put in scope
  or the switch is turned on.
- **Watches nothing.** A coordinator whose stored scope is `selected` with an
  empty effective set shows, at the top of this section and of the
  coordinator's Configure page, the notice "This coordinator watches no
  board." with **Choose boards**. In the Watches section the notice is
  informational with no button, since the board list is on the same screen;
  on the Identity section and any other section **Choose boards** is a link
  to the Configure page's Watches section,
  `/settings/workspaces/:id/coordinators/:cid?section=watches` (managers
  only; readers see the notice without the link). The notice follows the
  stored value, not the draft.
- **Reader.** A reader sees the switch and each board's state, all disabled,
  with no Put/Take buttons.

## Client watch filter

**Placement (`003.4`).** Needs you, the Queue and the count strip read
their tasks from the client store and their stalls from
`GET /coordinator-stalls`, so the filter runs in the client, in one pure
module, `apps/web/lib/coordinator/watch-filter.ts`, used by every consumer
below; no other code compares a task's workflow with the watch set. It
exports `isTaskWatched(task, watchSet)` (true when the set is `all`, or the
task's `workflowId` is in the effective set; a task whose `workflowId` is
null is never watched) and `filterWatched({tasks, stalls}, watchSet)`, which
keeps the watched tasks and the stalls whose task is among them. A stall row
whose task is absent from the loaded snapshots or archived is dropped, as
classification already ignores it. `AttentionTask` gains
`workflowId: string | null`, set from the snapshot the task was read from.

The watch set input is the effective set from `GET .../settings` (`scope` and
effective `workflow_ids`), read through the hook
`useCoordinatorWatchSet(workspaceId, coordinatorId)`. It reads on screen
mount, on **Try again** and on each `coordinator.updated` event for this
coordinator (a Watches save by any manager publishes it, so open screens
follow the change without a reload), keeps its last successful value with its
load time, and is not read at all while the phase-2 flag is off (no filter,
phase-1 behaviour). Reads are numbered and only the latest response is
applied, so a late older response never replaces a newer value or a newer
failure; the kept value belongs to one coordinator and is discarded when the
viewed coordinator changes, so the screens are back in the not-yet-loaded
state (no task or stall items) until the new coordinator's read completes. It
is a new hook: `useCoordinatorWatches` of the copilot panel reads the
coordinator DTO for a hint and keeps no error state, so it is not reused and
is not changed. A task moving between workflows, or a change of the watch set
while task snapshots are still loading, is not special: the filter is a pure
function of the latest tasks, stalls and watch set at render time, and a
snapshot task whose `workflowId` is missing or null is not watched.

In `use-coordinator-attention.ts` the filtered tasks and stalls feed exactly
`classify` (so the Needs you items, every Queue group and the count strip)
and `computeNeedsYouCount` (the toast's "Next" count). `openTasksById` (a
proposal card's source-task head) and `tasks` (task availability for What it
did) stay unfiltered: the coordinator's own proposals always show with their
source task's identifier, and What it did never says "Task no longer
available" for a task that still exists. What it did rows are the
coordinator's own history and are not filtered. The sidebar badge is the
list route's `open_proposals`, which counts proposals only and is not
filtered.

**Watch set not available.** Before the first watch-set read completes, the
screens show no task or stall items and no counts derived from them (the same
loading state as a tasks input that has never loaded), and the proposals
show. If the first read fails, the screens do the same and the banner gains
the line "Could not load which boards this coordinator watches." with
**Try again**; after an earlier success a failed re-read keeps the last set
and the line reads "... Showing what was loaded at <time>." The line comes
after the three input lines of
[needs-you](needs-you.md#failure-and-recovery) and exists only while the
phase-2 flag is on. It fails closed: an unloadable watch set never shows
unwatched tasks.

**Watches nothing.** With an empty effective set the filter keeps no task
and no stall; Needs you shows only the coordinator's proposals and the
notice of [Watches UI](#watches-ui) with **Choose boards** (a link to
`/settings/workspaces/:id/coordinators/:cid?section=watches` for managers;
Needs you is `/workspaces/:id/coordinator/:cid`, a different page). The Queue groups are empty and each empty
group shows the phase-1 empty text; the count strip shows zeros (a filtered
zero, not an unavailable state).

