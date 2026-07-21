---
status: draft
created: 2026-07-19
amended: 2026-07-21
owner: Kandev
---

# Workspace GitHub Authentication

## Why

GitHub credentials must not silently cross workspace boundaries. A local workspace may only need a
human PAT or a named `gh` CLI account, while unattended company automation benefits from a GitHub
App's short-lived, repository-scoped installation tokens. Users also need to keep work and personal
automation under different GitHub Apps without operating separate Kandev deployments.

## What

- Every workspace chooses exactly one automation source: PAT, a named `gh` CLI account, a verified
  GitHub App installation, or the migration-only `legacy_shared` source.
- GitHub App registration is configured from the workspace GitHub settings flow. There is no
  singleton GitHub App settings page and no automatically active deployment App.
- A workspace may select a GitHub App registration already known to the Kandev deployment, import
  an existing GitHub App that the user owns, or create a new GitHub App through GitHub's App
  Manifest flow. Import and creation guide the user through ownership, callback, webhook,
  permission, visibility, and installation requirements.
- The deployment stores a catalog of GitHub App registrations because a user may intentionally
  reuse one App across workspaces. Each workspace still selects and installs an App independently.
  Selecting an existing registration never binds another workspace automatically.
- Users who require independent root credentials, bot identity, revocation, or ownership create a
  separate registration for each trust boundary. Work and personal workspaces can therefore use
  different Apps.
- Reusing one registration shares its App private key, client secret, webhook secret, permission
  policy, and bot identity. Installation tokens, workspace repository scope, connection generation,
  broker leases, health, and personal OAuth tokens remain workspace isolated.
- A newly created App defaults to private, meaning GitHub permits installation only on the account
  that owns it. The user may explicitly choose public when the same App must be installable on other
  GitHub accounts or organizations. Public does not list the App in Marketplace, reveal secrets, or
  grant repository access without installation approval.
- PAT and named CLI automation act as the verified human account. A separate `My GitHub` connection
  is only offered when workspace automation uses a GitHub App, because App installations are not
  people and cannot provide authenticated-viewer semantics.
- User-triggered mutations prefer the workspace's verified personal connection, then a human
  PAT/CLI automation connection, then the App installation. The UI always identifies the effective
  actor and never labels an App mutation as human-attributed.
- Background watches, cleanup, repository discovery, workflow sync, managed agent git transport,
  and agent-initiated GitHub operations always use the workspace automation connection. Personal
  credentials are never exposed to agents or executors.
- Managed executors receive task/repository/generation-bound broker leases. App installation tokens
  are minted for the requested repository and cached only in memory. PAT/CLI tokens retain their
  provider-granted scope once delivered to a trusted agent subprocess.
- Explicit executor-profile `GITHUB_TOKEN` or `GH_TOKEN` values remain unmanaged operator overrides
  and take precedence over the workspace broker.
- The unpublished `KANDEV_GITHUB_APP_*` configuration introduced on this branch is removed. Setting
  those variables does not create or configure a registration; operators use the guided import
  flow for an App they already own.
- Existing released workspaces migrate to `legacy_shared`; new workspaces start disconnected.
  Once a workspace leaves legacy mode it cannot return. The unpublished singleton registration
  schema on this branch is rewritten directly and receives no compatibility migration.
- Copying a workspace copies repository preferences but never copies a PAT, CLI account selection,
  App installation binding, registration secret, or personal identity.

## Choosing A Method

| Method | Use when | Benefits | Costs and limits |
| --- | --- | --- | --- |
| PAT | A local or simple workspace should act as one person | Fast setup; human attribution | Long-lived bearer secret; agents receive its full provider scope |
| Named `gh` CLI | The desired human account already exists in local `gh` auth | No token copied into Kandev; deterministic account selection | Depends on host CLI credentials; remote agents still use the brokered bearer token |
| GitHub App | An organization or unattended workspace needs managed automation | Short-lived repository-scoped tokens; independent revocation; App attribution | Requires App ownership, callback/webhook configuration, installation, and a public HTTPS URL for full lifecycle health |

