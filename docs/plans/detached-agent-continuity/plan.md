---
created: 2026-09-27
status: draft
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
legacy_specs: []
---

# Implementation Plan: Detached Agent Continuity

## Overview

A remote agent keeps working through a backend link loss, and Kandev
reconnects to it by itself. The package is stacked on the durable delivery
work (PR #3598, branch `feature/investigate-durable-cce`) and uses its
journal, replay cursor, and reconciliation. Rebase onto that branch, or onto
`main` once it lands, before starting each wave.

The waves run in this order, each for a reason:

1. **agentctl first** (offline budget, then waiting Kandev calls). These
   parts are self-contained and make an agent safe to leave detached.
2. **Backend Disconnected state.** It stops a link loss from failing the
   turn.
3. **Reconnect coordinator and redial interface.** They depend on the
   Disconnected state existing.
4. **Per-executor redials and the UI.** Both depend on the coordinator.

Sprites is last and blocked until a test environment exists.

The origin incident is recorded in
[ADR-2026-09-27-backend-dialed-detached-agents](../../decisions/2026-09-27-backend-dialed-detached-agents.md).

## Scope

### In scope

- The Disconnected link state, and Stop while disconnected.
- A reconnect coordinator with three triggers: reachability, backoff, and
  user Reconnect.
- The `RemoteTransportRedialer` contract, with SSH, remote Docker, Sprites,
  and Kubernetes implementations.
- Kandev tool calls that wait while detached, with keepalive and the new
  error texts.
- The offline budget: agentctl detach clock, per-profile override, and the
  unowned-reaper bound.
- Agent guidance in the system context.
- Conversation notices and the Disconnected UI on desktop and phone.

### Out of scope

- The five durable-delivery items requested in the PR #3598 review
  (https://github.com/kdlbs/kandev/pull/3598#issuecomment-5852538250):
  - the reconciliation window;
  - resolving the recovery block after replay;
  - persisting the uncertain-delivery code;
  - non-blocking `updatesCh` while detached;
  - no re-initialize on resume-with-reuse.
- agentctl dialing the backend, and any outbox or replay of tool calls.
- Reconnecting across a backend restart.
- Local and worktree executors.

## Technical approach

Symbols and paths are defined in the
[system design](../../specs/platform/system-design/detached-agent-continuity.md).
This section lists the integration points per slice.

- **Offline budget** (task 01):
  - `agentctl/server/process/attachment.go` detach clock;
  - `(*Adapter).Cancel` on expiry;
  - the journaled `agent_link.offline_budget_exhausted` event;
  - `offline_budget_minutes` in `profileConfigAuthoritativeKeys`;
  - `OfflineBudget` in `agentctl.CreateInstanceRequest` and
    `config.InstanceOverrides`;
  - the lower bound in `ownershipperiod.Resolve`.
- **Waiting Kandev calls** (task 02):
  - `ChannelBackendClient.RequestPayload` waits on an `AttachmentWaiter`;
  - `FailStreamRequests` uses `ErrKandevCallOutcomeUnknown`;
  - one `emitKeepAlivePings` wrapper covers every `RequestPayload` handler;
  - a `connectionLossSection` in `sysprompt.go` and `kandev-context.md`.
- **Disconnected state** (task 03):
  - a branch before the failure path in `handleStreamDisconnectWithAttempt`;
  - `LinkState` on `AgentExecution`;
  - the held prompt waiter;
  - `events.AgentctlDisconnected` and its WS mapping;
  - `task_sessions.metadata.agent_link`;
  - Stop sets `pending_stop`.
- **Reconnect coordinator** (task 04):
  - the `RemoteTransportRedialer` interface and the redial error types;
  - a coordinator in `lifecycle`, single-flight through `remoteRefreshGroup`;
  - an `events.ExecutorReachabilityChanged` subscriber;
  - the backoff timer;
  - the `session.reconnect` WS action;
  - the Kubernetes adapter over `RefreshRemoteInstance`;
  - the orchestrator notices.
- **Per-executor redials** (tasks 05-07): SSH replaces the lost
  `sshSessionState`, remote Docker retains `targets` on loss, and Sprites
  re-proxies.
- **UI** (task 08):
  - `SessionAgentctlStatus` gains `disconnected`;
  - a `DisconnectedSessionBanner`;
  - card and tooltip indicators;
  - copy in six locales.

Executor compatibility:

| Executor | Transport | Redial | Evidence | Fallback |
| --- | --- | --- | --- | --- |
| SSH | SSH client and local forward | Task 05 | Unit tests plus the containers E2E with a network cut | n/a |
| Remote Docker | SSH, Docker dial-stdio, forward | Task 06 | Unit tests plus the containers E2E | n/a |
| Kubernetes | port-forward | Existing refresher, adapted in task 04 | Existing refresh tests plus an adapter test | n/a |
| Sprites | `sprite.ProxyPort` | Task 07 (blocked) | Needs a Sprites test account | Current failure path until done |
| Local, worktree, Docker (local) | same host | None | n/a | Current failure path |

## ASCII UI preview

### UI-01: Disconnected banner, desktop chat (session disconnected)

Maps to AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.1, 001.5, and 002.3.

```text
+----------------------------------------------------------------------+
| (conversation above)                                                 |
+----------------------------------------------------------------------+
| [!] Disconnected from neo since 02:00                                |
|     The agent keeps working on the host. Updates sync on reconnect.  |
|     It pauses at 02:15 if still disconnected.                        |
|     Next attempt in 40s                  [ Reconnect ]  [ Stop ]     |
+----------------------------------------------------------------------+
| Message input (disabled while disconnected)                          |
+----------------------------------------------------------------------+
```

Required structure: host, since time, pause time, Reconnect as the primary
action, then Stop. The retry countdown is illustrative.

### UI-02: Disconnected banner, phone (same state)

Maps to AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.1.

```text
+--------------------------------+
| [!] Disconnected from neo      |
|     since 02:00                |
| Agent keeps working. Pauses at |
| 02:15 if still disconnected.   |
| [ Reconnect ]        [ Stop ]  |
+--------------------------------+
```

Required structure: no horizontal overflow, and both actions stay visible
without scrolling the banner.

### UI-03: Conversation notices after reconnect

Maps to AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3, 006.4, and 006.5.

```text
  ... agent messages produced while detached, in order ...
  -- Turn ended while disconnected at 02:31 --
  -- Reconnected after 8h 10m --
```

```text
  ... agent messages ...
  -- Paused at 02:15: offline budget reached. Waiting for your instruction. --
  -- Reconnected after 8h 10m --
```

### UI-04: Task card and remote status (session disconnected)

Maps to AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.2.

```text
+----------------------------------+
| Fix silent Office heartbeat 403  |
| (cloud-off) Disconnected         |   tooltip: "Disconnected from neo
+----------------------------------+    since 02:00. Reconnecting."
```

## Tests

| AC | Evidence |
| --- | --- |
| 004.1 | `orchestrator/executor/executor_state_test.go` `TestOfflineBudgetProfileResolution` |
| 004.2, 004.3 | `agentctl/server/process/attachment_test.go` `TestDetachClockCancelsTurnAtBudget` |
| 004.4 | `attachment_test.go` `TestOfflineBudgetRestartsAfterAttach` |
| 004.5 | `common/ownershipperiod/period_test.go` `TestResolveCoversOfflineBudget` |
| 003.1, 003.3 | `mcp/server/backend_client_test.go` `TestRequestPayloadWaitsWhileDetached` |
| 003.2 | `mcp/server/handlers_test.go` `TestKandevCallKeepAliveWhileWaiting` |
| 003.4 | `backend_client_test.go` `TestFailStreamRequestsReportsUnknownOutcome` |
| 003.5 | `agentctl/server/process/manager_permission_test.go` `TestPermissionParkedUntilBudgetCancel` |
| 004.2, 005.1 | `backend_client_test.go` `TestRequestPayloadBudgetExhaustedText` |
| 005.2 | `sysprompt/sysprompt_test.go` `TestKandevContextHasConnectionLossSection` |
| 001.1, 001.2 | `lifecycle/manager_events_disconnect_test.go` `TestStreamDisconnectEntersDisconnected`, `TestDisconnectedHasNoTimerExit` |
| 001.5, 001.6 | `lifecycle/manager_events_disconnect_test.go` `TestStopWhileDisconnectedCancelsOnReconnect` |
| 006.6 | `orchestrator/agent_link_test.go` `TestAgentLinkMetadataPersisted` |
| 002.1, 002.2, 002.3 | `lifecycle/reconnect_coordinator_test.go` `TestReconnectOnReachability`, `TestReconnectBackoffSchedule`, `TestUserReconnectCancelsTimer` |
| 002.4, 002.5 | `reconnect_coordinator_test.go` `TestReconnectSkipsInitializeAndReplays` |
| 001.3, 001.4 | `reconnect_coordinator_test.go` `TestReconnectTargetGoneReconciles` |
| 001.7 | `agentctl/server/process` `TestAgentPgidRecord` (task 01); `lifecycle/executor_ssh_redial_test.go` `TestSSHOrphanReap`; `executor_remote_docker_redial_test.go` `TestRemoteDockerOrphanReap` |
| 006.3, 006.4, 006.5 | `orchestrator/agent_link_notices_test.go` `TestReconnectNotices` |
| 002.6 | `lifecycle/executor_ssh_redial_test.go`, `executor_remote_docker_redial_test.go`, `executor_kubernetes_redial_test.go`, `executor_sprites_redial_test.go` |

## E2E tests

| Flow | AC | File | Project |
| --- | --- | --- | --- |
| Banner, Reconnect, and Stop on a seeded Disconnected session | 006.1, 001.5, 002.3 | `e2e/tests/session/detached-agent-continuity.spec.ts` | `chromium` |
| Same flow on a phone, no overflow | 006.1 | `e2e/tests/session/mobile-detached-agent-continuity.spec.ts` | `mobile-chrome` |
| Card and tooltip indicator, reload keeps state | 006.2, 006.6 | `detached-agent-continuity.spec.ts` | `chromium` |
| Real SSH executor: cut the network, agent keeps working, network back, auto reconnect, replay once, notice | 001.1, 002.1, 002.4, 002.5, 006.3 | `e2e/tests/ssh/detached-reconnect.spec.ts` | `containers` |

## Work orders

- [ ] [Task 01: Offline budget in agentctl](task-01-offline-budget.md)
- [ ] [Task 02: Kandev tool calls wait while detached](task-02-waiting-kandev-calls.md)
- [ ] [Task 03: Disconnected link state](task-03-disconnected-link-state.md)
- [ ] [Task 04: Reconnect coordinator and redial contract](task-04-reconnect-coordinator.md)
- [ ] [Task 05: SSH redial](task-05-ssh-redial.md)
- [ ] [Task 06: Remote Docker redial](task-06-remote-docker-redial.md)
- [ ] [Task 07: Sprites redial](task-07-sprites-redial.md) (blocked: needs a test environment)
- [ ] [Task 08: Disconnected UI and notices](task-08-disconnected-ui.md)

Dependency order:

- Wave 1: tasks 01 and 03 (parallel-safe; disjoint packages).
- Wave 2: task 02 (after 01) and task 04 (after 03).
- Wave 3: tasks 05, 06 and 08 (after 04). Tasks 05 and 06 also need task
  01's `agent.pgid` record.
- Wave 4: task 07.

## Verification results

Pending.

## Risks

- **PR #3598 moves under this branch.**
  - It is open, large, and still changing.
  - Every symbol here is verified at `0bef2668a`.
  - Re-verify the `lifecycle` and `mcp/server` citations after each rebase.
- **The #3598 items are prerequisites for a clean end state.** Without them:
  - a detached agent still stalls after 100 events;
  - a reconnect can meet the uncertain-delivery block.

  Task 04 must not work around either. If #3598 lands without them, file a
  follow-up rather than duplicating them here.
- **Harness tool timeouts.** Claude declares 2 h. Codex behavior for a
  long-waiting MCP call is unverified. Task 02 must measure it before relying
  on the budget.
- **Holding the prompt waiter** (task 03) changes a hot path. A missed exit
  would leave a session RUNNING forever. Task 03's tests must cover every
  exit.
- **Sprites** has no test account here. Task 07 stays blocked, not guessed.
