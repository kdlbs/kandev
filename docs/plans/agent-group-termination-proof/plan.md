---
created: 2026-11-02
status: implemented
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
- The durable prompt binding and the reconciliation identity record it as `OriginalAgentGroup`.
- `processidentity.OwnedGroupTerminated` for Linux and Darwin. Other platforms return
  `ErrUnverifiableIdentity`.
- Every original-owner termination check accepts runtime-session termination OR
  agent-group termination.

### Out of scope

- Remote executors, which keep authenticated surviving-execution evidence.
- UI changes. Resume and automatic recovery follow from the existing proof consumers.
- Repairing bindings recorded before this change. They keep the runtime-only rule.
- Containment or signalling changes.

## Technical approach

Work order 01, identity capture:
- `internal/common/processidentity`: no change to `Identity`. Its `GroupID` and `BirthToken`
  already describe a process group leader.
- agentctl (`internal/agentctl/server/instance`, `internal/agentctl/server/api/delivery_protocol.go`):
  after the agent process starts, capture `processidentity.Capture(pid)` and expose it as
  `agent_process` in the delivery status of that instance. Omit it until the child is running.
- The client (`internal/agent/runtime/agentctl`) decodes it on `DeliveryStatus`.
- Lifecycle (`durable_prompt_turn_recovery.go`, `delivery_reconciliation.go`,
  `delivery_record_identity.go`, and `models.AgentDeliveryRecovery` plus its silent-restore
  checkpoint copy) adds an optional `original_agent_group` next to `original_runtime`. Only a
  valid identity whose PID equals its GroupID is recorded.

Work order 02, proof evaluation:
- `processidentity.OwnedGroupTerminated(identity)` (Linux procfs scan, Darwin `processGroupsInSession`
  equivalent). The group counts as terminated when no live process has that PGID with a start
  time at or after the recorded birth. If the leader is alive, it returns `false`. Errors fail closed.
- `processidentity.OriginalOwnerTerminated(runtime Identity, group *Identity)` in
  `internal/common/processidentity`. It is shared code, so both agentctl's journal and
  lifecycle can import it. Every call site listed in the root cause uses it.

| Runtime | Group identity | Proof |
|---|---|---|
| Local standalone (shared agentctl) | Recorded | Group exit or runtime exit |
| Local standalone, legacy binding | Absent | Runtime exit only (unchanged) |
| Docker/SSH/Sprites/K8s agentctl | Recorded inside the remote host | Unchanged: remote evidence rules apply, and local group proof is not consulted |
| Windows | Any | Unverifiable (unchanged) |

## Tests

| Criterion | Evidence |
|---|---|
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20, 006.21 | `processidentity/group_termination_linux_test.go`: `TestOwnedGroupTerminated`. Covers a live group, an exited group, a reused PID with a different birth, and a malformed identity. |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.17, 006.20 | `lifecycle/delivery_record_group_proof_test.go`: `TestOriginalOwnerTerminatedByAgentGroup`. The runtime is alive and the recorded group has exited, so reconstruction reports `process_terminated: true`. It fails on main. |
| Legacy fallback | The same file: `TestOriginalOwnerLegacyBindingNeedsRuntimeExit`. |
| Retained journal path | `agentctl/journal/retained_group_proof_test.go`: `TestRetainedRecoveryAcceptsAgentGroupTermination`. |
| Capture | `agentctl/server/api` and `lifecycle` tests that assert the descriptor field and the binding round-trip. |

## E2E tests

None in this package. A shared-agentctl crash scenario needs the containers or kill-agentctl
harness. It is recorded as a follow-up for the E2E owner.

## Work orders

- [x] [Task 01: Record the agent process group](task-01-record-agent-group.md)
- [x] [Task 02: Accept agent group termination as proof](task-02-group-termination-proof.md)

## Verification results

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

## Risks

- Process-group reuse after PID wraparound is guarded by the birth token. A missing birth
  fails closed.
- Mixed-version agentctl peers omit the field. Recovery falls back to the runtime-only rule.
- An agent that spawns helpers outside its group is not covered. Containment already shares
  that boundary.
