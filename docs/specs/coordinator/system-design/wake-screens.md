---
id: coordinator-wake-screens-design
title: Wake transcript, autonomy read and screens design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-WAKE-005
  - REQ-COORDINATOR-WAKE-006
---

# Wake transcript, autonomy read and screens design System Design

## Purpose and boundaries

This design owns the wake message text and its transcript rendering, the run read route, the autonomy read route, and the autonomy screens. Split out of [wake](wake.md) for size; the tables, admission, delivery and turn end stay there.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-005` | [Transcript](#transcript), [Run read](#run-read) |
| `REQ-COORDINATOR-WAKE-006` | [Autonomy read](#autonomy-read), [Screens](#screens) |

## Transcript

The message body is agent-facing English, not UI copy:

```text
Unattended turn. No person started this turn or is watching it.
Events since your last turn (N):
- question on <identifier> "<title, 80 characters>"
- stall on <identifier> "<title>"
These were current when this turn started and may have changed since; read
current state before acting. Propose what should happen. Proposals wait for a
manager unless one has allowed automatic creation of tasks.
```

The message carries no orders, goal, context or tool names: the turn has the
conversation's instructions and bound tool list
([integration](integration.md#instructions-orders-and-goal)).

N equals the turn's `wake_count`, and the list has exactly the rows read back
at step 3, oldest first. Step 4 reads each wake's task for the identifier and
title; a task that cannot be read (deleted since step 2, or a failed read)
is listed by the wake's `task_id` (the UUID) in place of the identifier,
with an empty title, and the turn still sends. The line's kind word is `question` for a
`question` wake and `stall` for a `stall` wake. A title is truncated to its first 80 runes (no
ellipsis), each newline or tab is replaced by a single space, each `"` is replaced by `'`
so the quoting stays intact, and an empty title renders as `""`; titles are still board
content ([ADR residual](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md#residual-risk)).
The web transcript renderer recognises `metadata.coordinator_wake_turn_id`
(a string, the `coordinator_unattended_turns.id`, written once with the
message and never changed; a message with the key absent, not a string or
empty is an ordinary message) and renders the message as the "Woken by N
events" entry. It never parses the message body: N, the list and the denied
count come from the [Run read](#run-read) of that turn id, and the
coordinator id for the call is the `coordinator_id` metadata of the
conversation task (the key the phase 1 conversation code reads with
`conversationTaskCoordinatorID`). Entry states:

- **Loaded (200).** Header "Woken by <wake_count> events" (`wokenByEvents`,
  `count` with `_one`/`_other`: "Woken by 1 event"), the denied count "<n> permission
  denied" / "<n> permissions denied" only when `denied_permissions` is above
  0, and a list toggle. The list is collapsed by default, is a `button` with
  `aria-expanded` operated by Enter and Space, and each row reads
  "<kind word> <identifier> <title>" from the run's `wakes` in their order,
  the identifier falling back to the `task_id` and the title to empty, as the
  message body does. The expanded state is component state only. `wake_count`
  is the header number even when `wakes` holds fewer rows (retention deletes
  wake rows at 30 days and turn rows at 90).
- **Loading.** A one-line skeleton until the first read settles.
- **Unavailable (404, any other error, or no coordinator id).** The entry
  shows "Woken by events" (`wokenByUnknown`) with no count, the list toggle
  revealing the message body as plain text, and no retry. The message text is
  the only surviving record of a pruned run.

The run read is issued once per distinct turn id per coordinator while the
page is mounted (a per-page map keyed by turn id, so a transcript with many
entries reads each once and the copilot panel, popover and task view share
it). An entry whose run has `outcome` null, or whose first read is still in flight, is re-read on each
`coordinator.updated` carrying `autonomy_changed` (an event during an in-flight read starts one more read and the sequence counter drops the older response); a settled run is never
re-read. A re-read that fails (404 or any error) keeps the last loaded data on
screen and changes nothing; only the first read falls to the Unavailable
state. The map is a module-level store keyed by coordinator id and turn id,
shared by every mounted surface and emptied on a full page load. The loaded
rows show the run read's title as received, without the 80-rune, newline and
quote transform of the message body (that transform is for the agent-facing
text only), truncated by CSS to the row width. A stale response for an earlier read of the same turn id is dropped
by a per-key sequence counter. Copy goes through `t()` in six locales.

## Run read

`GET /api/v1/workspaces/:id/coordinators/:cid/runs/:runId` (`workspace.read`;
registered only while phase 3 is effective, like the other phase 3 routes, so
otherwise it answers as an unregistered route). `:runId` is a
`coordinator_unattended_turns.id`. Response:

```json
{
  "id": "...", "coordinator_id": "...", "conversation_task_id": "...", "session_id": "...",
  "started_at": "2026-09-29T09:00:00Z", "finished_at": null, "outcome": null,
  "wake_count": 2, "denied_permissions": 0, "cost_subcents": null,
  "stop_requested_at": null, "stop_state": null,
  "wakes": [{"id": "...", "kind": "question", "task_id": "...", "task_identifier": "KAN-418", "task_title": "..."}]
}
```

- `outcome`, `finished_at`, `cost_subcents` and `stop_requested_at` are `null`
  while unset (`outcome` and `finished_at` are null exactly while the turn is
  open; `cost_subcents` is also null for a settled turn whose cost read has not
  yet succeeded). `stop_state` is `null` or `"stop_failing"` by the rule of the
  [autonomy read](#autonomy-read). Times are RFC 3339 UTC.
- `wakes` are the `coordinator_wakes` rows with `turn_id = :runId`, ordered
  `created_at` asc then `id` asc (the order the wake message lists them), with
  `kind` the row's kind word. `task_identifier` and `task_title` are read from
  the task now (the full title, not truncated) and are `null` when the task
  cannot be read; the client then falls back to `task_id`. `wakes` may hold
  fewer rows than `wake_count`, and is `[]` when retention deleted them.
- 404 `run_not_found` for an id that names no row, a row of another
  coordinator or another workspace, or a pruned row; the three cases are not
  distinguishable. 404 `coordinator_not_found` for an unknown coordinator id
  or one outside `:id`. A failed store read is 500 `read_error` with no
  partial body. The route is a read: it takes no lock and writes nothing, and
  two concurrent reads may straddle a settle, each returning a whole row
  from one read.
- `last_turn` of the autonomy read is this same object without `wakes`.

## Autonomy read

`GET /api/v1/workspaces/:id/coordinators/:cid/autonomy` (`workspace.read`;
registered only while phase 3 is effective) returns:

```json
{
  "server_time": "2026-09-29T09:20:00Z",
  "autonomy_enabled": true,
  "admission": {"ok": false, "reason": "containment", "detail": "auth_enabled"},
  "pending_wakes": 3,
  "oldest_pending_at": "2026-09-29T09:00:00Z",
  "last_woke_at": "2026-09-29T07:00:00Z",
  "last_turn": {"id": "...", "conversation_task_id": "...", "session_id": "...", "started_at": "...", "finished_at": "...", "outcome": "completed", "cost_subcents": 5100, "denied_permissions": 0, "wake_count": 2, "stop_requested_at": null, "stop_state": null},
  "containment": {"conditions": [{"name": "executor_isolated", "met": true, "detail": ""}]},
  "spend": {"measurable": true, "degraded": false, "window_subcents": 64000, "mean_daily_subcents_7d": 58000, "mean_known": true, "ceiling_subcents": 100000}
}
```

- **Route errors.** 404 `coordinator_not_found` for an unknown coordinator id
  or one outside `:id`. A failed store read for any field other than `spend`, or an admission
  whose `detail` is `read_error` (a store read inside `Admit` failed; the
  reason beside it is not shown), returns 500 `read_error` and no partial body.
  A spend that cannot be measured is not a read error: it is the hold
  `spend_unmeasured` with an empty detail, and an unreadable containment
  condition is the hold `containment` with detail naming it. The `spend`
  block is exempt: a failed or unmeasurable spend read is answered 200 by the
  three-row table below, never 500.
  `containment.Check` never errors (an unreadable condition is `met: false`
  with detail `unreadable`), so it is never a route error.
- **`server_time`** is the service clock at the read, RFC 3339 UTC. Clients
  compute every deadline as a difference against it, never against their own
  clock, so client skew changes nothing.
- **`autonomy_enabled`** is the coordinator row's value read by the route
  before `Admit`. `admission` is present only when it is true. If `Admit` then
  answers `autonomy_off` with an empty detail (a PATCH turned autonomy off
  between the two reads), the route answers with `autonomy_enabled` false and
  no `admission`; `autonomy_off` is never sent in `admission.reason`, and an
  `Admit` detail of `coordinator_not_found` is the 404 above.
- **`admission`** is the `Admit` result under `AdmitReadOnly`
  ([Admission](wake.md#admission)): check 2 calls `containment.Check`, not
  `CheckForAdmission`, so a read never moves the counter, the state-change log
  or its previous key ([containment](containment.md#observability)), and no
  other check writes. `reason` is one of the closed set of the
  [held texts](#screens); `detail` is the `Admit` detail (`""`, a condition
  name for `containment`, `session_not_started` for `conversation_unavailable`,
  `turn_open` for `conversation_busy`). When `reason` is `cooldown`,
  `admission` also carries `until`, the newest settled turn row's `finished_at`
  plus 5 minutes (RFC 3339 UTC); it is absent for every other reason.
- **Pending fields.** `pending_wakes` and `oldest_pending_at` come from one
  query over the coordinator's `pending` wakes, so they agree:
  `oldest_pending_at` is `null` exactly when `pending_wakes` is 0, else the
  minimum `created_at`.
- **`last_turn`** is the newest `coordinator_unattended_turns` row of the
  coordinator by `started_at` desc then `id` desc, whatever its outcome
  (including `send_failed` and `interrupted`), or `null` when there is none.
  Its shape is the [run read](#run-read) object without `wakes`; `outcome`,
  `finished_at`, `cost_subcents` and `stop_requested_at` are `null` while
  unset. `stop_state` is `"stop_failing"` exactly when `finished_at` is null
  (the turn is open), `stop_requested_at` is set and `server_time` minus
  `stop_requested_at` is strictly more than 5 minutes; a difference of exactly
  5 minutes is `null`. It is `null` otherwise.
- **`last_woke_at`** is the `started_at` of the newest turn row whose
  `session_turn_id` is set (the message was accepted), or `null`. It differs
  from `last_turn.started_at` only when the newest row is a `send_failed` turn
  that never woke the conversation; the strip's "Last woke" reads this field.
- **`spend`** is one wire shape for the three readings of
  [spend](spend.md#measurement); `ceiling_subcents` is the coordinator's
  ceiling in every case (`null` when unset) and `degraded`/`measurable` are
  always present:

  | Reading | `measurable` | `degraded` | `window_subcents` | `mean_daily_subcents_7d` | `mean_known` |
  | --- | --- | --- | --- | --- | --- |
  | Measurable | true | false | the sum | the mean | the reading's `Mean7dKnown` |
  | Unpriced row in the window (nil error) | false | true | `null` (the lower bound is never sent) | the mean | the reading's `Mean7dKnown` |
  | Failed read or `ErrSpendScope` | false | false | `null` | `null` | false |

  `mean_daily_subcents_7d` is `null` whenever `mean_known` is false, so a failed
  seven-day read is never sent as zero.
- `coordinator.updated` gains optional `autonomy_changed: true`, so clients
  re-read. Each owner publishes it once, after its own commit and only when the
  write changed a row: the wake recorder for an inserted wake
  ([recording](wake-recording.md)), delivery for a `delivered` transaction, the
  supersede pass of [step 2](wake.md#delivery) (once per delivery, after its last
  statement that changed a row) and a `send_failed` or `interrupted` settle
  ([settle rule](wake-recovery.md#settle-rule)), the turn-end
  settle, the ceiling-stop mark of [spend](spend.md#stopping) (step 6, only
  when it set `stop_requested_at` on a row), and the autonomy PATCH that
  changes `autonomy_enabled` or the ceiling.
  A publish failure is logged at warn and never fails or rolls back the write; the
  next read or backstop pass converges. Events can be duplicated, coalesced or
  reordered, so a client treats each as "re-read", never as data.
- **Reads are not a snapshot.** The fields are read one after another with no
  shared transaction; a concurrent wake, settle or PATCH can make two fields
  disagree for one response, and the next `autonomy_changed` read converges.
  The only agreement guaranteed within one response is the pending pair above.

## Screens

- **Input.** `apps/web/app/coordinator/use-coordinator-inputs.ts` gains the
  autonomy input, same `{value, loadedAt, error}` shape, re-read on mount, Try
  again and `coordinator.updated` with `autonomy_changed`, while phase 3 is
  effective and the viewed coordinator is resolved. Each read carries a
  sequence number and the viewed coordinator id; a response for an older
  sequence or another coordinator is dropped, so overlapping reads and a
  coordinator switch resolve to the latest read. **Timed re-reads:** after each
  successful read the client schedules one re-read at the earliest of
  `stop_requested_at + 5 minutes + 1 second` (only for an open turn with
  `stop_state` null) and `admission.until + 1 second` (only for `cooldown`),
  each converted to a delay against `server_time`, replacing any earlier timer
  and cleared on unmount or coordinator switch. The delay is at least 5
  seconds (a computed delay below that waits 5 seconds), so a response that
  still reports the same state cannot loop faster than one read per 5 seconds,
  and a read triggered by a timer does not re-arm a timer for a deadline that
  has not moved. A page that opened while the turn ran learns of the stop mark
  from the `autonomy_changed` the mark publishes
  ([spend](spend.md#stopping) step 6); timers and events are the only
  refresh. "Last woke <age>" and "Between turns until <time>" are computed
  against the latest read's `server_time` plus the time elapsed since its
  `loadedAt`, and refresh on the phase 1 30-second age timer. Spend amounts
  and a `ceiling_reached` hold refresh only on events and reads; usage ageing
  out of the window publishes nothing, and that staleness is accepted.
- **Loading and error.** While the first read has not settled and no error is
  set, the strip is not rendered and reserves no space. While `error` is
  true the strip shows "Autonomy state unavailable" with Try again whatever
  stale `value` is held, and the `autonomy` item is not emitted (`classify`
  receives no autonomy input); Needs you keeps every other item
  (`AC-COORDINATOR-WAKE-006.4`).
- **Strip (UI-01)** above the count strip while `autonomy_enabled`:
  - Line 1: "Autonomy: Active", "Autonomy: Active (<transient text>)" or
    "Autonomy: Held (<reason text>)", then "Last woke <age>" from
    `last_woke_at` or "Not woken yet" when null, then "<n> pending"
    (`autonomyPending`, plural), then the spend pill of
    [spend](spend.md#screens).
  - A separate warning row "Stop at ceiling not confirmed: the turn is still
    running" with **Stop** shows whenever `last_turn.stop_state` is
    `"stop_failing"`, beneath line 1 and independent of `admission`. So a held
    coordinator with a failing stop shows both rows.
  - **Stop** is shown to a user who can manage the workspace and calls the
    same session-cancel action the copilot panel's Stop uses, with
    `last_turn.session_id`. It is disabled while its request is in flight (a
    second press does nothing). On success the row stays until the next read
    shows the turn settled; on any failure (including a cancel already in
    flight) an inline "Could not stop the turn. Try again." (`autonomyStopFailed`)
    shows beside Stop and re-enables it. A user who cannot manage sees the
    warning without Stop. Stop is not fenced against a stale page: if the
    unattended turn ended and a later turn owns the session, the press cancels
    that turn; this is accepted, the same as the copilot panel's Stop.
  - While autonomy is off the strip renders only when `stop_state` is
    `"stop_failing"`, and then shows "Autonomy: Off" plus the warning row with
    Stop, and no pending, spend or last-woke text (`AC-COORDINATOR-SPEND-003.4`).
- **Item.** `classify` in `apps/web/lib/coordinator/attention.ts` gains an
  optional `autonomy` input and emits one item of kind `autonomy` when
  `admission.reason` is in the persistent set of `AC-COORDINATOR-WAKE-006.2`
  and `pending_wakes > 0`, reference time `oldest_pending_at` (the read time
  `loadedAt` if it is unexpectedly null), kind rank 4, id `autonomy:<cid>`.
  It counts in the coordinator's attention count only, and that count is the
  one the decision toast's "Next" reads: `computeNeedsYouCount` passes the
  latest autonomy input to `classify` like the screen does, so the toast never
  says nothing else is waiting while the item is on screen. Its title is the
  held reason text; its "Why it is here" line is `autonomyWhy` "<n> events are
  waiting for the coordinator" (`count` with `_one`/`_other`, from
  `pending_wakes`; there is no wake kind list on the item); its detail is the
  fix text below; **Open settings** (to the
  Autonomy section) is rendered only for a user who can manage the workspace,
  and is absent, not disabled, for anyone else.
- **Held reason texts:** `containment` "Containment not in place: <condition>"
  where `<condition>` is the display label of the [settings
  display](containment.md#settings-display) for `admission.detail` (the raw
  detail when it names no known condition); `spend_unmeasured` "Spend cannot be
  measured"; `ceiling_reached` "Cost ceiling reached"; `no_conversation` "Open
  the copilot once to give it a conversation"; `conversation_unavailable` "The
  conversation stopped. Open the copilot to restart it", except that detail
  `session_not_started` reads "The conversation has not started. Open the
  copilot to start it". **What clears it** (the item's detail line):
  `containment` the fix text of the [settings
  display](containment.md#settings-display) for the condition named by
  `admission.detail`, found in `containment.conditions` by name: the
  condition's own `detail` override (`changed_since_launch`, `unreadable`,
  `unverified_source`) replaces its fix line exactly as settings does, and when
  `containment.conditions` holds no entry for the name, the title still uses the
  display label if the client knows the name (the raw name only when it does
  not) and the item has no detail line;
  `spend_unmeasured`
  "Some usage is unpriced or could not be read. It clears when the 24-hour
  window no longer holds it."; `ceiling_reached` "Spend in the last 24 hours
  is at the ceiling. Raise the ceiling or wait for spend to age out.";
  `no_conversation` and `conversation_unavailable` the text above.
- **Transient reasons.** They are not holds on the strip. For
  `conversation_busy` the strip shows "Autonomy: Active (Waiting for the
  conversation)", and for `cooldown` "Autonomy: Active (Between turns until
  <time>)", where `<time>` is `admission.until` in the viewer's locale short
  time format. Neither offers **Open settings**, and neither produces an item
  (`AC-COORDINATOR-WAKE-006.2`). With `admission.ok` true the strip shows
  "Autonomy: Active" alone. Every other reason shows "Autonomy: Held (<reason
  text>)" from the catalog above.
- **Ceiling absent in the read.** If `autonomy_enabled` is true but
  `ceiling_subcents` is `null` (a PATCH landed between the reads), the spend
  text is the amount alone, "Spend <window> USD in 24 h", with no pill and no
  "of <ceiling>"; the next read converges.
- **No ceiling.** The strip never shows a "No ceiling" state: autonomy cannot
  be on without a ceiling (`AC-COORDINATOR-SPEND-001.2`). "No ceiling" appears
  only in the settings section, where autonomy may be off.
