---
id: "00-reconcile-managed-checkout-origin"
title: "Reconcile managed checkout origin"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 00: Reconcile Managed Checkout Origin

## Acceptance

- Existing and newly materialized Kandev-managed GitHub checkouts use canonical HTTPS in managed
  mode and the host-detected clone protocol in executor-inherited mode before Local/Worktree
  preparation continues.
- A policy-resolution or origin-update failure stops preparation; no stale-transport fallback is
  used.
- Repositories registered as user-managed local checkouts retain their configured origin.
- Public documentation explains the Local/Worktree transport behavior and conditional-include
  effect.

## TDD Sequence

1. Add filesystem-backed regression cases for executor HTTPS-to-SSH reconciliation, managed
   SSH-to-HTTPS reconciliation, and the local-source no-op.
2. Run the focused executor test and record the expected RED assertion that the origin remains on
   the old transport.
3. Add a focused repoclone origin-update test and minimal production operation.
4. Wire policy-aware reconciliation into repository preparation.
5. Run the exact targeted backend and public-doc commands below and record GREEN results.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor ./internal/repoclone -run 'Test.*(RepoLocalPath.*(GitHubOrigin|OriginUpdateFailure|DoesNotRewriteUserManagedOrigin)|SetOriginURL)' -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_default_branch_test.go`
- `docs/public/integrations.md`
- `docs/specs/integrations/requirements/github-authentication.md`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. The contract, implementation, tests, and documentation describe one behavior and
should land together.

## Inputs

- The confirmed root cause and backend boundary in `plan.md`.
- The task-policy transport scenarios in the workspace GitHub authentication spec.
- `repoclone.Cloner.BuildCloneURLWithHost` and `DetectGitProtocol`.
- `executor.ensureRepoLocalPath`, `ensureRepoCloned`, and
  `configureGitHubCredentialBrokerForRepositories`.

## Output Contract

Report the RED failure, final origin-reconciliation behavior, files changed, exact GREEN command
results, public-doc validation, remaining risks, and task/plan status updates in the same primary
conversation.

## Recorded Results

- RED: `TestEnsureRepoLocalPath_ReconcilesGitHubOriginForCredentialPolicy` failed because both
  policy transitions left the original remote URL unchanged.
- GREEN: the targeted Go command passed six tests across `executor` and `repoclone`.
- Public docs: `node --test scripts/validate-public-docs.test.mjs` passed 58 tests and
  `node scripts/validate-public-docs.mjs` validated 41 published pages.
- Review remediation: concurrent origin updates now share the existing per-repository mutex;
  `TestSetOriginURLSerializesConcurrentUpdates` fails before the lock and passes after it.
