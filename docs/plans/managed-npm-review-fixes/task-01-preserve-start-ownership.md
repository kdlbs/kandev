---
id: "01-preserve-start-ownership"
title: "Preserve repair ownership on rejected starts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.17
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 01: Preserve repair ownership on rejected starts

## Summary

Correct Start admission before any repair-handle or timer mutation.
A rejected duplicate request leaves the running process and pending publication usable.

## In scope

- Move detachment after admission, running/starting, passthrough, and revision rejection checks.
- Preserve the candidate handle, timer, initialization state, retained-owner list, and physical-cache evidence on rejection.
- Keep accepted stopped-process context misses able to detach a stale handle.
- Cover pending, already-published, closed-admission, and successor-generation schedules.

## Out of scope

- Command recognition, owner policy, toolchain caching, and a new retry allowance.

## Acceptance

1. `TestManagedRuntimeRepairRejectedStartPreservesTicket` starts a real owned candidate and rejects a second Start before initialize. Initialization still publishes and another process can acquire the context lock.
2. Rejected starts preserve selected-generation evidence and the original expiry deadline. A stale callback cannot detach or stop a successor.
3. Split `TestManagedRuntimeRepairNormalMissDetachesStaleHandle`: its current closed-admission expectation contradicts criterion 007.17. Only an accepted stopped-process miss detaches.

## Verification

Run sequentially from the repository root after the required failing regression.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'ManagedRuntimeRepairRejectedStart|ManagedRuntimeRepairNormalMiss|ManagedRuntimeRepairPublication|ManagedRuntimePublicationDeadline' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/process -run 'ManagedRuntimeRepairRejectedStart|ManagedRuntimeRepairNormalMiss|ManagedRuntimeRepairPublication' -count=1)
```

## Files likely touched

- apps/backend/internal/agentctl/server/process/manager.go
- apps/backend/internal/agentctl/server/process/managed_runtime_repair.go
- apps/backend/internal/agentctl/server/process/managed_runtime_repair_reuse_test.go
- apps/backend/internal/agentctl/server/process/managed_runtime_start_admission_test.go (new)
- apps/backend/internal/agentctl/server/api/agent.go and its initialize tests

## Dependencies

None.

## Risks

- Closing a live unpublished handle retains its files and lock. A rejected request must never become its cleanup owner.
- Preserve the stopped-process stale-handle correction from the original package.

## Parallelism

`sequential`

## Inputs

- Review findings 1 and 5, paired reuse design: Start admission and publication ownership.
- Existing process publication, expiry, configuration-revision, and teardown tests.

## Results

Done on 2026-11-09.

- Start detachment now runs only after admission, running/passthrough, revision, and journal rejection checks.
- Added rejected-start, closed-admission, accepted-miss, and stale-successor regressions.
- Focused process verification passed:
  `go test -trimpath -tags fts5 ./internal/agentctl/server/process -run 'ManagedRuntimeRepairRejectedStart|ManagedRuntimeRepairClosedAdmission|ManagedRuntimeRepairAcceptedNormalMiss|ManagedRuntimeRepairStaleExpiry|ManagedRuntimeRepairNormalMiss|ManagedRuntimeRepairPublication|ManagedRuntimePublicationDeadline' -count=1`.
