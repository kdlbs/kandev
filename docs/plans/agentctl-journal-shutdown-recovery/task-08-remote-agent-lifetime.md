---
id: "08-remote-agent-lifetime"
title: "Preserve remote agents across backend shutdown"
status: completed
wave: 6
depends_on:
  - "05-silent-restart-recovery"
  - "07-long-outage-retention"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.6
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 08: Preserve remote agents across backend shutdown

## Outcome and ownership

Keep local/worktree shutdown terminating by default and preserve remote agentctl plus active agents when the backend stops.
Own runtime lifecycle shutdown, remote adoption, relevant backendapp wiring, and focused lifecycle/browser tests after Task 05 integration.
Audit Sprites, SSH, remote Docker, Kubernetes, and plugin remote separately. A saved environment alone is not evidence that its agent survives.
Core changes belong here; report any necessary provider-plugin repository change before editing another repository.

## Required behavior

Keep `features.agentSurvival` off in shipped profiles and scoped to local/worktree opt-in.
Remote survival is independent of that flag. Backend shutdown closes host-side connections without stopping remote processes or deleting instances.
Explicit Stop still cancels/stops its exact agent; archive, reset, and deletion retain authorized cleanup semantics.
Do not classify a local Docker daemon as remote solely because it uses a container.
On startup, adopt the recorded authenticated remote owner and replay without ACP initialize/load/new/prompt for an already initialized live agent.
Refresh transport credentials without replacing compute or native identity. Backend-only operations may wait while disconnected.
Provider expiry or missing compute remains a separate failure, never an excuse to silently recreate work.

## Acceptance

1. Shutdown tests prove local default termination, local opt-in survival, and remote survival with the local flag both false and true. Explicit Stop tests prove remote agents remain stoppable.
2. A real detached agentctl continues output after backend disconnect and reconnects after restart with unchanged execution/native identity, no new prompt, and complete ordered replay.
3. The Sprites adapter path receives explicit coverage, including proxy teardown and startup adoption. Record provider-backed smoke evidence when credentials are available; report unavailable provider validation separately.

## Verification

Observe the failing lifecycle regression before changing production code.
Run from `apps/backend`:

```bash
GOMAXPROCS=2 go test -p 1 -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/orchestrator -count=1
go test ./internal/runtimeflags ./internal/common/config ./internal/profiles
```

Extend and run the relevant managed executor browser selection with retries disabled, sequentially after Task 05's desktop/phone selections.
Record exact test names, provider boundary exercised, environment identity, and prompt count.
Update public executor and session docs without claiming an untested provider guarantee.
Update the root engineering guide to describe `features.agentSurvival` as a local/worktree opt-in, independent of remote lifetime.

## Results

Task 08 implementation and local lifecycle verification are complete. Task 06 owns current-head integration and CI; these results do not establish that the PR is green.

