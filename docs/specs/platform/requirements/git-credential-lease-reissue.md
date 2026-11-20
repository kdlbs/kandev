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
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.9:** When an idle managed lease expires and the broker removes it, a still-authorized execution with a valid reissue capability shall recover the Git or `gh` request with one replacement lease.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.10:** If the execution lacks a reissue capability and the broker returns HTTP 401 with a lease-invalid or lease-expired code, Git and `gh` shall preserve the broker error and advise using **Restart session** if access is still authorized. This applies to Git and GitHub code prefixes. A lease-revoked or other failure shall retain its existing handling. An eligible session that starts a fresh process after **Restart session** shall receive a new lease and, when reissue is enabled, a capability, including after adoption of a retained lease-only execution.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.11:** Backend startup shall report the selected signer source or the disabled state once. The diagnostic shall omit sensitive values.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.12:** Reissue shall be enabled by default for eligible managed launches with the existing workspace Git configuration. An operator signing key shall be optional. A valid default capability shall allow recovery after idle expiry and backend restart on the same installation with the same secrets master key.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.13:** When an operator signing key remains configured, its previously issued capabilities shall remain usable under the same eligibility rules. A default capability from an installation with a different secrets master key shall be rejected.
- **AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.14:** Operators shall be able to disable reissue at backend startup regardless of the configured signing key. With reissue disabled, eligible managed launches shall still receive redeemable leases. They shall receive no capabilities, and the broker shall reject exchanges of existing capabilities through both Git and GitHub routes.

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

The capability signer defaults to the installation's stable signing material.
An explicit backend signing key overrides the installation default. Disabling
reissue suspends issuance and exchange of capabilities. Reissue requires a
valid capability regardless of the source of signing material.

## API surface

- `POST /api/v1/github/credentials/resolve` continues to redeem an opaque
  lease for a credential. Its error code distinguishes lease-invalid,
  lease-expired, and lease-revoked responses so a helper can decide whether a
  reissue is eligible.
- `POST /api/v1/github/credentials/reissue` accepts a reissue capability in the
  JSON field `reissue_capability`, the same name used by
  `KANDEV_GITHUB_CREDENTIAL_SCOPES` entries, and the helper's exact non-secret
  repository request. It returns a new opaque lease and expiry, never a Git
  credential. The route is unauthenticated at HTTP middleware level because the
  capability self-authenticates in the handler.
- `KANDEV_GITHUB_CREDENTIAL_REISSUE_CAPABILITY` carries the default helper
  capability. Multi-repository `KANDEV_GITHUB_CREDENTIAL_SCOPES` entries carry
  the matching capability beside each lease.

## State machine

1. Launch issues a scoped lease. When reissue is enabled, it also issues a
   scoped reissue capability.
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
Reissue capabilities survive only in the still-running execution environment.
They remain verifiable across backend restart with the same signer. The broker
checks expiry and live task, session, and repository eligibility during reissue.

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
