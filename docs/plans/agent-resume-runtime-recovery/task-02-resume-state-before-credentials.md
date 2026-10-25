---
id: "02-resume-state-before-credentials"
title: "Transition resume state before credential issuance"
status: completed
wave: 2
depends_on:
  - "01-preserve-resume-token"
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 02: Transition resume state before credential issuance

## Acceptance

- A failed or cancelled session is guardedly persisted as `STARTING` before
  `LaunchAgent` can request a GitHub credential lease.
- Terminal stale-execution cleanup and concurrent-resume protection retain
  their existing behavior after the earlier state mutation.
- Launch failure rolls back a still-`STARTING` session without overwriting a
  concurrent terminal transition.

## Verification

- `cd apps/backend && go test ./internal/orchestrator/executor -run 'TestResumeSession'`
- `cd apps/backend && go test ./internal/orchestrator -run 'TestResumeTaskSession'`
- `cd apps/backend && go test ./internal/backendapp -run 'TestGitHubBrokerScopeAuthorizer'`

The repository-observing launch regression must first fail because it sees the
terminal state at the credential boundary, then pass after the transition is
moved.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_test.go`
- `apps/backend/internal/orchestrator/task_operations_resume_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. It shares the session-recovery state contract established by Task
01 and must land after it.

## Inputs

- Repair spec `Persistence and security constraints`
- Plan section `Persist STARTING before credential issuance`
- Existing `persistResumeState`, `updateSessionStarting`,
  `validateAndLockResume`, and terminal stale-cleanup tests
- Existing credential-broker terminal rejection in
  `apps/backend/internal/backendapp/services.go`

## Output contract

Report the red/green test evidence, exact state transition and rollback
ordering, concurrency behavior, exact test results, files changed, blockers,
and risk notes. Mark this task `done` and update its plan checkbox in the same
conversation.
