# ADR-2026-07-31-provider-neutral-git-credential-broker: Provider-Neutral Git Credential Broker

**Status:** accepted
**Date:** 2026-07-31
**Area:** backend, protocol, security

## Context

GitHub has lease mechanics in a GitHub-specific broker, while other paths inject raw
credentials into executor wiring. A repository-provider plugin needs clone, fetch, and
push credentials without persisting tokens in URLs, task state, environment variables,
logs, or process arguments. Self-managed repository URLs must retain exact host, path,
and context path.

## Decision

Kandev extracts generic lease issuance, redemption, authorization, expiry, and
revocation into `internal/gitcredentials/`. A composite resolver preserves current
GitHub behavior through an adapter and adds an optional plugin `ResolveGitCredential`
RPC. The plugin receives host-verified plugin/provider/workspace/task/session/
repository/exact-host/path scope and returns a generic short-lived credential shape;
provider-specific username/token formatting remains plugin-owned.

Credential-capable plugins also implement `GetGitCredentialBinding` for the same exact
scope. It returns an opaque non-secret connection generation. Kandev stores that value
when issuing a lease and compares it before and after each secret redemption; an empty,
unsupported, rotated, or disconnected binding revokes the lease. The host never
resolves a secret merely to inspect its generation.

Executors receive opaque short-lived helper leases. Redeeming a lease re-resolves
against the live provider, so OAuth refresh does not require a task restart. Leases are
scoped to exact workspace, task, session, repository, host, and path; they revoke on
teardown, plugin disable/error/uninstall, credential-generation change, and connection
reset. Disabling, failing, or uninstalling a plugin immediately revokes every lease for
each provider it declared; later redemption does not wait for expiry. Host matching is
normalized, while repository paths remain case-sensitive and exact. The helper carries
the requested path without appending or removing `.git`. Remote executor transport uses
HTTPS.

Host-local initial repository materialization uses the same verified scope before it
creates a checkout. The orchestrator and workspace-source materializer must carry the
canonical task ID, active session ID, and persisted repository ID explicitly into the
clone credential request; the cloner must not reconstruct them from provider data or
accept browser-selected identifiers. A plugin-provider clone without that complete
scope fails closed. GitHub's legacy workspace credential adapter remains compatible,
but generic plugin providers never receive a weaker initial-clone request than later
fetch/push lease redemption.
`RepositoryCloneURL` remains authoritative and is never reconstructed from provider
fragments.

Before creating a worktree from an existing plugin-provider checkout, the orchestrator
performs a strict origin refresh through that same exact scope. It then marks the
in-memory launch as already refreshed; local and worktree preparers consume local and
`origin/*` refs without issuing a second unauthenticated fetch or pull. A failed scoped
refresh fails the launch closed.

## Consequences

New repository providers can participate in Git operations without host credential
branches or secret leakage. The host must carry stricter lease scope and lifecycle
checks, and plugins must resolve credentials promptly while handling rotation. GitLab
is not migrated as part of this decision.

## Alternatives Considered

- Put provider tokens in clone URLs or executor environment variables: rejected
  because both spread durable or observable secrets.
- Add a Bitbucket broker beside GitHub's: rejected because it repeats a cross-provider
  security boundary.
- Reconstruct clone URLs from provider fields: rejected because Data Center context
  paths and configured origins would be lost or silently changed.
