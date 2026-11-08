---
id: "02-shared-profile-secret-grants"
title: "Owner-granted shared agent profile secrets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SHARED-PROFILE-SECRET-001
  - REQ-AGENTS-SHARED-PROFILE-SECRET-002
acceptance_criteria:
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.1
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.2
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.3
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.4
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.5
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.6
  - AC-AGENTS-SHARED-PROFILE-SECRET-001.7
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.1
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.2
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.3
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.4
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.5
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.6
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.7
  - AC-AGENTS-SHARED-PROFILE-SECRET-002.8
system_design:
  - ../../specs/agents/system-design/shared-profile-secrets.md
---

# Task 02: Owner-granted shared agent profile secrets

## Summary

Let a global secret's owner share one agent-profile binding with every
authorized starter. Fail closed with actionable errors for every other use.

## In scope

- `ProfileEnvVar.SharedByUserID` and the `shared` profile DTO field.
- Owner-only grant writes, revocation, re-verification, and binding-exact
  application.
- Grant stripping on duplicate and rejection on nested create and executor
  profiles.
- Owner-bound global secret reads in `internal/secrets`.
- Lifecycle grant resolution on the strict and non-strict paths, the grantor
  resolver wiring, and actionable error hints.

## Out of scope

- Web UI.
- Workspace-secret and cross-organization grants.
- Other secret consumers.
- Inferred admin or global ownership.

## Files

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_crud.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/settings/controller/profile_secret_grants.go`
- `apps/backend/internal/agent/settings/controller/profile_secret_grants_test.go`
- `apps/backend/internal/agent/settings/controller/profile_secret_scope_test.go`
- `apps/backend/internal/secrets/owner_grant.go`
- `apps/backend/internal/secrets/owner_grant_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/shared_profile_secret.go`
- `apps/backend/internal/agent/runtime/lifecycle/shared_profile_secret_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_profile_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/backendapp/auth.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/executor_profile_secret_share_test.go`

## Verification

```bash
cd apps/backend
go test -trimpath -race -count=1 -timeout 600s \
  -run 'TestSharedProfileSecret|TestExecutorProfileBindingsNeverHonorAgentProfileShares|TestStrictEnvironment(HonorsShared|DeniesSharedSecret|NamesProfileAndReason)|TestExecutorProfilesRejectSharedSecretGrants|TestOwnerSharesSecret|TestProfileSecret|TestOwnerRevokes|TestDuplicateProfileDropsSecretShare|TestOwnerGrantedSecretStore' \
  ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller \
  ./internal/secrets ./internal/task/service
```

## Results

- **Base reproduction.** On unmodified base `52309e61`:
  - The owner's binding resolved.
  - A same-organization collaborator failed before launch with
    `env key "SHARED_TOKEN" unavailable`, which gave no reason.
  - Base has no way to grant shared use.
- **Authorized starters.** With the fix, the following resolve the granted
  binding: the owner, a same-organization collaborator, and an agent using the
  collaborator's personal access token.
- **Lifecycle denials.** Eleven cases fail closed with a fixed reason, and none
  leaks a synthetic ID or value:
  - no grant;
  - inactive grantor;
  - mismatched grantor identity;
  - synthetic grantor;
  - cross-organization starter;
  - resolver not wired;
  - forged grantor who does not own the secret;
  - workspace-scoped secret;
  - missing secret;
  - unauthenticated starter;
  - the owner trying to borrow a collaborator's private secret.
- **Lifecycle behavior.**
  - A partial resolution drops only the denied entry.
  - Cancellation propagates.
  - Auth-disabled resolution stays unscoped.
  - Executor-profile bindings never use agent-profile grants. The test goes
    through `ExecutorProfileEnvForSession` with a granted executor-profile row
    and an active same-org grantor. The owner resolves the value; the
    collaborator gets `ErrProfileSecretUnavailable` and no environment.
    Removing `withoutSecretShares` from that path in a disposable copy made
    the test fail.
  - Strict resolution honors only the exact binding and names the profile and
    reason on denial.
- **Controller.** The following are covered:
  - the grantor is not exposed in responses;
  - non-owner, synthetic, and identity-free share attempts are rejected;
  - literal and nested-create shares are rejected;
  - a grant survives collaborator edits that keep the binding;
  - a grant does not follow a changed binding;
  - the owner can revoke, and a collaborator cannot re-share;
  - a grant fails closed after the owner deletes the secret;
  - duplicates drop grants;
  - forged grant fields are cleared.
- **Store.** The secrets store matches only owned global secrets.
- **Executor profiles.** The task service rejects executor-profile grants and
  leaves the stored row unchanged.
- **Focused gates.** The focused `-race` run passes. Build, vet, lint, and
  gofmt gates are recorded in the [plan](plan.md#verification-results).
