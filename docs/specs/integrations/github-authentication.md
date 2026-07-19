---
status: shipped
created: 2026-07-19
owner: Kandev
---

# Workspace GitHub Authentication

## Why

GitHub authentication is currently shared by the whole Kandev installation, so changing the
authenticated account can silently change repository access and GitHub attribution for every
workspace. Local users need deterministic PAT or `gh` CLI isolation per workspace, while a
company-hosted installation needs organization-managed GitHub App automation without losing the
option to perform personal, human-attributed GitHub work.

## What

- Each workspace has at most one **automation connection**. Its source is one of `pat`, `gh_cli`,
  `github_app_installation`, or the migration-only `legacy_shared` source.
- Each Kandev user may have at most one optional **personal connection per workspace**. In this
  feature, the personal source is a GitHub App user access token. The current single-user runtime
  owns this connection as `default-user`; the contract remains keyed by `user_id` for a future
  authenticated multi-user runtime.
- A PAT or selected `gh` CLI login is a human automation identity and also supplies personal
  behavior when no separate personal connection exists.
- A GitHub App installation is an automation identity, not a person. It can read and mutate the
  repositories granted to the installation, including creating pull requests, submitting reviews,
  and merging when its permissions allow; GitHub attributes those mutations to the App.
- When automation uses a GitHub App, `My GitHub` and operations that require authenticated-viewer
  semantics require a verified personal connection. Kandev does not treat an installation or an
  unverified typed login as the current GitHub user.
- User-triggered GitHub mutations prefer the personal connection, then fall back to a human
  automation connection, then to the App installation. The UI identifies the effective actor and
  never presents an App-attributed mutation as human-attributed.
- Background watches, cleanup, CI automation, and repository discovery always use the workspace
  automation connection. Managed agent git transport and agent-initiated GitHub operations use it
  as well. Personal credentials are never exposed to an agent or executor.
- Managed executor authentication uses a task/repository/generation-bound broker lease unless an
  executor profile explicitly supplies `GITHUB_TOKEN` or `GH_TOKEN`. An explicit profile token is
  an unmanaged operator override and does not inherit workspace isolation.
- The externally reachable credential-broker base URL is deployment configuration independent of
  GitHub App registration, so PAT-only and named CLI workspaces can use remote executors.
- Remote executors probe the broker's exact resolution route before clone or agent startup. An
  unauthenticated `GET` returns only `204 No Content` when the in-process broker is ready; token
  redemption remains a lease-authenticated `POST` to the same route.
- A review watch snapshots a verified target GitHub login when it is created. App-backed polling
  uses `review-requested:<login>` instead of `@me`; creating a user-targeted review watch requires a
  verified personal or human automation identity. Migrated watches that cannot derive a verified
  target are disabled with an actionable reconnect error.
- A personal connection cannot expand the workspace repository boundary. Its effective repository
  set is the intersection of the workspace repository scope, the automation connection's allowed
  repositories, and the personal account's access.
- Selecting `gh_cli` stores the GitHub host and login, not the token. Kandev resolves that exact
  account with `gh auth token --hostname <host> --user <login>` and never calls `gh auth switch`.
- A brokered GitHub App installation token is minted for only the repository whose lease is
  redeemed. PAT and selected CLI tokens remain bearer credentials with their provider-granted
  repository and operation scopes once delivered to a trusted agent subprocess. Lease matching
  prevents accidental redemption for another repository; it does not cryptographically narrow a
  PAT or CLI token.
- PATs and GitHub App user tokens are stored through Kandev's encrypted secret store. GitHub App
  installation tokens are short-lived and cached only in memory. The App private key, client
  secret, and webhook secret are deployment-owned configuration and are never returned by a
  workspace API or injected into an executor.
- One GitHub App registration is supported per Kandev deployment. Any number of workspaces may bind
  to installations of that App, but each workspace binds to only one installation.
