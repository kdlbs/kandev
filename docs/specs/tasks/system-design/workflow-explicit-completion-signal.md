---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
---

# Completion signal recovery system design

## Purpose and boundaries

The tasks system owns completion eligibility and its recovery instructions.
This design owns turn attribution, entry-bound signal claims, and recovery
instructions. The issue #4705 extension below is implemented; the associated work
orders record validation. Existing diagnostics and advancement policy remain.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001 | Applicability response; Durable output-turn attribution |
| REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003 | Diagnostic response; Entry-bound completion; Verification |

## Applicability response (`advances`/`note`)

`accepted:true` means the signal was durably recorded — it does not mean the
step will actually advance. A step whose `auto_advance_requires_signal` is
`false` never reads the recorded signal at turn end, so a caller could accept a
signal that changes nothing and have no way to tell from the response alone
(this is exactly what happened in ISSUE-5: an agent woken on a non-auto-start
step called `step_complete_kandev`, got `accepted:true`, and nothing moved).

`internal/mcp/handlers/handlers.go:handleStepComplete` resolves the calling
turn's step through the existing `h.workflowCtrl.GetStep` call already used
for the stale-turn diagnostic above, and additively sets two response fields
before returning:

- `advances` (bool) — `true` only when the step's `AutoAdvanceRequiresSignal`
  is set and `WorkflowStep.AdvancesOnTurnComplete()` reports a move the
  engine runs on turn completion.
- `note` (string) — present only when `advances` is `false`, explaining that
  the step does not advance on a completion signal, or, for a signal-gated
  step, that it has no `on_turn_complete` move that runs automatically.

`internal/workflow/models.WorkflowStep.AdvancesOnTurnComplete` mirrors
`internal/workflow/engine.compileOnTurnComplete` and `evaluateActions`:
`move_to_next` and `move_to_previous` count, `move_to_step` counts only with a
non-empty `step_id` that differs from the current step, and a move marked
`requires_approval` does not count because neither engine nor legacy turn
completion runs it. An unguarded self-target is selected first and blocks later
actions, but the engine does not transition to the current step. A guarded
self-target can fall through to later actions when its guard is not satisfied.
`disable_plan_mode` changes session settings and never moves the task. A move
behind a `wait_for_quorum` guard counts, because quorum re-evaluation can still
apply it. The field describes configuration, not a promise: a clarification
barrier, an unsatisfied guard, or a last-step `move_to_next` can still leave the
task in place.

A signal-gated step without such a move still accepts and records the signal.
The turn-end gate finds the signal, the engine selects no transition, and the
session waits for input. The bag entry stays until a new user turn or a
clarification pause clears it, or until the task leaves the step and the entry
becomes stale. A signal on a non-gated step is handled the same way today: it
is recorded and does not drive a transition. Rejecting the call instead was
considered and not chosen. It would change the `accepted` contract that
signal-gated prompts rely on, and it would answer the same "this signal will
not move the task" situation with an error for gated steps and a success for
non-gated steps.

`accepted` is never changed by this — ADR 0015 semantics and existing agent
prompts depend on it staying `true` once the signal is recorded. When the step
lookup itself fails (no workflow controller wired, or the step ID does not
resolve), both `advances` and `note` are omitted entirely rather than defaulted
to a guessed value — a caller must not infer "does not advance" from a lookup
failure that says nothing about the step's actual configuration.

## Diagnostic response

`internal/mcp/handlers/handlers.go:handleStepComplete` compares the launch step
from `stepCompletionLaunchStep` with the task's current workflow step.
The mismatch continues to return `VALIDATION_ERROR` before the signal claim.

The error retains its existing prefix for compatibility. Its text includes both
step IDs and the recovery instruction. This information belongs in the message,
because `internal/mcp/server/server.go:stepCompleteHandler` forwards `err.Error()`.
A details-only change would not reliably reach the agent.

Suggested message:

> workflow step changed before signal was recorded. This turn started in step
> <launch_step_id>. The current step is <current_step_id>. No signal was recorded.
> Retrying in this turn cannot recover. End this turn and ask the user to resume
> this session for the current step. After satisfying that step, signal completion
> from the new turn. Do not move the task solely to bypass this error.

A missing stamp or failed turn read remains a separate internal error.
The message does not promise that a concurrent future move cannot occur.

## Recovery instructions

The registered tool description explains the distinction between a stale-turn
error and `already_signaled`. A duplicate preserves the existing accepted signal.
A stale-turn rejection records nothing and requires a fresh turn.

`internal/sysprompt/sysprompt.go` supplies concise recovery guidance in
`stepCompleteSection` and `officeStepCompleteInstruction`. Gating and mode
visibility stay unchanged. The long explanation resides in the returned error.
This keeps prompt and tool-description size budgets intact.

The agent ends its stale turn with a clear recovery request. A user resumes the
same session on the current step. Normal turn creation stamps that step.
The agent evaluates current-step work before its final completion call.
No tool call silently creates a turn, restamps an old turn, or moves the task.

