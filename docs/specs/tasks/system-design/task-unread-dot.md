---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
---

# Task unread dot system design

## Purpose and boundaries

The task status summary is the tasks system's bounded, per-task projection for
task lists. The unread dot is one more field of that projection, so it reaches
every task-list consumer, survives reloads, and works for tasks whose
transcripts the browser never loaded. Browser-only detection was rejected: the
client receives `session.message.added` only for sessions it subscribed to, and
it keeps no message history for other tasks.

This design uses, and does not change, the session read cursor, the
`mark-read` endpoint, and the monotonic cursor write owned by the
[unread-divider design](../../office/system-design/unread-divider.md). It
changes which message the client acknowledges (see
[Read acknowledgement](#read-acknowledgement)) and keeps the divider's
first-visit rule. It does not change the Quick Chat indicators.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-TASK-UNREAD-DOT-001` | [Unread predicate](#unread-predicate), [Ranks](#ranks), [Projection](#projection), [Read acknowledgement](#read-acknowledgement), [Restore and repair](#restore-and-repair), [Frontend](#frontend), [Failure and recovery](#failure-and-recovery), [Cost](#cost) |

## Unread predicate

A message qualifies as unread-capable when all of these hold:

- It was not written by the user (`author_type != "user"`). The user's own
  prompts never make a session unread, with or without a cursor.
- Its type, with an empty type normalized to `message` (as `newMessageEvent`
  does), is in the transcript-visible set (`VISIBLE_MESSAGE_TYPES` in
  `apps/web/hooks/processed-message-filtering.ts`: message, content, tool
  calls and variants, progress, status, error, thinking, todo,
  script_execution, agent_plan).
- It is not a setup-script row (`script_execution` with
  `metadata.script_type == "setup"`).
- It is not a subagent child (`metadata.parent_tool_call_id` set).
- It is not a `status` row whose content is `New session started` or
  `Session resumed`.
- It is not a recovery card (`metadata.recovery_actions == true`). The
  transcript keeps only the first card per stamp, so a later duplicate can never
  become the read cursor.
- It is not a footer-action row (`metadata.actions` is an array and
  `recovery_actions` is not set). `useProcessedMessages` moves these rows into
  `footerActionMessages`, outside the rendered items that supply the cursor, so
  reading can never advance the cursor to them. Git-failure, stall, and
  transient-retry notices are such rows.
- It is not a permission or clarification request. Those already drive the
  needs-attention affordance, and their visibility depends on sibling messages.

The canonical visible-type list lives in
`apps/packages/types/src/unread-message-visible-types.json`. The frontend
builds `VISIBLE_MESSAGE_TYPES` in
`apps/web/hooks/processed-message-filtering.ts` from that manifest; the
`@kandev/types` package exports the JSON path. The backend keeps a Go
membership set for the predicate, and its contract test loads the same
manifest and requires exact set equality. A frontend filtering test exercises
every manifest type as visible. This guards both additions and removals
without parsing source text.

The server does not mirror render-time filtering. Rows that the transcript
drops or collapses (for example boot-resume and empty-turn-notice
de-duplication, or a collapsed todo snapshot) can still qualify on the server.
[Read acknowledgement](#read-acknowledgement) makes this harmless: the client
acknowledges the newest loaded message, not the newest rendered one, so reading
a transcript to its end always moves the cursor past every row the server
counted.

## Ranks

Message order is the rank `(created_at, id)`, the same order
`UpdateTaskSessionLastReadMessageID` uses. Every rank, whether from an event
payload or a repository read, is normalized to UTC and truncated to
microseconds. Event payloads carry nanosecond `created_at`
(`newMessageEvent`), while Postgres `TIMESTAMPTZ` stores microseconds, so
untruncated ranks for the same message would compare unequal. Comparison also
treats `newestID == readID` as read before comparing times.

A session with a cursor is unread when its newest qualifying rank is greater
than the cursor rank. A session without a cursor has a zero read rank, so it is
unread as soon as any qualifying message exists. This is the notification
first-visit rule; the divider keeps its own rule and draws nothing on a
cursorless session's first visit.

## Projection

`TaskStatusSummary` in `apps/backend/internal/task/statussummary/model.go`
carries `UnreadSessionIDs []string` (`json:"unread_session_ids,omitempty"`):
the sorted IDs of sessions that are unread. A task is unread when the list is
non-empty. `SemanticEqual` and `SemanticJSON` include it, so a change bumps the
revision and publishes `TaskStatusSummaryUpdated` through the existing path.
Omitting the empty list keeps existing rows byte-identical. Session IDs, not a
boolean, let the open task's row exclude the session the user is reading while
still showing unread work in its other sessions. The list holds only session
IDs of the task, which the task-list consumer already receives, so it adds no
content and stays bounded by the task's session count.

Unread is derived from ranks, not toggled by events, so event arrival order
cannot change the result. `sessionObservation` (`projector.go`) carries the
cursor rank and the newest qualifying rank (`newestKnown`). The restore loader
sets `newestKnown` for every existing session, with or without a cursor. A
session first seen after restore becomes known when its first qualifying
message or cursor event loads its ranks. Any handled event persists the
summary when the derived list differs from the stored one, so a cold restore
whose ranks already include the triggering message still writes the corrected
value.

| Source | Effect |
| --- | --- |
| `events.MessageAdded`, qualifying | If `unreadObserved` is false, reload every session in the current session observation snapshot before folding the event; a successful load replaces all unread ranks and marks them observed. If observed but the session's ranks are unknown (a session created after restore), load that session first. Then raise the newest rank to the message's rank if greater. Idempotent, so a CAS replay is harmless. |
| New `events.TaskSessionReadCursorChanged` | Raise the cursor rank to the payload's rank if greater. When `unreadObserved` is false, the loader refreshes every session before the event is folded; otherwise an unknown newest rank is filled first. An unparseable timestamp ignores the event for unread. |
| Same event with `reset: true` | Replace the cursor rank with the payload's, even if lower, and reload the affected session's ranks (or every session when unread is unobserved), because the tracked newest rank may be a cursor-rank placeholder rather than a qualifying message. Published only by the e2e read-cursor route. |
| `events.MessageDeleted` of the tracked newest message | Reload that session's ranks through the loader below, or all sessions when unread is unobserved. |
| New subscription `events.SessionRemoved` | Evict that session from the known session set and reload unread ranks for all remaining sessions. Other observation fields keep their current behavior. |

The production cursor write is monotonic, so the cursor only rises. The e2e
route force-sets a cursor to rewind it (`handleE2ESetSessionReadCursor` is a raw
update that bypasses the guard), which is how specs seed "read up to an earlier
message" after a task was already viewed. The `reset` flag lets a warm projector
follow that rewind. The route needs the event bus that `registerE2EResetRoutes`
already holds, plus a session read and a message read for `task_id` and
`created_at`.

A message that arrives before or after its own cursor event converges to the
same state, so the sequence "mark read, then the late message event" cannot
re-arm the dot.

`MarkSessionRead` in `apps/backend/internal/task/service/service_sessions.go`
publishes `TaskSessionReadCursorChanged` after every successful write, with
`task_id`, `session_id`, and the persisted `last_read_message_id` and its
`last_read_created_at`, read back after the write. The recompute is idempotent,
so no change detection is needed. A stale request republishes the persisted
cursor, which changes nothing. The event is added to the projector's
subscription list in `Projector.Start`. No new WebSocket action is needed:
clients already receive the summary replacement.

The projector owns no repository handle; every database input is an injected
`ProjectorConfig` loader. A new loader, `LoadSessionUnreadState`, wired in
`apps/backend/internal/backendapp/gateway.go` next to
`LoadSessionObservations`, returns the ranks for requested sessions.
The projector calls it in `handleEvent` before folding qualifying
`MessageAdded`, cursor events, or a deletion of the tracked newest message.
When `unreadObserved` is false, a qualifying `MessageAdded` or any cursor event
requests every session in the current session observation snapshot, so a
successful read can restore task-wide observation; when true, it requests only
the affected session. `SessionRemoved` first evicts the session from the known
session set, then requests unread state for every remaining session. A failed
read leaves `unreadObserved` false and preserves the aggregate baseline.
`applySourceEventLocked` stays synchronous and context-free and only folds
already-loaded values. A CAS rebase calls `restoreSessionObservations`, which
reads ranks again for every session of the task; the replayed event is then
folded without another read.

## Read acknowledgement

Server eligibility and client acknowledgement share one boundary: the newest
message the client has loaded for the displayed session. `useSessionReadTracking`
receives that message from the raw session message list
(`newestAcknowledgeableMessageId` in `lib/session-unread-divider.ts`), not the
newest rendered item. The synthetic `task-description` placeholder is skipped
because it never exists as a stored row. Footer-action rows are skipped too:
they never qualify, and a transient-retry notice is deleted when its retry ends,
which would leave a cursor with no rank. Every message the server can count is
stored, delivered, and loaded before it can render, so acknowledging the newest
remaining row always moves the cursor past every qualifying row, including rows
the transcript collapses or hides.

The divider keeps rendering from rendered items. Because the cursor can now
point at an unrendered row, `dividerCursorForRenderedItems` maps it to the
newest rendered message at or before it in raw order before
`findUnreadDividerItemId` runs. When nothing rendered precedes it, the cursor is
passed through unchanged, which `findUnreadDividerItemId` already treats as
"the whole loaded window is unread". A cursor outside the loaded window keeps
that existing behavior.

## Restore and repair

One repository read, `ListSessionUnreadState(ctx, sessionIDs)`, returns for each
requested session that still exists its cursor rank (zero when unset) and its
newest qualifying rank. Sessions that no longer exist are absent, which is how
the projector distinguishes a removed session from a cursorless one. List loads,
restores, and repairs request every session of the task. A cursor event
normally requests the one session it needs; when task-wide observation is
unknown, it requests all session IDs in the observation snapshot instead.

For a cursor-bearing session the newest rank is the newest qualifying rank
after the cursor, or the cursor rank itself when nothing qualifies after it. For
a cursorless session it is the newest qualifying rank in the session, or the
zero rank when nothing qualifies. `newestKnown` is always true. All requested
sessions share one batched tail query per parameter chunk: each session reads at
most one page of its newest rows, bounded below by its cursor when it has a
live one. Only a session whose tail fills a page without a qualifying row falls
back to the paged backwards walk. A cursor whose message was deleted has no
rank, so its walk has no lower bound and every message counts as after it,
matching the cursor write guard. Both queries first exclude rows that can
never qualify: `author_type = 'user'` and types outside
`statussummary.UnreadCandidateMessageTypes()` (the visible set plus the empty
type). That filter is a necessary condition of the predicate, so it never drops
a qualifying row, and a long run of user prompts or hidden rows stays inside
one query instead of being paged into Go. The full predicate is the Go
function above, applied to the loaded pages. Its metadata checks are not
expressed in SQL: `dialect.JSONExtract` extracts text and cannot test
array-ness, and it encodes booleans differently per dialect.

- **Restore after restart or rebase:** `RebuildSession` carries the ranks.
  `loadTaskSessionObservations` fills them, and `restoreSessionObservations`
  copies them into `sessionObservation`. `applySummaryBaseline` carries only the
  stored aggregate, which is used solely while the unread read is unobserved
  (see Failure and recovery); it cannot supply per-session ranks. A nil loader
  does not authorize deriving unread from empty ranks: preserve the stored
  aggregate as an unobserved baseline. The field is omitted only when no
  authoritative baseline or loader exists.
- **Missing rows:** `rebuildMissingSummaries` fills the aggregate from the same
  read.
- **Existing rows:** `ReconcileTaskStatusSummaries` repairs pending action,
  activity, launch queue, completion gate, and the unread aggregate for rows
  that already exist (`reconcileExistingSummary`, `summaryNeedsReconcile`). It
  reads unread state
  through a narrow `SessionUnreadStateRepository` interface that the service
  type-asserts on its session repository, the same pattern as
  `completionGateSummary`'s `repository.TaskCompletionGateRepository`; no setter
  or extra wiring is needed. A repository that does not implement it projects no
  unread field.
  `summaryNeedsReconcile` compares the batch result with the row's aggregate, so
  tasks that were unread before an upgrade and tasks whose event was missed
  repair on the next list load. The read runs once per list load for the listed
  tasks. When the repair's CAS write is rejected, the retry re-reads the unread
  value for that task (as it already reloads the row, pending action, and launch
  queue). Otherwise a stale `true` computed before a concurrent mark-read could
  overwrite the projector's newer `false` on an idle task, with no later event to
  correct it.

## Frontend

`TaskStatusSummary` in `apps/web/lib/types/task-status-summary.ts` gains
`unread_session_ids?: string[]`. The WS handler stores the replacement as it
already does; `sidebarQuerySummaryChanged` is unaffected because the field does
not alter sidebar query results.

Both row builders map the list onto `TaskSwitcherItem.unreadSessionIds`
(`task-switcher-types.ts`) through `statusSummaryUnreadSessionIds`, which
returns `undefined` for an empty list: `sidebarStatus` in
`components/task/task-session-sidebar-item.ts` for the desktop sidebar and
`sheetStatus` in `components/task/mobile/session-task-switcher-sheet-item.ts`
for the mobile switcher. The two builders never apply the user-local rules.

The shared row in `components/task/task-switcher-row.tsx` is the only place
that decides display, because both surfaces render through it. The setting is
the portable user preference `unread_task_dot` (`userSettings.unreadTaskDot`),
default false, edited by `UnreadTaskDotSettings` directly under the New divider
row. It enters through the task-switcher prop chain, not a store read inside a
row: desktop and mobile task-list owners read `useUnreadTaskDotDisplay`
(`hooks/domains/sidebar/use-unread-task-dot-display.ts`) and supply
`unreadMessagesEnabled`, `unreadDotHideDelayMs`, and `displayedSessionId` to
`TaskSwitcherProps`; `buildTaskRowProps` copies them into `TaskRowBaseProps`,
and `task-switcher-tree.tsx` forwards the base props to `TaskRow`. This
explicit chain includes `TaskSwitcherProps`, its row-prop builder, the tree row
props, and the desktop/mobile callers. No intermediate layer reads the settings
or silently defaults them.

`shouldShowUnreadDot` (`components/task/task-unread-dot-visibility.ts`) is the
display rule:

```text
showDot = enabled                                     // AC .1, .3, .4
          && (lingering                               // AC .5
              || (!isSelected && unreadSessionIds non-empty)
              || (isSelected && some id != displayedSessionId))  // AC .17
```

`isSelected` is the row's existing `task.id === activeTaskId || task.id ===
selectedTaskId`. The desktop sidebar passes its route-aware `highlightedTaskId`,
and the mobile switcher passes the store's `activeTaskId`. `displayedSessionId`
is the store's `tasks.activeSessionId`. A message for the open task's displayed
session never produces a visible dot; an unread session of the open task that
is not displayed keeps the dot.

`lingering` comes from `useUnreadDotLinger` (`components/task/use-unread-dot-linger.ts`).
It becomes true when the row turns selected while its dot is showing and the
hide delay is positive, and it ends after the delay or when the row is
deselected. The delay is the portable preference
`unread_task_dot_hide_delay_seconds` (`userSettings.unreadTaskDotHideDelaySeconds`),
default 2 and validated to 0..60 by the backend. Its settings row renders only
while the dot switch is on, and its help text states that it changes
appearance only. The owners convert it to `unreadDotHideDelayMs`. Lingering is
purely presentational: the read cursor still advances as soon as the
transcript is read, and a cursor write during the delay does not hide the dot
early.

`useSessionReadTracking` advances the cursor while `unreadDivider ||
unreadTaskDot` and the browser page is visible (`document.visibilityState`), and
returns a divider anchor only while `unreadDivider`. `TaskChatPanel` passes it
`newestAcknowledgeableMessageId(messages)`, the newest loaded message including
rows the transcript hides (AC .6), so reading a session to its end always
acknowledges every message the server can count. The divider anchor is mapped
onto the rendered items by `dividerCursorForRenderedItems` before placement. A
background tab or minimized window that still shows a task does not clear its
dot.
`TaskChatPanel` also tracks only a session owned by the task it is shown for
(`isSessionReadableInPanel`): a Dockview chat panel keeps its saved session
across task switches, and must not read another task's session. A
user who enables only the dot still clears dots by reading, and a
user who turns the divider off does not freeze the cursor behind an enabled
dot. With both settings off the cursor stops, and the dot gate hides the
stale flags that leaves behind.

`TaskItem` receives the row's computed `hasUnreadMessages` and `TaskItemLeadingBadges`
(`task-item-leading-badges.tsx`) renders `TaskUnreadDot` as the last badge,
after the title like the other badges. It follows the existing
`TaskAgentErrorIcon` pattern: a fixed-size in-flow `span` with `role="img"`
and `aria-label` from `task:taskHasUnreadMessages`. It adds no line, so row
height is unchanged. It occupies one badge slot at the end of the badge group,
so the title truncates slightly earlier. The dot color uses a theme token
distinct from the Quick Chat blue and emerald bubbles.

User-facing documentation changes with it: the dot has its own setting copy
(`settings:showUnreadDotOnTasks`, `settings:unreadTaskDotShort`,
`settings:unreadTaskDotInfo`) in seven locales, the divider copy no longer
mentions the dot, and `docs/public/configuration.md` and
`docs/public/operations.md` describe it.

## Failure and recovery

- A missed event leaves the dot stale until the next list-load reconcile
  repairs the existing row, or the next relevant event.
- The projector re-observes unread state on restore, list reconciliation, and
  `SessionRemoved`. While observation fails, it keeps the current aggregate
  baseline and does not claim the data is observed. A successful task-wide
  load replaces all session unread ranks, sets `unreadObserved=true`, and
  derives the aggregate from those ranks. A successful one-session load can
  update only that session; it cannot mark task-wide state observed.
- When `unreadObserved=false`, any event that requires unread refresh
  (qualifying `MessageAdded`, any cursor event, or deletion of the tracked
  newest message) reloads every session in the current session observation
  snapshot, not only the event's session. That snapshot is the authoritative
  session set restored for the task. On success, replace the complete
  per-session unread map and set `unreadObserved=true`; on failure, retain the
  aggregate baseline. When already observed, events may load only the affected
  session.
- `SessionRemoved` evicts that session from the known session set and triggers
  a task-wide unread reload rather than inferring it was the only contributor.
  On success, the remaining session set and ranks replace the old set,
  including removal. On failure, the prior unread list remains, including the
  removed session, because a partial read cannot prove which remaining sessions
  are unread. The displayed dot may therefore remain stale, even when the
  deleted session was its only contributor, until a later successful task-wide
  refresh. That refresh removes the dot if no remaining session is unread.
- A nil unread loader means the projector has no authoritative unread source.
  If a stored summary exists, preserve its unread session list as unobserved
  baseline; omit the field only when there is neither loader nor stored
  summary. A nil loader never empties a stored list.
- A failed unread read never blocks other summary fields or clears a true
  dot. The gateway loader returns no ranks and marks the snapshot's unread data
  unobserved (`SessionObservationSnapshot.UnreadObserved == false`) instead of
  failing the snapshot. `projectionState` gains `unreadObserved` and an unread
  baseline seeded in `applySummaryBaseline` from `summary.UnreadSessionIDs`, as
  active errors are seeded from `ActiveError`. While unread is unobserved,
  `deriveSummary` uses the baseline instead of the empty per-session ranks, so a
  cold restore or CAS rebase with a failed read keeps a non-empty list. The
  projector increments an explicit
  `task_status_summary_unread_load_failures_total` counter, because the existing
  handler-failure counter only counts errors returned from `handleEvent`.
- `LoadSessionUnreadState` calls in `handleEvent` may be task-wide (every
  session when unread is unobserved) or per-session (unknown newest rank,
  `reset`, or deletion of the tracked newest message after observation). On
  error they record the counter and skip only the unread change. They never
  return an error, so the same event's other effects, such as the
  pending-request clear on `MessageDeleted`, still apply.
- A deleted session stops contributing through `SessionRemoved`. A hidden
  session (`hide_panel`) still counts; it can be reopened from the session
  reopen menu, and reading it advances the cursor.
- The frontend and Go predicate share an exact type contract:
  `apps/packages/types/src/unread-message-visible-types.json` is consumed by
  frontend filtering, and the Go contract test requires the backend membership
  set to match it exactly. Both additions and removals fail validation until
  both classifiers agree.
- A task with no unread session projects no field.

## Persistence

No new table or column. The summary payload stored with the existing summary
row carries the optional `unread_session_ids` field. Rows written by earlier
builds decode with no unread sessions and are corrected by the list-load
reconcile. The cursor remains the only persisted read state.

## Cost

Without mitigation, a qualifying message in a viewed session adds its session
to the unread list and the following mark-read removes it: two summary writes
and two workspace-wide `task.status_summary.updated` broadcasts per message
for an actively viewed task, and a dot flicker in other windows.

The projector therefore holds a write whose only change is a newly unread
session (`unreadRiseOnly`) for `DefaultUnreadRiseHold`, 1.5 seconds, through
one per-task flush scheduled by `holdUnreadRiseLocked`
(`statussummary/projector_unread.go`). The flush re-derives the summary under
the task lock and writes only if it still differs from the stored one, so a
mark-read inside the window leaves nothing to write. Every other change,
including a session leaving the list and any non-unread field, writes the
complete derived summary immediately, held rise included; the later flush then
finds nothing to write. The hold never delays a dot clearing or another summary
field. Cursor writes are not held: `mark-read` persists the cursor
immediately. `Projector.Close` cancels pending flushes.

A throwaway SQLite harness through the real projector and repository measured
(one run each):

| Workload | Summary writes / broadcasts | Time |
| --- | --- | --- |
| 200 viewed messages, each marked read, no hold | 400 / 400 | 11.3 ms per message |
| Same, 1.5 s hold | 2 / 2 | 6.2 ms per message |
| 200 background messages, never read | 1 / 1 | 2.5-3.4 ms per message |
| List read: 40 sessions x 5000 rows, cursor 100 rows from the end | n/a | 60-67 ms |
| List read: same, no cursor, newest row qualifies | n/a | 56 ms |
| List read: same, no cursor, no row qualifies | n/a | 143-158 ms (4.1 s before the SQL candidate filter) |

The remaining slow shape is a long run of rows that pass the SQL candidate
filter but fail a metadata check, such as subagent children; those rows are
still paged into Go 64 at a time. Each CAS rebase and each list load reads
every session of the affected tasks once.

## Security

The field lists the task's own session IDs, which the summary already exposes
through `primary_session`. No message content, message ID, or count enters the
summary. Workspace scoping follows the existing summary event routing.

## Observability

The projector logs a debug line when a task's unread aggregate flips, carrying
`task_id` but no message content. `metrics_vars.go` gains two unlabeled
counters, `task_status_summary_unread_flips_total` and
`task_status_summary_unread_load_failures_total`, following the existing
unlabeled counters.

## Related decisions

- [ADR 0041: backend-owned portable user settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
