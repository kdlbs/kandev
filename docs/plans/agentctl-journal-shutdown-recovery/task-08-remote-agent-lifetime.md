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

Authorized on 2026-10-10. Common lifecycle and plugin regressions reproduced the required failures; implementation is in progress. Final verification remains pending.

- The focused RED selection failed on missing built-in remote inventory, released recovery guards, remote shutdown stopping agents under both local-flag values, explicit Stop retaining a pending guard, and destruction of an established plugin environment with a missing token.
- Local Docker termination and incomplete plugin provisioning cleanup remained passing controls.
- The focused Sprites and SSH adapter tests passed without race instrumentation. They cover exact live attachment, transport cleanup, and retry. The final integrated race gate remains pending.
- Docker recovery, changed-port rejection during credential refresh, authentication-error classification, and persisted-target teardown passed their focused race selection. After fixture connection metadata and control-port forwarding were corrected, the Kubernetes-focused race selection passed in 1.135 seconds.
- The real SSH browser case now keeps the fixture backend offline beyond the mock response timer, checks the remote PID during the outage, and asserts unchanged conversation identity and once-only replay. Scoped ESLint, sleep lint, and Playwright test discovery passed; browser execution remains pending.
- Coordinator review found that plugin retries reused an obsolete checkpoint revision after a transient failure. Reconnect and pending Stop regressions reproduced the failure and pass after reloading the checkpoint for the same immutable owner.
- Partial replay coverage passes: the first event remains projected after a timeout, the recovery guard stays active, and the next attachment resumes from the saved cursor.
- Current SSH/Sprites hydration and pending Stop tests pass. Lifecycle lint reports zero issues after refactoring. The combined race run identified an SDK WebSocket race in the Sprites probe, a zero-port SSH fixture, and duplicate standalone inventory classification. The fixes passed the full lifecycle race suite (111.462 seconds). Runtime agentctl, orchestrator handlers, executor, and SQLite packages also passed. The remaining orchestrator gate and managed browser selections are pending.

Post-rebase backend race verification passed all six affected packages, including lifecycle and orchestrator. The managed SSH selection passed both graceful restart and hard backend outage scenarios with retries disabled (1.7 minutes). The latter expects the live idle `ready` state after the remote turn finishes, checks the exact remote PID with `kill -0`, preserves session and control-port identity, recreates the local forward, and verifies once-only output. The first attempt had a stale `running` expectation; no remote runtime defect was found. Final current-base integration remains pending after main advanced.

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

Final current-base validation after rebasing onto `43f55a6` passed both SSH scenarios with retries disabled: hard backend outage and graceful restart. The same fresh source also passed four phone and three focused desktop recovery cases. An initial SSH invocation stopped in global setup because the Linux mock-agent helper was stale; no tests ran. Rebuilding that helper through `build-mock-agent-linux` restored freshness, and the subsequent SSH run passed both cases without changing source or weakening the guard. Linux SSH process survival and authenticated replay were exercised; live Sprites, remote Docker, Kubernetes, plugin service infrastructure, and real provider CLIs remain outside local coverage. Task 06 owns current-head CI and delivery.
