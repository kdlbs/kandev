---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001
created: 2026-07-31
owners:
  - kandev
---
# Bitbucket Connector Plugin System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Teams using Bitbucket Cloud or Bitbucket Data Center need repository, pull-request,
and task workflows without putting Bitbucket API knowledge or credentials into the
Kandev host. The connector must feel native where users already create, link, review,
and reference work, while remaining independently releasable as an official plugin.

## What

- Kandev ships the official `kandev-plugin-bitbucket` from its dedicated public
  repository. Its manifest pins `min_kandev_version` to the first released host
  version containing the required generic contracts; it never guesses an unreleased
  version.
- The initial release follows Kandev's current unsigned marketplace trust contract:
  the generated internal `checksums.txt` is mandatory, the host reports the package
  as unsigned, and neither repository claims cryptographically verified publisher
  provenance. Signing is future host-wide work rather than a Bitbucket release gate,
  as recorded in
  [ADR-2026-08-01-bitbucket-initial-release-remains-unsigned](../../../decisions/2026-08-01-bitbucket-initial-release-remains-unsigned.md).
- The plugin supports Bitbucket Cloud and Bitbucket Data Center through separate
  adapters behind one Bitbucket domain. Cloud and Data Center have full capability
  parity wherever their APIs provide an equivalent operation. Capability flags hide
  unavailable controls or explain version-specific limits; the UI never presents a
  non-working equivalent.
- A workspace connects with Cloud API token or OAuth 2.0, and with Data Center
  personal/HTTP access token or OAuth 2.0 when its administrator configured an
  incoming OAuth application link. Cloud app passwords are not accepted. OAuth client
  registrations are bring-your-own per workspace; no client secret ships in the public
  plugin.
- The plugin owns Bitbucket REST payloads, OAuth and token rules, product/version
  probes, pagination, rate-limit handling, health polling, secret refresh, connector
  screens, watches, normalized review/status data, and provider actions. The host owns
  reusable authenticated action, repository-provider, task-action, review-provider,
  reference-source, task ownership, credential-broker, and code-host presentation
  contracts. GitHub, GitLab, and compatible plugins use the same host-rendered task
  status and review-detail anatomy rather than provider-owned approximations.
- Users browse/search repositories, select branches, inspect pull-request URLs, launch
  tasks from pull requests, link/unlink existing tasks, and create pull requests from
  a task checkout branch. Remote descriptors preserve the exact credential-free clone
  URL, including Data Center context paths.
- Native **Create PR** remains conditional on a task checkout containing commits and
  having no linked change request. For a registered provider, Kandev keeps the shared
  dialog, pushes the verified checkout branch through its normal Git path, and then
  invokes the provider's authenticated create callback. Unsupported provider options,
  such as draft creation, are hidden rather than silently ignored. This boundary is
  recorded in
  [ADR-2026-08-10-plugin-change-request-mutations](../../../decisions/2026-08-10-plugin-change-request-mutations.md).
- Plugin task actions appear in native task surfaces. Inside the existing **Link**
  submenu, the required entry is named **Bitbucket Pull Request**; it never repeats the
  parent verb as “Link Bitbucket Pull Request.” Selecting it opens the same host-owned
  task change-request dialog used by GitHub: title and explanatory description, one
  labeled input, inline validation, Cancel and Save footer actions, disabled/submitting
  state, success toast, and close-on-success. Provider code owns reference parsing and
  the authenticated link mutation, not modal anatomy. Desktop and mobile expose the same
  capability and host-native dialog behavior.
- Bitbucket pull requests appear through the native review-provider registry in desktop
  selectors, dock/detail panels, task center, and mobile review navigation. The plugin
  maps Bitbucket detail, checks, reviews, comments, and capabilities into the host's
  provider-neutral change-request model and renders the same host-owned detail component
  as GitHub. The plugin supplies callbacks for supported mutations; the host owns header,
  sections, action placement, loading/error anatomy, scrolling, and responsive behavior.
  Unsupported Cloud/Data Center capabilities are omitted or explained without changing
  the shared layout.
- Linked Bitbucket pull requests expose host-owned unlink controls in shared desktop
  popovers/review anatomy and touch-sized mobile surfaces. Unlink removes only the
  selected task association, refreshes review/status data, and immediately removes the
  corresponding task indicator without deleting the remote pull request. An explicit
  unlink suppresses source-branch auto-link for that task/pull-request pair until a
  manual relink and detaches watch ownership without deleting the task.
- Composer `#` search consumes the plugin's dynamically registered
  `bitbucket`/`pull_request` reference source. Kandev constructs canonical reference
  identity, and every selected reference is reauthorized by the live plugin at message
  submission so stale, tampered, cross-workspace, or disabled-plugin selections fail
  closed.
