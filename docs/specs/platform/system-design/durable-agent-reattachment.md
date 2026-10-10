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
A running peer permits reattachment while existing turn admission continues to block a second prompt.
A complete peer record still requires ordered canonical replay and terminal effect settlement.

If an executor's transport has been destroyed, return transport_unavailable while retaining recoverable identity.
Remote startup adoption uses the lifecycle retry path below to supply an authenticated replacement connection.
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

### Automatic restart recovery without prompt dispatch

Local/worktree processes stop on normal backend shutdown by default. The existing local survival flag remains off unless an operator enables it.
Supported remote executors instead detach host transport and preserve remote agentctl and active agents independently of that local flag.
Startup must first authenticate and adopt a surviving remote execution; native restore is reserved for confirmed process termination.
For an exact local predecessor, a terminal row can lack its former PID. If row liveness is unknown, the pinned authenticated runtime identity may prove that the old process and its owned descendants have terminated.
This proof applies only to the matching standalone execution. A live process, mismatched owner, incomplete identity, or failed inspection blocks restoration; remote rows never use this local proof.
Audit proxy teardown and executor-specific stop paths, including Sprites, so preserving an environment cannot mask a stopped agent.
Explicit Stop and authorized archive/reset/delete cleanup remain separate from backend shutdown.

The lifecycle manager retains unresolved remote records behind a retryable recovery guard. The guard prevents a fresh launch while remote liveness or authenticated ownership is unknown.
The existing remote-status loop retries a bounded, rotating set of records on its normal tick, with a deadline for each attempt. Shutdown cancels and joins the loop.
Each attempt validates the durable execution, task, session, and environment before attachment and again before tracking. Stop, archive, cleanup, and ownership changes fence a stale attempt.
Successful attachment shares startup's authenticated evidence and replay path. It clears pending recovery only after tracking succeeds.
Failed attachment closes temporary host clients without stopping remote processes or deleting their environments. Pending records remain eligible for later attempts without a new prompt.
Each adapter supplies cleanup for the local transport created by that attempt. Rejected adoption invokes that cleanup without calling remote Stop.
Cleanup checks the exact transport identity before removing adapter state, so a stale attempt cannot close its successor.
Final ownership validation and execution registration serialize with explicit Stop. A changed durable execution ID blocks adoption instead of repairing the row to match an older runtime.
Plugin attempts reload the current durable checkpoint for that same owner. They preserve provider, environment, operation, and execution identity while accepting checkpoint revisions saved by an earlier attempt.
This applies to both reconnect and pending Stop, so a transient failure after checkpoint persistence does not make later attempts stale.

This revision supersedes the manual interruption form and batch continuation design.
It implements AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 through 006.24.
The earlier explicit continuation implementation remains historical evidence in the package results.

The orchestrator owns one bounded startup recovery worker after lifecycle adoption and startup reconciliation complete.
It uses durable recovery records, not browser state or the count of disconnected streams.
Each candidate passes existing authorization, route, archive, process-identity, native-state, and execution admission checks.
Recover the authenticated owner from authoritative persisted ownership; do not synthesize an administrator identity or bypass session scopes.
Auth-disabled installations can have an empty stored workspace owner. The auth resolver permits that existing unscoped mode, while the checkpoint still pins the exact empty owner and organization.
Enabled authentication requires a current eligible identity and rejects an empty or disabled owner.
Office and automation remain with their schedulers. Ordinary recovery uses automatic admission, including capacity limits.
Shutdown cancels and joins the worker before repository or runtime teardown.
Transient transport unavailability uses bounded retry. Failed candidates do not block the remaining candidates.
Each startup candidate has a two-minute deadline and at most three native launch attempts per pass.
The worker owns this retry budget. The durable checkpoint preserves identity, not a permanent retry-exhaustion counter.
A later backend restart can retry the same checkpoint after fresh owner and liveness validation.

For a surviving owned process, use `RecoverAgentPromptStreamWithIdentity` and the existing adoption/replay path.
Do not initialize, load, or resume a native conversation that remains live.
For confirmed process termination, load the original native conversation with `NoInitialPrompt` and pause pending queued work through durable Auto-run state.
A missing execution record alone does not prove termination.
Missing native state has no fresh-session or history-continuation fallback in this automatic operation.

