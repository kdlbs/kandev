---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
---

# Interrupted prompt recovery

## Ownership and boundaries

Platform owns backend-to-agentctl uncertainty and its admission block. This supplements
[durable delivery](durable-agent-delivery.md). [Harness continuity](../../agents/system-design/harness-session-continuity.md)
and the [harness boundary decision](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
retain authority for native resume, configuration, and explicit history continuation.

The existing Resume action routes through explicit session recovery. This repair uses
existing controls, recovery actions, and persistence.

## Requirement mapping

| Requirement | Section |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .7 and .10 | Workspace-only access |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .8, .9, .11 and .12 | Manual resume |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .1 through .6 | Existing delivery and continuity designs |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .25 and .26 | Bounded projection wait |

## Workspace-only access

`Service.LaunchSession` must not consult prompt admission for `IntentRestoreWorkspace`.
Keep task/session binding, archive, execution permissions, and workspace ownership checks.
Reject a nonempty prompt on workspace-only requests. Workspace restoration does not resolve
a prompt recovery block, including when `RecoveryAction` is supplied. No ACP initialize, new,
load, resume, prompt, or queue drain may occur. Quick Chat requires no Git repository.

Workspace inspection must preserve durable restart recovery ownership. An existing owned
workspace execution remains accessible. A generic prompt block alone may permit cold
workspace creation only when fresh evidence contains no saved execution, delivery binding,
or pending recovery source or candidate. Recheck this at creation and registration.
Active or malformed delivery recovery evidence blocks competing allocation, even for an
explicit workspace request. Wait for exact-owner reconnection or restoration instead of
replacing its row. See [durable reattachment](durable-agent-reattachment.md).

## Manual resume

`useManualResumeSession` calls `session.recover` with action `resume`, matching the existing
task recovery control. It passes the explicit action through `RecoverSessionWithOptions` and
`LaunchSession`, allowing native resume after an interrupted prompt left a block.

Route the shared manual hook through the same explicit recovery endpoint. A typed service
method returns the existing launch-shaped response so `applyManualResumeResponse`, optimistic
starting rollback, workspace fields, active-session request fencing, archive checks, and error
feedback continue to work. Use the existing 60-second manual resume timeout. Do not change
`requestSessionRecover` callers that expect `Promise<void>`.

Automatic open/focus paths still use `session.launch` with their existing activation source.
They cannot acquire explicit recovery authorization. Do not pass a recovery action directly
on an ordinary launch request to bypass the recovery coordinator.

Explicit native recovery must await the current agent-start or reattachment result before
acknowledging uncertain delivery or resolving its database block. An initialized flag retained
from recovered inventory is not proof that the current authenticated reattachment succeeded.
A current-start failure must leave the block and historical unknown submission intact.

`RecoverSessionWithOptions` retains existing runtime control, workspace recovery preflight,
configuration restoration, native-token preservation, Office dispatcher handoff, and
live durable-work refusal. An `unresolved_durable_work` block backed by interrupted-unknown
SQL evidence can use explicit native Resume for ordinary chat sessions when the lifecycle adapter supports journal
acknowledgement; Office retains its existing refusal because recovery delegates admission to its scheduler; prepared, accepted, and dispatching SQL work still blocks that path.
Its successful recovery resolves the matching current block with authorized action `resume`;
the old submission remains `interrupted_unknown`.
No historical prompt is supplied to native resume. Preserve existing duplicate submission
admission and accepted queue-claim handling. This repair adds no new dispatch authority.

The read-only notice passes the existing guarded manual Resume callback to shared recovery
actions when the composer does not own recovery. Details stay collapsed and busy state
disables the action; pointer-aware shared button sizing preserves phone touch targets.

Native Resume waits for existing runtime readiness and defers block resolution. After native initialization,
the lifecycle owner reads authenticated journal status and the full session submission list.
It validates session, incarnation, stream, generation, and healthy durable storage, rejects live
or incomplete-terminal work before mutation, and retires only interrupted-unknown records
after matching every unresolved SQL row to its peer record by ID, session, incarnation,
generation, and payload hash. Missing or conflicting evidence prevents every retirement write.
It then re-reads both stores and requires retained retirement for each unresolved SQL row
before clearing the database block. Already-retired matching records make this idempotent.
The eligible journal records belong to the current incarnation at or before the current generation.
Historical blocks may predate recovery-notice metadata. After the same authenticated retirement
check, SQL resolution accepts that case only when persisted ACP native identity matches the
current harness generation, the block uses that incarnation and generation, and its exact
submission remains interrupted-unknown in that session and generation. A stream identity is
required. Missing native metadata, a superseding generation, or mismatched submission keeps
the block open. Resolution preserves metadata and historical uncertainty rather than creating
a recovery snapshot whose original process identity is unavailable.
The process owner serializes retirement against prompt dispatch and checks the trusted owner. Retirement preserves unknown
state and payload and makes duplicate admission or dispatch of that ID fail. Existing newer
generation retirement records unfinished superseded work as interrupted-unknown and retains
its payload; previously terminal outcomes remain unchanged. A refreshed capability must be clear
before SQL block resolution and parked queue release. Legacy peers retain ordinary compatibility;
authentication and storage errors fail closed.

On backend adoption, unknown SQL history is excluded from active work only when authenticated
full peer records retain matching retirement, uncertain state, session/incarnation, generation,
and payload hash. Missing or conflicting proof remains blocked. No recovery summary field,
new endpoint, migration, native conversation replacement, or generation rotation is needed.

## Bounded projection wait

The [silent prompt work package](../../../plans/silent-prompt-recovery/plan.md)
records implementation and verification. `nativeResumeProofCursor` returns the
distinct `errNativeResumeProjectionLag` sentinel when received and projected
sequences differ. It wraps `ErrDurableAdoptionBlocked` for compatibility.
The retry loop owned by `GetNativeResumeDeliveryEvidence` must match
`errNativeResumeProjectionLag` with `errors.Is` before handling
`ErrDurableAdoptionBlocked`. Compatibility handling must not short-circuit the
bounded projection wait. Never classify errors by message text.

`GetNativeResumeDeliveryEvidence` owns the wait because it owns the complete
proof. Retry only verified projection lag or monotonic same-owner stream progress
after authentication, cursor coverage, submission retirement, and identity
validation. Either result starts the bounded wait, including on the first proof
attempt. Capture session, execution,
incarnation, stream, native session, and harness generation from that proof
attempt. Use a 10-second maximum and 200 ms polling, shortened by caller
cancellation/deadline. Do not detach this wait from the recovery request.

Poll the cursor through the existing repository and verify current execution
ownership. No event-bus, projection, cancellation, or stream lock may span the
wait. A notification primitive may replace polling if it already has the same
bounded and cancellation-safe behavior. Do not add a general retry service.

When the cursor catches up, discard the partial proof and rerun the full
existing authenticated status, SQL/journal inventory, retirement, cursor and
final-status checks. Revalidate captured identity before returning. One full
proof with equal cursors and matching opening/closing authenticated status is
sufficient; no additional quiet polling interval is required. If same-owner
stream progress invalidates that proof, retry no sooner than the next 200 ms
poll, using the original deadline. A stream that prevents a stable full proof
throughout the budget times out and stays blocked. Changed identity,
storage/authentication failure, unresolved live work, and conflicting evidence
fail immediately. Never retry native initialization or send a prompt as part of
this loop. Empty streams keep their current behavior.

The existing transactional SQL resolver remains the final authority. Resolving
one block cannot clear unrelated restrictions. Timeout returns the typed blocked
result and preserves retired history and prior uncertainty for a later explicit
attempt. Cancellation or a replacement execution prevents a late result from
resolving the block. Existing concurrent recovery ownership prevents duplicate
resolution and queue release.

The browser retains its existing 60-second manual Resume timeout and busy
controls on desktop and phone. No extra action or new layout is needed.
Tests use a projection barrier and real journal/SQL state to prove one-request
success, timeout, cancellation, owner change, same-owner progress, unavailable
storage, no historical resend, and preservation of unrelated blocks. Continuously
advancing status must time out on the original deadline without a tight retry
loop. Unresolved journal work, even with a lagging cursor, must return blocked
before starting a wait or scheduling a retry.

## Failure and compatibility

Native-state loss remains an existing typed history-continuation failure, not an automatic
fallback. Resume failure leaves the block open. Late replies must not update another active
session. Workspace inspection through an owned execution remains usable while the original
delivery block is open; cold allocation follows the ownership checks above.
Task chat and Quick Chat share the manual hook. Desktop and phone use existing controls and
layouts; the read-only notice reuses the localized Resume label and shared recovery actions.

## Persistence and verification

Use existing recovery blocks, generations, submissions, and native metadata. No migrations
are required. Existing SQL behavior and required-store registration remain unchanged.

Tests reproduce ordinary workspace admission failure and the manual Resume wire mismatch
before production edits. Service tests check response fields and timeout; hook tests prove
manual recovery with no automatic recovery grant and stale-result fencing. Real-repository
orchestrator tests preserve the native ID and unknown submission through explicit recovery,
retain blocks after failure, and show that no prompt dispatch occurs. Browser evidence covers
repository-free Quick Chat, existing Resume, reload, and a distinct new follow-up instruction.

Follow-up verification includes the no-composer-owner read-only browser state on desktop
and phone, real bbolt acknowledgement, no replay and distinct new delivery, busy/foreign-owner
rejection, failed journal acknowledgement retaining SQL blocks, journal reopen, and authenticated
adoption of acknowledged unknown history.

Canonical interrupted-prompt blocks with a submission ID and no structured runtime
recovery record retain the existing explicit Resume action. They do not select
Retry-only controls. A runtime recovery record or legacy uncertain-delivery error
still selects state-only Retry. Restart restoration follows the automatic
restore-only contract in the durable reattachment design.
The browser tests prove the existing Resume target remains clickable on desktop
and at a 320-pixel phone width, with a touch target of at least 44 pixels.

An open delivery block keeps ordinary input disabled even without a coarse session
error. Canonical interrupted work retains existing Resume; structured runtime
recovery retains Retry. Terminal SQL settlement advances
the session snapshot timestamp in the block-resolution transaction. The backend
publishes the cleared block array even when no runtime recovery metadata exists.
Explicit Resume publishes the same projection after resolving the saved block,
before releasing parked queue work. A later queue failure does not hide a
committed block resolution from the browser.