The workspace UI explains that an App is recommended for background jobs and managed agents, but
it does not claim agents can only use the selected method. Explicit executor credentials and other
unmanaged tools remain outside Kandev's workspace credential contract.

## Identity And Routing

| Purpose | First choice | Fallback | Attribution |
| --- | --- | --- | --- |
| Background reads and writes | Workspace automation | None | Automation principal |
| Managed agent git and GitHub access | Workspace automation | None | Automation principal |
| `My GitHub` reads | Personal connection | Human PAT/CLI automation | Human principal |
| User-triggered mutation | Personal connection | Human PAT/CLI, then App installation | Effective principal shown in UI |

An App-only workspace without personal OAuth remains usable for automation and App-attributed
mutations. `My GitHub` instead offers a personal connection created with the same App registration
as the workspace installation.

## GitHub App Policy

Kandev-created Apps request repository metadata read; contents read/write; pull requests read/write;
issues read/write; checks, statuses, and Actions read; administration read; organization members
read; and workflows write. The UI exposes this policy through a permissions button and dialog, not
a row of chips. The App subscribes to `installation`, `installation_repositories`, and
`github_app_authorization`.

An imported App must meet the same callback, setup URL, webhook, event, and permission requirements.
Kandev validates the App identity and reports missing capabilities after installation. It does not
silently change an imported App's GitHub settings. The guide provides exact values and GitHub links
for the user to apply.

## Data Model

### `github_app_registrations`

One row per GitHub App known to this Kandev deployment. Registration metadata is catalog state;
workspace use is represented only by an explicit workspace connection.

| Field | Type | Constraint |
| --- | --- | --- |
| `id` | UUID text | Primary key; allocated before manifest creation |
| `source` | enum text | `managed` (manifest-created) or `imported` |
| `display_name` | text | Non-empty user-facing disambiguator |
| `github_host` | text | `github.com` in this feature |
| `app_id` | int64 | Positive; unique with `github_host` |
| `client_id` | text | Non-secret App OAuth client identifier |
| `slug` | text | Verified App slug |
| `owner_login` | text | Verified App owner |
| `owner_type` | enum text | `User` or `Organization` |
| `visibility` | enum text | `private` or `public` |
| `public_base_url` | text | Canonical public HTTPS origin |
| `created_for_workspace_id` | text nullable | Provenance only; not an automatic binding |
| `credential_generation` | int64 | Positive; cache and lease invalidation key |
| `credential_secret_id` | text | Non-empty encrypted bundle pointer |
| `status` | enum text | `active` or `invalid` |
| `webhook_status` | enum text | `unverified`, `verified`, or `failing` |
| `last_webhook_at` | timestamp nullable | Last correctly signed delivery for this registration |
| `last_error` | text nullable | Sanitized validation or runtime failure |
| `created_at`, `updated_at` | timestamp | Required |

Managed and imported credentials are immutable encrypted bundles under
`github:app-registration:<registration-id>:g<generation>:<nonce>`. A versioned bundle contains the
private key, client secret, and webhook secret. Metadata points to the active bundle only after the
bundle is durable. Every registration is represented by one verified catalog row and encrypted
bundle; there is no synthetic, configuration-backed, or globally active registration.

### `github_app_registration_flows`

Manifest flows store `state_hash`, preallocated `registration_id`, initiating `workspace_id` and
`user_id`, owner type/login, display name, visibility, canonical public base URL, manifest revision,
expiry, consumption time, and creation time. A new flow for the same workspace supersedes its older
unconsumed flow. State is random, hashed at rest, single-use, and expires after one hour.

### `github_workspace_connections`

One row per configured workspace automation identity. Existing fields remain, with:

| Field | Type | Constraint |
| --- | --- | --- |
| `app_registration_id` | UUID text nullable | Required only for `github_app_installation`; FK to `github_app_registrations.id` with delete restricted |

For an App connection, `installation_id`, verified account login/type, and
`app_registration_id` must all be present. PAT/CLI/legacy rows must have no registration ID.

### `github_user_connections`

