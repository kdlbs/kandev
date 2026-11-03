---
id: "02-group-termination-proof"
title: "Accept agent group termination as proof"
status: done
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

Add `processidentity.OwnedGroupTerminated` and `OriginalOwnerTerminated(runtime, group)`, and
use them in every original-owner termination check. A session whose recorded agent group
has exited becomes recoverable while the shared agentctl keeps running.

## In scope

- Linux and Darwin group termination checks. Other platforms stay unverifiable.
- The call sites in `agentctl/journal/retained.go`, `agentctl/journal/retained_gap.go`,
  `lifecycle/delivery_record_evidence.go`, and `lifecycle/already_resumed_recovery.go`.

## Out of scope

- Identity capture (task 01), remote executors, UI, and containment.

## Acceptance

1. With the runtime alive and the recorded group exited, reconstruction and retained
   recovery report the process as terminated, and restart recovery proceeds.
2. A live group, a reused PID with another birth, a missing group identity, or an
   unverifiable platform keeps today's result.
3. `TestOriginalOwnerTerminatedByAgentGroup` fails on the current code before the production change.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/common/processidentity -run 'GroupTerminated|OriginalOwner')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'OriginalOwner|AgentGroup')
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
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_group_proof_test.go` (new)
- `apps/backend/internal/agentctl/journal/retained_group_proof_test.go` (new)

## Dependencies

Task 01.

## Risks

- A procfs race during the scan must count an exiting process as gone only on ESRCH or ENOENT.
- Darwin process-group enumeration needs the same birth revalidation as session enumeration.
  If that is unavailable, Darwin stays unverifiable.

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
