---
id: "01-harden-repository-identity"
title: "Harden repository credential identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Harden Repository Credential Identity

## Acceptance

- The shared resolver rejects HTTPS or SSH credentials, queries, and fragments before
  canonicalization can discard them.
- Rejection errors do not include the supplied remote or a contained secret.
- Executor preflight and broker authorization derive identity from the same persisted repository
  fields and never use a local checkout origin as authorization data.
- The identity type contains only fields consumed by authorization.
- The package remains provider-neutral and contains no plugin-specific contract.

## TDD Sequence

1. Add table-driven resolver cases for an `ssh://` password, query, and fragment and for SCP-style
   query and fragment input. Assert that each error omits the input and marker secret.
2. Run the focused identity test and record RED cases that the current canonicalization accepts.
3. Add an executor/broker parity case for a GitHub provider row with a valid local origin but no
   persisted remote or owner/name identity. Record the current preflight/broker disagreement.
4. Validate original remote syntax before conversion, remove the unused canonical URL result, and
   make executor preflight use the persisted remote.
5. Run the focused packages and record GREEN results.

## Verification

```bash
cd apps/backend && go test ./internal/gitcredentials -run 'TestResolveRepositoryIdentity' -count=1
cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp -run 'Test.*GitCredential.*(Identity|Preflight|Authorization)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/gitcredentials/identity.go`
- `apps/backend/internal/gitcredentials/identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_preflight_test.go`
- `apps/backend/internal/backendapp/git_credentials.go`
- focused broker authorization tests under `apps/backend/internal/backendapp/`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the corrected persisted-identity contract.

## Inputs

- The managed identity rules in the GitHub authentication specification.
- `gitcredentials.ResolveRepositoryIdentity`.
- `executor.gitCredentialCloneIdentity`.
- The broker authorizer configured in `backendapp.ConfigureGitCredentialBroker`.

## Output Contract

Report the exact RED failures, the accepted remote forms, the common persisted identity source,
the GREEN commands, files changed, and remaining risks. Update this task and the parent plan with
the recorded results.

## Recorded Results

- RED: five SSH password/query/fragment cases passed unsafe input through canonicalization.
- RED: executor preflight accepted a valid local checkout origin when persisted authorization
  identity was incomplete.
- GREEN: `go test ./internal/gitcredentials -run TestResolveRepositoryIdentity -count=1` passed 25
  tests.
- GREEN: the focused executor/backend application identity and admission command passed 11 tests.
- `RepositoryIdentity.CanonicalURL` was removed because no authorization caller used it.