- **Settings > Integrations > Bitbucket** provides connection/authentication, health,
  and saved-watch management through the plugin's native integration-settings
  contribution. The `/bitbucket` page uses the same host-owned dashboard primitives
  as `/github` and `/gitlab`: compact scope controls, result title/count, repository
  and query filters, refresh, and one full-width bordered pull-request
  list. Rows use the same inert-container, external-title, metadata, linked-task, and
  sole **Task** preset-menu anatomy. Preset selection opens the native
  `TaskCreateDialog` directly; no Bitbucket launch intermediary or row-level Review
  action exists. Dashboard scope selection commits its normalized filter into the
  visible query field, matching GitHub/GitLab; manually committed query changes and
  the active scope always drive the same provider request. Scope and pull-request
  state glyphs use host-owned semantic integration icons rather than plugin-drawn
  copies. Review remains in normal linked-task desktop/mobile review surfaces.
  The preset menu uses the exact first-party glyphs: eye for **Review**, message for
  **Address feedback**, and tool for **Fix CI**.
  Committed custom queries and repository filters can be named, saved, selected, and
  deleted with the same scope-bar workflow as GitHub/GitLab. Saved queries are
  per-user, per-workspace plugin state through `host.storage`; saving commits and stores
  the visible query draft even when the user has not pressed Enter first.
  Existing workspace repositories use the dialog's normal REST create path. First-use
  Bitbucket repositories use the dialog's optional create transport through the
  authenticated `tasks.launch` action; the server resolves the repository descriptor
  from the connected pull request and uses a per-dialog launch ID for retry-only
  deduplication. Browser payloads cannot nominate a trusted repository descriptor, and
  separate Task-menu launches may create separate tasks for the same pull request.
  After a Bitbucket repository is persisted, the dialog's native branch picker routes
  through the plugin's workspace-scoped `repositories.branches` action. The host derives
  its descriptor from that repository row, so the picker has Cloud/Data Center branches
  without a GitHub fallback, local-clone dependency, or browser-selected authority.
  This boundary is recorded in
  [ADR-2026-08-06-plugin-code-host-dashboard-parity](../../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md).
  Watch creation and management remain in **Settings > Integrations > Bitbucket**;
  the dashboard does not add a provider-specific Watch button absent from GitHub/GitLab.
- Watches use authenticated polling, not Bitbucket event webhooks. OAuth callback is
  the only required public webhook in v1. A watch writes a durable `creating`
  reservation before task creation, stamps plugin/watch/external-PR ownership, and
  reconciles unfinished reservations after restart. Reset/delete previews its cascade
  and deletes only tasks owned by this plugin/watch; manually linked or adopted tasks
  remain.
- Manual links, pull requests created from a task, and watch-created tasks all expose
  one durable task-to-pull-request association to the dashboard, native Review
  provider, and task-topbar status control. Restart recovery preserves watch-owned
  associations without duplicating them into manual-link state.
- Task refresh detects an externally created open pull request by exact source-branch
  match against each host-verified Bitbucket checkout and persists that association
  idempotently. The browser cannot nominate the repository or branch, and a transient
  discovery failure never removes or hides existing links.
- A linked Bitbucket pull request contributes normalized status through its registered
  review-provider snapshot. Kandev automatically renders that status in both places used
  by GitHub: the compact task-topbar pull-request control and the CI chip above the chat
  composer (including passthrough sessions). Both use the exact shared trigger and
  popover/drawer body, not a plugin slot or provider-owned lookalike. Desktop hover uses
  the same delay; touch/mobile uses the same bounded, internally scrollable drawer.
  Opening either surface refreshes immediately, the provider polls in the background
  around every 90 seconds, and clicking through opens the registered Bitbucket Review
  surface. Cloud Pipelines and Data Center build statuses are normalized from the pull
  request's source/head commit, never its destination commit.
- The review provider exposes one bounded workspace association snapshot. Kandev uses
  it to render its semantic pull-request glyph beside linked tasks in task switcher,
  Kanban, and rich task-list rows, without issuing one provider request per task. On
  mouse hover or keyboard focus, that glyph opens the same host-owned structured
  pull-request summary used by GitHub (number, title, review, CI, and available state),
  resolving detail lazily through the registered review-provider snapshot. A plain
  count-only tooltip is not parity. Touch/mobile continues through the native task
  status and Review surfaces; no required information or action is hover-only.
- Clone, fetch, and push resolve through the provider-neutral short-lived credential
  broker. Secrets never appear in clone URLs, task metadata/state, environment
  variables, command arguments, logs, or executor payloads.
  Initial host-side materialization carries the same host-derived workspace, task,
  active-session, repository, exact-host, and exact-path scope to the plugin; an
  incomplete scope is rejected instead of falling back to a workspace-only secret.
  Existing managed checkouts are refreshed through that exact scope before worktree
  creation; local and worktree execution then use refreshed refs without a second
  network operation.

## Capability matrix

