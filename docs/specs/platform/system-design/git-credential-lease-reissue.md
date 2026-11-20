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
| `REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security), [Observability](#observability) |

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
- `internal/backendapp` selects the signer once at startup using the existing
  secrets master key provider or the configured override.
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
normalized `gitcredentials.Scope` and an expiry. Leases remain in memory.

### Signer selection

Select the signer in `backendapp.provideRepositories`, after loading the
secrets master key, in this order:

1. When `githubCredentialBroker.disableReissue` is true, select no signer.
   This backend startup setting defaults to `false`. It also accepts
   `KANDEV_GITHUB_CREDENTIAL_BROKER_DISABLE_REISSUE`.
2. When `githubCredentialBroker.reissueSigningKey` is nonblank, pass its
   configured value to the existing `gitcredentials.NewReissueCapabilitySigner`
   constructor. Keep the existing interpretation and support for
   `KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY`.
3. For an absent or blank override, use the 32-byte secrets master key as
   the HKDF-SHA-256 input key material. Use a `nil` salt, the fixed info string
   `kandev.gitcredentials.reissue.installation.v1`, and a 32-byte output.
   Encode the output with `base64.RawURLEncoding` (unpadded base64url) and pass
   that string to `gitcredentials.NewReissueCapabilitySigner`. The HKDF inputs,
   encoding, and existing constructor's key interpretation are part of the
   default signer's compatibility contract.

Store only the resulting signer on
`Repositories.gitCredentialReissueSigner *gitcredentials.ReissueCapabilitySigner`.
In `initGitCredentialAndRemoteWiring`, pass `repos.gitCredentialReissueSigner` to
`newGitCredentialBroker(githubSvc gitHubCredentialSource, pluginSvc *plugins.Service, repo gitCredentialTaskRepository, reissueSigner *gitcredentials.ReissueCapabilitySigner) *gitcredentials.Broker`.
The factory sets the selected signer on the shared `gitcredentials.Broker`.
Keep key material out of executor environments, settings responses, and logs.

The secrets store owns master-key loading and persistence. Failure to
initialize the master key already prevents startup. Propagate unexpected
signer initialization errors as startup errors. A successful boot has a signer
unless `disableReissue` is true. With reissue disabled, ordinary leases remain
available. The broker's nil-signer check rejects capability exchange through
both API route prefixes and the launch path issues leases alone.

The default lease expires after 12 hours of inactivity. A successful
redemption renews it. The capability expires 30 days after issuance.

## Control flow

1. Launch issues a lease and a capability sealed with the same normalized
   scope when a signer is available. Both travel only in the managed helper
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

When an expired lease is still in the map, `Broker.loadLease` returns an expired
lease response. Once the broker has swept that entry, it returns an invalid
lease response. Both codes permit one reissue with a valid capability. The
invalid-lease response alone leaves the cause of the missing record unknown.

When `resolveWithReissue` receives HTTP 401 with a `git_credential_lease_invalid`,
`git_credential_lease_expired`, `github_credential_lease_invalid`, or
`github_credential_lease_expired` code and an empty capability, it wraps the
typed broker error with conditional recovery guidance. If the session and
repository are still authorized, use **Restart session** to get a fresh lease.
If the session is already stopped, use **Resume**. This path sends no reissue
request. Lease-revoked and other failures retain their existing handling. Git
and the `gh` shim share this path. An agent provider can still show its own
generic error in Chat.

An upgraded backend can adopt a retained agentctl with its lease-only launch
environment. Reconnect and resume can also reuse that process. **Restart session**
stops the current execution, then resumes the same session. After the old
execution stops successfully, the fresh launch runs the executor's resume
credential preparation and receives a new lease and, when enabled, a
capability. The workspace and session remain available for resume. A failed
stop or unverified process identity follows existing recovery safeguards.

A reissue replaces the previous lease for that capability. Concurrent helper
processes in one session can therefore invalidate each other's replacement
between issuance and retry; the failed request succeeds on a later sequential
call. The helper never loops.

## Persistence

Leases are intentionally in-memory and disappear on backend restart.
Capabilities survive only in the running execution environment and expire.
No lease, capability, or credential plaintext is written to the task database.
The installation signer remains stable across restarts with the same
`master.key`. The explicit override remains stable while its value stays the
same. A capability issued by a different signer cannot be verified. The
existing secrets loader generates a replacement if the key file is missing,
unreadable, or the wrong length and it can write a new key. The replacement
cannot verify previous default capabilities or decrypt existing stored secrets.
Restore the matching `master.key` with the installation data. A signer-source
log of `installation` covers both an existing key and a generated replacement.

## Security

Leases and capabilities are opaque bearer values. They are never logged, never
returned by browser-facing APIs, and never stored with plaintext Git
credentials. The reissue route is unauthenticated at the HTTP middleware layer
because the capability self-authenticates; the broker then verifies the exact
task, session, repository, provider, host, path, and optional parent identity
against live state. A capability grants no permission beyond its launch-time
scope.

Eligible managed launches on Docker, SSH, Kubernetes, and Sprites receive the
same bearer capability as other managed launches. Previously lease-only
deployments will expose this capability in managed helper environments. A
copied value can authorize lease replacement after the original 12-hour idle
lease expires and for up to 30 days from issuance, subject to live authorization
and provider checks. Operators can disable issuance and exchange at backend
startup. Re-enabling with the same active key can restore the use of still-valid
capabilities. To prevent the active signer from verifying those capabilities,
select a different signing-key override, restart the backend, and restart
affected sessions. Preserve `master.key` for the secret store.

## Observability

Failures surface through the broker's HTTP error codes and ordinary service
errors. Lease and capability values are not metric labels, log fields, or event
payloads.

At startup, log `git_credential_reissue.signer_selected` once at Info level
with a `source` field of `installation`, `configured`, or `disabled`. This
diagnostic reports signer selection. It omits key, lease, capability, and
credential values.

## Related decisions

- [Provider-neutral Git credential broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
- [Default lease reissue signer](../../../decisions/2026-11-18-default-git-credential-reissue-signer.md)
- [Idle lease recovery work](../../../plans/git-credential-idle-lease-recovery/plan.md)
