---
id: "01-record-launch-proof"
title: "Record immutable SSH controller launch proof"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.18
  - AC-EXECUTORS-SSH-EXECUTOR-001.24
system_design:
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
---

# Task 01: Record immutable SSH controller launch proof

## Summary

Capture the controller's own process identity during authenticated launch.
Store the receipt in durable metadata and preserve it only for that controller.
This task leaves the current missing-directory refusal intact.

## In scope

- Add the versioned shared receipt and managed launch binding. Generate a fresh
  launch generation for every attempt and return self-captured evidence beside
  the bootstrap token. Keep non-SSH handshake compatibility.
- Pass the typed owner binding through `AgentctlStartupConfig`. Reject a receipt
  with another PID, generation, owner, path, or SSH target binding.
- Carry the receipt through `sshSessionState`, `buildInstance`, recovery state,
  `persistentMetadataKeys`, and the existing registration persistence barrier.
- Clear it in replacement/reset paths, including broker refresh and failed
  launch retries. Exclude it from `buildRunningFromExecution` prior-row fallback.
- Keep the private launch marker out of provider subprocess environments.

## Out of scope

Remote signalling, forced shutdown, cleanup-state changes, public docs, and
proof creation for an already-running legacy process.

## Acceptance

1. `TestHandshakeSSHLaunchReceiptBindsSelfAndOwner` proves the receipt identifies
   the responding process and the generated binding. Missing or altered proof
   fails a new supported Linux SSH launch before successful publication.
2. `TestSSHLaunchReceiptSurvivesPersistenceAndResume` enters through the launch
   producer and SQL serialization. Same-controller resume preserves the receipt
   despite a new local execution ID. No test supplies a receipt only at its consumer.
3. `TestSSHLaunchReceiptReplacementDoesNotInheritPriorProof` covers replacement,
   broker reset, handshake retry, absent current metadata, and prior-row merge.
   Receipt persistence failure uses the current rollback path and retains stop
   failures. Non-SSH and legacy resume behavior remains valid.

## Verification

Run from the repository root. Add new focused tests before production changes.

```sh
(cd apps/backend && go test -trimpath -race ./internal/common/sshlaunch ./internal/common/config ./internal/agentctl/server/config -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/api -run 'TestHandshake|TestSSHLaunch' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHLaunchReceipt|TestBuildRunningFromExecution|TestSSH.*(Resume|Broker|Handshake)|TestRetryAgentctlHandshake' -count=1)
```

Before completion, run `bash scripts/lint-go-changed` with every Go file changed
by this work order. Pass explicit paths from the repository root.

## Files likely touched

- `apps/backend/internal/common/sshlaunch/receipt.go` and `receipt_test.go` (new)
- `apps/backend/internal/common/config/agentctl.go` and `agentctl_test.go` (new test file)
- `apps/backend/internal/agentctl/server/api/control_server.go`
- `apps/backend/internal/agentctl/server/api/handshake_ssh_launch_test.go` (new)
- `apps/backend/internal/agentctl/server/config/config.go` and its tests
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_launch_receipt_test.go` (new)

## Dependencies

None. Read the paired design's launch-receipt and ownership sections first.

## Risks

Generic metadata merging can restore a stale receipt. Port retries and
same-session replacement can also retain mutable launch state accidentally.
Use immutable copies and test the production producer-to-persistence boundary.

## Parallelism

`sequential`

## Inputs

- [SSH requirements](../../specs/executors/requirements/ssh-executor.md), AC-001.18 and AC-001.24
- [Reconciliation design](../../specs/executors/system-design/ssh-agentctl-reconciliation.md)
- [Launch-proof decision](../../decisions/2026-11-07-ssh-controller-launch-proof.md)
- `apps/backend/AGENTS.md` and scoped agentctl/API guidance
- `persistence_test.go`, `executor_ssh_instance_identity_test.go`, and `handshake_test.go` as existing patterns

## Results

Implemented.

- `internal/common/sshlaunch` owns the versioned receipt, owner/target binding,
  generation generation, metadata projection, and closed reconcile wire types.
  The receipt carries the controller's self-captured `processidentity.Identity`
  and effective UID, never a token or environment dump.
- `AgentctlStartupConfig.SSHLaunch` carries the per-attempt binding to the
  managed agentctl child. `config.Config.SSHLaunchBinding` exposes it to the
  control server, and `buildLaunchReceipt` captures the responding process
  during the nonce-authenticated handshake. The response adds `launch` beside
  the token; an unbound launch omits it.
- `SSHExecutor.startAgentctlAndHandshake` creates a fresh generation per
  attempt on a Linux remote, sets `KANDEV_INTERNAL_SSH_LAUNCH_GENERATION`, and
  rejects a missing, mismatching, or wrong-PID receipt before publishing the
  launch. macOS launches issue no proof because the pidfd helper is Linux-only;
  they keep the legacy guarded stop and confirmed-absence paths. The launch
  marker is stripped from agent subprocess environments by
  `launchwrapper.StripControlCredentials`.
- The receipt flows through `sshSessionState`, `buildInstance`,
  `buildInstanceForLostRace`, `buildResumedInstance`, recovery state,
  `persistentMetadataKeys`, and `sessionScopedMetadataKeys`. Reset paths clear
  it through `clearSSHResumeRuntimeMetadata`, and `buildRunningFromExecution`
  excludes it from prior-row fallback. An unmanaged/direct caller with
  `Configured=false` cannot carry a binding and keeps the legacy launch shape.

Tests (all passing):

- `TestSSHLaunchReceiptSurvivesPersistenceAndResume`
- `TestSSHLaunchReceiptReplacementDoesNotInheritPriorProof`
- `TestSSHLaunchReceiptProducerBindsGenerationAndSelf`
- `TestSSHLaunchReceiptAbsentForNonLinuxPlatforms`
- `TestHandshakeSSHLaunchReceiptBindsSelfAndOwner`
- `TestHandshakeWithoutBindingOmitsLaunchReceipt`
- `internal/common/sshlaunch` unit tests for schema, metadata round trip,
  generation uniqueness, and binding validation.

Verification:

```sh
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/sshlaunch ./internal/common/config ./internal/common/launchwrapper ./internal/agentctl/server/config -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/api -run 'TestHandshake' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHLaunchReceipt' -count=1)
```

`bash scripts/lint-go-changed` reported no issues for every file changed by
this work order.
