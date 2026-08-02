---
status: building
created: 2026-07-19
amended: 2026-08-02
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
- Background watches, cleanup, repository discovery, and workflow sync always use the workspace
  automation connection. Personal credentials are never exposed to agents or executors.
- Task Git credential routing is a separate workspace policy. **Managed workspace credentials**
  gives attached GitHub repositories task/repository/generation-bound broker leases from the
  workspace automation connection. **Inherit executor Git credentials** injects no GitHub broker
  helper or `gh` shim: Local and Worktree tasks use host-visible Git/SSH credentials, while remote
  tasks use credentials configured in that executor.
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
- Git failures while inspecting or reconciling a managed checkout preserve a bounded,
  credential-redacted diagnostic. Git's dubious-ownership failure is classified as a service/data
  ownership mismatch with guidance to restore the intended Kandev service account or reconcile the
  managed data owner. Kandev does not bypass Git's ownership protection with a broad
  `safe.directory` entry.
- Under managed routing, App installation tokens are minted for the requested repository and cached
  only in memory. PAT/CLI tokens retain their provider-granted scope once delivered to a trusted
  agent subprocess. GitHub HTTPS and the broker-aware `gh` shim fail closed rather than consulting
  another ambient helper after a managed-helper failure.
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

### Task Git credential resolution

- Missing settings start at `managed`.
- New workspace creation attempts to persist `executor` before the workspace is exposed for task
  launch; a persistence failure leaves the existing managed compatibility fallback in place.
- Operator-authorized creation snapshots a validated active host `gh` host/login as a named CLI
  connection when available; member creation and unavailable/invalid host CLI leave automation
  disconnected.
- `managed + explicit profile GITHUB_TOKEN/GH_TOKEN -> executor_profile`.
- `managed + attached GitHub repository + active workspace connection -> workspace broker`.
- `executor -> executor-visible credentials`, regardless of PAT/CLI/App automation method.
- Initial launch and resume run the same resolution. A successful operation replaces the session
  snapshot; a failed operation leaves the previous snapshot unchanged.
- Each attached repository is resolved once for an individual launch or resume. The resolved
  repository set is shared by task configuration and credential-snapshot construction rather than
  triggering repeated materialization or origin mutation.
- Changing the workspace policy affects new launches and the next resume, not an already-running
  process.

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
- Named GitHub CLI bearer tokens remain memory-only and are re-resolved from the exact selected
  host/login after at most five minutes. Connection-generation invalidation remains immediate.
- Public base URL validation requires a canonical HTTPS origin with no credentials, query, or
  fragment and rejects loopback, private, link-local, or non-globally-routable DNS results. Kandev
  does not fetch the supplied URL as validation.
- App private keys, client secrets, webhook secrets, personal tokens, and live installation tokens
  never enter executor environments. Only brokered PAT/CLI tokens or repository-restricted
  installation tokens reach a managed trusted child operation; explicit executor-profile
  `GITHUB_TOKEN`/`GH_TOKEN` values are an unmanaged exception and can reach that child instead.
- Managed helper configuration resets the inherited GitHub HTTPS helper chain, disables terminal
  prompts, and activates Kandev's `agentctl`/`gh` tool directory only for broker-enabled task
  instances. It does not claim to prevent a host-authority agent from manually switching a remote
  to SSH or invoking another credential-bearing tool.
- The helper command uses a Kandev-owned absolute executable variable rather than searching ambient
  `PATH`. The variable is bound before executor preparation and refreshed from `os.Executable` by
  a running `agentctl`. Unix shell-startup restoration composes with an inherited `BASH_ENV`,
  resolves simple environment-variable references in that hook path from the effective child
  environment, remains conditional on the broker contract, and never places broker leases, tokens,
  or credential scopes in shell arguments or startup files.
- The effective task Git environment is runtime-only. It is copied only after the existing
  task/session or task-environment ownership check, is never persisted in task metadata or terminal
  records, and is never written to logs, errors, browser payloads, or process arguments.
