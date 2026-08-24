# ADR-2026-08-14-current-turn-clarification-ownership: Current Turn Owns Active Clarification

**Status:** accepted (amended 2026-08-15, 2026-08-24)
**Date:** 2026-08-14
**Area:** backend, frontend, protocol, workflow

## Context

Kandev deliberately keeps a clarification answerable when its MCP wait times out or disconnects.
The persisted question stays pending and receives `agent_disconnected=true`; a late answer can then
resume the same session through an event fallback.

Several consumers treated every historical pending row as operational. Turn completion repeatedly
detached and republished old bundles, the workflow guard scanned full session history, and a persisted
task summary could retain the resulting question indicator indefinitely. Other consumers already
limited pending actions to the latest surviving message turn. That boundary can move backward when
the final message of a newer turn is deleted even though its durable turn record remains. Those
conflicting definitions let an older request reappear after a newer request was rejected and let an
inert request block workflow transitions.

Task summaries also identify only the kind of pending action. A real question owned by a secondary
session can therefore make a task row correct while normal task navigation opens the primary session
and hides the question.

## Decision

The session's current turn owns active clarification state.

- A pending clarification is operational only when its message belongs to the session's newest durable
  **conversational** `task_session_turns` record — a turn whose `metadata.lifecycle_only` flag is not
  set. "Set" means exactly the value-set `{true, 1, "true", "1"}`; every other encoding — an absent
  key, `null`, `false`, `0`, `""`, `"false"`, `"0"`, any other string, an object, or an array — is
  conversational, matching `turnMetadataFlagPredicate`'s `IN ('true','1')` test on both dialects. That
  marker identifies a synthetic turn born already-completed by lifecycle bookkeeping (for example the
  agent-boot-on-resume record), not a turn representing genuine conversation; a lifecycle turn is
  excluded from current-turn resolution entirely, however recent. Among conversational turns,
  the newest is the one with no open successor: an open turn (`completed_at IS NULL`) always outranks
  a completed one regardless of timestamps, and only when both candidates are open or both are
  completed do `started_at`, then `created_at`, then `id` (all descending) break the tie. Missing
  status retains the existing legacy meaning of pending only after current-turn ownership matches.
- Deleting messages never moves current-turn ownership backward or reactivates a superseded bundle.
- Detachment preserves late-answer behavior only while that turn remains current. Once a newer turn is
  accepted, older pending rows become inert history without requiring a destructive rewrite.
- One repository derivation supplies the backend clarification guard, detach/expiry fallback,
  response validation, per-session pending projection, and task-summary reconciliation.
- Message and clarification events trigger a fresh authoritative projection. Event arrival order and
  one in-memory request identity per session do not define persisted pending state.
- Response handling follows the same ownership rule. An atomic current-turn claim precedes both live
  waiter delivery and detached resume publication. Terminal message updates become visible only after
  delivery succeeds; a failed detached publication returns an error and restores the bundle to pending
  when its turn is still current. Retryability is acknowledged only after the live projector durably
  refreshes the task summary from those restored rows; exhausted summary compare-and-set retries return
  an error instead of false acknowledgement. Message publication then converges other clients.
  Detached orchestrator acceptance uses a finite deadline. A detached current-turn rejection persists
  without resuming; any response to a superseded or terminal bundle returns conflict without a write or
  resume event.
- If historical per-row failures leave a current-turn bundle with both terminal and pending siblings,
  the response claim transitions only the pending rows and preserves terminal siblings. Failed delivery
  restores only rows owned by that claim.
- Repeated detachment of an already detached bundle performs no write and publishes no duplicate
  message occurrence.
- Existing task summaries reconcile their pending field on normal task-list and boot reads. Repair uses
  compare-and-set, preserves unrelated fields, advances revision only for a semantic change, and
  publishes the complete corrected replacement.
- Frontend transcript logic consumes the existing durable turn list when available and uses the
  server's per-session `pending_action` while turn history is unavailable. Task-row navigation selects
  the newest matching input-capable session before remembered or primary preferences.

This decision narrows the clarification hard-pause lifetime in
[ADR 0015](0015-explicit-completion-signal-for-auto-advance.md). A detached question remains a hard
barrier while its turn is current, not forever after unrelated later turns are accepted.
This ownership rule stands independently if ADR 0015 is later rejected, superseded, or amended again.

## Consequences

- Old pending metadata remains useful audit evidence but is not operational state by itself.
- Late answers remain supported across disconnects and restarts until the user or system starts newer
  work in that session.
- Workflow transitions cannot be blocked by an older-turn question.
- Sidebar and phone indicators converge from repository truth after restart without a database
  cleanup migration.
- A task-row question icon leads to the session that owns the question, including a non-primary
  session.
- Pending projection performs bounded database reads on pending-sensitive events and normal list/boot
  hydration. It avoids unbounded transcript delivery or inactive-session subscriptions.
- SQLite and PostgreSQL must select the same newest durable conversational turn; dialect-sensitive
  repository tests are required.

## Alternatives considered

### Treat every historical pending row as active

Rejected. It preserves unlimited late answers by allowing obsolete requests to reappear, emit fresh
events, and block workflow progress after newer turns.

### Expire every question immediately when its waiter disconnects

Rejected. A short MCP timeout or transport loss would discard required user input even when the user
has not moved the conversation forward.

### Rewrite old rows when every new turn starts

Rejected. It adds a mutation to every turn-entry path and changes transcript history to enforce a
rule that can be reconstructed from existing turn identity.

### Derive ownership from the latest surviving message

Rejected. Deleting the last message in a newer turn would roll the boundary backward and reactivate
older pending history even though the newer `task_session_turns` record remains durable.

### Add a dedicated clarification-state table

Rejected for this repair. Existing message status, bundle identity, and turn identity contain the
needed authority. A new table would add migration and dual-write failure modes without changing the
product rule.

### Put the owning session in the task summary

Rejected. Task session list responses already expose bounded per-session `pending_action`. Loading
that existing list only when a pending task is activated keeps the task summary constant-size and
avoids another identity contract.
