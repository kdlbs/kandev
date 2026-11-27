---
id: "01-preserve-credential-path"
title: "Preserve repository paths in checkout credentials"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.3
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
---

# Task 01: Preserve repository paths in checkout credentials

## Summary

Preserve the executor's global `credential.useHttpPath=true` through the
worktree credential filter. Prove that Git retains the repository path and
selects the existing authorized scope.

## In scope

- Add `TestBuildWorktreeCreateRequestPreservesGlobalCredentialPath` using
  the exact indexed configuration emitted by `configureGitCredentialEnvironment`.
- Retain the host-specific test and exclusions for profile controls and secrets.
- Add an isolated real-Git regression named
  `TestCheckoutCredentialEnvironmentRealGitPath` in a focused lifecycle test file.
  Consume the real filter output, then invoke Git's credential protocol with
  the actual agentctl helper and a local broker that returns synthetic credentials.
- Retain or extend authenticated fetch coverage in the worktree package when
  extracting the diagnostic fixture. Exercise repeated checkouts and exact SHA.
- Make the smallest filter correction after the regression fails as expected.

## Out of scope

Metadata inheritance, credential issuance, scope authorization, automatic
recovery, task persistence, UI, and external GitHub test dependencies.

## Acceptance

1. The global true setting survives alongside host-specific true settings.
   Managed helper ordering and unrelated environment exclusions remain intact.
2. With isolated Git configuration, real Git reaches the broker using the exact
   scoped repository path. An incorrect repository still fails before broker access.
3. Repeated same-repository contribution preparation retains the validated SHA.
   No lease, binding, or branch-policy change is necessary for the correction.

## Verification

Start with the focused regression and record its expected failure. Build any
helper fixture into a test-owned temporary directory. Do not depend on the
diagnostic binary under `/tmp` or on developer Git configuration.

Run these commands from the repository root after the correction:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorktreeCreateRequest|CheckoutCredentialEnvironment)' -count=1)
(cd apps/backend && go test -tags fts5 ./cmd/agentctl -run '^TestGitHubCredentialHelper' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/worktree -run '^TestCreateWorktree_RemoteContribution' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/worktree-credential-path
```

Name any new worktree regression with the documented prefix so this command
includes it. Record every command and actual result before marking the task done.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/repository_checkout_options.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree_env_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/checkout_credential_path_test.go` (new)
- `apps/backend/internal/testutil/agentctl.go` (shared test helper)
- This work order and its plan status/results.

## Dependencies

None. The user explicitly requested implementation after the design package was completed.

## Risks

The current test fixture uses a host-specific key and hides the production
mismatch. Real Git tests must isolate HOME, global/system configuration,
indexed Git configuration, credential helpers, and network destinations.
Synthetic test credentials must never depend on live credentials.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/remote-contribution-tasks.md), criteria 001.2 and 001.3.
- [System design](../../specs/tasks/system-design/remote-contribution-tasks.md), Permissions and Persistence guarantees.
- [Diagnostic evidence](plan.md#evidence-and-root-cause).
- `executor_credentials.go:configureGitCredentialEnvironment` for emitted configuration.
- `github_credential.go:newGitHubCredentialBrokerClientForInput` for scope matching.
- `env_preparer_worktree_env_test.go` for filtering test patterns.
- `manager_remote_contribution_test.go` for Git fixture patterns.

## Results

Completed on 2026-10-01. The filter preserves the executor's global
`credential.useHttpPath=true`; the existing host-specific true value remains
supported, and profile controls and unrelated Git settings remain excluded.
The isolated real-Git test passes for the scoped repository and confirms a
different repository is rejected before broker access. The worktree test
passes three authenticated contribution preparations at the exact provider
head SHA. All targeted tests, documentation checks, and `make -C apps/backend
build` passed; both regression tests were confirmed failing before the filter
change.

## PR review hardening results

Added `TestCheckoutCredentialEnvironmentThroughWorktreeCreate` to pass the
production-filtered request into `worktree.Manager.Create` and verify three
authenticated fetches, broker scope, and the resulting head SHA. Removed the
redundant worktree-only fetch fixture. Moved the shared agentctl fixture
builder into `internal/testutil`, removed the nonexistent
`SetupScriptEnvironment` alternative from the lifecycle test pattern, and
cleared/restored inherited Git location and indexed-config variables in the
integration test. The shared builder disables Go VCS stamping because the
credential-helper binary does not need repository metadata and CI can lack a
usable Git status context during nested builds.

These commands passed after the review changes:

```bash
(cd apps/backend && env GIT_DIR= GIT_WORK_TREE= GIT_INDEX_FILE= GIT_OBJECT_DIRECTORY= GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.helper GIT_CONFIG_VALUE_0=/bin/false go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestCheckoutCredentialEnvironment(RealGitPath|ThroughWorktreeCreate)$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorktreeCreateRequest|CheckoutCredentialEnvironment)' -count=1)
(cd apps/backend && go test -tags fts5 ./cmd/agentctl -run '^TestGitHubCredentialHelper' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/worktree -run '^TestCreateWorktree_RemoteContribution' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run '^TestConfigureGitHubCredentialBroker$' -count=1)
(cd apps/backend && go test ./internal/testutil)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
make -C apps/backend build
```
