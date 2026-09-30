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
While phase 3 is not effective, `kindFilter` also hides `kind = 'improvement'`
rows: it takes the phase 3 flag beside phase 2, and every query that uses it
(count, list and the single read behind the decision routes) excludes those
rows, so they are not listed, not counted and not decidable (approve, reject and
reply answer an absent proposal, 404). The stale-claim sweep, the startup
recovery, the read that completes a claim already held and the fallback read
of `readForApprove` (the second read it makes with the literal `true` when
the filtered read misses) keep passing the literal `true` phase 2 passes
today. So a claim left `approving` on an improvement settles
`failed`/`outcome_unknown` whatever the flag, and an Approve whose claim is
held when phase 3 turns off still completes. A manager's Approve on a hidden
improvement that is `approving` goes through that fallback exactly as it does
for a hidden phase 2 kind: a claim that is not yet stale answers 409 with the
proposal, a stale one is settled `failed`/`outcome_unknown` and the settled
proposal is returned, as for a phase 2 kind. An Approve on a hidden
improvement in any other status answers 404, as do reject and reply for every
status. The builder extends the existing fallback condition from "phase 2 off"
to "phase 2 off or phase 3 off"; all of this touches only rows the fence
already owns. `registerKinds` stays unconditional, so a row that reappears with phase
3 is recovered like any other. A `coordinator_pending_changes` row is
unaffected by the flag, but its routes are unregistered while phase 3 is off
(see [Pending changes](#pending-changes)). Turning phase 3 back on shows every
improvement with the status it had.

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

`propose_improvement_kandev` is offered on the coordinator surface only
while phase 3 is effective (its entry in a conversation's bound list is
conditional, below), added to the exact-name auto-approve list and the
guard's allowed coordinator actions, and absent from every other surface. The
action stays in the guard's dispatch whatever the flag, so a call from a
conversation opened while phase 3 was on reaches guard check 0 and gets the
phase 1 unknown-action error rather than a transport-level unknown-tool
error. It
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
2. `rationale` trimmed, 1 to 10,000 characters (counted in runes, as the phase
   1 fields are); the trimmed value is stored.
3. `context` through the phase 1 coordinator context validation (trimmed, at
   most 4,000 characters); the trimmed value is stored. An empty trimmed value
   is refused naming `context` (an improvement replaces the instructions with
   text; blanking them is a manager's edit). A value equal to the context stored at that moment is refused naming
   `context`; the comparison uses the value read inside the limit transaction
   (step 5's `context_before`), so a PATCH landing between validation and
   insert cannot leave a proposal whose before equals its after. That read
   happens after step 4 and the limit check, so this refusal is made last: a
   call that has an equal context and also invalid evidence names `evidence`,
   and one over the limit answers the limit error; only a call valid in every
   other respect is refused naming `context` here.
4. `evidence` is an array of 1 to 10 objects, in the order given, stored in
   that order. Each object has exactly one key, `run_id` or `task_id`, with a
   non-empty string value; any other shape (both keys, neither, an extra key,
   a non-string or empty value, a non-object entry) is refused naming
   `evidence`. A `run_id` or `task_id` that repeats another entry's is refused
   naming `evidence`. Each `run_id` names a `coordinator_unattended_turns` row
   of this coordinator, open or settled (the call is normally made during the
   open turn, which is a valid citation); a run of another coordinator or one
   that does not exist is refused naming `evidence`, the same message for both.
   Each `task_id` names a task in the coordinator's workspace, of any state,
   archived included and watched or not (the Watches filter narrows what the
   coordinator reads, not what it may cite); one outside the workspace or
   absent is refused naming `evidence`, the same message for both. At least
   one entry is a `run_id`, else `evidence` is refused.
5. The open-proposal limit, with the phase 1 counting transaction.

The guard's reference and watch checks read top-level id fields only, so the
nested evidence ids are checked here and nowhere else.

On success, in one transaction, the row is inserted with `kind =
'improvement'`, `status = 'pending'`, `target_task_id` null and
`context_before` read in the same transaction as the limit count; the
`proposed` activity row is written with class `improvement`, the coordinator
principal's actor and `unattended_turn_id` stamped by
[the resolver](integration.md#log-rows); and `coordinator.updated` is published
after commit. The tool returns `{proposal_id, status}`. Two calls made at once
each pass or fail the limit on their own count under the transaction's lock;
neither is deduplicated, because a second identical call is a second proposal
(the equal-context check compares with the stored context, not with other
proposals). The coordinator's system prompt (phase 1 `prompt.go`) gains one
paragraph describing the tool and that approval applies nothing.

**Guard.** The tool's action is registered in the guard's allowed coordinator
actions and in `coordinatorPrincipalOnlyActions`. It is not a policy propose
action: `ProposeActionFor` does not return it, so guard check 3 (the policy
`Allows` test), which would otherwise be always false for a class outside
`AllActions`, does not apply, and `actionClass` maps it to `ActionImprovement`
for the refusal rows of checks 1 and 2. While phase 3 is not effective the
guard answers the phase 1 unknown-action error before check 1, as its check 0
does, so no bound-list read and no activity row happens
([integration](integration.md#tool-list)).

## Card

`ProposalCard` branches on `kind`. The improvement branch
(`apps/web/app/coordinator/proposal-card/improvement-card.tsx`, beside
`proposal-card.tsx` and `kind-body.tsx`):

- Header "Improvement", title, rationale, and the pill "Changes coordinator
  context".
- "Runs behind it", in the order stored. Each `run_id` is resolved through
  `GET .../coordinators/:cid/runs/:runId` ([run read](wake-screens.md#run-read),
  `workspace.read`) and shows start time, outcome and cost as that read
  returns them: a null `outcome` reads "In progress", and a null
  `cost_subcents` omits the cost. A 404 `run_not_found` shows "Run record
  expired"; any other failure of the read shows "Run unavailable" for that
  entry only, and the rest of the card renders. Each `task_id` shows the task's
  identifier and title from the workflow snapshots, or "Task no longer
  available" when the task is not in them. Entries load independently; the
  order shown is the stored order whatever the completion order of the reads.
- **Show the change** reveals a line diff of `context_before` and
  `context_after` rendered by one small shared component,
  `apps/web/components/coordinators/context-diff.tsx`, built on the existing
  `lineDiff` and `DiffLine` of `components/task/task-plan-diff.ts` (the file
  diff viewers need file context and the plan dialog is bound to revisions, so
  neither fits); the card and the settings rows both use it. Its left side is
  labelled "Context when proposed", not "current", because the coordinator's
  context may have changed since; the two labels are new copy in six locales. The card records that it was shown in component state.
- For managers on a `pending` improvement (a `failed` one shows only
  **Approve as a reviewable change** and **Reject**, with no **Reply**, because
  the reply route answers 409 for `failed`; the diff gate applies to its Approve
  as to a pending one): **Approve as a reviewable change**, disabled with the hint "Show the
  change first" until the diff has been shown in this card instance; **Reject**;
  and **Reply with a condition** ([relay](relay.md#cards)). No **Edit**. The gate
  is a review aid on the client; the approve route does not require proof that
  the diff was shown, and needs `workspace.manage` as for every proposal. A
  reader sees the same evidence and diff and no decision controls. Whether the
  base is already stale at approval time is not checked: the Apply 409 is the
  signal.
- An `approved` improvement shows one line by the state of its change, read
  from `change_status` on the proposal DTO: `pending` (or null, when no change
  row exists) "Approved as a reviewable change. Nothing was applied." with
  **Open settings**; `applied` "Approved as a reviewable change. Applied to the
  context."; `discarded` "Approved as a reviewable change. Discarded, nothing
  was applied." **Open settings** goes to
  `/settings/workspaces/:id/coordinators/:cid?section=autonomy` (the plural
  path and `section` query the goal note builds, `goal-note.tsx`), which opens
  the Autonomy tab; the "Changes waiting for you" section is inside it, and the
  page scrolls it into view once its list has loaded. `change_status` is an improvement-only DTO field
  read with a left join on `coordinator_pending_changes.proposal_id`; it is
  absent for every other kind.

The approve route refuses edits for an improvement with 400 naming `edits`.

## Approve

The phase 1 approve route branches on `kind` after the claim:

1. Claim as for tasks (`pending` or `failed` to `approving`, token, frozen
   spec), through the kinds registry.
2. Instead of the task create, insert the `coordinator_pending_changes` row
   with `INSERT ... ON CONFLICT (proposal_id) DO NOTHING` (in `Execute`, its
   own statement), then complete the proposal `approved` with `task_id` null,
   fenced by the claim token, through `completeKind` and the same
   `settleDecision` transaction that writes the `approved` activity row.
3. The improvement executor reports `ReRunsOnStaleClaim() = false`, because
   the built reclaim path refuses to re-run a kind that reports true. A claim
   left `approving` past the stale window is settled `failed` with the reason
   `outcome_unknown` and no second execution
   (`AC-COORDINATOR-INTEGRATION-006.5`); a manager's Approve on that `failed`
   card claims it again and re-runs step 2, which the unique `proposal_id`
   makes idempotent.

Step 2's insert and its completion are separate transactions, so a crash or a
lost fence between them leaves a change row whose proposal is not `approved`.
That row is inert: every read and write of a change requires its proposal to
be `approved` ([Pending changes](#pending-changes)), so a stale sweep that
settles the proposal `failed`, or a Reject of that `failed` proposal, leaves a
row nobody can list or apply. A later Approve finds the row by `proposal_id`
(the insert does nothing) and its completion makes the row visible. The only
way the row is ever non-`pending` is an Apply or Discard, which needs the
proposal `approved`, so a retried insert never meets a settled row.

The approve, fail and reject activity rows of an improvement carry class
`improvement`, taken from the executor's `Action()`, not the `create_task`
class `completeProposalStore` writes today and not the `unknown` class
`rejectProposalStore` writes for a non-policy kind; both take the class from
`kindAction(kind)`. The approved row has a null target task and detail
`approvedDetail`. An improvement has no per-action policy class among the six.
The guard's policy re-check returns nil for it, so a task-creation setting
never governs it ([integration](integration.md#proposal-statuses-and-kinds)).

The coordinator's configuration is not touched
(`AC-COORDINATOR-IMPROVEMENTS-003.1`). The automatic path of
[automatic](automatic.md#automatic-approval) is only in the task tool, so an
improvement is never approved automatically
(`AC-COORDINATOR-IMPROVEMENTS-002.3`). A repeated Approve of an `approved`
improvement, or one whose claim another caller holds, gets the kind-agnostic
409 with the current proposal, and writes and stores nothing more.

## Pending changes

Routes under `/api/v1/workspaces/:id/coordinators/:cid/`, registered only while
phase 3 is effective (otherwise unregistered routes, as for the
[run read](wake-screens.md#run-read)). Every change lookup is
`WHERE id = ? AND coordinator_id = ?` under a coordinator that belongs to
`:id`, and requires the change's proposal to be `approved`; a miss on any of
these is 404 `change_not_found`, indistinguishable, and a failed store read is
500 `read_error` with nothing changed.

| Route | Scope | Result |
| --- | --- | --- |
| `GET pending-changes` | `workspace.read` | `{"changes": [...]}`, always an array (`[]` when none) of changes with `status = 'pending'` whose proposal is `approved`, `created_at` asc then `id` asc |
| `POST pending-changes/:chid/apply` | `workspace.manage` | 200 the coordinator; 404; 409; 400 |
| `POST pending-changes/:chid/discard` | `workspace.manage` | 200 the change (`status` `discarded`); 404; 409 |

A change is serialized as `{id, coordinator_id, proposal_id, proposal_title,
field, base_value, new_value, status, decided_by, created_at, updated_at}`,
with `proposal_title` read from the proposal's `spec_json` and `decided_by` null
until settled. The list is read in one query and is a snapshot: a change
settling while it is read appears or not, whole.

Apply takes the same per-coordinator write lock a PATCH takes (SQLite BEGIN
IMMEDIATE, PostgreSQL `SELECT ... FOR UPDATE` on the coordinator row) before it
reads anything, through a store method that shares `patchCoordinatorBody`'s
locking, and performs, in that one transaction:

1. read the change and its proposal's status (404 as above);
2. require `status = 'pending'`, else 409 with the change as it now is (so a
   repeated Apply after an Apply is 409 carrying `status: "applied"`, and the
   caller can tell its own earlier success from a concurrent discard);
3. re-validate `new_value` with the phase 1 context validation (400 naming
   `context`, change left `pending`);
4. read the coordinator row and compare its stored `context` with `base_value`;
   when they differ answer 409 with `{"reason": "context_changed"}` and leave
   the change `pending`;
5. run the phase 1 PATCH validator on the merged row, which resolves the agent
   profile: every refusal is returned as the PATCH returns it (400 naming the field:
   `agent_profile_id`, `executor_profile_id`, or the autonomy interlock's
   field), the change staying `pending`;
6. perform the phase 1 PATCH write of `context = new_value`, which clears
   `conversation_task_id` and increments `config_revision` because the context
   changed, with the added guard `AND context = ?` bound to `base_value`; a
   write that changes zero rows rolls back and answers the same 409
   `context_changed`;
7. set the change `applied` and `decided_by` (the deciding user's id, null when
   the principal has none) with `WHERE id = ? AND status = 'pending'`, rolling
   back with 409 and the current change when that changes zero rows.

Because the lock is taken first, a manager's context PATCH cannot commit
between steps 4 and 6: it either committed before Apply took the lock (Apply
then answers 409 `context_changed` and the edit stays) or commits after Apply
and applies on top of it. Neither write is lost. The guard in step 6 is defence
in depth and is tested at the store level with a base that does not match.
After commit Apply archives the old conversation through the same
`onConversationCleared` callback a PATCH uses (best effort, a failure is logged
and the response stays 200, exactly as a PATCH), starts no turn on the new
conversation, and publishes `coordinator.updated`. Applying one change makes
every other pending change with the same base answer 409 `context_changed`,
which the manager discards; there is no bulk action and no ordering rule beyond
the list order.

Discard is one conditional update to `discarded` (`decided_by` set) with
`WHERE id = ? AND coordinator_id = ? AND status = 'pending'` and the proposal
guard; when it changes no row the change is re-read once, answering 404 when
it is absent or its proposal is not `approved`, and 409 with the change
otherwise. It publishes `coordinator.updated` after commit. Concurrent applies
or discards settle once; the loser gets 409 with the change
(`AC-COORDINATOR-IMPROVEMENTS-003.3`). Neither writes an activity row: the
`approved` row records the decision, and the change's own state is its status.
Applied and discarded changes are not returned by the list route (Discard and a 409 return the change they settled), and are kept until
the coordinator or workspace is deleted. While phase 3 is off a pending change
can be neither applied nor discarded, because its routes are unregistered.

The settings page for one coordinator lists pending changes under "Changes
waiting for you", loaded from the list route: a loading skeleton, "No changes
waiting for you." when empty, and "Could not load the changes. Try again." with
a retry control when the read fails. Each row shows the title, the diff of
`base_value` and `new_value`, and **Apply** and **Discard** for managers; a
reader sees the row and the diff and neither control. After Apply or Discard
succeeds, or answers 409 or 404, the list is refetched and the row disappears
if it is no longer pending; any other failure keeps the row and shows the
server's message. A 409 `context_changed` shows "The context changed since this
was proposed. Discard it, or ask the coordinator to propose again." A 409
without that reason shows "This change was already settled." The section
follows the list order and keeps it after a refetch; a lower `created_at`
change is never moved. After a successful Apply the page refetches the coordinator and sets the
context field of the Identity form's draft and of its saved baseline to the
fetched context, discarding an unsaved context edit there (the draft was written
against the base Apply just replaced). Unsaved edits to the name and the two
profile fields are kept, and the page's dirty state then reflects only those.
Apply adds no confirmation copy of its own: the row leaving the list and the
Identity context showing the applied text are the confirmation, so this design
adds no locale key for it (the error and 409 messages above are the only new
copy). A page that does not hold the Identity form needs no reset. It is rendered in the phase 3 Autonomy body of settings
([integration](integration.md#settings-layout)), only while phase 3 is
effective.

## Security

- The apply, discard and run-read routes are refused to a coordinator
  principal by the guard; the coordinator's surface has only the propose tool.
- `context_before` is captured server-side; the tool cannot supply it.
- The evidence validation stops a coordinator citing another coordinator's
  runs or another workspace's tasks.

## Observability

`coordinator_improvement_total{event}` with `proposed`, `approved`, `applied`,
`discarded`, `apply_conflict`, and an info log per apply with the coordinator
id and proposal id. `apply_conflict` counts a 409 `context_changed`, from step
4 or from the guarded write of step 6; a 409 for an already-settled change is
not counted. `approved` counts a settled Approve, so a retried Approve of a
`failed` improvement counts once when it completes. The label set is closed;
no coordinator, proposal or change id is ever a label.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
