---
id: "01-reconcile-prepared-workspace-origins"
title: "Reconcile prepared-workspace origins"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.11
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.12
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/integrations/system-design/github-authentication-03.md
---

# Task 01: Reconcile Prepared-Workspace Origins

## Summary

Run the existing repository-preparation pass before `LaunchPreparedSession` enters its
existing-workspace branch. Reuse the resolved repository set if the request falls through to the
full launch path.

## In scope

- Add the fast-path regression before production changes.
- Prove that origin convergence finishes before the agent starts.
- Prove that an already-canonical origin has no write.
- Prove that one launch prepares each attached repository once.
- Wrap the real cloner to count calls while preserving its Git inspection and no-op behavior.
- Hoist and reuse the existing repository-resolution result.
- Preserve fail-closed repository-preparation errors.

## Out of scope

- Implementing or changing #3069 protocol detection, which is supplied by PR #3078.
- Adding #3072 host `gh` credential bridging.
- Changing `repoclone.Cloner.SetOriginURL` no-op or serialization behavior.
- Changing user-managed local checkout behavior.

## Acceptance

- The regression fails before the correction because the prepared-workspace branch starts the
  agent with a stale managed origin.
- After the correction, all attached managed GitHub origins are canonical before agent start.
- One launch prepares each attached repository once, and an already-canonical origin has no write.

## Verification

```bash
go test -tags fts5 ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart$' -count=1
go test -tags fts5 ./internal/orchestrator/executor ./internal/repoclone
```

Run these commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`

## Dependencies

- PR #3078, originally headed by commit `b976260e69b5abf421cfa9742dfc5c37d533ac0f`, is merged
  into `main` as `e4d17b3925` and supplies the dynamic, host-aware protocol resolver. Use its
  context-aware `RepoCloner` contract.

## Risks

- The asynchronous agent-start test needs a bounded completion signal. It must not use a fixed
  sleep.
- The context-aware #3069 contract is now integrated; the regression fixture must continue to pass
  the launch context through protocol-aware clone URL construction.

## Parallelism

`sequential`

## Inputs

- `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` and its acceptance criteria 001.11 and 001.12.
- The repository-preparation and task-resolution sections in the three GitHub authentication
  system-design documents.
- `LaunchPreparedSession`, `startAgentOnExistingWorkspace`, and
  `resolveAllRepoInfoForSession`.
- Existing origin tests in `executor_resume_clone_transport_test.go` and `repoclone/clone_test.go`.

## Results

The RED regression failed before the production change because the prepared-workspace fast path
started the agent with the stale HTTPS origin. The GREEN implementation hoists
`resolveAllRepoInfoForSession` before that branch and reuses the resolved slice for full-launch
fallbacks.

The regression uses two real Git checkouts and the real `repoclone.Cloner.SetOriginURL` behavior.
It proves both origins are canonical before `StartAgentProcess`, the canonical checkout remains
write-locked without failure, and each attached repository receives one preparation call.

Verification passed:

```text
go test -tags fts5 ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart$' -count=1
go test -tags fts5 ./internal/orchestrator/executor ./internal/repoclone
go test -tags fts5 ./internal/orchestrator/executor -run 'TestEnsureRepoLocalPathReevaluatesGitHubProtocol$' -count=1
python3 scripts/lint-spec-files.py --all
```
