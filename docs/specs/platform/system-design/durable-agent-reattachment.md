---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
---

# Durable delivery reconciliation and surviving-agent reattachment

## Scope and ownership

The platform owns durable delivery state and evidence. Lifecycle owns execution control; orchestrator owns admission and workflow effects.
This design extends [durable delivery](durable-agent-delivery.md) and [stream processing](durable-agent-stream-processing.md).
It complements [local runtime replacement](agent-runtime-availability.md): transport loss does not prove that agentctl died.
A surviving remote server must not enter the local replacement coordinator.

| Requirement | Design boundary |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001 | Journal shutdown and stream lifetime |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003 | State-only reconciliation and admission fences |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004 | Detached journal delivery and ordered reattachment |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005 | Terminal settlement and projection barrier |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 | Repeatable reconciliation, persistent session state |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007 | Live adoption, legacy compatibility, presentation |

## Detached journal delivery

`process.Manager.forwardUpdates` currently commits then blocks on updatesCh.
Durable publication must treat memory notifications as wakeups, not the only source of committed records.
Use a bounded coalesced wake mechanism. A durable stream writer reads journal pages from its own last-written sequence.
After each drain, compare the committed high-water before sleeping; register wake observation before checking the tail.
This prevents a lost wake when a commit races the drain-to-wait transition.
A final terminal or quiet single event must be delivered even if no later event arrives to reveal a gap.
All durable producers, including direct errors and completion, signal only after commit.

Capture stream owner identity once per writer and reject changed ownership.
Continue using bounded page sizes, sequence checks, per-connection authentication, and cursor recovery.
MCP/control traffic must not starve behind a large retained backlog; interleave bounded work without breaking event order.
Detachment does not acknowledge or prune events. Existing quota, reserve, health failure, and cancellation rules remain authoritative.
Legacy delivery keeps its bounded queue and explicit overload semantics; it cannot silently drop payloads or claim journal replay.
Do not add an unbounded channel or increase capacity as the repair.

## Journal shutdown and stream lifetime

The journal owns a terminal closed state under its existing mutex.
Every public database operation checks that state while holding the same lock used for database access.
An operation admitted before close finishes before close obtains exclusive ownership.
An operation admitted after close returns a typed closed-journal error.
Replay, acknowledgment, submission reads and writes, retirement, health, and compaction follow this rule.
Repeated close remains safe. A closed journal never becomes an empty successful replay.

Instance teardown first closes admission and cancels its stream writers and readers.
It waits for those owners within the existing teardown deadline, then closes the journal.
Late consumers still receive the closed-state error, including after a drain deadline expires.
API stream writers handle this result as instance shutdown and release their sockets and goroutines.
They must not panic or affect another instance in the shared agentctl process.
Backend detach remains different from instance teardown and leaves the journal open.
Committed records and exclusive file ownership survive the close/reopen cycle.

## Repeatable reconciliation

Replace the disconnect-only boolean query with one reusable lifecycle operation.
It accepts immutable session, execution/owner, incarnation, harness generation, stream, submission, and runtime epoch where applicable.
It returns a typed outcome: running_attached, terminal_settled, uncertain, owner_mismatch, or transport_unavailable.
A socket becoming connected is not terminal settlement.

Disconnect initiates at most three attempts within ten seconds, with cancellable waits of one and two seconds between attempts.
Each query is bounded by the remaining window; no request can extend the overall deadline.
Persist reconnecting state and fence new admission before the first attempt.
At exhaustion, persist uncertain state rather than declaring the prompt failed or completing its turn.
A later explicit retry or caller-provided reachability trigger can invoke a new bounded cycle for the same identity.
Only one cycle owns that execution at once; duplicate callers join or receive its current state.
Shutdown, Stop, successor ownership, and local runtime retirement cancel the obsolete cycle.
A late old result cannot clear state belonging to a successor.

Reuse RecoverAgentPromptStream and RetrySessionDelivery as entry points where practical.
Both disconnect-triggered and user-triggered paths must use the same reconciler.
Re-read status and durable evidence through the current authenticated client; do not trust cached transient state.
Pin the exact client for each bounded network request, then release its lease.
Do not hold a client read lease across identity capture, callbacks, or asynchronous stream attachment.
Check that the client still owns the execution before and after each reconciliation attempt.
A running peer permits reattachment while existing turn admission continues to block a second prompt.
A complete peer record still requires ordered canonical replay and terminal effect settlement.

