---
id: coordinator-improvements-design
title: Improvement proposals design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-IMPROVEMENTS-001
  - REQ-COORDINATOR-IMPROVEMENTS-002
  - REQ-COORDINATOR-IMPROVEMENTS-003
  - REQ-COORDINATOR-INTEGRATION-003
  - REQ-COORDINATOR-INTEGRATION-006
---

# Improvement proposals System Design

## Purpose and boundaries

An improvement is a second proposal kind in the existing proposal store. Its
approve branch stores a pending change instead of creating a task, and a
manager applies that change through the coordinator settings' existing context
edit. The evidence it cites is the unattended turn rows of
[wake](wake.md#store). Nothing here lets the coordinator write its own
configuration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-IMPROVEMENTS-001` | [Store](#store), [Tool](#tool) |
| `REQ-COORDINATOR-IMPROVEMENTS-002` | [Card](#card) |
| `REQ-COORDINATOR-IMPROVEMENTS-003` | [Approve](#approve), [Pending changes](#pending-changes) |

## Store

Phase 2's `coordinator_proposals.kind` column (default `create_task`) carries
the value `improvement`, registered as a `KindExecutor` in the phase 2 kinds
registry ([integration](integration.md#proposal-statuses-and-kinds)); this
document adds no column to that table. For an improvement, `spec_json` is:

```json
{
  "title": "...",
  "rationale": "...",
  "context_before": "...",
  "context_after": "...",
  "evidence": [{"run_id": "..."}, {"task_id": "..."}]
}
```

`final_spec_json` is written at claim as for tasks, equal to `spec_json`.
The open-proposal count, the 25 limit, `status=pending` and `status=all`
listing, `coordinator.updated`, stale-claim recovery and the reply route of
[relay](relay.md#reply-route) are kind-agnostic and unchanged.

`coordinator_pending_changes`:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | indexed with `status, created_at` |
| `proposal_id` | text not null unique | one change per approved improvement |
| `field` | text not null | `context` |
| `base_value` | text not null | `context_before` |
| `new_value` | text not null | `context_after` |
| `status` | text not null | `pending`, `applied`, `discarded` |
| `decided_by` | text null | user id for apply or discard |
| `created_at`, `updated_at` | timestamp not null | |

Deleted with the coordinator and on `workspace.deleted`.

## Tool

`propose_improvement_kandev` is registered on the coordinator surface only
while phase 3 is effective, added to the exact-name auto-approve list and the
guard's allowed coordinator actions, and absent from every other surface. It
enters a conversation's bound list through the `phase3` input of
`ToolNames`, evaluated when the conversation is opened, so a conversation
opened before phase 3 was on has no such tool
(`AC-COORDINATOR-INTEGRATION-003.3`).
Arguments: `title`, `rationale`, `context`, `evidence` (array of objects with
exactly one of `run_id` or `task_id`). There is no `in_reply_to`: a reply to
an improvement is delivered with text that asks for a new improvement
([relay](relay.md#reply-delivery)), and the improvement card never shows
"Revised after your reply".

Validation, in order, each failure refusing the call naming the field and
storing nothing (`AC-COORDINATOR-IMPROVEMENTS-001.2`):

1. `title` trimmed, 1 to 60 characters.
2. `rationale` at most 10,000 characters.
3. `context` through the phase 1 coordinator context validation (trimmed, at
   most 4,000 characters); equal to the current trimmed context is refused
   naming `context`.
4. `evidence` has 1 to 10 entries; each `run_id` names a
   `coordinator_unattended_turns` row of this coordinator; each `task_id`
   names a task in the coordinator's workspace; at least one entry is a
   `run_id`.
5. The open-proposal limit, with the phase 1 counting transaction.

On success the row is inserted with `kind = 'improvement'`, `status =
'pending'` and `context_before` read in the same transaction as the limit
count, and `coordinator.updated` is published. The tool returns
`{proposal_id, status}`. The coordinator's system prompt (phase 1
`prompt.go`) gains one paragraph describing the tool and that approval
applies nothing.

## Card

`ProposalCard` branches on `kind`. The improvement branch
(`apps/web/app/coordinator/components/improvement-card.tsx`):

- Header "Improvement", title, rationale, and the pill "Changes coordinator
  context".
- "Runs behind it": each `run_id` resolved through
  `GET .../coordinators/:cid/runs/:runId` ([run read](wake-screens.md#run-read), `workspace.read`, 404 `run_not_found` when pruned)
  shows start time, outcome and cost; a 404 shows "Run record expired". Each
  `task_id` shows the task's identifier and title from the workflow
  snapshots, or "Task no longer available".
- **Show the change** reveals a line diff of `context_before` and
  `context_after` using the existing diff viewer component; the card records
  that it was shown in component state.
- For managers on a `pending` improvement (a `failed` one keeps Approve and Reject):
  **Approve as a reviewable change**, disabled with the hint "Show the change
  first" until the diff has been shown in this card instance; **Reject**; and
  **Reply with a condition** ([relay](relay.md#cards)). No **Edit**.
- An `approved` improvement shows "Approved as a reviewable change. Nothing
  was applied." with **Open settings**.

The approve route refuses edits for an improvement with 400 naming `edits`.

## Approve

The phase 1 approve route branches on `kind` after the claim:

1. Claim as for tasks (`pending` or `failed` to `approving`, token, frozen
   spec), through the kinds registry.
2. Instead of the task create, insert the `coordinator_pending_changes` row
   with `INSERT ... ON CONFLICT (proposal_id) DO NOTHING`, then complete the
   proposal `approved` with `task_id` null, fenced by the claim token.
3. The improvement executor reports `ReRunsOnStaleClaim() = false`, because
   the built reclaim path refuses to re-run a kind that reports true. A claim
   left `approving` past the stale window is settled `failed` with the reason
   `outcome_unknown` and no second execution
   (`AC-COORDINATOR-INTEGRATION-006.5`); a manager's Approve on that `failed`
   card claims it again and re-runs step 2, which the unique `proposal_id`
   makes idempotent.

An improvement has no per-action policy class among the six. Its activity
rows use the activity-only class `improvement` and the guard's policy
re-check returns nil for it, so a task-creation setting never governs it
([integration](integration.md#proposal-statuses-and-kinds)).

The coordinator's configuration is not touched
(`AC-COORDINATOR-IMPROVEMENTS-003.1`). The automatic path of
[automatic](automatic.md#automatic-approval) is only in the task tool, so an
improvement is never approved automatically
(`AC-COORDINATOR-IMPROVEMENTS-002.3`).

## Pending changes

Routes under `/api/v1/workspaces/:id/coordinators/:cid/`, phase 3 only:

| Route | Scope | Result |
| --- | --- | --- |
| `GET pending-changes` | `workspace.read` | `{changes: [...]}` with `status = 'pending'`, `created_at` asc then id |
| `POST pending-changes/:chid/apply` | `workspace.manage` | the coordinator, 404, 409 |
| `POST pending-changes/:chid/discard` | `workspace.manage` | the change, 404, 409 |

Apply runs in one transaction: read the change (404), require `pending`
(409 with the change otherwise), read the coordinator row, compare its
trimmed `context` with `base_value` (409 with `{"reason": "context_changed"}`
when they differ, change left `pending`), then perform the phase 1 PATCH
write of `context = new_value`, which clears `conversation_task_id` and
increments `config_revision` because the context changed. That write is
conditional: its `UPDATE coordinators ... WHERE id = ? AND context = ?` binds
`base_value`, so a manager's context PATCH that commits between the read and
the write makes it change zero rows, and Apply rolls back and returns the same
409 `context_changed` rather than overwriting the edit. Finally it sets the
change `applied` with `WHERE id = ? AND status = 'pending'`, rolling back with
409 when that changes zero rows. After commit it archives
the old conversation exactly as a PATCH does. Discard is the conditional
update to `discarded`. Concurrent applies or discards settle once; the loser
gets 409 (`AC-COORDINATOR-IMPROVEMENTS-003.3`).

The settings page for one coordinator lists pending changes under "Changes
waiting for you" with the diff, **Apply** and **Discard**; a 409
`context_changed` shows "The context changed since this was proposed.
Discard it, or ask the coordinator to propose again."

## Security

- The apply, discard and run-read routes are refused to a coordinator
  principal by the guard; the coordinator's surface has only the propose tool.
- `context_before` is captured server-side; the tool cannot supply it.
- The evidence validation stops a coordinator citing another coordinator's
  runs or another workspace's tasks.

## Observability

`coordinator_improvement_total{event}` with `proposed`, `approved`,
`applied`, `discarded`, `apply_conflict`, and an info log per apply with the
coordinator id and proposal id.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
