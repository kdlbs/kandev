---
id: "03-network-budgets"
title: "Cover network callers and failure recovery"
status: done
wave: 3
depends_on: ["02-owned-cleanup"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.2
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.6
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 03: Cover network callers and failure recovery

## Summary

Every network caller has a recorded finite post-admission budget; selected credentials remain authoritative and subsequent recovery succeeds.

## In scope

Complete the caller inventory in plan.md using repository-wide construction, runner, and manual admission searches.
Record each network operation's execution budget, selected environment, admission class, and required/optional error owner.
An inherited context alone is not proof of a finite post-admission execution budget.

Migrate GitOperator and local checkout preparation to AfterAcquire builders.
Keep existing operation-specific budgets. For uncovered ordinary fetch/pull use 30 seconds; use five minutes for uncovered clone, submodule update, and push.
A shorter parent lifetime still wins. These defaults are internal operation constants, not new public settings.
Preserve worktree manager fetch/pull budgets, tracker ten-second commands, Office 30-second commands, and fresh-branch/refresh 30-second commands.

Keep repoclone's managed-helper isolation and existing transport selection. Preserve argument validation and push error sanitization.
Replace duplicate prompt implementations only after shared enforcement covers their direct starts.
Audit bootstrap, clone fetch/checkout, submodules, backendapp, and all network verbs including ls-remote.
Add focused tests for every modified caller package; record the exact added test names in Results.

Write `TestGitOperatorAuthenticationFailureAndRecovery` with a local HTTP fixture.
Assert a required failure returns a usable sanitized error and releases the operation lock, then restore fixture credentials and retry on the same operator.
Add `TestLocalCheckoutFetchDeadline` and `TestCloneDeadlineAfterAdmission` for the uncovered lifecycle routes.
Use existing scheduler test patterns for canceled queues and queue waits exceeding the execution budget before a successful admitted command.
Preserve fresh-branch's documented best-effort fetch behavior and repository refresh cooldown/singleflight cleanup.

### Merged host bridge compatibility

Reuse the real Git/fake CLI fixtures in `executor_host_gh_bridge_test.go` and `executor_host_gh_bridge_profile_test.go`.
Extend them to exercise the production final Git runner with the effective routed environment, not only raw `git credential fill`.
Add `TestExecutorHostGHBridgeNoninteractiveExecution` and `TestExecutorHostGHBridgeRecoveryAfterUnavailableProbe`.
Cover Local/Worktree host-helper success without global setup, selected profile credential directories, and an absolute CLI path containing spaces.
Preserve earlier user helpers, explicit public/enterprise token precedence, unrelated-host/remote exclusion, and managed-helper failure without host invocation.
Exercise launch/resume/prepared-workspace propagation and reconfiguration that removes an obsolete marker-owned helper while retaining user entries.

Separate two recovery cases. An installed helper which fails can succeed on a later operation in the same instance.
An absent bridge is re-evaluated by the existing launch/resume/prepared-start path; assert recovery on the same backend without adding per-command probes.
Use fake CLI responses and isolated configuration for these tests. Do not use a developer's stored login.
The local HTTP fixture remains necessary to prove successful authenticated network Git, while credential-fill tests prove host-specific bridge routing.

## Out of scope

Credential-authority changes, transport fallback, live-instance changes, and unrelated refactors.

## Acceptance

- Every network caller has a recorded finite post-admission budget; selected credentials remain authoritative and subsequent recovery succeeds.
- Run new behavioral tests before implementation and record the expected failure, then the passing result.
- Preserve the contracts and exclusions in the linked design.

## Verification

```bash
(cd apps/backend && go test ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/gitbootstrap ./internal/task/service ./internal/office/configloader ./internal/backendapp ./internal/worktree -count=1 -timeout=10m)
(cd apps/backend && go test ./internal/common/subproc ./internal/gitconfigenv -count=1 -timeout=5m)
(cd apps/backend && go test ./internal/orchestrator/executor ./internal/agent/runtime/environment ./internal/agent/runtime/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/api -count=1 -timeout=10m)
rg -n 'NewGitCommand|AcquireGit|RunGit|ExecGit' apps/backend --glob '*.go' --glob '!**/*_test.go'
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge_test.go` (available after dependency integration)
- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge_profile_test.go` (available after dependency integration)
- `apps/backend/internal/agentctl/server/process/manager_temp_test.go` (existing reconfiguration coverage)

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_auth_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local_test.go`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_auth_test.go`
- `apps/backend/internal/gitbootstrap/gitbootstrap.go`
- `apps/backend/internal/task/service/fresh_branch.go`
- `apps/backend/internal/task/service/repository_fetch.go`
- `apps/backend/internal/office/configloader/git.go`
- `apps/backend/internal/backendapp/ (network Git callers found by inventory)`

## Dependencies

02-owned-cleanup.

## Risks

Credential helper and process-lifecycle compatibility require real subprocess evidence. Do not infer success from environment assertions alone.

## Parallelism

`sequential`

## Inputs

- [Execution design](../../specs/platform/system-design/git-subprocess-execution.md).
- [Plan evidence and caller inventory](plan.md).
- Scoped AGENTS.md and relevant fix, TDD, and E2E skills.

## Results

Implemented after Task 02 on the verified PR #3635 descendant `9cc146ee21c296d553ae684531219a5c711a4213`.

Results: network-capable Git callers now choose finite post-admission budgets while preserving established operation-specific deadlines, selected credential environments, refresh cooldowns, and required/optional error ownership. GitOperator authentication failures release the operation lock and a later operation can recover. Comparison-target recovery re-evaluates unavailable targets only from an explicit fresh status request, and moving internal comparison refs are force-updated only at their exact cache ref.

Checks passed:

- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/gitbootstrap ./internal/task/service ./internal/office/configloader ./internal/backendapp ./internal/worktree -count=1 -timeout=10m)`
- `(cd apps/backend && go test ./internal/common/subproc ./internal/gitconfigenv -count=1 -timeout=5m)`
- `(cd apps/backend && go test ./internal/orchestrator/executor ./internal/agent/runtime/environment ./internal/agent/runtime/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/api -count=1 -timeout=10m)`
- Production Git source audit in `internal/common/subproc`.
