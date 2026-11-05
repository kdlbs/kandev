---
id: "02-durable-checkpoint-lifecycle"
title: "Durable checkpoint lifecycle"
status: completed
wave: 2
depends_on:
  - "01-git-history-content"
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.4
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.5
  - AC-TASKS-TURN-CHANGES-004.8
  - AC-TASKS-TURN-CHANGES-004.9
  - AC-TASKS-TURN-CHANGES-004.10
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-change-preservation.md
---

# Task 02: Durable checkpoint lifecycle

## Summary

Preserve accepted Git objects before disposable-source loss. Retry bounded temporary failures without recapturing history or blocking successor writes after end acceptance.

## In scope

Agentctl object export, backend archive import/verification, durable source receipts, runtime scheduling, restart resumption, source/read leases, and bounded cleanup.

## Out of scope

Rendering caches, provider provisioning, new release toggles, old failed-row repair, and unqualified remote-platform support claims.

## Acceptance

1. Historical reads succeed from a verified archive after the original fixture executor is destroyed, with exact original endpoint identity.
2. Temporary preservation errors retain pending work and completed repository sources, bounded across restart by the original recovery window.
3. Successor admission, explicit Stop/Cancel, shutdown, duplicate notifications, and destruction respect their independent ordering and time bounds.

## Verification

Record a behavioral RED regression before production changes. Use real Git/database fixtures for source and cleanup claims.

```bash
cd apps/backend
go test -trimpath ./internal/task/changes ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'TurnGitHistory|TurnChangePreservation|TurnChange.*Shutdown' -count=1
go test -trimpath ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'TurnCheckpoint' -count=1
go test -trimpath -race ./internal/task/changes ./internal/agent/runtime/lifecycle -run 'TurnGitHistory|TurnChangePreservation' -count=1
```

Run changed-package Go lint with actual paths. For web commands, install workspace dependencies from `apps/` if this worktree lacks them.
Use the configured PostgreSQL conformance fixtures where applicable, or record missing coverage explicitly.

## Files likely touched

- `apps/backend/internal/task/changes/coordinator_finish.go`
- `apps/backend/internal/task/changes/coordinator.go`
- `apps/backend/internal/task/changes/git_archive.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_recovery.go`
- `apps/backend/internal/backendapp/turn_changes_runtime.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_export.go`
- `apps/backend/internal/agentctl/server/api/git_turn_changes.go`
- `apps/backend/internal/agent/runtime/agentctl/client_turn_changes.go`

Add focused adjacent regression files. Extract helpers rather than exceeding scoped size limits.

## Dependencies

01-git-history-content

## Inputs

- [System design](../../specs/tasks/system-design/turn-change-preservation.md)
- Root and scoped AGENTS guidance, relevant existing tests, and the accepted decision.

## Risks

Do not retain an archive through object alternates that depend on disposable source storage. Measure transfer and deduplicated growth.

## Parallelism

`sequential`

## Results

Preservation now runs during terminal processing before executor ref cleanup, imports the accepted object closure into backend-owned storage, and verifies object hashes, closure, refs, ownership, and endpoint trees.
Temporary failures retry inside a 90-second bounded budget and settle as explicitly pending. A durable retrier resumes the original accepted pair through a live execution, persists attempts and a three-minute deadline, publishes recovered summaries, and settles explicitly at the deadline. The terminal summary budget is 90 seconds, separate from the 20-second capture fence.
Pending rows keep their executor refs, successful repositories stay independent, duplicate imports reuse one pack and receipt, and restart no longer converts unverified pending rows to final loss before the recovery owner runs.

Behavioral evidence:

- `go test -trimpath ./internal/task/changes ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'TurnGitHistory|TurnChangePreservation|TurnChange.*Shutdown' -count=1`
- `go test -trimpath ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'TurnCheckpoint' -count=1`
- `go test -trimpath -race ./internal/task/changes ./internal/agent/runtime/lifecycle -run 'TurnGitHistory|TurnChangePreservation' -count=1`
- Desktop and mobile turn-changed-files E2E passed against the real common-Git read path.
- Regression coverage includes `TestPreservationRetrierRecoversTemporaryFailureAcrossPasses`, `TestPreservationRetrierSettlesExpiredWindowWithoutCallingExecutor`, `TestPreservationRetrierLeavesPendingWorkWhenExecutorIsGone`, and `TestTurnGitHistoryCommonSourceSurvivesLinkedWorktreeRemoval`.

Remaining gap: object-transfer and deduplicated-growth measurements still require a remote/disposable executor fixture; this work order used local Git and mock-executor evidence.
