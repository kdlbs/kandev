---
id: coordinator-shadow-dream-design
title: Coordinator shadow dream design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-SHADOW-DREAM-001
  - REQ-COORDINATOR-SHADOW-DREAM-002
  - REQ-COORDINATOR-SHADOW-DREAM-003
  - REQ-COORDINATOR-SHADOW-DREAM-004
  - REQ-COORDINATOR-SHADOW-DREAM-005
  - REQ-COORDINATOR-SHADOW-DREAM-006
---

# Coordinator shadow dream System Design

## Purpose and boundaries

The dream is a system-defined bounded episode that reviews recorded turns,
outcomes and overrides and reports suggested changes. In phase 3.1 it is
Shadow: it has no tool that writes, and its output is a stored report no turn
reads. It starts only from the trigger of `001.1`, checked on the wake backstop
pass, under phase 3's admission, containment and spend ceiling. It is designed
so the later Active dream reuses the trigger, collection, extraction, gate and
replay unchanged and adds only an apply path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-SHADOW-DREAM-001` | [Trigger and lease](#trigger-and-lease) |
| `REQ-COORDINATOR-SHADOW-DREAM-002` | [The episode](#the-episode), [Opening message](#opening-message) |
| `REQ-COORDINATOR-SHADOW-DREAM-003` | [Answer and report](#answer-and-report) |
| `REQ-COORDINATOR-SHADOW-DREAM-004` | [Gate and replay](#gate-and-replay) |
| `REQ-COORDINATOR-SHADOW-DREAM-005` | [Learning section](#learning-section), [Routes](#routes) |
| `REQ-COORDINATOR-SHADOW-DREAM-006` | [Ratings](#ratings), [Health](#health) |

## Tables

`coordinators` gains `shadow_dream_enabled` (bool, default false).

`coordinator_dreams`: `id`, `coordinator_id`, `status` (`running`, `ok`,
`clean`, `partial`, `failed`, `skipped`), `reason`, `window_start`,
`window_end`, `input_hash`, `turn_ids` (JSON), `considered` (JSON, labelled
agent-reported on read), `model`, `cost_micros` (nullable), `started_at`,
`refreshed_at`, `finished_at`. Index on `(coordinator_id, started_at, id)`.

`coordinator_dream_items`: `id`, `dream_id`, `position`, `kind`, `text`,
`cited_turn_ids` (JSON), `gate` (`pass` or the refusal reason), `replay_id`
(nullable), `replay_verdict`. Unique `(dream_id, position)`.

`coordinator_dream_ratings`: `item_id`, `user_id`, `rating`, `rated_at`; primary
key `(item_id, user_id)`.

## Trigger and lease

`dream.Scheduler.Tick(coordinatorID)` runs at the end of each wake backstop pass
(it adds a call in the pass, never a second timer). It evaluates in order and
stops at the first failing condition, storing no row and reporting that
condition to the health computation (`001.6`): flag effective; autonomy on;
`shadow_dream_enabled`; not paused (paused first among admission-like
conditions); containment check passes; spend measurable and below ceiling; at
least 24 hours since the last dream started; then evidence: at least 5 completed
non-dream turns and at least one decided proposal or override in the window
(`001.1`).

The window is `(prev accepted window_end, or max(now-30d, first ledger row))` to
`now`, computed once and stored (`001.2`). Turns with trigger `dream` are
excluded from the window and every count (`001.5`).

**Lease.** The insert of the `running` row is conditional: `INSERT INTO
coordinator_dreams ... SELECT ... WHERE NOT EXISTS (SELECT 1 FROM coordinator_dreams
WHERE coordinator_id = ? AND status = 'running' AND refreshed_at > now - 5m)`.
Zero rows inserted means another dream holds the lease and the tick ends. The
running episode refreshes `refreshed_at` every minute. The backstop expires a
`running` row past 5 minutes: `UPDATE ... SET status = 'failed', reason =
'lease_lost', finished_at = ? WHERE id = ? AND status = 'running'`, cancels the
episode if active, and leaves the window (the next window starts from the last
accepted dream) (`001.3`). A completion arriving afterwards updates with `WHERE
status = 'running'`, matches nothing and changes nothing.

**Input hash.** SHA-256 of the projected evidence bytes, the prompt version
constant and the model the profile resolves to. An accepted row with the same
hash makes the tick store one `skipped`/`unchanged` row, at most once per hash
(a unique partial index on `(coordinator_id, input_hash)` for skipped rows)
(`001.4`).

## The episode

The episode is an ephemeral coordinator-origin session in the coordinator's
workspace, created through the same runner and origin attribution as
unattended turns of phase 3, so the containment check
([containment](containment.md#req-coordinator-containment-001-the-containment-check))
and the spend reader and `CancelTurn` treat it as coordinator spend
([spend](spend.md#req-coordinator-spend-003-stopping-at-the-ceiling)) (`002.3`). It
uses the coordinator's agent profile and never touches the conversation task,
its reference or its queue (`002.1`).

**Tool profile.** The session is opened with an explicit profile
`dreamProfile = {list_coordinator_turns_kandev}` defined as a constant in the
dream package, not derived from `ActionSettings` (`002.2`). The guard rejects
any other action as unknown before policy evaluation and writes no activity
row. A test asserts the constant's length and content, and asserts a guarded
call of each registered action from that session is refused.

**One turn, bounded.** One prompt, a 20-minute wall clock; at expiry the system
cancels the session and sets the row `failed`, reason `timeout` (`002.4`). The
ceiling stop uses the phase 3 `CancelTurn` path and sets `failed` with the
reason `ceiling`. Pause during a running episode cancels it and sets `failed`,
reason `paused` ([pause](pause.md#pause-and-the-episode)). The ledger records the
turn with trigger `dream` and the model stamp; the session is archived on end
and nothing is delivered to the conversation or Needs you (`002.7`). The only
starter is `Scheduler.Tick`; no tool, route, wake or delivery calls it (`002.6`).

## Opening message

`dream.Evidence.Build(window)` reads stored rows only: per turn its trigger,
verdict and call counts by action; per decided proposal kind, decision, edited
field names, reason code, task result, cost and title truncated to 80
characters, all wrapped as data (`002.5`). It caps at 200 turns (newest
first by `(started_at, id)`) and 60,000 characters, dropping oldest turns first,
and records which turn ids it included as `turn_ids`. It never reads message
text, descriptions, child-task conversations or intake text. The episode's
one tool is the ledger tool, which returns digests, never text.

## Answer and report

The agent must answer with one JSON object `{items: [...], considered: [...]}`.
`parse.Answer` rejects unknown keys, more than 10 items, more than 20 considered
entries or an unknown item kind: status `failed`, reason `bad_output`, nothing
stored (`003.1`). Item kinds: `note_add`, `note_update`, `note_retire`,
`context_diff`, `standing_order_add`, `standing_order_retire`. Text is at
most 500 characters; a `context_diff` at most the coordinator context limit.

After the gate and replays, one transaction stores the items, replay links,
considered list, model, cost and final status: `ok` (no items), `clean` (every
item passed gate and was replayed), `partial` (any refused or not measured),
`failed` (bad output, timeout, lease loss, ceiling stop, run error) (`003.2`,
`003.4`). Items keep refused entries with their reason (`003.3`). A row that is
not `running` never changes except by ratings and retention (`003.4`). Reports
are kept 400 days and deleted with the coordinator.

## Gate and replay

`gate.Check(item, ctx)` is pure and deterministic: no model call, first failing
reason in this order (`004.2`, `004.3`): `size`, `thin_evidence` (fewer than 2
distinct cited turn ids), `citation_unresolved` (a cited turn missing, of another
coordinator, outside the window or a dream turn), `credential` (containment's
credential patterns), `forbidden_target` (permissions, watches, autonomy,
ceiling, tool profile or the human-written context named as the thing to
change), `no_target` (every `note_update` and `note_retire` in phase 3.1),
`bad_target` (standing order retired or not the coordinator's).

Up to 5 passing items, in report order, are replayed as candidates through
[replay](replay.md) (context text applied to the current configuration); the
sixth onwards, or any whose budget ran out, is `unmeasured` (`004.1`). The
replay's budget uses the same spend reservations, so a replay-heavy dream stops
at the ceiling like anything else.

No item is written to context, standing orders, notes, settings or any store a
turn reads; the dream package imports no writer of those, verified by the same
import-boundary test style as replay (`004.4`). Standing-order and context
items are described as they would apply and never change `config_revision`
(`004.5`).

## Ratings

`PUT /coordinators/:id/dreams/:dreamId/items/:itemId/rating` with
`{rating}` in `useful`, `not_useful`, `harmful` (manager only, else 403; an item
not of the named dream is 404). It upserts `(item_id, user_id)`; the last commit
wins (`006.1`, `006.2`). An item's rating is the newest `rated_at` across managers,
ties by user id ascending. The agreement measure counts an item only with a
rating and a replay verdict in `blocked`, `improvement`, `not_an_improvement`; it
agrees when `improvement` meets `useful`, or `blocked` or `not_an_improvement`
meets `not_useful` or `harmful` (`006.4`).

## Health

`dream.Health(coordinatorID)` is a pure function of the stored rows and the same
admission conditions the scheduler uses: `off` (not enabled), `running`,
`waiting` (with the failing condition and its fix), `fresh` (last accepted within
36 hours, or the evidence debt not older than 36 hours), `stale` (debt for more
than 36 hours without an accepted dream), `failed` (more than 72 hours or last
dream `failed`) (`006.3`). Fixes: "Turn autonomy on", "Resume", "Raise the
ceiling". An unreadable input gives no health state and the section shows the
failure, never `fresh`.

## Routes

All flag-gated (404 otherwise) and workspace-member readable unless stated:

- `GET /coordinators/:id/learning`: switch, health, measures reference.
- `PUT /coordinators/:id/learning` `{shadow_dream_enabled}`: manager only;
  saved on its own request, no conversation archive, no `policy_revision`
  change (it is not policy); publishes `coordinator.updated`.
- `GET /coordinators/:id/dreams?before=&limit=`: 20 per page, `(started_at,
  id)` cursor.
- `GET /coordinators/:id/dreams/:dreamId`: report with items, replay verdicts,
  ratings, and the considered list.
- `PUT .../items/:itemId/rating`.

A coordinator principal is refused on all of them with 403 (`005.1`).

## Learning section

A new section `learning` on the coordinator settings page (sections after
Watches). It shows: the statement "Shadow changes nothing", the switch (disabled
with the reason while the workspace's coordinator list has not loaded; turning
it on while autonomy is off is accepted and the health reads "Waiting: autonomy is
off") (`005.2`), the health line with its fix, the five measures
([outcomes](outcomes.md#screens)), and the report list newest first with status,
window, item count and cost, "No dreams yet" when empty, and a failure state
that never says "No dreams yet" (`005.3`, `005.6`).

The report detail shows the window, turns read, each item with cited turns, gate
result, replay verdict and the rating control, the considered list marked
"Reported by the agent, not verified", and a "Retire or supersede" section that
is shown even with no item. It never offers to apply, approve or copy an item
(`005.4`). On a phone the section is a single-column stack of cards with 44 px
touch targets, the detail is a full-screen page and the rating is a segmented
control (`005.5`). Copy is added in six locales; no em dash.

## Error handling

| Failure | Behaviour |
| --- | --- |
| Evidence read fails | No episode, no row, health shows failure |
| Runner errors | Row `failed`, reason `run_error` |
| Lease refresh fails | Episode continues; expiry follows |
| Report transaction fails | Row `failed`, reason `store_error`; window unchanged |

## Testing

Trigger matrix with each admission condition, the lease under two schedulers on
PostgreSQL, expiry, unchanged skip once, the dream profile constant and refusal
of every other action, parse limits, gate order table, replay cap of 5, rating
upsert and tie, health thresholds at 36 and 72 hours, retention, and the
Learning section including phone layout.