The existing operator move remains documented as a separate manual decision.
An error does not grant an agent new authority to use `move_task_kandev`.

## Persistence and compatibility

### Watchdog scan projection

The periodic stuck-signal watchdog uses `ListStepSignalWatchdogSessions`, a
read-only projection of session ID, task ID, state, passthrough status, and only
the `pending_step_completion_signal` metadata entry. It retains the general
active-session reader's CREATED, STARTING, RUNNING, and WAITING_FOR_INPUT scope
and start-time ordering, including sessions on archived and automation tasks.
It does not join workspace/execution tables or load configuration snapshots,
worktrees, MCP attachments, or model catalogs. The general active-session
reader continues returning full sessions for its other consumers.

SQLite and PostgreSQL extract the pending metadata entry in SQL. This bounds
payload transfer and Go decoding by the signal size; database JSON evaluation
can still read the original metadata. No schema, index, retention, or archive
policy changes. The projected sessions are observations only: recovery still
re-reads full sessions and turns under the existing cancellation, generation,
liveness, and delivery guards. Office detection and deduplication retain the
same pending-signal population and never gain transition authority.

There is no schema change, new response envelope, or new MCP argument.
`workflow_step_id_at_start` remains an immutable historical fact.
The conditional signal claim and duplicate handling retain their behavior and
add the entry fence described below.
RUNNING and WAITING_FOR_INPUT sessions both retain the mismatch guard.
Idleness does not prove that work belongs to the new step.

## Durable output-turn attribution

