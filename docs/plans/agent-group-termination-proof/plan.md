---
created: 2026-11-02
status: in_progress
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
legacy_specs: []
---

# Fix plan: prove original-owner termination per agent process group

## Overview

Durable delivery recovery needs proof that the original owner of an interrupted prompt
stopped. Today that proof is the termination of the shared local agentctl process. That
process serves every session and outlives the backend, so a session whose own agent has
exited can still never recover while other tasks run. This plan records the agent child's
process-group identity and accepts its termination as proof. Work order 01 carries the
identity through agentctl, the client, and the prompt binding. Work order 02 evaluates it in
every termination check. Work order 01 must land first, because the check needs recorded
evidence.

## Evidence and root cause

Task "Migrar para 3DF 7 e .NET 10" (session `933bd6fa`), 2026-11-02.

- The recovery metadata recorded `original_runtime` = PID 3161425, which was
  `agentctl -port=39429`. It started at 09:36, survived the 10:03 backend crash, and was
  parent to other tasks' agents (`atomica-code` PID 3271910, a Claude Code ACP session).
- The session's own agent execution `307c9d4e` was released at 10:04:22 ("idle reclaim").
- Reconstruction kept reporting `process_terminated: false`. Retry connection returned
  `outcome: uncertain`, and the UI offered only Retry connection and Stop. Two backend
  restarts that left agentctl alive changed nothing.
- Recovery moved on only after the operator killed agentctl 3161425, which also killed every
  other running agent.
- `ps` shows each agent child with its own process group (PGID = PID, from `Setpgid` in
  `agentctl/server/process/procattr_*.go`), inside the agentctl OS session (SID 3470833).

Root cause. `persistDurablePromptTurnBinding` (`lifecycle/durable_prompt_turn_recovery.go`)
and `captureDeliveryReconciliationIdentityWithClient` (`lifecycle/delivery_reconciliation.go`)
record `client.RuntimeProcessIdentity()`, the agentctl runtime. Every termination check calls
`processidentity.OwnedSessionTerminated` on that identity:
- `agentctl/journal/retained.go`
- `agentctl/journal/retained_gap.go`
- `lifecycle/delivery_record_evidence.go`
- `lifecycle/already_resumed_recovery.go`

That proof cannot succeed while the shared runtime lives.

This violates AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 (automatic reconciliation after
restart), 006.17 (explicit continuation after confirmed termination), and 006.21. No
requirement changes. The system design and
[ADR 2026-11-02](../../decisions/2026-11-02-agent-process-group-termination-proof.md)
define the group-level proof.

## Scope

### In scope

- agentctl reports the agent child's process-group birth identity in its authenticated
  delivery descriptor. This is an additive field.
- The durable prompt binding and reconciliation identity carry optional `AgentGroupID` and
  `AgentBirthToken` fields on `original_runtime`.
- `processidentity.OwnedGroupTerminated` provides Linux group proof. Darwin and other platforms
  return `ErrUnverifiableIdentity` for group inspection and retain their prior runtime proof.
- Original-owner checks accept runtime-session termination or agent-group termination.
- For a live standalone row, lifecycle uses only the exact agent-group proof because row
  liveness describes the shared agentctl instance. For unknown row liveness, lifecycle uses
  `processidentity.OriginalOwnerTerminated`.

### Out of scope

- Remote executors, which keep authenticated surviving-execution evidence.
- UI changes. Resume and automatic recovery follow from the existing proof consumers.
- Repairing bindings recorded before this change. They keep the runtime-only rule.
- Containment or signalling changes.

## Technical approach

Work order 01, identity capture:
- `internal/common/processidentity.Identity` adds optional `AgentGroupID` and `AgentBirthToken`
  fields. `WithAgentGroup` records only a group leader with a birth token.
- `process.Manager` captures the child identity after lifecycle installation and adapter
  connection succeed. It publishes `agent_process` in its delivery descriptor and clears it
  after `Wait`. Failed startup publishes no child identity.
- The client uses the shared `journal.RecoveryDescriptor` type and copies the child identity
  at both cache boundaries.
- Lifecycle stores the fields inside `original_runtime`. Existing prompt bindings, recovery
  metadata, silent-restore checkpoints, and journal requests carry them without a new field.

Work order 02, proof evaluation:
- Linux `OwnedGroupTerminated` requires a valid birth token and a group leader identity.
  A previous boot proves termination. On the same boot, `kill(-pgid, 0)` must return `ESRCH`.
  This operation sends no signal. Existing groups and permission errors cannot authorize
  recovery. Unreaped members can delay proof until the operating system removes the group.
- `OriginalOwnerTerminated(identity)` accepts runtime-session termination or Linux group
  absence. Unsupported optional group evidence preserves a known runtime result.
- Journal retained recovery, retained gap recovery, record evidence, and already-resumed
  recovery use this shared helper. Independent admission checks still apply.
- Lifecycle record evidence does not let a matching live execution ID block a successful
  original-owner proof. Silent restart uses agent-group proof for a live standalone row and
  the shared original-owner proof for an unknown row. Session and execution identity checks,
  owner revalidation, and conditional cleanup remain in force.

