---
id: "02-group-termination-proof"
title: "Accept agent group termination as proof"
status: in_progress
wave: 2
depends_on: ["01-record-agent-group"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.17
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 02: Accept agent group termination as proof

## Summary

Add `processidentity.OwnedGroupTerminated` and `OriginalOwnerTerminated(identity)`, and
use them in original-owner termination checks. An absent recorded agent group can supply
termination proof while shared agentctl remains active. Independent admission checks still apply.

## In scope

- Linux group absence proof. Darwin keeps runtime-session proof. Other platforms retain
  their existing unverifiable result.
- The call sites in `agentctl/journal/retained.go`, `agentctl/journal/retained_gap.go`,
  `lifecycle/delivery_record_evidence.go`, and `lifecycle/already_resumed_recovery.go`.
- Lifecycle record evidence evaluates original-owner proof when the execution ID remains listed.
- Silent restart uses agent-group proof for a live standalone row and original-owner proof for
  unknown row liveness. Existing execution, recovery-owner, and cleanup guards remain active.

## Out of scope

- Identity capture (task 01), remote executors, UI, and containment.

## Acceptance

1. With the runtime alive and the recorded group exited, reconstruction and retained
   recovery report the process as terminated, and restart recovery proceeds. A live
   standalone row can proceed on agent-group proof; unknown row liveness uses original-owner
   proof.
2. A live group, a reused PID with another birth, a missing group identity, or an
   unverifiable platform keeps today's result. Mismatched executions and successors remain
   protected.
3. `TestOriginalOwnerTerminatedByAgentGroup` fails on the current code before the production change.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/common/processidentity -run 'GroupTerminated|OriginalOwner')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'OriginalOwner|AgentGroup')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'HistoricalProcessTerminationAcceptsExitedAgentGroupForLiveExecution')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/orchestrator -run 'SilentRestoreSourceDeathProof|SilentRestoreSourceCleanupRejectsExistingWorkspaceSuccessor|SilentRestoreSourceCleanupKeepsWorkspaceSuccessor')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/journal -run 'Retained')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/common/processidentity ./internal/agentctl/journal ./internal/agent/runtime/lifecycle ./internal/orchestrator)
(bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go'))
(python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all)
```

## Files likely touched

- `apps/backend/internal/common/processidentity/processidentity_linux.go`, `processidentity_darwin.go`, `processidentity_other.go`, `processidentity_windows.go`
- `apps/backend/internal/common/processidentity/group_termination_linux_test.go` (new)
- `apps/backend/internal/agentctl/journal/retained.go`, `retained_gap.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_evidence.go`, `already_resumed_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_live_execution_linux_test.go`
- `apps/backend/internal/orchestrator/silent_restart_recovery.go`, `silent_restart_recovery_source_liveness_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_group_proof_test.go` (new)
- `apps/backend/internal/agentctl/journal/retained_group_proof_test.go` (new)

## Dependencies

Task 01.

## Risks

- A procfs snapshot cannot prove group absence. Linux uses the kernel group existence check.
- Existing groups, reused IDs, and permission errors cannot prove termination.
- Unreaped group members can delay proof. Darwin group inspection remains unsupported.

## Parallelism

`sequential`

## Inputs

- `durable-restart-reconciliation.md` and ADR `2026-11-02-agent-process-group-termination-proof.md`.
- Existing tests: `processidentity_linux_test.go`, `termination_linux_test.go`.

## Results

- `TestRetainedRecoveryAcceptsAgentGroupTermination` (journal) failed on main with
  `Should be true` (an exited agent group with a live runtime), then passed after the call
  sites moved to `processidentity.OriginalOwnerTerminated`.
- `TestOwnedGroupTerminated` and `TestOriginalOwnerTerminatedByAgentGroup` pass, covering a live
  group, an exited group, a missing birth, a non-leader, and the legacy runtime-only binding.
- Call sites switched: `agentctl/journal/retained.go` (two), `retained_gap.go`,
  `lifecycle/delivery_record_evidence.go`, and `already_resumed_recovery.go`.
  `retired_cleanup.go` keeps `OwnedSessionTerminated`, because it cleans up the runtime itself.
- `go test -trimpath -race -count=1 ./internal/common/processidentity ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/... ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite`: all `ok` (lifecycle 227s, sqlite 381s, process 319s).
- `GOOS=darwin` and `GOOS=windows` `go vet` of the changed platform packages: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`: passed.

Review corrections add surviving-child, changed-birth, previous-boot, retained recovery,
history-gap, registered-instance, and unknown-liveness regressions.

The source fix is in commit `d278ecd6ce52f2896bfd75647cb55fd158791510`. Its pre-commit hooks
passed: documentation catalog, architecture, specification structure, gofmt, and changed-package
Go lint. The first Go lint
attempt used default resources, reported zero issues, and timed out after five minutes. The same
hook passed with `GOMAXPROCS=4 GOMEMLIMIT=6GiB`. Commit-message validation passed. Local Go tests
were not run. Current-head CI is pending after the push.
