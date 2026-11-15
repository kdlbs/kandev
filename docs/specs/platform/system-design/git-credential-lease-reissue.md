---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
created: 2026-10-30
owners:
  - kandev
---
# Git Credential Lease Reissue System Design

## Purpose and boundaries

Platform owns the managed Git credential lease and reissue contract for every
executor type. The contract spans the backend broker, the shared helper request
wire format, and the `agentctl` helper that Git and the `gh` shim invoke.

The design uses the provider-neutral broker in `internal/gitcredentials` and
GitHub's adapter in `internal/github`. GitHub-specific routes remain
compatibility aliases of the provider-neutral `/api/v1/git/credentials/*`
routes. Workspace connection authority and credential resolution stay with the
existing GitHub authentication design; this design owns only the lease recovery
path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

- `internal/githubauth` is the neutral shared contract package for both tiers.
  It owns the `KANDEV_GITHUB_CREDENTIAL_*` environment names, scope path
  canonicalization, and `CredentialBrokerRequest`, the request shape the helper
  sends and the broker routes bind. It is not owned by the backend tier or by
  `cmd/agentctl`.
- `cmd/agentctl` runs the managed Git credential helper. It builds a request
  from the process environment plus Git's input, resolves one credential, and
  on an eligible lease failure performs one reissue and one replacement
  redemption.
- `internal/github.Controller` exposes `/api/v1/github/credentials/resolve`,
  `/api/v1/github/credentials/reissue`, and their provider-neutral
  `/api/v1/git/credentials/*` aliases. The reissue handler decodes
  `githubauth.CredentialBrokerRequest` and passes its `ReissueCapability` to the
  service.
- `internal/github.Service.ReissueGitHubCredentialLease` delegates to the
  `CredentialBroker` adapter.
- `internal/gitcredentials.Broker` owns lease storage, capability validation,
  live scope authorization, provider binding checks, and replacement issuance.
  `ReissueCapabilitySigner` seals and opens the opaque capability.
- `internal/orchestrator/executor` composes the launch environment. It writes
  one scope per repository, including the matching capability in
  `KANDEV_GITHUB_CREDENTIAL_SCOPES` through `githubCredentialScope`.
- `internal/auth/httpmw` allowlists the resolve and reissue routes because the
  capability authenticates reissue inside the handler.

## Data and contracts

`githubauth.CredentialBrokerRequest` is the single wire contract:

| Field | Meaning |
| --- | --- |
| `lease` | Opaque lease redeemed by `/resolve`; absent on reissue |
| `reissue_capability` | Opaque execution capability for `/reissue` |
| `task_id`, `session_id`, `repository_id` | Exact execution scope |
| `owner`, `repo`, `host`, `path` | Exact repository request |
| `provider_id`, `parent_provider_id` | Optional provider identity |

`reissue_capability` is also the JSON name inside
`KANDEV_GITHUB_CREDENTIAL_SCOPES` entries, which
`orchestrator/executor.githubCredentialScope` writes and the helper decodes.
The field names are therefore a persisted format and must not be renamed or
aliased.

The routes return only leases or credentials. A reissue response carries
`token` and `expires_at`; it never returns a Git credential. Resolve failures
use `*_credential_lease_invalid`, `*_credential_lease_expired`, or
`*_credential_lease_revoked` so the helper can decide whether reissue is
eligible. A malformed body returns `*_invalid_request`, a capability failure
returns `*_credential_reissue_denied`, and a scope failure returns
`*_credential_denied`.

The capability is an encrypted, authenticated bearer value with claims for the
normalized `gitcredentials.Scope` and an expiry. Its signer comes from the
stable `githubCredentialBroker.reissueSigningKey` configuration
(`KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY`). Leases remain
in-memory.

## Control flow

1. Launch issues a lease and, when a signer is configured, a capability sealed
   with the same normalized scope. Both travel only in the managed helper
   environment.
2. The helper validates Git's input against the issued scope and redeems the
   lease through `/resolve`.
3. When redemption returns an eligible lease failure, the helper posts the same
   non-secret request with `reissue_capability` and without the failed lease.
4. The handler binds the shared request type and passes the capability to the
   broker.
5. The broker validates the capability, compares its sealed scope with the
   helper request, refreshes provider-owned binding data, re-runs live
   authorization, and issues one replacement lease bound to the current
   credential generation.
6. The helper redeems the replacement lease once. No further reissue or scope
   fallback follows.

## Failure and recovery

Capability-invalid, expired, forged, scope-mismatched, terminal-session, and
changed-repository requests are denied and issue no lease. Provider and
transport failures are not retried. A lease scope error or a failed replacement
redemption ends the helper request.

A backend restart discards the lease map but not a capability signed with the
stable key. A new broker therefore reissues from live state. A workspace
credential rotation changes the provider binding; the old lease is revoked and
one reissue binds the replacement to the new generation.

A reissue replaces the previous lease for that capability. Concurrent helper
processes in one session can therefore invalidate each other's replacement
between issuance and retry; the failed request succeeds on a later sequential
call. The helper never loops.

## Persistence

Leases are intentionally in-memory and disappear on backend restart.
Capabilities survive only in the running execution environment and expire.
No lease, capability, or credential plaintext is written to the task database.
The signing key is installation configuration and must remain stable across
restarts.

## Security

Leases and capabilities are opaque bearer values. They are never logged, never
returned by browser-facing APIs, and never stored with plaintext Git
credentials. The reissue route is unauthenticated at the HTTP middleware layer
because the capability self-authenticates; the broker then verifies the exact
task, session, repository, provider, host, path, and optional parent identity
against live state. A capability grants no permission beyond its launch-time
scope.

## Observability

Failures surface through the broker's HTTP error codes and ordinary service
errors. Lease and capability values are not metric labels, log fields, or event
payloads.

## Related decisions

- [Provider-neutral Git credential broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