- Indexed Git configuration is validated and composed as a single ordered block at environment
  merge boundaries. Kandev never replaces a complete inherited block merely by assigning its own
  `GIT_CONFIG_COUNT`; managed helper reset semantics are expressed as later Git config entries.

## Failure Modes

- A registration create/import failure never replaces workspace auth or exposes submitted secrets.
- A duplicate import directs the user to select the known registration instead of storing another
  copy of its root credentials.
- Callback route/state/registration/workspace mismatches fail closed and consume no unrelated flow.
- An invalid webhook signature performs no delivery claim, health update, or connection mutation.
- Missing App permissions produce capability-specific diagnostics; unrelated capabilities continue
  to work.
- GitHub CLI account discovery and token resolution tolerate host CLI releases both before and
  after structured status and multi-account flags. Genuine discovery failures are shown as errors,
  not as an empty account list. A CLI without named-token support may resolve only the active
  account; selecting another stored login fails with guidance to activate it or upgrade the CLI.
- If the managed `agentctl` helper, broker, or `gh` shim is unavailable, the command fails with a
  managed-credential error and does not fall through to another HTTPS helper or interactive prompt.
- If an authorized task terminal, passthrough PTY, or task-scoped command cannot receive its
  effective managed Git environment, Kandev fails that process start before it runs the requested
  command. It does not silently fall back to an ambient credential helper or host `gh` login.
- If any environment source supplies a malformed or unreasonably large indexed Git configuration,
  task environment preparation fails with a sanitized configuration error rather than silently
  truncating, partially merging, or executing a different block.
- If executor inheritance is selected but no usable credential exists in that executor, Git/SSH
  reports its normal authentication failure. Kandev does not probe or guess the actor.
- If host `gh` is absent, unauthenticated, or fails active-account validation while a workspace is
  created, the workspace remains disconnected for automation, retains executor task access, and
  creation succeeds.
- If executor-default persistence fails while a workspace is created, the workspace remains
  available with the existing managed task-access fallback and a warning for retry/configuration;
  Kandev does not claim executor inheritance was applied.
- If Kandev cannot reconcile a managed checkout's `origin` with the selected task policy, Local and
  Worktree preparation fails before the agent starts instead of silently using the other policy's
  transport.
- If Git rejects a managed checkout because its filesystem owner differs from the Kandev service
  account, preparation fails with an ownership-specific, credential-safe diagnostic. Kandev does
  not retry with a global trust override, suppress the Git check, or mutate filesystem ownership.
- Generic origin-inspection and origin-update failures include only bounded, credential-redacted
  Git output. Credentials embedded in URL userinfo or known authentication material never appear
  in logs, session errors, or browser payloads.
- Deleting a registration with any workspace or personal reference returns
  `github_app_registration_in_use` with a non-secret binding count.
- Changing workspace auth while a flow is open makes the stale callback fail without reverting the
  newer connection.
- A PAT replacement is validated against GitHub before it replaces the current workspace
  connection. An invalid PAT leaves the previous connection unchanged, keeps the submitted draft
  available for correction, and shows the validation error in the connection dialog.
- If a previously valid GitHub credential expires or is revoked, My GitHub stays on the current
  route and renders a reconnect/loading error instead of treating GitHub's 401 as an expired Kandev
  login session.

## Persistence Guarantees

Registrations, workspace/personal bindings, task Git credential policy, credential generations,
health, auth flows, webhook dedupe, and successful task-session credential snapshots survive
restart. Installation tokens, App JWTs, CLI-derived tokens, and broker lease plaintext remain
memory-only. Active encrypted-bundle pointers are crash consistent; orphan inactive bundles are
reconciled after restart. Restart rebuilds runtime clients independently for every valid stored
registration and never creates a global default.

## UX And Mobile Contract

- Workspace GitHub settings lead with the active automation identity and a **Change connection**
  command. The method chooser presents GitHub CLI first, followed by PAT and GitHub App, with
  descriptions rather than a segmented tab control.