| Runtime | Group identity | Proof |
|---|---|---|
| Linux local standalone | Recorded | Group absence or runtime-session exit |
| Darwin local standalone | Any | Runtime-session exit only |
| Local legacy binding | Absent | Runtime-session exit only |
| Remote executors | Any | Existing authenticated remote evidence |
| Windows | Any | Unverifiable, unchanged |

## Tests

| Criterion | Evidence |
|---|---|
| Live group, exited group, surviving child, changed birth, previous boot | `internal/common/processidentity/group_termination_linux_test.go`: `TestOwnedGroupTerminated`, `TestOwnedGroupTerminationRequiresNoSurvivingChild`, `TestOwnedGroupTerminationRejectsChangedBirth` |
| Group proof and runtime-only binding | The same file: `TestOriginalOwnerTerminatedByAgentGroup` |
| Unsupported optional group proof on Darwin | `internal/common/processidentity/group_termination_darwin_test.go`: `TestOriginalOwnerPreservesDarwinRuntimeOnlyProof` |
| Retained reconstruction and ordinary retained recovery | `internal/agentctl/journal/retained_group_proof_linux_test.go`: `TestRetainedRecoveryAcceptsAgentGroupTermination` |
| Retained history-gap recovery | The same file: `TestRetainedGapAcceptsAgentGroupTermination` |
| Child identity lifecycle and failed startup | `internal/agentctl/server/process/agent_process_identity_linux_test.go`: `TestAgentProcessIdentityTracksRunningGroupLeader`, `TestAgentProcessIdentityAbsentAfterFailedStartup` |
| Client cache ownership | `internal/agent/runtime/agentctl/client_delivery_agent_process_test.go`: `TestDeliveryDescriptorOwnsAgentProcessIdentity` |
| Binding and reconciliation helper | `internal/agent/runtime/lifecycle/agent_process_group_test.go`: `TestWithAgentProcessGroupRecordsReportedAgentGroup`, `TestBoundAgentProcessGroupReusesPromptBinding` |
| Registered and unknown source-row recovery gates | `internal/agent/runtime/lifecycle/delivery_record_live_execution_linux_test.go`: `TestHistoricalProcessTerminationAcceptsExitedAgentGroupForLiveExecution`; `internal/orchestrator/silent_restart_recovery_source_liveness_test.go`: `TestSilentRestoreSourceDeathProof` |
| Successor remains protected during cleanup | `internal/orchestrator/silent_restart_recovery_source_liveness_test.go`: `TestSilentRestoreSourceCleanupRejectsExistingWorkspaceSuccessor`, `TestSilentRestoreSourceCleanupKeepsWorkspaceSuccessor` |

Paths in this table are relative to `apps/backend/`. The original contributor verification
results below do not cover the added review regressions.

## E2E tests

None in this package. A shared-agentctl crash scenario needs the containers or kill-agentctl
harness. It is recorded as a follow-up for the E2E owner.

## Work orders

- [x] [Task 01: Record the agent process group](task-01-record-agent-group.md)
- [ ] [Task 02: Accept agent group termination as proof](task-02-group-termination-proof.md)

## Contributor verification results

These results belong to the original contributor commit. They do not cover the later review fixes.

- `go test -trimpath -race -count=1 ./internal/common/processidentity ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/... ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite`: all `ok` (lifecycle 227s, sqlite 381s, process 319s).
- `GOOS=darwin` and `GOOS=windows` `go vet` of the changed platform packages: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`: passed.

Deviation from the plan: the agent group travels as two optional comparable fields on
`processidentity.Identity` (`agent_group_id`, `agent_birth_token`), set with `WithAgentGroup`.
There is no separate `OriginalAgentGroup` field, so the existing `original_runtime` plumbing
(bindings, recovery metadata, silent-restore checkpoints, journal requests) carries it
unchanged. Darwin group proof returns `ErrUnverifiableIdentity`, so Darwin keeps the
runtime-only rule.

## Review follow-up

The fixup addresses the remaining recovery gates in review thread
[4454081765](https://github.com/kdlbs/kandev/pull/5015#discussion_r4454081765). Record evidence
now evaluates original-owner proof even when the execution ID remains listed. Silent restart
uses agent-group proof when a standalone row is alive and combined original-owner proof when
row liveness is unknown. Tests cover both paths and retain the existing successor protections.
Fixup commit `d278ecd6ce52f2896bfd75647cb55fd158791510` passed the repository pre-commit
hooks, including Go lint, docs catalog, architecture, and specification checks. The default
Go lint resource settings timed
out after reporting zero issues; the same hook passed with `GOMAXPROCS=4 GOMEMLIMIT=6GiB`.
Local Go tests were not run. Current-head CI is pending after the push.

## Risks

- The birth token identifies the boot. Existing or reused group IDs cannot prove group
  absence. A missing birth token fails closed.
- Mixed-version agentctl peers omit the field. Recovery falls back to the runtime-only rule.
- An agent that spawns helpers outside its group is not covered. Containment already shares
  that boundary.
