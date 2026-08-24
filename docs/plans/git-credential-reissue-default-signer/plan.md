---
spec: docs/specs/git-credential-lease-reissue/spec.md
created: 2026-08-23
status: done
---

# Implementation Plan: Default Git Credential Reissue Signer

## Overview

The managed Git credential broker currently configures a reissue signer only
when `githubCredentialBroker.reissueSigningKey` is explicitly set. An empty
default therefore launches sessions without reissue capabilities, so replacing
a valid workspace GitHub connection invalidates their leases until resume. The
fix makes a cryptographically random process-local signer the default while
preserving the configured stable key as the cross-restart path.

## Confirmed root cause

`apps/backend/internal/backendapp/git_credentials.go:newGitCredentialBroker`
passes the configured key to `gitcredentials.NewReissueCapabilitySigner` and
silently leaves the broker without a signer when the key is empty. The executor
then falls back to a lease without `reissue_capability`; `agentctl` correctly
performs no reissue when the workspace connection generation invalidates that
lease. Existing tests cover reissue with an explicitly configured signer but do
not cover the default empty-key wiring.

## Backend

### Ephemeral signer construction

- Update `apps/backend/internal/backendapp/git_credentials.go` so
  `newGitCredentialBroker` selects the configured stable signer when non-empty
  and otherwise creates a process-local key with `crypto/rand.Text()` before
  constructing the signer. Go's secure-random contract terminates the process
  on entropy failure, so the broker cannot silently fall back to lease-only
  behavior.

### Security boundaries

- The generated key remains memory-only and never enters config source
  metadata, logs, browser payloads, executor payloads, or the secret store.
- Existing repository-exact capability claims, live authorization, one-reissue
  limit, and configured stable-key behavior remain unchanged.
- No running process receives a broader or newly attached repository scope.

## Frontend

No frontend behavior or copy changes are required. The existing settings UI
continues to treat task credential policy changes as launch/resume scoped; this
repair only restores an already-issued managed repository scope after workspace
credential generation changes.

## Tests

- **What:** constructing two production brokers with empty configured keys
  yields process-local signers: each issues a non-empty reissue capability and
  one broker rejects the other's capability. A configured key retains stable
  signer behavior across broker construction.
  **File:** `apps/backend/internal/backendapp/git_credentials_test.go`.
  **How:** package-level wiring tests using a normalized non-secret scope; no
  provider credential resolution is required.
- **What:** the helper continues to reissue exactly once after a lease-revoked
  response in multi-scope mode.
  **File:** `apps/backend/cmd/agentctl/github_credential_test.go`.
  **How:** retain and run the existing HTTP-level regression.

## E2E Tests

No browser E2E is required. The changed behavior is exercised at the broker
construction and helper HTTP boundaries, and it has no new UI action.

## Public documentation

Update `docs/public/configuration.md`, `docs/public/executors.md`,
`docs/public/integrations.md`, `docs/docker.md`, and `docs/k8s.md` to
distinguish automatic same-process credential-rotation recovery from configured
cross-restart recovery. The
configuration reference continues to show an empty operator default; empty now
means an automatic process-local signer, not disabled reissue.

## Verification Results

- Task 01 RED reproduced `git credential lease reissue is unavailable` with the default empty key.
- Task 01 GREEN passed 2 production broker wiring tests and the existing `agentctl` multi-scope single-reissue test.
- Task 02 passed 61/61 public-doc tests and validated 41 published pages.
- No credential, lease, capability, or signing-key value was read, logged, or persisted.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-default-reissue-signer](task-01-default-reissue-signer.md)

Wave 2:

- [x] [task-02-document-reissue-default](task-02-document-reissue-default.md)

The tasks are sequential because the public documentation must describe the
behavior proven by Task 01. No subagent execution is authorized by these waves.

## Risks

- Sessions launched before the fixed backend cannot be retrofitted with a
  missing capability and still require resume or a fresh launch.
- An automatically generated signer intentionally cannot recover across a
  backend restart; operators needing that property must configure the existing
  stable key.
- The implementation must use `crypto/rand`, never time, UUID, connection
  metadata, or a shared fixed fallback, for the process-local key.
