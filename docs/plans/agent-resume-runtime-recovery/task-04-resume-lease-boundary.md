---
id: "04-resume-lease-boundary"
title: "Repair resume lease-boundary ordering"
status: done
wave: 3
depends_on:
  - "02-resume-state-before-credentials"
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 04: Repair resume lease-boundary ordering

## Acceptance

- A `FAILED` or `CANCELLED` managed-GitHub session is guardedly persisted as
  `STARTING` before `buildResumeRequest` invokes the credential lease issuer.
- Request preparation that depends on the prior session state retains its
  existing behavior, and `LaunchAgent` does not perform a duplicate state
  transition.
- A credential issuance or later request-build failure rolls a still-`STARTING`
  session back without overwriting a concurrent terminal transition.

## Verification

- `cd apps/backend && go test ./internal/orchestrator/executor -run 'TestResumeSession_(PersistsStartingBeforeCredentialLease|RollsBackStartingWhenCredentialLeaseFails)' -count=1`
- `cd apps/backend && go test ./internal/orchestrator/executor -run 'TestResumeSession|TestRollbackResumeState' -count=1`
- `cd apps/backend && go test ./internal/orchestrator -run 'TestResumeTaskSession' -count=1`
- `cd apps/backend && go test ./internal/backendapp -run 'TestGitHubBrokerScopeAuthorizer' -count=1`
- `cd apps/backend && go test -race ./internal/orchestrator/executor -run 'TestResumeSession_(PersistsStartingBeforeCredentialLease|RollsBackStartingWhenCredentialLeaseFails|ConcurrentResume)' -count=1`

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_test.go`
- `docs/specs/agents/requirements/agent-resume-runtime-recovery.md`
- `docs/plans/agent-resume-runtime-recovery/plan.md`

## Dependencies

Task 02. This task corrects the lease-boundary assumption in that completed
implementation.

## Parallelism

Sequential. Production and regression-test changes share the resume lifecycle
path.

## Inputs

- Repair spec `Persistence and security constraints` and amended regression
  scenarios
- Plan section `Repair the credential-boundary ordering regression`
- `Executor.ResumeSession`, `buildResumeRequest`, `persistResumeState`, and
  `rollbackResumeStateAfterLaunchFailure`
- `githubBrokerScopeAuthorizer` terminal-session rejection, which remains
  unchanged

## Output contract

Report red/green evidence at the credential issuer, rollback and concurrency
behavior, exact test results, files changed, blockers, and risk notes. Mark
this task `done`, its plan checkbox complete, and the repair spec `shipped` in
the same conversation.
