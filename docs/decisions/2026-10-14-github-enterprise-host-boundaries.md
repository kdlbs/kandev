# ADR-2026-10-14-github-enterprise-host-boundaries: Bind GitHub operations to their host

**Status:** accepted
**Date:** 2026-10-14
**Area:** backend, frontend, security

## Context

The CLI picker already exposes Enterprise accounts, but token resolution rejects
non-GitHub.com hosts. Other clients, OAuth URLs, and workflow identities still
assume GitHub.com. Removing one guard would validate or use Enterprise
credentials against the wrong server.

The user selected GHES and Enterprise Cloud on `*.ghe.com`, including PAT, CLI,
App, and personal OAuth support. They explicitly selected GitHub's documented
server-side OAuth exchange for GHES versions without PKCE. Existing
[authentication ownership](0047-github-authentication-ownership.md) and
[provider origin identity](2026-07-20-repository-provider-origin-identity.md)
already require workspace and origin isolation.

## Decision

Preserve one automation connection per workspace. Resolve an immutable host
value from that connection or App registration and derive web, REST, GraphQL,
and CLI token-family behavior centrally. Durable external resources retain
their original host; changing the workspace connection never reinterprets them.
Callback state, credential generations, caches, and broker authorization include
or are bound to the same host identity.

OAuth security policy is explicit and captured at flow creation. GitHub.com and
GHE.com use PKCE. The GHES compatibility policy uses the confidential-client
authorization-code flow with a client secret, single-use state, fixed callback,
and workspace/registration/host/generation binding. No error-driven downgrade or
cross-host exchange is allowed. A future GHES PKCE policy must be based on
verified provider support, not on ignoring an exchange failure.

This decision establishes design intent; delivery remains pending in the
[implementation plan](../plans/github-enterprise-hosts/plan.md).

## Consequences

The first CLI fix needs host-aware transport as well as token resolution.
Host changes fence existing work; users may use another workspace to keep both
hosts active. Supporting GHES requires a distinct OAuth policy and capability
tests rather than assuming its APIs match GitHub.com. Existing authenticated
request, callback, and broker boundaries remain mandatory on every host.

## Alternatives considered

- A process-wide GitHub host: cannot support concurrent workspaces on different
  installations and risks ambient credential routing.
- Accept every host in the CLI token resolver alone: saves an unusable connection
  while later clients still target GitHub.com.
- Multiple hosts per workspace: changes established routing, watch ownership,
  and credential selection beyond the requested capability.
- Require PKCE everywhere: excludes personal OAuth on GHES versions without it;
  the user chose the documented server flow instead.
- Retry OAuth without PKCE after failure: makes the security policy dependent
  on errors and permits an unintended downgrade.

## References

- [Feature design](../specs/integrations/system-design/github-enterprise-hosts.md)
- [GHES App user authorization](https://docs.github.com/en/enterprise-server%403.20/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app)
- [GitHub CLI environment semantics](https://cli.github.com/manual/gh_help_environment)
