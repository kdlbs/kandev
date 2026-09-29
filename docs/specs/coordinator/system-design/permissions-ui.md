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
The web code of the coordinator page lives under `components/coordinators/`
and `app/coordinator/`; nothing of it is under `app/settings`.

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
  the response. Save is disabled while that PUT is in flight.
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
  zero for a coordinator that has only ever been `denied`).
- **Review link.** Each action row carries its own **Review the last 30
  days** link (`001.8`), enabled whether or not the counts are zero. It
  routes to `/workspaces/:id/coordinator/:cid/queue?class=<action>` and the
  Queue page scrolls its What it did section into view on arrival when the
  `class` parameter is present ([what it did UI](what-it-did-ui.md)); the
  filter preselect is that document's contract.
- When Start an agent is not Denied in the draft, the note of `001.9` shows
  under its row.
- **Reader.** A reader sees the six rows with their stored choice as
  disabled radios and the counts and links; the save bar shows no Save
  (readers hold no contributor that is dirty, and no control can change a
  draft).
- Leaving with an unsaved draft uses the phase-1 unsaved-changes guard, which
  covers the one contributor.
- **A `coordinator.updated` refetch** of the settings (another manager's
  save) never replaces an unsaved draft member: it updates the stored
  baseline only, so the draft stays dirty against the new baseline and a
  following Save sends it; a member the draft did not change follows the new
  stored value. The last committed save still wins on the server (`004.2`).
- **Save error.** A PUT 400 shows one inline error above the save bar naming
  the field from the response: `policy.actions.<action>` errors under the
  May do heading, `watches` errors under the Watches heading, and the draft is
  kept; a 403 or 5xx shows the phase-1 save-failure toast and keeps the
  draft. A `watches_foreign_workflow` (a board deleted after the page loaded)
  refreshes the board list and keeps the draft with that board removed.
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
  boards" with **Try again**, and the switch stays usable but a `selected`
  draft cannot be edited until the list loads.
- **Switching off.** Turning the switch off starts a `selected` draft with
  every listed board in scope (nothing changes for the coordinator until
  Save; unchanged from what it watched). Turning it on sets scope `all`
  (the ids are ignored on save). Turning it on and off again restarts from
  every board, not from the earlier draft.
- **Last board.** **Take this board out of scope** on the only in-scope board
  of the draft shows the inline error "Keep at least one board in scope."
  and leaves it in scope (`003.6`). A `selected` draft therefore never has
  zero boards, except an unedited stored empty set left by a workflow
  deletion; that set is not sent by a save that did not touch Watches
  (see [Settings routes](permissions.md#settings-routes)), so it never blocks Save.
- **Watches nothing.** A coordinator whose stored scope is `selected` with an
  empty effective set shows, at the top of this section and of the
  coordinator's Configure page, the notice "This coordinator watches no
  board." with **Choose boards**. In the Watches section the notice is
  informational and **Choose boards** turns the switch on, which is the
  simplest way back; on the Identity section and any other section
  **Choose boards** is a link to `?section=watches` (managers only; readers
  see the notice without the link). The notice follows the stored value, not
  the draft.
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
phase-1 behaviour).

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
`?section=watches` for managers). The Queue groups are empty and each empty
group shows the phase-1 empty text; the count strip shows zeros (a filtered
zero, not an unavailable state).