This package does not implement executor-specific redial or a permanent background retry loop.
If an executor's transport has been destroyed, return transport_unavailable while retaining recoverable identity.
A later executor redial hook can supply the authenticated replacement connection and invoke the same operation.
The ten-second initial window does not define a maximum lifetime for recoverable uncertainty.

Established Docker environments remain retained after recoverable agent failure. Cleanup stops process ownership without force-removing the container, so a later authorized resume can reuse its workspace. Fresh bootstrap rollback, task/session deletion, and explicit force-stop remain destructive. This policy does not authorize a prompt resend.

### Recovery without an in-memory execution

`RetrySessionDelivery` resolves recovery identity from durable session, generation, submission, and environment records before requiring a live client.
`RecoverAgentPromptStream` remains the live-execution path, not the sole recovery authority.
Execution removal must not discard the recovery descriptor or its original runtime ownership evidence.
The replacement runtime opens a retained journal only through the existing authenticated, exclusive ownership boundary.
It can read and project evidence without starting a harness or sending a prompt.
Unknown ownership, a locked journal, or ambiguous identity produces a typed blocked result.

The recovery response carries a bounded outcome, reason, allowed actions, and an observed recovery revision.
Outcomes distinguish attached, settled, uncertain, unavailable evidence, and blocked ownership.
Responses contain no raw paths, credentials, or provider output.
The request remains authorized for the exact task/session pair.
Repeated requests join the existing bounded operation. Late responses cannot overwrite a newer recovery revision.

Initial prompt submission must persist its canonical identity before harness dispatch, as later prompt submissions already require.
The lifecycle and orchestrator use that same identity when recording runtime loss.
An older session with missing canonical submission data must retain a typed unresolved notice.
Recovery does not invent a completed submission or silently discard the notice because a lookup failed.
Unique retained journal and generation evidence can reconstruct the association through the existing repository contract.
Ambiguous records remain blocked and retain an explicit explanation.

### Explicit continuation after confirmed interruption

State-only retry never starts another prompt. An uncertain result can advertise a separate native resume action only after ownership checks succeed.
The existing `resume` recovery request gains an optional interruption acknowledgment, observed recovery identity/revision, new instruction, and idempotency key.
This branch reuses session authorization, admission serialization, native restore, and durable submission preparation.
The server rechecks the original process termination and all independent recovery blocks at the mutation boundary.
A missing execution entry or an available replacement runtime alone cannot authorize continuation.

The request preserves the same task, session, worktree, and recoverable native conversation.
It records acknowledgment of the old uncertainty without converting that submission into completed or cancelled work.
Native restore alone does not release durable admission. The current implementation retires unresolved submissions only for forced history continuation.
For acknowledged native recovery, admit a new delivery generation while retaining the original native conversation ID.
After native load succeeds, retire only the proven interrupted submission through a generation-fenced journal operation.
The existing retirement method writes `cancelled` and drops the payload. Extend its explicit recovery contract to retain the original uncertain outcome and evidence.
This retirement must remain effective in capability, recovery-descriptor, and admission queries without claiming provider cancellation.
Persist recovery progress before dispatch. A crash between native load, retirement, projection, and dispatch must resume the same recovery operation.
A prepared checkpoint precedes the atomic generation commit, so a retry may restore it after rechecking process termination, ownership, revision, and independent blocks.
A restored checkpoint can have dispatched already. Retry checks its stable canonical instruction message, which is written only at agentctl acceptance, before completing acceptance bookkeeping.
Backend submission admission alone is not receiver acceptance. Missing acceptance evidence remains restored-but-blocked without redispatch.
Do not advertise a ready session merely because native loading or stream attachment succeeded.
Native initialization defers its default prompt. Only the explicitly supplied new instruction receives a new submission identifier.
For ordinary first-instruction launch, the runtime accepts an already admitted canonical row only when the launch handoff identifies that message, its full owner and payload match, and this execution has not dispatched a prompt. Runtime-owned duplicates still require reconciliation.
The interrupted payload, queued claims, permissions, and tool calls are never replayed by this action.
Duplicate requests return the first operation result without another dispatch.
Unrelated recovery blocks and Office scheduler ownership remain effective.
An Office action returns through its scheduler, never a direct chat launch.

