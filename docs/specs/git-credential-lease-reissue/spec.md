---
status: building
created: 2026-08-20
amended: 2026-08-23
owner: kandev
---

# Git Credential Lease Reissue

## Why

A long-running execution can outlive the in-memory Git credential lease held by
the backend. A workspace credential rotation or backend restart then leaves Git
and the `gh` shim permanently unable to redeem credentials even though the task,
session, repository, and current workspace connection remain valid.

The default local installation previously disabled reissue whenever the operator
did not configure a stable signing key. Re-selecting or replacing an otherwise
valid workspace GitHub connection therefore stranded already-running managed
sessions even when the backend process itself had not restarted.

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
- When no stable reissue signing key is configured, the backend SHALL create a
  cryptographically random process-local signer before it launches managed
  executions. Sessions launched by that backend process can recover after a
  workspace credential rotation without a resume or replacement session.
- A configured stable signing key remains the only mechanism that preserves
  reissue capability verification across a backend restart. The automatic
  process-local signer MUST NOT be persisted, logged, or exposed.
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

The capability signer uses either a stable configured backend signing key or a
cryptographically random process-local key generated at backend startup. Both
keys remain backend-only. The process-local key is intentionally not persisted;
capabilities it issued become unverifiable after that backend exits.

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
- Failure to obtain operating-system cryptographic randomness terminates
  backend startup; Kandev does not silently launch managed sessions without
  their documented reissue capability.

## Persistence guarantees

Leases remain intentionally in-memory and disappear on backend restart.
Reissue capabilities survive only in the still-running execution environment
and remain verifiable across a backend restart only when the configured signer
is stable. A capability issued by the automatic process-local signer remains
valid for credential rotations during that backend process, but not after a
restart. Capabilities expire and are rendered useless by live authorization
once their task/session/repository is no longer eligible.

## Scenarios

- **GIVEN** a running execution with a valid reissue capability, **WHEN** the
  workspace credential generation rotates and the old lease is redeemed,
  **THEN** the helper obtains one lease bound to the new generation and the
  same Git request succeeds.
- **GIVEN** no stable reissue signing key is configured and a managed execution
  was launched by the current backend process, **WHEN** an administrator
  replaces or re-selects the workspace GitHub connection, **THEN** the next Git
  or `gh` credential request reissues once and succeeds without resuming or
  replacing the session.
- **GIVEN** a running execution with a capability signed by the configured
  stable key, **WHEN** the backend restarts and has no prior lease records,
  **THEN** the helper obtains one newly issued lease and the same Git request
  succeeds.
- **GIVEN** a running execution whose capability was signed by the automatic
  process-local signer, **WHEN** the backend restarts, **THEN** the old
  capability fails closed and the execution needs a resume or fresh launch to
  receive a capability from the new process.
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
- Retrofitting a capability into a session launched before this behavior was
  available.
- Expanding or replacing a session's launch-time repository scope or changing
  its task Git credential policy without a resume.
- Persisting an automatically generated signing key.
- Retrying arbitrary broker, network, provider, or authorization failures.
