---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
created: 2026-07-19
owners:
  - Kandev
---
# Workspace GitHub Authentication System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- Named CLI account discovery accepts a successful `gh auth status` report from either stdout or
  stderr, while preserving non-empty stdout as the authoritative command result.
- User-triggered mutations prefer the workspace's verified personal connection, then a human
  PAT/CLI automation connection, then the App installation. The UI always identifies the effective
  actor and never labels an App mutation as human-attributed.
- Background watches, cleanup, repository discovery, and workflow sync always use the workspace
  automation connection. Personal credentials are never exposed to agents or executors.
- Task Git credential routing is a separate workspace policy. **Managed workspace credentials**
  gives attached GitHub repositories task/repository/generation-bound broker leases from the
  workspace automation connection. **Inherit executor Git credentials** injects no GitHub broker
  helper or `gh` shim: Local and Worktree tasks use host-visible Git/SSH credentials, while remote
  tasks use credentials configured in that executor.
- A task that is explicitly created for a pre-PR fork contribution under managed task credentials
  may carry one server-authored, versioned `contribution_destination` on its canonical repository
  attachment. Managed routing may issue one additional lease for that exact fork. The fork is a
  push destination only: canonical repository identity, `origin`, issue lookup, and pull-request
  targeting remain unchanged. Executor-owned identities are opaque and do not receive a
  workspace-authored destination binding.
- Every newly created workspace attempts to persist **Inherit executor Git credentials** as its
  initial task policy. After a successful settings write, if creation is performed by an internal
  trusted caller, an auth-disabled synthetic administrator, or a real administrator while host
  `gh` has an authenticated active account, Kandev also stores that exact host/login as the new
  workspace's named CLI automation connection. Non-admin member-created workspaces never receive
  the server operator's CLI identity automatically.
- If host `gh` is absent, unauthenticated, or cannot validate its active account, workspace creation
  still succeeds with executor task access and disconnected GitHub automation. If the executor
  settings write fails, creation still succeeds but the existing managed compatibility fallback
  remains until the workspace is configured or retried. Existing workspaces are never migrated to
  these defaults; their connection, persisted policy, and legacy missing/invalid policy fallback
  remain unchanged.
- Workspace settings present the automation identity and task Git credential routing as one
  **Workspace GitHub access** group. The page shows a compact read-only summary of both effective
  choices; the existing **Change GitHub connection** dialog contains the full controls for the
  automation method and task routing policy. The task policy remains independent in behavior and
  persistence even though the related choices share one configuration surface.
- The connection dialog has one **Save changes** action for local PAT or named CLI connection
  drafts and task Git access. Method-specific **Connect token**, **Use account**, and **Save task
  access** actions are not shown. GitHub App create, import, and install remain explicit workflow
  actions because they leave Kandev for GitHub rather than saving a local connection draft.
- For Kandev-managed GitHub checkouts used by Local and Worktree tasks, the selected task policy
  also controls the persisted `origin` transport. Managed routing uses canonical GitHub HTTPS.
  Executor inheritance uses the host's detected `gh` clone protocol, including SSH, and reconciles
  an existing managed checkout when the policy changes. This makes Git conditional includes based
  on `remote.*.url` observe the same transport the task uses. Kandev never rewrites the remote of a
  repository registered as a user-managed local checkout.
- Repository preparation resolves each attached repository once per launch or resume and reuses
  that result for primary-repository configuration, multi-repository configuration, and credential
  routing. Origin reconciliation is serialized per managed checkout, compares the current and
  desired canonical URLs, and performs no write when they already match.
- Repository preparation validates any contribution destination before issuing credentials, adds a
  collision-resistant dedicated fork remote, and reconstructs it on launch and resume. It never
  accepts a fork inferred from the checkout's current remotes or a caller-provided repository name.
- Git failures while inspecting or reconciling a managed checkout preserve a bounded,
  credential-redacted diagnostic. Git's dubious-ownership failure is classified as a service/data
  ownership mismatch with guidance to restore the intended Kandev service account or reconcile the
  managed data owner. Kandev does not bypass Git's ownership protection with a broad
  `safe.directory` entry.
- Under managed routing, App installation tokens are minted for the requested repository and cached
  only in memory. PAT/CLI tokens retain their provider-granted scope once delivered to a trusted
  agent subprocess. GitHub HTTPS and the broker-aware `gh` shim fail closed rather than consulting
  another ambient helper after a managed-helper failure.
- A managed pre-PR fork destination must be writable by the same workspace automation source that
  supplies the task leases. Human PAT and named CLI automation actors may own or create their fork.
  An App installation without direct target write access is not silently paired with a user's
  personal fork or personal token.
- Managed Git helper execution does not depend on the post-startup `PATH`: Git resolves an
  absolute Kandev-owned `agentctl` executable published before the first managed Git operation.
  Local and Worktree preparation binds the helper to the standalone launcher's absolute executable
  before checkout or setup scripts run. Remote preparation binds it to the installed executor
  binary before cloning, and a running `agentctl` publishes its own executable for child processes.
  Non-interactive Unix login shells that replace their inherited `PATH` restore the managed
  CLI-shim directory after profile initialization for broker-enabled tasks, while preserving
  pre-existing Bash environment hooks, including hook paths containing `$VAR` or `${VAR}`
  references from the effective child environment. Broker-disabled and executor-inheritance
  processes receive no shell hook or managed-tool path.