Missing native state still uses the existing explicit history-continuation contract, with its separate user choice.
Missing process ownership or conflicting retained identities remain blocked, even after user acknowledgment.
No recovery request silently creates another conversation or clears every open recovery cause.

### Bulk recovery of existing conversations

Offer Resume interrupted sessions from the shared runtime recovery notice, with a selectable session list and per-session eligibility reasons.
Keep the same control available on phone, using stacked session rows and touch-sized actions.
Selection is explicit; the number of disconnected streams alone cannot identify active interrupted tasks.
The batch delegates to the same generation-fenced per-session recovery operation with bounded concurrency.
Each item carries its own observed identity, recovery revision, continuation instruction, and idempotency key.
One blocked session does not prevent other eligible sessions from recovering. Refresh preserves progress and completed item results.
Browser requests, in-flight operations, and results are scoped to the task, session, and full recovery identity. A later interruption starts with a blank instruction and unchecked acknowledgment.
A saved request remains available when the same identity advances its recovery revision, even if the acceptance reply was lost.
Retry sends that original idempotency key and immutable instruction to recover the accepted result without a fresh preflight or redispatch.
A completed result stays visible across the revision change. Late replies cannot overwrite a different interruption's checkpoint or current batch result.
The Resume button keeps its accessible name while a translated status region announces progress.
Preserve queued user instructions and their order without redispatching an uncertain claim.
The operation never uses fresh-start, a new Kandev session, or history replacement as an implicit fallback.
Report restored-but-blocked separately from work that has actually accepted its continuation instruction.

## Submission-specific block settlement

The [missing delivery record design](durable-agent-record-recovery.md) defines a
bounded reconstruction path before this settlement flow. Verified reconstruction
restores delivery associations only. Ambiguous records remain blocked.

SessionRecoveryBlock currently identifies session/incarnation/generation but not the original submission.
Add a durable binding between an agent_delivery block and submission ID plus stream/turn identity where needed.
Use a typed repository contract and additive registered schema migration, not provider text or an in-memory map.
Existing blocks without verifiable binding remain open; migrate only when durable evidence uniquely identifies their original submission.
Never attach an old ambiguous block to whichever submission happens to be active now.

After ordered terminal projection, reconcile the authoritative submission state and queue claim through existing guards.

Ordinary chat completion performs session side effects under the admission guard, then releases that guard before a synchronous Git snapshot. The snapshot still finishes before the handler returns and cannot change a successor's session state. Office and automation capture before their settlement can tear down the runtime. Prompt admission rejects an unconfigured executor before claiming a turn or changing session state.
Resolve the matching block with compare-and-set on block identity, reason, submission, incarnation, and generation.
Idempotent duplicate settlement returns success without repeated workflow or queue effects.
Terminal projection may finish before the prompt RPC returns. Its completion handler must preserve the matching submission's recorded terminal outcome instead of requiring it to remain dispatching or creating a new delivery recovery block.
The agentctl journal commits a definitive `complete` event and its matching dispatching submission's completed state atomically, before replay can expose completion and admit a successor. The event must match the submission's session, incarnation, harness generation, and bound stream. Previously recorded uncertain, failed, cancelled, or retired outcomes remain unchanged.
An unreadable, unresolved, or mismatched submission still fails closed.
Do not use the broad manual recovery resolver as an unconditional automatic unblock operation.
Other native-state, configuration, authorization, workspace, runtime-loss, and operator-required blocks remain effective.
If the existing single-open-block shape coalesces causes, preserve every cause before enabling automatic settlement.
A resolved delivery cause must not erase a concurrent independent cause.

Projection, outcome settlement, and block resolution can cross repositories.
Use a persisted settlement intent or existing durable effect machinery so interruption between stages can resume safely.
A later reconciliation must finish settlement even if the projected cursor already passed the terminal event.
Release queue eligibility only after all required settlements commit and all other guards pass.
Restore the original queue claim; never resend the completed submission or synthesize another turn completion.
Failed and cancelled terminal outcomes settle according to existing workflow policy, not as successful completion.
Office and automation re-enter their normal budget/provenance/authorization gates.