- Existing workspaces migrate to `legacy_shared` and initially retain the current install-wide
  resolution behavior. Once a workspace chooses PAT, `gh` CLI, or App authentication, it cannot
  switch back to `legacy_shared`. Workspaces created after the migration start disconnected.
- Only `legacy_shared` consults an ambient host-active `gh` account, backend `GITHUB_TOKEN` or
  `GH_TOKEN`, or old globally named token secret. Managed workspace sources never fall back to
  those credentials.
- Copying workspace settings does not copy PATs, personal credentials, CLI account selection, or an
  App installation binding. The target workspace chooses its own connection.

## Identity And Routing

| Purpose | First choice | Fallback | Attribution |
| --- | --- | --- | --- |
| Background reads and writes | Workspace automation | None | Automation principal |
| Managed agent git and GitHub access | Workspace automation | None | Automation principal |
| `My GitHub` reads | Personal connection | Human PAT/CLI automation | Human principal |
| User-triggered mutation | Personal connection | Human PAT/CLI, then App installation | Effective principal shown in UI |

If an App-only workspace has no personal connection, automation continues to work and supported
manual mutations run as the App. `My GitHub` is unavailable and shows a personal-connect action.
An explicit executor-profile `GITHUB_TOKEN` or `GH_TOKEN` overrides the managed-agent row and is
outside workspace authentication routing.

## GitHub App Permissions

The settings status reports each missing capability instead of reducing the entire connection to a
single connected/disconnected boolean. Full Kandev behavior requires the App registration and
installation to grant:

- Repository metadata: read.
- Contents: read and write, including HTTP clone/fetch/push through the executor credential helper.
- Pull requests: read and write.
- Issues: read and write.
- Checks, commit statuses, and Actions: read.
- Administration: read where branch-protection details are requested.
- Organization members: read where organization or team membership is requested.
- Workflows: write when an agent must modify files under `.github/workflows`.

The App subscribes to `installation`, `installation_repositories`, and
`github_app_authorization` webhooks. Polling remains the source for PR, issue, review, and CI watch
content; those events are not required for this feature.

## Data Model

### `github_workspace_connections`

One row per configured workspace automation identity.

| Field | Type | Constraint |
| --- | --- | --- |
| `workspace_id` | text | Primary key; workspace foreign key |
| `source` | enum text | `legacy_shared`, `pat`, `gh_cli`, `github_app_installation` |
| `github_host` | text | Required; `github.com` in this feature |
| `login` | text nullable | Verified PAT/CLI login |
| `installation_id` | int64 nullable | Required only for App source |
| `installation_account_login` | text nullable | Verified GitHub account/org login |
| `installation_account_type` | text nullable | `User` or `Organization` |
| `status` | enum text | `active`, `invalid`, `suspended`, `revoked` |
| `credential_generation` | int64 | Increments whenever auth material or source changes |
| `last_error` | text nullable | Last observable connection error |
| `created_at`, `updated_at` | timestamp | Required |

The PAT secret key is deterministic from `workspace_id` and is never returned by the API. A
`legacy_shared` row has no workspace secret and delegates to the pre-migration shared resolver.

### `github_user_connections`

One optional personal GitHub identity per `(workspace_id, user_id)`.

| Field | Type | Constraint |
| --- | --- | --- |
| `workspace_id`, `user_id` | text | Composite primary key |
| `github_user_id` | int64 | Verified stable GitHub user ID |
| `login` | text | Verified GitHub login |
| `status` | enum text | `active`, `invalid`, `revoked` |
| `access_expires_at` | timestamp | Required for expiring user tokens |
| `refresh_expires_at` | timestamp nullable | Returned by GitHub when enabled |
| `credential_generation` | int64 | Increments on refresh, replacement, or revocation |
| `last_error` | text nullable | Last refresh or validation error |
| `created_at`, `updated_at` | timestamp | Required |

