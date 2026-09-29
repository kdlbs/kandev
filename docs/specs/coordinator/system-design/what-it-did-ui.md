---
id: coordinator-what-it-did-ui-design
title: What it did UI design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-ACTIVITY-LOG-002
  - REQ-COORDINATOR-ACTIVITY-LOG-003
---

# What it did UI System Design

The Queue section that renders a coordinator's activity rows. The rows, routes
and undo it reads are specified in [What it did (activity log)](activity-log.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-ACTIVITY-LOG-002` | [Data](#data), [Rows](#rows), [Copy table](#copy-table) |
| `REQ-COORDINATOR-ACTIVITY-LOG-003` | [Undo flow](#undo-flow), [Copy table](#copy-table) |

## Placement

`apps/web/app/coordinator/queue/what-it-did.tsx` and `what-it-did-row.tsx`,
below the phase-1 Queue groups, fed by
`hooks/domains/coordinator/use-activity.ts`. It renders only with
`features.coordinatorPhase2` on and mounts after the groups.

## Data

One list state per (coordinator, class filter): loaded rows, the last
page's `next_cursor`, a request generation, the message map and a status
(`loading | loaded | failed`).

- **Reads.** The first page loads on mount. **Load more** appends the page
  after the last loaded row's cursor and is ignored while a Load more is in
  flight. Every other refresh is a **re-read**: it fetches page 1, then follows
  `next_cursor` once for each further page that was loaded, and replaces the loaded rows and the cursor
  only when the last page has arrived. A re-read therefore never merges: rows
  are exactly what the server returned, in server order (`created_at DESC, id
  DESC`), the cursor is the last re-read page's, and a row beyond the re-read span drops off the end. A failed re-read keeps the rows shown and sets no banner; only the
  first load and Load more have a failure state (below). The client sends no `limit` on any page. A re-read that starts supersedes an in-flight Load more (its
  response is dropped) and Load more is disabled while a re-read is in flight.
  An event or reconnect while the first load is in flight or after it failed
  is ignored; Retry decides.
- **Triggers.** A `coordinator.updated` event for the coordinator, a
  reconnect of the WebSocket, and every undo outcome that says the row's state
  may have changed (200, `already_undone`, `not_undoable`, 404) trigger a
  re-read. Re-reads never overlap: one triggered while another is in flight
  queues at most one trailing re-read. A filter change or a change of the selected coordinator discards
  the list and message map and starts a first load.
- **Stale responses.** Each first load, re-read and Load more carries the
  generation current when it started; the generation increments on a filter
  or coordinator change and on unmount. An older generation's response is
  dropped, and an undo response for a row no longer listed, or under a
  newer generation, sets no message.

- `?class=` preselects the filter (May do's link uses it); an unrecognised
  value selects All and is left in the address until the filter changes. A
  filter change replaces the current history entry (no new entry) with
  `?class=<class>`, and removes `class` when All is chosen. The filter offers All, the six classes and "Unknown action".
  When the page loads with a `class` parameter present, What it did is scrolled
  into view once on arrival (May do's **Review the last 30 days** link).
- Names: the workspace member list (`listWorkspaceMembers`), read on mount
  and at most once more per mount, gives `user_id` to `display_name`.
  The member list is the only name source: a person who holds access through
  an inherited role has no member row, and "a former member" therefore means
  "not in the workspace member list", an accepted limit. The second read runs when a
  list arrival (first load, re-read or Load more) leaves a row's person absent
  from a loaded list, or the first read failed, and the first read started more
  than 30 seconds ago; one member read is in flight at a time, and the list
  stays `loaded` while it runs. State is `loading | failed |
  loaded` (a failed second read sets `failed`); `loading` and `failed` render the no-person outcome forms in both
  the outcome line and the Undo cell ("Undone, <time>"), and `loaded` with the
  id absent renders "a former member" (`002.8`). A member with an empty or
  missing `display_name`, and a null `actor_user_id`, render the no-person
  form in every state.
- Step names and task availability come from `useCoordinatorTasks` through
  the Queue's `useCoordinatorAttention` result, which is extended to also
  return `tasks`, `loadedAt` and `error`; the Queue passes them to the section
  as props, so the section calls no hook of its own for them:
  `stepNameByWorkflowStep` (keyed `${workflowId}:${stepId}`) is flattened
  into a step-id to title map (ids are unique across workflows); a
  `from_step_id` absent from the map is unknown. A `target_task_id` in `tasks` is available. The set becomes trusted the first time a result arrives with `loadedAt` set
  and `error` false, held in a section-level flag that a later failure does not clear; `loadedAt` alone never trusts the set, as the hook sets it with `error` true on a partial failure (`002.9`).
- **List states.** While the first load is in flight the section shows
  `activityLoading` and no empty text. A failed first load (any error, including 403 and 404) shows
  `activityLoadFailed` with a **Retry** button (`activityRetry`) and never the
  empty text. A failed Load more keeps the rows and button and shows
  `activityLoadMoreFailed` beneath the list until the next attempt.

With status `loaded` and no rows the text is `activityEmpty` for All,
`activityFilteredEmpty` for a class.

## Rows

Columns When, Action, Action class, How it was authorised, Undo. The When cell
shows `created_at` (for a coalesced refusal too, so the column agrees with the
list order) as `formatRelative` (`lib/i18n/formats.ts`); its exact time is `formatDateTime` in the viewer's
locale and time zone, shown in a tooltip that opens on hover and on keyboard
focus (the cell is focusable); a refusal with `refusal_count` above 1
adds `activityLastRepeat` to that tooltip, its time `formatDateTime` of `updated_at`. An empty or unparsable timestamp shows nothing. The Action cell text is chosen in this order: a refused row shows its
reason text; a rejected or failed row shows `detail` with its prefix
(`activityRejectedDetail` / `activityFailedDetail`), or `activityRejected` /
`activityFailed` alone when the detail is empty; any other row shows `detail`,
or the class label when it is empty. The text (line-clamped to two lines, the full text as the title
attribute, rendered as text) with the identifier link after it, separated by a single space: the
identifier links to the task when `target_task_identifier` is set, "Open
task" when only the snapshots hold it, "Task no longer available" as plain
text under `002.9`, nothing when the row has no `target_task_id` (an identifier without an id is not shown). Phone width
stacks each row as a card (When and class, Action, authorisation, then a
full-width Undo).

Undo cell, first match wins: "Undone by <name>, <time>" when `undone_at` is
set (`003.6`); **Undo** for a manager when `undoable` is true; "No undo" on
every row of class `message` or `resume` whatever its outcome and for readers
too; nothing otherwise, including the `undone` outcome row, whose undoer shows
in How it was authorised. A row's failure message (`003.11`) renders below the
Undo button in a `role="status"` region, even when the cell
would otherwise be empty.

## Copy table

All keys are in the `coordinator` namespace, six locales, each a whole
sentence with interpolation only for names, times, counts and codes (never a
noun phrase spliced into a sentence). The one exception is `activityFormerMember`,
a name value for `{{name}}` in the "by" outcome keys, translated as a short noun
phrase that reads correctly after "by"; a locale whose grammar cannot do that
translates those keys to read correctly with it. "x" is the ASCII letter. No em dash.

| Key | English |
| --- | --- |
| `activityTitle` | What it did |
| `activityEmpty` | It has not done anything yet. |
| `activityFilteredEmpty` | Nothing matches this filter. |
| `activityFilterLabel` | Action class |
| `activityFilterAll` | All |
| `activityClassCreateTask` / `StartAgent` / `Message` / `Move` / `Resume` / `Stop` / `Unknown` | Create task / Start agent / Message task / Move task / Resume task / Stop task / Unknown action |
| `activityAuthRequires` / `activityAuthDenied` | Requires approval / Denied |
| `activityAuthDeniedRepeated` | Denied x {{count}} |
| `activityApprovedBy` / `ApprovedByEdited` | Approved by {{name}} / Approved by {{name}}, with edits |
| `activityApproved` / `ApprovedEdited` | Approved / Approved, with edits |
| `activityRejectedBy` / `activityRejected` | Rejected by {{name}} / Rejected |
| `activityFailed` | Failed |
| `activityUndoneBy` / `activityUndone` | Undone by {{name}} / Undone |
| `activityFormerMember` | a former member (the `{{name}}` value) |
| `activityRejectedDetail` / `activityFailedDetail` | Rejected: {{detail}} / Failed: {{detail}} |
| `activityRefusedBindingInvalid` | Its tool settings could not be read. |
| `activityRefusedNotInProfile` | It called something it is not allowed to use. |
| `activityRefusedPolicyDenied` | A manager has set this action to Denied. |
| `activityRefusedCode` / `activityRefused` | Refused. {{code}} / Refused. |
| `activityTaskGone` / `activityOpenTask` | Task no longer available / Open task |
| `activityUndoAction` / `activityNoUndo` | Undo / No undo |
| `activityUndoneByAt` / `activityUndoneAt` | Undone by {{name}}, {{time}} / Undone, {{time}} |
| `activityUndoTitle` | Undo this? |
| `activityUndoConfirmCreate` / `...CreateNoId` | the two create sentences of `003.10` |
| `activityUndoConfirmMove` / `...MoveNoStep` / `...MoveNoId` / `...MoveNoIdNoStep` | the four move sentences of `003.10` |
| `activityUndoConfirm` / `activityUndoCancel` | Undo / Cancel |
| `activityConflictMoved` (moved, archived, unknown or absent reason) | It has moved since |
| `activityConflictAgentRunning` | An agent is working on it. Stop it, then undo. |
| `activityConflictStepDeleted` | The step it came from no longer exists. |
| `activityConflictStepDone` | The step it came from is now a finishing step. |
| `activityConflictStepFull` | The step it came from is full. |
| `activityNotUndoable` | This can no longer be undone |
| `activityUndoFailed` | Undo failed. Try again. |
| `activityGone` | This action is no longer listed. |
| `activityLoadMore` | Load more |
| `activityLoading` | Loading what it did |
| `activityLoadFailed` / `activityRetry` | What it did could not be loaded. / Retry |
| `activityLoadMoreFailed` | More could not be loaded. Try again. |
| `activityLastRepeat` | Last repeated {{time}} |

Traditional Chinese comes from `pnpm run i18n:zh-hant`.

## Undo flow

Undo opens the dialog (`003.10`). The dialog is built on the base
`AlertDialog` whose content sets `enterConfirms` false: Enter activates whichever button has
focus (Cancel on open), so the dialog has no default confirm; Escape and Cancel
close it and send nothing (a click outside does nothing, as AlertDialog blocks it), and focus returns to the row's Undo button (or to
the section heading when the row is gone or its Undo became text). Confirming closes the dialog and
sends `POST activity/:rid/undo`, marking that row's Undo `aria-disabled` (it
keeps focus and ignores activation) until the response settles and, after a
200, until the re-read it triggers settles, so a row has at most one request in flight; another row's Undo may
run at the same time. Results:

- 200: re-read; the returned original row is not used (the re-read supplies
  it); no message. If that re-read fails, Undo is clickable again, the row unchanged, with
  no message (a second Undo answers 409 `already_undone` and re-reads).
- 409 `already_undone`: re-read, no message.
- 409 `undo_conflict`: the text by `reason`; an unknown or absent reason (a code the client predates, or no `reason`) reads
  `activityConflictMoved`. No re-read, so the row stays and Undo stays
  clickable.
- 409 `not_undoable`: `activityNotUndoable`, re-read at once.
- 404: `activityGone` in the section notice, re-read at once.
- Anything else, including a network error, a 403 and a 409 with an
  unrecognised `code`: `activityUndoFailed`, no re-read, Undo stays clickable.

The message map is `Map<rowId, {text, survives, setAt}>` plus the section notice (same rule, no row); the re-read counter rises when a re-read starts, never when queued, and `setAt` is its value when the message is set. `not_undoable` and
404 set `survives` true. Only a successful re-read numbered above `setAt` clears one. `survives` is consumed by the first re-read that starts after the
refusal settles (the refusal's own at-once re-read; a queued trailing one is it), when it settles
(success, failure or superseded): the message stays
through it and `survives` becomes false. A re-read already in flight when the
refusal settled neither clears nor consumes. An event or reconnect
re-read starting later coalesces into that trailing one. A successful re-read numbered above `setAt`
clears a message with `survives` false, as do a
filter or coordinator change and a new confirmed Undo of that row (`003.11`);
a failed re-read and Load more clear nothing. A second failure on one row replaces its message.
