---
spec: docs/specs/platform/requirements/git-credential-lease-reissue.md
created: 2026-08-20
status: completed
---

# Implementation Plan: Git Credential Lease Reissue

## Overview

Add an encrypted, execution-scoped reissue capability alongside each managed Git
credential lease. The generic broker validates that capability and issues a
fresh lease from live authorization and provider binding; the existing helper
then performs one replacement redemption. A capability reissue replaces the
previous lease for that capability, so short-lived helper processes cannot
consume the workspace lease limit. This retains in-memory leases while making
a running execution recover after a credential-generation change or backend
restart.

## Backend

### Capability and broker contracts

- Add sealing, validation, expiry, and exact-scope comparison to
  `apps/backend/internal/gitcredentials/` without retaining credentials.
- Extend `Broker` with capability issue/reissue operations that call its
  existing authorization path, preserving live authorizer and resolver checks.
- Let providers refresh rotating binding generations during reissue without
  changing the sealed task, session, or repository identity.
- Wire a stable signer from backend configuration in
  `apps/backend/internal/backendapp/` and leave capability issuance disabled
  when no stable key exists.

### Executor environment injection

- Extend `apps/backend/internal/orchestrator/executor/executor_credentials.go`
  scope records and managed-environment cleanup to carry a capability matching
  each issued scope.
- Extend `apps/backend/internal/githubauth/environment.go` and
  `apps/backend/internal/agent/runtime/lifecycle/remote_github_env.go` so
  Docker, SSH, and local managed helper environments receive only the required
  opaque capability values.

### HTTP and helper recovery

- Add provider-neutral `/api/v1/git/credentials/*` broker routes and precise
  reissuable error codes. Keep the GitHub paths as compatibility aliases and
  preserve public-route self-authentication in
  `apps/backend/internal/auth/httpmw/middleware.go`.
- Update `apps/backend/cmd/agentctl/github_credential.go` to recognize only
  eligible lease errors, request one replacement lease with the matching
  capability, and retry the original redemption once. The same command powers
  the Git helper and `gh` shim.

## Tests

- **Capability contract:** `internal/gitcredentials/*_test.go` covers valid
  reissue, exact-scope mismatch, forged token, and expiry.
- **Broker HTTP:** `internal/github/*_test.go` verifies self-authenticated
  reissue, backend-restart-equivalent new broker recovery, and no lease for
  forged/expired capabilities.
- **Executor wiring:** `internal/orchestrator/executor/*_test.go` asserts every
  single and multi-repository helper scope receives only its matching opaque
  capability.
- **Helper integration:** `cmd/agentctl/github_credential_test.go` asserts
  credential-generation revoked and missing-lease responses reissue once,
  retry successfully in single- and multi-scope mode, and reject
  non-reissuable/fake/expired paths. The `gh` shim uses the same one-shot flow.
- **Lease lifecycle:** repeated reissue keeps one active lease for a
  capability. Contribution destination reissue refreshes the current binding
  generation after an authorized connection rotation.

## Verification Results

- `go build ./...` and affected-package `go vet` passed locally.
- Focused regressions cover credential-generation rotation, an empty lease map
  after restart, forged/expired/scope-mismatched capabilities, helper retry
  limits, HTTP middleware admission, executor injection, and remote executor
  environment forwarding.
- `gofmt -l`, `git diff --check`, and public-doc validation passed locally.
- `golangci-lint --new-from-rev` was not run because `golangci-lint` is not
  installed in this worktree environment.

## Implementation Waves

1. [task-01-capability-and-broker](task-01-capability-and-broker.md)
2. [task-02-executor-and-helper](task-02-executor-and-helper.md)
3. [task-03-regression-verification](task-03-regression-verification.md)

## Risks

- The reissue route is publicly reachable, so its only authority is the
  capability plus repeated live scope authorization. It must never accept an
  old lease as reissue proof.
- A non-stable signer cannot safely support cross-restart recovery. Its
  disabled path must remain explicit and fail closed.