One optional personal identity per `(workspace_id, user_id)`. Add required
`app_registration_id`, which must equal the current workspace App connection's registration. Access
and refresh tokens remain encrypted under workspace/user-derived keys. Switching away from that
App deletes the old personal secrets and increments its generation before the new automation
connection becomes visible.

### `github_auth_flows`

Installation and personal OAuth flows include `app_registration_id` in addition to workspace,
user, expected connection generation, PKCE material, expiry, and consumption state. A callback is
valid only when both its route registration ID and stored registration ID match.

### `github_webhook_deliveries`

The primary key is `(app_registration_id, delivery_id)`. A delivery is claimed only after the
registration-specific HMAC signature is verified. The row records event, terminal result, received
time, and processed time without payload secrets or tokens.

Background PR, task, and review-watch records retain `workspace_id`; review watches also retain the
verified target human login. Missing or contradictory ownership fails closed.

## API Surface

All non-public endpoints require workspace authorization. The current trusted-single-user runtime
maps this to `default-user`; mutually untrusted deployments require real workspace/admin roles
before exposing registration management.

### Registration catalog

- `GET /api/v1/github/app/registrations?workspace_id=<id>` returns accessible non-secret
  registrations, source, identity, visibility, callback URLs, health, whether each is selected by
  this workspace, and sharing implications. It never returns credentials.
- `POST /api/v1/github/app/registrations/manifest/start` accepts `workspace_id`, `display_name`,
  owner type/login, visibility, and public base URL. It returns the GitHub owner-specific manifest
  submission URL, generated manifest, state, registration ID, revision, and expiry.
- `GET /api/v1/github/app/registrations/:registrationId/manifest/callback` consumes state, converts
  the one-hour code, verifies identity and policy, commits an encrypted bundle plus metadata, and
  returns to that workspace's GitHub settings. It does not select or install the App automatically.
- `POST /api/v1/github/app/registrations/import/prepare` creates a short-lived, single-use import
  preparation for the initiating workspace. It returns `registration_id`, `public_base_url`,
  `manifest_callback_url`, `install_callback_url`, `personal_callback_url`, `webhook_url`,
  `setup_url`, `permissions`, `events`, and `expires_at` so the user can configure the existing App
  before submitting any root credentials.
- `POST /api/v1/github/app/registrations/import` consumes the prepared `registration_id` and accepts
  the workspace context, label, App ID, client ID/secret, slug, private key, webhook secret, owner,
  and visibility. It verifies the App via GitHub before atomically persisting it. Duplicate
  `(host, app_id)` returns `github_app_already_registered` and the non-secret existing registration
  ID.
- `PATCH /api/v1/github/app/registrations/:registrationId` changes only `display_name`.
- `DELETE /api/v1/github/app/registrations/:registrationId` deletes a registration only when no
  workspace or personal connection references it.

### Workspace automation

- `GET /api/v1/github/status?workspace_id=<id>` returns automation, personal identity, effective
  actors, App registration metadata, capabilities, missing permissions, and migration state.
- `GET /api/v1/github/auth/gh-cli/accounts?workspace_id=<id>` lists exact local host/login choices.
- `PUT /api/v1/github/workspace-connection?workspace_id=<id>` configures validated PAT or named CLI
  auth. App connections can only be committed by the verified installation callback.
- `POST /api/v1/github/app/install/start` accepts `workspace_id` and `app_registration_id`, stores a
  single-use flow, and returns the registration-specific GitHub installation URL.
- `GET /api/v1/github/app/registrations/:registrationId/install/callback` verifies state, App,
  installation, authorizing user, and owner association before atomically replacing workspace
  automation. Failure leaves the previous automation connection unchanged.
- `DELETE /api/v1/github/workspace-connection?workspace_id=<id>` removes workspace secret material
  and the App installation binding but never deletes or uninstalls the registration.
- `POST /api/v1/github/app/registrations/:registrationId/webhook` is public. It chooses exactly that
  registration's webhook secret, validates HMAC before parsing or claiming the delivery, and only
  mutates connections whose registration and installation both match.

### Personal identity

- `POST /api/v1/github/personal-connection/start` uses the workspace's active App registration and
  returns its PKCE/state authorization URL.
