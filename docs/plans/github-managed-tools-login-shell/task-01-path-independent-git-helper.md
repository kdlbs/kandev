---
id: "01-path-independent-git-helper"
title: "Path-independent Git credential helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Path-Independent Git Credential Helper

## Acceptance

- Managed Git invokes the instance-owned `agentctl` helper even when the active shell `PATH` does
  not contain the helper executable, including an absolute helper path containing whitespace.
- The helper reset/order and fail-closed behavior remain unchanged; no ambient helper or prompt is
  used when the managed helper is missing or fails.
- Executor inheritance removes both current and legacy managed-helper entries while preserving
  unrelated indexed Git configuration in order.

## Verification

RED first:

```bash
cd apps/backend && rtk go test ./internal/orchestrator/executor -run 'TestConfigureGitHubCredentialBrokerHelperSurvivesPathReset' -count=1
```

GREEN and focused regression checks:

```bash
cd apps/backend && rtk go test ./internal/orchestrator/executor -run 'Test.*(GitHubCredential|ManagedGitHubHelper)' -count=1
```

## Files likely touched

- `apps/backend/internal/githubauth/environment.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the shared managed-helper and shim-environment constants established
here.

## Inputs

- Spec: managed runtime-tool behavior, security constraints, failure modes, and login-PATH scenarios.
- Plan: confirmed root cause and path-independent helper design.
- Existing patterns: `configureGitHubCredentialBrokerForRepositories`,
  `removeManagedGitHubCredentials`, and `gitconfigenv.Filter`.

## Output contract

Report the RED failure reason, final helper command and quoting behavior, legacy-removal behavior,
files changed, exact test results, security/secret boundary, and synchronized task/plan status.

## Results

- RED: `cd apps/backend && rtk go test ./internal/orchestrator/executor -run 'TestConfigureGitHubCredentialBrokerHelperSurvivesPathReset' -count=1`
  failed as expected with `agentctl: not found` and Git's terminal-prompt-disabled error.
- GREEN: the same regression test initially passed after the helper changed to the shim-directory
  variable. Task 03 superseded that lifetime assumption with a dedicated absolute helper-executable
  variable so preparation-time Git works before the shim directory exists.
- GREEN: `cd apps/backend && rtk go test ./internal/orchestrator/executor -run 'Test.*(GitHubCredential|ManagedGitHubHelper)' -count=1`
  passed (7 tests).
- The regression uses a temporary helper path containing whitespace and fake non-secret credentials;
  the test process owns and removes its temporary directory automatically. No credential values,
  leases, or tokens were persisted. Final `git diff --check` passed after Task 02 completed.