- Under managed routing, every authorized task execution surface receives the same current
  task-scoped Git environment: the agent subprocess, terminal shells, passthrough-agent PTYs, and
  task-scoped command processes. This includes the broker contract, managed indexed Git
  configuration, and the `agentctl`/`gh` shim-first `PATH`; it does not grant access to a browser
  client, an unrelated host shell, or another workspace's task environment.
- Kandev composes the indexed `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_<n>` /
  `GIT_CONFIG_VALUE_<n>` protocol across host, executor, profile, task, and agentctl boundaries.
  Unrelated entries such as hooks, notes, safe-directory, and URL-rewrite settings survive in their
  original order; later Kandev credential entries affect only their intended GitHub credential
  keys. An already-forwarded suffix is not duplicated.
- Explicit executor-profile `GITHUB_TOKEN` or `GH_TOKEN` values remain unmanaged operator overrides
  and take precedence over the workspace broker when managed routing is selected.
- Every successful task launch or resume records a non-secret session snapshot of the selected
  policy, effective credential source, known method and actor, transport, executor, and capture
  time. Inherited and profile-token actors are labeled runtime-selected instead of being probed.
- The unpublished `KANDEV_GITHUB_APP_*` configuration introduced on this branch is removed. Setting
  those variables does not create or configure a registration; operators use the guided import
  flow for an App they already own.
- Existing released workspaces migrate to `legacy_shared`; upgrades do not rewrite their
  connection or task policy. New operator-authorized workspaces use the active authenticated host
  `gh` account when one can be validated, while member-created workspaces and workspaces created
  without usable host `gh` remain disconnected. Once a workspace leaves legacy mode it cannot
  return. The unpublished singleton registration
  schema on this branch is rewritten directly and receives no compatibility migration. A valid
  legacy connection supplies both the existing API client abstraction and an in-memory Git
  transport credential so provider-backed repositories can be rematerialized from legacy shared
  managed paths into workspace-isolated clone roots.
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

The automation method and task Git credential policy are independent. Selecting a named `gh`
account does not inherit the host's Git configuration: in managed mode Kandev resolves that exact
host/login and brokers its token to the task. Users who want traditional host or executor behavior
select **Inherit executor Git credentials** explicitly.

## Identity And Routing

| Purpose | First choice | Fallback | Attribution |
| --- | --- | --- | --- |
| Background reads and writes | Workspace automation | None | Automation principal |
| Managed task git and `gh` access | Workspace automation | Explicit profile token | Automation principal, or runtime-selected override |
| Inherited task Git access | Executor-visible Git/SSH credentials | None | Runtime-selected |
| `My GitHub` reads | Personal connection | Human PAT/CLI automation | Human principal |
| User-triggered mutation | Personal connection | Human PAT/CLI, then App installation | Effective principal shown in UI |

An App-only workspace without personal OAuth remains usable for automation and App-attributed
mutations. `My GitHub` instead offers a personal connection created with the same App registration
as the workspace installation.

## GitHub App Policy

Kandev-created Apps request repository metadata read; contents read/write; pull requests read/write;
issues read/write; checks, statuses, and Actions read; administration read; organization members
read; and workflows write. The UI exposes this policy through a permissions button and dialog, not
a row of chips. The App subscribes to the configurable `push` and `check_run` events. GitHub sends
`installation`, `installation_repositories`, and `github_app_authorization` lifecycle events
automatically, so they are handled but are not part of the requested event policy.

An imported App must meet the same callback, OAuth-on-install, webhook, event, and permission
requirements. Kandev validates the App identity and reports missing capabilities after installation.
It does not silently change an imported App's GitHub settings. The guide provides exact values and
GitHub links for the user to apply.

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

### `github_workspace_settings`

The non-secret operational settings row adds `task_git_credentials_mode`, with allowed values:

| Value | Behavior |
| --- | --- |
| `managed` | Inject the workspace broker contract for attached GitHub repositories unless an explicit executor-profile token overrides it. Existing missing/invalid values continue to normalize here for upgrade compatibility. |
| `executor` | Default persisted for newly created workspaces. Inject no Kandev GitHub helper or `gh` shim; use credentials available where the selected executor runs. |

Missing or invalid persisted values normalize to `managed`. Workspace-settings copy includes this
policy because it is operational configuration, not authentication material.

### Task-session credential snapshot

After a successful launch or resume, task-session metadata contains a versioned
`git_credential_snapshot` with:

- selected policy (`managed` or `executor`);
- effective source (`workspace`, `executor_profile`, or `executor`);
- workspace method when known (`pat`, `gh_cli`, `github_app_installation`, or
  `legacy_shared`);
- a known human/App actor label, or `runtime_selected` when Kandev does not inspect the credential;
- transport (`managed_https`, `profile_token`, or `executor_selected`), executor type, and capture
  timestamp.

The snapshot contains no token, lease, helper path, credential-file path, or SSH key detail. A
failed launch/resume does not replace the last successful snapshot.

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
  `permissions`, `events`, and `expires_at` so the user can configure the existing App before
  submitting any root credentials. The legacy `setup_url` response field is retained for client
  compatibility and is always empty because OAuth-on-install disables GitHub's Setup URL.
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
- `GET /api/v1/github/workspace-settings?workspace_id=<id>` returns repository scope, saved
  preferences, and `task_git_credentials_mode`.
- `PUT /api/v1/github/workspace-settings` accepts a partial
  `task_git_credentials_mode` update and rejects unknown values without changing the prior policy.
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