- Method descriptions state where the credential is stored/resolved and how managed tasks receive
  it. The same access group shows a compact **Task access** summary and the **Change GitHub
  connection** dialog visibly explains and edits **Managed workspace credentials** and **Inherit
  executor Git credentials**, including local/Worktree versus remote behavior and explicit
  profile-token precedence. The page does not repeat those controls in a standalone settings
  section.
- The Task Git access heading exposes supplementary help that explains the managed Git credential
  helper, repository-scoped broker lease redemption, broker-aware `gh` shim, executor inheritance,
  and when a changed policy takes effect. The visible option descriptions remain plain-language
  decision guidance and use the same explanatory typography as the rest of the dialog.
- One **Save changes** submission persists every changed local draft in the dialog: a PAT or named
  CLI workspace connection and the task Git access policy. It reports success only after all
  changed drafts save, keeps failed drafts available for retry, and never implies that one setting
  silently determines the other. The action stays visible in a fixed bottom row while the content
  above owns scrolling; a bottom fade cues additional content. App create/import/install remain
  separate workflow actions.
- Task Git access options use compact spacing while retaining their full descriptions and minimum
  touch targets.
- GitHub App selection first explains when to use it and the sharing/isolation trade-off, then lists
  known registrations and actions to **Add existing App** or **Create new App**.
- The import guide provides copyable callback, setup, and webhook URLs; required permissions/events;
  exact GitHub settings navigation; bounded secret inputs; validation; and an install handoff.
- The manifest guide asks owner, visibility, display name, and public URL. Visibility help explicitly
  distinguishes installability from Marketplace publication and repository access.
- Permission details use a button and dialog. Current actor, installation account, App label, source,
  visibility, webhook health, and sharing warning are scannable without exposing secrets.
- `My GitHub identity` appears as a connectable section only for App automation. For PAT/CLI,
  Workspace GitHub access states that My GitHub and user-triggered actions use the same verified
  human identity; the page does not render a redundant identity section or a fake selector.
- The workspace identity and task-access summary lines expose concise help through a tooltip on
  hover or keyboard focus and the same explanation in a 44px-target drawer on touch devices.
- Refreshing an already loaded workspace GitHub status keeps the current identity, task-access
  summary, and actions visible while the request is in flight. The refresh control alone shows
  progress and prevents duplicate activation. A workspace whose status has not loaded yet still
  shows the initial connection-status placeholder and never inherits another workspace's data.
- When rate-limit snapshots are available, the connection status row exposes a **Show GitHub API
  limits** icon. Its desktop tooltip and touch drawer show remaining and total API requests,
  GraphQL query points, Search requests, and reset timing. Exhausted buckets explain that
  background PR and issue checks are paused.
- Desktop and mobile support the same create/import/select/install/switch/disconnect flows. Mobile
  uses a single-column sheet/page, one scroll owner, safe-area padding, 44px targets, no fixed footer,
  and no horizontal overflow. External GitHub navigation is deliberate and returns to the same
  workspace settings route.
- The desktop connection dialog is wide enough for the three method cards and explanatory content
  without cramped columns. Mobile keeps the existing full-height single-column drawer; its one
  **Save changes** action remains inside the scroll owner and is at least 44px tall.
- The Changes panel branch disclosure includes the active session's launch-time Git credential
  snapshot. Desktop supports hover and keyboard focus; coarse-pointer/mobile users open the same
  information in a 44px-target drawer. Unknown inherited/profile actors are explicitly labeled
  runtime-selected.

## Scenarios

- **GIVEN** a brand-new Kandev database and an authenticated active host `gh` account, **WHEN** the
  initial workspace is seeded, **THEN** its automation connection records that exact CLI
  host/login, its task access is executor inheritance, and its first task does not request a
  managed credential lease.
- **GIVEN** an administrator or internal trusted caller and an authenticated active host `gh`
  account, **WHEN** a workspace is created, **THEN** the workspace records that exact named CLI
  automation connection and executor task access.
- **GIVEN** a non-admin member and an authenticated server-operator `gh` account, **WHEN** the
  member creates a workspace, **THEN** the workspace has executor task access but no automatic
  GitHub automation connection.