- `GET /api/v1/github/app/registrations/:registrationId/personal/callback` validates route, state,
  PKCE, current workspace registration, and GitHub user before storing tokens.
- `DELETE /api/v1/github/personal-connection?workspace_id=<id>` deletes only the current user's
  workspace personal connection and secrets.

Existing `/api/v1/github/token` remains a one-release compatibility alias with mandatory
`workspace_id` and a deprecation header.

## State Machines

### Registration

- `absent -> registering`: start a manifest flow or prepare an import with a preallocated ID.
- `registering -> active`: verify GitHub identity and durable credential bundle, then publish catalog
  metadata and hot-load the registration generation.
- `registering -> absent`: cancellation, expiry, replay, conversion, validation, or persistence
  failure leaves no selectable registration. An orphan App created on GitHub is reported with
  recovery instructions.
- `active -> invalid`: credential load, signing, or identity validation fails; workspaces selecting
  it fail closed while PAT/CLI workspaces continue.
- `active -> absent`: delete an unreferenced managed/imported registration.

Webhook health is independent: `unverified -> verified` after the first correctly signed delivery;
post-signature processing failures produce `failing`; a later valid successful delivery restores
`verified`.

### Workspace automation

- The current connection remains active while PAT/CLI validation, App creation/import, and App
  installation are pending.
- A successful replacement increments the workspace credential generation, revokes old broker
  leases, clears incompatible personal auth, and then exposes the new connection atomically.
- Installation suspension, deletion, permission loss, or registration invalidity updates only
  connections matching both registration ID and installation ID.
- Disconnect removes workspace-owned secrets and leaves the reusable registration untouched.

## Permissions And Security

- Registration list/create/import/delete requires registration-manager authority plus access to the
  initiating workspace. Under the current single-user trust model `default-user` has both; future
  multi-user deployments must replace this provisional rule.
- Workspace connection and personal identity actions require access to that workspace.
- Registration IDs are not secrets. They select a candidate key; HMAC verification is still
  mandatory before any webhook payload is trusted.
- Secret request fields have bounded bodies, are excluded from structured logs and errors, are
  stored only in the encrypted secret store, and are never returned by status/catalog APIs.
- Runtime and token-cache keys include registration ID, registration generation, installation ID,
  workspace ID, and repository scope as applicable. No lookup may fall back to another
  registration or workspace.
- Public base URL validation requires a canonical HTTPS origin with no credentials, query, or
  fragment and rejects loopback, private, link-local, or non-globally-routable DNS results. Kandev
  does not fetch the supplied URL as validation.
- App private keys, client secrets, webhook secrets, personal tokens, and live installation tokens
  never enter executor environments. Only brokered PAT/CLI tokens or repository-restricted
  installation tokens reach the trusted child operation.

## Failure Modes

- A registration create/import failure never replaces workspace auth or exposes submitted secrets.
- A duplicate import directs the user to select the known registration instead of storing another
  copy of its root credentials.
- Callback route/state/registration/workspace mismatches fail closed and consume no unrelated flow.
- An invalid webhook signature performs no delivery claim, health update, or connection mutation.
- Missing App permissions produce capability-specific diagnostics; unrelated capabilities continue
  to work.
- Deleting a registration with any workspace or personal reference returns
  `github_app_registration_in_use` with a non-secret binding count.
- Changing workspace auth while a flow is open makes the stale callback fail without reverting the
  newer connection.

## Persistence Guarantees

Registrations, workspace/personal bindings, credential generations, health, auth flows, and webhook
dedupe survive restart. Installation tokens, App JWTs, CLI-derived tokens, and broker lease plaintext
remain memory-only. Active encrypted-bundle pointers are crash consistent; orphan inactive bundles
are reconciled after restart. Restart rebuilds runtime clients independently for every valid stored
registration and never creates a global default.

## UX And Mobile Contract

- Workspace GitHub settings lead with the active automation identity and a **Change connection**
  command. The method chooser uses a menu/list with PAT, GitHub CLI, and GitHub App descriptions,
  not a segmented tab control.
