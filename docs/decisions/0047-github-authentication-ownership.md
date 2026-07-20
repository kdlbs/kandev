# 0047 — Separate GitHub deployment, workspace automation, and personal identities

> Amended by
> [ADR-2026-07-20-managed-github-app-registration](2026-07-20-managed-github-app-registration.md):
> deployment App registration may be environment-managed or created through Kandev's runtime App
> Manifest onboarding flow; workspace and personal ownership boundaries are unchanged.

- Status: accepted
- Date: 2026-07-19
- Area: backend, frontend, security
- Related: [Workspace GitHub Authentication](../specs/integrations/github-authentication.md),
  [ADR 0030](0030-workspace-scoped-integration-settings.md)

## Context

Kandev currently constructs one GitHub client for the entire installation. It discovers a `gh` CLI
login, environment token, or globally named secret and then reuses that client for interactive
queries, workspace watches, cleanup, CI automation, and executor token injection. Repository scope
settings are workspace-owned, but the identity that enforces them is not.

Self-hosted companies need a different trust model from a local single-user installation. An
organization may install a GitHub App to authorize repository automation centrally, while an
individual still needs authenticated-viewer queries and human attribution. GitHub exposes these as
different principals: installation access tokens act as the App installation, while App user access
tokens act on behalf of a user. Installation tokens cannot be treated as "the authenticated user."

GitHub App installation tokens expire after one hour, so copying one into an executor environment
at task launch is not a complete transport design. The App private key cannot be exposed to agents
to let them mint replacements.

## Decision

GitHub authentication has three ownership layers and is resolved by purpose:

1. **Deployment-owned App registration.** A deployment may configure one GitHub App registration.
   The App ID, client ID, private key, client secret, webhook secret, slug, and public callback URL
   are operator configuration. Workspace and user APIs never read or return this secret material.
2. **Workspace-owned automation connection.** Each workspace selects exactly one automation source:
   PAT, a named `gh` CLI host/login, a verified App installation, or the temporary
   `legacy_shared` migration source. Background work and agents always use this identity.
3. **User-owned personal connection within a workspace.** At most one verified App user connection
   exists per `(workspace_id, user_id)`. It provides authenticated-viewer semantics and human
   attribution. It is never injected into an agent. The current runtime uses `default-user`; future
   authentication can replace the current-user resolver without changing credential ownership.

The GitHub service resolves a credential using `(workspace_id, user_id, purpose)` and returns a
principal description and capability set with the client. Purpose is explicit: automation,
personal read, personal mutation, or git transport. A request never reads a process-global client.
Caches, rate limits, and singleflight keys include workspace, credential generation, and principal.

For local `gh` CLI auth, Kandev stores only host/login and runs
`gh auth token --hostname <host> --user <login>`. It does not call `gh auth switch`, and child
processes used for discovery strip inherited `GH_TOKEN` and `GITHUB_TOKEN` so ambient environment
state cannot override the selected account. Derived CLI tokens remain memory-only.

For managed git transport, an executor gets a Git credential helper and one
task/workspace/repository/generation-bound broker lease per attached GitHub repository. The helper
asks Kandev for a current automation token on each matching credential request. App installation
tokens are minted with GitHub's repository restriction for the redeemed repository and can refresh
during long tasks. The broker cannot mint after disconnect or revocation. App private keys and
personal tokens never enter the executor.

PAT/CLI automation uses the same helper contract, but those provider-issued bearer tokens cannot be
cryptographically narrowed after redemption. The lease and exact HTTPS host/path matching prevent
accidental cross-repository redemption; the agent subprocess remains trusted with every repository
and operation granted to the bearer token. A PATH-prepended `gh` shim redeems the primary
repository lease for each child invocation and isolates host `gh` configuration. This gives
App-backed `gh` commands primary-repository scope while Git's helper can select every attached
repository lease. An explicit profile `GITHUB_TOKEN` or `GH_TOKEN` is an unmanaged operator
override and bypasses broker selection.

Review watches persist the verified target login captured from a human connection at creation.
Installation-token polling renders explicit review qualifiers for that login rather than `@me`.
An arbitrary typed login is not accepted as current-user proof.

App setup and personal authorization use single-use state, PKCE for the user web flow, verified
installation/user association, signed webhooks, and delivery-ID deduplication. Installation tokens
are minted with an App JWT, cached in memory until shortly before expiry, and never persisted.
Personal access/refresh tokens are encrypted in the secret store and refreshed atomically.

Existing workspaces receive an explicit `legacy_shared` row on migration so upgrades preserve the
current behavior. New workspaces do not. Leaving legacy mode is irreversible, and legacy mode is
not a target architecture: it is a visible compatibility bridge while users reconfigure each
workspace. This narrowly amends ADR 0030's migration rule for GitHub authentication only.
Only `legacy_shared` may consult the active host `gh` account, backend `GITHUB_TOKEN`/`GH_TOKEN`, or
old globally named token secret; managed workspace connections do not use ambient auth as fallback.

## Consequences

- A different primary GitHub account requires a different workspace. Kandev does not add arbitrary
  N-account routing inside a workspace.
- A hosted workspace can use an App for organization-controlled automation and a personal user
  token for `My GitHub` and human-attributed actions.
- An App-only workspace can still create PRs, review, merge, and run watches when permissions allow,
  but GitHub attributes those actions to the App and `My GitHub` remains unavailable.
- Personal access is intersected with the workspace repository scope and automation-visible
  repositories, so attaching a broad user token cannot expand the workspace trust boundary.
- Every GitHub service entry point and background record must supply or derive a workspace. Missing
  ownership fails closed instead of falling back to another connection.
- Health is no longer one install-wide GitHub boolean. Status is workspace-specific and reports
  automation, personal identity, capabilities, and rate limits separately.
- Executors gain renewable GitHub credentials without receiving long-lived deployment or personal
  secrets.
- App-backed executor access is provider-enforced per repository. PAT/CLI executor access remains
  bounded by the provider token itself, so agents are part of the workspace automation trust
  boundary even though lease redemption is repository-matched.
- The GitHub REST/GraphQL token client becomes token-source-neutral; PAT, installation, and user
  tokens share transport behavior but retain distinct principal metadata.

## Alternatives Considered

### One connection per workspace, with no personal overlay

Rejected. A company would have to choose between centrally managed App automation and user-specific
queries/attribution, or create duplicate workspaces for the same organizational repository set.

### Arbitrary multiple connections with per-repository routing

Rejected. It requires conflict resolution for watches, cache ownership, mutations, and agent git
credentials without a demonstrated user need. One automation identity plus an optional personal
identity covers the confirmed local and hosted scenarios.

### Use the App installation as the authenticated user

Rejected. GitHub treats an installation as the App principal. It can query explicit login
qualifiers and perform permitted mutations, but it does not supply authenticated-viewer semantics
or human attribution.

### Inject installation tokens at task launch

Rejected. Installation tokens expire after one hour, so long-running tasks would fail. Giving the
executor the App private key to self-refresh would cross the deployment secret boundary.

### Continue using the active `gh` account

Rejected. `gh auth switch` changes host-global state and allows one workspace or process to affect
another. Named account resolution is deterministic and does not mutate the user's CLI configuration.

### Immediately disconnect all existing workspaces

Rejected. It would break upgrades and background automation without giving operators time to
configure each workspace. The explicit legacy mode preserves behavior while making the remaining
migration visible.