- **GIVEN** no usable authenticated host `gh` account, **WHEN** an authorized caller creates a
  workspace, **THEN** workspace creation succeeds, task access is executor inheritance, and
  GitHub automation remains disconnected.
- **GIVEN** an existing installation with managed, disconnected, or legacy missing workspace
  settings, **WHEN** Kandev upgrades, **THEN** those connections and task policies remain
  unchanged.

- **GIVEN** a workspace GitHub status is visible, **WHEN** the user refreshes it and the status
  request is still pending, **THEN** the existing workspace content remains visible and usable
  while the refresh control is busy, on both desktop and mobile.
- **GIVEN** the user navigates to a workspace with no loaded GitHub status, **WHEN** its initial
  status request is pending, **THEN** the UI shows the connection-status placeholder and no status
  from the previous workspace.
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
- **GIVEN** a named CLI workspace in managed mode, **WHEN** a task launches, **THEN** Kandev resolves
  the selected host/login, makes both managed `git` and `gh` available in standalone and remote
  runtimes, and does not depend on the host's currently active CLI account.
- **GIVEN** an authorized user opens a terminal, uses a passthrough-agent PTY, or starts a
  task-scoped command in a managed task, **WHEN** it accesses an attached GitHub repository,
  **THEN** it receives the same task-scoped broker helper and `gh` shim environment as the agent
  subprocess.
- **GIVEN** an unauthorised user or a terminal for another task environment, **WHEN** it attempts to
  open a terminal or start a process, **THEN** it cannot receive the managed task Git environment.
- **GIVEN** a workspace selects executor inheritance, **WHEN** a Local/Worktree or remote task
  launches, **THEN** Kandev injects no broker helper/shim and the task uses host-visible or
  executor-configured credentials respectively.
- **GIVEN** a configured workspace, **WHEN** the user views Workspace GitHub access, **THEN** one
  compact summary identifies both the workspace automation identity and the effective task access
  mode without rendering a separate Task Git credentials settings section.
- **GIVEN** a PAT or named CLI workspace, **WHEN** the user views Workspace GitHub access, **THEN**
  the page states that the same account powers My GitHub and user-triggered actions without
  rendering a separate My GitHub identity section, and accessible help explains both the workspace
  identity and task-access summary.
- **GIVEN** a workspace status with GitHub rate-limit snapshots, **WHEN** the user hovers, focuses,
  or taps **Show GitHub API limits**, **THEN** the disclosure shows the remaining and total API,
  GraphQL query, and Search quotas with reset timing for that workspace connection.
- **GIVEN** a PAT or named CLI connection draft and a changed task access mode, **WHEN** the user
  presses the dialog's single **Save changes** action, **THEN** both drafts are persisted, the dialog
  closes only after both succeed, and reopening shows the selected account and task mode.
- **GIVEN** a user enters an invalid replacement PAT, **WHEN** GitHub rejects it during **Save
  changes**, **THEN** the dialog remains open, the submitted PAT remains available for correction,
  an error is shown, and the previously active workspace connection is unchanged.
- **GIVEN** a configured PAT has expired or been revoked, **WHEN** the user opens My GitHub and the
  provider data request returns 401, **THEN** the page remains on `/github`, shows an authentication
  loading error, and does not navigate to the Kandev login screen.
- **GIVEN** a changed task access mode but no PAT or CLI connection change, **WHEN** the user presses
  **Save changes**, **THEN** only the task policy is persisted and the selected automation identity
  remains unchanged.
- **GIVEN** managed task access, **WHEN** the user hovers, focuses, or taps the Task Git access help,
  **THEN** it explains that Git calls Kandev's `agentctl` credential helper for attached GitHub
  HTTPS repositories, the helper redeems a scoped broker lease on demand, `gh` uses a broker-aware
  shim, and credentials are not written into the repository.
- **GIVEN** the host `gh` clone protocol is SSH and a Kandev-managed GitHub checkout currently has
  an HTTPS `origin`, **WHEN** the workspace selects executor inheritance and launches a Local or
  Worktree task, **THEN** Kandev changes that managed checkout's `origin` to the canonical SSH URL
  before task preparation so matching Git conditional includes apply.
