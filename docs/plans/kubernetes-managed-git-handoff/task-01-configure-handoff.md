---
id: "01-configure-handoff"
title: "Repair managed Git configuration handoff"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.8
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.9
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.10
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 01: Repair managed Git configuration handoff

## Summary

Make fresh and resumed agent configuration carry the current managed credential
contract and Kubernetes helper path. Start with failing producer-to-consumer
regressions; preserve explicit removal rather than inheriting old credentials.

## In scope

- Recreate the temporary boundary reproduction as permanent coverage, including
  initial `ToAgentExecution` with no refresh, existing workspace with a fresh
  `SetExecutionEnv`, and `prepareRestartedKubernetesAgentctl`.
- Exercise the actual configure request through agentctl `Configure` and
  `ConfigureWithEnvironment`, then run a Git subprocess from the effective child
  environment using synthetic credentials and deterministic helper/broker fixtures.
- Assert absent refresh, explicit nil/empty/partial refresh, changed lease,
  changed repository scope, missing/wrong helper path and malformed indexed config.
- Preserve ordinary profile environment, CA environment and user Git entries.
  Prove no stale lease/helper/reset pair survives an explicit removal, while a
  managed failure cannot fall through to an inherited helper.
- Normalize only executor-owned paths and retain scoped broker authorization.
  Do not infer freshness merely from a nonempty inherited credential field.
- Audit orchestrator initial and existing-workspace producers, and retain existing
  profile recovery and credential-removal regression coverage.

## Out of scope

Live acceptance and PR operations belong to task 02. No public API, schema,
profile-default, credential-policy, or cluster-resource ownership redesign.

## Acceptance

1. Meaningful regressions fail on the base for dropped credentials and incorrect
   executor helper, then pass through the actual configuration/process boundary.
2. Fresh, existing-workspace and restarted-agentctl paths carry the current
   contract; explicit replacement/removal cannot revive stale managed state.
3. Existing stale-removal tests remain semantically unchanged and pass, as do
   profile recovery, repository denial, TLS and targeted race checks.

## Verification

Run from the repository root; use a new test file rather than growing oversized
existing test files. Record exact test names and results after implementation.

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/githubauth ./internal/gitconfigenv -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/githubauth ./internal/gitconfigenv -count=1)
(cd apps/backend && golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/process/... ./internal/orchestrator/executor/... ./internal/githubauth/... ./internal/gitconfigenv/... --new-from-rev=d60274528129dc17d413357c104c9c427c4e7af9 --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before the fix, run only the new regression names and retain their expected
failure receipt. Expand package/lint scope if the implementation changes another
package. Run all listed commands after the final implementation change.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_managed_git_handoff_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_managed_git_handoff_test.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/githubauth/environment.go`
- Adjacent tests for changed producers and shared cleanup behavior.
- `apps/backend/internal/orchestrator/executor/session_coresidency_test.go`
  (test-only correction exposed by the required race run).

## Dependencies

None.

## Risks

Indexed Git reset/helper entries have order-sensitive semantics. A simple map
merge or unconditional inherited-credential preservation is insufficient.
Do not log synthetic fixtures in a way that would log actual credentials in
production. Keep runtime snapshots memory-only.

## Parallelism

`sequential`

## Inputs

- [Plan and reproduction](plan.md).
- [Owning design](../../specs/executors/system-design/kubernetes-task-pod.md#managed-git-environment-handoff).
- Existing `TestComposeExecutionRuntimeEnvironmentRemovesObsoleteManagedCredentials`
  and `TestManagerConfigureRemovesObsoleteManagedCredentialEnvironment`.
- `newConfigureCaptureAgentctlClient` and current Kubernetes task-pod fixtures.

## Results

- RED: permanent lifecycle cases failed for lost fresh credentials, the host
  helper path and retained generated helper/reset entries after explicit removal.
  Agentctl overlay removal also failed. The recovered-snapshot restart regression
  failed with a missing helper path before normalization was added.
- GREEN: the handoff regression now sends the real HTTP configure request into
  the actual agentctl process manager; fresh/replacement/removal cases and
  recovered restart pass. Real Git credential subprocess tests pass for both
  configure modes and deny a foreign repository. Existing stale-field removal
  regressions remain unchanged and pass.
- Full targeted non-race package run passed. Final race and lint runs also passed; results are
  recorded in the plan.
- The required race run exposed a pre-existing timing-sensitive assertion in
  `TestLaunchPreparedSession_ObservesWorkingSiblingOnAgentStart`; it also failed
  on the untouched base with `go test -race ./internal/orchestrator/executor -run
  '^TestLaunchPreparedSession_ObservesWorkingSiblingOnAgentStart$' -count=10`.
  The test now waits for asynchronous startup, checks both existing observations
  before process start, and seeds the task needed by its successful-start path.
  That exact race command passes. No co-residency production behavior changed.

PR #3909 review regressions first failed for the retained generated
`credential.useHttpPath=true` entry, mixed-case Git variable names, and removal
of an unowned legacy helper. The cleanup now uses adjacent generated entries
and inherited broker ownership, preserving user helpers and case-sensitive URL
subsections. Direct classifier and lifecycle tests cover these boundaries; the
process-manager test exercises legacy user-helper preservation through Configure.
The five-package race command above passed again after these fixes.