The tasks system owns fallback turn creation and completion eligibility. Platform
owns inbox ordering, replay, and acknowledgment; its
[delivery design](../../platform/system-design/durable-agent-delivery.md#backend-inbox-and-projection)
remains authoritative for that transport. This implements AC-001.4 and AC-001.5.
The [entry-bound attribution decision](../../../decisions/2026-10-22-entry-bound-delivery-turns.md)
records the failure and compatibility choices.

### Creation and replay

Only the new-row branch of `sqlite.Repository.canonicalDeliveryTurnTx` gains
stamping. It uses `readTaskStepInTx`'s task-row locking discipline and
`stampTurnWorkflowEntryTx` to obtain step and latest ledger ID from one locked
state, serializes the resulting metadata, and retains the deterministic ID and
`ON CONFLICT (id) DO NOTHING`. It stays inside the caller's projection transaction;
calling public `CreateTurnWithStepStamp` would open a separate transaction and
break projection atomicity. A real task with no step omits both keys. A legacy
stepped task with no ledger row keeps the existing omission of entry ID; zero
is an internal comparison value, never a fabricated ledger entry.

The transaction commits the fallback row, canonical message or shell message,
effect receipts, projected-at markers, and projected cursor together. A missing
task, failed task/ledger read, serialization failure, insert failure, or commit
failure rolls it back. No ordinary telemetry fallback commits a new unstamped
canonical row after an error. The received inbox remains available for replay.

All callers are covered:

- `persistCanonicalAgentMessageTx`: text and thinking, singleton and batch.
- `canonicalShellStartTurnTx`: foreground pending/running shell tools without
  an explicit turn. Terminal shell-status events only resolve an open turn;
  background tools and shell-output updates do not acquire new fallback authority.
- Requested-turn and existing-open-turn branches return the existing identity
  without reads that could overwrite metadata. Even an old unstamped turn stays
  unstamped; the completion handler never searches backwards for a stamped turn.
- Projected inbox replay skips turn creation. A deterministic-ID conflict leaves
  the stored row and metadata unchanged, including a closed row retried directly.
  Duplicate message content and effect identity remain governed by current replay.

### Locks and retry

SQLite's writer pool serializes writers; reserve the writer before reads using
the existing write-transaction convention. Use the existing writer pool, never
the read-only pool, for attribution.

PostgreSQL projection already acquires session-turn, delivery-session, and
stream advisory locks in `lockAgentDeliveryEventWritesMode`. A plain blocking
`FOR UPDATE` added afterwards can invert task-first workflow paths, including
`RecordAgentDeliveryWorkflowOutcome`. Do not introduce that wait cycle or
reorder global session/admission locks as part of this repair.

For the new fallback branch, add a narrowly scoped nonblocking mode of the same
task-row read (`FOR UPDATE NOWAIT`). Success holds the ordinary task row lock
through entry lookup and insertion. SQLSTATE `55P03` is projection contention:
roll back the entire projection transaction before retrying from a fresh
transaction and rechecking stored inbox/event identity, open turn selection,
step, entry, effects and cursor. Retry outside all locks with a cancellable
10 ms exponential delay capped at 250 ms, bounded by the caller context.
Cancellation returns promptly; no sleep retains a connection or transaction.
Other SQL/serialization errors are returned, not treated as contention.
Ordinary `readTaskStepInTx` callers retain their blocking mode and semantics.

This handles task-first movers and receipt writers without introducing a new
advisory-lock family. An intervening normal prompt can legitimately win the
retry and supply the existing open turn. Do not stamp against a pre-lock task
snapshot. Prove actual independent PostgreSQL connections and observed lock
ownership, rather than relying on a one-connection pool to appear serialized.

## Entry-bound completion

This implements AC-003.6 and AC-003.7. Investigation found that the current handler
and pending signal carry only the step ID. The stored turn entry stamp is
currently unused by completion, so A -> B -> A can accept an old turn.

Resolve the latest turn's immutable step and entry stamp together. Keep missing
step-stamp failures separate from stale-entry validation; never use an earlier
turn or the unlocked current task to fill a missing stamp. Positive entry IDs
must be parsed losslessly from typed and JSON-rehydrated metadata. A missing
entry stamp is eligible only when the locked task has no ledger entry either.
The existing no-turn compatibility path can bind to the locked task's entry
inside the claim transaction; it must not invent turn history.

`sqlite/step_completion_entry.go:ClaimStepCompletionSignal` compares the
expected step and entry under the task row lock, using
`getLatestTaskStepTransitionIDTx`. Keep the existing task/session ownership
check and first-writer atomicity. Avoid adding a session-turn advisory lock
after the task lock: ordinary turn stamping takes those locks in the reverse
order. Production may not silently fall back to the older step-only claimer
when entry evidence exists. The older step-only method remains for unrelated callers; production completion
uses the entry-aware repository capability.

An optional `workflow_step_transition_id` in `PendingStepCompletionSignal`
uses the existing session JSON bag; no SQL column or new MCP argument is needed.
The claim records the locked entry, including the legitimate zero-entry case.
New writes preserve presence, so zero and legacy absence are distinguishable.
The decoder retains typed/map parity. Duplicate claims compare entry as well as
step: the first accepted payload for one entry wins; an old or unproven entry
cannot mask a fresh eligible completion. Publish failures retry the same claim.

Both turn-end implementations and the waiting-session subscriber/watchdog must
require matching entry evidence when consuming signals. Propagate the stored
entry into `models.WithWorkflowStepCompletionGuard` so the existing task move
CAS (`taskStepTransitionGuardMatches`) checks it again under the task lock at
the transition boundary. Preserve cancellation, clarification, and workflow
policy guards. A read or fence failure creates no transition or delivery receipt
that claims one occurred. Context guards apply only to the source task/step;
destination entry actions keep their existing admission rules. The completion
variant also marks ownership of the handoff carry slot: the locked CAS rebase
retains its prepared update or removal while refreshing unrelated metadata.
Ordinary arrival guards do not acquire that ownership. A rejected legacy
completion move conditionally clears its observed signal and settles the finished
turn to waiting through the existing session-state path.

Delayed bus events remain wakeups to reread durable state; their payload is not
authority to overwrite the bag. Stale cleanup must condition its removal on the
observed signal identity, so a newer entry's signal survives a cleanup race.
Signal handoff/blocker carry, transition audit, and both engine and legacy paths
use only an entry-matching signal.

### Compatibility and recovery

Historical turns are never backfilled. New self-woken turns can complete in the
current entry. Old turns without a step stamp retain their current internal
error and need a fresh turn; old turns with a missing or mismatched entry cannot
complete a known later entry. Existing entry-less pending signals remain readable
but cannot authorize a known entry; a fresh eligible signal can replace them.
Unledgered legacy tasks retain the zero-entry compatibility case without
synthesizing history. No completion instruction bypasses question barriers,
changes tool visibility, creates a user message, or moves a task to recover.

`docs/public/automation-and-mcp.md` and `workflow-tips.md` explain normal
self-wake completion and fresh-turn recovery for older/unattributed turns or
revisited steps. No new UI or screenshots are required.

## Verification

Handler tests use the SQLite-backed existing fixtures. They reproduce Review to
Work movement after turn creation, reject repeated stale calls, and accept a
call after a new Work turn starts. Both session states retain the guard.

Server tests exercise error forwarding through the MCP handler. Prompt tests
cover gated task and Office contexts, omission from ungated instructions, and
existing size budgets. These are protocol tests, not browser layout changes.

Handler tests also cover the `advances`/`note` response fields directly: a
signal-gated step with a move action returns `advances:true` with no `note`; a
non-signal-gated step returns `advances:false` with a `note`; a signal-gated
step without a runnable move returns `advances:false` with a `note` while the
signal is still recorded and published; a step that fails to resolve omits
both fields. A model test pins `AdvancesOnTurnComplete`, and an engine test
compares it with the compiled `on_turn_complete` actions.

## Related decisions and contracts

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- [Turn stamp design](workflow-task-step-transition-ledger.md)
- [Requirements](../requirements/workflow-explicit-completion-signal.md)

## Implementation plans

- [Issue 3772 recovery package](../../../plans/step-completion-stale-turn-recovery/plan.md)
- [Applicability response package](../../../plans/office-assignment-step-eligibility/plan.md)

- [Fallback output-turn repair package](../../../plans/agent-delivery-turn-step-stamp/plan.md)