- **GIVEN** a Kandev-managed GitHub checkout currently has an SSH `origin`, **WHEN** the workspace
  selects managed credentials and launches a Local or Worktree task, **THEN** Kandev changes that
  managed checkout's `origin` to canonical HTTPS before task preparation.
- **GIVEN** a Kandev-managed checkout already has the canonical origin selected by the task policy,
  **WHEN** a task launches or resumes, **THEN** Kandev inspects the origin but does not rewrite
  `.git/config`.
- **GIVEN** a task with one or more attached repositories, **WHEN** launch or resume builds the
  primary, multi-repository, and credential configuration, **THEN** each repository is prepared
  once and the same resolved result is reused by all three consumers.
- **GIVEN** a managed checkout is owned by `brewuser` while the Kandev service runs as root,
  **WHEN** Git rejects origin inspection or reconciliation as dubious ownership, **THEN** task
  preparation stops and reports that the service account and managed repository owner disagree,
  without suggesting `safe.directory=*`.
- **GIVEN** Git emits a failure containing credential-bearing URL userinfo, **WHEN** the failure is
  returned through task preparation, **THEN** the diagnostic retains actionable Git context but
  redacts the credential and bounds the output length.
- **GIVEN** a repository is registered from a user-managed local checkout, **WHEN** either task Git
  credential policy is selected, **THEN** Kandev leaves its configured `origin` unchanged.
- **GIVEN** managed mode and an explicit executor-profile GitHub token, **WHEN** a task launches,
  **THEN** the profile token wins and the session disclosure labels its actor runtime-selected.
- **GIVEN** a managed helper cannot execute or redeem its lease, **WHEN** Git requests GitHub HTTPS
  credentials, **THEN** the command fails without falling through to a personal helper or prompt.
- **GIVEN** a broker-enabled managed task whose login profile replaces its inherited `PATH`,
  **WHEN** Git requests GitHub HTTPS credentials, **THEN** the configured helper invokes the
  instance-owned `agentctl` directly and does not search or fall through to an ambient helper.
- **GIVEN** a broker-enabled Local or Worktree task whose checkout or setup script invokes Git
  before the task instance is created, **WHEN** Git requests GitHub HTTPS credentials, **THEN** the
  configured helper invokes the standalone launcher's absolute `agentctl` executable without
  consulting `PATH` or an ambient helper.
- **GIVEN** a broker-enabled Docker or Sprites task whose prepare script clones before `agentctl`
  starts, **WHEN** Git requests GitHub HTTPS credentials during that clone, **THEN** the configured
  helper invokes the already-installed absolute executor binary and redeems the task lease without
  consulting `PATH` or an ambient helper.
- **GIVEN** a broker-enabled managed task with an existing Bash environment hook, **WHEN** a
  non-interactive login shell replaces `PATH`, **THEN** the existing hook still runs and the
  Kandev-managed `agentctl` and `gh` shims are restored ahead of ambient tools before the requested
  command starts.
- **GIVEN** that existing Bash environment hook is expressed as `$HOME/hook.sh` or
  `${KANDEV_HOOK_ROOT}/hook.sh`, **WHEN** Kandev composes its managed startup fragment, **THEN** it
  resolves the reference from the effective child environment and sources the intended hook rather
  than a filename containing literal dollar-sign text.
- **GIVEN** the host or executor exports indexed Git config for `core.hooksPath` and
  `notes.augment.mergeStrategy`, **WHEN** Kandev appends its managed GitHub helper configuration,
  **THEN** the agent receives one contiguous block containing the original entries first and the
  Kandev entries afterward, and a real Git commit still runs the configured hook.
- **GIVEN** Docker or a remote control process already contains the same task Git config suffix,
  **WHEN** that suffix is forwarded again while creating or configuring an agent instance,
  **THEN** Kandev emits it once while retaining executor-added `safe.directory` and URL rewrites.
- **GIVEN** a workspace policy or automation connection changes after launch, **WHEN** the user
  views the running session, **THEN** the Changes disclosure still shows its launch snapshot; a
  successful resume records and shows the newly resolved contract.
