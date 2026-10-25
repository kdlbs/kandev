---
status: active
system: platform
created: 2026-08-20
owners:
  - kandev
---
# Git Credential Lease Reissue Requirements

## Overview

A long-running execution can outlive the in-memory Git credential lease held by the backend. A workspace credential rotation or backend restart then leaves Git and the `gh` shim permanently unable to redeem credentials even though the task, session, repository, and current workspace connection remain valid.

## Requirements

### REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001: Git Credential Lease Reissue

**Intent:** A long-running execution can outlive the in-memory Git credential lease held by the backend. A workspace credential rotation or backend restart then leaves Git and the `gh` shim permanently unable to redeem credentials even though the task, session, repository, and current workspace connection remain valid.

#### Acceptance criteria

- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.1:** A managed Git credential helper SHALL make at most one reissue attempt for a single credential request after the broker reports a reissuable lease failure.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.2:** A reissue uses a dedicated execution capability, never the failed lease as proof of authority.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.3:** The broker SHALL issue the replacement lease only after it verifies the exact task, session, repository, provider, host, path, and optional provider-parent identity carried by the capability against live state.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.4:** A replacement lease SHALL bind to the current provider credential generation. A live execution therefore recovers after an authorized workspace credential rotation or after a backend restart discarded the old in-memory lease map.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.5:** The helper SHALL retry only lease-invalid, lease-expired, or lease-revoked responses. Lease-revoked is included because a credential-generation change deliberately revokes the old lease. Scope denial, malformed requests, capability failures, provider failures, and transport failures remain fail-closed with no retry.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.6:** Capabilities and leases are opaque bearer values. They SHALL never be logged, returned by browser-facing APIs, or stored with plaintext Git credentials.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.7:** **GIVEN** a running execution with a valid reissue capability, **WHEN** the workspace credential generation rotates and the old lease is redeemed, **THEN** the helper obtains one lease bound to the new generation and the same Git request succeeds.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.8:** **GIVEN** a running execution with a valid reissue capability, **WHEN** the backend restarts and has no prior lease records, **THEN** the helper obtains one newly issued lease and the same Git request succeeds.

## Migrated source detail

## Why

A long-running execution can outlive the in-memory Git credential lease held by
the backend. A workspace credential rotation or backend restart then leaves Git
and the `gh` shim permanently unable to redeem credentials even though the task,
session, repository, and current workspace connection remain valid.

## What

- A managed Git credential helper SHALL make at most one reissue attempt for a
  single credential request after the broker reports a reissuable lease failure.
- A reissue uses a dedicated execution capability, never the failed lease as
  proof of authority.
- The broker SHALL issue the replacement lease only after it verifies the exact
  task, session, repository, provider, host, path, and optional provider-parent
  identity carried by the capability against live state.
- A replacement lease SHALL bind to the current provider credential generation.
  A live execution therefore recovers after an authorized workspace credential
  rotation or after a backend restart discarded the old in-memory lease map.
- The helper SHALL retry only lease-invalid, lease-expired, or lease-revoked
  responses. Lease-revoked is included because a credential-generation change
  deliberately revokes the old lease. Scope denial, malformed requests,
  capability failures, provider failures, and transport failures remain
  fail-closed with no retry.
- Capabilities and leases are opaque bearer values. They SHALL never be logged,
  returned by browser-facing APIs, or stored with plaintext Git credentials.

## Data model

`GitCredentialReissueCapability` is an encrypted, authenticated opaque execution capability. Its
claims are the exact non-secret `gitcredentials.Scope` identity, an issue time,
and an expiry. The capability is injected only into the managed runtime helper
environment with the matching lease scope. No capability, lease, or credential
plaintext is written to the task database.

The capability signer uses a stable configured backend signing key. When no
stable signer is configured, reissue capability issuance is unavailable and the
existing managed-lease path remains fail-closed; a process restart does not gain
an unauthenticated recovery path.

## API surface

- `POST /api/v1/github/credentials/resolve` continues to redeem an opaque
  lease for a credential. Its error code distinguishes lease-invalid,
  lease-expired, and lease-revoked responses so a helper can decide whether a
  reissue is eligible.
- `POST /api/v1/github/credentials/reissue` accepts a reissue capability and
  the helper's exact non-secret repository request. It returns a new opaque
  lease and expiry, never a Git credential. The route is unauthenticated at
  HTTP middleware level because the capability self-authenticates in the
  handler.
- `KANDEV_GITHUB_CREDENTIAL_REISSUE_CAPABILITY` carries the default helper
  capability. Multi-repository `KANDEV_GITHUB_CREDENTIAL_SCOPES` entries carry
  the matching capability beside each lease.

## State machine

1. Launch issues a scoped lease and a scoped reissue capability.
2. Helper redeems the lease.
3. On an eligible lease failure, helper validates its local Git input against
   the issued scope, exchanges the capability for one replacement lease, and
   retries redemption once.
4. The replacement redemption succeeds or fails closed. The helper does not
   loop or fall back to another repository scope.

## Permissions

The capability grants no broader permission than the launch-time scope. On
every reissue the broker re-runs live task/session/repository authorization and
the provider binding check. A terminal session, removed task repository,
disabled connection, changed repository identity, forged capability, expired
capability, or mismatched helper request is denied.

## Failure modes

- A malformed, forged, expired, or scope-mismatched capability returns an
  authorization failure and issues no lease.
- A terminal session or changed task/repository binding returns an authorization
  failure and issues no lease.
- A disabled, disconnected, or otherwise unavailable provider returns its
  existing failure; the helper does not retry another credential source.
- A failed reissue or replacement redemption is returned to Git/`gh` after the
  single allowed retry.

## Persistence guarantees

Leases remain intentionally in-memory and disappear on backend restart.
Reissue capabilities survive only in the still-running execution environment
and remain verifiable across a backend restart when the configured signer is
stable. They expire and are rendered useless by live authorization once their
task/session/repository is no longer eligible.

## Scenarios

- **GIVEN** a running execution with a valid reissue capability, **WHEN** the
  workspace credential generation rotates and the old lease is redeemed,
  **THEN** the helper obtains one lease bound to the new generation and the
  same Git request succeeds.
- **GIVEN** a running execution with a valid reissue capability, **WHEN** the
  backend restarts and has no prior lease records, **THEN** the helper obtains
  one newly issued lease and the same Git request succeeds.
- **GIVEN** a forged or expired capability, **WHEN** the helper requests a
  lease, **THEN** the broker issues no lease and returns an authorization
  failure.
- **GIVEN** a capability for repository A, **WHEN** a helper requests
  repository B or a terminal session's scope, **THEN** the broker issues no
  lease.
- **GIVEN** a lease scope error or a failed replacement redemption, **WHEN**
  the helper handles the response, **THEN** it performs no further reissue
  attempt or fallback.

## Out of scope

- Persisting lease records or Git credentials.
- Reissuing executor-profile, GitLab, Azure DevOps, or user-supplied tokens.
- Repairing an execution whose agent process or helper environment has stopped.
- Retrying arbitrary broker, network, provider, or authorization failures.