| Capability                                                                        | Cloud                   | Data Center                                             |
| --------------------------------------------------------------------------------- | ----------------------- | ------------------------------------------------------- |
| API token/PAT connection and health                                               | Required                | Required                                                |
| OAuth 2.0 connect and refresh                                                     | Required                | Required when an incoming OAuth application link exists |
| Repository/project browse and search                                              | Required                | Required                                                |
| Native repository picker, branch selection, and PR URL inspection                 | Required                | Required                                                |
| Scoped clone/fetch/push credential broker                                         | Required                | Required                                                |
| Launch task, link/unlink, and create PR from task                                 | Required                | Required                                                |
| Native desktop/mobile review panel                                                | Required                | Required                                                |
| Composer `#` pull-request search and submitted context                            | Required                | Required                                                |
| Queue, files/diff, commits, comments/threads, reviewers, approvals, merge/decline | Required                | Required where server API supports thread semantics     |
| Status presentation                                                               | Pipelines               | Commit/build status                                     |
| Watches with deduplicated task creation                                           | Required                | Required                                                |
| Bitbucket Issues                                                                  | Not supported; use Jira | Not supported; use Jira                                 |

## Connection, permissions, and secrets

Connection state is `unconfigured`, `checking`, `connected`, `auth_required`, or
`unavailable`. Workspace-scoped encrypted plugin secrets hold token/PAT credentials,
OAuth registrations, grants, and rotating refresh tokens. State stores only non-secret
connection metadata and an atomically rotated credential generation. Refresh is
singleflight per workspace/generation/connection epoch. Disconnect or connection
replacement removes completed refresh data and fences an exchange already in flight;
that stale result cannot be returned, cached, or persisted. Logs and returned errors
redact headers, query parameters, and secret values.

The host authenticates browser actions through normal Kandev session middleware and
authorizes every claimed workspace, task, and repository before dispatch. It derives
task-to-workspace relationships server-side. Plugins receive verified actor/resource
context separately from bounded untrusted JSON. The connector can create or cascade-
delete only task trees stamped `metadata.source = "plugin:kandev-plugin-bitbucket"`;
pre-existing task links remain plugin state rather than host-owned task provenance.

Data Center accepts private-network installations intentionally. Production connections
require HTTPS, reject URL credentials, retain redirect origin, and enforce connect/read
timeouts and response-size limits. HTTP is development-only behind an explicit setting.

## API and host contracts

The required generic seams are defined by
[authenticated plugin actions](../../../decisions/2026-07-31-authenticated-plugin-actions.md),
[repository-provider extensions](../../../decisions/2026-07-31-plugin-repository-provider-extensions.md),
and [provider-neutral git credential brokerage](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md).
Their frozen protocol/UI references are
[`GRPC-CONTRACT.md`](../../../plans/plugins/GRPC-CONTRACT.md),
[`HOST-DATA-API.proto`](../../../plans/plugins/HOST-DATA-API.proto), and
[`PLUGIN-API.md`](../../../plans/plugins/PLUGIN-API.md).

The plugin declares actions including `connection.get`, `connection.save`,
`oauth.start`, `repositories.list`, `pullrequests.get`, and `watches.update`; each
has a resource scope and bounded body. It declares ownership of provider ID
`bitbucket` and the `bitbucket`/`pull_request` reference source. The browser calls
declared actions only through the authenticated host action route; public plugin
webhooks remain reserved for provider callbacks.

## Failure modes

- A disabled, unavailable, or timed-out action plugin returns a bounded actionable
  error; it does not expose the public webhook route as a browser fallback.
- A failed connection, expired credential, or refresh denial transitions visibly to
  `auth_required` or `unavailable`; existing non-secret connection metadata stays
  intact. Health probes run around every 90 seconds with jitter/backoff.
- Cloud rate limits and Data Center transient failures use adapter-owned bounded retry/
  backoff. Unsupported product/version capabilities show an unsupported explanation.
- An invalid or incomplete provider descriptor, host/path mismatch, stale broker
  lease, disabled provider, or submission reauthorization denial fails closed.
- Watch create/recovery failures preserve durable reservations and surface the last
  error. A reset/delete never removes a task whose plugin provenance does not match.

## Persistence guarantees

Plugin state persists connection metadata, capability probe result, watches, cursors,
dedupe keys, reservations, links, and recovery/error state through the Host state API.
Durable pull-request identity is provider ID plus provider connection scope plus
immutable repository ID plus pull-request number; repository namespace/path and display
keys are never dedupe authority. Provider cursors are bound to that immutable identity
and their normalized query. Connection replacement pauses a bound watch before any
provider request; explicit resume rebinds it and clears stale pagination state.
Encrypted secrets persist separately. Credential broker leases are short-lived and
non-durable; they are re-resolved from the live plugin and revoked on task/session/
workspace teardown, plugin disable, connection reset, or credential-generation change.
The plugin exposes that generation only as an opaque, non-secret credential binding;
the host checks it before and after each lease redemption and fails closed when absent
or changed. Plugin disable, error, or uninstall also immediately revokes every lease
for its declared provider; exact repository path matching remains case-sensitive.