Separate restore-only checkpoints from instruction-bearing `ContinuationSnapshot` records.
The checkpoint identifies the session, incarnation, observed recovery revision, old generation, candidate execution, and target native conversation.
Use a registered additive migration if the existing restore-attempt contract cannot represent those fields and stages.
Do not manufacture a prompt submission ID, instruction message, or receiver-acceptance marker for a restore-only operation.

Persist the checkpoint before native load. Revalidate it under the existing session admission and lifecycle locks.
Before native restore, use the existing durable Auto-run pause when pending queue entries exist. Preserve their IDs, contents, and order.
Publish the queue status so the pause remains visible and survives another restart. Explicit Auto-run re-enable retains its existing behavior.
The checkpoint moves from `prepared` to `candidate_allocated`, then to `candidate_launching` before launch side effects.
An allocated candidate with no execution row can start only while the durable launching marker is absent.
A launching candidate with no execution row has unknown liveness and remains blocked.
Reuse a live candidate only after verifying its execution identity and native conversation.
Allocate a fresh execution identity for replacement only after confirming the exact previous candidate has stopped.
Persist `candidate_dead` with that exact execution identity before deleting its runtime record.
On restart, this stage permits cleanup to finish when the record is absent. A remaining record must match and be proven dead again.
After cleanup, move from `candidate_dead` to `candidate_allocated` with a fresh execution identity.
Do not rotate directly from `candidate_launching`; deletion before durable death evidence would make a crash indistinguishable from unknown liveness.
After native identity verification, retire only the identified interrupted journal submission without changing its uncertain outcome.
Commit the new delivery generation, matching recovery-block resolution, and checkpoint outcome with compare-and-set guards.
Preserve unrelated blocks and the interrupted queue claim. Recovery itself never drains queued user work.
A later normal user message uses ordinary admission against the restored generation.
A crash between journal retirement and SQL commit resumes the same checkpoint after rechecking current ownership.
If a new execution already owns that checkpoint, reconcile it instead of comparing it only with the old execution ID.
A foreign execution, generation, or native identity remains blocked.

Workspace reads must not allocate a competing execution while durable recovery is unresolved.
Check this at the workspace-only creation boundary, preserving access through an existing owned workspace execution.
The restore launch keeps its pinned candidate identity and allocation checkpoint; it must not adopt an unrelated workspace-only successor.
A generic prompt block without a saved executor identity, delivery binding, or pending restore identity does not alone prevent workspace-only creation.
Creation and registration must recheck that distinction; the workspace operation never clears the prompt block or starts an agent.

Silent restoration must also suppress persisted boot-status messages that would synthesize a lifecycle-only turn.
Setup and boot execution still run; ordinary starts and explicit resumes retain their status reporting.
Clear a delivery-uncertainty error only for the exact restored execution and submission.
Keep submission identity in a structured error field, separate from sanitized display text.
Frontend metadata merging uses the same identity and newer-recovery checks, preserving unrelated or newer errors.
Legacy sanitized errors without exact identity remain as history; successful restoration and dismissal must not expose recovery actions for that history.

The `continued` phase from the earlier implementation retains an old identity for historical evidence.
Do not treat it as an unresolved recovery candidate or submit it to old-generation retry validation.
Clear only obsolete presentation metadata when durable checkpoint and generation evidence prove successful restoration.
Do not clear unrelated errors or rewrite unknown outcomes as successful completion.

Remove `InterruptedSessionsRecovery` from the app status surface and phone task panel.
Remove the interruption-specific instruction form from `SessionStoppedBanner` and retire its unused hook/service wiring.
Remove the new batch continuation API if it has no remaining supported caller; do not retain a hidden prompt-dispatch alternative.
Existing generic recovery choices for unrelated failures remain governed by their own contracts.

## Submission-specific block settlement

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

Successful restart recovery leaves the normal chat and composer visible on desktop and phone.
It adds no global form, overlay, selection workflow, or user message.
An unresolved failure uses the existing session-local status and safe Retry/Stop controls.
Phone retains its existing chat scroll owner, safe-area handling, and touch targets.
Browser tests assert both the absence of the interruption forms and unchanged prompt counts after restart.

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
Detached MCP waiting/offline budgets and richer remote-working UI remain follow-up work.