Access and refresh tokens use deterministic secret-store keys derived from both `workspace_id` and
`user_id`. Tokens are replaced atomically before the database expiry metadata is advanced.

### `github_auth_flows`

Short-lived, single-use records bind a cryptographically random OAuth/setup state to
`workspace_id`, `user_id`, flow kind, PKCE verifier, expiry, and consumption time. Expired or
consumed state cannot complete a connection.

### `github_webhook_deliveries`

The GitHub delivery ID is unique. A received-at timestamp and terminal processing result provide
idempotency and an audit trail without persisting webhook secrets or access tokens.

### Workspace ownership on background records

`github_pr_watches` and `github_task_prs` persist `workspace_id`. Existing rows backfill through
their task/session relationship. Background work fails closed when a workspace cannot be derived;
it never falls back to another workspace's connection.

`github_review_watches` persists `target_login`, the verified human login captured when the watch is
created. It is not an authentication credential and cannot be supplied as an arbitrary replacement
for the current user.

## API Surface

All JSON errors include a stable `code` and human-readable `error`. Except for the public callback
and webhook endpoints, `workspace_id` is required and must be authorized for the current Kandev
user.

### Connection status and configuration

- `GET /api/v1/github/status?workspace_id=<id>` returns `automation`, `personal`,
  `effective_personal_actor`, `effective_manual_mutation_actor`, and `github_app_available`.
  Automation status includes source, verified actor, App installation metadata, capabilities,
  missing permissions, rate limits, and migration state. Personal status never returns tokens.
- `GET /api/v1/github/auth/gh-cli/accounts` returns locally authenticated host/login choices and
  marks the selected workspace account when `workspace_id` is supplied.
- `PUT /api/v1/github/workspace-connection?workspace_id=<id>` accepts exactly one source:
  `{ "source": "pat", "token": "..." }` or
  `{ "source": "gh_cli", "host": "github.com", "login": "..." }`.
  The credential is validated before the connection is replaced.
- `DELETE /api/v1/github/workspace-connection?workspace_id=<id>` removes workspace-owned secret
  material and leaves the workspace disconnected. It never deletes host `gh` credentials or the
  deployment App registration.
- Existing `/api/v1/github/token` requests remain compatibility aliases for one release, require
  `workspace_id`, and return a deprecation header.

### GitHub App installation

- `POST /api/v1/github/app/install/start` with `{ "workspace_id": "..." }` creates a single-use
  flow and returns an operator-visible GitHub installation/authorization URL.
- `GET /api/v1/github/app/install/callback` consumes GitHub's callback, verifies the state and
  installation association using the authorizing GitHub user, stores only verified installation
  metadata, and redirects to settings with a non-secret result code.
- `DELETE /api/v1/github/app/installation?workspace_id=<id>` removes the workspace binding; it does
  not uninstall the App from GitHub.
- `POST /api/v1/github/app/webhook` is public, requires a valid GitHub HMAC signature, deduplicates
  on delivery ID, and updates installation/repository/user-authorization state.

### Personal connection

- `POST /api/v1/github/personal-connection/start` with `{ "workspace_id": "..." }` starts the
  GitHub App web flow with state and PKCE and returns its authorization URL.
- `GET /api/v1/github/personal-connection/callback` validates state and PKCE, verifies the returned
  GitHub user, stores access/refresh secrets, and redirects to settings with a non-secret result.
- `DELETE /api/v1/github/personal-connection?workspace_id=<id>` deletes only the current user's
  personal connection and secrets.

## State Machines

### Workspace automation

- `disconnected -> active`: a PAT/CLI account validates, or a verified App installation completes.
- `legacy_shared -> active`: the workspace explicitly chooses a new source; this is irreversible.
- `active -> invalid`: validation or token minting proves credentials unusable.
- `active -> suspended`: GitHub reports an App installation suspension.
- `active|invalid|suspended -> revoked`: GitHub reports deletion, or the user removes the binding.
- `invalid|suspended -> active`: validation succeeds, a token is replaced, or GitHub reports
  unsuspension.

