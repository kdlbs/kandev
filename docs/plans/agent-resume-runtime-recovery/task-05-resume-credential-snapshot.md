---
id: "05-resume-credential-snapshot"
title: "Persist resume credential snapshot"
status: done
wave: 4
depends_on:
  - "04-resume-lease-boundary"
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 05: Persist resume credential snapshot

## Acceptance

- A successful managed-GitHub resume persists the current non-secret
  `git_credential_snapshot` after credential setup and before `LaunchAgent`.
- The metadata write is guarded by the session's current `STARTING` state and
  cannot overwrite a concurrent terminal transition.
- A metadata persistence failure aborts launch and uses the existing guarded
  resume rollback path.

## Verification

- `cd apps/backend && go test ./internal/orchestrator/executor -run 'TestResumeSession_(PersistsCredentialSnapshot|RollsBackWhenCredentialSnapshotPersistenceFails|DoesNotLaunchWhenCredentialSnapshotPersistenceLosesTerminalRace)' -count=1`
- `cd apps/backend && go test ./internal/orchestrator/executor -run 'TestResumeSession|TestRollbackResumeState' -count=1`
- `cd apps/backend && go test -race ./internal/orchestrator/executor -run 'TestResumeSession_(PersistsCredentialSnapshot|RollsBackWhenCredentialSnapshotPersistenceFails|DoesNotLaunchWhenCredentialSnapshotPersistenceLosesTerminalRace|ConcurrentResume)' -count=1`

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_test.go`
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go`
- `docs/specs/agents/requirements/agent-resume-runtime-recovery.md`
- `docs/plans/agent-resume-runtime-recovery/plan.md`

## Parallelism

Sequential. The metadata write shares the resume lifecycle and state guard with
Task 04.

## Output contract

Report red/green evidence, the expected-state guard behavior, exact targeted
test results, and whether any public documentation changed.
