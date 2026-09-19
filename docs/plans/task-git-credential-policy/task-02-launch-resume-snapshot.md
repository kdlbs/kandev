---
id: "02-launch-resume-snapshot"
title: "Resolve launch and resume credentials with a session snapshot"
status: done
wave: 2
depends_on: ["00-indexed-git-config-composition", "01-policy-persistence"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Resolve Launch And Resume Credentials With A Session Snapshot

## Acceptance

- Initial launch and resume share one resolver for managed, executor-inherited, and explicit
  profile-token outcomes across every attached repository.
- Managed GitHub HTTPS resets inherited helpers and fails closed; executor inheritance injects no
  Kandev broker/helper/shim contract.
- Only a successful launch/resume persists the versioned, non-secret session credential snapshot;
  actor labels are known only for workspace identities Kandev can describe.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/task/models -run 'Test.*(GitHubCredential|GitCredential|Resume.*Credential|Launch.*Credential)'
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/models_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- focused launch/resume integration tests
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/services_github_broker_test.go`
- this task file and `plan.md`

## Dependencies

Tasks 00 and 01.

## Parallelism

Parallel-safe with Task 04 after Task 01: backend orchestration/model files and frontend
settings files are disjoint.

## Inputs

- Spec `Task Git credential resolution`, snapshot data model, failure modes, and scenarios.
- Existing `resolveAllRepoInfo`, `persistLaunchState`, and `persistResumeState` paths.
- Task 01's policy/actor descriptor.
- Task 00's structured Git config composer; managed helper reset/additions must append to the
  existing block rather than assigning an unrelated `GIT_CONFIG_COUNT`.

## Output contract

Report the exact precedence table, launch/resume call sites, snapshot JSON shape, durable-write
point, helper reset ordering, RED/GREEN results, files changed, and update task/plan status.
