---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-10-03
updated: 2026-10-03
owners:
  - kandev
---

# Workspace stream continuity design

## Purpose and boundaries

Platform owns workspace Git publication and delivery. Tasks owns execution identity and canonical workspace bindings.
This design supplements [workspace Git status](workspace-git-status.md), rather than creating another publication authority.

The lifecycle manager implements the correction. Agent startup must not retire workspace callbacks on an unchanged execution and client.
The [session turn settlement design](../../tasks/system-design/session-turn-settlement.md) still owns ACP callback and prompt-generation rejection.
The [environment-owned design](../../tasks/system-design/environment-owned-git-status.md) still owns source eligibility and persistence.

## Requirement mapping

| Acceptance criterion | Design boundary |
| --- | --- |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43` | Callback lifetime and promotion |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27` | Replacement and teardown rejection |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2`, `.20` | Existing accepted publication reaches subscribers |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29`, `.31` | Shared desktop/phone state and unchanged polling |

## Components and responsibilities

- `StreamManager.buildWorkspaceCallbacks` forwards workspace events for their originating execution and agentctl client.
- `StreamManager.connectWorkspaceStream` retains the existing attached connection during promotion.
- `AgentExecution.withAgentCtlClient` holds the source lease through callback work and publication.
- `AgentExecution.beginStartupAttemptWithID` advances ACP identity without changing workspace identity.
- `Manager.handleGitStatusUpdate` rejects an execution that is no longer current for its session.
- Existing event publishers and environment-keyed frontend state retain snapshot quality, ordering, and repository scope.

## Callback lifetime and control flow

1. Workspace-only preparation connects the workspace stream before any ACP process starts.
2. The callbacks capture the execution and the client that opened that stream.
3. Agent promotion advances ACP startup generation on that same execution.
4. The attached-stream guard preserves the workspace connection and its original callbacks.
5. Later workspace notifications enter the captured client lease and reach their existing handlers.
6. Git publication checks current execution ownership and preserves the existing environment/repository identity.

Remove the ACP startup lease from the common workspace callback forwarding path.
Retain the captured-client lease through callback publication. A pointer check followed by an unprotected callback is insufficient.
Production stream construction must always supply its originating client. Existing clientless construction is a test seam, not production authority.

Apply this lifetime consistently to shell, Git, file, and process callbacks, including connection/error callbacks.
These channels belong to the workspace runtime. Their payloads retain the originating execution rather than a fresh session lookup.
No new generation counter, per-promotion reconnect, or stream replacement mechanism is required.

ACP update streams, prompt completion, readiness, and disconnect callbacks retain their existing startup-generation leases.
Existing guards for asynchronous foreground Git reads also remain intact.
This correction changes the lifetime of attached workspace callbacks only.

## Replacement and teardown rejection

Client replacement takes the exclusive source lease. An admitted old callback completes before replacement publishes its new client.
After replacement, callbacks from the former client do not enter downstream handlers.
Terminal client detachment likewise prevents later callbacks from that client.
Git updates from an execution no longer registered for its session remain rejected by the manager.

Keep stream cancellation, `Close`, `Wait`, and conditional stream clearing intact.
Manager shutdown must drain stream goroutines without a leaked reader or an indefinite readiness wait.
Tests must retain ACP stale-generation rejection independently of workspace continuity.

## Failure and recovery

Transport failure retains existing connection retry and teardown behavior.
Missed notifications retain existing bounded foreground recovery and delayed retry.
Recovery cannot substitute for a functioning attached workspace stream after promotion.
No faster polling, larger timeout, new retry loop, or changed Git admission policy forms part of this repair.

## Data, persistence, and security

The existing workspace stream payloads and `status_update` events remain compatible.
No database migration, configuration key, feature flag, provider branch, or permission change is required.
Canonical environment checks, session authorization, repository selection, and stale snapshot ordering remain unchanged.
Diagnostics must exclude file contents, credentials, and private workspace paths.

## Desktop and phone outcomes

Desktop keeps the existing task Changes panel. Phone keeps bottom navigation to Changes and the full-height diff surface.
Both surfaces accept post-promotion file membership and settled details from the shared environment state.
The existing loading indicator settles from accepted updates without focus cycling or manual refresh.
No layout, touch target, copy, navigation, or localization change is proposed.

## Observability and verification

Use existing focus, poll-mode, stream-connection, publication, and frontend status logs to correlate one execution.
Distinguish request duration, time until a later recovery, and time from accepted publication to visible state.
Global poll-mode or enrichment messages without source identity cannot establish a task's delay.

Deterministic Go regressions cover reused callbacks, repeated startup, client replacement/detachment, execution rejection, and shutdown.
A real workspace WebSocket fixture must cross the callback boundary and reach the manager publisher.
Desktop and phone browser scenarios prepare a CREATED session with a live workspace-only runtime, open Files and Changes, start the agent, and observe a real Git mutation through the same execution.
Completed-session resume creates a new runtime execution, so completed-workspace restoration remains a separate regression and does not serve as promotion-continuity evidence.
Read-response or store injection cannot prove this regression.

## Related decisions and delivery

The [progressive publication ADR](../../../decisions/2026-09-30-progressive-workspace-git-refresh.md) remains authoritative.
This repair aligns existing callback leases with their existing owners. It requires no new ADR.

See the [fix plan and work orders](../../../plans/workspace-stream-continuity/plan.md).
