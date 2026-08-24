---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
created: 2026-07-19
owners:
  - Kandev
---
# Workspace GitHub Authentication System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- Managed admission resolves the task policy and the selected executor profile before it validates
  repository identity. An explicit profile `GITHUB_TOKEN` or `GH_TOKEN` bypasses managed broker
  admission because the profile token is the effective source.
- Managed admission completes before Kandev creates, rebinds, resumes, or changes a session, and
  before a workflow transition changes the current step. A policy error or an invalid repository
  identity leaves the task, workflow, and session state unchanged.
- Each attached repository is resolved once for an individual launch or resume. The resolved
  repository set is shared by task configuration and credential-snapshot construction rather than
  triggering repeated materialization or origin mutation.
- Broker issuance and redemption use the same persisted repository identity fields. A local Git
  checkout and its `origin` are materialization data, not authorization data.
- A public `github.com` remote can supply an in-memory origin when persisted provider-host metadata
  is absent. Kandev does not rewrite repository provider identity from partial legacy metadata.
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
- Repository identity accepts credential-free HTTPS and SSH remote forms. Authorities must remain
  ASCII-only and percent-free before URL parsing or case folding. It rejects an SSH password,
  query, or fragment before canonicalization, and an error never contains the rejected
  secret-bearing remote.
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
- The broker accepts a fork owner/repository only when it exactly matches a valid
  `contribution_destination` or `remote_contribution` on the same authorized task-repository
  attachment. Unknown binding versions, malformed URLs, cross-workspace rows, unrelated forks, and
  target mismatches fail closed.
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
- If a contribution task cannot prove direct target write access or prepare an exact fork writable
  by the workspace automation connection, task creation fails before launch. A same-name repository
  that is not a fork of the canonical target is reported as a conflict and is never overwritten.
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
