---
status: draft
system: platform
requirements:
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

This package does not implement executor-specific redial or a permanent background retry loop.
If an executor's transport has been destroyed, return transport_unavailable while retaining recoverable identity.
A later executor redial hook can supply the authenticated replacement connection and invoke the same operation.
The ten-second initial window does not define a maximum lifetime for recoverable uncertainty.

Established Docker environments remain retained after recoverable agent failure. Cleanup stops process ownership without force-removing the container, so a later authorized resume can reuse its workspace. Fresh bootstrap rollback, task/session deletion, and explicit force-stop remain destructive. This policy does not authorize a prompt resend.

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
Localize new copy in all six shipped languages. No raw credentials or provider errors reach the notice.

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
Executor redial, long-horizon triggers, detached MCP waiting/offline budgets, and richer remote-working UI remain follow-up work.