- The initial focused regressions exposed missing built-in remote inventory, remote shutdown stopping agents under either local-flag value, pending guards after explicit Stop, and loss of an established plugin environment when a transient token was unavailable. The fixes preserve local Docker termination and incomplete-plugin cleanup. Focused Sprites and SSH adapter tests cover exact live attachment, local transport cleanup, and retry.
- Lifecycle race coverage passed after fixing the Sprites probe WebSocket race, SSH zero-port fixture metadata, and duplicate standalone inventory classification. Focused Docker credential-refresh and persisted-target teardown races passed. Kubernetes ownership metadata and control-port forwarding regressions passed their focused race selection. Runtime agentctl, orchestrator handlers, executor, SQLite, and the six affected post-rebase backend packages also passed their recorded race selections.
- Plugin retry and pending-Stop regressions now reload the checkpoint for the same immutable owner after a transient failure. Partial replay coverage confirms that already-projected output remains visible, the recovery guard stays active after timeout, and a later attachment resumes from the saved cursor.
- The managed SSH recovery selection passed with retries disabled for graceful restart and hard backend outage. The outage case verified the remote PID while the backend was offline, unchanged conversation identity and control port, recreation of the local forward, and once-only output. Its first attempt had a stale `running` expectation; the live idle `ready` state was correct after the remote turn completed.
- A separate deletion-cleanup regression reproduced a completed SSH agent left alive after backend restart and task deletion removed its session row. The fix allows persisted-row cleanup only for explicit task deletion, only when the missing-session sentinel is returned, and only for one exact unchanged executor owner. Focused positive and negative lifecycle tests passed normally and under race; scoped lifecycle lint reported zero issues. A fresh managed retries-zero mock-agent browser equivalent then completed an SSH task, deleted its disposable profile, restarted the backend, ran `e2eReset`, and verified both that the remote PID stopped and that the SSH session row was retired (`1 passed`, 1.3 minutes; log `/tmp/pr4380-ssh-task-delete-mock-equivalent-fresh.log`).
- The existing managed OpenCode npm recovery case was discovered and attempted with retries disabled on a fresh build, but it stopped before the test body. Setup could not obtain OpenCode's `effective_version` metadata, so this run does not verify the npm startup path. The mock-agent browser equivalent verifies the task-deletion cleanup sequence only.
- The current real Kind reproduction remains red. The 60-second “Sibling survived restart” wait expired while the Pod stayed Running and Ready with the same UID, execution, and owner. The user's new message and turn persisted without creating a delivery submission; the retryable session-recovery guard was surfaced as a terminal agent error, and the later lifecycle retry failed closed because it had no pending-delivery proof. Task 06 owns the pending retryable-guard/queue integration fix. There is no green current Kind proof yet.
- No live Sprites provider smoke or real-provider CLI validation was available locally. Provider-backed behavior remains distinct from the local adapter and mock-agent coverage above.

### Initial integration findings

The following findings describe the implementation before Task 08 changes. Final test results above track their resolution.

- `executor_sprites.go`, `executor_ssh.go`, `executor_remote_docker.go`, and `executor_kubernetes.go` currently return no startup instances. Existing resume helpers are not automatic live adoption.
- Startup inventory in `manager_lifecycle.go` only loads standalone and plugin-remote records. Built-in remote rows must reach the recovery guard and their adapters. Existing `ListExecutorsRunning` can supply records without a new schema.
- `manager_interaction.go` detaches plugin-remote unconditionally and standalone only with the survival flag. Other built-in remote agents still enter the agentctl stop path.
- Docker metadata also uses `is_remote`; that key alone cannot distinguish a local Docker daemon from a remote executor.
- Existing Sprites reconnect can replace an instance for credential refresh or provision a missing sandbox. Live adoption must not call those replacement paths.
- Audit `manager_lifecycle.go` recovery deadline and unreconstructable-stop paths. A transient proxy/authentication outage does not prove that a preserved remote process is dead and must not kill it.
- Cover the full `StopAllAgents` and `Manager.Stop` sequence. `executorRegistry.CloseAll` runs after detach, so every adapter must close local transports without killing preserved remote processes. Sprites currently has tracked proxies but no `Close` implementation.
- `executor_plugin_recovery.go` currently destroys an attachable recorded environment when its transient auth token is empty. Distinguish incomplete provisioning cleanup from an established live record whose local secret is temporarily unavailable; the latter must remain preserved and blocked.
- Existing plugin attachment constructs `ExecutorInstance` without live `ProviderSessionID`, environment/source roots, or `DeliveryStatus`. All remote adoption paths must supply authenticated live evidence before `restoreRecoveredDelivery`; a transport readiness check alone does not establish a recoverable conversation.

These findings define implementation boundaries, not completed verification.

### Retry ownership

The lifecycle manager owns pending remote adoption. The existing remote-status loop retries a bounded, rotating set of saved records with a deadline for each attempt. Shutdown cancels and joins this work. A retained recovery guard blocks fresh launches while connectivity or authenticated identity remains unknown.

Each retry checks the saved execution, task, session, and environment identity before attachment and before tracking. Stop, archive, cleanup, or ownership changes fence stale retries. Successful adoption shares the startup replay path and releases the guard only after tracking succeeds. Failed attachment closes temporary host-side clients and preserves remote resources.

A narrow read-only task-service snapshot supplies current session, workspace authorization, archive, cleanup, and environment ownership fields. This projection uses existing durable records and introduces no schema. Missing snapshot support blocks adoption rather than assuming ownership.