- **GIVEN** an authenticated host GitHub CLI without structured status or named-token flags,
  **WHEN** an operator selects its sole account, **THEN** Kandev discovers and validates that
  account without requiring a CLI upgrade.
- **GIVEN** a valid migrated `legacy_shared` connection and a legacy shared managed checkout,
  **WHEN** a task needs a workspace-isolated checkout, **THEN** Kandev resolves the same automation
  identity's Git credential and clones into that workspace's managed root without persisting or
  exposing the token.
- **GIVEN** desktop and mobile viewports, **WHEN** users complete every App flow, **THEN** actions and
  disclosures remain usable without clipping, overlap, or desktop-only capability.
- **GIVEN** a mobile coarse-pointer viewport, **WHEN** the user opens Change GitHub connection,
  **THEN** the task access controls share the existing full-height drawer's single scroll owner,
  remain touch reachable, clear the safe area, and introduce no horizontal overflow.
- **GIVEN** desktop fine-pointer and mobile coarse-pointer task views, **WHEN** the branch
  disclosure is opened, **THEN** both show the same credential policy, method, actor truth, and
  transport without horizontal overflow.

## Success Criteria

- No runtime, callback, webhook, cache, broker, or status path resolves a GitHub App without both
  workspace and registration identity where workspace ownership is required.
- A seeded E2E run proves different Apps for work/personal workspaces and intentional App reuse.
- Secret scans find no PAT, private key, client secret, webhook secret, personal token, refresh
  token, or live installation token in logs, API snapshots, redirects, process arguments, or
  executor environments.
- Standalone, container, and remote task tests prove the managed helper is discoverable only for
  broker-enabled instances and their authorized task terminals/processes, while executor
  inheritance receives no Kandev GitHub helper/shim.
- A real Git subprocess test proves that host/executor indexed hooks and notes config survive
  managed credential injection, and focused tests prove ordered composition and overlap handling
  across standalone, container, and remote launch shapes.

## Out Of Scope

- Multiple automation connections or per-repository credential routing inside one workspace.
- Automatically editing an imported App's GitHub settings or uninstalling an App on disconnect.
- Automatic private-key/client-secret rotation; users replace an unbound stored registration or
  import the replacement App credentials through the guided flow.
- GitHub Enterprise Server, enterprise-owned Apps, or hosts other than `github.com`.
- Kandev multi-user login, workspace membership, or RBAC implementation.
- Publishing Apps to GitHub Marketplace.
- Discovering or verifying the actor behind inherited credential managers, SSH agents, or explicit
  profile tokens.
- Preventing a host-authority agent from manually selecting another Git transport outside
  Kandev-injected managed HTTPS and `gh` commands.

## Implementation Plan

See [the original authentication implementation plan](../../plans/github-authentication/plan.md)
and the
[task Git credential policy follow-up plan](../../plans/task-git-credential-policy/plan.md), plus
the
[executor clone transport repair plan](../../plans/github-executor-clone-transport/plan.md), and
the [managed task terminal environment plan](../../plans/task-terminal-git-environment/plan.md),
and the
[managed GitHub login-shell repair plan](../../plans/github-managed-tools-login-shell/plan.md),
and the
[system-service identity guardrails repair plan](../../plans/system-service-identity-guardrails/plan.md).
The new-workspace default repair is tracked in
[the new workspace GitHub access defaults plan](../../plans/new-workspace-github-access-defaults/plan.md).

## Decision

See [ADR-2026-07-21-workspace-selectable-github-app-registrations](../../decisions/2026-07-21-workspace-selectable-github-app-registrations.md)
and
[ADR-2026-07-27-task-git-credential-policy](../../decisions/2026-07-27-task-git-credential-policy.md).
New-workspace defaults are defined by
[ADR-2026-08-02-new-workspace-github-access-defaults](../../decisions/2026-08-02-new-workspace-github-access-defaults.md).
The system-service ownership boundary is defined by
[ADR-2026-07-31-system-service-user-continuity](../../decisions/2026-07-31-system-service-user-continuity.md).
