---
status: approved
created: 2026-08-15
owner: kandev
---

# Redmine Connector Plugin

## Why

Teams whose planning lives in a self-hosted or vendor-hosted Redmine instance cannot
connect it to Kandev today. They have no way to browse Redmine issues, link a Kandev
task to an issue, keep the two in step, or have Redmine issues turn into Kandev tasks
automatically. Every other issue tracker Kandev supports (Jira, Linear, Sentry,
GitLab, Azure DevOps) is a native `internal/<name>` integration, but Redmine is
deliberately **not** — as of 2026-08-15, `kdlbs/kandev` PR #2117 landed the generic
plugin host seams (authenticated manifest actions, `registerIntegrationSettings`,
`registerTaskAction({placement:"link"})` + `host.openTaskLinkDialog`, dynamic composer
`reference_sources`, and `PluginOwnedTaskTrees` cascade delete) that previously forced
every issue-tracker integration down the native path. Redmine is the first issue
tracker built as a Kandev plugin, proving that path the same way
`kdlbs/kandev-plugin-bitbucket` proved it for source-control providers.

An earlier native `internal/redmine` implementation was built and fully passing
(backend + frontend + E2E) before this redirection; it is preserved for reference on
the `archive/redmine-native-implementation` branch. Its acceptance criteria and
field-mapping/echo-suppression design carry over conceptually to this plugin, but no
code from it lands in `kdlbs/kandev` — per the plugin model, all Redmine-specific
logic lives in the plugin repository.

## What

- **Repository.** `yattdev/kandev-plugin-redmine`, bootstrapped from
  `kdlbs/kandev-plugin-template`. All Redmine API knowledge, credentials, and
  Redmine-shaped persistence live there. The host stays provider-neutral: `kdlbs/kandev`
  gains no Redmine-specific code, only this spec/plan, a contract E2E suite, and (once
  a release exists) a `plugin-registry/plugins.yaml` catalog pointer.
- **No new host contracts required.** Unlike Bitbucket, which needed PR #2117's seams
  built alongside it, those seams already exist on `main` by the time this plugin is
  built. This spec's "API and host contracts" section names which existing seams the
  plugin uses; none are new.
- **Connection.** API-key auth only (`X-Redmine-API-Key`), entered directly (no OAuth
  redirect — Redmine core has none). One connection per Kandev workspace. Validated via
  `GET /users/current.json` before being accepted.
- **Workspace-scoped secrets on a flat namespace.** The host's plugin secret RPCs are
  keyed only by plugin ID (`plugin:<id>:secret:<key>`), not by workspace. The plugin
  composes the workspace ID into the secret key itself and encrypts the API key with
  workspace-derived key material before storing it, mirroring the pattern
  `kdlbs/kandev-plugin-bitbucket` uses for its own credential isolation. Non-secret
  connection metadata (base URL, health state, cursor) lives in host `plugin_state`
  scoped to `workspace`.
- **Own health polling.** There is no host `healthpoll` equivalent for plugins. The
  plugin runs its own ~90s-interval, jittered health probe against
  `GET /users/current.json` per connected workspace, the same convention
  `kandev-plugin-bitbucket` uses, and records `last_ok` / `last_error` in
  `plugin_state`.
- **Project selection.** `GET /projects.json`, paginated via `offset`/`limit` (100 cap)
  driven by `total_count`. The user selects which project(s) sync.
