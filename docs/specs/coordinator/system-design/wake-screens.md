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

This design owns the wake message text and its transcript rendering, the autonomy read route, and the autonomy screens. Split out of [wake](wake.md) for size; the tables, admission, delivery and turn end stay there.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-005` | [Transcript](#transcript) |
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
with an empty title, and the turn still sends. Titles are quoted, truncated to 80 characters (an empty title renders
as `""`) and stripped of newlines; they are still board
content ([ADR residual](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md#residual-risk)).
The web transcript renderer recognises `metadata.coordinator_wake_turn_id`
and renders the message as the "Woken by N events" entry with an expandable
list and, when non-zero, the denied-permission count, using the turn row from
the autonomy read. Copy goes through `t()` in six locales.

## Autonomy read

`GET /api/v1/workspaces/:id/coordinators/:cid/autonomy` (`workspace.read`)
returns:

```json
{
  "autonomy_enabled": true,
  "admission": {"ok": false, "reason": "containment", "detail": "auth_enabled"},
  "pending_wakes": 3,
  "oldest_pending_at": "2026-09-29T09:00:00Z",
  "last_turn": {"id": "...", "started_at": "...", "outcome": "completed", "cost_subcents": 5100, "denied_permissions": 0, "stop_state": null},
  "containment": {"conditions": [{"name": "executor_isolated", "met": true, "detail": ""}]},
  "spend": {"window_subcents": 64000, "mean_daily_subcents_7d": 58000, "measurable": true, "degraded": false, "ceiling_subcents": 100000}
}
```

It runs `Admit` with `AdmitReadOnly` ([Admission](wake.md#admission)): check 2 calls
`containment.Check`, not `CheckForAdmission`, so a read never moves the
counter, the state-change log or its previous key
([containment](containment.md#observability)), and no other check writes. `admission` is present only when autonomy is on.
When `admission.reason` is `cooldown`, `admission` also carries `until`, the
newest settled turn row's `finished_at` plus 5 minutes (RFC 3339 UTC); it is
absent for every other reason.
`last_turn.stop_state` is `null`, or `"stop_failing"` per
[spend](spend.md#stopping).
`coordinator.updated` gains optional `autonomy_changed: true`, so clients
re-read. Each owner publishes it once, after its own commit and only when the
write changed a row: the wake recorder for an inserted wake
([recording](wake-recording.md)), delivery for a `delivered` transaction, a
`superseded` settle and a `send_failed` or `interrupted` settle, the turn-end
settle, and the autonomy PATCH that changes `autonomy_enabled` or the ceiling.
A publish failure is logged at warn and never fails or rolls back the write; the
next read or backstop pass converges.

## Screens

- `apps/web/app/coordinator/use-coordinator-inputs.ts` gains the autonomy
  input, same `{value, loadedAt, error}` shape, re-read on mount, Try again
  and `coordinator.updated` with `autonomy_changed`.
- The autonomy strip (UI-01) renders above the count strip while
  `autonomy_enabled`: "Autonomy: Active" or "Autonomy: Held (<reason text>)",
  "Last woke <age>" or "Not woken yet", "<n> pending", and the spend pill from
  [spend](spend.md#screens). A read error shows "Autonomy state unavailable"
  with Try again (`AC-COORDINATOR-WAKE-006.4`). While autonomy is off, the
  strip renders only when `last_turn.stop_state` is `"stop_failing"`, and
  then shows "Autonomy: Off" with the stop warning and Stop of
  [spend](spend.md#stopping), so a turn left open by turning autonomy off
  still surfaces a failing stop (`AC-COORDINATOR-SPEND-003.4`).
- `classify` in `apps/web/lib/coordinator/attention.ts` gains an optional
  `autonomy` input and emits one item of kind `autonomy` when the reason is in
  the persistent set of `AC-COORDINATOR-WAKE-006.2` and `pending_wakes > 0`,
  reference time `oldest_pending_at`, kind rank 4, id `autonomy:<cid>`. It
  counts in the coordinator's attention count only.
- Held reason texts: `containment` "Containment not in place: <condition>";
  `spend_unmeasured` "Spend cannot be measured"; `ceiling_reached` "Cost
  ceiling reached"; `no_conversation` "Open the copilot once to give it a
  conversation"; `conversation_unavailable` "The conversation stopped. Open
  the copilot to restart it". The item offers **Open settings** to managers.
- The two transient reasons are not holds on the strip. For
  `conversation_busy` the strip shows "Autonomy: Active (Waiting for the
  conversation)", and for `cooldown` "Autonomy: Active (Between turns until
  <time>)", where `<time>` is `admission.until` in the viewer's locale short
  time format. Neither offers **Open settings**, and neither produces an item
  (`AC-COORDINATOR-WAKE-006.2`). With `admission.ok` true the strip shows
  "Autonomy: Active" alone. Every other reason shows "Autonomy: Held (<reason
  text>)" from the catalog above.