- GitHub App selection first explains when to use it and the sharing/isolation trade-off, then lists
  known registrations and actions to **Add existing App** or **Create new App**.
- The import guide provides copyable callback, setup, and webhook URLs; required permissions/events;
  exact GitHub settings navigation; bounded secret inputs; validation; and an install handoff.
- The manifest guide asks owner, visibility, display name, and public URL. Visibility help explicitly
  distinguishes installability from Marketplace publication and repository access.
- Permission details use a button and dialog. Current actor, installation account, App label, source,
  visibility, webhook health, and sharing warning are scannable without exposing secrets.
- `My GitHub identity` appears as a connectable section only for App automation. For PAT/CLI it shows
  that personal actions use the same verified human identity and offers no fake selector.
- Desktop and mobile support the same create/import/select/install/switch/disconnect flows. Mobile
  uses a single-column sheet/page, one scroll owner, safe-area padding, 44px targets, no fixed footer,
  and no horizontal overflow. External GitHub navigation is deliberate and returns to the same
  workspace settings route.

## Scenarios

- **GIVEN** two workspaces, **WHEN** each selects a different App registration and installation,
  **THEN** status, tokens, webhooks, repositories, actors, and revocation remain isolated.
- **GIVEN** two workspaces intentionally reuse one registration, **WHEN** each installs it into a
  different account, **THEN** each workspace receives only its own installation and repository
  scope while the UI identifies the shared root App identity.
- **GIVEN** a private managed App, **WHEN** creation completes, **THEN** the UI says it is installable
  only on its owner and does not imply Marketplace publication.
- **GIVEN** a user chooses public, **WHEN** the manifest is submitted, **THEN** `public: true` is sent
  and the confirmation explains that installation approval still controls repository access.
- **GIVEN** a correctly configured existing App, **WHEN** the import is verified, **THEN** it appears
  in the workspace chooser without becoming the active connection until installation succeeds.
- **GIVEN** an imported App misses a required GitHub setting, **WHEN** validation or installation
  runs, **THEN** the guide identifies the exact setting without returning submitted secrets.
- **GIVEN** App creation, import, or installation is canceled, **WHEN** the user returns, **THEN** the
  previous workspace automation connection remains active.
- **GIVEN** an App workspace with personal OAuth, **WHEN** it switches registration or to PAT/CLI,
  **THEN** the incompatible personal connection is removed and its old tokens cannot be resolved.
- **GIVEN** a webhook for registration A, **WHEN** it is sent to registration B's route or signature,
  **THEN** no delivery or workspace state is mutated.
- **GIVEN** a PAT or named CLI workspace, **WHEN** an agent uses the managed credential helper,
  **THEN** it receives that workspace's automation token and the UI does not promise provider-side
  repository narrowing.
- **GIVEN** desktop and mobile viewports, **WHEN** users complete every App flow, **THEN** actions and
  disclosures remain usable without clipping, overlap, or desktop-only capability.

## Success Criteria

- No runtime, callback, webhook, cache, broker, or status path resolves a GitHub App without both
  workspace and registration identity where workspace ownership is required.
- A seeded E2E run proves different Apps for work/personal workspaces and intentional App reuse.
- Secret scans find no PAT, private key, client secret, webhook secret, personal token, refresh
  token, or live installation token in logs, API snapshots, redirects, process arguments, or
  executor environments.

## Out Of Scope

- Multiple automation connections or per-repository credential routing inside one workspace.
- Automatically editing an imported App's GitHub settings or uninstalling an App on disconnect.
- Automatic private-key/client-secret rotation; users replace an unbound stored registration or
  import the replacement App credentials through the guided flow.
- GitHub Enterprise Server, enterprise-owned Apps, or hosts other than `github.com`.
- Kandev multi-user login, workspace membership, or RBAC implementation.
- Publishing Apps to GitHub Marketplace.

## Implementation Plan

See [the implementation plan](../../plans/github-authentication/plan.md).

## Decision

See [ADR-2026-07-21-workspace-selectable-github-app-registrations](../../decisions/2026-07-21-workspace-selectable-github-app-registrations.md).