- **Field mapping.** `GET /issue_statuses.json`, `/trackers.json`,
  `/enumerations/issue_priorities.json` fetched live and surfaced in the settings UI.
  No status, tracker, or priority name is ever hardcoded in the plugin. Status maps to
  a Kandev workflow step (with an `isClosed` flag per status); tracker maps to a Kandev
  task label; priority maps to a Kandev task priority (`critical|high|medium|low`).
  Custom fields are listed from `/custom_fields.json` when the key is admin, or derived
  from the union of `custom_fields` observed on fetched issues (with a "derived from
  recent issues" note in the UI) when that endpoint 403s.
- **Issue read/write.** `GET /issues.json` (always with `status_id=*` — Redmine
  defaults to open-only, which would silently drop closed-issue updates) and
  `GET /issues/:id.json?include=journals,attachments,relations` for detail.
  `POST /issues.json` / `PUT /issues/:id.json` for create/update, with attachments via
  the two-step `POST /uploads.json` token flow.
- **Task linking via the shared host dialog.** A Kandev task links to a Redmine issue
  through `registerTaskAction({placement:"link"})` and `host.openTaskLinkDialog` — the
  same shared native Link surface GitHub/Bitbucket use — not a plugin-drawn modal.
  Linking writes issue id/url onto the task via `Tasks().Update`.
- **Bidirectional sync.** Polling with a stored `updated_on` cursor per connection
  (1-second overlap subtraction — Redmine's `updated_on` has only second granularity),
  `status_id=*` always sent. Manual write-back by default; two per-connection opt-in
  toggles, `autoStatusWriteback` and `syncTitleDescription`, enable automatic outbound
  PRs. Echo suppression via recorded `last_pushed_status_id` /
  `last_pushed_title` / `last_pushed_description_hash` compared against inbound
  observations before applying, so a write-back round-trip never bounces the task.
  Inbound status changes move the task via the same transition path a manual kanban
  drag uses (not a raw column write), so `on_exit`/`on_enter` hooks and WS broadcast
  fire normally.
- **Composer mentions.** `#` search resolves Redmine issues through the plugin's
  `reference_sources` registration with submit-time reauthorization, matching the
  pattern `kandev-plugin-bitbucket` uses for repository/PR references.
- **Issue watchers.** A structured filter creates one Kandev task per newly matching
  issue, deduplicated by `(issue_watch_id, issue_id)`, with a per-watch
  `maxInflightTasks` throttle. Watcher-created tasks are plugin-owned
  (`Tasks().Create` with `plugin:<id>` provenance), so `PluginOwnedTaskTrees` cascade
  delete cleans them up if the watch or connection is removed.
- **Settings UI.** `registerIntegrationSettings({ id: "redmine", label, description,
  icon, Component })` contributes the connection form, project picker, field-mapping
  table, sync-option toggles, and watcher management — natively rendered inside the
  Kandev SPA, not an iframe.

## Capability matrix

| Capability | Redmine |
| --- | --- |
| API auth | API key (`X-Redmine-API-Key`), no OAuth |
| Connection scope | One per Kandev workspace |
| Project selection | Yes, multi-select |
| Field mapping (status/tracker/priority) | Yes, read live, never hardcoded |
| Custom fields | Listed when admin; derived (with UI note) otherwise |
| Issue read | Yes, `status_id=*` always sent |
| Issue write (create/update) | Yes |
| Attachments | Yes, two-step upload-token flow |
| Task linking | Yes, via shared host Link dialog |
| Inbound sync (status/title/description) | Yes, polling, cursor-based |
| Outbound write-back | Opt-in per connection (`autoStatusWriteback`, `syncTitleDescription`) |
| Composer `#` mentions | Yes, via `reference_sources` |
| Issue watchers | Yes, with throttle cap |
| Webhooks | No (Redmine core has none) |
| MCP exposure | No |
| Time entries / wiki / repository endpoints | No |

## Connection, permissions, and secrets

Connection state is one of `disconnected | connecting | connected | degraded`. A
connection is `connected` once `GET /users/current.json` succeeds; `degraded` when the
last health probe failed but the key has not been explicitly removed; `disconnected`
when no config exists.

Secrets: the plugin encrypts the Redmine API key with workspace-derived key material
before calling `SetSecret("redmine:<workspace_id>:api_key", ...)` — the workspace ID
is folded into the secret key itself, since the host's `GetSecret`/`SetSecret`/
`DeleteSecret` RPCs are namespaced only by plugin ID, not by workspace. Rotation
replaces the stored ciphertext under the same key; revocation calls `DeleteSecret` and
clears the `plugin_state` connection row. `GetConfig` (which returns secret values in
cleartext to the plugin process) is never logged in full, and the API key never
appears in a host RPC response, log line, or task metadata.

Authorization: only the workspace that owns a connection may read, modify, or use it.
The plugin enforces this itself since the host's plugin RPC surface has no
per-workspace secret partition — every read/write path derives its `plugin_state` and
secret keys from the caller's own workspace ID, never from a request-body-supplied
one, closing the same body-supplied-workspace class of bug the native integrations'
`AGENTS.md` warns about for config-copy-style operations.

Network: direct outbound HTTPS from the plugin process to the configured base URL,
following the `kandev-plugin-bitbucket` self-hosted pattern (validated host, no
outbound connector, no skip-TLS-verify toggle, `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`
honored for free by Go's default transport).

## API and host contracts

No new host contracts are required. This plugin is built entirely on generic seams
already shipped in `kdlbs/kandev` (landed by PR #2117 and used first by
`kandev-plugin-bitbucket`):

- `registerIntegrationSettings({ id, label, description, icon?, Component })` — the
  settings page, index card, and workspace navigation entry.
- `registerTaskAction({ placement: "link" })` + `host.openTaskLinkDialog` — the shared
  native Link surface; the plugin supplies issue search/resolution, the host renders
  the dialog.
- `reference_sources` dynamic composer search-source registration with submit-time
  reauthorization — for `#` issue mentions.
- `PluginOwnedTaskTrees` (`PreviewPluginOwnedTaskTree` / `DeletePluginOwnedTaskTreeRequest`)
  — cascade delete for watcher-created tasks.
- `Tasks().Create` / `Tasks().Update` with `plugin:<id>` provenance — task creation and
  metadata writes.
- `GetState` / `SetState` (scope `workspace`) — connection metadata, sync cursor,
  health state, echo-suppression bookkeeping.
- `GetSecret` / `SetSecret` / `DeleteSecret` (flat, plugin-ID-scoped) — encrypted API
  key storage, workspace isolation built by the plugin as described above.
- `OnEvent` — none required for v1; sync and watcher polling run on the plugin's own
  timers, not host event subscriptions.

Contracts explicitly **not** used: the provider-neutral Git credential broker and
native repository-provider extensions (source-control-specific; Redmine is an issue
tracker, not a Git host) and `registerReviewProvider` (change-request/PR-shaped; a
Redmine issue is not a code review).

## Failure modes

- Invalid API key → connection state `degraded`, `last_error` set to a message naming
  the rejection; the stored key is not deleted; the settings page shows a reconnect
  banner. The plugin's webhook/action handlers never surface a bare HTTP 401 to the
  Kandev frontend for this case (see the native implementation's discovery that any
  401 from any endpoint triggers the SPA's global session-expiry redirect) — a
  Redmine-credential rejection is reported as a distinct plugin action error code, not
  a host-level 401.
- REST API disabled on the Redmine instance (403 on `/users/current.json`) → distinct
  error naming Administration > Settings > API.
- Unreachable host → distinct error, does not overwrite a previously-healthy
  connection's stored state.
- 429 / 5xx from Redmine → capped exponential backoff with jitter; the sync cursor and
  health state are preserved, never advanced past unread changes.
- A Redmine outage does not prevent the Kandev host from starting or degrade any other
  plugin or integration — failures are isolated to this plugin's own process per the
  host's plugin supervision model.
- Disabling the plugin preserves its config, state, secrets, and watcher data;
  uninstalling removes them, including cascading deletion of watcher-created task
  trees via `PluginOwnedTaskTrees`.

## Persistence guarantees

- Connection metadata, sync cursor, and health state live in host `plugin_state`
  scoped to `workspace` — durable across plugin restarts and upgrades.
- The Redmine API key lives only in the host's encrypted secret store, keyed by the
  plugin-composed `redmine:<workspace_id>:api_key` string.
- Issue-watch dedup keys `(issue_watch_id, issue_id)` and echo-suppression fields
  (`last_pushed_status_id`, `last_pushed_title`, `last_pushed_description_hash`) are
  plugin-owned durable state — either `plugin_state` or the plugin's own
  `KANDEV_PLUGIN_DATA_DIR`-backed store, whichever the implementation task settles on;
  either way they survive restarts so a redeployed plugin does not re-create
  already-seen watcher tasks or replay an already-applied write-back.
- Uninstalling the plugin deletes `KANDEV_PLUGIN_DATA_DIR` and all `plugin_state`/
  secret entries for it; disabling preserves all of the above.

## Scenarios

**GIVEN** a workspace with no Redmine connection
**WHEN** the user submits a base URL and a valid API key in the plugin's settings page
**THEN** the plugin validates against `GET /users/current.json`, stores the encrypted
key, and the connection state becomes `connected`

**GIVEN** a workspace with no Redmine connection
**WHEN** the user submits a base URL and an invalid API key
**THEN** the plugin reports a distinct rejection error, the connection state stays
`disconnected`, and nothing is persisted

**GIVEN** a connected workspace
**WHEN** the Redmine REST API is disabled server-side
**THEN** the plugin reports a distinct "API disabled" error naming where to re-enable
it

**GIVEN** a connected workspace with projects selected
**WHEN** the user opens the field-mapping UI
**THEN** it lists live statuses, trackers, and priorities fetched from the instance,
with no hardcoded names anywhere in the plugin source

**GIVEN** a Kandev task
**WHEN** the user links it to a Redmine issue via the native Link action
**THEN** the shared host Link dialog opens, the plugin resolves the issue, and the
task carries the issue id/url afterward

**GIVEN** a linked task and a Redmine issue whose status changes to a mapped, closed
status
**WHEN** the next sync poll runs
**THEN** the Kandev task transitions to the mapped workflow step, proving `status_id=*`
was sent (closed issues are not silently dropped)

**GIVEN** `autoStatusWriteback` enabled on a connection
**WHEN** a linked task moves to a workflow step with a status mapping
**THEN** the plugin issues `PUT /issues/:id.json` with the mapped status within one
event-loop turn, and the following inbound poll does not re-apply or bounce the change

**GIVEN** `autoStatusWriteback` disabled
**WHEN** a linked task moves to a mapped workflow step
**THEN** no outbound PUT is issued; the manual "Set Redmine status" action still works

**GIVEN** an issue watch with `maxInflightTasks: 1` and one open watcher-created task
**WHEN** a second matching issue appears
**THEN** no second task is created until the first closes

**GIVEN** a connection at risk of duplicate watcher tasks
**WHEN** the same issue is observed across two consecutive polls
**THEN** exactly one task exists for it, deduplicated by `(issue_watch_id, issue_id)`

**GIVEN** an active connection
**WHEN** the API key is rotated via the settings page
**THEN** the new key replaces the old one and existing sync/watch state is unaffected

**GIVEN** an active connection
**WHEN** the user deletes the connection
**THEN** the secret and all connection-scoped `plugin_state` are removed and syncing
stops

**GIVEN** the plugin is uninstalled
**WHEN** the uninstall completes
**THEN** all watcher-created task trees are cascade-deleted via `PluginOwnedTaskTrees`,
and all secrets/state/data-dir content are removed

**GIVEN** the plugin is merely disabled (not uninstalled)
**WHEN** it is re-enabled later
**THEN** its connection, mappings, watches, and sync cursor are all still present and
functional

**GIVEN** a workspace typing `#` in the task composer
**WHEN** they search for a Redmine issue
**THEN** matching issues from that workspace's connected projects appear via the
plugin's `reference_sources` registration, reauthorized at submit time

## Out of scope

- Webhook-based real-time sync (Redmine core has no webhooks; polling is the
  baseline).
- Time-entry tracking, wiki, and repository endpoints.
- Exposing Redmine tools to agents over MCP.
- `X-Redmine-Switch-User` attribution or any per-user credential tier — a single
  workspace-scoped API key attributes all plugin-authored changes.
- An outbound connector, relay, or on-prem deployment path for unreachable instances.
- Per-connection proxy fields, custom-CA, or skip-TLS-verify options.
- Mapping Redmine statuses across more than one Kandev workflow per connection.
- Syncing journals/comments, relations, or attachments *inbound* onto Kandev tasks
  (read-only display in the issue dialog only).
- Redmine as a source-control provider or a repository picker entry.
- A standalone "My Redmine" browse page (no AC requires it; deferred to a follow-up if
  requested).

## Implementation plan

See [`../../plans/redmine-plugin/plan.md`](../../plans/redmine-plugin/plan.md).
