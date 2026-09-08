---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-08-18
status: complete
---

# Fix Plan: Make Managed Git Credential Admission Atomic

## Overview

PR #2787 adds a shared repository identity resolver and checks managed Git credential identity
before selected session changes. The direction fits the provider-neutral broker architecture, but
four gaps can still authorize different identities or mutate task state before a later rejection.

The repair keeps one provider-neutral resolver. It makes persisted repository metadata the only
authorization source, validates secret-bearing SSH syntax before normalization, and runs one
read-only admission gate before all normal, Office, and workflow session mutations. It also removes
the startup migration that infers persistent provider identity from an incomplete legacy tuple.

## Confirmed Root Causes

### SSH syntax is normalized before it is validated

`gitcredentials.parseRemote` first calls `repoclone.CanonicalHTTPSCloneURL`. That conversion drops
an `ssh://` password, query, or fragment before the resolver checks the canonical result. SCP-style
input can also retain a query or fragment in the path. The resolver therefore accepts remote text
that violates its credential-free input contract.

### Issuance and redemption use different identity sources

Executor preflight calls `gitCredentialCloneIdentity`, which can inspect `repository.LocalPath` and
read the checkout's Git origin. Broker authorization uses persisted `RemoteURL`, `ProviderHost`,
`Owner`, and `Name`. A repository with only a usable local origin can pass preflight and then fail
when the broker issues a credential.

### Admission is incomplete and occurs after some state changes

`PreflightManagedGitCredentials` ignores a task-policy resolution error and cannot determine that
an explicit executor-profile token overrides the managed broker. Office session creation and reuse
bypass the normal `PrepareSession` gate. A workflow transition persists the destination step before
the later profile-switch preflight runs. These paths can reject the launch after a session or the
workflow state has changed.

### The migration infers a semantic identity from partial metadata

`repository_credential_origin_migration.go` writes `provider_host` when it sees only a provider and
remote URL. It does not have the full verified provider tuple, and nullable legacy fields can also
abort its scan into Go strings. The accepted repository-origin decision requires existing rows to
remain hostless when the database cannot infer their origin safely. Runtime resolution already
recognizes public `github.com` remotes without this write.

## Repair Architecture

### Shared repository identity

- Validate the original HTTPS, `ssh://`, and SCP-style syntax before any canonical conversion.
- Reject URL userinfo passwords, queries, and fragments with bounded errors that do not echo input.
- Remove the unused `RepositoryIdentity.CanonicalURL` field.
- Make executor preflight pass persisted repository identity fields only. Do not inspect a local
  checkout during credential authorization.
- Keep `internal/gitcredentials` provider-neutral. GitHub-specific policy stays in its callers.

### Read-only credential admission

- Make policy-resolution errors fail closed.
- Resolve the selected executor profile before managed identity validation.
- Treat a configured executor-profile `GITHUB_TOKEN` or `GH_TOKEN`, including a secret reference,
  as the effective unmanaged source. Admission must not reveal the secret value.
- Put policy, profile-source, and repository checks behind one read-only executor admission method.
- Call that method before normal session preparation changes state and before Office code creates,
  rebinds, or resumes a session.

### Workflow transition boundary

- Resolve the destination agent and effective executor profile without changing state.
- Run credential admission before `processOnExit` and before the workflow engine applies the
  transition.
- Keep post-transition session switching as the execution step. A failed admission must leave the
  source step, current session, and prompt routing unchanged.

### Persistence

- Remove the new repository credential-origin migration and its registration from base migrations.
- Add a regression test that runs startup migrations for incomplete legacy repository metadata and
  verifies that `provider_host` remains empty.
- Do not add a replacement backfill. Repository discovery or import remains responsible for
  persisting verified provider-origin identity.

## Tests

- Reject `ssh://` passwords, queries, fragments, and SCP-style query or fragment input without
  leaking the supplied remote in the error.
- Reject a provider-backed repository that has only a local checkout origin in both executor
  preflight and broker authorization.
- Return task-policy resolver errors before session mutation.
- Allow an invalid managed identity when an explicit executor-profile GitHub token is the selected
  source.
- Prove that fresh and reused Office sessions remain unchanged after failed admission.
- Prove that a failed destination-profile admission leaves the workflow on its source step and
  does not route the destination prompt.
- Prove that startup migration does not infer `provider_host` from partial legacy metadata.

## Documentation

- Update the durable GitHub authentication specification with the atomic admission, persisted
  identity, SSH validation, and no-backfill rules.
- No public documentation or frontend copy changes are required. The repair makes failure behavior
  match the existing task Git access contract.

## Implementation Tasks

- [x] [task-01-harden-repository-identity](task-01-harden-repository-identity.md) - done
- [x] [task-02-centralize-session-admission](task-02-centralize-session-admission.md) - done
- [x] [task-03-remove-unsafe-origin-backfill](task-03-remove-unsafe-origin-backfill.md) - done

Execution is sequential in the primary conversation. Task 02 depends on the corrected identity
contract from Task 01. Task 03 then restores the persistence boundary and locks it with a startup
regression test.

## Risks And Out Of Scope

- Existing database rows that already have a verified `provider_host` remain unchanged.
- This repair does not discover provider identity from arbitrary local Git configuration.
- This repair does not change the workspace policy model, broker lease scope, or plugin API.
- Profile-token admission checks only whether the selected profile configures the override. It must
  not resolve or log the token during the read-only gate.
- Workflow admission must use the same profile that the later switch will use. A duplicated profile
  selection rule would recreate the time-of-check/time-of-use gap.

## Recorded Results

- The resolver now rejects secret-bearing SSH forms before canonicalization and returns only safe,
  repository-scoped errors.
- Executor issuance and broker redemption use the same persisted repository fields.
- Policy, remote profile token, normal session, Office session, and workflow transition admission
  now complete before the related state mutation.
- The repository credential-origin backfill was removed. Startup migration keeps partial legacy
  provider identity unchanged.
- Targeted verification passed 25 resolver tests, 11 identity/admission tests, 41 executor
  lifecycle tests, 13 race-enabled workflow transition tests, and 598 SQLite repository tests.