### Personal connection

- `disconnected -> active`: a single-use OAuth/PKCE callback validates.
- `active -> active`: refresh succeeds and token secrets plus expiry metadata change atomically.
- `active -> invalid`: refresh or verification fails; Kandev does not fall back to another user's
  credentials.
- `active|invalid -> revoked`: the user disconnects or GitHub sends
  `github_app_authorization.revoked`.

## Permissions

- A workspace member who can edit integrations may configure or disconnect its automation
  connection. The current single-user runtime treats `default-user` as that member.
- A personal connection can be created, inspected, or removed only by its owning `user_id`.
- Only the deployment operator can configure the App ID, client ID, private key, client secret,
  webhook secret, slug, and public callback base URL.
- The webhook endpoint has no user session requirement and authorizes only by signature plus
  recognized App/installation identifiers.
- Repository operations are denied when the target is outside the effective workspace repository
  boundary, even if a personal token could access it.

## Failure Modes

- PAT or CLI validation failure leaves the previous workspace connection unchanged.
- Missing `gh`, an unknown selected login, or `gh auth token` failure marks only that workspace
  invalid and includes account-specific diagnostics. Kandev never changes the host's active `gh`
  account.
- Incomplete App configuration leaves PAT, CLI, and legacy connections available while settings
  reports App setup as unavailable.
- Invalid, expired, replayed, workspace-mismatched, spoofed, or inaccessible setup/OAuth callbacks
  fail closed and persist no connection.
- Installation tokens refresh before expiry and coalesce concurrent refreshes. A mint failure does
  not reuse an expired token and affects only that installation.
- A personal refresh failure does not fall back to another user. Personal features show reconnect;
  App-backed automation remains available.
- Invalid webhook signatures return `401`; duplicate valid deliveries return success without
  applying the transition twice; unknown installations do not create workspace bindings.
- Missing App permissions produce a capability-specific error. Unrelated operations continue when
  their permissions are present.
- Agent credential-broker failure causes git/GitHub authentication to fail closed. Kandev never
  falls back to a host-global token or a personal token.
- Remote executor launch fails before cloning when its configured broker route does not return the
  exact readiness response; network errors, redirects, wrong routes, and server errors do not fall
  back to embedding a token in a clone URL.
- Rate limits and response caches are isolated by workspace, credential generation, principal, and
  GitHub resource bucket.

## Persistence Guarantees

- Workspace and personal connection metadata, encrypted long-lived secrets, webhook delivery IDs,
  and OAuth/setup flow state survive backend restarts.
- Installation tokens and CLI-derived tokens are memory-only. They are reacquired after restart and
  refreshed before expiry.
- Executor git credentials are fetched on demand through a task-scoped credential helper. Long
  running tasks remain able to authenticate after an installation token expires without receiving
  the App private key.
- Agent-issued `gh` commands run through a broker-aware shim that obtains the primary repository's
  automation token per invocation, sets it only on the child `gh` process, and isolates host CLI
  configuration. It cannot read the user's host-active account. Git's credential helper can select
  the matching lease for every repository attached to the task; with App auth, the `gh` shim is
  intentionally primary-repository scoped.
- Disconnect and revocation increment credential generation, clear relevant caches, and prevent a
  previously issued broker lease from minting another token.

## Scenarios

- **GIVEN** two workspaces using different PATs, **WHEN** each lists accessible repositories,
  **THEN** each result and rate-limit status comes only from that workspace's PAT.
- **GIVEN** two host `gh` logins, **WHEN** a workspace selects one login, **THEN** all of its GitHub
  calls resolve that exact login without changing the host's active login.
- **GIVEN** an existing installation after upgrade, **WHEN** no workspace has been reconfigured,
  **THEN** every pre-existing workspace continues using shared authentication and shows a migration
  label.
