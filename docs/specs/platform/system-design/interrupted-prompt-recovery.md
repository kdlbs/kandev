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
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .8 and .9 | Manual resume |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criteria .1 through .6 | Existing delivery and continuity designs |

## Workspace-only access

`Service.LaunchSession` must not consult prompt admission for `IntentRestoreWorkspace`.
Keep task/session binding, archive, execution permissions, and workspace ownership checks.
Reject a nonempty prompt on workspace-only requests. Workspace restoration does not resolve
a prompt recovery block, including when `RecoveryAction` is supplied. No ACP initialize, new,
load, resume, prompt, or queue drain may occur. Quick Chat requires no Git repository.

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

`RecoverSessionWithOptions` retains existing runtime control, workspace recovery preflight,
configuration restoration, native-token preservation, Office dispatcher handoff, and
`unresolved_durable_work` refusal. Its successful recovery resolves the matching current
block with authorized action `resume`; the old submission remains `interrupted_unknown`.
No historical prompt is supplied to native resume. Preserve existing duplicate submission
admission and accepted queue-claim handling. This repair adds no new dispatch authority.

## Failure and compatibility

Native-state loss remains an existing typed history-continuation failure, not an automatic
fallback. Resume failure leaves the block open. Late replies must not update another active
session. Workspace restoration remains usable while the original delivery block is open.
Task chat and Quick Chat share the manual hook. Desktop and phone use existing controls and
layouts; no rendered markup or localization change is required.

## Persistence and verification

Use existing recovery blocks, generations, submissions, and native metadata. No migrations
are required. Existing SQL behavior and required-store registration remain unchanged.

Tests reproduce ordinary workspace admission failure and the manual Resume wire mismatch
before production edits. Service tests check response fields and timeout; hook tests prove
manual recovery with no automatic recovery grant and stale-result fencing. Real-repository
orchestrator tests preserve the native ID and unknown submission through explicit recovery,
retain blocks after failure, and show that no prompt dispatch occurs. Browser evidence covers
repository-free Quick Chat, existing Resume, reload, and a distinct new follow-up instruction.