## Persisted recovery presentation

Persist a session-scoped delivery recovery record through the task repository.
Include bounded phase/reason, immutable submission/stream identity, and a revision for stale-update rejection.
Use existing session metadata or a typed additive field consistently; make one view model authoritative.
The in-memory execution FailureCode and AgentctlError notification alone do not establish persistent UI state.
Keep compatibility with DURABLE_DELIVERY_UNCERTAIN where the current composer consumes last_agent_error.
Clear only the matching delivery notice when reconciliation succeeds; preserve unrelated errors.

Prompt-error rollback must not make unresolved work appear idle/ready or complete the turn solely because transport broke.
Audit generic failure handlers, startup cleanup, task stall reconciliation, and terminal publication for this distinction.
A stored coarse session state can remain compatible, but admission and rendering must derive unresolved status from the authoritative record.
The global local-runtime alert and this per-session recovery notice are independent.

Desktop and phone show reconnecting, uncertain, or recovered state in the existing chat surface.
Retry connection queries/reconnects only; Stop targets the original owned work and reports unconfirmed cancellation honestly.
Do not offer context continuation merely because transport failed.
Keep one chat scroll owner, stacked 44px phone actions, keyboard access, and safe-area clearance.
Localize new copy in all seven shipped catalogs. No raw credentials or provider errors reach the notice.

The missing-execution result replaces the generic resume error with a localized cause and available next actions.
The card announces progress and success or failure even when its title does not change.
The action row contains the buttons themselves. Busy text, warnings, and disclosures sit outside that row.
The row owns its outer spacing, so a nested retry wrapper cannot shift Retry connection relative to Stop.
Both buttons use `controlSizingClassName`: 28px desktop controls and at least 44px phone or coarse-pointer targets.
Phone actions stack at the existing 768px boundary without horizontal overflow.
The existing recovery card and mobile runtime-replacement tests are the nearest shipped exemplars.

Eligible continuation expands an inline instruction field and explicit acknowledgment within the same card.
The user reviews the warning and supplies the next instruction before activating Resume session.
This rare recovery flow stays in the chat scroll region on desktop and phone, without another overlay or scroll owner.
Unauthorized or blocked users see the reason, workspace access, and safe state-only controls.
New copy uses all seven shipped catalogs, including Korean and generated Traditional Chinese variants.

## Surviving-agent reattachment

`manager_startup.go` currently routes reuseExisting into initializeAgentSession.
Introduce explicit startup disposition: created_by_attempt or reattached_existing.
Carry that disposition and immutable cleanup ownership through lifecycle and executor startup results/errors.
A boolean at the final cleanup site is insufficient if earlier error paths already stopped the process.

For a surviving initialized peer, verify executor ownership, native session association, and durable capability through existing status/adoption seams.
Then restore execution identity and replay through the shared reconciler without ACP initialize, load, new, or prompt.
For genuinely uninitialized or missing peers, use the ordinary startup path only after proving the attempt owns creation.
Unknown evidence blocks; it does not authorize reinitializing a busy peer.
Legacy peers require authenticated positive identity evidence and their existing safe status/reattachment path.
If a legacy peer cannot prove association, preserve it and report unsupported reattachment instead of guessing.

Only resources created and owned by the failed attempt are eligible for automatic startup teardown.
Failed reattachment may close its new tunnel/client but must not force-stop the pre-existing harness or agentctl.
Explicit authorized Stop remains separate. A stale start failure cannot stop a later successor.
Do not write REVIEW or final task failure solely because transport reattachment failed.

## Compatibility and implementation evidence

No feature flag or standalone-runtime policy change is introduced.
New durable block/phase fields require SQLite fresh/reopen/upgrade and PostgreSQL conformance evidence.
Old ambiguous records fail closed. Preserve journals, native state, and user edits across upgrades.
Reuse existing agent_delivery_* metrics with bounded reasons; keep IDs out of metric labels.
The [work package](../../../plans/durable-agent-reattachment/plan.md) owns implementation and validation.
The [shutdown and recovery repair](../../../plans/agentctl-journal-shutdown-recovery/plan.md) owns the later closed-journal and missing-execution regressions.
Executor redial, long-horizon triggers, detached MCP waiting/offline budgets, and richer remote-working UI remain follow-up work.