- **GIVEN** a workspace created after upgrade, **WHEN** GitHub settings first open, **THEN** the
  workspace is disconnected and does not inherit the legacy shared credential.
- **GIVEN** a legacy workspace, **WHEN** it validates and selects a PAT, **THEN** it stops using
  shared authentication and cannot select legacy mode again.
- **GIVEN** a configured deployment App, **WHEN** an organization admin completes installation,
  **THEN** only the verified installation is bound to the initiating workspace.
- **GIVEN** a spoofed, expired, replayed, or wrong-workspace App callback, **WHEN** it is handled,
  **THEN** no workspace binding or secret is created.
- **GIVEN** an App installation with pull-request and contents write permissions, **WHEN** workspace
  automation creates or merges a PR, **THEN** GitHub attributes the action to the App.
- **GIVEN** App-only automation and no personal connection, **WHEN** the user opens `My GitHub`,
  **THEN** results are withheld and a personal-connect action is shown while background automation
  remains operational.
- **GIVEN** an App workspace with a valid personal connection, **WHEN** the user opens `My GitHub`
  or submits a review, **THEN** the verified user's view and attribution are used within the
  workspace repository boundary.
- **GIVEN** a personal account that can access an extra repository outside the App installation or
  workspace scope, **WHEN** it queries or mutates that repository through Kandev, **THEN** Kandev
  denies the operation.
- **GIVEN** a revoked personal authorization, **WHEN** the webhook arrives, **THEN** personal
  features require reconnect and workspace App automation remains active.
- **GIVEN** a suspended or deleted installation, **WHEN** the webhook arrives, **THEN** affected
  workspaces stop App-backed operations and display the installation state.
- **GIVEN** a task running longer than one hour with App automation, **WHEN** it performs another git
  fetch or push, **THEN** the credential helper obtains a fresh installation token without exposing
  deployment or personal secrets.
- **GIVEN** a task attached to two App-visible repositories, **WHEN** Git requests credentials for
  each HTTPS path, **THEN** the helper redeems the matching lease and each returned installation
  token is restricted to that repository.
- **GIVEN** a workspace PAT or selected CLI account with access beyond one task repository, **WHEN**
  an agent redeems its matching lease, **THEN** the agent subprocess receives that bearer token's
  full provider-granted scope and is treated as trusted with the workspace automation grant.
- **GIVEN** a profile with an explicit `GITHUB_TOKEN` or `GH_TOKEN`, **WHEN** the task launches,
  **THEN** that unmanaged profile credential takes precedence over the managed workspace broker.
- **GIVEN** background watches in two workspaces, **WHEN** the poller runs concurrently, **THEN**
  every watch resolves its own workspace automation connection and never falls back across
  workspaces.
- **GIVEN** a review watch created for a verified user and App automation, **WHEN** the poller runs,
  **THEN** it searches `review-requested:<verified-login>` with the installation identity and never
  interprets the App as `@me`.
- **GIVEN** a workspace copy, **WHEN** the target opens GitHub settings, **THEN** repository settings
  may be copied but authentication is disconnected and must be chosen explicitly.
- **GIVEN** desktop and mobile settings viewports, **WHEN** automation and personal states change,
  **THEN** both identities, actors, diagnostics, and connect/disconnect actions remain usable
  without clipping or overlap.

## Out Of Scope

- Multiple interchangeable automation accounts or per-repository credential routing inside one
  workspace. A different primary GitHub account belongs in another workspace.
- More than one App registration per Kandev deployment.
- GitHub Enterprise Server or hosts other than `github.com`.
- Implementing Kandev multi-user login, workspace membership, or role management; this feature only
  keeps personal auth ownership keyed by `user_id`.
- Copying authentication secrets between workspaces.
- Replacing existing PR/issue/CI pollers with webhook-driven content synchronization.
- Uninstalling the GitHub App from the GitHub organization when a workspace disconnects.

## Implementation Plan

See [the implementation plan](../../plans/github-authentication/plan.md).
